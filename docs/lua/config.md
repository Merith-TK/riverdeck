# `config` -- Per-Button Configuration

```lua
local config = require("config")
-- or:
local config = require("riverdeck.config")
```

Provides access to per-button configuration values. Config is defined as a schema with defaults, and overridden per-button by the user.

## Legacy API

```lua
-- Get a single value (returns the override if set, otherwise the schema default)
local val = config.get("volume")

-- Get all merged key/value pairs as a table
local all = config.all()

-- Get the schema definition
local schema = config.schema()
-- Returns: { {key, label, type, default, description}, ... }
```

## New API (`cfg.script`)

```lua
local cfg = require("riverdeck.config")

-- Declare defaults in your script
cfg.script.defaultdata = {
    volume = 5,
    mode = "normal"
}

-- Sync: merges defaultdata with any saved overrides
cfg.script.sync()

-- Read/write values
local vol = cfg.script.data.volume
cfg.script.data.volume = 10

-- Save overrides back to disk
cfg.script.save()
```

## Config Sources

| Navigation Mode | Source |
|----------------|--------|
| Folder mode | Sibling `.config.json` file next to the Lua script |
| Layout mode | Button `metadata` map in `layout.json` |

### Folder Mode `.config.json` Format

```json
{
  "schema": [
    {
      "key": "volume",
      "label": "Volume",
      "type": "number",
      "default": "5",
      "description": "Default volume level"
    }
  ],
  "overrides": {
    "volume": "8"
  }
}
```

All values are stored as strings, even numbers and booleans. Parse as needed.
