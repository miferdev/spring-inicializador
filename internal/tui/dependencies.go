package tui

// Screen 2 of the wizard (docs/ui.md): the dependency panel, one list component
// with two levels (D4). The root shows the categories, `enter` opens one, `esc`
// backs out, and `/` flattens the whole catalog from either level (D5). It is
// built on the bubbles list component, which owns the visible window, so this
// file only describes a level, a row, the checked set and the keys the screen
// must decide for itself.
//
// The screen owns one value: the checked set (US3.2). It is a set of ids, not a
// property of the visible rows, so filtering or changing level can neither drop
// nor duplicate a selection. The category rows live in categories.go, the search
// ranking in rank.go and the window budget in frame_test.go.

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

	// categories is the root's rows, already in display order (D7). It is a view
	// of the catalog, which keeps the server's order.
	categories []categoryItem

	// level is what the screen shows, and open names the category the
	// dependencies level belongs to. searchFrom is the level a search started
	// from, so leaving the search returns there instead of to the root.
	level      dependencyLevel
	open       string
	searchFrom dependencyLevel

	// rootCursor is the root row the last category was opened from, so esc comes
	// back to it instead of to the top (US4.2). Only the root remembers a
	// position: one cursor per category would be the navigation stack this screen
	// does not build.
	rootCursor int

	// selected is the checked set, keyed by dependency id. Nothing derives it
	// from the visible rows, which is why a filter change cannot drop or
	// duplicate a selection.
	selected map[string]struct{}

	// renders is the counter the frame tests point the row delegate at, so they
	// can count the rows one frame writes (US3.3). It is nil in the program: the
	// counter never reaches the screen, and the delegate's guard is one branch
	// per rendered row. It lives here so a selection change, which rebuilds the
	// delegate, cannot silently disarm the probe.
	renders *int

	// width and height are the terminal's, not the list's: the header and the two
	// footer lines span the whole width, and a wide terminal splits the body into
	// the list and the detail region (US5.1).
	width  int
	height int

	list  list.Model
	theme Theme
}

// newDependenciesModel builds the screen over the catalog, opening on the
// category root. seeded carries the ids the command line already answered
// (--deps), which arrive checked, so that flag answers this screen the way the
// project flags answer form rows. width and height are the terminal size; the
// header and the footer take one row each.
func newDependenciesModel(cat *catalog.Catalog, seeded []string, width, height int, theme Theme) dependenciesModel {
	var dependencies []catalog.Dependency
	var categories []catalog.Category
	if cat != nil {
		dependencies = cat.Dependencies
		categories = cat.Categories
	}

	// The list hands the filter one target per row, in row order, and every rank
	// we return is an index into the flat catalog. The search rows are built from
	// dependencies once, so the capture below stays aligned with the list for the
	// life of the screen.
	screen := dependenciesModel{
		dependencies: dependencies,
		categories:   categoryItems(rankCategories(categories)),
		selected:     newSelection(seeded),
		level:        levelCategories,
		width:        width,
		height:       height,
		theme:        theme,
	}

	screen.list = list.New(screen.categoryRows(), categoryDelegate{theme: theme}, screen.listWidth(), dependenciesListHeight(height))
	// The component's own chrome (title, filter bar, status bar, pagination dots,
	// help) is off: the dots are Unicode, the screen owns its header and its two
	// footer lines, and the header is what states the query. The filter bar also
	// reserved a blank line above the first row; not drawing it gives the window
	// that row back (US4.4). Filtering itself is unaffected: it is driven by the
	// component's filter state, not by whether the bar is drawn.
	screen.list.SetShowTitle(false)
	screen.list.SetShowFilter(false)
	screen.list.SetShowStatusBar(false)
	screen.list.SetShowPagination(false)
	screen.list.SetShowHelp(false)
	screen.list.SetStatusBarItemName("dependency", "dependencies")
	screen.list.DisableQuitKeybindings()
	screen.list.FilterInput.Prompt = dependenciesSearchPrompt
	screen.list.Filter = func(term string, _ []string) []list.Rank {
		positions := rankDependencies(term, dependencies)
		ranks := make([]list.Rank, 0, len(positions))
		for _, position := range positions {
			ranks = append(ranks, list.Rank{Index: position})
		}
		return ranks
	}

	return screen
}

