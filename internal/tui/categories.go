package tui

// The category level of the dependency screen (US4.1): the root list, its display
// ordering, and the one delegate that shows a name with its right-aligned count.
//
// Display order is D7: dependency count descending, ties alphabetical. It lives
// here because it is a UI decision; catalog.Parse keeps the server's order and
// never sorts.

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/miferdev/spring-inicializador/internal/catalog"
)

// categoryItem is one row of the root: a category and how many dependencies it
// holds.
type categoryItem struct{ category catalog.Category }

// FilterValue is the row's human identity, which the component asks of every
// item. Searching never runs on this list: / leaves the root for the flat
// catalog before a filter starts, because a search scoped to a category would
// hide results the user cannot see they are missing (D5).
func (i categoryItem) FilterValue() string { return i.category.Name }

// rankCategories orders categories for display: the largest first, ties
// alphabetical (D7). It copies the slice, because the server's order is catalog's
// to keep and this order is the screen's to choose.
func rankCategories(categories []catalog.Category) []catalog.Category {
	ordered := append([]catalog.Category(nil), categories...)

	sort.SliceStable(ordered, func(i, j int) bool {
		left, right := ordered[i], ordered[j]
		if len(left.Dependencies) != len(right.Dependencies) {
			return len(left.Dependencies) > len(right.Dependencies)
		}
		return left.Name < right.Name
	})

	return ordered
}

// categoryCount is a category count with the right noun.
func categoryCount(n int) string {
	if n == 1 {
		return "1 category"
	}
	return fmt.Sprintf("%d categories", n)
}

// categoryDelegate renders one category per line: a focus marker, the name, and
// the number of dependencies it holds, right-aligned. The count is muted like
// every other secondary label, and the name is what gives way when the line is
// too narrow for both.
type categoryDelegate struct{ theme Theme }

// Height is one line per row.
func (categoryDelegate) Height() int { return 1 }

// Spacing is fewer rows on screen, not more: a gap costs a row.
func (categoryDelegate) Spacing() int { return 0 }

// Update is a no-op: a category row owns no state.
func (categoryDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

// Render writes one row.
func (d categoryDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	row, ok := item.(categoryItem)
	if !ok {
		return
	}

	// "> SQL              (19)": the marker, a space, the name, the padding that
	// pushes the count to the edge, and the count. The count never gives way:
	// the name truncates to keep it.
	const (
		markers = 2 // the focus marker and the space after it
		minGap  = 1
	)

	count := d.theme.Muted.Render(fmt.Sprintf("(%d)", len(row.category.Dependencies)))
	name := row.category.Name
	if budget := m.Width() - markers - ansi.StringWidth(count) - minGap; budget > 0 {
		name = ansi.Truncate(name, budget, "...")
	}

	padding := m.Width() - markers - ansi.StringWidth(name) - ansi.StringWidth(count)
	if padding < minGap {
		padding = minGap
	}

	fmt.Fprintf(w, "%s %s%s%s", focusMarker(index == m.Index()), name, strings.Repeat(" ", padding), count)
}
