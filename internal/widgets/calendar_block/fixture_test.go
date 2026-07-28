// fixture_test.go — the two calendars the signed contract is drawn from.
//
// Harptos of Imix (Deepwinter 1523) and the Gregorian real-world calendar
// (July 2026), with the same fourteen events the mockup carries. Reproducing
// the contract's own data is what lets the render tests, the browser probe and
// the screenshot generator all be checked against the signed stills rather than
// against each other.
package calendar_block

import (
	"math"
	"strconv"
)

// ── the axis table: the locked (hue, pattern, glyph) triples ────────────────
type evType struct {
	axis    string
	pattern string
	glyph   string
	rank    int
}

var fxTypes = map[string]evType{
	"session":   {"var(--ev-session)", "p5", "■", 0},
	"quest":     {"var(--ev-quest)", "p2", "▲", 1},
	"festival":  {"var(--ev-festival)", "p4", "✦", 2},
	"social":    {"var(--ev-social)", "p1", "◆", 3},
	"downtime":  {"var(--ev-downtime)", "p3", "●", 4},
	"celestial": {"var(--ev-celestial)", "p6", "☾", 5},
}

type fxEvent struct {
	id         string
	day        int
	title      string
	kind       string
	gmOnly     bool // visibility == dm_only        → the gold NOTCH
	restricted bool // a visibility_rules restriction → the gold DIAMOND
	tied       bool
}

// fxHarptosEvents mirrors the mockup's EV list for the harptos calendar, in its
// declared order. The stacking order is FIXED by type rank, so two cells with
// the same load always look the same.
var fxHarptosEvents = []fxEvent{
	{"ev-1", 3, "Council of Wards", "social", false, false, true},
	{"ev-2", 5, "Barrow scouting", "quest", true, false, true},
	{"ev-3", 5, "Caravan due", "downtime", false, false, false},
	{"ev-4", 5, "Ward levy", "social", false, false, false},
	{"ev-5", 5, "Smith’s deadline", "downtime", false, false, false},
	{"ev-6", 5, "Rumour: the pale", "social", false, false, false},
	{"ev-7", 5, "Tithe collected", "downtime", false, false, false},
	{"ev-8", 8, "Supply run", "downtime", false, false, true},
	{"ev-9", 8, "Into the Barrow", "quest", true, false, false},
	{"ev-10", 12, "Nissa’s recital", "social", false, true, true},
	{"ev-11", 14, "Emberfall Vigil", "festival", false, false, false},
	{"ev-12", 17, "Ward-court summons", "social", false, true, false},
	{"ev-13", 21, "Frost fair", "festival", false, false, false},
	{"ev-14", 26, "Warden’s writ due", "quest", true, false, false},
}

// fxMoons are the three bodies the grid draws. A fourth (Sable, 88.2 days) is
// declared in the contract and deliberately past the grid's ceiling — the grid
// draws at most three so the month can never grow with the fiction.
var fxMoons = []struct {
	name   string
	period float64
	newAt  float64
}{
	{"Alder", 31.4, 12},
	{"Umber", 46.5, 19 - 46.5/2},
	{"Flint", 11.3, 3},
}

func fxPhase(period, newAt float64, day int) float64 {
	p := math.Mod(math.Mod((float64(day)-newAt)/period, 1)+1, 1)
	return p
}

func fxMoonDiscs(day int) []MoonDisc {
	out := make([]MoonDisc, 0, len(fxMoons))
	for _, m := range fxMoons {
		ph := fxPhase(m.period, m.newAt, day)
		out = append(out, MoonDisc{
			Name:   m.name,
			Illum:  (1 - math.Cos(2*math.Pi*ph)) / 2,
			Waxing: ph <= 0.5,
		})
	}
	return out
}

func fxMarks(day int, gm bool) []Mark {
	var out []Mark
	for rank := 0; rank < len(fxTypes); rank++ {
		for _, e := range fxHarptosEvents {
			t := fxTypes[e.kind]
			if e.day != day || t.rank != rank {
				continue
			}
			if e.gmOnly && !gm {
				continue // PERMISSION IS ABSENCE: the row is not in a player's DOM
			}
			m := Mark{
				EventID: e.id,
				Title:   e.title,
				Axis:    t.axis,
				Pattern: t.pattern,
				Glyph:   t.glyph,
				Named:   true,
			}
			if gm {
				switch {
				case e.gmOnly:
					m.Audience = &AudienceMark{Label: "GM only"}
				case e.restricted:
					m.Audience = &AudienceMark{Label: "Restricted", Restricted: true}
				}
			}
			out = append(out, m)
		}
	}
	return out
}

