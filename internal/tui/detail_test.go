package tui

// Tests for US5.1: at 120 columns the dependency screen shows a detail region
// beside the list, in the metadata's own words, following the cursor. Below 120
// columns nothing changes. No test needs a terminal or a network.

import (
	"strings"
	"testing"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/miferdev/spring-inicializador/internal/catalog"
)

// down is the message the event loop delivers for the down arrow.
func down() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyDown} }

// sizedScreen builds the dependency screen at a terminal size.
func sizedScreen(t *testing.T, cat *catalog.Catalog, width, height int) dependenciesModel {
	t.Helper()
	return newDependenciesModel(cat, nil, width, height, Default())
}

// detailText is the region's content as one string, for the assertions that care
// about words rather than about wrapping.
func detailText(screen dependenciesModel) string {
	return strings.Join(screen.detailLines(detailWidth, dependenciesListHeight(screen.height)), " ")
}

// TestDetailShowsTheFocusedDependency is the story's core: name once, category,
// the full description wrapped, and the guide titles, from the metadata.
func TestDetailShowsTheFocusedDependency(t *testing.T) {
	cat := fixtureCatalog(t)
	screen := openRootCategory(t, sizedScreen(t, cat, 120, 30), "Web")

	if !screen.showDetail() {
		t.Fatal("120 columns did not split the body into list and detail")
	}
	if got, want := screen.listWidth(), 120-detailWidth-detailGap; got != want {
		t.Errorf("list width = %d, want %d beside a %d-column detail region", got, want, detailWidth)
	}

	dependency := mustDependency(t, cat, "web")
	detail := detailText(screen)

	if first := screen.detailLines(detailWidth, 27)[0]; first != dependency.Name {
		t.Errorf("detail starts with %q, want the name %q", first, dependency.Name)
	}
	if strings.Count(detail, dependency.Name) != 1 {
		t.Errorf("detail shows the name %d times, want it once: %q", strings.Count(detail, dependency.Name), detail)
	}
	// The category is the region's second line, not a word that happens to appear
	// in the name.
	if got := screen.detailLines(detailWidth, 27)[1]; got != dependency.Category {
		t.Errorf("detail's second line = %q, want the category %q", got, dependency.Category)
	}

	// The whole description, in the metadata's words: wrapped, never clipped.
	for _, word := range strings.Fields(dependency.Description) {
		if !strings.Contains(detail, word) {
			t.Errorf("detail lost the description word %q: %q", word, detail)
		}
	}
	if got := len(wrapText(dependency.Description, detailWidth)); got < 2 {
		t.Errorf("the description wrapped to %d line(s), want it to wrap in %d columns", got, detailWidth)
	}

	if !strings.Contains(detail, "Guides") {
		t.Errorf("detail has no Guides heading: %q", detail)
	}
	if len(dependency.Guides) != 3 {
		t.Fatalf("the fixture gives web %d guides, want the 3 this story measures", len(dependency.Guides))
	}
	for _, guide := range dependency.Guides {
		if !strings.Contains(detail, guide.Title) {
			t.Errorf("detail does not show the guide %q: %q", guide.Title, detail)
		}
	}

	// The two regions are side by side in the frame, and the separator sits in one
	// column: the detail region is exactly as wide as it was laid out to be.
	lines := strings.Split(screen.view(), "\n")
	for _, line := range lines[1 : len(lines)-2] {
		// The gap is " | ": the list takes listWidth cells, then a space, then
		// the separator.
		if got, want := strings.Index(line, "|"), screen.listWidth()+1; got != want {
			t.Errorf("separator at column %d, want %d: %q", got, want, line)
		}
		if width := ansi.StringWidth(line); width > 120 {
			t.Errorf("body line is %d cells wide, want at most 120: %q", width, line)
		}
	}
	for _, r := range strings.Join(screen.detailLines(detailWidth, 27), "") {
		if r > unicode.MaxASCII {
			t.Errorf("the detail region is not ASCII (found %q)", r)
			break
		}
	}
}

