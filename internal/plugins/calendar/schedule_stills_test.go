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
	"fmt"
	"regexp"
	"strconv"
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
	return scheduleRenderBodyZoom(t, isGM, "week")
}

// scheduleRenderBodyZoom is the same render at a named zoom.
func scheduleRenderBodyZoom(t *testing.T, isGM bool, zoom string) string {
	t.Helper()
	var sb strings.Builder
	if err := scheduleBody(scheduleShotData(isGM, zoom)).Render(context.Background(), &sb); err != nil {
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

// 7 · THE COUNT LANE'S NUMERAL AND ITS PEAK HOUR ARE STACKED, NOT SIDE BY SIDE.
// The sealed mockup's cell is `display:grid;place-items:center` and nothing
// else, so grid's own row auto-flow puts `1` above `@ 19`, both centred — which
// is what the still shows in all seven columns. The build added
// `grid-auto-flow: column; gap: 4px`, two declarations the mockup does not have,
// and every cell rendered `1   @ 19` on one line.
//
// It is a two-property diff and it changes what the lane IS: stacked, the
// numeral is the datum and the hour is its footnote; inline, they read as a pair
// of equal facts and the count stops being the thing you can see from across the
// room.
func TestScheduleStills_TheCountLaneStacksItsNumeralOverItsHour(t *testing.T) {
	css := scheduleStrip(scheduleCSS(t, "calendar-schedule.css"))
	rule := scheduleRuleFor(t, css, ".cal-schedule .sc-row.cnt .sc-cell")
	if !strings.Contains(rule, "place-items: center") {
		t.Errorf("the count cell lost the mockup's centring; got:\n%s", rule)
	}
	for _, forbidden := range []string{"grid-auto-flow", "gap:"} {
		if strings.Contains(rule, forbidden) {
			t.Errorf("the count cell declares %q, which the sealed mockup does not — "+
				"it is what turns the drawn stack into a row; got:\n%s", forbidden, rule)
		}
	}
}

// 8 · THE MATRIX HEAD NAMES THE DAYS AND THE HOURS IT IS SHOWING. The drawing
// prints `Mon 20 – Sun 26 Jul · 16:00–24:00 · times in …`; the build reused the
// page's generic frame line and printed `week of 20 Jul 2026 · times in …`.
//
// Both name the week. Only one names the BAND — and this is the one surface
// whose entire subject is which hours are on screen. A member whose windows all
// fall outside the band gets a lane that says so; a reader who cannot see what
// the band IS has no way to act on that.
func TestScheduleStills_TheMatrixHeadNamesItsDaysAndItsHours(t *testing.T) {
	m := scheduleBuildMatrix(scheduleOracleInput(true))
	for _, want := range []string{"Mon 20 – Sun 26 Jul", "16:00–24:00", "times in Chicago"} {
		if !strings.Contains(m.Frame, want) {
			t.Errorf("matrix head = %q, missing %q", m.Frame, want)
		}
	}

	// A campaign with no zone still names its days and its band — the missing
	// zone is a named absence, never a reason to drop the two facts that do not
	// depend on it.
	in := scheduleOracleInput(true)
	in.Zone, in.ZoneLeaf = "", ""
	nz := scheduleBuildMatrix(in)
	for _, want := range []string{"Mon 20 – Sun 26 Jul", "16:00–24:00", "no time zone is set"} {
		if !strings.Contains(nz.Frame, want) {
			t.Errorf("zone-less matrix head = %q, missing %q", nz.Frame, want)
		}
	}
}

// 9 · A MEMBER WITH NO ZONE IS AN UNKNOWN, NOT A FAULT — IN THE MATRIX TOO. The
// drawing's lane emits `NEED('no zone')`, the grey "not known" vocabulary, and
// the still renders it grey. The build emitted `.badge.warn` and rendered it
// amber, i.e. as something broken.
//
// The ROSTER's amber `zone not set` is correct and stays: that row carries the
// repair beside it (`Ask →`) and is where the campaign is asked to do something
// about it. The matrix lane carries no repair and is not asking — it is
// reporting that this member's clock cannot be computed. Same fact, two
// registers, and the register is chosen by whether there is an action attached.
func TestScheduleStills_TheMatrixMissingZoneIsGreyNotAmber(t *testing.T) {
	html := scheduleRenderBody(t, true)
	matrix := scheduleSectionHTML(t, html, "sc-matrix")
	if !strings.Contains(matrix, `<span class="badge need">no zone</span>`) {
		t.Error("the matrix lane's missing-zone chip is not the grey NEED chip the drawing emits")
	}
	if strings.Contains(matrix, `<span class="badge warn">no zone</span>`) {
		t.Error("the matrix lane still prints a missing zone as an amber fault")
	}
	// …and the roster's amber pair is untouched, repair and all.
	roster := scheduleSectionHTML(t, html, "sc-roster")
	if !strings.Contains(roster, `<span class="badge warn">zone not set</span>`) {
		t.Error("the roster's amber `zone not set` must stay — it is the one with the repair")
	}
}

// 10 · THE MATRIX FITS THE PHONE THE PRODUCT ACTUALLY GIVES IT — AND THE SHOT
// HAS TO BE TAKEN IN THAT PHONE.
//
// The built mobile shot dragged sideways inside the matrix panel and clipped the
// SUN column, where `wg-schedule-gm-mobile-light.png` fits all seven days. The
// page was not the liar: the HARNESS was. `.shotwrap` padded 20px at every
// width, and the product's own `<main class="px-3 py-3 md:px-5 md:py-4">` pads
// 12px below 768px — so the stand-in shell handed the surface 8px less than the
// page it stands in for, and 8px is exactly the margin the drawn geometry has.
//
// TWO CLAIMS, BOTH ARITHMETIC, BOTH HERE. First: the drawn narrow geometry
// (76 + 94 + 7 × 24) fits the width the shipping page gives it at 390. Second:
// the harness's shell gives the SAME width, so a shot can never again report a
// drag the product does not have — or, worse, hide one it does.
//
// A `documentElement.scrollWidth == clientWidth` check cannot see any of this: a
// nested `overflow-x:auto` container never contributes to the document's own
// scrollable overflow, so the page was honestly not dragging while the panel
// inside it was. The measurement that catches it is per-element, and it is now
// in the shooter's report; this is the same fact stated where it can be re-run
// without a browser.
func TestScheduleStills_TheMatrixFitsThePhoneTheProductActuallyGives(t *testing.T) {
	css := scheduleStrip(scheduleCSS(t, "calendar-schedule.css"))
	narrow := scheduleMediaBlock(t, css, "max-width: 640px")

	idw := schedulePxDecl(t, scheduleRuleFor(t, narrow, ".cal-schedule .sc-grid"), "--idw")
	sayw := schedulePxDecl(t, scheduleRuleFor(t, narrow, ".cal-schedule .sc-grid"), "--sayw")
	colmin := schedulePxDecl(t, scheduleRuleFor(t, narrow, ".cal-schedule .sc-grid"), "--colmin")
	bodyPad := schedulePxDecl(t, scheduleRuleFor(t, narrow, ".cal-schedule .sc-body"), "padding")

	// The grid's own declared floor, for the Gregorian seven the still shows.
	grid := idw + sayw + 7*colmin

	// What the page gives it: the viewport, less the app shell's own padding,
	// less the panel's hairline, less the body's padding, less the scroll
	// container's hairline. Every term is a real box on the way down.
	//
	// ── AMENDED, AND IT IS A SECOND ARM RATHER THAN A MOVED THRESHOLD:
	//    C-CALV4-MOBILE [MOB-6] SIGNED.
	//
	// This assertion ran at 390 and only at 390, which is exactly how "tuned
	// for 390" came to mean "broken at 360" without anybody being told.
	// MEASURED: `.sc-wrap` computed overflow-x `hidden` with 346 of 346 at 390
	// but `auto` with 331 of 338 at 375 and 316 of 338 at 360 — 7px and 22px of
	// sideways drag on the page a player uses to say when they are free.
	//
	// `schedulePhoneViewport` is BYTE-UNCHANGED and its row still runs. The
	// same computation now also runs at `scheduleNarrowViewport`, so the
	// surface has a floor rather than a favourite phone.
	for _, vw := range []int{schedulePhoneViewport, scheduleNarrowViewport} {
		budget := vw - 2*schedulePagePadNarrow -
			2*scheduleHairline - 2*bodyPad - 2*scheduleHairline
		if grid > budget {
			t.Errorf("the narrow matrix needs %dpx (%d ident + %d say + 7 × %d) and the page "+
				"gives it %dpx at %dpx — the panel drags sideways and the last day clips",
				grid, idw, sayw, colmin, budget, vw)
		}
		t.Logf("the narrow matrix needs %dpx (%d ident + %d say + 7 × %d) against a %dpx "+
			"budget at %dpx", grid, idw, sayw, colmin, budget, vw)
	}

	// AND `--colmin` DOES NOT PAY FOR IT. The 22px comes out of the fixed
	// terms, never out of the day columns: this surface's own signed floor is
	// "THE CELL IS THE TARGET (>=24x24 at every width)", and shaving it is
	// named as a STOP-AND-FLAG rather than an option.
	if colmin < 24 {
		t.Errorf("--colmin is %dpx at ≤640 — the surface's own signed cell floor is 24x24 at "+
			"every width, and the narrow budget may not be bought out of the day columns",
			colmin)
	}

	// The fidelity harness must stand in the same phone. A shot taken in a
	// narrower shell than the product's is not a picture of the product.
	if !strings.Contains(scheduleShotPage, fmt.Sprintf("padding: %dpx", schedulePagePadNarrow)) {
		t.Errorf("the harness page does not pad %dpx at phone width — it cannot stand in "+
			"for <main class=\"px-3 …\"> and its mobile shot is not the product's",
			schedulePagePadNarrow)
	}
	if !strings.Contains(scheduleShotPage, fmt.Sprintf("min-width: %dpx", schedulePagePadBreak)) {
		t.Errorf("the harness page does not switch shells at the product's own %dpx "+
			"breakpoint", schedulePagePadBreak)
	}
}

// scheduleMediaBlock returns the body of the first @media rule whose query
// contains q, with its own nesting intact.
func scheduleMediaBlock(t *testing.T, css, q string) string {
	t.Helper()
	i := strings.Index(css, q)
	if i < 0 {
		t.Fatalf("no @media rule mentioning %q", q)
	}
	open := strings.Index(css[i:], "{")
	if open < 0 {
		t.Fatalf("@media %q has no body", q)
	}
	open += i
	depth := 0
	for j := open; j < len(css); j++ {
		switch css[j] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return css[open+1 : j]
			}
		}
	}
	t.Fatalf("@media %q is unterminated", q)
	return ""
}

