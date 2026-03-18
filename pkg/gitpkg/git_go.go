package gitpkg

import (
	"fmt"
)

// NOTE: The go-git implementation is a stub. To enable full go-git support,
// add github.com/go-git/go-git/v5 to go.mod and implement these functions.
//
// The native backend (system git) is preferred when available, so this stub
// is sufficient for most deployments. The go-git backend exists as a
// fallback for environments without git installed (e.g. some CI containers
// or Windows deployments without Git for Windows).

func gogitClone(url, targetDir, refName string, depth int) error {
	return fmt.Errorf("go-git backend not compiled in; install git or set git_backend: native")
}

func gogitFetch(repoDir string) error {
	return fmt.Errorf("go-git backend not compiled in; install git or set git_backend: native")
}

func gogitCheckout(repoDir, refName string) error {
	return fmt.Errorf("go-git backend not compiled in; install git or set git_backend: native")
}

func gogitPull(repoDir string) error {
	return fmt.Errorf("go-git backend not compiled in; install git or set git_backend: native")
}

func gogitListTags(url string) ([]string, error) {
	return nil, fmt.Errorf("go-git backend not compiled in; install git or set git_backend: native")
}
