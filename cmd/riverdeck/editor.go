package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/merith-tk/riverdeck/pkg/layout"
	"github.com/merith-tk/riverdeck/pkg/streamdeck"
)

// OpenEditor launches the standalone riverdeck-wails editor binary (expected
// alongside this executable).  The editor runs entirely in-process -- no TCP
// port is opened.  Falls back to a log message when the binary is not found.
func (a *App) OpenEditor() {
	helperName := "riverdeck-wails"
	if runtime.GOOS == "windows" {
		helperName += ".exe"
	}
	exe, err := os.Executable()
	if err != nil {
		log.Printf("[!] Could not resolve executable path: %v", err)
		return
	}
	helper := filepath.Join(filepath.Dir(exe), helperName)
	if _, statErr := os.Stat(helper); statErr != nil {
		log.Printf("[!] Editor binary not found: %s", helper)
		return
	}
	cmd := exec.Command(helper,
		"-configdir", a.configPath,
		"-cols", fmt.Sprintf("%d", a.device.Cols()),
		"-rows", fmt.Sprintf("%d", a.device.Rows()),
		"-model", a.device.ModelName(),
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if startErr := cmd.Start(); startErr != nil {
		log.Printf("[!] Failed to start editor: %v", startErr)
		return
	}
	log.Printf("[*] Editor window spawned (pid %d)", cmd.Process.Pid)
}

// createNavigator constructs the appropriate navigator based on the configured
// navigation style:
//
//	"folder" (default) - file-browser Navigator, same behaviour as before.
//	"layout"           - declarative LayoutNavigator; fails if layout.json absent.
//	"auto"             - LayoutNavigator when layout.json exists, else folder.
func (a *App) createNavigator(dev streamdeck.DeviceIface, dir string) streamdeck.NavigatorIface {
	style := a.config.UI.NavigationStyle

	useLayout := false
	switch style {
	case "layout":
		useLayout = true
	case "auto":
		useLayout = layout.Exists(dir)
	}

	if useLayout {
		lay, err := layout.Load(dir)
		if err != nil {
			log.Printf("[!] Failed to load layout.json (%v), falling back to folder navigation", err)
		} else if lay != nil {
			log.Printf("[*] Navigation mode: layout (%d pages)", len(lay.Pages))
			return streamdeck.NewLayoutNavigator(dev, dir, lay)
		} else if style == "layout" {
			// "layout" was explicitly set but no layout.json exists - create a blank one.
			lay = layout.NewEmpty()
			if serr := layout.Save(dir, lay); serr != nil {
				log.Printf("[!] Could not create blank layout.json: %v", serr)
			} else {
				log.Printf("[*] Navigation mode: layout (new empty layout created)")
			}
			return streamdeck.NewLayoutNavigator(dev, dir, lay)
		}
		// auto + no layout.json -> fall through to folder.
	}

	log.Printf("[*] Navigation mode: folder")
	return streamdeck.NewNavigator(dev, dir)
}
