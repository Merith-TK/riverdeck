// Package gitpkg provides a hybrid Git client that prefers the system git
// binary when available and falls back to a pure-Go implementation.
//
// The active backend is selected once at Init() and used for all subsequent
// operations. The config option "auto" (default) picks the native binary when
// found on PATH, otherwise falls back to go-git.
package gitpkg

import (
	"fmt"
	"os/exec"
)

// Backend selects the Git implementation to use.
type Backend int

const (
	// BackendAuto detects the best available backend at Init() time.
	// Prefers system git when found on PATH, otherwise uses go-git.
	BackendAuto Backend = iota

	// BackendNative forces use of the system git binary.
	BackendNative

	// BackendGoGit forces use of the pure-Go go-git library.
	BackendGoGit
)

// ActiveBackend is the backend selected by Init().
var ActiveBackend Backend

// Init selects the git backend based on a config string.
//
// configuredBackend should be one of: "auto", "native", "go-git".
// Any unrecognised value is treated as "auto".
func Init(configuredBackend string) {
	switch configuredBackend {
	case "native":
		ActiveBackend = BackendNative
		fmt.Println("[gitpkg] backend: native (forced)")
	case "go-git":
		ActiveBackend = BackendGoGit
		fmt.Println("[gitpkg] backend: go-git (forced)")
	default:
		// Auto-detect: prefer system git.
		if _, err := exec.LookPath("git"); err == nil {
			ActiveBackend = BackendNative
			fmt.Println("[gitpkg] backend: native (auto-detected)")
		} else {
			ActiveBackend = BackendGoGit
			fmt.Println("[gitpkg] backend: go-git (fallback, git not found on PATH)")
		}
	}
}

// Clone clones url into targetDir, checking out refName (branch or tag).
// depth=0 means a full clone; depth=1 is a shallow clone (faster for tags).
func Clone(url, targetDir, refName string, depth int) error {
	switch ActiveBackend {
	case BackendNative:
		return nativeClone(url, targetDir, refName, depth)
	default:
		return gogitClone(url, targetDir, refName, depth)
	}
}

// Fetch fetches the latest refs from origin inside repoDir.
func Fetch(repoDir string) error {
	switch ActiveBackend {
	case BackendNative:
		return nativeFetch(repoDir)
	default:
		return gogitFetch(repoDir)
	}
}

// Checkout switches repoDir to refName (branch or tag).
func Checkout(repoDir, refName string) error {
	switch ActiveBackend {
	case BackendNative:
		return nativeCheckout(repoDir, refName)
	default:
		return gogitCheckout(repoDir, refName)
	}
}

// Pull fetches and merges origin into the current branch in repoDir.
func Pull(repoDir string) error {
	switch ActiveBackend {
	case BackendNative:
		return nativePull(repoDir)
	default:
		return gogitPull(repoDir)
	}
}

// ListTags returns all tags for the remote repository at url.
// Requires network access; uses git ls-remote or equivalent.
func ListTags(url string) ([]string, error) {
	switch ActiveBackend {
	case BackendNative:
		return nativeListTags(url)
	default:
		return gogitListTags(url)
	}
}
