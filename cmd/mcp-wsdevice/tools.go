package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/merith-tk/riverdeck/pkg/layout"
	"github.com/merith-tk/riverdeck/pkg/wsclient"
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
		deviceID = wsclient.LoadOrCreateDeviceID(configDir, ".mcp-device-id")
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
	hello := wsclient.BuildHelloMsg(deviceID, "Claude MCP Client", 3, 5, 64, []string{"png"})
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

func toolGetFrame(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	key, err := req.RequireInt("key")
	if err != nil {
		return mcp.NewToolResultError("key parameter required"), nil
	}

	state.mu.Lock()
	connected := state.connected
	frame := state.frameData[key]
	label := state.labels[key]
	inputID := ""
	if key >= 0 && key < len(state.inputIDs) {
		inputID = state.inputIDs[key]
	}
	state.mu.Unlock()

	if !connected {
		return mcp.NewToolResultError("not connected"), nil
	}
	if frame == "" {
		return mcp.NewToolResultText(fmt.Sprintf("key %d (%s): no frame data available", key, inputID)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("key=%d id=%s label=%q data_len=%d\nbase64:%s", key, inputID, label, len(frame), frame)), nil
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

func toolSetBrightness(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	value, err := req.RequireInt("value")
	if err != nil {
		return mcp.NewToolResultError("value parameter required (0-100)"), nil
	}
	if value < 0 || value > 100 {
		return mcp.NewToolResultError("value must be 0-100"), nil
	}

	state.mu.Lock()
	connected := state.connected
	state.mu.Unlock()

	if !connected {
		return mcp.NewToolResultError("not connected (call rd_connect first)"), nil
	}

	if err := sendJSON(map[string]any{"type": "setbrightness", "value": value}); err != nil {
		return mcp.NewToolResultError("send setbrightness: " + err.Error()), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Brightness set to %d%%", value)), nil
}

func toolReadLayout(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	state.mu.Lock()
	savedDeviceID := state.deviceID
	savedConfigDir := state.configDir
	state.mu.Unlock()

	deviceID := req.GetString("device_id", savedDeviceID)
	configDir := req.GetString("config_dir", savedConfigDir)
	if configDir == "" {
		configDir = defaultConfigDir()
	}
	if deviceID == "" {
		return mcp.NewToolResultError("no device_id available (call rd_connect first or provide device_id parameter)"), nil
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

func toolReadConfig(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	state.mu.Lock()
	savedConfigDir := state.configDir
	state.mu.Unlock()

	configDir := req.GetString("config_dir", savedConfigDir)
	if configDir == "" {
		configDir = defaultConfigDir()
	}

	configPath := filepath.Join(configDir, "config.yml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("read config from %s: %v", configPath, err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Config from %s:\n\n%s", configPath, string(data))), nil
}

func toolListDevices(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	state.mu.Lock()
	savedConfigDir := state.configDir
	state.mu.Unlock()

	configDir := req.GetString("config_dir", savedConfigDir)
	if configDir == "" {
		configDir = defaultConfigDir()
	}

	devicesDir := filepath.Join(configDir, ".config", "devices")
	entries, err := os.ReadDir(devicesDir)
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("No device data found in %s (%v)", devicesDir, err)), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Devices in %s:\n", devicesDir)
	for _, entry := range entries {
		if entry.IsDir() {
			devPath := filepath.Join(devicesDir, entry.Name(), "device.json")
			if devData, readErr := os.ReadFile(devPath); readErr == nil {
				var geo map[string]any
				if json.Unmarshal(devData, &geo) == nil {
					name, _ := geo["name"].(string)
					source, _ := geo["source"].(string)
					fmt.Fprintf(&sb, "  %s  name=%q source=%s\n", entry.Name()[:8], name, source)
				}
			} else {
				fmt.Fprintf(&sb, "  %s  (no device.json)\n", entry.Name()[:8])
			}
		}
	}
	return mcp.NewToolResultText(sb.String()), nil
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

	hello := wsclient.BuildHelloMsg(deviceID, "Claude MCP Client", 3, 5, 64, []string{"png"})
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
