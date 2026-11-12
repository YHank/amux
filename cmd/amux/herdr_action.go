package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/kyhan/amux/internal/herdr"
	"github.com/kyhan/amux/internal/ui"
	"github.com/spf13/cobra"
)

func performHerdrAction(a *ui.Action) error {
	switch a.Kind {
	case "attach":
		if !a.Popup && os.Getenv("HERDR_ENV") == "1" {
			if a.PaneID == "" {
				return errors.New("no Herdr pane selected")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_, err := herdr.Call(ctx, a.Session, "pane.focus", map[string]string{"pane_id": a.PaneID})
			return err
		}
		if a.TerminalID == "" {
			return errors.New("no Herdr terminal selected")
		}
		if a.Popup && os.Getenv("HERDR_ENV") == "1" && os.Getenv("AMUX_HERDR_POPUP") == "" {
			return openHerdrSurface("", "terminal", "popup", "85%", "85%", map[string]string{
				"AMUX_HERDR_SESSION":  a.Session,
				"AMUX_HERDR_TERMINAL": a.TerminalID,
			})
		}
		if a.Popup && os.Getenv("TMUX") != "" && os.Getenv("HERDR_ENV") != "1" {
			args := append([]string{"herdr"}, herdr.Args(a.Session, "terminal", "attach", a.TerminalID, "--takeover")...)
			return runInherit("tmux", "display-popup", "-w", "85%", "-h", "85%", "-E", shellJoin(args))
		}
		return runHerdrAttach(a.Session, a.TerminalID)
	case "manage":
		_, args, err := claudeActionArgs(a)
		if err != nil {
			return err
		}
		return runInheritDir(a.CWD, "claude", args...)
	default:
		return fmt.Errorf("unsupported Herdr action %q", a.Kind)
	}
}

func openHerdrDashboard(width, height string) error {
	return openHerdrSurface("", "dashboard", "popup", width, height, nil)
}

func openHerdrSurface(session, entrypoint, placement, width, height string, environment map[string]string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	path, err := writeHerdrPlugin()
	if err != nil {
		return err
	}
	if _, err := herdr.Run(ctx, session, "plugin", "link", path, "--enabled"); err != nil {
		return err
	}
	if environment == nil {
		environment = make(map[string]string)
	}
	environment["AMUX_HERDR_POPUP"] = ""
	if placement == "popup" {
		environment["AMUX_HERDR_POPUP"] = "1"
	}
	params := map[string]any{
		"plugin_id":  "amux.dashboard",
		"entrypoint": entrypoint,
		"placement":  placement,
		"env":        environment,
		"focus":      true,
	}
	if placement == "popup" {
		params["width"], params["height"] = width, height
	}
	_, err = herdr.Call(ctx, session, "plugin.pane.open", params)
	return err
}

// The plugin entrypoint receives opaque targets through the environment, so
// neither a session name nor terminal ID is evaluated as shell code.
func herdrAttachCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "herdr-attach",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			terminal := os.Getenv("AMUX_HERDR_TERMINAL")
			session := os.Getenv("AMUX_HERDR_SESSION")
			if terminal == "" || session == "" {
				return errors.New("Herdr popup terminal context is missing")
			}
			return runHerdrAttach(session, terminal)
		},
	}
}
