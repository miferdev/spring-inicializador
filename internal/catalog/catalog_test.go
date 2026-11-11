package catalog

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

// These literals describe the frozen fixture (see testdata/README.md). They
// live only in tests because they are a property of the capture, not a rule
// the parser or any production code may rely on.
const (
	fixtureCategories   = 23
	fixtureDependencies = 206
)

func parseFixtureBytes(t *testing.T) []byte {
	t.Helper()

	data, err := os.ReadFile("testdata/metadata-client.json")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	return data
}

func parseFixture(t *testing.T) *Catalog {
	t.Helper()

	c, err := Parse(parseFixtureBytes(t))
	if err != nil {
		t.Fatalf("Parse(fixture): %v", err)
	}
	return c
}

func TestParseFixtureShape(t *testing.T) {
	c := parseFixture(t)

	if got := len(c.Categories); got != fixtureCategories {
		t.Errorf("categories = %d, want %d", got, fixtureCategories)
	}
	if got := len(c.Dependencies); got != fixtureDependencies {
		t.Errorf("dependencies = %d, want %d", got, fixtureDependencies)
	}

	inCategories := 0
	for _, cat := range c.Categories {
		inCategories += len(cat.Dependencies)
	}
	if inCategories != len(c.Dependencies) {
		t.Errorf("dependencies listed in categories = %d, flat = %d", inCategories, len(c.Dependencies))
	}

	for _, want := range []string{"web", "data-jpa", "postgresql"} {
		if _, ok := c.Lookup(want); !ok {
			t.Errorf("Lookup(%q) not found in fixture", want)
		}
	}
}

func TestParseOrderingIsStable(t *testing.T) {
	data := parseFixtureBytes(t)

	first, err := Parse(data)
	if err != nil {
		t.Fatalf("first Parse: %v", err)
	}
	second, err := Parse(data)
	if err != nil {
		t.Fatalf("second Parse: %v", err)
	}

	categoryNames := func(c *Catalog) []string {
		names := make([]string, 0, len(c.Categories))
		for _, cat := range c.Categories {
			names = append(names, cat.Name)
		}
		return names
	}
	if got, want := categoryNames(second), categoryNames(first); !reflect.DeepEqual(got, want) {
		t.Errorf("category order differs:\nfirst:  %v\nsecond: %v", want, got)
	}

	dependencyIDs := func(c *Catalog) []string {
		ids := make([]string, 0, len(c.Dependencies))
		for _, dep := range c.Dependencies {
			ids = append(ids, dep.ID)
		}
		return ids
	}
	if got, want := dependencyIDs(second), dependencyIDs(first); !reflect.DeepEqual(got, want) {
		t.Errorf("dependency order differs:\nfirst:  %v\nsecond: %v", want, got)
	}

	if !reflect.DeepEqual(first.Dependencies, second.Dependencies) {
		t.Error("dependency contents differ between parses")
	}

	for _, dep := range first.Dependencies {
		got, ok := second.Lookup(dep.ID)
		if !ok {
			t.Errorf("second parse lost dependency %q", dep.ID)
			continue
		}
		if !reflect.DeepEqual(got, dep) {
			t.Errorf("Lookup(%q) differs between parses", dep.ID)
		}
	}

	if !reflect.DeepEqual(categoryNames(first), categoryNames(second)) {
		t.Error("category names differ between parses")
	}
}

func TestLookup(t *testing.T) {
	c := parseFixture(t)

	tests := []struct {
		name         string
		id           string
		wantFound    bool
		wantName     string
		wantCategory string
	}{
		{name: "web", id: "web", wantFound: true, wantName: "Spring Web", wantCategory: "Web"},
		{name: "postgresql", id: "postgresql", wantFound: true, wantCategory: "SQL"},
		{name: "unknown", id: "nope", wantFound: false},
		{name: "empty id", id: "", wantFound: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dep, ok := c.Lookup(tc.id)
			if ok != tc.wantFound {
				t.Fatalf("Lookup(%q) found = %v, want %v", tc.id, ok, tc.wantFound)
			}
			if !tc.wantFound {
				if !reflect.DeepEqual(dep, Dependency{}) {
					t.Errorf("Lookup(%q) returned non-zero dependency on miss: %+v", tc.id, dep)
				}
				return
			}
			if tc.wantName != "" && dep.Name != tc.wantName {
				t.Errorf("Lookup(%q).Name = %q, want %q", tc.id, dep.Name, tc.wantName)
			}
			if dep.Category != tc.wantCategory {
				t.Errorf("Lookup(%q).Category = %q, want %q", tc.id, dep.Category, tc.wantCategory)
			}
		})
	}
}

