// calendar_v2_ux_pair_test.go — C-CAL-UX-PAIR. Markup-level pins for both
// fixes:
//
//   Fix 1: the Month-grid ribbon's drag-to-resize handle ([data-ribbon-resize])
//   is a WRITE affordance (event_grid.js's initRibbonResize wires a PUT on
//   drag) — gate-split audit found it rendering for every role, not just
//   Scribes, so a player could see/attempt-grab a resize handle whose JS is
//   now Scribe-gated too (calendar_v2_ux_pair.templ change, see
//   monthWeekRows). This pins the markup side of that fix.
//
//   Fix 2: the viewer-zone time hint's server-rendered anchor points — the
//   drawer's WHEN-section hint line and the mobile agenda card's hint marker
//   span — must exist in the markup for event_grid.js / calendar_v2_shell.js
//   to fill in client-side. No time math is tested here (that's the JS pure-
//   math suite, test/js/calendar_v2_rt_hint.test.mjs) — just that the DOM
//   hooks exist.

package calendar

import (
	"context"
	"strings"
	"testing"
)

// TestMonthWeekRows_RibbonResizeHandle_ScribeOnly pins that the multi-day
// ribbon's resize handle only renders for Scribes; players get the ribbon
// chip (still clickable → read-only quick-edit) without the resize affordance.
func TestMonthWeekRows_RibbonResizeHandle_ScribeOnly(t *testing.T) {
	endDay := 10
	base := CalendarV2ViewData{
		ActiveCalendar: gregorian2026(),
		Year:           2026, Month: 6, Day: 8,
		Events: []Event{{ID: "war", Year: 2026, Month: 6, Day: 8, EndDay: &endDay, Visibility: "everyone"}},
	}

	render := func(isScribe bool) string {
		t.Helper()
		data := base
		data.IsScribe = isScribe
		var sb strings.Builder
		if err := monthWeekRows(data).Render(context.Background(), &sb); err != nil {
			t.Fatalf("render monthWeekRows(isScribe=%v): %v", isScribe, err)
		}
		return sb.String()
	}

	scribeHTML := render(true)
	if !strings.Contains(scribeHTML, "data-ribbon-resize") {
		t.Error("scribe render must include the resize handle for a closed-right ribbon segment")
	}
	if !strings.Contains(scribeHTML, `data-event-id="war"`) {
		t.Fatal("fixture sanity: the war ribbon segment must render at all")
	}

	playerHTML := render(false)
	if !strings.Contains(playerHTML, `data-event-id="war"`) {
		t.Error("players must still see the ribbon chip itself (read-only tap target)")
	}
	if strings.Contains(playerHTML, "data-ribbon-resize") {
		t.Error("players must NOT receive the resize handle — it's a write affordance (event_grid.js initRibbonResize PUTs on drag)")
	}
}

// TestEventV2Drawer_RtTimeHintMarkerPresent pins the drawer WHEN-section's
// hint anchor: a hidden, read-only <p data-rt-time-hint> that
// event_grid.js's updateRtTimeHint() fills in client-side. The drawer only
// renders for Scribes (server-gated at the call site — Fix 1's boundary), so
// this test renders it directly with IsScribe implied by the caller (no
// role branching inside eventV2Drawer itself).
func TestEventV2Drawer_RtTimeHintMarkerPresent(t *testing.T) {
	data := CalendarV2ViewData{ActiveCalendar: gregorian2026(), Year: 2026, Month: 6, Day: 8}
	var sb strings.Builder
	if err := eventV2Drawer(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render eventV2Drawer: %v", err)
	}
	html := sb.String()
	if !strings.Contains(html, "data-rt-time-hint") {
		t.Error("drawer WHEN section must carry the data-rt-time-hint anchor for the viewer-zone hint")
	}
	if !strings.Contains(html, `data-time-fields`) {
		t.Fatal("fixture sanity: the time-fields block must render")
	}
	// Hidden by default (server never knows the viewer's browser zone) —
	// only event_grid.js's updateRtTimeHint() unhides it.
	timeFieldsIdx := strings.Index(html, "data-time-fields")
	hintIdx := strings.Index(html, "data-rt-time-hint")
	if hintIdx < timeFieldsIdx {
		t.Error("the hint line must follow the time inputs (WHEN section reading order)")
	}
	// The hint <p> itself carries class="hidden" by default.
	hintTagStart := strings.LastIndex(html[:hintIdx], "<p")
	if hintTagStart < 0 || !strings.Contains(html[hintTagStart:hintIdx], "hidden") {
		t.Error("the rt-time-hint line must be hidden by default (server can't know the viewer's zone)")
	}
}

// TestMobileAgendaCard_RtHintMarkerPresent pins the mobile agenda card's
// hint anchor: a hidden <span data-rt-hint> calendar_v2_shell.js's
// wireEventTimeHints fills in for real-time calendars.
func TestMobileAgendaCard_RtHintMarkerPresent(t *testing.T) {
	ev := mobileAgendaEvent{EventID: "ev-1", Title: "Siege", TimeLabel: "19:00", Category: "Combat", IsPublic: true}
	var sb strings.Builder
	if err := mobileAgendaCard(ev).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render mobileAgendaCard: %v", err)
	}
	html := sb.String()
	if !strings.Contains(html, "data-rt-hint") {
		t.Error("mobile agenda card subtitle must carry the data-rt-hint marker")
	}
	if !strings.Contains(html, `data-event-card="agenda"`) {
		t.Fatal("fixture sanity: agenda card must keep its data-event-card hook (C-CAL-MOBILE-AGENDA)")
	}
}
