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
	"github.com/keyxmakerx/chronicle/internal/patch"
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

	// THE RECORD'S OWN INVENTORY, READ OFF THE TYPE BY REFLECTION
	// (GETEVENT-FIELD-BOUND-IS-A-DENYLIST, round-4 fix-forward). The bound below
	// used to be a hand-written list of fifteen forbidden names, and a deny-list
	// only catches what whoever wrote it thought of: an `owner_id`, a `notes`, a
	// future `attendees` added to eventEditorRecord would have passed it in
	// silence. That is the exact shape the PAYLOAD's equivalent law was
	// fix-forwarded away from one round earlier (DC2-PAYLOAD-OMITEMPTY), and the
	// two halves of one field law should not be guarded to two standards.
	//
	// Every optional field on the record is `omitempty`, so marshalling a
	// populated literal would have the same blind spot reflection does not: a
	// field cannot be added to the type without appearing here, whatever its tag.
	//
	// The fixture cross-check below is the OTHER half and it stays: reflection
	// catches a field ADDED to the type, the fixture catches a key LEAKING onto
	// the wire that the type does not declare — a raw Event marshalled by mistake,
	// say. Neither subsumes the other.
	wantRecord := []string{
		"all_day", "calendar_id", "category", "collect_rsvps", "day", "description",
		"description_html", "end_day", "end_hour", "end_minute", "end_month",
		"end_year", "entity_id", "id", "is_recurring", "month", "name",
		"recurrence_interval", "recurrence_type", "start_hour", "start_minute",
		"visibility", "visibility_rules", "year",
	}
	declared := jsonKeySet(t, eventEditorRecord{})
	if got := sortedKeys(declared); !equalStrings(got, wantRecord) {
		t.Errorf("eventEditorRecord declares JSON keys %v, want exactly %v — §8 fixes "+
			"this route at ONLY FIELDS THE EDITOR WRITES BACK. A new field here is a "+
			"deliberate widening of the wave's one new read route and belongs in this "+
			"list with a reason, not in the struct alone", got, wantRecord)
	}
	for k := range body {
		if !declared[k] {
			t.Errorf("the response carries the key %q, which eventEditorRecord does not "+
				"declare — this is not a general-purpose event API", k)
		}
	}

	// The named refusals stay under the reflection assertion rather than instead
	// of it. They cost nothing, and each one names a thing somebody genuinely
	// reached for: no campaign data, no member list, no roster, no calendar
	// structure, no authorship, no timestamps.
	//
	// `collect_rsvps` LEFT THIS LIST (C-RSVP-P10, 2026-08-16) and the reason is
	// not that the refusal was wrong in spirit — it is that this key was never
	// an instance of it. The list refuses data about OTHER things (the campaign,
	// the roster, the calendar's structure, who wrote the row); collect_rsvps is
	// this event's own state, authored from this very editor through its own
	// shipped endpoint. Withholding it did not keep the record narrow, it made
	// the editor unable to round-trip a control it renders: the box painted
	// UNCHECKED for an event whose collection was already ON, so there was no
	// gesture that could turn it back OFF, and the hint under it described a
	// first invitation for a party that had already been invited. The same
	// argument DC2-RECUR-DATALOSS made for is_recurring, one field over.
	//
	// It is Scribe-gated, which is asserted separately below — a Player still
	// gets no key at all.
	for _, bad := range []string{
		"campaign_id", "members", "roster", "months", "weekdays", "moons",
		"created_by", "created_at", "updated_at",
		"entity_name", "entity_icon", "entity_color", "color", "icon",
	} {
		if declared[bad] {
			t.Errorf("the record declares %q — this is not a general-purpose event API", bad)
		}
		if _, ok := body[bad]; ok {
			t.Errorf("the record carries %q — this is not a general-purpose event API", bad)
		}
	}
}

// ── DC2-RECUR-DATALOSS — the round-2 fix-forward, both halves ───────────────
//
// The defect the two tests below exist to keep dead: renaming a RECURRING event
// through the day-card editor silently un-repeated it. The editor authors no
// recurrence in this stage (§5's table marks it PARTIAL), so its body carried
// no `is_recurring` key — and on this route OMISSION IS A WRITE, because the
// shipped PUT binds `is_recurring` into a value-typed bool and the service
// writes it unguarded on purpose. Worse, the nil-guarded siblings survive, so
// the row lands in the exact half-state C-CAL-RECURRING-PARTIAL-STATE-CLEANUP
// already had to clean up once: is_recurring=false with recurrence_type,
// recurrence_interval and recurrence_end_* all still populated.
//
// The fix is the record + the client, together, and neither half works alone:
// the record hands the editor what it does not offer, and the editor sends it
// back. Half one is pinned here; half two is pinned on the wire in
// test/js/daycard_editor_requests.test.mjs ("a RECURRING event keeps repeating
// after a title-only save" and "the edit door round-trips the recurrence the
// route now hands it").

