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
		{"omp via bun wrapper", proc{comm: "bun", args: "bun /Users/x/.bun/bin/omp"}, "omp"},
		{"opencode foreground", proc{comm: "opencode", args: "opencode"}, "opencode"},
		{"opencode executable suffix", proc{comm: "opencode.exe", args: "opencode.exe"}, "opencode"},
		{"opencode absolute argv", proc{comm: "bun", args: "/opt/homebrew/bin/opencode"}, "opencode"},
		{"unrelated node", proc{comm: "node", args: "node server.js"}, ""},
		{"ordinary filename named opencode", proc{comm: "cat", args: "cat /tmp/opencode"}, ""},
		{"later node argument named opencode", proc{comm: "node", args: "node server.js opencode"}, ""},
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

func TestLooksLikeChoicePrompt(t *testing.T) {
	// Verified live: this is the real footer Claude Code renders while a
	// question/AskUserQuestion-style choice is pending.
	pending := []string{
		"────────────────────────────────────────",
		" ☐ Choice",
		"",
		"Pick A or B?",
		"",
		"❯ 1. A",
		"     Option A",
		"  2. B",
		"     Option B",
		"────────────────────────────────────────",
		"Enter to select · ↑/↓ to navigate · Esc to cancel",
	}
	if !looksLikeChoicePrompt("claude", pending) {
		t.Error("expected pending choice prompt to be detected")
	}

	// Verified live against OpenCode 1.18.5's question tool.
	openCodePending := []string{
		"Alpha와 Beta 중 하나를 선택해 주세요.",
		"",
		"1. Alpha",
		"   Alpha를 선택합니다.",
		"2. Beta",
		"   Beta를 선택합니다.",
		"3. Type your own answer",
		"",
		"↑↓ select  enter submit  esc dismiss",
	}
	if !looksLikeChoicePrompt("opencode", openCodePending) {
		t.Error("expected OpenCode question prompt to be detected")
	}

	// The bare "❯ " cursor glyph alone appears in ordinary idle/typing
	// screens too — must not trigger a false positive on its own.
	idleTyping := []string{
		"──────────────────────────────",
		"❯ describe a task for a new session",
		"──────────────────────────────",
		"  enter to open · space to reply · ctrl+x to delete · ? for shortcuts",
	}
	if looksLikeChoicePrompt("claude", idleTyping) {
		t.Error("idle typing prompt (bare ❯, no choice footer) must not match")
	}

	working := []string{
		"❯ do the thing",
		"",
		"⏺ Running 1 shell command…",
		"",
		"✽ Pouncing… (4s · ↓ 75 tokens · thought for 2s)",
	}
	if looksLikeChoicePrompt("claude", working) {
		t.Error("actively-working screen must not match")
	}

	if looksLikeChoicePrompt("claude", nil) {
		t.Error("nil lines must not match")
	}
}

func TestLooksLikeChoicePromptIgnoresScrollbackMentions(t *testing.T) {
	// Reproduces a real false positive: the marker phrase appearing in old
	// scrollback (e.g. prose that quotes it, or a past genuine prompt that has
	// since scrolled up) must not count — only the current bottom of screen.
	lines := make([]string, 0, 20)
	lines = append(lines, "some earlier message that happens to quote the exact")
	lines = append(lines, "footer text: Enter to select · ↑/↓ to navigate · Esc to cancel")
	lines = append(lines, "")
	for i := 0; i < 15; i++ {
		lines = append(lines, "later, ordinary output line")
	}
	lines = append(lines, "❯ ")
	lines = append(lines, "working normally, nothing pending")

	if looksLikeChoicePrompt("claude", lines) {
		t.Error("marker text buried in scrollback (not the current bottom of screen) must not match")
	}

	// Sanity: the same marker at the actual bottom still matches.
	lines[len(lines)-1] = "Enter to select · ↑/↓ to navigate · Esc to cancel"
	if !looksLikeChoicePrompt("claude", lines) {
		t.Error("marker text at the current bottom of screen should still match")
	}

	lines[len(lines)-1] = "↑↓ select  enter submit  esc dismiss"
	if !looksLikeChoicePrompt("opencode", lines) {
		t.Error("OpenCode question footer at the current bottom should match")
	}
}

func TestLooksLikeChoicePromptStripsANSI(t *testing.T) {
	// capturePane now captures with -e (color codes preserved for the live
	// preview); detection must still match a footer wrapped in real SGR codes.
	colored := []string{
		"\x1b[38;5;214m ☐ Choice\x1b[0m",
		"Pick A or B?",
		"\x1b[1m❯ 1. A\x1b[0m",
		"  2. B",
		"\x1b[38;5;240mEnter to select · ↑/↓ to navigate · Esc to cancel\x1b[0m",
	}
	if !looksLikeChoicePrompt("claude", colored) {
		t.Error("choice prompt wrapped in ANSI color codes should still be detected")
	}

	openCodeColored := []string{
		"1. Alpha",
		"2. Beta",
		"\x1b[38;5;240m↑↓ select  enter submit  esc dismiss\x1b[0m",
	}
	if !looksLikeChoicePrompt("opencode", openCodeColored) {
		t.Error("ANSI-wrapped OpenCode question footer should be detected")
	}
	if looksLikeChoicePrompt("claude", openCodeColored) {
		t.Error("OpenCode question footer must not affect a Claude pane")
	}
}

