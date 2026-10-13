# internal

**Parent context:** `../AGENTS.md`
**Generated:** 2026-07-23 · **Updated:** 2026-10-12

## Purpose
Holds the non-public application packages: discovery and domain models, local note storage, and Bubble Tea presentation. Go's `internal` convention prevents imports from outside this repository subtree.

## Key Files

| File | Description |
|------|-------------|
| _None_ | Functionality is organized into package subdirectories. |

## Subdirectories

| Directory | Purpose |
|-----------|---------|
| `collect/` | Collects normalized snapshots from tmux (interactive agents) and Claude/OMP daemons (background agents). Provides unified `Snapshot`, `Project`, and `Agent` models (see `collect/AGENTS.md`). |
| `notes/` | Persists user-authored annotations for agents keyed by session/pane ID in application config JSON. Prunes notes when agents finish (see `notes/AGENTS.md`). |
| `ui/` | Bubble Tea model, message handling, list/detail rendering, and terminal styling. Consumes `collect` snapshots and manages note visibility (see `ui/AGENTS.md`). |

## Data Flow

```
tmux panes + process tree + ~/.claude/jobs/ state
              ↓
        collect.Snapshot
              ↓
    ui.Model (Bubble Tea state)
     /                    \
    /                      \
 Rendering              notes.Store
                      (persistence)
```

- **Collect** is stateless: each refresh interrogates tmux, reads daemon directories, and emits a fresh snapshot.
- **UI** owns Bubble Tea event handling, filtering, sorting, selection, and two-pane layout. Commands deferred from Update trigger pane actions (send text, kill).
- **Notes** handles per-agent persistence outside of tmux/Claude/OMP state, with best-effort locking and multi-dashboard coordination via periodic Reload.

## For AI Agents

### Working In This Directory
- Preserve the data flow: `collect` produces domain snapshots; `ui` renders and acts on them; `notes` owns persistent annotations.
- Keep OS/process and external command interactions bounded and error-aware.
- Do not expose internal packages as a public API.

### Testing Requirements
- Run `go test ./...`; package tests live alongside their implementation.

### Common Patterns
- Packages favor explicit models over coupling rendering directly to system inspection.
- Tests use the standard Go `testing` package in `_test.go` files.

## Dependencies

### Internal
- `ui` consumes `collect.Snapshot`, `collect.Agent`, and `collect.Project` types.
- `ui` calls `notes.Store.{Get,Set,Reload,Prune}` to manage annotations.
- `notes` has no internal package dependencies.
- `collect` has no internal package dependencies.

### External
- `ui` uses **Bubble Tea** for event-driven state machine, **Lip Gloss** for terminal styling, and **Reflow** for text wrapping/layout.
- `collect` uses only the Go standard library (process inspection, filesystem, JSON, sockets, time).
- `notes` uses only the Go standard library (filesystem, JSON, syscall advisory file locking).

<!-- MANUAL: Any manually added notes below this line are preserved on regeneration -->

## Manual Notes

Notes under this heading are written by humans and preserved on regeneration.
