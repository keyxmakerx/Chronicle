// schedule_build.go — the five pure builders (C-CALV4-RSVP-P8 Part B).
//
// Each takes the same fully-resolved scheduleBuildInput and returns one
// surface's view model. Nothing here reads a repository, a request or a clock
// it was not handed, which is what lets the oracle tests reproduce every number
// on the page from the same visible set the viewer got.
package calendar

import (
	"fmt"
	"sort"
	"strings"
)

// --- S1 · THE VERDICT -------------------------------------------------------

// scheduleBuildVerdict assembles S1: the page leads with the ANSWER, and the
// matrix beneath it is the evidence you audit the answer against.
func scheduleBuildVerdict(in scheduleBuildInput) ScheduleVerdict {
	members := scheduleMembers(in)
	total := len(members)
	v := ScheduleVerdict{
		Title: "When to play",
		Frame: scheduleFrameLine(in),
		Mode:  "tallies only — names are not in your view",
	}
	if in.IsGM {
		v.Mode = "ranked from this week's saved availability"
	}
	if total == 0 {
		v.Headline = "Pick a window"
		v.Rec = "this campaign has no members to rank a week for"
		v.Caption = scheduleVerdictCaption(false)
		return v
	}

	saved := scheduleSavedCount(in, members)
	windows := scheduleRankWindows(in, members)
	noneSaved := saved == 0
	quorum := saved >= scheduleQuorum
	ranked := quorum && len(windows) > 0

	// THE CHIPS. WG-3 is SIGNED: the window IS derived server-side from the
	// overlay's own per-hour free counts, so the live chip is the PERMANENT
	// `derived · not stored`, never the spec's `recommender not built` — that
	// wording pre-dates the ruling and would leave a "needs backend" chip over a
	// backend that exists, the inversion WG-8 retired.
	if ranked {
		v.Chips = append(v.Chips, scheduleNeed(in.IsGM, "derived · not stored")...)
	}
	v.Chips = append(v.Chips, scheduleNeed(in.IsGM, "does not know what's already booked")...)
	if len(v.Chips) == 0 {
		v.Chips = nil
	}

	// The cards, in DATE order, with rank printed. Selection is a URL state, so
	// choosing a window reloads the page with the same cards in the same places.
	if ranked {
		v.Candidates = scheduleCandidateCards(in, members, windows)
		v.More, v.MoreLabel, v.MoreID = scheduleMoreWindows(in, windows, total)
	} else {
		v.Fault = scheduleVerdictFault(in, saved, total, noneSaved, quorum)
	}

	sel := scheduleSelectedCandidate(v.Candidates, in.Cand)
	if sel != nil {
		v.Headline = sel.When
		v.HeadlineZone = in.ZoneLeaf
		v.HeadlineTitle = in.Zone
		w := scheduleWindowForRank(windows, sel.Rank)
		v.Rec = fmt.Sprintf("%d of %d free · %d prefer it", w.Free, total, w.Prefer)
	} else {
		v.Headline = "Pick a window"
		if noneSaved {
			v.Rec = "nothing is saved yet, so nothing can be ranked"
		} else {
			v.Rec = "the matrix below still shows where the outlines stack"
		}
	}

	// THE HONESTY LINE. It counts members with nothing saved, and it says
	// "shows no saved availability" rather than "is unavailable", because the
	// tool cannot tell those apart (ledger #3).
	//
	// It is derived from the AGGREGATE (saved vs total) rather than from the
	// lanes, so a player's page prints the same number the Director's does — the
	// count is a fact about the campaign, not about who may see whom.
	switch {
	case noneSaved:
		v.WarnLine, v.WarnTone = "nobody has saved a week yet", "warn"
	case saved < total:
		n := total - saved
		v.WarnLine = fmt.Sprintf("%d %s no saved availability", n,
			schedulePlural(n, "member shows", "members show"))
		v.WarnTone = "warn"
	default:
		v.WarnLine = fmt.Sprintf("%d of %d answered", scheduleAnswered(members), total)
		v.WarnTone = "good"
	}

	// THE DIRECTOR'S TWO CONTROLS.
	if in.IsGM {
		// PROPOSE KEEPS ITS CHIP. There is still no propose-from-window write
		// path (ledger #19), and a page whose primary action is a fiction is
		// worse than a page with no primary action. The chip is VISIBLE beside
		// it, never `title` alone: `title` is not announced by several screen
		// readers and is unreachable by touch (WG-5).
		v.Propose = &BenchAction{
			Label: "Propose this window", Fill: true, NeedsBackend: true,
			Title: "there is no propose-from-window write path",
		}
		// NUDGE IS LIVE. The sealed mockup drew it disabled with
		// `NEED('no reminder endpoint')`, and that stopped being true on
		// 2026-07-29 when C-CALV4-RSVP-P8B shipped POST /calendar/ask. The
		// drawing's own "still open" list said so. Shipping the drawn chip over
		// a built backend would be the exact inversion WG-8 retired, so the
		// RECONCILIATION lands here: one live control, built by the SAME
		// benchRsvpAsk the Bench's is, so the two cannot drift into different
		// cooldown readouts or different refusals.
		v.Ask = benchRsvpAsk(benchRsvpInput{
			CampaignID: in.CampaignID, CSRFToken: in.CSRFToken, EventID: in.EventID,
			MailConfigured: in.MailConfigured, AskState: in.AskState,
		})
		// ITEM 11 IS IMPLEMENTED, NOT CLOSED. The unconfigured-SMTP sentence
		// ships as a single shared package constant and renders as `.badge.warn`
		// plus this line — never as `.badge.need`, because an unconfigured mail
		// server is a deployment fact in the `zone not set` register, not a
		// build gap.
		if !in.MailConfigured {
			v.MailLine = mailNotConfiguredLine
		}
	}

	v.Caption = scheduleVerdictCaption(ranked)
	return v
}

