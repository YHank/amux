package collect

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func opencodeIntentFixture(t *testing.T, schema string) string {
	t.Helper()
	sqlite, err := exec.LookPath("sqlite3")
	if err != nil {
		t.Skip("sqlite3 not installed")
	}
	root := t.TempDir()
	cmd := exec.Command(sqlite, filepath.Join(root, "opencode.db"))
	cmd.Stdin = strings.NewReader(schema)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create intent fixture: %v: %s", err, out)
	}
	return root
}

func TestOpenCodeIntentExactSessionLatestRequest(t *testing.T) {
	root := opencodeIntentFixture(t, `
CREATE TABLE session_v2 (id TEXT PRIMARY KEY, title TEXT);
CREATE TABLE session_message (session_id TEXT, type TEXT, seq INTEGER, data TEXT);
CREATE INDEX session_message_session_type_seq_idx ON session_message(session_id,type,seq);
INSERT INTO session_v2 VALUES
 ('ses_exact','Exact title'),
 ('ses_other','Other title'),
 ('ses_titleOnly','Title only'),
 ('ses_same','Same request'),
 ('ses_requestOnly',''),
 ('ses_null','No text'),
 ('ses_bounded',replace(hex(zeroblob(600)),'0',char(54620)));
INSERT INTO session_message VALUES
 ('ses_exact','user',9,'{"text":"latest exact request"}'),
 ('ses_exact','user',1,'{"text":"old exact request"}'),
 ('ses_exact','assistant',100,'{"text":"must not use assistant text"}'),
 ('ses_other','user',999,'{"text":"other session request"}'),
 ('ses_same','user',1,'{"text":" Same request "}'),
 ('ses_requestOnly','user',1,'{"text":"Request without title"}'),
 ('ses_null','user',1,'{}'),
 ('ses_bounded','user',1,json_object('text',replace(hex(zeroblob(600)),'0','R'))),
 ('ses_missing','user',1,'{"text":"orphan request"}');
`)
	for _, tc := range []struct {
		id   string
		want string
	}{
		{"ses_exact", "Exact title\nlatest exact request"},
		{"ses_other", "Other title\nother session request"},
		{"ses_titleOnly", "Title only"},
		{"ses_same", "Same request"},
		{"ses_requestOnly", "Request without title"},
		{"ses_null", "No text"},
		{"ses_bounded", strings.Repeat("\uD55C", 1000) + "\n" + strings.Repeat("R", 1000)},
		{"ses_missing", ""},
		{"../ses_exact", ""},
		{"ses_exact' OR 1=1 --", ""},
		{"", ""},
	} {
		t.Run(tc.id, func(t *testing.T) {
			if got := opencodeIntent(context.Background(), root, tc.id); got != tc.want {
				t.Fatalf("intent = %q, want %q", got, tc.want)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := opencodeIntent(ctx, root, "ses_exact"); got != "" {
		t.Fatalf("cancelled query returned %q", got)
	}
}

func TestOpenCodeIntentUnavailableV2ReturnsEmpty(t *testing.T) {
	root := opencodeIntentFixture(t, `
CREATE TABLE session (id TEXT PRIMARY KEY, title TEXT);
INSERT INTO session VALUES ('ses_old','Unsupported old title');
`)
	if got := opencodeIntent(context.Background(), root, "ses_old"); got != "" {
		t.Fatalf("unsupported schema returned %q", got)
	}
	if got := opencodeIntent(context.Background(), t.TempDir(), "ses_old"); got != "" {
		t.Fatalf("missing database returned %q", got)
	}
}
