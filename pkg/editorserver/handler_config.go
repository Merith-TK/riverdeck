package editorserver

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/merith-tk/riverdeck/pkg/scripting"
	"gopkg.in/yaml.v3"
)

// handleAppConfig provides read/write access to the application config.yml.
//
// GET  /api/app-config   -> returns config.yml contents as JSON
// POST /api/app-config   <- JSON object; merged into existing config.yml and saved
//
// The file is round-tripped through a yaml.Node tree so that key order and
// indentation from the original file are preserved on every save.
func (s *Server) handleAppConfig(w http.ResponseWriter, r *http.Request) {
	cfgPath := filepath.Join(s.cfg.ConfigDir, "config.yml")

	switch r.Method {
	case http.MethodGet:
		data, err := os.ReadFile(cfgPath)
		if os.IsNotExist(err) {
			writeJSON(w, map[string]interface{}{})
			return
		}
		if err != nil {
			http.Error(w, "read error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		var cfg map[string]interface{}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			http.Error(w, "parse error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if cfg == nil {
			cfg = map[string]interface{}{}
		}
		writeJSON(w, cfg)

	case http.MethodPost:
		var patch map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		// Load the existing file into a yaml.Node tree so we can patch
		// individual keys without disturbing the original ordering or style.
		var docNode yaml.Node
		if data, err := os.ReadFile(cfgPath); err == nil && len(data) > 0 {
			_ = yaml.Unmarshal(data, &docNode)
		}
		// Ensure we have a DocumentNode wrapping a MappingNode.
		if docNode.Kind != yaml.DocumentNode || len(docNode.Content) == 0 {
			docNode = yaml.Node{
				Kind: yaml.DocumentNode,
				Content: []*yaml.Node{
					{Kind: yaml.MappingNode, Tag: "!!map"},
				},
			}
		}
		mapping := docNode.Content[0]
		if mapping.Kind != yaml.MappingNode {
			mapping = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			docNode.Content[0] = mapping
		}

		// Merge patch: for each section, update only the keys present in the
		// patch so original keys and ordering are preserved.
		for sectionKey, sectionVal := range patch {
			sectionMap, isMap := sectionVal.(map[string]interface{})
			if !isMap {
				// Scalar top-level key.
				yamlSetKey(mapping, sectionKey, sectionVal)
				continue
			}
			// Find the existing section node.
			secNode := yamlFindValue(mapping, sectionKey)
			if secNode == nil || secNode.Kind != yaml.MappingNode {
				// Section absent or not a map: set it wholesale.
				yamlSetKey(mapping, sectionKey, sectionMap)
				continue
			}
			// Section exists: update only the keys that appear in the patch.
			for k, v := range sectionMap {
				yamlSetKey(secNode, k, v)
			}
		}

		// Write back preserving 2-space indentation.
		// Encode the mapping node directly (not the document wrapper) to avoid
		// emitting a leading "---" document marker.
		f, err := os.Create(cfgPath)
		if err != nil {
			http.Error(w, "create error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		enc := yaml.NewEncoder(f)
		enc.SetIndent(2)
		encErr := enc.Encode(mapping)
		closeErr := enc.Close()
		f.Close()
		if encErr != nil {
			http.Error(w, "encode error: "+encErr.Error(), http.StatusInternalServerError)
			return
		}
		if closeErr != nil {
			http.Error(w, "close error: "+closeErr.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// yamlFindValue returns the value node for key inside a YAML mapping node,
// or nil if the key is not present.
func yamlFindValue(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// yamlSetKey sets key to val inside a YAML mapping node.  If the key already
// exists the value node is replaced in-place so the original order is kept.
// If the key is absent the key+value pair is appended.
func yamlSetKey(mapping *yaml.Node, key string, val interface{}) {
	// Encode val to YAML bytes then parse into a Node.
	raw, err := yaml.Marshal(val)
	if err != nil {
		return
	}
	var newDoc yaml.Node
	if err := yaml.Unmarshal(raw, &newDoc); err != nil {
		return
	}
	if newDoc.Kind != yaml.DocumentNode || len(newDoc.Content) == 0 {
		return
	}
	newValNode := newDoc.Content[0]

	// Replace in-place if the key already exists.
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content[i+1] = newValNode
			return
		}
	}
	// Key not found — append.
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: key, Tag: "!!str"},
		newValNode,
	)
}

// handleConfig provides read/write access to per-script configuration.
//
// GET  /api/config?path=relative/script.lua
//
//	Returns the .config.json for the given script (or 404 if none).
//
// POST /api/config?path=relative/script.lua
//
//	Saves a new .config.json for the script.
//	Body: {"schema":[...], "overrides":{...}}
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	rel := r.URL.Query().Get("path")
	if rel == "" {
		http.Error(w, "missing path query parameter", http.StatusBadRequest)
		return
	}
	clean := filepath.Clean(rel)
	if strings.Contains(clean, "..") {
		http.Error(w, "path traversal not allowed", http.StatusBadRequest)
		return
	}
	abs := filepath.Join(s.cfg.ConfigDir, clean)

	switch r.Method {
	case http.MethodGet:
		cfg, err := scripting.LoadScriptConfig(abs)
		if err != nil {
			http.Error(w, "read error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if cfg == nil {
			http.Error(w, "no config", http.StatusNotFound)
			return
		}
		writeJSON(w, cfg)

	case http.MethodPost:
		var cfg scripting.ScriptConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := scripting.SaveScriptConfig(abs, &cfg); err != nil {
			http.Error(w, "write error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
