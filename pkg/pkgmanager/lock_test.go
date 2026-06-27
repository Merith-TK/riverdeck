package pkgmanager

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadLock_NotExist(t *testing.T) {
	dir := t.TempDir()
	lf, err := LoadLock(dir)
	if err != nil {
		t.Fatalf("LoadLock on empty dir: %v", err)
	}
	if lf == nil {
		t.Fatal("LoadLock returned nil map")
	}
	if len(lf) != 0 {
		t.Errorf("LoadLock returned %d entries, want 0", len(lf))
	}
}

func TestSaveAndLoadLock(t *testing.T) {
	dir := t.TempDir()
	lf := LockFile{
		"packages/mypkg/lib/util.lua": "abc123",
		"packages/mypkg/daemon.lua":   "def456",
	}
	if err := SaveLock(dir, lf); err != nil {
		t.Fatalf("SaveLock: %v", err)
	}

	loaded, err := LoadLock(dir)
	if err != nil {
		t.Fatalf("LoadLock: %v", err)
	}
	if len(loaded) != 2 {
		t.Errorf("LoadLock returned %d entries, want 2", len(loaded))
	}
	if loaded["packages/mypkg/lib/util.lua"] != "abc123" {
		t.Errorf("hash mismatch for util.lua")
	}
	if loaded["packages/mypkg/daemon.lua"] != "def456" {
		t.Errorf("hash mismatch for daemon.lua")
	}
}

func TestUpdatePackageLock(t *testing.T) {
	pkgDir := t.TempDir()
	packagesDir := t.TempDir()

	// Create a dummy file in pkgDir.
	pkgFile := filepath.Join(pkgDir, "test.lua")
	if err := os.WriteFile(pkgFile, []byte("local x = 1"), 0644); err != nil {
		t.Fatal(err)
	}

	lf := make(LockFile)
	if err := UpdatePackageLock(lf, packagesDir, pkgDir); err != nil {
		t.Fatalf("UpdatePackageLock: %v", err)
	}

	// Compute relative path from packagesDir
	rel, err := filepath.Rel(packagesDir, pkgFile)
	if err != nil {
		t.Fatal(err)
	}
	rel = filepath.ToSlash(rel)

	hash, ok := lf[rel]
	if !ok {
		t.Fatalf("UpdatePackageLock did not add entry for %s", rel)
	}
	if len(hash) != 64 {
		t.Errorf("SHA256 hash length = %d, want 64", len(hash))
	}
}

func TestRemovePackageLock(t *testing.T) {
	pkgsDir := t.TempDir()
	pkgDir := filepath.Join(pkgsDir, "github.com", "user", "repo")

	lf := LockFile{
		"github.com/user/repo/lib/util.lua":   "aaa",
		"github.com/user/repo/daemon.lua":     "bbb",
		"github.com/other/repo/main.lua":      "ccc",
	}

	RemovePackageLock(lf, pkgsDir, pkgDir)

	if _, ok := lf["github.com/user/repo/lib/util.lua"]; ok {
		t.Error("RemovePackageLock did not remove lib/util.lua")
	}
	if _, ok := lf["github.com/user/repo/daemon.lua"]; ok {
		t.Error("RemovePackageLock did not remove daemon.lua")
	}
	if lf["github.com/other/repo/main.lua"] != "ccc" {
		t.Error("RemovePackageLock removed unrelated entry")
	}
}

func TestVerifyPackage(t *testing.T) {
	pkgsDir := t.TempDir()
	pkgDir := filepath.Join(pkgsDir, "mypkg")

	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a file and compute its hash.
	filePath := filepath.Join(pkgDir, "test.lua")
	if err := os.WriteFile(filePath, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	lf := make(LockFile)
	if err := UpdatePackageLock(lf, pkgsDir, pkgDir); err != nil {
		t.Fatal(err)
	}

	// Verify should pass.
	mismatches := VerifyPackage(lf, pkgsDir, pkgDir)
	if len(mismatches) > 0 {
		t.Errorf("VerifyPackage returned mismatches for valid files: %v", mismatches)
	}

	// Modify the file — should now mismatch.
	if err := os.WriteFile(filePath, []byte("world"), 0644); err != nil {
		t.Fatal(err)
	}
	mismatches = VerifyPackage(lf, pkgsDir, pkgDir)
	if len(mismatches) == 0 {
		t.Error("VerifyPackage should report mismatch after file modification")
	}
}

func TestVerifyPackage_UnlockedFile(t *testing.T) {
	pkgsDir := t.TempDir()
	pkgDir := filepath.Join(pkgsDir, "mypkg")

	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a file but don't add it to the lock.
	filePath := filepath.Join(pkgDir, "unlocked.lua")
	if err := os.WriteFile(filePath, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	lf := make(LockFile)
	mismatches := VerifyPackage(lf, pkgsDir, pkgDir)
	if len(mismatches) == 0 {
		t.Error("VerifyPackage should report unlocked file")
	}
}

func TestSHA256File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	hash, err := sha256File(path)
	if err != nil {
		t.Fatalf("sha256File: %v", err)
	}
	// known SHA-256 of "hello"
	want := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if hash != want {
		t.Errorf("sha256File('hello') = %q, want %q", hash, want)
	}
}
