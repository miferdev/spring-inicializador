package tui

// Tests for US5.4: the appearance the flags choose. ASCII by default, the Unicode
// border under --fancy with nothing else moving, and no escape byte at all under
// --no-color while the glyphs still carry the state. No test needs a terminal.

import (
	"reflect"
	"strings"
	"testing"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// forceColor makes lipgloss render styles the way a true-color terminal would, so
// the "no escape byte" assertions have something to prove. It restores the profile
// afterwards.
func forceColor(t *testing.T) {
	t.Helper()

	original := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(original) })
}

// paneAt renders the Web category at a size with a theme.
func paneAt(t *testing.T, theme Theme, width, height int) dependenciesModel {
	t.Helper()
	return openRootCategory(t, newDependenciesModel(fixtureCatalog(t), nil, width, height, theme), "Web")
}

// TestDefaultFrameIsASCII: a run that asked for nothing gets the ASCII set, with no
// escape byte and no character outside it.
func TestDefaultFrameIsASCII(t *testing.T) {
	view := paneAt(t, Default(), 120, 30).view()

	if strings.Contains(view, "\x1b") {
		t.Error("the default frame contains an escape byte")
	}
	for _, r := range view {
		if r > unicode.MaxASCII {
			t.Errorf("the default frame is not ASCII (found %q)", r)
			break
		}
	}
	if got := Default().Border.Left; got != "|" {
		t.Errorf("the default separator = %q, want the ASCII %q", got, "|")
	}
}

// TestFancySwapsOnlyTheBorderGlyphs is the story's diff: the two frames have the
// same shape, and the characters that change are exactly the theme's border.
func TestFancySwapsOnlyTheBorderGlyphs(t *testing.T) {
	plain := strings.Split(paneAt(t, Default(), 120, 30).view(), "\n")
	fancy := strings.Split(paneAt(t, NewTheme(Settings{Fancy: true}), 120, 30).view(), "\n")

	if len(plain) != len(fancy) {
		t.Fatalf("the frames have %d and %d lines, want the layout unchanged", len(plain), len(fancy))
	}

	diff := map[rune]rune{}
	for i := range plain {
		left, right := []rune(plain[i]), []rune(fancy[i])
		if len(left) != len(right) {
			t.Fatalf("line %d has %d and %d characters, want the same width", i+1, len(left), len(right))
		}
		for j := range left {
			if left[j] == right[j] {
				continue
			}
			if before, seen := diff[left[j]]; seen && before != right[j] {
				t.Errorf("the ASCII glyph %q maps to both %q and %q", left[j], before, right[j])
			}
			diff[left[j]] = right[j]
		}
	}

	// The glyph table this story promises: the border's vertical edge, and only it.
	if got := Default().Border.Left; got != "|" {
		t.Fatalf("the default separator = %q, want %q", got, "|")
	}
	if got := NewTheme(Settings{Fancy: true}).Border.Left; got != "│" {
		t.Fatalf("the fancy separator = %q, want %q", got, "│")
	}
	want := map[rune]rune{'|': '│'}
	if !reflect.DeepEqual(diff, want) {
		t.Errorf("the frames differ in %v, want only the border glyphs %v", diff, want)
	}
	if got := fancy[len(fancy)-1]; got != plain[len(plain)-1] {
		t.Errorf("the hints line changed with --fancy: %q, want %q", got, plain[len(plain)-1])
	}
}

// TestNoColorFrameHasNoEscapeBytes: --no-color is not a dimmer theme, it is no
// styling at all, and the state is still legible through the glyphs.
func TestNoColorFrameHasNoEscapeBytes(t *testing.T) {
	forceColor(t)

	// A styled theme does render escapes under a color terminal, so the assertion
	// below is about --no-color, not about the test environment.
	if colored := paneAt(t, Default(), 120, 30).view(); !strings.Contains(colored, "\x1b") {
		t.Fatal("the styled frame carried no escape, so this test would prove nothing")
	}

	screen := newDependenciesModel(fixtureCatalog(t), []string{"web"}, 120, 30, NewTheme(Settings{NoColor: true}))
	screen = openRootCategory(t, screen, "Web")
	view := screen.view()

	if strings.Contains(view, "\x1b") {
		t.Errorf("the --no-color frame contains an escape byte")
	}
	for _, want := range []string{"[x] Spring Web", "[ ] Spring Reactive Web", "|", "> [x]"} {
		if !strings.Contains(view, want) {
			t.Errorf("the --no-color frame lost %q; view:\n%s", want, view)
		}
	}
}

// TestDisableColorReachesEveryComponent: the form is painted by huh and the rows by
// the list component, so the process-wide profile is the only thing that can keep
// both plain.
func TestDisableColorReachesEveryComponent(t *testing.T) {
	forceColor(t)

	theme := Default()
	if got := theme.Notice.Render("x"); !strings.Contains(got, "\x1b") {
		t.Fatalf("the styled notice rendered %q, want an escape to strip", got)
	}
	if got := theme.huhForm().Focused.FocusedButton.Render("x"); !strings.Contains(got, "\x1b") {
		t.Fatalf("huh's button rendered %q, want an escape to strip", got)
	}

	DisableColor()

	renders := []struct {
		name string
		got  string
	}{
		{name: "muted", got: theme.Muted.Render("x")},
		{name: "notice", got: theme.Notice.Render("x")},
		{name: "huh button", got: theme.huhForm().Focused.FocusedButton.Render("x")},
		{name: "huh placeholder", got: theme.huhForm().Focused.TextInput.Placeholder.Render("x")},
	}
	for _, rendered := range renders {
		if strings.Contains(rendered.got, "\x1b") {
			t.Errorf("%s still renders an escape: %q", rendered.name, rendered.got)
		}
	}
}

// TestNewThemeSettings pins the two knobs independently: glyphs and styles.
func TestNewThemeSettings(t *testing.T) {
	forceColor(t)

	cases := []struct {
		name     string
		settings Settings
		border   string
		colored  bool
	}{
		{name: "default", settings: Settings{}, border: "|", colored: true},
		{name: "fancy", settings: Settings{Fancy: true}, border: "│", colored: true},
		{name: "no color", settings: Settings{NoColor: true}, border: "|", colored: false},
		{name: "fancy, no color", settings: Settings{Fancy: true, NoColor: true}, border: "│", colored: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			theme := NewTheme(tc.settings)
			if got := theme.Border.Left; got != tc.border {
				t.Errorf("separator = %q, want %q", got, tc.border)
			}
			gotColored := strings.Contains(theme.Notice.Render("x"), "\x1b")
			if gotColored != tc.colored {
				t.Errorf("the notice is styled = %v, want %v", gotColored, tc.colored)
			}
		})
	}
}
