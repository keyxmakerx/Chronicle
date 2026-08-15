// cell_probe_test.go — DOES THE DAY CELL ACTUALLY LOOK LIKE C-CALV4-SPEC §1?
//
// WHY THIS FILE EXISTS, AND WHAT IT REPLACES. The day cell's first slice shipped
// with three defects that every green test in this package was structurally
// incapable of seeing, and one of them was hidden by a test written IN THE SAME
// COMMIT that diagnosed the pattern:
//
//	cell_corners_test.go proved the event-mark corner by asserting that the
//	stylesheet contains the string `bottom: calc(var(--avail-h)`, and proved the
//	availability reservation by counting `<span class="avail">` in the markup we
//	had just emitted. Both passed. Meanwhile the event-mark strip was 0.00px
//	wide on every cell in the month — `right: auto` with only a
//	`max-inline-size`, over `flex: 1` children with empty content — so the
//	spec's THIRD CORNER painted nothing at all, at every width, on the only
//	calendar the operator uses. The commit message called `wantRow :=
//	r.ColumnW >= MoonRowColWidthMin` "a tautology dressed as a measurement" and
//	was right; the replacement was the same shape and hid a live regression on
//	its first outing.
//
// So the three markup-and-cascade claims are gone and these are measurements. A
// corner is proved by a RECT WITH AREA and by a PAINTED PIXEL; a collision is
// proved by an INTERSECTION and by elementFromPoint; the era tint is proved by
// READING THE COLOUR BACK OUT OF THE LAYOUT and comparing it with the ground it
// is supposed to sit above — in both themes, because "lighter than the page"
// inverts and a single-arm assertion would have missed that the tint pushed one
// way on dark and the other on light.
//
// WHAT REMAINS A STRING ASSERTION, STATED PLAINLY. The hover wash is composited
// under a pointer, and nothing in this repo can drive one — headless Chromium's
// :hover is a hit-test state, not an event. So the hover arm reads the browser's
// own PARSED rule out of the CSSOM (not the file), resolves its colour through a
// live element, and composites it over the fill it was measured on. That proves
// the wash is a layer rather than a replacement and prints the resulting pixel;
// it does not prove a mouse ever produced it.
//
// IT SKIPS HONESTLY under -short or with no Chromium, and a skipped run is NOT a
// pass. Both probes are registered in tools/check-browser-probes.sh, so on a
// machine that CAN run them, not running them is an error.
//
//	go test ./internal/widgets/calendar_block/ -run CellProbe -v
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

// ── what one host reports ───────────────────────────────────────────────────

// cpSwatch is a colour read back out of the layout: the string the browser
// computed, and the sRGB it actually paints. Every colour in this file goes
// through a canvas rather than through a parser of our own, because the sheet
// speaks oklch, color-mix and relative colour syntax and a hand-rolled reader
// would be a second implementation to be wrong in.
type cpSwatch struct {
	CSS string  `json:"css"`
	R   float64 `json:"r"`
	G   float64 `json:"g"`
	B   float64 `json:"b"`
	Lum float64 `json:"lum"`
}

// cpEra is one era's tint, grouped by the --erahue the producer stamped.
type cpEra struct {
	Hue   string   `json:"hue"`
	Cells int      `json:"cells"`
	Fill  cpSwatch `json:"fill"`
}

