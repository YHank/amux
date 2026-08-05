// Package ui implements the amux terminal dashboard on top of Bubble Tea.
package ui

import (
	"os/exec"
	"sort"
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

// Action is what amux should do after the TUI exits.
type Action struct {
	Kind       string // "attach" | "manage"
	TmuxTarget string // for attach
	Session    string // for attach when not already inside tmux
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
	working int // visible working agents for a project header
}

// Model is the Bubble Tea model for the dashboard.
type Model struct {
	snap    collect.Snapshot
	rows    []row // flattened headers + agents (post-filter)
	sel     int   // index into selectable agent rows
	selRows []int // indices into rows that are selectable (agents)

	filter          string
	filtering       bool
	showDone        bool            // reveal completed, stopped, and dismissed agents
	hidden          int             // count of agents hidden by the current view
	acknowledged    map[string]bool // session-local error acknowledgements by agent ID
	dismissed       map[string]bool // session-local background dismissals by agent ID
	stacked         bool            // render the agent list above the detail pane
	hideDetail      bool            // hide the detail pane so the list fills the viewport
	preferencesPath string
	notesStore      *notes.Store
	suggestingNote  bool   // asking whether to prefill a first OMP note from its tmux title
	suggestedNote   string // OMP task title offered as the note prefill
	suggestedNoteID string // agent that supplied suggestedNote; prevents cross-pane saves after refresh
	naming          bool   // capturing a note for the selected agent
	namingAgentID   string // agent that owns the in-progress note
	noteInput       string
	composing       bool // capturing a message to send into the selected agent's pane
	composeInput    string
	notice          string // brief, self-clearing status line (e.g. "sent to <pane>")

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
		acknowledged:     map[string]bool{},
		dismissed:        map[string]bool{},
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

type killedMsg struct {
	target string
	name   string
	err    error
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
		return killedMsg{target: target, name: name, err: err}
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
		m.pruneAcknowledged()
		m.pruneDismissed()
		m.rebuildRows()
		if m.suggestingNote {
			if a := m.selectedAgent(); a == nil || a.ID != m.suggestedNoteID {
				m.clearNoteSuggestion()
			}
		}
		if m.naming {
			if a := m.selectedAgent(); a == nil || a.ID != m.namingAgentID {
				m.naming = false
				m.namingAgentID = ""
				m.noteInput = ""
			}
		}
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
			return m, nil
		}
		m.removePane(msg.target)
		m.notice = "killed " + msg.name
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func ompNoteSuggestion(a *collect.Agent) string {
	if a == nil || a.Kind != collect.KindInteractive || a.Tool != "omp" {
		return ""
	}
	title := strings.TrimSpace(a.PaneTitle)
	if title == "" {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(title, "π:"))
}

func (m *Model) clearNoteSuggestion() {
	m.suggestingNote = false
	m.suggestedNote = ""
	m.suggestedNoteID = ""
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

	// A first OMP note can be prefilled from its tmux title (for example,
	// "π: fix student-search dropdown"). y accepts the suggestion, n opens a
	// blank editor, and esc cancels the note request.
	if m.suggestingNote {
		a := m.selectedAgent()
		if a == nil || a.ID != m.suggestedNoteID {
			m.clearNoteSuggestion()
			return m, nil
		}
		switch msg.String() {
		case "y", "Y":
			m.noteInput = m.suggestedNote
		case "n", "N":
			m.noteInput = ""
		case "esc":
			m.noteInput = ""
			m.clearNoteSuggestion()
			return m, nil
		default:
			return m, nil
		}
		m.clearNoteSuggestion()
		m.namingAgentID = a.ID
		m.naming = true
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
			if m.namingAgentID != "" {
				m.notesStore.Set(m.namingAgentID, strings.TrimSpace(m.noteInput))
			}
			m.naming = false
			m.namingAgentID = ""
			m.rebuildRows()
		case tea.KeyEsc:
			m.naming = false
			m.namingAgentID = ""
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
	case "J":
		m.moveProject(1)
	case "K":
		m.moveProject(-1)
	case "/":
		m.filtering = true
	case "i":
		if a := m.selectedAgent(); a != nil {
			m.noteInput = m.notesStore.Get(a.ID)
			if m.noteInput == "" {
				if suggestion := ompNoteSuggestion(a); suggestion != "" {
					m.suggestingNote = true
					m.suggestedNote = suggestion
					m.suggestedNoteID = a.ID
					return m, nil
				}
			}
			m.namingAgentID = a.ID
			m.naming = true
		}
	case "a":
		if a := m.selectedAgent(); a != nil && a.Life == collect.LifeError {
			if m.acknowledged == nil {
				m.acknowledged = map[string]bool{}
			}
			m.acknowledged[a.ID] = true
			m.notice = "acknowledged " + a.Name
			m.rebuildRows()
		}
	case "c":
		if a := m.selectedAgent(); a != nil && a.Kind == collect.KindInteractive && a.TmuxTarget != "" {
			m.composing = true
			m.composeInput = ""
		}
	case "x":
		if a := m.selectedAgent(); a != nil {
			if a.Kind == collect.KindInteractive && a.TmuxTarget != "" {
				m.confirmingKill = true
				m.killTarget = a.TmuxTarget
				m.killName = a.Name
			} else if a.Kind == collect.KindBackground {
				if m.dismissed == nil {
					m.dismissed = map[string]bool{}
				}
				m.dismissed[a.ID] = true
				m.notice = "dismissed " + a.Name
				m.rebuildRows()
			}
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
			if action := actionFor(a); action != nil {
				m.action = action
				m.quit = true
				return m, tea.Quit
			}
			m.notice = "no attach or resume action for " + a.Tool
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

func (m *Model) pruneAcknowledged() {
	for id := range m.acknowledged {
		keep := false
		for _, a := range m.snap.Agents {
			if a.ID == id && a.Life == collect.LifeError {
				keep = true
				break
			}
		}
		if !keep {
			delete(m.acknowledged, id)
		}
	}
}

// removePane updates the displayed snapshot immediately after tmux confirms a
// pane was killed. Waiting for the next polling tick leaves a stale row on
// screen even though the action has already succeeded.
func (m *Model) removePane(target string) {
	agents := make([]collect.Agent, 0, len(m.snap.Agents))
	for _, a := range m.snap.Agents {
		if a.Kind == collect.KindInteractive && a.TmuxTarget == target {
			continue
		}
		agents = append(agents, a)
	}
	m.snap.Agents = agents

	projects := make([]collect.Project, 0, len(m.snap.Projects))
	for _, p := range m.snap.Projects {
		projectAgents := make([]collect.Agent, 0, len(p.Agents))
		for _, a := range p.Agents {
			if a.Kind == collect.KindInteractive && a.TmuxTarget == target {
				continue
			}
			projectAgents = append(projectAgents, a)
		}
		if len(projectAgents) == 0 {
			continue
		}
		p.Agents = projectAgents
		projects = append(projects, p)
	}
	m.snap.Projects = projects
	m.rebuildRows()
}

func (m *Model) pruneDismissed() {
	for id := range m.dismissed {
		keep := false
		for _, a := range m.snap.Agents {
			if a.ID == id {
				keep = true
				break
			}
		}
		if !keep {
			delete(m.dismissed, id)
		}
	}
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

// moveProject selects the first visible agent in the adjacent project. Like
// j/k, movement stops at the list boundary rather than wrapping unexpectedly.
func (m *Model) moveProject(d int) {
	if d == 0 || m.sel < 0 || m.sel >= len(m.selRows) {
		return
	}
	current := m.rows[m.selRows[m.sel]].project
	if current == nil {
		return
	}
	for i := m.sel + d; i >= 0 && i < len(m.selRows); i += d {
		candidate := m.rows[m.selRows[i]].project
		if candidate == nil || candidate.Key == current.Key {
			continue
		}
		for i > 0 {
			previous := m.rows[m.selRows[i-1]].project
			if previous == nil || previous.Key != candidate.Key {
				break
			}
			i--
		}
		m.sel = i
		return
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
	if a.Kind == collect.KindBackground && a.Tool == "claude" && a.Live {
		return &Action{Kind: "manage", CWD: a.CWD}
	}
	return nil
}

func hiddenByDefault(l collect.Lifecycle) bool {
	return l == collect.LifeDone || l == collect.LifeStopped
}

type visibleProject struct {
	project *collect.Project
	agents  []*collect.Agent
}

func visibleAgentRank(a *collect.Agent, acknowledged map[string]bool) int {
	if a.Life == collect.LifeError && acknowledged[a.ID] {
		return collect.AttentionRank(collect.LifeStopped)
	}
	return collect.AttentionRank(a.Life)
}

func visibleProjectAttention(agents []*collect.Agent, acknowledged map[string]bool) (rank, count int) {
	rank = collect.AttentionRank(collect.Lifecycle("")) + 1
	for _, a := range agents {
		if a.Life == collect.LifeError && acknowledged[a.ID] {
			continue
		}
		r := visibleAgentRank(a, acknowledged)
		switch {
		case r < rank:
			rank, count = r, 1
		case r == rank:
			count++
		}
	}
	return rank, count
}

func visibleProjectLatest(agents []*collect.Agent, acknowledged map[string]bool, rank int) time.Time {
	var latest time.Time
	for _, a := range agents {
		if visibleAgentRank(a, acknowledged) != rank ||
			(a.Life == collect.LifeError && acknowledged[a.ID]) {
			continue
		}
		if a.UpdatedAt.After(latest) {
			latest = a.UpdatedAt
		}
	}
	return latest
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
	visible := make([]visibleProject, 0, len(m.snap.Projects))
	for i := range m.snap.Projects {
		p := &m.snap.Projects[i]
		matched := make([]*collect.Agent, 0, len(p.Agents))
		for j := range p.Agents {
			a := &p.Agents[j]
			if f != "" && !agentMatches(a, p, m.notesStore.Get(a.ID), f) {
				continue
			}
			// Dismissed background agents never reappear in the current
			// dashboard session. Completed/stopped agents and acknowledged
			// errors otherwise fold away until history is shown.
			if m.dismissed[a.ID] {
				continue
			}
			acknowledgedError := a.Life == collect.LifeError && m.acknowledged[a.ID]
			if (hiddenByDefault(a.Life) || acknowledgedError) && !m.showDone {
				m.hidden++
				continue
			}
			matched = append(matched, a)
		}
		sort.SliceStable(matched, func(i, j int) bool {
			return visibleAgentRank(matched[i], m.acknowledged) <
				visibleAgentRank(matched[j], m.acknowledged)
		})
		if len(matched) > 0 {
			visible = append(visible, visibleProject{project: p, agents: matched})
		}
	}
	sort.SliceStable(visible, func(i, j int) bool {
		ri, ci := visibleProjectAttention(visible[i].agents, m.acknowledged)
		rj, cj := visibleProjectAttention(visible[j].agents, m.acknowledged)
		if ri != rj {
			return ri < rj
		}
		if ci != cj {
			return ci > cj
		}
		li := visibleProjectLatest(visible[i].agents, m.acknowledged, ri)
		lj := visibleProjectLatest(visible[j].agents, m.acknowledged, rj)
		if !li.Equal(lj) {
			return li.After(lj)
		}
		return visible[i].project.Key < visible[j].project.Key
	})
	for _, vp := range visible {
		working := 0
		for _, a := range vp.agents {
			if a.Life == collect.LifeWorking {
				working++
			}
		}
		m.rows = append(m.rows, row{kind: rowHeader, project: vp.project, working: working})
		for _, a := range vp.agents {
			m.selRows = append(m.selRows, len(m.rows))
			m.rows = append(m.rows, row{kind: rowAgent, project: vp.project, agent: a})
		}
	}
	if prevID != "" {
		found := false
		for i, ri := range m.selRows {
			if a := m.rows[ri].agent; a != nil && a.ID == prevID {
				m.sel = i
				found = true
				break
			}
		}
		if !found {
			m.sel = 0
		}
	}
	if m.sel >= len(m.selRows) {
		m.sel = len(m.selRows) - 1
	}
	if m.sel < 0 {
		m.sel = 0
	}
}

func agentMatches(a *collect.Agent, p *collect.Project, note, f string) bool {
	return strings.Contains(strings.ToLower(a.Name), f) ||
		strings.Contains(strings.ToLower(a.Session), f) ||
		strings.Contains(strings.ToLower(a.Window), f) ||
		strings.Contains(strings.ToLower(a.Detail), f) ||
		strings.Contains(strings.ToLower(a.Tool), f) ||
		strings.Contains(strings.ToLower(a.Model), f) ||
		strings.Contains(strings.ToLower(p.Name), f) ||
		strings.Contains(strings.ToLower(a.CWD), f) ||
		strings.Contains(strings.ToLower(note), f)
}
