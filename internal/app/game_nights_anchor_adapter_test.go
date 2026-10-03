// game_nights_anchor_adapter_test.go: the sessions-to-calendar seam for the
// anchor-move warning. The integration test wires the real services over a
// real database exactly as routes.go does and proves the preview names real
// sessions; the unit test pins the adapter's mapping.
package app

import (
	"context"
	"io/fs"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
)

type worldRangeSessionsService struct {
	sessions.SessionService
	gotFrom, gotTo sessions.WorldDate
	gotLimit       int
	result         []sessions.Session
}

func (f *worldRangeSessionsService) ListPlannedSessionsInWorldDateRange(_ context.Context, _ string, from, to sessions.WorldDate, limit int) ([]sessions.Session, error) {
	f.gotFrom, f.gotTo, f.gotLimit = from, to, limit
	return f.result, nil
}

func TestGameNightsAnchorMoveAdapter_MapsSessionsAndForwardsRange(t *testing.T) {
	y, m, d := 1000, 4, 9
	svc := &worldRangeSessionsService{result: []sessions.Session{
		{Name: "Dated", CalendarYear: &y, CalendarMonth: &m, CalendarDay: &d},
		{Name: "Incomplete", CalendarYear: &y},
	}}
	a := &gameNightsAnchorMoveAdapter{svc: svc}

	got, err := a.SessionsInWorldDateRange(context.Background(), "camp", 1000, 1, 2, 1003, 12, 30, 3)
	if err != nil {
		t.Fatalf("SessionsInWorldDateRange: %v", err)
	}
	if svc.gotFrom != (sessions.WorldDate{Year: 1000, Month: 1, Day: 2}) || svc.gotTo != (sessions.WorldDate{Year: 1003, Month: 12, Day: 30}) || svc.gotLimit != 3 {
		t.Errorf("forwarded range %+v..%+v limit %d", svc.gotFrom, svc.gotTo, svc.gotLimit)
	}
	want := calendar.AffectedSession{Name: "Dated", OldWorldYear: 1000, OldWorldMonth: 4, OldWorldDay: 9}
	if len(got) != 1 || got[0] != want {
		t.Errorf("got %+v, want only %+v (a session without a full date is dropped)", got, want)
	}
}

// TestPreviewAnchorMove_ListsRealSessionsOnceWired is the end-to-end proof:
// real calendar service + real sessions service/repository + the adapter,
// wired the way routes.go wires them.
func TestPreviewAnchorMove_ListsRealSessionsOnceWired(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTimelineTestDB(t)
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()

	sub, err := fs.Sub(sessions.MigrationsFS, database.PluginMigrationsSubdir)
	if err != nil {
		t.Fatalf("sub-FS: %v", err)
	}
	for _, res := range database.RunPluginMigrations(db, []database.PluginSchema{{Slug: "sessions", MigrationsFS: sub}}) {
		if !res.Healthy {
			t.Fatalf("%s plugin migrations did not apply: %v", res.Slug, res.Error)
		}
	}

	campID, ownerID := newCalRoundTripCampaign(t, db, "anchor-preview")

	calSvc := calendar.NewCalendarService(calendar.NewCalendarRepository(db),
		calendar.NewEventRepository(db), calendar.NewEventKindRepository(db), calendar.NewWeatherRepository(db))
	cal, err := calSvc.CreateCalendar(ctx, campID, calendar.CreateCalendarInput{Name: "Anchored", CurrentYear: 1000, Visibility: "everyone"})
	if err != nil {
		t.Fatalf("CreateCalendar: %v", err)
	}
	months := make([]calendar.MonthInput, 12)
	for i := range months {
		months[i] = calendar.MonthInput{Name: "M", Days: 30, SortOrder: i}
	}
	if err := calSvc.SetMonths(ctx, cal.ID, campID, months); err != nil {
		t.Fatalf("SetMonths: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`UPDATE calendars SET current_year = 1000, current_month = 1, current_day = 1,
		 anchor_year = 1000, anchor_month = 1, anchor_day = 1, anchor_real_date = '2020-01-01' WHERE id = ?`, cal.ID); err != nil {
		t.Fatalf("set anchor: %v", err)
	}

	sessSvc := sessions.NewSessionService(sessions.NewSessionRepository(db), nil, nil)
	ymd := func(y, m, d int) (*int, *int, *int) { return &y, &m, &d }
	for _, s := range []struct {
		name    string
		y, m, d int
	}{
		{"Siege of Dawn", 1000, 3, 10},
		{"Harvest Feast", 1000, 9, 2},
		{"Far Future Night", 1050, 1, 1},
	} {
		y, m, d := ymd(s.y, s.m, s.d)
		if _, err := sessSvc.CreateSession(ctx, campID, sessions.CreateSessionInput{
			Name: s.name, CalendarYear: y, CalendarMonth: m, CalendarDay: d, CreatedBy: ownerID,
		}); err != nil {
			t.Fatalf("CreateSession %q: %v", s.name, err)
		}
	}

	newRealDate := time.Date(2020, 1, 8, 0, 0, 0, 0, time.UTC)
	preview := func() *calendar.AnchorMovePreview {
		t.Helper()
		got, err := calSvc.PreviewAnchorMove(ctx, cal.ID, campID, 1000, 1, 1, newRealDate)
		if err != nil {
			t.Fatalf("PreviewAnchorMove: %v", err)
		}
		return got
	}

	if got := preview(); got.DeltaDays != 7 || len(got.Affected) != 0 {
		t.Fatalf("unwired: delta=%d affected=%d, want delta 7 and none listed", got.DeltaDays, len(got.Affected))
	}

	calSvc.(interface {
		SetGameNightsAffectedByAnchorMove(calendar.GameNightsAffectedByAnchorMove)
	}).SetGameNightsAffectedByAnchorMove(&gameNightsAnchorMoveAdapter{svc: sessSvc})

	got := preview()
	if len(got.Affected) != 2 || got.Affected[0].Name != "Siege of Dawn" || got.Affected[1].Name != "Harvest Feast" {
		t.Fatalf("wired affected = %+v, want the two sessions inside the window, soonest first", got.Affected)
	}
	// Siege of Dawn is world day 69 after the anchor: 2020-01-01 + 69d = 2020-03-10.
	first := got.Affected[0]
	if first.OldYear != 2020 || first.OldMonth != 3 || first.OldDay != 10 || first.NewDay != 17 {
		t.Errorf("first affected dates = old %d-%d-%d new %d-%d-%d, want 2020-3-10 -> 2020-3-17",
			first.OldYear, first.OldMonth, first.OldDay, first.NewYear, first.NewMonth, first.NewDay)
	}
}
