# residual-overview — riverdeck

## orientation

project:   riverdeck
root:      /home/merith/workspace/github.com/Merith-TK/riverdeck
stack:     Go 1.24.4, gopher-lua (Lua 5.1), Wails v2 (GUI editor), Fyne (debug-prober only)
shared:    pkg/ — library packages; no binary, no main
binaries:  riverdeck, riverdeck-debug-prober, riverdeck-simulator, riverdeck-wails
config:    $HOME/.config/riverdeck/ (Linux/macOS); %APPDATA%\.riverdeck (Windows)
           config.yml (YAML), layout.json (JSON); waterfall: defaults -> file -> user edits
aesthetic: none stated
test cmd:  go test ./...
build cmd: mage Build  (or: go build ./...)

## derived rules

- pkg/ is library-only; binaries live in cmd/
- Config keys: snake_case YAML
- Lua scripts must return a table; passive(key, state) receives two args and returns a table;
  trigger(state) receives one arg; background(state) is a coroutine
- state table is set as a Lua global AND passed explicitly to passive/trigger/background
- Every installed package lives under <configDir>/.packages/<id>/
- The bundled riverdeck package is always overwritten from the embed on startup
- Layout navigation requires exactly one action:"home" button per page (enforced by layout.Validate)
- EditorServer callbacks (OnLayoutSaved, GetMode, OnModeChanged) are declared in Config but never
  populated by either binary; dead API surface in practice

## assumptions

- The Lua ctx API shown in starter templates (ctx.text(), ctx.color()) is NOT the actual runtime API;
  the real runner passes (key, state) and expects a return table with text/color/image fields.
  This inconsistency is treated as a VIOLATION.
- RegisterAllWithLogPrefix and OpenFirst are assumed to be intentional public API for external
  consumers, as documented in their godoc comments.
- fyne.io/fyne/v2 is a legitimate dependency for riverdeck-debug-prober only; correct usage.
- Config struct fields parsed from YAML but never read in application logic (ScriptingConfig,
  DeviceConfig, LoggingConfig, SecurityConfig, NetworkConfig, UIConfig.ShowHiddenFiles,
  UIConfig.Labels, PerformanceConfig.ImageCacheSize, PerformanceConfig.CompressImages) are
  forward-reserved stubs — noted as ISSUE but not assumed to be bugs.

## review scope

Full audit — all packages and binaries in the repo.

---

## violations

### [VIOLATION] Lua starter templates use a nonexistent ctx API

**Files:** `pkg/editorserver/handler_lua.go:29,49,70,90-94`, `pkg/editorserver/handler_custom.go:137`
**Category:** interface/implementation mismatch

All builtin Lua starter templates and the custom-template default script expose a `ctx` object API
(`ctx.text("...")`, `ctx.color(r,g,b)`). This API does not exist anywhere in the runtime.

The actual call convention (`pkg/scripting/runner.go:658-660`) is:
- `passive(key, state)` — called with two arguments, expects a **return table** with fields
  `text`, `color`, `image`, `text_color`
- The docstring at `runner.go:12-18` documents this contract

Any script written by a user starting from these templates will silently produce no output.
`parseAppearance` (`runner.go`) will receive `nil` from the function call and silently return
an empty appearance struct.

**Fix:** Rewrite all builtin template bodies to use the correct return-table convention:

```lua
-- correct passive template
function M.passive(key, state)
    return {
        text = tostring(state.count),
        color = {r=0, g=128, b=255},
    }
end
```

---

### [VIOLATION] Background worker template receives state incorrectly

**File:** `pkg/editorserver/handler_lua.go:46-55`
**Category:** interface/implementation mismatch

The "Background Worker" starter template defines `function M.background()` with no parameters,
then reads `state.count` as if `state` is a Lua global. The runner (`runner.go:532`) calls
`background(state)` passing state as the first argument.

The same template also defines `passive` that reads a local `state` variable set from the global —
inconsistent with how the runtime actually delivers state. The dual-access pattern
(`runner.go:179` sets `L.SetGlobal("state", r.state)` AND passes it as an arg) means the global
happens to work, but it is undocumented, fragile, and internally inconsistent with the template's
own `passive` function.

**Fix:** All template functions must declare their parameters explicitly:

```lua
function M.background(state)
    -- use state argument, not the global
    while true do
        state.count = (state.count or 0) + 1
        coroutine.yield(1.0)
    end
end
```

---

## issues

### [ISSUE] Eight config struct sections are parsed but never consumed

