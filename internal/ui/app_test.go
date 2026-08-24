package ui

import (
	"errors"
	"path/filepath"
	"slices"
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

func TestProjectViewToggleFlattensAndPrefixesAgentNames(t *testing.T) {
	a := collect.Agent{ID: "a", Kind: collect.KindBackground, Life: collect.LifeWorking, Project: "/alpha", Name: "worker"}
	b := collect.Agent{ID: "b", Kind: collect.KindBackground, Life: collect.LifeIdle, Project: "/beta", Name: "reviewer"}
	m := Model{
		snap: collect.Snapshot{
			Projects: []collect.Project{
				{Key: "/alpha", Name: "alpha", Agents: []collect.Agent{a}},
				{Key: "/beta", Name: "beta", Agents: []collect.Agent{b}},
			},
			Agents: []collect.Agent{a, b},
		},
		width: 120,
	}
	m.rebuildRows()
	if len(m.rows) != 4 || m.rows[0].kind != rowHeader {
		t.Fatalf("grouped rows = %#v, want project headers and agents", m.rows)
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	m = next.(Model)
	if !m.flatView {
		t.Fatal("p did not enable flat view")
	}
	for _, r := range m.rows {
		if r.kind == rowHeader {
			t.Fatalf("flat view retained project header: %#v", r)
		}
	}
	list := m.renderList(120, 12)
	for _, name := range []string{"alpha · worker", "beta  · reviewer"} {
		if !strings.Contains(list, name) {
			t.Errorf("flat list = %q, missing aligned project-prefixed name %q", list, name)
		}
	}
	if m.flatProjectWidth != lipgloss.Width("alpha") {
		t.Errorf("flat project width = %d, want %d", m.flatProjectWidth, lipgloss.Width("alpha"))
	}
	if strings.Contains(m.renderFooter(), "J/K project") {
		t.Errorf("flat footer still advertises project navigation: %q", m.renderFooter())
	}

	m.sel = 1
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'J'}})
	if got := next.(Model).sel; got != 1 {
		t.Errorf("J moved selection in flat view to %d, want 1", got)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	m = next.(Model)
	if m.flatView || len(m.rows) != 4 || m.rows[0].kind != rowHeader {
		t.Errorf("second p did not restore grouped view: flat=%v rows=%#v", m.flatView, m.rows)
	}
}

func TestFlatViewOrdersByStatusChangeRecency(t *testing.T) {
	now := time.Date(2026, time.August, 19, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name             string
		doneChangedAt    time.Time
		workingChangedAt time.Time
		wantFirstAgentID string
	}{
		{
			name:             "newer working agent",
			doneChangedAt:    now.Add(-time.Minute),
			workingChangedAt: now,
			wantFirstAgentID: "working",
		},
		{
			name:             "newer completed agent",
			doneChangedAt:    now,
			workingChangedAt: now.Add(-time.Minute),
			wantFirstAgentID: "done",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			done := collect.Agent{
				ID: "done", Kind: collect.KindBackground, Life: collect.LifeDone,
				Project: "/alpha", Name: "done", StatusChangedAt: tc.doneChangedAt,
			}
			working := collect.Agent{
				ID: "working", Kind: collect.KindBackground, Life: collect.LifeWorking,
				Project: "/beta", Name: "working", StatusChangedAt: tc.workingChangedAt,
			}
			m := Model{
				snap: collect.Snapshot{
					Projects: []collect.Project{
						{Key: "/alpha", Name: "alpha", Agents: []collect.Agent{done}},
						{Key: "/beta", Name: "beta", Agents: []collect.Agent{working}},
					},
					Agents: []collect.Agent{done, working},
				},
				showDone: true,
			}
			m.rebuildRows()
			next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
			m = next.(Model)

			if got := m.rows[m.selRows[0]].agent; got == nil || got.ID != tc.wantFirstAgentID {
				t.Errorf("first flat agent = %#v, want %q", got, tc.wantFirstAgentID)
			}
		})
	}
}

func TestFlatViewUsesProcessStartWhenStatusChangesTie(t *testing.T) {
	now := time.Date(2026, time.August, 19, 12, 0, 0, 0, time.UTC)
	older := collect.Agent{
		ID: "older", Kind: collect.KindInteractive, Life: collect.LifeWorking,
		Project: "/alpha", Name: "older", StatusChangedAt: now, ProcessStartedAt: now.Add(-time.Hour),
	}
	newer := collect.Agent{
		ID: "newer", Kind: collect.KindInteractive, Life: collect.LifeWorking,
		Project: "/beta", Name: "newer", StatusChangedAt: now, ProcessStartedAt: now.Add(-time.Minute),
	}
	m := Model{
		snap: collect.Snapshot{
			Projects: []collect.Project{
				{Key: "/alpha", Name: "alpha", Agents: []collect.Agent{older}},
				{Key: "/beta", Name: "beta", Agents: []collect.Agent{newer}},
			},
			Agents: []collect.Agent{older, newer},
		},
		flatView: true,
	}
	m.rebuildRows()
	if got := m.rows[m.selRows[0]].agent; got == nil || got.ID != "newer" {
		t.Errorf("first flat agent = %#v, want newer activity", got)
	}
}

func TestInitialFlatOrderingUsesFrozenCPU(t *testing.T) {
	now := time.Date(2026, time.August, 20, 9, 0, 0, 0, time.UTC)
	low := collect.Agent{ID: "low", Kind: collect.KindInteractive, PID: 101, Life: collect.LifeWorking, CPU: 5}
	high := collect.Agent{ID: "high", Kind: collect.KindInteractive, PID: 102, Life: collect.LifeWorking, CPU: 20}
	m := Model{}
	next := projSnap(low, high)
	next.Taken = now
	m.recordStatusChangeTimes(&next)
	m.snap = next

	next = projSnap(low, high)
	next.Taken = now.Add(time.Minute)
	m.recordStatusChangeTimes(&next)
	m.snap = next
	m.flatView = true
	m.rebuildRows()
	if !m.initialCPUSeeded {
		t.Fatal("second snapshot did not freeze initial CPU ordering")
	}
	if got := m.rows[m.selRows[0]].agent; got == nil || got.ID != "high" {
		t.Errorf("first seeded flat agent = %#v, want higher frozen CPU", got)
	}

	next = projSnap(
		collect.Agent{ID: "low", Kind: collect.KindInteractive, PID: 101, Life: collect.LifeWorking, CPU: 30},
		collect.Agent{ID: "high", Kind: collect.KindInteractive, PID: 102, Life: collect.LifeWorking, CPU: 1},
	)
	next.Taken = now.Add(2 * time.Minute)
	m.recordStatusChangeTimes(&next)
	m.snap = next
	m.rebuildRows()
	if got := m.rows[m.selRows[0]].agent; got == nil || got.ID != "high" {
		t.Errorf("first refreshed flat agent = %#v, want frozen initial CPU order", got)
	}
}

