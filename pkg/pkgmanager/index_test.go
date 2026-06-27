package pkgmanager

import (
	"path/filepath"
	"testing"
)

func TestLoadPackages_NotExist(t *testing.T) {
	dir := t.TempDir()
	pf, err := LoadPackages(dir)
	if err != nil {
		t.Fatalf("LoadPackages on empty dir: %v", err)
	}
	if pf == nil {
		t.Fatal("LoadPackages returned nil map")
	}
	if len(pf) != 0 {
		t.Errorf("LoadPackages returned %d entries, want 0", len(pf))
	}
}

func TestSaveAndLoadPackages(t *testing.T) {
	dir := t.TempDir()
	disabled := false
	pf := PackagesFile{
		"merith-tk.riverdeck-packages": PackageEntry{
			Path:          "github.com/merith-tk/riverdeck-packages",
			Packages:      []string{"ytmd", "obs"},
			DaemonEnabled: &disabled,
		},
	}
	if err := SavePackages(dir, pf); err != nil {
		t.Fatalf("SavePackages: %v", err)
	}

	loaded, err := LoadPackages(dir)
	if err != nil {
		t.Fatalf("LoadPackages: %v", err)
	}
	if len(loaded) != 1 {
		t.Errorf("LoadPackages returned %d entries, want 1", len(loaded))
	}
	entry, ok := loaded["merith-tk.riverdeck-packages"]
	if !ok {
		t.Fatal("missing key 'merith-tk.riverdeck-packages'")
	}
	if entry.Path != "github.com/merith-tk/riverdeck-packages" {
		t.Errorf("Path = %q, want %q", entry.Path, "github.com/merith-tk/riverdeck-packages")
	}
	if len(entry.Packages) != 2 {
		t.Errorf("Packages = %v, want [ytmd obs]", entry.Packages)
	}
}

func TestIsDaemonEnabled(t *testing.T) {
	enabled := true
	disabled := false

	tests := []struct {
		name    string
		pf      PackagesFile
		key     string
		sub     string
		want    bool
	}{
		{
			name: "no entry",
			pf:   PackagesFile{},
			key:  "nonexistent",
			sub:  "",
			want: false,
		},
		{
			name: "entry disabled",
			pf: PackagesFile{
				"pkg": {DaemonEnabled: &disabled},
			},
			key:  "pkg",
			sub:  "",
			want: false,
		},
		{
			name: "entry enabled",
			pf: PackagesFile{
				"pkg": {DaemonEnabled: &enabled},
			},
			key:  "pkg",
			sub:  "",
			want: true,
		},
		{
			name: "sub-package overrides parent",
			pf: PackagesFile{
				"multi": {
					DaemonEnabled: &disabled,
					SubPackages: map[string]SubEntry{
						"sub1": {DaemonEnabled: &enabled},
					},
				},
			},
			key:  "multi",
			sub:  "sub1",
			want: true,
		},
		{
			name: "sub-package with no setting falls back to false",
			pf: PackagesFile{
				"multi": {
					DaemonEnabled: &enabled,
					SubPackages: map[string]SubEntry{
						"sub1": {},
					},
				},
			},
			key:  "multi",
			sub:  "sub1",
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.pf.IsDaemonEnabled(tc.key, tc.sub)
			if got != tc.want {
				t.Errorf("IsDaemonEnabled(%q, %q) = %v, want %v", tc.key, tc.sub, got, tc.want)
			}
		})
	}
}

func TestSetDaemonEnabled(t *testing.T) {
	pf := make(PackagesFile)

	pf.SetDaemonEnabled("pkg", "", true)
	if !*pf["pkg"].DaemonEnabled {
		t.Error("SetDaemonEnabled(pkg, true) failed")
	}

	pf.SetDaemonEnabled("multi", "sub1", true)
	if !*pf["multi"].SubPackages["sub1"].DaemonEnabled {
		t.Error("SetDaemonEnabled(multi, sub1, true) failed")
	}

	pf.SetDaemonEnabled("multi", "sub1", false)
	if *pf["multi"].SubPackages["sub1"].DaemonEnabled {
		t.Error("SetDaemonEnabled(multi, sub1, false) failed")
	}
}

func TestResolve(t *testing.T) {
	pkgsDir := t.TempDir()
	pf := PackagesFile{
		"merith-tk.riverdeck-packages": PackageEntry{
			Path:     "github.com/merith-tk/riverdeck-packages",
			Packages: []string{"ytmd", "obs"},
		},
		"user.simple-clock": PackageEntry{
			Path: "github.com/user/simple-clock",
		},
	}

	tests := []struct {
		name       string
		module     string
		wantPath   string
		wantFound  bool
	}{
		{
			name:       "sub-package with library",
			module:     "merith-tk.riverdeck-packages.ytmd.lib.api",
			wantPath:   filepath.Join(pkgsDir, "github.com/merith-tk/riverdeck-packages", "ytmd", "lib", "api.lua"),
			wantFound:  true,
		},
		{
			name:       "single package library",
			module:     "user.simple-clock.lib.util",
			wantPath:   filepath.Join(pkgsDir, "github.com/user/simple-clock", "lib", "util.lua"),
			wantFound:  true,
		},
		{
			name:       "unknown module",
			module:     "unknown.pkg.thing",
			wantFound:  false,
		},
		{
			name:       "sub-package with no path remainder",
			module:     "merith-tk.riverdeck-packages.ytmd",
			wantFound:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path, found := pf.Resolve(tc.module, pkgsDir)
			if found != tc.wantFound {
				t.Errorf("Resolve(%q) found=%v, want %v", tc.module, found, tc.wantFound)
			}
			if found && path != tc.wantPath {
				t.Errorf("Resolve(%q) path=%q, want %q", tc.module, path, tc.wantPath)
			}
		})
	}
}

func TestResolve_InvalidSubPackage(t *testing.T) {
	pkgsDir := t.TempDir()
	pf := PackagesFile{
		"multi": PackageEntry{
			Path:     "repo",
			Packages: []string{"validpkg"},
		},
	}

	// "invalidpkg" is not in the Packages list, so resolution should fall
	// through to looking for a file under the repo root.
	path, found := pf.Resolve("multi.invalidpkg.test", pkgsDir)
	if !found {
		t.Fatal("Resolve should still work for unknown sub-packages, using repo root")
	}
	want := filepath.Join(pkgsDir, "repo", "invalidpkg", "test.lua")
	if path != want {
		t.Errorf("Resolve path = %q, want %q", path, want)
	}
}
