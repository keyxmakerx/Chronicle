package calendar

// schedule_oracle_test.go — the count oracle for the /schedule page
// (C-CALV4-RSVP-P8 Part B, WG-9's signed fixture shape).
//
// THE FIXTURE IS WG-9's, and its load-bearing member is the DEPARTED one: five
// members in the campaign, three answered, and a sixth stored answer row
// belonging to somebody who has left. Every number this page prints must be
// recomputed from the visible roster, so the departed row must change NO number
// and appear in NO list — and the only way to prove that is to put one in the
// input and read the output.
//
// THE SECOND CLAIM IS THE ONE THIS SLICE ADDS: rank 1 of the Verdict IS the
// Bench's derived window. They are one click apart and they may not disagree
// about when to play. It is asserted through both public builders over the same
// availability, not by reading the shared helper twice.

import (
	"strings"
	"testing"
	"time"
)

// scheduleOracleWeek is the Monday every fixture below is anchored to.
func scheduleOracleWeek() time.Time {
	return time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
}

// scheduleOracleRoster is WG-9's five: an owner, a co-DM Scribe, two players
// with zones and one player with NONE (the zone-less state is first-class and
// has to survive every builder).
func scheduleOracleRoster() []BenchRosterMember {
	return []BenchRosterMember{
		{UserID: "u-kael", Name: "Kael Ashvane", Role: "Owner", IsOwner: true, TZ: "America/Chicago"},
		{UserID: "u-bryn", Name: "Bryn Aldercroft", Role: "Scribe", IsCoDM: true, TZ: "America/New_York"},
		{UserID: "u-tam", Name: "Tam Orwick", Role: "Player", TZ: "Europe/London"},
		{UserID: "u-nissa", Name: "Nissa Fen", Role: "Player", TZ: "Asia/Tokyo"},
		{UserID: "u-rell", Name: "Rell Vantry", Role: "Player"},
	}
}

// scheduleOracleAnswers is three real answers PLUS one belonging to a member who
// has left the campaign. The departed row is the whole point.
func scheduleOracleAnswers() map[string]string {
	return map[string]string{
		"u-kael":     RSVPYes,
		"u-bryn":     RSVPYes,
		"u-tam":      RSVPMaybe,
		"u-departed": RSVPYes,
	}
}

