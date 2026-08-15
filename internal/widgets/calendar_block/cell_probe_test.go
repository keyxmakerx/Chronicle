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
//
// Rule is the tile's 1px EDGE on that era's cells, read off `::before`. It is
// here rather than beside the fill because the edge is derived FROM the fill
// (--tile-rule reads --cellbase), so an era whose fill carried its hue and
// whose edge did not would be a build where the derivation had been replaced
// by a flat token again — which is exactly what shipped.
type cpEra struct {
	Hue   string   `json:"hue"`
	Cells int      `json:"cells"`
	Fill  cpSwatch `json:"fill"`
	Rule  cpSwatch `json:"rule"`
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

	// ── THE THREE RULES OF THE GRID, IN LOUDNESS ORDER ────────────────────
	//
	// NoEraRule is the untinted day surface's tile edge; FiveColRule is
	// `.cell.half`'s border-right, the counting aid; the selected edge is per
	// era in Sel[].BorderPx. All three are read as PIXELS rather than as token
	// names, because "one step off the fill" is a claim about what the engine
	// paints and the token it happens to be written as cannot answer it.
	NoEraRule   cpSwatch `json:"noEraRule"`
	FiveColRule cpSwatch `json:"fiveColRule"`
	HasFiveCol  bool     `json:"hasFiveCol"`

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

	// ── SELECTION, MEASURED ON A TINTED DAY ───────────────────────────────
	//
	// C-CALV4-TILES §4. Selection used to be written as the `background:`
	// SHORTHAND against --surface-card, at higher specificity than the fill
	// chain, so a selected day silently dropped its era tint — and the day
	// that left its era was the one the reader had just pointed at. Nothing
	// in the suite could see it: the markup was identical and the stylesheet
	// said what it meant to say. So it is read back out of the layout, on a
	// cell that carries an era, with its own radio actually checked.
	// ONE ENTRY PER ERA, not one overall. A single selected cell can only be
	// compared with its own resting state, and on dark that comparison is not
	// enough: the shipped shorthand mixed --surface-card with the blue accent,
	// which happens to carry the same R−B sign as the teal era, so a
	// same-cell test passed on two of four hosts while the defect was live on
	// all four. TWO selected eras cannot agree by accident — under the
	// shorthand they are the same pixel, because neither reads --cellbase.
	Sel []cpSelection `json:"sel"`
}

