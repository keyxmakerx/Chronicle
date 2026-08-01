// schedule_copy.go — S4's builder, the page's printed sentences, and the small
// helpers the builders share (C-CALV4-RSVP-P8 Part B).
//
// THE SENTENCES ARE THE SURFACE. Every caption on this page states a fact the
// numbers above it cannot state about themselves — what the score cannot
// include, why the fine and coarse reads disagree, why the counts are
// recomputed rather than stored. They live here, in one place, because a caption
// that drifts out of agreement with the arithmetic is worse than no caption at
// all, and one file is where that is checkable.
package calendar

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// --- S4 · THE PAINTER -------------------------------------------------------

// scheduleBuildPainter assembles S4. Filled in by the write stage; this is the
// frame and its two refusals.
func scheduleBuildPainter(in scheduleBuildInput) SchedulePainter {
	p := SchedulePainter{
		Title:    "My availability · week of " + in.WeekStart.Format("2 Jan"),
		Frame:    schedulePainterFrame(in),
		ZoneHref: "/settings/profile",
	}
	// THE DEGRADED FLOOR IS A NAMED FAULT, NOT A BLANK. When the scheduler seam
	// is not answering, availability entry genuinely cannot be offered, and the
	// box says so — the same rule the addon fault box obeys (§2.4), applied to
	// the condition that can actually occur behind this route's addon guard.
	if in.Avail == nil {
		p.Fault = &ScheduleFault{
			Headline: "Availability entry is not answering",
			Detail: "The scheduler could not be reached for this campaign, so your week " +
				"cannot be painted right now. Nothing you have already saved has changed.",
		}
		return p
	}
	if in.Zone == "" {
		// AVAILABILITY IS A ZONE-LOCAL WALL CLOCK, never a UTC instant. Saving a
		// week against a zone nobody set would store a guess, and every reader's
		// conversion of it would inherit the guess.
		p.Fault = &ScheduleFault{
			Headline: "Set a time zone before painting your week",
			Detail: "Your availability is saved as a wall clock in your own zone, and no zone " +
				"is set for you or for this calendar — so there is nothing to save it against " +
				"yet. Set yours in your profile and this grid fills in.",
		}
		return p
	}
	p.Form = schedulePaintForm(in)
	return p
}

