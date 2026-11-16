package tui

// Tests for US6.1: the review screen summarises the wizard's answers before
// anything touches disk. It is a view over the model, so these tests drive the real
// transitions and read the frame. No test needs a terminal or a network.

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// reviewScreen builds the wizard at a size, answers it as the flags would, and
// opens the review the way a person does.
func reviewScreen(t *testing.T, width, height int, provided Values, dependencies []string, destination string) model {
	t.Helper()

	cat := fixtureCatalog(t)
	wizard := newModel(cat, newProjectForm(cat, provided, Default()), dependencies, Default(), destination)
	next, _ := wizard.Update(tea.WindowSizeMsg{Width: width, Height: height})
	wizard, ok := next.(model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.model", next)
	}
	return wizard
}

// openReview drives ctrl+d on the dependency pane and returns the review.
func openReview(t *testing.T, wizard model) model {
	t.Helper()

	next, cmd := wizard.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	reviewed, ok := next.(model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.model", next)
	}
	if msgs := commandMessages(cmd); len(msgs) != 0 {
		t.Errorf("ctrl+d from the dependency pane produced %v, want the wizard to keep running", msgs)
	}
	return reviewed
}

// TestCtrlDOpensTheReview is the first criterion: the dependency pane's finish key
// now leads to the review instead of ending the run, and nothing is written.
func TestCtrlDOpensTheReview(t *testing.T) {
	wizard := reviewScreen(t, 80, 24, nil, nil, ".")
	wizard.screen = screenDependencies

	reviewed := openReview(t, wizard)
	if reviewed.screen != screenReview {
		t.Fatalf("screen = %v after ctrl+d, want the review", reviewed.screen)
	}
	if got := reviewed.View(); !strings.HasPrefix(got, "Review") {
		t.Errorf("the review frame does not start with its title; view:\n%s", got)
	}
}

// TestReviewListsEveryDecision pins the frame line by line: every decision, in the
// order a person checks them, with the metadata's own labels.
func TestReviewListsEveryDecision(t *testing.T) {
	provided := Values{
		KeyGroupID:     "com.acme",
		KeyArtifactID:  "billing",
		KeyName:        "billing",
		KeyPackageName: "com.acme.billing",
		KeyBuild:       "gradle-kotlin",
		KeyLanguage:    "kotlin",
		KeyPackaging:   "war",
		KeyJavaVersion: "21",
		KeyBootVersion: "4.0.9.RELEASE",
	}

	reviewed := openReview(t, func() model {
		wizard := reviewScreen(t, 80, 24, provided, []string{"data-jpa", "web"}, "/tmp/out")
		wizard.screen = screenDependencies
		return wizard
	}())

	want := []string{
		"Review",
		"Project       billing",
		"Coordinates   com.acme:billing",
		"Package       com.acme.billing",
		"Build         Gradle - Kotlin (gradlew)",
		"Boot          4.0.9.RELEASE",
		"Language      Kotlin",
		"Java          21",
		"Packaging     War",
		// The ids come out in catalog order, not in the order they were seeded.
		"Deps          web, data-jpa",
		"Destination   /tmp/out",
		"",
		"[ Generate ]",
		"enter generate  esc back",
	}

	got := strings.Split(reviewed.View(), "\n")
	if len(got) != len(want) {
		t.Fatalf("the review has %d lines, want %d:\n%s", len(got), len(want), reviewed.View())
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i+1, got[i], want[i])
		}
	}
}

// TestEnterOnReviewFinishes: the review's enter hands the caller exactly the options
// the wizard composed, and nothing else.
func TestEnterOnReviewFinishes(t *testing.T) {
	provided := Values{KeyGroupID: "com.acme", KeyArtifactID: "billing", KeyName: "billing"}
	reviewed := openReview(t, func() model {
		wizard := reviewScreen(t, 80, 24, provided, []string{"web"}, "/tmp/out")
		wizard.screen = screenDependencies
		return wizard
	}())

	next, cmd := reviewed.Update(tea.KeyMsg{Type: tea.KeyEnter})
	msgs := commandMessages(cmd)
	if len(msgs) == 0 {
		t.Fatal("enter on the review produced no message; the run would hang")
	}
	if _, ok := msgs[0].(tea.QuitMsg); !ok {
		t.Fatalf("enter produced %T, want tea.QuitMsg", msgs[0])
	}

	wizard, ok := next.(model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.model", next)
	}
	finished := wizard.finished()
	if finished.GroupID != "com.acme" || finished.ArtifactID != "billing" {
		t.Errorf("finished coordinates = %s:%s, want com.acme:billing", finished.GroupID, finished.ArtifactID)
	}
	if got, want := strings.Join(finished.Deps, ","), "web"; got != want {
		t.Errorf("finished deps = %q, want %q", got, want)
	}
	// The destination is a review fact, never part of the request.
	if finished.Name != "billing" || finished.PackageName != "com.acme.billing" {
		t.Errorf("finished = %+v, want the form's values", finished)
	}
}

