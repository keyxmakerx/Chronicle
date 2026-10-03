package calendar

import (
	"fmt"
	"strings"
	"testing"
)

// foundryPayload is a complete module payload; tests splice in the parts they
// vary so each case reads as the one thing it changes.
func foundryPayload(months, seasons, moons, eras string) []byte {
	return []byte(`{"schema_version":1,"source":"calendaria","source_id":"x","name":"Harptos",
"description":"A calendar for the Realms","year_zero":0,"current_year":1492,"current_month":3,
"current_day":7,"current_hour":13,"current_minute":45,"seconds_per_round":6,"allow_negative_years":false,
"months":` + months + `,
"weekdays":[{"name":"One","abbreviation":"1","rest_day":false,"ordinal":1},{"name":"Two","abbreviation":"2","rest_day":true,"ordinal":2}],
"seasons":` + seasons + `,"moons":` + moons + `,"eras":` + eras + `}`)
}

const foundryMonths = `[
{"name":"Hammer","abbreviation":"Ham","days":30,"intercalary":false,"ordinal":1,"leap_extra_days":null},
{"name":"Alturiak","abbreviation":"Alt","days":28,"intercalary":false,"ordinal":2,"leap_extra_days":29},
{"name":"Midwinter","abbreviation":"Mid","days":1,"intercalary":true,"ordinal":3,"leap_extra_days":null},
{"name":"Ches","abbreviation":"Che","days":30,"intercalary":false,"ordinal":4,"leap_extra_days":30}]`

func TestParseFoundryImport_Structure(t *testing.T) {
	ir, err := ParseFoundryImport(foundryPayload(foundryMonths, `[]`, `[]`, `[]`))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		got  any
		want any
	}{
		{"format", ir.Format, FormatCalendaria},
		{"calendar name", ir.CalendarName, "Harptos"},
		{"month count", len(ir.Months), 4},
		{"hours per day", ir.Settings.HoursPerDay, 24},
		{"minutes per hour", ir.Settings.MinutesPerHour, 60},
		{"seconds per minute", ir.Settings.SecondsPerMinute, 60},
		{"current year setting", ir.Settings.CurrentYear, 1492},
		{"today year", ir.Today.Year, 1492},
		{"today month", *ir.Today.Month, 3},
		{"today day", *ir.Today.Day, 7},
		{"today hour", ir.Today.Hour, 13},
		{"today minute", ir.Today.Minute, 45},
		{"description", *ir.Settings.Description, "A calendar for the Realms"},
		{"weekday count", len(ir.Weekdays), 2},
		{"weekday 0 rest", ir.Weekdays[0].IsRestDay, false},
		{"weekday 1 name", ir.Weekdays[1].Name, "Two"},
		{"weekday 1 rest", ir.Weekdays[1].IsRestDay, true},
		{"weekday 1 order", ir.Weekdays[1].SortOrder, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("got %v, want %v", tc.got, tc.want)
			}
		})
	}

	// Calendaria's leapDays is the month's TOTAL length in a leap year, so the
	// extra is the difference; equal to days means no extra.
	months := []struct {
		name        string
		days, extra int
		intercalary bool
	}{
		{"Hammer", 30, 0, false},
		{"Alturiak", 28, 1, false},
		{"Midwinter", 1, 0, true},
		{"Ches", 30, 0, false},
	}
	for i, want := range months {
		t.Run("month "+want.name, func(t *testing.T) {
			m := ir.Months[i]
			if m.Name != want.name || m.Days != want.days || m.SortOrder != i ||
				m.LeapYearDays != want.extra || m.IsIntercalary != want.intercalary {
				t.Errorf("month %d = %+v, want %+v at order %d", i, m, want, i)
			}
		})
	}
}

