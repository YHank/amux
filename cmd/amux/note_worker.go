package main

import (
	"github.com/kyhan/amux/internal/notegen"
	"github.com/spf13/cobra"
)

func noteWorkerCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "note-worker",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return notegen.Work(cmd.Context(), cmd.InOrStdin())
		},
	}
}
