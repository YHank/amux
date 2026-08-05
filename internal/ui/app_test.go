package ui

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kyhan/amux/internal/collect"
	"github.com/kyhan/amux/internal/notes"
)

func mkAgent(id string, life collect.Lifecycle) collect.Agent {
	return collect.Agent{ID: id, Kind: collect.KindBackground, Life: life, Project: "/p", Name: id}
}
func TestProjectNavigationMovesBetweenVisibleProjects(t *testing.T) {
	a1 := collect.Agent{ID: "a1", Kind: collect.KindBackground, Life: collect.LifeWorking, Project: "/a", Name: "a1"}
	a2 := collect.Agent{ID: "a2", Kind: collect.KindBackground, Life: collect.LifeWorking, Project: "/a", Name: "a2"}
	b1 := collect.Agent{ID: "b1", Kind: collect.KindBackground, Life: collect.LifeWorking, Project: "/b", Name: "b1"}
	c1 := collect.Agent{ID: "c1", Kind: collect.KindBackground, Life: collect.LifeWorking, Project: "/c", Name: "c1"}
	m := Model{snap: collect.Snapshot{
		Projects: []collect.Project{
			{Key: "/a", Name: "a", Agents: []collect.Agent{a1, a2}},
			{Key: "/b", Name: "b", Agents: []collect.Agent{b1}},
			{Key: "/c", Name: "c", Agents: []collect.Agent{c1}},
		},
		Agents: []collect.Agent{a1, a2, b1, c1},
	}}
	m.rebuildRows()

	for _, step := range []struct {
		key  rune
		want string
	}{
		{'J', "b1"},
		{'J', "c1"},
		{'J', "c1"}, // next-project boundary
		{'K', "b1"},
		{'K', "a1"},
		{'K', "a1"}, // previous-project boundary
	} {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{step.key}})
		m = next.(Model)
		if got := m.selectedAgent(); got == nil || got.ID != step.want {
			t.Fatalf("%c selected %#v, want %q", step.key, got, step.want)
		}
	}

	m.filter = "b"
	m.rebuildRows()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'J'}})
	if got := next.(Model).selectedAgent(); got == nil || got.ID != "b1" {
		t.Fatalf("J crossed filtered project boundary: %#v", got)
	}
}

func TestDismissalUsesVisibleProjectWorkingCount(t *testing.T) {
	dismissed := collect.Agent{
		ID: "dismissed", Kind: collect.KindBackground, Life: collect.LifeWorking,
		Project: "/p", Name: "dismissed",
	}
	kept := collect.Agent{
		ID: "kept", Kind: collect.KindBackground, Life: collect.LifeWorking,
		Project: "/p", Name: "kept",
	}
	m := Model{snap: projSnap(dismissed, kept), width: 80, height: 12}
	m.rebuildRows()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = next.(Model)
	if list := m.renderList(80, 12); !strings.Contains(list, "▶1") || strings.Contains(list, "▶2") {
		t.Errorf("project header did not use visible working count: %q", list)
	}
}

// TestSelectionAnchorsAcrossRefresh verifies the cursor stays on the same agent
// when an earlier agent finishes and drops out of the default (active) view,
// instead of silently pointing at whatever now occupies the old row index.
func TestSelectionAnchorsAcrossRefresh(t *testing.T) {
	m := Model{snap: projSnap(
		mkAgent("a", collect.LifeWorking),
		mkAgent("b", collect.LifeWorking),
		mkAgent("c", collect.LifeWorking),
	)}
	m.rebuildRows()
	m.sel = 1 // select "b"
	if got := m.selectedAgent(); got == nil || got.ID != "b" {
		t.Fatalf("precondition: selected %v, want b", got)
	}

	// "a" finishes and is hidden by default → every row below shifts up.
	m.snap = projSnap(
		mkAgent("a", collect.LifeDone),
		mkAgent("b", collect.LifeWorking),
		mkAgent("c", collect.LifeWorking),
	)
	m.rebuildRows()

	if got := m.selectedAgent(); got == nil || got.ID != "b" {
		t.Errorf("selection drifted to %v after refresh, want b", got)
	}
	if m.hidden != 1 {
		t.Errorf("hidden count = %d, want 1", m.hidden)
	}
}

// TestSelectionClampsWhenSelectedAgentVanishes verifies a sane fallback when the
// very agent under the cursor disappears.
func TestSelectionClampsWhenSelectedAgentVanishes(t *testing.T) {
	m := Model{snap: projSnap(
		mkAgent("a", collect.LifeWorking),
		mkAgent("b", collect.LifeWorking),
	)}
	m.rebuildRows()
	m.sel = 1 // "b"

	// "b" finishes and is hidden; only "a" remains visible.
	m.snap = projSnap(
		mkAgent("a", collect.LifeWorking),
		mkAgent("b", collect.LifeDone),
	)
	m.rebuildRows()

	if got := m.selectedAgent(); got == nil || got.ID != "a" {
		t.Errorf("after selected agent vanished, selected %v, want a (clamped)", got)
	}
}

