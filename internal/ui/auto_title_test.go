package ui

import (
	"errors"
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kyhan/amux/internal/collect"
	"github.com/kyhan/amux/internal/notegen"
	"github.com/kyhan/amux/internal/notes"
)

func autoTitleTestModel(t *testing.T) Model {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	m := Model{notesStore: notes.Load(), width: 100, height: 24, snap: projSnap(
		collect.Agent{ID: "a", Kind: collect.KindInteractive, Life: collect.LifeWorking, Project: "/p"},
		collect.Agent{ID: "b", Kind: collect.KindInteractive, Life: collect.LifeWorking, Project: "/p"},
	), noteLauncher: func(notegen.Request) (func() error, error) {
		return func() error { return nil }, nil
	}}
	m.rebuildRows()
	return m
}

func generateNote(m Model) (Model, tea.Cmd) {
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	next, cmd := next.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	return next.(Model), cmd
}

func completeNote(t *testing.T, m Model, request uint64, title string, err error) Model {
	t.Helper()
	owner := m.autoTitleRequests[request]
	if _, saveErr := notes.Open(m.notesStore.Path()).Complete(owner.agentID, owner.token, title, err); saveErr != nil {
		t.Fatal(saveErr)
	}
	next, _ := m.Update(autoTitleMsg{agentID: owner.agentID, request: request, err: err})
	return next.(Model)
}

func TestAutoTitleSavesOriginalAgentWhileNavigationContinues(t *testing.T) {
	// Given
	m := autoTitleTestModel(t)
	owner := m.selectedAgent().ID
	m, cmd := generateNote(m)
	request := m.noteRequest
	if cmd == nil || m.fetchingNote || m.naming || m.showNoteMenu {
		t.Fatal("generation must leave the dashboard usable")
	}
	// When
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = next.(Model)
	selected := m.selectedAgent().ID
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = completeNote(t, next.(Model), request, "generated title", nil)
	// Then
	if !m.showHelp || selected == owner || m.selectedAgent().ID != selected || m.naming ||
		m.notesStore.Get(owner) != "generated title" || m.notesStore.Get(selected) != "" {
		t.Fatal("completion interrupted navigation or saved to the wrong agent")
	}
}

func TestAutoTitleRepeatedRequestsApplyOnlyNewestResult(t *testing.T) {
	for _, order := range [][]int{{0, 1, 2}, {2, 1, 0}, {1, 2, 0}} {
		t.Run(fmt.Sprint(order), func(t *testing.T) {
			// Given
			m := autoTitleTestModel(t)
			owner := m.selectedAgent().ID
			var requests []uint64
			for range 3 {
				m, _ = generateNote(m)
				requests = append(requests, m.noteRequest)
			}
			// When
			for _, i := range order {
				title := "old"
				if i == 2 {
					title = "newest"
				}
				m = completeNote(t, m, requests[i], title, nil)
			}
			// Then
			if m.notesStore.Get(owner) != "newest" || len(m.autoTitleRequests) != 0 {
				t.Fatal("older completion replaced the newest result")
			}
		})
	}
}

func TestAutoTitleSupportsDifferentAgentsAndFiltering(t *testing.T) {
	// Given
	m := autoTitleTestModel(t)
	first := m.selectedAgent().ID
	m, _ = generateNote(m)
	firstRequest := m.noteRequest
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = next.(Model)
	second := m.selectedAgent().ID
	m, _ = generateNote(m)
	secondRequest := m.noteRequest
	// When
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	m = completeNote(t, next.(Model), secondRequest, "second", nil)
	m = completeNote(t, m, firstRequest, "first", nil)
	// Then
	if !m.filtering || m.filter != "p" || m.notesStore.Get(first) != "first" || m.notesStore.Get(second) != "second" {
		t.Fatal("completion interrupted filtering or crossed note ownership")
	}
}

func TestAutoTitleManualCancellationDiscardsLateResults(t *testing.T) {
	for _, action := range []string{"esc", "i", "I", "refresh"} {
		t.Run(action, func(t *testing.T) {
			// Given
			m := autoTitleTestModel(t)
			owner := m.selectedAgent().ID
			m.notesStore.Set(owner, "manual")
			m, _ = generateNote(m)
			request := m.autoTitleRequests[m.noteRequest]
			// When
			if action == "refresh" {
				next, _ := m.Update(snapshotMsg{snap: collect.Snapshot{LivePaneIDs: map[string]bool{}}})
				m = next.(Model)
			} else {
				key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(action)}
				if action == "esc" {
					key.Type = tea.KeyEsc
				}
				next, _ := m.Update(key)
				m = next.(Model)
			}
			applied, err := notes.Open(m.notesStore.Path()).Complete(owner, request.token, "late", nil)
			// Then
			if err != nil || applied {
				t.Fatalf("cancelled completion applied=%v err=%v", applied, err)
			}
		})
	}
}

func TestAutoTitleQuitDoesNotCancelDurableRequest(t *testing.T) {
	// Given
	m := autoTitleTestModel(t)
	owner := m.selectedAgent().ID
	m, _ = generateNote(m)
	request := m.autoTitleRequests[m.noteRequest]
	// When
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	applied, err := notes.Open(m.notesStore.Path()).Complete(owner, request.token, "after quit", nil)
	// Then
	if !next.(Model).quit || cmd == nil || err != nil || !applied || notes.Load().Get(owner) != "after quit" {
		t.Fatalf("quit cancelled durable generation: applied=%v err=%v", applied, err)
	}
}

func TestAutoTitleFailurePreservesUserText(t *testing.T) {
	// Given
	m := autoTitleTestModel(t)
	owner := m.selectedAgent().ID
	m.notesStore.Set(owner, "manual")
	m, _ = generateNote(m)
	// When
	m = completeNote(t, m, m.noteRequest, "", errors.New("offline"))
	// Then
	if m.notesStore.Get(owner) != "manual" || m.naming || m.notice == "" {
		t.Fatal("failure changed manual text or was hidden")
	}
}
