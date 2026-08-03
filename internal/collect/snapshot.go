package collect

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Collect gathers a full snapshot: interactive agents from tmux, background
// Claude Code jobs (~/.claude/jobs), and background omp daemon tasks
// (~/.omp/run/daemons), correlated and grouped by project.
//
// It performs exactly one `ps`, one `tmux list-panes`, and a small, bounded
// number of directory scans/globs, so a refresh is cheap enough to run on a
// sub-second timer.
func Collect(now time.Time) (Snapshot, error) {
	procs, err := snapshotProcs()
	if err != nil {
		return Snapshot{}, err
	}
	inter, err := interactiveAgents(procs)
	if err != nil {
		return Snapshot{}, err
	}
	bg, err := backgroundAgents(liveShorts())
	if err != nil {
		return Snapshot{}, err
	}
	ompBg, err := ompBackgroundAgents()
	if err != nil {
		return Snapshot{}, err
	}

	all := append(inter, bg...)
	all = append(all, ompBg...)
	snap := Snapshot{
		Agents:   all,
		Projects: group(all),
		Taken:    now,
	}
	return snap, nil
}

// AttentionRank orders lifecycles by how urgently a person needs to act.
// Routine work follows actionable/uncertain states; completed work sinks.
func AttentionRank(l Lifecycle) int {
	switch l {
	case LifeInput:
		return 0
	case LifeError:
		return 1
	case LifeStale:
		return 2
	case LifeUnknown:
		return 3
	case LifeWorking:
		return 4
	case LifeIdle:
		return 5
	case LifeStopped:
		return 6
	case LifeDone:
		return 7
	default:
		return 8
	}
}

// group buckets agents by project key and sorts everything for stable display.
func group(agents []Agent) []Project {
	byKey := map[string]*Project{}
	for _, a := range agents {
		p, ok := byKey[a.Project]
		if !ok {
			p = &Project{Key: a.Project, Name: projectName(a.Project)}
			byKey[a.Project] = p
		}
		p.Agents = append(p.Agents, a)
	}

	projects := make([]Project, 0, len(byKey))
	for _, p := range byKey {
		sort.SliceStable(p.Agents, func(i, j int) bool {
			ai, aj := p.Agents[i], p.Agents[j]
			// Interactive before background when equally ranked.
			if AttentionRank(ai.Life) != AttentionRank(aj.Life) {
				return AttentionRank(ai.Life) < AttentionRank(aj.Life)
			}
			if ai.Kind != aj.Kind {
				return ai.Kind == KindInteractive
			}
			return ai.UpdatedAt.After(aj.UpdatedAt)
		})
		projects = append(projects, *p)
	}

	// Projects inherit their most urgent agent's rank. More agents at that
	// rank win ties, then the freshest agent contributing that rank.
	sort.SliceStable(projects, func(i, j int) bool {
		ri, ci := projectAttention(projects[i])
		rj, cj := projectAttention(projects[j])
		if ri != rj {
			return ri < rj
		}
		if ci != cj {
			return ci > cj
		}
		li, lj := projectLatest(projects[i], ri), projectLatest(projects[j], rj)
		if !li.Equal(lj) {
			return li.After(lj)
		}
		return projects[i].Key < projects[j].Key
	})
	return projects
}

func projectAttention(p Project) (rank, count int) {
	rank = AttentionRank(Lifecycle("")) + 1
	for _, a := range p.Agents {
		r := AttentionRank(a.Life)
		switch {
		case r < rank:
			rank, count = r, 1
		case r == rank:
			count++
		}
	}
	return rank, count
}

func projectLatest(p Project, rank int) time.Time {
	var latest time.Time
	for _, a := range p.Agents {
		if AttentionRank(a.Life) == rank && a.UpdatedAt.After(latest) {
			latest = a.UpdatedAt
		}
	}
	return latest
}

// projectKey normalizes a working directory to a stable grouping key. When the
// path is inside a git repo, the repo root is used so a session and the
// background agent it spawned (which may sit in a worktree or subdir) land in
// the same bucket.
func projectKey(cwd string) string {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return "(unknown)"
	}
	if root := gitRoot(cwd); root != "" {
		return root
	}
	return filepath.Clean(cwd)
}

// gitRoot walks up from dir looking for a .git entry, without shelling out.
func gitRoot(dir string) string {
	dir = filepath.Clean(dir)
	for {
		if fi, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			_ = fi
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func projectName(key string) string {
	if key == "" || key == "(unknown)" {
		return "(unknown)"
	}
	return filepath.Base(key)
}
