package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func popupTestServer(t *testing.T) func(...string) string {
	t.Helper()
	// Darwin limits Unix socket paths to 104 bytes.
	t.Setenv("TMPDIR", "/tmp")
	socket := filepath.Join(t.TempDir(), "tmux.sock")
	t.Setenv("TMUX", socket+",0,0")
	t.Setenv("TMUX_PANE", "")
	run := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("tmux", append([]string{"-S", socket}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("tmux %q: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	run("-f", "/dev/null", "new-session", "-d", "-s", "source", "-n", "amux", "cat")
	t.Cleanup(func() {
		if out, err := exec.Command("tmux", "-S", socket, "kill-server").CombinedOutput(); err != nil {
			t.Errorf("cleanup tmux: %v: %s", err, out)
		}
	})
	return run
}

func TestPopupKeepsAgentWhenSourceSessionSelectsAmux(t *testing.T) {
	run := popupTestServer(t)
	target := run("new-window", "-d", "-P", "-F", "#{pane_id}", "-t", "source", "-n", "agent", "cat")
	run("select-window", "-t", "source:0")
	agentWindow := run("display-message", "-p", "-t", target, "#{window_id}")
	sourceWindow := run("display-message", "-p", "-t", "source:", "#{window_id}")

	popup, err := createPopupSession(target)
	if err != nil {
		t.Fatal(err)
	}
	run("select-window", "-t", "source:0")

	if got := run("display-message", "-p", "-t", popup+":", "#{window_id}"); got != agentWindow {
		t.Fatalf("popup followed the amux window: got %s, want %s", got, agentWindow)
	}
	if got := run("display-message", "-p", "-t", "source:", "#{window_id}"); got != sourceWindow {
		t.Fatalf("opening popup changed the source window: got %s, want %s", got, sourceWindow)
	}
	run("kill-session", "-t", "="+popup)
	if got := run("display-message", "-p", "-t", target, "#{pane_id}"); got != target {
		t.Fatalf("closing popup lost the original agent pane: %s", got)
	}
}

func TestPopupDetectionWithoutClientMarker(t *testing.T) {
	run := popupTestServer(t)
	pane := run("display-message", "-p", "-t", "source:0", "#{pane_id}")
	paneTTY := run("display-message", "-p", "-t", pane, "#{pane_tty}")
	popupTTY := run("split-window", "-d", "-P", "-F", "#{pane_tty}", "-t", pane, "cat")
	for _, tc := range []struct {
		name, tty, pane string
		want            bool
	}{
		{"normal pane", paneTTY, pane, false},
		{"generic popup", popupTTY, pane, true},
		{"popup without pane environment", popupTTY, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("AMUX_CLIENT", "")
			t.Setenv("TMUX_PANE", tc.pane)
			input, err := os.Open(tc.tty)
			if err != nil {
				t.Fatal(err)
			}
			original := os.Stdin
			os.Stdin = input
			t.Cleanup(func() {
				os.Stdin = original
				if err := input.Close(); err != nil {
					t.Error(err)
				}
			})
			got, err := runningInPopup()
			if err != nil || got != tc.want {
				t.Fatalf("popup=%v, want %v: %v", got, tc.want, err)
			}
		})
	}
}