// scheduleOracleAvail builds a week overlay with a clear peak on Saturday
// (day index 5) at 19:00 and lesser peaks elsewhere, plus minute-accurate lanes
// for four of the five members. Rell has NONE — which is ledger #3's case and
// must read as an unknown, never as a refusal.
//
// ── THE PER-HOUR COUNTS ARE DERIVED FROM THE LANES, NOT HAND-WRITTEN ───────
//
// They used to be a hand-written stub that filled in only the peak hours,
// because WEEK zoom reads nothing else: a column is a whole day and the lane
// prints that day's maximum. DAY zoom reads EVERY hour of one day, and the stub
// disagreed with its own lanes at four of them — most visibly Saturday 21:00,
// where the stub said 4 and the lanes say 3 (Nissa's window closes at 21:00).
// A fixture that can draw a page saying "nobody is free at 17:00" directly above
// a lane showing an outline at 17:00 is an instrument that manufactures defects,
// which is the failure this slice already hit once with the harness's own
// padding. Production's overlay computes both from one source; so does this now.
//
// It is BEHAVIOUR-PRESERVING for every week-zoom reading: each day's peak, and
// the hour it falls at, are identical before and after (checked by
// TestScheduleOracle_FixtureCountsAgreeWithItsOwnLanes).
func scheduleOracleAvail() *BenchAvailability {
	day := func(date string, free, prefer map[int]int) BenchAvailabilityDay {
		f := make([]int, 24)
		p := make([]int, 24)
		for h, n := range free {
			f[h] = n
		}
		for h, n := range prefer {
			p[h] = n
		}
		return BenchAvailabilityDay{Date: date, Free: f, Prefer: p}
	}
	seg := func(d, s, e int, state string) BenchLaneSegment {
		return BenchLaneSegment{DayIndex: d, StartMinute: s, EndMinute: e, State: state}
	}
	av := &BenchAvailability{
		WeekStart: "2026-07-20",
		Days: []BenchAvailabilityDay{
			day("2026-07-20", map[int]int{19: 1}, nil),
			day("2026-07-21", map[int]int{16: 1, 17: 1, 18: 1}, nil),
			day("2026-07-22", map[int]int{19: 3, 20: 3, 21: 3}, map[int]int{19: 1, 20: 1, 21: 1}),
			day("2026-07-23", map[int]int{17: 1, 18: 1, 19: 1}, nil),
			day("2026-07-24", map[int]int{21: 2, 22: 2, 23: 2}, nil),
			day("2026-07-25", map[int]int{19: 4, 20: 4, 21: 4}, map[int]int{19: 2, 20: 2, 21: 2}),
			day("2026-07-26", map[int]int{20: 3, 21: 3, 22: 3}, map[int]int{20: 1, 21: 1, 22: 1}),
		},
		WithPattern: 4,
		FreeDays: map[string][]bool{
			"u-kael":  {false, false, true, false, true, true, true},
			"u-bryn":  {true, false, true, false, false, true, true},
			"u-tam":   {false, true, true, false, false, true, false},
			"u-nissa": {false, false, false, true, true, true, true},
		},
		Lanes: map[string][]BenchLaneSegment{
			"u-kael": {
				seg(2, 16*60+30, 23*60, AvailAvailable),
				seg(2, 19*60, 22*60, AvailPreferred),
				seg(4, 20*60, 24*60, AvailAvailable),
				seg(5, 17*60, 24*60, AvailAvailable),
				seg(5, 19*60, 23*60, AvailPreferred),
				seg(6, 19*60, 23*60+30, AvailAvailable),
			},
			"u-bryn": {
				seg(0, 19*60, 22*60, AvailAvailable),
				seg(2, 18*60, 21*60+30, AvailAvailable),
				seg(5, 18*60, 22*60+30, AvailAvailable),
				seg(5, 19*60, 22*60+30, AvailPreferred),
				seg(6, 20*60, 24*60, AvailAvailable),
			},
			"u-tam": {
				seg(1, 16*60, 19*60, AvailAvailable),
				seg(2, 19*60, 23*60, AvailAvailable),
				seg(5, 19*60, 24*60, AvailAvailable),
			},
			"u-nissa": {
				seg(3, 17*60, 20*60, AvailAvailable),
				seg(4, 21*60, 24*60, AvailAvailable),
				seg(5, 19*60, 21*60, AvailAvailable),
				seg(6, 20*60, 23*60, AvailAvailable),
				seg(6, 20*60, 23*60, AvailPreferred),
			},
			// Rell: NOTHING. Ledger #3's case.
			"u-rell": {},
		},
	}
	scheduleOracleDeriveCounts(av)
	return av
}

// scheduleOracleDeriveCounts recomputes every day's per-hour Free/Prefer arrays
// from the fixture's own lanes, at the TOP OF THE HOUR — the same coarse read
// the count lane advertises and the same one production's overlay performs.
func scheduleOracleDeriveCounts(av *BenchAvailability) {
	for d := range av.Days {
		free := make([]int, 24)
		prefer := make([]int, 24)
		for _, lanes := range av.Lanes {
			seenFree := make([]bool, 24)
			seenPref := make([]bool, 24)
			for _, g := range lanes {
				if g.DayIndex != d {
					continue
				}
				for h := 0; h < 24; h++ {
					if g.StartMinute > h*60 || h*60 >= g.EndMinute {
						continue
					}
					// One member counts ONCE per hour however many of their own
					// windows cover it, and a preferred hour is also a free one
					// — preferred sits inside available by construction.
					if !seenFree[h] {
						seenFree[h] = true
						free[h]++
					}
					if g.State == AvailPreferred && !seenPref[h] {
						seenPref[h] = true
						prefer[h]++
					}
				}
			}
		}
		av.Days[d].Free, av.Days[d].Prefer = free, prefer
	}
}

func scheduleOracleInput(isGM bool) scheduleBuildInput {
	av := scheduleOracleAvail()
	if !isGM {
		// A PLAYER'S PAYLOAD CARRIES NO LANES. The adapter builds FreeDays and
		// Lanes inside one `includeDetail` branch, so a player receives neither
		// — the fixture reproduces that rather than pretending otherwise.
		av.FreeDays = nil
		av.Lanes = nil
	}
	return scheduleBuildInput{
		IsGM:           isGM,
		ViewerID:       "u-tam",
		CampaignID:     "camp-1",
		CSRFToken:      "tok",
		Roster:         scheduleOracleRoster(),
		Avail:          av,
		Answers:        scheduleOracleAnswers(),
		Zone:           "America/Chicago",
		ZoneLeaf:       "Chicago",
		WeekStart:      scheduleOracleWeek(),
		BandFrom:       16,
		BandTo:         24,
		Scope:          "week",
		MailConfigured: true,
		AskState:       ScheduleAskState{Ready: true},
	}
}

