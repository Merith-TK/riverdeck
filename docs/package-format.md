# Riverdeck Package Format

> **Version:** 1.0  
> **Date:** 2026-03-11

---

## Overview

A Riverdeck **package** is a directory inside `<configDir>/.packages/` that
bundles Lua libraries, button templates, icons, and (optionally) a background
daemon.  The package system lets third-party authors distribute reusable
functionality that end-users install by dropping a directory into the
`.packages/` folder.

```
<configDir>/
  .packages/
    vendor.pkgname/               ← Package root (directory name = default ID)
      manifest.json               ← Package metadata (optional but recommended)
      lib/                        ← Lua libraries (auto-added to package.path)
        mylib.lua
      templates/                  ← Button script templates
        volume_up.lua
        volume_down.lua
      icons/                      ← Icon images
        vol-up.png
        vol-down.png
      data/                       ← Persistent runtime data (auto-created)
      daemon.lua                  ← Optional background daemon script
```

---

## manifest.json

The manifest is optional.  When absent, the directory name is used as both
the ID and the display name, and `lib/` discovery still works.

### Full Schema

```json
{
  "id": "vendor.pkgname",
  "name": "Human-Readable Name",
  "version": "1.0.0",
  "description": "Short one-liner shown during boot and in the editor.",
  "order_index": 10,

  "provides": {
    "libraries": ["lib/mylib.lua"],
    "buttons":   ["templates/volume_up.lua"],
    "icon_packs": ["icons"],
    "templates": [
      {
        "id": "volume_up",
        "label": "Volume Up",
        "icon": "icons/vol-up.png",
        "script": "templates/volume_up.lua",
        "description": "Increases system volume by the configured step.",
        "metadata_schema": [
          {
            "key": "step",
            "label": "Volume Step",
            "type": "number",
            "default": "5",
            "description": "How much to increase volume per press."
          }
        ]
      }
    ]
  },

  "requires": ["vendor.other"],

  "daemon": "daemon.lua"
}
```

### Field Reference

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | `string` | No | Canonical package ID (e.g. `vendor.pkgname`). Defaults to directory name. |
| `name` | `string` | No | Human-readable display name. |
| `version` | `string` | No | Free-form version string (e.g. `1.0.0`, `2024-03-01`). |
| `description` | `string` | No | One-line description. |
| `order_index` | `int` | No | Controls editor package browser sort order. Lower values first. |
| `provides.libraries` | `string[]` | No | Relative paths to Lua library files (informational). |
| `provides.buttons` | `string[]` | No | Relative paths to standalone button scripts (informational). |
| `provides.icon_packs` | `string[]` | No | Relative paths to directories of icon images. |
| `provides.templates` | `ButtonTemplate[]` | No | Inline button template definitions (see below). |
| `requires` | `string[]` | No | IDs of packages this one depends on. Missing deps produce a boot warning. |
| `daemon` | `string` | No | Path to daemon script. Auto-detects `daemon.lua` if absent. Set to `"-"` to disable auto-detection. |

---

## Button Templates

A `ButtonTemplate` is a reusable button definition that users reference from
`layout.json` as `pkg://vendor.pkgname/template_id`.  Templates appear in
the editor's package browser for drag-and-drop assignment.

### Template Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | `string` | **Yes** | Template identifier (unique within the package). Full ref: `pkg://<pkgID>/<id>`. |
| `label` | `string` | **Yes** | Default button label text. |
| `icon` | `string` | No | Relative path to the default icon image. |
| `script` | `string` | **Yes** | Relative path to the Lua script (from the package root). |
| `description` | `string` | No | Short description shown in the editor tooltip. |
| `metadata_schema` | `MetadataField[]` | No | Per-instance configurable fields (see below). |

### MetadataSchema Fields

Each `MetadataField` defines a knob that the editor renders as a form input.
When a user assigns the template to a button, the editor shows these fields
and stores the values in `layout.json -> button.metadata`.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `key` | `string` | **Yes** | Metadata map key (e.g. `"url"`, `"step"`). |
| `label` | `string` | **Yes** | Human-readable field label. |
| `type` | `string` | No | Input type: `"text"` (default), `"number"`, or `"bool"`. |
| `default` | `string` | No | Pre-filled default value for new buttons. |
| `description` | `string` | No | One-line hint shown below the field in the editor. |

---

## Accessing Config from Lua

Template scripts (and folder-mode scripts with a `.config.json` sibling)
can read their per-button configuration via the `config` module:

