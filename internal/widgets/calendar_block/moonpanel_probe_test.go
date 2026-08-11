// moonpanel_probe_test.go — the moon panel, measured on a real engine.
//
// C-CALV4-MOONS: MN-G5 (zero layout cost) · MN-G6 (the register's motion) ·
// MN-G7 (reduced motion is instant AND complete) · MN-G12 (a coarse pointer
// reaches the panel in one tap) · MN-G8 (every dimension asserts non-zero
// first).
//
// WHY THESE FOUR CANNOT BE A CSS GREP. "Zero layout cost" is a claim about
// rects DURING a transition, not about which properties are declared; "instant
// and complete" is a claim about where the open state LANDS under reduced
// motion, and a shortened transition looks identical in the sheet; and the
// panel's own box is the thing sky_measure_probe taught this package to
// distrust — it passed on a 0×0 disc for weeks, so every rect below is checked
// non-zero before it is compared to anything.
//
// THE OPEN IS A REAL HIT TEST. The click is dispatched on the node
// `document.elementFromPoint()` resolves at the cluster's painted centre, never
// on a node this script chose. A panel that opens when you call .click() on the
// label but not when a pointer lands on it is not open.
//
// IT SKIPS HONESTLY under -short or with no Chromium, and a skipped run is NOT
// a pass.
//
//	go test ./internal/widgets/calendar_block/ -run MoonPanelProbe -v
package calendar_block

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// mpSample is one frame's geometry: the surfaces that must not move, plus the
// panel's own box, plus the phase of the transition it was taken in.
type mpSample struct {
	T     float64 `json:"t"`
	Fixed string  `json:"fixed"` // grid + three cells + two rows, as one string
	PanW  float64 `json:"panW"`
	PanH  float64 `json:"panH"`
	PanOp float64 `json:"panOp"`
	Clip  string  `json:"clip"`
}

// mpReading is the whole run.
type mpReading struct {
	Reduced bool `json:"reduced"`
	// MN-G8 — the control's painted box, before anything is clicked.
	ClusterW float64 `json:"clusterW"`
	ClusterH float64 `json:"clusterH"`
	Discs    int     `json:"discs"`
	// The hit test at the cluster's centre and what it resolved to.
	Hit string `json:"hit"`
	// MN-G6 — the computed transition on `.mpan`, closed then open.
	DurClosed string `json:"durClosed"`
	DurOpen   string `json:"durOpen"`
	PropList  string `json:"propList"`
	// The overlay claim: the panel's top against the row's bottom, and whether
	// it actually covers the row beneath.
	RowBottom  float64 `json:"rowBottom"`
	PanTop     float64 `json:"panTop"`
	NextRowTop float64 `json:"nextRowTop"`
	PanBottom  float64 `json:"panBottom"`
	// The samples, from the click onward.
	Samples []mpSample `json:"samples"`
	// One tap, and the tab that came up with it.
	OpenedInOneTap bool `json:"openedInOneTap"`
	GraphVisible   bool `json:"graphVisible"`
	DetailsVisible bool `json:"detailsVisible"`
	// The interlock's markup half, read off the live DOM.
	MoonRadios int `json:"moonRadios"`
	Checked    int `json:"checked"`
	// How many CSSTransition objects the open actually started. Zero means the
	// fold never ran and every sample below would agree with itself about
	// nothing.
	Anims int `json:"anims"`
}

