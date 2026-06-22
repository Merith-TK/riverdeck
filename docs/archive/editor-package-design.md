# Design: Editor Overhaul + Package Management System

> Status: DECISIONS FINALIZED

---

## All Core Questions Resolved

| Topic | Decision |
|-------|----------|
| Wails | **Keep** — overhaul editor codebase, not replace |
| Manifest filename | `riverdeck.pkg.manifest.json` everywhere; `"packages"` array = registry |
| Import separator | **Dots** — `merith-tk.riverdeck-packages.ytmd.lib.api` |
| Config dir | **Restructure now** with auto-migration |
| Module naming | `require("riverdeck.config")` with subfields: `cfg.script.defaultdata`, `cfg.script.sync()` |
| Import resolver | **Install-index** — `.config/packages/.index.json` |
| Git backend | **Hybrid** — go-git fallback, prefers system `git`; config option to force |
| Daemon config | **Central** `.config/packages.cfg.json`; per-package daemon toggles |
| Update timing | **Startup + manual button** |
| Single-pkg repos | No `packages` array = whole repo is the package |
| Editor package UI | **Separate modal** in Wails editor header |

---

## 1. Config Directory Restructure

### New Structure
```
~/.riverdeck/               (platform equivalent)
  .config.yml               ← auto-migrated from old config.yml
  .config/
    packages/               ← was .packages/
      .index.json           ← import resolver index
      packages.cfg.json     ← daemon enable/disable + version pins
      riverdeck.lock        ← checksum lock for all installed files
      # Hand-dropped packages:
      mypkg/
        riverdeck.pkg.manifest.json
      # Git-installed packages:
      github.com/
        merith-tk/
          riverdeck-packages/
            riverdeck.pkg.manifest.json
          riverdeck-packages@dev/
    devices/                ← was devices/
      {serial}/
        layout.json
    layouts/
      default.json
  # folder-mode scripts (unchanged, at root)
  myscript.lua
  _boot.lua
```

### Auto-Migration (on first new-version boot)
- Detect old `config.yml` → copy to `.config.yml`, remove old
- Detect old `.packages/` → move to `.config/packages/`, remove old
- Detect old `devices/` → move to `.config/devices/`, remove old
- Write migration notice to log

---

## 2. Package Manifest Format (`riverdeck.pkg.manifest.json`)

### Single-Package Repo (no `"packages"` array)
```json
{
  "id": "simple-clock",
  "name": "Simple Clock",
  "version": "1.0.0",
  "description": "A digital clock button",
  "source": "github.com/user/simple-clock",
  "provides": {
    "templates": [
      {
        "id": "clock",
        "label": "Clock",
        "script": "templates/clock.lua",
        "description": "Shows current time"
      }
    ]
  },
  "daemon": "daemon.lua"
}
```

### Multi-Package Repo (has `"packages"` array at repo root)
```json
{
  "riverdeck_registry_version": 1,
  "source": "github.com/merith-tk/riverdeck-packages",
  "packages": [
    {
      "id": "ytmd",
      "path": "ytmd",
      "name": "YouTube Music Desktop",
      "version": "1.2.0",
      "description": "Control YTMD from Stream Deck"
    },
    {
      "id": "obs",
      "path": "obs",
      "name": "OBS Control",
      "version": "0.5.0"
    }
  ]
}
```

Each sub-package has its own `riverdeck.pkg.manifest.json` at its `path/`.

---

## 3. Package Config File (`.config/packages/packages.cfg.json`)

```json
{
  "github.com/user/simple-clock": {
    "daemon_enabled": true,
    "update_channel": "release",
    "pinned_tag": "v1.0.0"
  },
  "github.com/merith-tk/riverdeck-packages": {
    "update_channel": "release",
    "pinned_tag": "v2.0.0",
    "packages": {
      "ytmd": {
        "daemon_enabled": true
      },
      "obs": {
        "daemon_enabled": false
      }
    }
  },
  "mypkg": {
    "daemon_enabled": false
  }
}
```

