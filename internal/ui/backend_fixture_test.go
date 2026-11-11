package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kyhan/amux/internal/collect"
)

// This executable implements the external CLI protocol over private files.
// Observable pane state proves dispatch, without mutating any live Herdr session.
func herdrActionFixture(t *testing.T) (Model, collect.Agent, string) {
	t.Helper()
	m := backendTestModel(t)
	root := t.TempDir()
	t.Setenv("HERDR_UI_TEST_ROOT", root)
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	script := `#!/bin/sh
set -eu
[ "$1" = "--session" ]
session="$2"
shift 2
[ "$session" = "captured" ]
base="$HERDR_UI_TEST_ROOT"
case "$1:$2" in
pane:send-text) printf '%s' "$4" > "$base/$3.input";;
pane:send-keys) [ "$4" = "enter" ]; cp "$base/$3.input" "$base/$3.submitted";;
pane:close) rm "$base/$3.live";;
pane:read) cat "$base/$3.output";;
tab:rename) printf '%s' "$4" > "$base/$3.title";;
pane:rename) printf '%s' "$4" > "$base/$3.title";;
tab:create)
  [ "$3" = "--workspace" ]; [ "$4" = "captured-workspace" ]
  [ "$5" = "--cwd" ]; [ -d "$6" ]
  [ "$7" = "--label" ]; [ "$9" = "--no-focus" ]
  : > "$base/created.live"
  printf '{"result":{"root_pane":{"pane_id":"created","terminal_id":"created-terminal"}}}'
  ;;
workspace:create)
  [ "$3" = "--cwd" ]; [ -d "$4" ]
  [ "$5" = "--label" ]; [ "$6" = "amux claude" ]; [ "$7" = "--no-focus" ]
  : > "$base/created.live"
  printf '{"result":{"root_pane":{"pane_id":"created","terminal_id":"created-terminal"}}}'
  ;;
pane:run) [ -f "$base/$3.live" ]; printf '%s' "$4" > "$base/$3.command";;
*) exit 41;;
esac
`
	if err := os.WriteFile(filepath.Join(root, "herdr"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{"captured-pane.live": "", "other.live": "",
		"captured-pane.output": "CAPTURED_OUTPUT\n", "other.output": "WRONG_OUTPUT\n",
		"captured-tab.title": "original", "other-tab.title": "untouched"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	a := collect.Agent{ID: "herdr:captured:captured-pane", Backend: collect.BackendHerdr,
		PaneID: "captured-pane", TerminalID: "captured-terminal", WorkspaceID: "captured-workspace", TabID: "captured-tab",
		Session: "captured", Name: "Herdr agent", Window: "original", Kind: collect.KindInteractive,
		Life: collect.LifeWorking, Project: root, CWD: root, Tool: "codex"}
	m.backend, m.loading = collect.BackendHerdr, false
	m.snap = projSnap(a)
	m.rebuildRows()
	return m, a, root
}

func readActionFile(t *testing.T, root, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func replaceSelectedHerdrAgent(m Model) Model {
	other := *m.selectedAgent()
	other.ID, other.PaneID, other.TabID, other.TerminalID, other.WorkspaceID =
		"herdr:other:other", "other", "other-tab", "other-terminal", "other-workspace"
	other.Session = "other"
	m.snap = projSnap(other)
	m.rebuildRows()
	return m
}
