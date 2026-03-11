// Package streamdeck provides a Go library for interfacing with Elgato Stream Deck devices.
package streamdeck

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"sync"

	"github.com/sstallion/go-hid"
)

// Device represents an opened Stream Deck device.
type Device struct {
	hid     *hid.Device
	Info    DeviceInfo
	Model   Model
	readMu  sync.Mutex // serialises HID reads  (ReadKeys / ReadWithTimeout)
	writeMu sync.Mutex // serialises HID writes (writeImageData, feature reports)

	// Performance settings
	jpegQuality int
}

// KeyEvent represents a key press or release event.
type KeyEvent struct {
	Key     int
	Pressed bool
}

// Open opens a Stream Deck device by its HID path.
func Open(path string) (*Device, error) {
	dev, err := hid.OpenPath(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open device: %w", err)
	}

	// Get device info
	manufacturer, _ := dev.GetMfrStr()
	product, _ := dev.GetProductStr()
	serial, _ := dev.GetSerialNbr()

	// We need to get product ID - enumerate to find it
	var productID uint16
	err = hid.Enumerate(VendorID, 0x0000, func(info *hid.DeviceInfo) error {
		if info.Path == path {
			productID = info.ProductID
		}
		return nil
	})
	if err != nil {
		dev.Close()
		return nil, fmt.Errorf("failed to get product ID: %w", err)
	}

	model, _ := LookupModel(productID)
	if model.ProductID == 0 {
		model = Model{
			Name:      fmt.Sprintf("Unknown Stream Deck (PID: 0x%04X)", productID),
			ProductID: productID,
		}
	}

	d := &Device{
		hid:   dev,
		Model: model,
		Info: DeviceInfo{
			Path:         path,
			Serial:       serial,
			Manufacturer: manufacturer,
			Product:      product,
			Model:        model,
			Firmware:     getFirmwareVersion(dev),
		},
	}

	return d, nil
}

// OpenWithConfig opens a Stream Deck device with performance configuration.
func OpenWithConfig(path string, jpegQuality int) (*Device, error) {
	d, err := Open(path)
	if err != nil {
		return nil, err
	}

	// Set JPEG quality (clamp to valid range)
	if jpegQuality < 1 {
		jpegQuality = 1
	} else if jpegQuality > 100 {
		jpegQuality = 100
	}
	d.jpegQuality = jpegQuality

	return d, nil
}

// OpenFirst opens the first Stream Deck device found.
func OpenFirst() (*Device, error) {
	devices, err := Enumerate()
	if err != nil {
		return nil, err
	}
	if len(devices) == 0 {
		return nil, fmt.Errorf("no Stream Deck devices found")
	}
	return Open(devices[0].Path)
}

// Close closes the device.
// Acquires both readMu and writeMu to drain any in-flight HID operations first.
func (d *Device) Close() error {
	d.writeMu.Lock()
	defer d.writeMu.Unlock()
	d.readMu.Lock()
	defer d.readMu.Unlock()
	if d.hid != nil {
		return d.hid.Close()
	}
	return nil
}

// SetBrightness sets the brightness of the Stream Deck (0-100).
func (d *Device) SetBrightness(percent int) error {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}

	d.writeMu.Lock()
	defer d.writeMu.Unlock()

	data := make([]byte, 32)
	data[0] = 0x03
	data[1] = 0x08
	data[2] = byte(percent)

	_, err := d.hid.SendFeatureReport(data)
	return err
}

// Reset resets the Stream Deck to its default state.
func (d *Device) Reset() error {
	d.writeMu.Lock()
	defer d.writeMu.Unlock()

	data := make([]byte, 32)
	data[0] = 0x03
	data[1] = 0x02

	_, err := d.hid.SendFeatureReport(data)
	return err
}

// SetImage sets the image on a specific key.
func (d *Device) SetImage(keyIndex int, img image.Image) error {
	if keyIndex < 0 || keyIndex >= d.Model.Keys {
		return fmt.Errorf("key index %d out of range (0-%d)", keyIndex, d.Model.Keys-1)
	}
	if d.Model.PixelSize == 0 {
		return fmt.Errorf("device does not support images")
	}

	prepared := d.prepareImage(img)
	imageData, err := d.encodeImage(prepared)
	if err != nil {
		return err
	}

	d.writeMu.Lock()
	defer d.writeMu.Unlock()
	return d.writeImageData(keyIndex, imageData)
}

