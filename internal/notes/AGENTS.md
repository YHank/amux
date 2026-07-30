<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-07-23 | Updated: 2026-07-23 -->

# notes

## Purpose
Persists short, user-authored annotations for tracked agents in an application configuration JSON file and prunes notes when the associated agent no longer exists.

## Key Files

| File | Description |
|------|-------------|
| `notes.go` | Loads, saves, updates, deletes, and prunes note records using the user configuration directory. |
| `notes_test.go` | Unit tests for note persistence and lifecycle behavior. |

## Subdirectories

| Directory | Purpose |
|-----------|---------|
| _None_ | This package has no subdirectories. |

## For AI Agents

### Working In This Directory
- Keep notes isolated from tmux, Claude, and OMP state; this package owns only amux's configuration file.
- Preserve best-effort, error-swallowing persistence and platform-neutral configuration-directory lookup.
- Do not change note storage format without a compatibility strategy and tests.

### Testing Requirements
- Run `go test ./internal/notes` while iterating and `go test ./...` before completion.

### Common Patterns
- Storage uses standard-library JSON and `os.UserConfigDir()`-based path selection.
- Notes are keyed by agent identifier and pruned against the current snapshot.

## Dependencies

### Internal
- Called by `internal/ui` and CLI action handlers.

### External
- Uses the Go standard library only.

<!-- MANUAL: Any manually added notes below this line are preserved on regeneration -->
