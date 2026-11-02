package collect

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const claudeSessionFixture = `{"pid":37766,"sessionId":"session-live","cwd":"/work/my_project","startedAt":1793579768640,"tmux":"work:@413.%422"}`
const claudeRunningFixture = `{"agentType":"general-purpose","description":"Review code","spawnDepth":1,"requestShape":"background"}`

func writeClaudeFixture(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o700); err != nil {
		t.Fatal(err)
	}
}

func TestClaudeSubagentsExactLiveSession(t *testing.T) {
	for _, tc := range []struct {
		name    string
		session string
		start   time.Time
		want    []Subagent
	}{
		{"live", claudeSessionFixture, time.UnixMilli(1793579768640), []Subagent{
			{ID: "nested", Description: "Nested review", Life: LifeWorking},
			{ID: "running", Description: "Review code", Life: LifeWorking},
		}},
		{"reused pid", claudeSessionFixture, time.UnixMilli(1793579768640).Add(time.Hour), nil},
		{"wrong pid", `{"pid":1,"sessionId":"session-live","cwd":"/work/my_project","startedAt":1793579768640,"tmux":"work:@413.%422"}`, time.UnixMilli(1793579768640), nil},
		{"wrong pane", `{"pid":37766,"sessionId":"session-live","cwd":"/work/my_project","startedAt":1793579768640,"tmux":"work:@413.%99"}`, time.UnixMilli(1793579768640), nil},
		{"malformed session", `{`, time.UnixMilli(1793579768640), nil},
		{"missing identity", `{}`, time.UnixMilli(1793579768640), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeClaudeFixture(t, filepath.Join(root, "sessions", "37766.json"), tc.session)
			dir := filepath.Join(root, "projects", "-work-my-project", "session-live", "subagents")
			for id, data := range map[string]string{
				"running": claudeRunningFixture,
				"nested":  `{"agentType":"general-purpose","description":"Nested review","spawnDepth":2,"parentAgentId":"running","requestShape":"background"}`,
				"done":    `{"agentType":"general-purpose","spawnDepth":1,"requestShape":"background","finished":true}`,
				"stale":   `{"agentType":"general-purpose","spawnDepth":2,"parentAgentId":"done","requestShape":"background"}`,
				"orphan":  `{"agentType":"general-purpose","spawnDepth":2,"parentAgentId":"missing","requestShape":"background"}`,
				"cycle":   `{"agentType":"general-purpose","spawnDepth":2,"parentAgentId":"cycle","requestShape":"background"}`,
				"broken":  `{`,
				"empty":   `{}`,
			} {
				writeClaudeFixture(t, filepath.Join(dir, "agent-"+id+".meta.json"), data)
			}
			// A newer project session and another PID must never be substituted.
			writeClaudeFixture(t, filepath.Join(root, "projects", "-work-my-project", "session-newer", "subagents", "agent-unrelated.meta.json"), claudeRunningFixture)
			writeClaudeFixture(t, filepath.Join(root, "sessions", "999.json"), claudeSessionFixture)
			got, _ := claudeSubagents(root, proc{pid: 37766, startedAt: tc.start}, pane{paneID: "%422"})
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("subagents = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestClaudeSubagentsRefreshCompletionAndMissingData(t *testing.T) {
	root := t.TempDir()
	parent := proc{pid: 37766, startedAt: time.UnixMilli(1793579768640)}
	pn := pane{paneID: "%422"}
	if got, _ := claudeSubagents(root, parent, pn); len(got) != 0 {
		t.Fatalf("missing session: %#v", got)
	}
	writeClaudeFixture(t, filepath.Join(root, "sessions", "37766.json"), claudeSessionFixture)
	child := filepath.Join(root, "projects", "-work-my-project", "session-live", "subagents", "agent-running.meta.json")
	writeClaudeFixture(t, child, claudeRunningFixture)
	if got, _ := claudeSubagents(root, parent, pn); len(got) != 1 || got[0].Life != LifeWorking {
		t.Fatalf("running child: %#v", got)
	}
	writeClaudeFixture(t, child, `{"agentType":"general-purpose","spawnDepth":1,"requestShape":"background","finished":true}`)
	if got, _ := claudeSubagents(root, parent, pn); len(got) != 0 {
		t.Fatalf("completed child must disappear immediately: %#v", got)
	}
}

func TestInteractiveClaudeSubagentsPromoteIdleButPreserveInput(t *testing.T) {
	for _, tc := range []struct {
		name    string
		preview string
		want    Lifecycle
		status  string
		child   bool
	}{
		{"idle", "", LifeWorking, "", true},
		{"input", "Enter to select · ↑/↓ to navigate · Esc to cancel", LifeInput, "", true},
		{"busy team", "", LifeWorking, "busy", false},
		{"idle team", "", LifeIdle, "idle", false},
		{"busy input", "Enter to select · ↑/↓ to navigate · Esc to cancel", LifeInput, "busy", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			bin := filepath.Join(home, "bin")
			writeClaudeFixture(t, filepath.Join(bin, "tmux"), "#!/bin/sh\nprintf '%s\\n' '"+tc.preview+"'\n")
			t.Setenv("PATH", bin)
			root := filepath.Join(home, ".claude")
			session := strings.TrimSuffix(claudeSessionFixture, "}") + `,"status":"` + tc.status + `"}`
			writeClaudeFixture(t, filepath.Join(root, "sessions", "37766.json"), session)
			child := claudeRunningFixture
			if !tc.child {
				child = `{"agentType":"verify-a","spawnDepth":0,"requestShape":"background","taskKind":"in_process_teammate"}`
			}
			writeClaudeFixture(t, filepath.Join(root, "projects", "-work-my-project", "session-live", "subagents", "agent-running.meta.json"), child)
			parent := proc{pid: 37766, comm: "claude", startedAt: time.UnixMilli(1793579768640)}
			procs := &procTable{byPID: map[int]proc{37766: parent}}
			got := interactiveAgents(procs, []pane{{pid: 37766, paneID: "%422", path: home, target: "work:1.1"}})
			wantChildren := 0
			if tc.child {
				wantChildren = 1
			}
			if len(got) != 1 || got[0].Life != tc.want || len(got[0].Subagents) != wantChildren {
				t.Fatalf("interactive collection = %#v, want parent %s and %d running children", got, tc.want, wantChildren)
			}
		})
	}
}