// schedulePaintForm builds the painted grid and its controls.
//
// ── THE WRITE PATH IS THE SCHEDULER'S, AND IT IS TWO ROUTES BECAUSE THE
//    SEGMENT MEANS TWO THINGS ────────────────────────────────────────────────
//
//	Every week      → PUT …/availability/mine        the member's NORMAL HOURS
//	This week only  → PUT …/availability/exceptions  a DATE EXCEPTION
//
// Both already ship, both are Player+, and both enforce the composition
// invariant in the scheduler's own service — "an offer only ever adds, and never
// downgrades an hour already marked preferred". Part B writes through them and
// adds neither a route nor a migration: a second writer would be a second place
// to get that invariant wrong.
func schedulePaintForm(in scheduleBuildInput) *SchedulePaintForm {
	// THE VIEWER'S OWN WEEK, FROM THE READ THAT IS ABOUT THEM. Not from
	// Avail.Lanes: a player's overlay carries no member at all, so seeding the
	// Painter from it would leave every player staring at an empty grid over
	// their own saved availability. One path, both roles.
	me := scheduleMember{Lanes: in.OwnLanes}
	for _, m := range scheduleMembers(in) {
		if m.UserID == in.ViewerID {
			me.Axis, me.Pattern, me.Token = m.Axis, m.Pattern, m.Token
		}
	}
	// A member with no roster row still paints — the identity channel simply
	// falls back to the neutral one rather than the grid losing its marks.
	if me.Pattern == "" {
		me.Axis, me.Pattern = benchRsvpIdentity(0)
	}

	f := &SchedulePaintForm{
		SaveURL:       "/campaigns/" + in.CampaignID + "/availability/mine",
		ExceptionsURL: "/campaigns/" + in.CampaignID + "/availability/exceptions",
		CSRFToken:     in.CSRFToken,
		Zone:          in.Zone,
		WeekLabel:     "week of " + in.WeekStart.Format("2 Jan"),
		Axis:          me.Axis,
		Pattern:       me.Pattern,
		Scope:         in.Scope,
		PrefOpen:      in.PrefOpen,
		PrefNote: "Preferred always sits inside available — the server composes them, so " +
			"marking an hour preferred also marks it playable.",
		Foot: schedulePaintFoot(in.Scope),
	}
	f.ScopeNote = schedulePaintScopeNote(in.Scope)
	f.ScopeOptions = []ScheduleToggle{
		{Key: "week", Label: "This week only", Pressed: in.Scope == "week",
			Href: scheduleHref(in.CampaignID, in.Base, "scope", "week")},
		{Key: "recurring", Label: "Every week", Pressed: in.Scope == "recurring",
			Href: scheduleHref(in.CampaignID, in.Base, "scope", "recurring")},
	}

	for h := in.BandFrom; h < in.BandTo; h++ {
		f.Hours = append(f.Hours, ScheduleHourHead{
			Label: strconv.Itoa(h),
			Major: (h-in.BandFrom)%6 == 0 && h != in.BandFrom,
		})
	}
	f.Days = schedulePaintDays(in, me, "free")
	f.PrefDays = schedulePaintDays(in, me, "pref")

	windows, minutes := 0, 0
	for _, g := range me.Lanes {
		if g.State == AvailPreferred {
			continue
		}
		windows++
		minutes += g.EndMinute - g.StartMinute
	}
	f.Summary = fmt.Sprintf("%d %s this week · %d h %d m · cap is 8 windows per week on the offer path",
		windows, schedulePlural(windows, "window", "windows"), minutes/60, minutes%60)
	if windows == 0 {
		f.Empty = "You have not marked any hours. The schedule cannot tell “never answered” " +
			"from “busy all week”, so one marked window is worth more than none."
	}

	// AN UNBUILT AFFORDANCE IS DIRECTOR-TIER. For a player it is ABSENT, not
	// disabled: a dead button whose only explanation is a hover is the same
	// failure the disabled primary button exists to avoid, and scaffolding for a
	// gap nobody is asking that player about is the Director's business.
	// Permission is absence, and it binds scaffolding too.
	if in.IsGM {
		f.CopyWeek = &BenchAction{
			Label: "Copy last week", NeedsBackend: true,
			Title: "there is no copy-week write path",
		}
		// THE ONE DASHED BAND ON THIS WHOLE PAGE. Dashed means "not built yet"
		// and NOTHING else — an unavailable feature prints a fault, never a
		// dashed reserve (ledger #21).
		f.Reserve = &ScheduleReserve{
			Head: "Not built yet",
			Body: "importing from an external calendar lands here",
		}
	}
	return f
}

// schedulePaintDays builds the seven day rows, seeded from the viewer's own
// saved week so what they see is what they already told the campaign.
func schedulePaintDays(in scheduleBuildInput, me scheduleMember, kind string) []SchedulePaintDay {
	out := []SchedulePaintDay{}
	for d := 0; d < scheduleDayCount(in); d++ {
		date := scheduleDayDate(in, d)
		day := SchedulePaintDay{DayKey: date, Label: date}
		if t, err := timeParseISO(date); err == nil {
			day.Label = t.Format("Mon 2 Jan")
			day.Weekday = int(t.Weekday())
		}
		for h := in.BandFrom; h < in.BandTo; h++ {
			on := scheduleFreeAt(me, d, h)
			what := "I can play"
			if kind == "pref" {
				on = schedulePrefAt(me, d, h)
				what = "I would prefer this"
			}
			day.Cells = append(day.Cells, SchedulePaintCell{
				ID:      kind + "-" + date + "-" + strconv.Itoa(h),
				Name:    kind,
				Value:   date + "T" + scheduleHour(h),
				Checked: on,
				Major:   (h-in.BandFrom)%6 == 0 && h != in.BandFrom,
				DayKey:  date,
				Hour:    h,
				Label:   day.Label + ", " + scheduleHour(h) + " — " + what,
			})
		}
		out = append(out, day)
	}
	return out
}

