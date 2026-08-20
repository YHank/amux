<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-07-23 | Updated: 2026-08-20 -->

# collect

## Purpose

Collects a normalized snapshot of AI coding agents from two disjoint discovery sources: interactive agents running in tmux panes (via `tmux list-panes` and process-tree inspection) and background agents managed by Claude Code and Oh My Pi daemons (via on-disk state in `~/.claude/jobs/` and `~/.omp/run/daemons/`). Correlates and groups agents by project (git root or working directory).

## Key Files

| File | Description |
|------|-------------|
| `model.go` | Core data structures: `Kind` (interactive/background), `Lifecycle` (working/input/idle/done/stopped/error/stale/unknown), `Agent` (unified agent record), `Project` (grouped agents by working directory), `Snapshot` (point-in-time state). `Agent.Recency()` is the single definition of "when did this agent last change state" and the primary sort key everywhere: the dashboard's observed transition first, the state file's write timestamp only until there is one. `Agent.Age()` prefers the opposite order, because the file knows when the state really changed and the observation only knows when amux started watching. |
| `tmux.go` | Interactive agent discovery via tmux pane inspection. Parses `tmux list-panes` output (8 tab-separated fields), captures from 40 scrollback lines above the visible screen through its bottom, detects choice prompts (footer heuristic bounded to 8 lines) and working state from pane content: Braille/half-circle glyph matching for omp, the `esc interrupt` footer for OpenCode, and the live status row (`… (3m 29s · ↓ 9.7k tokens)`, bounded to 12 lines) for Claude Code. `trailingLines` drops blank bottom rows before every one of those bounded scans. |
| `proc.go` | Process-tree snapshot and inspection. Runs single `ps -Awwo pid,ppid,%cpu,etime,comm,args` command, builds in-memory parent→child index for fast tree walks, identifies AI tool processes (claude, codex, aider, gemini, omp, opencode), measures subtree CPU. |
| `snapshot.go` | Orchestrates collection: calls `snapshotProcs()`, `interactiveAgents()`, `backgroundAgents()`, and `ompBackgroundAgents()`, filters dead-daemon records via liveness checks, groups by `projectKey()` (git root or working directory), sorts by attention-rank then recency. Main entry point: `Collect(now)`. |
| `jobs.go` | Parses Claude Code background job state from `~/.claude/jobs/<id>/state.json`. Decodes `jobState` struct (state, detail, model, tokens, intent, tempo, suggested reply). Correlates with daemon liveness via rendezvous socket check in `/tmp/cc-daemon-*/*/rv/*.sock`. |
| `omp.go` | Parses Oh My Pi daemon registry (`~/.omp/run/daemons/<hash>/daemons/<name>/meta.json`). Decodes omp daemon state (starting/running/stopped/exited/failed). Resolves project directory via client registry, checks daemon process liveness via PID signaling. |
| `liveness.go` | Verifies daemon liveness: `liveShorts()` globs Claude Code rendezvous sockets (`/tmp/cc-daemon-*/*/rv/*.sock`), `processAlive()` probes PID with signal 0 for process existence. Both are authoritative runtime signals, not file timestamps. |
| `collect_test.go` | 20+ deterministic unit tests: parser/liveness via fixture data (no live tmux dependency). Covers `detectTool()`, `parsePaneLine()`, `parseProcLine()`, `parseElapsed()`, `interactiveLife()` (spinner glyphs), `looksLikeChoicePrompt()` (with ANSI stripping), `contentShowsSpinner()`, `normalizeLife()`, `jobToAgent()`, and grouping/attention-ranking functions. |
| `omp_test.go` | 5 omp-specific tests: `ompNormalizeLife()` (state→lifecycle with daemon liveness), `ompTaskLive()` (task state vs. owner daemon liveness), `msToTime()` (epoch-ms parsing), `ompTaskToAgent()` (daemon task→Agent), liveness propagation. |
## Subdirectories

| Directory | Purpose |
|-----------|---------|
| _None_ | This package has no subdirectories. |

## For AI Agents
### Working In This Directory
- Treat all discovery sources (tmux, ps, sockets, home-directory state files) as unreliable external inputs. Missing servers, unparseable files, and transient pane closures must not crash collection.
- Collection runs on a recurring refresh cycle and must remain fast: keep process and tmux snapshots bounded to one `ps` and one `tmux list-panes` call, reuse the in-memory process tree, and keep liveness checks proportional to discovered daemon records.
- Normalize all raw agent state (Claude's `state` field, omp's `state` enum, tmux pane title spinners, pane content heuristics) into the unified `Lifecycle` enum before propagating to UI or commands.
- Liveness is an authoritative runtime signal from sockets or process existence, not inferred from timestamps or file modification times. A "working" job whose daemon socket is gone is stale; a "working" job with a live socket stays working no matter how long ago `state.json` was written.
- Pane titles are not a lifecycle signal for current tool versions. Claude Code 2.1.237 leaves `✳ <task>` in the title after a turn ends, and omp's title is static, so both tools are judged from their pane body instead (`contentShowsSpinner`). A spinner-glyph title still wins where present, keeping older Claude Code builds working. Any new working-state marker must be verified against panes captured live in **both** states before it is trusted — the star family was added and removed once already on a one-sided reading.
- Every content heuristic is bounded to the trailing lines of the capture, because scrollback (including this repo's own source quoted into a pane) contains the marker text verbatim.

### Testing Requirements
- Run `go test ./internal/collect` while iterating and `go test ./...` before completion.
- Write fixture-based tests with explicit expected cases, not live-tmux-dependent tests. All parser and lifecycle tests must work offline with canned data.
- Test each discovery source independently: interactive agents via canned pane data, Claude jobs via fixture `state.json` structures, omp daemons via fixture directory layouts and JSON files.
- Cover boundary cases: missing files, empty directories, malformed JSON, invalid process fields, absent daemon directories, dead sockets, closed panes, zero PIDs.

### Common Patterns
- Collection separates source-specific parsing (`interactiveAgents`, `backgroundAgents`, `ompBackgroundAgents`) from the normalized `Snapshot` model.
- All state normalization flows through `Lifecycle` and `Kind` to ensure consistent semantics across tmux spinners, Claude job state strings, and omp daemon enums.
- Daemon liveness is determined once per refresh (socket glob + PID signaling) and checked before including an agent in the snapshot.

## Dependencies

### Internal
- Defines `Agent`, `Project`, and `Snapshot` types consumed by `internal/ui` (rendering) and `cmd/amux` (main loop refresh).
- Has no internal package dependencies; collection is a pure function of external system state.

### External
- **Process Inspection**: Go standard library `os/exec` (run `ps`), `os` (PID signaling for liveness), `syscall` (signal handling).
- **Filesystem**: Go standard library `os`, `filepath` (home directory, stat, ReadDir, Glob for sockets).
- **Tmux**: External `tmux` binary (`list-panes`, `capture-pane` commands), parsed as tab-separated text and ANSI-stripped output.
- **Claude Code**: `~/.claude/jobs/<id>/state.json` (JSON-decoded), `/tmp/cc-daemon-*/*/rv/*.sock` (socket existence via filepath.Glob).
- **Oh My Pi**: `~/.omp/run/daemons/<hash>/daemons/<name>/meta.json` (JSON-decoded), PID lookups via signal 0 probe.
- **Data Formats**: Go `encoding/json` (Claude and OMP state files), `time` (start time approximation via ps etime parsing).
<!-- MANUAL: Any manually added notes below this line are preserved on regeneration -->
