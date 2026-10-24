package sessions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// game_nights_list_test.go pins the Game nights list's member controls: the
// calendar link only when the server says so, and a planned night answered
// from its row, by a real button that never sits inside the row's link.

func renderGameNights(t *testing.T, member bool, userID string, feed bool, list []Session) string {
	t.Helper()
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1", Name: "Saltmarsh"}, IsMember: member}
	var sb strings.Builder
	if err := SessionListFragment(cc, groupSessions(list, "2026-10-24"), "csrf", false, false, userID, SessionPlan{}, feed).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	return sb.String()
}

// rowHTML is one night's row, from its opening tag to the next row's.
func rowHTML(t *testing.T, html, sessionID string) string {
	t.Helper()
	at := strings.Index(html, "/sessions/"+sessionID+`"`)
	if at < 0 {
		t.Fatalf("no row for %s", sessionID)
	}
	start := strings.LastIndex(html[:at], `<div class="gn-row`)
	end := strings.Index(html[at:], `<div class="gn-row`)
	if end < 0 {
		return html[start:]
	}
	return html[start : at+end]
}

func TestGameNightsList_AnswerFromTheRow(t *testing.T) {
	past := "2026-10-01"
	planned := func(id string, attendees ...Attendee) Session {
		return Session{ID: id, Name: "Night " + id, Status: StatusPlanned, Attendees: attendees}
	}
	cases := []struct {
		name     string
		member   bool
		session  Session
		answer   bool   // the Answer pill
		mine     string // the answered badge's class suffix, or ""
		pressed  string // the pick drawn pressed, or ""
		controls bool   // any answer control at all
	}{
		{name: "invited member", member: true, session: planned("s1", Attendee{UserID: "u1", Status: RSVPInvited}), answer: true, controls: true},
		{name: "member with no row yet", member: true, session: planned("s2"), answer: true, controls: true},
		{name: "member going", member: true, session: planned("s3", Attendee{UserID: "u1", Status: RSVPAccepted}), mine: RSVPAccepted, pressed: RSVPAccepted, controls: true},
		{name: "member maybe", member: true, session: planned("s4", Attendee{UserID: "u1", Status: RSVPTentative}), mine: RSVPTentative, pressed: RSVPTentative, controls: true},
		{name: "member can't", member: true, session: planned("s5", Attendee{UserID: "u1", Status: RSVPDeclined}), mine: RSVPDeclined, pressed: RSVPDeclined, controls: true},
		{name: "finished night", member: true, session: Session{ID: "s6", Name: "Done", Status: StatusCompleted, Attendees: []Attendee{{UserID: "u1", Status: RSVPAccepted}}}},
		{name: "cancelled night", member: true, session: Session{ID: "s7", Name: "Off", Status: StatusCancelled}},
		{name: "planned night whose date has passed", member: true, session: Session{ID: "s9", Name: "Last week", Status: StatusPlanned, ScheduledDate: &past,
			Attendees: []Attendee{{UserID: "u1", Status: RSVPInvited}}}},
		{name: "visitor on a public list", member: false, session: planned("s8", Attendee{UserID: "u2", Status: RSVPAccepted})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			row := rowHTML(t, renderGameNights(t, tc.member, "u1", false, []Session{tc.session}), tc.session.ID)
			if got := strings.Contains(row, "data-gn-row"); got != tc.controls {
				t.Fatalf("answer controls = %v, want %v: %s", got, tc.controls, row)
			}
			if got := strings.Contains(row, `class="gn-answer"`); got != tc.answer {
				t.Errorf("Answer pill = %v, want %v", got, tc.answer)
			}
			if strings.Contains(row, "animate-pulse") {
				t.Errorf("the faint pulsing RSVP chip is gone: %s", row)
			}
			if tc.mine != "" && !strings.Contains(row, `class="gn-mine gn-mine-`+tc.mine+`"`) {
				t.Errorf("an answered night shows the answer as a button, want gn-mine-%s: %s", tc.mine, row)
			}
			for _, st := range []string{RSVPAccepted, RSVPTentative, RSVPDeclined} {
				want := tc.controls
				if got := strings.Contains(row, `data-gn-rsvp="`+st+`"`); got != want {
					t.Errorf("pick %s present = %v, want %v", st, got, want)
				}
				pressed := regexp.MustCompile(`data-gn-rsvp="` + st + `" aria-pressed="true"`).MatchString(row)
				if pressed != (tc.pressed == st) {
					t.Errorf("pick %s pressed = %v, want %v", st, pressed, tc.pressed == st)
				}
			}
			if tc.controls && !strings.Contains(row, `data-gn-url="/campaigns/camp-1/sessions/`+tc.session.ID+`/rsvp"`) {
				t.Errorf("the row must carry its own RSVP route: %s", row)
			}
			// The row opens the night from its title link, and no button is
			// ever inside that link.
			link := regexp.MustCompile(`(?s)<a class="gn-link"[^>]*>.*?</a>`).FindString(row)
			if link == "" || strings.Contains(link, "<button") {
				t.Errorf("the title link must exist and hold no button: %q", link)
			}
		})
	}
}

