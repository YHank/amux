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

func installKeybind(key string) error {
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
	// switch when the user attaches from within the popup.
	line := fmt.Sprintf("bind-key %s display-popup -w 85%% -h 85%% -e AMUX_CLIENT='#{client_name}' -E '%s'", key, self)

	if existing, err := os.ReadFile(conf); err == nil {
		if strings.Contains(string(existing), "AMUX_CLIENT") && strings.Contains(string(existing), self) {
			fmt.Println("amux keybinding already present in", conf)
			return nil
		}
	}

	f, err := os.OpenFile(conf, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	fmt.Fprintf(w, "\n# amux — AI agent tracker popup\n%s\n", line)
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Printf("Added to %s:\n  %s\n\nReload with:  tmux source-file %s\nThen press:   <prefix> %s\n", conf, line, conf, key)
	return nil
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
