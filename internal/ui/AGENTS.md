# ui

**Parent context:** `../AGENTS.md`
**Generated:** 2026-07-23 · **Updated:** 2026-11-13

## Purpose
Implements the interactive amux dashboard with Bubble Tea: application state and event updates, agent list and detail rendering, styling, and display utilities. The Model owns dashboard state (selection, filter, visibility modes, persistent preferences, notes, the active tmux/Herdr backend tab) and interprets keyboard input; the View renders dashboard presentation, including the modal popups (help, launch, edit, move, Codex note); tmux and Herdr actions run through deferred Bubble Tea commands while note and preference persistence occurs during state updates.

## Key Files

| File | Description |
|------|-------------|
| `app.go` | Bubble Tea Model, `Action`, Init/Update/View contract, message types (tickMsg, snapshotMsg, sentMsg, killedMsg), `handleKey` dispatch (modal handlers first, then global keys), event handlers, state mutation, row ordering/filtering (`rebuildRows`), and tmux `sendToPane`/`killPane` command builders. |
| `view.go` | Rendering logic: `keyboardHelpEntries` and the help popup, popup routing in `View`, two-pane layout (side-by-side and stacked), backend tabs plus attention header, list headers/agent rows, detail panel with pane preview and subagents, footer controls. |
| `styles.go` | Lip Gloss style registry: color palette (green/yellow/blue/gray/red/orange for lifecycle states), lifecycle glyphs (▶ ◆ ● ✖ ⚠ ■ ✓), lifecycle labels, `kindGlyph` (▤ interactive, ◇ background), and component styles (title, help, project, selection, muted, faint, meta, detail key, note, window, divider, panel). |
| `util.go` | Display helpers: ANSI-aware string truncation, word wrapping, multi-byte rune handling for backspace (`dropLastRune`), rune-safe `clip`, compact token/duration formatting (`humanTokens`, `humanDuration`). |
| `preferences.go` | Loads and saves `preferences.json` under the platform configuration directory (`amux/preferences.json`): selected backend, stacked/hide-detail/show-checklist/flat-view toggles, and the remembered flat/grouped agent order for each backend. |
| `backend.go` | tmux/Herdr backend tabs: `SelectedBackend`, `NewWithBackend`, `switchBackend` (resets snapshot-bound state and bumps `generation` so stale messages are dropped by `acceptBackend`), backend-specific notes store and remembered orders, `refreshCommand` (`collect.CollectBackend`), `interactiveTarget`, namespace-scoped note pruning, `removeAgent`. |
| `backend_actions.go` | Backend-dispatched pane actions: `sendAgentCommand`, `killAgentCommand`, `renameAgentCommand`/`renameCapturedAgent` (tmux window or Herdr tab/pane rename), and `captureNoteAgent` (captures a Herdr pane via `herdr pane read` for note generation). |
| `startup.go` | `startupAgentID` resolves the invoking pane to select after the first snapshot: `AMUX_START_PANE` then `TMUX_PANE` for tmux; `HERDR_ACTIVE_PANE_ID` then `HERDR_PANE_ID`, matched through `HERDR_SOCKET_PATH` and `herdr session list --json` to a `herdr:<session>:<pane>` ID. |
| `launch_menu.go` | Launch popup (`c`): project-path picker with cursor editing, then tool picker; builds the `launchTarget` from the selected agent or startup pane. |
| `launch_command.go` | `launchTools` allowlist (`claude`, `codex`, `omo`, `omp`, `opencode`), `runLaunch` (resolves the owning tmux session, then `tmux new-window`), `launchedMsg`, and `handleLaunched`. |
| `launch_history.go` | Persistent launch-path history (`launch-history.json` beside `preferences.json`), written under a file lock with atomic replace; `launchDirectory` path validation; `rememberSelectedLaunchProject`. |
| `herdr_launch.go` | `runHerdrLaunch`: creates a Herdr tab (or workspace when none is known) without focusing it and runs the tool in its root pane; requires an explicit named session. |
| `move_menu.go` | "Open agent" popup shown on `enter` for an interactive agent: fullscreen or popup attach, `projectTmuxTargets`, and `Resume`. |
| `edit_menu.go` | Edit popup (`i`) choosing window title (`t`) or note (`i`); first-note flow (`beginNoteEdit`), `clearSelectedNote`, `renameWindow` (tmux), `handleRenamedWindow`, and the edit/note popup renderer. |
| `title_picker.go` | Window-title picker inside the edit popup: preset list, `closestTitle` fuzzy match (exact, prefix, substring, then edit distance), add/delete presets. |
| `title_presets.go` | Loads, validates, and atomically saves title presets in `amux/window-titles.json`; `defaultTitlePresets` (`start`, `stay`, `review`, `commit`, `gitedit`, `end`) apply only when the file is missing. |
| `note_menu.go` | Codex note popup (`o`): generate a note or choose the Codex model through `notegen.Models`/`notegen.SelectModel`. |
| `auto_title.go` | Detached Codex note generation (`fetchAutoTitle`): starts a `notegen` worker that outlives amux, tracks requests per agent, applies or cancels results (`applyAutoTitle`, `cancelAutoTitles`, `pruneAutoTitles`). |
| `purpose_title.go` | Foreground Codex purpose-title inference (`fetchPurposeTitle`, 1-minute timeout) that prefills the note editor; falls back to the pane-title suggestion on error. |
| `checklist.go` | OmO plan checklist panel (`l`): shares the detail-panel slot with the preview (`toggleDetailPanel`), renders plan items by status with per-step durations, folds long plans, and pins a "Background tasks" section listing running subagents. |
| `app_test.go` | Unit tests for dashboard model updates and interaction behavior. |
| `preferences_test.go` | Unit tests for preference loading and saving. |
| `*_test.go` (feature files) | Per-feature tests: `auto_title_test.go`, `backend_test.go`, `backend_actions_test.go`, `backend_fixture_test.go`, `checklist_test.go`, `edit_menu_test.go`, `launch_history_test.go`, `launch_history_process_test.go`, `launch_menu_test.go`, `move_menu_test.go`, `note_menu_test.go`, `purpose_title_test.go`, `startup_test.go`, `subagents_test.go`, `title_picker_test.go`, `title_presets_test.go`, `performance_test.go`. |

