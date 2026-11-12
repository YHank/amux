package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/creack/pty"
	"github.com/kyhan/amux/internal/herdr"
	"golang.org/x/sys/unix"
)

// runHerdrAttach keeps Herdr on a controlling tty while translating amux's
// Ctrl-Space,d into Herdr's Ctrl-B,q. A doubled Ctrl-Space sends a literal
// Ctrl-Space. Selecting an agent takes over any stale attach client, because
// Herdr allows one direct client per terminal.
func runHerdrAttach(session, terminal string) (result error) {
	size, err := pty.GetsizeFull(os.Stdin)
	if err != nil {
		return fmt.Errorf("read attach terminal size: %w", err)
	}
	state, err := term.MakeRaw(os.Stdin.Fd())
	if err != nil {
		return fmt.Errorf("prepare attach terminal: %w", err)
	}
	defer func() { result = errors.Join(result, term.Restore(os.Stdin.Fd(), state)) }()

	resizes := make(chan os.Signal, 1)
	signal.Notify(resizes, syscall.SIGWINCH)
	defer signal.Stop(resizes)

	cmd := exec.Command("herdr", herdr.Args(session, "terminal", "attach", terminal, "--takeover")...)
	master, err := pty.StartWithSize(cmd, size)
	if err != nil {
		return fmt.Errorf("start Herdr attach: %w", err)
	}
	defer master.Close()
	// Raw syscalls below must never block on the PTY. Poll owns readiness,
	// including writes when Herdr temporarily stops consuming input.
	fd := int(master.Fd())
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	waited := false
	defer func() {
		if !waited {
			if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				result = errors.Join(result, err)
			}
			<-wait
		}
	}()
	if err := unix.SetNonblock(fd, true); err != nil {
		return fmt.Errorf("prepare attach PTY: %w", err)
	}

	var keys detachKeys
	detached := false
	var detachDeadline time.Time
	var pending []byte
	var output [4096]byte
	for {
		select {
		case err := <-wait:
			waited = true
			// Flush the child's last output without waiting on descendants
			// that may still hold an inherited slave descriptor.
			for {
				n, readErr := unix.Read(fd, output[:])
				if n > 0 {
					if _, writeErr := os.Stdout.Write(output[:n]); writeErr != nil {
						return errors.Join(err, writeErr)
					}
				}
				if readErr != nil || n == 0 {
					break
				}
			}
			return err
		case <-resizes:
			if err := pty.InheritSize(os.Stdin, master); err != nil {
				return fmt.Errorf("resize attach PTY: %w", err)
			}
		default:
		}
		if detached && time.Now().After(detachDeadline) {
			return fmt.Errorf("Herdr attach did not exit after detach")
		}
		polls := []unix.PollFd{
			{Fd: int32(os.Stdin.Fd())},
			{Fd: int32(fd), Events: unix.POLLIN},
		}
		if len(pending) > 0 {
			polls[1].Events |= unix.POLLOUT
		} else if !detached {
			polls[0].Events = unix.POLLIN
		}
		if _, err := unix.Poll(polls, 25); err != nil {
			if errors.Is(err, unix.EINTR) {
				continue
			}
			return fmt.Errorf("poll attach terminal: %w", err)
		}
		if polls[1].Revents&unix.POLLOUT != 0 && len(pending) > 0 {
			n, err := unix.Write(fd, pending)
			if err != nil && !errors.Is(err, unix.EAGAIN) && !errors.Is(err, unix.EINTR) {
				return fmt.Errorf("write attach input: %w", err)
			}
			if n > 0 {
				pending = pending[n:]
			}
		}
		if polls[1].Revents&(unix.POLLIN|unix.POLLHUP) != 0 {
			n, err := unix.Read(fd, output[:])
			if n > 0 {
				if _, err := os.Stdout.Write(output[:n]); err != nil {
					return fmt.Errorf("write attach output: %w", err)
				}
			}
			if err != nil && !errors.Is(err, unix.EAGAIN) && !errors.Is(err, unix.EINTR) && !errors.Is(err, unix.EIO) {
				return fmt.Errorf("read attach output: %w", err)
			}
		}
		if polls[0].Revents&(unix.POLLIN|unix.POLLHUP|unix.POLLERR|unix.POLLNVAL) != 0 {
			// No read-ahead: keys following detach belong to the dashboard.
			var input [1]byte
			n, err := unix.Read(int(os.Stdin.Fd()), input[:])
			if err != nil {
				if errors.Is(err, unix.EINTR) {
					continue
				}
				return fmt.Errorf("read attach input: %w", err)
			}
			if n == 0 {
				return fmt.Errorf("attach terminal input closed")
			}
			forward, detach := keys.feed(input[0])
			pending = append(pending, forward...)
			if detach {
				pending = append(pending, 2, 'q')
				detached = true
				detachDeadline = time.Now().Add(3 * time.Second)
			}
		} else if len(pending) == 0 && !detached {
			// Input paused inside an escape: it was a lone Esc, not a sequence.
			pending = keys.flush()
		}
	}
}

