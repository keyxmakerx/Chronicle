package ai_workspace

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/connector"
)

func renderConnect(t *testing.T, v ConnectView) string {
	t.Helper()
	var buf bytes.Buffer
	if err := aiConnectSection(v).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// The connect section in each state: unavailable, nothing yet, the key just
// made (the only time the key is on the page), and connected.
func TestAIPage_ConnectSection(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	used := now.Add(-5 * time.Minute)
	conn := &connector.Connection{ID: 1, CampaignID: "camp-1", KeyHint: "WXYZ", Privacy: aiexport.PrivacyModePermitted,
		ClientName: "Claude", CreatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(connector.KeyLifetime), LastUsedAt: &used}
	const key = "chai_secretsecretsecretsecretsecretsecretWXYZ"
	base := ConnectView{CampaignID: "camp-1", Available: true, Endpoint: "https://c.example/api/ai/campaigns/camp-1/mcp", Now: now}

	cases := []struct {
		name    string
		v       func() ConnectView
		want    []string
		notWant []string
	}{
		{"not available", func() ConnectView { return ConnectView{CampaignID: "camp-1", Now: now} },
			nil, []string{"Make a key", key}},
		{"nothing yet", func() ConnectView { return base },
			[]string{"Make a key", `value="players"`, `value="owner"`, "/campaigns/camp-1/ai/connection", `href="#copy-and-paste"`},
			[]string{key, "Disconnect", "What it did", "Pause"}},
		{"the key just made", func() ConnectView { v := base; v.Conn = conn; v.NewKey = key; return v },
			[]string{key, "Bearer " + key, "claude mcp add --transport http chronicle", base.Endpoint, "only this once"},
			[]string{"Make a key"}},
		{"connected", func() ConnectView { v := base; v.Conn = conn; return v },
			[]string{"Claude is connected", "sees what you see", "only looks", "key ends in WXYZ", "last used 5 minutes ago", "Disconnect", "/ai/connection/disconnect",
				"/ai/connection/pause", `{&#34;connection&#34;:&#34;1&#34;}`, "/ai/connection/access", `name="connection" value="1"`, "Change what it may do",
				`sees: &#39;owner&#39;`, `change: &#39;look&#39;`, "wasOwner: true", "What it did", `href="#copy-and-paste"`},
			[]string{key, "Resume", "is paused"}},
		{"paused", func() ConnectView {
			v := base
			c := *conn
			p := now.Add(-2 * time.Minute)
			c.PausedAt = &p
			v.Conn = &c
			return v
		},
			[]string{"Claude is paused", "Paused by you 2 minutes ago", "nothing it asked for is lost", "/ai/connection/resume", "Resume"},
			[]string{"/ai/connection/pause", "is connected"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			html := renderConnect(t, tc.v())
			for _, w := range tc.want {
				if !strings.Contains(html, w) {
					t.Errorf("missing %q", w)
				}
			}
			for _, w := range tc.notWant {
				if strings.Contains(html, w) {
					t.Errorf("should not show %q", w)
				}
			}
		})
	}
}

func TestAppName(t *testing.T) {
	for in, want := range map[string]string{
		"Claude":      "Claude",
		"claude-ai":   "Claude",
		"Claude Code": "Claude Code",
		"claude-code": "Claude Code",
		"ChatGPT":     "ChatGPT",
		"open-webui":  "Open WebUI",
		"LM Studio":   "LM Studio",
		"mystery bot": "Your AI",
		"":            "Your AI",
	} {
		if got := appName(&connector.Connection{ClientName: in}); got != want {
			t.Errorf("appName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestConnStatus(t *testing.T) {
	now := time.Now()
	used := now
	cases := []struct {
		conn *connector.Connection
		want string
	}{
		{nil, "none"},
		{&connector.Connection{ExpiresAt: now.Add(time.Hour)}, "waiting"},
		{&connector.Connection{ExpiresAt: now.Add(time.Hour), LastUsedAt: &used}, "connected"},
		{&connector.Connection{ExpiresAt: now, LastUsedAt: &used}, "ended"},
		{&connector.Connection{ExpiresAt: now.Add(time.Hour), LastUsedAt: &used, PausedAt: &used}, "paused"},
		{&connector.Connection{ExpiresAt: now, PausedAt: &used}, "ended"},
	}
	for _, tc := range cases {
		if got := connStatus(ConnectView{Conn: tc.conn, Now: now}); got != tc.want {
			t.Errorf("got %q, want %q", got, tc.want)
		}
	}
}

// A sitting reads as one line: when it started in the owner's zone, what
// the AI did, and whether anything it asked for still waits.
func TestSittingLines(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("no time zone data")
	}
	now := time.Date(2026, 10, 25, 3, 0, 0, 0, time.UTC) // 11 pm on the 24th in New York
	v := ConnectView{Now: now, Loc: ny}
	for _, tc := range []struct {
		at   time.Time
		want string
	}{
		{time.Date(2026, 10, 24, 18, 44, 0, 0, time.UTC), "Today, 2:44 pm"},
		{time.Date(2026, 10, 24, 3, 30, 0, 0, time.UTC), "Yesterday, 11:30 pm"},
		{time.Date(2026, 10, 20, 23, 15, 0, 0, time.UTC), "Tuesday, 7:15 pm"},
		{time.Date(2026, 9, 1, 16, 0, 0, 0, time.UTC), "Tuesday 1 September, 12:00 pm"},
		{time.Date(2025, 12, 31, 17, 0, 0, 0, time.UTC), "31 December 2025, 12:00 pm"},
	} {
		if got := sittingWhen(v, tc.at); got != tc.want {
			t.Errorf("sittingWhen(%v) = %q, want %q", tc.at, got, tc.want)
		}
	}

	for _, tc := range []struct {
		s    connector.Sitting
		want string
	}{
		{connector.Sitting{Reads: 3, Lookups: 1, Asks: 1}, "Read 3 pages, looked up 1 thing, asked for changes once"},
		{connector.Sitting{Lookups: 41}, "Looked up 41 things"},
		{connector.Sitting{Asks: 2, Refused: 1}, "Asked for changes twice, 1 not answered"},
		{connector.Sitting{Reads: 1, Asks: 4}, "Read 1 page, asked for changes 4 times"},
		{connector.Sitting{}, "Connected"},
	} {
		if got := sittingSummary(tc.s); got != tc.want {
			t.Errorf("sittingSummary(%+v) = %q, want %q", tc.s, got, tc.want)
		}
	}

	start := time.Date(2026, 10, 24, 18, 0, 0, 0, time.UTC)
	s := connector.Sitting{Start: start, End: start.Add(10 * time.Minute), Asks: 1}
	v.Requests = []connector.Request{{CreatedAt: start.Add(10 * time.Minute)}, {CreatedAt: start.Add(time.Hour)}}
	if got := sittingTag(v, s); got != "1 waiting for you" {
		t.Errorf("tag with a waiting request = %q", got)
	}
	v.Requests = nil
	if got := sittingTag(v, s); got != "" {
		t.Errorf("tag after it was answered = %q", got)
	}
	if got := sittingTag(v, connector.Sitting{Lookups: 2}); got != "only looked" {
		t.Errorf("tag of a sitting that only looked = %q", got)
	}
}
