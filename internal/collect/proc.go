package collect

import (
	"bufio"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// proc is one row of the process snapshot.
type proc struct {
	pid  int
	ppid int
	comm string  // short command name (basename of exec)
	args string  // full argv
	cpu  float64 // %CPU
}

// procTable is a single `ps` snapshot plus a parent→children index, so tree
// walks never shell out per-process. One exec per refresh — cheap and fast.
type procTable struct {
	byPID    map[int]proc
	children map[int][]int
}

// aiTools maps a detectable command name to its canonical tool label.
var aiTools = map[string]string{
	"claude": "claude",
	"codex":  "codex",
	"aider":  "aider",
	"gemini": "gemini",
}

// snapshotProcs runs one `ps` and builds an in-memory process tree.
func snapshotProcs() (*procTable, error) {
	// -A all procs, -w wide, -o custom columns. comm before args so the fixed
	// columns parse cleanly and args (which contains spaces) is last.
	cmd := exec.Command("ps", "-Awwo", "pid=,ppid=,%cpu=,comm=,args=")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	t := &procTable{byPID: map[int]proc{}, children: map[int][]int{}}
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimLeft(sc.Text(), " ")
		if line == "" {
			continue
		}
		// Split off the three leading numeric/float fields, keep the rest.
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		cpu, _ := strconv.ParseFloat(fields[2], 64)
		if err1 != nil || err2 != nil {
			continue
		}
		comm := fields[3]
		// args is everything after the first 4 whitespace-separated fields.
		args := trimNFields(line, 4)
		t.byPID[pid] = proc{pid: pid, ppid: ppid, comm: comm, args: args, cpu: cpu}
		t.children[ppid] = append(t.children[ppid], pid)
	}
	return t, sc.Err()
}

// trimNFields returns the substring of line after the first n whitespace fields.
func trimNFields(line string, n int) string {
	s := line
	for i := 0; i < n; i++ {
		s = strings.TrimLeft(s, " \t")
		idx := strings.IndexAny(s, " \t")
		if idx < 0 {
			return ""
		}
		s = s[idx:]
	}
	return strings.TrimLeft(s, " \t")
}

// detectTool reports the AI tool a process represents, or "" if none.
// It checks the comm basename first, then the first argv token, so both
// `claude` and `/path/to/claude agents` and `node .../codex` style launches
// are caught.
func detectTool(p proc) string {
	if tool, ok := aiTools[base(p.comm)]; ok {
		return tool
	}
	// Fall back to the argv[0] basename.
	if first := firstToken(p.args); first != "" {
		if tool, ok := aiTools[base(first)]; ok {
			return tool
		}
	}
	// Interpreted CLIs (codex/aider) may show up as `node`/`python` with the
	// tool name embedded in a later arg. Match on a word boundary to avoid
	// false positives from unrelated paths.
	for name, tool := range aiTools {
		if containsCmd(p.args, name) {
			return tool
		}
	}
	return ""
}

// findAgentProc searches root and all descendants for the first process that
// looks like an AI tool. Returns the matching proc and true if found.
func (t *procTable) findAgentProc(root int) (proc, bool) {
	// BFS so we prefer the shallowest (closest to the pane) match.
	queue := []int{root}
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		if p, ok := t.byPID[pid]; ok {
			if detectTool(p) != "" {
				return p, true
			}
		}
		queue = append(queue, t.children[pid]...)
	}
	return proc{}, false
}

// subtreeCPU sums %CPU across a process and all descendants.
func (t *procTable) subtreeCPU(root int) float64 {
	total := 0.0
	queue := []int{root}
	seen := map[int]bool{}
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		if seen[pid] {
			continue
		}
		seen[pid] = true
		if p, ok := t.byPID[pid]; ok {
			total += p.cpu
		}
		queue = append(queue, t.children[pid]...)
	}
	return total
}

func base(s string) string {
	s = strings.TrimPrefix(s, "-") // login shells show as "-zsh"
	return filepath.Base(s)
}

func firstToken(s string) string {
	if i := strings.IndexAny(s, " \t"); i >= 0 {
		return s[:i]
	}
	return s
}

// containsCmd reports whether name appears in args as a standalone command
// token (surrounded by whitespace or a path separator), not as a substring.
func containsCmd(args, name string) bool {
	for _, tok := range strings.Fields(args) {
		if base(tok) == name {
			return true
		}
	}
	return false
}
