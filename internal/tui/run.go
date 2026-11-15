package tui

import (
	"errors"
	"fmt"
	"io"

	"github.com/charmbracelet/huh"

	"github.com/miferdev/spring-inicializador/internal/catalog"
	"github.com/miferdev/spring-inicializador/internal/options"
)

// RunForm shows the project form and returns the options it built. stdin and
// stdout are the terminal handles the caller wants the form on; huh runs the
// form in a Bubble Tea program, so it owns raw mode and the alternate screen
// while it is up.
//
// ok is false when the user aborted with esc or ctrl+c. That is not an error:
// there is nothing to do, and the caller exits successfully without writing
// anything.
func RunForm(cat *catalog.Catalog, stdin io.Reader, stdout io.Writer) (options.Options, bool, error) {
	project := newProjectForm(cat)

	if err := project.form.WithInput(stdin).WithOutput(stdout).Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return options.Options{}, false, nil
		}
		return options.Options{}, false, fmt.Errorf("tui: running the project form: %w", err)
	}

	return optionsFromState(cat, project.state), true, nil
}
