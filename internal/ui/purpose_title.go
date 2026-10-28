package ui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kyhan/amux/internal/collect"
	"github.com/kyhan/amux/internal/notegen"
)

type purposeTitleMsg struct {
	agentID string
	request uint64
	title   string
	err     error
}

func (m Model) fetchPurposeTitle(a collect.Agent) (tea.Model, tea.Cmd) {
	m.clearNoteSuggestion()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	m.fetchingNote = true
	m.noteCancel = cancel
	m.noteRequest++
	m.suggestedNoteID = a.ID
	m.suggestedNote = noteSuggestion(&a)
	request := m.noteRequest
	return m, func() tea.Msg {
		defer cancel()
		title, err := inferPurposeTitle(ctx, a)
		return purposeTitleMsg{agentID: a.ID, request: request, title: title, err: err}
	}
}

func (m Model) applyPurposeTitle(msg purposeTitleMsg) Model {
	if !m.fetchingNote || msg.request != m.noteRequest || msg.agentID != m.suggestedNoteID {
		return m
	}
	a := m.selectedAgent()
	if a == nil || a.ID != msg.agentID {
		m.clearNoteSuggestion()
		return m
	}
	m.noteInput = msg.title
	if msg.err != nil {
		m.noteInput = m.suggestedNote
		m.noteError = "Codex failed: " + msg.err.Error()
	}
	m.clearNoteSuggestion()
	m.naming = true
	m.namingAgentID = msg.agentID
	return m
}

func inferPurposeTitle(ctx context.Context, a collect.Agent) (string, error) {
	return notegen.Title(ctx, a, notegen.SelectedModel())
}
