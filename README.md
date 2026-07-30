# amux — AI agent tracker for tmux

**See every AI coding agent you have running — at a glance, in one place.**

`amux` is a lightweight, fast TUI that unifies two things no other tool shows
together:

1. **Interactive agents** — `claude`, `omp` ([Oh My Pi](https://www.npmjs.com/package/@oh-my-pi/pi-coding-agent)),
   `codex`, `aider`, `gemini` running in the foreground of any tmux pane,
   across every session.
2. **Background agents** — headless daemons that have **no tmux pane at all**
   and are otherwise invisible: Claude Code jobs started with the `agents`
   command, and Oh My Pi's own named background tasks.

Both are grouped by the **project** they belong to, so a background agent shows
up right next to the interactive session that spawned it.

It's the agent-centric companion to [`lunemis/tap/mux`](https://github.com/lunemis/mux):
where mux is a tmux *session switcher* with AI badges, amux is an *agent
dashboard* that also surfaces the headless daemons mux can't see.

```
amux  agent tracker                                          ▶ 4 working  ·  ✓ 25 done
tmux_agent_check ▶3                          │ ▶ working  claude-cf5d445b:0.0
❯ ▶ ▤ claude-cf5d445b:0.0            4%cpu    │
  ▶ ▤ claude_all_-cf5d445b:0.0       4%cpu    │ kind      interactive (tmux pane)
  ▶ ◇ 82ea9a49              opus[1m] · 1m     │ tool      claude
english_featfree ▶1                          │ project   /Volumes/SDWORK/git/util/…
  ▶ ▤ claude-370c3804:0.0            1%cpu    │ pane      claude-cf5d445b:0.0
                                             │ status
                                             │ ⠐ claude
j/k move · enter attach · / filter · d +25 done · r refresh · q quit    updated 12:05:16
```

`▤` = interactive tmux pane · `◇` = headless background daemon
`▶` working · `◆` input needed · `●` idle · `⚠` stale · `■` stopped · `✓` done

## Why

In an AI-heavy workflow you might have Claude working in one pane, a background
agent committing in another repo, and three more finished jobs you forgot about.
tmux's `choose-session` only lists names, and the built-in `agents` view is
per-invocation. amux gives you the **global, cross-session** picture: what's
working right now, what's waiting on you, and what's done — grouped by project.

## Features

- **Both agent kinds, one view** — tmux panes *and* headless daemons
  (`~/.claude/jobs`, `~/.omp/run/daemons`).
- **Project grouping** — agents bucket under their git repo root.
- **Accurate liveness** — background agents are checked against their daemon's
  live process/socket, not a timestamp, so a long-running agent never looks
  dead and a crashed one never looks alive.
- **Working vs. idle, not just "alive"** — an interactive pane isn't just
  "live" or not: amux reads the spinner glyph Claude Code/Oh My Pi write into
  the pane title while actively generating, so you can tell a busy agent from
  one that's alive but sitting at a prompt waiting for you.
- **Knows when an agent is waiting on *you*** — a distinct `◆ input` state for
  an agent that has asked a question (e.g. Claude Code's AskUserQuestion) and
  is blocked on your choice, detected from the pane's actual content, not
  guessed from the title.
- **Live preview, in color** — the detail panel shows the selected agent's
  real terminal output with its own colors intact, refreshed every cycle,
  mux-style — not just a flat one-line status.
- **Reply without switching panes** — press `c` on an interactive agent to
  type a message and send it straight into that pane (types it, then presses
  Enter for you), so you can answer a pending question from the dashboard.
  Interactive only — there's no way to message a headless background job from
  outside `claude agents` itself.
- **Active-first** — finished agents are folded away by default (`d` reveals
  them); the count is always shown, never silently dropped.
- **Your own notes, local to amux** — press `i` to jot a short note on any
  agent (why it's running, what you're waiting on); saved to disk, shown in
  the list and detail panel, never written back to tmux/Claude/omp.
- **Live detail** — model, tokens, CPU, age, and the original task for the
  selected agent.
- **Attach in a keystroke** — `Enter` switches to an interactive agent's pane;
  for a background agent it prints the `claude --resume` command.
- **Statusbar widget** — `amux status` prints a compact `▶2 ◆1` summary.
- **Popup mode** — summon amux as a floating overlay over any session.

## Scope

amux tracks **AI agents**: `claude` and `omp` (interactive and background),
plus `codex`/`aider`/`gemini` when they run interactively in a pane. Ordinary
shells, editors, and prompt-theme sessions (e.g. an idle `oh-my-posh` shell
with no agent running in it) are intentionally **not** listed — they're
environment, not agents. Model/token detail on background agents is currently
Claude-specific; omp background tasks show name and state only.

> Note: `omp` here is [Oh My Pi](https://www.npmjs.com/package/@oh-my-pi/pi-coding-agent),
> a full coding-agent CLI — unrelated to the `oh-my-posh` shell prompt theme
> despite the name collision. If your tmux session naming makes that ambiguous,
> `amux`'s `tool` field in the detail panel always disambiguates.

## Install

> **Note:** the Go module path (`github.com/kyhan/amux`) is a placeholder. Point
> it at your own repo before publishing a Homebrew tap.

```bash
# From source
git clone <your-repo> amux && cd amux
make install            # builds and installs to /usr/local/bin

# Or directly
go build -o amux ./cmd/amux
```

## Usage

```bash
amux                    # open the dashboard
amux popup              # open as a floating tmux popup (inside tmux)
amux status             # compact summary for the tmux status bar
amux setup-keybind      # bind <prefix> a to the popup (writes ~/.tmux.conf)
```

### Keybindings

| Key | Action |
|---|---|
| `j` / `k` | Move down / up |
| `g` / `G` | Jump to first / last |
| `Enter` | Attach to the selected agent's pane (or print resume cmd) |
| `c` | Compose a message and send it into an interactive agent's pane |
| `/` | Filter by name, project, model, or path |
| `i` | Add / edit a note on the selected agent |
| `d` | Show / hide finished agents |
| `r` | Refresh now |
| `q` / `x` | Quit |

### tmux status bar

```tmux
set -g status-right '#(amux status)'
```

## How it works

Each 2-second refresh does exactly one `ps`, one `tmux list-panes`, one
`tmux capture-pane` per detected AI-tool pane, and a handful of bounded
directory scans/globs — no background daemon of its own. Interactive agents
are found by walking each pane's process tree for an AI-tool foreground
process; the captured pane content doubles as the live preview and as the
signal for the `◆ input` state (matched against Claude Code's own
AskUserQuestion-style choice-prompt footer — verified against a real pending
prompt, not guessed, though other tools' or dialogs' wording may not be
caught). Background agents are read straight from each source's own on-disk
state:

- Claude Code: `~/.claude/jobs/*/state.json`; liveness from the daemon's
  `/tmp/cc-daemon-*/*/rv/<id>.sock` rendezvous socket.
- Oh My Pi: `~/.omp/run/daemons/*/daemons/*/meta.json`; liveness from the
  owning client's pid in the sibling `clients/*.json` registry.

Notes you add with `i` live in their own file (`$XDG_CONFIG_HOME/amux/notes.json`,
or the platform equivalent via `os.UserConfigDir()`), keyed by agent ID and
pruned automatically once that agent's run ends — amux never touches any other
tool's files.

## Requirements

- tmux (popup mode needs 3.2+)
- Go 1.24+ to build
- macOS or Linux

## License

MIT
