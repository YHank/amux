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
		{"omo foreground", proc{comm: "omo", args: "omo"}, "omo"},
		{"omo display process title", proc{comm: "OmO", args: "OmO"}, "omo"},
		{"omo absolute executable", proc{comm: "/Users/x/.local/bin/omo", args: "/Users/x/.local/bin/omo"}, "omo"},
		{"omo versioned runtime with truncated comm", proc{comm: "/Users/x/.o", args: "/Users/x/.omo/binary-runtime/5.1.30/omo"}, "omo"},
		{"omo via bun wrapper", proc{comm: "bun", args: "bun /Users/x/.local/bin/omo"}, "omo"},
		{"omo via node wrapper", proc{comm: "node", args: "node --no-warnings /usr/local/bin/omo"}, "omo"},
		{"ordinary filename named omo", proc{comm: "cat", args: "cat /tmp/omo"}, ""},
		{"ordinary filename named OmO", proc{comm: "cat", args: "cat /tmp/OmO"}, ""},
		{"later bun argument named omo", proc{comm: "bun", args: "bun server.js omo"}, ""},
		{"later node argument named OmO", proc{comm: "node", args: "node server.js OmO"}, ""},
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

func TestFindOMOAgentProc(t *testing.T) {
	procs := procTable{
		byPID: map[int]proc{
			100: {pid: 100, comm: "-zsh", args: "-zsh"},
			101: {pid: 101, ppid: 100, comm: "/Users/x/.o", args: "/Users/x/.omo/binary-runtime/5.1.30/omo"},
			102: {pid: 102, ppid: 101, comm: "/Users/x/.o", args: "/Users/x/.omo/binary-runtime/5.1.30/omo --internal-rpc-host-supervisor"},
		},
		children: map[int][]int{100: {101}, 101: {102}},
	}
	got, ok := procs.findAgentProc(100)
	if !ok || got.pid != 101 || detectTool(got) != "omo" {
		t.Fatalf("findAgentProc = %#v, %v; want the foreground omo process", got, ok)
	}
}

func TestPaneIDsIncludeUnrecognizedAgents(t *testing.T) {
	panes := []pane{
		{paneID: "%320", pid: 100},
		{paneID: "%321", pid: 200},
	}
	procs := &procTable{
		byPID: map[int]proc{
			100: {pid: 100, comm: "future-agent", args: "future-agent"},
		},
		children: map[int][]int{},
	}
	if agents := interactiveAgents(procs, panes); len(agents) != 0 {
		t.Fatalf("unrecognized or missing processes should not produce agents: %#v", agents)
	}
	ids := paneIDs(panes)
	if !ids["%320"] || !ids["%321"] || len(ids) != 2 {
		t.Fatalf("note retention must include both existing panes: %#v", ids)
	}
	if ids := paneIDs(nil); ids == nil || len(ids) != 0 {
		t.Fatalf("a successful empty inventory must differ from a failed scan: %#v", ids)
	}
}

func TestParsePaneLine(t *testing.T) {
	pn, ok := parsePaneLine("work\x1feditor\x1fwork:2.1\x1f%7\x1f1234\x1f/tmp/project\x1fClaude\x1f1")
	if !ok {
		t.Fatal("parsePaneLine returned false for a complete pane line")
	}
	if pn.session != "work" || pn.window != "editor" || pn.target != "work:2.1" ||
		pn.paneID != "%7" || pn.pid != 1234 || pn.path != "/tmp/project" ||
		pn.title != "Claude" || !pn.active {
		t.Errorf("parsePaneLine() = %#v, want all pane fields preserved", pn)
	}

	if _, ok := parsePaneLine("work\x1feditor\x1fwork:2.1\x1f%7\x1f1234\x1f/tmp/project\x1fClaude"); ok {
		t.Error("parsePaneLine accepted a line with fewer than eight fields")
	}
}

