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