func fxTied(day int) bool {
	for _, e := range fxHarptosEvents {
		if e.day == day && e.tied {
			return true
		}
	}
	return false
}

// fxHarptos builds the ten-day-week calendar: 30 days, three tenday rows, two
// eras, one intercalary festival day that belongs to no tenday.
func fxHarptos(gm bool) BlockData {
	const (
		week = 10
		days = 30
		lead = 0
		half = 5 // the five-column rule, as the PRODUCER decides it
	)
	names := []string{"Sar", "Mol", "Zor", "Wir", "Nym", "Lyr", "Tam", "Kes", "Vel", "Odd"}
	wds := make([]Weekday, 0, week)
	for i, n := range names {
		wds = append(wds, Weekday{Name: n, Abbr: n, Index: i, Half: i+1 == half})
	}

	// Two eras with DISTINCT band hues, because --bandhue is driven by the ERA
	// and never hardcoded per calendar (calendar-v4.html:1566 is the defect).
	type era struct {
		label string
		from  int
		to    int
		hue   string
	}
	eras := []era{
		{"Reckoning of Wards", 1, 17, "oklch(0.55 0.12 200)"},
		{"Age of the Emberfall", 18, 30, "oklch(0.62 0.10 70)"},
	}

	rows := make([]WeekRow, 0, 3)
	for r := 0; r*week < lead+days; r++ {
		row := WeekRow{Index: r}
		first := r*week + 1 - lead
		last := first + week - 1
		if last > days {
			last = days
		}
		for i, e := range eras {
			from, to := e.from, e.to
			if from < first {
				from = first
			}
			if to > last {
				to = last
			}
			if to < from {
				continue
			}
			b := EraBand{
				Label:     e.label,
				StartCol:  ((from - 1 + lead) % week) + 1,
				Span:      to - from + 1,
				BandHue:   e.hue,
				OpenLeft:  from > e.from,
				OpenRight: to < e.to,
				Edge:      from == e.from && i > 0,
			}
			// THE SEASON IS A SUFFIX ON ROW 0 ONLY.
			if r == 0 {
				b.Suffix = "Long Night"
			}
			b.Half = b.StartCol+b.Span-1 == half
			row.Bands = append(row.Bands, b)
		}
		for c := 1; c <= week; c++ {
			day := r*week + c - lead
			cell := DayCell{Col: c, Half: c == half}
			if day >= 1 && day <= days {
				cell.Day = day
				cell.IsToday = day == 14
				cell.Marks = fxMarks(day, gm)
				cell.Moons = fxMoonDiscs(day)
				cell.Tied = fxTied(day)
				// Deliberately UNTRIMMED. Density is a container query, so the
				// producer cannot know which subtree will be visible and must
				// hand over everything the viewer may see; each subtree applies
				// its own ceiling. MoreCount stays 0 here because nothing was
				// dropped upstream.
			}
			row.Cells = append(row.Cells, cell)
		}
		rows = append(rows, row)
	}

	linked, total := 1, 4
	return BlockData{
		CalendarID:   "cal-1",
		CalendarSlug: "harptos-of-imix",
		Name:         "Harptos of Imix",
		CalHue:       "harptos",
		Pattern:      "p1",
		Letter:       "H",
		IsDefault:    true,
		IsActive:     true,
		DateLabel:    "14 Deepwinter 1523",
		SeasonLabel:  "Long Night",
		EraLabel:     "RoW",
		Month: MonthGeometry{
			Index:    0,
			Year:     1523,
			Name:     "Deepwinter",
			WeekLen:  week,
			Lead:     lead,
			Days:     days,
			RowCount: len(rows),
			Weekdays: wds,
			Rows:     rows,
			Intercalary: []IntercalaryDay{{
				Name: "Midwinter · intercalary — belongs to no tenday",
				Day:  1,
			}},
			TodayDay: 14, MoonsDeclared: len(fxMoons) + 1,
		},
		Sync: SyncPill{
			State:     "ok",
			Full:      "In sync · Foundry · pushed 2m ago · " + strconv.Itoa(linked) + " of " + strconv.Itoa(total) + " linked",
			Compact:   "In sync · " + strconv.Itoa(linked) + " of " + strconv.Itoa(total),
			Linked:    linked,
			Total:     total,
			Transport: "Foundry",
			PushedAgo: "pushed 2m ago",
		},
		// The signed stills show the Ledger docked and the Shelf at the foot,
		// so the still's layer state has both keys ON — the zones are
		// layer-owned (data.go's 8-key registry) and absent under bare DEF.
		Layers: LayerState{Enabled: []string{"moons", "eras", "weeknums", "ledger", "shelf"}},
		Ledger: LedgerStub{NeedsBackend: true},
		Shelf:  ShelfStub{NeedsBackend: true},
		Viewer: ViewerContext{IsGM: gm, UserID: "u-1", TieMode: "whole", Zone: "America/Chicago"},
	}
}

