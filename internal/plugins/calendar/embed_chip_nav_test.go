// embed_chip_nav_test.go — C-CAL-EMBED-CHIPS-TAP. Pins the chosen fix for the
// wave-8 gate finding (#549 PASS_WITH_NOTES): the dashboard/entity-page
// calendar embed rendered event chips as <button> with cursor-pointer + a
// hover ring but no click handler anywhere in the embed context (the V1
// scripts that once wired them were correctly deleted by #549) — a silently
// inert affordance. Option A (tap → navigate) was chosen over Option B
// (demote to non-interactive spans): embeds live on dashboards/entity pages,
// so leaving the page to the V2 calendar shell on tap is the honest
// affordance, and it costs zero new routes.
package calendar

import (
	"context"
	"strings"
	"testing"
)

// embedChipCalendar builds a minimal Calendar for exercising dayCell in
// isolation: one month, long enough to hold the test day.
func embedChipCalendar() *Calendar {
	return &Calendar{
		ID:   "cal-1",
		Name: "Test Calendar",
		Months: []Month{
			{Name: "Longmonth", Days: 31},
		},
	}
}

// TestDayCellEventChip_LinksToV2FocusedOnDate is the Option A pin: the chip
// is a real <a> (not a <button>) whose href lands on the V2 shell for the
// SAME calendar, cursored to the day CELL it's rendered under via the
// year/month/day query params ShowV2 already parses (handler_v2.go) — no new
// route. Deliberately uses the cell's year/month/day, not the event's own
// Year/Month/Day fields, since for a recurring event those hold the
// original occurrence's creation date rather than the date being displayed.
func TestDayCellEventChip_LinksToV2FocusedOnDate(t *testing.T) {
	data := CalendarViewData{
		Calendar:   embedChipCalendar(),
		Year:       2026,
		MonthIndex: 7,
		CampaignID: "camp-1",
		Events: []Event{
			// Year/Month deliberately differ from the cell being rendered —
			// as a recurring event's stored fields would — to prove the href
			// uses the cell's date, not the event's.
			{ID: "evt-1", Name: "Feast Day", Day: 18, Month: 3, Year: 2020, Visibility: "everyone", IsRecurring: true},
		},
	}
	var buf strings.Builder
	if err := dayCell(data, 18).Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()

	if strings.Contains(html, "<button") {
		t.Errorf("event chip must not render as a <button> (no click handler exists in the embed context); got:\n%s", html)
	}
	// templ HTML-escapes the query string's "&" to "&amp;" on render.
	wantHref := "/campaigns/camp-1/calendar/v2/cal-1?year=2026&amp;month=7&amp;day=18"
	if !strings.Contains(html, `<a href="`+wantHref+`"`) {
		t.Errorf("event chip must link to the V2 shell cursored to the cell's date; want href %q in:\n%s", wantHref, html)
	}
	if !strings.Contains(html, "Feast Day") {
		t.Error("chip must still render the event name")
	}
}

// TestDayCellEventChip_NoEventsRendersNoChip guards against dayCell emitting
// a stray anchor/button when a day has no events.
func TestDayCellEventChip_NoEventsRendersNoChip(t *testing.T) {
	data := CalendarViewData{
		Calendar:   embedChipCalendar(),
		Year:       2026,
		MonthIndex: 7,
		CampaignID: "camp-1",
	}
	var buf strings.Builder
	if err := dayCell(data, 5).Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	if strings.Contains(html, "<a href") || strings.Contains(html, "<button") {
		t.Errorf("a day with no events must not render an event chip; got:\n%s", html)
	}
}
