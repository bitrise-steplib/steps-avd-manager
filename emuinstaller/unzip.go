package emuinstaller

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// unzip extracts src into dest, preserving the file modes recorded in the archive.
// The emulator binaries rely on their executable bit, so the mode must survive extraction.
func unzip(src, dest string) error {
	reader, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("open archive %s: %w", src, err)
	}
	defer reader.Close() //nolint:errcheck

	if err := os.MkdirAll(dest, 0755); err != nil {
		return fmt.Errorf("create destination %s: %w", dest, err)
	}

	root := filepath.Clean(dest) + string(os.PathSeparator)
	for _, file := range reader.File {
		path := filepath.Join(dest, file.Name) //nolint:gosec // guarded against path traversal below
		if !strings.HasPrefix(path, root) {
			return fmt.Errorf("archive entry escapes the destination directory: %s", file.Name)
		}

		if err := extractEntry(file, path); err != nil {
			return err
		}
	}

	return nil
}

func extractEntry(file *zip.File, path string) error {
	if file.FileInfo().IsDir() {
		if err := os.MkdirAll(path, file.Mode()); err != nil {
			return fmt.Errorf("create directory %s: %w", path, err)
		}
		return nil
	}

	// Archives don't always list a directory entry before the files it contains.
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create directory %s: %w", filepath.Dir(path), err)
	}

	source, err := file.Open()
	if err != nil {
		return fmt.Errorf("open archive entry %s: %w", file.Name, err)
	}
	defer source.Close() //nolint:errcheck

	target, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
	if err != nil {
		return fmt.Errorf("create file %s: %w", path, err)
	}

	if _, err := io.Copy(target, source); err != nil { //nolint:gosec // trusted archive from the Android emulator repository
		target.Close() //nolint:errcheck,gosec
		return fmt.Errorf("write file %s: %w", path, err)
	}

	if err := target.Close(); err != nil {
		return fmt.Errorf("close file %s: %w", path, err)
	}

	return nil
}
