// css_contract_test.go — the stylesheet IS the contract, so it is pinned.
//
// The render tests assert on templ output and cannot see whether the sheet
// actually defines the container queries the markup depends on. That is the gap
// #568 fell into: every DOM assertion stayed green while the stylesheet dropped
// the utilities the markup named. These read the sheet itself.
//
// PIN DISCIPLINE (COMMON §3): every assertion below flattens whitespace first
// and uses strings.Count / strings.Contains. Nothing here uses a bare
// strings.Index result as a slice bound — that PANICS on a rename instead of
// failing cleanly, which is how calendar_v2_design_pass1_test.go:147 behaves and
// why it must never be copied.
package calendar_block

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// blockCSSPath resolves the sheet on disk. Separate from blockCSS because the
// generated ANSWER ladder is written back through it under UPDATE_ANSWER_LADDER.
func blockCSSPath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path")
	}
	root := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	return filepath.Join(root, "static", "css", "calendar-block.css")
}

func blockCSS(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(blockCSSPath(t))
	if err != nil {
		t.Fatalf("read calendar-block.css: %v", err)
	}
	if len(body) == 0 {
		t.Fatal("calendar-block.css is empty")
	}
	return string(body)
}

var cssCommentRe = regexp.MustCompile(`(?s)/\*.*?\*/`)

// stripComments removes comment bodies so prose about "transitions" cannot trip
// a rule that forbids them.
func stripComments(css string) string { return cssCommentRe.ReplaceAllString(css, " ") }

// ── the motion budget ───────────────────────────────────────────────────────

// motionBudget is the SIGNED budget (C-CALV4-LEDGER-P6 §8) expressed as data,
// so the test below is an ALLOWLIST rather than a ban.
var motionBudget = struct {
	// guard is the at-rule everything that moves must live inside. Under
	// reduced motion the colour change is instantaneous and every law of the
	// budget still holds — which is itself the proof that ANSWERED is a colour
	// STATE and not an animation.
	guard string
	// properties are the only things this stylesheet may transition. "none" is
	// in the set because M5's non-target silence and the gold rail's refusal
	// are both expressed as `transition: none`.
	properties map[string]bool
	// keyframes are the only named animations that may exist at all.
	keyframes map[string]bool
	// refused are outside the budget entirely, and they are REFUSALS rather
	// than omissions — each was considered and each is named in §MOTION.
	refused []string
}{
	guard: "@media (prefers-reduced-motion: no-preference)",
	properties: map[string]bool{
		"background-color": true, // canon A5: ANSWERED changes background-colour…
		"color":            true, // …and ink hue. ONLY.
		"transform":        true, // the rail's scaleX swell — never a width (§10 defect 3)
		"none":             true, // M5 silence, and the gold GM rail's refusal
		// ── AMENDED BY NAME, C-CALV4-MOONS [MN-3] / MN-G6 ──────────────────
		// The moon panel folds open, which is the canon's own second clause
		// ("one panel folds open over the rows beneath it") printed on every
		// render this slice was recovered from. Three properties, each argued
		// in §MOONPANEL's motion block rather than slipped in, and each pinned
		// to that one surface by TestCSS_TheMoonPanelIsTheOnlyAmendedSurface
		// below — so the amendment cannot spread by being available.
		"clip-path":          true, // the reveal. NOT block-size: a height animation on a grid child is LAYOUT
		"opacity":            true, // the content ramp, exactly as the band's .skpane-pad carries it
		"content-visibility": true, // [SKY-3]'s eighth-property argument, repeated: without it the close is a cut
	},
	keyframes: map[string]bool{
		"m-latch": true, // centre → corners on the viewer's own explicit act
	},
	refused: []string{"will-change", "@starting-style", "view-transition"},
}

// TestCSS_NoMotionAtAll — REWRITTEN AS AN ALLOWLIST by C-CALV4-LEDGER-P6, and
// deliberately not renamed and not deleted. A test that stops asserting is
// worse than no test, and this one still fails on everything outside the §8
// budget; what changed is that the budget is no longer empty.
//
// THE RULE, for whoever reads this next: the Block may transition
// background-colour, ink hue and one transform, on the surfaces that ANSWER
// each other, inside a prefers-reduced-motion guard — and nothing else, ever.
// The month grid never moves. If you are here because you added a transition
// and this failed, the question to answer is not "how do I allow it" but
// "which signed law does it satisfy" — canon A5 and A6, the 2026-07-27 motion
// policy, and §MOTION in the stylesheet itself all say what the answers are.
func TestCSS_NoMotionAtAll(t *testing.T) {
	code := stripComments(blockCSS(t))

	// 1. The refusals. These are not "not yet"; they are decided.
	for _, bad := range motionBudget.refused {
		if strings.Contains(code, bad) {
			t.Errorf("calendar-block.css contains %q — outside the motion budget entirely, "+
				"and refused rather than omitted (§MOTION)", bad)
		}
	}

	// 2. EVERYTHING THAT MOVES LIVES INSIDE THE REDUCED-MOTION GUARD. Not a
	//    politeness: it is what makes the budget provable — under reduced
	//    motion the Block still obeys every law above, so ANSWERED is a colour
	//    state rather than an animation.
	inside, outside, ok := splitAtRuleBlock(code, motionBudget.guard)
	if !ok {
		t.Fatalf("the stylesheet has no %q block — the whole motion budget must live inside one",
			motionBudget.guard)
	}
	for _, bad := range []string{"transition", "animation", "@keyframes"} {
		if strings.Contains(outside, bad) {
			t.Errorf("%q appears OUTSIDE %s — a viewer who asked for no motion would still get it",
				bad, motionBudget.guard)
		}
	}

	// 3. Only the allowlisted properties may be transitioned.
	transRe := regexp.MustCompile(`transition\s*:\s*([^;}]+)`)
	for _, m := range transRe.FindAllStringSubmatch(inside, -1) {
		for _, part := range strings.Split(m[1], ",") {
			fields := strings.Fields(part)
			if len(fields) == 0 {
				continue
			}
			if !motionBudget.properties[fields[0]] {
				t.Errorf("transition on %q is outside the motion budget — the budget is "+
					"background-color, color and transform, and nothing else (canon A5)", fields[0])
			}
		}
	}

	// 4. Only the allowlisted keyframes may exist, and only they may be run.
	kfRe := regexp.MustCompile(`@keyframes\s+([A-Za-z0-9_-]+)`)
	for _, m := range kfRe.FindAllStringSubmatch(inside, -1) {
		if !motionBudget.keyframes[m[1]] {
			t.Errorf("@keyframes %s is not in the motion budget — the budget names exactly "+
				"one, and it closes a ring in place rather than moving anything", m[1])
		}
	}
	animRe := regexp.MustCompile(`(^|[;{\s])animation\s*:\s*([^;}]+)`)
	for _, m := range animRe.FindAllStringSubmatch(inside, -1) {
		named := false
		for name := range motionBudget.keyframes {
			if strings.Contains(m[2], name) {
				named = true
			}
		}
		if !named {
			t.Errorf("animation %q runs something outside the budget's one keyframe set",
				strings.TrimSpace(m[2]))
		}
	}

	// 5. THE GOLD GM RAIL NEVER SWELLS (cv4:435). A permission marker that
	//    animates reads as a state change, so it is refused at the property
	//    level as well as declared `transform: none` in §ANSWER.
	flat := strings.Join(strings.Fields(inside), " ")
	if !strings.Contains(flat, ".lrow .gr { transition: none; }") {
		t.Error("the gold GM rail must explicitly refuse the transition its sibling rail takes")
	}

	// 6. M5 NON-TARGET SILENCE. Adding Blocks must add RESTING cost only; a
	//    Bench with four Blocks stays cheap because a Block nobody is pointing
	//    at transitions nothing.
	if !strings.Contains(flat, ":not(:hover):not(:focus-within):not([data-swapping])") {
		t.Error("M5 is missing: rail and row transitions must be killed unless the Block is " +
			"hovered, focused within or swapping — as a SELECTOR, not as a JS budget")
	}
}

