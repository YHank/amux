package ui

import (
	"context"
	"encoding/json"
	"os"
	"strings"

	"github.com/kyhan/amux/internal/collect"
	"github.com/kyhan/amux/internal/herdr"
)

func startupAgentID(backend collect.Backend) string {
	if backend != collect.BackendHerdr {
		if pane := strings.TrimSpace(os.Getenv("AMUX_START_PANE")); pane != "" {
			return pane
		}
		return strings.TrimSpace(os.Getenv("TMUX_PANE"))
	}
	// Native shortcut popups receive ACTIVE_PANE_ID for the invoking pane;
	// PANE_ID is only available when running directly inside a managed pane.
	pane := strings.TrimSpace(os.Getenv("HERDR_ACTIVE_PANE_ID"))
	if pane == "" {
		pane = strings.TrimSpace(os.Getenv("HERDR_PANE_ID"))
	}
	socket := strings.TrimSpace(os.Getenv("HERDR_SOCKET_PATH"))
	if pane == "" || socket == "" {
		return ""
	}
	// Pane IDs are local to a Herdr server. Resolve the caller's socket to the
	// same session-qualified ID used by collection, not the UI-focused pane.
	out, err := herdr.Run(context.Background(), "", "session", "list", "--json")
	if err != nil {
		return ""
	}
	var inventory struct {
		Sessions []struct {
			Name       string `json:"name"`
			Running    bool   `json:"running"`
			SocketPath string `json:"socket_path"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal(out, &inventory); err != nil {
		return ""
	}
	for _, session := range inventory.Sessions {
		if session.Running && session.Name != "" && session.SocketPath == socket {
			return "herdr:" + session.Name + ":" + pane
		}
	}
	return ""
}
