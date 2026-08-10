<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-07-23 | Updated: 2026-08-10 -->

# collect

## Purpose
Collects a normalized snapshot of AI coding agents. It interrogates tmux panes and process trees for interactive agents, reads Claude and Oh My Pi daemon state for headless agents, and determines agent liveness.

## Key Files

| File | Description |
|------|-------------|
| `model.go` | Shared agent, status, project, and snapshot data structures. |
| `tmux.go` | Lists tmux panes and captures tmux metadata. |
| `proc.go` | Walks process information to identify supported AI tools. |
| `snapshot.go` | Captures terminal-pane content for previews and state detection. |
| `jobs.go` | Reads Claude Code background job state. |
| `omp.go` | Reads Oh My Pi daemon metadata. |
| `liveness.go` | Verifies daemon liveness from sockets or owning processes. |
| `collect_test.go` | Unit coverage for collection and classification behavior. |
| `omp_test.go` | Unit coverage for OMP daemon parsing. |

## Subdirectories

| Directory | Purpose |
|-----------|---------|
| _None_ | This package has no subdirectories. |

## For AI Agents

### Working In This Directory
- Treat tmux, `ps`, sockets, and home-directory state files as unreliable external inputs; handle absence and malformed data without crashing the dashboard.
- Keep collection bounded: this code runs on the recurring refresh path.
- Update `model.go` before propagating a new data field to UI or commands.

### Testing Requirements
- Run `go test ./internal/collect` while iterating and `go test ./...` before completion.
- Add deterministic fixture-style tests for parser and liveness behavior; avoid dependence on a developer's live tmux state.

### Common Patterns
- Collection separates source-specific parsing from the normalized snapshot model.
- Liveness is checked from authoritative runtime evidence rather than timestamps.

## Dependencies

### Internal
- The package defines models consumed by `internal/ui` and `cmd/amux`.

### External
- Uses the Go standard library for process execution, filesystem access, JSON decoding, and Unix socket/process inspection.

<!-- MANUAL: Any manually added notes below this line are preserved on regeneration -->
