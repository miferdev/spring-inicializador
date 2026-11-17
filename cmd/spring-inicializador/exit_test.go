package main

// Tests for US6.4: the exit codes. The wizard reports how it ended — finished, left
// on purpose, interrupted — and each maps to its own code, with nothing written on
// anything but success. The fake Initializr keeps the suite offline.

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/miferdev/spring-inicializador/internal/catalog"
	"github.com/miferdev/spring-inicializador/internal/options"
	"github.com/miferdev/spring-inicializador/internal/tui"
)

// stubOutcome makes the wizard answer with an outcome without opening a program.
func stubOutcome(t *testing.T, outcome tui.Outcome) {
	t.Helper()

	original := runForm
	runForm = func(cat *catalog.Catalog, provided tui.Values, dependencies []string, settings tui.Settings, stdin io.Reader, stdout io.Writer) (options.Options, string, tui.Outcome, error) {
		return options.Defaults(cat), stubLocation(provided), outcome, nil
	}
	t.Cleanup(func() { runForm = original })
}

// TestWizardOutcomeMapsToExitCode is the mapping this story introduces: only
// "finished" writes, "left" is a quiet success and "interrupted" is 130.
func TestWizardOutcomeMapsToExitCode(t *testing.T) {
	cases := []struct {
		outcome tui.Outcome
		code    int
	}{
		{outcome: tui.Finished, code: exitOK},
		{outcome: tui.Left, code: exitOK},
		{outcome: tui.Interrupted, code: exitInterrupted},
	}

	for _, tc := range cases {
		t.Run(tc.outcome.String(), func(t *testing.T) {
			fake := newFakeInitializr(t)
			terminalStdin(t)
			terminalStdout(t)
			stubOutcome(t, tc.outcome)

			output := filepath.Join(t.TempDir(), "checkout")
			var stdout, stderr bytes.Buffer
			code := run(fake.args("--output", output), &stdout, &stderr)

			if code != tc.code {
				t.Fatalf("exit code = %d, want %d (stderr: %s)", code, tc.code, stderr.String())
			}
			if tc.outcome == tui.Finished {
				return
			}

			// Leaving and interrupting write nothing and explain nothing. The one
			// line on stdout is the catalog step that had already happened before
			// the wizard opened (US6.2); nothing after it may appear.
			if got, want := stdout.String(), "Catalog ........ ok   (206 dependencies)\n"; got != want {
				t.Errorf("stdout = %q, want only the catalog step %q after %v", got, want, tc.outcome)
			}
			if stderr.Len() != 0 {
				t.Errorf("stderr = %q, want nothing after %v", stderr.String(), tc.outcome)
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Errorf("output directory %s exists, want nothing written", output)
			}
			if fake.generationCalls != 0 {
				t.Errorf("generation calls = %d, want none after %v", fake.generationCalls, tc.outcome)
			}
		})
	}
}

// TestCtrlCEndToEnd drives the real wizard with the ctrl+c byte: the run ends 130,
// the report stream stays empty, and nothing is written.
func TestCtrlCEndToEnd(t *testing.T) {
	fake := newFakeInitializr(t)
	terminalStdin(t)
	terminalStdout(t)

	original := runForm
	runForm = func(cat *catalog.Catalog, provided tui.Values, dependencies []string, settings tui.Settings, stdin io.Reader, stdout io.Writer) (options.Options, string, tui.Outcome, error) {
		// The wizard's own output goes to the terminal it would have; the report
		// stream below is what a script captures, and it must stay clean.
		return tui.Run(cat, provided, dependencies, settings, strings.NewReader("\x03"), io.Discard)
	}
	t.Cleanup(func() { runForm = original })

	output := filepath.Join(t.TempDir(), "checkout")
	var stdout, stderr bytes.Buffer
	code := runWithDeadline(t, fake.args("--output", output), &stdout, &stderr)

	if code != exitInterrupted {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitInterrupted, stderr.String())
	}
	// The catalog step had already happened when the wizard opened; the wizard's own
	// frame never reaches this stream, and nothing after the interruption does.
	if got, want := stdout.String(), "Catalog ........ ok   (206 dependencies)\n"; got != want {
		t.Errorf("stdout = %q, want only the catalog step %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want an interruption to stay silent", stderr.String())
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Errorf("output directory %s exists, want nothing written", output)
	}
	if fake.generationCalls != 0 {
		t.Errorf("generation calls = %d, want none after an interruption", fake.generationCalls)
	}
}

// TestExitCodeTable pins the other three codes where they already are: a rejection is
// a runtime failure, a bad value is a usage error, and a dry run succeeds.
func TestExitCodeTable(t *testing.T) {
	cases := []struct {
		name    string
		prepare func(*fakeInitializr)
		args    func(*fakeInitializr) []string
		code    int
	}{
		{
			name: "success",
			args: func(f *fakeInitializr) []string {
				return f.args("--deps", "web", "--output", t.TempDir(), "--yes")
			},
			code: exitOK,
		},
		{
			name: "server rejection",
			prepare: func(f *fakeInitializr) {
				f.status = 400
				f.reject = `{"status":400,"message":"no"}`
			},
			args: func(f *fakeInitializr) []string {
				return f.args("--output", t.TempDir(), "--yes")
			},
			code: exitFailure,
		},
		{
			name: "dry run",
			args: func(f *fakeInitializr) []string {
				return f.args("--dry-run", "--yes")
			},
			code: exitOK,
		},
		{
			name: "bad flag value",
			args: func(*fakeInitializr) []string {
				return []string{"--build", "nope"}
			},
			code: exitUsage,
		},
		{
			name: "unknown flag",
			args: func(*fakeInitializr) []string {
				return []string{"--nope"}
			},
			code: exitUsage,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeInitializr(t)
			pipeStdin(t)
			if tc.prepare != nil {
				tc.prepare(fake)
			}

			var stdout, stderr bytes.Buffer
			if code := run(tc.args(fake), &stdout, &stderr); code != tc.code {
				t.Fatalf("exit code = %d, want %d (stderr: %s)", code, tc.code, stderr.String())
			}
		})
	}
}

// TestHelpListsEveryExitCode is the documentation half: the table a script author
// reads has to carry all four codes.
func TestHelpListsEveryExitCode(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--help"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}

	for _, want := range []string{
		"0 success",
		"1 server or filesystem failure",
		"2 usage error",
		"130 interrupted",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("help text does not document %q", want)
		}
	}
	assertNoANSI(t, stdout.String())
}
