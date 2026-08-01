// builder_css_contract_test.go — THE WIZARD'S MOTION BUDGET, EXPRESSED AS A
// TEST, AND IT LANDED BEFORE THE WIZARD'S CSS DID.
//
// THE FINDING THIS FILE CLOSES (C-CALV4-WIZARD-P13 §9): a wizard stylesheet
// would otherwise be governed by NOTHING.
//
//	static/css/calendar-block.css → motionBudget (css_contract_test.go:60-101)
//	static/css/calendar-bench.css → TestBenchCSS_NoMotionAtAll (an allowlist
//	                                since C-CALV4-BENCH-R2 [BR2-2])
//	internal/plugins/calendar/*.templ|*.css (ONE level) →
//	                                tools/check-v2-motion-discipline.sh, which
//	                                bans exactly three tokens and does not
//	                                police static/css/ at all
//	static/css/calendar-builder.css → nothing, and it is not even in the shell
//	                                guard's path
//
// decisions/2026-07-27-motion-policy.md §6 item 3 asked for the property
// allowlist on 2026-07-27, verbatim: "This is the durable part — everything
// above is advice until a guard enforces it." It was never built. W-H is the
// first wave that needs it and the wave the operator's signature is on the
// motion of ("animations are top notch", 2026-07-29), so W-H builds it.
//
// IT IS SCOPED TO THE WIZARD'S OWN SHEET. Widening
// tools/check-v2-motion-discipline.sh repo-wide would red-CI unrelated plugin
// surfaces; that is a hygiene slice and it is BOOKED (dispatch §9.2), not taken
// here.
//
// PIN DISCIPLINE (COMMON §3), inherited from css_contract_test.go: every
// assertion strips comments first and nothing uses a bare strings.Index result
// as a slice bound — that PANICS on a rename instead of failing cleanly.
package calendar

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// builderCSSPath resolves the wizard's sheet on disk, the way blockCSSPath
// resolves the Block's.
func builderCSSPath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path")
	}
	root := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	return filepath.Join(root, "static", "css", "calendar-builder.css")
}

func builderCSSRaw(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(builderCSSPath(t))
	if err != nil {
		t.Fatalf("read calendar-builder.css: %v", err)
	}
	if len(body) == 0 {
		t.Fatal("calendar-builder.css is empty")
	}
	return string(body)
}

var builderCommentRe = regexp.MustCompile(`(?s)/\*.*?\*/`)

// builderCSS is the sheet with comment bodies removed, so prose ABOUT
// transitions cannot trip a rule that forbids them.
func builderCSS(t *testing.T) string {
	t.Helper()
	return builderCommentRe.ReplaceAllString(builderCSSRaw(t), " ")
}

// ── the budget, expressed as data ───────────────────────────────────────────

