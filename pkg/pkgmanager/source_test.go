package pkgmanager

import (
	"testing"
)

func TestParseSource_Basic(t *testing.T) {
	src, err := ParseSource("github.com/user/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if src.Host != "github.com" {
		t.Errorf("Host = %q, want %q", src.Host, "github.com")
	}
	if src.User != "user" {
		t.Errorf("User = %q, want %q", src.User, "user")
	}
	if src.Repo != "repo" {
		t.Errorf("Repo = %q, want %q", src.Repo, "repo")
	}
	if src.RepoDir != "github.com/user/repo" {
		t.Errorf("RepoDir = %q, want %q", src.RepoDir, "github.com/user/repo")
	}
	if src.Branch != "" {
		t.Errorf("Branch = %q, want empty", src.Branch)
	}
	if src.Tag != "" {
		t.Errorf("Tag = %q, want empty", src.Tag)
	}
	if src.PkgPath != "" {
		t.Errorf("PkgPath = %q, want empty", src.PkgPath)
	}
}

func TestParseSource_WithTag(t *testing.T) {
	src, err := ParseSource("github.com/user/repo@v1.2.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if src.Repo != "repo" {
		t.Errorf("Repo = %q, want %q", src.Repo, "repo")
	}
	if src.Tag != "v1.2.0" {
		t.Errorf("Tag = %q, want %q", src.Tag, "v1.2.0")
	}
	if src.Branch != "" {
		t.Errorf("Branch = %q, want empty", src.Branch)
	}
	if src.RepoDir != "github.com/user/repo@v1.2.0" {
		t.Errorf("RepoDir = %q, want %q", src.RepoDir, "github.com/user/repo@v1.2.0")
	}
}

func TestParseSource_WithBranch(t *testing.T) {
	src, err := ParseSource("github.com/user/repo@dev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if src.Tag != "" {
		t.Errorf("Tag = %q, want empty", src.Tag)
	}
	if src.Branch != "dev" {
		t.Errorf("Branch = %q, want %q", src.Branch, "dev")
	}
	if src.RepoDir != "github.com/user/repo@dev" {
		t.Errorf("RepoDir = %q, want %q", src.RepoDir, "github.com/user/repo@dev")
	}
}

func TestParseSource_WithSubPackage(t *testing.T) {
	src, err := ParseSource("github.com/user/repo/ytmd")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if src.Repo != "repo" {
		t.Errorf("Repo = %q, want %q", src.Repo, "repo")
	}
	if src.PkgPath != "ytmd" {
		t.Errorf("PkgPath = %q, want %q", src.PkgPath, "ytmd")
	}
}

func TestParseSource_TagAndSubPackage(t *testing.T) {
	src, err := ParseSource("github.com/user/repo@v2.0.0/ytmd")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if src.Repo != "repo" {
		t.Errorf("Repo = %q, want %q", src.Repo, "repo")
	}
	if src.Tag != "v2.0.0" {
		t.Errorf("Tag = %q, want %q", src.Tag, "v2.0.0")
	}
	if src.PkgPath != "ytmd" {
		t.Errorf("PkgPath = %q, want %q", src.PkgPath, "ytmd")
	}
}

func TestParseSource_GitLab(t *testing.T) {
	src, err := ParseSource("gitlab.com/group/project")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if src.Host != "gitlab.com" {
		t.Errorf("Host = %q, want %q", src.Host, "gitlab.com")
	}
	if src.User != "group" {
		t.Errorf("User = %q, want %q", src.User, "group")
	}
}

func TestParseSource_CustomHost(t *testing.T) {
	src, err := ParseSource("git.hostname.tld/user/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if src.Host != "git.hostname.tld" {
		t.Errorf("Host = %q, want %q", src.Host, "git.hostname.tld")
	}
}

func TestParseSource_StripSchemes(t *testing.T) {
	tests := []string{
		"https://github.com/user/repo",
		"http://github.com/user/repo",
		"git://github.com/user/repo",
	}
	for _, raw := range tests {
		src, err := ParseSource(raw)
		if err != nil {
			t.Fatalf("ParseSource(%q) unexpected error: %v", raw, err)
		}
		if src.Host != "github.com" {
			t.Errorf("ParseSource(%q) Host = %q, want %q", raw, src.Host, "github.com")
		}
	}
}

func TestParseSource_Errors(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "expected host/user/repo"},
		{"github.com", "expected host/user/repo"},
		{"github.com/user", "expected host/user/repo"},
	}
	for _, tc := range tests {
		_, err := ParseSource(tc.input)
		if err == nil {
			t.Errorf("ParseSource(%q) expected error containing %q, got nil", tc.input, tc.want)
			continue
		}
		pe, ok := err.(*ParseError)
		if !ok {
			t.Errorf("ParseSource(%q) error type = %T, want *ParseError", tc.input, err)
		}
		if pe != nil && pe.Input != tc.input {
			t.Errorf("ParseError.Input = %q, want %q", pe.Input, tc.input)
		}
	}
}

func TestParseSource_TrailingSlash(t *testing.T) {
	src, err := ParseSource("github.com/user/repo/ytmd/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if src.PkgPath != "ytmd" {
		t.Errorf("PkgPath = %q, want %q", src.PkgPath, "ytmd")
	}
}

func TestCloneURL(t *testing.T) {
	src := PackageSource{
		Host: "github.com",
		User: "merith-tk",
		Repo: "riverdeck-packages",
	}
	want := "https://github.com/merith-tk/riverdeck-packages"
	if got := src.CloneURL(); got != want {
		t.Errorf("CloneURL() = %q, want %q", got, want)
	}
}

func TestRef(t *testing.T) {
	tests := []struct {
		tag    string
		branch string
		want   string
	}{
		{"v1.0.0", "", "v1.0.0"},
		{"", "main", "main"},
		{"v1.0.0", "dev", "v1.0.0"}, // tag takes precedence
		{"", "", ""},
	}
	for _, tc := range tests {
		src := PackageSource{Tag: tc.tag, Branch: tc.branch}
		if got := src.Ref(); got != tc.want {
			t.Errorf("Ref(Tag=%q, Branch=%q) = %q, want %q", tc.tag, tc.branch, got, tc.want)
		}
	}
}

func TestIsTag(t *testing.T) {
	tests := []struct {
		ref  string
		want bool
	}{
		{"v1.0.0", true},
		{"v0.1", true},
		{"v2", false},           // no dot
		{"main", false},         // not a version
		{"dev", false},          // not a version
		{"abc123def456abc123def456abc123def456abcd", true}, // 40-char hex SHA
		{"abc123", false},       // too short
		{"", false},
	}
	for _, tc := range tests {
		if got := isTag(tc.ref); got != tc.want {
			t.Errorf("isTag(%q) = %v, want %v", tc.ref, got, tc.want)
		}
	}
}
