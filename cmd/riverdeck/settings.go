package main

// settings.go - implements the settings overlay menu for the Stream Deck.
//
// The settings page is a virtual overlay (not a real folder) that appears when
// the user presses the reserved back/settings key while at the navigation root.
//
// Pagination mirrors normal navigation:
//   - Back key: exits settings when on page 0; shows PG^ and goes to the
//     previous settings page when on page > 0.
//   - T1  key: shows PGv and advances to the next settings page when more
//     pages exist; otherwise shown dim (inert).
//   - T2  key: always dim / free - never consumed by settings pagination.
//
// Page layout by device size:
//
// 3+ rows (MK.2, XL, ...) -- single page, all controls visible at once:
//
//	Row 0:  [<-/SET] [EXIT] [..] [RELOAD] [..] [CFGDIR]
//	Row 1:  [T1 dim] [BRT-] [B:XX%] [BRT+]
//	Row 2:  [T2 dim] [TMO-] [T:XXs] [TMO+]
//
// 2 rows (Neo, smol, ...) -- two pages:
//
//	Page 0  Row 0: [<-]   [EXIT] [RELOAD] [CFGDIR]
//	        Row 1: [PGv]  [BRT-] [B:XX%]  [BRT+]
//	Page 1  Row 0: [PG^]  [EXIT] [RELOAD] [CFGDIR]
//	        Row 1: [dim]  [TMO-] [T:XXs]  [TMO+]
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
// by row, col-0 reserved keys excluded). A value of -1 means the control is
// disabled/hidden on the current device.
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

// settingsPageCount returns the total number of settings pages for this device.
// Devices with 3+ rows can show all controls on a single page.
// Smaller devices paginate: page 0 = brightness, page 1 = timeout.
func (a *App) settingsPageCount() int {
	if a.device.Rows() >= 3 {
		return 1
	}
	return 2
}

// calcSettingsLayout returns slot indices into the contentKeys slice for every
// settings control, computed from the current device geometry and active settings page.
//
// System row (row 0) column assignments by content-col count (cc = Cols()-1):
//   - cc >= 4 : [EXIT] [ ] [RELOAD] ... [CFGDIR]  (CFGDIR at far right)
//   - cc == 3 : [EXIT] [RELOAD] [CFGDIR]           (tight fit, no gap)
//   - cc == 2 : [EXIT] [CFGDIR]                    (RELOAD disabled)
//   - cc == 1 : [EXIT]                              (everything else disabled)
//
// Control rows:
//   - rows >= 3 : brightness on row 1, timeout on row 2 (single page)
//   - rows == 2 : one control row; page 0 = brightness, page 1 = timeout
//
// Slot -1 means the button is disabled/hidden on this device.
func (a *App) calcSettingsLayout() settingsLayout {
	cc := a.device.Cols() - 1 // content columns per row
	if cc < 1 {
		cc = 1
	}
	rows := a.device.Rows()
	page := a.settingsPage

	// Row starting offsets into contentKeys
	row0 := 0
	row1 := cc
	row2 := cc * 2

	// Start with everything disabled
	sl := settingsLayout{
		exit:   row0 + 0,
		reload: -1, openDir: -1,
		brtDown: -1, brtVal: -1, brtUp: -1,
		tmoDown: -1, tmoVal: -1, tmoUp: -1,
	}

	// System row: RELOAD and CFGDIR positions depend on available width
	switch {
	case cc >= 4:
		sl.reload = row0 + 2
		sl.openDir = row0 + cc - 1 // far right
	case cc == 3:
		sl.reload = row0 + 1 // tight: EXIT RELOAD CFGDIR
		sl.openDir = row0 + 2
	case cc == 2:
		sl.openDir = row0 + 1 // EXIT CFGDIR (no room for reload)
		// cc == 1: EXIT only
	}

	// Control rows
	if rows >= 3 {
		// All controls fit on one page
		sl.brtDown = row1 + 0
		sl.brtVal = row1 + 1
		sl.brtUp = row1 + 2
		sl.tmoDown = row2 + 0
		sl.tmoVal = row2 + 1
		sl.tmoUp = row2 + 2
	} else {
		// 2-row device: paginate the control row
		if page == 0 {
			sl.brtDown = row1 + 0
			sl.brtVal = row1 + 1
			sl.brtUp = row1 + 2
		} else {
			sl.tmoDown = row1 + 0
			sl.tmoVal = row1 + 1
			sl.tmoUp = row1 + 2
		}
	}

	return sl
}

