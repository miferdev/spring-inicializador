package tui

// Golden frames for the dependency pane (US5.2): the files under testdata/ are the
// pane as it renders at the three breakpoints, at the category root and inside one
// category. They are compared byte for byte, so any layout change has to be seen in
// a diff.
//
// Rewrite them with:
//
//	go test ./internal/tui -run TestGoldenDependencyPane -update
//
// and read the diff before keeping it. The ASCII frames are ASCII by construction: a
// non-ASCII byte fails the test, so a glyph a legacy codepage cannot draw cannot slip
// in. The one fancy frame may add its border's glyphs and nothing else. Trailing
// spaces are trimmed — they never reach the screen, and an editor that strips them
// must not fail the suite.

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"github.com/charmbracelet/lipgloss"
)

// updateGolden rewrites the golden files instead of comparing them.
var updateGolden = flag.Bool("update", false, "rewrite the golden frames")

// TestGoldenDependencyPane pins the pane at each breakpoint, at the category root
// and inside one category.
func TestGoldenDependencyPane(t *testing.T) {
	cat := fixtureCatalog(t)

	cases := []struct {
		name          string
		width, height int
		open          string // empty means the category root
		fancy         bool
	}{
		{name: "80x24-root", width: 80, height: 24},
		{name: "80x24-web", width: 80, height: 24, open: "Web"},
		{name: "110x24-root", width: 110, height: 24},
		{name: "110x24-web", width: 110, height: 24, open: "Web"},
		{name: "120x30-root", width: 120, height: 30},
		{name: "120x30-web", width: 120, height: 30, open: "Web"},
		// The same frame under --fancy: only the border glyphs may differ (US5.4).
		{name: "120x30-web-fancy", width: 120, height: 30, open: "Web", fancy: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			theme := Default()
			if tc.fancy {
				theme = NewTheme(Settings{Fancy: true})
			}

			screen := newDependenciesModel(cat, nil, tc.width, tc.height, theme)
			if tc.open != "" {
				screen = openRootCategory(t, screen, tc.open)
			}

			compareGolden(t, filepath.Join("testdata", "dependencies-"+tc.name+".golden"), screen.view(), theme.Border)
		})
	}
}

// TestGoldenReviewFrames pins the review at both sizes. Its lines are fixed, so the
// two frames are identical: the golden proves the summary does not depend on the
// width (US6.1).
func TestGoldenReviewFrames(t *testing.T) {
	for _, size := range []struct {
		name          string
		width, height int
	}{
		{name: "80x24", width: 80, height: 24},
		{name: "120x30", width: 120, height: 30},
	} {
		t.Run(size.name, func(t *testing.T) {
			// The default answers plus two dependencies: a state the flags can reach.
			wizard := reviewScreen(t, size.width, size.height, nil, []string{"web", "data-jpa"}, ".")
			wizard.screen = screenReview

			compareGolden(t, filepath.Join("testdata", "review-"+size.name+".golden"), wizard.View(), Default().Border)
		})
	}
}

// TestGoldenFormSections pins each section of the project form at both sizes: one
// page at a time, with its header, and the hint bar huh draws under it. The three
// sections are the whole form (US7.1, US7.2); the frames are the reason the screen
// fits a 24-row terminal (US7.3).
//
// Rewrite them with:
//
//	go test ./internal/tui -run TestGoldenFormSections -update
func TestGoldenFormSections(t *testing.T) {
	cases := []struct {
		name    string
		section int
	}{
		{name: "identity", section: 0},
		{name: "build", section: 1},
		{name: "versions", section: 2},
	}

	for _, size := range []struct {
		name          string
		width, height int
	}{
		{name: "80x24", width: 80, height: 24},
		{name: "120x30", width: 120, height: 30},
	} {
		for _, tc := range cases {
			t.Run(size.name+"-"+tc.name, func(t *testing.T) {
				h := newSizedHarness(t, size.width, size.height)
				section := h.model.project.sections[tc.section]
				h.walkTo(section.fields[0].key)

				compareFormGolden(
					t,
					filepath.Join("testdata", "form-"+size.name+"-"+tc.name+".golden"),
					h.model.View(),
					sectionHint(section),
				)
			})
		}
	}
}