// scheduleVerdictFault is the signed refusal that REPLACES the cards. The
// instrument does not vanish and it does not fabricate: no zero-scored card, no
// em-dashes standing in for data.
func scheduleVerdictFault(in scheduleBuildInput, saved, total int, noneSaved, quorum bool) *ScheduleCandidateFault {
	chip := func(s string) string {
		if in.IsGM {
			return s
		}
		return ""
	}
	switch {
	case noneSaved:
		return &ScheduleCandidateFault{
			Headline: "No availability saved by anyone",
			Detail: "the matrix below is empty for the same reason — nobody has filled in a " +
				"week yet",
			Chip: chip("nothing to rank"),
		}
	case !quorum:
		return &ScheduleCandidateFault{
			Headline: "Not enough saved availability to rank",
			Detail: fmt.Sprintf("%d of %d members have saved a week — a ranking from two "+
				"people's data is a guess wearing a number", saved, total),
			Chip: chip("quorum not met"),
		}
	default:
		return &ScheduleCandidateFault{
			Headline: "Nobody is free at any hour this week",
			Detail: "the matrix below still shows where the outlines stack; the count lane " +
				"names the peak",
			Chip: chip("nothing to rank"),
		}
	}
}

// scheduleCandidateCards turns the top-scoring windows into cards.
//
// RANKED BY SCORE, PRINTED BY DATE. The rank is assigned from the score order
// and then the slice is re-sorted chronologically, which is the whole
// anti-motion-by-data mechanic: when somebody answers, the numeral re-inks and
// the reason sentence rewrites in place, and no card moves.
func scheduleCandidateCards(in scheduleBuildInput, members []scheduleMember, windows []scheduleWindow) []ScheduleCandidate {
	n := scheduleCandidateCount
	if len(windows) < n {
		n = len(windows)
	}
	total := len(members)
	cards := make([]ScheduleCandidate, 0, n)
	for i := 0; i < n; i++ {
		w := windows[i]
		date := scheduleDayDate(in, w.Day)
		c := ScheduleCandidate{
			Rank:   i + 1,
			DayKey: date,
			When:   scheduleWindowLabel(in, w),
			Zone:   in.ZoneLeaf,
			Why:    scheduleWhy(in, members, w, total),
			TallyFree: fmt.Sprintf("%d / %d free", w.Free, total),
			TallyPrefer: fmt.Sprintf("%d of %d %s", w.Prefer, total,
				schedulePlural(w.Prefer, "prefers", "prefer")),
			Href: scheduleHref(in.CampaignID, in.Base, "cand", scheduleItoa(i+1)),
		}
		if in.IsGM {
			c.Outs, c.OutsExtra, c.Unknowns, c.UnknownsExtra, c.EveryoneFree =
				scheduleOutColumn(members, w)
		}
		cards = append(cards, c)
	}
	sort.SliceStable(cards, func(i, j int) bool { return cards[i].DayKey < cards[j].DayKey })
	// Selection is resolved AFTER the sort so `cand=1` always means rank 1,
	// wherever rank 1 sits on the page.
	want := strings.TrimSpace(in.Cand)
	if want == "" {
		want = "1"
	}
	for i := range cards {
		cards[i].Selected = scheduleItoa(cards[i].Rank) == want
	}
	return cards
}

// scheduleSelectedCandidate finds the chosen card, defaulting to rank 1.
func scheduleSelectedCandidate(cards []ScheduleCandidate, want string) *ScheduleCandidate {
	for i := range cards {
		if cards[i].Selected {
			return &cards[i]
		}
	}
	return nil
}

// scheduleWindowForRank recovers the scored window behind a printed rank.
func scheduleWindowForRank(windows []scheduleWindow, rank int) scheduleWindow {
	if rank >= 1 && rank <= len(windows) {
		return windows[rank-1]
	}
	return scheduleWindow{}
}

// scheduleOutColumn splits the members who are NOT free into the two groups the
// mark vocabulary distinguishes.
//
// LEDGER #3, ENFORCED IN THE INK. A member who never saved a week is NOT "out" —
// the tool cannot tell that apart from "busy all week". Known-busy members get a
// FILLED swatch under `out`; never-answered members get a HOLLOW swatch under
// `no answer`. Calling an unknown a refusal is the exact lie this page exists to
// avoid.
func scheduleOutColumn(members []scheduleMember, w scheduleWindow) (outs []ScheduleStamp, outsExtra int,
	unknowns []ScheduleStamp, unknownsExtra int, everyoneFree bool) {
	const cap = 3
	busy, unk := []ScheduleStamp{}, []ScheduleStamp{}
	for _, m := range members {
		if scheduleFreeAt(m, w.Day, w.Hour) {
			continue
		}
		s := ScheduleStamp{Token: m.Token, Axis: m.Axis, Pattern: m.Pattern}
		if scheduleHasAny(m) {
			busy = append(busy, s)
			continue
		}
		s.Hollow = true
		s.Pattern = ""
		unk = append(unk, s)
	}
	if len(busy)+len(unk) == 0 {
		return nil, 0, nil, 0, true
	}
	if len(busy) > cap {
		outs, outsExtra = busy[:cap], len(busy)-cap
	} else {
		outs = busy
	}
	if len(unk) > cap {
		unknowns, unknownsExtra = unk[:cap], len(unk)-cap
	} else {
		unknowns = unk
	}
	return outs, outsExtra, unknowns, unknownsExtra, false
}

