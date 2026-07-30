<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-07-23 | Updated: 2026-07-23 -->

# amux CLI

## Purpose
Implements the `amux` executable: Cobra command construction, interactive application startup, popup/status/setup-keybind subcommands, and user actions that affect tmux panes.

## Key Files

| File | Description |
|------|-------------|
| `main.go` | Defines the root command, version metadata, and default TUI startup path. |
| `commands.go` | Defines `popup`, `status`, and `setup-keybind` commands plus tmux configuration helpers. |
| `action.go` | Dispatches the post-TUI attach action or prints the resume command for a headless agent. |

## Subdirectories

| Directory | Purpose |
|-----------|---------|
| _None_ | This package has no subdirectories. |

## For AI Agents

### Working In This Directory
- Keep Cobra flag definitions and user-visible command behavior here.
- Quote shell/tmux arguments safely; tmux interactions affect a user's live sessions.
- Route agent discovery through `internal/collect` and UI state, including note persistence, through `internal/ui`.

### Testing Requirements
- Run `go test ./...` after changing command semantics.
- Smoke-test tmux-affecting commands in a disposable tmux session where practical.

### Common Patterns
- Root and subcommands return `*cobra.Command` constructors.
- The release build injects `main.version` through Makefile linker flags.

## Dependencies

### Internal
- `internal/collect` supplies snapshots and statuses.
- `internal/ui` runs the Bubble Tea application and owns its transitive note persistence.

### External
- `github.com/spf13/cobra` provides command and flag parsing.

<!-- MANUAL: Any manually added notes below this line are preserved on regeneration -->
