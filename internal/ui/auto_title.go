package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kyhan/amux/internal/collect"
	"github.com/kyhan/amux/internal/notegen"
)

type autoTitleMsg struct {
	agentID string
	request uint64
	err     error
}

type autoTitleRequest struct {
	agentID string
	token   string
}

func launchNote(request notegen.Request) (func() error, error) {
	cmd, err := notegen.Start(request)
	if err != nil {
		return nil, err
	}
	return cmd.Wait, nil
}

func (m Model) fetchAutoTitle(a collect.Agent) (tea.Model, tea.Cmd) {
	if m.autoTitleRequests == nil {
		m.autoTitleRequests = make(map[uint64]autoTitleRequest)
		m.autoTitleLatest = make(map[string]uint64)
	}
	token, err := m.notesStore.Begin(a.ID)
	if err != nil {
		m.notice = "Codex failed: " + err.Error()
		return m, nil
	}
	launcher := m.noteLauncher
	if launcher == nil {
		launcher = launchNote
	}
	// Start before acknowledging the key, so q cannot outrun process creation.
	wait, err := launcher(notegen.Request{
		StorePath: m.notesStore.Path(), Agent: a, Model: notegen.SelectedModel(), Token: token,
	})
	if err != nil {
		_, saveErr := m.notesStore.Complete(a.ID, token, "", err)
		m.notice = fmt.Sprintf("Codex failed: %v", err)
		if saveErr != nil {
			m.notice += ": " + saveErr.Error()
		}
		return m, nil
	}
	m.noteRequest++
	request := m.noteRequest
	m.autoTitleRequests[request] = autoTitleRequest{agentID: a.ID, token: token}
	m.autoTitleLatest[a.ID] = request
	m.notice = "Codex note started (continues after quit)"
	return m, func() tea.Msg {
		return autoTitleMsg{agentID: a.ID, request: request, err: wait()}
	}
}

func (m Model) applyAutoTitle(msg autoTitleMsg) Model {
	request, ok := m.autoTitleRequests[msg.request]
	if !ok || request.agentID != msg.agentID {
		return m
	}
	delete(m.autoTitleRequests, msg.request)
	if m.autoTitleLatest[msg.agentID] != msg.request {
		return m
	}
	delete(m.autoTitleLatest, msg.agentID)
	state := m.notesStore.Generation(msg.agentID)
	if state.Token != request.token {
		return m
	}
	m.notesStore.Reload()
	switch {
	case state.Error != "":
		m.notice = "Codex failed: " + state.Error
	case msg.err != nil:
		m.notice = "Codex failed: " + msg.err.Error()
	case state.Done:
		m.notice = "Codex note applied to " + msg.agentID
	}
	m.rebuildRows()
	return m
}

func (m *Model) cancelAutoTitles(agentID string) {
	if agentID != "" {
		if err := m.notesStore.Cancel(agentID); err != nil {
			m.notice = "Codex cancellation failed: " + err.Error()
		}
	}
	for id, request := range m.autoTitleRequests {
		if agentID == "" || request.agentID == agentID {
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
