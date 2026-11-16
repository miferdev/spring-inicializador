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

// testArchive mirrors the shape of a real starter.zip: an explicit directory
// entry for the top-level project directory, a regular file and an executable
// script.
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
	dest := t.TempDir()

	if _, err := extract.Extract(testArchive(t), dest, false); err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// The archive's top-level directory is preserved, not flattened.
	dirInfo, err := os.Stat(filepath.Join(dest, "demo"))
	if err != nil {
		t.Fatalf("stat demo: %v", err)
	}
	if !dirInfo.IsDir() {
		t.Fatalf("demo is not a directory")
	}

	checks := []struct {
		path string
		body string
		mode os.FileMode
	}{
		{"demo/pom.xml", "<project/>\n", 0o644},
		{"demo/src/main/java/com/example/demo/DemoApplication.java", "package com.example.demo;\n", 0o644},
		{"demo/mvnw", "#!/bin/sh\n", 0o755},
	}
	for _, check := range checks {
		full := filepath.Join(dest, filepath.FromSlash(check.path))

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

func TestExtractInvalidArchive(t *testing.T) {
	if _, err := extract.Extract([]byte("not a zip"), t.TempDir(), false); err == nil {
		t.Fatal("Extract accepted a non-zip body")
	}
}

// A destination with anything in it is somebody's project. Refusing before the
// first write is what makes a typo in --output harmless.
func TestExtractRefusesExistingDestination(t *testing.T) {
	dest := t.TempDir()
	keep := filepath.Join(dest, "keep.txt")
	if err := os.WriteFile(keep, []byte("mine"), 0o644); err != nil {
		t.Fatalf("seed destination: %v", err)
	}

	result, err := extract.Extract(testArchive(t), dest, false)
	if err == nil {
		t.Fatal("Extract result.Replaced a non-empty destination without force")
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
	if _, err := os.Stat(filepath.Join(dest, "demo")); !errors.Is(err, fs.ErrNotExist) {
		t.Error("Extract wrote into a destination it refused")
	}
}

func TestExtractOverwritesWithForce(t *testing.T) {
	dest := t.TempDir()
	stale := filepath.Join(dest, "demo", "pom.xml")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatalf("seed destination: %v", err)
	}
	if err := os.WriteFile(stale, []byte("stale"), 0o644); err != nil {
		t.Fatalf("seed stale file: %v", err)
	}

	result, err := extract.Extract(testArchive(t), dest, true)
	if err != nil {
		t.Fatalf("Extract with force: %v", err)
	}
	if !result.Replaced {
		t.Error("result.Replaced = false, want true when force replaced an existing destination")
	}

	body, err := os.ReadFile(stale)
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
	result, err := extract.Extract(testArchive(t), t.TempDir(), false)
	if err != nil {
		t.Fatalf("Extract into an empty destination: %v", err)
	}
	if result.Replaced {
		t.Error("result.Replaced = true, want false for an empty destination")
	}
}
