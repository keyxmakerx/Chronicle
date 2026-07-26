package calendar

// RSVP invite fan-out + rendered-surface tests (C-CAL-RSVP-P1).
//
// The fan-out is the one place where getting visibility wrong is unrecoverable:
// an email cannot be unsent. These tests pin the two gates (members-only, then
// per-recipient visibility), the nil-safe degradation when SMTP is absent, and
// the escaping of operator-authored text into an HTML body assembled by
// concatenation.

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func newFanOutHandler(evt *Event, dir *mockMemberDir, mailer *mockMailer, notifier *mockNotifier) *RSVPHandler {
	h := NewRSVPHandler(newTestRSVPService(&mockRSVPRepo{}, evt, testCalendar(nil)))
	h.SetMemberDirectory(dir)
	if mailer != nil {
		h.SetMailSender(mailer, "https://chronicle.example.test")
	}
	if notifier != nil {
		h.SetRSVPNotifier(notifier)
	}
	return h
}

// TestFanOut_VisibilityGated is the leak pin: a dm_only event must not put its
// title in a Player's inbox or bell. Only the Owner and the co-DM are reached.
func TestFanOut_VisibilityGated(t *testing.T) {
	evt := testEvent(func(e *Event) { e.Visibility = "dm_only" })
	dir := &mockMemberDir{
		members: []campaigns.CampaignMember{
			{UserID: "owner", Role: campaigns.RoleOwner, Email: "owner@example.test"},
			{UserID: "scribe", Role: campaigns.RoleScribe, Email: "scribe@example.test"},
			{UserID: "player", Role: campaigns.RolePlayer, Email: "player@example.test"},
			{UserID: "codm", Role: campaigns.RolePlayer, Email: "codm@example.test"},
		},
		dmGrant: map[string]bool{"codm": true},
	}
	mailer := &mockMailer{configured: true}
	notifier := &mockNotifier{}

	newFanOutHandler(evt, dir, mailer, notifier).
		fanOutInvites(context.Background(), "camp-1", "Vale of Ash", testCalendar(nil), evt)

	got := strings.Join(mailer.sent, ",")
	for _, blocked := range []string{"scribe@example.test", "player@example.test"} {
		if strings.Contains(got, blocked) {
			t.Errorf("%s must not be emailed about a dm_only event; recipients = %v", blocked, mailer.sent)
		}
	}
	for _, allowed := range []string{"owner@example.test", "codm@example.test"} {
		if !strings.Contains(got, allowed) {
			t.Errorf("%s should be emailed (Owner / co-DM see dm_only); recipients = %v", allowed, mailer.sent)
		}
	}
	// The bell notification rides the SAME gate.
	if strings.Contains(strings.Join(notifier.userIDs, ","), "player") {
		t.Errorf("a Player must not be notified about a dm_only event; got %v", notifier.userIDs)
	}
}

// TestFanOut_MembersOnly pins that the roster is the recipient list. A campaign
// member with no address is skipped for EMAIL but still gets the in-app bell.
func TestFanOut_MembersOnly(t *testing.T) {
	evt := testEvent(nil)
	dir := &mockMemberDir{members: []campaigns.CampaignMember{
		{UserID: "u1", Role: campaigns.RolePlayer, Email: "u1@example.test"},
		{UserID: "u2", Role: campaigns.RolePlayer}, // no address on file
	}}
	mailer := &mockMailer{configured: true}
	notifier := &mockNotifier{}

	newFanOutHandler(evt, dir, mailer, notifier).
		fanOutInvites(context.Background(), "camp-1", "Vale of Ash", testCalendar(nil), evt)

	if len(mailer.sent) != 1 || mailer.sent[0] != "u1@example.test" {
		t.Errorf("only members with an address are emailed; got %v", mailer.sent)
	}
	if len(notifier.userIDs) != 2 {
		t.Errorf("every visible member gets the in-app notification; got %v", notifier.userIDs)
	}
}

// TestFanOut_NilSafeWithoutSMTP pins the dispatch's nil-safety requirement: with
// no mail sender (or an unconfigured one), in-app RSVP still works — the bell
// fires and nothing panics.
func TestFanOut_NilSafeWithoutSMTP(t *testing.T) {
	evt := testEvent(nil)
	dir := &mockMemberDir{members: []campaigns.CampaignMember{
		{UserID: "u1", Role: campaigns.RolePlayer, Email: "u1@example.test"},
	}}

	for _, tc := range []struct {
		name   string
		mailer *mockMailer
	}{
		{"no mail sender wired", nil},
		{"mail sender wired but unconfigured", &mockMailer{configured: false}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			notifier := &mockNotifier{}
			var m *mockMailer
			if tc.mailer != nil {
				m = tc.mailer
			}
			h := newFanOutHandler(evt, dir, m, notifier)
			h.fanOutInvites(context.Background(), "camp-1", "Vale of Ash", testCalendar(nil), evt)

			if len(notifier.userIDs) != 1 {
				t.Errorf("in-app notification must still fire; got %v", notifier.userIDs)
			}
			if tc.mailer != nil && len(tc.mailer.sent) != 0 {
				t.Errorf("an unconfigured mailer must not be used; got %v", tc.mailer.sent)
			}
		})
	}
}

