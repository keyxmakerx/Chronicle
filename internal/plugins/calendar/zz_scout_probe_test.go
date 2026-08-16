package calendar

// SCOUTING PROBES — not fixes, not guards. Each asserts the CURRENT (suspected
// wrong) behaviour so the report can point at a run rather than at an argument.
// Delete or re-point when the defects are fixed.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// PROBE 4 — "Out this week" resolves the week in UTC, so a member far enough
// east or west of UTC blocks the WRONG WEEK.
//
// rsvp_service.go:528 rsvpWeekDates anchors on now.UTC() for a fantasy calendar
// (and on the event's Y/M/D-in-UTC otherwise) and then walks back to Monday.
// The redeeming member's own zone never enters the calculation, and the email
// form has no zone control either — the link is a one-tap action.
//
// The two cases below are ordinary tabletop realities, not exotica: a New
// Zealand player answering over Monday breakfast, and a Samoan / Hawaiian
// player answering late on Sunday night.
func TestProbe_OutThisWeekResolvesTheWeekInUTCNotTheMembersZone(t *testing.T) {
	fantasy := testCalendar(nil) // no real-time anchor → the "now" fallback
	evt := testEvent(nil)

	cases := []struct {
		name        string
		zone        string
		localMoment string // what the member's own clock reads when they tap
		wantMonday  string // the Monday of THEIR week
	}{
		{"Pacific/Auckland, Monday breakfast", "Pacific/Auckland", "2026-08-17 08:30", "2026-08-17"},
		{"Pacific/Pago_Pago, Sunday night", "Pacific/Pago_Pago", "2026-08-16 22:00", "2026-08-10"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			loc, err := time.LoadLocation(tc.zone)
			if err != nil {
				t.Skipf("tzdata missing for %s", tc.zone)
			}
			local, err := time.ParseInLocation("2006-01-02 15:04", tc.localMoment, loc)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			gotMonday, dates := rsvpWeekDates(fantasy, evt, local.UTC(), "")

			t.Logf("member local %s (%s) → UTC %s", tc.localMoment, tc.zone, local.UTC().Format("2006-01-02 15:04"))
			t.Logf("  blocked week = %s .. %s", dates[0], dates[6])
			t.Logf("  the member's OWN week starts %s", tc.wantMonday)

			if gotMonday == tc.wantMonday {
				t.Fatalf("probe stale: the member's own week was resolved correctly (%s)", gotMonday)
			}
			t.Logf("CONFIRMED: blocked %s, member meant %s — a whole week out, "+
				"and the seven exception rows land on days they never intended",
				gotMonday, tc.wantMonday)
		})
	}
}

// PROBE 5 — the emailed "suggest another time" form asks for wall-clock times
// and never says WHICH ZONE they will be read in.
//
// rsvp_email.go:425 rsvpSuggestPage renders three <input type="date"> +
// <input type="time"> rows with no zone label, no hidden zone field and no
// JavaScript (deliberately — it renders for a possibly-logged-out member).
// rsvp_handler.go:880 parseOfferedWindows turns them into bare minute numbers,
// and app/routes.go:679 stamps them with users.timezone — which defaults to
// "UTC" when the member has never set one (db/migrations/000001: timezone
// VARCHAR(50) DEFAULT NULL).
func TestProbe_EmailedSuggestFormStatesNoTimezone(t *testing.T) {
	page := rsvpSuggestPage("Harvest Feast — Flamerule 27, 2026", "/calendar-rsvp/tok", "csrf", "")

	for _, needle := range []string{"w0date", "w0from", "w0to"} {
		if !strings.Contains(page, needle) {
			t.Fatalf("probe stale: the form no longer carries %q", needle)
		}
	}
	// Nothing on the page names a zone, offers one, or carries one.
	for _, zoneish := range []string{"timezone", "time zone", "UTC", "tz", "zone"} {
		if strings.Contains(strings.ToLower(page), strings.ToLower(zoneish)) {
			t.Fatalf("probe stale: the page mentions %q — a zone may have been added", zoneish)
		}
	}
	t.Log("CONFIRMED: the member types 18:00–22:00 with no idea whose 18:00 it is; " +
		"the server reads it in users.timezone, UTC when unset")
}

