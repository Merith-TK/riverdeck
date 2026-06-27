# Lua Scripting Reference

Scripts are Lua files placed in the Riverdeck config directory. Each script controls a single button.

## Script Architecture

Scripts use **module mode**: return a table with lifecycle function fields.

```lua
return {
    -- Called on each passive update cycle to get the button label
    label = function()
        return "Hello"
    end,

    -- Called when the button is pressed
    trigger = function()
        -- do something
    end,

    -- Long-running background coroutine (use system.sleep() to yield)
    background = function()
        while true do
            -- update some state
            system.sleep(1000)
        end
    end
}
```

All fields are optional. A script with only `trigger` is a simple press-action button. A script with only `label`/`background` is a read-only status display.

### Lifecycle Functions

| Field | Description |
|-------|-------------|
| `label` | Called repeatedly at `passive_fps` rate. Return a string to display on the button. |
| `trigger` | Called when the button is pressed. |
| `background` | Long-running coroutine. Use `system.sleep(ms)` to yield. Restarted based on `RESTART_POLICY`. |

### Global Constants

These globals are injected into every script at load time:

| Global | Type | Description |
|--------|------|-------------|
| `CONFIG_DIR` | string | Absolute path to the Riverdeck config directory |
| `SCRIPT_PATH` | string | Absolute path to this script file |
| `RESTART_POLICY` | string | Controls background restart behavior (see below) |

### Restart Policy

Set `RESTART_POLICY` at the top of your script to control what happens when the background coroutine exits:

```lua
RESTART_POLICY = "always"   -- restart whenever it exits (default)
RESTART_POLICY = "never"    -- run once, do not restart
RESTART_POLICY = "once"     -- restart once on first exit, then never again
```

## Module Index

### Application Modules

These modules provide domain-specific functionality for interacting with the host system and Riverdeck itself.

| Module | Alias | Description |
|--------|-------|-------------|
| [`config`](config.md) | `riverdeck.config` | Per-button configuration |
| [`file`](file.md) | `riverdeck.file` | File read/write within config directory |
| [`http`](http.md) | `riverdeck.http` | HTTP GET, POST, custom requests |
| [`shell`](shell.md) | `riverdeck.shell` | Shell commands, open URLs, launch terminals |
| [`store`](store.md) | `riverdeck.store` | Cross-script shared key-value store |
| [`pkg_data`](pkg_data.md) | `riverdeck.pkg_data` | Package-scoped file and JSON storage |
| [`streamdeck`](streamdeck.md) | `riverdeck.streamdeck` | Direct hardware control |
| [`system`](system.md) | `riverdeck.system` | OS info, env vars, sleep, refresh |

All modules can be required by their short name or by the `riverdeck.*` alias:

```lua
local http = require("http")
-- same as:
local http = require("riverdeck.http")
```

### Standard Library Extensions

These modules extend Lua's built-in capabilities with Go implementations.

| Module | Description |
|--------|-------------|
| [`json`](json.md) | JSON encode and decode |
| [`time`](time.md) | Timestamps, date decomposition, formatting |
| [`log`](log.md) | Levelled logging to stdout |
| [`strings`](strings.md) | String utilities (split, trim, replace, case, etc.) |
| [`base64`](base64.md) | Base64 encode/decode (standard and URL-safe) |
| [`crypto`](crypto.md) | MD5, SHA-1, SHA-256 hashing |
| [`regex`](regex.md) | Regular expressions (Go RE2 syntax) |
| [`path`](path.md) | Cross-platform file path utilities |
| [`mathx`](mathx.md) | clamp, round, lerp |

## Package Imports

Scripts that are part of an installed package can import library modules using dot-notation:

```lua
local api = require("vendor.package-name.lib.api")
```

Package imports are resolved via the package index at `.config/packages/.index.json`. See [docs/package-format.md](../package-format.md) for the package system reference.
