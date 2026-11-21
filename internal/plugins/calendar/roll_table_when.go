// roll_table_when.go decides which lines of a saved rolling table can come up
// on a given night. A line may carry day conditions ("when"); the rolling
// tables plugin validates them and the browser's Roll one reads them with the
// generator engine. The server reads the same shape here, from the facts the
// calendar already holds (moons, seasons, weekdays, months, the night's
// stored weather and the calendar's climate), so a nightly event roll obeys a
// table exactly as Roll one does.
package calendar

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"strings"
)

// errNothingFits says no line of the table can come up that night. A roll
// ahead leaves such a night unrolled, as Roll one says "nothing fits here".
var errNothingFits = errors.New("no line of the table can come up that night")

// noFitError is errNothingFits with the reason, so a read can tell the
// Director why a night was left unrolled. errors.Is(err, errNothingFits)
// still matches it.
type noFitError struct{ why string }

func (e *noFitError) Error() string        { return errNothingFits.Error() }
func (e *noFitError) Is(target error) bool { return target == errNothingFits }

// nightFacts is what a night's conditions are tested against.
type nightFacts struct {
	cal     *Calendar
	d       DayDate
	abs     int
	weather *DayWeather
	// weatherRead is whether the day's weather was looked up successfully, so
	// "no weather" can be told from "the lookup failed".
	weatherRead bool
	climate     string
}

// nightFactsFor gathers a night's facts. Weather and climate are read only
// when the table has a line that names them, since they cost a query. A fact
// that cannot be read is simply absent, which makes a condition on it false
// (the engine's own answer when a day has no weather).
func (s *calendarService) nightFactsFor(ctx context.Context, cal *Calendar, d DayDate, table *RollTable) *nightFacts {
	f := &nightFacts{cal: cal, d: d, abs: cal.absDayIndex(d.Year, d.Month, d.Day)}
	needWeather, needClimate := false, false
	for _, e := range table.Entries {
		w, ok := parseWhen(e.When)
		if !ok {
			continue
		}
		needWeather = needWeather || w.mentions("weather")
		needClimate = needClimate || w.mentions("climate")
	}
	if s.weatherRepo == nil {
		return f
	}
	if needWeather {
		if days, err := s.weatherRepo.ListDays(ctx, cal.ID, d.Year, d.Month); err == nil {
			f.weatherRead = true
			for i := range days {
				if days[i].Day == d.Day {
					f.weather = &days[i]
				}
			}
		}
	}
	if needClimate {
		f.climate = DefaultWeatherClimate
		if got, err := s.weatherRepo.GetSettings(ctx, cal.ID); err == nil && got != nil && validClimate(got.Climate) {
			f.climate = got.Climate
		}
	}
	return f
}

// eligibleEntries are the lines whose conditions hold on the night.
func eligibleEntries(table *RollTable, f *nightFacts) []RollTableEntry {
	out := make([]RollTableEntry, 0, len(table.Entries))
	for _, e := range table.Entries {
		if f.holds(e.When) {
			out = append(out, e)
		}
	}
	return out
}

// unmetOrder fixes the order the reasons are told in.
var unmetOrder = []string{"season", "month", "weekday", "moon", "weather", "climate", "rest", "festival", "not"}

// whyNothingFits says, in words, which conditions kept every line of the
// table from coming up that night: the union of the conditions each line
// failed, each with the night's own fact where one helps ("it is autumn").
func whyNothingFits(table *RollTable, f *nightFacts) string {
	failed := map[string]bool{}
	for _, e := range table.Entries {
		w, ok := parseWhen(e.When)
		if !ok {
			failed["unreadable"] = true
			continue
		}
		for k, v := range w {
			if !f.condition(k, v) {
				failed[k] = true
			}
		}
	}
	var parts []string
	for _, k := range unmetOrder {
		if !failed[k] {
			continue
		}
		parts = append(parts, f.unmetPhrase(k))
		delete(failed, k)
	}
	if len(failed) > 0 {
		parts = append(parts, "a condition that can’t be read")
	}
	if len(parts) == 0 {
		return "No line can come up that night."
	}
	return "The lines wait on " + joinPhrases(parts) + "."
}

func joinPhrases(p []string) string {
	switch len(p) {
	case 1:
		return p[0]
	case 2:
		return p[0] + " and " + p[1]
	}
	return strings.Join(p[:len(p)-1], ", ") + " and " + p[len(p)-1]
}

// unmetPhrase names one failed condition, with the night's fact beside it.
func (f *nightFacts) unmetPhrase(key string) string {
	switch key {
	case "season":
		name, kind := f.seasonOf()
		if name == "" {
			name = kind
		}
		return "the season (it is " + strings.ToLower(name) + ")"
	case "month":
		if f.d.Month >= 1 && f.d.Month <= len(f.cal.Months) {
			return "the month (it is " + f.cal.Months[f.d.Month-1].Name + ")"
		}
		return "the month"
	case "weekday":
		if idx := f.cal.WeekdayIndex(f.d.Year, f.d.Month, f.d.Day); idx >= 0 && idx < len(f.cal.Weekdays) {
			return "the weekday (it is " + f.cal.Weekdays[idx].Name + ")"
		}
		return "the weekday"
	case "moon":
		return "the moon"
	case "weather":
		if !f.weatherRead {
			return "the weather (it couldn’t be read)"
		}
		if f.weather == nil {
			return "the weather (none is set for that day)"
		}
		return "the weather"
	case "climate":
		return "the climate"
	case "rest":
		return "rest days"
	case "festival":
		return "festival days"
	}
	return "a night the lines rule out"
}