// TestGetEvent_TheRecordCarriesRecurrenceSoTheEditorCanRoundTripIt.
func TestGetEvent_TheRecordCarriesRecurrenceSoTheEditorCanRoundTripIt(t *testing.T) {
	evt := dayCardRouteEvent("everyone", nil)
	evt.IsRecurring = true
	evt.RecurrenceType = blockStrPtr("yearly")
	evt.RecurrenceInterval = intPtr(1)

	h := dayCardRouteHandler(dayCardRouteCal("everyone"), evt)
	rec, err := serveGetEvent(h, campaigns.RoleScribe, "u-scribe", "cal-1", "ev-1")
	if err != nil {
		t.Fatalf("GetEventAPI: %v", err)
	}
	var body struct {
		IsRecurring        *bool   `json:"is_recurring"`
		RecurrenceType     *string `json:"recurrence_type"`
		RecurrenceInterval *int    `json:"recurrence_interval"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if body.IsRecurring == nil || !*body.IsRecurring {
		t.Error("the record drops `is_recurring`. The editor cannot round-trip a field it " +
			"was never given, and an omitted key on the shipped PUT is a WRITE of false")
	}
	if body.RecurrenceType == nil || *body.RecurrenceType != "yearly" {
		t.Error("the record drops `recurrence_type`")
	}
	if body.RecurrenceInterval == nil || *body.RecurrenceInterval != 1 {
		t.Error("the record drops `recurrence_interval`")
	}
}

// TestUpdateEvent_IsRecurringAbsentPreserves_ExplicitFalseClears —
// AMENDED IN SWEEP R4 (2026-08-07), and named as such.
//
// This test used to be TestUpdateEvent_AnOmittedIsRecurringIsAWriteOfFalse
// and pinned the opposite fact as a deliberate non-fix: C-CAL-NULL-PRESERVE
// excluded value-typed fields on the reasoning that "IsRecurring — bool:
// false IS the value, not 'absent'", so the write path could not tell an
// author clearing a repeat from a client that never mentioned it, and the
// obligation lived in the client's round-trip.
//
// The coordinator's 2026-08-07 ruling supplies the missing distinction:
// patch.Field carries presence, so absent and false are now different
// things. The old expectation is inverted ON PURPOSE — it was the booked
// hazard, live on three Foundry calendar-sync paths that push five-key
// bodies. The client's round-trip is now redundant rather than load-bearing;
// it is left in place because a client that sends what it means is still
// correct, and the eventEditorRecord comment still explains why it exists.
//
// Both directions are pinned here so the amendment cannot be read as a
// weakening: an explicit false STILL turns recurrence off.
func TestUpdateEvent_IsRecurringAbsentPreserves_ExplicitFalseClears(t *testing.T) {
	run := func(t *testing.T, in UpdateEventInput) *Event {
		t.Helper()
		var written *Event
		repo := &mockCalendarRepo{
			getEventFn: func(_ context.Context, _ string) (*Event, error) { return seededEvent(), nil },
			updateEventFn: func(_ context.Context, evt *Event) error {
				written = evt
				return nil
			},
		}
		if err := newTestCalendarService(repo).UpdateEvent(context.Background(), "evt-1", in); err != nil {
			t.Fatalf("UpdateEvent: %v", err)
		}
		if written == nil {
			t.Fatal("repo.UpdateEvent not called")
		}
		return written
	}

	// Exactly what a title-only save binds to when the body omits the key.
	written := run(t, UpdateEventInput{
		Name: patch.Of("Renamed Title"), Year: patch.Of(1492), Month: patch.Of(7), Day: patch.Of(15),
		Visibility: patch.Of("everyone"),
	})
	if !written.IsRecurring {
		t.Fatal("an OMITTED is_recurring turned recurrence off; absent must preserve")
	}
	if written.RecurrenceType == nil || *written.RecurrenceType != "yearly" {
		t.Fatal("recurrence_type stopped surviving the write")
	}

	// …and an author who actually clears the repeat still clears it.
	written = run(t, UpdateEventInput{
		Name: patch.Of("Renamed Title"), Year: patch.Of(1492), Month: patch.Of(7), Day: patch.Of(15),
		Visibility: patch.Of("everyone"), IsRecurring: patch.Of(false),
	})
	if written.IsRecurring {
		t.Fatal("an EXPLICIT is_recurring=false must still turn recurrence off")
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
		// handler overrides the default two-fixture repo for the one case that
		// needs a SECOND resolvable calendar. Nil means the default.
		handler func() *Handler
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
		{
			// THE OWNERSHIP BRANCH, and it is the row this table was missing
			// (DC2-W5A-OWNERSHIP, round-2 fix-forward). The handler's own
			// header claims FOUR conditions answer alike; only three were
			// pinned here, and the unpinned one is the branch that separates
			// "this event exists on a calendar you can see" from "this event
			// exists on one you cannot" — the exact distinction a pairing
			// attack is looking for. TestGetEvent_TheCalIdMustOwnTheEvent
			// asserts the branch REFUSES; this asserts it refuses THE SAME WAY.
			//
			// It needs both calendars resolvable and both inside camp-1, so
			// that requireEventInCampaign passes and the ownership check is
			// what actually fires — otherwise the case silently re-tests the
			// cross-campaign gate above.
			name:  "the :calId is visible but does not own the event",
			calID: "cal-1", eid: "ev-1",
			handler: func() *Handler {
				visible := dayCardRouteCal("everyone")
				sibling := &Calendar{ID: "cal-sibling", CampaignID: "camp-1", Visibility: "everyone"}
				evt := dayCardRouteEvent("everyone", nil)
				evt.CalendarID = sibling.ID
				return NewHandler(NewCalendarService(&mockCalendarRepo{
					getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
						switch id {
						case visible.ID:
							return visible, nil
						case sibling.ID:
							return sibling, nil
						}
						return nil, apperror.NewNotFound("calendar not found")
					},
					getEventFn: func(_ context.Context, id string) (*Event, error) {
						if id == evt.ID {
							return evt, nil
						}
						return nil, apperror.NewNotFound("event not found")
					},
				}))
			},
		},
	}

	var first string
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := dayCardRouteHandler(tc.cal, tc.evt)
			if tc.handler != nil {
				h = tc.handler()
			}
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

// ── C-RSVP-P10: the arming state has to survive the round trip ──────────────
//
// THE GAP THIS CLOSES IS BETWEEN TWO PASSING SUITES, WHICH IS WHY IT IS HERE
// AND NOT IN EITHER OF THEM. `collect_rsvps` was pinned twice, in opposite
// directions, by tests that never saw each other:
//
//   - this file asserted the key must be ABSENT from the response;
//   - test/js/daycard_rsvp_collect.test.mjs built a fixture event that CARRIED
//     it and asserted the checkbox follows it.
//
// Both were green for the whole of the feature's life, and the product shipped
// a checkbox that rendered UNCHECKED for an event whose RSVP collection was
// already ON — which left the GM no gesture to turn it off, because you cannot
// uncheck a box that is already drawn unchecked.
//
// The lesson is the same one the week/day-view revert taught: a test that feeds
// a hand-built fixture never discovers that the producer does not build it. So
// this asserts the SERVER's own record, for the same event state the JS test
// hands its widget.

func TestGetEvent_CarriesTheRSVPArmingStateToItsOwnEditor(t *testing.T) {
	for _, tc := range []struct {
		name  string
		armed bool
	}{
		{"armed", true},
		{"not armed", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			evt := dayCardRouteEvent("everyone", nil)
			evt.CollectRSVPs = tc.armed
			h := dayCardRouteHandler(dayCardRouteCal("everyone"), evt)

			rec, err := serveGetEvent(h, campaigns.RoleScribe, "u-1", "cal-1", "ev-1")
			if err != nil {
				t.Fatalf("GetEventAPI: %v", err)
			}
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("response is not JSON: %v", err)
			}
			got, ok := body["collect_rsvps"]
			if !ok {
				t.Fatalf("a Scribe's record has no collect_rsvps key — the day card's "+
					"checkbox paints from this and would render unchecked for an event "+
					"whose collection is %v, leaving no way to turn it off", tc.armed)
			}
			if got != tc.armed {
				t.Errorf("collect_rsvps = %v, want %v", got, tc.armed)
			}
		})
	}
}

// A Player never sees the key at all. The control is Scribe-gated in the
// producer (daycard.templ's CanCollectRSVPs), so shipping the state to a
// Player would be telling them something no surface of theirs can act on.
func TestGetEvent_RSVPArmingStateIsScribeGated(t *testing.T) {
	evt := dayCardRouteEvent("everyone", nil)
	evt.CollectRSVPs = true
	h := dayCardRouteHandler(dayCardRouteCal("everyone"), evt)

	rec, err := serveGetEvent(h, campaigns.RolePlayer, "u-1", "cal-1", "ev-1")
	if err != nil {
		t.Fatalf("GetEventAPI: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if _, ok := body["collect_rsvps"]; ok {
		t.Error("a Player's record carries collect_rsvps; it is gated with visibility " +
			"and visibility_rules and must travel with them")
	}
}
