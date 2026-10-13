package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kyhan/amux/internal/collect"
	"github.com/kyhan/amux/internal/notes"
)

func TestPurposeTitleAutomaticallyFetchesAtTwentyRunes(t *testing.T) {
	for _, tc := range []struct {
		title string
		fetch bool
	}{
		{"", true},
		{"π > short task", true},
		{strings.Repeat("a", 20), true},
		{strings.Repeat("한", 20), true},
		{strings.Repeat("a", 21), false},
		{strings.Repeat("한", 21), false},
	} {
		t.Run(tc.title, func(t *testing.T) {
			// Given
			a := collect.Agent{ID: "pane", Kind: collect.KindInteractive, Tool: "omp",
				Life: collect.LifeWorking, Project: "/p", PaneTitle: tc.title}
			m := Model{snap: projSnap(a)}
			m.rebuildRows()
			// When
			next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
			got := next.(Model)
			defer got.clearNoteSuggestion()
			// Then
			if got.fetchingNote != tc.fetch || (cmd != nil) != tc.fetch || got.suggestingNote == tc.fetch {
				t.Fatalf("fetch=%v suggest=%v cmd=%v; want fetch=%v", got.fetchingNote, got.suggestingNote, cmd != nil, tc.fetch)
			}
		})
	}
}

func TestPurposeTitleManualFetchOpensEditableNoteAndSaves(t *testing.T) {
	// Given
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	a := collect.Agent{ID: "pane", Kind: collect.KindInteractive, Tool: "codex",
		Life: collect.LifeWorking, Project: "/p", PaneTitle: strings.Repeat("x", 21)}
	m := Model{snap: projSnap(a), notesStore: notes.Load()}
	m.rebuildRows()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	// When
	next, cmd := next.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	m = next.(Model)
	if !m.fetchingNote || cmd == nil {
		t.Fatal("o did not start fetching")
	}
	next, _ = m.Update(purposeTitleMsg{agentID: a.ID, request: m.noteRequest, title: "학생 검색 개선"})
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" 작업")})
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	// Then
	if got := m.notesStore.Get(a.ID); got != "학생 검색 개선 작업" {
		t.Fatalf("saved note = %q", got)
	}
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	if got := next.(Model); !got.naming || got.fetchingNote || cmd != nil {
		t.Fatal("existing note must open directly")
	}
}

func TestPurposeTitleDiscardsCancelledAndStaleResults(t *testing.T) {
	for _, action := range []string{"escape", "refresh", "retry"} {
		t.Run(action, func(t *testing.T) {
			// Given
			a := collect.Agent{ID: "pane", Kind: collect.KindInteractive, Tool: "omp",
				Life: collect.LifeWorking, Project: "/p", PaneTitle: "task"}
			m := Model{snap: projSnap(a)}
			m.rebuildRows()
			next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
			m = next.(Model)
			oldRequest := m.noteRequest
			switch action {
			case "escape", "retry":
				next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
			case "refresh":
				next, _ = m.Update(snapshotMsg{snap: projSnap(mkAgent("other", collect.LifeWorking))})
			}
			m = next.(Model)
			if action == "retry" {
				next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
				m = next.(Model)
			}
			defer m.clearNoteSuggestion()
			// When
			next, _ = m.Update(purposeTitleMsg{agentID: a.ID, request: oldRequest, title: "stale title"})
			// Then
			got := next.(Model)
			if got.naming || got.noteInput != "" || got.fetchingNote != (action == "retry") {
				t.Fatalf("stale result changed editor: %#v", got)
			}
		})
	}
}

func TestPurposeTitleFailureKeepsManualInputAvailable(t *testing.T) {
	// Given
	a := collect.Agent{ID: "pane", Kind: collect.KindInteractive, Tool: "omp",
		Life: collect.LifeWorking, Project: "/p", PaneTitle: "task"}
	m := Model{snap: projSnap(a)}
	m.rebuildRows()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	m = next.(Model)
	// When
	next, _ = m.Update(purposeTitleMsg{agentID: a.ID, request: m.noteRequest, err: errors.New("offline")})
	// Then
	got := next.(Model)
	if !got.naming || got.fetchingNote || got.noteInput != "task" || got.noteError == "" {
		t.Fatalf("failure did not open manual editor: %#v", got)
	}
}

