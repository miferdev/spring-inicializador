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

// TestMayPrompt pins the flag half of the gate: a generation flag answers part
// of the form, so the run must not prompt for the rest.
func TestMayPrompt(t *testing.T) {
	cases := []struct {
		name string
		args []string
		yes  bool
		want bool
	}{
		{"no arguments", nil, false, true},
		{"--yes", []string{"--yes"}, true, false},
		{"--group-id", []string{"--group-id", "com.acme"}, false, false},
		{"--artifact-id", []string{"--artifact-id", "billing"}, false, false},
		{"--deps", []string{"--deps", "web"}, false, false},
		{"--output", []string{"--output", "out"}, false, true},
		{"--force", []string{"--force"}, false, true},
		{"--server", []string{"--server", "http://127.0.0.1:1"}, false, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			flags, _ := newFlagSet(io.Discard)
			if err := flags.Parse(tc.args); err != nil {
				t.Fatalf("Parse(%v): %v", tc.args, err)
			}
			if got := mayPrompt(passedFlags(flags), tc.yes); got != tc.want {
				t.Errorf("mayPrompt(%v, %v) = %v, want %v", tc.args, tc.yes, got, tc.want)
			}
		})
	}
}
