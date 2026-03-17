package main

import (
	"image/color"
	"log"

	"github.com/merith-tk/riverdeck/pkg/imaging"
	"github.com/merith-tk/riverdeck/pkg/layout"
	"github.com/merith-tk/riverdeck/pkg/scripting"
	"github.com/merith-tk/riverdeck/pkg/streamdeck"
	"github.com/merith-tk/riverdeck/pkg/wsdevice"
)

// runWSDevice drives the layout session for a single WebSocket software client.
// It is called from the WS server's onConnect callback and blocks until the
// client disconnects.
//
// Each session gets its own ScriptManager bound to the WS device so that
// script passive loops, triggers, and background workers run independently
// per client — identical to how a hardware device session works.
func (a *App) runWSDevice(dev *wsdevice.Device) {
	id := dev.ID()

	lay, err := layout.LoadForDevice(a.configPath, id)
	if err != nil {
		log.Printf("[wsdevice] layout load error id=%s: %v", id, err)
		lay = layout.NewEmpty()
	}

	nav := streamdeck.NewLayoutNavigator(dev, a.configPath, lay)

	// Boot a script manager bound to this WS device.
	scriptMgr := scripting.NewScriptManager(dev, a.configPath, a.config.Application.PassiveFPS)
	if err := scriptMgr.Boot(dev.Context()); err != nil {
		log.Printf("[wsdevice] script boot error id=%s: %v", id, err)
	}
	defer scriptMgr.Shutdown()

	// Wire key-update callbacks so passive/trigger results paint onto the WS device.
	scriptMgr.SetKeyUpdateCallback(func(keyIndex int, appearance *scripting.KeyAppearance) {
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
	})

	nav.SetScriptValidator(scriptMgr.IsUsableScript)

	if err := nav.RenderPage(); err != nil {
		log.Printf("[wsdevice] initial RenderPage error: %v", err)
	}
	scriptMgr.SetVisibleScripts(nav.GetVisibleScripts())
	scriptMgr.StartPassiveLoop()

	log.Printf("[wsdevice] session started id=%s layout=%d pages", id, len(lay.Pages))

	events := make(chan streamdeck.KeyEvent, 10)
	dev.ListenKeys(dev.Context(), events)

	for event := range events {
		if !event.Pressed {
			continue
		}
		log.Printf("[wsdevice] key press id=%s key=%d", id, event.Key)
		item, navigated, kErr := nav.HandleKeyPress(event.Key)
		if kErr != nil {
			log.Printf("[wsdevice] key error id=%s key=%d: %v", id, event.Key, kErr)
			continue
		}
		if navigated {
			scriptMgr.SetVisibleScripts(nil)
			log.Printf("[wsdevice] navigated, rendering page id=%s", id)
			if rErr := nav.RenderPage(); rErr != nil {
				log.Printf("[wsdevice] RenderPage error id=%s: %v", id, rErr)
			} else {
				log.Printf("[wsdevice] RenderPage ok id=%s", id)
			}
			scriptMgr.SetVisibleScripts(nav.GetVisibleScripts())
		}
		if item != nil && item.Script != "" {
			scriptPath := item.Script
			go func() {
				if err := scriptMgr.TriggerScript(scriptPath); err != nil {
					log.Printf("[wsdevice] script error id=%s: %v", id, err)
				}
				scriptMgr.RefreshScript(scriptPath)
			}()
		}
	}

	log.Printf("[wsdevice] session ended id=%s", id)
}
