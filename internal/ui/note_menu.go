package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kyhan/amux/internal/collect"
	"github.com/kyhan/amux/internal/notegen"
)

type noteModelsMsg struct {
	request uint64
	models  []notegen.Model
	err     error
}

func (m Model) loadNoteModels() (tea.Model, tea.Cmd) {
	m.choosingNoteModel = true
	m.modelLoading = true
	m.modelError = ""
	m.modelRequest++
	request := m.modelRequest
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	m.modelCancel = cancel
	return m, func() tea.Msg {
		defer cancel()
		models, err := notegen.Models(ctx)
		return noteModelsMsg{request: request, models: models, err: err}
	}
}

func (m *Model) closeNoteMenu() {
	if m.modelCancel != nil {
		m.modelCancel()
		m.modelCancel = nil
	}
	m.showNoteMenu = false
	m.choosingNoteModel = false
	m.modelLoading = false
}

func (m Model) handleNoteMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.closeNoteMenu()
		return m, nil
	case "q", "ctrl+c":
		m.closeNoteMenu()
		m.persistViewPreferences()
		m.quit = true
		return m, tea.Quit
	}
	if !m.choosingNoteModel {
		switch msg.String() {
		case "o":
			a := m.selectedAgent()
			m.closeNoteMenu()
			if a != nil && a.Kind == collect.KindInteractive {
				return m.fetchAutoTitle(*a)
			}
		case "m":
			return m.loadNoteModels()
		}
		return m, nil
	}
	if m.modelLoading {
		return m, nil
	}
	switch msg.String() {
	case "j", "down":
		m.noteModelIndex = min(m.noteModelIndex+1, max(0, len(m.noteModels)-1))
	case "k", "up":
		m.noteModelIndex = max(0, m.noteModelIndex-1)
	case "r":
		return m.loadNoteModels()
	case "enter":
		if len(m.noteModels) > 0 {
			model := m.noteModels[m.noteModelIndex].Model
			if err := notegen.SelectModel(model); err != nil {
				m.modelError = err.Error()
			} else {
				m.closeNoteMenu()
				m.notice = "Codex model: " + model
			}
		}
	}
	return m, nil
}

func (m Model) renderNoteMenu() string {
	w := min(58, m.width-2)
	inner := w - 4
	model := notegen.SelectedModel()
	if model == "" {
		model = "Codex default"
	}
	lines := []string{"Codex note", "o  generate note", "m  choose model", "model: " + model, "esc close"}
	if m.choosingNoteModel {
		lines = []string{"Codex models"}
		switch {
		case m.modelLoading:
			lines = append(lines, "loading available models...")
		case m.modelError != "":
			lines = append(lines, "failed: "+m.modelError)
		default:
			page := max(1, m.height-5)
			start := max(0, m.noteModelIndex-page+1)
			for i := start; i < min(start+page, len(m.noteModels)); i++ {
				item := m.noteModels[i]
				prefix := "  "
				if i == m.noteModelIndex {
					prefix = "> "
				}
				label := prefix + item.Model
				if item.Model == model || (model == "Codex default" && item.IsDefault) {
					label += " *"
				}
				lines = append(lines, label)
			}
			lines = append(lines, fmt.Sprintf("%d/%d", m.noteModelIndex+1, len(m.noteModels)))
		}
		lines = append(lines, "j/k move · enter save · r reload · esc close")
	}
	lines = lines[:min(len(lines), m.height-2)]
	for i, line := range lines {
		line = truncateWidth(line, inner)
		if i == 0 || strings.HasPrefix(line, "> ") {
			line = styleTitle.Render(line)
		}
		lines[i] = line
	}
	panel := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
		BorderForeground(cAccent).Padding(0, 1).Width(w).Render(strings.Join(lines, "\n"))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel)
}
