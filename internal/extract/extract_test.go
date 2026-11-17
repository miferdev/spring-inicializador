package extract_test

// Tests for the extraction contract: the destination is the project's own
// directory, the archive's single top-level directory is renamed to it, and a
// failure leaves neither a project nor a staging directory behind (VS8.2).

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

// testArchive mirrors the shape of a real starter.zip: an explicit directory
// entry for the top-level project directory, a regular file and an executable
// script. The top directory is named after the server's baseDir, which is the
// artifact id the user asked for — not the folder the project is written into.
func testArchive(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)

	dirHeader := &zip.FileHeader{Name: "demo/", Method: zip.Store}
	dirHeader.SetMode(os.ModeDir | 0o755)
	if _, err := writer.CreateHeader(dirHeader); err != nil {
		t.Fatalf("create directory entry: %v", err)
	}

	files := []struct {
		name string
		body string
		mode os.FileMode
	}{
		{"demo/pom.xml", "<project/>\n", 0o644},
		{"demo/src/main/java/com/example/demo/DemoApplication.java", "package com.example.demo;\n", 0o644},
		{"demo/mvnw", "#!/bin/sh\n", 0o755},
	}
	for _, file := range files {
		header := &zip.FileHeader{Name: file.name, Method: zip.Deflate}
		header.SetMode(file.mode)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatalf("create %s: %v", file.name, err)
		}
		if _, err := entry.Write([]byte(file.body)); err != nil {
			t.Fatalf("write %s: %v", file.name, err)
		}
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}
	return buf.Bytes()
}

func TestExtract(t *testing.T) {
	project := filepath.Join(t.TempDir(), "proyecto1")

	result, err := extract.Extract(testArchive(t), project, false)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if result.Replaced {
		t.Error("result.Replaced = true, want false for a new project")
	}
	if result.Files != 3 {
		t.Errorf("result.Files = %d, want 3", result.Files)
	}
	// The executable keeps its name relative to the project, not to the archive.
	if got, want := strings.Join(result.Executable, ","), "mvnw"; got != want {
		t.Errorf("result.Executable = %q, want %q", got, want)
	}

	checks := []struct {
		path string
		body string
		mode os.FileMode
	}{
		{"pom.xml", "<project/>\n", 0o644},
		{"src/main/java/com/example/demo/DemoApplication.java", "package com.example.demo;\n", 0o644},
		{"mvnw", "#!/bin/sh\n", 0o755},
	}
	for _, check := range checks {
		full := filepath.Join(project, filepath.FromSlash(check.path))

		body, err := os.ReadFile(full)
		if err != nil {
			t.Errorf("read %s: %v", check.path, err)
			continue
		}
		if string(body) != check.body {
			t.Errorf("%s body = %q, want %q", check.path, body, check.body)
		}

		info, err := os.Stat(full)
		if err != nil {
			t.Errorf("stat %s: %v", check.path, err)
			continue
		}
		if got := info.Mode().Perm(); got != check.mode {
			t.Errorf("%s mode = %o, want %o", check.path, got, check.mode)
		}
	}
}