type cpReading struct {
	Label   string  `json:"label"`
	Theme   string  `json:"theme"`
	Host    float64 `json:"host"`
	ColumnW float64 `json:"columnW"`

	// the ground the cells are supposed to be raised above, and the day
	// surface that carries no era at all (the intercalary festival row).
	Ground   cpSwatch `json:"ground"`
	NoEra    cpSwatch `json:"noEra"`
	HasNoEra bool     `json:"hasNoEra"`
	Eras     []cpEra  `json:"eras"`

	// ── THE THIRD CORNER ──────────────────────────────────────────────────
	// Cells carrying marks, the narrowest strip found, the narrowest SEGMENT
	// found, and whether every segment is really inked. A strip with a box and
	// no paint is the failure sky_disc_paint_probe was built for.
	//
	// UnderShown SAYS WHICH MARK THE THIRD CORNER IS CURRENTLY DRAWING. Above
	// 84px of column the cell swaps `.cunder` for `.cnamed` and the corner is
	// held by named chips instead of the strip; the strip is then inside a
	// `display: none` subtree and measuring it would report the swap as a
	// regression. Each density is asserted on the mark it actually draws.
	UnderShown   bool    `json:"underShown"`
	NamedShown   bool    `json:"namedShown"`
	ChipCells    int     `json:"chipCells"`
	MarkCells    int     `json:"markCells"`
	UlMinW       float64 `json:"ulMinW"`
	UlMaxW       float64 `json:"ulMaxW"`
	SegMinW      float64 `json:"segMinW"`
	SegsMeasured int     `json:"segsMeasured"`
	SegsInked    int     `json:"segsInked"`
	UlSpill      float64 `json:"ulSpill"`     // how far the strip leaves its own cell
	UlClearance  float64 `json:"ulClearance"` // gap to the reserved strip below it

	// ── THE RESERVED FLOOR ────────────────────────────────────────────────
	AvailCells  int     `json:"availCells"`
	DatedCells  int     `json:"datedCells"`
	AvailH      float64 `json:"availH"`
	AvailBottom float64 `json:"availBottom"` // px between the slot and the cell's bottom
	AvailWidth  float64 `json:"availWidth"`  // slot width as a fraction of the cell's
	AvailInked  int     `json:"availInked"`  // must be 0: it is a reservation

	// ── THE FOURTH CORNER, AND THE SILHOUETTE IT USED TO EAT ──────────────
	DogearCells  int     `json:"dogearCells"`
	DogearOnRow  float64 `json:"dogearOnRow"`  // worst dogear × moon-row intersection, px²
	DogearOnUl   float64 `json:"dogearOnUl"`   // worst dogear × event-mark intersection, px²
	DogearOnAud  float64 `json:"dogearOnAud"`  // worst dogear × audience-diamond intersection
	AudPaired    int     `json:"audPaired"`    // fold cells the DATA also gave a diamond
	AudTested    int     `json:"audTested"`    // fold cells a diamond was measured on
	DiscHits     int     `json:"discHits"`     // dm_only cells whose disc resolves to the control
	DiscHitFirst string  `json:"discHitFirst"` // …and what the first one resolved to

	// ── THE HOVER WASH, AS A LAYER ────────────────────────────────────────
	HoverDeclaresColour bool     `json:"hoverDeclaresColour"`
	HoverRule           string   `json:"hoverRule"`
	Hovered             cpSwatch `json:"hovered"`
	HoveredOnEra        string   `json:"hoveredOnEra"`
}

