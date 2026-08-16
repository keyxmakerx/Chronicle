package sessions

import (
	"time"

	"github.com/keyxmakerx/chronicle/internal/timeutil"
)

// Week cadence for recurring availability (C-RSVP-P9).
//
// Until now a stored block was "this weekday, every week, forever" and nothing
// else was expressible. Real groups do not all work that way: alternating-week
// games are common, and the only way to express one was to hand-punch a
// per-date exception every fortnight — a chore nobody sustains, which meant the
// heatmap slowly drifted away from the truth.
//
// A block therefore carries a CADENCE: every week, or one of two alternating
// tracks. The tracks are deliberately NOT called "odd/even" anywhere a person
// can see, because odd-versus-even is only meaningful once you know what is
// being counted; the UI names them by an actual date ("week of 16 Aug") that it
// derives from these functions.

// Week cadence values, stored in member_availability.week_parity.
//
// CadenceEveryWeek is 0 so that the column's DEFAULT 0 makes every row written
// before this feature existed mean exactly what it meant then — every week.
// That is the whole reason 0 is not one of the two tracks.
const (
	CadenceEveryWeek = 0
	CadenceWeekA     = 1
	CadenceWeekB     = 2
)

// cadenceEpoch is the Sunday alternating weeks are counted from.
//
// 1970-01-04 is a Sunday, which matches day_of_week's 0=Sunday convention, so a
// week here is the same week the rest of the availability code already means.
// The epoch is fixed and global ON PURPOSE: if each campaign (or each member)
// counted from its own start, two members in the same campaign could disagree
// about which track a given real week is, and the overlay would silently
// combine incompatible answers. One epoch for everyone makes "week A" a fact
// about the calendar rather than about the person answering.
var cadenceEpoch = time.Date(1970, time.January, 4, 0, 0, 0, 0, time.UTC)

// ValidWeekCadence reports whether v is a cadence the store accepts.
func ValidWeekCadence(v int) bool {
	return v == CadenceEveryWeek || v == CadenceWeekA || v == CadenceWeekB
}

// weekIndexOf returns how many whole Sunday-started weeks separate d from the
// epoch. Negative for dates before 1970, which is why the modulo below is
// normalised rather than trusted.
func weekIndexOf(d timeutil.CivilDate) int {
	utcMidnight := time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC)
	days := int(utcMidnight.Sub(cadenceEpoch).Hours() / 24)
	// Floor division, not truncation: Go's / rounds toward zero, so a date
	// before the epoch would land in the wrong week and the two tracks would
	// swap over on 1970-01-04. Nobody schedules a game in 1969, but a wrong
	// answer that only shows up for one input is exactly the kind of thing that
	// survives review, so it is handled rather than assumed away.
	if days < 0 {
		days -= 6
	}
	return days / 7
}

// WeekCadenceFor returns which alternating track the real date d falls in —
// CadenceWeekA or CadenceWeekB. It never returns CadenceEveryWeek: "every week"
// is a property a BLOCK can have, not a property a date can have.
func WeekCadenceFor(d timeutil.CivilDate) int {
	if weekIndexOf(d)%2 == 0 {
		return CadenceWeekA
	}
	return CadenceWeekB
}

// cadenceApplies reports whether a block stored with cadence blockCadence is in
// force on the real date d.
//
// An unrecognised stored value applies EVERY week. That direction is chosen
// deliberately: the failure mode of "applies too often" is a member shown as
// free when they meant alternate weeks, which they can see and correct on their
// own grid; the failure mode of "applies never" is availability that silently
// vanishes from the heatmap with nothing on any screen to explain it.
func cadenceApplies(blockCadence int, d timeutil.CivilDate) bool {
	switch blockCadence {
	case CadenceWeekA, CadenceWeekB:
		return blockCadence == WeekCadenceFor(d)
	default:
		return true
	}
}

// CadenceLabel names a track by a real date inside it — the Sunday that starts
// the first such week on or after from. The UI shows "week of 16 Aug", never
// "odd weeks", so the member is choosing between two weeks they can point at on
// a calendar instead of decoding a convention.
func CadenceLabel(cadence int, from timeutil.CivilDate) timeutil.CivilDate {
	// Walk back to the Sunday that starts `from`'s own week, then forward a
	// week at a time until the track matches. At most one step is ever needed,
	// but the loop states the rule instead of encoding "the other one is +7".
	sunday := from.AddDays(-int(from.Weekday()))
	for i := 0; i < 2; i++ {
		if WeekCadenceFor(sunday) == cadence {
			return sunday
		}
		sunday = sunday.AddDays(7)
	}
	return sunday
}
