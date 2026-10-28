package notegen

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestSelectedModelPersistsAcrossProcesses(t *testing.T) {
	// Given
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	if err := SelectModel("fixture-model"); err != nil {
		t.Fatal(err)
	}
	// When
	cmd := exec.Command(os.Args[0], "-test.run=^TestReadModelProcess$")
	cmd.Env = append(os.Environ(), "AMUX_READ_MODEL=1")
	out, err := cmd.Output()
	// Then
	if err != nil || string(out) != "fixture-model" {
		t.Fatalf("restarted selection=%q err=%v", out, err)
	}
	if err := SelectModel("replacement"); err != nil {
		t.Fatal(err)
	}
	if SelectedModel() != "replacement" {
		t.Fatal("selection did not change")
	}
}

func TestReadModelProcess(t *testing.T) {
	if os.Getenv("AMUX_READ_MODEL") != "1" {
		return
	}
	os.Stdout.WriteString(SelectedModel())
	os.Exit(0)
}

func TestModelsUsesAppServerPagination(t *testing.T) {
	// Given: a real stdio executable with paginated JSON-RPC responses.
	dir := t.TempDir()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	script := `#!/bin/sh
read init
printf '{"id":1,"result":{}}\n'
read initialized
read first
printf '{"method":"notification","params":{}}\n'
printf '{"id":2,"result":{"data":[{"id":"one","model":"one","displayName":"First","isDefault":true}],"nextCursor":"page-two"}}\n'
read second
case "$second" in *page-two*) ;; *) exit 2 ;; esac
printf '{"id":3,"result":{"data":[{"id":"two","model":"two"},{"id":"hidden","model":"hidden","hidden":true}],"nextCursor":null}}\n'
read end
`
	if err := os.WriteFile(filepath.Join(dir, "codex"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// When
	models, err := Models(ctx)
	// Then
	if err != nil || len(models) != 2 || models[0].Model != "one" || !models[0].IsDefault || models[1].Model != "two" {
		t.Fatalf("models=%v err=%v", models, err)
	}
}
