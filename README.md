# Riverdeck

A programmable interface for Elgato Stream Deck devices. Write Lua scripts to control button actions, appearances, and background tasks. Supports folder-based navigation and a declarative layout system.

## Features

- **Device Auto-Detection** -- Automatically detects and configures connected Stream Deck devices
- **Lua Scripting** -- Program button actions with Lua scripts for maximum flexibility
- **Folder Navigation** -- Organize scripts in a hierarchical folder structure
- **Real-time Updates** -- Scripts can dynamically update button appearances
- **Passive Loops** -- Background script execution for status indicators and animations
- **GIF Support** -- Animated GIF rendering on individual buttons
- **Package System** -- Modular script packages with templates, shared libraries, and daemon scripts
- **Settings Overlay** -- In-device settings menu accessible from any page
- **Display Sleep** -- Configurable timeout with activity-based wake
- **WebSocket Devices** -- Connect virtual or custom hardware devices over the network
- **Layout Mode** *(experimental)* -- Declarative JSON-based page navigation as an alternative to folder browsing

## Requirements

- Go 1.24+
- CGO enabled (required for HID library)
- A connected Elgato Stream Deck device (or a WebSocket device client)

## Usage

> **Before the first build,** run `mage build` to generate embedded icon resources. The build will fail without this step.
>
> ```bash
> go install github.com/magefile/mage@latest
> mage build
> ```

```bash
go run ./cmd/riverdeck
```

Or build and run:

```bash
go build ./cmd/riverdeck
./riverdeck
```

## Configuration

The config directory location depends on platform:

| Platform | Path |
|----------|------|
| Linux / macOS | `~/.config/riverdeck/` |
| Windows | `%APPDATA%\.riverdeck\` |

You can override the config directory by passing `--config <path>` on the command line.

### Directory Layout

```
~/.config/riverdeck/
├── .config.yml              # Application configuration
├── layout.json              # Declarative layout (layout mode)
├── _boot.lua                # Optional boot animation script
├── .config/
│   ├── packages/            # Installed script packages
│   └── devices/             # Per-device geometry snapshots
├── apps/                    # Application launch scripts
├── media/                   # Media control scripts
└── system/                  # System control scripts
```

### `.config.yml`

```yaml
application:
  brightness: 75        # Display brightness (0-100)
  passive_fps: 30       # Background script update rate (frames per second)
  timeout: 30           # Display sleep timeout in seconds (0 = never)
  navigation_style: auto  # "folder", "layout", or "auto"

network:
  websocket_enabled: false
  websocket_port: 9000

logging:
  level: info
```

Setting `navigation_style: auto` (the default) uses layout mode when `layout.json` is present and falls back to folder navigation otherwise.

See [docs/config-reference.md](docs/config-reference.md) for the full schema.

## Navigation Modes

### Folder Navigation

The default mode. Scripts and directories inside the config directory are mapped directly to buttons. Subdirectories become nested pages with automatic pagination. A reserved column provides Back, Home, and Settings buttons.

### Layout Mode *(experimental)*

> Layout mode is experimental and not finished. Expect missing features, bugs, and breaking changes.

Layout mode uses `layout.json` to declaratively define pages and button positions. Buttons are placed at explicit grid slots rather than derived from the filesystem.

Enable it explicitly:

```yaml
application:
  navigation_style: layout
```

`layout.json` format (v2):

```json
{
  "layouts": {
    "default": {
      "pages": {
        "main": {
          "buttons": {
            "0": { "label": "Home", "script": "home.lua" },
            "1": { "label": "Media", "script": "media.lua" }
          }
        }
      }
    }
  },
  "devices": {
    "DEVICE-SERIAL-ID": "default"
  }
}
```

The `"devices"` map assigns each physical device to a named layout. Devices not in the map use the `"default"` layout if it exists.

## WebSocket Devices *(experimental)*

Riverdeck can accept virtual devices over a WebSocket connection. These devices declare their grid size and input layout on connect and receive image/label updates in return. This allows anything that can open a WebSocket to act as a Riverdeck device -- a web app, mobile app, Raspberry Pi Zero W with GPIO buttons, or a bridge to other hardware.

Enable the WebSocket server:

```yaml
network:
  websocket_enabled: true
  websocket_port: 9000
