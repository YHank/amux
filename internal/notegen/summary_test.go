package notegen

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kyhan/amux/internal/collect"
)

func TestSummaryAllowsMultipleLinesAndTitleStillRejectsThem(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	script := "#!/bin/sh\ncat >/dev/null\nprintf '%s' \"$AMUX_TEST_OUTPUT\"\n"
	if err := os.WriteFile(filepath.Join(dir, "codex"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	a := collect.Agent{Tool: "claude", Intent: "Review payments", Preview: []string{"checking refunds"}}
	t.Setenv("AMUX_TEST_OUTPUT", "결제 검토\n환불 확인 중")
	got, err := Summary(context.Background(), a, "")
	if err != nil || got != "결제 검토\n환불 확인 중" {
		t.Fatalf("summary=%q err=%v", got, err)
	}
	if _, err := Title(context.Background(), a, ""); err == nil {
		t.Fatal("the existing title contract must still reject multiple lines")
	}
	for _, invalid := range []string{"", "bad\x1b[31m", strings.Repeat("a", 4001)} {
		t.Setenv("AMUX_TEST_OUTPUT", invalid)
		if _, err := Summary(context.Background(), a, ""); err == nil {
			t.Fatal("invalid summary accepted")
		}
	}
}
