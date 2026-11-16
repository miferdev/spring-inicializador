package main

import (
	"bytes"
	"io"
	"net/http"
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
	runForm = func(cat *catalog.Catalog, provided tui.Values, dependencies []string, settings tui.Settings, destination string, stdin io.Reader, stdout io.Writer) (options.Options, bool, error) {
		formCalls++
		return tui.Run(cat, provided, dependencies, settings, destination, stdin, stdout)
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

// assertPlainText is the non-interactive contract: a log may be grepped and shown in
// a CI console, so neither stream may carry a terminal control character — no
// escape, no carriage return, no bell, no tab. Newlines are the only control byte a
// line-oriented report needs (US6.3).
func assertPlainText(t *testing.T, streams ...string) {
	t.Helper()

	for _, stream := range streams {
		for _, r := range stream {
			if r == '\n' {
				continue
			}
			if r < 0x20 || r == 0x7f {
				t.Errorf("output carries the control character %q:\n%q", r, stream)
			}
		}
	}
}

// TestNonInteractiveOutputIsPlainText walks every path a pipe can take and holds it
// to the same contract: no escape byte, no carriage return, no other control
// sequence, on stdout or stderr.
func TestNonInteractiveOutputIsPlainText(t *testing.T) {
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
			name: "generation rejection",
			prepare: func(f *fakeInitializr) {
				f.status = http.StatusBadRequest
				f.reject = `{"status":400,"message":"Invalid Spring Boot version"}`
			},
			args: func(f *fakeInitializr) []string {
				return f.args("--output", t.TempDir(), "--yes")
			},
			code: exitFailure,
		},
		{
			name:    "extraction failure",
			prepare: func(f *fakeInitializr) { f.archive = []byte("not a zip") },
			args: func(f *fakeInitializr) []string {
				return f.args("--output", t.TempDir(), "--yes")
			},
			code: exitFailure,
		},
		{
			name: "dry run",
			args: func(f *fakeInitializr) []string {
				return f.args("--dry-run", "--yes", "--output", t.TempDir())
			},
			code: exitOK,
		},
		{
			name: "version",
			args: func(*fakeInitializr) []string { return []string{"--version"} },
			code: exitOK,
		},
		{
			name: "help",
			args: func(*fakeInitializr) []string { return []string{"--help"} },
			code: exitOK,
		},
		{
			name: "usage error",
			args: func(*fakeInitializr) []string { return []string{"--nope"} },
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
			code := run(tc.args(fake), &stdout, &stderr)
			if code != tc.code {
				t.Fatalf("run exit code = %d, want %d (stderr: %s)", code, tc.code, stderr.String())
			}
			assertPlainText(t, stdout.String(), stderr.String())
		})
	}
}

// TestPipedStdoutNeverPaintsTheWizard is the fix this story makes: a terminal on
// stdin is not enough when the report is going into a pipe. The wizard clears the
// screen and moves the cursor, which would corrupt the log it was piped into, so it
// must not open.
func TestPipedStdoutNeverPaintsTheWizard(t *testing.T) {
	fake := newFakeInitializr(t)
	terminalStdin(t) // os.Stdin looks like a terminal

	formCalls := 0
	original := runForm
	runForm = func(cat *catalog.Catalog, provided tui.Values, dependencies []string, settings tui.Settings, destination string, stdin io.Reader, stdout io.Writer) (options.Options, bool, error) {
		formCalls++
		return tui.Run(cat, provided, dependencies, settings, destination, stdin, stdout)
	}
	t.Cleanup(func() { runForm = original })

	// A buffer is not a terminal, so this run is a piped one.
	var stdout, stderr bytes.Buffer
	code := run(fake.args("--output", t.TempDir(), "--deps", "web"), &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}
	if formCalls != 0 {
		t.Fatalf("the run opened the wizard %d times with stdout piped", formCalls)
	}
	assertPlainText(t, stdout.String(), stderr.String())
	if !strings.Contains(stdout.String(), "Created ") {
		t.Fatalf("stdout = %q, want the project to be generated", stdout.String())
	}
}

// TestPipedRunWithoutFlagsUsesTheModelDefaults covers the CI case the tool exists
// for: nothing on the command line but the server, no --yes, nobody at the terminal.
// The run must apply the model defaults, generate, and never wait for a keypress.
func TestPipedRunWithoutFlagsUsesTheModelDefaults(t *testing.T) {
	fake := newFakeInitializr(t)
	pipeStdin(t)

	output := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := runWithDeadline(t, fake.args("--output", output), &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Created ") {
		t.Errorf("stdout = %q, want the project to be generated", stdout.String())
	}

	// The metadata defaults crossed the wire, exactly as --dry-run would print them.
	form := fake.generationForm
	for field, want := range map[string]string{
		"type":        "maven-project",
		"groupId":     "com.example",
		"artifactId":  "demo",
		"javaVersion": "17",
	} {
		if got := form.Get(field); got != want {
			t.Errorf("%s = %q, want the model default %q", field, got, want)
		}
	}
	assertPlainText(t, stdout.String(), stderr.String())
}
