// bench_gmconsole.go — the GM world-state CONSOLE's producer on the Bench
// (C-CALV4-GM-CONSOLE).
//
// THIS IS A REHOUSING, NOT A FEATURE. Every control below already exists and
// already writes: `PUT /campaigns/:id/calendar/world-state` is unchanged, its
// `RequireCapability(CanControlWorldState)` gate is unchanged, and no route,
// capability or migration is added. What moves is the DOOR — the console's only
// mount today is `calendar_v2.templ:152`, on the page `C-CALV4-V2SUNSET` [VS-1]
// (SIGNED) has committed to deleting, so without this slice the sunset takes
// nine live GM controls with it and the Bench is left with the two date verbs
// C-CALV4-GAMEREADY §2 landed.
//
// ── WHICH CALENDAR, AND WHY IT IS NAMED RATHER THAN INFERRED ─────────────────
//
// The Bench stacks up to TWO Blocks (primary + real-world), so "the calendar"
// is ambiguous on this page in a way it never was on the V2 shell. The console
// drives the PRIMARY calendar and says so on the wire: both URLs carry an
// explicit `?calendarId=`, exactly as benchDateVerbs already does.
//
// LEANING ON THE ENDPOINT'S ACTIVE-CALENDAR FALLBACK WOULD BE A TRAP, and it is
// one the sunset arms. `resolveWorldStateCalendar` falls back to the viewer's
// ACTIVE calendar, and the only UI that can change which calendar that is —
// `POST /calendar/v2/switch` — dies with the shell. A console that relied on the
// fallback would therefore drive a row the user can no longer change, for the
// rest of the campaign's life. (Booked as C-CALV4-BENCH-CALID.)
//
// ── THE TWO FLOORS, AND THE ONE THIS SLICE CHOSE ─────────────────────────────
//
// The V2 console gates Set time and Set date on CanControlWorldState (Owner OR
// co-DM). The Bench's own date-verb row gates Set date on the STRICTER
// Owner-only floor and steps on CanControlWorldState — "a co-DM steps and does
// not set", [GR-SIGN-A](b) SIGNED, which BenchDateVerbs' header calls the
// shipped asymmetry by name.
//
// REHOUSING PUTS BOTH ON ONE PAGE, so one of them has to give: a co-DM must not
// see two Set-date controls a foot apart that answer differently. This slice
// takes the STRICTER of the two shipped floors for the ABSOLUTE SETTERS —
// CanSetAbsolute is `in.IsOwner`, the same predicate BenchDateVerbs.CanSet
// reads — and leaves the STEP verbs on CanControlWorldState, the same predicate
// BenchDateVerbs.CanStep reads. The split is "step vs set", which is exactly the
// distinction [GR-SIGN-A](b) drew, applied to the clock as well as the date.
//
// THE DIRECTION IS DELIBERATE AND IT IS THE SAFE ONE. Narrowing costs a co-DM
// two controls they hold on the doomed V2 page and can still reach by stepping;
// widening would move a floor the operator reserved ("WIDENING WHO MAY SET THE
// WORLD'S DATE IS THE OPERATOR'S CALL, not a side effect of a playability fix").
// No server floor moves in either direction — the PUT still accepts a co-DM's
// absolute set, because it always did.
//
// ── WHAT THE PRODUCER DOES *NOT* CARRY, AND WHY ──────────────────────────────
//
// NOT THE WORLD-STATE SEED. buildBench never calls BuildWorldStateSeed and this
// slice does not make it: the seed costs three reads (day weather, celestial
// events, moon phases) on a page every player lands on, for a console that
// defaults COLLAPSED and that most page loads never open. The markup instead
// declares `data-gm-standalone` and the driver GETs the seed from the same URL
// it PUTs to, ON FIRST OPEN — `GET /calendar/world-state` is already Player+
// and public-capable and returns the identical seed the PUT echoes back, so the
// active-weather ring and the active event chips are built from the server's
// answer rather than from a second producer that could disagree with it.
//
// NOT A REAL-TIME GATE. `guardManualDateChange` (service.go) refuses `advance`
// and `time` on a calendar that tracks real time while weather / celestial /
// mood stay authorable (RC-5), so six of these controls are refused 100% of the
// time on such a calendar. They are STILL RENDERED here, deliberately: the
// Bench's own date-verb row renders unconditionally on the same page, and
// hiding one while showing the other would ship the very inconsistency the
// floor decision above exists to remove. The server answers a named validation
// error the driver surfaces. Closing it for BOTH surfaces at once is booked as
// C-CALV4-REALTIME-DATE-ABSENCE.
package calendar

