package tui

// The detail region (US5.1): what the focused dependency is, in the metadata's own
// words. It has no state of its own — every frame asks the list what is focused and
// renders that — which is why moving the cursor, filtering or changing level needs
// no bookkeeping here.

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/miferdev/spring-inicializador/internal/catalog"
)

// detailGuides is how many guide titles the region shows, in the metadata's order.
const detailGuides = 3

// focusedDependency is the dependency row under the cursor. It is false at the
// category root, where the rows are categories, and when nothing matches, where
// there is nothing to describe.
func (d dependenciesModel) focusedDependency() (catalog.Dependency, bool) {
	item, ok := d.list.SelectedItem().(dependencyItem)
	if !ok {
		return catalog.Dependency{}, false
	}
	return item.dependency, true
}

// detailLines is the region's content: the name once, the category muted, the
// description wrapped to the region, and the guide titles when the dependency has
// any. A dependency with no guides gets no heading at all, rather than an empty one
// (US5.1). The lines are clipped to height, so the region can never push the frame
// past the terminal.
func (d dependenciesModel) detailLines(width, height int) []string {
	dependency, ok := d.focusedDependency()
	if !ok || width <= 0 || height <= 0 {
		return nil
	}

	lines := []string{
		fitLine(dependency.Name, width),
		d.theme.Muted.Render(fitLine(dependency.Category, width)),
		"",
	}
	lines = append(lines, wrapText(dependency.Description, width)...)

	if titles := guideTitles(dependency.Guides); len(titles) > 0 {
		lines = append(lines, "", "Guides")
		for _, title := range titles[:min(len(titles), detailGuides)] {
			lines = append(lines, fitLine(" - "+title, width))
		}
	}

	if len(lines) > height {
		lines = lines[:height]
	}
	return lines
}

// guideTitles is the metadata's guide titles, without the untitled links the
// server sometimes publishes: the client never invents a title to fill the gap.
func guideTitles(guides []catalog.Link) []string {
	titles := make([]string, 0, len(guides))
	for _, guide := range guides {
		if title := strings.TrimSpace(guide.Title); title != "" {
			titles = append(titles, title)
		}
	}
	return titles
}

// wrapText breaks text into lines no wider than width, at word boundaries, so the
// description reflows to the region instead of being clipped. A word wider than a
// whole line gets its own line, cut with "...", so nothing disappears without a
// mark (US5.1).
func wrapText(text string, width int) []string {
	if width <= 0 {
		return nil
	}

	lines := make([]string, 0, 4)
	line := ""
	for _, word := range strings.Fields(text) {
		switch {
		case ansi.StringWidth(word) > width:
			if line != "" {
				lines = append(lines, line)
				line = ""
			}
			lines = append(lines, ansi.Truncate(word, width, "..."))
		case line == "":
			line = word
		case ansi.StringWidth(line)+1+ansi.StringWidth(word) <= width:
			line += " " + word
		default:
			lines = append(lines, line)
			line = word
		}
	}
	if line != "" {
		lines = append(lines, line)
	}

	return lines
}
