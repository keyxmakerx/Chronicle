// block_real_date_test.go — the anchor reaching the Block, end to end through
// the real producer (C-CALV4-ANCHOR).
//
// real_date_anchor_test.go proves the arithmetic and real_date_anchor_write_test.go
// proves the write. This file proves the JOIN: that projectBlock actually calls
// it, that every cell in a rendered month gets its own consecutive date, and —
// the half that matters most — that an UN-ANCHORED calendar emits nothing at
// all rather than a plausible-looking default.
//
// The last one is the reason this file exists as a producer test rather than a
// widget test. `DayCell.RealDate` is a plain string; a widget test can only
// assert what it was handed. Whether the PRODUCER invents a date when it has no
// anchor is only observable here.
package calendar

import (
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	calblock "github.com/keyxmakerx/chronicle/internal/widgets/calendar_block"
)

// brdCalendar is a small irregular fantasy calendar with a ten-day week — the
// operator's own shape, and irregular so a month walk that is off by one cannot
// round-trip by accident.
func brdCalendar(anchor bool) *Calendar {
	c := &Calendar{
		ID: "cal-brd", CampaignID: "camp-1", Name: "Harptos", Mode: "fantasy",
		Months: []Month{
			{Name: "Hammer", Days: 31, SortOrder: 0},
			{Name: "Alturiak", Days: 28, SortOrder: 1},
			{Name: "Ches", Days: 30, SortOrder: 2},
		},
		Weekdays: []Weekday{
			{Name: "1st"}, {Name: "2nd"}, {Name: "3rd"}, {Name: "4th"}, {Name: "5th"},
			{Name: "6th"}, {Name: "7th"}, {Name: "8th"}, {Name: "9th"}, {Name: "10th"},
		},
		CurrentYear: 1492, CurrentMonth: 1, CurrentDay: 5,
		HoursPerDay: 24, MinutesPerHour: 60,
	}
	if anchor {
		y, m, d := 1492, 1, 1
		rd := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
		c.AnchorYear, c.AnchorMonth, c.AnchorDay, c.AnchorRealDate = &y, &m, &d, &rd
	}
	return c
}

func brdProject(t *testing.T, cal *Calendar) calblock.BlockData {
	t.Helper()
	return projectBlock(BlockProjectionInput{
		Calendar: cal, MonthIndex: 0, Year: 1492,
		Viewer: BlockViewer{UserID: "u-1", Role: int(campaigns.RoleOwner)},
	})
}

// datedCells returns every cell that carries a real day (not a lead/trail
// blank), in grid order.
func datedCells(d calblock.BlockData) []calblock.DayCell {
	var out []calblock.DayCell
	for _, r := range d.Month.Rows {
		for _, c := range r.Cells {
			if c.Day > 0 {
				out = append(out, c)
			}
		}
	}
	return out
}

// TestBlockRealDate_UnanchoredEmitsNothing is the half that must not regress.
//
// EVERY CALENDAR IN THE PRODUCT IS UN-ANCHORED RIGHT NOW. If the producer
// substituted anything — today, the zero time, a formatted "1 Jan 0001" — then
// every day card in every campaign would print a confident, wrong real date,
// and the Ledger has no way to tell a real answer from a manufactured one.
func TestBlockRealDate_UnanchoredEmitsNothing(t *testing.T) {
	cells := datedCells(brdProject(t, brdCalendar(false)))
	if len(cells) == 0 {
		t.Fatal("the month produced no dated cells — this test would pass vacuously")
	}
	for _, c := range cells {
		if c.RealDate != "" {
			t.Fatalf("day %d carries RealDate %q on a calendar with NO anchor. The mapping "+
				"refuses rather than guessing precisely so this string is either a fact or "+
				"absent; a default here prints a wrong date on every campaign in the product",
				c.Day, c.RealDate)
		}
	}
	t.Logf("%d dated cells, 0 real dates — the un-anchored calendar stayed silent", len(cells))
}