const moonPanelScript = `function(done){
  var host = document.querySelector('.probe-host');
  var block = host.querySelector('.block');
  var row = host.querySelector('.wk');
  var rows = [].slice.call(host.querySelectorAll('.wk'));
  var grid = host.querySelector('.grid');
  var cluster = row.querySelector('.phctl');
  var panel = row.querySelector('.mpan');
  var r1 = function(v){ return Math.round(v * 100) / 100; };
  var box = function(el){
    if (!el) return 'MISSING';
    var r = el.getBoundingClientRect();
    return [r1(r.left), r1(r.top), r1(r.width), r1(r.height)].join(',');
  };
  var cells = [].slice.call(host.querySelectorAll('.wk .cell')).slice(0, 3);
  // THE SURFACES THAT MAY NOT MOVE (MN-G5): the grid, three sample cells and
  // the first two rows. One string, so a diff is a diff and not six.
  var fixed = function(){
    return [box(grid)].concat(cells.map(box)).concat(rows.slice(0,2).map(box)).join(' | ');
  };
  var desc = function(el){
    if (!el) return '';
    var c = (el.getAttribute && el.getAttribute('class')) || '';
    return el.tagName.toLowerCase() + (c ? '.' + c.trim().split(/\s+/).join('.') : '');
  };
  var reduced = matchMedia('(prefers-reduced-motion: reduce)').matches;
  var cs = getComputedStyle(panel);
  var out = {
    reduced: reduced,
    clusterW: 0, clusterH: 0,
    discs: cluster ? cluster.querySelectorAll('.ph').length : 0,
    hit: '', durClosed: cs.transitionDuration, durOpen: '', propList: cs.transitionProperty,
    rowBottom: 0, panTop: 0, nextRowTop: 0, panBottom: 0,
    samples: [], openedInOneTap: false, graphVisible: false, detailsVisible: false, anims: 0,
    moonRadios: host.querySelectorAll('.moonpick').length,
    checked: host.querySelectorAll('.moonpick:checked').length
  };
  if (!cluster) { done(out); return; }
  var cr = cluster.getBoundingClientRect();
  out.clusterW = r1(cr.width); out.clusterH = r1(cr.height);
  if (cr.width <= 0 || cr.height <= 0) { done(out); return; }

  var hit = document.elementFromPoint(cr.left + cr.width / 2, cr.top + cr.height / 2);
  out.hit = desc(hit);

  out.samples.push({ t: -1, fixed: fixed(), panW: 0, panH: 0, panOp: 0, clip: '' });
  // ONE TAP. The click goes to the node the browser resolved, not to the label.
  if (hit) hit.click();

  // THE TRANSITION IS SEEKED, NOT WAITED ON, and that is a MEASURED fact about
  // this harness rather than a preference. Two earlier constructions produced
  // nothing usable: a requestAnimationFrame loop reading performance.now() never
  // reached its own exit condition under --virtual-time-budget, and a setTimeout
  // ladder ran to completion with every sample still at the CLOSED value —
  // virtual time advances timers but does not advance the animation clock, so
  // the fold sat at t=0 for the whole run and a probe that trusted it would have
  // reported "the panel never opens" about a panel that opens.
  //
  // getAnimations() hands back the live CSSTransition objects and seeking
  // its currentTime is exact and repeatable, which is strictly better than
  // sampling a wall clock: the same millisecond is read the same way on every
  // machine. Reading getComputedStyle right after the seek forces the style
  // recalc that makes the seek visible.
  var anims = panel.getAnimations();
  out.anims = anims.length;
  var sampleAt = function(ms){
    anims.forEach(function(a){ try { a.currentTime = ms; } catch (e) {} });
    var pr = panel.getBoundingClientRect();
    var ps = getComputedStyle(panel);
    out.samples.push({
      t: ms, fixed: fixed(),
      panW: r1(pr.width), panH: r1(pr.height),
      panOp: parseFloat(ps.opacity), clip: ps.clipPath
    });
  };
  [0, 16, 40, 80, 120, 160, 200].forEach(sampleAt);
  // …and the END of the fold, wherever the register put it.
  anims.forEach(function(a){ try { a.finish(); } catch (e) {} });
  sampleAt(200);
  finish();

  function finish(){
    var pr2 = panel.getBoundingClientRect();
    var rr = row.getBoundingClientRect();
    out.rowBottom = r1(rr.bottom);
    out.panTop = r1(pr2.top);
    out.panBottom = r1(pr2.bottom);
    out.nextRowTop = rows[1] ? r1(rows[1].getBoundingClientRect().top) : -1;
    out.openedInOneTap = pr2.width > 0 && pr2.height > 0 && parseFloat(getComputedStyle(panel).opacity) > 0.99;
    out.durOpen = getComputedStyle(panel).transitionDuration;
    var g = block.querySelector('.mpgraph'), dt = block.querySelector('.mpdetails');
    out.graphVisible = getComputedStyle(g).display !== 'none';
    out.detailsVisible = getComputedStyle(dt).display !== 'none';
    out.checked = host.querySelectorAll('.moonpick:checked').length;
    done(out);
  }
}`