// cpScript is the whole measurement, run once per host.
//
// EVERY NUMBER HERE IS READ OFF A RECT, A HIT TEST OR A PIXEL. Nothing asks the
// stylesheet whether it says what it says — the one exception is flagged in its
// own comment, and even that reads the browser's parsed rule rather than the
// file's text.
const cpScript = `
function(host){
  var root = host.querySelector('.cal-block-host') || host;
  var r2 = function(v){ return Math.round(v * 100) / 100 };
  var box = function(el){
    if (!el) return null;
    var r = el.getBoundingClientRect();
    return { l: r.left, t: r.top, w: r.width, h: r.height, r: r.right, b: r.bottom };
  };
  var vis = function(b){ return !!b && b.w > 0 && b.h > 0 };
  var desc = function(el){
    if (!el) return 'NONE';
    var c = (el.getAttribute && el.getAttribute('class')) || '';
    return el.tagName.toLowerCase() + (c ? '.' + c.trim().split(/\s+/).join('.') : '');
  };
  var area = function(a, b){
    if (!vis(a) || !vis(b)) return 0;
    var w = Math.max(0, Math.min(a.r, b.r) - Math.max(a.l, b.l));
    var h = Math.max(0, Math.min(a.b, b.b) - Math.max(a.t, b.t));
    return w * h;
  };

  // THE CANVAS IS THE COLOUR READER. The sheet speaks oklch, color-mix and
  // relative colour syntax; a parser written here would be a second
  // implementation of colour to be wrong in, and it is the RENDERED value the
  // spec's "lighter than the page" is a claim about. over() composites a
  // possibly-translucent colour onto an opaque one exactly as the compositor
  // would, which is how the hover wash is measured.
  var cv = document.createElement('canvas'); cv.width = cv.height = 4;
  var cx = cv.getContext('2d', { willReadFrequently: true });
  var over = function(base, top){
    cx.clearRect(0, 0, 4, 4);
    if (base) { cx.fillStyle = base; cx.fillRect(0, 0, 4, 4) }
    if (top)  { cx.fillStyle = top;  cx.fillRect(0, 0, 4, 4) }
    var d = cx.getImageData(1, 1, 1, 1).data;
    return {
      css: top || base || '',
      r: d[0], g: d[1], b: d[2],
      lum: r2(0.2126 * d[0] + 0.7152 * d[1] + 0.0722 * d[2])
    };
  };
  var swatch = function(el, prop){
    if (!el) return { css: '', r: 0, g: 0, b: 0, lum: -1 };
    return over(getComputedStyle(el)[prop || 'backgroundColor'], null);
  };

  var out = {
    label: host.getAttribute('data-label') || '',
    theme: host.closest('.dark') ? 'dark' : 'light',
    host: r2(root.getBoundingClientRect().width), columnW: 0,
    ground: swatch(root.querySelector('.grid')),
    noEra: { css: '', r: 0, g: 0, b: 0, lum: -1 }, hasNoEra: false, eras: [],
    underShown: false, namedShown: false, chipCells: 0,
    markCells: 0, ulMinW: -1, ulMaxW: 0, segMinW: -1, segsMeasured: 0, segsInked: 0,
    ulSpill: 0, ulClearance: 1e9,
    availCells: 0, datedCells: 0, availH: 0, availBottom: 0, availWidth: 2, availInked: 0,
    dogearCells: 0, dogearOnRow: 0, dogearOnUl: 0, dogearOnAud: 0,
    audPaired: 0, audTested: 0,
    discHits: 0, discHitFirst: '',
    hoverDeclaresColour: false, hoverRule: '', hoveredOnEra: '',
    hovered: { css: '', r: 0, g: 0, b: 0, lum: -1 }
  };

  var cells = [].slice.call(root.querySelectorAll('.cell[data-day]'));
  if (!cells.length) return out;
  var c0 = box(cells[0]);
  var cs0 = getComputedStyle(cells[0]);
  out.columnW = r2(c0.w - parseFloat(cs0.borderLeftWidth) - parseFloat(cs0.borderRightWidth));
  out.underShown = vis(box(cells[0].querySelector('.cunder')));
  out.namedShown = vis(box(cells[0].querySelector('.cnamed')));

  // THE CONTAINING BLOCK OF EVERY CORNER IS THE CELL'S PADDING BOX, not its
  // border box: an absolutely positioned child is laid out inside the border,
  // so "bottom: 0" sits one rule-width above the cell's own bottom edge. That
  // is correct — the availability strip belongs INSIDE the ruled cell — and a
  // probe that compared against the border box would report a 1px defect on
  // every cell and a clean reading only on the last column, which carries no
  // right-hand rule.
  var padBox = function(el){
    var b = box(el), s = getComputedStyle(el);
    return {
      l: b.l + parseFloat(s.borderLeftWidth), t: b.t + parseFloat(s.borderTopWidth),
      r: b.r - parseFloat(s.borderRightWidth), b: b.b - parseFloat(s.borderBottomWidth),
      w: b.w - parseFloat(s.borderLeftWidth) - parseFloat(s.borderRightWidth),
      h: b.h - parseFloat(s.borderTopWidth) - parseFloat(s.borderBottomWidth)
    };
  };

  // ── THE ERA TINT, GROUPED BY THE HUE THE PRODUCER STAMPED ───────────────
  // Grouping by --erahue rather than by day range is what makes "the two eras
  // are the same colour" expressible at all: two groups, two fills, and the
  // caller compares them. The un-tinted arm is the intercalary festival row,
  // which is a day surface that no era covers.
  var byHue = {};
  for (var i = 0; i < cells.length; i++) {
    var hue = (cells[i].style.getPropertyValue('--erahue') || '').trim();
    if (!hue) continue;
    if (!byHue[hue]) byHue[hue] = { hue: hue, cells: 0, fill: swatch(cells[i]) };
    byHue[hue].cells++;
  }
  for (var k in byHue) { if (byHue.hasOwnProperty(k)) out.eras.push(byHue[k]) }
  var interc = root.querySelector('.interc');
  if (interc && vis(box(interc))) { out.noEra = swatch(interc); out.hasNoEra = true }

  // ── THE CORNERS, WALKED OVER EVERY DATED CELL ───────────────────────────
  for (var n = 0; n < cells.length; n++) {
    var cell = cells[n], cb = padBox(cell);
    out.datedCells++;
    if (cell.querySelector('.cnamed .chip')) out.chipCells++;

    var av = cell.querySelector('.avail'), ab = box(av);
    if (av) {
      out.availCells++;
      if (ab) {
        out.availH = r2(ab.h);
        out.availBottom = Math.max(out.availBottom, r2(Math.abs(cb.b - ab.b)));
        out.availWidth = Math.min(out.availWidth, r2(ab.w / cb.w));
      }
      // A RESERVATION IS INVISIBLE. Anything painted here is a feature nobody
      // built, and the operator would read it as one.
      var acs = getComputedStyle(av);
      var apx = over('rgb(0,0,0)', acs.backgroundColor);
      if (apx.r + apx.g + apx.b > 0 || acs.backgroundImage !== 'none') out.availInked++;
    }

    // Only a DRAWN strip is measured: past the named-density flip ".cunder" is
    // "display: none" and its strip has no box at all, which is the swap
    // working rather than the corner failing.
    var ul = cell.querySelector('.ul'), ub = box(ul);
    if (ul && !out.underShown) { ul = null; ub = null }
    if (ul) {
      out.markCells++;
      var w = ub ? ub.w : 0;
      if (out.ulMinW < 0 || w < out.ulMinW) out.ulMinW = r2(w);
      if (w > out.ulMaxW) out.ulMaxW = r2(w);
      if (ub) {
        var sx = Math.max(0, cb.l - ub.l) + Math.max(0, ub.r - cb.r);
        var sy = Math.max(0, cb.t - ub.t) + Math.max(0, ub.b - cb.b);
        out.ulSpill = Math.max(out.ulSpill, r2(Math.max(sx, sy)));
        if (ab) out.ulClearance = Math.min(out.ulClearance, r2(ab.t - ub.b));
      }
      var segs = [].slice.call(ul.querySelectorAll('.ulseg'));
      for (var s = 0; s < segs.length; s++) {
        out.segsMeasured++;
        var sb = box(segs[s]);
        var sw = sb ? sb.w : 0;
        if (out.segMinW < 0 || sw < out.segMinW) out.segMinW = r2(sw);
        // INKED means a rect with area AND a non-transparent fill. Either
        // alone is the failure this file exists for.
        var scs = getComputedStyle(segs[s]);
        var spx = over('rgb(0,0,0)', scs.backgroundColor);
        var inked = vis(sb) && (spx.r + spx.g + spx.b > 0 || scs.backgroundImage !== 'none');
        if (inked) out.segsInked++;
      }
    }

    // ── THE GM FOLD, AND WHETHER IT STILL EATS THE SILHOUETTE ────────────
    var dg = cell.querySelector('.dogear');
    if (dg) {
      out.dogearCells++;
      var db = box(dg);
      out.dogearOnRow = Math.max(out.dogearOnRow, r2(area(db, box(cell.querySelector('.phrow')))));
      out.dogearOnUl = Math.max(out.dogearOnUl, r2(area(db, ub)));

      // THE DIAMOND IS INJECTED WHEN THE DATA DOES NOT HAPPEN TO PAIR THEM.
      // Both gold marks now live on the bottom edge, but no day in this
      // fixture carries a dm_only event AND a restricted one, so measuring
      // only the cells that have both would be an arm that passes because it
      // has no subject. The renderer's own markup is cloned into a fold cell,
      // measured and removed — the same technique the collision was found
      // with. audPaired counts the cells the DATA paired; audTested counts
      // the cells actually measured.
      var aud = cell.querySelector('.audmark'), injected = false;
      if (aud) { out.audPaired++ }
      if (!aud) {
        var donor = root.querySelector('.audmark');
        if (donor) { aud = donor.cloneNode(true); cell.appendChild(aud); injected = true }
      }
      if (aud && vis(box(aud))) {
        out.audTested++;
        out.dogearOnAud = Math.max(out.dogearOnAud, r2(area(db, box(aud))));
      }
      if (injected) aud.remove();

      // AND THE TAP. elementFromPoint at the painted centre of the primary
      // disc: on a day carrying a fold this used to resolve to the fold — an
      // aria-hidden span that neither acts nor falls through — so the moon
      // control had ZERO hittable area on exactly the days the GM most wants
      // it. Scrolled into view first because elementFromPoint is
      // VIEWPORT-relative and this page stacks several hosts.
      var ctl = cell.querySelector('.phctl');
      var disc = cell.querySelector('.ph');
      if (ctl && disc) {
        cell.scrollIntoView({ block: 'center', inline: 'center' });
        var pb = box(disc);
        if (vis(pb)) {
          var hit = document.elementFromPoint(pb.l + pb.w / 2, pb.t + pb.h / 2);
          if (!out.discHitFirst) out.discHitFirst = desc(hit);
          if (hit && (hit === ctl || ctl.contains(hit))) out.discHits++;
        }
      }
    }
  }
  if (out.ulClearance === 1e9) out.ulClearance = -1;

  // ── THE HOVER WASH ──────────────────────────────────────────────────────
  //
  // THE ONE ARM THAT IS NOT A PIXEL A POINTER PRODUCED, and it says so. CSS
  // :hover is a hit-test state; headless Chromium has no pointer to put on a
  // cell, so the rule is read out of the BROWSER'S PARSED CSSOM — not the
  // file's text — its colour is resolved through a live element inside the
  // host (so --text-primary means what it means in this theme), and it is
  // COMPOSITED over the fill of a real era-tinted cell. That is enough to
  // prove the wash is a LAYER rather than a REPLACEMENT: a shorthand shows up
  // here as a declared background-color, which is exactly the regression that
  // made a hovered cell drop its era and read grey.
  var target = null;
  for (var si = 0; si < document.styleSheets.length && !target; si++) {
    var rules;
    try { rules = document.styleSheets[si].cssRules } catch (e) { continue }
    for (var ri = 0; ri < rules.length; ri++) {
      if (rules[ri].selectorText === '.cal-block-host .cell:hover') { target = rules[ri]; break }
    }
  }
  if (target) {
    out.hoverRule = target.style.cssText;
    out.hoverDeclaresColour =
      target.style.getPropertyValue('background-color') !== '' ||
      target.style.getPropertyValue('background') !== '';
    var img = target.style.getPropertyValue('background-image');
    // first top-level argument of the gradient, by balanced parens
    var wash = '';
    var open = img.indexOf('(');
    if (open >= 0) {
      var depth = 0;
      for (var p = open; p < img.length; p++) {
        if (img[p] === '(') depth++;
        else if (img[p] === ')') { depth--; if (depth === 0) break }
        else if (img[p] === ',' && depth === 1) { wash = img.slice(open + 1, p).trim(); break }
      }
    }
    var tinted = null;
    for (var q = 0; q < cells.length && !tinted; q++) {
      if ((cells[q].style.getPropertyValue('--erahue') || '').trim()) tinted = cells[q];
    }
    if (wash && tinted) {
      var probe = document.createElement('i');
      probe.style.position = 'absolute';
      probe.style.backgroundColor = wash;
      tinted.appendChild(probe);
      var resolved = getComputedStyle(probe).backgroundColor;
      probe.remove();
      out.hoveredOnEra = (tinted.style.getPropertyValue('--erahue') || '').trim();
      out.hovered = over(getComputedStyle(tinted).backgroundColor, resolved);
    }
  }
  return out;
}`

