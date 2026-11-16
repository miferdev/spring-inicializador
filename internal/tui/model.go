package tui

// The root model (US3.1): one Bubble Tea model with a screen enum. The screens are
// views over one formState, never copies of it, which is what makes going back
// free: returning from the dependency screen cannot lose an answer because there
// is nothing to restore. The wizard is sequential (D9): project form, dependency
// pane, review.

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	"github.com/miferdev/spring-inicializador/internal/catalog"
	"github.com/miferdev/spring-inicializador/internal/options"
)

// screen identifies the screen the wizard shows. The set is closed and grows by
// slice: this story ships Project and Dependencies, docs/ui.md adds Review and
// Run.
type screen int

const (
	screenProject screen = iota
	screenDependencies
	screenReview
)

// String names the screen, so a test failure says where the wizard stopped.
func (s screen) String() string {
	switch s {
	case screenProject:
		return "project"
	case screenDependencies:
		return "dependencies"
	case screenReview:
		return "review"
	default:
		return fmt.Sprintf("screen(%d)", int(s))
	}
}

// defaultWidth and defaultHeight size the screens until the terminal reports its
// own size. A program always receives a WindowSize message at start up; this is
// only the size a model built outside a program starts from.
const (
	defaultWidth  = 80
	defaultHeight = 24
)

// model is the root Bubble Tea model. It owns the screens and nothing else: the
// answers live in project.state, and every screen reads that one value.
type model struct {
	// catalog is what the review reads its labels from; it is also what the
	// options are resolved against, so the wizard is built from exactly one.
	catalog *catalog.Catalog

	project *projectForm
	form    formModel
	deps    dependenciesModel
	theme   Theme

	// destination is where the command line said the project should be written
	// (--output). It is a review fact, not a request parameter, so it never enters
	// options.Options (US6.1).
	destination string

	screen screen
	width  int
	height int
}

// newModel builds the wizard over a project form the command line already
// answered, with the dependency ids --deps already answered checked. Everything
// is created once: switching screens reuses the same state and the same list, so
// the round trip allocates nothing and loses nothing.
func newModel(cat *catalog.Catalog, project *projectForm, dependencies []string, theme Theme, destination string) model {
	return model{
		catalog:     cat,
		project:     project,
		form:        newFormModel(project),
		deps:        newDependenciesModel(cat, dependencies, defaultWidth, defaultHeight, theme),
		theme:       theme,
		destination: destination,
		screen:      screenProject,
	}
}

// finished is the options the wizard built: the project values it edited and the
// checked dependencies, in catalog order.
func (m model) finished() options.Options {
	opts := optionsFromState(m.catalog, m.project.state)
	opts.Deps = m.deps.chosen()
	return opts
}

// Init starts the project form. The dependency list needs no start-up command.
func (m model) Init() tea.Cmd {
	if m.form.Form == nil {
		return nil
	}
	return m.form.Init()
}

// Update routes one message to the visible screen.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = size.Width, size.Height
		m.deps = m.deps.resize(size.Width, size.Height)
	}

	switch m.screen {
	case screenReview:
		return m.updateReview(msg)
	case screenDependencies:
		return m.updateDependencies(msg)
	}
	return m.updateProject(msg)
}

// View renders the visible screen.
func (m model) View() string {
	switch m.screen {
	case screenReview:
		return m.reviewView()
	case screenDependencies:
		return m.deps.view()
	}
	if m.form.Form == nil {
		return ""
	}
	return m.form.View()
}

// updateProject delegates to the form and watches the two states the form can
// reach by itself. Completed is the interesting one: the form is done asking, so
// the dependency screen opens instead of the program quitting, which is the
// whole reason a root model exists.
func (m model) updateProject(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := m.form.Update(msg)
	if form, ok := updated.(formModel); ok {
		m.form = form
	}

	if m.project.form != nil {
		switch m.project.form.State {
		case huh.StateAborted:
			// Aborted is the caller's "nothing to do": the same interrupt the
			// form alone used to raise, so the command line keeps its contract.
			return m, tea.Interrupt
		case huh.StateCompleted:
			m.screen = screenDependencies
		}
	}
	return m, cmd
}

// updateDependencies delegates to the list and applies the navigation the list
// cannot decide: back to the form, or on to the review, which is now the last
// screen before anything is written (US6.1).
func (m model) updateDependencies(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd, action := m.deps.update(msg)
	m.deps = updated

	switch action {
	case actionBack:
		m.showProjectForm()
	case actionFinish:
		m.screen = screenReview
	}
	return m, cmd
}

// updateReview handles the confirmation screen: enter generates, esc goes back to
// the dependency pane, ctrl+c aborts. Nothing else matters here, because the
// review holds no state of its own.
func (m model) updateReview(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch key.Type {
	case tea.KeyCtrlC:
		return m, tea.Interrupt
	case tea.KeyEnter:
		return m, tea.Quit
	case tea.KeyEsc:
		// The dependency pane kept its level, cursor, query and selection the whole
		// time: going back is showing it again.
		m.screen = screenDependencies
	}
	return m, nil
}

// showProjectForm comes back to the form. The huh form that completed renders
// nothing and ignores every message, so its widgets are rebuilt over the state
// they edited: the answers are the state, never the widgets, which is why the
// round trip cannot lose one.
func (m *model) showProjectForm() {
	m.project.reopen()
	m.form = newFormModel(m.project)
	m.screen = screenProject

	if m.width > 0 && m.height > 0 {
		// A fresh form has no size yet, and the terminal will not report one
		// again just because a screen was revisited.
		m.form.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	}
}

// newFormModel wraps the project form in the Bubble Tea model the root drives.
func newFormModel(project *projectForm) formModel {
	return formModel{
		Form:         project.form,
		state:        &project.state,
		packageInput: project.packageInput,
		firstKey:     project.firstKey,
	}
}