// splitAtRuleBlock returns (inside, outside, ok) for the first at-rule block
// whose prelude matches `head`, by brace matching. Fails cleanly rather than
// slicing on a bare strings.Index result, which PANICS on a rename — see the
// PIN DISCIPLINE note at the top of this file.
func splitAtRuleBlock(code, head string) (inside, outside string, ok bool) {
	i := strings.Index(code, head)
	if i < 0 {
		return "", code, false
	}
	j := strings.Index(code[i:], "{")
	if j < 0 {
		return "", code, false
	}
	start := i + j
	depth := 0
	for k := start; k < len(code); k++ {
		switch code[k] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return code[start : k+1], code[:i] + code[k+1:], true
			}
		}
	}
	return "", code, false
}

// TestCSS_GuardB1_DashAndGapNeverAnimate. Morphing a dash pattern destroys the
// greyscale identity channel the signed greyscale render promises, and it is the
// obvious "improvement" the next hand reaches for. Kept as its own test so it
// survives W-B reintroducing motion for everything else.
func TestCSS_GuardB1_DashAndGapNeverAnimate(t *testing.T) {
	code := stripComments(blockCSS(t))
	for _, decl := range []string{"transition", "animation"} {
		for _, line := range strings.Split(code, "\n") {
			l := strings.ToLower(line)
			if strings.Contains(l, decl) && (strings.Contains(l, "--dash") || strings.Contains(l, "--gap")) {
				t.Errorf("canon guard B1: %q must never appear in a %s — %s", "--dash/--gap", decl, strings.TrimSpace(line))
			}
		}
	}
}

// TestCSS_GuardB2_NoFontShorthand. The `font:` shorthand with `inherit` as the
// family is invalid CSS. It silently dropped EIGHTY-ONE declarations in the v4
// mockup and nearly all type fell back to inherited 14px, with nothing failing
// loudly. This file uses longhands only.
func TestCSS_GuardB2_NoFontShorthand(t *testing.T) {
	code := stripComments(blockCSS(t))
	shorthand := regexp.MustCompile(`(^|[;{\s])font\s*:`)
	if loc := shorthand.FindStringIndex(code); loc != nil {
		t.Errorf("canon guard B2: the `font:` shorthand appears near %q — use longhands",
			strings.TrimSpace(code[loc[0]:min(loc[0]+80, len(code))]))
	}
}

// ── the cascade regime ──────────────────────────────────────────────────────

// TestCSS_UnlayeredAndSelfContained. input.css's content is layered; an
// unlayered standalone sheet beats every layer, which is exactly why the two
// cascade regimes never fight. Adding @layer here would put this sheet BACK into
// the fight it was written to avoid.
func TestCSS_UnlayeredAndSelfContained(t *testing.T) {
	code := stripComments(blockCSS(t))
	for _, bad := range []string{"@layer", "@apply", "@tailwind", "theme(", "@import"} {
		if strings.Contains(code, bad) {
			t.Errorf("calendar-block.css contains %q — the sheet is unlayered and self-contained "+
				"per the rendering-canvas CSS exemption", bad)
		}
	}
	// The exemption marker must stay, so the Wave-6 customization-readiness
	// sweep skips this file knowingly rather than treating it as debt.
	raw := blockCSS(t)
	for _, want := range []string{"EXEMPTION MARKER", "rendering-canvas-css-exemption"} {
		if !strings.Contains(raw, want) {
			t.Errorf("the exemption marker fragment %q is missing from the header comment", want)
		}
	}
}

var (
	// A rule prelude is everything between the previous }/;/{ and the next {.
	preludeRe = regexp.MustCompile(`(?s)(^|[;{}])([^;{}]*)\{`)
)

// stripKeyframes removes @keyframes blocks whole.
func stripKeyframes(code string) string {
	for {
		i := strings.Index(code, "@keyframes")
		if i < 0 {
			return code
		}
		j := strings.Index(code[i:], "{")
		if j < 0 {
			return code[:i]
		}
		depth, end := 0, -1
		for k := i + j; k < len(code); k++ {
			switch code[k] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					end = k + 1
				}
			}
			if end >= 0 {
				break
			}
		}
		if end < 0 {
			return code[:i]
		}
		code = code[:i] + code[end:]
	}
}