func TestInferPurposeTitleProcessBoundary(t *testing.T) {
	for _, tc := range []struct {
		name   string
		script string
		want   string
	}{
		{"success", "printf '학생 검색 개선\\n'", "학생 검색 개선"},
		{"empty", "true", ""},
		{"multiple lines", "printf 'first\\nsecond\\n'", ""},
		{"failed", "exit 1", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Given: fake only the external AI and tmux executables.
			dir := t.TempDir()
			input := filepath.Join(dir, "input")
			t.Setenv("TITLE_INPUT", input)
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			script := "#!/bin/sh\ncat > \"$TITLE_INPUT\"\n" + tc.script + "\n"
			if err := os.WriteFile(filepath.Join(dir, "omp"), []byte(script), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte("#!/bin/sh\nprintf 'ACTIVITY_SENTINEL\\n'\n"), 0755); err != nil {
				t.Fatal(err)
			}
			// When
			title, err := inferPurposeTitle(context.Background(), collect.Agent{TmuxTarget: "%1"})
			// Then
			if title != tc.want || (err != nil) != (tc.want == "") {
				t.Fatalf("title=%q err=%v, want=%q", title, err, tc.want)
			}
			prompt, readErr := os.ReadFile(input)
			if readErr != nil || !strings.Contains(string(prompt), "ACTIVITY_SENTINEL") {
				t.Fatalf("pane data not passed on stdin: %v", readErr)
			}
		})
	}
}

func TestInferPurposeTitleLimitsPreviewOnStdin(t *testing.T) {
	lines := make([]string, 300)
	for i := range lines {
		lines[i] = fmt.Sprintf("LINE_%03d", i)
	}
	for _, tc := range []struct {
		name  string
		input string
		want  string
		token string
	}{
		{"short", "LINE_000\nLINE_001\n", "LINE_000\nLINE_001", "LINE_"},
		{"100 lines", strings.Join(lines[:100], "\n"), strings.Join(lines[:100], "\n"), "LINE_"},
		{"101 lines", strings.Join(lines[:101], "\n"), strings.Join(lines[1:101], "\n"), "LINE_"},
		{"300 lines", strings.Join(lines, "\n"), strings.Join(lines[200:], "\n"), "LINE_"},
		{"8192 bytes", strings.Repeat("Z", 8192), strings.Repeat("Z", 8192), "Z"},
		{"oversized", "DISCARDED\n" + strings.Repeat("Z", 10000) + "RECENT", strings.Repeat("Z", 8186) + "RECENT", "Z"},
		{"Korean boundary", "DISCARDED\n" + strings.Repeat("갛", 3000) + "RECENT", strings.Repeat("갛", 2728) + "RECENT", "갛"},
	} {
		for _, source := range []string{"preview", "tmux"} {
			t.Run(tc.name+"/"+source, func(t *testing.T) {
				// Given: observe the actual subprocess stdin, not prompt prose.
				dir := t.TempDir()
				inputPath := filepath.Join(dir, "input")
				capturePath := filepath.Join(dir, "capture")
				t.Setenv("TITLE_INPUT", inputPath)
				t.Setenv("TITLE_CAPTURE", capturePath)
				t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
				if err := os.WriteFile(filepath.Join(dir, "omp"), []byte("#!/bin/sh\ncat > \"$TITLE_INPUT\"\nprintf 'title\\n'\n"), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte("#!/bin/sh\ncat \"$TITLE_CAPTURE\"\n"), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(capturePath, []byte(tc.input), 0600); err != nil {
					t.Fatal(err)
				}
				a := collect.Agent{Preview: strings.Split(tc.input, "\n")}
				if source == "tmux" {
					a.TmuxTarget = "%1"
				}
				// When
				if _, err := inferPurposeTitle(context.Background(), a); err != nil {
					t.Fatal(err)
				}
				// Then: only the expected newest fixture data reaches omp.
				prompt, err := os.ReadFile(inputPath)
				if err != nil {
					t.Fatal(err)
				}
				got := string(prompt)
				if !utf8.Valid(prompt) || !strings.HasSuffix(got, tc.want) ||
					strings.Count(got, tc.token) != strings.Count(tc.want, tc.token) ||
					strings.Contains(got, "DISCARDED") {
					t.Fatalf("unexpected context: valid UTF-8=%v, fixture tokens=%d, want=%d",
						utf8.Valid(prompt), strings.Count(got, tc.token), strings.Count(tc.want, tc.token))
				}
			})
		}
	}
}
