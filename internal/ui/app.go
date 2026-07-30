// Package ui implements the amux terminal dashboard on top of Bubble Tea.
package ui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kyhan/amux/internal/collect"
)

// refreshEvery is how often the dashboard re-scans the environment.
const refreshEvery = 2 * time.Second

// Action is what amux should do after the TUI exits (e.g. attach to a session).
type Action struct {
	Kind       string // "attach" | "resume"
	TmuxTarget string // for attach
	Session    string // for attach when not already inside tmux
	SessionID  string // for resume
	CWD        string
}

// rowKind tags each rendered line so the cursor can skip headers.
type rowKind int

const (
	rowHeader rowKind = iota
	rowAgent
)

type row struct {
	kind    rowKind
	project *collect.Project
	agent   *collect.Agent
}

// Model is the Bubble Tea model for the dashboard.
type Model struct {
	snap    collect.Snapshot
	rows    []row // flattened headers + agents (post-filter)
	sel     int   // index into selectable agent rows
	selRows []int // indices into rows that are selectable (agents)

	filter    string
	filtering bool
	showDone  bool // reveal finished (done/stopped/error) agents
	hidden    int  // count of finished agents hidden by the current view

	width, height int
	err           error
	loading       bool
	lastRefresh   time.Time

	action *Action // set on quit when the user chose to attach/resume
	quit   bool
}

// New builds an initial model.
func New() Model {
	return Model{loading: true}
}

// Chosen returns the action selected before quitting, if any.
func (m Model) Chosen() *Action { return m.action }

// --- messages ---

type tickMsg time.Time
type snapshotMsg struct {
	snap collect.Snapshot
	err  error
}

func refreshCmd() tea.Msg {
	snap, err := collect.Collect(time.Now())
	return snapshotMsg{snap: snap, err: err}
}

func tick() tea.Cmd {
	return tea.Tick(refreshEvery, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// Init kicks off the first scan and the refresh timer.
func (m Model) Init() tea.Cmd {
	return tea.Batch(refreshCmd, tick())
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tickMsg:
		return m, tea.Batch(refreshCmd, tick())

	case snapshotMsg:
		m.loading = false
		m.lastRefresh = time.Now()
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		m.snap = msg.snap
		m.rebuildRows()
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Filter entry mode captures most keys as text.
	if m.filtering {
		switch msg.Type {
		case tea.KeyEnter:
			m.filtering = false
		case tea.KeyEsc:
			m.filtering = false
			m.filter = ""
			m.rebuildRows()
		case tea.KeyBackspace:
			if m.filter != "" {
				m.filter = m.filter[:len(m.filter)-1]
				m.rebuildRows()
			}
		case tea.KeyRunes, tea.KeySpace:
			m.filter += string(msg.Runes)
			m.rebuildRows()
		}
		return m, nil
	}

	switch msg.String() {
	case "q", "ctrl+c":
		m.quit = true
		return m, tea.Quit
	case "j", "down":
		m.moveSel(1)
	case "k", "up":
		m.moveSel(-1)
	case "g", "home":
		m.sel = 0
	case "G", "end":
		if n := len(m.selRows); n > 0 {
			m.sel = n - 1
		}
	case "/":
		m.filtering = true
	case "esc":
		if m.filter != "" {
			m.filter = ""
			m.rebuildRows()
		}
	case "r":
		return m, refreshCmd
	case "d":
		m.showDone = !m.showDone
		m.rebuildRows()
	case "enter":
		if a := m.selectedAgent(); a != nil {
			m.action = actionFor(a)
			m.quit = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *Model) moveSel(d int) {
	n := len(m.selRows)
	if n == 0 {
		return
	}
	m.sel += d
	if m.sel < 0 {
		m.sel = 0
	}
	if m.sel >= n {
		m.sel = n - 1
	}
}

func (m Model) selectedAgent() *collect.Agent {
	if m.sel < 0 || m.sel >= len(m.selRows) {
		return nil
	}
	return m.rows[m.selRows[m.sel]].agent
}

func actionFor(a *collect.Agent) *Action {
	if a.Kind == collect.KindInteractive && a.TmuxTarget != "" {
		return &Action{Kind: "attach", TmuxTarget: a.TmuxTarget, Session: a.Session, CWD: a.CWD}
	}
	return &Action{Kind: "resume", SessionID: a.ID, CWD: a.CWD}
}

// rebuildRows flattens the snapshot into display rows, honoring the filter, and
// clamps the selection.
func (m *Model) rebuildRows() {
	// Remember which agent is selected so the cursor stays on it across a
	// refresh — otherwise an agent finishing (and vanishing) or a new one
	// appearing would shift every row and silently move the selection to a
	// different agent, exactly when the user is watching a transition.
	prevID := ""
	if a := m.selectedAgent(); a != nil {
		prevID = a.ID
	}

	m.rows = m.rows[:0]
	m.selRows = m.selRows[:0]
	m.hidden = 0
	f := strings.ToLower(strings.TrimSpace(m.filter))
	for i := range m.snap.Projects {
		p := &m.snap.Projects[i]
		var matched []*collect.Agent
		for j := range p.Agents {
			a := &p.Agents[j]
			if f != "" && !agentMatches(a, p, f) {
				continue
			}
			// Finished agents are hidden unless explicitly revealed; keep a
			// running count so the view never silently drops them.
			if a.Life.Terminal() && !m.showDone {
				m.hidden++
				continue
			}
			matched = append(matched, a)
		}
		if len(matched) == 0 {
			continue
		}
		m.rows = append(m.rows, row{kind: rowHeader, project: p})
		for _, a := range matched {
			m.selRows = append(m.selRows, len(m.rows))
			m.rows = append(m.rows, row{kind: rowAgent, project: p, agent: a})
		}
	}
	if prevID != "" {
		for i, ri := range m.selRows {
			if a := m.rows[ri].agent; a != nil && a.ID == prevID {
				m.sel = i
				break
			}
		}
	}
	if m.sel >= len(m.selRows) {
		m.sel = len(m.selRows) - 1
	}
	if m.sel < 0 {
		m.sel = 0
	}
}

func agentMatches(a *collect.Agent, p *collect.Project, f string) bool {
	return strings.Contains(strings.ToLower(a.Name), f) ||
		strings.Contains(strings.ToLower(a.Detail), f) ||
		strings.Contains(strings.ToLower(a.Model), f) ||
		strings.Contains(strings.ToLower(p.Name), f) ||
		strings.Contains(strings.ToLower(a.CWD), f)
}
