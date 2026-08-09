// theater_css_test.go — C-CALV4-THEATER (slice R2-3). The theater's sheet is
// GUARDED rather than trusted.
//
// The register's monopoly guard already reaches one file past the Bench
// (TestDayCardCSS_CarriesNoMotionOfItsOwn). This slice adds a SECOND satellite
// sheet, and a satellite out of reach of TestBenchCSS_NoMotionAtAll quietly
// growing a second grammar is precisely the failure [DC-6] SIGNED named. So the
// same guard is extended to calendar-theater.css verbatim, and joined by the
// scoping and defines-what-the-markup-names disciplines the other two sheets
// carry.
package calendar

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func theaterCSS(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path")
	}
	root := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	body, err := os.ReadFile(filepath.Join(root, "static", "css", "calendar-theater.css"))
	if err != nil {
		t.Fatalf("read calendar-theater.css: %v", err)
	}
	if len(body) == 0 {
		t.Fatal("calendar-theater.css is empty")
	}
	return string(body)
}

// THE REGISTER IS A MONOPOLY, AND THIS SHEET IS THE SECOND PLACE IT COULD HAVE
// BEEN BROKEN ([TH-3] SIGNED, [DC-6] SIGNED).
//
// The theater's two reveal rules live BY NAME inside the ONE register section
// of calendar-bench.css. Nothing here may declare a transition, a keyframe or a
// duration token of its own — a surface that wants a different feel is asking
// to LEAVE the register, and that is a signature rather than a style choice.
func TestTheaterCSS_CarriesNoMotionOfItsOwn(t *testing.T) {
	code := benchCommentRe.ReplaceAllString(theaterCSS(t), " ")
	for _, bad := range []string{
		"transition", "animation", "@keyframes", "will-change",
		"@starting-style", "view-transition", "--disc-",
	} {
		if strings.Contains(code, bad) {
			t.Errorf("calendar-theater.css contains %q — the theater's motion belongs in the "+
				"ONE register section of calendar-bench.css ([TH-3] SIGNED); a second sheet "+
				"with its own grammar is laundering ([DC-6])", bad)
		}
	}
}

// theaterAdmittedRoots is the whole scope of this sheet, as LITERAL class
// tokens rather than prefixes — the tightening the day card's guard learned the
// hard way at its stage 7, where a `strings.Contains` cut admitted
// `.cal-daycard-anything-at-all` by accident.
//
// TWO ROOTS AND A REASON FOR THE SECOND. `cal-theater` is the scaffold.
// `cal-theater-lock` is the class the module puts on the documentElement while
// the theater is open, because `<dialog>` gives inertness and focus containment
// for free and does NOT give scroll-lock — the page behind must not scroll, and
// that rule cannot name the scaffold because it is not inside it.
var theaterAdmittedRoots = []string{".cal-theater", ".cal-theater-lock"}

var theaterSelectorClassRe = regexp.MustCompile(`\.[A-Za-z_][\w-]*`)

func theaterSelectorIsRooted(sel string) bool {
	for _, tok := range theaterSelectorClassRe.FindAllString(sel, -1) {
		for _, root := range theaterAdmittedRoots {
			if tok == root {
				return true
			}
		}
	}
	return false
}

// EVERY SELECTOR IS SCOPED. This sheet is unlayered and outranks the whole
// layered app cascade, and `dialog`, `::backdrop`, `.thead`, `.thd`, `.tbody`
// and `.tclose` are exactly the generic nouns
// TestBenchCSS_EverySelectorIsScoped and TestDayCardCSS_EverySelectorIsScoped
// exist to catch. It reads the sheet BY BRACE (cssSelectors), not by line, so a
// rule written entirely on one line is examined too.
func TestTheaterCSS_EverySelectorIsScoped(t *testing.T) {
	sels := cssSelectors(benchCommentRe.ReplaceAllString(theaterCSS(t), " "))
	if len(sels) < 6 {
		t.Fatalf("only %d selectors found; the parser stopped reading the sheet", len(sels))
	}
	for _, sel := range sels {
		for _, part := range strings.Split(sel, ",") {
			p := strings.TrimSpace(part)
			if p == "" {
				continue
			}
			if !theaterSelectorIsRooted(p) {
				t.Errorf("unscoped selector in calendar-theater.css: %q (in prelude %q) — "+
					"an unlayered sheet that names a generic noun outranks the whole app cascade", p, sel)
			}
		}
	}
}

