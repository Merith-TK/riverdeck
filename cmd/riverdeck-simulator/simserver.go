package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"image/color"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
)

// ── Probe JSON loading ────────────────────────────────────────────────────────

// SimSpec holds the subset of a ProbeResult needed to run the simulator.
// It matches the JSON field names produced by riverdeck-debug-prober.
type SimSpec struct {
	ModelName    string `json:"model_name"`
	VendorID     uint16 `json:"vendor_id"`
	ProductID    uint16 `json:"product_id"`
	Cols         int    `json:"cols"`
	Rows         int    `json:"rows"`
	Keys         int    `json:"keys"`
	PixelSize    int    `json:"pixel_size"`
	ImageFormat  string `json:"image_format"`
	Manufacturer string `json:"manufacturer"`
	Product      string `json:"product"`
	Serial       string `json:"serial"`
	Firmware     string `json:"firmware"`
}

// ── Display state ─────────────────────────────────────────────────────────────

type keyState struct {
	imgData    []byte      // PNG bytes (nil = no image set)
	solidColor *color.RGBA // solid colour (nil = not set)
}

// ── SSE broadcaster ───────────────────────────────────────────────────────────

type sseBroadcaster struct {
	mu      sync.Mutex
	clients map[chan []byte]struct{}
}

func newSSEBroadcaster() *sseBroadcaster {
	return &sseBroadcaster{clients: make(map[chan []byte]struct{})}
}

func (b *sseBroadcaster) subscribe() (ch chan []byte, unsub func()) {
	ch = make(chan []byte, 128)
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.clients, ch)
		b.mu.Unlock()
		close(ch)
	}
}

func (b *sseBroadcaster) send(eventName, jsonData string) {
	msg := []byte("event: " + eventName + "\ndata: " + jsonData + "\n\n")
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.clients {
		select {
		case ch <- msg:
		default: // drop if browser is slow
		}
	}
}

// ── Simulator state ───────────────────────────────────────────────────────────

type SimState struct {
	spec     SimSpec
	tcpPort  int
	httpPort int

	mu         sync.RWMutex
	keys       []keyState
	brightness int

	sse *sseBroadcaster

	// The active riverdeck TCP connection (nil when disconnected).
	riverdeckMu   sync.Mutex
	riverdeckConn net.Conn
}

func newSimState(spec SimSpec, tcpPort, httpPort int) *SimState {
	s := &SimState{
		spec:       spec,
		tcpPort:    tcpPort,
		httpPort:   httpPort,
		keys:       make([]keyState, spec.Keys),
		brightness: 100,
		sse:        newSSEBroadcaster(),
	}
	return s
}

// ── TCP server (riverdeck side) ───────────────────────────────────────────────

// startTCPServer listens for riverdeck connections on tcpPort.
// Only one connection is active at a time; a new connection replaces the old one.
func (s *SimState) startTCPServer() {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", s.tcpPort))
	if err != nil {
		log.Fatalf("TCP listen :%d: %v", s.tcpPort, err)
	}
	log.Printf("TCP server listening on :%d  (riverdeck connects here)", s.tcpPort)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("TCP accept error: %v", err)
			continue
		}
		// Close any existing connection.
		s.riverdeckMu.Lock()
		if s.riverdeckConn != nil {
			s.riverdeckConn.Close()
		}
		s.riverdeckConn = conn
		s.riverdeckMu.Unlock()

		log.Printf("riverdeck connected from %s", conn.RemoteAddr())
		s.sse.send("connected", `{}`)
		go s.handleRiverdeckConn(conn)
	}
}

// devInfoJSON returns the devinfo JSON line sent to riverdeck on connect.
func (s *SimState) devInfoJSON() string {
	type devInfoMsg struct {
		Type         string `json:"type"`
		ModelName    string `json:"model_name"`
		VendorID     uint16 `json:"vendor_id"`
		ProductID    uint16 `json:"product_id"`
		Cols         int    `json:"cols"`
		Rows         int    `json:"rows"`
		Keys         int    `json:"keys"`
		PixelSize    int    `json:"pixel_size"`
		ImageFormat  string `json:"image_format"`
		Manufacturer string `json:"manufacturer"`
		Product      string `json:"product"`
		Serial       string `json:"serial"`
		Firmware     string `json:"firmware"`
	}
	msg := devInfoMsg{
		Type:         "devinfo",
		ModelName:    s.spec.ModelName,
		VendorID:     s.spec.VendorID,
		ProductID:    s.spec.ProductID,
		Cols:         s.spec.Cols,
		Rows:         s.spec.Rows,
		Keys:         s.spec.Keys,
		PixelSize:    s.spec.PixelSize,
		ImageFormat:  s.spec.ImageFormat,
		Manufacturer: s.spec.Manufacturer,
		Product:      s.spec.Product,
		Serial:       s.spec.Serial,
		Firmware:     s.spec.Firmware,
	}
	b, _ := json.Marshal(msg)
	return string(b)
}