// builderBudget is the SIGNED budget (C-CALV4-WIZARD-P13 §5.2, [WZ-8] SIGNED)
// expressed as data, so every assertion below is an ALLOWLIST rather than a
// ban. Its SMALLNESS is the point: five passes done exactly right read as
// craft; twelve done approximately read as noise (motion policy §3).
var builderBudget = struct {
	// guard is the at-rule everything that moves must live inside. [WZ-9]
	// SIGNED: Chronicle's shape, not the mockup's. static/css/input.css:31-47
	// already ships a global total-disable OUTSIDE every cascade layer, so
	// porting the mockup's token-zeroing `(reduce)` block would be a second,
	// weaker, contradictory policy in a product that has exactly one.
	guard string
	// properties are the only things this sheet may transition or move in a
	// keyframe. "none" is in the set because a refusal is spelled
	// `transition: none`.
	properties map[string]bool
	// clipPathOnlyIn — clip-path is a keyframe-only property and appears in
	// exactly one keyframe. It is NEVER a transition-property: canon A6 forbids
	// a travelling indicator and a latch is the sanctioned substitute.
	clipPathOnlyIn string
	// keyframes are the only named animations that may exist at all. No fifth.
	keyframes map[string]bool
	// refused are outside the budget entirely, and they are REFUSALS rather
	// than omissions. `view-transition` is refused here even though canon A2
	// PERMITS one for "builder step → builder step": W-H declines a permission
	// it does not need, because passes 1-3 already are that transition in CSS
	// with no JS (dispatch §5.5). That refusal is this sheet's, not the
	// product's — the Block's separate refusal is likewise scoped to its sheet.
	refused []string
	// durationTokens are the ONLY custom properties whose value may carry a
	// literal time. They are the definition sites of the composed ladder; every
	// rule in the sheet composes them and none restates a number.
	durationTokens map[string]bool
	// ladderProperty is a DELAY unit and illegal as a duration. It is legal in
	// exactly one prelude, and stepPanelClass is what that prelude must name.
	ladderProperty string
	stepPanelClass string
	// classAllow are the non-`wz-` class tokens this sheet may name. `dark` is
	// <html>'s theme marker (tailwind darkMode:'class', static/js/theme.js) and
	// is an ANCESTOR selector, never a descendant — it cannot reach a node
	// inside the Block, which is what the prefix rule exists to prevent.
	classAllow map[string]bool
}{
	guard: "@media (prefers-reduced-motion: no-preference)",
	properties: map[string]bool{
		"opacity":          true,
		"transform":        true,
		"background-color": true,
		"color":            true,
		"border-color":     true,
		"outline-color":    true,
		"box-shadow":       true,
		"none":             true,
	},
	clipPathOnlyIn: "m-latch",
	keyframes: map[string]bool{
		"m-latch":   true, // pass 3 + pass 5 — a latch, never a traveller
		"w-in":      true, // pass 2 — the station's rows, on the ladder
		"w-in-fine": true, // pass 1 — the title, at 0ms, because information never waits
		"pv-swap":   true, // pass 4 — the WHOLE preview surface, opacity only
	},
	refused: []string{"will-change", "@starting-style", "view-transition"},
	durationTokens: map[string]bool{
		"--t-fast": true, "--t-base": true, "--t-long": true,
		"--m-engage": true, "--m-release": true, "--m-state": true,
		"--m-exit": true, "--m-step": true,
	},
	ladderProperty: "--m-step",
	stepPanelClass: ".wz-panel",
	classAllow:     map[string]bool{"cal-builder": true, "dark": true},
}

// blockScopeRoot is the Block's own scoping-root class, assembled from two
// halves so that THIS FILE does not put the forbidden literal on disk in a
// place a careless grep would count. The rule it enforces is dispatch §5.4
// mechanism (3): the wizard's sheet names it ZERO times.
const blockScopeRoot = "cal-block" + "-host"

// ── a CSS block walker ──────────────────────────────────────────────────────

// builderRule is one brace-delimited block: its prelude (selector or at-rule),
// the at-rule preludes enclosing it outermost-first, and the declaration text
// directly inside it with nested blocks removed.
type builderRule struct {
	prelude   string
	ancestors []string
	decls     string
}

// builderParse walks the sheet by brace matching. It never slices on a bare
// strings.Index result, so a rename fails an assertion rather than panicking.
func builderParse(code string, ancestors []string, out *[]builderRule) {
	var prelude strings.Builder
	for i := 0; i < len(code); {
		switch code[i] {
		case '{':
			depth, j := 1, i+1
			for ; j < len(code) && depth > 0; j++ {
				switch code[j] {
				case '{':
					depth++
				case '}':
					depth--
				}
			}
			body := code[i+1 : max(j-1, i+1)]
			p := strings.Join(strings.Fields(prelude.String()), " ")
			prelude.Reset()
			if strings.Contains(body, "{") {
				inner := append(append([]string{}, ancestors...), p)
				builderParse(body, inner, out)
				*out = append(*out, builderRule{prelude: p, ancestors: ancestors})
			} else {
				*out = append(*out, builderRule{prelude: p, ancestors: ancestors, decls: body})
			}
			i = j
		case ';':
			if p := strings.Join(strings.Fields(prelude.String()), " "); p != "" {
				*out = append(*out, builderRule{prelude: p, ancestors: ancestors})
			}
			prelude.Reset()
			i++
		default:
			prelude.WriteByte(code[i])
			i++
		}
	}
}

func builderRules(t *testing.T) []builderRule {
	t.Helper()
	var out []builderRule
	builderParse(builderCSS(t), nil, &out)
	return out
}

// isSelector reports whether a prelude is a real selector rather than an
// at-rule, and whether it sits inside a @keyframes block (where `from`/`to`/
// percentages are not selectors at all).
func (r builderRule) isSelector() bool {
	if strings.HasPrefix(r.prelude, "@") || r.prelude == "" {
		return false
	}
	for _, a := range r.ancestors {
		if strings.HasPrefix(a, "@keyframes") {
			return false
		}
	}
	return true
}

func (r builderRule) insideKeyframes() (string, bool) {
	for _, a := range r.ancestors {
		if strings.HasPrefix(a, "@keyframes") {
			return strings.TrimSpace(strings.TrimPrefix(a, "@keyframes")), true
		}
	}
	return "", false
}

