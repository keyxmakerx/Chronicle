package timeutil

import "time"

// ZoneAbbrev renders the zone abbreviation for an IANA zone AT a specific
// instant (e.g. "CST" in January, "CDT" in July for America/Chicago) — DST is
// a property of the date, not the zone, so the instant matters as much as the
// zone name. This is a thin wrapper: Go's own Format("MST") layout already
// resolves the correct abbreviation for `at` in `tz`, and already degrades to
// a signed numeric offset (e.g. "+0545") for a zone with no named
// abbreviation (Asia/Kathmandu). There is no additional fallback to write;
// this function exists to document and pin that behavior, not to add to it.
//
// tz is loaded via LoadLocation, so an empty or unresolvable zone degrades to
// UTC rather than panicking — callers that must instead treat "no zone" as its
// own state (never a UTC guess) should check IsValidLocation first, or use
// ZoneAbbrevOrEmpty.
func ZoneAbbrev(tz string, at time.Time) string {
	loc := LoadLocation(tz)
	return at.In(loc).Format("MST")
}

// ZoneAbbrevOrEmpty is ZoneAbbrev for callers that must tell "zone not set or
// not valid" apart from any real abbreviation (the house rule that a
// per-member clock never guesses UTC and presents it as fact — see
// sessions/.ai.md "Role and Zone Display Rules"). It returns "" instead of
// silently resolving an empty/garbage tz to UTC's abbreviation.
func ZoneAbbrevOrEmpty(tz string, at time.Time) string {
	if !IsValidLocation(tz) {
		return ""
	}
	return ZoneAbbrev(tz, at)
}
