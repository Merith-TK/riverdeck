// Command mcp-wsdevice is an MCP server that acts as a riverdeck WebSocket
// software client.  It lets Claude connect to a running riverdeck instance,
// inspect the current layout state, simulate button presses, and read per-
// device layout files -- all without needing manual interaction.
//
// Tools exposed:
//
//	rd_connect    - Open a WS connection to the riverdeck WS device server.
//	rd_disconnect - Close the current WS connection.
//	rd_get_state  - Report connection status, device info, and recent messages.
//	rd_press_key  - Simulate a button press (sends key-down then key-up).
//	rd_read_layout - Read the layout.json for the connected device UUID.
package main

import (
	"log"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	log.SetFlags(0)
	log.SetOutput(os.Stderr)

	s := server.NewMCPServer("riverdeck-wsdevice", "1.0.0",
		server.WithToolCapabilities(true),
	)

	s.AddTool(mcp.NewTool("rd_connect",
		mcp.WithDescription("Connect to the riverdeck WebSocket device server and register as a software client device. Receives devinfo handshake and starts streaming key images."),
		mcp.WithNumber("port",
			mcp.Description("TCP port the riverdeck WS server is listening on (default 9000)"),
		),
		mcp.WithString("uuid",
			mcp.Description("Optional UUID for session resumption -- reuses a previous device identity"),
		),
		mcp.WithString("config_dir",
			mcp.Description("Path to the riverdeck config directory (default ~/.riverdeck)"),
		),
	), toolConnect)

	s.AddTool(mcp.NewTool("rd_disconnect",
		mcp.WithDescription("Close the current WebSocket connection to the riverdeck server."),
	), toolDisconnect)

	s.AddTool(mcp.NewTool("rd_get_state",
		mcp.WithDescription("Return the current connection status, device geometry, key image update log, and recent message history."),
	), toolGetState)

	s.AddTool(mcp.NewTool("rd_press_key",
		mcp.WithDescription("Simulate a physical button press on the virtual device (sends key-down then key-up). Reports which keys were redrawn as a result."),
		mcp.WithNumber("key",
			mcp.Required(),
			mcp.Description("Physical key index (0-based, row-major: key = row*cols + col)"),
		),
	), toolPressKey)

	s.AddTool(mcp.NewTool("rd_read_layout",
		mcp.WithDescription("Read and return the layout.json file for the currently connected device UUID from the riverdeck config directory."),
	), toolReadLayout)

	if err := server.ServeStdio(s); err != nil {
		log.Fatalf("mcp-wsdevice: %v", err)
	}
}
