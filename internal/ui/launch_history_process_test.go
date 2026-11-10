package ui

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"
)

func TestLaunchHistoryConcurrentProcesses(t *testing.T) {
	const workers, writes = 8, 32
	if path := os.Getenv("AMUX_LAUNCH_HISTORY_TEST_PATH"); path != "" {
		worker := os.Getenv("AMUX_LAUNCH_HISTORY_TEST_WORKER")
		for i := range writes {
			if err := saveLaunchPath(path, "/worker-"+worker+"/"+strconv.Itoa(i)); err != nil {
				t.Fatal(err)
			}
		}
		return
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "launch-history.json")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	type writer struct {
		cmd    *exec.Cmd
		output bytes.Buffer
	}
	processes := make([]writer, workers)
	for i := range processes {
		p := &processes[i]
		p.cmd = exec.CommandContext(ctx, executable, "-test.run=^TestLaunchHistoryConcurrentProcesses$")
		p.cmd.Env = append(os.Environ(), "AMUX_LAUNCH_HISTORY_TEST_PATH="+path,
			fmt.Sprintf("AMUX_LAUNCH_HISTORY_TEST_WORKER=%d", i))
		p.cmd.Stdout, p.cmd.Stderr = &p.output, &p.output
		if err := p.cmd.Start(); err != nil {
			t.Errorf("start history writer %d: %v", i, err)
		}
	}
	for i := range processes {
		p := &processes[i]
		if p.cmd.Process == nil {
			continue
		}
		if err := p.cmd.Wait(); err != nil {
			t.Errorf("history writer %d: %v: %s", i, err, p.output.String())
		}
	}
	paths, err := loadLaunchHistory(path)
	if err != nil {
		t.Fatal(err)
	}
	want := make([]string, 0, workers*writes)
	for worker := range workers {
		for i := range writes {
			want = append(want, fmt.Sprintf("/worker-%d/%d", worker, i))
		}
	}
	slices.Sort(paths)
	slices.Sort(want)
	if !slices.Equal(paths, want) {
		t.Fatalf("concurrent dashboards lost selected paths: got %d, want %d", len(paths), len(want))
	}
}
