package main

import (
	"fmt"
	"image/color"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/merith-tk/riverdeck/pkg/streamdeck"
)

// handleKeyEvent processes a single key event.
// It handles navigation, toggle states, and script triggers based on the key pressed.
func (a *App) handleKeyEvent(event streamdeck.KeyEvent) error {
	// ── Emergency kill combo tracking (runs on EVERY event, press or release) ──
	// Holding all 4 corners + center simultaneously triggers an immediate hard exit.
	// While the back/home key is held, all other key presses are suppressed so
	// that navigating towards the combo never accidentally fires scripts.
	a.heldKeysMu.Lock()
	if event.Pressed {
		a.heldKeys[event.Key] = true
	} else {
		delete(a.heldKeys, event.Key)
	}
	panicTriggered := false
	if event.Pressed && len(a.panicCombo) > 0 {
		allHeld := true
		for _, k := range a.panicCombo {
			if !a.heldKeys[k] {
				allHeld = false
				break
			}
		}
		panicTriggered = allHeld
	}
	newBackHeld := a.heldKeys[streamdeck.KeyBack]
	backTransition := newBackHeld != a.backHeld
	if backTransition {
		a.backHeld = newBackHeld
	}
	a.heldKeysMu.Unlock()

	// Fire the hold-change hook outside the lock.
	if backTransition {
		a.handleBackHoldChange(newBackHeld)
	}

	if panicTriggered {
		a.triggerEmergencyExit()
		return nil
	}

	// Only handle key presses, not releases
	if !event.Pressed {
		return nil
	}

	// Back key acts as a modifier / lock while held: swallow any other key press
	// so that building up the emergency combo never fires scripts or navigation.
	if newBackHeld && event.Key != streamdeck.KeyBack {
		return nil
	}

	// Reset / restart the inactivity sleep timer on every key press.
	a.lastActivity = time.Now()
	a.resetSleepTimer()

	// If the display is sleeping, the first key press only wakes it up.
	if a.wakeDisplay() {
		if a.inSettings {
			a.renderSettingsPage()
		} else {
			_ = a.nav.RenderPage()
		}
		return nil
	}

	// In settings mode all keys are handled by the settings handler.
	if a.inSettings {
		return a.handleSettingsKeyEvent(event.Key)
	}

	// At root on page 0, the back key opens the settings menu.
	// On page > 0 it acts as PG^ and is handled inside HandleKeyPress.
	if event.Key == streamdeck.KeyBack && a.nav.IsAtRoot() && a.nav.PageIndex() == 0 {
		a.enterSettings()
		return nil
	}

	// Intercept T1/T2 BEFORE passing to the navigator.
	// Pagination takes priority: T1 = next page when more pages exist.
	if event.Key == a.nav.Toggle1Key() {
		if a.nav.NextPage() {
			// Page changed -- same re-render path as any navigation.
			a.stopAllGIFAnims()
			a.scriptMgr.SetVisibleScripts(nil)
			if err := a.nav.RenderPage(); err != nil {
				log.Printf("RenderPage failed: %v", err)
			}
			a.updateVisibleScripts()
			return nil
		}
		// No next page: T1 is free for scripts.
		if a.scriptMgr.HasT1Script() {
			go func() {
				if err := a.scriptMgr.TriggerT1(); err != nil {
					log.Printf("T1 trigger: %v", err)
				}
			}()
		}
		return nil
	}
	// T2 is never consumed by pagination.
	if event.Key == a.nav.Toggle2Key() {
		if a.scriptMgr.HasT2Script() {
			go func() {
				if err := a.scriptMgr.TriggerT2(); err != nil {
					log.Printf("T2 trigger: %v", err)
				}
			}()
		}
		return nil
	}

	// Handle the key press
	item, navigated, err := a.nav.HandleKeyPress(event.Key)
	if err != nil {
		return fmt.Errorf("handling key press: %w", err)
	}

	if navigated {
		// Cancel any running GIF animations before the new page renders.
		a.stopAllGIFAnims()
		// Clear visible scripts BEFORE render to prevent race condition
		a.scriptMgr.SetVisibleScripts(nil)

		// Page changed, re-render
		if err := a.nav.RenderPage(); err != nil {
			log.Printf("RenderPage failed: %v", err)
		}

		// Update visible scripts for passive updates
		a.updateVisibleScripts()

		page, _ := a.nav.LoadPage()
		if page != nil {
			relPath, _ := filepath.Rel(a.configPath, page.Path)
			if relPath == "." {
				relPath = "/"
			} else {
				relPath = "/" + relPath
			}
			log.Printf("[*] Navigated to: %s (%d items)", relPath, len(page.Items))
		}
	} else if item != nil {
		// Action/script triggered
		log.Printf("[*] Action triggered: %s", item.Name)
		if item.Script != "" {
			log.Printf("    Script: %s", item.Script)
			// Run trigger asynchronously so the event loop never blocks waiting
			// for a slow trigger function (HTTP, shell, sleep, etc.)
			scriptPath := item.Script
			go func() {
				if err := a.scriptMgr.TriggerScript(scriptPath); err != nil {
					log.Printf("Script error: %v", err)
				}
				// Refresh only the triggered key instead of redrawing the whole page
				a.scriptMgr.RefreshScript(scriptPath)
			}()
		}
	}

	return nil
}

