package main

// Tests for VS9.2: the version the request carries is the one Maven Central
// resolves, and a server that really publishes the legacy suffix still works through
// one retry. The fake Initializr keeps the suite offline, and it records every
// generation request so a test can see each attempt.

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// bootLine is the report line that names the version the request carried.
func bootLine(version string) string {
	return "Boot ........... ok   (" + version + ")"
}

// TestTheDefaultVersionIsRequestedPlain: the catalog publishes 4.1.2.RELEASE, Maven
// Central has 4.1.2, and the story's whole point is that the request carries the
// second one.
func TestTheDefaultVersionIsRequestedPlain(t *testing.T) {
	fake := newFakeInitializr(t)
	pipeStdin(t)
	output := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := run(fake.args("--deps", "web,h2", "--output", output, "--yes"), &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}

	if got := fake.generationForm.Get("bootVersion"); got != "4.1.2" {
		t.Errorf("bootVersion = %q, want the plain 4.1.2", got)
	}
	if !strings.Contains(stdout.String(), bootLine("4.1.2")+"\n") {
		t.Errorf("stdout = %q, want it to name the requested version", stdout.String())
	}
	if strings.Contains(stdout.String(), "4.1.2.RELEASE") {
		t.Errorf("stdout = %q, want no legacy suffix on a run that did not retry", stdout.String())
	}
}

// TestATypedLegacyVersionIsRequestedPlain: one rule, whether the value came from the
// catalog or from the command line.
func TestATypedLegacyVersionIsRequestedPlain(t *testing.T) {
	cases := []struct {
		name    string
		typed   string
		request string
	}{
		{name: "legacy suffix", typed: "2.3.12.RELEASE", request: "2.3.12"},
		{name: "already plain", typed: "4.1.2", request: "4.1.2"},
		{name: "other qualifier untouched", typed: "4.2.0.RC2", request: "4.2.0.RC2"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeInitializr(t)
			pipeStdin(t)

			var stdout, stderr bytes.Buffer
			code := run(fake.args("--boot-version", tc.typed, "--output", t.TempDir(), "--yes"), &stdout, &stderr)
			if code != exitOK {
				t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
			}

			if got := fake.generationForm.Get("bootVersion"); got != tc.request {
				t.Errorf("bootVersion = %q, want %q", got, tc.request)
			}
			if !strings.Contains(stdout.String(), bootLine(tc.request)+"\n") {
				t.Errorf("stdout = %q, want it to name %q", stdout.String(), tc.request)
			}
			if fake.generationCalls != 1 {
				t.Errorf("generation calls = %d, want one: the server accepted the request", fake.generationCalls)
			}
		})
	}
}

// TestARetriedGenerationSucceedsWithThePublishedVersion: a server that really
// publishes .RELEASE artifacts (Boot 2.x, self-hosted) refuses the plain form and
// accepts the published one, and the run still generates the project.
func TestARetriedGenerationSucceedsWithThePublishedVersion(t *testing.T) {
	fake := newFakeInitializr(t)
	fake.rejectVersions = map[string]string{
		"4.1.2": "Invalid Spring Boot version '4.1.2', accepted: 4.1.2.RELEASE",
	}
	pipeStdin(t)
	output := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := run(fake.args("--deps", "web", "--output", output, "--yes"), &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}

	// Two attempts, in order, and the second one carried the published id.
	if got, want := len(fake.generationForms), 2; got != want {
		t.Fatalf("generation requests = %d, want %d (the plain form, then the published one)", got, want)
	}
	if got := fake.generationForms[0].Get("bootVersion"); got != "4.1.2" {
		t.Errorf("first request bootVersion = %q, want 4.1.2", got)
	}
	if got := fake.generationForms[1].Get("bootVersion"); got != "4.1.2.RELEASE" {
		t.Errorf("second request bootVersion = %q, want the published 4.1.2.RELEASE", got)
	}

	report := stdout.String()
	if !strings.Contains(report, bootLine("4.1.2")+"\n") {
		t.Errorf("stdout = %q, want the first attempt's version", report)
	}
	if !strings.Contains(report, "Generating ..... fail\n") {
		t.Errorf("stdout = %q, want the rejected attempt reported", report)
	}
	if !strings.Contains(report, "Boot ........... retry (published 4.1.2.RELEASE)\n") {
		t.Errorf("stdout = %q, want the retry line naming the published version", report)
	}
	if !strings.Contains(report, "Generating ..... ok") {
		t.Errorf("stdout = %q, want the retry to have succeeded", report)
	}
	// The pom comes from the retry: the project is on disk.
	if _, err := os.Stat(filepath.Join(output, "demo", "pom.xml")); err != nil {
		t.Errorf("the project was not written: %v", err)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want nothing: the retry succeeded", stderr.String())
	}
}