// handleRiverdeckConn manages a single riverdeck ↔ simulator TCP session.
func (s *SimState) handleRiverdeckConn(conn net.Conn) {
	defer func() {
		conn.Close()
		s.riverdeckMu.Lock()
		if s.riverdeckConn == conn {
			s.riverdeckConn = nil
		}
		s.riverdeckMu.Unlock()
		log.Printf("riverdeck disconnected")
		s.sse.send("disconnected", `{}`)
	}()

	// Send device info immediately.
	fmt.Fprintln(conn, s.devInfoJSON())

	// Read commands from riverdeck.
	scanner := bufio.NewScanner(conn)
	// Increase scanner buffer to handle large base64-encoded images.
	buf := make([]byte, 4*1024*1024)
	scanner.Buffer(buf, len(buf))

	for scanner.Scan() {
		line := scanner.Bytes()
		if err := s.handleCommand(line); err != nil {
			log.Printf("command error: %v", err)
		}
	}
}

// ── Command dispatch ──────────────────────────────────────────────────────────

type inboundCmd struct {
	Type  string `json:"type"`
	Key   int    `json:"key"`
	Value int    `json:"value"`
	Data  string `json:"data"` // base64 PNG for setimage
	R     int    `json:"r"`
	G     int    `json:"g"`
	B     int    `json:"b"`
}

func (s *SimState) handleCommand(raw []byte) error {
	var cmd inboundCmd
	if err := json.Unmarshal(raw, &cmd); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}

	switch cmd.Type {
	case "setimage":
		return s.cmdSetImage(cmd.Key, cmd.Data)
	case "setkeycolor":
		return s.cmdSetKeyColor(cmd.Key, cmd.R, cmd.G, cmd.B)
	case "setbrightness":
		return s.cmdSetBrightness(cmd.Value)
	case "clear":
		return s.cmdClear()
	case "reset":
		return s.cmdClear() // treat reset same as clear for display
	default:
		// Unknown command -- ignore silently.
	}
	return nil
}

func (s *SimState) cmdSetImage(keyIndex int, b64data string) error {
	imgBytes, err := base64.StdEncoding.DecodeString(b64data)
	if err != nil {
		return fmt.Errorf("base64 decode: %w", err)
	}

	s.mu.Lock()
	if keyIndex >= 0 && keyIndex < len(s.keys) {
		s.keys[keyIndex].imgData = imgBytes
		s.keys[keyIndex].solidColor = nil
	}
	s.mu.Unlock()

	// Broadcast to browser via SSE (keep the base64 data as-is for the <img> src).
	payload, _ := json.Marshal(map[string]any{
		"key":  keyIndex,
		"data": b64data,
	})
	s.sse.send("setimage", string(payload))
	return nil
}

func (s *SimState) cmdSetKeyColor(keyIndex, r, g, b int) error {
	c := &color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255}

	s.mu.Lock()
	if keyIndex >= 0 && keyIndex < len(s.keys) {
		s.keys[keyIndex].imgData = nil
		s.keys[keyIndex].solidColor = c
	}
	s.mu.Unlock()

	payload, _ := json.Marshal(map[string]any{
		"key": keyIndex,
		"r":   r, "g": g, "b": b,
	})
	s.sse.send("setkeycolor", string(payload))
	return nil
}

func (s *SimState) cmdSetBrightness(value int) error {
	s.mu.Lock()
	s.brightness = value
	s.mu.Unlock()

	payload, _ := json.Marshal(map[string]any{"value": value})
	s.sse.send("setbrightness", string(payload))
	return nil
}

func (s *SimState) cmdClear() error {
	s.mu.Lock()
	for i := range s.keys {
		s.keys[i].imgData = nil
		s.keys[i].solidColor = nil
	}
	s.mu.Unlock()

	s.sse.send("clear", `{}`)
	return nil
}

