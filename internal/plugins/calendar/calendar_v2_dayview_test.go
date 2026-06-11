// calendar_v2_dayview_test.go — the day MINI-VIEW (cordinator#33 item 4): the
// first tier on a date-cell click. The card itself (events + worldstate peek)
// is for ALL roles, but the create path ("Add event") is SERVER-GATED to
// Scribes — markup-level, not a CSS hide — so a player never receives it.
package calendar

import (
	"context"
	"strings"
	"testing"
)

func renderDayMiniView(t *testing.T, isScribe bool) string {
	t.Helper()
	var sb strings.Builder
	if err := dayDetailPopoverV2(CalendarV2ViewData{IsScribe: isScribe}).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render day mini-view: %v", err)
	}
	return sb.String()
}

// TestDayMiniView_ScribeGetsAddEvent: a Scribe gets the full card — the
// worldstate peek + events list + close, PLUS the "Add event" create button.
func TestDayMiniView_ScribeGetsAddEvent(t *testing.T) {
	html := renderDayMiniView(t, true)
	for _, want := range []string{
		`id="cal-v2-day-popover"`,
		"card card-elev",              // matches the quick-edit design language
		"data-day-popover-worldstate", // reused worldstate peek container
		"data-day-popover-list",       // events list
		"data-day-popover-close",      // close ×
		"data-day-popover-add",        // Scribe-only create path
	} {
		if !strings.Contains(html, want) {
			t.Errorf("scribe day mini-view missing %q", want)
		}
	}
}

// TestDayMiniView_PlayerNoAddEvent: a player sees the card (events +
// worldstate) but NOT the Add-event button — the gate is the markup.
func TestDayMiniView_PlayerNoAddEvent(t *testing.T) {
	html := renderDayMiniView(t, false)
	for _, want := range []string{"data-day-popover-worldstate", "data-day-popover-list", "data-day-popover-close"} {
		if !strings.Contains(html, want) {
			t.Errorf("player day mini-view missing %q", want)
		}
	}
	if strings.Contains(html, "data-day-popover-add") {
		t.Errorf("player day mini-view must NOT contain the Add-event button (server-gated to Scribes)")
	}
}

// TestMonthDayCell_CarriesDayCellHook: every month day cell must carry the
// data-day-cell hook (all roles) the shell uses to open the mini-view on a
// date click. (readRepoFile lives in calendar_v2_daypeek_test.go.)
func TestMonthDayCell_CarriesDayCellHook(t *testing.T) {
	src := readRepoFile(t, "internal/plugins/calendar/calendar_v2.templ")
	if !strings.Contains(src, "data-day-cell=") {
		t.Errorf("monthDayCell must carry data-day-cell so the mini-view opens on a date click")
	}
}
