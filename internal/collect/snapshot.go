package collect

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Collect gathers a full snapshot: interactive agents from tmux + background
// agents from ~/.claude/jobs, correlated and grouped by project.
//
// It performs exactly one `ps`, one `tmux list-panes`, and one directory scan,
// so a refresh is cheap enough to run on a sub-second timer.
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

	all := append(inter, bg...)
	snap := Snapshot{
		Agents:   all,
		Projects: group(all),
		Taken:    now,
	}
	return snap, nil
}

// lifeRank orders agents within a project: active work first, input next, then
// idle/live, with finished agents sinking to the bottom.
var lifeRank = map[Lifecycle]int{
	LifeInput:   0,
	LifeWorking: 1,
	LifeIdle:    2,
	LifeStale:   3,
	LifeUnknown: 4,
	LifeStopped: 5,
	LifeError:   6,
	LifeDone:    7,
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
			if lifeRank[ai.Life] != lifeRank[aj.Life] {
				return lifeRank[ai.Life] < lifeRank[aj.Life]
			}
			if ai.Kind != aj.Kind {
				return ai.Kind == KindInteractive
			}
			return ai.UpdatedAt.After(aj.UpdatedAt)
		})
		projects = append(projects, *p)
	}

	// Projects with active work float to the top; then by most-recent activity.
	sort.SliceStable(projects, func(i, j int) bool {
		wi, wj := projects[i].Working(), projects[j].Working()
		if wi != wj {
			return wi > wj
		}
		return projectLatest(projects[i]).After(projectLatest(projects[j]))
	})
	return projects
}

func projectLatest(p Project) time.Time {
	var latest time.Time
	for _, a := range p.Agents {
		if a.UpdatedAt.After(latest) {
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
