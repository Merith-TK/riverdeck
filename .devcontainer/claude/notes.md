# Claude Working Notes

## Environment
- **Project root**: `/workspace/github.com/Merith-TK/riverdeck/`
- **Test config dir**: `$PWD/.riverdeck` (i.e. `/workspace/github.com/Merith-TK/riverdeck/.riverdeck/`)
- **No symlinks** — `.devcontainer/` is a normal directory
- `go build ./...` always emits `ayatana-appindicator3-0.1` pkg-config warning from `getlantern/systray` — pre-existing, not our error; use `go build ./pkg/...` to isolate real errors

## File Conventions
- Task tracking → `.devcontainer/claude/todo.md`
- Lessons/notes → `.devcontainer/claude/notes.md` (this file)
- MCP config → `.mcp.json` at **project root** (NOT `.claude/mcp.json`)
- Auto-memory → `~/.claude/projects/-workspace-github-com-Merith-TK-riverdeck/memory/`
- Do NOT write memory or tracking files anywhere inside the project directory except `.devcontainer/claude/`

## What Was Built (session 2026-03-16)

### `pkg/wsdevice/` — WebSocket virtual device
- `device.go` — `Device` implements `streamdeck.DeviceIface` over a gorilla WebSocket connection
- `server.go` — HTTP server at `/ws`, upgrades to WebSocket, assigns UUID, calls `onConnect(dev)`
- Protocol (server→client): `devinfo`, `setimage` (base64 PNG), `setkeycolor`, `setbrightness`, `clear`, `reset`
- Protocol (client→server): `keyevent {type, key, pressed}`
- Session resume: `?uuid=<id>` query param

### `pkg/layout/layout.go`
- Added `DeviceLayoutDir(configDir, deviceID) string` → `configDir/devices/{deviceID}`

### `cmd/riverdeck/` — app changes
- `config.go`: `NetworkConfig` + `websocket_enabled` (default false) + `websocket_port` (default 9000)
- `app.go`: `wsServer *wsdevice.Server` field; WS server started in `Init` when enabled + layout/auto nav style
- `wsdevice.go`: `App.runWSDevice(dev)` — per-connection layout loop; seeds per-device layout.json from root; navigation works; **script execution not yet implemented** (logs path only)

### `cmd/mcp-wsdevice/` — MCP test server (3 files)
- `main.go` — server setup + tool registration + ServeStdio
- `state.go` — `deviceState`, WS readLoop, `sendJSON`, helpers
- `tools.go` — 5 tool handlers
- Registered via `.mcp.json` at project root: `go run ./cmd/mcp-wsdevice/`

### MCP Tools
| Tool | Purpose |
|------|---------|
| `rd_connect(port, uuid?, config_dir?)` | Connect; pass `config_dir=/workspace/github.com/Merith-TK/riverdeck/.riverdeck` for local testing |
| `rd_disconnect()` | Close connection |
| `rd_get_state()` | Status, devinfo, key update times, message log |
| `rd_press_key(key)` | key-down + key-up; reports redrawn keys |
| `rd_read_layout()` | Read per-device layout.json |

### go.mod additions
- `github.com/google/uuid` v1.6.0 (direct)
- `github.com/gorilla/websocket` v1.5.3 (direct)
- `github.com/mark3labs/mcp-go` v0.45.0 (direct)

## Architecture Rules
- `LayoutNavigator.configDir` = **global configPath only** — used for script resolution
- Layouts live in a single `layout.json` with named sections; devices reference layout names
- `layout.LoadForDevice(configDir, deviceID)` is the main API — returns the Layout for a device
- `layout.SaveLayout(configDir, name, lay)` saves one named layout without touching others
- Editor server always manages the `"default"` layout (wails binary doesn't know device serial)
- MCP server state is process-global — persists across tool calls in one session

## Layout File Format (v2)
```json
{
  "layouts": {
    "default": { "pages": [...] },
    "gaming":  { "pages": [...] }
  },
  "devices": {
    "ABC123serial": "gaming"
  }
}
```
- Devices not in `"devices"` map fall back to `"default"`
- Old-format files (`{"pages":[...]}`) auto-promoted to `layouts["default"]` on load (read-only compat)
- `devices/` subdirectory approach is GONE — per-device layout.json files are no longer used
