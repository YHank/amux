package ui

import (
	"context"
	"os"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kyhan/amux/internal/collect"
)

type launchMenuState struct {
	open      bool
	tools     bool
	busy      bool
	paths     []string
	index     int
	toolIndex int
	input     string
	cursor    int // rune offset, independent of rendered cell width
	error     string
	target    launchTarget
}

func (m Model) openLaunchMenu() (tea.Model, tea.Cmd) {
	history, err := loadLaunchHistory(launchHistoryPath(m.preferencesPath))
	state := launchMenuState{open: true}
	state.target.backend = m.selectedBackend()
	if err != nil {
		state.error = err.Error()
	}
	var selected, seeds []string
	if a := m.selectedAgent(); a != nil {
		selected = []string{a.CWD, a.Project}
		if a.Kind == collect.KindInteractive {
			state.target = launchTarget{backend: m.selectedBackend(), pane: a.TmuxTarget, session: a.Session,
				workspace: a.WorkspaceID, terminal: a.TerminalID, tab: a.TabID}
			if a.Backend == collect.BackendHerdr {
				state.target.pane = a.PaneID
			}
			if tmuxNumericID(a.ID, '%') {
				state.target.pane = a.ID
			}
		}
	}
	if state.target.backend != collect.BackendHerdr && state.target.pane == "" && state.target.session == "" {
		for i, pane := range []string{m.startupPaneID, os.Getenv("AMUX_START_PANE"), os.Getenv("TMUX_PANE")} {
			if tmuxNumericID(pane, '%') {
				state.target = launchTarget{pane: pane, rejectPopup: i == 2}
				break
			}
		}
	}
	for _, project := range m.snap.Projects {
		seeds = append(seeds, project.Key)
	}
	for _, a := range m.snap.Agents {
		seeds = append(seeds, a.CWD, a.Project)
	}
	state.paths = mergeLaunchPaths(selected, history, seeds, []string{m.currentDir})
	if len(state.paths) > 0 {
		state.input = state.paths[0]
		state.cursor = len([]rune(state.input))
	}
	m.launchMenu = state
	return m, nil
}

func (m Model) handleLaunchMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := &m.launchMenu
	if s.busy {
		return m, nil // Keep the pending launch visible; do not submit twice.
	}
	if msg.String() == "esc" || msg.String() == "ctrl+c" {
		if s.tools {
			s.tools = false
		} else {
			s.open = false
		}
		s.error = ""
		return m, nil
	}
	if s.tools {
		switch msg.String() {
		case "up", "k", "shift+tab":
			s.toolIndex = (s.toolIndex + len(launchTools) - 1) % len(launchTools)
		case "down", "j", "tab":
			s.toolIndex = (s.toolIndex + 1) % len(launchTools)
		case "enter":
			s.busy = true
			s.error = ""
			target, path, tool := s.target, s.input, launchTools[s.toolIndex]
			backend, generation := m.selectedBackend(), m.generation
			return m, func() tea.Msg {
				return launchedMsg{path: path, tool: tool, backend: backend, generation: generation,
					err: runLaunch(context.Background(), target, path, tool)}
			}
		}
		return m, nil
	}
	runes := []rune(s.input)
	switch msg.String() {
	case "up", "down", "tab", "shift+tab":
		// The final empty entry is an explicit new-path choice.
		delta := 1
		if msg.String() == "up" || msg.String() == "shift+tab" {
			delta = -1
		}
		s.index = (s.index + delta + len(s.paths) + 1) % (len(s.paths) + 1)
		s.input = ""
		if s.index < len(s.paths) {
			s.input = s.paths[s.index]
		}
		s.cursor = len([]rune(s.input))
		s.error = ""
	case "left":
		s.cursor = max(0, s.cursor-1)
	case "right":
		s.cursor = min(len(runes), s.cursor+1)
	case "home", "ctrl+a":
		s.cursor = 0
	case "end", "ctrl+e":
		s.cursor = len(runes)
	case "ctrl+u":
		s.input, s.cursor = "", 0
	case "backspace", "ctrl+h":
		if s.cursor > 0 {
			s.input = string(runes[:s.cursor-1]) + string(runes[s.cursor:])
			s.cursor--
		}
	case "delete":
		if s.cursor < len(runes) {
			s.input = string(runes[:s.cursor]) + string(runes[s.cursor+1:])
		}
	case " ":
		s.input = string(runes[:s.cursor]) + " " + string(runes[s.cursor:])
		s.cursor++
	case "enter":
		path, err := launchDirectory(s.input, m.currentDir)
		if err != nil {
			s.error = err.Error()
			return m, nil
		}
		s.input, s.cursor = path, len([]rune(path))
		if err := saveLaunchPath(launchHistoryPath(m.preferencesPath), path); err != nil {
			s.error = err.Error()
			return m, nil
		}
		s.tools, s.error = true, ""
	default:
		if msg.Type == tea.KeyRunes && !msg.Alt {
			var text strings.Builder
			for _, r := range msg.Runes {
				if !unicode.IsControl(r) {
					text.WriteRune(r)
				}
			}
			s.input = string(runes[:s.cursor]) + text.String() + string(runes[s.cursor:])
			s.cursor += len([]rune(text.String()))
		}
	}
	return m, nil
}

func launchDisplayText(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, text)
}

func (m Model) renderLaunchMenu() string {
	if m.width < 6 || m.height < 4 {
		return truncateWidth("Launch", max(0, m.width))
	}
	w := min(68, m.width-2)
	inner := w - 4
	s := m.launchMenu
	title, help := "Launch project", "up/down history | edit path | enter tools | esc cancel"
	lines := []string{title}
	if s.tools {
		lines = append(lines, launchDisplayText(s.input))
		help = "up/down choose | enter launch | esc back"
	} else {
		runes := []rune(s.input)
		// Show the edit cursor and its nearby suffix even for long CJK paths.
		input := string(runes[:s.cursor]) + "|" + string(runes[s.cursor:])
		for lipgloss.Width(input) > inner && len([]rune(input)) > 1 && !strings.HasPrefix(input, "|") {
			input = string([]rune(input)[1:])
		}
		lines = append(lines, launchDisplayText(input))
	}
	reserved := 3
	if s.error != "" || s.busy {
		reserved++
	}
	page := max(0, m.height-2-reserved)
	count, selected := len(s.paths)+1, s.index
	if s.tools {
		count, selected = len(launchTools), s.toolIndex
	}
	start := max(0, selected-page+1)
	for i := start; i < min(count, start+page); i++ {
		label := "New directory..."
		if s.tools {
			label = launchTools[i]
		} else if i < len(s.paths) {
			label = launchDisplayText(s.paths[i])
		}
		prefix := "  "
		if i == selected {
			prefix = "> "
		}
		lines = append(lines, prefix+label)
	}
	if s.busy {
		lines = append(lines, "Launching...")
	} else if s.error != "" {
		lines = append(lines, launchDisplayText(s.error))
	}
	lines = append(lines, help)
	lines = lines[:min(len(lines), m.height-2)]
	for i, line := range lines {
		if lipgloss.Width(line) > inner {
			line = truncateWidth(line, inner)
		}
		lines[i] = line
		if i == 0 || strings.HasPrefix(line, "> ") {
			lines[i] = styleTitle.Render(lines[i])
		}
	}
	panel := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
		BorderForeground(cAccent).Padding(0, 1).Width(w).Render(strings.Join(lines, "\n"))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel)
}
