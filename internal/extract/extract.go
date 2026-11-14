// Package extract writes an in-memory project archive to disk. It never
// fetches: it receives bytes plus a destination directory. Atomic renames and
// path-escaping rejection belong to US1.6; the structure here keeps those
// additions local.
package extract

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	// defaultDirMode and defaultFileMode are used only when the archive stores
	// no permission bits at all; an entry with no mode would otherwise be
	// written unreadable.
	defaultDirMode  = 0o755
	defaultFileMode = 0o644

	// Entries are created privately and widened to their stored mode with an
	// explicit chmod afterwards. Both MkdirAll and OpenFile apply the process
	// umask, so creating 0755 directly would lose the group and other bits
	// under a restrictive umask — a checkout with mvnw at 0700 is not the
	// project the server shipped.
	privateDirMode  = 0o700
	privateFileMode = 0o600
)

// Extract unpacks data into destDir, preserving the archive's top-level
// directory (the server sends `demo/...`, so `--output .` yields `./demo`) and
// each entry's stored permission bits, including the executable bit on mvnw and
// gradlew.
//
// A destination that already exists and holds anything is refused unless force
// is set. Nothing is written before that check, so a refused run leaves the
// existing directory exactly as it was. The second return value reports whether
// an existing destination was replaced, which lets the caller say "overwrote"
// instead of "created".
func Extract(data []byte, destDir string, force bool) (bool, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return false, fmt.Errorf("extract: opening archive: %w", err)
	}

	empty, err := dirIsEmpty(destDir)
	if err != nil {
		return false, fmt.Errorf("extract: inspecting %s: %w", destDir, err)
	}
	if !empty && !force {
		return false, fmt.Errorf("extract: destination %s already exists and is not empty; pass --force to overwrite", destDir)
	}

	if err := os.MkdirAll(destDir, privateDirMode); err != nil {
		return false, fmt.Errorf("extract: creating destination %s: %w", destDir, err)
	}

	for _, file := range reader.File {
		if err := extractFile(file, destDir); err != nil {
			return false, fmt.Errorf("extract: %s: %w", file.Name, err)
		}
	}
	return !empty, nil
}

// dirIsEmpty reports whether path is missing or holds nothing. A missing
// destination is not a conflict: that is the normal first run.
func dirIsEmpty(path string) (bool, error) {
	entries, err := os.ReadDir(path)
	if errors.Is(err, fs.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
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
		if err := os.MkdirAll(path, privateDirMode); err != nil {
			return err
		}
		return chmod(path, perm)
	}
	if !mode.IsRegular() {
		// Symlinks and devices are not part of a Spring Initializr archive;
		// refusing them keeps a hostile archive from creating one.
		return fmt.Errorf("unsupported file type %s", mode.Type())
	}

	if err := os.MkdirAll(filepath.Dir(path), privateDirMode); err != nil {
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

	dst, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, privateFileMode)
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

	// Widen from the private mode now that the content is in place.
	return chmod(path, perm)
}

// chmod applies the archive's permission bits exactly, bypassing the umask that
// applied when the entry was created.
func chmod(path string, perm os.FileMode) error {
	if err := os.Chmod(path, perm); err != nil {
		return fmt.Errorf("setting permissions: %w", err)
	}
	return nil
}
