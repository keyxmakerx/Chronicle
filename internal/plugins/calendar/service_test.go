// service_test.go: table-driven tests for CalendarService against the
// fakes in mocks_test.go. Covers per-role visibility filtering (including a
// co-DM grant and a public visitor), hidden moons + visibility_rules,
// dm_only authorization on create, input validation, not-found
// unification, and cross-campaign/cross-calendar id scoping.
package calendar

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// --- Viewer fixtures ---
//
// A co-DM's viewer is built exactly the way the handler builds it in
// production (permissions.RequestViewer(cc.VisibilityRole(), userID), see
// handler.go's viewerFrom): campaigns.CampaignContext.VisibilityRole()
// promotes IsDmGranted to RoleOwner for content-visibility purposes, so at
// the service boundary a co-DM's viewer is indistinguishable from an
// Owner's. That promotion is what "a co-DM grant if the viewer supports it"
// means here — there is no separate co-DM code path to test, only that the
// service treats a RoleOwner-valued viewer as trusted regardless of why the
// caller resolved it to that role.
func playerViewer(userID string) permissions.Viewer {
	return permissions.RequestViewer(int(permissions.RolePlayer), userID)
}
func scribeViewer(userID string) permissions.Viewer {
	return permissions.RequestViewer(int(permissions.RoleScribe), userID)
}
func ownerViewer(userID string) permissions.Viewer {
	return permissions.RequestViewer(int(permissions.RoleOwner), userID)
}
func coDMViewer(userID string) permissions.Viewer {
	return permissions.RequestViewer(int(permissions.RoleOwner), userID) // see doc comment above
}
func publicViewer() permissions.Viewer {
	return permissions.RequestViewer(int(permissions.RoleNone), "")
}

const (
	testCampaignA = "camp-a"
	testCampaignB = "camp-b"
)

func strPtr(s string) *string { return &s }

// --- Visibility filter: calendars ---

