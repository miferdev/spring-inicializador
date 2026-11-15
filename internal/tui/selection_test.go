package tui

// Tests for US3.2: picking dependencies. A row is checked with space, the footer
// says how many are picked, the set survives every filter change and the form
// round trip, and finishing hands exactly the checked set to the request in
// catalog order. No test needs a terminal or a network.

import (
	"reflect"
	"strings"
	"testing"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/miferdev/spring-inicializador/internal/catalog"
)

// spaceKey is the message a terminal sends for the space bar. bubbletea keeps
// the rune, which is what lets a text input type a space while the query is
// being edited.
func spaceKey() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
}

// positionOf is the catalog position of a dependency id, or -1.
func positionOf(cat *catalog.Catalog, id string) int {
	for i, dependency := range cat.Dependencies {
		if dependency.ID == id {
			return i
		}
	}
	return -1
}

// focusRow puts the cursor on a dependency by pressing up and down, the way a
// person does, and fails when the row is not there.
func (h *harness) focusRow(id string) {
	h.t.Helper()

	want := positionOf(h.cat, id)
	if want < 0 {
		h.t.Fatalf("the fixture has no dependency %q", id)
	}

	have := h.model.deps.list.Index()
	for ; have < want; have++ {
		h.press(tea.KeyDown)
	}
	for ; have > want; have-- {
		h.press(tea.KeyUp)
	}

	if item, ok := h.model.deps.list.SelectedItem().(dependencyItem); !ok || item.dependency.ID != id {
		h.t.Fatalf("focus = %#v, want the row %q", h.model.deps.list.SelectedItem(), id)
	}
}

// TestSpaceTogglesTheFocusedRowAndStays is the story's core: space flips the row
// under the cursor and the cursor does not move, because the user is picking
// several rows in a row.
func TestSpaceTogglesTheFocusedRowAndStays(t *testing.T) {
	h := newHarness(t)
	h.openDependencies()

	if got := h.model.deps.footer(); !strings.Contains(got, "0 selected") {
		t.Fatalf("footer = %q, want it to start at 0 selected", got)
	}

	h.focusRow("web")
	row := h.model.deps.list.Index()

	h.send(spaceKey())

	if got := h.model.deps.chosen(); !reflect.DeepEqual(got, []string{"web"}) {
		t.Errorf("checked = %v, want web", got)
	}
	if got := h.model.deps.list.Index(); got != row {
		t.Errorf("space moved the cursor to row %d, want it to stay on %d", got, row)
	}
	if view := h.model.deps.view(); !strings.Contains(view, "[x] Spring Web") {
		t.Errorf("the checked row is not marked; view:\n%s", view)
	}
	if got := h.model.deps.footer(); !strings.Contains(got, "1 selected: web") {
		t.Errorf("footer = %q, want 1 selected: web", got)
	}

	h.send(spaceKey())

	if got := h.model.deps.chosen(); len(got) != 0 {
		t.Errorf("checked = %v after a second space, want none", got)
	}
	if got := h.model.deps.list.Index(); got != row {
		t.Errorf("the second space moved the cursor to row %d, want %d", got, row)
	}
	if view := h.model.deps.view(); !strings.Contains(view, "[ ] Spring Web") {
		t.Errorf("the unchecked row is still marked; view:\n%s", view)
	}
}

// TestFooterCountsSelections pins the footer wording: the count is always there,
// and the ids follow in catalog order once something is checked.
func TestFooterCountsSelections(t *testing.T) {
	h := newHarness(t)
	h.openDependencies()

	if got, want := h.model.deps.footer(), "206 dependencies  0 selected"; got != want {
		t.Fatalf("footer = %q, want %q", got, want)
	}

	h.focusRow("web")
	h.send(spaceKey())
	h.focusRow("data-jpa")
	h.send(spaceKey())

	// data-jpa was checked last but the request, and therefore the footer, is in
	// catalog order.
	if got, want := h.model.deps.footer(), "206 dependencies  2 selected: web, data-jpa"; got != want {
		t.Errorf("footer = %q, want %q", got, want)
	}

	// A search does not hide the selection.
	h.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	h.typeText("sql")
	h.press(tea.KeyEnter)
	if got := h.model.deps.footer(); !strings.Contains(got, "2 selected: web, data-jpa") {
		t.Errorf("footer = %q while searching, want the selection kept", got)
	}
}

