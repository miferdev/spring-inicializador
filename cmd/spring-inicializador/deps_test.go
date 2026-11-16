package main

// The caller contract for dependency selection (US3.2): --deps answers the
// dependency screen the way the project flags answer form rows, and the set the
// screen returns is the set that gets generated. No test starts the wizard: the
// runForm seam is a function variable for exactly this.

import (
	"bytes"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/miferdev/spring-inicializador/internal/catalog"
	"github.com/miferdev/spring-inicializador/internal/options"
	"github.com/miferdev/spring-inicializador/internal/tui"
)

func TestDependenciesReachTheWizardAndTheRequest(t *testing.T) {
	fake := newFakeInitializr(t)
	terminalStdin(t)
	terminalStdout(t)

	var seeded []string
	var reviewed string
	original := runForm
	runForm = func(cat *catalog.Catalog, provided tui.Values, dependencies []string, settings tui.Settings, destination string, stdin io.Reader, stdout io.Writer) (options.Options, tui.Outcome, error) {
		seeded = append([]string(nil), dependencies...)
		reviewed = destination

		// What the screen would return after the user unchecked data-jpa.
		chosen := options.Defaults(cat)
		chosen.Deps = []string{"web"}
		return chosen, tui.Finished, nil
	}
	t.Cleanup(func() { runForm = original })

	output := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := runWithDeadline(t, fake.args("--deps", "web,data-jpa", "--output", output), &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}
	if want := []string{"web", "data-jpa"}; !reflect.DeepEqual(seeded, want) {
		t.Errorf("the wizard was seeded with %v, want the --deps value %v", seeded, want)
	}
	if reviewed != output {
		t.Errorf("the wizard was told the destination %q, want the --output %q", reviewed, output)
	}
	if got := fake.generationForm.Get("dependencies"); got != "web" {
		t.Errorf("dependencies = %q, want the set the screen returned, web", got)
	}
	if !strings.Contains(stdout.String(), "Created ") {
		t.Errorf("stdout = %q, want the destination report", stdout.String())
	}
	assertNoANSI(t, stdout.String())
}
