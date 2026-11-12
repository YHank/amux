package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kyhan/amux/internal/collect"
)

func (m Model) projectTmuxTargets(selected *collect.Agent) []string {
	if m.selectedBackend() != collect.BackendTmux {
		return nil
	}
	targets := []string{selected.TmuxTarget}
	if selected.Project == "" {
		return targets
	}
	for _, a := range m.snap.Agents {
		if a.Project == selected.Project && validBackend(a.Backend) == collect.BackendTmux &&
			a.Kind == collect.KindInteractive && a.TmuxTarget != "" && a.TmuxTarget != selected.TmuxTarget {
			targets = append(targets, a.TmuxTarget)
		}
	}
	return targets
}

func (m Model) handleMoveMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.moveAction = nil
		return m, nil
	case "ctrl+c":
		m.moveAction = nil
		m.cancelAutoTitles("")
		m.persistViewPreferences()
		m.quit = true
		return m, tea.Quit
	case "j", "k", "up", "down", "tab", "shift+tab":
		m.movePopup = !m.movePopup
		return m, nil
	case "f":
		m.movePopup = false
	case "p":
		m.movePopup = true
	case "enter":
	default:
		return m, nil
	}
	m.moveAction.Popup = m.movePopup
	m.action = m.moveAction
	m.moveAction = nil
	m.cancelAutoTitles("")
	m.persistViewPreferences()
	m.quit = true
	return m, tea.Quit
}

// Resume returns to the same list after detaching from an agent popup.
func (m Model) Resume() Model {
	m.action = nil
	m.quit = false
	return m
}

func (m Model) renderMoveMenu() string {
	w := min(58, m.width-2)
	lines := []string{
		"Open agent",
		"  f  Fullscreen",
		"  p  Popup",
		"j/k select | enter open | esc cancel",
	}
	if m.moveAction.Backend == collect.BackendTmux {
		lines[2] = "  p  Project popup (window tabs)"
	}
	selected := 1
	if m.movePopup {
		selected = 2
	}
	lines[selected] = "> " + strings.TrimSpace(lines[selected])
	if m.notice != "" {
		lines = append(lines, m.notice)
	}
	lines = lines[:min(len(lines), m.height-2)]
	for i, line := range lines {
		lines[i] = truncateWidth(line, w-4)
		if i == 0 || i == selected {
			lines[i] = styleTitle.Render(lines[i])
		}
	}
	panel := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
		BorderForeground(cAccent).Padding(0, 1).Width(w).Render(strings.Join(lines, "\n"))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel)
}
