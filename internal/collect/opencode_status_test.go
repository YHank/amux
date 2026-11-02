package collect

import (
	"strings"
	"testing"
)

func TestOpenCodeRunningSubagentFooter(t *testing.T) {
	// The live 2.2.5 footer counts only family sessions whose status is running.
	footer := "/work/project  ↓ 1 subagent · 93.0K (15%)  ctrl+p commands"
	for _, tc := range []struct {
		name  string
		tool  string
		lines []string
		want  bool
	}{
		{"running child", "opencode", []string{"Build · GPT-6.1", footer}, true},
		{"running children", "opencode", []string{strings.ReplaceAll(footer, "1 subagent", "2 subagents")}, true},
		{"colored footer", "opencode", []string{"\x1b[36m" + footer + "\x1b[0m", "", ""}, true},
		{"idle", "opencode", []string{"/work/project  93.0K (15%)  ctrl+p commands"}, false},
		{"zero children", "opencode", []string{strings.ReplaceAll(footer, "1 subagent", "0 subagents")}, false},
		{"completed child", "opencode", []string{"General finished · Review", "ctrl+p commands"}, false},
		{"other tool", "claude", []string{footer}, false},
		{"scrollback", "opencode", append([]string{footer}, strings.Split(strings.Repeat("idle\n", contentSpinnerScanLines), "\n")...), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := contentShowsSpinner(tc.tool, tc.lines); got != tc.want {
				t.Fatalf("contentShowsSpinner() = %v, want %v", got, tc.want)
			}
		})
	}
}