// cpSelection is one era's day, read at rest and with its own radio checked.
//
// BorderPx is the selected edge as a PIXEL. Border (the CSS string) is enough
// to prove selection changed the edge at all; only the pixel can be compared
// with the tile rule's own distance from the ground, which is what "selection
// is allowed to be loud, the tile edge is not" actually asserts.
type cpSelection struct {
	Hue        string   `json:"hue"`
	Rest       cpSwatch `json:"rest"`
	RestBorder string   `json:"restBorder"`
	Fill       cpSwatch `json:"fill"`
	Border     string   `json:"border"`
	BorderPx   cpSwatch `json:"borderPx"`
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
  // THE DAY SURFACE'S FILL LIVES ON ITS TILE (C-CALV4-TILES §1), so every
  // reading of "what colour is this cell" has to ask the pseudo-element.
  // Asking the element instead now answers a transparent black for every cell
  // in the month, which would read as "the fill is gone" rather than as "the
  // probe is looking in the wrong place" — the two are indistinguishable in
  // the payload, which is why this helper exists rather than an inline call.
  var tileSwatch = function(el){
    if (!el) return { css: '', r: 0, g: 0, b: 0, lum: -1 };
    return over(getComputedStyle(el, '::before').backgroundColor, null);
  };
  // AND ITS EDGE. Same pseudo-element, same reason. borderTopColor rather than
  // the shorthand because a shorthand read is a string; this has to be a pixel,
  // and all four sides of the tile carry the same declaration.
  var tileRuleSwatch = function(el){
    if (!el) return { css: '', r: 0, g: 0, b: 0, lum: -1 };
    return over(getComputedStyle(el, '::before').borderTopColor, null);
  };

  var out = {
    label: host.getAttribute('data-label') || '',
    theme: host.closest('.dark') ? 'dark' : 'light',
    host: r2(root.getBoundingClientRect().width), columnW: 0,
    ground: swatch(root.querySelector('.grid')),
    noEra: { css: '', r: 0, g: 0, b: 0, lum: -1 }, hasNoEra: false, eras: [],
    noEraRule: { css: '', r: 0, g: 0, b: 0, lum: -1 },
    fiveColRule: { css: '', r: 0, g: 0, b: 0, lum: -1 }, hasFiveCol: false,
    underShown: false, namedShown: false, chipCells: 0,
    markCells: 0, ulMinW: -1, ulMaxW: 0, segMinW: -1, segsMeasured: 0, segsInked: 0,
    ulSpill: 0, ulClearance: 1e9,
    availCells: 0, datedCells: 0, availH: 0, availBottom: 0, availWidth: 2, availInked: 0,
    dogearCells: 0, dogearOnRow: 0, dogearOnUl: 0, dogearOnAud: 0,
    audPaired: 0, audTested: 0,
    discHits: 0, discHitFirst: '',
    hoverDeclaresColour: false, hoverRule: '', hoveredOnEra: '',
    hovered: { css: '', r: 0, g: 0, b: 0, lum: -1 },
    sel: []
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
    if (!byHue[hue]) {
      byHue[hue] = { hue: hue, cells: 0, fill: tileSwatch(cells[i]), rule: tileRuleSwatch(cells[i]) };
    }
    byHue[hue].cells++;
  }
  for (var k in byHue) { if (byHue.hasOwnProperty(k)) out.eras.push(byHue[k]) }
  var interc = root.querySelector('.interc');
  if (interc && vis(box(interc))) { out.noEra = tileSwatch(interc); out.hasNoEra = true }
  // THE UNTINTED TILE'S EDGE IS READ WITHOUT THE VISIBILITY GATE THE FILL
  // TAKES, and deliberately. The fill's gate exists because "the ground shows
  // between two tiles" is a claim about a DRAWN surface; the rule's band is a
  // claim about a COMPUTED colour, and the intercalary row is display:none at
  // the narrow hosts. Gating it the same way would silently reduce the band to
  // the two wide hosts — half the census, on the arm where a too-loud edge is
  // hardest to see. The tinted arms below are measured on cells that are drawn
  // at every host, so the drawn case is covered either way.
  if (interc) { out.noEraRule = tileRuleSwatch(interc) }
  // THE FIVE-COLUMN RULE is a border on .cell itself, not on the tile: it is
  // drawn in the 3px gutter BETWEEN two tiles, which is the whole reason it
  // survived the tile construction as the one remaining border on the element.
  var halfCell = root.querySelector('.cell.half');
  if (halfCell) {
    out.fiveColRule = over(getComputedStyle(halfCell).borderRightColor, null);
    out.hasFiveCol = true;
  }

  // ── SELECTION, ON A TINTED DAY, WITH ITS OWN RADIO CHECKED ─────────────
  //
  // The day pick is a hidden radio read by :has(), so a selected day is
  // produced here exactly the way a click produces one — no class is moved
  // and no state is faked. The resting fill is read from the SAME cell first,
  // so the two readings cannot be about two different eras. It is unchecked
  // again immediately, because one radio group serves the whole month and a
  // day left checked would silently unselect the next era's day.
  var selSeen = {};
  for (var s = 0; s < cells.length; s++) {
    var eh = (cells[s].style.getPropertyValue('--erahue') || '').trim();
    var pick = cells[s].querySelector('.daypick');
    if (!eh || !pick || selSeen[eh]) continue;
    selSeen[eh] = true;
    var rec = {
      hue: eh,
      rest: tileSwatch(cells[s]),
      restBorder: getComputedStyle(cells[s], '::before').borderTopColor,
      fill: null, border: '', borderPx: { css: '', r: 0, g: 0, b: 0, lum: -1 }
    };
    pick.checked = true;
    rec.fill = tileSwatch(cells[s]);
    rec.border = getComputedStyle(cells[s], '::before').borderTopColor;
    rec.borderPx = tileRuleSwatch(cells[s]);
    pick.checked = false;
    out.sel.push(rec);
  }

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
      if (rules[ri].selectorText === '.cal-block-host .cell:hover::before') { target = rules[ri]; break }
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
      out.hovered = over(getComputedStyle(tinted, '::before').backgroundColor, resolved);
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

	// ── SOFTNESS IS A BAND, AND THE MISSING HALF WAS THE CEILING ────────────
	//
	// This was `eraSeparation = 8.0`, a FLOOR with nothing above it, and that
	// is precisely how a tint separating two eras by 19/255 on light and 29/255
	// on dark shipped through a green suite. The reference's ENTIRE page-to-cell
	// pop step is 13/255 (C-CALV4-TILE-RECIPE §1), so 29 is 2.2× the strongest
	// step anywhere in the reference grid — at that strength the era stops
	// reading as a grouping laid over a month and starts reading as two
	// different calendars side by side. The operator's word for what they wanted
	// was "soft", and a one-sided assertion cannot express soft: every number
	// above the floor passed, including every number that was too loud.
	//
	// So the assertion is a BAND. Below the floor two eras are the same colour
	// and the tint is not grouping anything; above the ceiling it is shouting.
	// Both ends are measured in a real engine, in both themes, because chroma
	// reads weaker at low lightness and the two arms carry different numbers
	// (0.010 light, 0.012 dark) for exactly that reason.
	const (
		eraSeparationMin = 4.0
		eraSeparationMax = 12.0
	)

	for i, c := range cpCases() {
		r := readings[i]
		t.Run(c.label, func(t *testing.T) {
			if len(r.Eras) < 2 {
				t.Fatalf("%d era tints found — the fixture spans two eras, so a comparison "+
					"between them is what this test is for", len(r.Eras))
			}
			t.Logf("ground %s lum %.1f", r.Ground.CSS, r.Ground.Lum)

			// ── THE GROUND AND THE FILL MUST BE DIFFERENT COLOURS ─────────
			//
			// C-CALV4-TILES §2. The tile construction only produces the
			// reference's "popped out" if the 3px it opens shows something
			// DIFFERENT from what the tile is filled with. The two tokens
			// already differ in both themes (--surface-inset vs --surface-card,
			// which swap between light and dark), but nothing asserted it, and
			// a build that pointed both at the same token would ship a grid of
			// invisible tiles: perfect geometry, no separation, and every
			// geometric probe in this package still green.
			//
			// Measured as sRGB on the untinted day surface, so the era tint is
			// not doing the work.
			if r.HasNoEra {
				d := maxf(maxf(absf(r.NoEra.R-r.Ground.R), absf(r.NoEra.G-r.Ground.G)),
					absf(r.NoEra.B-r.Ground.B))
				t.Logf("ground vs untinted tile fill: worst channel %.1f/255", d)
				if d < popMargin {
					t.Errorf("the grid's ground and the tile's fill differ by %.1f/255 on their "+
						"widest channel. The tile's whole construction is that 3px of GROUND "+
						"shows between two fills; ground and fill at the same value is a month "+
						"of invisible tiles, and every geometric assertion in this package "+
						"stays green through it", d)
				}
			}

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
					t.Logf("era separation %s vs %s: worst channel %.1f/255 (band %.0f–%.0f)",
						x.Hue, y.Hue, d, eraSeparationMin, eraSeparationMax)
					if d < eraSeparationMin {
						t.Errorf("eras %s and %s render %.1f/255 apart on their widest channel, "+
							"below the %.0f floor. A tint that cannot distinguish two eras is not "+
							"grouping, and it is what an oklch mix from a hue-0 white produces: it "+
							"rotates a few degrees off white and never adopts the era's hue at all",
							x.Hue, y.Hue, d, eraSeparationMin)
					}
					if d > eraSeparationMax {
						t.Errorf("eras %s and %s render %.1f/255 apart on their widest channel, "+
							"above the %.0f ceiling. The reference's ENTIRE page-to-cell pop step "+
							"is 13/255, so an era-to-era difference at this strength reads as two "+
							"different calendars rather than as a grouping over one month — and "+
							"the era is now the ONLY thing the grid says about eras, so it is the "+
							"loudest thing on the surface. THIS ARM IS THE ONE THAT WAS MISSING: "+
							"the assertion was a bare floor, and 19/255 on light and 29/255 on "+
							"dark shipped straight through it",
							x.Hue, y.Hue, d, eraSeparationMax)
					}
				}
			}

			// ── A SELECTED DAY KEEPS ITS ERA (C-CALV4-TILES §4) ──────────
			//
			// WHAT SHIPPED. `.cell.sel` / `:has(> .daypick:checked)` declared
			// `background: color-mix(in oklch, var(--surface-card) 96%,
			// var(--accent))` — a SHORTHAND, so it reset background-color; at
			// higher specificity than the fill chain; and naming --surface-card
			// rather than --cellbase. A selected day therefore painted the
			// UNTINTED surface plus a wash of accent, i.e. it left its era, and
			// the day that left its era was the one the reader had just pointed
			// at. Nothing could see it: the markup is identical either way.
			//
			// TWO ERAS ARE MEASURED, NOT ONE, AND THAT IS THE LOAD-BEARING PART.
			// Comparing a selected cell only with its own resting state passes on
			// dark, because --surface-card mixed with the blue accent happens to
			// carry the same R−B sign as the fixture's teal era — measured, with
			// the shorthand restored: light flips the cast −10.0 → +4.0 and reds,
			// dark goes −9.0 → −13.0 and stays green. TWO selected eras cannot
			// agree by accident: under the shorthand neither reads --cellbase, so
			// both paint the SAME pixel, and the difference between them goes to
			// zero in every theme. Verified red on all four hosts by restoring
			// the declaration.
			if len(r.Sel) < 2 {
				t.Errorf("%d era(s) offered a day carrying a selection control. The fixture "+
					"must dock the Ledger (which is what emits the day-pick radios) and span "+
					"TWO eras, or the cross-era arm below has nothing to compare and the "+
					"same-cell arms pass on a theme where they cannot discriminate",
					len(r.Sel))
			}
			for _, s := range r.Sel {
				restCast := s.Rest.R - s.Rest.B
				selCast := s.Fill.R - s.Fill.B
				t.Logf("selection on era %s: rest rgb(%.0f,%.0f,%.0f) cast %+.1f border %s → "+
					"selected rgb(%.0f,%.0f,%.0f) cast %+.1f border %s",
					s.Hue, s.Rest.R, s.Rest.G, s.Rest.B, restCast, s.RestBorder,
					s.Fill.R, s.Fill.G, s.Fill.B, selCast, s.Border)

				if restCast*selCast <= 0 {
					t.Errorf("era %s: the colour cast flips or vanishes when the day is SELECTED "+
						"(rest %+.1f, selected %+.1f). Selection is written against --cellbase "+
						"precisely so that whatever the resting fill is — plain surface or era "+
						"tint — the selected fill is that same colour at another lightness",
						s.Hue, restCast, selCast)
				}
				if absf(s.Fill.Lum-s.Rest.Lum) < 1.0 {
					t.Errorf("era %s: a selected day fills at lum %.1f against a resting %.1f — "+
						"the two are the same surface, so selection is carried by the edge alone",
						s.Hue, s.Fill.Lum, s.Rest.Lum)
				}
				if s.Border == s.RestBorder {
					t.Errorf("era %s: the tile's rule is %s both at rest and when selected. "+
						"Selection is IDENTITY: the edge is the accent, and nothing else in the "+
						"grid is", s.Hue, s.Border)
				}
			}
			// THE CROSS-ERA ARM. Two SELECTED days in two different eras must
			// still be as far apart as two resting ones — and no further.
			//
			// THE CEILING HERE IS THE RESTING PAIR'S OWN SEPARATION PLUS A
			// ROUNDING ALLOWANCE, not the 4–12 band above, and that is
			// deliberate rather than a loosened assertion. The band is a claim
			// about how loud the TINT is; this is a claim about what SELECTION
			// is allowed to do to it, which is: change lightness, and nothing
			// else. Measured, the light arm lands at 12.0 against a resting
			// 11.0 — reusing the band's own ceiling would put the assertion
			// exactly on its boundary, where a rounding wobble reds a correct
			// build and teaches the next hand to widen the number.
			for a := 0; a < len(r.Sel); a++ {
				for b := a + 1; b < len(r.Sel); b++ {
					x, y := r.Sel[a], r.Sel[b]
					selD := maxf(maxf(absf(x.Fill.R-y.Fill.R), absf(x.Fill.G-y.Fill.G)),
						absf(x.Fill.B-y.Fill.B))
					restD := maxf(maxf(absf(x.Rest.R-y.Rest.R), absf(x.Rest.G-y.Rest.G)),
						absf(x.Rest.B-y.Rest.B))
					t.Logf("era separation %s vs %s: at rest %.1f/255 · SELECTED %.1f/255",
						x.Hue, y.Hue, restD, selD)
					if selD < eraSeparationMin {
						t.Errorf("two SELECTED days in different eras render %.1f/255 apart "+
							"(they are %.1f apart at rest), below the %.0f floor two resting days "+
							"must clear. The selected fill is not reading --cellbase: it is "+
							"painting ONE colour for every era, which is exactly what "+
							"`background: color-mix(…var(--surface-card)…)` did — selection eats "+
							"the tint, on the one day the reader is pointing at",
							selD, restD, eraSeparationMin)
					}
					if selD > restD+2.0 {
						t.Errorf("two SELECTED days in different eras render %.1f/255 apart "+
							"against %.1f at rest. Selection may change LIGHTNESS only; a wider "+
							"separation when selected means the expression is touching chroma or "+
							"hue as well, and the softness ceiling stops holding for a chosen day",
							selD, restD)
					}
				}
			}

			// ── THE HOVER WASH IS A LAYER, NOT A REPLACEMENT ─────────────
			t.Logf("hover rule: %s", r.HoverRule)
			if r.HoverRule == "" {
				t.Error("no `.cal-block-host .cell:hover::before` rule was found in the parsed " +
					"CSSOM. The wash moved onto the TILE with the fill (C-CALV4-TILES §1) — on " +
					"the cell it would paint the whole border box and close the 3px gutter, so " +
					"a hovered day would be the one cell in the month that is not a tile")
			}
			if r.HoverDeclaresColour {
				t.Error("`.cell:hover::before` declares a background COLOUR (or the shorthand, which " +
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

// ── THE TILE'S RULE HAS A CEILING NOW, AND THE MISSING CEILING WAS THE DEFECT ─

// cpGroundGap is one surface's distance from the grid ground, in luminance.
//
// LUMINANCE AND NOT WORST-CHANNEL, and the choice is load-bearing. The tile's
// edge is derived from --cellbase, so on a tinted day it carries the era's HUE
// as well as its lightness — a worst-channel reading of a hued edge charges it
// for chroma, which is the one component the softness law is not about. The
// question here is "how far off the ground does this edge sit", and that is a
// lightness question.
func cpGroundGap(s, ground cpSwatch) float64 { return absf(s.Lum - ground.Lum) }

// TestCellProbe_TheTileRuleIsTheQuietestRuleInTheGrid.
//
// WHY THIS EXISTS AT ALL, STATED PLAINLY: A STEP WITH A FLOOR AND NO CEILING IS
// HOW THE LAST TWO LOUD THINGS SHIPPED.
//
//	· The era tint had `eraSeparation = 8.0` as a FLOOR and nothing above it, so
//	  a tint separating two eras by 19/255 (light) and 29/255 (dark) sailed
//	  through a fully green suite — 1.5× and 2.2× the reference's ENTIRE
//	  page-to-cell pop step. C-CALV4-TILES §3 turned it into a band, 4–12.
//	· The tile's own RULE then shipped inside that same change — a change whose
//	  stated purpose was softness — at --rule-structural, a general STRUCTURAL
//	  rule token. Measured: a 50/255 fill→rule step on light and 29–32/255 on
//	  dark, 3.6× and 2.3× the reference's +14, and roughly 3× the era separation
//	  the tile is framing. It had a floor of nothing and a ceiling of nothing.
//	  Every geometric probe in this package stayed green, because geometry is
//	  not what was wrong.
//
// A tile edge is not a structural rule and must not borrow one. So:
//
// THE LAW — THE TILE RULE SITS ONE POP STEP ON THE FAR SIDE OF THE GROUND FROM
// THE FILL, and it is expressed AGAINST THE GROUND so that one law covers both
// themes. It cannot be expressed against the fill, because the two themes have
// different room to move:
//
//	dark   the fill is ABOVE the ground (25.0 → 34.8) with headroom left, so
//	       the rule goes further ABOVE the fill and reads as a lit edge —
//	       the reference's own construction, +14 over the fill.
//	light  the fill is ABOVE the ground too (242.9 → 255.0) but CLAMPED AT
//	       WHITE, so "further away from the ground" has nowhere to go. Copying
//	       the reference's +14 in the other direction lands the rule on 241,
//	       which IS the ground (242.9), and the edge vanishes. The rule
//	       therefore crosses the ground and sits one pop step BELOW it.
//
// Both arms are then the same sentence — the rule is about one pop step off the
// ground, on whichever side the fill is not — and the band below is that
// sentence with numbers. It is deliberately wide because it spans two themes
// whose pop steps differ (+12.1 light, +9.8 dark) and because a hued edge sits
// a little further out than a neutral one; the values that made this test
// necessary are 36.1 (light) and 39.8 (dark) and both are outside it.
func TestCellProbe_TheTileRuleIsTheQuietestRuleInTheGrid(t *testing.T) {
	if testing.Short() {
		t.Skip("browser probe: skipped under -short (CI's mode) — a skipped run is NOT a pass")
	}
	chrome := findProbeChromium()
	if chrome == "" {
		t.Skip("browser probe: no Chromium binary found")
	}
	readings := cpRun(t, chrome, cpCases())

	// THE BAND. Floor: an edge closer than this to the ground it sits in is an
	// edge nobody can see, and the tile's whole construction is a fill, a rule
	// and a gutter — losing the rule leaves two of three. Ceiling: twice the
	// reference's pop step plus the room a hued edge needs. Measured, all six
	// readings per theme: 12.9–16.3 on light, 23.9–28.1 on dark.
	const (
		tileRuleVsGroundMin = 8.0
		tileRuleVsGroundMax = 32.0
	)

	for i, c := range cpCases() {
		r := readings[i]
		t.Run(c.label, func(t *testing.T) {
			t.Logf("ground %s lum %.1f", r.Ground.CSS, r.Ground.Lum)

			// Every tile edge in the host: the untinted day surface and each
			// era's. The tinted ones are what a viewer actually sees on the
			// operator's calendar, which is covered end to end by two eras.
			type ruleReading struct {
				what string
				sw   cpSwatch
			}
			rules := []ruleReading{{"untinted tile", r.NoEraRule}}
			for _, e := range r.Eras {
				rules = append(rules, ruleReading{"tile on era " + e.Hue, e.Rule})
			}
			if len(rules) < 2 {
				t.Fatal("no era tile edges were read — the fixture spans two eras, so this " +
					"census is empty and every arm below is vacuous")
			}

			loudestTile := 0.0
			for _, rr := range rules {
				if rr.sw.Lum < 0 {
					t.Errorf("%s: no edge colour was resolved at all. The tile's rule lives on "+
						"`::before`; a reading of -1 means the probe found no pseudo-element, "+
						"not that the edge is subtle", rr.what)
					continue
				}
				gap := cpGroundGap(rr.sw, r.Ground)
				if gap > loudestTile {
					loudestTile = gap
				}
				t.Logf("%s: rgb(%.0f,%.0f,%.0f) lum %.1f · %.1f from the ground (band %.0f–%.0f)",
					rr.what, rr.sw.R, rr.sw.G, rr.sw.B, rr.sw.Lum, gap,
					tileRuleVsGroundMin, tileRuleVsGroundMax)
				if gap < tileRuleVsGroundMin {
					t.Errorf("%s sits %.1f from the ground in luminance, below the %.0f floor. "+
						"Softening a rule until it disappears is not softness — the tile is a "+
						"fill, a rule and a gutter, and an invisible rule leaves two of three. "+
						"This is the failure mode of copying the reference's dark +14 onto "+
						"light: 14 below white is 241, and the ground is 242.9",
						rr.what, gap, tileRuleVsGroundMin)
				}
				if gap > tileRuleVsGroundMax {
					t.Errorf("%s sits %.1f from the ground in luminance, above the %.0f ceiling. "+
						"THIS IS THE ARM THAT DID NOT EXIST, and its absence is the actual "+
						"defect it guards: the tile edge shipped at --rule-structural — 36.1 "+
						"from the ground on light and 39.7 on dark, a 50/255 fill→rule step and "+
						"3× the era separation the tile is framing — inside a change whose "+
						"stated purpose was softness, past a suite that banded the era tint and "+
						"left this step with a floor of nothing and a ceiling of nothing. A tile "+
						"edge is not a STRUCTURAL rule and must not borrow the structural token",
						rr.what, gap, tileRuleVsGroundMax)
				}
			}

			// ── THE ORDER OF THE THREE RULES ──────────────────────────────
			//
			// Softening the tile is only correct if the two rules that are
			// SUPPOSED to be loud stay loud. Both are measured rather than
			// asserted by token name, because "one full ramp step above" is a
			// claim about paint.
			if !r.HasFiveCol {
				t.Error("no `.cell.half` was found, so the five-column rule was never measured. " +
					"It is the counting aid on a ten-day week — humans cannot count to ten " +
					"across identical columns — and the whole point of softening the tile " +
					"edge is that this rule stays a step above it")
			} else {
				fiveGap := cpGroundGap(r.FiveColRule, r.Ground)
				t.Logf("five-column rule: rgb(%.0f,%.0f,%.0f) lum %.1f · %.1f from the ground "+
					"(loudest tile edge %.1f · gap %+.1f)", r.FiveColRule.R, r.FiveColRule.G,
					r.FiveColRule.B, r.FiveColRule.Lum, fiveGap, loudestTile, fiveGap-loudestTile)
				if fiveGap <= loudestTile {
					t.Errorf("the five-column rule sits %.1f from the ground and the loudest tile "+
						"edge sits %.1f. The counting aid reads as a counting aid ONLY because "+
						"it is a step above every other rule in the grid; at or below the tile's "+
						"own edge it is one more hairline among thirty, and 5+5 stops being "+
						"instant", fiveGap, loudestTile)
				}
			}

			for _, s := range r.Sel {
				if s.BorderPx.Lum < 0 {
					t.Errorf("era %s: the SELECTED edge resolved to no colour, so the arm below "+
						"proved nothing", s.Hue)
					continue
				}
				selGap := cpGroundGap(s.BorderPx, r.Ground)
				t.Logf("selected edge on era %s: %s rgb(%.0f,%.0f,%.0f) · %.1f from the ground "+
					"(loudest tile edge %.1f · gap %+.1f)", s.Hue, s.Border,
					s.BorderPx.R, s.BorderPx.G, s.BorderPx.B, selGap, loudestTile,
					selGap-loudestTile)
				if selGap <= loudestTile {
					t.Errorf("era %s: the SELECTED tile's edge sits %.1f from the ground and the "+
						"resting tile edge sits %.1f. Selection is IDENTITY and is allowed to be "+
						"loud — it is the one thing in the grid that may be — so softening the "+
						"resting edge must never be done by softening this one too. --accent at "+
						"full strength, always", s.Hue, selGap, loudestTile)
				}
			}

			// ── THE EDGE CARRIES THE ERA'S HUE, AND THAT IS WHY IT IS
			//    DERIVED FROM --cellbase RATHER THAN FROM THE GROUND ───────
			//
			// A tint on the fill inside a grey box reads as a coloured field
			// in a frame; a tint on the fill AND the edge reads as one soft
			// object. Deriving --tile-rule from --cellbase gets that for free
			// and is the reason the derivation is written that way — so it is
			// asserted, or the next hand "simplifies" it back to a flat token
			// and loses the property with no test to say so.
			for _, e := range r.Eras {
				if e.Rule.Lum < 0 {
					continue
				}
				fillCast := e.Fill.R - e.Fill.B
				ruleCast := e.Rule.R - e.Rule.B
				t.Logf("era %s: fill cast %+.1f (R-B) · edge cast %+.1f", e.Hue, fillCast, ruleCast)
				if fillCast*ruleCast <= 0 {
					t.Errorf("era %s: the tile's FILL casts %+.1f and its EDGE casts %+.1f — the "+
						"edge does not carry the era's hue. --tile-rule is derived from "+
						"--cellbase precisely so a tinted tile is one soft object rather than a "+
						"coloured field inside a grey box; an edge on a flat token puts the box "+
						"back", e.Hue, fillCast, ruleCast)
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
