package tui

// The review screen (US6.1): the summary the wizard shows before anything touches
// disk. It is a view over the model — no state, no commands — so esc cannot change a
// value and nothing can be written before enter.

import (
	"path/filepath"
	"strings"

	"github.com/miferdev/spring-inicializador/internal/catalog"
	"github.com/miferdev/spring-inicializador/internal/options"
)

// reviewLabelWidth is the column the values start in: wide enough for the longest
// label ("Coordinates", "Destination") plus a gap.
const reviewLabelWidth = 14

// reviewView renders the screen: a title, one line per decision, the Generate line
// and the keys that work here.
func (m model) reviewView() string {
	lines := append([]string{"Review"}, m.reviewLines()...)
	lines = append(lines, "", "[ Generate ]")

	hints := fitLine("enter generate"+footerGap+"esc back", m.width)
	return strings.Join(append(lines, m.theme.Muted.Render(hints)), "\n")
}

// reviewLines is one plain line per decision, in the order a person checks them: the
// identity of the project, the toolchain, what was picked and where it will go
// (US6.1).
func (m model) reviewLines() []string {
	opts := m.finished()

	var language, packaging catalog.Select
	if m.catalog != nil {
		language, packaging = m.catalog.Language, m.catalog.Packaging
	}

	rows := []struct{ label, value string }{
		{"Project", orDefault(opts.Name)},
		{"Coordinates", coordinates(opts)},
		{"Package", orDefault(opts.PackageName)},
		{"Build", m.buildLabel(opts.Build)},
		{"Boot", orDefault(opts.BootVersion)},
		{"Language", selectName(language, opts.Language)},
		{"Java", orDefault(opts.JavaVersion)},
		{"Packaging", selectName(packaging, opts.Packaging)},
		{"Deps", dependenciesLine(opts.Deps)},
		{"Destination", m.projectPath(opts)},
	}

	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, fitLine(reviewLine(row.label, row.value), m.width))
	}
	return lines
}

// projectPath is where the run will write: the Location the Project section asked
// for, plus the project's own name. filepath.Join is what turns "." into a path a
// reader can check, without inventing a leading "./" (VS8).
func (m model) projectPath(opts options.Options) string {
	return filepath.Join(m.project.state.Location, opts.Name)
}

// reviewLine is one label and its value in the value column.
func reviewLine(label, value string) string {
	padding := reviewLabelWidth - len(label)
	if padding < 1 {
		padding = 1
	}
	return label + strings.Repeat(" ", padding) + value
}

// dependenciesLine is the checked ids as the request will send them, in catalog
// order, or an explicit "none": an empty column would read as a missing line.
func dependenciesLine(deps []string) string {
	if len(deps) == 0 {
		return "none"
	}
	return strings.Join(deps, ", ")
}

// coordinates is the group and the artifact as the request names them.
func coordinates(opts options.Options) string {
	if opts.GroupID == "" && opts.ArtifactID == "" {
		return "default"
	}
	return orDefault(opts.GroupID) + ":" + orDefault(opts.ArtifactID)
}

// buildLabel is the build with the wrapper script it ships, so the review names the
// tool the generated project will use. The label itself comes from the metadata,
// like the form's options.
func (m model) buildLabel(build string) string {
	name := ""
	if m.catalog != nil {
		for _, value := range m.catalog.Type.Values {
			if mapped, ok := options.BuildName(value.ID); ok && mapped == build {
				name = value.Name
				break
			}
		}
	}
	if name == "" {
		name = build
	}
	if name == "" {
		return "default"
	}
	return name + " (" + buildWrapper(build) + ")"
}

// buildWrapper is the executable the generated project ships with.
func buildWrapper(build string) string {
	if build == options.BuildMaven {
		return "mvnw"
	}
	return "gradlew"
}

// selectName is the metadata's label for a select value, or the id when the server
// published no label for it.
func selectName(sel catalog.Select, id string) string {
	for _, value := range sel.Values {
		if value.ID == id && value.Name != "" {
			return value.Name
		}
	}
	return orDefault(id)
}

// orDefault is the value, or "default" when the request leaves the choice to the
// server: an empty column would read as a mistake.
func orDefault(value string) string {
	if strings.TrimSpace(value) == "" {
		return "default"
	}
	return value
}
