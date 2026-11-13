package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/kyhan/amux/internal/collect"
)

func TestChecklistShowsCurrentWorkWithoutPlan(t *testing.T) {
	for _, tool := range []string{"claude", "opencode", "codex", "omo"} {
		t.Run(tool, func(t *testing.T) {
			agent := collect.Agent{ID: "agent", Tool: tool, Kind: collect.KindInteractive,
				Life: collect.LifeWorking, Project: "/p", Intent: "Review the payment flow",
				Subagents: []collect.Subagent{{ID: "child", Description: "Check refunds", Life: collect.LifeWorking}}}
			m := Model{snap: projSnap(agent), preferencesPath: t.TempDir() + "/preferences.json"}
			m.rebuildRows()
			m = pressKey(m, 'l')
			out := m.renderChecklist(50, 14)
			for _, want := range []string{"Current work", "Review the payment flow", "Check refunds"} {
				if !strings.Contains(out, want) {
					t.Fatalf("missing %q in:\n%s", want, out)
				}
			}
			if strings.Contains(out, "no plan recorded") {
				t.Fatalf("summary was replaced by empty-plan notice:\n%s", out)
			}
		})
	}
}

func TestChecklistPlanTakesPriorityOverSummary(t *testing.T) {
	m := Model{snap: projSnap(collect.Agent{ID: "agent", Kind: collect.KindInteractive, Project: "/p",
		Intent: "OLD-REQUEST", Checklist: []collect.ChecklistItem{{Content: "CURRENT-PLAN", Status: collect.ChecklistActive}}})}
	m.rebuildRows()
	out := m.renderChecklist(50, 14)
	if !strings.Contains(out, "CURRENT-PLAN") || strings.Contains(out, "OLD-REQUEST") || strings.Contains(out, "Current work") {
		t.Fatalf("recorded plan must take priority:\n%s", out)
	}
}

func TestCurrentWorkFitsSmallPanel(t *testing.T) {
	m := Model{snap: projSnap(collect.Agent{ID: "agent", Kind: collect.KindInteractive, Project: "/p",
		Intent: strings.Repeat("한글 작업 요약 ", 100), Life: collect.LifeInput})}
	m.rebuildRows()
	out := m.renderChecklist(24, 8)
	if !strings.Contains(out, "한글") {
		t.Fatalf("summary missing:\n%s", out)
	}
	if lipgloss.Height(out) > 8 {
		t.Fatalf("height overflow: %d", lipgloss.Height(out))
	}
	for _, line := range strings.Split(out, "\n") {
		if lipgloss.Width(line) > 24 {
			t.Fatalf("width overflow: %q", line)
		}
	}
}