// splitSelectorList splits a rule prelude on its TOP-LEVEL commas only.
//
// REFRESHED BY C-CALV4-LEDGER-P6, and it was a latent defect in the guard, not
// a change of intent: a bare strings.Split(",") tears
// `:is(:hover, :focus-within)` in half and then reports the tail as an
// unscoped selector. That would have false-alarmed on any functional
// pseudo-class taking a selector LIST, which is a whole family of correct CSS
// — including every rule in the generated ANSWER ladder. The scoping rule
// itself is unchanged: nothing may match outside a .cal-block-host subtree.
func splitSelectorList(prelude string) []string {
	var out []string
	depth, start := 0, 0
	for i, r := range prelude {
		switch r {
		case '(', '[':
			depth++
		case ')', ']':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, prelude[start:i])
				start = i + 1
			}
		}
	}
	return append(out, prelude[start:])
}

// TestCSS_EverySelectorIsScoped. An unlayered sheet outranks the app's layered
// CSS, so a bare `.badge` rule in here would silently restyle the whole product.
// Nothing may match outside a .cal-block-host subtree.
func TestCSS_EverySelectorIsScoped(t *testing.T) {
	// @keyframes bodies carry KEYFRAME SELECTORS (from/to/50%), which are not
	// selectors at all and cannot be scoped. Removed before the scan by
	// C-CALV4-LEDGER-P6; the scoping rule itself is unchanged.
	code := stripKeyframes(stripComments(blockCSS(t)))
	var offenders []string
	for _, m := range preludeRe.FindAllStringSubmatch(code, -1) {
		prelude := strings.TrimSpace(m[2])
		if prelude == "" || strings.HasPrefix(prelude, "@") {
			continue // at-rules carry their own nested preludes, matched separately
		}
		for _, sel := range splitSelectorList(prelude) {
			sel = strings.TrimSpace(sel)
			if sel == "" {
				continue
			}
			if !strings.Contains(sel, ".cal-block-host") && !strings.Contains(sel, ".dark .cal-block-host") {
				offenders = append(offenders, sel)
			}
		}
	}
	if len(offenders) > 0 {
		t.Errorf("unscoped selector(s) would leak out of the Block and outrank the app's "+
			"layered CSS:\n  %s", strings.Join(offenders, "\n  "))
	}
}

// ── ten-day weeks are native ────────────────────────────────────────────────

// TestCSS_NoLiteralWeekLength. --week-len drives every column count. calv3
// hardcoded repeat(7,1fr) and that is the very week length this brief exists to
// escape.
func TestCSS_NoLiteralWeekLength(t *testing.T) {
	code := stripComments(blockCSS(t))
	if m := regexp.MustCompile(`repeat\(\s*\d`).FindString(code); m != "" {
		t.Errorf("a literal column count reached the grid: %q — every repeat() must take var(--week-len)", m)
	}
	gtc := regexp.MustCompile(`grid-template-columns\s*:\s*([^;}]+)`)
	found := 0
	for _, m := range gtc.FindAllStringSubmatch(code, -1) {
		val := strings.TrimSpace(m[1])
		found++
		if strings.Contains(val, "subgrid") {
			continue // rows inherit the month's tracks; that is the point of subgrid
		}
		if strings.Contains(val, "var(--week-len)") {
			continue
		}
		if strings.Contains(val, "minmax(0, 1fr) auto") {
			continue // the full-tier body: instrument + docked Ledger, not a week
		}
		// EXTENDED TO THE ALMANAC by C-CALV4-SHELF-P7. Zone D adds two grids
		// that are not the month, and both are allowed BY NAME rather than by
		// widening the rule:
		//
		//   · `auto 1fr` — the .kv definition list (Tonight and Moons). A
		//     two-column key/value list; there is no day in it.
		//   · `62px` — the Almanac LANE's fixed moon-name column, and only
		//     that. The lane's day columns come from grid-auto-flow, asserted
		//     below, so the day count is structural and no literal or variable
		//     repeat() exists for it at all.
		//
		// EXTENDED AGAIN BY C-CALV4-LAYERS-P9, by NAME and for the same reason:
		//
		//   · `52px` — the illumination graph's fixed moon-name column, on
		//     .sfrow and .sfaxis. The signed builder writes
		//     `repeat(30, minmax(0,1fr))` for the day columns TWICE, and a
		//     thirty-day month is exactly the assumption this rule exists to
		//     escape, so the graph takes them from grid-auto-flow as well. The
		//     allowlist grows one literal; the rule does not widen.
		if val == "auto 1fr" || val == "62px" || val == "52px" {
			continue
		}
		t.Errorf("grid-template-columns:%s is neither a subgrid, a --week-len repeat, the "+
			"body split, nor one of the Almanac's two named grids", val)
	}
	if found < 3 {
		t.Errorf("only %d grid-template-columns declarations found; expected at least the grid, "+
			"the row subgrid and the full-tier body", found)
	}
	if !strings.Contains(code, "calc(var(--week-len) * 30px)") {
		t.Error("the grid's min-width must scale with --week-len, not with a literal")
	}

	// ── the Almanac's own half of the same rule ────────────────────────────
	//
	// The lane is ONE COLUMN PER DAY OF THE MONTH — never per weekday, never a
	// literal. The mockup writes `repeat(30, 1fr)` twice (cv4:881, :887) and a
	// thirty-day month is exactly the assumption this whole brief exists to
	// escape, so the shipped lane uses grid-auto-flow instead: the columns
	// follow the CELLS, which the producer emitted one per real day.
	if !strings.Contains(code, "grid-auto-flow: column") {
		t.Error("the Almanac lane must take its day columns from grid-auto-flow — a repeat() " +
			"count would either be a literal month length or a second place the day count lives")
	}
	// --alm-days may appear ONLY inside a calc(). It is a length multiplier for
	// the scroller's minimum width, and the moment it becomes a repeat() count
	// the month's day count is declared in two places again.
	for _, m := range regexp.MustCompile(`[^(\s]*\(?[^;{}]*var\(--alm-days\)[^;{}]*`).FindAllString(code, -1) {
		if !strings.Contains(m, "calc(") {
			t.Errorf("--alm-days is used outside a calc(): %q — it is a LENGTH multiplier, "+
				"never a column count", strings.TrimSpace(m))
		}
	}
}

