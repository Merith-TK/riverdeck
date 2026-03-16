package streamdeck

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net"
	"sync"
	"time"

	xdraw "golang.org/x/image/draw"
)

// SimClient connects to a running riverdeck-simulator process and implements
// DeviceIface so the application can use it transparently in place of real
// hardware.  Communication uses newline-delimited JSON over a plain TCP socket.
type SimClient struct {
	conn net.Conn
	mu   sync.Mutex // serialises writes to conn

	// Device info received from the simulator on connect.
	model Model
	info  DeviceInfo

	// Buffered channel for key events arriving from the simulator.
	keyEventsCh chan KeyEvent

	ctx    context.Context
	cancel context.CancelFunc
}

// ConnectSim dials the simulator at addr ("host:port"), waits for the devinfo
// handshake, and returns a fully initialised SimClient.
func ConnectSim(addr string) (*SimClient, error) {
	conn, err := net.DialTimeout("tcp", addr, 15*time.Second)
	if err != nil {
		return nil, fmt.Errorf("dial simulator %s: %w", addr, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	sc := &SimClient{
		conn:        conn,
		keyEventsCh: make(chan KeyEvent, 64),
		ctx:         ctx,
		cancel:      cancel,
	}

	// First line from the simulator MUST be the devinfo message.
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		conn.Close()
		cancel()
		return nil, fmt.Errorf("simulator closed before sending devinfo")
	}
	if err := sc.parseInbound(scanner.Bytes()); err != nil {
		conn.Close()
		cancel()
		return nil, fmt.Errorf("parse devinfo: %w", err)
	}

	// Kick off the background reader that forwards key events.
	go sc.readLoop(scanner)

	return sc, nil
}

// readLoop reads newline-delimited JSON from the simulator until the connection
// closes or the context is cancelled.
func (sc *SimClient) readLoop(scanner *bufio.Scanner) {
	defer sc.cancel()
	for scanner.Scan() {
		select {
		case <-sc.ctx.Done():
			return
		default:
		}
		_ = sc.parseInbound(scanner.Bytes())
	}
}

// simInbound is the union of all message types received from the simulator.
type simInbound struct {
	Type string `json:"type"`

	// devinfo
	ModelNameF   string `json:"model_name"`
	VendorID     uint16 `json:"vendor_id"`
	ProductID    uint16 `json:"product_id"`
	ColsF        int    `json:"cols"`
	RowsF        int    `json:"rows"`
	KeysF        int    `json:"keys"`
	PixelSizeF   int    `json:"pixel_size"`
	ImageFormat  string `json:"image_format"`
	Manufacturer string `json:"manufacturer"`
	Product      string `json:"product"`
	Serial       string `json:"serial"`
	Firmware     string `json:"firmware"`

	// keyevent
	Key     int  `json:"key"`
	Pressed bool `json:"pressed"`
}

func (sc *SimClient) parseInbound(data []byte) error {
	var msg simInbound
	if err := json.Unmarshal(data, &msg); err != nil {
		return err
	}
	switch msg.Type {
	case "devinfo":
		sc.model = Model{
			Name:        msg.ModelNameF,
			ProductID:   msg.ProductID,
			Cols:        msg.ColsF,
			Rows:        msg.RowsF,
			Keys:        msg.KeysF,
			PixelSize:   msg.PixelSizeF,
			ImageFormat: msg.ImageFormat,
		}
		sc.info = DeviceInfo{
			Path:         "sim://" + sc.conn.RemoteAddr().String(),
			Serial:       msg.Serial,
			Manufacturer: msg.Manufacturer,
			Product:      msg.Product,
			Model:        sc.model,
			Firmware:     msg.Firmware,
		}
	case "keyevent":
		select {
		case sc.keyEventsCh <- KeyEvent{Key: msg.Key, Pressed: msg.Pressed}:
		default: // drop if buffer full -- should be rare
		}
	}
	return nil
}

// sendJSON encodes v as JSON and appends a newline delimiter before writing.
func (sc *SimClient) sendJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	sc.mu.Lock()
	defer sc.mu.Unlock()
	_, err = sc.conn.Write(data)
	return err
}

// ── DeviceIface ────────────────────────────────────────────────────────────

func (sc *SimClient) Close() error {
	sc.cancel()
	return sc.conn.Close()
}

func (sc *SimClient) SetBrightness(percent int) error {
	return sc.sendJSON(map[string]any{"type": "setbrightness", "value": percent})
}

func (sc *SimClient) Clear() error {
	return sc.sendJSON(map[string]any{"type": "clear"})
}

