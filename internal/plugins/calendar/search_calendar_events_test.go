package calendar

// search_calendar_events_test.go pins SearchCalendarEvents' player
// protection: a dm_only calendar is never even searched for a Player (it is
// filtered out of ListCalendars before EventRepository.SearchEvents is
// called for it), and the role a Player search runs as reaches the
// repository's own SQL dm_only filter unchanged.

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

func TestSearchCalendarEvents_DmOnlyCalendarNeverSearchedForPlayer(t *testing.T) {
	calRepo := &fakeCalendarRepo{
		listByCampaignFn: func(_ context.Context, campaignID string) ([]Calendar, error) {
			return []Calendar{
				{ID: "cal-open", CampaignID: campaignID, Visibility: "everyone"},
				{ID: "cal-dm", CampaignID: campaignID, Visibility: "dm_only"},
			}, nil
		},
	}
	var searched []string
	eventRepo := &fakeEventRepo{
		searchFn: func(_ context.Context, calendarID, _ string, _ int) ([]Event, error) {
			searched = append(searched, calendarID)
			return nil, nil
		},
	}
	svc := newTestCalendarService(calRepo, eventRepo, nil, nil)

	if _, err := svc.SearchCalendarEvents(context.Background(), testCampaignA, "feast", int(permissions.RolePlayer)); err != nil {
		t.Fatalf("SearchCalendarEvents: %v", err)
	}

	for _, id := range searched {
		if id == "cal-dm" {
			t.Fatalf("a dm_only calendar must never be searched for a Player, got calendars searched = %v", searched)
		}
	}
	if len(searched) != 1 || searched[0] != "cal-open" {
		t.Errorf("expected only the visible calendar to be searched, got %v", searched)
	}
}

func TestSearchCalendarEvents_OwnerSearchesDmOnlyCalendarToo(t *testing.T) {
	calRepo := &fakeCalendarRepo{
		listByCampaignFn: func(_ context.Context, campaignID string) ([]Calendar, error) {
			return []Calendar{
				{ID: "cal-open", CampaignID: campaignID, Visibility: "everyone"},
				{ID: "cal-dm", CampaignID: campaignID, Visibility: "dm_only"},
			}, nil
		},
	}
	var searched []string
	eventRepo := &fakeEventRepo{
		searchFn: func(_ context.Context, calendarID, _ string, _ int) ([]Event, error) {
			searched = append(searched, calendarID)
			return nil, nil
		},
	}
	svc := newTestCalendarService(calRepo, eventRepo, nil, nil)

	if _, err := svc.SearchCalendarEvents(context.Background(), testCampaignA, "feast", int(permissions.RoleOwner)); err != nil {
		t.Fatalf("SearchCalendarEvents: %v", err)
	}

	if len(searched) != 2 {
		t.Errorf("an Owner's search must reach every calendar including dm_only ones, got %v", searched)
	}
}

func TestSearchCalendarEvents_RolePassedThroughToRepository(t *testing.T) {
	calRepo := &fakeCalendarRepo{
		listByCampaignFn: func(_ context.Context, campaignID string) ([]Calendar, error) {
			return []Calendar{{ID: "cal-open", CampaignID: campaignID, Visibility: "everyone"}}, nil
		},
	}
	var gotRole int
	eventRepo := &fakeEventRepo{
		searchFn: func(_ context.Context, _, _ string, role int) ([]Event, error) {
			gotRole = role
			return nil, nil
		},
	}
	svc := newTestCalendarService(calRepo, eventRepo, nil, nil)

	if _, err := svc.SearchCalendarEvents(context.Background(), testCampaignA, "feast", int(permissions.RoleScribe)); err != nil {
		t.Fatalf("SearchCalendarEvents: %v", err)
	}
	if gotRole != int(permissions.RoleScribe) {
		t.Errorf("role passed to EventRepository.SearchEvents = %d, want %d — its own SQL dm_only filter depends on this", gotRole, int(permissions.RoleScribe))
	}
}

