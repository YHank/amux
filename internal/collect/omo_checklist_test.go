package collect

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func omoTodoLine(ts, op, phases string) string {
	return `{"type":"message","timestamp":"` + ts + `","message":{"role":"toolResult","toolName":"todo","isError":false,"details":{"op":"` + op + `","phases":` + phases + `}}}` + "\n"
}

func appendFile(t *testing.T, path, text string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(text); err != nil {
		t.Fatal(err)
	}
}

func TestOMOChecklistFollowsLiveSessionTranscript(t *testing.T) {
	root := t.TempDir()
	cwd := "/work/my_app"
	start := time.UnixMilli(1794459475000)
	dir := filepath.Join(root, "sessions", "--work-my_app--")
	writeClaudeFixture(t, filepath.Join(dir, "session-holders", "sess-1", "4242.json"),
		`{"pid":4242,"processStartedAtMs":1794459475000,"cwd":"/work/my_app"}`)
	log := filepath.Join(dir, "2026-11-13T00-00-00-000Z_sess-1.jsonl")
	appendFile(t, log, `{"type":"message","message":{"role":"user","content":"quote \"toolName\":\"todo\""}}`+"\n"+
		omoTodoLine("2026-11-13T00:00:00Z", "init", `[{"name":"A","tasks":[{"content":"read","status":"in_progress"},{"content":"edit","status":"pending"}]},{"name":"B","tasks":[{"content":"test","status":"pending"}]}]`)+
		`{"type":"message","message":{"role":"toolResult","toolName":"todo","isError":true,"details":{"op":"done"}}}`+"\n")

	ap := proc{pid: 4242, startedAt: start}
	got, _ := omoChecklist(root, ap, cwd)
	if len(got) != 3 || got[0].Status != ChecklistActive || got[2].Phase != "B" ||
		!got[0].StartedAt.Equal(time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("initial checklist = %#v", got)
	}

	// The second update arrives in two writes; the partial line must wait.
	done := omoTodoLine("2026-11-13T00:06:58Z", "done", `[{"name":"A","tasks":[{"content":"read","status":"completed"},{"content":"edit","status":"in_progress"}]},{"name":"B","tasks":[{"content":"test","status":"pending"}]}]`)
	appendFile(t, log, done[:40])
	if again, _ := omoChecklist(root, ap, cwd); again[0].Status != ChecklistActive {
		t.Fatalf("partial line was applied: %#v", again)
	}
	appendFile(t, log, done[40:])
	got, _ = omoChecklist(root, ap, cwd)
	if got[0].Status != ChecklistDone || got[0].FinishedAt.Sub(got[0].StartedAt) != 6*time.Minute+58*time.Second ||
		got[1].Status != ChecklistActive || got[1].StartedAt.IsZero() {
		t.Fatalf("updated checklist = %#v", got)
	}

	for name, other := range map[string]proc{
		"reused pid":   {pid: 4242, startedAt: start.Add(time.Hour)},
		"other pid":    {pid: 7, startedAt: start},
		"no start":     {pid: 4242},
		"relative cwd": {pid: 4242, startedAt: start},
	} {
		path := cwd
		if name == "relative cwd" {
			path = "work/my_app"
		}
		if got, _ := omoChecklist(root, other, path); got != nil {
			t.Fatalf("%s: checklist = %#v, want none", name, got)
		}
	}
}

func TestOMOChecklistInitResetsTimingsAndShrinkRereads(t *testing.T) {
	log := filepath.Join(t.TempDir(), "s.jsonl")
	appendFile(t, log, omoTodoLine("2026-11-13T00:00:00Z", "init", `[{"name":"A","tasks":[{"content":"x","status":"in_progress"}]}]`)+
		omoTodoLine("2026-11-13T01:00:00Z", "init", `[{"name":"A","tasks":[{"content":"x","status":"in_progress"}]}]`))
	cache := &omoChecklistCache{states: map[string]*omoChecklistState{}}
	now := time.Now()
	if got := cache.read(log, now); len(got) != 1 || got[0].StartedAt.Hour() != 1 {
		t.Fatalf("init should restart timings: %#v", got)
	}
	if err := os.WriteFile(log, []byte(omoTodoLine("2026-11-13T02:00:00Z", "rm", `[]`)), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := cache.read(log, now); len(got) != 0 {
		t.Fatalf("rewritten transcript must be reread: %#v", got)
	}
}