func TestListCalendars_VisibilityFilterPerRole(t *testing.T) {
	everyone := Calendar{ID: "cal-everyone", CampaignID: testCampaignA, Visibility: "everyone"}
	dmOnly := Calendar{ID: "cal-dm", CampaignID: testCampaignA, Visibility: "dm_only"}
	restricted := Calendar{ID: "cal-restricted", CampaignID: testCampaignA, Visibility: "everyone",
		VisibilityRules: strPtr(`{"allowed_users":["u-1"]}`)}

	repo := &fakeCalendarRepo{
		listByCampaignFn: func(_ context.Context, _ string) ([]Calendar, error) {
			return []Calendar{everyone, dmOnly, restricted}, nil
		},
	}
	svc := newTestCalendarService(repo, nil, nil, nil)

	tests := []struct {
		name   string
		viewer permissions.Viewer
		want   []string // calendar IDs expected, in order
	}{
		{"public visitor sees only everyone, and only if the allow-list admits them", publicViewer(), []string{"cal-everyone"}},
		{"player sees everyone but not dm_only or a restricted allow-list they're not on", playerViewer("u-2"), []string{"cal-everyone"}},
		{"the allow-listed player also sees the restricted calendar", playerViewer("u-1"), []string{"cal-everyone", "cal-restricted"}},
		{"scribe follows the same rule as player (no special calendar-visibility carve-out)", scribeViewer("u-2"), []string{"cal-everyone"}},
		{"owner sees everything, allow-list included", ownerViewer("u-owner"), []string{"cal-everyone", "cal-dm", "cal-restricted"}},
		{"a co-DM (promoted to RoleOwner) sees everything like an owner", coDMViewer("u-codm"), []string{"cal-everyone", "cal-dm", "cal-restricted"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.ListCalendars(context.Background(), testCampaignA, tt.viewer)
			if err != nil {
				t.Fatalf("ListCalendars: %v", err)
			}
			gotIDs := make([]string, len(got))
			for i, c := range got {
				gotIDs[i] = c.ID
			}
			if !equalStrings(gotIDs, tt.want) {
				t.Errorf("got %v, want %v", gotIDs, tt.want)
			}
		})
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// --- Visibility filter: events ---

func TestListEventsForMonth_VisibilityFilterPerUser(t *testing.T) {
	// The repo already applies the SQL dm_only filter keyed on role (see
	// ListEventsForMonth's real implementation); the fake mirrors that same
	// contract so the test exercises the service's ADDITIONAL Go-side
	// visibility_rules filter on top, exactly like production.
	visible := Event{ID: "evt-open", CalendarID: "cal-1", Visibility: "everyone"}
	restricted := Event{ID: "evt-restricted", CalendarID: "cal-1", Visibility: "everyone",
		VisibilityRules: strPtr(`{"denied_users":["u-blocked"]}`)}
	dmOnly := Event{ID: "evt-dm", CalendarID: "cal-1", Visibility: "dm_only"}

	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: testCampaignA}, nil
		},
	}
	eventRepo := &fakeEventRepo{
		listForMonthFn: func(_ context.Context, _ string, _, _, role int) ([]Event, error) {
			all := []Event{visible, restricted}
			if permissions.CanSeeDmOnly(role) {
				all = append(all, dmOnly)
			}
			return all, nil
		},
	}
	svc := newTestCalendarService(calRepo, eventRepo, nil, nil)

	tests := []struct {
		name   string
		viewer permissions.Viewer
		want   []string
	}{
		{"public visitor: open event only, denied by the deny-list is anonymous-denied too (ADR-049)", publicViewer(), []string{"evt-open"}},
		{"denied user does not see the restricted event even though its base visibility is everyone", playerViewer("u-blocked"), []string{"evt-open"}},
		{"a different player sees the restricted event (deny-list names one user, not everyone)", playerViewer("u-other"), []string{"evt-open", "evt-restricted"}},
		{"owner sees dm_only and the deny-listed event both", ownerViewer("u-owner"), []string{"evt-open", "evt-restricted", "evt-dm"}},
		{"co-DM (promoted) sees dm_only like an owner", coDMViewer("u-blocked"), []string{"evt-open", "evt-restricted", "evt-dm"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.ListEventsForMonth(context.Background(), "cal-1", testCampaignA, 100, 1, tt.viewer)
			if err != nil {
				t.Fatalf("ListEventsForMonth: %v", err)
			}
			gotIDs := make([]string, len(got))
			for i, e := range got {
				gotIDs[i] = e.ID
			}
			if !equalStrings(gotIDs, tt.want) {
				t.Errorf("got %v, want %v", gotIDs, tt.want)
			}
		})
	}
}

// TestUpcomingEvents_VisibilityFilterAndOwnCurrentDate pins UpcomingEvents
// (Part B, #764's calendar-preview "Coming up" list): it reads the
// calendar's OWN current date rather than a caller-supplied one, and applies
// the same per-user visibility filter as every other read in this package.
func TestUpcomingEvents_VisibilityFilterAndOwnCurrentDate(t *testing.T) {
	visible := Event{ID: "evt-open", CalendarID: "cal-1", Visibility: "everyone"}
	dmOnly := Event{ID: "evt-dm", CalendarID: "cal-1", Visibility: "dm_only"}

	var gotYear, gotMonth, gotDay int
	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: testCampaignA, CurrentYear: 1024, CurrentMonth: 3, CurrentDay: 12}, nil
		},
	}
	eventRepo := &fakeEventRepo{
		listUpcomingFn: func(_ context.Context, _ string, year, month, day, role, _ int) ([]Event, error) {
			gotYear, gotMonth, gotDay = year, month, day
			all := []Event{visible}
			if permissions.CanSeeDmOnly(role) {
				all = append(all, dmOnly)
			}
			return all, nil
		},
	}
	svc := newTestCalendarService(calRepo, eventRepo, nil, nil)

	got, err := svc.UpcomingEvents(context.Background(), "cal-1", testCampaignA, 5, playerViewer("u-1"))
	if err != nil {
		t.Fatalf("UpcomingEvents: %v", err)
	}
	if gotYear != 1024 || gotMonth != 3 || gotDay != 12 {
		t.Errorf("expected the calendar's own current date (1024-03-12), got %d-%d-%d", gotYear, gotMonth, gotDay)
	}
	if len(got) != 1 || got[0].ID != "evt-open" {
		t.Errorf("a Player must not see the dm_only event, got %v", got)
	}

	got, err = svc.UpcomingEvents(context.Background(), "cal-1", testCampaignA, 5, ownerViewer("u-owner"))
	if err != nil {
		t.Fatalf("UpcomingEvents (owner): %v", err)
	}
	if len(got) != 2 {
		t.Errorf("the owner must see the dm_only event too, got %v", got)
	}
}

