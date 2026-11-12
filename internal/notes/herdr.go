package notes

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// LoadHerdr isolates Herdr notes from older tmux dashboards, whose global
// Prune removes every ID outside their tmux inventory. The legacy file stays
// untouched, including any requests held by already running workers.
func LoadHerdr() *Store {
	legacyPath, err := defaultPath()
	if err != nil {
		return &Store{m: map[string]string{}}
	}
	s := Open(filepath.Join(filepath.Dir(legacyPath), "herdr-notes.json"))
	if _, err := os.Stat(s.path); !errors.Is(err, os.ErrNotExist) {
		return s
	}
	legacy, ok := readNotes(legacyPath)
	if !ok {
		return s
	}
	// Import once, under the destination lock. Reopening must never resurrect
	// a cleared note or overwrite a newer note with an old shared-file copy.
	s.update(func(stored map[string]string, _ map[string]Generation) {
		if _, err := os.Stat(s.path); !errors.Is(err, os.ErrNotExist) {
			return
		}
		for id, note := range legacy {
			if strings.HasPrefix(id, "herdr:") {
				stored[id] = note
			}
		}
	})
	return s
}
