<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-07-23 | Updated: 2026-08-20 -->

# cmd

## Purpose
Contains executable entrypoints for the `amux` project. Implements the CLI command structure using Cobra, command wiring to internal services, and user-facing actions that interact with tmux and Claude agents.

## Key Files

| File | Description |
|------|-------------|
| `amux/main.go` | Defines the root `amux` command, version metadata, default TUI launch, and Cobra subcommand registration. |
| `amux/commands.go` | Implements `popup`, `status`, and `setup-keybind` subcommands; handles tmux configuration file parsing and rewriting. |
| `amux/action.go` | Dispatches post-TUI user actions: tmux pane attachment and Claude Code launcher integration. |
| `amux/commands_test.go` | Unit tests for CLI argument handling, tmux quoting, keybind setup, and status line formatting. |

## Subdirectories

| Directory | Purpose |
|-----------|---------|
| `amux/` | Root command package, subcommand implementations, and action dispatch (see `amux/AGENTS.md`). |

## For AI Agents

### Working In This Directory
- Keep CLI command definitions and flag parsing in the appropriate command files.
- Quote shell and tmux arguments carefully; actions directly affect the user's live tmux sessions and Claude Code environment.
- Route agent discovery and UI state through `internal/collect` and `internal/ui`; do not duplicate logic.
- Do not import `internal` packages from external modules; this package is part of the application boundary.

### Testing Requirements
- Run `go test ./...` after changing command behavior or tmux interaction logic.
- Verify `setup-keybind` command handles existing `~/.tmux.conf` files correctly, including in-place replacement of marked sections.
- Smoke-test tmux-affecting commands (attach, popup) in a disposable tmux session when making behavioral changes.

### Common Patterns
- Cobra commands are constructed as functions returning `*cobra.Command`.
- The release build injects `main.version` through Makefile linker flags (not hand-edited).
- Tmux operations use careful quoting: `tmuxDQuote` for config values, `shellQuote` for shell command composition.
- Actions run either in-tmux (attach, create window) or spawn external processes (Claude Code) with preserved CWD and client context.

## Dependencies

### Internal
- `internal/collect` provides `Snapshot` for discovering tmux sessions and agents.
- `internal/ui` provides the Bubble Tea model, action types, and notes persistence.

### External
- `github.com/spf13/cobra` for command and flag parsing.
- `github.com/charmbracelet/bubbletea` for TUI program initialization (imported via `internal/ui`).

<!-- MANUAL: Any manually added notes below this line are preserved on regeneration -->
