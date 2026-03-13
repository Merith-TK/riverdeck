// Package editorserver provides the Riverdeck layout editor API as an
// http.Handler suitable for embedding inside a Wails AssetServer or any
// other in-process HTTP mux.  No TCP listener is opened.
//
// Usage:
//
//	srv := editorserver.New(editorserver.Config{...})
//	h   := srv.Handler()   // returns http.Handler for /api/* routes
package editorserver

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/merith-tk/riverdeck/pkg/layout"
	"github.com/merith-tk/riverdeck/pkg/resolver"
	"github.com/merith-tk/riverdeck/pkg/scripting"
	"gopkg.in/yaml.v3"
)

// DeviceDimensions describes the physical Stream Deck geometry exposed over
// the API so the editor can draw the correct grid size.
type DeviceDimensions struct {
	Cols         int    `json:"cols"`
	Rows         int    `json:"rows"`
	Keys         int    `json:"keys"`
	ModelName    string `json:"model_name"`
	ReservedKeys []int  `json:"reserved_keys,omitempty"`
}

// Config holds the constructor parameters for Server.
type Config struct {
	// ConfigDir is the Riverdeck config directory (layout.json lives here).
	ConfigDir string

	// Packages is the list of installed packages (may be nil).
	Packages []*scripting.ScannedPackage

	// Device describes the connected Stream Deck dimensions.
	Device DeviceDimensions

	// OnLayoutSaved is called (in a new goroutine) after a successful
	// POST /api/layout so the main app can reload the navigator.
	// May be nil.
	OnLayoutSaved func(l *layout.Layout)

	// GetMode returns the current navigation style string ("folder", "layout",
	// "auto").  Used by GET /api/mode.  May be nil (returns "folder").
	GetMode func() string

	// OnModeChanged is called when the editor POSTs a new navigation style via
	// POST /api/mode.  May be nil (mode changes are ignored).
	OnModeChanged func(style string)
}

// Server holds the editor API state.
type Server struct {
	cfg    Config
	mu     sync.RWMutex
	layout *layout.Layout // current in-memory layout (nil until first load)
}

// New creates a new Server with the given configuration.
func New(cfg Config) *Server {
	s := &Server{cfg: cfg}
	// Pre-load the layout so it's available immediately.
	if lay, err := layout.Load(cfg.ConfigDir); err == nil && lay != nil {
		s.layout = lay
	}
	return s
}

// UpdateLayout replaces the in-memory layout so the editor always serves
// fresh data.
func (s *Server) UpdateLayout(l *layout.Layout) {
	s.mu.Lock()
	s.layout = l
	s.mu.Unlock()
}

// UpdatePackages replaces the installed packages list.
func (s *Server) UpdatePackages(pkgs []*scripting.ScannedPackage) {
	s.mu.Lock()
	s.cfg.Packages = pkgs
	s.mu.Unlock()
}

// Handler returns an http.Handler that serves all /api/* routes.  Static
// editor assets are served by the Wails AssetServer, not by this handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	return mux
}

// ── Route registration ────────────────────────────────────────────────────────

func (s *Server) registerRoutes(mux *http.ServeMux) {
	// API routes (static files are served by the Wails AssetServer)
	mux.HandleFunc("/api/layout", s.handleLayout)
	mux.HandleFunc("/api/packages", s.handlePackages)
	mux.HandleFunc("/api/scripts", s.handleScripts)
	mux.HandleFunc("/api/device", s.handleDevice)
	mux.HandleFunc("/api/mode", s.handleMode)
	mux.HandleFunc("/api/file", s.handleFile)
	mux.HandleFunc("/api/folder/assign", s.handleFolderAssign)
	mux.HandleFunc("/api/resource", s.handleResource)
	mux.HandleFunc("/api/icons/", s.handleIcons)
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/app-config", s.handleAppConfig)
	mux.HandleFunc("/api/lua/new", s.handleLuaNew)
	mux.HandleFunc("/api/lua/templates", s.handleLuaTemplates)
}

