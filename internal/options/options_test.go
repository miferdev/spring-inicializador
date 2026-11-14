package options_test

import (
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/miferdev/spring-inicializador/internal/catalog"
	"github.com/miferdev/spring-inicializador/internal/options"
)

// fixtureCatalog parses the checked-in metadata capture. The test performs
// local file I/O only; there is no network access.
func fixtureCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "catalog", "testdata", "metadata-client.json"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	c, err := catalog.Parse(data)
	if err != nil {
		t.Fatalf("catalog.Parse(fixture): %v", err)
	}
	return c
}

// TestDefaultsFromFixture pins every default this project takes from the
// metadata document, including decision D3: Maven wins over the server's
// `gradle-project` default while the flag and the future form agree.
func TestDefaultsFromFixture(t *testing.T) {
	got := options.Defaults(fixtureCatalog(t))

	want := options.Options{
		GroupID:     "com.example",
		ArtifactID:  "demo",
		Name:        "demo", // metadata has no name default; derived from artifactId
		Description: "",
		PackageName: "com.example.demo",
		Build:       "maven",
		Language:    "java",
		Packaging:   "jar",
		JavaVersion: "17",
		BootVersion: "4.1.2.RELEASE",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Defaults(fixture) = %+v, want %+v", got, want)
	}

	// D3 is about the wire value, not the flag value.
	if got.BuildType() != "maven-project" {
		t.Errorf("Defaults(fixture).BuildType() = %q, want %q", got.BuildType(), "maven-project")
	}
}

// TestDefaultsFallbacks covers the two text fields the server leaves blank:
// name and packageName have no `default` in the metadata, so they must fall
// back to values derived from the identifiers instead of staying empty.
func TestDefaultsFallbacks(t *testing.T) {
	c := &catalog.Catalog{
		Text: map[string]string{
			"groupId":    "com.acme",
			"artifactId": "my-app",
		},
	}

	got := options.Defaults(c)

	if got.Name != "my-app" {
		t.Errorf("Name = %q, want %q (fallback to artifact id)", got.Name, "my-app")
	}
	if got.PackageName != "com.acme.myapp" {
		t.Errorf("PackageName = %q, want %q (derived, Java-safe)", got.PackageName, "com.acme.myapp")
	}
	if got.Build != "maven" {
		t.Errorf("Build = %q, want %q even without metadata", got.Build, "maven")
	}
}

// TestDefaultsNilCatalog documents the zero-value behaviour: a catalog with no
// metadata still yields a usable Options with the D3 build default.
func TestDefaultsNilCatalog(t *testing.T) {
	got := options.Defaults(nil)

	if got.Build != "maven" || got.BuildType() != "maven-project" {
		t.Errorf("Defaults(nil) = %+v, want Build maven/maven-project", got)
	}
	if got.Language != "" || got.ArtifactID != "" || got.PackageName != "" {
		t.Errorf("Defaults(nil) invented values: %+v", got)
	}
}

func TestBuildType(t *testing.T) {
	cases := []struct {
		build string
		want  string
	}{
		{"maven", "maven-project"},
		{"gradle", "gradle-project"},
		{"gradle-kotlin", "gradle-project-kotlin"},
		{"", ""},
		{"unknown", ""},
	}

	for _, tc := range cases {
		t.Run(tc.build, func(t *testing.T) {
			got := options.Options{Build: tc.build}.BuildType()
			if got != tc.want {
				t.Errorf("BuildType(%q) = %q, want %q", tc.build, got, tc.want)
			}
		})
	}
}

// TestForm pins the exact POST body: the documented parameter names, the
// build-type mapping, and the two conditional keys (bootVersion, dependencies).
func TestForm(t *testing.T) {
	base := options.Options{
		GroupID:     "com.example",
		ArtifactID:  "demo",
		Name:        "demo",
		Description: "Demo project for Spring Boot",
		PackageName: "com.example.demo",
		Build:       options.BuildMaven,
		Language:    "java",
		Packaging:   "jar",
		JavaVersion: "17",
		Deps:        []string{"web", "data-jpa"},
	}

	want := url.Values{
		"type":         {"maven-project"},
		"language":     {"java"},
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

	if got := base.Form(); !reflect.DeepEqual(got, want) {
		t.Errorf("Form() = %v, want %v", got, want)
	}

	t.Run("boot version included when set", func(t *testing.T) {
		opts := base
		opts.BootVersion = "4.1.2.RELEASE"
		if got := opts.Form().Get("bootVersion"); got != "4.1.2.RELEASE" {
			t.Errorf("Form() bootVersion = %q, want %q", got, "4.1.2.RELEASE")
		}
	})

	t.Run("dependencies omitted when empty", func(t *testing.T) {
		opts := base
		opts.Deps = nil
		form := opts.Form()
		if _, ok := form["dependencies"]; ok {
			t.Errorf("Form() sent dependencies for no deps: %v", form)
		}
	})

	t.Run("dependency order preserved", func(t *testing.T) {
		opts := base
		opts.Deps = []string{"data-jpa", "web"}
		if got := opts.Form().Get("dependencies"); got != "data-jpa,web" {
			t.Errorf("Form() dependencies = %q, want server order preserved", got)
		}
	})
}
