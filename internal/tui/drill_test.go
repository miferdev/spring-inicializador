package tui

// Tests for US4.2: the category level remembers where the user left it. The root
// cursor comes back with esc, selections are global across levels, and a search
// started inside a category returns to that category before the root. No test
// needs a terminal or a network.

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// openRootCategory moves the root cursor to a category and enters it.
func openRootCategory(t *testing.T, screen dependenciesModel, name string) dependenciesModel {
	t.Helper()

	if screen.level != levelCategories {
		t.Fatalf("level = %v, want the category root before opening %q", screen.level, name)
	}

	want := -1
	for i, item := range screen.list.Items() {
		if row, ok := item.(categoryItem); ok && row.category.Name == name {
			want = i
			break
		}
	}
	if want < 0 {
		t.Fatalf("the root is not showing the category %q", name)
	}

	have := screen.list.Index()
	for ; have < want; have++ {
		screen = drive(t, screen, tea.KeyMsg{Type: tea.KeyDown})
	}
	for ; have > want; have-- {
		screen = drive(t, screen, tea.KeyMsg{Type: tea.KeyUp})
	}
	screen = drive(t, screen, tea.KeyMsg{Type: tea.KeyEnter})

	if screen.level != levelDependencies || screen.open != name {
		t.Fatalf("enter landed on level %v open %q, want %q", screen.level, screen.open, name)
	}
	return screen
}

// moveToDependency puts the cursor on a dependency the open category shows.
func moveToDependency(t *testing.T, screen dependenciesModel, id string) dependenciesModel {
	t.Helper()

	want := -1
	for i, item := range screen.list.VisibleItems() {
		if row, ok := item.(dependencyItem); ok && row.dependency.ID == id {
			want = i
			break
		}
	}
	if want < 0 {
		t.Fatalf("the screen is not showing the dependency %q", id)
	}

	have := screen.list.Index()
	for ; have < want; have++ {
		screen = drive(t, screen, tea.KeyMsg{Type: tea.KeyDown})
	}
	for ; have > want; have-- {
		screen = drive(t, screen, tea.KeyMsg{Type: tea.KeyUp})
	}
	return screen
}

// checkDependency focuses a dependency and checks it.
func checkDependency(t *testing.T, screen dependenciesModel, id string) dependenciesModel {
	t.Helper()

	screen = moveToDependency(t, screen, id)
	screen = drive(t, screen, spaceKey())
	if _, ok := screen.selected[id]; !ok {
		t.Fatalf("space did not check %q", id)
	}
	return screen
}

// focusedCategory names the root row under the cursor, for failure messages.
func focusedCategory(screen dependenciesModel) string {
	if item, ok := screen.list.SelectedItem().(categoryItem); ok {
		return item.category.Name
	}
	return "none"
}

// categoryIndex is where a category sits in the root's display order.
func categoryIndex(t *testing.T, screen dependenciesModel, name string) int {
	t.Helper()

	for i, item := range screen.list.Items() {
		if row, ok := item.(categoryItem); ok && row.category.Name == name {
			return i
		}
	}
	t.Fatalf("the root is not showing the category %q", name)
	return -1
}

// TestEscRestoresTheCategoryCursor is the story's core: esc comes back to the
// category the user opened, not to the top of the list.
func TestEscRestoresTheCategoryCursor(t *testing.T) {
	cat := fixtureCatalog(t)
	screen := rootScreen(t, cat)

	const fifth = 4
	names := rootCategoryNames(screen)
	for range fifth {
		screen = drive(t, screen, tea.KeyMsg{Type: tea.KeyDown})
	}
	if got := screen.list.Index(); got != fifth {
		t.Fatalf("root cursor = %d, want %d", got, fifth)
	}
	want := names[fifth]

	opened := drive(t, screen, tea.KeyMsg{Type: tea.KeyEnter})
	if opened.level != levelDependencies || opened.open != want {
		t.Fatalf("enter landed on level %v open %q, want %q", opened.level, opened.open, want)
	}

	back := drive(t, opened, tea.KeyMsg{Type: tea.KeyEsc})
	if back.level != levelCategories {
		t.Fatalf("esc left level %v, want the category root", back.level)
	}
	if got := back.list.Index(); got != fifth {
		t.Errorf("root cursor = %d after esc, want %d (the category that was opened)", got, fifth)
	}
	if got := focusedCategory(back); got != want {
		t.Errorf("focused row = %q, want the category %q", got, want)
	}
}

