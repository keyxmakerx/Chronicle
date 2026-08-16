package calendar

// The emailed "suggest another time" form must name the zone its times will be
// read in.
//
// THE DEFECT: the page rendered three bare date + from + to rows with no zone
// label, no hidden zone field and no JavaScript. parseOfferedWindows turned them
// into bare minute numbers and the write path stamped them with the member's
// stored zone — "UTC" whenever they had never set one, which is the default. A
// New York player typing "Sat 18:00–22:00" had it stored as 18:00–22:00 UTC =
// 14:00–18:00 their time, and the Director's overlay and the computed
// best-window then claimed they had offered a weekday afternoon they cannot
// make. The confirmation page told them their times "were added to your
// availability" without ever naming a zone, so neither party could notice.

import (
	"strings"
	"testing"
)

func TestSuggestPage_NamesTheZoneTimesAreReadIn(t *testing.T) {
	page := rsvpSuggestPage("Harvest Feast — Flamerule 27", "/calendar-rsvp/tok", "csrf", "", "America/New_York")
	if !strings.Contains(page, "America/New_York") {
		t.Fatalf("the form does not name the zone its times will be read in; page=%s", page)
	}
	// The rows are still there — the disclosure must not have replaced the form.
	for _, want := range []string{"w0date", "w0from", "w0to"} {
		if !strings.Contains(page, want) {
			t.Errorf("the form lost its %q input", want)
		}
	}
}

// TestSuggestPage_NoZoneSaysSoAndSaysWhereToFixIt pins the harder half. "Times
// are read as UTC" alone is a correct sentence that leaves a member who does not
// live in UTC with no action to take.
func TestSuggestPage_NoZoneSaysSoAndSaysWhereToFixIt(t *testing.T) {
	page := rsvpSuggestPage("Harvest Feast", "/calendar-rsvp/tok", "csrf", "", "")
	if !strings.Contains(page, "UTC") {
		t.Fatal("a member with no zone set is not told their times will be read as UTC")
	}
	if !strings.Contains(page, "availability page") {
		t.Errorf("the page states the problem but not the remedy; page=%s", page)
	}
}

// TestSuggestPage_ZoneIsEscaped keeps the new interpolation inside the file's
// standing rule: every value on these hand-concatenated pages is escaped. The
// zone reaches here from stored data, so it is not exempt.
func TestSuggestPage_ZoneIsEscaped(t *testing.T) {
	page := rsvpSuggestPage("Feast", "/calendar-rsvp/tok", "csrf", "", `"><script>alert(1)</script>`)
	if strings.Contains(page, "<script>alert(1)</script>") {
		t.Fatalf("the zone was interpolated unescaped; page=%s", page)
	}
}

// TestSuggestPage_ReRenderKeepsTheZone pins the [GR-7] re-render path: a refused
// submission comes back to this form, and it must still say which zone.
func TestSuggestPage_ReRenderKeepsTheZone(t *testing.T) {
	page := rsvpSuggestPage("Feast", "/calendar-rsvp/tok", "csrf",
		"Please add a time that would work.", "Europe/London")
	if !strings.Contains(page, "Europe/London") {
		t.Error("the re-rendered form dropped the zone disclosure")
	}
	if !strings.Contains(page, "Please add a time that would work.") {
		t.Error("the re-rendered form dropped the reason")
	}
}
