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
}

// Update delegates to the form and then lets the state react to the new values.
// This is the only place the derivation is driven; the rule itself lives in
// formState.followPackage.
func (m formModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := m.Form.Update(msg)
	if form, ok := updated.(*huh.Form); ok {
		m.Form = form
	}

	if m.state.followPackage() && m.packageInput != nil {
		// huh's input owns a second copy of the text: a value written behind
		// its back has to be pushed in, or the next blur overwrites the state.
		m.packageInput.Value(&m.state.PackageName)
	}
	if m.packageInput != nil {
		m.packageInput.Description(m.state.packageMarker())
	}

	return m, cmd
}

// RunForm shows the project form and returns the options it built. stdin and
// stdout are the terminal handles the caller wants the form on; the program
// owns raw mode and the alternate screen while it is up.
//
// ok is false when the user aborts with ctrl+c. That is not an error: there is
// nothing to do, and the caller exits successfully without writing anything.
func RunForm(cat *catalog.Catalog, stdin io.Reader, stdout io.Writer) (options.Options, bool, error) {
	project := newProjectForm(cat)

	// huh.Form.Run normally wires these two. Running the form through our own
	// program means keeping the same contract: enter finishes the form and
	// ctrl+c interrupts it.
	project.form.SubmitCmd = tea.Quit
	project.form.CancelCmd = tea.Interrupt

	model, err := tea.NewProgram(
		formModel{Form: project.form, state: &project.state, packageInput: project.packageInput},
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
