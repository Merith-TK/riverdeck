package platform

import (
	"os"
	"path/filepath"
	"runtime"
)

// ConfigDir returns the canonical Riverdeck configuration directory.
//
// If override is non-empty it is resolved to an absolute path and returned.
// Otherwise the platform default is used:
//
//	Windows : %APPDATA%\.riverdeck
//	Other   : $HOME/.config/riverdeck
//
// Falls back to ".riverdeck" relative to the working directory when the
// platform defaults cannot be determined.
func ConfigDir(override string) string {
	if override != "" {
		abs, err := filepath.Abs(override)
		if err != nil {
			return override
		}
		return abs
	}
	if runtime.GOOS == "windows" {
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, ".riverdeck")
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".config", "riverdeck")
	}
	return ".riverdeck"
}

// PackagesDir returns the path to the installed packages directory.
//
//	<configDir>/.config/packages/
func PackagesDir(configDir string) string {
	return filepath.Join(configDir, ".config", "packages")
}

// DevicesDir returns the path to the per-device configuration directory.
//
//	<configDir>/.config/devices/
func DevicesDir(configDir string) string {
	return filepath.Join(configDir, ".config", "devices")
}

// DeviceSessionDir returns the effective config root for a device session
// based on the configured multi-device mode.
//
//	mode "shared"     -> rootDir                     (all devices share one config tree)
//	mode "layout"     -> rootDir                     (layout.json devices map routes by UUID)
//	mode "individual" -> rootDir / .device / <id>    (each device gets its own config subtree)
//
// The returned path is not created; callers should os.MkdirAll when needed.
func DeviceSessionDir(rootDir, deviceID, mode string) string {
	switch mode {
	case "individual":
		return filepath.Join(rootDir, ".device", deviceID)
	default:
		return rootDir
	}
}

// ConfigFile returns the path to the main application config file.
//
//	<configDir>/.config.yml
func ConfigFile(configDir string) string {
	return filepath.Join(configDir, ".config.yml")
}
