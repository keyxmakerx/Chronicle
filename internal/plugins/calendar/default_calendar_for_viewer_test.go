// default_calendar_for_viewer_test.go pins GetDefaultCalendarForViewer's
// three outcomes: no default calendar, a default calendar the viewer may not
// see, and a visible default calendar loaded and gated exactly like
// GetCalendarForViewer (shared via finishCalendarForViewer). This is the
// entry point the "skybox" dashboard/template block uses (see
// internal/app/routes.go), which knows a campaign id but not a calendar id.
package calendar

import (
	"context"
	"testing"
)

func TestGetDefaultCalendarForViewer(t *testing.T) {
	visibleMoon := Moon{ID: 1, CalendarID: "cal-default", Name: "Luna"}
	hiddenMoon := Moon{ID: 2, CalendarID: "cal-default", Name: "Secret Moon", HiddenFromPlayers: true}

	t.Run("no default calendar returns NotFound", func(t *testing.T) {
		calRepo := &fakeCalendarRepo{
			getDefaultFn: func(_ context.Context, _ string) (*Calendar, error) { return nil, nil },
		}
		svc := newTestCalendarService(calRepo, nil, nil, nil)

		_, err := svc.GetDefaultCalendarForViewer(context.Background(), testCampaignA, playerViewer("u-1"))
		assertNotFound(t, err)
	})

	t.Run("default calendar in another campaign returns NotFound", func(t *testing.T) {
		calRepo := &fakeCalendarRepo{
			getDefaultFn: func(_ context.Context, _ string) (*Calendar, error) {
				return &Calendar{ID: "cal-default", CampaignID: "camp-other", Visibility: "everyone"}, nil
			},
		}
		svc := newTestCalendarService(calRepo, nil, nil, nil)

		_, err := svc.GetDefaultCalendarForViewer(context.Background(), testCampaignA, playerViewer("u-1"))
		assertNotFound(t, err)
	})

	t.Run("dm_only default calendar hidden from a player", func(t *testing.T) {
		calRepo := &fakeCalendarRepo{
			getDefaultFn: func(_ context.Context, _ string) (*Calendar, error) {
				return &Calendar{ID: "cal-default", CampaignID: testCampaignA, Visibility: "dm_only"}, nil
			},
		}
		svc := newTestCalendarService(calRepo, nil, nil, nil)

		_, err := svc.GetDefaultCalendarForViewer(context.Background(), testCampaignA, playerViewer("u-1"))
		assertNotFound(t, err)

		cal, err := svc.GetDefaultCalendarForViewer(context.Background(), testCampaignA, ownerViewer("u-owner"))
		if err != nil {
			t.Fatalf("owner must see a dm_only default calendar: %v", err)
		}
		if cal.ID != "cal-default" {
			t.Errorf("got calendar %q, want cal-default", cal.ID)
		}
	})

	t.Run("visible default calendar is loaded and gated like GetCalendarForViewer", func(t *testing.T) {
		calRepo := &fakeCalendarRepo{
			getDefaultFn: func(_ context.Context, _ string) (*Calendar, error) {
				return &Calendar{ID: "cal-default", CampaignID: testCampaignA, Visibility: "everyone"}, nil
			},
			getMoonsFn: func(_ context.Context, _ string) ([]Moon, error) {
				return []Moon{visibleMoon, hiddenMoon}, nil
			},
			getErasFn: func(_ context.Context, _ string) ([]Era, error) {
				return []Era{{ID: 1, Name: "Known Era", StartYear: 1, StartMonth: 1, StartDay: 1},
					{ID: 2, Name: "Secret Era", StartYear: 5, StartMonth: 1, StartDay: 1, HiddenUntilBegins: true}}, nil
			},
		}
		kindRepo := &fakeEventKindRepo{
			listFn: func(_ context.Context, _ string) ([]EventKind, error) {
				return []EventKind{{ID: 1, Name: "Holiday"}}, nil
			},
		}
		svc := newTestCalendarService(calRepo, nil, kindRepo, nil)

		playerCal, err := svc.GetDefaultCalendarForViewer(context.Background(), testCampaignA, playerViewer("u-1"))
		if err != nil {
			t.Fatalf("GetDefaultCalendarForViewer (player): %v", err)
		}
		if len(playerCal.Moons) != 1 || playerCal.Moons[0].ID != visibleMoon.ID {
			t.Errorf("player must see only the non-hidden moon, got %+v", playerCal.Moons)
		}
		if len(playerCal.Eras) != 1 || playerCal.Eras[0].Name != "Known Era" || playerCal.EventKinds != nil {
			t.Errorf("player must see only the unhidden era and no event kinds, got eras=%+v kinds=%+v", playerCal.Eras, playerCal.EventKinds)
		}

		ownerCal, err := svc.GetDefaultCalendarForViewer(context.Background(), testCampaignA, ownerViewer("u-owner"))
		if err != nil {
			t.Fatalf("GetDefaultCalendarForViewer (owner): %v", err)
		}
		if len(ownerCal.Moons) != 2 || len(ownerCal.Eras) != 2 || len(ownerCal.EventKinds) != 1 {
			t.Errorf("owner must see everything, got moons=%+v eras=%+v kinds=%+v", ownerCal.Moons, ownerCal.Eras, ownerCal.EventKinds)
		}
	})
}