// ── HTTP + SSE server (browser side) ─────────────────────────────────────────

func (s *SimState) startHTTPServer() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/events", s.handleSSE)
	mux.HandleFunc("/keyevent", s.handleKeyEvent)

	addr := fmt.Sprintf(":%d", s.httpPort)
	log.Printf("HTTP server listening on %s  (open in browser)", addr)

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("HTTP server: %v", err)
	}
}

func (s *SimState) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// Build sequential key indices for the template.
	keySize := 100
	gap := 8

	indices := make([]int, s.spec.Keys)
	for i := range indices {
		indices[i] = i
	}

	tmpl, err := template.New("sim").Parse(webpageTemplate)
	if err != nil {
		http.Error(w, "template error: "+err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = tmpl.Execute(w, map[string]any{
		"ModelName":  s.spec.ModelName,
		"Cols":       s.spec.Cols,
		"Keys":       s.spec.Keys,
		"KeySize":    keySize,
		"Gap":        gap,
		"WsPort":     s.tcpPort,
		"KeyIndices": indices,
	})
}

func (s *SimState) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch, unsub := s.sse.subscribe()
	defer unsub()

	// Send a snapshot of the current display state to the new browser client.
	s.sendSnapshot(w, flusher)

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			_, err := w.Write(msg)
			if err != nil {
				return
			}
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// sendSnapshot pushes the current display state to a freshly connected browser.
func (s *SimState) sendSnapshot(w http.ResponseWriter, flusher http.Flusher) {
	s.mu.RLock()
	keys := make([]keyState, len(s.keys))
	copy(keys, s.keys)
	brightness := s.brightness
	s.mu.RUnlock()

	// Brightness
	bp, _ := json.Marshal(map[string]any{"value": brightness})
	fmt.Fprintf(w, "event: setbrightness\ndata: %s\n\n", bp)

	// Key images / colours
	for i, k := range keys {
		if k.imgData != nil {
			payload, _ := json.Marshal(map[string]any{
				"key":  i,
				"data": base64.StdEncoding.EncodeToString(k.imgData),
			})
			fmt.Fprintf(w, "event: setimage\ndata: %s\n\n", payload)
		} else if k.solidColor != nil {
			c := k.solidColor
			payload, _ := json.Marshal(map[string]any{
				"key": i, "r": int(c.R), "g": int(c.G), "b": int(c.B),
			})
			fmt.Fprintf(w, "event: setkeycolor\ndata: %s\n\n", payload)
		}
	}

	// Connection state
	s.riverdeckMu.Lock()
	connected := s.riverdeckConn != nil
	s.riverdeckMu.Unlock()
	if connected {
		fmt.Fprintf(w, "event: connected\ndata: {}\n\n")
	}

	flusher.Flush()
}

func (s *SimState) handleKeyEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Key     int  `json:"key"`
		Pressed bool `json:"pressed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad JSON", http.StatusBadRequest)
		return
	}

	// Forward key event to the connected riverdeck over TCP.
	msg, _ := json.Marshal(map[string]any{
		"type":    "keyevent",
		"key":     body.Key,
		"pressed": body.Pressed,
	})

	s.riverdeckMu.Lock()
	conn := s.riverdeckConn
	s.riverdeckMu.Unlock()

	if conn != nil {
		line := append(msg, '\n')
		if _, err := conn.Write(line); err != nil {
			log.Printf("write keyevent to riverdeck: %v", err)
			// Connection likely broken -- it will be cleaned up by the reader.
		}
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintln(w, `{"ok":true}`)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// probeJSONError is returned when the probe JSON doesn't contain usable data.
func validateSpec(spec SimSpec) error {
	var missing []string
	if spec.Cols == 0 {
		missing = append(missing, "cols")
	}
	if spec.Rows == 0 {
		missing = append(missing, "rows")
	}
	if spec.Keys == 0 {
		missing = append(missing, "keys")
	}
	if len(missing) > 0 {
		return fmt.Errorf("probe JSON is missing required fields: %s", strings.Join(missing, ", "))
	}
	if spec.ModelName == "" {
		spec.ModelName = fmt.Sprintf("Unknown (PID 0x%04X)", spec.ProductID)
	}
	return nil
}