// schedulePaintScopeNote says what THIS scope's marks mean, in one sentence,
// beside the control that sets it. Two scopes, two tables, two sentences.
func schedulePaintScopeNote(scope string) string {
	if scope == "recurring" {
		return "sets your normal hours — every week from now on, until you change them"
	}
	return "marks a date exception — it replaces that day's usual pattern"
}

// schedulePaintFoot is the compose rule, printed. It is the one thing a member
// cannot work out by looking at the grid.
func schedulePaintFoot(scope string) string {
	s := "Offering a window only ever adds. It can never take away a time you already gave, " +
		"and it never downgrades an hour you already marked preferred."
	if scope == "week" {
		s += " A window you set on a specific date replaces that day's usual pattern. Clearing " +
			"it puts the usual pattern back."
	}
	return s
}

// --- the printed sentences --------------------------------------------------

// scheduleFrameLine is the panel header's frame: which week, in whose zone.
func scheduleFrameLine(in scheduleBuildInput) string {
	week := "week of " + in.WeekStart.Format("2 Jan 2006")
	if in.ZoneLeaf == "" {
		return week + " · no time zone is set"
	}
	return week + " · times in " + in.ZoneLeaf
}

// scheduleMatrixFrame is the matrix head's frame: WHICH DAYS, WHICH HOURS, and
// in whose zone — `Mon 20 – Sun 26 Jul · 16:00–24:00 · times in Chicago`.
//
// IT IS NOT scheduleFrameLine, AND THE DIFFERENCE IS THE BAND. Every other panel
// on this page is about a week and the generic "week of 20 Jul 2026" says
// everything they need. This one is a grid of HOURS, its lanes can report "3
// windows outside 16–24", and a reader who cannot see what the band is has no
// way to act on that sentence. The drawing prints both; so does this.
func scheduleMatrixFrame(in scheduleBuildInput) string {
	line := scheduleDaySpan(in) + " · " +
		fmt.Sprintf("%02d:00–%02d:00", in.BandFrom, in.BandTo)
	if in.ZoneLeaf == "" {
		// A missing zone is a NAMED absence, never a reason to drop the two
		// facts that do not depend on it.
		return line + " · no time zone is set"
	}
	return line + " · times in " + in.ZoneLeaf
}

// scheduleDaySpan names the columns' own first and last day.
//
// IT READS THE OVERLAY, never a literal seven: a calendar's week length is its
// own business, and this line is the one place a reader would notice the page
// disagreeing with the grid beneath it. The month is printed once when both ends
// share one and twice when they do not, because "Mon 28 – Sun 3 Aug" is a date
// range nobody can parse.
func scheduleDaySpan(in scheduleBuildInput) string {
	days := scheduleDayCount(in)
	first, ferr := timeParseISO(scheduleDayDate(in, 0))
	last, lerr := timeParseISO(scheduleDayDate(in, days-1))
	if ferr != nil || lerr != nil {
		return "week of " + in.WeekStart.Format("2 Jan 2006")
	}
	if first.Equal(last) {
		return first.Format("Mon 2 Jan")
	}
	if first.Month() == last.Month() && first.Year() == last.Year() {
		return first.Format("Mon 2") + " – " + last.Format("Mon 2 Jan")
	}
	return first.Format("Mon 2 Jan") + " – " + last.Format("Mon 2 Jan")
}

// schedulePainterFrame states which zone the viewer's OWN marks are stored in.
func schedulePainterFrame(in scheduleBuildInput) string {
	if in.ZoneLeaf == "" {
		return "you have not set a time zone — set one and your week can be saved against it"
	}
	return "saved in " + in.ZoneLeaf + " — your zone"
}

