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

// atFirstField reports whether the focused row is the first one the form asks,
// which is where "back" has nothing left to go back to.
func (m formModel) atFirstField() bool {
	focused := m.Form.GetFocusedField()
	if focused == nil {
		return false
	}
	return focused.GetKey() == string(m.firstKey)
}

// RunForm asks for the project options the command line did not answer and
// returns the options it built. provided carries the values the flags already
// set; those rows are not asked for. stdin and stdout are the terminal handles
// the caller wants the form on; the program owns raw mode and the alternate
// screen while it is up.
//
// When every row is already answered there is nothing to ask, so no program is
// started at all: the options come back without touching the terminal.
//
// ok is false when the user aborts with ctrl+c anywhere, or with esc on the
// first row where there is no previous row to step back to. That is not an
// error: there is nothing to do, and the caller exits successfully without
// writing anything.
func RunForm(cat *catalog.Catalog, provided Values, stdin io.Reader, stdout io.Writer) (options.Options, bool, error) {
	project := newProjectForm(cat, provided)
	if !project.asks() {
		return optionsFromState(cat, project.state), true, nil
	}

	// huh.Form.Run normally wires these two. Running the form through our own
	// program means keeping the same contract: enter finishes the form and
	// ctrl+c interrupts it. esc on the first row reuses the same abort path.
	project.form.SubmitCmd = tea.Quit
	project.form.CancelCmd = tea.Interrupt

	model, err := tea.NewProgram(
		formModel{
			Form:         project.form,
			state:        &project.state,
			packageInput: project.packageInput,
			firstKey:     project.firstKey,
		},
		tea.WithInput(stdin),
		tea.WithOutput(stdout),
	).Run()

	// Bubble Tea reports the ctrl+c interruption as an error; the form has
	// already recorded the abort, and both mean "nothing to do".
	if errors.Is(err, tea.ErrInterrupted) {
		return options.Options{}, false, nil
	}
	if err != nil {
		return options.Options{}, false, fmt.Errorf("tui: running the project form: %w", err)
	}

	final, ok := model.(formModel)
	if !ok {
		return options.Options{}, false, fmt.Errorf("tui: project form returned %T, want tui.formModel", model)
	}
	if final.Form.State == huh.StateAborted {
		return options.Options{}, false, nil
	}

	return optionsFromState(cat, *final.state), true, nil
}