// TestCSS_BandHalfDrawsNoRightEdge. EraBand.Half means the half column lands
// INSIDE this band (r51, data.go) — but a band is one grid item spanning many
// columns, so its own border-right lands at the band's edge, wherever that
// happens to be, never reliably at the half column. Only the dedicated
// .halfrule ruler may draw the five-column rule across the band row (P5 §3.5);
// the half class on a band is a semantic marker and draws nothing.
func TestCSS_BandHalfDrawsNoRightEdge(t *testing.T) {
	code := stripComments(blockCSS(t))
	blockRe := regexp.MustCompile(`(?s)([^;{}]*)\{([^}]*)\}`)
	// The border shorthand sets all four sides, so it would smuggle the right
	// edge back in without the literal "border-right" ever appearing.
	borderShorthand := regexp.MustCompile(`(^|[;\s])border\s*:`)
	sawRuler := false
	for _, m := range blockRe.FindAllStringSubmatch(code, -1) {
		sel, body := strings.TrimSpace(m[1]), m[2]
		if strings.Contains(sel, ".halfrule") {
			// The ruler element is the ONE place the rule may be drawn — and
			// must keep being drawn, or deleting it becomes the next "fix".
			if strings.Contains(body, "border-right: 1px solid var(--rule-structural-strong)") {
				sawRuler = true
			}
			continue
		}
		if strings.Contains(sel, ".band") && strings.Contains(sel, ".half") &&
			(strings.Contains(body, "border-right") || borderShorthand.MatchString(body)) {
			t.Errorf("%q draws a border on the band's own edge — the half class marks the band, "+
				"it draws nothing; the .halfrule ruler owns the rule at the half column", sel)
		}
	}
	if !sawRuler {
		t.Error("the .halfrule ruler lost its structural border-right — it is the only element " +
			"that draws the five-column rule across the band row")
	}
}

// ── sizing is CSS, and it is these numbers ──────────────────────────────────

// TestCSS_SizingIsContainerQueries pins the four size-class thresholds and the
// density threshold to the same numbers sizing.go tests. A JS implementation is
// broken by construction on this platform (htmx.config.allowScriptTags = false,
// boot.js:163), so if these ever move the whole sizing story moves with them.
func TestCSS_SizingIsContainerQueries(t *testing.T) {
	code := stripComments(blockCSS(t))
	for _, want := range []string{
		"container-name: cal-block",
		"container-name: cal-cell",
		"@container cal-block (min-width: 900px)",                           // full
		"@container cal-block (min-width: 240px) and (max-width: 299.98px)", // mini
		"@container cal-block (max-width: 239.98px)",                        // sub-mini
		"@container cal-cell (min-width: 84px)",                             // density
		"@container cal-cell (min-width: 30px)",                             // the silhouette
		"@container cal-cell (min-width: 35px)",                             // the date's full size
		"@container cal-cell (min-width: 75px)",                             // the moon expansion
	} {
		if !strings.Contains(code, want) {
			t.Errorf("the stylesheet is missing %q", want)
		}
	}
	// ALL FOUR CELL THRESHOLDS ARE READ OUT OF GO, so the sheet and sizing.go
	// cannot drift the way they could while the discs shared the named row's
	// query and nothing named the number twice.
	for _, pin := range []struct {
		px   float64
		what string
	}{
		{NamedColWidthMin, "the named-event density flip"},
		{MoonSilhouetteColWidthMin, "the always-visible primary silhouette"},
		{CellCompactColWidthMin, "the date's full type size"},
		{MoonExpandColWidthMin, "the moon cluster's hover/focus expansion"},
	} {
		q := fmt.Sprintf("@container cal-cell (min-width: %gpx)", pin.px)
		if !strings.Contains(code, q) {
			t.Errorf("%s is %g in Go and the sheet declares no %q — the two thresholds are "+
				"the same contract written in two languages", pin.what, pin.px, q)
		}
	}
	// ── THE LADDER IS ORDERED, AND EACH RUNG ANSWERS A DIFFERENT QUESTION ───
	//
	// silhouette < date < expansion < names. Collapsing any adjacent pair is
	// the category error the original single 40px query WAS: one disc, a date,
	// three discs and an event name cost wildly different amounts of width, and
	// gating two of them together means the cheaper one is refused at widths it
	// would fit in — which is how the operator's ten-day phone drew nothing.
	for _, ord := range []struct {
		lo, hi     float64
		loN, hiN   string
		collapsing string
	}{
		{MoonSilhouetteColWidthMin, CellCompactColWidthMin,
			"the silhouette", "the date's full size",
			"one 8px disc costs less than a 12px date plus a 10px disc"},
		{CellCompactColWidthMin, MoonExpandColWidthMin,
			"the date's full size", "the expansion",
			"a 45px expanded row costs far more than a 16.7px date"},
		{MoonExpandColWidthMin, NamedColWidthMin,
			"the expansion", "the named-event flip",
			"three discs and a `+` cost less than an event NAME"},
	} {
		if ord.lo >= ord.hi {
			t.Errorf("%s (%g) must stay BELOW %s (%g). They are separate numbers because %s; "+
				"collapsing them is the defect the split was made to fix",
				ord.loN, ord.lo, ord.hiN, ord.hi, ord.collapsing)
		}
	}
	// ── THE 40px QUERY IS ASSERTED ABSENT: IT IS THE SHIPPED DEFECT ─────────
	//
	// It gated all three discs together, and the operator's ten-day Harptos
	// column measures 30.0px on a 360px phone, 33.0px at 390px and 37.0px at
	// 430px — all three under 40. That one query is exactly why the moons were
	// invisible on the only calendar they use.
	if strings.Contains(code, "@container cal-cell (min-width: 40px)") {
		t.Error("the sheet still carries `@container cal-cell (min-width: 40px)` — the " +
			"single all-or-nothing moon query. A ten-day week measures 30.0px on a phone, " +
			"so that query is what made the operator's own calendar draw no moons at all")
	}
	// ── AND THE SILHOUETTE'S THRESHOLD IS TIED TO THE GRID'S OWN FLOOR ──────
	//
	// MoonSilhouetteColWidthMin is the std tier's column floor, not a taste. If
	// a future change lowers `.grid`'s min-width, a column can become narrower
	// than the silhouette plus the date — which is a moon lying ACROSS a date
	// rather than a missing moon, and that is the worse of the two defects.
	if !strings.Contains(code, "min-width: calc(var(--week-len) * 30px)") {
		t.Errorf("the grid no longer declares the 30px-per-column floor that "+
			"MoonSilhouetteColWidthMin (%g) is read off. Lower it and a column can be "+
			"narrower than the silhouette plus the date",
			MoonSilhouetteColWidthMin)
	}
	// std is the BASELINE rather than a query, so a Block rendered before its
	// container is measured degrades to the middle class, not the widest.
	if strings.Contains(code, "@container cal-block (min-width: 300px)") {
		t.Error("std must be the unqueried baseline; a min-width:300px query would make " +
			"the pre-measurement state the mini one")
	}
	// The declared heights are invariant, so dropping a Block into any host page
	// can never shove that page around.
	for _, want := range []string{
		"min-height: 200px", "max-height: 520px", // std baseline
		"min-height: 220px", "max-height: 780px", // full
		"height: 180px", // mini
	} {
		if !strings.Contains(code, want) {
			t.Errorf("a declared height is missing: %q", want)
		}
	}
	// The docked Ledger's width IS the 300px the full-tier arithmetic subtracts.
	if !strings.Contains(code, "width: 300px") {
		t.Errorf("the docked Ledger must be %dpx wide — the same %dpx ColWidth subtracts",
			ledgerDock, ledgerDock)
	}
}

