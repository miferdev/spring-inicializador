package main

// The run report (US6.2): one plain line per pipeline step, printed by the command
// line as the work happens. It is not a screen (D10): the wizard composes options
// and returns, and everything after that is text.

import (
	"fmt"
	"io"
	"strings"
)

// The step labels, in the order they happen.
const (
	stepCatalog     = "Catalog"
	stepGenerating  = "Generating"
	stepExtracting  = "Extracting"
	stepPermissions = "Permissions"
)

// stepLabelWidth is the column the outcome starts in: the label, a space and the
// dots that carry the eye across.
const stepLabelWidth = 16

// stepReport writes the run's progress: one line per step, its outcome, and what
// the step produced. A slow generation then looks like work rather than a hang, and
// a failure points at the step that failed (US6.2).
type stepReport struct{ out io.Writer }

// ok reports a finished step and the detail of what it did.
func (r stepReport) ok(label, detail string) {
	fmt.Fprintf(r.out, "%s ok   (%s)\n", stepDots(label), detail)
}

// fail reports the step that stopped the run. The reason follows on stderr, so the
// report stays one aligned line per step.
func (r stepReport) fail(label string) {
	fmt.Fprintf(r.out, "%s fail\n", stepDots(label))
}

// stepDots is the label followed by the dots that pad it to the outcome column.
func stepDots(label string) string {
	dots := stepLabelWidth - len(label) - 1
	if dots < 1 {
		dots = 1
	}
	return label + " " + strings.Repeat(".", dots)
}

// count is a number and its noun, in the singular when it is one.
func count(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// size is an archive's size the way a person reads it: bytes below a kilobyte,
// rounded kilobytes below a megabyte, and one decimal above that.
func size(bytes int) string {
	switch {
	case bytes < 1024:
		return fmt.Sprintf("%d bytes", bytes)
	case bytes < 1024*1024:
		return fmt.Sprintf("%d KB", (bytes+512)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
	}
}

// executables is the permissions step's detail: the files the extraction actually
// made executable, named as the user sees them inside the project, or an explicit
// statement when the archive shipped none. The archive's top-level directory is the
// project itself — the server names it after baseDir, which this tool sets to the
// artifact id — so it is not repeated here (US6.2).
// executables names the files the extraction made executable, as the archive
// spells them relative to the project; the extraction already dropped the
// archive's top directory (VS8.2).
func executables(files []string) string {
	if len(files) == 0 {
		return "no executable files"
	}
	return strings.Join(files, ", ")
}