func TestSelectionResetsWhenMiddleAgentVanishes(t *testing.T) {
	m := Model{snap: projSnap(
		mkAgent("a", collect.LifeWorking),
		mkAgent("b", collect.LifeWorking),
		mkAgent("c", collect.LifeWorking),
	)}
	m.rebuildRows()
	m.sel = 1 // "b"
	m.snap = projSnap(
		mkAgent("a", collect.LifeWorking),
		mkAgent("b", collect.LifeDone),
		mkAgent("c", collect.LifeWorking),
	)
	m.rebuildRows()
	if got := m.selectedAgent(); got == nil || got.ID != "a" {
		t.Errorf("after middle agent vanished, selected %v, want a", got)
	}
}

func TestActionableErrorsStayVisibleByDefault(t *testing.T) {
	m := Model{snap: projSnap(
		mkAgent("error", collect.LifeError),
		mkAgent("done", collect.LifeDone),
		mkAgent("stopped", collect.LifeStopped),
		mkAgent("working", collect.LifeWorking),
	)}
	m.rebuildRows()

	if m.hidden != 2 {
		t.Fatalf("hidden count = %d, want only done and stopped hidden", m.hidden)
	}
	got := make([]string, 0, len(m.selRows))
	for _, ri := range m.selRows {
		got = append(got, m.rows[ri].agent.ID)
	}
	if want := []string{"error", "working"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("visible agents = %v, want %v", got, want)
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = next.(Model)
	if m.hidden != 3 {
		t.Fatalf("acknowledged error left hidden count %d, want 3", m.hidden)
	}
	if got := m.selectedAgent(); got == nil || got.ID != "working" {
		t.Errorf("after acknowledging error selected %v, want working agent", got)
	}
	if got := m.notice; got != "acknowledged error" {
		t.Errorf("acknowledgement notice = %q", got)
	}

	m.showDone = true
	m.rebuildRows()
	if len(m.selRows) != 4 {
		t.Errorf("showDone exposed %d agents, want 4", len(m.selRows))
	}
	if got := m.rows[m.selRows[0]].agent.ID; got != "working" {
		t.Errorf("history mode promoted %q over working agent", got)
	}
}

func TestAcknowledgedErrorNoLongerPrioritizesProject(t *testing.T) {
	now := time.Now()
	m := Model{
		snap: collect.Snapshot{Projects: []collect.Project{
			{
				Key:  "/error",
				Name: "error",
				Agents: []collect.Agent{
					{ID: "error", Life: collect.LifeError, UpdatedAt: now},
					{ID: "working", Life: collect.LifeWorking, UpdatedAt: now},
				},
			},
			{
				Key:    "/stale",
				Name:   "stale",
				Agents: []collect.Agent{{ID: "stale", Life: collect.LifeStale, UpdatedAt: now.Add(-time.Hour)}},
			},
		}},
		acknowledged: map[string]bool{"error": true},
	}

	m.rebuildRows()
	if got := m.rows[0].project.Key; got != "/stale" {
		t.Errorf("acknowledged error project stayed ahead of stale attention: got %q", got)
	}
	m.showDone = true
	m.rebuildRows()
	if got := m.rows[0].project.Key; got != "/stale" {
		t.Errorf("history mode re-promoted acknowledged error project: got %q", got)
	}
}

func TestAcknowledgedErrorDoesNotAffectAttentionRecency(t *testing.T) {
	now := time.Now()
	m := Model{
		snap: collect.Snapshot{Projects: []collect.Project{
			{
				Key:  "/a",
				Name: "a",
				Agents: []collect.Agent{
					{ID: "error", Life: collect.LifeError, UpdatedAt: now.Add(time.Hour)},
					{ID: "working-a", Life: collect.LifeWorking, UpdatedAt: now.Add(-time.Hour)},
				},
			},
			{
				Key:    "/b",
				Name:   "b",
				Agents: []collect.Agent{{ID: "working-b", Life: collect.LifeWorking, UpdatedAt: now}},
			},
		}},
		acknowledged: map[string]bool{"error": true},
		showDone:     true,
	}

	m.rebuildRows()
	if got := m.rows[0].project.Key; got != "/b" {
		t.Errorf("acknowledged error changed project recency order: got %q", got)
	}
}

func TestAcknowledgedErrorsDoNotIncreaseHistoryProjectCount(t *testing.T) {
	m := Model{
		snap: collect.Snapshot{Projects: []collect.Project{
			{
				Key:  "/a",
				Name: "a",
				Agents: []collect.Agent{
					{ID: "stopped-a", Life: collect.LifeStopped},
					{ID: "error-a1", Life: collect.LifeError},
					{ID: "error-a2", Life: collect.LifeError},
				},
			},
			{
				Key:  "/b",
				Name: "b",
				Agents: []collect.Agent{
					{ID: "stopped-b1", Life: collect.LifeStopped},
					{ID: "stopped-b2", Life: collect.LifeStopped},
				},
			},
		}},
		acknowledged: map[string]bool{"error-a1": true, "error-a2": true},
		showDone:     true,
	}

	m.rebuildRows()
	if got := m.rows[0].project.Key; got != "/b" {
		t.Errorf("acknowledged errors inflated history project count: got %q", got)
	}
}

func TestHeaderListsAttentionStatesFirst(t *testing.T) {
	agents := []collect.Agent{
		mkAgent("working", collect.LifeWorking),
		mkAgent("input", collect.LifeInput),
		mkAgent("idle", collect.LifeIdle),
		mkAgent("stale", collect.LifeStale),
		mkAgent("error", collect.LifeError),
		mkAgent("unknown", collect.LifeUnknown),
	}
	m := Model{snap: collect.Snapshot{Agents: agents}, width: 200}
	header := m.renderHeader()

	labels := []string{"input", "error", "stale", "unknown", "working", "idle"}
	last := -1
	for _, label := range labels {
		at := strings.Index(header, label)
		if at <= last {
			t.Fatalf("header attention order is wrong at %q: %q", label, header)
		}
		last = at
	}

	m.acknowledged = map[string]bool{"error": true}
	header = m.renderHeader()
	if strings.Contains(header, "error") {
		t.Errorf("acknowledged error remained in attention header: %q", header)
	}
	if !strings.Contains(header, "history") {
		t.Errorf("acknowledged error was not represented as history: %q", header)
	}
}

func projSnap(agents ...collect.Agent) collect.Snapshot {
	return collect.Snapshot{
		Projects: []collect.Project{{Key: "/p", Name: "p", Agents: append([]collect.Agent(nil), agents...)}},
		Agents:   append([]collect.Agent(nil), agents...),
	}
}

func TestSlashFilterMatchesSavedNotes(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")

	store := notes.Load()
	store.Set("b", "Release blocker 메모")
	m := Model{
		snap:       projSnap(mkAgent("a", collect.LifeWorking), mkAgent("b", collect.LifeIdle)),
		notesStore: store,
	}
	m.rebuildRows()

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("release")})
	m = next.(Model)

	if len(m.selRows) != 1 {
		t.Fatalf("note filter matched %d agents, want 1", len(m.selRows))
	}
	if got := m.selectedAgent(); got == nil || got.ID != "b" {
		t.Errorf("note filter selected %v, want agent b", got)
	}

	// Editing away the only matching note must update the active filter
	// immediately, without waiting for the next agent refresh.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	m = next.(Model)
	m.noteInput = "different note"
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if len(m.selRows) != 0 {
		t.Errorf("edited note left %d stale filter matches, want 0", len(m.selRows))
	}
}

