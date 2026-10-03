// events_by_ids_test.go: table-driven tests for the batch viewer-filtered
// event read and the name-only calendar read. The batch read must agree,
// event for event, with GetEventForViewer, so each case also checks parity
// against the single read.
package calendar

import (
	"context"
	"reflect"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

func ptrStr(s string) *string { return &s }

func TestListEventsByIDsForViewer(t *testing.T) {
	events := map[string]Event{
		"open":   {ID: "open", CalendarID: "cal-1", Visibility: "everyone"},
		"secret": {ID: "secret", CalendarID: "cal-1", Visibility: "dm_only"},
		"denied": {ID: "denied", CalendarID: "cal-1", Visibility: "everyone",
			VisibilityRules: ptrStr(`{"denied_users":["u-1"]}`)},
		"other-cal": {ID: "other-cal", CalendarID: "cal-2", Visibility: "everyone"},
	}

	tests := []struct {
		name       string
		calendar   Calendar
		ids        []string
		viewer     permissions.Viewer
		want       []string
		wantNotFnd bool
	}{
		{"player sees only everyone events, in request order", Calendar{Visibility: "everyone"},
			[]string{"open", "secret", "denied"}, playerViewer("u-1"), []string{"open"}, false},
		{"owner sees dm_only and rule-denied events", Calendar{Visibility: "everyone"},
			[]string{"open", "secret", "denied"}, ownerViewer("u-owner"), []string{"open", "secret", "denied"}, false},
		{"co-DM matches owner", Calendar{Visibility: "everyone"},
			[]string{"secret"}, coDMViewer("u-codm"), []string{"secret"}, false},
		{"another player is not blocked by u-1's deny rule", Calendar{Visibility: "everyone"},
			[]string{"denied"}, playerViewer("u-2"), []string{"denied"}, false},
		{"request order and duplicates collapse", Calendar{Visibility: "everyone"},
			[]string{"denied", "open", "denied"}, ownerViewer("u-owner"), []string{"denied", "open"}, false},
		{"unknown ids are absent", Calendar{Visibility: "everyone"},
			[]string{"nope", "open"}, playerViewer("u-1"), []string{"open"}, false},
		{"id on another calendar is absent", Calendar{Visibility: "everyone"},
			[]string{"other-cal", "open"}, ownerViewer("u-owner"), []string{"open"}, false},
		{"empty slice", Calendar{Visibility: "everyone"},
			[]string{}, playerViewer("u-1"), nil, false},
		{"nil slice", Calendar{Visibility: "everyone"},
			nil, ownerViewer("u-owner"), nil, false},
		{"hidden calendar is NotFound for a player", Calendar{Visibility: "dm_only"},
			[]string{"open"}, playerViewer("u-1"), nil, true},
		{"hidden calendar is readable by the owner", Calendar{Visibility: "dm_only"},
			[]string{"open"}, ownerViewer("u-owner"), []string{"open"}, false},
		{"hidden calendar is NotFound for the public even with empty ids", Calendar{Visibility: "dm_only"},
			nil, publicViewer(), nil, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cal := tc.calendar
			cal.ID, cal.CampaignID = "cal-1", testCampaignA
			calRepo := &fakeCalendarRepo{
				getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
					c := cal
					c.ID = id
					return &c, nil
				},
			}
			batchCalls := 0
			eventRepo := &fakeEventRepo{
				getEventFn: func(_ context.Context, id string) (*Event, error) {
					if e, ok := events[id]; ok {
						return &e, nil
					}
					return nil, nil
				},
				getEventsByIDsFn: func(_ context.Context, calendarID string, ids []string) ([]Event, error) {
					batchCalls++
					var out []Event
					for _, id := range ids {
						if e, ok := events[id]; ok && e.CalendarID == calendarID {
							out = append(out, e)
						}
					}
					return out, nil
				},
			}
			svc := newTestCalendarService(calRepo, eventRepo, nil, nil)

			got, err := svc.ListEventsByIDsForViewer(context.Background(), "cal-1", testCampaignA, tc.ids, tc.viewer)
			if tc.wantNotFnd {
				if err == nil {
					t.Fatalf("expected NotFound, got %v", got)
				}
				assertNotFound(t, err)
				return
			}
			if err != nil {
				t.Fatalf("ListEventsByIDsForViewer: %v", err)
			}
			var gotIDs []string
			for _, e := range got {
				gotIDs = append(gotIDs, e.ID)
			}
			if !reflect.DeepEqual(gotIDs, tc.want) {
				t.Errorf("ids = %v, want %v", gotIDs, tc.want)
			}
			if len(tc.ids) > 0 && batchCalls != 1 {
				t.Errorf("repo batch reads = %d, want exactly 1", batchCalls)
			}

			// Parity with the single read, for every id asked.
			visible := map[string]bool{}
			for _, id := range gotIDs {
				visible[id] = true
			}
			for _, id := range tc.ids {
				_, serr := svc.GetEventForViewer(context.Background(), id, "cal-1", testCampaignA, tc.viewer)
				if (serr == nil) != visible[id] {
					t.Errorf("id %q: single read ok=%v but batch visible=%v", id, serr == nil, visible[id])
				}
			}
		})
	}
}

func TestGetCalendarNameForViewer(t *testing.T) {
	tests := []struct {
		name       string
		visibility string
		viewer     permissions.Viewer
		campaign   string
		want       string
		notFound   bool
	}{
		{"everyone calendar to player", "everyone", playerViewer("u-1"), testCampaignA, "Harptos", false},
		{"dm_only calendar hidden from player", "dm_only", playerViewer("u-1"), testCampaignA, "", true},
		{"dm_only calendar visible to owner", "dm_only", ownerViewer("u-o"), testCampaignA, "Harptos", false},
		{"calendar in another campaign is NotFound", "everyone", ownerViewer("u-o"), "other-campaign", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calRepo := &fakeCalendarRepo{
				getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
					return &Calendar{ID: id, CampaignID: testCampaignA, Name: "Harptos", Visibility: tc.visibility}, nil
				},
				// A sub-resource load would hit these; the name read must not.
				getMonthsFn: func(context.Context, string) ([]Month, error) {
					t.Error("name-only read must not load sub-resources")
					return nil, nil
				},
			}
			svc := newTestCalendarService(calRepo, nil, nil, nil)
			got, err := svc.GetCalendarNameForViewer(context.Background(), "cal-1", tc.campaign, tc.viewer)
			if tc.notFound {
				if err == nil {
					t.Fatalf("expected NotFound, got %q", got)
				}
				assertNotFound(t, err)
				return
			}
			if err != nil || got != tc.want {
				t.Errorf("got (%q, %v), want %q", got, err, tc.want)
			}
		})
	}
}
