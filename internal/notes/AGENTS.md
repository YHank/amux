<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-07-23 | Updated: 2026-08-20 -->

# notes

## Purpose
Persists short, user-authored annotations for tracked agents in an application configuration JSON file and prunes notes when the associated agent no longer exists.

## Key Files

| File | Description |
|------|-------------|
| `notes.go` | Loads, saves, updates, deletes, and prunes note records using the user configuration directory; exports `Load()`, `Store.{Get,Set,Reload,Prune}`. |
| `notes_test.go` | Unit tests for store persistence and lifecycle behavior. |

## Subdirectories

| Directory | Purpose |
|-----------|---------|
| _None_ | This package has no subdirectories. |

## For AI Agents

### Working In This Directory
#### Storage Lifecycle
- Notes are stored in `$XDG_CONFIG_HOME/amux/notes.json` (or `~/.config/amux/notes.json` on Linux, `~/Library/Application\ Support/amux/notes.json` on macOS). Resolved via `os.UserConfigDir()`.
- `Load()` returns a `*Store` backed by an in-memory map; a missing file is treated as empty. File read/parse failures degrade to an empty in-memory store (never fatal).
- Each note is keyed by an agent ID. `Prune()` removes entries not present in the current live-ID set, including notes left behind after a pane or agent disappears.

#### JSON/Config Path Behavior
- Storage is a flat JSON object: `{"agent-id-1": "note text", "agent-id-2": "note text"}`.
- JSON is marshaled with indent (`"  "` formatting) and file permissions are `0o600` (readable/writable by owner only).
- Atomic writes via temp-file + rename: ensures readers never see partial JSON.
- Missing config directory is created on-demand with `os.MkdirAll(..., 0o755)`.
- Empty note string (`""`) causes the key to be deleted from disk (not stored as null).

#### Concurrency & Error Guarantees
- **Locking**: `update()` acquires an advisory lock (`.lock` file via `syscall.Flock(…LOCK_EX)`) before read-modify-write, allowing multiple dashboards to coordinate safely without file corruption.
- **Lock scope**: Lock is held for the entire read-modify-write cycle (read persisted notes, apply mutation, write to disk). Released on exit.
- **Silent degradation**: I/O failures (config-directory resolution, lock, directory creation, read, write, or JSON decoding) are swallowed. `Load()` returns an empty in-memory store when it cannot resolve or read its file; failed updates and reloads preserve the current in-memory map.
- **Reload semantics**: `Reload()` replaces the in-memory map when it can read valid persisted JSON, or when the notes file is absent and therefore represents an empty store. Invalid JSON or null data leave the current in-memory map unchanged.

### Testing Requirements
- Run `go test ./internal/notes` while iterating and `go test ./...` before completion.
- Tests cover persistence, clearing, pruning, nil-store safety, and reload behavior.

### Common Patterns
- `Store` is a struct holding a file path and an in-memory map; zero-value `*Store` is safe (nil pointer checks in all public methods).
- `Get()` reads directly from the in-memory map; `Set()` and `Prune()` persist changes through `update()` mutation closures.
- `Prune()` is called periodically when fresh snapshots arrive to remove stale notes.
- Test isolated `Store` persistence behavior with paths under `t.TempDir()`; separately cover the real `Load()` configuration-path resolution.


## Dependencies

### Internal
- Called by `internal/ui` (via `ui.Model` which holds a `*notes.Store` and calls `Get`, `Set`, `Reload`, `Prune`).

### External
- Uses the Go standard library only: `encoding/json` (unmarshal), `errors` (ErrNotExist checks), `os` (file I/O), `path/filepath` (path construction), `syscall` (Flock advisory locking).

<!-- MANUAL: Any manually added notes below this line are preserved on regeneration -->