func TestLookupMatchesFlatSlice(t *testing.T) {
	c := parseFixture(t)

	if len(c.Dependencies) != fixtureDependencies {
		t.Fatalf("flat dependencies = %d, want %d", len(c.Dependencies), fixtureDependencies)
	}

	unique := make(map[string]struct{}, len(c.Dependencies))
	for i := range c.Dependencies {
		dep := c.Dependencies[i]
		unique[dep.ID] = struct{}{}

		got, ok := c.Lookup(dep.ID)
		if !ok {
			t.Errorf("Lookup(%q) not found for flat dependency %d", dep.ID, i)
			continue
		}
		if !reflect.DeepEqual(got, dep) {
			t.Errorf("Lookup(%q) = %+v, want flat entry %d = %+v", dep.ID, got, i, dep)
		}
	}
	if len(unique) != len(c.Dependencies) {
		t.Errorf("flat slice has %d entries but %d unique IDs", len(c.Dependencies), len(unique))
	}
}

func TestLookupIgnoresEmptyID(t *testing.T) {
	data := []byte(`{"dependencies":{"values":[
		{"name":"A","values":[{"id":"","name":"Nameless"}]}
	]}}`)
	c, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse(): %v", err)
	}

	if len(c.Dependencies) != 1 {
		t.Fatalf("flat dependencies = %d, want 1", len(c.Dependencies))
	}
	if len(c.Categories) != 1 || len(c.Categories[0].Dependencies) != 1 {
		t.Fatalf("category membership = %+v, want one dependency", c.Categories)
	}
	if _, ok := c.Lookup(""); ok {
		t.Error("Lookup(\"\") found a dependency with an empty id")
	}
}

func TestParseGuidesFromFixture(t *testing.T) {
	c := parseFixture(t)

	tests := []struct {
		name           string
		id             string
		wantCount      int
		wantEmptyTitle bool
	}{
		{name: "array of guides", id: "web", wantCount: 3},
		{name: "single object guide", id: "data-jpa", wantCount: 1},
		{name: "guide without title", id: "htmx", wantCount: 1, wantEmptyTitle: true},
		{name: "no guide links", id: "dgs-codegen", wantCount: 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dep, ok := c.Lookup(tc.id)
			if !ok {
				t.Fatalf("Lookup(%q) not found", tc.id)
			}
			if got := len(dep.Guides); got != tc.wantCount {
				t.Fatalf("len(Guides) = %d, want %d", got, tc.wantCount)
			}
			for i, link := range dep.Guides {
				if link.Href == "" {
					t.Errorf("Guides[%d].Href is empty", i)
				}
				if tc.wantEmptyTitle {
					if link.Title != "" {
						t.Errorf("Guides[%d].Title = %q, want empty", i, link.Title)
					}
					continue
				}
				if link.Title == "" {
					t.Errorf("Guides[%d].Title is empty", i)
				}
			}
		})
	}
}