func TestAppendLiveBackgroundsExcludesPersistedRecords(t *testing.T) {
	interactive := Agent{ID: "pane", Kind: KindInteractive, Tool: "claude"}
	live := Agent{ID: "live", Kind: KindBackground, Tool: "claude", Live: true}
	deadClaude := Agent{ID: "dead-claude", Kind: KindBackground, Tool: "claude"}
	deadOMP := Agent{ID: "dead-omp", Kind: KindBackground, Tool: "omp"}

	got := appendLiveBackgrounds([]Agent{interactive}, []Agent{live, deadClaude, deadOMP})
	if len(got) != 2 || got[0].ID != "pane" || got[1].ID != "live" {
		t.Fatalf("live background filter = %#v, want only interactive and live background", got)
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

func TestParseProcLineUsesElapsedTimeForProcessStart(t *testing.T) {
	capturedAt := time.Date(2026, time.August, 19, 12, 0, 0, 0, time.UTC)
	p, ok := parseProcLine("123 1 2.5 01:30 claude claude --cwd /tmp/project", capturedAt)
	if !ok {
		t.Fatal("parseProcLine returned false for a complete process row")
	}
	if p.pid != 123 || p.ppid != 1 || p.cpu != 2.5 || p.comm != "claude" || p.args != "claude --cwd /tmp/project" {
		t.Errorf("parseProcLine() = %#v, want parsed process fields", p)
	}
	if want := capturedAt.Add(-90 * time.Second); !p.startedAt.Equal(want) {
		t.Errorf("startedAt = %v, want %v", p.startedAt, want)
	}

	if elapsed, ok := parseElapsed("2-03:04:05"); !ok || elapsed != 51*time.Hour+4*time.Minute+5*time.Second {
		t.Errorf("parseElapsed() = %v, %v; want 51h4m5s, true", elapsed, ok)
	}
	if _, ok := parseElapsed("1-02:03"); ok {
		t.Error("parseElapsed accepted a day-prefixed elapsed time without hours")
	}

	p, ok = parseProcLine("123 1 2.5 unknown claude claude", capturedAt)
	if !ok {
		t.Fatal("parseProcLine rejected a process row with an unavailable elapsed time")
	}
	if !p.startedAt.IsZero() {
		t.Errorf("startedAt = %v, want zero for invalid elapsed time", p.startedAt)
	}
}

func TestPreviewFingerprintExcludesVolatileRows(t *testing.T) {
	lines := []string{
		"\x1b[31m⠐ working task\x1b[0m",
		"↓ 12.9k",
		"tokens: 12345 · 15 tok/s",
		"reduce tokens before retry",
		"press ↓ to continue",
		"result contains ⠐ symbol",
		"processed ↓ 12.9k rows",
		"\x1b[32mmeaningful output\x1b[0m",
	}
	want := "reduce tokens before retry\npress ↓ to continue\nresult contains ⠐ symbol\nprocessed ↓ 12.9k rows\nmeaningful output"
	if got := previewFingerprint(lines); got != want {
		t.Errorf("preview fingerprint = %q, want %q", got, want)
	}
}

// A working Claude Code pane reprints its status row every second with a new
// elapsed time. Left in the fingerprint it would report activity on every
// refresh, which both reorders the flat view constantly and — because the
// debounce only accepts two identical readings in a row — hides the real
// output changes underneath it.
func TestPreviewFingerprintExcludesClaudeStatusRow(t *testing.T) {
	first := previewFingerprint([]string{
		"⏺ Update(internal/ui/app.go)",
		"· Doodling… (3m 29s · ↓ 9.7k tokens)",
	})
	second := previewFingerprint([]string{
		"⏺ Update(internal/ui/app.go)",
		"✳ Doodling… (3m 31s · ↓ 9.9k tokens · esc to interrupt)",
	})
	if first != second {
		t.Errorf("ticking status row changed the fingerprint: %q vs %q", first, second)
	}
	if first != "⏺ Update(internal/ui/app.go)" {
		t.Errorf("fingerprint = %q, want the real output line only", first)
	}
}

// A partially drawn screen leaves blank rows under its last real output. They
// must not consume the scan window, or a prompt footer sitting just inside it
// scrolls out of view and the pane silently stops reporting that it is waiting.
func TestTrailingBlankRowsDoNotShrinkTheScanWindow(t *testing.T) {
	pending := []string{
		"Pick A or B?",
		"❯ 1. A",
		"  2. B",
		"Enter to select · ↑/↓ to navigate · Esc to cancel",
		"", "", "", "", "", "", "", "",
	}
	if !looksLikeChoicePrompt("claude", pending) {
		t.Error("blank bottom rows hid a pending choice prompt from the scan window")
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
		// Claude Code 2.1.228 titles, captured live. The ◐/◑ panes were the
		// ones with an active token counter in the body; they regressed to
		// LifeIdle when only Braille counted as a spinner.
		{"◑ Investigate running agent showing idle state", LifeWorking},
		{"◐ Claude Code", LifeWorking},
		// ✳ leads the title of a Claude Code pane sitting at an empty prompt.
		// It looks spinner-adjacent but means the opposite; the panes below
		// were all verified idle from their bodies.
		{"✳ 학생 상담 및 분석용 API 8개 제작", LifeIdle},
		{"✳ Claude Code", LifeIdle},
		{"claude agents", LifeIdle},                  // no spinner — at a menu/idle
		{"π: tmux_agent_check", LifeIdle},            // omp idle title, no spinner
		{"OmO - translate text - project", LifeIdle}, // omo title stays static at rest
		{"", LifeIdle}, // no title at all
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

func TestOMOContentShowsSpinner(t *testing.T) {
	// Reuse pi's supported spinner format; the idle layout was observed in
	// a live OmO pane. No title or footer branding indicates active work.
	working := []string{"\x1b[36m ⠸ Running Go test suite ⟨esc⟩\x1b[0m", "", "❯", "project • main", "(😺 OmO Native by Q Kim)"}
	if !contentShowsSpinner("omo", working) {
		t.Error("expected omo's ANSI-wrapped pi spinner to indicate work")
	}
	idle := []string{"Finished the task.", "────────────────", "❯", "────────────────", "project • main", "(😺 OmO Native by Q Kim)"}
	if contentShowsSpinner("omo", idle) {
		t.Error("omo's idle prompt and branding must not indicate work")
	}
	oldSpinner := append([]string{"⠸ Previous task"}, make([]string, contentSpinnerScanLines)...)
	oldSpinner[len(oldSpinner)-1] = "❯"
	if contentShowsSpinner("omo", oldSpinner) {
		t.Error("omo spinner outside the trailing scan window must be ignored")
	}
}

func TestOMONativeWorkingBorder(t *testing.T) {
	// Captured live from OmO panes working without a leading spinner and
	// from idle/completed panes. The active border sits above the prompt.
	working := []string{
		"[•] Verify commands",
		"[ ] Report results",
		"── • Working (6s • esc to interrupt) ────────────────────",
		"❯",
		"──────────────────────────────────────────────────────",
		"/project • main • $1.026 • 119K/400K (29.7%) (auto)",
		"(😺 OmO Native by Q Kim) 🤖 5 Pursuing goal (6m)",
	}
	idle := []string{
		"Finished the task.",
		"──────────────────────────────────────────────────────",
		"❯",
		"──────────────────────────────────────────────────────",
		"/project • main • $6.520 • 261K/400K (65.3%) (auto)",
		"(😺 OmO Native by Q Kim) 🤖 5 Goal achieved (13m)",
	}
	oldBorder := append([]string{working[2]}, make([]string, contentSpinnerScanLines)...)
	oldBorder[len(oldBorder)-1] = "❯"
	cases := []struct {
		name  string
		lines []string
		want  bool
	}{
		{"working without a spinner", working, true},
		{"ANSI-wrapped border", []string{"\x1b[36m── • Working (3s • esc to interrupt) ──\x1b[0m", "❯"}, true},
		{"idle after completion", idle, false},
		{"goal text alone", []string{"❯", "(😺 OmO Native by Q Kim) 🤖 Pursuing goal (6m)"}, false},
		{"ordinary working text", []string{"Working (6s • esc to interrupt)", "❯"}, false},
		{"quoted border", []string{"> ── • Working (6s • esc to interrupt) ──", "❯"}, false},
		{"missing interrupt marker", []string{"── • Working (6s) ──", "❯"}, false},
		{"border in scrollback", oldBorder, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := contentShowsSpinner("omo", tc.lines); got != tc.want {
				t.Fatalf("contentShowsSpinner = %v, want %v", got, tc.want)
			}
		})
	}
	for _, tool := range []string{"omp", "claude", "opencode", "codex"} {
		if contentShowsSpinner(tool, working) {
			t.Errorf("OmO's working border must not mark %s as working", tool)
		}
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

	// Real Claude Code 2.1.228 fixture captured live mid-task. The star glyph
	// itself is still not a spinner (the same family leads an idle pane
	// title — see isSpinnerGlyph); what marks the pane working is the row's
	// live counter, which only renders while a turn generates.
	claudeWorking := []string{
		"⏺ Update(admin/html/subpage/todayStatus/tools/tools.js)",
		"⎿  Added 2 lines, removed 7 lines",
		"Tools.js 잔여 ban_seq 파라미터 확인",
		"✶ Wibbling… (2m 32s · ↓ 10.2k tokens)",
		"⎿  Tip: You can control how big a workflow is just by prompting.",
		"──────────────────────────────────────",
		"❯",
		"──────────────────────────────────────",
		"branch:feat/yhank/today_status (wt:english_kangan) | !5 ?2",
		"[OMC#4.15.7] | Model: Opus 5 | 5h:[--------]5%(4h45m) wk:[#-------]13%",
		"40%(400k) | Opus 5 (1M context) | feat/yhank/today_status | v2.1.228",
		"⏵⏵ auto mode on (shift+tab to cycle) · ← 1 agent",
	}
	if !contentShowsSpinner("claude", claudeWorking) {
		t.Error("expected Claude Code's live status counter to be detected")
	}

	// Real Claude Code 2.1.237 fixtures captured live from three panes at once.
	// The working pane's title ("✳ 단축키 p 토글 정렬 기능 수정") was
	// indistinguishable from the two finished panes' titles, so the body row is
	// the only thing separating them. Note the status row sits nine lines above
	// the bottom, under a three-line input box and a four-row statusline.
	claudeWorking237 := []string{
		"", "", "", "", "",
		"· Doodling… (3m 29s · ↓ 9.7k tokens)",
		"",
		"──────────────────────────────────────",
		"❯",
		"──────────────────────────────────────",
		"  branch:main | +5 !9 ⇡2",
		"  [OMC#4.15.10L] | Model: Opus 5 | session:3m | ctx:[#---------]10% (100k)",
		"  10%(100k) | Opus 5 (1M context) | main | v2.1.237",
		"  ⏵⏵ auto mode on (shift+tab to cycle) · ← 1 agent",
	}
	if !contentShowsSpinner("claude", claudeWorking237) {
		t.Error("expected Claude Code 2.1.237's status row to be detected under a tall statusline")
	}

	// Same build, turn finished: the row loses its ellipsis and counter.
	claudeFinished237 := []string{
		"✻ Churned for 7m 52s",
		"",
		"※ recap: 일일 탭에 푸시 전송 팝업을 추가했습니다. (disable recaps in /config)",
		"──────────────────────────────────────",
		"❯",
		"──────────────────────────────────────",
		"  branch:feat/yhank/today_status (wt:english_kangan) | +5 !1 ?3",
		"  23%(230k) | Opus 5 (1M context) | feat/yhank/today_status | v2.1.235",
		"  ⏵⏵ auto mode on (shift+tab to cycle) · ← 1 agent",
	}
	if contentShowsSpinner("claude", claudeFinished237) {
		t.Error("a finished Claude Code turn must not be detected as working")
	}

	// The "claude agents" launcher screen: idle, and its blank bottom rows must
	// not shift the scan window off the real content either.
	claudeLauncher := []string{
		" A different way to work with Claude: hand off a bigger task than you would chat through.",
		"──────────────────────────────────────",
		"❯ describe a task for a new session",
		"──────────────────────────────────────",
		"  ? for shortcuts",
		"", "", "",
	}
	if contentShowsSpinner("claude", claudeLauncher) {
		t.Error("the Claude Code launcher screen must not be detected as working")
	}

	// Scrollback guard, Claude flavour: this repo's own source and any pane
	// quoting it carry the marker text, so it must not match from history.
	claudeBuried := append([]string{"✶ Wibbling… (2m 32s · ↓ 10.2k tokens)"},
		make([]string, 20)...)
	for i := range claudeBuried {
		if claudeBuried[i] == "" {
			claudeBuried[i] = "filler"
		}
	}
	if contentShowsSpinner("claude", claudeBuried) {
		t.Error("a Claude status row outside the trailing scan window must not match")
	}

	// The bare input box on its own is idle, not working: none of the box
	// drawing or mode-indicator glyphs may count as a spinner.
	claudeIdle := []string{
		"⏺ Done — 3 files changed.",
		"──────────────────────────────────────",
		"❯",
		"──────────────────────────────────────",
		"branch:feat/yhank/today_status (wt:english_kangan) | !5 ?2",
		"⏵⏵ auto mode on (shift+tab to cycle)",
	}
	if contentShowsSpinner("claude", claudeIdle) {
		t.Error("idle Claude Code content must not be detected as working")
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
	spinners := []rune{
		'⠂', '⠐', '⠸', '⠦', // Braille — older Claude Code titles, omp body lines
		'◐', '◑', '◒', '◓', // half circles — Claude Code 2.1.228 working titles
	}
	for _, r := range spinners {
		if !isSpinnerGlyph(r) {
			t.Errorf("%q (U+%04X) should be a spinner glyph", r, r)
		}
	}
	// The star family must stay out: Claude Code 2.1.228 leads an *idle* pane
	// title with ✳, so treating stars as spinners marked every waiting agent as
	// working. '·' and '∗' are ordinary punctuation for the same reason.
	for _, r := range []rune{'a', '가', 'π', ' ', '✳', '✶', '✻', '✽', '✢', '·', '∗', '●', '○'} {
		if isSpinnerGlyph(r) {
			t.Errorf("%q (U+%04X) should not be a spinner glyph", r, r)
		}
	}
}

func TestJobToAgentCarriesLiveness(t *testing.T) {
	js := jobState{SessionID: "session-123", DaemonShort: "daemon-1"}
	a := jobToAgent(js, "fallback-dir", map[string]bool{"daemon-1": true})
	if a.ID != "session-123" || !a.Live {
		t.Fatalf("live job fields lost: %#v", a)
	}

	js.SessionID = ""
	a = jobToAgent(js, "fallback-dir", map[string]bool{"daemon-1": false})
	if a.ID != "fallback-dir" {
		t.Errorf("fallback identity = %q, want directory ID", a.ID)
	}
	if a.Live {
		t.Error("dead daemon reported live")
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

func TestGroupOrdersProjectAgentsByStatusChangeTime(t *testing.T) {
	now := time.Now()
	agents := []Agent{
		{ID: "older-error", Kind: KindInteractive, Life: LifeError, Project: "/p", StatusChangedAt: now.Add(-time.Minute)},
		{ID: "newer-idle", Kind: KindBackground, Life: LifeIdle, Project: "/p", StatusChangedAt: now},
	}

	got := group(agents)[0].Agents
	if got[0].ID != "newer-idle" || got[1].ID != "older-error" {
		t.Errorf("project agents = %#v, want newest status transition first", got)
	}
}

func TestAttentionRankPriority(t *testing.T) {
	order := []Lifecycle{
		LifeInput,
		LifeError,
		LifeStale,
		LifeUnknown,
		LifeWorking,
		LifeIdle,
		LifeStopped,
		LifeDone,
	}
	for i := 1; i < len(order); i++ {
		if AttentionRank(order[i-1]) >= AttentionRank(order[i]) {
			t.Fatalf("%s must rank ahead of %s", order[i-1], order[i])
		}
	}
}

func TestGroupPrioritizesProjectsNeedingIntervention(t *testing.T) {
	now := time.Now()
	agents := []Agent{
		{ID: "working-1", Kind: KindInteractive, Life: LifeWorking, Project: "/p/busy", UpdatedAt: now},
		{ID: "working-2", Kind: KindBackground, Life: LifeWorking, Project: "/p/busy", UpdatedAt: now},
		{ID: "error-bg", Kind: KindBackground, Life: LifeError, Project: "/p/error", UpdatedAt: now.Add(-time.Hour)},
		{ID: "error-interactive", Kind: KindInteractive, Life: LifeError, Project: "/p/error", UpdatedAt: now.Add(-2 * time.Hour)},
		{ID: "input", Kind: KindBackground, Life: LifeInput, Project: "/p/input", UpdatedAt: now.Add(-3 * time.Hour)},
	}

	projs := group(agents)
	gotProjects := []string{projs[0].Key, projs[1].Key, projs[2].Key}
	wantProjects := []string{"/p/input", "/p/error", "/p/busy"}
	for i := range wantProjects {
		if gotProjects[i] != wantProjects[i] {
			t.Fatalf("project order = %v, want %v", gotProjects, wantProjects)
		}
	}
	if got := projs[1].Agents[0].ID; got != "error-interactive" {
		t.Errorf("equal-rank interactive agent should sort first, got %q", got)
	}
}

func TestGroupAttentionTiesUseCountThenRecency(t *testing.T) {
	now := time.Now()
	agents := []Agent{
		{ID: "more-1", Life: LifeError, Project: "/p/more", UpdatedAt: now.Add(-3 * time.Hour)},
		{ID: "more-2", Life: LifeError, Project: "/p/more", UpdatedAt: now.Add(-4 * time.Hour)},
		{ID: "recent", Life: LifeError, Project: "/p/recent", UpdatedAt: now},
		{ID: "older", Life: LifeError, Project: "/p/older", UpdatedAt: now.Add(-time.Hour)},
	}

	projs := group(agents)
	got := []string{projs[0].Key, projs[1].Key, projs[2].Key}
	want := []string{"/p/more", "/p/recent", "/p/older"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("project tie order = %v, want %v", got, want)
		}
	}
}

func TestGroupRecencyUsesBestAttentionRankOnly(t *testing.T) {
	now := time.Now()
	agents := []Agent{
		{ID: "error-a", Life: LifeError, Project: "/p/a", UpdatedAt: now.Add(-time.Hour)},
		{ID: "working-a", Life: LifeWorking, Project: "/p/a", UpdatedAt: now.Add(time.Hour)},
		{ID: "error-b", Life: LifeError, Project: "/p/b", UpdatedAt: now},
	}

	projs := group(agents)
	if got := projs[0].Key; got != "/p/b" {
		t.Errorf("lower-priority activity changed project recency order: got %q", got)
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
