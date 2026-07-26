// sizing_test.go — the arithmetic pin.
//
// THE POINT OF THIS FILE is TestDivergence_Signed: it reproduces the signed
// divergence proof (cordinator mockups/calendar-v4.html:2504 —
// mockups/renders/v4-divergence-light.png) exactly. If the contract's model of a
// column ever drifts, this fails before a screenshot does.
package calendar_block

import (
	"math"
	"testing"
)

func round1(f float64) float64 { return math.Round(f*10) / 10 }

// TestDivergence_Signed is the gate. Same host width, two week lengths: a
// seven-column calendar can honestly afford names sooner than a ten-column one.
//
// SCOPED TO FULL TIER ON PURPOSE. ColWidth's full-tier form subtracts 300px for
// the docked Ledger UNCONDITIONALLY; at std and mini the Ledger is not beside
// the month and that subtraction does not apply. At full tier the Block always
// docks the Ledger zone — as a `needs backend` stub in wave 1 — so the
// subtraction is real from the first commit.
func TestDivergence_Signed(t *testing.T) {
	for _, tc := range []struct {
		name    string
		week    int
		hostW   int
		wantCol float64
		wantNam bool
	}{
		{"Harptos · 10 columns · 1040px host → underline", 10, 1040, 68.2, false},
		{"Real world · 7 columns · 1040px host → named", 7, 1040, 97.4, true},
		// The Bench's own host width, where a ten-day week finally earns names.
		{"Harptos · 10 columns · 1232px host → named", 10, 1232, 87.4, true},
		// The exact boundary the 84px rule puts a ten-day week at.
		{"Harptos · 10 columns · 1198px host → named, just", 10, 1198, 84.0, true},
		{"Harptos · 10 columns · 1197px host → underline, just", 10, 1197, 83.9, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := round1(ColWidth(TierFull, tc.hostW, tc.week)); got != tc.wantCol {
				t.Errorf("ColWidth(full, %d, %d) = %.1fpx; the signed sheet says %.1fpx",
					tc.hostW, tc.week, got, tc.wantCol)
			}
			if got := IsNamed(TierFull, tc.hostW, tc.week, 3); got != tc.wantNam {
				t.Errorf("IsNamed(full, %d, %d, 3) = %v; want %v", tc.hostW, tc.week, got, tc.wantNam)
			}
			// The shipped CSS implements the column-width clause alone; at three
			// rows the retired clause cannot change the answer, so the two must
			// agree here. Where they DIVERGE is pinned separately below.
			if got := IsNamedCSS(TierFull, tc.hostW, tc.week); got != tc.wantNam {
				t.Errorf("IsNamedCSS(full, %d, %d) = %v; want %v — the stylesheet and the contract disagree",
					tc.hostW, tc.week, got, tc.wantNam)
			}
		})
	}
}

// TestSizeSheetCaption_IsWrong records the contract defect rather than papering
// over it. sizeSheet (calendar-v4.html:2483) captions host 1040 as "10 columns
// at 100.2px → named chips"; the arithmetic says 68.2px, and the sheet's own
// rendered Block draws underlines. The contract is immutable, so this is booked,
// not fixed (COMMON §1: stop and flag, do not improve the design).
func TestSizeSheetCaption_IsWrong(t *testing.T) {
	const captioned = 100.2
	got := round1(ColWidth(TierFull, 1040, 10))
	if got == captioned {
		t.Fatalf("sizeSheet's caption now agrees with the arithmetic (%.1fpx). Either the "+
			"contract was re-signed or ColWidth changed; re-read the dispatch before "+
			"deleting this test.", got)
	}
	if got >= NamedColWidthMin {
		t.Errorf("ColWidth(full,1040,10) = %.1fpx, which would earn NAMED density; the "+
			"signed divergence render shows underline", got)
	}
}

func TestSizeClass_Thresholds(t *testing.T) {
	for _, tc := range []struct {
		hostW int
		want  string
	}{
		{2000, TierFull},
		{900, TierFull},
		{899, TierStd},
		{358, TierStd}, // the signed std/mobile still: 390px device less its gutters
		{300, TierStd},
		{299, TierMini},
		{280, TierMini}, // sizeSheet's MINI panel
		{240, TierMini},
		{239, TierSubmini},
		{220, TierSubmini}, // sizeSheet's SUB-MINI panel
		{0, TierSubmini},
	} {
		if got := SizeClass(tc.hostW); got != tc.want {
			t.Errorf("SizeClass(%d) = %q; want %q", tc.hostW, got, tc.want)
		}
	}
}

