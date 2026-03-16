package wsdevice

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/merith-tk/riverdeck/pkg/streamdeck"
)

// ConnectFunc is invoked for each new WebSocket device connection.
// It is called in the HTTP server's goroutine for that connection and should
// block until the session is done (i.e. run the device event loop).
type ConnectFunc func(dev *Device)

var upgrader = websocket.Upgrader{
	// Allow all origins so web apps on any domain can connect.
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Server manages WebSocket connections from software client devices.
// Each connection is upgraded to WebSocket, assigned a UUID, and wrapped in a
// Device that implements streamdeck.DeviceIface.
type Server struct {
	model      streamdeck.Model
	onConnect  ConnectFunc
	httpServer *http.Server
}

// NewServer creates a WebSocket device server.
//
//   - port:      TCP port to listen on (e.g. 9000).
//   - model:     Virtual device geometry served to all connected clients.
//   - onConnect: Called once per connection; receives a fully initialised Device.
func NewServer(port int, model streamdeck.Model, fn ConnectFunc) *Server {
	s := &Server{
		model:     model,
		onConnect: fn,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.serveWS)
	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}
	return s
}

// Start begins listening for WebSocket connections in the background.
// The listener shuts down when ctx is cancelled.
// Returns an error only if the TCP listener cannot be bound.
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

// serveWS handles the HTTP -> WebSocket upgrade and drives a Device session.
// It honours an optional ?uuid= query parameter so clients can resume sessions.
func (s *Server) serveWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[wsdevice] upgrade error: %v", err)
		return
	}

	// Use client-supplied UUID for session resumption; otherwise generate a new one.
	id := r.URL.Query().Get("uuid")
	if id == "" {
		id = uuid.New().String()
	}

	dev := newDevice(conn, id, s.model)

	// Send the devinfo handshake immediately so the client knows its identity and
	// the grid geometry before any images arrive.
	if err := dev.sendJSON(map[string]any{
		"type":       "devinfo",
		"uuid":       id,
		"cols":       s.model.Cols,
		"rows":       s.model.Rows,
		"keys":       s.model.Keys,
		"pixel_size": s.model.PixelSize,
		"model_name": s.model.Name,
	}); err != nil {
		log.Printf("[wsdevice] devinfo send error uuid=%s: %v", id, err)
		_ = conn.Close()
		return
	}

	log.Printf("[wsdevice] client connected uuid=%s addr=%s", id, conn.RemoteAddr())

	// readLoop populates keyEventsCh and cancels the device context on close.
	go dev.readLoop()

	// onConnect drives the layout session; it blocks until the session ends.
	s.onConnect(dev)
}