// ── canon D3 ────────────────────────────────────────────────────────────────

// TestCSS_D3_RingIsCarriedInsideBoxShadow. The border-thickening is a 0.5px ring
// carried INSIDE box-shadow, so a shadow animating alone is structurally
// impossible. That is the "shadows from nowhere" rejection, fixed at the
// primitive rather than per component.
func TestCSS_D3_RingIsCarriedInsideBoxShadow(t *testing.T) {
	code := stripComments(blockCSS(t))
	shadowRe := regexp.MustCompile(`box-shadow\s*:\s*([^;}]+)`)
	sawRing := false
	for _, m := range shadowRe.FindAllStringSubmatch(code, -1) {
		val := strings.TrimSpace(m[1])
		if strings.Contains(val, "var(--elev-1)") {
			if !strings.Contains(val, "0 0 0 .5px") {
				t.Errorf("box-shadow:%s raises elevation without carrying the 0.5px ring — "+
					"that is a shadow from nowhere", val)
			}
			sawRing = true
		}
	}
	if !sawRing {
		t.Error("the D3 hover primitive is missing: no box-shadow pairs --elev-1 with the ring")
	}
	// Rest is ALWAYS none, and selection is identity rather than elevation.
	flat := regexp.MustCompile(`\s+`).ReplaceAllString(code, " ")
	if !strings.Contains(flat, ".surf { background: var(--surface-card); border: 1px solid var(--rule-structural); box-shadow: none;") {
		t.Error(".surf must rest at box-shadow:none")
	}
	if !strings.Contains(flat, ".surf.act:active { background: color-mix(in oklch, var(--surface-card) 88%, var(--text-primary)); box-shadow: none;") {
		t.Error("press must RETURN the shadow to none — elevation decreases under the finger")
	}
	if !strings.Contains(flat, ".surf.sel:hover { border-color: var(--accent); background: color-mix(in oklch, var(--surface-card) 96%, var(--accent)); box-shadow: inset 0 0 0 1.5px var(--accent);") {
		t.Error("selection must be an inset ring with NO outer shadow — identity, not elevation")
	}
}

// ── colour is never load-bearing ────────────────────────────────────────────

// TestCSS_EightLockedPatterns. Every one of the eight must resolve with the hue
// removed; p7 and p8 are unassigned headroom and must still be defined, or the
// next axis to need one silently renders as p1.
func TestCSS_EightLockedPatterns(t *testing.T) {
	code := stripComments(blockCSS(t))
	// p1 is the bare `background-color: var(--axis)` case on .rail/.ulseg.
	if !strings.Contains(code, ".ulseg {\n  --dash: 100%;\n  --gap: 0px;\n  background-color: var(--axis);") &&
		!strings.Contains(regexp.MustCompile(`\s+`).ReplaceAllString(code, " "),
			".ulseg { --dash: 100%; --gap: 0px; background-color: var(--axis);") {
		t.Error("p1 (solid) is the unclassed baseline on .rail/.ulseg and must set --dash/--gap")
	}
	for _, p := range []string{"p2", "p3", "p4", "p5", "p6", "p7", "p8"} {
		if !strings.Contains(code, "."+p) {
			t.Errorf("locked pattern %s is not defined — colour would become the only channel for it", p)
		}
	}
	// Each dashed pattern owns a --dash and a --gap.
	for _, p := range []string{"p2", "p3", "p4", "p6", "p8"} {
		re := regexp.MustCompile(`\.` + p + `\s*\{[^}]*--dash[^}]*--gap[^}]*\}`)
		if !re.MatchString(code) {
			t.Errorf("%s must declare BOTH --dash and --gap", p)
		}
	}
	// p5 and p7 split the cross axis / notch the main axis rather than dashing.
	for _, want := range []string{".rail.p5", ".ulseg.p5", ".rail.p7", ".ulseg.p7"} {
		if !strings.Contains(code, want) {
			t.Errorf("%s must be defined on both mark orientations", want)
		}
	}
}

// TestCSS_ChannelDiscipline. --axis is the ONLY channel the marks layer may
// reference and it is FORBIDDEN from referencing --accent (canon A7). The moment
// accent means "related to what you're pointing at" it stops meaning "selected".
func TestCSS_ChannelDiscipline(t *testing.T) {
	code := stripComments(blockCSS(t))
	blockRe := regexp.MustCompile(`(?s)([^;{}]*)\{([^}]*)\}`)
	markSelectors := []string{".rail", ".ulseg", ".chip", ".tick i", ".ph"}
	for _, m := range blockRe.FindAllStringSubmatch(code, -1) {
		sel, body := strings.TrimSpace(m[1]), m[2]
		isMark := false
		for _, ms := range markSelectors {
			if strings.Contains(sel, ms) {
				isMark = true
				break
			}
		}
		if isMark && strings.Contains(body, "var(--accent)") {
			t.Errorf("the marks layer references --accent in %q — forbidden by canon A7", sel)
		}
	}
	// …and none of the three inherited colour channels is @property-registered
	// as animatable. That is a recorded REFUSAL, not an omission.
	if strings.Contains(code, "@property") {
		t.Error("canon A7 is a REFUSAL: --axis / --cal / --bandhue are never @property-registered " +
			"as animatable colours. Interpolating one is a per-frame style recalculation across " +
			"up to 900 gradient-backed nodes.")
	}
}

