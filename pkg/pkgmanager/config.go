package pkgmanager

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// pkgConfigFile is the name of the package configuration file.
const pkgConfigFile = "packages.cfg.json"

// PackagesCfg maps repo source strings to their per-installation settings.
type PackagesCfg map[string]RepoCfg

// RepoCfg holds per-repository configuration.
type RepoCfg struct {
	// DaemonEnabled controls whether the repo-level daemon is started.
	// For multi-package repos, daemon settings live in Packages.
	DaemonEnabled *bool `json:"daemon_enabled,omitempty"`

	// UpdateChannel is "release" (track tags) or "branch:<name>" (dev mode).
	UpdateChannel string `json:"update_channel,omitempty"`

	// PinnedTag pins a specific version tag, overriding UpdateChannel when
	// UpdateChannel is "release".
	PinnedTag string `json:"pinned_tag,omitempty"`

	// Packages holds per-sub-package settings for multi-package repos.
	Packages map[string]SubPkgCfg `json:"packages,omitempty"`
}

// SubPkgCfg holds per-sub-package settings inside a multi-package repo.
type SubPkgCfg struct {
	DaemonEnabled *bool `json:"daemon_enabled,omitempty"`
}

// LoadPackagesCfg reads packages.cfg.json from the packages directory.
// Returns an empty config if the file does not exist.
func LoadPackagesCfg(packagesDir string) (PackagesCfg, error) {
	data, err := os.ReadFile(filepath.Join(packagesDir, pkgConfigFile))
	if os.IsNotExist(err) {
		return make(PackagesCfg), nil
	}
	if err != nil {
		return nil, err
	}
	var cfg PackagesCfg
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg == nil {
		cfg = make(PackagesCfg)
	}
	return cfg, nil
}

// SavePackagesCfg writes cfg to the packages directory.
func SavePackagesCfg(packagesDir string, cfg PackagesCfg) error {
	if err := os.MkdirAll(packagesDir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(packagesDir, pkgConfigFile), data, 0644)
}

// IsDaemonEnabled returns whether the daemon is enabled for a given repo and
// optional sub-package. sub may be empty for single-package repos.
//
// Defaults to false when no explicit setting exists.
func (cfg PackagesCfg) IsDaemonEnabled(repoKey, sub string) bool {
	repo, ok := cfg[repoKey]
	if !ok {
		return false
	}
	if sub != "" {
		if spCfg, ok := repo.Packages[sub]; ok && spCfg.DaemonEnabled != nil {
			return *spCfg.DaemonEnabled
		}
		return false
	}
	if repo.DaemonEnabled != nil {
		return *repo.DaemonEnabled
	}
	return false
}

// SetDaemonEnabled sets the daemon-enabled flag for a repo (and optional sub-package).
func (cfg PackagesCfg) SetDaemonEnabled(repoKey, sub string, enabled bool) {
	repo := cfg[repoKey]
	if sub != "" {
		if repo.Packages == nil {
			repo.Packages = make(map[string]SubPkgCfg)
		}
		sp := repo.Packages[sub]
		sp.DaemonEnabled = &enabled
		repo.Packages[sub] = sp
	} else {
		repo.DaemonEnabled = &enabled
	}
	cfg[repoKey] = repo
}
