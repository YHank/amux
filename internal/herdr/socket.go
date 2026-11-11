package herdr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"time"
)

// RPCError is the server's structured error response.
type RPCError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("herdr: %s: %s", e.Code, e.Message)
}

// Call sends one newline-delimited JSON request and returns its raw result.
// An explicit session resolves its running socket from session list, ignoring
// inherited sockets. An empty session uses HERDR_SOCKET_PATH, then the default
// running session. Nil params are encoded as an empty object.
func Call(ctx context.Context, session, method string, params any) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	path, err := socketPath(ctx, session)
	if err != nil {
		return nil, err
	}
	if params == nil {
		params = struct{}{}
	}
	request, err := json.Marshal(struct {
		ID     string `json:"id"`
		Method string `json:"method"`
		Params any    `json:"params"`
	}{"amux", method, params})
	if err != nil {
		return nil, fmt.Errorf("herdr %s request: %w", method, err)
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", path)
	if err != nil {
		return nil, fmt.Errorf("herdr %s connect: %w", method, err)
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() {
		conn.Close()
	})
	defer stop()
	deadline, _ := ctx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		return nil, fmt.Errorf("herdr %s deadline: %w", method, err)
	}
	if _, err := conn.Write(append(request, '\n')); err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return nil, fmt.Errorf("herdr %s write: %w", method, err)
	}
	var response struct {
		ID     string          `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  *RPCError       `json:"error"`
	}
	// Bound untrusted socket responses independently of the time limit.
	if err := json.NewDecoder(io.LimitReader(conn, 16<<20)).Decode(&response); err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return nil, fmt.Errorf("herdr %s response: %w", method, err)
	}
	if response.ID != "amux" {
		return nil, fmt.Errorf("herdr %s: response id %q does not match request", method, response.ID)
	}
	if response.Error != nil {
		return nil, response.Error
	}
	if len(response.Result) == 0 {
		return nil, fmt.Errorf("herdr %s: missing result", method)
	}
	return response.Result, nil
}

func socketPath(ctx context.Context, session string) (string, error) {
	if session == "" {
		if path := os.Getenv("HERDR_SOCKET_PATH"); path != "" {
			return path, nil
		}
	}
	out, err := Run(ctx, "", "session", "list", "--json")
	if err != nil {
		return "", err
	}
	var inventory struct {
		Sessions []struct {
			Name       string `json:"name"`
			Running    bool   `json:"running"`
			Default    bool   `json:"default"`
			SocketPath string `json:"socket_path"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal(out, &inventory); err != nil {
		return "", fmt.Errorf("herdr socket inventory: %w", err)
	}
	for _, candidate := range inventory.Sessions {
		if (session == candidate.Name || (session == "" && candidate.Default)) && candidate.Running && candidate.SocketPath != "" {
			return candidate.SocketPath, nil
		}
	}
	return "", fmt.Errorf("herdr session %q: %w", session, errors.New("no running session socket"))
}