// mpRunProbe boots one page and returns its reading.
func mpRunProbe(t *testing.T, chrome string, extraFlags []string) mpReading {
	t.Helper()
	d := fxAlmanac(t, true)
	markup := regexp.MustCompile(`<link[^>]*>`).ReplaceAllString(render(t, d), "")
	page := `<!doctype html><html><head><meta charset="utf-8"><style>` +
		`html,body{margin:0;background:#fff}` + blockCSS(t) +
		`.probe-host{display:block;margin:8px;max-width:none;width:1232px}` +
		`</style></head><body><div class="probe-host">` + markup + `</div>` +
		`<script>document.addEventListener('DOMContentLoaded',function(){` +
		`setTimeout(function(){(` + moonPanelScript + `)(function(out){` +
		`document.body.setAttribute('data-probe', JSON.stringify(out));});},200);});</script>` +
		`</body></html>`

	path := filepath.Join(t.TempDir(), "moonpanel.html")
	if err := os.WriteFile(path, []byte(page), 0o644); err != nil { //nolint:gosec // test artefact
		t.Fatalf("write probe page: %v", err)
	}
	args := append([]string{
		"--headless", "--no-sandbox", "--disable-gpu", "--hide-scrollbars",
		"--window-size=1320,1600", "--virtual-time-budget=6000",
	}, extraFlags...)
	args = append(args, "--dump-dom", "file://"+path)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, chrome, args...).Output()
	if err != nil {
		t.Fatalf("chromium: %v", err)
	}
	m := probePayloadRe.FindStringSubmatch(string(out))
	if m == nil {
		t.Fatal("no probe payload — the page script did not run")
	}
	var r mpReading
	if err := json.Unmarshal([]byte(html.UnescapeString(m[1])), &r); err != nil {
		t.Fatalf("probe payload: %v", err)
	}
	return r
}

