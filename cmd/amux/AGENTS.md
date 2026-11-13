# amux CLI

**Parent context:** `../AGENTS.md`
**Generated:** 2026-07-23 · **Updated:** 2026-11-13

## Purpose
Implements the `amux` executable: Cobra command construction, interactive application startup via Bubble Tea, the `--backend tmux|herdr` selection, popup/status/setup-keybind subcommands, hidden `note-worker` and `herdr-attach` subcommands, and post-TUI action dispatch for tmux/Herdr attachment and Claude Code process launching.

## Key Files

| File | Description |
|------|-------------|
| `main.go` | Root `amux` command with version metadata, a `--backend` flag, default TUI launch via `runTUIWithBackend` (loops on popup actions and resumes the model), and registration of `popup`, `status`, `setup-keybind`, `note-worker`, and `herdr-attach`. |
| `backend.go` | `addBackendFlag` and `commandBackend`: resolve `--backend` to `collect.BackendTmux` or `collect.BackendHerdr`, fall back to `ui.SelectedBackend()` (saved selection) when empty, and reject other values with `unknown backend`. |
| `commands.go` | Implements `popupCmd`, `statusCmd`, and `setupKeybindCmd` with tmux-safe quoting (`tmuxDQuote`), keybind file editing, and status bar formatting. `popup` delegates to `openHerdrDashboard` for Herdr when `HERDR_ENV=1`, otherwise runs `tmux display-popup` and requires `TMUX`. |
| `action.go` | Dispatches user actions: `performAction` sends Herdr-backend actions to `performHerdrAction`, otherwise routes to `attachPopup`/`attachTmux` (pane navigation) or `launchClaudeAction` (window creation and Claude agent launch). Also holds the shared helpers `shellQuote`, `shellJoin`, `runInherit`, and `runInheritDir`. |
| `popup.go` | tmux popup attach: `createPopupSession` links the target window and its project peers into a temporary `amux-popup-<pid>` session, `runningInPopup` detects an existing popup, and `attachPopup` reuses that popup or opens `display-popup`, then kills the temporary session. |
| `herdr_action.go` | Herdr action dispatch: `performHerdrAction` (`attach` focuses the pane, opens a Herdr plugin popup, uses tmux `display-popup`, or attaches in place; `manage` launches Claude), `openHerdrDashboard`, `openHerdrSurface` (links the plugin and calls `plugin.pane.open`), and the hidden `herdr-attach` command that reads `AMUX_HERDR_SESSION` and `AMUX_HERDR_TERMINAL`. |
| `herdr_attach.go` | `runHerdrAttach` runs `herdr terminal attach --takeover` on a PTY and translates amux's Ctrl-Space,d detach into Herdr's Ctrl-B,q; `detachKeys` and `csiKey` recognize legacy NUL, kitty CSI u, and modifyOtherKeys encodings. |
| `herdr_plugin.go` | `writeHerdrPlugin` writes `herdr-plugin.toml` (plugin `amux.dashboard` with `dashboard` and `terminal` panes that run this executable) under `<UserConfigDir>/amux/herdr-plugin`. |
| `note_worker.go` | Hidden `note-worker` command that calls `notegen.Work` with stdin; `notegen.Start` launches it as a detached (`Setsid`) background process so note generation and persistence outlive the dashboard. |
| `commands_test.go` | 9 unit tests covering Claude action arguments, tmux window command quoting, invoking-session and focus argument selection, status line ordering, key validation, `tmuxDQuote`, and keybind block replacement. |
| `backend_test.go` | Tests that the `--backend` flag overrides the saved selection and that `status` rejects an unknown backend. |
| `popup_test.go` | Tests against a real tmux server: popup keeps the agent when the source session selects amux, groups project windows while keeping source panes, and detects a popup without an `AMUX_CLIENT` marker. |
| `popup_size_test.go` | Tests that a popup attached without `TMUX` keeps the terminal size; `TestPopupSizeFixture` is a child-process fixture gated by `AMUX_POPUP_SIZE_TARGET`. |
| `herdr_action_test.go` | Asserts the Herdr wire contract with a fake Unix socket: fullscreen attach focuses the captured pane. |
| `herdr_attach_test.go` | PTY tests for `runHerdrAttach` detach-key translation across legacy, kitty, and modifyOtherKeys input; `TestHerdrAttachFixture` is a child-process fixture gated by `AMUX_HERDR_ATTACH_ROLE`. |

