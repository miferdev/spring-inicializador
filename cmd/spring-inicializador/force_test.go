package main

import (
	"bytes"
	"strings"
	"testing"
)

// Running twice into the same output must fail the second time and say which
// flag overrides the refusal, then succeed with it and report the overwrite.
func TestRunRefusesToOverwriteWithoutForce(t *testing.T) {
	fake := newFakeInitializr(t)
	output := t.TempDir()

	var stdout, stderr bytes.Buffer
	if code := run(fake.args("--output", output, "--yes"), &stdout, &stderr); code != exitOK {
		t.Fatalf("first run exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code := run(fake.args("--output", output, "--yes"), &stdout, &stderr)
	if code != exitFailure {
		t.Fatalf("second run exit code = %d, want %d", code, exitFailure)
	}
	if !strings.Contains(stderr.String(), "--force") {
		t.Fatalf("stderr = %q, want it to name --force", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run(fake.args("--output", output, "--yes", "--force"), &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("forced run exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}
	// The step report precedes it since US6.2, so the line is looked for, not
	// prefixed to, the output.
	if !strings.Contains(stdout.String(), "Overwrote ") {
		t.Fatalf("stdout = %q, want it to say the project was overwritten", stdout.String())
	}
}
