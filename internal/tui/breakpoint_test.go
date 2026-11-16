package tui

// Tests for US5.2: the dependency pane's breakpoints. Below 100 columns the frame
// is exactly what it was; between 100 and 119 the focused row carries its
// description as one muted line under it; at 120 the detail region comes back. A
// resize across those bands must not cost the user their place. No test needs a
// terminal or a network.

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// inlineLine is the description line the middle band inserts under the focused
// row, or "" when the frame has none. No row starts with the indent: a row starts
// with its focus marker and a space.
func inlineLine(screen dependenciesModel) string {
	for _, line := range strings.Split(screen.view(), "\n") {
		if strings.HasPrefix(line, inlineIndent) {
			return line
		}
	}
	return ""
}

// lineWith is the index of the first frame line starting with prefix, or -1.
func lineWith(screen dependenciesModel, prefix string) int {
	for i, line := range strings.Split(screen.view(), "\n") {
		if strings.HasPrefix(line, prefix) {
			return i
		}
	}
	return -1
}

// assertScreenState fails when a resize lost the level, the open category, the
// selection, the cursor or the query.
func assertScreenState(t *testing.T, got, want dependenciesModel, where string) {
	t.Helper()

	if got.level != want.level {
		t.Errorf("%s: level = %v, want %v", where, got.level, want.level)
	}
	if got.open != want.open {
		t.Errorf("%s: open category = %q, want %q", where, got.open, want.open)
	}
	if wantSel, gotSel := want.chosen(), got.chosen(); strings.Join(gotSel, ",") != strings.Join(wantSel, ",") {
		t.Errorf("%s: checked = %v, want %v", where, gotSel, wantSel)
	}
	if got.list.Index() != want.list.Index() {
		t.Errorf("%s: cursor = %d, want %d", where, got.list.Index(), want.list.Index())
	}
	if got.list.FilterValue() != want.list.FilterValue() {
		t.Errorf("%s: query = %q, want %q", where, got.list.FilterValue(), want.list.FilterValue())
	}
}

// TestMiddleBandDescribesTheFocusedRow is the band's core: one muted line directly
// under the focused row, cut to the width, and no other row disturbed.
func TestMiddleBandDescribesTheFocusedRow(t *testing.T) {
	cat := fixtureCatalog(t)
	screen := openRootCategory(t, sizedScreen(t, cat, 110, 24), "Web")

	if !screen.showInlineDescription() {
		t.Fatal("110 columns did not turn on the description under the focused row")
	}
	if screen.showDetail() {
		t.Fatal("110 columns split the body, want the list with an inline line")
	}

	row := lineWith(screen, "> [ ] Spring Web")
	if row < 0 {
		t.Fatalf("the focused Spring Web row is not on screen; view:\n%s", screen.view())
	}

	lines := strings.Split(screen.view(), "\n")
	description := lines[row+1]
	dependency := mustDependency(t, cat, "web")

	if !strings.HasPrefix(description, inlineIndent+"Build web, including RESTful") {
		t.Errorf("the line under the row is %q, want the description indented under it", description)
	}
	if width := ansi.StringWidth(description); width != 110 {
		t.Errorf("the description line is %d cells wide, want it cut to the 110-cell terminal: %q", width, description)
	}
	if !strings.HasSuffix(description, "...") {
		t.Errorf("the description line %q was cut without ...", description)
	}
	if len(dependency.Description) <= 110-len(inlineIndent) && !strings.Contains(description, dependency.Description) {
		t.Errorf("the description line %q lost the metadata's text", description)
	}

	// Exactly one line: the row after it is the next row, not a second description.
	if next := lines[row+2]; !strings.HasPrefix(next, "  [ ] ") {
		t.Errorf("the line after the description is %q, want the next row", next)
	}
	if got := frameLines(screen.view()); got > 24 {
		t.Errorf("the frame is %d lines at 110x24, want at most 24", got)
	}
}

