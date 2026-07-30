package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/kyhan/amux/internal/collect"
)

// View renders the whole dashboard.
func (m Model) View() string {
	if m.width == 0 {
		return "loading amux…"
	}
	if m.height < 5 || m.width < 20 {
		return m.renderTooSmall()
	}
	if m.loading && len(m.snap.Agents) == 0 {
		return "scanning for AI agents…"
	}

	header := m.renderHeader()
	footer := m.renderFooter()
	bodyH := m.height - 2
	if m.hideDetail {
		return header + "\n" + fitPane(m.renderList(m.width, bodyH), m.width, bodyH) + "\n" + footer
	}
	if m.stacked {
		return m.renderStackedBody(header, footer, bodyH)
	}

	availableW := m.width - 2 // vline renders a leading space plus the divider glyph.
	listW := availableW * 40 / 100
	if listW < 1 {
		listW = 1
	}
	if listW >= availableW {
		listW = availableW - 1
	}
	detailW := availableW - listW
	list := fitPane(m.renderList(listW, bodyH), listW, bodyH)
	detail := fitPane(m.renderDetail(detailW, bodyH), detailW, bodyH)
	body := lipgloss.JoinHorizontal(lipgloss.Top,
		list,
		styleDivider.Render(vline(bodyH)),
		detail,
	)
	return header + "\n" + body + "\n" + footer
}