// scheduleWhy is THE REASON SENTENCE — the written reason a window ranks where
// it does, and the thing that makes overruling the tool possible.
//
// PERMISSION IS ABSENCE, AND IT BINDS THE PROSE. A player's payload carries no
// member, so this sentence may not name anybody: the numbers survive, the names
// do not exist. A Director-only fact leaking through a sentence would be the
// loudest oracle on the page precisely because it reads as innocuous copy — and
// it is the same rule that removes the out column and the per-member lanes.
func scheduleWhy(in scheduleBuildInput, members []scheduleMember, w scheduleWindow, total int) string {
	bits := []string{}
	if w.Free == total {
		bits = append(bits, "everyone free")
	} else {
		bits = append(bits, fmt.Sprintf("%d of %d free", w.Free, total))
	}
	if w.Prefer > 0 {
		bits = append(bits, fmt.Sprintf("%d %s it", w.Prefer, schedulePlural(w.Prefer, "prefers", "prefer")))
	}

	// ── EVERYTHING BELOW THIS LINE NEEDS LANE DATA, AND A PLAYER HAS NONE ──
	//
	// "who is out", "how many never answered" and "whose local start is 01:00"
	// are all facts about INDIVIDUAL members, and a player's payload carries no
	// member at all. Deriving them anyway from an absent map does not produce a
	// vaguer sentence — it produces a FALSE one: every member reads as
	// never-having-answered, and the card would tell a player that five of five
	// people ignored the question.
	//
	// So the clauses are gated on the lane data EXISTING, not on IsGM. The
	// numbers a player keeps (free, preferred) are aggregates and are true; the
	// sentences they lose are the ones their payload cannot support. That is the
	// same rule that removes the out column and the per-member lanes, applied to
	// prose — where it matters most, because prose reads as innocuous.
	if in.Avail == nil || in.Avail.Lanes == nil {
		return strings.Join(bits, " · ")
	}

	busy, unknown := []string{}, 0
	for _, m := range members {
		if scheduleFreeAt(m, w.Day, w.Hour) {
			continue
		}
		if scheduleHasAny(m) {
			busy = append(busy, m.First)
			continue
		}
		unknown++
	}
	if len(busy) > 0 {
		if in.IsGM {
			bits = append(bits, strings.Join(busy, " and ")+" out")
		} else {
			bits = append(bits, fmt.Sprintf("%d out", len(busy)))
		}
	}
	if unknown > 0 {
		bits = append(bits, fmt.Sprintf("%d never answered", unknown))
	}

	// The antisocial clause: the one fact a Director cannot see by looking at a
	// grid drawn in their OWN zone.
	worst, worstName, found := 0, "", false
	for _, m := range members {
		if !scheduleFreeAt(m, w.Day, w.Hour) {
			continue
		}
		h, _, ok := scheduleLocalHour(in, m, w.Day, w.Hour)
		if !ok || !scheduleAntisocial(h) {
			continue
		}
		if !found {
			worst, worstName, found = h, m.First, true
		}
	}
	if found {
		if in.IsGM {
			bits = append(bits, fmt.Sprintf("%s for %s", scheduleHour(worst), worstName))
		} else {
			bits = append(bits, fmt.Sprintf("%s for someone", scheduleHour(worst)))
		}
	} else {
		bits = append(bits, "nobody's local start before 08:00")
	}
	return strings.Join(bits, " · ")
}

// scheduleMoreWindows is the lower-scoring tail, in a popover.
//
// ORDERED BY DATE, WITH THE SCORE PRINTED ON THE RIGHT. A list ordered by score
// would put the ranking's own tail in a second, competing order, and the reader
// would have to work out which of the two orders the page believes.
func scheduleMoreWindows(in scheduleBuildInput, windows []scheduleWindow, total int) ([]ScheduleMoreRow, string, string) {
	if len(windows) <= scheduleCandidateCount {
		return nil, "", ""
	}
	tail := windows[scheduleCandidateCount:]
	rows := make([]ScheduleMoreRow, 0, len(tail))
	top := 0
	for _, w := range tail {
		if w.Free > top {
			top = w.Free
		}
		rows = append(rows, ScheduleMoreRow{
			When:  scheduleWindowLabel(in, w),
			Score: fmt.Sprintf("%d / %d free", w.Free, total),
		})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].When < rows[j].When })
	label := fmt.Sprintf("%d more %s score %d / %d or lower →",
		len(tail), schedulePlural(len(tail), "window", "windows"), top, total)
	return rows, label, "sc-pop-more"
}

// --- S2 · THE MATRIX --------------------------------------------------------

