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
	}
	return p
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
	return in.Session.Name
}

// scheduleAnswerSub is the player's answer header.
func scheduleAnswerSub(in scheduleBuildInput) string {
	if in.Session == nil {
		return "no slot chosen yet"
	}
	return in.Session.Name
}

// scheduleVerdictCaption states what the score is and — more importantly — what
// it CANNOT include. The second half is PERMANENT: this grid shows availability
// only, so a window may collide with something already booked (conflict
// detection is ledger #16 and explicitly out of scope for this slice; the
// caption is how the surface stays honest about it rather than implying
// otherwise).
func scheduleVerdictCaption(ranked bool) string {
	key := ""
	if ranked {
		key = "How it ranks: heads free at the top of the hour first, then how many marked the " +
			"hour preferred, then how many people it wakes before 08:00 or keeps up past 23:00 in " +
			"their own zone. Ties break toward the later date, so an earlier window is never " +
			"dropped by a coin flip. "
	}
	return key + "What the score cannot include: this grid shows availability only — it does not " +
		"know what is already on the calendar, so a window may collide with something already " +
		"booked. One week at a time: the schedule reads a week, not a month. Cards sit in date " +
		"order and never move; the number on the left is the rank."
}

// scheduleMatrixCaptions are the marks' key and the two disagreements the
// numbers cannot state about themselves.
func scheduleMatrixCaptions(in scheduleBuildInput, wrapped bool) []string {
	caps := []string{
		"The marks: a hollow outline is an hour someone can play; the same outline filled is an " +
			"hour they'd prefer; nothing at all means no free time saved. Every outline carries " +
			"that person's own dash pattern, so the grid reads with the colour off.",
		"Fine and coarse disagree, on purpose: the outlines are minute-accurate, and the count " +
			"lane samples the top of the hour. Someone free from 18:30 raises their own lane at " +
			"18:30 but does not raise the 18:00 count.",
		"This grid shows availability only — it does not know what is already on the calendar.",
	}
	if wrapped {
		caps = append(caps, "Identity wraps after eight — the ninth lane reuses the first hue "+
			"with a different pattern, and the roster below is the authority on who is who.")
	}
	_ = in
	return caps
}

// scheduleRosterCaption names why the counts are recomputed, which is the one
// fact the numbers cannot state about themselves.
func scheduleRosterCaption() string {
	return "Counts are recomputed from these rows, not from the stored tally, because the stored " +
		"tally still counts people who have left the campaign. Zone names print the last part of " +
		"the IANA identifier — hover for the full one. Chronicle has no abbreviation helper, so " +
		"nothing here will ever say “CDT” until it does. An answer of in, maybe or out " +
		"answers an event, not a week — the two only line up when the calendar is anchored to " +
		"real time."
}

func scheduleAnswerDirectorCaption() string {
	return "A Director never sets someone else's answer. “Awaiting reply” is this roster " +
		"minus the people who replied — there is no invitee table, so on a player's page this " +
		"group does not exist at all."
}

func scheduleAnswerPlayerCaption() string {
	return "All five are words, not glyphs. Out just this week writes two things: it records your " +
		"RSVP as no and marks you unavailable for this whole real-world week — days you already " +
		"set by hand are left alone. Suggest a better time records your answer as maybe, never " +
		"yes and never no, because a note without a status would otherwise be counted as a " +
		"decision you did not make."
}

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
