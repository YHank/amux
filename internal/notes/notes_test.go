package notes

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSetGetPersistsToDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.json")
	s := &Store{path: path, m: map[string]string{}}

	s.Set("agent-1", "결제 버그 재현 중")
	if got := s.Get("agent-1"); got != "결제 버그 재현 중" {
		t.Fatalf("Get after Set = %q", got)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("notes file not written: %v", err)
	}
	var onDisk map[string]string
	if err := json.Unmarshal(data, &onDisk); err != nil {
		t.Fatalf("notes file not valid json: %v", err)
	}
	if onDisk["agent-1"] != "결제 버그 재현 중" {
		t.Errorf("on-disk value = %q", onDisk["agent-1"])
	}
}

func TestSetEmptyClears(t *testing.T) {
	s := &Store{m: map[string]string{"a": "note"}}
	s.Set("a", "")
	if got := s.Get("a"); got != "" {
		t.Errorf("expected cleared note, got %q", got)
	}
	if _, ok := s.m["a"]; ok {
		t.Error("expected key removed from map, not just emptied")
	}
}

func TestPruneDropsDeadAgents(t *testing.T) {
	s := &Store{m: map[string]string{"live": "keep me", "dead": "drop me"}}
	s.Prune(map[string]bool{"live": true})
	if s.Get("live") != "keep me" {
		t.Error("live agent's note should survive prune")
	}
	if s.Get("dead") != "" {
		t.Error("dead agent's note should be pruned")
	}
}

func TestNilStoreIsSafe(t *testing.T) {
	var s *Store
	if got := s.Get("x"); got != "" {
		t.Errorf("nil store Get = %q, want empty", got)
	}
	// Must not panic.
	s.Set("x", "y")
	s.Prune(map[string]bool{})
}

// TestLoadRoundTrip exercises the real Load()/Set() path end to end, including
// os.UserConfigDir() resolution, by pointing HOME at a scratch directory.
func TestLoadRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "") // avoid Linux CI picking up a stray value

	empty := Load()
	if got := empty.Get("abc"); got != "" {
		t.Fatalf("fresh store should be empty, got %q", got)
	}
	empty.Set("abc", "hello")

	reloaded := Load()
	if got := reloaded.Get("abc"); got != "hello" {
		t.Errorf("reloaded value = %q, want %q", got, "hello")
	}
}