// scheduleBuildMatrix assembles S2: members × days, per-person outlines,
// aggregate density and the computed window's bracket, at FULL WIDTH with no
// side rail beside it — the rail's job is done by the Verdict above.
func scheduleBuildMatrix(in scheduleBuildInput) ScheduleMatrix {
	members := scheduleMembers(in)
	total := len(members)
	m := ScheduleMatrix{
		Title:         "Who is free when",
		Frame:         scheduleMatrixFrame(in),
		IdentCap:      "everyone",
		IdentCapShort: "everyone",
		SayCap:        "time",
		Scope:         "anonymous totals only — your own week is in “My availability” above",
	}
	if in.IsGM {
		// The phone's word for the same column is the drawing's own `who`: at
		// 390 the identity column is 76px and "who is free" wraps to two lines,
		// which pushes the header band taller than every row under it.
		m.IdentCap, m.IdentCapShort = "who is free", "who"
		m.Scope = "per-member lanes · owner / co-DM only"
	}
	// THE SAY COLUMN NAMES WHOSE CLOCK IT IS. Once a window is chosen the column
	// holds each member's OWN local start, so calling it "time" would invite the
	// reader to think it is the page's zone — the one confusion this column
	// exists to prevent.
	if in.Session != nil && in.Session.Anchored {
		m.SayCap = "their time"
	}
	m.Denominator = fmt.Sprintf("free of %d in the campaign", total)
	m.DenominatorShort = fmt.Sprintf("free of %d", total)
	m.Zero = "Nobody has filled in a week yet. The grid works the moment one person does — " +
		"one marked window is worth more than none."
	// THE COUNT LANE WEARS ITS SAMPLING RULE PERMANENTLY. It disagrees with the
	// minute-accurate lanes above it, and the chip is how it says so in plain
	// sight rather than hoping nobody notices (ledger #7).
	m.CountChip = scheduleNeed(in.IsGM, "on the hour")

	if in.Avail == nil || len(in.Avail.Days) == 0 {
		m.Caption = scheduleMatrixCaption(in, false)
		return m
	}

	m.Cols = scheduleColumns(in)

	// PLAYER RENDERING: NO MEMBER LANES AT ALL, INCLUDING THEIR OWN.
	// OverlayMember is omitted wholesale from a player's payload, so this loop
	// simply has nothing to walk — there is no `if IsGM` here, and that is
	// deliberate: a permission expressed as a template branch is one refactor
	// away from being lost. Their own week is directly above, in the Painter, at
	// the same geometry, and the header says so. No ghost row, no lock, no
	// "+4 hidden". Permission is ABSENCE.
	if in.Avail.Lanes != nil {
		for _, mem := range members {
			m.Lanes = append(m.Lanes, scheduleLaneFor(in, mem, m.Cols))
		}
	}

	m.Density, m.Counts = scheduleAggregateLanes(in, members, m.Cols, total)
	m.Bracket = scheduleBracketFor(in, members, m.Cols)
	if in.IsGM {
		m.Pops = schedulePopovers(in, members, m.Cols, total)
	}

	if total > len(benchRsvpHues) {
		m.Chips = append(m.Chips, scheduleNeed(in.IsGM, "identity wraps after 8")...)
	}
	m.Caption = scheduleMatrixCaption(in, total > len(benchRsvpHues))
	if scheduleSavedCount(in, members) > 0 {
		m.Zero = ""
	}
	return m
}

// scheduleColumns builds the column set — THE ONE PLACE THE ZOOM IS DECIDED.
//
// --cols is emitted SERVER-SIDE as days × slots. The stylesheet never multiplies
// inside repeat() and no CSS on this surface writes a week length or an hour
// count — a calendar's week length is its own business and this page does not
// assume the Gregorian one.
//
// ── WEEK ZOOM: the cell is the DAY, and the outline inside it is still
// minute-positioned across the whole visible band, so *where in the evening*
// survives the compression.
//
// ── DAY ZOOM: the cell is the HOUR of ONE day, at full size. NOTHING ELSE
// CHANGES — the same overlay, the same lanes, the same minute-accurate marks,
// the same aggregates, narrowed to one date and re-sliced by hour. That is why
// the day view needs no route, no query and no new seam: every fact it draws is
// already in the week payload the Matrix ships, and the only thing this function
// does differently is decide what a column MEANS.
func scheduleColumns(in scheduleBuildInput) []ScheduleCol {
	if scheduleDayZoom(in) {
		return scheduleHourColumns(in)
	}
	days := scheduleDayCount(in)
	out := []ScheduleCol{}
	for d := 0; d < days; d++ {
		date := scheduleDayDate(in, d)
		head, sub := date, ""
		if t, err := timeParseISO(date); err == nil {
			head, sub = t.Format("Mon"), scheduleItoa(t.Day())
		}
		when := strings.TrimSpace(head + " " + sub)
		out = append(out, ScheduleCol{
			Head: head, Sub: sub, Day: d, DayKey: date,
			Key: date, When: when, DayLabel: when,
			// The weekend's leading edge takes the heavier structural rule —
			// the week's own emphasis, not a decoration.
			Major:       d == days-2,
			StartMinute: in.BandFrom * 60,
			EndMinute:   in.BandTo * 60,
		})
	}
	return out
}

// scheduleHourColumns is DAY zoom's column set: one column per hour of the
// visible band, all on the selected date.
//
// EVERY COLUMN KEEPS `data-day`. Guard B4's rule is that a dated node carries
// the ANSWER key, and an hour of Saturday is still Saturday — so DayKey stays
// the date on all eight columns and `Key` (date + hour) carries the identity the
// popover ids need. Losing data-day here would make the day view the one part of
// this page that stops answering.
func scheduleHourColumns(in scheduleBuildInput) []ScheduleCol {
	d := scheduleSelectedDay(in)
	date := scheduleDayDate(in, d)
	dayLabel := date
	if t, err := timeParseISO(date); err == nil {
		dayLabel = t.Format("Mon 2")
	}
	out := []ScheduleCol{}
	for h := in.BandFrom; h < in.BandTo; h++ {
		out = append(out, ScheduleCol{
			Head: scheduleItoa(h), Sub: "", Day: d, DayKey: date,
			Key:      date + "T" + scheduleItoa(h),
			When:     dayLabel + ", " + scheduleHour(h),
			DayLabel: dayLabel,
			// The drawing's own rule: every sixth hour after the band's first
			// takes the heavier rule, and the band's first never does — an
			// emphasis on the leading edge would just be a second panel border.
			Major:       (h-in.BandFrom)%6 == 0 && h != in.BandFrom,
			StartMinute: h * 60,
			EndMinute:   (h + 1) * 60,
		})
	}
	return out
}