// TestGetEventForViewer_DMOnly_HiddenFromPlayerVisibleToOwner pins the
// single-item read path (no SQL role filter, unlike the list) applies the
// exact same dm_only + visibility_rules predicate as the list.
func TestGetEventForViewer_DMOnly_HiddenFromPlayerVisibleToOwner(t *testing.T) {
	evt := Event{ID: "evt-1", CalendarID: "cal-1", Visibility: "dm_only"}
	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: testCampaignA}, nil
		},
	}
	eventRepo := &fakeEventRepo{
		getEventFn: func(_ context.Context, id string) (*Event, error) {
			if id == evt.ID {
				e := evt
				return &e, nil
			}
			return nil, nil
		},
	}
	svc := newTestCalendarService(calRepo, eventRepo, nil, nil)

	if _, err := svc.GetEventForViewer(context.Background(), "evt-1", "cal-1", testCampaignA, playerViewer("u-1")); err == nil {
		t.Error("expected NotFound for a player reading a dm_only event directly by id")
	} else {
		assertNotFound(t, err)
	}
	if _, err := svc.GetEventForViewer(context.Background(), "evt-1", "cal-1", testCampaignA, ownerViewer("u-owner")); err != nil {
		t.Errorf("owner must be able to read a dm_only event: %v", err)
	}
	if _, err := svc.GetEventForViewer(context.Background(), "evt-1", "cal-1", testCampaignA, coDMViewer("u-codm")); err != nil {
		t.Errorf("a co-DM must be able to read a dm_only event like an owner: %v", err)
	}
}

// --- Hidden moons ---

// TestGetCalendarForViewer_HiddenMoonsFilteredForPlayerNotForOwner pins rule
// 1's moon requirement: GetMoons itself returns every moon regardless of
// caller, so the SERVICE must filter HiddenFromPlayers ones for anyone who
// does not skip the per-user layer.
func TestGetCalendarForViewer_HiddenMoonsFilteredForPlayerNotForOwner(t *testing.T) {
	visibleMoon := Moon{ID: 1, CalendarID: "cal-1", Name: "Luna"}
	hiddenMoon := Moon{ID: 2, CalendarID: "cal-1", Name: "Secret Moon", HiddenFromPlayers: true}

	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: testCampaignA, Visibility: "everyone"}, nil
		},
		getMoonsFn: func(_ context.Context, _ string) ([]Moon, error) {
			return []Moon{visibleMoon, hiddenMoon}, nil
		},
	}
	svc := newTestCalendarService(calRepo, nil, nil, nil)

	playerCal, err := svc.GetCalendarForViewer(context.Background(), "cal-1", testCampaignA, playerViewer("u-1"))
	if err != nil {
		t.Fatalf("GetCalendarForViewer (player): %v", err)
	}
	if len(playerCal.Moons) != 1 || playerCal.Moons[0].ID != visibleMoon.ID {
		t.Errorf("player must see only the non-hidden moon, got %+v", playerCal.Moons)
	}

	ownerCal, err := svc.GetCalendarForViewer(context.Background(), "cal-1", testCampaignA, ownerViewer("u-owner"))
	if err != nil {
		t.Fatalf("GetCalendarForViewer (owner): %v", err)
	}
	if len(ownerCal.Moons) != 2 {
		t.Errorf("owner must see every moon including hidden ones, got %+v", ownerCal.Moons)
	}
}

