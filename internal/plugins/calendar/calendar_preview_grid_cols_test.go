package calendar

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"testing"
)

// TestPreviewMonthGrid_ColsMatchesWeekdayCount pins the fix for the
// hardcoded 7-column grid (static/css/calendar_v5.css' .calv5-mgrid
// default): previewMonthGrid must set --cols inline from the calendar's own
// weekday count, since 3 of the 4 shipped presets (Harptos 10, Dwarven 6,
// Blank 10) aren't 7-day weeks and would otherwise render wrapped/
// misaligned. Uses a 10-weekday calendar (Harptos' shape) as the
// non-7 case, plus a 7-weekday control so the fix doesn't just special-case
// "not 7".
func TestPreviewMonthGrid_ColsMatchesWeekdayCount(t *testing.T) {
	tests := []struct {
		name         string
		weekdayCount int
	}{
		{"Harptos-shaped: 10 weekdays", 10},
		{"Dwarven-shaped: 6 weekdays", 6},
		{"Gregorian-shaped: 7 weekdays", 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			weekdays := make([]Weekday, tt.weekdayCount)
			for i := range weekdays {
				weekdays[i] = Weekday{Name: "Day"}
			}
			cal := &Calendar{
				ID: "cal-1", CurrentYear: 1, CurrentMonth: 1, CurrentDay: 1,
				Months:   []Month{{Name: "Firstmonth", Days: 30}},
				Weekdays: weekdays,
			}

			var buf bytes.Buffer
			if err := previewMonthGrid(cal, nil).Render(context.Background(), &buf); err != nil {
				t.Fatalf("Render: %v", err)
			}
			html := buf.String()
			want := "--cols:" + strconv.Itoa(tt.weekdayCount)
			if !strings.Contains(html, want) {
				t.Errorf("expected %q in the rendered grid, got:\n%s", want, html)
			}
		})
	}
}

// TestPreviewMonthGrid_EmptyWeekdaysNeverZerosCols guards the defensive
// floor: an empty Weekdays list must never render --cols:0, which would
// hide the grid entirely (worse than the wrong-but-visible 7-column
// default).
func TestPreviewMonthGrid_EmptyWeekdaysNeverZerosCols(t *testing.T) {
	cal := &Calendar{ID: "cal-1", CurrentYear: 1, CurrentMonth: 1, CurrentDay: 1, Months: []Month{{Name: "Firstmonth", Days: 30}}}

	var buf bytes.Buffer
	if err := previewMonthGrid(cal, nil).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(buf.String(), "--cols:0") {
		t.Errorf("must never render --cols:0, got:\n%s", buf.String())
	}
}
