package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/kyhan/amux/internal/ui"
)

// performAction runs the post-TUI action: switch/attach to a tmux pane, or print
// a resume command for a headless background agent.
func performAction(a *ui.Action) error {
	switch a.Kind {
	case "attach":
		return attachTmux(a)
	case "resume":
		printResume(a)
		return nil
	}
	return nil
}

// attachTmux jumps to the selected pane. Inside tmux we switch-client (works
// even from a popup); outside we attach the session to the current terminal.
func attachTmux(a *ui.Action) error {
	if a.TmuxTarget == "" {
		return fmt.Errorf("no tmux target")
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

// printResume tells the user how to bring a headless background agent to the
// foreground. Printed rather than executed so the user stays in control of
// where the interactive session lands.
func printResume(a *ui.Action) {
	fmt.Println()
	fmt.Println("Resume this background agent with:")
	if a.CWD != "" {
		fmt.Printf("  (cd %q && claude --resume %s)\n", a.CWD, a.SessionID)
	} else {
		fmt.Printf("  claude --resume %s\n", a.SessionID)
	}
}

func runInherit(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
