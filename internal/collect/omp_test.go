package collect

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestOmpNormalizeLife(t *testing.T) {
	cases := []struct {
		state string
		live  bool
		want  Lifecycle
	}{
		{"exited", false, LifeDone},
		{"stopped", false, LifeDone},
		{"failed", false, LifeError},
		{"running", true, LifeWorking},
		{"running", false, LifeStale}, // owning client died mid-run
		{"starting", true, LifeWorking},
		{"weird", true, LifeUnknown},
	}
	for _, c := range cases {
		if got := ompNormalizeLife(c.state, c.live); got != c.want {
			t.Errorf("ompNormalizeLife(%q, %v) = %q, want %q", c.state, c.live, got, c.want)
		}
	}
}

func TestOmpTaskLiveRequiresRunningTaskAndLiveOwner(t *testing.T) {
	for _, c := range []struct {
		state     string
		ownerLive bool
		want      bool
	}{
		{"running", true, true},
		{"starting", true, true},
		{"exited", true, false},
		{"failed", true, false},
		{"running", false, false},
	} {
		if got := ompTaskLive(c.state, c.ownerLive); got != c.want {
			t.Errorf("ompTaskLive(%q, %v) = %v, want %v", c.state, c.ownerLive, got, c.want)
		}
	}
}

func TestMsToTime(t *testing.T) {
	if !msToTime(0).IsZero() {
		t.Error("msToTime(0) should be zero")
	}
	if !msToTime(-1).IsZero() {
		t.Error("msToTime(negative) should be zero")
	}
	if msToTime(1_700_000_000_000).IsZero() {
		t.Error("msToTime(positive) should not be zero")
	}
}

func TestOmpTaskToAgentDetail(t *testing.T) {
	code := 1
	var m ompDaemonMeta
	m.Daemon.Name = "stage-color-sync"
	m.Daemon.ID = "abc-123"
	m.Daemon.State = "failed"
	m.Daemon.ExitCode = &code
	m.Daemon.CreatedAt = 1_700_000_000_000

	a := ompTaskToAgent(m, "/nonexistent/hash/dir")
	if a.Tool != "omp" || a.Kind != KindBackground {
		t.Errorf("got tool=%q kind=%q", a.Tool, a.Kind)
	}
	if a.Life != LifeError {
		t.Errorf("got life=%q, want error", a.Life)
	}
	if a.ID != "omp-task:abc-123" {
		t.Errorf("got id=%q", a.ID)
	}
	if a.Detail != "state: failed (exit 1)" {
		t.Errorf("got detail=%q", a.Detail)
	}
}

func TestOmpTaskToAgentCarriesDaemonLiveness(t *testing.T) {
	hashDir := t.TempDir()
	clients := filepath.Join(hashDir, "clients")
	if err := os.Mkdir(clients, 0o755); err != nil {
		t.Fatal(err)
	}
	client := filepath.Join(clients, "live.json")
	if err := os.WriteFile(client, []byte(`{"pid":`+strconv.Itoa(os.Getpid())+`,"projectDir":"/project"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var m ompDaemonMeta
	m.Daemon.ID = "live-task"
	m.Daemon.State = "running"
	if a := ompTaskToAgent(m, hashDir); !a.Live || a.Life != LifeWorking {
		t.Fatalf("live OMP task = %#v", a)
	}

	if err := os.WriteFile(client, []byte(`{"pid":-1,"projectDir":"/project"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if a := ompTaskToAgent(m, hashDir); a.Live || a.Life != LifeStale {
		t.Fatalf("dead OMP task = %#v", a)
	}
}
