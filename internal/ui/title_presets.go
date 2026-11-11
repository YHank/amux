package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

var defaultTitlePresets = []string{"start", "stay", "review", "commit", "gitedit", "end"}

// defaultTitlePresetsPath returns the preset file location, or "" when the
// user config directory cannot be determined.
func defaultTitlePresetsPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "amux", "window-titles.json")
}

// normalizeTitlePresets trims labels and rejects empty, control-character and
// case-insensitive duplicate entries. The result is never nil.
func normalizeTitlePresets(titles []string) ([]string, error) {
	out := make([]string, 0, len(titles))
	seen := make(map[string]struct{}, len(titles))
	for i, raw := range titles {
		title := strings.TrimSpace(raw)
		if title == "" {
			return nil, fmt.Errorf("title preset %d is empty", i+1)
		}
		if strings.IndexFunc(title, unicode.IsControl) >= 0 {
			return nil, fmt.Errorf("title preset %q contains control characters", title)
		}
		key := strings.ToLower(title)
		if _, dup := seen[key]; dup {
			return nil, fmt.Errorf("duplicate title preset %q", title)
		}
		seen[key] = struct{}{}
		out = append(out, title)
	}
	return out, nil
}

// loadTitlePresets reads presets from path. A missing file yields the
// defaults; an existing file (including an empty list) is authoritative.
func loadTitlePresets(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return append([]string(nil), defaultTitlePresets...), nil
		}
		return nil, fmt.Errorf("read title presets %s: %w", path, err)
	}
	var titles []string
	if err := json.Unmarshal(data, &titles); err != nil {
		return nil, fmt.Errorf("parse title presets %s: %w", path, err)
	}
	if titles == nil {
		return nil, fmt.Errorf("parse title presets %s: expected a JSON array", path)
	}
	out, err := normalizeTitlePresets(titles)
	if err != nil {
		return nil, fmt.Errorf("invalid title presets %s: %w", path, err)
	}
	return out, nil
}

// saveTitlePresets validates and atomically writes titles to path. An empty
// list is stored as [] so it is not confused with a missing file.
func saveTitlePresets(path string, titles []string) error {
	clean, err := normalizeTitlePresets(titles)
	if err != nil {
		return fmt.Errorf("invalid title presets: %w", err)
	}
	data, err := json.MarshalIndent(clean, "", "  ")
	if err != nil {
		return fmt.Errorf("encode title presets: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create title presets dir %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".window-titles-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp title presets file: %w", err)
	}
	tmpName := tmp.Name()
	fail := func(step string, err error) error {
		return fmt.Errorf("%s %s: %w", step, tmpName, errors.Join(err, tmp.Close(), os.Remove(tmpName)))
	}
	if _, err := tmp.Write(data); err != nil {
		return fail("write", err)
	}
	if err := tmp.Sync(); err != nil {
		return fail("sync", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmpName, errors.Join(err, os.Remove(tmpName)))
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace title presets %s: %w", path, errors.Join(err, os.Remove(tmpName)))
	}
	return nil
}
