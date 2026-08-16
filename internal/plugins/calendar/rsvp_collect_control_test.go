// rsvp_collect_control_test.go — C-CALV4-GAMEREADY §4 [GR-6] and §5 [GR-10].
//
// THE OPERATOR'S OWN GO/NO-GO GATE. "Collect RSVPs" is the switch the operator
// named as what has to work before they start their game, and it was reachable
// from exactly ONE place in the entire product: the legacy V2 event drawer
// (calendar_v2.templ:1380, wired by event_grid.js, which only calendar_v2.templ
// loads). That shell is a committed deletion. In the rendered DOM,
// `[data-rsvp-collect-toggle]` appeared 1× in v2_month_gm.html and 0× in
// bench_gm.html, and `daycard.templ` contained ZERO occurrences of "rsvp".
//
// Meanwhile EVERY downstream RSVP surface is gated on the flag it sets — the
// Bench session tile and /schedule both run through benchRsvpPickSession — so a
// campaign that could not reach the switch got a player-facing Answer panel
// saying "You: no answer", a three-paragraph caption explaining the options, and
// NO BUTTONS.
package calendar

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// TestRSVPCollect_ControlAudience pins WHO RECEIVES THE CONTROL, on the marker
// attributes rather than on any copy, so a wording change cannot retire the
// guarantee.
//
// THE FLOOR IS NOT THIS TEST'S CHOICE AND NOT THE DISPATCH'S. `routes.go:431`
// already carries RequireRole(RoleScribe) on PUT .../rsvp-collection, and a
// control must never render to somebody the route will reject. So the assertion
// is an agreement between two files, and the failure it prevents is a Player
// clicking a switch that 403s — or worse, a Player reaching the invite moment
// for the party they are a member of.
func TestRSVPCollect_ControlAudience(t *testing.T) {
	base := benchFxData(true, true)

	for _, tc := range []struct {
		name  string
		mount DayCardMount
		want  bool
	}{
		{
			name:  "a Player receives no collect control at all",
			mount: DayCardMount{CampaignID: "camp-1"},
			want:  false,
		},
		{
			// The construction that could not happen by accident but must not
			// happen by refactor either: an authoring viewer whose collect floor
			// is explicitly false renders no control. This is what proves the
			// gate reads its OWN field rather than borrowing CanCreate.
			name:  "an authoring viewer WITHOUT the collect floor still receives nothing",
			mount: DayCardMount{CanCreate: true, CanAuthorDmOnly: true, CampaignID: "camp-1"},
			want:  false,
		},
		{
			name:  "a Scribe at the route's own floor receives it",
			mount: DayCardMount{CanCreate: true, CanCollectRSVPs: true, CampaignID: "camp-1"},
			want:  true,
		},
		{
			name: "an Owner receives it",
			mount: DayCardMount{
				CanCreate: true, CanAuthorDmOnly: true, CanDelete: true,
				CanRestrict: true, CanCollectRSVPs: true, CampaignID: "camp-1",
			},
			want: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := base
			data.DayCard = tc.mount
			body := renderBench(t, data)
			if !strings.Contains(body, "data-cal-daycard") {
				t.Fatal("the card did not mount; every assertion below would be vacuous")
			}
			for _, marker := range []string{"data-de-rsvp-toggle", "data-de-rsvp-hint"} {
				got := strings.Contains(body, marker)
				if got != tc.want {
					if tc.want {
						t.Errorf("%q is missing at a floor the route accepts — the operator "+
							"cannot arm their own gate from v4", marker)
						continue
					}
					t.Errorf("%q is rendered above this viewer's floor — permission is "+
						"ABSENCE, not a disabled control", marker)
				}
			}
		})
	}
}

