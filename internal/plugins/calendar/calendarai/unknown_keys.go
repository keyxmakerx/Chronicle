package calendarai

import (
	"sort"
	"strings"
)

// engineKeys ride on every block: the engine reads them itself.
var engineKeys = []string{"kind", "action", "name", "reason"}

// keySet is a kind's own keys plus the engine's.
func keySet(own ...string) map[string]bool {
	m := make(map[string]bool, len(own)+len(engineKeys))
	for _, k := range engineKeys {
		m[k] = true
	}
	for _, k := range own {
		m[k] = true
	}
	return m
}

// unknownKeyWarnings names each top-level key the kind does not read. The
// rest of the block still applies, so this warns and never blocks; a silent
// drop would let an AI believe a setting it can't make had been made.
func unknownKeyWarnings(r Record, known map[string]bool, kind string) []string {
	var keys []string
	for k := range r.Fields {
		if !known[strings.ToLower(k)] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = "`" + k + "` is not a key of the " + kind + " block, so it was ignored"
	}
	return out
}

// calendarKeys and eventKeys are every key the two kinds read. A key added to
// a kind's Doc belongs here too; TestKnownKeysCoverDocs fails when one is
// missing.
var calendarKeys = keySet("rename_to", "main", "year_label", "current_year", "current_month", "current_day",
	"months", "weekdays", "seasons", "moons", "sun", "eras", "leap_year_every",
	"tiers", "hemisphere", "forecasts", "month_starts_new_week", "era_look")

var eventKeys = keySet("year", "month", "day", "end_year", "end_month", "end_day", "hour", "minute",
	"end_hour", "end_minute", "all_day", "visibility", "color", "icon", "rename_to", "importance", "category",
	"page", "calendar", "announced", "move_to_year", "move_to_month", "move_to_day",
	"repeat", "repeat_every", "repeat_times", "repeat_on", "repeat_offset_days",
	"repeat_until_year", "repeat_until_month", "repeat_until_day", "roll")