// PROBE 6 — an RSVP note, once written, can never be removed by the member.
//
// normalizeRSVPNote (rsvp_service.go:335) maps "" and "   " to nil, and
// UpsertRSVP's ON DUPLICATE KEY does note = COALESCE(VALUES(note), note)
// (rsvp_repository.go:90), so a nil note PRESERVES the stored one. SuggestTime
// refuses an empty note outright. There is no other write path to the column
// and no DELETE. The Owner's Responders[].Note keeps showing it forever.
func TestProbe_RSVPNoteCannotBeCleared(t *testing.T) {
	var wroteNote *string
	var noteWrites int
	repo := &mockRSVPRepo{
		upsertFn: func(_ context.Context, r *EventRSVP) error { wroteNote = r.Note; return nil },
		setNoteFn: func(_ context.Context, _, _, note string) error {
			noteWrites++
			return nil
		},
	}
	evt := testEvent(nil)
	svc := newTestRSVPService(repo, evt, testCalendar(nil))

	// The member tries to clear it by re-answering with an empty note.
	if err := svc.SetMyRSVP(context.Background(), evt, "u1", 1,
		SetRSVPRequest{Status: RSVPYes, Note: probeStr("")}); err != nil {
		t.Fatalf("SetMyRSVP: %v", err)
	}
	if wroteNote != nil {
		t.Fatalf("probe stale: an empty note now reaches the repository as %q", *wroteNote)
	}

	// ...and by whitespace.
	wroteNote = probeStr("sentinel")
	if err := svc.SetMyRSVP(context.Background(), evt, "u1", 1,
		SetRSVPRequest{Status: RSVPYes, Note: probeStr("   ")}); err != nil {
		t.Fatalf("SetMyRSVP: %v", err)
	}
	if wroteNote != nil {
		t.Fatalf("probe stale: whitespace now reaches the repository")
	}

	// ...and through the suggestion path, which refuses empty outright.
	err := svc.SuggestTime(context.Background(), evt, "u1", 1, "")
	if err == nil {
		t.Fatal("probe stale: SuggestTime now accepts an empty note")
	}
	if noteWrites != 0 {
		t.Fatalf("probe stale: %d note writes reached the repo", noteWrites)
	}
	t.Logf("CONFIRMED: every clearing attempt is a no-op (SuggestTime refuses: %v); "+
		"COALESCE(VALUES(note), note) then keeps the old text on the Owner's list forever", err)
}

func probeStr(s string) *string { return &s }

// PROBE 7 — turning "Collect RSVPs" OFF kills every outstanding emailed link,
// including the "you've already answered" reassurance page.
//
// resolveToken (rsvp_handler.go:934) refuses when !evt.CollectRSVPs, and
// rsvpDeadTokenPage (line 726) applies the SAME condition before it will admit
// that an answer exists. A Director who closes collection after the head-count
// therefore turns every link in every inbox into "RSVP Failed — this RSVP link
// is invalid or has expired", which is the exact sentence [GR-8] was written to
// stop players seeing.
func TestProbe_CollectionOffTurnsLiveLinksIntoTheFailurePage(t *testing.T) {
	future := time.Now().UTC().Add(24 * time.Hour)
	used := time.Now().UTC().Add(-time.Hour)
	evt := testEvent(func(e *Event) { e.CollectRSVPs = false }) // Director closed it
	cal := testCalendar(nil)

	repo := &mockRSVPRepo{
		findTokenFn: func(_ context.Context, tok string) (*EventRSVPToken, error) {
			return &EventRSVPToken{Token: tok, EventID: evt.ID, UserID: "u1",
				Action: RSVPActionYes, UsedAt: &used, ExpiresAt: future}, nil
		},
		getUserFn: func(_ context.Context, _, _ string) (*EventRSVP, error) {
			return &EventRSVP{ID: "r1", EventID: evt.ID, UserID: "u1", Status: RSVPYes}, nil
		},
	}
	h := NewRSVPHandler(newTestRSVPService(repo, evt, cal))
	h.SetMemberDirectory(&mockMemberDir{members: []campaigns.CampaignMember{{UserID: "u1", Role: campaigns.RolePlayer}}})

	page := h.rsvpDeadTokenPage(context.Background(), "tok", nil)
	if strings.Contains(page, "already answered") {
		t.Fatal("probe stale: the reassurance page now survives collection being closed")
	}
	if !strings.Contains(page, "RSVP Failed") {
		t.Fatalf("probe stale: unexpected page: %s", page)
	}
	t.Log("CONFIRMED: a member who answered YES, is still on the roster, and re-opens " +
		"their own link is told the link is invalid — after collection was closed")
}