// TestMiddleBandFollowsTheCursorAndTheFilter: the line describes whatever is
// focused, in a category and in a search.
func TestMiddleBandFollowsTheCursorAndTheFilter(t *testing.T) {
	cat := fixtureCatalog(t)
	screen := openRootCategory(t, sizedScreen(t, cat, 110, 24), "Web")

	if got := inlineLine(screen); !strings.Contains(got, "Build web, including RESTful") {
		t.Fatalf("the first row's description line = %q, want Spring Web's", got)
	}

	moved := drive(t, screen, down())
	if got := focusedDependencyID(t, moved); got != "webflux" {
		t.Fatalf("focus = %q, want webflux", got)
	}
	// The line is under the row that moved, not under the one it left.
	row := lineWith(moved, "> [ ] Spring Reactive Web")
	if row < 0 {
		t.Fatalf("the focused webflux row is not on screen; view:\n%s", moved.view())
	}
	if next := strings.Split(moved.view(), "\n")[row+1]; !strings.HasPrefix(next, inlineIndent+"Build reactive web applications") {
		t.Errorf("the line under the focused row is %q, want its description", next)
	}
	if got := inlineLine(moved); !strings.Contains(got, "Build reactive web applications") {
		t.Errorf("the description line did not follow the cursor: %q", got)
	}

	applied := searchFor(t, sizedScreen(t, cat, 110, 24), "postgres")
	if got := focusedDependencyID(t, applied); got != "postgresql" {
		t.Fatalf("first result = %q, want postgresql", got)
	}
	if got := inlineLine(applied); !strings.Contains(got, "A JDBC and R2DBC driver") {
		t.Errorf("the search's description line = %q, want the PostgreSQL driver's", got)
	}
}

// TestMiddleBandKeepsAFullWindowInsideTheTerminal: when the rows fill the window,
// the description's row is reserved, so the frame stays exactly the terminal's
// height instead of growing one line past it.
func TestMiddleBandKeepsAFullWindowInsideTheTerminal(t *testing.T) {
	cat := fixtureCatalog(t)
	full := searchFor(t, sizedScreen(t, cat, 110, 24), "sql")

	if !full.showInlineDescription() {
		t.Fatal("110 columns did not turn on the description under the focused row")
	}
	if got := len(full.list.VisibleItems()); got < dependenciesListHeight(24) {
		t.Fatalf("the query shows %d rows, want enough to fill the window", got)
	}
	if got := frameLines(full.view()); got > 24 {
		t.Errorf("the frame is %d lines at 110x24 with a full window, want at most 24", got)
	}
	if inlineLine(full) == "" {
		t.Error("the full window rendered no description")
	}
}

// TestBelow100KeepsTheListOnly pins the shape the narrower terminals had before
// this story: no region, no inline line, the same frame.
func TestBelow100KeepsTheListOnly(t *testing.T) {
	cat := fixtureCatalog(t)
	screen := openRootCategory(t, sizedScreen(t, cat, 80, 24), "Web")

	if screen.showInlineDescription() || screen.showDetail() {
		t.Error("80 columns turned on a description")
	}
	if got := inlineLine(screen); got != "" {
		t.Errorf("80 columns rendered an inline description: %q", got)
	}

	lines := strings.Split(screen.view(), "\n")
	if got, want := lines[0], "< Web (18)  23 categories  206 dependencies"; got != want {
		t.Errorf("header = %q, want %q", got, want)
	}
	if got := strings.TrimRight(lines[1], " "); got != "> [ ] Spring Web Web" {
		t.Errorf("first body line = %q, want the row as it was before this story", got)
	}
	if strings.Contains(screen.view(), "|") {
		t.Errorf("80 columns carry a separator:\n%s", screen.view())
	}
	if got := frameLines(screen.view()); got != 21 {
		t.Errorf("frame is %d lines at 80x24 for Web, want the 21 it had", got)
	}
}

