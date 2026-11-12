package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/creack/pty"
)

func TestHerdrAttachPTY(t *testing.T) {
	for _, tc := range []struct {
		name, input, want string
		exit              string
	}{
		{"detach", "hello\x00dZ", "68656c6c6f0271", ""},
		{"nonmatching_prefix", "\x00x\x03\x1b[A\x7f\x00d", "0078031b5b417f0271", ""},
		{"escaped_prefix", "\x00\x00d\x00d", "00640271", ""},
		{"native_detach", "\x02q", "0271", ""},
		// Herdr popups encode keys as kitty CSI u once the attach client asks.
		{"kitty_detach", "hi\x1b[32;5ud", "68690271", ""},
		{"kitty_release_detach", "\x1b[32;5u\x1b[32;5:3u\x1b[100u", "0271", ""},
		{"kitty_lock_detach", "\x1b[32;133u\x1b[100;129u", "0271", ""},
		{"kitty_nonmatching", "\x1b[32;5ux\x1b[A\x1b[32;5u\x1b[32;5u\x00d", "1b5b33323b3575781b5b411b5b33323b35750271", ""},
		{"modify_other_keys_detach", "\x1b[27;5;32~d", "0271", ""},
		{"lone_escape", "\x00d", "1b0271", ""},
		{"child_exit", "!", "21", ""},
		{"child_failure", "!", "21", "7"},
		{"detach_timeout", "\x00d", "0271", "hang"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			master, cmd, lines := startHerdrAttachFixture(t, tc.exit, tc.name == "detach")
			expectHerdrAttachLine(t, lines, "WRAPPER")
			expectHerdrAttachLine(t, lines, "ARGS --session fixture-session terminal attach fixture-terminal --takeover")
			expectHerdrAttachLine(t, lines, "READY 80 24")
			if err := pty.Setsize(master, &pty.Winsize{Cols: 101, Rows: 37}); err != nil {
				t.Fatal(err)
			}
			if err := cmd.Process.Signal(syscall.SIGWINCH); err != nil {
				t.Fatal(err)
			}
			expectHerdrAttachLine(t, lines, "SIZE 101 37")
			if tc.name == "lone_escape" {
				// Esc must reach the agent once input pauses, before the next key.
				if _, err := master.Write([]byte{0x1b}); err != nil {
					t.Fatal(err)
				}
				expectHerdrAttachLine(t, lines, "INPUT 1b")
			}
			// Write the prefix separately: matching must span Read calls.
			for _, b := range []byte(tc.input) {
				if _, err := master.Write([]byte{b}); err != nil {
					t.Fatal(err)
				}
			}
			expectHerdrAttachLine(t, lines, "BYTES "+tc.want)
			wantReturn := "RETURN ok"
			if tc.exit == "hang" {
				wantReturn = "RETURN Herdr attach did not exit after detach"
			} else if tc.exit != "" {
				wantReturn = "RETURN exit status " + tc.exit
			}
			expectHerdrAttachLine(t, lines, wantReturn)
			expectHerdrAttachLine(t, lines, "RESTORED")
			if tc.name == "detach" {
				// The unread suffix belongs to the resumed dashboard.
				if _, err := master.Write([]byte("\n")); err != nil {
					t.Fatal(err)
				}
				expectHerdrAttachLine(t, lines, "SUFFIX 5a0a")
			}
			if err := cmd.Wait(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func startHerdrAttachFixture(t *testing.T, exit string, suffix bool) (*os.File, *exec.Cmd, <-chan string) {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	script := "#!/bin/sh\nexec \"$AMUX_HERDR_ATTACH_TEST_BINARY\" -test.run='^TestHerdrAttachFixture$' -- \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, "herdr"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "-test.run=^TestHerdrAttachFixture$")
	cmd.Env = append(os.Environ(), "AMUX_HERDR_ATTACH_ROLE=wrapper",
		"AMUX_HERDR_ATTACH_TEST_BINARY="+binary, "AMUX_HERDR_ATTACH_EXIT="+exit,
		"PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"TERM=dumb",
		fmt.Sprintf("AMUX_HERDR_ATTACH_SUFFIX=%t", suffix))
	master, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	lines := make(chan string, 32)
	scanDone := make(chan struct{})
	go func() {
		defer close(scanDone)
		defer close(lines)
		var line []byte
		var b [1]byte
		for {
			if _, err := master.Read(b[:]); err != nil {
				return
			}
			if b[0] == '\n' {
				lines <- string(line)
				line = line[:0]
			} else {
				line = append(line, b[0])
			}
		}
	}()
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				t.Error(err)
			}
			cmd.Wait()
		}
		master.Close()
		<-scanDone
	})
	return master, cmd, lines
}