// scheduleLaneFor builds one member's row.
func scheduleLaneFor(in scheduleBuildInput, m scheduleMember, cols []ScheduleCol) ScheduleLane {
	lane := ScheduleLane{
		Token: m.Token, Name: m.First, Axis: m.Axis, Pattern: m.Pattern,
		Answer: m.Answer, Tone: m.Tone,
	}

	// THE SAY COLUMN. A member with no zone gets a LITERALLY EMPTY clock and the
	// repair beside it — never "--:--", never a dash, never a UTC guess.
	if m.TZ == "" {
		lane.ZoneMissing = true
		// [GR-15]: the viewer's own row repairs itself at /account; a GM gets
		// the roster; a Player looking at someone else's row gets no control.
		lane.AskHref, lane.AskLabel = benchZoneRepair(in.CampaignID, in.ViewerID, m.UserID, in.IsGM)
	} else if clock, next, ok := scheduleLocalHourAt(in, m); ok {
		lane.LocalTime, lane.NextDay = clock, next
		lane.Antisocial = scheduleAntisocialClock(clock)
	}

	// HONESTY, WHERE THE SHAPE WOULD BE. An empty lane cannot say whether this
	// member is busy or has never answered, so it says exactly that, in the
	// NEUTRAL register — the tool does not know, and an unknown is not a fault.
	if !scheduleHasAny(m) {
		lane.Note = "no availability saved — this cannot be told apart from “busy all week” yet"
		lane.NoteShort = "nothing saved"
		lane.NoteChip = scheduleNeed(in.IsGM, "no pattern")
		return lane
	}

	// A BAND MAY NEVER SILENTLY HIDE SOMEBODY. When every one of a member's
	// windows falls outside the visible band the lane says so, counts them, and
	// names the repair.
	if outside := scheduleOutsideBand(in, m); outside > 0 {
		lane.NoteWarn = fmt.Sprintf("%d %s outside %02d–%02d", outside,
			schedulePlural(outside, "window", "windows"), in.BandFrom, in.BandTo)
		lane.Note = "this band hides them — widen it above"
		return lane
	}

	for _, c := range cols {
		lane.Cells = append(lane.Cells, scheduleCellFor(in, m, c))
	}
	return lane
}

// scheduleOutsideBand counts a member's windows when NONE of them intersects
// the visible band, and returns 0 otherwise.
func scheduleOutsideBand(in scheduleBuildInput, m scheduleMember) int {
	lo, hi := in.BandFrom*60, in.BandTo*60
	for _, g := range m.Lanes {
		if g.EndMinute > lo && g.StartMinute < hi {
			return 0
		}
	}
	return len(m.Lanes)
}

// scheduleCellFor builds one matrix cell.
//
// THE CELL IS THE TARGET (≥24×24 at every width); the OUTLINE inside it is a
// MARK, not a control. That is what fixes an 8px-tall interactive lozenge, and
// it is why `busy` is drawn as NOTHING — the same absence rule as permission.
func scheduleCellFor(in scheduleBuildInput, m scheduleMember, c ScheduleCol) ScheduleCell {
	cell := ScheduleCell{Major: c.Major, DayKey: c.DayKey, Axis: m.Axis}
	span := c.EndMinute - c.StartMinute
	if span <= 0 {
		return cell
	}
	const markCap = 3
	shown := 0
	segs := []BenchLaneSegment{}
	for _, g := range m.Lanes {
		if g.DayIndex != c.Day || g.EndMinute <= c.StartMinute || g.StartMinute >= c.EndMinute {
			continue
		}
		segs = append(segs, g)
	}
	for _, g := range segs {
		if shown == markCap {
			// A 4th window in one cell turns NEUTRAL; the exact list is in the
			// popover. A stack of five marks in a 24px cell is not information.
			cell.Marks = append(cell.Marks, ScheduleMark{From: "0", To: "100", Rest: true})
			break
		}
		s, e := g.StartMinute, g.EndMinute
		if s < c.StartMinute {
			s = c.StartMinute
		}
		if e > c.EndMinute {
			e = c.EndMinute
		}
		cell.Marks = append(cell.Marks, ScheduleMark{
			From:    fmt.Sprintf("%.4f", float64(s-c.StartMinute)/float64(span)*100),
			To:      fmt.Sprintf("%.4f", float64(e-c.StartMinute)/float64(span)*100),
			Pattern: m.Pattern,
			Prefer:  g.State == AvailPreferred,
			// A window that CONTINUES past this column's edge grows no end cap
			// and no radius there — otherwise every hour boundary in DAY zoom
			// would print a false start and a false finish.
			ContLeft:  g.StartMinute < c.StartMinute,
			ContRight: g.EndMinute > c.EndMinute,
		})
		shown++
	}
	cell.Label = scheduleCellLabel(m, c, segs)
	if len(segs) > 0 && in.IsGM {
		cell.PopID = schedulePopID("pc", m.UserID, c.Key)
	}
	return cell
}

