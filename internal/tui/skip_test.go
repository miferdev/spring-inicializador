package tui

// Tests for US7.2: the sections the flags answered are gone, including from the
// counter. A run that answered every row of Project opens on Details and says 1/3,
// a run that answered a section in the middle walks straight past it, and a run
// that answered everything never opens the wizard at all. The model is driven with
// the messages the event loop delivers, so no test needs a terminal.

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/miferdev/spring-inicializador/internal/options"
)

// projectAnswers are the flags that answer every row of Project: the name and where
// it goes (--name and --output).
func projectAnswers() Values {
	return Values{
		KeyName:     "billing",
		KeyLocation: "/tmp/out",
	}
}

// detailsAnswers are the flags that answer every row of Details.
func detailsAnswers() Values {
	return Values{
		KeyGroupID:     "com.acme",
		KeyPackageName: "com.acme.billing",
	}
}

// buildAnswers are the flags that answer every row of Build.
func buildAnswers() Values {
	return Values{
		KeyBuild:     options.BuildGradle,
		KeyLanguage:  options.LanguageKotlin,
		KeyPackaging: options.PackagingWar,
	}
}

// versionsAnswers are the flags that answer every row of Versions.
func versionsAnswers() Values {
	return Values{
		KeyJavaVersion: "21",
		KeyBootVersion: "4.0.9.RELEASE",
	}
}

// merge is the union of some answer sets, written out because the order of a map
// literal is not something a test should depend on twice.
func merge(sets ...Values) Values {
	all := Values{}
	for _, set := range sets {
		for key, value := range set {
			all[key] = value
		}
	}
	return all
}

// TestASkippedProjectLeavesTheCounterAtOneOfThree: every Project row was answered,
// so Details is the first of the three sections that will be shown, and there is no
// earlier section for esc to go back to.
func TestASkippedProjectLeavesTheCounterAtOneOfThree(t *testing.T) {
	h := newProvidedHarness(t, 80, 24, projectAnswers())

	if got := frameTitle(h.model.View()); got != "Details 1/3" {
		t.Fatalf("frame starts with %q, want %q", got, "Details 1/3")
	}
	if got := h.focused(); got != KeyGroupID {
		t.Fatalf("focus = %q, want Details' first row %q", got, KeyGroupID)
	}
	if got := h.model.project.firstKey; got != KeyGroupID {
		t.Errorf("firstKey = %q, want %q", got, KeyGroupID)
	}

	// Leaving: the interrupt command is inspected, not fed back, because a real
	// program stops on it.
	next, cmd := h.model.Update(escKey())
	left, ok := next.(model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.model", next)
	}
	if left.outcome != Left {
		t.Errorf("outcome = %v after esc on the first row shown, want %v", left.outcome, Left)
	}
	if cmd == nil {
		t.Fatal("esc on the first row shown produced no command; the program would keep running")
	}
	if _, ok := cmd().(tea.InterruptMsg); !ok {
		t.Errorf("esc produced %T, want the interrupt that ends the program", cmd())
	}
}

// TestASkippedProjectAndDetailsLeaveOneSection: only Versions is left, it is the
// whole form, and enter still opens the dependency pane.
func TestASkippedProjectAndDetailsLeaveOneSection(t *testing.T) {
	h := newProvidedHarness(t, 80, 24, merge(projectAnswers(), detailsAnswers(), buildAnswers()))

	if got := frameTitle(h.model.View()); got != "Versions 1/1" {
		t.Fatalf("frame starts with %q, want %q", got, "Versions 1/1")
	}
	if got := h.focused(); got != KeyJavaVersion {
		t.Fatalf("focus = %q, want Versions' first row %q", got, KeyJavaVersion)
	}

	h.walkTo(KeyBootVersion)
	h.press(tea.KeyEnter)

	if h.model.screen != screenDependencies {
		t.Fatalf("screen = %v after enter on the last row, want the dependencies", h.model.screen)
	}
}