// scheduleSlotLabel names the event whose answers the page is reading.
func scheduleSlotLabel(in scheduleBuildInput) string {
	if in.Session == nil {
		return "No session collecting RSVPs"
	}
	if !in.Session.Anchored {
		// A session with no anchored instant has no clock to state, and stating
		// one anyway is the exact error the zone-less member's empty clock
		// refuses to make.
		return in.Session.Name + " · no time set"
	}
	if in.Zone == "" {
		return in.Session.Name
	}
	loc, err := time.LoadLocation(in.Zone)
	if err != nil {
		return in.Session.Name
	}
	// THE SLOT IS NAMED WITH ITS WEEKDAY, ITS CLOCK AND ITS ZONE, because "which
	// slot are we answering" is the question every row underneath is an answer
	// to — and on a page whose other four surfaces are all about a WEEK, an hour
	// with no day attached is the one ambiguity this head cannot afford. The
	// drawing prints `slot Sat 19:00 Chicago` and the weekday is the half a
	// reader checks first.
	return in.Session.Name + " · slot " + in.Session.Instant.In(loc).Format("Mon 15:04") +
		" " + in.ZoneLeaf
}

// scheduleAnswerSub is the player's answer header.
//
// IT NAMES THE EVENT, ITS SLOT, AND — WHEN THEY DIFFER — THE VIEWER'S OWN CLOCK.
// The whole subject of this page is that one instant is a different clock for
// each person reading it, so a player's own clock for the slot they are being
// asked about is the single fact they came for; the drawn head prints it as
// `· your 01:00`.
//
// IT NAMES THE EVENT AND NOT THE HIGHLIGHTED CANDIDATE, which is where this
// parts from the drawing on purpose: the tri-state below posts to an EVENT's
// RSVP, and this panel's own chip reads `RSVP answers an event, not a week`. A
// head naming the selected window would contradict the chip directly under it.
//
// The clause appears only when the viewer's clock DIFFERS from the printed one.
// `19:00 Chicago · your 19:00` is not a second fact, it is the same fact twice,
// and a head that repeats itself teaches the reader to stop reading it.
func scheduleAnswerSub(in scheduleBuildInput) string {
	if in.Session == nil {
		return "no slot chosen yet"
	}
	// An unanchored session HAS no clock, and inventing one here would be the
	// same error the zone-less member's literally empty clock refuses to make.
	if !in.Session.Anchored || in.Zone == "" {
		return in.Session.Name
	}
	loc, err := time.LoadLocation(in.Zone)
	if err != nil {
		return in.Session.Name
	}
	slot := in.Session.Instant.In(loc).Format("15:04")
	sub := in.Session.Name + " · " + slot + " " + in.ZoneLeaf

	// The viewer's own clock comes from the SAME helper the roster row prints
	// from, so the head and the row can never disagree about what time it is
	// for the person reading them.
	for _, m := range scheduleMembers(in) {
		if m.UserID != in.ViewerID {
			continue
		}
		clock, nextDay, ok := scheduleLocalHourAt(in, m)
		if !ok || clock == slot {
			break
		}
		sub += " · your " + clock
		if nextDay {
			// The roster marks the roll-over beside the same clock; a head that
			// dropped it would print an hour on the wrong day.
			sub += " +1d"
		}
		break
	}
	return sub
}

// scheduleVerdictCaption states what the score is and — more importantly — what
// it CANNOT include. The second half is PERMANENT: this grid shows availability
// only, so a window may collide with something already booked (conflict
// detection is ledger #16 and explicitly out of scope for this slice; the
// caption is how the surface stays honest about it rather than implying
// otherwise).
func scheduleVerdictCaption(ranked bool) ScheduleCaption {
	c := ScheduleCaption{}
	if ranked {
		c = append(c,
			scheduleLead("How it ranks:"),
			scheduleSay(" heads free at the top of the hour first, then how many marked the "+
				"hour "),
			scheduleWord("preferred"),
			scheduleSay(", then how many people it wakes before 08:00 or keeps up past 23:00 in "+
				"their own zone. Ties break toward the later date, so an earlier window is never "+
				"dropped by a coin flip. "),
		)
	}
	return append(c,
		scheduleLead("What the score cannot include:"),
		scheduleSay(" this grid shows availability only — it does not "+
			"know what is already on the calendar, so a window may collide with something already "+
			"booked. One week at a time: the schedule reads a week, not a month. Cards sit in date "+
			"order and never move; the number on the left is the rank."),
	)
}

