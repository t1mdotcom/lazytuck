package tui

import (
	"charm.land/lipgloss/v2"

	"github.com/t1mdotcom/lazytuck/internal/state"
)

var (
	colAccent = lipgloss.Color("6") // cyan: focused border, titles
	colDim    = lipgloss.Color("8")
	colOK     = lipgloss.Color("2")
	colWarn   = lipgloss.Color("3")
	colBad    = lipgloss.Color("1")

	stAccent   = lipgloss.NewStyle().Foreground(colAccent)
	stDim      = lipgloss.NewStyle().Foreground(colDim)
	stOK       = lipgloss.NewStyle().Foreground(colOK)
	stWarn     = lipgloss.NewStyle().Foreground(colWarn)
	stBad      = lipgloss.NewStyle().Foreground(colBad)
	stBold     = lipgloss.NewStyle().Bold(true)
	stSelected = lipgloss.NewStyle().Reverse(true)
)

// glyph returns the one-character marker and its style for a state.
func glyph(s state.State) (string, lipgloss.Style) {
	switch s {
	case state.Linked:
		return "✓", stOK
	case state.Missing:
		return "○", stWarn
	case state.Same:
		return "=", stWarn
	case state.Drift:
		return "±", stWarn
	case state.Foreign:
		return "✗", stBad
	case state.Dangling:
		return "↯", stBad
	case state.Shadowed:
		return "↑", stDim
	case state.Unsupported:
		return "?", stDim
	default: // inactive
		return "·", stDim
	}
}
