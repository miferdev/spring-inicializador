package tui

import (
	"errors"
	"fmt"
	"io"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	"github.com/miferdev/spring-inicializador/internal/catalog"
	"github.com/miferdev/spring-inicializador/internal/options"
)

// formModel is the Bubble Tea model around the project form. huh exposes no
// hook between updates, so the model embeds the form, delegates every message
// to it, and then repeats the package derivation. The form is a view over one
// formState, so navigation never copies the values being edited.
type formModel struct {
	*huh.Form

	state        *formState
	packageInput *huh.Input
	firstKey     Key

	// kinds is the widget kind of every row, so the hint line can speak about the
	// row the form is on without walking the sections on every frame.
	kinds map[Key]fieldKind
}

// Update delegates to the form and then lets the state react to the new values.
// This is the only place the derivation is driven; the rule itself lives in
// formState.followPackage. When the package row is not asked for there is no
// widget to keep in sync, and a value the command line supplied is never touched.
//
// esc is the one key the form does not already own: it means "back", which is a
// step to the previous row or, on the first row, the same abort ctrl+c gives.
func (m formModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.Form == nil {
		return m, nil
	}

	if key, ok := msg.(tea.KeyMsg); ok && key.Type == tea.KeyEsc {
		if m.atFirstField() {
			// Nothing to go back to: reuse the form's own ctrl+c path so esc and
			// ctrl+c leave the same state.
			msg = tea.KeyMsg{Type: tea.KeyCtrlC}
		} else {
			// Step back now. The form's previous-field message is synchronous, so
			// delivering it here is the same step the event loop would take.
			return m.Update(huh.PrevField())
		}
	}

	updated, cmd := m.Form.Update(msg)
	if form, ok := updated.(*huh.Form); ok {
		m.Form = form
	}

	if m.packageInput != nil {
		if m.state.followPackage() {
			// huh's input owns a second copy of the text: a value written behind
			// its back has to be pushed in, or the next blur overwrites the state.
			m.packageInput.Value(&m.state.PackageName)
		}
		m.packageInput.Description(m.state.packageMarker())
	}

	return m, cmd
}

// The hint lines the form draws under its rows, in ASCII. huh's own help bar spells
// its keys with glyphs no public option of the library changes (↑ ↓ •), and ASCII is
// the default here, so the bar is hidden in newForm and the form speaks for itself.
//
// "enter next" is honest on every row: inside a section it moves to the next field,
// and on a section's last row it opens the next section — which on the last section
// is the dependency pane. "esc back" is the step this model adds on top of huh's
// keymap, and "tab next field" is huh's other key for the same step.
const (
	formHintText   = "enter next  esc back  tab next field"
	formHintSelect = "up/down move  / filter  enter next  esc back"
)

// hintLine is the line under the rows: the keys that work on the row the form is
// on. A text row has no up/down and no filter, so it gets the other line. Like every
// line a screen composes, it is trimmed to the terminal width instead of wrapping.
func (m formModel) hintLine(width int) string {
	hint := formHintText
	if focused := m.Form.GetFocusedField(); focused != nil && m.kinds[Key(focused.GetKey())] == fieldSelect {
		hint = formHintSelect
	}
	return fitLine(hint, width)
}

// frame is the whole project screen: huh's rows and the hint line under them. huh's
// own help bar is hidden, so this line is the only place the keys are named.
func (m formModel) frame(width int) string {
	if m.Form == nil {
		return ""
	}

	view := m.Form.View()
	if view == "" {
		// A finished or aborted form renders nothing, and nothing is what the screen
		// should be: a hint for keys that no longer work would be a lie.
		return ""
	}
	return view + "\n" + m.hintLine(width)
}

// atFirstField reports whether the focused row is the first one the form asks,
// which is where "back" has nothing left to go back to.
func (m formModel) atFirstField() bool {
	focused := m.Form.GetFocusedField()
	if focused == nil {
		return false
	}
	return focused.GetKey() == string(m.firstKey)
}

// Run asks for the project options the command line did not answer, then opens
// the dependency list, and returns the options the wizard built. provided
// carries the project values the flags already set, and dependencies carries the
// ids --deps already answered: those rows are not asked for, and those
// dependencies open checked, so a flag answers its screen either way. stdin and
// stdout are the terminal handles the caller wants the wizard on; the program
// owns raw mode and the alternate screen while it is up.
//
// When every project row is already answered there is nothing to ask, so no
// program is started at all: the options come back without touching the
// terminal, with the dependencies exactly as the flags gave them.
//
// The wizard ends with enter on the review screen, which is the last screen: it
// returns the options it summarised, with the checked dependencies in catalog
// order, and the caller writes them. destination is where the command line said the
// project goes (--output); it is a fact of the review, not part of the request, so
// it never enters options.Options (US6.1).
//
// The outcome says how it ended: Finished with options to write, Left when the user
// backed out on purpose (esc on the form's first row), or Interrupted when ctrl+c
// stopped it on any screen. Left and Interrupted are not errors — the caller decides
// the exit code from the outcome — and neither may write anything (US6.4).
func Run(cat *catalog.Catalog, provided Values, dependencies []string, settings Settings, destination string, stdin io.Reader, stdout io.Writer) (options.Options, Outcome, error) {
	theme := NewTheme(settings)
	if settings.NoColor {
		// The form is painted by huh and the rows by the list component: only the
		// process-wide color profile reaches all of them (US5.4).
		DisableColor()
	}
	project := newProjectForm(cat, provided, theme)
	if !project.asks() {
		opts := optionsFromState(cat, project.state)
		opts.Deps = dependencies
		return opts, Finished, nil
	}

	// The root model decides where enter and esc lead, so the form keeps its
	// default commands (none): a completed form opens the dependency list
	// instead of quitting the program.
	last, err := tea.NewProgram(
		newModel(cat, project, dependencies, theme, destination),
		tea.WithInput(stdin),
		tea.WithOutput(stdout),
	).Run()

	// Bubble Tea reports both aborts as the same interrupt; the model recorded which
	// one it was before the form collapsed them, and the caller's exit code depends
	// on the difference (US6.4).
	if errors.Is(err, tea.ErrInterrupted) {
		if wizard, ok := last.(model); ok && wizard.outcome == Left {
			return options.Options{}, Left, nil
		}
		return options.Options{}, Interrupted, nil
	}
	if err != nil {
		return options.Options{}, Finished, fmt.Errorf("tui: running the wizard: %w", err)
	}

	wizard, ok := last.(model)
	if !ok {
		return options.Options{}, Finished, fmt.Errorf("tui: wizard returned %T, want tui.model", last)
	}
	if wizard.project.form != nil && wizard.project.form.State == huh.StateAborted {
		// Defensive: the abort reaches the program as an interrupt, so this is only
		// reachable if a screen stops delivering that command.
		if wizard.outcome == Interrupted {
			return options.Options{}, Interrupted, nil
		}
		return options.Options{}, Left, nil
	}

	return wizard.finished(), Finished, nil
}
