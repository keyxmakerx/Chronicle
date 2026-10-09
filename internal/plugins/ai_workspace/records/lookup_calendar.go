package records

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

// calDate is a date on the campaign calendar; Before orders them.
type calDate struct{ Y, M, D int }

func (a calDate) Before(b calDate) bool {
	if a.Y != b.Y {
		return a.Y < b.Y
	}
	if a.M != b.M {
		return a.M < b.M
	}
	return a.D < b.D
}

// parseDateText reads "Deepwinter 1 1492", "1 Deepwinter 1492" or
// "1492-3-1": the last number is the year, another number the day, and the
// rest the month's name (or its number).
func parseDateText(cal *calendar.Calendar, s string) (calDate, error) {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", " "))
	if parts := strings.Split(s, "-"); len(parts) == 3 {
		y, e1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		m, e2 := strconv.Atoi(strings.TrimSpace(parts[1]))
		d, e3 := strconv.Atoi(strings.TrimSpace(parts[2]))
		if e1 == nil && e2 == nil && e3 == nil {
			return checkDate(cal, calDate{y, m, d}, s)
		}
	}
	var nums []int
	var words []string
	for _, f := range strings.Fields(s) {
		if n, err := strconv.Atoi(f); err == nil {
			nums = append(nums, n)
		} else {
			words = append(words, f)
		}
	}
	if len(nums) != 2 || len(words) == 0 {
		return calDate{}, badRequestf("%s is not a date like %q", quote(s), exampleDate(cal))
	}
	name := strings.Join(words, " ")
	for i, mo := range cal.Months {
		if sameName(mo.Name, name) {
			return checkDate(cal, calDate{nums[1], i + 1, nums[0]}, s)
		}
	}
	return calDate{}, badRequestf("%s is not a month of %s", quote(name), cal.Name)
}

func checkDate(cal *calendar.Calendar, d calDate, s string) (calDate, error) {
	if d.M < 1 || d.M > len(cal.Months) || d.D < 1 || d.D > cal.MonthDays(d.M-1, d.Y) {
		return calDate{}, badRequestf("%s is not a day on %s", quote(s), cal.Name)
	}
	return d, nil
}

func exampleDate(cal *calendar.Calendar) string {
	if len(cal.Months) == 0 {
		return "1492-1-1"
	}
	return cal.Months[0].Name + " 1 1492"
}

// dateRange reads `from`/`to`, or `year` with an optional `month`.
func dateRange(cal *calendar.Calendar, r Record) (from, to calDate, err error) {
	if f := r.Str("from"); f != "" {
		if from, err = parseDateText(cal, f); err != nil {
			return
		}
		to = from
		if t := r.Str("to"); t != "" {
			if to, err = parseDateText(cal, t); err != nil {
				return
			}
		}
		if to.Before(from) {
			err = badRequestf("`to` is before `from`")
		}
		return
	}
	y, ok, err := r.Int("year")
	if err != nil {
		return from, to, err
	}
	if !ok {
		return from, to, badRequestf("it needs `from` and `to` (like %q), or `year` and `month`", exampleDate(cal))
	}
	if len(cal.Months) == 0 {
		return from, to, badRequestf("%s has no months", cal.Name)
	}
	from, to = calDate{y, 1, 1}, calDate{y, len(cal.Months), cal.MonthDays(len(cal.Months)-1, y)}
	if ms := r.Str("month"); ms != "" {
		m, _, merr := r.Int("month")
		if merr != nil {
			m = 0
			for i, mo := range cal.Months {
				if sameName(mo.Name, ms) {
					m = i + 1
				}
			}
		}
		if m < 1 || m > len(cal.Months) {
			return from, to, badRequestf("month: %s is not a month of %s", quote(ms), cal.Name)
		}
		from, to = calDate{y, m, 1}, calDate{y, m, cal.MonthDays(m-1, y)}
	}
	return from, to, nil
}

// months lists each (year, month) the range touches, capped.
func months(cal *calendar.Calendar, from, to calDate) ([]calDate, error) {
	var out []calDate
	for cur := (calDate{from.Y, from.M, 1}); !(calDate{to.Y, to.M, 1}).Before(cur); {
		if len(out) == maxCalMonths {
			return nil, badRequestf("ask for %d months or fewer at a time", maxCalMonths)
		}
		out = append(out, cur)
		cur.M++
		if cur.M > len(cal.Months) {
			cur.M, cur.Y = 1, cur.Y+1
		}
	}
	return out, nil
}

func rangeLabel(cal *calendar.Calendar, from, to calDate) string {
	if from.Y == to.Y && from.M == to.M && from.D == 1 && to.D == cal.MonthDays(to.M-1, to.Y) {
		return fmt.Sprintf("%s %d", cal.Months[from.M-1].Name, from.Y)
	}
	if from == to {
		return dateText(cal, from)
	}
	return dateText(cal, from) + " to " + dateText(cal, to)
}

