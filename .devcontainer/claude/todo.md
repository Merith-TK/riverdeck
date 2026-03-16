# Session — WS Protocol Redesign

## Completed
- [x] 1. `pkg/streamdeck/iface.go` — add `SetLabel`, change `EncodeKeyImage(keyIndex, img)`
- [x] 2. `pkg/streamdeck/device.go` — stub `SetLabel`, update `EncodeKeyImage` sig
- [x] 3. `pkg/streamdeck/simclient.go` — same stubs
- [x] 4. `pkg/streamdeck/layout_navigator.go` — pass keyIndex to `EncodeKeyImage`; send `SetLabel` after render
- [x] 5. `pkg/streamdeck/navigation.go` — pass keyIndex to `EncodeKeyImage` (folder navigator)
- [x] 6. `cmd/riverdeck/gif.go` — pass keyIndex to `EncodeKeyImage`
- [x] 7. `pkg/wsdevice/device.go` — full rewrite: hello-driven, format negotiation, label, ping/pong
- [x] 8. `pkg/wsdevice/server.go` — full rewrite: multi-device, hello/ack, duplicate rejection, keepalive
- [x] 9. `cmd/riverdeck/app.go` — remove hardcoded wsModel from NewServer call
- [x] 10. `cmd/riverdeck/wsdevice.go` — UUID() → ID()
- [x] 11. `cmd/mcp-wsdevice/state.go` — persistent device ID, hello/ack/frame/label handling
- [x] 12. `cmd/mcp-wsdevice/tools.go` — send hello, `input` events, labels in state, rd_list_inputs
- [x] 13. `cmd/mcp-wsdevice/main.go` — add rd_list_inputs tool registration
- [x] 14. `mage build` — clean

## Outstanding
- [ ] End-to-end test via MCP tools (requires Claude Code restart to reload mcp-wsdevice)