func TestParseFoundryImport_Seasons(t *testing.T) {
	tests := []struct {
		name    string
		seasons string
		want    []Season // Name, Start/End Month/Day, Color
	}{
		{
			// Calendaria addresses months from 0; the first month is 0.
			name: "0-based month range",
			seasons: `[{"name":"Winter","color":"#aabbcc","month_start":0,"month_end":1,"day_start":null,"day_end":null},
{"name":"Spring","color":"#00ff00","month_start":3,"month_end":3,"day_start":null,"day_end":null}]`,
			want: []Season{
				{Name: "Winter", StartMonth: 1, StartDay: 1, EndMonth: 2, EndDay: 28, Color: "#aabbcc"},
				{Name: "Spring", StartMonth: 4, StartDay: 1, EndMonth: 4, EndDay: 30, Color: "#00ff00"},
			},
		},
		{
			name:    "month range with day bounds inside the first and last month",
			seasons: `[{"name":"Thaw","color":"#112233","month_start":0,"month_end":1,"day_start":10,"day_end":20}]`,
			want: []Season{
				{Name: "Thaw", StartMonth: 1, StartDay: 10, EndMonth: 2, EndDay: 20, Color: "#112233"},
			},
		},
		{
			// No month indices at all: day_start/day_end are days of the year
			// (Hammer 30 + Alturiak 28 = 58).
			name:    "day-of-year range",
			seasons: `[{"name":"Frost","color":"#abcdef","month_start":null,"month_end":null,"day_start":31,"day_end":58}]`,
			want: []Season{
				{Name: "Frost", StartMonth: 2, StartDay: 1, EndMonth: 2, EndDay: 28, Color: "#abcdef"},
			},
		},
		{
			name:    "name with a localization prefix is stripped",
			seasons: `[{"name":"CALENDARIA.Season.Autumn","color":"bad-color","month_start":null,"month_end":null,"day_start":1,"day_end":30}]`,
			want: []Season{
				{Name: "Autumn", StartMonth: 1, StartDay: 1, EndMonth: 1, EndDay: 30, Color: "#808080"},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ir, err := ParseFoundryImport(foundryPayload(foundryMonths, tc.seasons, `[]`, `[]`))
			if err != nil {
				t.Fatal(err)
			}
			if len(ir.Seasons) != len(tc.want) {
				t.Fatalf("got %d seasons, want %d: %+v", len(ir.Seasons), len(tc.want), ir.Seasons)
			}
			for i, w := range tc.want {
				if ir.Seasons[i] != w {
					t.Errorf("season %d = %+v, want %+v", i, ir.Seasons[i], w)
				}
			}
		})
	}
}

func TestParseFoundryImport_Moons(t *testing.T) {
	tests := []struct {
		name                                 string
		moon                                 string
		cycle, adjust                        float64
		refYear, refMonth, refDay            int
		wantName, wantColor                  string
		wantRandomWarning, wantZeroOffsetToo bool
	}{
		{
			name:  "reference date at year start",
			moon:  `{"name":"Selune","color":"#ffffff","cycle_length":30,"cycle_day_adjust":0,"phase_mode":"fixed","reference_date":{"year":0,"month":1,"day":1},"phases":[]}`,
			cycle: 30, refYear: 0, refMonth: 1, refDay: 1, wantName: "Selune", wantColor: "#ffffff",
		},
		{
			name:  "cycle day adjust and a later reference date",
			moon:  `{"name":"CALENDARIA.Moon.Luna","color":"#cccccc","cycle_length":29.5,"cycle_day_adjust":3.25,"phase_mode":"fixed","reference_date":{"year":1491,"month":3,"day":12},"phases":[]}`,
			cycle: 29.5, adjust: 3.25, refYear: 1491, refMonth: 3, refDay: 12, wantName: "Luna", wantColor: "#cccccc",
		},
		{
			name:  "randomized phase mode still imports",
			moon:  `{"name":"Wild","color":"#123456","cycle_length":10,"cycle_day_adjust":0,"phase_mode":"randomized","reference_date":{"year":0,"month":1,"day":1},"phases":[]}`,
			cycle: 10, refYear: 0, refMonth: 1, refDay: 1, wantName: "Wild", wantColor: "#123456", wantRandomWarning: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ir, err := ParseFoundryImport(foundryPayload(foundryMonths, `[]`, `[`+tc.moon+`]`, `[]`))
			if err != nil {
				t.Fatal(err)
			}
			if len(ir.Moons) != 1 {
				t.Fatalf("got %d moons, want 1", len(ir.Moons))
			}
			m := ir.Moons[0]
			// The importer must agree with the shared offset arithmetic, so a
			// Foundry import and a Calendaria file place the moon identically.
			wantOffset := moonPhaseOffsetFromReference(ir.Months, 0, 0, tc.cycle, tc.adjust, tc.refYear, tc.refMonth, tc.refDay)
			if m.PhaseOffset != wantOffset {
				t.Errorf("PhaseOffset = %v, want %v", m.PhaseOffset, wantOffset)
			}
			if m.Name != tc.wantName || m.CycleDays != tc.cycle || m.Color != tc.wantColor {
				t.Errorf("moon = %+v, want name %q cycle %v color %q", m, tc.wantName, tc.cycle, tc.wantColor)
			}
			if got := hasWarning(ir.Warnings, "random phases"); got != tc.wantRandomWarning {
				t.Errorf("random-phase warning present = %v, want %v (%v)", got, tc.wantRandomWarning, ir.Warnings)
			}
		})
	}

	// The adjust must actually move the offset (guards against the importer
	// dropping cycle_day_adjust while the comparison above uses the same value).
	t.Run("adjust shifts the offset", func(t *testing.T) {
		offset := func(adjust string) float64 {
			ir, err := ParseFoundryImport(foundryPayload(foundryMonths, `[]`,
				`[{"name":"M","color":"#ffffff","cycle_length":30,"cycle_day_adjust":`+adjust+`,"phase_mode":"fixed","reference_date":{"year":0,"month":1,"day":1}}]`, `[]`))
			if err != nil {
				t.Fatal(err)
			}
			return ir.Moons[0].PhaseOffset
		}
		zero, five := offset("0"), offset("5")
		want := zero - 5
		if want < 0 {
			want += 30
		}
		if five != want {
			t.Errorf("offset with adjust 5 = %v, want %v (adjust 0 gave %v)", five, want, zero)
		}
	})
}