// ── THE DEPARTED MEMBER CHANGES NOTHING ────────────────────────────────────

// TestScheduleOracle_DepartedAnswerChangesNoNumberAndAppearsInNoList — WG-9's
// load-bearing case. `u-departed` holds a stored `yes`, is not on the roster, and
// must therefore be invisible to every count and every list on the page.
func TestScheduleOracle_DepartedAnswerChangesNoNumberAndAppearsInNoList(t *testing.T) {
	in := scheduleOracleInput(true)
	roster := scheduleBuildRoster(in)
	answer := scheduleBuildAnswer(in)

	if got, want := len(roster.Rows), 5; got != want {
		t.Errorf("roster rows = %d, want %d — the departed row leaked into the table", got, want)
	}
	if !strings.Contains(roster.Sub, "5 in the campaign · 3 answered") {
		t.Errorf("roster subtitle = %q, want the recomputed 5/3 — a stored tally would say 4", roster.Sub)
	}
	if !strings.Contains(answer.Sub, "3 of 5") {
		t.Errorf("answer subtitle = %q, want 3 of 5", answer.Sub)
	}
	if got, want := len(answer.Rows)+len(answer.Awaiting), 5; got != want {
		t.Errorf("answer rows+awaiting = %d, want %d", got, want)
	}
	for _, r := range append(append([]ScheduleRosterRow{}, roster.Rows...), answer.Rows...) {
		if strings.Contains(strings.ToLower(r.Name), "departed") {
			t.Errorf("a departed member appears in a printed list: %q", r.Name)
		}
	}
}

// ── RANK 1 IS THE BENCH'S DERIVED WINDOW ───────────────────────────────────

// TestScheduleOracle_RankOneIsTheBenchDerivedWindow — the two surfaces are one
// click apart and may not disagree about when to play. Both are driven from the
// SAME BenchAvailability through their own public builders, so this fails if
// either side is re-implemented rather than sharing the arithmetic.
func TestScheduleOracle_RankOneIsTheBenchDerivedWindow(t *testing.T) {
	in := scheduleOracleInput(true)
	v := scheduleBuildVerdict(in)

	var rank1 ScheduleCandidate
	for _, c := range v.Candidates {
		if c.Rank == 1 {
			rank1 = c
		}
	}
	if rank1.Rank != 1 {
		t.Fatalf("the Verdict printed no rank-1 card: %+v", v.Candidates)
	}

	bench := benchRsvpBuild(benchRsvpInput{
		IsGM: true, ViewerID: in.ViewerID, CampaignID: in.CampaignID,
		Roster: in.Roster, Avail: in.Avail, Answers: in.Answers,
		ViewerZone: in.Zone, ViewerZoneSource: "member",
		MailConfigured: true, AskState: ScheduleAskState{Ready: true},
	})
	if !bench.RecDerived {
		t.Fatalf("the Bench refused to derive a window from the same overlay: %q", bench.Rec)
	}
	// The Bench prints "Most free: Sat 19:00–22:00 …"; the card prints
	// "Sat 25 Jul · 19:00–22:00". The shared claim is the DAY and the START.
	if !strings.Contains(bench.Rec, "Sat") || !strings.Contains(rank1.When, "Sat") {
		t.Errorf("day disagreement: bench %q vs card %q", bench.Rec, rank1.When)
	}
	if !strings.Contains(bench.Rec, "19:00") || !strings.Contains(rank1.When, "19:00") {
		t.Errorf("start-hour disagreement: bench %q vs card %q", bench.Rec, rank1.When)
	}
	if rank1.DayKey != "2026-07-25" {
		t.Errorf("rank-1 day key = %q, want 2026-07-25", rank1.DayKey)
	}
}

// ── THE CARDS SIT IN DATE ORDER AND NEVER MOVE ─────────────────────────────

// TestScheduleOracle_CardsAreInDateOrderAndRankIsPrinted — the anti-motion
// mechanic. Rank is an ORDINAL printed on the left, not a position: a person
// answering re-inks the numeral and rewrites the reason sentence IN PLACE, and
// may not move a single element on this page.
func TestScheduleOracle_CardsAreInDateOrderAndRankIsPrinted(t *testing.T) {
	v := scheduleBuildVerdict(scheduleOracleInput(true))
	if len(v.Candidates) < 2 {
		t.Fatalf("want at least two candidate cards, got %d", len(v.Candidates))
	}
	prev := ""
	ranks := map[int]bool{}
	for _, c := range v.Candidates {
		if prev != "" && c.DayKey < prev {
			t.Errorf("cards are not in date order: %q came after %q", c.DayKey, prev)
		}
		prev = c.DayKey
		if c.Rank < 1 {
			t.Errorf("card %q carries no printed rank", c.When)
		}
		if ranks[c.Rank] {
			t.Errorf("rank %d is printed twice", c.Rank)
		}
		ranks[c.Rank] = true
	}
}