// TestFooterTruncatesTheIDsButNotTheCount proves the truncation rule: whole ids
// only, "..." at the cut, the exact count kept, and one line that fits.
func TestFooterTruncatesTheIDsButNotTheCount(t *testing.T) {
	cat := fixtureCatalog(t)

	ids := make([]string, 0, 20)
	for _, dependency := range cat.Dependencies[:20] {
		ids = append(ids, dependency.ID)
	}

	screen := newDependenciesModel(cat, ids, 60, 20, Default())
	footer := screen.footer()

	if !strings.Contains(footer, "20 selected:") {
		t.Errorf("footer = %q, want the exact count even when the ids are cut", footer)
	}
	if !strings.HasSuffix(footer, "...") {
		t.Errorf("footer = %q, want the cut marked with ...", footer)
	}
	if width := ansi.StringWidth(footer); width > 60 {
		t.Errorf("footer is %d cells wide, want at most 60: %q", width, footer)
	}
	for _, r := range footer {
		if r > unicode.MaxASCII {
			t.Errorf("footer %q is not ASCII (found %q)", footer, r)
			break
		}
	}

	// The ids are a catalog-order prefix of the selection: nothing is reordered
	// and nothing is invented to fill the line.
	parts := strings.SplitN(footer, "20 selected: ", 2)
	if len(parts) != 2 {
		t.Fatalf("footer = %q, want the count followed by the ids", footer)
	}
	shown := strings.Split(strings.TrimSuffix(parts[1], ", ..."), ", ")
	for i, id := range shown {
		if id != ids[i] {
			t.Errorf("id #%d shown = %q, want %q in catalog order", i+1, id, ids[i])
		}
	}

	// Wide enough for all twenty ids: no ellipsis, nothing dropped.
	wide := newDependenciesModel(cat, ids, 400, 20, Default())
	if got := wide.footer(); strings.Contains(got, "...") {
		t.Errorf("footer at 400 columns = %q, want every id", got)
	}
}

// TestSelectionSurvivesFiltering is the acceptance criterion the brief calls out:
// a filter never drops or duplicates the set.
func TestSelectionSurvivesFiltering(t *testing.T) {
	h := newHarness(t)
	h.openDependencies()

	// Find the driver by half a memory, apply the filter, then check it: space is
	// a character while the query is being typed.
	h.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	h.typeText("postgres")
	h.press(tea.KeyEnter)
	h.send(spaceKey())
	if got := h.model.deps.chosen(); !reflect.DeepEqual(got, []string{"postgresql"}) {
		t.Fatalf("checked = %v, want postgresql", got)
	}

	// Clearing the query keeps the set.
	h.press(tea.KeyEsc)
	if got := h.model.deps.chosen(); !reflect.DeepEqual(got, []string{"postgresql"}) {
		t.Fatalf("checked = %v after clearing the query, want postgresql", got)
	}

	// A different filter neither drops it nor duplicates it.
	h.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	h.typeText("sql")
	h.press(tea.KeyEnter)
	if got := h.model.deps.chosen(); !reflect.DeepEqual(got, []string{"postgresql"}) {
		t.Fatalf("checked = %v while filtered to sql, want postgresql", got)
	}
	if view := h.model.deps.view(); !strings.Contains(view, "[x] PostgreSQL Driver") {
		t.Errorf("the checked row lost its mark while filtering; view:\n%s", view)
	}
	if got := h.model.deps.footer(); !strings.Contains(got, "1 selected: postgresql") {
		t.Errorf("footer = %q, want the selection kept", got)
	}
}

// TestEnterReturnsTheCheckedSetInCatalogOrder proves the request two identical
// selections produce is identical: ids leave in catalog order, not in the order
// they were checked.
func TestEnterReturnsTheCheckedSetInCatalogOrder(t *testing.T) {
	h := newHarness(t)
	h.openDependencies()

	// h2 is #50 in the catalog, web is #7 and data-jpa is #41.
	for _, id := range []string{"h2", "web", "data-jpa"} {
		h.focusRow(id)
		h.send(spaceKey())
	}

	_, cmd := h.model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	msgs := commandMessages(cmd)
	if len(msgs) == 0 {
		t.Fatal("enter on the dependency screen produced no message; the run would hang")
	}
	if _, ok := msgs[0].(tea.QuitMsg); !ok {
		t.Fatalf("enter produced %T, want tea.QuitMsg", msgs[0])
	}

	got := h.model.finished(h.cat).Deps
	want := []string{"web", "data-jpa", "h2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Deps = %v, want %v in catalog order", got, want)
	}
}

