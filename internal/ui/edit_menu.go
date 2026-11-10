package ui

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kyhan/amux/internal/collect"
)

type editMenuState struct {
	open    bool
	agentID string
	title   bool
	input   string
	target  string
	saving  bool
	err     string
}

type renamedWindowMsg struct {
	target string
	title  string
	err    error
}

func (m Model) beginNoteEdit() (tea.Model, tea.Cmd) {
	a := m.selectedAgent()
	if a == nil {
		return m, nil
	}
	m.cancelAutoTitles(a.ID)
	m.noteError = ""
	m.noteInput = m.notesStore.Get(a.ID)
	if m.noteInput == "" {
		if a.Kind == collect.KindInteractive && utf8.RuneCountInString(noteSuggestion(a)) <= 20 {
			return m.fetchPurposeTitle(*a)
		}
		if suggestion := noteSuggestion(a); suggestion != "" {
			m.suggestingNote = true
			m.suggestedNote = suggestion
			m.suggestedNoteID = a.ID
			return m, nil
		}
	}
	m.namingAgentID = a.ID
	m.naming = true
	return m, nil
}

func (m Model) handleEditMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.editMenu.saving {
		return m, nil
	}
	if msg.Type == tea.KeyEsc {
		m.editMenu = editMenuState{}
		return m, nil
	}
	a := m.selectedAgent()
	if a == nil || a.ID != m.editMenu.agentID {
		m.editMenu = editMenuState{}
		return m, nil
	}
	if !m.editMenu.title {
		switch msg.String() {
		case "i":
			m.editMenu = editMenuState{}
			return m.beginNoteEdit()
		case "t":
			if a.Kind != collect.KindInteractive || a.TmuxTarget == "" {
				m.editMenu.err = "Window titles require a tmux pane"
				return m, nil
			}
			m.editMenu.title = true
			m.editMenu.input = a.Window
			m.editMenu.target = a.TmuxTarget
			if strings.HasPrefix(a.ID, "%") {
				m.editMenu.target = a.ID
			}
		}
		return m, nil
	}
	switch msg.Type {
	case tea.KeyEnter:
		title := strings.TrimSpace(m.editMenu.input)
		if title == "" || strings.ContainsFunc(title, unicode.IsControl) {
			m.editMenu.err = "Enter a non-empty, single-line title"
			return m, nil
		}
		m.editMenu.saving = true
		m.editMenu.err = ""
		return m, renameWindow(m.editMenu.target, title)
	case tea.KeyBackspace:
		m.editMenu.input = dropLastRune(m.editMenu.input)
	case tea.KeyCtrlU:
		m.editMenu.input = ""
	case tea.KeyRunes, tea.KeySpace:
		m.editMenu.input += string(msg.Runes)
	}
	return m, nil
}

func renameWindow(target, title string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "tmux", "rename-window", "-t", target, "--", strings.ReplaceAll(title, "#", "##"))
		cmd.WaitDelay = time.Second
		out, err := cmd.CombinedOutput()
		if err != nil {
			err = fmt.Errorf("rename window: %w: %s", err, strings.TrimSpace(string(out)))
		}
		return renamedWindowMsg{target: target, title: title, err: err}
	}
}

func (m Model) handleRenamedWindow(msg renamedWindowMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.editMenu.saving = false
		m.editMenu.err = msg.err.Error()
		return m, nil
	}
	m.editMenu = editMenuState{}
	m.notice = "window title: " + msg.title
	return m, refreshCmd
}

func (m Model) renderEditPopup() string {
	w := min(76, m.width-2)
	lines := []string{"Edit selected agent", "t  window title", "i  note", "esc cancel"}
	switch {
	case m.editMenu.title:
		lines = []string{"Window title", popupInputTail(m.editMenu.input, w-4), "enter save | ctrl+u clear | esc cancel"}
		if m.editMenu.saving {
			lines[2] = "Saving window title..."
		}
	case m.fetchingNote:
		lines = []string{"Edit note", "Codex: fetching purpose title...", "esc cancel"}
	case m.suggestingNote:
		lines = []string{"Use task title as note?", m.suggestedNote, "y/enter prefill | n blank | o Codex", "esc cancel"}
	case m.naming:
		lines = []string{"Edit note", popupInputTail(m.noteInput, w-4), "enter save | ctrl+u clear | esc cancel"}
	}
	errText := m.editMenu.err
	if m.naming && m.noteError != "" {
		errText = m.noteError
	}
	if errText != "" {
		lines = append(lines[:len(lines)-1], errText, lines[len(lines)-1])
	}
	lines = lines[:min(len(lines), m.height-2)]
	for i, line := range lines {
		if lipgloss.Width(line) > w-4 {
			line = truncateWidth(line, w-4)
		}
		lines[i] = line
		if i == 0 {
			lines[i] = styleTitle.Render(lines[i])
		}
	}
	panel := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
		BorderForeground(cAccent).Padding(0, 1).Width(w).Render(strings.Join(lines, "\n"))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel)
}

func popupInputTail(input string, width int) string {
	runes := []rune(input)
	for len(runes) > 0 && lipgloss.Width(string(runes))+1 > width {
		runes = runes[1:]
	}
	return string(runes) + "▏"
}
