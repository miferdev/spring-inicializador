package tui

// Tests for US6.4: the wizard reports how it ended, so the command line can give
// "finished", "left on purpose" and "interrupted" their own exit codes. The screens
// themselves do not change; only the outcome does. No test needs a terminal.

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	"github.com/miferdev/spring-inicializador/internal/options"
)

// interrupt runs a message through the root model and reports the model it produced
// and whether the command is the one that stops the program.
func interrupt(t *testing.T, m model, msg tea.Msg) (model, bool) {
	t.Helper()

	next, cmd := m.Update(msg)
	updated, ok := next.(model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.model", next)
	}
	if cmd == nil {
		return updated, false
	}
	if _, ok := cmd().(tea.InterruptMsg); ok {
		return updated, true
	}
	return updated, false
}

// TestEscOnTheFormFirstRowIsLeaving: backing out on purpose is not an interruption,
// even though the form reaches it through the same abort path (US6.4).
func TestEscOnTheFormFirstRowIsLeaving(t *testing.T) {
	h := newHarness(t)

	if got := h.focused(); got != KeyGroupID {
		t.Fatalf("focus = %q, want the first row %q", got, KeyGroupID)
	}

	next, cmd := h.model.Update(escKey())
	left, ok := next.(model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.model", next)
	}
	if left.outcome != Left {
		t.Errorf("outcome = %v after esc on the first row, want %v", left.outcome, Left)
	}
	if cmd == nil {
		t.Fatal("esc on the first row produced no command; the program would keep running")
	}
	if _, ok := cmd().(tea.InterruptMsg); !ok {
		t.Errorf("esc produced %T, want the interrupt that ends the program", cmd())
	}
}

// TestCtrlCAtEveryScreenInterrupts covers the other half: ctrl+c is always an
// interruption, never a deliberate exit, wherever the focus is.
func TestCtrlCAtEveryScreenInterrupts(t *testing.T) {
	h := newHarness(t)

	// The form, on its first row and on a later one.
	updated, ok := interrupt(t, h.model, tea.KeyMsg{Type: tea.KeyCtrlC})
	if !ok {
		t.Fatal("ctrl+c on the form's first row did not interrupt")
	}
	if updated.outcome != Interrupted {
		t.Errorf("outcome = %v after ctrl+c on the form, want %v", updated.outcome, Interrupted)
	}

	h = newHarness(t)
	h.send(huh.NextField())
	if _, ok := interrupt(t, h.model, tea.KeyMsg{Type: tea.KeyCtrlC}); !ok {
		t.Error("ctrl+c on a later form row did not interrupt")
	}

	// The dependency pane, at its root and inside a category.
	h = newHarness(t)
	h.openDependencies()
	if _, ok := interrupt(t, h.model, tea.KeyMsg{Type: tea.KeyCtrlC}); !ok {
		t.Error("ctrl+c on the dependency root did not interrupt")
	}

	h = newHarness(t)
	h.openDependencies()
	h.openCategory("Web")
	if _, ok := interrupt(t, h.model, tea.KeyMsg{Type: tea.KeyCtrlC}); !ok {
		t.Error("ctrl+c inside a category did not interrupt")
	}

	// The review.
	h = newHarness(t)
	h.openDependencies()
	h.press(tea.KeyCtrlD)
	if h.model.screen != screenReview {
		t.Fatalf("screen = %v, want the review", h.model.screen)
	}
	if _, ok := interrupt(t, h.model, tea.KeyMsg{Type: tea.KeyCtrlC}); !ok {
		t.Error("ctrl+c on the review did not interrupt")
	}
}

// TestOutcomeNames keeps the type readable wherever it is reported.
func TestOutcomeNames(t *testing.T) {
	cases := map[Outcome]string{
		Finished:    "finished",
		Left:        "left",
		Interrupted: "interrupted",
	}
	for outcome, want := range cases {
		if got := outcome.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", int(outcome), got, want)
		}
	}
}

// TestRunReportsAnInterrupt drives the real program with the ctrl+c byte: the
// caller gets Interrupted, not an error and not "nothing to do".
func TestRunReportsAnInterrupt(t *testing.T) {
	cat := fixtureCatalog(t)

	type result struct {
		opts    options.Options
		outcome Outcome
		err     error
	}
	done := make(chan result, 1)
	go func() {
		var stdout bytes.Buffer
		opts, outcome, err := Run(cat, nil, nil, Settings{}, ".", strings.NewReader("\x03"), &stdout)
		done <- result{opts: opts, outcome: outcome, err: err}
	}()

	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("Run: %v", got.err)
		}
		if got.outcome != Interrupted {
			t.Errorf("outcome = %v after ctrl+c, want %v", got.outcome, Interrupted)
		}
		if !reflect.DeepEqual(got.opts, options.Options{}) {
			t.Errorf("options = %+v after an interruption, want none", got.opts)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run blocked: the scripted ctrl+c did not stop the form")
	}
}
