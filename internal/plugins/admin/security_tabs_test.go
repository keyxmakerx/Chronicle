package admin

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

func TestNormalizeSecurityTab(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", SecurityTabOverview},
		{"overview", SecurityTabOverview},
		{"sessions", SecurityTabSessions},
		{"log", SecurityTabLog},
		{"signup", SecurityTabSignup},
		{"nope", SecurityTabOverview},
		{"<script>", SecurityTabOverview},
		{"LOG", SecurityTabOverview},
	}
	for _, tc := range tests {
		if got := normalizeSecurityTab(tc.in); got != tc.want {
			t.Errorf("normalizeSecurityTab(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSecurityPageMarksCurrentTab(t *testing.T) {
	tests := []struct {
		tab       string
		wantLabel string
	}{
		{SecurityTabOverview, "Overview"},
		{SecurityTabSessions, "Signed in now (1)"},
		{SecurityTabLog, "Sign-in log"},
		{SecurityTabSignup, "Who can sign up"},
	}
	for _, tc := range tests {
		t.Run(tc.tab, func(t *testing.T) {
			data := SecurityPageData{
				Tab:      tc.tab,
				Stats:    &SecurityStats{ActiveSessions: 1},
				Sessions: []auth.SessionInfo{{Name: "A", Email: "a@example.com", TokenHash: "h"}},
			}
			var sb strings.Builder
			if err := securityTabs(data).Render(context.Background(), &sb); err != nil {
				t.Fatal(err)
			}
			out := sb.String()
			if n := strings.Count(out, `aria-current="page"`); n != 1 {
				t.Fatalf("aria-current count = %d, want 1: %s", n, out)
			}
			i := strings.Index(out, `aria-current="page"`)
			end := strings.Index(out[i:], "</a>")
			if !strings.Contains(out[i:i+end], tc.wantLabel) {
				t.Errorf("current tab is not %q: %s", tc.wantLabel, out[i:i+end])
			}
		})
	}
}

func TestWorthALook(t *testing.T) {
	now := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	fail := func(email, ip string, ago time.Duration) SecurityEvent {
		return SecurityEvent{EventType: EventLoginFailed, IPAddress: ip, CreatedAt: now.Add(-ago), Details: map[string]any{"email": email}}
	}
	other := SecurityEvent{EventType: EventLogout, CreatedAt: now, Details: map[string]any{"email": "a@x.io"}}
	tests := []struct {
		name   string
		events []SecurityEvent
		want   int
	}{
		{"none", nil, 0},
		{"single typo is not flagged", []SecurityEvent{fail("a@x.io", "1.1.1.1", time.Minute)}, 0},
		{"repeat is flagged", []SecurityEvent{fail("a@x.io", "1.1.1.1", time.Minute), fail("a@x.io", "1.1.1.1", time.Hour)}, 1},
		{"old failures ignored", []SecurityEvent{fail("a@x.io", "1.1.1.1", 30*time.Hour), fail("a@x.io", "1.1.1.1", 31*time.Hour)}, 0},
		{"two people one each", []SecurityEvent{fail("a@x.io", "1.1.1.1", time.Minute), fail("b@x.io", "1.1.1.1", time.Minute)}, 0},
		{"other event types ignored", []SecurityEvent{other, other}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := worthALook(tc.events, now); len(got) != tc.want {
				t.Errorf("got %d items %+v, want %d", len(got), got, tc.want)
			}
		})
	}
	got := worthALook([]SecurityEvent{fail("a@x.io", "1.1.1.1", time.Minute), fail("a@x.io", "2.2.2.2", time.Hour)}, now)
	if len(got) != 1 || !strings.Contains(got[0].Title, "2 wrong passwords for a@x.io") || !strings.Contains(got[0].Detail, "2 addresses") {
		t.Errorf("unexpected item: %+v", got)
	}
}