// TestRSVPCollect_CreateModeHintIsTheV2WordingVerbatim.
//
// [GR-6] rules the CREATE-mode state disabled, carrying the V2 drawer's own
// hint — "reuse the wording verbatim". THIS IS THE ONE PLACE A DISABLED CONTROL
// IS CORRECT, because it is disabled by SEQUENCE and not by PERMISSION:
// CollectRSVPs is deliberately off the shared UpdateEvent path
// (model.go:627-632), so it cannot ride the create payload — there is no event
// to collect against yet. The audience rule above is untouched by it; a
// non-Scribe sees no markup at all rather than a disabled control.
func TestRSVPCollect_CreateModeHintIsTheV2WordingVerbatim(t *testing.T) {
	data := benchFxData(true, true)
	data.DayCard = DayCardMount{CanCreate: true, CanCollectRSVPs: true, CampaignID: "camp-1"}
	body := renderBench(t, data)

	// The V2 drawer's sentence, character for character (calendar_v2.templ:1384).
	const v2Hint = "Save the event first, then invite the party"
	if !strings.Contains(body, v2Hint) {
		t.Errorf("the create-mode hint must be the V2 drawer's wording verbatim; %q is absent", v2Hint)
	}
	// SERVER-RENDERED DISABLED, not disabled by a script that may not have run.
	// The editor opens in create mode, so the markup's own initial state has to
	// be the create-mode state.
	idx := strings.Index(body, "data-de-rsvp-toggle")
	if idx < 0 {
		t.Fatal("no collect control rendered")
	}
	seg := body[max0(idx-220) : idx+80]
	if !strings.Contains(seg, "disabled") {
		t.Errorf("the control must ship DISABLED, not be disabled later by a script; got %q", seg)
	}
}

func max0(i int) int {
	if i < 0 {
		return 0
	}
	return i
}

// --- [GR-10] the PUT reports its mail state ---------------------------------

