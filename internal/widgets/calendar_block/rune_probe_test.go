// rune_probe_test.go — C-CALV4-TILES §9.1/§9.2/§9.3, MEASURED IN A BROWSER.
//
// WHY THESE THREE CLAIMS NEEDED A BROWSER AND NOT A STRING.
//
//	· "the hover expansion grows RIGHTWARD, so the primary disc moves 0.00px"
//	  is a claim about flex layout under a state change. The stylesheet says
//	  `left: 4px` and the old one said `right: 4px`; both are perfectly valid
//	  CSS and only the engine can say which one displaces the disc the pointer
//	  is already on. It was 35.05px before this change, and the number in the
//	  dispatch was not re-derived here — it is re-measured, on the old anchor
//	  and the new one, by this file.
//
//	· "at named density the cell shows chips and NO runes, below it the runes
//	  replace the bars" is §9.3, and it is enforced by which SUBTREE the rune
//	  block sits in. A `display: none` ancestor is invisible to every string
//	  assertion in this package and to `getComputedStyle` on the element
//	  itself, which happily reports a 9px inline-size for a box that does not
//	  exist.
//
//	· "the rune ink is NOT the raw --ev-* value" is a claim about a colour the
//	  sheet never writes down: `oklch(from var(--axis) 0.36 0.10 h)` is
//	  computed against whatever hue the producer stamped on that mark. Only the
//	  engine resolves it, and the whole point of the deepening is that the
//	  result must MEASURABLY differ from the token it was derived from.
//
// THE NARROW ARM DECODES PIXELS, following tile_paint_probe_test.go's rule and
// for its reason: a masked element with a resolved background-colour and a rect
// with area can still paint nothing, if the mask resolves to a shape with no
// coverage or the paint composites to the surface under it. "A corner is proved
// by a RECT WITH AREA and by a PAINTED PIXEL" — this file does both, and says
// which arm proved what.
//
// IT SKIPS HONESTLY under -short or with no Chromium, and a skipped run is NOT
// a pass. All three are registered in tools/check-browser-probes.sh.
//
//	go test ./internal/widgets/calendar_block/ -run RuneProbe -v
package calendar_block

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ── what one host reports ───────────────────────────────────────────────────

// rpSwatch is a colour read back out of the layout: the string the browser
// computed and the sRGB it paints. Resolved through a canvas rather than a
// parser of our own, for the reason cell_probe_test.go gives at length — the
// sheet speaks oklch and relative colour syntax, and a second implementation of
// colour is a second thing to be wrong in.
type rpSwatch struct {
	CSS string  `json:"css"`
	R   float64 `json:"r"`
	G   float64 `json:"g"`
	B   float64 `json:"b"`
	Lum float64 `json:"lum"`
}

// rpInk pairs one rune's resolved ink with the RAW axis token it was derived
// from, read off the same element in the same pass. Pairing them here rather
// than comparing two separate readings is what makes "the ink is not the raw
// palette" a per-mark claim: two marks of different types must each deepen
// their own hue, and a build that deepened one and passed the other through
// would average out across the month.
type rpInk struct {
	Pattern string   `json:"pattern"`
	Axis    rpSwatch `json:"axis"`
	Ink     rpSwatch `json:"ink"`
	Masked  bool     `json:"masked"`
	Gold    bool     `json:"gold"`
}

// rpSeg is one rune's box plus what it needs to count as painted.
type rpSeg struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	W      float64 `json:"w"`
	H      float64 `json:"h"`
	Masked bool    `json:"masked"`
	Inked  bool    `json:"inked"`
}

type rpReading struct {
	Label   string  `json:"label"`
	Theme   string  `json:"theme"`
	Host    float64 `json:"host"`
	ColumnW float64 `json:"columnW"`

	// ── §9.3, THE DENSITY RULING ──────────────────────────────────────────
	UnderShown bool `json:"underShown"`
	NamedShown bool `json:"namedShown"`
	ChipCells  int  `json:"chipCells"`
	// RuneBoxes counts `.ulseg` elements WITH A RECT. At named density the
	// whole `.cunder` subtree is display:none, so this must be 0 — not "0px
	// wide", not "transparent": absent.
	RuneCells  int     `json:"runeCells"`
	RuneBoxes  int     `json:"runeBoxes"`
	RuneMasked int     `json:"runeMasked"`
	RuneInked  int     `json:"runeInked"`
	RuneW      float64 `json:"runeW"`
	RuneH      float64 `json:"runeH"`
	// The worst rune/date and rune/cell relationships in the month. A rune
	// printing through the numeral is the mini tier's own failure mode and the
	// reason that tier gets its own size step.
	RuneOnDate float64 `json:"runeOnDate"` // px² of overlap with the numeral's INK
	RuneSpill  float64 `json:"runeSpill"`  // px the block leaves its own cell by
	// One marked cell's block and the first rune in it, for the pixel pass.
	ShotCell rpSeg   `json:"shotCell"`
	ShotRune rpSeg   `json:"shotRune"`
	Segs     []rpSeg `json:"segs"`

	// ── §9.2, THE INK ─────────────────────────────────────────────────────
	Inks []rpInk `json:"inks"`

	// ── §9.1, THE EXPANSION ───────────────────────────────────────────────
	//
	// RestDiscX / OpenDiscX are the PRIMARY disc's left edge before and after
	// the cluster's own radio takes focus. OpenDiscs proves the expansion
	// actually happened: a build where focus did nothing would report a
	// displacement of 0.00px for the wrong reason, which is the shape of
	// tautology this package's probes exist to refuse.
	HasCluster bool    `json:"hasCluster"`
	FocusOK    bool    `json:"focusOK"`
	RestDiscs  int     `json:"restDiscs"`
	OpenDiscs  int     `json:"openDiscs"`
	RestDiscX  float64 `json:"restDiscX"`
	OpenDiscX  float64 `json:"openDiscX"`
	RestRowW   float64 `json:"restRowW"`
	OpenRowW   float64 `json:"openRowW"`
	// …and the same measurement with the anchor put back the way it was, in
	// the same engine, in the same pass. Without it "0.00px" is a number with
	// nothing to be better than.
	OldDiscShift float64 `json:"oldDiscShift"`
}