func expectHerdrAttachLine(t *testing.T, lines <-chan string, want string) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	var seen []string
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				t.Fatalf("PTY closed awaiting %q; saw %q", want, seen)
			}
			seen = append(seen, line)
			if strings.TrimSpace(line) == want {
				return
			}
		case <-timer.C:
			t.Fatalf("timeout awaiting %q; saw %q", want, seen)
		}
	}
}

// TestHerdrAttachFixture runs only in isolated child processes. The downstream
// requires a terminal, reads actual bytes, and independently recognizes Herdr's
// native detach sequence.
func TestHerdrAttachFixture(t *testing.T) {
	switch os.Getenv("AMUX_HERDR_ATTACH_ROLE") {
	case "":
		return
	case "wrapper":
		fmt.Println("WRAPPER")
		// Match the dashboard's raw/restore cycle before taking the snapshot.
		// Darwin sets its transient PENDIN bit on the first such cycle.
		initial, err := term.MakeRaw(os.Stdin.Fd())
		if err != nil {
			t.Fatal(err)
		}
		if err := term.Restore(os.Stdin.Fd(), initial); err != nil {
			t.Fatal(err)
		}
		before, err := term.GetState(os.Stdin.Fd())
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Setenv("AMUX_HERDR_ATTACH_ROLE", "child"); err != nil {
			t.Fatal(err)
		}
		err = runHerdrAttach("fixture-session", "fixture-terminal")
		if err != nil {
			fmt.Printf("RETURN %v\n", err)
		} else {
			fmt.Println("RETURN ok")
		}
		after, err := term.GetState(os.Stdin.Fd())
		if err != nil || !reflect.DeepEqual(before, after) {
			t.Fatalf("terminal not restored: %v before=%+v after=%+v", err, before, after)
		}
		fmt.Println("RESTORED")
		if os.Getenv("AMUX_HERDR_ATTACH_SUFFIX") == "true" {
			var suffix [2]byte
			if _, err := io.ReadFull(os.Stdin, suffix[:]); err != nil {
				t.Fatal(err)
			}
			fmt.Printf("SUFFIX %x\n", suffix)
		}
	case "child":
		if !term.IsTerminal(os.Stdin.Fd()) || !term.IsTerminal(os.Stdout.Fd()) {
			t.Fatal("attach requires terminal stdin and stdout")
		}
		state, err := term.MakeRaw(os.Stdin.Fd())
		if err != nil {
			t.Fatal(err)
		}
		defer term.Restore(os.Stdin.Fd(), state)
		resizes := make(chan os.Signal, 1)
		signal.Notify(resizes, syscall.SIGWINCH)
		defer signal.Stop(resizes)
		w, h, err := term.GetSize(os.Stdin.Fd())
		if err != nil {
			t.Fatal(err)
		}
		fmt.Printf("ARGS %s\n", strings.Join(flag.Args(), " "))
		fmt.Printf("READY %d %d\n", w, h)
		<-resizes
		w, h, err = term.GetSize(os.Stdin.Fd())
		if err != nil {
			t.Fatal(err)
		}
		fmt.Printf("SIZE %d %d\n", w, h)
		var received []byte
		for {
			var b [1]byte
			if _, err := io.ReadFull(os.Stdin, b[:]); err != nil {
				t.Fatal(err)
			}
			received = append(received, b[0])
			fmt.Printf("INPUT %02x\n", b[0])
			if b[0] == '!' || bytes.HasSuffix(received, []byte{2, 'q'}) {
				fmt.Printf("BYTES %x\n", received)
				if os.Getenv("AMUX_HERDR_ATTACH_EXIT") == "hang" {
					for {
						if _, err := io.ReadFull(os.Stdin, b[:]); err != nil {
							t.Fatal(err)
						}
					}
				}
				if os.Getenv("AMUX_HERDR_ATTACH_EXIT") == "7" {
					os.Exit(7)
				}
				return
			}
		}
	}
}
