# collect

**Parent context:** `../AGENTS.md`
**Generated:** 2026-07-23 · **Updated:** 2026-11-13

## Purpose

Collects a normalized snapshot of AI coding agents from two disjoint discovery sources: interactive agents running in terminal panes (tmux via `tmux list-panes`, or Herdr sessions via the `herdr` CLI, both combined with process-tree inspection) and background agents managed by Claude Code and Oh My Pi daemons (via on-disk state in `~/.claude/jobs/` and `~/.omp/run/daemons/`). Correlates and groups agents by project (git root or working directory).

## Key Files

| File | Description |
|------|-------------|
| `model.go` | Core data structures: `Backend` (tmux/herdr), `Kind` (interactive/background), `Lifecycle` (working/input/idle/done/stopped/error/stale/unknown), `Subagent` (child task of a live interactive session), `ChecklistItem`/`ChecklistStatus` (one entry of an agent's todo plan), `Agent` (unified agent record), `Project` (grouped agents by working directory), `Snapshot` (point-in-time state, including the `LivePaneIDs` pane inventory where nil means unreadable). `Agent.Recency()` is the single definition of "when did this agent last change state" and the primary sort key everywhere: the dashboard's observed transition first, the state file's write timestamp only until there is one. `Agent.Age()` prefers the opposite order, because the file knows when the state really changed and the observation only knows when amux started watching. |
| `tmux.go` | Interactive agent discovery via tmux pane inspection. Parses `tmux list-panes` output (8 `\x1f`-separated fields), captures from 40 scrollback lines above the visible screen through its bottom, detects choice prompts (footer heuristic bounded to 8 lines; Claude Code, OpenCode and OmO footers) and working state from pane content: Braille/half-circle glyph matching for omp, the `esc interrupt` footer and running-subagent footer for OpenCode, the active editor border for OmO, and the live status row (`… (3m 29s · ↓ 9.7k tokens)`, bounded to 12 lines) for Claude Code. `trailingLines` drops blank bottom rows before every one of those bounded scans. Also builds `previewFingerprint`, which drops spinner and token-counter rows so volatile rows do not count as activity. |
| `interactive.go` | Backend-neutral interactive enrichment. `interactiveAgents` captures tmux panes with up to 4 concurrent workers while preserving pane order; `collectInteractivePreview` (shared with Herdr) turns a pane, its agent process and its ANSI preview into an `Agent`: title/content lifecycle, choice-prompt override to `LifeInput`, Claude subagents (an idle parent is promoted to `LifeWorking` when its session is busy or has running children), and the OmO checklist. |
| `herdr.go` | Interactive agent discovery through Herdr. `herdrAgents` lists sessions with `herdr session list --json`, reads each running session with `api snapshot`, resolves each pane's agent process via `pane process-info` (the shell PID subtree first, then foreground processes), and reads the last 40 lines with `pane read --format ansi`. Agent IDs are namespaced `herdr:<session>:<pane_id>` and the tab label becomes `Window`. Any failed or malformed inventory returns the agents found so far with a nil pane-ID map and an error, so callers never treat it as pane deletion; a failed preview read keeps the agent with an empty preview. Uses `internal/herdr` for CLI access. |
| `claude_subagents.go` | Reads Claude Code's small lifecycle records (`~/.claude/sessions/<pid>.json` and `projects/<escaped-cwd>/<sessionId>/subagents/agent-*.meta.json`), never the transcripts, each capped at 64 KiB. A session counts only when its PID matches, its start time is within 2 seconds of the process start (rejects a reused PID), and, under tmux, its `tmux` field ends in the pane ID; Herdr ownership comes from process-info instead. Returns background children with an unbroken unfinished parent chain, plus whether the session status is `busy`. |
| `omo_checklist.go` | Reads an OmO session's todo plan from its append-only JSONL transcript. A live process is tied to a session through `sessions/<cwd>/session-holders/<sessionId>/<pid>.json` matched on PID and start time. A per-transcript cache (10 minute TTL) remembers the parsed byte offset so a refresh reads only newly appended complete lines, and rereads when the file shrinks. `ChecklistItem` start and finish times come from the transcript timestamps; an `init` operation resets them. |
| `omo_choice.go` | Detects an OmO pending question card (`? Question pending (N unanswered)` with the `/answer` reply controls directly above the editor prompt), so a question outranks a running child's spinner. Called from `looksLikeChoicePrompt`. |
| `proc.go` | Process-tree snapshot and inspection. Runs single `ps -Awwo pid,ppid,%cpu,etime,comm,args` command (scanner limit 1 MiB per line), builds in-memory parent→child index for fast tree walks, identifies AI tool processes (claude, codex, aider, gemini, omp, omo, opencode) including interpreter-wrapped launches (`node`/`bun`/`python` plus OmO's bundled `cli.js`), derives process start time from `etime`, measures subtree CPU. |
| `snapshot.go` | Orchestrates collection: `Collect(now)` is the tmux entry point and `CollectBackend(now, backend)` selects tmux or Herdr (Herdr runs under a 10 second timeout). Calls `snapshotProcs()`, `interactiveAgents()` or `herdrAgents()`, `backgroundAgents()`, and `ompBackgroundAgents()`, drops background records whose daemon is not live, groups by `projectKey()` (git root or working directory), sorts by recency, attention-rank, then kind. A Herdr failure returns the partial snapshot together with the error. |
| `jobs.go` | Parses Claude Code background job state from `~/.claude/jobs/<id>/state.json`. Decodes `jobState` struct (state, detail, model, tokens, intent, tempo, suggested reply). Correlates with daemon liveness via rendezvous socket check in `/tmp/cc-daemon-*/*/rv/*.sock`. |
| `omp.go` | Parses Oh My Pi daemon registry (`~/.omp/run/daemons/<hash>/daemons/<name>/meta.json`). Decodes omp daemon state (starting/running/stopped/exited/failed). Resolves project directory via client registry, checks daemon process liveness via PID signaling. |
| `liveness.go` | Verifies daemon liveness: `liveShorts()` globs Claude Code rendezvous sockets (`/tmp/cc-daemon-*/*/rv/*.sock`), `processAlive()` probes PID with signal 0 for process existence. Both are authoritative runtime signals, not file timestamps. |
| `collect_test.go` | 31 deterministic unit tests: parser/liveness via fixture data (no live tmux dependency). Covers `detectTool()` (including OmO launch shapes), `parsePaneLine()`, `parseProcLine()`, `interactiveLife()` (spinner glyphs), `looksLikeChoicePrompt()` (with ANSI stripping and scrollback), `contentShowsSpinner()`, `previewFingerprint()`, `trailingLines()`, `normalizeLife()`, `jobToAgent()`, `appendLiveBackgrounds()`, and project grouping/attention-ranking functions. |
| `omp_test.go` | 5 omp-specific tests: `ompNormalizeLife()` (state→lifecycle with daemon liveness), `ompTaskLive()` (task state vs. owner daemon liveness), `msToTime()` (epoch-ms parsing), `ompTaskToAgent()` (daemon task→Agent), liveness propagation. |
| `herdr_test.go` | Offline Herdr tests driven by fake `ps` and `herdr` (and `tmux`) shell scripts on `PATH`: multi-session normalization and namespaced IDs, one `ps` per refresh, incomplete inventory (partial, malformed, missing, no pane ID, process-info failure, absent binary) returning a nil `LivePaneIDs`, preview-capture failure keeping the agent, empty inventory counting as complete, tmux identity unchanged, and Claude subagent ownership under Herdr. |
| `interactive_test.go` | Verifies `interactiveAgents` runs four pane captures concurrently (fake `tmux` re-executes the test binary against an HTTP barrier), keeps pane order and ownership, and leaves a failed capture with an empty preview. |
| `claude_subagents_test.go` | Fixture tests for `claudeSubagents`: exact live session match, rejection of reused/wrong PID, wrong pane, and malformed or empty records, nested/finished/orphan/cyclic parent chains, a finished child disappearing on the next read, and promotion of an idle parent to working while an input prompt stays `LifeInput`. |
| `omo_checklist_test.go` | Tests `omoChecklist` against fixture holder files and transcripts: session lookup by PID and start time, partial trailing lines held back until complete, error and non-todo records ignored, timing derivation, `init` resetting timings, and a shrunken transcript being reread. |
| `omo_choice_test.go` | Tests the OmO pending-question screen: current card detected (also with ANSI color), answered/scrollback/other-tool cases rejected, input not mutating the preview, and a pending question overriding a running child's spinner in `interactiveAgents`. |
| `opencode_status_test.go` | Tests the OpenCode `↓ N subagent(s) ·` footer as a working signal: only a positive running count within the scan window and only for the `opencode` tool. |
| `proc_snapshot_test.go` | Tests `snapshotProcs` with a fake `ps`: a 128 KiB argument line is kept intact, a line above the 1 MiB scanner limit returns an error. |
| `live_probe_test.go` | Opt-in live probe (`AMUX_LIVE_PROBE=1`) that runs `Collect` against the real tmux server and logs interactive agents; skipped by default. |
| `herdr_live_test.go` | Opt-in read-only Herdr probe (`AMUX_HERDR_LIVE_SESSION=<name>`): compares the live `session.snapshot` panes with `CollectBackend(…, BackendHerdr)` inventory, checks agents are normalized, and optionally requires a preview containing `AMUX_HERDR_LIVE_EXPECT`; skipped by default. |
| `performance_test.go` | Opt-in benchmarks (`AMUX_LIVE_BENCH=1`): `BenchmarkCollectLive` times a full `Collect`, and `BenchmarkCollectionStages` times each stage (ps, list-panes, interactive, capture, background, group, project key) against the current machine. |

## Subdirectories

| Directory | Purpose |
|-----------|---------|
| _None_ | This package has no subdirectories. |

## For AI Agents

### Working In This Directory
- Treat all discovery sources (tmux, Herdr, ps, sockets, home-directory state files) as unreliable external inputs. Missing servers, unparseable files, and transient pane closures must not crash collection.
- Collection runs on a recurring refresh cycle and must remain fast: keep process snapshots bounded to one `ps` per refresh, one `tmux list-panes` call (tmux) or the per-session Herdr calls, reuse the in-memory process tree, and keep liveness checks proportional to discovered daemon records. Pane captures run on at most 4 workers.
- An incomplete pane inventory must leave `Snapshot.LivePaneIDs` nil (tmux `list-panes` failure, any Herdr failure) so callers do not treat missing panes as deleted; agents found before the failure are still returned.
- Normalize all raw agent state (Claude's `state` field, omp's `state` enum, tmux pane title spinners, pane content heuristics) into the unified `Lifecycle` enum before propagating to UI or commands.
- Liveness is an authoritative runtime signal from sockets or process existence, not inferred from timestamps or file modification times. A "working" job whose daemon socket is gone is stale; a "working" job with a live socket stays working no matter how long ago `state.json` was written. The same rule applies to per-session records (Claude sessions, OmO holders): they are trusted only when PID and process start time match the live process.
- Per-refresh reads of tool state must stay small: Claude session and subagent metadata is size-capped, and OmO transcripts are read incrementally from a cached offset, never rescanned.
- Pane titles are not a lifecycle signal for current tool versions. Claude Code 2.1.237 leaves `✳ <task>` in the title after a turn ends, and omp's title is static, so both tools are judged from their pane body instead (`contentShowsSpinner`). A spinner-glyph title still wins where present, keeping older Claude Code builds working. Any new working-state marker must be verified against panes captured live in **both** states before it is trusted — the star family was added and removed once already on a one-sided reading.
- Every content heuristic is bounded to the trailing lines of the capture, because scrollback (including this repo's own source quoted into a pane) contains the marker text verbatim.

### Testing Requirements
- Run `go test ./internal/collect` while iterating and `go test ./...` before completion.
- Write fixture-based tests with explicit expected cases, not live-tmux-dependent tests. All parser and lifecycle tests must work offline with canned data.
- Test each discovery source independently: interactive agents via canned pane data, Herdr via fake `herdr`/`ps`/`tmux` scripts placed first on `PATH` with `HOME` set to a temp dir, Claude jobs and sessions via fixture `state.json` and `.meta.json` structures, OmO via fixture holder files and JSONL transcripts, omp daemons via fixture directory layouts and JSON files.
- Cover boundary cases: missing files, empty directories, malformed JSON, invalid process fields, absent daemon directories, dead sockets, closed panes, zero PIDs, reused PIDs (start time mismatch), partial trailing transcript lines, oversized process lines, and incomplete Herdr inventories.
- Live tests are opt-in and skipped by default; they inspect the current machine and are not part of the offline suite: `AMUX_LIVE_PROBE=1` (`live_probe_test.go`), `AMUX_HERDR_LIVE_SESSION` with optional `AMUX_HERDR_LIVE_EXPECT` (`herdr_live_test.go`, read-only), and `AMUX_LIVE_BENCH=1` for the benchmarks in `performance_test.go`.
- Heuristic tables (`collect_test.go`, `opencode_status_test.go`, `omo_choice_test.go`) must include a scrollback case proving a marker outside the scan window is ignored, and an other-tool case proving the marker is scoped to its tool.

### Common Patterns
- Collection separates source-specific parsing (`interactiveAgents`, `herdrAgents`, `backgroundAgents`, `ompBackgroundAgents`) from the normalized `Snapshot` model; tmux and Herdr share `collectInteractivePreview` for enrichment.
- All state normalization flows through `Lifecycle` and `Kind` to ensure consistent semantics across tmux spinners, Claude job state strings, and omp daemon enums.
- Daemon liveness is determined once per refresh (socket glob + PID signaling) and checked before including an agent in the snapshot.
- Tests fake external commands by writing shell scripts named `ps`, `tmux` or `herdr` into a temp dir and setting `PATH` to it.

## Dependencies

### Internal
- Defines `Agent`, `Project`, and `Snapshot` types consumed by `internal/ui` (rendering) and `cmd/amux` (main loop refresh).
- Depends on `internal/herdr` (bounded Herdr CLI runner used by `herdr.go`; its socket `Call` is used by `herdr_live_test.go`). No other internal package dependencies; collection is otherwise a pure function of external system state.

### External
- **Process Inspection**: Go standard library `os/exec` (run `ps`), `os` (PID signaling for liveness), `syscall` (signal handling).
- **Filesystem**: Go standard library `os`, `filepath` (home directory, stat, ReadDir, Glob for sockets and OmO holders).
- **Tmux**: External `tmux` binary (`list-panes`, `capture-pane` commands), parsed as `\x1f`-separated text and ANSI-stripped output.
- **Herdr**: External `herdr` binary (`session list`, `api snapshot`, `pane process-info`, `pane read`), JSON-decoded.
- **Claude Code**: `~/.claude/jobs/<id>/state.json` (JSON-decoded), `~/.claude/sessions/<pid>.json` and `~/.claude/projects/*/<sessionId>/subagents/agent-*.meta.json` (JSON-decoded), `/tmp/cc-daemon-*/*/rv/*.sock` (socket existence via filepath.Glob).
- **OmO**: `~/.omo/agent/sessions/--<cwd>--/` holder files and JSONL transcripts (JSON-decoded).
- **Oh My Pi**: `~/.omp/run/daemons/<hash>/daemons/<name>/meta.json` (JSON-decoded), PID lookups via signal 0 probe.
- **Data Formats**: Go `encoding/json` (Claude, OmO and OMP state files, Herdr output), `time` (start time approximation via ps etime parsing).

## Manual Notes

Notes under this heading are written by humans and preserved on regeneration.