// rpScript is the whole measurement, run once per host.
const rpScript = `
function(host){
  var root = host.querySelector('.cal-block-host') || host;
  var r2 = function(v){ return Math.round(v * 100) / 100 };
  var box = function(el){
    if (!el) return null;
    var r = el.getBoundingClientRect();
    return { l: r.left, t: r.top, w: r.width, h: r.height, r: r.right, b: r.bottom };
  };
  var vis = function(b){ return !!b && b.w > 0 && b.h > 0 };
  var seg = function(b, masked, inked){
    return b ? { x: r2(b.l), y: r2(b.t), w: r2(b.w), h: r2(b.h), masked: !!masked, inked: !!inked }
             : { x: 0, y: 0, w: 0, h: 0, masked: false, inked: false };
  };
  var cv = document.createElement('canvas'); cv.width = cv.height = 4;
  var cx = cv.getContext('2d', { willReadFrequently: true });
  var over = function(base, top){
    cx.clearRect(0, 0, 4, 4);
    if (base) { cx.fillStyle = base; cx.fillRect(0, 0, 4, 4) }
    if (top)  { cx.fillStyle = top;  cx.fillRect(0, 0, 4, 4) }
    var d = cx.getImageData(1, 1, 1, 1).data;
    return { css: top || base || '', r: d[0], g: d[1], b: d[2],
             lum: r2(0.2126 * d[0] + 0.7152 * d[1] + 0.0722 * d[2]) };
  };
  // A CUSTOM PROPERTY IS NOT A COLOUR UNTIL SOMETHING PAINTS IT. Reading
  // --axis off getComputedStyle returns the token TEXT (an oklch() string that
  // may itself be a var() chain), so it is pushed through a live element's
  // background-color and read back — the same route the ink itself takes, so
  // the two readings are comparable by construction.
  var resolveOn = function(el, expr){
    var probe = document.createElement('i');
    probe.style.position = 'absolute';
    probe.style.backgroundColor = expr;
    el.appendChild(probe);
    var css = getComputedStyle(probe).backgroundColor;
    probe.remove();
    return over(css, null);
  };
  // The numeral's INK, not its box: .dn is a full-width block whose text is
  // merely aligned inside it, so a box-vs-box test reports a collision with
  // everything in the cell at every width (moon_reach_probe's own finding).
  var numInk = function(cl){
    var named = cl.querySelector('.cnamed'), under = cl.querySelector('.cunder');
    var n = null;
    if (named && vis(box(named))) n = named.querySelector('.dn');
    else if (under && vis(box(under))) n = under.querySelector('.dn');
    if (!n) return null;
    var rg = document.createRange(); rg.selectNodeContents(n);
    var rr = rg.getBoundingClientRect();
    return { l: rr.left, t: rr.top, w: rr.width, h: rr.height, r: rr.right, b: rr.bottom };
  };
  var areaOf = function(a, b){
    if (!vis(a) || !vis(b)) return 0;
    var w = Math.max(0, Math.min(a.r, b.r) - Math.max(a.l, b.l));
    var h = Math.max(0, Math.min(a.b, b.b) - Math.max(a.t, b.t));
    return w * h;
  };

  var out = {
    label: host.getAttribute('data-label') || '',
    theme: host.closest('.dark') ? 'dark' : 'light',
    host: r2(root.getBoundingClientRect().width), columnW: 0,
    underShown: false, namedShown: false, chipCells: 0,
    runeCells: 0, runeBoxes: 0, runeMasked: 0, runeInked: 0, runeW: 0, runeH: 0,
    runeOnDate: 0, runeSpill: 0,
    shotCell: seg(null), shotRune: seg(null), segs: [], inks: [],
    hasCluster: false, focusOK: false, restDiscs: 0, openDiscs: 0,
    restDiscX: 0, openDiscX: 0, restRowW: 0, openRowW: 0, oldDiscShift: -1
  };

  var cells = [].slice.call(root.querySelectorAll('.cell[data-day]'));
  if (!cells.length) return out;
  var cs0 = getComputedStyle(cells[0]);
  var c0 = box(cells[0]);
  out.columnW = r2(c0.w - parseFloat(cs0.borderLeftWidth) - parseFloat(cs0.borderRightWidth));
  out.underShown = vis(box(cells[0].querySelector('.cunder')));
  out.namedShown = vis(box(cells[0].querySelector('.cnamed')));

  // ── §9.3 · WHICH MARK THE CELL DRAWS, WALKED OVER THE WHOLE MONTH ───────
  var inkSeen = {};
  for (var n = 0; n < cells.length; n++) {
    var cell = cells[n], cb = box(cell);
    if (cell.querySelector('.cnamed .chip')) out.chipCells++;
    var ul = cell.querySelector('.ul');
    if (!ul) continue;
    out.runeCells++;
    var ub = box(ul), nb = numInk(cell);
    if (vis(ub)) {
      out.runeOnDate = Math.max(out.runeOnDate, r2(areaOf(ub, nb)));
      var sx = Math.max(0, cb.l - ub.l) + Math.max(0, ub.r - cb.r);
      var sy = Math.max(0, cb.t - ub.t) + Math.max(0, ub.b - cb.b);
      out.runeSpill = Math.max(out.runeSpill, r2(Math.max(sx, sy)));
    }
    var segs = [].slice.call(ul.querySelectorAll('.ulseg'));
    for (var s = 0; s < segs.length; s++) {
      var el = segs[s], sb = box(el), st = getComputedStyle(el);
      var masked = st.maskImage !== 'none' && st.maskImage !== '' ||
                   st.webkitMaskImage !== 'none' && st.webkitMaskImage !== '';
      var px = over('rgb(0,0,0)', st.backgroundColor);
      var inked = vis(sb) && (px.r + px.g + px.b) > 0;
      if (vis(sb)) {
        out.runeBoxes++;
        out.runeW = r2(sb.w); out.runeH = r2(sb.h);
        if (masked) out.runeMasked++;
        if (inked) out.runeInked++;
        if (out.segs.length < 12) out.segs.push(seg(sb, masked, inked));
        if (!out.shotRune.w) { out.shotRune = seg(sb, masked, inked); out.shotCell = seg(cb, false, false) }
      }
      // ── §9.2 · THE INK, AGAINST THE RAW TOKEN ON THE SAME ELEMENT ─────
      var cls = (el.getAttribute('class') || '');
      var pat = (cls.match(/\bp[1-8]\b/) || ['none'])[0];
      var isGold = !!cell.querySelector(':scope > .dogear');
      var key = pat + (isGold ? '-gold' : '');
      if (!inkSeen[key] && vis(sb)) {
        inkSeen[key] = true;
        out.inks.push({
          pattern: pat, masked: masked, gold: isGold,
          axis: resolveOn(el, isGold ? 'var(--gold)' : 'var(--axis)'),
          ink: over(st.backgroundColor, null)
        });
      }
    }
  }

  // ── §9.1 · THE EXPANSION, AND WHAT THE OLD ANCHOR COST ──────────────────
  //
  // NO HOVER IS SYNTHESISED. The sheet gives the expansion two triggers in ONE
  // declaration block — .phctl:hover and the cluster radio's :focus-visible
  // — and focus is the half a script can drive honestly. Measured on a cell
  // whose column clears the 75px expansion rung; below it nothing expands and
  // there is nothing to displace.
  var ctlCell = null;
  for (var k = 0; k < cells.length && !ctlCell; k++) {
    if (cells[k].querySelector('.moonpick') && cells[k].querySelector('.ph')) ctlCell = cells[k];
  }
  if (ctlCell) {
    var row = ctlCell.querySelector('.phrow');
    var radio = ctlCell.querySelector('.moonpick');
    var discsOf = function(){
      return [].slice.call(row.querySelectorAll('.ph')).filter(function(d){ return vis(box(d)) });
    };
    out.hasCluster = true;
    var rest = discsOf();
    out.restDiscs = rest.length;
    if (rest.length) out.restDiscX = r2(box(rest[0]).l);
    out.restRowW = r2(vis(box(row)) ? box(row).w : 0);
    radio.focus();
    out.focusOK = radio.matches(':focus-visible');
    var open = discsOf();
    out.openDiscs = open.length;
    if (open.length) out.openDiscX = r2(box(open[0]).l);
    out.openRowW = r2(vis(box(row)) ? box(row).w : 0);

    // THE CONTROL ARM: the SAME cluster, in the SAME engine, with the anchor
    // put back to what it was one change ago. Without it "0.00px" is a number
    // with nothing to be better than, and a build that broke the expansion
    // entirely would report the same 0.00 and read as a pass.
    if (open.length) {
      var before = getComputedStyle(row);
      row.style.left = 'auto';
      row.style.right = before.left;   // mirror the same inset onto the other edge
      var oldOpen = discsOf();
      var oldOpenX = oldOpen.length ? box(oldOpen[0]).l : 0;
      radio.blur();
      var oldRest = discsOf();
      var oldRestX = oldRest.length ? box(oldRest[0]).l : 0;
      out.oldDiscShift = r2(Math.abs(oldOpenX - oldRestX));
      row.style.left = ''; row.style.right = '';
    }
    radio.blur();
  }
  return out;
}`

