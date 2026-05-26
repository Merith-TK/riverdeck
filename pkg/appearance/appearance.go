package appearance

import (
	"image"
	"image/color"
	"image/draw"

	"github.com/merith-tk/riverdeck/pkg/imaging"
	"github.com/merith-tk/riverdeck/pkg/scripting"
	"github.com/merith-tk/riverdeck/pkg/streamdeck"
)

// ApplyKeyAppearance renders a KeyAppearance onto a deck key using layered compositing.
//
// Render order (bottom to top):
//  1. Solid color fill (appearance.Color)
//  2. Icon image composited over the color (appearance.Icon), preserving alpha so
//     transparent icon areas reveal the color layer beneath
//  3. Text drawn on top of everything (appearance.Text)
func ApplyKeyAppearance(dev streamdeck.DeviceIface, nav streamdeck.NavigatorIface, keyIndex int, appearance *scripting.KeyAppearance) {
	if appearance == nil {
		return
	}

	bgColor := color.RGBA{
		R: uint8(appearance.Color[0]),
		G: uint8(appearance.Color[1]),
		B: uint8(appearance.Color[2]),
		A: 255,
	}
	textColor := color.RGBA{
		R: uint8(appearance.TextColor[0]),
		G: uint8(appearance.TextColor[1]),
		B: uint8(appearance.TextColor[2]),
		A: 255,
	}

	if appearance.Icon != "" {
		if iconImg, loadErr := imaging.LoadImage(appearance.Icon); loadErr == nil {
			// Step 1: create solid color background.
			size := dev.PixelSize()
			base := image.NewRGBA(image.Rect(0, 0, size, size))
			draw.Draw(base, base.Bounds(), &image.Uniform{bgColor}, image.Point{}, draw.Src)

			// Step 2: composite resized icon over the color background.
			resized := dev.ResizeImage(iconImg)
			draw.Draw(base, base.Bounds(), resized, image.Point{}, draw.Over)

			// Step 3: optionally draw text on top.
			var final image.Image = base
			if appearance.Text != "" {
				final = nav.RenderTextOnImage(base, appearance.Text, textColor)
			}
			_ = dev.SetImage(keyIndex, final)
			return
		}
		// Icon failed to load — fall through to color+text rendering.
	}

	// No icon: plain color background with optional text.
	if appearance.Text != "" {
		img := nav.CreateTextImageWithColors(appearance.Text, bgColor, textColor)
		_ = dev.SetImage(keyIndex, img)
	} else {
		_ = dev.SetKeyColor(keyIndex, bgColor)
	}
}
