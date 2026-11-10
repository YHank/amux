package ui

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

var launchTools = [...]string{"claude", "codex", "omo", "omp", "opencode"}

type launchTarget struct {
	pane        string
	session     string
	rejectPopup bool // TMUX_PANE alone may identify amux's temporary session.
}

type launchedMsg struct {
	path string
	tool string
	err  error
}

func tmuxNumericID(value string, prefix byte) bool {
	if len(value) < 2 || value[0] != prefix {
		return false
	}
	for _, r := range value[1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// runLaunch resolves the captured pane to its current owning session. It never
// uses tmux's implicit current session, which may be amux's dedicated popup.
func runLaunch(ctx context.Context, target launchTarget, path, tool string) error {
	if !slices.Contains(launchTools[:], tool) {
		return fmt.Errorf("unsupported launch tool %q", tool)
	}
	dir, err := launchDirectory(path, "")
	if err != nil {
		return err
	}
	resolve := target.pane
	if resolve == "" && target.session != "" {
		resolve = "=" + target.session + ":"
	}
	if resolve == "" {
		return errors.New("no project tmux session selected; select an interactive agent")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "tmux", "display-message", "-p", "-t", resolve, "#{session_id}\t#{session_name}")
	cmd.WaitDelay = time.Second
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("resolve project tmux session: %w: %s", err, strings.TrimSpace(string(out)))
	}
	session, name, _ := strings.Cut(strings.TrimSpace(string(out)), "\t")
	if !tmuxNumericID(session, '$') {
		return fmt.Errorf("tmux returned invalid session ID %q", session)
	}
	if target.rejectPopup && strings.HasPrefix(name, "amux-popup") {
		return errors.New("cannot find the original project tmux session; select an interactive agent")
	}
	// tmux expands formats in -c; escape literal '#' so directory names cannot
	// accidentally become tmux format expressions. The tool is allowlisted.
	cmd = exec.CommandContext(ctx, "tmux", "new-window", "-t", session+":",
		"-c", strings.ReplaceAll(dir, "#", "##"), "-n", "", tool)
	cmd.WaitDelay = time.Second
	out, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("launch %s: %w: %s", tool, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (m Model) handleLaunched(msg launchedMsg) (tea.Model, tea.Cmd) {
	m.launchMenu.busy = false
	if msg.err != nil {
		m.launchMenu.error = msg.err.Error()
		m.notice = msg.err.Error()
		return m, nil
	}
	m.launchMenu.open = false
	m.notice = fmt.Sprintf("Launched %s in %s", msg.tool, msg.path)
	return m, refreshCmd
}
