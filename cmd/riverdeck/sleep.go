package main

// sleep.go -- display sleep/wake timeout management.
//
// Extracted from app.go for clarity. All methods operate on *App.

import (
	"log"
	"time"
)

// resetSleepTimer resets (or starts) the inactivity sleep timer.
// Must be called after any key activity and after timeout config changes.
func (a *App) resetSleepTimer() {
	a.sleepMu.Lock()
	defer a.sleepMu.Unlock()

	if a.sleepTimer != nil {
		a.sleepTimer.Stop()
		a.sleepTimer = nil
	}

	if a.config.Application.Timeout <= 0 {
		return // disabled
	}

	duration := time.Duration(a.config.Application.Timeout) * time.Second
	a.sleepTimer = time.AfterFunc(duration, func() {
		a.sleepMu.Lock()
		defer a.sleepMu.Unlock()
		if !a.sleeping {
			a.sleeping = true
			log.Println("[*] Display sleeping (timeout)")
			_ = a.device.SetBrightness(0)
		}
	})
}

// wakeDisplay restores brightness if the display is sleeping.
// Returns true if the device was actually woken (caller should swallow the key).
func (a *App) wakeDisplay() bool {
	a.sleepMu.Lock()
	defer a.sleepMu.Unlock()

	if !a.sleeping {
		return false
	}
	a.sleeping = false
	log.Println("[*] Display waking up")
	_ = a.device.SetBrightness(a.config.Application.Brightness)
	return true
}
