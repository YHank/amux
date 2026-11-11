package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kyhan/amux/internal/collect"
)

func TestEnterOffersMoveWithoutLeavingList(t *testing.T) {
	a := collect.Agent{ID: "agent", Kind: collect.KindInteractive, Life: collect.LifeWorking,
		Project: "/p", Name: "agent", TmuxTarget: "session:0.1"}
	m := Model{snap: projSnap(a), width: 80, height: 24}
	m.rebuildRows()

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd != nil || m.Chosen() != nil || m.quit || m.moveAction == nil {
		t.Fatal("Enter must offer a choice without moving or quitting")
	}
	for _, size := range [][2]int{{20, 5}, {40, 12}, {100, 24}} {
		m.width, m.height = size[0], size[1]
		view := m.View()
		if lipgloss.Width(view) > m.width || strings.Count(view, "\n")+1 > m.height {
			t.Fatalf("move menu overflows %v", size)
		}
	}
}

func TestMoveMenuDispatchesChosenTarget(t *testing.T) {
	t.Setenv("TMUX", "/tmp/amux-test,1,0")
	for _, tc := range []struct {
		name  string
		keys  []tea.KeyMsg
		popup bool
	}{
		{"default", []tea.KeyMsg{{Type: tea.KeyEnter}}, false},
		{"fullscreen", []tea.KeyMsg{{Type: tea.KeyRunes, Runes: []rune{'f'}}}, false},
		{"popup", []tea.KeyMsg{{Type: tea.KeyRunes, Runes: []rune{'p'}}}, true},
		{"navigation", []tea.KeyMsg{{Type: tea.KeyDown}, {Type: tea.KeyEnter}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := collect.Agent{ID: "agent", Kind: collect.KindInteractive, Life: collect.LifeWorking,
				Project: "/p", Name: "agent", TmuxTarget: "session:0.1"}
			m := Model{snap: projSnap(a)}
			m.rebuildRows()
			next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m = next.(Model)
			// Refreshes must not redirect a pending choice to another agent.
			m.snap = projSnap(collect.Agent{ID: "other", Kind: collect.KindInteractive,
				Life: collect.LifeWorking, Project: "/p", TmuxTarget: "other:0.0"})
			m.rebuildRows()
			var cmd tea.Cmd
			for _, key := range tc.keys {
				next, cmd = m.Update(key)
				m = next.(Model)
			}
			action := m.Chosen()
			if cmd == nil || !m.quit || action == nil || action.TmuxTarget != a.TmuxTarget ||
				action.Popup != tc.popup {
				t.Fatalf("wrong move: action=%+v quit=%v", action, m.quit)
			}
			if tc.popup {
				m = m.Resume()
				if m.Chosen() != nil || m.quit || m.selectedAgent().ID != "other" {
					t.Fatal("popup return must restore the list without replaying the action")
				}
			}
		})
	}
}

func TestMoveMenuCancelKeepsSelection(t *testing.T) {
	a := collect.Agent{ID: "agent", Kind: collect.KindInteractive, Life: collect.LifeWorking,
		Project: "/p", TmuxTarget: "session:0.1"}
	m := Model{snap: projSnap(a)}
	m.rebuildRows()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	next, cmd := next.(Model).Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if cmd != nil || m.Chosen() != nil || m.quit || m.moveAction != nil || m.selectedAgent().ID != a.ID {
		t.Fatal("cancel must return to the same selection without acting")
	}
}

func TestPopupOutsideTmuxDispatchesSelectedPane(t *testing.T) {
	t.Setenv("TMUX", "")
	m := Model{moveAction: &Action{Kind: "attach", TmuxTarget: "session:0.1"}}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	m = next.(Model)
	action := m.Chosen()
	if cmd == nil || !m.quit || action == nil || !action.Popup || action.TmuxTarget != "session:0.1" {
		t.Fatalf("popup outside tmux did not dispatch selected pane: %+v", action)
	}
}