import "fmt"

// BenchGMConsole is the Bench's GM world-state console, resolved by
// benchGMConsole and rendered by benchGMConsoleView.
//
// PERMISSION IS ABSENCE, TWICE OVER. `Live` false means the console is not in
// the page's DOM at all — not hidden, not disabled, not a CSS rule — and
// CanAuthorDmOnly false means the dm_only checkbox is not in the console's DOM
// even for a viewer who holds everything else. Both mirror gates the server
// already enforces: RequireCapability(CanControlWorldState) on the route, and
// PutWorldState's own re-check of cc.CanAuthorDmOnly() before it stores
// storageVisibilityDMOnly. The markup gate is defence in depth, never the
// boundary.
type BenchGMConsole struct {
	// Live is the whole render gate: cc.CanControlWorldState() AND a primary
	// calendar to act on. A player, a Scribe and a logged-out visitor all
	// resolve it false.
	Live bool

	// CanAuthorDmOnly is the SECOND, distinct gate — cc.CanAuthorDmOnly() — and
	// it is its own field rather than Live reused. The two read the same
	// predicate today and the server checks them at different moments (the
	// route vs. inside the handler), so a shared field would move both the day
	// one of them moves. Same reasoning DayCardMount.CanRestrict is written up
	// under.
	CanAuthorDmOnly bool

	// CanSetAbsolute is the OWNER floor of the Time sheet (Set time / Set
	// date). See this file's header: it is BenchDateVerbs.CanSet's predicate,
	// chosen so the page cannot show a co-DM two Set-date controls with
	// different answers.
	CanSetAbsolute bool

	// URL is the EXISTING world-state endpoint with an explicit ?calendarId=,
	// and it is ONE field because it is one path: the console PUTs its writes to
	// it and GETs its opening seed from it (GET is Player+ and public-capable,
	// PUT is capability-gated — same route, two methods, two floors, already
	// shipped). No route is added, so internal/wire/routes_snapshot.txt is
	// untouched.
	URL  string
	CSRF string

	// Clock is the in-world time as the calendar's own HH:MM, rendered
	// server-side so the console's header reads correctly on first paint —
	// before any seed has been fetched. The driver repaints it from the seed
	// afterwards.
	Clock string

	// Year / Month / Day / Hour / Minute seed the Time sheet from the
	// CALENDAR'S OWN current date, never from the navigated month cursor: Set
	// date changes where the world IS, and prefilling it from wherever the GM
	// happened to browse would make one control mean different things on
	// different pages. Same rule, same words, as BenchDateVerbs.
	Year, Month, Day, Hour, Minute int

	// Months are the month NAMES in calendar order, for the Set-date <select>.
	// Names rather than the Month structs: the select needs a label and a
	// 1-based index and nothing else, and handing the templ the full struct
	// invites it to compute.
	Months []string
}

// benchGMConsole resolves the console for ONE calendar — the primary — or the
// zero value when this viewer may not drive the world state.
//
// IT COMPUTES NOTHING ABOUT TIME. Every verb's payload is built in the browser
// from the values below and the rollover (month end, year end, leap geometry,
// clamping) is the SERVER'S, in advanceClock + validateAdvanceMagnitude. A
// client that did its own arithmetic here would be a second definition of when
// a year turns over — the same reason benchDateVerbs computes nothing.
func benchGMConsole(cal *Calendar, in benchInput) BenchGMConsole {
	if cal == nil || !in.CanControlWorldState {
		return BenchGMConsole{}
	}
	months := make([]string, 0, len(cal.Months))
	for i := range cal.Months {
		months = append(months, cal.Months[i].Name)
	}
	return BenchGMConsole{
		Live:            true,
		CanAuthorDmOnly: in.CanAuthorDmOnly,
		CanSetAbsolute:  in.IsOwner,
		URL: fmt.Sprintf("/campaigns/%s/calendar/world-state?calendarId=%s",
			in.Campaign.ID, cal.ID),
		CSRF:   in.CSRFToken,
		Clock:  cal.FormatCurrentTime(),
		Year:   cal.CurrentYear,
		Month:  cal.CurrentMonth,
		Day:    cal.CurrentDay,
		Hour:   cal.CurrentHour,
		Minute: cal.CurrentMinute,
		Months: months,
	}
}