// TestSeededDependenciesArriveChecked is the flag parity the brief asks for: the
// ids --deps already answered open checked, so the screen answers that flag the
// way the form answers its rows.
func TestSeededDependenciesArriveChecked(t *testing.T) {
	// The flag order is deliberately not the catalog order.
	h := newSeededHarness(t, "data-jpa", "web")
	h.openDependencies()

	if got, want := h.model.deps.chosen(), []string{"web", "data-jpa"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("checked = %v, want %v", got, want)
	}
	if got := h.model.deps.footer(); !strings.Contains(got, "2 selected: web, data-jpa") {
		t.Errorf("footer = %q, want both seeded ids", got)
	}

	// Both rows render as checked, and a row nobody picked does not.
	delegate := dependencyDelegate{theme: Default(), selected: h.model.deps.selected}
	for _, id := range []string{"web", "data-jpa"} {
		var row strings.Builder
		delegate.Render(&row, h.model.deps.list, 0, dependencyItem{dependency: mustDependency(t, h.cat, id)})
		if !strings.Contains(row.String(), "[x]") {
			t.Errorf("row %q is not marked as checked", row.String())
		}
	}
	var untouched strings.Builder
	delegate.Render(&untouched, h.model.deps.list, 0, dependencyItem{dependency: mustDependency(t, h.cat, "h2")})
	if !strings.Contains(untouched.String(), "[ ]") {
		t.Errorf("row %q is marked checked although nobody picked it", untouched.String())
	}

	// Finishing without touching a key returns exactly the seeded set.
	if got, want := h.model.finished(h.cat).Deps, []string{"web", "data-jpa"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Deps = %v, want %v", got, want)
	}
}

// TestSelectionSurvivesTheFormRoundTrip: esc back to the form keeps the set, and
// the list shows it again when the form is finished.
func TestSelectionSurvivesTheFormRoundTrip(t *testing.T) {
	h := newHarness(t)
	h.openDependencies()

	h.focusRow("web")
	h.send(spaceKey())
	before := h.model.deps.chosen()

	h.press(tea.KeyEsc) // no query: back to the form
	if h.model.screen != screenProject {
		t.Fatalf("screen = %v after esc, want the project form", h.model.screen)
	}

	h.openDependencies()
	if got := h.model.deps.chosen(); !reflect.DeepEqual(got, before) {
		t.Errorf("checked = %v after the round trip, want %v", got, before)
	}
	if view := h.model.deps.view(); !strings.Contains(view, "[x] Spring Web") {
		t.Errorf("the checked row lost its mark after the round trip; view:\n%s", view)
	}
	if got := h.model.deps.footer(); !strings.Contains(got, "1 selected: web") {
		t.Errorf("footer = %q after the round trip, want the selection kept", got)
	}
}

// mustDependency looks a dependency up in the fixture.
func mustDependency(t *testing.T, cat *catalog.Catalog, id string) catalog.Dependency {
	t.Helper()

	dependency, ok := cat.Lookup(id)
	if !ok {
		t.Fatalf("the fixture has no dependency %q", id)
	}
	return dependency
}

// TestSeededIDsTheCatalogDoesNotPublishSurvive pins what --deps means for a typo:
// the id still reaches the request, exactly as it does without the screen, so the
// server can explain it instead of the wizard quietly dropping it.
func TestSeededIDsTheCatalogDoesNotPublishSurvive(t *testing.T) {
	h := newSeededHarness(t, "webb", "web", "adbc")
	h.openDependencies()

	// Known ids keep catalog order; the rest follow in lexical order, because
	// they have no catalog position to sort by.
	if got, want := h.model.deps.chosen(), []string{"web", "adbc", "webb"}; !reflect.DeepEqual(got, want) {
		t.Errorf("checked = %v, want %v", got, want)
	}
	if got := h.model.deps.footer(); !strings.Contains(got, "3 selected: web, adbc, webb") {
		t.Errorf("footer = %q, want the flag ids kept", got)
	}
	if got, want := h.model.finished(h.cat).Deps, []string{"web", "adbc", "webb"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Deps = %v, want %v", got, want)
	}
}
