package collect

import (
	"os"
	"testing"
	"time"
)

func TestLiveProbe(t *testing.T) {
	if os.Getenv("AMUX_LIVE_PROBE") == "" {
		t.Skip("live probe")
	}
	snap, err := Collect(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range snap.Agents {
		if a.Kind != KindInteractive {
			continue
		}
		t.Logf("%-6s %-8s %-8s title=%q", a.ID, a.Tool, a.Life, a.PaneTitle)
	}
}
