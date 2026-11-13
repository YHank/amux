package collect

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRequestTextExcludesInjectedContextAndToolResults(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{`"결제   흐름을 검토해줘"`, "결제 흐름을 검토해줘"},
		{`[{"type":"tool_result","content":"not a request"},{"type":"text","text":"Check refunds"},{"type":"text","text":"<system-reminder>noise</system-reminder>"}]`, "Check refunds"},
		{`[{"type":"input_text","text":"<environment_context>cwd</environment_context>Fix bug"}]`, "Fix bug"},
		{`"<recalled-memory source=\"x\">noise</recalled-memory>"`, ""},
		{`"Another Claude session sent a message: <teammate-message teammate_id=\"reviewer\">review finished</teammate-message>"`, ""},
		{`"<div>user supplied HTML</div>"`, "<div>user supplied HTML</div>"},
		{`{}`, ""},
	} {
		if got := requestText(json.RawMessage(tc.input)); got != tc.want {
			t.Errorf("requestText(%s) = %q, want %q", tc.input, got, tc.want)
		}
	}
	long, err := json.Marshal(strings.Repeat("한", 1100))
	if err != nil {
		t.Fatal(err)
	}
	if got := requestText(long); got != strings.Repeat("한", 1000)+"…" {
		t.Fatal("long request must be bounded without splitting Unicode")
	}
}

func TestClaudeCurrentRequestFollowsUserMessages(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".claude")
	writeClaudeFixture(t, filepath.Join(root, "sessions", "37766.json"), claudeSessionFixture)
	log := filepath.Join(root, "projects", "-work-my-project", "session-live.jsonl")
	writeClaudeFixture(t, log, `{"type":"user","message":{"role":"user","content":"Old request"}}`+"\n"+
		`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"Review the payment flow"}]}}`+"\n"+
		`{"type":"user","isMeta":true,"message":{"content":"Injected instructions"}}`+"\n"+
		`{"type":"user","isSidechain":true,"message":{"content":"Child request"}}`+"\n"+
		`{"type":"user","message":{"content":"Another Claude session sent a message: <teammate-message teammate_id=\"reviewer\">review finished</teammate-message>"}}`+"\n"+
		`{"type":"user","isCompactSummary":true,"message":{"content":"Compaction summary"}}`+"\n"+
		`{"type":"user","message":{"content":[{"type":"tool_result","content":"Tool output"}]}}`+"\n")
	parent := proc{pid: 37766, comm: "claude", startedAt: time.UnixMilli(1793579768640)}
	procs := &procTable{byPID: map[int]proc{37766: parent}}
	pn := pane{paneID: "%422", path: "/work/my_project"}
	got := collectInteractivePreview(procs, pn, parent, home, true, nil)
	if got.Intent != "Review the payment flow" || len(got.Checklist) != 0 {
		t.Fatalf("current request = %q, plan=%#v", got.Intent, got.Checklist)
	}
	appendFile(t, log, `{"type":"user","message":{"content":"Verify refunds"}}`+"\n")
	got = collectInteractivePreview(procs, pn, parent, home, true, nil)
	if got.Intent != "Verify refunds" {
		t.Fatalf("request did not refresh: %q", got.Intent)
	}
}

func TestCodexCurrentRequestUsesVerifiedSession(t *testing.T) {
	root := t.TempDir()
	log := codexRolloutFixture(t, root, codexTestSession, codexTestSession)
	appendFile(t, log, `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Review billing"}]}}`+"\n")
	items, intent := codexChecklist(root, codexTestSession)
	if len(items) != 0 || intent != "Review billing" {
		t.Fatalf("plan=%#v request=%q", items, intent)
	}
}

func TestOMOCurrentRequestIgnoresBackgroundNotifications(t *testing.T) {
	log := filepath.Join(t.TempDir(), "session.jsonl")
	appendFile(t, log, `{"type":"message","message":{"role":"user","content":[{"type":"text","text":"Review billing"}]}}`+"\n"+
		`{"type":"message","message":{"role":"user","content":"<system-reminder>task completed</system-reminder>"}}`+"\n")
	cache := &omoChecklistCache{states: map[string]*omoChecklistState{}}
	items, intent := cache.readWithIntent(log, time.Now())
	if len(items) != 0 || intent != "Review billing" {
		t.Fatalf("plan=%#v request=%q", items, intent)
	}
}
