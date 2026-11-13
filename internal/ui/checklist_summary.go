package ui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kyhan/amux/internal/collect"
	"github.com/kyhan/amux/internal/notegen"
)

type checklistSummary struct {
	text       string
	err        error
	request    uint64
	cancel     context.CancelFunc
	startedAt  time.Time
	backend    collect.Backend
	generation uint64
}

type checklistSummaryMsg struct {
	id      string
	request uint64
	text    string
	err     error
}

func (m *Model) cancelChecklistSummaries() {
	for id, entry := range m.checklistSummaries {
		if entry.cancel != nil {
			entry.cancel()
			delete(m.checklistSummaries, id)
		}
	}
}

func (m *Model) pruneChecklistSummaries() {
	for id, entry := range m.checklistSummaries {
		found := false
		for _, a := range m.snap.Agents {
			if a.ID == id && a.Backend == entry.backend && m.generation == entry.generation {
				delta := a.ProcessStartedAt.Sub(entry.startedAt)
				found = delta >= -2*time.Second && delta <= 2*time.Second
				break
			}
		}
		if !found {
			if entry.cancel != nil {
				entry.cancel()
			}
			delete(m.checklistSummaries, id)
		}
	}
	if m.quit {
		m.cancelChecklistSummaries()
	}
}

func (m Model) fetchChecklistSummary(a collect.Agent) (tea.Model, tea.Cmd) {
	if a.ID == "" || a.Tool == "omo" || a.Kind != collect.KindInteractive {
		m.notice = "OmO uses its recorded plan"
		return m, nil
	}
	model := notegen.SelectedModel()
	if m.checklistSummaries == nil {
		m.checklistSummaries = make(map[string]checklistSummary)
	}
	if previous := m.checklistSummaries[a.ID]; previous.cancel != nil {
		previous.cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	m.checklistSummaryRequest++
	request := m.checklistSummaryRequest
	m.checklistSummaries[a.ID] = checklistSummary{request: request, cancel: cancel, startedAt: a.ProcessStartedAt, backend: a.Backend, generation: m.generation}
	m.showChecklist, m.hideDetail = true, false
	m.persistViewPreferences()
	return m, func() tea.Msg {
		defer cancel()
		captured, err := captureNoteAgent(ctx, a)
		var text string
		if err == nil {
			text, err = notegen.Summary(ctx, captured, model)
		}
		return checklistSummaryMsg{id: a.ID, request: request, text: text, err: err}
	}
}

func (m *Model) applyChecklistSummary(msg checklistSummaryMsg) {
	entry, ok := m.checklistSummaries[msg.id]
	if !ok || entry.request != msg.request {
		return
	}
	entry.text, entry.err, entry.cancel = msg.text, msg.err, nil
	m.checklistSummaries[msg.id] = entry
}