// TestMoonPanelProbe_TheFoldCostsNothingAndObeysTheRegister is MN-G5 + MN-G6 +
// MN-G8 + MN-G12's one-tap half.
func TestMoonPanelProbe_TheFoldCostsNothingAndObeysTheRegister(t *testing.T) {
	if testing.Short() {
		t.Skip("the moon panel probe needs a browser; skipped under -short (CI's mode)")
	}
	chrome := findProbeChromium()
	if chrome == "" {
		t.Skip("no Chromium binary found (set CHROMIUM_BIN) — a skipped probe is NOT a pass")
	}
	r := mpRunProbe(t, chrome, nil)

	t.Logf("cluster %.2f×%.2fpx · %d discs · hit %q · %d moon radios (%d checked)",
		r.ClusterW, r.ClusterH, r.Discs, r.Hit, r.MoonRadios, r.Checked)
	t.Logf("transition closed=%q open=%q properties=%q", r.DurClosed, r.DurOpen, r.PropList)
	t.Logf("row bottom %.2f · panel %.2f→%.2f · next row top %.2f",
		r.RowBottom, r.PanTop, r.PanBottom, r.NextRowTop)
	for _, s := range r.Samples {
		t.Logf("  t=%.2fms · panel %.2f×%.2f · opacity %.3f · clip %s", s.T, s.PanW, s.PanH, s.PanOp, s.Clip)
	}

	// ── MN-G8 · the painted box, first, always ──────────────────────────────
	if r.ClusterW <= 0 || r.ClusterH <= 0 {
		t.Fatalf("the disc cluster paints %.2f×%.2fpx — a hit test at the centre of a "+
			"zero-size box is a hit test at an arbitrary page point, and every claim "+
			"below would be about nothing (MN-G8)", r.ClusterW, r.ClusterH)
	}
	if r.Discs != moonCap {
		t.Errorf("the cluster paints %d discs, want %d. [MN-1] is signed at three and "+
			"[MN-6] keeps all three always present and static", r.Discs, moonCap)
	}
	if r.MoonRadios == 0 {
		t.Fatal("no `.moonpick` radios in the DOM — the control has no state to hold")
	}

	// ── MN-G12 (the half a headless run can assert) · ONE TAP ───────────────
	if !r.OpenedInOneTap {
		t.Errorf("a single hit-tested click at the cluster's centre (which resolved to %s) "+
			"did not leave the panel open and fully revealed. There is no reveal-then-act "+
			"two-step on this control, on any pointer", r.Hit)
	}
	if r.Checked != 1 {
		t.Errorf("%d moon radios are checked after one tap, want exactly 1. ONE PANEL AT A "+
			"TIME is the radio group, in the markup rather than in a rule", r.Checked)
	}
	// The default tab is the Graph, and exactly one tab is showing.
	if !r.GraphVisible || r.DetailsVisible {
		t.Errorf("tabs after open: graph=%v details=%v — exactly one of the two shows, and "+
			"the Graph is the one v4-moons-graph-only.png opens on", r.GraphVisible, r.DetailsVisible)
	}

	// ── the canon's own words: OVER the rows beneath ────────────────────────
	if r.PanTop <= 0 || r.PanBottom <= r.PanTop {
		t.Fatalf("the panel's box is %.2f→%.2f — nothing to say about what it covers (MN-G8)",
			r.PanTop, r.PanBottom)
	}
	if diff := r.PanTop - r.RowBottom; diff < -1 || diff > 1 {
		t.Errorf("the panel's top is %.2f and its row's bottom is %.2f (%.2fpx apart). "+
			"\"menus appear where you clicked\" — it opens AT the row whose cluster was "+
			"pressed, not centred on the page and not docked to an edge", r.PanTop, r.RowBottom, diff)
	}
	if r.NextRowTop > 0 && r.PanBottom <= r.NextRowTop {
		t.Errorf("the panel ends at %.2f and the next row starts at %.2f — it is not "+
			"OVER anything. The canon's clause is \"one panel folds open over the rows "+
			"beneath it\"", r.PanBottom, r.NextRowTop)
	}

	// ── MN-G5 · ZERO LAYOUT COST, sampled ACROSS the transition ─────────────
	if len(r.Samples) < 4 {
		t.Fatalf("only %d samples across the open — too few to claim anything about what "+
			"happened during it", len(r.Samples))
	}
	base := r.Samples[0].Fixed
	if strings.Contains(base, "MISSING") {
		t.Fatalf("a sampled surface is missing: %s", base)
	}
	for _, s := range r.Samples {
		if s.Fixed != base {
			t.Errorf("geometry moved at t=%.2fms.\n  before: %s\n  during: %s\n"+
				"The month grid never moves: cells, marks, moons and era bands do not "+
				"animate, reflow or change size, and the sky band is the ONE named "+
				"interior exception (register clause 4 as amended). This panel is out of "+
				"flow precisely so it needs no second one (MN-G5)", s.T, base, s.Fixed)
			break
		}
	}
	// AND THE PANEL ITSELF REALLY DID ANIMATE. A fold that never moved would
	// satisfy every claim above for the wrong reason — which is the failure this
	// probe's own first two constructions actually produced.
	if r.Anims == 0 {
		t.Fatal("the open started NO CSS transitions. MN-G5 above would then be passing " +
			"because nothing happened, which is the shape of every guard this arc has had " +
			"to rewrite")
	}
	first, last := r.Samples[1], r.Samples[len(r.Samples)-1]
	if first.PanOp >= last.PanOp || last.PanOp < 0.99 {
		t.Errorf("opacity went %.3f → %.3f across the open — the reveal did not run, so "+
			"MN-G5 above passed because nothing happened", first.PanOp, last.PanOp)
	}
	// A REVEAL WITH A MIDDLE. A discrete flip would show 0 then 1 and nothing
	// between; the register's grammar is a ramp.
	mid := false
	for _, s := range r.Samples {
		if s.PanOp > 0.02 && s.PanOp < 0.98 {
			mid = true
		}
	}
	if !mid {
		t.Error("no sample caught the fold part-way. The register's grammar is a clip-reveal " +
			"plus an opacity RAMP; a surface that jumps is a surface that left the register")
	}

	// ── MN-G6 · the register's motion, and no fourth mechanism ──────────────
	if got := mpMS(r.DurClosed); got != 160 {
		t.Errorf("the closed-state transition-duration is %q (%.0fms), want 160ms. The BASE "+
			"rule carries the CLOSE timing and the open state overrides it, which is how "+
			"\"close is faster than open\" becomes structural", r.DurClosed, got)
	}
	if got := mpMS(r.DurOpen); got != 200 {
		t.Errorf("the open-state transition-duration is %q (%.0fms), want 200ms", r.DurOpen, got)
	}
	if mpMS(r.DurOpen) <= mpMS(r.DurClosed) {
		t.Errorf("close (%q) is not faster than open (%q) — register clause 2",
			r.DurClosed, r.DurOpen)
	}
	if strings.Contains(r.PropList, "transform") {
		t.Errorf("the panel transitions `transform` (%q). It is REFUSED by name, including "+
			"scale, exactly as it is on the sky band ([SKY-3])", r.PropList)
	}
	for _, want := range []string{"clip-path", "opacity", "content-visibility"} {
		if !strings.Contains(r.PropList, want) {
			t.Errorf("the panel does not transition %q (%q). The fold is a clip-reveal plus "+
				"an opacity ramp held open by content-visibility — drop any one and the "+
				"close becomes a cut, which is the defect sky_close_probe measured on the "+
				"band", want, r.PropList)
		}
	}
}

