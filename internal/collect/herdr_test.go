package collect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func herdrFixture(t *testing.T, mode string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("PATH", dir)
	t.Setenv("AMUX_HERDR_MODE", mode)
	t.Setenv("AMUX_PS_TRACE", filepath.Join(dir, "ps.calls"))
	scripts := map[string]string{
		"ps": `#!/bin/sh
printf 'ps\n' >> "$AMUX_PS_TRACE"
printf '%s\n' '100 1 0.0 01:00 zsh -zsh' '101 100 2.0 00:30 opencode opencode' '102 101 3.0 00:10 worker worker' '110 1 0.0 01:00 zsh -zsh'
`,
		"herdr": `#!/bin/sh
if [ "$1" = session ]; then
  case "$AMUX_HERDR_MODE" in
    empty) printf '{"sessions":[]}\n'; exit 0 ;;
    inventory) printf '{}\n'; exit 0 ;;
  esac
  printf '{"sessions":[{"name":"alpha","running":true},{"name":"beta","running":true},{"name":"stopped","running":false}]}\n'
  exit 0
fi
[ "$1" = --session ] || exit 10
session=$2
shift 2
case "$session:$1:$2" in
  alpha:api:snapshot)
    printf '{"result":{"snapshot":{"tabs":[{"tab_id":"w1:t1","label":"review"}],"panes":[{"pane_id":"w1:p1","terminal_id":"term_a","workspace_id":"w1","tab_id":"w1:t1","cwd":"/base","foreground_cwd":"%s","terminal_title":"Agent","agent_session":{"agent":"opencode","kind":"id","value":"%s"}},{"pane_id":"w1:p2","cwd":"/shell","agent_status":"working"}]}}}\n' "$HOME" "$AMUX_HERDR_AGENT_SESSION"
    ;;
  beta:api:snapshot)
    case "$AMUX_HERDR_MODE" in
      partial) printf 'socket gone\n' >&2; exit 2 ;;
      malformed) printf '{broken\n'; exit 0 ;;
      missing) printf '{}\n'; exit 0 ;;
      paneid) printf '{"result":{"snapshot":{"panes":[{}]}}}\n'; exit 0 ;;
    esac
    printf '{"result":{"snapshot":{"panes":[{"pane_id":"w1:p1","terminal_id":"term_b","workspace_id":"w1","tab_id":"w1:t2","cwd":"/fallback","terminal_title":"Codex","agent_session":{"agent":"codex","kind":"id","value":"%s"}}]}}}\n' "$AMUX_HERDR_CODEX_SESSION"
    ;;
  alpha:pane:process-info)
    case "$4" in
      w1:p1) printf '{"result":{"process_info":{"shell_pid":100,"foreground_processes":[]}}}\n' ;;
      w1:p2) printf '{"result":{"process_info":{"shell_pid":110,"foreground_processes":[{"pid":110,"name":"zsh","cmdline":"zsh"}]}}}\n' ;;
      *) exit 11 ;;
    esac
    ;;
  beta:pane:process-info)
    [ "$AMUX_HERDR_MODE" != process ] || exit 3
    printf '{"result":{"process_info":{"shell_pid":200,"foreground_processes":[{"pid":201,"name":"node","argv":["node","/opt/bin/codex"]}]}}}\n'
    ;;
  alpha:pane:read)
    [ "$AMUX_HERDR_MODE" != capture ] || exit 4
    printf '\033[32mesc interrupt\033[0m   \r\n'
    ;;
  beta:pane:read) printf 'Ready\n' ;;
  *) printf 'unexpected command\n' >&2; exit 12 ;;
esac
`,
	}
	for name, script := range scripts {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestCollectHerdrNormalizesAgentsAcrossSessions(t *testing.T) {
	dir := herdrFixture(t, "")
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)

	snap, err := CollectBackend(now, BackendHerdr)

	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Agents) != 2 || !snap.Taken.Equal(now) {
		t.Fatalf("snapshot = %+v, want two recognized agents", snap)
	}
	a, b := snap.Agents[0], snap.Agents[1]
	if a.ID != "herdr:alpha:w1:p1" || a.Backend != BackendHerdr || a.Session != "alpha" ||
		a.PaneID != "w1:p1" || a.TerminalID != "term_a" || a.WorkspaceID != "w1" || a.TabID != "w1:t1" ||
		a.TmuxTarget != "" || a.Tool != "opencode" || a.Kind != KindInteractive {
		t.Fatalf("alpha identity = %+v", a)
	}
	if a.PID != 101 || a.CPU != 5 || a.ProcessStartedAt.IsZero() || a.Life != LifeWorking ||
		a.CWD != dir || len(a.Preview) != 1 || a.Preview[0] != "\x1b[32mesc interrupt\x1b[0m" {
		t.Fatalf("shared process/lifecycle/preview enrichment = %+v", a)
	}
	if a.Window != "review" || a.PaneTitle != "Agent" {
		t.Fatalf("tab label must survive collection independently of terminal title: window=%q title=%q", a.Window, a.PaneTitle)
	}
	if b.ID != "herdr:beta:w1:p1" || b.Tool != "codex" || b.PID != 201 ||
		b.CWD != "/fallback" || b.TerminalID != "term_b" || b.Life != LifeIdle {
		t.Fatalf("foreground argv fallback = %+v", b)
	}
	if len(snap.LivePaneIDs) != 3 || !snap.LivePaneIDs[a.ID] || !snap.LivePaneIDs[b.ID] ||
		!snap.LivePaneIDs["herdr:alpha:w1:p2"] {
		t.Fatalf("namespaced inventory must retain ordinary shells: %v", snap.LivePaneIDs)
	}
	calls, err := os.ReadFile(filepath.Join(dir, "ps.calls"))
	if err != nil || string(calls) != "ps\n" {
		t.Fatalf("one ps per refresh: %q, %v", calls, err)
	}
}