func TestSlashFilterMatchesToolWhenPaneNameDiffers(t *testing.T) {
	openCode := mkAgent("open", collect.LifeWorking)
	openCode.Name = "omp-776820fb:3.0"
	openCode.Tool = "opencode"
	omp := mkAgent("omp", collect.LifeWorking)
	omp.Name = "omp-776820fb:4.0"
	omp.Tool = "omp"

	m := Model{snap: projSnap(openCode, omp)}
	m.rebuildRows()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("opencode")})
	m = next.(Model)

	if len(m.selRows) != 1 {
		t.Fatalf("tool filter matched %d agents, want 1", len(m.selRows))
	}
	if got := m.selectedAgent(); got == nil || got.ID != "open" {
		t.Errorf("tool filter selected %v, want OpenCode agent", got)
	}
	if got := agentRowName(m.selectedAgent()); got != "opencode · omp-776820fb:3.0" {
		t.Errorf("row name = %q, want tool-disambiguated pane name", got)
	}
}

func TestAgentRowNameIncludesTmuxWindow(t *testing.T) {
	interactive := collect.Agent{
		Kind: collect.KindInteractive, Name: "work:2.1", Session: "work", Window: "editor", Tool: "claude",
	}
	if got := agentRowName(&interactive); got != "claude · work » editor" {
		t.Errorf("interactive row name = %q, want %q", got, "claude · work » editor")
	}

	interactive.Window = ""
	if got := agentRowName(&interactive); got != "claude · work:2.1" {
		t.Errorf("window-less interactive row name = %q, want name fallback", got)
	}

	background := collect.Agent{Kind: collect.KindBackground, Name: "job", Session: "work", Window: "editor", Tool: "claude"}
	if got := agentRowName(&background); got != "claude · job" {
		t.Errorf("background row name = %q, want name fallback", got)
	}
}