// TestCursorRestoreWorksFromTheMiddleAndTheLastCategory proves the restore is not
// an accident of the first page: a cursor on the last category, which lives on the
// second page, comes back too.
func TestCursorRestoreWorksFromTheMiddleAndTheLastCategory(t *testing.T) {
	cat := fixtureCatalog(t)

	cases := []struct {
		name  string
		index int
	}{
		{name: "middle", index: 10},
		{name: "last", index: len(cat.Categories) - 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			screen := rootScreen(t, cat)
			names := rootCategoryNames(screen)
			want := names[tc.index]

			for range tc.index {
				screen = drive(t, screen, tea.KeyMsg{Type: tea.KeyDown})
			}
			if got := screen.list.Index(); got != tc.index {
				t.Fatalf("root cursor = %d, want %d", got, tc.index)
			}

			opened := drive(t, screen, tea.KeyMsg{Type: tea.KeyEnter})
			back := drive(t, opened, tea.KeyMsg{Type: tea.KeyEsc})

			if got := back.list.Index(); got != tc.index {
				t.Errorf("root cursor = %d after esc, want %d", got, tc.index)
			}
			if got := focusedCategory(back); got != want {
				t.Errorf("focused row = %q, want %q", got, want)
			}
		})
	}
}

// TestSelectionsStayAcrossCategories covers the second criterion: picking in one
// category, leaving, and picking in another keeps both, everywhere.
func TestSelectionsStayAcrossCategories(t *testing.T) {
	cat := fixtureCatalog(t)
	screen := rootScreen(t, cat)

	screen = openRootCategory(t, screen, "SQL")
	screen = checkDependency(t, screen, "data-jpa")

	screen = drive(t, screen, tea.KeyMsg{Type: tea.KeyEsc})
	screen = openRootCategory(t, screen, "Web")
	screen = checkDependency(t, screen, "web")

	// Web shows its own row checked and counts both.
	if view := screen.view(); !strings.Contains(view, "[x] Spring Web") {
		t.Errorf("the Web row is not marked; view:\n%s", view)
	}
	if got, want := screen.statusLine(), "18 dependencies  2 selected: web, data-jpa"; got != want {
		t.Errorf("footer = %q, want %q", got, want)
	}

	// Back in SQL, its row is still checked and the count is still global.
	screen = drive(t, screen, tea.KeyMsg{Type: tea.KeyEsc})
	screen = openRootCategory(t, screen, "SQL")
	if view := screen.view(); !strings.Contains(view, "[x] Spring Data JPA") {
		t.Errorf("the SQL row lost its check after visiting Web; view:\n%s", view)
	}
	if got := screen.statusLine(); !strings.Contains(got, "2 selected: web, data-jpa") {
		t.Errorf("footer = %q, want both selections counted", got)
	}
}

// TestFooterCountsSelectionsFromOtherCategories covers the third criterion from
// inside a category: the result half is the category's count, the selected half is
// everything the wizard has.
func TestFooterCountsSelectionsFromOtherCategories(t *testing.T) {
	cat := fixtureCatalog(t)
	screen := rootScreen(t, cat)

	screen = openRootCategory(t, screen, "SQL")
	screen = checkDependency(t, screen, "data-jpa")
	screen = drive(t, screen, tea.KeyMsg{Type: tea.KeyEsc})

	screen = openRootCategory(t, screen, "Web")
	if got, want := screen.statusLine(), "18 dependencies  1 selected: data-jpa"; got != want {
		t.Errorf("footer inside Web = %q, want %q", got, want)
	}

	// The header scopes the level and keeps the global totals: the category's own
	// size is the scope, not the whole picture (US4.3).
	if got, want := screen.headerLine(), "< Web (18)  23 categories  206 dependencies"; got != want {
		t.Errorf("header = %q, want %q", got, want)
	}
}