// scheduleCellLabel is the cell's accessible name. It states the FACT, at the
// resolution the mark carries, so a screen-reader user gets the minute-accurate
// truth the sighted reader gets from the outline's position.
func scheduleCellLabel(m scheduleMember, c ScheduleCol, segs []BenchLaneSegment) string {
	when := c.When
	if len(segs) == 0 {
		return fmt.Sprintf("%s, %s: no free time saved", m.First, when)
	}
	label := fmt.Sprintf("%s, %s: free %s to %s", m.First, when,
		scheduleMinute(segs[0].StartMinute), scheduleMinute(segs[0].EndMinute))
	for _, g := range segs {
		if g.State == AvailPreferred {
			label += ", prefers it"
			break
		}
	}
	if n := len(segs) - 1; n > 0 {
		label += fmt.Sprintf(", and %d more %s", n, schedulePlural(n, "window", "windows"))
	}
	return label
}

// scheduleAggregateLanes builds the two lanes EVERYONE gets: the achromatic
// density bar and the exact number.
//
// THE HOUR THE COUNT IS TAKEN AT IS THE ONE THING THE ZOOM CHANGES. In week zoom
// a column is a whole day, so the number is that day's PEAK inside the band and
// the lane prints `@ 19` beside it, because a count with no hour is unreadable.
// In day zoom a column already IS an hour: the number is the count AT that hour
// and the `@ h` sub is dropped, since a `@ 19` under a column headed `19` is the
// same fact printed twice. Both readings come from scheduleFreeCount, so the two
// zooms cannot disagree about how many people are free.
func scheduleAggregateLanes(in scheduleBuildInput, members []scheduleMember, cols []ScheduleCol,
	total int) ([]ScheduleDensity, []ScheduleCount) {
	dayZoom := scheduleDayZoom(in)
	dens := make([]ScheduleDensity, 0, len(cols))
	counts := make([]ScheduleCount, 0, len(cols))
	top := 0
	peaks := make([]int, len(cols))
	free := make([]int, len(cols))
	for i, c := range cols {
		if dayZoom {
			h := c.StartMinute / 60
			peaks[i], free[i] = h, scheduleFreeCount(in, members, c.Day, h)
			if free[i] > top {
				top = free[i]
			}
			continue
		}
		bestH, bestN := in.BandFrom, -1
		for h := in.BandFrom; h < in.BandTo; h++ {
			if n := scheduleFreeCount(in, members, c.Day, h); n > bestN {
				bestH, bestN = h, n
			}
		}
		if bestN < 0 {
			bestN = 0
		}
		peaks[i], free[i] = bestH, bestN
		if bestN > top {
			top = bestN
		}
	}
	for i, c := range cols {
		dens = append(dens, ScheduleDensity{
			Free: free[i], Total: total, DayKey: c.DayKey, Major: c.Major,
			Title: fmt.Sprintf("%d of %d free at %s", free[i], total, scheduleHour(peaks[i])),
		})
		cnt := ScheduleCount{
			Free: free[i],
			Peak: free[i] == top && top > 0, DayKey: c.DayKey, Major: c.Major,
			Label: fmt.Sprintf("%d of %d free at %s", free[i], total, scheduleHour(peaks[i])),
		}
		if !dayZoom {
			cnt.PeakHour = fmt.Sprintf("@ %d", peaks[i])
		}
		// DIRECTOR: each numeral is a button naming who is free and who is
		// missing. PLAYER: a plain number — the names are ABSENT from their DOM,
		// not greyed.
		if in.IsGM {
			cnt.PopID = schedulePopID("pn", "", c.Key)
		}
		counts = append(counts, cnt)
	}
	return dens, counts
}

// scheduleBracketFor draws the derived window over the columns it spans.
//
// ACCENT IS THE TOOL TALKING (canon A7): the computed span takes --accent, and a
// human-offered or human-proposed span would take --rule-editorial instead. The
// accent never colours event data.
func scheduleBracketFor(in scheduleBuildInput, members []scheduleMember, cols []ScheduleCol) *ScheduleBracket {
	if scheduleSavedCount(in, members) < scheduleQuorum {
		return nil
	}
	windows := scheduleRankWindows(in, members)
	if len(windows) == 0 {
		return nil
	}
	sel := 1
	if n, err := atoiSafe(in.Cand); err == nil && n >= 1 && n <= len(windows) {
		sel = n
	}
	w := windows[sel-1]

	// DAY ZOOM: the bracket spans the window's HOURS, and it is drawn only when
	// the chosen window is on the day that is on screen.
	//
	// The drawing finds its start column by hour alone, which on its own default
	// state (the ranked window and the day view both land on Saturday) is the
	// same answer. It is not the same answer once a reader steps the day: an
	// hour-only match would print the accent bracket — the TOOL saying "play
	// here" — across a Wednesday whose numbers never produced it. The bracket is
	// the page's answer, and an answer drawn over the wrong day is worse than no
	// bracket, so this one refuses instead.
	if scheduleDayZoom(in) {
		if w.Day != scheduleSelectedDay(in) {
			return nil
		}
		lo := w.Hour - in.BandFrom
		if lo < 0 || lo >= len(cols) {
			return nil
		}
		hi := lo + w.Length
		if hi > len(cols) {
			hi = len(cols)
		}
		return &ScheduleBracket{
			Start: lo + 2, End: hi + 2,
			Label: scheduleHour(w.Hour) + "–" + scheduleHour(w.Hour+w.Length),
		}
	}

	date := scheduleDayDate(in, w.Day)
	label := scheduleHour(w.Hour)
	if t, err := timeParseISO(date); err == nil {
		label = t.Format("Mon") + " " + scheduleHour(w.Hour)
	}
	// Grid lines past the identity column: column 1 is the label, so day index
	// d spans lines d+2 to d+3.
	return &ScheduleBracket{Start: w.Day + 2, End: w.Day + 3, Label: label}
}

