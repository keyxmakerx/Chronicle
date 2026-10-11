package records

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

// repeatNone turns repetition off on `action: update`.
const repeatNone = "none"

// repeatSpec is how an event row asks to repeat. The repeat types are the
// calendar's own list and a rule names the calendar's moons, weekdays,
// months, seasons and events by name, so anything an owner adds to the
// calendar can be repeated by without a change here.
type repeatSpec struct {
	set   bool   // a repeat key was written
	typ   string // "" with set means repeat: none
	every *int
	until *[3]int
	times *int
	rule  json.RawMessage
}

// repeatTypes lists what `repeat:` accepts, for the Doc and refusals.
func repeatTypes() string {
	return strings.Join(append(append([]string(nil), calendar.RecurrenceTypes...), repeatNone), ", ")
}

// readRepeat reads the repeat keys. evs is the calendar's events, which a
// rule's `event:` conditions are matched against.
func readRepeat(cal *calendar.Calendar, r Record, evs []calendar.Event) (repeatSpec, error) {
	var s repeatSpec
	typ := strings.ToLower(r.Str("repeat"))
	others := []string{"repeat_every", "repeat_times", "repeat_on", "repeat_offset_days", "repeat_until_year", "repeat_until_month", "repeat_until_day"}
	if typ == "" {
		for _, k := range others {
			if r.Has(k) {
				return s, badRequestf("%s needs repeat too (one of %s)", k, repeatTypes())
			}
		}
		return s, nil
	}
	s.set = true
	if typ == repeatNone {
		for _, k := range others {
			if r.Has(k) {
				return s, badRequestf("%s does not go with repeat: none", k)
			}
		}
		return s, nil
	}
	if !calendar.IsSupportedRecurrenceType(typ) {
		return s, badRequestf("repeat: %q is not one of %s", typ, repeatTypes())
	}
	s.typ = typ
	n, ok, err := r.Int("repeat_every")
	if err != nil {
		return s, err
	}
	if ok {
		if n < 1 {
			return s, badRequestf("repeat_every must be 1 or more")
		}
		s.every = &n
	}
	// Weekly and biweekly have a fixed step the calendar ignores an interval
	// for; custom is "every N weeks" and means nothing without one.
	switch {
	case ok && (typ == calendar.RecurrenceWeekly || typ == calendar.RecurrenceBiWeekly):
		return s, badRequestf("repeat_every doesn't go with repeat: %s; use repeat: custom for every N weeks", typ)
	case !ok && typ == calendar.RecurrenceCustom:
		return s, badRequestf("repeat: custom needs repeat_every, the number of weeks between")
	}
	if n, ok, err := r.Int("repeat_times"); err != nil {
		return s, err
	} else if ok {
		if n < 1 {
			return s, badRequestf("repeat_times must be 1 or more")
		}
		s.times = &n
	}
	if y, m, d, ok, err := readDate(cal, r, "repeat_until_"); err != nil {
		return s, err
	} else if ok {
		s.until = &[3]int{y, m, d}
	}
	if typ != calendar.RecurrenceByRule {
		for _, k := range []string{"repeat_on", "repeat_offset_days"} {
			if r.Has(k) {
				return s, badRequestf("%s only goes with repeat: rule", k)
			}
		}
		return s, nil
	}
	rule, err := readRule(cal, r, evs)
	if err != nil {
		return s, err
	}
	// The interval would be stored but ignored by a rule; its every-Nth is
	// the rule's own.
	if s.every != nil {
		rule.Every = *s.every
		s.every = nil
	}
	raw, err := json.Marshal(rule)
	if err != nil {
		return s, err
	}
	// The calendar's own parser has the last word on the rule's shape.
	if _, err := calendar.ParseRecurrenceRule(raw); err != nil {
		return s, badRequestf("repeat_on: %s", planError(err))
	}
	s.rule = raw
	return s, nil
}

// summary is the review screen's wording, e.g. "repeats yearly".
func (s repeatSpec) summary() string {
	if !s.set {
		return ""
	}
	if s.typ == "" {
		return "stops repeating"
	}
	out := "repeats " + s.typ
	if s.typ == calendar.RecurrenceByRule {
		out = "repeats by a rule"
	}
	if s.every != nil && *s.every > 1 {
		out += fmt.Sprintf(" (every %d)", *s.every)
	}
	if s.times != nil {
		out += fmt.Sprintf(", %d times", *s.times)
	}
	return out
}

// toCreate sets the repeat fields on a new event.
func (s repeatSpec) toCreate(in *calendar.CreateEventInput) {
	if !s.set || s.typ == "" {
		return
	}
	typ := s.typ
	in.IsRecurring, in.RecurrenceType = true, &typ
	in.RecurrenceInterval = s.every
	in.RecurrenceMaxOccurrences = s.times
	if s.until != nil {
		y, m, d := s.until[0], s.until[1], s.until[2]
		in.RecurrenceEndYear, in.RecurrenceEndMonth, in.RecurrenceEndDay = &y, &m, &d
	}
	in.RecurrenceRule = s.rule
}