// TestRSVPCollect_ReportsMailState is the fourth dead end, and the one whose
// consequence is discovered on the day of the session.
//
// THE MEASUREMENT (audit probe P4). With mailer.IsConfigured=false,
// PUT rsvp-collection {"enabled":true} returned HTTP 200 {"collect_rsvps":true}
// — no warning field, no differing status. MAIL ATTEMPTS = 0. The client then
// unconditionally printed "RSVPs are open — the party has been invited." The
// operator arms their gate, reads that, and STOPS CHECKING.
//
// This is the exact condition AskAvailabilityAPI refuses loudly for, using the
// shared mailNotConfiguredLine constant — the invite moment was the one place
// that constant was not used.
//
// THE ASSERTION IS AGAINST THE CONSTANT, NEVER A COPY OF ITS TEXT. A test that
// pasted the sentence would go green against a handler that had drifted from
// the Bench's wording, which is precisely what a single shared constant exists
// to make impossible.
func TestRSVPCollect_ReportsMailState(t *testing.T) {
	for _, tc := range []struct {
		name         string
		mailer       *mockMailer
		wantEmailed  bool
		wantNotice   bool
		alreadyOnRow bool
	}{
		{
			name: "SMTP unconfigured: emailed=false and the honest sentence",
			// The condition the audit measured — and note the arming still
			// SUCCEEDS. Refusing the PUT would break the no-SMTP and
			// fantasy-calendar operators outright, since in-app answering works
			// end to end with no mail server at all.
			mailer: &mockMailer{configured: false}, wantEmailed: false, wantNotice: true,
		},
		{
			name:   "no mail sender wired at all: same answer, same sentence",
			mailer: nil, wantEmailed: false, wantNotice: true,
		},
		{
			name:   "SMTP working: emailed=true and NO notice",
			mailer: &mockMailer{configured: true}, wantEmailed: true, wantNotice: false,
		},
		{
			// Turning collection OFF sends nothing and claims nothing. A notice
			// here would be a true sentence answering a question nobody asked.
			name:   "turning it OFF claims nothing either way",
			mailer: &mockMailer{configured: false}, wantEmailed: false, wantNotice: false,
			alreadyOnRow: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The event starts NOT collecting for the three arming cases, and
			// already collecting for the OFF case — the transition is what
			// decides whether a fan-out happens, so the starting state has to be
			// the one that transition begins from.
			evt := testEvent(func(e *Event) { e.CollectRSVPs = tc.alreadyOnRow })
			h := NewRSVPHandler(newTestRSVPService(&mockRSVPRepo{}, evt, testCalendar(nil)))
			// FIXTURE GROWN, ASSERTIONS UNCHANGED. `emailed` no longer means
			// "SMTP is configured" — it means "mail is going out for THIS call",
			// which is also false when the roster is empty or everyone is inside
			// the 24h per-recipient floor. An empty directory (what this used to
			// pass) is now a genuine nobody-was-emailed case, so the SMTP-working
			// row needs a real addressable member to still be about SMTP.
			h.SetMemberDirectory(&mockMemberDir{members: []campaigns.CampaignMember{
				{UserID: "player-1", Email: "p1@example.test", Role: campaigns.RolePlayer},
			}})
			if tc.mailer != nil {
				h.SetMailSender(tc.mailer, "https://chronicle.example.test")
			}

			enabled := "true"
			if tc.alreadyOnRow {
				enabled = "false"
			}
			body := serveCollectionPUT(t, h, `{"enabled":`+enabled+`}`)

			if got, _ := body["emailed"].(bool); got != tc.wantEmailed {
				t.Errorf("emailed = %v, want %v — the response must report whether mail is "+
					"actually going out", got, tc.wantEmailed)
			}
			notice, hasNotice := body["notice"].(string)
			if hasNotice != tc.wantNotice {
				t.Errorf("notice present = %v, want %v; body = %+v", hasNotice, tc.wantNotice, body)
			}
			if tc.wantNotice && notice != mailNotConfiguredLine {
				t.Errorf("the notice must be the SHARED constant, not a copy of its words:\n"+
					" got %q\nwant %q", notice, mailNotConfiguredLine)
			}
			// The arming itself always succeeds — that is the OUT clause.
			if got, _ := body["collect_rsvps"].(bool); got != (enabled == "true") {
				t.Errorf("collect_rsvps = %v; the arming must succeed regardless of mail state", got)
			}
		})
	}
}

// serveCollectionPUT drives the shipped PUT and decodes its JSON.
func serveCollectionPUT(t *testing.T, h *RSVPHandler, payload string) map[string]any {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPut,
		"/campaigns/camp-1/calendars/cal-1/events/evt-1/rsvp-collection", strings.NewReader(payload))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "calId", "eid")
	c.SetParamValues("camp-1", "cal-1", "evt-1")
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: "camp-1", Name: "Vale of Ash"},
		MemberRole: campaigns.RoleScribe, IsMember: true,
	})
	c.Set("auth_user_id", "gm-1")
	if err := h.SetRSVPCollectionAPI(c); err != nil {
		t.Fatalf("rsvp-collection PUT: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decoding the response: %v (body %q)", err, rec.Body.String())
	}
	return out
}

// --- [GR-6] the 24h per-recipient floor, REUSED --------------------------

