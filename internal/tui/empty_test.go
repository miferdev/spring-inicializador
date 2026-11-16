package tui

// Tests for US4.4: an empty level says so in one line and keeps every key on
// screen, a search that matches nothing clears with one esc and leaves with the
// next, and no frame carries dead blank rows. No test needs a terminal or a
// network.

import (
	"strings"
	"testing"

	"github.com/miferdev/spring-inicializador/internal/catalog"
)

// emptyCatalog is a fixture-shaped catalog with one category that holds nothing:
// a server may publish that, and the screen must not look frozen if it does.
func emptyCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()

	web := catalog.Dependency{ID: "web", Name: "Spring Web", Category: "Web"}
	return &catalog.Catalog{
		Categories: []catalog.Category{
			{Name: "Empty"},
			{Name: "Web", Dependencies: []*catalog.Dependency{&web}},
		},
		Dependencies: []catalog.Dependency{web},
	}
}

// blankRuns reports every run of two or more consecutive blank lines, as the
// line number where the run starts and how long it is.
func blankRuns(view string) []struct{ line, length int } {
	var (
		runs   []struct{ line, length int }
		start  int
		length int
	)

	for i, line := range strings.Split(view, "\n") {
		if strings.TrimSpace(line) == "" {
			if length == 0 {
				start = i + 1
			}
			length++
			continue
		}
		if length >= 2 {
			runs = append(runs, struct{ line, length int }{start, length})
		}
		length = 0
	}
	if length >= 2 {
		runs = append(runs, struct{ line, length int }{start, length})
	}

	return runs
}

// TestEmptyCategorySaysSoAndStaysNavigable covers the first criterion: a category
// with nothing in it explains itself in one line, keeps the header and the hints,
// and esc still goes back.
func TestEmptyCategorySaysSoAndStaysNavigable(t *testing.T) {
	cat := emptyCatalog(t)
	screen := openRootCategory(t, rootScreen(t, cat), "Empty")

	if got := len(screen.list.VisibleItems()); got != 0 {
		t.Fatalf("the empty category shows %d rows, want none", got)
	}

	lines := strings.Split(screen.view(), "\n")
	if got := lines[0]; !strings.HasPrefix(got, "< Empty (0)") {
		t.Errorf("header = %q, want it scoped to the empty category", got)
	}

	const explanation = "No dependencies in this category."
	found := false
	for _, line := range lines[1:] {
		if line == explanation {
			found = true
		}
	}
	if !found {
		t.Errorf("the frame does not explain the empty category (%q); view:\n%s", explanation, screen.view())
	}

	for _, fragment := range []string{"ctrl+d", "esc back", "enter/space pick"} {
		if !strings.Contains(screen.view(), fragment) {
			t.Errorf("the frame does not offer %q; view:\n%s", fragment, screen.view())
		}
	}

	back := drive(t, screen, escKey())
	if back.level != levelCategories {
		t.Errorf("esc left level %v, want the category root", back.level)
	}
}

// TestEmptySearchSaysSoAndEscClearsThenLeaves covers the second criterion: the
// miss is one line that quotes the query, and esc takes the query first, then the
// level.
func TestEmptySearchSaysSoAndEscClearsThenLeaves(t *testing.T) {
	cat := fixtureCatalog(t)
	screen := searchFor(t, rootScreen(t, cat), "zzzz")

	if got := len(screen.list.VisibleItems()); got != 0 {
		t.Fatalf("the search shows %d rows, want none", got)
	}

	const explanation = `No dependency matches "zzzz".`
	if view := screen.view(); !strings.Contains(view, explanation) {
		t.Errorf("the frame does not explain the empty search (%q); view:\n%s", explanation, view)
	}
	if view := screen.view(); !strings.Contains(view, "esc clear") {
		t.Errorf("the frame does not offer the esc that clears; view:\n%s", view)
	}

	// The first esc clears the query and keeps the user in the search.
	cleared := drive(t, screen, escKey())
	if cleared.level != levelSearch {
		t.Fatalf("esc left the search for level %v, want it to stay", cleared.level)
	}
	if got := cleared.list.FilterValue(); got != "" {
		t.Errorf("filter = %q after the first esc, want it cleared", got)
	}
	if got := len(cleared.list.VisibleItems()); got != len(cat.Dependencies) {
		t.Errorf("visible rows = %d after clearing, want all %d", got, len(cat.Dependencies))
	}

	// The second esc leaves for the level the search started from.
	back := drive(t, cleared, escKey())
	if back.level != levelCategories {
		t.Errorf("the second esc left level %v, want the category root", back.level)
	}
}

