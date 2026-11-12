package ui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kyhan/amux/internal/collect"
)

func TestHerdrStartupSelectsCallerInItsSession(t *testing.T) {
	for _, saved := range []collect.Backend{collect.BackendTmux, collect.BackendHerdr} {
		for _, caller := range []struct{ name, pane, active string }{
			{"pane", " w9:p2 ", ""},
			{"shortcut-popup", "", " w9:p2 "},
			{"shortcut-overrides-inherited-pane", "w9:p1", "w9:p2"},
		} {
			t.Run(string(saved)+"/"+caller.name, func(t *testing.T) {
				t.Setenv("HOME", t.TempDir())
				t.Setenv("XDG_CONFIG_HOME", "")
				t.Setenv("AMUX_START_PANE", "%unrelated")
				t.Setenv("TMUX_PANE", "%unrelated")
				t.Setenv("HERDR_PANE_ID", caller.pane)
				t.Setenv("HERDR_ACTIVE_PANE_ID", caller.active)
				t.Setenv("HERDR_SOCKET_PATH", "/caller/herdr.sock")
				bin := t.TempDir()
				t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
				script := `#!/bin/sh
if [ "$*" != "session list --json" ]; then exit 1; fi
printf '%s\n' '{"sessions":[{"name":"other","running":true,"socket_path":"/other/herdr.sock"},{"name":"caller","running":true,"socket_path":"/caller/herdr.sock"}]}'
`
				if err := os.WriteFile(filepath.Join(bin, "herdr"), []byte(script), 0755); err != nil {
					t.Fatal(err)
				}
				saveViewPreferences(defaultPreferencesPath(), viewPreferences{Backend: saved})
				first := collect.Agent{
					ID: "herdr:other:w9:p2", Backend: collect.BackendHerdr,
					Session: "other", PaneID: "w9:p2", Kind: collect.KindInteractive,
					Life: collect.LifeInput, Project: "/p", Name: "start",
				}
				target := collect.Agent{
					ID: "herdr:caller:w9:p2", Backend: collect.BackendHerdr,
					Session: "caller", PaneID: "w9:p2", Kind: collect.KindInteractive,
					Life: collect.LifeWorking, Project: "/p", Name: "2",
				}
				m := NewWithBackend(collect.BackendHerdr)
				msg := snapshotMsg{backend: collect.BackendHerdr, snap: projSnap(first, target)}
				next, _ := m.Update(msg)
				m = next.(Model)
				if got := m.selectedAgent(); got == nil || got.ID != target.ID {
					t.Fatalf("selected agent = %#v, want caller %q", got, target.ID)
				}
				next, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
				m = next.(Model)
				next, _ = m.Update(msg)
				m = next.(Model)
				if got := m.selectedAgent(); got == nil || got.ID != first.ID {
					t.Fatalf("refresh overrode navigation: %#v", got)
				}
			})
		}
	}
}
