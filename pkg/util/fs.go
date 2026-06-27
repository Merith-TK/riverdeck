package util

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func ExtractFS(srcFS fs.FS, destDir string, prefix string) error {
	return fs.WalkDir(srcFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := path
		if prefix != "" {
			rel = strings.TrimPrefix(rel, prefix)
			rel = strings.TrimPrefix(rel, "/")
		}
		if rel == "" || rel == "." {
			return nil
		}
		dest := filepath.Join(destDir, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(dest, 0755)
		}
		if mkErr := os.MkdirAll(filepath.Dir(dest), 0755); mkErr != nil {
			return mkErr
		}
		srcFile, openErr := srcFS.Open(path)
		if openErr != nil {
			return openErr
		}
		defer srcFile.Close()
		destFile, createErr := os.Create(dest)
		if createErr != nil {
			return createErr
		}
		defer destFile.Close()
		_, copyErr := io.Copy(destFile, srcFile)
		return copyErr
	})
}
