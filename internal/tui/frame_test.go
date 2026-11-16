package tui

// Tests for US3.3: the dependency screen renders the window, never the catalog.
// The row delegate counts what it actually writes, so these tests measure the
// component rather than trusting its reputation, and the frame-height and
// frame-cost guards catch the regressions the counts cannot see. No test needs a
// terminal: the screen is driven with the messages the event loop delivers.

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/miferdev/spring-inicializador/internal/catalog"
)

// countRenders arms the row delegate with counter and returns the screen, so a
// test can count the rows one frame writes. It lives here because only tests use
// it: the screen itself never counts and never reads the counter back.
func (d dependenciesModel) countRenders(counter *int) dependenciesModel {
	d.renders = counter
	d.list.SetDelegate(d.rowDelegate())
	return d
}

// dependencyScreen builds the screen at a terminal size, on the level that holds
// the whole catalog: the frame budget is about the 206 dependency rows, not the
// 23 category rows the screen opens on (US4.1).
func dependencyScreen(t *testing.T, cat *catalog.Catalog, width, height int) dependenciesModel {
	t.Helper()
	return newDependenciesModel(cat, nil, width, height, Default()).show(levelSearch)
}

// renderedRows renders one frame and reports how many rows the delegate wrote.
func renderedRows(screen dependenciesModel, counter *int) int {
	*counter = 0
	_ = screen.view()
	return *counter
}

// assertRenderedRows is the story's bound: a frame writes the rows that fit, not
// one per catalog row.
func assertRenderedRows(t *testing.T, screen dependenciesModel, counter *int, where string) {
	t.Helper()

	total := len(screen.dependencies)
	limit := screen.list.Paginator.PerPage + 1

	rows := renderedRows(screen, counter)
	switch {
	case rows == 0:
		t.Errorf("one frame at the %s rendered no rows", where)
	case rows > limit:
		t.Errorf("one frame at the %s rendered %d rows, want at most %d (the window plus one line)", where, rows, limit)
	case rows >= total:
		t.Errorf("one frame at the %s rendered %d rows, want fewer than the %d in the catalog", where, rows, total)
	}
}

// drive delivers one message and the commands it returns, the way the event loop
// does, so a filter is really applied by the time a frame is counted.
func drive(t *testing.T, screen dependenciesModel, msg tea.Msg) dependenciesModel {
	t.Helper()

	queue := []tea.Msg{msg}
	for steps := 0; len(queue) > 0; steps++ {
		if steps > 200 {
			t.Fatal("the screen kept producing messages")
		}

		updated, cmd, _ := screen.update(queue[0])
		screen = updated
		queue = queue[1:]
		queue = append(queue, commandMessages(cmd)...)
	}
	return screen
}

// frameLines is how many lines a frame occupies.
func frameLines(view string) int {
	if view == "" {
		return 0
	}
	return strings.Count(view, "\n") + 1
}

// TestFrameRendersOnlyTheVisibleWindow is the story's core: one View calls the row
// delegate once per row that fits, not once per catalog row.
func TestFrameRendersOnlyTheVisibleWindow(t *testing.T) {
	cat := fixtureCatalog(t)
	if len(cat.Dependencies) != 206 {
		t.Fatalf("the fixture has %d dependencies, want the 206 this story measures", len(cat.Dependencies))
	}

	screen := dependencyScreen(t, cat, 80, 24)
	counter := 0
	screen = screen.countRenders(&counter)

	assertRenderedRows(t, screen, &counter, "first page")
}

// TestWindowStaysBoundedWhileScrolling proves the bound does not come from the
// first page being lucky: the middle and the end of the catalog render the same
// window, and the last page is not padded by rendering the rest of the rows.
func TestWindowStaysBoundedWhileScrolling(t *testing.T) {
	cat := fixtureCatalog(t)
	screen := dependencyScreen(t, cat, 80, 24)
	counter := 0
	screen = screen.countRenders(&counter)

	// The middle: four or five pages in.
	for range 100 {
		screen = drive(t, screen, tea.KeyMsg{Type: tea.KeyDown})
	}
	if got := screen.list.Index(); got != 100 {
		t.Fatalf("cursor = %d after 100 downs, want 100", got)
	}
	assertRenderedRows(t, screen, &counter, "middle of the catalog")

	// The end: the last page holds fewer rows than the window, so the component
	// pads it instead of reaching further into the catalog.
	screen = drive(t, screen, tea.KeyMsg{Type: tea.KeyEnd})
	if got, want := screen.list.Index(), len(cat.Dependencies)-1; got != want {
		t.Fatalf("cursor = %d at the end, want %d", got, want)
	}
	assertRenderedRows(t, screen, &counter, "end of the catalog")
}

