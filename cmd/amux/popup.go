package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/kyhan/amux/internal/ui"
)

// createPopupSession links the project's windows into a separate session so
// navigation in the original sessions cannot replace the popup with amux.
func createPopupSession(a *ui.Action) (string, error) {
	target := a.TmuxTarget
	out, err := exec.Command("tmux", "display-message", "-p", "-t", target,
		"#{window_id}\t#{pane_id}").Output()
	if err != nil {
		return "", fmt.Errorf("resolve popup target: %w", err)
	}
	window, pane, ok := strings.Cut(strings.TrimSpace(string(out)), "\t")
	if !ok {
		return "", fmt.Errorf("invalid popup target: %q", out)
	}
	windows := []string{window}
	seen := map[string]bool{window: true}
	for _, target := range a.ProjectTargets {
		if target == a.TmuxTarget {
			continue
		}
		out, err := exec.Command("tmux", "display-message", "-p", "-t", target, "#{window_id}").Output()
		if err != nil {
			// A peer may have closed since the move menu was opened.
			continue
		}
		peer := strings.TrimSpace(string(out))
		if peer != "" && !seen[peer] {
			windows = append(windows, peer)
			seen[peer] = true
		}
	}
	session := "amux-popup-" + strconv.Itoa(os.Getpid())
	if out, err := exec.Command("tmux", "new-session", "-d", "-s", session).CombinedOutput(); err != nil {
		return "", fmt.Errorf("create popup session: %w: %s", err, out)
	}
	args := []string{"link-window", "-k", "-s", window, "-t", session + ":{start}"}
	for _, peer := range windows[1:] {
		args = append(args, ";", "link-window", "-d", "-s", peer, "-t", session+":")
	}
	args = append(args, ";", "set-option", "-t", session, "status", "on",
		";", "select-window", "-t", session+":"+window,
		";", "select-pane", "-t", pane)
	if out, err := exec.Command("tmux", args...).CombinedOutput(); err != nil {
		cleanupErr := exec.Command("tmux", "kill-session", "-t", "="+session).Run()
		return "", errors.Join(fmt.Errorf("link popup target: %w: %s", err, out), cleanupErr)
	}
	return session, nil
}

func runningInPopup() (bool, error) {
	if os.Getenv("AMUX_CLIENT") != "" {
		return true, nil
	}
	// Generic display-popup bindings do not set AMUX_CLIENT. Their input
	// terminal is the popup PTY, not the invoking pane's terminal.
	out, err := exec.Command("tmux", "display-message", "-p", "#{pane_tty}").Output()
	if err != nil {
		return false, fmt.Errorf("resolve invoking pane terminal: %w", err)
	}
	pane, err := os.Stat(strings.TrimSpace(string(out)))
	if err != nil {
		return false, fmt.Errorf("stat invoking pane terminal: %w", err)
	}
	input, err := os.Stdin.Stat()
	if err != nil {
		return false, fmt.Errorf("stat popup input terminal: %w", err)
	}
	return !os.SameFile(input, pane), nil
}

// attachPopup reuses the popup hosting amux, or opens an agent popup over the
// invoking client. The temporary session owns only links, never the agents.
func attachPopup(a *ui.Action) (err error) {
	if a.TmuxTarget == "" {
		return fmt.Errorf("no tmux target")
	}
	tmuxEnv := os.Getenv("TMUX")
	session, err := createPopupSession(a)
	if err != nil {
		return err
	}
	defer func() {
		if cleanupErr := exec.Command("tmux", "kill-session", "-t", "="+session).Run(); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("close popup session: %w", cleanupErr))
		}
	}()
	out, err := exec.Command("tmux", "display-message", "-p", "-t", "="+session, "#{socket_path}").Output()
	if err != nil {
		return fmt.Errorf("resolve popup socket: %w", err)
	}
	socket := strings.TrimSpace(string(out))
	args := []string{"-u", "TMUX", "-u", "AMUX_CLIENT", "tmux",
		"-S", socket, "attach-session", "-E", "-t", "=" + session}
	if tmuxEnv == "" {
		// The current terminal may itself be a popup with TMUX unset.
		// Attach directly to preserve its size instead of nesting an overlay.
		return runInherit("env", args...)
	}
	reuse, err := runningInPopup()
	if err != nil {
		return err
	}
	if reuse {
		return runInherit("env", args...)
	}
	client := currentClient()
	if client == "" {
		return fmt.Errorf("cannot resolve popup client")
	}
	return runInherit("tmux", "display-popup", "-c", client, "-w", "85%", "-h", "85%", "-E",
		shellJoin(append([]string{"env"}, args...)))
}
