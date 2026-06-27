package editorserver

import (
	"net/http"

	"github.com/merith-tk/riverdeck/pkg/layout"
)

// handleDevices serves cached device geometry records.
//
// GET /api/devices          → list all known device geometries
// GET /api/devices?id=<id>  → single device geometry
func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	devices, _ := layout.LoadAllDeviceGeometries(s.cfg.ConfigDir)
	if devices == nil {
		devices = []*layout.DeviceGeometry{}
	}
	if id := r.URL.Query().Get("id"); id != "" {
		for _, d := range devices {
			if d.ID == id {
				writeJSON(w, d)
				return
			}
		}
		http.NotFound(w, r)
		return
	}
	writeJSON(w, devices)
}
