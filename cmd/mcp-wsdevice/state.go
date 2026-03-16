package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// deviceState holds everything the MCP server knows about the current WS session.
type deviceState struct {
	mu        sync.Mutex
	conn      *websocket.Conn
	connected bool

	// Assigned by the server on connect.
	uuid string

	// Where to look for layout files (configDir/devices/{uuid}/layout.json).
	configDir string

	// Device geometry from the devinfo handshake.
	cols      int
	rows      int
	keys      int
	pixelSize int
	modelName string

	// Human-readable log of the last 30 events (image data stripped).
	messages []string

	// Per-key last-update timestamps (populated when setimage arrives).
	keyUpdates map[int]time.Time
}

var state = &deviceState{
	keyUpdates: make(map[int]time.Time),
}

func (s *deviceState) logMsg(msg string) {
	const maxLog = 30
	s.messages = append(s.messages, fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05.000"), msg))
	if len(s.messages) > maxLog {
		s.messages = s.messages[len(s.messages)-maxLog:]
	}
}

type wsMsg struct {
	Type      string `json:"type"`
	UUID      string `json:"uuid"`
	Cols      int    `json:"cols"`
	Rows      int    `json:"rows"`
	Keys      int    `json:"keys"`
	PixelSize int    `json:"pixel_size"`
	ModelName string `json:"model_name"`
	Key       int    `json:"key"`
	Value     int    `json:"value"`
	R, G, B   int
}

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
			case "devinfo":
				state.uuid = msg.UUID
				state.cols = msg.Cols
				state.rows = msg.Rows
				state.keys = msg.Keys
				state.pixelSize = msg.PixelSize
				state.modelName = msg.ModelName
				state.logMsg(fmt.Sprintf("devinfo: uuid=%s model=%s %dx%d (%d keys, %dpx)",
					msg.UUID, msg.ModelName, msg.Cols, msg.Rows, msg.Keys, msg.PixelSize))
			case "setimage":
				state.keyUpdates[msg.Key] = time.Now()
				state.logMsg(fmt.Sprintf("setimage: key=%d (image data received)", msg.Key))
			case "setkeycolor":
				var full struct {
					Key int `json:"key"`
					R   int `json:"r"`
					G   int `json:"g"`
					B   int `json:"b"`
				}
				_ = json.Unmarshal(raw, &full)
				state.keyUpdates[full.Key] = time.Now()
				state.logMsg(fmt.Sprintf("setkeycolor: key=%d rgb(%d,%d,%d)", full.Key, full.R, full.G, full.B))
			case "setbrightness":
				state.logMsg(fmt.Sprintf("setbrightness: %d%%", msg.Value))
			case "clear":
				state.logMsg("clear")
			case "reset":
				state.logMsg("reset")
			default:
				state.logMsg(fmt.Sprintf("unknown: %s", string(raw)))
			}
			state.mu.Unlock()
		}
	}()
}

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

func defaultConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".riverdeck"
	}
	return filepath.Join(home, ".riverdeck")
}

const uuidFileName = ".mcp-client-uuid"

// loadStoredUUID reads the persisted UUID from configDir/.mcp-client-uuid.
// Returns "" if the file does not exist or cannot be read.
func loadStoredUUID(configDir string) string {
	data, err := os.ReadFile(filepath.Join(configDir, uuidFileName))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// storeUUID writes uuid to configDir/.mcp-client-uuid so it survives restarts.
func storeUUID(configDir, uuid string) {
	_ = os.WriteFile(filepath.Join(configDir, uuidFileName), []byte(uuid+"\n"), 0644)
}
