<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-07-23 | Updated: 2026-08-10 -->

# ui

## Purpose
Implements the interactive amux dashboard with Bubble Tea: application state and event updates, agent list and detail rendering, styling, and display utilities.

## Key Files

| File | Description |
|------|-------------|
| `app.go` | Bubble Tea model, initialization, refresh/update handling, and interactive state transitions. |
| `view.go` | Renders the agent list, detail panel, controls, and status information. |
| `styles.go` | Lip Gloss style, color, and layout definitions. |
| `util.go` | Rendering and display helper functions. |
| `app_test.go` | Unit tests for application update and presentation behavior. |
| `preferences.go` | Loads and saves persisted detail-panel visibility and layout preferences. |
| `preferences_test.go` | Unit coverage for preference load and save behavior. |

## Subdirectories

| Directory | Purpose |
|-----------|---------|
| _None_ | This package has no subdirectories. |

## For AI Agents

### Working In This Directory
- Keep Bubble Tea updates deterministic: event handling changes state; commands perform deferred I/O.
- Preserve documented keyboard behavior and the active-first display semantics.
- Keep rendering resilient to narrow terminals and incomplete collection data.

### Testing Requirements
- Run `go test ./internal/ui` while iterating and `go test ./...` before completion.
- For interaction changes, manually exercise the affected key path in a tmux terminal when possible.

### Common Patterns
- Bubble Tea follows model/update/view separation.
- Lip Gloss owns styling rather than embedding terminal escape sequences in view logic.
- The UI consumes normalized `collect` data and accesses notes through the notes package.

## Dependencies

### Internal
- `internal/collect` provides agents and snapshots.
- `internal/notes` persists user annotations.

### External
- `github.com/charmbracelet/bubbletea` provides the state-machine runtime.
- `github.com/charmbracelet/lipgloss` provides terminal styling.
- `github.com/muesli/reflow` assists with terminal text layout.

<!-- MANUAL: Any manually added notes below this line are preserved on regeneration -->
