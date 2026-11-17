package extract_test

import (
	"archive/zip"
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/miferdev/spring-inicializador/internal/extract"
)

type zipEntry struct {
	name string
	body string
	mode os.FileMode
}

func buildZip(t *testing.T, entries ...zipEntry) []byte {
	t.Helper()

	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for _, entry := range entries {
		mode := entry.mode
		if mode == 0 {
			mode = 0o644
		}
		header := &zip.FileHeader{Name: entry.name, Method: zip.Deflate}
		header.SetMode(mode)

		file, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatalf("create %s: %v", entry.name, err)
		}
		if _, err := file.Write([]byte(entry.body)); err != nil {
			t.Fatalf("write %s: %v", entry.name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}
	return buf.Bytes()
}

// A hostile archive must not write outside the destination. The check happens
// before anything is created, so the escaping file simply never exists.
func TestExtractRejectsEscapingEntry(t *testing.T) {
	location := t.TempDir()
	project := filepath.Join(location, "checkout")

	archive := buildZip(t, zipEntry{name: "../escaped.txt", body: "pwned"})

	_, err := extract.Extract(archive, project, false)
	if err == nil {
		t.Fatal("Extract accepted an entry that escapes the destination")
	}
	if !strings.Contains(err.Error(), "outside the destination") {
		t.Errorf("error = %q, want it to name the escaping entry", err)
	}
	if _, err := os.Stat(filepath.Join(location, "escaped.txt")); !errors.Is(err, fs.ErrNotExist) {
		t.Error("escaped.txt was written outside the destination")
	}
}

// A failure halfway through must leave no half-extracted project and no staging
// directory behind for the next run to trip over.
func TestExtractLeavesNothingBehindOnFailure(t *testing.T) {
	location := t.TempDir()
	project := filepath.Join(location, "checkout")

	archive := buildZip(t,
		zipEntry{name: "demo/pom.xml", body: "<project/>\n"},
		zipEntry{name: "demo/../../escaped.txt", body: "pwned"},
	)

	if _, err := extract.Extract(archive, project, false); err == nil {
		t.Fatal("Extract accepted an entry that escapes the destination")
	}

	if _, err := os.Stat(project); !errors.Is(err, fs.ErrNotExist) {
		t.Error("a failed extraction left the project behind")
	}
	entries, err := os.ReadDir(location)
	if err != nil {
		t.Fatalf("read location: %v", err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("a failed extraction left %v behind", names)
	}
}

// With --force the project directory is replaced, not merged: files from the
// previous project do not survive into the new one — but the siblings next to it do.
func TestExtractForceReplacesOnlyTheProject(t *testing.T) {
	location := t.TempDir()
	project := filepath.Join(location, "proyecto1")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	stale := filepath.Join(project, "leftover.txt")
	if err := os.WriteFile(stale, []byte("old project"), 0o644); err != nil {
		t.Fatalf("seed project file: %v", err)
	}
	sibling := filepath.Join(location, "otro.txt")
	if err := os.WriteFile(sibling, []byte("somebody else's work"), 0o644); err != nil {
		t.Fatalf("seed sibling: %v", err)
	}

	archive := buildZip(t, zipEntry{name: "demo/pom.xml", body: "<project/>\n"})

	result, err := extract.Extract(archive, project, true)
	if err != nil {
		t.Fatalf("Extract with force: %v", err)
	}
	if !result.Replaced {
		t.Error("result.Replaced = false, want true")
	}
	if _, err := os.Stat(stale); !errors.Is(err, fs.ErrNotExist) {
		t.Error("leftover.txt survived a forced replacement")
	}
	if _, err := os.Stat(filepath.Join(project, "pom.xml")); err != nil {
		t.Errorf("new project missing: %v", err)
	}
	if body, err := os.ReadFile(sibling); err != nil || string(body) != "somebody else's work" {
		t.Errorf("the sibling was touched: body %q, err %v", body, err)
	}
}

// --force must never be able to delete the directory the user is standing in.
func TestExtractRefusesToReplaceWorkingDirectory(t *testing.T) {
	dest := t.TempDir()
	t.Chdir(dest)

	if err := os.WriteFile(filepath.Join(dest, "keep.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatalf("seed working directory: %v", err)
	}

	archive := buildZip(t, zipEntry{name: "demo/pom.xml", body: "<project/>\n"})

	if _, err := extract.Extract(archive, ".", true); err == nil {
		t.Fatal("Extract replaced the working directory")
	}

	if _, err := os.Stat(filepath.Join(dest, "keep.txt")); err != nil {
		t.Fatalf("keep.txt was removed by a refused run: %v", err)
	}
}

// A filesystem root is never a project directory.
func TestExtractRefusesFilesystemRoot(t *testing.T) {
	archive := buildZip(t, zipEntry{name: "demo/pom.xml", body: "<project/>\n"})

	if _, err := extract.Extract(archive, string(filepath.Separator), true); err == nil {
		t.Fatal("Extract accepted a filesystem root as the project directory")
	}
}
