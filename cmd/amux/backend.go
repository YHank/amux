package main

import (
	"fmt"

	"github.com/kyhan/amux/internal/collect"
	"github.com/kyhan/amux/internal/ui"
	"github.com/spf13/cobra"
)

func addBackendFlag(cmd *cobra.Command) {
	cmd.Flags().String("backend", "", "terminal backend: tmux or herdr (default: saved selection)")
}

func commandBackend(cmd *cobra.Command) (collect.Backend, error) {
	value, err := cmd.Flags().GetString("backend")
	if err != nil {
		return "", err
	}
	switch collect.Backend(value) {
	case "":
		return ui.SelectedBackend(), nil
	case collect.BackendTmux, collect.BackendHerdr:
		return collect.Backend(value), nil
	default:
		return "", fmt.Errorf("unknown backend %q: choose tmux or herdr", value)
	}
}
