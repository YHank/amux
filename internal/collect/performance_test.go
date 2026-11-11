package collect

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

// Live benchmarks are opt-in because they inspect the machine's current agents.
func BenchmarkCollectLive(b *testing.B) {
	if os.Getenv("AMUX_LIVE_BENCH") == "" {
		b.Skip("set AMUX_LIVE_BENCH=1 to measure the current tmux server")
	}
	b.ReportAllocs()
	var snap Snapshot
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var err error
		snap, err = Collect(time.Now())
		if err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(len(snap.Agents)), "agents/op")
}

func BenchmarkCollectionStages(b *testing.B) {
	if os.Getenv("AMUX_LIVE_BENCH") == "" {
		b.Skip("set AMUX_LIVE_BENCH=1 to measure the current tmux server")
	}
	procs, err := snapshotProcs()
	if err != nil {
		b.Fatal(err)
	}
	panes, err := listPanes()
	if err != nil {
		b.Fatal(err)
	}
	agents := interactiveAgents(procs, panes)
	psPath, err := exec.LookPath("ps")
	if err != nil {
		b.Fatal(err)
	}
	stages := []struct {
		name string
		run  func() error
	}{
		{"ps", func() error { _, err := snapshotProcs(); return err }},
		{"ps-command", func() error {
			_, err := exec.Command(psPath, "-Awwo", "pid=,ppid=,%cpu=,etime=,comm=,args=").Output()
			return err
		}},
		{"lookup-ps", func() error { _, err := exec.LookPath("ps"); return err }},
		{"lookup-tmux", func() error { _, err := exec.LookPath("tmux"); return err }},
		{"project-key", func() error {
			for _, a := range agents {
				projectKey(a.CWD)
			}
			return nil
		}},
		{"list-panes", func() error { _, err := listPanes(); return err }},
		{"interactive", func() error { interactiveAgents(procs, panes); return nil }},
		{"capture", func() error {
			for _, a := range agents {
				capturePane(a.TmuxTarget, previewLines)
			}
			return nil
		}},
		{"background", func() error {
			if _, err := backgroundAgents(liveShorts()); err != nil {
				return err
			}
			_, err := ompBackgroundAgents()
			return err
		}},
		{"group", func() error { group(agents); return nil }},
	}
	for _, stage := range stages {
		b.Run(stage.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if err := stage.run(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
