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

// previewLines is how many trailing lines of a pane's real content amux reads
// per refresh — enough to reliably contain a choice-prompt block and to fill
// the detail panel, cheap enough to fetch every cycle for a handful of panes.
const previewLines = 40

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
		preview := capturePane(pn.target, previewLines)
		life := interactiveLife(pn.title)
		needsInput := looksLikeChoicePrompt(preview)
		if needsInput {
			life = LifeInput
		}
		agents = append(agents, Agent{
			ID:         pn.paneID,
			Kind:       KindInteractive,
			Tool:       tool,
			Name:       pn.target,
			Life:       life,
			State:      "live",
			Detail:     titleOrCmd(pn.title, ap),
			CWD:        pn.path,
			Project:    projectKey(pn.path),
			TmuxTarget: pn.target,
			Session:    pn.session,
			PID:        ap.pid,
			CPU:        procs.subtreeCPU(ap.pid),
			Preview:    preview,
			NeedsInput: needsInput,
		})
	}
	return agents, nil
}

// capturePane returns the last n lines of a pane's actual terminal content,
// right-trimmed. Best-effort: a capture failure (pane closed mid-scan, etc.)
// yields nil rather than an error, since a missing preview shouldn't drop the
// whole agent from the snapshot.
func capturePane(target string, n int) []string {
	out, err := exec.Command("tmux", "capture-pane", "-p", "-t", target, "-S", "-"+strconv.Itoa(n)).Output()
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	return lines
}

// choicePromptScanLines bounds how many trailing lines are checked for the
// choice-prompt footer. A genuine pending prompt is always the current bottom
// of the screen — scanning the whole preview window instead risks matching
// scrollback that merely mentions the phrase. This exact failure mode was
// reproduced live: a conversation that was busy explaining this very
// heuristic (quoting the marker text verbatim) scrolled that quote into a
// monitored pane's history, and a whole-preview scan flagged it as pending.
const choicePromptScanLines = 8

// looksLikeChoicePrompt reports whether the captured pane content shows the
// agent mid-question, blocked on the user picking one of several options —
// e.g. Claude Code's AskUserQuestion UI:
//
//	 ☐ Choice
//	Pick A or B?
//	❯ 1. A
//	  2. B
//	Enter to select · ↑/↓ to navigate · Esc to cancel
//
// The footer line is the signal: verified live against a real pending
// question (captured mid-prompt) and against ordinary idle/typing screens
// (which use the same "❯ " cursor glyph but never this footer), so matching
// on it avoids false positives from "❯ " alone. This is intentionally
// conservative — dialects of this UI or other tools' own approval prompts that
// phrase their footer differently won't be caught.
func looksLikeChoicePrompt(lines []string) bool {
	tail := lines
	if len(tail) > choicePromptScanLines {
		tail = tail[len(tail)-choicePromptScanLines:]
	}
	for _, l := range tail {
		if strings.Contains(l, "Enter to select") && strings.Contains(l, "navigate") {
			return true
		}
	}
	return false
}

// interactiveLife distinguishes an AI-tool pane that is actively generating
// from one that's alive but idle (waiting on the user).
//
// Claude Code and Oh My Pi both write an animated spinner glyph as the first
// character of the tmux pane title while actively working (e.g. "⠂ my task"),
// and drop it once they're idle/waiting for input (e.g. "claude agents",
// "π: my task"). The glyph is a Braille Pattern character (U+2800–U+28FF), a
// block used exclusively for spinner animations, not real text — verified
// live by capturing this exact pane's title mid-generation vs. an idle
// sibling pane. Absent that signal (no title, or a tool that doesn't set one),
// we can't confirm active generation, so default to idle rather than assuming.
func interactiveLife(title string) Lifecycle {
	t := strings.TrimSpace(title)
	if t == "" {
		return LifeIdle
	}
	r := []rune(t)[0]
	if r >= 0x2800 && r <= 0x28FF {
		return LifeWorking
	}
	return LifeIdle
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