## Subdirectories

| Directory | Purpose |
|-----------|---------|
| _None_ | This package has no subdirectories. |

## For AI Agents

### Working In This Directory
#### State & Event Handling (Bubble Tea Model Pattern)

**Model**: The `Model` struct is the single source of truth for the dashboard. It holds:
- Current snapshot (`snap`) and display rows (`rows`, `sel`, `selRows`)
- Interactive state: filter, visibility (showDone, flatView, hideDetail, showChecklist, stacked), view mode (grouped/flat)
- Active backend (`backend`, tmux or Herdr) and a `generation` counter; per-backend remembered orders (`tmuxFlatOrder`/`tmuxGroupedOrder`, `herdrFlatOrder`/`herdrGroupedOrder`)
- Session-local acknowledgements and dismissals, pruned when the corresponding agent no longer qualifies for them
- Note/compose input and UI modes (naming, composing, showHelp, suggestingNote, fetchingNote)
- Popup states: `launchMenu`, `editMenu`, `moveAction`/`movePopup`, `showNoteMenu`/`choosingNoteModel`
- Codex note tracking: `autoTitleRequests`/`autoTitleLatest`, `noteLauncher` (test seam for the detached worker)
- Keyboard confirmation state (confirmingKill)
- Rendering dimensions and error state

**Init**: Launches a refresh scan and a periodic tick every 2 seconds. Discovery runs on each refresh; notes are reloaded separately at a 10-second interval.

**Update** handles messages:
1. `tickMsg` → schedules `refreshCommand` to collect a fresh snapshot; notes reload separately when due
2. `snapshotMsg` → dropped unless `acceptBackend` matches its backend and `generation`; otherwise applies the snapshot, records status-change times, prunes auto-title requests/notes/acknowledged/dismissed lists, rebuilds row display, selects `startupPaneID` once, and persists preferences
3. `tea.KeyMsg` → delegates to `handleKey`; when the selection changes, `rememberSelectedLaunchProject` records the project in launch history
4. `tea.WindowSizeMsg` → updates viewport dimensions
5. `sentMsg` updates the notice; a successful `killedMsg` removes its agent (or pane) from the current snapshot and display
6. `launchedMsg`, `renamedWindowMsg`, `autoTitleMsg`, `purposeTitleMsg`, `noteModelsMsg` → handled by the popup/feature files (`handleLaunched`, `handleRenamedWindow`, `applyAutoTitle`, `applyPurposeTitle`, model list loading)