// ── the rig ─────────────────────────────────────────────────────────────────

// rpCase is one host: a width, a theme and a label.
type rpCase struct {
	label string
	width int
	dark  bool
}

// rpCases spans the two sides of the 84px named-density flip AND the mini tier,
// in both themes.
//
// THE MINI HOST IS NOT DECORATION. §DENSITY forces `.cunder` to 26px there, so
// the rune block has a 9px band between the numeral's foot and the availability
// reservation and its own size step to fit it. That step was chosen off this
// measurement; without a case here it would be a number in a comment.
//
// NOR IS THE 1150px ONE. The runes have THREE sizes and the two obvious hosts
// only reach two of them: the phone's 9×12 and the mini tier's 6×9. Between the
// 75px expansion rung and the 84px named flip the cell still draws runes and
// draws them at the DESKTOP 13×17, one row, no wrap — a nine-pixel band of
// column width that no other case in this package lands in, and the only place
// the `max-inline-size: none` that unwraps them can be wrong. Measured: a
// 1150px host gives a 80.4px column (1100 → 75.4, 1200 → 85.4 and flips to
// chips), so 1150 sits in the middle of the band rather than on either edge.
func rpCases() []rpCase {
	return []rpCase{
		{"light · 366px host · narrow density", 366, false},
		{"dark · 366px host · narrow density", 366, true},
		{"light · 1150px host · narrow density, desktop runes", 1150, false},
		{"light · 1400px host · named density", 1400, false},
		{"dark · 1400px host · named density", 1400, true},
		{"light · 280px host · mini tier", 280, false},
	}
}

