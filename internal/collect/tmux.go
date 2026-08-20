package collect

import (
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// pane is one tmux pane as reported by `tmux list-panes`.
type pane struct {
	session string
	window  string
	target  string // session:win.pane
	paneID  string // %<n>, stable across renames
	pid     int
	path    string
	title   string
	active  bool
}

const paneFormat = "#{session_name}\x1f#{window_name}\x1f#{session_name}:#{window_index}.#{pane_index}" +
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
		pn, ok := parsePaneLine(line)
		if !ok {
			continue
		}
		panes = append(panes, pn)
	}
	return panes, nil
}

func parsePaneLine(line string) (pane, bool) {
	f := strings.Split(line, "\x1f")
	if len(f) < 8 {
		return pane{}, false
	}
	pid, _ := strconv.Atoi(f[4])
	return pane{
		session: f[0],
		window:  f[1],
		target:  f[2],
		paneID:  f[3],
		pid:     pid,
		path:    f[5],
		title:   f[6],
		active:  f[7] == "1",
	}, true
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
		if life == LifeIdle && contentShowsSpinner(tool, preview) {
			life = LifeWorking
		}
		needsInput := looksLikeChoicePrompt(tool, preview)
		if needsInput {
			life = LifeInput
		}
		agents = append(agents, Agent{
			ID:               pn.paneID,
			Window:           pn.window,
			Kind:             KindInteractive,
			Tool:             tool,
			Name:             pn.target,
			Life:             life,
			State:            "live",
			Detail:           titleOrCmd(pn.title, ap),
			CWD:              pn.path,
			Project:          projectKey(pn.path),
			ProcessStartedAt: ap.startedAt,
			TmuxTarget:       pn.target,
			Session:          pn.session,
			PaneTitle:        pn.title,
			PID:              ap.pid,
			CPU:              procs.subtreeCPU(ap.pid),
			Preview:          preview,
			NeedsInput:       needsInput,
		})
	}
	return agents, nil
}

// capturePane returns a pane's actual terminal content ending at the bottom of
// the visible screen, right-trimmed, with the pane's own ANSI color/style codes
// preserved (-e) so the live preview shows real colors instead of flat text.
//
// Note that n is NOT the number of lines returned. "-S -n" sets the start of
// the capture range n lines up into the scrollback; with no "-E" the range runs
// to the bottom of the visible screen, so the result is n scrollback lines plus
// the whole screen — typically well over n lines. Callers that want a bounded
// window must slice the tail themselves (see contentSpinnerScanLines and
// choicePromptScanLines), and must budget for multi-line status bars sitting
// between the real signal and the last line. Best-effort: a
// capture failure (pane closed mid-scan, etc.) yields nil rather than an
// error, since a missing preview shouldn't drop the whole agent from the
// snapshot.
func capturePane(target string, n int) []string {
	out, err := exec.Command("tmux", "capture-pane", "-p", "-e", "-t", target, "-S", "-"+strconv.Itoa(n)).Output()
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	return lines
}

var ansiCSI = regexp.MustCompile("\x1b\\[[0-9;]*[a-zA-Z]")

