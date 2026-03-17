package main

import (
	"flag"
	"log"
	"sync"

	"github.com/getlantern/systray"
	"github.com/merith-tk/riverdeck/resources"
)

var configDir = flag.String("configdir", "", "Configuration directory path")

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	flag.Parse()

	// Track the currently running riverdeck App so the systray menu can
	// cancel it.  Protected by a mutex because the goroutine writes it and
	// the systray callbacks read it.
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

	// Run the riverdeck event loop in a background goroutine. When it exits
	// permanently (no restart requested) we call systray.Quit() so the
	// process terminates cleanly.
	go func() {
		for {
			rd := NewApp()
			setActive(rd)

			if err := rd.Init(*configDir); err != nil {
				log.Fatal(err)
			}

			if err := rd.Run(); err != nil {
				rd.Shutdown()
				log.Fatal(err)
			}

			rd.Shutdown()

			if !rd.restartRequested {
				systray.Quit()
				return
			}

			// In-process restart: loop back to NewApp() above.
			log.Println("[*] Restarting in-process...")
		}
	}()

	// Block the main thread running the systray event loop.
	systray.Run(func() {
		// onReady: set up the tray icon and menu.
		systray.SetIcon(resources.IconPNG)
		systray.SetTitle("Riverdeck")
		systray.SetTooltip("Riverdeck - Stream Deck Controller")

		mEditor := systray.AddMenuItem("Open Editor", "Launch the layout editor")
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("Quit Riverdeck", "Stop Riverdeck and exit")

		go func() {
			for {
				select {
				case <-mEditor.ClickedCh:
					activeMu.Lock()
					rd := activeApp
					activeMu.Unlock()
					if rd != nil {
						rd.OpenEditor()
					}
				case <-mQuit.ClickedCh:
					cancelActive()
				}
			}
		}()
	}, func() {
		// onExit: nothing to clean up.
	})
}
