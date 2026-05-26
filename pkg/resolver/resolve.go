// Package resolver provides URI scheme parsing and resolution for Riverdeck
// resource references.
//
// Four URI schemes are supported:
//
//	pkg://packagename#iconname        - named icon from a package's icon registry
//	http://... / https://...          - remote web resource
//	/path/from/config/root            - leading slash = config-root-relative
//	./relative/path  or  plain/path   - relative to the caller-supplied base dir
//
// The old pkg://packagename/relative/path slash form is no longer supported.
// All package asset references must use the #iconname registry lookup.
//
// Lua scripts MAY NOT be loaded from the web (SchemeWeb).  Callers must check
// IsLuaForbidden before executing any resolved path as Lua code.
//
// Example:
//
//	ref := resolver.Parse("pkg://riverdeck#home")
//	abs, err := resolver.Resolve(ref, configDir, packages)
//	// abs == "<configDir>/.packages/riverdeck/icons/home.svg"
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
	// SchemeFile is the default: a path relative to a caller-supplied base
	// directory (or absolute filesystem path).
	// Examples: "./icons/foo.png", "audio/vol_up.lua".
	SchemeFile Scheme = iota

	// SchemePackage references a named icon registered in an installed package's
	// icon registry.
	// Format: "pkg://packagename#iconname"
	SchemePackage

	// SchemeWeb references a remote HTTP/HTTPS resource.
	// Format: "http://..." or "https://..."
	SchemeWeb

	// SchemeConfigRoot references a path relative to the Riverdeck config
	// directory.  A leading "/" in the raw string triggers this scheme.
	// Example: "/system/icons/cpu.svg" -> "<configDir>/system/icons/cpu.svg"
	SchemeConfigRoot
)

// ResourceRef is a parsed resource reference.
type ResourceRef struct {
	// Scheme identifies the type of reference.
	Scheme Scheme

	// PackageName is the package ID extracted from a pkg:// URI.
	// Empty for non-package references.
	PackageName string

	// IconName is the icon registry key extracted from a pkg://#iconname URI.
	// Empty for non-package references.
	IconName string

	// RelPath is:
	//   - for SchemeFile:       the raw path string (may be relative or absolute)
	//   - for SchemeConfigRoot: the path after the leading "/"
	//   - for SchemeWeb:        empty (use RawURL)
	//   - for SchemePackage:    empty (use IconName for registry lookup)
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
		// "pkg://packagename#iconname"
		body := strings.TrimPrefix(raw, "pkg://")
		hash := strings.IndexByte(body, '#')
		if hash < 0 {
			// "pkg://packagename" with no icon name -- unresolvable, return as-is.
			return ResourceRef{
				Scheme:      SchemePackage,
				PackageName: body,
				IconName:    "",
				RawURL:      raw,
			}
		}
		return ResourceRef{
			Scheme:      SchemePackage,
			PackageName: body[:hash],
			IconName:    body[hash+1:],
			RawURL:      raw,
		}

	case strings.HasPrefix(raw, "http://"), strings.HasPrefix(raw, "https://"):
		return ResourceRef{
			Scheme: SchemeWeb,
			RawURL: raw,
		}

	case strings.HasPrefix(raw, "/"):
		// Leading slash = config-root-relative.
		return ResourceRef{
			Scheme:  SchemeConfigRoot,
			RelPath: raw[1:], // strip the leading slash
			RawURL:  raw,
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
//
// Note: Package IDs should follow reverse-DNS notation (e.g. "com.example.mypkg")
// to avoid conflicts between packages from different authors. The built-in
// "riverdeck" package is exempt as it ships with the program itself.
type PackageInfo struct {
	// ID is the canonical package identifier, e.g. "riverdeck" or "com.example.mypkg".
	ID string
	// Dir is the absolute filesystem path to the package's root directory.
	Dir string
	// Icons maps named icon keys to their paths relative to Dir.
	// Populated from the package manifest's "icons" map.
	// Example: {"home": "icons/home.svg", "back": "icons/back.svg"}
	Icons map[string]string
}

// ErrPackageNotFound is returned when a pkg:// reference names an unknown package.
var ErrPackageNotFound = errors.New("resolver: package not found")

// ErrIconNotFound is returned when a pkg://#iconname reference names an icon
// that is not registered in the package's icon registry.
var ErrIconNotFound = errors.New("resolver: icon not found in package registry")

// ErrWebLuaForbidden is returned when a caller attempts to resolve a web URI
// as a Lua script.
var ErrWebLuaForbidden = errors.New("resolver: Lua scripts cannot be loaded from the web")

// Resolve converts a ResourceRef into an absolute filesystem path (or raw URL
// for web references where the caller chooses how to fetch).
//
//   - SchemeFile       -> filepath.Join(baseDir, ref.RelPath)  (or ref.RelPath if absolute)
//   - SchemeConfigRoot -> filepath.Join(configDir, ref.RelPath)
//   - SchemePackage    -> looks up ref.IconName in pkg.Icons, returns filepath.Join(pkg.Dir, relPath)
//   - SchemeWeb        -> ref.RawURL (caller decides whether to fetch; check IsLuaForbidden first)
//
// baseDir is the directory used to resolve SchemeFile relative paths (typically
// the script's own directory or the config directory).
// configDir is used for SchemeConfigRoot resolution.
// packages is a slice of PackageInfo; pass nil if no packages are installed.
func Resolve(ref ResourceRef, baseDir, configDir string, packages []PackageInfo) (string, error) {
	switch ref.Scheme {
	case SchemeFile:
		if ref.RelPath == "" {
			return "", fmt.Errorf("resolver: empty file path")
		}
		if filepath.IsAbs(ref.RelPath) {
			return filepath.Clean(ref.RelPath), nil
		}
		return filepath.Join(baseDir, filepath.FromSlash(ref.RelPath)), nil

	case SchemeConfigRoot:
		if ref.RelPath == "" {
			return filepath.Clean(configDir), nil
		}
		return filepath.Join(configDir, filepath.FromSlash(ref.RelPath)), nil

	case SchemePackage:
		for _, pkg := range packages {
			if pkg.ID == ref.PackageName {
				if ref.IconName == "" {
					return "", fmt.Errorf("%w: %q has no icon name (use pkg://name#iconname)", ErrIconNotFound, ref.PackageName)
				}
				relPath, ok := pkg.Icons[ref.IconName]
				if !ok {
					return "", fmt.Errorf("%w: %q not registered in package %q", ErrIconNotFound, ref.IconName, ref.PackageName)
				}
				return filepath.Join(pkg.Dir, filepath.FromSlash(relPath)), nil
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
func ResolveString(raw, baseDir, configDir string, packages []PackageInfo) (string, error) {
	return Resolve(Parse(raw), baseDir, configDir, packages)
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
// the scheme of a reference.  Returns "PKG", "WEB", "CFG", or "FILE".
func SchemeBadge(ref ResourceRef) string {
	switch ref.Scheme {
	case SchemePackage:
		return "PKG"
	case SchemeWeb:
		return "WEB"
	case SchemeConfigRoot:
		return "CFG"
	default:
		return "FILE"
	}
}
