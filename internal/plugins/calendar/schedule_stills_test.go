package calendar

// schedule_stills_test.go — the four things holding the BUILT page beside the
// signed stills actually caught (C-CALV4-RSVP-P8 Part B, fidelity pass 2).
//
// THE FIDELITY GATE IS PIXELS, AND THIS IS WHAT PIXELS ARE FOR. Every defect
// below passed the render tests, the oracle and the CSS contract: each one is a
// rule that exists in `mockups/calendar-v4-schedule.html` and did not get
// carried into the shipped sheet, or a sentence the drawing prints and the
// producer did not. None of them is visible in a string assertion, and all four
// are obvious the moment the shot sits next to `wg-schedule-gm-desktop-light`.
//
// They are pinned here rather than folded into the CSS contract file because
// their JUSTIFICATION is a comparison, not a rule — and a future reader
// wondering why the zone chip refuses uppercase deserves to find the answer
// beside the other three rather than in a list of scoping assertions.

import (
	"context"
	"strings"
	"testing"
	"time"
)

// 1 · THE ZONE CHIP MAY NOT BE UPPERCASED, because it is not a word — it is the
// last segment of an IANA identifier, and `America/New_York` has no member
// spelled `NEW_YORK`. `.badge` is uppercase for a reason (it is a label
// vocabulary) and the sealed mockup carves the two zone-bearing badges out of it
// by name: `.badge.mono,.badge.tz{text-transform:none;letter-spacing:0}`.
//
// The still prints `Chicago` and `New_York`; the first shot printed `CHICAGO`
// and `NEW_YORK`, which is the surface faking an identifier it promised in its
// own caption never to fake ("zone names print the last part of the IANA
// identifier — Chronicle has no abbreviation helper, so nothing here will ever
// say CDT until it does").
func TestScheduleStills_ZoneChipKeepsTheIdentifiersOwnCase(t *testing.T) {
	css := scheduleStrip(scheduleCSS(t, "calendar-schedule.css"))
	rule := scheduleRuleFor(t, css, ".cal-schedule .badge.tz")
	if !strings.Contains(rule, "text-transform: none") {
		t.Errorf(".cal-schedule .badge.tz must undo .badge's uppercase, got:\n%s", rule)
	}
	// The `+1d` roll-over badge rides the same class and gains the same
	// exemption, which is what the still shows: `+1d`, never `+1D`.
	if !strings.Contains(css, ".cal-schedule .badge.tz") {
		t.Fatal("the tz badge rule vanished")
	}
}

// 2 · THE CARD'S ZONE IS A MUTED SIBLING, NEVER PART OF THE TIME. The sealed
// mockup styles it twice — once in the head (`.sc-head .hl .z`) and once in the
// candidate card (`.calrow .sc-when .nm .z`) — and only the first was carried.
// Unstyled, the card's `.z` inherits `.nm`'s 14px/650 primary ink, so
// "Sat 25 Jul · 19:00–22:00 Chicago" reads as ONE typographic unit and the zone
// looks like part of the numeral. That is precisely the confusion L15's rule
// exists to prevent.
func TestScheduleStills_CandidateZoneIsTheMutedSibling(t *testing.T) {
	css := scheduleStrip(scheduleCSS(t, "calendar-schedule.css"))
	rule := scheduleRuleFor(t, css, ".cal-schedule .sc-cands .calrow .sc-when .nm .z")
	for _, want := range []string{"font-weight: 500", "font-size: 10px", "var(--text-muted)"} {
		if !strings.Contains(rule, want) {
			t.Errorf("the candidate card's zone is missing %q; got:\n%s", want, rule)
		}
	}
}

// 3 · THE COUNT LANE'S NUMERALS ARE CONTROLS AND MUST CLEAR THE FLOOR. The
// signature covers a measured state, and the 24px pointer floor is part of it
// (decisions/2026-07-29-schedule-mockup-signed.md §1). The sheet already carries
// the mockup's target-floor block for the `.seg` — and stopped there, leaving
// the seven count-lane numerals at their printed 20px height, MEASURED at 20px
// to the pointer with `elementFromPoint`.
//
// The repair is the mockup's own: a transparent `::after` centred on the box, so
// the 20px row rhythm the matrix is signed at does not move by a pixel.
func TestScheduleStills_CountLaneNumeralsMeetTheTargetFloor(t *testing.T) {
	css := scheduleStrip(scheduleCSS(t, "calendar-schedule.css"))
	rule := scheduleRuleFor(t, css, ".cal-schedule .sc-row.cnt .sc-cell::after")
	if !strings.Contains(rule, "height: 24px") || !strings.Contains(rule, "margin-top: -12px") {
		t.Errorf("the count lane's hit-area pad is not the mockup's 24px centred pad; got:\n%s", rule)
	}
	// A pad on a static box lands in the page's top-left corner. The pad is
	// only a pad if its subject is positioned — which the cell already is,
	// because the same rule is what lets a mark sit inside it.
	pos := scheduleRuleFor(t, css, ".cal-schedule .sc-cell")
	if !strings.Contains(pos, "position: relative") {
		t.Error("the cell must be positioned or its hit-area pad detaches from it")
	}
}

