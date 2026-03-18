package editorserver

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/merith-tk/riverdeck/pkg/scripting"
)

// handlePackagesSub routes /api/packages/{vendor}/{name}/template[/{id}]
func (s *Server) handlePackagesSub(w http.ResponseWriter, r *http.Request) {
	// Strip "/api/packages/" prefix and split path segments.
	rel := strings.TrimPrefix(r.URL.Path, "/api/packages/")
	rel = strings.Trim(rel, "/")
	parts := strings.Split(rel, "/")

	// Expect at least vendor/name/template
	if len(parts) < 3 || parts[2] != "template" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	vendor := parts[0]
	name := parts[1]
	pkgID := vendor + "." + name
	pkgDir := filepath.Join(s.cfg.ConfigDir, ".packages", pkgID)

	switch r.Method {
	case http.MethodPost:
		// POST /api/packages/{vendor}/{name}/template — create/update a template
		var req struct {
			TemplateID  string `json:"templateID"`
			DisplayName string `json:"displayName"`
			Lua         string `json:"lua"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		if req.TemplateID == "" {
			http.Error(w, "templateID required", http.StatusBadRequest)
			return
		}
		// Write the Lua file.
		tmplDir := filepath.Join(pkgDir, "templates")
		if err := os.MkdirAll(tmplDir, 0755); err != nil {
			http.Error(w, "mkdir error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		luaPath := filepath.Join(tmplDir, req.TemplateID+".lua")
		if err := os.WriteFile(luaPath, []byte(req.Lua), 0644); err != nil {
			http.Error(w, "write error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		// Update manifest.json to include the template.
		if err := s.upsertManifestTemplate(pkgDir, pkgID, scripting.ButtonTemplate{
			ID:     req.TemplateID,
			Label:  req.DisplayName,
			Script: "templates/" + req.TemplateID + ".lua",
		}); err != nil {
			http.Error(w, "manifest update error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	case http.MethodDelete:
		// DELETE /api/packages/{vendor}/{name}/template/{id}
		if len(parts) < 4 {
			http.Error(w, "template id required", http.StatusBadRequest)
			return
		}
		tmplID := parts[3]
		luaPath := filepath.Join(pkgDir, "templates", tmplID+".lua")
		_ = os.Remove(luaPath)
		if err := s.removeManifestTemplate(pkgDir, tmplID); err != nil {
			http.Error(w, "manifest update error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// upsertManifestTemplate adds or replaces a ButtonTemplate entry in manifest.json.
func (s *Server) upsertManifestTemplate(pkgDir, pkgID string, tmpl scripting.ButtonTemplate) error {
	manifest := s.loadOrInitManifest(pkgDir, pkgID)
	found := false
	for i, t := range manifest.Provides.Templates {
		if t.ID == tmpl.ID {
			manifest.Provides.Templates[i] = tmpl
			found = true
			break
		}
	}
	if !found {
		manifest.Provides.Templates = append(manifest.Provides.Templates, tmpl)
	}
	return s.writeManifest(pkgDir, manifest)
}

// removeManifestTemplate removes a ButtonTemplate entry from manifest.json.
func (s *Server) removeManifestTemplate(pkgDir, tmplID string) error {
	manifest := s.loadOrInitManifest(pkgDir, filepath.Base(pkgDir))
	filtered := manifest.Provides.Templates[:0]
	for _, t := range manifest.Provides.Templates {
		if t.ID != tmplID {
			filtered = append(filtered, t)
		}
	}
	manifest.Provides.Templates = filtered
	return s.writeManifest(pkgDir, manifest)
}

func (s *Server) loadOrInitManifest(pkgDir, pkgID string) scripting.PackageManifest {
	data, err := os.ReadFile(filepath.Join(pkgDir, "manifest.json"))
	if err != nil {
		return scripting.PackageManifest{ID: pkgID}
	}
	var m scripting.PackageManifest
	if json.Unmarshal(data, &m) != nil {
		return scripting.PackageManifest{ID: pkgID}
	}
	return m
}

func (s *Server) writeManifest(pkgDir string, m scripting.PackageManifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(pkgDir, "manifest.json"), data, 0644)
}

func (s *Server) handlePackages(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.handlePackageCreate(w, r)
		return
	}
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

// handlePackageCreate handles POST /api/packages — create a new local package.
func (s *Server) handlePackageCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Vendor      string `json:"vendor"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Vendor == "" || req.Name == "" {
		http.Error(w, "vendor and name required", http.StatusBadRequest)
		return
	}
	pkgID := req.Vendor + "." + req.Name
	pkgDir := filepath.Join(s.cfg.ConfigDir, ".packages", pkgID)
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		http.Error(w, "mkdir error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	manifest := scripting.PackageManifest{
		ID:          pkgID,
		Name:        req.Name,
		Description: req.Description,
	}
	if err := s.writeManifest(pkgDir, manifest); err != nil {
		http.Error(w, "manifest write error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("[editorserver] created package %s", pkgID)
	writeJSON(w, map[string]string{"id": pkgID})
}

func (s *Server) handleScripts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var scripts []string
	_ = filepath.WalkDir(s.cfg.ConfigDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		base := filepath.Base(path)
		// Skip hidden files/directories (dot-prefixed).
		if len(base) > 0 && base[0] == '.' {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if strings.ToLower(filepath.Ext(path)) != ".lua" {
			return nil
		}
		rel, _ := filepath.Rel(s.cfg.ConfigDir, path)
		scripts = append(scripts, filepath.ToSlash(rel))
		return nil
	})

	// ?tree=1 returns a nested directory tree structure.
	if r.URL.Query().Get("tree") == "1" {
		writeJSON(w, buildScriptTree(scripts))
		return
	}
	writeJSON(w, scripts)
}

// scriptTreeNode is one node in the directory tree returned by GET /api/scripts?tree=1.
type scriptTreeNode struct {
	Name     string            `json:"name"`
	Path     string            `json:"path,omitempty"` // only set for leaf files
	Children []*scriptTreeNode `json:"children,omitempty"`
}

func buildScriptTree(scripts []string) *scriptTreeNode {
	root := &scriptTreeNode{Name: ""}
	for _, s := range scripts {
		parts := strings.Split(s, "/")
		cur := root
		for i, p := range parts {
			if i == len(parts)-1 {
				// leaf file
				cur.Children = append(cur.Children, &scriptTreeNode{Name: p, Path: s})
			} else {
				// directory node
				var found *scriptTreeNode
				for _, c := range cur.Children {
					if c.Name == p && c.Path == "" {
						found = c
						break
					}
				}
				if found == nil {
					found = &scriptTreeNode{Name: p}
					cur.Children = append(cur.Children, found)
				}
				cur = found
			}
		}
	}
	return root
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