**Key dispatch order** (`handleKey`): launch menu, edit menu, move menu, Codex note menu, `fetchingNote`, kill confirmation, help popup, note-suggestion prompt, compose input, note input, filter input, then the global keys. A mode earlier in this list consumes the key before later ones see it.

**Backend commands** are returned from Update and execute asynchronously. tmux uses `sendToPane`/`killPane` (`app.go`); Herdr uses the `herdr` CLI through `internal/herdr` (`backend_actions.go`, 5-second timeout). Each command captures the backend and `generation` at creation so a result arriving after a tab switch is ignored. Note and preference persistence occurs synchronously in the update path.

#### View Rendering (Bubble Tea View Function)

The `View()` method assembles the dashboard from the model state; rendered age labels also use the current time.

- **Popup precedence**: `View` returns, in order, the help popup, launch menu, edit popup (also used for note entry, the first-note suggestion, and purpose-title fetching), move menu, and Codex note menu; otherwise the dashboard.
- **Header rows**: `renderBackendTabs` (`[tmux]` / `[Herdr]`, with a `Tab/Shift+Tab` hint when it fits) and the attention-count header precede the body; the footer is the last row.
- **Layout modes**: Side-by-side (list uses 40% of available width and detail receives the remainder) or stacked (list above detail, split by height).
- **List rendering**: Project headers (grouped view) or flat agent names with project prefix; each agent row shows lifecycle glyph (colored), name, note preview, status/age metadata
- **Detail panel**: `renderDetailPanel` shows either the preview detail (`renderDetail`) or the plan checklist (`renderChecklist`, when `showChecklist` is set). The preview detail shows the selected agent's metadata plus its available preview, status, subagents, and task details. Interactive previews are clipped to the available panel space.
- **Plan checklist**: Items come from `collect.Agent.Checklist` (OmO todo lists); the header shows done/total, phase labels appear only with more than one phase, and a plan taller than the panel first folds finished items above, then the tail. The "Background tasks" section lists `Subagents` that are working and stays pinned to the bottom unless fewer than two body rows would remain.
- **Help popup**: Scrollable keyboard reference (`keyboardHelpEntries`, 20 entries); fits height constraints by truncating or scrolling.
- **Footer**: Context-sensitive controls or input prompts, with a notice, `Codex: N pending` while note workers run, or last-refresh timestamp.
- **Terminal constraints**: Renders a too-small message below 20 columns or 5 rows; an open help popup requires at least 6 rows.

#### Keyboard & Interaction Semantics

**Movement** (j/k/↑/↓, g/G/Home/End): Move selection within visible rows, skip headers; g/Home jumps to the first agent and G/End to the last. J/K navigate between project groups in grouped view only; wrapped selection stays at list edge.

**Backend tab** (Tab / Shift+Tab): Switches between the tmux and Herdr backends (`switchBackend`). It cancels pending auto-titles, clears the snapshot and all selection, filter, input, and popup state, bumps `generation`, loads that backend's notes store, and triggers a fresh refresh. The selection persists to `preferences.json`.

**Selection** (enter): For interactive panes (tmux or Herdr), opens the move menu (see below) and exits only after a choice. For live background Claude agents, immediately returns a `manage` action (open the Claude agents manager). Other agents show a "no attach or resume action" notice.

