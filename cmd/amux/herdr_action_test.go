package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/kyhan/amux/internal/ui"
)

// Assert the Herdr wire contract: fullscreen selects the captured pane rather
// than opening a second terminal that merely mirrors it.
func TestHerdrFullscreenFocusesSelectedPane(t *testing.T) {
	for _, popup := range []string{"", "1"} {
		t.Run("popup="+popup, func(t *testing.T) {
			dir, err := os.MkdirTemp("", "amux-rpc-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := os.RemoveAll(dir); err != nil {
					t.Error(err)
				}
			})
			path := filepath.Join(dir, "socket")
			listener, err := net.Listen("unix", path)
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			t.Setenv("HERDR_ENV", "1")
			t.Setenv("AMUX_HERDR_POPUP", popup)
			t.Setenv("HERDR_SOCKET_PATH", path)
			t.Setenv("PATH", t.TempDir())
			done := make(chan error, 1)
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					done <- err
					return
				}
				defer conn.Close()
				var request struct {
					ID     string            `json:"id"`
					Method string            `json:"method"`
					Params map[string]string `json:"params"`
				}
				if err := json.NewDecoder(conn).Decode(&request); err != nil {
					done <- err
					return
				}
				if request.Method != "pane.focus" || request.Params["pane_id"] != "w2:p3" {
					done <- fmt.Errorf("unexpected focus request: %+v", request)
					return
				}
				_, err = fmt.Fprintf(conn, "{\"id\":%q,\"result\":{\"type\":\"pane_info\"}}\n", request.ID)
				done <- err
			}()
			if err := performHerdrAction(&ui.Action{Kind: "attach", PaneID: "w2:p3"}); err != nil {
				t.Fatal(err)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}
