package sessions

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// sentMail is one email a test mailer was handed.
type sentMail struct{ to, subject, plain, html string }

type listMailer struct{ sent []sentMail }

func (m *listMailer) SendHTMLMail(_ context.Context, to []string, subject, plain, html string) error {
	m.sent = append(m.sent, sentMail{to[0], subject, plain, html})
	return nil
}

func (m *listMailer) IsConfigured(context.Context) bool { return true }

func TestEmailNightChange(t *testing.T) {
	str := func(s string) *string { return &s }
	before := &Session{ID: "s1", Name: "Night", ScheduledDate: str("2026-11-07"), ScheduledTime: str("19:00"), ScheduledTZ: str("UTC")}
	moved := &Session{ID: "s1", Name: "Night", ScheduledDate: str("2026-11-08"), ScheduledTime: str("19:00"), ScheduledTZ: str("UTC")}
	repeating := *moved
	repeating.IsRecurring = true
	members := []campaigns.CampaignMember{
		{UserID: "owner", DisplayName: "Kara", Role: campaigns.RoleOwner, Email: "kara@x.test"},
		{UserID: "going", Email: "going@x.test"},
		{UserID: "maybe", Email: "maybe@x.test"},
		{UserID: "cant", Email: "cant@x.test"},
		{UserID: "muted", Email: "muted@x.test"},
		{UserID: "noaddr"},
	}
	for _, tc := range []struct {
		name        string
		change      nightChange
		before, s   *Session
		subject     string
		plain       []string
		refuse      []string
		wantAnswers bool
	}{
		{"a one-off night moved asks again", nightMoved, before, moved, "Moved: Night — Therin",
			[]string{"It has moved to Sun, Nov 8, 2026", "It was Sat, Nov 7, 2026", "Can you still make it?", "Maybe: https://x.test/rsvp/", "/suggest"},
			nil, true},
		{"a repeating night moved links to its page", nightMoved, before, &repeating, "Moved: Night — Therin",
			[]string{"Check my answer: https://x.test/campaigns/c1/sessions/s1"}, []string{"/rsvp/"}, false},
		{"cancelled keeps the answer", nightCancelled, nil, moved, "Cancelled: Night — Therin",
			[]string{"The game night on Sun, Nov 8, 2026", "is cancelled.", "Your answer is kept", "See game nights: https://x.test/campaigns/c1/game-nights"},
			[]string{"/rsvp/"}, false},
		{"back on", nightBackOn, nil, moved, "Back on: Night — Therin",
			[]string{"It's back on for Sun, Nov 8, 2026", "See the game night: https://x.test/campaigns/c1/sessions/s1",
				`"A game night moves, is cancelled or comes back" is on for email.`},
			[]string{"/rsvp/"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mailer := &listMailer{}
			repo, _ := tokenRecorder()
			repo.listGoingOrMaybeUserIDsFn = func(context.Context, string, string) ([]string, error) {
				return []string{"going", "maybe", "muted", "noaddr"}, nil
			}
			h := &Handler{svc: serviceOn(repo, "2026-11-01"), mailer: mailer, baseURL: "https://x.test",
				memberLister: &stubMemberLister{members: members}, recipients: muteFilter{muted: map[string]bool{"muted": true}}}
			mc := mailCampaign{ID: "c1", Name: "Therin", GMTerm: "GM"}
			h.emailNightChange(context.Background(), mc, tc.change, tc.before, tc.s, "owner")

			var to []string
			for _, m := range mailer.sent {
				to = append(to, m.to)
			}
			if strings.Join(to, ",") != "going@x.test,maybe@x.test" {
				t.Fatalf("emailed %v, want only who said Going or Maybe, has an address and wants it", to)
			}
			m := mailer.sent[0]
			if m.subject != tc.subject {
				t.Errorf("subject = %q, want %q", m.subject, tc.subject)
			}
			for _, w := range tc.plain {
				if !strings.Contains(m.plain, w) {
					t.Errorf("plain text is missing %q:\n%s", w, m.plain)
				}
			}
			for _, bad := range tc.refuse {
				if strings.Contains(m.plain, bad) {
					t.Errorf("plain text carries %q:\n%s", bad, m.plain)
				}
			}
			if got := strings.Contains(m.plain, "Going: https://x.test/rsvp/"); got != tc.wantAnswers {
				t.Errorf("one-press answers = %v, want %v", got, tc.wantAnswers)
			}
		})
	}
}

func TestEmailNightChange_ChatShowsWhoChangedIt(t *testing.T) {
	mailer := &listMailer{}
	repo := &mockSessionRepo{listGoingOrMaybeUserIDsFn: func(context.Context, string, string) ([]string, error) { return []string{"p"}, nil }}
	h := &Handler{svc: serviceOn(repo, "2026-11-01"), mailer: mailer, baseURL: "https://x.test",
		memberLister: &stubMemberLister{members: []campaigns.CampaignMember{
			{UserID: "o", DisplayName: "Kara", Role: campaigns.RoleOwner},
			{UserID: "p", Email: "p@x.test"},
		}}}
	h.emailNightChange(context.Background(), mailCampaign{ID: "c1", Name: "Therin", Layout: "chat", GMTerm: "DM"},
		nightCancelled, nil, &Session{ID: "s1", Name: "Night"}, "o")
	if len(mailer.sent) != 1 || !strings.Contains(mailer.sent[0].html, ">Kara, your DM</b> in Therin") {
		t.Fatalf("want one Chat email from Kara, got %+v", mailer.sent)
	}
	if !strings.Contains(mailer.sent[0].plain, "This game night is cancelled.") {
		t.Errorf("an undated night still says it is cancelled:\n%s", mailer.sent[0].plain)
	}
}

func TestEmailNightChange_NobodyToTell(t *testing.T) {
	mailer := &listMailer{}
	h := &Handler{svc: serviceOn(&mockSessionRepo{}, "2026-11-01"), mailer: mailer,
		memberLister: &stubMemberLister{members: []campaigns.CampaignMember{{UserID: "p", Email: "p@x.test"}}}}
	h.emailNightChange(context.Background(), mailCampaign{ID: "c1"}, nightCancelled, nil, &Session{ID: "s1"}, "")
	if len(mailer.sent) != 0 {
		t.Errorf("nobody said Going or Maybe, but %d emails went out", len(mailer.sent))
	}
}
