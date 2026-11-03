package sessions

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/email"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// tokenRecorder mints RSVP tokens through the real service and remembers
// which token carries which answer.
func tokenRecorder() (*mockSessionRepo, map[string]string) {
	byAction := map[string]string{}
	return &mockSessionRepo{createRSVPTokenFn: func(_ context.Context, t *RSVPToken) error {
		byAction[t.Action] = t.Token
		return nil
	}}, byAction
}

func assertBodies(t *testing.T, m *captureMailer, plain, html []string) {
	t.Helper()
	for _, w := range plain {
		if !strings.Contains(m.lastPlain, w) {
			t.Errorf("plain text is missing %q:\n%s", w, m.lastPlain)
		}
	}
	for _, w := range html {
		if !strings.Contains(m.lastHTML, w) {
			t.Errorf("HTML is missing %q:\n%s", w, m.lastHTML)
		}
	}
	for _, body := range []string{m.lastPlain, m.lastHTML} {
		for _, bad := range []string{"🎲", "📅", "✓", "✗", "Session Invite"} {
			if strings.Contains(body, bad) {
				t.Errorf("body carries the old %q", bad)
			}
		}
	}
}

func TestRSVPEmail_OffersMaybeInTheCampaignLook(t *testing.T) {
	mailer := &captureMailer{}
	repo, tokens := tokenRecorder()
	h := &Handler{svc: NewSessionService(repo, nil, nil), mailer: mailer, baseURL: "https://x.test",
		userDir: &stubUserDir{tz: "America/Chicago"}}
	date, at, tz := "2026-10-27", "19:00", "America/New_York"
	session := &Session{ID: "s1", Name: "The Ferryman's Toll", ScheduledDate: &date, ScheduledTime: &at, ScheduledTZ: &tz}
	h.sendRSVPEmails(context.Background(), session, mailCampaign{ID: "c1", Name: "Therin", Accent: "#2563eb"},
		[]campaigns.CampaignMember{{UserID: "u1", Email: "a@b.test"}})

	maybe := tokens[RSVPTentative]
	if maybe == "" {
		t.Fatal("no Maybe token was minted")
	}
	assertBodies(t, mailer,
		[]string{
			"Therin\n\nThe Ferryman's Toll\n\n",
			"Tue, Oct 27, 2026 · 6:00 PM CDT. Can you make it?",
			"Going: https://x.test/rsvp/" + tokens[RSVPAccepted] + "\n",
			"Maybe: https://x.test/rsvp/" + maybe + "\n",
			"Can't make it: https://x.test/rsvp/" + tokens[RSVPDeclined] + "\n",
			`You get this because "A game night is planned or a time is suggested" is on for email.`,
			"https://x.test/account#notifications",
		},
		[]string{"color:#2563eb", `href="https://x.test/rsvp/` + maybe + `"`, ">Maybe<"},
	)
}

func TestRSVPEmail_WithoutADateStillAsks(t *testing.T) {
	mailer := &captureMailer{}
	repo, _ := tokenRecorder()
	h := &Handler{svc: NewSessionService(repo, nil, nil), mailer: mailer, baseURL: "https://x.test"}
	h.sendRSVPEmails(context.Background(), &Session{ID: "s1", Name: "Night"}, mailCampaign{Name: "Camp"},
		[]campaigns.CampaignMember{{UserID: "u1", Email: "a@b.test"}})
	assertBodies(t, mailer, []string{"The date isn't set yet. Can you make it?"}, nil)
}

