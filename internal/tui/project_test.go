package tui

// Tests for VS8.1: the wizard asks what the project is called and where it goes.
// The name is the single source for the artifact id, the server's `name` and the
// folder, and the location is the parent directory the run writes into. The model
// is driven with the messages the event loop delivers, so no test needs a terminal.

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/miferdev/spring-inicializador/internal/options"
)

// TestTheNameIsTheSingleSource: whatever reaches the Name row reaches both fields
// the server receives, and the package follows the group and that name.
func TestTheNameIsTheSingleSource(t *testing.T) {
	h := newSizedHarness(t, 80, 24)

	if got := h.focused(); got != KeyName {
		t.Fatalf("focus = %q, want the Project section's first row %q", got, KeyName)
	}

	h.typeText("billing")
	if !strings.Contains(h.model.project.state.Name, "billing") {
		t.Fatalf("typing did not reach the name row: %q", h.model.project.state.Name)
	}

	opts := optionsFromState(h.cat, h.model.project.state)
	if opts.ArtifactID != opts.Name {
		t.Errorf("artifact id = %q, name = %q, want one value for both", opts.ArtifactID, opts.Name)
	}
	if opts.ArtifactID == options.Defaults(h.cat).ArtifactID {
		t.Errorf("artifact id = %q, want the typed name", opts.ArtifactID)
	}
	if want := options.DerivePackageName(opts.GroupID, opts.Name); opts.PackageName != want {
		t.Errorf("PackageName = %q, want the package derived from group and name %q", opts.PackageName, want)
	}

	// The request carries the same value in the three places the server reads it.
	request := opts.Form()
	if request.Get("artifactId") != opts.Name || request.Get("name") != opts.Name || request.Get("baseDir") != opts.Name {
		t.Errorf("request = artifactId %q, name %q, baseDir %q; want %q in all three",
			request.Get("artifactId"), request.Get("name"), request.Get("baseDir"), opts.Name)
	}
}

// TestTheLocationDefaultsToTheCurrentDirectory and is the parent the project is
// written into, not the project itself.
func TestTheLocationDefaultsToTheCurrentDirectory(t *testing.T) {
	h := newSizedHarness(t, 80, 24)

	if got := h.model.project.state.Location; got != locationDefault {
		t.Fatalf("Location = %q, want %q", got, locationDefault)
	}

	// The review shows the resolved project path, never the bare location.
	wizard := reviewScreen(t, 80, 24, nil, nil)
	wizard.screen = screenReview
	if want := "Destination   " + filepath.Join(locationDefault, wizard.finished().Name); !strings.Contains(wizard.View(), want) {
		t.Errorf("the review does not show %q; view:\n%s", want, wizard.View())
	}
}

// TestTypingTheLocationMovesTheDestination: the row is typed into like any other,
// and the destination the run reports is the location joined with the name.
func TestTypingTheLocationMovesTheDestination(t *testing.T) {
	h := newSizedHarness(t, 80, 24)

	h.walkTo(KeyLocation)
	typed := "/tmp/misproyectos"
	h.typeText(typed)

	location := h.model.project.state.Location
	if !strings.HasSuffix(location, typed) {
		t.Fatalf("Location = %q, want it to end with the typed %q", location, typed)
	}

	h.model.screen = screenReview
	want := "Destination   " + filepath.Join(location, h.model.finished().Name)
	if !strings.Contains(h.model.View(), want) {
		t.Errorf("the review does not show %q; view:\n%s", want, h.model.View())
	}

	if h.model.project.state.Location == locationDefault {
		t.Error("the typed location did not move the destination")
	}
}
