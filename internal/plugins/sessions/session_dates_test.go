package sessions

import "testing"

// TestNormalizeDates pins the scanned-DATE cleanup: with parseTime on, the
// driver hands DATE columns to a string as RFC 3339, which the night
// expansion and the date tiles cannot parse.
func TestNormalizeDates(t *testing.T) {
	str := func(v string) *string { return &v }
	tests := []struct {
		name string
		in   *string
		want *string
	}{
		{"nil stays nil", nil, nil},
		{"plain date kept", str("2026-10-14"), str("2026-10-14")},
		{"rfc3339 cut", str("2026-10-14T00:00:00Z"), str("2026-10-14")},
		{"empty kept", str(""), str("")},
		{"non-date kept", str("not a date at all"), str("not a date at all")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Session{ScheduledDate: tt.in, RecurrenceEndDate: tt.in}
			s.normalizeDates()
			for _, got := range []*string{s.ScheduledDate, s.RecurrenceEndDate} {
				if (got == nil) != (tt.want == nil) || (got != nil && *got != *tt.want) {
					t.Fatalf("got %v, want %v", deref(got), deref(tt.want))
				}
			}
		})
	}

	// A weekly series anchored on a scanned date expands past its first night.
	s := Session{ScheduledDate: str("2026-10-14T00:00:00Z"), IsRecurring: true, RecurrenceType: str(RecurrenceWeekly)}
	s.normalizeDates()
	if got := occurrenceDates(s, "2026-10-01", "2026-10-31"); len(got) != 3 {
		t.Fatalf("weekly series nights = %v, want 3", got)
	}
}

func deref(v *string) string {
	if v == nil {
		return "<nil>"
	}
	return *v
}