## Subdirectories

| Directory | Purpose |
|-----------|---------|
| _None_ | This package has no subdirectories. |

## For AI Agents

### Working In This Directory
- Keep Cobra flag definitions and user-visible command behavior in this package; do not duplicate logic with `internal/collect`, `internal/herdr`, or `internal/ui`.
- Add `--backend` to a command through `addBackendFlag` and read it with `commandBackend`; do not parse the flag by hand.
- Branch tmux versus Herdr behavior in `performAction` (by `ui.Action.Backend`) and in `popupCmd`; Herdr-specific code lives in the `herdr_*.go` files.
- Quote tmux config values with `tmuxDQuote` (backslash-escape in double quotes); quote shell arguments with `shellQuote` (POSIX single-quote escaping).
- Preserve client context when launching tmux popups; use `AMUX_CLIENT` environment variable for multi-client scenarios.
- Pass Herdr session and terminal targets to the `herdr-attach` entrypoint through `AMUX_HERDR_SESSION` and `AMUX_HERDR_TERMINAL`, never through shell-evaluated arguments.
- Keep `note-worker` and `herdr-attach` hidden; they are internal entrypoints launched by amux itself.
- Root command with no arguments runs the Bubble Tea dashboard via `runTUIWithBackend`; subcommands have their own RunE handlers.

### Testing Requirements
- Run `go test ./...` after changing command semantics or tmux interaction logic.
- `popup_test.go`, `popup_size_test.go`, and `herdr_attach_test.go` start real tmux servers or PTY child processes; the `tmux` binary must be on `PATH`.
- Smoke-test tmux-affecting commands in a disposable tmux session when making behavioral changes; verify keybind setup handles in-place replacement correctly.

### Common Patterns
- Cobra commands are constructed as functions returning `*cobra.Command`.
- The release build injects `main.version` through Makefile linker flags (not edited by hand).
- Subcommands (`popup`, `status`, `setup-keybind`) return `*cobra.Command` with flag definitions and RunE handlers; `note-worker` and `herdr-attach` are `Hidden: true` and take no arguments.
- Post-TUI actions marshalled through `ui.Action` type: dispatched by `performAction` and routed to `attachTmux`, `attachPopup`, `launchClaudeAction`, or `performHerdrAction`.
- Tests that need an external program re-execute the test binary as a fixture (`TestPopupSizeFixture`, `TestHerdrAttachFixture`), skipped unless their environment variable is set.

## Dependencies

### Internal
- `internal/collect` provides `Snapshot`, `Backend` (`BackendTmux`, `BackendHerdr`), and `CollectBackend` for discovering agents and status.
- `internal/ui` provides `Model` (Bubble Tea), `Action` type for post-TUI dispatch, `SelectedBackend`, and note persistence.
- `internal/herdr` provides the Herdr socket `Call`, CLI `Run`, and `Args` helpers.
- `internal/notegen` provides `Work`, the entrypoint behind `note-worker`.

### External
- `github.com/spf13/cobra` for command and flag parsing.
- `github.com/charmbracelet/bubbletea` for TUI program runtime.
- `github.com/charmbracelet/x/term`, `github.com/creack/pty`, and `golang.org/x/sys/unix` for the Herdr attach PTY and raw terminal handling.

## Manual Notes

Notes under this heading are written by humans and preserved on regeneration.
