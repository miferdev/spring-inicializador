// Package extract writes an in-memory project archive to disk. It never
// fetches: it receives bytes plus a destination directory, and it either leaves
// a complete project behind or nothing at all.
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
	"sort"
	"strings"
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

	// stagingPrefix names the temporary sibling directory an extraction is
	// assembled in before it is renamed into place.
	stagingPrefix = ".spring-inicializador-"
)

// Result is what an extraction did: whether an existing destination was replaced,
// how many files were written, and which of them the archive made executable. The
// caller reports these instead of guessing from the archive (US6.2).
type Result struct {
	// Replaced reports that an existing destination was moved aside and replaced.
	Replaced bool

	// Files is how many regular files were written.
	Files int

	// Executable names the files written with an executable bit, as the archive
	// spells them ("demo/mvnw"), sorted. A Maven project ships mvnw, a Gradle one
	// gradlew; an archive may ship none.
	Executable []string
}

// Extract unpacks data into destDir, preserving the archive's top-level
// directory (the server sends `demo/...`, so `--output .` yields `./demo`) and
// each entry's stored permission bits, including the executable bit on mvnw and
// gradlew.
//
// A destination that already exists and holds anything is refused unless force
// is set; nothing is written before that check, so a refused run leaves the
// existing directory exactly as it was. The result reports whether an existing
// destination was replaced, which lets the caller say "overwrote" instead of
// "created", and what was written, which lets it report the extraction and the
// permissions it applied (US6.2).
//
// The archive is assembled in a temporary sibling directory and renamed into
// place only once every entry has been written, so a failure partway through
// cannot leave a half-extracted project behind.
func Extract(data []byte, destDir string, force bool) (Result, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return Result{}, fmt.Errorf("extract: opening archive: %w", err)
	}

	exists, empty, err := dirState(destDir)
	if err != nil {
		return Result{}, fmt.Errorf("extract: inspecting %s: %w", destDir, err)
	}
	replacing := exists && !empty
	if replacing && !force {
		return Result{}, fmt.Errorf("extract: destination %s already exists and is not empty; pass --force to overwrite", destDir)
	}
	if replacing {
		if err := refuseUnsafeDestination(destDir); err != nil {
			return Result{}, err
		}
	}

	parent := filepath.Dir(destDir)
	if err := os.MkdirAll(parent, privateDirMode); err != nil {
		return Result{}, fmt.Errorf("extract: creating parent of %s: %w", destDir, err)
	}

	staging, err := os.MkdirTemp(parent, stagingPrefix)
	if err != nil {
		return Result{}, fmt.Errorf("extract: creating staging directory: %w", err)
	}
	// Until the rename succeeds, the staging directory is this function's
	// problem; afterwards it belongs to the caller's destination.
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(staging)
		}
	}()

	result := Result{Replaced: replacing}
	for _, file := range reader.File {
		perm, err := extractFile(file, staging)
		if err != nil {
			return Result{}, fmt.Errorf("extract: %s: %w", file.Name, err)
		}
		if file.FileInfo().IsDir() {
			continue
		}

		result.Files++
		if perm&0o111 != 0 {
			result.Executable = append(result.Executable, file.Name)
		}
	}
	sort.Strings(result.Executable)

	if err := adopt(staging, destDir, exists, replacing); err != nil {
		return Result{}, err
	}
	committed = true
	return result, nil
}

