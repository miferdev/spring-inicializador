package tui

// Tests for US7.2: the sections the flags answered are gone, including from the
// counter. A run that answered every row of Identity opens on Build and says 1/2,
// a run that answered a section in the middle walks straight past it, and a run
// that answered everything never opens the wizard at all. The model is driven with
// the messages the event loop delivers, so no test needs a terminal.

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/miferdev/spring-inicializador/internal/options"
)

// identityAnswers are the flags that answer every row of Identity.
func identityAnswers() Values {
	return Values{
		KeyGroupID:     "com.acme",
		KeyArtifactID:  "billing",
		KeyName:        "billing",
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

// TestASkippedIdentityLeavesTheCounterAtOneOfTwo: every Identity row was answered,
// so Build is the first of the two sections that will be shown, and there is no
// earlier section for esc to go back to.
func TestASkippedIdentityLeavesTheCounterAtOneOfTwo(t *testing.T) {
	h := newProvidedHarness(t, 80, 24, identityAnswers())

	if got := frameTitle(h.model.View()); got != "Build 1/2" {
		t.Fatalf("frame starts with %q, want %q", got, "Build 1/2")
	}
	if got := h.focused(); got != KeyBuild {
		t.Fatalf("focus = %q, want Build's first row %q", got, KeyBuild)
	}
	if got := h.model.project.firstKey; got != KeyBuild {
		t.Errorf("firstKey = %q, want %q", got, KeyBuild)
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

// TestASkippedIdentityAndBuildLeaveOneSection: only Versions is left, it is the
// whole form, and enter still opens the dependency pane.
func TestASkippedIdentityAndBuildLeaveOneSection(t *testing.T) {
	h := newProvidedHarness(t, 80, 24, merge(identityAnswers(), buildAnswers()))

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

// TestASkippedMiddleSectionKeepsTheCrossings: Build was answered, so Identity is
// 1/2 and Versions 2/2, and the two crossings straddle the section that is not
// there.
func TestASkippedMiddleSectionKeepsTheCrossings(t *testing.T) {
	h := newProvidedHarness(t, 80, 24, buildAnswers())

	if got := frameTitle(h.model.View()); got != "Identity 1/2" {
		t.Fatalf("frame starts with %q, want %q", got, "Identity 1/2")
	}

	h.walkTo(KeyPackageName)
	h.press(tea.KeyEnter)

	if got := h.focused(); got != KeyJavaVersion {
		t.Errorf("focus after enter on Identity's last row = %q, want Versions' first row %q", got, KeyJavaVersion)
	}
	if got := frameTitle(h.model.View()); got != "Versions 2/2" {
		t.Errorf("frame starts with %q, want %q", got, "Versions 2/2")
	}

	h.press(tea.KeyEsc)

	if got := h.focused(); got != KeyPackageName {
		t.Errorf("focus after esc = %q, want Identity's last row %q", got, KeyPackageName)
	}
	if got := frameTitle(h.model.View()); got != "Identity 1/2" {
		t.Errorf("frame starts with %q after esc, want %q", got, "Identity 1/2")
	}
}

// TestEveryRowAnsweredNeverOpensTheWizard keeps the carried-over contract: with
// nothing left to ask there is no form at all, and the options are the flags'.
func TestEveryRowAnsweredNeverOpensTheWizard(t *testing.T) {
	cat := fixtureCatalog(t)
	provided := merge(identityAnswers(), buildAnswers(), versionsAnswers())

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
}

// TestNothingAnsweredStillNumbersThreeSections is the untouched case: nothing was
// answered, so the counter is the whole form.
func TestNothingAnsweredStillNumbersThreeSections(t *testing.T) {
	h := newProvidedHarness(t, 80, 24, nil)

	want := []string{"Identity 1/3", "Build 2/3", "Versions 3/3"}
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

// TestDerivationStillWorksWithOnlyIdentityShown: with Build and Versions answered,
// Identity is the whole wizard and the package still follows group and artifact
// until a human edits it.
func TestDerivationStillWorksWithOnlyIdentityShown(t *testing.T) {
	h := newProvidedHarness(t, 80, 24, merge(buildAnswers(), versionsAnswers()))

	if got := frameTitle(h.model.View()); got != "Identity 1/1" {
		t.Fatalf("frame starts with %q, want %q", got, "Identity 1/1")
	}

	h.typeText("acme.")
	h.walkTo(KeyArtifactID)
	h.typeText("billing")

	derived := func() string {
		return options.DerivePackageName(h.model.project.state.GroupID, h.model.project.state.ArtifactID)
	}
	if got, want := h.model.project.state.PackageName, derived(); got != want {
		t.Fatalf("PackageName = %q, want the derived %q", got, want)
	}

	// A human edit stops it, exactly as with three sections on screen.
	h.walkTo(KeyPackageName)
	h.typeText("z")
	edited := h.model.project.state.PackageName
	if h.model.project.state.packageAuto {
		t.Fatal("typing into the package field left the derivation active")
	}

	h.walkBackTo(KeyArtifactID)
	h.typeText("y")
	if got := h.model.project.state.PackageName; got != edited {
		t.Errorf("PackageName = %q after the artifact changed, want the human value %q", got, edited)
	}
}