// TestGetCalendarForViewer_ErasAndEventKindsStrippedForPlayer pins that
// event kinds and eras (calendar STRUCTURE, Owner-only end to end) never
// reach a Player through the nested calendar read either.
func TestGetCalendarForViewer_ErasAndEventKindsStrippedForPlayer(t *testing.T) {
	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: testCampaignA, Visibility: "everyone"}, nil
		},
		getErasFn: func(_ context.Context, _ string) ([]Era, error) {
			return []Era{{ID: 1, Name: "Secret Era"}}, nil
		},
	}
	kindRepo := &fakeEventKindRepo{
		listFn: func(_ context.Context, _ string) ([]EventKind, error) {
			return []EventKind{{ID: 1, Name: "Holiday"}}, nil
		},
	}
	svc := newTestCalendarService(calRepo, nil, kindRepo, nil)

	playerCal, err := svc.GetCalendarForViewer(context.Background(), "cal-1", testCampaignA, playerViewer("u-1"))
	if err != nil {
		t.Fatalf("GetCalendarForViewer (player): %v", err)
	}
	if playerCal.Eras != nil {
		t.Errorf("player must not see eras (calendar structure, Owner only), got %+v", playerCal.Eras)
	}
	if playerCal.EventKinds != nil {
		t.Errorf("player must not see event kinds (calendar structure, Owner only), got %+v", playerCal.EventKinds)
	}

	ownerCal, err := svc.GetCalendarForViewer(context.Background(), "cal-1", testCampaignA, ownerViewer("u-owner"))
	if err != nil {
		t.Fatalf("GetCalendarForViewer (owner): %v", err)
	}
	if len(ownerCal.Eras) != 1 || len(ownerCal.EventKinds) != 1 {
		t.Errorf("owner must see eras and event kinds, got eras=%+v kinds=%+v", ownerCal.Eras, ownerCal.EventKinds)
	}
}

// --- Not-found unification ---

// assertNotFound fails the test unless err is an *apperror.AppError with a
// 404 status — the one shape every "doesn't exist / not yours / hidden"
// response in this package must take, so a wrong-campaign or hidden id can
// never be distinguished from a genuinely missing one.
func assertNotFound(t *testing.T, err error) {
	t.Helper()
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected an *apperror.AppError, got %T: %v", err, err)
	}
	if appErr.Code != 404 {
		t.Fatalf("expected 404 not_found, got code=%d type=%s message=%q", appErr.Code, appErr.Type, appErr.Message)
	}
}

// TestGetEventForViewer_NotFoundIsUniform proves the three ways an event
// read can fail (missing id, wrong calendar/campaign, hidden by visibility)
// all produce the identical NotFound — GetEvent's own (nil, nil) contract
// is unified here, not left for a handler to distinguish.
func TestGetEventForViewer_NotFoundIsUniform(t *testing.T) {
	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			if id == "cal-1" {
				return &Calendar{ID: id, CampaignID: testCampaignA}, nil
			}
			return nil, apperror.NewNotFound("calendar not found")
		},
	}
	eventRepo := &fakeEventRepo{
		getEventFn: func(_ context.Context, id string) (*Event, error) {
			switch id {
			case "evt-in-cal1":
				return &Event{ID: id, CalendarID: "cal-1", Visibility: "everyone"}, nil
			case "evt-in-other-cal":
				return &Event{ID: id, CalendarID: "cal-2", Visibility: "everyone"}, nil // belongs elsewhere
			case "evt-dm":
				return &Event{ID: id, CalendarID: "cal-1", Visibility: "dm_only"}, nil
			default:
				return nil, nil // genuinely missing
			}
		},
	}
	svc := newTestCalendarService(calRepo, eventRepo, nil, nil)

	cases := []struct {
		name       string
		eventID    string
		calendarID string
	}{
		{"genuinely missing event id", "evt-does-not-exist", "cal-1"},
		{"event belongs to a different calendar than the one named in the URL", "evt-in-other-cal", "cal-1"},
		{"event visible to nobody below Owner (dm_only)", "evt-dm", "cal-1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.GetEventForViewer(context.Background(), tc.eventID, tc.calendarID, testCampaignA, playerViewer("u-1"))
			assertNotFound(t, err)
		})
	}

	// The wrong-CAMPAIGN case, at the calendar level: :calid must belong to
	// :id. "cal-404" is not served by getByIDFn's known-id branch.
	if _, err := svc.GetEventForViewer(context.Background(), "evt-in-cal1", "cal-404", testCampaignA, playerViewer("u-1")); err == nil {
		t.Error("expected NotFound for a calendar id that does not exist")
	} else {
		assertNotFound(t, err)
	}
}

