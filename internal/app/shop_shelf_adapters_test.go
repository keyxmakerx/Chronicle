package app

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

func TestShelfDayOf(t *testing.T) {
	month := []calendar.Month{{ID: 1, Name: "Deepwinter", Days: 30, SortOrder: 1}, {ID: 2, Name: "Frostmoot", Days: 30, SortOrder: 2}}
	at := func(h, m int) *calendar.Calendar {
		return &calendar.Calendar{CurrentYear: 3, CurrentMonth: 2, CurrentDay: 12, CurrentHour: h, CurrentMinute: m,
			HoursPerDay: 24, MinutesPerHour: 60, Months: month}
	}
	c := at(12, 0)
	dawn := dawnHour(c, 3, 2, 12, 24)
	if dawn <= 0 || dawn >= 12 {
		t.Fatalf("dawn hour %d is not in the morning", dawn)
	}
	today := int64(c.CurrentDayIndex())
	cases := []struct {
		name      string
		h, m      int
		wantIdx   int64
		wantHours int
	}{
		{"just before dawn is still yesterday's shelf", dawn - 1, 30, today - 1, 1},
		{"midnight is still yesterday's shelf", 0, 0, today - 1, dawn},
		{"dawn changes the shelf", dawn, 0, today, 24},
		{"evening counts on to tomorrow's dawn", 20, 15, today, 24 + dawn - 20},
	}
	for _, tc := range cases {
		d := shelfDayOf(at(tc.h, tc.m))
		if d.Index != tc.wantIdx || d.HoursToDawn != tc.wantHours {
			t.Errorf("%s: got day %d, %d h; want day %d, %d h", tc.name, d.Index, d.HoursToDawn, tc.wantIdx, tc.wantHours)
		}
	}
}

func TestShelfDayAdapter(t *testing.T) {
	ctx := context.Background()
	if d, err := (&shelfDayAdapter{}).ShelfDay(ctx, "camp", 1, "u1"); d != nil || err != nil {
		t.Errorf("no calendar plugin: %v %v", d, err)
	}
	none := &fakeCraftingCalendar{err: apperror.NewNotFound("default calendar")}
	if d, err := (&shelfDayAdapter{cal: none}).ShelfDay(ctx, "camp", 1, "u1"); d != nil || err != nil {
		t.Errorf("no calendar: %v %v", d, err)
	}
	if _, err := (&shelfDayAdapter{cal: &fakeCraftingCalendar{err: errors.New("db down")}}).ShelfDay(ctx, "camp", 3, "gm"); apperror.SafeCode(err) == http.StatusNotFound || err == nil {
		t.Errorf("a fault must be an error: %v", err)
	}
	t.Run("the countdown only for a viewer who can see the calendar", func(t *testing.T) {
		cal := &calendar.Calendar{CurrentYear: 3, CurrentMonth: 2, CurrentDay: 12, CurrentHour: 20, HoursPerDay: 24, MinutesPerHour: 60,
			Months: []calendar.Month{{ID: 1, Name: "A", Days: 30, SortOrder: 1}, {ID: 2, Name: "B", Days: 30, SortOrder: 2}}}
		hidden := &viewerGatedCalendar{cal: cal}
		d, err := (&shelfDayAdapter{cal: hidden}).ShelfDay(ctx, "camp", permissions.RolePlayer, "u1")
		if err != nil || d == nil || d.HoursToDawn != -1 || d.Index != int64(cal.CurrentDayIndex()) {
			t.Errorf("hidden calendar: %+v %v", d, err)
		}
		hidden.visible = true
		if d, _ := (&shelfDayAdapter{cal: hidden}).ShelfDay(ctx, "camp", permissions.RolePlayer, "u1"); d == nil || d.HoursToDawn <= 0 {
			t.Errorf("visible calendar: %+v", d)
		}
		hidden.visible = false
		if d, _ := (&shelfDayAdapter{cal: hidden}).ShelfDay(ctx, "camp", permissions.RoleOwner, "gm"); d == nil || d.HoursToDawn <= 0 {
			t.Errorf("owner: %+v", d)
		}
	})
}

// viewerGatedCalendar answers the system read, and a request viewer's only
// when visible.
type viewerGatedCalendar struct {
	cal     *calendar.Calendar
	visible bool
}

func (f *viewerGatedCalendar) GetDefaultCalendarForViewer(_ context.Context, _ string, v permissions.Viewer) (*calendar.Calendar, error) {
	if v.IsSystem() || f.visible {
		return f.cal, nil
	}
	return nil, apperror.NewNotFound("default calendar")
}
