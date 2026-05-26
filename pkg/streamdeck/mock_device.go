package streamdeck

import (
	"context"
	"image"
	"image/color"
	"sync"
)

// MockDevice implements DeviceIface for unit testing. It records all calls
// and provides hooks to simulate device behavior without real HID hardware.
//
// Zero value is usable; call NewMockDevice() to initialise the default model.
type MockDevice struct {
	mu sync.Mutex

	// Model to report for Cols/Rows/Keys/PixelSize/ModelName.
	Model Model

	// Brightness records the last SetBrightness value.
	Brightness int

	// Images records every SetImage call by key index.
	Images map[int]image.Image

	// KeyColors records every SetKeyColor call by key index.
	KeyColors map[int]color.Color

	// Labels records every SetLabel call by key index.
	Labels map[int]string

	// ClearCount counts Clear() calls.
	ClearCount int

	// ResetCount counts Reset() calls.
	ResetCount int

	// CloseCount counts Close() calls.
	CloseCount int

	// EncodeResults holds pre-configured results for EncodeKeyImage.
	// Keyed by key index; if absent returns a default byte slice.
	EncodeResults map[int]EncodeResult

	// KeyState is the simulated key state returned by ReadKeys.
	// Zero value means all keys up.
	KeyState []bool

	// KeyEvents are sent by ListenKeys on each poll cycle.
	// If nil, ListenKeys never sends events (keeps channel open).
	KeyEvents []KeyEvent

	// ListenKeysHook, if set, is called at the start of ListenKeys.
	// Useful for injecting events or blocking until test setup is done.
	ListenKeysHook func(ctx context.Context, events chan<- KeyEvent)
}

// EncodeResult holds the result of an EncodeKeyImage call.
type EncodeResult struct {
	Data []byte
	Err  error
}

// NewMockDevice creates a MockDevice initialised with a default XL model.
func NewMockDevice() *MockDevice {
	return &MockDevice{
		Model: Model{
			Name:        "Stream Deck XL (mock)",
			ProductID:   0x006c,
			Cols:        8,
			Rows:        4,
			Keys:        32,
			PixelSize:   96,
			ImageFormat: "JPEG",
		},
		Images:        make(map[int]image.Image),
		KeyColors:     make(map[int]color.Color),
		Labels:        make(map[int]string),
		EncodeResults: make(map[int]EncodeResult),
	}
}

// NewMockDevicePedal creates a MockDevice configured as a Stream Deck Pedal.
func NewMockDevicePedal() *MockDevice {
	return &MockDevice{
		Model: Model{
			Name:      "Stream Deck Pedal (mock)",
			ProductID: 0x0086,
			Cols:      3,
			Rows:      1,
			Keys:      3,
			PixelSize: 0,
		},
		Images:    make(map[int]image.Image),
		KeyColors: make(map[int]color.Color),
		Labels:    make(map[int]string),
	}
}

// -- DeviceIface implementation ------------------------------------------------

func (m *MockDevice) SetBrightness(percent int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Brightness = percent
	return nil
}

func (m *MockDevice) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ClearCount++
	return nil
}

func (m *MockDevice) Reset() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ResetCount++
	return nil
}

func (m *MockDevice) SetImage(keyIndex int, img image.Image) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Images[keyIndex] = img
	return nil
}

func (m *MockDevice) SetKeyColor(keyIndex int, c color.Color) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.KeyColors[keyIndex] = c
	return nil
}

func (m *MockDevice) EncodeKeyImage(keyIndex int, img image.Image) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if res, ok := m.EncodeResults[keyIndex]; ok {
		return res.Data, res.Err
	}
	return []byte{0, 1, 2, 3}, nil
}

func (m *MockDevice) WriteKeyData(keyIndex int, imageData []byte) error {
	return nil
}

func (m *MockDevice) ResizeImage(src image.Image) image.Image {
	size := m.Model.PixelSize
	if size == 0 {
		return src
	}
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	return dst
}

func (m *MockDevice) SetLabel(keyIndex int, text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Labels[keyIndex] = text
	return nil
}

func (m *MockDevice) ReadKeys() ([]bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.KeyState == nil {
		m.KeyState = make([]bool, m.Model.Keys)
	}
	return m.KeyState, nil
}

func (m *MockDevice) WaitForKeyPress(ctx context.Context) (int, error) {
	ch := make(chan KeyEvent, 1)
	go m.ListenKeys(ctx, ch)
	for ev := range ch {
		if ev.Pressed {
			return ev.Key, nil
		}
	}
	return -1, ctx.Err()
}

func (m *MockDevice) ListenKeys(ctx context.Context, events chan<- KeyEvent) {
	go func() {
		defer close(events)
		if m.ListenKeysHook != nil {
			m.ListenKeysHook(ctx, events)
			return
		}
		<-ctx.Done()
	}()
}

func (m *MockDevice) KeyToCoord(keyIndex int) (col, row int) {
	if m.Model.Cols == 0 {
		return 0, 0
	}
	return keyIndex % m.Model.Cols, keyIndex / m.Model.Cols
}

func (m *MockDevice) CoordToKey(col, row int) int {
	return row*m.Model.Cols + col
}

func (m *MockDevice) Cols() int     { return m.Model.Cols }
func (m *MockDevice) Rows() int     { return m.Model.Rows }
func (m *MockDevice) Keys() int     { return m.Model.Keys }
func (m *MockDevice) PixelSize() int { return m.Model.PixelSize }
func (m *MockDevice) ModelName() string { return m.Model.Name }

func (m *MockDevice) GetInfo() DeviceInfo {
	return DeviceInfo{
		Path:   "/mock/device",
		Serial: "MOCK00001",
		Model:  m.Model,
	}
}

func (m *MockDevice) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CloseCount++
	return nil
}