// ── the tie toggle's flip (C-CALV4-HOST-P3 §3) ──────────────────────────────

// TestCSS_TieToggleFlipsInkWithoutJS. The toggle's whole legality rests on the
// stylesheet: the markup tests can see that two radios exist, but only this can
// see whether anything reads them. Without the `:has()` rules the control is
// present, focusable, and does nothing — which is worse than absent.
//
// It also pins the TWO ink levels. The signed setTie() inks an untied mark at
// 0.28 in tied mode and at 0.7 in whole mode (calendar-v4.html:2823-2824);
// before this slice the sheet carried one branch at one level, so tied mode
// under-dimmed and whole mode did not dim at all.
func TestCSS_TieToggleFlipsInkWithoutJS(t *testing.T) {
	code := stripComments(blockCSS(t))
	flat := strings.Join(strings.Fields(code), " ")

	for _, want := range []string{
		`:has(.tiepick[data-tie-pick="tied"]:checked)`,
		`:has(.tiepick[data-tie-pick="whole"]:checked)`,
	} {
		if !strings.Contains(flat, want) {
			t.Errorf("no rule reads the tie radios (%s missing) — the control would be inert, "+
				"and it cannot be rescued with JS: a <script> in an HTMX-swapped fragment never runs", want)
		}
	}
	// The mark classes the flip re-inks must both be covered — a day carrying one
	// named chip and one underline segment must dim consistently.
	for _, want := range []string{".chip.untied", ".ulseg.untied"} {
		if strings.Count(flat, want) < 2 {
			t.Errorf("%s must be re-inked by BOTH the baseline and the :has() flip", want)
		}
	}
	// The server-rendered baseline survives for engines without :has().
	if !strings.Contains(flat, `[data-tie-mode="whole"]`) || !strings.Contains(flat, `[data-tie-mode="tied"]`) {
		t.Error("both data-tie-mode baselines must exist — they are the answer on an engine without :has()")
	}

	// Two levels, ordered. Tied mode must dim HARDER than whole mode, and whole
	// mode must still dim: an untied mark at full ink makes "which of these are
	// actually this entity's" unanswerable in whole mode.
	tied, whole := cssVarNumber(t, code, "--tied-ink"), cssVarNumber(t, code, "--untied-ink")
	if !(tied < whole) {
		t.Errorf("--tied-ink (%v) must be lower than --untied-ink (%v)", tied, whole)
	}
	if !(whole < 1) {
		t.Errorf("--untied-ink (%v) must dim: whole mode still distinguishes the entity's own days", whole)
	}
	if !(tied > 0) {
		t.Errorf("--tied-ink (%v) must stay visible: the toggle changes ink, never membership", tied)
	}
}

// cssVarNumber reads a numeric custom-property value out of the sheet. Fails
// cleanly when the property is gone, rather than panicking on a slice bound —
// see the PIN DISCIPLINE note at the top of this file.
func cssVarNumber(t *testing.T, code, name string) float64 {
	t.Helper()
	re := regexp.MustCompile(regexp.QuoteMeta(name) + `\s*:\s*([0-9.]+)\s*;`)
	m := re.FindStringSubmatch(code)
	if m == nil {
		t.Fatalf("calendar-block.css does not define %s", name)
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		t.Fatalf("%s is not a number: %q", name, m[1])
	}
	return v
}

// ── the ANSWER ladder (C-CALV4-LEDGER-P6 §1, CTS-1/CTS-2) ───────────────────

// TestCSS_AnswerLadderIsGenerated. CSS cannot compare two dynamic attribute
// values, so day answering needs one static rule set per day ordinal — ~144
// rules that nobody will ever hand-maintain correctly. They are GENERATED from
// answerLadderCSS() into the marked block at the foot of the sheet, and this
// test regenerates and diffs, which is the repo's existing snapshot idiom
// (UPDATE_ROUTES_SNAPSHOT, UPDATE_SANITIZE_SNAPSHOT).
//
// Regenerate with: UPDATE_ANSWER_LADDER=1 go test ./internal/widgets/calendar_block/
func TestCSS_AnswerLadderIsGenerated(t *testing.T) {
	path := blockCSSPath(t)
	raw := blockCSS(t)

	start := strings.Index(raw, answerLadderBegin)
	end := strings.Index(raw, answerLadderEnd)
	if start < 0 || end < 0 || end < start {
		t.Fatalf("the generated ladder's markers are missing or inverted — %q … %q must both "+
			"appear, in that order", answerLadderBegin, answerLadderEnd)
	}
	got := raw[start : end+len(answerLadderEnd)]
	want := answerLadderCSS()
	if got == want {
		return
	}
	if os.Getenv("UPDATE_ANSWER_LADDER") == "" {
		t.Errorf("the generated ANSWER ladder is stale (%d bytes on disk, %d generated). "+
			"Regenerate: UPDATE_ANSWER_LADDER=1 go test ./internal/widgets/calendar_block/",
			len(got), len(want))
		return
	}
	updated := raw[:start] + want + raw[end+len(answerLadderEnd):]
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		t.Fatalf("rewrite the ladder: %v", err)
	}
	t.Logf("ANSWER ladder regenerated: %d bytes (%d keys × 3 rules)",
		len(want), len(answerLadderKeys()))
}

