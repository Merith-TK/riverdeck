// Package pkgmanager provides package installation, removal, and update
// management for Riverdeck. Packages are installed into the configured
// packages directory (.config/packages/) from git repositories.
//
// # Install URL formats
//
//	github.com/user/repo                    single-pkg, main branch
//	github.com/user/repo@branch             single-pkg, specific branch
//	github.com/user/repo@v1.2.0             single-pkg, tag
//	github.com/user/repo/ytmd               sub-package from multi-pkg repo
//	github.com/user/repo@v2.0.0/ytmd        sub-package + version pin
package pkgmanager

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/merith-tk/riverdeck/pkg/gitpkg"
	"github.com/merith-tk/riverdeck/pkg/platform"
)

// Manager handles package installation and lifecycle for one config directory.
type Manager struct {
	configDir   string
	packagesDir string
}

// New creates a Manager for the given Riverdeck config directory.
func New(configDir string) *Manager {
	return &Manager{
		configDir:   configDir,
		packagesDir: platform.PackagesDir(configDir),
	}
}

// Install downloads and installs the package identified by rawURL.
//
// The install algorithm:
//  1. Parse URL.
//  2. Clone / update repo to a tmp dir.
//  3. Read riverdeck.pkg.manifest.json at repo root.
//  4. Move repo into final position under packagesDir.
//  5. Update .index.json, riverdeck.lock, packages.cfg.json.
func (m *Manager) Install(rawURL string) error {
	src, err := ParseSource(rawURL)
	if err != nil {
		return fmt.Errorf("parse source: %w", err)
	}

	targetDir := filepath.Join(m.packagesDir, filepath.FromSlash(src.RepoDir))

	// Skip if already installed at exact version.
	if _, err := os.Stat(targetDir); err == nil {
		return fmt.Errorf("package already installed at %s; use Update() to upgrade", targetDir)
	}

	if err := os.MkdirAll(filepath.Dir(targetDir), 0755); err != nil {
		return fmt.Errorf("mkdirall: %w", err)
	}

	log.Printf("[pkgmanager] cloning %s → %s", src.CloneURL(), targetDir)
	depth := 1
	if src.Branch != "" {
		depth = 0 // full clone for branches
	}
	if err := gitpkg.Clone(src.CloneURL(), targetDir, src.Ref(), depth); err != nil {
		return fmt.Errorf("clone: %w", err)
	}

	// Load (or create) packages.json, then add entry for the new package.
	pf, _ := LoadPackages(m.packagesDir)
	shorthand := m.shorthand(src)
	entry := PackageEntry{Path: filepath.ToSlash(src.RepoDir)}

	// Read manifest to detect multi-package repos and populate Packages list.
	manifest, _ := readManifest(targetDir)
	if manifest != nil && len(manifest.Packages) > 0 {
		for _, p := range manifest.Packages {
			entry.Packages = append(entry.Packages, p.ID)
		}
	}
	// Default: daemon disabled.
	disabled := false
	entry.DaemonEnabled = &disabled
	pf[shorthand] = entry
	if err := SavePackages(m.packagesDir, pf); err != nil {
		log.Printf("[pkgmanager] warning: could not save packages.json: %v", err)
	}

	// Update lock.
	lf, _ := LoadLock(m.packagesDir)
	_ = UpdatePackageLock(lf, m.packagesDir, targetDir)
	if err := SaveLock(m.packagesDir, lf); err != nil {
		log.Printf("[pkgmanager] warning: could not save lock: %v", err)
	}

	log.Printf("[pkgmanager] installed %s", filepath.ToSlash(src.RepoDir))
	return nil
}

// Remove uninstalls the package identified by rawURL or shorthand name.
func (m *Manager) Remove(rawURL string) error {
	src, err := ParseSource(rawURL)
	if err != nil {
		// Try treating rawURL as a direct shorthand or directory name.
		return m.removeByDir(rawURL)
	}
	return m.removeByDir(src.RepoDir)
}