// scheduleMatrixCaption is the marks' key and the two disagreements the numbers
// cannot state about themselves — ONE flowing paragraph, as the drawing returns
// one. Three notes in three blocks read as three unrelated remarks; the mockup's
// own producer joins them, and it is one key to one grid.
func scheduleMatrixCaption(in scheduleBuildInput, wrapped bool) ScheduleCaption {
	c := ScheduleCaption{
		scheduleLead("The marks:"),
		scheduleSay(" a hollow outline is an hour someone can play; the same outline filled is an "+
			"hour they'd "),
		scheduleWord("prefer"),
		scheduleSay("; nothing at all means no free time saved. Every outline carries "+
			"that person's own dash pattern, so the grid reads with the colour off. "),
		scheduleLead("Fine and coarse disagree, on purpose:"),
		scheduleSay(" the outlines are minute-accurate, and the count "+
			"lane samples the top of the hour. Someone free from 18:30 raises their own lane at "+
			"18:30 but does not raise the 18:00 count. "),
		scheduleLead("This grid shows availability only"),
		scheduleSay(" — it does not know what is already on the calendar."),
	}
	if wrapped {
		c = append(c,
			scheduleSay(" "),
			scheduleLead("Identity wraps after eight"),
			scheduleSay(" — the ninth lane reuses the first hue "+
				"with a different pattern, and the roster below is the authority on who is who."))
	}
	_ = in
	return c
}

// scheduleRosterCaption names why the counts are recomputed, which is the one
// fact the numbers cannot state about themselves.
func scheduleRosterCaption() ScheduleCaption {
	return ScheduleCaption{
		scheduleLead("Counts are recomputed from these rows"),
		scheduleSay(", not from the stored tally, because the stored "+
			"tally still counts people who have left the campaign. Zone names print the last part of "+
			"the IANA identifier — hover for the full one. Chronicle has no abbreviation helper, so "+
			"nothing here will ever say “CDT” until it does. An answer of "),
		scheduleWord("in"),
		scheduleSay(", "),
		scheduleWord("maybe"),
		scheduleSay(" or "),
		scheduleWord("out"),
		scheduleSay(" answers "),
		scheduleLead("an event"),
		scheduleSay(", not a week — the two only line up when the calendar is anchored to "+
			"real time."),
	}
}

func scheduleAnswerDirectorCaption() ScheduleCaption {
	return ScheduleCaption{scheduleSay(
		"A Director never sets someone else's answer. “Awaiting reply” is this roster " +
			"minus the people who replied — there is no invitee table, so on a player's page this " +
			"group does not exist at all.")}
}

func scheduleAnswerPlayerCaption() ScheduleCaption {
	return ScheduleCaption{
		scheduleSay("All five are words, not glyphs. "),
		scheduleLead("Out just this week"),
		scheduleSay(" writes two things: it records your RSVP as "),
		scheduleWord("no"),
		scheduleSay(" and marks you unavailable for this whole real-world week — days you already "+
			"set by hand are left alone. "),
		scheduleLead("Suggest a better time"),
		scheduleSay(" records your answer as "),
		scheduleWord("maybe"),
		scheduleSay(", never yes and never no, because a note without a status would otherwise be "+
			"counted as a decision you did not make."),
	}
}

// --- the caption runs -------------------------------------------------------
//
// Three constructors, so a caption reads on the page of source the way it reads
// on the screen and the emphasis cannot drift into a markup string.

// scheduleSay is plain prose.
func scheduleSay(s string) ScheduleRun { return ScheduleRun{Text: s} }

// scheduleLead is the drawn BOLD lead-in — the phrase a reader scans for.
func scheduleLead(s string) ScheduleRun { return ScheduleRun{Text: s, Em: "b"} }

// scheduleWord is a vocabulary word the caption QUOTES rather than uses:
// `preferred`, `in`, `maybe`, `out`, `no`. Drawn italic so "an answer of in,
// maybe or out" cannot be read as a sentence that is itself simply in.
func scheduleWord(s string) ScheduleRun { return ScheduleRun{Text: s, Em: "i"} }

