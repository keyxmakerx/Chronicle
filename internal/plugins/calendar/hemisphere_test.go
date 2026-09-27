// hemisphere_test.go: table-driven tests for the hemisphere-driven season
// auto-seeding UpdateCalendar performs on a real-life calendar's first (or
// changed) hemisphere choice — see defaultRealLifeSeasons and
// seedHemisphereSeasonsIfEmpty in service.go.
package calendar

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

// realLifeCalRepo builds a fakeCalendarRepo over a stored real-life calendar,
// recording whatever SetSeasons is called with (or not called at all).
func realLifeCalRepo(stored Calendar, existingSeasons []Season) (*fakeCalendarRepo, *[]Season, *bool) {
	var saved []Season
	setCalled := false
	repo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			c := stored
			return &c, nil
		},
		getSeasonsFn: func(_ context.Context, _ string) ([]Season, error) {
			return existingSeasons, nil
		},
		setSeasonsFn: func(_ context.Context, _ string, seasons []Season) error {
			setCalled = true
			saved = seasons
			return nil
		},
	}
	return repo, &saved, &setCalled
}

func TestUpdateCalendar_HemisphereSeedsDefaultSeasons(t *testing.T) {
	base := Calendar{ID: "cal-1", CampaignID: testCampaignA, Name: "Earth", Mode: ModeRealLife,
		HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60}

	t.Run("first hemisphere choice on an empty real-life calendar seeds the four northern seasons", func(t *testing.T) {
		repo, saved, _ := realLifeCalRepo(base, nil)
		svc := newTestCalendarService(repo, nil, nil, nil)
		if err := svc.UpdateCalendar(context.Background(), "cal-1", testCampaignA, UpdateCalendarInput{
			Name: "Earth", Hemisphere: patch.Of(HemisphereNorth),
		}); err != nil {
			t.Fatalf("update: %v", err)
		}
		if len(*saved) != 4 {
			t.Fatalf("expected 4 seeded seasons, got %d: %+v", len(*saved), *saved)
		}
		want := []struct {
			name           string
			sm, sd, em, ed int
		}{
			{"Spring", 3, 20, 6, 20},
			{"Summer", 6, 21, 9, 22},
			{"Autumn", 9, 23, 12, 20},
			{"Winter", 12, 21, 3, 19},
		}
		for i, w := range want {
			s := (*saved)[i]
			if s.Name != w.name || s.StartMonth != w.sm || s.StartDay != w.sd || s.EndMonth != w.em || s.EndDay != w.ed {
				t.Errorf("season %d = %+v, want name=%s %d/%d-%d/%d", i, s, w.name, w.sm, w.sd, w.em, w.ed)
			}
		}
	})

	t.Run("southern hemisphere keeps the same dates but flips the season names", func(t *testing.T) {
		repo, saved, _ := realLifeCalRepo(base, nil)
		svc := newTestCalendarService(repo, nil, nil, nil)
		if err := svc.UpdateCalendar(context.Background(), "cal-1", testCampaignA, UpdateCalendarInput{
			Name: "Earth", Hemisphere: patch.Of(HemisphereSouth),
		}); err != nil {
			t.Fatalf("update: %v", err)
		}
		if len(*saved) != 4 {
			t.Fatalf("expected 4 seeded seasons, got %d", len(*saved))
		}
		// Same four reference dates as the northern set (pinned exactly by
		// the test above); only the names differ, on the identical dates.
		northern := defaultRealLifeSeasons(HemisphereNorth)
		wantNames := []string{"Autumn", "Winter", "Spring", "Summer"}
		for i, name := range wantNames {
			s := (*saved)[i]
			if s.Name != name {
				t.Errorf("season %d name = %q, want %q", i, s.Name, name)
			}
			n := northern[i]
			if s.StartMonth != n.StartMonth || s.StartDay != n.StartDay || s.EndMonth != n.EndMonth || s.EndDay != n.EndDay {
				t.Errorf("season %d dates = %d/%d-%d/%d, want the same dates as the northern set (%d/%d-%d/%d)",
					i, s.StartMonth, s.StartDay, s.EndMonth, s.EndDay, n.StartMonth, n.StartDay, n.EndMonth, n.EndDay)
			}
		}
	})

	t.Run("a calendar that already has a season is never touched by a hemisphere change", func(t *testing.T) {
		repo, _, setCalled := realLifeCalRepo(base, []Season{{Name: "Custom Season"}})
		svc := newTestCalendarService(repo, nil, nil, nil)
		if err := svc.UpdateCalendar(context.Background(), "cal-1", testCampaignA, UpdateCalendarInput{
			Name: "Earth", Hemisphere: patch.Of(HemisphereNorth),
		}); err != nil {
			t.Fatalf("update: %v", err)
		}
		if *setCalled {
			t.Errorf("SetSeasons must not be called when the calendar already has a season")
		}
	})

	t.Run("a fantasy-mode calendar is never auto-seeded regardless of hemisphere", func(t *testing.T) {
		fantasy := base
		fantasy.Mode = ModeFantasy
		repo, _, setCalled := realLifeCalRepo(fantasy, nil)
		svc := newTestCalendarService(repo, nil, nil, nil)
		if err := svc.UpdateCalendar(context.Background(), "cal-1", testCampaignA, UpdateCalendarInput{
			Name: "Faerun", Hemisphere: patch.Of(HemisphereNorth),
		}); err != nil {
			t.Fatalf("update: %v", err)
		}
		if *setCalled {
			t.Errorf("a fantasy-mode calendar must never be auto-seeded")
		}
	})

	t.Run("resending the hemisphere already stored is a no-op, not a reseed", func(t *testing.T) {
		north := HemisphereNorth
		already := base
		already.Hemisphere = &north
		repo, _, setCalled := realLifeCalRepo(already, nil)
		svc := newTestCalendarService(repo, nil, nil, nil)
		if err := svc.UpdateCalendar(context.Background(), "cal-1", testCampaignA, UpdateCalendarInput{
			Name: "Earth", Hemisphere: patch.Of(HemisphereNorth),
		}); err != nil {
			t.Fatalf("update: %v", err)
		}
		if *setCalled {
			t.Errorf("resending the stored hemisphere must not reseed")
		}
	})

	t.Run("an absent Hemisphere field never seeds anything", func(t *testing.T) {
		repo, _, setCalled := realLifeCalRepo(base, nil)
		svc := newTestCalendarService(repo, nil, nil, nil)
		if err := svc.UpdateCalendar(context.Background(), "cal-1", testCampaignA, UpdateCalendarInput{
			Name: "Earth",
		}); err != nil {
			t.Fatalf("update: %v", err)
		}
		if *setCalled {
			t.Errorf("an update that never mentions hemisphere must not seed seasons")
		}
	})

	t.Run("an unsupported hemisphere value is rejected", func(t *testing.T) {
		repo, _, _ := realLifeCalRepo(base, nil)
		svc := newTestCalendarService(repo, nil, nil, nil)
		err := svc.UpdateCalendar(context.Background(), "cal-1", testCampaignA, UpdateCalendarInput{
			Name: "Earth", Hemisphere: patch.Of("east"),
		})
		wantValidationErr(t, err, "bad hemisphere")
	})
}

