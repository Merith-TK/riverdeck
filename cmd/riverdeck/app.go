package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/merith-tk/riverdeck/pkg/editorserver"
	"github.com/merith-tk/riverdeck/pkg/layout"
	"github.com/merith-tk/riverdeck/pkg/platform"
	"github.com/merith-tk/riverdeck/pkg/scripting"
	"github.com/merith-tk/riverdeck/pkg/streamdeck"
	"github.com/merith-tk/riverdeck/pkg/util"
	"github.com/merith-tk/riverdeck/pkg/wsdevice"
	"github.com/merith-tk/riverdeck/resources"
)

// App is the multi-device orchestrator. It owns the root config, the
// WebSocket server, and a slice of DeviceSessions — one per connected
// hardware device.
type App struct {
	config     *Config
	configPath string
	ctx        context.Context
	cancel     context.CancelFunc

	restartRequested bool

	sessions   []*DeviceSession
	wsServer   *wsdevice.Server
	editorAddr string // set by startEditorServer when the editor HTTP server is running
}

// NewApp creates a new application instance.
func NewApp() *App {
	return &App{}
}

// Init initializes the application, enumerates all hardware devices, and
// creates a DeviceSession for each one. Previously the app only opened
// devices[0]; now any number of physical Stream Decks are supported.
//
// The multi-device mode (shared / individual / layout) determines how each
// session's config directory is resolved (see DeviceSessionDir).
func (a *App) Init(configDir string) error {
	dir := ConfigDir(configDir)
	absDir, err := filepath.Abs(dir)
	if err != nil {
		absDir = dir
	}
	a.configPath = absDir

	migrateConfigDir(absDir)

	config, err := LoadConfig(absDir)
	if err != nil {
		log.Printf("Warning: Failed to load config, using defaults: %v", err)
		config = DefaultConfig()
	}
	a.config = config

	log.Printf("[*] Config directory: %s", absDir)
	log.Printf("[*] Multi-device mode: %s", config.Device.MultiMode)
	log.Printf("[*] Configuration loaded")

	if err := streamdeck.Init(); err != nil {
		return fmt.Errorf("failed to init streamdeck: %w", err)
	}

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
		fmt.Println("===================================================")
		fmt.Printf("  Model:        %s\n", info.Model.Name)
		fmt.Printf("  Product:      %s\n", info.Product)
		fmt.Printf("  Manufacturer: %s\n", info.Manufacturer)
		fmt.Printf("  Serial:       %s\n", info.Serial)
		fmt.Printf("  Firmware:     %s\n", info.Firmware)
		fmt.Printf("  Product ID:   0x%04X\n", info.Model.ProductID)
		fmt.Println("---------------------------------------------------")
		fmt.Printf("  Layout:       %d columns x %d rows\n", info.Model.Cols, info.Model.Rows)
		fmt.Printf("  Total Keys:   %d\n", info.Model.Keys)
		if info.Model.PixelSize > 0 {
			fmt.Printf("  Icon Size:    %d x %d pixels\n", info.Model.PixelSize, info.Model.PixelSize)
			fmt.Printf("  Image Format: %s\n", info.Model.ImageFormat)
		} else {
			fmt.Println("  Icon Size:    N/A (no display)")
		}
		fmt.Println("===================================================")
		fmt.Println()
	}

	// Always refresh the bundled riverdeck package.
	defaultPkgDest := filepath.Join(platform.PackagesDir(absDir), "riverdeck")
	if rmErr := os.RemoveAll(defaultPkgDest); rmErr != nil {
		log.Printf("[!] Could not remove old riverdeck package: %v", rmErr)
	}
	pkgFS := resources.DefaultPackagesFS()
	if extractErr := util.ExtractFS(pkgFS, defaultPkgDest, "riverdeck"); extractErr != nil {
		log.Printf("[!] Could not extract default riverdeck package: %v", extractErr)
	} else {
		log.Printf("[*] Refreshed bundled riverdeck package at %s", defaultPkgDest)
	}

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

	// Create root context for the application.
	a.ctx, a.cancel = context.WithCancel(context.Background())

	// Open each hardware device that has a display.
	for _, info := range devices {
		if info.Model.PixelSize == 0 {
			log.Printf("[skip] %s (serial %s) has no display (e.g., Pedal)", info.Model.Name, info.Serial)
			continue
		}

		hwDev, openErr := streamdeck.OpenWithConfig(info.Path, a.config.Performance.JPEGQuality)
		if openErr != nil {
			log.Printf("[!] Failed to open %s (serial %s): %v", info.Model.Name, info.Serial, openErr)
			continue
		}

		fmt.Printf("Opened %s (serial %s)\n", info.Model.Name, info.Serial)

		// Resolve the session config dir per multi-mode.
		sessionDir := platform.DeviceSessionDir(absDir, info.Serial, a.config.Device.MultiMode)
		if a.config.Device.MultiMode == "individual" {
			if mkErr := os.MkdirAll(sessionDir, 0755); mkErr != nil {
				log.Printf("[!] Could not create device config dir %s: %v", sessionDir, mkErr)
			}
		}

		// Merge device-level config on top of global.
		sessionConfig := LoadDeviceConfig(a.config, sessionDir)

		onShutdown := func() { a.cancel() }
		onRestart := func() {
			a.restartRequested = true
			a.cancel()
		}

		session, sessErr := NewDeviceSession(hwDev, info, sessionDir, absDir, sessionConfig, a.ctx, onShutdown, onRestart)
		if sessErr != nil {
			log.Printf("[!] Session creation failed for %s: %v", info.Serial, sessErr)
			hwDev.Close()
			continue
		}

		a.sessions = append(a.sessions, session)
	}

	if len(a.sessions) == 0 {
		return fmt.Errorf("no usable Stream Deck devices could be opened")
	}

	// Start WebSocket device server when enabled (any navigation style).
	if a.config.Network.WebSocketEnabled {
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

	a.startEditorServer()

	return nil
}

