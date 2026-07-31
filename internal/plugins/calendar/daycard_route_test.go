package calendar

// daycard_route_test.go — §8's security review, in executable form
// (C-CALV4-DAYCARD, R2-2a, [DC-8] SIGNED).
//
// THIS SLICE SPENDS ITS WHOLE ROUTE BUDGET ON ONE READ, and each heading below
// is one claim from the review the route is paid for with. The pattern is
// block_prefs_route_test.go's, deliberately: the wave's previous single new
// route was reviewed this way and re-authoring the shape would fork the
// discipline.
//
// THE CLAIM THAT MATTERS MOST IS THE W5a ONE, and it is the hardest to hold by
// eye: an event on a calendar this viewer cannot see, an event their own filter
// removes, an event whose :calId does not own it, and an event id that does not
// exist must be INDISTINGUISHABLE — one branch, one body, one status. Not
// 403-vs-404. Not a different message. app_dashboard.go:96 states why in
// capitals: UNIFYING THE TWO REOPENS THE W5a LEAK.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// dayCardRouteCal is the calendar the fixture event lives on.
func dayCardRouteCal(visibility string) *Calendar {
	return &Calendar{
		ID: "cal-1", CampaignID: "camp-1", Name: "Harptos", Visibility: visibility,
		CurrentYear: 1523, CurrentMonth: 1, CurrentDay: 14,
	}
}

func dayCardRouteEvent(visibility string, rules *string) *Event {
	desc := "the long prose the card never prints"
	return &Event{
		ID: "ev-1", CalendarID: "cal-1", Name: "Council of Wards",
		Description: &desc,
		Year:        1523, Month: 1, Day: 3,
		Visibility: visibility, VisibilityRules: rules,
	}
}

// serveGetEvent drives GetEventAPI the way the editor would.
func serveGetEvent(h *Handler, role campaigns.Role, userID, calID, eventID string) (*httptest.ResponseRecorder, error) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet,
		"/campaigns/camp-1/calendars/"+calID+"/events/"+eventID, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "calId", "eid")
	c.SetParamValues("camp-1", calID, eventID)
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: role, IsMember: true,
	})
	if userID != "" {
		c.Set("auth_user_id", userID)
	}
	return rec, h.GetEventAPI(c)
}

func dayCardRouteHandler(cal *Calendar, evt *Event) *Handler {
	return NewHandler(NewCalendarService(&mockCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			if cal != nil && id == cal.ID {
				return cal, nil
			}
			return nil, apperror.NewNotFound("calendar not found")
		},
		getEventFn: func(_ context.Context, id string) (*Event, error) {
			if evt != nil && id == evt.ID {
				return evt, nil
			}
			return nil, apperror.NewNotFound("event not found")
		},
	}))
}

// §8: "What it returns: one event record, and ONLY fields the editor writes
// back." It is not a general-purpose event API and must not become one.
func TestGetEvent_ReturnsTheEditorsFieldsAndNothingElse(t *testing.T) {
	h := dayCardRouteHandler(dayCardRouteCal("everyone"), dayCardRouteEvent("everyone", nil))
	rec, err := serveGetEvent(h, campaigns.RoleScribe, "u-scribe", "cal-1", "ev-1")
	if err != nil {
		t.Fatalf("GetEventAPI: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	for _, want := range []string{"id", "calendar_id", "name", "year", "month", "day"} {
		if _, ok := body[want]; !ok {
			t.Errorf("the editor cannot open without %q", want)
		}
	}
	// The route's own bound: no campaign data, no member list, no roster, no
	// audience resolution, no calendar structure, no authorship, no timestamps.
	for _, bad := range []string{
		"campaign_id", "members", "roster", "months", "weekdays", "moons",
		"created_by", "created_at", "updated_at", "collect_rsvps",
		"entity_name", "entity_icon", "entity_color", "color", "icon",
	} {
		if _, ok := body[bad]; ok {
			t.Errorf("the record carries %q — this is not a general-purpose event API", bad)
		}
	}
}

// §8 + [DC-9]: the ROLE FLOOR IS NOT THE SECURITY BOUNDARY. A player may read
// an event they can already see — the card lists it and the Ledger prints it —
// but `visibility_rules` NAMES OTHER USERS and neither audience field is theirs.
func TestGetEvent_TheAudienceFieldsRideTheAuthoringFloor(t *testing.T) {
	rules := `{"allowed_users":["u-gm","u-nissa"]}`
	for _, tc := range []struct {
		name      string
		role      campaigns.Role
		wantThose bool
	}{
		{"player", campaigns.RolePlayer, false},
		{"scribe", campaigns.RoleScribe, true},
		{"owner", campaigns.RoleOwner, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := dayCardRouteHandler(dayCardRouteCal("everyone"),
				dayCardRouteEvent("everyone", &rules))
			// The viewer is INSIDE the allow-list, so the filter admits them at
			// every floor and the only variable left is the role.
			rec, err := serveGetEvent(h, tc.role, "u-nissa", "cal-1", "ev-1")
			if err != nil {
				t.Fatalf("GetEventAPI: %v", err)
			}
			has := strings.Contains(rec.Body.String(), "visibility_rules")
			if has != tc.wantThose {
				t.Errorf("visibility_rules present = %v, want %v", has, tc.wantThose)
			}
			// The rules arrive as a JSON STRING, so the member names are
			// escaped inside it — match the bare name rather than a quoted one.
			if got := strings.Contains(rec.Body.String(), "u-nissa"); got != tc.wantThose {
				t.Errorf("a %s %s the allow-list's member names", tc.name,
					map[bool]string{true: "should receive", false: "must not receive"}[tc.wantThose])
			}
		})
	}
}

