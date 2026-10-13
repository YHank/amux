# ui

**Parent context:** `../AGENTS.md`
**Generated:** 2026-07-23 · **Updated:** 2026-10-12

## Purpose
Implements the interactive amux dashboard with Bubble Tea: application state and event updates, agent list and detail rendering, styling, and display utilities. The Model owns dashboard state (selection, filter, visibility modes, persistent preferences, notes) and interprets keyboard input; the View renders dashboard presentation; tmux actions run through deferred Bubble Tea commands while note and preference persistence occurs during state updates.

## Key Files

| File | Description |
|------|-------------|
| `app.go` | Bubble Tea Model, Init/Update/View contract, message types (tickMsg, snapshotMsg, sentMsg, killedMsg), event handlers, state mutation, and Bubble Tea command builders for tmux operations. |
| `view.go` | Rendering logic: keyboard help popup, two-pane layout (side-by-side and stacked), list headers/agent rows, detail panel with pane preview, footer controls, and styling application. |
| `styles.go` | Lip Gloss style registry: color palette (green/yellow/blue/gray/red/orange for lifecycle states), lifecycle glyphs (▶ ◆ ● ✖ ⚠ ■ ✓), lifecycle labels, and component styles (header, footer, selected, muted, note). |
| `util.go` | Display helpers: ANSI-aware string truncation, word wrapping, multi-byte rune handling for backspace, compact token/duration formatting (`humanTokens`, `humanDuration`). |
| `app_test.go` | Unit tests for dashboard model updates and interaction behavior. |
| `preferences.go` | Loads and saves detail-panel visibility, layout mode (stacked/side-by-side), and flat-view toggles using the platform configuration directory. |
| `preferences_test.go` | Unit tests for preference loading and saving. |

## Subdirectories

| Directory | Purpose |
|-----------|---------|
| _None_ | This package has no subdirectories. |

## For AI Agents

### Working In This Directory
#### State & Event Handling (Bubble Tea Model Pattern)

**Model**: The `Model` struct is the single source of truth for the dashboard. It holds:
- Current snapshot (`snap`) and display rows (`rows`, `sel`, `selRows`)
- Interactive state: filter, visibility (showDone, flatView, hideDetail, stacked), view mode (grouped/flat)
- Session-local acknowledgements and dismissals, pruned when the corresponding agent no longer qualifies for them
- Note/compose input and UI modes (naming, composing, showHelp, suggestingNote)
- Keyboard confirmation state (confirmingKill)
- Rendering dimensions and error state

**Init**: Launches a refresh scan and a periodic tick every 2 seconds. Discovery runs on each refresh; notes are reloaded separately at a 10-second interval.