// TestBlockRealDate_AnchoredFillsEveryDayConsecutively.
//
// Not a spot check: every dated cell in the month must carry a date, and the
// dates must advance by exactly one day in grid order. An off-by-one at a week
// boundary — the grid walks rows, not days — would leave the arithmetic correct
// and the GRID wrong, which no test in real_date_anchor_test.go can see.
func TestBlockRealDate_AnchoredFillsEveryDayConsecutively(t *testing.T) {
	cal := brdCalendar(true)
	cells := datedCells(brdProject(t, cal))
	if len(cells) != 31 {
		t.Fatalf("Hammer produced %d dated cells, want 31", len(cells))
	}

	var prev time.Time
	for i, c := range cells {
		if c.RealDate == "" {
			t.Fatalf("day %d carries no real date on an ANCHORED calendar", c.Day)
		}
		got, err := time.Parse("Mon 2 Jan 2006", c.RealDate)
		if err != nil {
			t.Fatalf("day %d's real date %q does not parse as the producer's own format: %v",
				c.Day, c.RealDate, err)
		}
		if i == 0 {
			// Day 1 is the anchor itself.
			if want := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC); !got.Equal(want) {
				t.Fatalf("day 1 renders %s but the anchor pins it to %s",
					got.Format("2006-01-02"), want.Format("2006-01-02"))
			}
		} else if want := prev.AddDate(0, 0, 1); !got.Equal(want) {
			t.Fatalf("day %d renders %s; the previous cell was %s, so the grid skipped or "+
				"repeated a real date. The grid walks ROWS — an error at a week boundary is "+
				"invisible to the arithmetic tests",
				c.Day, got.Format("2006-01-02"), prev.Format("2006-01-02"))
		}
		prev = got
	}
	t.Logf("31 consecutive real dates, %s → %s", cells[0].RealDate, cells[len(cells)-1].RealDate)
}

// TestBlockRealDate_TheWeekdayIsTheGREGORIANOne. The label's weekday must be the
// real world's, not the fantasy tenday's — the tenday is already named beside it
// in the panel, and two different weekday systems in one line is the kind of
// thing nobody notices until a session is booked on the wrong day.
func TestBlockRealDate_TheWeekdayIsTheGregorianOne(t *testing.T) {
	cells := datedCells(brdProject(t, brdCalendar(true)))
	for _, c := range cells[:7] {
		got, err := time.Parse("Mon 2 Jan 2006", c.RealDate)
		if err != nil {
			t.Fatalf("day %d: %v", c.Day, err)
		}
		// The label's own weekday word must agree with the parsed date's.
		if want := got.Format("Mon"); c.RealDate[:3] != want {
			t.Errorf("day %d renders %q, whose weekday disagrees with its own date (%s). "+
				"The calendar declares a TEN-day week; if a fantasy weekday has leaked into "+
				"this label it is naming a day that does not exist in the real world",
				c.Day, c.RealDate, want)
		}
	}
}

// TestBlockRealDate_ReachesTheLedgerDayPanel closes the last gap: the producer
// fills the cell, but the panel reads its own context struct, and a field that
// stops being copied across renders as a silently missing line.
func TestBlockRealDate_ReachesTheLedgerDayPanel(t *testing.T) {
	for _, tc := range []struct {
		name    string
		anchor  bool
		wantAny bool
	}{
		{"anchored", true, true},
		{"un-anchored", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := brdProject(t, brdCalendar(tc.anchor))
			d.Layers = calblock.LayerState{Enabled: []string{"ledger"}}
			d.Ledger = calblock.LedgerStub{NeedsBackend: false}
			html := seamRenderBlockData(t, d)

			has := strings.Contains(html, `class="ldrd"`)
			if has != tc.wantAny {
				if tc.wantAny {
					t.Error("the anchored Block rendered NO real-date line in the day panel — " +
						"the producer filled the cell and the panel dropped it")
				} else {
					t.Error("the un-anchored Block rendered a real-date line — the panel is " +
						"printing something for a day whose real date is not known")
				}
			}
			if tc.wantAny && !strings.Contains(html, "Sat 3 Oct 2026") {
				t.Error("the anchor's own day is missing its date from the rendered panel")
			}
		})
	}
}
