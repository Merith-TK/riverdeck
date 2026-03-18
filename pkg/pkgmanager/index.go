package pkgmanager

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// indexFile is the name of the import-resolver index inside the packages dir.
const indexFile = ".index.json"

// IndexEntry is one entry in the import resolver index.
type IndexEntry struct {
	// Path is the physical directory under .config/packages/.
	// For git-installed packages this is the RepoDir, e.g.
	// "github.com/merith-tk/riverdeck-packages".
	Path string `json:"path"`

	// Packages lists sub-package IDs within a multi-package repo.
	// Nil or empty for single-package repos.
	Packages []string `json:"packages,omitempty"`
}

// ImportIndex maps shorthand import names to physical package locations.
//
// Example entry: "merith-tk.riverdeck-packages" → {Path: "github.com/…", Packages: ["ytmd","obs"]}
type ImportIndex map[string]IndexEntry

// LoadIndex reads the .index.json from the packages directory.
// Returns an empty index if the file does not exist.
func LoadIndex(packagesDir string) (ImportIndex, error) {
	data, err := os.ReadFile(filepath.Join(packagesDir, indexFile))
	if os.IsNotExist(err) {
		return make(ImportIndex), nil
	}
	if err != nil {
		return nil, err
	}
	var idx ImportIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, err
	}
	if idx == nil {
		idx = make(ImportIndex)
	}
	return idx, nil
}

// SaveIndex writes idx to the packages directory.
func SaveIndex(packagesDir string, idx ImportIndex) error {
	if err := os.MkdirAll(packagesDir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(packagesDir, indexFile), data, 0644)
}

// Resolve resolves a dot-separated Lua module name to an absolute file path
// using the index. Returns ("", false) when the module name is not managed by
// the index.
//
// Resolution algorithm for "merith-tk.riverdeck-packages.ytmd.lib.api":
//  1. Try progressively longer dot-prefixes as shorthand keys.
//  2. On match, consume the next segment as sub-package (if Packages is non-empty).
//  3. Remaining segments → path with "/" separator + ".lua".
//
// Full path: packagesDir/<entry.Path>[/<subpkg>]/<rest>.lua
func (idx ImportIndex) Resolve(moduleName, packagesDir string) (string, bool) {
	// Try each possible prefix length.
	dotParts := strings.Split(moduleName, ".")
	for prefixLen := len(dotParts); prefixLen >= 1; prefixLen-- {
		shorthand := strings.Join(dotParts[:prefixLen], ".")
		entry, ok := idx[shorthand]
		if !ok {
			continue
		}
		rest := dotParts[prefixLen:]
		base := filepath.Join(packagesDir, filepath.FromSlash(entry.Path))

		// If the entry has sub-packages, the next segment is the sub-package dir.
		if len(entry.Packages) > 0 && len(rest) > 0 {
			subpkg := rest[0]
			found := false
			for _, p := range entry.Packages {
				if p == subpkg {
					found = true
					break
				}
			}
			if found {
				base = filepath.Join(base, subpkg)
				rest = rest[1:]
			}
		}

		if len(rest) == 0 {
			// Module name resolved to a directory, not a file — no resolution.
			return "", false
		}

		candidate := filepath.Join(base, filepath.Join(rest...)) + ".lua"
		return candidate, true
	}
	return "", false
}
