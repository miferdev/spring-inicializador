package tui

// Tests for US3.1: the second screen. The root model is driven with the messages
// the event loop delivers, and the commands those messages return are run so the
// next message arrives the way it would in a real program: huh only completes
// the form after its next-field command has made the round trip. Nothing here
// opens a TTY or starts a Bubble Tea program.

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/cursor"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	"github.com/miferdev/spring-inicializador/internal/catalog"
	"github.com/miferdev/spring-inicializador/internal/options"
)

// harness is the root model plus the fixture it was built from.
type harness struct {
	t     *testing.T
	cat   *catalog.Catalog
	model model
}

// newHarness builds the wizard at a fixed terminal size. The list's filter input
// is switched to a static cursor: a blinking one schedules a timer after every
// keystroke, and the timer proves nothing about filtering.
func newHarness(t *testing.T) *harness {
	t.Helper()

	cat := fixtureCatalog(t)
	root := newModel(cat, newProjectForm(cat, nil))
	root.deps.list.FilterInput.Cursor.SetMode(cursor.CursorStatic)

	next, _ := root.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	sized, ok := next.(model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.model", next)
	}
	return &harness{t: t, cat: cat, model: sized}
}

// send delivers one message, then runs the commands it produced and feeds their
// messages back, which is what tea.Program does with a single keypress.
func (h *harness) send(msg tea.Msg) {
	h.t.Helper()

	queue := []tea.Msg{msg}
	for steps := 0; len(queue) > 0; steps++ {
		if steps > 500 {
			h.t.Fatal("the model kept asking for work; check for a command loop")
		}

		next, cmd := h.model.Update(queue[0])
		queue = queue[1:]

		root, ok := next.(model)
		if !ok {
			h.t.Fatalf("Update returned %T, want tui.model", next)
		}
		h.model = root
		queue = append(queue, commandMessages(cmd)...)
	}
}

// commandBudget is how long a command gets to answer. Every command that changes
// state in this package is a pure function and answers immediately; the ones
// that do not answer are cursor blink timers, which only repaint the cursor and
// would otherwise stall the test for their blink interval.
const commandBudget = 10 * time.Millisecond

// commandMessages flattens a command tree into the messages it produces. A blink
// message is dropped even if one arrives: feeding it back would schedule the
// next blink for ever.
func commandMessages(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}

	msg, answered := awaitCommand(cmd)
	if !answered {
		return nil
	}

	switch msg := msg.(type) {
	case nil:
		return nil
	case tea.BatchMsg:
		var all []tea.Msg
		for _, sub := range msg {
			all = append(all, commandMessages(sub)...)
		}
		return all
	case cursor.BlinkMsg:
		return nil
	default:
		return []tea.Msg{msg}
	}
}

// awaitCommand runs one command with a short budget and reports whether it
// answered. A command that misses the budget is left to finish on its own.
func awaitCommand(cmd tea.Cmd) (tea.Msg, bool) {
	answered := make(chan tea.Msg, 1)
	go func() { answered <- cmd() }()

	select {
	case msg := <-answered:
		return msg, true
	case <-time.After(commandBudget):
		return nil, false
	}
}

// press sends a key that carries no runes.
func (h *harness) press(keyType tea.KeyType) {
	h.t.Helper()
	h.send(tea.KeyMsg{Type: keyType})
}

