// Package main implements the Riverdeck Stream Deck interface application.
//
// This application provides a programmable interface for Elgato Stream Deck devices,
// allowing users to create custom button actions via Lua scripts.
//
// Key components:
// - Device management: Discovery, opening, and control of Stream Deck devices
// - Script management: Loading and executing Lua scripts for button actions
// - Navigation: Folder-based navigation through script collections
// - Event handling: Processing key presses and script triggers
//
// Contributors can extend functionality by:
// - Adding new script APIs in the scripting package
// - Implementing custom navigation logic in the streamdeck package
// - Modifying the App struct for additional features
package main

import (
	"context"
	"fmt"
	"image/color"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/merith-tk/riverdeck/pkg/imaging"
	"github.com/merith-tk/riverdeck/pkg/scripting"
	"github.com/merith-tk/riverdeck/pkg/streamdeck"
)

// App represents the main application.
type App struct {
	device     *streamdeck.Device
	scriptMgr  *scripting.ScriptManager
	nav        *streamdeck.Navigator
	config     *Config
	configPath string
	ctx        context.Context
	cancel     context.CancelFunc

	// Settings overlay
	inSettings       bool
	settingsPage     int  // future: scroll through setting rows
	exitConfirming   bool // true after first EXIT press, waiting for confirmation
	restartRequested bool // set by RELOAD; main loop relaunches the process

	// Display sleep / timeout
	sleepMu      sync.Mutex
	sleeping     bool
	sleepTimer   *time.Timer
	lastActivity time.Time

	// Per-key GIF animation goroutines.
	// Each running animation holds a cancel func; replace/cancel it to stop.
	gifAnimsMu sync.Mutex
	gifAnims   map[int]context.CancelFunc
}

// NewApp creates a new application instance.
func NewApp() *App {
	return &App{
		gifAnims: make(map[int]context.CancelFunc),
	}
}

// Init initializes the application, including device discovery and setup.
// It performs the following steps:
// 1. Initializes the Stream Deck library
// 2. Enumerates available devices and selects the first one
// 3. Opens the device and sets initial brightness
// 4. Creates the config directory structure
// 5. Initializes the script manager and navigator
// 6. Sets up key update callbacks and passive loops
//
// Returns an error if initialization fails at any step.
func (a *App) Init(configDir string) error {
	// Determine config path first
	configPath := getConfigPath(configDir)

	// Ensure config directory exists
	absConfigPath, err := ensureConfigDir(configPath)
	if err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	a.configPath = absConfigPath

	// Load configuration
	config, err := LoadConfig(absConfigPath)
	if err != nil {
		log.Printf("Warning: Failed to load config, using defaults: %v", err)
		config = DefaultConfig()
	}
	a.config = config

	fmt.Printf("\n[*] Config directory: %s\n", absConfigPath)
	fmt.Printf("[*] Configuration loaded\n")

	// Initialize the streamdeck library
	if err := streamdeck.Init(); err != nil {
		return fmt.Errorf("failed to init streamdeck: %w", err)
	}

	// Probe for all Stream Deck devices
	fmt.Println("\n[*] Scanning for Stream Deck devices...")

	devices, err := streamdeck.Enumerate()
	if err != nil {
		return fmt.Errorf("failed to enumerate devices: %w", err)
	}

	if len(devices) == 0 {
		fmt.Println("No Stream Deck devices found.")
		return fmt.Errorf("no devices found")
	}

	fmt.Printf("Found %d Stream Deck device(s):\n\n", len(devices))

	for i, info := range devices {
		fmt.Printf("Device #%d:\n", i+1)
		streamdeck.PrintDeviceInfo(info)
		fmt.Println()
	}

	// Use the first device
	info := devices[0]
	if info.Model.PixelSize == 0 {
		fmt.Println("First device has no display (e.g., Pedal). Skipping.")
		return fmt.Errorf("device has no display")
	}

	fmt.Printf("Opening %s...\n", info.Model.Name)

	dev, err := streamdeck.OpenWithConfig(info.Path, a.config.Performance.JPEGQuality)
	if err != nil {
		return fmt.Errorf("failed to open device: %w", err)
	}
	a.device = dev

	// Set brightness from config
	if err := dev.SetBrightness(a.config.Application.Brightness); err != nil {
		log.Printf("SetBrightness failed: %v", err)
	}

	fmt.Printf("\n[*] Config directory: %s\n", a.configPath)

	// Create script manager and boot (loads scripts, starts background workers)
	fmt.Println("[*] Booting script manager...")
	a.scriptMgr = scripting.NewScriptManager(dev, absConfigPath, a.config.Application.PassiveFPS)

	// Create a context for the entire application
	a.ctx, a.cancel = context.WithCancel(context.Background())

	// Boot scripts (shows loading indicator, loads all scripts)
	if err := a.scriptMgr.Boot(a.ctx); err != nil {
		log.Printf("Warning: Script boot error: %v", err)
	}

	// Create navigator
	a.nav = streamdeck.NewNavigator(dev, absConfigPath)
	a.nav.SetScriptValidator(a.scriptMgr.IsUsableScript)

	// Set up passive key updates from scripts
	a.setupKeyUpdateCallback()

	// Start the passive update loop (15fps)
	a.scriptMgr.StartPassiveLoop()

	return nil
}