// EncodeKeyImage prepares and encodes an image for a key without holding the HID lock.
// Use together with WriteKeyData for parallel page rendering:
//
//	data, err := dev.EncodeKeyImage(img)   // concurrent-safe, no lock
//	dev.WriteKeyData(keyIndex, data)        // serialised HID write
func (d *Device) EncodeKeyImage(img image.Image) ([]byte, error) {
	if d.Model.PixelSize == 0 {
		return nil, fmt.Errorf("device does not support images")
	}
	prepared := d.prepareImage(img)
	return d.encodeImage(prepared)
}

// WriteKeyData writes pre-encoded image bytes to a key with the HID lock held.
// Pair with EncodeKeyImage for parallel encode -> serial write patterns.
func (d *Device) WriteKeyData(keyIndex int, imageData []byte) error {
	if keyIndex < 0 || keyIndex >= d.Model.Keys {
		return fmt.Errorf("key index %d out of range (0-%d)", keyIndex, d.Model.Keys-1)
	}
	d.writeMu.Lock()
	defer d.writeMu.Unlock()
	return d.writeImageData(keyIndex, imageData)
}

// prepareImage resizes and rotates the image for Stream Deck display.
func (d *Device) prepareImage(src image.Image) image.Image {
	size := d.Model.PixelSize
	bounds := src.Bounds()

	// Create destination image
	dst := image.NewRGBA(image.Rect(0, 0, size, size))

	// If source is correct size, just copy with rotation
	if bounds.Dx() == size && bounds.Dy() == size {
		// Rotate 180 degrees
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				dst.Set(size-1-x, size-1-y, src.At(bounds.Min.X+x, bounds.Min.Y+y))
			}
		}
		return dst
	}

	// Scale the image to fit
	scaleX := float64(bounds.Dx()) / float64(size)
	scaleY := float64(bounds.Dy()) / float64(size)

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			// Sample from source with rotation (180 degrees)
			srcX := int(float64(size-1-x) * scaleX)
			srcY := int(float64(size-1-y) * scaleY)
			dst.Set(x, y, src.At(bounds.Min.X+srcX, bounds.Min.Y+srcY))
		}
	}

	return dst
}

// encodeImage encodes the image to the appropriate format for this device.
func (d *Device) encodeImage(img image.Image) ([]byte, error) {
	var buf bytes.Buffer

	switch d.Model.ImageFormat {
	case "JPEG":
		quality := d.jpegQuality
		if quality == 0 {
			quality = 90 // default
		}
		err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality})
		if err != nil {
			return nil, fmt.Errorf("jpeg encode: %w", err)
		}
	case "BMP":
		// BMP encoding for older devices
		err := encodeBMP(&buf, img)
		if err != nil {
			return nil, fmt.Errorf("bmp encode: %w", err)
		}
	default:
		// Default to JPEG
		quality := d.jpegQuality
		if quality == 0 {
			quality = 90 // default
		}
		err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality})
		if err != nil {
			return nil, fmt.Errorf("jpeg encode: %w", err)
		}
	}

	return buf.Bytes(), nil
}

// writeImageData writes raw image data to a key.
func (d *Device) writeImageData(keyIndex int, imageData []byte) error {
	// Stream Deck MK.2/V2 uses 1024 byte pages with 8 byte header
	pageSize := 1024
	headerSize := 8
	payloadSize := pageSize - headerSize

	totalPages := (len(imageData) + payloadSize - 1) / payloadSize

	for page := 0; page < totalPages; page++ {
		start := page * payloadSize
		end := start + payloadSize
		if end > len(imageData) {
			end = len(imageData)
		}
		chunk := imageData[start:end]

		isLastPage := page == totalPages-1

		// Build the report
		report := make([]byte, pageSize)
		report[0] = 0x02           // Report ID for image
		report[1] = 0x07           // Command
		report[2] = byte(keyIndex) // Key index
		if isLastPage {
			report[3] = 0x01 // Last page flag
		} else {
			report[3] = 0x00
		}
		report[4] = byte(len(chunk) & 0xFF) // Payload length low byte
		report[5] = byte(len(chunk) >> 8)   // Payload length high byte
		report[6] = byte(page & 0xFF)       // Page number low byte
		report[7] = byte(page >> 8)         // Page number high byte

		copy(report[headerSize:], chunk)

		_, err := d.hid.Write(report)
		if err != nil {
			return fmt.Errorf("write page %d: %w", page, err)
		}
	}

	return nil
}