// detachKeys finds Ctrl-Space,d in the legacy NUL encoding and in the kitty
// (CSI u) and modifyOtherKeys encodings that a Herdr popup sends once the
// attached terminal enables them.
type detachKeys struct {
	held   []byte // incomplete escape sequence
	prefix []byte // pending Ctrl-Space, plus any key releases since
}

func (k *detachKeys) feed(b byte) ([]byte, bool) {
	if len(k.held) == 0 && b != 0x1b {
		return k.key([]byte{b})
	}
	k.held = append(k.held, b)
	if len(k.held) == 1 || k.held[1] == '[' && len(k.held) < 32 && (len(k.held) == 2 || b >= 0x20 && b <= 0x3f) {
		return nil, false
	}
	seq := k.held
	k.held = nil
	return k.key(seq)
}

func (k *detachKeys) flush() []byte {
	if len(k.held) == 0 {
		return nil
	}
	seq := k.held
	k.held = nil
	forward, _ := k.key(seq) // an incomplete sequence is never the detach key
	return forward
}

func (k *detachKeys) key(seq []byte) ([]byte, bool) {
	code, mods, event, csi := csiKey(seq)
	isPrefix := len(seq) == 1 && seq[0] == 0 || csi && code == ' ' && mods == 4 && event != 3
	if k.prefix == nil {
		if isPrefix {
			k.prefix = seq
			return nil, false
		}
		return seq, false
	}
	switch {
	case csi && event == 3:
		k.prefix = append(k.prefix, seq...)
		return nil, false
	case len(seq) == 1 && seq[0] == 'd' || csi && code == 'd' && mods == 0:
		k.prefix = nil
		return nil, true
	case isPrefix:
		k.prefix = nil
		return seq, false
	}
	forward := append(k.prefix, seq...)
	k.prefix = nil
	return forward, false
}

// csiKey decodes kitty "CSI code[:alts] [; mods[:event] [; text]] u" and
// modifyOtherKeys "CSI 27 ; mods ; code ~". Mods exclude Caps/Num Lock.
func csiKey(seq []byte) (code, mods, event int, ok bool) {
	if len(seq) < 4 || seq[0] != 0x1b || seq[1] != '[' {
		return 0, 0, 0, false
	}
	fields := strings.Split(string(seq[2:len(seq)-1]), ";")
	switch seq[len(seq)-1] {
	case 'u':
	case '~':
		if len(fields) != 3 || fields[0] != "27" {
			return 0, 0, 0, false
		}
		fields = []string{fields[2], fields[1]}
	default:
		return 0, 0, 0, false
	}
	code, err := strconv.Atoi(strings.SplitN(fields[0], ":", 2)[0])
	if err != nil {
		return 0, 0, 0, false
	}
	mods, event = 1, 1
	if len(fields) > 1 {
		parts := strings.SplitN(fields[1], ":", 2)
		if mods, err = strconv.Atoi(parts[0]); err != nil {
			return 0, 0, 0, false
		}
		if len(parts) == 2 {
			if event, err = strconv.Atoi(parts[1]); err != nil {
				return 0, 0, 0, false
			}
		}
	}
	return code, (mods - 1) &^ (64 | 128), event, true
}
