package collect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var omoPendingQuestionScreen = []string{
	"Todo",
	"[•] review in progress",
	"? Question pending (1 unanswered) · 30m",
	"  Scope - apply to new links or existing links?",
	"[ New links (Recommended) ]  [ Existing links ]  [ own answer… ]",
	"click an option · shift+drag (option+drag on iTerm2) to select text",
	"enter or option+a to answer · /answer · or just type your reply",
	"────────────────────────────────────────",
	"❯ ",
	"────────────────────────────────────────",
	"Claude quota | Codex quota",
	" ⠧ background review · running",
	"/project • main",
	"OmO Native omo 5.1.69 waiting_human · children: 1",
}

func TestOmOChoicePrompt(t *testing.T) {
	// Captured from OmO 5.1.68 while request_user_input was awaiting a choice.
	footer := "\x1b[90m ↑↓ move  1-9 select  space select  enter next  tab next question  c comment  esc cancel\x1b[0m"
	pending := []string{
		"Ask user · 29m",
		"→ 1. Allow",
		"  2. Decline",
		"Type your own answer...",
		"Submit (0/1 answered) - Enter advances",
		footer,
		"────────────────────────────────────────",
		"Claude quota | Codex quota",
		"/project • model",
		"OmO Native omo 5.1.68 waiting_human",
		"", "",
	}
	history := append([]string{footer}, make([]string, choicePromptScanLines)...)
	for i := 1; i < len(history); i++ {
		history[i] = "Later ordinary output"
	}
	colored := make([]string, len(omoPendingQuestionScreen))
	for i, line := range omoPendingQuestionScreen {
		colored[i] = "\x1b[36m" + line + "\x1b[0m"
	}
	for _, tt := range []struct {
		name  string
		tool  string
		lines []string
		want  bool
	}{
		{"current pending card with running child", "omo", omoPendingQuestionScreen, true},
		{"colored current pending card", "omo", colored, true},
		{"current card for another tool", "omp", omoPendingQuestionScreen, false},
		{"answered card", "omo", strings.Split(strings.ReplaceAll(strings.Join(omoPendingQuestionScreen, "\n"), "(1 unanswered)", "(0 unanswered)"), "\n"), false},
		{"current card in scrollback", "omo", append(append([]string{}, omoPendingQuestionScreen...), strings.Split(strings.Repeat("ordinary output\n", 12)+"❯", "\n")...), false},
		{"instructions without question", "omo", omoPendingQuestionScreen[3:], false},
		{"pending text without answer controls", "omo", []string{"? Question pending (1 unanswered)", "─", "❯"}, false},
		{"pending choice with status bar and blank rows", "omo", pending, true},
		{"old choice in scrollback", "omo", history, false},
		{"other tool", "omp", pending, false},
		{"idle footer", "omo", []string{"OmO Native omo 5.1.68 waiting_human"}, false},
		{"partial instructions", "omo", []string{"space select  esc cancel"}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			original := strings.Join(tt.lines, "\n")
			if got := looksLikeChoicePrompt(tt.tool, tt.lines); got != tt.want {
				t.Errorf("looksLikeChoicePrompt(%q) = %v, want %v", tt.tool, got, tt.want)
			}
			if strings.Join(tt.lines, "\n") != original {
				t.Fatal("question detection changed the pane preview")
			}
		})
	}
}

func TestOmOQuestionTakesPriorityOverRunningChild(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("PATH", dir)
	script := "#!/bin/sh\nprintf '%s\\n' '" + strings.Join(omoPendingQuestionScreen, "\n") + "'\n"
	if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	procs := &procTable{byPID: map[int]proc{10: {pid: 10, comm: "omo"}}}
	agents := interactiveAgents(procs, []pane{{pid: 10, paneID: "%1", target: "work:0.0", path: dir}})
	if len(agents) != 1 || !agents[0].NeedsInput || agents[0].Life != LifeInput {
		t.Fatalf("pending question must override the child spinner: %+v", agents)
	}
}
