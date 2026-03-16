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
	"io"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/merith-tk/riverdeck/pkg/imaging"
	"github.com/merith-tk/riverdeck/pkg/layout"
	"github.com/merith-tk/riverdeck/pkg/scripting"
	"github.com/merith-tk/riverdeck/pkg/streamdeck"
	"github.com/merith-tk/riverdeck/pkg/wsdevice"
	"github.com/merith-tk/riverdeck/resources"
)

// App represents the main application.
type App struct {
	device     streamdeck.DeviceIface
	simMode    bool // true when connected to simulator (no HID init)
	scriptMgr  *scripting.ScriptManager
	nav        streamdeck.NavigatorIface
	config     *Config
	configPath string
	ctx        context.Context
	cancel     context.CancelFunc

	// Settings overlay
	inSettings       bool
	settingsPage     int  // future: scroll through setting rows
	exitConfirming   bool // true after first EXIT press, waiting for confirmation
	restartRequested bool // set by RELOAD; main loop restarts in-process

	// Display sleep / timeout
	sleepMu      sync.Mutex
	sleeping     bool
	sleepTimer   *time.Timer
	lastActivity time.Time

	// Per-key GIF animation goroutines.
	// Each running animation holds a cancel func; replace/cancel it to stop.
	gifAnimsMu sync.Mutex
	gifAnims   map[int]context.CancelFunc

	// Emergency "oh shit" kill combo: all 4 corners + center held simultaneously.
	// panicCombo is computed from the device geometry in Init().
	heldKeysMu sync.Mutex
	heldKeys   map[int]bool
	panicCombo []int

	// backHeld is true while the back/home/settings key is physically held down.
	// Transitions are dispatched to handleBackHoldChange for future system hooks.
	backHeld bool

	// wsServer manages WebSocket software-client device connections.
	// Nil when WebSocket support is disabled or not in layout mode.
	wsServer *wsdevice.Server
}

// NewApp creates a new application instance.
func NewApp() *App {
	return &App{
		gifAnims: make(map[int]context.CancelFunc),
		heldKeys: make(map[int]bool),
	}
}

