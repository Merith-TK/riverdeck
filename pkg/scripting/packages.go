package scripting

// packages.go - discovers and parses installed packages from the .packages/
// subdirectory of the config root.
//
// Directory layout expected by ScanPackages:
//
//	<configDir>/
//	  .packages/
//	    vendor.pkgname/          ← one directory per package
//	      manifest.json          ← optional package metadata
//	      lib/                   ← Lua libraries (added to package.path)
//	        mylib.lua
//	      buttons/               ← ready-made button scripts (informational)
//	        some_button.lua
//
// manifest.json schema:
//
//	{
//	  "id":          "vendor.pkgname",
//	  "name":        "Human-readable name",
//	  "version":     "1.2.3",
//	  "description": "Short one-liner",
//	  "provides": {
//	    "libraries": ["lib/mylib.lua"],
//	    "buttons":   ["buttons/some_button.lua"]
//	  },
//	  "requires": ["vendor.other"]
//	}
//
// A package without manifest.json is still valid as long as it contains a lib/
// subdirectory -- the directory name is used as the package ID.
//
// Scripts in any directory can then use:
//
//	local mylib = require('mylib')
//
// and Lua will resolve it from .packages/vendor.pkgname/lib/mylib.lua
// automatically, because ScanPackages adds every lib/ path to the
// package.path that gets set on each ScriptRunner.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// MetadataField describes a single editable metadata key for a ButtonTemplate.
// When a layout button references a template, the editor renders a form field
// for every entry in MetadataSchema.  Values are stored in
// layout.LayoutButton.Metadata at the same key.
type MetadataField struct {
	// Key is the Metadata map key, e.g. "url" or "volume".
	Key string `json:"key"`

	// Label is the human-readable field label shown in the editor.
	Label string `json:"label"`

	// Type is the input type: "text", "number", or "bool".
	// Defaults to "text" when absent.
	Type string `json:"type,omitempty"`

	// Default is the pre-filled default value for new buttons.
	Default string `json:"default,omitempty"`

	// Description is an optional one-line hint shown below the field.
	Description string `json:"description,omitempty"`
}

// ButtonTemplate is a reusable button definition shipped inside a package.
// Users reference it in layout.json as "pkg://vendor.pkg/template_id".
type ButtonTemplate struct {
	// ID is the local identifier, e.g. "volume_up".
	// The full reference key is "pkg://<packageID>/<id>".
	ID string `json:"id"`

	// Label is the default button label text.
	Label string `json:"label"`

	// Icon is a relative path (from the package root) to the default icon image.
	Icon string `json:"icon,omitempty"`

	// Script is a relative path (from the package root) to the Lua script.
	Script string `json:"script"`

	// Description is a short human-readable description shown in the editor.
	Description string `json:"description,omitempty"`

	// MetadataSchema defines the per-instance configurable fields for this
	// template.  The editor renders a form field for each entry.
	MetadataSchema []MetadataField `json:"metadata_schema,omitempty"`
}

// PackageManifest is the parsed contents of a package's manifest.json.
// All fields are optional; absent fields receive zero values.
type PackageManifest struct {
	// ID is the canonical package identifier, e.g. "vendor.pkgname".
	// When absent, the containing directory name is used as the ID.
	ID string `json:"id"`

	// Name is a human-readable display name.
	Name string `json:"name"`

	// Version is a free-form version string (e.g. "1.0.0" or "2024-03-01").
	Version string `json:"version"`

	// Description is a short one-line description shown during boot.
	Description string `json:"description"`

	// Provides declares what the package ships.
	Provides struct {
		// Libraries lists relative paths to Lua library files (informational).
		Libraries []string `json:"libraries"`
		// Buttons lists relative paths to pre-built button scripts (informational).
		Buttons []string `json:"buttons"`
		// IconPacks lists relative paths to directories containing image files
		// that can be referenced by layout buttons as "vendor.pkg/icons/name.png".
		IconPacks []string `json:"icon_packs"`
		// Templates is an inline list of reusable button templates this package ships.
		Templates []ButtonTemplate `json:"templates"`
	} `json:"provides"`

	// Requires lists IDs of other packages that must be installed.
	// A missing dependency causes a warning at boot, not a hard failure.
	Requires []string `json:"requires"`

	// Daemon is a relative path (from the package root) to a Lua script that
	// is launched as a long-running background daemon when the package is loaded.
	// When absent, ScanPackages auto-detects daemon.lua in the package root.
	// Set to "-" to explicitly disable auto-detection.
	Daemon string `json:"daemon"`

	// OrderIndex controls the position of this package in the editor's package
	// browser panel.  Lower values sort first; packages without an OrderIndex
	// (zero value) sort after those with explicit positive values, then
	// alphabetically by ID.
	OrderIndex int `json:"order_index,omitempty"`
}

// ScannedPackage holds a resolved package ready to be used by ScriptManager.
type ScannedPackage struct {
	// Manifest holds the parsed (or defaulted) package metadata.
	Manifest PackageManifest

	// Dir is the absolute path to the package's root directory.
	Dir string

	// LibDir is the absolute path to the lib/ subdirectory, or empty string
	// when no lib/ directory exists.
	LibDir string

	// DaemonScript is the absolute path to the daemon Lua script, or empty
	// string when the package ships no daemon.
	DaemonScript string

	// DataDir is the absolute path to the package's persistent data directory:
	//   <configDir>/.packages/<vendor.pkg>/data/
	// This directory is created by ScanPackages if it does not yet exist.
	// It is passed to daemon runners as their packageDataDir so they receive
	// the pkg_data module scoped to this path.
	DataDir string

	// ResolvedTemplates is the list of this package's button templates with
	// Script and Icon fields converted to absolute paths.
	ResolvedTemplates []ResolvedButtonTemplate

	// ResolvedIconPackDirs is the list of absolute directory paths for this
	// package's icon packs (one per entry in Manifest.Provides.IconPacks).
	ResolvedIconPackDirs []string
}