// 4 · THE TWO HEADS NAME THE SLOT AND WHOSE CLOCK IT IS. The drawing's roster
// head reads `Session 41 · slot Sat 19:00 Chicago` and its player head reads
// `Sat 25 Jul · 19:00 Chicago · your 01:00`. The built pair dropped the weekday
// from one and everything but the event name from the other — and on a page
// whose entire subject is "the same instant is a different clock for each of
// us", a player's own clock for the slot is the one fact they came for.
//
// THE PLAYER'S HEAD NAMES THE EVENT, NOT THE SELECTED WINDOW, which is where
// this deliberately parts from the drawing: the tri-state posts to an event's
// RSVP, and the panel's own chip says `RSVP answers an event, not a week`. A
// head that named the highlighted candidate would contradict the chip directly
// underneath it.
func TestScheduleStills_TheHeadsNameTheSlotAndTheViewersOwnClock(t *testing.T) {
	in := scheduleOracleInput(true)
	in.ViewerID = "u-kael"
	in.Session = &BenchRsvpSession{
		Name: "Session 41", DaysUntil: 0, Anchored: true,
		Instant: time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC), // 19:00 Chicago
	}

	// The Director's roster head carries the weekday the still prints. The
	// instant is midnight UTC on the 26th, which IS Saturday the 25th at 19:00
	// in Chicago — the weekday is a fact about the reader's zone, not the
	// stored one, which is half of why printing it matters.
	if got := scheduleSlotLabel(in); !strings.Contains(got, "slot Sat 19:00 Chicago") {
		t.Errorf("roster slot head = %q, want it to name the weekday, the clock and the zone", got)
	}

	// The player's answer head carries the slot AND their own clock. Tam is in
	// London, so the same instant is 01:00 the next day — the exact case the
	// page exists to make legible.
	p := scheduleOracleInput(false)
	p.ViewerID = "u-tam"
	p.Session = in.Session
	got := scheduleAnswerSub(p)
	for _, want := range []string{"Session 41", "19:00 Chicago", "your 01:00", "+1d"} {
		if !strings.Contains(got, want) {
			t.Errorf("player answer head = %q, missing %q", got, want)
		}
	}

	// A session with no anchored instant still may not invent a clock — for
	// either head. This is the same refusal the zone-less member's empty clock
	// makes, and the fix above must not have quietly widened it.
	bare := p
	bare.Session = &BenchRsvpSession{Name: "Session 41"}
	if got := scheduleAnswerSub(bare); strings.Contains(got, "your") || strings.Contains(got, ":") {
		t.Errorf("an unanchored session produced a clock: %q", got)
	}
}

// scheduleRuleFor returns the declaration body of the first rule whose selector
// list contains sel. It exists so a fidelity assertion reads the DECLARATIONS of
// the rule it means rather than "is this substring somewhere in the file", which
// would pass against a rule for an entirely different selector.
func scheduleRuleFor(t *testing.T, css, sel string) string {
	t.Helper()
	for i := 0; i < len(css); {
		open := strings.Index(css[i:], "{")
		if open < 0 {
			break
		}
		open += i
		close := strings.Index(css[open:], "}")
		if close < 0 {
			break
		}
		close += open
		head := css[i:open]
		if j := strings.LastIndex(head, "}"); j >= 0 {
			head = head[j+1:]
		}
		for _, one := range strings.Split(head, ",") {
			if strings.TrimSpace(one) == sel {
				return css[open+1 : close]
			}
		}
		i = close + 1
	}
	t.Fatalf("no rule found for selector %q", sel)
	return ""
}

// ─────────────────────────────────────────────────────────────────────────────
// FIDELITY PASS 3 — what the third comparison caught. Every item below is a
// difference between the sealed drawing and the built page that no string
// assertion could see, because the strings were RIGHT and their SHAPE was not.
// ─────────────────────────────────────────────────────────────────────────────

