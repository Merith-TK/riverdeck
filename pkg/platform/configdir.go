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