// THE SHEET DEFINES WHAT THE MARKUP NAMES. This is the gap #568 fell into:
// every DOM assertion stayed green while the stylesheet dropped what the markup
// depended on. theater.templ names every class below and this file is the only
// place they are defined.
func TestTheaterCSS_DefinesWhatTheMarkupNames(t *testing.T) {
	code := theaterCSS(t)
	for _, want := range []string{
		"dialog.cal-theater",
		"dialog.cal-theater::backdrop",
		".cal-bench.cal-theater .thead",
		".cal-bench.cal-theater .thd",
		".cal-bench.cal-theater .tclose",
		".cal-bench.cal-theater .tbody",
		"html.cal-theater-lock",
	} {
		if !strings.Contains(code, want) {
			t.Errorf("calendar-theater.css does not define %q", want)
		}
	}
}

// ── THE THREE MEASURED CLAIMS ───────────────────────────────────────────────
//
// §14 is a MEASUREMENT gate: there is no theater mockup, so geometry is settled
// by numbers. These assertions pin the numbers IN THE SHEET, so that a later
// hand cannot move the surface below full tier while every DOM assertion stays
// green — which is exactly the shape of the defect the arc keeps meeting.

// [TH-1]: the UA box is reset EXPLICITLY, never by inheritance.
func TestTheaterCSS_ResetsTheUABoxExplicitly(t *testing.T) {
	code := benchCommentRe.ReplaceAllString(theaterCSS(t), " ")
	box := cssRuleBody(t, code, "dialog.cal-theater")
	for _, want := range []string{"padding: 0", "border: 0", "max-inline-size:"} {
		if !strings.Contains(box, want) {
			t.Errorf("dialog.cal-theater does not state %q — a <dialog> arrives with "+
				"`max-width: calc(100%% - 12px)`, `padding: 1em` and a border from the UA "+
				"sheet, which is 46px of inset before the theater has spent a pixel of its own", want)
		}
	}
}

// [TH-1]'s 60px budget and [TH-16]'s height budget, both as arithmetic over the
// sheet's own declared numbers rather than as a claim in a comment.
//
// The theater's total horizontal inset around `.cal-block-host` is the viewport
// gutter (the difference between 100vw and the dialog's inline-size) plus the
// content region's padding, both sides. Full tier is
// `@container cal-block (min-width: 900px)`, so at the 1024px floor §14 names:
//
//	1024 − gutter − padding ≥ 900
//
// which is exactly the budget being ≤ 60 with 124px of headroom to spare. The
// numbers are extracted from the sheet so that changing one turns this red.
func TestTheaterCSS_TheHorizontalInsetStaysInsideItsBudget(t *testing.T) {
	code := benchCommentRe.ReplaceAllString(theaterCSS(t), " ")

	gutter := cssFirstInt(t, cssRuleBody(t, code, "dialog.cal-theater"), `inline-size:\s*calc\(100vw\s*-\s*(\d+)px\)`)
	pad := cssFirstInt(t, cssRuleBody(t, code, ".cal-bench.cal-theater .tbody"), `padding:\s*(\d+)px`)

	inset := gutter + 2*pad
	if inset > 60 {
		t.Errorf("the theater's total horizontal inset is %dpx (a %dpx viewport gutter + %dpx of "+
			"content padding each side); [TH-1] SIGNED caps it at 60px, because below 900px of "+
			"`.cal-block-host` the surface is not full tier and the slice has not shipped",
			inset, gutter, pad)
	}
	// …and the floor §14 names, computed rather than asserted.
	if host := 1024 - inset; host < 900 {
		t.Errorf("at a 1024px viewport `.cal-block-host` computes to %dpx, below the 900px "+
			"full-tier container floor — a STOP-AND-FLAG, not a trim", host)
	}
	// The cap the theater ADOPTS deliberately from `.cal-bench` is ≥ 900 too, so
	// full tier holds at every viewport the cap can produce.
	widest := cssFirstInt(t, cssRuleBody(t, code, "dialog.cal-theater"), `max-inline-size:\s*(\d+)px`)
	if got := widest - 2*pad; got < 900 {
		t.Errorf("at the widest the theater can be, `.cal-block-host` computes to %dpx — below full tier", got)
	}
}

