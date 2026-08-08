package calendar

// months_edit_impact_test.go — C-CALV4-GAMEREADY §9, [GR-17] and [GR-18].
//
// [GR-17] IS THE REPRODUCTION AND IT CAME FIRST. §9 arrived as a [READ]
// finding — read from source, never observed running — and a [READ] finding is
// a report before it is a bug. So the first test in this file is the
// reproduction the ruling demanded, written against a REAL MariaDB
// (`make test-db-up`) and the SHIPPED repository, not a fake: the whole claim
// is about what delete-and-reinsert does to rows that reference months by
// POSITION, and a mock repo cannot answer that.
//
// IT REPRODUCED. Both halves. See TestMonthsEdit_Reproduction for the numbers.
//
// [GR-18] then binds what may ship: a COUNT and a SENTENCE. Zero migrations,
// zero data writes, never a rewrite of a stored Month. [GR-SIGN-B] (SIGNED
// 2026-08-07) adds the sharper half: WARN, NEVER REFUSE — locking the operator
// out of editing their own months, in the exact week they are most likely to
// reshape them, is worse than the bug — AND THE COUNT IS A BEFORE/AFTER
// COMPARISON, NOT A BOUNDS CHECK, because a `month > len(months)` count catches
// DELETION and reports ZERO for INSERTION, which is the likelier edit.

import (
	"context"
	"testing"
)

// monthsFixture seeds a six-month calendar carrying events in months 4, 5 and 6
// — the shape [GR-17] names — and returns the repo and the calendar.
func monthsFixture(t *testing.T) (CalendarRepository, *Calendar) {
	t.Helper()
	db := newCalendarScratchSchema(t)
	ctx := context.Background()
	campaignID := calTestSeedCampaign(t, db)
	repo := NewCalendarRepository(db)

	cal := &Calendar{
		ID: calTestID(t), CampaignID: campaignID, Name: "Harptos of Imix", Mode: ModeFantasy,
		IsDefault: true, CurrentYear: 1523, CurrentMonth: 1, CurrentDay: 1,
		HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60,
	}
	if err := repo.Create(ctx, cal); err != nil {
		t.Fatalf("create calendar: %v", err)
	}
	if err := repo.SetMonths(ctx, cal.ID, monthsSix()); err != nil {
		t.Fatalf("set months: %v", err)
	}
	for _, e := range []struct {
		name        string
		month, day  int
	}{
		{"Spring Rites", 4, 3},
		{"The Long Siege", 5, 11},
		{"Midsummer Fair", 5, 20},
		{"Harvest Moot", 6, 2},
	} {
		row := Event{
			ID: calTestID(t), CalendarID: cal.ID, Name: e.name,
			Year: 1523, Month: e.month, Day: e.day, AllDay: true,
			Visibility: storageVisibilityEveryone,
		}
		if err := repo.CreateEvent(ctx, &row); err != nil {
			t.Fatalf("create event %q: %v", e.name, err)
		}
	}
	return repo, cal
}

func monthsSix() []MonthInput {
	return []MonthInput{
		{Name: "Deepwinter", Days: 30, SortOrder: 1},
		{Name: "Thawrun", Days: 30, SortOrder: 2},
		{Name: "Sunfall", Days: 30, SortOrder: 3},
		{Name: "Greengrass", Days: 30, SortOrder: 4},
		{Name: "Highsun", Days: 30, SortOrder: 5},
		{Name: "Leaffall", Days: 30, SortOrder: 6},
	}
}

// monthsWithIntercalaryAtFive is the edit [GR-17] names: an intercalary month
// inserted at position 5 — exactly what a GM does while iterating on a
// Harptos-shaped calendar in the week before a game.
func monthsWithIntercalaryAtFive() []MonthInput {
	return []MonthInput{
		{Name: "Deepwinter", Days: 30, SortOrder: 1},
		{Name: "Thawrun", Days: 30, SortOrder: 2},
		{Name: "Sunfall", Days: 30, SortOrder: 3},
		{Name: "Greengrass", Days: 30, SortOrder: 4},
		{Name: "Shieldmeet", Days: 1, SortOrder: 5, IsIntercalary: true},
		{Name: "Highsun", Days: 30, SortOrder: 6},
		{Name: "Leaffall", Days: 30, SortOrder: 7},
	}
}

