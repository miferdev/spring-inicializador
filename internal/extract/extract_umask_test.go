//go:build !windows

package extract_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/miferdev/spring-inicializador/internal/extract"
)

// A restrictive process umask must not strip the permission bits the archive
// stores. CI images and servers routinely run with umask 077 or 027, and under
// those a mvnw created with 0755 would end up 0700 — present, but not the
// executable the project expects.
func TestExtractIgnoresProcessUmask(t *testing.T) {
	previous := syscall.Umask(0o077)
	t.Cleanup(func() { syscall.Umask(previous) })

	dest := t.TempDir()
	if _, err := extract.Extract(testArchive(t), dest, false); err != nil {
		t.Fatalf("Extract: %v", err)
	}

	cases := map[string]os.FileMode{
		"demo/mvnw":    0o755,
		"demo/pom.xml": 0o644,
	}
	for path, want := range cases {
		info, err := os.Stat(filepath.Join(dest, filepath.FromSlash(path)))
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s mode = %o, want %o", path, got, want)
		}
	}
}