// setupKeyUpdateCallback sets up the callback for script-driven key updates.
// This allows Lua scripts to dynamically change button appearances.
func (a *App) setupKeyUpdateCallback() {
	a.scriptMgr.SetKeyUpdateCallback(func(keyIndex int, appearance *scripting.KeyAppearance) {
		if appearance == nil {
			return
		}

		// Don't let passive/background scripts paint over the settings overlay
		// or a sleeping (blank) display.
		if a.inSettings {
			return
		}
		a.sleepMu.Lock()
		isSleeping := a.sleeping
		a.sleepMu.Unlock()
		if isSleeping {
			return
		}

		// Check for custom image first
		if appearance.Image != "" {
			// Animated GIF: spin up a per-key animation goroutine.
			if strings.ToLower(filepath.Ext(appearance.Image)) == ".gif" {
				a.startGIFAnim(keyIndex, appearance)
				return
			}

			// Static image: cancel any running GIF for this key first.
			a.stopGIFAnim(keyIndex)
			img, err := imaging.LoadImage(appearance.Image)
			if err == nil {
				// Resize to fit key and display
				resized := a.device.ResizeImage(img)
				a.device.SetImage(keyIndex, resized)
				return
			}
			// Fall through to color/text if image load fails
			log.Printf("Image load failed: %v", err)
		} else {
			// No image - cancel any running GIF for this key.
			a.stopGIFAnim(keyIndex)
		}

		// Apply appearance to key
		c := color.RGBA{
			R: uint8(appearance.Color[0]),
			G: uint8(appearance.Color[1]),
			B: uint8(appearance.Color[2]),
			A: 255,
		}
		if appearance.Text != "" {
			// Create text image with appearance colors
			img := a.nav.CreateTextImageWithColors(
				appearance.Text,
				c,
				color.RGBA{
					R: uint8(appearance.TextColor[0]),
					G: uint8(appearance.TextColor[1]),
					B: uint8(appearance.TextColor[2]),
					A: 255,
				},
			)
			a.device.SetImage(keyIndex, img)
		} else {
			a.device.SetKeyColor(keyIndex, c)
		}
	})
}

// Run starts the main event loop and handles user interactions.
// It renders the initial page, sets up signal handling for graceful shutdown,
// and processes key events from the Stream Deck device.
func (a *App) Run() error {
	// Helper to update visible scripts
	updateVisibleScripts := func() {
		a.scriptMgr.SetVisibleScripts(a.nav.GetVisibleScripts())
	}

	// Render initial page
	fmt.Println("[*] Loading page...")
	a.scriptMgr.SetVisibleScripts(nil) // Clear before render
	if err := a.nav.RenderPage(); err != nil {
		log.Printf("Warning: RenderPage failed: %v", err)
	}

	// Show current path
	page, _ := a.nav.LoadPage()
	if page != nil {
		fmt.Printf("[*] Current: %s (%d items, page %d/%d)\n",
			page.Path, len(page.Items), page.PageIndex+1, page.TotalPages)
	}

	fmt.Println("\n[*] Navigation ready (Ctrl+C to exit)...")
	fmt.Println("    - Column 0: Reserved (Back/<SET>, Toggle1, Toggle2)")
	fmt.Println("    - Columns 1-4: Folder/action buttons")
	fmt.Println("    - Press '<-' to go back; press 'SET' at root to open settings")

	// Initialise the activity timer and last-activity timestamp.
	a.lastActivity = time.Now()
	a.resetSleepTimer()

	// Update visible scripts for initial page
	updateVisibleScripts()

	// Handle Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\n\nExiting...")
		a.cancel()
	}()

	// Listen for key events
	events := make(chan streamdeck.KeyEvent, 10)
	a.device.ListenKeys(a.ctx, events)

	for event := range events {
		if err := a.handleKeyEvent(event); err != nil {
			log.Printf("Error handling key event: %v", err)
		}
	}

	fmt.Println("Done!")
	return nil
}

// handleKeyEvent processes a single key event.
// It handles navigation, toggle states, and script triggers based on the key pressed.
func (a *App) handleKeyEvent(event streamdeck.KeyEvent) error {
	// Only handle key presses, not releases
	if !event.Pressed {
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

	// At root, the back/settings key opens the settings menu.
	if event.Key == streamdeck.KeyBack && a.nav.IsAtRoot() {
		a.enterSettings()
		return nil
	}

	// Intercept T1/T2 BEFORE passing to the navigator so the old toggle
	// logic inside HandleKeyPress never fires for these keys.
	if event.Key == streamdeck.KeyToggle1 {
		if a.scriptMgr.HasT1Script() {
			go func() {
				if err := a.scriptMgr.TriggerT1(); err != nil {
					log.Printf("T1 trigger: %v", err)
				}
			}()
		}
		// No script assigned: key is reserved/inert.
		return nil
	}
	if event.Key == streamdeck.KeyToggle2 {
		if a.scriptMgr.HasT2Script() {
			go func() {
				if err := a.scriptMgr.TriggerT2(); err != nil {
					log.Printf("T2 trigger: %v", err)
				}
			}()
		}
		// No script assigned: key is reserved/inert.
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
			fmt.Printf("[*] Navigated to: %s (%d items)\n", relPath, len(page.Items))
		}
	} else if item != nil {
		// Action/script triggered
		fmt.Printf("[*] Action triggered: %s\n", item.Name)
		if item.Script != "" {
			fmt.Printf("    Script: %s\n", item.Script)
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
	a.scriptMgr.SetToggleScripts(t1Script, streamdeck.KeyToggle1, t2Script, streamdeck.KeyToggle2)
}

// Shutdown cleans up resources.
// It shuts down the script manager, closes the device, and exits the Stream Deck library.
func (a *App) Shutdown() {
	a.stopAllGIFAnims()
	if a.scriptMgr != nil {
		a.scriptMgr.Shutdown()
	}
	if a.device != nil {
		// Blank the display on exit to prevent burn-in.
		_ = a.device.SetBrightness(0)
		_ = a.device.Clear()
		a.device.Close()
	}
	streamdeck.Exit()
}
