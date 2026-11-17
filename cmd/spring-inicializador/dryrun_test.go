package main

// Tests for US2.4: --dry-run resolves everything a real run would, prints it in
// a fixed order, and never generates or writes. The golden file is the contract:
// two identical invocations must produce identical bytes.

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// updateGolden rewrites the golden file instead of comparing against it:
//
//	go test ./cmd/spring-inicializador -run TestDryRunGolden -update
var updateGolden = flag.Bool("update", false, "rewrite the golden files")

// dryRunArgs is the fixed input the dry-run tests share: two identifiers and a
// dependency list, so the output exercises flags, derived values and the
// catalog defaults at once.
func dryRunArgs(fake *fakeInitializr, extra ...string) []string {
	args := []string{
		"--dry-run",
		"--group-id", "com.acme",
		"--artifact-id", "billing",
		"--deps", "web,postgresql",
	}
	return fake.args(append(args, extra...)...)
}

// runDryRun executes the program with a deadline, so a dry run that wrongly
// opened the form would fail with a clear message instead of hanging the suite.
func runDryRun(t *testing.T, args []string) (stdout, stderr string, code int) {
	t.Helper()

	var out, errOut bytes.Buffer
	code = runWithDeadline(t, args, &out, &errOut)
	return out.String(), errOut.String(), code
}

// parseDryRun turns the report into a map, failing on a malformed or duplicated
// line so a broken format cannot pass the value assertions.
func parseDryRun(t *testing.T, output string) map[string]string {
	t.Helper()

	values := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSuffix(output, "\n"), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok || key == "" {
			t.Fatalf("dry run line %q is not key=value", line)
		}
		if _, duplicate := values[key]; duplicate {
			t.Fatalf("dry run prints %q twice", key)
		}
		values[key] = value
	}
	return values
}

// TestDryRunGolden pins the exact bytes a known input prints. Without -update it
// fails on any difference.
func TestDryRunGolden(t *testing.T) {
	fake := newFakeInitializr(t)
	pipeStdin(t)

	stdout, stderr, code := runDryRun(t, dryRunArgs(fake))
	if code != exitOK {
		t.Fatalf("run exit code = %d, want %d; stderr: %s", code, exitOK, stderr)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty on success", stderr)
	}

	golden := filepath.Join("testdata", "dryrun.golden")
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
			t.Fatalf("creating %s: %v", filepath.Dir(golden), err)
		}
		if err := os.WriteFile(golden, []byte(stdout), 0o644); err != nil {
			t.Fatalf("writing %s: %v", golden, err)
		}
		return
	}

	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("reading %s: %v (run the test with -update to create it)", golden, err)
	}
	if stdout != string(want) {
		t.Errorf("dry run output differs from %s\n--- got ---\n%s--- want ---\n%s", golden, stdout, want)
	}
}

// TestDryRunIsRepeatable compares two invocations in code: the report may not
// carry a timestamp, a port, a path or anything else that moves.
func TestDryRunIsRepeatable(t *testing.T) {
	fake := newFakeInitializr(t)
	pipeStdin(t)

	first, _, code := runDryRun(t, dryRunArgs(fake))
	if code != exitOK {
		t.Fatalf("first run exit code = %d, want %d", code, exitOK)
	}
	second, _, code := runDryRun(t, dryRunArgs(fake))
	if code != exitOK {
		t.Fatalf("second run exit code = %d, want %d", code, exitOK)
	}

	if first != second {
		t.Errorf("two identical invocations differ:\n--- first ---\n%s--- second ---\n%s", first, second)
	}
}

// TestDryRunOutputIsPlainText pins the shape: no escape byte, no tab, one
// key=value line per field, so nothing depends on the terminal width.
func TestDryRunOutputIsPlainText(t *testing.T) {
	fake := newFakeInitializr(t)
	pipeStdin(t)

	stdout, _, code := runDryRun(t, dryRunArgs(fake))
	if code != exitOK {
		t.Fatalf("run exit code = %d, want %d", code, exitOK)
	}

	if strings.ContainsRune(stdout, '\x1b') {
		t.Error("output contains an ANSI escape")
	}
	if strings.ContainsRune(stdout, '\t') {
		t.Error("output contains a tab")
	}
	if !strings.HasSuffix(stdout, "\n") {
		t.Error("output does not end with a newline")
	}

	lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
	for _, line := range lines {
		if _, _, ok := strings.Cut(line, "="); !ok {
			t.Errorf("line %q is not key=value", line)
		}
	}
}

