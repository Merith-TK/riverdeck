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

// Exists reports whether a layout.json file exists in configDir.
func Exists(configDir string) bool {
	_, err := os.Stat(LayoutPath(configDir))
	return err == nil
}

// LoadFile reads layout.json from configDir and returns a normalised LayoutFile.
// Old-format files ({"pages":[...]}) are automatically promoted to
// Layouts["default"].  Returns (nil, nil) when the file does not exist.
func LoadFile(configDir string) (*LayoutFile, error) {
	data, err := os.ReadFile(LayoutPath(configDir))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading layout.json: %w", err)
	}

	var f LayoutFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parsing layout.json: %w", err)
	}

	// Backward compat: old format had pages at the top level.
	if len(f.Pages) > 0 && len(f.Layouts) == 0 {
		if f.Layouts == nil {
			f.Layouts = make(map[string]*Layout)
		}
		f.Layouts["default"] = &Layout{Pages: f.Pages}
		f.Pages = nil
	}

	return &f, nil
}

// LoadForDevice returns the Layout assigned to deviceID in configDir/layout.json.
// If deviceID has no explicit assignment, "default" is used.
// Returns a new empty Layout when the file does not exist or the layout is absent.
func LoadForDevice(configDir, deviceID string) (*Layout, error) {
	f, err := LoadFile(configDir)
	if err != nil {
		return nil, err
	}
	if f == nil || len(f.Layouts) == 0 {
		return NewEmpty(), nil
	}

	name := "default"
	if deviceID != "" {
		if f.Devices != nil {
			if assigned, ok := f.Devices[deviceID]; ok {
				name = assigned
			}
		}
	}

	if lay, ok := f.Layouts[name]; ok {
		return lay, nil
	}
	// Final fallback: "default" layout (in case a named layout was deleted).
	if name != "default" {
		if lay, ok := f.Layouts["default"]; ok {
			return lay, nil
		}
	}
	return NewEmpty(), nil
}

// SaveLayout updates (or creates) the named layout in configDir/layout.json.
// Other layouts and device assignments in the file are preserved.
func SaveLayout(configDir, name string, lay *Layout) error {
	f, err := LoadFile(configDir)
	if err != nil {
		return err
	}
	if f == nil {
		f = &LayoutFile{}
	}
	if f.Layouts == nil {
		f.Layouts = make(map[string]*Layout)
	}
	f.Layouts[name] = lay
	return writeFile(configDir, f)
}

// DeviceDir returns the per-device cache directory (.devices/{id}/).
// The dot-prefix causes folder-mode navigation to skip the directory.
func DeviceDir(configDir, deviceID string) string {
	return filepath.Join(configDir, ".devices", deviceID)
}

// SaveDeviceGeometry writes a DeviceGeometry snapshot to
// configDir/.devices/{id}/device.json.
func SaveDeviceGeometry(configDir string, g *DeviceGeometry) error {
	dir := DeviceDir(configDir, g.ID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "device.json"), data, 0644)
}

// LoadAllDeviceGeometries reads all device.json files from
// configDir/.devices/*/device.json and returns them.
func LoadAllDeviceGeometries(configDir string) ([]*DeviceGeometry, error) {
	root := filepath.Join(configDir, ".devices")
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []*DeviceGeometry
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(root, e.Name(), "device.json")
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var g DeviceGeometry
		if json.Unmarshal(data, &g) == nil {
			out = append(out, &g)
		}
	}
	return out, nil
}

// AssignDeviceLayout updates the Devices map in configDir/layout.json so that
// deviceID is assigned to layoutName.
func AssignDeviceLayout(configDir, deviceID, layoutName string) error {
	f, err := LoadFile(configDir)
	if err != nil {
		return err
	}
	if f == nil {
		f = &LayoutFile{}
	}
	if f.Devices == nil {
		f.Devices = make(map[string]string)
	}
	f.Devices[deviceID] = layoutName
	return writeFile(configDir, f)
}

// writeFile serialises f to configDir/layout.json atomically.
func writeFile(configDir string, f *LayoutFile) error {
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling layout file: %w", err)
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
