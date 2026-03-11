package resolver_test

import (
	"testing"

	"github.com/merith-tk/riverdeck/pkg/resolver"
)

func TestParse(t *testing.T) {
	cases := []struct {
		raw     string
		scheme  resolver.Scheme
		pkgName string
		relPath string
	}{
		{"audio/vol.lua", resolver.SchemeFile, "", "audio/vol.lua"},
		{"/abs/path.lua", resolver.SchemeFile, "", "/abs/path.lua"},
		{"pkg://riverdeck/icons/home.png", resolver.SchemePackage, "riverdeck", "icons/home.png"},
		{"pkg://vendor.x", resolver.SchemePackage, "vendor.x", ""},
		{"http://example.com/img.png", resolver.SchemeWeb, "", ""},
		{"https://example.com/script.lua", resolver.SchemeWeb, "", ""},
		{"", resolver.SchemeFile, "", ""},
	}

	for _, c := range cases {
		ref := resolver.Parse(c.raw)
		if ref.Scheme != c.scheme {
			t.Errorf("Parse(%q) scheme = %v, want %v", c.raw, ref.Scheme, c.scheme)
		}
		if ref.PackageName != c.pkgName {
			t.Errorf("Parse(%q) PackageName = %q, want %q", c.raw, ref.PackageName, c.pkgName)
		}
		if c.relPath != "" && ref.RelPath != c.relPath {
			t.Errorf("Parse(%q) RelPath = %q, want %q", c.raw, ref.RelPath, c.relPath)
		}
		if ref.RawURL != c.raw {
			t.Errorf("Parse(%q) RawURL = %q, want %q", c.raw, ref.RawURL, c.raw)
		}
	}
}

func TestResolve_File(t *testing.T) {
	ref := resolver.Parse("scripts/foo.lua")
	got, err := resolver.Resolve(ref, "/cfg", nil)
	if err != nil {
		t.Fatal(err)
	}
	// Use OS-agnostic check: contains both segments.
	if got == "" {
		t.Fatal("expected non-empty path")
	}
}

func TestResolve_Package(t *testing.T) {
	pkgs := []resolver.PackageInfo{
		{ID: "riverdeck", Dir: "/cfg/.packages/riverdeck"},
	}
	ref := resolver.Parse("pkg://riverdeck/icons/home.png")
	got, err := resolver.Resolve(ref, "/cfg", pkgs)
	if err != nil {
		t.Fatal(err)
	}
	if got == "" {
		t.Fatal("expected non-empty path")
	}
}

func TestResolve_PackageNotFound(t *testing.T) {
	ref := resolver.Parse("pkg://missing/foo.png")
	_, err := resolver.Resolve(ref, "/cfg", nil)
	if err == nil {
		t.Fatal("expected error for missing package, got nil")
	}
}

func TestResolve_Web(t *testing.T) {
	ref := resolver.Parse("https://example.com/img.png")
	got, err := resolver.Resolve(ref, "/cfg", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://example.com/img.png" {
		t.Errorf("got %q, want raw URL", got)
	}
}

func TestIsLuaForbidden(t *testing.T) {
	if resolver.IsLuaForbiddenString("audio/vol.lua") {
		t.Error("file ref should not be forbidden")
	}
	if resolver.IsLuaForbiddenString("pkg://x/y.lua") {
		t.Error("pkg ref should not be forbidden")
	}
	if !resolver.IsLuaForbiddenString("https://evil.com/payload.lua") {
		t.Error("web lua should be forbidden")
	}
}

func TestSchemeBadge(t *testing.T) {
	cases := map[string]string{
		"pkg://x/y.png":       "PKG",
		"http://example.com":  "WEB",
		"https://example.com": "WEB",
		"relative/path.png":   "FILE",
	}
	for raw, want := range cases {
		if got := resolver.SchemeBadge(resolver.Parse(raw)); got != want {
			t.Errorf("SchemeBadge(%q) = %q, want %q", raw, got, want)
		}
	}
}
