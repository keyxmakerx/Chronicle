// schedule_sections.go — the five surfaces' view models and their pure
// builders (C-CALV4-RSVP-P8 Part B, §10's S1–S5).
//
// EVERY BUILDER HERE IS PURE. scheduleBuildInput arrives fully resolved from
// buildSchedule, so each surface can be reproduced in a test from the same
// visible set the viewer got — which is the only construction under which the
// count oracle can prove that the Director's number and the player's number are
// the same number computed the same way.
//
// ── EVERY NUMBER IS RECOMPUTED FROM THE VISIBLE ROSTER ────────────────────
//
// No stored RSVP aggregate reaches this page, for the reason P8A states at
// length: EventRSVPSummary.Counts is raw rows while the named lists drop
// ex-members, so a stored aggregate printed beside a membership-filtered name
// list is a counts-vs-names disagreement BY CONSTRUCTION, and it grows every
// time somebody leaves a campaign. A departed member holding a stored answer row
// therefore changes no number here and appears in no list.
package calendar

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

// scheduleQuorum is the number of members with saved availability below which
// the Verdict REFUSES to rank (WG-3). It is benchRsvpQuorum, shared rather than
// re-declared: two surfaces with two thresholds would be two answers to "is
// there enough data to say", which is the exact question this page exists for.
const scheduleQuorum = benchRsvpQuorum

// scheduleCandidateCount is how many ranked windows the Verdict prints. Three,
// as drawn. Everything below them lives in the "N more windows" popover, which
// is ordered BY DATE and prints its score — a list ordered by score would put
// the ranking's own tail in a second, competing order.
const scheduleCandidateCount = 3

// scheduleCandidateLength is the ranked window's length in hours. A session is
// not an hour, and printing a one-hour recommendation for a three-hour game is
// the kind of small lie that costs a surface its credibility.
const scheduleCandidateLength = 3

// scheduleAntisocialEarly / Late bound the local hours that take --warn ink,
// shared with the Bench for the same reason the quorum is: they are the two
// cases where getting it wrong wakes somebody at 5am.
const (
	scheduleAntisocialEarly = benchRsvpAntisocialEarly
	scheduleAntisocialLate  = benchRsvpAntisocialLate
)

// scheduleBuildInput is everything the five pure builders need.
type scheduleBuildInput struct {
	// IsGM is the ONLY permission input and it decides what is BUILT.
	IsGM       bool
	ViewerID   string
	CampaignID string
	CSRFToken  string

	// Roster is every member in the overlay's stable order. THE ORDER IS THE
	// IDENTITY KEY: a member answering may not move a single element.
	Roster []BenchRosterMember
	// Avail is the week overlay; nil when the sessions read is unavailable.
	Avail *BenchAvailability
	// Answers is the RAW STORED SET keyed by user id, ex-members included,
	// filtered exactly once — visibly, here — against the roster this page
	// actually prints.
	Answers map[string]string
	Session *BenchRsvpSession
	// EventID / CalendarID are the collecting session's, for the RSVP form's
	// action. Empty is normal: a campaign may have no session collecting.
	EventID    string
	CalendarID string

	Zone      string
	ZoneLeaf  string
	WeekStart time.Time
	BandFrom  int
	BandTo    int

	// Zoom is "week" or "day" and it decides what a COLUMN IS — a day or an
	// hour of one day. It is carried into the builder rather than read off
	// ScheduleData because every surface derived from the columns (the
	// aggregates, the bracket, the popover ids, the head's own frame) has to
	// agree with them, and a zoom that only the template knew about is a zoom
	// half the page does not obey. Day is the selected ISO date, already clamped
	// into the week by scheduleResolveDay.
	Zoom string
	Day  string

	// OwnLanes is the VIEWER'S OWN composed week, read through
	// ScheduleOwnWeekReader. The Painter is built from THIS and never from
	// Avail.Lanes, so a Director and a player paint from one path: a player's
	// overlay carries no member at all, and their own week has to come from a
	// read that is about them rather than about everybody.
	OwnLanes []BenchLaneSegment

	Cand     string
	Scope    string
	PrefOpen bool
	SugOpen  bool
	Base     url.Values

	MailConfigured bool
	AskState       ScheduleAskState
}

