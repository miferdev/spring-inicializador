package tui

// Tests for US7.1: the form is a sequence of sections and one is on screen at a
// time. Project → Details → Build → Versions, enter walks forward across the
// section boundaries and esc walks back across them, and the frame says which
// section it is and how many there are. The model is driven with the messages the
// event loop delivers, so no test needs a terminal.

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	"github.com/miferdev/spring-inicializador/internal/options"
)

// frameTitle is the first line of a form frame: the section header.
func frameTitle(view string) string {
	return strings.TrimRight(strings.SplitN(view, "\n", 2)[0], " ")
}

// sectionKeysOf lists the rows of one section, in the order the form asks them.
func sectionKeysOf(section section) []Key {
	keys := make([]Key, 0, len(section.fields))
	for _, f := range section.fields {
		keys = append(keys, f.key)
	}
	return keys
}

// walkTo moves the focus to the row with the given key, one next-field step at a
// time, the way tab and enter move inside a section. A step that crosses a
// section boundary comes back as a command, which the harness delivers exactly as
// the event loop would.
func (h *harness) walkTo(key Key) {
	h.t.Helper()

	for guard := 0; h.focused() != key; guard++ {
		if guard > askedRows(h.model.project.fields)+1 {
			h.t.Fatalf("never reached %q; focus is %q", key, h.focused())
		}
		h.send(huh.NextField())
	}
}

// walkBackTo moves the focus back to the row with the given key one esc at a
// time, which is the only key that moves backwards.
func (h *harness) walkBackTo(key Key) {
	h.t.Helper()

	for guard := 0; h.focused() != key; guard++ {
		if guard > askedRows(h.model.project.fields)+1 {
			h.t.Fatalf("never reached %q going back; focus is %q", key, h.focused())
		}
		h.press(tea.KeyEsc)
	}
}

// assertFitsEighty checks that no line of the frame is wider than a small
// terminal: the whole point of one section per frame is that it fits.
func assertFitsEighty(t *testing.T, view string) {
	t.Helper()

	for i, line := range strings.Split(view, "\n") {
		if width := len([]rune(line)); width > 80 {
			t.Errorf("line %d is %d columns wide, want at most 80: %q", i, width, line)
		}
	}
}

// TestTheFormShowsFourSectionsInOrder pins the split: every row the form asks
// belongs to exactly one section, and the sections are Project, Details, Build and
// Versions.
func TestTheFormShowsFourSectionsInOrder(t *testing.T) {
	form := newProjectForm(fixtureCatalog(t), nil, Default())

	want := []struct {
		name string
		keys []Key
	}{
		{"Project", []Key{KeyName, KeyLocation}},
		{"Details", []Key{KeyGroupID, KeyPackageName}},
		{"Build", []Key{KeyBuild, KeyLanguage, KeyPackaging}},
		{"Versions", []Key{KeyJavaVersion, KeyBootVersion}},
	}

	if len(form.sections) != len(want) {
		t.Fatalf("form has %d sections, want %d", len(form.sections), len(want))
	}

	var flattened []Key
	seen := make(map[Key]int)
	for i, expected := range want {
		section := form.sections[i]
		if section.name != expected.name {
			t.Errorf("section %d name = %q, want %q", i, section.name, expected.name)
			continue
		}

		got := sectionKeysOf(section)
		if len(got) != len(expected.keys) {
			t.Errorf("%s holds %v, want %v", expected.name, got, expected.keys)
			continue
		}
		for j, key := range expected.keys {
			if got[j] != key {
				t.Errorf("%s row %d = %q, want %q", expected.name, j, got[j], key)
			}
			seen[key]++
			flattened = append(flattened, got[j])
		}
	}

	// A row in two sections would render twice; a row in none would be asked
	// nowhere. The flat list has to be the sections concatenated, or the two views
	// of the same form have already drifted apart.
	for key, count := range seen {
		if count != 1 {
			t.Errorf("row %q appears %d times across the sections, want once", key, count)
		}
	}
	if got, want := len(flattened), len(form.fields); got != want {
		t.Fatalf("sections hold %d rows, the flat list %d", got, want)
	}
	for i, f := range form.fields {
		if flattened[i] != f.key {
			t.Fatalf("row %d is %q in the flat list but %q in the sections", i, f.key, flattened[i])
		}
	}
}

