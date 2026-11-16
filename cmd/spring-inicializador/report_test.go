package main

// Tests for US6.2: the run report. One plain line per pipeline step, the failing
// step named, nothing after it, and the permissions step reporting the files the
// extraction actually made executable. The fake Initializr keeps the suite offline.

import (
	"archive/zip"
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// plainArchive is a project archive whose files carry no executable bit: the server
// does not have to ship a wrapper, and the report says so instead of guessing.
func plainArchive(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for _, file := range []struct{ name, body string }{
		{"demo/pom.xml", "<project/>\n"},
		{"demo/README.md", "# demo\n"},
	} {
		header := &zip.FileHeader{Name: file.name, Method: zip.Deflate}
		header.SetMode(0o644)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatalf("create %s: %v", file.name, err)
		}
		if _, err := entry.Write([]byte(file.body)); err != nil {
			t.Fatalf("write %s: %v", file.name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}

	return buf.Bytes()
}

// outputLines is stdout split into its lines, without the trailing empty one.
func outputLines(stdout bytes.Buffer) []string {
	return strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
}

// TestRunReportsEveryStep is the success shape: the four steps in order, each ok,
// then the destination and the command that runs the project.
func TestRunReportsEveryStep(t *testing.T) {
	fake := newFakeInitializr(t)
	output := filepath.Join(t.TempDir(), "checkout")
	destination := filepath.Join(output, "demo")

	var stdout, stderr bytes.Buffer
	code := run(fake.args("--deps", "web,data-jpa", "--output", output, "--yes"), &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("run exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty on success", stderr.String())
	}

	want := []string{
		"Catalog ........ ok   (206 dependencies)",
		"Generating ..... ok   (" + size(len(fake.archive)) + ")",
		"Extracting ..... ok   (2 files)",
		"Permissions .... ok   (mvnw)",
		"Created " + destination,
		"Next: cd " + destination + " && ./mvnw spring-boot:run",
	}
	if got := outputLines(stdout); !reflect.DeepEqual(got, want) {
		t.Errorf("stdout lines =\n%v\nwant\n%v", got, want)
	}
	// One line per event: no padding, no spinner frame, no repeated line (US6.3).
	if strings.Contains(stdout.String(), "\n\n") {
		t.Errorf("stdout has a blank line; a piped run logs one line per event:\n%q", stdout.String())
	}
	assertNoANSI(t, stdout.String())
}

// TestReportNextCommandPerBuild: the next command follows the resolved build, never
// a fixed string.
func TestReportNextCommandPerBuild(t *testing.T) {
	cases := []struct {
		build   string
		command string
	}{
		{build: "maven", command: "./mvnw spring-boot:run"},
		{build: "gradle", command: "./gradlew bootRun"},
		{build: "gradle-kotlin", command: "./gradlew bootRun"},
	}

	for _, tc := range cases {
		t.Run(tc.build, func(t *testing.T) {
			fake := newFakeInitializr(t)

			var stdout, stderr bytes.Buffer
			code := run(fake.args("--build", tc.build, "--output", t.TempDir(), "--yes"), &stdout, &stderr)
			if code != exitOK {
				t.Fatalf("run exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
			}

			lines := outputLines(stdout)
			last := lines[len(lines)-1]
			if !strings.HasPrefix(last, "Next: cd ") || !strings.HasSuffix(last, "&& "+tc.command) {
				t.Errorf("last line = %q, want it to end with %q", last, "&& "+tc.command)
			}
		})
	}
}

// TestGenerationFailureStopsTheReport: a rejected request prints the failing step,
// nothing after it runs, and the server's sentence lands on stderr.
func TestGenerationFailureStopsTheReport(t *testing.T) {
	const message = "Invalid Spring Boot version '3.5.6', Spring Boot compatibility range is >=4.0.0"

	fake := newFakeInitializr(t)
	fake.status = http.StatusBadRequest
	fake.reject = `{"status":400,"message":"` + message + `"}`

	output := filepath.Join(t.TempDir(), "checkout")
	var stdout, stderr bytes.Buffer
	code := run(fake.args("--output", output, "--yes"), &stdout, &stderr)

	if code != exitFailure {
		t.Fatalf("run exit code = %d, want %d", code, exitFailure)
	}
	text := stdout.String()
	if !strings.Contains(text, "Catalog ........ ok") {
		t.Errorf("stdout = %q, want the catalog step reported", text)
	}
	if !strings.Contains(text, "Generating ..... fail") {
		t.Errorf("stdout = %q, want the generating step named as failed", text)
	}
	for _, later := range []string{"Extracting", "Permissions", "Created ", "Next: "} {
		if strings.Contains(text, later) {
			t.Errorf("stdout = %q, want nothing about %q after the failure", text, later)
		}
	}
	if !strings.Contains(stderr.String(), message) {
		t.Errorf("stderr = %q, want the server's message", stderr.String())
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Errorf("output directory %s exists, want nothing written", output)
	}
}

// TestExtractionFailureStopsTheReport: a broken archive fails its own step, and the
// permissions step never claims anything.
func TestExtractionFailureStopsTheReport(t *testing.T) {
	fake := newFakeInitializr(t)
	fake.archive = []byte("not a zip")

	output := filepath.Join(t.TempDir(), "checkout")
	var stdout, stderr bytes.Buffer
	code := run(fake.args("--output", output, "--yes"), &stdout, &stderr)

	if code != exitFailure {
		t.Fatalf("run exit code = %d, want %d", code, exitFailure)
	}
	lines := outputLines(stdout)
	want := []string{
		"Catalog ........ ok   (206 dependencies)",
		"Generating ..... ok   (" + size(len(fake.archive)) + ")",
		"Extracting ..... fail",
	}
	if !reflect.DeepEqual(lines, want) {
		t.Errorf("stdout lines =\n%v\nwant\n%v", lines, want)
	}
	if !strings.Contains(stderr.String(), "opening archive") {
		t.Errorf("stderr = %q, want the extraction failure", stderr.String())
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Errorf("output directory %s exists, want nothing written", output)
	}
}

// TestPermissionsReport: the step names the executable files that were written, or
// says there were none.
func TestPermissionsReport(t *testing.T) {
	t.Run("wrapper named", func(t *testing.T) {
		fake := newFakeInitializr(t)

		var stdout, stderr bytes.Buffer
		if code := run(fake.args("--output", t.TempDir(), "--yes"), &stdout, &stderr); code != exitOK {
			t.Fatalf("run exit code = %d (stderr: %s)", code, stderr.String())
		}
		if !strings.Contains(stdout.String(), "Permissions .... ok   (mvnw)") {
			t.Errorf("stdout = %q, want the executable named", stdout.String())
		}
	})

	t.Run("none", func(t *testing.T) {
		fake := newFakeInitializr(t)
		fake.archive = plainArchive(t)

		var stdout, stderr bytes.Buffer
		if code := run(fake.args("--output", t.TempDir(), "--yes"), &stdout, &stderr); code != exitOK {
			t.Fatalf("run exit code = %d (stderr: %s)", code, stderr.String())
		}
		if !strings.Contains(stdout.String(), "Permissions .... ok   (no executable files)") {
			t.Errorf("stdout = %q, want the step to say there was nothing to make executable", stdout.String())
		}
	})
}

// TestReportHelpers pins the small formatting rules the report depends on.
func TestReportHelpers(t *testing.T) {
	if got, want := executables([]string{"demo/gradlew", "demo/mvnw"}, "demo"), "gradlew, mvnw"; got != want {
		t.Errorf("executables = %q, want %q", got, want)
	}
	if got, want := executables([]string{"other/mvnw"}, "demo"), "other/mvnw"; got != want {
		t.Errorf("executables = %q, want %q: only the project's own directory is trimmed", got, want)
	}
	if got, want := executables(nil, "demo"), "no executable files"; got != want {
		t.Errorf("executables = %q, want %q", got, want)
	}

	for _, tc := range []struct {
		bytes int
		want  string
	}{
		{bytes: 369, want: "369 bytes"},
		{bytes: 1024, want: "1 KB"},
		{bytes: 15 * 1024, want: "15 KB"},
		{bytes: 1536, want: "2 KB"},
		{bytes: 3 * 1024 * 1024, want: "3.0 MB"},
	} {
		if got := size(tc.bytes); got != tc.want {
			t.Errorf("size(%d) = %q, want %q", tc.bytes, got, tc.want)
		}
	}

	for _, tc := range []struct {
		n    int
		want string
	}{
		{n: 0, want: "0 dependencies"},
		{n: 1, want: "1 dependency"},
		{n: 206, want: "206 dependencies"},
	} {
		if got := count(tc.n, "dependency", "dependencies"); got != tc.want {
			t.Errorf("count(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

// TestDryRunPrintsNoStepReport: a dry run prints the request instead of running the
// pipeline, so it carries no step lines at all.
func TestDryRunPrintsNoStepReport(t *testing.T) {
	fake := newFakeInitializr(t)

	var stdout, stderr bytes.Buffer
	code := run(fake.args("--dry-run", "--yes", "--output", t.TempDir()), &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("run exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}

	for _, step := range []string{stepCatalog, stepGenerating, stepExtracting, stepPermissions} {
		if strings.Contains(stdout.String(), step+" ") {
			t.Errorf("dry run printed the %s step; stdout:\n%s", step, stdout.String())
		}
	}
	if fake.generationCalls != 0 {
		t.Errorf("generation calls = %d, want none on a dry run", fake.generationCalls)
	}
}
