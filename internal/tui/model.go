package tui

// The root model (US3.1): one Bubble Tea model with a screen enum, as
// docs/ui.md describes. The screens are views over one formState, never copies
// of it, which is what makes going back free: returning from the dependency
// screen cannot lose an answer because there is nothing to restore.

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	"github.com/miferdev/spring-inicializador/internal/catalog"
)

// screen identifies the screen the wizard shows. The set is closed and grows by
// slice: this story ships Project and Dependencies, docs/ui.md adds Review and
// Run.
type screen int

const (
	screenProject screen = iota
	screenDependencies
)

// String names the screen, so a test failure says where the wizard stopped.
func (s screen) String() string {
	switch s {
	case screenProject:
		return "project"
	case screenDependencies:
		return "dependencies"
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
// answers live in project.state, and both screens read that one value.
type model struct {
	project *projectForm
	form    formModel
	deps    dependenciesModel
	screen  screen
	width   int
	height  int
}

// newModel builds the wizard over a project form the command line already
// answered. Everything is created once: switching screens reuses the same state
// and the same list, so the round trip allocates nothing and loses nothing.
func newModel(cat *catalog.Catalog, project *projectForm) model {
	return model{
		project: project,
		form:    newFormModel(project),
		deps:    newDependenciesModel(cat, defaultWidth, defaultHeight, Default()),
		screen:  screenProject,
	}
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

	if m.screen == screenDependencies {
		return m.updateDependencies(msg)
	}
	return m.updateProject(msg)
}

// View renders the visible screen.
func (m model) View() string {
	if m.screen == screenDependencies {
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
// cannot decide: back to the form, or the end of the run.
func (m model) updateDependencies(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd, action := m.deps.update(msg)
	m.deps = updated

	switch action {
	case actionBack:
		m.showProjectForm()
	case actionFinish:
		return m, tea.Quit
	}
	return m, cmd
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
