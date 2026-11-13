package collect

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInteractiveClaudeChecklist(t *testing.T) {
	for _, source := range []string{"tasks", "todos"} {
		t.Run(source, func(t *testing.T) {
			home := t.TempDir()
			root := filepath.Join(home, ".claude")
			writeClaudeFixture(t, filepath.Join(root, "sessions", "37766.json"), claudeSessionFixture)
			if source == "tasks" {
				dir := filepath.Join(root, "tasks", "session-live")
				writeClaudeFixture(t, filepath.Join(dir, "10.json"), `{"id":"10","subject":"Verify changes","status":"pending"}`)
				writeClaudeFixture(t, filepath.Join(dir, "2.json"), `{"id":"2","subject":"Implement changes","status":"in_progress"}`)
				writeClaudeFixture(t, filepath.Join(dir, "1.json"), `{"id":"1","subject":"Read code","status":"completed"}`)
			} else {
				log := filepath.Join(root, "projects", "-work-my-project", "session-live.jsonl")
				writeClaudeFixture(t, log, `{"type":"user","timestamp":"2026-11-13T00:00:00Z","toolUseResult":{"newTodos":[{"content":"Read code","status":"completed"},{"content":"Implement changes","status":"in_progress"},{"content":"Verify changes","status":"pending"}]}}`+"\n")
			}
			parent := proc{pid: 37766, comm: "claude", startedAt: time.UnixMilli(1793579768640)}
			procs := &procTable{byPID: map[int]proc{37766: parent}}
			pn := pane{paneID: "%422", path: "/work/my_project"}
			got := collectInteractivePreview(procs, pn, parent, home, true, nil).Checklist
			if len(got) != 3 || got[0].Content != "Read code" || got[0].Status != ChecklistDone ||
				got[1].Content != "Implement changes" || got[1].Status != ChecklistActive ||
				got[2].Content != "Verify changes" || got[2].Status != ChecklistPending {
				t.Fatalf("checklist = %#v", got)
			}
			parent.startedAt = parent.startedAt.Add(time.Hour)
			if got := collectInteractivePreview(procs, pn, parent, home, true, nil).Checklist; len(got) != 0 {
				t.Fatalf("reused PID borrowed a plan: %#v", got)
			}
		})
	}
}

func TestClaudeTodosFollowSuccessfulUpdates(t *testing.T) {
	log := filepath.Join(t.TempDir(), "session.jsonl")
	line := func(timestamp, todos string) string {
		return `{"type":"user","timestamp":"` + timestamp + `","toolUseResult":{"newTodos":` + todos + `}}` + "\n"
	}
	appendFile(t, log, line("2026-11-13T00:00:00Z", `[{"content":"Implement","status":"in_progress"}]`))
	cache := &omoChecklistCache{states: map[string]*omoChecklistState{}, applyLine: applyClaudeTodos}
	now := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
	got := cache.read(log, now)
	if len(got) != 1 || !got[0].StartedAt.Equal(now) {
		t.Fatalf("initial plan = %#v", got)
	}
	appendFile(t, log, `{"type":"assistant","toolUseResult":{"newTodos":[]}}`+"\n"+
		`{"type":"user","isSidechain":true,"toolUseResult":{"newTodos":[]}}`+"\n"+
		`{"type":"user","toolUseResult":"error"}`+"\n")
	done := line("2026-11-13T00:02:00Z", `[{"content":"Implement","status":"completed"}]`)
	appendFile(t, log, done[:35])
	if got := cache.read(log, now); len(got) != 1 || got[0].Status != ChecklistActive {
		t.Fatalf("unrelated/partial records changed the plan: %#v", got)
	}
	appendFile(t, log, done[35:])
	got = cache.read(log, now)
	if len(got) != 1 || got[0].Status != ChecklistDone || got[0].FinishedAt.Sub(got[0].StartedAt) != 2*time.Minute {
		t.Fatalf("completion = %#v", got)
	}
	appendFile(t, log, line("2026-11-13T00:03:00Z", `[]`))
	if got := cache.read(log, now); len(got) != 0 {
		t.Fatalf("cleared plan retained items: %#v", got)
	}
}

func TestClaudeTasksUseExplicitSessionList(t *testing.T) {
	root := t.TempDir()
	sessionID := "12345678-aaaa-bbbb-cccc-123456789abc"
	dir := filepath.Join(root, "tasks", "session-12345678")
	writeClaudeFixture(t, filepath.Join(dir, ".session-list"), "shared_review")
	task := filepath.Join(root, "tasks", "shared_review", "1.json")
	writeClaudeFixture(t, task, `{"id":"1","subject":"Shared review","status":"in_progress"}`)
	writeClaudeFixture(t, filepath.Join(root, "tasks", "unrelated", "1.json"), `{"id":"1","subject":"Wrong session","status":"pending"}`)
	got, found := claudeTaskChecklist(root, sessionID)
	if !found || len(got) != 1 || got[0].Content != "Shared review" || got[0].Status != ChecklistActive {
		t.Fatalf("linked plan = %#v, found=%v", got, found)
	}
	writeClaudeFixture(t, task, `{"id":"1","subject":"Shared review","status":"completed"}`)
	if got, _ := claudeTaskChecklist(root, sessionID); len(got) != 1 || got[0].Status != ChecklistDone {
		t.Fatalf("updated task = %#v", got)
	}
	if err := os.Remove(task); err != nil {
		t.Fatal(err)
	}
	if got, _ := claudeTaskChecklist(root, sessionID); len(got) != 0 {
		t.Fatalf("deleted task retained: %#v", got)
	}
	writeClaudeFixture(t, filepath.Join(dir, ".session-list"), "../unrelated")
	if got, _ := claudeTaskChecklist(root, sessionID); len(got) != 0 {
		t.Fatalf("unsafe list pointer followed: %#v", got)
	}
}
