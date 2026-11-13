package collect

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var codexChecklistID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type codexChecklistEntry struct {
	path       string
	resolvedAt time.Time
	lastUsed   time.Time
	reader     *omoChecklistCache
}

var codexChecklists = struct {
	sync.Mutex
	entries map[string]*codexChecklistEntry
}{entries: map[string]*codexChecklistEntry{}}

// codexChecklist reads the exact session reported by the terminal integration.
// root is CODEX_HOME, normally ~/.codex. Rollout filenames and session_meta IDs
// must both match; cwd and launch arguments do not identify a selected session.
// Path lookup happens once per cached session, with a five-second negative cache.
func codexChecklist(root, sessionID string) ([]ChecklistItem, string) {
	if !filepath.IsAbs(root) || !codexChecklistID.MatchString(sessionID) {
		return nil, ""
	}
	now := time.Now()
	codexChecklists.Lock()
	defer codexChecklists.Unlock()
	for key, entry := range codexChecklists.entries {
		if now.Sub(entry.lastUsed) > omoChecklistCacheTTL {
			delete(codexChecklists.entries, key)
		}
	}
	key := filepath.Join(root, sessionID)
	entry := codexChecklists.entries[key]
	if entry == nil || (entry.path == "" && now.Sub(entry.resolvedAt) >= 5*time.Second) {
		entry = &codexChecklistEntry{
			resolvedAt: now,
			reader:     newCodexChecklistReader(sessionID),
		}
		paths, err := filepath.Glob(filepath.Join(root, "sessions", "*", "*", "*", "rollout-*-"+sessionID+".jsonl"))
		if err == nil && len(paths) == 1 {
			if info, err := os.Stat(paths[0]); err == nil && info.Mode().IsRegular() {
				entry.path = paths[0]
			}
		}
		codexChecklists.entries[key] = entry
	}
	entry.lastUsed = now
	if entry.path == "" {
		return nil, ""
	}
	return entry.reader.readWithIntent(entry.path, now)
}

type codexPlanRecord struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Payload   struct {
		ID        string          `json:"id"`
		Type      string          `json:"type"`
		Name      string          `json:"name"`
		CallID    string          `json:"call_id"`
		Arguments string          `json:"arguments"`
		Output    string          `json:"output"`
		Role      string          `json:"role"`
		Content   json.RawMessage `json:"content"`
	} `json:"payload"`
}

// Each reader owns its pending calls, including calls split across refreshes.
// A replaced/truncated transcript gets a new shared-reader state and resets them.
func newCodexChecklistReader(sessionID string) *omoChecklistCache {
	reader := &omoChecklistCache{states: map[string]*omoChecklistState{}}
	var state *omoChecklistState
	var verified bool
	pending := map[string][]ChecklistItem{}
	reader.applyLine = func(st *omoChecklistState, line []byte) {
		if st != state {
			state, verified = st, false
			clear(pending)
		}
		if !bytes.Contains(line, []byte(`"session_meta"`)) && !bytes.Contains(line, []byte(`"function_call`)) && !bytes.Contains(line, []byte(`"user"`)) {
			return
		}
		var rec codexPlanRecord
		if json.Unmarshal(line, &rec) != nil {
			return
		}
		if rec.Type == "session_meta" {
			verified = rec.Payload.ID == sessionID
			st.items = nil
			st.intent = ""
			clear(pending)
			return
		}
		if !verified || rec.Type != "response_item" {
			return
		}
		if rec.Payload.Type == "message" && rec.Payload.Role == "user" {
			if text := requestText(rec.Payload.Content); text != "" {
				st.intent = text
			}
			return
		}
		if rec.Payload.CallID == "" {
			return
		}
		switch rec.Payload.Type {
		case "function_call":
			if rec.Payload.Name != "update_plan" {
				return
			}
			var args struct {
				Plan *[]struct {
					Step   string          `json:"step"`
					Status ChecklistStatus `json:"status"`
				} `json:"plan"`
			}
			if json.Unmarshal([]byte(rec.Payload.Arguments), &args) != nil || args.Plan == nil {
				return
			}
			var items []ChecklistItem
			for _, step := range *args.Plan {
				switch step.Status {
				case ChecklistPending, ChecklistActive, ChecklistDone:
				default:
					return
				}
				if strings.TrimSpace(step.Step) == "" {
					return
				}
				items = append(items, ChecklistItem{Content: step.Step, Status: step.Status})
			}
			pending[rec.Payload.CallID] = items
		case "function_call_output":
			items, ok := pending[rec.Payload.CallID]
			delete(pending, rec.Payload.CallID)
			// The Codex plan handler returns this exact acknowledgement on
			// success; parse errors and rejected calls leave the plan intact.
			if !ok || rec.Payload.Output != "Plan updated" {
				return
			}
			previous := make(map[string]ChecklistItem, len(st.items))
			for _, item := range st.items {
				previous[item.Content] = item
			}
			for i := range items {
				item := &items[i]
				prior := previous[item.Content]
				item.StartedAt, item.FinishedAt = prior.StartedAt, prior.FinishedAt
				switch item.Status {
				case ChecklistActive:
					if item.StartedAt.IsZero() {
						item.StartedAt = rec.Timestamp
					}
					item.FinishedAt = time.Time{}
				case ChecklistDone:
					if item.FinishedAt.IsZero() {
						item.FinishedAt = rec.Timestamp
					}
				case ChecklistPending:
					item.StartedAt, item.FinishedAt = time.Time{}, time.Time{}
				}
			}
			st.items = items
		}
	}
	return reader
}
