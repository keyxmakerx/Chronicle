// months_edit_impact.go — C-CALV4-GAMEREADY §9 [GR-18], under [GR-SIGN-B]
// (SIGNED 2026-08-07: WARN, NEVER REFUSE).
//
// ── THE DEFECT, REPRODUCED BEFORE IT WAS FIXED ───────────────────────────────
//
// Events store `Month int` as a 1-based POSITION, and every projection resolves
// it positionally. `SetMonths` is delete-and-reinsert with no event
// reconciliation and no warning, and `ApplyImport` does the same. So editing the
// month list silently re-dates events that reference it.
//
// months_edit_repro_test.go ran this against a real MariaDB before a line of
// this file existed ([GR-17]: "a [READ] finding is a report before it is a
// bug"). BOTH HALVES REPRODUCED:
//
//	insert an intercalary month at position 5 → 3 of 4 events silently move
//	  one month later; ZERO are stranded
//	delete the last month                     → its events point at a position
//	  nothing renders
//
// ── WHY THE COUNT IS A BEFORE/AFTER COMPARISON AND NOT A BOUNDS CHECK ────────
//
// This is the half [GR-SIGN-B] was escalated to settle. A `month > len(months)`
// count catches only DELETION. INSERTION is the likelier edit — it is what a GM
// does while iterating on their calendar in the week before a game — and every
// shifted event still lands on a REAL month, just the wrong one, so a
// stranded-count reports ZERO while the operator's whole year has moved.
//
// A warning that reads 0 while the calendar silently shifts is worse than no
// warning, because it CERTIFIES THE DAMAGE. So the count compares each event's
// RESOLVED month name before against after, and the sentence states both
// numbers separately.
//
// ── AND WHAT THIS FILE MAY NOT DO ────────────────────────────────────────────
//
// [GR-18] binds: a COUNT and a SENTENCE. Zero migrations, zero data writes,
// NEVER a rewrite of a stored Month. The correct new month for a shifted event
// is not derivable — two months can swap names, a name can move, a renamed
// month is indistinguishable from a replaced one — which is exactly why this
// slice warns rather than "fixes", and why the real reconciliation is booked as
// C-CAL-MONTHS-RECONCILE rather than improvised here.
//
// LAYERING. The repository owns the SQL and still does delete-and-reinsert; the
// COUNT is service work (this file); the SENTENCE is rendered by a handler or a
// template. The repository does not compute the warning.
package calendar

import (
	"context"
	"fmt"
)

// MonthEditImpact is what a structural month edit would do to the events that
// already reference the list, expressed as the two numbers [GR-SIGN-B] named.
//
// The two are DISJOINT and counted differently on purpose:
//
//	Stranded — events whose stored position does not exist in the NEW list.
//	           A state: "no longer lands on a real month."
//	Shifted  — events that resolve in BOTH lists but to a different month
//	           NAME. A delta: it is only meaningful against the edit that
//	           caused it, which is why it is reported at the moment of the
//	           save and not on any standing surface.
type MonthEditImpact struct {
	Stranded int
	Shifted  int
}

// Any reports whether the edit disturbs anything at all. A save that changed
// nothing must say nothing — a warning the operator sees on every save is a
// warning they learn to dismiss.
func (m MonthEditImpact) Any() bool { return m.Stranded > 0 || m.Shifted > 0 }

// Sentence is the operator-facing wording, and it is the whole deliverable on
// their side. Empty when nothing moved.
//
// The two clauses are separate because they are separate facts and the reader
// can do something different about each: a stranded event needs its date
// re-picked, a shifted one may be exactly what the operator intended.
func (m MonthEditImpact) Sentence() string {
	strandedClause := fmt.Sprintf("%d events no longer land on a real month", m.Stranded)
	if m.Stranded == 1 {
		strandedClause = "1 event no longer lands on a real month"
	}
	shiftedClause := fmt.Sprintf("%d events now fall in a different month than before", m.Shifted)
	if m.Shifted == 1 {
		shiftedClause = "1 event now falls in a different month than before"
	}
	switch {
	case m.Stranded > 0 && m.Shifted > 0:
		return strandedClause + ", and " + shiftedClause + "."
	case m.Stranded > 0:
		return strandedClause + "."
	case m.Shifted > 0:
		return shiftedClause + "."
	}
	return ""
}

// MonthEditImpact computes what replacing a calendar's month list would do,
// WITHOUT writing anything. It is a pure read: two queries and arithmetic.
//
// It is exported so a caller that wants to warn BEFORE committing (a preview, a
// confirm step some later slice may add) can ask the same question the write
// path answers — one implementation, so a preview and a save can never disagree
// about how many events are about to move.
func (s *calendarService) MonthEditImpact(ctx context.Context, calendarID string, next []MonthInput) (MonthEditImpact, error) {
	before, err := s.repo.GetMonths(ctx, calendarID)
	if err != nil {
		return MonthEditImpact{}, err
	}
	events, err := s.repo.ListAllEvents(ctx, calendarID)
	if err != nil {
		return MonthEditImpact{}, err
	}
	return monthEditImpact(before, next, events), nil
}

// monthEditImpact is the arithmetic, split out so it is testable without a
// database and so the write path can reuse the read it already has.
//
// A calendar with NO months before the edit reports nothing: there is no
// "before" to have moved away from, and calling a first-ever month list a
// re-dating would fire on every calendar's creation.
func monthEditImpact(before []Month, next []MonthInput, events []Event) MonthEditImpact {
	var out MonthEditImpact
	if len(before) == 0 {
		return out
	}
	nameBefore := func(m int) string {
		if m < 1 || m > len(before) {
			return ""
		}
		return before[m-1].Name
	}
	nameAfter := func(m int) string {
		if m < 1 || m > len(next) {
			return ""
		}
		return next[m-1].Name
	}
	for i := range events {
		was, now := nameBefore(events[i].Month), nameAfter(events[i].Month)
		switch {
		case now == "":
			// STRANDED. Counted even when it was already stranded before the
			// edit: this number is the STATE the save leaves behind, which is
			// what the operator has to act on, not a delta they can argue with.
			out.Stranded++
		case was != "" && was != now:
			out.Shifted++
		}
	}
	return out
}

// StrandedEventCounts is the READ-ONLY standing surface's source ([GR-18]): how
// many events, per calendar in a campaign, currently point at a month position
// that calendar no longer has.
//
// It exists because the warning at the moment of the save is not enough. A GM
// who dismisses a toast — or whose structural edit came in over the sync API —
// has no other way to learn the state, and the events do not announce
// themselves: they simply stop rendering. This is the surface that says so
// after the fact.
//
// It reports STRANDED only. There is no "before" on a standing surface, so
// there is no honest shift number to print there.
func (s *calendarService) StrandedEventCounts(ctx context.Context, campaignID string) (map[string]int, error) {
	return s.repo.StrandedEventCounts(ctx, campaignID)
}