// schedulePopovers builds the Director's per-cell and per-column detail.
//
// TOP LAYER, so they escape the matrix's own horizontal scroll container — which
// is the whole reason a cell's detail is a popover and not an inline expansion.
func schedulePopovers(in scheduleBuildInput, members []scheduleMember, cols []ScheduleCol,
	total int) []SchedulePop {
	out := []SchedulePop{}
	for _, m := range members {
		for _, c := range cols {
			rows := []SchedulePopRow{}
			for _, g := range m.Lanes {
				// THE POPOVER BELONGS TO THE COLUMN, NOT TO THE DAY. Filtering
				// on the day alone was indistinguishable from filtering on the
				// column while a column WAS a day; once it is an hour, the 16:00
				// detail would list a 17:00–24:00 window the 16:00 cell draws
				// nothing for — and would mint a popover for every empty cell
				// in the row. The drawing filters on the span in both zooms.
				if g.DayIndex != c.Day || g.State == AvailPreferred ||
					g.EndMinute <= c.StartMinute || g.StartMinute >= c.EndMinute {
					continue
				}
				rows = append(rows, SchedulePopRow{
					Text: scheduleMinute(g.StartMinute) + "–" + scheduleMinute(g.EndMinute),
					Note: "your " + in.ZoneLeaf,
				})
			}
			if len(rows) == 0 {
				continue
			}
			for _, g := range m.Lanes {
				if g.DayIndex == c.Day && g.State == AvailPreferred &&
					g.EndMinute > c.StartMinute && g.StartMinute < c.EndMinute {
					rows = append(rows, SchedulePopRow{Text: "preferred"})
					break
				}
			}
			out = append(out, SchedulePop{
				ID:   schedulePopID("pc", m.UserID, c.Key),
				Axis: m.Axis,
				Head: m.First + " · " + c.DayLabel,
				Rows: rows,
				Foot: "outlines are minute-accurate; the count below samples the top of the hour",
			})
		}
	}
	dayZoom := scheduleDayZoom(in)
	for _, c := range cols {
		// In day zoom the column IS the hour, so there is no peak to search
		// for: the popover names the same hour the numeral was counted at.
		bestH := c.StartMinute / 60
		if !dayZoom {
			bestN := -1
			bestH = in.BandFrom
			for h := in.BandFrom; h < in.BandTo; h++ {
				if n := scheduleFreeCount(in, members, c.Day, h); n > bestN {
					bestH, bestN = h, n
				}
			}
		}
		freeNames, missNames := []string{}, []string{}
		for _, m := range members {
			if scheduleFreeAt(m, c.Day, bestH) {
				freeNames = append(freeNames, m.First)
			} else {
				missNames = append(missNames, m.First)
			}
		}
		out = append(out, SchedulePop{
			ID:   schedulePopID("pn", "", c.Key),
			Head: c.DayLabel + " · " + scheduleHour(bestH),
			Rows: []SchedulePopRow{
				{Text: fmt.Sprintf("%d free", len(freeNames)), Note: scheduleNamesOr(freeNames)},
				{Text: fmt.Sprintf("%d not free", len(missNames)), Note: scheduleNamesOr(missNames)},
			},
			Foot: fmt.Sprintf("counted at the top of the hour, out of %d in the campaign", total),
		})
	}
	return out
}

func scheduleNamesOr(names []string) string {
	if len(names) == 0 {
		return "nobody"
	}
	return strings.Join(names, ", ")
}

// schedulePopID mints a DOM-safe popover id.
func schedulePopID(prefix, userID, dayKey string) string {
	clean := func(s string) string {
		var b strings.Builder
		for _, r := range s {
			switch {
			case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
				b.WriteRune(r)
			}
		}
		return b.String()
	}
	if userID == "" {
		return prefix + "-" + clean(dayKey)
	}
	return prefix + "-" + clean(userID) + "-" + clean(dayKey)
}

// --- S3 · THE ROSTER --------------------------------------------------------

// scheduleBuildRoster assembles S3 — W-G item 4's permanent home, and the one
// place a role is printed truthfully.
func scheduleBuildRoster(in scheduleBuildInput) ScheduleRoster {
	members := scheduleMembers(in)
	r := ScheduleRoster{
		SlotLabel: scheduleSlotLabel(in),
		Sub: fmt.Sprintf("%d in the campaign · %d answered",
			len(in.Roster), scheduleAnswered(members)),
		Caption: scheduleRosterCaption(),
	}
	r.Chips = scheduleChips(
		scheduleNeed(in.IsGM, "friendly zone names need a backend helper"),
		scheduleNeed(in.IsGM, "RSVP answers an event, not a week"),
	)

	// A PLAYER'S ROSTER CONTAINS ONLY THEIR OWN ROW. No "awaiting reply" column
	// (not derivable for them), no other names, no greyed placeholders.
	rows := members
	if !in.IsGM {
		rows = nil
		for _, m := range members {
			if m.UserID == in.ViewerID {
				rows = append(rows, m)
			}
		}
	}
	for _, m := range rows {
		r.Rows = append(r.Rows, scheduleRosterRowFor(in, m))
	}
	if in.IsGM {
		r.SlotPopID = "sc-pop-slot"
	}
	// Spec §2.3 also puts an `awaiting reply` group HERE. It is drawn ONCE, in
	// S5, because on one page the same derived group appearing twice makes the
	// reader check whether the two agree. The roster's job is identity, role and
	// clock; S5 is where "who still owes an answer" lives.
	return r
}

