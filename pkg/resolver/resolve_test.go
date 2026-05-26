package resolver_test

import (
	"testing"

	"github.com/merith-tk/riverdeck/pkg/resolver"
)

func TestParse_File(t *testing.T) {
	cases := []struct {
		raw     string
		scheme  resolver.Scheme
		relPath string
	}{
		{"audio/vol.lua", resolver.SchemeFile, "audio/vol.lua"},
		{"./icons/cpu.png", resolver.SchemeFile, "./icons/cpu.png"},
		{"", resolver.SchemeFile, ""},
	}
	for _, c := range cases {
		ref := resolver.Parse(c.raw)
		if ref.Scheme != c.scheme {
			t.Errorf("Parse(%q) scheme = %v, want %v", c.raw, ref.Scheme, c.scheme)
		}
		if ref.RelPath != c.relPath {
			t.Errorf("Parse(%q) RelPath = %q, want %q", c.raw, ref.RelPath, c.relPath)
		}
		if ref.RawURL != c.raw {
			t.Errorf("Parse(%q) RawURL = %q, want %q", c.raw, ref.RawURL, c.raw)
		}
	}
}

func TestParse_ConfigRoot(t *testing.T) {
	ref := resolver.Parse("/system/icons/cpu.svg")
	if ref.Scheme != resolver.SchemeConfigRoot {
		t.Errorf("expected SchemeConfigRoot, got %v", ref.Scheme)
	}
	if ref.RelPath != "system/icons/cpu.svg" {
		t.Errorf("RelPath = %q, want %q", ref.RelPath, "system/icons/cpu.svg")
	}
}

func TestParse_Package(t *testing.T) {
	ref := resolver.Parse("pkg://riverdeck#home")
	if ref.Scheme != resolver.SchemePackage {
		t.Errorf("expected SchemePackage, got %v", ref.Scheme)
	}
	if ref.PackageName != "riverdeck" {
		t.Errorf("PackageName = %q, want %q", ref.PackageName, "riverdeck")
	}
	if ref.IconName != "home" {
		t.Errorf("IconName = %q, want %q", ref.IconName, "home")
	}
}

func TestParse_PackageNoIcon(t *testing.T) {
	ref := resolver.Parse("pkg://vendor.x")
	if ref.Scheme != resolver.SchemePackage {
		t.Errorf("expected SchemePackage, got %v", ref.Scheme)
	}
	if ref.PackageName != "vendor.x" {
		t.Errorf("PackageName = %q, want %q", ref.PackageName, "vendor.x")
	}
	if ref.IconName != "" {
		t.Errorf("IconName should be empty, got %q", ref.IconName)
	}
}

func TestParse_Web(t *testing.T) {
	for _, raw := range []string{"http://example.com/img.png", "https://example.com/script.lua"} {
		ref := resolver.Parse(raw)
		if ref.Scheme != resolver.SchemeWeb {
			t.Errorf("Parse(%q) scheme = %v, want SchemeWeb", raw, ref.Scheme)
		}
	}
}

func TestResolve_File(t *testing.T) {
	ref := resolver.Parse("scripts/foo.lua")
	got, err := resolver.Resolve(ref, "/base", "/cfg", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got == "" {
		t.Fatal("expected non-empty path")
	}
}

func TestResolve_FileRelative(t *testing.T) {
	ref := resolver.Parse("./icons/cpu.png")
	got, err := resolver.Resolve(ref, "/scripts/mydir", "/cfg", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got == "" {
		t.Fatal("expected non-empty path")
	}
}

func TestResolve_ConfigRoot(t *testing.T) {
	ref := resolver.Parse("/system/icons/cpu.svg")
	got, err := resolver.Resolve(ref, "/base", "/cfg", nil)
	if err != nil {
		t.Fatal(err)
	}
	// Should resolve against configDir, not baseDir.
	want := "/cfg/system/icons/cpu.svg"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolve_Package(t *testing.T) {
	pkgs := []resolver.PackageInfo{
		{
			ID:  "riverdeck",
			Dir: "/cfg/.packages/riverdeck",
			Icons: map[string]string{
				"home": "icons/home.svg",
				"back": "icons/back.svg",
			},
		},
	}
	ref := resolver.Parse("pkg://riverdeck#home")
	got, err := resolver.Resolve(ref, "/base", "/cfg", pkgs)
	if err != nil {
		t.Fatal(err)
	}
	want := "/cfg/.packages/riverdeck/icons/home.svg"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolve_PackageIconNotFound(t *testing.T) {
	pkgs := []resolver.PackageInfo{
		{ID: "riverdeck", Dir: "/cfg/.packages/riverdeck", Icons: map[string]string{"home": "icons/home.svg"}},
	}
	ref := resolver.Parse("pkg://riverdeck#missing")
	_, err := resolver.Resolve(ref, "/base", "/cfg", pkgs)
	if err == nil {
		t.Fatal("expected error for missing icon name, got nil")
	}
}

func TestResolve_PackageNotFound(t *testing.T) {
	ref := resolver.Parse("pkg://missing#home")
	_, err := resolver.Resolve(ref, "/base", "/cfg", nil)
	if err == nil {
		t.Fatal("expected error for missing package, got nil")
	}
}

func TestResolve_Web(t *testing.T) {
	ref := resolver.Parse("https://example.com/img.png")
	got, err := resolver.Resolve(ref, "/base", "/cfg", nil)
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
	if resolver.IsLuaForbiddenString("pkg://riverdeck#home") {
		t.Error("pkg ref should not be forbidden")
	}
	if !resolver.IsLuaForbiddenString("https://evil.com/payload.lua") {
		t.Error("web lua should be forbidden")
	}
}

func TestSchemeBadge(t *testing.T) {
	cases := map[string]string{
		"pkg://riverdeck#home": "PKG",
		"http://example.com":   "WEB",
		"https://example.com":  "WEB",
		"/config/path.png":     "CFG",
		"relative/path.png":    "FILE",
	}
	for raw, want := range cases {
		if got := resolver.SchemeBadge(resolver.Parse(raw)); got != want {
			t.Errorf("SchemeBadge(%q) = %q, want %q", raw, got, want)
		}
	}
}
