package collect

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kyhan/amux/internal/herdr"
)

func TestHerdrLiveReadOnly(t *testing.T) {
	session := os.Getenv("AMUX_HERDR_LIVE_SESSION")
	if session == "" {
		t.Skip("opt-in Herdr read-only probe")
	}
	result, err := herdr.Call(context.Background(), session, "session.snapshot", nil)
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Snapshot struct {
			Panes []herdrPane `json:"panes"`
		} `json:"snapshot"`
	}
	if err := json.Unmarshal(result, &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Snapshot.Panes) == 0 {
		t.Fatal("live probe requires an existing pane in the named session")
	}
	snap, err := CollectBackend(time.Now(), BackendHerdr)
	if err != nil {
		t.Fatal(err)
	}
	for _, pn := range response.Snapshot.Panes {
		id := "herdr:" + session + ":" + pn.PaneID
		if !snap.LivePaneIDs[id] {
			t.Errorf("live inventory omitted %s", id)
		}
		t.Logf("live pane %s terminal=%s", id, pn.TerminalID)
	}
	expected := os.Getenv("AMUX_HERDR_LIVE_EXPECT")
	matched := false
	for _, a := range snap.Agents {
		if a.Backend == BackendHerdr {
			if a.Tool == "" || a.PaneID == "" || a.TmuxTarget != "" {
				t.Fatalf("unnormalized Herdr agent: %+v", a)
			}
			t.Logf("agent %s tool=%s life=%s pid=%d preview=%q", a.ID, a.Tool, a.Life, a.PID, a.Preview)
			if a.Session == session && expected != "" && strings.Contains(strings.Join(a.Preview, "\n"), expected) {
				matched = true
			}
		}
	}
	if expected != "" && !matched {
		t.Fatalf("no recognized live agent preview contains %q", expected)
	}
}
