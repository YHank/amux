package ui

import (
	"errors"
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kyhan/amux/internal/collect"
	"github.com/kyhan/amux/internal/notes"
)

func autoTitleTestModel(t *testing.T) Model {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	m := Model{notesStore: notes.Load(), width: 100, height: 24, snap: projSnap(
		collect.Agent{ID: "a", Kind: collect.KindInteractive, Life: collect.LifeWorking, Project: "/p"},
		collect.Agent{ID: "b", Kind: collect.KindInteractive, Life: collect.LifeWorking, Project: "/p"},
	)}
	m.rebuildRows()
	return m
}

func TestAutoTitleSavesOriginalAgentWhileNavigationContinues(t *testing.T) {
	// Given
	m := autoTitleTestModel(t)
	owner := m.selectedAgent().ID
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	m = next.(Model)
	request := m.noteRequest
	defer m.cancelAutoTitles("")
	if cmd == nil || m.fetchingNote || m.naming {
		t.Fatal("o must launch a command without entering an input mode")
	}
	// When: navigate, open help and receive a result without Enter.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = next.(Model)
	selected := m.selectedAgent().ID
	if selected == owner {
		t.Fatal("selection did not move while a request was pending")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	next, _ = next.(Model).Update(snapshotMsg{snap: m.snap})
	next, _ = next.(Model).Update(autoTitleMsg{agentID: owner, request: request, title: "generated title"})
	m = next.(Model)
	// Then
	if !m.showHelp || m.selectedAgent().ID != selected || m.naming {
		t.Fatal("completion interrupted the user's current UI")
	}
	if m.notesStore.Get(owner) != "generated title" || m.notesStore.Get(selected) != "" || len(m.autoTitleRequests) != 0 {
		t.Fatal("completion must save only the original agent")
	}
}

func TestAutoTitleRepeatedRequestsApplyOnlyNewestResult(t *testing.T) {
	for _, order := range [][]int{{0, 1, 2}, {2, 1, 0}, {1, 2, 0}} {
		t.Run(fmt.Sprint(order), func(t *testing.T) {
			// Given
			m := autoTitleTestModel(t)
			owner := m.selectedAgent().ID
			var ids []uint64
			for range 3 {
				next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
				m = next.(Model)
				if cmd == nil {
					t.Fatal("each o must emit its own asynchronous request")
				}
				ids = append(ids, m.noteRequest)
			}
			defer m.cancelAutoTitles("")
			if len(m.autoTitleRequests) != 3 {
				t.Fatal("repeated o requests must coexist")
			}
			// When
			for _, i := range order {
				title := "old"
				if i == 2 {
					title = "newest"
				}
				next, _ := m.Update(autoTitleMsg{agentID: owner, request: ids[i], title: title})
				m = next.(Model)
			}
			// Then
			if m.notesStore.Get(owner) != "newest" || len(m.autoTitleRequests) != 0 || len(m.autoTitleLatest) != 0 {
				t.Fatal("an out-of-order older result replaced the newest request")
			}
		})
	}
}

func TestAutoTitleSupportsDifferentAgentsAndFiltering(t *testing.T) {
	// Given
	m := autoTitleTestModel(t)
	first := m.selectedAgent().ID
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	m = next.(Model)
	firstRequest := m.noteRequest
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = next.(Model)
	second := m.selectedAgent().ID
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	m = next.(Model)
	secondRequest := m.noteRequest
	defer m.cancelAutoTitles("")
	// When: filter input continues while both results arrive.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	next, _ = next.(Model).Update(autoTitleMsg{agentID: second, request: secondRequest, title: "second"})
	next, _ = next.(Model).Update(autoTitleMsg{agentID: first, request: firstRequest, title: "first"})
	m = next.(Model)
	// Then
	if !m.filtering || m.filter != "p" || m.notesStore.Get(first) != "first" || m.notesStore.Get(second) != "second" {
		t.Fatal("concurrent completion interrupted filtering or crossed note ownership")
	}
}

func TestAutoTitleCancellationDiscardsLateResults(t *testing.T) {
	for _, action := range []string{"esc", "i", "I", "q", "refresh"} {
		t.Run(action, func(t *testing.T) {
			// Given
			m := autoTitleTestModel(t)
			owner := m.selectedAgent().ID
			m.notesStore.Set(owner, "manual")
			next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
			m = next.(Model)
			request := m.noteRequest
			defer m.cancelAutoTitles("")
			// When
			if action == "refresh" {
				next, _ = m.Update(snapshotMsg{snap: collect.Snapshot{LivePaneIDs: map[string]bool{}}})
			} else {
				key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(action)}
				if action == "esc" {
					key = tea.KeyMsg{Type: tea.KeyEsc}
				}
				next, _ = m.Update(key)
			}
			next, _ = next.(Model).Update(autoTitleMsg{agentID: owner, request: request, title: "late"})
			m = next.(Model)
			// Then
			want := "manual"
			if action == "I" || action == "refresh" {
				want = ""
			}
			if m.notesStore.Get(owner) != want || len(m.autoTitleRequests) != 0 {
				t.Fatal("late result recreated or replaced a cancelled note")
			}
		})
	}
}

func TestAutoTitleFailureAndChangedNotePreserveUserText(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "failure", true: "changed"}[changed], func(t *testing.T) {
			// Given
			m := autoTitleTestModel(t)
			owner := m.selectedAgent().ID
			m.notesStore.Set(owner, "manual")
			next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
			m = next.(Model)
			msg := autoTitleMsg{agentID: owner, request: m.noteRequest, title: "generated"}
			want := "manual"
			if changed {
				want = "new manual"
				m.notesStore.Set(owner, want)
			} else {
				msg.err = errors.New("offline")
			}
			// When
			next, _ = m.Update(msg)
			m = next.(Model)
			// Then
			if m.notesStore.Get(owner) != want || m.naming || len(m.autoTitleRequests) != 0 {
				t.Fatal("completion replaced user text or opened a modal")
			}
			if !changed && m.notice == "" {
				t.Fatal("failure must be visible")
			}
		})
	}
}
