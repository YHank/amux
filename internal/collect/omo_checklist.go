package collect

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// OmO keeps a session's todo list only inside its append-only JSONL
// transcript: every todo tool result repeats the whole list in
// message.details.phases. A live process is tied to its session through
// sessions/<cwd>/session-holders/<sessionId>/<pid>.json.

type omoHolder struct {
	PID                int   `json:"pid"`
	ProcessStartedAtMs int64 `json:"processStartedAtMs"`
}

type omoTodoRecord struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Message   struct {
		Role     string `json:"role"`
		ToolName string `json:"toolName"`
		IsError  bool   `json:"isError"`
		Details  *struct {
			Op     string `json:"op"`
			Phases *[]struct {
				Name  string `json:"name"`
				Tasks []struct {
					Content string `json:"content"`
					Status  string `json:"status"`
				} `json:"tasks"`
			} `json:"phases"`
		} `json:"details"`
	} `json:"message"`
}

var omoTodoMarker = []byte(`"toolName":"todo"`)

// omoSessionDirName mirrors OmO's per-cwd session directory naming,
// e.g. /Users/me/app -> --Users-me-app--.
func omoSessionDirName(cwd string) string {
	return "--" + strings.ReplaceAll(strings.Trim(cwd, "/"), "/", "-") + "--"
}

// omoChecklist returns the latest todo list of the OmO session held by the
// live process ap. Holders are matched on PID and process start time so a
// reused PID never borrows another session's plan.
func omoChecklist(root string, ap proc, cwd string) []ChecklistItem {
	if ap.pid <= 0 || ap.startedAt.IsZero() || !filepath.IsAbs(cwd) {
		return nil
	}
	dir := filepath.Join(root, "sessions", omoSessionDirName(cwd))
	holders, _ := filepath.Glob(filepath.Join(dir, "session-holders", "*", strconv.Itoa(ap.pid)+".json"))
	var transcript string
	var newest time.Time
	for _, holderPath := range holders {
		var h omoHolder
		if !readClaudeRecord(holderPath, &h) || h.PID != ap.pid || h.ProcessStartedAtMs <= 0 {
			continue
		}
		delta := ap.startedAt.Sub(time.UnixMilli(h.ProcessStartedAtMs))
		if delta < -2*time.Second || delta > 2*time.Second {
			continue
		}
		id := filepath.Base(filepath.Dir(holderPath))
		if !claudeRecordID.MatchString(id) {
			continue
		}
		logs, _ := filepath.Glob(filepath.Join(dir, "*_"+id+".jsonl"))
		for _, log := range logs {
			if info, err := os.Stat(log); err == nil && info.ModTime().After(newest) {
				transcript, newest = log, info.ModTime()
			}
		}
	}
	if transcript == "" {
		return nil
	}
	return omoChecklists.read(transcript, time.Now())
}

type omoChecklistState struct {
	offset   int64 // bytes of complete lines already applied
	items    []ChecklistItem
	lastUsed time.Time
}

// omoChecklistCache remembers how far each transcript was parsed so a refresh
// reads only newly appended lines instead of rescanning a large log.
type omoChecklistCache struct {
	mu     sync.Mutex
	states map[string]*omoChecklistState
}

var omoChecklists = &omoChecklistCache{states: map[string]*omoChecklistState{}}

const omoChecklistCacheTTL = 10 * time.Minute

func (c *omoChecklistCache) read(path string, now time.Time) []ChecklistItem {
	c.mu.Lock()
	defer c.mu.Unlock()
	for p, st := range c.states {
		if now.Sub(st.lastUsed) > omoChecklistCacheTTL {
			delete(c.states, p)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		delete(c.states, path)
		return nil
	}
	st := c.states[path]
	if st == nil || info.Size() < st.offset {
		st = &omoChecklistState{}
		c.states[path] = st
	}
	st.lastUsed = now
	if info.Size() > st.offset {
		st.consume(path, info.Size())
	}
	return slices.Clone(st.items)
}

func (st *omoChecklistState) consume(path string, size int64) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()
	if _, err := file.Seek(st.offset, io.SeekStart); err != nil {
		return
	}
	reader := bufio.NewReaderSize(io.LimitReader(file, size-st.offset), 64*1024)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			// A trailing partial line is still being written; retry it next refresh.
			return
		}
		st.offset += int64(len(line))
		if bytes.Contains(line, omoTodoMarker) {
			st.apply(line)
		}
	}
}

func (st *omoChecklistState) apply(line []byte) {
	var rec omoTodoRecord
	if json.Unmarshal(line, &rec) != nil || rec.Type != "message" ||
		rec.Message.Role != "toolResult" || rec.Message.ToolName != "todo" || rec.Message.IsError ||
		rec.Message.Details == nil || rec.Message.Details.Phases == nil {
		return
	}
	previous := make(map[string]ChecklistItem, len(st.items))
	if rec.Message.Details.Op != "init" { // init replaces the plan and its timings
		for _, item := range st.items {
			previous[item.Content] = item
		}
	}
	var items []ChecklistItem
	for _, phase := range *rec.Message.Details.Phases {
		for _, task := range phase.Tasks {
			item := ChecklistItem{Phase: phase.Name, Content: task.Content, Status: ChecklistStatus(task.Status)}
			if prior, ok := previous[task.Content]; ok {
				item.StartedAt, item.FinishedAt = prior.StartedAt, prior.FinishedAt
			}
			switch item.Status {
			case ChecklistActive:
				if item.StartedAt.IsZero() {
					item.StartedAt = rec.Timestamp
				}
				item.FinishedAt = time.Time{}
			case ChecklistDone, ChecklistDropped:
				if item.FinishedAt.IsZero() {
					item.FinishedAt = rec.Timestamp
				}
			default:
				item.StartedAt, item.FinishedAt = time.Time{}, time.Time{}
			}
			items = append(items, item)
		}
	}
	st.items = items
}
