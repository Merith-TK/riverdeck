# Multi-Device Support with WebSocket Software Clients

## Plan
- [x] Create `pkg/wsdevice/device.go` -- WSDevice implementing DeviceIface over WebSocket
- [x] Create `pkg/wsdevice/server.go` -- HTTP server with /ws endpoint, UUID assignment
- [x] Modify `pkg/layout/layout.go` -- add `DeviceLayoutDir(configDir, id)` helper
- [x] Modify `cmd/riverdeck/config.go` -- extend NetworkConfig with `websocket_enabled` / `websocket_port`
- [x] Modify `cmd/riverdeck/app.go` -- add `wsServer` field, start WS server in Init
- [x] Create `cmd/riverdeck/wsdevice.go` -- `runWSDevice` per-connection layout loop
- [x] Promote `google/uuid` and `gorilla/websocket` to direct deps in go.mod
- [x] Verify: `go build ./pkg/...` and `go vet ./pkg/...` pass clean

## MCP wsdevice tool
- [x] Create `cmd/mcp-wsdevice/main.go` -- Go MCP server acting as WS client
- [x] Add `github.com/mark3labs/mcp-go` as direct dependency
- [x] Register server in `.mcp.json` (project root, stdio transport)
- [x] `go vet ./cmd/mcp-wsdevice/` passes clean

### Tools exposed
| Tool | Purpose |
|------|---------|
| `rd_connect(port, uuid?, config_dir?)` | Connect to WS server, receive devinfo |
| `rd_disconnect()` | Close WS connection |
| `rd_get_state()` | Connection status, device info, message log |
| `rd_press_key(key)` | Simulate button press (down + up), reports key redraws |
| `rd_read_layout()` | Read layout.json for connected device UUID |

## Review
All packages build and vet cleanly. The only build noise (`ayatana-appindicator3-0.1`) is a pre-existing devcontainer environment issue with the systray library, unrelated to this feature.

### To enable the feature
Add to `~/.riverdeck/config.yml`:
```yaml
network:
  websocket_enabled: true
  websocket_port: 9000
ui:
  navigation_style: layout
```
Connect via `ws://localhost:9000/ws` -- receive `devinfo` then `setimage` frames per key.
Session resumption: `ws://localhost:9000/ws?uuid=<previous-uuid>`.
