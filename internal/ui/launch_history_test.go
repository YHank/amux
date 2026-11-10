package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"github.com/kyhan/amux/internal/collect"
)

func TestLaunchHistoryRetainsAllPreviouslySelectedPaths(t *testing.T) {
	path := filepath.Join(t.TempDir(), "launch-history.json")
	previous := make([]string, 150)
	for i := range previous {
		previous[i] = "/project/" + strconv.Itoa(i)
	}
	data, err := json.Marshal(previous)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := saveLaunchPath(path, "/new-project"); err != nil {
		t.Fatal(err)
	}

	paths, err := loadLaunchHistory(path)
	want := append([]string{"/new-project"}, previous...)
	if err != nil || !reflect.DeepEqual(paths, want) {
		t.Fatalf("history lost previously selected paths: got %d, want %d; err=%v", len(paths), len(want), err)
	}
}

func TestLaunchDirectoryRejectsInvalidPaths(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"", file, filepath.Join(root, "missing")} {
		t.Run(path, func(t *testing.T) {
			if _, err := launchDirectory(path, root); err == nil {
				t.Fatalf("accepted invalid directory %q", path)
			}
		})
	}
}

func TestLaunchHistoryMergesDiskAndPersistsMRU(t *testing.T) {
	path := launchHistoryPath(filepath.Join(t.TempDir(), "preferences.json"))
	for _, dir := range []string{"/first", "/second", "/first/.", "/third"} {
		if err := saveLaunchPath(path, dir); err != nil {
			t.Fatal(err)
		}
	}
	paths, err := loadLaunchHistory(path)
	if err != nil || !reflect.DeepEqual(paths, []string{"/third", "/first", "/second"}) {
		t.Fatalf("reloaded history=%q err=%v", paths, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("history permissions: info=%v err=%v", info, err)
	}
}

func TestLaunchHistoryPreservesUnreadableData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "launch-history.json")
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := saveLaunchPath(path, "/new"); err == nil {
		t.Fatal("corrupt history must produce an error")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "{broken" {
		t.Fatalf("history overwritten: %q err=%v", data, err)
	}
}

func TestRememberSelectedLaunchProjectPersistsNormalSelection(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first")
	second := filepath.Join(root, "second")
	for _, path := range []string{first, second} {
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	prefs := filepath.Join(root, "preferences.json")
	path := launchHistoryPath(prefs)
	if err := saveLaunchPath(path, first); err != nil {
		t.Fatal(err)
	}
	a := collect.Agent{ID: "selected", Kind: collect.KindInteractive, Life: collect.LifeWorking,
		Project: second} // Project is the fallback when CWD is unavailable.
	m := Model{snap: projSnap(a), preferencesPath: prefs}
	m.rebuildRows()

	m.rememberSelectedLaunchProject()

	paths, err := loadLaunchHistory(path)
	if err != nil || !reflect.DeepEqual(paths, []string{second, first}) {
		t.Fatalf("normal selection not persisted: %q err=%v", paths, err)
	}
}

func TestLaunchHistoryDoesNotRewriteUnchangedSelection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "launch-history.json")
	if err := saveLaunchPath(path, "/selected"); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := saveLaunchPath(path, "/selected"); err != nil {
		t.Fatal(err)
	}

	after, err := os.Stat(path)
	if err != nil || !os.SameFile(before, after) {
		t.Fatalf("unchanged selection replaced history: %v", err)
	}
}
