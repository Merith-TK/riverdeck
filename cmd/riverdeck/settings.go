package main

// settings.go - implements the settings overlay menu for the Stream Deck.
//
// The settings page is a virtual overlay (not a real folder) that appears when
// the user presses the reserved back/settings key while at the navigation root.
//
// Layout (5-col x 3-row MK.2 example):
//
//	Col 0 (reserved)  Col 1      Col 2      Col 3      Col 4
//	Row 0:  [BACK]   [EXIT]    [     ]   [     ]   [OPENDIR]
//	Row 1:  [     ]  [BRT-]   [B:XX%]   [BRT+]    [     ]
//	Row 2:  [     ]  [TMO-]   [T:XXs]   [TMO+]    [     ]
//
// Brightness steps: ±5, clamped to [5, 100].
// Timeout cycles:   0 (never) -> 30 -> 60 -> 120 -> 300 -> 0 ...

import (
	"fmt"
	"image/color"
	"log"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/merith-tk/riverdeck/pkg/streamdeck"
)

// timeoutValues is the ordered list of selectable timeout durations (seconds).
// 0 means "never sleep".
var timeoutValues = []int{0, 30, 60, 120, 300}

// Settings content-key slot indices (positions within contentKeys slice).
// Slots map to content keys left-to-right, row by row, skipping col-0 reserved keys.
const (
	// Row 0 - system buttons
	sSlotExit = 0 // EXIT    (row 0, col 1)
	// slot 1 intentionally empty
	sSlotReload  = 2 // RELOAD  (row 0, col 3)
	sSlotOpenDir = 3 // CFGDIR  (row 0, col 4 - top-right)

	// Row 1 - brightness
	sSlotBrtDown = 4 // BRT-
	sSlotBrtVal  = 5 // B:XX%  (display only)
	sSlotBrtUp   = 6 // BRT+
	// slot 7 empty

	// Row 2 - timeout
	sSlotTmoDown = 8  // TMO-
	sSlotTmoVal  = 9  // timeout value display
	sSlotTmoUp   = 10 // TMO+
	// slot 11 empty
)

// enterSettings switches the App into settings mode and renders the settings page.
func (a *App) enterSettings() {
	a.inSettings = true
	fmt.Println("[*] Entering settings menu")
	a.renderSettingsPage()
}

// exitSettings leaves settings mode and returns to the normal navigation page.
func (a *App) exitSettings() {
	a.inSettings = false
	a.exitConfirming = false
	fmt.Println("[*] Exiting settings menu")

	// Re-render the regular navigation page
	if err := a.nav.RenderPage(); err != nil {
		log.Printf("RenderPage after settings exit: %v", err)
	}
	a.updateVisibleScripts()
}