func TestParseGuideNull(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "null guide",
			data: []byte(`{"dependencies":{"values":[{"name":"A","values":[{"id":"null-guide","_links":{"guide":null}}]}]}}`),
		},
		{
			name: "links without guide",
			data: []byte(`{"dependencies":{"values":[{"name":"A","values":[{"id":"no-guide","_links":{"reference":{"href":"https://example.test"}}}]}]}}`),
		},
		{
			name: "null links",
			data: []byte(`{"dependencies":{"values":[{"name":"A","values":[{"id":"no-links","_links":null}]}]}}`),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, err := Parse(tc.data)
			if err != nil {
				t.Fatalf("Parse(): %v", err)
			}
			dep := c.Dependencies[0]
			if dep.Guides != nil {
				t.Errorf("Guides = %+v, want nil", dep.Guides)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "nil input", data: nil},
		{name: "empty input", data: []byte{}},
		{name: "truncated JSON", data: []byte("{")},
		{name: "dependencies values wrong type", data: []byte(`{"dependencies":{"values":42}}`)},
		{name: "category values wrong type", data: []byte(`{"dependencies":{"values":[{"name":"A","values":"nope"}]}}`)},
		{name: "guide wrong type", data: []byte(`{"dependencies":{"values":[{"name":"A","values":[{"id":"x","_links":{"guide":42}}]}]}}`)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.data)
			if err == nil {
				t.Fatalf("Parse() error = nil, want an error (catalog = %+v)", got)
			}
			if !strings.HasPrefix(err.Error(), "catalog:") {
				t.Errorf("error = %q, want a \"catalog:\" prefix", err)
			}
		})
	}
}

func TestParseTolerance(t *testing.T) {
	t.Run("no categories is a valid catalog", func(t *testing.T) {
		c, err := Parse([]byte(`{"dependencies":{"values":[]}}`))
		if err != nil {
			t.Fatalf("Parse(): %v", err)
		}
		if len(c.Categories) != 0 {
			t.Errorf("categories = %d, want 0", len(c.Categories))
		}
		if len(c.Dependencies) != 0 {
			t.Errorf("dependencies = %d, want 0", len(c.Dependencies))
		}
		if _, ok := c.Lookup("web"); ok {
			t.Error("Lookup on empty catalog found a dependency")
		}
	})

	t.Run("unknown fields are tolerated", func(t *testing.T) {
		data := []byte(`{
			"surprise": true,
			"dependencies": {
				"values": [{
					"name": "Web",
					"unknown": "ignored",
					"values": [{"id": "web", "name": "Spring Web", "extra": [1, 2]}]
				}],
				"other": "ignored"
			},
			"type": {"type": "action", "default": "maven-project", "values": [{"id": "maven-project", "name": "Maven"}]},
			"extraSelect": {"default": "x"}
		}`)
		c, err := Parse(data)
		if err != nil {
			t.Fatalf("Parse(): %v", err)
		}
		if len(c.Categories) != 1 || len(c.Dependencies) != 1 {
			t.Fatalf("categories = %d, dependencies = %d, want 1 and 1", len(c.Categories), len(c.Dependencies))
		}
		if c.Type.Default != "maven-project" {
			t.Errorf("Type.Default = %q, want %q", c.Type.Default, "maven-project")
		}
	})

	t.Run("category with zero dependencies is retained", func(t *testing.T) {
		c, err := Parse([]byte(`{"dependencies":{"values":[{"name":"Empty","values":[]}]}}`))
		if err != nil {
			t.Fatalf("Parse(): %v", err)
		}
		if len(c.Categories) != 1 {
			t.Fatalf("categories = %d, want 1", len(c.Categories))
		}
		if c.Categories[0].Name != "Empty" {
			t.Errorf("category name = %q, want %q", c.Categories[0].Name, "Empty")
		}
		if len(c.Categories[0].Dependencies) != 0 {
			t.Errorf("category dependencies = %d, want 0", len(c.Categories[0].Dependencies))
		}
	})
}

