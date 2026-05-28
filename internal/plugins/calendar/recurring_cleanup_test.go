// recurring_cleanup_test.go pins the Q-V2-6 cleanup: the calendar
// repository's event-listing SQL must NOT contain partial-yearly
// expansion (the OR-branch that matched yearly recurring events
// across years by recurrence_type). V3 will ship unified recurring
// expansion; until then, recurring events must surface exactly once
// at their stored (year, month, day) like any other event.
//
// This is a source-level pin (reads the repository.go file as text)
// because the SQL is built via fmt.Sprintf — a runtime DB integration
// test would catch this too but adds MariaDB to the test path. The
// source-level pin is cheap, deterministic, and zero-flake.

package calendar

import (
	"os"
	"strings"
	"testing"
)

// TestRecurringYearlyExpansion_NotInRepoSQL — pin that the partial-
// yearly SQL handling cleaned up in C-CAL-RECURRING-PARTIAL-STATE-
// CLEANUP does not return. Specifically, no listing query should
// branch on `recurrence_type = 'yearly'` to expand stored events
// across years.
//
// If V3 reintroduces a proper expansion engine, that engine will
// (per Q-V2-6 plan) live in a dedicated expander, not via inline SQL
// match logic. If you're failing this test by adding a unified
// expander, replace the pinned substring with the new expander's
// invariant.
func TestRecurringYearlyExpansion_NotInRepoSQL(t *testing.T) {
	data, err := os.ReadFile("repository.go")
	if err != nil {
		t.Fatalf("read repository.go: %v", err)
	}
	src := string(data)

	// The two distinctive patterns of the partial-yearly handling
	// (both removed in this cleanup). Either reappearing is the
	// regression signal.
	forbidden := []string{
		`is_recurring = 1 AND e.recurrence_type = 'yearly'`,
		`is_recurring = 1 AND recurrence_type = 'yearly'`,
	}
	for _, pat := range forbidden {
		if strings.Contains(src, pat) {
			t.Errorf("repository.go contains forbidden partial-yearly SQL pattern %q — see Q-V2-6 resolution at decisions/2026-05-28-cal-timeline-v2-design.md. V3 will ship unified expansion; do not reintroduce inline SQL handling.", pat)
		}
	}
}
