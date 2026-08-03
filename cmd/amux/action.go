package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/kyhan/amux/internal/ui"
)

// performAction runs the post-TUI action.
func performAction(a *ui.Action) error {
	switch a.Kind {
	case "attach":
		return attachTmux(a)
	case "manage":
		return launchClaudeAction(a)
	}
	return nil
}

// attachTmux jumps to the selected pane. Inside tmux we switch-client (works
// even from a popup); outside we attach the session to the current terminal.
func attachTmux(a *ui.Action) error {
	if a.TmuxTarget == "" {
		return fmt.Errorf("no tmux target")
	}
	// From a popup, switch-client with no -c would target the popup's own
	// ephemeral client (destroyed on close), not the client that opened it.
	if c := os.Getenv("AMUX_CLIENT"); c != "" {
		return runInherit("tmux", "switch-client", "-c", c, "-t", a.TmuxTarget)
	}
	if os.Getenv("TMUX") != "" {
		return runInherit("tmux", "switch-client", "-t", a.TmuxTarget)
	}
	target := a.Session
	if target == "" {
		target = a.TmuxTarget
	}
	return runInherit("tmux", "attach-session", "-t", target)
}

func claudeActionArgs(a *ui.Action) (windowName string, args []string, err error) {
	if a.Kind != "manage" {
		return "", nil, fmt.Errorf("unsupported Claude action %q", a.Kind)
	}
	args = []string{"agents"}
	if a.CWD != "" {
		args = append(args, "--cwd", a.CWD)
	}
	return "claude-agents", args, nil
}

func launchClaudeAction(a *ui.Action) error {
	windowName, args, err := claudeActionArgs(a)
	if err != nil {
		return err
	}
	if a.CWD != "" {
		info, statErr := os.Stat(a.CWD)
		if statErr != nil || !info.IsDir() {
			fmt.Fprintf(os.Stderr, "amux: cannot use agent cwd %q\n", a.CWD)
			printClaudeFallback("claude", args, a.CWD)
			return nil
		}
	}
	claudePath, err := exec.LookPath("claude")
	if err != nil {
		fmt.Fprintln(os.Stderr, "amux: claude executable not found")
		printClaudeFallback("claude", args, a.CWD)
		return nil
	}

	if os.Getenv("TMUX") == "" {
		if err := runInheritDir(a.CWD, claudePath, args...); err != nil {
			fmt.Fprintln(os.Stderr, "amux: Claude action failed:", err)
			printClaudeFallback(claudePath, args, a.CWD)
		}
		return nil
	}

	session, err := invokingTmuxSession()
	if err != nil {
		fmt.Fprintln(os.Stderr, "amux:", err)
		printClaudeFallback(claudePath, args, a.CWD)
		return nil
	}
	windowID, created, err := createClaudeWindow(session, windowName, a.CWD, claudePath, args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "amux:", err)
		if created {
			printCreatedWindowRecovery(windowID)
		} else {
			printClaudeFallback(claudePath, args, a.CWD)
		}
		return nil
	}
	if err := focusTmuxWindow(windowID); err != nil {
		fmt.Fprintln(os.Stderr, "amux: resume window created but focus failed:", err)
		printCreatedWindowRecovery(windowID)
	}
	return nil
}

func invokingTmuxSessionArgs(client, pane string) []string {
	switch {
	case client != "":
		return []string{"display-message", "-p", "-c", client, "#{client_session}"}
	case pane != "":
		return []string{"display-message", "-p", "-t", pane, "#{session_name}"}
	default:
		return []string{"display-message", "-p", "#{session_name}"}
	}
}

func invokingTmuxSession() (string, error) {
	args := invokingTmuxSessionArgs(os.Getenv("AMUX_CLIENT"), os.Getenv("TMUX_PANE"))
	out, err := exec.Command("tmux", args...).Output()
	if err != nil {
		return "", fmt.Errorf("cannot resolve invoking tmux session: %w", err)
	}
	session := strings.TrimSpace(string(out))
	if session == "" {
		return "", fmt.Errorf("cannot resolve invoking tmux session: empty result")
	}
	return session, nil
}

func claudeWindowCommand(claudePath string, claudeArgs []string) string {
	return "exec " + shellJoin(append([]string{claudePath}, claudeArgs...))
}

func createClaudeWindow(session, windowName, cwd, claudePath string, claudeArgs []string) (string, bool, error) {
	command := claudeWindowCommand(claudePath, claudeArgs)
	args := []string{
		"new-window", "-d", "-P", "-F", "#{window_id}",
		"-t", session + ":", "-n", windowName,
	}
	if cwd != "" {
		args = append(args, "-c", cwd)
	}
	args = append(args, command)
	out, err := exec.Command("tmux", args...).CombinedOutput()
	if err != nil {
		return "", false, fmt.Errorf("cannot create Claude tmux window: %w: %s", err, strings.TrimSpace(string(out)))
	}
	windowID := strings.TrimSpace(string(out))
	if !strings.HasPrefix(windowID, "@") {
		return windowID, true, fmt.Errorf("Claude tmux window was created but returned target %q", windowID)
	}
	return windowID, true, nil
}

func focusTmuxArgs(client, windowID string) []string {
	if client != "" {
		return []string{"switch-client", "-c", client, "-t", windowID}
	}
	return []string{"select-window", "-t", windowID}
}

func focusTmuxWindow(windowID string) error {
	return exec.Command("tmux", focusTmuxArgs(os.Getenv("AMUX_CLIENT"), windowID)...).Run()
}

func printCreatedWindowRecovery(windowID string) {
	if strings.HasPrefix(windowID, "@") {
		fmt.Printf("Claude window is still running. Open it with:\n  tmux select-window -t %s\n", shellQuote(windowID))
		return
	}
	fmt.Println("Claude window may still be running. Locate it with:")
	fmt.Println("  tmux list-windows -a")
}

func printClaudeFallback(claudePath string, args []string, cwd string) {
	command := shellJoin(append([]string{claudePath}, args...))
	fmt.Println("Run manually:")
	if cwd != "" {
		fmt.Printf("  cd %s && %s\n", shellQuote(cwd), command)
		return
	}
	fmt.Println("  " + command)
}

func shellJoin(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = shellQuote(arg)
	}
	return strings.Join(quoted, " ")
}

func runInherit(name string, args ...string) error {
	return runInheritDir("", name, args...)
}

func runInheritDir(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