// The hint bar huh draws under a section, spelled out because two things depend on
// it: the golden pins it byte for byte, and US7.3 requires it to stay readable at 80
// columns. A section of text rows shows the single binding an input has; a section
// of selects shows the arrows, the filter and the step back.
const (
	inputHint  = "enter next"
	selectHint = "↑ up • ↓ down • / filter • shift+tab back • enter select"
)

// sectionHint is the hint bar a section's rows produce.
func sectionHint(section section) string {
	if len(section.fields) > 0 && section.fields[0].kind == fieldText {
		return inputHint
	}
	return selectHint
}

// compareFormGolden compares a project-form frame to its golden. A form frame is
// the only screen this project does not draw end to end: huh draws the hint bar and
// spells it with its own glyphs ("↑ up • ↓ down"), which no public option of the
// library can change. So the ASCII rule covers every line the form draws, and the
// hint bar is asserted exactly instead — the golden still holds it byte for byte,
// so a glyph that drifts still fails the test (US7.3).
func compareFormGolden(t *testing.T, path, frame, hint string) {
	t.Helper()

	lines := strings.Split(strings.TrimRight(frame, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("%s: the frame has %d lines, want fields and a hint bar", path, len(lines))
	}
	if got := strings.TrimRight(lines[len(lines)-1], " "); got != hint {
		t.Errorf("%s: hint bar = %q, want %q", path, got, hint)
	}

	want := goldenFrame(frame)
	body := want
	if split := strings.LastIndex(strings.TrimRight(want, "\n"), "\n"); split >= 0 {
		body = want[:split]
	}
	assertASCIIFrame(t, path, body, nil)

	writeOrCompareGolden(t, path, want)
}

// compareGolden compares a frame to its golden file, or rewrites the file when the
// test runs with -update. The frame is ASCII, except that a frame drawn with a
// Unicode border may use that border's own glyphs.
func compareGolden(t *testing.T, path, frame string, border lipgloss.Border) {
	t.Helper()

	want := goldenFrame(frame)
	assertASCIIFrame(t, path, want, borderGlyphs(border))
	writeOrCompareGolden(t, path, want)
}

// assertASCIIFrame refuses a frame that carries a rune a legacy codepage cannot
// draw, apart from the glyphs of the border it draws with.
func assertASCIIFrame(t *testing.T, path, frame string, allowed map[rune]bool) {
	t.Helper()

	for _, r := range frame {
		if r > unicode.MaxASCII && !allowed[r] {
			t.Fatalf("%s has %q, which is neither ASCII nor one of its border's glyphs", path, r)
		}
	}
}

// writeOrCompareGolden writes the golden with -update, and otherwise compares it
// byte for byte.
func writeOrCompareGolden(t *testing.T, path, want string) {
	t.Helper()

	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("creating %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
			t.Fatalf("writing %s: %v", path, err)
		}
		return
	}

	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading golden %s (run with -update to create it): %v", path, err)
	}
	if got := string(saved); got != want {
		t.Errorf("the frame does not match %s\n--- want ---\n%s\n--- got ---\n%s", path, got, want)
	}
}

// borderGlyphs is every character a border can draw: the only non-ASCII runes a
// fancy frame may contain.
func borderGlyphs(border lipgloss.Border) map[rune]bool {
	edges := []string{
		border.Top, border.Bottom, border.Left, border.Right,
		border.TopLeft, border.TopRight, border.BottomLeft, border.BottomRight,
		border.MiddleLeft, border.MiddleRight, border.Middle,
		border.MiddleTop, border.MiddleBottom,
	}

	glyphs := make(map[rune]bool, len(edges))
	for _, edge := range edges {
		for _, r := range edge {
			glyphs[r] = true
		}
	}
	return glyphs
}

// goldenFrame is what a golden file holds: the frame with trailing spaces trimmed
// from every line and one final newline.
func goldenFrame(frame string) string {
	lines := strings.Split(strings.TrimRight(frame, "\n"), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}
	return strings.Join(lines, "\n") + "\n"
}
