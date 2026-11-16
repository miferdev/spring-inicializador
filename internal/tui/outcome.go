package tui

import "fmt"

// Outcome is how the wizard ended. The caller needs the difference between a run
// that finished, one the user left on purpose and one that was interrupted: they are
// three different exit codes, and only one of them is something to report (US6.4).
//
// It is meaningful only when Run's error is nil.
type Outcome int

const (
	// Finished means the user confirmed on the review: the options are complete and
	// the caller should write the project.
	Finished Outcome = iota

	// Left means the user backed out on purpose — esc on the form's first row —
	// without confirming: there is nothing to do, and nothing to count as failure.
	Left

	// Interrupted means ctrl+c: the run was asked to stop, and nothing may be
	// written.
	Interrupted
)

// String names the outcome, so a failure message or a test says which one it was.
func (o Outcome) String() string {
	switch o {
	case Finished:
		return "finished"
	case Left:
		return "left"
	case Interrupted:
		return "interrupted"
	default:
		return fmt.Sprintf("outcome(%d)", int(o))
	}
}
