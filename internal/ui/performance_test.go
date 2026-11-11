package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kyhan/amux/internal/collect"
)

// Fixtures use fixed identities, timestamps, ordering and preview content.
// View still uses the production clock for age labels; no timing is asserted.
func benchmarkDashboard(b *testing.B, count int) (Model, collect.Snapshot) {
	b.Helper()
	dir := b.TempDir()
	b.Setenv("HOME", dir)
	b.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	b.Setenv("AMUX_START_PANE", "")
	b.Setenv("TMUX_PANE", "")
	now := time.Date(2026, time.November, 1, 12, 0, 0, 0, time.UTC)
	snap := collect.Snapshot{Taken: now, LivePaneIDs: make(map[string]bool, count)}
	m := New()
	for i := 0; i < count; i++ {
		if i%4 == 0 {
			name := fmt.Sprintf("project-%02d", i/4)
			snap.Projects = append(snap.Projects, collect.Project{
				Key: "/bench/" + name, Name: name,
			})
		}
		p := &snap.Projects[len(snap.Projects)-1]
		preview := make([]string, 24)
		for line := range preview {
			preview[line] = fmt.Sprintf("preview-%02d: step %02d inspecting dashboard rendering", i, line)
		}
		a := collect.Agent{
			ID: fmt.Sprintf("%%%d", i+1), Kind: collect.KindInteractive,
			Tool: "claude", Name: fmt.Sprintf("agent-%02d", i),
			Model: "sonnet", Life: collect.LifeWorking,
			Project: p.Key, CWD: p.Key, Window: "start", Session: "bench",
			TmuxTarget: fmt.Sprintf("bench:%d.%d", i/4, i%4),
			PID:        1000 + i, CPU: float64(count - i),
			CreatedAt: now.Add(-time.Hour), ProcessStartedAt: now.Add(-time.Hour),
			UpdatedAt: now, StatusChangedAt: now, ActivityAt: now,
			Preview: preview, PreviewFingerprint: fmt.Sprintf("preview-%02d", i),
		}
		p.Agents = append(p.Agents, a)
		snap.Agents = append(snap.Agents, a)
		snap.LivePaneIDs[a.ID] = true
		m.notesStore.Set(a.ID, fmt.Sprintf("task-%02d", i))
	}
	next, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 44})
	m = next.(Model)
	next, cmd := m.Update(snapshotMsg{snap: snap})
	m = next.(Model)
	if cmd != nil || m.err != nil || len(m.selRows) != count {
		b.Fatalf("dashboard fixture: rows=%d err=%v cmd=%v", len(m.selRows), m.err, cmd != nil)
	}
	view := m.View()
	if !strings.Contains(view, "agent-00") || !strings.Contains(view, "preview-00:") {
		b.Fatal("dashboard fixture did not render an agent and its preview")
	}
	return m, snap
}

// Each operation is one navigation key plus a full dashboard render.
// Traverse down and back up so neither clamping nor a fixed row dominates.
func BenchmarkDashboardNavigation(b *testing.B) {
	for _, count := range []int{8, 32} {
		b.Run(fmt.Sprintf("agents=%d", count), func(b *testing.B) {
			m, _ := benchmarkDashboard(b, count)
			keys := make([]tea.KeyMsg, 2*(count-1))
			for i := range keys {
				key := 'j'
				if i >= count-1 {
					key = 'k'
				}
				keys[i] = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}}
			}
			next, cmd := m.Update(keys[0])
			moved := next.(Model)
			if cmd != nil || moved.selectedAgent() == nil || moved.selectedAgent().ID != "%2" ||
				!strings.Contains(moved.View(), "preview-01:") {
				b.Fatal("navigation did not select and render the next agent")
			}
			var view string
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				next, _ := m.Update(keys[i%len(keys)])
				m = next.(Model)
				view = m.View()
			}
			b.StopTimer()
			if !strings.Contains(view, "preview-") {
				b.Fatal("navigation lost the dashboard preview")
			}
		})
	}
}

// Measure a successful steady-state snapshot, including real note pruning
// and persistence in the isolated directory. Collection itself is excluded.
func BenchmarkDashboardSnapshot(b *testing.B) {
	for _, count := range []int{8, 32} {
		b.Run(fmt.Sprintf("agents=%d", count), func(b *testing.B) {
			m, snap := benchmarkDashboard(b, count)
			msg := snapshotMsg{snap: snap}
			var view string
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				next, _ := m.Update(msg)
				m = next.(Model)
				view = m.View()
			}
			b.StopTimer()
			if m.err != nil || len(m.selRows) != count ||
				!strings.Contains(view, "preview-00:") {
				b.Fatal("snapshot did not retain the populated dashboard")
			}
		})
	}
}

// Open the actual i,t editor and type once outside timing. Each operation
// completes the same uncompleted draft with Tab and renders the title popup.
func BenchmarkTitleCompletion(b *testing.B) {
	for _, count := range []int{8, 32} {
		for _, input := range []string{"rev", "reviw"} {
			b.Run(fmt.Sprintf("agents=%d/input=%s", count, input), func(b *testing.B) {
				m, _ := benchmarkDashboard(b, count)
				if err := saveTitlePresets(defaultTitlePresetsPath(),
					[]string{"start", "stay", "review", "commit", "gitedit", "end"}); err != nil {
					b.Fatal(err)
				}
				for _, key := range []tea.KeyMsg{
					{Type: tea.KeyRunes, Runes: []rune{'i'}},
					{Type: tea.KeyRunes, Runes: []rune{'t'}},
					{Type: tea.KeyCtrlU},
					{Type: tea.KeyRunes, Runes: []rune(input)},
				} {
					next, cmd := m.Update(key)
					if cmd != nil {
						b.Fatal("opening and typing in the title picker returned a command")
					}
					m = next.(Model)
				}
				tab := tea.KeyMsg{Type: tea.KeyTab}
				next, cmd := m.Update(tab)
				completed := next.(Model)
				if cmd != nil || !completed.editMenu.title || completed.editMenu.input != "review" ||
					!strings.Contains(completed.View(), "> review") {
					b.Fatal("Tab did not complete and render the review preset")
				}
				var view string
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					next, _ := m.Update(tab)
					completed = next.(Model)
					view = completed.View()
				}
				b.StopTimer()
				if completed.editMenu.input != "review" || !strings.Contains(view, "> review") {
					b.Fatal("title completion lost the completed preset")
				}
			})
		}
	}
}
