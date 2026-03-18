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

	"github.com/merith-tk/riverdeck/pkg/resolver"
	"github.com/merith-tk/riverdeck/pkg/scripting"
)

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

	case http.MethodDelete:
		if err := os.Remove(abs); err != nil {
			if os.IsNotExist(err) {
				http.Error(w, "not found", http.StatusNotFound)
			} else {
				http.Error(w, "delete error: "+err.Error(), http.StatusInternalServerError)
			}
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
