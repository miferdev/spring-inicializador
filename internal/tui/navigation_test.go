package tui

// Tests for US2.5: tab, shift+tab and esc move the focus, never the values. The
// model is driven with the same messages the event loop delivers, so no test
// needs a terminal.

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

// focusKey is the key of the row the form is on, or "" when nothing is focused.
func focusKey(model formModel) Key {
	focused := model.Form.GetFocusedField()
	if focused == nil {
		return ""
	}
	return Key(focused.GetKey())
}

// rowSnapshot renders one line per row: key, label and current value. Two forms
// built from the same state must produce the same lines, before and after
// navigation.
func rowSnapshot(form *projectForm) []string {
	values := form.state.pointers()
	lines := make([]string, 0, len(form.fields))
	for _, field := range form.fields {
		lines = append(lines, string(field.key)+"="+field.label+"="+*values[field.key])
	}
	return lines
}

// apply sends one message and returns the model, ignoring the commands. Those
// only carry cursor blinks here: navigation and typing update the state during
// the message itself.
func apply(t *testing.T, model formModel, msg tea.Msg) formModel {
	t.Helper()

	next, _ := model.Update(msg)
	updated, ok := next.(formModel)
	if !ok {
		t.Fatalf("Update returned %T, want formModel", next)
	}
	return updated
}

func escKey() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyEsc}
}

// TestEscOnFirstFieldAborts proves there is nothing before the first row to go
// back to, so esc leaves the form aborted with every value untouched.
func TestEscOnFirstFieldAborts(t *testing.T) {
	model := newTestModel(t)
	if got := focusKey(model); got != KeyGroupID {
		t.Fatalf("focus = %q, want the first row %q", got, KeyGroupID)
	}

	before := model.state
	next, _ := model.Update(escKey())
	aborted, ok := next.(formModel)
	if !ok {
		t.Fatalf("Update returned %T, want formModel", next)
	}

	if aborted.Form.State != huh.StateAborted {
		t.Errorf("form state = %v, want aborted", aborted.Form.State)
	}
	if aborted.state != before {
		t.Errorf("state changed on abort: %+v, want %+v", aborted.state, before)
	}
}

// TestRunAbortsOnEscAtTheFirstField drives the real program with a scripted
// esc byte instead of a terminal: the form must report "nothing to do".
func TestRunAbortsOnEscAtTheFirstField(t *testing.T) {
	cat := fixtureCatalog(t)

	type result struct {
		opts options.Options
		ok   bool
		err  error
	}
	done := make(chan result, 1)
	go func() {
		var stdout bytes.Buffer
		opts, ok, err := Run(cat, nil, strings.NewReader("\x1b"), &stdout)
		done <- result{opts: opts, ok: ok, err: err}
	}()

	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("Run: %v", got.err)
		}
		if got.ok {
			t.Fatalf("Run reported ok after esc on the first row (options %+v)", got.opts)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run blocked: the scripted esc did not abort the form")
	}
}

// TestCtrlCAbortsFromAnyField pins the other half of the contract: esc steps
// back, but ctrl+c still ends the form wherever the focus happens to be.
func TestCtrlCAbortsFromAnyField(t *testing.T) {
	model := newTestModel(t)
	model = apply(t, model, huh.NextField())
	model = apply(t, model, huh.NextField())
	if got := focusKey(model); got != KeyName {
		t.Fatalf("focus = %q, want %q", got, KeyName)
	}

	before := model.state
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	aborted, ok := next.(formModel)
	if !ok {
		t.Fatalf("Update returned %T, want formModel", next)
	}

	if aborted.Form.State != huh.StateAborted {
		t.Errorf("form state = %v, want aborted", aborted.Form.State)
	}
	if aborted.state != before {
		t.Errorf("state changed on abort: %+v, want %+v", aborted.state, before)
	}
}

// TestEscMovesBackOneField proves esc on a later row steps back one row and
// leaves every value byte-identical.
func TestEscMovesBackOneField(t *testing.T) {
	model := newTestModel(t)
	model = apply(t, model, huh.NextField()) // Artifact
	model = apply(t, model, huh.NextField()) // Name
	model = apply(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})

	if got := focusKey(model); got != KeyName {
		t.Fatalf("focus = %q, want %q", got, KeyName)
	}
	before := model.state

	model = apply(t, model, escKey())

	if got := focusKey(model); got != KeyArtifactID {
		t.Errorf("focus = %q, want the previous row %q", got, KeyArtifactID)
	}
	if model.state != before {
		t.Errorf("esc changed a value: %+v, want %+v", model.state, before)
	}
}

