// Package wsdevice provides a WebSocket-backed virtual Stream Deck device.
//
// A software client (web app, mobile app, desktop app) connects via WebSocket
// and receives the layout state as a stream of JSON messages.  Key-press events
// are sent back from the client over the same connection.
//
// Protocol – server → client (JSON text frames):
//
//	{"type":"devinfo",      "uuid":"…","cols":5,"rows":3,"keys":15,"pixel_size":72,"model_name":"Virtual Device"}
//	{"type":"setimage",     "key":0,"data":"<base64 PNG>"}
//	{"type":"setkeycolor",  "key":0,"r":255,"g":0,"b":0}
//	{"type":"setbrightness","value":75}
//	{"type":"clear"}
//	{"type":"reset"}
//
// Protocol – client → server (JSON text frames):
//
//	{"type":"keyevent","key":0,"pressed":true}
package wsdevice

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/merith-tk/riverdeck/pkg/streamdeck"
	xdraw "golang.org/x/image/draw"
)

// Device implements streamdeck.DeviceIface over a gorilla WebSocket connection.
// Each connection represents one logical software client device.
type Device struct {
	conn        *websocket.Conn
	mu          sync.Mutex // serialises writes
	uuid        string
	model       streamdeck.Model
	info        streamdeck.DeviceInfo
	keyEventsCh chan streamdeck.KeyEvent
	ctx         context.Context
	cancel      context.CancelFunc
}

func newDevice(conn *websocket.Conn, uuid string, model streamdeck.Model) *Device {
	ctx, cancel := context.WithCancel(context.Background())
	d := &Device{
		conn:        conn,
		uuid:        uuid,
		model:       model,
		keyEventsCh: make(chan streamdeck.KeyEvent, 64),
		ctx:         ctx,
		cancel:      cancel,
	}
	d.info = streamdeck.DeviceInfo{
		Path:         "ws://" + conn.RemoteAddr().String(),
		Serial:       uuid,
		Manufacturer: "Riverdeck",
		Product:      "Virtual Device",
		Model:        model,
	}
	return d
}

// UUID returns the unique identifier assigned to this software client device.
func (d *Device) UUID() string { return d.uuid }

// Done returns a channel that is closed when the WebSocket connection closes.
func (d *Device) Done() <-chan struct{} { return d.ctx.Done() }

// Context returns the device's lifecycle context.
func (d *Device) Context() context.Context { return d.ctx }

// wsInbound is the union of all message types received from a client.
type wsInbound struct {
	Type    string `json:"type"`
	Key     int    `json:"key"`
	Pressed bool   `json:"pressed"`
}

// readLoop reads JSON messages from the WebSocket and routes them.
// It cancels the device context when the connection closes.
func (d *Device) readLoop() {
	defer d.cancel()
	for {
		_, msg, err := d.conn.ReadMessage()
		if err != nil {
			return
		}
		var in wsInbound
		if err := json.Unmarshal(msg, &in); err != nil {
			continue
		}
		if in.Type == "keyevent" {
			select {
			case d.keyEventsCh <- streamdeck.KeyEvent{Key: in.Key, Pressed: in.Pressed}:
			default: // drop if buffer full
			}
		}
	}
}

// sendJSON encodes v as JSON and sends it as a WebSocket text frame.
func (d *Device) sendJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.conn.WriteMessage(websocket.TextMessage, data)
}

// ── DeviceIface ────────────────────────────────────────────────────────────

func (d *Device) Close() error {
	d.cancel()
	return d.conn.Close()
}

func (d *Device) SetBrightness(percent int) error {
	return d.sendJSON(map[string]any{"type": "setbrightness", "value": percent})
}

func (d *Device) Clear() error {
	return d.sendJSON(map[string]any{"type": "clear"})
}

func (d *Device) Reset() error {
	return d.sendJSON(map[string]any{"type": "reset"})
}

func (d *Device) SetImage(keyIndex int, img image.Image) error {
	data, err := d.EncodeKeyImage(img)
	if err != nil {
		return err
	}
	return d.WriteKeyData(keyIndex, data)
}

func (d *Device) SetKeyColor(keyIndex int, c color.Color) error {
	r, g, b, _ := c.RGBA()
	return d.sendJSON(map[string]any{
		"type": "setkeycolor",
		"key":  keyIndex,
		"r":    int(r >> 8),
		"g":    int(g >> 8),
		"b":    int(b >> 8),
	})
}

// EncodeKeyImage resizes img to the device pixel size and encodes it as PNG.
// No 180° rotation is applied — software clients display images as-is.
func (d *Device) EncodeKeyImage(img image.Image) ([]byte, error) {
	resized := d.ResizeImage(img)
	var buf bytes.Buffer
	if err := png.Encode(&buf, resized); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// WriteKeyData sends pre-encoded image bytes as a base64 setimage message.
func (d *Device) WriteKeyData(keyIndex int, imageData []byte) error {
	return d.sendJSON(map[string]any{
		"type": "setimage",
		"key":  keyIndex,
		"data": base64.StdEncoding.EncodeToString(imageData),
	})
}

// ResizeImage scales src to the device pixel size.
func (d *Device) ResizeImage(src image.Image) image.Image {
	size := d.pixelSizeOrDefault()
	b := src.Bounds()
	if b.Dx() == size && b.Dy() == size {
		return src
	}
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	xdraw.BiLinear.Scale(dst, dst.Bounds(), src, b, xdraw.Over, nil)
	return dst
}

func (d *Device) pixelSizeOrDefault() int {
	if d.model.PixelSize > 0 {
		return d.model.PixelSize
	}
	return 72
}

func (d *Device) ReadKeys() ([]bool, error) {
	select {
	case ev := <-d.keyEventsCh:
		keys := make([]bool, d.model.Keys)
		if ev.Key >= 0 && ev.Key < d.model.Keys {
			keys[ev.Key] = ev.Pressed
		}
		return keys, nil
	case <-time.After(100 * time.Millisecond):
		return make([]bool, d.model.Keys), nil
	case <-d.ctx.Done():
		return nil, d.ctx.Err()
	}
}

func (d *Device) WaitForKeyPress(ctx context.Context) (int, error) {
	for {
		select {
		case <-ctx.Done():
			return -1, ctx.Err()
		case ev := <-d.keyEventsCh:
			if ev.Pressed {
				return ev.Key, nil
			}
		}
	}
}

func (d *Device) ListenKeys(ctx context.Context, events chan<- streamdeck.KeyEvent) {
	go func() {
		defer close(events)
		for {
			select {
			case <-ctx.Done():
				return
			case <-d.ctx.Done():
				return
			case ev, ok := <-d.keyEventsCh:
				if !ok {
					return
				}
				select {
				case events <- ev:
				case <-ctx.Done():
					return
				case <-d.ctx.Done():
					return
				}
			}
		}
	}()
}

func (d *Device) KeyToCoord(keyIndex int) (col, row int) {
	if d.model.Cols == 0 {
		return 0, 0
	}
	return keyIndex % d.model.Cols, keyIndex / d.model.Cols
}

func (d *Device) CoordToKey(col, row int) int {
	return row*d.model.Cols + col
}

func (d *Device) Cols() int                    { return d.model.Cols }
func (d *Device) Rows() int                    { return d.model.Rows }
func (d *Device) Keys() int                    { return d.model.Keys }
func (d *Device) PixelSize() int               { return d.model.PixelSize }
func (d *Device) ModelName() string            { return d.model.Name }
func (d *Device) GetInfo() streamdeck.DeviceInfo { return d.info }