// TestResizeReflowsAcrossBreakpoints drives 80 -> 110 -> 120 -> 80 and checks that
// the layout changes while the user's state does not.
func TestResizeReflowsAcrossBreakpoints(t *testing.T) {
	cat := fixtureCatalog(t)
	start := openRootCategory(t, sizedScreen(t, cat, 80, 24), "Web")
	start = drive(t, start, down())     // webflux
	start = drive(t, start, spaceKey()) // checked
	if got, want := start.chosen(), []string{"webflux"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("setup checked %v, want %v", got, want)
	}

	wide := start.resize(110, 24)
	assertScreenState(t, wide, start, "80 -> 110")
	if !wide.showInlineDescription() || inlineLine(wide) == "" {
		t.Errorf("110 columns did not describe the focused row; view:\n%s", wide.view())
	}
	if got := frameLines(wide.view()); got > 24 {
		t.Errorf("the frame is %d lines at 110x24, want at most 24", got)
	}

	widest := wide.resize(120, 24)
	assertScreenState(t, widest, start, "80 -> 120")
	if !widest.showDetail() {
		t.Error("120 columns did not split the body")
	}
	if got := inlineLine(widest); got != "" {
		t.Errorf("120 columns kept the inline line: %q", got)
	}
	if !strings.Contains(widest.view(), " | ") {
		t.Errorf("120 columns have no separator; view:\n%s", widest.view())
	}
	if got := frameLines(widest.view()); got > 24 {
		t.Errorf("the frame is %d lines at 120x24, want at most 24", got)
	}

	back := widest.resize(80, 24)
	assertScreenState(t, back, start, "80 -> 120 -> 80")
	if back.showDetail() || back.showInlineDescription() {
		t.Error("80 columns still show a description")
	}
	if got := inlineLine(back); got != "" {
		t.Errorf("the inline line survived the trip back to 80: %q", got)
	}
	if strings.Contains(back.view(), "|") {
		t.Errorf("the separator survived the trip back to 80:\n%s", back.view())
	}
	if got := frameLines(back.view()); got != 21 {
		t.Errorf("frame is %d lines at 80x24 after the trip, want 21", got)
	}
}

// TestFilteredListSurvivesResizes: a query and its cursor cross the bands too.
func TestFilteredListSurvivesResizes(t *testing.T) {
	cat := fixtureCatalog(t)
	filtered := searchFor(t, sizedScreen(t, cat, 80, 24), "spring")
	row := filtered.list.Index()

	for _, width := range []int{110, 120, 80} {
		resized := filtered.resize(width, 24)
		if got := resized.list.FilterValue(); got != "spring" {
			t.Errorf("at %d columns the query is %q, want spring", width, got)
		}
		if got := resized.list.Index(); got != row {
			t.Errorf("at %d columns the cursor is %d, want %d", width, got, row)
		}
		if resized.level != levelSearch {
			t.Errorf("at %d columns the level is %v, want the search", width, resized.level)
		}
		if got := frameLines(resized.view()); got > 24 {
			t.Errorf("at %d columns the frame is %d lines, want at most 24", width, got)
		}
	}
}

// TestHintsReadableAt80x24 covers the fourth criterion: every key hint is on
// screen, untruncated, and nothing reaches past the terminal's width or height.
func TestHintsReadableAt80x24(t *testing.T) {
	cat := fixtureCatalog(t)

	cases := []struct {
		name   string
		screen dependenciesModel
	}{
		{name: "root", screen: sizedScreen(t, cat, 80, 24)},
		{name: "category", screen: openRootCategory(t, sizedScreen(t, cat, 80, 24), "Web")},
		{name: "search", screen: searchFor(t, sizedScreen(t, cat, 80, 24), "postgres")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			view := tc.screen.view()
			lines := strings.Split(view, "\n")

			if got, want := lines[len(lines)-1], tc.screen.keyHints(); got != want {
				t.Errorf("hints line = %q, want the full %q", got, want)
			}
			status := lines[len(lines)-2]
			if !strings.Contains(status, "selected") {
				t.Errorf("the status line above the hints is %q, want the selection count", status)
			}
			for i, line := range lines {
				if width := ansi.StringWidth(line); width > 80 {
					t.Errorf("line %d is %d cells wide, want at most 80: %q", i+1, width, line)
				}
			}
			if got := frameLines(view); got > 24 {
				t.Errorf("frame is %d lines, want at most 24", got)
			}
			for _, key := range []string{"enter", "esc", "ctrl+d"} {
				if !strings.Contains(tc.screen.keyHints(), key) {
					t.Errorf("hints %q do not mention %q", tc.screen.keyHints(), key)
				}
			}
		})
	}
}
