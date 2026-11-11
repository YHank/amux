package herdr

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestArgsSelectsSessionWithoutMutatingCaller(t *testing.T) {
	for _, session := range []string{"", "named session"} {
		t.Run(session, func(t *testing.T) {
			args := make([]string, 2, 6)
			copy(args, []string{"pane", "list"})
			want := []string{"pane", "list"}
			if session != "" {
				want = []string{"--session", session, "pane", "list"}
			}
			got := Args(session, args...)
			got[len(got)-1] = "changed"
			if args[1] != "list" {
				t.Fatal("Args aliases caller's slice")
			}
			got[len(got)-1] = "list"
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("Args = %v, want %v", got, want)
			}
		})
	}
}

func installCLI(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	if err := os.WriteFile(filepath.Join(dir, "herdr"), []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestRunPreservesRawOutputAndSelectsExplicitSession(t *testing.T) {
	t.Setenv("HERDR_SOCKET_PATH", "/inherited/socket")
	t.Setenv("HERDR_SOCKET", "/legacy/socket")
	installCLI(t, `if [ "$1" = --session ] && [ "$2" = "named session" ] && [ -z "$HERDR_SOCKET_PATH" ] && [ -z "$HERDR_SOCKET" ]; then
printf '\033[32mready\033[0m\n'
else
printf 'wrong session context\n' >&2
exit 2
fi
`)

	out, err := Run(context.Background(), "named session", "pane", "list")

	if err != nil || string(out) != "\x1b[32mready\x1b[0m\n" {
		t.Fatalf("Run = %q, %v", out, err)
	}
}

func TestRunReportsStderrAndCancellation(t *testing.T) {
	installCLI(t, "printf 'socket unavailable\\n' >&2\nexit 7\n")
	t.Run("stderr", func(t *testing.T) {
		_, err := Run(context.Background(), "", "api", "snapshot")
		if err == nil || !strings.Contains(err.Error(), "socket unavailable") || !strings.Contains(err.Error(), "exit status 7") {
			t.Fatalf("missing command diagnostics: %v", err)
		}
	})
	t.Run("cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := Run(ctx, "", "api", "snapshot")
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation lost: %v", err)
		}
	})
}
