# Configuration Reference

Riverdeck reads its configuration from `.config.yml` in the config directory.

**Config file location:**

| Platform | Path |
|----------|------|
| Linux / macOS | `~/.config/riverdeck/.config.yml` |
| Windows | `%APPDATA%\.riverdeck\.config.yml` |

The file is created automatically on first run with all defaults filled in.

## Environment Variable Overrides

Any config value can be overridden with an environment variable. The prefix is `RIVERDECK_` and nested keys use `__` as a separator:

```bash
RIVERDECK_APPLICATION__BRIGHTNESS=80
RIVERDECK_DEVICE__MULTI_MODE=individual
RIVERDECK_NETWORK__WEBSOCKET_ENABLED=true
RIVERDECK_LOGGING__LEVEL=debug
```

Environment variables are applied on top of the file config and take precedence.

---

## `application`

Core device and scripting behavior.

```yaml
application:
  brightness: 75        # Display brightness 0-100 (default: 75)
  passive_fps: 30       # Background script update rate in FPS (default: 30)
  timeout: 0            # Display sleep timeout in seconds; 0 = never (default: 0)
  debug: false          # Enable debug logging (default: false)
  git_backend: auto     # Package manager git backend: "auto", "native", "go-git" (default: "auto")
```

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `brightness` | int | `75` | Display brightness percentage (0–100) |
| `passive_fps` | int | `30` | How many times per second `label` functions are called |
| `timeout` | int | `0` | Seconds of inactivity before display sleeps; `0` disables sleep |
| `debug` | bool | `false` | Enables verbose debug output |
| `git_backend` | string | `"auto"` | Git backend for package installation (`"auto"`, `"native"`, `"go-git"`) |

---

## `device`

Physical device detection and multi-device behavior.

```yaml
device:
  auto_detect: true     # Automatically detect connected devices (default: true)
  path: ""              # Force a specific HID device path (default: "")
  model: ""             # Force a specific model name (default: "")
  multi_mode: shared    # Multi-device mode: "shared", "individual", or "layout" (default: "shared")
```

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `auto_detect` | bool | `true` | Scan for connected Stream Deck devices at startup |
| `path` | string | `""` | Override the HID device path (leave empty for auto-detection) |
| `model` | string | `""` | Force a specific model name (leave empty for auto-detection) |
| `multi_mode` | string | `"shared"` | How multiple devices share config (see below) |

### `multi_mode` Values

| Value | Description |
|-------|-------------|
| `shared` | All devices share one config tree |
| `individual` | Each device gets its own config subtree under `.device/<id>/` |
| `layout` | Devices are routed by serial ID via the `layout.json` devices map |

---

## `ui`

Navigation style and display labels.

```yaml
ui:
  navigation_style: auto   # "folder", "layout", or "auto" (default: "folder")
  show_hidden_files: false  # Show files starting with . in folder navigation (default: false)
  labels:
    back: "<-"
    home: "HOME"
```

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `navigation_style` | string | `"folder"` | `"folder"`, `"layout"`, or `"auto"` (auto uses layout if `layout.json` exists) |
| `show_hidden_files` | bool | `false` | Show dotfiles in folder navigation |
| `labels` | map | see below | Override built-in button labels |

### Default Labels

| Key | Default |
|-----|---------|
| `back` | `"<-"` |
| `home` | `"HOME"` |

---

## `scripting`

Script execution limits.

```yaml
scripting:
  enable_background: true       # Allow background coroutines (default: true)
  execution_timeout: 30         # Max seconds a trigger/label call can run (default: 30)
  max_concurrent_scripts: 10    # Max number of scripts running at once (default: 10)
```

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `enable_background` | bool | `true` | Allow scripts to define `background` functions |
| `execution_timeout` | int | `30` | Timeout in seconds for `trigger` and `label` calls |
| `max_concurrent_scripts` | int | `10` | Maximum number of simultaneous script runners |

---

## `performance`

Image rendering settings.

```yaml
performance:
  image_cache_size: 50    # Number of cached rendered frames (default: 50)
  compress_images: true   # Compress frames before sending to device (default: true)
  jpeg_quality: 90        # JPEG compression quality 1-100 (default: 90)
```

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `image_cache_size` | int | `50` | Number of rendered key frames to cache |
| `compress_images` | bool | `true` | Enable image compression |
| `jpeg_quality` | int | `90` | JPEG quality for JPEG-format devices (1–100) |

---

## `network`

HTTP client and WebSocket server settings.

```yaml
network:
  http_timeout: 10          # HTTP client timeout in seconds (default: 10)
  verify_ssl: true          # Verify SSL certificates in script HTTP calls (default: true)
  websocket_enabled: false  # Enable WebSocket device server (default: false)
  websocket_port: 9000      # WebSocket server port (default: 9000)
```

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `http_timeout` | int | `10` | Timeout in seconds for HTTP requests made by scripts |
| `verify_ssl` | bool | `true` | Whether to verify SSL certificates in `http` module calls |
| `websocket_enabled` | bool | `false` | Start the WebSocket server for virtual devices |
| `websocket_port` | int | `9000` | Port the WebSocket server listens on |

See [cmd/wsdevice/README.md](../cmd/wsdevice/README.md) for the WebSocket device protocol.

---

## `logging`

Log output settings.

```yaml
logging:
  level: info           # Log level: "debug", "info", "warn", "error" (default: "info")
  file: ""              # Log to a file path (empty = stdout only) (default: "")
  max_file_size: 10     # Max log file size in MB before rotation (default: 10)
  max_files: 5          # Number of rotated log files to keep (default: 5)
```

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `level` | string | `"info"` | Minimum log level: `"debug"`, `"info"`, `"warn"`, `"error"` |
| `file` | string | `""` | Path to write logs to (empty = stdout only) |
| `max_file_size` | int | `10` | Log file rotation size in MB |
| `max_files` | int | `5` | Number of rotated files to retain |

---

## `security`

Script sandboxing settings.

```yaml
security:
  restrict_file_access: true    # Limit file module to config directory (default: true)
  allowed_commands: []          # Allowlist for shell.exec (empty = allow all) (default: [])
  block_network: false          # Disable http module in all scripts (default: false)
```

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `restrict_file_access` | bool | `true` | Limit the `file` module to paths within the config directory |
| `allowed_commands` | []string | `[]` | If non-empty, only commands in this list can be run via `shell.exec` |
| `block_network` | bool | `false` | If `true`, the `http` module is disabled for all scripts |

---

## Per-Device Config Override

You can place a `.config.yml` in a device's individual config directory to override a subset of settings for that device only. Supported overrides:

- `application.brightness`
- `application.passive_fps`
- `application.timeout`
- `ui.navigation_style`
- `ui.show_hidden_files`
- `ui.labels`

Device config directories are at `<configDir>/.device/<deviceID>/` when `device.multi_mode` is `"individual"`.

---

## Full Example

```yaml
application:
  brightness: 75
  passive_fps: 30
  timeout: 60

device:
  auto_detect: true
  multi_mode: layout

ui:
  navigation_style: auto
  labels:
    back: "BACK"
    home: "HOME"

network:
  websocket_enabled: true
  websocket_port: 9000

logging:
  level: info
```