func (m Model) renderTooSmall() string {
	lines := []string{truncateWidth("amux", m.width)}
	if m.height > 1 {
		lines = append(lines, truncateWidth("v/m", m.width))
	}
	if m.height > 2 {
		lines = append(lines, truncateWidth("terminal too small", m.width))
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderStackedBody(header, footer string, bodyH int) string {
	listH := (bodyH - 1) * 35 / 100
	if listH < 1 {
		listH = 1
	}
	detailH := bodyH - listH - 1
	if detailH < 1 {
		detailH = 1
		listH = bodyH - detailH - 1
	}
	list := fitPane(m.renderList(m.width, listH), m.width, listH)
	detail := fitPane(m.renderDetail(m.width, detailH), m.width, detailH)
	divider := styleDivider.Render(strings.Repeat("─", m.width))
	return header + "\n" + list + "\n" + divider + "\n" + detail + "\n" + footer
}

func fitPane(content string, width, height int) string {
	return lipgloss.NewStyle().Width(width).Height(height).Render(clipLines(content, height))
}

func clipLines(content string, maxLines int) string {
	if maxLines <= 0 {
		return ""
	}
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	return strings.Join(lines, "\n")
}

func vline(h int) string {
	lines := make([]string, h)
	for i := range lines {
		lines[i] = " │"
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderHeader() string {
	c := m.snap.Counts()
	title := styleTitle.Render("amux")
	sub := styleFaint.Render("agent tracker")

	segs := []string{}
	add := func(l collect.Lifecycle, label string) {
		if c[l] > 0 {
			st := lipgloss.NewStyle().Foreground(lifeColor(l))
			segs = append(segs, st.Render(fmt.Sprintf("%s %d %s", lifeGlyph(l), c[l], label)))
		}
	}
	add(collect.LifeWorking, "working")
	add(collect.LifeInput, "input")
	add(collect.LifeIdle, "idle")
	add(collect.LifeStale, "stale")
	add(collect.LifeError, "error")
	// Collapse finished states into one muted count.
	fin := c[collect.LifeDone] + c[collect.LifeStopped]
	if fin > 0 {
		segs = append(segs, styleMuted.Render(fmt.Sprintf("✓ %d done", fin)))
	}

	left := title + " " + sub
	right := strings.Join(segs, styleFaint.Render("  ·  "))
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m Model) renderFooter() string {
	if m.confirmingKill {
		return truncateWidth(lipgloss.NewStyle().Foreground(cRed).Bold(true).Render("kill "+m.killName+"?")+
			styleFaint.Render("  y to confirm · any other key cancels"), m.width)
	}
	if m.composing {
		return truncateWidth(styleTitle.Render("send: ")+m.composeInput+styleFaint.Render("▏  (enter to send · esc to cancel)"), m.width)
	}
	if m.naming {
		return truncateWidth(styleTitle.Render("note: ")+m.noteInput+styleFaint.Render("▏  (enter to save · esc to cancel)"), m.width)
	}
	if m.filtering {
		return truncateWidth(styleTitle.Render("/")+m.filter+styleFaint.Render("▏  (enter to apply · esc to clear)"), m.width)
	}
	doneKey := "d done"
	if m.hidden > 0 && !m.showDone {
		doneKey = fmt.Sprintf("d +%d done", m.hidden)
	}
	keys := []string{
		"j/k move", "enter attach", "c send", "x kill", "/ filter", "i note", doneKey, "v layout", "m preview", "q quit",
	}
	help := styleHelp.Render(strings.Join(keys, "  ·  "))
	stamp := ""
	switch {
	case m.notice != "":
		c := cGreen
		if strings.HasPrefix(m.notice, "send failed") || strings.HasPrefix(m.notice, "kill failed") {
			c = cRed
		}
		stamp = lipgloss.NewStyle().Foreground(c).Render(m.notice)
	case !m.lastRefresh.IsZero():
		stamp = styleFaint.Render("updated " + m.lastRefresh.Format("15:04:05"))
	}
	gap := m.width - lipgloss.Width(help) - lipgloss.Width(stamp)
	if gap < 1 {
		gap = 1
	}
	if m.filter != "" {
		help = styleMeta.Render("filter: "+m.filter) + "  " + help
		gap = m.width - lipgloss.Width(help) - lipgloss.Width(stamp)
		if gap < 1 {
			gap = 1
		}
	}
	return truncateWidth(help+strings.Repeat(" ", gap)+stamp, m.width)
}

func (m Model) renderList(w, h int) string {
	if len(m.selRows) == 0 {
		var empty string
		switch {
		case m.filter != "":
			empty = "no agents match “" + m.filter + "”"
		case m.hidden > 0:
			empty = fmt.Sprintf("no active agents · %d finished (press d to show)", m.hidden)
		default:
			empty = "no AI agents found"
		}
		return styleFaint.Render(empty)
	}

	// Vertical window so the selection stays visible.
	selRow := 0
	if m.sel < len(m.selRows) {
		selRow = m.selRows[m.sel]
	}
	start := 0
	if selRow >= h {
		start = selRow - h + 1
	}
	end := start + h
	if end > len(m.rows) {
		end = len(m.rows)
	}

	var b strings.Builder
	now := time.Now()
	for i := start; i < end; i++ {
		r := m.rows[i]
		if r.kind == rowHeader {
			b.WriteString(m.renderProjectHeader(r.project, w))
		} else {
			selected := m.sel < len(m.selRows) && m.selRows[m.sel] == i
			b.WriteString(m.renderAgentRow(r.agent, w, selected, now))
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func (m Model) renderProjectHeader(p *collect.Project, w int) string {
	name := styleProj.Render(p.Name)
	wk := ""
	if n := p.Working(); n > 0 {
		wk = lipgloss.NewStyle().Foreground(cGreen).Render(fmt.Sprintf(" ▶%d", n))
	}
	return truncateWidth(name+wk, w)
}

func (m Model) renderAgentRow(a *collect.Agent, w int, selected bool, now time.Time) string {
	glyph := lipgloss.NewStyle().Foreground(lifeColor(a.Life)).Render(lifeGlyph(a.Life))
	kind := styleFaint.Render(kindGlyph(a.Kind))

	name := a.Name
	nameStyle := stylePanel
	if selected {
		nameStyle = styleSel
	}

	// Right-aligned meta: model + tokens/cpu + age.
	meta := agentMeta(a, now)

	cursor := "  "
	if selected {
		cursor = styleSel.Render("❯ ")
	}

	left := fmt.Sprintf("%s%s %s %s", cursor, glyph, kind, nameStyle.Render(name))
	if note := m.notesStore.Get(a.ID); note != "" {
		left += "  " + styleNote.Render("» "+note)
	}
	metaR := styleFaint.Render(meta)

	gap := w - lipgloss.Width(left) - lipgloss.Width(metaR)
	if gap < 1 {
		// Truncate the name to make room.
		left = truncateWidth(left, w-lipgloss.Width(metaR)-1)
		gap = w - lipgloss.Width(left) - lipgloss.Width(metaR)
		if gap < 1 {
			gap = 1
		}
	}
	return left + strings.Repeat(" ", gap) + metaR
}

// agentMeta builds the compact right-hand metadata string for a row.
func agentMeta(a *collect.Agent, now time.Time) string {
	parts := []string{}
	if a.Model != "" {
		parts = append(parts, a.Model)
	}
	if a.Kind == collect.KindBackground && a.Tokens > 0 {
		parts = append(parts, humanTokens(a.Tokens))
	}
	if a.Kind == collect.KindInteractive && a.CPU >= 0.1 {
		parts = append(parts, fmt.Sprintf("%.0f%%cpu", a.CPU))
	}
	if age := a.Age(now); age > 0 && !a.UpdatedAt.IsZero() {
		parts = append(parts, humanDuration(age))
	}
	return strings.Join(parts, " · ")
}

func (m Model) renderDetail(w, h int) string {
	a := m.selectedAgent()
	if a == nil {
		return styleFaint.Render("select an agent")
	}
	now := time.Now()
	var b strings.Builder

	head := lipgloss.NewStyle().Foreground(lifeColor(a.Life)).Bold(true).
		Render(lifeGlyph(a.Life) + " " + lifeLabel(a.Life))
	b.WriteString(head + "  " + styleTitle.Render(a.Name) + "\n")
	if note := m.notesStore.Get(a.ID); note != "" {
		b.WriteString(styleNote.Render("» "+wrap(note, w)) + "\n")
	}
	b.WriteString("\n")

	kv := func(k, v string) {
		if strings.TrimSpace(v) == "" {
			return
		}
		b.WriteString(styleDetailKey.Render(fmt.Sprintf("%-9s", k)) + " " + v + "\n")
	}
	kindLabel := "background (headless)"
	if a.Kind == collect.KindInteractive {
		kindLabel = "interactive (tmux pane)"
	}
	kv("kind", kindLabel)
	kv("tool", a.Tool)
	kv("model", a.Model)
	kv("project", a.Project)
	if a.Kind == collect.KindInteractive {
		kv("pane", a.TmuxTarget)
		if a.CPU >= 0.1 {
			kv("cpu", fmt.Sprintf("%.1f%%", a.CPU))
		}
	} else {
		kv("session", a.ID)
		if a.Tokens > 0 {
			kv("tokens", humanTokens(a.Tokens))
		}
	}
	if !a.UpdatedAt.IsZero() {
		kv("updated", humanDuration(a.Age(now))+" ago")
	}

	if a.Kind == collect.KindInteractive {
		if a.NeedsInput {
			b.WriteString("\n" + lipgloss.NewStyle().Foreground(cYellow).Bold(true).
				Render("◆ waiting on your input") + "\n")
		}
		if len(a.Preview) > 0 {
			b.WriteString("\n" + styleDetailKey.Render("preview") + "\n")
			used := strings.Count(b.String(), "\n") + 1
			b.WriteString(renderPreview(a.Preview, w, h-used))
		} else if a.Detail != "" {
			b.WriteString("\n" + styleDetailKey.Render("status") + "\n")
			b.WriteString(wrap(a.Detail, w) + "\n")
		}
		return b.String()
	}

	if a.NeedsInput && a.Suggested != "" {
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(cYellow).Bold(true).Render("↩ suggested reply") + "\n")
		b.WriteString(wrap(a.Suggested, w) + "\n")
	}

	if a.Detail != "" {
		b.WriteString("\n" + styleDetailKey.Render("status") + "\n")
		b.WriteString(wrap(a.Detail, w) + "\n")
	}
	if a.Intent != "" {
		b.WriteString("\n" + styleDetailKey.Render("task") + "\n")
		b.WriteString(styleFaint.Render(wrap(clip(a.Intent, 400), w)) + "\n")
	}

	return b.String()
}

// ansiReset closes out any color/style left open by a captured line, so a
// pane's own ANSI codes can never bleed into the rest of amux's UI below it.
const ansiReset = "\x1b[0m"

// renderPreview renders the tail of a pane's captured content that fits in
// maxLines, each line truncated to w columns — a live, mux-style window into
// what the agent is actually doing right now. Lines carry the pane's own
// ANSI color codes (from capturePane's -e capture) rendered as-is, not
// flattened to a single dim color, so the preview looks like the real thing.
func renderPreview(lines []string, w, maxLines int) string {
	if maxLines < 3 {
		maxLines = 3
	}
	tail := lines
	if len(tail) > maxLines {
		tail = tail[len(tail)-maxLines:]
	}
	out := make([]string, len(tail))
	for i, l := range tail {
		out[i] = truncateWidth(l, w) + ansiReset
	}
	return strings.Join(out, "\n")
}