// [TH-16] part 4, which is the part that will be got wrong: the register's
// reveal animates `.tbox`'s `block-size`, so a clamp or a scroll container
// placed THERE animates against a clamped box — the transition either does not
// run at all or runs to the wrong size, and every guard stays green.
func TestTheaterCSS_TheScrollContainerIsTheContentRegionAndNeverTheAnimatedBox(t *testing.T) {
	code := benchCommentRe.ReplaceAllString(theaterCSS(t), " ")

	body := cssRuleBody(t, code, ".cal-bench.cal-theater .tbody")
	if !strings.Contains(body, "overflow-y: auto") {
		t.Error("the content region does not scroll — at the 768px viewport floor a full-tier " +
			"theater wants ~850px and the close control would be unreachable")
	}
	if !strings.Contains(body, "max-block-size:") {
		t.Error("the content region carries no block-size budget, so nothing bounds the scroll")
	}
	if !strings.Contains(body, "overscroll-behavior: contain") {
		t.Error("a scroll that reaches the end of the region chains to the page behind it")
	}
	if !strings.Contains(cssRuleBody(t, code, "dialog.cal-theater"), "max-block-size:") {
		t.Error("the <dialog> relies on the UA's default block size — [TH-16] requires an explicit one")
	}
	// The two boxes that must NOT be the scroll container, by name.
	for _, sel := range []string{".tbox", ".cal-block-host"} {
		for _, r := range cssRules(t, code) {
			if !strings.Contains(r[0], sel) {
				continue
			}
			for _, banned := range []string{"overflow", "max-block-size", "max-height"} {
				if strings.Contains(r[1], banned) {
					t.Errorf("`%s` declares %q — %s must never be clamped or scrolled by this "+
						"sheet ([TH-16]: the reveal animates .tbox's block-size, and "+
						".cal-block-host is the container-query container)", r[0], banned, sel)
				}
			}
		}
	}
}

// ── tiny CSS readers, local to this file ────────────────────────────────────

var theaterRuleRe = regexp.MustCompile(`(?s)([^{}]+)\{([^{}]*)\}`)

func cssRules(t *testing.T, code string) [][2]string {
	t.Helper()
	var out [][2]string
	for _, m := range theaterRuleRe.FindAllStringSubmatch(code, -1) {
		sel := strings.TrimSpace(m[1])
		if sel == "" || strings.HasPrefix(sel, "@") {
			continue
		}
		// An at-rule prelude leaves its `@media …` text glued to the first
		// inner selector; drop everything up to the last `{` we skipped.
		if i := strings.LastIndex(sel, "{"); i >= 0 {
			sel = strings.TrimSpace(sel[i+1:])
		}
		out = append(out, [2]string{sel, m[2]})
	}
	if len(out) == 0 {
		t.Fatal("no rules parsed out of the sheet")
	}
	return out
}

func cssRuleBody(t *testing.T, code, selector string) string {
	t.Helper()
	for _, r := range cssRules(t, code) {
		if r[0] == selector {
			return r[1]
		}
	}
	t.Fatalf("no rule with the prelude %q", selector)
	return ""
}

func cssFirstInt(t *testing.T, body, pattern string) int {
	t.Helper()
	m := regexp.MustCompile(pattern).FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("pattern %q found nothing in %q", pattern, body)
	}
	n := 0
	for _, c := range m[1] {
		n = n*10 + int(c-'0')
	}
	return n
}
