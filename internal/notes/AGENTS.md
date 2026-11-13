# notes

**Parent context:** `../AGENTS.md`
**Generated:** 2026-07-23 · **Updated:** 2026-11-13

## Purpose
Persists short per-agent notes in application configuration JSON files, tracks durable AI note-generation requests (token, done, error) next to them, and keeps Herdr notes in a store separate from tmux notes. Notes are visible only inside amux.

## Key Files

| File | Description |
|------|-------------|
| `notes.go` | Defines `Store` and the notes-file persistence: `Load()`, `Store.{Get,Set,Reload,Prune}`, the locked read-modify-write `update()`, and the `readNotes`/`writeNotes` helpers. |
| `notes_test.go` | Unit tests for set/get persistence, clearing, pruning, nil-store safety, `Load()` round trip, `Reload()` behavior, and merging writes from separate dashboards. |
| `generation.go` | Defines `Generation{Token, Done, Error}` and the AI note-generation request API: `Open(path)`, `Store.{Path,Begin,Cancel,Complete,Generation}`, and `readGenerations()` for the `.requests` sidecar. |
| `generation_test.go` | Tests that a stale `Complete` is rejected after a newest request, manual `Set`, same-text `Set`, clear, `Cancel`, or `Prune` from another process, and that concurrent completions for different agents are both kept. |
| `herdr.go` | Defines `LoadHerdr()`, which opens the separate `herdr-notes.json` store and imports `herdr:` notes from the legacy file once. |
| `herdr_test.go` | Tests that `LoadHerdr()` imports only `herdr:` notes without changing the legacy store or its requests, and does not re-import stale legacy notes after reopening. |

## Subdirectories

| Directory | Purpose |
|-----------|---------|
| _None_ | This package has no subdirectories. |

## For AI Agents

### Working In This Directory
#### Storage Lifecycle
- `Load()` resolves the tmux notes file as `notes.json` under `os.UserConfigDir()/amux/` (for example `$XDG_CONFIG_HOME/amux/notes.json` or `~/.config/amux/notes.json` on Linux, `~/Library/Application Support/amux/notes.json` on macOS).
- `Load()` returns a `*Store` backed by an in-memory map; a missing file is treated as empty. If the config directory cannot be resolved, the store has an empty `path` and stays in memory only. If the file exists but cannot be read or parsed, `Load()` keeps an empty map (never fatal).
- Each note is keyed by an agent ID (tmux pane ID, session ID, or a `herdr:`-prefixed ID).
- `Prune(liveIDs)` removes both notes and generation requests for IDs absent from `liveIDs`.

#### Herdr Notes
- `LoadHerdr()` opens `herdr-notes.json` in the same `amux` config directory as `notes.json` (via `Open`). If the config directory cannot be resolved, it returns an empty in-memory store.
- The separate file exists because older tmux dashboards call a global `Prune` that removes every ID outside their tmux inventory.
- When `herdr-notes.json` does not exist yet, `LoadHerdr()` imports only entries whose ID starts with `herdr:` from the legacy `notes.json`. The import runs inside `update()` under the destination lock and re-checks that the file is still absent, so reopening never resurrects a cleared note or overwrites a newer one.
- The legacy `notes.json` and its `.requests` file are never modified by the import, so workers already running against the legacy store stay valid.
- `internal/ui` calls `notes.LoadHerdr()` when the Herdr backend is selected and `notes.Load()` otherwise.

#### Generation (AI Note) Requests
- A `Generation` is `{"token": string, "done": bool (omitted when false), "error": string (omitted when empty)}`, stored per agent ID.
- `Begin(id)` creates a random token with `rand.Text()`, stores `Generation{Token: token}` for the ID, and returns the token. It returns an error on a nil store. Callers reserve the request before spawning the asynchronous worker.
- `Complete(id, token, title, generationErr)` runs under the same lock as all note writes. It applies only if a request exists for `id`, its token matches, and it is not already `Done`. It then marks the request `Done`; with a non-nil `generationErr` it records `Error` and leaves the note untouched, otherwise it stores `title` as the note. It returns whether the title was applied.
- `Cancel(id)` deletes the request. `Set(id, ...)` (any value, including an identical or empty one) also deletes the request, so a manual edit always invalidates an in-flight generation, including the same-text ("ABA") case.
- `Generation(id)` returns the stored request, or the zero value on a nil store or read error. For a store with an empty `path` it reads the in-memory requests.
- `Open(path)` builds a store for an explicit path and calls `Reload()`; `internal/notegen` uses it in the detached worker with the `StorePath` it inherited, and `Path()` supplies that path (empty on a nil store).

