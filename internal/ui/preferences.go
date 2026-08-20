package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
)

type viewPreferences struct {
	Stacked    bool `json:"stacked"`
	HideDetail bool `json:"hide_detail"`
	FlatView   bool `json:"flat_view"`
	// Agent IDs in the order each view last displayed them. Restoring these
	// lets a fresh dashboard open in the layout the user left rather than the
	// one it settles into once a second snapshot arrives. Both views are kept
	// separately because `p` switches between two genuinely different
	// orderings, and the user expects each to come back as they left it.
	FlatOrder    []string `json:"flat_order,omitempty"`
	GroupedOrder []string `json:"grouped_order,omitempty"`
}

// equal reports whether two preference sets would serialize identically, so a
// save that would not change the file can be skipped. viewPreferences carries
// slices and is therefore not comparable with ==.
func (p viewPreferences) equal(other viewPreferences) bool {
	return p.Stacked == other.Stacked &&
		p.HideDetail == other.HideDetail &&
		p.FlatView == other.FlatView &&
		slices.Equal(p.FlatOrder, other.FlatOrder) &&
		slices.Equal(p.GroupedOrder, other.GroupedOrder)
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

// saveViewPreferences overwrites the file wholesale. Two amux instances
// sharing one config directory therefore race, last writer wins — the same
// contract the layout toggles have always had.
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
