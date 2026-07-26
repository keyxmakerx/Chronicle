package sessions

// Temporary offered availability (C-CAL-RSVP-P2).
//
// The whole reason this code exists is one invariant: exception rows REPLACE the
// recurring pattern for a date (effectiveBlocks, availability_overlay.go). So an
// "I could also do Tuesday evening" offer that wrote only the offered window
// would leave the member LESS available than before — the exact opposite of what
// they just said. Every test below is some form of that claim.

import (
	"reflect"
	"testing"
	"time"
)

// 2026-08-04 is a Tuesday; used throughout so the recurring weekday lines up.
const offerTuesday = "2026-08-04"

func blocksEqual(t *testing.T, got, want []ExceptionBlockDTO) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("composed day mismatch\n got: %+v\nwant: %+v", got, want)
	}
}

// TestComposeOfferedDay_ExtendsRecurringPatternInsteadOfErasingIt is the
// headline case: a member whose usual Tuesday is 18:00–22:00 offers 20:00–23:00.
// The result must be 18:00–23:00 — the union — not the bare offer.
func TestComposeOfferedDay_ExtendsRecurringPatternInsteadOfErasingIt(t *testing.T) {
	recurring := []AvailabilityBlock{
		{DayOfWeek: int(time.Tuesday), StartMinute: 18 * 60, EndMinute: 22 * 60, State: AvailAvailable},
	}
	offers := []AvailabilityWindowDTO{{OnDate: offerTuesday, StartMinute: 20 * 60, EndMinute: 23 * 60}}

	got, err := composeOfferedDay(offerTuesday, offers, recurring, nil)
	if err != nil {
		t.Fatalf("composeOfferedDay: %v", err)
	}
	blocksEqual(t, got, []ExceptionBlockDTO{
		{StartMinute: 18 * 60, EndMinute: 23 * 60, State: AvailAvailable},
	})
}

// TestComposeOfferedDay_KeepsUnrelatedHours pins that a DISJOINT offer keeps the
// member's usual hours as a separate block rather than replacing them.
func TestComposeOfferedDay_KeepsUnrelatedHours(t *testing.T) {
	recurring := []AvailabilityBlock{
		{DayOfWeek: int(time.Tuesday), StartMinute: 9 * 60, EndMinute: 11 * 60, State: AvailAvailable},
	}
	offers := []AvailabilityWindowDTO{{OnDate: offerTuesday, StartMinute: 19 * 60, EndMinute: 21 * 60}}

	got, err := composeOfferedDay(offerTuesday, offers, recurring, nil)
	if err != nil {
		t.Fatalf("composeOfferedDay: %v", err)
	}
	blocksEqual(t, got, []ExceptionBlockDTO{
		{StartMinute: 9 * 60, EndMinute: 11 * 60, State: AvailAvailable},
		{StartMinute: 19 * 60, EndMinute: 21 * 60, State: AvailAvailable},
	})
}

// TestComposeOfferedDay_PreferredIsNotDowngraded: a generic offer must not
// overwrite an explicit preference. Offering 18:00–22:00 across a preferred
// 19:00–20:00 leaves the preferred hour intact.
func TestComposeOfferedDay_PreferredIsNotDowngraded(t *testing.T) {
	recurring := []AvailabilityBlock{
		{DayOfWeek: int(time.Tuesday), StartMinute: 19 * 60, EndMinute: 20 * 60, State: AvailPreferred},
	}
	offers := []AvailabilityWindowDTO{{OnDate: offerTuesday, StartMinute: 18 * 60, EndMinute: 22 * 60}}

	got, err := composeOfferedDay(offerTuesday, offers, recurring, nil)
	if err != nil {
		t.Fatalf("composeOfferedDay: %v", err)
	}
	blocksEqual(t, got, []ExceptionBlockDTO{
		{StartMinute: 18 * 60, EndMinute: 19 * 60, State: AvailAvailable},
		{StartMinute: 19 * 60, EndMinute: 20 * 60, State: AvailPreferred},
		{StartMinute: 20 * 60, EndMinute: 22 * 60, State: AvailAvailable},
	})
}

// TestComposeOfferedDay_OverwritesUnavailable: the member is now explicitly
// saying they COULD make this time, so an `unavailable` override yields to it.
func TestComposeOfferedDay_OverwritesUnavailable(t *testing.T) {
	existing := []AvailabilityException{
		{OnDate: offerTuesday, StartMinute: 0, EndMinute: 1440, State: AvailUnavailable},
	}
	offers := []AvailabilityWindowDTO{{OnDate: offerTuesday, StartMinute: 18 * 60, EndMinute: 22 * 60}}

	got, err := composeOfferedDay(offerTuesday, offers, nil, existing)
	if err != nil {
		t.Fatalf("composeOfferedDay: %v", err)
	}
	blocksEqual(t, got, []ExceptionBlockDTO{
		{StartMinute: 0, EndMinute: 18 * 60, State: AvailUnavailable},
		{StartMinute: 18 * 60, EndMinute: 22 * 60, State: AvailAvailable},
		{StartMinute: 22 * 60, EndMinute: 1440, State: AvailUnavailable},
	})
}