// fxGregorian is the seven-column half of the divergence proof: same host, same
// component, names instead of underlines, and no era band to draw.
func fxGregorian() BlockData {
	const (
		week = 7
		days = 31
		lead = 3
	)
	names := []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
	abbr := []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
	wds := make([]Weekday, 0, week)
	for i := range names {
		// No half rule on a seven-day week: 3+4 needs no counting aid.
		wds = append(wds, Weekday{Name: names[i], Abbr: abbr[i], Index: i})
	}

	rows := make([]WeekRow, 0, 6)
	for r := 0; r*week < lead+days; r++ {
		row := WeekRow{Index: r}
		for c := 1; c <= week; c++ {
			day := r*week + c - lead
			cell := DayCell{Col: c}
			if day >= 1 && day <= days {
				cell.Day = day
				cell.IsToday = day == 25
				if day == 25 {
					t := fxTypes["session"]
					cell.Marks = []Mark{{
						EventID: "ev-100", Title: "Session 41",
						Axis: t.axis, Pattern: t.pattern, Glyph: t.glyph, Named: true,
					}}
				}
			}
			row.Cells = append(row.Cells, cell)
		}
		rows = append(rows, row)
	}

	return BlockData{
		CalendarID:   "cal-2",
		CalendarSlug: "real-world",
		Name:         "Real world / Gregorian",
		CalHue:       "real",
		Pattern:      "p2",
		Letter:       "R",
		IsRealWorld:  true,
		DateLabel:    "Sat 25 Jul 2026",
		Month: MonthGeometry{
			Index: 6, Year: 2026, Name: "July",
			WeekLen: week, Lead: lead, Days: days,
			RowCount: len(rows), Weekdays: wds, Rows: rows, TodayDay: 25,
			MoonsDeclared: 1,
		},
		Sync: SyncPill{
			State:   "pause",
			Full:    "Paused · date push paused — tracks real time · 1 of 4 linked",
			Compact: "Paused · tracks real time · 1 of 4",
			Linked:  1, Total: 4,
		},
		// Both zone keys ON: the Bench still docks its Ledger, and keeping
		// "shelf" enabled is what makes the noShelf assertion prove Hidden —
		// with the key off, the Shelf's absence would be the layer gate's.
		Layers: LayerState{Enabled: []string{"moons", "ledger", "shelf"}},
		Ledger: LedgerStub{NeedsBackend: true},
		Shelf:  ShelfStub{Hidden: true}, // the Bench's real-world Block renders with noShelf
		Viewer: ViewerContext{IsGM: true, UserID: "u-1", TieMode: "whole", Zone: "America/Chicago"},
	}
}

// fxDwarvenFault is the signed honesty state: a calendar that cannot resolve a
// date says so WHERE THE DATE WOULD GO.
func fxDwarvenFault() BlockData {
	d := fxHarptos(true)
	d.CalendarID = "cal-4"
	d.CalendarSlug = "dwarven-deep-count"
	d.Name = "Dwarven Deep-count"
	d.CalHue = "dwarven"
	d.Pattern = "p4"
	d.Letter = "D"
	d.DateLabel = "" // no date element is emitted at all
	d.EraLabel = ""
	d.Fault = "Needs eras — 0 eras defined, dates cannot resolve"
	d.Sync = SyncPill{State: "none", Full: "Not linked · 0 of 4 linked", Compact: "Not linked · 0 of 4", Linked: 0, Total: 4}
	return d
}
