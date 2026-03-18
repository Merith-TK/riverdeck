package editorserver

import (
	"encoding/json"
	"net/http"

	"github.com/merith-tk/riverdeck/pkg/layout"
)

func (s *Server) handleLayout(w http.ResponseWriter, r *http.Request) {
	deviceID := r.URL.Query().Get("device")

	switch r.Method {
	case http.MethodGet:
		if deviceID != "" {
			lay, err := layout.LoadForDevice(s.cfg.ConfigDir, deviceID)
			if err != nil {
				http.Error(w, "load error: "+err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, lay)
			return
		}
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
		s.mu.RLock()
		layoutName := s.layoutName
		s.mu.RUnlock()
		if err := layout.SaveLayout(s.cfg.ConfigDir, layoutName, &incoming); err != nil {
			http.Error(w, "save failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		// If a device ID was provided, update its layout assignment.
		if deviceID != "" {
			if err := layout.AssignDeviceLayout(s.cfg.ConfigDir, deviceID, layoutName); err != nil {
				// non-fatal: log but continue
				_ = err
			}
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
