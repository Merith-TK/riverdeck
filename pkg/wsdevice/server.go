package wsdevice

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ConnectFunc is invoked for each successfully handshaked WebSocket device.
// It runs in its own goroutine and must block until the session is done.
type ConnectFunc func(dev *Device)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Server manages WebSocket connections from software client devices.
// Multiple simultaneous clients are supported; each must provide a stable
// unique ID in the hello message.
type Server struct {
	onConnect  ConnectFunc
	httpServer *http.Server
	brightness int // current brightness sent to clients in the ack

	mu      sync.Mutex
	devices map[string]*Device // id → active device
}

// NewServer creates a WebSocket device server listening on port.
// fn is called once per connection after a successful hello/ack handshake.
func NewServer(port int, fn ConnectFunc) *Server {
	s := &Server{
		onConnect:  fn,
		devices:    make(map[string]*Device),
		brightness: 75, // sensible default
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.serveWS)
	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}
	return s
}

// SetBrightness stores the current brightness level so it can be included in
// the ack handshake for newly connecting clients.  Call this whenever the
// application brightness changes.
func (s *Server) SetBrightness(percent int) {
	s.mu.Lock()
	s.brightness = percent
	s.mu.Unlock()
}

// Start begins listening for WebSocket connections in the background.
// The server shuts down when ctx is cancelled.
func (s *Server) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return fmt.Errorf("wsdevice listen %s: %w", s.httpServer.Addr, err)
	}
	go func() {
		if serveErr := s.httpServer.Serve(ln); serveErr != nil && serveErr != http.ErrServerClosed {
			log.Printf("[wsdevice] serve error: %v", serveErr)
		}
	}()
	go func() {
		<-ctx.Done()
		_ = s.httpServer.Close()
	}()
	return nil
}

// ── hello / ack message types ─────────────────────────────────────────────

type helloMsg struct {
	Type    string      `json:"type"`
	ID      string      `json:"id"`
	Name    string      `json:"name"`
	Rows    int         `json:"rows"`
	Cols    int         `json:"cols"`
	Formats []string    `json:"formats"`
	Inputs  []inputSpec `json:"inputs"`
}

type inputSpec struct {
	ID      string      `json:"id"`
	Type    string      `json:"type"`
	X       *int        `json:"x"`
	Y       *int        `json:"y"`
	Display displaySpec `json:"display"`
}

type displaySpec struct {
	Image       bool     `json:"image"`
	ImageWidth  int      `json:"imageWidth"`
	ImageHeight int      `json:"imageHeight"`
	Text        bool     `json:"text"`
	Formats     []string `json:"formats"`
}

func sendAck(conn *websocket.Conn, status, reason string, brightness int) {
	msg := map[string]any{"type": "ack", "status": status, "brightness": brightness}
	if reason != "" {
		msg["reason"] = reason
	}
	data, _ := json.Marshal(msg)
	_ = conn.WriteMessage(websocket.TextMessage, data)
}

// serveWS upgrades the connection, runs the hello/ack handshake, then drives
// the device session.
func (s *Server) serveWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[wsdevice] upgrade error: %v", err)
		return
	}

	// ── Keepalive: extend read deadline on each pong. ──────────────────────
	conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(15 * time.Second))
		return nil
	})

	// ── Hello handshake (10 s timeout). ───────────────────────────────────
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, raw, err := conn.ReadMessage()
	if err != nil {
		log.Printf("[wsdevice] read hello error: %v", err)
		_ = conn.Close()
		return
	}
	// Restore the keepalive deadline after reading hello.
	conn.SetReadDeadline(time.Now().Add(15 * time.Second))

	var hello helloMsg
	if err := json.Unmarshal(raw, &hello); err != nil {
		sendAck(conn, "error", "invalid hello JSON", 0)
		_ = conn.Close()
		return
	}
	if hello.Type != "hello" {
		sendAck(conn, "error", "expected hello message", 0)
		_ = conn.Close()
		return
	}
	if hello.ID == "" || hello.Name == "" || hello.Rows == 0 || hello.Cols == 0 || len(hello.Inputs) == 0 {
		sendAck(conn, "error", "missing required hello fields: id, name, rows, cols, inputs", 0)
		_ = conn.Close()
		return
	}

	// ── Duplicate detection. ───────────────────────────────────────────────
	s.mu.Lock()
	if _, exists := s.devices[hello.ID]; exists {
		s.mu.Unlock()
		sendAck(conn, "error", "Device already connected", 0)
		log.Printf("[wsdevice] rejected duplicate connection for id=%s addr=%s", hello.ID, conn.RemoteAddr())
		_ = conn.Close()
		return
	}
	dev := newDevice(conn, hello)
	s.devices[hello.ID] = dev
	brightness := s.brightness
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.devices, hello.ID)
		s.mu.Unlock()
	}()

	sendAck(conn, "ok", "", brightness)
	log.Printf("[wsdevice] client connected id=%s name=%q addr=%s inputs=%d",
		hello.ID, hello.Name, conn.RemoteAddr(), len(hello.Inputs))

	// ── Start keepalive ping loop. ─────────────────────────────────────────
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-dev.ctx.Done():
				return
			case <-ticker.C:
				dev.mu.Lock()
				err := conn.WriteMessage(websocket.PingMessage, nil)
				dev.mu.Unlock()
				if err != nil {
					dev.cancel()
					return
				}
			}
		}
	}()

	// ── readLoop populates keyEventsCh and cancels on close. ──────────────
	go dev.readLoop()

	// ── onConnect drives the layout session (blocks until disconnect). ─────
	s.onConnect(dev)
	log.Printf("[wsdevice] client disconnected id=%s", hello.ID)
}
