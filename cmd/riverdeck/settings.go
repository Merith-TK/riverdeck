package main

// settings.go - implements the settings overlay menu for the Stream Deck.
//
// The settings page is a virtual overlay (not a real folder) that appears when
// the user presses the reserved back/settings key while at the navigation root.
//
// Layout adapts automatically to the connected device width. For illustration,
// a 5-col MK.2 example (cc = contentCols = cols-1 = 4) looks like:
//
//	Col 0 (reserved)  Col 1      Col 2      Col 3      Col 4
//	Row 0:  [BACK]   [EXIT]    [     ]   [RELOAD]  [CFGDIR]
//	Row 1:  [T1  ]   [BRT-]   [B:XX%]   [BRT+]    [     ]
//	Row 2:  [T2  ]   [TMO-]   [T:XXs]   [TMO+]    [     ]
//
// On an XL (8 cols, cc=7):
//
//	Row 0:  [BACK]  [EXIT] [ ] [RELOAD] [ ] [ ] [ ] [CFGDIR]
//	Row 1:  [T1  ]  [BRT-] [B:XX%] [BRT+] ...
//	Row 2:  [T2  ]  [TMO-] [T:XXs] [TMO+] ...
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
)

// timeoutValues is the ordered list of selectable timeout durations (seconds).
// 0 means "never sleep".
var timeoutValues = []int{0, 30, 60, 120, 300}

// settingsLayout holds dynamically-computed content-key slot indices for the
// settings screen. Slots index into the contentKeys slice (left-to-right, row
// by row, col-0 reserved keys excluded).
//
// Layout (per row, where cc = contentCols = device.Cols()-1):
//
//	Row 0 (system)    : EXIT=0, <empty>=1, RELOAD=2, ..., CFGDIR=cc-1
//	Row 1 (brightness): BRT-=cc,   B:XX%=cc+1, BRT+=cc+2
//	Row 2 (timeout)   : TMO-=2cc, T:XXs=2cc+1, TMO+=2cc+2
type settingsLayout struct {
	exit    int
	reload  int
	openDir int
	brtDown int
	brtVal  int
	brtUp   int
	tmoDown int
	tmoVal  int
	tmoUp   int
}

// calcSettingsLayout returns slot indices computed for the current device geometry.
// All slot indices are into the contentKeys slice (col-0 reserved keys excluded).
//
// For very narrow devices (cc < 3) there is not enough room on row 0 for both
// RELOAD and CFGDIR; in that case RELOAD is placed on row 1 and the brightness
// controls shift to row 2. On cc >= 3 everything fits on its natural row.
func (a *App) calcSettingsLayout() settingsLayout {
	cc := a.device.Cols() - 1 // content columns per row
	if cc < 1 {
		cc = 1
	}

	// Row offsets
	row0 := 0
	row1 := cc
	row2 := cc * 2

	sl := settingsLayout{
		exit:    row0 + 0,
		openDir: row0 + cc - 1,
	}

	if cc >= 3 {
		// Enough room: RELOAD on row 0, brightness on row 1, timeout on row 2.
		sl.reload = row0 + 2
		sl.brtDown = row1 + 0
		sl.brtVal = row1 + 1
		sl.brtUp = row1 + 2
		sl.tmoDown = row2 + 0
		sl.tmoVal = row2 + 1
		sl.tmoUp = row2 + 2
	} else {
		// Narrow device (e.g. Mini 3-col, cc=2): push everything down one row.
		sl.reload = row1 + 0
		sl.brtDown = row2 + 0
		sl.brtVal = row2 + 1
		sl.brtUp = row2 + cc - 1 // last of row 2 if only 2 content cols
		sl.tmoDown = cc*3 + 0
		sl.tmoVal = cc*3 + 1
		sl.tmoUp = cc*3 + cc - 1
	}
	return sl
}

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
// Slot positions are computed dynamically so the layout adapts to any device
// width (Mini, MK.2/Original, XL, etc.).
func (a *App) renderSettingsPage() {
	sl := a.calcSettingsLayout()
	contentKeys := a.nav.GetContentKeys()

	// Black-out all keys first via the device's clear helper, then paint ours.
	totalKeys := a.device.Keys()
	for i := 0; i < totalKeys; i++ {
		a.device.SetKeyColor(i, color.RGBA{0, 0, 0, 255})
	}

	// Reserved col-0 key: back arrow to exit settings
	backImg := a.nav.CreateTextImageWithColors("<-", color.RGBA{100, 100, 100, 255}, color.White)
	a.device.SetImage(a.nav.BackKey(), backImg)

	// T1 / T2 are page-scroll arrows for settings.
	// Currently there is only one settings page so they are shown dimmed.
	const totalSettingsPages = 1
	if a.settingsPage > 0 {
		t1Img := a.nav.CreateTextImageWithColors("PG^", color.RGBA{80, 80, 80, 255}, color.White)
		a.device.SetImage(a.nav.Toggle1Key(), t1Img)
	} else {
		t1Img := a.nav.CreateTextImageWithColors("PG^", color.RGBA{30, 30, 30, 255}, color.RGBA{80, 80, 80, 255})
		a.device.SetImage(a.nav.Toggle1Key(), t1Img)
	}
	if a.settingsPage < totalSettingsPages-1 {
		t2Img := a.nav.CreateTextImageWithColors("PGv", color.RGBA{80, 80, 80, 255}, color.White)
		a.device.SetImage(a.nav.Toggle2Key(), t2Img)
	} else {
		t2Img := a.nav.CreateTextImageWithColors("PG▼", color.RGBA{30, 30, 30, 255}, color.RGBA{80, 80, 80, 255})
		a.device.SetImage(a.nav.Toggle2Key(), t2Img)
	}

	// Helper to set a content key by slot index.
	// Slot -1 means the control is disabled on this device size.
	setSlot := func(slot int, text string, bg, fg color.RGBA) {
		if slot < 0 || slot >= len(contentKeys) {
			return
		}
		img := a.nav.CreateTextImageWithColors(text, bg, fg)
		a.device.SetImage(contentKeys[slot], img)
	}

	// -- System row (row 0) ----------------------------------------------------
	if a.exitConfirming {
		setSlot(sl.exit, "SURE?", color.RGBA{200, 0, 0, 255}, color.RGBA{255, 220, 220, 255})
	} else {
		setSlot(sl.exit, "EXIT", color.RGBA{140, 20, 20, 255}, color.RGBA{255, 180, 180, 255})
	}
	setSlot(sl.reload, "RELOAD", color.RGBA{20, 100, 20, 255}, color.RGBA{160, 255, 160, 255})
	setSlot(sl.openDir, "CFGDIR", color.RGBA{20, 80, 80, 255}, color.RGBA{160, 230, 230, 255})

	// -- Brightness row (row 1) ------------------------------------------------
	setSlot(sl.brtDown, "BRT-", color.RGBA{40, 40, 120, 255}, color.RGBA{160, 160, 255, 255})
	setSlot(sl.brtVal,
		fmt.Sprintf("B:%d%%", a.config.Application.Brightness),
		color.RGBA{20, 20, 60, 255}, color.RGBA{200, 200, 255, 255})
	setSlot(sl.brtUp, "BRT+", color.RGBA{40, 40, 120, 255}, color.RGBA{160, 160, 255, 255})

	// -- Timeout row (row 2) ---------------------------------------------------
	setSlot(sl.tmoDown, "TMO-", color.RGBA{40, 80, 40, 255}, color.RGBA{160, 255, 160, 255})
	tmoText := fmtTimeout(a.config.Application.Timeout)
	setSlot(sl.tmoVal, tmoText, color.RGBA{20, 40, 20, 255}, color.RGBA{160, 255, 160, 255})
	setSlot(sl.tmoUp, "TMO+", color.RGBA{40, 80, 40, 255}, color.RGBA{160, 255, 160, 255})
}