func TestParseDuplicates(t *testing.T) {
	data := []byte(`{"dependencies":{"values":[
		{"name":"A","values":[{"id":"dup","name":"First"},{"id":"dup","name":"Second"}]},
		{"name":"B","values":[{"id":"dup","name":"Third"}]}
	]}}`)
	c, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse(): %v", err)
	}

	if len(c.Dependencies) != 1 {
		t.Fatalf("dependencies = %d, want 1", len(c.Dependencies))
	}
	dep, ok := c.Lookup("dup")
	if !ok {
		t.Fatal("Lookup(\"dup\") not found")
	}
	if dep.Name != "First" {
		t.Errorf("Lookup(\"dup\").Name = %q, want %q", dep.Name, "First")
	}
	if dep.Category != "A" {
		t.Errorf("Lookup(\"dup\").Category = %q, want %q", dep.Category, "A")
	}
	if len(c.Categories[0].Dependencies) != 1 {
		t.Errorf("category A dependencies = %d, want 1", len(c.Categories[0].Dependencies))
	}
	if len(c.Categories[1].Dependencies) != 0 {
		t.Errorf("category B dependencies = %d, want 0", len(c.Categories[1].Dependencies))
	}
	if c.Categories[0].Dependencies[0] != &c.Dependencies[0] {
		t.Error("category A does not point at the stored dependency")
	}
	for _, later := range []string{"Second", "Third"} {
		if c.Dependencies[0].Name == later {
			t.Errorf("later duplicate %q survived in the flat slice", later)
		}
	}
}

// TestCategoryPointersResolve proves that every category holds pointers into
// the flat Dependencies slice and that walking the categories in order
// reproduces that slice entry by entry.
func TestCategoryPointersResolve(t *testing.T) {
	c := parseFixture(t)

	cursor := 0
	for ci := range c.Categories {
		cat := &c.Categories[ci]
		for _, dep := range cat.Dependencies {
			if cursor >= len(c.Dependencies) {
				t.Fatalf("category %q lists more dependencies than the flat slice holds", cat.Name)
			}
			flat := &c.Dependencies[cursor]
			if dep != flat {
				t.Fatalf("category %q entry %q does not point at c.Dependencies[%d]", cat.Name, dep.ID, cursor)
			}
			if !reflect.DeepEqual(*dep, *flat) {
				t.Fatalf("category %q entry %q dereferences to different values than c.Dependencies[%d]", cat.Name, dep.ID, cursor)
			}
			if dep.Category != cat.Name {
				t.Errorf("dependency %q Category = %q, owning category = %q", dep.ID, dep.Category, cat.Name)
			}
			cursor++
		}
	}
	if cursor != len(c.Dependencies) {
		t.Errorf("category walk consumed %d dependencies, flat slice holds %d", cursor, len(c.Dependencies))
	}
}

func TestSelects(t *testing.T) {
	c := parseFixture(t)

	tests := []struct {
		name        string
		sel         Select
		wantDefault string
		wantValues  int
	}{
		{name: "type", sel: c.Type, wantDefault: "gradle-project", wantValues: 5},
		{name: "packaging", sel: c.Packaging, wantDefault: "jar", wantValues: 2},
		{name: "javaVersion", sel: c.JavaVersion, wantDefault: "17", wantValues: 4},
		{name: "language", sel: c.Language, wantDefault: "java", wantValues: 3},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.sel.Default != tc.wantDefault {
				t.Errorf("Default = %q, want %q", tc.sel.Default, tc.wantDefault)
			}
			if got := len(tc.sel.Values); got != tc.wantValues {
				t.Errorf("len(Values) = %d, want %d", got, tc.wantValues)
			}
		})
	}

	t.Run("bootVersion", func(t *testing.T) {
		if c.BootVersion.Default != "4.1.2.RELEASE" {
			t.Errorf("Default = %q, want %q", c.BootVersion.Default, "4.1.2.RELEASE")
		}
		found := false
		for _, v := range c.BootVersion.Values {
			if v.ID == c.BootVersion.Default {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Default %q is not present in Values", c.BootVersion.Default)
		}
	})
}

func TestTextDefaults(t *testing.T) {
	c := parseFixture(t)

	tests := []struct {
		key  string
		want string
	}{
		{key: "groupId", want: "com.example"},
		{key: "artifactId", want: "demo"},
		{key: "version", want: "0.0.1-SNAPSHOT"},
		{key: "packageName", want: "com.example.demo"},
		{key: "name", want: ""},
		{key: "description", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			got, ok := c.Text[tc.key]
			if !ok {
				t.Fatalf("Text[%q] missing", tc.key)
			}
			if got != tc.want {
				t.Errorf("Text[%q] = %q, want %q", tc.key, got, tc.want)
			}
		})
	}
}
