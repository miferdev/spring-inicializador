package tui

// Tests for US4.3: the header says where the user is — the open category or the
// query — and a search started anywhere still spans the whole catalog (D5). No
// test needs a terminal or a network.

import (
	"strings"
	"testing"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// headerOf is the first line of the frame, which must be the header.
func headerOf(t *testing.T, screen dependenciesModel) string {
	t.Helper()

	header, _, found := strings.Cut(screen.view(), "\n")
	if !found {
		t.Fatal("the frame has no header line")
	}
	return header
}

// searchFor opens a search with term and presses enter. A query that matches
// something is applied; a query that matches nothing keeps the query while it is
// still being typed, because the component would otherwise drop it — either way
// the term is on screen and the rows reflect it.
func searchFor(t *testing.T, screen dependenciesModel, term string) dependenciesModel {
	t.Helper()

	screen = drive(t, screen, keyRunes("/"))
	if screen.level != levelSearch {
		t.Fatalf("/ landed on level %v, want the whole-catalog search", screen.level)
	}
	screen = drive(t, screen, keyRunes(term))
	screen = drive(t, screen, tea.KeyMsg{Type: tea.KeyEnter})
	if got := screen.list.FilterValue(); got != term {
		t.Fatalf("the %q query is not on screen (filter %q)", term, got)
	}
	return screen
}

// TestHeaderStatesTheScopeOfEachLevel pins the exact header of the three levels:
// totals at the root, the open category with its size, and the query being
// searched. Every one is one ASCII line and the frame's first line.
func TestHeaderStatesTheScopeOfEachLevel(t *testing.T) {
	cat := fixtureCatalog(t)
	root := rootScreen(t, cat)
	web := openRootCategory(t, root, "Web")
	searching := searchFor(t, web, "postgres")

	cases := []struct {
		name   string
		screen dependenciesModel
		want   string
	}{
		{
			name:   "root",
			screen: root,
			want:   "23 categories  206 dependencies",
		},
		{
			name:   "category",
			screen: web,
			want:   "< Web (18)  23 categories  206 dependencies",
		},
		{
			name:   "search",
			screen: searching,
			want:   `Search: "postgres"  23 categories  206 dependencies`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.screen.headerLine(); got != tc.want {
				t.Errorf("headerLine = %q, want %q", got, tc.want)
			}
			if got := headerOf(t, tc.screen); got != tc.want {
				t.Errorf("first frame line = %q, want %q", got, tc.want)
			}
			// US3.3's bound: the terminal height plus one line of slack. The
			// scoped header must not have added a line.
			if got := frameLines(tc.screen.view()); got > 24+1 {
				t.Errorf("frame is %d lines at 80x24, want at most 25", got)
			}
			for _, r := range tc.screen.headerLine() {
				if r > unicode.MaxASCII {
					t.Errorf("header %q is not ASCII (found %q)", tc.screen.headerLine(), r)
					break
				}
			}
		})
	}
}

// TestHeaderIsStable proves the header is a function of the state: two screens
// driven to the same place render the same line, and asking twice does not move.
func TestHeaderIsStable(t *testing.T) {
	cat := fixtureCatalog(t)

	build := func() dependenciesModel {
		screen := rootScreen(t, cat)
		screen = openRootCategory(t, screen, "SQL")
		return searchFor(t, screen, "postgres")
	}

	first, second := build(), build()
	if got, want := first.headerLine(), second.headerLine(); got != want {
		t.Errorf("identical screens rendered %q and %q", got, want)
	}
	if again := first.headerLine(); again != first.headerLine() {
		t.Errorf("headerLine changed between calls: %q then %q", first.headerLine(), again)
	}

	// The level's scope is what changes, not the totals: both screens agree on the
	// global picture.
	const totals = "23 categories  206 dependencies"
	for _, screen := range []dependenciesModel{first, second} {
		if !strings.HasSuffix(screen.headerLine(), totals) {
			t.Errorf("header %q does not end with the totals %q", screen.headerLine(), totals)
		}
	}
}

