package notegen

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/kyhan/amux/internal/collect"
	"github.com/kyhan/amux/internal/notes"
)

func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == "note-worker" {
		err := Work(context.Background(), os.Stdin)
		if done := os.Getenv("AMUX_WORKER_DONE"); done != "" {
			file, openErr := os.OpenFile(done, os.O_WRONLY, 0)
			if openErr != nil {
				os.Exit(2)
			}
			fmt.Fprintln(file, err)
			file.Close()
		}
		if err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestDetachedParentProcess(t *testing.T) {
	raw := os.Getenv("AMUX_PARENT_REQUEST")
	if raw == "" {
		return
	}
	var request Request
	if err := json.Unmarshal([]byte(raw), &request); err != nil {
		t.Fatal(err)
	}
	cmd, err := Start(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Process.Release(); err != nil {
		t.Fatal(err)
	}
	// Exit the real launching process without running defers.
	os.Exit(0)
}

func TestDetachedCompletionAfterActualParentExit(t *testing.T) {
	// Given: exact FIFO events gate the provider and signal persisted completion.
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	pipes := make(map[string]*os.File)
	for _, name := range []string{"ready", "gate", "done"} {
		path := filepath.Join(dir, name)
		if err := syscall.Mkfifo(path, 0o600); err != nil {
			t.Fatal(err)
		}
		file, err := os.OpenFile(path, os.O_RDWR, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		pipes[name] = file
	}
	t.Setenv("AMUX_WORKER_DONE", filepath.Join(dir, "done"))
	t.Setenv("AMUX_READY", filepath.Join(dir, "ready"))
	t.Setenv("AMUX_GATE", filepath.Join(dir, "gate"))
	script := "#!/bin/sh\ncat >/dev/null\nprintf R > \"$AMUX_READY\"\nread token < \"$AMUX_GATE\"\nprintf 'DETACHED_RESULT\\n'\n"
	if err := os.WriteFile(filepath.Join(dir, "codex"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	store := notes.Load()
	token, err := store.Begin("agent")
	if err != nil {
		t.Fatal(err)
	}
	request, err := json.Marshal(Request{StorePath: store.Path(), Agent: collect.Agent{ID: "agent"}, Token: token})
	if err != nil {
		t.Fatal(err)
	}
	ready, done := make(chan error, 1), make(chan error, 1)
	go func() { _, err := io.ReadFull(pipes["ready"], make([]byte, 1)); ready <- err }()
	go func() { _, err := io.ReadFull(pipes["done"], make([]byte, 1)); done <- err }()
	parent := exec.Command(os.Args[0], "-test.run=^TestDetachedParentProcess$")
	parent.Env = append(os.Environ(), "AMUX_PARENT_REQUEST="+string(request))
	// When: the parent exits before the provider is released.
	if out, err := parent.CombinedOutput(); err != nil {
		t.Fatalf("parent exit: %v %s", err, out)
	}
	awaitWorkerEvent(t, ready)
	if store.Get("agent") != "" {
		t.Fatal("provider completed before gate release")
	}
	if _, err := pipes["gate"].WriteString("release\n"); err != nil {
		t.Fatal(err)
	}
	awaitWorkerEvent(t, done)
	// Then: a fresh process's store reads the detached result.
	if got := notes.Load().Get("agent"); got != "DETACHED_RESULT" {
		t.Fatalf("detached result=%q", got)
	}
}

func awaitWorkerEvent(t *testing.T, event <-chan error) {
	t.Helper()
	select {
	case err := <-event:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("worker event deadline exceeded")
	}
}
