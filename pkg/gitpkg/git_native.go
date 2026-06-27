package gitpkg

import (
	"fmt"
	"os/exec"
	"strings"
)

// nativeClone clones url into targetDir using the system git binary.
func nativeClone(url, targetDir, refName string, depth int) error {
	args := []string{"clone"}
	if depth > 0 {
		args = append(args, "--depth", fmt.Sprintf("%d", depth))
	}
	if refName != "" {
		args = append(args, "--branch", refName)
	}
	args = append(args, "--", url, targetDir)
	return runGit(args...)
}

// nativeFetch fetches latest refs from origin in repoDir.
func nativeFetch(repoDir string) error {
	return runGitInDir(repoDir, "fetch", "--tags", "origin")
}

// nativeCheckout checks out refName in repoDir.
func nativeCheckout(repoDir, refName string) error {
	return runGitInDir(repoDir, "checkout", refName)
}

// nativePull fetches and merges in repoDir.
func nativePull(repoDir string) error {
	return runGitInDir(repoDir, "pull", "--ff-only")
}

// nativeListTags returns all tags for the remote at url.
func nativeListTags(url string) ([]string, error) {
	out, err := exec.Command("git", "ls-remote", "--tags", "--refs", url).Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-remote: %w", err)
	}
	var tags []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		// Format: "<sha>\trefs/tags/<name>"
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		ref := strings.TrimPrefix(parts[1], "refs/tags/")
		tags = append(tags, ref)
	}
	return tags, nil
}

// runGit runs a git command with no working directory.
func runGit(args ...string) error {
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, out)
	}
	return nil
}

// runGitInDir runs a git command with a specific working directory.
func runGitInDir(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s (in %s): %w\n%s", strings.Join(args, " "), dir, err, out)
	}
	return nil
}
