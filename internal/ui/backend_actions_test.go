package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kyhan/amux/internal/collect"
	"github.com/kyhan/amux/internal/notegen"
)

func TestHerdrComposeSubmitsToAgentCapturedBeforeSelectionChanges(t *testing.T) {
	m, _, root := herdrActionFixture(t)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = replaceSelectedHerdrAgent(next.(Model))
	text := "literal ; $HOME #{pane_id}"
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)})
	next, cmd := next.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("compose did not dispatch")
	}
	next, _ = next.(Model).Update(cmd())
	if got := readActionFile(t, root, "captured-pane.submitted"); got != text {
		t.Fatalf("submitted %q", got)
	}
	if !strings.Contains(next.(Model).notice, "sent to") {
		t.Fatal("send did not succeed")
	}
}

func TestHerdrKillClosesCapturedPaneAndLeavesOtherSelection(t *testing.T) {
	m, _, root := herdrActionFixture(t)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = replaceSelectedHerdrAgent(next.(Model))
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if cmd == nil {
		t.Fatal("kill did not dispatch")
	}
	next, _ = next.(Model).Update(cmd())
	if _, err := os.Stat(filepath.Join(root, "captured-pane.live")); !os.IsNotExist(err) {
		t.Fatalf("captured pane still live: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "other.live")); err != nil {
		t.Fatal("other pane was closed")
	}
	if next.(Model).selectedAgent() == nil || next.(Model).selectedAgent().PaneID != "other" {
		t.Fatal("kill removed new selection")
	}
}

func TestHerdrTitleActionsUseCapturedTab(t *testing.T) {
	for _, key := range []string{"edit", "I", "o"} {
		t.Run(key, func(t *testing.T) {
			m, a, root := herdrActionFixture(t)
			m.notesStore.Set(a.ID, "note")
			want := "stay"
			var next tea.Model
			var cmd tea.Cmd
			if key == "edit" {
				want = "saved title"
				next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
				m = replaceSelectedHerdrAgent(next.(Model))
				next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
				next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyCtrlU})
				next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(want)})
				next, cmd = next.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
			} else {
				if key == "o" {
					want = "start"
				}
				next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
				next = replaceSelectedHerdrAgent(next.(Model))
			}
			if cmd == nil {
				t.Fatal("title action did not dispatch")
			}
			next, _ = next.(Model).Update(cmd())
			if got := readActionFile(t, root, "captured-tab.title"); got != want {
				t.Fatalf("title=%q, want %q; notice %s", got, want, next.(Model).notice)
			}
			if got := readActionFile(t, root, "other-tab.title"); got != "untouched" {
				t.Fatalf("other tab title=%q", got)
			}
		})
	}
}

func TestHerdrMoveActionCarriesCapturedNativeTargetsAndPopup(t *testing.T) {
	m, a, _ := herdrActionFixture(t)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = replaceSelectedHerdrAgent(next.(Model))
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	action := next.(Model).Chosen()
	if action == nil || action.Backend != collect.BackendHerdr || action.PaneID != a.PaneID ||
		action.TerminalID != a.TerminalID || action.WorkspaceID != a.WorkspaceID || action.TabID != a.TabID ||
		action.Session != a.Session || !action.Popup {
		t.Fatalf("action=%+v", action)
	}
}

func TestManageActionUsesSelectedBackend(t *testing.T) {
	m := backendTestModel(t)
	m.backend = collect.BackendHerdr
	m.snap = projSnap(collect.Agent{ID: "daemon", Kind: collect.KindBackground, Life: collect.LifeWorking,
		Project: "/p", CWD: "/p", Tool: "claude", Live: true})
	m.rebuildRows()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	action := next.(Model).Chosen()
	if action == nil || action.Kind != "manage" || action.Backend != collect.BackendHerdr {
		t.Fatalf("action=%+v", action)
	}
}