**File:** `cmd/riverdeck/config.go:36-71`
**Category:** dead config fields

The following fields are defined in the config struct, written to `config.yml` via `DefaultConfig`,
saved via `SaveConfig`, loaded via `LoadConfig`, but **never read by any application logic**:

| Section | Fields |
|---|---|
| `ScriptingConfig` | `EnableBackground`, `ExecutionTimeout`, `MaxConcurrentScripts` |
| `DeviceConfig` | `AutoDetect`, `Path`, `Model` |
| `LoggingConfig` | `Level`, `File`, `EnableConsole` |
| `SecurityConfig` | `AllowNetworkAccess`, `AllowFileSystemAccess`, `AllowProcessExecution` |
| `NetworkConfig` | `ProxyURL`, `Timeout`, `MaxRetries` |
| `UIConfig` | `ShowHiddenFiles`, `Labels` |
| `PerformanceConfig` | `ImageCacheSize`, `CompressImages` |

Users who edit these fields get no effect. This violates the principle of least surprise for
operators: a config key that does nothing is misleading.

**Fix:** Either wire each field to real behaviour, or remove the struct fields, YAML keys, and
`DefaultConfig` entries. Keeping stubs only makes sense if a ticket or comment links the stub to
planned work.

---

### [ISSUE] EditorServer callback fields are never populated

**File:** `pkg/editorserver/server.go:44-51`
**Category:** dead callbacks / silent no-ops

`editorserver.Config` declares three callback fields:
- `OnLayoutSaved func(layout []byte)`
- `GetMode func() string`
- `OnModeChanged func(mode string)`

The only caller, `cmd/riverdeck-wails/main.go`, constructs `editorserver.Config` without
populating any of these. As a result:
- `handleMode` (POST) silently no-ops because `OnModeChanged` is nil
- `handleLayout` silently skips `OnLayoutSaved`
- `handleMode` (GET) returns an empty string because `GetMode` is nil

**Fix:** Either remove the callback fields and make the server self-contained (store mode
internally, persist layout directly), or ensure the wails binary wires them and document the
integration contract.

---

### [ISSUE] Stale "future" comment on a fully-implemented field

**File:** `cmd/riverdeck/app.go:53`
**Category:** stale comment

```go
settingsPage int // future: scroll through setting rows
```

`settingsPage` is fully implemented throughout `settings.go`. The "future:" annotation is stale
and misleading.

**Fix:** Remove "future:" from the comment, or remove the comment entirely.

---

### [ISSUE] PrintDeviceInfo belongs in cmd/, not pkg/streamdeck

**File:** `pkg/streamdeck/enumerate.go:84-103`
**Category:** wrong layer / stdout bypass

`PrintDeviceInfo` writes device enumeration directly to stdout via `fmt.Println`/`fmt.Printf`.
The rest of the application uses `log.Printf` (which writes to stderr with timestamps). This
function is presentation-layer logic that has leaked into a library package.

The caller at `cmd/riverdeck/app.go:155` is inside device-selection logic that itself mixes
`fmt.Printf` and `log.Printf`.

**Fix:** Move the formatting into `cmd/riverdeck/app.go` (inline or a local helper). Remove
`PrintDeviceInfo` from `pkg/streamdeck`.

---

### [ISSUE] Coroutine first-resume sentinel pattern is opaque

**File:** `pkg/scripting/runner.go:530-560`
**Category:** confusing control flow

`runBackgroundCoroutine` uses `r.bgFunc` as a sentinel to distinguish the first resume from
subsequent resumes:
- When `r.bgFunc != nil`: first resume — pass `{bgFunc, state}` as args, then nil `r.bgFunc`
- When `r.bgFunc == nil`: subsequent resume — pass `{nil}` as args

The slice `resumeArgs` is built in both cases but the nil-resume case constructs
`[]lua.LValue{nil}` and then calls `r.L.Resume(r.bgThread, nil)` — the slice is never used.
The branching logic uses `len(resumeArgs) > 1` as the discriminant, which is only
accidentally correct.

**Fix:** Replace the sentinel pattern with an explicit boolean:

```go
type Runner struct {
    // ...
    bgStarted bool
}

// first resume
if !r.bgStarted {
    r.bgStarted = true
    r.L.Resume(r.bgThread, r.bgFunc, r.state)
} else {
    r.L.Resume(r.bgThread)
}
```

---

## smells

### [SMELL] RegisterAllWithLogPrefix is exported dead code

**File:** `pkg/lualib/register.go:29-46`