// TestExtractRenamesTheArchiveTopDirectory is the story's core: the archive ships
// `demo/...`, the user asked for `proyecto1`, so the project must land in
// `proyecto1/` with no `proyecto1/demo/` inside it.
func TestExtractRenamesTheArchiveTopDirectory(t *testing.T) {
	project := filepath.Join(t.TempDir(), "proyecto1")

	if _, err := extract.Extract(testArchive(t), project, false); err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if _, err := os.Stat(filepath.Join(project, "pom.xml")); err != nil {
		t.Errorf("proyecto1/pom.xml is missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, "demo")); !errors.Is(err, fs.ErrNotExist) {
		t.Error("proyecto1/demo still exists: the archive's top directory was not renamed")
	}
}

// TestExtractLeavesASiblingLocationAlone: a non-empty location without the project
// folder is the normal case, and the extraction must not touch what is already
// there (VS8.2 fixes the bug where any non-empty --output was refused).
func TestExtractLeavesASiblingLocationAlone(t *testing.T) {
	location := t.TempDir()
	sibling := filepath.Join(location, "otro.txt")
	if err := os.WriteFile(sibling, []byte("not mine"), 0o644); err != nil {
		t.Fatalf("seed sibling: %v", err)
	}

	project := filepath.Join(location, "proyecto1")
	if _, err := extract.Extract(testArchive(t), project, false); err != nil {
		t.Fatalf("Extract into a non-empty location: %v", err)
	}

	if body, err := os.ReadFile(sibling); err != nil || string(body) != "not mine" {
		t.Errorf("sibling changed: body %q, err %v", body, err)
	}
	if _, err := os.Stat(filepath.Join(project, "pom.xml")); err != nil {
		t.Errorf("the project was not written: %v", err)
	}
}

func TestExtractInvalidArchive(t *testing.T) {
	if _, err := extract.Extract([]byte("not a zip"), filepath.Join(t.TempDir(), "p"), false); err == nil {
		t.Fatal("Extract accepted a non-zip body")
	}
}

// A destination with anything in it is somebody's project. Refusing before the
// first write is what makes a typo in the location harmless.
func TestExtractRefusesExistingDestination(t *testing.T) {
	location := t.TempDir()
	project := filepath.Join(location, "proyecto1")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	keep := filepath.Join(project, "keep.txt")
	if err := os.WriteFile(keep, []byte("mine"), 0o644); err != nil {
		t.Fatalf("seed destination: %v", err)
	}

	result, err := extract.Extract(testArchive(t), project, false)
	if err == nil {
		t.Fatal("Extract accepted a non-empty project directory without force")
	}
	if result.Replaced {
		t.Error("result.Replaced = true, want false on a refused run")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("error = %q, want it to name the flag that overrides the refusal", err)
	}
	if body, err := os.ReadFile(keep); err != nil || string(body) != "mine" {
		t.Errorf("existing file changed: body %q, err %v", body, err)
	}
	if _, err := os.Stat(filepath.Join(project, "pom.xml")); !errors.Is(err, fs.ErrNotExist) {
		t.Error("Extract wrote into a project directory it refused")
	}
}

func TestExtractOverwritesWithForce(t *testing.T) {
	location := t.TempDir()
	project := filepath.Join(location, "proyecto1")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	stale := filepath.Join(project, "stale.txt")
	if err := os.WriteFile(stale, []byte("stale"), 0o644); err != nil {
		t.Fatalf("seed stale file: %v", err)
	}

	result, err := extract.Extract(testArchive(t), project, true)
	if err != nil {
		t.Fatalf("Extract with force: %v", err)
	}
	if !result.Replaced {
		t.Error("result.Replaced = false, want true when force replaced an existing project")
	}

	if _, err := os.Stat(stale); !errors.Is(err, fs.ErrNotExist) {
		t.Error("stale.txt survived a forced replacement")
	}
	body, err := os.ReadFile(filepath.Join(project, "pom.xml"))
	if err != nil {
		t.Fatalf("read overwritten file: %v", err)
	}
	if string(body) != "<project/>\n" {
		t.Errorf("pom.xml body = %q, want the archive contents", body)
	}
}

// An empty directory is the normal state of `mkdir checkout && --output checkout`,
// so it is not a conflict.
func TestExtractAllowsEmptyExistingDestination(t *testing.T) {
	project := filepath.Join(t.TempDir(), "proyecto1")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	result, err := extract.Extract(testArchive(t), project, false)
	if err != nil {
		t.Fatalf("Extract into an empty project directory: %v", err)
	}
	if result.Replaced {
		t.Error("result.Replaced = true, want false for an empty destination")
	}
}

// TestExtractRejectsSeveralTopLevelEntries: flattening an archive with two roots
// would drop or merge somebody's files, and the server never sends one.
func TestExtractRejectsSeveralTopLevelEntries(t *testing.T) {
	archive := buildZip(t,
		zipEntry{name: "demo/pom.xml", body: "<project/>\n"},
		zipEntry{name: "other/readme.md", body: "# other\n"},
	)
	location := t.TempDir()
	project := filepath.Join(location, "proyecto1")

	_, err := extract.Extract(archive, project, false)
	if err == nil {
		t.Fatal("Extract accepted an archive with two top-level entries")
	}
	if !strings.Contains(err.Error(), "top-level entries") {
		t.Errorf("error = %q, want it to name the problem", err)
	}
	if !strings.Contains(err.Error(), "demo") || !strings.Contains(err.Error(), "other") {
		t.Errorf("error = %q, want it to name both entries", err)
	}
	if entries, err := os.ReadDir(location); err != nil || len(entries) != 0 {
		t.Errorf("a refused extraction left %v behind (err %v)", entries, err)
	}
}

// TestExtractRejectsAFileAtTheTopLevel: an archive whose single top-level entry is
// a file has no project directory to rename.
func TestExtractRejectsAFileAtTheTopLevel(t *testing.T) {
	archive := buildZip(t, zipEntry{name: "pom.xml", body: "<project/>\n"})

	_, err := extract.Extract(archive, filepath.Join(t.TempDir(), "proyecto1"), false)
	if err == nil {
		t.Fatal("Extract accepted an archive without a project directory")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("error = %q, want it to say the top-level entry is not a directory", err)
	}
}