// TestEnterOnProjectsLastRowOpensDetails is the crossing this story adds: the last
// row of a section moves to the next section instead of finishing the form.
func TestEnterOnProjectsLastRowOpensDetails(t *testing.T) {
	h := newSizedHarness(t, 80, 24)

	if got := frameTitle(h.model.View()); got != "Project 1/4" {
		t.Fatalf("frame starts with %q, want %q", got, "Project 1/4")
	}

	h.walkTo(KeyLocation)
	if got := h.focused(); got != KeyLocation {
		t.Fatalf("focus = %q, want Project's last row %q", got, KeyLocation)
	}

	h.press(tea.KeyEnter)

	if got := h.focused(); got != KeyGroupID {
		t.Errorf("focus after enter on Project's last row = %q, want Details' first row %q", got, KeyGroupID)
	}
	if got := frameTitle(h.model.View()); got != "Details 2/4" {
		t.Errorf("frame starts with %q after enter, want %q", got, "Details 2/4")
	}
	if h.model.screen != screenProject {
		t.Errorf("screen = %v after enter inside the form, want the project form", h.model.screen)
	}
}

// TestEnterOnVersionsLastRowOpensTheDependencies is the other side of the
// crossing: the last section still finishes the form, which opens the dependency
// list exactly as before (US3.1).
func TestEnterOnVersionsLastRowOpensTheDependencies(t *testing.T) {
	h := newSizedHarness(t, 80, 24)

	h.walkTo(KeyLocation)
	h.press(tea.KeyEnter)
	h.walkTo(KeyPackageName)
	h.press(tea.KeyEnter)
	h.walkTo(KeyPackaging)
	h.press(tea.KeyEnter)

	if got := h.focused(); got != KeyJavaVersion {
		t.Fatalf("focus = %q, want Versions' first row %q", got, KeyJavaVersion)
	}

	h.walkTo(KeyBootVersion)
	h.press(tea.KeyEnter)

	if h.model.screen != screenDependencies {
		t.Fatalf("screen = %v after enter on Versions' last row, want the dependencies", h.model.screen)
	}
}

// TestEscOnASectionsFirstRowGoesBack is the backwards crossing: esc on the first
// row of a section returns to the last row of the previous one.
func TestEscOnASectionsFirstRowGoesBack(t *testing.T) {
	cases := []struct {
		name      string
		leave     Key // the first row of the section to leave
		want      Key // the previous section's last row
		wantTitle string
	}{
		{name: "Details to Project", leave: KeyGroupID, want: KeyLocation, wantTitle: "Project 1/4"},
		{name: "Build to Details", leave: KeyBuild, want: KeyPackageName, wantTitle: "Details 2/4"},
		{name: "Versions to Build", leave: KeyJavaVersion, want: KeyPackaging, wantTitle: "Build 3/4"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newSizedHarness(t, 80, 24)

			h.walkTo(tc.leave)
			if got := h.focused(); got != tc.leave {
				t.Fatalf("focus = %q, want %q", got, tc.leave)
			}

			h.press(tea.KeyEsc)

			if got := h.focused(); got != tc.want {
				t.Errorf("focus after esc = %q, want %q", got, tc.want)
			}
			if got := frameTitle(h.model.View()); got != tc.wantTitle {
				t.Errorf("frame starts with %q after esc, want %q", got, tc.wantTitle)
			}
			if h.model.screen != screenProject {
				t.Errorf("screen = %v after esc inside the form, want the project form", h.model.screen)
			}
		})
	}
}