// TestTabAndShiftTabReturnToTheSameField walks forward with tab and back with
// shift+tab. A field answers those keys with a navigation command, which the
// event loop delivers as a message; the test delivers the same message.
func TestTabAndShiftTabReturnToTheSameField(t *testing.T) {
	model := newTestModel(t)
	before := model.state

	model = apply(t, model, tea.KeyMsg{Type: tea.KeyTab})
	model = apply(t, model, huh.NextField())
	if got := focusKey(model); got != KeyArtifactID {
		t.Fatalf("after tab: focus = %q, want %q", got, KeyArtifactID)
	}

	model = apply(t, model, tea.KeyMsg{Type: tea.KeyShiftTab})
	model = apply(t, model, huh.PrevField())
	if got := focusKey(model); got != KeyGroupID {
		t.Errorf("after shift+tab: focus = %q, want the first row %q", got, KeyGroupID)
	}
	if model.state != before {
		t.Errorf("navigation changed a value: %+v, want %+v", model.state, before)
	}
}

// TestSameStateRendersTheSameFields is the "view over one value" criterion: two
// forms from the same state describe the same rows, and navigating the first one
// does not change what it describes.
func TestSameStateRendersTheSameFields(t *testing.T) {
	cat := fixtureCatalog(t)
	first := newProjectForm(cat, nil)
	second := newProjectForm(cat, nil)

	if got, want := rowSnapshot(first), rowSnapshot(second); !reflect.DeepEqual(got, want) {
		t.Fatalf("two forms from the same state differ:\n got %v\nwant %v", got, want)
	}

	model := formModel{
		Form:         first.form,
		state:        &first.state,
		packageInput: first.packageInput,
		firstKey:     first.firstKey,
	}
	before := rowSnapshot(first)

	model = apply(t, model, huh.NextField())
	model = apply(t, model, huh.NextField())
	model = apply(t, model, escKey())
	model = apply(t, model, huh.PrevField())

	if after := rowSnapshot(first); !reflect.DeepEqual(after, before) {
		t.Errorf("navigation changed the rendered rows:\nbefore %v\nafter  %v", before, after)
	}
}

// TestEscBackToEditedPackageKeepsTheHumanValue checks the derivation rule across
// navigation: coming back to the package field does not re-arm it, and changing
// the artifact afterwards does not overwrite the human edit.
func TestEscBackToEditedPackageKeepsTheHumanValue(t *testing.T) {
	model := newTestModel(t)

	// Group -> Artifact -> Name -> Package.
	for range 3 {
		model = apply(t, model, huh.NextField())
	}
	model = apply(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})

	edited := model.state.PackageName
	if edited == "com.example.demo" {
		t.Fatalf("typing did not reach the package field (still %q)", edited)
	}
	if model.state.packageAuto {
		t.Fatal("typing into the package field left the derivation active")
	}

	// esc goes back to Name; forward returns to the package with its value.
	model = apply(t, model, escKey())
	if got := focusKey(model); got != KeyName {
		t.Fatalf("after esc: focus = %q, want %q", got, KeyName)
	}
	model = apply(t, model, huh.NextField())
	if got := focusKey(model); got != KeyPackageName {
		t.Fatalf("after returning: focus = %q, want %q", got, KeyPackageName)
	}
	if model.state.PackageName != edited {
		t.Errorf("PackageName = %q, want the human value %q", model.state.PackageName, edited)
	}
	if model.state.packageAuto {
		t.Error("coming back to the package field re-armed the derivation")
	}

	// Back to the artifact id and change it: the edit still wins.
	model = apply(t, model, escKey()) // Name
	model = apply(t, model, escKey()) // Artifact
	if got := focusKey(model); got != KeyArtifactID {
		t.Fatalf("focus = %q, want %q", got, KeyArtifactID)
	}
	model = apply(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})

	if model.state.PackageName != edited {
		t.Errorf("PackageName = %q, want the human value %q after the artifact changed", model.state.PackageName, edited)
	}
	if model.state.packageAuto {
		t.Error("changing the artifact re-armed the derivation")
	}
}