// TestMoonPanelProbe_ReducedMotionIsInstantAndComplete is MN-G7, and the word
// that matters is COMPLETE.
//
// A shortened transition is not reduced motion — it is the same motion in less
// time, and it reads as a glitch rather than as a preference honoured. The
// construction here is the sky band's exactly: every transition lives inside one
// `prefers-reduced-motion: no-preference` wrapper, so under `reduce` there is NO
// RULE AT ALL and the open lands on the end state in the first frame. Nothing
// seeds a start geometry, so there is no mid-reveal to land in.
func TestMoonPanelProbe_ReducedMotionIsInstantAndComplete(t *testing.T) {
	if testing.Short() {
		t.Skip("the moon panel probe needs a browser; skipped under -short (CI's mode)")
	}
	chrome := findProbeChromium()
	if chrome == "" {
		t.Skip("no Chromium binary found (set CHROMIUM_BIN) — a skipped probe is NOT a pass")
	}
	r := mpRunProbe(t, chrome, []string{"--force-prefers-reduced-motion"})

	// THE FLAG HAS TO HAVE TAKEN. A run that quietly stayed at no-preference
	// would assert the opposite of this test's claim and pass.
	if !r.Reduced {
		t.Skip("Chromium did not report prefers-reduced-motion: reduce — the flag did not " +
			"take on this build, and a run at the wrong preference is not evidence")
	}
	t.Logf("reduced · transition closed=%q open=%q", r.DurClosed, r.DurOpen)
	for _, s := range r.Samples {
		t.Logf("  t=%.2fms · panel %.2f×%.2f · opacity %.3f", s.T, s.PanW, s.PanH, s.PanOp)
	}

	if got := mpMS(r.DurClosed); got != 0 {
		t.Errorf("under reduced motion the panel still declares a %q transition. Every rule "+
			"must live inside the one no-preference wrapper, so `reduce` gets NO RULE AT "+
			"ALL rather than a faster one (MN-G7)", r.DurClosed)
	}
	if len(r.Samples) < 2 {
		t.Fatal("too few samples to say where the open landed")
	}
	// INSTANT: the very first frame after the tap is already the end state.
	f := r.Samples[1]
	if f.PanOp < 0.99 || f.PanW <= 0 || f.PanH <= 0 {
		t.Errorf("the first frame after the tap is %.2f×%.2fpx at opacity %.3f — under "+
			"reduced motion the open is instant AND COMPLETE, never shortened", f.PanW, f.PanH, f.PanOp)
	}
	// COMPLETE: and it stayed there.
	if !r.OpenedInOneTap {
		t.Error("the panel is not fully revealed at the end of a reduced-motion open")
	}
}

