package calendar

// months_edit_impact_test.go — C-CALV4-GAMEREADY §9 [GR-18], the guard for what
// the reproduction in months_edit_repro_test.go proved. Read that file first:
// it is [GR-17], it ran before a line of this fix was written, and it is where
// the numbers come from.

import (
	"context"
	"strings"
	"testing"
)

// TestMonthEditImpact_CountsBothNumbers is [GR-18]'s guard: the service must
// report BOTH numbers, and it must report them WITHOUT writing anything.
//
// The insertion case is the one that matters. A `month > len(months)` bounds
// check would report Stranded=0 / Shifted=(not computed) here, which is a
// warning that reads as "nothing happened" while the operator's whole year
// moves — and [GR-SIGN-B] rules that certifying the damage is worse than not
// warning at all.
func TestMonthEditImpact_CountsBothNumbers(t *testing.T) {
	repo, cal := monthsFixture(t)
	ctx := context.Background()
	svc := &calendarService{repo: repo}

	// INSERTION: three shifted, none stranded.
	got, err := svc.MonthEditImpact(ctx, cal.ID, monthsWithIntercalaryAtFive())
	if err != nil {
		t.Fatalf("impact of an insertion: %v", err)
	}
	if got.Stranded != 0 || got.Shifted != 3 {
		t.Errorf("insertion at position 5 reported stranded=%d shifted=%d; want 0 and 3. "+
			"A bounds check would have said 0 and nothing, which is the [GR-SIGN-B] defect.",
			got.Stranded, got.Shifted)
	}

	// DELETION of the last month: one stranded (Harvest Moot), none shifted —
	// the four earlier months keep their names and positions.
	got, err = svc.MonthEditImpact(ctx, cal.ID, monthsSix()[:5])
	if err != nil {
		t.Fatalf("impact of a deletion: %v", err)
	}
	if got.Stranded != 1 || got.Shifted != 0 {
		t.Errorf("deleting the last month reported stranded=%d shifted=%d; want 1 and 0",
			got.Stranded, got.Shifted)
	}

	// A NO-OP EDIT REPORTS NOTHING. A warning that fires on a save that changed
	// nothing is a warning the operator learns to dismiss.
	got, err = svc.MonthEditImpact(ctx, cal.ID, monthsSix())
	if err != nil {
		t.Fatalf("impact of a no-op: %v", err)
	}
	if got.Any() {
		t.Errorf("an unchanged month list reported stranded=%d shifted=%d; both must be 0",
			got.Stranded, got.Shifted)
	}

	// A RENAME IN PLACE IS A SHIFT, and it should be: the event now prints a
	// different month name than the operator authored it under.
	renamed := monthsSix()
	renamed[4].Name = "Flamerule"
	got, err = svc.MonthEditImpact(ctx, cal.ID, renamed)
	if err != nil {
		t.Fatalf("impact of a rename: %v", err)
	}
	if got.Shifted != 2 {
		t.Errorf("renaming month 5 reported shifted=%d; want 2 — the two events in it now "+
			"render under a name their author never chose", got.Shifted)
	}

	// AND IT IS A READ. Nothing above may have written anything: not the months,
	// not the events. This is the [GR-18] bound — zero data writes — asserted
	// rather than promised.
	months, err := repo.GetMonths(ctx, cal.ID)
	if err != nil {
		t.Fatalf("months after four impact reads: %v", err)
	}
	if len(months) != 6 || months[4].Name != "Highsun" {
		t.Fatalf("MonthEditImpact WROTE to the month list: %d months, month 5 is %q",
			len(months), monthNameFor(months, 5))
	}
	events, err := repo.ListAllEvents(ctx, cal.ID)
	if err != nil {
		t.Fatalf("events after four impact reads: %v", err)
	}
	for _, e := range events {
		if e.Name == "Harvest Moot" && e.Month != 6 {
			t.Errorf("MonthEditImpact rewrote a stored Month (%q is now %d); [GR-18] forbids "+
				"every data write, and the correct new month for a shifted event is not "+
				"derivable anyway", e.Name, e.Month)
		}
	}
}

