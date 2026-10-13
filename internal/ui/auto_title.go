package ui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kyhan/amux/internal/collect"
)

type autoTitleMsg purposeTitleMsg

type autoTitleRequest struct {
	agentID string
	note    string
	cancel  context.CancelFunc
}

func (m Model) fetchAutoTitle(a collect.Agent) (tea.Model, tea.Cmd) {
	if m.autoTitleRequests == nil {
		m.autoTitleRequests = make(map[uint64]autoTitleRequest)
		m.autoTitleLatest = make(map[string]uint64)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	m.noteRequest++
	request := m.noteRequest
	m.autoTitleRequests[request] = autoTitleRequest{
		agentID: a.ID,
		note:    m.notesStore.Get(a.ID),
		cancel:  cancel,
	}
	m.autoTitleLatest[a.ID] = request
	m.notice = ""
	return m, func() tea.Msg {
		defer cancel()
		title, err := inferPurposeTitle(ctx, a)
		return autoTitleMsg{agentID: a.ID, request: request, title: title, err: err}
	}
}

func (m Model) applyAutoTitle(msg autoTitleMsg) Model {
	request, ok := m.autoTitleRequests[msg.request]
	if !ok || request.agentID != msg.agentID {
		return m
	}
	latest := m.autoTitleLatest[msg.agentID] == msg.request
	request.cancel()
	delete(m.autoTitleRequests, msg.request)
	pending := false
	for _, other := range m.autoTitleRequests {
		if other.agentID == msg.agentID {
			pending = true
			break
		}
	}
	if !pending {
		delete(m.autoTitleLatest, msg.agentID)
	}
	if !latest || m.notesStore.Get(msg.agentID) != request.note {
		return m
	}
	if msg.err != nil {
		m.notice = "omp failed for " + msg.agentID + ": " + msg.err.Error()
		return m
	}
	m.notesStore.Set(msg.agentID, msg.title)
	m.notice = "omp note applied to " + msg.agentID
	m.rebuildRows()
	return m
}

// An empty agentID cancels all requests when the dashboard exits.
func (m *Model) cancelAutoTitles(agentID string) {
	for id, request := range m.autoTitleRequests {
		if agentID == "" || request.agentID == agentID {
			request.cancel()
			delete(m.autoTitleRequests, id)
		}
	}
	if agentID == "" {
		clear(m.autoTitleLatest)
	} else {
		delete(m.autoTitleLatest, agentID)
	}
}

func (m *Model) pruneAutoTitles() {
	for _, request := range m.autoTitleRequests {
		found := false
		for _, a := range m.snap.Agents {
			if a.ID == request.agentID {
				found = true
				break
			}
		}
		if !found {
			m.cancelAutoTitles(request.agentID)
		}
	}
}
