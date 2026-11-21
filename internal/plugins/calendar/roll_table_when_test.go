package calendar

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// whenCal is twelve 30-day months, a seven-day week with a rest day, one
// cycling moon, one moon set by hand and a winter that wraps the year end.
func whenCal() *Calendar {
	var months []Month
	for i := 1; i <= 12; i++ {
		months = append(months, Month{Name: fmt.Sprintf("M%d", i), Days: 30, IsIntercalary: i == 12})
	}
	days := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	var wds []Weekday
	for i, d := range days {
		wds = append(wds, Weekday{Name: d, SortOrder: i, IsRestDay: d == "Sun"})
	}
	return &Calendar{
		ID: "c", Mode: ModeFantasy, Months: months, Weekdays: wds,
		Moons: []Moon{
			{ID: 1, Name: "Luna", CycleDays: 30, PhaseOffset: 4, PhaseMode: MoonCycle},
		},
		Seasons: []Season{
			{Name: "Deep Winter", StartMonth: 11, StartDay: 1, EndMonth: 2, EndDay: 30},
			{Name: "Spring Thaw", StartMonth: 3, StartDay: 1, EndMonth: 5, EndDay: 30},
			{Name: "The Rains", StartMonth: 6, StartDay: 1, EndMonth: 8, EndDay: 30},
			{Name: "Harvest", StartMonth: 9, StartDay: 1, EndMonth: 10, EndDay: 30},
		},
	}
}

func factsOn(cal *Calendar, y, m, d int) *nightFacts {
	return &nightFacts{cal: cal, d: DayDate{Year: y, Month: m, Day: d}, abs: cal.absDayIndex(y, m, d)}
}

func raw(s string) json.RawMessage { return json.RawMessage(s) }

// Each condition the rolling tables service accepts is answered from the
// calendar's own facts; one it does not know never holds.
func TestNightFactsHold(t *testing.T) {
	cal := whenCal()
	// A date that is the rest day (Sun), in M3 (spring), and one that is not.
	var sunday DayDate
	for d := 1; d <= 7; d++ {
		if cal.WeekdayIndex(2, 3, d) == 6 {
			sunday = DayDate{Year: 2, Month: 3, Day: d}
		}
	}
	on := factsOn(cal, sunday.Year, sunday.Month, sunday.Day)
	other := factsOn(cal, sunday.Year, sunday.Month, sunday.Day+1)
	tests := []struct {
		name string
		f    *nightFacts
		when string
		want bool
	}{
		{"no conditions", on, ``, true},
		{"null", on, `null`, true},
		{"empty set", on, `{}`, true},
		{"month by number", on, `{"month":[3]}`, true},
		{"month by name", on, `{"month":["m3"]}`, true},
		{"month elsewhere", on, `{"month":[4,"M5"]}`, false},
		{"weekday by name", on, `{"weekday":["Sun"]}`, true},
		{"weekday by number (1-based)", on, `{"weekday":[7]}`, true},
		{"weekday elsewhere", other, `{"weekday":["Sun"]}`, false},
		{"rest day", on, `{"rest":true}`, true},
		{"not a rest day", other, `{"rest":true}`, false},
		{"rest false", other, `{"rest":false}`, true},
		{"festival month", factsOn(cal, 2, 12, 5), `{"festival":true}`, true},
		{"not a festival", on, `{"festival":true}`, false},
		{"season by kind", on, `{"season":["spring"]}`, true},
		{"season by name", on, `{"season":["spring thaw"]}`, true},
		{"season wrong", on, `{"season":["winter","summer"]}`, false},
		{"wet season reads as summer", factsOn(cal, 2, 7, 1), `{"season":["summer"]}`, true},
		{"wet season by word", factsOn(cal, 2, 7, 1), `{"season":["wet"]}`, true},
		{"winter wraps the year end", factsOn(cal, 2, 1, 15), `{"season":["winter"]}`, true},
		{"harvest is autumn", factsOn(cal, 2, 9, 9), `{"season":["autumn"]}`, true},
		{"not inverts", on, `{"not":{"season":["spring"]}}`, false},
		{"not of a miss holds", on, `{"not":{"season":["winter"]}}`, true},
		{"chance does not exclude", on, `{"chance":0.1}`, true},
		{"several must all hold", on, `{"month":[3],"rest":false}`, false},
		{"weather with none known", on, `{"weather":["rain"]}`, false},
		{"climate with none known", on, `{"climate":["temperate"]}`, false},
		{"an unknown condition never holds", on, `{"tide":["high"]}`, false},
		{"not decoding never holds", on, `{"month":"three"}`, false},
		{"not a set never holds", on, `["month"]`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.f.holds(raw(tt.when)); got != tt.want {
				t.Errorf("holds(%s) = %v, want %v", tt.when, got, tt.want)
			}
		})
	}
}

