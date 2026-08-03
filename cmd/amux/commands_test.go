package main

import (
	"strings"
	"testing"

	"github.com/kyhan/amux/internal/collect"
)

func TestStatusLineOrdersAttentionFirst(t *testing.T) {
	snap := collect.Snapshot{Agents: []collect.Agent{
		{Life: collect.LifeIdle},
		{Life: collect.LifeWorking},
		{Life: collect.LifeUnknown},
		{Life: collect.LifeStale},
		{Life: collect.LifeError},
		{Life: collect.LifeInput},
	}}
	if got, want := statusLine(snap, true), "◆1 ✖1 ⚠1 ·1 ▶1 ●1"; got != want {
		t.Errorf("statusLine = %q, want %q", got, want)
	}
}

func TestTmuxDQuote(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`/tmp/o'brien/amux`, `"/tmp/o'brien/amux"`},
		{`/tmp/o"brien/amux`, `"/tmp/o\"brien/amux"`},
		{`/tmp/o\brien/amux`, `"/tmp/o\\brien/amux"`},
		{`/tmp/o'b"r\ien/amux`, `"/tmp/o'b\"r\\ien/amux"`},
		{`/plain/path/amux`, `"/plain/path/amux"`},
	}
	for _, c := range cases {
		if got := tmuxDQuote(c.in); got != c.want {
			t.Errorf("tmuxDQuote(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestValidKeySpec(t *testing.T) {
	valid := []string{"a", "A", "Space", "M-x", "C-a", "F12", "1"}
	for _, k := range valid {
		if !validKeySpec(k) {
			t.Errorf("validKeySpec(%q) = false, want true", k)
		}
	}
	invalid := []string{"", "a b", "a'b", `a"b`, `a\b`, "a;b", "a\nb", "a\tb"}
	for _, k := range invalid {
		if validKeySpec(k) {
			t.Errorf("validKeySpec(%q) = true, want false", k)
		}
	}
}

func TestReplaceMarkedBlockAppendsWhenAbsent(t *testing.T) {
	content := "set -g status-right 'x'\n"
	block := "# amux-keybind — v1\nbind-key a display-popup ...\n"
	_, replaced := replaceMarkedBlock(content, "# amux-keybind", block)
	if replaced {
		t.Error("expected no replacement when marker is absent")
	}
}

func TestReplaceMarkedBlockUpdatesInPlace(t *testing.T) {
	content := "set -g status-right 'x'\n\n# amux-keybind — old install\nbind-key a display-popup -E \"/old/path/amux\"\n\nset -g mouse on\n"
	block := "# amux-keybind — new install\nbind-key b display-popup -E \"/new/path/amux\"\n"

	got, replaced := replaceMarkedBlock(content, "# amux-keybind", block)
	if !replaced {
		t.Fatal("expected marker to be found and replaced")
	}
	want := "set -g status-right 'x'\n\n# amux-keybind — new install\nbind-key b display-popup -E \"/new/path/amux\"\n\nset -g mouse on\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}

	// The regression this guards: matching on the OLD path (e.g. via
	// strings.Contains(existing, self)) would miss this case entirely, since
	// the file contains /old/path/amux but self is now /new/path/amux —
	// leading to a stale binding left behind plus a duplicate appended.
	if strings.Contains(got, "/old/path/amux") {
		t.Error("stale path should have been fully replaced, not left behind")
	}
}
