# Editor v2 -- Design Document

> **Status:** In progress  
> **Date:** 2026-03-11

---

## 1. Overview

The current Riverdeck editor is a localhost-only web application
(`pkg/editorserver` + `resources/editor/index.html`) displayed inside a
WebView2 window (`riverdeck-editor-window`).  The editor lets users
manage layout pages, assign package templates to button slots, and
edit Lua scripts inline via an embedded Monaco editor.

Editor v2 addresses five gaps:

| # | Gap | Solution |
|---|-----|----------|
| 1 | No drag-and-drop for package templates | Drag templates from the package panel onto the deck grid |
| 2 | No tool to create/edit packages | Documented format; dedicated `riverdeck-pkg-editor` CLI later |
| 3 | No way to create new Lua files or use starter templates | "New script" button + bundled Lua starter template |
| 4 | No per-button configuration that the editor can render (dual-table) | `config` Lua module + JSON-backed defaults/overrides |
| 5 | Web server attack surface | Long-term: **Fyne-native editor** replacing web UI entirely |

---

## 2. Configuration System (Dual-Table)

### 2.1 Concept

Each Lua script may expose configurable fields ("knobs") that the GUI editor
renders as form inputs.  The system uses two layers:

- **Defaults** -- shipped by the package author; define the schema + initial values.
- **Overrides** -- user customisations; only stores values that differ from defaults.

At runtime the `config` module merges both and hands the result to the script.

### 2.2 Storage

| Navigation mode | Where defaults live | Where overrides live |
|-----------------|--------------------|--------------------|
| **Folder** | `<script>.config.json` sibling file | Same file, `"overrides"` key |
| **Layout** | Package template `MetadataSchema` | `layout.json` -> button `metadata` map |

#### Folder-mode config file (`volume.config.json`)

```json
{
  "schema": [
    {"key": "step", "label": "Volume Step", "type": "number", "default": "5"},
    {"key": "target", "label": "Target Device", "type": "text", "default": ""}
  ],
  "overrides": {
    "step": "10"
  }
}
```

The schema array uses the same `MetadataField` shape already defined in
`pkg/scripting/packages.go`, so Package templates and standalone scripts
share one schema format.

#### Layout-mode config

In layout mode the schema comes from the package template's
`metadata_schema`, and overrides are stored inline in
`layout.json -> button.metadata`.  No additional file needed.

### 2.3 Lua API -- `require('config')`

```lua
local config = require('config')

-- Read a merged value (override wins over default).
local step = config.get("step")          -- "10"   (overridden)
local target = config.get("target")      -- ""     (default)

-- Read all merged values as a table.
local all = config.all()                 -- {step="10", target=""}

-- Get only schema entries (for introspection).
local schema = config.schema()
-- Returns: {{key="step", label="Volume Step", type="number", default="5"}, ...}
```

The module is read-only at runtime.  The editor writes overrides through the
REST API or (future) Fyne UI.

### 2.4 Go implementation

New module: `pkg/scripting/modules/config.go`

```
ConfigModule {
    schema   []MetadataField   // from .config.json or template
    defaults map[string]string  // extracted from schema
    overrides map[string]string // user customisations
}
```

`ScriptRunner` populates it:

- **Folder mode:** reads `<scriptPath>.config.json`; merges schema defaults + overrides.
- **Layout mode:** uses the template's `MetadataSchema` for defaults and the
  button's `Metadata` for overrides.  Both are passed from `ScriptManager.Boot()`
  which now receives the current layout.

---

## 3. Drag-and-Drop Templates

### Web editor (current)

The package panel templates already render as clickable rows.  Adding
drag-and-drop means:

1. Set `draggable="true"` on `.pkg-tmpl` elements.
2. Store `{pkgID, templateKey}` in `dataTransfer`.
3. Add `dragover`/`drop` handlers on `.key-cell` elements.
4. On drop: call the existing `applyTemplate(pkg, tmpl)` logic with the
   target slot derived from the drop target's `data-slot` attribute.

### Fyne editor (future)

Fyne's `widget.List` + `dnd` API (or custom `CanvasObject` drag hooks).

---

## 4. New Lua File Creation

### Starter template

A bundled Lua template is embedded in `resources/packages/riverdeck/templates/starter.lua`:

```lua
-- Riverdeck button script
-- Rename and customise this file.

local M = {}

--- passive(state) is called every ~1s to update the button appearance.
--- Return a table with {text=, color={r,g,b}} to update the display.
function M.passive(state)
    return {
        text  = "Hello",
        color = {r = 100, g = 100, b = 255},
    }
end

--- trigger(state) is called when the button is pressed.
function M.trigger(state)
    -- Your action here
end

return M
```

### Web editor UX

Add a "+ New Script" button in the config panel (visible when action=script).
Flow:

1. User clicks "+ New Script".
2. Prompt for filename (pre-filled with `button_<slot>.lua`).
3. POST to `/api/file?path=<name>` with the starter template contents.
4. Auto-populate the button's `script` field and open in Monaco.

### API endpoint

`POST /api/file/new` -- creates a new Lua file from the starter template.
Body: `{"path": "myscript.lua"}`  
Returns: `{"path": "myscript.lua", "created": true}`  
409 if file already exists.

---

## 5. Fyne-native Editor (Replaces Web UI)

### 5.1 Rationale

- Eliminates the HTTP attack surface entirely -- no localhost server needed.
- No dependency on WebView2 runtime (common pain point on clean Windows installs).
- Consistent look-and-feel with the system tray and settings overlay.
- Can share Go data structures directly (no JSON serialisation boundary).

### 5.2 Architecture

```
cmd/riverdeck-gui/           ← existing standalone; becomes full editor
pkg/editor/                  ← NEW: Fyne-based editor widgets
    editor.go                   Main editor container
    grid.go                     Deck grid canvas widget
    page_tabs.go                Page tab strip
    button_config.go            Button property panel
    package_browser.go          Package template browser
    lua_editor.go               Lua code editor panel
    config_form.go              Auto-generated config form from schema
    types.go                    Shared UI types
```

### 5.3 Widget breakdown

| Widget | Fyne type | Purpose |
|--------|-----------|---------|
| `DeckGrid` | Custom `CanvasObject` | Draws the key grid as rounded rects; handles click + drag-drop |
| `PageTabs` | `container.AppTabs` or custom tab strip | Switchable pages |
| `ButtonConfig` | `widget.Form` + dynamic fields | Properties panel for selected button |
| `PackageBrowser` | `widget.Tree` | Collapsible package -> template list; drag source |
| `LuaEditor` | `widget.Entry` (multiline, monospace) | Basic Lua editing; Fyne lacks Monaco but is functional |
| `ConfigForm` | `widget.Form` | Auto-rendered from `MetadataSchema` |

### 5.4 Data flow

```
                 layout.Layout (shared ptr)
                        │
    ┌───────────────────┼───────────────────┐
    │                   │                   │
DeckGrid          PageTabs          ButtonConfig
    │                   │                   │
    └───────── onLayoutChanged() ───────────┘
                        │
                  layout.Save()
                        │
                  OnLayoutSaved callback
                        │
                  app.reloadNavigator()
```

No HTTP involved.  The editor widgets operate directly on `*layout.Layout`.

### 5.5 Migration path

1. **Phase 1 (now):** Ship backend infrastructure (config module, APIs, drag-and-drop, new file).
   Web editor continues to be the primary UI.
2. **Phase 2:** Build `pkg/editor` with Fyne widgets.  Initially launched from
   `cmd/riverdeck-gui` as a standalone editor.
3. **Phase 3:** Integrate into the main `cmd/riverdeck` process as a menu action
   ("Open Native Editor") alongside the existing webview option.
4. **Phase 4:** Deprecate web editor; remove `pkg/editorserver`.

---

## 6. Package Format Documentation

See [package-format.md](package-format.md) for the full specification.

Summary:

```
.packages/
  vendor.pkgname/
    manifest.json       # Package metadata + template definitions
    lib/                # Lua libraries (auto-added to package.path)
    templates/          # Button script templates
    icons/              # Icon images
    data/               # Persistent runtime data (auto-created)
    daemon.lua          # Optional background daemon
```

---

## 7. Open Questions

- **Lua editor in Fyne:** `widget.Entry` is basic -- no syntax highlighting,
  no autocomplete.  Consider embedding a `canvas.Text`-based custom editor
  with basic keyword highlighting, or use an external editor launch command
  (`$EDITOR` / VS Code / Notepad++).
- **Config hot-reload:** Should changing a config value in the editor
  trigger an immediate script restart, or require a manual "Reload" action?
  Current recommendation: immediate restart with debounce.
- **Package distribution:** Future work -- a package registry / ZIP import
  flow.  Out of scope for v2.
