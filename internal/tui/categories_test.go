package tui

// Tests for US4.1: the dependency screen opens on the categories, ordered by size
// (D7), and every key that used to finish now has a level to act on — ctrl+d is
// the way out. The screen is driven with the messages the event loop delivers, so
// no test needs a terminal or a network.

import (
	"reflect"
	"strings"
	"testing"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/miferdev/spring-inicializador/internal/catalog"
)

// keyRunes is the message a terminal sends for typed text.
func keyRunes(text string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)}
}

// rootScreen builds the dependency screen at its root, at a terminal size.
func rootScreen(t *testing.T, cat *catalog.Catalog) dependenciesModel {
	t.Helper()
	return newDependenciesModel(cat, nil, 80, 24, Default())
}

// categoryNames is the order a list of categories is in.
func categoryNames(categories []catalog.Category) []string {
	names := make([]string, 0, len(categories))
	for _, category := range categories {
		names = append(names, category.Name)
	}
	return names
}

// dependenciesOf builds n distinct dependencies, for the synthetic catalogs the
// ordering test needs.
func dependenciesOf(n int) []*catalog.Dependency {
	dependencies := make([]*catalog.Dependency, 0, n)
	for i := range n {
		dependencies = append(dependencies, &catalog.Dependency{
			ID:   "dep-" + string(rune('a'+i)),
			Name: "Dependency " + string(rune('A'+i)),
		})
	}
	return dependencies
}

// TestRootListsEveryCategoryWithItsCount is the story's first criterion: the root
// is the catalog's categories, each with how many dependencies it holds.
func TestRootListsEveryCategoryWithItsCount(t *testing.T) {
	cat := fixtureCatalog(t)
	if len(cat.Categories) != 23 || len(cat.Dependencies) != 206 {
		t.Fatalf("the fixture has %d categories and %d dependencies, want the 23 and 206 this story measures",
			len(cat.Categories), len(cat.Dependencies))
	}

	screen := rootScreen(t, cat)
	if screen.level != levelCategories {
		t.Fatalf("level = %v, want the category root", screen.level)
	}

	want := make(map[string]int, len(cat.Categories))
	for _, category := range cat.Categories {
		want[category.Name] = len(category.Dependencies)
	}

	rows := screen.list.Items()
	if len(rows) != len(cat.Categories) {
		t.Fatalf("root rows = %d, want every one of the %d categories", len(rows), len(cat.Categories))
	}

	sum := 0
	for _, item := range rows {
		row, ok := item.(categoryItem)
		if !ok {
			t.Fatalf("root row %T is not a category", item)
		}
		count := len(row.category.Dependencies)
		if got := want[row.category.Name]; got != count {
			t.Errorf("category %q holds %d dependencies, want %d", row.category.Name, count, got)
		}
		sum += count
		delete(want, row.category.Name)
	}
	if len(want) != 0 {
		t.Errorf("the root is missing categories %v", want)
	}
	if sum != len(cat.Dependencies) {
		t.Errorf("the counts sum to %d, want the catalog's %d dependencies", sum, len(cat.Dependencies))
	}

	// The frame shows the names and their counts.
	view := screen.view()
	for _, fragment := range []string{"AI", "(50)", "SQL", "(19)"} {
		if !strings.Contains(view, fragment) {
			t.Errorf("the root does not show %q; view:\n%s", fragment, view)
		}
	}
}

