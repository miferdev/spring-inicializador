package catalog_test

// Fetch is exercised through httptest only: the metadata endpoint is a third
// party, so the suite must pass with networking disabled.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/miferdev/spring-inicializador/internal/catalog"
)

func TestFetch(t *testing.T) {
	fixture := fixtureRead(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/metadata/client" {
			t.Errorf("path = %q, want %q", r.URL.Path, "/metadata/client")
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(fixture); err != nil {
			t.Errorf("write fixture: %v", err)
		}
	}))
	defer server.Close()

	// Normalization belongs to the caller, not to Fetch: mirror production by
	// normalizing first, then assert the request path is not doubled.
	base, err := catalog.NormalizeBaseURL(server.URL + "/")
	if err != nil {
		t.Fatalf("NormalizeBaseURL: %v", err)
	}

	got, err := catalog.Fetch(context.Background(), server.Client(), base)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(got.Categories) != fixtureExpectedCategories {
		t.Errorf("categories = %d, want %d", len(got.Categories), fixtureExpectedCategories)
	}
	if len(got.Dependencies) != fixtureExpectedDependencies {
		t.Errorf("dependencies = %d, want %d", len(got.Dependencies), fixtureExpectedDependencies)
	}
}

func TestFetchNon200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream is down", http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := catalog.Fetch(context.Background(), server.Client(), server.URL)
	if err == nil {
		t.Fatal("Fetch returned no error for a 500 response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error %q does not name the status", err)
	}
}

func TestFetchInvalidBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer server.Close()

	_, err := catalog.Fetch(context.Background(), server.Client(), server.URL)
	if err == nil {
		t.Fatal("Fetch returned no error for an unparsable body")
	}
}
