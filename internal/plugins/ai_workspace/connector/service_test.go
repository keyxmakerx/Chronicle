package connector

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
)

func newTestService() (*Service, *memRepo, owners, *time.Time) {
	repo := &memRepo{disabled: map[string]bool{}}
	own := owners{"camp-1": "owner-1", "camp-2": "owner-2"}
	svc := NewService(repo, own)
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	return svc, repo, own, &now
}

// A key is the prefix and 256 random bits; only its hash and last four
// characters are stored.
func TestConnect_KeyIsShownOnceAndStoredHashed(t *testing.T) {
	svc, repo, _, _ := newTestService()
	raw, c, err := svc.Connect(context.Background(), "camp-1", "owner-1", aiexport.PrivacyModeSafe, false)
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^chai_[A-Za-z0-9_-]{43}$`).MatchString(raw) {
		t.Fatalf("key %q is not chai_ + 43 base64url characters", raw)
	}
	stored := repo.conns[0]
	if stored.KeyHash != hashKey(raw) || strings.Contains(stored.KeyHash, raw[5:]) {
		t.Errorf("stored hash %q is not the key's SHA-256", stored.KeyHash)
	}
	if stored.KeyHint != raw[len(raw)-4:] {
		t.Errorf("hint = %q", stored.KeyHint)
	}
	if got := c.ExpiresAt.Sub(c.CreatedAt); got != KeyLifetime {
		t.Errorf("lifetime = %v, want %v", got, KeyLifetime)
	}
	raw2, _, _ := svc.Connect(context.Background(), "camp-1", "owner-1", aiexport.PrivacyModeSafe, false)
	if raw2 == raw {
		t.Error("two keys came out the same")
	}
}

func TestConnect_Refusals(t *testing.T) {
	cases := []struct {
		name       string
		campaign   string
		user       string
		privacy    aiexport.PrivacyMode
		wantStored int
	}{
		{"everything is not offered", "camp-1", "owner-1", aiexport.PrivacyModeEverything, 0},
		{"not the owner", "camp-1", "player-1", aiexport.PrivacyModeSafe, 0},
		{"another campaign's owner", "camp-2", "owner-1", aiexport.PrivacyModeSafe, 0},
		{"deleted campaign", "camp-gone", "owner-1", aiexport.PrivacyModeSafe, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, _, _ := newTestService()
			if _, _, err := svc.Connect(context.Background(), tc.campaign, tc.user, tc.privacy, false); err == nil {
				t.Fatal("connected")
			}
			if len(repo.conns) != tc.wantStored {
				t.Errorf("stored %d connections", len(repo.conns))
			}
		})
	}
}

// A new key revokes the old one: a campaign has one connection at a time.
func TestConnect_RevokesTheEarlierKey(t *testing.T) {
	svc, _, _, _ := newTestService()
	ctx := context.Background()
	old, _, _ := svc.Connect(ctx, "camp-1", "owner-1", aiexport.PrivacyModeSafe, false)
	if _, _, err := svc.Connect(ctx, "camp-1", "owner-1", aiexport.PrivacyModePermitted, false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Authenticate(ctx, "camp-1", old); !errors.Is(err, ErrKeyEnded) {
		t.Errorf("old key: err = %v, want ErrKeyEnded", err)
	}
}

func TestAuthenticate(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		// setup runs after a Safe key for camp-1 is made, and returns the
		// key and campaign to present.
		setup func(svc *Service, repo *memRepo, own owners, now *time.Time, raw string) (key, campaign string)
		want  error
	}{
		{"the key opens its campaign", func(_ *Service, _ *memRepo, _ owners, _ *time.Time, raw string) (string, string) {
			return raw, "camp-1"
		}, nil},
		{"no key", func(_ *Service, _ *memRepo, _ owners, _ *time.Time, _ string) (string, string) {
			return "", "camp-1"
		}, ErrBadKey},
		{"a wrong key", func(_ *Service, _ *memRepo, _ owners, _ *time.Time, raw string) (string, string) {
			last := "A"
			if strings.HasSuffix(raw, "A") {
				last = "B"
			}
			return raw[:len(raw)-1] + last, "camp-1"
		}, ErrBadKey},
		{"a sync API key", func(_ *Service, _ *memRepo, _ owners, _ *time.Time, raw string) (string, string) {
			return "chron_" + raw[5:], "camp-1"
		}, ErrBadKey},
		{"another campaign's address", func(_ *Service, _ *memRepo, _ owners, _ *time.Time, raw string) (string, string) {
			return raw, "camp-2"
		}, ErrBadKey},
		{"disconnected", func(svc *Service, _ *memRepo, _ owners, _ *time.Time, raw string) (string, string) {
			_ = svc.Disconnect(ctx, "camp-1")
			return raw, "camp-1"
		}, ErrKeyEnded},
		{"ran out", func(_ *Service, _ *memRepo, _ owners, now *time.Time, raw string) (string, string) {
			*now = now.Add(KeyLifetime)
			return raw, "camp-1"
		}, ErrKeyEnded},
		{"the maker no longer owns it", func(_ *Service, _ *memRepo, own owners, _ *time.Time, raw string) (string, string) {
			own["camp-1"] = "someone-else"
			return raw, "camp-1"
		}, ErrOwnerGone},
		{"the campaign was deleted", func(_ *Service, _ *memRepo, own owners, _ *time.Time, raw string) (string, string) {
			delete(own, "camp-1")
			return raw, "camp-1"
		}, ErrOwnerGone},
		{"the maker's account was disabled", func(_ *Service, repo *memRepo, _ owners, _ *time.Time, raw string) (string, string) {
			repo.disabled["owner-1"] = true
			return raw, "camp-1"
		}, ErrBadKey},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, own, now := newTestService()
			raw, _, err := svc.Connect(ctx, "camp-1", "owner-1", aiexport.PrivacyModeSafe, false)
			if err != nil {
				t.Fatal(err)
			}
			key, campaign := tc.setup(svc, repo, own, now, raw)
			c, name, err := svc.Authenticate(ctx, campaign, key)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if tc.want == nil && (c == nil || c.CampaignID != "camp-1" || name != "Ashfall") {
				t.Errorf("got %+v, %q", c, name)
			}
		})
	}
}

func TestLimiter(t *testing.T) {
	l := newLimiter()
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	for i := 0; i < callsPerMinute; i++ {
		if ok, _ := l.allow(1, now); !ok {
			t.Fatalf("call %d refused", i+1)
		}
	}
	ok, wait := l.allow(1, now.Add(10*time.Second))
	if ok || wait != 50*time.Second {
		t.Errorf("over the minute: ok=%v wait=%v", ok, wait)
	}
	if ok, _ := l.allow(2, now); !ok {
		t.Error("one connection's budget stopped another")
	}
	// The day budget holds across minutes.
	at := now
	for spent := callsPerMinute; spent < callsPerDay; spent += callsPerMinute {
		at = at.Add(time.Minute)
		for i := 0; i < callsPerMinute && spent+i < callsPerDay; i++ {
			if ok, _ := l.allow(1, at); !ok {
				t.Fatalf("refused under the day budget at %d", spent+i)
			}
		}
	}
	if ok, _ := l.allow(1, at.Add(time.Minute)); ok {
		t.Error("allowed past the day budget")
	}
	if ok, _ := l.allow(1, now.Add(24*time.Hour)); !ok {
		t.Error("the day budget didn't come back")
	}
}

func TestCleanClientName(t *testing.T) {
	for in, want := range map[string]string{
		"Claude Code":               "Claude Code",
		"<script>alert(1)</script>": "scriptalert(1)/script",
		"  ChatGPT\n\tconnector  ":  "ChatGPTconnector",
		strings.Repeat("a", 100):    strings.Repeat("a", clientNameMax),
		"Open WebUI (v0.6)":         "Open WebUI (v0.6)",
		"\u202eevil\u200b":          "evil",
	} {
		if got := cleanClientName(in); got != want {
			t.Errorf("cleanClientName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOneLine(t *testing.T) {
	if got := oneLine("a\nb\t c", 10); got != "a b c" {
		t.Errorf("got %q", got)
	}
	got := oneLine(strings.Repeat("é", 20), 9)
	if len(got) > 9 || !strings.HasSuffix(got, "…") || !strings.HasPrefix(got, "é") {
		t.Errorf("got %q", got)
	}
}

// Decided merges applied and turned-down requests, newest answer first, and
// leaves out waiting ones and other campaigns'.
func TestDecided_NewestAnswerFirst(t *testing.T) {
	svc, repo, _, now := newTestService()
	at := func(h int) *time.Time { t := now.Add(time.Duration(h) * time.Hour); return &t }
	repo.reqs = []*Request{
		{ID: 1, CampaignID: "camp-1", Summary: "old applied", Status: RequestAnswered, DecidedAt: at(1)},
		{ID: 2, CampaignID: "camp-1", Summary: "turned down", Status: RequestTurnedDown, DecidedAt: at(3)},
		{ID: 3, CampaignID: "camp-1", Summary: "new applied", Status: RequestAnswered, DecidedAt: at(2)},
		{ID: 4, CampaignID: "camp-1", Summary: "still waiting", Status: RequestWaiting},
		{ID: 5, CampaignID: "camp-2", Summary: "elsewhere", Status: RequestAnswered, DecidedAt: at(4)},
	}
	tests := []struct {
		n    int
		want []string
	}{
		{5, []string{"turned down", "new applied", "old applied"}},
		{2, []string{"turned down", "new applied"}},
	}
	for _, tt := range tests {
		got, err := svc.Decided(context.Background(), "camp-1", tt.n)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, q := range got {
			names = append(names, q.Summary)
		}
		if strings.Join(names, ",") != strings.Join(tt.want, ",") {
			t.Errorf("Decided(%d) = %v, want %v", tt.n, names, tt.want)
		}
	}
}