func (r builderRule) insideGuard() bool {
	for _, a := range r.ancestors {
		if strings.Contains(strings.Join(strings.Fields(a), " "), builderBudget.guard) {
			return true
		}
	}
	return false
}

// builderDecls splits a declaration body into (property, value) pairs.
func builderDecls(body string) [][2]string {
	var out [][2]string
	for _, d := range strings.Split(body, ";") {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		i := strings.Index(d, ":")
		if i < 0 {
			continue
		}
		out = append(out, [2]string{
			strings.TrimSpace(d[:i]),
			strings.Join(strings.Fields(d[i+1:]), " "),
		})
	}
	return out
}

// ── ASSERTION FAMILY 1 — the property allowlist ─────────────────────────────

// TestBuilderCSS_PropertyAllowlist. Every property named in a `transition`,
// a `transition-property` or a @keyframes block is one of the eight the budget
// names — plus clip-path, and ONLY inside @keyframes m-latch.
func TestBuilderCSS_PropertyAllowlist(t *testing.T) {
	for _, r := range builderRules(t) {
		kf, inKF := r.insideKeyframes()
		for _, d := range builderDecls(r.decls) {
			prop, val := d[0], d[1]

			if inKF {
				if prop == "clip-path" {
					if kf != builderBudget.clipPathOnlyIn {
						t.Errorf("clip-path animates inside @keyframes %s — it is legal in "+
							"@keyframes %s ONLY (canon A6: a latch, never a traveller)",
							kf, builderBudget.clipPathOnlyIn)
					}
					continue
				}
				if !builderBudget.properties[prop] {
					t.Errorf("@keyframes %s moves %q — outside the eight-property budget", kf, prop)
				}
				continue
			}

			if prop != "transition" && prop != "transition-property" {
				continue
			}
			for _, part := range strings.Split(val, ",") {
				fields := strings.Fields(part)
				if len(fields) == 0 {
					continue
				}
				named := fields[0]
				if named == "clip-path" {
					t.Errorf("`%s` transitions clip-path — clip-path is a KEYFRAME-only "+
						"property in this sheet and is never a transition-property", r.prelude)
					continue
				}
				if !builderBudget.properties[named] {
					t.Errorf("`%s` transitions %q — outside the eight-property budget "+
						"(dispatch §5.2, [WZ-8] SIGNED)", r.prelude, named)
				}
			}
		}
	}
}

// ── ASSERTION FAMILY 2 — the keyframe allowlist ─────────────────────────────

// TestBuilderCSS_KeyframeAllowlist. Exactly four keyframes exist, all four are
// budgeted, and no `animation` shorthand runs anything else.
func TestBuilderCSS_KeyframeAllowlist(t *testing.T) {
	code := builderCSS(t)

	kfRe := regexp.MustCompile(`@keyframes\s+([A-Za-z0-9_-]+)`)
	seen := map[string]bool{}
	for _, m := range kfRe.FindAllStringSubmatch(code, -1) {
		if !builderBudget.keyframes[m[1]] {
			t.Errorf("@keyframes %s is not in the budget — the budget names exactly four "+
				"and there is no fifth", m[1])
		}
		seen[m[1]] = true
	}
	for name := range builderBudget.keyframes {
		if !seen[name] {
			t.Errorf("@keyframes %s is budgeted but missing — a pass that is signed and "+
				"not shipped is a hole in the register, not a saving", name)
		}
	}

	animRe := regexp.MustCompile(`(^|[;{\s])animation\s*:\s*([^;}]+)`)
	for _, m := range animRe.FindAllStringSubmatch(code, -1) {
		named := false
		for name := range builderBudget.keyframes {
			if strings.Contains(m[2], name) {
				named = true
			}
		}
		if !named {
			t.Errorf("animation %q runs something outside the budget's four keyframes",
				strings.TrimSpace(m[2]))
		}
	}
}

// ── ASSERTION FAMILY 3 — the reduced-motion wrap, and no !important ─────────

