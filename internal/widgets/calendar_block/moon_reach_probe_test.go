// moon_reach_probe_test.go — CAN ANYONE ACTUALLY SEE THE MOON DISCS?
//
// WHY THIS FILE EXISTS. The per-day moon discs shipped, were reviewed and were
// screenshotted, and then no user could see them. `.phrow` is `display:none` at
// baseline, and its ONLY promotion was `@container cal-cell (min-width: 84px)` —
// the same query that promotes the NAMED-event subtree, which is where the row
// was nested. A day cell reaches 84px only for a SEVEN-day week at a viewport of
// 1024px or more; on a phone it is 30–53px, and a TEN-day week tops out at
// 82.4px on any monitor because .cal-bench caps the Block at 1180px. So the
// discs, the `.phrow.phctl` opener and the whole moon panel behind it rendered
// into a `display:none` subtree for essentially everybody.
//
// The existing probes could not catch it. container_query_probe_test.go
// measures DENSITY (named vs underline) and is right about it; moonpanel_probe
// _test.go measures the panel at a hardcoded 1232px host, where the discs are
// visible. Neither ever asked the operator's question: at the width of a phone,
// is there a moon on the screen?
//
// WHAT THIS FILE MEASURES, and why it is anchored to VIEWPORTS and not to host
// widths. A container query measures the CELL, so a host-width table cannot be
// read back to "what does a person with a 390px phone see" without the app
// shell's arithmetic in between. That arithmetic is reproduced here, once, from
// the shipped shell, and every row prints the whole chain:
//
//	viewport  →  main content box  →  .cal-bench cap  →  host  →  column
//
//	internal/templates/layouts/app.templ:73   <main … px-3 md:px-5 …>
//	                                          12px each side < md, 20px at md+
//	internal/templates/layouts/app.templ:117  the sidebar is `w-64` (256px) and
//	                                          `md:static md:shrink-0`, so from
//	                                          768px up it TAKES layout width.
//	                                          Default is pinned (app.templ:113,
//	                                          `localStorage… !== 'false'`); the
//	                                          48px auto-collapsed state is
//	                                          measured as its own arm below.
//	static/css/calendar-bench.css:1204        .cal-bench{max-width:1180px}
//	static/css/calendar-bench.css:431         .stack is a gapless flex column
//	                                          and .benchblock is display:block,
//	                                          so host == the capped content box
//
// The 1180px cap is why a 1920px monitor and a 1440px monitor are the SAME
// measurement, and it is why "buy a bigger screen" was never a fix.
//
// IT SKIPS HONESTLY under -short or with no Chromium, and a skipped run is NOT
// a pass. Registered in tools/check-browser-probes.sh so that on a machine that
// CAN run it, not running it is an error.
//
//	go test ./internal/widgets/calendar_block/ -run MoonReachProbe -v
package calendar_block

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// ── the shell chain, as arithmetic ─────────────────────────────────────────

const (
	// mrSidebarPx is the pinned sidebar's `w-64`. It is layout width from the
	// `md` breakpoint up (`md:static md:shrink-0`), and nothing below it.
	mrSidebarPx = 256
	// mrSidebarCollapsedPx is the auto-collapse strip (.sidebar-collapsed,
	// input.css:1144). A viewer who has unpinned the sidebar gets 208px more.
	mrSidebarCollapsedPx = 48
	// mrPadNarrow / mrPadWide are <main>'s px-3 / md:px-5, per side.
	mrPadNarrow = 12
	mrPadWide   = 20
	// mrMdPx is Tailwind's `md` breakpoint, where both of the above flip.
	mrMdPx = 768
	// mrBenchCapPx is .cal-bench's --bench-measure.
	mrBenchCapPx = 1180
)

// mrPadOverspillMax is how far the coarse-pointer touch target is allowed to
// reach past its own day cell, in px — and it is the SHIPPED construction's
// own arithmetic rather than a tolerance picked to make a test pass. The disc
// row sits 4px in from the cell's right edge and MN-G12's pad is 7px, so a
// wide cell's target has ended 3.0px past the rule since that pad landed.
const mrPadOverspillMax = 3.0

// mrHostWidth resolves a viewport width to the width the Block's host box
// actually gets on the Bench. sidebar is the sidebar's own layout width at
// md and up; it is ignored below md, where the sidebar is off-canvas.
func mrHostWidth(viewport, sidebar int) int {
	w := viewport
	if viewport >= mrMdPx {
		w -= sidebar + 2*mrPadWide
	} else {
		w -= 2 * mrPadNarrow
	}
	if w > mrBenchCapPx {
		w = mrBenchCapPx
	}
	return w
}

// mrCase is one row of the census.
type mrCase struct {
	viewport int
	sidebar  int
	weekLen  int
	label    string
	data     BlockData
}

// mrReading is what the in-page script measures for one case. Every dimension
// is read off a real rect: MN-G8's rule, learned the hard way when a 0×0 disc
// passed a display-property assertion for weeks.
type mrReading struct {
	Host       float64 `json:"host"`
	ColumnW    float64 `json:"columnW"` // the density container's own content box
	CellW      float64 `json:"cellW"`   // border box
	CellH      float64 `json:"cellH"`
	NamedShown bool    `json:"namedShown"`
	UnderShown bool    `json:"underShown"`

	// the discs
	RowShown bool    `json:"rowShown"`
	RowW     float64 `json:"rowW"`
	RowH     float64 `json:"rowH"`
	Discs    int     `json:"discs"`
	DiscW    float64 `json:"discW"`
	DiscH    float64 `json:"discH"`

	// ── THE RESTING CENSUS, WALKED OVER EVERY DATED CELL IN THE MONTH ──────
	//
	// The fields above describe ONE cell — the first that carries moons — and
	// that was enough while the row was an all-or-nothing ornament gated well
	// clear of the date. It is not enough now. The silhouette rests 4px from
	// the cell's right edge and the date rests at its left, so whether the two
	// collide depends on HOW WIDE THE DATE'S INK IS, and that varies across the
	// month: "3" is half the width of "30". A probe that measured day 3 and
	// declared the month clear would miss every two-digit day in it, which is
	// two thirds of a Harptos month.
	Cells        int     `json:"cells"`        // dated cells carrying a moon row
	RestDiscsMin int     `json:"restDiscsMin"` // fewest discs painted in any cell
	RestDiscsMax int     `json:"restDiscsMax"` // most discs painted in any cell
	RestPlus     int     `json:"restPlus"`     // cells painting a `+` AT REST — must be 0
	WorstOverlap float64 `json:"worstOverlap"` // px², the worst date-ink collision
	WorstDay     string  `json:"worstDay"`     // which day produced it
	WorstNumW    float64 `json:"worstNumW"`    // the widest date ink in the month
	WorstSpill   float64 `json:"worstSpill"`

	// ── THE EXPANSION, MEASURED THROUGH ITS OWN RULE ───────────────────────
	//
	// Driven by :focus-visible, not by a synthesised hover — the two are the
	// SAME declaration block in the sheet, and focus is the half a script can
	// honestly drive. It is measured on the WIDEST-DATE cell in the month, so
	// the expansion is judged against the worst collision it can have rather
	// than the first one it meets.
	ExpFocusOK bool    `json:"expFocusOK"` // :focus-visible really matched
	ExpDiscs   int     `json:"expDiscs"`
	ExpPlus    bool    `json:"expPlus"`
	ExpRowW    float64 `json:"expRowW"`
	ExpOverlap float64 `json:"expOverlap"`
	ExpSpill   float64 `json:"expSpill"`

	// ── THE SECOND TARGET (C-CALV4-SPEC §4, operator's answer 2) ───────────
	//
	// "Clicking the hovered silhouette opens the moon section; clicking
	// anywhere else in the cell opens the Ledger. Two targets, one cell,
	// cleanly separated." The moon half is measured above. This is the OTHER
	// half, and without it the probe would prove the moon target works while
	// saying nothing about whether it swallowed the day.
	//
	// Hit-tested at a point deliberately AWAY from the moon — low and to the
	// left, where the date and the event marks live — then really clicked, and
	// the day's own radio read back. A moon control that had grown over the
	// cell would resolve here and leave DselChecks false.
	DselHit    string `json:"dselHit"`    // what a tap in the cell body resolves to
	DselChecks bool   `json:"dselChecks"` // …and it really selects the day
	MoonStole  bool   `json:"moonStole"`  // the moon control answered instead

	// the opener, and whether a pointer landing on its painted centre reaches it
	CtlW  float64 `json:"ctlW"`
	CtlH  float64 `json:"ctlH"`
	Hit   string  `json:"hit"`
	HitOK bool    `json:"hitOK"`

	// the TOUCH target — the painted box plus whatever the coarse-pointer pad
	// adds — walked outward with elementFromPoint rather than read off the CSS.
	PadW     float64 `json:"padW"`
	PadH     float64 `json:"padH"`
	PadSteal float64 `json:"padSteal"` // how far it reaches into the next day
	PadHit   bool    `json:"padHit"`   // a point inside the pad really resolves to the control

	// does anything the discs sit on top of get covered? The day numeral is the
	// one thing in the cell that may not be occluded.
	NumBox  string  `json:"numBox"`
	RowBox  string  `json:"rowBox"`
	Overlap float64 `json:"overlap"` // intersection area with the numeral's INK, px²
	Spill   float64 `json:"spill"`   // how far the row hangs out of its own cell, px

	// a hover-free world: was the panel reached by a plain hit-tested click,
	// with no hover synthesised anywhere in this probe?
	OpensPanel bool `json:"opensPanel"`

	// which pointer arm produced this reading, read back from the page itself.
	Coarse bool `json:"coarse"`
}

