# cmd

**Parent context:** `../AGENTS.md`
**Generated:** 2026-07-23 · **Updated:** 2026-11-13

## Purpose
Contains executable entrypoints for the `amux` project. Implements the CLI command structure using Cobra, command wiring to internal services, and user-facing actions that interact with tmux, Herdr, and Claude agents.

## Key Files

| File | Description |
|------|-------------|
| `amux/main.go` | Defines the root `amux` command, version metadata, `--backend` flag, default TUI launch, and Cobra subcommand registration. |
| `amux/backend.go` | Resolves `--backend tmux\|herdr` (falling back to the saved selection) for commands. |
| `amux/commands.go` | Implements `popup`, `status`, and `setup-keybind` subcommands; handles tmux configuration file parsing and rewriting. |
| `amux/action.go` | Dispatches post-TUI user actions: tmux/Herdr pane attachment and Claude Code launcher integration. |
| `amux/popup.go` | tmux popup attach via a temporary linked session. |
| `amux/herdr_action.go`, `herdr_attach.go`, `herdr_plugin.go` | Herdr focus/popup dispatch, PTY-based `herdr terminal attach` with detach-key translation, and generated Herdr plugin manifest. |
| `amux/note_worker.go` | Hidden `note-worker` command for detached note generation. |
| `amux/*_test.go` | Unit and tmux/PTY/socket fixture tests for the files above (see `amux/AGENTS.md`). |

## Subdirectories

| Directory | Purpose |
|-----------|---------|
| `amux/` | Root command package, subcommand implementations, and action dispatch (see `amux/AGENTS.md`). |

## For AI Agents

### Working In This Directory
- Keep CLI command definitions and flag parsing in the appropriate command files.
- Quote shell and tmux arguments carefully; actions directly affect the user's live tmux and Herdr sessions and Claude Code environment.
- Route agent discovery and UI state through `internal/collect` and `internal/ui`, Herdr access through `internal/herdr`, and note generation through `internal/notegen`; do not duplicate logic.
- Do not import `internal` packages from external modules; this package is part of the application boundary.

### Testing Requirements
- Run `go test ./...` after changing command behavior or tmux/Herdr interaction logic.
- Verify `setup-keybind` command handles existing `~/.tmux.conf` files correctly, including in-place replacement of marked sections.
- Smoke-test tmux/Herdr-affecting commands (attach, popup) in a disposable tmux session or Herdr session when making behavioral changes.

### Common Patterns
- Cobra commands are constructed as functions returning `*cobra.Command`.
- The release build injects `main.version` through Makefile linker flags (not hand-edited).
- Tmux operations use careful quoting: `tmuxDQuote` for config values, `shellQuote` for shell command composition.
- Actions run either in-tmux/in-Herdr (attach, create window) or spawn external processes (Claude Code) with preserved CWD and client context.

## Dependencies

### Internal
- `internal/collect` provides `Snapshot` and backend identifiers for discovering tmux/Herdr sessions and agents.
- `internal/herdr` provides bounded Herdr CLI and socket access for focus and popup actions.
- `internal/notegen` provides the detached note-generation worker.
- `internal/ui` provides the Bubble Tea model, action types, and notes persistence.

### External
- `github.com/spf13/cobra` for command and flag parsing.
- `github.com/charmbracelet/bubbletea` for TUI program initialization (imported via `internal/ui`).
- `github.com/creack/pty` for the Herdr terminal attach PTY.

## Manual Notes

Notes under this heading are written by humans and preserved on regeneration.