// rowDelegate is the one place the row delegate is built, so the checked set and
// the frame tests' counter always travel together.
func (d dependenciesModel) rowDelegate() dependencyDelegate {
	return dependencyDelegate{theme: d.theme, selected: d.selected, renders: d.renders}
}

// dependencyLevel is what the dependency screen shows. The set is closed: the
// panel has two real levels (D4) and one way of looking at the whole catalog.
type dependencyLevel int

const (
	// levelCategories is the root: every category, ordered by size (D7).
	levelCategories dependencyLevel = iota

	// levelDependencies is one open category: only its dependencies.
	levelDependencies

	// levelSearch is the whole catalog while a query is on screen. A search is a
	// way of looking at the catalog, not a level of its own (D5).
	levelSearch
)

// String names the level, so a test failure says where the screen was.
func (l dependencyLevel) String() string {
	switch l {
	case levelCategories:
		return "categories"
	case levelDependencies:
		return "dependencies"
	case levelSearch:
		return "search"
	default:
		return fmt.Sprintf("level(%d)", int(l))
	}
}

// categoryItems turns the ordered categories into root rows.
func categoryItems(categories []catalog.Category) []categoryItem {
	items := make([]categoryItem, 0, len(categories))
	for _, category := range categories {
		items = append(items, categoryItem{category: category})
	}
	return items
}

// dependencyRows turns catalog values into list rows.
func dependencyRows(dependencies []catalog.Dependency) []list.Item {
	rows := make([]list.Item, 0, len(dependencies))
	for _, dependency := range dependencies {
		rows = append(rows, dependencyItem{dependency: dependency})
	}
	return rows
}

// categoryRows is the root's item slice.
func (d dependenciesModel) categoryRows() []list.Item {
	rows := make([]list.Item, 0, len(d.categories))
	for _, item := range d.categories {
		rows = append(rows, item)
	}
	return rows
}

// openCategory is the category the dependencies level belongs to. The rows and
// the header both read it, so the list and its scope can never disagree.
func (d dependenciesModel) openCategory() (catalog.Category, bool) {
	for _, item := range d.categories {
		if item.category.Name == d.open {
			return item.category, true
		}
	}
	return catalog.Category{}, false
}

// rows is the item slice the current level shows. The list is one component: a
// level change swaps its rows, never its identity.
func (d dependenciesModel) rows() []list.Item {
	switch d.level {
	case levelSearch:
		return dependencyRows(d.dependencies)
	case levelDependencies:
		category, ok := d.openCategory()
		if !ok {
			return nil
		}
		dependencies := make([]catalog.Dependency, 0, len(category.Dependencies))
		for _, dependency := range category.Dependencies {
			dependencies = append(dependencies, *dependency)
		}
		return dependencyRows(dependencies)
	default:
		return d.categoryRows()
	}
}

// show returns the screen at level, with that level's rows and no query on them.
// It is the only place the list's items and delegate are replaced, so a level
// change cannot leave the rows of the old level behind, and the search ranker —
// which indexes the flat catalog — can never run over category rows.
func (d dependenciesModel) show(level dependencyLevel) dependenciesModel {
	if level == levelDependencies && d.open == "" {
		level = levelCategories
	}
	if level == levelSearch && d.level != levelSearch {
		d.searchFrom = d.level
	}
	d.level = level

	d.list.ResetFilter()
	if level == levelCategories {
		d.list.SetDelegate(categoryDelegate{theme: d.theme})
	} else {
		d.list.SetDelegate(d.rowDelegate())
	}
	d.list.SetItems(d.rows())
	d.list.Select(d.landingRow(level))

	return d.applySize()
}

