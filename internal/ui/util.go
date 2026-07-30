package ui

import (
	"fmt"
	"time"

	"github.com/muesli/reflow/truncate"
	"github.com/muesli/reflow/wordwrap"
)

// truncateWidth shortens s to at most w display columns, ANSI-aware, adding an
// ellipsis when it cuts.
func truncateWidth(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return truncate.StringWithTail(s, uint(w), "…")
}

// wrap hard-wraps s to width w, preserving styling.
func wrap(s string, w int) string {
	if w <= 0 {
		return s
	}
	return wordwrap.String(s, w)
}

// clip limits a string to n runes without styling concerns (used on raw text).
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// humanTokens renders a token count compactly: 86375 → "86.4k".
func humanTokens(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// humanDuration renders a coarse age: 47m, 2h, 3d.
func humanDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
