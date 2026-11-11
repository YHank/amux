package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kyhan/amux/internal/collect"
	"github.com/kyhan/amux/internal/herdr"
)

func (m Model) sendAgentCommand(a collect.Agent, text string) tea.Cmd {
	backend, generation := m.selectedBackend(), m.generation
	return func() tea.Msg {
		if a.Backend != collect.BackendHerdr {
			msg := sendToPane(a.TmuxTarget, text)().(sentMsg)
			msg.backend, msg.generation = backend, generation
			return msg
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := herdr.Run(ctx, a.Session, "pane", "send-text", a.PaneID, text)
		if err == nil {
			_, err = herdr.Run(ctx, a.Session, "pane", "send-keys", a.PaneID, "enter")
		}
		return sentMsg{target: a.PaneID, err: err, backend: backend, generation: generation}
	}
}

func (m Model) killAgentCommand(a collect.Agent) tea.Cmd {
	backend, generation := m.selectedBackend(), m.generation
	return func() tea.Msg {
		var msg killedMsg
		if a.Backend == collect.BackendHerdr {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, err := herdr.Run(ctx, a.Session, "pane", "close", a.PaneID)
			msg = killedMsg{target: a.PaneID, name: a.Name, err: err}
		} else {
			msg = killPane(a.TmuxTarget, a.Name)().(killedMsg)
		}
		msg.backend, msg.generation, msg.agentID = backend, generation, a.ID
		return msg
	}
}

func renameCapturedAgent(a collect.Agent, title string, automatic bool) tea.Cmd {
	if a.Backend != collect.BackendHerdr {
		target := a.TmuxTarget
		if strings.HasPrefix(a.ID, "%") {
			target = a.ID
		}
		return renameWindow(target, title, automatic)
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		// Collection uses the tab label as Window; renaming it keeps the title
		// editor and automatic start/stay labels consistent after a refresh.
		var err error
		if a.TabID != "" {
			_, err = herdr.Run(ctx, a.Session, "tab", "rename", a.TabID, title)
		} else {
			_, err = herdr.Run(ctx, a.Session, "pane", "rename", a.PaneID, title)
		}
		if err != nil {
			err = fmt.Errorf("rename title: %w", err)
		}
		return renamedWindowMsg{target: a.ID, title: title, err: err, automatic: automatic}
	}
}

func (m Model) renameAgentCommand(a collect.Agent, title string, automatic bool) tea.Cmd {
	if !interactiveTarget(a) {
		return nil
	}
	backend, generation := m.selectedBackend(), m.generation
	return func() tea.Msg {
		msg := renameCapturedAgent(a, title, automatic)().(renamedWindowMsg)
		msg.backend, msg.generation = backend, generation
		return msg
	}
}

// notegen's existing worker consumes Preview when TmuxTarget is empty. Capture
// Herdr explicitly before handing it off, so its process never follows focus.
func captureNoteAgent(ctx context.Context, a collect.Agent) (collect.Agent, error) {
	if a.Backend != collect.BackendHerdr {
		return a, nil
	}
	out, err := herdr.Run(ctx, a.Session, "pane", "read", a.PaneID,
		"--source", "recent", "--lines", "40", "--format", "ansi")
	if err != nil {
		return a, fmt.Errorf("capture Herdr pane: %w", err)
	}
	a.TmuxTarget = ""
	a.Preview = strings.Split(strings.TrimRight(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n"), "\n")
	return a, nil
}
