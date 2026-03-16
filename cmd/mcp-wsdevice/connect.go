package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// ── Protocol types ────────────────────────────────────────────────────────

// wsMsg is the union of all server -> client message types.
type wsMsg struct {
	Type       string `json:"type"`
	Status     string `json:"status"`
	Reason     string `json:"reason"`
	Brightness int    `json:"brightness"`
	ID         string `json:"id"`
	Text       string `json:"text"`
	Data       string `json:"data"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	Value      int    `json:"value"`
}

// helloInput describes a single input in the hello message.
type helloInput struct {
	ID      string       `json:"id"`
	Type    string       `json:"type"`
	X       int          `json:"x"`
	Y       int          `json:"y"`
	Display helloDisplay `json:"display"`
}

// helloDisplay describes the display capabilities of an input.
type helloDisplay struct {
	Image       bool     `json:"image"`
	ImageWidth  int      `json:"imageWidth"`
	ImageHeight int      `json:"imageHeight"`
	Text        bool     `json:"text"`
	Formats     []string `json:"formats"`
}

// helloMsg is the opening handshake sent by this client on connect.
type helloMsg struct {
	Type    string       `json:"type"`
	ID      string       `json:"id"`
	Name    string       `json:"name"`
	Rows    int          `json:"rows"`
	Cols    int          `json:"cols"`
	Formats []string     `json:"formats"`
	Inputs  []helloInput `json:"inputs"`
}

// ── Hello message builder ─────────────────────────────────────────────────

// buildHelloMsg constructs a 5x3 grid hello (15 buttons btn0-btn14).
func buildHelloMsg(deviceID string) helloMsg {
	const (
		rows      = 3
		cols      = 5
		pixelSize = 72
	)
	inputs := make([]helloInput, rows*cols)
	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			idx := row*cols + col
			inputs[idx] = helloInput{
				ID:   fmt.Sprintf("btn%d", idx),
				Type: "button",
				X:    col,
				Y:    row,
				Display: helloDisplay{
					Image:       true,
					ImageWidth:  pixelSize,
					ImageHeight: pixelSize,
					Text:        true,
					Formats:     []string{"png"},
				},
			}
		}
	}
	return helloMsg{
		Type:    "hello",
		ID:      deviceID,
		Name:    "Claude MCP Client",
		Rows:    rows,
		Cols:    cols,
		Formats: []string{"png"},
		Inputs:  inputs,
	}
}

// inputIDToIndex converts "btnN" -> N.  Returns -1 if not parseable.
func inputIDToIndex(id string) int {
	if !strings.HasPrefix(id, "btn") {
		return -1
	}
	n, err := strconv.Atoi(strings.TrimPrefix(id, "btn"))
	if err != nil {
		return -1
	}
	return n
}

// ── WebSocket I/O ─────────────────────────────────────────────────────────

// sendJSON encodes v as JSON and sends it over the current connection.
func sendJSON(v any) error {
	state.mu.Lock()
	conn := state.conn
	state.mu.Unlock()
	if conn == nil {
		return fmt.Errorf("not connected")
	}
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	return conn.WriteMessage(websocket.TextMessage, data)
}

// startReadLoop dispatches incoming server messages into state.
func startReadLoop(conn *websocket.Conn) {
	go func() {
		for {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				state.mu.Lock()
				if state.conn == conn {
					state.connected = false
					state.conn = nil
					state.logMsg("disconnected: " + err.Error())
				}
				state.mu.Unlock()
				return
			}

			var msg wsMsg
			_ = json.Unmarshal(raw, &msg)

			state.mu.Lock()
			switch msg.Type {
			case "ack":
				if msg.Status == "ok" {
					state.brightness = msg.Brightness
				}
				state.logMsg(fmt.Sprintf("ack: status=%s brightness=%d reason=%s", msg.Status, msg.Brightness, msg.Reason))
			case "setbrightness":
				state.brightness = msg.Value
				state.logMsg(fmt.Sprintf("setbrightness: %d", msg.Value))
			case "frame":
				idx := inputIDToIndex(msg.ID)
				if idx >= 0 {
					state.keyUpdates[idx] = time.Now()
					state.logMsg(fmt.Sprintf("frame: id=%s key=%d (%dx%d)", msg.ID, idx, msg.Width, msg.Height))
				}
			case "label":
				idx := inputIDToIndex(msg.ID)
				if idx >= 0 {
					state.labels[idx] = msg.Text
					state.logMsg(fmt.Sprintf("label: id=%s key=%d text=%q", msg.ID, idx, msg.Text))
				}
			case "layoutChange":
				state.logMsg(fmt.Sprintf("layoutChange: %s", string(raw)))
			default:
				state.logMsg(fmt.Sprintf("unknown: %s", string(raw)))
			}
			state.mu.Unlock()
		}
	}()
}

// ── Device ID persistence ─────────────────────────────────────────────────

const deviceIDFileName = ".mcp-device-id"

// loadOrCreateDeviceID reads the stored device ID or generates and saves a new one.
func loadOrCreateDeviceID(configDir string) string {
	path := filepath.Join(configDir, deviceIDFileName)
	if data, err := os.ReadFile(path); err == nil {
		if id := strings.TrimSpace(string(data)); id != "" {
			return id
		}
	}
	id := uuid.New().String()
	_ = os.WriteFile(path, []byte(id+"\n"), 0644)
	return id
}

func defaultConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".riverdeck"
	}
	return filepath.Join(home, ".riverdeck")
}
