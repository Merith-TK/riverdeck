package main

import (
	"log"

	"github.com/merith-tk/riverdeck/pkg/layout"
	"github.com/merith-tk/riverdeck/pkg/streamdeck"
	"github.com/merith-tk/riverdeck/pkg/wsdevice"
)

// runWSDevice drives the layout session for a single WebSocket software client.
// It is called from the WS server's onConnect callback and blocks until the
// client disconnects.
//
// The layout for the client is resolved from layout.json by device UUID.
// If no explicit assignment exists the "default" layout is used, allowing
// multiple software clients to share layouts with hardware devices.
//
// Script execution is not supported for WS clients in this iteration; buttons
// with action:"page"/"back"/"home" still navigate the layout normally.
func (a *App) runWSDevice(dev *wsdevice.Device) {
	uuid := dev.UUID()

	lay, err := layout.LoadForDevice(a.configPath, uuid)
	if err != nil {
		log.Printf("[wsdevice] layout load error uuid=%s: %v", uuid, err)
		lay = layout.NewEmpty()
	}

	nav := streamdeck.NewLayoutNavigator(dev, a.configPath, lay)

	if err := dev.SetBrightness(a.config.Application.Brightness); err != nil {
		log.Printf("[wsdevice] SetBrightness error: %v", err)
	}
	if err := nav.RenderPage(); err != nil {
		log.Printf("[wsdevice] initial RenderPage error: %v", err)
	}

	log.Printf("[wsdevice] session started uuid=%s layout=%d pages", uuid, len(lay.Pages))

	events := make(chan streamdeck.KeyEvent, 10)
	dev.ListenKeys(dev.Context(), events)

	for event := range events {
		if !event.Pressed {
			continue
		}
		item, navigated, kErr := nav.HandleKeyPress(event.Key)
		if kErr != nil {
			log.Printf("[wsdevice] key error uuid=%s key=%d: %v", uuid, event.Key, kErr)
			continue
		}
		if navigated {
			if rErr := nav.RenderPage(); rErr != nil {
				log.Printf("[wsdevice] RenderPage error uuid=%s: %v", uuid, rErr)
			}
		}
		if item != nil && item.Script != "" {
			// Script execution for WS clients is not yet implemented.
			log.Printf("[wsdevice] script button (not executed) uuid=%s script=%s", uuid, item.Script)
		}
	}

	log.Printf("[wsdevice] session ended uuid=%s", uuid)
}
