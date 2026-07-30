package collect

import (
	"testing"
	"time"
)

func TestDetectTool(t *testing.T) {
	cases := []struct {
		name string
		p    proc
		want string
	}{
		{"claude foreground", proc{comm: "claude", args: "claude agents --cwd ."}, "claude"},
		{"login shell", proc{comm: "-zsh", args: "-zsh"}, ""},
		{"plain zsh", proc{comm: "zsh", args: "zsh"}, ""},
		{"codex via argv0 basename", proc{comm: "node", args: "node /usr/local/bin/codex --foo"}, "codex"},
		{"absolute claude path", proc{comm: "claude", args: "/Users/x/.local/bin/claude"}, "claude"},
		{"omp via bun wrapper", proc{comm: "bun", args: "bun /Users/ky-han/.bun/bin/omp"}, "omp"},
		{"unrelated node", proc{comm: "node", args: "node server.js"}, ""},
	}
	for _, c := range cases {
		if got := detectTool(c.p); got != c.want {
			t.Errorf("%s: detectTool = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestTrimNFields(t *testing.T) {
	cases := []struct {
		line string
		n    int
		want string
	}{
		{"111 222 3.4 claude claude agents --cwd .", 4, "claude agents --cwd ."},
		{"1 2 3 4", 4, ""},
		{"1 2 3", 4, ""},
		{"  10   20  0.0  comm  the args", 4, "the args"},
	}
	for _, c := range cases {
		if got := trimNFields(c.line, c.n); got != c.want {
			t.Errorf("trimNFields(%q,%d) = %q, want %q", c.line, c.n, got, c.want)
		}
	}
}

func TestNormalizeLife(t *testing.T) {
	cases := []struct {
		state string
		live  bool
		sug   string
		want  Lifecycle
	}{
		{"done", false, "", LifeDone},
		{"working", true, "", LifeWorking},
		{"working", false, "", LifeStale}, // daemon gone mid-work
		{"stopped", true, "", LifeStopped},
		{"error", false, "", LifeError},
		{"input", false, "", LifeInput},
		{"idle", true, "", LifeIdle},
		{"", false, "please confirm", LifeInput}, // unknown state but awaiting reply
		{"weird", false, "", LifeUnknown},
	}
	for _, c := range cases {
		js := jobState{State: c.state, Suggested: c.sug}
		if got := normalizeLife(js, c.live); got != c.want {
			t.Errorf("normalizeLife(state=%q live=%v sug=%q) = %q, want %q",
				c.state, c.live, c.sug, got, c.want)
		}
	}
}

func TestModelFromFlags(t *testing.T) {
	if got := modelFromFlags([]string{"--agent", "claude", "--model", "opus[1m]"}); got != "opus[1m]" {
		t.Errorf("got %q", got)
	}
	if got := modelFromFlags([]string{"--agent", "claude"}); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
	if got := modelFromFlags([]string{"--model"}); got != "" { // dangling flag, no value
		t.Errorf("expected empty for dangling flag, got %q", got)
	}
}

func TestInteractiveLife(t *testing.T) {
	cases := []struct {
		title string
		want  Lifecycle
	}{
		{"⠂ tmux agent monitor tui", LifeWorking}, // Braille spinner (U+2802) — actively generating
		{"⠐ claude", LifeWorking},
		{"claude agents", LifeIdle},       // no spinner — at a menu/idle
		{"π: tmux_agent_check", LifeIdle}, // omp idle title, no spinner
		{"", LifeIdle},                    // no title at all
		{"   ", LifeIdle},
	}
	for _, c := range cases {
		if got := interactiveLife(c.title); got != c.want {
			t.Errorf("interactiveLife(%q) = %q, want %q", c.title, got, c.want)
		}
	}
}

func TestJobName(t *testing.T) {
	if got := jobName(jobState{Name: "my-feature\t205"}, "abc"); got != "my-feature" {
		t.Errorf("explicit name: got %q", got)
	}
	if got := jobName(jobState{RespawnFlags: []string{"--name", "feat-6\t12"}}, "abc"); got != "feat-6" {
		t.Errorf("name flag: got %q", got)
	}
	if got := jobName(jobState{DaemonShort: "82ea9a49"}, "dir"); got != "82ea9a49" {
		t.Errorf("short fallback: got %q", got)
	}
	if got := jobName(jobState{}, "dirname"); got != "dirname" {
		t.Errorf("dir fallback: got %q", got)
	}
}

func TestLifecycleTerminal(t *testing.T) {
	terminal := []Lifecycle{LifeDone, LifeStopped, LifeError}
	for _, l := range terminal {
		if !l.Terminal() {
			t.Errorf("%s should be terminal", l)
		}
	}
	live := []Lifecycle{LifeWorking, LifeInput, LifeIdle, LifeStale, LifeUnknown}
	for _, l := range live {
		if l.Terminal() {
			t.Errorf("%s should not be terminal", l)
		}
	}
}

func TestGroupOrdersProjectsAndAgents(t *testing.T) {
	now := time.Now()
	agents := []Agent{
		{ID: "a1", Kind: KindBackground, Life: LifeDone, Project: "/p/idle", UpdatedAt: now.Add(-2 * time.Hour)},
		{ID: "a2", Kind: KindBackground, Life: LifeWorking, Project: "/p/busy", UpdatedAt: now.Add(-1 * time.Minute)},
		{ID: "a3", Kind: KindInteractive, Life: LifeWorking, Project: "/p/busy", UpdatedAt: now.Add(-5 * time.Minute)},
		{ID: "a4", Kind: KindBackground, Life: LifeDone, Project: "/p/busy", UpdatedAt: now.Add(-3 * time.Hour)},
	}
	projs := group(agents)
	if len(projs) != 2 {
		t.Fatalf("want 2 projects, got %d", len(projs))
	}
	// Project with active work must come first.
	if projs[0].Key != "/p/busy" {
		t.Errorf("busy project should sort first, got %q", projs[0].Key)
	}
	// Within busy: working agents before the done one; interactive before
	// background at equal rank.
	got := []string{projs[0].Agents[0].ID, projs[0].Agents[1].ID, projs[0].Agents[2].ID}
	want := []string{"a3", "a2", "a4"} // interactive-working, bg-working, then done
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("busy agent order = %v, want %v", got, want)
			break
		}
	}
}

func TestProjectName(t *testing.T) {
	if got := projectName("/Volumes/SDWORK/git/util/tmux_agent_check"); got != "tmux_agent_check" {
		t.Errorf("got %q", got)
	}
	if got := projectName("(unknown)"); got != "(unknown)" {
		t.Errorf("got %q", got)
	}
}
