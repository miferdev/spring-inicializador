package main

// Tests for when a run may open the interactive form. No test starts the form:
// --yes or a stdin that is not a character device is exactly the contract being
// pinned, and both keep the suite free of a terminal.

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/miferdev/spring-inicializador/internal/tui"
)

// pipeStdin replaces os.Stdin with the read end of a pipe nobody ever writes
// to. Any attempt to prompt would block on the first key, so a run that returns
// proves the form was never opened.
func pipeStdin(t *testing.T) {
	t.Helper()

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	original := os.Stdin
	os.Stdin = reader
	t.Cleanup(func() {
		os.Stdin = original
		reader.Close()
		writer.Close()
	})
}

// runWithDeadline runs the program in the background so a prompt that blocks on
// stdin fails the test with a message instead of hanging the suite.
func runWithDeadline(t *testing.T, args []string, stdout, stderr io.Writer) int {
	t.Helper()

	done := make(chan int, 1)
	go func() {
		done <- run(args, stdout, stderr)
	}()

	select {
	case code := <-done:
		return code
	case <-time.After(10 * time.Second):
		t.Fatal("run blocked: it may be prompting on stdin")
		return -1
	}
}

// TestRunYesNeverPrompts pins the CI contract: --yes with a pipe stdin must
// complete without the form, even with no project flag at all.
func TestRunYesNeverPrompts(t *testing.T) {
	fake := newFakeInitializr(t)
	pipeStdin(t)

	output := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := runWithDeadline(t, fake.args("--output", output, "--yes"), &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("run exit code = %d, want %d; stderr: %s", code, exitOK, stderr.String())
	}
	if got := fake.generationForm.Get("type"); got != "maven-project" {
		t.Errorf("type = %q, want maven-project (D3)", got)
	}
	if !strings.Contains(stdout.String(), "Created ") {
		t.Errorf("stdout = %q, want the destination report", stdout.String())
	}
	assertNoANSI(t, stdout.String())
}

// TestRunNonTerminalStdinSkipsForm proves the form never opens when stdin is
// not a character device, so a pipeline that forgets --yes still completes
// instead of waiting for a keypress that will never arrive.
func TestRunNonTerminalStdinSkipsForm(t *testing.T) {
	fake := newFakeInitializr(t)
	pipeStdin(t)

	output := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := runWithDeadline(t, fake.args("--output", output), &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("run exit code = %d, want %d; stderr: %s", code, exitOK, stderr.String())
	}
	if got := fake.generationForm.Get("type"); got != "maven-project" {
		t.Errorf("type = %q, want the flag path with the Maven default", got)
	}
	if !strings.Contains(stdout.String(), "Created ") {
		t.Errorf("stdout = %q, want the destination report", stdout.String())
	}
	assertNoANSI(t, stdout.String())
}

// TestProvidedByFlags pins the flag-to-row mapping the form uses: every project
// flag answers exactly one row, and --deps answers none because the dependency
// pane belongs to VS3.
func TestProvidedByFlags(t *testing.T) {
	cases := []struct {
		args  []string
		key   tui.Key
		value string
	}{
		{[]string{"--group-id", "com.acme"}, tui.KeyGroupID, "com.acme"},
		{[]string{"--artifact-id", "billing"}, tui.KeyArtifactID, "billing"},
		{[]string{"--name", "billing"}, tui.KeyName, "billing"},
		{[]string{"--package-name", "org.custom"}, tui.KeyPackageName, "org.custom"},
		{[]string{"--build", "gradle"}, tui.KeyBuild, "gradle"},
		{[]string{"--language", "kotlin"}, tui.KeyLanguage, "kotlin"},
		{[]string{"--packaging", "war"}, tui.KeyPackaging, "war"},
		{[]string{"--java", "21"}, tui.KeyJavaVersion, "21"},
		{[]string{"--boot-version", "4.0.9.RELEASE"}, tui.KeyBootVersion, "4.0.9.RELEASE"},
	}

	for _, tc := range cases {
		t.Run(string(tc.key), func(t *testing.T) {
			flags, cmd := newFlagSet(io.Discard)
			if err := flags.Parse(tc.args); err != nil {
				t.Fatalf("Parse(%v): %v", tc.args, err)
			}

			provided := providedByFlags(cmd, passedFlags(flags))
			if len(provided) != 1 {
				t.Fatalf("providedByFlags(%v) = %v, want exactly one row", tc.args, provided)
			}
			if got := provided[tc.key]; got != tc.value {
				t.Errorf("provided[%q] = %q, want %q", tc.key, got, tc.value)
			}
		})
	}

	t.Run("deps answers no row", func(t *testing.T) {
		flags, cmd := newFlagSet(io.Discard)
		if err := flags.Parse([]string{"--deps", "web,data-jpa"}); err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if provided := providedByFlags(cmd, passedFlags(flags)); len(provided) != 0 {
			t.Errorf("providedByFlags(--deps) = %v, want no form row", provided)
		}
	})
}