Fields:
- `update_channel`: `"release"` (use git tags) or `"branch:main"` (dev mode)
- `pinned_tag`: specific tag to use; overridden by `update_channel: "branch:*"`
- `daemon_enabled`: bool (top-level for single-package, per-entry under `packages` for multi)

---

## 4. Git Package (`pkg/gitpkg/`)

Three-file structure:
```
pkg/gitpkg/
  git.go          ← public API, Init() detection, dispatch
  git_native.go   ← exec("git") implementation
  git_go.go       ← go-git implementation
```

### Public API
```go
package gitpkg

type Backend int
const (
    BackendAuto   Backend = iota
    BackendNative
    BackendGoGit
)

var ActiveBackend Backend

func Init(configuredBackend string) // "auto", "native", "go-git"

func Clone(url, targetDir, refName string, depth int) error
func Fetch(repoDir string) error
func Checkout(repoDir, refName string) error
func ListTags(url string) ([]string, error)
func Pull(repoDir string) error
```

### Config option (in `.config.yml`)
```yaml
application:
  git_backend: "auto"   # "auto" | "native" | "go-git"
```

---

## 5. Package Manager (`pkg/pkgmanager/`)

```
pkg/pkgmanager/
  manager.go      ← Manager struct, Install/Remove/Update/List
  source.go       ← ParseSource(), URL format parsing
  index.go        ← .index.json read/write
  lock.go         ← riverdeck.lock read/write/verify
  config.go       ← packages.cfg.json read/write
```

### Install URL Formats
```
github.com/user/repo                    single-pkg, main branch
github.com/user/repo@branch             single-pkg, specific branch
github.com/user/repo@v1.2.0             single-pkg, tag (version pin)
github.com/user/repo/ytmd               specific package from multi-pkg repo
github.com/user/repo@v2.0.0/ytmd        specific package + version pin
gitlab.com/user/repo                    GitLab (same format)
git.hostname.tld/user/repo              custom git host (git clone only)
```

### Install Algorithm
1. Parse URL → `PackageSource{Host, User, Repo, Branch, Tag, PkgPath}`
2. Determine target dir in `.config/packages/`
3. `gitpkg.Clone(url, tmpdir, ref, 1)` (depth=1 for tags; full for branches)
4. Read `riverdeck.pkg.manifest.json` at repo root
5. If `packages` array: show list → user selects which to install
6. If no `packages` array: whole repo = one package
7. Copy selected package(s) to final target dirs
8. Update `.index.json` (shorthand → path mapping)
9. Update `riverdeck.lock` (SHA256 of all installed files)
10. Update `packages.cfg.json` (daemon_enabled=false by default)
11. Show pre-install review to user (list of Lua files)

### Pre-Install Review
Before step 7, display all `.lua` files to be installed:
```
Package: youtube-music-desktop v1.2.0
  Daemon:     daemon.lua
  Templates:  templates/play_pause.lua, templates/volume.lua
  Libraries:  lib/ytmd_client.lua

[Review files] [Cancel] [Install]
```

---

## 6. Import Index (`.config/packages/.index.json`)

```json
{
  "merith-tk.riverdeck-packages": {
    "path": "github.com/merith-tk/riverdeck-packages",
    "packages": ["ytmd", "obs"]
  },
  "user.simple-clock": {
    "path": "github.com/user/simple-clock"
  },
  "mypkg": {
    "path": "mypkg"
  }
}
```

### Resolution algorithm for `require("merith-tk.riverdeck-packages.ytmd.lib.api")`
1. Split at dots, check for `@branch` suffix on second segment
2. Try `merith-tk.riverdeck-packages` → look up in index → `github.com/merith-tk/riverdeck-packages`
3. Next segment `ytmd` → is in `packages` list → sub-package
4. Remaining `lib.api` → `lib/api.lua`
5. Full path: `.config/packages/github.com/merith-tk/riverdeck-packages/ytmd/lib/api.lua`

