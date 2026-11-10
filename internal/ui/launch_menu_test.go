package ui

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kyhan/amux/internal/collect"
)

func TestLaunchMenuEditsHistoryAndReturnsFromTools(t *testing.T) {
	root := t.TempDir()
	other := filepath.Join(root, "한글 project ")
	if err := os.Mkdir(other, 0o755); err != nil {
		t.Fatal(err)
	}
	m := Model{preferencesPath: filepath.Join(root, "config", "preferences.json"),
		currentDir: root, width: 60, height: 16}
	if err := saveLaunchPath(launchHistoryPath(m.preferencesPath), other); err != nil {
		t.Fatal(err)
	}
	next, _ := m.openLaunchMenu()
	m = next.(Model)
	key := func(k tea.KeyMsg) tea.Cmd {
		t.Helper()
		next, cmd := m.handleLaunchMenu(k)
		m = next.(Model)
		return cmd
	}
	key(tea.KeyMsg{Type: tea.KeyDown})
	key(tea.KeyMsg{Type: tea.KeyUp})
	if m.launchMenu.input != other {
		t.Fatal("arrows must restore the persisted path")
	}
	key(tea.KeyMsg{Type: tea.KeyBackspace})
	key(tea.KeyMsg{Type: tea.KeySpace})
	key(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.launchMenu.tools || m.launchMenu.input != other {
		t.Fatalf("edited valid history did not advance: %+v", m.launchMenu)
	}
	key(tea.KeyMsg{Type: tea.KeyUp})
	key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("sh; touch bad")})
	if m.launchMenu.toolIndex != len(launchTools)-1 {
		t.Fatal("tool navigation or restricted selection failed")
	}
	key(tea.KeyMsg{Type: tea.KeyEsc})
	if m.launchMenu.tools || !m.launchMenu.open || m.launchMenu.input != other {
		t.Fatal("escape must return to the same editable path")
	}
	key(tea.KeyMsg{Type: tea.KeyEsc})
	if m.launchMenu.open {
		t.Fatal("second escape must cancel")
	}
}

func TestLaunchMenuFallsBackToOriginAfterStartupSelectionCleared(t *testing.T) {
	for _, tc := range []struct {
		name, origin, pane, want string
		rejectPopup              bool
	}{
		{"origin", "%12", "%99", "%12", false},
		{"current", "", "%99", "%99", true},
		{"no pane", "", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("AMUX_START_PANE", tc.origin)
			t.Setenv("TMUX_PANE", tc.pane)
			m := Model{preferencesPath: filepath.Join(t.TempDir(), "preferences.json")}
			next, _ := m.openLaunchMenu()
			target := next.(Model).launchMenu.target
			if target.pane != tc.want || target.rejectPopup != tc.rejectPopup {
				t.Fatalf("wrong origin fallback: %+v", target)
			}
		})
	}
}

func TestLaunchMenuNewPathErrorsAndUnicodeEditing(t *testing.T) {
	root := t.TempDir()
	m := Model{currentDir: root, preferencesPath: filepath.Join(root, "preferences.json")}
	next, _ := m.openLaunchMenu()
	m = next.(Model)
	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyDown}, // Explicit new-directory choice.
		{Type: tea.KeyRunes, Runes: []rune("한글")},
		{Type: tea.KeyLeft},
		{Type: tea.KeyDelete},
		{Type: tea.KeyEnter},
	} {
		next, _ = m.handleLaunchMenu(key)
		m = next.(Model)
	}
	if m.launchMenu.input != "한" || m.launchMenu.tools || m.launchMenu.error == "" {
		t.Fatalf("invalid directory advanced or Unicode editing failed: %+v", m.launchMenu)
	}
	if _, err := os.Stat(launchHistoryPath(m.preferencesPath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid directory persisted: %v", err)
	}
}

func TestLaunchMenuCapturesStablePaneAndBoundsRendering(t *testing.T) {
	a := collect.Agent{ID: "%123", Kind: collect.KindInteractive, Project: "/한글/project",
		CWD: "/한글/project/sub", TmuxTarget: "old:1.0", Session: "old", Life: collect.LifeWorking}
	m := Model{snap: projSnap(a), preferencesPath: filepath.Join(t.TempDir(), "preferences.json")}
	m.rebuildRows()
	next, _ := m.openLaunchMenu()
	m = next.(Model)
	m.snap = projSnap(collect.Agent{ID: "%999", Kind: collect.KindInteractive, Project: "/other"})
	m.rebuildRows()
	if m.launchMenu.target.pane != "%123" || m.launchMenu.input != a.CWD {
		t.Fatalf("launch selection was not captured: %+v", m.launchMenu)
	}
	m.launchMenu.error = "bad path\n\x1b[2J" + strings.Repeat("한", 100)
	for _, tools := range []bool{false, true} {
		m.launchMenu.tools = tools
		for _, size := range [][2]int{{6, 4}, {20, 5}, {40, 12}, {100, 24}} {
			m.width, m.height = size[0], size[1]
			view := m.renderLaunchMenu()
			if lipgloss.Width(view) > m.width || lipgloss.Height(view) > m.height {
				t.Fatalf("launch menu overflows %v: %dx%d", size, lipgloss.Width(view), lipgloss.Height(view))
			}
		}
	}
}

