package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/miferdev/spring-inicializador/internal/catalog"
	"github.com/miferdev/spring-inicializador/internal/options"
	"github.com/miferdev/spring-inicializador/internal/tui"
)

// Redirecting stdin is how every script and CI job says "nobody is here". It
// must not open the form even without --yes: /dev/null is a character device,
// so a device check is not an isatty check, and getting this wrong turns a
// pipeline into a hang or a bubbletea error.
func TestRedirectedStdinNeverOpensTheForm(t *testing.T) {
	fake := newFakeInitializr(t)

	file, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open %q: %v", os.DevNull, err)
	}
	originalStdin := os.Stdin
	os.Stdin = file
	t.Cleanup(func() {
		os.Stdin = originalStdin
		file.Close()
	})

	formCalls := 0
	originalForm := runForm
	runForm = func(cat *catalog.Catalog, provided tui.Values, dependencies []string, stdin io.Reader, stdout io.Writer) (options.Options, bool, error) {
		formCalls++
		return tui.Run(cat, provided, dependencies, stdin, stdout)
	}
	t.Cleanup(func() { runForm = originalForm })

	var stdout, stderr bytes.Buffer
	code := run(fake.args("--output", t.TempDir(), "--deps", "web"), &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}
	if formCalls != 0 {
		t.Fatalf("the run opened the form %d times with stdin redirected", formCalls)
	}
	if !strings.Contains(stdout.String(), "Created ") {
		t.Fatalf("stdout = %q, want the project to be generated", stdout.String())
	}
}