// TestInviteEmail_EscapesOperatorText pins that an event name is inert in the
// HTML body. The body is assembled by concatenation, so escaping is the only
// thing between a crafted name and injected markup.
func TestInviteEmail_EscapesOperatorText(t *testing.T) {
	evt := testEvent(func(e *Event) { e.Name = `Feast <img src=x onerror="alert(1)">` })
	h := newFanOutHandler(evt, &mockMemberDir{}, &mockMailer{configured: true}, nil)

	subject, plain, htmlBody := h.renderInviteEmail(`Vale "of" Ash`, testCalendar(nil), evt, map[string]string{
		RSVPActionYes: "t1", RSVPActionMaybe: "t2", RSVPActionNo: "t3",
		RSVPActionOutWeek: "t4", RSVPActionSuggest: "t5",
	})

	// Assert on the DANGEROUS form specifically: `onerror=` alone also matches
	// the harmless escaped rendering (onerror=&#34;…), so testing for it would
	// pass a leak and fail a correct escape.
	if strings.Contains(htmlBody, "<img src=x") || strings.Contains(htmlBody, `onerror="alert`) {
		t.Errorf("event name must be escaped in the HTML body; got %q", htmlBody)
	}
	if !strings.Contains(htmlBody, "&lt;img") {
		t.Error("expected the escaped form of the event name in the body")
	}
	// The campaign name rides the same escape.
	if !strings.Contains(htmlBody, "Vale &#34;of&#34; Ash") {
		t.Error("campaign name must be escaped in the HTML body")
	}
	if !strings.Contains(subject, "Vale") {
		t.Errorf("subject should name the campaign; got %q", subject)
	}
	// Every action must be reachable from the email.
	for _, tok := range []string{"t1", "t2", "t3", "t4", "t5"} {
		want := "https://chronicle.example.test/calendar-rsvp/" + tok
		if !strings.Contains(htmlBody, want) {
			t.Errorf("HTML body missing link %s", want)
		}
		if !strings.Contains(plain, want) {
			t.Errorf("plain-text body missing link %s", want)
		}
	}
	if !strings.Contains(plain, "expire in 7 days") {
		t.Error("the plain-text body should state the expiry")
	}
}

func TestInviteEmail_CarriesFantasyDateLine(t *testing.T) {
	evt := testEvent(func(e *Event) { e.Year, e.Month, e.Day = 1492, 4, 15 })
	h := newFanOutHandler(evt, &mockMemberDir{}, &mockMailer{configured: true}, nil)
	_, plain, _ := h.renderInviteEmail("Vale of Ash", testCalendar(nil), evt, map[string]string{})

	// Month 4 is the 4th entry (1-based) — "Marpenoth" in the fixture.
	if !strings.Contains(plain, "Marpenoth 15, 1492") {
		t.Errorf("the email must carry the event's in-world date line; got %q", plain)
	}
}

func TestRSVPDateLine_OutOfRangeMonthDegrades(t *testing.T) {
	// A calendar whose months were edited down after the event was authored must
	// still produce a readable line rather than panicking on the emailed path.
	evt := testEvent(func(e *Event) { e.Month = 99 })
	if got := rsvpDateLine(testCalendar(nil), evt); !strings.Contains(got, "month 99") {
		t.Errorf("out-of-range month should degrade to a plain label; got %q", got)
	}
	if got := rsvpDateLine(nil, evt); got == "" {
		t.Error("a nil calendar must not produce an empty date line")
	}
}

// --- standalone token pages ---

func TestTokenPages_EscapeAndCarryCSRF(t *testing.T) {
	evil := `Feast <script>alert(1)</script>`

	confirm := rsvpConfirmPage("Confirm", evil, "/calendar-rsvp/tok", "Confirm — Going", "csrf-123")
	if strings.Contains(confirm, "<script>alert(1)</script>") {
		t.Error("confirm page must escape interpolated text")
	}
	if !strings.Contains(confirm, `value="csrf-123"`) {
		t.Error("confirm page must carry the CSRF token in a hidden field")
	}
	if !strings.Contains(confirm, `<form method="POST" action="/calendar-rsvp/tok">`) {
		t.Error("confirm page must POST back to the token URL")
	}

	suggest := rsvpSuggestPage(evil, "/calendar-rsvp/tok", "csrf-123")
	if strings.Contains(suggest, "<script>alert(1)</script>") {
		t.Error("suggest page must escape interpolated text")
	}
	if !strings.Contains(suggest, `name="note"`) {
		t.Error("suggest page must render the free-text field")
	}
	// The structured rows are the point of the page — a note alone can't be
	// scheduled against.
	for i := 0; i < rsvpSuggestFormRows; i++ {
		for _, part := range []string{"date", "from", "to"} {
			if !strings.Contains(suggest, `name="w`+fmt.Sprint(i)+part+`"`) {
				t.Errorf("suggest page must render row %d's %s field", i, part)
			}
		}
	}
	if !strings.Contains(suggest, `value="csrf-123"`) {
		t.Error("suggest page must carry the CSRF token")
	}

	result := rsvpResultPage("Done", evil, true)
	if strings.Contains(result, "<script>alert(1)</script>") {
		t.Error("result page must escape interpolated text")
	}
	if strings.Contains(result, "<form") {
		t.Error("the terminal result page must offer nothing to submit")
	}
}
