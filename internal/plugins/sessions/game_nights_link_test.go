package sessions

import "testing"

func TestGameNightsTarget(t *testing.T) {
	tests := []struct {
		name                 string
		cal, sid, date, want string
	}{
		{"no calendar, the list", "", "", "", "/campaigns/c1/sessions"},
		{"no calendar, the session's own page", "", "s1", "2026-10-08", "/campaigns/c1/sessions/s1"},
		{"a night opens its day", "k1", "s1", "2026-10-08", "/campaigns/c1/calendars/k1/view?date=2026-10-08&night=s1"},
		{"no night named opens the next one", "k1", "", "", "/campaigns/c1/calendars/k1/view?night=next"},
		{"a session with no date opens its own page", "k1", "s1", "", "/campaigns/c1/sessions/s1"},
		{"a bad date opens the session's own page", "k1", "s1", "Thursday", "/campaigns/c1/sessions/s1"},
		{"ids are escaped", "k/1", "s&x", "2026-10-08", "/campaigns/c1/calendars/k%2F1/view?date=2026-10-08&night=s%26x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := gameNightsTarget("c1", tt.cal, tt.sid, tt.date); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSidebarNightDate(t *testing.T) {
	tests := []struct {
		name string
		s    Session
		want string
	}{
		{"one-off", Session{ScheduledDate: strp("2026-10-08")}, "2026-10-08"},
		{"no date", Session{}, ""},
		{"a series opens its next night", Session{ScheduledDate: strp("2026-09-24"), IsRecurring: true, RecurrenceType: strp(RecurrenceWeekly)}, "2026-10-08"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sidebarNightDate(tt.s, "2026-10-03"); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSessionPlanFrom(t *testing.T) {
	tests := []struct {
		name            string
		date, clock, tz string
		want            SessionPlan
	}{
		{"whole plan", "2026-10-08", "19:00", "America/Chicago", SessionPlan{"2026-10-08", "19:00", "America/Chicago"}},
		{"no date, no plan", "", "19:00", "America/Chicago", SessionPlan{}},
		{"bad date, no plan", "Thursday", "19:00", "", SessionPlan{}},
		{"bad time dropped", "2026-10-08", "7pm", "", SessionPlan{Date: "2026-10-08"}},
		{"unknown zone dropped", "2026-10-08", "19:00", "Mars/Olympus", SessionPlan{Date: "2026-10-08", Time: "19:00"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sessionPlanFrom(tt.date, tt.clock, tt.tz); got != tt.want {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}
