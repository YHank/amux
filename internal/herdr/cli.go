// Package herdr provides bounded access to the installed Herdr CLI.
package herdr

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Args prefixes a named session. An empty session uses Herdr's default context,
// allowing global commands such as session list.
func Args(session string, args ...string) []string {
	out := make([]string, 0, len(args)+2)
	if session != "" {
		out = append(out, "--session", session)
	}
	return append(out, args...)
}

// Run preserves raw stdout (including ANSI), includes stderr on failure, and
// honors the earlier of the caller's deadline and a three-second command limit.
func Run(ctx context.Context, session string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "herdr", Args(session, args...)...)
	if session != "" {
		for _, variable := range os.Environ() {
			if strings.HasPrefix(variable, "HERDR_SOCKET=") || strings.HasPrefix(variable, "HERDR_SOCKET_PATH=") {
				continue
			}
			cmd.Env = append(cmd.Env, variable)
		}
	}
	cmd.WaitDelay = 100 * time.Millisecond
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return nil, fmt.Errorf("herdr session %q %s: %w: %s", session, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}