// TestDetailShowsOneGuideAndHidesAnEmptySection covers the three guide cases the
// fixture offers: three, one, and none.
func TestDetailShowsOneGuideAndHidesAnEmptySection(t *testing.T) {
	cat := fixtureCatalog(t)

	// data-jpa: one guide.
	sql := openRootCategory(t, sizedScreen(t, cat, 120, 30), "SQL")
	dataJPA := drive(t, drive(t, sql, down()), down())
	if got := focusedDependencyID(t, dataJPA); got != "data-jpa" {
		t.Fatalf("SQL focus = %q, want data-jpa", got)
	}
	detail := detailText(dataJPA)
	if !strings.Contains(detail, "Guides") {
		t.Errorf("data-jpa has no Guides heading: %q", detail)
	}
	if got := strings.Count(detail, " - "); got != 1 {
		t.Errorf("data-jpa shows %d guide titles, want 1: %q", got, detail)
	}
	if !strings.Contains(detail, "Accessing Data with JPA") {
		t.Errorf("data-jpa does not show its guide: %q", detail)
	}

	// dgs-codegen: no guides at all, and therefore no empty heading.
	tools := openRootCategory(t, sizedScreen(t, cat, 120, 30), "Developer Tools")
	dgs := drive(t, tools, down())
	if got := focusedDependencyID(t, dgs); got != "dgs-codegen" {
		t.Fatalf("Developer Tools focus = %q, want dgs-codegen", got)
	}
	detail = detailText(dgs)
	if strings.Contains(detail, "Guides") {
		t.Errorf("dgs-codegen rendered a Guides heading with no guides: %q", detail)
	}
	if !strings.Contains(detail, "GraphQL DGS Code Generation") {
		t.Errorf("dgs-codegen detail lost its name: %q", detail)
	}
	if !strings.Contains(detail, "Generate data types and type-safe APIs") {
		t.Errorf("dgs-codegen detail lost its description: %q", detail)
	}
}

// TestDetailFollowsTheCursor: moving down describes the row that is now focused.
func TestDetailFollowsTheCursor(t *testing.T) {
	cat := fixtureCatalog(t)
	screen := openRootCategory(t, sizedScreen(t, cat, 120, 30), "Web")

	first := detailText(screen)
	if !strings.Contains(first, "Build web, including RESTful") {
		t.Fatalf("the first row's detail is %q, want Spring Web's description", first)
	}

	moved := drive(t, screen, down())
	if got := focusedDependencyID(t, moved); got != "webflux" {
		t.Fatalf("focus = %q, want webflux", got)
	}
	second := detailText(moved)
	if first == second {
		t.Fatal("the detail did not change with the cursor")
	}
	if want := mustDependency(t, cat, "webflux").Description; !strings.Contains(second, want) {
		t.Errorf("detail after moving = %q, want webflux's description %q", second, want)
	}
	if strings.Contains(second, "Build web, including RESTful") {
		t.Errorf("detail still shows the previous dependency: %q", second)
	}
}

// TestDetailFollowsAFilteredResult: the region describes the focused row of the
// filtered list, so a search reads before it picks.
func TestDetailFollowsAFilteredResult(t *testing.T) {
	cat := fixtureCatalog(t)
	applied := searchFor(t, sizedScreen(t, cat, 120, 30), "postgres")

	if got := focusedDependencyID(t, applied); got != "postgresql" {
		t.Fatalf("first result = %q, want postgresql", got)
	}
	if detail := detailText(applied); !strings.Contains(detail, "A JDBC and R2DBC driver") {
		t.Errorf("the first result's detail = %q, want the PostgreSQL driver's description", detail)
	}

	moved := drive(t, applied, down())
	if got := focusedDependencyID(t, moved); got != "spring-ai-postgresml" {
		t.Fatalf("second result = %q, want spring-ai-postgresml", got)
	}
	if detail := detailText(moved); !strings.Contains(detail, "Spring AI support for the PostgresML") {
		t.Errorf("the second result's detail = %q, want PostgresML's description", detail)
	}
	if view := moved.view(); !strings.Contains(view, "PostgresML") {
		t.Errorf("the frame does not show the focused result's detail; view:\n%s", view)
	}
}

