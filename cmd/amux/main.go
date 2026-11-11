// Command amux is a lightweight TUI that tracks AI coding agents across every
// tmux session and every headless Claude Code background daemon, grouped by the
// project they belong to.
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/kyhan/amux/internal/collect"
	"github.com/kyhan/amux/internal/ui"
)

// version is overridden at build time via -ldflags "-X main.version=…".
var version = "dev"

func main() {
	root := &cobra.Command{
		Use:          "amux",
		Short:        "Track AI agents across tmux, Herdr, and background daemons",
		Version:      version,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			selected, err := commandBackend(cmd)
			if err != nil {
				return err
			}
			return runTUIWithBackend(selected)
		},
	}
	addBackendFlag(root)
	root.AddCommand(popupCmd(), statusCmd(), setupKeybindCmd(), noteWorkerCmd(), herdrAttachCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "amux:", err)
		os.Exit(1)
	}
}

// runTUI launches the dashboard, then performs whatever action the user chose.
func runTUI() error {
	return runTUIWithBackend(ui.SelectedBackend())
}

func runTUIWithBackend(selected collect.Backend) error {
	m := ui.NewWithBackend(selected)
	for {
		p := tea.NewProgram(m, tea.WithAltScreen())
		final, err := p.Run()
		if err != nil {
			return err
		}
		var ok bool
		m, ok = final.(ui.Model)
		if !ok || m.Chosen() == nil {
			return nil
		}
		act := m.Chosen()
		if err := performAction(act); err != nil {
			return err
		}
		if !act.Popup {
			return nil
		}
		m = m.Resume()
	}
}
