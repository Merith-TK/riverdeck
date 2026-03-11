package main

import (
	"flag"
	"log"
	"os"
	"os/exec"
	"sync"

	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/driver/desktop"
)

var configDir = flag.String("configdir", "", "Configuration directory path")
var simAddr = flag.String("sim", "", "Connect to riverdeck-simulator at host:port instead of real hardware")

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	flag.Parse()

	// Create the Fyne application. This must happen on the main thread.
	// It owns the OS event loop and drives the systray icon.
	a := fyneapp.NewWithID("io.github.merith-tk.riverdeck")
	a.SetIcon(resourceIconSvg)

	// Track the currently running riverdeck App so the systray menu can
	// cancel it. Protected by a mutex because the goroutine writes it and
	// the Fyne event loop reads it.
	var (
		activeMu  sync.Mutex
		activeApp *App
	)
	setActive := func(rd *App) {
		activeMu.Lock()
		activeApp = rd
		activeMu.Unlock()
	}
	cancelActive := func() {
		activeMu.Lock()
		rd := activeApp
		activeMu.Unlock()
		if rd != nil && rd.cancel != nil {
			rd.cancel()
		}
	}

	// Systray menu: a single "Quit Riverdeck" item.
	// Cancelling the active app's context causes Run() to return, at which
	// point the goroutine below will call fyneApp.Quit() and the process exits.
	menu := fyne.NewMenu("Riverdeck",
		fyne.NewMenuItem("Quit Riverdeck", func() {
			cancelActive()
			// fyneApp.Quit() is called by the goroutine once Run() returns,
			// so we don't need to call it here.
		}),
	)
	// SetSystemTrayIcon / SetSystemTrayMenu live on the desktop.App interface.
	if desk, ok := a.(desktop.App); ok {
		desk.SetSystemTrayIcon(resourceIconSvg)
		desk.SetSystemTrayMenu(menu)
	}

	// Run the riverdeck event loop in a background goroutine. When it exits
	// permanently (no restart requested) we call fyneApp.Quit() so the process
	// terminates cleanly.
	go func() {
		for {
			rd := NewApp()
			setActive(rd)

			if err := rd.Init(*configDir, *simAddr); err != nil {
				log.Fatal(err)
			}

			if err := rd.Run(); err != nil {
				rd.Shutdown()
				log.Fatal(err)
			}

			rd.Shutdown()

			if !rd.restartRequested {
				a.Quit()
				return
			}

			// Relaunch: re-exec the current binary with the same arguments.
			// Shutdown() above already closed the HID device so the new process
			// can claim it cleanly.
			exe, err := os.Executable()
			if err != nil {
				log.Printf("Restart: could not resolve executable: %v - falling back to in-process restart", err)
				continue
			}
			cmd := exec.Command(exe, os.Args[1:]...)
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Start(); err != nil {
				log.Printf("Restart: exec failed: %v - falling back to in-process restart", err)
				continue
			}
			// The new process is running; exit this one.
			a.Quit()
			os.Exit(0)
		}
	}()

	// Block the main thread running the Fyne/OS event loop (systray lives here).
	a.Run()
}
