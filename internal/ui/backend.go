package ui

import (
	"encoding/json"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kyhan/amux/internal/collect"
)

func validBackend(backend collect.Backend) collect.Backend {
	if backend == collect.BackendHerdr {
		return backend
	}
	return collect.BackendTmux
}

// SelectedBackend returns the saved tab for CLI defaults without starting a UI.
func SelectedBackend() collect.Backend {
	return validBackend(loadViewPreferences(defaultPreferencesPath()).Backend)
}

// NewWithBackend overrides the saved tab for an explicit CLI backend option.
func NewWithBackend(backend collect.Backend) Model {
	m := New()
	if backend = validBackend(backend); backend != m.selectedBackend() {
		m.activateBackend(backend)
		if backend == collect.BackendHerdr {
			m.startupPaneID = ""
		} else {
			m.startupPaneID = strings.TrimSpace(os.Getenv("AMUX_START_PANE"))
			if m.startupPaneID == "" {
				m.startupPaneID = strings.TrimSpace(os.Getenv("TMUX_PANE"))
			}
		}
	}
	return m
}

func (m Model) selectedBackend() collect.Backend { return validBackend(m.backend) }

func (m Model) renderBackendTabs() string {
	tmux, herdr := " tmux ", " Herdr "
	if m.selectedBackend() == collect.BackendHerdr {
		herdr = styleTitle.Render("[Herdr]")
	} else {
		tmux = styleTitle.Render("[tmux]")
	}
	tabs := tmux + " " + herdr
	hint := "  Tab/Shift+Tab"
	if lipgloss.Width(tabs+hint) <= m.width {
		tabs += styleFaint.Render(hint)
	}
	return truncateWidth(tabs, m.width)
}

func (m *Model) storeBackendOrder() {
	if m.selectedBackend() == collect.BackendHerdr {
		m.herdrFlatOrder, m.herdrGroupedOrder = m.flatOrder, m.groupedOrder
	} else {
		m.tmuxFlatOrder, m.tmuxGroupedOrder = m.flatOrder, m.groupedOrder
	}
}

func (m *Model) activateBackend(backend collect.Backend) {
	m.storeBackendOrder()
	m.backend = backend
	if backend == collect.BackendHerdr {
		m.flatOrder, m.groupedOrder = m.herdrFlatOrder, m.herdrGroupedOrder
	} else {
		m.flatOrder, m.groupedOrder = m.tmuxFlatOrder, m.tmuxGroupedOrder
	}
	m.refreshRememberedRank()
}

func (m Model) switchBackend() (tea.Model, tea.Cmd) {
	m.clearNoteSuggestion()
	m.closeNoteMenu()
	// Detached note workers retain their captured agent and continue safely.
	m.cancelAutoTitles("")
	backend := collect.BackendHerdr
	if m.selectedBackend() == backend {
		backend = collect.BackendTmux
	}
	m.activateBackend(backend)
	m.generation++
	m.snap = collect.Snapshot{}
	m.rows, m.selRows, m.sel = nil, nil, 0
	m.initialRecencyAt, m.lastRefresh = time.Time{}, time.Time{}
	m.initialCPU, m.previewCandidates, m.lastPreviewFingerprint = nil, nil, nil
	m.initialCPUSeeded = false
	m.acknowledged, m.dismissed = map[string]bool{}, map[string]bool{}
	m.startupPaneID, m.currentProjectKey = "", ""
	m.filter, m.notice, m.noteError = "", "", ""
	m.filtering, m.naming, m.composing, m.confirmingKill, m.showHelp = false, false, false, false, false
	m.namingAgentID, m.noteInput, m.composeInput, m.killTarget, m.killName = "", "", "", "", ""
	m.composeAgent, m.killAgent, m.noteAgent = collect.Agent{}, collect.Agent{}, collect.Agent{}
	m.editMenu, m.launchMenu, m.moveAction, m.action = editMenuState{}, launchMenuState{}, nil, nil
	m.loading, m.err = true, nil
	m.persistViewPreferences()
	return m, m.refreshCommand()
}

func (m Model) acceptBackend(backend collect.Backend, generation uint64) bool {
	return validBackend(backend) == m.selectedBackend() && generation == m.generation
}

func (m Model) refreshCommand() tea.Cmd {
	backend, generation := m.selectedBackend(), m.generation
	return func() tea.Msg {
		snap, err := collect.CollectBackend(time.Now(), backend)
		return snapshotMsg{snap: snap, err: err, backend: backend, generation: generation}
	}
}

func interactiveTarget(a collect.Agent) bool {
	if a.Kind != collect.KindInteractive {
		return false
	}
	if a.Backend == collect.BackendHerdr {
		return a.Session != "" && a.PaneID != ""
	}
	return a.TmuxTarget != ""
}

// Enumerate only this namespace, then mutate individual IDs through Store's
// locked read/modify/write API. Global Prune would erase the inactive backend,
// including notes another dashboard wrote after our inventory was captured.
func (m *Model) pruneBackendNotes(live map[string]bool) {
	if m.notesStore == nil {
		return
	}
	if m.notesStore.Path() == "" {
		return
	}
	for _, path := range []string{m.notesStore.Path(), m.notesStore.Path() + ".requests"} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var entries map[string]json.RawMessage
		if err := json.Unmarshal(data, &entries); err != nil {
			continue
		}
		for id := range entries {
			isHerdr := strings.HasPrefix(id, "herdr:")
			if isHerdr == (m.selectedBackend() == collect.BackendHerdr) && !live[id] {
				m.notesStore.Set(id, "")
			}
		}
	}
}

func (m *Model) removeAgent(id string) {
	for i := len(m.snap.Agents) - 1; i >= 0; i-- {
		if m.snap.Agents[i].ID == id {
			m.snap.Agents = append(m.snap.Agents[:i:i], m.snap.Agents[i+1:]...)
		}
	}
	projects := make([]collect.Project, 0, len(m.snap.Projects))
	for _, p := range m.snap.Projects {
		agents := make([]collect.Agent, 0, len(p.Agents))
		for _, a := range p.Agents {
			if a.ID != id {
				agents = append(agents, a)
			}
		}
		if len(agents) > 0 {
			p.Agents = agents
			projects = append(projects, p)
		}
	}
	m.snap.Projects = projects
	m.rebuildRows()
}