// --- small shared helpers ---------------------------------------------------

// timeParseISO parses a YYYY-MM-DD column key.
func timeParseISO(s string) (time.Time, error) {
	return time.Parse("2006-01-02", s)
}

// atoiSafe is strconv.Atoi over trimmed input, so a stray space in a query
// parameter is not a different answer from no parameter at all.
func atoiSafe(s string) (int, error) { return strconv.Atoi(strings.TrimSpace(s)) }

// scheduleLocalHourAt renders a member's own clock for the resolved session's
// instant, or ok=false when there is nothing honest to print.
func scheduleLocalHourAt(in scheduleBuildInput, m scheduleMember) (clock string, nextDay bool, ok bool) {
	if in.Session == nil || !in.Session.Anchored || m.TZ == "" {
		return "", false, false
	}
	loc, err := time.LoadLocation(m.TZ)
	if err != nil {
		return "", false, false
	}
	local := in.Session.Instant.In(loc)
	if in.Zone != "" {
		if here, herr := time.LoadLocation(in.Zone); herr == nil {
			nextDay = local.Day() != in.Session.Instant.In(here).Day()
		}
	}
	return local.Format("15:04"), nextDay, true
}

// scheduleAntisocialClock reads the warn threshold off a printed HH:MM.
func scheduleAntisocialClock(clock string) bool {
	h, err := strconv.Atoi(strings.SplitN(clock, ":", 2)[0])
	if err != nil {
		return false
	}
	return scheduleAntisocial(h)
}

// --- templ view helpers -----------------------------------------------------

// scheduleToneClass maps an honesty line's tone onto the sheet's ink modifiers.
func scheduleToneClass(tone string) string {
	switch tone {
	case "warn":
		return "warn"
	case "good":
		return "good"
	case "mute":
		return "mute"
	default:
		return ""
	}
}

// scheduleInkClass is the Answer headline's ink: the viewer's own answer
// coloured by what it says, never by a badge beside it.
func scheduleInkClass(tone string) string {
	switch tone {
	case "warn":
		return "warnink"
	case "ok":
		return "okink"
	default:
		return ""
	}
}

// scheduleGridStyle emits --cols SERVER-SIDE as the column count. CSS never
// multiplies inside repeat() and no stylesheet on this surface writes a week
// length or an hour count: a calendar's week length is its own business.
func scheduleGridStyle(cols int) string {
	return "--cols:" + strconv.Itoa(cols)
}

// scheduleMarkStyle positions one minute-accurate mark across its column.
func scheduleMarkStyle(m ScheduleMark) string {
	return "--m0:" + m.From + "%;--m1:" + m.To + "%"
}

// scheduleDensityStyle maps a column's free count onto the bar's own two custom
// properties. The FLOOR is deliberate: a column where nobody is free still draws
// a bar, at the lowest step, because a vanished bar reads as "no data" and this
// lane's whole job is to distinguish "nobody can make it" from "nobody has said".
func scheduleDensityStyle(d ScheduleDensity) string {
	t := d.Total
	if t <= 0 {
		t = 1
	}
	return "--f:" + strconv.Itoa(d.Free) + ";--t:" + strconv.Itoa(t)
}

// scheduleBracketStyle positions the derived window's bracket over the columns
// it spans. Grid LINES, not day indices — column 1 is the identity column.
func scheduleBracketStyle(b *ScheduleBracket) string {
	if b == nil {
		return ""
	}
	return "--from:" + strconv.Itoa(b.Start) + ";--to:" + strconv.Itoa(b.End)
}

// scheduleBool renders an aria-pressed value.
func scheduleBool(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// schedulePaintGridStyle sizes the painted grid. The identity column is the day
// label and the SAY end is zero-width — the Painter has no verdict column,
// because the member painting it is not being told anything.
func schedulePaintGridStyle(hours int) string {
	return "--cols:" + strconv.Itoa(hours) + ";--idw:118px;--sayw:0px;--colmin:34px"
}
