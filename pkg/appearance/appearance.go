package appearance

import (
	"image/color"

	"github.com/merith-tk/riverdeck/pkg/imaging"
	"github.com/merith-tk/riverdeck/pkg/scripting"
	"github.com/merith-tk/riverdeck/pkg/streamdeck"
)

func ApplyKeyAppearance(dev streamdeck.DeviceIface, nav streamdeck.NavigatorIface, keyIndex int, appearance *scripting.KeyAppearance) {
	if appearance == nil {
		return
	}
	if appearance.Image != "" {
		if img, loadErr := imaging.LoadImage(appearance.Image); loadErr == nil {
			_ = dev.SetImage(keyIndex, dev.ResizeImage(img))
			return
		}
	}
	c := color.RGBA{
		R: uint8(appearance.Color[0]),
		G: uint8(appearance.Color[1]),
		B: uint8(appearance.Color[2]),
		A: 255,
	}
	if appearance.Text != "" {
		img := nav.CreateTextImageWithColors(appearance.Text, c, color.RGBA{
			R: uint8(appearance.TextColor[0]),
			G: uint8(appearance.TextColor[1]),
			B: uint8(appearance.TextColor[2]),
			A: 255,
		})
		_ = dev.SetImage(keyIndex, img)
	} else {
		_ = dev.SetKeyColor(keyIndex, c)
	}
}
