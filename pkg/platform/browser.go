// Package platform provides cross-platform utility functions shared across
// multiple Riverdeck binaries.  These include opening URLs and directories in
// the user's desktop environment.
package platform

import (
	"log"
	"os/exec"
	"runtime"
)

// OpenBrowser launches the system default browser at the given URL.
// On failure it logs a warning but does not return an error because this is
// always a best-effort operation.
func OpenBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default: // linux, freebsd, etc.
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("[!] Could not open browser: %v (visit %s manually)", err, url)
	}
}

// OpenFolder opens the given directory in the system's default file manager
// (Explorer on Windows, Finder on macOS, xdg-open on Linux).
func OpenFolder(dir string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", dir)
	case "darwin":
		cmd = exec.Command("open", dir)
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	return cmd.Start()
}
