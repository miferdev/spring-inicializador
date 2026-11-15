package tui

// Tests for US3.1's list and search: the rows come from the parsed fixture, the
// ranking follows the weight order the story fixes (id, then display name, then
// category), and a miss explains itself instead of swallowing the query. They
// drive the real list component, so no test needs a terminal.

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/miferdev/spring-inicializador/internal/catalog"
)

// ranked applies the search to the catalog and returns the rows it would show,
// best first.
func ranked(cat *catalog.Catalog, term string) []catalog.Dependency {
	positions := rankDependencies(term, cat.Dependencies)
	deps := make([]catalog.Dependency, 0, len(positions))
	for _, position := range positions {
		deps = append(deps, cat.Dependencies[position])
	}
	return deps
}

// found reports where the dependency with id appears in the ranking, or -1.
func found(ranked []catalog.Dependency, id string) int {
	for i, dep := range ranked {
		if dep.ID == id {
			return i
		}
	}
	return -1
}

// TestDependencyListIsBuiltFromTheCatalog proves the screen is the parsed
// fixture, all of it, and that a row renders the dependency name with its
// category as the muted label.
func TestDependencyListIsBuiltFromTheCatalog(t *testing.T) {
	h := newHarness(t)
	h.openDependencies()

	deps := h.model.deps
	if got, want := len(deps.list.Items()), len(h.cat.Dependencies); got != want {
		t.Fatalf("list rows = %d, want %d (the whole flat catalog)", got, want)
	}
	if len(h.cat.Dependencies) == 0 {
		t.Fatal("the fixture has no dependencies")
	}

	target := h.cat.Dependencies[0]
	var row strings.Builder
	dependencyDelegate{theme: Default()}.Render(&row, deps.list, 0, dependencyItem{dependency: target})

	if !strings.Contains(row.String(), target.Name) {
		t.Errorf("row %q does not show the dependency name %q", row.String(), target.Name)
	}
	if !strings.Contains(row.String(), target.Category) {
		t.Errorf("row %q does not show the category %q", row.String(), target.Category)
	}
	if !strings.HasPrefix(row.String(), ">") {
		t.Errorf("row %q is the focused row but carries no focus marker", row.String())
	}

	view := deps.view()
	if !strings.Contains(view, target.Name) {
		t.Errorf("the screen does not render the first row %q", target.Name)
	}
}

// TestTypingFiltersLive covers the story's first criterion: / focuses the filter
// and every keystroke narrows the list, through the component's own key path.
func TestTypingFiltersLive(t *testing.T) {
	h := newHarness(t)
	h.openDependencies()

	before := len(h.model.deps.list.VisibleItems())
	if before != len(h.cat.Dependencies) {
		t.Fatalf("visible rows before searching = %d, want all %d", before, len(h.cat.Dependencies))
	}

	h.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if !h.model.deps.list.SettingFilter() {
		t.Fatal("/ did not focus the filter")
	}

	h.typeText("postgres")

	visible := h.model.deps.list.VisibleItems()
	if len(visible) >= before {
		t.Errorf("visible rows after typing = %d, want fewer than %d", len(visible), before)
	}
	if len(visible) == 0 {
		t.Fatal("postgres matched nothing")
	}
	if got := h.model.deps.list.FilterValue(); got != "postgres" {
		t.Errorf("filter = %q, want postgres", got)
	}
	if view := h.model.deps.view(); !strings.Contains(view, "postgres") {
		t.Errorf("the query is not on screen; view:\n%s", view)
	}
	if first, ok := visible[0].(dependencyItem); !ok || first.dependency.ID != "postgresql" {
		t.Errorf("first result = %#v, want the postgresql driver", visible[0])
	}

	// While the query is being typed a space is a character, not a toggle: the
	// key belongs to the filter until the list is browsing again (US3.2).
	h.send(spaceKey())
	if got := h.model.deps.list.FilterValue(); got != "postgres " {
		t.Errorf("filter = %q after space, want it typed into the query", got)
	}
	if got := len(h.model.deps.list.VisibleItems()); got != len(visible) {
		t.Errorf("space changed the visible rows: %d, want %d", got, len(visible))
	}
	if got := h.model.deps.chosen(); len(got) != 0 {
		t.Errorf("space checked %v while the query was being typed", got)
	}
}

