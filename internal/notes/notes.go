// Package notes stores short, user-authored labels for agents — a place to
// jot down why a given run exists ("왜 이 작업을 하고 있는지"). Notes live only
// on disk locally and are never written back to tmux, Claude, or omp; they're
// visible only inside amux.
package notes

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// Store persists agent notes as a flat agent-ID -> text map. Notes are
// intentionally scoped to an agent's current run: an ID (tmux pane ID or
// session ID) doesn't survive past that run ending, so Prune drops notes for
// runs that are no longer present rather than accumulating forever.
type Store struct {
	path     string
	m        map[string]string
	requests map[string]Generation
}

func defaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "amux", "notes.json"), nil
}

// Load reads the notes file, starting empty if it doesn't exist yet. A
// resolution failure degrades to an in-memory-only store rather than an
// error — notes are a convenience layer, never worth failing startup over.
func Load() *Store {
	s := &Store{m: map[string]string{}}
	path, err := defaultPath()
	if err != nil {
		return s
	}
	s.path = path
	if reloaded, ok := readNotes(path); ok {
		s.m = reloaded
	}
	return s
}

// Reload replaces the in-memory notes with the persisted notes when the file
// is readable and valid. It lets concurrently running dashboards observe notes
// saved by each other without losing their current state on a transient read.
func (s *Store) Reload() {
	if s == nil || s.path == "" {
		return
	}
	if reloaded, ok := readNotes(s.path); ok {
		s.m = reloaded
	}
}

// Get returns the note for id, or "" if there isn't one. Safe to call on a nil
// *Store (e.g. a zero-value Model in tests) — behaves as an empty store.
func (s *Store) Get(id string) string {
	if s == nil {
		return ""
	}
	return s.m[id]
}

// Set stores the note for id (or clears it, if note is empty) and persists.
// A no-op on a nil *Store.
func (s *Store) Set(id, note string) {
	if s == nil {
		return
	}
	s.update(func(stored map[string]string, requests map[string]Generation) {
		delete(requests, id)
		if note == "" {
			delete(stored, id)
		} else {
			stored[id] = note
		}
	})
}

// Prune drops notes for agent IDs no longer present in liveIDs, so notes.json
// doesn't accumulate labels for runs that ended long ago. A no-op on a nil
// *Store.
func (s *Store) Prune(liveIDs map[string]bool) {
	if s == nil {
		return
	}
	s.update(func(stored map[string]string, requests map[string]Generation) {
		for id := range requests {
			if !liveIDs[id] {
				delete(requests, id)
			}
		}
		for id := range stored {
			if !liveIDs[id] {
				delete(stored, id)
			}
		}
	})
}

// update performs a disk-backed read-modify-write operation under an advisory
// lock. Each dashboard therefore applies its mutation to the latest persisted
// map rather than overwriting notes written since its last Reload.
func (s *Store) update(mutate func(map[string]string, map[string]Generation)) error {
	if s.m == nil {
		s.m = map[string]string{}
	}
	if s.path == "" {
		if s.requests == nil {
			s.requests = map[string]Generation{}
		}
		mutate(s.m, s.requests)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}

	lock, err := os.OpenFile(s.path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	stored, ok := readNotes(s.path)
	if !ok {
		return fmt.Errorf("cannot read notes")
	}
	requests, err := readGenerations(s.path)
	if err != nil {
		return err
	}
	mutate(stored, requests)
	// Invalidation is committed before note changes: a failed write can never
	// leave an older generation authorized to overwrite manual input.
	if !writeNotes(s.path+".requests", requests) {
		return fmt.Errorf("cannot save note requests")
	}
	if !writeNotes(s.path, stored) {
		return fmt.Errorf("cannot save notes")
	}
	s.m = stored
	s.requests = requests
	return nil
}

// readNotes reads a complete persisted map. A missing file is an empty store;
// unreadable, malformed, and null data are rejected without altering callers.
func readNotes(path string) (map[string]string, bool) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, true
	}
	if err != nil {
		return nil, false
	}
	stored := map[string]string{}
	if err := json.Unmarshal(data, &stored); err != nil || stored == nil {
		return nil, false
	}
	return stored, true
}

// writeNotes atomically replaces the notes file, so readers see either the old
// complete map or the new complete map, never a partially written JSON file.
func writeNotes(path string, stored any) bool {
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return false
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".notes-*.tmp")
	if err != nil {
		return false
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return false
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return false
	}
	if err := file.Close(); err != nil {
		return false
	}
	return os.Rename(tempPath, path) == nil
}
