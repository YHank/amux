package collect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// jobState mirrors the fields amux reads from ~/.claude/jobs/<id>/state.json.
// The file has more keys; we decode only what the dashboard needs.
type jobState struct {
	State        string          `json:"state"`
	Detail       string          `json:"detail"`
	Tempo        string          `json:"tempo"`
	Tokens       int64           `json:"tokens"`
	Intent       string          `json:"intent"`
	Name         string          `json:"name"`
	Output       json.RawMessage `json:"output"`
	CWD          string          `json:"cwd"`
	OriginCWD    string          `json:"originCwd"`
	SessionID    string          `json:"sessionId"`
	DaemonShort  string          `json:"daemonShort"`
	RespawnFlags []string        `json:"respawnFlags"`
	Needs        json.RawMessage `json:"needs"`
	Suggested    string          `json:"suggestedReply"`
	CreatedAt    time.Time       `json:"createdAt"`
	UpdatedAt    time.Time       `json:"updatedAt"`
}

// jobsDir is the on-disk home for background agent state.
func jobsDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return filepath.Join(h, ".claude", "jobs")
	}
	return ""
}

// backgroundAgents reads every ~/.claude/jobs/<id>/state.json into an Agent.
// live is the set of daemonShort ids with a live rendezvous socket, used to
// tell a still-running "working" agent from one whose daemon has died.
func backgroundAgents(live map[string]bool) ([]Agent, error) {
	dir := jobsDir()
	if dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var agents []Agent
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		f := filepath.Join(dir, e.Name(), "state.json")
		data, err := os.ReadFile(f)
		if err != nil {
			continue // missing/racing state file — skip, not fatal
		}
		var js jobState
		if err := json.Unmarshal(data, &js); err != nil {
			continue
		}
		agents = append(agents, jobToAgent(js, e.Name(), live))
	}
	return agents, nil
}

func jobToAgent(js jobState, dirName string, live map[string]bool) Agent {
	cwd := js.CWD
	if cwd == "" {
		cwd = js.OriginCWD
	}
	id := js.SessionID
	if id == "" {
		id = dirName
	}
	life := normalizeLife(js, live[js.DaemonShort])
	return Agent{
		ID:         id,
		Kind:       KindBackground,
		Tool:       "claude",
		Live:       live[js.DaemonShort],
		Name:       jobName(js, dirName),
		Model:      modelFromFlags(js.RespawnFlags),
		Life:       life,
		State:      js.State,
		Tempo:      js.Tempo,
		Tokens:     js.Tokens,
		Detail:     firstLine(js.Detail),
		Intent:     strings.TrimSpace(js.Intent),
		CWD:        cwd,
		Project:    projectKey(cwd),
		CreatedAt:  js.CreatedAt,
		UpdatedAt:  js.UpdatedAt,
		NeedsInput: life == LifeInput,
		Suggested:  strings.TrimSpace(js.Suggested),
		Short:      js.DaemonShort,
	}
}

// normalizeLife maps the raw state string onto a Lifecycle. live reports whether
// the session's daemon still has a rendezvous socket. Liveness only arbitrates
// the working-vs-stale question: a "working" job whose daemon is gone is stale,
// while a "working" job with a live socket stays working no matter how long ago
// state.json was last written.
func normalizeLife(js jobState, live bool) Lifecycle {
	switch strings.ToLower(strings.TrimSpace(js.State)) {
	case "done", "complete", "completed", "success":
		return LifeDone
	case "stopped", "cancelled", "canceled":
		return LifeStopped
	case "error", "failed", "failure":
		return LifeError
	case "input", "needs input", "needs_input", "waiting", "blocked":
		return LifeInput
	case "idle":
		return LifeIdle
	case "working", "running", "active":
		if !live {
			return LifeStale // daemon died mid-work
		}
		return LifeWorking
	default:
		// An unrecognized non-terminal state with a suggested reply is an
		// input prompt; otherwise unknown.
		if strings.TrimSpace(js.Suggested) != "" || (len(js.Needs) > 0 && string(js.Needs) != "null") {
			return LifeInput
		}
		return LifeUnknown
	}
}

// modelFromFlags pulls the value following `--model` in respawnFlags.
func modelFromFlags(flags []string) string {
	for i := 0; i < len(flags)-1; i++ {
		if flags[i] == "--model" {
			return flags[i+1]
		}
	}
	return ""
}

// jobName resolves a human label: explicit name > --name flag > short id.
func jobName(js jobState, dirName string) string {
	if n := strings.TrimSpace(js.Name); n != "" {
		return cleanName(n)
	}
	for i := 0; i < len(js.RespawnFlags)-1; i++ {
		if js.RespawnFlags[i] == "--name" {
			return cleanName(js.RespawnFlags[i+1])
		}
	}
	if js.DaemonShort != "" {
		return js.DaemonShort
	}
	return dirName
}

// cleanName strips the trailing "\t<n>" bookkeeping suffix some names carry.
func cleanName(n string) string {
	if i := strings.IndexByte(n, '\t'); i >= 0 {
		return strings.TrimSpace(n[:i])
	}
	return strings.TrimSpace(n)
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}
