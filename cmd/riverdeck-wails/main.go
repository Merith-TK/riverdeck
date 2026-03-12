// Command riverdeck-wails is the native Wails v2 layout editor for Riverdeck.
//
// The embedded editor HTML/JS is served from Go-embedded assets while /api/*
// routes are handled in-process by the editorserver HTTP handlers -- no
// external port is opened.
package main

import (
	"flag"
	"log"
	"os"

	"github.com/merith-tk/riverdeck/pkg/editorserver"
	"github.com/merith-tk/riverdeck/pkg/platform"
	"github.com/merith-tk/riverdeck/pkg/scripting"
	"github.com/merith-tk/riverdeck/resources"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

var (
	flagConfigDir = flag.String("configdir", "", "Config directory (default: platform default)")
	flagCols      = flag.Int("cols", 5, "Device column count for editor grid")
	flagRows      = flag.Int("rows", 3, "Device row count for editor grid")
	flagModel     = flag.String("model", "Stream Deck", "Device model name shown in editor")
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	flag.Parse()

	configDir := platform.ConfigDir(*flagConfigDir)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		log.Fatalf("Failed to create config directory %s: %v", configDir, err)
	}
	log.Printf("[*] Config directory: %s", configDir)

	// Scan packages (best-effort; non-fatal if none found).
	pkgs, err := scripting.ScanPackages(configDir)
	if err != nil {
		log.Printf("[!] Package scan: %v (continuing without packages)", err)
		pkgs = nil
	}

	cols := *flagCols
	rows := *flagRows

	srv := editorserver.New(editorserver.Config{
		ConfigDir: configDir,
		Packages:  pkgs,
		Device: editorserver.DeviceDimensions{
			Cols:      cols,
			Rows:      rows,
			Keys:      cols * rows,
			ModelName: *flagModel,
		},
	})

	apiHandler := srv.Handler()

	if err := wails.Run(&options.App{
		Title:  "Riverdeck Editor",
		Width:  1100,
		Height: 720,
		AssetServer: &assetserver.Options{
			Assets:  resources.EditorAssetsFS(),
			Handler: apiHandler,
		},
	}); err != nil {
		log.Fatalf("Wails error: %v", err)
	}
}