// TestComposeOfferedDay_ExistingExceptionsWinOverRecurring pins the precedence
// the overlay itself uses: when a date already carries exception rows, THOSE are
// the day — the recurring pattern is not consulted.
func TestComposeOfferedDay_ExistingExceptionsWinOverRecurring(t *testing.T) {
	recurring := []AvailabilityBlock{
		{DayOfWeek: int(time.Tuesday), StartMinute: 9 * 60, EndMinute: 17 * 60, State: AvailAvailable},
	}
	existing := []AvailabilityException{
		{OnDate: offerTuesday, StartMinute: 20 * 60, EndMinute: 21 * 60, State: AvailAvailable},
	}
	offers := []AvailabilityWindowDTO{{OnDate: offerTuesday, StartMinute: 21 * 60, EndMinute: 22 * 60}}

	got, err := composeOfferedDay(offerTuesday, offers, recurring, existing)
	if err != nil {
		t.Fatalf("composeOfferedDay: %v", err)
	}
	// The 9–17 recurring block must NOT reappear.
	blocksEqual(t, got, []ExceptionBlockDTO{
		{StartMinute: 20 * 60, EndMinute: 22 * 60, State: AvailAvailable},
	})
}

// TestComposeOfferedDay_OtherDatesIgnored pins that an exception on a DIFFERENT
// date doesn't bleed into this one.
func TestComposeOfferedDay_OtherDatesIgnored(t *testing.T) {
	existing := []AvailabilityException{
		{OnDate: "2026-08-05", StartMinute: 0, EndMinute: 1440, State: AvailUnavailable},
	}
	offers := []AvailabilityWindowDTO{{OnDate: offerTuesday, StartMinute: 18 * 60, EndMinute: 19 * 60}}

	got, err := composeOfferedDay(offerTuesday, offers, nil, existing)
	if err != nil {
		t.Fatalf("composeOfferedDay: %v", err)
	}
	blocksEqual(t, got, []ExceptionBlockDTO{
		{StartMinute: 18 * 60, EndMinute: 19 * 60, State: AvailAvailable},
	})
}

// TestComposeOfferedDay_MergesTwoOffersOnOneDay pins that two windows offered on
// the same date compose together rather than the second replacing the first.
func TestComposeOfferedDay_MergesTwoOffersOnOneDay(t *testing.T) {
	offers := []AvailabilityWindowDTO{
		{OnDate: offerTuesday, StartMinute: 10 * 60, EndMinute: 12 * 60},
		{OnDate: offerTuesday, StartMinute: 12 * 60, EndMinute: 14 * 60}, // abuts the first
		{OnDate: offerTuesday, StartMinute: 20 * 60, EndMinute: 21 * 60}, // disjoint
	}
	got, err := composeOfferedDay(offerTuesday, offers, nil, nil)
	if err != nil {
		t.Fatalf("composeOfferedDay: %v", err)
	}
	blocksEqual(t, got, []ExceptionBlockDTO{
		{StartMinute: 10 * 60, EndMinute: 14 * 60, State: AvailAvailable}, // abutting windows coalesce
		{StartMinute: 20 * 60, EndMinute: 21 * 60, State: AvailAvailable},
	})
}

func TestComposeOfferedDay_RejectsBadDate(t *testing.T) {
	offers := []AvailabilityWindowDTO{{OnDate: "nope", StartMinute: 0, EndMinute: 60}}
	if _, err := composeOfferedDay("nope", offers, nil, nil); err == nil {
		t.Error("a malformed date must be rejected")
	}
}

// TestRunsToBlocks_OmitsUnpaintedMinutes pins that a gap produces NO row. On a
// date carrying exception rows, the rows ARE the effective day, so a gap already
// means "not available" — an explicit unavailable row would be redundant.
func TestRunsToBlocks_OmitsUnpaintedMinutes(t *testing.T) {
	canvas := make([]string, 1440)
	for i := 60; i < 120; i++ {
		canvas[i] = AvailAvailable
	}
	blocksEqual(t, runsToBlocks(canvas), []ExceptionBlockDTO{
		{StartMinute: 60, EndMinute: 120, State: AvailAvailable},
	})

	blocksEqual(t, runsToBlocks(make([]string, 1440)), []ExceptionBlockDTO{})
}

// TestAddMyAvailableWindows_Validation covers the guards that run before any
// write: empty submissions, over-cap submissions, bad zones, and bad ranges.
func TestAddMyAvailableWindows_Validation(t *testing.T) {
	svc := &sessionService{}
	ok := []AvailabilityWindowDTO{{OnDate: offerTuesday, StartMinute: 60, EndMinute: 120}}

	tests := []struct {
		name    string
		tz      string
		windows []AvailabilityWindowDTO
	}{
		{"no windows", "UTC", nil},
		{"too many windows", "UTC", make([]AvailabilityWindowDTO, maxOfferedWindows+1)},
		{"invalid timezone", "Not/AZone", ok},
		{"backwards range", "UTC", []AvailabilityWindowDTO{{OnDate: offerTuesday, StartMinute: 600, EndMinute: 60}}},
		{"out-of-day range", "UTC", []AvailabilityWindowDTO{{OnDate: offerTuesday, StartMinute: 0, EndMinute: 2000}}},
		{"malformed date", "UTC", []AvailabilityWindowDTO{{OnDate: "5th of never", StartMinute: 60, EndMinute: 120}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// A nil repo is safe: every case above must fail validation BEFORE
			// any repository call, which is itself part of the claim.
			if err := svc.AddMyAvailableWindows(t.Context(), "camp-1", "u1", tt.tz, tt.windows); err == nil {
				t.Error("expected rejection")
			}
		})
	}
}