// ResolvedButtonTemplate is a ButtonTemplate whose Script and Icon paths have
// been converted to absolute filesystem paths.
type ResolvedButtonTemplate struct {
	// PackageID is the ID of the package that provides this template.
	PackageID string
	// Key is the full reference key: "pkg://<packageID>/<template.ID>".
	Key string
	// Template is the original manifest entry (including MetadataSchema).
	Template ButtonTemplate
	// AbsScript is the absolute path to the Lua script.
	AbsScript string
	// AbsIcon is the absolute path to the icon image, or empty if none.
	AbsIcon string
}

// ScanPackages reads <configDir>/.packages/ and returns all installed packages
// in deterministic (alphabetical by ID) order.
//
// A missing .packages/ directory is silently ignored (nil, nil is returned).
// Individual packages with unreadable or malformed manifest.json are logged and
// still included -- the lib/ directory can still be found without a manifest.
func ScanPackages(configDir string) ([]*ScannedPackage, error) {
	packagesDir := filepath.Join(configDir, ".packages")

	entries, err := os.ReadDir(packagesDir)
	if os.IsNotExist(err) {
		return nil, nil // no packages directory - nothing to do
	}
	if err != nil {
		return nil, fmt.Errorf("scanning .packages: %w", err)
	}

	var packages []*ScannedPackage

	for _, entry := range entries {
		if !entry.IsDir() {
			continue // only directories are packages
		}

		pkgDir := filepath.Join(packagesDir, entry.Name())
		pkg := &ScannedPackage{Dir: pkgDir}

		// Parse manifest.json (optional - missing file is not an error).
		manifestPath := filepath.Join(pkgDir, "manifest.json")
		if data, readErr := os.ReadFile(manifestPath); readErr == nil {
			if jsonErr := json.Unmarshal(data, &pkg.Manifest); jsonErr != nil {
				fmt.Printf("[!] Package %s: invalid manifest.json: %v\n", entry.Name(), jsonErr)
				// Fall through - still try to use lib/ even with a bad manifest.
			}
		}

		// Use the directory name as ID when the manifest provides none.
		if pkg.Manifest.ID == "" {
			pkg.Manifest.ID = entry.Name()
		}

		// Detect lib/ subdirectory.
		libDir := filepath.Join(pkgDir, "lib")
		if info, statErr := os.Stat(libDir); statErr == nil && info.IsDir() {
			pkg.LibDir = libDir
		}

		// Resolve daemon script path.
		// Priority: manifest.Daemon > auto-detect daemon.lua.
		// A manifest value of "-" disables daemon entirely.
		switch pkg.Manifest.Daemon {
		case "-":
			// Explicitly disabled - leave DaemonScript empty.
		case "":
			// Auto-detect: use daemon.lua if it exists in the package root.
			auto := filepath.Join(pkgDir, "daemon.lua")
			if _, statErr := os.Stat(auto); statErr == nil {
				pkg.DaemonScript = auto
			}
		default:
			// Manifest-specified path (relative to package root).
			pkg.DaemonScript = filepath.Join(pkgDir, pkg.Manifest.Daemon)
		}

		// Resolve and pre-create the package data directory.
		// This is always <pkgDir>/data/ regardless of the manifest.
		pkg.DataDir = filepath.Join(pkgDir, "data")
		if err := os.MkdirAll(pkg.DataDir, 0755); err != nil {
			fmt.Printf("[!] Package %s: failed to create data dir: %v\n", pkg.Manifest.ID, err)
			pkg.DataDir = "" // daemon will proceed without pkg_data
		}

		// Resolve button templates to absolute paths.
		for _, tmpl := range pkg.Manifest.Provides.Templates {
			rt := ResolvedButtonTemplate{
				PackageID: pkg.Manifest.ID,
				Key:       "pkg://" + pkg.Manifest.ID + "/" + tmpl.ID,
				Template:  tmpl,
			}
			if tmpl.Script != "" {
				rt.AbsScript = filepath.Join(pkgDir, tmpl.Script)
			}
			if tmpl.Icon != "" {
				rt.AbsIcon = filepath.Join(pkgDir, tmpl.Icon)
			}
			pkg.ResolvedTemplates = append(pkg.ResolvedTemplates, rt)
		}

		// Resolve icon pack directories to absolute paths.
		for _, rel := range pkg.Manifest.Provides.IconPacks {
			abs := filepath.Join(pkgDir, rel)
			if info, statErr := os.Stat(abs); statErr == nil && info.IsDir() {
				pkg.ResolvedIconPackDirs = append(pkg.ResolvedIconPackDirs, abs)
			}
		}

		packages = append(packages, pkg)
	}

	// Sort by OrderIndex first (ascending), then alphabetically by ID.
	// Packages with OrderIndex==0 sort after those with positive OrderIndex.
	sort.Slice(packages, func(i, j int) bool {
		oi, oj := packages[i].Manifest.OrderIndex, packages[j].Manifest.OrderIndex
		if oi != oj {
			// OrderIndex 0 (unset) sorts after any explicit positive value.
			if oi == 0 {
				return false
			}
			if oj == 0 {
				return true
			}
			return oi < oj
		}
		return packages[i].Manifest.ID < packages[j].Manifest.ID
	})

	return packages, nil
}
