package timeutil

import (
	"testing"
	"time"
)

// TestZoneAbbrev_DSTBoundary pins that the abbreviation is resolved from the
// INSTANT, not just the zone name, across a real DST transition.
func TestZoneAbbrev_DSTBoundary(t *testing.T) {
	jan := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	jul := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)

	if got := ZoneAbbrev("America/Chicago", jan); got != "CST" {
		t.Errorf("America/Chicago in January = %q, want CST", got)
	}
	if got := ZoneAbbrev("America/Chicago", jul); got != "CDT" {
		t.Errorf("America/Chicago in July = %q, want CDT", got)
	}
}

// TestZoneAbbrev_NoNamedAbbreviation pins the numeric-offset degradation for a
// zone tzdata gives no alphabetic abbreviation (UTC+5:45).
func TestZoneAbbrev_NoNamedAbbreviation(t *testing.T) {
	at := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	got := ZoneAbbrev("Asia/Kathmandu", at)
	if got != "+0545" {
		t.Errorf("Asia/Kathmandu = %q, want +0545", got)
	}
}

// TestZoneAbbrev_UTC pins the universal zone's own abbreviation.
func TestZoneAbbrev_UTC(t *testing.T) {
	if got := ZoneAbbrev("UTC", time.Now()); got != "UTC" {
		t.Errorf("UTC = %q, want UTC", got)
	}
}

// TestZoneAbbrevOrEmpty_InvalidInput pins that an empty or garbage zone never
// resolves to a UTC guess presented as fact.
func TestZoneAbbrevOrEmpty_InvalidInput(t *testing.T) {
	at := time.Now()
	for _, tz := range []string{"", "Not/AZone", "garbage"} {
		if got := ZoneAbbrevOrEmpty(tz, at); got != "" {
			t.Errorf("ZoneAbbrevOrEmpty(%q) = %q, want \"\"", tz, got)
		}
	}
}

// TestZoneAbbrevOrEmpty_ValidZone pins the pass-through case still works.
func TestZoneAbbrevOrEmpty_ValidZone(t *testing.T) {
	jan := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	if got := ZoneAbbrevOrEmpty("America/Chicago", jan); got != "CST" {
		t.Errorf("ZoneAbbrevOrEmpty(America/Chicago) = %q, want CST", got)
	}
}
