// Package resolver provides URI scheme parsing and resolution for Riverdeck
// resource references.
//
// Three URI schemes are supported:
//
//	pkg://packagename/relative/path   - asset bundled inside a package
//	http://... / https://...          - remote web resource
//	anything else                     - path relative to the config directory
//
// Lua scripts MAY NOT be loaded from the web (SchemeWeb).  Callers must check
// IsLuaForbidden before executing any resolved path as Lua code.
//
// Example:
//
//	ref := resolver.Parse("pkg://riverdeck/icons/home.png")
//	abs, err := resolver.Resolve(ref, configDir, packages)
//	// abs == "<configDir>/.packages/riverdeck/icons/home.png"
//
//	ref2 := resolver.Parse("https://example.com/script.lua")
//	resolver.IsLuaForbidden(ref2) // true
package resolver

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// Scheme identifies what kind of resource reference has been parsed.
type Scheme int

const (
	// SchemeFile is the default: a path relative to the config directory (or
	// absolute).  Examples: "audio/vol_up.lua", "/home/user/.config/foo.lua".
	SchemeFile Scheme = iota

	// SchemePackage references an asset bundled inside an installed package.
	// Format: "pkg://packagename/relative/path"
	SchemePackage

	// SchemeWeb references a remote HTTP/HTTPS resource.
	// Format: "http://..." or "https://..."
	SchemeWeb
)

// ResourceRef is a parsed resource reference.
type ResourceRef struct {
	// Scheme identifies the type of reference.
	Scheme Scheme

	// PackageName is the package ID extracted from a pkg:// URI.
	// Empty for non-package references.
	PackageName string

	// RelPath is:
	//   - for SchemePackage: the path after "pkg://packagename/"
	//   - for SchemeFile:    the raw path string (may be relative or absolute)
	//   - for SchemeWeb:     empty (use RawURL)
	RelPath string

	// RawURL is the original URI string, always set.
	RawURL string
}

// Parse converts a raw string into a ResourceRef without touching the
// filesystem.  It never returns an error; ambiguous or empty strings default
// to SchemeFile.
func Parse(raw string) ResourceRef {
	switch {
	case strings.HasPrefix(raw, "pkg://"):
		// "pkg://packagename/rest/of/path"
		body := strings.TrimPrefix(raw, "pkg://")
		slash := strings.IndexByte(body, '/')
		if slash < 0 {
			// "pkg://packagename" with no trailing path -- treat the package root.
			return ResourceRef{
				Scheme:      SchemePackage,
				PackageName: body,
				RelPath:     "",
				RawURL:      raw,
			}
		}
		return ResourceRef{
			Scheme:      SchemePackage,
			PackageName: body[:slash],
			RelPath:     body[slash+1:],
			RawURL:      raw,
		}

	case strings.HasPrefix(raw, "http://"), strings.HasPrefix(raw, "https://"):
		return ResourceRef{
			Scheme: SchemeWeb,
			RawURL: raw,
		}

	default:
		return ResourceRef{
			Scheme:  SchemeFile,
			RelPath: raw,
			RawURL:  raw,
		}
	}
}

// PackageInfo is a minimal package descriptor required by Resolve.
// Callers convert their package lists to []PackageInfo before calling Resolve.
type PackageInfo struct {
	// ID is the canonical package identifier, e.g. "riverdeck" or "vendor.pkgname".
	ID string
	// Dir is the absolute filesystem path to the package's root directory.
	Dir string
}

// ErrPackageNotFound is returned when a pkg:// reference names an unknown package.
var ErrPackageNotFound = errors.New("resolver: package not found")

// ErrWebLuaForbidden is returned when a caller attempts to resolve a web URI
// as a Lua script.
var ErrWebLuaForbidden = errors.New("resolver: Lua scripts cannot be loaded from the web")

// Resolve converts a ResourceRef into an absolute filesystem path (or raw URL
// for web references where the caller chooses how to fetch).
//
//   - SchemeFile  -> filepath.Join(configDir, ref.RelPath)  (or ref.RelPath if absolute)
//   - SchemePackage -> <package.Dir>/<ref.RelPath>
//   - SchemeWeb   -> ref.RawURL (caller decides whether to fetch; check IsLuaForbidden first)
//
// packages is a slice of PackageInfo; pass nil if no packages are installed.
func Resolve(ref ResourceRef, configDir string, packages []PackageInfo) (string, error) {
	switch ref.Scheme {
	case SchemeFile:
		if ref.RelPath == "" {
			return "", fmt.Errorf("resolver: empty file path")
		}
		if filepath.IsAbs(ref.RelPath) {
			return filepath.Clean(ref.RelPath), nil
		}
		return filepath.Join(configDir, filepath.FromSlash(ref.RelPath)), nil

	case SchemePackage:
		for _, pkg := range packages {
			if pkg.ID == ref.PackageName {
				return filepath.Join(pkg.Dir, filepath.FromSlash(ref.RelPath)), nil
			}
		}
		return "", fmt.Errorf("%w: %q", ErrPackageNotFound, ref.PackageName)

	case SchemeWeb:
		return ref.RawURL, nil

	default:
		return "", fmt.Errorf("resolver: unknown scheme %d", ref.Scheme)
	}
}

// ResolveString is a convenience wrapper that parses raw then calls Resolve.
func ResolveString(raw, configDir string, packages []PackageInfo) (string, error) {
	return Resolve(Parse(raw), configDir, packages)
}

// IsLuaForbidden reports whether a ResourceRef must not be executed as Lua
// code.  Currently this returns true only for web (http/https) references.
func IsLuaForbidden(ref ResourceRef) bool {
	return ref.Scheme == SchemeWeb
}

// IsLuaForbiddenString is a convenience wrapper.
func IsLuaForbiddenString(raw string) bool {
	return IsLuaForbidden(Parse(raw))
}

// SchemeBadge returns a short label suitable for display in a UI to indicate
// the scheme of a reference.  Returns "PKG", "WEB", or "FILE".
func SchemeBadge(ref ResourceRef) string {
	switch ref.Scheme {
	case SchemePackage:
		return "PKG"
	case SchemeWeb:
		return "WEB"
	default:
		return "FILE"
	}
}
