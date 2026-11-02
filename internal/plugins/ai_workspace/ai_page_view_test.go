package ai_workspace

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/connector"
)

// Each pair of what it sees and whether it asks is one level, except the
// pair only Custom reaches.
func TestLevelOf(t *testing.T) {
	cases := []struct {
		name string
		conn *connector.Connection
		want string
	}{
		{"no key yet", nil, "players"},
		{"players, look", &connector.Connection{Privacy: aiexport.PrivacyModeSafe}, "players"},
		{"owner, look", &connector.Connection{Privacy: aiexport.PrivacyModePermitted}, "everything"},
		{"owner, ask", &connector.Connection{Privacy: aiexport.PrivacyModePermitted, CanAsk: true}, "ask"},
		{"players, ask", &connector.Connection{Privacy: aiexport.PrivacyModeSafe, CanAsk: true}, "custom"},
	}
	for _, tc := range cases {
		if got := levelOf(tc.conn); got != tc.want {
			t.Errorf("%s: levelOf = %q, want %q", tc.name, got, tc.want)
		}
	}
	for _, l := range aiLevels() {
		c := &connector.Connection{Privacy: aiexport.PrivacyModeSafe, CanAsk: l.Change == "ask"}
		if l.Sees == "owner" {
			c.Privacy = aiexport.PrivacyModePermitted
		}
		if got := levelOf(c); got != l.ID {
			t.Errorf("level %q's own settings read back as %q", l.ID, got)
		}
		if last := l.Covers[len(l.Covers)-1]; !last.Never {
			t.Errorf("level %q doesn't end on what it never does", l.ID)
		}
	}
}

// A sitting's calls carry where their tick sits, what kind they are, and
// whether a request still waits.
func TestSittingRows(t *testing.T) {
	start := time.Date(2026, 10, 1, 20, 0, 0, 0, time.UTC)
	ask := start.Add(10 * time.Minute)
	s := connector.Sitting{Start: start, End: start.Add(20 * time.Minute), Calls: []connector.Call{
		{Tool: "read_page", At: start.Add(20 * time.Minute), Summary: "Read Brimstone Keep", OK: true},
		{Tool: toolAskName, At: ask, Summary: "Asked: Add the Ash Road", Note: "sent for your review", OK: true},
		{Tool: toolAskName, At: start.Add(5 * time.Minute), Summary: "Asked: Old one", OK: true},
		{Tool: "read_note", At: start, Summary: "Read a note", Note: "not available", OK: false},
	}}
	v := ConnectView{Requests: []connector.Request{{Summary: "Add the Ash Road", CreatedAt: ask.Add(2 * time.Second)}}}
	rows := sittingRows(v, s)
	want := []struct {
		kind    string
		left    float64
		waiting bool
	}{{"look", 100, false}, {"ask", 50, true}, {"ask", 25, false}, {"look", 0, false}}
	for i, w := range want {
		r := rows[i]
		if r.Kind != w.kind || r.Left != w.left || r.Waiting != w.waiting {
			t.Errorf("row %d = %s %.0f %t, want %s %.0f %t", i, r.Kind, r.Left, r.Waiting, w.kind, w.left, w.waiting)
		}
	}
	if rows[3].Icon != "fa-circle-info" || waitingIn(rows) != 1 || failedIn(rows) != 1 {
		t.Errorf("icon %q, waiting %d, failed %d", rows[3].Icon, waitingIn(rows), failedIn(rows))
	}
	one := sittingRows(v, connector.Sitting{Start: start, End: start, Calls: s.Calls[:1]})
	if one[0].Left != 50 {
		t.Errorf("a one-moment sitting's tick sits at %.0f, want the middle", one[0].Left)
	}
}

// What it did shows the filters with their counts, and each call carries
// what they read.
func TestAIActivity_Filters(t *testing.T) {
	now := time.Date(2026, 10, 1, 21, 0, 0, 0, time.UTC)
	start := now.Add(-time.Hour)
	calls := []connector.Call{
		{ID: 2, Tool: toolAskName, At: start.Add(time.Minute), Summary: "Asked: Add the Ash Road", OK: true},
		{ID: 1, Tool: "read_page", At: start, Summary: "Read Brimstone Keep", Note: "not available", OK: false},
	}
	v := ConnectView{CampaignID: "camp-1", Available: true, Now: now, Loc: time.UTC, Shown: sittingsStep, Calls: calls,
		Sittings: []connector.Sitting{{Start: start, End: start.Add(time.Minute), Calls: calls, Asks: 1, Refused: 1}},
		Requests: []connector.Request{{Summary: "Add the Ash Road", CreatedAt: start.Add(time.Minute)}}}
	html := renderActivity(t, v)
	for _, w := range []string{"Everything", "Requests", "Waiting for you", "Couldn&#39;t answer", "<b>2</b>", "Find a page or request",
		`data-kind="ask" data-wait`, `data-fail`, `data-text="read brimstone keep not available"`, "Couldn't answer: not available",
		"1 waiting", "1 couldn't answer", "left: calc(3px + (100% - 8px) * 1.000)", "Today", "8:00 pm to 8:01 pm", "Unfold all"} {
		if !strings.Contains(html, w) {
			t.Errorf("missing %q", w)
		}
	}
}

func renderActivity(t *testing.T, v ConnectView) string {
	t.Helper()
	var buf strings.Builder
	if err := aiActivityList(v).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}
