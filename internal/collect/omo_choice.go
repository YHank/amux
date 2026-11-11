package collect

import (
	"regexp"
	"strings"
)

var omoPendingQuestion = regexp.MustCompile(`^\? Question pending \([1-9][0-9]* unanswered\)`)

// OmO 5.1.69 keeps unanswered cards above the editor even while children run.
// Require a positive pending count and reply controls immediately above the
// current editor, so a quoted question or an old card is not an input signal.
func omoHasPendingQuestion(lines []string) bool {
	tail := trailingLines(lines, 20)
	recent := make([]string, len(tail))
	for i, line := range tail {
		recent[i] = strings.TrimSpace(stripANSI(line))
	}
	input := -1
	for i := len(recent) - 1; i >= max(0, len(recent)-choicePromptScanLines); i-- {
		if recent[i] == "❯" || strings.HasPrefix(recent[i], "❯ ") {
			input = i
			break
		}
	}
	for i := input - 1; i >= 0; i-- {
		line := recent[i]
		if strings.Trim(line, "─━╶╴ ") == "" || omoWorkingStatus.MatchString(line) {
			continue
		}
		if !strings.HasPrefix(line, "enter or option+a to answer") ||
			!strings.Contains(line, "/answer") ||
			!strings.HasSuffix(line, "or just type your reply") {
			return false
		}
		for _, above := range recent[:i] {
			if omoPendingQuestion.MatchString(above) {
				return true
			}
		}
		return false
	}
	return false
}
