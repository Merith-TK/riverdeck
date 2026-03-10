// Package streamdeck provides a Go library for interfacing with Elgato Stream Deck devices.
package streamdeck

// VendorID is the USB vendor ID for Elgato devices.
const VendorID = 0x0fd9

// Model contains specifications for a Stream Deck model.
type Model struct {
	Name        string
	ProductID   uint16
	Cols        int
	Rows        int
	Keys        int
	PixelSize   int
	ImageFormat string // "JPEG" or "BMP"
}

// Known Stream Deck models indexed by their USB Product ID.
var Models = map[uint16]Model{
	0x0060: {Name: "Stream Deck Original", ProductID: 0x0060, Cols: 5, Rows: 3, Keys: 15, PixelSize: 72, ImageFormat: "BMP"},
	0x0063: {Name: "Stream Deck Mini", ProductID: 0x0063, Cols: 3, Rows: 2, Keys: 6, PixelSize: 80, ImageFormat: "BMP"},
	0x006c: {Name: "Stream Deck XL", ProductID: 0x006c, Cols: 8, Rows: 4, Keys: 32, PixelSize: 96, ImageFormat: "JPEG"},
	0x006d: {Name: "Stream Deck Original V2", ProductID: 0x006d, Cols: 5, Rows: 3, Keys: 15, PixelSize: 72, ImageFormat: "JPEG"},
	0x0080: {Name: "Stream Deck MK.2", ProductID: 0x0080, Cols: 5, Rows: 3, Keys: 15, PixelSize: 72, ImageFormat: "JPEG"},
	0x0084: {Name: "Stream Deck XL V2", ProductID: 0x0084, Cols: 8, Rows: 4, Keys: 32, PixelSize: 96, ImageFormat: "JPEG"},
	0x0086: {Name: "Stream Deck Pedal", ProductID: 0x0086, Cols: 3, Rows: 1, Keys: 3, PixelSize: 0, ImageFormat: ""},
	0x0090: {Name: "Stream Deck Neo", ProductID: 0x0090, Cols: 4, Rows: 2, Keys: 8, PixelSize: 96, ImageFormat: "JPEG"},
	0x009a: {Name: "Stream Deck +", ProductID: 0x009a, Cols: 4, Rows: 2, Keys: 8, PixelSize: 120, ImageFormat: "JPEG"},
}

// LookupModel returns the Model for a given product ID.
// If the product ID is unknown, it returns a placeholder Model with basic info.
func LookupModel(productID uint16) (Model, bool) {
	model, known := Models[productID]
	return model, known
}