// renderSettingsPage draws all settings keys on the Stream Deck.
// It is called every time a setting changes so the display stays in sync.
func (a *App) renderSettingsPage() {
	contentKeys := a.nav.GetContentKeys()

	// Black-out all keys first via the device's clear helper, then paint ours.
	totalKeys := a.device.Model.Keys
	for i := 0; i < totalKeys; i++ {
		a.device.SetKeyColor(i, color.RGBA{0, 0, 0, 255})
	}

	// Reserved col-0 key: back arrow to exit settings
	backImg := a.nav.CreateTextImageWithColors("<-", color.RGBA{100, 100, 100, 255}, color.White)
	a.device.SetImage(streamdeck.KeyBack, backImg)

	// T1 / T2 are page-scroll arrows for settings.
	// Currently there is only one settings page so they are shown dimmed.
	const totalSettingsPages = 1
	if a.settingsPage > 0 {
		t1Img := a.nav.CreateTextImageWithColors("PG^", color.RGBA{80, 80, 80, 255}, color.White)
		a.device.SetImage(streamdeck.KeyToggle1, t1Img)
	} else {
		t1Img := a.nav.CreateTextImageWithColors("PG^", color.RGBA{30, 30, 30, 255}, color.RGBA{80, 80, 80, 255})
		a.device.SetImage(streamdeck.KeyToggle1, t1Img)
	}
	if a.settingsPage < totalSettingsPages-1 {
		t2Img := a.nav.CreateTextImageWithColors("PGv", color.RGBA{80, 80, 80, 255}, color.White)
		a.device.SetImage(streamdeck.KeyToggle2, t2Img)
	} else {
		t2Img := a.nav.CreateTextImageWithColors("PG▼", color.RGBA{30, 30, 30, 255}, color.RGBA{80, 80, 80, 255})
		a.device.SetImage(streamdeck.KeyToggle2, t2Img)
	}

	// Helper to set a content key by slot index
	setSlot := func(slot int, text string, bg, fg color.RGBA) {
		if slot >= len(contentKeys) {
			return
		}
		img := a.nav.CreateTextImageWithColors(text, bg, fg)
		a.device.SetImage(contentKeys[slot], img)
	}

	// -- System row (row 0) ----------------------------------------------------
	if a.exitConfirming {
		setSlot(sSlotExit, "SURE?", color.RGBA{200, 0, 0, 255}, color.RGBA{255, 220, 220, 255})
	} else {
		setSlot(sSlotExit, "EXIT", color.RGBA{140, 20, 20, 255}, color.RGBA{255, 180, 180, 255})
	}
	setSlot(sSlotReload, "RELOAD", color.RGBA{20, 100, 20, 255}, color.RGBA{160, 255, 160, 255})
	setSlot(sSlotOpenDir, "CFGDIR", color.RGBA{20, 80, 80, 255}, color.RGBA{160, 230, 230, 255})

	// -- Brightness row (row 1) ------------------------------------------------
	setSlot(sSlotBrtDown, "BRT-", color.RGBA{40, 40, 120, 255}, color.RGBA{160, 160, 255, 255})
	setSlot(sSlotBrtVal,
		fmt.Sprintf("B:%d%%", a.config.Application.Brightness),
		color.RGBA{20, 20, 60, 255}, color.RGBA{200, 200, 255, 255})
	setSlot(sSlotBrtUp, "BRT+", color.RGBA{40, 40, 120, 255}, color.RGBA{160, 160, 255, 255})

	// -- Timeout row (row 2) ---------------------------------------------------
	setSlot(sSlotTmoDown, "TMO-", color.RGBA{40, 80, 40, 255}, color.RGBA{160, 255, 160, 255})
	tmoText := fmtTimeout(a.config.Application.Timeout)
	setSlot(sSlotTmoVal, tmoText, color.RGBA{20, 40, 20, 255}, color.RGBA{160, 255, 160, 255})
	setSlot(sSlotTmoUp, "TMO+", color.RGBA{40, 80, 40, 255}, color.RGBA{160, 255, 160, 255})
}

