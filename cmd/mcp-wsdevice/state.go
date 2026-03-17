package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// deviceState holds everything the MCP server knows about the current WS session.
type deviceState struct {
	mu        sync.Mutex
	conn      *websocket.Conn
	connected bool

	// Persistent device ID sent in the hello message (stored in .mcp-device-id).
	deviceID string

	// Where to look for layout files.
	configDir string

	// Input list declared in the hello we sent; used for press_key -> input ID mapping.
	inputIDs []string // indexed by key slot

	// Brightness received from ack or setbrightness messages.
	brightness int

	// Current label text per key index (received via label messages).
	labels map[int]string

	// Per-key last-update timestamps (populated when frame arrives).
	keyUpdates map[int]time.Time

	// Per-key raw base64 image data from the most recent frame message.
	frameData map[int]string

	// Human-readable log of the last 30 events.
	messages []string
}

var state = &deviceState{
	keyUpdates: make(map[int]time.Time),
	labels:     make(map[int]string),
	frameData:  make(map[int]string),
}

func (s *deviceState) logMsg(msg string) {
	const maxLog = 30
	s.messages = append(s.messages, fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05.000"), msg))
	if len(s.messages) > maxLog {
		s.messages = s.messages[len(s.messages)-maxLog:]
	}
}
