package collect

import "testing"

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
	for _, tt := range []struct {
		name  string
		tool  string
		lines []string
		want  bool
	}{
		{"pending choice with status bar and blank rows", "omo", pending, true},
		{"old choice in scrollback", "omo", history, false},
		{"other tool", "omp", pending, false},
		{"idle footer", "omo", []string{"OmO Native omo 5.1.68 waiting_human"}, false},
		{"partial instructions", "omo", []string{"space select  esc cancel"}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := looksLikeChoicePrompt(tt.tool, tt.lines); got != tt.want {
				t.Errorf("looksLikeChoicePrompt(%q) = %v, want %v", tt.tool, got, tt.want)
			}
		})
	}
}
