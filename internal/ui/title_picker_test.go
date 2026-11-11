package ui

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kyhan/amux/internal/collect"
)

func titlePickerTestModel(t *testing.T) Model {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	a := collect.Agent{ID: "%10", Kind: collect.KindInteractive, Life: collect.LifeIdle,
		Project: "/p", Window: "start", TmuxTarget: "work:0.0"}
	m := Model{snap: projSnap(a), width: 80, height: 24}
	m.rebuildRows()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	return next.(Model)
}

func TestTitlePickerTabCompletesClosestSavedTitle(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  string
	}{
		{"", "start"},
		{"sta", "start"},
		{"STAY", "stay"},
		{"view", "review"},
		{"reviw", "review"},
		{"comit", "commit"},
		{"gitedti", "gitedit"},
		{"배퍼", "배포"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			m := titlePickerTestModel(t)
			m.editMenu.presets = append(m.editMenu.presets, "배포")
			next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
			next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tc.input)})

			next, cmd := next.(Model).Update(tea.KeyMsg{Type: tea.KeyTab})

			got := next.(Model)
			if cmd != nil || got.editMenu.input != tc.want || got.editMenu.presets[got.editMenu.index] != tc.want {
				t.Fatalf("Tab(%q) = %q, want %q", tc.input, got.editMenu.input, tc.want)
			}
		})
	}
}

func TestTitlePickerAddsAndDeletesAcrossReopen(t *testing.T) {
	m := titlePickerTestModel(t)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("deploy")})
	next, cmd := next.(Model).Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	if cmd != nil || next.(Model).editMenu.err != "" {
		t.Fatalf("add failed: %s", next.(Model).editMenu.err)
	}
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyEsc})
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("depl")})
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyTab})
	if got := next.(Model).editMenu.input; got != "deploy" {
		t.Fatalf("added title did not survive reopen: %q", got)
	}

	next, cmd = next.(Model).Update(tea.KeyMsg{Type: tea.KeyCtrlD})

	saved, err := loadTitlePresets(defaultTitlePresetsPath())
	if err != nil || cmd != nil || slices.Contains(saved, "deploy") || slices.Contains(next.(Model).editMenu.presets, "deploy") {
		t.Fatalf("deleted title still present: %v, err=%v", saved, err)
	}
}

func TestTitlePickerRejectsDuplicateAndPreservesListOnSaveFailure(t *testing.T) {
	for _, scenario := range []string{"duplicate", "write failure"} {
		t.Run(scenario, func(t *testing.T) {
			m := titlePickerTestModel(t)
			before := slices.Clone(m.editMenu.presets)
			if scenario == "write failure" {
				if err := os.MkdirAll(defaultTitlePresetsPath(), 0o700); err != nil {
					t.Fatal(err)
				}
				m.editMenu.input = "deploy"
			}

			next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})

			got := next.(Model)
			if cmd != nil || got.editMenu.err == "" || !slices.Equal(got.editMenu.presets, before) {
				t.Fatal("failed add must show an error and preserve the list")
			}
		})
	}
}

func TestTitlePickerEmptyListAcceptsTextAndTabIsNoop(t *testing.T) {
	m := titlePickerTestModel(t)
	for range len(m.editMenu.presets) {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
		m = next.(Model)
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("custom")})
	next, cmd := next.(Model).Update(tea.KeyMsg{Type: tea.KeyTab})
	saved, err := loadTitlePresets(defaultTitlePresetsPath())
	if err != nil || len(saved) != 0 || cmd != nil || next.(Model).editMenu.input != "custom" {
		t.Fatalf("empty list lost custom input or reset defaults: %v, err=%v", saved, err)
	}
}

func TestAutomaticRenameDoesNotCloseAnotherTitleEditor(t *testing.T) {
	for _, err := range []error{nil, errors.New("rename failed")} {
		m := titlePickerTestModel(t)
		m.editMenu.saving = true
		next, _ := m.Update(renamedWindowMsg{target: "%10", title: "start", err: err, automatic: true})
		if !next.(Model).editMenu.open || next.(Model).editMenu.input != "start" {
			t.Fatal("automatic rename completion closed or changed the title editor")
		}
	}
}

func TestTitlePickerShowsInputAboveScrollablePresets(t *testing.T) {
	m := titlePickerTestModel(t)
	for _, size := range [][2]int{{20, 5}, {40, 10}, {80, 24}, {120, 32}} {
		for _, index := range []int{0, 5} {
			m.width, m.height = size[0], size[1]
			m.editMenu.index = index
			m.editMenu.input = "draft"
			view := m.View()
			if lipgloss.Width(view) > m.width || lipgloss.Height(view) > m.height {
				t.Fatalf("picker overflows %v", size)
			}
			input, selected := strings.Index(view, "draft"), strings.Index(view, "> "+m.editMenu.presets[index])
			if input < 0 || selected <= input {
				t.Fatalf("input must be above visible selection at %v: %s", size, view)
			}
		}
	}
}