// Clear clears all keys on the Stream Deck (sets them to black).
func (d *Device) Clear() error {
	if d.Model.PixelSize == 0 {
		return nil // No display to clear
	}
	black := image.NewRGBA(image.Rect(0, 0, d.Model.PixelSize, d.Model.PixelSize))
	for i := 0; i < d.Model.Keys; i++ {
		if err := d.SetImage(i, black); err != nil {
			return fmt.Errorf("clear key %d: %w", i, err)
		}
	}
	return nil
}

// SetKeyColor sets a key to a solid color.
func (d *Device) SetKeyColor(keyIndex int, c color.Color) error {
	if d.Model.PixelSize == 0 {
		return fmt.Errorf("device does not support images")
	}
	size := d.Model.PixelSize
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(img, img.Bounds(), &image.Uniform{c}, image.Point{}, draw.Src)
	return d.SetImage(keyIndex, img)
}

// ResizeImage scales an image to fit the device's key size.
// Maintains aspect ratio and centers the image.
// OPTIMIZATION: Use Lanczos3 resampling for better quality at similar speed
func (d *Device) ResizeImage(src image.Image) image.Image {
	size := d.Model.PixelSize
	if size == 0 {
		return src
	}

	srcBounds := src.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()

	// If already correct size, return as-is
	if srcW == size && srcH == size {
		return src
	}

	// Create destination image
	dst := image.NewRGBA(image.Rect(0, 0, size, size))

	// Calculate scale to fit while maintaining aspect ratio
	scaleX := float64(size) / float64(srcW)
	scaleY := float64(size) / float64(srcH)
	scale := scaleX
	if scaleY < scaleX {
		scale = scaleY
	}

	newW := int(float64(srcW) * scale)
	newH := int(float64(srcH) * scale)

	// Center offset
	offsetX := (size - newW) / 2
	offsetY := (size - newH) / 2

	// Use bilinear interpolation for better quality (still fast)
	for y := 0; y < newH; y++ {
		srcY := float64(srcBounds.Min.Y) + float64(y)/scale
		for x := 0; x < newW; x++ {
			srcX := float64(srcBounds.Min.X) + float64(x)/scale

			// Bilinear interpolation
			x0 := int(srcX)
			y0 := int(srcY)
			x1 := min(x0+1, srcBounds.Max.X-1)
			y1 := min(y0+1, srcBounds.Max.Y-1)

			wx := srcX - float64(x0)
			wy := srcY - float64(y0)

			c00 := color.RGBAModel.Convert(src.At(x0, y0)).(color.RGBA)
			c10 := color.RGBAModel.Convert(src.At(x1, y0)).(color.RGBA)
			c01 := color.RGBAModel.Convert(src.At(x0, y1)).(color.RGBA)
			c11 := color.RGBAModel.Convert(src.At(x1, y1)).(color.RGBA)

			// Interpolate colors
			r := uint8((1-wx)*(1-wy)*float64(c00.R) + wx*(1-wy)*float64(c10.R) + (1-wx)*wy*float64(c01.R) + wx*wy*float64(c11.R))
			g := uint8((1-wx)*(1-wy)*float64(c00.G) + wx*(1-wy)*float64(c10.G) + (1-wx)*wy*float64(c01.G) + wx*wy*float64(c11.G))
			b := uint8((1-wx)*(1-wy)*float64(c00.B) + wx*(1-wy)*float64(c10.B) + (1-wx)*wy*float64(c01.B) + wx*wy*float64(c11.B))
			a := uint8((1-wx)*(1-wy)*float64(c00.A) + wx*(1-wy)*float64(c10.A) + (1-wx)*wy*float64(c01.A) + wx*wy*float64(c11.A))

			dst.Set(offsetX+x, offsetY+y, color.RGBA{r, g, b, a})
		}
	}

	return dst
}