func TestNightReminderEmail(t *testing.T) {
	for _, tc := range []struct {
		name      string
		recurring bool
		wantMaybe bool
	}{
		{"a one-off night answers in one press", false, true},
		{"a repeating night opens the calendar", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mailer := &captureMailer{}
			repo, tokens := tokenRecorder()
			h := &Handler{svc: NewSessionService(repo, nil, nil), mailer: mailer, baseURL: "https://x.test"}
			session := &Session{ID: "s1", CampaignID: "c1", Name: `<b>Night</b>`}
			night := GameNight{Date: "2026-11-07", Time: "19:00", TZ: "America/Chicago", Recurring: tc.recurring}
			h.sendNightReminderEmails(context.Background(), session, night, mailCampaign{ID: "c1", Name: "Camp"}, "Kara",
				[]campaigns.CampaignMember{{UserID: "u1", Email: "a@b.test"}})

			calendar := "Answer on the calendar: https://x.test" + nightsLink("c1", "s1", "2026-11-07")
			assertBodies(t, mailer, []string{"Kara asked whether you're coming on Sat Nov 7 at 7:00 PM (America/Chicago).", calendar}, []string{"&lt;b&gt;Night"})
			gotMaybe := strings.Contains(mailer.lastPlain, "Maybe: https://x.test/rsvp/")
			if gotMaybe != tc.wantMaybe || (tc.wantMaybe && tokens[RSVPTentative] == "") {
				t.Errorf("Maybe offered = %v, want %v:\n%s", gotMaybe, tc.wantMaybe, mailer.lastPlain)
			}
			if strings.Contains(mailer.lastHTML, "<b>Night") {
				t.Error("the game night's name reached the HTML unescaped")
			}
		})
	}
}

func TestProposalEmail_EachTimeKeepsItsOwnAnswers(t *testing.T) {
	mailer := &captureMailer{}
	repo := &mockSessionRepo{createProposalTokenFn: func(_ context.Context, _ *SlotProposalToken) error { return nil }}
	h := &Handler{svc: NewSessionService(repo, nil, nil), mailer: mailer, userDir: &stubUserDir{tz: "UTC"}, baseURL: "https://x.test"}
	start := time.Date(2026, 11, 7, 19, 0, 0, 0, time.UTC)
	options := []ProposalOptionView{
		{Option: SlotProposalOption{ID: "o1", StartsAtUTC: start, EndsAtUTC: start.Add(3 * time.Hour)}},
		{Option: SlotProposalOption{ID: "o2", StartsAtUTC: start.Add(24 * time.Hour), EndsAtUTC: start.Add(27 * time.Hour)}},
	}
	h.sendProposalEmail(context.Background(), mailCampaign{ID: "c1", Name: "Therin"}, nil, &SlotProposal{ID: "p1", Title: "Next arc"}, options,
		campaigns.CampaignMember{UserID: "u1", Email: "a@b.test"})

	first := renderLocalSlotForTZ(options[0].Option, "UTC")
	second := renderLocalSlotForTZ(options[1].Option, "UTC")
	plain := mailer.lastPlain
	i, j := strings.Index(plain, first.DateLabel), strings.Index(plain, second.DateLabel)
	if i < 0 || j < 0 {
		t.Fatalf("a time is missing:\n%s", plain)
	}
	if n := strings.Count(plain[i:j], "  Yes: https://x.test/proposals/respond/"); n != 1 {
		t.Errorf("the first time carries %d Yes answers, want 1:\n%s", n, plain)
	}
	assertBodies(t, mailer, []string{"Which of these work for you? All times in UTC.", "  Maybe: ", "  No: "}, nil)
}

func TestConfirmTimesEmail(t *testing.T) {
	mailer := &captureMailer{}
	h := &Handler{mailer: mailer, baseURL: "https://x.test", memberLister: &stubMemberLister{members: []campaigns.CampaignMember{
		{UserID: "u1", Email: "a@b.test"},
	}}}
	h.sendConfirmEmails(context.Background(), mailCampaign{ID: "c1", Name: `Vale & Ash`}, "Kara", []string{"u1"})
	assertBodies(t, mailer,
		[]string{"Kara asked everyone in Vale & Ash to check the times they can play.", "Check my times: https://x.test/campaigns/c1/game-nights\n",
			`"The owner asks me to fill in or confirm my free times"`},
		[]string{"Vale &amp; Ash", `href="https://x.test/campaigns/c1/game-nights"`},
	)
}