#### JSON/Config Path Behavior
- The notes file is a flat JSON object: `{"agent-id-1": "note text", "agent-id-2": "note text"}`.
- Requests live in a sidecar `<notes path>.requests` (for example `notes.json.requests`): a JSON object of agent ID to `Generation`. A missing sidecar is an empty set; unreadable, malformed, or `null` content makes `update()` fail.
- Both files are written by `writeNotes` as indented JSON (`"  "`), through a temp file named `.notes-*.tmp` in the same directory, `chmod 0o600`, then `os.Rename`, so readers never see partial JSON.
- `update()` creates the config directory on demand with `os.MkdirAll(..., 0o755)`.
- An empty note string (`""`) deletes the key from the notes file instead of storing an empty value.

#### Concurrency & Error Guarantees
- **Locking**: `update()` opens `<notes path>.lock` and takes `syscall.Flock(..., LOCK_EX)` before the read-modify-write, so multiple dashboards and detached workers coordinate without overwriting each other. The lock is held until `update()` returns.
- **Write order**: `update()` reads both files, applies the mutation, writes `.requests` first, then the notes file. A failed write can therefore never leave an older generation authorized to overwrite manual input.
- **Errors**: `update()` returns errors for directory creation, lock open/flock, unreadable notes (`cannot read notes`), unreadable requests, and write failures (`cannot save note requests`, `cannot save notes`). `Set` and `Prune` ignore that error; `Begin`, `Cancel`, and `Complete` return it. A failed update leaves the in-memory maps unchanged.
- **In-memory-only stores**: when `path` is empty, `update()` mutates the in-memory notes and requests maps directly with no locking or disk I/O.
- **Reload semantics**: `Reload()` replaces the in-memory map when it can read valid persisted JSON, or when the notes file is absent and therefore represents an empty store. Invalid JSON, `null`, or other read errors leave the current in-memory map unchanged. `Reload()` does not touch the requests; `Generation()` always reads the sidecar from disk.

### Testing Requirements
- Run `go test ./internal/notes` while iterating and `go test ./...` before completion.
- Tests cover persistence, clearing, pruning, nil-store safety, `Reload()` behavior, cross-process generation invalidation, concurrent completion merging, and Herdr legacy import.
- `generation_test.go` re-executes the test binary as a child process (`TestGenerationMutationProcess`, driven by the `AMUX_STORE` and `AMUX_MUTATION` environment variables) to simulate a separate dashboard.

### Common Patterns
- `Store` holds a file path, an in-memory notes map, and an in-memory requests map (used only when `path` is empty). Every public method tolerates a nil `*Store` (returning zero values, an error from `Begin`, or doing nothing) except `Complete`, which has no nil check.
- `Get()` reads directly from the in-memory map; `Set()`, `Prune()`, `Begin()`, `Cancel()`, and `Complete()` persist through `update()` mutation closures that receive the latest notes and requests maps.
- Test isolated `Store` persistence with paths under `t.TempDir()`; cover real `Load()`/`LoadHerdr()` path resolution by setting `HOME` to a temp directory and clearing `XDG_CONFIG_HOME`.

## Dependencies

### Internal
- Called by `internal/ui`: `Load`, `LoadHerdr`, `Get`, `Set`, `Reload`, `Path`, `Begin`, `Complete`, `Cancel`, and `Generation`. The UI prunes stale entries by calling `Set(id, "")` for IDs missing from the live inventory (`pruneBackendNotes` in `internal/ui/backend.go`).
- Called by `internal/notegen` (`Work` in `worker.go`): `Open`, `Generation`, and `Complete`.

### External
- Uses the Go standard library only: `crypto/rand` (request tokens), `encoding/json`, `errors`, `fmt`, `os`, `path/filepath`, `strings`, and `syscall` (Flock advisory locking).

## Manual Notes

Notes under this heading are written by humans and preserved on regeneration.
