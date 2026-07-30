package main

import (
	"bufio"
	"fmt"
	"os"
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
			return runInherit("tmux", "display-popup", "-w", width, "-h", height, "-E", self)
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
	line := fmt.Sprintf("bind-key %s run-shell '%s popup'", key, self)

	if existing, err := os.ReadFile(conf); err == nil {
		if strings.Contains(string(existing), self+" popup") {
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
