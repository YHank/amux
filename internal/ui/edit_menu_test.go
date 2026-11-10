package ui

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kyhan/amux/internal/collect"
	"github.com/kyhan/amux/internal/notes"
)

func TestWindowTitlePopupRenamesOnlyCapturedWindow(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux is required for the isolated rename boundary test")
	}
	dir, err := os.MkdirTemp("/tmp", "amux-rename-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Error(err)
		}
	})
	socket := filepath.Join(dir, "tmux.sock")
	t.Setenv("TMUX", socket+",0,0")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	call := func(args ...string) string {
		t.Helper()
		out, err := exec.CommandContext(ctx, tmux, append([]string{"-S", socket}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("tmux: %v: %s", err, out)
		}
		return strings.TrimSpace(string(out))
	}
	pane := call("-f", "/dev/null", "new-session", "-d", "-s", "rename-test", "-n", "original", "-P", "-F", "#{pane_id}", "/bin/sh")
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 3*time.Second)
		defer stop()
		if out, err := exec.CommandContext(cleanup, tmux, "-S", socket, "kill-server").CombinedOutput(); err != nil {
			t.Errorf("cleanup tmux: %v: %s", err, out)
		}
	})
	other := call("new-window", "-t", "=rename-test:", "-n", "other", "-P", "-F", "#{pane_id}", "/bin/sh")
	a := collect.Agent{ID: pane, Kind: collect.KindInteractive, Life: collect.LifeIdle,
		Project: dir, Session: "rename-test", Window: "original", TmuxTarget: "rename-test:0.0"}
	m := Model{snap: projSnap(a), width: 80, height: 24}
	m.rebuildRows()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	const title = "-새 제목 ; $HOME #{session_name}"
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(title)})
	next, cmd := next.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("title save did not issue a command")
	}
	next, _ = next.(Model).Update(cmd())
	if got := call("display-message", "-p", "-t", pane, "#{window_name}"); got != title {
		t.Fatalf("renamed title = %q, want literal %q", got, title)
	}
	if got := call("display-message", "-p", "-t", other, "#{window_name}"); got != "other" {
		t.Fatalf("unselected window was renamed: %q", got)
	}
	if next.(Model).editMenu.open {
		t.Fatal("successful rename left popup open")
	}
}

func TestEditPopupChoosesNoteWithoutChangingWindowTitle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	a := collect.Agent{ID: "%10", Kind: collect.KindInteractive, Life: collect.LifeIdle,
		Project: "/p", Session: "work", Window: "original", TmuxTarget: "work:0.0"}
	store := notes.Load()
	store.Set(a.ID, "existing note")
	m := Model{snap: projSnap(a), notesStore: store, width: 80, height: 24}
	m.rebuildRows()

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	m = next.(Model)
	if cmd != nil || m.naming || !strings.Contains(m.View(), "t  window title") || !strings.Contains(m.View(), "i  note") {
		t.Fatal("first i must offer title and note without starting an editor or external work")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("수정 메모")})
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if got := notes.Load().Get(a.ID); got != "수정 메모" {
		t.Fatalf("saved note = %q", got)
	}
	if m.selectedAgent().Window != "original" || m.naming {
		t.Fatal("editing a note changed the window or left the editor open")
	}
}

func TestWindowTitlePopupKeepsDraftOnFailureAndCancelsSafely(t *testing.T) {
	a := collect.Agent{ID: "%10", Kind: collect.KindInteractive, Life: collect.LifeIdle,
		Project: "/p", Session: "work", Window: "original", TmuxTarget: "work:0.0"}
	m := Model{snap: projSnap(a), width: 80, height: 24}
	m.rebuildRows()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("새 제목")})
	next, cmd := next.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("saving a valid title did not produce a command")
	}
	next, _ = next.(Model).Update(renamedWindowMsg{target: a.ID, err: errors.New("pane closed")})
	m = next.(Model)
	if view := m.View(); !strings.Contains(view, "새 제목") || !strings.Contains(view, "pane closed") {
		t.Fatalf("failed save lost draft or error: %s", view)
	}
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil || next.(Model).editMenu.open || next.(Model).selectedAgent().Window != "original" {
		t.Fatal("escape must cancel without renaming")
	}
}

func TestEditPopupClosesWhenSourcePaneDisappears(t *testing.T) {
	a := mkAgent("a", collect.LifeWorking)
	m := Model{snap: projSnap(a), width: 80, height: 24}
	m.rebuildRows()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	next, _ = next.(Model).Update(snapshotMsg{snap: projSnap(mkAgent("b", collect.LifeIdle))})
	next, cmd := next.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	if cmd != nil || next.(Model).editMenu.open {
		t.Fatal("popup could edit a different agent after refresh")
	}
}

func TestEditPopupFitsViewportWithLongUnicodeInput(t *testing.T) {
	for _, size := range []struct{ w, h int }{{20, 5}, {40, 10}, {120, 32}} {
		for _, mode := range []string{"menu", "title", "note", "suggestion", "fetch"} {
			m := Model{width: size.w, height: size.h}
			input := strings.Repeat("프로젝트 경로 ", 40)
			switch mode {
			case "menu":
				m.editMenu.open = true
			case "title":
				m.editMenu = editMenuState{open: true, title: true, input: input}
			case "note":
				m.naming, m.noteInput = true, input
			case "suggestion":
				m.suggestingNote, m.suggestedNote = true, input
			case "fetch":
				m.fetchingNote = true
			}
			view := m.View()
			if lipgloss.Width(view) > size.w || lipgloss.Height(view) > size.h {
				t.Fatalf("%s overflows %dx%d: %dx%d", mode, size.w, size.h, lipgloss.Width(view), lipgloss.Height(view))
			}
			if (mode == "title" || mode == "note") && !strings.Contains(view, "▏") {
				t.Fatalf("%s hides the input cursor at %dx%d: %q", mode, size.w, size.h, view)
			}
		}
	}
}
