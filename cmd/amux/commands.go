package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/kyhan/amux/internal/collect"
)

// popupCmd opens amux as a floating tmux popup over the current session.
func popupCmd() *cobra.Command {
	var width, height string
	cmd := &cobra.Command{
		Use:   "popup",
		Short: "Open amux as a tmux popup overlay",
		RunE: func(cmd *cobra.Command, args []string) error {
			if os.Getenv("TMUX") == "" {
				return fmt.Errorf("popup requires running inside tmux")
			}
			self, err := os.Executable()
			if err != nil {
				self = "amux"
			}
			// Capture the client that is opening the popup so the eventual
			// switch-client targets it and not the popup's throwaway client.
			inner := shellQuote(self)
			if client := currentClient(); client != "" {
				inner = "AMUX_CLIENT=" + shellQuote(client) + " " + inner
			}
			return runInherit("tmux", "display-popup", "-w", width, "-h", height, "-E", inner)
		},
	}
	cmd.Flags().StringVarP(&width, "width", "w", "85%", "popup width")
	cmd.Flags().StringVarP(&height, "height", "H", "85%", "popup height")
	return cmd
}

// statusCmd prints a compact, tmux-colored summary for the status bar.
func statusCmd() *cobra.Command {
	var plain bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Print a one-line AI agent summary for the tmux status bar",
		RunE: func(cmd *cobra.Command, args []string) error {
			snap, err := collect.Collect(time.Now())
			if err != nil {
				return nil // never break a status bar
			}
			fmt.Print(statusLine(snap, plain))
			return nil
		},
	}
	cmd.Flags().BoolVar(&plain, "plain", false, "no tmux color codes")
	return cmd
}

// statusLine renders "▶2 ◆1 ●3" for active agents, colored for tmux unless plain.
func statusLine(snap collect.Snapshot, plain bool) string {
	c := snap.Counts()
	order := []struct {
		life  collect.Lifecycle
		glyph string
		color string
	}{
		{collect.LifeInput, "◆", "colour214"},
		{collect.LifeWorking, "▶", "colour42"},
		{collect.LifeIdle, "●", "colour39"},
		{collect.LifeStale, "⚠", "colour208"},
	}
	var segs []string
	for _, o := range order {
		if c[o.life] == 0 {
			continue
		}
		seg := fmt.Sprintf("%s%d", o.glyph, c[o.life])
		if !plain {
			seg = fmt.Sprintf("#[fg=%s]%s#[default]", o.color, seg)
		}
		segs = append(segs, seg)
	}
	return strings.Join(segs, " ")
}

// setupKeybindCmd appends a popup keybinding to ~/.tmux.conf.
func setupKeybindCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup-keybind [key]",
		Short: "Add a tmux popup keybinding to ~/.tmux.conf (default: a)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := "a"
			if len(args) == 1 {
				key = args[0]
			}
			return installKeybind(key)
		},
	}
}

// keybindMarker prefixes the comment line amux writes above its own
// bind-key line in ~/.tmux.conf. It's the stable identifier used to find and
// replace a previous amux binding on a later setup-keybind run — matching on
// it alone (not also on the executable path) so re-running after amux was
// moved or upgraded updates the existing line in place instead of leaving a
// stale, unmatched one behind and appending a duplicate next to it.
const keybindMarker = "# amux-keybind"

func installKeybind(key string) error {
	if !validKeySpec(key) {
		return fmt.Errorf("invalid key %q: must not contain whitespace, quotes, backslashes, or ;", key)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	conf := filepath.Join(home, ".tmux.conf")
	self, err := os.Executable()
	if err != nil {
		self = "amux"
	}
	// display-popup -e sets AMUX_CLIENT in the popup, format-expanded by tmux
	// against the client that pressed the key — the correct outer client to
	// switch when the user attaches from within the popup. self is double-
	// quoted via tmuxDQuote, not wrapped in literal single quotes: tmux's
	// config lexer has no escape for a single quote inside a single-quoted
	// token, so a path containing one (e.g. an account named "o'brien")
	// would end the quote early and corrupt the rest of the file — verified
	// live against a real tmux instance, where exactly that shape spilled
	// into and swallowed the following config line.
	line := fmt.Sprintf("bind-key %s display-popup -w 85%% -h 85%% -e AMUX_CLIENT='#{client_name}' -E %s",
		key, tmuxDQuote(self))
	block := keybindMarker + " — AI agent tracker popup\n" + line + "\n"

	if existing, err := os.ReadFile(conf); err == nil {
		if updated, replaced := replaceMarkedBlock(string(existing), keybindMarker, block); replaced {
			if err := os.WriteFile(conf, []byte(updated), 0o644); err != nil {
				return err
			}
			fmt.Printf("Updated %s:\n  %s\n\nReload with:  tmux source-file %s\nThen press:   <prefix> %s\n", conf, line, conf, key)
			return nil
		}
	}

	f, err := os.OpenFile(conf, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	fmt.Fprintf(w, "\n%s", block)
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Printf("Added to %s:\n  %s\n\nReload with:  tmux source-file %s\nThen press:   <prefix> %s\n", conf, line, conf, key)
	return nil
}

// replaceMarkedBlock finds the line starting with marker in content and
// replaces it and the line right after it (the marker's own comment + the
// bind-key line it describes) with block. Reports false, unchanged, if the
// marker isn't present, so the caller falls back to appending.
func replaceMarkedBlock(content, marker, block string) (string, bool) {
	lines := strings.Split(content, "\n")
	for i, l := range lines {
		if !strings.HasPrefix(l, marker) {
			continue
		}
		end := i + 2 // the marker/comment line plus the bind-key line after it
		if end > len(lines) {
			end = len(lines)
		}
		newLines := strings.Split(strings.TrimRight(block, "\n"), "\n")
		result := append([]string{}, lines[:i]...)
		result = append(result, newLines...)
		result = append(result, lines[end:]...)
		return strings.Join(result, "\n"), true
	}
	return content, false
}

// validKeySpec reports whether key is safe to splice, unquoted, into a
// tmux.conf bind-key line as the key token. A real tmux key name never
// contains whitespace, quote characters, a backslash, or `;` (tmux's command
// separator) — rejecting those outright is simpler and safer than trying to
// quote a token that isn't supposed to be a quoted string in the first place.
func validKeySpec(key string) bool {
	return key != "" && !strings.ContainsAny(key, " \t\n\r'\"\\;")
}

// tmuxDQuote wraps s in tmux config-file double quotes, escaping backslashes
// and double quotes the way tmux's own config lexer expects (backslash-escape,
// same as a POSIX shell double-quoted string). Verified live against a real
// tmux instance for embedded ', ", \, and all three combined — tmux's
// single-quoted strings have no escape for an embedded single quote at all,
// so a single-quote-wrapped value containing one is not just wrong but
// actively corrupts the rest of the config file (confirmed: it swallowed the
// following line whole). Double-quoting sidesteps that entirely.
func tmuxDQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

// currentClient returns the name of the tmux client in the current context, or
// "" if none can be determined. Used to tag a popup with its invoking client.
func currentClient() string {
	out, err := exec.Command("tmux", "display-message", "-p", "#{client_name}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// shellQuote wraps s in single quotes for safe use inside a `sh -c` string.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
