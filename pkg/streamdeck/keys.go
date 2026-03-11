package streamdeck

import (
	"context"
	"time"
)

// ReadKeys reads the current state of all keys.
// Returns a slice of booleans where true means the key is pressed.
func (d *Device) ReadKeys() ([]bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Read buffer size depends on device, use generous buffer
	buf := make([]byte, 512)
	n, err := d.hid.ReadWithTimeout(buf, 100*time.Millisecond)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		// No data available, return current state as all unpressed
		return make([]bool, d.Model.Keys), nil
	}

	// Parse key states - format depends on device generation
	// For MK.2/V2: first byte is report ID (0x01), then key states starting at offset 4
	keys := make([]bool, d.Model.Keys)
	keyOffset := 4 // MK.2/V2 offset
	for i := 0; i < d.Model.Keys && keyOffset+i < n; i++ {
		keys[i] = buf[keyOffset+i] != 0
	}

	return keys, nil
}

// WaitForKeyPress blocks until a key is pressed or the context is cancelled.
// Returns the index of the pressed key.
func (d *Device) WaitForKeyPress(ctx context.Context) (int, error) {
	prevState := make([]bool, d.Model.Keys)

	for {
		select {
		case <-ctx.Done():
			return -1, ctx.Err()
		default:
		}

		keys, err := d.ReadKeys()
		if err != nil {
			return -1, err
		}

		// Check for newly pressed keys
		for i, pressed := range keys {
			if pressed && !prevState[i] {
				return i, nil
			}
		}

		copy(prevState, keys)
		time.Sleep(10 * time.Millisecond)
	}
}

// ListenKeys starts listening for key events and sends them to the provided channel.
// Closes the channel when context is cancelled.
func (d *Device) ListenKeys(ctx context.Context, events chan<- KeyEvent) {
	go func() {
		defer close(events)
		prevState := make([]bool, d.Model.Keys)

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			keys, err := d.ReadKeys()
			if err != nil {
				continue
			}

			// Detect state changes
			for i, pressed := range keys {
				if pressed != prevState[i] {
					select {
					case events <- KeyEvent{Key: i, Pressed: pressed}:
					case <-ctx.Done():
						return
					}
				}
			}

			copy(prevState, keys)
			time.Sleep(10 * time.Millisecond)
		}
	}()
}

// KeyToCoord converts a key index to (col, row) coordinates.
func (d *Device) KeyToCoord(keyIndex int) (col, row int) {
	if d.Model.Cols == 0 {
		return 0, 0
	}
	return keyIndex % d.Model.Cols, keyIndex / d.Model.Cols
}

// CoordToKey converts (col, row) coordinates to a key index.
func (d *Device) CoordToKey(col, row int) int {
	return row*d.Model.Cols + col
}

// Cols returns the number of columns on the device.
func (d *Device) Cols() int {
	return d.Model.Cols
}

// Rows returns the number of rows on the device.
func (d *Device) Rows() int {
	return d.Model.Rows
}

// Keys returns the total number of keys on the device.
func (d *Device) Keys() int {
	return d.Model.Keys
}

// PixelSize returns the pixel dimensions for key images.
func (d *Device) PixelSize() int {
	return d.Model.PixelSize
}

// ModelName returns the human-readable name of the device model.
func (d *Device) ModelName() string {
	return d.Model.Name
}

// GetInfo returns the DeviceInfo for this device.
func (d *Device) GetInfo() DeviceInfo {
	return d.Info
}
