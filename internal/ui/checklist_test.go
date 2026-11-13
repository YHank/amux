package ui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kyhan/amux/internal/collect"
)

func pressKey(m Model, key rune) Model {
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
	return next.(Model)
}

func TestChecklistAndPreviewShareDetailPanel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	m := Model{preferencesPath: path}

	steps := []struct {
		key                  rune
		hidden, showingCheck bool
	}{
		{'l', false, true},
		{'m', false, false},
		{'m', true, false},
		{'l', false, true},
		{'l', true, true},
	}
	for i, step := range steps {
		m = pressKey(m, step.key)
		if m.hideDetail != step.hidden || m.showChecklist != step.showingCheck {
			t.Fatalf("step %d (%c): hidden=%v checklist=%v, want %v/%v",
				i, step.key, m.hideDetail, m.showChecklist, step.hidden, step.showingCheck)
		}
	}
	if got := loadViewPreferences(path); !got.HideDetail || !got.ShowChecklist {
		t.Fatalf("saved preferences = %#v", got)
	}
}

func TestChecklistRendersInBothLayouts(t *testing.T) {
	now := time.Now()
	agent := collect.Agent{ID: "%1", Name: "omo", Tool: "omo", Kind: collect.KindInteractive,
		Life: collect.LifeWorking, Project: "/p", TmuxTarget: "s:0.0",
		Preview: []string{"PANE-PREVIEW-TEXT"},
		Checklist: []collect.ChecklistItem{
			{Phase: "A", Content: "read the monthly flow", Status: collect.ChecklistDone,
				StartedAt: now.Add(-20 * time.Minute), FinishedAt: now.Add(-13*time.Minute - 2*time.Second)},
			{Phase: "A", Content: "fix the refund total", Status: collect.ChecklistActive, StartedAt: now.Add(-12 * time.Minute)},
			{Phase: "B", Content: "regression check", Status: collect.ChecklistPending},
		}}
	m := Model{snap: projSnap(agent), showChecklist: true, width: 100, height: 24}
	m.rebuildRows()

	for _, stacked := range []bool{false, true} {
		m.stacked = stacked
		view := m.View()
		for _, want := range []string{"1 of 3 done", "fix the refund total", "done · 6m 58s", "running · 12m", "pending", "Background tasks", "no running tasks"} {
			if !strings.Contains(view, want) {
				t.Fatalf("stacked=%v: checklist view lacks %q:\n%s", stacked, want, view)
			}
		}
		if strings.Contains(view, "PANE-PREVIEW-TEXT") {
			t.Fatalf("stacked=%v: checklist must replace the preview", stacked)
		}
		if lipgloss.Height(view) > m.height {
			t.Fatalf("stacked=%v: height %d overflows", stacked, lipgloss.Height(view))
		}
		for _, line := range strings.Split(view, "\n") {
			if lipgloss.Width(line) > m.width {
				t.Fatalf("stacked=%v: line overflows: %q", stacked, line)
			}
		}
	}
}

func TestChecklistKeepsActiveItemVisibleWhenTall(t *testing.T) {
	var items []collect.ChecklistItem
	for i := 0; i < 12; i++ {
		items = append(items, collect.ChecklistItem{Phase: "A", Content: "finished step", Status: collect.ChecklistDone})
	}
	items = append(items, collect.ChecklistItem{Phase: "A", Content: "CURRENT-STEP", Status: collect.ChecklistActive})
	for i := 0; i < 10; i++ {
		items = append(items, collect.ChecklistItem{Phase: "A", Content: "later step", Status: collect.ChecklistPending})
	}
	m := Model{snap: projSnap(collect.Agent{ID: "%1", Name: "omo", Kind: collect.KindInteractive,
		Life: collect.LifeWorking, Project: "/p", Checklist: items})}
	m.rebuildRows()

	for _, h := range []int{10, 14} {
		out := m.renderChecklist(60, h)
		if lipgloss.Height(out) > h {
			t.Fatalf("h=%d: checklist needs %d rows and would be clipped:\n%s", h, lipgloss.Height(out), out)
		}
		for _, want := range []string{"CURRENT-STEP", "12 finished above", "more pending", "Background tasks", "no running tasks"} {
			if !strings.Contains(out, want) {
				t.Fatalf("h=%d: short panel lacks %q:\n%s", h, want, out)
			}
		}
	}
	empty := Model{snap: projSnap(collect.Agent{ID: "%2", Name: "x", Project: "/p"})}
	empty.rebuildRows()
	if !strings.Contains(empty.renderChecklist(60, 14), "no plan recorded") {
		t.Fatal("agent without a plan should say so")
	}
}
