package pkgmanager

import (
	"strings"
)

// PackageSource holds a parsed package install URL.
//
// Supported formats:
//
//	github.com/user/repo                    single-pkg, main branch
//	github.com/user/repo@branch             single-pkg, specific branch
//	github.com/user/repo@v1.2.0             single-pkg, tag (version pin)
//	github.com/user/repo/ytmd               specific package from multi-pkg repo
//	github.com/user/repo@v2.0.0/ytmd        specific package + version pin
//	gitlab.com/user/repo                    GitLab (same format)
//	git.hostname.tld/user/repo              custom git host
type PackageSource struct {
	// Host is the git host, e.g. "github.com", "gitlab.com".
	Host string

	// User is the repository owner.
	User string

	// Repo is the repository name.
	Repo string

	// Branch is the branch to clone. Empty means default branch (usually "main").
	Branch string

	// Tag is a version tag to pin, e.g. "v1.2.0". Mutually exclusive with Branch.
	Tag string

	// PkgPath is the sub-directory for a specific package inside a multi-pkg
	// repo. Empty means the whole repo is the package.
	PkgPath string

	// RepoDir is the directory name to use under .config/packages/.
	// For versioned installs this includes the ref, e.g. "github.com/user/repo@dev".
	RepoDir string
}

// ParseSource parses a package install URL string into a PackageSource.
//
// Returns an error for malformed or unsupported URLs.
func ParseSource(raw string) (PackageSource, error) {
	// Strip any leading scheme (https://, git://, etc.)
	raw = strings.TrimPrefix(raw, "https://")
	raw = strings.TrimPrefix(raw, "http://")
	raw = strings.TrimPrefix(raw, "git://")

	var src PackageSource

	// Split off the sub-package path: everything after the third slash segment.
	// e.g. "github.com/user/repo/ytmd" → base="github.com/user/repo", pkg="ytmd"
	// But we need to be careful about the @ref component in the repo part.
	//
	// Strategy: find host/user/repo first, then the rest is pkg path.
	parts := strings.SplitN(raw, "/", 4)
	if len(parts) < 3 {
		return src, &ParseError{Input: raw, Reason: "expected host/user/repo"}
	}

	src.Host = parts[0]
	src.User = parts[1]
	repoAndRef := parts[2]
	if len(parts) == 4 {
		src.PkgPath = strings.TrimSuffix(parts[3], "/")
	}

	// Split repo name from @ref.
	if atIdx := strings.Index(repoAndRef, "@"); atIdx >= 0 {
		src.Repo = repoAndRef[:atIdx]
		ref := repoAndRef[atIdx+1:]
		// Distinguish tag from branch: tags conventionally start with "v" and
		// contain dots, but we treat any ref as a tag if it matches "v\d+…"
		// and as a branch otherwise.
		if isTag(ref) {
			src.Tag = ref
		} else {
			src.Branch = ref
		}
	} else {
		src.Repo = repoAndRef
	}

	if src.Host == "" || src.User == "" || src.Repo == "" {
		return src, &ParseError{Input: raw, Reason: "missing host, user, or repo"}
	}

	// Build the repo directory name: host/user/repo[@ref]
	repoDir := src.Host + "/" + src.User + "/" + src.Repo
	if src.Tag != "" {
		repoDir += "@" + src.Tag
	} else if src.Branch != "" {
		repoDir += "@" + src.Branch
	}
	src.RepoDir = repoDir

	return src, nil
}

// CloneURL returns the HTTPS clone URL for the source.
func (s PackageSource) CloneURL() string {
	return "https://" + s.Host + "/" + s.User + "/" + s.Repo
}

// Ref returns the git ref to check out, preferring Tag over Branch.
// Returns empty string when neither is set (use default branch).
func (s PackageSource) Ref() string {
	if s.Tag != "" {
		return s.Tag
	}
	return s.Branch
}

// isTag returns true when ref looks like a version tag (starts with "v" and
// contains at least one dot, or is an exact SHA).
func isTag(ref string) bool {
	if len(ref) == 0 {
		return false
	}
	if strings.HasPrefix(ref, "v") && strings.Contains(ref, ".") {
		return true
	}
	// 40-char hex SHA
	if len(ref) == 40 {
		for _, c := range ref {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
		return true
	}
	return false
}

// ParseError is returned when a source URL cannot be parsed.
type ParseError struct {
	Input  string
	Reason string
}

func (e *ParseError) Error() string {
	return "invalid package source " + e.Input + ": " + e.Reason
}