func TestRenderAgentRowIncludesWindowBeforeNote(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")

	agent := collect.Agent{
		ID: "pane", Kind: collect.KindInteractive, Name: "work:2.1",
		Session: "work", Window: "editor", Tool: "claude",
	}
	store := notes.Load()
	store.Set(agent.ID, "ship it")
	m := Model{notesStore: store}

	row := m.renderAgentRow(&agent, 120, false, time.Now())
	nameAt, noteAt := strings.Index(row, "work » editor"), strings.Index(row, "» ship it")
	if nameAt < 0 || noteAt < 0 || nameAt >= noteAt {
		t.Errorf("row = %q, want window name before note", row)
	}
}

func TestSlashFilterMatchesTmuxWindow(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")

	editor := collect.Agent{
		ID: "editor", Kind: collect.KindInteractive, Life: collect.LifeWorking, Project: "/p",
		Name: "work:2.1", Session: "work", Window: "editor",
	}
	deploy := editor
	deploy.ID = "deploy"
	deploy.Name = "work:3.1"
	deploy.Window = "deploy"
	m := Model{snap: projSnap(editor, deploy), notesStore: notes.Load()}
	m.rebuildRows()

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("editor")})
	m = next.(Model)
	if len(m.selRows) != 1 {
		t.Fatalf("window filter matched %d agents, want 1", len(m.selRows))
	}
	if got := m.selectedAgent(); got == nil || got.ID != editor.ID {
		t.Errorf("window filter selected %v, want editor agent", got)
	}
}

func TestSnapshotRefreshUpdatesTmuxWindowName(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")

	agent := collect.Agent{
		ID: "pane", Kind: collect.KindInteractive, Life: collect.LifeWorking, Project: "/p",
		Name: "work:2.1", Session: "work", Window: "old-window",
	}
	m := Model{snap: projSnap(agent), notesStore: notes.Load(), width: 100, height: 12}
	m.rebuildRows()

	agent.Window = "new-window"
	next, _ := m.Update(snapshotMsg{snap: projSnap(agent)})
	m = next.(Model)
	if list := m.renderList(100, 12); !strings.Contains(list, "work » new-window") {
		t.Errorf("refreshed list = %q, want updated tmux window name", list)
	}
}

// Only a live Claude daemon exposes the native manager action.
func TestBackgroundClaudeActionsRequireLiveClaude(t *testing.T) {
	live := collect.Agent{
		Kind: collect.KindBackground, Tool: "claude", Live: true, CWD: "/project",
	}
	if got := actionFor(&live); got == nil || got.Kind != "manage" {
		t.Fatalf("live Claude action = %#v, want native agent manager", got)
	}

	inactive := live
	inactive.Live = false
	if got := actionFor(&inactive); got != nil {
		t.Errorf("inactive Claude produced action %#v", got)
	}

	unsupported := live
	unsupported.Tool = "omp"
	if got := actionFor(&unsupported); got != nil {
		t.Errorf("unsupported background tool produced action %#v", got)
	}
}

func TestUnsupportedBackgroundEnterDoesNotQuit(t *testing.T) {
	agent := collect.Agent{
		ID: "omp-bg", Kind: collect.KindBackground, Tool: "omp",
		Life: collect.LifeInput, Project: "/p", Name: "omp-bg",
	}
	m := Model{snap: projSnap(agent)}
	m.rebuildRows()

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.quit || m.action != nil || cmd != nil {
		t.Fatalf("unsupported background Enter quit or acted: quit=%v action=%#v cmd=%v", m.quit, m.action, cmd)
	}
	if !strings.Contains(m.notice, "no attach or resume action") {
		t.Errorf("missing unavailable-action notice: %q", m.notice)
	}
}

func TestFooterShowsSelectedAgentCapability(t *testing.T) {
	live := collect.Agent{
		ID: "live", Kind: collect.KindBackground, Tool: "claude", Live: true,
		Life: collect.LifeInput, Project: "/p", Name: "live",
	}
	m := Model{snap: projSnap(live), width: 200}
	m.rebuildRows()
	footer := m.renderFooter()
	if !strings.Contains(footer, "enter agents") || strings.Contains(footer, "c send") {
		t.Errorf("live background footer = %q", footer)
	}
	if detail := m.renderDetail(100, 30); !strings.Contains(detail, "open Claude agents") {
		t.Errorf("live background detail = %q", detail)
	}

}

