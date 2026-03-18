// Package wsdevice provides a WebSocket-backed virtual Stream Deck device.
//
// A software client (web app, mobile app, custom hardware) connects via
// WebSocket and sends a hello message declaring its identity and capabilities.
// The server responds with ack, then begins pushing frame and label messages
// per input.  Key events arrive as input messages from the client.
//
// Protocol - client -> server (JSON text frames):
//
//	{"type":"hello","id":"<stable-id>","name":"My Device","rows":3,"cols":5,
//	  "formats":["png"],"inputs":[{"id":"btn0","type":"button","x":0,"y":0,
//	  "display":{"image":true,"imageWidth":72,"imageHeight":72,"text":true}},...]}
//	{"type":"input","id":"btn0","event":"press"}
//	{"type":"input","id":"btn0","event":"release"}
//	{"type":"input","id":"dial0","event":"valueInc"}
//
// Protocol - server -> client (JSON text frames):
//
//	{"type":"ack","status":"ok"}
//	{"type":"ack","status":"error","reason":"<reason>"}
//	{"type":"frame","id":"btn0","width":72,"height":72,"data":"<base64>"}
//	{"type":"label","id":"btn0","text":"Volume"}
//	{"type":"layoutChange","layoutId":"<id>","layoutName":"<name>"}
package wsdevice

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/merith-tk/riverdeck/pkg/streamdeck"
	xdraw "golang.org/x/image/draw"
)

// Device implements streamdeck.DeviceIface over a gorilla WebSocket connection.
// It is constructed from the hello message sent by the client.
type Device struct {
	conn *websocket.Conn
	mu   sync.Mutex // serialises writes
	id   string     // client-provided stable ID
	name string

	// Grid geometry derived from the hello message.
	cols int
	rows int

	// Ordered input list (index == key slot used by DeviceIface).
	inputs     []InputDescriptor
	inputByID  map[string]int // input ID -> key index
	coordToKey map[[2]int]int // [col,row] -> key index
	keyToCoord map[int][2]int // key index -> [col,row]

	// Image format negotiation: per-input overrides fall back to device level.
	devFormats   []string         // device-level supported formats
	inputFormats map[int][]string // per-input format overrides

	keyEventsCh chan streamdeck.KeyEvent
	ctx         context.Context
	cancel      context.CancelFunc
	info        streamdeck.DeviceInfo
}

// newDevice constructs a Device from a parsed hello message.
func newDevice(conn *websocket.Conn, h helloMsg) *Device {
	ctx, cancel := context.WithCancel(context.Background())

	inputs := make([]InputDescriptor, len(h.Inputs))
	inputByID := make(map[string]int, len(h.Inputs))
	coordToKey := make(map[[2]int]int)
	keyToCoord := make(map[int][2]int)
	inputFormats := make(map[int][]string)

	for i, spec := range h.Inputs {
		inp := InputDescriptor{
			ID:          spec.ID,
			Type:        spec.Type,
			Image:       spec.Display.Image,
			ImageWidth:  spec.Display.ImageWidth,
			ImageHeight: spec.Display.ImageHeight,
			Text:        spec.Display.Text,
			Formats:     spec.Display.Formats,
		}
		if spec.X != nil && spec.Y != nil {
			inp.X, inp.Y, inp.HasXY = *spec.X, *spec.Y, true
			coordToKey[[2]int{inp.X, inp.Y}] = i
			keyToCoord[i] = [2]int{inp.X, inp.Y}
		}
		inputs[i] = inp
		inputByID[spec.ID] = i
		if len(spec.Display.Formats) > 0 {
			inputFormats[i] = spec.Display.Formats
		}
	}

	d := &Device{
		conn:         conn,
		id:           h.ID,
		name:         h.Name,
		cols:         h.Cols,
		rows:         h.Rows,
		inputs:       inputs,
		inputByID:    inputByID,
		coordToKey:   coordToKey,
		keyToCoord:   keyToCoord,
		devFormats:   h.Formats,
		inputFormats: inputFormats,
		keyEventsCh:  make(chan streamdeck.KeyEvent, 64),
		ctx:          ctx,
		cancel:       cancel,
	}
	d.info = streamdeck.DeviceInfo{
		Path:         "ws://" + conn.RemoteAddr().String(),
		Serial:       h.ID,
		Manufacturer: "Riverdeck",
		Product:      h.Name,
	}
	return d
}

// ID returns the client-provided stable device identifier.
func (d *Device) ID() string { return d.id }

// Done returns a channel closed when the WebSocket connection closes.
func (d *Device) Done() <-chan struct{} { return d.ctx.Done() }

