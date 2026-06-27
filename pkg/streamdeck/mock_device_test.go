package streamdeck

import (
	"context"
	"image"
	"image/color"
	"testing"
)

func TestNewMockDevice_Defaults(t *testing.T) {
	m := NewMockDevice()
	if m.Cols() != 8 {
		t.Errorf("Cols = %d, want 8", m.Cols())
	}
	if m.Rows() != 4 {
		t.Errorf("Rows = %d, want 4", m.Rows())
	}
	if m.Keys() != 32 {
		t.Errorf("Keys = %d, want 32", m.Keys())
	}
	if m.PixelSize() != 96 {
		t.Errorf("PixelSize = %d, want 96", m.PixelSize())
	}
	if m.ModelName() != "Stream Deck XL (mock)" {
		t.Errorf("ModelName = %q, want %q", m.ModelName(), "Stream Deck XL (mock)")
	}
}

func TestMockDevice_SetBrightness(t *testing.T) {
	m := NewMockDevice()
	if err := m.SetBrightness(50); err != nil {
		t.Fatalf("SetBrightness: %v", err)
	}
	if m.Brightness != 50 {
		t.Errorf("Brightness = %d, want 50", m.Brightness)
	}
}

func TestMockDevice_SetImage(t *testing.T) {
	m := NewMockDevice()
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	if err := m.SetImage(5, img); err != nil {
		t.Fatalf("SetImage: %v", err)
	}
	if _, ok := m.Images[5]; !ok {
		t.Error("SetImage did not record image at key 5")
	}
}

func TestMockDevice_SetKeyColor(t *testing.T) {
	m := NewMockDevice()
	c := color.RGBA{R: 255, G: 0, B: 0, A: 255}
	if err := m.SetKeyColor(3, c); err != nil {
		t.Fatalf("SetKeyColor: %v", err)
	}
	recorded, ok := m.KeyColors[3]
	if !ok {
		t.Fatal("SetKeyColor did not record at key 3")
	}
	r, g, b, a := recorded.RGBA()
	if r != 65535 || g != 0 || b != 0 || a != 65535 {
		t.Errorf("KeyColors[3] = (%d, %d, %d, %d), want (65535, 0, 0, 65535)", r, g, b, a)
	}
}

func TestMockDevice_ClearAndReset(t *testing.T) {
	m := NewMockDevice()
	m.Clear()
	m.Reset()
	m.Clear()
	if m.ClearCount != 2 {
		t.Errorf("ClearCount = %d, want 2", m.ClearCount)
	}
	if m.ResetCount != 1 {
		t.Errorf("ResetCount = %d, want 1", m.ResetCount)
	}
}

func TestMockDevice_Close(t *testing.T) {
	m := NewMockDevice()
	m.Close()
	m.Close()
	if m.CloseCount != 2 {
		t.Errorf("CloseCount = %d, want 2", m.CloseCount)
	}
}

func TestMockDevice_EncodeKeyImage(t *testing.T) {
	m := NewMockDevice()
	data, err := m.EncodeKeyImage(0, nil)
	if err != nil {
		t.Fatalf("EncodeKeyImage: %v", err)
	}
	if len(data) != 4 {
		t.Errorf("EncodeKeyImage data len = %d, want 4", len(data))
	}

	// Pre-configured result
	m.EncodeResults[1] = EncodeResult{Data: []byte{0xFF}, Err: nil}
	data, err = m.EncodeKeyImage(1, nil)
	if err != nil {
		t.Fatalf("EncodeKeyImage with result: %v", err)
	}
	if len(data) != 1 || data[0] != 0xFF {
		t.Errorf("EncodeKeyImage with result = %v, want [255]", data)
	}
}

func TestMockDevice_CoordMapping(t *testing.T) {
	m := NewMockDevice() // 8 cols x 4 rows

	tests := []struct {
		key  int
		col  int
		row  int
	}{
		{0, 0, 0},
		{7, 7, 0},
		{8, 0, 1},
		{31, 7, 3},
	}
	for _, tc := range tests {
		col, row := m.KeyToCoord(tc.key)
		if col != tc.col || row != tc.row {
			t.Errorf("KeyToCoord(%d) = (%d,%d), want (%d,%d)", tc.key, col, row, tc.col, tc.row)
		}
		key := m.CoordToKey(tc.col, tc.row)
		if key != tc.key {
			t.Errorf("CoordToKey(%d,%d) = %d, want %d", tc.col, tc.row, key, tc.key)
		}
	}
}

func TestMockDevice_GetInfo(t *testing.T) {
	m := NewMockDevice()
	info := m.GetInfo()
	if info.Path != "/mock/device" {
		t.Errorf("GetInfo().Path = %q, want %q", info.Path, "/mock/device")
	}
	if info.Serial != "MOCK00001" {
		t.Errorf("GetInfo().Serial = %q, want %q", info.Serial, "MOCK00001")
	}
}

func TestNewMockDevicePedal(t *testing.T) {
	m := NewMockDevicePedal()
	if m.Keys() != 3 {
		t.Errorf("Pedal Keys = %d, want 3", m.Keys())
	}
	if m.PixelSize() != 0 {
		t.Errorf("Pedal PixelSize = %d, want 0", m.PixelSize())
	}
	if m.Cols() != 3 {
		t.Errorf("Pedal Cols = %d, want 3", m.Cols())
	}
	if m.Rows() != 1 {
		t.Errorf("Pedal Rows = %d, want 1", m.Rows())
	}
}

func TestMockDevice_ListenKeysHook(t *testing.T) {
	m := NewMockDevice()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := make(chan KeyEvent, 10)
	m.ListenKeysHook = func(ctx context.Context, ch chan<- KeyEvent) {
		ch <- KeyEvent{Key: 1, Pressed: true}
		ch <- KeyEvent{Key: 1, Pressed: false}
		cancel()
	}

	done := make(chan struct{})
	go func() {
		m.ListenKeys(ctx, events)
		close(done)
	}()

	var received []KeyEvent
	for ev := range events {
		received = append(received, ev)
	}
	<-done

	if len(received) != 2 {
		t.Errorf("received %d events, want 2", len(received))
	}
	if received[0].Key != 1 || !received[0].Pressed {
		t.Errorf("event[0] = (key=%d, pressed=%v), want (1, true)", received[0].Key, received[0].Pressed)
	}
}
