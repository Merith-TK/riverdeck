package main

import (
	"fyne.io/fyne/v2"
	"github.com/merith-tk/riverdeck/resources"
)

// resourceIconSvg is the application icon, sourced from resources/icon.svg
// and used for both the process icon and the systray icon.
var resourceIconSvg = &fyne.StaticResource{
	StaticName:    "icon.svg",
	StaticContent: resources.IconSVG,
}
