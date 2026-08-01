// schedule_build.go — the five pure builders (C-CALV4-RSVP-P8 Part B).
//
// Each takes the same fully-resolved scheduleBuildInput and returns one
// surface's view model. Nothing here reads a repository, a request or a clock
// it was not handed, which is what lets the oracle tests reproduce every number
// on the page from the same visible set the viewer got.
package calendar

import "fmt"

// scheduleBuildVerdict assembles S1.
func scheduleBuildVerdict(in scheduleBuildInput) ScheduleVerdict {
	members := scheduleMembers(in)
	v := ScheduleVerdict{
		Title: "When to play",
		Frame: scheduleFrameLine(in),
		Mode: "tallies only — names are not in your view",
	}
	if in.IsGM {
		v.Mode = "ranked from this week's saved availability"
	}
	if len(members) == 0 {
		v.Headline = "Pick a window"
		v.Rec = "this campaign has no members to rank a week for"
		v.Caption = scheduleVerdictCaption(false)
		return v
	}
	v.Chips = scheduleChips(
		scheduleNeed(in.IsGM, "does not know what's already booked"),
	)
	v.Headline = "Pick a window"
	v.Rec = "nothing is saved yet, so nothing can be ranked"
	v.Fault = &ScheduleCandidateFault{
		Headline: "No availability saved by anyone",
		Detail: "the matrix below is empty for the same reason — nobody has filled in a week yet",
	}
	if in.IsGM {
		v.Fault.Chip = "nothing to rank"
	}
	v.WarnLine, v.WarnTone = "nobody has saved a week yet", "warn"
	v.Caption = scheduleVerdictCaption(false)
	return v
}

// scheduleBuildMatrix assembles S2.
func scheduleBuildMatrix(in scheduleBuildInput) ScheduleMatrix {
	m := ScheduleMatrix{
		Title:    "Who is free when",
		Frame:    scheduleFrameLine(in),
		IdentCap: "everyone",
		SayCap:   "time",
		Scope: "anonymous totals only — your own week is in “My availability” above",
	}
	if in.IsGM {
		m.IdentCap = "who is free"
		m.Scope = "per-member lanes · owner / co-DM only"
	}
	m.Denominator = fmt.Sprintf("free of %d in the campaign", len(in.Roster))
	m.Zero = "Nobody has filled in a week yet. The grid works the moment one person does — " +
		"one marked window is worth more than none."
	m.Captions = scheduleMatrixCaptions(in, false)
	return m
}

// scheduleBuildRoster assembles S3.
func scheduleBuildRoster(in scheduleBuildInput) ScheduleRoster {
	r := ScheduleRoster{
		SlotLabel: scheduleSlotLabel(in),
		Sub: fmt.Sprintf("%d in the campaign · %d answered",
			len(in.Roster), scheduleAnswered(scheduleMembers(in))),
		Caption: scheduleRosterCaption(),
	}
	return r
}

// scheduleBuildPainter assembles S4.
func scheduleBuildPainter(in scheduleBuildInput) SchedulePainter {
	p := SchedulePainter{
		Title:    "My availability · week of " + in.WeekStart.Format("2 Jan"),
		Frame:    schedulePainterFrame(in),
		ZoneHref: "/settings/profile",
	}
	// THE DEGRADED FLOOR IS A NAMED FAULT, NOT A BLANK. When the scheduler seam
	// is not answering, availability entry genuinely cannot be offered, and the
	// box says so and says who can look at it — the same rule the addon fault
	// box obeys (§2.4).
	if in.Avail == nil {
		p.Fault = &ScheduleFault{
			Headline: "Availability entry is not answering",
			Detail: "The scheduler could not be reached for this campaign, so your week " +
				"cannot be painted right now. Nothing you have already saved has changed.",
		}
	}
	return p
}

// scheduleBuildAnswer assembles S5.
func scheduleBuildAnswer(in scheduleBuildInput) ScheduleAnswer {
	if in.IsGM {
		return ScheduleAnswer{
			Director: true,
			Title:    "Who has answered",
			Sub: fmt.Sprintf("%d of %d · recomputed from these rows",
				scheduleAnswered(scheduleMembers(in)), len(in.Roster)),
			Chips:   scheduleNeed(true, "RSVP answers an event, not a week"),
			Caption: scheduleAnswerDirectorCaption(),
		}
	}
	return ScheduleAnswer{
		Title:   "Your answer",
		Sub:     scheduleAnswerSub(in),
		Caption: scheduleAnswerPlayerCaption(),
	}
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
// Availability is a zone-local wall clock, never a UTC instant, so this sentence
// is load-bearing rather than decorative.
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
// it CANNOT include. The second half is permanent: this grid shows availability
// only, so a window may collide with something already booked (ledger #16 is
// explicitly out of scope for this slice, and the caption is how the surface
// stays honest about it rather than pretending otherwise).
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

// --- templ view helpers -----------------------------------------------------

// scheduleToneClass maps an honesty line's tone onto the sheet's ink modifiers.
// Kept beside the builders rather than in the templ file because it is the same
// two-way mapping benchToneClass performs, and a second copy in a second place
// is how the two drift.
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
