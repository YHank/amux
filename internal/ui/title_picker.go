package ui

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) handleTitlePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.editMenu.err = m.editMenu.presetsErr
	switch msg.Type {
	case tea.KeyEnter:
		title := strings.TrimSpace(m.editMenu.input)
		if title == "" || strings.ContainsFunc(title, unicode.IsControl) {
			m.editMenu.err = "Enter a non-empty, single-line title"
			return m, nil
		}
		m.editMenu.saving = true
		return m, renameWindow(m.editMenu.target, title, false)
	case tea.KeyUp, tea.KeyDown:
		if len(m.editMenu.presets) == 0 {
			return m, nil
		}
		step := 1
		if msg.Type == tea.KeyUp {
			step = -1
		}
		m.editMenu.index = max(0, min(m.editMenu.index+step, len(m.editMenu.presets)-1))
		m.editMenu.input = m.editMenu.presets[m.editMenu.index]
	case tea.KeyTab:
		m.editMenu.index = closestTitle(m.editMenu.input, m.editMenu.presets)
		if m.editMenu.index >= 0 {
			m.editMenu.input = m.editMenu.presets[m.editMenu.index]
		}
	case tea.KeyCtrlN, tea.KeyCtrlD:
		if m.editMenu.presetsErr != "" {
			return m, nil
		}
		presets := slices.Clone(m.editMenu.presets)
		if msg.Type == tea.KeyCtrlN {
			presets = append(presets, strings.TrimSpace(m.editMenu.input))
		} else {
			if m.editMenu.index < 0 || len(presets) == 0 {
				return m, nil
			}
			presets = slices.Delete(presets, m.editMenu.index, m.editMenu.index+1)
		}
		if err := saveTitlePresets(defaultTitlePresetsPath(), presets); err != nil {
			m.editMenu.err = err.Error()
			return m, nil
		}
		m.editMenu.presets = presets
		if msg.Type == tea.KeyCtrlN {
			m.editMenu.index = len(presets) - 1
		} else {
			m.editMenu.index = min(m.editMenu.index, len(presets)-1)
		}
		m.editMenu.input = ""
		if m.editMenu.index >= 0 {
			m.editMenu.input = presets[m.editMenu.index]
		}
	case tea.KeyBackspace:
		m.editMenu.input = dropLastRune(m.editMenu.input)
		m.editMenu.index = closestTitle(m.editMenu.input, m.editMenu.presets)
	case tea.KeyCtrlU:
		m.editMenu.input = ""
		m.editMenu.index = closestTitle("", m.editMenu.presets)
	case tea.KeyRunes, tea.KeySpace:
		m.editMenu.input += string(msg.Runes)
		m.editMenu.index = closestTitle(m.editMenu.input, m.editMenu.presets)
	}
	return m, nil
}

// Prefer exact, prefix, then substring matches before edit distance.
// Equal matches keep the user's saved list order.
func closestTitle(input string, titles []string) int {
	input = strings.ToLower(strings.TrimSpace(input))
	best, bestRank, bestDistance := -1, 4, 0
	for i, title := range titles {
		title = strings.ToLower(title)
		rank := 3
		switch {
		case title == input:
			rank = 0
		case strings.HasPrefix(title, input):
			rank = 1
		case strings.Contains(title, input):
			rank = 2
		}
		distance := 0
		if rank == 3 {
			left, right := []rune(input), []rune(title)
			row := make([]int, len(right)+1)
			for j := range row {
				row[j] = j
			}
			for j, a := range left {
				previous := row[0]
				row[0] = j + 1
				for k, b := range right {
					cost := 0
					if a != b {
						cost = 1
					}
					old := row[k+1]
					row[k+1] = min(row[k]+1, row[k+1]+1, previous+cost)
					previous = old
				}
			}
			distance = row[len(right)]
		}
		if rank < bestRank || (rank == bestRank && distance < bestDistance) {
			best, bestRank, bestDistance = i, rank, distance
		}
	}
	return best
}

func (m Model) renderTitlePicker() string {
	w := min(76, m.width-2)
	height := m.height - 2
	lines := []string{
		fmt.Sprintf("Window title  %d/%d", m.editMenu.index+1, len(m.editMenu.presets)),
		popupInputTail(m.editMenu.input, w-4),
	}
	footer := []string{}
	if height >= 6 {
		footer = []string{"up/down choose | enter save | tab complete", "ctrl+n add | ctrl+d delete | ctrl+u clear | esc cancel"}
	}
	if m.editMenu.err != "" {
		footer = append([]string{m.editMenu.err}, footer...)
	}
	if m.editMenu.saving {
		footer = []string{"Saving window title..."}
	}
	page := max(0, height-len(lines)-len(footer))
	start := max(0, m.editMenu.index-page+1)
	for i := start; i < min(start+page, len(m.editMenu.presets)); i++ {
		prefix := "  "
		if i == m.editMenu.index {
			prefix = "> "
		}
		lines = append(lines, prefix+m.editMenu.presets[i])
	}
	if len(m.editMenu.presets) == 0 && page > 0 {
		lines = append(lines, "(empty list: type a title, ctrl+n to add)")
	}
	lines = append(lines, footer...)
	lines = lines[:min(len(lines), height)]
	for i, line := range lines {
		if lipgloss.Width(line) > w-4 {
			line = truncateWidth(line, w-4)
		}
		if i == 0 || strings.HasPrefix(line, "> ") {
			line = styleTitle.Render(line)
		}
		lines[i] = line
	}
	panel := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
		BorderForeground(cAccent).Padding(0, 1).Width(w).Render(strings.Join(lines, "\n"))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel)
}
