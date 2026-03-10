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
// subdirectory — the directory name is used as the package ID.
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
	} `json:"provides"`

	// Requires lists IDs of other packages that must be installed.
	// A missing dependency causes a warning at boot, not a hard failure.
	Requires []string `json:"requires"`

	// Daemon is a relative path (from the package root) to a Lua script that
	// is launched as a long-running background daemon when the package is loaded.
	// When absent, ScanPackages auto-detects daemon.lua in the package root.
	// Set to "-" to explicitly disable auto-detection.
	Daemon string `json:"daemon"`
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
}

// ScanPackages reads <configDir>/.packages/ and returns all installed packages
// in deterministic (alphabetical by ID) order.
//
// A missing .packages/ directory is silently ignored (nil, nil is returned).
// Individual packages with unreadable or malformed manifest.json are logged and
// still included — the lib/ directory can still be found without a manifest.
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

		packages = append(packages, pkg)
	}

	// Sort alphabetically by ID so the require path order is deterministic.
	sort.Slice(packages, func(i, j int) bool {
		return packages[i].Manifest.ID < packages[j].Manifest.ID
	})

	return packages, nil
}
