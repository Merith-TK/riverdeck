// Package layout defines a declarative page-based layout model for Stream Deck
// button grids.  A Layout contains named pages, each with positioned buttons
// that reference scripts, icons, and navigation targets.  The package handles
// loading, saving, and validating layout.json files.
package layout

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Validate checks a Layout for structural correctness and returns a list of
// human-readable error strings.  An empty slice means the layout is valid.
//
// Current checks:
//   - The layout must have at least one page.
//   - Every page must contain exactly one button with action "home".
func Validate(l *Layout) []string {
	var errs []string
	if len(l.Pages) == 0 {
		errs = append(errs, "layout has no pages")
		return errs
	}
	for _, page := range l.Pages {
		count := 0
		for _, btn := range page.Buttons {
			if btn.Action == "home" {
				count++
			}
		}
		name := page.Name
		if name == "" {
			name = "(unnamed)"
		}
		switch count {
		case 0:
			errs = append(errs, fmt.Sprintf("page %q is missing a SET/HOME button (action: \"home\")", name))
		case 1:
			// good
		default:
			errs = append(errs, fmt.Sprintf("page %q has %d SET/HOME buttons (action: \"home\"); exactly one required", name, count))
		}
	}
	return errs
}

const layoutFileName = "layout.json"

// LayoutPath returns the canonical path of the layout file inside configDir.
func LayoutPath(configDir string) string {
	return filepath.Join(configDir, layoutFileName)
}

// DeviceLayoutDir returns the per-device configuration directory for the given
// device identifier.  Hardware devices use their serial number; software clients
// use a UUID.  The layout.json for that device lives at:
//
//	configDir/devices/{deviceID}/layout.json
func DeviceLayoutDir(configDir, deviceID string) string {
	return filepath.Join(configDir, "devices", deviceID)
}

// Exists reports whether a layout.json file exists in configDir.
func Exists(configDir string) bool {
	_, err := os.Stat(LayoutPath(configDir))
	return err == nil
}

// Load reads layout.json from configDir and parses it.
// Returns (nil, nil) when the file does not exist so callers can fall back
// gracefully to the file-browser navigator.
func Load(configDir string) (*Layout, error) {
	data, err := os.ReadFile(LayoutPath(configDir))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading layout.json: %w", err)
	}

	var l Layout
	if err := json.Unmarshal(data, &l); err != nil {
		return nil, fmt.Errorf("parsing layout.json: %w", err)
	}
	return &l, nil
}

// Save writes l as layout.json into configDir (creating the directory if needed).
// The file is written atomically: it is first written to a temp file and then
// renamed so a concurrent reader never sees a half-written file.
func Save(configDir string, l *Layout) error {
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling layout: %w", err)
	}

	tmp := LayoutPath(configDir) + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("writing layout.json.tmp: %w", err)
	}
	if err := os.Rename(tmp, LayoutPath(configDir)); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("renaming layout.json.tmp: %w", err)
	}
	return nil
}