// enterSettings switches the App into settings mode and renders the settings page.
func (a *App) enterSettings() {
	a.inSettings = true
	log.Println("[*] Entering settings menu")
	a.renderSettingsPage()
}

// exitSettings leaves settings mode and returns to the normal navigation page.
func (a *App) exitSettings() {
	a.inSettings = false
	a.exitConfirming = false
	log.Println("[*] Exiting settings menu")

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

	// Reserved col-0 key.
	// Mirrors normal navigation: PG^ when on a page past the first, otherwise <-.
	if a.settingsPage > 0 {
		backImg := a.nav.CreateTextImageWithColors("PG^", color.RGBA{60, 60, 60, 255}, color.White)
		a.device.SetImage(a.nav.BackKey(), backImg)
	} else {
		backImg := a.nav.CreateTextImageWithColors("<-", color.RGBA{100, 100, 100, 255}, color.White)
		a.device.SetImage(a.nav.BackKey(), backImg)
	}

	// T1: PGv when more settings pages exist ahead; dim otherwise.
	// T2: always dim/free -- never consumed by settings pagination.
	totalPages := a.settingsPageCount()
	t1Key := a.nav.Toggle1Key()
	t2Key := a.nav.Toggle2Key()
	if t1Key < totalKeys {
		if a.settingsPage < totalPages-1 {
			t1Img := a.nav.CreateTextImageWithColors("PGv", color.RGBA{60, 60, 60, 255}, color.White)
			a.device.SetImage(t1Key, t1Img)
		} else {
			t1Img := a.nav.CreateTextImageWithColors("T1", color.RGBA{30, 30, 30, 255}, color.RGBA{80, 80, 80, 255})
			a.device.SetImage(t1Key, t1Img)
		}
	}
	if t2Key < totalKeys {
		t2Img := a.nav.CreateTextImageWithColors("T2", color.RGBA{30, 30, 30, 255}, color.RGBA{80, 80, 80, 255})
		a.device.SetImage(t2Key, t2Img)
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
	totalPages := a.settingsPageCount()

	// Back key: go to previous settings page when past page 0; exit settings on page 0.
	if keyIndex == a.nav.BackKey() {
		if a.settingsPage > 0 {
			a.settingsPage--
			a.renderSettingsPage()
			return nil
		}
		a.exitSettings()
		return nil
	}

	// T1: advance to next settings page when more exist; inert otherwise.
	if keyIndex == a.nav.Toggle1Key() {
		if a.settingsPage < totalPages-1 {
			a.settingsPage++
			a.renderSettingsPage()
		}
		return nil
	}

	// T2: never used for settings pagination -- ignore.
	if t2Key := a.nav.Toggle2Key(); t2Key < a.device.Keys() && keyIndex == t2Key {
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
			log.Println("[*] EXIT confirmed - shutting down")
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
		log.Println("[*] RELOAD pressed - restarting")
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
		log.Printf("[*] Opening config directory: %s", a.configPath)
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
	log.Printf("[*] Brightness -> %d%%", v)
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
	log.Printf("[*] Timeout -> %s", fmtTimeout(a.config.Application.Timeout))
	// Reset the sleep timer with the new value
	a.resetSleepTimer()
}

// persistConfig writes the current config to disk.
func (a *App) persistConfig() {
	if err := SaveConfig(a.config, a.configPath); err != nil {
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