func TestCollectHerdrChecklistFollowsSelectedOpenCodeSession(t *testing.T) {
	home := herdrFixture(t, "")
	data := filepath.Join(home, "data")
	t.Setenv("XDG_DATA_HOME", data)
	for _, id := range []string{"ses_first", "ses_second"} {
		writeClaudeFixture(t, filepath.Join(data, "opencode", "storage", "todo", id+".json"),
			`[{"content":"`+id+` task","status":"in_progress"}]`)
	}
	for _, id := range []string{"ses_first", "ses_second", ""} {
		t.Setenv("AMUX_HERDR_AGENT_SESSION", id)
		snap, err := CollectBackend(time.Now(), BackendHerdr)
		if err != nil {
			t.Fatal(err)
		}
		got := snap.Agents[0].Checklist
		if id == "" {
			if len(got) != 0 {
				t.Fatalf("missing session borrowed a plan: %#v", got)
			}
		} else if len(got) != 1 || got[0].Content != id+" task" || got[0].Status != ChecklistActive {
			t.Fatalf("selected %s: checklist = %#v", id, got)
		}
	}
}

func TestCollectHerdrChecklistFollowsSelectedCodexSession(t *testing.T) {
	home := herdrFixture(t, "")
	root := filepath.Join(home, "codex-data")
	t.Setenv("CODEX_HOME", root)
	for _, id := range []string{codexTestSession, codexOtherSession} {
		log := codexRolloutFixture(t, root, id, id)
		appendFile(t, log, codexPlanCall(t, "plan", `[{"step":"`+id+`","status":"pending"}]`)+
			codexPlanResult(t, "plan", "2026-11-13T00:00:02Z", "Plan updated"))
	}
	for _, id := range []string{codexTestSession, codexOtherSession, ""} {
		t.Setenv("AMUX_HERDR_CODEX_SESSION", id)
		snap, err := CollectBackend(time.Now(), BackendHerdr)
		if err != nil {
			t.Fatal(err)
		}
		got := snap.Agents[1].Checklist
		if id == "" {
			if len(got) != 0 {
				t.Fatalf("missing session borrowed a plan: %#v", got)
			}
		} else if len(got) != 1 || got[0].Content != id || got[0].Status != ChecklistPending {
			t.Fatalf("selected %s: checklist = %#v", id, got)
		}
	}
}

