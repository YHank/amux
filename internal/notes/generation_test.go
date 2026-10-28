package notes

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGenerationProtectionAcrossProcesses(t *testing.T) {
	for _, action := range []string{"newest", "manual", "same", "clear", "cancel", "prune"} {
		t.Run(action, func(t *testing.T) {
			// Given
			path := filepath.Join(t.TempDir(), "notes.json")
			store := Open(path)
			store.Set("agent", "original")
			old, err := store.Begin("agent")
			if err != nil {
				t.Fatal(err)
			}
			// When: a separate dashboard changes the durable request contract.
			cmd := exec.Command(os.Args[0], "-test.run=^TestGenerationMutationProcess$")
			cmd.Env = append(os.Environ(), "AMUX_STORE="+path, "AMUX_MUTATION="+action)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("mutation: %v %s", err, out)
			}
			applied, err := store.Complete("agent", old, "stale", nil)
			// Then
			if err != nil || applied {
				t.Fatalf("stale completion applied=%v err=%v", applied, err)
			}
			want := "original"
			switch action {
			case "manual":
				want = "manual"
			case "clear", "prune":
				want = ""
			case "newest":
				want = "newest"
			}
			if got := Open(path).Get("agent"); got != want {
				t.Fatalf("note=%q want=%q", got, want)
			}
		})
	}
}

func TestGenerationMutationProcess(t *testing.T) {
	path := os.Getenv("AMUX_STORE")
	if path == "" {
		return
	}
	store := Open(path)
	switch os.Getenv("AMUX_MUTATION") {
	case "newest":
		token, err := store.Begin("agent")
		if err != nil {
			t.Fatal(err)
		}
		if applied, err := store.Complete("agent", token, "newest", nil); err != nil || !applied {
			t.Fatalf("new completion=%v err=%v", applied, err)
		}
	case "manual":
		store.Set("agent", "manual")
	case "same":
		store.Set("agent", "original")
	case "clear":
		store.Set("agent", "")
	case "cancel":
		if err := store.Cancel("agent"); err != nil {
			t.Fatal(err)
		}
	case "prune":
		store.Prune(map[string]bool{})
	}
}

func TestGenerationMergesConcurrentAgentResults(t *testing.T) {
	// Given
	path := filepath.Join(t.TempDir(), "notes.json")
	a, b := Open(path), Open(path)
	first, err := a.Begin("first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := b.Begin("second")
	if err != nil {
		t.Fatal(err)
	}
	// When
	results := make(chan error, 2)
	go func() { _, err := a.Complete("first", first, "one", nil); results <- err }()
	go func() { _, err := b.Complete("second", second, "two", nil); results <- err }()
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	// Then
	reloaded := Open(path)
	if reloaded.Get("first") != "one" || reloaded.Get("second") != "two" {
		t.Fatal("independent agent completion lost an update")
	}
}