// TestComposeOnlyForInteractive verifies `c` is a no-op on a background agent
// and enters compose mode for an interactive one.
func TestComposeOnlyForInteractive(t *testing.T) {
	bg := collect.Agent{ID: "b1", Kind: collect.KindBackground, Life: collect.LifeWorking, Project: "/p", Name: "b1"}
	m := Model{snap: projSnap(bg)}
	m.rebuildRows()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if next.(Model).composing {
		t.Error("c should not enter compose mode for a background agent")
	}

	inter := collect.Agent{ID: "i1", Kind: collect.KindInteractive, Life: collect.LifeWorking,
		Project: "/p", Name: "i1", TmuxTarget: "sess:0.0"}
	m = Model{snap: projSnap(inter)}
	m.rebuildRows()
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if !next.(Model).composing {
		t.Fatal("c should enter compose mode for an interactive agent")
	}
}

// TestComposeEnterSendsAndExits verifies typed text accumulates and Enter both
// exits compose mode and returns a non-nil send command.
func TestComposeEnterSendsAndExits(t *testing.T) {
	inter := collect.Agent{ID: "i1", Kind: collect.KindInteractive, Life: collect.LifeWorking,
		Project: "/p", Name: "i1", TmuxTarget: "sess:0.0"}
	m := Model{snap: projSnap(inter)}
	m.rebuildRows()

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	mm := next.(Model)

	next, _ = mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hi")})
	mm = next.(Model)
	if mm.composeInput != "hi" {
		t.Fatalf("composeInput = %q, want %q", mm.composeInput, "hi")
	}

	next, cmd := mm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm = next.(Model)
	if mm.composing {
		t.Error("enter should exit compose mode")
	}
	if mm.composeInput != "" {
		t.Error("enter should clear the compose buffer")
	}
	if cmd == nil {
		t.Error("expected a non-nil send command")
	}
}

// TestComposeEscCancelsWithoutSending verifies Esc discards the draft and
// never returns a send command.
func TestComposeEscCancelsWithoutSending(t *testing.T) {
	inter := collect.Agent{ID: "i1", Kind: collect.KindInteractive, Life: collect.LifeWorking,
		Project: "/p", Name: "i1", TmuxTarget: "sess:0.0"}
	m := Model{snap: projSnap(inter)}
	m.rebuildRows()

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("nope")})
	next, cmd := next.(Model).Update(tea.KeyMsg{Type: tea.KeyEsc})
	mm := next.(Model)
	if mm.composing || mm.composeInput != "" {
		t.Errorf("esc should clear compose state, got composing=%v input=%q", mm.composing, mm.composeInput)
	}
	if cmd != nil {
		t.Error("esc should never return a send command")
	}
}

// TestComposeEnterWithEmptyTextDoesNotSend verifies pressing enter on a blank
// draft exits compose mode without triggering a send.
func TestComposeEnterWithEmptyTextDoesNotSend(t *testing.T) {
	inter := collect.Agent{ID: "i1", Kind: collect.KindInteractive, Life: collect.LifeWorking,
		Project: "/p", Name: "i1", TmuxTarget: "sess:0.0"}
	m := Model{snap: projSnap(inter)}
	m.rebuildRows()

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	next, cmd := next.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Error("enter with an empty draft should not send")
	}
	if next.(Model).composing {
		t.Error("enter should still exit compose mode with an empty draft")
	}
}

