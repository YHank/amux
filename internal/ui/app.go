// Package ui implements the amux terminal dashboard on top of Bubble Tea.
package ui

import (
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kyhan/amux/internal/collect"
	"github.com/kyhan/amux/internal/notes"
)

// refreshEvery is how often the dashboard re-scans the environment.
const refreshEvery = 2 * time.Second

// notesRefreshEvery is how often the dashboard reloads notes saved by another
// amux instance. Agent discovery needs to be responsive; disk-backed notes do
// not, so they refresh less frequently.
const notesRefreshEvery = 10 * time.Second

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

	stacked         bool // render the agent list above the detail pane
	hideDetail      bool // hide the detail pane so the list fills the viewport
	preferencesPath string
	notesStore      *notes.Store
	naming          bool // capturing a note for the selected agent
	noteInput       string

	composing    bool // capturing a message to send into the selected agent's pane
	composeInput string
	notice       string // brief, self-clearing status line (e.g. "sent to <pane>")

	confirmingKill bool // "kill <pane>? y to confirm" — any other key cancels
	killTarget     string
	killName       string

	width, height    int
	err              error
	loading          bool
	lastRefresh      time.Time
	lastNotesRefresh time.Time

	action *Action // set on quit when the user chose to attach/resume
	quit   bool
}

// New builds an initial model.
func New() Model {
	path := defaultPreferencesPath()
	prefs := loadViewPreferences(path)
	return Model{
		loading:          true,
		notesStore:       notes.Load(),
		lastNotesRefresh: time.Now(),
		preferencesPath:  path,
		stacked:          prefs.Stacked,
		hideDetail:       prefs.HideDetail,
	}
}

func (m Model) persistViewPreferences() {
	saveViewPreferences(m.preferencesPath, viewPreferences{
		Stacked:    m.stacked,
		HideDetail: m.hideDetail,
	})
}

// Chosen returns the action selected before quitting, if any.
func (m Model) Chosen() *Action { return m.action }

// --- messages ---

type tickMsg time.Time
type snapshotMsg struct {
	snap collect.Snapshot
	err  error
}

// sentMsg reports the outcome of injecting a composed message into a pane.
type sentMsg struct {
	target string
	err    error
}

// killedMsg reports the outcome of killing a tmux pane.
type killedMsg struct {
	name string
	err  error
}

func refreshCmd() tea.Msg {
	snap, err := collect.Collect(time.Now())
	return snapshotMsg{snap: snap, err: err}
}

// sendToPane types text into a tmux pane as if the user had typed it, then
// presses Enter — the same two-step tmux send-keys sequence a human would
// trigger by hand. -l sends the text literally (no key-name interpretation),
// so no shell is involved and no injection risk exists regardless of content.
func sendToPane(target, text string) tea.Cmd {
	return func() tea.Msg {
		if err := exec.Command("tmux", "send-keys", "-t", target, "-l", text).Run(); err != nil {
			return sentMsg{target: target, err: err}
		}
		if err := exec.Command("tmux", "send-keys", "-t", target, "Enter").Run(); err != nil {
			return sentMsg{target: target, err: err}
		}
		return sentMsg{target: target}
	}
}

