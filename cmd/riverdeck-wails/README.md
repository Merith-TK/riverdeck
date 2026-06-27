# riverdeck-wails

> **Experimental -- large portions do not work.**
>
> This editor is in early development. The UI renders but many editing operations are broken, incomplete, or absent. Do not rely on it for any production workflow.

A standalone desktop GUI for designing Riverdeck button layouts, built with [Wails v2](https://wails.io/).

## What Works

- Launching the application
- Viewing the editor interface

## What Doesn't Work

- Most editing operations
- Saving layout changes reliably
- Script file management
- Package browsing
- Drag-and-drop layout editing

## Building

Wails must be installed:

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

Then from the project root:

```bash
cd cmd/riverdeck-wails
wails build
```

For live development with hot reload:

```bash
wails dev
```

The dev server also exposes Go methods at `http://localhost:34115` for browser-based testing.

## Architecture

The editor frontend communicates with the main Riverdeck process via the embedded HTTP API (`pkg/editorserver`). The Wails app shells around this and provides a native window and OS integration (file dialogs, tray, etc.).

Until the editor reaches a functional state, layout files can be edited directly as JSON. See the [layout.json format in the main README](../../README.md#layout-mode-experimental).
