package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type viewPreferences struct {
	Stacked    bool `json:"stacked"`
	HideDetail bool `json:"hide_detail"`
}

func defaultPreferencesPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "amux", "preferences.json")
}

func loadViewPreferences(path string) viewPreferences {
	if path == "" {
		return viewPreferences{}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return viewPreferences{}
	}

	var prefs viewPreferences
	if err := json.Unmarshal(data, &prefs); err != nil {
		return viewPreferences{}
	}
	return prefs
}

func saveViewPreferences(path string, prefs viewPreferences) {
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}

	data, err := json.MarshalIndent(prefs, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o600)
}