// toUpdate sets the repeat fields on a change. Naming a repeat replaces the
// whole repeat, so an end date or count from before doesn't linger on a new
// pattern; leaving the repeat keys out keeps the event's repeat as it is.
func (s repeatSpec) toUpdate(in *calendar.UpdateEventInput) {
	if !s.set {
		return
	}
	if s.typ == "" {
		in.IsRecurring = patch.Of(false)
		in.RecurrenceType = patch.Null[string]()
		return
	}
	in.IsRecurring, in.RecurrenceType = patch.Of(true), patch.Of(s.typ)
	in.RecurrenceInterval = optField(s.every)
	in.RecurrenceMaxOccurrences = optField(s.times)
	if s.until != nil {
		in.RecurrenceEndYear, in.RecurrenceEndMonth, in.RecurrenceEndDay = patch.Of(s.until[0]), patch.Of(s.until[1]), patch.Of(s.until[2])
	} else {
		in.RecurrenceEndYear, in.RecurrenceEndMonth, in.RecurrenceEndDay = patch.Null[int](), patch.Null[int](), patch.Null[int]()
	}
	if s.rule != nil {
		in.RecurrenceRule = patch.Of(s.rule)
	}
}

func optField(p *int) patch.Field[int] {
	if p == nil {
		return patch.Null[int]()
	}
	return patch.Of(*p)
}

// ---- repeat: rule ----

// ruleKeys are the keys one `repeat_on` condition may use. Each condition
// is one kind; the pairs (moon+phase, weekday+nth, event+days) are one
// condition each.
var ruleKeys = map[string]bool{
	"moon": true, "phase": true, "weekday": true, "nth": true, "weekdays": true,
	"day": true, "month": true, "months": true, "season": true, "season_start": true,
	"event": true, "days": true,
}

// readRule builds a rule from `repeat_on` (a list of conditions a day must
// all meet) and `repeat_offset_days`.
func readRule(cal *calendar.Calendar, r Record, evs []calendar.Event) (calendar.RecurrenceRule, error) {
	var rule calendar.RecurrenceRule
	items := r.List("repeat_on")
	if len(items) == 0 {
		return rule, badRequestf("repeat: rule needs repeat_on, a list of conditions such as `- moon: %s` with `phase: full`", exampleMoon(cal))
	}
	for i, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			return rule, badRequestf("repeat_on item %d must be keys such as `moon:` and `phase:`, not %q", i+1, fmt.Sprint(it))
		}
		c, err := readCondition(cal, lowerMap(m), evs)
		if err != nil {
			return rule, badRequestf("repeat_on item %d: %s", i+1, planError(err))
		}
		rule.Match = append(rule.Match, c)
	}
	n, _, err := r.Int("repeat_offset_days")
	if err != nil {
		return rule, err
	}
	rule.OffsetDays = n
	return rule, nil
}

func lowerMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[strings.ToLower(strings.TrimSpace(k))] = v
	}
	return out
}

func exampleMoon(cal *calendar.Calendar) string {
	if len(cal.Moons) > 0 {
		return cal.Moons[0].Name
	}
	return "<moon>"
}

