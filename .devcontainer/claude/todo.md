# Session Summary — 2026-03-16 (continued)

## Completed
- [x] Unified layout.json format: `{"layouts": {"default": {...}}, "devices": {"id": "name"}}`
- [x] `pkg/layout/types.go` — added `LayoutFile` struct
- [x] `pkg/layout/layout.go` — new API: `LoadFile`, `LoadForDevice`, `SaveLayout`; removed `Load`, `Save`, `DeviceLayoutDir`
- [x] `cmd/riverdeck/editor.go` — `createNavigator` uses `LoadForDevice(dir, dev.GetInfo().Serial)`
- [x] `cmd/riverdeck/wsdevice.go` — simplified: removed per-device dir seeding; just `LoadForDevice`
- [x] `pkg/editorserver/server.go` — uses `LoadForDevice`; added `layoutName` field
- [x] `pkg/editorserver/handler_layout.go` — uses `SaveLayout(configDir, layoutName, lay)`
- [x] `cmd/mcp-wsdevice/tools.go` — `rd_read_layout` fixed: uses `LoadForDevice(configDir, uuid)` directly
- [x] `.riverdeck/layout.json` — migrated to new format
- [x] `go build ./pkg/... ./cmd/...` clean

## Outstanding / Next Session

### Not yet implemented
- Script execution for WS clients (`runWSDevice` logs path but doesn't run scripts)
- Hardware device layout migration to per-device dir was dropped in favour of the new unified format

### Testing
- Run: `go run ./cmd/riverdeck/ -configdir .riverdeck`
- Connect via MCP: `rd_connect(port=9000, config_dir="/workspace/github.com/Merith-TK/riverdeck/.riverdeck")`
- `rd_read_layout` should now return the layout from the unified layout.json