// TestBothFormsRejectedPrintsThePublishedMessage: the retry is the last word, so the
// message is the one the server gave for the published value.
func TestBothFormsRejectedPrintsThePublishedMessage(t *testing.T) {
	const publishedRejection = "Invalid Spring Boot version '4.1.2.RELEASE', Spring Boot compatibility range is >=3.0.0"

	fake := newFakeInitializr(t)
	fake.rejectVersions = map[string]string{
		"4.1.2":         "Invalid Spring Boot version '4.1.2'",
		"4.1.2.RELEASE": publishedRejection,
	}
	pipeStdin(t)
	location := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := run(fake.args("--deps", "web", "--output", location, "--yes"), &stdout, &stderr)

	if code != exitFailure {
		t.Fatalf("exit code = %d, want %d", code, exitFailure)
	}
	if got, want := strings.TrimSpace(stderr.String()), "spring-inicializador: "+publishedRejection; got != want {
		t.Errorf("stderr = %q, want the published value's message %q", got, want)
	}
	if got := len(fake.generationForms); got != 2 {
		t.Errorf("generation requests = %d, want exactly two", got)
	}
	if entries, err := os.ReadDir(location); err != nil || len(entries) != 0 {
		t.Errorf("a refused run wrote %v (err %v)", entries, err)
	}
}

// TestAFailedConnectionIsNotRetried: a network failure is not a rejected version, so
// it costs one attempt, not two.
func TestAFailedConnectionIsNotRetried(t *testing.T) {
	fake := newFakeInitializr(t)
	fake.dropGeneration = true
	pipeStdin(t)

	var stdout, stderr bytes.Buffer
	code := run(fake.args("--output", t.TempDir(), "--yes"), &stdout, &stderr)

	if code != exitFailure {
		t.Fatalf("exit code = %d, want %d", code, exitFailure)
	}

	// The server side finishes after the client gives up, so wait for it before
	// reading the counters.
	select {
	case <-fake.droppedGeneration:
	case <-time.After(10 * time.Second):
		t.Fatal("the server never dropped the connection")
	}
	if fake.generationCalls != 1 {
		t.Errorf("generation calls = %d, want exactly one", fake.generationCalls)
	}
	if strings.Contains(stdout.String(), "retry") {
		t.Errorf("stdout = %q, want no retry line for a network failure", stdout.String())
	}
	if !strings.Contains(stderr.String(), "spring-inicializador:") {
		t.Errorf("stderr = %q, want the failure explained", stderr.String())
	}
}

// TestAServerFailureIsNotRetried: a 5xx is the service failing, not refusing the
// version.
func TestAServerFailureIsNotRetried(t *testing.T) {
	fake := newFakeInitializr(t)
	fake.status = http.StatusInternalServerError
	fake.reject = `{"status":500,"message":"Internal Server Error"}`
	pipeStdin(t)

	var stdout, stderr bytes.Buffer
	code := run(fake.args("--output", t.TempDir(), "--yes"), &stdout, &stderr)

	if code != exitFailure {
		t.Fatalf("exit code = %d, want %d", code, exitFailure)
	}
	if fake.generationCalls != 1 {
		t.Errorf("generation calls = %d, want exactly one", fake.generationCalls)
	}
	if !strings.Contains(stderr.String(), "Internal Server Error") {
		t.Errorf("stderr = %q, want the server's message", stderr.String())
	}
}

// TestATypedVersionIsNotDowngraded: a value the user typed is theirs; a rejection is
// the server's answer, not a reason to ask for a different version.
func TestATypedVersionIsNotDowngraded(t *testing.T) {
	fake := newFakeInitializr(t)
	fake.rejectVersions = map[string]string{"9.9.9": "Invalid Spring Boot version '9.9.9'"}
	pipeStdin(t)

	var stdout, stderr bytes.Buffer
	code := run(fake.args("--boot-version", "9.9.9", "--output", t.TempDir(), "--yes"), &stdout, &stderr)

	if code != exitFailure {
		t.Fatalf("exit code = %d, want %d", code, exitFailure)
	}
	if fake.generationCalls != 1 {
		t.Errorf("generation calls = %d, want exactly one: the version was never rewritten", fake.generationCalls)
	}
	if !strings.Contains(stderr.String(), "Invalid Spring Boot version '9.9.9'") {
		t.Errorf("stderr = %q, want the server's message for the typed value", stderr.String())
	}
}
