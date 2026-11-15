package main

// End-to-end tests for the headless pipeline. Both upstream endpoints are faked
// with httptest servers, so the suite passes with networking disabled and never
// touches start.spring.io.

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixtureBytes(t *testing.T) []byte {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", "internal", "catalog", "testdata", "metadata-client.json"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	return data
}

// starterArchive builds a minimal but complete project archive with the same
// top-level layout the real server produces.
func starterArchive(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)

	dirHeader := &zip.FileHeader{Name: "demo/", Method: zip.Store}
	dirHeader.SetMode(os.ModeDir | 0o755)
	if _, err := writer.CreateHeader(dirHeader); err != nil {
		t.Fatalf("create directory entry: %v", err)
	}

	files := []struct {
		name string
		body string
		mode os.FileMode
	}{
		{"demo/pom.xml", "<project/>\n", 0o644},
		{"demo/mvnw", "#!/bin/sh\n", 0o755},
	}
	for _, file := range files {
		header := &zip.FileHeader{Name: file.name, Method: zip.Deflate}
		header.SetMode(file.mode)
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

// fakeInitializr serves the metadata fixture and a crafted project archive from
// one origin, like the real service, and records the generation request.
type fakeInitializr struct {
	baseURL string

	metadata []byte
	archive  []byte
	status   int
	reject   string

	generationMethod string
	generationType   string
	generationForm   url.Values

	// metadataCalls and generationCalls count the requests each endpoint
	// actually received, so a failing run can prove it never reached one.
	metadataCalls   int
	generationCalls int
}

func newFakeInitializr(t *testing.T) *fakeInitializr {
	t.Helper()

	fake := &fakeInitializr{
		metadata: fixtureBytes(t),
		archive:  starterArchive(t),
		status:   http.StatusOK,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/metadata/client", func(w http.ResponseWriter, r *http.Request) {
		fake.metadataCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fake.metadata)
	})
	mux.HandleFunc("/starter.zip", func(w http.ResponseWriter, r *http.Request) {
		fake.generationCalls++
		fake.generationMethod = r.Method
		fake.generationType = r.Header.Get("Content-Type")
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		fake.generationForm = r.PostForm
		if fake.status != http.StatusOK {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(fake.status)
			_, _ = io.WriteString(w, fake.reject)
			return
		}
		_, _ = w.Write(fake.archive)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	fake.baseURL = server.URL
	return fake
}

// args returns the flag list every end-to-end run starts from.
func (f *fakeInitializr) args(extra ...string) []string {
	return append([]string{"--server", f.baseURL}, extra...)
}

func assertNoANSI(t *testing.T, output string) {
	t.Helper()
	if strings.ContainsRune(output, '\x1b') {
		t.Errorf("output contains an ANSI escape: %q", output)
	}
}

func TestRunGeneratesProject(t *testing.T) {
	fake := newFakeInitializr(t)
	output := filepath.Join(t.TempDir(), "checkout")

	var stdout, stderr bytes.Buffer
	code := run(fake.args(
		"--deps", "web,data-jpa",
		"--output", output,
		"--yes",
	), &stdout, &stderr)

	if code != 0 {
		t.Fatalf("run exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty on success", stderr.String())
	}

	for _, name := range []string{"pom.xml", "mvnw"} {
		if _, err := os.Stat(filepath.Join(output, "demo", name)); err != nil {
			t.Errorf("generated file %s: %v", name, err)
		}
	}

	// The report names the destination and the next step, in ASCII only.
	text := stdout.String()
	destination := filepath.Join(output, "demo")
	if !strings.Contains(text, "Created "+destination) {
		t.Errorf("stdout does not name the destination: %q", text)
	}
	if !strings.Contains(text, "cd "+destination+" && ./mvnw spring-boot:run") {
		t.Errorf("stdout does not name the next command: %q", text)
	}
	assertNoANSI(t, text)

	// The request that crossed the wire used the documented contract.
	if fake.generationMethod != http.MethodPost {
		t.Errorf("generation method = %q, want POST", fake.generationMethod)
	}
	if fake.generationType != "application/x-www-form-urlencoded" {
		t.Errorf("generation Content-Type = %q", fake.generationType)
	}
	if got := fake.generationForm.Get("type"); got != "maven-project" {
		t.Errorf("type = %q, want maven-project (D3)", got)
	}
	if got := fake.generationForm.Get("dependencies"); got != "web,data-jpa" {
		t.Errorf("dependencies = %q, want %q", got, "web,data-jpa")
	}
	if got := fake.generationForm.Get("baseDir"); got != "demo" {
		t.Errorf("baseDir = %q, want demo", got)
	}
	if got := fake.generationForm.Get("javaVersion"); got != "17" {
		t.Errorf("javaVersion = %q, want 17", got)
	}
}

func TestRunOverridesIdentifiers(t *testing.T) {
	fake := newFakeInitializr(t)

	var stdout, stderr bytes.Buffer
	code := run(fake.args(
		"--group-id", "com.acme",
		"--artifact-id", "billing",
		"--output", t.TempDir(),
		"--yes",
	), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run exit code = %d, want 0; stderr: %s", code, stderr.String())
	}

	form := fake.generationForm
	if got := form.Get("groupId"); got != "com.acme" {
		t.Errorf("groupId = %q, want com.acme", got)
	}
	if got := form.Get("artifactId"); got != "billing" {
		t.Errorf("artifactId = %q, want billing", got)
	}
	if got := form.Get("baseDir"); got != "billing" {
		t.Errorf("baseDir = %q, want billing", got)
	}
	if got := form.Get("name"); got != "billing" {
		t.Errorf("name = %q, want billing (derived from artifact id)", got)
	}
	if got := form.Get("packageName"); got != "com.acme.billing" {
		t.Errorf("packageName = %q, want com.acme.billing", got)
	}
}

func TestRunServerRejection(t *testing.T) {
	const message = "Invalid Spring Boot version '3.5.6', Spring Boot compatibility range is >=4.0.0"

	fake := newFakeInitializr(t)
	fake.status = http.StatusBadRequest
	fake.reject = `{"status":400,"message":"` + message + `"}`

	var stdout, stderr bytes.Buffer
	code := run(fake.args(
		"--output", t.TempDir(),
		"--yes",
	), &stdout, &stderr)

	if code != 1 {
		t.Errorf("run exit code = %d, want 1 for a server rejection", code)
	}
	// The server's sentence, not the JSON envelope around it.
	if !strings.Contains(stderr.String(), message) {
		t.Errorf("stderr %q does not carry the server message", stderr.String())
	}
	if strings.Contains(stderr.String(), `"status":400`) {
		t.Errorf("stderr %q carries the JSON envelope instead of the message", stderr.String())
	}
	assertNoANSI(t, stderr.String())
}

// TestReportErrorExitCodes pins the failure mapping: a wrapped cancellation is
// an interruption (130), anything else is a runtime failure (1).
func TestReportErrorExitCodes(t *testing.T) {
	t.Run("interrupted", func(t *testing.T) {
		var stderr bytes.Buffer
		err := fmt.Errorf("render: generating project: %w", context.Canceled)
		if code := reportError(&stderr, err); code != exitInterrupted {
			t.Errorf("exit code = %d, want %d", code, exitInterrupted)
		}
		if !strings.Contains(stderr.String(), "interrupted") {
			t.Errorf("stderr = %q, want an interruption notice", stderr.String())
		}
	})

	t.Run("failure", func(t *testing.T) {
		var stderr bytes.Buffer
		if code := reportError(&stderr, errors.New("boom")); code != exitFailure {
			t.Errorf("exit code = %d, want %d", code, exitFailure)
		}
		if !strings.Contains(stderr.String(), "boom") {
			t.Errorf("stderr = %q, want the error text", stderr.String())
		}
	})
}

func TestRunUsageErrors(t *testing.T) {
	t.Run("unknown flag", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		if code := run([]string{"--nope"}, &stdout, &stderr); code != 2 {
			t.Errorf("run exit code = %d, want 2", code)
		}
		if stderr.Len() == 0 {
			t.Error("stderr is empty for a usage error")
		}
	})

	t.Run("positional argument", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		if code := run([]string{"unexpected"}, &stdout, &stderr); code != 2 {
			t.Errorf("run exit code = %d, want 2", code)
		}
	})
}