// TestColWidth_StdDoesNotSubtractTheLedger is the trap the dispatch names
// explicitly. Reusing the full-tier form at std would under-report a ten-day
// week's columns by 30px each.
func TestColWidth_StdDoesNotSubtractTheLedger(t *testing.T) {
	const host = 800
	full := ColWidth(TierFull, host, 10)
	std := ColWidth(TierStd, host, 10)
	// The full-tier form subtracts the docked Ledger AND the instrument's own
	// horizontal padding; the std form subtracts neither, because at std the
	// Ledger sits below the month rather than beside it.
	wantDelta := round1(float64(ledgerDock+instPad) / 10)
	if delta := round1(std - full); delta != wantDelta {
		t.Errorf("std minus full = %.1fpx per column; want the docked Ledger plus the "+
			"instrument padding spread over the week (%.1fpx)", delta, wantDelta)
	}
	if round1(float64(ledgerDock)/10) != 30.0 {
		t.Errorf("the docked Ledger is no longer 300px; the full-tier arithmetic and the " +
			"stylesheet's .layer-col width must move together")
	}
	// The signed std still: host 358, ten columns, 34.2px measured → underline.
	if got := round1(ColWidth(TierStd, 358, 10)); got != 34.2 {
		t.Errorf("ColWidth(std,358,10) = %.1fpx; the signed v4-block-std-mobile still says 34.2px", got)
	}
	if IsNamed(TierStd, 358, 10, 3) {
		t.Error("std at 358px must be underline density")
	}
}

// TestIsNamed_RetiredRowHeightClause makes the deliberate divergence VISIBLE.
//
// The mockup's second clause (`instrument / rows >= 68`, with a 470px constant
// at full tier against a measured 563px) refuses named density for a month tall
// enough to need seven rows, even when its columns are wide enough. The shipped
// CSS does not implement it: a container query measures the column, and vertical
// fit is settled by real layout. This test asserts the two DISAGREE exactly
// there, so the divergence cannot be silently reintroduced or silently lost.
func TestIsNamed_RetiredRowHeightClause(t *testing.T) {
	// Seven rows of a wide-column month: 470/7 = 67.1 < 68, so the mockup says
	// underline; the measured column is comfortably over 84px, so the CSS says
	// named.
	const hostW, week, rows = 1232, 7, 7
	if got := round1(ColWidth(TierFull, hostW, week)); got < NamedColWidthMin {
		t.Fatalf("test setup: ColWidth(full,%d,%d) = %.1fpx, expected a wide column", hostW, week, got)
	}
	if IsNamed(TierFull, hostW, week, rows) {
		t.Error("the mockup's row-height clause should refuse named density at 7 rows")
	}
	if !IsNamedCSS(TierFull, hostW, week) {
		t.Error("the shipped CSS clause should allow named density: the column is wide enough")
	}
	// And the constant that makes it so is the booked 470, NOT the measured 563.
	// Correcting it here was explicitly refused (canon amendments §D).
	if mockupRowHeightClause(TierFull, 7) {
		t.Error("the full-tier instrument constant is no longer 470px — correcting it to " +
			"the measured 563px desynchronises every signed still and is a stop-and-flag")
	}
}

func TestIsNamed_MiniAndSubminiAreNeverNamed(t *testing.T) {
	for _, tier := range []string{TierMini, TierSubmini} {
		if IsNamed(tier, 4000, 1, 1) {
			t.Errorf("%s must never carry named chips, whatever the arithmetic says", tier)
		}
		if IsNamedCSS(tier, 4000, 1) {
			t.Errorf("%s must never carry named chips in the CSS clause either", tier)
		}
	}
}

func TestColWidth_ZeroWeekIsSafe(t *testing.T) {
	if got := ColWidth(TierFull, 1232, 0); got != 0 {
		t.Errorf("ColWidth with a zero week length = %v; want 0 (no divide by zero)", got)
	}
}
