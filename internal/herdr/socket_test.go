package herdr

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func socketFixture(t *testing.T, handler func(net.Conn)) string {
	t.Helper()
	// macOS Unix socket paths cannot fit the long test-name TempDir prefix.
	dir, err := os.MkdirTemp("", "h-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Error(err)
		}
	})
	path := filepath.Join(dir, "rpc")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		handler(conn)
	}()
	t.Cleanup(func() {
		if err := listener.Close(); err != nil {
			t.Error(err)
		}
		<-done
	})
	return path
}

func TestCallResolvesSessionAndReturnsRawResult(t *testing.T) {
	for _, session := range []string{"", "named"} {
		t.Run(session, func(t *testing.T) {
			path := socketFixture(t, func(conn net.Conn) {
				line, err := bufio.NewReader(conn).ReadBytes('\n')
				if err != nil {
					t.Error(err)
					return
				}
				var request struct {
					ID     string          `json:"id"`
					Method string          `json:"method"`
					Params json.RawMessage `json:"params"`
				}
				if err := json.Unmarshal(line, &request); err != nil {
					t.Error(err)
					return
				}
				if request.Method != "session.snapshot" || string(request.Params) != "{}" {
					t.Errorf("invalid wire request: %s", line)
					return
				}
				if _, err := fmt.Fprintf(conn, "{\"id\":%q,\"result\":{\"snapshot\":{\"panes\":[]}}}\n", request.ID); err != nil {
					t.Error(err)
				}
			})
			if session == "" {
				t.Setenv("HERDR_SOCKET_PATH", path)
				t.Setenv("PATH", t.TempDir()) // inherited sockets need no CLI.
			} else {
				t.Setenv("HERDR_SOCKET_PATH", "/wrong/inherited/socket")
				t.Setenv("AMUX_TEST_SOCKET", path)
				installCLI(t, `printf '{"sessions":[{"name":"named","running":true,"socket_path":"%s"}]}\n' "$AMUX_TEST_SOCKET"`+"\n")
			}

			result, err := Call(context.Background(), session, "session.snapshot", nil)

			if err != nil || string(result) != `{"snapshot":{"panes":[]}}` {
				t.Fatalf("Call = %s, %v", result, err)
			}
		})
	}
}

func TestCallRejectsWireErrors(t *testing.T) {
	for _, tc := range []struct {
		name     string
		response string
		code     string
	}{
		{"server error", `{"id":"amux","error":{"code":"not_found","message":"missing pane"}}`, "not_found"},
		{"mismatched id", `{"id":"other","result":{}}`, ""},
		{"missing result", `{"id":"amux"}`, ""},
		{"malformed", `{broken`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := socketFixture(t, func(conn net.Conn) {
				if _, err := bufio.NewReader(conn).ReadBytes('\n'); err != nil {
					t.Error(err)
					return
				}
				if _, err := fmt.Fprintln(conn, tc.response); err != nil {
					t.Error(err)
				}
			})
			t.Setenv("HERDR_SOCKET_PATH", path)

			_, err := Call(context.Background(), "", "pane.focus", map[string]string{"pane_id": "w1:p1"})

			if err == nil {
				t.Fatal("invalid response accepted")
			}
			if tc.code != "" {
				var rpcErr *RPCError
				if !errors.As(err, &rpcErr) || rpcErr.Code != tc.code || rpcErr.Message != "missing pane" {
					t.Fatalf("structured server error lost: %v", err)
				}
			}
		})
	}
}

func TestCallCancellationInterruptsSocketRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	path := socketFixture(t, func(conn net.Conn) {
		if _, err := bufio.NewReader(conn).ReadBytes('\n'); err != nil {
			t.Error(err)
			return
		}
		cancel()
		var b [1]byte
		if _, err := conn.Read(b[:]); err == nil {
			t.Error("cancelled client kept connection open")
		}
	})
	t.Setenv("HERDR_SOCKET_PATH", path)

	_, err := Call(ctx, "", "session.snapshot", nil)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("socket cancellation lost: %v", err)
	}
}
