// Command riverdeck-debug-prober probes every connected Elgato Stream Deck device.
// By default it opens a graphical wizard; pass -c for CLI-only terminal output.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"github.com/merith-tk/riverdeck/pkg/streamdeck"
	"github.com/sstallion/go-hid"
)

func main() {
	cliMode := flag.Bool("c", false, "CLI mode: print terminal report instead of launching GUI")
	outputPath := flag.String("output", "", "CLI: write JSON report to this file (e.g. probe.json)")
	listenSec := flag.Int("listen", 3, "CLI: seconds to listen for key events (0 = skip)")
	allReports := flag.Bool("all-reports", false, "CLI: probe all feature report IDs 0x00\u20130x2F (slower)")
	flag.Parse()

	if *cliMode {
		runCLI(*outputPath, *listenSec, *allReports)
		return
	}

	// Default: graphical wizard.
	a := app.New()
	w := a.NewWindow("Stream Deck Device Prober")
	w.Resize(fyne.NewSize(720, 520))
	w.SetFixedSize(false)

	state := newAppState(a, w)
	state.showStep(stepSetup)

	w.ShowAndRun()
}

func runCLI(outputPath string, listenSec int, allReports bool) {
	listenDur := time.Duration(listenSec) * time.Second

	if err := streamdeck.Init(); err != nil {
		fatalf("HID init failed: %v\n", err)
	}
	defer streamdeck.Exit()

	// Enumerate ALL HID devices first and filter by VendorID manually.
	// On Windows, passing a non-zero VID to hid.Enumerate can miss devices
	// depending on HIDAPI version and exclusive-access state.
	var rawDevices []hid.DeviceInfo
	var allCount int
	err := hid.Enumerate(0x0000, 0x0000, func(info *hid.DeviceInfo) error {
		allCount++
		if info.VendorID == streamdeck.VendorID {
			rawDevices = append(rawDevices, *info)
		}
		return nil
	})
	if err != nil {
		fatalf("Enumeration failed: %v\n", err)
	}

	if len(rawDevices) == 0 {
		fmt.Printf("No Elgato devices found (scanned %d total HID devices).\n", allCount)
		if allCount == 0 {
			fmt.Println("No HID devices were visible at all -- try running as Administrator.")
		} else {
			fmt.Println("Tip: close Elgato Stream Deck software if it holds exclusive access,")
			fmt.Println("     or try running this tool as Administrator.")
		}
		return
	}

	fmt.Printf("Found %d Elgato device(s). Probing...\n\n", len(rawDevices))

	var results []ProbeResult

	for i, raw := range rawDevices {
		fmt.Printf("[%d/%d] Probing %s (PID 0x%04X) @ %s\n",
			i+1, len(rawDevices), raw.ProductStr, raw.ProductID, raw.Path)

		result := ProbeDevice(raw, listenDur, allReports)
		PrintReport(result)
		results = append(results, result)
	}

	if outputPath != "" {
		if err := writeJSON(outputPath, results); err != nil {
			fmt.Fprintf(os.Stderr, "WARNING: could not write JSON: %v\n", err)
		} else {
			fmt.Printf("\nJSON report written to: %s\n", outputPath)
		}
	}
}

func writeJSON(path string, results []ProbeResult) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(results)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "ERROR: "+format, args...)
	os.Exit(1)
}
