package calendar

// bench_rsvp_oracle_test.go — C-CALV4-RSVP-P8 Part A's GATE (§6).
//
// WHY A COUNT TEST IS THE GATE AND A FIDELITY SCREENSHOT IS NOT. The RSVP panel
// multiplies the number of counts on screen, and every one of them is ABOUT
// PEOPLE: the tally in `Session 41 · today · 3 / 5`, the density row's
// numerator and its denominator, the silent list, and the derived window's
// "N of M members". A screenshot proves a number was rendered. It cannot prove
// the number was computed from the set the viewer is entitled to.
//
// THE ASSERTION IS NOT "THE NUMBERS ARE RIGHT". That is P6's discipline
// (block_count_oracle_test.go, itself modelled on TestBlockCountsAreNotAnOracle)
// and this file extends it: every number a viewer receives must be
// INDEPENDENTLY REPRODUCIBLE FROM THAT VIEWER'S OWN VISIBLE SET, recomputed
// here. A count that happens to be right because it was computed pre-filter
// passes a value assertion and fails this one.
//
// IT LIVES IN PACKAGE calendar AND IT EXTENDS THE SEAM FIXTURE. benchFxRsvp
// and renderBench are the Bench suite's own helpers; re-authoring a parallel
// fixture or a parallel render helper is what forking the suite means, and
// block_seam_test.go's header explains at length why it is forbidden.
//
// ── THE SIGNED FIXTURE (WG-9) ──────────────────────────────────────────────
//
//	5 in the campaign · 3 answered · 1 DEPARTED member with a stored row
//
// The departed row is the load-bearing case and the reason this fixture was
// escalated rather than assumed: it is the one case that cannot be
// reconstructed from a screenshot, and a fixture that omitted it would let the
// stored tally back in silently. EventRSVPSummary.Counts is raw rows while the
// named lists drop ex-members, so a stored aggregate beside a filtered name
// list is a counts-vs-names disagreement BY CONSTRUCTION.

import (
	"strconv"
	"strings"
	"testing"
)

// ── assertion 1 ────────────────────────────────────────────────────────────
//
// The panel prints 3 / 5, and BOTH halves are recomputed here from the roster
// rather than read back off the panel.

func TestRsvpOracle_TallyIsRecomputedFromTheRoster(t *testing.T) {
	for _, gm := range []bool{true, false} {
		in := benchFxRsvpInput(gm)
		p := benchRsvpBuild(in)

		// Recompute independently: walk the ROSTER (the membership-filtered set
		// the panel prints) and count who holds a stored answer.
		want := 0
		for _, m := range in.Roster {
			if _, ok := in.Answers[m.UserID]; ok {
				want++
			}
		}
		if want != 3 || len(in.Roster) != 5 {
			t.Fatalf("the signed fixture moved: %d answered of %d in the campaign, want 3 of 5",
				want, len(in.Roster))
		}
		if !strings.Contains(p.Headline, "3 / 5") {
			t.Errorf("gm=%v headline = %q; the recomputed tally is 3 / 5", gm, p.Headline)
		}
	}
}

// ── assertion 2 ────────────────────────────────────────────────────────────
//
// The departed member's stored row changes NO number and appears in NO list.
//
// Proven by DIFFERENCE, which is stronger than an equality against a literal:
// the same fixture is built with and without the departed row and every printed
// number must be byte-identical. A panel that reached for the stored tally
// anywhere would move by exactly one here.

func TestRsvpOracle_DepartedMemberChangesNothing(t *testing.T) {
	for _, gm := range []bool{true, false} {
		with := benchRsvpBuild(benchFxRsvpInput(gm))

		clean := benchFxRsvpInput(gm)
		if _, ok := clean.Answers[benchFxDepartedID]; !ok {
			t.Fatalf("the fixture lost its departed row — WG-9's load-bearing case is gone")
		}
		delete(clean.Answers, benchFxDepartedID)
		without := benchRsvpBuild(clean)

		if with.Headline != without.Headline {
			t.Errorf("gm=%v the departed member moved the tally: %q vs %q",
				gm, with.Headline, without.Headline)
		}
		if with.Silent != without.Silent {
			t.Errorf("gm=%v the departed member moved the silent list: %q vs %q",
				gm, with.Silent, without.Silent)
		}
		if len(with.Members) != len(without.Members) || len(with.Members) != 5 {
			t.Errorf("gm=%v member table is %d rows, want the 5 in the campaign",
				gm, len(with.Members))
		}
		for i := range with.Density {
			if with.Density[i] != without.Density[i] {
				t.Errorf("gm=%v the departed member moved density column %d", gm, i)
			}
		}
		// And they are nowhere in the rendered surface, by name or by id.
		html := renderBench(t, benchFxDataRsvp(gm, gm))
		for _, trace := range []string{benchFxDepartedID, benchFxDepartedName} {
			if strings.Contains(html, trace) {
				t.Errorf("gm=%v a departed member's stored RSVP row reached the DOM (%q)", gm, trace)
			}
		}
	}
}