// TestRankCategoriesOrdersByCountThenName pins D7: the biggest category first,
// ties alphabetical, whatever order the server used.
func TestRankCategoriesOrdersByCountThenName(t *testing.T) {
	// Three dependencies, two categories tied: only the name can break the tie.
	categories := []catalog.Category{
		{Name: "Zeta", Dependencies: dependenciesOf(3)},
		{Name: "Alpha", Dependencies: dependenciesOf(3)},
		{Name: "Beta", Dependencies: dependenciesOf(1)},
	}
	if got, want := categoryNames(rankCategories(categories)), []string{"Alpha", "Zeta", "Beta"}; !reflect.DeepEqual(got, want) {
		t.Errorf("rankCategories = %v, want %v (count descending, ties alphabetical)", got, want)
	}

	// The parser keeps the server's order; the screen is what reorders. The
	// document below lists the small category first, so a parse that sorted would
	// be visible.
	const doc = `{"dependencies":{"values":[
		{"name":"Zeta","values":[{"id":"z1","name":"Z1"}]},
		{"name":"Alpha","values":[{"id":"a1","name":"A1"},{"id":"a2","name":"A2"}]}
	]}}`
	parsed, err := catalog.Parse([]byte(doc))
	if err != nil {
		t.Fatalf("catalog.Parse: %v", err)
	}
	if got, want := categoryNames(parsed.Categories), []string{"Zeta", "Alpha"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Parse kept %v, want the server order %v", got, want)
	}

	screen := rootScreen(t, parsed)
	if got, want := rootCategoryNames(screen), []string{"Alpha", "Zeta"}; !reflect.DeepEqual(got, want) {
		t.Errorf("the root shows %v, want the display order %v", got, want)
	}

	// The fixture proves the same thing on real data: the server publishes
	// Developer Tools first, and D7 puts AI first.
	fixture := fixtureCatalog(t)
	if first := fixture.Categories[0].Name; first != "Developer Tools" {
		t.Fatalf("the fixture's first category = %q, want the server's Developer Tools", first)
	}
	if got := rootCategoryNames(rootScreen(t, fixture))[0]; got != "AI" {
		t.Errorf("the root's first category = %q, want AI, the largest", got)
	}
}

// rootCategoryNames is the root's display order.
func rootCategoryNames(screen dependenciesModel) []string {
	names := make([]string, 0, len(screen.categories))
	for _, item := range screen.categories {
		names = append(names, item.category.Name)
	}
	return names
}

// TestCategoryRowTruncatesTheNameAndKeepsTheCountRight covers the third criterion:
// a long name gives way so the count stays on the right edge.
func TestCategoryRowTruncatesTheNameAndKeepsTheCountRight(t *testing.T) {
	const long = "A category name so long that it cannot possibly fit in the row"
	dependency := catalog.Dependency{ID: "x", Name: "X", Category: long}
	cat := &catalog.Catalog{
		Categories:   []catalog.Category{{Name: long, Dependencies: []*catalog.Dependency{&dependency}}},
		Dependencies: []catalog.Dependency{dependency},
	}

	screen := newDependenciesModel(cat, nil, 40, 12, Default())
	var row strings.Builder
	categoryDelegate{theme: Default()}.Render(&row, screen.list, 0, screen.list.Items()[0])

	line := row.String()
	if width := ansi.StringWidth(line); width != 40 {
		t.Errorf("row is %d cells wide, want the whole 40-cell line: %q", width, line)
	}
	if !strings.HasSuffix(line, "(1)") {
		t.Errorf("row %q does not end with the count, so the count is not right-aligned", line)
	}
	if !strings.Contains(line, "...") {
		t.Errorf("row %q did not truncate the long name", line)
	}
	if !strings.HasPrefix(line, ">") {
		t.Errorf("row %q is the focused row but carries no focus marker", line)
	}
	for _, r := range line {
		if r > unicode.MaxASCII {
			t.Errorf("row %q is not ASCII (found %q)", line, r)
			break
		}
	}

	// A short name needs no truncation and keeps the same right edge.
	screen = newDependenciesModel(fixtureCatalog(t), nil, 80, 24, Default())
	row.Reset()
	categoryDelegate{theme: Default()}.Render(&row, screen.list, 0, screen.list.Items()[0])
	if got := row.String(); strings.Contains(got, "...") {
		t.Errorf("row %q truncated a name that fits", got)
	}
	if width := ansi.StringWidth(row.String()); width != 80 {
		t.Errorf("row is %d cells wide, want 80", width)
	}
}