func (sc *SimClient) Reset() error {
	return sc.sendJSON(map[string]any{"type": "reset"})
}

func (sc *SimClient) SetImage(keyIndex int, img image.Image) error {
	data, err := sc.EncodeKeyImage(keyIndex, img)
	if err != nil {
		return err
	}
	return sc.WriteKeyData(keyIndex, data)
}

func (sc *SimClient) SetKeyColor(keyIndex int, c color.Color) error {
	r, g, b, _ := c.RGBA()
	return sc.sendJSON(map[string]any{
		"type": "setkeycolor",
		"key":  keyIndex,
		"r":    int(r >> 8),
		"g":    int(g >> 8),
		"b":    int(b >> 8),
	})
}

// EncodeKeyImage resizes img to the device pixel size and encodes it as PNG.
// Unlike the real Device, no 180° rotation is applied -- the simulator shows
// images exactly as the script author intended.
// keyIndex is accepted for interface compatibility but is not used by SimClient.
func (sc *SimClient) EncodeKeyImage(_ int, img image.Image) ([]byte, error) {
	size := sc.pixelSizeOrDefault()
	resized := sc.resizeTo(img, size)
	var buf bytes.Buffer
	if err := png.Encode(&buf, resized); err != nil {
		return nil, fmt.Errorf("png encode: %w", err)
	}
	return buf.Bytes(), nil
}

// SetLabel is a no-op for SimClient; labels are baked into the key image.
func (sc *SimClient) SetLabel(_ int, _ string) error { return nil }

// WriteKeyData sends pre-encoded image bytes (PNG from EncodeKeyImage) to the
// simulator as a base64-encoded setimage message.
func (sc *SimClient) WriteKeyData(keyIndex int, imageData []byte) error {
	return sc.sendJSON(map[string]any{
		"type": "setimage",
		"key":  keyIndex,
		"data": base64.StdEncoding.EncodeToString(imageData),
	})
}

// ResizeImage scales src to the key pixel size without rotation.
func (sc *SimClient) ResizeImage(src image.Image) image.Image {
	return sc.resizeTo(src, sc.pixelSizeOrDefault())
}

func (sc *SimClient) resizeTo(src image.Image, size int) image.Image {
	b := src.Bounds()
	if b.Dx() == size && b.Dy() == size {
		return src
	}
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	xdraw.BiLinear.Scale(dst, dst.Bounds(), src, b, xdraw.Over, nil)
	return dst
}

func (sc *SimClient) pixelSizeOrDefault() int {
	if sc.model.PixelSize > 0 {
		return sc.model.PixelSize
	}
	return 72
}

func (sc *SimClient) ReadKeys() ([]bool, error) {
	select {
	case ev := <-sc.keyEventsCh:
		keys := make([]bool, sc.model.Keys)
		if ev.Key >= 0 && ev.Key < sc.model.Keys {
			keys[ev.Key] = ev.Pressed
		}
		return keys, nil
	case <-time.After(100 * time.Millisecond):
		return make([]bool, sc.model.Keys), nil
	case <-sc.ctx.Done():
		return nil, sc.ctx.Err()
	}
}

func (sc *SimClient) WaitForKeyPress(ctx context.Context) (int, error) {
	for {
		select {
		case <-ctx.Done():
			return -1, ctx.Err()
		case ev := <-sc.keyEventsCh:
			if ev.Pressed {
				return ev.Key, nil
			}
		}
	}
}

func (sc *SimClient) ListenKeys(ctx context.Context, events chan<- KeyEvent) {
	go func() {
		defer close(events)
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-sc.keyEventsCh:
				if !ok {
					return
				}
				select {
				case events <- ev:
				case <-ctx.Done():
					return
				}
			case <-sc.ctx.Done():
				return
			}
		}
	}()
}

func (sc *SimClient) KeyToCoord(keyIndex int) (col, row int) {
	if sc.model.Cols == 0 {
		return 0, 0
	}
	return keyIndex % sc.model.Cols, keyIndex / sc.model.Cols
}

func (sc *SimClient) CoordToKey(col, row int) int {
	return row*sc.model.Cols + col
}

func (sc *SimClient) Cols() int           { return sc.model.Cols }
func (sc *SimClient) Rows() int           { return sc.model.Rows }
func (sc *SimClient) Keys() int           { return sc.model.Keys }
func (sc *SimClient) PixelSize() int      { return sc.model.PixelSize }
func (sc *SimClient) ModelName() string   { return sc.model.Name }
func (sc *SimClient) GetInfo() DeviceInfo { return sc.info }
