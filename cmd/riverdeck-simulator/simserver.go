package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

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

// ── Display state ─────────────────────────────────────────────────────────────

type keyState struct {
	imgData []byte // PNG/JPEG bytes (nil = no image set)
	label   string
}

// ── Simulator state ───────────────────────────────────────────────────────────

type SimState struct {
	spec     SimSpec
	deviceID string
	wsAddr   string
	httpPort int

	mu         sync.RWMutex
	keys       []keyState
	brightness int

	sse *sseBroadcaster

	// The active WebSocket connection to riverdeck (nil when disconnected).
	wsMu sync.Mutex
	conn *websocket.Conn
}

func newSimState(spec SimSpec, deviceID, wsAddr string, httpPort int) *SimState {
	s := &SimState{
		spec:       spec,
		deviceID:   deviceID,
		wsAddr:     wsAddr,
		httpPort:   httpPort,
		keys:       make([]keyState, spec.Keys),
		brightness: 100,
		sse:        newSSEBroadcaster(),
	}
	return s
}

// ── WebSocket client (riverdeck side) ─────────────────────────────────────────

// connectAndRun connects to the riverdeck WS server and blocks until disconnected.
// It performs the hello/ack handshake then drives the receive loop.
// Reconnects automatically every 5 seconds on failure.
func (s *SimState) connectAndRun() {
	for {
		if err := s.runSession(); err != nil {
			log.Printf("WS session ended: %v — retrying in 5s", err)
		}
		time.Sleep(5 * time.Second)
	}
}

func (s *SimState) runSession() error {
	log.Printf("Connecting to riverdeck at %s ...", s.wsAddr)
	conn, _, err := websocket.DefaultDialer.Dial(s.wsAddr, nil)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	// Send hello.
	hello := s.buildHello()
	data, _ := json.Marshal(hello)
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return fmt.Errorf("send hello: %w", err)
	}

	// Wait for ack.
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, raw, err := conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("read ack: %w", err)
	}
	conn.SetReadDeadline(time.Time{}) // clear deadline

	var ack struct {
		Type       string `json:"type"`
		Status     string `json:"status"`
		Reason     string `json:"reason"`
		Brightness int    `json:"brightness"`
	}
	if err := json.Unmarshal(raw, &ack); err != nil || ack.Type != "ack" {
		return fmt.Errorf("expected ack, got: %s", string(raw))
	}
	if ack.Status != "ok" {
		return fmt.Errorf("ack error: %s", ack.Reason)
	}

	log.Printf("Connected to riverdeck  (device=%s, brightness=%d%%)", s.deviceID, ack.Brightness)
	s.wsMu.Lock()
	s.conn = conn
	s.wsMu.Unlock()

	s.cmdSetBrightness(ack.Brightness)
	s.sse.send("connected", `{}`)

	defer func() {
		s.wsMu.Lock()
		if s.conn == conn {
			s.conn = nil
		}
		s.wsMu.Unlock()
		s.sse.send("disconnected", `{}`)
	}()

	// Keepalive: pings arrive as control frames and don't return from
	// ReadMessage, so we must reset the deadline in the PingHandler.
	// The handler also sends the required pong reply.
	conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	conn.SetPingHandler(func(data string) error {
		conn.SetReadDeadline(time.Now().Add(15 * time.Second))
		return conn.WriteControl(websocket.PongMessage, []byte(data), time.Now().Add(5*time.Second))
	})

	// Receive loop.
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}
		conn.SetReadDeadline(time.Now().Add(15 * time.Second))
		s.handleServerMessage(raw)
	}
}

// buildHello constructs the hello message declaring our device identity.
func (s *SimState) buildHello() map[string]any {
	inputs := make([]map[string]any, s.spec.Keys)
	for i := 0; i < s.spec.Keys; i++ {
		col := i % s.spec.Cols
		row := i / s.spec.Cols
		inputs[i] = map[string]any{
			"id":   fmt.Sprintf("btn%d", i),
			"type": "button",
			"x":    col,
			"y":    row,
			"display": map[string]any{
				"image":       true,
				"imageWidth":  s.spec.PixelSize,
				"imageHeight": s.spec.PixelSize,
				"text":        true,
				"formats":     []string{"png", "jpeg"},
			},
		}
	}
	return map[string]any{
		"type":    "hello",
		"id":      s.deviceID,
		"name":    s.spec.ModelName,
		"rows":    s.spec.Rows,
		"cols":    s.spec.Cols,
		"formats": []string{"png", "jpeg"},
		"inputs":  inputs,
	}
}

