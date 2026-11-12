package notes

import (
	"os"
	"testing"
)

func TestLoadHerdrImportsLegacyNotesWithoutChangingLegacyStore(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	legacy := Load()
	legacy.Set("%54", "tmux note")
	legacy.Set("herdr:default:w9:p2", "existing Herdr note")
	token, err := legacy.Begin("herdr:default:w9:p2")
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(legacy.Path())
	if err != nil {
		t.Fatal(err)
	}

	store := LoadHerdr()

	if got := store.Get("herdr:default:w9:p2"); got != "existing Herdr note" {
		t.Fatalf("imported Herdr note=%q", got)
	}
	if got := store.Get("%54"); got != "" {
		t.Fatalf("imported unrelated tmux note=%q", got)
	}
	after, err := os.ReadFile(legacy.Path())
	if err != nil || string(before) != string(after) {
		t.Fatalf("import changed legacy notes: err=%v", err)
	}
	if legacy.Generation("herdr:default:w9:p2").Token != token {
		t.Fatal("import invalidated a legacy worker")
	}
}

func TestLoadHerdrDoesNotReimportStaleLegacyNotes(t *testing.T) {
	for _, replacement := range []string{"new note", ""} {
		t.Run(replacement, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("XDG_CONFIG_HOME", "")
			legacy := Load()
			id := "herdr:default:w9:p2"
			legacy.Set(id, "old note")
			store := LoadHerdr()
			store.Set(id, replacement)

			if got := LoadHerdr().Get(id); got != replacement {
				t.Fatalf("reopen restored stale legacy note: got=%q want=%q", got, replacement)
			}
		})
	}
}
