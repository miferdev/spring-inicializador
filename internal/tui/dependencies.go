package tui

// Screen 2 of the wizard (docs/ui.md): the flat, searchable dependency list over
// the catalog's 206 rows. It is built on the bubbles list component, which owns
// the visible window, so this file only describes a row, the search and the two
// navigation keys the screen must decide for itself.
//
// The screen changes no project value yet: US3.2 adds selection, US3.3 measures
// the window and VS4 adds the category level above this list.

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/miferdev/spring-inicializador/internal/catalog"
)

// The screen's own copy. It is English like the rest of the interface and lives
// here because VS5 owns the styles, not the words.
const dependenciesSearchPrompt = "Search: "

// dependencyItem is one row: the catalog value, and the single string the list
// interface asks of every item.
type dependencyItem struct{ dependency catalog.Dependency }

// FilterValue is the row's human identity. The list's own fuzzy filter does not
// run on this screen — rankDependencies does, because it needs the id, the name
// and the category separately — but the interface requires a value, and the name
// is what a filter would sensibly match.
func (i dependencyItem) FilterValue() string { return i.dependency.Name }

// dependenciesModel is the dependency screen.
type dependenciesModel struct {
	// dependencies is the flat catalog in server order, and row i of the list is
	// dependencies[i]. The list is built once and never reordered, which is what
	// lets rankDependencies answer with positions instead of values.
	dependencies []catalog.Dependency
	list         list.Model
	theme        Theme
}

// newDependenciesModel builds the screen over the flat catalog. width and height
// are the terminal size; the status line takes one row of it.
func newDependenciesModel(cat *catalog.Catalog, width, height int, theme Theme) dependenciesModel {
	var dependencies []catalog.Dependency
	if cat != nil {
		dependencies = cat.Dependencies
	}

	items := make([]list.Item, 0, len(dependencies))
	for _, dependency := range dependencies {
		items = append(items, dependencyItem{dependency: dependency})
	}

	// The list hands the filter one target per row, in row order, and every rank
	// we return is an index into that same slice. The rows are built from
	// dependencies once, so the capture below stays aligned with the list for the
	// life of the screen.
	listWidth, listHeight := dependenciesListSize(width, height)
	screen := list.New(items, dependencyDelegate{theme: theme}, listWidth, listHeight)
	// The component's own chrome (title, status bar, pagination dots, help) is
	// off: the dots are Unicode, the screen owns its one footer line, and the
	// key hints belong to the root screen. Pagination still pages; only its
	// rendering is hidden.
	screen.SetShowTitle(false)
	screen.SetShowStatusBar(false)
	screen.SetShowPagination(false)
	screen.SetShowHelp(false)
	screen.SetStatusBarItemName("dependency", "dependencies")
	screen.DisableQuitKeybindings()
	screen.FilterInput.Prompt = dependenciesSearchPrompt
	screen.Filter = func(term string, _ []string) []list.Rank {
		positions := rankDependencies(term, dependencies)
		ranks := make([]list.Rank, 0, len(positions))
		for _, position := range positions {
			ranks = append(ranks, list.Rank{Index: position})
		}
		return ranks
	}

	return dependenciesModel{dependencies: dependencies, list: screen, theme: theme}
}

// resize applies a terminal size, keeping the row the status line takes.
func (d dependenciesModel) resize(width, height int) dependenciesModel {
	d.list.SetSize(dependenciesListSize(width, height))
	return d
}

// dependenciesListSize is the space the list component gets: the terminal minus
// the status line row. The floor keeps the component's paginator arithmetic
// sane on a terminal that reports almost nothing.
func dependenciesListSize(width, height int) (int, int) {
	return max(1, width), max(2, height) - 1
}

// view renders the list window and the one status line this screen owns. The
// component renders only the current page, which is the bounded window US3.3
// measures; nothing here asks for the whole slice.
func (d dependenciesModel) view() string {
	view := d.list.View()
	status := d.statusLine()
	if status == "" {
		return view
	}
	return view + "\n" + d.theme.Muted.Render(status)
}

