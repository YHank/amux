package collect

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// opencodeIntent reads the exact v2 session's title and latest user request.
// root is OpenCode's data directory; unavailable v2 storage returns no summary.
func opencodeIntent(ctx context.Context, root, sessionID string) string {
	if ctx.Err() != nil || !filepath.IsAbs(root) || !opencodeSessionID.MatchString(sessionID) {
		return ""
	}
	db := filepath.Join(root, "opencode.db")
	if info, err := os.Stat(db); err != nil || !info.Mode().IsRegular() {
		return ""
	}
	queryCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	// Both lookups use exact keys: session_v2's primary key and
	// session_message_session_type_seq_idx(session_id,type,seq).
	query := `SELECT substr(s.title,1,1000) AS title,
		(SELECT substr(json_extract(m.data,'$.text'),1,1000)
		 FROM session_message m WHERE m.session_id=s.id AND m.type='user'
		 ORDER BY m.seq DESC LIMIT 1) AS request
		FROM session_v2 s WHERE s.id='` + sessionID + `' LIMIT 1;`
	cmd := exec.CommandContext(queryCtx, "sqlite3", "-readonly", "-json", db, query)
	var out opencodeTodoOutput
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil || out.overflow || ctx.Err() != nil {
		return ""
	}
	var rows []struct {
		Title   string `json:"title"`
		Request string `json:"request"`
	}
	if json.Unmarshal(out.data, &rows) != nil || len(rows) != 1 {
		return ""
	}
	title, request := strings.TrimSpace(rows[0].Title), strings.TrimSpace(rows[0].Request)
	switch {
	case request == "" || request == title:
		return title
	case title == "":
		return request
	default:
		return title + "\n" + request
	}
}
