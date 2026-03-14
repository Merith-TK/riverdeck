# Claude Working Notes

## Rules / Lessons Learned

### 2026-03-14 — CLAUDE.md workflow
- **DO NOT** touch `.devcontainer/do-not-commit/` — it is a symlink to `~/.claude` for devcontainer persistent auth
- Task tracking goes in `.devcontainer/claude/todo.md` (checkable items)
- Lessons / patterns go in `.devcontainer/claude/notes.md` (this file)
- Memory system (`memory/MEMORY.md`) is separate and is auto-loaded per conversation

### Environment
- `go build ./...` always emits `ayatana-appindicator3-0.1` pkg-config warning from `getlantern/systray` — pre-existing devcontainer issue, not a real error
- Use `go build ./pkg/...` or filter output to isolate real errors from the systray noise

### Architecture Patterns
- All virtual devices (SimClient TCP, wsdevice WebSocket) implement `streamdeck.DeviceIface` — fully transparent to the rest of the app
- `LayoutNavigator.configDir` is used for script path resolution — always pass the **global** `configPath`, never a device-specific subdir, or scripts won't resolve
- Per-device layouts: `configDir/devices/{id}/layout.json` via `layout.DeviceLayoutDir()`