const moonReachScript = `
function(root){
  var r1 = function(v){ return Math.round(v * 100) / 100 };
  var box = function(el){
    if (!el) return null;
    var r = el.getBoundingClientRect();
    return { l: r.left, t: r.top, w: r.width, h: r.height, r: r.right, b: r.bottom };
  };
  var str = function(b){ return b ? [r1(b.l), r1(b.t), r1(b.w), r1(b.h)].join(',') : 'MISSING' };
  var vis = function(b){ return !!b && b.w > 0 && b.h > 0 };
  var desc = function(el){
    if (!el) return '';
    var c = (el.getAttribute && el.getAttribute('class')) || '';
    return el.tagName.toLowerCase() + (c ? '.' + c.trim().split(/\s+/).join('.') : '');
  };
  // THE MEASURED CELL is the first in-range day cell that actually carries
  // moons — a cell with none would report "no discs" for the honest reason and
  // make the census say the opposite of what it means.
  var cell = null, cells = [].slice.call(root.querySelectorAll('.cell[data-day]'));
  for (var i = 0; i < cells.length; i++) {
    if (cells[i].querySelector('.phrow') || cells[i].querySelector('.moonpick')) { cell = cells[i]; break }
  }
  if (!cell) cell = cells[0];
  var out = {
    host: r1(root.getBoundingClientRect().width),
    columnW: 0, cellW: 0, cellH: 0, namedShown: false, underShown: false,
    rowShown: false, rowW: 0, rowH: 0, discs: 0, discW: 0, discH: 0,
    ctlW: 0, ctlH: 0, hit: '', hitOK: false,
    padW: 0, padH: 0, padSteal: 0, padHit: false,
    numBox: '', rowBox: '', overlap: 0, spill: 0, opensPanel: false,
    cells: 0, restDiscsMin: 0, restDiscsMax: 0, restPlus: 0,
    worstOverlap: 0, worstDay: '', worstNumW: 0, worstSpill: 0,
    expFocusOK: false, expDiscs: 0, expPlus: false, expRowW: 0,
    expOverlap: 0, expSpill: 0
  };
  if (!cell) return out;
  var cs = getComputedStyle(cell);
  var cb = box(cell);
  out.cellW = r1(cb.w); out.cellH = r1(cb.h);
  out.columnW = r1(cb.w
    - parseFloat(cs.borderLeftWidth) - parseFloat(cs.borderRightWidth)
    - parseFloat(cs.paddingLeft) - parseFloat(cs.paddingRight));
  out.namedShown = vis(box(cell.querySelector('.cnamed')));
  out.underShown = vis(box(cell.querySelector('.cunder')));

  var row = cell.querySelector('.phrow');
  var rb = box(row);
  out.rowShown = vis(rb);
  if (rb) { out.rowW = r1(rb.w); out.rowH = r1(rb.h); out.rowBox = str(rb) }
  var discs = row ? [].slice.call(row.querySelectorAll('.ph')) : [];
  // A disc that lays out at 0x0 is not a disc. Count only painted ones.
  var painted = discs.filter(function(d){ return vis(box(d)) });
  out.discs = painted.length;
  if (painted.length) {
    var db = box(painted[0]);
    out.discW = r1(db.w); out.discH = r1(db.h);
  }

  // THE DAY NUMERAL, MEASURED AS INK AND NOT AS A BOX. .dn is a full-column
  // display:block whose text is merely aligned inside it, so a box-vs-box
  // intersection reports a collision with every disc row at every width —
  // including the signed named cell, where the drawing puts the discs in the
  // corner precisely BECAUSE the glyphs are at the other end. A Range over the
  // numeral's own contents returns the glyphs (or the today pill), which is
  // what a reader actually sees.
  var num = null;
  var named = cell.querySelector('.cnamed'), under = cell.querySelector('.cunder');
  if (out.namedShown && named) num = named.querySelector('.dn');
  else if (under) num = under.querySelector('.dn');
  var nb = null;
  if (num) {
    var rng = document.createRange();
    rng.selectNodeContents(num);
    var nr = rng.getBoundingClientRect();
    nb = { l: nr.left, t: nr.top, w: nr.width, h: nr.height, r: nr.right, b: nr.bottom };
  }
  if (nb) out.numBox = str(nb);
  if (nb && rb && vis(nb) && vis(rb)) {
    var ow = Math.max(0, Math.min(nb.r, rb.r) - Math.max(nb.l, rb.l));
    var oh = Math.max(0, Math.min(nb.b, rb.b) - Math.max(nb.t, rb.t));
    out.overlap = r1(ow * oh);
  }
  // …and the row may not hang out of the cell it belongs to, in either axis.
  if (rb && vis(rb)) {
    var xo = Math.max(0, cb.l - rb.l) + Math.max(0, rb.r - cb.r);
    var yo = Math.max(0, cb.t - rb.t) + Math.max(0, rb.b - cb.b);
    out.spill = r1(Math.max(xo, yo));
  }

  // ── THE RESTING CENSUS, OVER EVERY DATED CELL IN THE MONTH ─────────────
  //
  // WHY EVERY CELL AND NOT THE FIRST. The silhouette rests 4px from the cell's
  // right edge and the date rests at its left, so whether they collide depends
  // on how wide the DATE'S INK is — and "3" is half the width of "30". The
  // single-cell measurement above would clear a month whose two-digit days all
  // collide, which is two thirds of a Harptos month. The widest date in the
  // month is also the cell the expansion is measured on, below.
  var numInk = function(cl){
    var named = cl.querySelector('.cnamed'), under = cl.querySelector('.cunder');
    var n = null;
    if (named && vis(box(named))) n = named.querySelector('.dn');
    else if (under) n = under.querySelector('.dn');
    if (!n) return null;
    var rg = document.createRange();
    rg.selectNodeContents(n);
    var rr = rg.getBoundingClientRect();
    return { l: rr.left, t: rr.top, w: rr.width, h: rr.height, r: rr.right, b: rr.bottom };
  };
  var rectOverlap = function(a, b){
    if (!vis(a) || !vis(b)) return 0;
    var ow = Math.max(0, Math.min(a.r, b.r) - Math.max(a.l, b.l));
    var oh = Math.max(0, Math.min(a.b, b.b) - Math.max(a.t, b.t));
    return ow * oh;
  };
  var spillOf = function(inner, outer){
    var sx = Math.max(0, outer.l - inner.l) + Math.max(0, inner.r - outer.r);
    var sy = Math.max(0, outer.t - inner.t) + Math.max(0, inner.b - outer.b);
    return Math.max(sx, sy);
  };
  var dated = [].slice.call(root.querySelectorAll('.cell[data-day]'));
  var lo = -1, hi = 0, plusRest = 0, worstOv = 0, worstDay = '', worstNumW = 0;
  var worstSp = 0, seen = 0, widestCell = null, widestW = -1;
  for (var k = 0; k < dated.length; k++) {
    var cl = dated[k];
    var rw = cl.querySelector('.phrow');
    if (!rw) continue;
    seen++;
    var rwb = box(rw);
    var painted = [].slice.call(rw.querySelectorAll('.ph')).filter(function(d){ return vis(box(d)) });
    if (lo < 0 || painted.length < lo) lo = painted.length;
    if (painted.length > hi) hi = painted.length;
    var plg = rw.querySelector('.phplus');
    if (plg && vis(box(plg))) plusRest++;
    var nbk = numInk(cl);
    if (nbk && nbk.w > worstNumW) worstNumW = r1(nbk.w);
    if (nbk && nbk.w > widestW) { widestW = nbk.w; widestCell = cl }
    if (vis(rwb)) {
      var ovk = rectOverlap(nbk, rwb);
      if (ovk > worstOv) { worstOv = ovk; worstDay = cl.getAttribute('data-day-ord') || '?' }
      var spk = spillOf(rwb, box(cl));
      if (spk > worstSp) worstSp = spk;
    }
  }
  out.cells = seen;
  out.restDiscsMin = (lo < 0) ? 0 : lo;
  out.restDiscsMax = hi;
  out.restPlus = plusRest;
  out.worstOverlap = r1(worstOv);
  out.worstDay = worstDay;
  out.worstNumW = worstNumW;
  out.worstSpill = r1(worstSp);

  // ── THE EXPANSION, MEASURED THROUGH ITS OWN RULE ───────────────────────
  //
  // NO HOVER IS SYNTHESISED HERE EITHER. The sheet gives the expansion two
  // triggers in ONE declaration block — .phctl:hover and the cluster radio's
  // :focus-visible — and focus is the half a script can drive honestly. So
  // this measures the real rule through its real selector, and it doubles as
  // the keyboard arm's own test: a change that broke focus expansion but left
  // hover working would redden here.
  //
  // ON THE WIDEST-DATE CELL, so the expansion is judged against the worst
  // collision it can have rather than the first one it meets.
  if (widestCell) {
    var wrow = widestCell.querySelector('.phrow');
    var radio = widestCell.querySelector('.moonpick');
    if (wrow && radio) {
      radio.focus();
      out.expFocusOK = radio.matches(':focus-visible');
      var eb = box(wrow);
      out.expDiscs = [].slice.call(wrow.querySelectorAll('.ph')).filter(function(d){ return vis(box(d)) }).length;
      var epg = wrow.querySelector('.phplus');
      out.expPlus = !!(epg && vis(box(epg)));
      if (vis(eb)) {
        out.expRowW = r1(eb.w);
        out.expOverlap = r1(rectOverlap(numInk(widestCell), eb));
        out.expSpill = r1(spillOf(eb, box(widestCell)));
      }
      radio.blur();
    }
  }

  return out;
}`

