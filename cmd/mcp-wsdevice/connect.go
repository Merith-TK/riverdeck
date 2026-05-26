package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gorilla/websocket"
	"github.com/merith-tk/riverdeck/pkg/wsclient"
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
	// Respond to server pings to keep the connection alive.
	conn.SetPingHandler(func(data string) error {
		return conn.WriteControl(websocket.PongMessage, []byte(data), time.Now().Add(5*time.Second))
	})
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
				idx := wsclient.InputIDToIndex(msg.ID)
				if idx >= 0 {
					state.keyUpdates[idx] = time.Now()
					state.frameData[idx] = msg.Data
					state.logMsg(fmt.Sprintf("frame: id=%s key=%d (%dx%d) data_len=%d", msg.ID, idx, msg.Width, msg.Height, len(msg.Data)))
				}
			case "label":
				idx := wsclient.InputIDToIndex(msg.ID)
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

func defaultConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".riverdeck"
	}
	return filepath.Join(home, ".riverdeck")
}
