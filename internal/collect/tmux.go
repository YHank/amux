package collect

import (
	"os/exec"
	"strconv"
	"strings"
)

// pane is one tmux pane as reported by `tmux list-panes`.
type pane struct {
	session string
	target  string // session:win.pane
	paneID  string // %<n>, stable across renames
	pid     int
	path    string
	title   string
	active  bool
}

const paneFormat = "#{session_name}\x1f#{session_name}:#{window_index}.#{pane_index}" +
	"\x1f#{pane_id}\x1f#{pane_pid}\x1f#{pane_current_path}\x1f#{pane_title}\x1f#{?pane_active,1,0}"

// TmuxRunning reports whether a tmux server is up. When false, amux still works
// as a pure background-agent dashboard.
func TmuxRunning() bool {
	return exec.Command("tmux", "has-session").Run() == nil
}

// listPanes returns every pane across every session. An absent server is not an
// error — it just means there are no interactive panes to show.
func listPanes() ([]pane, error) {
	out, err := exec.Command("tmux", "list-panes", "-a", "-F", paneFormat).Output()
	if err != nil {
		// No server / no sessions: report zero panes, not a hard failure.
		return nil, nil
	}
	var panes []pane
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line == "" {
			continue
		}
		f := strings.Split(line, "\x1f")
		if len(f) < 7 {
			continue
		}
		pid, _ := strconv.Atoi(f[3])
		panes = append(panes, pane{
			session: f[0],
			target:  f[1],
			paneID:  f[2],
			pid:     pid,
			path:    f[4],
			title:   f[5],
			active:  f[6] == "1",
		})
	}
	return panes, nil
}

// interactiveAgents scans every pane for a foreground AI process and builds an
// Agent for each match.
func interactiveAgents(procs *procTable) ([]Agent, error) {
	panes, err := listPanes()
	if err != nil {
		return nil, err
	}
	var agents []Agent
	for _, pn := range panes {
		ap, ok := procs.findAgentProc(pn.pid)
		if !ok {
			continue
		}
		tool := detectTool(ap)
		agents = append(agents, Agent{
			ID:         pn.paneID,
			Kind:       KindInteractive,
			Tool:       tool,
			Name:       pn.target,
			Life:       LifeWorking, // a live foreground process is, by definition, live
			State:      "live",
			Detail:     titleOrCmd(pn.title, ap),
			CWD:        pn.path,
			Project:    projectKey(pn.path),
			TmuxTarget: pn.target,
			Session:    pn.session,
			PID:        ap.pid,
			CPU:        procs.subtreeCPU(ap.pid),
		})
	}
	return agents, nil
}

// titleOrCmd prefers a meaningful pane title; otherwise falls back to the argv.
func titleOrCmd(title string, p proc) string {
	t := strings.TrimSpace(title)
	// tmux titles often carry spinner glyphs or the bare tool name; keep them
	// only when they add information beyond the command.
	if t != "" {
		return t
	}
	return p.args
}
