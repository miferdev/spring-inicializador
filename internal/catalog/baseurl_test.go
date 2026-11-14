package catalog_test

import (
	"strings"
	"testing"

	"github.com/miferdev/spring-inicializador/internal/catalog"
)

func TestNormalizeBaseURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr string
	}{
		{name: "bare host assumes https", input: "start.spring.io", want: "https://start.spring.io"},
		{name: "host with port", input: "localhost:8080", want: "https://localhost:8080"},
		{name: "scheme and host", input: "https://start.spring.io", want: "https://start.spring.io"},
		{name: "plain http is kept", input: "http://127.0.0.1:18080", want: "http://127.0.0.1:18080"},
		{name: "trailing slash dropped", input: "https://start.spring.io/", want: "https://start.spring.io"},
		{name: "repeated trailing slashes dropped", input: "https://start.spring.io///", want: "https://start.spring.io"},
		{name: "path prefix kept", input: "https://acme.test/initializr/", want: "https://acme.test/initializr"},
		{name: "surrounding spaces trimmed", input: "  https://acme.test  ", want: "https://acme.test"},

		{name: "empty", input: "", wantErr: "empty"},
		{name: "blank", input: "   ", wantErr: "empty"},
		{name: "wrong scheme", input: "ftp://acme.test", wantErr: "expected http or https"},
		{name: "missing host", input: "https://", wantErr: "no host"},
		{name: "query refused", input: "https://acme.test/?a=1", wantErr: "query or fragment"},
		{name: "credentials refused", input: "https://user:pw@acme.test", wantErr: "credentials"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := catalog.NormalizeBaseURL(test.input)

			if test.wantErr != "" {
				if err == nil {
					t.Fatalf("NormalizeBaseURL(%q) = %q, want an error", test.input, got)
				}
				if !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("error = %q, want it to mention %q", err, test.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("NormalizeBaseURL(%q) returned error: %v", test.input, err)
			}
			if got != test.want {
				t.Fatalf("NormalizeBaseURL(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}
