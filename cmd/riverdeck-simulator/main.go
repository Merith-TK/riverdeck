// Command riverdeck-simulator runs a graphical Stream Deck emulator backed by
// a probe-JSON dump produced by riverdeck-debug-prober.
//
// Usage:
//
//	riverdeck-simulator -probe probe.json
//	riverdeck-simulator -probe probe.json -index 0 -port 7001 -http 7002
//
// Point riverdeck at the simulator with:
//
//	riverdeck --sim localhost:7001
//
// The simulator opens a browser window at http://localhost:7002 showing an
// interactive grid of keys. Clicking a key fires press/release events that are
// forwarded to the riverdeck process. Image writes from riverdeck are displayed
// on the matching key button in real time.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/merith-tk/riverdeck/pkg/platform"
)

func main() {
	probeFile := flag.String("probe", "", "Path to probe JSON file produced by riverdeck-debug-prober  (required)")
	probeIndex := flag.Int("index", 0, "Index of the device entry to simulate when the probe file contains multiple devices")
	tcpPort := flag.Int("port", 7001, "TCP port for riverdeck to connect to  (riverdeck --sim localhost:<port>)")
	httpPort := flag.Int("http", 7002, "HTTP port for the browser UI")
	httpAddr := flag.String("http-addr", "localhost", "Address to bind the HTTP server to (default: localhost)")
	noOpen := flag.Bool("no-open", false, "Do not automatically open the browser")
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

	// Ensure image format is set (default to JPEG for older probes).
	if spec.ImageFormat == "" {
		spec.ImageFormat = "JPEG"
	}
	// Ensure pixel size has a sane default.
	if spec.PixelSize == 0 {
		spec.PixelSize = 72
	}

	log.Printf("Simulating: %s  (%d cols x %d rows, %d keys, %dpx, %s)",
		spec.ModelName, spec.Cols, spec.Rows, spec.Keys, spec.PixelSize, spec.ImageFormat)

	state := newSimState(spec, *tcpPort, *httpPort)

	// Start TCP server for riverdeck in the background.
	go state.startTCPServer()

	// Start HTTP server for browser in the background.
	go state.startHTTPServer()

	// Give servers a moment to bind.
	time.Sleep(200 * time.Millisecond)

	browserURL := fmt.Sprintf("http://%s:%d", *httpAddr, *httpPort)
	log.Printf("Browser UI: %s", browserURL)
	log.Printf("Connect riverdeck with: riverdeck --sim %s:%d", *httpAddr, *tcpPort)

	if !*noOpen {
		platform.OpenBrowser(browserURL)
	}

	// Block forever -- servers run in goroutines.
	select {}
}

// loadProbeSpec reads a probe JSON file and returns the SimSpec at the given index.
// The file may contain a single ProbeResult object or an array of them.
func loadProbeSpec(path string, index int) (SimSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SimSpec{}, fmt.Errorf("read %s: %w", path, err)
	}

	// Try array first (the prober writes an array when multiple devices are probed).
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

	// Fall back to single object.
	var single SimSpec
	if err := json.Unmarshal(data, &single); err != nil {
		return SimSpec{}, fmt.Errorf("parse JSON: %w", err)
	}
	return single, nil
}
