package editorserver

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/merith-tk/riverdeck/pkg/scripting"
)

// customPkgDir returns the absolute path to the local-only _custom package.
func (s *Server) customPkgDir() string {
	return filepath.Join(s.cfg.ConfigDir, ".packages", "_custom")
}

// ensureCustomPackage creates the _custom package directory and manifest if
// they do not already exist.
func (s *Server) ensureCustomPackage() error {
	dir := s.customPkgDir()
	if err := os.MkdirAll(filepath.Join(dir, "templates"), 0755); err != nil {
		return err
	}
	manifest := filepath.Join(dir, "manifest.json")
	if _, err := os.Stat(manifest); err == nil {
		return nil // already exists
	}
	data := []byte(`{
  "id": "_custom",
  "name": "Custom Templates",
  "description": "User-created and duplicated templates stored locally.",
  "version": "1.0.0",
  "order_index": 9999,
  "provides": {
    "templates": []
  }
}
`)
	return os.WriteFile(manifest, data, 0644)
}

// handleCustomTemplate manages custom layout-mode templates.
//
// POST /api/custom-template  — create a new custom template (from scratch or duplicated)
//
//	Body: {"id":"my_btn","label":"My Button","description":"...","source_script":"" or "pkg://...","lua_content":"..."}
//	If source_script is set, its content is copied. Otherwise lua_content or a starter is used.
//	Returns: {"template_key":"pkg://_custom/my_btn"}
//
// DELETE /api/custom-template?id=my_btn  — delete a custom template
//
// GET /api/custom-template  — list all custom templates
func (s *Server) handleCustomTemplate(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listCustomTemplates(w, r)
	case http.MethodPost:
		s.createCustomTemplate(w, r)
	case http.MethodDelete:
		s.deleteCustomTemplate(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) listCustomTemplates(w http.ResponseWriter, _ *http.Request) {
	if err := s.ensureCustomPackage(); err != nil {
		http.Error(w, "init custom package: "+err.Error(), http.StatusInternalServerError)
		return
	}

	manifest, err := s.loadCustomManifest()
	if err != nil {
		http.Error(w, "read manifest: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, manifest.Provides.Templates)
}

func (s *Server) createCustomTemplate(w http.ResponseWriter, r *http.Request) {
	if err := s.ensureCustomPackage(); err != nil {
		http.Error(w, "init custom package: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var req struct {
		ID           string `json:"id"`
		Label        string `json:"label"`
		Description  string `json:"description"`
		SourceScript string `json:"source_script"` // pkg://vendor/template_id — copy content from this
		LuaContent   string `json:"lua_content"`   // raw Lua for brand-new scripts
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate ID: alphanumeric, underscore, dash only.
	req.ID = strings.TrimSpace(req.ID)
	if req.ID == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	for _, ch := range req.ID {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == '_' || ch == '-') {
			http.Error(w, "id may only contain letters, digits, underscore, and dash", http.StatusBadRequest)
			return
		}
	}
	if req.Label == "" {
		req.Label = req.ID
	}

	// Determine Lua content.
	var luaBody string
	if req.SourceScript != "" {
		// Duplicate from an existing package template.
		body, err := s.readTemplateScript(req.SourceScript)
		if err != nil {
			http.Error(w, "read source: "+err.Error(), http.StatusNotFound)
			return
		}
		luaBody = body
	} else if req.LuaContent != "" {
		luaBody = req.LuaContent
	} else {
		// Default starter script.
		luaBody = `-- Custom Riverdeck button
local M = {}

function M.passive(ctx)
  ctx.text("Hello")
end

function M.trigger(ctx)
  -- called when the key is pressed
end

return M
`
	}

	// Write the Lua script.
	scriptRel := "templates/" + req.ID + ".lua"
	scriptAbs := filepath.Join(s.customPkgDir(), scriptRel)
	if err := os.MkdirAll(filepath.Dir(scriptAbs), 0755); err != nil {
		http.Error(w, "mkdir: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := os.WriteFile(scriptAbs, []byte(luaBody), 0644); err != nil {
		http.Error(w, "write script: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Update the manifest to include the new template.
	manifest, err := s.loadCustomManifest()
	if err != nil {
		http.Error(w, "read manifest: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Check for duplicate.
	for _, t := range manifest.Provides.Templates {
		if t.ID == req.ID {
			http.Error(w, "template id already exists: "+req.ID, http.StatusConflict)
			return
		}
	}

	manifest.Provides.Templates = append(manifest.Provides.Templates, scripting.ButtonTemplate{
		ID:          req.ID,
		Label:       req.Label,
		Description: req.Description,
		Script:      scriptRel,
	})

	if err := s.saveCustomManifest(manifest); err != nil {
		http.Error(w, "save manifest: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{
		"template_key": "pkg://_custom/" + req.ID,
	})
}

func (s *Server) deleteCustomTemplate(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id parameter", http.StatusBadRequest)
		return
	}

	manifest, err := s.loadCustomManifest()
	if err != nil {
		http.Error(w, "read manifest: "+err.Error(), http.StatusInternalServerError)
		return
	}

	found := false
	filtered := manifest.Provides.Templates[:0]
	for _, t := range manifest.Provides.Templates {
		if t.ID == id {
			found = true
			// Remove the script file (best-effort).
			if t.Script != "" {
				_ = os.Remove(filepath.Join(s.customPkgDir(), t.Script))
			}
			continue
		}
		filtered = append(filtered, t)
	}
	if !found {
		http.Error(w, "template not found", http.StatusNotFound)
		return
	}
	manifest.Provides.Templates = filtered

	if err := s.saveCustomManifest(manifest); err != nil {
		http.Error(w, "save manifest: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// readTemplateScript resolves a template key (pkg://vendor/id) and returns
// its Lua script content.
func (s *Server) readTemplateScript(templateKey string) (string, error) {
	s.mu.RLock()
	pkgs := s.cfg.Packages
	s.mu.RUnlock()

	for _, pkg := range pkgs {
		for _, rt := range pkg.ResolvedTemplates {
			if rt.Key == templateKey {
				if rt.AbsScript == "" {
					return "", fmt.Errorf("template has no script")
				}
				data, err := os.ReadFile(rt.AbsScript)
				if err != nil {
					return "", err
				}
				return string(data), nil
			}
		}
	}
	return "", fmt.Errorf("template %q not found", templateKey)
}

// loadCustomManifest reads the _custom package manifest.
func (s *Server) loadCustomManifest() (*scripting.PackageManifest, error) {
	data, err := os.ReadFile(filepath.Join(s.customPkgDir(), "manifest.json"))
	if err != nil {
		return nil, err
	}
	var m scripting.PackageManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// saveCustomManifest writes the _custom package manifest.
func (s *Server) saveCustomManifest(m *scripting.PackageManifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.customPkgDir(), "manifest.json"), data, 0644)
}

// handleCustomTemplateFile provides read/write access to a custom template's
// Lua script for the Monaco editor.
//
// GET  /api/custom-template/file?id=my_btn  → Lua source as text/plain
// POST /api/custom-template/file?id=my_btn  ← new Lua source in body
func (s *Server) handleCustomTemplateFile(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" || strings.Contains(id, "..") || strings.ContainsAny(id, "/\\") {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	scriptPath := filepath.Join(s.customPkgDir(), "templates", id+".lua")

	switch r.Method {
	case http.MethodGet:
		data, err := os.ReadFile(scriptPath)
		if os.IsNotExist(err) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "read error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write(data)

	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := os.WriteFile(scriptPath, body, 0644); err != nil {
			http.Error(w, "write error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
