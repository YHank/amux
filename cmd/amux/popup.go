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

// createPopupSession links the agent's window into a separate session so
// navigation in its original session cannot replace the popup with amux.
func createPopupSession(target string) (string, error) {
	out, err := exec.Command("tmux", "display-message", "-p", "-t", target,
		"#{window_id}\t#{pane_id}").Output()
	if err != nil {
		return "", fmt.Errorf("resolve popup target: %w", err)
	}
	window, pane, ok := strings.Cut(strings.TrimSpace(string(out)), "\t")
	if !ok {
		return "", fmt.Errorf("invalid popup target: %q", out)
	}
	session := "amux-popup-" + strconv.Itoa(os.Getpid())
	if out, err := exec.Command("tmux", "new-session", "-d", "-s", session).CombinedOutput(); err != nil {
		return "", fmt.Errorf("create popup session: %w: %s", err, out)
	}
	if out, err := exec.Command("tmux", "link-window", "-k", "-s", window, "-t", session+":0",
		";", "select-window", "-t", session+":"+window,
		";", "select-pane", "-t", pane).CombinedOutput(); err != nil {
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
// invoking client. The temporary session owns only a link, never the agent.
func attachPopup(a *ui.Action) (err error) {
	if a.TmuxTarget == "" {
		return fmt.Errorf("no tmux target")
	}
	tmuxEnv := os.Getenv("TMUX")
	session, err := createPopupSession(a.TmuxTarget)
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
		// A terminal outside tmux has no client on which to draw an overlay.
		// Attach a temporary host to this terminal, open the agent popup there,
		// then remove the host so runTUI can restore the original list.
		host := session + "-host"
		popup := shellJoin([]string{"tmux", "-S", socket, "display-popup",
			"-w", "85%", "-h", "85%", "-E", shellJoin(append([]string{"env"}, args...))})
		closeHost := shellJoin([]string{"tmux", "-S", socket, "kill-session", "-t", "=" + host})
		return runInherit("tmux", "-S", socket, "new-session", "-s", host, popup+"; "+closeHost)
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
