package tui

// These tests exercise the pure half of the form: the field descriptions built
// from the catalog and the mapping back to options.Options. They never open a
// terminal, so every offering can be asserted without rendering a frame.
//
// The fixture is a frozen capture of the metadata document; the value lists
// asserted here describe that payload and move with it deliberately.

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/huh"

	"github.com/miferdev/spring-inicializador/internal/catalog"
	"github.com/miferdev/spring-inicializador/internal/options"
)

// fixtureCatalog parses the checked-in metadata capture. It performs local file
// I/O only; there is no network access.
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

func fieldByKey(t *testing.T, fields []field, key Key) field {
	t.Helper()

	for _, f := range fields {
		if f.key == key {
			return f
		}
	}
	t.Fatalf("the form has no field %q", key)
	return field{}
}

func choiceValues(f field) []string {
	values := make([]string, 0, len(f.choices))
	for _, c := range f.choices {
		values = append(values, c.value)
	}
	return values
}

func choiceLabels(f field) []string {
	labels := make([]string, 0, len(f.choices))
	for _, c := range f.choices {
		labels = append(labels, c.label)
	}
	return labels
}

// TestFormOffersWhatTheServerPublishes is the core of US2.2: every select is
// populated from the catalog, in the server's order, and the build select
// offers only the three project archives, never the build-file types.
func TestFormOffersWhatTheServerPublishes(t *testing.T) {
	form := newProjectForm(fixtureCatalog(t), nil, Default())

	wantOrder := []struct {
		key   Key
		label string
	}{
		{KeyName, "Name"},
		{KeyLocation, "Location"},
		{KeyGroupID, "Group"},
		{KeyPackageName, "Package"},
		{KeyBuild, "Build"},
		{KeyLanguage, "Language"},
		{KeyPackaging, "Packaging"},
		{KeyJavaVersion, "Java"},
		{KeyBootVersion, "Boot"},
	}
	if len(form.fields) != len(wantOrder) {
		t.Fatalf("form has %d fields, want %d", len(form.fields), len(wantOrder))
	}
	for i, want := range wantOrder {
		got := form.fields[i]
		if got.key != want.key || got.label != want.label {
			t.Errorf("field %d = (%q, %q), want (%q, %q)", i, got.key, got.label, want.key, want.label)
		}
	}

	// Every row must be wired to the state, or the form would edit a discarded
	// string and optionsFromState would silently return defaults.
	values := form.state.pointers()
	for _, f := range form.fields {
		if values[f.key] == nil {
			t.Errorf("field %q is not bound to the form state", f.key)
		}
	}

	for _, key := range []Key{KeyName, KeyLocation, KeyGroupID, KeyPackageName} {
		f := fieldByKey(t, form.fields, key)
		if f.kind != fieldText {
			t.Errorf("field %q kind = %d, want a text field", key, f.kind)
		}
		if _, ok := f.widget(values[key]).(*huh.Input); !ok {
			t.Errorf("field %q does not render as an input", key)
		}
	}

	cases := []struct {
		key        Key
		wantValues []string
		wantLabels []string
	}{
		{
			KeyBuild,
			[]string{"gradle", "gradle-kotlin", "maven"},
			[]string{"Gradle - Groovy", "Gradle - Kotlin", "Maven"},
		},
		{
			KeyLanguage,
			[]string{"java", "kotlin", "groovy"},
			[]string{"Java", "Kotlin", "Groovy"},
		},
		{
			KeyPackaging,
			[]string{"jar", "war"},
			[]string{"Jar", "War"},
		},
		{
			KeyJavaVersion,
			[]string{"27", "25", "21", "17"},
			[]string{"27", "25", "21", "17"},
		},
		{
			KeyBootVersion,
			// The picker and the request use the id Maven Central resolves, so the
			// legacy .RELEASE suffix the catalog still publishes is gone (VS9.2).
			[]string{
				"4.2.0.BUILD-SNAPSHOT",
				"4.2.0.RC2",
				"4.1.3.BUILD-SNAPSHOT",
				"4.1.2",
				"4.0.10.BUILD-SNAPSHOT",
				"4.0.9",
			},
			[]string{
				"4.2.0 (SNAPSHOT)",
				"4.2.0 (RC2)",
				"4.1.3 (SNAPSHOT)",
				"4.1.2",
				"4.0.10 (SNAPSHOT)",
				"4.0.9",
			},
		},
	}
	for _, tc := range cases {
		f := fieldByKey(t, form.fields, tc.key)
		if f.kind != fieldSelect {
			t.Errorf("field %q kind = %d, want a select", tc.key, f.kind)
			continue
		}
		if got := choiceValues(f); !reflect.DeepEqual(got, tc.wantValues) {
			t.Errorf("%q offers %v, want %v in server order", tc.key, got, tc.wantValues)
		}
		if got := choiceLabels(f); !reflect.DeepEqual(got, tc.wantLabels) {
			t.Errorf("%q labels = %v, want %v", tc.key, got, tc.wantLabels)
		}
		if _, ok := f.widget(values[tc.key]).(*huh.Select[string]); !ok {
			t.Errorf("field %q does not render as a select", tc.key)
		}
	}
}