Documented as "currently unused internally" and retained "for external consumers." No known
external consumers exist. Duplicates `RegisterAll` with one extra parameter. If there are no
external consumers, remove it. If it is intended public API, remove the "currently unused"
comment (it undermines confidence in the function).

---

### [SMELL] OpenFirst is exported dead code

**File:** `pkg/streamdeck/device.go:101-113`

Documented as "currently unused by the application" and retained "as a convenience for external
consumers and testing." No tests use it. Exported dead code in a library package is a maintenance
liability — callers can't exist if there are none.

---

### [SMELL] pkg/resolver exports several functions with no internal callers

**File:** `pkg/resolver/resolve.go:157-183`

`ResolveString`, `IsLuaForbiddenString`, and `SchemeBadge` have no callers inside the repo.
`SchemeBadge` is covered by tests in `resolve_test.go` but unused in application code. These are
convenience wrappers that add API surface without reducing internal complexity. Acceptable if
`pkg/resolver` is meant to be a consumed library; remove if it is internal-only.

---

### [SMELL] PageByName has no callers

**File:** `pkg/layout/types.go:114-122`

`PageByName` is exported but has no callers in the repo. `PageIndexByName` (a functional
near-duplicate) is used throughout. `PageByName` is dead exported API. Remove it or consolidate
the two into a single function.

---

### [SMELL] Boot() uses fmt.Printf instead of log.Printf

**File:** `pkg/scripting/manager.go:138-160`

`Boot()` uses `fmt.Printf` throughout for package scan reporting, startup messages, and error
output. Every other package uses `log.Printf`. This means some startup output bypasses log
formatting (timestamp, level prefix). Standardise on `log.Printf`.

---

### [SMELL] Boot() is a 135-line method doing four unrelated things

**File:** `pkg/scripting/manager.go:209-265`

`Boot()` performs: package scanning, daemon startup, boot animation, and script scanning/loading.
Each phase has enough complexity to justify its own method. The current shape makes individual
phases untestable in isolation.

**Fix:** Extract `scanAndBootPackages`, `runBootAnimation`, and `loadScripts` as separate methods
called sequentially from `Boot`.

---

### [SMELL] Monaco editor has no offline fallback

**File:** `pkg/editorserver/monaco.go`

Monaco is fetched from a CDN and cached locally on first use. On first launch without internet
access, the editor silently fails with an HTTP 502. There is no user-facing message explaining
that an internet connection is required for the first launch.

**Fix:** Either bundle Monaco as an embed (larger binary, fully offline) or display a clear
user-facing error when the CDN fetch fails.

---

### [SMELL] cmd/riverdeck-wails/app.go is a comment-only file

**File:** `cmd/riverdeck-wails/app.go`

The file contains only a comment stating it is intentionally empty. A comment-only file with no
code and no build tags adds no information that the absence of the file would not convey.

**Fix:** Delete the file. If the Wails convention requires an `app.go`, add a minimal build
constraint comment explaining why it is empty, or add the App struct stub.

---

### [SMELL] Config directory is logged twice at startup

**File:** `cmd/riverdeck/app.go:117,212`

`log.Printf("[*] Config directory: %s", ...)` is called at two separate points during startup
with the same value. One of the two log lines should be removed.

---

### [SMELL] Stale Lanczos3 comment on bilinear resize

**File:** `pkg/streamdeck/device.go:406-408`

```go
// OPTIMIZATION: Use Lanczos3 resampling for better quality at similar speed
```

The function uses bilinear interpolation, not Lanczos3. This is a stale comment from a planned
change that was not implemented.

**Fix:** Remove the comment or implement Lanczos3 resampling.

---

## notes

### [NOTE] Only pkg/resolver has tests

14 of 15 packages have zero test files. `pkg/scripting`, `pkg/streamdeck`, `pkg/layout`, and
`pkg/editorserver` all have zero coverage. The resolver tests are thorough for their scope.

---

### [NOTE] state is accessible two ways in Lua scripts

`runner.go:179` sets `state` as a Lua global (`L.SetGlobal("state", r.state)`) AND passes it
explicitly as an argument to `passive`, `trigger`, and `background`. Scripts can access state
either way. The dual-access pattern is undocumented and could confuse script authors — the
starter templates use the global form for `background` but the docstring at `runner.go:12`
describes it as a passed parameter.

---

## verification log

- build: PASS — `go build ./...` (exit 0)
- tests: PASS — `go test ./...` (exit 0; only pkg/resolver has test files; 14 packages untested)
- audit date: 2026-06-26