// PROBE 8 — arming "Collect RSVPs" answers `"emailed": true` while the shared
// 24h floor silently suppresses every single invite.
//
// [GR-6] made fanOutInvites read the SAME per-recipient floor the schedule ask
// writes (rsvp_email.go:101,117 — one calendar_schedule_asks log, both paths).
// [GR-10] made this endpoint report its mail state (rsvp_handler.go:330-336).
// The two do not compose: `emailed` is computed from `fannedOut && mailer
// configured` only, so a Director who used "Ask availability" this morning and
// arms the gate this afternoon is told mail is going out when the fan-out will
// skip 100% of the roster.
//
// This is the exact false-honesty class the [GR-10] comment block was written
// to close, reopened one file over.
func TestProbe_CollectRSVPsClaimsEmailedWhileTheFloorSkipsEveryone(t *testing.T) {
	mailer := &mockMailer{configured: true}
	evt := testEvent(func(e *Event) { e.CollectRSVPs = false }) // arming it now
	cal := testCalendar(nil)

	everyoneRecentlyMailed := []string{"u1", "u2", "u3"}
	repo := &mockRSVPRepo{
		recentAskFn: func(_ context.Context, _ string, _ time.Time) ([]string, error) {
			return everyoneRecentlyMailed, nil
		},
	}
	h := NewRSVPHandler(newTestRSVPService(repo, evt, cal))
	h.SetMailSender(mailer, "https://chronicle.test")
	h.SetMemberDirectory(&mockMemberDir{members: []campaigns.CampaignMember{
		{UserID: "u1", Role: campaigns.RolePlayer, Email: "u1@example.test"},
		{UserID: "u2", Role: campaigns.RolePlayer, Email: "u2@example.test"},
		{UserID: "u3", Role: campaigns.RolePlayer, Email: "u3@example.test"},
	}})
	h.SetRSVPNotifier(&mockNotifier{})

	// What the endpoint would put in the body for this call.
	fannedOut := true // req.Enabled && !evt.CollectRSVPs
	claimed := fannedOut && h.mailer != nil && h.mailer.IsConfigured(context.Background())

	// What the fan-out actually does.
	evt.CollectRSVPs = true
	h.fanOutInvites(context.Background(), cal.CampaignID, "Imix", cal, evt, "dm-1")

	mailer.mu.Lock()
	attempts := mailer.attempts
	mailer.mu.Unlock()

	t.Logf(`response body would say "emailed": %v; actual SendHTMLMail attempts: %d`, claimed, attempts)
	if !claimed {
		t.Fatal("probe stale: the endpoint no longer claims mail is going out")
	}
	if attempts != 0 {
		t.Fatalf("probe stale: %d sends happened — the floor may no longer apply to invites", attempts)
	}
	t.Log("CONFIRMED: the operator is told the party was emailed; nobody was. " +
		"There is no notice in the body for this case — mailNotConfiguredLine only " +
		"covers the no-SMTP branch.")
}