// ── PERMISSION IS ABSENCE, AND IT BINDS THE PROSE ──────────────────────────

// TestScheduleOracle_PlayerPayloadContainsNoOtherMember — asserted about the
// BUILT DATA, not about the rendered HTML. A permission expressed as a template
// branch is one refactor away from being lost.
func TestScheduleOracle_PlayerPayloadContainsNoOtherMember(t *testing.T) {
	in := scheduleOracleInput(false)
	v := scheduleBuildVerdict(in)
	m := scheduleBuildMatrix(in)
	r := scheduleBuildRoster(in)
	a := scheduleBuildAnswer(in)

	others := []string{"Kael", "Bryn", "Nissa", "Rell", "Ashvane", "Aldercroft", "Fen", "Vantry"}

	if len(m.Lanes) != 0 {
		t.Errorf("a player's matrix carries %d member lanes — including their own, there must be 0", len(m.Lanes))
	}
	if len(r.Rows) != 1 {
		t.Errorf("a player's roster carries %d rows, want exactly their own", len(r.Rows))
	}
	if a.Director {
		t.Error("a player received the Director's answer surface")
	}
	if len(a.Awaiting) != 0 {
		t.Errorf("a player received an `awaiting reply` group of %d — it is not derivable for them", len(a.Awaiting))
	}

	// The reason sentence is the sneakiest leak on the page, because it reads as
	// innocuous copy.
	for _, c := range v.Candidates {
		for _, name := range others {
			if strings.Contains(c.Why, name) {
				t.Errorf("a player's reason sentence names %q: %q", name, c.Why)
			}
		}
		if len(c.Outs)+len(c.Unknowns) != 0 {
			t.Errorf("a player's card carries an out column of %d stamps", len(c.Outs)+len(c.Unknowns))
		}
	}
	// No `needs backend` chip reaches a player, anywhere on the page.
	for _, chips := range [][]scheduleChip{v.Chips, m.Chips, r.Chips, a.Chips} {
		for _, c := range chips {
			if !c.Warn {
				t.Errorf("a player received a `needs backend` chip: %q", c.Text)
			}
		}
	}
	if v.Propose != nil || v.Ask != nil {
		t.Error("a player received a Director-tier control")
	}
}

// TestScheduleOracle_DirectorAndPlayerCountTheSameWay — one arithmetic. IsGM
// gates what is ABSENT from the payload, never HOW anything is counted.
func TestScheduleOracle_DirectorAndPlayerCountTheSameWay(t *testing.T) {
	gm := scheduleBuildVerdict(scheduleOracleInput(true))
	pl := scheduleBuildVerdict(scheduleOracleInput(false))
	if gm.Headline != pl.Headline {
		t.Errorf("headline differs by role: GM %q vs player %q", gm.Headline, pl.Headline)
	}
	if gm.Rec != pl.Rec {
		t.Errorf("count line differs by role: GM %q vs player %q", gm.Rec, pl.Rec)
	}
	if len(gm.Candidates) != len(pl.Candidates) {
		t.Errorf("candidate count differs by role: GM %d vs player %d", len(gm.Candidates), len(pl.Candidates))
	}
	for i := range gm.Candidates {
		if gm.Candidates[i].TallyFree != pl.Candidates[i].TallyFree {
			t.Errorf("card %d tally differs by role: GM %q vs player %q",
				i, gm.Candidates[i].TallyFree, pl.Candidates[i].TallyFree)
		}
	}
}

// ── LEDGER #3: NEVER-ANSWERED IS NOT A REFUSAL ─────────────────────────────