// handleSettingsKeyEvent processes a key press while in settings mode.
func (a *App) handleSettingsKeyEvent(keyIndex int) error {
	// Back key: leave settings
	if keyIndex == streamdeck.KeyBack {
		a.exitSettings()
		return nil
	}

	// T1/T2 scroll through settings pages (future expansion; no-op on single page)
	const totalSettingsPages = 1
	if keyIndex == streamdeck.KeyToggle1 {
		if a.settingsPage > 0 {
			a.settingsPage--
			a.renderSettingsPage()
		}
		return nil
	}
	if keyIndex == streamdeck.KeyToggle2 {
		if a.settingsPage < totalSettingsPages-1 {
			a.settingsPage++
			a.renderSettingsPage()
		}
		return nil
	}

	contentKeys := a.nav.GetContentKeys()

	// Map the physical key index to a slot index
	slot := -1
	for i, ck := range contentKeys {
		if ck == keyIndex {
			slot = i
			break
		}
	}

	switch slot {
	case sSlotExit:
		// handled above (double-press confirm)
	default:
		// Any other key press cancels a pending exit confirmation.
		if a.exitConfirming {
			a.exitConfirming = false
			a.renderSettingsPage()
			return nil
		}
	}

	switch slot {
	case sSlotExit:
		if !a.exitConfirming {
			// First press: ask for confirmation.
			a.exitConfirming = true
			a.renderSettingsPage()
			// Auto-cancel confirmation after 3 s.
			go func() {
				time.Sleep(3 * time.Second)
				if a.exitConfirming {
					a.exitConfirming = false
					a.renderSettingsPage()
				}
			}()
		} else {
			// Second press: confirmed - flash only the EXIT key, then quit.
			fmt.Println("[*] EXIT confirmed - shutting down")
			contentKeys := a.nav.GetContentKeys()
			if sSlotExit < len(contentKeys) {
				img := a.nav.CreateTextImageWithColors("BYE",
					color.RGBA{180, 0, 0, 255},
					color.RGBA{255, 200, 200, 255})
				a.device.SetImage(contentKeys[sSlotExit], img)
			}
			time.Sleep(500 * time.Millisecond)
			a.cancel()
		}
		return nil
	case sSlotReload:
		fmt.Println("[*] RELOAD pressed - restarting")
		contentKeys := a.nav.GetContentKeys()
		if sSlotReload < len(contentKeys) {
			img := a.nav.CreateTextImageWithColors("...",
				color.RGBA{80, 60, 0, 255},
				color.RGBA{255, 210, 80, 255})
			a.device.SetImage(contentKeys[sSlotReload], img)
		}
		time.Sleep(300 * time.Millisecond)
		a.restartRequested = true
		a.cancel()
		return nil
	case sSlotOpenDir:
		fmt.Printf("[*] Opening config directory: %s\n", a.configPath)
		if err := openConfigDir(a.configPath); err != nil {
			log.Printf("openConfigDir: %v", err)
		}
		return nil
	case sSlotBrtDown:
		a.adjustBrightness(-5)
	case sSlotBrtUp:
		a.adjustBrightness(+5)
	case sSlotTmoDown:
		a.stepTimeout(-1)
	case sSlotTmoUp:
		a.stepTimeout(+1)
	default:
		// Unbound key - ignore
		return nil
	}

	// Persist config after any change
	a.persistConfig()
	// Refresh the settings display
	a.renderSettingsPage()
	return nil
}

// adjustBrightness changes brightness by delta, clamped to [5, 100], and
// immediately applies it to the device.
func (a *App) adjustBrightness(delta int) {
	v := a.config.Application.Brightness + delta
	if v < 5 {
		v = 5
	}
	if v > 100 {
		v = 100
	}
	a.config.Application.Brightness = v
	if err := a.device.SetBrightness(v); err != nil {
		log.Printf("SetBrightness: %v", err)
	}
	fmt.Printf("[*] Brightness -> %d%%\n", v)
}

// stepTimeout advances (delta=+1) or retreats (delta=-1) through timeoutValues.
func (a *App) stepTimeout(delta int) {
	current := a.config.Application.Timeout
	idx := 0
	for i, v := range timeoutValues {
		if v == current {
			idx = i
			break
		}
	}
	idx = (idx + delta + len(timeoutValues)) % len(timeoutValues)
	a.config.Application.Timeout = timeoutValues[idx]
	fmt.Printf("[*] Timeout -> %s\n", fmtTimeout(a.config.Application.Timeout))
	// Reset the sleep timer with the new value
	a.resetSleepTimer()
}

// persistConfig writes the current config to disk.
func (a *App) persistConfig() {
	cfgFile := filepath.Join(a.configPath, "config.yml")
	if err := SaveConfig(a.config, cfgFile); err != nil {
		log.Printf("SaveConfig: %v", err)
	}
}

// fmtTimeout returns a human-readable label for a timeout value in seconds.
func fmtTimeout(seconds int) string {
	if seconds == 0 {
		return "T:OFF"
	}
	if seconds < 60 {
		return fmt.Sprintf("T:%ds", seconds)
	}
	return fmt.Sprintf("T:%dm", seconds/60)
}

// openConfigDir opens the given directory in the system's default file manager
// (Explorer on Windows, Finder/open on macOS, xdg-open on Linux).
func openConfigDir(dir string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", filepath.ToSlash(dir))
	case "darwin":
		cmd = exec.Command("open", dir)
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	return cmd.Start()
}
