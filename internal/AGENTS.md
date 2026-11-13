# internal

**Parent context:** `../AGENTS.md`
**Generated:** 2026-07-23 · **Updated:** 2026-11-13

## Purpose
Holds the non-public application packages: agent discovery and domain models, bounded Herdr CLI/socket access, local note storage, Codex-backed note and title generation, and Bubble Tea presentation. Go's `internal` convention prevents imports from outside this repository subtree.

## Key Files

| File | Description |
|------|-------------|
| _None_ | Functionality is organized into package subdirectories. |

## Subdirectories

| Directory | Purpose |
|-----------|---------|
| `collect/` | Collects normalized snapshots from tmux and Herdr (interactive agents) and Claude/OMP daemons (background agents). Provides unified `Snapshot`, `Project`, and `Agent` models (see `collect/AGENTS.md`). |
| `herdr/` | Bounded wrapper around the installed `herdr` CLI plus a newline-delimited JSON socket RPC client (see `herdr/AGENTS.md`). |
| `notegen/` | Integrates local Codex with amux: model catalog and selection, pane-based title generation, and the detached note-generation worker (see `notegen/AGENTS.md`). |
| `notes/` | Persists user-authored annotations for agents keyed by session/pane ID in application config JSON, with a separate file for Herdr notes. Prunes notes when agents finish (see `notes/AGENTS.md`). |
| `ui/` | Bubble Tea model, message handling, list/detail rendering, menus, and terminal styling. Consumes `collect` snapshots and manages notes (see `ui/AGENTS.md`). |

## Data Flow

```
tmux panes + herdr sessions + process tree + ~/.claude/jobs/ state
              ↓
        collect.Snapshot
              ↓
    ui.Model (Bubble Tea state)
     /          |            \
Rendering   notes.Store    notegen (Codex titles/notes)
            (persistence)  herdr  (focus / launch actions)
```

- **Collect** is stateless: each refresh interrogates tmux and Herdr, reads daemon directories, and emits a fresh snapshot.
- **Herdr** is a thin transport layer; `collect` uses it for discovery and `ui` uses it for actions.
- **UI** owns Bubble Tea event handling, filtering, sorting, selection, and two-pane layout. Commands deferred from Update trigger pane actions (send text, kill, launch, move).
- **Notes** handles per-agent persistence outside of tmux/Claude/OMP state, with best-effort locking and multi-dashboard coordination via periodic Reload.
- **Notegen** runs Codex-backed generation either inline (titles) or in a detached worker process that writes results back through `notes`.

## For AI Agents

### Working In This Directory
- Preserve the layering: `herdr` and `notes` import no internal packages; `collect` may import `herdr`; `notegen` may import `collect` and `notes`; `ui` may import all four; only `cmd/amux` wires them together. Do not introduce an import that closes a cycle.
- Keep OS/process, socket, and external command interactions (tmux, herdr, codex) bounded by timeouts or contexts and error-aware.
- Do not expose internal packages as a public API.

### Testing Requirements
- Run `go test ./...`; package tests live alongside their implementation.

### Common Patterns
- Packages favor explicit models over coupling rendering directly to system inspection.
- Tests use the standard Go `testing` package in `_test.go` files.

## Dependencies

### Internal
- `collect` imports `herdr`.
- `notegen` imports `collect` and `notes`.
- `ui` imports `collect`, `herdr`, `notegen`, and `notes`; it consumes `collect.Snapshot`, `collect.Agent`, and `collect.Project` and calls `notes.Store.{Get,Set,Reload,Prune}`.
- `herdr` and `notes` have no internal package dependencies.

### External
- `ui` uses **Bubble Tea** for event-driven state machine, **Lip Gloss** for terminal styling, and **Reflow** for text wrapping/layout.
- `collect`, `herdr`, `notegen`, and `notes` use only the Go standard library (process inspection, filesystem, JSON, sockets, time, syscall advisory file locking); `notegen` shells out to the installed `codex` and `tmux` binaries and `herdr` to the `herdr` binary.

## Manual Notes

Notes under this heading are written by humans and preserved on regeneration.
