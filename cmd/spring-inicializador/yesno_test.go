package main

// Tests for US6.5: --yes and --no mean what they say. --yes takes every default
// without asking and never destroys; --no declines the one destructive action this
// tool has — replacing a non-empty output directory — even when --force asks for
// it. The two together are a usage error, not whichever flag was read last.

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

// seedOutput makes the location look like somebody's working folder with a
// finished earlier run in it: a sibling file next to the project directory, and a
// project directory named after the default project. The conflict is measured on
// the project directory, never on the location (VS8.2).
func seedOutput(t *testing.T) (location, project, marker string) {
	t.Helper()

	location = t.TempDir()
	if err := os.WriteFile(filepath.Join(location, "otro.txt"), []byte("saved\n"), 0o644); err != nil {
		t.Fatalf("seeding %s: %v", location, err)
	}

	project = filepath.Join(location, "demo")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatalf("seeding %s: %v", project, err)
	}
	marker = filepath.Join(project, "keep.txt")
	if err := os.WriteFile(marker, []byte("untouched\n"), 0o644); err != nil {
		t.Fatalf("seeding %s: %v", marker, err)
	}
	return location, project, marker
}

// assertUntouched proves a refused run wrote nothing: the marker still holds the
// bytes the seed wrote, and the directory gained no entry.
func assertUntouched(t *testing.T, dir, marker string) {
	t.Helper()

	contents, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("the refused run removed %s: %v", marker, err)
	}
	if string(contents) != "untouched\n" {
		t.Errorf("%s = %q, want the bytes the seed wrote", marker, contents)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	if len(entries) != 1 {
		t.Errorf("%s holds %d entries after a refused run, want the 1 it started with", dir, len(entries))
	}
}

// helpLine returns the help text line documenting flag, matched by its exact name
// so that "--no" is not satisfied by the "--no-color" line.
func helpLine(help, flag string) string {
	for _, line := range strings.Split(help, "\n") {
		if strings.HasPrefix(line, "  "+flag+" ") {
			return line
		}
	}
	return ""
}

// TestYesAndNoCannotBeCombined: the two flags answer every question in opposite
// directions, so the run refuses to guess (US6.5).
func TestYesAndNoCannotBeCombined(t *testing.T) {
	fake := newFakeInitializr(t)
	pipeStdin(t)
	location, project, marker := seedOutput(t)

	var stdout, stderr bytes.Buffer
	code := run(fake.args("--output", location, "--yes", "--no"), &stdout, &stderr)

	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitUsage, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want nothing: a usage error explains itself on stderr", stdout.String())
	}
	for _, want := range []string{"--yes and --no cannot be combined", "Usage:"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("stderr = %q, want it to mention %q", stderr.String(), want)
		}
	}
	if fake.metadataCalls != 0 || fake.generationCalls != 0 {
		t.Errorf("requests = %d metadata, %d generation; a usage error must not reach the server",
			fake.metadataCalls, fake.generationCalls)
	}
	assertUntouched(t, project, marker)
}

// TestYesAloneRefusesToReplace: --yes accepts defaults, it does not grant
// permission to destroy. The refusal names the flag that would grant it.
func TestYesAloneRefusesToReplace(t *testing.T) {
	fake := newFakeInitializr(t)
	pipeStdin(t)
	location, project, marker := seedOutput(t)

	var stdout, stderr bytes.Buffer
	code := run(fake.args("--output", location, "--yes"), &stdout, &stderr)

	if code != exitFailure {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitFailure, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--force") {
		t.Errorf("stderr = %q, want it to name --force", stderr.String())
	}
	if strings.Contains(stdout.String(), "Created ") {
		t.Errorf("stdout = %q, want no created line after a refusal", stdout.String())
	}
	assertUntouched(t, project, marker)
}

// TestNoRefusesToReplaceBeforeAnyRequest: --no declines the destructive action on
// the project's own directory. A name on the command line makes the destination
// known before the first request; a name that only the metadata provides costs the
// metadata request, and the refusal still comes before generation (VS8.2).
func TestNoRefusesToReplaceBeforeAnyRequest(t *testing.T) {
	cases := []struct {
		name            string
		args            []string
		metadataAllowed int
	}{
		{name: "named on the command line", args: []string{"--name", "demo"}},
		{name: "named by the metadata", args: nil, metadataAllowed: 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeInitializr(t)
			pipeStdin(t)
			location, project, marker := seedOutput(t)

			args := append([]string{"--output", location, "--no"}, tc.args...)
			var stdout, stderr bytes.Buffer
			code := run(fake.args(args...), &stdout, &stderr)

			if code != exitFailure {
				t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitFailure, stderr.String())
			}
			for _, want := range []string{"--no", "--force"} {
				if !strings.Contains(stderr.String(), want) {
					t.Errorf("stderr = %q, want it to name %q", stderr.String(), want)
				}
			}
			if fake.generationCalls != 0 {
				t.Errorf("generation calls = %d, want none: a refusal must not spend one", fake.generationCalls)
			}
			if fake.metadataCalls > tc.metadataAllowed {
				t.Errorf("metadata calls = %d, want at most %d", fake.metadataCalls, tc.metadataAllowed)
			}
			if tc.metadataAllowed == 0 && stdout.Len() != 0 {
				t.Errorf("stdout = %q, want nothing before the first step", stdout.String())
			}
			assertUntouched(t, project, marker)
		})
	}
}