// TestScheduleOracle_NeverAnsweredIsNotCountedOut — Rell saved nothing. The tool
// cannot tell that apart from "busy all week", so the card must file them under
// `no answer` with a HOLLOW swatch, never under `out`, and the lane must print
// the neutral sentence rather than a warning.
func TestScheduleOracle_NeverAnsweredIsNotCountedOut(t *testing.T) {
	in := scheduleOracleInput(true)
	v := scheduleBuildVerdict(in)
	m := scheduleBuildMatrix(in)

	found := false
	for _, c := range v.Candidates {
		for _, s := range c.Unknowns {
			if !s.Hollow {
				t.Errorf("a never-answered stamp is not hollow: %+v", s)
			}
			found = true
		}
		for _, s := range c.Outs {
			if s.Hollow {
				t.Errorf("a known-busy stamp is drawn hollow: %+v", s)
			}
		}
		if strings.Contains(c.Why, "never answered") {
			found = true
		}
	}
	if !found {
		t.Error("no card distinguished `never answered` from `out` — that is ledger #3, " +
			"the single most important gap this page exists to keep honest")
	}

	var rell ScheduleLane
	for _, l := range m.Lanes {
		if l.Name == "Rell" {
			rell = l
		}
	}
	if rell.Name == "" {
		t.Fatal("the matrix printed no lane for the member who saved nothing")
	}
	if len(rell.Cells) != 0 {
		t.Error("a member with nothing saved was given cells — the lane must carry the sentence instead")
	}
	if !strings.Contains(rell.Note, "busy all week") {
		t.Errorf("the empty lane does not say what it does not know: %q", rell.Note)
	}
	if rell.NoteWarn != "" {
		t.Errorf("an unknown was drawn in the warning register: %q", rell.NoteWarn)
	}
}

// ── THE COARSE READ DISAGREES WITH THE FINE ONE, IN PLAIN SIGHT ────────────

// TestScheduleOracle_CountLaneWearsItsSamplingRule — the count lane samples the
// top of the hour and the lanes above it are minute-accurate. Kael is free from
// 16:30 on Wednesday: their own mark starts at 16:30 and the 16:00 count does
// not move. The page must WEAR that, permanently.
func TestScheduleOracle_CountLaneWearsItsSamplingRule(t *testing.T) {
	m := scheduleBuildMatrix(scheduleOracleInput(true))
	if len(m.CountChip) == 0 {
		t.Fatal("the count lane carries no sampling chip")
	}
	if !strings.Contains(m.CountChip[0].Text, "hour") {
		t.Errorf("the count lane's chip does not name its sampling rule: %q", m.CountChip[0].Text)
	}
	// The caption is ONE paragraph and this assertion reads its words, not its
	// emphasis — where the eye lands is the stills' business, not the oracle's.
	joined := m.Caption.Text()
	if !strings.Contains(joined, "18:30") {
		t.Errorf("the caption does not state the fine/coarse disagreement: %q", joined)
	}
}

// TestScheduleOracle_QuorumRefusesRatherThanGuessing — a confident ranking over
// an empty week is the one way this surface can lie. Below three members with
// saved availability it prints the refusal and NO cards.
func TestScheduleOracle_QuorumRefusesRatherThanGuessing(t *testing.T) {
	in := scheduleOracleInput(true)
	in.Avail.WithPattern = 2
	v := scheduleBuildVerdict(in)
	if len(v.Candidates) != 0 {
		t.Errorf("below quorum the Verdict still printed %d cards", len(v.Candidates))
	}
	if v.Fault == nil {
		t.Fatal("below quorum the Verdict printed no refusal row")
	}
	if !strings.Contains(v.Fault.Detail, "guess wearing a number") {
		t.Errorf("the quorum refusal does not say why: %q", v.Fault.Detail)
	}
}

// TestScheduleOracle_PlayerReasonSentenceClaimsNothingItCannotKnow — the
// sneakiest failure this page could ship.
//
// A player's payload carries no member, so "N never answered", "X out" and
// "01:00 for someone" are not merely unavailable to them — deriving them anyway
// from an absent lane map produces a CONFIDENT LIE: every member reads as
// never-having-answered, and the card tells a player that five of five people
// ignored the question. The numbers that survive are aggregates and are true.
func TestScheduleOracle_PlayerReasonSentenceClaimsNothingItCannotKnow(t *testing.T) {
	v := scheduleBuildVerdict(scheduleOracleInput(false))
	if len(v.Candidates) == 0 {
		t.Fatal("the player's Verdict printed no cards")
	}
	for _, c := range v.Candidates {
		for _, forbidden := range []string{"never answered", "out", "local start"} {
			if strings.Contains(c.Why, forbidden) {
				t.Errorf("a player's reason sentence claims %q from a payload with no lanes: %q",
					forbidden, c.Why)
			}
		}
		if !strings.Contains(c.Why, "free") {
			t.Errorf("a player's reason sentence lost the aggregate it CAN state: %q", c.Why)
		}
	}
	// The Director, over the same fixture, keeps all of it.
	gm := scheduleBuildVerdict(scheduleOracleInput(true))
	joined := ""
	for _, c := range gm.Candidates {
		joined += c.Why + " | "
	}
	if !strings.Contains(joined, "never answered") {
		t.Errorf("the Director lost the clause the lane data supports: %q", joined)
	}
}