// TestCSS_AnswerLadderAgreesWithGo pins the CTS-2 bound in BOTH places at once.
//
// The Go constants decide which days get a selection control (instrument.templ
// gates dayPick on dayAnswers / intercalaryAnswers) and the sheet decides which
// days a control can actually filter with. If those two numbers drift, a day
// gets a radio that no rule reads — a control that is present, focusable, and
// silently does nothing, which is the one outcome CTS-2 ruled out.
//
// It also pins the SECOND KEY NAMESPACE. An intercalary day's key is `iN`, and
// a ladder written only over `1..N` would make Midwinter stop answering with
// nothing failing — guard B4's failure mode one level up.
func TestCSS_AnswerLadderAgreesWithGo(t *testing.T) {
	code := stripComments(blockCSS(t))
	pickRe := regexp.MustCompile(`\.daypick\[data-day-pick="([^"]+)"\]:checked`)

	seen := map[string]bool{}
	for _, m := range pickRe.FindAllStringSubmatch(code, -1) {
		seen[m[1]] = true
	}
	for _, k := range answerLadderKeys() {
		if !seen[k] {
			t.Errorf("the sheet has no rule for ladder key %q — a day inside the Go bound "+
				"(%d ordinary + %d intercalary) whose control nothing reads",
				k, answerLadderDays, answerLadderIntercalary)
		}
		delete(seen, k)
	}
	for k := range seen {
		t.Errorf("the sheet carries ladder key %q, which is past the Go bound — the markup "+
			"never emits a control for it", k)
	}

	// Both namespaces, explicitly, so a future "simplification" to a numeric
	// loop fails here rather than in a fidelity review six months later.
	for _, want := range []string{`data-day-pick="1"`, `data-day-pick="i1"`,
		`data-lday="i1"`, `data-day-ord="i1"`} {
		if !strings.Contains(code, want) {
			t.Errorf("the ladder is missing %q — the intercalary namespace is not optional", want)
		}
	}
	for _, bad := range []string{`data-day-pick="41"`, `data-day-pick="i9"`} {
		if strings.Contains(code, bad) {
			t.Errorf("the ladder reaches past its own bound with %q", bad)
		}
	}
}

// TestCSS_AnswerLadderChangesNothingButVisibility. The Ledger's row filter is
// the ONE sanctioned content change in the Block, and the motion policy bounds
// it: "Cells, marks, moons and era bands do not animate, reflow, or change
// size." A ladder rule that set a height, a padding or a margin would make
// choosing a day reflow the month — the exact thing the docked Ledger exists to
// avoid, and the reason .lrows carries a min-height instead.
//
// So every generated rule may declare EXACTLY ONE of: display, or --answer.
//
// AND THE VALUE IS CHECKED, NOT ONLY THE PROPERTY. Allowlisting `display` by
// name is not enough and the fix round proved it: `display: block` on a flex
// row is EXACTLY the box change the paragraph above forbids, and it shipped
// under a guard that read the property and stopped. The ladder may only take a
// surface OUT of the flow (`none`) or put one back as itself (`revert`); it may
// never assert what kind of box anything is.
//
// The selector is checked too. A reveal rule may only name the two surfaces
// that are display:none at rest — the head's per-day context line and the
// per-day empty state. Any reveal that could reach `.lrow` is the shipped
// defect returning.
func TestCSS_AnswerLadderChangesNothingButVisibility(t *testing.T) {
	ladder := answerLadderCSS()
	ruleRe := regexp.MustCompile(`(?s)([^{}\n][^{}]*)\{([^}]*)\}`)
	allowedValues := map[string][]string{
		"display":  {"none", "revert"},
		"--answer": {"1"},
	}
	for _, m := range ruleRe.FindAllStringSubmatch(ladder, -1) {
		sel := strings.TrimSpace(m[1])
		for _, decl := range strings.Split(m[2], ";") {
			decl = strings.TrimSpace(decl)
			if decl == "" {
				continue
			}
			parts := strings.SplitN(decl, ":", 2)
			prop := strings.TrimSpace(parts[0])
			val := ""
			if len(parts) == 2 {
				val = strings.TrimSpace(parts[1])
			}
			ok, known := allowedValues[prop]
			if !known {
				t.Errorf("a generated ladder rule declares %q — the ladder may only change "+
					"what is SHOWN (display) and who is answered (--answer). Anything that "+
					"changes a box makes choosing a day reflow the month.", prop)
				continue
			}
			if !slices.Contains(ok, val) {
				t.Errorf("a generated ladder rule sets %s:%s on %q — the allowed values are %v. "+
					"`display:block` on a flex row is a BOX CHANGE, not a visibility change: it "+
					"collapses the signed one-line row into a stack while its fixed height, and "+
					"therefore every geometric assertion in the suite, stays identical.",
					prop, val, sel, ok)
			}
			// A FILTER NAMES ITS ZONE. `.lrow` is emitted by TWO surfaces from
			// wave 2 on — the docked Ledger and the Shelf's Upcoming panel,
			// which reuses the primitive verbatim so the zones cannot drift —
			// so an unscoped `.lrow:not(...)` filters the Shelf as well, and
			// the Shelf's body is content-driven under its 132px ceiling. That
			// made the Block 618px unselected and 532px selected: the docked
			// Ledger's own promise broken by its own mechanism. The filter must
			// name .lrows, which is what its doc comment always claimed.
			if prop == "display" && val == "none" && strings.Contains(sel, ".lrow") &&
				!strings.Contains(sel, ".lrows .lrow") {
				t.Errorf("a ladder filter rule targets %q — the row filter is bounded to "+
					"`.lrows .lrow`. Unscoped it reaches the Shelf's Upcoming panel, which "+
					"emits the SAME row primitive, and choosing a day then reflows the Block.", sel)
			}
			// A reveal names its surfaces. `.lrow` is never one of them.
			//
			// WIDENED BY ONE, EXPLICITLY, AT STAGE 3. `.ldp.lday[` — the
			// Ledger's day panel — is the third surface that is display:none
			// at rest, and it is added HERE, BY NAME, rather than by relaxing
			// the match. That distinction is the whole guard: the list is an
			// allowlist of specific tokens, so a FOURTH surface still fails
			// this test and has to be argued for, exactly as this one was
			// (helpers.go, answerLadderCSS rule 2). A loosened predicate —
			// "any selector carrying .lday" — would have let the row back in
			// the moment anyone re-added the token to ledgerRowClass, which is
			// the defect TestLedger_RowCarriesNoRevealToken exists to lock
			// from the other side.
			revealSurfaces := []string{".lctx[", ".lzero.lday[", ".ldp.lday["}
			if prop == "display" && val != "none" {
				for _, part := range strings.Split(sel, ",") {
					part = strings.TrimSpace(part)
					if part == "" {
						continue
					}
					named := false
					for _, s := range revealSurfaces {
						if strings.Contains(part, s) {
							named = true
							break
						}
					}
					if !named {
						t.Errorf("a ladder reveal rule targets %q — a reveal may only name the three "+
							"surfaces that are display:none at rest (%v). Anything wider reaches the "+
							"Ledger row, which is visible at rest and is filtered by attribute, never "+
							"revealed by class.", part, revealSurfaces)
					}
				}
			}
		}
	}
}

