package editorserver

import (
	"encoding/json"
	"net/http"

	"github.com/merith-tk/riverdeck/pkg/pkgmanager"
	"github.com/merith-tk/riverdeck/pkg/platform"
)

// handlePkgInstall handles POST /api/pkg/install
//
// Body: {"url": "github.com/user/repo"}
func (s *Server) handlePkgInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.URL == "" {
		http.Error(w, "url required", http.StatusBadRequest)
		return
	}
	mgr := pkgmanager.New(s.cfg.ConfigDir)
	if err := mgr.Install(req.URL); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handlePkgRemove handles POST /api/pkg/remove
//
// Body: {"url": "github.com/user/repo"}
func (s *Server) handlePkgRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.URL == "" {
		http.Error(w, "url required", http.StatusBadRequest)
		return
	}
	mgr := pkgmanager.New(s.cfg.ConfigDir)
	if err := mgr.Remove(req.URL); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handlePkgUpdate handles POST /api/pkg/update
//
// Body: {"url": "github.com/user/repo"}
func (s *Server) handlePkgUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.URL == "" {
		http.Error(w, "url required", http.StatusBadRequest)
		return
	}
	mgr := pkgmanager.New(s.cfg.ConfigDir)
	if err := mgr.Update(req.URL); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handlePkgList handles GET /api/pkg/list
//
// Returns all installed packages.
func (s *Server) handlePkgList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	mgr := pkgmanager.New(s.cfg.ConfigDir)
	pkgs, err := mgr.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, pkgs)
}

// handlePkgDaemon handles POST /api/pkg/daemon
//
// Body: {"repo": "github.com/user/repo", "sub": "", "enabled": true}
func (s *Server) handlePkgDaemon(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Repo    string `json:"repo"`
		Sub     string `json:"sub"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Repo == "" {
		http.Error(w, "repo required", http.StatusBadRequest)
		return
	}
	packagesDir := platform.PackagesDir(s.cfg.ConfigDir)
	pf, err := pkgmanager.LoadPackages(packagesDir)
	if err != nil {
		http.Error(w, "packages.json load error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	pf.SetDaemonEnabled(req.Repo, req.Sub, req.Enabled)
	if err := pkgmanager.SavePackages(packagesDir, pf); err != nil {
		http.Error(w, "packages.json save error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
