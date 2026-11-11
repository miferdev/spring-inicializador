package catalog_test

// These tests validate the checked-in metadata fixture as raw bytes. They are
// deliberately independent of the production catalog package: they decode the
// file with their own minimal structs so a broken parser cannot hide a broken
// fixture, and a fixture refresh cannot silently change parser expectations.
//
// The 23/206 counts describe this specific captured payload. They live here,
// in the fixture test, and must never appear in production code.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureExpectedCategories and fixtureExpectedDependencies describe the
// frozen payload behind testdata/metadata-client.json. A fixture replacement
// is a deliberate act: update the file, the sibling README checksum and these
// two numbers together.
const (
	fixtureExpectedCategories   = 23
	fixtureExpectedDependencies = 206
)

// fixtureLink is one entry of a dependency `guide` link, whether the server
// serializes it as a single object or as an array of objects.
type fixtureLink struct {
	Href  string `json:"href"`
	Title string `json:"title"`
}

// fixtureDependency is the minimal dependency shape this test cares about.
// Links stay raw so the guide polymorphism (object vs array) is visible here
// instead of being flattened by the decoder.
type fixtureDependency struct {
	ID          string                     `json:"id"`
	Name        string                     `json:"name"`
	Description string                     `json:"description"`
	Links       map[string]json.RawMessage `json:"_links"`
}

type fixtureCategory struct {
	Name   string              `json:"name"`
	Values []fixtureDependency `json:"values"`
}

// fixtureSelectEntry is one option of a select field such as `type` or
// `javaVersion`; the server sends `id`/`name` pairs, not bare strings.
type fixtureSelectEntry struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type fixtureSelect struct {
	Default string               `json:"default"`
	Values  []fixtureSelectEntry `json:"values"`
}

type fixtureMetadata struct {
	Links        map[string]json.RawMessage `json:"_links"`
	Dependencies struct {
		Values []fixtureCategory `json:"values"`
	} `json:"dependencies"`
	Type        fixtureSelect `json:"type"`
	Packaging   fixtureSelect `json:"packaging"`
	JavaVersion fixtureSelect `json:"javaVersion"`
	Language    fixtureSelect `json:"language"`
	BootVersion fixtureSelect `json:"bootVersion"`
	// The remaining project fields are also objects ({type, default}) rather
	// than bare strings; keep them raw because this test only asserts they are
	// present (see TestFixtureTopLevelShape).
	GroupID     json.RawMessage `json:"groupId"`
	ArtifactID  json.RawMessage `json:"artifactId"`
	Version     json.RawMessage `json:"version"`
	Name        json.RawMessage `json:"name"`
	Description json.RawMessage `json:"description"`
	PackageName json.RawMessage `json:"packageName"`
}

// fixtureRead returns the raw fixture bytes from testdata. It performs local
// file I/O only; there is no network access anywhere in this file.
func fixtureRead(t *testing.T) []byte {
	t.Helper()
	path := filepath.Join("testdata", "metadata-client.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	if len(raw) == 0 {
		t.Fatalf("fixture %s is empty", path)
	}
	return raw
}

func fixtureLoad(t *testing.T) fixtureMetadata {
	t.Helper()
	raw := fixtureRead(t)
	var md fixtureMetadata
	if err := json.Unmarshal(raw, &md); err != nil {
		t.Fatalf("decode fixture as JSON: %v", err)
	}
	return md
}

// TestFixtureTopLevelShape proves the file is a full metadata/client document,
// not a trimmed or hand-written mock: every documented top-level key and every
// real generation link is present.
func TestFixtureTopLevelShape(t *testing.T) {
	raw := fixtureRead(t)

	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		t.Fatalf("decode fixture as JSON object: %v", err)
	}

	for _, key := range []string{
		"_links", "dependencies", "type", "packaging", "javaVersion",
		"language", "bootVersion", "groupId", "artifactId", "version",
		"name", "description", "packageName",
	} {
		if _, ok := top[key]; !ok {
			t.Errorf("fixture is missing top-level key %q", key)
		}
	}

	var links map[string]json.RawMessage
	if err := json.Unmarshal(top["_links"], &links); err != nil {
		t.Fatalf("decode _links: %v", err)
	}
	for _, rel := range []string{
		"gradle-project", "gradle-project-kotlin", "gradle-build",
		"maven-project", "maven-build", "dependencies",
	} {
		if _, ok := links[rel]; !ok {
			t.Errorf("fixture _links is missing rel %q", rel)
		}
	}
}