// TestLedger_RowCarriesNoRevealToken is the same rule stated on the OTHER side
// of the collision, in Go, where the row's class list is actually built.
//
// The guard above stops the sheet from reaching the row. This one stops the row
// from walking into the sheet. Either lock alone would have prevented what
// shipped; both together mean the defect cannot be re-opened by editing one
// file, which matters because the two files have different owners in wave 2 and
// the connection between them is a bare string token.
func TestLedger_RowCarriesNoRevealToken(t *testing.T) {
	for _, m := range []Mark{
		{Title: "plain"},
		{Title: "tied", Tied: true},
	} {
		cls := ledgerRowClass(m)
		for _, f := range strings.Fields(cls) {
			if f == "lday" {
				t.Errorf("ledgerRowClass(%+v) = %q — `.lday` marks a surface that is HIDDEN at "+
					"rest and revealed when its day is chosen. The row is visible at rest and "+
					"hidden by the filter; carrying the token made every listed row collapse "+
					"out of flex the moment a day was picked.", m, cls)
			}
		}
		if !strings.HasPrefix(cls, "lrow") {
			t.Errorf("ledgerRowClass(%+v) = %q — the row class leads with `lrow`", m, cls)
		}
	}
}

// ── C-CALV4-MOONS: the amendment is bounded, and the ladder is generated ────

// TestCSS_TheMoonPanelIsTheOnlyAmendedSurface is the OTHER HALF of the motion
// budget amendment, and it is what makes the amendment a widening of exactly
// one surface rather than of the sheet.
//
// `clip-path`, `opacity` and `content-visibility` were added to the allowlist
// for the moon panel's fold ([MN-3], MN-G6). An allowlist entry is available to
// every rule in the file the moment it exists, and "it was already allowed" is
// how a bounded exception becomes a general licence — the ladder's own scoping
// defect happened twice for the same reason (LEDGER-P6 §4.5, SHELF-P7). So each
// of the three may be transitioned on `.mpan` and nowhere else.
func TestCSS_TheMoonPanelIsTheOnlyAmendedSurface(t *testing.T) {
	code := stripComments(blockCSS(t))
	inside, _, ok := splitAtRuleBlock(code, motionBudget.guard)
	if !ok {
		t.Fatalf("the stylesheet has no %q block", motionBudget.guard)
	}
	amended := []string{"clip-path", "opacity", "content-visibility"}
	// Walk every rule inside the guard: prelude up to `{`, then its body.
	ruleRe := regexp.MustCompile(`(?s)([^{}]+)\{([^{}]*)\}`)
	found := map[string]int{}
	for _, m := range ruleRe.FindAllStringSubmatch(inside, -1) {
		prelude, body := m[1], m[2]
		if !strings.Contains(body, "transition") {
			continue
		}
		for _, prop := range amended {
			if !regexp.MustCompile(`transition[^;}]*\b` + regexp.QuoteMeta(prop) + `\b`).MatchString(body) {
				continue
			}
			found[prop]++
			if !strings.Contains(prelude, ".mpan") {
				t.Errorf("%q is transitioned on a selector that is not the moon panel:\n  %s\n"+
					"It was added to the motion budget for `.mpan`'s fold and for nothing "+
					"else ([MN-3], MN-G6). A bounded exception that becomes generally "+
					"available stops being bounded.", prop, strings.TrimSpace(prelude))
			}
		}
	}
	// AND THE AMENDMENT IS NOT DEAD. An allowlist entry with no rule behind it
	// is a widening nobody is paying for, and it would survive a revert of the
	// panel unnoticed.
	for _, prop := range amended {
		if found[prop] == 0 {
			t.Errorf("%q is in the motion budget but nothing transitions it. The moon "+
				"panel's fold is what bought this entry; if the fold is gone the entry "+
				"goes with it (MN-G6)", prop)
		}
	}
}

// TestCSS_MoonLadderIsGenerated is the ANSWER ladder's regeneration idiom,
// applied to the marking ladder [MN-3] option (a) called for. Same env var, so
// one regeneration command keeps both blocks honest.
//
// Regenerate with: UPDATE_ANSWER_LADDER=1 go test ./internal/widgets/calendar_block/
func TestCSS_MoonLadderIsGenerated(t *testing.T) {
	path := blockCSSPath(t)
	raw := blockCSS(t)

	start := strings.Index(raw, moonLadderBegin)
	end := strings.Index(raw, moonLadderEnd)
	if start < 0 || end < 0 || end < start {
		t.Fatalf("the generated moon ladder's markers are missing or inverted — %q … %q "+
			"must both appear, in that order", moonLadderBegin, moonLadderEnd)
	}
	got := raw[start : end+len(moonLadderEnd)]
	want := moonLadderCSS()
	if got == want {
		return
	}
	if os.Getenv("UPDATE_ANSWER_LADDER") == "" {
		t.Errorf("the generated MOON ladder is stale (%d bytes on disk, %d generated). "+
			"Regenerate: UPDATE_ANSWER_LADDER=1 go test ./internal/widgets/calendar_block/",
			len(got), len(want))
		return
	}
	updated := raw[:start] + want + raw[end+len(moonLadderEnd):]
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		t.Fatalf("rewrite the moon ladder: %v", err)
	}
	t.Logf("MOON ladder regenerated: %d bytes (%d keys × 1 rule)",
		len(want), len(answerLadderKeys()))
}

// TestCSS_MoonLadderIsScopedToTheRow is the scoping the ladder's older sibling
// lost TWICE (LEDGER-P6 §4.5's reveal rule; SHELF-P7's unscoped `.lrow` filter,
// measured as an 86px height change on the commonest interaction).
//
// Every marking rule must start at `.wk`. A rule that said `.block:has(...)`
// would mark the day in EVERY row's panel at once — all of them closed but one,
// and therefore invisible, which is exactly how the other two survived review.
func TestCSS_MoonLadderIsScopedToTheRow(t *testing.T) {
	for _, line := range strings.Split(moonLadderCSS(), "\n") {
		if !strings.Contains(line, "moonpick") {
			continue
		}
		if !strings.HasPrefix(line, ".cal-block-host .wk:has(") {
			t.Errorf("a moon ladder rule is not scoped to the week row:\n  %s\n"+
				"An unscoped :has() marks the day in every row's panel at once, and the "+
				"rows that are closed hide the fact", line)
		}
	}
}
