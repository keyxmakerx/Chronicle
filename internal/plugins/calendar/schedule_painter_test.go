package calendar

// schedule_painter_test.go — S4, the ONE surface on this page that writes
// (C-CALV4-RSVP-P8 Part B).
//
// THE WHOLE POINT OF THESE TESTS IS THAT PART B ADDS NO WRITE ROUTE. The
// painted grid posts through the SCHEDULER'S OWN shipped endpoints, and the two
// scopes go to two different ones because they mean two different things:
//
//	Every week      → PUT /campaigns/:id/availability/mine        (normal hours)
//	This week only  → PUT /campaigns/:id/availability/exceptions  (date override)
//
// Forking a second availability write path would fork the composition invariant
// with it — "an offer only ever adds, and never downgrades an hour already
// marked preferred" is the scheduler's own rule, enforced in its service, and a
// second writer is a second place to get it wrong.

import (
	"strings"
	"testing"
)

func schedulePainterInput(isGM bool) scheduleBuildInput {
	in := scheduleOracleInput(isGM)
	in.ViewerID = "u-kael"
	// THE PAINTER IS SEEDED FROM THE OWN-WEEK READ, not from the overlay — the
	// fixture reproduces that for BOTH roles, because a player's overlay carries
	// no member at all and their own week has to arrive some other way.
	in.OwnLanes = scheduleOracleAvail().Lanes["u-kael"]
	return in
}

// THE TWO SCOPES ARE TWO DIFFERENT SENTENCES, AND TWO DIFFERENT TABLES.
func TestSchedulePainter_ScopeSegmentNamesWhatItWrites(t *testing.T) {
	week := scheduleBuildPainter(schedulePainterInput(false))
	if week.Form == nil {
		t.Fatal("the Painter built no form")
	}
	// The note is a run sequence — its WORDS are what this asserts; the drawn
	// emphasis on the table's name is pinned in schedule_stills_test.go.
	if !strings.Contains(week.Form.ScopeNote.Text(), "date exception") {
		t.Errorf("the `this week only` note does not say it replaces that day's usual "+
			"pattern: %q", week.Form.ScopeNote.Text())
	}

	in := schedulePainterInput(false)
	in.Scope = "recurring"
	every := scheduleBuildPainter(in)
	if !strings.Contains(every.Form.ScopeNote.Text(), "normal hours") {
		t.Errorf("the `every week` note does not say it sets the member's normal hours: %q",
			every.Form.ScopeNote.Text())
	}
	if week.Form.ScopeNote.Text() == every.Form.ScopeNote.Text() {
		t.Error("both scopes print the same sentence — the control would be decorative")
	}
}

// PART B ADDS NO WRITE ROUTE. Both targets are the scheduler's shipped ones.
func TestSchedulePainter_WritesThroughTheShippedSchedulerRoutes(t *testing.T) {
	p := scheduleBuildPainter(schedulePainterInput(false))
	if got, want := p.Form.SaveURL, "/campaigns/camp-1/availability/mine"; got != want {
		t.Errorf("recurring save target = %q, want the shipped %q", got, want)
	}
	if got, want := p.Form.ExceptionsURL, "/campaigns/camp-1/availability/exceptions"; got != want {
		t.Errorf("per-date save target = %q, want the shipped %q", got, want)
	}
	if strings.Contains(p.Form.SaveURL, "/schedule") {
		t.Error("the Painter invented a route under its own namespace")
	}
}

// THE GRID IS THE MEMBER'S OWN WEEK, SEEDED FROM WHAT THEY ALREADY SAVED, and
// the preferred grid is a SUBSET of the available one — the scheduler composes
// them, so marking an hour preferred also marks it playable.
func TestSchedulePainter_SeedsFromTheViewersOwnLanes(t *testing.T) {
	p := scheduleBuildPainter(schedulePainterInput(false))
	if len(p.Form.Days) == 0 {
		t.Fatal("the Painter drew no day rows")
	}
	checked, prefChecked := 0, 0
	for _, d := range p.Form.Days {
		for _, c := range d.Cells {
			if c.Checked {
				checked++
			}
		}
	}
	for _, d := range p.Form.PrefDays {
		for _, c := range d.Cells {
			if c.Checked {
				prefChecked++
			}
		}
	}
	if checked == 0 {
		t.Error("the viewer's own saved week did not seed the grid")
	}
	if prefChecked == 0 {
		t.Error("the viewer's own preferred hours did not seed the WOULD PREFER grid")
	}
	if prefChecked > checked {
		t.Errorf("preferred (%d) exceeds available (%d) — preferred always sits INSIDE "+
			"available, and the server composes them", prefChecked, checked)
	}
}

