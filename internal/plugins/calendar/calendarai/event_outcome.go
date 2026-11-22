package calendarai

import (
	"strings"

	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

// outcomeDoc is the event kind's paragraph on what became of an event.
const outcomeDoc = "What became of an event: `status` is `planned`, `happened`, `moved` or `called_off` (the calendar's own four words; an event nobody answers stays planned and reads as happened once its day has passed). `moved` goes with `move_to_year`/`move_to_month`/`move_to_day` on `action: update`, and the calendar keeps a mark on the day it was planned for. `what_happened` is a line on what happened, plain text; `what_happened: none` clears it. Both are for one-off events: a repeating event's dates are skipped or moved one at a time on the calendar. Players see them only on events they can already see."

// outcomeSpec is a row's status and line, as read.
type outcomeSpec struct {
	status      string // "" when the row leaves it out
	line        string
	lineSet     bool
	lineCleared bool
}

func (o outcomeSpec) set() bool { return o.status != "" || o.lineSet }

// statusWords are what a summary says each status does.
var statusWords = map[string]string{
	calendar.EventStatusPlanned:   "marks it planned",
	calendar.EventStatusHappened:  "marks it happened",
	calendar.EventStatusMoved:     "marks it moved",
	calendar.EventStatusCalledOff: "calls it off",
}

// readOutcome reads `status` and `what_happened`. A status is matched
// loosely ("Called off" is called_off) so the AI may write it as a person
// would, and stored as the calendar's own word.
func readOutcome(r Record) (outcomeSpec, error) {
	var o outcomeSpec
	if r.Has("status") {
		raw := strings.ToLower(strings.Join(strings.Fields(strings.ReplaceAll(r.Str("status"), "_", " ")), "_"))
		if !calendar.IsEventStatus(raw) {
			return o, badRequestf("status must be planned, happened, moved or called_off")
		}
		o.status = raw
	}
	if r.Has("what_happened") {
		o.lineSet = true
		o.line = r.Str("what_happened")
		switch {
		case strings.EqualFold(o.line, pageNone):
			if r.Action != ActionUpdate {
				return o, badRequestf("what_happened: none only applies to action: update; leave it out instead")
			}
			o.lineCleared, o.line = true, ""
		case o.line == "":
			return o, badRequestf("what_happened needs a line, or none to clear it")
		case len([]rune(o.line)) > calendar.MaxOutcomeChars:
			return o, badRequestf("what_happened can be up to %d characters", calendar.MaxOutcomeChars)
		}
	}
	return o, nil
}

// check refuses what the calendar would: a status on a repeating event, a
// move on a new event, and a move with no new date.
func (o outcomeSpec) check(r Record, repeating, moving bool) error {
	if !o.set() {
		return nil
	}
	if r.Action == ActionDelete {
		return badRequestf("status and what_happened don't go with action: delete")
	}
	if repeating && (o.status != "" && o.status != calendar.EventStatusPlanned || o.lineSet && !o.lineCleared) {
		return badRequestf("status and what_happened are for one-off events; a repeating event's dates are skipped or moved one at a time on the calendar")
	}
	if o.status == calendar.EventStatusMoved {
		if r.Action == ActionCreate {
			return badRequestf("a new event can't be moved yet; add it on its day, then move it with action: update")
		}
		if !moving {
			return badRequestf("status: moved needs move_to_year, move_to_month and move_to_day")
		}
	}
	return nil
}

// summary is what the review list says the row does to the event's outcome.
func (o outcomeSpec) summary() string {
	var parts []string
	if o.status != "" {
		parts = append(parts, statusWords[o.status])
	}
	switch {
	case o.lineCleared:
		parts = append(parts, "clears what happened")
	case o.lineSet:
		parts = append(parts, "notes what happened: "+quote(o.line))
	}
	return strings.Join(parts, ", ")
}

func withOutcome(summary string, o outcomeSpec) string {
	if s := o.summary(); s != "" {
		return summary + ", " + s
	}
	return summary
}

func (o outcomeSpec) toCreate(in *calendar.CreateEventInput) {
	in.Status = o.status
	if o.lineSet && !o.lineCleared {
		line := o.line
		in.Outcome = &line
	}
}

func (o outcomeSpec) toUpdate(in *calendar.UpdateEventInput) {
	if o.status != "" {
		in.Status = patch.Of(o.status)
	}
	switch {
	case o.lineCleared:
		in.Outcome = patch.Null[string]()
	case o.lineSet:
		in.Outcome = patch.Of(o.line)
	}
}

// outcomeText is how a lookup line says what became of an event: nothing
// for a planned one, so an unanswered calendar reads as it always did.
func outcomeText(cal *calendar.Calendar, e calendar.Event) string {
	var s string
	switch e.Status {
	case calendar.EventStatusHappened:
		s = ", status happened"
	case calendar.EventStatusCalledOff:
		s = ", status called_off"
	case calendar.EventStatusMoved:
		s = ", status moved"
		if e.MovedFromYear != nil && e.MovedFromMonth != nil && e.MovedFromDay != nil {
			s += " (planned for " + dateLabel(cal, *e.MovedFromYear, *e.MovedFromMonth, *e.MovedFromDay) + ")"
		}
	}
	if e.Outcome != nil && *e.Outcome != "" {
		s += ", what happened: " + quote(*e.Outcome)
	}
	return s
}