// handleSettingsKeyEvent processes a key press while in settings mode.
func (a *App) handleSettingsKeyEvent(keyIndex int) error {
	// Back key: leave settings
	if keyIndex == a.nav.BackKey() {
		a.exitSettings()
		return nil
	}

	// T1/T2 scroll through settings pages (future expansion; no-op on single page)
	const totalSettingsPages = 1
	if keyIndex == a.nav.Toggle1Key() {
		if a.settingsPage > 0 {
			a.settingsPage--
			a.renderSettingsPage()
		}
		return nil
	}
	if keyIndex == a.nav.Toggle2Key() {
		if a.settingsPage < totalSettingsPages-1 {
			a.settingsPage++
			a.renderSettingsPage()
		}
		return nil
	}

	contentKeys := a.nav.GetContentKeys()
	sl := a.calcSettingsLayout()

	// Map the physical key index to a slot index
	slot := -1
	for i, ck := range contentKeys {
		if ck == keyIndex {
			slot = i
			break
		}
	}

	switch slot {
	case sl.exit:
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
	case sl.exit:
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
			if sl.exit < len(contentKeys) {
				img := a.nav.CreateTextImageWithColors("BYE",
					color.RGBA{180, 0, 0, 255},
					color.RGBA{255, 200, 200, 255})
				a.device.SetImage(contentKeys[sl.exit], img)
			}
			time.Sleep(500 * time.Millisecond)
			a.cancel()
		}
		return nil
	case sl.reload:
		fmt.Println("[*] RELOAD pressed - restarting")
		if sl.reload < len(contentKeys) {
			img := a.nav.CreateTextImageWithColors("...",
				color.RGBA{80, 60, 0, 255},
				color.RGBA{255, 210, 80, 255})
			a.device.SetImage(contentKeys[sl.reload], img)
		}
		time.Sleep(300 * time.Millisecond)
		a.restartRequested = true
		a.cancel()
		return nil
	case sl.openDir:
		fmt.Printf("[*] Opening config directory: %s\n", a.configPath)
		if err := openConfigDir(a.configPath); err != nil {
			log.Printf("openConfigDir: %v", err)
		}
		return nil
	case sl.brtDown:
		a.adjustBrightness(-5)
	case sl.brtUp:
		a.adjustBrightness(+5)
	case sl.tmoDown:
		a.stepTimeout(-1)
	case sl.tmoUp:
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
