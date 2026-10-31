// export_calendar_backup_integration_test.go proves a campaign backup
// carries a calendar's painted weather days, a hand-set moon and an event
// roll through a real export and import against MariaDB, the way
// export_calendar_roundtrip_integration_test.go does for the rest of the
// calendar. Skipped under -short; run with `make test-int-local`.
package app

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func TestCalendarCampaignExportImport_DaysMoonsRollsDBRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTimelineTestDB(t)
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	srcCampaignID, _ := newCalRoundTripCampaign(t, db, "bk-src")
	dstCampaignID, _ := newCalRoundTripCampaign(t, db, "bk-dst")

	calSvc := calendar.NewCalendarService(calendar.NewCalendarRepository(db), calendar.NewEventRepository(db),
		calendar.NewEventKindRepository(db), calendar.NewWeatherRepository(db))
	wired := calSvc.(interface {
		SetEventRollRepository(calendar.EventRollRepository)
		SetMoonMarkRepository(calendar.MoonMarkRepository)
	})
	wired.SetEventRollRepository(calendar.NewEventRollRepository(db))
	wired.SetMoonMarkRepository(calendar.NewMoonMarkRepository(db))
	sys := permissions.SystemViewer(3)
	must := func(err error, what string) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
	}

	// --- Source: one month, a hand-set moon, painted days, a rolling event ---
	src, err := calSvc.CreateCalendar(ctx, srcCampaignID, calendar.CreateCalendarInput{Name: "Reckoning", CurrentYear: 5})
	must(err, "create calendar")
	must(calSvc.SetDefaultCalendar(ctx, srcCampaignID, src.ID), "set default")
	must(calSvc.SetMonths(ctx, src.ID, srcCampaignID, []calendar.MonthInput{{Name: "One", Days: 30}}), "set months")
	must(calSvc.SetMoons(ctx, src.ID, srcCampaignID, []calendar.MoonInput{
		{Name: "Umbra", CycleDays: 30, Color: "#ffffff"}, {Name: "Selune", CycleDays: 28, Color: "#dddddd"},
	}), "set moons")
	srcFull, err := calSvc.GetCalendarForViewer(ctx, src.ID, srcCampaignID, sys)
	must(err, "load calendar")
	marks := calSvc.(calendar.MoonMarkService)
	var umbra int
	for _, m := range srcFull.Moons {
		if m.Name == "Umbra" {
			umbra = m.ID
		}
	}
	_, err = marks.SetMoonMode(ctx, src.ID, srcCampaignID, umbra, calendar.MoonManual, map[string]string{calendar.MoonFull: "Blazing"}, sys)
	must(err, "set moon by hand")
	_, err = marks.SetMoonMark(ctx, src.ID, srcCampaignID, umbra, calendar.MoonMark{Year: 5, Month: 1, Day: 4, State: calendar.MoonFull, Shown: true}, sys)
	must(err, "set moon mark")

	icon := "cloud-rain"
	must(calSvc.SetDayWeather(ctx, src.ID, srcCampaignID, []calendar.DayWeatherInput{
		{Year: 5, Month: 1, Day: 3, WeatherInput: calendar.WeatherInput{Icon: &icon}},
		{Year: 5, Month: 1, Day: 4, Source: calendar.WeatherSourceGenerated},
	}), "paint days")
	_, err = calSvc.LockDayWeather(ctx, src.ID, srcCampaignID, []calendar.DayDate{{Year: 5, Month: 1, Day: 3}}, true)
	must(err, "lock day")

	evt, err := calSvc.CreateEvent(ctx, src.ID, srcCampaignID, calendar.CreateEventInput{
		Name: "Bloodnight", Year: 5, Month: 1, Day: 3, Visibility: "everyone", CanAuthorDmOnly: true,
	})
	must(err, "create event")
	rolls := calSvc.(calendar.EventRollService)
	_, err = rolls.SetEventRoll(ctx, evt.ID, src.ID, srcCampaignID, &calendar.RollConfig{
		Source: calendar.RollSourceDice, Dice: "2d10", Results: []calendar.RollBand{{Name: "Calm"}},
	}, 5, sys)
	must(err, "set roll")
	night := calendar.DayDate{Year: 5, Month: 1, Day: 3}
	_, err = rolls.RollEventNights(ctx, evt.ID, src.ID, srcCampaignID, calendar.RollNightsInput{Dates: []calendar.DayDate{night}}, sys)
	must(err, "roll night")
	_, err = rolls.LockEventRollNights(ctx, evt.ID, src.ID, srcCampaignID, []calendar.DayDate{night}, true, sys)
	must(err, "lock night")
	told := true
	must(rolls.TellEventRollNight(ctx, evt.ID, src.ID, srcCampaignID, night, &told, sys), "tell night")
	srcView, err := rolls.GetEventRoll(ctx, evt.ID, src.ID, srcCampaignID, 5, sys)
	must(err, "read source roll")

	// --- Export, then import into the second campaign ---
	data, err := (&calendarExportAdapter{svc: calSvc}).ExportCalendar(ctx, srcCampaignID, func(string) string { return "" })
	must(err, "export")
	report := campaigns.NewImportReport()
	must((&calendarImportAdapter{svc: calSvc}).ImportCalendar(ctx, dstCampaignID, data, campaigns.NewIDMap(dstCampaignID), report), "import")
	if report.HasFailures() {
		t.Fatalf("import reported failures: %+v", report.Failures())
	}

	dst, err := calSvc.GetDefaultCalendarForViewer(ctx, dstCampaignID, sys)
	must(err, "load restored calendar")
	days, err := calSvc.ListDayWeather(ctx, dst.ID, dstCampaignID, 5, 0, sys)
	must(err, "list restored days")
	if len(days) != 2 || days[0].Source != calendar.WeatherSourceManual || days[0].Icon == nil || *days[0].Icon != icon ||
		days[0].Locked == nil || !*days[0].Locked || days[1].Source != calendar.WeatherSourceGenerated || *days[1].Locked {
		t.Errorf("restored day weather = %+v", days)
	}

	for _, m := range dst.Moons {
		switch m.Name {
		case "Umbra":
			if m.PhaseMode != calendar.MoonManual || m.StateNames[calendar.MoonFull] != "Blazing" ||
				len(m.Marks) != 1 || m.Marks[0].State != calendar.MoonFull || !m.Marks[0].Shown {
				t.Errorf("restored Umbra = %+v", m)
			}
		case "Selune":
			if m.PhaseMode != calendar.MoonCycle {
				t.Errorf("Selune should still follow its cycle: %+v", m)
			}
		}
	}

	dstEvents, err := calSvc.ListAllEventsForCalendar(ctx, dst.ID, dstCampaignID, sys)
	must(err, "list restored events")
	if len(dstEvents) != 1 {
		t.Fatalf("restored %d events, want 1", len(dstEvents))
	}
	got, err := rolls.GetEventRoll(ctx, dstEvents[0].ID, dst.ID, dstCampaignID, 5, sys)
	must(err, "read restored roll")
	if got.Config == nil || got.Config.Dice != "2d10" || len(got.Nights) != 1 {
		t.Fatalf("restored roll = %+v", got)
	}
	want, n := srcView.Nights[0], got.Nights[0]
	if n.Total == nil || *n.Total != *want.Total || n.Locked == nil || !*n.Locked || n.Told == nil || !*n.Told {
		t.Errorf("restored night = %+v, want total %d, locked and told", n, *want.Total)
	}
}