// statusLine is the line under the list. It replaces the component's own status
// bar so the wording is this tool's: an empty result names the query and says
// how to clear it, which the component only reports as "Nothing matched".
func (d dependenciesModel) statusLine() string {
	total := len(d.dependencies)
	if total == 0 {
		// The component already says "No dependencies." for an empty catalog.
		return ""
	}

	query := strings.TrimSpace(d.list.FilterValue())
	visible := len(d.list.VisibleItems())

	switch {
	case query == "":
		return dependencyCount(total)
	case visible == 0:
		return fmt.Sprintf("No dependency matches %q; esc clears the search.", query)
	default:
		return fmt.Sprintf("Showing %d of %d for %q", visible, total, query)
	}
}

// dependencyCount is a row count with the right noun.
func dependencyCount(n int) string {
	if n == 1 {
		return "1 dependency"
	}
	return fmt.Sprintf("%d dependencies", n)
}

// dependencyAction is what the screen asks the root model to do after one
// message. The screen holds no navigation decision of its own: the root model
// owns the screen enum.
type dependencyAction int

const (
	actionStay dependencyAction = iota
	actionBack
	actionFinish
)

// update handles one message and reports what the root must do next. The keys
// this screen decides are the two whose meaning the component cannot know:
// esc is level aware, and enter finishes the run, which is the wizard's exit
// until US3.2 gives rows a selection and VS6 adds the review step.
func (d dependenciesModel) update(msg tea.Msg) (dependenciesModel, tea.Cmd, dependencyAction) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.Type {
		case tea.KeyEsc:
			// A query on screen is what esc is for here: clearing it keeps the
			// list where the user left it (docs/ui.md). Only an empty query goes
			// back to the form.
			if d.querying() {
				d.list.ResetFilter()
				return d, nil, actionStay
			}
			return d, nil, actionBack

		case tea.KeyEnter:
			if d.list.SettingFilter() {
				if len(d.list.VisibleItems()) == 0 {
					// The component would reset the filter here, dropping the
					// query the user is still editing. Keep it and stay.
					return d, nil, actionStay
				}
				break // the component applies the filter
			}
			return d, nil, actionFinish

		case tea.KeyCtrlC:
			return d, tea.Interrupt, actionStay
		}
	}

	updated, cmd := d.list.Update(msg)
	d.list = updated
	return d, cmd, actionStay
}

// querying reports whether a search is on screen, applied or still being typed.
func (d dependenciesModel) querying() bool {
	return d.list.FilterValue() != "" || d.list.SettingFilter()
}

// dependencyDelegate renders one dependency per line: a focus marker, the name,
// and the category as a muted label. One line per row keeps the window cheap to
// fill, which is the shape US3.3 measures.
type dependencyDelegate struct{ theme Theme }

// Height is one line per row.
func (dependencyDelegate) Height() int { return 1 }

// Spacing is fewer rows on screen, not more: a gap costs a row.
func (dependencyDelegate) Spacing() int { return 0 }

// Update is a no-op: a row owns no state of its own until US3.2 adds selection.
func (dependencyDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

// Render writes one row.
func (d dependencyDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	row, ok := item.(dependencyItem)
	if !ok {
		return
	}

	// "> name category": the marker and the two gaps around the name. The name
	// is what gives way, because a wrapped row would break the component's row
	// arithmetic.
	const chrome = 3

	label := d.theme.Muted.Render(row.dependency.Category)
	name := row.dependency.Name
	if budget := m.Width() - ansi.StringWidth(label) - chrome; budget > 0 {
		name = ansi.Truncate(name, budget, "...")
	}

	fmt.Fprintf(w, "%s %s %s", focusMarker(index == m.Index()), name, label)
}

// focusMarker says which row the cursor is on. A row is focused, never selected:
// space does nothing until US3.2 adds toggling, and the marker must not claim
// otherwise.
func focusMarker(focused bool) string {
	if focused {
		return ">"
	}
	return " "
}
