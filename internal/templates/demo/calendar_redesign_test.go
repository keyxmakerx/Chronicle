// calendar_redesign_test.go — C-CAL-WORLDSTATE-INTERACTION-REDESIGN (R1 + R2).
//
// Static guards on the time-control + date-setter redesign: the sun is now a
// passive visual, the TIME readout is the draggable + keyboard slider (the
// a11y the hardening deferred), and the date readout opens a day/named-month/
// year setter that commits via setWorldState({date}). Drag/keyboard behaviour
// needs a real DOM — guarded structurally here (the dispatch's sanctioned
// fallback) + the operator's visual gate.

package demo

import (
	"strings"
	"testing"
)

// TestCalAlmanac_SunDemoted — the sun is a passive, aria-hidden visual; it is
// no longer the interactive drag/slider target.
func TestCalAlmanac_SunDemoted(t *testing.T) {
	html := renderAlmanac(t)
	if !strings.Contains(html, "cal-almanac-sky__sun--passive") {
		t.Errorf("sun must be marked passive (cal-almanac-sky__sun--passive)")
	}
	// the old interactive sun affordances are gone.
	if strings.Contains(html, `aria-label="Time of day — drag to scrub"`) {
		t.Errorf("the old draggable-sun aria-label must be removed")
	}
	js := readCalAlmanacJS(t)
	for _, gone := range []string{"registerInitBlock('sun-drag-scrub'", "Map x 8%..92%"} {
		if strings.Contains(js, gone) {
			t.Errorf("the sun-drag wiring must be removed: %q", gone)
		}
	}
}

// TestCalAlmanac_TimeControlSlider — the TIME readout is the drag + keyboard
// slider with full role=slider a11y.
func TestCalAlmanac_TimeControlSlider(t *testing.T) {
	js := readCalAlmanacJS(t)
	for _, m := range []string{
		"registerInitBlock('time-control'",
		"setAttribute('role', 'slider')",
		"setAttribute('aria-valuenow'",
		"aria-valuetext",
		"case 'ArrowLeft': case 'ArrowDown'", // keyboard stepping
		"case 'PageDown'", "case 'Home'", "case 'End'",
		"dx / 600",                // pixels→time drag mapping
		"function openTimeInput(", // click-to-type preserved
		"__calSyncTimeAria",       // a11y kept in sync on external changes
	} {
		if !strings.Contains(js, m) {
			t.Errorf("time-control marker missing: %q", m)
		}
	}
	// parseTime (type-to-set) still present.
	if !strings.Contains(js, "function parseTime(str, hpd)") {
		t.Errorf("parseTime (type-to-set) must be preserved")
	}
}

// TestCalAlmanac_DateSetter — clicking the date opens a day/named-month/year
// setter that commits via setWorldState({date}); month uses the calendar's
// named months (not 1–12).
func TestCalAlmanac_DateSetter(t *testing.T) {
	html := renderAlmanac(t)
	for _, m := range []string{
		"data-cal-sky-date",   // the trigger
		"data-cal-datesetter", // the popover (role=dialog)
		`role="dialog"`,
		"data-cal-datesetter-day", "data-cal-datesetter-month", "data-cal-datesetter-year",
		"data-cal-datesetter-go", "data-cal-datesetter-cancel",
	} {
		if !strings.Contains(html, m) {
			t.Errorf("date-setter markup missing: %q", m)
		}
	}
	// the month <select> is populated with the calendar's NAMED months.
	d := CalAlmanacMock()
	if len(d.Months) > 0 && !strings.Contains(html, ">"+d.Months[0].Name+"</option>") {
		t.Errorf("date-setter month select must list named months (e.g. %q)", d.Months[0].Name)
	}
	js := readCalAlmanacJS(t)
	for _, m := range []string{
		"registerInitBlock('date-setter'",
		"setWorldState({ date: { year: y, month: mo, day: day }",
		"function monthLen(", // day-range from the month's length
	} {
		if !strings.Contains(js, m) {
			t.Errorf("date-setter logic marker missing: %q", m)
		}
	}
}