// TestSearchFromInsideACategorySpansTheWholeCatalog is the D5 proof the story
// asks for: a search never narrows to the open category, so a SQL search still
// shows the AI dependency that shares the term.
func TestSearchFromInsideACategorySpansTheWholeCatalog(t *testing.T) {
	cat := fixtureCatalog(t)
	screen := openRootCategory(t, rootScreen(t, cat), "SQL")

	searching := drive(t, screen, keyRunes("/"))
	// A search with nothing typed is still a search of everything, and says so.
	if got := searching.headerLine(); !strings.HasPrefix(got, "Search: ") {
		t.Errorf("header = %q, want the search scope", got)
	}
	if got := len(searching.list.Items()); got != len(cat.Dependencies) {
		t.Fatalf("search rows = %d, want all %d dependencies", got, len(cat.Dependencies))
	}

	applied := searchFor(t, screen, "postgres")

	// The result set spans categories: the SQL driver and the AI one, which is
	// outside the category the search started from.
	shown := make(map[string]string, len(applied.list.VisibleItems()))
	for _, item := range applied.list.VisibleItems() {
		row, ok := item.(dependencyItem)
		if !ok {
			t.Fatalf("result row %T is not a dependency", item)
		}
		shown[row.dependency.ID] = row.dependency.Category
	}
	if got, want := shown["postgresql"], "SQL"; got != want {
		t.Errorf("postgresql category = %q, want %q", got, want)
	}
	if got, want := shown["spring-ai-postgresml"], "AI"; got != want {
		t.Errorf("spring-ai-postgresml category = %q, want %q; a search must not exclude rows outside the open category", got, want)
	}

	// The header says it is a search, not the category it started from.
	if got := applied.headerLine(); !strings.HasPrefix(got, `Search: "postgres"`) {
		t.Errorf("header = %q, want the search scope", got)
	}
}

// TestLeavingSearchRestoresTheLevelAndItsHeader covers the third criterion: esc
// from search lands back in the category, header and rows included, and a second
// esc lands on the root row that was open.
func TestLeavingSearchRestoresTheLevelAndItsHeader(t *testing.T) {
	cat := fixtureCatalog(t)
	root := rootScreen(t, cat)
	sqlRow := categoryIndex(t, root, "SQL")

	inside := openRootCategory(t, root, "SQL")
	applied := searchFor(t, inside, "postgres")

	// The first esc clears the query and keeps the user in the search; the second
	// leaves it for the level it started from.
	cleared := drive(t, applied, tea.KeyMsg{Type: tea.KeyEsc})
	if cleared.level != levelSearch {
		t.Fatalf("the first esc left the search for level %v, want it to stay", cleared.level)
	}
	if got := cleared.list.FilterValue(); got != "" {
		t.Fatalf("filter = %q after the first esc, want it cleared", got)
	}

	backToCategory := drive(t, cleared, tea.KeyMsg{Type: tea.KeyEsc})
	if backToCategory.level != levelDependencies || backToCategory.open != "SQL" {
		t.Fatalf("esc left the search for level %v open %q, want SQL", backToCategory.level, backToCategory.open)
	}
	if got, want := backToCategory.headerLine(), "< SQL (19)  23 categories  206 dependencies"; got != want {
		t.Errorf("header after leaving search = %q, want %q", got, want)
	}
	if got, want := len(backToCategory.list.Items()), len(mustCategory(t, cat, "SQL").Dependencies); got != want {
		t.Errorf("SQL rows after leaving search = %d, want %d", got, want)
	}

	backToRoot := drive(t, backToCategory, tea.KeyMsg{Type: tea.KeyEsc})
	if backToRoot.level != levelCategories {
		t.Fatalf("esc left level %v, want the category root", backToRoot.level)
	}
	if got, want := backToRoot.headerLine(), "23 categories  206 dependencies"; got != want {
		t.Errorf("header at the root = %q, want %q", got, want)
	}
	if got := backToRoot.list.Index(); got != sqlRow {
		t.Errorf("root cursor = %d, want the SQL row %d", got, sqlRow)
	}
}

// TestHeaderKeepsANonASCIIQueryASCII pins the escape: the header is one ASCII
// line whatever the user typed, because the query is quoted ASCII-only.
func TestHeaderKeepsANonASCIIQueryASCII(t *testing.T) {
	cat := fixtureCatalog(t)
	screen := drive(t, rootScreen(t, cat), keyRunes("/"))
	screen = drive(t, screen, keyRunes("caf\u00e9"))

	header := screen.headerLine()
	if !strings.Contains(header, `caf\u00e9`) {
		t.Errorf("header = %q, want the query escaped", header)
	}
	for _, r := range header {
		if r > unicode.MaxASCII {
			t.Errorf("header %q is not ASCII (found %q)", header, r)
			break
		}
	}
}

