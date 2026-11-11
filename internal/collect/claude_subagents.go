package collect

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Claude 2.1.316 keeps small, mutable lifecycle records next to its potentially
// huge transcripts. Read only those records each refresh, never the JSONL logs.
type claudeSession struct {
	PID       int    `json:"pid"`
	SessionID string `json:"sessionId"`
	CWD       string `json:"cwd"`
	StartedAt int64  `json:"startedAt"`
	Tmux      string `json:"tmux"`
	Status    string `json:"status"`
}

type claudeChild struct {
	AgentType     string `json:"agentType"`
	Description   string `json:"description"`
	ParentAgentID string `json:"parentAgentId"`
	SpawnDepth    int    `json:"spawnDepth"`
	RequestShape  string `json:"requestShape"`
	Finished      bool   `json:"finished"`
}

var claudeProjectEscape = regexp.MustCompile(`[^a-zA-Z0-9-]`)
var claudeRecordID = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)

func claudeSubagents(root string, parent proc, pn pane) ([]Subagent, bool) {
	var session claudeSession
	if !readClaudeRecord(filepath.Join(root, "sessions", strconv.Itoa(parent.pid)+".json"), &session) ||
		session.PID != parent.pid || parent.pid <= 0 ||
		!claudeRecordID.MatchString(session.SessionID) || !filepath.IsAbs(session.CWD) || pn.paneID == "" {
		return nil, false
	}
	// Herdr ownership comes from process-info and the exact PID/start time
	// below; Claude's tmux metadata is absent outside tmux.
	if pn.backend != BackendHerdr && !strings.HasSuffix(session.Tmux, "."+pn.paneID) {
		return nil, false
	}
	// ps etime is rounded to seconds. Allow that precision, but reject a stale
	// session file left behind for a reused PID. File recency is never liveness.
	start := time.UnixMilli(session.StartedAt)
	delta := parent.startedAt.Sub(start)
	if session.StartedAt <= 0 || parent.startedAt.IsZero() || delta < -2*time.Second || delta > 2*time.Second {
		return nil, false
	}
	busy := session.Status == "busy"
	project := claudeProjectEscape.ReplaceAllString(session.CWD, "-")
	dir := filepath.Join(root, "projects", project, session.SessionID, "subagents")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, busy
	}
	children := make(map[string]claudeChild)
	var order []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, "agent-") || !strings.HasSuffix(name, ".meta.json") {
			continue
		}
		id := strings.TrimSuffix(strings.TrimPrefix(name, "agent-"), ".meta.json")
		var child claudeChild
		if !claudeRecordID.MatchString(id) || !readClaudeRecord(filepath.Join(dir, name), &child) ||
			child.AgentType == "" || child.SpawnDepth < 1 || child.RequestShape != "background" {
			continue
		}
		children[id] = child
		order = append(order, id)
	}
	var result []Subagent
	for _, id := range order {
		// A killed/finished ancestor can leave an unfinished descendant record.
		// Require an unbroken active chain back to the live interactive session.
		current := id
		seen := make(map[string]bool)
		active := true
		for current != "" {
			child, ok := children[current]
			if !ok || child.Finished || seen[current] ||
				(child.ParentAgentID == "" && child.SpawnDepth != 1) {
				active = false
				break
			}
			seen[current] = true
			current = child.ParentAgentID
		}
		if active {
			result = append(result, Subagent{ID: id, Description: children[id].Description, Life: LifeWorking})
		}
	}
	return result, busy
}

// Invalid, absent or partially rewritten metadata is simply unavailable for
// this scan. A size cap also prevents a wrong file from becoming a log rescan.
func readClaudeRecord(path string, value any) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 64*1024+1))
	if err != nil || len(data) > 64*1024 {
		return false
	}
	return json.Unmarshal(data, value) == nil
}