// whenSet is a line's conditions as stored: a map from condition to its raw
// value, decoded as each is read.
type whenSet map[string]json.RawMessage

func parseWhen(raw json.RawMessage) (whenSet, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, true
	}
	var w whenSet
	if json.Unmarshal(raw, &w) != nil {
		return nil, false
	}
	return w, true
}

func (w whenSet) mentions(key string) bool {
	if _, ok := w[key]; ok {
		return true
	}
	if raw, ok := w["not"]; ok {
		if inner, ok := parseWhen(raw); ok {
			return inner.mentions(key)
		}
	}
	return false
}

// holds reports whether every condition holds. A line with no conditions
// comes up any night. A condition this code does not know, or one that does
// not decode, never holds, so a line is never picked on a night it was meant
// to exclude.
func (f *nightFacts) holds(raw json.RawMessage) bool {
	w, ok := parseWhen(raw)
	if !ok {
		return false
	}
	return f.holdsSet(w)
}

func (f *nightFacts) holdsSet(w whenSet) bool {
	for k, v := range w {
		if !f.condition(k, v) {
			return false
		}
	}
	return true
}

func (f *nightFacts) condition(key string, v json.RawMessage) bool {
	switch key {
	case "season":
		return f.season(v)
	case "month":
		return f.month(v)
	case "weekday":
		return f.weekday(v)
	case "rest":
		var want bool
		return json.Unmarshal(v, &want) == nil && want == f.isRestDay()
	case "festival":
		var want bool
		return json.Unmarshal(v, &want) == nil && want == f.cal.monthIsIntercalary(f.d.Month)
	case "moon":
		return f.moon(v)
	case "weather":
		return f.weatherIn(v)
	case "climate":
		var ids []string
		return json.Unmarshal(v, &ids) == nil && f.climate != "" && containsFold(ids, f.climate)
	case "chance":
		// A line's own odds are scaled by chance when the table is drawn
		// from; being eligible does not depend on it.
		var c float64
		return json.Unmarshal(v, &c) == nil && c >= 0 && c <= 1
	case "not":
		inner, ok := parseWhen(v)
		return ok && !f.holdsSet(inner)
	}
	return false
}

func containsFold(list []string, s string) bool {
	for _, x := range list {
		if strings.EqualFold(strings.TrimSpace(x), s) {
			return true
		}
	}
	return false
}

// names reads a condition list of names and whole numbers.
func names(v json.RawMessage) ([]any, bool) {
	var items []any
	if json.Unmarshal(v, &items) != nil {
		return nil, false
	}
	return items, true
}

func (f *nightFacts) month(v json.RawMessage) bool {
	items, ok := names(v)
	if !ok {
		return false
	}
	name := ""
	if f.d.Month >= 1 && f.d.Month <= len(f.cal.Months) {
		name = f.cal.Months[f.d.Month-1].Name
	}
	for _, it := range items {
		switch x := it.(type) {
		case float64:
			if int(x) == f.d.Month {
				return true
			}
		case string:
			if strings.EqualFold(strings.TrimSpace(x), name) {
				return true
			}
		}
	}
	return false
}

func (f *nightFacts) weekday(v json.RawMessage) bool {
	items, ok := names(v)
	if !ok {
		return false
	}
	idx := f.cal.WeekdayIndex(f.d.Year, f.d.Month, f.d.Day)
	if idx < 0 || idx >= len(f.cal.Weekdays) {
		return false
	}
	name := f.cal.Weekdays[idx].Name
	for _, it := range items {
		switch x := it.(type) {
		case float64:
			if int(x)-1 == idx {
				return true
			}
		case string:
			if strings.EqualFold(strings.TrimSpace(x), name) {
				return true
			}
		}
	}
	return false
}

func (f *nightFacts) isRestDay() bool {
	idx := f.cal.WeekdayIndex(f.d.Year, f.d.Month, f.d.Day)
	return idx >= 0 && idx < len(f.cal.Weekdays) && f.cal.Weekdays[idx].IsRestDay
}

func (f *nightFacts) weatherIn(v json.RawMessage) bool {
	var ids []string
	if json.Unmarshal(v, &ids) != nil || f.weather == nil {
		return false
	}
	for _, id := range ids {
		if f.weather.PresetID != nil && *f.weather.PresetID == id {
			return true
		}
		if f.weather.Icon != nil && *f.weather.Icon == id {
			return true
		}
	}
	return false
}

// --- seasons ---