// §8, THE W5a CLAUSE. Four ways to be refused, ONE answer. If any of these
// diverges — a different status, a different message, a different shape — the
// route becomes an oracle for the existence of events the viewer may not see.
func TestGetEvent_HiddenFilteredAndMissingAreTheSameAnswer(t *testing.T) {
	dm := "dm_only"
	cases := []struct {
		name  string
		cal   *Calendar
		evt   *Event
		calID string
		eid   string
	}{
		{
			name: "the event does not exist",
			cal:  dayCardRouteCal("everyone"), evt: nil, calID: "cal-1", eid: "ev-nope",
		},
		{
			name: "the event is dm_only and the viewer is a player",
			cal:  dayCardRouteCal("everyone"), evt: dayCardRouteEvent(dm, nil),
			calID: "cal-1", eid: "ev-1",
		},
		{
			name:  "the event's audience excludes this player",
			cal:   dayCardRouteCal("everyone"),
			evt:   dayCardRouteEvent("everyone", blockStrPtr(`{"allowed_users":["u-gm"]}`)),
			calID: "cal-1", eid: "ev-1",
		},
		{
			name: "the CALENDAR is hidden from this player",
			cal:  dayCardRouteCal("dm_only"), evt: dayCardRouteEvent("everyone", nil),
			calID: "cal-1", eid: "ev-1",
		},
		{
			name: "the calendar does not exist",
			cal:  dayCardRouteCal("everyone"), evt: dayCardRouteEvent("everyone", nil),
			calID: "cal-other", eid: "ev-1",
		},
	}

	var first string
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := dayCardRouteHandler(tc.cal, tc.evt)
			rec, err := serveGetEvent(h, campaigns.RolePlayer, "u-bryn", tc.calID, tc.eid)
			if err == nil {
				t.Fatalf("the route answered %d instead of refusing", rec.Code)
			}
			got := err.Error()
			if i == 0 {
				first = got
				return
			}
			if got != first {
				t.Errorf("refusal = %q; the first case refused with %q. HIDDEN, FILTERED "+
					"AND MISSING MUST BE THE SAME ANSWER — a difference here is an oracle "+
					"for the existence of events this viewer may not see", got, first)
			}
		})
	}
}

// §8's IDOR clause, the half that is easy to forget: :calId must not be trusted
// to own :eid. An attacker who knows an event id on a calendar they CAN see
// must not be able to read one on a calendar they cannot, by pairing them.
func TestGetEvent_TheCalIdMustOwnTheEvent(t *testing.T) {
	evt := dayCardRouteEvent("everyone", nil)
	evt.CalendarID = "cal-other"
	h := dayCardRouteHandler(dayCardRouteCal("everyone"), evt)
	if _, err := serveGetEvent(h, campaigns.RoleOwner, "u-gm", "cal-1", "ev-1"); err == nil {
		t.Error("the route trusted a :calId that does not own :eid")
	}
}

