// zones.go — the single canonical curated IANA timezone list (C-TZ-CONSOLIDATION).
//
// Before this file, three UI surfaces (account settings, calendar real-time
// anchor, availability scheduler) each hand-curated their own IANA zone list
// for a <select> dropdown. They drifted: a zone added to one was missing from
// another (Step-0 inventory: cordinator/reports/chronicle/2026-07-18-c-tz-
// consolidation.md). The full IANA tz database is ~600 entries — too many for
// a usable dropdown — so every surface ships a curated subset plus whatever
// zone the browser detects / the user already has stored. This file is that
// subset, defined exactly once.
//
// CommonZones is the UNION of the three pre-consolidation lists (pinned by
// zones_test.go against the old lists verbatim): consolidating must never
// remove a zone any surface could previously offer, since a calendar or
// availability row may already be stored against it.
package timeutil

import (
	"encoding/json"
	"time"
)

// Zone is one curated timezone option: the IANA identifier consumers store
// and send back, plus the label shown in the picker. The two are identical
// today (no surface has ever shown a friendlier name) — kept as separate
// fields so a future display-name pass doesn't have to touch every consumer.
type Zone struct {
	Value string
	Label string
}

// commonZoneNames is the curated region list, validated against the host's
// tzdata at call time (an entry missing from the host's tzdata is silently
// dropped rather than rendering an unselectable option — same behavior the
// three pre-consolidation lists each had independently). UTC leads the list
// (the universal, zero-offset default); the rest keep the pre-consolidation
// alphabetical-by-region order so the visible option order doesn't reshuffle
// for the two surfaces that already rendered it that way.
var commonZoneNames = []string{
	"UTC",
	"Africa/Cairo", "Africa/Johannesburg", "Africa/Lagos", "Africa/Nairobi",
	"America/Anchorage", "America/Argentina/Buenos_Aires", "America/Bogota",
	"America/Chicago", "America/Denver", "America/Halifax", "America/Los_Angeles",
	"America/Mexico_City", "America/New_York", "America/Phoenix",
	"America/Santiago", "America/Sao_Paulo", "America/St_Johns", "America/Toronto",
	"America/Vancouver",
	"Asia/Baghdad", "Asia/Bangkok", "Asia/Colombo", "Asia/Dubai", "Asia/Hong_Kong",
	"Asia/Istanbul", "Asia/Jakarta", "Asia/Karachi", "Asia/Kolkata", "Asia/Manila",
	"Asia/Seoul", "Asia/Shanghai", "Asia/Singapore", "Asia/Taipei", "Asia/Tehran",
	"Asia/Tokyo",
	"Atlantic/Reykjavik",
	"Australia/Adelaide", "Australia/Brisbane", "Australia/Melbourne",
	"Australia/Perth", "Australia/Sydney",
	"Europe/Amsterdam", "Europe/Athens", "Europe/Berlin", "Europe/Brussels",
	"Europe/Dublin", "Europe/Helsinki", "Europe/Lisbon", "Europe/London",
	"Europe/Madrid", "Europe/Moscow", "Europe/Oslo", "Europe/Paris",
	"Europe/Prague", "Europe/Rome", "Europe/Stockholm", "Europe/Vienna",
	"Europe/Warsaw", "Europe/Zurich",
	"Pacific/Auckland", "Pacific/Fiji", "Pacific/Guam", "Pacific/Honolulu",
}

// CommonZones returns the canonical curated timezone list — value+label pairs
// for a picker — validated against the host's tz database. This is the ONLY
// hand-curated zone list in the codebase; every dropdown (account settings,
// calendar real-time anchor, availability scheduler) renders from it.
func CommonZones() []Zone {
	zones := make([]Zone, 0, len(commonZoneNames))
	for _, name := range commonZoneNames {
		if _, err := time.LoadLocation(name); err == nil {
			zones = append(zones, Zone{Value: name, Label: name})
		}
	}
	return zones
}

// CommonZonesJSON returns CommonZones' values (not labels) as a JSON array,
// for server-embedding into HTML via a data attribute — how a JS consumer
// (e.g. the availability scheduler) reads the canonical list without a new
// endpoint and without hand-rolling its own copy.
func CommonZonesJSON() string {
	zones := CommonZones()
	values := make([]string, len(zones))
	for i, z := range zones {
		values[i] = z.Value
	}
	b, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(b)
}
