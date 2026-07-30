// Package notes stores short, user-authored labels for agents — a place to
// jot down why a given run exists ("왜 이 작업을 하고 있는지"). Notes live only
// on disk locally and are never written back to tmux, Claude, or omp; they're
// visible only inside amux.
package notes

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Store persists agent notes as a flat agent-ID -> text map. Notes are
// intentionally scoped to an agent's current run: an ID (tmux pane ID or
// session ID) doesn't survive past that run ending, so Prune drops notes for
// runs that are no longer present rather than accumulating forever.
type Store struct {
	path string
	m    map[string]string
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
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &s.m)
	}
	return s
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
	if note == "" {
		delete(s.m, id)
	} else {
		s.m[id] = note
	}
	s.save()
}

// Prune drops notes for agent IDs no longer present in liveIDs, so notes.json
// doesn't accumulate labels for runs that ended long ago. A no-op on a nil
// *Store.
func (s *Store) Prune(liveIDs map[string]bool) {
	if s == nil {
		return
	}
	changed := false
	for id := range s.m {
		if !liveIDs[id] {
			delete(s.m, id)
			changed = true
		}
	}
	if changed {
		s.save()
	}
}

// save writes the current map to disk. Errors are swallowed for the same
// reason resolution failures are: a note is a nice-to-have, never a reason to
// disrupt the dashboard.
func (s *Store) save() {
	if s.path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return
	}
	data, err := json.MarshalIndent(s.m, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(s.path, data, 0o600)
}