```

WebSocket devices only support layout mode. See [cmd/wsdevice/README.md](cmd/wsdevice/README.md) for the protocol spec.

## Editors *(experimental)*

> The editors are experimental and largely unfinished. Large portions do not work.

Two editor components are available for working with layout mode:

- **`riverdeck-wails`** -- A standalone desktop GUI built with [Wails v2](https://wails.io/). See [cmd/riverdeck-wails/README.md](cmd/riverdeck-wails/README.md) for status and build instructions.
- **Editor HTTP API** -- An embedded HTTP server (`pkg/editorserver`) that serves the editor frontend and exposes endpoints for reading and writing `layout.json` and script files.

## Supported Models

| Model | PID | Cols | Rows | Keys |
|-------|-----|------|------|------|
| Stream Deck Original | 0x0060 | 5 | 3 | 15 |
| Stream Deck Mini | 0x0063 | 3 | 2 | 6 |
| Stream Deck XL | 0x006c | 8 | 4 | 32 |
| Stream Deck Original V2 | 0x006d | 5 | 3 | 15 |
| Stream Deck MK.2 | 0x0080 | 5 | 3 | 15 |
| Stream Deck XL V2 | 0x0084 | 8 | 4 | 32 |
| Stream Deck Pedal | 0x0086 | 3 | 1 | 3 |
| Stream Deck Neo | 0x0090 | 4 | 2 | 8 |
| Stream Deck + | 0x009a | 4 | 2 | 8 |

See [docs/hardware-implementation-plan.md](docs/hardware-implementation-plan.md) for notes on hardware support status and models that require additional investigation.

## Scripting

Scripts are Lua files placed in the config directory. See [docs/lua/README.md](docs/lua/README.md) for the full scripting reference including all available modules.

Quick example:

```lua
-- Button that shows current time and opens a URL on press
local time = require("time")
local shell = require("shell")

return {
    label = function()
        local d = time.date()
        return string.format("%02d:%02d", d.hour, d.minute)
    end,
    trigger = function()
        shell.open("https://example.com")
    end
}
```

## Architecture

### Commands (`cmd/`)

| Command | Description |
|---------|-------------|
| `riverdeck` | Main application -- device communication, scripting, and systray |
| `riverdeck-wails` | Layout editor GUI *(experimental, largely non-functional)* |
| `riverdeck-debug-prober` | Device inspection and HID feature report tool (GUI + CLI) |
| `riverdeck-simulator` | Software simulator for testing without hardware |
| `mcp-wsdevice` | MCP server for AI-driven testing via WebSocket |
| `wsdevice/` | Reference client implementations *(planned, not yet implemented)* |

### Packages (`pkg/`)

| Package | Description |
|---------|-------------|
| `pkg/streamdeck/` | Low-level HID device communication, navigation, key handling, device models |
| `pkg/scripting/` | Lua script execution, lifecycle management, `ScriptManager` |
| `pkg/scripting/modules/` | Built-in Lua modules: `config`, `file`, `http`, `shell`, `store`, `pkg_data`, `streamdeck`, `system` |
| `pkg/lualib/` | Lua stdlib extensions: `json`, `base64`, `crypto`, `regex`, `strings`, `time`, `log`, `path`, `mathx` |
| `pkg/layout/` | Layout v2 model types and `layout.json` load/save logic |
| `pkg/editorserver/` | HTTP API server for the layout editor |
| `pkg/imaging/` | Image loading, resizing, and GIF frame extraction |
| `pkg/platform/` | Config directory resolution (cross-platform paths) |
| `pkg/prober/` | Device enumeration and HID feature report probing |
| `pkg/resolver/` | `pkg://` URI resolver for package-relative resources |
| `pkg/appearance/` | Layered key rendering (color → icon → text) |
| `pkg/gitpkg/` | Git backend for package installation (native git exec) |
| `pkg/pkgmanager/` | Package install/remove/update/list with lock file |
| `pkg/wsdevice/` | WebSocket virtual device implementing the device interface |
| `pkg/wsclient/` | Shared client-side types for building WebSocket `hello` messages |
| `pkg/util/` | Internal utility helpers |

## Dependencies

- `github.com/sstallion/go-hid` -- HID device communication
- `github.com/yuin/gopher-lua` -- Lua script execution
- `golang.org/x/image` -- Image processing
- `github.com/getlantern/systray` -- System tray integration
- `github.com/gorilla/websocket` -- WebSocket server for virtual devices
- `github.com/wailsapp/wails/v2` -- Desktop GUI for `riverdeck-wails`
- `fyne.io/fyne/v2` -- GUI toolkit for `riverdeck-debug-prober`