// TestNoOverridesForce pins the rule for the contradictory pair: --force grants the
// destructive action and --no declines it, and declining wins, because replacing a
// project cannot be undone while a missed generation can be re-run. The refusal is
// loud (exit 1 naming --no), so a script that wants the replacement drops --no.
func TestNoOverridesForce(t *testing.T) {
	fake := newFakeInitializr(t)
	pipeStdin(t)
	location, project, marker := seedOutput(t)

	var stdout, stderr bytes.Buffer
	code := run(fake.args("--output", location, "--no", "--force"), &stdout, &stderr)

	if code != exitFailure {
		t.Fatalf("exit code = %d, want %d: --no must override --force (stderr: %s)", code, exitFailure, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--no") {
		t.Errorf("stderr = %q, want it to name the flag that refused", stderr.String())
	}
	if fake.generationCalls != 0 {
		t.Errorf("generation calls = %d, want none: the refusal precedes the request", fake.generationCalls)
	}
	assertUntouched(t, project, marker)
}

// TestNoGeneratesWhenNothingIsRefused covers the harmless cases: an empty
// location, a missing one and one that already holds somebody else's files all have
// nothing to decline, so the run is an ordinary one, still plain (US6.3, VS8.2).
func TestNoGeneratesWhenNothingIsRefused(t *testing.T) {
	cases := []struct {
		name    string
		prepare func(t *testing.T) string
	}{
		{
			name: "existing empty directory",
			prepare: func(t *testing.T) string {
				return t.TempDir()
			},
		},
		{
			name: "missing directory",
			prepare: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "checkout")
			},
		},
		{
			name: "location full of other work",
			prepare: func(t *testing.T) string {
				location := t.TempDir()
				if err := os.WriteFile(filepath.Join(location, "otro.txt"), []byte("not mine\n"), 0o644); err != nil {
					t.Fatalf("seed location: %v", err)
				}
				return location
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeInitializr(t)
			pipeStdin(t)
			output := tc.prepare(t)

			var stdout, stderr bytes.Buffer
			code := run(fake.args("--output", output, "--no", "--deps", "web"), &stdout, &stderr)

			if code != exitOK {
				t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
			}
			if stderr.Len() != 0 {
				t.Errorf("stderr = %q, want empty on success", stderr.String())
			}
			if !strings.Contains(stdout.String(), "Created ") {
				t.Fatalf("stdout = %q, want the project to be generated", stdout.String())
			}
			if _, err := os.Stat(filepath.Join(output, "demo", "pom.xml")); err != nil {
				t.Errorf("the project was not written: %v", err)
			}
			assertPlainText(t, stdout.String(), stderr.String())
		})
	}
}

// TestNoNeverOpensTheWizard: a flag that answers every prompt with "no" cannot let
// the form ask something, so a terminal run with --no stays scripted.
func TestNoNeverOpensTheWizard(t *testing.T) {
	fake := newFakeInitializr(t)
	terminalStdin(t)
	terminalStdout(t)

	formCalls := 0
	original := runForm
	runForm = func(cat *catalog.Catalog, provided tui.Values, dependencies []string, settings tui.Settings, stdin io.Reader, stdout io.Writer) (options.Options, string, tui.Outcome, error) {
		formCalls++
		return tui.Run(cat, provided, dependencies, settings, stdin, stdout)
	}
	t.Cleanup(func() { runForm = original })

	output := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := run(fake.args("--output", output, "--no"), &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}
	if formCalls != 0 {
		t.Errorf("the run opened the wizard %d times with --no", formCalls)
	}
}

// TestNoStillAllowsADryRun: a dry run writes nothing, so an occupied destination is
// nothing for --no to decline, and the resolved options are still printed.
func TestNoStillAllowsADryRun(t *testing.T) {
	fake := newFakeInitializr(t)
	pipeStdin(t)
	location, project, marker := seedOutput(t)

	var stdout, stderr bytes.Buffer
	code := run(fake.args("--dry-run", "--no", "--output", location), &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "artifact-id=") {
		t.Errorf("stdout = %q, want the resolved options", stdout.String())
	}
	assertUntouched(t, project, marker)
}

// TestHelpDocumentsYesAndNo is the documentation half: both flags have their own
// line, and --no is not merely the prefix of --no-color.
func TestHelpDocumentsYesAndNo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--help"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}

	for _, flag := range []string{"--yes", "--no"} {
		if line := helpLine(stdout.String(), flag); line == "" {
			t.Errorf("help does not document %s", flag)
		}
	}
}