// TestASkippedMiddleSectionKeepsTheCrossings: Details was answered, so Project is
// 1/3 and Build 2/3, and the two crossings straddle the section that is not there.
func TestASkippedMiddleSectionKeepsTheCrossings(t *testing.T) {
	h := newProvidedHarness(t, 80, 24, detailsAnswers())

	if got := frameTitle(h.model.View()); got != "Project 1/3" {
		t.Fatalf("frame starts with %q, want %q", got, "Project 1/3")
	}

	h.walkTo(KeyLocation)
	h.press(tea.KeyEnter)

	if got := h.focused(); got != KeyBuild {
		t.Errorf("focus after enter on Project's last row = %q, want Build's first row %q", got, KeyBuild)
	}
	if got := frameTitle(h.model.View()); got != "Build 2/3" {
		t.Errorf("frame starts with %q, want %q", got, "Build 2/3")
	}

	h.press(tea.KeyEsc)

	if got := h.focused(); got != KeyLocation {
		t.Errorf("focus after esc = %q, want Project's last row %q", got, KeyLocation)
	}
	if got := frameTitle(h.model.View()); got != "Project 1/3" {
		t.Errorf("frame starts with %q after esc, want %q", got, "Project 1/3")
	}
}

// TestEveryRowAnsweredNeverOpensTheWizard keeps the carried-over contract: with
// nothing left to ask there is no form at all, and the options are the flags'.
func TestEveryRowAnsweredNeverOpensTheWizard(t *testing.T) {
	cat := fixtureCatalog(t)
	provided := merge(projectAnswers(), detailsAnswers(), buildAnswers(), versionsAnswers())

	if Asks(cat, provided) {
		t.Fatal("Asks = true although every row was answered")
	}

	project := newProjectForm(cat, provided, Default())
	if project.form != nil {
		t.Error("the form was built although every row was answered")
	}
	if project.asks() {
		t.Error("project.asks() = true although every row was answered")
	}

	opts := optionsFromState(cat, project.state)
	if opts.GroupID != "com.acme" || opts.Build != options.BuildGradle || opts.JavaVersion != "21" {
		t.Errorf("options = %+v, want the flag values", opts)
	}
	if opts.ArtifactID != "billing" || opts.Name != "billing" {
		t.Errorf("options name = (%q, %q), want the flag value billing", opts.ArtifactID, opts.Name)
	}
	if project.state.Location != "/tmp/out" {
		t.Errorf("Location = %q, want the --output value", project.state.Location)
	}
}

// TestNothingAnsweredStillNumbersFourSections is the untouched case: nothing was
// answered, so the counter is the whole form.
func TestNothingAnsweredStillNumbersFourSections(t *testing.T) {
	h := newProvidedHarness(t, 80, 24, nil)

	want := []string{"Project 1/4", "Details 2/4", "Build 3/4", "Versions 4/4"}
	for i, title := range want {
		if got := frameTitle(h.model.View()); got != title {
			t.Errorf("frame starts with %q, want %q", got, title)
		}
		if i == len(want)-1 {
			break
		}

		section := h.model.project.sections[i]
		h.walkTo(section.fields[len(section.fields)-1].key)
		h.press(tea.KeyEnter)
	}
}

// TestDerivationFollowsAFlaggedGroupAndTheTypedName: with Details, Build and
// Versions answered, the group comes from a flag and the name from the Project
// section, and the package follows both until a human edits it.
func TestDerivationFollowsAFlaggedGroupAndTheTypedName(t *testing.T) {
	provided := merge(Values{KeyGroupID: "com.acme"}, buildAnswers(), versionsAnswers())
	h := newProvidedHarness(t, 80, 24, provided)

	if got := frameTitle(h.model.View()); got != "Project 1/2" {
		t.Fatalf("frame starts with %q, want %q", got, "Project 1/2")
	}

	h.typeText("acme.")

	derivedPackage := func() string {
		state := h.model.project.state
		return options.DerivePackageName(state.GroupID, state.Name)
	}
	if got, want := h.model.project.state.PackageName, derivedPackage(); got != want {
		t.Fatalf("PackageName = %q, want the derived %q", got, want)
	}

	// A human edit of the package stops it, exactly as with four sections on
	// screen: the field is one section away now.
	h.walkTo(KeyPackageName)
	h.typeText("z")
	edited := h.model.project.state.PackageName
	if h.model.project.state.packageAuto {
		t.Fatal("typing into the package row left the derivation active")
	}

	h.walkBackTo(KeyName)
	h.typeText("y")
	if got := h.model.project.state.PackageName; got != edited {
		t.Errorf("PackageName = %q after the name changed, want the human value %q", got, edited)
	}
}