func TestGameNightsList_CalendarLinkOnlyWhenOffered(t *testing.T) {
	on := renderGameNights(t, true, "u1", true, nil)
	for _, want := range []string{"Add to your calendar", "data-gn-feed-toggle", `id="gn-feed"`, "Only you have this link.", "Replace link",
		"Replacing stops the old link at once", "Keep this link", `data-gn-feed-close aria-label="Close"`} {
		if !strings.Contains(on, want) {
			t.Errorf("with the feed on, the page is missing %q", want)
		}
	}
	// The link is made when the panel first opens, never on page load.
	if strings.Contains(on, "/sessions/feed/") && regexp.MustCompile(`/sessions/feed/[A-Za-z0-9_-]+\.ics`).MatchString(on) {
		t.Errorf("the page must not carry a feed link before the member asks for one")
	}
	off := renderGameNights(t, true, "u1", false, nil)
	if strings.Contains(off, "Add to your calendar") || strings.Contains(off, "data-gn-feed") {
		t.Errorf("with the feed off, there is no calendar link")
	}
}

// TestListSessions_CalendarLinkFollowsTheSwitch pins who the handler offers
// the calendar link to: a signed-in member, while the owner's feed switch is on.
func TestListSessions_CalendarLinkFollowsTheSwitch(t *testing.T) {
	cases := []struct {
		name    string
		member  bool
		userID  string
		enabled bool
		want    bool
	}{
		{name: "member, feed on", member: true, userID: "u1", enabled: true, want: true},
		{name: "member, feed off", member: true, userID: "u1", enabled: false, want: false},
		{name: "visitor, feed on", member: false, userID: "", enabled: true, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mockSessionRepo{
				listByCampaignFn: func(context.Context, string) ([]Session, error) { return nil, nil },
				isCalendarFeedEnabledFn: func(context.Context, string) (bool, error) {
					return tc.enabled, nil
				},
			}
			h := NewHandler(NewSessionService(repo, nil, nil))
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/campaigns/c1/sessions", nil)
			req.Header.Set("HX-Request", "true")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			role := campaigns.RolePlayer
			if !tc.member {
				role = campaigns.RoleNone
			}
			c.Set("campaign_context", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: role, IsMember: tc.member})
			if tc.userID != "" {
				c.Set("auth_user_id", tc.userID)
			}
			if err := h.ListSessions(c); err != nil {
				t.Fatalf("ListSessions: %v", err)
			}
			if got := strings.Contains(rec.Body.String(), "Add to your calendar"); got != tc.want {
				t.Errorf("calendar link shown = %v, want %v", got, tc.want)
			}
		})
	}
}
