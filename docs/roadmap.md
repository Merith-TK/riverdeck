# Riverdeck Roadmap

Last updated: 2026-05-25

This document is a living overview of the entire codebase — what exists, what works, what is incomplete, and what is planned. It is organized by area so individual contributors can orient quickly.

---

## Table of Contents

1. [Project Overview](#1-project-overview)
2. [Current Architecture](#2-current-architecture)
3. [Feature Status](#3-feature-status)
4. [Known Gaps and Issues](#4-known-gaps-and-issues)
5. [Hardware Support](#5-hardware-support)
6. [Scripting System](#6-scripting-system)
7. [Package Manager](#7-package-manager)
8. [Editor and UI](#8-editor-and-ui)
9. [WebSocket Device Server](#9-websocket-device-server)
10. [Testing](#10-testing)
11. [Planned Work (Prioritized)](#11-planned-work-prioritized)

---

## 1. Project Overview

Riverdeck is a Stream Deck controller daemon written in Go. It turns a physical (or virtual) Stream Deck into a scriptable macro pad driven by Lua scripts. Key design goals:

- **No cloud required** — fully local, no Elgato software needed
- **Lua-first scripting** — every button is a `.lua` file; passive loops, trigger handlers, and background daemons
- **Two navigation modes** — filesystem (browse a directory tree on-device) and layout (declarative `layout.json` pages)
- **Extensible packages** — community packages installed from git repositories, each with Lua libraries and assets
- **Virtual device support** — WebSocket protocol lets software clients pose as Stream Decks (used for the MCP bridge, simulator, and future Android/web clients)

---

## 2. Current Architecture

### Packages (`pkg/`)

| Package | Role |
|---|---|
| `pkg/appearance` | `ApplyKeyAppearance` — composites background color + icon + text onto a device key |
| `pkg/editorserver` | Embedded HTTP REST server for the web/Wails editor (experimental) |
| `pkg/gitpkg` | Hybrid Git backend — dispatches to system `git` binary or `go-git` fallback |
| `pkg/imaging` | Image loading, scaling, GIF frame extraction |
| `pkg/layout` | `layout.json` data model, load/save, device geometry cache |
| `pkg/lualib` | Pure-Go Lua stdlib extensions (`strings`, `json`, `time`, `log`, `base64`, `crypto`, `regex`, `path`, `url`, `utils`, `mathx`) |
| `pkg/pkgmanager` | Package install/remove/update, `.index.json`, `riverdeck.lock` |
| `pkg/platform` | Config directory resolution, browser launch |
| `pkg/prober` | HID device enumeration and feature-report capture (dev tooling) |
| `pkg/resolver` | `pkg://` URI resolver for icon/asset references |
| `pkg/scripting` | Lua script lifecycle engine (runner, manager, passive loop, background workers) |
| `pkg/scripting/modules` | Domain Lua modules: `http`, `shell`, `file`, `store`, `config`, `pkg_data`, `streamdeck`, `system` |
| `pkg/streamdeck` | HID device driver, model registry, `Navigator` (filesystem), `LayoutNavigator` |
| `pkg/util` | File I/O helpers, `SanitizeFilename`, `ExtractFS` |
| `pkg/wsclient` | WebSocket client helpers: hello message builder, stable device ID, input ID mapping |
| `pkg/wsdevice` | WebSocket virtual Stream Deck server — accepts software clients as hardware peers |

### Commands (`cmd/`)

| Command | Role |
|---|---|
| `cmd/riverdeck` | Main daemon: HID device sessions, scripting, system tray, config, WS server |
| `cmd/riverdeck-wails` | Wails v2 desktop editor (experimental GUI) |
| `cmd/riverdeck-simulator` | Software device simulator (HTTP + WebSocket) for hardware-free development |
| `cmd/riverdeck-debug-prober` | HID packet capture tool for hardware research |
| `cmd/wsdevice` | Reference WebSocket client implementations (web, Android stubs) |
| `cmd/mcp-wsdevice` | MCP (Model Context Protocol) bridge — exposes the virtual deck as LLM tools |

---

## 3. Feature Status

### Working

- **Filesystem navigation mode** — browses `.riverdeck/` as a directory tree; `.directory.lua` passive/background scripts run per-folder; T1/T2 keys wired to directory script hooks
- **Layout navigation mode** — declarative `layout.json` pages; page stacks, back navigation, slot-based button placement
- **Auto navigation mode** — selects layout if `layout.json` exists, falls back to filesystem
- **Lua scripting engine** — `background()`, `passive()`, `trigger()`, `t1_*`/`t2_*` hooks; passive FPS-throttled loop; background coroutines with `system.sleep()`
- **`riverdeck.*` module aliases** — all built-in Lua modules accessible under both bare names and `riverdeck.` namespace
- **Package `lib/` loading** — installed package `lib/` directories added to `package.path`; `require('ytm')` style imports work
- **Custom Lua package searcher** — dot-notation imports (`require('owner.repo.subpkg.lib')`) resolved via `packages.json` index
- **`pkg://` URI scheme** — icon and asset references resolved to package-local paths; used in `layout.json` and Lua `passive()` returns
- **WebSocket device server** — starts in any `navigation_style`; creates `Navigator` or `LayoutNavigator` matching config
- **Multi-device support** — `shared` / `individual` / `layout` multi-mode; per-device session config dirs
- **MCP bridge** (`cmd/mcp-wsdevice`) — 9 tools: `rd_connect`, `rd_disconnect`, `rd_get_state`, `rd_get_frame`, `rd_press_key`, `rd_read_layout`, `rd_read_config`, `rd_list_devices`, `rd_set_brightness`
- **Device geometry cache** — WS clients save grid shape to `.config/devices/<id>/geometry.json` for editor use
- **Config hot-reload** — restart-on-config-change via system tray; individual `RestartPolicy` per script
- **JPEG compression** for key images; configurable quality
- **GIF animations** on keys (pre-encoded per-frame)
- **Display sleep / timeout** — idle timeout dims the deck; any key press wakes it
- **Emergency kill combo** — simultaneous corner-key hold exits cleanly
- **System tray** integration (Linux/macOS)

### Experimental / Incomplete

- **Layout editor** — `pkg/editorserver` REST API exists and is wired; Wails GUI (`cmd/riverdeck-wails`) compiles but is not feature-complete; web editor (`resources/editor/`) loads but Monaco tab has a known activation bug
- **Package manager UI** — backend (`pkg/pkgmanager`) is fully implemented; no polished user-facing install flow yet
- **`security.allowed_commands` / `security.block_network`** — declared in config but not enforced in `shell.go` or `http.go`
- **Config hot-reload debounce** — restart is immediate; debounce on rapid config saves not yet implemented
- **Dial / touch support** — `held`, `valueInc`, `valueDec`, `value` WS events mapped to `pressed=true` as a stub; actual dial scripting API not defined

---

## 4. Known Gaps and Issues

### Security

- `security.allowed_commands` config field is parsed but never checked in `pkg/scripting/modules/shell.go` — any `shell.exec()` call succeeds regardless
- `security.block_network` is parsed but never checked in `pkg/scripting/modules/http.go`
- `pkg/scripting/modules/file.go` has a `restrict_file_access` check, but the other two modules do not

**Priority: High** — these are silent no-ops that give users a false sense of sandboxing.

### Editor

- Monaco editor never activates — `require(['vs/editor/editor.main'], callback)` does not fire when the Lua tab is shown (`resources/editor/editor.js`); identified bug, not yet fixed
- No Lua syntax validation or linting in the editor
- `cmd/wsdevice/web/` and `cmd/wsdevice/android/` directories exist but are empty

### Config

- No hot-reload debounce — saving a file rapidly triggers multiple restarts
- Device-level config merges only a subset of keys (brightness, passive_fps, timeout, navigation_style, show_hidden_files, labels); adding new config fields requires manual merge additions in `LoadDeviceConfig`

### Scripting

- `ytm` package require path: scripts using `require('ytm')` rely on `package.path` injection from the installed `riverdeck.ytmdesktop` package's `lib/` directory — if the package is not installed, the require silently fails at runtime with no user-visible error
- No structured error reporting from Lua scripts to the editor or system tray
- Background daemon scripts have no resource limits (CPU, memory)

### Package Manager

- No package registry or browsing UI — installs require knowing a git URL
- No ZIP/tarball package import
- `packages.cfg.json` daemon opt-in has no UI toggle outside the editor

---

## 5. Hardware Support

See `docs/hardware-implementation-plan.md` for full details. Summary:

| Device | Status |
|---|---|
| Stream Deck Mini (6-key) | Supported |
| Stream Deck MK.1 / MK.2 (15-key) | Supported |
| Stream Deck XL (32-key) | Supported |
| Stream Deck Pedal | Partial — `PixelSize==0` guard exists; confirm dump needed |
| Stream Deck + (4 dials + 8 keys + LCD strip) | Partial — key HID works; dials and LCD strip not implemented |
| Stream Deck Neo (8 keys + touch strip) | Partial — keys work; touch strip not implemented |
| Stream Deck Module (rack units) | Not supported — PID unknown, needs hardware dump |
| Stream Deck Studio | Not supported — PID unknown, side-dial layout field not defined |
| Stream Deck + XL | Do not add — geometry unverified (4×9 key count mismatch) |

### Dial / Touch API (Stream Deck + and Neo)

The following `DeviceIface` methods are needed but not yet defined or implemented:

- `ListenDials(ctx, chan DialEvent)` — emit `valueInc`/`valueDec`/`press`/`release` per dial
- `ListenTouch(ctx, chan TouchEvent)` — emit swipe/tap events from Neo touch strip
- `SetStatusBarImage(img)` — write to Neo status bar display
- `SetLCDStripImage(segment, img)` — write to Stream Deck + LCD strip

Lua API hooks for dials and touch are not yet designed.

---

## 6. Scripting System

### Current Lua API Surface

**Lifecycle hooks** (returned by each script file):

```lua
script.background(state)      -- long-running coroutine; use system.sleep(ms)
script.passive(key, state)    -- called at passive_fps; return KeyAppearance table
script.trigger(state)         -- called on key press
script.t1_passive/t1_trigger  -- T1 reserved-key variants (driven by .directory.lua)
script.t2_passive/t2_trigger  -- T2 reserved-key variants
```

**KeyAppearance return fields:**

```lua
{ color={r,g,b}, icon="pkg://name#icon", text="label", text_color={r,g,b} }
```

**Built-in modules:**

| Module | Key functions |
|---|---|
| `system` | `sleep(ms)`, `refresh()`, OS detection, env vars |
| `http` | `get(url)`, `post(url, body)`, custom requests |
| `shell` | `exec(cmd)`, `exec_async(cmd)`, `open(path)`, `terminal(cmd)` |
| `file` | `read(path)`, `write(path, data)`, `list(path)` |
| `store` | `set(k,v)`, `get(k)`, `has(k)`, `delete(k)` — cross-script shared map |
| `config` | `get(key)`, `all()`, `schema()` |
| `pkg_data` | `read/write/json_read/json_write` scoped to package `data/` dir |
| `streamdeck` | Direct HW control (brightness, key color, layout ops) |

**Lua stdlib extensions** (`require()`-able): `strings`, `json`, `time`, `log`, `mathx`, `base64`, `crypto`, `regex`, `path`, `url`, `utils`

### Gaps

- No `dial` or `touch` Lua hooks
- No structured error surface — script failures go to stderr/log only
- No per-script resource limits
- `store` module is in-memory only — no persistence across restarts (use `pkg_data` for persistence)
- No `require()` from within a package's own `lib/` relative path (only bare name and dot-notation work)

---

## 7. Package Manager

### Implemented

- Install from git URL: clone, resolve sub-packages, write `.index.json` and `riverdeck.lock`
- Remove: delete directory, update index and lock
- Update: pull latest, re-resolve
- `lib/` path injection into Lua `package.path`
- Custom Lua searcher for dot-notation imports via `packages.json`
- `pkg://` URI resolution for icons/assets
- Daemon opt-in via `packages.cfg.json`

### Not Implemented

- Package registry / discovery — no browsing, no `riverdeck pkg search`
- ZIP/tarball import
- Version pinning UI (lock file exists but no user workflow to pin/unpin)
- Automatic dependency resolution between packages
- Package update notifications

---

## 8. Editor and UI

### Web Editor (`resources/editor/`)

- Loads in any browser; served by `pkg/editorserver`
- Layout editor: drag/drop buttons, page management
- Config panel: edit `.config.yml` in-browser
- **Bug:** Monaco (Lua editor tab) never activates — `require()` callback does not fire

### Wails Desktop Editor (`cmd/riverdeck-wails`)

- Wails v2 app shell exists and compiles
- Shares backend with `pkg/editorserver`
- Feature parity with web editor — neither is complete
- Marked experimental in README

### System Tray

- Working on Linux/macOS
- Actions: open editor, reload config, quit
- No per-device status or script error notifications

### Planned (from `docs/editor-v2-design.md`)

- Drag-and-drop button templates
- Dual-table config (global + per-device overlay)
- In-editor Lua file creation from templates
- Phase migration toward Fyne-native editor (4-phase plan)
- Open question: Lua syntax highlighting in Fyne (`widget.Entry` has no highlighting — needs canvas widget or external `$EDITOR` launch)

---

## 9. WebSocket Device Server

### Protocol

Bidirectional JSON over WebSocket. Clients send a `hello` frame then `input` (key/dial) events. Server pushes `frame` (image), `label` (text), `setbrightness`, `clear`, `reset`, `layoutChange`.

Full spec in `pkg/wsdevice/device.go` and `pkg/wsclient/hello.go`.

### Current State

- Server starts whenever `websocket_enabled: true` in config, regardless of `navigation_style`
- WS sessions run the same navigator (filesystem or layout) as hardware sessions
- Session resume by UUID: reconnecting with the same ID resumes the last session
- Device geometry saved to `.config/devices/<id>/geometry.json` on connect
- Keepalive: server pings every 10s, disconnects if no pong within 15s

### Stub / Incomplete

- Dial events (`valueInc`, `valueDec`, `held`) mapped to `pressed=true` — no real dial support
- `cmd/wsdevice/web/` — placeholder directory, no web client
- `cmd/wsdevice/android/` — placeholder directory, no Android client

### MCP Bridge (`cmd/mcp-wsdevice`)

9 tools exposed to LLMs via MCP protocol:

| Tool | Description |
|---|---|
| `rd_connect` | Connect to WS server, send hello, await ack |
| `rd_disconnect` | Close connection |
| `rd_get_state` | Connection status, labels, frame timestamps, message log |
| `rd_get_frame` | Raw base64 PNG for a specific key |
| `rd_press_key` | Simulate key press + release |
| `rd_read_layout` | Read `layout.json` without a live connection |
| `rd_read_config` | Read `.config.yml` |
| `rd_list_devices` | List known device geometry files |
| `rd_set_brightness` | Set device brightness |

---

## 10. Testing

### Current Coverage

| Package | Tests |
|---|---|
| `pkg/layout` | `layout_test.go` — save/load, validation, old-format promotion |
| `pkg/pkgmanager` | `index_test.go`, `lock_test.go`, `source_test.go` — 24 tests |
| `pkg/resolver` | `resolve_test.go` |
| `pkg/streamdeck` | `mock_device_test.go` — 11 tests; `MockDevice` implementing `DeviceIface` |

### No Tests

`pkg/appearance`, `pkg/editorserver`, `pkg/gitpkg`, `pkg/imaging`, `pkg/lualib`, `pkg/platform`, `pkg/prober`, `pkg/scripting` (entire engine + all 8 modules), `pkg/util`, `pkg/wsclient`, `pkg/wsdevice`, all `cmd/` binaries.

### Goals

- Cover `pkg/scripting` — at minimum, runner lifecycle, module registration, passive/trigger/background dispatch
- Cover `pkg/lualib` — pure functions, no I/O, easy to unit test
- Cover `pkg/wsdevice` — protocol framing, hello/ack, frame push
- Integration test: `cmd/riverdeck-simulator` + `cmd/mcp-wsdevice` as end-to-end harness
- Target: >60% statement coverage across `pkg/`

---

## 11. Planned Work (Prioritized)

### P0 — Security Fixes

- [ ] Enforce `security.allowed_commands` in `pkg/scripting/modules/shell.go`
- [ ] Enforce `security.block_network` in `pkg/scripting/modules/http.go`

### P1 — Core Reliability

- [ ] Fix Monaco editor activation bug in `resources/editor/editor.js`
- [ ] Add structured Lua error reporting to system tray and editor
- [ ] Config save debounce to prevent rapid restart loops
- [ ] Extend `LoadDeviceConfig` to auto-merge new fields without manual additions

### P2 — Hardware Expansion

- [ ] Define `ListenDials` / `ListenTouch` on `DeviceIface`
- [ ] Implement dial events for Stream Deck + (PID `0x0084`)
- [ ] Implement touch strip for Stream Deck Neo (PID `0x009a`)
- [ ] Design Lua hooks for `dial_change(index, delta, state)` and `touch(event, state)`
- [ ] Collect hardware dumps for Stream Deck Module, Studio, and verify XL geometry
- [ ] Implement `SetLCDStripImage` for Stream Deck + LCD bar
- [ ] Implement `SetStatusBarImage` for Stream Deck Neo status bar

### P3 — Package Ecosystem

- [ ] Package registry / discovery endpoint
- [ ] `riverdeck pkg search <query>` CLI command
- [ ] ZIP/tarball package import
- [ ] Version pinning workflow (UI + CLI)
- [ ] Automatic package dependency resolution

### P4 — Editor Polish

- [ ] Complete Wails desktop editor to feature parity with web editor
- [ ] Lua syntax highlighting (canvas widget or external editor integration)
- [ ] In-editor script template creation
- [ ] Per-device config overlay panel
- [ ] Package manager install/remove UI

### P5 — WebSocket Clients

- [ ] Web client (`cmd/wsdevice/web/`) — browser-based virtual deck
- [ ] Android client (`cmd/wsdevice/android/`) — mobile virtual deck
- [ ] Real dial event handling (end-to-end: WS event → Lua `dial_change` hook)

### P6 — Testing Infrastructure

- [ ] `pkg/lualib` unit tests (all pure functions)
- [ ] `pkg/scripting` integration tests using `MockDevice`
- [ ] `pkg/wsdevice` protocol tests
- [ ] End-to-end CI test using `cmd/riverdeck-simulator` + `cmd/mcp-wsdevice`
- [ ] Coverage reporting in CI (target: >60% across `pkg/`)
