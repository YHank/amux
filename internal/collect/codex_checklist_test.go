package collect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

const codexTestSession = "11111111-2222-3333-4444-555555555555"
const codexOtherSession = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

func codexPlanCall(t *testing.T, id, plan string) string {
	t.Helper()
	args, err := json.Marshal(`{"plan":` + plan + `}`)
	if err != nil {
		t.Fatal(err)
	}
	return `{"timestamp":"2026-11-13T00:00:01Z","type":"response_item","payload":{"type":"function_call","name":"update_plan","call_id":"` +
		id + `","arguments":` + string(args) + "}}\n"
}

func codexPlanResult(t *testing.T, id, timestamp, output string) string {
	t.Helper()
	body, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	return `{"timestamp":"` + timestamp + `","type":"response_item","payload":{"type":"function_call_output","call_id":"` +
		id + `","output":` + string(body) + "}}\n"
}

func codexRolloutFixture(t *testing.T, root, filenameID, metadataID string) string {
	t.Helper()
	path := filepath.Join(root, "sessions", "2026", "11", "13", "rollout-2026-11-13T00-00-00-"+filenameID+".jsonl")
	writeClaudeFixture(t, path, `{"type":"session_meta","payload":{"id":"`+metadataID+`","cwd":"/same/project"}}`+"\n")
	return path
}

func TestCodexChecklistUpdatesOnlyOnSuccessfulResult(t *testing.T) {
	root := t.TempDir()
	log := codexRolloutFixture(t, root, codexTestSession, codexTestSession)
	appendFile(t, log, codexPlanCall(t, "first", `[{"step":"read","status":"in_progress"},{"step":"edit","status":"pending"}]`))
	if got, _ := codexChecklist(root, codexTestSession); got != nil {
		t.Fatalf("unacknowledged call became a checklist: %#v", got)
	}
	start := time.Date(2026, 11, 13, 0, 0, 2, 0, time.UTC)
	appendFile(t, log, codexPlanResult(t, "first", start.Format(time.RFC3339), "Plan updated"))
	want := []ChecklistItem{
		{Content: "read", Status: ChecklistActive, StartedAt: start},
		{Content: "edit", Status: ChecklistPending},
	}
	if got, _ := codexChecklist(root, codexTestSession); !reflect.DeepEqual(got, want) {
		t.Fatalf("initial checklist = %#v, want %#v", got, want)
	}
	appendFile(t, log,
		codexPlanCall(t, "failed", `[{"step":"wrong","status":"completed"}]`)+
			codexPlanResult(t, "failed", "2026-11-13T00:01:00Z", "Error: invalid plan")+
			codexPlanResult(t, "unrelated", "2026-11-13T00:01:01Z", "Plan updated"))
	if got, _ := codexChecklist(root, codexTestSession); !reflect.DeepEqual(got, want) {
		t.Fatalf("failed or unrelated response changed checklist: %#v", got)
	}
	finished := start.Add(2 * time.Minute)
	appendFile(t, log,
		codexPlanCall(t, "next", `[{"step":"edit","status":"in_progress"},{"step":"read","status":"completed"}]`)+
			codexPlanResult(t, "next", finished.Format(time.RFC3339), "Plan updated"))
	want = []ChecklistItem{
		{Content: "edit", Status: ChecklistActive, StartedAt: finished},
		{Content: "read", Status: ChecklistDone, StartedAt: start, FinishedAt: finished},
	}
	if got, _ := codexChecklist(root, codexTestSession); !reflect.DeepEqual(got, want) {
		t.Fatalf("updated checklist = %#v, want %#v", got, want)
	}
	appendFile(t, log, codexPlanCall(t, "clear", `[]`)+
		codexPlanResult(t, "clear", "2026-11-13T00:03:00Z", "Plan updated"))
	if got, _ := codexChecklist(root, codexTestSession); len(got) != 0 {
		t.Fatalf("clear retained checklist: %#v", got)
	}
}

func TestCodexChecklistIsolatesExactSessions(t *testing.T) {
	root := t.TempDir()
	for _, id := range []string{codexTestSession, codexOtherSession} {
		log := codexRolloutFixture(t, root, id, id)
		appendFile(t, log, codexPlanCall(t, "same-call", `[{"step":"`+id+`","status":"pending"}]`)+
			codexPlanResult(t, "same-call", "2026-11-13T00:00:02Z", "Plan updated"))
	}
	for _, id := range []string{codexTestSession, codexOtherSession} {
		want := []ChecklistItem{{Content: id, Status: ChecklistPending}}
		if got, _ := codexChecklist(root, id); !reflect.DeepEqual(got, want) {
			t.Fatalf("session %s checklist = %#v, want %#v", id, got, want)
		}
	}
}

func TestCodexChecklistRejectsWrongSessionMetadata(t *testing.T) {
	root := t.TempDir()
	log := codexRolloutFixture(t, root, codexTestSession, codexOtherSession)
	appendFile(t, log, codexPlanCall(t, "wrong-session", `[{"step":"wrong","status":"pending"}]`)+
		codexPlanResult(t, "wrong-session", "2026-11-13T00:00:02Z", "Plan updated"))
	if got, _ := codexChecklist(root, codexTestSession); got != nil {
		t.Fatalf("mismatched metadata produced checklist: %#v", got)
	}
	if got, _ := codexChecklist(root, "../"+codexTestSession); got != nil {
		t.Fatalf("invalid session ID produced checklist: %#v", got)
	}
}

func TestCodexChecklistWaitsForCompleteResultLine(t *testing.T) {
	root := t.TempDir()
	log := codexRolloutFixture(t, root, codexTestSession, codexTestSession)
	appendFile(t, log, codexPlanCall(t, "partial", `[{"step":"wait","status":"pending"}]`))
	result := codexPlanResult(t, "partial", "2026-11-13T00:00:02Z", "Plan updated")
	appendFile(t, log, result[:len(result)-1])
	if got, _ := codexChecklist(root, codexTestSession); got != nil {
		t.Fatalf("partial result applied: %#v", got)
	}
	appendFile(t, log, "\n")
	want := []ChecklistItem{{Content: "wait", Status: ChecklistPending}}
	if got, _ := codexChecklist(root, codexTestSession); !reflect.DeepEqual(got, want) {
		t.Fatalf("complete result = %#v, want %#v", got, want)
	}
}

func TestCodexChecklistShrinkDiscardsPendingCalls(t *testing.T) {
	root := t.TempDir()
	log := codexRolloutFixture(t, root, codexTestSession, codexTestSession)
	appendFile(t, log, codexPlanCall(t, "old", `[{"step":"must not survive truncation","status":"pending"}]`))
	codexChecklist(root, codexTestSession)
	if err := os.WriteFile(log, []byte(codexPlanResult(t, "old", "2026-11-13T00:00:02Z", "Plan updated")), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, _ := codexChecklist(root, codexTestSession); got != nil {
		t.Fatalf("truncated rollout retained pending call: %#v", got)
	}
}
