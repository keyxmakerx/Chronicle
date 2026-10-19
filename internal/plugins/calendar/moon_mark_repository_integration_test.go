// moon_mark_repository_integration_test.go exercises MoonMarkRepository
// against a real MariaDB: the mode and state words read back with the moon,
// a mark replaced on its day, marks read only for manual moons, and the
// cascade from a deleted moon. Skipped under `-short`.
package calendar

import (
	"context"
	"testing"
)

func TestMoonMarkRepository_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	t.Cleanup(func() { db.Close() })

	ctx := context.Background()
	calRepo, marks := NewCalendarRepository(db), NewMoonMarkRepository(db)
	fix := newTestCampaign(t, db, "moonmarks")
	cal := newTestCalendar(testUUID(t), fix.CampaignID, "Marks")
	if err := calRepo.Create(ctx, cal); err != nil {
		t.Fatalf("Create calendar: %v", err)
	}
	if err := calRepo.SetMoons(ctx, cal.ID, []MoonInput{{Name: "Umbra", CycleDays: 30}}); err != nil {
		t.Fatalf("SetMoons: %v", err)
	}
	moons, err := calRepo.GetMoons(ctx, cal.ID)
	if err != nil || len(moons) != 1 || moons[0].PhaseMode != MoonCycle {
		t.Fatalf("GetMoons = %+v, %v", moons, err)
	}
	id := moons[0].ID

	if ok, err := marks.SetMode(ctx, cal.ID, id, MoonManual, map[string]string{MoonVanished: "Gone below"}); err != nil || !ok {
		t.Fatalf("SetMode = %v, %v", ok, err)
	}
	if ok, err := marks.SetMode(ctx, "another-calendar", id, MoonCycle, nil); err != nil || ok {
		t.Fatalf("SetMode from another calendar = %v, %v; want not found", ok, err)
	}
	moons, _ = calRepo.GetMoons(ctx, cal.ID)
	if moons[0].PhaseMode != MoonManual || moons[0].StateNames[MoonVanished] != "Gone below" {
		t.Fatalf("mode and words did not round-trip: %+v", moons[0])
	}

	for _, k := range []MoonMark{
		{Year: 5, Month: 1, Day: 3, State: MoonFull},
		{Year: 5, Month: 1, Day: 9, State: MoonDark, Shown: true},
		{Year: 5, Month: 1, Day: 3, State: MoonHalf, Shown: true}, // replaces the first
	} {
		if err := marks.PutMark(ctx, id, k); err != nil {
			t.Fatalf("PutMark: %v", err)
		}
	}
	got, err := marks.MarksFor(ctx, cal.ID)
	if err != nil || len(got[id]) != 2 {
		t.Fatalf("MarksFor = %+v, %v", got, err)
	}
	for _, k := range got[id] {
		if k.Day == 3 && (k.State != MoonHalf || !k.Shown) {
			t.Errorf("the replaced mark reads %+v", k)
		}
	}
	if n, err := marks.CountMarks(ctx, id); err != nil || n != 2 {
		t.Errorf("CountMarks = %d, %v", n, err)
	}
	if ok, err := marks.DeleteMark(ctx, id, DayDate{Year: 5, Month: 1, Day: 9}); err != nil || !ok {
		t.Errorf("DeleteMark = %v, %v", ok, err)
	}

	// A moon back on its cycle keeps its marks but no read returns them.
	if _, err := marks.SetMode(ctx, cal.ID, id, MoonCycle, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := marks.MarksFor(ctx, cal.ID); len(got) != 0 {
		t.Errorf("a cycling moon's marks were read: %+v", got)
	}

	if err := calRepo.SetMoons(ctx, cal.ID, nil); err != nil {
		t.Fatalf("SetMoons (delete): %v", err)
	}
	if n, _ := marks.CountMarks(ctx, id); n != 0 {
		t.Errorf("deleting the moon left %d marks", n)
	}
}
