package app

import (
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

func TestPickRealWorldCalendar(t *testing.T) {
	world := calendar.Calendar{ID: "w", Mode: calendar.ModeFantasy, IsDefault: true}
	manual := calendar.Calendar{ID: "m", Mode: calendar.ModeRealLife}
	real1 := calendar.Calendar{ID: "r1", Mode: calendar.ModeRealLife, TracksRealTime: true}
	real2 := calendar.Calendar{ID: "r2", Mode: calendar.ModeRealLife, TracksRealTime: true, IsDefault: true}
	tests := []struct {
		name string
		cals []calendar.Calendar
		want string
	}{
		{"none", nil, ""},
		{"only world calendars", []calendar.Calendar{world, manual}, ""},
		{"the first real one", []calendar.Calendar{world, real1}, "r1"},
		{"the default real one wins", []calendar.Calendar{real1, real2}, "r2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pickRealWorldCalendar(tt.cals); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