func (m *Manager) removeByDir(repoDir string) error {
	targetDir := filepath.Join(m.packagesDir, filepath.FromSlash(repoDir))
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		return fmt.Errorf("package not found: %s", repoDir)
	}
	if err := os.RemoveAll(targetDir); err != nil {
		return fmt.Errorf("remove: %w", err)
	}

	// Remove from lock.
	lf, _ := LoadLock(m.packagesDir)
	RemovePackageLock(lf, m.packagesDir, targetDir)
	_ = SaveLock(m.packagesDir, lf)

	// Remove from packages.json.
	pf, _ := LoadPackages(m.packagesDir)
	for k, entry := range pf {
		if filepath.ToSlash(entry.Path) == filepath.ToSlash(repoDir) {
			delete(pf, k)
		}
	}
	_ = SavePackages(m.packagesDir, pf)

	log.Printf("[pkgmanager] removed %s", repoDir)
	return nil
}

// Update fetches and updates an installed package.
func (m *Manager) Update(rawURL string) error {
	src, err := ParseSource(rawURL)
	if err != nil {
		return fmt.Errorf("parse source: %w", err)
	}
	targetDir := filepath.Join(m.packagesDir, filepath.FromSlash(src.RepoDir))
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		return fmt.Errorf("package not installed: %s", src.RepoDir)
	}

	log.Printf("[pkgmanager] updating %s", src.RepoDir)
	if err := gitpkg.Pull(targetDir); err != nil {
		return fmt.Errorf("pull: %w", err)
	}

	// Refresh lock after update.
	lf, _ := LoadLock(m.packagesDir)
	RemovePackageLock(lf, m.packagesDir, targetDir)
	_ = UpdatePackageLock(lf, m.packagesDir, targetDir)
	_ = SaveLock(m.packagesDir, lf)

	log.Printf("[pkgmanager] updated %s", src.RepoDir)
	return nil
}

// List returns all installed package directories under packagesDir.
func (m *Manager) List() ([]InstalledPackage, error) {
	return listPackages(m.packagesDir)
}

// InstalledPackage describes a single installed package or sub-package.
type InstalledPackage struct {
	// RepoDir is the path relative to packagesDir, e.g. "github.com/user/repo".
	RepoDir string

	// SubPkg is the sub-package ID for multi-package repos, or empty.
	SubPkg string

	// Name is the human-readable name from the manifest.
	Name string

	// Version is the version string from the manifest.
	Version string

	// DaemonEnabled reflects the setting in packages.cfg.json.
	DaemonEnabled bool
}

// listPackages walks the packages dir and returns discovered packages.
func listPackages(packagesDir string) ([]InstalledPackage, error) {
	cfg, _ := LoadPackages(packagesDir)
	var result []InstalledPackage

	_ = filepath.Walk(packagesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return nil
		}
		// Only look for riverdeck.pkg.manifest.json.
		manifestPath := filepath.Join(path, "riverdeck.pkg.manifest.json")
		data, readErr := os.ReadFile(manifestPath)
		if readErr != nil {
			return nil
		}
		var mf pkgManifest
		if json.Unmarshal(data, &mf) != nil {
			return nil
		}
		rel, _ := filepath.Rel(packagesDir, path)
		repoDir := filepath.ToSlash(rel)
		ip := InstalledPackage{
			RepoDir:       repoDir,
			Name:          mf.Name,
			Version:       mf.Version,
			DaemonEnabled: cfg.IsDaemonEnabled(repoDir, ""),
		}
		result = append(result, ip)
		// Don't recurse into sub-package dirs (they have their own manifests
		// but are listed via the parent).
		return nil
	})
	return result, nil
}

// shorthand converts a PackageSource to a dot-separated import shorthand.
// e.g. "github.com/merith-tk/riverdeck-packages" → "merith-tk.riverdeck-packages"
func (m *Manager) shorthand(src PackageSource) string {
	parts := []string{src.User}
	parts = append(parts, strings.Split(src.Repo, "-")...)
	// Simple heuristic: use "user.repo" format.
	sh := src.User + "." + src.Repo
	if src.Branch != "" {
		sh += "@" + src.Branch
	} else if src.Tag != "" {
		sh += "@" + src.Tag
	}
	_ = parts
	return sh
}

// pkgManifest is a lightweight struct for reading just the name/version from
// a riverdeck.pkg.manifest.json without the full heavy type.
type pkgManifest struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Version  string `json:"version"`
	Packages []struct {
		ID   string `json:"id"`
		Path string `json:"path"`
	} `json:"packages,omitempty"`
}

// readManifest reads the riverdeck.pkg.manifest.json from dir.
func readManifest(dir string) (*pkgManifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, "riverdeck.pkg.manifest.json"))
	if err != nil {
		return nil, err
	}
	var m pkgManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}