func TestParseFoundryImport_Eras(t *testing.T) {
	tests := []struct {
		name     string
		eras     string
		wantName string
		wantDesc *string
		wantYear int
		wantEnd  *int
	}{
		{
			name:     "abbreviation becomes the description",
			eras:     `[{"name":"Dale Reckoning","abbreviation":"DR","start_year":0,"end_year":null,"format":"suffix"}]`,
			wantName: "Dale Reckoning", wantDesc: strPtr("DR"), wantYear: 0,
		},
		{
			name:     "no abbreviation leaves the description empty",
			eras:     `[{"name":"Age","abbreviation":"","start_year":100,"end_year":200,"format":"prefix"}]`,
			wantName: "Age", wantYear: 100, wantEnd: intPtrForTest(200),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ir, err := ParseFoundryImport(foundryPayload(foundryMonths, `[]`, `[]`, tc.eras))
			if err != nil {
				t.Fatal(err)
			}
			if len(ir.Eras) != 1 {
				t.Fatalf("got %d eras, want 1", len(ir.Eras))
			}
			e := ir.Eras[0]
			if e.Name != tc.wantName || e.StartYear != tc.wantYear {
				t.Errorf("era = %+v", e)
			}
			// Month/day are normalised so the era starts on a real date.
			if e.StartMonth != 1 || e.StartDay != 1 {
				t.Errorf("era start = month %d day %d, want 1/1", e.StartMonth, e.StartDay)
			}
			if (e.Description == nil) != (tc.wantDesc == nil) || (e.Description != nil && *e.Description != *tc.wantDesc) {
				t.Errorf("Description = %v, want %v", e.Description, tc.wantDesc)
			}
			if (e.EndYear == nil) != (tc.wantEnd == nil) || (e.EndYear != nil && *e.EndYear != *tc.wantEnd) {
				t.Errorf("EndYear = %v, want %v", e.EndYear, tc.wantEnd)
			}
		})
	}
}

