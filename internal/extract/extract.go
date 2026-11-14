// Package extract writes an in-memory project archive to disk. It never
// fetches: it receives bytes plus a destination directory. Path-escaping
// rejection, existing-destination refusal and atomic renames belong to later
// stories (US1.5, US1.6); the structure here keeps those additions local.
package extract

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const defaultDirMode = 0o755

// defaultFileMode is used only when the archive stores no permission bits at
// all; a file with no mode would otherwise be written unreadable.
const defaultFileMode = 0o644

// Extract unpacks data into destDir, preserving the archive's top-level
// directory (the server sends `demo/...`, so `--output .` yields `./demo`) and
// each entry's stored permission bits.
func Extract(data []byte, destDir string) error {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("extract: opening archive: %w", err)
	}

	if err := os.MkdirAll(destDir, defaultDirMode); err != nil {
		return fmt.Errorf("extract: creating destination %s: %w", destDir, err)
	}

	for _, file := range reader.File {
		if err := extractFile(file, destDir); err != nil {
			return fmt.Errorf("extract: %s: %w", file.Name, err)
		}
	}
	return nil
}

func extractFile(file *zip.File, destDir string) error {
	// US1.6 rejects entries that escape destDir here, before any path is joined
	// or any write happens.
	path := filepath.Join(destDir, filepath.FromSlash(file.Name))
	mode := file.Mode()

	if file.FileInfo().IsDir() {
		perm := mode.Perm()
		if perm == 0 {
			perm = defaultDirMode
		}
		return os.MkdirAll(path, perm)
	}
	if !mode.IsRegular() {
		// Symlinks and devices are not part of a Spring Initializr archive;
		// refusing them keeps a hostile archive from creating one.
		return fmt.Errorf("unsupported file type %s", mode.Type())
	}

	if err := os.MkdirAll(filepath.Dir(path), defaultDirMode); err != nil {
		return fmt.Errorf("creating parent directory: %w", err)
	}

	perm := mode.Perm()
	if perm == 0 {
		perm = defaultFileMode
	}

	src, err := file.Open()
	if err != nil {
		return fmt.Errorf("opening archive entry: %w", err)
	}
	defer src.Close()

	dst, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return fmt.Errorf("creating file: %w", err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return fmt.Errorf("writing file: %w", err)
	}
	if err := dst.Close(); err != nil {
		return fmt.Errorf("closing file: %w", err)
	}
	return nil
}
