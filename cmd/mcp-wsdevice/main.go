// Command mcp-wsdevice is an MCP server that acts as a riverdeck WebSocket
// software client.  It lets Claude connect to a running riverdeck instance,
// inspect the current layout state (including labels), simulate button presses,
// and read per-device layout files -- all without needing manual interaction.
//
// Tools exposed:
//
//	rd_connect     - Open a WS connection and perform the hello/ack handshake.
//	rd_disconnect  - Close the current WS connection.
//	rd_get_state   - Report connection status, labels, frame updates, and message log.
//	rd_press_key   - Simulate a button press (sends press then release input events).
//	rd_read_layout - Read the layout.json for the connected device.
//	rd_list_inputs - List the inputs this MCP client declared in its hello message.
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

	s := server.NewMCPServer("riverdeck-wsdevice", "2.0.0",
		server.WithToolCapabilities(true),
	)

	s.AddTool(mcp.NewTool("rd_connect",
		mcp.WithDescription("Connect to the riverdeck WebSocket device server, send a hello handshake declaring a 5×3 button grid, and wait for ack. The device ID is persisted in .mcp-device-id so the same identity is reused across sessions."),
		mcp.WithNumber("port",
			mcp.Description("TCP port the riverdeck WS server is listening on (default 9000)"),
		),
		mcp.WithString("device_id",
			mcp.Description("Override the device ID for multidevice testing (default: load/generate from .mcp-device-id)"),
		),
		mcp.WithString("config_dir",
			mcp.Description("Path to the riverdeck config directory (default ~/.riverdeck)"),
		),
	), toolConnect)

	s.AddTool(mcp.NewTool("rd_disconnect",
		mcp.WithDescription("Close the current WebSocket connection to the riverdeck server."),
	), toolDisconnect)

	s.AddTool(mcp.NewTool("rd_get_state",
		mcp.WithDescription("Return the current connection status, current button labels, frame update timestamps, and recent message log."),
	), toolGetState)

	s.AddTool(mcp.NewTool("rd_press_key",
		mcp.WithDescription("Simulate a physical button press (sends input press then release events). Reports which keys received frame or label updates as a result."),
		mcp.WithNumber("key",
			mcp.Required(),
			mcp.Description("Physical key index (0-based, row-major: key = row*cols + col)"),
		),
	), toolPressKey)

	s.AddTool(mcp.NewTool("rd_read_layout",
		mcp.WithDescription("Read and return the layout.json for the currently connected device from the riverdeck config directory."),
	), toolReadLayout)

	s.AddTool(mcp.NewTool("rd_list_inputs",
		mcp.WithDescription("List the inputs (buttons) this MCP client declared in its hello message, with their IDs, grid positions, and display capabilities."),
	), toolListInputs)

	if err := server.ServeStdio(s); err != nil {
		log.Fatalf("mcp-wsdevice: %v", err)
	}
}
