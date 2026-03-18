# Editor Overhaul + Package Management Implementation

See design doc: `docs/editor-package-design.md`

## Steps

- [x] 1. Write design doc to `docs/editor-package-design.md`
- [x] 2. Config dir restructure + auto-migration (`pkg/platform/configdir.go`, `cmd/riverdeck/config.go`, `cmd/riverdeck/app.go`)
- [x] 3. `pkg/gitpkg/` — hybrid git backend (git.go, git_native.go, git_go.go)
- [x] 4. `pkg/pkgmanager/` — install/remove/update/list + index + lock (5 files)
- [x] 5. `riverdeck.config` module redesign (`pkg/scripting/modules/config.go`)
- [x] 6. `riverdeck.*` module aliases (`pkg/scripting/runner.go`)
- [x] 7. Custom Lua import searcher (`pkg/scripting/runner.go`)
- [x] 8. New editorserver API endpoints for install/remove/update/daemon-toggle (`pkg/editorserver/handler_pkgmgr.go`)
- [x] 9. Monaco editor fix (`resources/editor/editor.js`)
- [x] 10. Package manager modal UI (`resources/editor/index.html`, `resources/editor/editor.js`, `resources/editor/style.css`)
- [x] 11. Daemon explicit opt-in (`pkg/scripting/manager.go`)
- [ ] 12. Add go-git dependency (`go.mod`) — DEFERRED (stub in git_go.go; native git preferred)
- [x] 13. Build verification — PASSES (`go build ./...`)

## Review

All 10 design sections implemented:
- Config dir restructured: `.packages/` → `.config/packages/`, `.devices/` → `.config/devices/`, `config.yml` → `.config.yml`
- Auto-migration runs on first boot after upgrade
- `pkg/gitpkg/` hybrid backend (native preferred, go-git stub)
- `pkg/pkgmanager/` full install/remove/update/list + `.index.json` + `riverdeck.lock` + `packages.cfg.json`
- `riverdeck.config` redesigned with `cfg.script.defaultdata/sync()/data/save()` API
- `riverdeck.*` aliases for all built-in modules
- Custom Lua searcher for dot-notation package imports
- `/api/pkg/{install,remove,update,list,daemon}` endpoints
- Monaco layout fix: `monacoEditor.layout()` on tab switch
- Package manager modal with install/remove/update/daemon-toggle
- Daemon opt-in: only starts when `daemon_enabled=true` in `packages.cfg.json`