func TestFirstInteractiveOMPNoteOffersTitleAsPrefill(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")

	omp := collect.Agent{
		ID: "omp-pane", Kind: collect.KindInteractive, Tool: "omp",
		Life: collect.LifeWorking, Project: "/p", Name: "omp-pane",
		PaneTitle: "Handle dynamic OMP titles correctly",
	}
	m := Model{snap: projSnap(omp), notesStore: notes.Load(), width: 200}
	m.rebuildRows()

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	m = next.(Model)
	if !m.suggestingNote || m.naming {
		t.Fatalf("first OMP note should ask before editing: suggesting=%v naming=%v", m.suggestingNote, m.naming)
	}
	if !strings.Contains(m.renderFooter(), "Handle dynamic OMP titles correctly") {
		t.Errorf("suggestion prompt did not show OMP task: %q", m.renderFooter())
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = next.(Model)
	if m.suggestingNote || !m.naming {
		t.Fatalf("y should open the note editor: suggesting=%v naming=%v", m.suggestingNote, m.naming)
	}
	if got, want := m.noteInput, "Handle dynamic OMP titles correctly"; got != want {
		t.Errorf("suggested note = %q, want %q", got, want)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got, want := next.(Model).notesStore.Get("omp-pane"), "Handle dynamic OMP titles correctly"; got != want {
		t.Errorf("saved suggested note = %q, want %q", got, want)
	}
}

func TestEnterAcceptsOMPTitleSuggestion(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")

	omp := collect.Agent{
		ID: "omp-pane", Kind: collect.KindInteractive, Tool: "omp",
		Life: collect.LifeWorking, Project: "/p", Name: "omp-pane", PaneTitle: "dynamic title",
	}
	m := Model{snap: projSnap(omp), notesStore: notes.Load()}
	m.rebuildRows()

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.suggestingNote || !m.naming {
		t.Fatalf("enter should accept the suggestion: suggesting=%v naming=%v", m.suggestingNote, m.naming)
	}
	if got, want := m.noteInput, "dynamic title"; got != want {
		t.Errorf("enter-prefilled note = %q, want %q", got, want)
	}
}

func TestOMPSuggestedNoteEscCancels(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")

	omp := collect.Agent{
		ID: "omp-pane", Kind: collect.KindInteractive, Tool: "omp",
		Life: collect.LifeWorking, Project: "/p", Name: "omp-pane", PaneTitle: "task",
	}
	m := Model{snap: projSnap(omp), notesStore: notes.Load()}
	m.rebuildRows()

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	next, cmd := next.(Model).Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.suggestingNote || m.naming || m.noteInput != "" {
		t.Fatalf("esc should cancel the suggestion: suggesting=%v naming=%v note=%q", m.suggestingNote, m.naming, m.noteInput)
	}
	if cmd != nil || m.notesStore.Get("omp-pane") != "" {
		t.Fatal("esc should not save or send anything")
	}
}

func TestOMPSuggestedNoteIgnoresOtherKeysAndCancelsAfterPaneRefresh(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")

	omp := collect.Agent{
		ID: "omp-pane", Kind: collect.KindInteractive, Tool: "omp",
		Life: collect.LifeWorking, Project: "/p", Name: "omp-pane", PaneTitle: "task",
	}
	other := collect.Agent{
		ID: "other-pane", Kind: collect.KindInteractive, Tool: "claude",
		Life: collect.LifeWorking, Project: "/p", Name: "other-pane",
	}
	m := Model{snap: projSnap(omp, other), notesStore: notes.Load()}
	m.rebuildRows()

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = next.(Model)
	if !m.suggestingNote || m.naming {
		t.Fatalf("unrelated key should leave the suggestion pending: suggesting=%v naming=%v", m.suggestingNote, m.naming)
	}

	next, _ = m.Update(snapshotMsg{snap: projSnap(other)})
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = next.(Model)
	if m.suggestingNote || m.naming || m.noteInput != "" {
		t.Fatalf("refresh without the source pane should cancel the suggestion: suggesting=%v naming=%v note=%q", m.suggestingNote, m.naming, m.noteInput)
	}
	if m.notesStore.Get("other-pane") != "" {
		t.Fatal("vanished OMP pane's suggestion must not be saved on another pane")
	}
}

func TestAcceptedOMPSuggestedNoteCancelsIfSourcePaneDisappears(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")

	omp := collect.Agent{
		ID: "omp-pane", Kind: collect.KindInteractive, Tool: "omp",
		Life: collect.LifeWorking, Project: "/p", Name: "omp-pane", PaneTitle: "task",
	}
	other := collect.Agent{
		ID: "other-pane", Kind: collect.KindInteractive, Tool: "claude",
		Life: collect.LifeWorking, Project: "/p", Name: "other-pane",
	}
	m := Model{snap: projSnap(omp, other), notesStore: notes.Load()}
	m.rebuildRows()

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = next.(Model)
	if !m.naming || m.namingAgentID != "omp-pane" {
		t.Fatalf("accepted suggestion should remain owned by its source: naming=%v owner=%q", m.naming, m.namingAgentID)
	}

	next, _ = m.Update(snapshotMsg{snap: projSnap(other)})
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.naming || m.namingAgentID != "" || m.noteInput != "" {
		t.Fatalf("refresh without the source pane should cancel note editing: naming=%v owner=%q note=%q", m.naming, m.namingAgentID, m.noteInput)
	}
	if m.notesStore.Get("other-pane") != "" {
		t.Fatal("accepted OMP title must not be saved on another pane")
	}
}

func TestOMPSuggestedNoteCanBeDeclinedAndDoesNotRepeatForExistingNote(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")

	omp := collect.Agent{
		ID: "omp-pane", Kind: collect.KindInteractive, Tool: "omp",
		Life: collect.LifeWorking, Project: "/p", Name: "omp-pane", PaneTitle: "task",
	}
	m := Model{snap: projSnap(omp), notesStore: notes.Load()}
	m.rebuildRows()

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = next.(Model)
	if m.suggestingNote || !m.naming || m.noteInput != "" {
		t.Fatalf("declining should open a blank editor: %#v", m)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("manual note")})
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	m = next.(Model)
	if m.suggestingNote || !m.naming {
		t.Fatalf("existing OMP note should open directly: suggesting=%v naming=%v", m.suggestingNote, m.naming)
	}
	if got, want := m.noteInput, "manual note"; got != want {
		t.Errorf("existing note = %q, want %q", got, want)
	}
}

// TestXNoLongerQuits guards the exact regression reported: x used to be an
// alias for quit, which collided with its intended meaning (kill the
// selected agent, mux-style). Only q/ctrl+c may quit.
func TestXNoLongerQuits(t *testing.T) {
	m := Model{snap: projSnap(mkAgent("a", collect.LifeWorking))}
	m.rebuildRows()
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if next.(Model).quit || cmd != nil {
		t.Error("x must not quit the app")
	}
}

// TestKillOnlyForInteractive verifies x is a no-op on a background agent —
// there's no verified-safe way to kill a headless job from outside amux —
// and arms the confirmation prompt for an interactive one without killing yet.
func TestKillOnlyForInteractive(t *testing.T) {
	bg := collect.Agent{ID: "b1", Kind: collect.KindBackground, Life: collect.LifeWorking, Project: "/p", Name: "b1"}
	m := Model{snap: projSnap(bg)}
	m.rebuildRows()
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if next.(Model).confirmingKill || cmd != nil {
		t.Error("x should not arm a kill confirmation for a background agent")
	}

	inter := collect.Agent{ID: "i1", Kind: collect.KindInteractive, Life: collect.LifeWorking,
		Project: "/p", Name: "i1", TmuxTarget: "sess:0.0"}
	m = Model{snap: projSnap(inter)}
	m.rebuildRows()
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	mm := next.(Model)
	if !mm.confirmingKill || mm.killTarget != "sess:0.0" {
		t.Fatal("x should arm a kill confirmation for an interactive agent")
	}
	if cmd != nil {
		t.Error("arming the confirmation must not kill anything yet")
	}
}

func TestHeadlessXDismissesLocallyEvenInHistoryMode(t *testing.T) {
	bg := collect.Agent{
		ID: "headless", Kind: collect.KindBackground, Tool: "omp",
		Life: collect.LifeWorking, Project: "/p", Name: "headless",
	}
	m := Model{snap: projSnap(bg), width: 200}
	m.rebuildRows()
	if footer := m.renderFooter(); !strings.Contains(footer, "x dismiss") {
		t.Fatalf("background footer missing dismiss shortcut: %q", footer)
	}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = next.(Model)
	if cmd != nil || !m.dismissed["headless"] {
		t.Fatalf("headless x did not dismiss locally: cmd=%v dismissed=%v", cmd, m.dismissed)
	}
	if len(m.selRows) != 0 || !strings.Contains(m.notice, "dismissed headless") {
		t.Fatalf("dismissed background remains visible: rows=%d notice=%q", len(m.selRows), m.notice)
	}
	if header := m.renderHeader(); strings.Contains(header, "1 working") || strings.Contains(header, "history") {
		t.Errorf("dismissed background remains in dashboard summary: %q", header)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = next.(Model)
	if len(m.selRows) != 0 {
		t.Fatalf("history mode restored dismissed agent: %#v", m.selectedAgent())
	}

	stopped := bg
	stopped.ID = "stopped"
	stopped.Life = collect.LifeStopped
	m = Model{snap: projSnap(stopped), showDone: true}
	m.rebuildRows()
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if got := next.(Model); len(got.selRows) != 0 {
		t.Fatalf("dismissed stopped agent remained visible in history: %#v", got.selectedAgent())
	}
}

func TestDismissedStoppedAgentDoesNotDoubleCountHistory(t *testing.T) {
	stopped := collect.Agent{
		ID: "stopped", Kind: collect.KindBackground, Life: collect.LifeStopped,
		Project: "/p", Name: "stopped",
	}
	m := Model{
		snap:      projSnap(stopped),
		dismissed: map[string]bool{"stopped": true},
	}
	counts, history := m.attentionCounts()
	if counts[collect.LifeStopped] != 0 || history != 0 {
		t.Fatalf("dismissed stopped counts = %#v, history=%d; want stopped=0 history=0", counts, history)
	}
}

// TestKillConfirmRequiresY verifies only y/Y actually kills; every other key,
// including the agent's own selection keys, cancels without killing.
func TestKillConfirmRequiresY(t *testing.T) {
	inter := collect.Agent{ID: "i1", Kind: collect.KindInteractive, Life: collect.LifeWorking,
		Project: "/p", Name: "i1", TmuxTarget: "sess:0.0"}

	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyEsc},
		{Type: tea.KeyEnter},
		{Type: tea.KeyRunes, Runes: []rune{'n'}},
		{Type: tea.KeyRunes, Runes: []rune{'j'}},
	} {
		m := Model{snap: projSnap(inter)}
		m.rebuildRows()
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
		next, cmd := next.(Model).Update(key)
		mm := next.(Model)
		if mm.confirmingKill {
			t.Errorf("key %v should clear the confirmation state", key)
		}
		if cmd != nil {
			t.Errorf("key %v must not trigger a kill", key)
		}
	}

	m := Model{snap: projSnap(inter)}
	m.rebuildRows()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	next, cmd := next.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if next.(Model).confirmingKill {
		t.Error("y should clear the confirmation state")
	}
	if cmd == nil {
		t.Error("y should trigger the kill command")
	}
}