**Update** handles messages:
1. `tickMsg` → schedules `refreshCmd` to collect a fresh snapshot; notes reload separately when due
2. `snapshotMsg` → applies collect snapshot, records status-change times, prunes notes/acknowledged/dismissed lists, rebuilds row display
3. `tea.KeyMsg` → delegates to `handleKey` (handles j/k/J/K/g/G/enter/c/x/y///i/a/d/p/r/v/m/q)
4. `tea.WindowSizeMsg` → updates viewport dimensions
5. `sentMsg` updates the notice; a successful `killedMsg` removes its pane from the current snapshot and display

**Tmux commands** are returned from Update and execute asynchronously. `sendToPane` and `killPane` perform pane actions after the relevant interaction state is confirmed. Note and preference persistence occurs synchronously in the update path.

#### View Rendering (Bubble Tea View Function)

The `View()` method assembles the dashboard from the model state; rendered age labels also use the current time.

- **Layout modes**: Side-by-side (list uses 40% of available width and detail receives the remainder) or stacked (list above detail, split by height).
- **List rendering**: Project headers (grouped view) or flat agent names with project prefix; each agent row shows lifecycle glyph (colored), name, note preview, status/age metadata
- **Detail panel**: Shows the selected agent's metadata plus its available preview, status, and task details. Interactive previews are clipped to the available panel space.
- **Help popup**: Scrollable keyboard reference with 16 entries; fits height constraints by truncating or scrolling.
- **Footer**: Context-sensitive controls or input prompts, with a notice or last-refresh timestamp.
- **Terminal constraints**: Renders a too-small message below 20 columns or 5 rows; an open help popup requires at least 6 rows.

#### Keyboard & Interaction Semantics

**Movement** (j/k/↑/↓): Move selection within visible rows, skip headers. J/K navigate between project groups in grouped view; wrapped selection stays at list edge.

**Selection** (enter): For interactive panes, returns an Action with attach target. For live Claude agents, returns a manager action (e.g., open session browser).

**Compose** (c): Only for interactive panes. Opens input mode; Enter submits the text to the selected pane, and Esc cancels without sending.

**Kill** (x): Only for interactive panes (backgrounds are dismissed, not killed). Arms confirmation. y/Y kills; any other key cancels. Kills are irreversible; Model removes the pane from display immediately on success (doesn't wait for next refresh).

**Note** (i): Opens input mode for editing or creating a note on the selected agent. On first note to an interactive pane, offers task-title prefill: strips the leading run of symbols and punctuation, covering omp's `π:` and `π >` prefixes and Claude Code's spinner or star glyph; Codex uses the trimmed pane title as supplied. Other tools and backgrounds offer no suggestion.

**Filter** (/): Substring search across agent name, project, tool name, and saved note. Matching is case-insensitive. Filtering never removes agents from the snapshot (only hides them from display rows). Empty filter shows all; selection is preserved when the selected agent remains visible.

**Acknowledge** (a): Marks an error-state agent as acknowledged; acknowledged errors no longer drive project attention rank and are included only when history is shown.

**Views**:
- **d** (done): Shows or hides completed, stopped, and acknowledged-error history.
- **p** (project): Toggles flat (single prefixed list) vs. grouped (project sections with headers)

#### Ordering

`StatusChangedAt` is the primary sort key in both views, so an agent goes to the top the moment its lifecycle changes and stays there until another agent changes. Three rules keep that promise:

- **Activity never competes with it.** `ActivityAt` moves on nearly every refresh of a busy pane, so it only breaks ties between agents that last changed state at the same instant — which is the common case, since one refresh stamps every transition with its own `Taken` time. Ranking the two together is what buried real transitions in ordinary churn.
- **The startup cohort holds its remembered positions.** Every agent present when amux launches is stamped with the same `Taken` time, so the primary key cannot separate them and the list would otherwise be ordered by whichever weak tie-breaker happened to apply — then visibly resort once the next snapshot arrived. `rememberedRank` (restored from `preferences.json`) settles them instead, gated on `Recency().Equal(initialRecencyAt)` so it governs the startup cohort and nothing else: agents that transition later move by `Recency`, and two agents transitioning in the same refresh tie on a *different* time and fall through to activity as documented above. Look ranks up with the two-value form — a missing agent reads as position zero and would sort above everything the user actually placed.
- **Recency is `collect.Agent.Recency()`: the dashboard's own observation whenever it has one, the state file's timestamp only before that.** `UpdatedAt` is a background state file's *write* timestamp — a working job rewrites it as its token count climbs — so ranking on it would reintroduce the same churn problem the previous rule solves. Interactive agents never carry `UpdatedAt` at all, so both fields are needed for the two kinds to sort against each other; ongoing background progress still breaks ties at the activity tier. This applies to project headers (`visibleProjectLatest`) as much as to rows; keying headers on `UpdatedAt` alone tied every tmux-only project at the zero time and silently fell back to alphabetical order.
- **Attention rank stays a tiebreaker in the flat view, and the leading key for grouped project headers.** Grouped view is attention-first by design; the flat view is explicitly a recency list.
- **`initialCPU` freezes from the first snapshot that has agents**, not the second. It only orders startup-cohort agents with no remembered position, and seeding it a refresh late was itself a visible resort.

`collect.group` applies the same ordering when it builds the snapshot, but `rebuildRows` re-sorts everything the user actually sees — filtering, dismissals, and acknowledgements change which agents a project is ranked on.
- **v** (vertical): Toggles stacked (list above detail) vs. side-by-side layout
- **m** (manage/mute detail): Hides the detail panel so the list fills the viewport

The `p`, `v`, and `m` view preferences persist to `preferences.json` across sessions; `d` is session-local.

`preferences.json` also stores `flat_order` and `grouped_order`: the agent IDs each view last displayed, in order. `p` switches between two genuinely different layouts, so each is remembered apart and restored on entry. `rebuildRows` records the order it just built — except while a filter is active, since a filtered list would evict every agent it hides. `persistViewPreferences` skips writes that would not change the file, which matters because the two-second refresh calls it; the quit and attach paths flush explicitly, so only a hard kill loses the last order. Concurrent amux instances share one file, last writer wins.

#### Rendering Constraints & Resilience

- **Narrow terminals**: Applies width-aware clipping where individual rendering paths require it, preserving ANSI color codes and adding ellipses when truncated.
- **Small viewports**: Renders a too-small message below 20 columns or 5 rows; an open help popup requires at least 6 rows.
- **Incomplete data**: A snapshot can omit an interactive pane preview; detail rendering falls back to other available agent detail fields.
- **Multi-byte text**: Text-input helpers operate on runes, and display helpers account for rendered widths.
- **ANSI codes**: Pane preview preserves the agent's own terminal colors (from `capture-pane -e`). Closes with `\x1b[0m` reset so colors never bleed into amux UI.

### Testing Requirements

- Unit tests use Go's standard `testing` package in `app_test.go` and `preferences_test.go`.
- Run `go test ./internal/ui` while iterating and `go test ./...` before completion; manually exercise changed keyboard or rendering paths in a tmux terminal when practical.

### Common Patterns

1. **State-based rendering**: `View` derives dashboard content from `Model` state, while time-based age labels are computed at render time from `collect.Agent.Age`, which deliberately inverts `Recency()`'s preference: the state file records when a background agent's state actually changed, while `StatusChangedAt` only records when amux first saw it and would report a two-hour-old finished job as seconds old right after launch.
2. **Deferred tmux actions**: Update returns `tea.Cmd` values for pane send and kill operations; note and preference writes remain explicit synchronous state-update work.
3. **Message-driven state**: External events (keyboard, refresh timer) are wrapped in message types and handled sequentially in Update.
4. **Filter applied to display only**: The Snapshot is kept complete; filtering (rebuildRows) only removes rows from display, never from the underlying data, so operations like note pruning see the full agent set.
5. **Acknowledgement/dismissal locality**: Acknowledgement and dismissal maps are session-local and pruned as snapshots change, so stale entries do not persist.
6. **Preference persistence**: The grouped/flat, stacked, and detail-panel view preferences are saved when their toggles change.


## Dependencies

### Internal
- `internal/collect` provides `Snapshot`, `Project`, and `Agent` types; Model consumes one Snapshot per refresh.
- `internal/notes` provides `Store` for get/set/reload/prune operations; Model calls Reload every 10s to see notes saved by other amux instances.

### External
- **Bubble Tea** (`github.com/charmbracelet/bubbletea`) provides the event-driven state machine: Model struct, Init/Update/View contract, message types, commands, and the runtime loop.
- **Lip Gloss** (`github.com/charmbracelet/lipgloss`) provides styled rendering: colors, bold, italic, width/height constraints, pane fitting, and ANSI output.
- **Reflow** (`github.com/muesli/reflow`) provides text utilities: ANSI-aware truncation (truncate.StringWithTail) and word wrapping (wordwrap.String).
- Go standard library: `time`, `strings`, `sort`, `unicode`, `os/exec`, `os`, `path/filepath`, `encoding/json`.

<!-- MANUAL: Any manually added notes below this line are preserved on regeneration -->

## Manual Notes

Notes under this heading are written by humans and preserved on regeneration.
