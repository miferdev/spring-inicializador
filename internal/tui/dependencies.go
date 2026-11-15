package tui

// Screen 2 of the wizard (docs/ui.md): the flat, searchable dependency list over
// the catalog's 206 rows. It is built on the bubbles list component, which owns
// the visible window, so this file only describes a row, the search, the checked
// set and the keys the screen must decide for itself.
//
// The screen owns one value: the checked set (US3.2). It is a set of ids, not a
// property of the visible rows, so filtering can neither drop nor duplicate a
// selection. US3.3 measures the window and VS4 adds the category level above
// this list.

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

	// selected is the checked set, keyed by dependency id. Nothing derives it
	// from the visible rows, which is why a filter change cannot drop or
	// duplicate a selection.
	selected map[string]struct{}

	list  list.Model
	theme Theme
}

// newDependenciesModel builds the screen over the flat catalog. seeded carries
// the ids the command line already answered (--deps), which arrive checked, so
// that flag answers this screen the way the project flags answer form rows.
// width and height are the terminal size; the footer takes one row of it.
func newDependenciesModel(cat *catalog.Catalog, seeded []string, width, height int, theme Theme) dependenciesModel {
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
	selected := newSelection(seeded)

	listWidth, listHeight := dependenciesListSize(width, height)
	screen := list.New(items, dependencyDelegate{theme: theme, selected: selected}, listWidth, listHeight)
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

	return dependenciesModel{dependencies: dependencies, selected: selected, list: screen, theme: theme}
}

// resize applies a terminal size, keeping the row the footer takes.
func (d dependenciesModel) resize(width, height int) dependenciesModel {
	d.list.SetSize(dependenciesListSize(width, height))
	return d
}

// dependenciesListSize is the space the list component gets: the terminal minus
// the footer row. The floor keeps the component's paginator arithmetic sane on a
// terminal that reports almost nothing.
func dependenciesListSize(width, height int) (int, int) {
	return max(1, width), max(2, height) - 1
}

// newSelection turns the ids the command line already answered into the checked
// set. An id the catalog does not publish still enters the set: a typo in --deps
// has to reach the server exactly as it does today, not disappear quietly from
// the request.
func newSelection(seeded []string) map[string]struct{} {
	if len(seeded) == 0 {
		return nil
	}

	selected := make(map[string]struct{}, len(seeded))
	for _, id := range seeded {
		if id != "" {
			selected[id] = struct{}{}
		}
	}
	return selected
}

// withSelection returns the screen with selected as its set, and hands the set to
// the row delegate. The list keeps its delegate by value and never asks it to
// update, so every change has to be pushed into it.
func (d dependenciesModel) withSelection(selected map[string]struct{}) dependenciesModel {
	d.selected = selected
	d.list.SetDelegate(dependencyDelegate{theme: d.theme, selected: selected})
	return d
}

// toggle flips the check on id. The set is copied rather than mutated in place,
// so a screen value the root model has already handed out goes on describing the
// rows it described.
func (d dependenciesModel) toggle(id string) dependenciesModel {
	next := make(map[string]struct{}, len(d.selected)+1)
	for checked := range d.selected {
		next[checked] = struct{}{}
	}
	if _, ok := next[id]; ok {
		delete(next, id)
	} else if id != "" {
		next[id] = struct{}{}
	}
	return d.withSelection(next)
}

// toggleFocused flips the row under the cursor and leaves the cursor where it
// is: the user is checking several rows and must not chase the list.
func (d dependenciesModel) toggleFocused() dependenciesModel {
	item, ok := d.list.SelectedItem().(dependencyItem)
	if !ok {
		// Nothing is visible, so there is no row to check.
		return d
	}
	return d.toggle(item.dependency.ID)
}

// chosen is the checked set as the request needs it: ids in catalog order, so two
// identical selections always produce the same request and --dry-run stays
// diffable. An id the catalog does not publish can only come from --deps; it has
// no catalog position, so it follows the known ids in lexical order.
func (d dependenciesModel) chosen() []string {
	if len(d.selected) == 0 {
		return nil
	}

	chosen := make([]string, 0, len(d.selected))
	for _, dependency := range d.dependencies {
		if _, ok := d.selected[dependency.ID]; ok {
			chosen = append(chosen, dependency.ID)
		}
	}
	if len(chosen) == len(d.selected) {
		// Every checked id is a row of the catalog, so the catalog order above is
		// the whole answer; the common case allocates nothing else.
		return chosen
	}

	known := make(map[string]struct{}, len(chosen))
	for _, id := range chosen {
		known[id] = struct{}{}
	}
	var unknown []string
	for id := range d.selected {
		if _, ok := known[id]; !ok {
			unknown = append(unknown, id)
		}
	}
	sort.Strings(unknown)

	return append(chosen, unknown...)
}

// view renders the list window and the one footer line this screen owns. The
// component renders only the current page, which is the bounded window US3.3
// measures; nothing here asks for the whole slice.
func (d dependenciesModel) view() string {
	view := d.list.View()
	footer := d.footer()
	if footer == "" {
		return view
	}
	return view + "\n" + d.theme.Muted.Render(footer)
}