func TestSearchCalendarEvents_ResultShape(t *testing.T) {
	calRepo := &fakeCalendarRepo{
		listByCampaignFn: func(_ context.Context, campaignID string) ([]Calendar, error) {
			return []Calendar{{ID: "cal-open", CampaignID: campaignID, Visibility: "everyone"}}, nil
		},
	}
	eventRepo := &fakeEventRepo{
		searchFn: func(context.Context, string, string, int) ([]Event, error) {
			return []Event{{ID: "evt-1", Name: "Harvest Feast", KindIcon: "fa-wheat", KindColor: "#f59e0b"}}, nil
		},
	}
	svc := newTestCalendarService(calRepo, eventRepo, nil, nil)

	results, err := svc.SearchCalendarEvents(context.Background(), "camp-x", "feast", int(permissions.RolePlayer))
	if err != nil {
		t.Fatalf("SearchCalendarEvents: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	got := results[0]
	want := map[string]string{
		"id": "evt-1", "name": "Harvest Feast", "type_name": "Calendar Event",
		"type_icon": "fa-wheat", "type_color": "#f59e0b",
		"url": "/campaigns/camp-x/calendars/cal-open/view",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("result[%q] = %q, want %q (full result: %+v)", k, got[k], v, got)
		}
	}
}

func TestSearchCalendarEvents_NoKindIconFallsBack(t *testing.T) {
	calRepo := &fakeCalendarRepo{
		listByCampaignFn: func(_ context.Context, campaignID string) ([]Calendar, error) {
			return []Calendar{{ID: "cal-open", CampaignID: campaignID, Visibility: "everyone"}}, nil
		},
	}
	eventRepo := &fakeEventRepo{
		searchFn: func(context.Context, string, string, int) ([]Event, error) {
			return []Event{{ID: "evt-1", Name: "Unclassified Thing"}}, nil
		},
	}
	svc := newTestCalendarService(calRepo, eventRepo, nil, nil)

	results, err := svc.SearchCalendarEvents(context.Background(), testCampaignA, "thing", int(permissions.RolePlayer))
	if err != nil {
		t.Fatalf("SearchCalendarEvents: %v", err)
	}
	if got := results[0]["type_icon"]; got != "fa-calendar-day" {
		t.Errorf("type_icon fallback = %q, want fa-calendar-day", got)
	}
}

func TestSearchCalendarEvents_PerPersonRulesStayOutOfNonGMSearch(t *testing.T) {
	deny := `{"denied_users":["u-player"]}`
	allow := `{"allowed_users":["u-other"]}`
	calRepo := &fakeCalendarRepo{
		listByCampaignFn: func(_ context.Context, campaignID string) ([]Calendar, error) {
			return []Calendar{{ID: "cal-open", CampaignID: campaignID, Visibility: "everyone"}}, nil
		},
	}
	eventRepo := &fakeEventRepo{
		searchFn: func(_ context.Context, _, _ string, _ int) ([]Event, error) {
			return []Event{
				{ID: "ev-open", Name: "Feast", Visibility: "everyone"},
				{ID: "ev-denied", Name: "Feast of knives", Visibility: "everyone", VisibilityRules: &deny},
				{ID: "ev-allowlist", Name: "Feast for one", Visibility: "everyone", VisibilityRules: &allow},
			}, nil
		},
	}
	svc := newTestCalendarService(calRepo, eventRepo, nil, nil)

	tests := []struct {
		name string
		role int
		want []string
	}{
		{"player sees only the event without per-person rules", permissions.RolePlayer, []string{"ev-open"}},
		{"owner sees all", permissions.RoleOwner, []string{"ev-open", "ev-denied", "ev-allowlist"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.SearchCalendarEvents(context.Background(), testCampaignA, "feast", tt.role)
			if err != nil {
				t.Fatalf("SearchCalendarEvents: %v", err)
			}
			var ids []string
			for _, r := range got {
				ids = append(ids, r["id"])
			}
			if len(ids) != len(tt.want) {
				t.Fatalf("got %v, want %v", ids, tt.want)
			}
			for i := range ids {
				if ids[i] != tt.want[i] {
					t.Fatalf("got %v, want %v", ids, tt.want)
				}
			}
		})
	}
}

// A player's search must not find what the month grid hides from them: a
// future event announced only on its day, or one in a secret era.
func TestSearchCalendarEvents_HidesNotYetKnowableEventsFromPlayers(t *testing.T) {
	cal := Calendar{
		ID: "cal-1", CampaignID: testCampaignA, Mode: ModeFantasy, Visibility: "everyone",
		CurrentYear: 1000, CurrentMonth: 1, CurrentDay: 10,
		HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60,
	}
	ahead := AnnouncedAhead
	events := []Event{
		{ID: "evt-secret", CalendarID: "cal-1", Name: "Feast of Knives", Year: 1000, Month: 1, Day: 20, Visibility: "everyone", KindID: intPtr(1)},
		{ID: "evt-today", CalendarID: "cal-1", Name: "Feast Today", Year: 1000, Month: 1, Day: 10, Visibility: "everyone", KindID: intPtr(1)},
		{ID: "evt-ahead", CalendarID: "cal-1", Name: "Feast Announced", Year: 1000, Month: 1, Day: 25, Visibility: "everyone", Announced: &ahead},
	}
	calRepo := &fakeCalendarRepo{
		listByCampaignFn: func(context.Context, string) ([]Calendar, error) { return []Calendar{cal}, nil },
		getMonthsFn: func(context.Context, string) ([]Month, error) {
			return []Month{{Name: "First", Days: 30, SortOrder: 0}}, nil
		},
		getErasFn: func(context.Context, string) ([]Era, error) { return nil, nil },
	}
	eventRepo := &fakeEventRepo{
		searchFn: func(context.Context, string, string, int) ([]Event, error) {
			return append([]Event(nil), events...), nil
		},
	}
	kindRepo := &fakeEventKindRepo{
		listFn: func(context.Context, string) ([]EventKind, error) {
			return []EventKind{{ID: 1, Slug: "battle", Name: "Battle", DefaultAnnounced: AnnouncedOnDay}}, nil
		},
	}
	svc := newTestCalendarService(calRepo, eventRepo, kindRepo, nil)

	for _, tc := range []struct {
		role int
		want []string
	}{
		{int(permissions.RolePlayer), []string{"Feast Today", "Feast Announced"}},
		{int(permissions.RoleOwner), []string{"Feast of Knives", "Feast Today", "Feast Announced"}},
	} {
		results, err := svc.SearchCalendarEvents(context.Background(), testCampaignA, "feast", tc.role)
		if err != nil {
			t.Fatalf("role %d: SearchCalendarEvents: %v", tc.role, err)
		}
		var got []string
		for _, r := range results {
			got = append(got, r["name"])
		}
		if len(got) != len(tc.want) {
			t.Fatalf("role %d: got %v, want %v", tc.role, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("role %d: got %v, want %v", tc.role, got, tc.want)
				break
			}
		}
	}
}