// moonReachOpenScript is the SECOND phase, and splitting it out of the first is
// a correctness fix rather than tidiness.
//
// CHROMIUM'S :focus-visible IS STATEFUL ACROSS THE WHOLE DOCUMENT. The heuristic
// asks whether the most recent interaction was a POINTER one, so the moment any
// host's opener is `click()`ed, every LATER host's programmatic `.focus()` stops
// matching :focus-visible — and the expansion measured through it silently reads
// as "no expansion". Run as one pass this reported a working expansion for host
// 0 and a broken one for the other nineteen, which is indistinguishable from the
// rule being missing.
//
// So: every host's geometry (including the focus-driven expansion) is measured
// first, across the whole page, and only then does any host get clicked.
const moonReachOpenScript = `
function(root, out){
  var r1 = function(v){ return Math.round(v * 100) / 100 };
  var box = function(el){
    if (!el) return null;
    var r = el.getBoundingClientRect();
    return { l: r.left, t: r.top, w: r.width, h: r.height, r: r.right, b: r.bottom };
  };
  var vis = function(b){ return !!b && b.w > 0 && b.h > 0 };
  var desc = function(el){
    if (!el) return '';
    var c = (el.getAttribute && el.getAttribute('class')) || '';
    return el.tagName.toLowerCase() + (c ? '.' + c.trim().split(/\s+/).join('.') : '');
  };
  var cell = null, cells = [].slice.call(root.querySelectorAll('.cell[data-day]'));
  for (var i = 0; i < cells.length; i++) {
    if (cells[i].querySelector('.phrow') || cells[i].querySelector('.moonpick')) { cell = cells[i]; break }
  }
  if (!cell) return out;

  // ── THE SECOND TARGET, TAPPED BEFORE THE FIRST ─────────────────────────
  //
  // The day-selection half of C-CALV4-SPEC §4's "two targets, one cell,
  // cleanly separated", and it is measured FIRST because the moon tap below
  // opens a panel over the row and would change what a later hit test lands on.
  //
  // THE POINT IS CHOSEN AWAY FROM THE MOON ON PURPOSE: 30% across and 70% down,
  // which is the cell body where the date and the event marks live and where a
  // reader aiming at "this day" actually presses. A moon control that had been
  // grown until it covered the cell — the obvious way to chase the 44px touch
  // floor — resolves here and leaves dselChecks false.
  var dsel = cell.querySelector('.dsel');
  var dpick = cell.querySelector('.daypick');
  if (dsel && dpick) {
    cell.scrollIntoView({ block: 'center', inline: 'center' });
    var dcb = box(cell);
    var dp = document.elementFromPoint(dcb.l + dcb.w * 0.3, dcb.t + dcb.h * 0.7);
    out.dselHit = desc(dp);
    var mctl = cell.querySelector('.phctl');
    out.moonStole = !!(dp && mctl && (dp === mctl || mctl.contains(dp)));
    if (dp) {
      dp.click();
      out.dselChecks = !!dpick.checked;
      dpick.checked = false;
    }
  }

  // THE OPENER, AS A HIT TEST. elementFromPoint at the painted centre, then a
  // real click on whatever the browser resolved — never on a node this script
  // picked. NO HOVER IS SYNTHESISED anywhere in this probe: a touch device has
  // none, so an affordance that needs one does not exist for the operator.
  //
  // SCROLLED INTO VIEW FIRST, and that is a correctness fix rather than a
  // nicety: elementFromPoint is VIEWPORT-relative and returns null for a point
  // outside it. This page stacks twenty hosts, so all but the first few cells
  // sit thousands of pixels down — the first run of this probe reported "the
  // opener is not hit-testable" at every width, about a control that was fine.
  var ctl = cell.querySelector('.phctl');
  if (ctl) {
    ctl.scrollIntoView({ block: 'center', inline: 'center' });
    var kb = box(ctl);
    if (vis(kb)) {
      out.ctlW = r1(kb.w); out.ctlH = r1(kb.h);
      var hit = document.elementFromPoint(kb.l + kb.w / 2, kb.t + kb.h / 2);
      out.hit = desc(hit);
      out.hitOK = !!(hit && (hit === ctl || ctl.contains(hit)));

      // THE REAL TOUCH TARGET. Under a coarse pointer the label carries a
      // transparent ::before pad, so the box a finger can land on is bigger
      // than the box the discs paint. A pseudo-element has no rect to query,
      // so its USED insets are read — Chromium resolves the sheet's clamped
      // inline inset to a plain pixel value here, which is a measurement of
      // the layout and not a re-reading of the declaration. A content of
      // "none" is how the fine-pointer arm says there is no pad at all.
      var pcs = getComputedStyle(ctl, '::before');
      var hasPad = pcs.content && pcs.content !== 'none' && pcs.content !== 'normal';
      var pl = hasPad ? (parseFloat(pcs.left) || 0) : 0;
      var pr = hasPad ? (parseFloat(pcs.right) || 0) : 0;
      var pt = hasPad ? (parseFloat(pcs.top) || 0) : 0;
      var pb = hasPad ? (parseFloat(pcs.bottom) || 0) : 0;
      out.padW = r1(kb.w - pl - pr);
      out.padH = r1(kb.h - pt - pb);
      // …and whether that target reaches into the NEIGHBOURING day. The limit
      // is the cell's CONTENT box, not its border box: the 1px border-right is
      // the rule between two days and a target that crosses it puts one day's
      // moon control under a thumb aiming at the next.
      //
      // THE CELL IS RE-READ HERE. Every rect above was taken before
      // scrollIntoView, and getBoundingClientRect is VIEWPORT-relative — mixing
      // a pre-scroll cell with a post-scroll label produced a phantom 3-7px
      // overspill at every width, including cells three times wider than the
      // target. Two rects from two scroll positions are not comparable.
      var cb2 = box(cell);
      var ccs = getComputedStyle(cell);
      var ccl = cb2.l + parseFloat(ccs.borderLeftWidth) + parseFloat(ccs.paddingLeft);
      var ccr = cb2.r - parseFloat(ccs.borderRightWidth) - parseFloat(ccs.paddingRight);
      out.padSteal = r1(Math.max(0, ccl - (kb.l + pl)) + Math.max(0, (kb.r - pr) - ccr));
      // AND THE PAD IS REALLY HITTABLE, not merely declared. One point inside
      // the pad and outside the discs: a rule that computed correctly and
      // painted nothing would otherwise report a target nobody can press —
      // which is the exact failure sky_disc_paint_probe was built for.
      if (hasPad && out.padW > kb.w) {
        var pe = document.elementFromPoint(kb.l + pl + 1, kb.t + kb.h / 2);
        out.padHit = !!(pe && (pe === ctl || ctl.contains(pe)));
      }
      if (hit) {
        hit.click();
        // THE PANEL IS SEEKED TO ITS END, not waited on. --virtual-time-budget
        // advances TIMERS but not the animation clock, so a probe that read
        // opacity straight after the click measured the CLOSED value and
        // reported "the panel never opens" about a panel that opens — the same
        // trap moonpanel_probe_test.go documents having fallen into twice.
        var wkrow = cell.closest('.wk') || root;
        var pan = wkrow.querySelector('.mpan') || root.querySelector('.mpan');
        if (pan) {
          pan.getAnimations().forEach(function(a){ try { a.finish() } catch (e) {} });
          var pbx = box(pan);
          out.opensPanel = vis(pbx) && parseFloat(getComputedStyle(pan).opacity) > 0.5;
        }
        // put it back, so the next case in the same page is not measured open
        var checked = root.querySelector('.moonpick:checked');
        if (checked) checked.checked = false;
      }
    }
  }
  return out;
}`

