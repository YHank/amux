<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-07-23 | Updated: 2026-07-23 -->

# cmd

## Purpose
Contains executable entrypoints. This repository currently exposes one `amux` command.

## Key Files

| File | Description |
|------|-------------|
| _None_ | Command implementation is contained in the `amux` subdirectory. |

## Subdirectories

| Directory | Purpose |
|-----------|---------|
| `amux/` | Root command, Cobra subcommands, and interactive action handlers (see `amux/AGENTS.md`). |

## For AI Agents

### Working In This Directory
- Add executable-specific code beneath its command directory; do not import `internal` packages from external modules.
- Keep CLI parsing and command wiring separate from collection and rendering implementation.

### Testing Requirements
- Run `go test ./...` after changing command behavior.

### Common Patterns
- Commands are built with Cobra and call `internal` packages for implementation.

## Dependencies

### Internal
- `amux/` uses the repository's `internal` packages.

### External
- None directly at this directory level.

<!-- MANUAL: Any manually added notes below this line are preserved on regeneration -->