// adopt moves the finished staging directory into destDir. An empty destination
// has to be removed first, because a directory cannot be renamed onto an
// existing one. A destination being replaced is parked beside itself, so a
// failed rename can put it back instead of losing it.
func adopt(staging, destDir string, exists, replacing bool) error {
	if !replacing {
		if exists {
			if err := os.Remove(destDir); err != nil {
				return fmt.Errorf("extract: clearing empty %s: %w", destDir, err)
			}
		}
		if err := os.Rename(staging, destDir); err != nil {
			return fmt.Errorf("extract: moving project into %s: %w", destDir, err)
		}
		return nil
	}

	backup := destDir + ".replaced"
	if err := os.RemoveAll(backup); err != nil {
		return fmt.Errorf("extract: clearing stale backup %s: %w", backup, err)
	}
	if err := os.Rename(destDir, backup); err != nil {
		return fmt.Errorf("extract: moving existing %s aside: %w", destDir, err)
	}
	if err := os.Rename(staging, destDir); err != nil {
		// Put the original back; the caller sees the failure, not a vanished
		// project.
		_ = os.Rename(backup, destDir)
		return fmt.Errorf("extract: moving project into %s: %w", destDir, err)
	}
	if err := os.RemoveAll(backup); err != nil {
		return fmt.Errorf("extract: removing backup %s: %w", backup, err)
	}
	return nil
}

// refuseUnsafeDestination stops --force from replacing something that is not a
// project: the current directory, its parent, or a filesystem root. Those are
// never what a user meant, and removing them is not recoverable.
func refuseUnsafeDestination(destDir string) error {
	clean := filepath.Clean(destDir)
	unattended := clean == "." || clean == ".." || filepath.Dir(clean) == clean
	if unattended {
		return fmt.Errorf("extract: refusing to replace %s; point --output at a project directory", destDir)
	}
	return nil
}

// dirState reports whether path exists and, if it does, whether it holds
// anything. A missing destination is not a conflict: that is the normal first
// run.
func dirState(path string) (exists, empty bool, err error) {
	entries, err := os.ReadDir(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, true, nil
	}
	if err != nil {
		return false, false, err
	}
	return true, len(entries) == 0, nil
}

// extractFile writes one entry and returns the permission bits it applied, so the
// caller can report what the archive made executable.
func extractFile(file *zip.File, destDir string) (os.FileMode, error) {
	path, err := inside(destDir, file.Name)
	if err != nil {
		return 0, err
	}
	mode := file.Mode()

	if file.FileInfo().IsDir() {
		perm := mode.Perm()
		if perm == 0 {
			perm = defaultDirMode
		}
		if err := os.MkdirAll(path, privateDirMode); err != nil {
			return 0, err
		}
		return perm, chmod(path, perm)
	}
	if !mode.IsRegular() {
		// Symlinks and devices are not part of a Spring Initializr archive;
		// refusing them keeps a hostile archive from creating one.
		return 0, fmt.Errorf("unsupported file type %s", mode.Type())
	}

	if err := os.MkdirAll(filepath.Dir(path), privateDirMode); err != nil {
		return 0, fmt.Errorf("creating parent directory: %w", err)
	}

	perm := mode.Perm()
	if perm == 0 {
		perm = defaultFileMode
	}

	src, err := file.Open()
	if err != nil {
		return 0, fmt.Errorf("opening archive entry: %w", err)
	}
	defer src.Close()

	dst, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, privateFileMode)
	if err != nil {
		return 0, fmt.Errorf("creating file: %w", err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return 0, fmt.Errorf("writing file: %w", err)
	}
	if err := dst.Close(); err != nil {
		return 0, fmt.Errorf("closing file: %w", err)
	}

	// Widen from the private mode now that the content is in place.
	return perm, chmod(path, perm)
}

// inside resolves an archive entry name against destDir and rejects anything
// that would resolve above it, like "../etc/passwd" or "demo/../../x". The check
// happens before a single byte is written.
func inside(destDir, name string) (string, error) {
	path := filepath.Join(destDir, filepath.FromSlash(name))

	rel, err := filepath.Rel(destDir, path)
	if err != nil {
		return "", fmt.Errorf("resolving %q: %w", name, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("refusing entry outside the destination: %q", name)
	}
	return path, nil
}

// chmod applies the archive's permission bits exactly, bypassing the umask that
// applied when the entry was created.
func chmod(path string, perm os.FileMode) error {
	if err := os.Chmod(path, perm); err != nil {
		return fmt.Errorf("setting permissions: %w", err)
	}
	return nil
}