func TestRunVersionAndHelp(t *testing.T) {
	t.Run("version", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		if code := run([]string{"--version"}, &stdout, &stderr); code != 0 {
			t.Fatalf("run exit code = %d, want 0", code)
		}
		if !strings.Contains(stdout.String(), "spring-inicializador "+version) {
			t.Errorf("stdout = %q, want the version line", stdout.String())
		}
		assertNoANSI(t, stdout.String())
	})

	t.Run("help", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		if code := run([]string{"--help"}, &stdout, &stderr); code != 0 {
			t.Fatalf("run exit code = %d, want 0", code)
		}
		for _, flag := range []string{
			"--deps", "--output", "--server", "--group-id", "--artifact-id", "--yes",
			"--name", "--package-name", "--build", "--language", "--packaging",
			"--java", "--boot-version",
		} {
			if !strings.Contains(stdout.String(), flag) {
				t.Errorf("help text does not document %s", flag)
			}
		}
		assertNoANSI(t, stdout.String())
	})
}

// TestRunNeverReadsStdin proves the CI promise: stdin is connected to a pipe
// that is never written, so any read would block until the deadline.
func TestRunNeverReadsStdin(t *testing.T) {
	fake := newFakeInitializr(t)

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

	done := make(chan int, 1)
	go func() {
		done <- run(fake.args(
			"--deps", "web",
			"--output", t.TempDir(),
			"--yes",
		), io.Discard, io.Discard)
	}()

	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("run exit code = %d, want 0", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("run blocked: it may be reading stdin")
	}
}