func TestParseFoundryImport_NamesAndDescription(t *testing.T) {
	tests := []struct {
		name     string
		payload  string
		wantName string
		wantDesc *string
	}{
		{"localization key stripped from name", `{"schema_version":1,"source":"calendaria","name":"CALENDARIA.Calendar.Greyhawk","months":[{"name":"CALENDARIA.Month.Fire","days":30}]}`, "Greyhawk", nil},
		{"empty name falls back", `{"schema_version":1,"source":"calendaria","name":"","months":[{"name":"M","days":30}]}`, "Imported Calendar", nil},
		{"blank description is dropped", `{"schema_version":1,"source":"calendaria","name":"X","description":"   ","months":[{"name":"M","days":30}]}`, "X", nil},
		{"description carried", `{"schema_version":1,"source":"calendaria","name":"X","description":"  Plain words  ","months":[{"name":"M","days":30}]}`, "X", strPtr("Plain words")},
		{"full stops in text kept", `{"schema_version":1,"source":"calendaria","name":"Mr. Calendar","description":"A calm world. Rules apply.","months":[{"name":"M","days":30}]}`, "Mr. Calendar", strPtr("A calm world. Rules apply.")},
		{"dotted abbreviation-style name kept", `{"schema_version":1,"source":"calendaria","name":"D.R.","months":[{"name":"M","days":30}]}`, "D.R.", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ir, err := ParseFoundryImport([]byte(tc.payload))
			if err != nil {
				t.Fatal(err)
			}
			if ir.CalendarName != tc.wantName {
				t.Errorf("CalendarName = %q, want %q", ir.CalendarName, tc.wantName)
			}
			if (ir.Settings.Description == nil) != (tc.wantDesc == nil) ||
				(ir.Settings.Description != nil && *ir.Settings.Description != *tc.wantDesc) {
				t.Errorf("Description = %v, want %v", ir.Settings.Description, tc.wantDesc)
			}
		})
	}
	t.Run("month localization key stripped", func(t *testing.T) {
		ir, err := ParseFoundryImport([]byte(`{"schema_version":1,"source":"calendaria","name":"X","months":[{"name":"CALENDARIA.Month.Fire","days":30}]}`))
		if err != nil {
			t.Fatal(err)
		}
		if ir.Months[0].Name != "Fire" {
			t.Errorf("month name = %q, want Fire", ir.Months[0].Name)
		}
	})
}

func TestParseFoundryImport_Warnings(t *testing.T) {
	const noLeap = `[{"name":"A","days":30,"leap_extra_days":null},{"name":"B","days":30,"leap_extra_days":30}]`
	const randomMoon = `[{"name":"W","color":"#ffffff","cycle_length":10,"phase_mode":"randomized","reference_date":{"year":0,"month":1,"day":1}}]`
	tests := []struct {
		name                    string
		months, moons           string
		wantDayLength, wantLeap bool
		wantRandomMoon          bool
	}{
		{"always warns about day length", noLeap, `[]`, true, false, false},
		{"leap warning when a month gains days", foundryMonths, `[]`, true, true, false},
		{"leap length equal to days is no leap", noLeap, `[]`, true, false, false},
		{"randomized moon warns", noLeap, randomMoon, true, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ir, err := ParseFoundryImport(foundryPayload(tc.months, `[]`, tc.moons, `[]`))
			if err != nil {
				t.Fatal(err)
			}
			if got := hasWarning(ir.Warnings, "length of a day"); got != tc.wantDayLength {
				t.Errorf("day-length warning = %v, want %v (%v)", got, tc.wantDayLength, ir.Warnings)
			}
			if got := hasWarning(ir.Warnings, "leap days"); got != tc.wantLeap {
				t.Errorf("leap warning = %v, want %v (%v)", got, tc.wantLeap, ir.Warnings)
			}
			if got := hasWarning(ir.Warnings, "random phases"); got != tc.wantRandomMoon {
				t.Errorf("random-moon warning = %v, want %v (%v)", got, tc.wantRandomMoon, ir.Warnings)
			}
		})
	}
}

func TestParseFoundryImport_Errors(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		wantMsg string
	}{
		{"malformed JSON", `{"schema_version":`, "read the calendar"},
		{"not an object", `[]`, "read the calendar"},
		{"empty body", ``, "read the calendar"},
		{"newer schema version", `{"schema_version":2,"source":"calendaria","months":[{"name":"M","days":30}]}`, "version 1, not 2"},
		{"missing schema version", `{"source":"calendaria","months":[{"name":"M","days":30}]}`, "version 1, not 0"},
		{"other source", `{"schema_version":1,"source":"simple-calendar","months":[{"name":"M","days":30}]}`, "only Calendaria"},
		{"no months", `{"schema_version":1,"source":"calendaria","months":[]}`, "no months"},
		{"months absent", `{"schema_version":1,"source":"calendaria"}`, "no months"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ir, err := ParseFoundryImport([]byte(tc.payload))
			if err == nil {
				t.Fatalf("expected an error, got %+v", ir)
			}
			if ir != nil {
				t.Errorf("result must be nil on error, got %+v", ir)
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("error = %q, want it to contain %q", err, tc.wantMsg)
			}
		})
	}
}

