package collect

import (
	"encoding/json"
	"strings"
)

// requestText keeps user text, excluding tool results and injected context.
func requestText(content json.RawMessage) string {
	var text string
	if json.Unmarshal(content, &text) != nil {
		var blocks []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if json.Unmarshal(content, &blocks) != nil {
			return ""
		}
		var parts []string
		for _, block := range blocks {
			if block.Type == "text" || block.Type == "input_text" {
				parts = append(parts, cleanRequestText(block.Text))
			}
		}
		text = strings.Join(parts, "\n")
	}
	return cleanRequestText(text)
}

func cleanRequestText(text string) string {
	// Claude peer notifications are user-role records with this wrapper.
	if strings.HasPrefix(strings.TrimSpace(text), "Another Claude session sent a message:") &&
		strings.Contains(text, "<teammate-message") {
		return ""
	}
	for {
		text = strings.TrimSpace(text)
		stripped := false
		for _, tag := range []string{"system-reminder", "environment_context", "recalled-memory", "omo-senpi-task", "task-notification", "teammate-message", "local-command-stdout"} {
			if strings.HasPrefix(text, "<"+tag+">") || strings.HasPrefix(text, "<"+tag+" ") {
				end := "</" + tag + ">"
				i := strings.Index(text, end)
				if i < 0 {
					return ""
				}
				text, stripped = text[i+len(end):], true
				break
			}
		}
		if !stripped {
			break
		}
	}
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if len(runes) > 1000 {
		return string(runes[:1000]) + "…"
	}
	return text
}