// TestDefaultRealLifeSeasonsContainsDate confirms Season.ContainsDate's
// existing wrap-around logic already handles the Winter season spanning the
// Dec 21 -> Mar 19 year boundary, for both hemispheres' season sets.
func TestDefaultRealLifeSeasonsContainsDate(t *testing.T) {
	for _, hemi := range []string{HemisphereNorth, HemisphereSouth} {
		seasons := defaultRealLifeSeasons(hemi)
		if len(seasons) != 4 {
			t.Fatalf("%s: expected 4 seasons, got %d", hemi, len(seasons))
		}
		winter := seasons[3]
		if !winter.ContainsDate(12, 25) {
			t.Errorf("%s: Dec 25 should fall in %s (wrap-around start)", hemi, winter.Name)
		}
		if !winter.ContainsDate(1, 15) {
			t.Errorf("%s: Jan 15 should fall in %s (wrap-around continuation)", hemi, winter.Name)
		}
		if winter.ContainsDate(3, 20) {
			t.Errorf("%s: Mar 20 (the next season's start) should NOT fall in %s", hemi, winter.Name)
		}
		if winter.ContainsDate(12, 20) {
			t.Errorf("%s: Dec 20 (still the prior season) should NOT fall in %s", hemi, winter.Name)
		}
	}
}