// TestDryRunNeverGeneratesOrWrites proves the run stops after resolution: the
// catalog is fetched, /starter.zip is not, and the output directory is not
// created.
func TestDryRunNeverGeneratesOrWrites(t *testing.T) {
	fake := newFakeInitializr(t)
	pipeStdin(t)

	output := filepath.Join(t.TempDir(), "project")
	stdout, stderr, code := runDryRun(t, dryRunArgs(fake, "--output", output))
	if code != exitOK {
		t.Fatalf("run exit code = %d, want %d; stderr: %s", code, exitOK, stderr)
	}
	if stdout == "" {
		t.Error("stdout is empty, want the resolved options")
	}

	if fake.metadataCalls != 1 {
		t.Errorf("metadata calls = %d, want 1: the values come from the catalog", fake.metadataCalls)
	}
	if fake.generationCalls != 0 {
		t.Errorf("generation calls = %d, want 0 for a dry run", fake.generationCalls)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Errorf("output directory %s exists, want it untouched", output)
	}
}

// TestDryRunValues checks that the report carries flags, defaults and catalog
// values, in the documented key set.
func TestDryRunValues(t *testing.T) {
	fake := newFakeInitializr(t)
	pipeStdin(t)

	stdout, stderr, code := runDryRun(t, dryRunArgs(fake))
	if code != exitOK {
		t.Fatalf("run exit code = %d, want %d; stderr: %s", code, exitOK, stderr)
	}
	values := parseDryRun(t, stdout)

	want := map[string]string{
		"group-id":     "com.acme",         // flag
		"artifact-id":  "billing",          // flag
		"name":         "billing",          // follows the artifact id
		"package-name": "com.acme.billing", // derived from the identifiers
		"build":        "maven",            // decision D3
		"language":     "java",             // catalog default
		"packaging":    "jar",              // catalog default
		"java":         "17",               // catalog default
		"boot-version": "4.1.2",            // catalog default, without the legacy .RELEASE
		"deps":         "web,postgresql",   // flag
	}
	if len(values) != len(want) {
		t.Errorf("dry run prints %d keys, want %d: %v", len(values), len(want), values)
	}
	for key, expected := range want {
		if got := values[key]; got != expected {
			t.Errorf("%s = %q, want %q", key, got, expected)
		}
	}

	t.Run("flags override the defaults", func(t *testing.T) {
		fake := newFakeInitializr(t)
		pipeStdin(t)

		stdout, _, code := runDryRun(t, fake.args(
			"--yes", "--dry-run",
			"--name", "custom",
			"--package-name", "org.custom",
			"--build", "gradle",
			"--language", "kotlin",
			"--packaging", "war",
			"--java", "21",
			"--boot-version", "4.0.9.RELEASE",
		))
		if code != exitOK {
			t.Fatalf("run exit code = %d, want %d", code, exitOK)
		}
		values := parseDryRun(t, stdout)

		for key, expected := range map[string]string{
			"name":         "custom",
			"package-name": "org.custom",
			"build":        "gradle",
			"language":     "kotlin",
			"packaging":    "war",
			"java":         "21",
			"boot-version": "4.0.9",
		} {
			if got := values[key]; got != expected {
				t.Errorf("%s = %q, want %q", key, got, expected)
			}
		}
	})
}

// TestDryRunPrintsUnpublishedVersionsAsGiven pins that a dry run does not second
// guess the server: a version the catalog does not list is printed, and nothing is
// requested from the generation endpoint (VS9.1).
func TestDryRunPrintsUnpublishedVersionsAsGiven(t *testing.T) {
	fake := newFakeInitializr(t)
	pipeStdin(t)

	stdout, stderr, code := runDryRun(t, fake.args("--dry-run", "--java", "99", "--boot-version", "9.9.9"))
	if code != exitOK {
		t.Fatalf("run exit code = %d, want %d; stderr: %s", code, exitOK, stderr)
	}

	values := parseDryRun(t, stdout)
	if got := values["java"]; got != "99" {
		t.Errorf("java = %q, want the value as given, 99", got)
	}
	if got := values["boot-version"]; got != "9.9.9" {
		t.Errorf("boot-version = %q, want the value as given, 9.9.9", got)
	}
	if fake.generationCalls != 0 {
		t.Errorf("generation calls = %d, want 0", fake.generationCalls)
	}
}
