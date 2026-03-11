package streamdeck

import (
	"bytes"
	"image"
)

// encodeBMP encodes an image to BMP format for older Stream Deck devices.
func encodeBMP(w *bytes.Buffer, img image.Image) error {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// BMP row size must be aligned to 4 bytes
	rowSize := ((width*3 + 3) / 4) * 4
	imageSize := rowSize * height
	fileSize := 54 + imageSize // 54 = header size

	// BMP File Header (14 bytes)
	w.Write([]byte{'B', 'M'})      // Magic number
	writeLE32(w, uint32(fileSize)) // File size
	writeLE16(w, 0)                // Reserved
	writeLE16(w, 0)                // Reserved
	writeLE32(w, 54)               // Offset to pixel data

	// DIB Header (40 bytes - BITMAPINFOHEADER)
	writeLE32(w, 40)                // Header size
	writeLE32(w, uint32(width))     // Width
	writeLE32(w, uint32(height))    // Height (positive = bottom-up)
	writeLE16(w, 1)                 // Color planes
	writeLE16(w, 24)                // Bits per pixel
	writeLE32(w, 0)                 // Compression (none)
	writeLE32(w, uint32(imageSize)) // Image size
	writeLE32(w, 2835)              // Horizontal resolution (72 DPI)
	writeLE32(w, 2835)              // Vertical resolution (72 DPI)
	writeLE32(w, 0)                 // Colors in palette
	writeLE32(w, 0)                 // Important colors

	// Pixel data (bottom-up, BGR format)
	row := make([]byte, rowSize)
	for y := height - 1; y >= 0; y-- {
		for x := 0; x < width; x++ {
			r, g, b, _ := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			row[x*3+0] = byte(b >> 8) // B
			row[x*3+1] = byte(g >> 8) // G
			row[x*3+2] = byte(r >> 8) // R
		}
		w.Write(row)
	}

	return nil
}

func writeLE16(w *bytes.Buffer, v uint16) {
	w.WriteByte(byte(v))
	w.WriteByte(byte(v >> 8))
}

func writeLE32(w *bytes.Buffer, v uint32) {
	w.WriteByte(byte(v))
	w.WriteByte(byte(v >> 8))
	w.WriteByte(byte(v >> 16))
	w.WriteByte(byte(v >> 24))
}
