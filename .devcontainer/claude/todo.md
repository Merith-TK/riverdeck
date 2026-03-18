# Editor Overhaul + Package Management Implementation

See design doc: `docs/editor-package-design.md`

## Steps

- [ ] 1. Write design doc to `docs/editor-package-design.md` ← DONE
- [ ] 2. Config dir restructure + auto-migration (`pkg/platform/configdir.go`, `cmd/riverdeck/config.go`, `cmd/riverdeck/app.go`)
- [ ] 3. `pkg/gitpkg/` — hybrid git backend (git.go, git_native.go, git_go.go)
- [ ] 4. `pkg/pkgmanager/` — install/remove/update/list + index + lock (5 files)
- [ ] 5. `riverdeck.config` module redesign (`pkg/scripting/modules/config.go`)
- [ ] 6. `riverdeck.*` module aliases (`pkg/scripting/runner.go`)
- [ ] 7. Custom Lua import searcher (`pkg/scripting/runner.go`)
- [ ] 8. New editorserver API endpoints for install/remove/update/daemon-toggle (`pkg/editorserver/handler_packages.go`)
- [ ] 9. Monaco editor fix (`resources/editor/editor.js`)
- [ ] 10. Package manager modal UI (`resources/editor/index.html`, `resources/editor/editor.js`)
- [ ] 11. Daemon explicit opt-in (`pkg/scripting/manager.go`)
- [ ] 12. Add go-git dependency (`go.mod`)
- [ ] 13. Build verification

## Review
