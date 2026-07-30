<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-07-23 | Updated: 2026-07-23 -->

# internal

## Purpose
Holds the non-public application packages: discovery and domain models, local note storage, and Bubble Tea presentation. Go's `internal` convention prevents imports from outside this repository subtree.

## Key Files

| File | Description |
|------|-------------|
| _None_ | Functionality is organized into package subdirectories. |

## Subdirectories

| Directory | Purpose |
|-----------|---------|
| `collect/` | Discovers agents from tmux, processes, and daemon state stores (see `collect/AGENTS.md`). |
| `notes/` | Stores and prunes user-authored per-agent notes (see `notes/AGENTS.md`). |
| `ui/` | Bubble Tea model, update handling, rendering, and styles (see `ui/AGENTS.md`). |

## For AI Agents

### Working In This Directory
- Preserve the data flow: `collect` produces domain snapshots; `ui` renders and acts on them; `notes` owns persistent annotations.
- Keep OS/process and external command interactions bounded and error-aware.
- Do not expose internal packages as a public API.

### Testing Requirements
- Run `go test ./...`; package tests live alongside their implementation.

### Common Patterns
- Tests use the standard Go `testing` package in `_test.go` files.
- Packages favor explicit models over coupling rendering directly to system inspection.

## Dependencies

### Internal
- `ui` consumes `collect` snapshot models and `notes` persistence.

### External
- `ui` uses Bubble Tea, Lip Gloss, and Reflow.

<!-- MANUAL: Any manually added notes below this line are preserved on regeneration -->