// stripANSI removes terminal SGR/CSI escape sequences. Text-based detection
// (looksLikeChoicePrompt) runs on stripped lines so embedded color codes from
// capturePane's -e output can never split or hide the marker text it matches
// against.
func stripANSI(s string) string {
	return ansiCSI.ReplaceAllString(s, "")
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
// agent mid-question, blocked on the user picking one of several options.
// Verified footers currently include Claude Code's
// "Enter to select · ↑/↓ to navigate · Esc to cancel" and OpenCode's
// "↑↓ select  enter submit  esc dismiss".
//
// The footer is the signal: ordinary idle/typing screens share prompt glyphs
// and words with question screens but not the full interaction instructions.
// Matching the complete instruction combination keeps detection conservative.
func looksLikeChoicePrompt(tool string, lines []string) bool {
	tail := lines
	if len(tail) > choicePromptScanLines {
		tail = tail[len(tail)-choicePromptScanLines:]
	}
	for _, l := range tail {
		plain := stripANSI(l)
		if strings.Contains(plain, "Enter to select") && strings.Contains(plain, "navigate") {
			return true
		}
		if tool == "opencode" {
			lower := strings.ToLower(plain)
			if strings.Contains(lower, "↑↓ select") &&
				strings.Contains(lower, "enter submit") &&
				strings.Contains(lower, "esc dismiss") {
				return true
			}
		}
	}
	return false
}

// isSpinnerGlyph reports whether r is a character these tools use exclusively
// for animated "working" indicators, never for real text.
//
// The set is deliberately a union across tool versions, because the glyphs are
// NOT stable: Claude Code shipped a Braille spinner in older builds and moved
// to half-circle frames by 2.1.228. Matching only one family silently reports
// every busy pane as idle after an upgrade, so new families are added here
// rather than swapped in. Sources:
//
//   - U+2800–U+28FF (Braille) — older Claude Code pane titles, and Oh My Pi
//     pane body lines; both verified live.
//   - U+25D0–U+25D3 (◐◓◑◒) — Claude Code 2.1.228 pane titles, shown only while
//     the agent is actually generating. Confirmed two ways: the frame arrays
//     extracted straight out of the installed binary
//     (~/.local/share/claude/versions/2.1.228) are ["◐","◑"] and
//     ["◐","◓","◑","◒"], and live panes carrying these glyphs were the only
//     ones with an active token counter in the body.
//
// DO NOT add the star family (✳ U+2733, ✶ U+2736, ✻ U+273B, ✽ U+273D,
// ✢ U+2722) here. Claude Code 2.1.228 puts ✳ at the front of the pane title
// while the agent sits at an empty prompt — it is an IDLE marker, the exact
// opposite of a spinner. Accepting it reported every waiting pane as working.
// Verified live by cross-checking titles against pane bodies: every ✳ pane
// showed a bare "❯" prompt with no token counter, while ◐/◑ panes showed an
// active counter ("↓ 12.9k"); two panes were observed flipping ◐/◑ → ✳ as
// their task finished. The same star glyphs DO appear in Claude Code's in-body
// status line while working ("✶ Wibbling… (2m 32s · ↓ 10.2k tokens)"), so the
// family is ambiguous across positions and is not a safe signal anywhere.
// Claude Code panes are already covered by the title check.
//
// Also excluded for the same reason: '·' (U+00B7) and '∗' (U+2217).
func isSpinnerGlyph(r rune) bool {
	switch {
	case r >= 0x2800 && r <= 0x28FF:
		return true
	case r >= 0x25D0 && r <= 0x25D3:
		return true
	}
	return false
}

// interactiveLife distinguishes an AI-tool pane that is actively generating
// from one that's alive but idle (waiting on the user), using the tmux pane
// title alone.
//
// Claude Code writes a spinner glyph as the first character of the pane
// title while working (e.g. "◑ my task", or "⠂ my task" on older builds) and
// drops it when idle ("claude agents"). Which glyph family it uses varies by
// version — see isSpinnerGlyph. Oh My Pi does NOT: its title is a static
// "π: <task>" whether
// working or idle — verified live by capturing the same omp pane both mid-task
// and at rest, both showing the identical title shape. So this alone
// correctly detects Claude Code but always reads Idle for omp; omp's own
// signal is checked separately, in its pane content (see contentShowsSpinner).
func interactiveLife(title string) Lifecycle {
	t := strings.TrimSpace(title)
	if t == "" {
		return LifeIdle
	}
	if isSpinnerGlyph([]rune(t)[0]) {
		return LifeWorking
	}
	return LifeIdle
}

// contentSpinnerScanLines bounds how many trailing content lines are checked
// for an in-body spinner, for the same reason choicePromptScanLines is
// bounded: a genuine live spinner is always near the current bottom of
// screen, not buried in scrollback.
//
// It stays tight because the tools that need it — omp and OpenCode — render
// their spinner just above a one or two line input box, with no tall status
// bar in between. Claude Code, whose custom statuslines are what would justify
// a wider window, is detected from its pane title instead.
const contentSpinnerScanLines = 10

// contentShowsSpinner reports whether the trailing pane content contains a
// verified working indicator. Oh My Pi renders a Braille spinner as the first
// visible character of an ordinary line (for example, "⠦ Creating Claude
// config guide ⟨esc⟩"). OpenCode keeps its spinner inside a bordered command
// row, but renders "esc interrupt" in the active-task footer. Neither tool puts
// its working signal in the tmux title, so title-only detection would leave
// both looking idle while they work.
func contentShowsSpinner(tool string, lines []string) bool {
	tail := lines
	if len(tail) > contentSpinnerScanLines {
		tail = tail[len(tail)-contentSpinnerScanLines:]
	}
	for _, l := range tail {
		plain := strings.TrimSpace(stripANSI(l))
		if plain == "" {
			continue
		}
		// Verified live against OpenCode 1.18.5. Keep the full two-word
		// footer marker and scope it to OpenCode: either word alone is
		// ordinary terminal content, including in other agent panes.
		if tool == "opencode" && strings.Contains(strings.ToLower(plain), "esc interrupt") {
			return true
		}
		if isSpinnerGlyph([]rune(plain)[0]) {
			return true
		}
	}
	return false
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
