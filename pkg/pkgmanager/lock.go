package pkgmanager

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// lockFile is the name of the checksum lock file inside the packages dir.
const lockFile = "riverdeck.lock"

// LockFile maps relative file paths (within packagesDir) to SHA-256 checksums.
type LockFile map[string]string

// LoadLock reads the riverdeck.lock from the packages directory.
// Returns an empty lock if the file does not exist.
func LoadLock(packagesDir string) (LockFile, error) {
	data, err := os.ReadFile(filepath.Join(packagesDir, lockFile))
	if os.IsNotExist(err) {
		return make(LockFile), nil
	}
	if err != nil {
		return nil, err
	}
	var lf LockFile
	if err := json.Unmarshal(data, &lf); err != nil {
		return nil, err
	}
	if lf == nil {
		lf = make(LockFile)
	}
	return lf, nil
}

// SaveLock writes lf to the packages directory.
func SaveLock(packagesDir string, lf LockFile) error {
	if err := os.MkdirAll(packagesDir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(lf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(packagesDir, lockFile), data, 0644)
}

// UpdatePackageLock recomputes checksums for all files under pkgDir and
// stores them in lf using paths relative to packagesDir.
func UpdatePackageLock(lf LockFile, packagesDir, pkgDir string) error {
	return filepath.Walk(pkgDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		sum, err := sha256File(path)
		if err != nil {
			return nil // skip unreadable files
		}
		rel, err := filepath.Rel(packagesDir, path)
		if err != nil {
			return nil
		}
		lf[filepath.ToSlash(rel)] = sum
		return nil
	})
}

// RemovePackageLock removes all entries under pkgDir from lf.
func RemovePackageLock(lf LockFile, packagesDir, pkgDir string) {
	prefix, _ := filepath.Rel(packagesDir, pkgDir)
	prefix = filepath.ToSlash(prefix) + "/"
	for k := range lf {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			delete(lf, k)
		}
	}
}

// VerifyPackage checks that all files under pkgDir match their recorded
// checksums. Returns a list of mismatched or missing file paths.
func VerifyPackage(lf LockFile, packagesDir, pkgDir string) []string {
	var mismatches []string
	_ = filepath.Walk(pkgDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(packagesDir, path)
		if err != nil {
			return nil
		}
		key := filepath.ToSlash(rel)
		expected, ok := lf[key]
		if !ok {
			mismatches = append(mismatches, key+" (unlocked)")
			return nil
		}
		got, err := sha256File(path)
		if err != nil || got != expected {
			mismatches = append(mismatches, key)
		}
		return nil
	})
	sort.Strings(mismatches)
	return mismatches
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
