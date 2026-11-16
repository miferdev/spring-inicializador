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
// and read the diff before keeping it. The files are ASCII by construction: a
// non-ASCII byte fails the test, so a glyph a legacy codepage cannot draw cannot
// slip in. Trailing spaces are trimmed — they never reach the screen, and an editor
// that strips them must not fail the suite.

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
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
	}{
		{name: "80x24-root", width: 80, height: 24},
		{name: "80x24-web", width: 80, height: 24, open: "Web"},
		{name: "110x24-root", width: 110, height: 24},
		{name: "110x24-web", width: 110, height: 24, open: "Web"},
		{name: "120x30-root", width: 120, height: 30},
		{name: "120x30-web", width: 120, height: 30, open: "Web"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			screen := newDependenciesModel(cat, nil, tc.width, tc.height, Default())
			if tc.open != "" {
				screen = openRootCategory(t, screen, tc.open)
			}

			compareGolden(t, filepath.Join("testdata", "dependencies-"+tc.name+".golden"), screen.view())
		})
	}
}

// compareGolden compares a frame to its golden file, or rewrites the file when the
// test runs with -update.
func compareGolden(t *testing.T, path, frame string) {
	t.Helper()

	want := goldenFrame(frame)
	for _, r := range want {
		if r > unicode.MaxASCII {
			t.Fatalf("%s is not ASCII (found %q)", path, r)
		}
	}

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

// goldenFrame is what a golden file holds: the frame with trailing spaces trimmed
// from every line and one final newline.
func goldenFrame(frame string) string {
	lines := strings.Split(strings.TrimRight(frame, "\n"), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}
	return strings.Join(lines, "\n") + "\n"
}