// §8's cross-campaign clause. requireEventInCampaign is the shipped gate and
// this route rides it exactly as UpdateEventAPI does; the assertion exists
// because "it calls the same helper" is a claim about a line, not a behaviour.
func TestGetEvent_CrossCampaignEventIsNotFound(t *testing.T) {
	cal := dayCardRouteCal("everyone")
	other := &Calendar{ID: "cal-1", CampaignID: "camp-other"}
	h := NewHandler(NewCalendarService(&mockCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			if id == "cal-1" {
				return other, nil
			}
			return cal, nil
		},
		getEventFn: func(_ context.Context, _ string) (*Event, error) {
			return dayCardRouteEvent("everyone", nil), nil
		},
	}))
	if _, err := serveGetEvent(h, campaigns.RoleOwner, "u-gm", "cal-1", "ev-1"); err == nil {
		t.Error("an event on another campaign's calendar was readable")
	}
}

// §8's SNAPSHOT clause, asserted rather than remembered: exactly ONE literal
// path was added, it is registered on the authed cg group, and the Owner-floor
// event-categories GET was NOT widened ([DC-8](c) resolved to option ii — the
// palette is answered from the page payload instead).
func TestGetEvent_TheRouteIsLiteralAndTheAuthSurfaceDidNotWiden(t *testing.T) {
	src := readRepoFile(t, "internal/plugins/calendar/routes.go")

	want := `cg.GET("/calendars/:calId/events/:eid", h.GetEventAPI, campaigns.RequireRole(campaigns.RolePlayer))`
	if !strings.Contains(src, want) {
		t.Errorf("routes.go does not register the literal path — a route registered through "+
			"a variable is silently NOT snapshotted (wire_contract_test.go:180-188). Want:\n%s", want)
	}

	// (c): the categories GET keeps its Owner floor. Its METHOD/PATH pair never
	// moves, so the wire oracle could not have caught a change here — this is
	// the assertion that stands in for it.
	if !strings.Contains(src,
		`cg.GET("/calendars/:calId/event-categories", h.GetEventCategoriesAPI, campaigns.RequireRole(campaigns.RoleOwner))`) {
		t.Error("the event-categories GET's Owner floor moved. [DC-8](c) was resolved to " +
			"option ii — answer the palette from the page payload — precisely so this " +
			"auth surface would NOT widen, and a middleware change here is invisible to " +
			"routes_snapshot.txt because the METHOD/PATH pair is unchanged")
	}

	snapshot := readRepoFile(t, "internal/wire/routes_snapshot.txt")
	line := "GET\t/calendars/:calId/events/:eid\tinternal/plugins/calendar/routes.go"
	if !strings.Contains(snapshot, line) {
		t.Error("the new route is not in routes_snapshot.txt; regenerate it once, last")
	}
	// Exactly one GET for that path. PUT and DELETE already shipped on the same
	// path, so the count is taken on the whole line rather than on the path.
	if n := strings.Count(snapshot, line); n != 1 {
		t.Errorf("the snapshot carries %d GET entries for the new path; want exactly 1", n)
	}
}

// §8's EGRESS clause. This route adds no field to any export or AI-workspace
// DTO — the rsvp_egress_test.go precedent, applied to the one type this slice
// introduced. eventEditorRecord is a HANDLER-local projection and nothing
// serialises it but this route.
func TestGetEvent_TheRecordTypeHasExactlyOneProducer(t *testing.T) {
	src := readRepoFile(t, "internal/plugins/calendar/handler.go")
	if n := strings.Count(src, "newEventEditorRecord("); n != 2 {
		t.Errorf("newEventEditorRecord has %d references (want 2: its definition and its "+
			"ONE call site). A second producer is this record escaping into a surface "+
			"whose audience nobody reviewed", n)
	}
	for _, f := range []string{"export.go", "api_handler.go", "subresource_v2.go"} {
		if strings.Contains(readRepoFile(t, "internal/plugins/calendar/"+f), "eventEditorRecord") {
			t.Errorf("%s references eventEditorRecord — this route adds no field to any "+
				"export or module-API DTO", f)
		}
	}
}