// TestSearchReturnsToItsCategoryAndThenToTheRoot covers the interleaving
// requirement: a search remembers the category it started from, and backing out
// of the category still lands on the right root row.
func TestSearchReturnsToItsCategoryAndThenToTheRoot(t *testing.T) {
	cat := fixtureCatalog(t)
	screen := rootScreen(t, cat)

	screen = openRootCategory(t, screen, "SQL")
	sqlRow := categoryIndex(t, rootScreen(t, cat), "SQL")

	searching := drive(t, screen, keyRunes("/"))
	if searching.level != levelSearch {
		t.Fatalf("/ landed on level %v, want the whole-catalog search", searching.level)
	}
	typed := drive(t, searching, keyRunes("postgres"))
	applied := drive(t, typed, tea.KeyMsg{Type: tea.KeyEnter})
	if !applied.list.IsFiltered() {
		t.Fatal("the query was not applied")
	}

	// The query goes first: one esc clears it and the next leaves the search.
	cleared := drive(t, applied, tea.KeyMsg{Type: tea.KeyEsc})
	if cleared.level != levelSearch {
		t.Fatalf("the first esc left the search for level %v, want it to stay", cleared.level)
	}

	backToCategory := drive(t, cleared, tea.KeyMsg{Type: tea.KeyEsc})
	if backToCategory.level != levelDependencies || backToCategory.open != "SQL" {
		t.Fatalf("the second esc left the search for level %v open %q, want the SQL category", backToCategory.level, backToCategory.open)
	}

	backToRoot := drive(t, backToCategory, tea.KeyMsg{Type: tea.KeyEsc})
	if backToRoot.level != levelCategories {
		t.Fatalf("esc left level %v, want the category root", backToRoot.level)
	}
	if got := backToRoot.list.Index(); got != sqlRow {
		t.Errorf("root cursor = %d, want the SQL row %d", got, sqlRow)
	}
	if got := focusedCategory(backToRoot); got != "SQL" {
		t.Errorf("focused row = %q, want SQL", got)
	}
}

// TestReopeningACategoryStartsAtTheTop pins the choice this story made for the
// fourth criterion: the root cursor is remembered, a category's own cursor is not,
// because one cursor per category would be the navigation stack it does not build.
func TestReopeningACategoryStartsAtTheTop(t *testing.T) {
	cat := fixtureCatalog(t)
	screen := rootScreen(t, cat)

	screen = openRootCategory(t, screen, "AI")
	for range 3 {
		screen = drive(t, screen, tea.KeyMsg{Type: tea.KeyDown})
	}
	if got := screen.list.Index(); got != 3 {
		t.Fatalf("category cursor = %d, want 3", got)
	}

	back := drive(t, screen, tea.KeyMsg{Type: tea.KeyEsc})
	again := openRootCategory(t, back, "AI")
	if got := again.list.Index(); got != 0 {
		t.Errorf("reopened AI starts on row %d, want the top", got)
	}
}

// TestFinishReturnsTheGlobalSet covers the last criterion: ctrl+d hands over
// everything, wherever it was picked, in catalog order.
func TestFinishReturnsTheGlobalSet(t *testing.T) {
	cat := fixtureCatalog(t)
	screen := rootScreen(t, cat)

	screen = openRootCategory(t, screen, "SQL")
	screen = checkDependency(t, screen, "data-jpa")
	screen = drive(t, screen, tea.KeyMsg{Type: tea.KeyEsc})
	screen = openRootCategory(t, screen, "Web")
	screen = checkDependency(t, screen, "web")

	finished, _, action := screen.update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if action != actionFinish {
		t.Errorf("ctrl+d asked for %v, want the run to finish", action)
	}
	if got, want := finished.chosen(), []string{"web", "data-jpa"}; !reflect.DeepEqual(got, want) {
		t.Errorf("chosen = %v, want %v in catalog order", got, want)
	}
}

// TestLandingRowClampsToTheCatalog pins the restore as a number, including what
// happens if it ever points past the list: the root lands on a real row, and every
// other level starts at the top.
func TestLandingRowClampsToTheCatalog(t *testing.T) {
	screen := rootScreen(t, fixtureCatalog(t))

	screen.rootCursor = 999
	if got, want := screen.landingRow(levelCategories), len(screen.categories)-1; got != want {
		t.Errorf("landingRow(categories) = %d for an out-of-range cursor, want the last row %d", got, want)
	}

	screen.rootCursor = -5
	if got := screen.landingRow(levelCategories); got != 0 {
		t.Errorf("landingRow(categories) = %d for a negative cursor, want 0", got)
	}

	screen.rootCursor = 7
	for _, level := range []dependencyLevel{levelDependencies, levelSearch} {
		if got := screen.landingRow(level); got != 0 {
			t.Errorf("landingRow(%v) = %d, want the top", level, got)
		}
	}
}