var mrLinkRe = regexp.MustCompile(`<link[^>]*>`)

// mrCoarsePointerFlags makes Chromium report `(pointer: coarse)`.
//
// THE PACKAGE BELIEVED THIS WAS IMPOSSIBLE. moonpanel_probe_test.go's
// TestMoonPanelCSS_CoarsePointerHasNoHoverAndABiggerTarget says in its own doc
// comment that it is "browser-free because Chromium has no CLI switch for a
// coarse pointer and a probe that cannot emulate the condition cannot assert
// it", and so it greps the stylesheet instead. There is a switch: Blink's
// pointer-type settings are settable from the command line, and
// `matchMedia('(pointer: coarse)').matches` reads true under it — verified
// against chromium-1194 before this was relied on. That turns MN-G12's touch
// half from a CSS-source claim into a measured one, which is the difference
// between "the rule is written" and "the finger lands".
var mrCoarsePointerFlags = []string{
	"--blink-settings=primaryPointerType=2,availablePointerTypes=2",
}

// mrRun lays every case out in one page — each in its own host box, at the host
// width the shell chain gives it — and reads them all back in one Chromium run.
func mrRun(t *testing.T, chrome string, cases []mrCase, extraFlags ...string) []mrReading {
	t.Helper()
	css := blockCSS(t)

	var boxes strings.Builder
	for i, c := range cases {
		// EVERY HOST GETS ITS OWN CALENDAR SLUG, and that is a correctness fix
		// rather than tidiness. Every id and every radio-group NAME in the Block
		// is `<prefix>-<slug>-<hostEntity>` (helpers.go:2461), so twenty copies
		// of one fixture on one page share one radio group: `label[for]` resolves
		// to the FIRST match in the DOCUMENT and only one radio in the whole page
		// can be checked. The first run of this probe reported "the panel never
		// opens" at every width because of it — every click was landing on host
		// zero's radio. The slug is not rendered anywhere a measurement reads.
		d := c.data
		d.CalendarSlug = fmt.Sprintf("%s-h%d", d.CalendarSlug, i)
		// The <link> cannot resolve over file://; the sheet is inlined instead,
		// which also makes this a test of THIS sheet and not of a build artefact.
		markup := mrLinkRe.ReplaceAllString(render(t, d), "")
		fmt.Fprintf(&boxes, `<div class="probe-host" id="h%d" style="width:%dpx">%s</div>`,
			i, mrHostWidth(c.viewport, c.sidebar), markup)
	}

	// `pointerIsCoarse` rides back with the readings so a coarse arm that
	// silently failed to take effect cannot be mistaken for a fine one that
	// happened to measure the same numbers.
	page := `<!doctype html><html><head><meta charset="utf-8"><style>` +
		`html,body{margin:0;background:#fff}` +
		`.probe-host{display:block;margin:24px}` +
		css + `</style></head><body>` + boxes.String() +
		// TWO PASSES OVER THE WHOLE PAGE, AND THE ORDER IS LOAD-BEARING.
		// Chromium's :focus-visible heuristic is document-wide state: once any
		// host has been click()ed, programmatic focus stops matching it
		// everywhere. So every host's geometry — including the focus-driven
		// expansion — is measured first, and only then is anything clicked.
		`<script>document.addEventListener('DOMContentLoaded',function(){` +
		`var geom=` + moonReachScript + `;` +
		`var open=` + moonReachOpenScript + `;` +
		`var hosts=[].slice.call(document.querySelectorAll('.probe-host'));` +
		`var out=hosts.map(geom);` +
		`hosts.forEach(function(h,i){open(h,out[i])});` +
		`out.forEach(function(o){o.coarse=matchMedia('(pointer: coarse)').matches});` +
		`document.body.setAttribute('data-probe', JSON.stringify(out));});</script>` +
		`</body></html>`

	path := filepath.Join(t.TempDir(), "moonreach.html")
	if err := os.WriteFile(path, []byte(page), 0o644); err != nil { //nolint:gosec // test artefact
		t.Fatalf("write probe page: %v", err)
	}

	args := append([]string{
		"--headless", "--no-sandbox", "--disable-gpu", "--hide-scrollbars",
		"--window-size=2200,1400", "--virtual-time-budget=6000",
	}, extraFlags...)
	args = append(args, "--dump-dom", "file://"+path)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, chrome, args...).Output()
	if err != nil {
		t.Fatalf("chromium: %v", err)
	}
	m := probePayloadRe.FindStringSubmatch(string(out))
	if m == nil {
		t.Fatal("no probe payload in the rendered DOM — the page script did not run")
	}
	var readings []mrReading
	if err := json.Unmarshal([]byte(html.UnescapeString(m[1])), &readings); err != nil {
		t.Fatalf("probe payload: %v", err)
	}
	if len(readings) != len(cases) {
		t.Fatalf("probe returned %d readings for %d cases", len(readings), len(cases))
	}
	return readings
}

