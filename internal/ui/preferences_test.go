package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestViewPreferencesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "amux", "preferences.json")
	want := viewPreferences{
		Stacked: true, HideDetail: true, FlatView: true,
		FlatOrder:    []string{"%1", "%2"},
		GroupedOrder: []string{"%2", "%1"},
	}

	saveViewPreferences(path, want)
	if got := loadViewPreferences(path); !got.equal(want) {
		t.Fatalf("loadViewPreferences() = %#v, want %#v", got, want)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var persisted struct {
		Stacked      bool     `json:"stacked"`
		HideDetail   bool     `json:"hide_detail"`
		FlatView     bool     `json:"flat_view"`
		FlatOrder    []string `json:"flat_order"`
		GroupedOrder []string `json:"grouped_order"`
	}
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if !persisted.Stacked || !persisted.HideDetail || !persisted.FlatView {
		t.Fatalf("persisted JSON = %#v, want every layout toggle set to true", persisted)
	}
	if !slices.Equal(persisted.FlatOrder, want.FlatOrder) ||
		!slices.Equal(persisted.GroupedOrder, want.GroupedOrder) {
		t.Fatalf("persisted order = %#v / %#v, want %#v / %#v",
			persisted.FlatOrder, persisted.GroupedOrder, want.FlatOrder, want.GroupedOrder)
	}
}

// Skipping a write that would not change the file keeps the two-second refresh
// cycle from touching the disk while the list is steady.
func TestPersistViewPreferencesSkipsUnchangedWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	m := Model{preferencesPath: path, flatView: true, flatOrder: []string{"%1"}}

	m.persistViewPreferences()
	first, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}

	m.persistViewPreferences()
	again, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if !again.ModTime().Equal(first.ModTime()) {
		t.Error("an unchanged save rewrote the preferences file")
	}

	m.flatOrder = []string{"%1", "%2"}
	m.persistViewPreferences()
	if got := loadViewPreferences(path); !slices.Equal(got.FlatOrder, []string{"%1", "%2"}) {
		t.Errorf("changed order was not written: %#v", got.FlatOrder)
	}
}

func TestViewPreferencesDefaultsForMissingOrMalformedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	if got := loadViewPreferences(path); !got.equal(viewPreferences{}) {
		t.Fatalf("missing preferences = %#v, want zero value", got)
	}

	if err := os.WriteFile(path, []byte(`{"stacked":true,"hide_detail":true}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if got := loadViewPreferences(path); !got.equal(viewPreferences{Stacked: true, HideDetail: true}) {
		t.Fatalf("legacy preferences = %#v, want grouped view default", got)
	}

	if err := os.WriteFile(path, []byte("not JSON"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if got := loadViewPreferences(path); !got.equal(viewPreferences{}) {
		t.Fatalf("malformed preferences = %#v, want zero value", got)
	}
}

func TestNewRestoresViewPreferences(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	path := defaultPreferencesPath()
	if path == "" {
		t.Fatal("defaultPreferencesPath() returned an empty path")
	}
	want := viewPreferences{Stacked: true, HideDetail: true, FlatView: true}
	saveViewPreferences(path, want)

	m := New()
	if m.preferencesPath != path {
		t.Fatalf("New() preferencesPath = %q, want %q", m.preferencesPath, path)
	}
	if m.stacked != want.Stacked || m.hideDetail != want.HideDetail || m.flatView != want.FlatView {
		t.Fatalf("New() view preferences = stacked:%v hideDetail:%v flatView:%v, want %#v", m.stacked, m.hideDetail, m.flatView, want)
	}
}
