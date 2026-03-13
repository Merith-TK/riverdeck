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
	"net/http"
	"sync"

	"github.com/merith-tk/riverdeck/pkg/layout"
	"github.com/merith-tk/riverdeck/pkg/scripting"
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

func (s *Server) registerRoutes(mux *http.ServeMux) {
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
	mux.HandleFunc("/api/custom-template", s.handleCustomTemplate)
	mux.HandleFunc("/api/custom-template/file", s.handleCustomTemplateFile)
	mux.HandleFunc("/api/monaco/", s.handleMonaco)
}