// TestWindowStaysBoundedForTheAICategory covers the watch-out AGENTS.md names: the
// AI category is 50 rows, more than twice the window. It needs no special case,
// so the assertion is the same generic bound every other filter gets.
func TestWindowStaysBoundedForTheAICategory(t *testing.T) {
	cat := fixtureCatalog(t)

	ai := make(map[string]bool, 50)
	for _, dependency := range cat.Dependencies {
		if dependency.Category == "AI" {
			ai[dependency.ID] = true
		}
	}
	if len(ai) != 50 {
		t.Fatalf("the fixture's AI category has %d rows, want the 50 this story measures", len(ai))
	}

	screen := dependencyScreen(t, cat, 80, 24)
	screen = drive(t, screen, keyRunes("/"))
	screen = drive(t, screen, keyRunes("ai"))
	screen = drive(t, screen, tea.KeyMsg{Type: tea.KeyEnter})

	// The premise: the query is really applied and really shows the whole AI
	// category, so the frame below is the 50-row case and not a smaller accident.
	if !screen.list.IsFiltered() {
		t.Fatal("the ai query was not applied")
	}
	visible := screen.list.VisibleItems()
	if len(visible) >= len(cat.Dependencies) {
		t.Fatalf("filter ai shows all %d rows, so it is not filtering", len(visible))
	}
	if len(visible) < len(ai) {
		t.Fatalf("filter ai shows %d rows, want at least the %d AI rows", len(visible), len(ai))
	}
	shown := make(map[string]bool, len(visible))
	for _, item := range visible {
		if row, ok := item.(dependencyItem); ok {
			shown[row.dependency.ID] = true
		}
	}
	for id := range ai {
		if !shown[id] {
			t.Fatalf("filter ai does not show %q, so the AI category is not on screen", id)
		}
	}

	counter := 0
	screen = screen.countRenders(&counter)
	assertRenderedRows(t, screen, &counter, "AI category filter")
}

// TestFrameHeightStaysWithinTheTerminal checks the other half of "the window is
// the frame": whatever the page holds, the frame is the terminal's height and no
// more. The last page matters most, because the component pads it with blank
// lines rather than with rows.
func TestFrameHeightStaysWithinTheTerminal(t *testing.T) {
	cat := fixtureCatalog(t)

	for _, size := range []struct{ width, height int }{{80, 24}, {120, 30}} {
		screen := dependencyScreen(t, cat, size.width, size.height)

		if got, limit := frameLines(screen.view()), size.height+1; got > limit {
			t.Errorf("%dx%d first page occupies %d lines, want at most %d", size.width, size.height, got, limit)
		}

		screen = drive(t, screen, tea.KeyMsg{Type: tea.KeyEnd})
		if got, limit := frameLines(screen.view()), size.height+1; got > limit {
			t.Errorf("%dx%d last page occupies %d lines, want at most %d", size.width, size.height, got, limit)
		}
	}
}

// TestFrameCostStaysBoundedWithTheFullCatalog is the coarse guard the story asks
// for: a frame at 206 rows must not cost what rendering the whole catalog costs.
// It is not a benchmark, and two things keep it honest on a loaded machine.
//
//   - The budget is relative: the same screen over a 10-row catalog renders ten
//     rows instead of twenty-two, and costs about the same per frame, because the
//     frame's fixed work dominates. A full-catalog render costs roughly six times
//     more once the catalog holds 206 rows (measured on this machine: 86µs vs
//     503µs per frame, and 582µs vs 3.8ms under -race), so three times the
//     small-catalog cost separates the two without depending on machine speed.
//     The smallest of several runs is used, so a machine busy with something else
//     cannot inflate the baseline.
//   - An absolute ceiling remains, absurdly generous, so a render that hangs or
//     turns quadratic still fails.
func TestFrameCostStaysBoundedWithTheFullCatalog(t *testing.T) {
	cat := fixtureCatalog(t)
	if len(cat.Dependencies) < 206 {
		t.Fatalf("the fixture has %d dependencies, want the 206 this story measures", len(cat.Dependencies))
	}

	// The control: the same terminal, the same screen, a catalog small enough
	// that any per-catalog-row work in a frame shows up as a difference.
	full := dependencyScreen(t, cat, 80, 24)
	small := dependencyScreen(t, &catalog.Catalog{Dependencies: cat.Dependencies[:10]}, 80, 24)

	const frames = 200
	fullCost := cheapestOf(2, func() time.Duration { return timeFrames(full, frames) })
	smallCost := cheapestOf(2, func() time.Duration { return timeFrames(small, frames) })

	t.Logf("%d frames at 206 rows: %v (%v per frame); at 10 rows: %v (%v per frame)",
		frames, fullCost, fullCost/frames, smallCost, smallCost/frames)

	if ceiling := 5 * time.Second; fullCost > ceiling {
		t.Errorf("%d frames at 206 rows took %v, want under %v", frames, fullCost, ceiling)
	}

	// A relative budget, with a millisecond of floor so two fast measurements
	// cannot divide into noise.
	budget := 3*smallCost + time.Millisecond
	if fullCost > budget {
		t.Errorf("%d frames at 206 rows took %v; the same screen over 10 rows took %v, so the frame is doing work per catalog row instead of per row on screen",
			frames, fullCost, smallCost)
	}
}

// timeFrames renders frames frames and reports how long they took.
func timeFrames(screen dependenciesModel, frames int) time.Duration {
	start := time.Now()
	for range frames {
		_ = screen.view()
	}
	return time.Since(start)
}

// cheapestOf measures what repeat times and keeps the smallest, which is how a
// timing survives a machine that is busy with something else.
func cheapestOf(repeat int, measure func() time.Duration) time.Duration {
	best := measure()
	for range repeat - 1 {
		if got := measure(); got < best {
			best = got
		}
	}
	return best
}
