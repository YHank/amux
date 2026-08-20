// Package collect gathers the live state of AI coding agents from two disjoint
// sources and correlates them into a single project-grouped snapshot:
//
//  1. Interactive agents  — a supported AI CLI process (`claude`, `omp`,
//     `opencode`, `codex`, `aider`, or `gemini`) running in the foreground of
//     a tmux pane. These are visible via `tmux list-panes`
//     plus a process-tree walk.
//  2. Background agents    — headless Claude Code daemons launched with the
//     `agents` command. These have NO tmux pane; their state lives on disk in
//     ~/.claude/jobs/<id>/state.json.
//
// mux (lunemis/tap/mux) only ever sees source #1. Surfacing #2 — and grouping
// both by the project they were spawned from — is the whole point of amux.
package collect

import "time"

// Kind distinguishes where an agent lives.
type Kind string

const (
	KindInteractive Kind = "interactive" // foreground in a tmux pane
	KindBackground  Kind = "background"  // headless ~/.claude/jobs daemon
)

// Lifecycle is a normalized, tool-agnostic status used to pick a badge/color.
type Lifecycle string

const (
	LifeWorking Lifecycle = "working" // actively doing work
	LifeInput   Lifecycle = "input"   // waiting for the user
	LifeIdle    Lifecycle = "idle"    // alive but not doing anything
	LifeDone    Lifecycle = "done"    // finished successfully
	LifeStopped Lifecycle = "stopped" // stopped / cancelled
	LifeError   Lifecycle = "error"   // failed
	LifeStale   Lifecycle = "stale"   // non-terminal but not updated recently
	LifeUnknown Lifecycle = "unknown"
)

// Terminal reports whether the lifecycle represents a finished agent that will
// never change on its own again.
func (l Lifecycle) Terminal() bool {
	switch l {
	case LifeDone, LifeStopped, LifeError:
		return true
	default:
		return false
	}
}

// Agent is one AI agent, unified across both sources.
type Agent struct {
	ID   string // stable id: sessionId (background) or tmux pane id (interactive)
	Kind Kind
	Tool string // claude | omp | opencode | codex | aider | gemini

	Name  string    // job name, or "session:win.pane" for interactive
	Model string    // e.g. sonnet, opus[1m] (background only)
	Life  Lifecycle // normalized status
	State string    // raw state string from the source
	Tempo string    // active | idle (background only)

	Tokens int64  // cumulative tokens (background only)
	Detail string // one-line status / last result
	Intent string // original prompt / task

	CWD     string // working directory
	Project string // normalized project key (git root or cwd)

	CreatedAt        time.Time
	UpdatedAt        time.Time
	ProcessStartedAt time.Time // interactive process start time observed from ps
	ActivityAt       time.Time // interactive pane title or content last changed in amux
	StatusChangedAt  time.Time // when amux last observed this agent's Life change
	// NeedsInput reports that the agent asked a question and is waiting for the
	// user to pick an option — set from job state.json (background) or a live
	// pane's own content for the choice-prompt UI (interactive).
	NeedsInput bool

	// Interactive-only fields.
	TmuxTarget string   // "session:win.pane" — attach target
	Session    string   // tmux session name
	Window     string   // tmux window name
	PaneTitle  string   // raw tmux pane title; empty when the terminal did not set one
	PID        int      // foreground agent pid
	CPU        float64  // %CPU of the agent process subtree
	Preview    []string // last few lines of the pane's actual terminal output

	// Background-only extras (Claude Code jobs).
	Live      bool   // owning background daemon has a live socket/process
	Suggested string // suggested reply, if any
	Short     string // daemonShort id (for liveness correlation)
}

// Age returns how long ago the agent last changed state.
func (a Agent) Age(now time.Time) time.Duration {
	if a.UpdatedAt.IsZero() {
		return 0
	}
	return now.Sub(a.UpdatedAt)
}

// Project is a group of agents that share a working directory / repo.
type Project struct {
	Key    string  // normalized project path
	Name   string  // display name (basename)
	Agents []Agent // interactive first, then background; each sorted by liveness
}

// Live reports how many agents in the project are actively working.
func (p Project) Working() int {
	n := 0
	for _, a := range p.Agents {
		if a.Life == LifeWorking {
			n++
		}
	}
	return n
}

// Snapshot is a single point-in-time view of everything amux tracks.
type Snapshot struct {
	Projects []Project
	Agents   []Agent // flat list, all agents
	Taken    time.Time
}

// Counts summarizes agents by lifecycle for the statusbar widget.
func (s Snapshot) Counts() map[Lifecycle]int {
	m := map[Lifecycle]int{}
	for _, a := range s.Agents {
		m[a.Life]++
	}
	return m
}
