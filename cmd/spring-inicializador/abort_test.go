package main

// The caller contract for an aborted form: esc on the first row means "nothing
// to do", so the run succeeds without writing anything or requesting an archive.

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

func TestAbortedFormWritesNothing(t *testing.T) {
	fake := newFakeInitializr(t)

	// Pretend a person is at the terminal: the real check needs an isatty, and
	// the form itself is driven below with the esc byte a terminal would send.
	terminalStdin(t)
	terminalStdout(t)

	formCalls := 0
	originalForm := runForm
	runForm = func(cat *catalog.Catalog, provided tui.Values, dependencies []string, settings tui.Settings, stdin io.Reader, stdout io.Writer) (options.Options, string, tui.Outcome, error) {
		formCalls++
		// The real form, driven with a scripted esc instead of a terminal.
		return tui.Run(cat, provided, dependencies, settings, strings.NewReader("\x1b"), io.Discard)
	}
	t.Cleanup(func() { runForm = originalForm })

	output := filepath.Join(t.TempDir(), "project")
	var stdout, stderr bytes.Buffer
	code := runWithDeadline(t, fake.args("--output", output, "--deps", "web"), &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}
	if formCalls != 1 {
		t.Fatalf("the form was opened %d times, want 1", formCalls)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want nothing after an abort", stderr.String())
	}
	if strings.Contains(stdout.String(), "Created ") {
		t.Errorf("stdout = %q, want no project report after an abort", stdout.String())
	}
	if fake.generationCalls != 0 {
		t.Errorf("generation calls = %d, want 0", fake.generationCalls)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Errorf("output directory %s exists, want nothing written", output)
	}
}
