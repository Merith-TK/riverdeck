package main

import (
	"log"
	"os"

	"github.com/merith-tk/riverdeck/pkg/layout"
	"github.com/merith-tk/riverdeck/pkg/streamdeck"
	"github.com/merith-tk/riverdeck/pkg/wsdevice"
)

// runWSDevice drives the layout session for a single WebSocket software client.
// It is called from the WS server's onConnect callback and blocks until the
// client disconnects.
//
// Each client gets its own layout.json stored at:
//
//	configPath/devices/{uuid}/layout.json
//
// On first connection the layout is seeded from the root layout.json (if it
// exists) so the client starts with a usable configuration.
//
// Script execution is not supported for WS clients in this iteration; buttons
// with action:"page"/"back"/"home" still navigate the layout normally.
func (a *App) runWSDevice(dev *wsdevice.Device) {
	uuid := dev.UUID()
	devDir := layout.DeviceLayoutDir(a.configPath, uuid)

	if err := os.MkdirAll(devDir, 0755); err != nil {
		log.Printf("[wsdevice] failed to create device dir %s: %v", devDir, err)
		return
	}

	// Seed a layout.json for this device on first connection.
	if !layout.Exists(devDir) {
		if rootLay, _ := layout.Load(a.configPath); rootLay != nil {
			if err := layout.Save(devDir, rootLay); err != nil {
				log.Printf("[wsdevice] failed to seed layout for %s: %v", uuid, err)
			}
		} else {
			if err := layout.Save(devDir, layout.NewEmpty()); err != nil {
				log.Printf("[wsdevice] failed to create empty layout for %s: %v", uuid, err)
			}
		}
	}

	lay, err := layout.Load(devDir)
	if err != nil {
		log.Printf("[wsdevice] layout load error for %s: %v", uuid, err)
		lay = layout.NewEmpty()
	}
	if lay == nil {
		lay = layout.NewEmpty()
	}

	// Use the global configPath for script resolution so that pkg:// URIs and
	// relative script paths work correctly.  The layout file itself lives in
	// devDir, but everything else lives under the shared config root.
	nav := streamdeck.NewLayoutNavigator(dev, a.configPath, lay)

	if err := dev.SetBrightness(a.config.Application.Brightness); err != nil {
		log.Printf("[wsdevice] SetBrightness error: %v", err)
	}
	if err := nav.RenderPage(); err != nil {
		log.Printf("[wsdevice] initial RenderPage error: %v", err)
	}

	log.Printf("[wsdevice] session started uuid=%s", uuid)

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