// TestHeaderFitsNarrowTerminals pins the 40-column shapes: the scope survives and
// the totals give way, because the scope is the part the rows cannot tell.
func TestHeaderFitsNarrowTerminals(t *testing.T) {
	cat := fixtureCatalog(t)
	root := newDependenciesModel(cat, nil, 40, 24, Default())

	cases := []struct {
		name   string
		screen dependenciesModel
		want   string
	}{
		{
			name:   "root",
			screen: root,
			want:   "23 categories  206 dependencies",
		},
		{
			name:   "category",
			screen: openRootCategory(t, root, "Web"),
			want:   "< Web (18)",
		},
		{
			name:   "search",
			screen: searchFor(t, openRootCategory(t, root, "Web"), "postgres"),
			want:   `Search: "postgres"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.screen.headerLine()
			if got != tc.want {
				t.Errorf("header at 40 columns = %q, want %q", got, tc.want)
			}
			if width := ansi.StringWidth(got); width > 40 {
				t.Errorf("header is %d cells wide, want at most 40: %q", width, got)
			}
			if got != headerOf(t, tc.screen) {
				t.Errorf("first frame line = %q, want the header %q", headerOf(t, tc.screen), got)
			}
		})
	}
}

// TestHeaderTruncatesALongCategoryNameButKeepsItsCount is the truncation rule at
// 40 columns: the name gives way, the size stays, the line stays inside the
// terminal.
func TestHeaderTruncatesALongCategoryNameButKeepsItsCount(t *testing.T) {
	cat := fixtureCatalog(t)
	screen := newDependenciesModel(cat, nil, 40, 24, Default())
	screen = openRootCategory(t, screen, "VMware Tanzu Spring Enterprise Extensions")

	const want = "< VMware Tanzu Spring Enterprise ... (7)"
	if got := screen.headerLine(); got != want {
		t.Errorf("header = %q, want %q", got, want)
	}
	if got := ansi.StringWidth(screen.headerLine()); got > 40 {
		t.Errorf("header is %d cells wide, want at most 40", got)
	}
	if !strings.HasSuffix(screen.headerLine(), "(7)") {
		t.Errorf("header %q lost the category count", screen.headerLine())
	}
	for _, r := range screen.headerLine() {
		if r > unicode.MaxASCII {
			t.Errorf("header %q is not ASCII (found %q)", screen.headerLine(), r)
			break
		}
	}
}

// TestSearchResultsKeepTheCategoryLabel asserts the muted label rather than
// trusting that the row text happens to contain the category: the delegate is
// given a theme whose muted style is visible in a plain-text renderer, and the
// label must come out styled while the dependency name does not.
func TestSearchResultsKeepTheCategoryLabel(t *testing.T) {
	cat := fixtureCatalog(t)
	screen := searchFor(t, openRootCategory(t, rootScreen(t, cat), "SQL"), "postgres")

	theme := Default()
	theme.Muted = theme.Muted.Transform(func(label string) string { return "<<" + label + ">>" })
	delegate := dependencyDelegate{theme: theme}

	visible := screen.list.VisibleItems()
	if len(visible) == 0 {
		t.Fatal("the search shows no results")
	}
	for _, item := range visible {
		row, ok := item.(dependencyItem)
		if !ok {
			t.Fatalf("result row %T is not a dependency", item)
		}

		var rendered strings.Builder
		delegate.Render(&rendered, screen.list, 0, item)

		if want := "<<" + row.dependency.Category + ">>"; !strings.Contains(rendered.String(), want) {
			t.Errorf("result row %q does not carry its category through the muted style (%q)", rendered.String(), want)
		}
		if !strings.Contains(rendered.String(), row.dependency.Name) {
			t.Errorf("result row %q does not show the dependency name %q", rendered.String(), row.dependency.Name)
		}
		if strings.Contains(rendered.String(), "<<"+row.dependency.Name+">>") {
			t.Errorf("the dependency name went through the muted style: %q", rendered.String())
		}
	}
}
