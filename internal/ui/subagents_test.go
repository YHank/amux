package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/kyhan/amux/internal/collect"
)

func TestSubagentsRenderRunningCountAndTaskStatus(t *testing.T) {
	a := collect.Agent{
		ID: "%1", Kind: collect.KindInteractive, Tool: "claude", Name: "claude",
		Life: collect.LifeWorking,
		Subagents: []collect.Subagent{
			{ID: "a", Description: "결제 코드 검토", Life: collect.LifeWorking},
			{ID: "b", Description: "보안 검토", Life: collect.LifeWorking},
			{ID: "c", Description: "완료된 조사", Life: collect.LifeDone},
		},
	}
	for _, width := range []int{40, 100} {
		m := Model{
			rows:    []row{{agent: &a}},
			selRows: []int{0},
		}
		row := m.renderAgentRow(&a, nil, width, false, time.Time{})
		if !strings.Contains(row, "sub:2") || lipgloss.Width(row) > width {
			t.Fatalf("width %d row = %q", width, row)
		}
		detail := m.renderDetail(width, 40)
		for _, child := range a.Subagents {
			if !strings.Contains(detail, child.Description) {
				t.Errorf("detail missing task %q: %s", child.Description, detail)
			}
		}
	}
	a.Subagents = nil
	if got := agentMeta(&a, time.Time{}); got != "" {
		t.Fatalf("metadata without subagents = %q", got)
	}
}
