# Riverdeck

A programmable interface for Elgato Stream Deck devices. This Go application allows users to create custom button actions using Lua scripts, with folder-based navigation for organizing functionality.

## Features

- **Device Auto-Detection**: Automatically detects and configures connected Stream Deck devices
- **Lua Scripting**: Program button actions with Lua scripts for maximum flexibility
- **Folder Navigation**: Organize scripts in a hierarchical folder structure
- **Real-time Updates**: Scripts can dynamically update button appearances
- **Passive Loops**: Background script execution for status indicators and animations

## Usage

### Running the Application

```bash
go run .
```

Or build and run:

```bash
go build
./riverdeck
```

### Configuration

Scripts are stored in the config directory structure:

```
.riverdeck/interface/streamdeck/config/
├── apps/          # Application launch scripts
├── media/         # Media control scripts
└── system/        # System control scripts
```

Each script is a Lua file that defines button behavior. See the scripting documentation for available APIs.

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

The application is structured for maintainability and extensibility:

- `main.go`: Entry point and application lifecycle
- `app.go`: Main application logic and event handling
- `config.go`: Configuration management
- `pkg/scripting/`: Lua script execution and management
- `pkg/streamdeck/`: Low-level device communication and navigation

## Contributing

### Adding New Features

1. **Script APIs**: Extend functionality in `pkg/scripting/` by adding new Lua functions
2. **Device Support**: Add new models in `pkg/streamdeck/models.go`
3. **Navigation**: Modify navigation logic in `pkg/streamdeck/navigation.go`
4. **UI Components**: Enhance the App struct in `app.go` for new capabilities

### Code Organization

- Use interfaces for extensible components
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

## Notes

- Requires CGO for HID access
- Automatically selects appropriate image format based on device model
- Uses system fonts for text rendering
- Designed for integration with the broader Riverdeck ecosystem
