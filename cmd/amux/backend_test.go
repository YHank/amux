package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kyhan/amux/internal/collect"
	"github.com/spf13/cobra"
)

func TestBackendFlagOverridesSavedSelection(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	dir = filepath.Join(dir, "amux")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "preferences.json"), []byte(`{"backend":"herdr"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		flag string
		want collect.Backend
	}{
		{"", collect.BackendHerdr},
		{"tmux", collect.BackendTmux},
	} {
		t.Run(tc.flag, func(t *testing.T) {
			cmd := &cobra.Command{}
			addBackendFlag(cmd)
			if err := cmd.Flags().Set("backend", tc.flag); err != nil {
				t.Fatal(err)
			}
			got, err := commandBackend(cmd)
			if err != nil || got != tc.want {
				t.Fatalf("backend = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestStatusRejectsUnknownBackend(t *testing.T) {
	cmd := statusCmd()
	cmd.SetArgs([]string{"--backend", "unknown"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "unknown backend") {
		t.Fatalf("invalid backend error = %v", err)
	}
}