// killPane ends a tmux pane outright (its process along with it) — the same
// effect as mux's own session-delete. Irreversible, so the caller must only
// invoke this after an explicit confirmation.
func killPane(target, name string) tea.Cmd {
	return func() tea.Msg {
		err := exec.Command("tmux", "kill-pane", "-t", target).Run()
		return killedMsg{name: name, err: err}
	}
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
		if time.Time(msg).Sub(m.lastNotesRefresh) >= notesRefreshEvery {
			m.notesStore.Reload()
			m.lastNotesRefresh = time.Time(msg)
		}
		return m, tea.Batch(refreshCmd, tick())

	case snapshotMsg:
		m.loading = false
		m.lastRefresh = time.Now()
		m.notice = "" // let any prior send/error notice expire on its own
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		m.snap = msg.snap
		m.pruneNotes()
		m.rebuildRows()
		return m, nil

	case sentMsg:
		if msg.err != nil {
			m.notice = "send failed: " + msg.err.Error()
		} else {
			m.notice = "sent to " + msg.target
		}
		return m, nil

	case killedMsg:
		if msg.err != nil {
			m.notice = "kill failed: " + msg.err.Error()
		} else {
			m.notice = "killed " + msg.name
		}
		// No forced refresh here: an immediate refreshCmd would race its own
		// snapshotMsg handler, which clears m.notice — the killed/failed
		// notice would flash for under a frame. The regular 2s tick already
		// picks up the pane's disappearance; that small delay is the same
		// trade-off sentMsg already makes.
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Kill confirmation: only y/Y confirms; every other key cancels, no
	// exceptions — a destructive action should never fire on a stray keypress.
	if m.confirmingKill {
		target, name := m.killTarget, m.killName
		m.confirmingKill = false
		m.killTarget = ""
		m.killName = ""
		if s := msg.String(); s == "y" || s == "Y" {
			return m, killPane(target, name)
		}
		return m, nil
	}

	// Compose entry mode captures most keys as text, same shape as filtering.
	if m.composing {
		switch msg.Type {
		case tea.KeyEnter:
			m.composing = false
			text := m.composeInput
			m.composeInput = ""
			a := m.selectedAgent()
			if a == nil || a.TmuxTarget == "" || strings.TrimSpace(text) == "" {
				return m, nil
			}
			return m, sendToPane(a.TmuxTarget, text)
		case tea.KeyEsc:
			m.composing = false
			m.composeInput = ""
		case tea.KeyBackspace:
			m.composeInput = dropLastRune(m.composeInput)
		case tea.KeyRunes, tea.KeySpace:
			m.composeInput += string(msg.Runes)
		}
		return m, nil
	}

	// Note entry mode captures most keys as text, same shape as filtering.
	if m.naming {
		switch msg.Type {
		case tea.KeyEnter:
			if a := m.selectedAgent(); a != nil {
				m.notesStore.Set(a.ID, strings.TrimSpace(m.noteInput))
			}
			m.naming = false
		case tea.KeyEsc:
			m.naming = false
			m.noteInput = ""
		case tea.KeyBackspace:
			m.noteInput = dropLastRune(m.noteInput)
		case tea.KeyRunes, tea.KeySpace:
			m.noteInput += string(msg.Runes)
		}
		return m, nil
	}

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
			m.filter = dropLastRune(m.filter)
			m.rebuildRows()
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
	case "i":
		if a := m.selectedAgent(); a != nil {
			m.naming = true
			m.noteInput = m.notesStore.Get(a.ID)
		}
	case "c":
		if a := m.selectedAgent(); a != nil && a.Kind == collect.KindInteractive && a.TmuxTarget != "" {
			m.composing = true
			m.composeInput = ""
		}
	case "x":
		if a := m.selectedAgent(); a != nil && a.Kind == collect.KindInteractive && a.TmuxTarget != "" {
			m.confirmingKill = true
			m.killTarget = a.TmuxTarget
			m.killName = a.Name
		}
	case "esc":
		if m.filter != "" {
			m.filter = ""
			m.rebuildRows()
		}
	case "r":
		return m, refreshCmd
	case "v":
		m.stacked = !m.stacked
		m.persistViewPreferences()
	case "m":
		m.hideDetail = !m.hideDetail
		m.persistViewPreferences()
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

// pruneNotes drops any stored note whose agent isn't in the current snapshot
// (unfiltered — filtering never removes an agent from m.snap, only from the
// displayed rows), so notes.json doesn't accumulate labels for finished runs.
func (m *Model) pruneNotes() {
	live := make(map[string]bool, len(m.snap.Agents))
	for _, a := range m.snap.Agents {
		live[a.ID] = true
	}
	m.notesStore.Prune(live)
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
