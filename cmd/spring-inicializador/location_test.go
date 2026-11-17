package main

// Tests for VS8.2: the run writes into the project's own directory. The location
// is where the user keeps their work and is never replaced, an existing project
// directory is, and an archive that cannot be renamed cleanly is refused instead of
// flattened. The fake Initializr keeps the suite offline.

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// siblingLocation is a location that already holds somebody else's file, next to
// where the project will land. It is the folder the bug used to refuse (VS8.2).
func siblingLocation(t *testing.T) (location, sibling string) {
	t.Helper()

	location = t.TempDir()
	sibling = filepath.Join(location, "otro.txt")
	if err := os.WriteFile(sibling, []byte("not mine\n"), 0o644); err != nil {
		t.Fatalf("seed sibling: %v", err)
	}
	return location, sibling
}

// TestTheRunWritesIntoTheProjectDirectory: a non-empty location is not a conflict,
// and the archive's top directory is renamed, so there is no extra nesting.
func TestTheRunWritesIntoTheProjectDirectory(t *testing.T) {
	fake := newFakeInitializr(t)
	pipeStdin(t)
	location, sibling := siblingLocation(t)
	project := filepath.Join(location, "proyecto1")

	var stdout, stderr bytes.Buffer
	code := run(fake.args("--output", location, "--name", "proyecto1", "--deps", "web", "--yes"), &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}

	if _, err := os.Stat(filepath.Join(project, "pom.xml")); err != nil {
		t.Errorf("proyecto1/pom.xml is missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, "demo")); !os.IsNotExist(err) {
		t.Error("proyecto1/demo exists: the archive's top directory was not renamed")
	}
	if !strings.Contains(stdout.String(), "Created "+project+"\n") {
		t.Errorf("stdout = %q, want it to say Created %s", stdout.String(), project)
	}
	if body, err := os.ReadFile(sibling); err != nil || string(body) != "not mine\n" {
		t.Errorf("the sibling was touched: body %q, err %v", body, err)
	}
}

// TestForceReplacesOnlyTheProjectDirectory is the second shipped bug: --force must
// replace the project, never the folder the user keeps their projects in.
func TestForceReplacesOnlyTheProjectDirectory(t *testing.T) {
	fake := newFakeInitializr(t)
	pipeStdin(t)
	location, sibling := siblingLocation(t)
	project := filepath.Join(location, "proyecto1")

	args := fake.args("--output", location, "--name", "proyecto1", "--deps", "web", "--yes")
	var stdout, stderr bytes.Buffer
	if code := run(args, &stdout, &stderr); code != exitOK {
		t.Fatalf("first run exit code = %d (stderr: %s)", code, stderr.String())
	}

	// A file the previous project left behind must not survive the replacement.
	stale := filepath.Join(project, "stale.txt")
	if err := os.WriteFile(stale, []byte("old"), 0o644); err != nil {
		t.Fatalf("seed stale file: %v", err)
	}

	stdout.Reset()
	stderr.Reset()
	forced := append(append([]string{}, args...), "--force")
	if code := run(forced, &stdout, &stderr); code != exitOK {
		t.Fatalf("forced run exit code = %d (stderr: %s)", code, stderr.String())
	}

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Error("stale.txt survived the replacement")
	}
	if !strings.Contains(stdout.String(), "Overwrote "+project+"\n") {
		t.Errorf("stdout = %q, want it to say Overwrote %s", stdout.String(), project)
	}
	if body, err := os.ReadFile(sibling); err != nil || string(body) != "not mine\n" {
		t.Errorf("the sibling was replaced with the project: body %q, err %v", body, err)
	}
}

// TestAnEmptyProjectNameIsRefused: an empty name would make the destination the
// location itself, and --force would then replace the folder the user works in.
func TestAnEmptyProjectNameIsRefused(t *testing.T) {
	fake := newFakeInitializr(t)
	pipeStdin(t)
	location, sibling := siblingLocation(t)

	var stdout, stderr bytes.Buffer
	code := run(fake.args("--name", "", "--output", location, "--deps", "web", "--yes"), &stdout, &stderr)

	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "project name is empty") {
		t.Errorf("stderr = %q, want it to explain the empty name", stderr.String())
	}
	if fake.generationCalls != 0 {
		t.Errorf("generation calls = %d, want none before the name is checked", fake.generationCalls)
	}
	if body, err := os.ReadFile(sibling); err != nil || string(body) != "not mine\n" {
		t.Errorf("the location was touched: body %q, err %v", body, err)
	}
	if entries, err := os.ReadDir(location); err != nil || len(entries) != 1 {
		t.Errorf("the location holds %v after a refused run, want only the seed (err %v)", entries, err)
	}
}

// TestAnArchiveWithTwoTopLevelsIsRefused: flattening it would drop or merge files,
// so the run stops with the extraction's own message and writes nothing.
func TestAnArchiveWithTwoTopLevelsIsRefused(t *testing.T) {
	fake := newFakeInitializr(t)
	pipeStdin(t)

	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for _, entry := range []struct{ name, body string }{
		{"demo/pom.xml", "<project/>\n"},
		{"other/readme.md", "# other\n"},
	} {
		header := &zip.FileHeader{Name: entry.name, Method: zip.Deflate}
		header.SetMode(0o644)
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
	fake.archive = buf.Bytes()

	location := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := run(fake.args("--output", location, "--name", "proyecto1", "--deps", "web", "--yes"), &stdout, &stderr)

	if code != exitFailure {
		t.Fatalf("exit code = %d, want %d", code, exitFailure)
	}
	if !strings.Contains(stderr.String(), "top-level entries") {
		t.Errorf("stderr = %q, want it to name the problem", stderr.String())
	}
	if strings.Contains(stdout.String(), "Created ") {
		t.Errorf("stdout = %q, want no created line after a refused extraction", stdout.String())
	}
	if entries, err := os.ReadDir(location); err != nil || len(entries) != 0 {
		t.Errorf("a refused extraction left %v behind (err %v)", entries, err)
	}
}