func TestLaunchResultKeepsErrorsVisibleAndClosesOnSuccess(t *testing.T) {
	for _, fail := range []bool{false, true} {
		m := Model{launchMenu: launchMenuState{open: true, tools: true, busy: true}}
		msg := launchedMsg{path: "/project", tool: "claude"}
		if fail {
			msg.err = errors.New("target pane disappeared")
		}
		next, cmd := m.handleLaunched(msg)
		m = next.(Model)
		if m.launchMenu.busy || m.launchMenu.open != fail || m.notice == "" ||
			(fail && (m.launchMenu.error == "" || cmd != nil)) || (!fail && cmd == nil) {
			t.Fatalf("wrong launch completion: fail=%v state=%+v cmd=%v", fail, m.launchMenu, cmd)
		}
	}
}

func TestLaunchMenuPreservesCursorAtLastColumn(t *testing.T) {
	for _, input := range []string{strings.Repeat("a", 53), strings.Repeat("한", 26) + "a"} {
		m := Model{width: 60, height: 12,
			launchMenu: launchMenuState{open: true, input: input, cursor: len([]rune(input))}}
		view := m.renderLaunchMenu()
		if !strings.Contains(view, input+"|") {
			t.Fatalf("last-column cursor clipped for %q:\n%s", input, view)
		}
	}
}

// This exercises actual tmux session resolution, directory handling and process
// startup on a private socket, without touching the user's running tmux server.
func TestLaunchRunsInCapturedSessionWithLiteralDirectory(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux is required for the isolated launch boundary test")
	}
	root := t.TempDir()
	// Darwin's Unix socket path limit is shorter than testing.T.TempDir paths.
	socketDir, err := os.MkdirTemp("/tmp", "amux-launch-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(socketDir); err != nil {
			t.Errorf("cleanup isolated socket directory: %v", err)
		}
	})
	socket := filepath.Join(socketDir, "tmux.sock")
	bin := filepath.Join(root, "bin")
	dir := filepath.Join(root, "한글 ' ; $HOME #{session_name}")
	for _, path := range []string{bin, dir} {
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	witness := filepath.Join(root, "started")
	// A signal, rather than a sleep, synchronizes with the newly launched tool.
	script := "#!/bin/sh\npwd > \"$LAUNCH_WITNESS\"\n\"" + tmux +
		"\" wait-for -S launch-ready\nexec \"" + tmux + "\" wait-for launch-release\n"
	for _, tool := range launchTools {
		if err := os.WriteFile(filepath.Join(bin, tool), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TMUX", socket+",0,0")
	t.Setenv("LAUNCH_WITNESS", witness)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	call := func(args ...string) string {
		t.Helper()
		cmd := exec.CommandContext(ctx, tmux, append([]string{"-S", socket}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("tmux %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	call("-f", "/dev/null", "new-session", "-d", "-s", "intended", "-P", "-F", "#{pane_id}")
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cleanupCancel()
		if out, err := exec.CommandContext(cleanupCtx, tmux, "-S", socket, "kill-server").CombinedOutput(); err != nil {
			t.Errorf("cleanup isolated tmux: %v: %s", err, out)
		}
	})
	pane := call("display-message", "-p", "-t", "=intended:", "#{pane_id}")
	call("new-session", "-d", "-s", "amux-popup")
	popupPane := call("display-message", "-p", "-t", "=amux-popup:", "#{pane_id}")
	call("set-option", "-g", "default-shell", "/bin/sh")
	if err := runLaunch(ctx, launchTarget{pane: pane}, dir, launchTools[0]); err != nil {
		t.Fatal(err)
	}
	call("wait-for", "launch-ready")
	data, err := os.ReadFile(witness)
	if err != nil || strings.TrimSpace(string(data)) != dir {
		t.Fatalf("tool started in wrong directory: %q err=%v", data, err)
	}
	if got := call("list-windows", "-t", "=intended:", "-F", "#{window_id}"); len(strings.Fields(got)) != 2 {
		t.Fatalf("intended session did not receive a window: %q", got)
	}
	if got := call("list-windows", "-t", "=amux-popup:", "-F", "#{window_id}"); len(strings.Fields(got)) != 1 {
		t.Fatalf("popup session received a launch window: %q", got)
	}
	for _, tool := range []string{"sh", "exit", "claude; false", ""} {
		if err := runLaunch(ctx, launchTarget{pane: pane}, dir, tool); err == nil {
			t.Fatalf("accepted unsupported tool %q", tool)
		}
	}
	if err := runLaunch(ctx, launchTarget{pane: popupPane, rejectPopup: true}, dir, launchTools[0]); err == nil {
		t.Fatal("TMUX_PANE fallback must not launch inside amux's popup session")
	}
	if err := runLaunch(ctx, launchTarget{pane: "%999999999"}, dir, launchTools[0]); err == nil {
		t.Fatal("missing captured pane must not fall back to the popup session")
	}
	cancelled, stop := context.WithCancel(context.Background())
	stop()
	if err := runLaunch(cancelled, launchTarget{pane: pane}, dir, launchTools[0]); err == nil {
		t.Fatal("cancelled execution must fail visibly")
	}
}
