package collect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kyhan/amux/internal/herdr"
)

type herdrPane struct {
	PaneID        string `json:"pane_id"`
	TerminalID    string `json:"terminal_id"`
	WorkspaceID   string `json:"workspace_id"`
	TabID         string `json:"tab_id"`
	CWD           string `json:"cwd"`
	ForegroundCWD string `json:"foreground_cwd"`
	Title         string `json:"terminal_title"`
	AgentSession  struct {
		Agent string `json:"agent"`
		Kind  string `json:"kind"`
		Value string `json:"value"`
	} `json:"agent_session"`
}

// herdrAgents inventories all running sessions. Any incomplete inventory leaves
// LivePaneIDs nil, while agents from successful sessions remain available.
func herdrAgents(ctx context.Context, procs *procTable) ([]Agent, map[string]bool, error) {
	out, err := herdr.Run(ctx, "", "session", "list", "--json")
	if err != nil {
		return nil, nil, err
	}
	var inventory struct {
		Sessions []struct {
			Name    string `json:"name"`
			Running bool   `json:"running"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal(out, &inventory); err != nil {
		return nil, nil, fmt.Errorf("herdr session list: %w", err)
	}
	if inventory.Sessions == nil {
		return nil, nil, errors.New("herdr session list: missing sessions inventory")
	}
	ids := make(map[string]bool)
	var agents []Agent
	var failures []error
	home, homeErr := os.UserHomeDir()
	for _, session := range inventory.Sessions {
		if !session.Running {
			continue
		}
		if session.Name == "" {
			failures = append(failures, errors.New("herdr running session has no name"))
			continue
		}
		out, err := herdr.Run(ctx, session.Name, "api", "snapshot")
		if err != nil {
			failures = append(failures, err)
			continue
		}
		var response struct {
			Result struct {
				Snapshot *struct {
					Panes []herdrPane `json:"panes"`
					Tabs  []struct {
						ID    string `json:"tab_id"`
						Label string `json:"label"`
					} `json:"tabs"`
				} `json:"snapshot"`
			} `json:"result"`
		}
		if err := json.Unmarshal(out, &response); err != nil {
			failures = append(failures, fmt.Errorf("herdr snapshot %q: %w", session.Name, err))
			continue
		}
		if response.Result.Snapshot == nil || response.Result.Snapshot.Panes == nil {
			failures = append(failures, fmt.Errorf("herdr snapshot %q: missing panes inventory", session.Name))
			continue
		}
		titles := make(map[string]string, len(response.Result.Snapshot.Tabs))
		for _, tab := range response.Result.Snapshot.Tabs {
			titles[tab.ID] = tab.Label
		}
		for _, hp := range response.Result.Snapshot.Panes {
			if hp.PaneID == "" {
				failures = append(failures, fmt.Errorf("herdr snapshot %q: missing pane_id", session.Name))
				continue
			}
			id := "herdr:" + session.Name + ":" + hp.PaneID
			ids[id] = true
			ap, found, err := herdrAgentProc(ctx, session.Name, hp.PaneID, procs)
			if err != nil {
				failures = append(failures, err)
				continue
			}
			if !found {
				continue
			}
			cwd := hp.ForegroundCWD
			if cwd == "" {
				cwd = hp.CWD
			}
			pn := pane{backend: BackendHerdr, session: session.Name, paneID: hp.PaneID, path: cwd, title: hp.Title, window: titles[hp.TabID]}
			var preview []string
			if raw, err := herdr.Run(ctx, session.Name, "pane", "read", hp.PaneID, "--source", "recent", "--lines", "40", "--format", "ansi"); err == nil {
				preview = strings.Split(strings.TrimRight(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n"), "\n")
				for i := range preview {
					preview[i] = strings.TrimRight(preview[i], " \t")
				}
			}
			a := collectInteractivePreview(procs, pn, ap, home, homeErr == nil, preview)
			if hp.AgentSession.Agent == a.Tool && hp.AgentSession.Kind == "id" && homeErr == nil {
				switch a.Tool {
				case "opencode":
					dataHome := os.Getenv("XDG_DATA_HOME")
					if dataHome == "" {
						dataHome = filepath.Join(home, ".local", "share")
					}
					a.Checklist = opencodeChecklist(ctx, filepath.Join(dataHome, "opencode"), hp.AgentSession.Value)
					if len(a.Checklist) == 0 {
						a.Intent = opencodeIntent(ctx, filepath.Join(dataHome, "opencode"), hp.AgentSession.Value)
					}
				case "codex":
					root := os.Getenv("CODEX_HOME")
					if root == "" {
						root = filepath.Join(home, ".codex")
					}
					a.Checklist, a.Intent = codexChecklist(root, hp.AgentSession.Value)
				}
			}
			a.ID, a.Backend = id, BackendHerdr
			a.Name = session.Name + ":" + hp.PaneID
			a.TerminalID, a.WorkspaceID, a.TabID = hp.TerminalID, hp.WorkspaceID, hp.TabID
			agents = append(agents, a)
		}
	}
	if len(failures) > 0 {
		return agents, nil, errors.Join(failures...)
	}
	return agents, ids, nil
}

func herdrAgentProc(ctx context.Context, session, paneID string, procs *procTable) (proc, bool, error) {
	out, err := herdr.Run(ctx, session, "pane", "process-info", "--pane", paneID)
	if err != nil {
		return proc{}, false, err
	}
	var response struct {
		Result struct {
			ProcessInfo *struct {
				ShellPID   int `json:"shell_pid"`
				Foreground []struct {
					PID     int      `json:"pid"`
					Name    string   `json:"name"`
					Cmdline string   `json:"cmdline"`
					Argv    []string `json:"argv"`
				} `json:"foreground_processes"`
			} `json:"process_info"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &response); err != nil {
		return proc{}, false, fmt.Errorf("herdr process-info %q %q: %w", session, paneID, err)
	}
	info := response.Result.ProcessInfo
	if info == nil {
		return proc{}, false, fmt.Errorf("herdr process-info %q %q: missing process_info", session, paneID)
	}
	if info.ShellPID > 0 {
		if ap, found := procs.findAgentProc(info.ShellPID); found {
			return ap, true, nil
		}
	}
	for _, foreground := range info.Foreground {
		if foreground.PID <= 0 {
			continue
		}
		if ap, found := procs.findAgentProc(foreground.PID); found {
			return ap, true, nil
		}
		ap := proc{pid: foreground.PID, comm: foreground.Name, args: foreground.Cmdline}
		if ap.args == "" {
			ap.args = strings.Join(foreground.Argv, " ")
		}
		if detectTool(ap) != "" {
			return ap, true, nil
		}
	}
	return proc{}, false, nil
}
