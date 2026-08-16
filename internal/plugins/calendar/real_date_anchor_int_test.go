// real_date_anchor_int_test.go — the anchor against a REAL MariaDB.
//
// WHY THIS CLAIM NEEDED A DATABASE. Everything else about the anchor is
// arithmetic and can be proved with a struct. Exactly one thing cannot:
//
//	that migration 018's four columns EXIST, and that a nullable DATE
//	round-trips through the driver into a *time.Time and back.
//
// A mock repository is told what to return. It cannot tell you that
// `anchor_real_date` scans without `parseTime`, that `ADD COLUMN IF NOT EXISTS`
// applied at all, or that `calendarCols`' positional read still lines up after
// four columns were appended — and that last one is the failure mode that does
// not error: shift the list and MariaDB happily scans `visibility` into
// `RealTimeZone` and a date into an int, or worse, scans four values into the
// right types and the WRONG fields.
//
// SKIPS rather than fails with no server, per the house convention.
// `make test-db-up` provides one.
package calendar

import (
	"context"
	"testing"
	"time"
)

func TestAnchorInt_RoundTripsThroughTheRow(t *testing.T) {
	db := newCalendarScratchSchema(t)
	ctx := context.Background()
	_, cal := calTestSeedNavCalendar(t, db)
	repo := NewCalendarRepository(db)

	// ── 1. a fresh calendar is UN-ANCHORED ─────────────────────────────────
	//
	// Migration 018 NULL-backfills, so every calendar that already exists in
	// the operator's database must read back exactly like this. If the columns
	// defaulted to 0 instead, every campaign in the product would silently
	// acquire an anchor on year 0 month 0 day 0.
	got, err := repo.GetByID(ctx, cal.ID)
	if err != nil {
		t.Fatalf("reading the seeded calendar: %v", err)
	}
	if got == nil {
		t.Fatal("the seeded calendar did not read back")
	}
	if got.AnchorYear != nil || got.AnchorMonth != nil || got.AnchorDay != nil || got.AnchorRealDate != nil {
		t.Fatalf("a fresh calendar came back anchored: y=%v m=%v d=%v real=%v — migration 018 "+
			"must NULL-backfill, or every existing campaign acquires an anchor nobody set",
			got.AnchorYear, got.AnchorMonth, got.AnchorDay, got.AnchorRealDate)
	}
	if got.HasRealAnchor() {
		t.Error("HasRealAnchor() is true on a calendar with four NULLs")
	}

	// ── 2. the write, and the read back ────────────────────────────────────
	want := &RealDateAnchor{Year: 1492, Month: 4, Day: 14,
		RealDate: time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)}
	if err := repo.SetRealDateAnchor(ctx, cal.ID, want); err != nil {
		t.Fatalf("writing the anchor: %v", err)
	}
	got, err = repo.GetByID(ctx, cal.ID)
	if err != nil {
		t.Fatalf("re-reading: %v", err)
	}
	if got.AnchorYear == nil || got.AnchorMonth == nil || got.AnchorDay == nil || got.AnchorRealDate == nil {
		t.Fatalf("the anchor did not survive the round trip: y=%v m=%v d=%v real=%v",
			got.AnchorYear, got.AnchorMonth, got.AnchorDay, got.AnchorRealDate)
	}
	if *got.AnchorYear != 1492 || *got.AnchorMonth != 4 || *got.AnchorDay != 14 {
		t.Errorf("read back %d/%d/%d, want 1492/4/14 — if these are plausible but wrong, "+
			"check calendarCols against scanCalendar: the read is POSITIONAL",
			*got.AnchorYear, *got.AnchorMonth, *got.AnchorDay)
	}
	// The DATE comes back in whatever location the driver chose; compare the
	// calendar date, which is the only thing a DATE column carries.
	gy, gm, gd := got.AnchorRealDate.Date()
	if gy != 2026 || gm != time.October || gd != 3 {
		t.Errorf("the real date read back as %s, want 2026-10-03. A DATE that arrives shifted "+
			"by a day is a zone bug in the driver round trip, and it would move every "+
			"in-world day by one", got.AnchorRealDate.Format(time.RFC3339))
	}

	// ── 3. THE NEIGHBOURING COLUMNS ARE UNDISTURBED ────────────────────────
	//
	// This is the assertion the whole file is really for. `calendarCols` is read
	// POSITIONALLY by scanCalendar, so appending four columns is only safe if
	// the append really was an append. A shifted list can still scan cleanly and
	// put the right TYPES in the WRONG FIELDS.
	if got.Name != cal.Name {
		t.Errorf("Name read back as %q, want %q — the positional scan has shifted", got.Name, cal.Name)
	}
	if got.Mode != cal.Mode {
		t.Errorf("Mode read back as %q, want %q — the positional scan has shifted", got.Mode, cal.Mode)
	}
	if got.CampaignID != cal.CampaignID {
		t.Error("CampaignID read back wrong — the positional scan has shifted")
	}
	if got.Visibility == "" {
		t.Error("Visibility read back empty; it is the column immediately before the " +
			"real-time pair and would be the first casualty of a mis-ordered list")
	}

	// ── 4. the calendar can now answer, and its answer is the arithmetic's ──
	//
	// THE MONTHS ARE LOADED SEPARATELY, and that is the product's shape rather
	// than a test convenience: `calendarCols` reads the ROW, and the structure
	// sub-resources are eager-loaded by the service. A calendar straight out of
	// GetByID has no Months, so HasRealAnchor() is false on it and RealDateFor
	// refuses — which is correct (it has no structure to count over) and is
	// exactly why SetRealDateAnchor loads them before validating. The first
	// draft of this test assigned the seed helper's `cal.Months`, which the
	// helper never populates, and the refusal it got was real.
	months, err := repo.GetMonths(ctx, cal.ID)
	if err != nil {
		t.Fatalf("loading months: %v", err)
	}
	if len(months) == 0 {
		t.Fatal("the seeded calendar has no months — step 4 would prove nothing")
	}
	if got.HasRealAnchor() {
		t.Error("a row with no Months loaded reported HasRealAnchor() — it has no structure " +
			"to count over, so every date it produced would be nonsense")
	}
	got.Months = months
	rd, ok := got.RealDateFor(1492, 4, 14)
	if !ok {
		t.Fatal("a calendar read back WITH an anchor and its months still refuses to map its " +
			"own anchor day")
	}
	if y, m, d := rd.Date(); y != 2026 || m != time.October || d != 3 {
		t.Errorf("the stored anchor's own day maps to %s, not to the date it was pinned to",
			rd.Format("2006-01-02"))
	}

	// ── 5. the clear really clears, all four ───────────────────────────────
	if err := repo.SetRealDateAnchor(ctx, cal.ID, nil); err != nil {
		t.Fatalf("clearing: %v", err)
	}
	got, err = repo.GetByID(ctx, cal.ID)
	if err != nil {
		t.Fatalf("re-reading after the clear: %v", err)
	}
	if got.AnchorYear != nil || got.AnchorMonth != nil || got.AnchorDay != nil || got.AnchorRealDate != nil {
		t.Errorf("the clear left something behind: y=%v m=%v d=%v real=%v. Three NULLs and one "+
			"value is a row HasRealAnchor() rejects — an anchor the owner set and the product "+
			"silently ignores, which is the worst of the three states",
			got.AnchorYear, got.AnchorMonth, got.AnchorDay, got.AnchorRealDate)
	}
}