// schedulePxDecl reads one `prop: <n>px` out of a declaration body.
func schedulePxDecl(t *testing.T, rule, prop string) int {
	t.Helper()
	m := regexp.MustCompile(regexp.QuoteMeta(prop) + `:\s*(\d+)px`).FindStringSubmatch(rule)
	if m == nil {
		t.Fatalf("no %q declaration in:\n%s", prop, rule)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatalf("%q is not a pixel count: %v", prop, err)
	}
	return n
}

// 11 · TWO STRINGS THAT VANISH ON THE PHONE, ONE OF THEM AGAINST THIS SHEET'S
// OWN WRITTEN RULE.
//
// Shot at a true 390 the matrix loses two things the still keeps. Rell's empty
// lane is a `nowrap` + ellipsis box, so its sentence clips mid-word AND TAKES
// THE `NO PATTERN` CHIP WITH IT — an honesty chip that disappears at the
// smallest width, which is the one thing this page forbids anywhere it forbids
// anything. And the count lane's denominator clips into `FREE OF 5 …`: the exact
// string the sheet's own comment says may never be clipped, because "free of 5"
// with the rest cut off is precisely the "of 5 players" misreading the sentence
// exists to prevent.
//
// THE MOCKUP SOLVES BOTH BY SHORTENING THE WORDS, not by shrinking them: at
// NARROW it prints `nothing saved`, `free of 5` and `who`. Its producer re-runs
// on resize; a server-rendered page cannot, so it emits BOTH strings and lets
// one media query choose. Duplicated text is the price of a width-dependent
// sentence on a page that renders once, and it is cheaper than either a clipped
// honesty chip or a sentence that means something different truncated.
func TestScheduleStills_TheNarrowMatrixShortensItsWordsInsteadOfClippingThem(t *testing.T) {
	html := scheduleRenderBody(t, true)
	matrix := scheduleSectionHTML(t, html, "sc-matrix")
	for _, pair := range []struct{ long, short string }{
		{"free of 5 in the campaign", "free of 5"},
		{"no availability saved", "nothing saved"},
		{"who is free", "who"},
	} {
		if !strings.Contains(matrix, pair.long) {
			t.Errorf("the wide matrix lost %q", pair.long)
		}
		if !strings.Contains(matrix, ">"+pair.short+"<") {
			t.Errorf("the matrix carries no narrow form of %q — at 390 the wide one clips",
				pair.long)
		}
	}
	if !strings.Contains(matrix, `class="sc-narrow"`) || !strings.Contains(matrix, `class="sc-wide"`) {
		t.Error("the width-dependent strings are not marked for the media query to choose between")
	}

	// The swap itself: hidden by default, shown in the phone block, and the
	// wide one hidden there. A pair where both are visible prints the sentence
	// twice, which is worse than either failure it repairs.
	css := scheduleStrip(scheduleCSS(t, "calendar-schedule.css"))
	if !strings.Contains(css, ".cal-schedule .sc-narrow") {
		t.Fatal("no .sc-narrow rule at all")
	}
	narrow := scheduleMediaBlock(t, css, "max-width: 640px")
	for _, want := range []string{".cal-schedule .sc-wide", ".cal-schedule .sc-narrow"} {
		if !strings.Contains(narrow, want) {
			t.Errorf("the phone block does not swap %s", want)
		}
	}
}

