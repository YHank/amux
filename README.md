# amux — AI agent tracker for tmux

**See every AI coding agent you have running — at a glance, in one place.**

`amux` is a lightweight, fast TUI that unifies two things no other tool shows
together:

1. **Interactive agents** — `claude` (and `codex`/`aider`/`gemini`) running in
   the foreground of any tmux pane, across every session.
2. **Background agents** — headless Claude Code daemons started with the
   `agents` command, which have **no tmux pane at all** and are otherwise
   invisible.

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

- **Both agent kinds, one view** — tmux panes *and* `~/.claude/jobs` daemons.
- **Project grouping** — agents bucket under their git repo root.
- **Accurate liveness** — uses the daemon's live rendezvous socket, not a
  timestamp, so a long-running agent never looks dead and a crashed one never
  looks alive.
- **Active-first** — finished agents are folded away by default (`d` reveals
  them); the count is always shown, never silently dropped.
- **Live detail** — model, tokens, CPU, age, status line, and the original task
  for the selected agent.
- **Attach in a keystroke** — `Enter` switches to an interactive agent's pane;
  for a background agent it prints the `claude --resume` command.
- **Statusbar widget** — `amux status` prints a compact `▶2 ◆1` summary.
- **Popup mode** — summon amux as a floating overlay over any session.

## Scope

amux tracks **AI agents** — `claude` (interactive and background) plus
`codex`/`aider`/`gemini` when they run in a pane. Ordinary shells, editors, and
prompt sessions (e.g. `oh-my-posh` panes) are intentionally **not** listed:
they are environment, not agents. Background-agent detail (model, tokens, task)
is Claude-specific today.

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
| `/` | Filter by name, project, model, or path |
| `d` | Show / hide finished agents |
| `r` | Refresh now |
| `q` | Quit |

### tmux status bar

```tmux
set -g status-right '#(amux status)'
```

## How it works

Each 2-second refresh does exactly one `ps`, one `tmux list-panes`, and one scan
of `~/.claude/jobs/` — no background daemon, no polling per pane. Interactive
agents are found by walking each pane's process tree for a `claude`-family
foreground process; background agents are read straight from each job's
`state.json`. Liveness comes from the presence of the daemon's
`/tmp/cc-daemon-*/*/rv/<id>.sock` rendezvous socket.

## Requirements

- tmux (popup mode needs 3.2+)
- Go 1.24+ to build
- macOS or Linux

## License

MIT
