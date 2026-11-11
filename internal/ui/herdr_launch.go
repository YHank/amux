package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kyhan/amux/internal/herdr"
)

var errHerdrLaunchContext = errors.New("select an interactive Herdr agent with an explicit named session")

func runHerdrLaunch(ctx context.Context, target launchTarget, dir, tool string) error {
	// Neither workspace nor session may fall back to the user's focused pane.
	if target.session == "" {
		return errHerdrLaunchContext
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	args := []string{"tab", "create", "--workspace", target.workspace, "--cwd", dir, "--label", tool, "--no-focus"}
	if target.workspace == "" {
		args = []string{"workspace", "create", "--cwd", dir, "--label", "amux " + tool, "--no-focus"}
	}
	out, err := herdr.Run(ctx, target.session, args...)
	if err != nil {
		return fmt.Errorf("create Herdr tab: %w", err)
	}
	var response struct {
		Result struct {
			RootPane struct {
				PaneID     string `json:"pane_id"`
				TerminalID string `json:"terminal_id"`
			} `json:"root_pane"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &response); err != nil {
		return fmt.Errorf("decode Herdr tab: %w", err)
	}
	pane := response.Result.RootPane.PaneID
	if pane == "" {
		return errors.New("Herdr tab creation returned no root pane")
	}
	if _, err := herdr.Run(ctx, target.session, "pane", "run", pane, tool); err != nil {
		return fmt.Errorf("launch %s in Herdr tab %s: %w", tool, pane, err)
	}
	return nil
}