func TestHerdrLaunchCreatesAndRunsInCapturedWorkspace(t *testing.T) {
	m, _, root := herdrActionFixture(t)
	next, _ := m.openLaunchMenu()
	m = replaceSelectedHerdrAgent(next.(Model))
	m.launchMenu.tools = true
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("launch did not dispatch")
	}
	next, _ = next.(Model).Update(cmd())
	if got := readActionFile(t, root, "created.command"); got != "claude" {
		t.Fatalf("launched %q", got)
	}
	if next.(Model).launchMenu.open {
		t.Fatalf("launch failed: %s", next.(Model).notice)
	}
}

func TestHerdrNotesCaptureOriginalPaneBeforeLaunchingWorker(t *testing.T) {
	m, a, _ := herdrActionFixture(t)
	var captured collect.Agent
	m.noteLauncher = func(request notegen.Request) (func() error, error) {
		captured = request.Agent
		return func() error { return nil }, nil
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	m = replaceSelectedHerdrAgent(next.(Model))
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	if cmd == nil || captured.ID != a.ID || captured.Session != a.Session || captured.TmuxTarget != "" ||
		strings.Join(captured.Preview, "\n") != "CAPTURED_OUTPUT" {
		t.Fatalf("worker agent=%+v; notice %s", captured, next.(Model).notice)
	}
}

func TestHerdrLaunchRejectsImplicitFocusWithoutContext(t *testing.T) {
	_, _, root := herdrActionFixture(t)
	err := runLaunch(context.Background(), launchTarget{backend: collect.BackendHerdr}, root, "claude")
	if err == nil {
		t.Fatal("launch accepted implicit focused workspace")
	}
	if _, err := os.Stat(filepath.Join(root, "created.live")); !os.IsNotExist(err) {
		t.Fatal("created tab without explicit context")
	}
}

func TestHerdrLaunchCreatesNamedWorkspaceWithinCapturedSession(t *testing.T) {
	m, _, root := herdrActionFixture(t)
	next, _ := m.openLaunchMenu()
	m = next.(Model)
	m.launchMenu.target.workspace = ""
	m.launchMenu.tools = true
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("launch did not dispatch")
	}
	next, _ = next.(Model).Update(cmd())
	if got := readActionFile(t, root, "created.command"); got != "claude" {
		t.Fatalf("launched %q", got)
	}
	if next.(Model).launchMenu.open {
		t.Fatal("named workspace launch did not complete")
	}
}

func TestHerdrNoteCaptureFailureDoesNotLaunchWorkerOrUseStalePreview(t *testing.T) {
	m, a, root := herdrActionFixture(t)
	if err := os.Remove(filepath.Join(root, "captured-pane.output")); err != nil {
		t.Fatal(err)
	}
	called := false
	m.noteLauncher = func(notegen.Request) (func() error, error) {
		called = true
		return func() error { return nil }, nil
	}
	a.Preview = []string{"stale preview"}
	next, cmd := m.fetchAutoTitle(a)
	if cmd != nil || called || !strings.Contains(next.(Model).notice, "capture Herdr pane") {
		t.Fatal("capture failure was hidden or launched a worker")
	}
	if state := next.(Model).notesStore.Generation(a.ID); state.Token != "" {
		t.Fatal("failed capture created generation request")
	}
}

func TestHerdrPurposeTitleUsesCapturedPaneOutput(t *testing.T) {
	_, a, root := herdrActionFixture(t)
	input := filepath.Join(root, "codex-input")
	t.Setenv("HERDR_TITLE_INPUT", input)
	if err := os.WriteFile(filepath.Join(root, "codex"), []byte("#!/bin/sh\ncat > \"$HERDR_TITLE_INPUT\"\nprintf 'title\\n'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	title, err := inferPurposeTitle(context.Background(), a)
	if err != nil || title != "title" {
		t.Fatalf("purpose title=%q err=%v", title, err)
	}
	prompt := readActionFile(t, root, "codex-input")
	if !strings.Contains(prompt, "CAPTURED_OUTPUT") || strings.Contains(prompt, "WRONG_OUTPUT") {
		t.Fatal("purpose inference captured a different pane")
	}
}