// TestParseFoundryImport_OversizedStructureRefused: the shared structure
// limits stop a hostile payload before it reaches the database.
func TestParseFoundryImport_OversizedStructureRefused(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"schema_version":1,"source":"calendaria","months":[`)
	for i := 0; i < 5000; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"name":"M","days":1}`)
	}
	b.WriteString(`]}`)
	ir, err := ParseFoundryImport([]byte(b.String()))
	if err == nil || ir != nil {
		t.Fatalf("expected the month limit to refuse the payload, got %v", err)
	}
	if !strings.Contains(err.Error(), "maximum") {
		t.Errorf("error = %q, want it to name the maximum", err)
	}
}

func hasWarning(warnings []string, fragment string) bool {
	for _, w := range warnings {
		if strings.Contains(w, fragment) {
			return true
		}
	}
	return false
}

// TestParseFoundryImport_TimeClamped: the payload carries no day length, so a
// time from a longer Calendaria day lands on the last minute that exists, with
// a warning, instead of failing the whole import.
func TestParseFoundryImport_TimeClamped(t *testing.T) {
	tests := []struct {
		name              string
		hour, minute      int
		wantHour, wantMin int
		wantWarning       bool
	}{
		{"in range kept", 23, 59, 23, 59, false},
		{"hour past a 24-hour day", 26, 10, 23, 10, true},
		{"minute past the hour", 5, 75, 5, 59, true},
		{"negative minute", 5, -1, 5, 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			payload := fmt.Sprintf(`{"schema_version":1,"source":"calendaria","name":"X","current_month":1,"current_day":1,"current_hour":%d,"current_minute":%d,"months":[{"name":"M","days":30}]}`, tc.hour, tc.minute)
			ir, err := ParseFoundryImport([]byte(payload))
			if err != nil {
				t.Fatal(err)
			}
			if ir.Today.Hour != tc.wantHour || ir.Today.Minute != tc.wantMin {
				t.Errorf("time = %d:%02d, want %d:%02d", ir.Today.Hour, ir.Today.Minute, tc.wantHour, tc.wantMin)
			}
			found := false
			for _, w := range ir.Warnings {
				if strings.Contains(w, "time of day") {
					found = true
				}
			}
			if found != tc.wantWarning {
				t.Errorf("time warning = %v, want %v (%v)", found, tc.wantWarning, ir.Warnings)
			}
		})
	}
}

// TestParseFoundryImport_SeasonMonthsZeroBased: Calendaria's live API counts
// months from 0, so a calendar whose first season starts in its third month
// is not misread as 1-based.
func TestParseFoundryImport_SeasonMonthsZeroBased(t *testing.T) {
	months := `[{"name":"A","days":30},{"name":"B","days":30},{"name":"C","days":30},{"name":"D","days":30},{"name":"E","days":30},{"name":"F","days":30}]`
	payload := `{"schema_version":1,"source":"calendaria","name":"X","current_month":1,"current_day":1,"months":` + months +
		`,"seasons":[{"name":"Spring","month_start":2,"month_end":3},{"name":"Autumn","month_start":4,"month_end":5}]}`
	ir, err := ParseFoundryImport([]byte(payload))
	if err != nil {
		t.Fatal(err)
	}
	if got := ir.Seasons[0]; got.StartMonth != 3 || got.EndMonth != 4 {
		t.Errorf("Spring = months %d-%d, want 3-4", got.StartMonth, got.EndMonth)
	}
	if got := ir.Seasons[1]; got.StartMonth != 5 || got.EndMonth != 6 {
		t.Errorf("Autumn = months %d-%d, want 5-6", got.StartMonth, got.EndMonth)
	}
}