// ── Handlers ──────────────────────────────────────────────────────────────────

func (s *Server) handleLayout(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.mu.RLock()
		lay := s.layout
		s.mu.RUnlock()
		if lay == nil {
			lay = layout.NewEmpty()
		}
		writeJSON(w, lay)

	case http.MethodPost:
		var incoming layout.Layout
		if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		// Validate before saving.
		if errs := layout.Validate(&incoming); len(errs) > 0 {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"errors": errs})
			return
		}
		if err := layout.Save(s.cfg.ConfigDir, &incoming); err != nil {
			http.Error(w, "save failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		s.mu.Lock()
		s.layout = &incoming
		s.mu.Unlock()
		if s.cfg.OnLayoutSaved != nil {
			go s.cfg.OnLayoutSaved(&incoming)
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handlePackages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Dynamically re-scan packages from disk on every request so the editor
	// always reflects the current set of installed packages.  The result is
	// stored back into s.cfg.Packages so that handleFolderAssign,
	// handleResource, and handleIcons stay in sync.
	freshPkgs, scanErr := scripting.ScanPackages(s.cfg.ConfigDir)
	if scanErr != nil {
		log.Printf("[editorserver] package scan error: %v", scanErr)
	}
	s.mu.Lock()
	s.cfg.Packages = freshPkgs
	s.mu.Unlock()

	s.mu.RLock()
	pkgs := s.cfg.Packages
	s.mu.RUnlock()

	type metaField struct {
		Key         string `json:"key"`
		Label       string `json:"label"`
		Type        string `json:"type,omitempty"`
		Default     string `json:"default,omitempty"`
		Description string `json:"description,omitempty"`
	}
	type templateInfo struct {
		Key            string      `json:"key"`
		Label          string      `json:"label"`
		Description    string      `json:"description,omitempty"`
		IconURL        string      `json:"icon_url,omitempty"`
		HasScript      bool        `json:"has_script"`
		MetadataSchema []metaField `json:"metadata_schema,omitempty"`
	}
	type pkgInfo struct {
		ID          string         `json:"id"`
		Name        string         `json:"name"`
		Version     string         `json:"version"`
		Description string         `json:"description"`
		OrderIndex  int            `json:"order_index,omitempty"`
		Templates   []templateInfo `json:"templates"`
	}

	var result []pkgInfo
	for _, pkg := range pkgs {
		pi := pkgInfo{
			ID:          pkg.Manifest.ID,
			Name:        pkg.Manifest.Name,
			Version:     pkg.Manifest.Version,
			Description: pkg.Manifest.Description,
			OrderIndex:  pkg.Manifest.OrderIndex,
		}
		for _, rt := range pkg.ResolvedTemplates {
			ti := templateInfo{
				Key:         rt.Key,
				Label:       rt.Template.Label,
				Description: rt.Template.Description,
				HasScript:   rt.AbsScript != "",
			}
			if rt.AbsIcon != "" {
				// Expose icon via the /api/resource route using pkg:// URI.
				ti.IconURL = fmt.Sprintf("/api/resource?ref=pkg://%s/%s",
					rt.PackageID, rt.Template.Icon)
			}
			for _, mf := range rt.Template.MetadataSchema {
				ti.MetadataSchema = append(ti.MetadataSchema, metaField{
					Key:         mf.Key,
					Label:       mf.Label,
					Type:        mf.Type,
					Default:     mf.Default,
					Description: mf.Description,
				})
			}
			pi.Templates = append(pi.Templates, ti)
		}
		result = append(result, pi)
	}
	writeJSON(w, result)
}

func (s *Server) handleScripts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var scripts []string
	_ = filepath.WalkDir(s.cfg.ConfigDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.ToLower(filepath.Ext(path)) != ".lua" {
			return nil
		}
		// Skip hidden files and package internals.
		base := filepath.Base(path)
		if len(base) > 0 && base[0] == '.' {
			return nil
		}
		rel, _ := filepath.Rel(s.cfg.ConfigDir, path)
		scripts = append(scripts, rel)
		return nil
	})
	writeJSON(w, scripts)
}

