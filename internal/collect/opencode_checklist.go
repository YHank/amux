package collect

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var opencodeSessionID = regexp.MustCompile(`^ses_[A-Za-z0-9]+$`)

type opencodeTodo struct {
	Content string `json:"content"`
	Status  string `json:"status"`
}

// opencodeChecklist reads an exact session from OpenCode's data directory.
// The caller must supply the current session ID, for example from Herdr's
// agent_session_id report. Neither cwd nor database recency identifies a TUI.
// Reads are capped at 256 items and 64 KiB; unavailable SQLite storage falls
// back to the pre-SQLite storage/todo/<sessionID>.json format.
func opencodeChecklist(ctx context.Context, root, sessionID string) []ChecklistItem {
	if ctx.Err() != nil || !filepath.IsAbs(root) || !opencodeSessionID.MatchString(sessionID) {
		return nil
	}
	var todos []opencodeTodo
	db := filepath.Join(root, "opencode.db")
	available := false
	if info, err := os.Stat(db); err == nil && info.Mode().IsRegular() {
		queryCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer cancel()
		// session_id is the leading primary-key column; position orders that
		// same indexed range. Validation above excludes SQL metacharacters.
		query := "SELECT content,status FROM todo WHERE session_id='" + sessionID +
			"' ORDER BY position LIMIT 256;"
		cmd := exec.CommandContext(queryCtx, "sqlite3", "-readonly", "-json", db, query)
		var out opencodeTodoOutput
		cmd.Stdout = &out
		cmd.Stderr = io.Discard
		err := cmd.Run()
		if out.overflow {
			return nil
		}
		if err == nil {
			// sqlite3 emits no bytes for an empty result. That is authoritative
			// and must not resurrect an old JSON checklist.
			available = true
			if len(out.data) != 0 && json.Unmarshal(out.data, &todos) != nil {
				return nil
			}
		}
	}
	if ctx.Err() != nil {
		return nil
	}
	if !available {
		path := filepath.Join(root, "storage", "todo", sessionID+".json")
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || !readClaudeRecord(path, &todos) {
			return nil
		}
	}
	var items []ChecklistItem
	for _, todo := range todos[:min(len(todos), 256)] {
		var status ChecklistStatus
		switch todo.Status {
		case "pending":
			status = ChecklistPending
		case "in_progress":
			status = ChecklistActive
		case "completed":
			status = ChecklistDone
		case "cancelled":
			status = ChecklistDropped
		default:
			continue
		}
		if strings.TrimSpace(todo.Content) != "" {
			items = append(items, ChecklistItem{Content: todo.Content, Status: status})
		}
	}
	return items
}

type opencodeTodoOutput struct {
	data     []byte
	overflow bool
}

func (b *opencodeTodoOutput) Write(p []byte) (int, error) {
	if len(p) > 64*1024-len(b.data) {
		b.overflow = true
		return 0, io.ErrShortBuffer
	}
	b.data = append(b.data, p...)
	return len(p), nil
}
