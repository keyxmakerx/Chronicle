package calendar

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"testing"
)

// TestCalendarPeek_ColsMatchesWeekLength pins the peek's column count to the
// calendar's own week, since 3 of the 4 shipped presets (Harptos 10, Dwarven
// 6, Blank 10) aren't 7-day weeks and would otherwise draw a wrapped,
// misaligned month. A calendar with no weekdays falls back to 7, never 0.
func TestCalendarPeek_ColsMatchesWeekLength(t *testing.T) {
	tests := []struct {
		name         string
		weekdayCount int
		wantCols     int
	}{
		{"Harptos-shaped: 10 weekdays", 10, 10},
		{"Dwarven-shaped: 6 weekdays", 6, 6},
		{"Gregorian-shaped: 7 weekdays", 7, 7},
		{"no weekdays falls back to 7", 0, 7},
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
			if err := calendarPeek(cal, nil).Render(context.Background(), &buf); err != nil {
				t.Fatalf("Render: %v", err)
			}
			html := buf.String()
			if want := "--cols:" + strconv.Itoa(tt.wantCols); !strings.Contains(html, want) {
				t.Errorf("expected %q in the rendered peek, got:\n%s", want, html)
			}
			if cells := strings.Count(html, "<i"); cells%tt.wantCols != 0 {
				t.Errorf("the peek must draw whole weeks of %d, got %d cells", tt.wantCols, cells)
			}
		})
	}
}

// TestCalendarPeek_TodayAndEventMarks checks the peek shows real data: the
// month's name, today filled in once, and a mark on a day with an event.
func TestCalendarPeek_TodayAndEventMarks(t *testing.T) {
	cal := &Calendar{
		ID: "cal-1", CurrentYear: 12, CurrentMonth: 1, CurrentDay: 5,
		Months:   []Month{{Name: "Firstmonth", Days: 30}},
		Weekdays: []Weekday{{Name: "A"}, {Name: "B"}, {Name: "C"}, {Name: "D"}, {Name: "E"}, {Name: "F"}, {Name: "G"}},
	}
	events := []Event{{ID: "e1", Name: "Fair", Year: 12, Month: 1, Day: 9}}

	var buf bytes.Buffer
	if err := calendarPeek(cal, events).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	html := buf.String()
	for _, want := range []string{"Firstmonth", "<span>12</span>", `aria-hidden="true"`} {
		if !strings.Contains(html, want) {
			t.Errorf("expected %q in the peek, got:\n%s", want, html)
		}
	}
	if n := strings.Count(html, `class="t"`); n != 1 {
		t.Errorf("today must be marked exactly once, got %d in:\n%s", n, html)
	}
	if n := strings.Count(html, `class="e"`); n != 1 {
		t.Errorf("the one event day must carry one mark, got %d in:\n%s", n, html)
	}
}