// TestRSVPCollect_SecondArmingInsideTheFloorMailsNobodyTwice.
//
// THE HAZARD THIS SLICE MAKES WORSE BEFORE IT MAKES IT BETTER. The audit
// measured that toggling Collect RSVPs off and on RE-MAILS THE ENTIRE ROSTER
// with no cooldown of any kind — and §4 moves the toggle from a legacy drawer
// nobody can find into the day card the GM uses constantly. Easier to reach
// means easier to fan out, twice, by accident, into other people's inboxes,
// which is the one action in calendar-v4 that cannot be retracted.
//
// So the fan-out REUSES the 24h per-recipient floor the schedule-ask path
// already ships and the audit verified working: the same repository read, the
// same SKIP-don't-refuse semantics, the same silence. Not a second limiter with
// its own window and its own bugs.
//
// THE 6h CAMPAIGN COOLDOWN IS NOT APPLIED, and its absence is asserted: that one
// REFUSES the whole action, and refusing to arm the operator's own gate because
// they armed something six hours ago is the trade this slice forbids.
func TestRSVPCollect_SecondArmingInsideTheFloorMailsNobodyTwice(t *testing.T) {
	evt := testEvent(nil)
	mailer := &mockMailer{configured: true}

	// The floor's send log, as a fake table: what was written is what is read
	// back, which is the only way a skip-set test can be about the FLOOR rather
	// than about the mock.
	var recorded []string
	repo := &mockRSVPRepo{
		recordAskFn: func(_ context.Context, a *ScheduleAsk) error {
			recorded = append(recorded, a.RecipientUserID)
			return nil
		},
		recentAskFn: func(_ context.Context, _ string, _ time.Time) ([]string, error) {
			return recorded, nil
		},
	}
	h := NewRSVPHandler(newTestRSVPService(repo, evt, testCalendar(nil)))
	h.SetMailSender(mailer, "https://chronicle.example.test")
	h.SetMemberDirectory(&mockMemberDir{members: []campaigns.CampaignMember{
		{UserID: "u1", Role: campaigns.RolePlayer, Email: "u1@example.test"},
		{UserID: "u2", Role: campaigns.RolePlayer, Email: "u2@example.test"},
	}})

	ctx := context.Background()
	h.fanOutInvites(ctx, "camp-1", "Vale of Ash", testCalendar(nil), evt, "gm-1")
	if len(mailer.sent) != 2 {
		t.Fatalf("the first arming must invite the whole party; sent %v", mailer.sent)
	}

	// OFF, then ON again, ten minutes later. Before this slice that re-mailed
	// everybody.
	h.fanOutInvites(ctx, "camp-1", "Vale of Ash", testCalendar(nil), evt, "gm-1")
	if len(mailer.sent) != 2 {
		t.Errorf("a second arming inside the 24h floor re-mailed the roster: sent %v", mailer.sent)
	}

	// AND THE FLOOR SKIPS RATHER THAN REFUSES: a member who joined after the
	// first arming is invited, and nobody else is mailed twice.
	h.SetMemberDirectory(&mockMemberDir{members: []campaigns.CampaignMember{
		{UserID: "u1", Role: campaigns.RolePlayer, Email: "u1@example.test"},
		{UserID: "u2", Role: campaigns.RolePlayer, Email: "u2@example.test"},
		{UserID: "u3", Role: campaigns.RolePlayer, Email: "newcomer@example.test"},
	}})
	h.fanOutInvites(ctx, "camp-1", "Vale of Ash", testCalendar(nil), evt, "gm-1")
	if len(mailer.sent) != 3 || mailer.sent[2] != "newcomer@example.test" {
		t.Errorf("the floor must SKIP the already-mailed and still invite a new member; sent %v",
			mailer.sent)
	}
}