// typeText sends one key per rune, the way a terminal delivers typing.
func (h *harness) typeText(text string) {
	h.t.Helper()
	for _, r := range text {
		h.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

// focused is the key of the project row the form is on.
func (h *harness) focused() Key {
	h.t.Helper()

	focused := h.model.form.Form.GetFocusedField()
	if focused == nil {
		return ""
	}
	return Key(focused.GetKey())
}

// toLastRow moves the focus to the last row the form asks, using huh's own
// navigation message.
func (h *harness) toLastRow() {
	h.t.Helper()
	for range askedRows(h.model.project.fields) - 1 {
		h.send(huh.NextField())
	}
}

// openDependencies walks the form to its last row and finishes it.
func (h *harness) openDependencies() {
	h.t.Helper()

	h.toLastRow()
	h.press(tea.KeyEnter)
	if h.model.screen != screenDependencies {
		h.t.Fatalf("screen after enter on the last row = %v, want %v", h.model.screen, screenDependencies)
	}
}

// askedRows counts the rows that are questions; a note is not one.
func askedRows(fields []field) int {
	rows := 0
	for _, f := range fields {
		if f.kind != fieldNote {
			rows++
		}
	}
	return rows
}

// TestEnterOnTheLastRowOpensTheDependencies is the entrance to the second
// screen: enter on the last row the form asks shows the dependency list.
func TestEnterOnTheLastRowOpensTheDependencies(t *testing.T) {
	h := newHarness(t)

	if h.model.screen != screenProject {
		t.Fatalf("screen = %v before any key, want %v", h.model.screen, screenProject)
	}

	h.toLastRow()
	if got := h.focused(); got != KeyBootVersion {
		t.Fatalf("focus after walking to the end = %q, want the last row %q", got, KeyBootVersion)
	}

	h.press(tea.KeyEnter)

	if h.model.screen != screenDependencies {
		t.Fatalf("screen after enter on the last row = %v, want %v", h.model.screen, screenDependencies)
	}
}

// TestEscFromTheDependenciesKeepsEveryAnswer is the criterion carried over from
// VS2: a round trip through the dependency screen returns the same options, byte
// for byte, including a field edited by hand.
func TestEscFromTheDependenciesKeepsEveryAnswer(t *testing.T) {
	h := newHarness(t)

	// Edit an identifier by hand, the way a person would, and let the package
	// field derive from it.
	h.typeText("corp")
	before := optionsFromState(h.cat, h.model.project.state)
	if before.GroupID == options.Defaults(h.cat).GroupID {
		t.Fatalf("typing did not reach the group row (still %q)", before.GroupID)
	}
	if want := options.DerivePackageName(before.GroupID, before.ArtifactID); before.PackageName != want {
		t.Fatalf("PackageName = %q, want the derived %q", before.PackageName, want)
	}

	h.openDependencies()
	if got := len(h.model.deps.list.Items()); got == 0 {
		t.Fatal("the dependency screen has no rows")
	}

	// esc on a screen with no query means "back", not "quit".
	h.press(tea.KeyEsc)
	if h.model.screen != screenProject {
		t.Fatalf("screen = %v after esc, want the project form", h.model.screen)
	}

	after := optionsFromState(h.cat, h.model.project.state)
	if !reflect.DeepEqual(after, before) {
		t.Errorf("options after the round trip = %+v, want the values before %+v", after, before)
	}

	// The form is rendered again, not left blank by the finished huh form.
	view := h.model.View()
	if !strings.Contains(view, "Group") || !strings.Contains(view, before.GroupID) {
		t.Errorf("the form did not come back with its values; view:\n%s", view)
	}

	// The derivation rule survived the trip: the flag lives on the state, so a
	// new identifier change still recomputes the package name.
	h.typeText("x")
	derived := optionsFromState(h.cat, h.model.project.state)
	if want := options.DerivePackageName(derived.GroupID, derived.ArtifactID); derived.PackageName != want {
		t.Errorf("PackageName = %q after coming back and typing, want the derived %q", derived.PackageName, want)
	}
}

// TestEscClearsTheQueryBeforeItLeavesTheScreen is the level-aware half of the
// carried-over criterion: esc first leaves search mode, then clears the query,
// and only an empty query goes back to the form (docs/ui.md).
func TestEscClearsTheQueryBeforeItLeavesTheScreen(t *testing.T) {
	h := newHarness(t)
	h.openDependencies()

	// A filter that was focused but never typed into is still a mode to leave:
	// esc closes it instead of throwing away the whole screen.
	h.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if !h.model.deps.list.SettingFilter() {
		t.Fatal("/ did not focus the filter")
	}
	h.press(tea.KeyEsc)
	if h.model.screen != screenDependencies {
		t.Fatalf("esc left the screen although the filter was being edited (screen %v)", h.model.screen)
	}
	if h.model.deps.list.SettingFilter() {
		t.Error("esc did not leave search mode")
	}

	h.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	h.typeText("postgres")
	if got := h.model.deps.list.FilterValue(); got != "postgres" {
		t.Fatalf("filter = %q, want postgres", got)
	}

	h.press(tea.KeyEsc)
	if h.model.screen != screenDependencies {
		t.Fatalf("esc left the screen while a query was on it (screen %v)", h.model.screen)
	}
	if got := h.model.deps.list.FilterValue(); got != "" {
		t.Errorf("filter = %q after esc, want it cleared", got)
	}
	if got, want := len(h.model.deps.list.VisibleItems()), len(h.model.deps.list.Items()); got != want {
		t.Errorf("visible rows = %d after clearing the query, want all %d", got, want)
	}

	h.press(tea.KeyEsc)
	if h.model.screen != screenProject {
		t.Errorf("screen = %v after the second esc, want the project form", h.model.screen)
	}
}

// TestEnterOnTheDependencyScreenFinishesTheRun keeps the wizard able to end:
// with no selection yet (US3.2), enter is the key that hands the answers back to
// the caller so a project can be generated.
func TestEnterOnTheDependencyScreenFinishesTheRun(t *testing.T) {
	h := newHarness(t)
	h.openDependencies()

	_, cmd := h.model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	msgs := commandMessages(cmd)
	if len(msgs) == 0 {
		t.Fatal("enter on the dependency screen produced no message; the run would hang")
	}
	if _, ok := msgs[0].(tea.QuitMsg); !ok {
		t.Errorf("enter produced %T, want tea.QuitMsg", msgs[0])
	}
}

// TestCtrlCOnTheDependencyScreenInterrupts pins the abort contract on the second
// screen: ctrl+c still means "nothing to do", never "generate what we have".
func TestCtrlCOnTheDependencyScreenInterrupts(t *testing.T) {
	h := newHarness(t)
	h.openDependencies()

	_, cmd := h.model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	msgs := commandMessages(cmd)
	if len(msgs) == 0 {
		t.Fatal("ctrl+c on the dependency screen produced no message")
	}
	if _, ok := msgs[0].(tea.InterruptMsg); !ok {
		t.Errorf("ctrl+c produced %T, want tea.InterruptMsg", msgs[0])
	}
}