// TestHeaderStatesTheTotals covers the fourth criterion: both totals, on one line,
// above the list.
func TestHeaderStatesTheTotals(t *testing.T) {
	cat := fixtureCatalog(t)
	screen := rootScreen(t, cat)

	const want = "23 categories  206 dependencies"
	if got := screen.headerLine(); got != want {
		t.Errorf("header = %q, want %q", got, want)
	}

	header, _, found := strings.Cut(screen.view(), "\n")
	if !found {
		t.Fatal("the frame has no header line")
	}
	if header != want {
		t.Errorf("first frame line = %q, want the header %q", header, want)
	}

	// Opening a category keeps the totals in the header and puts the level's scope
	// in front of them; US4.3 pins the exact shapes in scope_test.go.
	inside := drive(t, screen, tea.KeyMsg{Type: tea.KeyEnter})
	if got := inside.headerLine(); !strings.HasSuffix(got, want) {
		t.Errorf("header inside a category = %q, want it to end with the totals %q", got, want)
	}
}

// TestEnterOpensACategoryAndEscReturnsToTheRoot covers the fifth criterion.
func TestEnterOpensACategoryAndEscReturnsToTheRoot(t *testing.T) {
	cat := fixtureCatalog(t)
	screen := rootScreen(t, cat)

	// AI is the largest category, so D7 puts it under the cursor.
	opened := drive(t, screen, tea.KeyMsg{Type: tea.KeyEnter})
	if opened.level != levelDependencies || opened.open != "AI" {
		t.Fatalf("enter landed on level %v open %q, want the AI category", opened.level, opened.open)
	}

	rows := opened.list.Items()
	if len(rows) == 0 {
		t.Fatal("the opened category has no rows")
	}
	for _, item := range rows {
		row, ok := item.(dependencyItem)
		if !ok {
			t.Fatalf("row %T is not a dependency", item)
		}
		if row.dependency.Category != "AI" {
			t.Errorf("the AI category shows %q, which belongs to %q", row.dependency.ID, row.dependency.Category)
		}
	}
	if got, want := len(rows), len(mustCategory(t, cat, "AI").Dependencies); got != want {
		t.Errorf("the AI category shows %d rows, want %d", got, want)
	}

	// esc backs out to the root, with the categories back.
	back := drive(t, opened, tea.KeyMsg{Type: tea.KeyEsc})
	if back.level != levelCategories {
		t.Errorf("esc left level %v, want the category root", back.level)
	}
	if got := len(back.list.Items()); got != len(cat.Categories) {
		t.Errorf("the root shows %d rows after esc, want the %d categories", got, len(cat.Categories))
	}
}

// mustCategory is the fixture's category with that name.
func mustCategory(t *testing.T, cat *catalog.Catalog, name string) catalog.Category {
	t.Helper()

	for _, category := range cat.Categories {
		if category.Name == name {
			return category
		}
	}
	t.Fatalf("the fixture has no category %q", name)
	return catalog.Category{}
}

// TestCtrlDFinishesFromEveryLevel covers the sixth criterion at the screen level:
// the finish key works wherever the screen is, and hands back the checked set.
func TestCtrlDFinishesFromEveryLevel(t *testing.T) {
	cat := fixtureCatalog(t)

	for _, open := range []bool{false, true} {
		screen := newDependenciesModel(cat, []string{"web"}, 80, 24, Default())
		if open {
			screen = drive(t, screen, tea.KeyMsg{Type: tea.KeyEnter})
			if screen.level != levelDependencies {
				t.Fatalf("enter did not open a category (level %v)", screen.level)
			}
		}

		finished, _, action := screen.update(tea.KeyMsg{Type: tea.KeyCtrlD})
		if action != actionFinish {
			t.Errorf("ctrl+d at level %v asked for %v, want the run to finish", screen.level, action)
		}
		if got, want := finished.chosen(), []string{"web"}; !reflect.DeepEqual(got, want) {
			t.Errorf("ctrl+d at level %v returned %v, want %v", screen.level, got, want)
		}
	}
}

