package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestViewPreferencesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "amux", "preferences.json")
	want := viewPreferences{Stacked: true, HideDetail: true, FlatView: true}

	saveViewPreferences(path, want)
	if got := loadViewPreferences(path); got != want {
		t.Fatalf("loadViewPreferences() = %#v, want %#v", got, want)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var persisted map[string]bool
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(persisted) != 3 || !persisted["stacked"] || !persisted["hide_detail"] || !persisted["flat_view"] {
		t.Fatalf("persisted JSON = %#v, want stacked, hide_detail, and flat_view set to true", persisted)
	}
}

func TestViewPreferencesDefaultsForMissingOrMalformedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	if got := loadViewPreferences(path); got != (viewPreferences{}) {
		t.Fatalf("missing preferences = %#v, want zero value", got)
	}

	if err := os.WriteFile(path, []byte(`{"stacked":true,"hide_detail":true}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if got := loadViewPreferences(path); got != (viewPreferences{Stacked: true, HideDetail: true}) {
		t.Fatalf("legacy preferences = %#v, want grouped view default", got)
	}

	if err := os.WriteFile(path, []byte("not JSON"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if got := loadViewPreferences(path); got != (viewPreferences{}) {
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