```lua
local config = require("config")

-- Get a single value (override wins over default)
local step = config.get("step") or "5"

-- Get all merged key-value pairs as a table
local all = config.all()

-- Get the schema (array of {key, label, type, default, description})
local schema = config.schema()
```

In **layout mode**, defaults come from the template's `metadata_schema` and
overrides come from the button's `metadata` map in `layout.json`.

In **folder mode**, both live in a sibling `.config.json` file:

```json
{
  "schema": [
    {
      "key": "step",
      "label": "Volume Step",
      "type": "number",
      "default": "5"
    }
  ],
  "overrides": {
    "step": "10"
  }
}
```

---

## Lua Libraries

Any `.lua` file placed in the `lib/` directory of a package is
automatically discoverable via `require()`.  Riverdeck adds every
`.packages/*/lib/` path to the Lua `package.path` for all script
runners.

```lua
-- In any button script:
local mylib = require("mylib")
-- Resolves to .packages/vendor.pkgname/lib/mylib.lua
```

---

## Daemons

A daemon is a long-running background Lua script that starts when the
package is loaded during boot.  It must export a `daemon()` function:

```lua
local M = {}
local system = require("system")

function M.daemon()
  while true do
    -- do background work (polling, websocket, etc.)
    system.sleep(5000)
  end
end

return M
```

Daemons have access to the `pkg_data` module for persistent storage
scoped to their package's `data/` directory.

Auto-detection:

- If `manifest.json` has `"daemon": "my_daemon.lua"`, that file is used.
- If `manifest.json` omits `daemon`, Riverdeck looks for `daemon.lua` in the
  package root.
- Set `"daemon": "-"` to explicitly disable auto-detection.

---

## Icon Packs

A package can ship directories of icon images that layout buttons can
reference.  Declare them in `provides.icon_packs`:

```json
{
  "provides": {
    "icon_packs": ["icons"]
  }
}
```

Buttons reference icons as `pkg://vendor.pkgname/icons/my-icon.png`.

Supported image formats: PNG, JPEG, SVG.

---

## Package Resolution

When Riverdeck encounters a `pkg://` URI (in layout button `script`,
`icon`, or `template` fields), it resolves through these steps:

1. Parse the URI: `pkg://<packageID>/<relative-path>`
2. Find the installed package with matching ID in `.packages/`
3. Join the package root with the relative path
4. Return the absolute filesystem path

Example: `pkg://riverdeck/templates/home.lua`
-> `<configDir>/.packages/riverdeck/templates/home.lua`

---

## Example: Minimal Package

```
.packages/
  mytools/
    manifest.json
    templates/
      clock.lua
```

**manifest.json:**

```json
{
  "id": "mytools",
  "name": "My Tools",
  "version": "1.0.0",
  "description": "A simple clock button.",
  "provides": {
    "templates": [
      {
        "id": "clock",
        "label": "Clock",
        "script": "templates/clock.lua",
        "description": "Shows the current time."
      }
    ]
  }
}
```

**templates/clock.lua:**

```lua
local M = {}
local time = require("time")

function M.passive(ctx)
  ctx.text(time.format(time.now(), "15:04:05"))
end

return M
```

---

## Example: Full Package with Daemon

```
.packages/
  homeassistant/
    manifest.json
    lib/
      ha_client.lua
    templates/
      light_toggle.lua
      sensor.lua
    icons/
      light.svg
      sensor.svg
    daemon.lua
```

**manifest.json:**

```json
{
  "id": "homeassistant",
  "name": "Home Assistant",
  "version": "2.0.0",
  "description": "Control Home Assistant entities from your Stream Deck.",
  "provides": {
    "libraries": ["lib/ha_client.lua"],
    "templates": [
      {
        "id": "light_toggle",
        "label": "Light Toggle",
        "icon": "icons/light.svg",
        "script": "templates/light_toggle.lua",
        "description": "Toggle a Home Assistant light entity.",
        "metadata_schema": [
          {
            "key": "entity_id",
            "label": "Entity ID",
            "type": "text",
            "description": "The Home Assistant entity ID (e.g. light.living_room)."
          },
          {
            "key": "ha_url",
            "label": "HA URL",
            "type": "text",
            "default": "http://homeassistant.local:8123",
            "description": "Home Assistant base URL."
          }
        ]
      },
      {
        "id": "sensor",
        "label": "Sensor Display",
        "icon": "icons/sensor.svg",
        "script": "templates/sensor.lua",
        "description": "Display a Home Assistant sensor value.",
        "metadata_schema": [
          {
            "key": "entity_id",
            "label": "Entity ID",
            "type": "text"
          }
        ]
      }
    ]
  },
  "daemon": "daemon.lua"
}
```
