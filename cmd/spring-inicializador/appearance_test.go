package main

// Tests for US5.4: the appearance flags. NO_COLOR is honoured by presence — even an
// empty value, which is what the convention says — and --fancy is only ever opt-in.
// The two reach the wizard through the runForm seam, so no test needs a terminal.

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/miferdev/spring-inicializador/internal/catalog"
	"github.com/miferdev/spring-inicializador/internal/options"
	"github.com/miferdev/spring-inicializador/internal/tui"
)

// unsetEnv removes a variable for the test and puts it back afterwards.
func unsetEnv(t *testing.T, key string) {
	t.Helper()

	original, had := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unsetting %s: %v", key, err)
	}
	t.Cleanup(func() {
		if had {
			os.Setenv(key, original)
		}
	})
}

// TestAppearanceHonoursNoColor pins the resolution: the flags, with NO_COLOR folded
// in by presence.
func TestAppearanceHonoursNoColor(t *testing.T) {
	unsetEnv(t, "NO_COLOR")

	if got := appearance(false, false); got != (tui.Settings{}) {
		t.Errorf("appearance(false, false) = %+v, want nothing asked for", got)
	}
	if got := appearance(true, false); got != (tui.Settings{Fancy: true}) {
		t.Errorf("appearance(true, false) = %+v, want fancy only", got)
	}

	// The convention is about presence, not about the value.
	for _, value := range []string{"", "1"} {
		t.Run("NO_COLOR="+value, func(t *testing.T) {
			t.Setenv("NO_COLOR", value)

			if got := appearance(false, false); got != (tui.Settings{NoColor: true}) {
				t.Errorf("appearance = %+v, want NO_COLOR to disable color", got)
			}
			if got := appearance(true, false); got != (tui.Settings{Fancy: true, NoColor: true}) {
				t.Errorf("appearance = %+v, want fancy and no color", got)
			}
		})
	}
}

// TestInteractiveRunPassesTheAppearance is the seam: the flags become the settings
// the wizard is built with.
func TestInteractiveRunPassesTheAppearance(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		noColor string
		setEnv  bool
		want    tui.Settings
	}{
		{name: "defaults", want: tui.Settings{}},
		{name: "fancy", args: []string{"--fancy"}, want: tui.Settings{Fancy: true}},
		{name: "no color flag", args: []string{"--no-color"}, want: tui.Settings{NoColor: true}},
		{name: "both flags", args: []string{"--fancy", "--no-color"}, want: tui.Settings{Fancy: true, NoColor: true}},
		{name: "NO_COLOR empty", setEnv: true, want: tui.Settings{NoColor: true}},
		{name: "NO_COLOR set", setEnv: true, noColor: "1", want: tui.Settings{NoColor: true}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeInitializr(t)
			terminalStdin(t)
			terminalStdout(t)
			if tc.setEnv {
				t.Setenv("NO_COLOR", tc.noColor)
			} else {
				unsetEnv(t, "NO_COLOR")
			}

			var got tui.Settings
			original := runForm
			runForm = func(cat *catalog.Catalog, provided tui.Values, dependencies []string, settings tui.Settings, destination string, stdin io.Reader, stdout io.Writer) (options.Options, tui.Outcome, error) {
				got = settings
				return options.Defaults(cat), tui.Finished, nil
			}
			t.Cleanup(func() { runForm = original })

			var stdout, stderr bytes.Buffer
			args := append(fake.args(tc.args...), "--output", t.TempDir())
			if code := runWithDeadline(t, args, &stdout, &stderr); code != exitOK {
				t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
			}
			if got != tc.want {
				t.Errorf("the wizard was given %+v, want %+v", got, tc.want)
			}
		})
	}
}