// mpMS parses a computed `transition-duration` list and returns its LARGEST
// component in milliseconds. The list is per-property and the totals are what
// the register declares, so the longest is the one the register names.
func mpMS(s string) float64 {
	var max float64
	for _, part := range strings.Split(s, ",") {
		p := strings.TrimSpace(part)
		var v float64
		switch {
		case strings.HasSuffix(p, "ms"):
			// A component that does not parse stays 0 and therefore never
			// becomes the maximum — which is the honest reading of "this list
			// did not contain a duration I understand".
			if _, err := fmt.Sscanf(p, "%fms", &v); err != nil {
				v = 0
			}
		case strings.HasSuffix(p, "s"):
			if _, err := fmt.Sscanf(p, "%fs", &v); err != nil {
				v = 0
			}
			v *= 1000
		}
		if v > max {
			max = v
		}
	}
	return max
}

// TestMoonPanelCSS_CoarsePointerHasNoHoverAndABiggerTarget is MN-G12's other
// half, read off the stylesheet.
//
// THIS COMMENT USED TO SAY "browser-free because Chromium has no CLI switch for
// a coarse pointer, and a probe that cannot emulate the condition cannot assert
// it". THAT WAS WRONG, and it cost this feature a real measurement: Blink's
// pointer settings are settable from the command line
// (`--blink-settings=primaryPointerType=2,availablePointerTypes=2`, verified
// against chromium-1194), and `matchMedia('(pointer: coarse)')` reads true under
// it. TestMoonReachProbe_TheOpenerOnATouchDevice now drives that arm for real
// and measures the target, the overspill and the one-tap open.
//
// THIS TEST STAYS ANYWAY, because the two ask different questions: this one
// asserts the RULES EXIST in the sheet at all — a deleted media block would make
// the browser arm measure a fine pointer's geometry and report it as a touch
// device's without noticing.
//
// TWO CLAIMS, both about the same media block: a coarse pointer gets NO hover
// state (there is no hover on touch, and a "hover" that latches on tap is the
// two-tap trap §5 rules out), and it gets a bigger hit area — WITHOUT the discs
// moving, because cells-zoom.png's anchor is the drawing and MN-G5 forbids
// paying for the target in layout.
func TestMoonPanelCSS_CoarsePointerHasNoHoverAndABiggerTarget(t *testing.T) {
	code := blockCSS(t)
	inside, _, ok := splitAtRuleBlock(code, "@media (pointer: coarse)")
	if !ok {
		t.Fatal("the sheet declares no `@media (pointer: coarse)` block — a coarse pointer " +
			"would inherit the fine pointer's hover state, which on touch means a tap that " +
			"looks like it did something and did not (MN-G12)")
	}
	if !strings.Contains(inside, ".phctl:hover .ph") {
		t.Error("the coarse block does not neutralise the cluster's hover state")
	}
	if !strings.Contains(inside, ".phctl::before") || !strings.Contains(inside, "position: absolute") {
		t.Error("the coarse block does not grow the cluster's hit area with an out-of-flow " +
			"pad. Growing it in flow would move the discs, which cells-zoom.png fixes and " +
			"MN-G5 forbids paying for")
	}
}