// readCondition turns one `repeat_on` entry into a rule condition, looking
// every name up in the calendar.
func readCondition(cal *calendar.Calendar, m map[string]any, evs []calendar.Event) (calendar.RuleCondition, error) {
	var c calendar.RuleCondition
	keys := make([]string, 0, len(m))
	for k := range m {
		if !ruleKeys[k] {
			return c, badRequestf("%q is not a repeat_on key", k)
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	has := func(k string) bool { v, ok := m[k]; return ok && v != nil }
	str := func(k string) string { return strings.TrimSpace(fmt.Sprint(m[k])) }
	only := func(want ...string) error {
		ok := map[string]bool{}
		for _, w := range want {
			ok[w] = true
		}
		for _, k := range keys {
			if !ok[k] {
				return badRequestf("%s does not go with %s; put it in its own repeat_on item", k, want[0])
			}
		}
		return nil
	}
	switch {
	case has("moon"):
		if err := only("moon", "phase"); err != nil {
			return c, err
		}
		id, err := moonID(cal, str("moon"))
		if err != nil {
			return c, err
		}
		phase := strings.ReplaceAll(strings.ToLower(str("phase")), " ", "_")
		if !has("phase") {
			return c, badRequestf("moon needs phase (new, first_quarter, full or last_quarter)")
		}
		c.Kind, c.MoonID, c.Phase = calendar.RuleMoonPhase, id, phase
	case has("weekday") || has("nth"):
		if err := only("weekday", "nth"); err != nil {
			return c, err
		}
		if !has("weekday") {
			return c, badRequestf("nth needs weekday")
		}
		wd, err := weekdayIndex(cal, str("weekday"))
		if err != nil {
			return c, err
		}
		c.Weekday = &wd
		c.Kind = calendar.RuleWeekday
		if has("nth") {
			n, err := nthOrLast(str("nth"), "nth")
			if err != nil {
				return c, err
			}
			c.Kind, c.N = calendar.RuleNthWeekday, n
		}
	case has("weekdays"):
		if err := only("weekdays"); err != nil {
			return c, err
		}
		for _, w := range anyList(m["weekdays"]) {
			wd, err := weekdayIndex(cal, strings.TrimSpace(fmt.Sprint(w)))
			if err != nil {
				return c, err
			}
			c.Weekdays = append(c.Weekdays, wd)
		}
		c.Kind = calendar.RuleWeekday
	case has("day"):
		if err := only("day"); err != nil {
			return c, err
		}
		d, err := nthOrLast(str("day"), "day")
		if err != nil {
			return c, err
		}
		c.Kind, c.Day = calendar.RuleDayOfMonth, d
	case has("month"):
		if err := only("month"); err != nil {
			return c, err
		}
		mo, err := monthNumber(cal, str("month"))
		if err != nil {
			return c, err
		}
		c.Kind, c.Month = calendar.RuleMonth, mo
	case has("months"):
		if err := only("months"); err != nil {
			return c, err
		}
		for _, v := range anyList(m["months"]) {
			mo, err := monthNumber(cal, strings.TrimSpace(fmt.Sprint(v)))
			if err != nil {
				return c, err
			}
			c.Months = append(c.Months, mo)
		}
		c.Kind = calendar.RuleMonths
	case has("season"):
		if err := only("season"); err != nil {
			return c, err
		}
		id, err := seasonID(cal, str("season"))
		if err != nil {
			return c, err
		}
		c.Kind, c.SeasonID = calendar.RuleSeason, id
	case has("season_start"):
		if err := only("season_start"); err != nil {
			return c, err
		}
		c.Kind = calendar.RuleSeasonStart
		// "any" is the first day of every season.
		if s := str("season_start"); !strings.EqualFold(s, "any") {
			id, err := seasonID(cal, s)
			if err != nil {
				return c, err
			}
			c.SeasonID = id
		}
	case has("event") || has("days"):
		if err := only("event", "days"); err != nil {
			return c, err
		}
		if !has("event") {
			return c, badRequestf("days needs event, the event to count from")
		}
		id, err := eventID(evs, str("event"))
		if err != nil {
			return c, err
		}
		c.Kind, c.EventID = calendar.RuleRelativeToEvent, id
		if has("days") {
			n, err := strconv.Atoi(str("days"))
			if err != nil {
				return c, badRequestf("days must be a whole number (negative for before)")
			}
			c.Kind, c.Days = calendar.RuleAfterEvent, &n
		}
	default:
		return c, badRequestf("an empty condition")
	}
	return c, nil
}

func anyList(v any) []any {
	if l, ok := v.([]any); ok {
		return l
	}
	return []any{v}
}

// nthOrLast reads 1, 2, … or "last" (the calendar's -1).
func nthOrLast(s, key string) (int, error) {
	if strings.EqualFold(s, "last") {
		return -1, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 0, badRequestf("%s must be a number from 1, or last", key)
	}
	return n, nil
}

func names[T any](items []T, name func(T) string) string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = name(it)
	}
	if len(out) == 0 {
		return "none"
	}
	return strings.Join(out, ", ")
}

func moonID(cal *calendar.Calendar, s string) (int, error) {
	for _, mo := range cal.Moons {
		if sameName(mo.Name, s) {
			return mo.ID, nil
		}
	}
	return 0, badRequestf("%q is not a moon of %s (moons: %s)", s, cal.Name, names(cal.Moons, func(m calendar.Moon) string { return m.Name }))
}

func seasonID(cal *calendar.Calendar, s string) (int, error) {
	for _, se := range cal.Seasons {
		if sameName(se.Name, s) {
			return se.ID, nil
		}
	}
	return 0, badRequestf("%q is not a season of %s (seasons: %s)", s, cal.Name, names(cal.Seasons, func(x calendar.Season) string { return x.Name }))
}

// weekdayIndex is the weekday's place in the week, counted from 0 as the
// rule stores it.
func weekdayIndex(cal *calendar.Calendar, s string) (int, error) {
	for i, w := range cal.Weekdays {
		if sameName(w.Name, s) {
			return i, nil
		}
	}
	return 0, badRequestf("%q is not a weekday of %s (weekdays: %s)", s, cal.Name, names(cal.Weekdays, func(w calendar.Weekday) string { return w.Name }))
}

// monthNumber reads a month name or its number, counted from 1.
func monthNumber(cal *calendar.Calendar, s string) (int, error) {
	if n, err := strconv.Atoi(s); err == nil && n >= 1 && n <= len(cal.Months) {
		return n, nil
	}
	for i, mo := range cal.Months {
		if sameName(mo.Name, s) {
			return i + 1, nil
		}
	}
	return 0, badRequestf("%q is not a month of %s", s, cal.Name)
}

func eventID(evs []calendar.Event, s string) (string, error) {
	var id string
	n := 0
	for _, e := range evs {
		if sameName(e.Name, s) {
			id, n = e.ID, n+1
		}
	}
	switch n {
	case 0:
		return "", badRequestf("no event called %q to repeat around; it has to be on the calendar already, so import it first", s)
	case 1:
		return id, nil
	}
	return "", badRequestf("%d events are called %q; rename one so the repeat can say which", n, s)
}
