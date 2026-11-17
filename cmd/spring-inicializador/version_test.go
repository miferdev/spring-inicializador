package main

// Tests for VS9.1: --java and --boot-version are the server's vocabulary, so the
// client sends them as given and repeats whatever the server answers. --build,
// --language and --packaging stay ours and are still checked before any request.
// The fake Initializr keeps the suite offline.

import (
	"bytes"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

// TestTheVersionIsSentAsGiven is the story's core: a version the catalog does not
// publish is still the user's call, and the request carries exactly what was typed.
func TestTheVersionIsSentAsGiven(t *testing.T) {
	fake := newFakeInitializr(t)
	pipeStdin(t)
	output := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := run(fake.args("--boot-version", "4.1.2", "--java", "21", "--deps", "web", "--output", output, "--yes"), &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}
	if got := fake.generationForm.Get("bootVersion"); got != "4.1.2" {
		t.Errorf("bootVersion = %q, want the value as given, 4.1.2", got)
	}
	if got := fake.generationForm.Get("javaVersion"); got != "21" {
		t.Errorf("javaVersion = %q, want 21", got)
	}
	if !strings.Contains(stdout.String(), "Created "+filepath.Join(output, "demo")) {
		t.Errorf("stdout = %q, want the project to be written", stdout.String())
	}
}

// TestARejectedVersionPrintsTheServerMessage: a value the server refuses is the
// server's answer, printed verbatim, and a failure like any other.
func TestARejectedVersionPrintsTheServerMessage(t *testing.T) {
	const rejection = `{"timestamp":"2026-11-17T00:00:00Z","status":400,"error":"Bad Request","message":"Invalid Spring Boot version '9.9.9', Spring Boot compatibility range is >=4.0.0","path":"/starter.zip"}`

	cases := []struct {
		name string
		args []string
	}{
		{name: "boot version", args: []string{"--boot-version", "9.9.9"}},
		{name: "java version", args: []string{"--java", "99"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeInitializr(t)
			fake.status = http.StatusBadRequest
			fake.reject = rejection
			pipeStdin(t)

			args := append(tc.args, "--deps", "web", "--output", t.TempDir(), "--yes")
			var stdout, stderr bytes.Buffer
			code := run(fake.args(args...), &stdout, &stderr)

			if code != exitFailure {
				t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitFailure, stderr.String())
			}
			if got, want := strings.TrimSpace(stderr.String()), "spring-inicializador: Invalid Spring Boot version '9.9.9', Spring Boot compatibility range is >=4.0.0"; got != want {
				t.Errorf("stderr = %q, want the server's message %q", got, want)
			}
			if strings.Contains(stderr.String(), "not published") {
				t.Errorf("stderr = %q, want no local list check", stderr.String())
			}
			if fake.generationCalls != 1 {
				t.Errorf("generation calls = %d, want the request to have been made", fake.generationCalls)
			}
			if strings.Contains(stdout.String(), "Created ") {
				t.Errorf("stdout = %q, want nothing created", stdout.String())
			}
		})
	}
}

// TestTheToolsOwnVocabularyIsStillCheckedLocally: these three are this tool's own
// words, so a typo still costs nothing.
func TestTheToolsOwnVocabularyIsStillCheckedLocally(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "build", args: []string{"--build", "nope"}},
		{name: "language", args: []string{"--language", "nope"}},
		{name: "packaging", args: []string{"--packaging", "nope"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeInitializr(t)
			pipeStdin(t)

			var stdout, stderr bytes.Buffer
			code := run(fake.args(tc.args...), &stdout, &stderr)

			if code != exitUsage {
				t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitUsage, stderr.String())
			}
			if !strings.Contains(stderr.String(), "is not valid; accepted:") {
				t.Errorf("stderr = %q, want the local vocabulary message", stderr.String())
			}
			if fake.metadataCalls != 0 || fake.generationCalls != 0 {
				t.Errorf("requests = %d metadata, %d generation; a usage error must not reach the server",
					fake.metadataCalls, fake.generationCalls)
			}
		})
	}
}

// TestAnEmptyVersionIsAUsageError: an empty value would silently mean "whatever the
// server defaults to", which is not what an empty flag says.
func TestAnEmptyVersionIsAUsageError(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "boot version", args: []string{"--boot-version="}},
		{name: "java version", args: []string{"--java="}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeInitializr(t)
			pipeStdin(t)

			var stdout, stderr bytes.Buffer
			code := run(fake.args(tc.args...), &stdout, &stderr)

			if code != exitUsage {
				t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitUsage, stderr.String())
			}
			if !strings.Contains(stderr.String(), "empty") {
				t.Errorf("stderr = %q, want it to explain the empty value", stderr.String())
			}
			if fake.metadataCalls != 0 || fake.generationCalls != 0 {
				t.Errorf("requests = %d metadata, %d generation; a usage error must not reach the server",
					fake.metadataCalls, fake.generationCalls)
			}
		})
	}
}

// TestDryRunPrintsTheVersionAsGiven: a dry run resolves the request without asking
// the server whether it likes the version.
func TestDryRunPrintsTheVersionAsGiven(t *testing.T) {
	fake := newFakeInitializr(t)
	pipeStdin(t)

	var stdout, stderr bytes.Buffer
	code := run(fake.args("--dry-run", "--boot-version", "4.1.2", "--java", "21"), &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}

	values := parseDryRun(t, stdout.String())
	for key, want := range map[string]string{"boot-version": "4.1.2", "java": "21"} {
		if got := values[key]; got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	if fake.generationCalls != 0 {
		t.Errorf("generation calls = %d, want none on a dry run", fake.generationCalls)
	}
}
