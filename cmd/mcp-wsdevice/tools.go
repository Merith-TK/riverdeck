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
	configDir := req.GetString("config_dir", "")
	if configDir == "" {
		configDir = defaultConfigDir()
	}

	// Allow overriding the device ID for multidevice testing.
	overrideID := req.GetString("device_id", "")

	state.mu.Lock()
	if state.connected {
		state.mu.Unlock()
		return mcp.NewToolResultError("already connected -- call rd_disconnect first"), nil
	}
	state.mu.Unlock()

	// Load or generate persistent device ID.
	deviceID := overrideID
	if deviceID == "" {
		deviceID = loadOrCreateDeviceID(configDir)
	}

	addr := fmt.Sprintf("ws://localhost:%d/ws", port)
	conn, _, err := websocket.DefaultDialer.Dial(addr, nil)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("dial %s: %v", addr, err)), nil
	}

	state.mu.Lock()
	state.conn = conn
	state.connected = true
	state.configDir = configDir
	state.deviceID = deviceID
	state.keyUpdates = make(map[int]time.Time)
	state.labels = make(map[int]string)
	state.frameData = make(map[int]string)
	state.messages = nil
	hello := buildHelloMsg(deviceID)
	state.inputIDs = make([]string, len(hello.Inputs))
	for i, inp := range hello.Inputs {
		state.inputIDs[i] = inp.ID
	}
	state.logMsg(fmt.Sprintf("connecting to %s as id=%s", addr, deviceID))
	state.mu.Unlock()

	// Send hello.
	if err := sendJSON(hello); err != nil {
		_ = conn.Close()
		state.mu.Lock()
		state.connected = false
		state.conn = nil
		state.mu.Unlock()
		return mcp.NewToolResultError("send hello: " + err.Error()), nil
	}

	// Start background reader (handles ack, frame, label, etc.).
	startReadLoop(conn)

	// Wait for ack (up to 2s).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		state.mu.Lock()
		msgs := append([]string(nil), state.messages...)
		state.mu.Unlock()
		for _, m := range msgs {
			if strings.Contains(m, "ack: status=ok") {
				return mcp.NewToolResultText(fmt.Sprintf(
					"Connected.\ndeviceID: %s\ninputs:   %d\nconfigDir: %s",
					deviceID, len(hello.Inputs), configDir,
				)), nil
			}
			if strings.Contains(m, "ack: status=error") {
				_ = conn.Close()
				state.mu.Lock()
				state.connected = false
				state.conn = nil
				state.mu.Unlock()
				return mcp.NewToolResultError("server rejected hello: " + m), nil
			}
		}
		time.Sleep(50 * time.Millisecond)
	}

	return mcp.NewToolResultError("timeout waiting for ack from server"), nil
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
		fmt.Fprintf(&sb, "deviceID:   %s\n", state.deviceID)
		fmt.Fprintf(&sb, "inputs:     %d\n", len(state.inputIDs))
		fmt.Fprintf(&sb, "brightness: %d%%\n", state.brightness)
		fmt.Fprintf(&sb, "configDir:  %s\n", state.configDir)
	}

	if len(state.keyUpdates) > 0 {
		fmt.Fprintf(&sb, "\nframe updates received:")
		for idx, id := range state.inputIDs {
			if t, ok := state.keyUpdates[idx]; ok {
				dataLen := len(state.frameData[idx])
				fmt.Fprintf(&sb, "\n  key %2d (%s)  last updated %s ago  data_len=%d",
					idx, id, time.Since(t).Round(time.Millisecond), dataLen)
			}
		}
	}

	if len(state.labels) > 0 {
		fmt.Fprintf(&sb, "\n\ncurrent labels:")
		for idx, text := range state.labels {
			inputID := ""
			if idx < len(state.inputIDs) {
				inputID = state.inputIDs[idx]
			}
			fmt.Fprintf(&sb, "\n  key %2d (%s)  %q", idx, inputID, text)
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
	inputIDs := state.inputIDs
	state.mu.Unlock()

	if !connected {
		return mcp.NewToolResultError("not connected"), nil
	}
	if key < 0 || key >= len(inputIDs) {
		return mcp.NewToolResultError(fmt.Sprintf("key %d out of range (0-%d)", key, len(inputIDs)-1)), nil
	}

	inputID := inputIDs[key]

	// Send press.
	if err := sendJSON(map[string]any{"type": "input", "id": inputID, "event": "press"}); err != nil {
		return mcp.NewToolResultError("send press: " + err.Error()), nil
	}
	time.Sleep(50 * time.Millisecond)
	// Send release.
	if err := sendJSON(map[string]any{"type": "input", "id": inputID, "event": "release"}); err != nil {
		return mcp.NewToolResultError("send release: " + err.Error()), nil
	}

	// Wait briefly for any resulting frame/label messages.
	time.Sleep(300 * time.Millisecond)

	state.mu.Lock()
	var updated []string
	for k := range state.inputIDs {
		if t, ok := state.keyUpdates[k]; ok && time.Since(t) < 500*time.Millisecond {
			updated = append(updated, fmt.Sprintf("key %d (%s)", k, state.inputIDs[k]))
		}
	}
	var labelChanges []string
	for k, text := range state.labels {
		if t, ok := state.keyUpdates[k]; ok && time.Since(t) < 500*time.Millisecond {
			labelChanges = append(labelChanges, fmt.Sprintf("key %d: %q", k, text))
		}
	}
	state.mu.Unlock()

	result := fmt.Sprintf("Pressed key %d (%s).", key, inputID)
	if len(updated) > 0 {
		result += "\nFrames updated: " + strings.Join(updated, ", ")
	}
	if len(labelChanges) > 0 {
		result += "\nLabels after press: " + strings.Join(labelChanges, "; ")
	}
	return mcp.NewToolResultText(result), nil
}

func toolReadLayout(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	state.mu.Lock()
	deviceID := state.deviceID
	configDir := state.configDir
	connected := state.connected
	state.mu.Unlock()

	if !connected || deviceID == "" {
		return mcp.NewToolResultError("not connected (call rd_connect first)"), nil
	}

	lay, err := layout.LoadForDevice(configDir, deviceID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("load layout from %s: %v", configDir, err)), nil
	}
	if lay == nil || len(lay.Pages) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("No layout found for id=%s in %s", deviceID, configDir)), nil
	}

	data, err := json.MarshalIndent(lay, "", "  ")
	if err != nil {
		return mcp.NewToolResultError("marshal layout: " + err.Error()), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("layout for id=%s (%s):\n\n%s", deviceID, configDir, string(data))), nil
}

func toolListInputs(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	state.mu.Lock()
	connected := state.connected
	inputIDs := append([]string(nil), state.inputIDs...)
	deviceID := state.deviceID
	state.mu.Unlock()

	if !connected {
		return mcp.NewToolResultError("not connected (call rd_connect first)"), nil
	}

	hello := buildHelloMsg(deviceID)
	var sb strings.Builder
	fmt.Fprintf(&sb, "Inputs declared by this MCP client (id=%s):\n", deviceID)
	for i, inp := range hello.Inputs {
		if i >= len(inputIDs) {
			break
		}
		fmt.Fprintf(&sb, "  [%2d] id=%-8s type=%-8s x=%d y=%d image=%v text=%v",
			i, inp.ID, inp.Type, inp.X, inp.Y, inp.Display.Image, inp.Display.Text)
		if len(inp.Display.Formats) > 0 {
			fmt.Fprintf(&sb, " formats=%v", inp.Display.Formats)
		}
		fmt.Fprintln(&sb)
	}
	return mcp.NewToolResultText(sb.String()), nil
}
