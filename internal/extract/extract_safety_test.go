package extract_test

import (
	"archive/zip"
	"bytes"
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
	parent := t.TempDir()
	dest := filepath.Join(parent, "checkout")

	archive := buildZip(t, zipEntry{name: "../escaped.txt", body: "pwned"})

	_, err := extract.Extract(archive, dest, false)
	if err == nil {
		t.Fatal("Extract accepted an entry that escapes the destination")
	}
	if !strings.Contains(err.Error(), "outside the destination") {
		t.Errorf("error = %q, want it to name the escaping entry", err)
	}
	if _, err := os.Stat(filepath.Join(parent, "escaped.txt")); !os.IsNotExist(err) {
		t.Error("escaped.txt was written outside the destination")
	}
}

// A failure halfway through must leave no half-extracted project and no staging
// directory behind for the next run to trip over.
func TestExtractLeavesNothingBehindOnFailure(t *testing.T) {
	parent := t.TempDir()
	dest := filepath.Join(parent, "checkout")

	archive := buildZip(t,
		zipEntry{name: "demo/pom.xml", body: "<project/>\n"},
		zipEntry{name: "../../escaped.txt", body: "pwned"},
	)

	if _, err := extract.Extract(archive, dest, false); err == nil {
		t.Fatal("Extract accepted an entry that escapes the destination")
	}

	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Error("a failed extraction left the destination behind")
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatalf("read parent: %v", err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("a failed extraction left %v behind", names)
	}
}

// With --force the destination is replaced, not merged: files from the previous
// project do not survive into the new one.
func TestExtractForceReplacesDestination(t *testing.T) {
	dest := t.TempDir()
	stale := filepath.Join(dest, "leftover.txt")
	if err := os.WriteFile(stale, []byte("old project"), 0o644); err != nil {
		t.Fatalf("seed destination: %v", err)
	}

	archive := buildZip(t, zipEntry{name: "demo/pom.xml", body: "<project/>\n"})

	result, err := extract.Extract(archive, dest, true)
	if err != nil {
		t.Fatalf("Extract with force: %v", err)
	}
	if !result.Replaced {
		t.Error("result.Replaced = false, want true")
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Error("leftover.txt survived a forced replacement")
	}
	if _, err := os.Stat(filepath.Join(dest, "demo", "pom.xml")); err != nil {
		t.Errorf("new project missing: %v", err)
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
