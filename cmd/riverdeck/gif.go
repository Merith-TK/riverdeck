package main

// gif.go — GIF animation management for per-key animated images.
//
// Extracted from app.go for clarity. All methods operate on *App.

import (
	"context"
	"image/color"
	"log"
	"time"

	"github.com/merith-tk/riverdeck/pkg/imaging"
	"github.com/merith-tk/riverdeck/pkg/scripting"
)

// stopGIFAnim cancels any running GIF animation goroutine for keyIndex.
func (a *App) stopGIFAnim(keyIndex int) {
	a.gifAnimsMu.Lock()
	defer a.gifAnimsMu.Unlock()
	if cancel, ok := a.gifAnims[keyIndex]; ok {
		cancel()
		delete(a.gifAnims, keyIndex)
	}
}

// startGIFAnim loads an animated GIF and starts a goroutine that cycles
// its frames onto keyIndex at the GIF's native frame rate.
// The appearance is captured so that any text/text_color settings are
// composited on top of every GIF frame.
// Any previously running animation for that key is cancelled first.
func (a *App) startGIFAnim(keyIndex int, appearance *scripting.KeyAppearance) {
	data, err := imaging.LoadGIFFrames(appearance.Image)
	if err != nil {
		log.Printf("GIF load failed for key %d: %v", keyIndex, err)
		return
	}
	if len(data.Frames) == 0 {
		return
	}

	// Snapshot the text fields so the goroutine doesn't race on the struct.
	text := appearance.Text
	textColor := color.RGBA{
		R: uint8(appearance.TextColor[0]),
		G: uint8(appearance.TextColor[1]),
		B: uint8(appearance.TextColor[2]),
		A: 255,
	}

	// Cancel any existing animation for this key.
	a.gifAnimsMu.Lock()
	if cancel, ok := a.gifAnims[keyIndex]; ok {
		cancel()
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.gifAnims[keyIndex] = cancel
	a.gifAnimsMu.Unlock()

	go func() {
		// frameTarget tracks when the current frame should have been shown.
		// Using a reference point prevents processing-time drift from
		// accumulating and causing the effective FPS to fall below the GIF's
		// encoded rate.
		frameTarget := time.Now()
		for {
			for i, frame := range data.Frames {
				select {
				case <-ctx.Done():
					return
				default:
				}

				// Skip frame if display is asleep or settings overlay is active.
				a.sleepMu.Lock()
				isSleeping := a.sleeping
				a.sleepMu.Unlock()
				if !isSleeping && !a.inSettings {
					resized := a.device.ResizeImage(frame)
					// Overlay text on the GIF frame when the script specifies it.
					if text != "" {
						resized = a.nav.RenderTextOnImage(resized, text, textColor)
					}
					_ = a.device.SetImage(keyIndex, resized)
				}

				// Per-frame delay: GIF delays are in centiseconds (×10 = ms).
				// Default to 1 centisecond (10 ms) when the frame has no delay
				// so that fast/untagged GIFs play at their intended speed.
				delay := 1
				if i < len(data.Delays) && data.Delays[i] > 0 {
					delay = data.Delays[i]
				}
				frameTarget = frameTarget.Add(time.Duration(delay) * 10 * time.Millisecond)
				sleepFor := time.Until(frameTarget)
				if sleepFor > 0 {
					select {
					case <-ctx.Done():
						return
					case <-time.After(sleepFor):
					}
				} else {
					// We're running behind – yield briefly and continue.
					select {
					case <-ctx.Done():
						return
					default:
					}
				}
			}
		}
	}()
}

// stopAllGIFAnims cancels every running GIF animation goroutine.
// Call this before navigating to a new page or on shutdown.
func (a *App) stopAllGIFAnims() {
	a.gifAnimsMu.Lock()
	defer a.gifAnimsMu.Unlock()
	for key, cancel := range a.gifAnims {
		cancel()
		delete(a.gifAnims, key)
	}
}
