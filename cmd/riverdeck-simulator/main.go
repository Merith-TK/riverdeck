// Command riverdeck-simulator runs a graphical Stream Deck emulator that
// connects to a running riverdeck instance as a WebSocket device client.
//
// Usage:
//
//	riverdeck-simulator -probe probe.json
//	riverdeck-simulator -probe probe.json -index 0 -riverdeck ws://localhost:9000/ws -http 7002
//
// The simulator opens a browser window at http://localhost:7002 showing an
// interactive grid of keys. Clicking a key fires press/release events that are
// forwarded to the riverdeck process over WebSocket. Images and labels pushed
// by riverdeck are displayed in real time.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/merith-tk/riverdeck/pkg/platform"
)

// SimSpec holds the subset of a ProbeResult needed to run the simulator.
// It matches the JSON field names produced by riverdeck-debug-prober.
type SimSpec struct {
	ModelName    string `json:"model_name"`
	VendorID     uint16 `json:"vendor_id"`
	ProductID    uint16 `json:"product_id"`
	Cols         int    `json:"cols"`
	Rows         int    `json:"rows"`
	Keys         int    `json:"keys"`
	PixelSize    int    `json:"pixel_size"`
	ImageFormat  string `json:"image_format"`
	Manufacturer string `json:"manufacturer"`
	Product      string `json:"product"`
	Serial       string `json:"serial"`
	Firmware     string `json:"firmware"`
}

func main() {
	probeFile  := flag.String("probe", "", "Path to probe JSON file produced by riverdeck-debug-prober  (required)")
	probeIndex := flag.Int("index", 0, "Index of the device entry to simulate when the probe file contains multiple devices")
	rdAddr     := flag.String("riverdeck", "ws://localhost:9000/ws", "WebSocket address of the riverdeck server")
	httpPort   := flag.Int("http", 7002, "HTTP port for the browser UI")
	httpAddr   := flag.String("http-addr", "localhost", "Address to bind the HTTP server to")
	deviceID   := flag.String("device-id", "", "Stable device ID (auto-generated and persisted if empty)")
	noOpen     := flag.Bool("no-open", false, "Do not automatically open the browser")
	flag.Parse()

	if *probeFile == "" {
		flag.Usage()
		fmt.Fprintln(os.Stderr, "\nError: -probe <file> is required.")
		os.Exit(1)
	}

	spec, err := loadProbeSpec(*probeFile, *probeIndex)
	if err != nil {
		log.Fatalf("load probe: %v", err)
	}

	if err := validateSpec(spec); err != nil {
		log.Fatalf("invalid probe data: %v", err)
	}

	if spec.ImageFormat == "" {
		spec.ImageFormat = "JPEG"
	}
	if spec.PixelSize == 0 {
		spec.PixelSize = 72
	}
	if spec.ModelName == "" {
		spec.ModelName = fmt.Sprintf("Simulated Device (PID 0x%04X)", spec.ProductID)
	}

	// Resolve device ID: flag > persisted file > generate new.
	devID := *deviceID
	if devID == "" {
		devID = loadOrCreateDeviceID()
	}

	log.Printf("Simulating: %s  (%d cols x %d rows, %d keys, %dpx, %s)",
		spec.ModelName, spec.Cols, spec.Rows, spec.Keys, spec.PixelSize, spec.ImageFormat)
	log.Printf("Device ID: %s", devID)

	state := newSimState(spec, devID, *rdAddr, *httpPort)

	// Connect to riverdeck WS server in the background (auto-reconnects).
	go state.connectAndRun()

	// Start HTTP server for browser in the background.
	go state.startHTTPServer()

	browserURL := fmt.Sprintf("http://%s:%d", *httpAddr, *httpPort)
	log.Printf("Browser UI: %s", browserURL)
	log.Printf("Connecting to riverdeck at: %s", *rdAddr)

	if !*noOpen {
		platform.OpenBrowser(browserURL)
	}

	// Block forever -- goroutines drive the work.
	select {}
}

// loadOrCreateDeviceID reads ~/.riverdeck/.sim-device-id or generates and saves a new one.
func loadOrCreateDeviceID() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return uuid.New().String()
	}
	dir := filepath.Join(home, ".riverdeck")
	_ = os.MkdirAll(dir, 0755)
	path := filepath.Join(dir, ".sim-device-id")

	if data, err := os.ReadFile(path); err == nil {
		if id := strings.TrimSpace(string(data)); id != "" {
			return id
		}
	}
	id := uuid.New().String()
	_ = os.WriteFile(path, []byte(id+"\n"), 0644)
	return id
}

// loadProbeSpec reads a probe JSON file and returns the SimSpec at the given index.
// The file may contain a single ProbeResult object or an array of them.
func loadProbeSpec(path string, index int) (SimSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SimSpec{}, fmt.Errorf("read %s: %w", path, err)
	}

	var arr []SimSpec
	if err := json.Unmarshal(data, &arr); err == nil {
		if len(arr) == 0 {
			return SimSpec{}, fmt.Errorf("probe JSON array is empty")
		}
		if index >= len(arr) {
			return SimSpec{}, fmt.Errorf("index %d out of range (file has %d entries)", index, len(arr))
		}
		return arr[index], nil
	}

	var single SimSpec
	if err := json.Unmarshal(data, &single); err != nil {
		return SimSpec{}, fmt.Errorf("parse JSON: %w", err)
	}
	return single, nil
}