// TestFrameHasNoBlankRuns covers the third criterion at both terminal sizes: the
// frames the story names have no dead rows and stay inside the terminal.
func TestFrameHasNoBlankRuns(t *testing.T) {
	cat := fixtureCatalog(t)

	for _, size := range []struct{ width, height int }{{80, 24}, {120, 30}} {
		root := newDependenciesModel(cat, nil, size.width, size.height, Default())

		frames := []struct {
			name   string
			screen dependenciesModel
		}{
			{name: "root", screen: root},
			{name: "category", screen: openRootCategory(t, root, "Web")},
			{name: "empty search", screen: searchFor(t, root, "zzzz")},
			{name: "search", screen: drive(t, root, keyRunes("/"))},
		}

		for _, frame := range frames {
			t.Run(frame.name, func(t *testing.T) {
				view := frame.screen.view()
				if got := frameLines(view); got > size.height+1 {
					t.Errorf("frame is %d lines at %dx%d, want at most %d", got, size.width, size.height, size.height+1)
				}
				for _, run := range blankRuns(view) {
					t.Errorf("frame has %d consecutive blank lines from line %d; view:\n%s", run.length, run.line, view)
				}

				// Stronger than the rule the story states: these four states have
				// no blank line at all. That is what proves the filter bar's dead
				// row is gone, because a single blank row would not be a run.
				for i, line := range strings.Split(view, "\n") {
					if strings.TrimSpace(line) == "" {
						t.Errorf("frame has a blank line at %d; view:\n%s", i+1, view)
					}
				}
			})
		}
	}
}

// TestKeyHintsAtEachLevel covers the fourth criterion: the hint row names the keys
// that work where the user is, including the way out.
func TestKeyHintsAtEachLevel(t *testing.T) {
	cat := fixtureCatalog(t)
	root := rootScreen(t, cat)

	cases := []struct {
		name   string
		screen dependenciesModel
		want   string
	}{
		{
			name:   "root",
			screen: root,
			want:   "enter open  / find  esc back  ctrl+d finish",
		},
		{
			// Web is focused on Spring Web, which has guides, so the g hint is
			// there too (US5.3).
			name:   "category",
			screen: openRootCategory(t, root, "Web"),
			want:   "enter/space pick  g guide  / find  esc back  ctrl+d finish",
		},
		{
			name:   "search",
			screen: searchFor(t, root, "postgres"),
			want:   "enter/space pick  esc clear  ctrl+d finish",
		},
		{
			name:   "search without a query",
			screen: drive(t, root, keyRunes("/")),
			want:   "enter/space pick  esc back  ctrl+d finish",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.screen.keyHints(); got != tc.want {
				t.Errorf("keyHints = %q, want %q", got, tc.want)
			}
			// The hints are on screen, as the frame's last line.
			lines := strings.Split(tc.screen.view(), "\n")
			if got := lines[len(lines)-1]; got != tc.want {
				t.Errorf("last frame line = %q, want the hints %q", got, tc.want)
			}
			for _, fragment := range []string{"ctrl+d", "esc"} {
				if !strings.Contains(tc.screen.view(), fragment) {
					t.Errorf("the frame does not mention %q; view:\n%s", fragment, tc.screen.view())
				}
			}
		})
	}

	// Inside a category the hints mention both ways to pick.
	if got := (openRootCategory(t, root, "Web")).keyHints(); !strings.Contains(got, "enter/space") {
		t.Errorf("category hints = %q, want both pick keys", got)
	}
}

// TestEmptyFramesAreShortNotBlank proves the empty states do not pad the screen
// with blank rows: they are a handful of lines, not a full terminal of nothing.
func TestEmptyFramesAreShortNotBlank(t *testing.T) {
	cat := fixtureCatalog(t)

	emptySearch := searchFor(t, rootScreen(t, cat), "zzzz")
	if got := frameLines(emptySearch.view()); got > 5 {
		t.Errorf("the empty search frame is %d lines, want it short; view:\n%s", got, emptySearch.view())
	}

	emptyCategory := openRootCategory(t, rootScreen(t, emptyCatalog(t)), "Empty")
	if got := frameLines(emptyCategory.view()); got > 5 {
		t.Errorf("the empty category frame is %d lines, want it short; view:\n%s", got, emptyCategory.view())
	}
}