// TestFormDefaults pins the documented default of every field, including
// decision D3: Maven wins over the metadata's `gradle-project`.
func TestFormDefaults(t *testing.T) {
	cat := fixtureCatalog(t)
	state := newProjectForm(cat, nil, Default()).state

	if state.Build != options.BuildMaven {
		t.Errorf("Build = %q, want %q (D3)", state.Build, options.BuildMaven)
	}
	if got := (options.Options{Build: state.Build}).BuildType(); got != "maven-project" {
		t.Errorf("BuildType() = %q, want %q (D3)", got, "maven-project")
	}
	if state.Language != "java" {
		t.Errorf("Language = %q, want java", state.Language)
	}
	if state.Packaging != "jar" {
		t.Errorf("Packaging = %q, want jar", state.Packaging)
	}
	if state.JavaVersion != "17" {
		t.Errorf("JavaVersion = %q, want 17", state.JavaVersion)
	}
	// The server's default is the newest supported Boot, not the newest id: the
	// list starts with snapshots. Take it from the catalog, in the form the request
	// carries: the legacy .RELEASE suffix is trimmed (VS9.2).
	if want := options.RequestedBootVersion(cat.BootVersion.Default); state.BootVersion != want {
		t.Errorf("BootVersion = %q, want the requested default %q", state.BootVersion, want)
	}
	if state.BootVersion != "4.1.2" {
		t.Errorf("BootVersion = %q, want the documented 4.1.2", state.BootVersion)
	}

	// Text fields start on the metadata defaults. name and description have no
	// `default` in the document, so name falls back to the artifact id.
	if state.GroupID != cat.Text["groupId"] {
		t.Errorf("GroupID = %q, want %q", state.GroupID, cat.Text["groupId"])
	}
	if state.PackageName != cat.Text["packageName"] {
		t.Errorf("PackageName = %q, want %q", state.PackageName, cat.Text["packageName"])
	}
	if cat.Text["name"] != "" {
		t.Fatalf("fixture now publishes a name default %q; the fallback below needs rethinking", cat.Text["name"])
	}
	if state.Name != cat.Text["artifactId"] {
		t.Errorf("Name = %q, want the metadata artifact id %q", state.Name, cat.Text["artifactId"])
	}

	// The location starts where --output starts: the current directory.
	if state.Location != locationDefault {
		t.Errorf("Location = %q, want %q", state.Location, locationDefault)
	}

	// The name is the single source: the request's artifact id is the same value.
	opts := optionsFromState(cat, state)
	if opts.ArtifactID != state.Name || opts.Name != state.Name {
		t.Errorf("options name = (%q, %q), want both %q", opts.ArtifactID, opts.Name, state.Name)
	}
}

// TestMissingMetadataKeyBecomesANote covers the server that stops publishing a
// key: the field degrades to an explanatory note, keeps an empty value, and the
// run does not crash.
func TestMissingMetadataKeyBecomesANote(t *testing.T) {
	cat := fixtureCatalog(t)
	cat.Packaging = catalog.Select{}

	form := newProjectForm(cat, nil, Default())

	f := fieldByKey(t, form.fields, KeyPackaging)
	if f.kind != fieldNote {
		t.Fatalf("Packaging kind = %d, want a note when the metadata key is missing", f.kind)
	}
	if f.initial != "" {
		t.Errorf("Packaging keeps the value %q, want empty", f.initial)
	}
	if !strings.Contains(strings.ToLower(f.note), "packaging") {
		t.Errorf("note %q does not name the field", f.note)
	}
	if !strings.Contains(f.note, "did not publish") {
		t.Errorf("note %q does not explain the missing metadata", f.note)
	}
	if _, ok := f.widget(nil).(*huh.Note); !ok {
		t.Errorf("missing Packaging does not render as a note")
	}

	// The other selects are untouched: one missing key does not degrade the form.
	if got := fieldByKey(t, form.fields, KeyJavaVersion); got.kind != fieldSelect {
		t.Errorf("JavaVersion kind = %d, want a select", got.kind)
	}

	if got := optionsFromState(cat, form.state).Packaging; got != "" {
		t.Errorf("Packaging = %q, want empty so the server applies its own default", got)
	}
}

// TestOptionsFromStateRoundTrips proves the form and the flags produce the same
// value: a state left on its defaults is exactly options.Defaults.
func TestOptionsFromStateRoundTrips(t *testing.T) {
	cat := fixtureCatalog(t)

	got := optionsFromState(cat, newProjectForm(cat, nil, Default()).state)
	want := options.Defaults(cat)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("optionsFromState(defaults) = %+v, want %+v", got, want)
	}
}