// ── assertion 3 ────────────────────────────────────────────────────────────
//
// "Silent" names exactly the members who are IN THE CAMPAIGN and hold NO row.
// It is DERIVED, never stored: there is no invitee table (ledger #13), so the
// only honest definition is roster-minus-answers, and it is recomputed here.

func TestRsvpOracle_SilentIsDerivedNotStored(t *testing.T) {
	in := benchFxRsvpInput(true)
	p := benchRsvpBuild(in)

	var want []string
	for _, m := range in.Roster {
		if _, ok := in.Answers[m.UserID]; !ok {
			want = append(want, m.Name)
		}
	}
	if len(want) != 2 {
		t.Fatalf("the signed fixture moved: %d silent, want 2", len(want))
	}
	for _, name := range want {
		if !strings.Contains(p.Silent, name) {
			t.Errorf("silent line %q omits %q", p.Silent, name)
		}
	}
	// Nobody who answered may appear there, and neither may the departed member.
	for _, m := range in.Roster {
		if _, ok := in.Answers[m.UserID]; ok && strings.Contains(p.Silent, m.Name) {
			t.Errorf("silent line %q names %q, who answered", p.Silent, m.Name)
		}
	}
	if strings.Contains(p.Silent, benchFxDepartedName) {
		t.Errorf("silent line %q names a member who has left the campaign", p.Silent)
	}
}

// ── assertion 4 ────────────────────────────────────────────────────────────
//
// A PLAYER'S HTML CONTAINS ZERO MEMBER-LANE DATA — and the density denominator
// is still 5, because the anonymous aggregate is post-filtered SERVER-SIDE from
// the set the viewer is entitled to, never recomputed from the rows in their
// own DOM. Computing it "from my DOM" would flatten every player's density to
// 1 of 1 and kill the lane (the W-G spec's audit V8, and it is written as a
// rule here because it will be implemented as written).

func TestRsvpOracle_PlayerReceivesNoLanesButTheFullDenominator(t *testing.T) {
	player := benchRsvpBuild(benchFxRsvpInput(false))
	gm := benchRsvpBuild(benchFxRsvpInput(true))

	if len(player.Lanes) != 0 {
		t.Errorf("a player received %d availability lanes; permission is ABSENCE, "+
			"and the absence is in the payload", len(player.Lanes))
	}
	if len(gm.Lanes) != 5 {
		t.Errorf("the Director received %d lanes, want one per member", len(gm.Lanes))
	}
	if len(player.Density) != 7 {
		t.Fatalf("a player received %d density columns, want 7", len(player.Density))
	}
	for i, d := range player.Density {
		if d.Total != 5 {
			t.Errorf("player density column %d denominator = %d, want the campaign's 5 — "+
				"the aggregate is filtered server-side, not from the viewer's own rows", i, d.Total)
		}
		if d != gm.Density[i] {
			t.Errorf("density column %d differs by role (%+v vs %+v); the aggregate is "+
				"everyone's and there is one arithmetic", i, d, gm.Density[i])
		}
	}
	// The full member table renders to a player — that is the SIGNED contract's
	// own reading and v4-bench-player-light.png shows it. It is asserted here so
	// nobody "fixes" it toward the unsigned spec's one-row player roster (§4).
	if len(player.Members) != 5 {
		t.Errorf("a player received %d member rows, want all 5 — answers, roles, zones and "+
			"local clocks are party-visible (§4)", len(player.Members))
	}

	html := renderBench(t, benchFxDataRsvp(false, false))
	if strings.Contains(html, "data-bench-rsvp-lane") {
		t.Error("a player's DOM carries an availability lane node")
	}
	for _, initials := range benchFxRsvpInitials() {
		if strings.Contains(html, `class="who"><span class="swatch`+initials) {
			t.Errorf("a player's DOM carries lane initials %q", initials)
		}
	}
	// A player receives NO `needs backend` chip anywhere on the page, and no
	// Director control (WG-8 / decisions/2026-07-27-needs-backend-audience.md).
	if strings.Contains(html, `class="badge need"`) {
		t.Error("a `needs backend` chip reached a player")
	}
	for _, ctl := range []string{">Propose<", ">Nudge<"} {
		if strings.Contains(html, ctl) {
			t.Errorf("a player's DOM carries the Director control %q — absent, not greyed", ctl)
		}
	}
}

