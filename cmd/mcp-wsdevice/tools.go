package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/merith-tk/riverdeck/pkg/layout"
)

func toolConnect(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	port := req.GetInt("port", 9000)
	uuid := req.GetString("uuid", "")
	configDir := req.GetString("config_dir", "")
	if configDir == "" {
		configDir = defaultConfigDir()
	}

	// Auto-resume: if no UUID was explicitly provided, try to load the stored one.
	resumed := false
	if uuid == "" {
		if stored := loadStoredUUID(configDir); stored != "" {
			uuid = stored
			resumed = true
		}
	}

	state.mu.Lock()
	if state.connected {
		state.mu.Unlock()
		return mcp.NewToolResultError("already connected -- call rd_disconnect first"), nil
	}
	state.mu.Unlock()

	addr := fmt.Sprintf("ws://localhost:%d/ws", port)
	if uuid != "" {
		addr += "?uuid=" + uuid
	}

	conn, _, err := websocket.DefaultDialer.Dial(addr, nil)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("dial %s: %v", addr, err)), nil
	}

	state.mu.Lock()
	state.conn = conn
	state.connected = true
	state.configDir = configDir
	state.keyUpdates = make(map[int]time.Time)
	state.messages = nil
	state.logMsg(fmt.Sprintf("connected to %s", addr))
	state.mu.Unlock()

	startReadLoop(conn)

	// Wait briefly for the devinfo handshake.
	time.Sleep(300 * time.Millisecond)

	state.mu.Lock()
	uid := state.uuid
	model := state.modelName
	cols, rows, keys := state.cols, state.rows, state.keys
	state.mu.Unlock()

	// Persist the UUID so future sessions auto-resume.
	storeUUID(configDir, uid)

	resumeNote := ""
	if resumed {
		resumeNote = "  (resumed stored session)"
	}

	return mcp.NewToolResultText(fmt.Sprintf(
		"Connected.%s\nuuid:  %s\nmodel: %s\ngrid:  %dx%d (%d keys)\nconfigDir: %s",
		resumeNote, uid, model, cols, rows, keys, configDir,
	)), nil
}

func toolDisconnect(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	state.mu.Lock()
	conn := state.conn
	state.connected = false
	state.conn = nil
	state.mu.Unlock()

	if conn == nil {
		return mcp.NewToolResultError("not connected"), nil
	}
	_ = conn.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	_ = conn.Close()
	return mcp.NewToolResultText("Disconnected."), nil
}

func toolGetState(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	state.mu.Lock()
	defer state.mu.Unlock()

	var sb strings.Builder
	fmt.Fprintf(&sb, "connected: %v\n", state.connected)
	if state.connected {
		fmt.Fprintf(&sb, "uuid:      %s\n", state.uuid)
		fmt.Fprintf(&sb, "model:     %s\n", state.modelName)
		fmt.Fprintf(&sb, "grid:      %dx%d (%d keys, %dpx)\n",
			state.cols, state.rows, state.keys, state.pixelSize)
		fmt.Fprintf(&sb, "configDir: %s\n", state.configDir)
	}

	if len(state.keyUpdates) > 0 {
		fmt.Fprintf(&sb, "\nkey images received:")
		for k := range state.keys {
			if t, ok := state.keyUpdates[k]; ok {
				fmt.Fprintf(&sb, "\n  key %2d  last updated %s ago", k, time.Since(t).Round(time.Millisecond))
			}
		}
	}

	if len(state.messages) > 0 {
		fmt.Fprintf(&sb, "\n\nmessage log (newest last):\n")
		for _, m := range state.messages {
			fmt.Fprintf(&sb, "  %s\n", m)
		}
	} else {
		fmt.Fprintf(&sb, "\n(no messages yet)")
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func toolPressKey(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	key, err := req.RequireInt("key")
	if err != nil {
		return mcp.NewToolResultError("key parameter required"), nil
	}

	state.mu.Lock()
	connected := state.connected
	keys := state.keys
	state.mu.Unlock()

	if !connected {
		return mcp.NewToolResultError("not connected"), nil
	}
	if keys > 0 && (key < 0 || key >= keys) {
		return mcp.NewToolResultError(fmt.Sprintf("key %d out of range (0-%d)", key, keys-1)), nil
	}

	// Send key-down.
	if err := sendJSON(map[string]any{"type": "keyevent", "key": key, "pressed": true}); err != nil {
		return mcp.NewToolResultError("send key-down: " + err.Error()), nil
	}
	// Brief hold -- mirrors a real button press.
	time.Sleep(50 * time.Millisecond)
	// Send key-up.
	if err := sendJSON(map[string]any{"type": "keyevent", "key": key, "pressed": false}); err != nil {
		return mcp.NewToolResultError("send key-up: " + err.Error()), nil
	}

	// Wait for any resulting setimage messages.
	time.Sleep(300 * time.Millisecond)

	state.mu.Lock()
	var updated []string
	for k := range state.keys {
		if t, ok := state.keyUpdates[k]; ok && time.Since(t) < 500*time.Millisecond {
			updated = append(updated, fmt.Sprintf("key %d", k))
		}
	}
	state.mu.Unlock()

	result := fmt.Sprintf("Pressed key %d.", key)
	if len(updated) > 0 {
		result += "\nKeys updated after press: " + strings.Join(updated, ", ")
	}
	return mcp.NewToolResultText(result), nil
}

func toolReadLayout(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	state.mu.Lock()
	uuid := state.uuid
	configDir := state.configDir
	connected := state.connected
	state.mu.Unlock()

	if !connected || uuid == "" {
		return mcp.NewToolResultError("not connected (call rd_connect first)"), nil
	}

	lay, err := layout.LoadForDevice(configDir, uuid)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("load layout from %s: %v", configDir, err)), nil
	}
	if lay == nil || len(lay.Pages) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("No layout found for uuid=%s in %s", uuid, configDir)), nil
	}

	data, err := json.MarshalIndent(lay, "", "  ")
	if err != nil {
		return mcp.NewToolResultError("marshal layout: " + err.Error()), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("layout for uuid=%s (%s):\n\n%s", uuid, configDir, string(data))), nil
}