// TestMonthEditImpact_Sentence pins the words, because the sentence is the
// whole deliverable on the operator's side and [GR-SIGN-B] specifies that it
// states BOTH numbers.
func TestMonthEditImpact_Sentence(t *testing.T) {
	for _, tc := range []struct {
		name string
		imp  MonthEditImpact
		want string
	}{
		{"nothing moved", MonthEditImpact{}, ""},
		{"only shifted", MonthEditImpact{Shifted: 34},
			"34 events now fall in a different month than before."},
		{"only stranded", MonthEditImpact{Stranded: 7},
			"7 events no longer land on a real month."},
		{"both, the sentence [GR-SIGN-B] wrote out", MonthEditImpact{Stranded: 7, Shifted: 34},
			"7 events no longer land on a real month, and 34 events now fall in a different month than before."},
		{"singulars", MonthEditImpact{Stranded: 1, Shifted: 1},
			"1 event no longer lands on a real month, and 1 event now falls in a different month than before."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.imp.Sentence(); got != tc.want {
				t.Errorf("Sentence() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestStrandedEventCounts_TheStandingSurface pins the read-only half of
// [GR-18] against a REAL database, because it is one aggregate query joining
// calendar_events to a grouped count of calendar_months and a fake cannot
// answer whether that SQL is right.
//
// The warning at the moment of the save is not enough on its own: a GM who
// dismissed it, or whose structural edit arrived over the sync API, has no
// other way to learn that some of their events have stopped rendering.
func TestStrandedEventCounts_TheStandingSurface(t *testing.T) {
	repo, cal := monthsFixture(t)
	ctx := context.Background()

	campaignID := cal.CampaignID
	got, err := repo.StrandedEventCounts(ctx, campaignID)
	if err != nil {
		t.Fatalf("stranded counts on an intact calendar: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("an intact calendar reported %v; a calendar with nothing stranded must be "+
			"ABSENT from the map, because a zero row is a row that says nothing", got)
	}

	// Delete the last TWO months: Harvest Moot (month 6) strands, and so does
	// nothing else — the other three events sit in months 4 and 5.
	if err := repo.SetMonths(ctx, cal.ID, monthsSix()[:5]); err != nil {
		t.Fatalf("shrinking the month list: %v", err)
	}
	got, err = repo.StrandedEventCounts(ctx, campaignID)
	if err != nil {
		t.Fatalf("stranded counts after a deletion: %v", err)
	}
	if got[cal.ID] != 1 {
		t.Errorf("after deleting month 6 the campaign reported %d stranded event(s) on %s; "+
			"want 1", got[cal.ID], cal.ID)
	}

	// Shrink harder: months 4, 5 and 6 all gone → all four events stranded.
	if err := repo.SetMonths(ctx, cal.ID, monthsSix()[:3]); err != nil {
		t.Fatalf("shrinking further: %v", err)
	}
	got, err = repo.StrandedEventCounts(ctx, campaignID)
	if err != nil {
		t.Fatalf("stranded counts after a bigger deletion: %v", err)
	}
	if got[cal.ID] != 4 {
		t.Errorf("with months 4-6 gone the campaign reported %d stranded; want 4", got[cal.ID])
	}

	// Restoring the months clears it — the surface reports a STATE, so it must
	// go quiet when the operator fixes their calendar, with no reload or
	// reconciliation step in between.
	if err := repo.SetMonths(ctx, cal.ID, monthsSix()); err != nil {
		t.Fatalf("restoring the month list: %v", err)
	}
	got, err = repo.StrandedEventCounts(ctx, campaignID)
	if err != nil {
		t.Fatalf("stranded counts after the repair: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("restoring the months left %v stranded; the standing line must clear itself", got)
	}
}

// TestBenchAttention_PrintsTheStrandedLine pins the Bench row's words and its
// silence. It is the surface the operator actually meets.
func TestBenchAttention_PrintsTheStrandedLine(t *testing.T) {
	cals := []Calendar{benchFxHarptos()}
	id := cals[0].ID

	if rows := benchAttentionRows(cals, "camp-1", nil); len(rows) != 0 {
		t.Fatalf("a campaign with nothing stranded printed %d attention row(s): %v",
			len(rows), rows)
	}
	if rows := benchAttentionRows(cals, "camp-1", map[string]int{id: 0}); len(rows) != 0 {
		t.Error("a zero count printed a row; a row that says nothing is worse than no row")
	}

	rows := benchAttentionRows(cals, "camp-1", map[string]int{id: 7})
	if len(rows) != 1 {
		t.Fatalf("want exactly one stranded row, got %d", len(rows))
	}
	if !strings.Contains(rows[0].Label, "7 events no longer land on a real month") {
		t.Errorf("the stranded row says %q; it must state the [GR-SIGN-B] sentence", rows[0].Label)
	}
	if !rows[0].Bad {
		t.Error("the stranded row must be marked bad — it is a fault, not an aside")
	}

	one := benchAttentionRows(cals, "camp-1", map[string]int{id: 1})
	if !strings.Contains(one[0].Label, "1 event no longer lands on a real month") {
		t.Errorf("the singular reads %q", one[0].Label)
	}
}
