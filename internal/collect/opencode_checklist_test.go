package collect

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestOpenCodeChecklistSQLiteExactSession(t *testing.T) {
	sqlite, err := exec.LookPath("sqlite3")
	if err != nil {
		t.Skip("sqlite3 not installed")
	}
	root := t.TempDir()
	db := filepath.Join(root, "opencode.db")
	cmd := exec.Command(sqlite, db)
	cmd.Stdin = strings.NewReader(`
CREATE TABLE todo (
 session_id TEXT NOT NULL, content TEXT NOT NULL, status TEXT NOT NULL,
 priority TEXT NOT NULL, position INTEGER NOT NULL,
 time_created INTEGER NOT NULL, time_updated INTEGER NOT NULL,
 PRIMARY KEY(session_id,position)
);
INSERT INTO todo VALUES
 ('ses_exact','finish','completed','low',2,1,2),
 ('ses_other','another session','in_progress','high',0,1,999),
 ('ses_exact','run','in_progress','high',1,1,2),
 ('ses_exact','wait','pending','high',0,1,2),
 ('ses_exact','drop','cancelled','low',3,1,2),
 ('ses_exact','unknown','future_status','low',4,1,2),
 ('ses_exact',' ','pending','low',5,1,2);
WITH RECURSIVE n(x) AS (
 VALUES(0) UNION ALL SELECT x+1 FROM n WHERE x<299
)
INSERT INTO todo SELECT 'ses_many','item '||x,'pending','low',x,1,2 FROM n;
INSERT INTO todo VALUES
 ('ses_large',replace(hex(zeroblob(35000)),'0','x'),'pending','low',0,1,2);
`)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create SQLite fixture: %v: %s", err, out)
	}
	for _, id := range []string{"ses_exact", "ses_empty", "ses_large"} {
		writeClaudeFixture(t, filepath.Join(root, "storage", "todo", id+".json"),
			`[{"content":"obsolete legacy item","status":"pending"}]`)
	}
	many := make([]ChecklistItem, 256)
	for i := range many {
		many[i] = ChecklistItem{Content: "item " + strconv.Itoa(i), Status: ChecklistPending}
	}
	for _, tc := range []struct {
		id   string
		want []ChecklistItem
	}{
		{"ses_exact", []ChecklistItem{
			{Content: "wait", Status: ChecklistPending},
			{Content: "run", Status: ChecklistActive},
			{Content: "finish", Status: ChecklistDone},
			{Content: "drop", Status: ChecklistDropped},
		}},
		{"ses_other", []ChecklistItem{{Content: "another session", Status: ChecklistActive}}},
		{"ses_empty", nil},
		{"ses_many", many},
		{"ses_large", nil},
		{"ses_exact' OR 1=1 --", nil},
	} {
		t.Run(tc.id, func(t *testing.T) {
			if got := opencodeChecklist(context.Background(), root, tc.id); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("checklist = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestOpenCodeChecklistLegacyWhenDatabaseUnavailable(t *testing.T) {
	for _, state := range []string{"missing", "invalid", "missing CLI"} {
		t.Run(state, func(t *testing.T) {
			root := t.TempDir()
			if state != "missing" {
				if err := os.WriteFile(filepath.Join(root, "opencode.db"), []byte("not SQLite"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if state == "missing CLI" {
				t.Setenv("PATH", t.TempDir())
			}
			writeClaudeFixture(t, filepath.Join(root, "storage", "todo", "ses_legacy.json"),
				`[{"id":"b","content":"second ID, first position","status":"in_progress","priority":"low"},{"id":"a","content":"last","status":"cancelled","priority":"high"}]`)
			want := []ChecklistItem{
				{Content: "second ID, first position", Status: ChecklistActive},
				{Content: "last", Status: ChecklistDropped},
			}
			if got := opencodeChecklist(context.Background(), root, "ses_legacy"); !reflect.DeepEqual(got, want) {
				t.Fatalf("legacy checklist = %#v, want %#v", got, want)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if got := opencodeChecklist(ctx, root, "ses_legacy"); got != nil {
				t.Fatalf("cancelled collection returned %#v", got)
			}
		})
	}
}

func TestOpenCodeChecklistRejectsOversizedLegacy(t *testing.T) {
	root := t.TempDir()
	writeClaudeFixture(t, filepath.Join(root, "storage", "todo", "ses_large.json"),
		`[{"content":"`+strings.Repeat("x", 64*1024)+`","status":"pending"}]`)
	if got := opencodeChecklist(context.Background(), root, "ses_large"); got != nil {
		t.Fatalf("oversized checklist = %#v", got)
	}
}