func (s *Server) handleDevice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, s.cfg.Device)
}

// handleMode serves and accepts the navigation style.
// GET /api/mode  -> {"style":"folder"}
// POST /api/mode ← {"style":"layout"}
func (s *Server) handleMode(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		style := "folder"
		if s.cfg.GetMode != nil {
			style = s.cfg.GetMode()
		}
		writeJSON(w, map[string]string{"style": style})

	case http.MethodPost:
		var req struct {
			Style string `json:"style"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		switch req.Style {
		case "folder", "layout", "auto":
		default:
			http.Error(w, "invalid style; must be folder, layout, or auto", http.StatusBadRequest)
			return
		}
		if s.cfg.OnModeChanged != nil {
			go s.cfg.OnModeChanged(req.Style)
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleFile provides read/write access to Lua files in the config directory.
// GET  /api/file?path=relative/path.lua  -> returns file contents as text/plain
// POST /api/file?path=relative/path.lua  ← body is the new file contents
//
// Path traversal is rejected (no ".." components).
// Web Lua references are rejected by the resolver.
func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	rel := r.URL.Query().Get("path")
	if rel == "" {
		http.Error(w, "missing path query parameter", http.StatusBadRequest)
		return
	}
	// Reject traversal attempts.
	clean := filepath.Clean(rel)
	if strings.Contains(clean, "..") {
		http.Error(w, "path traversal not allowed", http.StatusBadRequest)
		return
	}
	// Only .lua files are served/written.
	if strings.ToLower(filepath.Ext(clean)) != ".lua" {
		http.Error(w, "only .lua files are accessible", http.StatusBadRequest)
		return
	}
	abs := filepath.Join(s.cfg.ConfigDir, clean)

	switch r.Method {
	case http.MethodGet:
		data, err := os.ReadFile(abs)
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
			http.Error(w, "read body error: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
			http.Error(w, "mkdir error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if err := os.WriteFile(abs, body, 0644); err != nil {
			http.Error(w, "write error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleFolderAssign copies a package template Lua script into the config dir.
// POST /api/folder/assign
//
//	Body: {"slot": 3, "pkg_id": "riverdeck", "template_key": "pkg://riverdeck/home", "dest_dir": "main"}
//
// Returns: {"path": "main/home.lua"}
func (s *Server) handleFolderAssign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Slot        int    `json:"slot"`
		PkgID       string `json:"pkg_id"`
		TemplateKey string `json:"template_key"`
		DestDir     string `json:"dest_dir"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Re-scan packages so we have the latest state from disk.
	freshPkgs, scanErr := scripting.ScanPackages(s.cfg.ConfigDir)
	if scanErr != nil {
		log.Printf("[editorserver] folder/assign: package scan error: %v", scanErr)
	}
	s.mu.Lock()
	s.cfg.Packages = freshPkgs
	s.mu.Unlock()

	s.mu.RLock()
	pkgs := s.cfg.Packages
	s.mu.RUnlock()

	var absSrc string
	for _, pkg := range pkgs {
		if req.PkgID != "" && pkg.Manifest.ID != req.PkgID {
			continue
		}
		for _, rt := range pkg.ResolvedTemplates {
			if rt.Key == req.TemplateKey {
				absSrc = rt.AbsScript
				break
			}
		}
		if absSrc != "" {
			break
		}
	}
	if absSrc == "" {
		http.Error(w, "template not found", http.StatusNotFound)
		return
	}

	// Sanitise destination directory.
	destDir := filepath.Clean(req.DestDir)
	if strings.Contains(destDir, "..") {
		http.Error(w, "path traversal not allowed", http.StatusBadRequest)
		return
	}

	// Build destination path, avoiding overwrites with a numeric suffix.
	baseName := filepath.Base(absSrc)
	destAbs := filepath.Join(s.cfg.ConfigDir, destDir, baseName)
	if err := os.MkdirAll(filepath.Dir(destAbs), 0755); err != nil {
		http.Error(w, "mkdir error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// Avoid clobbering existing files.
	if _, err := os.Stat(destAbs); err == nil {
		ext := filepath.Ext(baseName)
		base := strings.TrimSuffix(baseName, ext)
		for i := 1; i < 1000; i++ {
			candidate := filepath.Join(s.cfg.ConfigDir, destDir, fmt.Sprintf("%s_%d%s", base, i, ext))
			if _, err2 := os.Stat(candidate); os.IsNotExist(err2) {
				destAbs = candidate
				baseName = filepath.Base(candidate)
				break
			}
		}
	}

	src, err := os.ReadFile(absSrc)
	if err != nil {
		http.Error(w, "read template: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := os.WriteFile(destAbs, src, 0644); err != nil {
		http.Error(w, "write copy: "+err.Error(), http.StatusInternalServerError)
		return
	}

	rel, _ := filepath.Rel(s.cfg.ConfigDir, destAbs)
	rel = filepath.ToSlash(rel)
	writeJSON(w, map[string]string{"path": rel})
}

// handleResource resolves and serves a resource by its URI (pkg://, http://, or file path).
// GET /api/resource?ref=pkg://riverdeck/icons/home.png
// Web references are proxied for images only (Lua is blocked).
func (s *Server) handleResource(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	raw := r.URL.Query().Get("ref")
	if raw == "" {
		http.Error(w, "missing ref query parameter", http.StatusBadRequest)
		return
	}

	ref := resolver.Parse(raw)

	// Block web Lua.
	if ref.Scheme == resolver.SchemeWeb &&
		strings.ToLower(filepath.Ext(ref.RawURL)) == ".lua" {
		http.Error(w, "web Lua resources are forbidden", http.StatusForbidden)
		return
	}

	s.mu.RLock()
	pkgs := s.cfg.Packages
	s.mu.RUnlock()

	// Convert packages to resolver.PackageInfo.
	pinfos := make([]resolver.PackageInfo, 0, len(pkgs))
	for _, pkg := range pkgs {
		pinfos = append(pinfos, resolver.PackageInfo{
			ID:  pkg.Manifest.ID,
			Dir: pkg.Dir,
		})
	}

	resolved, err := resolver.Resolve(ref, s.cfg.ConfigDir, pinfos)
	if err != nil {
		http.Error(w, "resolve error: "+err.Error(), http.StatusNotFound)
		return
	}

	if ref.Scheme == resolver.SchemeWeb {
		// Proxy the web resource.
		resp, err := http.Get(resolved) //nolint:noctx
		if err != nil {
			http.Error(w, "fetch error: "+err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
		return
	}

	http.ServeFile(w, r, resolved)
}

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

// luaTemplate is a starter template for creating new Lua scripts.
type luaTemplate struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Content     string `json:"content"`
}

// builtinLuaTemplates defines the set of available starter templates.
var builtinLuaTemplates = []luaTemplate{
	{
		ID:          "empty",
		Label:       "Minimal Button",
		Description: "Empty button with passive display and trigger handler.",
		Content: `-- Minimal Riverdeck button
local M = {}

function M.passive(ctx)
  ctx.text("Hello")
end

function M.trigger(ctx)
  -- called when the key is pressed
end

return M
`,
	},
	{
		ID:          "background",
		Label:       "Background Worker",
		Description: "Button with a background loop that updates the display.",
		Content: `-- Background worker button
local M = {}
local system = require("system")

function M.passive(ctx)
  local count = (state.count or 0)
  ctx.text(tostring(count))
end

function M.background()
  state.count = (state.count or 0) + 1
  system.sleep(1000)
end

return M
`,
	},
	{
		ID:          "config",
		Label:       "Configurable Button",
		Description: "Button that reads per-button config values set in the editor.",
		Content: `-- Configurable button (uses the config module)
local M = {}
local config = require("config")

function M.passive(ctx)
  local label = config.get("label") or "Click me"
  ctx.text(label)
end

function M.trigger(ctx)
  local action = config.get("action") or "none"
  -- perform action ...
end

return M
`,
	},
	{
		ID:          "toggle",
		Label:       "Toggle Button",
		Description: "Button that toggles between two states on each press.",
		Content: `-- Toggle button
local M = {}

function M.passive(ctx)
  if state.on then
    ctx.text("ON")
    ctx.color(0, 200, 0)
  else
    ctx.text("OFF")
    ctx.color(200, 0, 0)
  end
end

function M.trigger(ctx)
  state.on = not state.on
end

return M
`,
	},
}

// handleLuaTemplates returns the list of available starter templates.
// GET /api/lua/templates -> [{id, label, description, content}, ...]
func (s *Server) handleLuaTemplates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, builtinLuaTemplates)
}

// handleLuaNew creates a new Lua file from a starter template.
//
// POST /api/lua/new
//
//	Body: {"name": "my_button", "template": "empty", "dir": "subfolder"}
//	name:     filename without extension (required)
//	template: template ID from /api/lua/templates (default: "empty")
//	dir:      subdirectory within config dir (default: root)
//
// Returns: {"path": "subfolder/my_button.lua"}
func (s *Server) handleLuaNew(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Name     string `json:"name"`
		Template string `json:"template"`
		Dir      string `json:"dir"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate name.
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	// Restrict to safe filename characters.
	for _, ch := range req.Name {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == '_' || ch == '-') {
			http.Error(w, "name may only contain letters, digits, underscore, and dash", http.StatusBadRequest)
			return
		}
	}

	// Find the template content.
	if req.Template == "" {
		req.Template = "empty"
	}
	var content string
	for _, t := range builtinLuaTemplates {
		if t.ID == req.Template {
			content = t.Content
			break
		}
	}
	if content == "" {
		http.Error(w, "unknown template: "+req.Template, http.StatusBadRequest)
		return
	}

	// Resolve destination path.
	dir := filepath.Clean(req.Dir)
	if strings.Contains(dir, "..") {
		http.Error(w, "path traversal not allowed", http.StatusBadRequest)
		return
	}
	filename := req.Name + ".lua"
	destAbs := filepath.Join(s.cfg.ConfigDir, dir, filename)

	if err := os.MkdirAll(filepath.Dir(destAbs), 0755); err != nil {
		http.Error(w, "mkdir error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Refuse to overwrite.
	if _, err := os.Stat(destAbs); err == nil {
		http.Error(w, "file already exists: "+filename, http.StatusConflict)
		return
	}

	if err := os.WriteFile(destAbs, []byte(content), 0644); err != nil {
		http.Error(w, "write error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	rel, _ := filepath.Rel(s.cfg.ConfigDir, destAbs)
	rel = filepath.ToSlash(rel)
	writeJSON(w, map[string]string{"path": rel})
}

// handleIcons serves icon images from package icon-pack directories.
// URL format: GET /api/icons/<packageID>/<relative/path>
func (s *Server) handleIcons(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Strip "/api/icons/" prefix.
	rel := strings.TrimPrefix(r.URL.Path, "/api/icons/")
	if rel == "" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	// Find the matching package.
	s.mu.RLock()
	pkgs := s.cfg.Packages
	s.mu.RUnlock()

	parts := strings.SplitN(rel, "/", 2)
	if len(parts) < 2 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	pkgID, iconRel := parts[0], parts[1]
	for _, pkg := range pkgs {
		if pkg.Manifest.ID != pkgID {
			continue
		}
		for _, dir := range pkg.ResolvedIconPackDirs {
			candidate := filepath.Join(dir, filepath.FromSlash(iconRel))
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				http.ServeFile(w, r, candidate)
				return
			}
		}
	}
	http.Error(w, "not found", http.StatusNotFound)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		log.Printf("[editorserver] JSON encode error: %v", err)
	}
}