// TestNoDetailBelow120 is the compatibility half: 80 columns keeps the shape the
// screen had before this story, with no detail region and no separator column.
func TestNoDetailBelow120(t *testing.T) {
	cat := fixtureCatalog(t)
	screen := openRootCategory(t, sizedScreen(t, cat, 80, 24), "Web")

	if screen.showDetail() {
		t.Error("80 columns split the body, want the full-width list")
	}
	if got := screen.listWidth(); got != 80 {
		t.Errorf("list width = %d at 80 columns, want the whole terminal", got)
	}

	lines := strings.Split(screen.view(), "\n")
	if got, want := lines[0], "< Web (18)  23 categories  206 dependencies"; got != want {
		t.Errorf("header = %q, want %q", got, want)
	}
	// The row itself is unchanged; the component pads the frame's lines to the
	// widest row, which is why the check trims the padding.
	if got := strings.TrimRight(lines[1], " "); got != "> [ ] Spring Web Web" {
		t.Errorf("first body line = %q, want the row as it was before this story", got)
	}
	if strings.Contains(lines[1], "|") {
		t.Errorf("the row carries a separator at 80 columns: %q", lines[1])
	}

	view := screen.view()
	if strings.Contains(view, "Build web, including RESTful") || strings.Contains(view, "Guides") {
		t.Errorf("the detail content leaked into the narrow frame; view:\n%s", view)
	}
	// The old frame shape: header, 18 rows, status, hints.
	if got := frameLines(view); got != 21 {
		t.Errorf("frame is %d lines at 80x24 for Web, want the 21 it had before", got)
	}
}

// TestDetailWrapsAndStaysWithinTheTerminal covers the layout rule: the description
// reflows to the region, nothing is lost, and the frame never grows past the
// terminal.
func TestDetailWrapsAndStaysWithinTheTerminal(t *testing.T) {
	cat := fixtureCatalog(t)
	screen := openRootCategory(t, sizedScreen(t, cat, 120, 30), "Web")

	dependency := mustDependency(t, cat, "web")
	wrapped := wrapText(dependency.Description, detailWidth)
	if len(wrapped) < 2 {
		t.Fatalf("the description wrapped to %d line(s), want it to wrap", len(wrapped))
	}
	for _, line := range wrapped {
		if width := ansi.StringWidth(line); width > detailWidth {
			t.Errorf("wrapped line is %d cells wide, want at most %d: %q", width, detailWidth, line)
		}
	}

	if got := frameLines(screen.view()); got > 30+1 {
		t.Errorf("frame is %d lines at 120x30, want at most 31", got)
	}

	// A description that would not fit the region's height is clipped to it, so a
	// long text can never push the frame past the terminal.
	clipped := screen.detailLines(detailWidth, 5)
	if len(clipped) > 5 {
		t.Errorf("detail rows = %d with a height of 5, want the region clipped", len(clipped))
	}

	// A word wider than the region is cut with "...", never silently.
	long := wrapText("supercalifragilisticexpialidocious", 10)
	if len(long) != 1 || !strings.HasSuffix(long[0], "...") {
		t.Errorf("a long word wrapped to %q, want one line ending in ...", long)
	}
}

// focusedDependencyID is the id of the dependency under the cursor, for the tests
// that care which row the detail is describing.
func focusedDependencyID(t *testing.T, screen dependenciesModel) string {
	t.Helper()

	dependency, ok := screen.focusedDependency()
	if !ok {
		t.Fatal("no dependency is focused")
	}
	return dependency.ID
}
