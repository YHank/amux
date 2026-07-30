package collect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Oh My Pi (omp) is a separate full coding-agent CLI (bun-run, package
// @oh-my-pi/pi-coding-agent) — unrelated to the oh-my-posh shell prompt theme
// despite the name collision. Like Claude Code it has its own on-disk daemon
// registry for named background tasks:
//
//	~/.omp/run/daemons/<hash>/clients/<pid>-<uuid>.json   — live client {pid, projectDir}
//	~/.omp/run/daemons/<hash>/daemons/<name>/meta.json    — one background task's state
//
// meta.json's `state` is one of starting|running|stopped|exited|failed (this
// enum is taken directly from the omp CLI bundle, not guessed).

type ompDaemonMeta struct {
	Daemon struct {
		Name       string `json:"name"`
		ID         string `json:"id"`
		State      string `json:"state"`
		CreatedAt  int64  `json:"createdAt"` // epoch millis
		StartedAt  int64  `json:"startedAt"`
		ExitedAt   int64  `json:"exitedAt"`
		ExitCode   *int   `json:"exitCode"`
		Owner      string `json:"owner"`
		OutputByte int64  `json:"outputBytes"`
	} `json:"daemon"`
}

type ompClient struct {
	PID        int    `json:"pid"`
	ID         string `json:"id"`
	ProjectDir string `json:"projectDir"`
}

func ompRunDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return filepath.Join(h, ".omp", "run", "daemons")
	}
	return ""
}

// ompBackgroundAgents reads every named background task across every omp
// daemon instance into an Agent.
func ompBackgroundAgents() ([]Agent, error) {
	root := ompRunDir()
	if root == "" {
		return nil, nil
	}
	metas, err := filepath.Glob(filepath.Join(root, "*", "daemons", "*", "meta.json"))
	if err != nil {
		return nil, err
	}
	var agents []Agent
	for _, f := range metas {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var m ompDaemonMeta
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}
		hashDir := filepath.Dir(filepath.Dir(filepath.Dir(f))) // .../daemons/<hash>
		agents = append(agents, ompTaskToAgent(m, hashDir))
	}
	return agents, nil
}

func ompTaskToAgent(m ompDaemonMeta, hashDir string) Agent {
	d := m.Daemon
	project, live := ompHashDirContext(hashDir)

	updated := msToTime(d.ExitedAt)
	if updated.IsZero() {
		updated = msToTime(d.StartedAt)
	}
	if updated.IsZero() {
		updated = msToTime(d.CreatedAt)
	}

	detail := "state: " + d.State
	if d.ExitCode != nil {
		detail += " (exit " + strconv.Itoa(*d.ExitCode) + ")"
	}

	return Agent{
		ID:        "omp-task:" + d.ID,
		Kind:      KindBackground,
		Tool:      "omp",
		Name:      d.Name,
		Life:      ompNormalizeLife(d.State, live),
		State:     d.State,
		Detail:    detail,
		CWD:       project,
		Project:   projectKey(project),
		CreatedAt: msToTime(d.CreatedAt),
		UpdatedAt: updated,
	}
}

// ompHashDirContext resolves a daemon-hash directory to the project it belongs
// to (via any client registration under it, live or stale) and whether that
// daemon instance currently has a live client process — the omp equivalent of
// Claude's rendezvous-socket liveness check.
func ompHashDirContext(hashDir string) (project string, live bool) {
	matches, _ := filepath.Glob(filepath.Join(hashDir, "clients", "*.json"))
	for _, f := range matches {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var c ompClient
		if json.Unmarshal(data, &c) != nil {
			continue
		}
		if project == "" {
			project = c.ProjectDir
		}
		if c.PID > 0 && processAlive(c.PID) {
			live = true
		}
	}
	return project, live
}

// ompNormalizeLife maps omp's daemon state enum onto a Lifecycle. A
// running/starting task whose owning client process has died is stale, same
// reasoning as Claude's dead-daemon case.
func ompNormalizeLife(state string, live bool) Lifecycle {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "exited", "stopped":
		return LifeDone
	case "failed":
		return LifeError
	case "running", "starting":
		if !live {
			return LifeStale
		}
		return LifeWorking
	default:
		return LifeUnknown
	}
}

func msToTime(ms int64) time.Time {
	if ms <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms)
}
