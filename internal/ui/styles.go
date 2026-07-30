package ui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/kyhan/amux/internal/collect"
)

// Palette — chosen to read on both dark and light terminals via ANSI 256.
var (
	cGreen  = lipgloss.Color("42")  // working
	cYellow = lipgloss.Color("214") // input needed
	cBlue   = lipgloss.Color("39")  // idle / alive
	cGray   = lipgloss.Color("245") // done / muted
	cRed    = lipgloss.Color("203") // error
	cOrange = lipgloss.Color("208") // stale
	cFaint  = lipgloss.Color("240")
	cText   = lipgloss.Color("252")
	cAccent = lipgloss.Color("170") // headers / selection
)

var (
	styleTitle     = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	styleHelp      = lipgloss.NewStyle().Foreground(cFaint)
	styleProj      = lipgloss.NewStyle().Bold(true).Foreground(cText)
	styleProjPath  = lipgloss.NewStyle().Foreground(cFaint)
	styleSel       = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	styleMuted     = lipgloss.NewStyle().Foreground(cGray)
	styleFaint     = lipgloss.NewStyle().Foreground(cFaint)
	styleMeta      = lipgloss.NewStyle().Foreground(cBlue)
	styleDetailKey = lipgloss.NewStyle().Foreground(cFaint)

	styleDivider = lipgloss.NewStyle().Foreground(cFaint)
	stylePanel   = lipgloss.NewStyle().Foreground(cText)
)

// lifeStyle maps a lifecycle to its accent color.
func lifeColor(l collect.Lifecycle) lipgloss.Color {
	switch l {
	case collect.LifeWorking:
		return cGreen
	case collect.LifeInput:
		return cYellow
	case collect.LifeIdle:
		return cBlue
	case collect.LifeError:
		return cRed
	case collect.LifeStale:
		return cOrange
	case collect.LifeStopped, collect.LifeDone:
		return cGray
	default:
		return cGray
	}
}

// lifeGlyph is a compact status badge shown at the start of each row.
func lifeGlyph(l collect.Lifecycle) string {
	switch l {
	case collect.LifeWorking:
		return "▶"
	case collect.LifeInput:
		return "◆"
	case collect.LifeIdle:
		return "●"
	case collect.LifeError:
		return "✖"
	case collect.LifeStale:
		return "⚠"
	case collect.LifeStopped:
		return "■"
	case collect.LifeDone:
		return "✓"
	default:
		return "·"
	}
}

// lifeLabel is a short human word for the status.
func lifeLabel(l collect.Lifecycle) string {
	switch l {
	case collect.LifeWorking:
		return "working"
	case collect.LifeInput:
		return "input"
	case collect.LifeIdle:
		return "idle"
	case collect.LifeError:
		return "error"
	case collect.LifeStale:
		return "stale"
	case collect.LifeStopped:
		return "stopped"
	case collect.LifeDone:
		return "done"
	default:
		return "?"
	}
}

// kindGlyph distinguishes an interactive pane agent from a headless one.
func kindGlyph(k collect.Kind) string {
	if k == collect.KindInteractive {
		return "▤" // has a tmux pane
	}
	return "◇" // headless background daemon
}
