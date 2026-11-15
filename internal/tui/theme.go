package tui

import (
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// Theme is where every interactive screen reads its styles from. This story
// adds the muted label the dependency rows need; VS5 owns the rest (--fancy,
// --no-color, the panel frame) and extends this type instead of styling views.
type Theme struct {
	// Border frames the focused field. It is the ASCII set by default so every
	// codepage can draw it; --fancy will swap the value without moving a
	// single coordinate.
	Border lipgloss.Border

	// Muted labels secondary text: the category of a dependency row today, the
	// descriptions VS5 adds later. It carries no layout, so stripping the color
	// is all --no-color will have to do.
	Muted lipgloss.Style
}

// Default is the theme the wizard starts from: ASCII borders, and one grey for
// text that must not compete with the value next to it.
func Default() Theme {
	return Theme{
		Border: lipgloss.ASCIIBorder(),
		Muted:  lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "243", Dark: "245"}),
	}
}

// huhForm adapts the theme to the structure huh expects. Only the border is
// carried over; layout, colors and key hints stay huh's defaults so this file
// stays the single place a later story has to edit.
func (t Theme) huhForm() *huh.Theme {
	theme := huh.ThemeBase()
	theme.Focused.Base = theme.Focused.Base.BorderStyle(t.Border)
	theme.Focused.Card = theme.Focused.Base
	return theme
}
