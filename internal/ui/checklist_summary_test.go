package ui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kyhan/amux/internal/collect"
)

func openChecklistSummary(m Model) (Model, tea.Cmd) {
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	next, cmd := next.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	return next.(Model), cmd
}

func TestChecklistSummaryRunsOnlyOnOL(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("PATH", home+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := os.WriteFile(filepath.Join(home, "codex"), []byte("#!/bin/sh\ncat >/dev/null\nprintf '결제 흐름 검토\\n환불 검증 진행 중\\n'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	agent := collect.Agent{ID: "a", Tool: "claude", Kind: collect.KindInteractive, Project: "/p",
		Intent: "Review billing", Preview: []string{"refund validation"},
		Checklist: []collect.ChecklistItem{{Content: "Recorded plan", Status: collect.ChecklistActive}}}
	m := Model{snap: projSnap(agent), preferencesPath: filepath.Join(home, "prefs.json")}
	m.rebuildRows()
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = next.(Model)
	if cmd != nil || len(m.checklistSummaries) != 0 {
		t.Fatal("opening l alone must never generate a summary")
	}
	m, cmd = openChecklistSummary(m)
	if cmd == nil || m.hideDetail || !m.showChecklist || m.showNoteMenu {
		t.Fatal("o-l must close the menu, open the plan panel and start generation")
	}
	if !strings.Contains(m.renderChecklist(90, 20), "summarizing") {
		t.Fatal("pending summary must be visible")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	out := m.renderChecklist(90, 20)
	if !strings.Contains(out, "환불 검증 진행 중") || !strings.Contains(out, "Codex summary") {
		t.Fatalf("summary was not rendered:\n%s", out)
	}
	if len(m.selectedAgent().Checklist) != 1 || m.notesStore != nil {
		t.Fatal("summary must not replace the source plan or use the note store")
	}
	for i := 0; i < 2; i++ {
		next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
		m = next.(Model)
		if cmd != nil {
			t.Fatal("reopening l must reuse the summary without invoking Codex")
		}
	}
	if !strings.Contains(m.renderChecklist(90, 20), "환불 검증 진행 중") {
		t.Fatal("summary must survive closing and reopening l")
	}
	previous := m.checklistSummaries["a"].request
	m, cmd = openChecklistSummary(m)
	defer m.cancelChecklistSummaries()
	if cmd == nil || m.checklistSummaries["a"].request == previous {
		t.Fatal("repeating o-l must start a fresh summary")
	}
}

func TestChecklistSummaryExcludesOMO(t *testing.T) {
	m := Model{snap: projSnap(collect.Agent{ID: "omo", Tool: "omo", Kind: collect.KindInteractive, Project: "/p",
		Checklist: []collect.ChecklistItem{{Content: "Native plan", Status: collect.ChecklistActive}}}),
		showChecklist: true, preferencesPath: filepath.Join(t.TempDir(), "prefs.json")}
	m.rebuildRows()
	m, cmd := openChecklistSummary(m)
	if cmd != nil || len(m.checklistSummaries) != 0 || !strings.Contains(m.renderChecklist(80, 20), "Native plan") {
		t.Fatal("OmO must retain its native plan without invoking Codex")
	}
}

func TestChecklistSummaryRejectsStaleResultsAndShowsFailure(t *testing.T) {
	m := Model{snap: projSnap(collect.Agent{ID: "a", Tool: "codex", Kind: collect.KindInteractive, Project: "/p"}),
		showChecklist: true, preferencesPath: filepath.Join(t.TempDir(), "prefs.json")}
	m.rebuildRows()
	m, _ = openChecklistSummary(m)
	first := m.checklistSummaries["a"].request
	m, _ = openChecklistSummary(m)
	second := m.checklistSummaries["a"].request
	defer m.cancelChecklistSummaries()
	m.applyChecklistSummary(checklistSummaryMsg{id: "a", request: first, text: "Wrong result"})
	if m.checklistSummaries["a"].text != "" {
		t.Fatal("stale response was accepted")
	}
	m.checklistSummaries["a"].cancel()
	m.applyChecklistSummary(checklistSummaryMsg{id: "a", request: second, err: errors.New("provider offline")})
	if !strings.Contains(m.renderChecklist(80, 20), "provider offline") {
		t.Fatal("generation failure must be visible")
	}
	m, _ = openChecklistSummary(m)
	request := m.checklistSummaries["a"].request
	m.generation++
	m.pruneChecklistSummaries()
	m.applyChecklistSummary(checklistSummaryMsg{id: "a", request: request, text: "Cancelled result"})
	if _, ok := m.checklistSummaries["a"]; ok {
		t.Fatal("result from a prior backend generation was accepted")
	}
}
