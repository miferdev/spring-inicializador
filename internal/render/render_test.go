package render_test

// Render is exercised through httptest: the integration point is an HTTP
// request, and this test asserts exactly what crosses it.

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/miferdev/spring-inicializador/internal/options"
	"github.com/miferdev/spring-inicializador/internal/render"
)

func testOptions() options.Options {
	return options.Options{
		GroupID:     "com.example",
		ArtifactID:  "demo",
		Name:        "demo",
		Description: "Demo project for Spring Boot",
		PackageName: "com.example.demo",
		Build:       options.BuildMaven,
		Language:    "java",
		Packaging:   "jar",
		JavaVersion: "17",
		BootVersion: "4.1.2.RELEASE",
		Deps:        []string{"web", "data-jpa"},
	}
}

func TestRenderPostsForm(t *testing.T) {
	archive := []byte("PK\x03\x04 not really a zip")

	var (
		method      string
		path        string
		contentType string
		form        url.Values
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		contentType = r.Header.Get("Content-Type")
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		form = r.PostForm
		_, _ = w.Write(archive)
	}))
	defer server.Close()

	got, err := render.Render(context.Background(), server.Client(), server.URL, testOptions())
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if string(got) != string(archive) {
		t.Errorf("Render body = %q, want %q", got, archive)
	}

	if method != http.MethodPost {
		t.Errorf("method = %q, want POST", method)
	}
	if path != "/starter.zip" {
		t.Errorf("path = %q, want %q", path, "/starter.zip")
	}
	wantContentType := "application/x-www-form-urlencoded"
	if contentType != wantContentType {
		t.Errorf("Content-Type = %q, want %q", contentType, wantContentType)
	}

	want := url.Values{
		"type":         {"maven-project"},
		"language":     {"java"},
		"bootVersion":  {"4.1.2.RELEASE"},
		"baseDir":      {"demo"},
		"groupId":      {"com.example"},
		"artifactId":   {"demo"},
		"name":         {"demo"},
		"description":  {"Demo project for Spring Boot"},
		"packageName":  {"com.example.demo"},
		"packaging":    {"jar"},
		"javaVersion":  {"17"},
		"dependencies": {"web,data-jpa"},
	}
	if !reflect.DeepEqual(form, want) {
		t.Errorf("form = %v, want %v", form, want)
	}
}

// TestRenderRejectionCarriesBody proves a non-200 response surfaces the
// server's own text, so US1.4 can print it verbatim without changing the flow.
func TestRenderRejectionCarriesBody(t *testing.T) {
	const body = `{"timestamp":"2026-11-14T00:00:00Z","status":400,"error":"Bad Request",` +
		`"message":"Invalid Spring Boot version '3.5.6', Spring Boot compatibility range is >=4.0.0",` +
		`"path":"/starter.zip"}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, body)
	}))
	defer server.Close()

	_, err := render.Render(context.Background(), server.Client(), server.URL, testOptions())
	if err == nil {
		t.Fatal("Render returned no error for a 400 response")
	}
	if !strings.Contains(err.Error(), body) {
		t.Errorf("error %q does not carry the response body", err)
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error %q does not name the status", err)
	}
}