func TestKeyboardHelpPopupAndCompactFooter(t *testing.T) {
	m := Model{width: 100, height: 30}
	footer := m.renderFooter()

	for _, key := range []string{"j/k move", "? help", "q quit"} {
		if !strings.Contains(footer, key) {
			t.Errorf("compact footer = %q, missing %q", footer, key)
		}
	}
	if strings.Contains(footer, "d history") || strings.Contains(footer, "v layout") {
		t.Errorf("compact footer retained extended shortcuts: %q", footer)
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = next.(Model)
	if !m.showHelp {
		t.Fatal("? did not open keyboard help")
	}
	help := m.View()
	for _, text := range []string{"Keyboard shortcuts", "p", "toggle project-grouped and flat views", "j/k scroll · ? / esc close"} {
		if !strings.Contains(help, text) {
			t.Errorf("keyboard help = %q, missing %q", help, text)
		}
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if next.(Model).showHelp {
		t.Fatal("esc did not close keyboard help")
	}
}
func TestFlatViewUsesDeepestLaunchProjectAsTieBreaker(t *testing.T) {
	now := time.Date(2026, time.August, 20, 9, 0, 0, 0, time.UTC)
	parent := collect.Agent{ID: "parent", Life: collect.LifeWorking, StatusChangedAt: now}
	current := collect.Agent{ID: "current", Life: collect.LifeWorking, StatusChangedAt: now}
	m := Model{
		currentDir: "/workspace/current/nested/subdir",
		flatView:   true,
		snap: collect.Snapshot{
			Projects: []collect.Project{
				{Key: "/workspace/current", Name: "parent", Agents: []collect.Agent{parent}},
				{Key: "/workspace/current/nested", Name: "nested", Agents: []collect.Agent{current}},
			},
			Agents: []collect.Agent{parent, current},
		},
	}
	m.rebuildRows()
	if got := m.rows[m.selRows[0]].agent; got == nil || got.ID != "current" {
		t.Errorf("first flat agent = %#v, want deepest launch-project agent", got)
	}
}

func TestFlatViewPrioritizesRecencyBeforeLaunchProject(t *testing.T) {
	now := time.Date(2026, time.August, 20, 9, 0, 0, 0, time.UTC)
	newer := collect.Agent{ID: "newer", Life: collect.LifeWorking, StatusChangedAt: now.Add(time.Minute)}
	current := collect.Agent{ID: "current", Life: collect.LifeWorking, StatusChangedAt: now}
	m := Model{
		currentDir: "/workspace/current",
		flatView:   true,
		snap: collect.Snapshot{
			Projects: []collect.Project{
				{Key: "/workspace/other", Name: "other", Agents: []collect.Agent{newer}},
				{Key: "/workspace/current", Name: "current", Agents: []collect.Agent{current}},
			},
			Agents: []collect.Agent{newer, current},
		},
	}
	m.rebuildRows()
	if got := m.rows[m.selRows[0]].agent; got == nil || got.ID != "newer" {
		t.Errorf("first flat agent = %#v, want more recent agent", got)
	}
}

// A busy agent restamps its activity time on nearly every refresh, so an
// ordering that ranked activity alongside status changes let ordinary churn
// outrank the transition the user is actually watching for. Status wins.
func TestFlatViewStatusChangeOutranksActivity(t *testing.T) {
	now := time.Date(2026, time.August, 20, 9, 0, 0, 0, time.UTC)
	// busy also outranks transitioned on attention, so only the status time can
	// put transitioned first — the flat view is explicitly a recency list.
	transitioned := collect.Agent{
		ID: "transitioned", Kind: collect.KindInteractive, Life: collect.LifeWorking,
		Project: "/alpha", Name: "transitioned",
		StatusChangedAt: now, ActivityAt: now.Add(-time.Minute),
	}
	busy := collect.Agent{
		ID: "busy", Kind: collect.KindInteractive, Life: collect.LifeInput,
		Project: "/beta", Name: "busy",
		StatusChangedAt: now.Add(-time.Minute), ActivityAt: now,
	}
	m := Model{
		flatView: true,
		snap: collect.Snapshot{
			Projects: []collect.Project{
				{Key: "/beta", Name: "beta", Agents: []collect.Agent{busy}},
				{Key: "/alpha", Name: "alpha", Agents: []collect.Agent{transitioned}},
			},
			Agents: []collect.Agent{busy, transitioned},
		},
	}
	m.rebuildRows()
	if got := m.rows[m.selRows[0]].agent; got == nil || got.ID != "transitioned" {
		t.Errorf("first flat agent = %#v, want the agent whose status just changed", got)
	}
}

// Activity still orders agents that changed state at the same instant, which
// is every agent present when a single refresh stamps them all at once.
func TestFlatViewUsesActivityWhenStatusChangesTie(t *testing.T) {
	now := time.Date(2026, time.August, 20, 9, 0, 0, 0, time.UTC)
	stale := collect.Agent{
		ID: "stale", Kind: collect.KindInteractive, Life: collect.LifeWorking,
		Project: "/alpha", Name: "stale", StatusChangedAt: now, ActivityAt: now.Add(-time.Hour),
	}
	fresh := collect.Agent{
		ID: "fresh", Kind: collect.KindInteractive, Life: collect.LifeWorking,
		Project: "/beta", Name: "fresh", StatusChangedAt: now, ActivityAt: now,
	}
	m := Model{
		flatView: true,
		snap: collect.Snapshot{
			Projects: []collect.Project{
				{Key: "/alpha", Name: "alpha", Agents: []collect.Agent{stale}},
				{Key: "/beta", Name: "beta", Agents: []collect.Agent{fresh}},
			},
			Agents: []collect.Agent{stale, fresh},
		},
	}
	m.rebuildRows()
	if got := m.rows[m.selRows[0]].agent; got == nil || got.ID != "fresh" {
		t.Errorf("first flat agent = %#v, want the more recently active pane", got)
	}
}

// A working background job rewrites its state file as its token count climbs,
// bumping UpdatedAt without changing state. That write must not outrank an
// agent that genuinely transitioned, or background churn buries exactly the
// transitions the flat view exists to surface.
func TestBackgroundStateFileWritesDoNotOutrankTransitions(t *testing.T) {
	now := time.Date(2026, time.August, 20, 9, 0, 0, 0, time.UTC)
	churning := collect.Agent{
		ID: "churning", Kind: collect.KindBackground, Life: collect.LifeWorking,
		Project: "/alpha", Name: "churning",
		StatusChangedAt: now.Add(-time.Hour), UpdatedAt: now,
	}
	transitioned := collect.Agent{
		ID: "transitioned", Kind: collect.KindInteractive, Life: collect.LifeIdle,
		Project: "/beta", Name: "transitioned", StatusChangedAt: now.Add(-time.Minute),
	}
	m := Model{
		flatView: true,
		snap: collect.Snapshot{
			Projects: []collect.Project{
				{Key: "/alpha", Name: "alpha", Agents: []collect.Agent{churning}},
				{Key: "/beta", Name: "beta", Agents: []collect.Agent{transitioned}},
			},
			Agents: []collect.Agent{churning, transitioned},
		},
	}
	m.rebuildRows()
	if got := m.rows[m.selRows[0]].agent; got == nil || got.ID != "transitioned" {
		t.Errorf("first flat agent = %#v, want the agent whose status changed", got)
	}

	// The file timestamp still breaks ties one tier down, so background
	// progress orders backgrounds against each other.
	quiet := collect.Agent{
		ID: "quiet", Kind: collect.KindBackground, Life: collect.LifeWorking,
		Project: "/gamma", Name: "quiet",
		StatusChangedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour),
	}
	m.snap.Projects[1] = collect.Project{Key: "/gamma", Name: "gamma", Agents: []collect.Agent{quiet}}
	m.snap.Agents = []collect.Agent{churning, quiet}
	m.rebuildRows()
	if got := m.rows[m.selRows[0]].agent; got == nil || got.ID != "churning" {
		t.Errorf("first flat agent = %#v, want the background job still writing", got)
	}
}

// Interactive agents never carry UpdatedAt — it only exists for background
// state files — so ranking project headers on it alone tied every tmux-only
// project at the zero time and fell through to alphabetical order. A pane
// changing state has to lift its project too, not just its own row.
func TestGroupedProjectsFollowLatestStatusChange(t *testing.T) {
	now := time.Date(2026, time.August, 20, 9, 0, 0, 0, time.UTC)
	older := collect.Agent{
		ID: "older", Kind: collect.KindInteractive, Life: collect.LifeWorking,
		Project: "/alpha", Name: "older", StatusChangedAt: now.Add(-time.Minute),
	}
	newer := collect.Agent{
		ID: "newer", Kind: collect.KindInteractive, Life: collect.LifeWorking,
		Project: "/zeta", Name: "newer", StatusChangedAt: now,
	}
	m := Model{
		snap: collect.Snapshot{
			Projects: []collect.Project{
				{Key: "/alpha", Name: "alpha", Agents: []collect.Agent{older}},
				{Key: "/zeta", Name: "zeta", Agents: []collect.Agent{newer}},
			},
			Agents: []collect.Agent{older, newer},
		},
	}
	m.rebuildRows()
	if m.rows[0].kind != rowHeader || m.rows[0].project.Key != "/zeta" {
		t.Errorf("first project header = %#v, want the project whose agent just changed", m.rows[0])
	}
}

// The age column was blank for every tmux row, because Agent.Age keyed off the
// background-only UpdatedAt field.
func TestInteractiveRowShowsTimeSinceStatusChange(t *testing.T) {
	now := time.Date(2026, time.August, 20, 9, 0, 0, 0, time.UTC)
	a := &collect.Agent{
		ID: "%1", Kind: collect.KindInteractive, Life: collect.LifeWorking,
		StatusChangedAt: now.Add(-90 * time.Second),
	}
	if got := agentMeta(a, now); !strings.Contains(got, "1m") {
		t.Errorf("interactive row meta = %q, want an age since the status change", got)
	}
	if got := agentMeta(&collect.Agent{ID: "%2", Kind: collect.KindInteractive}, now); got != "" {
		t.Errorf("meta for an agent with no observed transition = %q, want empty", got)
	}
}

// flatIDs is the flat view's rendered order, for the ordering tests below.
func flatIDs(m Model) []string {
	ids := make([]string, 0, len(m.selRows))
	for _, ri := range m.selRows {
		if a := m.rows[ri].agent; a != nil {
			ids = append(ids, a.ID)
		}
	}
	return ids
}

// openWith feeds one snapshot into a fresh model, the way launching amux does,
// and returns the model with its first list built.
func openWith(m Model, snap collect.Snapshot, taken time.Time) Model {
	snap.Taken = taken
	m.recordStatusChangeTimes(&snap)
	m.snap = snap
	m.rebuildRows()
	return m
}

// The reported symptom: the list appeared, then visibly resorted itself a
// refresh later, because the CPU tie-breaker was frozen from the *second*
// snapshot. Nothing about the agents changed between the two renders, so
// neither may the order.
func TestOpeningOrderDoesNotResortOnTheSecondSnapshot(t *testing.T) {
	now := time.Date(2026, time.August, 20, 9, 0, 0, 0, time.UTC)
	// Alphabetical order disagrees with CPU order, so a late-seeded CPU
	// tie-breaker shows up as a swap.
	quiet := collect.Agent{ID: "a", Kind: collect.KindInteractive, PID: 1, Life: collect.LifeWorking, CPU: 5, Project: "/p", Name: "a"}
	busy := collect.Agent{ID: "b", Kind: collect.KindInteractive, PID: 2, Life: collect.LifeWorking, CPU: 20, Project: "/p", Name: "b"}

	m := openWith(Model{flatView: true}, projSnap(quiet, busy), now)
	first := flatIDs(m)
	if len(first) != 2 || first[0] != "b" {
		t.Fatalf("opening order = %v, want the busiest agent first", first)
	}

	m = openWith(m, projSnap(quiet, busy), now.Add(2*time.Second))
	if second := flatIDs(m); !slices.Equal(first, second) {
		t.Errorf("order resorted on the second snapshot: %v then %v", first, second)
	}
}

// Agents that have not changed state since launch open exactly where the user
// left them, rather than in whatever order the weaker tie-breakers produce.
func TestRememberedOrderAppliesToTheOpeningList(t *testing.T) {
	now := time.Date(2026, time.August, 20, 9, 0, 0, 0, time.UTC)
	agents := []collect.Agent{mkAgent("a", collect.LifeWorking), mkAgent("b", collect.LifeWorking), mkAgent("c", collect.LifeWorking)}

	m := Model{flatView: true, flatOrder: []string{"c", "a", "b"}}
	m.refreshRememberedRank()
	m = openWith(m, projSnap(agents...), now)
	if got := flatIDs(m); !slices.Equal(got, []string{"c", "a", "b"}) {
		t.Fatalf("opening order = %v, want the remembered order", got)
	}

	// A real transition still wins: memory only settles the startup cohort.
	next := projSnap(agents[0], collect.Agent{ID: "b", Kind: collect.KindBackground, Life: collect.LifeInput, Project: "/p", Name: "b"}, agents[2])
	m = openWith(m, next, now.Add(2*time.Second))
	if got := flatIDs(m); len(got) == 0 || got[0] != "b" {
		t.Errorf("order after a transition = %v, want the changed agent first", got)
	}
}

// An agent amux has never displayed has no remembered position. Reading the
// rank map without checking for presence would give it position zero and put
// it above every agent the user actually placed.
func TestUnrememberedAgentsDoNotInheritPositionZero(t *testing.T) {
	now := time.Date(2026, time.August, 20, 9, 0, 0, 0, time.UTC)
	// "gone" is a pane from the last session that no longer exists, so
	// "known" sits at position 1 and a phantom zero would outrank it.
	m := Model{flatView: true, flatOrder: []string{"gone", "known"}}
	m.refreshRememberedRank()
	m = openWith(m, projSnap(mkAgent("fresh", collect.LifeWorking), mkAgent("known", collect.LifeWorking)), now)
	if got := flatIDs(m); len(got) == 0 || got[0] != "known" {
		t.Errorf("opening order = %v, want the remembered agent above the new one", got)
	}
}

// p switches between two independently remembered layouts, and each has to
// come back the way it was left.
func TestEachViewRestoresItsOwnRememberedOrder(t *testing.T) {
	now := time.Date(2026, time.August, 20, 9, 0, 0, 0, time.UTC)
	agents := projSnap(mkAgent("a", collect.LifeWorking), mkAgent("b", collect.LifeWorking))

	m := Model{flatOrder: []string{"a", "b"}, groupedOrder: []string{"b", "a"}}
	m.refreshRememberedRank()
	m = openWith(m, agents, now)
	if got := flatIDs(m); !slices.Equal(got, []string{"b", "a"}) {
		t.Fatalf("grouped order = %v, want the remembered grouped order", got)
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	m = next.(Model)
	if got := flatIDs(m); !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("flat order = %v, want the remembered flat order", got)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	m = next.(Model)
	if got := flatIDs(m); !slices.Equal(got, []string{"b", "a"}) {
		t.Errorf("grouped order after toggling back = %v, want the remembered grouped order", got)
	}
}

// A project section returns to where it was, which is where its highest-placed
// agent sat — the saved order lists agents project by project.
func TestRememberedOrderRestoresProjectSections(t *testing.T) {
	now := time.Date(2026, time.August, 20, 9, 0, 0, 0, time.UTC)
	alpha := collect.Agent{ID: "alpha1", Kind: collect.KindBackground, Life: collect.LifeWorking, Project: "/alpha", Name: "alpha1"}
	zeta := collect.Agent{ID: "zeta1", Kind: collect.KindBackground, Life: collect.LifeWorking, Project: "/zeta", Name: "zeta1"}

	m := Model{groupedOrder: []string{"zeta1", "alpha1"}}
	m.refreshRememberedRank()
	m = openWith(m, collect.Snapshot{
		Projects: []collect.Project{
			{Key: "/alpha", Name: "alpha", Agents: []collect.Agent{alpha}},
			{Key: "/zeta", Name: "zeta", Agents: []collect.Agent{zeta}},
		},
		Agents: []collect.Agent{alpha, zeta},
	}, now)

	if m.rows[0].kind != rowHeader || m.rows[0].project.Key != "/zeta" {
		t.Errorf("first project header = %#v, want the remembered leading project", m.rows[0])
	}
}

// A filtered list hides agents; recording it would evict every hidden agent
// from the saved layout.
func TestFilteredRendersDoNotOverwriteTheRememberedOrder(t *testing.T) {
	now := time.Date(2026, time.August, 20, 9, 0, 0, 0, time.UTC)
	m := Model{flatView: true, notesStore: notes.Load()}
	m = openWith(m, projSnap(mkAgent("keep", collect.LifeWorking), mkAgent("other", collect.LifeWorking)), now)
	before := slices.Clone(m.flatOrder)

	m.filter = "keep"
	m.rebuildRows()
	if !slices.Equal(m.flatOrder, before) {
		t.Errorf("remembered order after filtering = %v, want it left at %v", m.flatOrder, before)
	}
}

// Launching amux has to pick the saved layout up from disk, not just hold it
// in memory for the session that wrote it.
func TestNewRestoresRememberedOrder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	saveViewPreferences(defaultPreferencesPath(), viewPreferences{
		FlatView:     true,
		FlatOrder:    []string{"%2", "%1"},
		GroupedOrder: []string{"%1", "%2"},
	})

	m := New()
	if !slices.Equal(m.flatOrder, []string{"%2", "%1"}) {
		t.Errorf("restored flat order = %v, want the saved one", m.flatOrder)
	}
	if got, ok := m.rememberedRank["%1"]; !ok || got != 1 {
		t.Errorf("rank for %%1 = %d (known %v), want position 1 from the flat order", got, ok)
	}
}

func TestKeyboardHelpScrollsWithinSmallTerminal(t *testing.T) {
	m := Model{width: 60, height: 12}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = next.(Model)
	help := m.View()
	if !strings.Contains(help, "Keyboard shortcuts") || !strings.Contains(help, "1-7 / 16") {
		t.Fatalf("first help page = %q", help)
	}
	lines := strings.Split(help, "\n")
	if len(lines) > m.height {
		t.Fatalf("help height = %d, want at most %d", len(lines), m.height)
	}
	for _, line := range lines {
		if got := lipgloss.Width(line); got > m.width {
			t.Fatalf("help line width = %d, want at most %d: %q", got, m.width, line)
		}
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = next.(Model)
	if m.helpOffset != 1 {
		t.Fatalf("help offset = %d, want 1 after j", m.helpOffset)
	}
	if help = m.View(); !strings.Contains(help, "2-8 / 16") {
		t.Errorf("scrolled help page = %q", help)
	}
}

func TestKeyboardHelpFitsNarrowTerminal(t *testing.T) {
	m := Model{width: 20, height: 6}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = next.(Model)
	help := m.View()
	lines := strings.Split(help, "\n")
	if len(lines) > m.height {
		t.Fatalf("narrow help height = %d, want at most %d: %q", len(lines), m.height, help)
	}
	for _, line := range lines {
		if got := lipgloss.Width(line); got > m.width {
			t.Fatalf("narrow help line width = %d, want at most %d: %q", got, m.width, line)
		}
	}
	if !strings.Contains(help, "j/k · ?/esc") {
		t.Errorf("narrow help missing compact close hint: %q", help)
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

func TestStartupPaneSelectsMatchingInteractiveAgent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("TMUX_PANE", "%first")
	t.Setenv("AMUX_START_PANE", " %target ")

	first := collect.Agent{
		ID: "%first", TmuxTarget: "%target", Kind: collect.KindInteractive,
		Life: collect.LifeInput, Project: "/p", Name: "first",
	}
	target := collect.Agent{
		ID: "%target", TmuxTarget: "session:0.1", Kind: collect.KindInteractive,
		Life: collect.LifeWorking, Project: "/p", Name: "target",
	}
	m := New()
	next, _ := m.Update(snapshotMsg{snap: projSnap(first, target)})
	m = next.(Model)

	if got := m.selectedAgent(); got == nil || got.ID != target.ID {
		t.Fatalf("selected agent = %#v, want %q", got, target.ID)
	}
}

func TestStartupPaneNoMatchKeepsDefaultSelection(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("AMUX_START_PANE", "")
	t.Setenv("TMUX_PANE", "%missing")

	first := collect.Agent{
		ID: "%first", Kind: collect.KindInteractive,
		Life: collect.LifeInput, Project: "/p", Name: "first",
	}
	other := collect.Agent{
		ID: "%other", Kind: collect.KindInteractive,
		Life: collect.LifeWorking, Project: "/p", Name: "other",
	}
	m := New()
	next, _ := m.Update(snapshotMsg{snap: projSnap(first, other)})
	m = next.(Model)

	if got := m.selectedAgent(); got == nil || got.ID != first.ID {
		t.Fatalf("selected agent = %#v, want default %q", got, first.ID)
	}
}

func TestStartupPaneNoMatchConsumesOneShotSelection(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("TMUX_PANE", "%target")
	t.Setenv("AMUX_START_PANE", "")

	first := collect.Agent{
		ID: "%first", Kind: collect.KindInteractive,
		Life: collect.LifeInput, Project: "/p", Name: "first",
	}
	target := collect.Agent{
		ID: "%target", Kind: collect.KindInteractive,
		Life: collect.LifeWorking, Project: "/p", Name: "target",
	}
	m := New()
	next, _ := m.Update(snapshotMsg{snap: projSnap(first)})
	m = next.(Model)
	next, _ = m.Update(snapshotMsg{snap: projSnap(first, target)})
	m = next.(Model)

	if got := m.selectedAgent(); got == nil || got.ID != first.ID {
		t.Fatalf("second snapshot selected %#v, want default %q after initial no-match", got, first.ID)
	}
}

func TestStartupPaneSelectionSurvivesFailedSnapshot(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("TMUX_PANE", "%target")
	t.Setenv("AMUX_START_PANE", "")

	first := collect.Agent{
		ID: "%first", Kind: collect.KindInteractive,
		Life: collect.LifeInput, Project: "/p", Name: "first",
	}
	target := collect.Agent{
		ID: "%target", Kind: collect.KindInteractive,
		Life: collect.LifeWorking, Project: "/p", Name: "target",
	}
	m := New()
	next, _ := m.Update(snapshotMsg{err: errors.New("collect failed")})
	m = next.(Model)
	next, _ = m.Update(snapshotMsg{snap: projSnap(first, target)})
	m = next.(Model)

	if got := m.selectedAgent(); got == nil || got.ID != target.ID {
		t.Fatalf("successful snapshot selected %#v, want %q after failed snapshot", got, target.ID)
	}
}

func TestStartupPaneSelectionDoesNotOverrideLaterNavigation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("TMUX_PANE", "%target")
	t.Setenv("AMUX_START_PANE", "")

	first := collect.Agent{
		ID: "%first", Kind: collect.KindInteractive,
		Life: collect.LifeInput, Project: "/p", Name: "first",
	}
	target := collect.Agent{
		ID: "%target", Kind: collect.KindInteractive,
		Life: collect.LifeWorking, Project: "/p", Name: "target",
	}
	m := New()
	next, _ := m.Update(snapshotMsg{snap: projSnap(first, target)})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = next.(Model)
	if got := m.selectedAgent(); got == nil || got.ID != first.ID {
		t.Fatalf("navigation selected %#v, want %q", got, first.ID)
	}

	next, _ = m.Update(snapshotMsg{snap: projSnap(first, target)})
	m = next.(Model)
	if got := m.selectedAgent(); got == nil || got.ID != first.ID {
		t.Fatalf("second snapshot selected %#v, want %q", got, first.ID)
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

// TestSelectionClampsWhenSelectedAgentVanishes verifies that deleting the last
// selected row falls back to the prior remaining agent.
func TestSelectionClampsWhenSelectedAgentVanishes(t *testing.T) {
	m := Model{snap: projSnap(
		mkAgent("a", collect.LifeWorking),
		mkAgent("b", collect.LifeWorking),
	)}
	m.rebuildRows()
	m.sel = 1 // select "b"

	// "b" finishes and is hidden; only "a" remains visible.
	m.snap = projSnap(
		mkAgent("a", collect.LifeWorking),
		mkAgent("b", collect.LifeDone),
	)
	m.rebuildRows()

	if got := m.selectedAgent(); got == nil || got.ID != "a" {
		t.Errorf("after final agent vanished, selected %v, want a", got)
	}
}

func TestSelectionMovesToPredecessorWhenMiddleAgentVanishes(t *testing.T) {
	m := Model{snap: projSnap(
		mkAgent("a", collect.LifeWorking),
		mkAgent("b", collect.LifeWorking),
		mkAgent("c", collect.LifeWorking),
	)}
	m.rebuildRows()
	m.sel = 1 // select "b"

	// "b" finishes and disappears; selection moves back one row to "a".
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

func TestStatusTransitionTimesPersistAcrossRefreshesAndOrderProjectAgents(t *testing.T) {
	first := time.Date(2026, time.August, 10, 9, 0, 0, 0, time.UTC)
	second := first.Add(time.Minute)
	m := Model{snap: projSnap(
		collect.Agent{ID: "transitioned", Life: collect.LifeWorking, StatusChangedAt: first, Project: "/p"},
		collect.Agent{ID: "unchanged", Life: collect.LifeError, StatusChangedAt: first, Project: "/p"},
	)}
	next := projSnap(
		collect.Agent{ID: "transitioned", Life: collect.LifeIdle, Project: "/p"},
		collect.Agent{ID: "unchanged", Life: collect.LifeError, Project: "/p"},
	)
	next.Taken = second

	m.recordStatusChangeTimes(&next)
	if got := next.Agents[0].StatusChangedAt; !got.Equal(second) {
		t.Errorf("transition timestamp = %v, want %v", got, second)
	}
	if got := next.Agents[1].StatusChangedAt; !got.Equal(first) {
		t.Errorf("unchanged timestamp = %v, want %v", got, first)
	}
	if got := next.Projects[0].Agents[0].StatusChangedAt; !got.Equal(second) {
		t.Errorf("project transition timestamp = %v, want %v", got, second)
	}

	m.snap = next
	m.rebuildRows()
	if got := m.rows[m.selRows[0]].agent.ID; got != "transitioned" {
		t.Errorf("first project agent = %q, want newest transition", got)
	}
}

func TestStatusTransitionTimeResetsForReplacementInteractiveProcess(t *testing.T) {
	first := time.Date(2026, time.August, 10, 9, 0, 0, 0, time.UTC)
	second := first.Add(time.Minute)
	m := Model{snap: projSnap(collect.Agent{
		ID: "pane", Kind: collect.KindInteractive, PID: 101,
		Life: collect.LifeWorking, StatusChangedAt: first, Project: "/p",
	})}
	next := projSnap(collect.Agent{
		ID: "pane", Kind: collect.KindInteractive, PID: 202,
		Life: collect.LifeWorking, Project: "/p",
	})
	next.Taken = second

	m.recordStatusChangeTimes(&next)
	if got := next.Agents[0].StatusChangedAt; !got.Equal(second) {
		t.Errorf("replacement process timestamp = %v, want %v", got, second)
	}
}

func TestInteractivePreviewChangeRefreshesFlatActivityOrder(t *testing.T) {
	first := time.Date(2026, time.August, 20, 9, 0, 0, 0, time.UTC)
	second := first.Add(time.Minute)
	third := second.Add(time.Minute)
	active := collect.Agent{
		ID: "active", Kind: collect.KindInteractive, PID: 101, Life: collect.LifeWorking,
		StatusChangedAt: first, ActivityAt: first, PaneTitle: "π: static task",
		Preview: []string{"old output"}, PreviewFingerprint: "old output", Project: "/p", Name: "active",
	}
	other := collect.Agent{
		ID: "other", Kind: collect.KindInteractive, PID: 102, Life: collect.LifeWorking,
		StatusChangedAt: first, ActivityAt: first, PaneTitle: "other task",
		Preview: []string{"stable output"}, PreviewFingerprint: "stable output", Project: "/p", Name: "other",
	}
	m := Model{snap: projSnap(active, other)}
	next := projSnap(
		collect.Agent{
			ID: "active", Kind: collect.KindInteractive, PID: 101, Life: collect.LifeWorking,
			PaneTitle: "π: static task", Preview: []string{"new output"}, PreviewFingerprint: "new output",
			Project: "/p", Name: "active",
		},
		other,
	)
	next.Taken = second

	m.recordStatusChangeTimes(&next)
	if got := next.Agents[0].ActivityAt; !got.Equal(first) {
		t.Errorf("unconfirmed preview activity = %v, want %v", got, first)
	}
	m.snap = next
	next = projSnap(
		collect.Agent{
			ID: "active", Kind: collect.KindInteractive, PID: 101, Life: collect.LifeWorking,
			PaneTitle: "π: static task", Preview: []string{"new output"}, PreviewFingerprint: "new output",
			Project: "/p", Name: "active",
		},
		other,
	)
	next.Taken = third
	m.recordStatusChangeTimes(&next)
	if got := next.Agents[0].ActivityAt; !got.Equal(third) {
		t.Errorf("confirmed preview activity = %v, want %v", got, third)
	}
	if got := next.Projects[0].Agents[0].ActivityAt; !got.Equal(third) {
		t.Errorf("project preview activity = %v, want %v", got, third)
	}
	if got := next.Agents[0].StatusChangedAt; !got.Equal(first) {
		t.Errorf("status timestamp = %v, want unchanged %v", got, first)
	}

	m.snap = next
	m.flatView = true
	m.rebuildRows()
	if got := m.rows[m.selRows[0]].agent; got == nil || got.ID != "active" {
		t.Errorf("first flat agent = %#v, want recently active pane", got)
	}
}

func TestMissingPanePreviewDoesNotCountAsActivity(t *testing.T) {
	prior := collect.Agent{
		Kind: collect.KindInteractive, PaneTitle: "task", Detail: "running", Preview: []string{"output"},
	}
	current := prior
	current.Preview = nil
	if interactiveActivityChanged(prior, current) {
		t.Fatal("missing current preview counted as pane activity")
	}

	prior.Preview = nil
	current.Preview = []string{"output"}
	if interactiveActivityChanged(prior, current) {
		t.Fatal("recovered preview counted as pane activity")
	}
}

func TestMissingPreviewPreservesPendingActivityCandidate(t *testing.T) {
	first := time.Date(2026, time.August, 20, 9, 0, 0, 0, time.UTC)
	changed := first.Add(time.Minute)
	recovered := changed.Add(2 * time.Minute)
	prior := collect.Agent{
		ID: "pane", Kind: collect.KindInteractive, PID: 101, Life: collect.LifeWorking,
		StatusChangedAt: first, ActivityAt: first, PaneTitle: "static task",
		Preview: []string{"old"}, PreviewFingerprint: "old", Project: "/p",
	}
	m := Model{snap: projSnap(prior)}
	next := projSnap(collect.Agent{
		ID: "pane", Kind: collect.KindInteractive, PID: 101, Life: collect.LifeWorking,
		PaneTitle: "static task", Preview: []string{"new"}, PreviewFingerprint: "new", Project: "/p",
	})
	next.Taken = changed
	m.recordStatusChangeTimes(&next)
	m.snap = next

	next = projSnap(collect.Agent{
		ID: "pane", Kind: collect.KindInteractive, PID: 101, Life: collect.LifeWorking,
		PaneTitle: "static task", Preview: nil, Project: "/p",
	})
	next.Taken = changed.Add(time.Minute)
	m.recordStatusChangeTimes(&next)
	m.snap = next

	next = projSnap(collect.Agent{
		ID: "pane", Kind: collect.KindInteractive, PID: 101, Life: collect.LifeWorking,
		PaneTitle: "static task", Preview: []string{"new"}, PreviewFingerprint: "new", Project: "/p",
	})
	next.Taken = recovered
	m.recordStatusChangeTimes(&next)
	if got := next.Agents[0].ActivityAt; !got.Equal(recovered) {
		t.Errorf("recovered preview activity = %v, want %v", got, recovered)
	}
}

func TestMissingPreviewDoesNotPromoteAcceptedFingerprint(t *testing.T) {
	first := time.Date(2026, time.August, 20, 9, 0, 0, 0, time.UTC)
	prior := collect.Agent{
		ID: "pane", Kind: collect.KindInteractive, PID: 101, Life: collect.LifeWorking,
		PaneTitle: "static task", Preview: []string{"stable"}, PreviewFingerprint: "stable", Project: "/p",
	}
	m := Model{}
	next := projSnap(prior)
	next.Taken = first
	m.recordStatusChangeTimes(&next)
	m.snap = next

	next = projSnap(collect.Agent{
		ID: "pane", Kind: collect.KindInteractive, PID: 101, Life: collect.LifeWorking,
		PaneTitle: "static task", Preview: nil, Project: "/p",
	})
	next.Taken = first.Add(time.Minute)
	m.recordStatusChangeTimes(&next)
	m.snap = next

	next = projSnap(prior)
	next.Taken = first.Add(2 * time.Minute)
	m.recordStatusChangeTimes(&next)
	if got := next.Agents[0].ActivityAt; !got.Equal(first) {
		t.Errorf("unchanged recovered preview activity = %v, want %v", got, first)
	}
}

func TestSpinnerOnlyTitleChangeDoesNotCountAsActivity(t *testing.T) {
	prior := collect.Agent{
		Kind: collect.KindInteractive, PaneTitle: "π ⠦ static task", Detail: "π ⠦ static task",
		Preview: []string{"stable output"}, PreviewFingerprint: "stable output",
	}
	current := prior
	current.PaneTitle = "π ⠧ static task"
	current.Detail = "π ⠧ static task"
	if interactiveActivityChanged(prior, current) {
		t.Fatal("spinner-only title change counted as pane activity")
	}
}

func TestMissingPanePreviewPreservesLifecycleAndRecency(t *testing.T) {
	first := time.Date(2026, time.August, 20, 9, 0, 0, 0, time.UTC)
	second := first.Add(time.Minute)
	prior := collect.Agent{
		ID: "pane", Kind: collect.KindInteractive, PID: 101, Life: collect.LifeWorking,
		NeedsInput: true, StatusChangedAt: first, ActivityAt: first,
		PaneTitle: "task", Detail: "running", Preview: []string{"output"}, Project: "/p",
	}
	m := Model{snap: projSnap(prior)}
	next := projSnap(collect.Agent{
		ID: "pane", Kind: collect.KindInteractive, PID: 101, Life: collect.LifeIdle,
		PaneTitle: "task", Detail: "running", Preview: nil, Project: "/p",
	})
	next.Taken = second

	m.recordStatusChangeTimes(&next)
	got := next.Agents[0]
	if got.Life != collect.LifeWorking || !got.NeedsInput {
		t.Errorf("capture failure lifecycle = %q input=%v, want working input=true", got.Life, got.NeedsInput)
	}
	if !got.StatusChangedAt.Equal(first) || !got.ActivityAt.Equal(first) {
		t.Errorf("capture failure timestamps = status:%v activity:%v, want %v", got.StatusChangedAt, got.ActivityAt, first)
	}
	projectAgent := next.Projects[0].Agents[0]
	if projectAgent.Life != collect.LifeWorking || !projectAgent.NeedsInput {
		t.Errorf("project capture failure lifecycle = %q input=%v, want working input=true", projectAgent.Life, projectAgent.NeedsInput)
	}
}

func TestPaneTitleChangeUpdatesActivityWithoutPreview(t *testing.T) {
	first := time.Date(2026, time.August, 20, 9, 0, 0, 0, time.UTC)
	second := first.Add(time.Minute)
	prior := collect.Agent{
		ID: "pane", Kind: collect.KindInteractive, PID: 101, Life: collect.LifeWorking,
		StatusChangedAt: first, ActivityAt: first, PaneTitle: "old task", Preview: []string{"output"}, Project: "/p",
	}
	m := Model{snap: projSnap(prior)}
	next := projSnap(collect.Agent{
		ID: "pane", Kind: collect.KindInteractive, PID: 101, Life: collect.LifeIdle,
		PaneTitle: "new task", Preview: nil, Project: "/p",
	})
	next.Taken = second

	m.recordStatusChangeTimes(&next)
	got := next.Agents[0]
	if got.Life != collect.LifeIdle || !got.StatusChangedAt.Equal(second) || !got.ActivityAt.Equal(second) {
		t.Errorf("title transition = life:%q status:%v activity:%v, want idle at %v", got.Life, got.StatusChangedAt, got.ActivityAt, second)
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

	row := m.renderAgentRow(&agent, nil, 120, false, time.Now())
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

func TestKeyboardHelpShowsSelectedAgentCapabilities(t *testing.T) {
	live := collect.Agent{
		ID: "live", Kind: collect.KindBackground, Tool: "claude", Live: true,
		Life: collect.LifeInput, Project: "/p", Name: "live",
	}
	m := Model{snap: projSnap(live), width: 200, height: 30}
	m.rebuildRows()
	footer := m.renderFooter()
	if !strings.Contains(footer, "? help") || strings.Contains(footer, "c send") {
		t.Errorf("compact footer = %q", footer)
	}
	help := m.renderHelpPopup()
	if !strings.Contains(help, "attach a pane or manage a live Claude agent") || !strings.Contains(help, "send text to the selected interactive pane") {
		t.Errorf("keyboard help = %q", help)
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

// TestNoteSuggestionByTool verifies which pane titles become note prefills:
// omp drops its "π:" prefix, Claude Code drops its leading spinner glyph, and
// other tools, empty titles, and non-interactive agents offer nothing.
func TestNoteSuggestionByTool(t *testing.T) {
	interactive := func(tool, title string) *collect.Agent {
		return &collect.Agent{Kind: collect.KindInteractive, Tool: tool, PaneTitle: title}
	}
	cases := []struct {
		name  string
		agent *collect.Agent
		want  string
	}{
		{"omp strips colon prefix", interactive("omp", "π: fix dropdown"), "fix dropdown"},
		// Live omp titles use "π > <task>"; the older "π: <task>" form above
		// still ships, so both have to survive the same strip.
		{"omp strips arrow prefix", interactive("omp", "π > Improve activity sorting"), "Improve activity sorting"},
		{"omp glyph only", interactive("omp", "π >"), ""},
		{"claude idle glyph", interactive("claude", "✳ 학생 개별 수강료 팝업창 개선"), "학생 개별 수강료 팝업창 개선"},
		{"claude spinner glyph", interactive("claude", "⠐ build the API docs"), "build the API docs"},
		{"claude bare title", interactive("claude", "plain title"), "plain title"},
		{"claude glyph only", interactive("claude", "✳"), ""},
		{"other tool", interactive("codex", "some task"), ""},
		{"empty title", interactive("claude", "  "), ""},
		{"background claude", &collect.Agent{Kind: collect.KindBackground, Tool: "claude", PaneTitle: "✳ task"}, ""},
		{"nil agent", nil, ""},
	}
	for _, tc := range cases {
		if got := noteSuggestion(tc.agent); got != tc.want {
			t.Errorf("%s: noteSuggestion = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestFirstInteractiveClaudeNoteOffersStrippedTitleAsPrefill mirrors the OMP
// flow for a Claude Code pane: the first note offers the pane title with its
// status glyph removed.
func TestFirstInteractiveClaudeNoteOffersStrippedTitleAsPrefill(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")

	cl := collect.Agent{
		ID: "claude-pane", Kind: collect.KindInteractive, Tool: "claude",
		Life: collect.LifeWorking, Project: "/p", Name: "claude-pane",
		PaneTitle: "✳ 윈도우 타이틀에 메모 기능 추가",
	}
	m := Model{snap: projSnap(cl), notesStore: notes.Load(), width: 200}
	m.rebuildRows()

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	m = next.(Model)
	if !m.suggestingNote || m.naming {
		t.Fatalf("first Claude note should ask before editing: suggesting=%v naming=%v", m.suggestingNote, m.naming)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = next.(Model)
	if m.suggestingNote || !m.naming {
		t.Fatalf("y should open the note editor: suggesting=%v naming=%v", m.suggestingNote, m.naming)
	}
	if got, want := m.noteInput, "윈도우 타이틀에 메모 기능 추가"; got != want {
		t.Errorf("suggested note = %q, want %q", got, want)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got, want := next.(Model).notesStore.Get("claude-pane"), "윈도우 타이틀에 메모 기능 추가"; got != want {
		t.Errorf("saved suggested note = %q, want %q", got, want)
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
	m := Model{snap: projSnap(bg), width: 200, height: 30}
	m.rebuildRows()
	if footer := m.renderFooter(); !strings.Contains(footer, "? help") || strings.Contains(footer, "x dismiss") {
		t.Fatalf("compact footer = %q", footer)
	}
	if help := m.renderHelpPopup(); !strings.Contains(help, "dismiss a background agent") {
		t.Fatalf("keyboard help missing dismiss shortcut: %q", help)
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

// TestInteractiveKillSelectsPreviousPane exercises the complete x/y kill flow,
// including the asynchronous success message, and moves selection back one row.
func TestInteractiveKillSelectsPreviousPane(t *testing.T) {
	for _, tc := range []struct {
		name     string
		selected int
		want     string
	}{
		{name: "middle selects predecessor", selected: 1, want: "a"},
		{name: "final selects predecessor", selected: 2, want: "b"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			agents := []collect.Agent{
				{ID: "a", Kind: collect.KindInteractive, Life: collect.LifeWorking, Project: "/p", Name: "a", TmuxTarget: "sess:0.0"},
				{ID: "b", Kind: collect.KindInteractive, Life: collect.LifeWorking, Project: "/p", Name: "b", TmuxTarget: "sess:0.1"},
				{ID: "c", Kind: collect.KindInteractive, Life: collect.LifeWorking, Project: "/p", Name: "c", TmuxTarget: "sess:0.2"},
			}
			m := Model{snap: projSnap(agents...)}
			m.rebuildRows()
			m.sel = tc.selected
			target := m.selectedAgent()
			if target == nil {
				t.Fatal("precondition: selected interactive agent is nil")
			}

			next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
			m = next.(Model)
			if !m.confirmingKill || m.killTarget != target.TmuxTarget || m.killName != target.Name {
				t.Fatalf("x armed kill for target=%q name=%q, want target=%q name=%q", m.killTarget, m.killName, target.TmuxTarget, target.Name)
			}
			if cmd != nil {
				t.Fatal("x should only arm confirmation")
			}

			next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
			m = next.(Model)
			if cmd == nil {
				t.Fatal("y should emit the kill command")
			}

			next, cmd = m.Update(killedMsg{target: target.TmuxTarget, name: target.Name})
			m = next.(Model)
			if cmd != nil {
				t.Fatal("successful kill should not emit a follow-up command")
			}
			if got := m.selectedAgent(); got == nil || got.ID != tc.want {
				t.Fatalf("selected agent after killing %q = %#v, want %q", target.ID, got, tc.want)
			}
		})
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

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	m = next.(Model)
	if !m.flatView {
		t.Fatal("p should enable flat project view")
	}
	want := viewPreferences{Stacked: true, HideDetail: true, FlatView: true, FlatOrder: []string{}}
	if got := loadViewPreferences(path); !got.equal(want) {
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