func TestStripANSI(t *testing.T) {
	in := "\x1b[38;5;214mhello\x1b[0m world\x1b[1m!\x1b[0m"
	if got := stripANSI(in); got != "hello world!" {
		t.Errorf("stripANSI(%q) = %q, want %q", in, got, "hello world!")
	}
	if got := stripANSI("plain text"); got != "plain text" {
		t.Errorf("stripANSI on plain text should be a no-op, got %q", got)
	}
}

func TestContentShowsSpinner(t *testing.T) {
	// Real omp fixtures, captured live mid-task.
	working1 := []string{
		" Todos", "  Review · 0/2", "",
		" ⠸ Running Go test suite ⟨esc⟩", "",
	}
	if !contentShowsSpinner("omp", working1) {
		t.Error("expected omp's in-body spinner line to be detected")
	}
	working2 := []string{
		"╰──────────────╯", "",
		" ⠦ Creating Claude config guide ⟨esc⟩", "",
	}
	if !contentShowsSpinner("omp", working2) {
		t.Error("expected second omp spinner fixture to be detected")
	}

	// Real OpenCode 1.18.5 fixture captured live mid-task. Its spinner sits
	// inside a bordered row, while the footer supplies the stable working marker.
	openCodeWorking := []string{
		"  ┃  ⠸ python3 download.py --start 1 --end 99999",
		"  ┃",
		"  ⬝⬝⬝⬝■■■■  esc interrupt      60.7K (12%)  ctrl+p commands  • OpenCode 1.18.5",
	}
	if !contentShowsSpinner("opencode", openCodeWorking) {
		t.Error("expected OpenCode's active-task footer to be detected")
	}

	openCodeColored := []string{
		"\x1b[38;5;240m  ⬝⬝■■  esc interrupt\x1b[0m",
	}
	if !contentShowsSpinner("opencode", openCodeColored) {
		t.Error("expected ANSI-wrapped OpenCode footer to be detected")
	}
	if contentShowsSpinner("claude", openCodeWorking) {
		t.Error("OpenCode footer must not mark a Claude pane as working")
	}

	// Real omp idle fixture: static "π: <task>" title context, no spinner
	// anywhere in the body — verified against the same tool's idle output.
	idle := []string{
		"", "Recent sessions", "No recent sessions", "",
		"╭──     GPT-5.6-Terra++ · low   ─────────────────────────────╮",
	}
	if contentShowsSpinner("omp", idle) {
		t.Error("idle omp content must not be detected as working")
	}

	openCodeIdle := []string{
		"  ┃",
		"  ┃",
		"  Hephaestus - Deep Agent · GPT-5.6 Terra Fast OpenAI · high",
		"  60.7K (12%)  ctrl+p commands  • OpenCode 1.18.5",
	}
	if contentShowsSpinner("opencode", openCodeIdle) {
		t.Error("idle OpenCode content must not be detected as working")
	}

	// Scrollback guard: a spinner line buried above the scan window must not
	// match, same reasoning as the choice-prompt scrollback fix.
	buried := append([]string{"⠸ old, scrolled-off spinner from a past task"},
		make([]string, 15)...)
	buried = append(buried, "ordinary idle line")
	for i := range buried {
		if buried[i] == "" {
			buried[i] = "filler"
		}
	}
	if contentShowsSpinner("omp", buried) {
		t.Error("a spinner line outside the trailing scan window must not match")
	}

	openCodeBuried := append([]string{"⬝⬝■■  esc interrupt"},
		make([]string, 15)...)
	openCodeBuried = append(openCodeBuried, "ordinary OpenCode idle footer")
	for i := range openCodeBuried {
		if openCodeBuried[i] == "" {
			openCodeBuried[i] = "filler"
		}
	}
	if contentShowsSpinner("opencode", openCodeBuried) {
		t.Error("an OpenCode working footer outside the trailing scan window must not match")
	}
}

func TestIsSpinnerGlyph(t *testing.T) {
	for _, r := range []rune{'⠂', '⠐', '⠸', '⠦'} {
		if !isSpinnerGlyph(r) {
			t.Errorf("%q (U+%04X) should be a spinner glyph", r, r)
		}
	}
	for _, r := range []rune{'a', '가', '✻', '✽', 'π', ' '} {
		if isSpinnerGlyph(r) {
			t.Errorf("%q (U+%04X) should not be a spinner glyph", r, r)
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