func TestNightFactsWeatherAndClimate(t *testing.T) {
	cal := whenCal()
	rain, sun := "rain", "sun"
	f := factsOn(cal, 2, 3, 3)
	f.weather = &DayWeather{PresetID: &rain, Icon: &sun}
	f.climate = "temperate"
	tests := []struct {
		when string
		want bool
	}{
		{`{"weather":["rain"]}`, true},
		{`{"weather":["sun"]}`, true},
		{`{"weather":["snow"]}`, false},
		{`{"climate":["Temperate"]}`, true},
		{`{"climate":["arctic"]}`, false},
		{`{"weather":["rain"],"climate":["arctic"]}`, false},
	}
	for _, tt := range tests {
		if got := f.holds(raw(tt.when)); got != tt.want {
			t.Errorf("holds(%s) = %v, want %v", tt.when, got, tt.want)
		}
	}
}

// The moon condition uses the calendar's own phase maths: full and new never
// share a night, a full moon comes up a few nights a cycle, and a named moon
// or a phase stretch narrows it as the engine does.
func TestNightFactsMoon(t *testing.T) {
	cal := whenCal()
	count := func(when string) (n int) {
		for d := 1; d <= 30; d++ {
			if factsOn(cal, 2, 3, d).holds(raw(when)) {
				n++
			}
		}
		return n
	}
	full := `{"moon":{"name":"any","phase":"full","within":1}}`
	newm := `{"moon":{"name":"any","phase":"new","within":1}}`
	for d := 1; d <= 30; d++ {
		f := factsOn(cal, 2, 3, d)
		if f.holds(raw(full)) && f.holds(raw(newm)) {
			t.Fatalf("day %d is both near a full and a new moon", d)
		}
	}
	tests := []struct {
		name     string
		when     string
		min, max int
	}{
		{"full within a day", full, 2, 3},
		{"new within a day", newm, 2, 3},
		{"full exactly", `{"moon":{"phase":"full","within":0}}`, 1, 1},
		{"full by name", `{"moon":{"name":"luna","phase":"full"}}`, 2, 3},
		{"another moon's full", `{"moon":{"name":"Umbra","phase":"full"}}`, 0, 0},
		{"waxing", `{"moon":{"phase":"waxing"}}`, 12, 15},
		{"waning", `{"moon":{"phase":"waning"}}`, 12, 15},
		{"any moon any phase", `{"moon":{}}`, 30, 30},
		{"not full", `{"not":` + full + `}`, 27, 28},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if n := count(tt.when); n < tt.min || n > tt.max {
				t.Errorf("%d nights hold, want %d to %d", n, tt.min, tt.max)
			}
		})
	}
}

func TestNightFactsManualMoon(t *testing.T) {
	cal := whenCal()
	cal.Moons = []Moon{{ID: 2, Name: "Umbra", PhaseMode: MoonManual, Marks: []MoonMark{
		{Year: 2, Month: 3, Day: 5, State: MoonFull},
		{Year: 2, Month: 3, Day: 9, State: MoonDark},
	}}}
	cal.indexMoonMarks()
	tests := []struct {
		day  int
		when string
		want bool
	}{
		{6, `{"moon":{"phase":"full"}}`, true},
		{10, `{"moon":{"phase":"full"}}`, false},
		{10, `{"moon":{"phase":"new"}}`, true},
		{4, `{"moon":{"phase":"full"}}`, false},
	}
	for _, tt := range tests {
		if got := factsOn(cal, 2, 3, tt.day).holds(raw(tt.when)); got != tt.want {
			t.Errorf("day %d %s = %v, want %v", tt.day, tt.when, got, tt.want)
		}
	}
}

// monthTable has one line for each of the fixture's two months (both land
// the event on the 3rd).
func monthTable(vis string) *RollTable {
	return &RollTable{Name: "Omens", Visibility: vis, Entries: []RollTableEntry{
		{Name: "Crows", Brief: "In One.", Weight: 1, When: raw(`{"month":["One"]}`)},
		{Name: "Frost", Brief: "In Two.", Weight: 1, When: raw(`{"month":[2]}`)},
	}}
}

func tableRoll(t *testing.T, tb *RollTable, mutate func(*RollConfig)) (*calendarService, *fakeRollRepo) {
	t.Helper()
	cfg := &RollConfig{Source: RollSourceTable, TableID: "omens", OnTheNight: true, TellPlayers: true}
	if mutate != nil {
		mutate(cfg)
	}
	svc, rolls := rollFixture(t, cfg, nil)
	svc.SetRollTableSource(fakeTables{"omens": tb})
	return svc, rolls
}