// --- Cross-campaign / cross-calendar scoping ---

// TestCalendarInCampaign_WrongCampaignIsNotFound pins that a calendar which
// genuinely exists, but in a DIFFERENT campaign than the URL names, answers
// exactly like a missing one — an attacker cannot use the response to learn
// "this id exists, just not here".
func TestCalendarInCampaign_WrongCampaignIsNotFound(t *testing.T) {
	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: testCampaignB}, nil // lives in campaign B
		},
	}
	svc := newTestCalendarService(calRepo, nil, nil, nil)

	_, missingErr := svc.GetCalendarForViewer(context.Background(), "cal-nope", testCampaignA, ownerViewer("u-owner"))
	_, wrongCampaignErr := svc.GetCalendarForViewer(context.Background(), "cal-real-but-b", testCampaignA, ownerViewer("u-owner"))

	assertNotFound(t, wrongCampaignErr)
	var missingAppErr, wrongAppErr *apperror.AppError
	errors.As(missingErr, &missingAppErr)
	errors.As(wrongCampaignErr, &wrongAppErr)
	if missingAppErr == nil || wrongAppErr == nil || missingAppErr.Message != wrongAppErr.Message {
		t.Errorf("a wrong-campaign id must read identically to a missing one: missing=%v wrongCampaign=%v", missingErr, wrongCampaignErr)
	}
}

// TestUpdateEra_CrossCalendarIDIsRejected proves an era genuinely belonging
// to calendar X cannot be edited through a URL naming calendar Y, even
// though UpdateEra's own repo call is calendar-scoped — the missing check
// would be "does calendarID (from the URL) belong to campaignID (from the
// URL)", not "does the era belong to calendarID", which is what this test
// pins by making calendarInCampaign itself fail (wrong campaign).
func TestUpdateEra_CrossCalendarIDIsRejected(t *testing.T) {
	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			if id == "cal-real" {
				return &Calendar{ID: id, CampaignID: testCampaignB}, nil // NOT campaign A
			}
			return nil, apperror.NewNotFound("calendar not found")
		},
		updateEraFn: func(_ context.Context, _ string, _ int, _ EraInput) error {
			t.Fatal("UpdateEra must not reach the repository once the calendar/campaign scoping check fails")
			return nil
		},
	}
	svc := newTestCalendarService(calRepo, nil, nil, nil)

	err := svc.UpdateEra(context.Background(), 1, "cal-real", testCampaignA, UpdateEraInput{Name: "Renamed"})
	assertNotFound(t, err)
}

// TestDeleteEvent_EventFromAnotherCalendarIsRejected proves an event
// belonging to a different calendar than :calid cannot be deleted, even
// when both calendars are in the SAME campaign.
func TestDeleteEvent_EventFromAnotherCalendarIsRejected(t *testing.T) {
	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: testCampaignA}, nil
		},
	}
	eventRepo := &fakeEventRepo{
		getEventFn: func(_ context.Context, id string) (*Event, error) {
			return &Event{ID: id, CalendarID: "cal-other"}, nil // a real event, wrong calendar
		},
		deleteEventFn: func(_ context.Context, _ string) error {
			t.Fatal("DeleteEvent must not reach the repository once the calendar scoping check fails")
			return nil
		},
	}
	svc := newTestCalendarService(calRepo, eventRepo, nil, nil)

	err := svc.DeleteEvent(context.Background(), "evt-1", "cal-mine", testCampaignA, ownerViewer("u-owner"))
	assertNotFound(t, err)
}