func rpRun(t *testing.T, chrome string, cases []rpCase) []rpReading {
	t.Helper()
	css := blockCSS(t)

	var boxes strings.Builder
	for i, c := range cases {
		d := fxAlmanac(t, true)
		// Each host gets its own slug: every id and radio-group NAME is derived
		// from it, so several copies on one page would share one group and only
		// one radio could ever be checked (moon_reach_probe's finding).
		d.CalendarSlug = fmt.Sprintf("%s-r%d", d.CalendarSlug, i)
		markup := cpLinkRe.ReplaceAllString(render(t, d), "")
		open, closeTag := "", ""
		if c.dark {
			open, closeTag = `<div class="dark">`, `</div>`
		}
		fmt.Fprintf(&boxes,
			`%s<div class="probe-host" data-label="%s" style="width:%dpx">%s</div>%s`,
			open, html.EscapeString(c.label), c.width, markup, closeTag)
	}

	page := `<!doctype html><html><head><meta charset="utf-8"><style>` +
		`html,body{margin:0;background:#888}` +
		`.probe-host{display:block;margin:24px}` +
		css + `</style></head><body>` + boxes.String() +
		`<script>document.addEventListener('DOMContentLoaded',function(){` +
		`var read=` + rpScript + `;` +
		`var out=[].slice.call(document.querySelectorAll('.probe-host')).map(read);` +
		`document.body.setAttribute('data-probe', JSON.stringify(out));});</script>` +
		`</body></html>`

	path := filepath.Join(t.TempDir(), "runeprobe.html")
	if err := os.WriteFile(path, []byte(page), 0o644); err != nil { //nolint:gosec // test artefact
		t.Fatalf("write probe page: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
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
	var readings []rpReading
	if err := json.Unmarshal([]byte(html.UnescapeString(m[1])), &readings); err != nil {
		t.Fatalf("probe payload: %v", err)
	}
	if len(readings) != len(cases) {
		t.Fatalf("probe returned %d readings for %d cases", len(readings), len(cases))
	}
	return readings
}

// ── §9.1 · THE CORNER MOVE ──────────────────────────────────────────────────

// TestRuneProbe_TheExpansionGrowsRightward is C-CALV4-TILES §9.1's first line,
// measured: "Hover grows the cluster RIGHTWARD, so the disc under the pointer
// moves 0.00px (it moved 35.05px when anchored right)."
//
// THIS IS A MOTION CLAIM IN A SHEET WITH A ZERO MOTION BUDGET. Nothing here
// transitions or animates — TestCSS_NoMotionAtAll sees to that — but a flex row
// anchored by its RIGHT edge that grows from one item to four moves every item
// in it, instantly, at the moment a pointer arrives. That is displacement
// without a transition, which is the loophole a `transition`-grep cannot close,
// and it lands on the one element C-CALV4-SPEC §4 makes unconditional.
//
// THE CONTROL ARM IS THE POINT. Measuring the new anchor alone would report
// 0.00px on a build where the expansion had stopped working entirely. So the
// same cluster is re-anchored to its old edge in the same pass and re-measured;
// the old number has to be LARGE for the new one's 0.00 to mean anything.
func TestRuneProbe_TheExpansionGrowsRightward(t *testing.T) {
	// THE GATE IS INLINE, NOT IN A HELPER: tools/check-browser-probes.sh takes
	// its census by looking for a Chromium finder inside each top-level Test
	// function's body, so a probe that reached the browser through a private
	// helper would be invisible to it — neither found nor demanded.
	if testing.Short() {
		t.Skip("browser probe: skipped under -short (CI's mode) — a skipped run is NOT a pass")
	}
	chrome := findProbeChromium()
	if chrome == "" {
		t.Skip("browser probe: no Chromium binary found")
	}
	readings := rpRun(t, chrome, rpCases())

	tested := 0
	for i, c := range rpCases() {
		r := readings[i]
		// The expansion has its own 75px rung; below it the cluster shows the
		// primary alone and there is nothing to displace.
		if r.ColumnW < MoonExpandColWidthMin {
			t.Logf("%s: column %.1fpx is under the %.0fpx expansion rung — no expansion here",
				c.label, r.ColumnW, MoonExpandColWidthMin)
			continue
		}
		t.Run(c.label, func(t *testing.T) {
			tested++
			if !r.HasCluster {
				t.Fatal("no cell carries a moon cluster with a radio — this arm has no subject")
			}
			t.Logf("column %.1fpx · discs %d→%d · row %.2f→%.2fpx · primary x %.2f→%.2f · "+
				":focus-visible %v", r.ColumnW, r.RestDiscs, r.OpenDiscs, r.RestRowW, r.OpenRowW,
				r.RestDiscX, r.OpenDiscX, r.FocusOK)

			if !r.FocusOK {
				t.Fatal("the cluster radio did not take :focus-visible, so the expansion never " +
					"fired and the displacement below would be 0.00px for the wrong reason. " +
					"Chromium's :focus-visible heuristic is stateful across the document — if " +
					"this starts failing, the page needs a keyboard interaction before the " +
					"first focus() rather than a looser assertion here")
			}
			if r.OpenDiscs <= r.RestDiscs {
				t.Fatalf("the cluster shows %d discs at rest and %d on focus. Nothing expanded, "+
					"so there was nothing to displace and the 0.00px below is a tautology",
					r.RestDiscs, r.OpenDiscs)
			}
			if r.OpenRowW <= r.RestRowW {
				t.Errorf("the cluster's box is %.2fpx at rest and %.2fpx open — it did not grow, "+
					"so the direction it grew in is not a question this reading can answer",
					r.RestRowW, r.OpenRowW)
			}

			// THE CLAIM: the disc already under the pointer does not move.
			shift := r.OpenDiscX - r.RestDiscX
			if shift < 0 {
				shift = -shift
			}
			t.Logf("primary silhouette displacement on expansion: %.2fpx "+
				"(old right-anchored construction, same engine, same pass: %.2fpx)",
				shift, r.OldDiscShift)
			if shift > 0.01 {
				t.Errorf("the primary silhouette moves %.2fpx when the cluster expands. It is "+
					"anchored LEFT precisely so the row grows into the cell's middle and the "+
					"disc the pointer is already on stays where it is; a right anchor grows "+
					"leftward and displaces it. Nothing in this sheet may move", shift)
			}
			// AND THE OLD ANCHOR STILL COSTS WHAT IT COST. If this ever reads
			// ~0 too, the control has stopped being a control and the arm above
			// is passing on a broken expansion.
			if r.OldDiscShift < 1 {
				t.Errorf("re-anchoring the same cluster to its old RIGHT edge displaces the "+
					"primary disc by only %.2fpx. The control arm exists so that 0.00px on the "+
					"new anchor means something; a control that measures nothing means the "+
					"expansion itself is broken and both readings are vacuous", r.OldDiscShift)
			}
		})
	}
	if tested == 0 {
		t.Fatal("no host in the census cleared the expansion rung — every arm was skipped and " +
			"this probe measured nothing")
	}
}

// ── §9.3 · THE DENSITY RULING ───────────────────────────────────────────────

// TestRuneProbe_TheDensityRuling is C-CALV4-TILES §9.3, measured on both sides
// of the flip: "a name beats a rune wherever there is room for one. At named
// density the cell keeps its chips and draws no runes; below it, runes replace
// the underline bars."
//
// WHY THE NAMED ARM ASSERTS ABSENCE OF A *BOX* AND NOT ABSENCE OF PAINT. The
// rune block lives inside `.cunder`, which is `display: none` at 84px and up.
// That is stronger than "not painting": the element has no rect, no layout and
// nothing to hide. An implementation that instead hid the runes with
// `opacity: 0` or a transparent ink would pass a paint test and still be wrong
// — the marks would be in the accessibility tree, in the hit region, and one
// cascade change from visible.
//
// AND THE NARROW ARM DECODES PIXELS, in TestRuneProbe_TheRunesAreOnTheScreen.
// Split into two functions rather than one because the guard's census demands a
// named PASS line per probe, and "the ruling holds" and "the ink reaches the
// screen" fail for different reasons and should be readable apart.
func TestRuneProbe_TheDensityRuling(t *testing.T) {
	if testing.Short() {
		t.Skip("browser probe: skipped under -short (CI's mode) — a skipped run is NOT a pass")
	}
	chrome := findProbeChromium()
	if chrome == "" {
		t.Skip("browser probe: no Chromium binary found")
	}
	readings := rpRun(t, chrome, rpCases())

	sawNamed, sawNarrow := false, false
	for i, c := range rpCases() {
		r := readings[i]
		t.Run(c.label, func(t *testing.T) {
			t.Logf("column %.1fpx · cunder %v · cnamed %v · %d chip cells · %d cells with a "+
				"rune block · %d rune boxes (%d masked, %d inked) at %.2f×%.2fpx",
				r.ColumnW, r.UnderShown, r.NamedShown, r.ChipCells, r.RuneCells,
				r.RuneBoxes, r.RuneMasked, r.RuneInked, r.RuneW, r.RuneH)

			if r.NamedShown {
				sawNamed = true
				// ── THE NAMED CELL KEEPS ITS CHIPS ────────────────────────
				if r.ChipCells == 0 {
					t.Fatal("named density and no cell carries a chip. §9.3's ruling is that a " +
						"NAME beats a rune; with neither drawn the cell's middle is blank and " +
						"the rune arm below would pass for the wrong reason")
				}
				// ── AND DRAWS NO RUNES AT ALL ─────────────────────────────
				if r.RuneBoxes != 0 {
					t.Errorf("%d rune boxes have a rect at named density. §9.3: at ≥84px of "+
						"column the cell keeps its event-NAME chips and draws NO runes — every "+
						"desktop render shown during design had the chips stripped by the "+
						"harness, which is why the runes looked like they belonged there. The "+
						"gate is `.cunder`'s own `display: none`; a box here means the block "+
						"was moved out of that subtree", r.RuneBoxes)
				}
				return
			}

			// ── BELOW THE FLIP, THE RUNES REPLACE THE BARS ────────────────
			sawNarrow = true
			if r.RuneCells == 0 {
				t.Fatal("no cell carries a rune block — the fixture has marks on eight days, " +
					"so every assertion below is vacuous")
			}
			if r.RuneBoxes == 0 {
				t.Fatal("the narrow cell draws no rune with a rect. This is the cell's most " +
					"important content at the only width the operator's own calendar produces")
			}
			if r.RuneMasked != r.RuneBoxes {
				t.Errorf("%d of %d runes resolve no mask. An unmasked `.ulseg` does not fail "+
					"visibly — it paints its whole box in the type's ink and reads as a "+
					"deliberate solid mark", r.RuneBoxes-r.RuneMasked, r.RuneBoxes)
			}
			if r.RuneInked != r.RuneBoxes {
				t.Errorf("%d of %d runes carry no ink. A rect with area and a transparent fill "+
					"is still an empty middle", r.RuneBoxes-r.RuneInked, r.RuneBoxes)
			}
			// ── AND THE BLOCK STAYS OFF THE DATE AND INSIDE ITS OWN CELL ──
			//
			// The mini tier is why this is measured rather than reasoned: it
			// forces `.cunder` to 26px, so the band between the numeral's foot
			// and the availability reservation is 9px and the baseline's 12px
			// rune would print through the date. The tier's own size step was
			// chosen off this number.
			t.Logf("worst rune/numeral overlap %.2fpx² · worst spill out of the cell %.2fpx",
				r.RuneOnDate, r.RuneSpill)
			if r.RuneOnDate > 0 {
				t.Errorf("the rune block overlaps the day numeral's ink by %.2fpx². Two marks "+
					"printing through each other is the corner architecture failing in the "+
					"middle of the cell instead of in a corner", r.RuneOnDate)
			}
			if r.RuneSpill > 0.01 {
				t.Errorf("the rune block leaves its own cell by %.2fpx — a mark on the wrong "+
					"day is worse than no mark", r.RuneSpill)
			}
		})
	}
	if !sawNamed || !sawNarrow {
		t.Fatalf("the census covered named=%v narrow=%v. §9.3 is a claim about BOTH sides of "+
			"the flip and one arm alone cannot express it", sawNamed, sawNarrow)
	}
}

// ── §9.2 · THE INK ──────────────────────────────────────────────────────────

// rpInkGapMin is how far the rune's ink must sit from the raw axis token it was
// derived from, as the largest single-channel distance in 0–255.
//
// IT IS READ OFF THE PALETTE AND OFF THE MEASUREMENT, not chosen to pass. The
// six --ev-* tokens sit at L 0.55–0.75; light ink is L 0.36 and dark ink L 0.72,
// with chroma pulled to 0.10/0.13. So the deepening moves a long way on LIGHT
// and only a little on DARK, where a token already near L 0.72 mostly just loses
// chroma. Measured on the fixture's own marks in this probe: 85–138/255 on
// light, 33–38/255 on dark. 12 sits under the smallest of those with room for
// the palette to be re-tuned, and far above the 0/255 a build that stopped
// deepening would report.
const rpInkGapMin = 12.0

// TestRuneProbe_TheRuneInkIsNotTheRawPalette is C-CALV4-TILES §9.2's "INK, NOT
// THE RAW PALETTE", measured in both themes.
//
// "The six --ev-* tokens sit at L 0.55–0.75, C 0.13–0.19 — correct for a 3px
// bar, primary-crayon at 13px as a letterform. The rune deepens its own axis,
// keeping only the hue."
//
// THE FAILURE THIS CATCHES IS SILENT IN BOTH DIRECTIONS. A build where
// --runeink stopped resolving falls back to `var(--rule-structural-strong)` and
// draws a month of correct-looking grey runes; a build where the deepening was
// removed draws correct-looking coloured ones. Neither changes a rect, a mask or
// a class, and neither can be seen from the stylesheet — the expression is
// `oklch(from var(--axis) …)` and only the engine resolves it.
//
// AND IT IS MEASURED PER MARK, not once per host. Two event types must each
// deepen their OWN hue; a build that deepened one and passed another through
// would average out across the month.
func TestRuneProbe_TheRuneInkIsNotTheRawPalette(t *testing.T) {
	if testing.Short() {
		t.Skip("browser probe: skipped under -short (CI's mode) — a skipped run is NOT a pass")
	}
	chrome := findProbeChromium()
	if chrome == "" {
		t.Skip("browser probe: no Chromium binary found")
	}
	readings := rpRun(t, chrome, rpCases())

	themes := map[string]int{}
	for i, c := range rpCases() {
		r := readings[i]
		if r.NamedShown {
			continue // no runes at named density, by §9.3 — measured next door
		}
		t.Run(c.label, func(t *testing.T) {
			if len(r.Inks) == 0 {
				t.Fatal("no rune ink could be read on this host — the fixture has marks on " +
					"eight days, so this arm has no subject")
			}
			themes[r.Theme]++
			for _, k := range r.Inks {
				gap := rpWorst(k.Ink, k.Axis)
				label := k.Pattern
				if k.Gold {
					label += " (GM-only, gold)"
				}
				t.Logf("%s: axis %s → ink %s · worst channel distance %.0f/255 · lum %.1f → %.1f",
					label, k.Axis.CSS, k.Ink.CSS, gap, k.Axis.Lum, k.Ink.Lum)
				if k.Ink.R+k.Ink.G+k.Ink.B == 0 {
					t.Errorf("%s inks to transparent black. --runeink did not resolve at all — "+
						"check that it is declared ON `.ulseg` and not on an ancestor: a custom "+
						"property inherits its COMPUTED value, so an `oklch(from var(--axis) …)` "+
						"declared where --axis does not exist computes to the "+
						"guaranteed-invalid value and every child inherits THAT", label)
				}
				if gap < rpInkGapMin {
					t.Errorf("%s inks %.0f/255 from its raw axis token, under the %.0f floor. "+
						"The rune must DEEPEN its axis rather than wear it: the --ev-* palette "+
						"is tuned for a 3px bar and reads primary-crayon at letterform size, "+
						"which is the operator's \"not pastel colors.. makes it look like "+
						"elementary\"", label, gap, rpInkGapMin)
				}
			}
			// THE GOLD CHANNEL IS PART OF THE SAME CLAIM. Permission rides with
			// the marks now (§8.1), so a GM-only day's runes must be gold AND
			// deepened — an undeepened gold glows out of the grid, which is the
			// failure the ink values were chosen against.
			gold := 0
			for _, k := range r.Inks {
				if k.Gold {
					gold++
				}
			}
			if gold == 0 {
				t.Error("no GM-only day's runes were measured. The GM fixture has three dm_only " +
					"days and the gold ink is the permission channel at this density — an arm " +
					"with no subject passes for the wrong reason")
			}

			// AND THE OVERFLOW TALLY IS NEUTRAL, ON A GM-ONLY DAY TOO.
			//
			// FOUND BY THIS PROBE, NOT BY READING. `.ulseg.rest` is the "there
			// are more" mark: no pattern class, no type, no permission — the
			// exact count lives in the Ledger. It inked NEUTRAL on light and
			// GOLD on dark, because `.dark .cal-block-host .cell:has(> .dogear)
			// .ulseg` carries one class more than the rule that makes it
			// neutral did. One mark with two meanings, split by theme, and
			// nothing in the stylesheet reads wrong.
			var rest, typed *rpInk
			for i := range r.Inks {
				k := &r.Inks[i]
				if !k.Gold {
					continue
				}
				if k.Pattern == "none" {
					rest = k
				} else if typed == nil {
					typed = k
				}
			}
			if rest != nil && typed != nil {
				t.Logf("on a GM-only day: typed rune %s vs overflow tally %s — %.0f/255 apart",
					typed.Ink.CSS, rest.Ink.CSS, rpWorst(typed.Ink, rest.Ink))
				if rpWorst(typed.Ink, rest.Ink) < rpInkGapMin {
					t.Errorf("the overflow tally inks %s, the same as this day's typed runes "+
						"(%s). `.rest` says THERE ARE MORE; it carries no type and no "+
						"permission, so a gold tally reads as a fourth GM-only event that does "+
						"not exist", rest.Ink.CSS, typed.Ink.CSS)
				}
			}
		})
	}
	// BOTH THEMES, ALWAYS. The direction inverts — light goes down away from the
	// ground, dark goes up — and a single-arm test would be green on whichever
	// arm happened to work.
	if themes["light"] == 0 || themes["dark"] == 0 {
		t.Fatalf("the census measured light=%d dark=%d narrow hosts. The ink inverts with the "+
			"theme and one arm cannot express that", themes["light"], themes["dark"])
	}
}

// rpWorst is the largest single-channel distance between two swatches, in
// 0–255 — the same measure every other colour claim in this package is made in.
func rpWorst(a, b rpSwatch) float64 {
	d := func(x, y float64) float64 {
		if x > y {
			return x - y
		}
		return y - x
	}
	m := d(a.R, b.R)
	if v := d(a.G, b.G); v > m {
		m = v
	}
	if v := d(a.B, b.B); v > m {
		m = v
	}
	return m
}

// ── §9.2 · AND THE INK REACHES THE SCREEN ───────────────────────────────────

// TestRuneProbe_TheRunesAreOnTheScreen decodes pixels, and it exists because
// every other arm in this file reads the CASCADE.
//
// tile_paint_probe_test.go's whole reason for being is that
// `getComputedStyle` answers what the cascade RESOLVED and not what the
// compositor DREW: a stray `*/` once swallowed `.cal-block-host .cell` entire
// and a probe measuring computed colours reported a healthy band across a grid
// that was painting nothing. A masked element is a second way to be wrong in
// the same direction — the ink can resolve perfectly and the mask can clip it
// to no coverage at all, and the reading is identical.
//
// SO THIS ONE LOOKS. It screenshots the narrow host, walks a 1px horizontal
// strip through the middle of one rune's own rect, and requires pixels that are
// neither the tile fill behind them nor the page ground. A rune is proved by a
// RECT WITH AREA (next door) and by a PAINTED PIXEL (here).
func TestRuneProbe_TheRunesAreOnTheScreen(t *testing.T) {
	if testing.Short() {
		t.Skip("browser probe: skipped under -short (CI's mode) — a skipped run is NOT a pass")
	}
	chrome := findProbeChromium()
	if chrome == "" {
		t.Skip("browser probe: no Chromium binary found")
	}

	for _, c := range []rpCase{
		{"light · 366px host · narrow density", 366, false},
		{"dark · 366px host · narrow density", 366, true},
	} {
		t.Run(c.label, func(t *testing.T) {
			img, r := rpShoot(t, chrome, c)
			if r.ShotRune.W <= 0 {
				t.Fatal("the DOM pass found no rune with a rect to screenshot — this arm has " +
					"no subject")
			}
			t.Logf("rune rect %.2f×%.2fpx at (%.2f,%.2f) inside a cell at (%.2f,%.2f)",
				r.ShotRune.W, r.ShotRune.H, r.ShotRune.X, r.ShotRune.Y,
				r.ShotCell.X, r.ShotCell.Y)

			// THE REFERENCE IS THE TILE'S OWN FILL, sampled from the same cell
			// just inside its top-left corner and well clear of the numeral, the
			// cluster and the rune band. Comparing against a hardcoded colour
			// would make this a test of the palette; comparing against the
			// surface the rune is drawn ON is the actual claim.
			refX := int(r.ShotCell.X + 4)
			refY := int(r.ShotCell.Y + r.ShotCell.H*0.55)
			ref := tpAt(t, img, refX, refY)

			y := int(r.ShotRune.Y + r.ShotRune.H/2)
			x0 := int(r.ShotRune.X)
			x1 := int(r.ShotRune.X + r.ShotRune.W + 0.5)
			ink, best := 0, 0
			for x := x0; x < x1; x++ {
				if d := tpAt(t, img, x, y).worst(ref); d > best {
					best = d
				}
				if tpAt(t, img, x, y).worst(ref) > 8 {
					ink++
				}
			}
			t.Logf("strip y=%d across x=%d..%d against the tile fill %s: %d/%d pixels differ, "+
				"worst channel distance %d/255", y, x0, x1, ref, ink, x1-x0, best)
			if ink == 0 {
				t.Errorf("not one pixel across the rune's own rect differs from the tile fill "+
					"it is drawn on (%s). The box has area and the cascade resolved an ink; "+
					"neither of those is paint. Either the mask clipped the glyph to no "+
					"coverage, or the ink composited to the surface under it", ref)
			}
		})
	}
}

// rpShoot renders one host, screenshots it, and returns the decoded image with
// the geometry the DOM pass reported.
//
// TWO CHROMIUM INVOCATIONS OVER THE SAME FILE, because --dump-dom and
// --screenshot cannot both be asked of one run; the page has no scripted
// animation and no clock, so the two agree. deviceScaleFactor 1 so one CSS
// pixel is one image pixel and the strip needs no arithmetic to line up.
func rpShoot(t *testing.T, chrome string, c rpCase) (image.Image, rpReading) {
	t.Helper()
	css := blockCSS(t)

	d := fxAlmanac(t, true)
	markup := cpLinkRe.ReplaceAllString(render(t, d), "")
	open, closeTag := "", ""
	if c.dark {
		open, closeTag = `<div class="dark">`, `</div>`
	}
	// THE PAGE GROUND IS MAGENTA, for tile_paint_probe's reason: if the Block
	// ever fails to paint its own ground a neutral page supplies a
	// plausible-looking grey and the probe measures the harness.
	page := `<!doctype html><html><head><meta charset="utf-8"><style>` +
		`html,body{margin:0;background:#ff00ff}` +
		`.probe-host{display:block;margin:20px}` +
		css + `</style></head><body>` +
		fmt.Sprintf(`%s<div class="probe-host" data-label="%s" style="width:%dpx">%s</div>%s`,
			open, html.EscapeString(c.label), c.width, markup, closeTag) +
		`<script>document.addEventListener('DOMContentLoaded',function(){` +
		`var read=` + rpScript + `;` +
		`document.body.setAttribute('data-probe', JSON.stringify(` +
		`read(document.querySelector('.probe-host'))));});</script>` +
		`</body></html>`

	dir := t.TempDir()
	path := filepath.Join(dir, "runepaint.html")
	if err := os.WriteFile(path, []byte(page), 0o644); err != nil { //nolint:gosec // test artefact
		t.Fatalf("write probe page: %v", err)
	}

	args := []string{
		"--headless", "--no-sandbox", "--disable-gpu", "--hide-scrollbars",
		"--window-size=1600,1200", "--virtual-time-budget=6000",
		"--force-device-scale-factor=1",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	dom, err := exec.CommandContext(ctx, chrome, append(append([]string{}, args...),
		"--dump-dom", "file://"+path)...).Output()
	if err != nil {
		t.Fatalf("chromium (dom pass): %v", err)
	}
	m := probePayloadRe.FindStringSubmatch(string(dom))
	if m == nil {
		t.Fatal("no probe payload in the rendered DOM — the page script did not run")
	}
	var r rpReading
	if err := json.Unmarshal([]byte(html.UnescapeString(m[1])), &r); err != nil {
		t.Fatalf("probe payload: %v", err)
	}

	shot := filepath.Join(dir, "runepaint.png")
	if _, err := exec.CommandContext(ctx, chrome, append(append([]string{}, args...),
		"--screenshot="+shot, "file://"+path)...).Output(); err != nil {
		t.Fatalf("chromium (screenshot pass): %v", err)
	}
	f, err := os.Open(shot) //nolint:gosec // test artefact in t.TempDir()
	if err != nil {
		t.Fatalf("open screenshot: %v", err)
	}
	defer f.Close() //nolint:errcheck // test artefact
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("decode screenshot: %v", err)
	}
	return img, r
}