// footer is the one line under the list: what the search found, and what is
// checked. Both halves are ASCII and the line never wraps, whatever the terminal
// width says.
func (d dependenciesModel) footer() string {
	ids := d.chosen()
	result := d.resultLine()
	if result == "" && len(ids) == 0 {
		// Nothing to report; the component already says "No dependencies."
		return ""
	}

	prefix := result
	if prefix != "" {
		prefix += footerGap
	}

	width := d.list.Width()
	line := prefix + selectionLine(ids, width-ansi.StringWidth(prefix))
	if width > 0 {
		// The safety net for a pathological width: the count survives because it
		// is at the front, and nothing here can overflow the terminal.
		line = ansi.Truncate(line, width, "...")
	}
	return line
}

// footerGap separates the search half of the footer from the selection half.
const footerGap = "  "

// resultLine is the first half of the footer: the row count, or what the query
// matched, or why the result is empty. It is empty only for an empty catalog,
// which the component already explains.
func (d dependenciesModel) resultLine() string {
	total := len(d.dependencies)
	if total == 0 {
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

// selectionLine is the second half of the footer: "N selected" and, once
// something is checked, its ids. The count is always exact — it is the number
// the user needs — and the ids are what gives way when the budget runs out:
// whole ids are dropped from the end and "..." marks the cut, so the line never
// wraps and never shows half an id.
func selectionLine(ids []string, budget int) string {
	head := fmt.Sprintf("%d selected", len(ids))
	if len(ids) == 0 {
		return head
	}

	line := head + ": "
	shown := 0
	for i, id := range ids {
		separator := ""
		if i > 0 {
			separator = ", "
		}
		candidate := line + separator + id

		// The mark of the cut has to fit before the id is accepted, or the line
		// would end up one id short and still overflow.
		tail := ""
		if i < len(ids)-1 {
			tail = ", ..."
		}
		if ansi.StringWidth(candidate)+ansi.StringWidth(tail) > budget {
			break
		}

		line = candidate
		shown = i + 1
	}

	switch {
	case shown == len(ids):
		return line
	case shown > 0:
		return line + ", ..."
	case ansi.StringWidth(head+": ...") <= budget:
		return head + ": ..."
	default:
		// Not even the mark of the cut fits: the count is the part that must
		// survive, and the footer truncates whatever is left.
		return head
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
// this screen decides are the ones the component cannot know: esc is level
// aware, space checks the focused row, and enter finishes the run. docs/ui.md's
// keybinding table still says enter toggles, while the screen footer in the same
// file says "enter done"; the footer and VS3's proof both mean done, so that is
// what enter does (US3.2).
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

		case tea.KeySpace:
			if d.list.SettingFilter() {
				// While the query is being typed a space is a character, not a
				// toggle: the key belongs to the filter until the list is
				// browsing again.
				break
			}
			return d.toggleFocused(), nil, actionStay

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

// dependencyDelegate renders one dependency per line: a focus marker, an ASCII
// checkbox, the name and the category as a muted label. One line per row keeps
// the window cheap to fill, which is the shape US3.3 measures.
type dependencyDelegate struct {
	theme Theme

	// selected is the checked set the rows read. The list keeps its delegate by
	// value and never asks it to update, so every change to the set is pushed in
	// through dependenciesModel.withSelection.
	selected map[string]struct{}
}

// Height is one line per row.
func (dependencyDelegate) Height() int { return 1 }

// Spacing is fewer rows on screen, not more: a gap costs a row.
func (dependencyDelegate) Spacing() int { return 0 }

// Update is a no-op: a row owns no state, the screen owns the set.
func (dependencyDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

// Render writes one row.
func (d dependencyDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	row, ok := item.(dependencyItem)
	if !ok {
		return
	}

	// "> [x] name category": the two markers, the gap between them and the gap
	// before the category. The name is what gives way, because a wrapped row
	// would break the component's row arithmetic.
	const chrome = 7

	label := d.theme.Muted.Render(row.dependency.Category)
	name := row.dependency.Name
	if budget := m.Width() - ansi.StringWidth(label) - chrome; budget > 0 {
		name = ansi.Truncate(name, budget, "...")
	}

	_, checked := d.selected[row.dependency.ID]
	fmt.Fprintf(w, "%s %s %s %s", focusMarker(index == m.Index()), checkMark(checked), name, label)
}

// checkMark is the box a row carries: [x] for a picked dependency, [ ] for the
// rest. It is the only mark of selection, so a terminal that strips every style
// (--no-color, VS5) still shows what is picked.
func checkMark(checked bool) string {
	if checked {
		return "[x]"
	}
	return "[ ]"
}

// focusMarker says which row the cursor is on. It is focus, not selection: the
// checkbox is what says a dependency is picked.
func focusMarker(focused bool) string {
	if focused {
		return ">"
	}
	return " "
}