// Init initializes the application, including device discovery and setup.
// It performs the following steps:
// 1. Initializes the Stream Deck library
// 2. Enumerates available devices (or connects to a simulator)
// 3. Opens the device and sets initial brightness
// 4. Creates the config directory structure
// 5. Initializes the script manager and navigator
// 6. Sets up key update callbacks and passive loops
//
// When simAddr is non-empty ("host:port") the app connects to a running
// riverdeck-simulator instance instead of opening real HID hardware.
//
// Returns an error if initialization fails at any step.
func (a *App) Init(configDir string, simAddr string) error {
	// Resolve the single canonical config directory.
	dir := ConfigDir(configDir)
	absDir, err := filepath.Abs(dir)
	if err != nil {
		absDir = dir
	}
	a.configPath = absDir

	// Load (or create) configuration.
	config, err := LoadConfig(absDir)
	if err != nil {
		log.Printf("Warning: Failed to load config, using defaults: %v", err)
		config = DefaultConfig()
	}
	a.config = config

	log.Printf("[*] Config directory: %s", absDir)
	log.Printf("[*] Configuration loaded")

	// Open device: real hardware or simulator.
	var dev streamdeck.DeviceIface
	if simAddr != "" {
		log.Printf("[*] Simulator mode: connecting to %s ...", simAddr)
		sc, err := streamdeck.ConnectSim(simAddr)
		if err != nil {
			return fmt.Errorf("failed to connect to simulator: %w", err)
		}
		dev = sc
		a.simMode = true
		log.Printf("[*] Simulator ready: %s (%dx%d, %d keys)",
			sc.ModelName(), sc.Cols(), sc.Rows(), sc.Keys())
	} else {
		// Initialize the streamdeck library
		if err := streamdeck.Init(); err != nil {
			return fmt.Errorf("failed to init streamdeck: %w", err)
		}

		// Probe for all Stream Deck devices
		log.Println("[*] Scanning for Stream Deck devices...")

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

		hwDev, err := streamdeck.OpenWithConfig(info.Path, a.config.Performance.JPEGQuality)
		if err != nil {
			return fmt.Errorf("failed to open device: %w", err)
		}
		dev = hwDev
	}
	a.device = dev

	// Compute the emergency kill combo from device geometry.
	// Use all 4 corners + center, deduplicated (small devices may share keys).
	// Require at least 3 distinct keys to form a meaningful combo.
	{
		cols := dev.Cols()
		rows := dev.Rows()
		centerRow := rows / 2
		centerCol := cols / 2
		candidates := []int{
			0,                          // top-left
			cols - 1,                   // top-right
			centerRow*cols + centerCol, // center
			(rows - 1) * cols,          // bottom-left
			rows*cols - 1,              // bottom-right
		}
		seen := make(map[int]bool)
		for _, k := range candidates {
			if !seen[k] {
				seen[k] = true
				a.panicCombo = append(a.panicCombo, k)
			}
		}
		if len(a.panicCombo) < 3 {
			// Device too small for a safe combo -- disable it.
			a.panicCombo = nil
			log.Printf("[*] Emergency kill combo: disabled (device too small)")
		} else {
			log.Printf("[*] Emergency kill combo: keys %v", a.panicCombo)
		}
	}

	// Set brightness from config
	if err := dev.SetBrightness(a.config.Application.Brightness); err != nil {
		log.Printf("SetBrightness failed: %v", err)
	}

	log.Printf("[*] Config directory: %s", a.configPath)

	// Always refresh the bundled riverdeck package so templates are physically
	// present on disk. Delete the old copy, then re-extract from the embed.
	defaultPkgDest := filepath.Join(absDir, ".packages", "riverdeck")
	if rmErr := os.RemoveAll(defaultPkgDest); rmErr != nil {
		log.Printf("[!] Could not remove old riverdeck package: %v", rmErr)
	}
	pkgFS := resources.DefaultPackagesFS()
	if extractErr := extractFS(pkgFS, defaultPkgDest, "riverdeck"); extractErr != nil {
		log.Printf("[!] Could not extract default riverdeck package: %v", extractErr)
	} else {
		log.Printf("[*] Refreshed bundled riverdeck package at %s", defaultPkgDest)
	}

	// Seed a starter layout.json only on the very first run.
	if !layout.Exists(absDir) {
		pkgFS2 := resources.DefaultPackagesFS()
		srcData, readErr := fs.ReadFile(pkgFS2, "riverdeck/examples/default_layout.json")
		if readErr == nil {
			destPath := filepath.Join(absDir, "layout.json")
			if writeErr := os.WriteFile(destPath, srcData, 0644); writeErr != nil {
				log.Printf("[!] Could not seed default layout: %v", writeErr)
			} else {
				log.Printf("[*] Seeded default layout.json from bundled package")
			}
		}
	}

	// Create script manager and boot (loads scripts, starts background workers)
	log.Println("[*] Booting script manager...")
	a.scriptMgr = scripting.NewScriptManager(dev, absDir, a.config.Application.PassiveFPS)

	// Create a context for the entire application
	a.ctx, a.cancel = context.WithCancel(context.Background())

	// Boot scripts (shows loading indicator, loads all scripts)
	if err := a.scriptMgr.Boot(a.ctx); err != nil {
		log.Printf("Warning: Script boot error: %v", err)
	}

	// Create navigator based on the configured navigation style.
	a.nav = a.createNavigator(dev, absDir)
	a.nav.SetScriptValidator(a.scriptMgr.IsUsableScript)

	// Set up passive key updates from scripts
	a.setupKeyUpdateCallback()

	// Start the passive update loop (15fps)
	a.scriptMgr.StartPassiveLoop()

	// Start WebSocket device server when enabled and in layout mode.
	// Software clients connect via ws://host:port/ws and receive the layout
	// as a stream of JSON messages, with key events flowing back.
	if a.config.Network.WebSocketEnabled {
		style := a.config.UI.NavigationStyle
		if style == "layout" || style == "auto" {
			port := a.config.Network.WebSocketPort
			if port == 0 {
				port = 9000
			}
			a.wsServer = wsdevice.NewServer(port, func(wsDev *wsdevice.Device) {
				a.runWSDevice(wsDev)
			})
			a.wsServer.SetBrightness(a.config.Application.Brightness)
			if startErr := a.wsServer.Start(a.ctx); startErr != nil {
				log.Printf("[!] WS device server failed to start: %v", startErr)
			} else {
				log.Printf("[*] WebSocket device server listening on :%d  (connect: ws://localhost:%d/ws)", port, port)
			}
		}
	}

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
	log.Println("[*] Loading page...")
	a.scriptMgr.SetVisibleScripts(nil) // Clear before render
	if err := a.nav.RenderPage(); err != nil {
		log.Printf("Warning: RenderPage failed: %v", err)
	}

	// Show current path
	page, _ := a.nav.LoadPage()
	if page != nil {
		log.Printf("[*] Current: %s (%d items, page %d/%d)",
			page.Path, len(page.Items), page.PageIndex+1, page.TotalPages)
	}

	log.Println("[*] Navigation ready (Ctrl+C to exit)...")
	log.Println("    - Column 0: Reserved (Back/<SET>, Toggle1, Toggle2)")
	log.Println("    - Columns 1-4: Folder/action buttons")
	log.Println("    - Press '<-' to go back; press 'SET' at root to open settings")

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

// extractFS copies all files from srcFS into destDir, preserving directory
// structure.  prefix is stripped from the front of each path (e.g. "riverdeck"
// when srcFS is rooted at packages/ but files live at riverdeck/**).
// The dest directory and any missing parents are created automatically.
func extractFS(srcFS fs.FS, destDir string, prefix string) error {
	return fs.WalkDir(srcFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Build destination path: strip the bundle prefix component.
		rel := path
		if prefix != "" {
			rel = strings.TrimPrefix(rel, prefix)
			rel = strings.TrimPrefix(rel, "/")
		}
		if rel == "" || rel == "." {
			return nil // skip root entry
		}
		dest := filepath.Join(destDir, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(dest, 0755)
		}
		// Ensure parent directory exists.
		if mkErr := os.MkdirAll(filepath.Dir(dest), 0755); mkErr != nil {
			return mkErr
		}
		// Open source file.
		srcFile, openErr := srcFS.Open(path)
		if openErr != nil {
			return openErr
		}
		defer srcFile.Close()
		// Write destination file (always overwrite).
		destFile, createErr := os.Create(dest)
		if createErr != nil {
			return createErr
		}
		defer destFile.Close()
		_, copyErr := io.Copy(destFile, srcFile)
		return copyErr
	})
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
	if !a.simMode {
		streamdeck.Exit()
	}
}
