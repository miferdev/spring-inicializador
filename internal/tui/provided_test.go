package tui

// Tests for US2.3: the values the command line supplied are not asked for. The
// form describes what is left, so these assertions need no terminal.

import (
	"bytes"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/miferdev/spring-inicializador/internal/catalog"
	"github.com/miferdev/spring-inicializador/internal/options"
)

func fieldKeys(fields []field) []Key {
	keys := make([]Key, 0, len(fields))
	for _, f := range fields {
		keys = append(keys, f.key)
	}
	return keys
}

// TestFormOmitsProvidedFields proves a flagged row disappears, the rest keep
// their order, and the flagged value wins over the catalog default.
func TestFormOmitsProvidedFields(t *testing.T) {
	cat := fixtureCatalog(t)
	provided := Values{
		KeyGroupID: "com.acme",
		KeyBuild:   options.BuildGradle,
	}

	form := newProjectForm(cat, provided, Default())

	wantOrder := []Key{
		KeyArtifactID, KeyName, KeyPackageName,
		KeyLanguage, KeyPackaging, KeyJavaVersion, KeyBootVersion,
	}
	if got := fieldKeys(form.fields); !reflect.DeepEqual(got, wantOrder) {
		t.Errorf("form fields = %v, want %v", got, wantOrder)
	}

	if form.state.GroupID != "com.acme" {
		t.Errorf("GroupID = %q, want the flagged com.acme", form.state.GroupID)
	}
	if form.state.Build != options.BuildGradle {
		t.Errorf("Build = %q, want the flagged %q", form.state.Build, options.BuildGradle)
	}
	// Rows nobody answered keep the catalog defaults.
	if form.state.Language != cat.Language.Default {
		t.Errorf("Language = %q, want the catalog default %q", form.state.Language, cat.Language.Default)
	}

	// The flagged build reaches the request instead of the D3/catalog default.
	opts := optionsFromState(cat, form.state)
	if opts.BuildType() != "gradle-project" {
		t.Errorf("BuildType() = %q, want gradle-project", opts.BuildType())
	}
	if opts.GroupID != "com.acme" {
		t.Errorf("GroupID = %q, want com.acme", opts.GroupID)
	}
}

// TestFormDerivesPackageFromFlaggedIdentifiers proves flagged identifiers feed
// the derivation, so the first frame already shows the derived name.
func TestFormDerivesPackageFromFlaggedIdentifiers(t *testing.T) {
	cat := fixtureCatalog(t)

	form := newProjectForm(cat, Values{
		KeyGroupID:    "com.acme",
		KeyArtifactID: "billing",
	}, Default())

	want := options.DerivePackageName("com.acme", "billing")
	if form.state.PackageName != want {
		t.Errorf("PackageName = %q, want the derived %q", form.state.PackageName, want)
	}
	if !form.state.packageAuto {
		t.Error("derivation is off although no flag supplied the package name")
	}
	if form.state.packageMarker() != autoMarker {
		t.Errorf("marker = %q, want %q", form.state.packageMarker(), autoMarker)
	}
}

// TestProvidedPackageNameSurvivesIdentifierChanges proves a flag-supplied package
// name is a human decision: derivation is off, the row is gone, and no later
// identifier change replaces the value.
func TestProvidedPackageNameSurvivesIdentifierChanges(t *testing.T) {
	cat := fixtureCatalog(t)

	form := newProjectForm(cat, Values{KeyPackageName: "org.custom"}, Default())

	for _, field := range form.fields {
		if field.key == KeyPackageName {
			t.Fatal("the package row is still asked for after --package-name")
		}
	}
	if form.packageInput != nil {
		t.Error("the omitted package row still has a widget the model could derive into")
	}
	if form.state.packageAuto {
		t.Error("derivation is active for a flag-supplied package name")
	}

	// Later identifier changes leave the value alone, directly and through the
	// model: there is no package widget, so the update loop cannot derive.
	form.state.GroupID = "com.acme"
	form.state.ArtifactID = "billing"
	if form.state.followPackage() {
		t.Error("followPackage overwrote a flag-supplied package name")
	}
	if got := optionsFromState(cat, form.state).PackageName; got != "org.custom" {
		t.Errorf("PackageName = %q, want the flag value org.custom", got)
	}
}

// TestAsks pins when the form still has something to put on screen.
func TestAsks(t *testing.T) {
	cat := fixtureCatalog(t)

	if !Asks(cat, nil) {
		t.Error("Asks(nil) = false, want true for a fresh form")
	}
	if !Asks(cat, Values{KeyBuild: options.BuildGradle}) {
		t.Error("Asks(partial) = false, want true while a row is unanswered")
	}

	// A metadata gap is a note, not a question: with packaging missing, a run
	// that answered the other eight rows has nothing to ask.
	gap := fixtureCatalog(t)
	gap.Packaging = catalog.Select{}
	answered := Values{
		KeyGroupID:     "com.acme",
		KeyArtifactID:  "billing",
		KeyName:        "billing",
		KeyPackageName: "com.acme.billing",
		KeyBuild:       options.BuildGradle,
		KeyLanguage:    options.LanguageKotlin,
		KeyJavaVersion: "21",
		KeyBootVersion: "4.0.9.RELEASE",
	}
	if Asks(gap, answered) {
		t.Error("Asks = true although only a note is left")
	}
}

// TestRunWithEveryFieldProvidedNeverStartsAProgram proves the form is not
// opened when the flags answered every row: Run must return before touching
// the terminal, carrying the dependencies the command line already answered.
func TestRunWithEveryFieldProvidedNeverStartsAProgram(t *testing.T) {
	cat := fixtureCatalog(t)
	provided := Values{
		KeyGroupID:     "com.acme",
		KeyArtifactID:  "billing",
		KeyName:        "billing",
		KeyPackageName: "com.acme.billing",
		KeyBuild:       options.BuildGradle,
		KeyLanguage:    options.LanguageKotlin,
		KeyPackaging:   options.PackagingWar,
		KeyJavaVersion: "21",
		KeyBootVersion: "4.0.9.RELEASE",
	}

	// A reader nobody writes to would block a program that asked for a key, and
	// a program would paint escape sequences on stdout.
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() {
		reader.Close()
		writer.Close()
	})

	var stdout bytes.Buffer
	type result struct {
		opts options.Options
		ok   bool
		err  error
	}
	done := make(chan result, 1)
	go func() {
		opts, ok, err := Run(cat, provided, []string{"web", "data-jpa"}, Settings{}, ".", reader, &stdout)
		done <- result{opts, ok, err}
	}()

	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("Run: %v", got.err)
		}
		if !got.ok {
			t.Fatal("Run reported an abort for a fully answered form")
		}
		if stdout.Len() != 0 {
			t.Errorf("Run wrote %q to the terminal, want no program", stdout.String())
		}
		if got.opts.GroupID != "com.acme" || got.opts.Build != options.BuildGradle {
			t.Errorf("options = %+v, want the flag values", got.opts)
		}
		if got.opts.PackageName != "com.acme.billing" {
			t.Errorf("PackageName = %q, want the flag value", got.opts.PackageName)
		}
		// No screen is shown, so the dependencies are exactly the flag's answer.
		if want := []string{"web", "data-jpa"}; !reflect.DeepEqual(got.opts.Deps, want) {
			t.Errorf("Deps = %v, want the flag value %v", got.opts.Deps, want)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run blocked: it may have started the form")
	}
}