// A night can only draw a line whose conditions hold that night.
func TestServerRollObeysWhen(t *testing.T) {
	svc, rolls := tableRoll(t, monthTable(""), nil)
	for i := 0; i < 20; i++ {
		got, err := svc.RollEventNights(context.Background(), "ev-blood", "cal", "camp", RollNightsInput{Year: 6}, owner)
		if err != nil || len(got.Nights) != 2 {
			t.Fatalf("roll the year = %+v %v", got, err)
		}
		if got.Nights[0].Result != "Crows" || got.Nights[1].Result != "Frost" {
			t.Fatalf("nights drew %q and %q, want Crows then Frost", got.Nights[0].Result, got.Nights[1].Result)
		}
	}
	if len(rolls.nights["ev-blood"]) != 2 {
		t.Errorf("stored %d nights", len(rolls.nights["ev-blood"]))
	}
}

// A night no line fits is left unrolled; asking for only such nights says so.
func TestServerRollWhenNothingFits(t *testing.T) {
	onlyTwo := &RollTable{Name: "Omens", Entries: []RollTableEntry{{Name: "Frost", Weight: 1, When: raw(`{"month":[2]}`)}}}
	svc, rolls := tableRoll(t, onlyTwo, nil)
	got, err := svc.RollEventNights(context.Background(), "ev-blood", "cal", "camp", RollNightsInput{Year: 6}, owner)
	if err != nil || len(got.Nights) != 1 || got.Nights[0].Month != 2 {
		t.Fatalf("year roll = %+v %v, want only month two rolled", got, err)
	}
	_, err = svc.RollEventNights(context.Background(), "ev-blood", "cal", "camp",
		RollNightsInput{Dates: []DayDate{{Year: 7, Month: 1, Day: 3}}}, owner)
	if apperror.SafeCode(err) != http.StatusUnprocessableEntity {
		t.Fatalf("rolling only a night nothing fits = %v, want 422", err)
	}
	if _, ok := rolls.nights["ev-blood"][DayDate{Year: 7, Month: 1, Day: 3}]; ok {
		t.Error("a night nothing fits was stored")
	}
}

// A night that arrives on its own follows the same rule: it is rolled from
// the lines that fit, or left alone.
func TestServerRollOnTheNightObeysWhen(t *testing.T) {
	svc, rolls := tableRoll(t, monthTable(""), nil)
	if _, err := svc.ListMonthRolls(context.Background(), "cal", "camp", 5, 2, player); err != nil {
		t.Fatal(err)
	}
	n, ok := rolls.nights["ev-blood"][DayDate{Year: 5, Month: 2, Day: 3}]
	if !ok || n.Entry != "Frost" {
		t.Fatalf("arrived night = %+v (stored %v), want Frost", n, ok)
	}

	onlyOne := &RollTable{Name: "Omens", Entries: []RollTableEntry{{Name: "Crows", Weight: 1, When: raw(`{"month":[1]}`)}}}
	svc, rolls = tableRoll(t, onlyOne, nil)
	if _, err := svc.ListMonthRolls(context.Background(), "cal", "camp", 5, 2, player); err != nil {
		t.Fatal(err)
	}
	if len(rolls.nights["ev-blood"]) != 0 {
		t.Errorf("a night no line fits was rolled: %+v", rolls.nights["ev-blood"])
	}
}

// An Only you table's night never reaches a player on its own, however the
// roll's own "tell players" is set, and neither does one from a table that
// can no longer be read. The Director telling the night is the one way.
func TestOnlyYouTableNightsStayFromPlayers(t *testing.T) {
	night := DayDate{Year: 5, Month: 1, Day: 3}
	stored := RollNight{Year: 5, Month: 1, Day: 3, Entry: "Crows", Source: RollNightAhead}
	yes, no := true, false
	tests := []struct {
		name  string
		table *RollTable
		told  *bool
		tell  bool
		want  bool
	}{
		{"everyone, roll tells", monthTable("everyone"), nil, true, true},
		{"no answer, roll tells", monthTable(""), nil, true, true},
		{"no answer, roll does not tell", monthTable(""), nil, false, false},
		{"only you, roll tells", monthTable(RollTableDMOnly), nil, true, false},
		{"only you, roll does not tell", monthTable(RollTableDMOnly), nil, false, false},
		{"only you, the Director tells that night", monthTable(RollTableDMOnly), &yes, true, true},
		{"everyone, the Director stops telling", monthTable("everyone"), &no, true, false},
		{"table gone", nil, nil, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, rolls := tableRoll(t, monthTable(""), func(c *RollConfig) { c.OnTheNight = false; c.TellPlayers = tt.tell })
			if tt.table == nil {
				svc.SetRollTableSource(fakeTables{})
			} else {
				svc.SetRollTableSource(fakeTables{"omens": tt.table})
			}
			n := stored
			n.Told = tt.told
			rolls.nights["ev-blood"] = map[DayDate]RollNight{night: n}
			ctx := context.Background()
			year, err := svc.GetEventRoll(ctx, "ev-blood", "cal", "camp", 5, player)
			if err != nil {
				t.Fatal(err)
			}
			month, err := svc.ListMonthRolls(ctx, "cal", "camp", 5, 1, player)
			if err != nil {
				t.Fatal(err)
			}
			if saw := len(year.Nights) == 1; saw != tt.want {
				t.Errorf("player year read saw the night = %v, want %v (%+v)", saw, tt.want, year.Nights)
			}
			if saw := len(month["ev-blood"]) == 1; saw != tt.want {
				t.Errorf("player month read saw the night = %v, want %v", saw, tt.want)
			}
			// The Director always sees it.
			dir, err := svc.GetEventRoll(ctx, "ev-blood", "cal", "camp", 5, owner)
			if err != nil || len(dir.Nights) != 1 {
				t.Errorf("Director read = %+v %v, want the night", dir, err)
			}
		})
	}
}