// TestPostgresRanksTheDriverInTheTopThree is the story's example: typing half a
// memory finds the driver whose id the user never saw.
func TestPostgresRanksTheDriverInTheTopThree(t *testing.T) {
	cat := fixtureCatalog(t)

	ranked := ranked(cat, "postgres")
	position := found(ranked, "postgresql")
	if position < 0 {
		t.Fatalf("postgres does not match postgresql; ranked %v", ranked)
	}
	if position >= 3 {
		t.Errorf("postgresql ranks #%d for postgres, want a top three result", position+1)
	}
	if ranked[0].ID != "postgresql" {
		t.Errorf("first result = %q, want postgresql: its id starts with the term", ranked[0].ID)
	}
	if ranked[0].Category != "SQL" {
		t.Errorf("postgresql category = %q, want SQL", ranked[0].Category)
	}
}

// TestRankingMatchesIDThenNameThenCategory pins the weight order: an id hit
// outranks a name hit, which outranks a category hit.
func TestRankingMatchesIDThenNameThenCategory(t *testing.T) {
	cat := fixtureCatalog(t)

	// An exact id is the strongest answer there is.
	if got := ranked(cat, "data-jpa"); len(got) == 0 || got[0].ID != "data-jpa" {
		t.Errorf("data-jpa ranked %v, want the data-jpa dependency first", got)
	}

	// jdbc matches only through its category: neither its id nor its name
	// contains "sql", so it proves the category is searched.
	all := ranked(cat, "sql")
	if found(all, "jdbc") < 0 {
		t.Fatalf("sql does not match jdbc by category; ranked %d rows", len(all))
	}

	// Field weight dominates: once a row that matched only through its category
	// appears, no row that matched by id or name may follow it.
	seenCategoryOnly := false
	for i, dep := range all {
		byText := fieldHit(dep.ID, "sql") || fieldHit(dep.Name, "sql")
		switch {
		case seenCategoryOnly && byText:
			t.Errorf("row #%d %q matches by id or name but ranks below a category-only row", i+1, dep.ID)
		case !byText:
			seenCategoryOnly = true
		}
	}

	// An empty query is not a search: every row stays, in catalog order.
	if got, want := rankDependencies("  ", cat.Dependencies), len(cat.Dependencies); len(got) != want {
		t.Errorf("blank query returned %d rows, want all %d", len(got), want)
	}
}

// fieldHit reports whether a field contains the term at all, which is the
// coarsest question the ranker asks.
func fieldHit(value, term string) bool {
	return strings.Contains(strings.ToLower(value), term)
}

// TestEmptyResultKeepsTheQueryForEditing covers the last acceptance criterion: a
// miss says so and leaves the query on screen, editable, instead of dropping it.
func TestEmptyResultKeepsTheQueryForEditing(t *testing.T) {
	h := newHarness(t)
	h.openDependencies()

	h.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	h.typeText("zzzz")

	if got := len(h.model.deps.list.VisibleItems()); got != 0 {
		t.Fatalf("visible rows = %d, want none", got)
	}

	view := h.model.deps.view()
	if !strings.Contains(view, "No dependency matches") {
		t.Errorf("the empty result is not explained; view:\n%s", view)
	}
	if !strings.Contains(view, "zzzz") {
		t.Errorf("the query is not visible; view:\n%s", view)
	}

	// Enter must not throw the query away: the list would reset the filter when
	// nothing matched, which would erase what the user is still editing.
	h.press(tea.KeyEnter)
	if got := h.model.deps.list.FilterValue(); got != "zzzz" {
		t.Errorf("filter = %q after enter on an empty result, want it kept", got)
	}
	if h.model.screen != screenDependencies {
		t.Errorf("screen = %v after enter on an empty result, want the list", h.model.screen)
	}

	// The query is still editable: a backspace reaches it.
	h.press(tea.KeyBackspace)
	if got := h.model.deps.list.FilterValue(); got != "zzz" {
		t.Errorf("filter = %q after backspace, want zzz", got)
	}

	// esc clears the miss and only then leaves.
	h.press(tea.KeyEsc)
	if got := h.model.deps.list.FilterValue(); got != "" {
		t.Errorf("filter = %q after esc, want it cleared", got)
	}
	if h.model.screen != screenDependencies {
		t.Errorf("screen = %v after esc on a query, want the list", h.model.screen)
	}
}
