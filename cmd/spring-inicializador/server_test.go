package main

import (
	"bytes"
	"strings"
	"testing"
)

// A typo in --server must fail as a usage error before any request is built,
// and it must name the form that was wrong instead of surfacing a confusing
// 404 from a half-assembled endpoint.
func TestRunRejectsMalformedServer(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{"--server", "ftp://acme.test"}, &stdout, &stderr)

	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr.String(), "expected http or https") {
		t.Fatalf("stderr = %q, want it to name the offending scheme", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want nothing on a usage error", stdout.String())
	}
}

// Spaces and a trailing slash are the two things people actually paste, so they
// must normalize instead of being rejected as usage errors.
func TestRunAcceptsMessyServerURL(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{"--server", "  http://127.0.0.1:1/  "}, &stdout, &stderr)

	if code != exitFailure {
		t.Fatalf("exit code = %d, want %d for an unreachable but valid server", code, exitFailure)
	}
	if strings.Contains(stderr.String(), "expected http or https") {
		t.Fatalf("stderr = %q: a messy but valid URL was rejected", stderr.String())
	}
}