// A night the roll on the night leaves unrolled because no line fits is told
// (only nights within the on-the-night window of today, which here is the
// event's 3rd of the second month; the first month's 3rd is too far back)
// to the Director, with the conditions that ruled the lines out; nobody else
// is told, and a roll that does not roll on the night attempts nothing.
func TestEventRollTellsTheDirectorWhichNightsNoLineFit(t *testing.T) {
	tb := func(whens ...string) *RollTable {
		t := &RollTable{Name: "Omens"}
		for i, w := range whens {
			t.Entries = append(t.Entries, RollTableEntry{Name: string(rune('A' + i)), Weight: 1, When: raw(w)})
		}
		return t
	}
	type unfit struct {
		month int
		why   string
	}
	tests := []struct {
		name    string
		table   *RollTable
		onNight bool
		want    []unfit
	}{
		{"a line fits every night", tb(`{"month":[1,2]}`), true, nil},
		{"one month has no line", tb(`{"month":[1]}`), true, []unfit{{2, "The lines wait on the month (it is Two)."}}},
		{"a month no line names", tb(`{"month":[3]}`), true, []unfit{{2, "The lines wait on the month (it is Two)."}}},
		{"weather that is not set", tb(`{"month":[1],"weather":["rain"]}`), true, []unfit{
			{2, "The lines wait on the month (it is Two) and the weather (none is set for that day)."}}},
		{"lines that fail different conditions", tb(`{"month":[3]}`, `{"moon":{"phase":"full"}}`), true, []unfit{
			{2, "The lines wait on the month (it is Two) and the moon."}}},
		{"not rolled on the night", tb(`{"month":[3]}`), false, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, rolls := tableRoll(t, tt.table, func(c *RollConfig) { c.OnTheNight = tt.onNight })
			got, err := svc.GetEventRoll(context.Background(), "ev-blood", "cal", "camp", 5, owner)
			if err != nil {
				t.Fatal(err)
			}
			var have []unfit
			for _, u := range got.Unfit {
				if u.Year != 5 || u.Day != 3 {
					t.Errorf("unfit night %+v is not one of the event's nights", u)
				}
				have = append(have, unfit{u.Month, u.Why})
			}
			if !reflect.DeepEqual(have, tt.want) {
				t.Errorf("unfit = %+v, want %+v", have, tt.want)
			}
			// What is left unrolled stays unrolled, and a player is told nothing of it.
			for _, u := range got.Unfit {
				if _, ok := rolls.nights["ev-blood"][u.DayDate]; ok {
					t.Errorf("night %+v was both rolled and reported", u.DayDate)
				}
			}
			pv, err := svc.GetEventRoll(context.Background(), "ev-blood", "cal", "camp", 5, player)
			if err != nil || len(pv.Unfit) != 0 {
				t.Errorf("player view = %+v %v, want no unfit nights", pv, err)
			}
		})
	}
}

// "None is set" is said only when the weather was read and the day had none.
func TestUnmetWeatherPhraseTellsNoneFromUnreadable(t *testing.T) {
	tests := []struct {
		name string
		f    nightFacts
		want string
	}{
		{"lookup failed", nightFacts{}, "the weather (it couldn’t be read)"},
		{"read, none set", nightFacts{weatherRead: true}, "the weather (none is set for that day)"},
		{"read, a day that does not match", nightFacts{weatherRead: true, weather: &DayWeather{}}, "the weather"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.f.unmetPhrase("weather"); got != tt.want {
				t.Errorf("phrase = %q, want %q", got, tt.want)
			}
		})
	}
}