// landingRow is the row a level starts on. The root goes back to the category the
// user last opened, so esc returns them to their place (US4.2); every other level
// starts at the top, because remembering one cursor per category would be the
// navigation stack this screen does not build.
func (d dependenciesModel) landingRow(level dependencyLevel) int {
	if level != levelCategories || len(d.categories) == 0 {
		return 0
	}

	row := max(d.rootCursor, 0)
	if last := len(d.categories) - 1; row > last {
		row = last
	}
	return row
}

// openFocusedCategory drills into the category under the cursor, remembering the
// row it was on so esc can come back to it.
func (d dependenciesModel) openFocusedCategory() dependenciesModel {
	item, ok := d.list.SelectedItem().(categoryItem)
	if !ok {
		return d
	}
	d.rootCursor = d.list.Index()
	d.open = item.category.Name
	return d.show(levelDependencies)
}

// startSearch shows the whole catalog: / means the same thing at every level,
// because a scoped search would hide results the user cannot see they are
// missing (D5).
func (d dependenciesModel) startSearch() dependenciesModel {
	return d.show(levelSearch)
}

// leaveSearch returns to the level the search started from, with no query on it.
func (d dependenciesModel) leaveSearch() dependenciesModel {
	return d.show(d.searchFrom)
}

// resize applies a terminal size.
func (d dependenciesModel) resize(width, height int) dependenciesModel {
	d.width, d.height = width, height
	return d.applySize()
}

// applySize gives the list its share of the terminal. It is the one place a size
// reaches the component, because the share depends on the level as well as on the
// terminal: the category root never splits (US5.1).
func (d dependenciesModel) applySize() dependenciesModel {
	d.list.SetSize(d.listWidth(), dependenciesListHeight(d.height))
	return d
}

// dependenciesListHeight is the rows the list component gets: the terminal minus
// the header and the two footer lines. The floor keeps the component's paginator
// arithmetic sane on a terminal that reports almost nothing.
func dependenciesListHeight(height int) int {
	return max(4, height) - 3
}

// detailMinWidth is the narrowest terminal that shows the detail region. Below it
// the list keeps the whole width; US5.2 owns what happens between 100 and 119.
const detailMinWidth = 120

// detailWidth is the detail region's width and detailGap the separator and the
// spaces around it, " | ". The list keeps the rest of the terminal.
const (
	detailWidth = 38
	detailGap   = 3
)

// showDetail reports whether the body is split: the terminal is wide enough and the
// level shows dependencies, because the category root has no dependency under the
// cursor to describe.
func (d dependenciesModel) showDetail() bool {
	return d.width >= detailMinWidth && d.level != levelCategories
}