var seasonWords = []struct {
	kind string
	re   *regexp.Regexp
}{
	{"wet", regexp.MustCompile(`(?i)\b(the )?rains?\b|\bwet\b|monsoon|flood`)},
	{"dry", regexp.MustCompile(`(?i)\bdry\b|drought|\bdust`)},
	{"winter", regexp.MustCompile(`(?i)winter|frost|rime|snow|\bice|cold|long ?night|deep ?(cold|dark)|hoar|\bdark`)},
	{"spring", regexp.MustCompile(`(?i)spring|thaw|bud|seed|green|waking|\bsow|lamb|blossom|bloom`)},
	{"summer", regexp.MustCompile(`(?i)summer|high ?sun|zenith|bright|heat|\bsun\b|sunreign|midyear|\bburn`)},
	{"autumn", regexp.MustCompile(`(?i)autumn|\bfall\b|harvest|fad(e|ing)|wan(e|ing)|turning|leaf|reap|withering`)},
}

// seasonKind types a season by the words in its name, in the engine's order.
func seasonKind(name string) string {
	for _, w := range seasonWords {
		if w.re.MatchString(name) {
			return w.kind
		}
	}
	return ""
}

var yearQuarters = []string{"winter", "spring", "summer", "autumn"}

// seasonOf is the night's season name and its canonical kind (winter,
// spring, summer or autumn). A calendar season whose name types as wet or
// dry reads as summer or winter, as in the engine. A day no season covers
// reads its kind from how far through the year it is.
func (f *nightFacts) seasonOf() (name, kind string) {
	if s := f.cal.SeasonForDate(f.d.Month, f.d.Day); s != nil {
		k := seasonKind(s.Name)
		switch k {
		case "wet":
			return s.Name, "summer"
		case "dry":
			return s.Name, "winter"
		case "":
		default:
			return s.Name, k
		}
		return s.Name, f.kindByYearPosition()
	}
	return "", f.kindByYearPosition()
}

func (f *nightFacts) kindByYearPosition() string {
	length := f.cal.YearLengthForYear(f.d.Year)
	if length <= 0 {
		return "winter"
	}
	doy := f.abs - f.cal.absDayIndex(f.d.Year, 1, 1)
	shift := math.Round(float64(length) * 0.04)
	th := (float64(doy) - shift) / float64(length)
	th -= math.Floor(th)
	return yearQuarters[int(math.Round(th*4))%4]
}

func (f *nightFacts) season(v json.RawMessage) bool {
	var want []string
	if json.Unmarshal(v, &want) != nil {
		return false
	}
	name, kind := f.seasonOf()
	nameKind := seasonKind(name)
	for _, x := range want {
		x = strings.ToLower(strings.TrimSpace(x))
		if x == kind || (name != "" && x == strings.ToLower(name)) {
			return true
		}
		if (x == "wet" || x == "dry") && nameKind == x {
			return true
		}
	}
	return false
}

// --- moons ---

// moon holds when some moon the condition names is at the phase: any moon
// for "any" or no name, else the one with that name.
func (f *nightFacts) moon(v json.RawMessage) bool {
	var c struct {
		Name   string   `json:"name"`
		Phase  string   `json:"phase"`
		Within *float64 `json:"within"`
	}
	if json.Unmarshal(v, &c) != nil {
		return false
	}
	within := 1.0
	if c.Within != nil {
		within = *c.Within
	}
	for i := range f.cal.Moons {
		m := &f.cal.Moons[i]
		if c.Name != "" && !strings.EqualFold(c.Name, "any") && !strings.EqualFold(c.Name, m.Name) {
			continue
		}
		if c.Phase == "" || moonPhaseMatches(m, f.abs, c.Phase, within) {
			return true
		}
	}
	return false
}

// moonTurningPoint is the phase a named turning point sits at, and the
// manual state that stands for it.
var moonTurningPoint = map[string]struct {
	at    float64
	state string
}{
	"full": {0.5, MoonFull}, "new": {0, MoonDark},
	"first-quarter": {0.25, MoonHalf}, "last-quarter": {0.75, MoonHalf},
}

// moonPhaseMatches is the engine's phaseMatches: a turning point within
// within days (plus half a day), or a stretch of the cycle. A moon set by
// hand matches a turning point on the days it holds that state, and a
// stretch by the phase its state draws as. A moon with no cycle, set by
// cycle, never matches.
func moonPhaseMatches(m *Moon, abs int, phase string, within float64) bool {
	if tp, ok := moonTurningPoint[phase]; ok {
		if m.Manual() {
			return m.StateOn(abs) == tp.state
		}
		if m.CycleDays <= 0 {
			return false
		}
		d := math.Mod(tp.at-m.MoonPhase(abs)+0.5, 1)
		if d < 0 {
			d++
		}
		return math.Abs((d-0.5)*m.CycleDays) <= within+0.5
	}
	if !m.Manual() && m.CycleDays <= 0 {
		return false
	}
	p := m.MoonPhase(abs)
	switch phase {
	case "waxing":
		return p > 0.03 && p < 0.47
	case "waning":
		return p > 0.53 && p < 0.97
	case "crescent":
		return (p > 0.03 && p < 0.22) || (p > 0.78 && p < 0.97)
	case "gibbous":
		return (p > 0.28 && p < 0.47) || (p > 0.53 && p < 0.72)
	}
	return false
}