// TestFixtureCounts pins the two numbers this fixture is expected to carry.
func TestFixtureCounts(t *testing.T) {
	md := fixtureLoad(t)

	categories := len(md.Dependencies.Values)
	dependencies := 0
	for _, c := range md.Dependencies.Values {
		dependencies += len(c.Values)
	}

	t.Logf("observed %d categories and %d dependencies", categories, dependencies)

	if categories != fixtureExpectedCategories {
		t.Errorf("category count = %d, want %d", categories, fixtureExpectedCategories)
	}
	if dependencies != fixtureExpectedDependencies {
		t.Errorf("dependency count = %d, want %d", dependencies, fixtureExpectedDependencies)
	}
}

// TestFixtureCategoriesAndDependencies proves each category and dependency
// carries the fields the parser must expose, and that names and IDs are
// unambiguous across the whole document.
func TestFixtureCategoriesAndDependencies(t *testing.T) {
	md := fixtureLoad(t)

	seenCategories := make(map[string]struct{}, len(md.Dependencies.Values))
	seenIDs := make(map[string]string, fixtureExpectedDependencies)

	for _, c := range md.Dependencies.Values {
		if strings.TrimSpace(c.Name) == "" {
			t.Errorf("category with empty name")
			continue
		}
		if c.Values == nil {
			t.Errorf("category %q has no values array", c.Name)
		}
		if _, dup := seenCategories[c.Name]; dup {
			t.Errorf("duplicate category name %q", c.Name)
		}
		seenCategories[c.Name] = struct{}{}

		for _, dep := range c.Values {
			if strings.TrimSpace(dep.ID) == "" {
				t.Errorf("dependency in category %q has empty id", c.Name)
			}
			if strings.TrimSpace(dep.Name) == "" {
				t.Errorf("dependency %q has empty name", dep.ID)
			}
			if strings.TrimSpace(dep.Description) == "" {
				t.Errorf("dependency %q has empty description", dep.ID)
			}
			if previous, dup := seenIDs[dep.ID]; dup {
				t.Errorf("duplicate dependency id %q (in %q and %q)", dep.ID, previous, c.Name)
			}
			seenIDs[dep.ID] = c.Name
		}
	}
}

// TestFixtureSelectDefaults proves the select fields are well formed: each has
// a non-empty default and that default is one of its own options. The server
// sends options as {id, name} objects, so the default is compared to the id.
func TestFixtureSelectDefaults(t *testing.T) {
	md := fixtureLoad(t)

	selects := map[string]fixtureSelect{
		"type":        md.Type,
		"packaging":   md.Packaging,
		"javaVersion": md.JavaVersion,
		"language":    md.Language,
		"bootVersion": md.BootVersion,
	}
	for name, sel := range selects {
		if strings.TrimSpace(sel.Default) == "" {
			t.Errorf("%s: default is empty", name)
			continue
		}
		found := false
		for _, v := range sel.Values {
			if v.ID == sel.Default {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s: default %q is not among its values", name, sel.Default)
		}
	}
}

// TestFixtureGuideAndLinkShapes proves the payload still exercises every shape
// the parser must survive, and reports the observed counts so a fixture
// replacement shows up as a deliberate diff. Counts are computed from the file,
// never assumed.
func TestFixtureGuideAndLinkShapes(t *testing.T) {
	md := fixtureLoad(t)

	var (
		guideArrays  int
		guideObjects int
		noLinks      int
		emptyTitles  int
		guideEntries int
	)
	for _, c := range md.Dependencies.Values {
		for _, dep := range c.Values {
			if len(dep.Links) == 0 {
				noLinks++
				continue
			}
			rawGuide, ok := dep.Links["guide"]
			if !ok {
				continue
			}

			var asArray []fixtureLink
			if err := json.Unmarshal(rawGuide, &asArray); err == nil {
				guideArrays++
				for _, g := range asArray {
					guideEntries++
					if strings.TrimSpace(g.Title) == "" {
						emptyTitles++
					}
				}
				continue
			}

			var asObject fixtureLink
			if err := json.Unmarshal(rawGuide, &asObject); err == nil {
				guideObjects++
				guideEntries++
				if strings.TrimSpace(asObject.Title) == "" {
					emptyTitles++
				}
				continue
			}

			t.Errorf("dependency %q has a guide that is neither an object nor an array", dep.ID)
		}
	}

	t.Logf("observed guide shapes: arrays=%d objects=%d guide entries=%d no _links=%d entries with empty/absent title=%d",
		guideArrays, guideObjects, guideEntries, noLinks, emptyTitles)

	cases := []struct {
		name  string
		count int
	}{
		{"dependency with guide as an array", guideArrays},
		{"dependency with guide as a single object", guideObjects},
		{"dependency with no _links", noLinks},
		{"guide entry with empty or absent title", emptyTitles},
	}
	for _, tc := range cases {
		if tc.count < 1 {
			t.Errorf("fixture no longer exercises %s (observed %d)", tc.name, tc.count)
		}
	}
}
