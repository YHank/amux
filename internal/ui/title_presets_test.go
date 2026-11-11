package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadTitlePresetsMissingFileReturnsDefaults(t *testing.T) {
	// Given
	path := filepath.Join(t.TempDir(), "window-titles.json")

	// When
	got, err := loadTitlePresets(path)

	// Then
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want := []string{"start", "stay", "review", "commit", "gitedit", "end"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	got[0] = "mutated"
	if defaultTitlePresets[0] != "start" {
		t.Fatal("defaults must be copied, not shared")
	}
}

func TestSaveLoadTitlePresetsRoundtrip(t *testing.T) {
	cases := map[string]struct {
		in   []string
		want []string
	}{
		"custom": {[]string{"  alpha ", "Beta"}, []string{"alpha", "Beta"}},
		"empty":  {[]string{}, []string{}},
		"nil":    {nil, []string{}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			// Given
			path := filepath.Join(t.TempDir(), "nested", "window-titles.json")

			// When
			if err := saveTitlePresets(path, tc.in); err != nil {
				t.Fatalf("save: %v", err)
			}
			got, err := loadTitlePresets(path)

			// Then
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if got == nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
			entries, err := os.ReadDir(filepath.Dir(path))
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 {
				t.Fatalf("temp file left behind: %v", entries)
			}
		})
	}
}

func TestTitlePresetsRejectInvalidEntries(t *testing.T) {
	cases := map[string][]string{
		"empty":     {"a", "  "},
		"control":   {"a\nb"},
		"duplicate": {"Review", " review "},
	}
	for name, titles := range cases {
		t.Run(name, func(t *testing.T) {
			// Given
			dir := t.TempDir()
			savePath := filepath.Join(dir, "save.json")
			loadPath := filepath.Join(dir, "load.json")
			data, err := json.Marshal(titles)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(loadPath, data, 0o644); err != nil {
				t.Fatal(err)
			}

			// When / Then
			if err := saveTitlePresets(savePath, titles); err == nil {
				t.Fatal("save accepted invalid titles")
			}
			if _, err := os.Stat(savePath); !os.IsNotExist(err) {
				t.Fatalf("invalid save must not create file: %v", err)
			}
			if _, err := loadTitlePresets(loadPath); err == nil {
				t.Fatal("load accepted invalid titles")
			}
		})
	}
}

func TestLoadTitlePresetsRejectsMalformedFiles(t *testing.T) {
	for name, content := range map[string]string{
		"garbage": "{not json",
		"object":  `{"a":1}`,
		"null":    "null",
		"empty":   "",
	} {
		t.Run(name, func(t *testing.T) {
			// Given
			path := filepath.Join(t.TempDir(), "window-titles.json")
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}

			// When
			_, err := loadTitlePresets(path)

			// Then
			if err == nil {
				t.Fatal("expected error")
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != content {
				t.Fatal("load must not modify the file")
			}
		})
	}
}

func TestLoadTitlePresetsReadFailure(t *testing.T) {
	// Given a directory where the file should be
	path := filepath.Join(t.TempDir(), "window-titles.json")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}

	// When / Then
	if _, err := loadTitlePresets(path); err == nil {
		t.Fatal("expected read error, not defaults")
	}
}

func TestSaveTitlePresetsWriteFailures(t *testing.T) {
	t.Run("parent is a file", func(t *testing.T) {
		// Given
		dir := t.TempDir()
		parent := filepath.Join(dir, "file")
		if err := os.WriteFile(parent, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}

		// When / Then
		if err := saveTitlePresets(filepath.Join(parent, "window-titles.json"), []string{"a"}); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("rename onto directory cleans temp", func(t *testing.T) {
		// Given
		dir := t.TempDir()
		path := filepath.Join(dir, "window-titles.json")
		if err := os.MkdirAll(filepath.Join(path, "child"), 0o755); err != nil {
			t.Fatal(err)
		}

		// When
		err := saveTitlePresets(path, []string{"a"})

		// Then
		if err == nil {
			t.Fatal("expected error")
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 {
			t.Fatalf("temp file left behind: %v", entries)
		}
	})
}