// Context returns the device's lifecycle context.
func (d *Device) Context() context.Context { return d.ctx }

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
		if in.Type == "input" {
			idx, ok := d.inputByID[in.ID]
			if !ok {
				continue
			}
			var pressed bool
			switch in.Event {
			case "press":
				pressed = true
			case "release":
				pressed = false
			default:
				// held, valueInc, valueDec, value -- treat as press for now.
				pressed = true
			}
			select {
			case d.keyEventsCh <- streamdeck.KeyEvent{Key: idx, Pressed: pressed}:
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

// chooseFormat returns the image format to use for a given key index.
// Priority: per-input override -> device-level -> "png".
func (d *Device) chooseFormat(keyIndex int) string {
	if fmts, ok := d.inputFormats[keyIndex]; ok && len(fmts) > 0 {
		return fmts[0]
	}
	if len(d.devFormats) > 0 {
		return d.devFormats[0]
	}
	return "png"
}

// pixelSizeForKey returns the declared pixel size for an input, falling back to 72.
func (d *Device) pixelSizeForKey(keyIndex int) int {
	if keyIndex >= 0 && keyIndex < len(d.inputs) {
		if d.inputs[keyIndex].ImageWidth > 0 {
			return d.inputs[keyIndex].ImageWidth
		}
	}
	return 72
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
	data, err := d.EncodeKeyImage(keyIndex, img)
	if err != nil {
		return err
	}
	return d.WriteKeyData(keyIndex, data)
}

func (d *Device) SetKeyColor(keyIndex int, c color.Color) error {
	size := d.pixelSizeForKey(keyIndex)
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: c}, image.Point{}, draw.Src)
	return d.SetImage(keyIndex, img)
}

// EncodeKeyImage resizes img for the given key's declared dimensions and
// encodes it in the format that input advertised (or the device default).
func (d *Device) EncodeKeyImage(keyIndex int, img image.Image) ([]byte, error) {
	size := d.pixelSizeForKey(keyIndex)
	resized := d.resizeTo(img, size)
	format := d.chooseFormat(keyIndex)

	var buf bytes.Buffer
	switch format {
	case "jpeg", "jpg":
		if err := jpeg.Encode(&buf, resized, &jpeg.Options{Quality: 90}); err != nil {
			return nil, fmt.Errorf("jpeg encode: %w", err)
		}
	default: // "png" and anything unrecognised
		if err := png.Encode(&buf, resized); err != nil {
			return nil, fmt.Errorf("png encode: %w", err)
		}
	}
	return buf.Bytes(), nil
}

// WriteKeyData sends pre-encoded image bytes as a frame message.
func (d *Device) WriteKeyData(keyIndex int, imageData []byte) error {
	if keyIndex < 0 || keyIndex >= len(d.inputs) {
		return fmt.Errorf("key index %d out of range", keyIndex)
	}
	inputID := d.inputs[keyIndex].ID
	size := d.pixelSizeForKey(keyIndex)
	return d.sendJSON(map[string]any{
		"type":   "frame",
		"id":     inputID,
		"width":  size,
		"height": size,
		"data":   base64.StdEncoding.EncodeToString(imageData),
	})
}

// SetLabel sends a label text update for the given key index.
func (d *Device) SetLabel(keyIndex int, text string) error {
	if keyIndex < 0 || keyIndex >= len(d.inputs) {
		return nil
	}
	if !d.inputs[keyIndex].Text {
		return nil // client didn't declare text support for this input
	}
	return d.sendJSON(map[string]any{
		"type": "label",
		"id":   d.inputs[keyIndex].ID,
		"text": text,
	})
}

// ResizeImage scales src to the default pixel size (first input's size or 72).
func (d *Device) ResizeImage(src image.Image) image.Image {
	return d.resizeTo(src, d.pixelSizeForKey(0))
}

func (d *Device) resizeTo(src image.Image, size int) image.Image {
	b := src.Bounds()
	if b.Dx() == size && b.Dy() == size {
		return src
	}
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	xdraw.BiLinear.Scale(dst, dst.Bounds(), src, b, xdraw.Over, nil)
	return dst
}

func (d *Device) ReadKeys() ([]bool, error) {
	keys := make([]bool, len(d.inputs))
	select {
	case ev := <-d.keyEventsCh:
		if ev.Key >= 0 && ev.Key < len(d.inputs) {
			keys[ev.Key] = ev.Pressed
		}
		return keys, nil
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
	if c, ok := d.keyToCoord[keyIndex]; ok {
		return c[0], c[1]
	}
	if d.cols == 0 {
		return 0, 0
	}
	return keyIndex % d.cols, keyIndex / d.cols
}

func (d *Device) CoordToKey(col, row int) int {
	if k, ok := d.coordToKey[[2]int{col, row}]; ok {
		return k
	}
	return row*d.cols + col
}

func (d *Device) Cols() int      { return d.cols }
func (d *Device) Rows() int      { return d.rows }
func (d *Device) Keys() int      { return len(d.inputs) }
func (d *Device) PixelSize() int { return d.pixelSizeForKey(0) }
func (d *Device) ModelName() string {
	return fmt.Sprintf("%s (%dx%d)", d.name, d.cols, d.rows)
}
func (d *Device) GetInfo() streamdeck.DeviceInfo { return d.info }
func (d *Device) Name() string                   { return d.name }
func (d *Device) Inputs() []InputDescriptor      { return d.inputs }
