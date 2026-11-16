package tui

import (
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Settings is the appearance the command line asked for. It is chosen once, before
// any screen is built, so every screen of the wizard draws the same way.
type Settings struct {
	// Fancy asks for the Unicode glyph set instead of the ASCII one. It changes
	// glyphs only: no coordinate, padding or copy moves.
	Fancy bool

	// NoColor strips every style. The glyphs stay, so a state is still legible
	// through the text itself.
	NoColor bool
}

// Theme is where every interactive screen reads its styles and glyphs from. A view
// never builds one: this file is the only place a style is created.
type Theme struct {
	// Border frames the focused field and separates the dependency pane's regions.
	// It is the ASCII set by default so every codepage can draw it; --fancy swaps
	// the value without moving a single coordinate.
	Border lipgloss.Border

	// Muted labels secondary text: the category of a dependency row today, the
	// descriptions VS5 adds later. It carries no layout, so stripping the color
	// is all --no-color will have to do.
	Muted lipgloss.Style

	// Notice is the one message the frame can carry, such as a guide it could not
	// open (US5.3). It is not muted: it is the line the user has to act on.
	Notice lipgloss.Style
}

// Default is the theme a run that asked for nothing gets: ASCII glyphs and one grey
// for text that must not compete with the value next to it.
func Default() Theme {
	return NewTheme(Settings{})
}

// NewTheme builds the theme for settings: the ASCII set and the styled text by
// default, the Unicode border under Fancy, and empty styles under NoColor.
func NewTheme(settings Settings) Theme {
	theme := Theme{
		Border: lipgloss.ASCIIBorder(),
		Muted:  lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "243", Dark: "245"}),
		Notice: lipgloss.NewStyle().Bold(true),
	}

	if settings.Fancy {
		// Only the glyphs: the border's edges are what a view can draw, and no
		// coordinate reads them.
		theme.Border = lipgloss.RoundedBorder()
	}
	if settings.NoColor {
		theme.Muted = lipgloss.NewStyle()
		theme.Notice = lipgloss.NewStyle()
	}

	return theme
}

// DisableColor turns styling off for everything that draws through lipgloss. The
// color profile is process-wide, which is exactly what --no-color means: the form
// is painted by huh and the rows by the list component, so emptying this package's
// styles alone would still let their escape bytes reach the terminal.
func DisableColor() {
	lipgloss.SetColorProfile(termenv.Ascii)
}

// huhForm adapts the theme to the structure huh expects. Only the border is
// carried over; layout, colors and key hints stay huh's defaults, and --no-color
// reaches them through DisableColor rather than by rebuilding a theme whose styles
// also carry the form's padding and text.
func (t Theme) huhForm() *huh.Theme {
	theme := huh.ThemeBase()
	theme.Focused.Base = theme.Focused.Base.BorderStyle(t.Border)
	theme.Focused.Card = theme.Focused.Base
	return theme
}