// 12 · THE EMPHASIS SWEEP — the three the caption pass did not reach, and one
// control that is 4px wider than the drawing.
//
// Stage 8 fixed the CAPTIONS. A full inventory of every `<b>` and `<i>` the
// sealed mockup emits inside its surface producers (as opposed to inside its own
// explanatory notes) then found three more, all outside a `.caption`: the
// Painter's scope note bolds WHICH TABLE the marks land in
// (`marks a <b>date exception</b>` / `sets your <b>normal hours</b>`), the
// Painter's foot bolds the compose rule's claim
// (`<b>Offering a window only ever adds.</b>`), and the suggest dock bolds the
// other way to say the same thing (`tick the windows in <b>My availability</b>
// below`). Each is a lead-in doing the same job as the caption lead-ins: it is
// the phrase a reader scans the sentence FOR.
//
// The `.seg` rung is a plain arithmetic miss — the mockup pads its buttons
// `0 8px` and the sheet shipped `0 10px`, so the Painter's scope control is 8px
// wider than the drawing at every width.
func TestScheduleStills_TheRemainingEmphasisAndTheSegRung(t *testing.T) {
	html := scheduleRenderBody(t, false)
	for _, want := range []string{
		"<b>date exception</b>",
		"<b>Offering a window only ever adds.</b>",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("the rendered page is missing the drawn emphasis %q", want)
		}
	}

	// The suggest dock is only drawn once a player opens it, so it is asserted
	// on the builder rather than on the default page.
	sug := scheduleOracleInput(false)
	sug.ViewerID, sug.SugOpen = "u-tam", true
	sug.EventID, sug.CalendarID = "ev-41", "cal-1"
	form := scheduleBuildAnswer(sug).Form
	if form == nil {
		t.Fatal("no answer form for a player")
	}
	var dock bool
	for _, r := range form.SuggestNote {
		if r.Em == "b" && strings.Contains(r.Text, "My availability") {
			dock = true
		}
	}
	if !dock {
		t.Errorf("the suggest dock does not name `My availability` as the drawing does: %q",
			form.SuggestNote.Text())
	}
	// The other scope wording carries its own bold, and it names the other
	// table — which is the entire distinction the segment exists to make.
	rec := scheduleOracleInput(false)
	rec.ViewerID, rec.Scope = "u-tam", "recurring"
	rec.OwnLanes = scheduleOracleAvail().Lanes["u-tam"]
	paint := scheduleBuildPainter(rec).Form
	if paint == nil {
		t.Fatal("no paint form on the recurring scope")
	}
	if got := paint.ScopeNote.Text(); !strings.Contains(got, "normal hours") {
		t.Errorf("the recurring scope note = %q", got)
	}
	var bold bool
	for _, r := range paint.ScopeNote {
		if r.Em == "b" && strings.Contains(r.Text, "normal hours") {
			bold = true
		}
	}
	if !bold {
		t.Error("`normal hours` is the phrase the sentence is scanned for, and it is not the lead-in")
	}

	css := scheduleStrip(scheduleCSS(t, "calendar-schedule.css"))
	rule := scheduleRuleFor(t, css, ".cal-schedule .seg button")
	if !strings.Contains(rule, "padding: 0 8px") {
		t.Errorf("the `.seg` rung must pad the sealed mockup's `0 8px`; got:\n%s", rule)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// FIDELITY PASS 4 — what a declaration-by-declaration diff of the sealed
// drawing against the shipped sheets caught, after every by-eye comparison had
// already passed. These are not "the strings were right and the shape was
// wrong"; these are declarations that were SUBSTITUTED for near-neighbours in
// transcription, where the near-neighbour is not neutral.
// ─────────────────────────────────────────────────────────────────────────────

// 9 · THE SELECTED SEGMENT IS A RING, NOT A HUE MIX. The drawing tints the
// pressed rung by mixing the accent into `transparent`:
//
//	background: color-mix(in oklch, transparent 96%, var(--accent))
//
// The sheet shipped `color-mix(in oklch, var(--surface-card) 88%, var(--accent))`
// instead. Those look interchangeable and are not. `--surface-card` is
// ACHROMATIC, so it carries no hue of its own, and oklch interpolation resolves
// a missing hue by taking the SHORT ARC — 0deg toward the accent's 270deg is
// 349.2deg the other way round the wheel, which is PINK. The pressed
// `Evening 16-24` pill measures rgb(252,232,241) on the built page against the
// still's rgb(248,249,254). Mixing into `transparent` — which is what the
// drawing does, deliberately — has no hue to interpolate and cannot drift.
//
// The drawing DOES write one surface-toward-accent mix, on `.surf.sel`, and
// annotates it "D-T1: this tint drifts rose in light. Booked upstream, not
// fixed here." That one is drawn as-is and stays. It is also the reason this
// test counts rather than forbids: exactly one such mix is sanctioned, and it
// is not this one.
//
// The same rule block lost three more drawn declarations while it was being
// written: the rung's `gap: 5px`, the whole `:hover` feedback state, and
// `cursor: not-allowed` on the disabled `Day` rung (the sheet said `default`,
// which reads as "this is not a control" rather than "this control is off").
func TestScheduleStills_TheSelectedSegmentIsTheDrawnRingNotAHueMix(t *testing.T) {
	css := scheduleStrip(scheduleCSS(t, "calendar-schedule.css"))

	sel := scheduleRuleFor(t, css, `.cal-schedule .seg button[aria-pressed="true"]`)
	if !strings.Contains(sel, "color-mix(in oklch, transparent 96%, var(--accent))") {
		t.Errorf("the pressed rung must tint from `transparent`, which has no hue to "+
			"interpolate; got:\n%s", sel)
	}
	if strings.Contains(sel, "--surface-card") {
		t.Errorf("the pressed rung mixes an achromatic surface toward the accent — that is "+
			"the short-arc hue drift that renders pink; got:\n%s", sel)
	}
	if !strings.Contains(sel, "inset 0 0 0 1.5px var(--accent)") {
		t.Errorf("the drawn ring is 1.5px, not 1px; got:\n%s", sel)
	}

	// Exactly one surface-toward-accent mix is sanctioned product-wide on this
	// sheet, and it is the drawing's own annotated `.surf.sel`.
	if n := strings.Count(css, "var(--surface-card) 96%, var(--accent)"); n != 1 {
		t.Errorf("the sanctioned `.surf.sel` tint appears %d times; the drawing writes it once", n)
	}
	if n := strings.Count(css, "var(--surface-card) 88%, var(--accent)"); n != 0 {
		t.Errorf("%d unsanctioned surface-toward-accent mixes survive", n)
	}

	rung := scheduleRuleFor(t, css, ".cal-schedule .seg button")
	if !strings.Contains(rung, "gap: 5px") {
		t.Errorf("the drawn rung sets `gap: 5px` for the rungs that carry a glyph; got:\n%s", rung)
	}

	hov := scheduleRuleFor(t, css, ".cal-schedule .seg button:hover")
	if !strings.Contains(hov, "color-mix(in oklch, transparent 90%, var(--accent))") ||
		!strings.Contains(hov, "var(--text-primary)") {
		t.Errorf("the segment gives no drawn hover feedback; got:\n%s", hov)
	}

	dis := scheduleRuleFor(t, css, ".cal-schedule .seg button:disabled")
	if !strings.Contains(dis, "cursor: not-allowed") {
		t.Errorf("a disabled rung is an OFF control, not a non-control; got:\n%s", dis)
	}
}

// 10 · THE PHONE KEEPS THE DRAWN BADGE AND THE ANSWER WELL. Two rules from the
// drawing's 640 block did not survive transcription into the sheet's, and both
// land on the surface whose 390 arithmetic this slice re-derived by hand.
//
//   - `.sc-row .say .badge{font-size:8.5px;padding:1px 3px;letter-spacing:0}` is
//     simply ABSENT. It sits in the drawing BETWEEN two rules the sheet DID
//     carry (`.sc-row .rs` and `.sc-row .who .cap`), so it was dropped, not
//     decided against. Every `.say` badge renders ~27% oversized at 390: the
//     `+1d` chip measures 34.5x19.0 against the drawing's 27.1x16.8.
//
//   - `.sc-row .rs{width:auto}` was rewritten as `min-width: 0`. The drawn
//     declaration is a NO-OP by design — no `width` is set at base — and its
//     whole effect is to LEAVE the base `min-width: 30px` alone, so the five
//     answers right-align as a column. `min-width: 0` collapses that well to
//     11.6px and `19:00` and `in` collide into `19:00 in`.
func TestScheduleStills_ThePhoneKeepsTheDrawnBadgeAndTheAnswerWell(t *testing.T) {
	css := scheduleStrip(scheduleCSS(t, "calendar-schedule.css"))
	narrow := scheduleMediaBlock(t, css, "max-width: 640px")

	badge := scheduleRuleFor(t, narrow, ".cal-schedule .sc-row .say .badge")
	for _, want := range []string{"font-size: 8.5px", "padding: 1px 3px", "letter-spacing: 0"} {
		if !strings.Contains(badge, want) {
			t.Errorf("the narrow `.say` badge is missing the drawn %q; got:\n%s", want, badge)
		}
	}

	rs := scheduleRuleFor(t, narrow, ".cal-schedule .sc-row .rs")
	if strings.Contains(rs, "min-width") {
		t.Errorf("the narrow answer well must keep the base `min-width: 30px` the drawing "+
			"leaves standing, or `19:00` and `in` collide; got:\n%s", rs)
	}
	base := scheduleRuleFor(t, css, ".cal-schedule .sc-row .rs")
	if !strings.Contains(base, "min-width: 30px") {
		t.Errorf("the base answer well is no longer 30px, so the narrow rule guards nothing; got:\n%s", base)
	}
}