func TestSuccessfulKillRemovesPaneImmediately(t *testing.T) {
	killed := collect.Agent{
		ID: "killed", Kind: collect.KindInteractive, Life: collect.LifeWorking,
		Project: "/p", Name: "killed", TmuxTarget: "sess:0.0",
	}
	kept := collect.Agent{
		ID: "kept", Kind: collect.KindInteractive, Life: collect.LifeWorking,
		Project: "/p", Name: "kept", TmuxTarget: "sess:0.1",
	}
	m := Model{snap: projSnap(killed, kept)}
	m.rebuildRows()

	next, cmd := m.Update(killedMsg{target: "sess:0.0", name: "killed"})
	m = next.(Model)
	if cmd != nil {
		t.Error("successful kill should update the existing snapshot, not wait for a refresh command")
	}
	if len(m.snap.Agents) != 1 || m.snap.Agents[0].ID != "kept" {
		t.Fatalf("killed pane remained in snapshot: %#v", m.snap.Agents)
	}
	if got := m.selectedAgent(); got == nil || got.ID != "kept" {
		t.Fatalf("selected agent after kill = %#v, want kept pane", got)
	}
	if !strings.Contains(m.notice, "killed killed") {
		t.Errorf("kill notice = %q", m.notice)
	}
}

func TestFailedKillKeepsPaneVisible(t *testing.T) {
	agent := collect.Agent{
		ID: "pane", Kind: collect.KindInteractive, Life: collect.LifeWorking,
		Project: "/p", Name: "pane", TmuxTarget: "sess:0.0",
	}
	m := Model{snap: projSnap(agent)}
	m.rebuildRows()

	next, _ := m.Update(killedMsg{target: "sess:0.0", name: "pane", err: errors.New("tmux unavailable")})
	m = next.(Model)
	if len(m.snap.Agents) != 1 {
		t.Fatal("failed kill removed a still-live pane")
	}
	if !strings.Contains(m.notice, "kill failed") {
		t.Errorf("failure notice = %q", m.notice)
	}
}