// EVERY TICK CARRIES ITS OWN WEEKDAY AND HOUR. The driver has to send a weekday
// index for the recurring path and a DATE for the exception path, and deriving
// either in JS from a printed label is how a Monday becomes a Sunday.
func TestSchedulePainter_EveryTickCarriesItsOwnDateAndWeekday(t *testing.T) {
	p := scheduleBuildPainter(schedulePainterInput(false))
	for _, d := range p.Form.Days {
		if d.DayKey == "" {
			t.Fatalf("day row %q carries no date key", d.Label)
		}
		if d.Weekday < 0 || d.Weekday > 6 {
			t.Errorf("day row %q carries weekday %d, outside 0..6", d.Label, d.Weekday)
		}
		for _, c := range d.Cells {
			if c.DayKey != d.DayKey {
				t.Errorf("tick %q carries date %q on a row dated %q", c.Value, c.DayKey, d.DayKey)
			}
			if c.Hour < 0 || c.Hour > 23 {
				t.Errorf("tick %q carries hour %d", c.Value, c.Hour)
			}
			if c.Label == "" {
				t.Errorf("tick %q has no accessible name", c.Value)
			}
		}
	}
}

// AN UNBUILT AFFORDANCE IS DIRECTOR-TIER, AND A PLAYER SIMPLY DOES NOT GET IT.
//
// "Copy last week" has no endpoint behind it. On the Director's page it ships
// disabled with a VISIBLE chip, because the Director is the person being asked
// to sign off on what is missing. On a player's page it is ABSENT — not
// disabled, not greyed, not a tooltip. A dead button whose only explanation is a
// hover is the same failure the disabled primary button exists to avoid, and
// scaffolding for a gap nobody is asking that player about is Director-tier:
// permission is absence, and it binds scaffolding too.
func TestSchedulePainter_UnbuiltScaffoldingIsDirectorOnly(t *testing.T) {
	gm := scheduleBuildPainter(schedulePainterInput(true))
	pl := scheduleBuildPainter(schedulePainterInput(false))

	if gm.Form.CopyWeek == nil {
		t.Fatal("the Director lost the disabled `Copy last week` control")
	}
	if !gm.Form.CopyWeek.NeedsBackend {
		t.Error("`Copy last week` renders without its visible chip — `title` may not be the " +
			"sole carrier of a disabled control's reason")
	}
	if pl.Form.CopyWeek != nil {
		t.Error("a player received unbuilt scaffolding — it must be ABSENT, not disabled")
	}
	if gm.Form.Reserve == nil {
		t.Error("the Director lost the ONE dashed reserve on this page")
	}
	if pl.Form.Reserve != nil {
		t.Error("a player received a dashed `not built yet` band")
	}
}

// THE COMPOSE RULE IS PRINTED, because it is the one thing a member cannot work
// out from the grid: offering a window only ever ADDS.
func TestSchedulePainter_PrintsTheComposeRule(t *testing.T) {
	p := scheduleBuildPainter(schedulePainterInput(false))
	if !strings.Contains(p.Form.Foot.Text(), "only ever adds") {
		t.Errorf("the Painter does not print the compose rule: %q", p.Form.Foot.Text())
	}
	if !strings.Contains(p.Form.Summary, "8 windows") {
		t.Errorf("the Painter does not print the offer-path cap: %q", p.Form.Summary)
	}
}

// A ZONE-LESS MEMBER CANNOT PAINT A WEEK, and the refusal says why rather than
// silently saving a guess. Availability is a zone-local wall clock; saving it
// against a zone nobody set would store a guess every reader's conversion then
// inherits.
func TestSchedulePainter_RefusesWithoutAZone(t *testing.T) {
	in := schedulePainterInput(false)
	in.Zone, in.ZoneLeaf = "", ""
	p := scheduleBuildPainter(in)
	if p.Fault == nil {
		t.Fatal("a zone-less member was offered a grid to paint")
	}
	if p.Form != nil {
		t.Error("the refusal and the form both rendered")
	}
	if !strings.Contains(p.Fault.Detail, "wall clock") {
		t.Errorf("the refusal does not say why: %q", p.Fault.Detail)
	}
}
