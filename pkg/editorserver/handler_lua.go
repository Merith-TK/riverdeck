package editorserver

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

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