// mrCases is the census: three phones, two tablets, three desktops, each for a
// seven-day week and for the ten-day week real in-world calendars have — plus
// the unpinned-sidebar arm at the two desktop widths where it changes the
// answer.
func mrCases(t *testing.T) []mrCase {
	t.Helper()
	ten := fxAlmanac(t, true) // Harptos: ten-day week, three drawn moons, register present
	seven := fxMoonSevenDay(t)

	var out []mrCase
	for _, vp := range []int{360, 390, 430, 768, 1024, 1280, 1440, 1920} {
		out = append(out,
			mrCase{vp, mrSidebarPx, 7, "week 7", seven},
			mrCase{vp, mrSidebarPx, 10, "week 10", ten},
		)
	}
	// the auto-collapsed sidebar: 208px more host, at the two widths where the
	// 1180px cap does not already swallow the difference.
	for _, vp := range []int{1280, 1440} {
		out = append(out,
			mrCase{vp, mrSidebarCollapsedPx, 7, "week 7 · sidebar collapsed", seven},
			mrCase{vp, mrSidebarCollapsedPx, 10, "week 10 · sidebar collapsed", ten},
		)
	}
	return out
}

// fxMoonSevenDay is the Gregorian fixture with moons actually on it. The
// shipped `fxGregorian` enables the moons LAYER and carries no moon data — it
// is the real-world calendar the report calls moonless — so a seven-day census
// row built on it as-is would measure the absence rather than the layout.
func fxMoonSevenDay(t *testing.T) BlockData {
	t.Helper()
	d := fxGregorian()
	d.Layers = LayerState{Enabled: []string{"moons", "ledger", "shelf"}}
	for ri := range d.Month.Rows {
		for ci := range d.Month.Rows[ri].Cells {
			c := &d.Month.Rows[ri].Cells[ci]
			if c.Day > 0 {
				c.Moons = fxMoonDiscs(c.Day)
			}
		}
	}
	d.Month.MoonsDeclared = len(fxMoons)
	for _, m := range fxMoons {
		am := AlmanacMoon{Name: m.name, PeriodDays: m.period, Drawn: true}
		for day := 1; day <= d.Month.Days; day++ {
			am.Days = append(am.Days, AlmanacDay{
				Day:   day,
				Illum: fxPhase(m.period, m.newAt, day),
				Phase: "waxing gibbous",
			})
		}
		d.Month.Almanac = append(d.Month.Almanac, am)
	}
	return d
}