func TestCollectHerdrIncompleteInventoryPreservesNotes(t *testing.T) {
	for _, mode := range []string{"partial", "malformed", "missing", "paneid", "process", "inventory", "absent"} {
		t.Run(mode, func(t *testing.T) {
			dir := herdrFixture(t, mode)
			wantAgents := 1
			if mode == "inventory" || mode == "absent" {
				wantAgents = 0
			}
			if mode == "absent" {
				if err := os.Remove(filepath.Join(dir, "herdr")); err != nil {
					t.Fatal(err)
				}
			}

			snap, err := CollectBackend(time.Now(), BackendHerdr)

			if err == nil || !strings.Contains(err.Error(), "herdr") || snap.LivePaneIDs != nil ||
				len(snap.Agents) != wantAgents {
				t.Fatalf("partial discovery must return agents, error and unsafe inventory: %+v, %v", snap, err)
			}
		})
	}
}

func TestCollectHerdrCaptureFailureRetainsAgent(t *testing.T) {
	herdrFixture(t, "capture")

	snap, err := CollectBackend(time.Now(), BackendHerdr)

	if err != nil || len(snap.Agents) != 2 || snap.LivePaneIDs == nil {
		t.Fatalf("capture failure dropped inventory: %+v, %v", snap, err)
	}
	if a := snap.Agents[0]; len(a.Preview) != 0 || a.Life != LifeIdle || a.PID != 101 {
		t.Fatalf("capture failure must preserve process with unavailable preview: %+v", a)
	}
}

func TestCollectHerdrEmptyInventoryIsComplete(t *testing.T) {
	herdrFixture(t, "empty")

	snap, err := CollectBackend(time.Now(), BackendHerdr)

	if err != nil || snap.LivePaneIDs == nil || len(snap.LivePaneIDs) != 0 || len(snap.Agents) != 0 {
		t.Fatalf("empty complete inventory = %+v, %v", snap, err)
	}
}

func TestCollectRetainsTmuxIdentity(t *testing.T) {
	dir := herdrFixture(t, "")
	script := "#!/bin/sh\ncase \"$1\" in\nlist-panes) printf 's\\037win\\037s:1.0\\037%%7\\037100\\037%s\\037Agent\\0371\\n' \"$HOME\" ;;\ncapture-pane) printf 'Ready\\n' ;;\n*) exit 1 ;;\nesac\n"
	if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	snap, err := Collect(time.Now())

	if err != nil || len(snap.Agents) != 1 {
		t.Fatalf("tmux snapshot = %+v, %v", snap, err)
	}
	a := snap.Agents[0]
	if a.ID != "%7" || a.PaneID != "%7" || a.Backend != BackendTmux || a.TmuxTarget != "s:1.0" ||
		!snap.LivePaneIDs["%7"] || a.TerminalID != "" {
		t.Fatalf("tmux identity changed: %+v", a)
	}
}

func TestHerdrClaudeSubagentsUseLiveProcessOwnership(t *testing.T) {
	for _, tc := range []struct {
		name    string
		preview []string
		start   time.Time
		life    Lifecycle
		count   int
	}{
		{"working child", nil, time.UnixMilli(1793579768640), LifeWorking, 1},
		{"input wins", []string{"Enter to select · ↑/↓ to navigate · Esc to cancel"}, time.UnixMilli(1793579768640), LifeInput, 1},
		{"reused pid", nil, time.UnixMilli(1793579768640).Add(time.Hour), LifeIdle, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			root := filepath.Join(home, ".claude")
			session := strings.Replace(claudeSessionFixture, `"tmux":"work:@413.%422"`, `"tmux":""`, 1)
			writeClaudeFixture(t, filepath.Join(root, "sessions", "37766.json"), session)
			writeClaudeFixture(t, filepath.Join(root, "projects", "-work-my-project", "session-live", "subagents", "agent-running.meta.json"), claudeRunningFixture)
			ap := proc{pid: 37766, comm: "claude", startedAt: tc.start}
			procs := &procTable{byPID: map[int]proc{ap.pid: ap}}
			pn := pane{backend: BackendHerdr, paneID: "w1:p1", path: home}

			a := collectInteractivePreview(procs, pn, ap, home, true, tc.preview)

			if a.Life != tc.life || len(a.Subagents) != tc.count {
				t.Fatalf("Herdr Claude enrichment = %+v, want %s with %d children", a, tc.life, tc.count)
			}
		})
	}
}
