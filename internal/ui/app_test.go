package ui

import (
	"testing"

	"github.com/kyhan/amux/internal/collect"
)

func mkAgent(id string, life collect.Lifecycle) collect.Agent {
	return collect.Agent{ID: id, Kind: collect.KindBackground, Life: life, Project: "/p", Name: id}
}

// TestSelectionAnchorsAcrossRefresh verifies the cursor stays on the same agent
// when an earlier agent finishes and drops out of the default (active) view,
// instead of silently pointing at whatever now occupies the old row index.
func TestSelectionAnchorsAcrossRefresh(t *testing.T) {
	proj := func(agents ...collect.Agent) collect.Snapshot {
		return collect.Snapshot{Projects: []collect.Project{{Key: "/p", Name: "p", Agents: agents}}}
	}

	m := Model{snap: proj(
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
	m.snap = proj(
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
	proj := func(agents ...collect.Agent) collect.Snapshot {
		return collect.Snapshot{Projects: []collect.Project{{Key: "/p", Name: "p", Agents: agents}}}
	}
	m := Model{snap: proj(
		mkAgent("a", collect.LifeWorking),
		mkAgent("b", collect.LifeWorking),
	)}
	m.rebuildRows()
	m.sel = 1 // "b"

	// "b" finishes and is hidden; only "a" remains visible.
	m.snap = proj(
		mkAgent("a", collect.LifeWorking),
		mkAgent("b", collect.LifeDone),
	)
	m.rebuildRows()

	if got := m.selectedAgent(); got == nil || got.ID != "a" {
		t.Errorf("after selected agent vanished, selected %v, want a (clamped)", got)
	}
}