// TestEscOnTheFirstSectionsFirstRowStillLeaves is the carried-over abort: the first
// row of the first section has nowhere to go back to, so esc leaves without
// writing.
func TestEscOnTheFirstSectionsFirstRowStillLeaves(t *testing.T) {
	h := newSizedHarness(t, 80, 24)
	before := h.model.project.state

	// The interrupt command is inspected, not fed back: a real program stops on
	// it, while the harness would deliver it as a message for ever.
	next, cmd := h.model.Update(escKey())
	left, ok := next.(model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.model", next)
	}
	if left.outcome != Left {
		t.Errorf("outcome = %v after esc on the first row, want %v", left.outcome, Left)
	}
	if cmd == nil {
		t.Fatal("esc on the first row produced no command; the program would keep running")
	}
	if _, ok := cmd().(tea.InterruptMsg); !ok {
		t.Errorf("esc produced %T, want the interrupt that ends the program", cmd())
	}
	if left.project.state != before {
		t.Errorf("state changed on esc: %+v, want %+v", left.project.state, before)
	}
	if left.screen != screenProject {
		t.Errorf("screen = %v after esc, want the project form", left.screen)
	}
}

// TestTheSectionCounterIsRightAtEachStep walks the whole form and checks the
// header of every section, and that no frame is wider than the terminal. Nothing is
// answered here, so the counter names all four sections (US7.2 covers the answered
// ones).
func TestTheSectionCounterIsRightAtEachStep(t *testing.T) {
	h := newSizedHarness(t, 80, 24)

	titles := []string{"Project 1/4", "Details 2/4", "Build 3/4", "Versions 4/4"}
	sections := h.model.project.sections
	for i, title := range titles {
		if got := frameTitle(h.model.View()); got != title {
			t.Errorf("frame starts with %q, want %q", got, title)
		}
		assertFitsEighty(t, h.model.View())

		h.walkTo(sections[i].fields[len(sections[i].fields)-1].key)
		if i == len(titles)-1 {
			break
		}
		h.press(tea.KeyEnter)
	}

	if h.model.screen != screenProject {
		t.Errorf("screen = %v after walking the sections, want the project form", h.model.screen)
	}
}

// TestPackageDerivationSurvivesSectionChanges is the derivation rule across the
// section boundaries: leaving a section and coming back must not re-arm it, and an
// edit must keep winning wherever it happened.
func TestPackageDerivationSurvivesSectionChanges(t *testing.T) {
	t.Run("an untouched package follows the identifiers", func(t *testing.T) {
		h := newSizedHarness(t, 80, 24)

		h.typeText("acme.")
		h.walkTo(KeyGroupID)
		h.typeText("corp.")

		derivedPackage := func() string {
			state := h.model.project.state
			return options.DerivePackageName(state.GroupID, state.Name)
		}
		if got, want := h.model.project.state.PackageName, derivedPackage(); got != want {
			t.Fatalf("PackageName = %q, want the derived %q", got, want)
		}

		// Out of Details, then back in: the package did not stop following.
		h.walkTo(KeyPackageName)
		h.press(tea.KeyEnter)
		if got := h.focused(); got != KeyBuild {
			t.Fatalf("focus after enter = %q, want %q", got, KeyBuild)
		}
		h.press(tea.KeyEsc)
		if got := h.focused(); got != KeyPackageName {
			t.Fatalf("focus after esc = %q, want %q", got, KeyPackageName)
		}
		if !h.model.project.state.packageAuto {
			t.Fatal("the section round trip left the derivation off; it should still be on")
		}

		h.walkBackTo(KeyName)
		h.typeText("x")
		if got, want := h.model.project.state.PackageName, derivedPackage(); got != want {
			t.Errorf("PackageName = %q after the name changed, want the derived %q", got, want)
		}
	})

	t.Run("an edited package stops following", func(t *testing.T) {
		h := newSizedHarness(t, 80, 24)

		h.walkTo(KeyPackageName)
		h.typeText("z")
		edited := h.model.project.state.PackageName
		if h.model.project.state.packageAuto {
			t.Fatal("typing into the package field left the derivation active")
		}

		// Leave the section and come back: the edit still owns the value.
		h.press(tea.KeyEnter)
		h.press(tea.KeyEsc)
		if got := h.focused(); got != KeyPackageName {
			t.Fatalf("focus after the round trip = %q, want %q", got, KeyPackageName)
		}
		if h.model.project.state.packageAuto {
			t.Error("the section round trip re-armed the derivation")
		}

		h.walkBackTo(KeyName)
		h.typeText("y")
		if got := h.model.project.state.PackageName; got != edited {
			t.Errorf("PackageName = %q after the name changed, want the human value %q", got, edited)
		}
	})
}
