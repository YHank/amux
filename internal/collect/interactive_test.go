package collect

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestInteractiveAgentsCaptureConcurrentlyAndPreserveOwnership(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var mu sync.Mutex
	started := 0
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		started++
		if started == 4 {
			close(release)
		}
		mu.Unlock()
		select {
		case <-release:
			fmt.Fprintln(w, r.URL.Query().Get("target"))
		case <-ctx.Done():
			http.Error(w, "captures did not overlap", http.StatusGatewayTimeout)
		}
	}))
	defer server.Close()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("AMUX_CAPTURE_TEST_BINARY", executable)
	t.Setenv("AMUX_CAPTURE_TEST_URL", server.URL)
	t.Setenv("AMUX_CAPTURE_TEST_HELPER", "1")
	t.Setenv("PATH", dir)
	script := "#!/bin/sh\nexec \"$AMUX_CAPTURE_TEST_BINARY\" -test.run '^TestCaptureProcessHelper$' -- \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	procs := &procTable{byPID: map[int]proc{}}
	var panes []pane
	for i := 0; i < 5; i++ {
		target := fmt.Sprintf("pane-%d", i)
		if i == 4 {
			target = "closed"
		}
		procs.byPID[i+1] = proc{pid: i + 1, comm: "codex"}
		panes = append(panes, pane{pid: i + 1, paneID: fmt.Sprintf("%%%d", i+1), target: target, path: dir})
	}
	panes = append(panes, pane{pid: 99, target: "not-an-agent"})

	agents := interactiveAgents(procs, panes)

	if ctx.Err() != nil {
		t.Fatal("four pane captures must overlap instead of waiting sequentially")
	}
	if len(agents) != 5 {
		t.Fatalf("got %d agents, want 5 including the closed capture", len(agents))
	}
	for i, agent := range agents {
		if agent.ID != panes[i].paneID || agent.TmuxTarget != panes[i].target {
			t.Fatalf("pane ownership/order changed at %d: %+v", i, agent)
		}
		if i < 4 && strings.Join(agent.Preview, "\n") != panes[i].target {
			t.Fatalf("preview belongs to another pane: %+v", agent)
		}
		if i == 4 && len(agent.Preview) != 0 {
			t.Fatalf("failed capture must leave an empty preview: %v", agent.Preview)
		}
	}
}

func TestCaptureProcessHelper(t *testing.T) {
	if os.Getenv("AMUX_CAPTURE_TEST_HELPER") != "1" {
		return
	}
	target := ""
	for i, arg := range os.Args {
		if arg == "-t" && i+1 < len(os.Args) {
			target = os.Args[i+1]
		}
	}
	if target == "closed" {
		os.Exit(1)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Get(os.Getenv("AMUX_CAPTURE_TEST_URL") + "?target=" + url.QueryEscape(target))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		if err := response.Body.Close(); err != nil {
			t.Fatal(err)
		}
		os.Exit(1)
	}
	if _, err := io.Copy(os.Stdout, response.Body); err != nil {
		t.Fatal(err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	os.Exit(0)
}