// TestBuilderCSS_ReducedMotionWrap. Everything that moves lives inside the
// no-preference guard, so a viewer who asked for no motion gets none — and
// nothing here carries `!important`, because the GLOBAL guard
// (static/css/input.css:31-47) must win. A wizard that out-shouted it would be
// the second policy [WZ-9] refused.
func TestBuilderCSS_ReducedMotionWrap(t *testing.T) {
	code := builderCSS(t)

	if strings.Contains(code, "!important") {
		t.Error("calendar-builder.css declares !important — the global reduced-motion " +
			"guard sits outside every cascade layer and must win; this sheet never out-shouts it")
	}
	if strings.Contains(code, "prefers-reduced-motion: reduce") ||
		strings.Contains(code, "prefers-reduced-motion:reduce") {
		t.Error("the mockup's token-zeroing (reduce) block was PORTED — [WZ-9] SIGNED " +
			"refuses it: input.css already disables everything globally, and a second, " +
			"weaker policy in a product that has exactly one is worse than none")
	}

	for _, r := range builderRules(t) {
		if strings.HasPrefix(r.prelude, "@keyframes") && !r.insideGuard() {
			t.Errorf("%s sits OUTSIDE %s", r.prelude, builderBudget.guard)
		}
		for _, d := range builderDecls(r.decls) {
			if !strings.HasPrefix(d[0], "transition") && !strings.HasPrefix(d[0], "animation") {
				continue
			}
			if _, inKF := r.insideKeyframes(); inKF {
				continue
			}
			if !r.insideGuard() {
				t.Errorf("`%s` declares %q outside %s — a viewer who asked for no motion "+
					"would still get it", r.prelude, d[0], builderBudget.guard)
			}
		}
	}
}

// ── ASSERTION FAMILY 4 — duration discipline ────────────────────────────────

var builderTimeRe = regexp.MustCompile(`(^|[^\w.-])\d+(\.\d+)?m?s\b`)

// TestBuilderCSS_DurationDiscipline. No literal time value exists outside the
// eight token DEFINITIONS, and the delay ladder appears in exactly one prelude.
//
// The definition sites are exempt for a reason that is not a loophole: a token
// cannot be composed from itself, so --t-fast's `100ms` is the one place the
// number can live. Every rule in the sheet then composes it. What this forbids
// is a rule restating a number — which is how a register drifts into twelve
// approximate durations without anybody deciding to.
func TestBuilderCSS_DurationDiscipline(t *testing.T) {
	for _, r := range builderRules(t) {
		for _, d := range builderDecls(r.decls) {
			prop, val := d[0], d[1]
			if builderTimeRe.MatchString(val) && !builderBudget.durationTokens[prop] {
				t.Errorf("`%s` declares %s: %s — a literal duration outside a token "+
					"definition. Compose --t-fast/--t-base/--t-long or the --m-* family "+
					"(dispatch §5.2)", r.prelude, prop, val)
			}
			// The ladder: --m-step is a DELAY unit and is legal in exactly one
			// prelude, the step panel's.
			if strings.Contains(val, "var("+builderBudget.ladderProperty+")") &&
				!strings.Contains(r.prelude, builderBudget.stepPanelClass) {
				t.Errorf("`%s` uses %s — the ladder lives INSIDE the step panel and "+
					"nowhere else: never inside the preview, never on the rail, never on "+
					"the gallery", r.prelude, builderBudget.ladderProperty)
			}
			if prop == builderBudget.ladderProperty && !strings.Contains(r.prelude, ".cal-builder") {
				t.Errorf("`%s` defines %s — its one definition site is the token prelude",
					r.prelude, builderBudget.ladderProperty)
			}
		}
	}
}

// ── ASSERTION FAMILY 5 — THE LEAK ASSERTIONS ────────────────────────────────

