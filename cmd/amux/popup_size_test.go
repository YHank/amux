package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/kyhan/amux/internal/ui"
)

func TestPopupWithoutTmuxKeepsTerminalSize(t *testing.T) {
	t.Setenv("TMPDIR", "/tmp")
	t.Setenv("TMUX_TMPDIR", t.TempDir())
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_PANE", "")
	t.Setenv("AMUX_CLIENT", "")
	t.Setenv("TERM", "xterm-256color")
	run := func(args ...string) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, "tmux", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("tmux %q: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	target := run("-f", "/dev/null", "new-session", "-d", "-P", "-F", "#{pane_id}", "-s", "source", "cat")
	t.Cleanup(func() { run("kill-server") })
	run("set-hook", "-g", "client-attached",
		"if-shell -F '#{m/r:^amux-popup-[0-9]+$,#{session_name}}' 'wait-for -S project-attached'")
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "-test.run=^TestPopupSizeFixture$")
	cmd.Env = append(os.Environ(), "AMUX_POPUP_SIZE_TARGET="+target)
	master, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: 100, Rows: 30})
	if err != nil {
		t.Fatal(err)
	}
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		if _, err := io.Copy(io.Discard, master); err != nil && !errors.Is(err, os.ErrClosed) {
			// A PTY reports EIO when its slave closes on Linux.
			return
		}
	}()
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			cancel()
			if err := cmd.Wait(); err == nil {
				t.Error("fixture unexpectedly exited successfully after cancellation")
			}
		}
		if err := master.Close(); err != nil {
			t.Error(err)
		}
		<-drained
	})
	run("wait-for", "project-attached")
	session := fmt.Sprintf("amux-popup-%d", cmd.Process.Pid)
	got := run("list-clients", "-t", "="+session, "-F", "#{client_width}x#{client_height}")
	if got != "100x30" {
		t.Fatalf("project popup shrank inside the existing terminal: got %s, want 100x30", got)
	}
	run("detach-client", "-s", "="+session)
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	if got := run("list-sessions", "-F", "#{session_name}"); got != "source" {
		t.Fatalf("temporary popup session was not removed: %s", got)
	}
	if got := run("display-message", "-p", "-t", target, "#{pane_id}"); got != target {
		t.Fatalf("agent pane was lost: %s", got)
	}
}

func TestPopupSizeFixture(t *testing.T) {
	target := os.Getenv("AMUX_POPUP_SIZE_TARGET")
	if target == "" {
		return
	}
	if err := attachPopup(&ui.Action{TmuxTarget: target}); err != nil {
		t.Fatal(err)
	}
}
