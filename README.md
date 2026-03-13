# Riverdeck

A programmable interface for Elgato Stream Deck devices. This Go application allows users to create custom button actions using Lua scripts, with support for folder-based navigation and a declarative layout system for organizing functionality.

## Features

- **Device Auto-Detection**: Automatically detects and configures connected Stream Deck devices
- **Lua Scripting**: Program button actions with Lua scripts for maximum flexibility
- **Folder Navigation**: Organize scripts in a hierarchical folder structure
- **Real-time Updates**: Scripts can dynamically update button appearances
- **Passive Loops**: Background script execution for status indicators and animations
- **GIF Support**: Animated GIF rendering on individual buttons
- **Package System**: Modular script packages with templates, shared libraries, and daemon scripts
- **Settings Overlay**: In-device settings menu accessible from any page
- **Display Sleep**: Configurable timeout with activity-based wake
- **Layout Mode** *(experimental -- not finished)*: Declarative, JSON-based page navigation as an alternative to folder browsing. See [Layout Mode](#layout-mode-experimental) below.
- **Editors** *(experimental -- not finished)*: Visual layout editor and HTTP API for designing button layouts. See [Editors](#editors-experimental) below.

## Usage

### Running the Application

```bash
go run ./cmd/riverdeck
```

Or build and run:

> ⚠️ **Before running `go build` or `go run` for the first time**, you must run `mage build` at least once to generate the icon file resources (e.g. `resources/icons/icon_64.png`). These files are embedded at compile time via `//go:embed` and the build will fail if they are missing.
>
> ```bash
> go install github.com/magefile/mage@latest
> mage build
> ```

```bash
go build ./cmd/riverdeck
./riverdeck
```

### Configuration

The application reads its configuration from the `.riverdeck/` directory in your home folder:

```
~/.riverdeck/
├── config.yml        # Application configuration
├── layout.json       # Declarative layout (used with layout mode)
├── _boot.lua         # Optional boot animation script
├── .packages/        # Script packages and shared libraries
├── apps/             # Application launch scripts
├── media/            # Media control scripts
└── system/           # System control scripts
```

Each script is a Lua file that defines button behavior. See [`STDLIB.md`](./.riverdeck/STDLIB.md) (included as an example in this repository) for available scripting APIs.

#### config.yml

Key settings in `config.yml`:

```yaml
application:
  brightness: 65      # Display brightness (0-100)
  passive_fps: 15     # Background script update rate
  timeout: 30         # Sleep timeout in seconds

ui:
  navigation_style: folder  # "folder", "layout", or "auto"
```

Setting `navigation_style` to `"auto"` (or omitting it) will use layout mode if `layout.json` exists, and fall back to folder navigation otherwise.

## Navigation Modes

### Folder Navigation

The default mode. Scripts and directories inside `.riverdeck/` are mapped directly to buttons. Subdirectories become nested pages with automatic pagination. A reserved column provides Back, Home, and Settings buttons.

### Layout Mode *(experimental)*

> ⚠️ **Layout mode is experimental and is in no way finished.** Expect missing features, bugs, and breaking changes.

Layout mode uses a `layout.json` file to declaratively define pages and button positions. Buttons are placed at explicit grid slots rather than derived from the filesystem, giving full control over layout and ordering. Enable it by setting `navigation_style: layout` in `config.yml` (or use `"auto"` to enable it automatically when `layout.json` is present).

Example `layout.json`:

```json
{
  "pages": [
    {
      "name": "Main",
      "buttons": [
        { "slot": 0, "label": "HOME", "action": "home" },
        { "slot": 1, "label": "Media", "action": "page", "target_page": "Media" }
      ]
    }
  ]
}
```

## Editors *(experimental)*

> ⚠️ **The layout editor and editor server are experimental and are in no way finished.** Expect missing features, bugs, and breaking changes.

Two editor components are provided for working with layout mode:

- **`riverdeck-wails`** -- A standalone desktop GUI (built with [Wails v2](https://wails.io/)) for visually designing button layouts. Build and run it with:

  ```bash
  go build ./cmd/riverdeck-wails
  ./riverdeck-wails
  ```

- **Editor HTTP API** -- An embedded HTTP server (`pkg/editorserver`) that serves the editor frontend and exposes endpoints for reading and writing `layout.json` and script files. This API is used internally by `riverdeck-wails`.

## Requirements

- Go 1.24+
- A connected Elgato Stream Deck device
- CGO enabled (required for HID library)

## Supported Models

- Original Stream Deck (PID 0x0060)
- Stream Deck V2 (PID 0x006d)
- Stream Deck Mini (PID 0x0063)
- Stream Deck XL (PID 0x006c)
- Stream Deck Mini MK2 (PID 0x0090)

## Architecture

The repository is organized into commands and packages:

### Commands (`cmd/`)

| Command | Description |
|---------|-------------|
| `riverdeck` | Main application -- device communication, scripting, and systray |
| `riverdeck-wails` | Layout editor GUI *(experimental)* |
| `riverdeck-debug-prober` | Device inspection and feature report debugging utility |
| `riverdeck-simulator` | Software simulator for testing without hardware |

### Packages (`pkg/`)

| Package | Description |
|---------|-------------|
| `pkg/streamdeck/` | Low-level device communication, navigation (folder and layout), key handling |
| `pkg/scripting/` | Lua script execution, lifecycle management, and package loading |
| `pkg/scripting/modules/` | Built-in Lua modules (`http`, `shell`, `file`, `store`, `config`, etc.) |
| `pkg/lualib/` | Lua standard library extensions (`json`, `base64`, `crypto`, `regex`, etc.) |
| `pkg/layout/` | Layout model types and `layout.json` load/save logic *(experimental)* |
| `pkg/editorserver/` | HTTP API server for the layout editor *(experimental)* |
| `pkg/imaging/` | Image loading, resizing, and GIF frame extraction |
| `pkg/platform/` | Platform-specific config directory resolution |
| `pkg/prober/` | Device enumeration and HID feature report probing |
| `pkg/resolver/` | `pkg://` URI resolver for package-relative resources |

## Contributing

### Adding New Features

1. **Script APIs**: Add new Lua modules in `pkg/scripting/modules/` or extend `pkg/lualib/`
2. **Device Support**: Add new models in `pkg/streamdeck/models.go`
3. **Navigation**: Folder navigation lives in `pkg/streamdeck/navigation.go`; layout navigation in `pkg/streamdeck/layout_navigator.go`
4. **UI Components**: Enhance the `App` struct in `cmd/riverdeck/app.go` for new capabilities

### Code Organization

- Use interfaces for extensible components (see `pkg/streamdeck/navigator_iface.go`)
- Add comprehensive documentation and comments
- Follow Go best practices and naming conventions
- Test changes with actual hardware when possible

### Submitting Changes

- Create feature branches for new functionality
- Include tests for new code
- Update documentation as needed
- Follow the project's issue templates for feature requests

## Dependencies

- `github.com/sstallion/go-hid`: HID device communication
- `github.com/yuin/gopher-lua`: Lua script execution
- `golang.org/x/image`: Image processing for button displays
- `github.com/getlantern/systray`: System tray integration
- `github.com/wailsapp/wails/v2`: Desktop GUI framework for the layout editor (`riverdeck-wails`)
- `fyne.io/fyne/v2`: GUI toolkit used by the debug prober (`riverdeck-debug-prober`)

## Notes

- Requires CGO for HID access
- Automatically selects the appropriate image format (JPEG/BMP) based on device model
- Uses system fonts for text rendering on buttons
- Designed for integration with the broader Riverdeck ecosystem