**Move menu** (after enter on an interactive agent): `f` fullscreen, `p` popup (labelled "Project popup (window tabs)" on tmux, which also captures the project's other tmux panes as `ProjectTargets`), `j`/`k`/↑/↓/Tab/Shift+Tab toggle the choice, `enter` opens it, `esc` or `q` cancels, `ctrl+c` quits amux. The chosen `Action` carries `Popup`; `Model.Resume` clears the action when returning from a popup.

**Compose** (s): Only for interactive panes. Opens input mode; Enter submits the text to the selected pane (tmux `send-keys -l` plus Enter, or Herdr `pane send-text` plus `send-keys enter`), and Esc cancels without sending. Blank text is not sent.

**Launch** (c): Opens the launch popup for starting an agent in a project directory. Step 1 is the path: Up/Down/Tab/Shift+Tab cycle history entries and a final blank "New directory..." entry; Left/Right/Home/End/Ctrl+A/Ctrl+E move the cursor; Backspace/Ctrl+H/Delete/Ctrl+U edit; Enter validates the directory (`~` expands, relative paths resolve against the launch directory), records it in history, and moves to step 2. Step 2 picks the tool (`claude`, `codex`, `omo`, `omp`, `opencode`) with Up/Down/j/k/Tab/Shift+Tab, Enter launches, and Esc/Ctrl+C steps back or closes. Candidate paths merge the selected agent's CWD/project, saved history, snapshot project paths, and the launch directory. tmux opens a new window in the selected pane's owning session (it refuses amux's own `amux-popup*` session when only `TMUX_PANE` identifies it); Herdr needs an interactive agent with an explicit named session and opens an unfocused tab in its workspace.

**Kill** (x): Only for interactive panes (backgrounds are dismissed, not killed). Arms confirmation. y/Y kills (tmux `kill-pane` or Herdr `pane close`); any other key cancels. Kills are irreversible; Model removes the agent from display immediately on success (doesn't wait for next refresh).

**Edit** (i): Opens the edit popup for the selected agent: `t` edits the window title and `i` edits the note; `esc` cancels. `t` requires an interactive pane (otherwise the popup shows "Titles require an interactive pane").

**Window title picker** (edit popup, `t`): The input starts as the current window name. Up/Down choose a preset, Tab completes to the closest preset, typing narrows by `closestTitle` (exact, prefix, substring, then edit distance), Ctrl+N saves the current input as a new preset, Ctrl+D deletes the highlighted preset, Ctrl+U clears, Enter saves a non-empty single-line title (tmux `rename-window` with `#` escaped, or Herdr tab/pane rename), Esc cancels. Presets live in `amux/window-titles.json`; if the file cannot be parsed or validated, the error is shown and add/delete are disabled.

**Note** (edit popup, `i`): Opens input mode for editing or creating a note on the selected agent (Enter saves, Ctrl+U clears, Esc cancels). A first note on an interactive pane whose pane-title suggestion is at most 20 runes (including none) first asks Codex for a purpose title (`fetchPurposeTitle`; Esc cancels, `q`/Ctrl+C quits), and falls back to the suggestion with a "Codex failed" error. A longer suggestion shows a prompt: `y`/Enter prefills it, `n` opens a blank editor, `o` asks Codex, Esc cancels. The suggestion strips the leading run of symbols and punctuation, covering omp's `π:` and `π >` prefixes and Claude Code's spinner or star glyph; omo strips the `OmO -` prefix; Codex uses the trimmed pane title as supplied. Other tools and backgrounds offer no suggestion.

**Clear note** (I): Cancels pending Codex work for the agent, clears its note, and sets an interactive pane's window title to `stay`.

**Codex note menu** (o): Opens the Codex note popup and immediately renames the selected interactive pane's window to `start`. Inside: `o` generates a note, `m` lists Codex models (`j`/`k`/↑/↓ move, `r` reload, Enter saves the choice via `notegen.SelectModel`), `esc` closes, `q`/Ctrl+C quits. Generation (`fetchAutoTitle`) runs a detached `notegen` worker that survives quitting amux; the result is applied when the worker exits. `esc` in the main view cancels pending generation for the selected agent.

**Filter** (/): Substring search across agent name, project, tool name, and saved note. Matching is case-insensitive. Filtering never removes agents from the snapshot (only hides them from display rows). Empty filter shows all; selection is preserved when the selected agent remains visible.

**Acknowledge** (a): Marks an error-state agent as acknowledged; acknowledged errors no longer drive project attention rank and are included only when history is shown.

**Views**:
- **d** (done): Shows or hides completed, stopped, and acknowledged-error history.
- **p** (project): Toggles flat (single prefixed list) vs. grouped (project sections with headers)
- **v** (vertical): Toggles stacked (list above detail) vs. side-by-side layout
- **m** (manage/mute detail): Shows or hides the detail panel so the list fills the viewport
- **l** (plan checklist): Shows the OmO plan checklist in the detail slot. `m` and `l` share the slot: the key for the content already showing hides the panel, and the other key swaps the content without hiding it.
- **?** (help): Opens the scrollable keyboard help; `j`/`k`/↑/↓ scroll, `?` or `esc` closes.
- **r** (refresh): Runs a refresh now. **q** / Ctrl+C: quits after cancelling pending auto-titles and flushing preferences.

#### Ordering

`StatusChangedAt` is the primary sort key in both views, so an agent goes to the top the moment its lifecycle changes and stays there until another agent changes. These rules keep that promise:

- **Activity never competes with it.** `ActivityAt` moves on nearly every refresh of a busy pane, so it only breaks ties between agents that last changed state at the same instant — which is the common case, since one refresh stamps every transition with its own `Taken` time. Ranking the two together is what buried real transitions in ordinary churn.
- **The startup cohort holds its remembered positions.** Every agent present when amux launches is stamped with the same `Taken` time, so the primary key cannot separate them and the list would otherwise be ordered by whichever weak tie-breaker happened to apply — then visibly resort once the next snapshot arrived. `rememberedRank` (restored from `preferences.json`) settles them instead, gated on `Recency().Equal(initialRecencyAt)` so it governs the startup cohort and nothing else: agents that transition later move by `Recency`, and two agents transitioning in the same refresh tie on a *different* time and fall through to activity as documented above. Look ranks up with the two-value form — a missing agent reads as position zero and would sort above everything the user actually placed.
- **Recency is `collect.Agent.Recency()`: the dashboard's own observation whenever it has one, the state file's timestamp only before that.** `UpdatedAt` is a background state file's *write* timestamp — a working job rewrites it as its token count climbs — so ranking on it would reintroduce the same churn problem the previous rule solves. Interactive agents never carry `UpdatedAt` at all, so both fields are needed for the two kinds to sort against each other; ongoing background progress still breaks ties at the activity tier. This applies to project headers (`visibleProjectLatest`) as much as to rows; keying headers on `UpdatedAt` alone tied every tmux-only project at the zero time and silently fell back to alphabetical order.
- **Attention rank stays a tiebreaker in the flat view, and the leading key for grouped project headers.** Grouped view is attention-first by design; the flat view is explicitly a recency list.
- **`initialCPU` freezes from the first snapshot that has agents**, not the second. It only orders startup-cohort agents with no remembered position, and seeding it a refresh late was itself a visible resort.

`collect.group` applies the same ordering when it builds the snapshot, but `rebuildRows` re-sorts everything the user actually sees — filtering, dismissals, and acknowledgements change which agents a project is ranked on.

The selected backend and the `p`, `v`, `m`, and `l` view preferences persist to `preferences.json` across sessions; `d` is session-local.

`preferences.json` also stores `flat_order` and `grouped_order` (tmux) and `herdr_flat_order` and `herdr_grouped_order` (Herdr): the agent IDs each view last displayed, in order. `p` switches between two genuinely different layouts, so each is remembered apart and restored on entry. `rebuildRows` records the order it just built — except while a filter is active, since a filtered list would evict every agent it hides. `persistViewPreferences` skips writes that would not change the file, which matters because the two-second refresh calls it; the quit and attach paths flush explicitly, so only a hard kill loses the last order. Concurrent amux instances share one file, last writer wins.

#### Rendering Constraints & Resilience

- **Narrow terminals**: Applies width-aware clipping where individual rendering paths require it, preserving ANSI color codes and adding ellipses when truncated.
- **Small viewports**: Renders a too-small message below 20 columns or 5 rows; an open help popup requires at least 6 rows.
- **Incomplete data**: A snapshot can omit an interactive pane preview; detail rendering falls back to other available agent detail fields.
- **Multi-byte text**: Text-input helpers operate on runes, and display helpers account for rendered widths.
- **ANSI codes**: Pane preview preserves the agent's own terminal colors (from `capture-pane -e`). Closes with `\x1b[0m` reset so colors never bleed into amux UI.

### Testing Requirements

- Unit tests use Go's standard `testing` package. `app_test.go` covers the core Model, key handling, ordering, and rendering; each feature file has a matching `_test.go` (`backend`, `backend_actions`, `launch_menu`, `launch_history`, `move_menu`, `edit_menu`, `title_picker`, `title_presets`, `note_menu`, `auto_title`, `purpose_title`, `checklist`, `startup`, `subagents`, `preferences`).
- Herdr behavior is tested without a live Herdr: `backend_fixture_test.go` installs a fake `herdr` executable on `PATH` that records pane input in private files, and `launch_history_process_test.go` exercises concurrent history writers across processes.
- `performance_test.go` holds benchmarks (`BenchmarkDashboardNavigation`, `BenchmarkDashboardSnapshot`, `BenchmarkTitleCompletion`); run them with `go test -bench . ./internal/ui`.
- Run `go test ./internal/ui` while iterating and `go test ./...` before completion; manually exercise changed keyboard or rendering paths in a tmux terminal when practical.

### Common Patterns

1. **State-based rendering**: `View` derives dashboard content from `Model` state, while time-based age labels are computed at render time from `collect.Agent.Age`, which deliberately inverts `Recency()`'s preference: the state file records when a background agent's state actually changed, while `StatusChangedAt` only records when amux first saw it and would report a two-hour-old finished job as seconds old right after launch.
2. **Deferred backend actions**: Update returns `tea.Cmd` values for pane send, kill, rename, launch, and refresh operations (tmux or Herdr); each stamps its result with the originating backend and `generation`, and `acceptBackend` drops results from a previous tab. Note and preference writes remain explicit synchronous state-update work.
3. **Message-driven state**: External events (keyboard, refresh timer) are wrapped in message types and handled sequentially in Update.
4. **Filter applied to display only**: The Snapshot is kept complete; filtering (rebuildRows) only removes rows from display, never from the underlying data, so operations like note pruning see the full agent set.
5. **Acknowledgement/dismissal locality**: Acknowledgement and dismissal maps are session-local and pruned as snapshots change, so stale entries do not persist.
6. **Preference persistence**: The backend tab, grouped/flat, stacked, detail-panel, and checklist view preferences are saved when their toggles change.
7. **Modal popups**: Launch, edit (title picker and note entry), move, and Codex note menus each keep their state in a dedicated field and are routed first by `handleKey` and `View`; a snapshot that no longer contains the target agent resets the note and edit popups, and a backend switch clears all popup state.
8. **Detached note workers**: Codex note generation runs in a `notegen` process with a `notes.Store` generation token (`Begin`/`Complete`/`Cancel`), so it can outlive amux; a `noteLauncher` field lets tests substitute the launcher.


## Dependencies

### Internal
- `internal/collect` provides `Snapshot`, `Project`, and `Agent` types; Model consumes one Snapshot per refresh.
- `internal/notes` provides `Store` for get/set/reload/prune operations and generation tracking (`Begin`, `Complete`, `Cancel`, `Generation`); `notes.Load` and `notes.LoadHerdr` select the tmux or Herdr namespace. Model calls Reload every 10s to see notes saved by other amux instances.
- `internal/herdr` provides `Run`, a bounded wrapper over the installed `herdr` CLI, used for Herdr pane send/close/read, tab and pane rename, tab/workspace creation, pane run, and `session list`.
- `internal/notegen` provides Codex integration for notes: `Title` (purpose-title inference), `Start` (detached worker), `Models`, `SelectModel`, `SelectedModel`, and the `Request` and `Model` types.

### External
- **Bubble Tea** (`github.com/charmbracelet/bubbletea`) provides the event-driven state machine: Model struct, Init/Update/View contract, message types, commands, and the runtime loop.
- **Lip Gloss** (`github.com/charmbracelet/lipgloss`) provides styled rendering: colors, bold, italic, width/height constraints, pane fitting, and ANSI output.
- **Reflow** (`github.com/muesli/reflow`) provides text utilities: ANSI-aware truncation (truncate.StringWithTail) and word wrapping (wordwrap.String).
- Go standard library: `time`, `strings`, `sort`, `unicode`, `os/exec`, `os`, `path/filepath`, `encoding/json`, `context`, `syscall` (launch-history file lock).
- External CLIs invoked at runtime: `tmux`, `herdr`, and (through `notegen`) Codex.

Content below this line is hand-maintained and preserved on regeneration.

## Manual Notes

Notes under this heading are written by humans and preserved on regeneration.
