package notegen

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/kyhan/amux/internal/collect"
	"github.com/kyhan/amux/internal/notes"
)

type Request struct {
	StorePath string        `json:"store_path"`
	Agent     collect.Agent `json:"agent"`
	Model     string        `json:"model"`
	Token     string        `json:"token"`
}

// Start transfers the entire request through an inherited regular file, not a
// pipe owned by the dashboard. Both the input and the new session survive an
// immediate parent exit or closure of the dashboard's controlling terminal.
func Start(request Request) (*exec.Cmd, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	file, err := os.CreateTemp("", "amux-note-request-*")
	if err != nil {
		return nil, err
	}
	defer file.Close()
	defer os.Remove(file.Name())
	if err := json.NewEncoder(file).Encode(request); err != nil {
		return nil, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	cmd := exec.Command(executable, "note-worker")
	cmd.Stdin = file
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start note worker: %w", err)
	}
	return cmd, nil
}

// Work is the hidden CLI worker entrypoint; persistence is owned here, never
// by a Bubble Tea completion message in the launching dashboard.
func Work(ctx context.Context, input io.Reader) error {
	var request Request
	if err := json.NewDecoder(input).Decode(&request); err != nil {
		return fmt.Errorf("read note request: %w", err)
	}
	if request.StorePath == "" || request.Agent.ID == "" || request.Token == "" {
		return fmt.Errorf("incomplete note request")
	}
	store := notes.Open(request.StorePath)
	if state := store.Generation(request.Agent.ID); state.Token != request.Token || state.Done {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	title, generationErr := Title(ctx, request.Agent, request.Model)
	_, err := store.Complete(request.Agent.ID, request.Token, title, generationErr)
	if err != nil {
		return fmt.Errorf("persist generated note: %w", err)
	}
	return generationErr
}
