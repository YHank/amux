package collect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSnapshotProcsAcceptsLongArgumentsWithinLimit(t *testing.T) {
	for _, tc := range []struct {
		name string
		size int
	}{{"grows buffer", 128 * 1024}, {"over limit", 2 * 1024 * 1024}} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("PATH", dir)
			args := "codex " + strings.Repeat("a", tc.size)
			script := "#!/bin/sh\nprintf '%s\\n' '100 1 0.1 00:10 codex " + args + "'\n"
			if err := os.WriteFile(filepath.Join(dir, "ps"), []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}

			procs, err := snapshotProcs()

			if tc.size > 1024*1024 {
				if err == nil {
					t.Fatal("process records above the scanner limit must return an error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := procs.byPID[100].args; got != args {
				t.Fatalf("long arguments truncated: got %d bytes, want %d", len(got), len(args))
			}
		})
	}
}
