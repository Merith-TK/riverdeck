package pkgmanager

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// packagesFile is the combined index + config file name inside the packages dir.
const packagesFile = "packages.json"

// PackageEntry is one entry in packages.json, combining import-resolver info
// with per-installation settings.
type PackageEntry struct {
	// Path is the physical directory under .config/packages/.
	// For git-installed packages this is the RepoDir, e.g.
	// "github.com/merith-tk/riverdeck-packages".
	Path string `json:"path"`

	// Packages lists sub-package IDs within a multi-package repo.
	// Nil or empty for single-package repos.
	Packages []string `json:"packages,omitempty"`

	// DaemonEnabled controls whether the package-level daemon is started.
	// For multi-package repos, use SubPackages instead.
	DaemonEnabled *bool `json:"daemon_enabled,omitempty"`

	// UpdateChannel is "release" (track tags) or "branch:<name>" (dev mode).
	UpdateChannel string `json:"update_channel,omitempty"`

	// PinnedTag pins a specific version tag when UpdateChannel is "release".
	PinnedTag string `json:"pinned_tag,omitempty"`

	// SubPackages holds per-sub-package settings for multi-package repos.
	SubPackages map[string]SubEntry `json:"sub_packages,omitempty"`
}

// SubEntry holds per-sub-package settings inside a multi-package repo.
type SubEntry struct {
	DaemonEnabled *bool `json:"daemon_enabled,omitempty"`
}

// PackagesFile is the in-memory representation of packages.json.
// Keys are dot-separated shorthand import names, e.g. "merith-tk.riverdeck-packages".
type PackagesFile map[string]PackageEntry

// LoadPackages reads packages.json from the packages directory.
// Returns an empty PackagesFile if the file does not exist.
func LoadPackages(packagesDir string) (PackagesFile, error) {
	data, err := os.ReadFile(filepath.Join(packagesDir, packagesFile))
	if os.IsNotExist(err) {
		return make(PackagesFile), nil
	}
	if err != nil {
		return nil, err
	}
	var pf PackagesFile
	if err := json.Unmarshal(data, &pf); err != nil {
		return nil, err
	}
	if pf == nil {
		pf = make(PackagesFile)
	}
	return pf, nil
}

// SavePackages writes pf to the packages directory as packages.json.
func SavePackages(packagesDir string, pf PackagesFile) error {
	if err := os.MkdirAll(packagesDir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(pf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(packagesDir, packagesFile), data, 0644)
}

// IsDaemonEnabled returns whether the daemon is enabled for a given key and
// optional sub-package name. sub may be empty for single-package repos.
// Defaults to false when no explicit setting exists.
func (pf PackagesFile) IsDaemonEnabled(key, sub string) bool {
	entry, ok := pf[key]
	if !ok {
		return false
	}
	if sub != "" {
		if sp, ok := entry.SubPackages[sub]; ok && sp.DaemonEnabled != nil {
			return *sp.DaemonEnabled
		}
		return false
	}
	if entry.DaemonEnabled != nil {
		return *entry.DaemonEnabled
	}
	return false
}

// SetDaemonEnabled sets the daemon-enabled flag for a key (and optional sub-package).
func (pf PackagesFile) SetDaemonEnabled(key, sub string, enabled bool) {
	entry := pf[key]
	if sub != "" {
		if entry.SubPackages == nil {
			entry.SubPackages = make(map[string]SubEntry)
		}
		sp := entry.SubPackages[sub]
		sp.DaemonEnabled = &enabled
		entry.SubPackages[sub] = sp
	} else {
		entry.DaemonEnabled = &enabled
	}
	pf[key] = entry
}

// Resolve resolves a dot-separated Lua module name to an absolute file path.
// Returns ("", false) when the module name is not in the packages file.
//
// Resolution for "merith-tk.riverdeck-packages.ytmd.lib.api":
//  1. Try progressively longer dot-prefixes as keys.
//  2. On match, consume next segment as sub-package if Packages is non-empty.
//  3. Remaining segments → path joined with "/" + ".lua".
func (pf PackagesFile) Resolve(moduleName, packagesDir string) (string, bool) {
	dotParts := strings.Split(moduleName, ".")
	for prefixLen := len(dotParts); prefixLen >= 1; prefixLen-- {
		key := strings.Join(dotParts[:prefixLen], ".")
		entry, ok := pf[key]
		if !ok {
			continue
		}
		rest := dotParts[prefixLen:]
		base := filepath.Join(packagesDir, filepath.FromSlash(entry.Path))

		// If the entry has sub-packages, the next segment is the sub-package dir.
		if len(entry.Packages) > 0 && len(rest) > 0 {
			subpkg := rest[0]
			for _, p := range entry.Packages {
				if p == subpkg {
					base = filepath.Join(base, subpkg)
					rest = rest[1:]
					break
				}
			}
		}

		if len(rest) == 0 {
			return "", false
		}

		candidate := filepath.Join(base, filepath.Join(rest...)) + ".lua"
		return candidate, true
	}
	return "", false
}