// TestMoonReachProbe_TheDiscsAreOnTheScreenAtAPhoneWidth is the guard.
//
// THE CLAIM IS THE OPERATOR'S OWN SENTENCE, measured: at the width of a phone,
// with moons in the data, are there moons on the screen?
//
// PROVEN RED. Reverting the two lines of the fix (the `@container cal-cell
// (min-width: 40px)` block in calendar-block.css, and moving `@moonRow` back
// inside `.cnamed` in instrument.templ) fails this test in 14 of its 20
// subtests — including all three phone widths for a seven-day calendar and
// every ten-day week at every viewport up to and including 1920px. Restoring
// them leaves 17 of 20 painting discs and the other three logging the
// deliberate degradation. Both runs are in the stage report.
//
// It asserts the DEGRADATION as well as the promotion, in the same loop and
// off the same readings, so "delete the query" cannot pass it: below the disc
// row's threshold the row must be GONE, and above it, present, three-strong,
// clear of the day numeral's ink and inside its own cell.
func TestMoonReachProbe_TheDiscsAreOnTheScreenAtAPhoneWidth(t *testing.T) {
	if testing.Short() {
		t.Skip("browser probe: skipped under -short (CI's mode) — a skipped run is NOT a pass")
	}
	chrome := findProbeChromium()
	if chrome == "" {
		t.Skip("browser probe: no Chromium binary found (set CHROMIUM_BIN) — a skipped probe is NOT a pass")
	}
	cases := mrCases(t)
	readings := mrRun(t, chrome, cases)

	t.Log("─── THE CENSUS ──────────────────────────────────────────────────────────────")
	t.Log("rest = discs painted with nothing pointed at or focused, across EVERY dated")
	t.Log("cell in the month.  exp = the same row with its radio :focus-visible, on the")
	t.Log("cell carrying the month's WIDEST date ink.  ink = that widest date, in px.")
	t.Logf("%-9s %-28s %-6s %-8s %-10s %-6s %-5s %-6s %-11s %-6s",
		"viewport", "arm", "host", "column", "density", "rest", "ink", "rest+",
		"exp (discs/+)", "opens")
	for i, c := range cases {
		r := readings[i]
		density := "underline"
		if r.NamedShown {
			density = "NAMED"
		}
		rest := fmt.Sprintf("%d", r.RestDiscsMax)
		if r.RestDiscsMin != r.RestDiscsMax {
			rest = fmt.Sprintf("%d–%d", r.RestDiscsMin, r.RestDiscsMax)
		}
		t.Logf("%-9s %-28s %-6.0f %-8.1f %-10s %-6s %-5.1f %-6d %-11s %-6v",
			fmt.Sprintf("%dpx", c.viewport), c.label, r.Host, r.ColumnW, density,
			rest, r.WorstNumW, r.RestPlus,
			fmt.Sprintf("%d/%v w=%.1f", r.ExpDiscs, r.ExpPlus, r.ExpRowW), r.OpensPanel)
	}
	t.Log("─────────────────────────────────────────────────────────────────────────────")

	for i, c := range cases {
		r := readings[i]
		name := fmt.Sprintf("%dpx-%s", c.viewport, strings.ReplaceAll(c.label, " ", "-"))
		t.Run(name, func(t *testing.T) {
			// Every case must have measured a real cell first (MN-G8).
			if r.CellW <= 0 || r.CellH <= 0 {
				t.Fatalf("no day cell laid out at host %.0fpx — nothing below is about anything",
					r.Host)
			}

			// ── THE PRIMARY SILHOUETTE IS ON THE SCREEN. ALWAYS. ────────────
			//
			// NO THRESHOLD APPEARS IN THIS ASSERTION, and that is the whole
			// point of it. The version this replaces said `wantRow := r.ColumnW
			// >= MoonRowColWidthMin` and then checked the CSS against it, which
			// is a tautology dressed as a measurement: it asks "does the
			// stylesheet agree with the Go constant", so moving the constant
			// moves the test with it and the operator's ten-day phone stayed
			// invisible through twenty green runs.
			//
			// C-CALV4-SPEC §4 makes the claim unconditional — "The primary moon
			// is ALWAYS silhouetted in the cell — visible at rest, not
			// hover-gated" — so the assertion is unconditional too. Every case,
			// every viewport, both week lengths.
			if !r.RowShown {
				t.Fatalf("host %.0fpx · column %.1fpx (%s): NO MOON ROW IS PAINTED. The primary "+
					"silhouette is always visible by spec — there is no width at which this "+
					"cell is allowed to show nothing. This is the shipped defect: a single "+
					"40px query gated all three discs together, and a ten-day week measures "+
					"30.0px on a phone", r.Host, r.ColumnW, c.label)
			}
			if r.Cells == 0 {
				t.Fatalf("host %.0fpx: the month walk found no dated cell carrying a moon row — "+
					"the census below would be vacuous", r.Host)
			}
			if r.RestDiscsMin != 1 || r.RestDiscsMax != 1 {
				t.Errorf("host %.0fpx · column %.1fpx: cells paint %d–%d discs AT REST across "+
					"%d dated cells; the spec's resting state is EXACTLY ONE — the primary "+
					"silhouette. Three at rest is what this slice replaced (it did not fit a "+
					"ten-day column); zero is the defect it fixed",
					r.Host, r.ColumnW, r.RestDiscsMin, r.RestDiscsMax, r.Cells)
			}
			if r.DiscW <= 0 || r.DiscH <= 0 {
				t.Errorf("host %.0fpx: a disc lays out at %.1f×%.1f. sky_disc_paint_probe "+
					"taught this package that `display:none`'s cousin here is a 0×0 inline box, "+
					"and a census that counted those would be the same silent pass again",
					r.Host, r.DiscW, r.DiscH)
			}

			// ── MN-G4 AT REST: no cell says anything about the ceiling ──────
			//
			// The `+` is narrowed by C-CALV4-SPEC §4, not deleted, and this is
			// the half that survives verbatim: MN-G4's argument was that "a
			// marker repeated in every cell was the noisiest thing on the
			// surface", and that is an argument about the RESTING month. It is
			// measured PAINTED here rather than grepped out of the markup,
			// because the `+` is now in the DOM at every width and only the
			// cascade keeps it out of sight.
			if r.RestPlus != 0 {
				t.Errorf("host %.0fpx: %d of %d cells paint a `+` AT REST. MN-G4 holds where "+
					"its own argument applies — thirty ceiling markers at once is the noise it "+
					"was written against. The `+` belongs to the hovered/focused cell alone",
					r.Host, r.RestPlus, r.Cells)
			}

			// ── and nothing is sitting on the date, in ANY cell ─────────────
			//
			// WALKED OVER THE WHOLE MONTH, not over the first cell that carries
			// moons. The silhouette rests at the cell's right and the date at
			// its left, so the collision depends on the date's INK WIDTH — "3"
			// is half of "30". Measuring one cell would clear a month whose
			// every two-digit day collides.
			if r.WorstOverlap > 0 {
				t.Errorf("host %.0fpx · column %.1fpx: the resting silhouette overlaps the date's "+
					"INK by %.0fpx² on day %q (widest date in the month: %.1fpx). A moon over the "+
					"date is not a denser cell, it is an unreadable one — and it is the two-digit "+
					"days that collide first, which is why this walks all %d of them",
					r.Host, r.ColumnW, r.WorstOverlap, r.WorstDay, r.WorstNumW, r.Cells)
			}
			if r.WorstSpill > 0.5 {
				t.Errorf("host %.0fpx · column %.1fpx: the silhouette hangs %.1fpx outside its "+
					"own cell. A mark that overflows its day is worse than one that degrades",
					r.Host, r.ColumnW, r.WorstSpill)
			}

			// ── THE EXPANSION, AND ITS OWN THRESHOLD ────────────────────────
			//
			// This one IS expressed against a constant, and legitimately so:
			// unlike the silhouette there IS a width at which the expansion
			// must not happen, so there is a real boundary to assert in both
			// directions. What keeps it from being the old tautology is that
			// the silhouette assertion above holds unconditionally — a future
			// hand who raises MoonExpandColWidthMin to 500 still cannot make
			// the operator's phone show nothing.
			if !r.ExpFocusOK {
				t.Fatalf("host %.0fpx: focusing the cluster's radio did not match "+
					":focus-visible, so the expansion below was never actually triggered and "+
					"every number in it is meaningless", r.Host)
			}
			// THE `+` IS CONDITIONAL ON THE DATA, so the expectation is read
			// from the fixture rather than assumed. The ten-day Harptos fixture
			// declares four moons against a grid ceiling of three and MUST show
			// one; the seven-day fixture declares exactly three and must NOT —
			// a `+` there would be a ceiling marker about a ceiling nothing has
			// hit. Asserting `+` unconditionally reddened every seven-day row in
			// this census about correct behaviour.
			wantExpand := r.ColumnW >= MoonExpandColWidthMin
			wantPlus := c.data.Month.MoonsDeclared > moonCap
			if wantExpand {
				if r.ExpDiscs != moonCap {
					t.Errorf("host %.0fpx · column %.1fpx: the expansion paints %d discs, want "+
						"%d. [MN-1] is signed at three and C-CALV4-SPEC §4 keeps it there — "+
						"\"expands to show up to three\"",
						r.Host, r.ColumnW, r.ExpDiscs, moonCap)
				}
				if r.ExpPlus != wantPlus {
					t.Errorf("host %.0fpx · column %.1fpx: the expansion paints `+`=%v, want %v. "+
						"This fixture declares %d moons against a grid ceiling of %d. "+
						"C-CALV4-SPEC §4: \"with a `+` if the calendar has more\" — and only "+
						"if it has more",
						r.Host, r.ColumnW, r.ExpPlus, wantPlus,
						c.data.Month.MoonsDeclared, moonCap)
				}
				if r.ExpOverlap > 0 {
					t.Errorf("host %.0fpx · column %.1fpx: the EXPANDED row overlaps the date's "+
						"ink by %.0fpx² (row %.1fpx wide). The expansion threshold exists "+
						"precisely so this cannot happen — %.0fpx is too low",
						r.Host, r.ColumnW, r.ExpOverlap, r.ExpRowW, MoonExpandColWidthMin)
				}
				if r.ExpSpill > 0.5 {
					t.Errorf("host %.0fpx · column %.1fpx: the EXPANDED row hangs %.1fpx outside "+
						"its cell (row %.1fpx wide). %.0fpx is too low a threshold",
						r.Host, r.ColumnW, r.ExpSpill, r.ExpRowW, MoonExpandColWidthMin)
				}
			} else {
				if r.ExpDiscs != 1 {
					t.Errorf("host %.0fpx · column %.1fpx: the cluster expanded to %d discs in a "+
						"column UNDER the %.0fpx expansion threshold. Below it the silhouette "+
						"stays alone and the moon SECTION carries the detail — expanding here "+
						"puts three discs and a `+` across the date",
						r.Host, r.ColumnW, r.ExpDiscs, MoonExpandColWidthMin)
				}
				if r.ExpPlus {
					t.Errorf("host %.0fpx · column %.1fpx: a `+` painted below the expansion "+
						"threshold", r.Host, r.ColumnW)
				}
			}

			// ── the opener works, without a hover ────────────────────────────
			// A `.phctl` only exists where the panel is reachable; where it is
			// not, moonRow deliberately emits a decorative row instead, and
			// there is nothing to open.
			if r.CtlW > 0 {
				if !r.HitOK {
					t.Errorf("host %.0fpx: a pointer at the opener's painted centre resolved to "+
						"%q, which is not the opener. A control the finger cannot land on is not "+
						"a control", r.Host, r.Hit)
				}
				if !r.OpensPanel {
					t.Errorf("host %.0fpx: one hit-tested click did not open the panel. The probe "+
						"synthesises NO hover anywhere — if the panel needs one, it does not exist "+
						"on a touch device", r.Host)
				}
			}
		})
	}

	// ── THE DEGRADATION IS STILL THERE ──────────────────────────────────────
	//
	// Deleting the query would pass every assertion above and be wrong: the
	// query exists so a cramped cell degrades to the underline instead of
	// overflowing. Both halves are asserted here against the SAME readings.
	var sawNamed, sawUnder bool
	for i, c := range cases {
		r := readings[i]
		if r.NamedShown && r.UnderShown {
			t.Errorf("%dpx %s: BOTH cell subtrees are laid out (column %.1fpx). Exactly one "+
				"shows; two is the overflow the density query exists to prevent",
				c.viewport, c.label, r.ColumnW)
		}
		if r.NamedShown {
			sawNamed = true
		}
		if r.UnderShown {
			sawUnder = true
		}
		// The named-event row keeps ITS OWN threshold, and it is the one the
		// contract signs (NamedColWidthMin). The discs' threshold is separate
		// and lower; that separation is the fix.
		if got, want := r.NamedShown, r.ColumnW >= NamedColWidthMin; got != want {
			t.Errorf("%dpx %s: named=%v at a measured %.1fpx column; the contract's threshold "+
				"is %.0fpx", c.viewport, c.label, got, r.ColumnW, NamedColWidthMin)
		}
	}
	if !sawUnder {
		t.Error("no case in the census fell back to the underline — the census cannot show " +
			"that the degradation still works, so it cannot show that the query was not " +
			"simply deleted")
	}
	if !sawNamed {
		t.Error("no case in the census reached NAMED density — the census has lost its " +
			"upper half and would not notice the named row disappearing")
	}

	// ── THE OPERATOR'S OWN SENTENCE, ASSERTED AS ITSELF ─────────────────────
	//
	// Everything above is expressed in thresholds and columns, and a threshold
	// can be moved in good faith by someone who never sees this file's point.
	// This last loop states the case that started the work in the words it was
	// reported in: a phone, a calendar with moons in the data — is there a moon
	// on the screen? If a future change makes that false again it reddens here,
	// whatever the arithmetic says.
	//
	// ── AND IT NOW COVERS THE TEN-DAY WEEK, WHICH IS THE REAL DEFECT ────────
	//
	// THE GAP THIS CLOSES, precisely. The census has carried ten-day rows since
	// it was written, so "the probe only tests a seven-day week" is not quite
	// what was wrong. What was wrong is subtler and worse: every ten-day
	// assertion was expressed as `wantRow := r.ColumnW >= MoonRowColWidthMin`,
	// which asks whether the STYLESHEET AGREES WITH THE GO CONSTANT — a
	// tautology that is satisfied just as well by "the constant is 40, the
	// column is 33, so painting nothing is correct". And the ONE assertion in
	// the file that was not a tautology — this loop, the operator's own
	// sentence — filtered `c.weekLen != 7` and threw the ten-day rows away.
	//
	// So the ten-day case was measured, logged, and never judged. Twenty green
	// runs reported "0 discs" for the operator's own calendar at every phone
	// width, in a passing test, in a table nobody had a reason to read. The
	// operator's calendar is Harptos: TEN days to the week.
	//
	// Both week lengths are now asserted by the same unconditional claim, and
	// the ten-day arm is the one that fails without this slice's CSS.
	phones := map[int]bool{360: true, 390: true, 430: true}
	weeks := map[int]bool{7: true, 10: true}
	checked := map[int]int{}
	for i, c := range cases {
		if !phones[c.viewport] || !weeks[c.weekLen] {
			continue
		}
		checked[c.weekLen]++
		r := readings[i]
		if !r.RowShown || r.RestDiscsMin != 1 || r.RestDiscsMax != 1 {
			t.Errorf("A %dpx PHONE, a %d-DAY WEEK, moons in the data: %d–%d discs painted at "+
				"rest across %d cells (column %.1fpx). This is the observation the whole fix "+
				"answers — \"I don't see the real moon on the real calendar\". A ten-day week "+
				"is the operator's OWN calendar (Harptos) and its column is %.1fpx here",
				c.viewport, c.weekLen, r.RestDiscsMin, r.RestDiscsMax, r.Cells, r.ColumnW,
				r.ColumnW)
		}
	}
	for wl := range weeks {
		if checked[wl] != len(phones) {
			t.Errorf("the census covered %d of the %d phone widths for a %d-day week — the "+
				"operator's own case has fallen out of the table", checked[wl], len(phones), wl)
		}
	}
}

