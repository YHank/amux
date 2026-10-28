// Command amux is a lightweight TUI that tracks AI coding agents across every
// tmux session and every headless Claude Code background daemon, grouped by the
// project they belong to.
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/kyhan/amux/internal/ui"
)

// version is overridden at build time via -ldflags "-X main.version=…".
var version = "dev"

func main() {
	root := &cobra.Command{
		Use:          "amux",
		Short:        "Track AI agents across tmux sessions and background daemons",
		Version:      version,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTUI()
		},
	}
	root.AddCommand(popupCmd(), statusCmd(), setupKeybindCmd(), noteWorkerCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "amux:", err)
		os.Exit(1)
	}
}

// runTUI launches the dashboard, then performs whatever action the user chose.
func runTUI() error {
	p := tea.NewProgram(ui.New(), tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return err
	}
	m, ok := final.(ui.Model)
	if !ok {
		return nil
	}
	if act := m.Chosen(); act != nil {
		return performAction(act)
	}
	return nil
}
