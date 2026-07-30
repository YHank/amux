package collect

import (
	"path/filepath"
	"strings"
)

// liveShorts returns the set of daemonShort ids that currently have a live
// rendezvous socket under the Claude Code daemon runtime dir.
//
// The daemon keeps one socket per running session at
// /tmp/cc-daemon-<uid>/<instance>/rv/<daemonShort>.sock. Its presence means the
// daemon process for that session is alive — regardless of whether a terminal
// is attached, and regardless of how long ago the session last wrote state.json.
// This is the authoritative liveness signal: it correctly keeps a long-running
// "working" agent live even when its state.json hasn't changed in minutes, and
// correctly flags a "working" job whose daemon has since died.
func liveShorts() map[string]bool {
	live := map[string]bool{}
	// Glob is cheap (a handful of entries); no per-session syscalls beyond it.
	matches, err := filepath.Glob("/tmp/cc-daemon-*/*/rv/*.sock")
	if err != nil {
		return live
	}
	for _, m := range matches {
		short := strings.TrimSuffix(filepath.Base(m), ".sock")
		if short != "" {
			live[short] = true
		}
	}
	return live
}