func TestMailCampaignOf_ReadsTheLayout(t *testing.T) {
	for _, tc := range []struct {
		name     string
		c        *campaigns.Campaign
		want     email.Layout
		wantTerm string
	}{
		{"no campaign is Note", nil, email.Note, ""},
		{"never customised is Note", &campaigns.Campaign{ID: "c1", Name: "Camp"}, email.Note, "GM"},
		{"the picked layout", &campaigns.Campaign{ID: "c1", Settings: `{"appearance":{"email_layout":"chat"}}`}, email.Chat, "GM"},
		{"an unknown layout falls back to Note", &campaigns.Campaign{ID: "c1", Settings: `{"appearance":{"email_layout":"fancy"}}`}, email.Note, "GM"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mc := mailCampaignOf(tc.c)
			if mc.Layout != tc.want || mc.GMTerm != tc.wantTerm {
				t.Errorf("layout %q term %q, want %q %q", mc.Layout, mc.GMTerm, tc.want, tc.wantTerm)
			}
		})
	}
}

func TestSenderAmong(t *testing.T) {
	mc := mailCampaign{GMTerm: "Director"}
	members := []campaigns.CampaignMember{
		{UserID: "o", DisplayName: "Kara", Role: campaigns.RoleOwner},
		{UserID: "p", DisplayName: "Bram", Role: campaigns.RolePlayer},
		{UserID: "n", Role: campaigns.RolePlayer},
	}
	for _, tc := range []struct {
		name, id, fallback string
		want               *email.Sender
	}{
		{"the owner runs the game", "o", "x", &email.Sender{Name: "Kara", Role: "your Director"}},
		{"a player is just named", "p", "x", &email.Sender{Name: "Bram"}},
		{"a member without a name takes the fallback", "n", "Someone", &email.Sender{Name: "Someone"}},
		{"not a member", "z", "Ex", &email.Sender{Name: "Ex"}},
		{"nobody known", "", " ", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := mc.senderAmong(members, tc.id, tc.fallback)
			if (got == nil) != (tc.want == nil) || got != nil && *got != *tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestTicketStub(t *testing.T) {
	str := func(s string) *string { return &s }
	for _, tc := range []struct {
		name string
		s    *Session
		zone string
		want *email.Stub
	}{
		{"no date", &Session{}, "", nil},
		{"a date without a time", &Session{ScheduledDate: str("2026-11-07")}, "UTC", &email.Stub{Weekday: "SAT", Day: "7", Month: "NOV"}},
		{"as stored without a member zone", &Session{ScheduledDate: str("2026-11-07"), ScheduledTime: str("19:30"), ScheduledTZ: str("America/New_York")}, "",
			&email.Stub{Weekday: "SAT", Day: "7", Month: "NOV", Time: "7:30pm"}},
		{"moved into the member's zone, across midnight", &Session{ScheduledDate: str("2026-11-07"), ScheduledTime: str("19:00"), ScheduledTZ: str("America/New_York")}, "Europe/London",
			&email.Stub{Weekday: "SUN", Day: "8", Month: "NOV", Time: "12am"}},
		{"a bad zone keeps the stored clock", &Session{ScheduledDate: str("2026-11-07"), ScheduledTime: str("19:00"), ScheduledTZ: str("Nowhere/Here")}, "UTC",
			&email.Stub{Weekday: "SAT", Day: "7", Month: "NOV", Time: "7pm"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ticketStub(tc.s, tc.zone)
			if (got == nil) != (tc.want == nil) || got != nil && *got != *tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestRSVPEmail_ChatLayoutIsFromTheOrganizer(t *testing.T) {
	mailer := &captureMailer{}
	repo, _ := tokenRecorder()
	h := &Handler{svc: NewSessionService(repo, nil, nil), mailer: mailer, baseURL: "https://x.test"}
	date := "2026-11-07"
	session := &Session{ID: "s1", Name: "Night", CreatedBy: "o", ScheduledDate: &date}
	mc := mailCampaign{ID: "c1", Name: "Therin", Accent: "#2563eb", Layout: email.Chat, GMTerm: "DM"}
	h.sendRSVPEmails(context.Background(), session, mc, []campaigns.CampaignMember{
		{UserID: "o", DisplayName: "Kara", Role: campaigns.RoleOwner},
		{UserID: "u1", Email: "a@b.test"},
	})
	assertBodies(t, mailer, []string{"Going: https://x.test/rsvp/"}, []string{">Kara, your DM</b> in Therin", "background:#2563eb", ">K<"})
}
