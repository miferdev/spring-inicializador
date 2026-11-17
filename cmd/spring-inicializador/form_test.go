package main

// Tests for when a run may open the interactive form. No test starts the form:
// --yes or a stdin that is not a character device is exactly the contract being
// pinned, and both keep the suite free of a terminal.

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
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

// TestTheNameAndLocationReachTheRequestAndTheRun is the flag half of VS8.1: --name
// (or its alias --artifact-id) is the project's single name, and --output is the
// parent directory the run writes into.
func TestTheNameAndLocationReachTheRequestAndTheRun(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "name", args: []string{"--name", "billing"}},
		{name: "artifact-id alias", args: []string{"--artifact-id", "billing"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeInitializr(t)
			pipeStdin(t)
			output := t.TempDir()

			args := append([]string{"--deps", "web", "--output", output, "--yes"}, tc.args...)
			var stdout, stderr bytes.Buffer
			if code := run(fake.args(args...), &stdout, &stderr); code != exitOK {
				t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
			}

			form := fake.generationForm
			for field, want := range map[string]string{
				"artifactId": "billing",
				"name":       "billing",
				"baseDir":    "billing",
			} {
				if got := form.Get(field); got != want {
					t.Errorf("%s = %q, want %q: the name is the single source", field, got, want)
				}
			}

			// --output is the location and the name is the project's folder: the
			// archive's own top directory is renamed to it, so there is no extra
			// nesting (VS8.2).
			if _, err := os.Stat(filepath.Join(output, "billing", "pom.xml")); err != nil {
				t.Errorf("the project was not written into %s: %v", filepath.Join(output, "billing"), err)
			}
			if want := "Created " + filepath.Join(output, "billing"); !strings.Contains(stdout.String(), want) {
				t.Errorf("stdout = %q, want it to name %q", stdout.String(), want)
			}
		})
	}
}

// TestProvidedByFlags pins the flag-to-row mapping the form uses: every project
// flag answers exactly one row, and --deps answers none because the dependency
// pane belongs to VS3. --artifact-id is an alias of --name (VS8), so the table
// names its row by hand.
func TestProvidedByFlags(t *testing.T) {
	cases := []struct {
		name  string
		args  []string
		key   tui.Key
		value string
	}{
		{name: "group-id", args: []string{"--group-id", "com.acme"}, key: tui.KeyGroupID, value: "com.acme"},
		{name: "artifact-id", args: []string{"--artifact-id", "billing"}, key: tui.KeyName, value: "billing"},
		{name: "name", args: []string{"--name", "billing"}, key: tui.KeyName, value: "billing"},
		{name: "output", args: []string{"--output", "/tmp/out"}, key: tui.KeyLocation, value: "/tmp/out"},
		{name: "package-name", args: []string{"--package-name", "org.custom"}, key: tui.KeyPackageName, value: "org.custom"},
		{name: "build", args: []string{"--build", "gradle"}, key: tui.KeyBuild, value: "gradle"},
		{name: "language", args: []string{"--language", "kotlin"}, key: tui.KeyLanguage, value: "kotlin"},
		{name: "packaging", args: []string{"--packaging", "war"}, key: tui.KeyPackaging, value: "war"},
		{name: "java", args: []string{"--java", "21"}, key: tui.KeyJavaVersion, value: "21"},
		{name: "boot-version", args: []string{"--boot-version", "4.0.9.RELEASE"}, key: tui.KeyBootVersion, value: "4.0.9.RELEASE"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
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

	t.Run("name wins over artifact-id", func(t *testing.T) {
		flags, cmd := newFlagSet(io.Discard)
		if err := flags.Parse([]string{"--artifact-id", "ident", "--name", "human"}); err != nil {
			t.Fatalf("Parse: %v", err)
		}

		provided := providedByFlags(cmd, passedFlags(flags))
		if got := provided[tui.KeyName]; got != "human" {
			t.Errorf("provided[name] = %q, want the --name value human", got)
		}
	})

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
