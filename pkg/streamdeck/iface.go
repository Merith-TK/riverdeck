package streamdeck

import (
	"context"
	"image"
	"image/color"
)

// DeviceIface abstracts a Stream Deck device so that both real hardware
// (via *Device) and the software simulator (via SimClient) can be used
// interchangeably by the application layer.
//
// All methods on *Device satisfy this interface. Add new methods here when
// adding capabilities that must also be provided by SimClient.
type DeviceIface interface {
	// Lifecycle
	Close() error

	// Display
	SetBrightness(percent int) error
	Clear() error
	Reset() error

	// Key images
	SetImage(keyIndex int, img image.Image) error
	SetKeyColor(keyIndex int, c color.Color) error
	EncodeKeyImage(img image.Image) ([]byte, error)
	WriteKeyData(keyIndex int, imageData []byte) error
	ResizeImage(src image.Image) image.Image

	// Key input
	ReadKeys() ([]bool, error)
	WaitForKeyPress(ctx context.Context) (int, error)
	ListenKeys(ctx context.Context, events chan<- KeyEvent)
	KeyToCoord(keyIndex int) (col, row int)
	CoordToKey(col, row int) int

	// Device info  (replaces direct .Model / .Info field access)
	Cols() int
	Rows() int
	Keys() int
	PixelSize() int
	ModelName() string
	GetInfo() DeviceInfo
}