// 5 · THE CAPTIONS CARRY THE DRAWING'S EMPHASIS. The sealed mockup writes every
// caption with a BOLD LEAD-IN and italic vocabulary words —
// `<b>How it ranks:</b>`, `<b>What the score cannot include:</b>`,
// `<b>The marks:</b>`, `<b>Fine and coarse disagree, on purpose:</b>`,
// `<b>Counts are recomputed from these rows</b>` — and the built page shipped
// all of them as flat, unemphasised prose.
//
// THIS IS NOT DECORATION. The lead-ins are how a reader FINDS "what the score
// cannot include" and "fine and coarse disagree, on purpose": two of this
// surface's own named honesty claims, drawn as the first thing the eye lands on
// in their paragraph and shipped as the middle of a grey wall. A page whose
// honesty is unfindable is a page whose honesty is decorative.
func TestScheduleStills_CaptionsCarryTheDrawnEmphasis(t *testing.T) {
	for _, role := range []struct {
		name string
		isGM bool
		want []string
	}{
		{"director", true, []string{
			"<b>How it ranks:</b>",
			"<i>preferred</i>",
			"<b>What the score cannot include:</b>",
			"<b>The marks:</b>",
			"<i>prefer</i>",
			"<b>Fine and coarse disagree, on purpose:</b>",
			"<b>This grid shows availability only</b>",
			"<b>Counts are recomputed from these rows</b>",
			"<b>an event</b>",
		}},
		// A player sees the same emphasis on the surfaces they get. The
		// Answer's caption is theirs alone and the drawing bolds both control
		// names inside it, because those two words are the ones a reader
		// scanning for "what does this button do" is looking for.
		{"player", false, []string{
			"<b>What the score cannot include:</b>",
			"<b>The marks:</b>",
			"<b>Out just this week</b>",
			"<i>no</i>",
			"<b>Suggest a better time</b>",
			"<i>maybe</i>",
		}},
	} {
		t.Run(role.name, func(t *testing.T) {
			html := scheduleRenderBody(t, role.isGM)
			for _, want := range role.want {
				if !strings.Contains(html, want) {
					t.Errorf("the rendered page is missing the drawn emphasis %q", want)
				}
			}
		})
	}
}

// 6 · THE MATRIX CAPTION IS ONE FLOWING PARAGRAPH. The mockup's `matrixCaption()`
// builds its bits in a slice and returns `bits.join(' ')` — ONE block of prose
// under the grid. The build emitted one `<p class="caption">` per bit, so the
// still's single paragraph rendered as three separated ones (four once identity
// wraps), which reads as three unrelated notes rather than one key.
func TestScheduleStills_TheMatrixCaptionIsOneParagraph(t *testing.T) {
	html := scheduleRenderBody(t, true)
	matrix := scheduleSectionHTML(t, html, "sc-matrix")
	if n := strings.Count(matrix, `class="caption"`); n != 1 {
		t.Errorf("the matrix drew %d caption blocks; the sealed mockup draws exactly 1", n)
	}
	// …and it is still every sentence, joined — not one sentence kept and the
	// rest dropped, which would trade a shape defect for a content one.
	for _, want := range []string{"The marks:", "Fine and coarse disagree", "availability only"} {
		if !strings.Contains(matrix, want) {
			t.Errorf("the joined matrix caption lost %q", want)
		}
	}
}

// scheduleRenderBody renders the real producer for one role, from the oracle's
// own fixture — the same assembly the fidelity harness shoots.
func scheduleRenderBody(t *testing.T, isGM bool) string {
	t.Helper()
	var sb strings.Builder
	if err := scheduleBody(scheduleShotData(isGM)).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	return sb.String()
}

// scheduleSectionHTML slices out one `<section class="… <class> …">` and its
// contents, so a per-panel assertion cannot accidentally read a neighbour's.
func scheduleSectionHTML(t *testing.T, html, class string) string {
	t.Helper()
	i := strings.Index(html, class)
	if i < 0 {
		t.Fatalf("no section carrying %q in the rendered page", class)
	}
	start := strings.LastIndex(html[:i], "<section")
	if start < 0 {
		t.Fatalf("%q is not inside a <section>", class)
	}
	end := strings.Index(html[start:], "</section>")
	if end < 0 {
		t.Fatalf("section carrying %q is unterminated", class)
	}
	return html[start : start+end]
}
