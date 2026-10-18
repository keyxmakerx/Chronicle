package sessions

import (
	"reflect"
	"testing"
)

// A monthly series started on the 31st must clamp to short months and come
// back to the 31st, not drift to the 3rd.
func TestMonthlyRecurrenceFromThe31st(t *testing.T) {
	monthly := RecurrenceMonthly
	start := "2025-01-31"
	sess := Session{ScheduledDate: &start, IsRecurring: true, RecurrenceType: &monthly}

	t.Run("occurrenceAt counts from the series start", func(t *testing.T) {
		tests := []struct {
			n    int
			want string
		}{
			{0, "2025-01-31"}, {1, "2025-02-28"}, {2, "2025-03-31"}, {3, "2025-04-30"},
			{4, "2025-05-31"}, {11, "2025-12-31"}, {13, "2026-02-28"}, {37, "2028-02-29"},
		}
		for _, tt := range tests {
			if got := occurrenceAt(&sess, tt.n); got != tt.want {
				t.Errorf("occurrenceAt(%d) = %s, want %s", tt.n, got, tt.want)
			}
		}
	})

	t.Run("computeNextOccurrence clamps instead of overflowing", func(t *testing.T) {
		if got := computeNextOccurrence(&sess); got != "2025-02-28" {
			t.Errorf("next = %s, want 2025-02-28", got)
		}
	})

	t.Run("occurrenceDates", func(t *testing.T) {
		want := []string{"2025-01-31", "2025-02-28", "2025-03-31", "2025-04-30", "2025-05-31"}
		if got := occurrenceDates(sess, "2025-01-01", "2025-05-31"); !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("nextUpcomingOccurrenceDate", func(t *testing.T) {
		tests := []struct{ today, want string }{
			{"2025-02-01", "2025-02-28"},
			{"2025-03-01", "2025-03-31"},
			{"2025-04-01", "2025-04-30"},
		}
		for _, tt := range tests {
			if got := nextUpcomingOccurrenceDate(&sess, tt.today); got != tt.want {
				t.Errorf("today %s: got %s, want %s", tt.today, got, tt.want)
			}
		}
	})
}