func TestViewTogglesPersistState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	m := Model{preferencesPath: path}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	m = next.(Model)
	if !m.stacked {
		t.Fatal("v should enable stacked layout")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = next.(Model)
	if !m.hideDetail {
		t.Fatal("m should hide the detail pane")
	}
	if got, want := loadViewPreferences(path), (viewPreferences{Stacked: true, HideDetail: true}); got != want {
		t.Fatalf("saved preferences = %#v, want %#v", got, want)
	}
}

func TestSideBySideViewFitsTerminalWidth(t *testing.T) {
	m := Model{
		snap:   projSnap(mkAgent("a", collect.LifeWorking)),
		width:  80,
		height: 24,
	}
	m.rebuildRows()
	for _, line := range strings.Split(m.View(), "\n") {
		if got := lipgloss.Width(line); got > m.width {
			t.Fatalf("side-by-side line width = %d, want at most %d: %q", got, m.width, line)
		}
	}
}

func TestDashboardReloadsSharedNotesEveryTenSeconds(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")

	m := New()
	otherDashboard := notes.Load()
	otherDashboard.Set("agent-1", "shared note")

	start := time.Now()
	m.lastNotesRefresh = start
	next, _ := m.Update(tickMsg(start.Add(notesRefreshEvery - time.Nanosecond)))
	m = next.(Model)
	if got := m.notesStore.Get("agent-1"); got != "" {
		t.Fatalf("note reloaded before interval: %q", got)
	}

	next, _ = m.Update(tickMsg(start.Add(notesRefreshEvery)))
	m = next.(Model)
	if got := m.notesStore.Get("agent-1"); got != "shared note" {
		t.Errorf("reloaded note = %q, want shared note", got)
	}
}
