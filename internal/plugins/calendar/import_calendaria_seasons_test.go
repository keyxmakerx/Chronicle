// import_calendaria_seasons_test.go — C-SWEEP-R4 stage 23,
// backlog/calendaria-seasons-ignore-monthstart.
//
// parseCalendaria only ever implemented Calendaria's DAY-OF-YEAR season shape,
// so every file authored in the MONTH-RANGE shape collapsed onto whatever its
// dayStart/dayEnd happened to be. The shipped Elven preset is one of those
// files, which is why the fixture below is the real payload rather than a
// hand-written stand-in: the defect was in a product surface (the Start
// gallery's Elven card), and a synthetic fixture could have been written to
// agree with either reading.
package calendar

import (
	"encoding/json"
	"testing"
)

// seasonRange is a compact, comparable rendering of one imported season.
type seasonRange struct {
	Name                 string
	StartMonth, StartDay int
	EndMonth, EndDay     int
}

func importedSeasonRanges(t *testing.T, raw []byte) []seasonRange {
	t.Helper()
	res, err := DetectAndParse(raw)
	if err != nil {
		t.Fatalf("DetectAndParse: %v", err)
	}
	out := make([]seasonRange, 0, len(res.Seasons))
	for _, s := range res.Seasons {
		out = append(out, seasonRange{s.Name, s.StartMonth, s.StartDay, s.EndMonth, s.EndDay})
	}
	return out
}

func assertSeasonRanges(t *testing.T, got, want []seasonRange) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d seasons, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("season %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestParseCalendaria_ElvenPresetSeasonsUseMonthRange is the regression proper.
//
// presets/elven.json has eight 45-day months and three seasons whose ONLY
// distinguishing fields are monthStart/monthEnd (0-2, 3-5, 6-7); all three
// carry dayStart 0 and dayEnd 45. Reading only the day fields produced
// 1/1 → 1/45 three times over — three identical ranges, mutually overlapping,
// covering one of eight months and leaving the other seven seasonless. The
// three ranges below tile the year exactly once.
func TestParseCalendaria_ElvenPresetSeasonsUseMonthRange(t *testing.T) {
	raw, err := builderPresetFS.ReadFile("presets/elven.json")
	if err != nil {
		t.Fatalf("read embedded elven preset: %v", err)
	}
	got := importedSeasonRanges(t, raw)
	assertSeasonRanges(t, got, []seasonRange{
		{"Budding", 1, 1, 3, 45}, // Aevel 1 → Lethra 45
		{"Zenith", 4, 1, 6, 45},  // Vanyr 1 → Serel 45
		{"Waning", 7, 1, 8, 45},  // Thalor 1 → Myrren 45
	})

	// The whole point of a season range is that a date lands in exactly one.
	// Reading the day fields alone put every day of the year in Budding or in
	// none, so this is the product-level statement of the same defect.
	res, err := DetectAndParse(raw)
	if err != nil {
		t.Fatalf("DetectAndParse: %v", err)
	}
	cal := &Calendar{Seasons: res.Seasons}
	for _, m := range res.Months {
		cal.Months = append(cal.Months, Month{Name: m.Name, Days: m.Days, SortOrder: m.SortOrder})
	}
	wantFor := map[int]string{1: "Budding", 2: "Budding", 3: "Budding",
		4: "Zenith", 5: "Zenith", 6: "Zenith", 7: "Waning", 8: "Waning"}
	for month := 1; month <= 8; month++ {
		for _, day := range []int{1, 23, 45} {
			s := cal.SeasonForDate(month, day)
			if s == nil {
				t.Errorf("month %d day %d belongs to no season", month, day)
				continue
			}
			if s.Name != wantFor[month] {
				t.Errorf("month %d day %d is in %q, want %q", month, day, s.Name, wantFor[month])
			}
		}
	}
}

// TestParseCalendaria_SeasonMonthBaseIsDetected pins the base discrimination.
//
// The two real Calendaria exports in cordinator/references/calendars DISAGREE
// on whether monthStart addresses the first month as 0 or as 1, so the base
// cannot be a constant. The fixtures below reproduce each file's season and
// month geometry exactly (names shortened; the fields that decide are the month
// count and the monthStart/monthEnd sets). If either reading were hard-coded,
// one of these two cases would come out shifted by a whole month.
func TestParseCalendaria_SeasonMonthBaseIsDetected(t *testing.T) {
	t.Run("zero-based (forbidden-lands shape)", func(t *testing.T) {
		// 8 months of 45/46 days; seasons at 0-1, 2-3, 4-5, 6-7 — a 1-based
		// reading has no month 0, so this file is 0-based.
		raw := calendariaSeasonFixture(
			[]int{45, 46, 46, 46, 45, 46, 45, 46},
			[]fixtureSeason{
				{"Spring", 1, iptr(0), iptr(1), 0, 45},
				{"Summer", 2, iptr(2), iptr(3), 0, 45},
				{"Autumn", 3, iptr(4), iptr(5), 0, 45},
				{"Winter", 4, iptr(6), iptr(7), 0, 45},
			})
		assertSeasonRanges(t, importedSeasonRanges(t, raw), []seasonRange{
			// dayEnd 45 is honoured verbatim, so a season closing on a 46-day
			// month closes on its day 45. That is what the payload says; see
			// calendariaSeasonRange.
			{"Spring", 1, 1, 2, 45},
			{"Summer", 3, 1, 4, 45},
			{"Autumn", 5, 1, 6, 45},
			{"Winter", 7, 1, 8, 45},
		})
	})

	t.Run("one-based (calendar-of-therin shape)", func(t *testing.T) {
		// 15 months of 24 days; seasons at 1-3, 4-6, 7-9, 10-12, 13-0, with no
		// ordinal and no day fields at all. A 0-based reading would leave the
		// FIRST month seasonless, so this file is 1-based — and the trailing
		// monthEnd 0 means "to the end of the year".
		months := make([]int, 15)
		for i := range months {
			months[i] = 24
		}
		raw := calendariaSeasonFixture(months, []fixtureSeason{
			{"Sprouting", 0, iptr(1), iptr(3), 0, 0},
			{"Crown", 0, iptr(4), iptr(6), 0, 0},
			{"Goldfall", 0, iptr(7), iptr(9), 0, 0},
			{"Stillfrost", 0, iptr(10), iptr(12), 0, 0},
			{"Greylight", 0, iptr(13), iptr(0), 0, 0},
		})
		assertSeasonRanges(t, importedSeasonRanges(t, raw), []seasonRange{
			{"Sprouting", 1, 1, 3, 24},
			{"Crown", 4, 1, 6, 24},
			{"Goldfall", 7, 1, 9, 24},
			{"Stillfrost", 10, 1, 12, 24},
			{"Greylight", 13, 1, 15, 24}, // monthEnd 0 → to the end of the year
		})
	})
}

// TestParseCalendaria_DayOfYearSeasonsUnchanged is the other half of the fix:
// a file that names no month at all must keep the day-of-year reading exactly.
// Without this, "teach the parser about monthStart" could quietly have become
// "reinterpret every season Chronicle has ever imported".
func TestParseCalendaria_DayOfYearSeasonsUnchanged(t *testing.T) {
	// Three 30-day months, seasons spanning days 1-30, 31-60 and 61-90 of the
	// YEAR — no monthStart anywhere.
	raw := calendariaSeasonFixture([]int{30, 30, 30}, []fixtureSeason{
		{"First", 1, nil, nil, 1, 30},
		{"Second", 2, nil, nil, 31, 60},
		{"Third", 3, nil, nil, 61, 90},
	})
	assertSeasonRanges(t, importedSeasonRanges(t, raw), []seasonRange{
		{"First", 1, 1, 1, 30},
		{"Second", 2, 1, 2, 30},
		{"Third", 3, 1, 3, 30},
	})
}

// TestCalendariaSeasonMonthBase covers the detector directly, including the
// case the discriminator is deliberately blind to.
func TestCalendariaSeasonMonthBase(t *testing.T) {
	tests := []struct {
		name         string
		seasons      []calSeason
		wantDeclared bool
		wantBase     int
	}{
		{"no months named at all", []calSeason{{DayStart: 1, DayEnd: 30}}, false, 0},
		{"smallest monthStart is 0 → zero-based",
			[]calSeason{{MonthStart: iptr(2)}, {MonthStart: iptr(0)}}, true, 0},
		{"smallest monthStart is 1 → one-based",
			[]calSeason{{MonthStart: iptr(4)}, {MonthStart: iptr(1)}}, true, 1},
		// monthEnd 0 on a 1-based file means "to the end of the year". If the
		// detector consulted monthEnd it would read that as evidence of
		// 0-basing and shift every therin-shaped season by a month.
		{"monthEnd 0 does not make a one-based file zero-based",
			[]calSeason{{MonthStart: iptr(1), MonthEnd: iptr(3)}, {MonthStart: iptr(13), MonthEnd: iptr(0)}}, true, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			declared, base := calendariaSeasonMonthBase(tt.seasons)
			if declared != tt.wantDeclared || base != tt.wantBase {
				t.Errorf("got (declared=%v, base=%d), want (declared=%v, base=%d)",
					declared, base, tt.wantDeclared, tt.wantBase)
			}
		})
	}
}