// TestRSVPCollect_TheBellIsNotRateLimited.
//
// THE FLOOR IS ON EMAIL ONLY, and that is a decision rather than an oversight.
// The in-app bell is free, retractable and already honest; suppressing it would
// hide a real state change from a member who is looking at the product, and
// [GR-10] rules the bell behaviour explicitly OUT of scope.
func TestRSVPCollect_TheBellIsNotRateLimited(t *testing.T) {
	evt := testEvent(nil)
	mailer := &mockMailer{configured: true}
	var recorded []string
	repo := &mockRSVPRepo{
		recordAskFn: func(_ context.Context, a *ScheduleAsk) error {
			recorded = append(recorded, a.RecipientUserID)
			return nil
		},
		recentAskFn: func(_ context.Context, _ string, _ time.Time) ([]string, error) {
			return recorded, nil
		},
	}
	h := NewRSVPHandler(newTestRSVPService(repo, evt, testCalendar(nil)))
	h.SetMailSender(mailer, "https://chronicle.example.test")
	h.SetMemberDirectory(&mockMemberDir{members: []campaigns.CampaignMember{
		{UserID: "u1", Role: campaigns.RolePlayer, Email: "u1@example.test"},
	}})

	notifier := &mockNotifier{}
	h.SetRSVPNotifier(notifier)
	ctx := context.Background()
	h.fanOutInvites(ctx, "camp-1", "Vale of Ash", testCalendar(nil), evt, "gm-1")
	h.fanOutInvites(ctx, "camp-1", "Vale of Ash", testCalendar(nil), evt, "gm-1")

	if len(mailer.sent) != 1 {
		t.Errorf("the EMAIL is floored; sent %v", mailer.sent)
	}
	if len(notifier.userIDs) != 2 {
		t.Errorf("the BELL is not floored — a member looking at the product must see the "+
			"state change both times; notified %v", notifier.userIDs)
	}
}

// TestRSVPCollect_FantasyOnlyCampaignIsToldWhy is [GR-6]'s copy change, and it
// closes the one empty-panel cause the panel could not state about itself.
//
// benchRsvpPickSession skips any row whose calendar is not real-life, and the
// reasoning is sound: an in-world date has no instant and no zone, so a
// zone-labelled real-world time on it would be a fabrication. But a fantasy-only
// campaign that turns Collect RSVPs ON therefore gets NO v4 RSVP surface at all
// and is told NOTHING — which reads as the feature being broken rather than as a
// calendar being missing, and sends the operator debugging on session-zero day.
//
// THE SENTENCE IS NARROW ON PURPOSE, and the two negative rows are what make it
// so: silence is still correct when something real-life is collecting (the panel
// has content) and when nothing is collecting at all (that is the "nobody has
// armed it yet" state, which §4's new control is the answer to).
func TestRSVPCollect_FantasyOnlyCampaignIsToldWhy(t *testing.T) {
	fantasy := &Calendar{ID: "cal-f", CampaignID: "camp-1", Mode: ModeFantasy}
	real := &Calendar{ID: "cal-r", CampaignID: "camp-1", Mode: ModeRealLife}

	for _, tc := range []struct {
		name     string
		upcoming []BlockUpcoming
		want     bool
	}{
		{
			name: "a fantasy-only campaign collecting RSVPs is told it needs a real-world calendar",
			upcoming: []BlockUpcoming{
				{Calendar: fantasy, Event: Event{ID: "e1", CollectRSVPs: true}},
			},
			want: true,
		},
		{
			name: "a campaign with a real-life collecting event is told nothing",
			upcoming: []BlockUpcoming{
				{Calendar: fantasy, Event: Event{ID: "e1", CollectRSVPs: true}},
				{Calendar: real, Event: Event{ID: "e2", CollectRSVPs: true}},
			},
			want: false,
		},
		{
			name: "a campaign collecting nothing at all is told nothing — that is a different state",
			upcoming: []BlockUpcoming{
				{Calendar: fantasy, Event: Event{ID: "e1"}},
			},
			want: false,
		},
		{
			name:     "an empty index says nothing",
			upcoming: nil,
			want:     false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := benchRsvpFantasyOnly(tc.upcoming); got != tc.want {
				t.Fatalf("benchRsvpFantasyOnly = %v, want %v", got, tc.want)
			}
			caps := benchRsvpCaptions(benchRsvpInput{FantasyOnlyRSVP: tc.want}, BenchRsvp{})
			said := false
			for _, c := range caps {
				if strings.Contains(c, "real-world calendar") {
					said = true
				}
			}
			if said != tc.want {
				t.Errorf("the panel said the fantasy-calendar sentence = %v, want %v; captions %q",
					said, tc.want, caps)
			}
		})
	}
}
