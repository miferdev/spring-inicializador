package tui

import (
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// Theme is where every interactive screen reads its styles from. This story
// only needs the border around the focused field; VS5 owns the rest (--fancy,
// --no-color, the panel frame) and extends this type instead of styling views.
type Theme struct {
	// Border frames the focused field. It is the ASCII set by default so every
	// codepage can draw it; --fancy will swap the value without moving a
	// single coordinate.
	Border lipgloss.Border
}

// Default is the theme the project form starts from: ASCII borders and no color
// decisions beyond the ones huh's own base theme already makes.
func Default() Theme {
	return Theme{Border: lipgloss.ASCIIBorder()}
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
