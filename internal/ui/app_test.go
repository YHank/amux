package ui

import (
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

func projSnap(agents ...collect.Agent) collect.Snapshot {
	return collect.Snapshot{Projects: []collect.Project{{Key: "/p", Name: "p", Agents: agents}}}
}

// TestComposeOnlyForInteractive verifies `c` is a no-op on a background agent
// (there's no way to message a headless job from outside `claude agents`) and
// enters compose mode for an interactive one.
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
