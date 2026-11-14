package extract_test

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
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

	if err := extract.Extract(testArchive(t), dest); err != nil {
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
	if err := extract.Extract([]byte("not a zip"), t.TempDir()); err == nil {
		t.Fatal("Extract accepted a non-zip body")
	}
}
