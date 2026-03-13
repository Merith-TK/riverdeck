package streamdeck

// text_image.go - package-level image-creation helpers shared by Navigator
// and LayoutNavigator.  Both exported method wrappers delegate here so the
// rendering logic lives in exactly one place.

import (
	"image"
	"image/color"
	"image/draw"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// createTextImageWithColors builds a solid-colour image with centred text.
// size is the pixel dimension of the square canvas.
func createTextImageWithColors(size int, text string, bgColor, textColor color.Color) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(img, img.Bounds(), &image.Uniform{bgColor}, image.Point{}, draw.Src)

	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(textColor),
		Face: basicfont.Face7x13,
	}
	textWidth := len(text) * 7 // basicfont ~7 px/char
	x := (size - textWidth) / 2
	if x < 2 {
		x = 2
	}
	y := size/2 + 4 // vertically centred
	d.Dot = fixed.Point26_6{X: fixed.I(x), Y: fixed.I(y)}
	d.DrawString(text)
	return img
}

// renderTextOnImage composites centred text over an already-rendered image
// without replacing the background.  The returned image is always a fresh
// *image.RGBA of the same dimensions as base.
func renderTextOnImage(size int, base image.Image, text string, textColor color.Color) image.Image {
	result := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(result, result.Bounds(), base, image.Point{}, draw.Src)

	d := &font.Drawer{
		Dst:  result,
		Src:  image.NewUniform(textColor),
		Face: basicfont.Face7x13,
	}
	textWidth := len(text) * 7
	x := (size - textWidth) / 2
	if x < 2 {
		x = 2
	}
	y := size/2 + 4
	d.Dot = fixed.Point26_6{X: fixed.I(x), Y: fixed.I(y)}
	d.DrawString(text)
	return result
}
