# Claude Working Notes

## Rules / Lessons Learned

### 2026-03-14 -- CLAUDE.md workflow
- **DO NOT** touch `.devcontainer/do-not-commit/` -- it is a symlink to `~/.claude` for devcontainer persistent auth
- Task tracking goes in `.devcontainer/claude/todo.md` (checkable items)
- Lessons / patterns go in `.devcontainer/claude/notes.md` (this file)
- Memory system (`memory/MEMORY.md`) is separate and is auto-loaded per conversation

### Environment
- `go build ./...` always emits `ayatana-appindicator3-0.1` pkg-config warning from `getlantern/systray` -- pre-existing devcontainer issue, not a real error
- Use `go build ./pkg/...` or filter output to isolate real errors from the systray noise

### MCP Server (cmd/mcp-wsdevice)
- Registered in `.mcp.json` at the **project root** (project-level, `go run` transport -- no pre-build needed)
- NOTE: `.claude/mcp.json` is WRONG -- project-level MCP lives at `.mcp.json` (root), not inside `.claude/`
- Uses `github.com/mark3labs/mcp-go` v0.45.0
- State is module-global -- persists across tool calls in the same server process
- WS readLoop runs in background goroutine; state protected by mutex
- `for k := range state.keys` uses Go 1.22+ integer range (fine on Go 1.24)

### Architecture Patterns
- All virtual devices (SimClient TCP, wsdevice WebSocket) implement `streamdeck.DeviceIface` -- fully transparent to the rest of the app
- `LayoutNavigator.configDir` is used for script path resolution -- always pass the **global** `configPath`, never a device-specific subdir, or scripts won't resolve
- Per-device layouts: `configDir/devices/{id}/layout.json` via `layout.DeviceLayoutDir()`
