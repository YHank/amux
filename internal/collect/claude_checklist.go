package collect

import (
	"bytes"
	"cmp"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

var claudeChecklists = &omoChecklistCache{
	states:    map[string]*omoChecklistState{},
	applyLine: applyClaudeTodos,
}

var claudeTaskListID = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func claudeChecklist(root string, session claudeSession) ([]ChecklistItem, string) {
	// TaskCreate/TaskUpdate persist separate files; TodoWrite is in-memory
	// in newer Claude releases, with successful updates in the transcript.
	items, found := claudeTaskChecklist(root, session.SessionID)
	if found && len(items) > 0 {
		return items, ""
	}
	project := claudeProjectEscape.ReplaceAllString(session.CWD, "-")
	todos, intent := claudeChecklists.readWithIntent(filepath.Join(root, "projects", project, session.SessionID+".jsonl"), time.Now())
	if found {
		return items, intent
	}
	return todos, intent
}

func claudeTaskChecklist(root, sessionID string) ([]ChecklistItem, bool) {
	listID := sessionID
	// Newer Claude versions name session-owned lists session-<UUID prefix>.
	shortID := sessionID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	shortDir := filepath.Join(root, "tasks", "session-"+shortID)
	if info, err := os.Stat(shortDir); err == nil && info.IsDir() {
		listID = "session-" + shortID
		// /clear can preserve a previous list through this explicit pointer.
		if file, err := os.Open(filepath.Join(shortDir, ".session-list")); err == nil {
			data, readErr := io.ReadAll(io.LimitReader(file, 257))
			file.Close()
			id := strings.TrimSpace(string(data))
			if readErr != nil || len(data) > 256 || !claudeTaskListID.MatchString(id) {
				return nil, true
			}
			listID = id
		}
	}
	dir := filepath.Join(root, "tasks", listID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, false
	}
	// Numeric IDs must keep 2 before 10.
	slices.SortFunc(entries, func(a, b os.DirEntry) int {
		x, _ := strconv.Atoi(strings.TrimSuffix(a.Name(), ".json"))
		y, _ := strconv.Atoi(strings.TrimSuffix(b.Name(), ".json"))
		return cmp.Compare(x, y)
	})
	var items []ChecklistItem
	found := false
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		found = true
		var task struct {
			Subject string          `json:"subject"`
			Status  ChecklistStatus `json:"status"`
		}
		if !readClaudeRecord(filepath.Join(dir, entry.Name()), &task) || task.Subject == "" {
			continue
		}
		switch task.Status {
		case ChecklistPending, ChecklistActive, ChecklistDone:
			items = append(items, ChecklistItem{Content: task.Subject, Status: task.Status})
		}
	}
	return items, found
}

func applyClaudeTodos(st *omoChecklistState, line []byte) {
	if !bytes.Contains(line, []byte(`"user"`)) {
		return
	}
	var record struct {
		Type             string    `json:"type"`
		IsSidechain      bool      `json:"isSidechain"`
		IsMeta           bool      `json:"isMeta"`
		IsCompactSummary bool      `json:"isCompactSummary"`
		Timestamp        time.Time `json:"timestamp"`
		Message          struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
		Result struct {
			Todos *[]struct {
				Content string          `json:"content"`
				Status  ChecklistStatus `json:"status"`
			} `json:"newTodos"`
		} `json:"toolUseResult"`
	}
	if json.Unmarshal(line, &record) != nil || record.Type != "user" || record.IsSidechain || record.IsMeta || record.IsCompactSummary {
		return
	}
	if text := requestText(record.Message.Content); text != "" {
		st.intent = text
	}
	if record.Result.Todos == nil {
		return
	}
	previous := make(map[string]ChecklistItem, len(st.items))
	for _, item := range st.items {
		previous[item.Content] = item
	}
	var items []ChecklistItem
	for _, todo := range *record.Result.Todos {
		if todo.Content == "" {
			continue
		}
		item := ChecklistItem{Content: todo.Content, Status: todo.Status}
		prior := previous[item.Content]
		switch item.Status {
		case ChecklistPending:
		case ChecklistActive:
			item.StartedAt = prior.StartedAt
			if item.StartedAt.IsZero() {
				item.StartedAt = record.Timestamp
			}
		case ChecklistDone:
			item.StartedAt, item.FinishedAt = prior.StartedAt, prior.FinishedAt
			if item.FinishedAt.IsZero() {
				item.FinishedAt = record.Timestamp
			}
		default:
			continue
		}
		items = append(items, item)
	}
	st.items = items
}