// scheduleRosterRowFor prints one member truthfully.
//
// ROLE IS THE TRUTH, NOT A GUESS (WG-4). Role.DisplayName() — Owner / Scribe /
// Player — plus a gold co-DM badge when the grant is present. The overlay's own
// former roleLabel ignored the grant and printed a lie: a co-DM rendered as
// "player" while receiving owner-tier detail, on the one surface whose entire
// subject is who-may-see-what.
func scheduleRosterRowFor(in scheduleBuildInput, m scheduleMember) ScheduleRosterRow {
	row := ScheduleRosterRow{
		Name: m.Name, Token: m.Token, Axis: m.Axis, Pattern: m.Pattern,
		Role: m.Role, IsCoDM: m.IsCoDM, Host: m.Host,
		Answer: m.Answer, Tone: m.Tone,
	}
	if m.TZ == "" {
		// A MISSING TIMEZONE CAN NEVER PRODUCE A TIME. The repair may never be
		// the thing that disappears on the smallest screen — but WHICH repair
		// depends on whose row it is ([GR-15]).
		row.AskHref, row.AskLabel = benchZoneRepair(in.CampaignID, in.ViewerID, m.UserID, in.IsGM)
		return row
	}
	row.Zone, row.ZoneTitle = m.ZoneLeaf, m.TZ
	if in.Session != nil && in.Session.Anchored && in.Zone != "" {
		if h, next, ok := scheduleLocalHourAt(in, m); ok {
			row.LocalTime = h
			row.NextDay = next
			row.Antisocial = scheduleAntisocialClock(h)
		}
	}
	return row
}

// --- S5 · THE ANSWER --------------------------------------------------------

// scheduleBuildAnswer assembles S5.
func scheduleBuildAnswer(in scheduleBuildInput) ScheduleAnswer {
	members := scheduleMembers(in)
	if in.IsGM {
		a := ScheduleAnswer{
			Director: true,
			Title:    "Who has answered",
			Sub: fmt.Sprintf("%d of %d · recomputed from these rows",
				scheduleAnswered(members), len(members)),
			Chips:   scheduleNeed(true, "RSVP answers an event, not a week"),
			Caption: scheduleAnswerDirectorCaption(),
		}
		for _, m := range members {
			row := scheduleRosterRowFor(in, m)
			if m.Answer == "—" {
				// A DIFFERENT CONDITION FROM THE ZONE REPAIR: this row is
				// awaiting an ANSWER, not a timezone, and this whole branch is
				// already inside `if in.IsGM`. So it keeps `Ask →` and the
				// roster unconditionally — the audience gate [GR-15] adds is
				// the enclosing branch, not a second check.
				row.AskHref, row.AskLabel = benchRsvpAskHref(in.CampaignID), "Ask →"
				a.Awaiting = append(a.Awaiting, row)
				continue
			}
			a.Rows = append(a.Rows, row)
		}
		return a
	}

	// THE PLAYER'S HALF.
	var me scheduleMember
	for _, m := range members {
		if m.UserID == in.ViewerID {
			me = m
		}
	}
	a := ScheduleAnswer{
		Title:   "Your answer",
		Sub:     scheduleAnswerSub(in),
		Caption: scheduleAnswerPlayerCaption(),
	}
	if me.Answer == "" || me.Answer == "—" {
		a.Unanswered = true
		a.Headline = "You: no answer"
		a.HeadlineTone = "warn"
	} else {
		a.Headline = "You: " + me.Answer
		if me.Answer == "in" {
			a.HeadlineTone = "ok"
		}
	}
	// The tri-state posts to the EXISTING Player+ RSVP route. Part B adds no
	// answer route: this is a RESTYLE, not a build.
	if in.EventID != "" && in.CalendarID != "" {
		a.Form = &ScheduleAnswerForm{
			Action: fmt.Sprintf("/campaigns/%s/calendars/%s/events/%s/rsvp",
				in.CampaignID, in.CalendarID, in.EventID),
			CSRFToken: in.CSRFToken,
			My:        me.Answer,
			Options: []ScheduleAnswerOption{
				{Value: RSVPYes, Label: "Yes", Pressed: me.Answer == "in"},
				{Value: RSVPMaybe, Label: "Maybe", Pressed: me.Answer == "maybe"},
				{Value: RSVPNo, Label: "No", Pressed: me.Answer == "out"},
			},
			OutWeekPopID: "sc-pop-outweek",
			SuggestHref:  scheduleHref(in.CampaignID, in.Base, "sug", "open"),
			SuggestOpen:  in.SugOpen,
			SuggestNote: ScheduleCaption{
				scheduleSay("0 / 500 · or tick the windows in "),
				// THE OTHER WAY TO SAY THE SAME THING, named where a member
				// reading the dock can act on it — the drawing bolds it for
				// exactly that reason.
				scheduleLead("My availability"),
				scheduleSay(" below; an offer only ever adds."),
			},
		}
	}
	if !in.MailConfigured {
		a.Foot = mailNotConfiguredLine
	}
	return a
}
