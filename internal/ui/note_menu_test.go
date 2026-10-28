package ui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kyhan/amux/internal/notegen"
)

func TestNoteMenuOpensBeforeGeneration(t *testing.T) {
	// Given
	m := autoTitleTestModel(t)
	// When
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	m = next.(Model)
	// Then
	if !m.showNoteMenu || cmd != nil || len(m.autoTitleRequests) != 0 {
		t.Fatal("o must open a menu without generating")
	}
}

func TestNoteModelSelectionPersistsAndIgnoresClosedResponse(t *testing.T) {
	// Given
	m := autoTitleTestModel(t)
	m.showNoteMenu = true
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = next.(Model)
	defer m.closeNoteMenu()
	if cmd == nil {
		t.Fatal("model list was not requested")
	}
	next, _ = m.Update(noteModelsMsg{request: m.modelRequest, models: []notegen.Model{
		{Model: "first", IsDefault: true}, {Model: "chosen"},
	}})
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyDown})
	// When
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	next, _ = m.Update(noteModelsMsg{request: m.modelRequest, err: errors.New("late")})
	// Then
	if notegen.SelectedModel() != "chosen" || next.(Model).showNoteMenu || next.(Model).modelError != "" {
		t.Fatal("model selection was lost or late response reopened the picker")
	}
}

func TestNoteMenuFitsTerminalSizes(t *testing.T) {
	// Given
	m := autoTitleTestModel(t)
	m.showNoteMenu = true
	for _, size := range [][2]int{{20, 5}, {40, 12}, {100, 24}} {
		for _, picker := range []bool{false, true} {
			m.width, m.height = size[0], size[1]
			m.choosingNoteModel = picker
			m.noteModels = []notegen.Model{{Model: "긴한글모델이름-long-model-name"}, {Model: "second"}}
			// When
			view := m.View()
			// Then
			if lipgloss.Width(view) > m.width || strings.Count(view, "\n")+1 > m.height {
				t.Fatalf("popup overflow at %v picker=%v: %dx%d", size, picker, lipgloss.Width(view), strings.Count(view, "\n")+1)
			}
		}
	}
}