// TestBuilderCSS_LeakGuard is the wave's most durable deliverable (dispatch
// §5.4/§9, and gate item §12.1(iv) in its MECHANICAL form).
//
// The design asks for a preview that cross-fades and the Block sits inside it,
// so this is where a well-meaning hand violates the Block's austerity. Three
// assertions, and each closes one route:
//
//	(a) every selector is rooted at .cal-builder — no rule escapes the surface
//	(b) every class token is `wz-`-prefixed — no rule can NAME a Block-internal
//	    node, which root-scoping alone does not prevent because a descendant
//	    combinator reaches straight through the Block's own scoping root
//	(c) that scoping root is never named at all
func TestBuilderCSS_LeakGuard(t *testing.T) {
	if n := strings.Count(builderCSSRaw(t), blockScopeRoot); n != 0 {
		t.Errorf("calendar-builder.css names the Block's scoping root %d time(s) — it must "+
			"be ZERO. The preview cross-fades by animating the WIZARD's own wrapper "+
			"(.wz-pv) with the Block inert inside it; naming that root is the only way "+
			"wizard motion can reach the month's interior (canon A8 L-M2)", n)
	}

	classRe := regexp.MustCompile(`\.(-?[A-Za-z_][A-Za-z0-9_-]*)`)
	for _, r := range builderRules(t) {
		if !r.isSelector() {
			continue
		}
		for _, sel := range strings.Split(r.prelude, ",") {
			sel = strings.TrimSpace(sel)
			if sel == "" {
				continue
			}
			if !strings.Contains(sel, ".cal-builder") {
				t.Errorf("selector %q is not rooted at .cal-builder — an unlayered sheet "+
					"outranks every Tailwind layer, so an unrooted rule restyles the product",
					sel)
			}
			for _, m := range classRe.FindAllStringSubmatch(sel, -1) {
				cls := m[1]
				if builderBudget.classAllow[cls] || strings.HasPrefix(cls, "wz-") {
					continue
				}
				t.Errorf("selector %q names class %q — every wizard class is `wz-`-prefixed. "+
					"`.badge`, `.field`, `.frow` and friends are LIVE PRODUCT CLASSES, and "+
					"`.cal-builder .badge{…}` would restyle every badge inside the embedded "+
					"Block (dispatch §1.1)", sel, cls)
			}
		}
	}
}

// ── ASSERTION FAMILY 6 — unlayered, self-contained, and the refusals ────────

// TestBuilderCSS_UnlayeredAndRefusals. input.css's content is layered; an
// unlayered standalone sheet beats every layer, which is exactly why the two
// cascade regimes never fight. Adding @layer here would put this sheet BACK
// into the fight it was written to avoid.
func TestBuilderCSS_UnlayeredAndRefusals(t *testing.T) {
	code := builderCSS(t)

	for _, bad := range []string{"@layer", "@apply", "@tailwind", "theme(", "@import"} {
		if strings.Contains(code, bad) {
			t.Errorf("calendar-builder.css contains %q — the sheet is unlayered and "+
				"self-contained, linked only through layouts.AssetURL", bad)
		}
	}
	for _, bad := range builderBudget.refused {
		if strings.Contains(code, bad) {
			t.Errorf("calendar-builder.css contains %q — outside the budget entirely, and "+
				"REFUSED rather than omitted (dispatch §5.2/§5.5)", bad)
		}
	}
	// The one sanctioned layout idiom motion policy §2 permits elsewhere, and
	// which this surface explicitly does not spend: no station discloses and
	// nothing unfolds, so the allowance must not sit spare in the guard.
	for _, bad := range []string{"0fr", "grid-template-rows: 0"} {
		if strings.Contains(code, bad) {
			t.Errorf("calendar-builder.css contains %q — the 0fr→1fr expand idiom is "+
				"forbidden here: no station discloses. A future station that needs one is "+
				"an amendment with a reason, not a spare allowance", bad)
		}
	}
	for _, r := range builderRules(t) {
		for _, d := range builderDecls(r.decls) {
			if strings.HasPrefix(d[0], "transition") && strings.Contains(d[1], "grid-template-rows") {
				t.Errorf("`%s` transitions grid-template-rows", r.prelude)
			}
		}
	}
}

// TestBuilderCSS_BlockAndBenchBudgetsUntouched pins the OTHER half of the leak
// guard: W-H widened neither neighbouring budget. LAYERS-P9 §2 refused to widen
// the Block's nine days after it was signed, on the grounds that "a second wave
// widening it is how an allowlist becomes a formality". W-H is the third, and
// it declines for the same reason — it did not need to, because its motion
// lives in its own file.
func TestBuilderCSS_BlockAndBenchBudgetsUntouched(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")

	for _, sheet := range []string{"calendar-block.css", "calendar-bench.css"} {
		body, err := os.ReadFile(filepath.Join(root, "static", "css", sheet))
		if err != nil {
			t.Fatalf("read %s: %v", sheet, err)
		}
		code := builderCommentRe.ReplaceAllString(string(body), " ")
		for name := range builderBudget.keyframes {
			if name == "m-latch" {
				continue // the Block's own single permitted keyframe, pre-dating W-H
			}
			if strings.Contains(code, name) {
				t.Errorf("%s names the wizard keyframe %q — wizard motion lives in "+
					"calendar-builder.css and nowhere else (dispatch §5.4 mechanism 1)",
					sheet, name)
			}
		}
		if strings.Contains(code, "cal-builder") {
			t.Errorf("%s names .cal-builder — the file separation is the first leak "+
				"mechanism and it runs in both directions", sheet)
		}
	}
}
