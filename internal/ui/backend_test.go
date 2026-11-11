package ui

import (
	"errors"
	"os"
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

func backendTestModel(t *testing.T) Model {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	return New()
}

func TestBackendCollectionErrorIsVisibleUntilRecovery(t *testing.T) {
	m := backendTestModel(t)
	m.width, m.height = 100, 24
	m.activateBackend(collect.BackendHerdr)
	next, _ := m.Update(snapshotMsg{backend: collect.BackendHerdr, err: errors.New("Herdr unavailable\nsecond session failed")})
	m = next.(Model)
	if view := m.View(); !strings.Contains(view, "Herdr unavailable") || !strings.Contains(view, "[Herdr]") {
		t.Fatalf("backend error must be visible without hiding tabs:\n%s", view)
	}
	if lines := strings.Count(m.View(), "\n") + 1; lines > m.height {
		t.Fatalf("multiline backend error overflowed viewport: %d > %d", lines, m.height)
	}
	next, _ = m.Update(snapshotMsg{backend: collect.BackendHerdr, snap: collect.Snapshot{Taken: time.Now()}})
	if view := next.(Model).View(); strings.Contains(view, "Herdr unavailable") {
		t.Fatalf("successful refresh left stale backend error:\n%s", view)
	}
}

func TestBackendTabsRestoreIndependentOrdersAndLegacyPreferences(t *testing.T) {
	m := backendTestModel(t)
	saveViewPreferences(m.preferencesPath, viewPreferences{Stacked: true, FlatView: true,
		FlatOrder: []string{"%2", "%1"}, GroupedOrder: []string{"%1", "%2"}})
	m = New()
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	if cmd == nil || m.selectedBackend() != collect.BackendHerdr || SelectedBackend() != collect.BackendHerdr {
		t.Fatal("Tab did not select and save Herdr")
	}
	m.flatOrder, m.groupedOrder = []string{"herdr:s:p2", "herdr:s:p1"}, []string{"herdr:s:p1", "herdr:s:p2"}
	m.persistViewPreferences()
	reopened := New()
	if !reopened.stacked || !reopened.flatView || !slices.Equal(reopened.flatOrder, m.flatOrder) ||
		!slices.Equal(reopened.groupedOrder, m.groupedOrder) {
		t.Fatal("Herdr tab did not restore its layout and orders")
	}
	next, _ = reopened.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	reopened = next.(Model)
	if reopened.selectedBackend() != collect.BackendTmux ||
		!slices.Equal(reopened.flatOrder, []string{"%2", "%1"}) ||
		!slices.Equal(reopened.groupedOrder, []string{"%1", "%2"}) {
		t.Fatal("returning to tmux lost the legacy saved orders")
	}
	if got := NewWithBackend(collect.BackendHerdr); got.selectedBackend() != collect.BackendHerdr ||
		!slices.Equal(got.flatOrder, []string{"herdr:s:p2", "herdr:s:p1"}) {
		t.Fatal("explicit backend override lost its remembered order")
	}
}

func TestSelectedBackendDefaultsToTmuxForInvalidOrMissingPreferences(t *testing.T) {
	m := backendTestModel(t)
	for _, backend := range []collect.Backend{"", "bad", collect.BackendTmux, collect.BackendHerdr} {
		saveViewPreferences(m.preferencesPath, viewPreferences{Backend: backend})
		want := collect.BackendTmux
		if backend == collect.BackendHerdr {
			want = backend
		}
		if got := SelectedBackend(); got != want || New().selectedBackend() != want {
			t.Fatalf("backend %q restored as %q, want %q", backend, got, want)
		}
	}
}

func TestBackendSwitchRejectsOldSnapshotsIncludingReturningToSameTab(t *testing.T) {
	m := backendTestModel(t)
	a := collect.Agent{ID: "%1", Kind: collect.KindInteractive, Life: collect.LifeWorking, Project: "/p", TmuxTarget: "s:0.0"}
	m.snap = projSnap(a)
	m.rebuildRows()
	m.filter, m.notice = "old filter", "old notice"
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	if len(m.rows) != 0 || len(m.snap.Agents) != 0 || !m.loading || m.filter != "" || m.notice != "" {
		t.Fatal("switch retained transient state or old rows")
	}
	next, _ = m.Update(snapshotMsg{snap: projSnap(a), err: errors.New("old error"), backend: collect.BackendTmux})
	m = next.(Model)
	if !m.loading || m.err != nil || len(m.rows) != 0 {
		t.Fatal("old backend snapshot was applied")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = next.(Model)
	next, _ = m.Update(snapshotMsg{snap: projSnap(a), backend: collect.BackendTmux})
	m = next.(Model)
	if !m.loading || len(m.rows) != 0 {
		t.Fatal("old generation was accepted after returning to tmux")
	}
	next, _ = m.Update(snapshotMsg{snap: projSnap(a), backend: collect.BackendTmux, generation: m.generation})
	if got := next.(Model); got.loading || got.selectedAgent() == nil {
		t.Fatal("current snapshot was rejected")
	}
}

func TestBackendNotesPruneOnlyActiveNamespaceAndKeepInactiveRequests(t *testing.T) {
	for _, backend := range []collect.Backend{collect.BackendTmux, collect.BackendHerdr} {
		t.Run(string(backend), func(t *testing.T) {
			m := backendTestModel(t)
			m.backend = backend
			active, inactive := "%old", "herdr:s:old"
			if backend == collect.BackendHerdr {
				active, inactive = inactive, active
			}
			store := m.notesStore
			store.Set(active, "expired")
			store.Set(inactive, "keep")
			token, err := store.Begin(inactive)
			if err != nil {
				t.Fatal(err)
			}
			store.Set("live", "live note")
			m.snap.LivePaneIDs = map[string]bool{"live": true}
			m.pruneNotes()
			got := notes.Open(store.Path())
			if got.Get(active) != "" || got.Get(inactive) != "keep" || got.Get("live") != "live note" ||
				got.Generation(inactive).Token != token {
				t.Fatal("pruning deleted the inactive backend or retained an expired active note")
			}
		})
	}
}

func TestBackendActionsIgnoreResultsFromPreviousGeneration(t *testing.T) {
	m := backendTestModel(t)
	m.backend, m.generation = collect.BackendHerdr, 3
	m.notice = "current notice"
	for _, msg := range []tea.Msg{
		sentMsg{backend: collect.BackendTmux, target: "%1"},
		killedMsg{backend: collect.BackendHerdr, generation: 1, target: "p1"},
		renamedWindowMsg{backend: collect.BackendHerdr, generation: 1, title: "old"},
		launchedMsg{backend: collect.BackendHerdr, generation: 1, path: "/old"},
	} {
		next, cmd := m.Update(msg)
		m = next.(Model)
		if m.notice != "current notice" || cmd != nil {
			t.Fatalf("stale %T changed current tab", msg)
		}
	}
}

func TestBackendTabsFitDashboardSizes(t *testing.T) {
	m := backendTestModel(t)
	m.loading = false
	m.snap = projSnap(collect.Agent{ID: "%1", Name: "agent", Kind: collect.KindInteractive,
		Life: collect.LifeWorking, Project: "/p", TmuxTarget: "s:0.0", ActivityAt: time.Now()})
	m.rebuildRows()
	for _, backend := range []collect.Backend{collect.BackendTmux, collect.BackendHerdr} {
		m.backend = backend
		for _, stacked := range []bool{false, true} {
			m.stacked = stacked
			for _, size := range [][2]int{{20, 5}, {40, 12}, {100, 24}} {
				m.width, m.height = size[0], size[1]
				view := m.View()
				for _, line := range strings.Split(view, "\n") {
					if lipgloss.Width(line) > m.width {
						t.Fatalf("width overflows %v: %q", size, line)
					}
				}
				if lipgloss.Height(view) > m.height {
					t.Fatalf("height overflows %v stacked=%v (%d): %q", size, stacked, lipgloss.Height(view), view)
				}
				if !strings.Contains(strings.Split(view, "\n")[0], "tmux") || !strings.Contains(view, "Herdr") {
					t.Fatal("backend tabs are not visible")
				}
			}
		}
	}
}

func TestBackendNotesKeepAllNotesWhenInventoryFails(t *testing.T) {
	m := backendTestModel(t)
	m.notesStore.Set("%old", "keep")
	m.notesStore.Set("herdr:s:old", "keep")
	m.pruneNotes()
	for _, id := range []string{"%old", "herdr:s:old"} {
		if m.notesStore.Get(id) != "keep" {
			t.Fatalf("failed inventory pruned %s", id)
		}
	}
}

func TestBackendPreferenceSavePreservesInactiveOrderAfterEmptySnapshot(t *testing.T) {
	m := backendTestModel(t)
	m.herdrFlatOrder = []string{"herdr:s:p1"}
	m.snap.LivePaneIDs = map[string]bool{}
	next, _ := m.Update(snapshotMsg{snap: m.snap})
	m = next.(Model)
	data, err := os.ReadFile(filepath.Clean(m.preferencesPath))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "herdr:s:p1") {
		t.Fatal("tmux snapshot pruned inactive order")
	}
}