// listWidth is the room the list gets: the whole terminal, or what the detail
// region leaves of it.
func (d dependenciesModel) listWidth() int {
	if !d.showDetail() {
		return max(1, d.width)
	}
	return max(1, d.width-detailWidth-detailGap)
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
	d.list.SetDelegate(d.rowDelegate())
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

// view renders the header, the window and the two footer lines. The component
// renders only the current page, which is the bounded window US3.3 measures;
// nothing here asks for the whole slice. A level with no rows shows one
// explanatory line instead of a window padded with blanks, so an empty screen
// never looks frozen (US4.4).
func (d dependenciesModel) view() string {
	width := d.width

	lines := []string{d.headerLine()}
	if len(d.list.VisibleItems()) == 0 {
		lines = append(lines, d.emptyLine())
	} else {
		lines = append(lines, d.body()...)
	}
	lines = append(lines, d.theme.Muted.Render(fitLine(d.statusLine(), width)))
	lines = append(lines, d.theme.Muted.Render(fitLine(d.keyHints(), width)))

	return strings.Join(lines, "\n")
}

// body is the region between the header and the footer: the list window, or the
// list window beside the detail region when the terminal is wide enough (US5.1).
// Both regions are joined row by row and the line's right side is trimmed, so a
// short side never adds trailing blanks.
func (d dependenciesModel) body() []string {
	listLines := strings.Split(trimBlankTail(d.list.View()), "\n")
	if !d.showDetail() {
		return listLines
	}

	detail := d.detailLines(detailWidth, dependenciesListHeight(d.height))
	rows := max(len(listLines), len(detail))
	separator := d.theme.Border.Left

	lines := make([]string, 0, rows)
	for i := range rows {
		left := strings.Repeat(" ", d.listWidth())
		if i < len(listLines) {
			// A row is as wide as its content, so the list side is padded to the
			// column the separator lives in.
			left = listLines[i] + strings.Repeat(" ", max(0, d.listWidth()-ansi.StringWidth(listLines[i])))
		}
		right := ""
		if i < len(detail) {
			right = detail[i]
		}
		lines = append(lines, strings.TrimRight(left+" "+separator+" "+right, " "))
	}
	return lines
}

// emptyLine is the one line a level with no rows shows instead of a padded
// window: why there is nothing to list (US4.4).
func (d dependenciesModel) emptyLine() string {
	if query := strings.TrimSpace(d.list.FilterValue()); query != "" {
		return fmt.Sprintf("No dependency matches %q.", query)
	}
	if d.level == levelCategories {
		return "The catalog has no categories."
	}
	return "No dependencies in this category."
}

// trimBlankTail drops the blank lines the list component pads a short page with,
// so a frame ends where its content ends and carries no dead rows (US4.4).
func trimBlankTail(view string) string {
	lines := strings.Split(view, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

// headerLine is the line above the list: where the user is, and the totals this
// screen is a view of. The scope leads because it is what the level is about; the
// totals follow and give way on a narrow terminal, because the scope is the part
// that cannot be guessed from the rows (US4.3).
func (d dependenciesModel) headerLine() string {
	width := d.width
	totals := categoryCount(len(d.categories)) + footerGap + dependencyCount(len(d.dependencies))
	scope := d.scopeLine(width)

	switch {
	case scope == "":
		return fitLine(totals, width)
	case width <= 0 || ansi.StringWidth(scope+footerGap+totals) <= width:
		return scope + footerGap + totals
	default:
		return fitLine(scope, width)
	}
}

// scopeLine is the level in the header, in the ASCII shape docs/ui.md draws: the
// open category with its size, or the query being searched (D5). The root has no
// scope of its own, because its totals are the whole story.
func (d dependenciesModel) scopeLine(width int) string {
	switch d.level {
	case levelDependencies:
		category, ok := d.openCategory()
		if !ok {
			return ""
		}

		count := fmt.Sprintf("(%d)", len(category.Dependencies))
		budget := width - ansi.StringWidth("< ") - ansi.StringWidth(" ") - ansi.StringWidth(count)
		return "< " + fitName(category.Name, budget) + " " + count

	case levelSearch:
		// The quoted query, empty or not: a search that has begun but has nothing
		// typed is still a search of everything. The + flag makes %q escape
		// anything outside ASCII, so the header stays one ASCII line whatever the
		// user typed.
		return "Search: " + fmt.Sprintf("%+q", strings.TrimSpace(d.list.FilterValue()))

	default:
		return ""
	}
}

// fitName truncates a name to budget cells, "... included", so a caller can keep
// the text that follows it. A budget too small to hold the mark leaves the name
// whole, and the caller's line truncation is the backstop.
func fitName(name string, budget int) string {
	if budget <= 0 {
		return name
	}
	return ansi.Truncate(name, budget, "...")
}

// fitLine trims a line the screen composed itself to the terminal width.
func fitLine(line string, width int) string {
	if width <= 0 {
		return line
	}
	return ansi.Truncate(line, width, "...")
}

// dependenciesFinishHint is the key that leaves the screen with everything it has
// picked. enter cannot do it any more: at the root it opens a category, and on a
// dependency row it checks one. The hint row always shows it, because a key
// nobody can discover is not a key.
const dependenciesFinishHint = "ctrl+d finish"

// statusLine is the first footer line: what the level shows and what is checked.
// The ids give way when the room runs out; the count survives, because the number
// is the answer to "did my space register?".
func (d dependenciesModel) statusLine() string {
	width := d.width

	line := d.resultLine()
	if line != "" {
		line += footerGap
	}

	return line + selectionLine(d.chosen(), width-ansi.StringWidth(line))
}

// keyHints is the second footer line: the keys that work at this level, so the
// panel never hides a key the user needs (US4.4). esc means two different things
// in a search, and the line says which one is next.
func (d dependenciesModel) keyHints() string {
	switch d.level {
	case levelCategories:
		return "enter open" + footerGap + "/ find" + footerGap + "esc back" + footerGap + dependenciesFinishHint
	case levelSearch:
		esc := "esc back"
		if strings.TrimSpace(d.list.FilterValue()) != "" {
			esc = "esc clear"
		}
		return "enter/space pick" + footerGap + esc + footerGap + dependenciesFinishHint
	default:
		return "enter/space pick" + footerGap + "/ find" + footerGap + "esc back" + footerGap + dependenciesFinishHint
	}
}

// footerGap separates the parts of the header and the footer.
const footerGap = "  "

// resultLine is the status line's lead: what the current level shows, or what the
// query matched. It is empty when there is nothing to count — the empty line above
// the footer explains that — and at the root, where the header states the totals.
func (d dependenciesModel) resultLine() string {
	query := strings.TrimSpace(d.list.FilterValue())
	visible := len(d.list.VisibleItems())

	switch {
	case visible == 0:
		return ""
	case query != "":
		return fmt.Sprintf("Showing %d of %d for %q", visible, len(d.dependencies), query)
	case d.level == levelCategories:
		return ""
	default:
		return dependencyCount(visible)
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
// aware, enter opens a category or checks a dependency, ctrl+d finishes, and /
// always means the whole catalog.
//
// ctrl+d replaced VS3's enter as the finish key, because enter now has a level to
// open; the footer advertises it. enter on a dependency row checks it, the same
// as space, so docs/ui.md's "enter toggles" holds at the level it describes.
func (d dependenciesModel) update(msg tea.Msg) (dependenciesModel, tea.Cmd, dependencyAction) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.Type {
		case tea.KeyCtrlC:
			return d, tea.Interrupt, actionStay

		case tea.KeyCtrlD:
			return d, nil, actionFinish

		case tea.KeyEsc:
			switch {
			case d.list.FilterValue() != "":
				// The query goes first: clearing it is what esc means in a search,
				// and the next esc leaves the search for the level it started
				// from (docs/ui.md, US4.4).
				d.list.ResetFilter()
				return d, nil, actionStay
			case d.list.SettingFilter():
				// Search mode with nothing typed: there is no query to clear.
				return d.leaveSearch(), nil, actionStay
			case d.level == levelDependencies:
				return d.show(levelCategories), nil, actionStay
			case d.level == levelSearch:
				return d.leaveSearch(), nil, actionStay
			default:
				return d, nil, actionBack
			}

		case tea.KeySpace:
			if d.list.SettingFilter() || d.level == levelCategories {
				// A space is a character while the query is being typed, and a
				// category is not something that can be checked.
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
			if d.level == levelCategories {
				return d.openFocusedCategory(), nil, actionStay
			}
			return d.toggleFocused(), nil, actionStay

		case tea.KeyRunes:
			if len(key.Runes) == 1 && key.Runes[0] == '/' && !d.list.SettingFilter() && d.level != levelSearch {
				// Switch before the component starts its filter: it must filter
				// the whole catalog, never the rows of the level / was pressed on
				// (D5).
				d = d.startSearch()
			}
		}
	}

	updated, cmd := d.list.Update(msg)
	d.list = updated
	return d, cmd, actionStay
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

	// renders, when non-nil, counts the rows this delegate writes. Only the frame
	// tests set it (dependenciesModel.countRenders); the guard costs one branch
	// per row and changes nothing the rows say.
	renders *int
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
	if d.renders != nil {
		(*d.renders)++
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
