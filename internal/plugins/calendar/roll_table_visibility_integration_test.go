// roll_table_visibility_integration_test.go checks, against a real MariaDB,
// that a table the Director marked Only you never puts a night's result in
// front of a player, and that a night draws only from the lines whose
// conditions hold. Skipped under `-short`.
package calendar

import (
	"context"
	"testing"
)

func TestEventRollTable_Integration_OnlyYouAndWhen(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	fix := newTestCampaign(t, db, "roll-table-vis")
	ctx := context.Background()
	calRepo, eventRepo := NewCalendarRepository(db), NewEventRepository(db)

	// Two 28-day months with today in year 5, month 2, day 10, so the nights
	// on the 3rd of both months are already past.
	cal := &Calendar{ID: testUUID(t), CampaignID: fix.CampaignID, Mode: ModeFantasy, Name: "Rolls",
		HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60, Visibility: "everyone",
		CurrentYear: 5, CurrentMonth: 2, CurrentDay: 10}
	if err := calRepo.Create(ctx, cal); err != nil {
		t.Fatalf("seed calendar: %v", err)
	}
	if err := calRepo.SetMonths(ctx, cal.ID, []MonthInput{{Name: "Alpha", Days: 28}, {Name: "Beta", Days: 28, SortOrder: 1}}); err != nil {
		t.Fatalf("seed months: %v", err)
	}
	if err := calRepo.SetWeekdays(ctx, cal.ID, []WeekdayInput{{Name: "A"}, {Name: "B", SortOrder: 1}, {Name: "C", SortOrder: 2}}); err != nil {
		t.Fatalf("seed weekdays: %v", err)
	}
	monthly := RecurrenceMonthly
	evt := &Event{ID: testUUID(t), CalendarID: cal.ID, Name: "Omen night", Year: 1, Month: 1, Day: 3,
		Visibility: "everyone", IsRecurring: true, RecurrenceType: &monthly, CreatedBy: &fix.UserID}
	if err := eventRepo.CreateEvent(ctx, evt); err != nil {
		t.Fatalf("create event: %v", err)
	}

	svc := NewCalendarService(calRepo, eventRepo, NewEventKindRepository(db), NewWeatherRepository(db)).(*calendarService)
	svc.SetEventRollRepository(NewEventRollRepository(db))
	table := func(vis string) *RollTable {
		return &RollTable{Name: "Omens", Visibility: vis, Entries: []RollTableEntry{
			{Name: "Crows", Brief: "Alpha only.", Weight: 1, When: raw(`{"month":["Alpha"]}`)},
			{Name: "Frost", Brief: "Beta only.", Weight: 1, When: raw(`{"month":[2]}`)},
		}}
	}
	cfg := &RollConfig{Source: RollSourceTable, TableID: "omens", TellPlayers: true}

	tests := []struct {
		name       string
		visibility string
		wantSeen   int
	}{
		{"only you", RollTableDMOnly, 0},
		{"everyone", "everyone", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc.SetRollTableSource(fakeTables{"omens": table(tt.visibility)})
			if _, err := svc.SetEventRoll(ctx, evt.ID, cal.ID, fix.CampaignID, cfg, 5, owner); err != nil {
				t.Fatalf("set roll: %v", err)
			}
			if _, err := svc.RollEventNights(ctx, evt.ID, cal.ID, fix.CampaignID, RollNightsInput{Year: 5}, owner); err != nil {
				t.Fatalf("roll the year: %v", err)
			}
			dir, err := svc.GetEventRoll(ctx, evt.ID, cal.ID, fix.CampaignID, 5, owner)
			if err != nil || len(dir.Nights) != 2 || dir.Nights[0].Result != "Crows" || dir.Nights[1].Result != "Frost" {
				t.Fatalf("Director sees %+v (err %v), want Crows then Frost from the lines that fit", dir, err)
			}

			got, err := svc.GetEventRoll(ctx, evt.ID, cal.ID, fix.CampaignID, 5, player)
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Nights) != tt.wantSeen {
				t.Fatalf("player year read saw %d nights, want %d: %+v", len(got.Nights), tt.wantSeen, got.Nights)
			}
			month, err := svc.ListMonthRolls(ctx, cal.ID, fix.CampaignID, 5, 1, player)
			if err != nil {
				t.Fatal(err)
			}
			if saw := len(month[evt.ID]); saw != tt.wantSeen/2 {
				t.Fatalf("player month read saw %d nights, want %d", saw, tt.wantSeen/2)
			}
			for _, n := range append(got.Nights, month[evt.ID]...) {
				if n.Director != "" || n.Total != nil || n.Locked != nil {
					t.Errorf("a player's night carries the Director's side: %+v", n)
				}
			}
		})
	}

	// The Director telling one night of an Only you table is the one way a
	// player learns it.
	svc.SetRollTableSource(fakeTables{"omens": table(RollTableDMOnly)})
	if _, err := svc.RollEventNights(ctx, evt.ID, cal.ID, fix.CampaignID, RollNightsInput{Year: 5}, owner); err != nil {
		t.Fatal(err)
	}
	yes := true
	if err := svc.TellEventRollNight(ctx, evt.ID, cal.ID, fix.CampaignID, DayDate{Year: 5, Month: 2, Day: 3}, &yes, owner); err != nil {
		t.Fatalf("tell the night: %v", err)
	}
	got, err := svc.GetEventRoll(ctx, evt.ID, cal.ID, fix.CampaignID, 5, player)
	if err != nil || len(got.Nights) != 1 || got.Nights[0].Result != "Frost" {
		t.Fatalf("player after the Director told one night = %+v (err %v), want just Frost", got.Nights, err)
	}
}
