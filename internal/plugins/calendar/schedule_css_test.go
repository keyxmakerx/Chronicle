package calendar

// schedule_css_test.go — the stylesheet IS the contract, so it is pinned
// (C-CALV4-RSVP-P8 Part B).
//
// The render tests assert on templ output and cannot see whether the sheet
// actually scopes itself, whether it quietly redefined a signed Bench selector,
// or whether somebody widened the motion allowlist. These read the sheet itself,
// and one of them reads BOTH sheets — because "this file redefines nothing the
// Bench defines" is a claim about a relationship, and a claim about a
// relationship asserted from one side is not asserted at all.
//
// PIN DISCIPLINE (COMMON §3): every assertion flattens comments out first and
// uses strings.Contains / strings.Count. Nothing here uses a bare strings.Index
// result as a slice bound.

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func scheduleCSSPath(t *testing.T, name string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path")
	}
	root := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	return filepath.Join(root, "static", "css", name)
}

func scheduleCSS(t *testing.T, name string) string {
	t.Helper()
	body, err := os.ReadFile(scheduleCSSPath(t, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	if len(body) == 0 {
		t.Fatalf("%s is empty", name)
	}
	return string(body)
}

var scheduleCommentRe = regexp.MustCompile(`(?s)/\*.*?\*/`)

// scheduleStrip removes comment bodies so prose about "transitions",
// "keyframes" or "@layer" cannot trip a rule that forbids them.
func scheduleStrip(css string) string { return scheduleCommentRe.ReplaceAllString(css, " ") }

// scheduleSelectorRe pulls the selector text ahead of each rule body.
var scheduleSelectorRe = regexp.MustCompile(`(?m)([^{};]+)\{`)

// TestScheduleCSS_EverySelectorIsScoped — the sheet is UNLAYERED, so it outranks
// the whole layered app cascade. A bare `.badge`, `.seg`, `.nm` or `.tok` rule
// here would restyle every one of those nouns product-wide, and they are the
// Bench's own vocabulary, deliberately reused so the two surfaces read alike —
// which makes the leak far likelier than usual, not less.
func TestScheduleCSS_EverySelectorIsScoped(t *testing.T) {
	code := scheduleStrip(scheduleCSS(t, "calendar-schedule.css"))
	for _, m := range scheduleSelectorRe.FindAllStringSubmatch(code, -1) {
		sel := strings.TrimSpace(m[1])
		if sel == "" || strings.HasPrefix(sel, "@") || strings.HasPrefix(sel, ")") {
			continue
		}
		for _, one := range strings.Split(sel, ",") {
			one = strings.TrimSpace(one)
			if one == "" {
				continue
			}
			if !strings.HasPrefix(one, ".cal-schedule") {
				t.Errorf("selector %q is not scoped under .cal-schedule — an unlayered "+
					"sheet outranks the whole app cascade and this would restyle the product", one)
			}
		}
	}
}

// scheduleBenchSelectors are the class names calendar-bench.css defines. The
// list is READ OFF THE SHEET rather than hand-written, because a hand-written
// list only catches what whoever wrote it thought of.
func scheduleBenchSelectors(t *testing.T) map[string]bool {
	t.Helper()
	code := scheduleStrip(scheduleCSS(t, "calendar-bench.css"))
	out := map[string]bool{}
	classRe := regexp.MustCompile(`\.[a-zA-Z][\w-]*`)
	for _, m := range scheduleSelectorRe.FindAllStringSubmatch(code, -1) {
		sel := strings.TrimSpace(m[1])
		if sel == "" || strings.HasPrefix(sel, "@") {
			continue
		}
		for _, one := range strings.Split(sel, ",") {
			one = strings.TrimSpace(one)
			if one == "" {
				continue
			}
			// The LAST class in the selector is the one being styled; a
			// descendant prefix is scope, not subject.
			classes := classRe.FindAllString(one, -1)
			if len(classes) == 0 {
				continue
			}
			out[classes[len(classes)-1]] = true
		}
	}
	return out
}

// TestScheduleCSS_RedefinesNoSignedBenchClass — THE CASCADE LAW, asserted.
//
// The sealed mockup declares `schedule` BEFORE `bench`, so a schedule rule can
// never override a signed Bench rule at any specificity. Production has no
// layers here, so the same guarantee has to be obtained BY CONSTRUCTION, and the
// construction is this:
//
//	a rule whose SUBJECT is a class calendar-bench.css also styles must be
//	qualified by an `sc-`-prefixed ANCESTOR, so it can only ever land on markup
//	this surface emits and can never reach a Bench element.
//
// That is not a technicality — it is the difference between reusing a signed
// primitive in a new context (legal, and what `.sc-head .hl` does) and
// redefining it (forbidden, and what would detach it everywhere). The Bench's
// own sheet already obeys the same discipline: it writes `.cal-bench .tile .hl`
// and `.cal-bench .rsvp .side .rec`, never a bare `.hl`.
//
// Where no context existed to qualify, the answer was a NEW `sc-` class beside
// the signed one — .sc-why (not .calrow .dt), .sc-foot (not .foot), .sc-body
// (not padding on .lhead). Those three are the drawing pass's own findings and
// they are the reason this test can be this strict.
func TestScheduleCSS_RedefinesNoSignedBenchClass(t *testing.T) {
	bench := scheduleBenchSelectors(t)
	code := scheduleStrip(scheduleCSS(t, "calendar-schedule.css"))
	classRe := regexp.MustCompile(`\.[a-zA-Z][\w-]*`)

	// The ONE unqualified exception, and it is listed because it was a
	// decision: `.seg` is not defined by calendar-bench.css at all (the sheet
	// only names it in a comment about not detaching it), and the rule here
	// grows a TRANSPARENT hit area to the 24px floor — no printed box, border,
	// pressed ring or ink weight moves.
	allowed := map[string]bool{"seg": true}

	for _, m := range scheduleSelectorRe.FindAllStringSubmatch(code, -1) {
		sel := strings.TrimSpace(m[1])
		if sel == "" || strings.HasPrefix(sel, "@") {
			continue
		}
		for _, one := range strings.Split(sel, ",") {
			one = strings.TrimSpace(one)
			classes := classRe.FindAllString(one, -1)
			if len(classes) == 0 {
				continue
			}
			subject := classes[len(classes)-1]
			if strings.HasPrefix(subject, ".sc-") || allowed[strings.TrimPrefix(subject, ".")] {
				continue
			}
			if !bench[subject] {
				continue
			}
			// A Bench-owned subject is legal ONLY inside this surface's own
			// markup, which means an `sc-` ancestor earlier in the selector.
			qualified := false
			for _, c := range classes[:len(classes)-1] {
				if strings.HasPrefix(c, ".sc-") {
					qualified = true
					break
				}
			}
			if !qualified {
				t.Errorf("selector %q styles %s, which calendar-bench.css also styles, without "+
					"an `sc-` ancestor to keep it off Bench markup — qualify it or add a new "+
					"`sc-` class beside the signed one", one, subject)
			}
		}
	}
}

// TestScheduleCSS_MotionIsTheSanctionedRegisterAndNothingElse.
//
// The operator signed "animations are top notch", and the signature maps that
// onto the register this design system already has: COLOUR AND OPACITY
// TRANSITIONS PLUS THE TOP-LAYER POPOVER. This asserts the four things that
// makes true:
//
//  1. the refusals stay refused (will-change / @starting-style / view-transition)
//  2. zero @keyframes
//  3. every transition lives inside ONE prefers-reduced-motion guard
//  4. only allowlisted properties are transitioned, and --dash / --gap never are
//     (canon guard B1 — morphing a dash pattern destroys the greyscale identity
//     channel)
//
// It also asserts the DISCLOSURE REGISTER MONOPOLY: there is exactly one in the
// product, it lives in calendar-bench.css under C-CALV4-BENCH-R2's guard, and
// opening a second one here would keep that guard green while defeating its
// purpose — which [DC-6] already refused once as laundering.
func TestScheduleCSS_MotionIsTheSanctionedRegisterAndNothingElse(t *testing.T) {
	code := scheduleStrip(scheduleCSS(t, "calendar-schedule.css"))

	for _, bad := range []string{"will-change", "@starting-style", "view-transition"} {
		if strings.Contains(code, bad) {
			t.Errorf("calendar-schedule.css contains %q — outside the motion budget entirely, "+
				"and refused rather than omitted", bad)
		}
	}
	if strings.Contains(code, "@keyframes") {
		t.Error("calendar-schedule.css declares a @keyframes — the sanctioned register is " +
			"colour/opacity transitions plus the top-layer popover, and nothing else")
	}
	if strings.Contains(code, "::details-content") || strings.Contains(code, "--disc-open") {
		t.Error("calendar-schedule.css opens a SECOND disclosure register — there is exactly " +
			"one, product-wide, in calendar-bench.css under C-CALV4-BENCH-R2's guard ([DC-6])")
	}

	guard := "@media (prefers-reduced-motion: no-preference)"
	if n := strings.Count(code, guard); n != 1 {
		t.Fatalf("found %d reduced-motion guards, want exactly 1 — the whole motion budget "+
			"must live inside one block so the budget is provable", n)
	}
	// Everything before the single guard must be motionless.
	head := code[:strings.Index(code, guard)]
	for _, prop := range []string{"transition:", "transition-property:", "animation:"} {
		if strings.Contains(head, prop) {
			t.Errorf("a %q declaration sits OUTSIDE the reduced-motion guard", prop)
		}
	}

	// The allowlist, and the B1 corollary.
	transitionRe := regexp.MustCompile(`transition:\s*([^;}]+)`)
	allowed := map[string]bool{"background-color": true, "color": true, "opacity": true}
	for _, m := range transitionRe.FindAllStringSubmatch(code, -1) {
		val := m[1]
		if strings.Contains(val, "--dash") || strings.Contains(val, "--gap") {
			t.Errorf("transition %q animates a greyscale-identity token (guard B1)", val)
		}
		prop := strings.Fields(strings.TrimSpace(val))
		if len(prop) == 0 {
			continue
		}
		if !allowed[prop[0]] {
			t.Errorf("transition %q is outside the sanctioned register (background-color, "+
				"color, opacity)", val)
		}
	}
}

// TestScheduleCSS_PrintsNoLayerStatement — production sheets are unlayered, and
// mixing one @layer into an unlayered sheet is worse than either regime alone:
// the layered half would lose to every unlayered rule in the same file.
func TestScheduleCSS_PrintsNoLayerStatement(t *testing.T) {
	code := scheduleStrip(scheduleCSS(t, "calendar-schedule.css"))
	if strings.Contains(code, "@layer") {
		t.Error("calendar-schedule.css declares an @layer — this sheet is unlayered, exactly " +
			"as calendar-block.css and calendar-bench.css are")
	}
}
