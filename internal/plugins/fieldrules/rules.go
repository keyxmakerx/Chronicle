package fieldrules

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Gives is what a rule gives for the followed field's value: the first row
// that matches, or "" when none does or the value is blank. Pure, so the
// matching rules are pinned by table tests.
func (r *Rule) Gives(value any) string {
	text := valueText(value)
	if text == "" {
		return ""
	}
	num, isNum := parseNumber(text)
	for _, row := range r.Rows {
		if row.From != nil || row.To != nil {
			if !isNum {
				continue
			}
			if row.From != nil && num < *row.From {
				continue
			}
			if row.To != nil && num > *row.To {
				continue
			}
			return row.Gives
		}
		if strings.EqualFold(strings.TrimSpace(row.Is), text) {
			return row.Gives
		}
		if n, ok := parseNumber(row.Is); ok && isNum && n == num {
			return row.Gives
		}
	}
	return ""
}

// valueText is a stored field value as the text a row is matched against.
func valueText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(x)
	case float64:
		if x == math.Trunc(x) && math.Abs(x) < 1e15 {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		if x {
			return "true"
		}
		return "false"
	default:
		return strings.TrimSpace(fmt.Sprint(x))
	}
}

func parseNumber(s string) (float64, bool) {
	n, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, false
	}
	return n, true
}

// ChooseWords picks, for each word, the local name a page sees: the nearest
// place in places (the page itself first, then up its parents) that names it,
// else the era. Words kept DM only count only for the Director. The result is
// keyed by the lower-cased usual word.
func ChooseWords(words []LocalWord, places []string, eraID int, director bool) map[string]string {
	rank := make(map[string]int, len(places))
	for i, p := range places {
		if _, seen := rank[p]; !seen {
			rank[p] = i
		}
	}
	type pick struct {
		local string
		at    int // place rank; len(places) for an era
	}
	best := map[string]pick{}
	for _, w := range words {
		if w.DMOnly && !director {
			continue
		}
		at := -1
		switch {
		case w.PlaceID != "":
			if r, ok := rank[w.PlaceID]; ok {
				at = r
			}
		case w.EraID != 0 && w.EraID == eraID:
			at = len(places)
		}
		if at < 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(w.Word))
		if cur, ok := best[key]; !ok || at < cur.at {
			best[key] = pick{local: w.Local, at: at}
		}
	}
	out := make(map[string]string, len(best))
	for k, p := range best {
		out[k] = p.local
	}
	return out
}

// Rename returns the local name for text when one applies. A plural made by
// adding "s" ("Marks") finds the singular word too, since prices are often
// written that way; the local word is used as the owner wrote it.
func Rename(words map[string]string, text string) (string, bool) {
	if len(words) == 0 {
		return "", false
	}
	key := strings.ToLower(strings.TrimSpace(text))
	if key == "" {
		return "", false
	}
	if l, ok := words[key]; ok {
		return l, true
	}
	if strings.HasSuffix(key, "s") {
		if l, ok := words[strings.TrimSuffix(key, "s")]; ok {
			return l, true
		}
	}
	return "", false
}

// MoonNow is one moon on the current date. Day is the day of its cycle, the
// new moon's day being 1, or 0 for a moon set by hand; Words are the phase
// words it matches today ("Waning Crescent", "waning", "crescent").
type MoonNow struct {
	Day   int
	Words []string
}

// Situation is what a rule may read besides the page's fields. Nil maps are
// facts that could not be read, and a test on a missing fact does not hold.
type Situation struct {
	// Moons, keyed by lower-cased moon name.
	Moons map[string]MoonNow
	// EventTags are the lower-cased names of today's events and their kinds.
	EventTags map[string]bool
	// DayPart is dawn, day, dusk or night; "" when the calendar can't say.
	DayPart string
	// Trackers are the page's trackers by lower-cased name.
	Trackers map[string]float64
}