// TestMoonReachProbe_TheOpenerOnATouchDevice drives the same census a SECOND
// time with Chromium reporting a coarse pointer, and measures the opener the
// way a thumb meets it.
//
// THREE THINGS ARE ASSERTED AND ONE IS REPORTED.
//
// Asserted — NO HOVER IS SYNTHESISED ANYWHERE IN THIS PROBE, in either arm. A
// touch device has no hover, so every panel that opens here opened from a plain
// hit-tested click. If the opener ever grows a hover-dependent step this arm
// stops opening the panel and reddens.
//
// Asserted — THE TOUCH TARGET MAY NOT REACH INTO THE NEXT DAY. The coarse pad
// is `inset-block: -7px`, which is 14px of extra inline reach on a control
// whose narrowest cell is now 43.3px wide. Unclamped, a thumb aimed at the edge
// of one day would land on its neighbour's moon control. The sheet clamps it
// with `max(-7px, (100% - 100cqi) / 2)` and this walks the real hit area with
// elementFromPoint to check the clamp holds.
//
// Asserted — the coarse arm must actually BE coarse. Read back from the page,
// so a flag that stopped working cannot quietly turn this into a second fine
// run that agrees with the first.
//
// REPORTED, NOT FAILED — THE 44px FLOOR ([MOB-7],
// internal/plugins/calendar/daycard_floors_probe_test.go:99). The opener does
// not meet it and cannot be made to inside this cell: an underline day cell is
// 52px tall, so a 44px target would take 85% of its block axis and 43 of its
// 43.3px inline axis — it would BE the cell, and it would take that area out of
// `.dsel`, the day-selection label, whose own target [MOB-7] already records as
// the tightest thing on the page. The number is printed at every width so the
// ceiling is a visible measured fact rather than a silent one. Raising it needs
// a drawing and a signature, not a CSS tweak.
func TestMoonReachProbe_TheOpenerOnATouchDevice(t *testing.T) {
	if testing.Short() {
		t.Skip("browser probe: skipped under -short (CI's mode) — a skipped run is NOT a pass")
	}
	chrome := findProbeChromium()
	if chrome == "" {
		t.Skip("browser probe: no Chromium binary found (set CHROMIUM_BIN) — a skipped probe is NOT a pass")
	}
	cases := mrCases(t)
	fine := mrRun(t, chrome, cases)
	coarse := mrRun(t, chrome, cases, mrCoarsePointerFlags...)

	const floor = 44.0

	// The arm has to be the arm it says it is, or nothing below means anything.
	if len(coarse) == 0 || !coarse[0].Coarse {
		t.Fatalf("the coarse arm did not report `(pointer: coarse)` — %v no longer makes "+
			"Chromium emulate a touch pointer, so this whole test would be a duplicate of "+
			"the fine one wearing a different name", mrCoarsePointerFlags)
	}
	if fine[0].Coarse {
		t.Fatal("the FINE arm reports a coarse pointer — the two arms are not separated")
	}

	t.Logf("%-9s %-28s %-11s %-12s %-14s %-9s %s",
		"viewport", "arm", "cell", "discs", "touch target", "vs 44px", "into next day")
	worstBlock, worstInline := math.Inf(1), math.Inf(1)
	for i, c := range cases {
		r := coarse[i]
		if r.CtlW <= 0 {
			t.Logf("%-9s %-28s %-11s %s", fmt.Sprintf("%dpx", c.viewport), c.label,
				fmt.Sprintf("%.1f×%.1f", r.CellW, r.CellH),
				"no opener in this arm — not asserted")
			continue
		}
		verdict := "OK"
		if r.PadH < floor {
			verdict = fmt.Sprintf("-%.0f", floor-r.PadH)
		}
		worstBlock = math.Min(worstBlock, r.PadH)
		worstInline = math.Min(worstInline, r.PadW)
		t.Logf("%-9s %-28s %-11s %-12s %-14s %-9s %.1fpx",
			fmt.Sprintf("%dpx", c.viewport), c.label,
			fmt.Sprintf("%.1f×%.1f", r.CellW, r.CellH),
			fmt.Sprintf("%.1f×%.1f", r.CtlW, r.CtlH),
			fmt.Sprintf("%.1f×%.1f", r.PadW, r.PadH), verdict, r.PadSteal)

		// ── the pad may not cross further than it always has ────────────────
		//
		// mrPadOverspillMax is NOT a tolerance chosen to make this pass. It is
		// the shipped construction's own arithmetic: the disc row is anchored
		// 4px in from the cell's right edge and MN-G12's pad is 7px, so a wide
		// cell's target has ended 3.0px past the rule since the pad landed —
		// measured here at 1280px/week 7 and unchanged by this stage. What the
		// clamp removes is the case this stage would otherwise have CREATED:
		// promoting the row into a 43.3px cell puts a 49px pad around it, wider
		// than the day it belongs to. Anything above 3.0 is that regression.
		if r.PadSteal > mrPadOverspillMax {
			t.Errorf("%dpx %s: the touch target reaches %.1fpx past its own cell, over the "+
				"%.1fpx the shipped 4px-inset-plus-7px-pad already produces (target %.1f×%.1f "+
				"in a %.1fpx column). Two adjacent days' moon controls overlapping means a "+
				"thumb near a cell edge opens the panel marked on the wrong date",
				c.viewport, c.label, r.PadSteal, mrPadOverspillMax, r.PadW, r.PadH, r.ColumnW)
		}
		// ── the pad is real, not merely declared ────────────────────────────
		if r.PadW > r.CtlW && !r.PadHit {
			t.Errorf("%dpx %s: the coarse pad computes to %.1f×%.1f but a point inside it does "+
				"not resolve to the control. A hit area that exists only in the cascade is the "+
				"0×0-disc failure in another costume", c.viewport, c.label, r.PadW, r.PadH)
		}
		// ── and a coarse pointer must never need a hover to get in ──────────
		if !r.OpensPanel {
			t.Errorf("%dpx %s: on a COARSE pointer one hit-tested click did not open the "+
				"panel. No hover is synthesised anywhere in this probe, because a touch "+
				"device has none — a control that needs one does not exist for the operator "+
				"(MN-G12)", c.viewport, c.label)
		}
		// The coarse pad may only ever ADD. A regression that removed it would
		// otherwise look like a clean run with smaller numbers.
		if f := fine[i]; f.CtlW > 0 && (r.PadW < f.CtlW || r.PadH < f.CtlH) {
			t.Errorf("%dpx %s: the coarse touch target (%.1f×%.1f) is SMALLER than the fine "+
				"pointer's painted box (%.1f×%.1f) — the pad has gone backwards",
				c.viewport, c.label, r.PadW, r.PadH, f.CtlW, f.CtlH)
		}
	}

	// ── THE SECOND TARGET, BY THUMB ─────────────────────────────────────────
	//
	// C-CALV4-SPEC §4, the operator's own answer 2: "Clicking the hovered
	// silhouette opens the moon section; clicking anywhere else in the cell
	// opens the Ledger. Two targets, one cell, cleanly separated."
	//
	// Everything above proves the FIRST target works by tap. This proves the
	// second one still does, and that is not a formality — the obvious way to
	// chase the unmet 44px floor is to grow the moon pad until it covers the
	// cell, and that would silently turn every tap on a day into a tap on its
	// moon. Asserted on the COARSE arm because that is where it would bite.
	tapped := 0
	for i, c := range cases {
		r := coarse[i]
		if r.DselHit == "" {
			continue // this Block has no docked Ledger, so it emits no day pick
		}
		tapped++
		if r.MoonStole {
			t.Errorf("%dpx %s: a tap in the CELL BODY resolved to the moon control (%q). The "+
				"two targets have stopped being separate — every tap meant for a day now "+
				"opens that day's moon panel instead", c.viewport, c.label, r.DselHit)
		}
		if !r.DselChecks {
			t.Errorf("%dpx %s: a tap in the cell body resolved to %q and did NOT select the "+
				"day. The Ledger answers the day pick, so a cell body that does not select "+
				"is a cell whose Ledger cannot be opened by thumb",
				c.viewport, c.label, r.DselHit)
		}
	}
	if tapped == 0 {
		t.Error("no case in the census carried a day-selection target, so the second half of " +
			"the two-target rule was never exercised — the guard would pass vacuously")
	}
	if math.IsInf(worstBlock, 1) {
		t.Fatal("no opener was rendered in any arm — the census measured nothing")
	}
	t.Logf("THE FLOOR, STATED PLAINLY: the smallest touch target is %.1f×%.1fpx against a "+
		"%.0fpx floor. It is not met and it cannot be met here — an underline day cell is "+
		"52.0px tall and 43.3px wide at the narrowest width that now draws discs, so a 44px "+
		"target would be the cell, and would take that area from the day-selection label "+
		"beneath it. BOOKED AND VISIBLE, never silently accepted.",
		worstInline, worstBlock, floor)

	// The one thing that IS a failure here: an opener with no painted box.
	for i, c := range cases {
		if r := coarse[i]; r.CtlW > 0 && r.CtlH <= 0 {
			t.Errorf("%dpx %s: the opener paints %.1f×%.1f — a zero block axis is not a small "+
				"target, it is no target", c.viewport, c.label, r.CtlW, r.CtlH)
		}
	}
}