// scheduleChip is one honesty chip.
//
// IT ROUTES THROUGH scheduleNeed, ALWAYS. The 2026-07-27 needs-backend-audience
// ruling is that a `needs backend` chip NEVER renders to a player, and this page
// is the product's heaviest consumer of them — fifteen on one screen on the
// default Director view. Making the audience gate structural (the builder emits
// nothing) rather than habitual (a template branch) is what keeps that true.
type scheduleChip struct {
	Text string
	// Warn marks the `.badge.warn` register — a DEPLOYMENT fact (an
	// unconfigured mail server, a zone nobody set), never a build gap. Diluting
	// `.badge.need` with those would leave a "needs backend" chip over a backend
	// that WAS built, which is the inversion WG-8 retired.
	Warn bool
}

// scheduleNeed emits a `needs backend` chip for a Director and NOTHING for a
// player. Every honesty chip on this page goes through it.
func scheduleNeed(isGM bool, text string) []scheduleChip {
	if !isGM {
		return nil
	}
	return []scheduleChip{{Text: text}}
}

// scheduleChips concatenates chip groups, dropping empties.
func scheduleChips(groups ...[]scheduleChip) []scheduleChip {
	out := []scheduleChip{}
	for _, g := range groups {
		out = append(out, g...)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// --- identity ---------------------------------------------------------------

// scheduleMember is one roster member with everything the page prints about
// them already resolved: the identity pair, the answer word, the local clock.
//
// COLOUR IS NEVER LOAD-BEARING ALONE. Hue and a LOCKED DASH PATTERN both ride
// here, and the pattern is what survives filter:grayscale(1). This is why
// OverlayMember.Color is ignored: it is ten hex values with one channel
// (ledger #17).
type scheduleMember struct {
	Index   int
	UserID  string
	Name    string
	First   string
	Token   string
	Role    string
	IsCoDM  bool
	Host    bool
	Axis    string
	Pattern string

	// TZ empty is a FIRST-CLASS STATE. users.timezone is NULLABLE and every
	// resolver in the product falls back to "UTC", so a clock rendered for a
	// zone-less member is a guess presented as a fact. The repair is printed
	// instead, and it survives at every width.
	TZ       string
	ZoneLeaf string

	Answer string
	Tone   string
	// Lanes are this member's minute-accurate availability runs for the week,
	// NIL for a player's payload — the absence is in the data.
	Lanes []BenchLaneSegment
}

// scheduleMembers resolves the roster into printable members.
func scheduleMembers(in scheduleBuildInput) []scheduleMember {
	out := make([]scheduleMember, 0, len(in.Roster))
	for i, m := range in.Roster {
		hue, pattern := benchRsvpIdentity(i)
		row := scheduleMember{
			Index: i, UserID: m.UserID, Name: m.Name,
			First:   scheduleFirstName(m.Name),
			Token:   benchRsvpInitials(m.Name),
			Role:    benchRoleLabel(m.Role),
			IsCoDM:  m.IsCoDM,
			Host:    m.IsOwner,
			Axis:    hue,
			Pattern: pattern,
			TZ:      strings.TrimSpace(m.TZ),
		}
		row.ZoneLeaf = scheduleZoneLeaf(row.TZ)
		row.Answer, row.Tone = benchRsvpAnswerWord(in.Answers[m.UserID])
		if in.Avail != nil && in.Avail.Lanes != nil {
			row.Lanes = in.Avail.Lanes[m.UserID]
		}
		out = append(out, row)
	}
	return out
}

// scheduleFirstName is the reason sentence's and the popover's short form. A
// Director-only fact leaking through a SENTENCE would be the loudest oracle on
// the page precisely because it reads as innocuous copy, so every caller of this
// is gated on IsGM at the call site.
func scheduleFirstName(name string) string {
	if f := strings.Fields(name); len(f) > 0 {
		return f[0]
	}
	return name
}

// --- the availability arithmetic --------------------------------------------

// scheduleFreeAt reports whether a member is free at the TOP OF THE HOUR.
//
// THIS IS THE COARSE READ, and it disagrees with the minute-accurate lanes above
// it IN PLAIN SIGHT: the count lane wears "on the hour" permanently and the
// caption says why (ledger #7). Someone free from 18:30 raises their own lane at
// 18:30 and does not raise the 18:00 count. Hiding that disagreement would be
// the easy lie; printing it is the whole subject of the surface.
func scheduleFreeAt(m scheduleMember, day, hour int) bool {
	for _, g := range m.Lanes {
		if g.DayIndex == day && g.StartMinute <= hour*60 && hour*60 < g.EndMinute {
			return true
		}
	}
	return false
}

// schedulePrefAt is the same read for the PREFERRED subset. Preferred always
// sits inside available — the scheduler composes them — so a preferred hour is
// also a free one.
func schedulePrefAt(m scheduleMember, day, hour int) bool {
	for _, g := range m.Lanes {
		if g.DayIndex == day && g.State == AvailPreferred &&
			g.StartMinute <= hour*60 && hour*60 < g.EndMinute {
			return true
		}
	}
	return false
}

// scheduleHasAny reports whether a member saved ANY availability this week.
//
// LEDGER #3, AND IT IS THE PAGE'S MOST IMPORTANT DISTINCTION. A member with no
// lane cannot be told apart from one who is busy all week, so this page says
// exactly that, in NEUTRAL ink, where the shape would be — the tool does not
// know, and an unknown is not a fault. Calling it a refusal is the exact lie
// this surface exists to avoid.
func scheduleHasAny(m scheduleMember) bool { return len(m.Lanes) > 0 }

// scheduleFreeCount is the aggregate at (day, hour) — an AGGREGATE, so it is
// safe at every role. It is read from the overlay's own per-hour counts when the
// lanes are absent (a player), and recomputed from the lanes when they are
// present, and the two agree because the overlay produced both.
func scheduleFreeCount(in scheduleBuildInput, members []scheduleMember, day, hour int) int {
	if in.Avail != nil && day >= 0 && day < len(in.Avail.Days) {
		d := in.Avail.Days[day]
		if hour >= 0 && hour < len(d.Free) {
			return d.Free[hour]
		}
	}
	n := 0
	for _, m := range members {
		if scheduleFreeAt(m, day, hour) {
			n++
		}
	}
	return n
}

// schedulePreferCount is the same for the preferred subset.
func schedulePreferCount(in scheduleBuildInput, members []scheduleMember, day, hour int) int {
	if in.Avail != nil && day >= 0 && day < len(in.Avail.Days) {
		d := in.Avail.Days[day]
		if hour >= 0 && hour < len(d.Prefer) {
			return d.Prefer[hour]
		}
	}
	n := 0
	for _, m := range members {
		if schedulePrefAt(m, day, hour) {
			n++
		}
	}
	return n
}

// scheduleSavedCount is how many members have saved anything this week — the
// number the quorum is measured against.
//
// It prefers the overlay's own WithPattern, which is an aggregate and therefore
// available at EVERY role: a player's page must be able to say "3 of 5 members
// have saved a week" without ever holding a lane.
func scheduleSavedCount(in scheduleBuildInput, members []scheduleMember) int {
	if in.Avail != nil && in.Avail.WithPattern > 0 {
		return in.Avail.WithPattern
	}
	n := 0
	for _, m := range members {
		if scheduleHasAny(m) {
			n++
		}
	}
	return n
}

// scheduleAnswered is how many of the VISIBLE roster answered. Recomputed, never
// read from a stored tally.
func scheduleAnswered(members []scheduleMember) int {
	n := 0
	for _, m := range members {
		if m.Answer != "—" {
			n++
		}
	}
	return n
}

// scheduleDayDate resolves the ISO date of column d.
func scheduleDayDate(in scheduleBuildInput, d int) string {
	if in.Avail != nil && d >= 0 && d < len(in.Avail.Days) {
		return in.Avail.Days[d].Date
	}
	return in.WeekStart.AddDate(0, 0, d).Format("2006-01-02")
}

// scheduleDayCount is how many columns the week has. It is the OVERLAY's own
// length wherever there is one, never a literal 7 — a calendar's week length is
// its own business and this page does not assume the Gregorian one.
func scheduleDayCount(in scheduleBuildInput) int {
	if in.Avail != nil && len(in.Avail.Days) > 0 {
		return len(in.Avail.Days)
	}
	return 7
}

// scheduleDayZoom reports whether THIS render draws one day's hours rather than
// the week's days.
//
// It is a function of the request and the overlay together, never of ?zoom
// alone: a day view needs a day that is actually IN the week the overlay
// returned, and a ?day the overlay cannot place is a query for a column that
// does not exist. scheduleSelectedDay resolves that index, so the two can never
// disagree about which day is on screen.
func scheduleDayZoom(in scheduleBuildInput) bool {
	return in.Zoom == "day" && scheduleSelectedDay(in) >= 0
}

// scheduleSelectedDay is the index of ?day among the overlay's own days, or -1
// when it names no day this week.
//
// IT MATCHES ON THE OVERLAY'S DATES, not on arithmetic from the Monday: the
// overlay is the authority on what days this week HAS (a calendar's week length
// is its own business), and deriving the index by subtraction would put the day
// view one column off on any calendar whose week is not seven Gregorian days.
func scheduleSelectedDay(in scheduleBuildInput) int {
	want := strings.TrimSpace(in.Day)
	if want == "" {
		return -1
	}
	for d, n := 0, scheduleDayCount(in); d < n; d++ {
		if scheduleDayDate(in, d) == want {
			return d
		}
	}
	return -1
}

// scheduleLocalHour converts the page's hour into a member's own local hour,
// returning ok=false when they have no zone (no clock is printed AT ALL) or the
// page has no zone to convert FROM.
func scheduleLocalHour(in scheduleBuildInput, m scheduleMember, day, hour int) (h int, nextDay bool, ok bool) {
	if m.TZ == "" || in.Zone == "" {
		return 0, false, false
	}
	from, err := time.LoadLocation(in.Zone)
	if err != nil {
		return 0, false, false
	}
	to, err := time.LoadLocation(m.TZ)
	if err != nil {
		return 0, false, false
	}
	date := in.WeekStart.AddDate(0, 0, day)
	at := time.Date(date.Year(), date.Month(), date.Day(), hour, 0, 0, 0, from)
	local := at.In(to)
	return local.Hour(), local.Day() != at.In(from).Day(), true
}

// scheduleAntisocial reports whether a local hour wakes somebody or keeps them
// up. Drawn in EVERY state, because these are the two cases the tool exists to
// stop the Director getting wrong.
func scheduleAntisocial(h int) bool {
	return h < scheduleAntisocialEarly || h >= scheduleAntisocialLate
}

// --- the ranked windows -----------------------------------------------------

// scheduleWindow is one candidate window, before it becomes a card.
type scheduleWindow struct {
	Day    int
	Hour   int
	Length int
	Free   int
	Prefer int
	// Antisocial is how many free members the window wakes or keeps up.
	Antisocial int
}

// scheduleRankWindows derives the ranked candidate windows.
//
// ── WG-3, SIGNED: THIS IS A DERIVATION, NOT A RECOMMENDER ─────────────────
//
// It is arithmetic over the overlay's own per-hour free counts, computed at
// render time and stored NOWHERE — which is why the chip beside it reads
// `derived · not stored` PERMANENTLY rather than `recommender not built`. It
// refuses outright below quorum, because a confident ranking over an empty week
// is the one way this surface can lie.
//
// ── RANK 1 IS THE BENCH'S WINDOW, BY CONSTRUCTION ─────────────────────────
//
// The head of this list is benchRsvpPeakRun's answer — the identical helper the
// Bench RSVP panel prints from. It is not "the same algorithm re-implemented":
// re-implementation is how two surfaces come to disagree about when to play, and
// this page and the Bench are one click apart. Ranks 2..N are the next-best
// windows on OTHER days, ordered by the printed key (free, then preferred, then
// how many people it wakes) with ties breaking toward the LATER date, so an
// earlier window is never dropped by a coin flip.
func scheduleRankWindows(in scheduleBuildInput, members []scheduleMember) []scheduleWindow {
	if in.Avail == nil || len(in.Avail.Days) == 0 {
		return nil
	}
	days := scheduleDayCount(in)

	score := func(day, hour int) scheduleWindow {
		w := scheduleWindow{Day: day, Hour: hour, Length: scheduleCandidateLength}
		w.Free = scheduleFreeCount(in, members, day, hour)
		w.Prefer = schedulePreferCount(in, members, day, hour)
		for _, m := range members {
			if !scheduleFreeAt(m, day, hour) {
				continue
			}
			if h, _, ok := scheduleLocalHour(in, m, day, hour); ok && scheduleAntisocial(h) {
				w.Antisocial++
			}
		}
		return w
	}

	// The peak, through the SHARED helper. Its day and start hour are rank 1.
	peakDay, peakHour, _, peakFree := benchRsvpPeakRun(in.Avail)
	out := []scheduleWindow{}
	seen := map[int]bool{}
	if peakDay >= 0 && peakFree > 0 {
		out = append(out, score(peakDay, peakHour))
		seen[peakDay] = true
	}

	// One candidate per remaining day: that day's own best hour inside the
	// visible band. One card per day keeps the three cards comparable — three
	// cards for three hours of one evening would be a ranking of nothing.
	rest := []scheduleWindow{}
	for d := 0; d < days; d++ {
		if seen[d] {
			continue
		}
		best := scheduleWindow{Day: d, Hour: -1}
		for h := in.BandFrom; h < in.BandTo; h++ {
			w := score(d, h)
			if w.Free == 0 {
				continue
			}
			if best.Hour < 0 || scheduleWindowBetter(w, best) {
				best = w
			}
		}
		if best.Hour >= 0 {
			rest = append(rest, best)
		}
	}
	sort.SliceStable(rest, func(i, j int) bool { return scheduleWindowBetter(rest[i], rest[j]) })
	return append(out, rest...)
}

// scheduleWindowBetter is the printed ranking key, in the printed order:
// heads free at the top of the hour first, then how many marked the hour
// preferred, then how many people it wakes before 08:00 or keeps up past 23:00
// in their own zone. Ties break toward the LATER date.
func scheduleWindowBetter(a, b scheduleWindow) bool {
	if a.Free != b.Free {
		return a.Free > b.Free
	}
	if a.Prefer != b.Prefer {
		return a.Prefer > b.Prefer
	}
	if a.Antisocial != b.Antisocial {
		return a.Antisocial < b.Antisocial
	}
	if a.Day != b.Day {
		return a.Day > b.Day
	}
	return a.Hour < b.Hour
}

// scheduleWindowLabel renders a window as its printed name.
func scheduleWindowLabel(in scheduleBuildInput, w scheduleWindow) string {
	date := scheduleDayDate(in, w.Day)
	label := date
	if t, err := time.Parse("2006-01-02", date); err == nil {
		label = t.Format("Mon 2 Jan")
	}
	return fmt.Sprintf("%s · %s–%s", label, scheduleHour(w.Hour), scheduleHour(w.Hour+w.Length))
}
