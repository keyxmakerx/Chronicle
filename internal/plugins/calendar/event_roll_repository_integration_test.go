// event_roll_repository_integration_test.go exercises EventRollRepository
// against a real MariaDB: the date-range read, the lock a year roll must
// not cross, first-write-wins for nights rolled on the night, and the
// cascade from a deleted event. Skipped under `-short`.
package calendar

import (
	"context"
	"testing"
)

func TestEventRollRepository_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	t.Cleanup(func() { db.Close() })

	ctx := context.Background()
	calRepo, eventRepo, rolls := NewCalendarRepository(db), NewEventRepository(db), NewEventRollRepository(db)
	fix := newTestCampaign(t, db, "rolls")
	cal := newTestCalendar(testUUID(t), fix.CampaignID, "Rolls")
	if err := calRepo.Create(ctx, cal); err != nil {
		t.Fatalf("Create calendar: %v", err)
	}
	evt := &Event{ID: testUUID(t), CalendarID: cal.ID, Name: "Bloodnight", Year: 5, Month: 1, Day: 3, Visibility: "everyone", AllDay: true}
	if err := eventRepo.CreateEvent(ctx, evt); err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}

	cfg := RollConfig{Source: RollSourceDice, Dice: "2d10", Results: []RollBand{{Name: "Any"}}}
	if err := rolls.SetConfig(ctx, evt.ID, cfg); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	got, err := rolls.GetConfig(ctx, evt.ID)
	if err != nil || got == nil || got.Dice != "2d10" {
		t.Fatalf("GetConfig = %+v, %v", got, err)
	}

	total := func(n int) *int { return &n }
	d1, d2, d3 := DayDate{Year: 5, Month: 1, Day: 3}, DayDate{Year: 5, Month: 2, Day: 3}, DayDate{Year: 6, Month: 1, Day: 3}
	if err := rolls.PutNights(ctx, evt.ID, []RollNight{
		{Year: 5, Month: 1, Day: 3, Total: total(4), Faces: []int{1, 3}, Source: RollNightAhead},
		{Year: 5, Month: 2, Day: 3, Total: total(9), Source: RollNightAhead},
		{Year: 6, Month: 1, Day: 3, Total: total(15), Source: RollNightAhead},
	}, false); err != nil {
		t.Fatalf("PutNights: %v", err)
	}

	year5, err := rolls.ListNights(ctx, evt.ID, DayDate{Year: 5, Month: 1, Day: 1}, DayDate{Year: 5, Month: 12, Day: 30})
	if err != nil || len(year5) != 2 || year5[0].date() != d1 || year5[1].date() != d2 {
		t.Fatalf("ListNights year 5 = %+v, %v", year5, err)
	}
	if len(year5[0].Faces) != 2 || year5[0].Faces[1] != 3 {
		t.Errorf("faces did not round-trip: %+v", year5[0].Faces)
	}

	// A locked night keeps its result through a reroll; an unlocked one
	// takes the new result and goes back to its result's own telling.
	if n, err := rolls.LockNights(ctx, evt.ID, []DayDate{d1}, true); err != nil || n != 1 {
		t.Fatalf("LockNights = %d, %v", n, err)
	}
	yes := true
	if _, err := rolls.SetTold(ctx, evt.ID, d2, &yes); err != nil {
		t.Fatalf("SetTold: %v", err)
	}
	if err := rolls.PutNights(ctx, evt.ID, []RollNight{
		{Year: 5, Month: 1, Day: 3, Total: total(20), Source: RollNightAhead},
		{Year: 5, Month: 2, Day: 3, Total: total(19), Source: RollNightAhead},
	}, false); err != nil {
		t.Fatalf("PutNights reroll: %v", err)
	}
	year5, _ = rolls.ListNights(ctx, evt.ID, d1, d2)
	if *year5[0].Total != 4 || !year5[0].Locked {
		t.Errorf("locked night was rerolled: %+v", year5[0])
	}
	if *year5[1].Total != 19 || year5[1].Told != nil {
		t.Errorf("unlocked night = %+v, want total 19 and told reset", year5[1])
	}

	// onlyNew keeps whatever is stored: the first reader's roll stands.
	if err := rolls.PutNights(ctx, evt.ID, []RollNight{{Year: 6, Month: 1, Day: 3, Total: total(2), Source: RollNightOnDay}}, true); err != nil {
		t.Fatalf("PutNights onlyNew: %v", err)
	}
	y6, _ := rolls.ListNights(ctx, evt.ID, d3, d3)
	if len(y6) != 1 || *y6[0].Total != 15 {
		t.Errorf("onlyNew replaced a stored night: %+v", y6)
	}

	if n, err := rolls.ClearNights(ctx, evt.ID, []DayDate{d1, d2, d3}, true); err != nil || n != 2 {
		t.Fatalf("ClearNights keeping locked = %d, %v; want 2", n, err)
	}

	cfgs, err := rolls.ConfigsFor(ctx, []string{evt.ID, "missing"})
	if err != nil || len(cfgs) != 1 || cfgs[evt.ID] == nil {
		t.Fatalf("ConfigsFor = %+v, %v", cfgs, err)
	}

	// Deleting the event takes its roll and nights with it.
	if err := eventRepo.DeleteEvent(ctx, evt.ID); err != nil {
		t.Fatalf("DeleteEvent: %v", err)
	}
	if got, err := rolls.GetConfig(ctx, evt.ID); err != nil || got != nil {
		t.Errorf("roll outlived its event: %+v, %v", got, err)
	}
	if left, _ := rolls.ListNights(ctx, evt.ID, d1, d3); len(left) != 0 {
		t.Errorf("nights outlived their event: %+v", left)
	}
}