// TestEscReturnsToTheDependencyPaneWithEverything: going back is showing the pane
// again, so the level, the query, the cursor and the selection are all untouched.
func TestEscReturnsToTheDependencyPaneWithEverything(t *testing.T) {
	cat := fixtureCatalog(t)
	wizard := newModel(cat, newProjectForm(cat, nil, Default()), nil, Default(), ".")
	next, _ := wizard.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	wizard = next.(model)

	// A category, a checked row, and a query on the whole catalog.
	h := &harness{t: t, cat: cat, model: wizard}
	h.openDependencies()
	h.send(keyRunes("/"))
	h.typeText("spring")
	h.press(tea.KeyEnter)
	h.press(tea.KeySpace)

	before := h.model.deps
	reviewed := openReview(t, h.model)
	if reviewed.screen != screenReview {
		t.Fatalf("screen = %v, want the review", reviewed.screen)
	}

	back, _ := reviewed.Update(tea.KeyMsg{Type: tea.KeyEsc})
	returned, ok := back.(model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.model", back)
	}
	if returned.screen != screenDependencies {
		t.Fatalf("screen = %v after esc, want the dependency pane", returned.screen)
	}

	after := returned.deps
	if after.level != before.level || after.open != before.open {
		t.Errorf("level/open = %v/%q after the review, want %v/%q", after.level, after.open, before.level, before.open)
	}
	if after.list.Index() != before.list.Index() {
		t.Errorf("cursor = %d after the review, want %d", after.list.Index(), before.list.Index())
	}
	if got, want := after.list.FilterValue(), before.list.FilterValue(); got != want {
		t.Errorf("query = %q after the review, want %q", got, want)
	}
	if got, want := strings.Join(after.chosen(), ","), strings.Join(before.chosen(), ","); got != want {
		t.Errorf("checked = %q after the review, want %q", got, want)
	}
}

// TestReviewSaysNoneForNoDependencies is the fifth criterion: an empty selection is
// stated, not left blank.
func TestReviewSaysNoneForNoDependencies(t *testing.T) {
	reviewed := openReview(t, func() model {
		wizard := reviewScreen(t, 80, 24, nil, nil, ".")
		wizard.screen = screenDependencies
		return wizard
	}())

	view := reviewed.View()
	if strings.Contains(view, "Deps          \n") || strings.Contains(view, "Deps          \r\n") {
		t.Errorf("the dependency line is blank; view:\n%s", view)
	}
	if !strings.Contains(view, "Deps          none") {
		t.Errorf("the review does not say the selection is empty; view:\n%s", view)
	}
}

// TestReviewShowsTheDestinationFromRun: the destination is threaded into the model
// and rendered as given, not resolved against the working directory.
func TestReviewShowsTheDestinationFromRun(t *testing.T) {
	for _, destination := range []string{".", "/tmp/out", "relative/dir"} {
		t.Run(destination, func(t *testing.T) {
			reviewed := openReview(t, func() model {
				wizard := reviewScreen(t, 120, 30, nil, nil, destination)
				wizard.screen = screenDependencies
				return wizard
			}())

			want := "Destination   " + destination
			if !strings.Contains(reviewed.View(), want) {
				t.Errorf("the review does not show %q; view:\n%s", want, reviewed.View())
			}
		})
	}
}

// TestCtrlCOnReviewAborts: the confirmation screen still has a way out that writes
// nothing.
func TestCtrlCOnReviewAborts(t *testing.T) {
	reviewed := openReview(t, func() model {
		wizard := reviewScreen(t, 80, 24, nil, []string{"web"}, ".")
		wizard.screen = screenDependencies
		return wizard
	}())

	_, cmd := reviewed.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	msgs := commandMessages(cmd)
	if len(msgs) == 0 {
		t.Fatal("ctrl+c on the review produced no message")
	}
	if _, ok := msgs[0].(tea.InterruptMsg); !ok {
		t.Errorf("ctrl+c produced %T, want tea.InterruptMsg", msgs[0])
	}
}
