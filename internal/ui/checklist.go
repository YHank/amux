package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/kyhan/amux/internal/collect"
)

// toggleDetailPanel lets `m` (preview) and `l` (checklist) share one panel
// slot: the key for the content already on screen hides the panel, the other
// key swaps the slot's content without hiding it.
func (m *Model) toggleDetailPanel(checklist bool) {
	if !m.hideDetail && m.showChecklist != checklist {
		m.showChecklist = checklist
	} else {
		m.hideDetail = !m.hideDetail
		m.showChecklist = checklist
	}
	m.persistViewPreferences()
}

func (m Model) renderDetailPanel(w, h int) string {
	if m.showChecklist {
		return m.renderChecklist(w, h)
	}
	return m.renderDetail(w, h)
}

type checklistBlock struct {
	phase    string
	line     string
	finished bool
	pending  bool
}

func (m Model) renderChecklist(w, h int) string {
	a := m.selectedAgent()
	if a == nil {
		return styleFaint.Render("select an agent")
	}
	now := time.Now()

	done := 0
	phases := map[string]bool{}
	blocks := make([]checklistBlock, 0, len(a.Checklist))
	for _, item := range a.Checklist {
		if item.Status == collect.ChecklistDone {
			done++
		}
		phases[item.Phase] = true
		blocks = append(blocks, checklistItemBlock(item, w, now))
	}

	header := styleTitle.Render("Plan")
	if len(a.Checklist) > 0 {
		header += "  " + styleMuted.Render(fmt.Sprintf("%d of %d done", done, len(a.Checklist)))
	}
	header += "  " + styleFaint.Render(agentRowName(a))
	top := []string{truncateWidth(header, w), ""}

	bottom := []string{"", styleDivider.Render(strings.Repeat("─", w)), styleProj.Render("Background tasks")}
	running := 0
	for _, child := range a.Subagents {
		if child.Life != collect.LifeWorking {
			continue
		}
		running++
		label := child.Description
		if label == "" {
			label = child.ID
		}
		glyph := lipgloss.NewStyle().Foreground(lifeColor(child.Life)).Render(lifeGlyph(child.Life))
		bottom = append(bottom, truncateWidth(glyph+" "+strings.Join(strings.Fields(label), " "), w))
	}
	if running == 0 {
		bottom = append(bottom, styleFaint.Render("no running tasks"))
	}

	if len(blocks) == 0 {
		body := []string{styleFaint.Render("no plan recorded for this agent"),
			styleFaint.Render(truncateWidth("plans are read from OmO todo lists", w))}
		return strings.Join(append(append(top, body...), bottom...), "\n")
	}

	showPhases := len(phases) > 1
	body := func(start, end int) []string {
		var lines []string
		if start > 0 {
			lines = append(lines, styleFaint.Render(fmt.Sprintf("… %d finished above", start)))
		}
		for i := start; i < end; i++ {
			if showPhases && (i == start || blocks[i].phase != blocks[i-1].phase) {
				lines = append(lines, styleDetailKey.Render(truncateWidth(blocks[i].phase, w)))
			}
			lines = append(lines, blocks[i].line)
		}
		if hidden := blocks[end:]; len(hidden) > 0 {
			label := "more pending"
			for _, block := range hidden {
				if !block.pending {
					label = "more"
				}
			}
			lines = append(lines, styleFaint.Render(fmt.Sprintf("… %d %s", len(hidden), label)))
		}
		return lines
	}
	// The background section stays pinned to the bottom. A taller plan first
	// folds finished items above the running one, then folds the tail, so the
	// running item is the last thing to leave the panel.
	available := h - len(top) - len(bottom)
	if available < 2 {
		bottom = nil
		available = h - len(top)
	}
	start, end := 0, len(blocks)
	for start < end && blocks[start].finished && len(body(start, end)) > available {
		start++
	}
	for end > start+1 && len(body(start, end)) > available {
		end--
	}
	return strings.Join(append(append(top, body(start, end)...), bottom...), "\n")
}

func checklistItemBlock(item collect.ChecklistItem, w int, now time.Time) checklistBlock {
	var glyph, meta string
	textStyle := stylePanel
	metaStyle := styleMuted
	finished := false
	switch item.Status {
	case collect.ChecklistDone:
		glyph = lipgloss.NewStyle().Foreground(cGreen).Render("✓")
		meta = "done"
		if !item.StartedAt.IsZero() && item.FinishedAt.After(item.StartedAt) {
			meta += " · " + checklistDuration(item.FinishedAt.Sub(item.StartedAt))
		}
		finished = true
	case collect.ChecklistActive:
		glyph = lipgloss.NewStyle().Foreground(cBlue).Bold(true).Render("◉")
		textStyle = stylePanel.Bold(true)
		metaStyle = styleMeta
		meta = "running"
		if !item.StartedAt.IsZero() && now.After(item.StartedAt) {
			meta += " · " + checklistDuration(now.Sub(item.StartedAt))
		}
	case collect.ChecklistDropped:
		glyph = styleFaint.Render("✕")
		textStyle = styleFaint.Strikethrough(true)
		metaStyle = styleFaint
		meta = "dropped"
		finished = true
	default:
		glyph = styleMuted.Render("○")
		textStyle = styleMuted
		metaStyle = styleFaint
		meta = "pending"
	}

	metaText := metaStyle.Render(meta)
	textW := w - 2 - lipgloss.Width(meta) - 2
	if textW < 8 {
		metaText = ""
		textW = max(1, w-2)
	}
	text := textStyle.Render(truncateWidth(strings.Join(strings.Fields(item.Content), " "), textW))
	gap := max(1, w-2-lipgloss.Width(text)-lipgloss.Width(metaText))
	// No final truncateWidth: reflow's StringWithTail also cuts a string that
	// exactly fills the width, which would clip the right-aligned meta.
	line := glyph + " " + text + strings.Repeat(" ", gap) + metaText
	return checklistBlock{phase: item.Phase, line: line, finished: finished, pending: !finished && item.Status != collect.ChecklistActive}
}

// checklistDuration is finer than humanDuration because plan steps typically
// last minutes: 42s, 6m 58s, 1h 4m.
func checklistDuration(d time.Duration) string {
	d = d.Round(time.Second)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	default:
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
	}
}