// TestSetMoonHidden_WrongCampaignIsRejected pins the same rule for the moon
// hidden-flag toggle: the repository's own SetMoonHidden is calendar-scoped
// SQL, but the calendar/campaign link must still be checked first.
func TestSetMoonHidden_WrongCampaignIsRejected(t *testing.T) {
	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: testCampaignB}, nil
		},
		setMoonHiddenFn: func(_ context.Context, _ string, _ int, _ bool) error {
			t.Fatal("SetMoonHidden must not reach the repository once the calendar/campaign scoping check fails")
			return nil
		},
	}
	svc := newTestCalendarService(calRepo, nil, nil, nil)

	err := svc.SetMoonHidden(context.Background(), 1, "cal-1", testCampaignA, true)
	assertNotFound(t, err)
}

// assertForbidden is assertNotFound's 403 twin — the shape a non-author's
// refused visibility write must take (never a silent downgrade, never a
// generic 500).
func assertForbidden(t *testing.T, err error) {
	t.Helper()
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected an *apperror.AppError, got %T: %v", err, err)
	}
	if appErr.Code != 403 {
		t.Fatalf("expected 403 forbidden, got code=%d type=%s message=%q", appErr.Code, appErr.Type, appErr.Message)
	}
}

// TestCreateEvent_DMOnlyAuthorization pins the create-time half of the
// dm_only authorization rule: CreateEvent has no stored row to weigh a
// visibility CHANGE against, so a caller not authorized to author dm_only
// content is refused outright for attempting to set visibility=dm_only at
// all, never silently downgraded to "everyone" — see canChangeVisibility's
// doc comment on why a downgrade is the wrong fix, and
// CreateEventInput.CanAuthorDmOnly's on why this travels as a plain bool
// rather than a Viewer parameter. visibility_rules is deliberately NOT
// gated here (canChangeVisibility's doc comment says why): a non-author
// setting an allow/deny list on their own new "everyone" event is an
// ordinary content decision within the Scribe route's existing authority.
func TestCreateEvent_DMOnlyAuthorization(t *testing.T) {
	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: testCampaignA}, nil
		},
	}
	svc := newTestCalendarService(calRepo, &fakeEventRepo{}, nil, nil)
	ctx := context.Background()

	t.Run("non-author sending dm_only is refused", func(t *testing.T) {
		_, err := svc.CreateEvent(ctx, "cal-1", testCampaignA, CreateEventInput{Name: "Secret", Visibility: "dm_only"})
		assertForbidden(t, err)
	})
	t.Run("non-author attaching visibility_rules to an everyone event is fine", func(t *testing.T) {
		rules := `{"denied_users":["u-1"]}`
		if _, err := svc.CreateEvent(ctx, "cal-1", testCampaignA, CreateEventInput{Name: "Secret", VisibilityRules: &rules}); err != nil {
			t.Errorf("visibility_rules on an everyone event must not require CanAuthorDmOnly: %v", err)
		}
	})
	t.Run("non-author creating an ordinary everyone event is fine", func(t *testing.T) {
		if _, err := svc.CreateEvent(ctx, "cal-1", testCampaignA, CreateEventInput{Name: "Feast"}); err != nil {
			t.Errorf("an ordinary create must not require CanAuthorDmOnly: %v", err)
		}
	})
	t.Run("an author may create a dm_only event", func(t *testing.T) {
		evt, err := svc.CreateEvent(ctx, "cal-1", testCampaignA, CreateEventInput{Name: "Secret", Visibility: "dm_only", CanAuthorDmOnly: true})
		if err != nil {
			t.Fatalf("an authorized caller must be able to create a dm_only event: %v", err)
		}
		if evt.Visibility != "dm_only" {
			t.Errorf("expected visibility dm_only, got %q", evt.Visibility)
		}
	})
}
