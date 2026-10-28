package ui

import (
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestAutoTitleAllowsKeysWhileProcessIsPending(t *testing.T) {
	// Given: the external process signals readiness and waits on an exact gate.
	m := autoTitleTestModel(t)
	dir := t.TempDir()
	readyPath, gatePath := filepath.Join(dir, "ready"), filepath.Join(dir, "gate")
	for _, path := range []string{readyPath, gatePath} {
		if err := syscall.Mkfifo(path, 0600); err != nil {
			t.Fatal(err)
		}
	}
	ready, err := os.OpenFile(readyPath, os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer ready.Close()
	gate, err := os.OpenFile(gatePath, os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer gate.Close()
	t.Setenv("TITLE_READY", readyPath)
	t.Setenv("TITLE_GATE", gatePath)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	script := "#!/bin/sh\ncat > /dev/null\nprintf R > \"$TITLE_READY\"\nread token < \"$TITLE_GATE\"\nprintf 'AUTO_TITLE\\n'\n"
	if err := os.WriteFile(filepath.Join(dir, "codex"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	started := make(chan error, 1)
	go func() {
		_, err := io.ReadFull(ready, make([]byte, 1))
		started <- err
	}()
	owner := m.selectedAgent().ID
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("o must start a command")
	}
	result := make(chan tea.Msg, 1)
	go func() { result <- cmd() }()
	finished := false
	defer func() {
		m.cancelAutoTitles("")
		if !finished {
			select {
			case <-result:
			case <-time.After(5 * time.Second):
				t.Error("cancelled command did not stop")
			}
		}
	}()
	select {
	case err := <-started:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("external process did not signal readiness")
	}
	// When: the provider is blocked, but navigation and another o still work.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = next.(Model)
	if m.selectedAgent().ID == owner {
		t.Fatal("navigation was blocked")
	}
	next, otherCmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	m = next.(Model)
	if otherCmd == nil || len(m.autoTitleRequests) != 2 {
		t.Fatal("a second request was blocked")
	}
	if _, err := gate.WriteString("finish\n"); err != nil {
		t.Fatal(err)
	}
	select {
	case msg := <-result:
		finished = true
		next, _ = m.Update(msg)
		m = next.(Model)
	case <-time.After(5 * time.Second):
		t.Fatal("released command did not complete")
	}
	// Then: no Enter is needed, and the selected other agent is untouched.
	if m.notesStore.Get(owner) != "AUTO_TITLE" || m.notesStore.Get(m.selectedAgent().ID) != "" || m.naming {
		t.Fatal("result was not automatically saved to its original agent")
	}
}