// ── assertion 5 ────────────────────────────────────────────────────────────
//
// The Director's and the player's 3 / 5 are the SAME NUMBER COMPUTED THE SAME
// WAY. The surface has one arithmetic, not a GM one and a player one — role
// gates what is IN the payload, never how anything is counted.

func TestRsvpOracle_OneArithmeticAcrossRoles(t *testing.T) {
	gm := benchRsvpBuild(benchFxRsvpInput(true))
	player := benchRsvpBuild(benchFxRsvpInput(false))

	if gm.Headline != player.Headline {
		t.Errorf("the tally differs by role: %q vs %q", gm.Headline, player.Headline)
	}
	if gm.Silent != player.Silent {
		t.Errorf("the silent list differs by role: %q vs %q", gm.Silent, player.Silent)
	}
	if len(gm.Members) != len(player.Members) {
		t.Errorf("the member table differs by role: %d vs %d rows", len(gm.Members), len(player.Members))
	}
	for i := range gm.Members {
		a, b := gm.Members[i], player.Members[i]
		if a.Name != b.Name || a.Answer != b.Answer || a.Zone != b.Zone || a.LocalTime != b.LocalTime {
			t.Errorf("member row %d differs by role: %+v vs %+v", i, a, b)
		}
	}
	// And the tally is reproducible from the rendered HTML both roles receive.
	for _, tc := range []struct {
		name string
		gm   bool
	}{{"director", true}, {"player", false}} {
		html := renderBench(t, benchFxDataRsvp(tc.gm, tc.gm))
		if !strings.Contains(html, "3 / 5") {
			t.Errorf("%s's DOM does not carry the recomputed 3 / 5", tc.name)
		}
	}
}

// ── the derived window joins the oracle (WG-3) ─────────────────────────────
//
// A number with no denominator behind it is exactly what this slice exists to
// stop shipping, so the derived peak is asserted the same way: reproduced from
// the same per-hour counts the panel was given, and REFUSED below quorum.

func TestRsvpOracle_DerivedWindowIsReproducibleAndRefusesBelowQuorum(t *testing.T) {
	in := benchFxRsvpInput(true)
	p := benchRsvpBuild(in)

	if !p.RecDerived {
		t.Fatalf("the derived window did not render: %q", p.Rec)
	}
	// Recompute the peak independently from the fixture's own hour counts.
	peak := 0
	for _, d := range in.Avail.Days {
		for _, n := range d.Free {
			if n > peak {
				peak = n
			}
		}
	}
	if !strings.Contains(p.Rec, "of 5 members") {
		t.Errorf("the window's denominator is not the campaign's 5: %q", p.Rec)
	}
	if !strings.Contains(p.Rec, strconv.Itoa(peak)+" of 5") {
		t.Errorf("window %q does not state the recomputed peak of %d", p.Rec, peak)
	}
	if p.Bracket == nil {
		t.Error("a derived window must draw its bracket")
	}
	if p.Why == "" {
		t.Error("the derived window must name what it cannot include (ledger #16)")
	}

	// QUORUM REFUSAL. Two members' data is a guess wearing a number.
	thin := benchFxRsvpInput(true)
	thin.Avail.WithPattern = benchRsvpQuorum - 1
	refused := benchRsvpBuild(thin)
	if refused.RecDerived || refused.Bracket != nil {
		t.Error("the window ranked below quorum")
	}
	if !strings.Contains(refused.Rec, "Not enough saved availability") {
		t.Errorf("the refusal must say so plainly; got %q", refused.Rec)
	}
}

