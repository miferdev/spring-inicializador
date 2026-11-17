package main

// Tests for US2.3: the project flags answer exactly what the form would, an
// invalid value fails before any work the server would reject, and a run whose
// flags answered every row never opens the form.

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

// TestFlagsReachTheRequest pins each new flag to the request field it fills,
// using the fake Initializr that already backs the end-to-end suite.
func TestFlagsReachTheRequest(t *testing.T) {
	cases := []struct {
		name  string
		args  []string
		field string
		want  string
	}{
		{"name", []string{"--name", "billing"}, "name", "billing"},
		{"package-name", []string{"--package-name", "org.custom.billing"}, "packageName", "org.custom.billing"},
		{"build maven", []string{"--build", "maven"}, "type", "maven-project"},
		{"build gradle", []string{"--build", "gradle"}, "type", "gradle-project"},
		{"build gradle-kotlin", []string{"--build", "gradle-kotlin"}, "type", "gradle-project-kotlin"},
		{"language", []string{"--language", "kotlin"}, "language", "kotlin"},
		{"packaging", []string{"--packaging", "war"}, "packaging", "war"},
		{"java", []string{"--java", "21"}, "javaVersion", "21"},
		{"boot-version", []string{"--boot-version", "4.0.9.RELEASE"}, "bootVersion", "4.0.9.RELEASE"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeInitializr(t)

			var stdout, stderr bytes.Buffer
			args := append(tc.args, "--yes", "--output", t.TempDir())
			if code := run(fake.args(args...), &stdout, &stderr); code != exitOK {
				t.Fatalf("run exit code = %d, want %d; stderr: %s", code, exitOK, stderr.String())
			}
			if got := fake.generationForm.Get(tc.field); got != tc.want {
				t.Errorf("%s = %q, want %q", tc.field, got, tc.want)
			}
		})
	}
}

// TestVocabularyFlagsFailBeforeAnyRequest covers the flags whose values are this
// tool's own vocabulary: a typo is a usage error and never reaches the server.
func TestVocabularyFlagsFailBeforeAnyRequest(t *testing.T) {
	cases := []struct {
		flag     string
		value    string
		accepted string
	}{
		{"build", "nope", "maven, gradle, gradle-kotlin"},
		{"language", "nope", "java, kotlin, groovy"},
		{"packaging", "nope", "jar, war"},
	}

	for _, tc := range cases {
		t.Run(tc.flag, func(t *testing.T) {
			fake := newFakeInitializr(t)

			var stdout, stderr bytes.Buffer
			code := run(fake.args("--"+tc.flag, tc.value), &stdout, &stderr)

			if code != exitUsage {
				t.Fatalf("run exit code = %d, want %d; stderr: %s", code, exitUsage, stderr.String())
			}
			if !strings.Contains(stderr.String(), tc.accepted) {
				t.Errorf("stderr = %q, want the accepted values %q", stderr.String(), tc.accepted)
			}
			if !strings.Contains(stderr.String(), tc.value) {
				t.Errorf("stderr = %q, want the rejected value %q", stderr.String(), tc.value)
			}
			if fake.metadataCalls != 0 || fake.generationCalls != 0 {
				t.Errorf("server received %d metadata and %d generation calls, want none",
					fake.metadataCalls, fake.generationCalls)
			}
		})
	}
}

// TestVersionFlagsReachTheServer covers the flags only the server can vouch for:
// whatever value they carry goes into the request unchanged, and the server's
// answer is the verdict. VS9.1 removed the local list check that used to reject
// `--java 99` here with the catalog's own values.
func TestVersionFlagsReachTheServer(t *testing.T) {
	cases := []struct {
		flag  string
		value string
		field string
	}{
		{"java", "99", "javaVersion"},
		{"boot-version", "9.9.9", "bootVersion"},
	}

	for _, tc := range cases {
		t.Run(tc.flag, func(t *testing.T) {
			fake := newFakeInitializr(t)
			pipeStdin(t)

			var stdout, stderr bytes.Buffer
			code := run(fake.args("--"+tc.flag, tc.value, "--output", t.TempDir(), "--yes"), &stdout, &stderr)

			if code != exitOK {
				t.Fatalf("run exit code = %d, want %d; stderr: %s", code, exitOK, stderr.String())
			}
			if got := fake.generationForm.Get(tc.field); got != tc.value {
				t.Errorf("%s = %q, want the value as given, %q", tc.field, got, tc.value)
			}
			if fake.metadataCalls != 1 || fake.generationCalls != 1 {
				t.Errorf("requests = %d metadata, %d generation; want the run to reach generation",
					fake.metadataCalls, fake.generationCalls)
			}
		})
	}
}

// terminalStdin makes a run believe stdin is a terminal, without needing one.
// The real check is isatty: /dev/null is a character device but not a terminal,
// so a test that swapped os.Stdin for it would silently exercise the scripted
// path instead of the interactive one.
// terminalStdout makes a run believe its report is going to a terminal, so the
// wizard opens on the paths where it should. Use it beside terminalStdin.
func terminalStdout(t *testing.T) {
	t.Helper()

	original := interactiveStdout
	interactiveStdout = func(io.Writer) bool { return true }
	t.Cleanup(func() { interactiveStdout = original })
}

// stubLocation is the location a stubbed wizard reports: the row --output answered,
// or the default the form would have started from. A stub that returned "" would
// send the extraction into the current directory.
func stubLocation(provided tui.Values) string {
	if location, ok := provided[tui.KeyLocation]; ok {
		return location
	}
	return "."
}

func terminalStdin(t *testing.T) {
	t.Helper()

	original := interactiveStdin
	interactiveStdin = func(*os.File) bool { return true }
	t.Cleanup(func() { interactiveStdin = original })
}

// TestAllProjectFlagsNeverOpenTheForm pins the "nothing left to ask" rule: with
// every project field on the command line, the run goes straight to generation
// even though stdin looks like a terminal.
func TestAllProjectFlagsNeverOpenTheForm(t *testing.T) {
	terminalStdin(t)

	formCalls := 0
	original := runForm
	runForm = func(cat *catalog.Catalog, provided tui.Values, dependencies []string, settings tui.Settings, stdin io.Reader, stdout io.Writer) (options.Options, string, tui.Outcome, error) {
		formCalls++
		return tui.Run(cat, provided, dependencies, settings, stdin, stdout)
	}
	t.Cleanup(func() { runForm = original })

	fake := newFakeInitializr(t)
	output := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := runWithDeadline(t, fake.args(
		"--group-id", "com.acme",
		"--artifact-id", "billing",
		"--name", "billing",
		"--package-name", "com.acme.billing",
		"--build", "gradle",
		"--language", "kotlin",
		"--packaging", "war",
		"--java", "21",
		"--boot-version", "4.0.9.RELEASE",
		"--output", output,
	), &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("run exit code = %d, want %d; stderr: %s", code, exitOK, stderr.String())
	}
	if formCalls != 0 {
		t.Errorf("the run opened the form %d times, want none with every field provided", formCalls)
	}

	form := fake.generationForm
	for field, want := range map[string]string{
		"groupId":     "com.acme",
		"artifactId":  "billing",
		"name":        "billing",
		"packageName": "com.acme.billing",
		"type":        "gradle-project",
		"language":    "kotlin",
		"packaging":   "war",
		"javaVersion": "21",
		"bootVersion": "4.0.9.RELEASE",
	} {
		if got := form.Get(field); got != want {
			t.Errorf("%s = %q, want %q", field, got, want)
		}
	}
	assertNoANSI(t, stdout.String())
}
