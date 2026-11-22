package calendarai

import (
	"strings"

	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

// eventTimes are an event row's start and end times as the block wrote them;
// a nil value was left out.
type eventTimes struct {
	startH, startM, endH, endM *int
	clearEnd                   bool // `end_hour: none`
}

// readEventTimes reads `hour`, `minute`, `end_hour` and `end_minute`. A time
// outside the calendar's own day is refused here, as the service would at
// apply, so the review row says so first.
func readEventTimes(cal *calendar.Calendar, r Record) (eventTimes, error) {
	var t eventTimes
	for _, p := range []struct {
		key  string
		dst  **int
		hour bool
	}{{"hour", &t.startH, true}, {"minute", &t.startM, false}, {"end_hour", &t.endH, true}, {"end_minute", &t.endM, false}} {
		if p.key == "end_hour" && strings.EqualFold(r.Str(p.key), pageNone) {
			if r.Action != ActionUpdate {
				return t, badRequestf("end_hour: none only applies to action: update; leave the end time out instead")
			}
			t.clearEnd = true
			continue
		}
		n, ok, err := r.Int(p.key)
		if err != nil {
			return t, err
		}
		if !ok {
			continue
		}
		limit, what := cal.MinutesPerHour, "hour"
		if p.hour {
			limit, what = cal.HoursPerDay, "day"
		}
		if n < 0 || (limit > 0 && n >= limit) {
			return t, badRequestf("%s: %d is outside this calendar's %s", p.key, n, what)
		}
		*p.dst = &n
	}
	if t.clearEnd && r.Has("end_minute") {
		return t, badRequestf("end_hour: none clears the end time; leave end_minute out")
	}
	if t.endM != nil && t.endH == nil {
		return t, badRequestf("end_minute needs end_hour")
	}
	return t, nil
}

// backwards reports an end time before the start time on a one-day span,
// the same refusal the service makes. start and end are the effective dates,
// startH/endH the effective hours (the block's, else the stored event's).
func backwards(start, end calDate, startH, startM, endH, endM *int) bool {
	if start != end || startH == nil || endH == nil {
		return false
	}
	at := func(h, m *int) int {
		if m == nil {
			return *h * 60
		}
		return *h*60 + *m
	}
	return at(endH, endM) < at(startH, startM)
}

// order is the plan-time check that an end time doesn't precede the start.
// existing is the stored event on an update; day is the block's start date
// (or its move_to date).
func (t eventTimes) order(existing *calendar.Event, day calDate, ended bool, end calDate) error {
	if t.startH == nil && t.endH == nil && t.startM == nil && t.endM == nil {
		return nil
	}
	startH, startM, endH, endM := t.startH, t.startM, t.endH, t.endM
	endDay := day
	if ended {
		endDay = end
	}
	if existing != nil {
		if startH == nil {
			startH = existing.StartHour
			if t.startM == nil {
				startM = existing.StartMinute
			}
		}
		if endH == nil && !t.clearEnd {
			endH = existing.EndHour
			if t.endM == nil {
				endM = existing.EndMinute
			}
		}
		if !ended && existing.EndYear != nil && existing.EndMonth != nil && existing.EndDay != nil {
			endDay = calDate{*existing.EndYear, *existing.EndMonth, *existing.EndDay}
		}
	}
	if t.clearEnd {
		endH = nil
	}
	if backwards(day, endDay, startH, startM, endH, endM) {
		return badRequestf("the event ends before it starts")
	}
	return nil
}