// updateVisibleScripts updates the visible scripts in the script manager and
// wires the T1/T2 keys to .directory.lua of the current folder if it defines
// t1_passive / t1_trigger / t2_passive / t2_trigger.
func (a *App) updateVisibleScripts() {
	a.scriptMgr.SetVisibleScripts(a.nav.GetVisibleScripts())

	// Determine T1/T2 script assignments from the current folder's .directory.lua
	dirScript := a.nav.CurrentDirScript()
	t1Script, t2Script := "", ""
	if dirScript != "" {
		if runner := a.scriptMgr.GetRunner(dirScript); runner != nil {
			if runner.HasT1Passive() || runner.HasT1Trigger() {
				t1Script = dirScript
			}
			if runner.HasT2Passive() || runner.HasT2Trigger() {
				t2Script = dirScript
			}
		}
	}
	a.scriptMgr.SetToggleScripts(t1Script, a.nav.Toggle1Key(), t2Script, a.nav.Toggle2Key())
}

// handleBackHoldChange is called whenever the back/home key transitions between
// held and released.  It is the designated hook for future system-level modifier
// behaviours (e.g. showing a system overlay, starting a hold timer, etc.).
//
// held == true  -> back key just went down
// held == false -> back key just came up
func (a *App) handleBackHoldChange(held bool) {
	if held {
		log.Println("[*] Back key held - input lock active")
	} else {
		log.Println("[*] Back key released - input lock cleared")
	}
}

// triggerEmergencyExit performs an immediate hard shutdown when the emergency
// "oh shit" kill combo (all four corners + center key held simultaneously) is
// detected.  It flashes all keys red so the user gets visual feedback, then
// tears down the device and calls os.Exit(1) -- bypassing the normal shutdown
// path to guarantee the process dies even if the event loop is stuck.
func (a *App) triggerEmergencyExit() {
	fmt.Println("\n[!!!] EMERGENCY EXIT: corners+center combo detected -- killing process")
	// Flash all keys red as a visible kill indicator.
	for i := 0; i < a.device.Keys(); i++ {
		_ = a.device.SetKeyColor(i, color.RGBA{255, 0, 0, 255})
	}
	time.Sleep(300 * time.Millisecond)
	// Blank the deck and tear down cleanly before hard-exiting.
	_ = a.device.SetBrightness(0)
	_ = a.device.Clear()
	a.device.Close()
	streamdeck.Exit()
	os.Exit(1)
}