// monthNameFor resolves an event's stored 1-based POSITION against a month list,
// which is exactly how every projection in this plugin resolves it
// (block_projection.go: `e.OccursOn(cal, year, mi+1, d)`).
func monthNameFor(months []Month, month int) string {
	if month < 1 || month > len(months) {
		return ""
	}
	return months[month-1].Name
}

// TestMonthsEdit_Reproduction is [GR-17], and it is the first thing §9 did.
//
// THE RESULT, STATED SO IT IS NOT RE-DERIVED:
//
//	INSERTION at position 5 — three of four events silently RE-DATE. Highsun's
//	two events and Leaffall's one all move one month later; nothing is
//	stranded, nothing warns, and a `month > len(months)` bounds check would
//	report ZERO.
//
//	DELETION of the last month — its events are STRANDED at a position nothing
//	renders. blockDateLine reports "Date out of range" for the CALENDAR's own
//	current date but nothing reports out-of-range EVENTS.
//
// Both halves reproduce. §9 is a bug, not a report.
func TestMonthsEdit_Reproduction(t *testing.T) {
	repo, cal := monthsFixture(t)
	ctx := context.Background()

	before, err := repo.GetMonths(ctx, cal.ID)
	if err != nil {
		t.Fatalf("months before: %v", err)
	}
	evBefore, err := repo.ListAllEvents(ctx, cal.ID)
	if err != nil {
		t.Fatalf("events before: %v", err)
	}
	if len(evBefore) != 4 {
		t.Fatalf("fixture drifted: %d events, want 4", len(evBefore))
	}
	was := map[string]string{}
	for _, e := range evBefore {
		was[e.Name] = monthNameFor(before, e.Month)
	}

	// --- HALF ONE: INSERTION ------------------------------------------------
	if err := repo.SetMonths(ctx, cal.ID, monthsWithIntercalaryAtFive()); err != nil {
		t.Fatalf("inserting an intercalary month: %v", err)
	}
	after, err := repo.GetMonths(ctx, cal.ID)
	if err != nil {
		t.Fatalf("months after insert: %v", err)
	}
	evAfter, err := repo.ListAllEvents(ctx, cal.ID)
	if err != nil {
		t.Fatalf("events after insert: %v", err)
	}

	shifted, stranded := 0, 0
	for _, e := range evAfter {
		now := monthNameFor(after, e.Month)
		switch {
		case now == "":
			stranded++
		case now != was[e.Name]:
			shifted++
			t.Logf("REPRODUCED (shift): %q was %s %d, now renders as %s %d",
				e.Name, was[e.Name], e.Day, now, e.Day)
		}
	}
	if shifted != 3 {
		t.Errorf("insertion at position 5 shifted %d event(s); the reproduction measured 3 "+
			"(both Highsun events and the Leaffall one)", shifted)
	}
	if stranded != 0 {
		t.Errorf("insertion stranded %d event(s); it should strand NONE — that is precisely "+
			"why a bounds check reports zero here", stranded)
	}

	// --- HALF TWO: DELETION -------------------------------------------------
	if err := repo.SetMonths(ctx, cal.ID, monthsSix()[:5]); err != nil {
		t.Fatalf("deleting the last month: %v", err)
	}
	shrunk, err := repo.GetMonths(ctx, cal.ID)
	if err != nil {
		t.Fatalf("months after delete: %v", err)
	}
	evShrunk, err := repo.ListAllEvents(ctx, cal.ID)
	if err != nil {
		t.Fatalf("events after delete: %v", err)
	}
	stranded = 0
	for _, e := range evShrunk {
		if monthNameFor(shrunk, e.Month) == "" {
			stranded++
			t.Logf("REPRODUCED (stranded): %q points at month %d of %d — nothing renders it",
				e.Name, e.Month, len(shrunk))
		}
	}
	if stranded == 0 {
		t.Error("deleting the last month stranded nothing; the [READ] finding did not " +
			"reproduce and §9 would close as a report")
	}

	// AND THE STORED ROWS WERE NEVER TOUCHED, which is the fact that makes this
	// silent: SetMonths reconciles nothing, so every event still holds the
	// position it was authored with.
	for _, e := range evShrunk {
		for _, b := range evBefore {
			if b.ID == e.ID && b.Month != e.Month {
				t.Errorf("%q's stored Month changed from %d to %d — the repository is not "+
					"supposed to rewrite events at all", e.Name, b.Month, e.Month)
			}
		}
	}
}
