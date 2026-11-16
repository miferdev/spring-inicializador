package tui

// Tests for US2.1: the package field follows group and artifact until a human
// edits it. The rule is a method on formState, so it is asserted without a
// terminal; the model tests drive Update with plain Bubble Tea messages.
//
// The round trip a fresh state must keep is covered by
// TestOptionsFromStateRoundTrips in form_test.go, which now starts from a state
// with the derivation armed.

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	"github.com/miferdev/spring-inicializador/internal/options"
)

// newTestModel builds the wrapper the program runs, without starting a program.
func newTestModel(t *testing.T) formModel {
	t.Helper()

	project := newProjectForm(fixtureCatalog(t), nil, Default())
	return formModel{
		Form:         project.form,
		state:        &project.state,
		packageInput: project.packageInput,
		firstKey:     project.firstKey,
	}
}

// TestPackageFollowsIdentifiers pins the story's example and the live behaviour
// behind it: every identifier change recomputes the field.
func TestPackageFollowsIdentifiers(t *testing.T) {
	state := newProjectForm(fixtureCatalog(t), nil, Default()).state

	state.GroupID = "com.acme"
	if !state.followPackage() {
		t.Fatal("followPackage reported no change after the group changed")
	}
	if want := "com.acme.demo"; state.PackageName != want {
		t.Errorf("after the group: PackageName = %q, want %q", state.PackageName, want)
	}

	state.ArtifactID = "billing"
	state.followPackage()
	want := options.DerivePackageName("com.acme", "billing")
	if want != "com.acme.billing" {
		t.Fatalf("DerivePackageName(com.acme, billing) = %q, want com.acme.billing", want)
	}
	if state.PackageName != want {
		t.Errorf("PackageName = %q, want %q", state.PackageName, want)
	}
	if state.packageMarker() != autoMarker {
		t.Errorf("marker = %q, want %q while deriving", state.packageMarker(), autoMarker)
	}

	// A second artifact change must re-derive again, not freeze on the first.
	state.ArtifactID = "ledger"
	state.followPackage()
	if want := options.DerivePackageName("com.acme", "ledger"); state.PackageName != want {
		t.Errorf("after the second artifact change: PackageName = %q, want %q", state.PackageName, want)
	}
}

// TestHumanEditWins proves an edit is permanent: later identifier changes never
// move the field again, and the (auto) marker disappears.
func TestHumanEditWins(t *testing.T) {
	state := newProjectForm(fixtureCatalog(t), nil, Default()).state

	state.PackageName = "org.custom"
	if state.followPackage() {
		t.Error("followPackage wrote a derived value over a human edit")
	}
	if state.packageAuto {
		t.Error("the derivation is still active after a human edit")
	}
	if marker := state.packageMarker(); marker != "" {
		t.Errorf("marker = %q, want none after a human edit", marker)
	}

	state.GroupID = "com.acme"
	state.ArtifactID = "billing"
	state.followPackage()

	if state.PackageName != "org.custom" {
		t.Errorf("PackageName = %q, want the human value org.custom", state.PackageName)
	}
	if state.packageMarker() != "" {
		t.Errorf("marker = %q, want none after a human edit", state.packageMarker())
	}
}

// TestClearingResumesDerivation proves an empty field re-arms the rule and never
// reaches the request empty.
func TestClearingResumesDerivation(t *testing.T) {
	state := newProjectForm(fixtureCatalog(t), nil, Default()).state
	state.PackageName = "org.custom"
	state.followPackage()

	state.PackageName = ""
	if !state.followPackage() {
		t.Error("an empty package did not re-arm the derivation")
	}
	if state.PackageName == "" {
		t.Fatal("the package stayed empty; the server would receive no package name")
	}
	if state.packageMarker() != autoMarker {
		t.Errorf("marker = %q, want %q after resuming", state.packageMarker(), autoMarker)
	}

	state.ArtifactID = "ledger"
	state.followPackage()

	want := options.DerivePackageName(state.GroupID, "ledger")
	if state.PackageName != want {
		t.Errorf("PackageName = %q, want the re-derived %q", state.PackageName, want)
	}
	if state.packageMarker() != autoMarker {
		t.Errorf("marker = %q, want %q after the artifact changed", state.packageMarker(), autoMarker)
	}
}

// TestFormModelTypingIntoPackageStopsDerivation drives a real key message
// through the wrapper: no terminal, but the same path the program takes.
func TestFormModelTypingIntoPackageStopsDerivation(t *testing.T) {
	model := newTestModel(t)

	// huh advances rows only through its own next-field message, which is what a
	// tab press produces: Group, Artifact, Name, then the package field.
	for range 3 {
		next, _ := model.Update(huh.NextField())
		model = next.(formModel)
	}

	before := model.state.PackageName
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	typed, ok := next.(formModel)
	if !ok {
		t.Fatalf("Update returned %T, want formModel", next)
	}

	if typed.state != model.state {
		t.Fatal("Update returned a copy of the state; the form must edit one value")
	}
	if typed.state.PackageName == before {
		t.Fatalf("the key did not reach the package field (still %q)", before)
	}
	if !strings.Contains(typed.state.PackageName, "x") {
		t.Errorf("PackageName = %q, want the typed x in it", typed.state.PackageName)
	}
	if typed.state.packageAuto {
		t.Error("typing into the package field left the derivation active")
	}
	if marker := typed.state.packageMarker(); marker != "" {
		t.Errorf("marker = %q, want none once a human owns the value", marker)
	}
}

// TestDerivedValueSurvivesFocusChanges proves the wrapper pushes the derived
// value back into the widget. huh's input keeps a second copy of the text, and
// every message that reaches it writes that copy into the state; a stale one
// would look like a human edit and stop the derivation.
func TestDerivedValueSurvivesFocusChanges(t *testing.T) {
	model := newTestModel(t)

	// Walk forward and back over the rows so the package input has been blurred
	// and focused the way navigation does, then land on the group row.
	for range 3 {
		next, _ := model.Update(huh.NextField())
		model = next.(formModel)
	}
	for range 3 {
		next, _ := model.Update(huh.PrevField())
		model = next.(formModel)
	}

	// Typing into the group row derives the package; the model, not this test,
	// must push that value into the package input.
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	model = next.(formModel)

	want := options.DerivePackageName(model.state.GroupID, model.state.ArtifactID)
	if model.state.PackageName != want {
		t.Fatalf("PackageName = %q, want the derived %q", model.state.PackageName, want)
	}

	// A message that reaches every field makes each input write its own copy
	// back into the state.
	next, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	model = next.(formModel)

	if model.state.PackageName != want {
		t.Errorf("PackageName = %q after the form refreshed, want %q", model.state.PackageName, want)
	}
	if !model.state.packageAuto {
		t.Error("the refresh stopped the derivation: the input held a stale value")
	}
}