// dateText is "Deepwinter 21 1492", the form a lookup accepts back.
func dateText(cal *calendar.Calendar, d calDate) string {
	if d.M >= 1 && d.M <= len(cal.Months) {
		return fmt.Sprintf("%s %d %d", cal.Months[d.M-1].Name, d.D, d.Y)
	}
	return fmt.Sprintf("%d-%d-%d", d.Y, d.M, d.D)
}

func (run *lookupRun) calendarOrErr() (*calendar.Calendar, error) {
	if run.l.Cal == nil {
		return nil, badRequestf("the calendar can't be looked up on this server")
	}
	return defaultCalendar(run.ctx, run.l.Cal, run.campaignID, run.a)
}

func (run *lookupRun) events(r Record) (LookupAnswer, error) {
	ans := LookupAnswer{Chip: "Calendar events"}
	cal, err := run.calendarOrErr()
	if err != nil {
		return ans, err
	}
	from, to, err := dateRange(cal, r)
	if err != nil {
		return ans, err
	}
	ms, err := months(cal, from, to)
	if err != nil {
		return ans, err
	}
	label := rangeLabel(cal, from, to)
	seen := map[string]bool{}
	var lines []string
	for _, m := range ms {
		evs, err := run.l.Cal.ListEventsForMonth(run.ctx, cal.ID, run.campaignID, m.Y, m.M, run.a.Viewer())
		if err != nil {
			return ans, err
		}
		for _, e := range evs {
			start := calDate{e.Year, e.Month, e.Day}
			end := start
			if e.EndYear != nil && e.EndMonth != nil && e.EndDay != nil {
				end = calDate{*e.EndYear, *e.EndMonth, *e.EndDay}
			}
			if seen[e.ID] || to.Before(start) || end.Before(from) {
				continue
			}
			seen[e.ID] = true
			when := dateText(cal, start)
			if end != start {
				when += " to " + dateText(cal, end)
			}
			who := "everyone"
			if e.Visibility == "dm_only" {
				who = "Directors only"
			}
			lines = append(lines, fmt.Sprintf("- %s: %s, %s", e.Name, when, who))
		}
	}
	ans.Chip = fmt.Sprintf("Calendar events · %s · %d", label, len(lines))
	var b strings.Builder
	fmt.Fprintf(&b, "## Events, %s (%d)\n\n", label, len(lines))
	if len(lines) == 0 {
		b.WriteString("None.\n")
	}
	writeLines(&b, lines)
	ans.Text = b.String()
	return ans, nil
}

func (run *lookupRun) weather(r Record) (LookupAnswer, error) {
	ans := LookupAnswer{Chip: "Weather"}
	cal, err := run.calendarOrErr()
	if err != nil {
		return ans, err
	}
	from, to, err := dateRange(cal, r)
	if err != nil {
		return ans, err
	}
	ms, err := months(cal, from, to)
	if err != nil {
		return ans, err
	}
	label := rangeLabel(cal, from, to)
	var lines []string
	for _, m := range ms {
		days, err := run.l.Cal.ListDayWeather(run.ctx, cal.ID, run.campaignID, m.Y, m.M, run.a.Viewer())
		if err != nil {
			return ans, err
		}
		for _, d := range days {
			at := calDate{d.Year, d.Month, d.Day}
			if at.Before(from) || to.Before(at) {
				continue
			}
			lines = append(lines, "- "+dateText(cal, at)+": "+weatherText(d))
		}
	}
	ans.Chip = fmt.Sprintf("Weather · %s · %d days", label, len(lines))
	var b strings.Builder
	fmt.Fprintf(&b, "## Weather, %s (%d days set)\n\n", label, len(lines))
	if len(lines) == 0 {
		b.WriteString("No day in this range has weather yet.\n")
	}
	writeLines(&b, lines)
	ans.Text = b.String()
	return ans, nil
}

func weatherText(d calendar.DayWeather) string {
	var parts []string
	if d.PresetLabel != nil && *d.PresetLabel != "" {
		parts = append(parts, *d.PresetLabel)
	}
	if d.TemperatureCelsius != nil {
		parts = append(parts, strconv.FormatFloat(*d.TemperatureCelsius, 'f', -1, 64)+"°C")
	}
	if d.Precipitation != nil && d.Precipitation.Type != nil && *d.Precipitation.Type != "" {
		parts = append(parts, *d.Precipitation.Type)
	}
	if d.Wind != nil && d.Wind.SpeedKPH != nil {
		parts = append(parts, "wind "+strconv.FormatFloat(*d.Wind.SpeedKPH, 'f', -1, 64)+" kph")
	}
	if len(parts) == 0 && d.Description != nil {
		parts = append(parts, oneLine(*d.Description, 120))
	}
	if len(parts) == 0 {
		return "set, no details"
	}
	return strings.Join(parts, ", ")
}

// decodeMeta reads a relation's JSON metadata; bad JSON reads as empty.
func decodeMeta(raw json.RawMessage) map[string]any {
	m := map[string]any{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &m)
	}
	return m
}