// ── the rig ─────────────────────────────────────────────────────────────────

var cpLinkRe = regexp.MustCompile(`<link[^>]*>`)

// cpCase is one host: a width, a theme and the fixture to draw in it.
type cpCase struct {
	label string
	width int
	dark  bool
}

// cpRun lays every case out in one page and reads them all back in one run.
//
// THE HOST WIDTH IS SET DIRECTLY rather than derived from the app shell's
// arithmetic, because this probe's subject is the CELL and its corners, not the
// chain that decides how wide a cell gets — moon_reach_probe_test.go owns that
// chain and reproduces it in full. What is chosen here is a column width on
// each side of the 84px named-density flip, which is the only cell-level
// threshold that changes which corners are occupied at all.
func cpRun(t *testing.T, chrome string, cases []cpCase) []cpReading {
	t.Helper()
	css := blockCSS(t)

	var boxes strings.Builder
	for i, c := range cases {
		d := fxAlmanac(t, true)
		// Each host gets its own slug: every id and radio-group NAME in the
		// Block is derived from it, so several copies on one page would share
		// one radio group and only one could ever be checked (moon_reach's own
		// finding). Nothing measured here reads the slug.
		d.CalendarSlug = fmt.Sprintf("%s-c%d", d.CalendarSlug, i)
		markup := cpLinkRe.ReplaceAllString(render(t, d), "")
		open, close := "", ""
		if c.dark {
			open, close = `<div class="dark">`, `</div>`
		}
		fmt.Fprintf(&boxes,
			`%s<div class="probe-host" data-label="%s" style="width:%dpx">%s</div>%s`,
			open, html.EscapeString(c.label), c.width, markup, close)
	}

	page := `<!doctype html><html><head><meta charset="utf-8"><style>` +
		`html,body{margin:0;background:#888}` +
		`.probe-host{display:block;margin:24px}` +
		css + `</style></head><body>` + boxes.String() +
		`<script>document.addEventListener('DOMContentLoaded',function(){` +
		`var read=` + cpScript + `;` +
		`var out=[].slice.call(document.querySelectorAll('.probe-host')).map(read);` +
		`document.body.setAttribute('data-probe', JSON.stringify(out));});</script>` +
		`</body></html>`

	path := filepath.Join(t.TempDir(), "cellprobe.html")
	if err := os.WriteFile(path, []byte(page), 0o644); err != nil { //nolint:gosec // test artefact
		t.Fatalf("write probe page: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, chrome,
		"--headless", "--no-sandbox", "--disable-gpu", "--hide-scrollbars",
		"--window-size=1800,1400", "--virtual-time-budget=6000",
		"--dump-dom", "file://"+path,
	).Output()
	if err != nil {
		t.Fatalf("chromium: %v", err)
	}
	m := probePayloadRe.FindStringSubmatch(string(out))
	if m == nil {
		t.Fatal("no probe payload in the rendered DOM — the page script did not run")
	}
	var readings []cpReading
	if err := json.Unmarshal([]byte(html.UnescapeString(m[1])), &readings); err != nil {
		t.Fatalf("probe payload: %v", err)
	}
	if len(readings) != len(cases) {
		t.Fatalf("probe returned %d readings for %d cases", len(readings), len(cases))
	}
	return readings
}

// cpCases: the operator's own phone column on each theme, and a column past the
// 84px named-density flip on each theme — the width at which the audience
// diamond joins the fold in the fourth corner.
func cpCases() []cpCase {
	return []cpCase{
		{"light · 366px host · underline density", 366, false},
		{"light · 1400px host · named density", 1400, false},
		{"dark · 366px host · underline density", 366, true},
		{"dark · 1400px host · named density", 1400, true},
	}
}

// ── the guards ──────────────────────────────────────────────────────────────

// TestCellProbe_EveryCornerPaintsAndNoTwoShareOne is C-CALV4-SPEC §1 measured.
//
// "The corners are the information architecture: date in one, ambient marks in
// another, event marks in a third. Keep them in corners; do not centre
// anything." Plus §6's reserved floor along the bottom edge.
//
// THE CLAIM IS PAINT, NOT PLACEMENT. Three of the four assertions below are
// about a rect having AREA and a fill having INK, because the defect this file
// replaces was a corner that had a perfectly correct anchor, a perfectly
// correct offset above the reserved strip, three correctly classed children —
// and no width, so nothing was ever drawn there. The fourth is an
// INTERSECTION: two marks in one corner is the other way a corner architecture
// fails, and it failed silently too.
//
// PROVEN RED. Restoring `max-inline-size` in place of `inline-size` on `.ul`
// fails the strip and segment arms at every width; restoring the fold to
// `top: 0; right: 0` fails the intersection and the hit test on all three
// dm_only days of the fixture.
func TestCellProbe_EveryCornerPaintsAndNoTwoShareOne(t *testing.T) {
	// THE GATE IS INLINE, NOT IN A HELPER, and that is not duplication for its
	// own sake: tools/check-browser-probes.sh takes its census by looking for a
	// Chromium finder INSIDE each top-level Test function's body. A probe that
	// reached the browser through a private helper would be invisible to the
	// census — neither found nor demanded — which is precisely the "nobody ever
	// enumerated the probes" gap that guard exists to close.
	if testing.Short() {
		t.Skip("browser probe: skipped under -short (CI's mode) — a skipped run is NOT a pass")
	}
	chrome := findProbeChromium()
	if chrome == "" {
		t.Skip("browser probe: no Chromium binary found")
	}
	readings := cpRun(t, chrome, cpCases())

	for i, c := range cpCases() {
		r := readings[i]
		t.Run(c.label, func(t *testing.T) {
			t.Logf("host %.1fpx · column %.1fpx · %d dated cells · density %s",
				r.Host, r.ColumnW, r.DatedCells, cpDensity(r))

			// ── THE THIRD CORNER PAINTS ──────────────────────────────────
			//
			// WHICH MARK holds the corner depends on the density: the strip
			// below 84px of column, named chips above it. Both are asserted;
			// neither arm is allowed to be vacuous, which is what the Fatal on
			// an empty subject is for.
			if !r.UnderShown {
				t.Logf("named density: the third corner is drawn by chips — %d cells carry one",
					r.ChipCells)
				if r.ChipCells == 0 {
					t.Fatal("`.cunder` is hidden and no cell carries a named chip either, so " +
						"the third corner is drawing NOTHING at this width and the strip arm " +
						"below would skip it in silence")
				}
			}
			if r.UnderShown {
				if r.MarkCells == 0 {
					t.Fatal("no cell carries an event-mark strip — the fixture has marks on " +
						"eight days, so this probe has no subject and every assertion below " +
						"is vacuous")
				}
				t.Logf("event marks: %d cells · strip %.2f–%.2fpx · narrowest segment %.2fpx · "+
					"%d/%d segments inked · clearance above the reserved floor %.2fpx",
					r.MarkCells, r.UlMinW, r.UlMaxW, r.SegMinW, r.SegsInked, r.SegsMeasured,
					r.UlClearance)
				if r.UlMinW <= 0 {
					t.Errorf("the event-mark strip is %.2fpx wide on some cell. This is the "+
						"corner C-CALV4-SPEC §1 gives to event marks and it is the cell's most "+
						"important content; a box with no width paints nothing and no string "+
						"assertion about its anchor can tell", r.UlMinW)
				}
				if r.SegMinW <= 0 {
					t.Errorf("an event-mark SEGMENT measures %.2fpx. `flex: 1` over "+
						"`flex-basis: 0` with empty content distributes nothing when the row "+
						"itself has no width", r.SegMinW)
				}
				if r.SegsInked != r.SegsMeasured {
					t.Errorf("%d of %d event-mark segments carry no ink. A rect with area and "+
						"a transparent fill is still an empty corner",
						r.SegsMeasured-r.SegsInked, r.SegsMeasured)
				}
				if r.UlSpill > 0.01 {
					t.Errorf("the event-mark strip leaves its own cell by %.2fpx — a mark on "+
						"the wrong day is worse than no mark", r.UlSpill)
				}
				// ── AND IT CLEARS THE FLOOR RESERVED UNDER IT ────────────
				if r.UlClearance < 0 {
					t.Errorf("the event-mark strip overlaps the reserved availability slot by "+
						"%.2fpx. The whole value of reserving the bottom edge now is that step "+
						"three fills it and relayouts nothing", -r.UlClearance)
				}
			}
			if r.AvailCells != r.DatedCells {
				t.Errorf("%d availability slots for %d dated cells — the reservation is "+
					"unconditional BY DESIGN, or the cell's interior reflows from day to day",
					r.AvailCells, r.DatedCells)
			}
			t.Logf("availability slot: %d/%d cells · %.2fpx tall · %.2fpx off the cell's "+
				"inner bottom · %.0f%% of its width · %d painted",
				r.AvailCells, r.DatedCells, r.AvailH, r.AvailBottom, r.AvailWidth*100,
				r.AvailInked)
			if r.AvailH <= 0 || r.AvailBottom > 0.01 || r.AvailWidth < 0.99 {
				t.Errorf("the availability slot measures %.2fpx tall, %.2fpx off the cell's "+
					"inner bottom, %.0f%% of its width — C-CALV4-SPEC §6 puts a strip ALONG "+
					"THE BOTTOM EDGE", r.AvailH, r.AvailBottom, r.AvailWidth*100)
			}
			if r.AvailInked != 0 {
				t.Errorf("%d availability slots are painted. It is a RESERVATION — painting it "+
					"now ships a feature nobody built, and the operator would read it as one",
					r.AvailInked)
			}

			// ── THE FOURTH CORNER DOES NOT SHARE THE AMBIENT ONE ─────────
			if r.DogearCells == 0 {
				t.Fatal("no cell carries the GM fold — the GM fixture has three dm_only days, " +
					"so the collision arm below would pass vacuously")
			}
			t.Logf("GM fold: %d cells · overlap with the moon row %.2fpx², with the event "+
				"marks %.2fpx², with the audience diamond %.2fpx² (%d cells measured, %d of "+
				"them paired by the data) · disc hit %d/%d (%s)",
				r.DogearCells, r.DogearOnRow, r.DogearOnUl, r.DogearOnAud,
				r.AudTested, r.AudPaired, r.DiscHits, r.DogearCells, r.DiscHitFirst)
			if r.NamedShown && r.AudTested == 0 {
				t.Error("the audience diamond is drawn at this density and no fold cell could " +
					"be measured against one, injected or otherwise — the two-gold-marks arm " +
					"has no subject and passes for the wrong reason")
			}
			if r.DogearOnRow > 0 {
				t.Errorf("the GM fold overlaps the always-visible moon silhouette by %.2fpx². "+
					"C-CALV4-SPEC §1 gives each mark a CORNER and §4 makes the silhouette "+
					"unconditional — two marks in one corner is how the one element that can "+
					"never be absent becomes the one element that can be made unreachable",
					r.DogearOnRow)
			}
			if r.DogearOnUl > 0 {
				t.Errorf("the GM fold overlaps the event-mark strip by %.2fpx² — it moved out "+
					"of one occupied corner into another", r.DogearOnUl)
			}
			if r.DogearOnAud > 0 {
				t.Errorf("the GM fold overlaps the audience diamond by %.2fpx². Two gold "+
					"permission marks stacked read as one mark nobody can name", r.DogearOnAud)
			}
			if r.DiscHits != r.DogearCells {
				t.Errorf("on %d of %d dm_only days a pointer at the silhouette's painted centre "+
					"resolves to %q instead of the moon control. The fold is an aria-hidden, "+
					"non-interactive span above `.dsel`, so it neither acts on the tap nor lets "+
					"it through: no panel, no day selected, the gesture consumed",
					r.DogearCells-r.DiscHits, r.DogearCells, r.DiscHitFirst)
			}
		})
	}
}

// TestCellProbe_TheEraTintCarriesTheEraAndKeepsThePop is C-CALV4-SPEC §1's
// "popped out" and §2's era decision, measured together because one expression
// produces both and it failed at both.
//
// WHAT SHIPPED. `color-mix(in oklch, var(--surface-card) 92%, var(--erahue))`
// interpolates the HUE ANGLE from --surface-card's hue 0, so the teal era
// computed to a pink at 8% and the two eras rendered 2/255 apart — an era tint
// that cannot distinguish two eras is not grouping. The same mix dragged
// LIGHTNESS toward an L≈0.55 ink and spent the entire 12-unit budget between
// the grid ground and white: era-A cells measured 242.7 against a ground of
// 242.9, i.e. DARKER than the surface they are supposed to be raised above.
//
// BOTH THEMES, ALWAYS. "Lighter than the page" inverts — on light the ground
// darkens because nothing is lighter than white, on dark the cell takes the
// lighter of the two surfaces — and the shipped tint pushed one way on light
// and the other on dark. A single-arm test would have been green on the arm
// that happened to work.
func TestCellProbe_TheEraTintCarriesTheEraAndKeepsThePop(t *testing.T) {
	// Inline for the census's sake — see the note on the probe above.
	if testing.Short() {
		t.Skip("browser probe: skipped under -short (CI's mode) — a skipped run is NOT a pass")
	}
	chrome := findProbeChromium()
	if chrome == "" {
		t.Skip("browser probe: no Chromium binary found")
	}
	readings := cpRun(t, chrome, cpCases())

	// THE POP'S MARGIN IS DELIBERATELY SMALL AND DELIBERATELY NOT ZERO. The
	// question the spec asks is directional — is the cell on the raised side of
	// its own ground — and a zero margin would be satisfied by a cell that
	// merely fails to be darker. 2.0 of 255 is about the smallest step that is
	// a step rather than rounding.
	const popMargin = 2.0
	// Two eras are DIFFERENT if any sRGB channel differs by this much. The
	// shipped tint managed 2; the band above the row, which got its hue right,
	// manages tens.
	const eraSeparation = 8.0

	for i, c := range cpCases() {
		r := readings[i]
		t.Run(c.label, func(t *testing.T) {
			if len(r.Eras) < 2 {
				t.Fatalf("%d era tints found — the fixture spans two eras, so a comparison "+
					"between them is what this test is for", len(r.Eras))
			}
			t.Logf("ground %s lum %.1f", r.Ground.CSS, r.Ground.Lum)
			if r.HasNoEra {
				t.Logf("day surface with NO era: rgb(%.0f,%.0f,%.0f) lum %.1f (%+.1f vs ground)",
					r.NoEra.R, r.NoEra.G, r.NoEra.B, r.NoEra.Lum, r.NoEra.Lum-r.Ground.Lum)
				if r.NoEra.Lum <= r.Ground.Lum+popMargin {
					t.Errorf("an un-tinted day surface is %.1f against a ground of %.1f. The "+
						"spec's first ingredient is a fill on the RAISED side of the page, and "+
						"this arm carries no era at all — so the tint is not even the excuse",
						r.NoEra.Lum, r.Ground.Lum)
				}
			}

			for _, e := range r.Eras {
				t.Logf("era %s · %d cells · rgb(%.0f,%.0f,%.0f) lum %.1f (%+.1f vs ground)",
					e.Hue, e.Cells, e.Fill.R, e.Fill.G, e.Fill.B, e.Fill.Lum,
					e.Fill.Lum-r.Ground.Lum)
				if e.Fill.Lum <= r.Ground.Lum+popMargin {
					t.Errorf("era %s fills its cells at lum %.1f against a ground of %.1f. The "+
						"era covers whole months, so this is the spec's \"fill slightly lighter "+
						"than the page\" measurably absent for the operator's entire calendar — "+
						"only the edge and the padding would survive", e.Hue, e.Fill.Lum,
						r.Ground.Lum)
				}
			}

			// EVERY PAIR, not just the first two: a third era that collided
			// with the second would otherwise ride along.
			for a := 0; a < len(r.Eras); a++ {
				for b := a + 1; b < len(r.Eras); b++ {
					x, y := r.Eras[a], r.Eras[b]
					d := maxf(maxf(absf(x.Fill.R-y.Fill.R), absf(x.Fill.G-y.Fill.G)),
						absf(x.Fill.B-y.Fill.B))
					t.Logf("era separation %s vs %s: worst channel %.1f/255", x.Hue, y.Hue, d)
					if d < eraSeparation {
						t.Errorf("eras %s and %s render %.1f/255 apart on their widest channel. "+
							"A tint that cannot distinguish two eras is not grouping, and it is "+
							"what an oklch mix from a hue-0 white produces: it rotates a few "+
							"degrees off white and never adopts the era's hue at all",
							x.Hue, y.Hue, d)
					}
				}
			}

			// ── THE HOVER WASH IS A LAYER, NOT A REPLACEMENT ─────────────
			t.Logf("hover rule: %s", r.HoverRule)
			if r.HoverDeclaresColour {
				t.Error("`.cell:hover` declares a background COLOUR (or the shorthand, which " +
					"resets one). It out-specifies the cell's own fill, so a hovered cell drops " +
					"both the pop and the era and composites 5% ink straight onto the grid " +
					"ground — measured 12 below the ground it should be above. The wash belongs " +
					"in `background-image`, over the fill")
			}
			if r.Hovered.Lum < 0 {
				t.Error("the hover wash could not be resolved — the rule was found but its " +
					"colour never reached a pixel, so this arm proved nothing")
			} else {
				t.Logf("hovered era-%s cell: rgb(%.0f,%.0f,%.0f) lum %.1f", r.HoveredOnEra,
					r.Hovered.R, r.Hovered.G, r.Hovered.B, r.Hovered.Lum)
				// The hue has to SURVIVE the wash. An achromatic wash over a
				// tinted fill keeps the channel order; a wash that replaced
				// the fill would flatten it to the ground's near-neutral.
				var era cpEra
				for _, e := range r.Eras {
					if e.Hue == r.HoveredOnEra {
						era = e
					}
				}
				if era.Hue != "" {
					restCast := era.Fill.R - era.Fill.B
					hoverCast := r.Hovered.R - r.Hovered.B
					t.Logf("era cast at rest %+.1f (R-B), hovered %+.1f", restCast, hoverCast)
					if restCast*hoverCast <= 0 {
						t.Errorf("the era's colour cast flips or vanishes under the hover wash "+
							"(rest %+.1f, hovered %+.1f). The hovered cell is the one cell the "+
							"reader is looking at; losing its era there is losing it where it "+
							"matters", restCast, hoverCast)
					}
				}
			}
		})
	}
}

// cpDensity names which subtree the cell is drawing, for the log line. It reads
// the MEASURED visibility rather than the width, so a threshold that moved and
// a threshold that stopped working read differently.
func cpDensity(r cpReading) string {
	switch {
	case r.NamedShown && !r.UnderShown:
		return "named"
	case r.UnderShown && !r.NamedShown:
		return "underline"
	default:
		return "BOTH-OR-NEITHER"
	}
}

func absf(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