// Result is what the rule gives for a page: the first row that matches what
// the rule follows and whose conditions hold, then each adjustment whose
// tracker test holds. Pure, like Gives.
func (r *Rule) Result(data map[string]any, sit Situation) string {
	out := r.match(data, sit)
	if out == "" || len(r.Adjust) == 0 {
		return out
	}
	num, ok := parseNumber(out)
	if !ok {
		return out
	}
	for _, a := range r.Adjust {
		v, read := sit.Trackers[key(a.Tracker)]
		if !read || !compare(a.Op, v, a.Value) {
			continue
		}
		num += a.Add
		if a.Min != nil && num < *a.Min {
			num = *a.Min
		}
		if a.Max != nil && num > *a.Max {
			num = *a.Max
		}
	}
	return valueText(num)
}

// match finds the first row that holds.
func (r *Rule) match(data map[string]any, sit Situation) string {
	follows := strings.TrimSpace(r.Follows)
	lower := strings.ToLower(follows)
	for _, row := range r.Rows {
		if !row.conditionsHold(sit) {
			continue
		}
		if row.Is == "" && row.From == nil && row.To == nil {
			return row.Gives
		}
		switch {
		case strings.HasPrefix(lower, FollowMoonPrefix):
			m, ok := sit.Moons[key(follows[len(FollowMoonPrefix):])]
			if ok && row.matchesMoon(m) {
				return row.Gives
			}
		case strings.HasPrefix(lower, FollowTrackerPrefix):
			v, ok := sit.Trackers[key(follows[len(FollowTrackerPrefix):])]
			if ok && row.matchesValue(v) {
				return row.Gives
			}
		case lower == FollowTimeOfDay:
			if sit.DayPart != "" && row.matchesValue(sit.DayPart) {
				return row.Gives
			}
		default:
			if row.matchesValue(data[follows]) {
				return row.Gives
			}
		}
	}
	return ""
}

func (row RuleRow) conditionsHold(sit Situation) bool {
	if row.EventTag != "" && !sit.EventTags[key(row.EventTag)] {
		return false
	}
	if row.Time != "" && !strings.EqualFold(strings.TrimSpace(row.Time), sit.DayPart) {
		return false
	}
	return true
}

// matchesValue is one row of Gives.
func (row RuleRow) matchesValue(value any) bool {
	one := Rule{Rows: []RuleRow{{Is: row.Is, From: row.From, To: row.To, Gives: "x"}}}
	return one.Gives(value) != ""
}

// matchesMoon matches a phase word, or a range of cycle days that may wrap.
func (row RuleRow) matchesMoon(m MoonNow) bool {
	if row.Is != "" {
		for _, w := range m.Words {
			if strings.EqualFold(strings.TrimSpace(row.Is), w) {
				return true
			}
		}
		return false
	}
	if m.Day == 0 {
		return false
	}
	d := float64(m.Day)
	switch {
	case row.From != nil && row.To != nil && *row.From > *row.To:
		return d >= *row.From || d <= *row.To
	case row.From != nil && d < *row.From:
		return false
	case row.To != nil && d > *row.To:
		return false
	}
	return true
}

// compare applies an adjustment's operator.
func compare(op string, a, b float64) bool {
	const eps = 1e-9
	switch op {
	case "gte":
		return a >= b-eps
	case "gt":
		return a > b+eps
	case "lte":
		return a <= b+eps
	case "lt":
		return a < b-eps
	case "eq":
		return math.Abs(a-b) <= eps
	}
	return false
}

// key is how names are matched: trimmed, ignoring case.
func key(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// Needs says which facts a rule reads besides fields, so a page asks only
// for what its rules use.
func (r *Rule) Needs() (calendar, trackers bool) {
	lower := strings.ToLower(strings.TrimSpace(r.Follows))
	calendar = strings.HasPrefix(lower, FollowMoonPrefix) || lower == FollowTimeOfDay
	trackers = strings.HasPrefix(lower, FollowTrackerPrefix) || len(r.Adjust) > 0
	for _, row := range r.Rows {
		calendar = calendar || row.EventTag != "" || row.Time != ""
	}
	return calendar, trackers
}