// TestEnterOnADependencyRowTogglesInsteadOfFinishing pins the key split: inside a
// category enter is a checkbox, so docs/ui.md's "enter toggles" is true where it
// says it is.
func TestEnterOnADependencyRowTogglesInsteadOfFinishing(t *testing.T) {
	cat := fixtureCatalog(t)
	screen := drive(t, rootScreen(t, cat), tea.KeyMsg{Type: tea.KeyEnter})

	item, ok := screen.list.SelectedItem().(dependencyItem)
	if !ok {
		t.Fatalf("the opened category focused %T, want a dependency", screen.list.SelectedItem())
	}

	checked, _, action := screen.update(tea.KeyMsg{Type: tea.KeyEnter})
	if action != actionStay {
		t.Errorf("enter on a dependency row asked for %v, want the screen to stay", action)
	}
	if got, want := checked.chosen(), []string{item.dependency.ID}; !reflect.DeepEqual(got, want) {
		t.Errorf("enter checked %v, want %v", got, want)
	}
	if checked.level != levelDependencies {
		t.Errorf("enter left the category (level %v)", checked.level)
	}

	// Enter again unchecks, the same as space.
	unchecked, _, _ := checked.update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := unchecked.chosen(); len(got) != 0 {
		t.Errorf("a second enter left %v checked, want none", got)
	}
}

// TestSearchFromTheRootCoversTheWholeCatalog covers the seventh criterion: / from
// the root leaves the categories for the flat catalog (D5), and the ranked rows
// carry their category as the muted label.
func TestSearchFromTheRootCoversTheWholeCatalog(t *testing.T) {
	cat := fixtureCatalog(t)
	screen := rootScreen(t, cat)
	if got := len(screen.list.Items()); got != len(cat.Categories) {
		t.Fatalf("root rows = %d, want the %d categories before searching", got, len(cat.Categories))
	}

	searching := drive(t, screen, keyRunes("/"))
	if searching.level != levelSearch {
		t.Fatalf("/ landed on level %v, want the whole-catalog search", searching.level)
	}
	if !searching.list.SettingFilter() {
		t.Error("/ did not focus the filter")
	}
	if got := len(searching.list.Items()); got != len(cat.Dependencies) {
		t.Fatalf("search rows = %d, want all %d dependencies", got, len(cat.Dependencies))
	}

	filtered := drive(t, searching, keyRunes("postgres"))
	applied := drive(t, filtered, tea.KeyMsg{Type: tea.KeyEnter})

	visible := applied.list.VisibleItems()
	if len(visible) == 0 {
		t.Fatal("postgres matched nothing")
	}
	if len(visible) == len(cat.Dependencies) {
		t.Fatalf("postgres matched all %d dependencies, so the query was not applied", len(visible))
	}
	first, ok := visible[0].(dependencyItem)
	if !ok || first.dependency.ID != "postgresql" {
		t.Fatalf("first result = %#v, want the postgresql driver", visible[0])
	}

	// The result row shows the dependency name and its category as the muted
	// label, which is the shape every dependency row has.
	var row strings.Builder
	applied.rowDelegate().Render(&row, applied.list, 0, visible[0])
	if !strings.Contains(row.String(), "PostgreSQL Driver") || !strings.Contains(row.String(), "SQL") {
		t.Errorf("result row %q does not show the name and its category", row.String())
	}

	// esc leaves the search and the root comes back, not an empty frame.
	back := drive(t, applied, tea.KeyMsg{Type: tea.KeyEsc})
	if back.level != levelCategories {
		t.Errorf("esc left level %v, want the category root", back.level)
	}
	if got := len(back.list.Items()); got != len(cat.Categories) {
		t.Errorf("the root shows %d rows after leaving the search, want %d", got, len(cat.Categories))
	}
}
