package collect

import (
	"os"
	"path/filepath"
	"sync"
)

// interactiveAgents bounds concurrent tmux captures while retaining pane order.
// Each worker owns one result slot; the process snapshot is read-only.
func interactiveAgents(procs *procTable, panes []pane) []Agent {
	results := make([]Agent, len(panes))
	found := make([]bool, len(panes))
	home, homeErr := os.UserHomeDir()
	jobs := make(chan int)
	var workers sync.WaitGroup
	for range min(4, len(panes)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := range jobs {
				pn := panes[i]
				ap, ok := procs.findAgentProc(pn.pid)
				if !ok {
					continue
				}
				results[i] = collectInteractiveAgent(procs, pn, ap, home, homeErr == nil)
				found[i] = true
			}
		}()
	}
	for i := range panes {
		jobs <- i
	}
	close(jobs)
	workers.Wait()
	var agents []Agent
	for i, agent := range results {
		if found[i] {
			agents = append(agents, agent)
		}
	}
	return agents
}

func collectInteractiveAgent(procs *procTable, pn pane, ap proc, home string, hasHome bool) Agent {
	return collectInteractivePreview(procs, pn, ap, home, hasHome, capturePane(pn.target, previewLines))
}

// collectInteractivePreview shares recognition-independent enrichment between
// terminal backends; preview retains the terminal's original ANSI formatting.
func collectInteractivePreview(procs *procTable, pn pane, ap proc, home string, hasHome bool, preview []string) Agent {
	tool := detectTool(ap)
	life := interactiveLife(pn.title)
	if life == LifeIdle && contentShowsSpinner(tool, preview) {
		life = LifeWorking
	}
	needsInput := looksLikeChoicePrompt(tool, preview)
	if needsInput {
		life = LifeInput
	}
	var subagents []Subagent
	if tool == "claude" && hasHome {
		var busy bool
		subagents, busy = claudeSubagents(filepath.Join(home, ".claude"), ap, pn)
		// Team agents can run while the main prompt is idle. Their
		// depth-zero metadata lacks completion markers; use the live
		// session's aggregate status instead of counting old records.
		if life == LifeIdle && (busy || len(subagents) > 0) {
			life = LifeWorking
		}
	}
	var checklist []ChecklistItem
	if tool == "omo" && hasHome {
		checklist = omoChecklist(filepath.Join(home, ".omo", "agent"), ap, pn.path)
	}
	return Agent{
		ID:                 pn.paneID,
		Backend:            BackendTmux,
		PaneID:             pn.paneID,
		Window:             pn.window,
		Kind:               KindInteractive,
		Tool:               tool,
		Name:               pn.target,
		Life:               life,
		State:              "live",
		Detail:             titleOrCmd(pn.title, ap),
		CWD:                pn.path,
		Project:            projectKey(pn.path),
		ProcessStartedAt:   ap.startedAt,
		TmuxTarget:         pn.target,
		Session:            pn.session,
		PaneTitle:          pn.title,
		PID:                ap.pid,
		CPU:                procs.subtreeCPU(ap.pid),
		Preview:            preview,
		PreviewFingerprint: previewFingerprint(preview),
		NeedsInput:         needsInput,
		Subagents:          subagents,
		Checklist:          checklist,
	}
}
