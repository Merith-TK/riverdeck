package editorserver

import (
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// monacoVersion is the pinned Monaco Editor version served by the proxy.
const monacoVersion = "0.52.2"

// monacoCDN is the upstream CDN base URL (no trailing slash).
const monacoCDN = "https://cdn.jsdelivr.net/npm/monaco-editor@" + monacoVersion + "/min"

// handleMonaco serves Monaco Editor assets from a local disk cache, falling
// back to the CDN on the first request for each file.  Subsequent requests
// (including offline) are served entirely from the cache.
//
// URL mapping: GET /api/monaco/vs/loader.js  ->  <cacheDir>/vs/loader.js
//
// Only .js, .css, .svg, .ttf, .woff, .woff2, and .map file extensions are allowed.
func (s *Server) handleMonaco(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Strip the route prefix to get the relative path (e.g. "vs/loader.js").
	rel := strings.TrimPrefix(r.URL.Path, "/api/monaco/")
	if rel == "" || strings.Contains(rel, "..") {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Allow only known safe extensions.
	ext := strings.ToLower(filepath.Ext(rel))
	switch ext {
	case ".js", ".css", ".svg", ".ttf", ".woff", ".woff2", ".map":
		// OK
	default:
		http.Error(w, "forbidden file type", http.StatusForbidden)
		return
	}

	cacheDir := filepath.Join(s.cfg.ConfigDir, ".cache", "monaco", monacoVersion)
	cached := filepath.Join(cacheDir, filepath.FromSlash(rel))

	// Serve from cache if the file already exists.
	if info, err := os.Stat(cached); err == nil && !info.IsDir() {
		serveMonacoFile(w, cached, ext)
		return
	}

	// Not cached -- fetch from CDN.
	cdnURL := monacoCDN + "/" + rel
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(cdnURL)
	if err != nil {
		http.Error(w, "CDN fetch failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		http.Error(w, "CDN returned "+resp.Status, resp.StatusCode)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "CDN read error: "+err.Error(), http.StatusBadGateway)
		return
	}

	// Write to cache (best-effort; serve even if caching fails).
	if mkErr := os.MkdirAll(filepath.Dir(cached), 0755); mkErr == nil {
		_ = os.WriteFile(cached, body, 0644)
	}

	ct := mime.TypeByExtension(ext)
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(body)
}

// serveMonacoFile sends a cached Monaco asset with the correct content type.
func serveMonacoFile(w http.ResponseWriter, path string, ext string) {
	ct := mime.TypeByExtension(ext)
	if ct == "" {
		ct = "application/octet-stream"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, "cache read error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(data)
}