// --- fixture construction ----------------------------------------------------

func iptr(v int) *int { return &v }

type fixtureSeason struct {
	Name             string
	Ordinal          int
	MonthStart       *int
	MonthEnd         *int
	DayStart, DayEnd int
}

// calendariaSeasonFixture emits a minimal Calendaria payload with the given
// month lengths and seasons. It is built through encoding/json rather than
// string concatenation so an absent monthStart is genuinely absent from the
// bytes — which is the thing the detector keys on.
func calendariaSeasonFixture(monthDays []int, seasons []fixtureSeason) []byte {
	months := map[string]any{}
	for i, d := range monthDays {
		key := string(rune('a'+i%26)) + string(rune('a'+i/26)) + "-month"
		months[key] = map[string]any{
			"name": key, "days": d, "ordinal": i + 1,
		}
	}
	out := map[string]any{
		"name":   "Season Fixture",
		"months": months,
		"days": map[string]any{"values": map[string]any{
			"d1": map[string]any{"name": "One", "ordinal": 1},
			"d2": map[string]any{"name": "Two", "ordinal": 2},
		}},
		"seasons": buildFixtureSeasons(seasons),
	}
	b, err := json.Marshal(out)
	if err != nil {
		panic(err)
	}
	return b
}

func buildFixtureSeasons(seasons []fixtureSeason) map[string]any {
	m := map[string]any{}
	for i, s := range seasons {
		row := map[string]any{"name": s.Name}
		if s.Ordinal != 0 {
			row["ordinal"] = s.Ordinal
		}
		if s.MonthStart != nil {
			row["monthStart"] = *s.MonthStart
		}
		if s.MonthEnd != nil {
			row["monthEnd"] = *s.MonthEnd
		}
		if s.DayStart != 0 {
			row["dayStart"] = s.DayStart
		}
		if s.DayEnd != 0 {
			row["dayEnd"] = s.DayEnd
		}
		// The key must not rank the seasons — the comparator under test is what
		// must, so the keys are authored in REVERSE of the expected order.
		m[string(rune('z'-i))+"-season"] = row
	}
	return m
}