---

## 7. Lua Runtime Changes (`pkg/scripting/`)

### New custom Lua searcher (in `runner.go`)
Register at index 5 in `package.searchers` (after standard Lua loaders):
- Checks if module name matches known import patterns
- Uses index lookup for shorthand paths
- Falls back to standard Lua resolution for unrecognized names

### `riverdeck.*` module aliases
Old name → new `riverdeck.X` alias (both work, old deprecated):
```
config    → riverdeck.config
store     → riverdeck.store
streamdeck→ riverdeck.streamdeck
http      → riverdeck.http
shell     → riverdeck.shell
file      → riverdeck.file
system    → riverdeck.system
pkg_data  → riverdeck.pkg_data
```

### `riverdeck.config` API redesign
```lua
local cfg = require("riverdeck.config")

cfg.script.defaultdata = {
    volume = 5,
    foo = "bar",
}

cfg.script.sync()

local vol = cfg.script.data.volume

cfg.script.data.volume = 10
cfg.script.save()
```

---

## 8. Editor Overhaul

### Monaco Lua Editor — Fix Activation
Current bug: Monaco never activates. Fix in `resources/editor/editor.js`:
- Ensure `require(['vs/editor/editor.main'], callback)` fires when the Lua editor tab is shown
- Attach to the `switchConfigTab('monaco')` event

### Package Manager Modal
New button in Wails editor header: `[Packages]` → opens modal.

Modal sections:
1. **Install** — URL input + "Browse" (fetches registry, shows package list)
2. **Installed list** — one row per installed package/sub-package
   - Shows: name, version, daemon toggle, update badge
   - Multi-package repos: expandable to show individual sub-packages
3. **Update** — "Check for updates" button + per-package update action

### New Script UX
- "New Script" button in button config panel creates a `.lua` stub
- Auto-opens in Monaco editor tab

### Package daemon toggle
In the modal, each package shows a daemon toggle switch.
Writes to `packages.cfg.json` immediately.

---

## 9. Implementation Order

1. **Config dir restructure + migration** (`pkg/platform/`, `cmd/riverdeck/`)
2. **`pkg/gitpkg/`** — hybrid git backend
3. **`pkg/pkgmanager/`** — install/remove/update/list + index + lock
4. **`riverdeck.config` module redesign** (`pkg/scripting/modules/config.go`)
5. **`riverdeck.*` module aliases** (`pkg/scripting/runner.go`)
6. **Custom Lua import searcher** (`pkg/scripting/runner.go`)
7. **New editorserver API endpoints** (install, remove, update, daemon-toggle)
8. **Monaco editor fix** (`resources/editor/editor.js`)
9. **Package manager modal UI** (`resources/editor/`, `cmd/riverdeck-wails/`)
10. **Daemon explicit opt-in** (change `ScriptManager.Boot()` to check `packages.cfg.json`)

---

## 10. Files to Create/Modify

| File | Action |
|------|--------|
| `pkg/gitpkg/git.go` + `git_native.go` + `git_go.go` | CREATE |
| `pkg/pkgmanager/manager.go` + `source.go` + `index.go` + `lock.go` + `config.go` | CREATE |
| `pkg/scripting/modules/config.go` | MODIFY (major redesign) |
| `pkg/scripting/runner.go` | MODIFY (aliases + custom searcher) |
| `pkg/scripting/manager.go` | MODIFY (daemon opt-in check) |
| `pkg/editorserver/handler_packages.go` | MODIFY (new endpoints) |
| `pkg/platform/configdir.go` | MODIFY (new paths) |
| `cmd/riverdeck/app.go` | MODIFY (migration on boot) |
| `cmd/riverdeck/config.go` | MODIFY (git_backend option) |
| `resources/editor/editor.js` | MODIFY (Monaco fix, pkg modal) |
| `resources/editor/index.html` | MODIFY (pkg modal HTML) |
| `go.mod` | MODIFY (add go-git) |
