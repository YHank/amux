<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-07-23 | Updated: 2026-08-20 -->

# amux CLI

## Purpose
Implements the `amux` executable: Cobra command construction, interactive application startup via Bubble Tea, popup/status/setup-keybind subcommands, and post-TUI action dispatch for tmux attachment and Claude Code process launching.

## Key Files

| File | Description |
|------|-------------|
| `main.go` | Root `amux` command with version metadata, default TUI launch via `runTUI()`, and Cobra subcommand registration. |
| `commands.go` | Implements `popupCmd`, `statusCmd`, and `setupKeybindCmd` with tmux-safe quoting (`tmuxDQuote`), keybind file editing, and status bar formatting. |
| `action.go` | Dispatches user actions: `performAction` routes to `attachTmux` (pane navigation) or `launchClaudeAction` (window creation and Claude agent launch). |
| `commands_test.go` | 9 unit tests covering action argument marshalling, tmux window command quoting, popup client context, status line ordering, and safe keybind file replacement. |

## Subdirectories

| Directory | Purpose |
|-----------|---------|
| _None_ | This package has no subdirectories. |

## For AI Agents

### Working In This Directory
- Keep Cobra flag definitions and user-visible command behavior in this package; do not duplicate logic with `internal/collect` or `internal/ui`.
- Quote tmux config values with `tmuxDQuote` (backslash-escape in double quotes); quote shell arguments with `shellQuote` (POSIX single-quote escaping).
- Preserve client context when launching tmux popups; use `AMUX_CLIENT` environment variable for multi-client scenarios.
- Root command with no arguments runs `runTUI()` to launch the Bubble Tea dashboard; subcommands have their own RunE handlers.

### Testing Requirements
- Run `go test ./...` after changing command semantics or tmux interaction logic.
- Smoke-test tmux-affecting commands in a disposable tmux session when making behavioral changes; verify keybind setup handles in-place replacement correctly.

### Common Patterns
- Cobra commands are constructed as functions returning `*cobra.Command`.
- The release build injects `main.version` through Makefile linker flags (not edited by hand).
- Subcommands (`popup`, `status`, `setup-keybind`) return `*cobra.Command` with flag definitions and RunE handlers.
- Post-TUI actions marshalled through `ui.Action` type: dispatched by `performAction` and routed to `attachTmux` or `launchClaudeAction`.

## Dependencies

### Internal
- `internal/collect` provides `Snapshot` for discovering tmux sessions and agent status.
- `internal/ui` provides `Model` (Bubble Tea), `Action` type for post-TUI dispatch, and note persistence.

### External
- `github.com/spf13/cobra` for command and flag parsing.
- `github.com/charmbracelet/bubbletea` for TUI program runtime (imported via `internal/ui`).

<!-- MANUAL: Any manually added notes below this line are preserved on regeneration -->