// startEditorServer starts an HTTP server that serves the layout editor UI and
// API when network.editor_enabled is true in the config.  The server binds to
// network.editor_host (default "127.0.0.1") to avoid accidental LAN exposure.
func (a *App) startEditorServer() {
	if !a.config.Network.EditorEnabled {
		return
	}
	if len(a.sessions) == 0 {
		log.Println("[!] Editor: no device sessions available, skipping")
		return
	}

	s := a.sessions[0]
	dev := s.device

	pkgs, _ := scripting.ScanPackages(a.configPath)

	edSrv := editorserver.New(editorserver.Config{
		ConfigDir: a.configPath,
		Packages:  pkgs,
		Device: editorserver.DeviceDimensions{
			Cols:      dev.Cols(),
			Rows:      dev.Rows(),
			Keys:      dev.Keys(),
			ModelName: dev.ModelName(),
		},
		OnLayoutSaved: func(l *layout.Layout) {
			for _, sess := range a.sessions {
				sess.reloadLayout(l)
			}
		},
		GetMode: func() string {
			return a.config.UI.NavigationStyle
		},
		OnModeChanged: func(style string) {
			a.config.UI.NavigationStyle = style
		},
	})

	mux := http.NewServeMux()
	mux.Handle("/api/", edSrv.Handler())
	mux.Handle("/", http.FileServer(http.FS(resources.EditorAssetsFS())))

	port := a.config.Network.EditorPort
	if port == 0 {
		port = 9001
	}
	host := a.config.Network.EditorHost
	if host == "" {
		host = "127.0.0.1"
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	a.editorAddr = addr
	go func() {
		log.Printf("[*] Layout editor available at http://%s", addr)
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Printf("[!] Editor server stopped: %v", err)
		}
	}()
}

// Run starts all device sessions (each in its own goroutine). It blocks
// until the app context is cancelled (shutdown / restart / signal).
func (a *App) Run() error {
	if len(a.sessions) == 0 {
		return fmt.Errorf("no sessions to run")
	}

	log.Printf("[*] Starting %d device session(s)...", len(a.sessions))
	for _, s := range a.sessions {
		go s.Run()
	}

	// Wait for context cancellation (triggered by exit, restart, or signal).
	<-a.ctx.Done()
	fmt.Println("\nDone!")
	return nil
}

// Shutdown tears down all resources, blanking each device on exit.
func (a *App) Shutdown() {
	for _, s := range a.sessions {
		s.Shutdown()
	}
	streamdeck.Exit()
}

// OpenEditor opens the system browser at the running layout editor's URL.
func (a *App) OpenEditor() {
	if a.editorAddr == "" {
		log.Println("[!] Editor server is not running (network.editor_enabled is false)")
		return
	}
	platform.OpenBrowser("http://" + a.editorAddr)
}

// migrateConfigDir performs one-time migrations from the old config directory
// layout to the new structure on first boot after an upgrade:
//
//   - config.yml       → .config.yml
//   - .packages/       → .config/packages/
//   - .devices/        → .config/devices/
func migrateConfigDir(configDir string) {
	newConfig := platform.ConfigFile(configDir)
	oldConfig := filepath.Join(configDir, "config.yml")
	if _, err := os.Stat(oldConfig); err == nil {
		if _, err2 := os.Stat(newConfig); os.IsNotExist(err2) {
			if err3 := os.Rename(oldConfig, newConfig); err3 == nil {
				log.Printf("[migration] config.yml → .config.yml")
			}
		}
	}

	newPkgs := platform.PackagesDir(configDir)
	oldPkgs := filepath.Join(configDir, ".packages")
	if _, err := os.Stat(oldPkgs); err == nil {
		if _, err2 := os.Stat(newPkgs); os.IsNotExist(err2) {
			if err3 := os.MkdirAll(filepath.Dir(newPkgs), 0755); err3 == nil {
				if err4 := os.Rename(oldPkgs, newPkgs); err4 == nil {
					log.Printf("[migration] .packages/ → .config/packages/")
				}
			}
		}
	}

	newDevices := platform.DevicesDir(configDir)
	oldDevices := filepath.Join(configDir, ".devices")
	if _, err := os.Stat(oldDevices); err == nil {
		if _, err2 := os.Stat(newDevices); os.IsNotExist(err2) {
			if err3 := os.MkdirAll(filepath.Dir(newDevices), 0755); err3 == nil {
				if err4 := os.Rename(oldDevices, newDevices); err4 == nil {
					log.Printf("[migration] .devices/ → .config/devices/")
				}
			}
		}
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