// handleServerMessage dispatches a JSON message from riverdeck.
func (s *SimState) handleServerMessage(raw []byte) {
	var msg struct {
		Type   string `json:"type"`
		ID     string `json:"id"`
		Width  int    `json:"width"`
		Height int    `json:"height"`
		Data   string `json:"data"`
		Text   string `json:"text"`
		Value  int    `json:"value"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		return
	}

	switch msg.Type {
	case "frame":
		keyIndex := inputIDToIndex(msg.ID)
		if keyIndex < 0 {
			log.Printf("frame: unrecognised id=%s", msg.ID)
			return
		}
		imgBytes, err := base64.StdEncoding.DecodeString(msg.Data)
		if err != nil {
			log.Printf("frame: base64 decode error key=%d: %v", keyIndex, err)
			return
		}
		s.mu.Lock()
		if keyIndex < len(s.keys) {
			s.keys[keyIndex].imgData = imgBytes
		}
		s.mu.Unlock()
		payload, _ := json.Marshal(map[string]any{"key": keyIndex, "data": msg.Data})
		s.sse.send("setimage", string(payload))

	case "label":
		keyIndex := inputIDToIndex(msg.ID)
		if keyIndex < 0 {
			return
		}
		s.mu.Lock()
		if keyIndex < len(s.keys) {
			s.keys[keyIndex].label = msg.Text
		}
		s.mu.Unlock()
		payload, _ := json.Marshal(map[string]any{"key": keyIndex, "text": msg.Text})
		s.sse.send("setlabel", string(payload))

	case "setbrightness":
		s.cmdSetBrightness(msg.Value)

	case "clear":
		s.cmdClear()

	case "layoutChange":
		s.sse.send("layoutChange", string(raw))
	}
}

func (s *SimState) cmdSetBrightness(value int) {
	s.mu.Lock()
	s.brightness = value
	s.mu.Unlock()
	payload, _ := json.Marshal(map[string]any{"value": value})
	s.sse.send("setbrightness", string(payload))
}

func (s *SimState) cmdClear() {
	s.mu.Lock()
	for i := range s.keys {
		s.keys[i].imgData = nil
		s.keys[i].label = ""
	}
	s.mu.Unlock()
	s.sse.send("clear", `{}`)
}

// inputIDToIndex maps "btnN" → N.  Returns -1 if not parseable.
func inputIDToIndex(id string) int {
	var n int
	if _, err := fmt.Sscanf(id, "btn%d", &n); err != nil {
		return -1
	}
	return n
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
		"WsAddr":     s.wsAddr,
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

	s.sendSnapshot(w, flusher)

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			if _, err := w.Write(msg); err != nil {
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

	bp, _ := json.Marshal(map[string]any{"value": brightness})
	fmt.Fprintf(w, "event: setbrightness\ndata: %s\n\n", bp)

	for i, k := range keys {
		if k.imgData != nil {
			payload, _ := json.Marshal(map[string]any{
				"key":  i,
				"data": base64.StdEncoding.EncodeToString(k.imgData),
			})
			fmt.Fprintf(w, "event: setimage\ndata: %s\n\n", payload)
		}
		if k.label != "" {
			payload, _ := json.Marshal(map[string]any{"key": i, "text": k.label})
			fmt.Fprintf(w, "event: setlabel\ndata: %s\n\n", payload)
		}
	}

	s.wsMu.Lock()
	connected := s.conn != nil
	s.wsMu.Unlock()
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

	event := "release"
	if body.Pressed {
		event = "press"
	}
	msg, _ := json.Marshal(map[string]any{
		"type":  "input",
		"id":    fmt.Sprintf("btn%d", body.Key),
		"event": event,
	})

	s.wsMu.Lock()
	if s.conn != nil {
		if err := s.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			log.Printf("write input event to riverdeck: %v", err)
		}
	}
	s.wsMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintln(w, `{"ok":true}`)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

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
		return fmt.Errorf("probe JSON is missing required fields: %s", fmt.Sprintf("%v", missing))
	}
	if spec.ModelName == "" {
		spec.ModelName = fmt.Sprintf("Unknown (PID 0x%04X)", spec.ProductID)
	}
	return nil
}
