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
			[]string{"Make a key", `value="players"`, `value="owner"`, "/campaigns/camp-1/ai/connection", "What it did"},
			[]string{key, "Disconnect"}},
		{"the key just made", func() ConnectView { v := base; v.Conn = conn; v.NewKey = key; return v },
			[]string{key, "Bearer " + key, "claude mcp add --transport http chronicle", base.Endpoint, "only this once"},
			[]string{"Make a key"}},
		{"connected", func() ConnectView { v := base; v.Conn = conn; return v },
			[]string{"Claude is connected", "sees what you see", "only looks", "key ends in WXYZ", "last used 5 minutes ago", "Disconnect", "/ai/connection/disconnect"},
			[]string{key}},
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
	}
	for _, tc := range cases {
		if got := connStatus(ConnectView{Conn: tc.conn, Now: now}); got != tc.want {
			t.Errorf("got %q, want %q", got, tc.want)
		}
	}
}
