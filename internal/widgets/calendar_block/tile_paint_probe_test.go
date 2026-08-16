// tile_paint_probe_test.go — IS THE TILE ACTUALLY ON THE SCREEN?
//
// WHY THIS FILE EXISTS, AND WHAT IT CAUGHT.
//
// C-CALV4-TILES rebuilt the day cell as a rounded tile drawn on `.cell::before`
// and softened its edge, and shipped with a browser probe measuring that edge —
// TestCellProbe_TheTileRuleIsTheQuietestRuleInTheGrid — which passed on all four
// hosts, in both themes, logging a healthy 12.9–28.1 band.
//
// The grid was drawing NOTHING. A stray `*/` had orphaned a comment paragraph,
// the orphan was parsed as a selector prelude, and it swallowed the whole
// `.cal-block-host .cell` rule — so `isolation: isolate` was gone, the
// `z-index: -1` pseudo-element fell behind the grid's ground, `container-type`
// reverted to `normal` and EVERY `@container cal-cell` query went dead. Thirty
// day cells painted a flat run of page ground; the always-visible moon
// silhouette was off at every width, which is the exact defect this whole arc
// was opened to fix. A vertical 1px screenshot strip through any tile read
// rgb(242,243,245) for fourteen pixels and nothing else.
//
// The edge probe could not see it because `getComputedStyle(el, '::before')`
// answers what the CASCADE resolved, not what the compositor drew — and the one
// reading that did come back plausible came from `.interc`, the single day
// surface still painting. cell_probe_test.go's own header already states the
// rule this file enforces: "A corner is proved by a RECT WITH AREA and by a
// PAINTED PIXEL." The edge probe does neither. This one does both.
//
// SO THIS PROBE DECODES PIXELS. It screenshots the real Block at a real width
// and walks a 1px strip across a tile's top edge and across the gutter between
// two neighbours, asserting the three flat steps the construction is made of —
// ground, rule, fill — appear IN THAT ORDER, with the gutter's ground between
// two tiles measurably present. Nothing here reads a computed style, on
// purpose: this is the arm that survives the cascade being wrong.
//
// IT SKIPS HONESTLY under -short or with no Chromium, and a skipped run is NOT
// a pass. Registered in tools/check-browser-probes.sh.
//
//	go test ./internal/widgets/calendar_block/ -run TilePaint -v
package calendar_block

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"image"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// tpRect is one element's border box in CSS pixels, as the page reports it.
type tpRect struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// tpGeom is what the DOM pass hands the pixel pass: where to look.
//
// Two cells, not one, because the gutter is the half of the construction that
// only exists BETWEEN neighbours — a tile can be drawn correctly and still have
// no space around it, which is precisely the "microseparation" the operator
// said was missing from the ruled build.
type tpGeom struct {
	Grid  tpRect `json:"grid"`
	CellA tpRect `json:"cellA"`
	CellB tpRect `json:"cellB"`
	OK    bool   `json:"ok"`
	Why   string `json:"why"`
}

// tpReadGeom runs in the page. It picks the first two horizontally adjacent
// dated cells of the first week row — adjacent so the gutter between them is
// the one the eye sees, rather than two cells from different rows whose gap is
// the row gap.
const tpReadGeom = `function(host){
  var out = {grid:null, cellA:null, cellB:null, ok:false, why:""};
  var box = function(el){ var r = el.getBoundingClientRect();
    return {x:r.left, y:r.top, w:r.width, h:r.height}; };
  var grid = host.querySelector('.grid');
  if (!grid) { out.why = "no .grid in the host"; return out; }
  out.grid = box(grid);
  var row = host.querySelector('.wk');
  if (!row) { out.why = "no .wk week row"; return out; }
  var cells = [].slice.call(row.querySelectorAll('.cell')).filter(function(c){
    return c.querySelector('.dn');
  });
  if (cells.length < 2) { out.why = "fewer than two dated cells in the first week row"; return out; }
  var a = box(cells[0]), b = box(cells[1]);
  if (Math.abs(a.y - b.y) > 0.5) { out.why = "the first two cells are not on one row"; return out; }
  out.cellA = a; out.cellB = b; out.ok = true;
  return out;
}`

// tpPx is a decoded pixel.
type tpPx struct{ R, G, B uint8 }

func (p tpPx) String() string { return fmt.Sprintf("rgb(%d,%d,%d)", p.R, p.G, p.B) }

// near reports whether two pixels are the same flat step. The threshold is
// deliberately tight: these are FLAT fills, not gradients, so anything that is
// not the same colour is a different step.
func (p tpPx) near(q tpPx, tol int) bool {
	d := func(a, b uint8) int { return int(math.Abs(float64(int(a) - int(b)))) }
	return d(p.R, q.R) <= tol && d(p.G, q.G) <= tol && d(p.B, q.B) <= tol
}

// worst is the largest single-channel distance, in 0–255. The same measure
// every other colour claim in this package is made in.
func (p tpPx) worst(q tpPx) int {
	d := func(a, b uint8) int { return int(math.Abs(float64(int(a) - int(b)))) }
	m := d(p.R, q.R)
	if v := d(p.G, q.G); v > m {
		m = v
	}
	if v := d(p.B, q.B); v > m {
		m = v
	}
	return m
}

// tpRun is one host: a width and a theme.
type tpRun struct {
	label string
	width int
	dark  bool
}

// tpShoot renders the Block, screenshots it, and returns the decoded image
// alongside the geometry the DOM reported. Two Chromium invocations over the
// SAME file, because --dump-dom and --screenshot cannot both be asked of one
// run; the page has no scripted animation and no clock, so the two agree.
func tpShoot(t *testing.T, chrome string, r tpRun) (image.Image, tpGeom) {
	t.Helper()
	css := blockCSS(t)

	d := fxAlmanac(t, true)
	markup := cpLinkRe.ReplaceAllString(render(t, d), "")
	open, closeTag := "", ""
	if r.dark {
		open, closeTag = `<div class="dark">`, `</div>`
	}

	// THE PAGE GROUND IS MAGENTA ON PURPOSE. If the Block ever fails to paint
	// its own ground, a neutral page would supply a plausible-looking grey and
	// this probe would measure the harness instead of the product — which is
	// the class of mistake it exists to catch. Magenta is in no palette here,
	// so a magenta reading is unambiguous.
	page := `<!doctype html><html><head><meta charset="utf-8"><style>` +
		`html,body{margin:0;background:#ff00ff}` +
		`.probe-host{display:block;margin:20px}` +
		css + `</style></head><body>` +
		fmt.Sprintf(`%s<div class="probe-host" style="width:%dpx">%s</div>%s`,
			open, r.width, markup, closeTag) +
		`<script>document.addEventListener('DOMContentLoaded',function(){` +
		`var read=` + tpReadGeom + `;` +
		`document.body.setAttribute('data-probe', JSON.stringify(` +
		`read(document.querySelector('.probe-host'))));});</script>` +
		`</body></html>`

	dir := t.TempDir()
	path := filepath.Join(dir, "tilepaint.html")
	if err := os.WriteFile(path, []byte(page), 0o644); err != nil { //nolint:gosec // test artefact
		t.Fatalf("write probe page: %v", err)
	}

	// deviceScaleFactor 1 so one CSS pixel is one image pixel and the strips
	// below need no arithmetic to line up with the geometry.
	const shared = "--force-device-scale-factor=1"
	args := []string{
		"--headless", "--no-sandbox", "--disable-gpu", "--hide-scrollbars",
		"--window-size=1600,1200", "--virtual-time-budget=6000", shared,
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
	var g tpGeom
	if err := json.Unmarshal([]byte(html.UnescapeString(m[1])), &g); err != nil {
		t.Fatalf("probe payload: %v", err)
	}
	if !g.OK {
		t.Fatalf("the page could not locate two adjacent dated cells: %s", g.Why)
	}

	shot := filepath.Join(dir, "tilepaint.png")
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
	return img, g
}

// tpAt reads one pixel, failing loudly rather than returning zero for a
// coordinate outside the shot — an out-of-frame read is black, and black is a
// plausible dark-theme ground.
func tpAt(t *testing.T, img image.Image, x, y int) tpPx {
	t.Helper()
	b := img.Bounds()
	if x < b.Min.X || x >= b.Max.X || y < b.Min.Y || y >= b.Max.Y {
		t.Fatalf("pixel (%d,%d) is outside the %dx%d screenshot — the window was too "+
			"small for the host, so every reading below would be off-frame black",
			x, y, b.Dx(), b.Dy())
	}
	c := img.At(x, y)
	r, g, bl, _ := c.RGBA()
	return tpPx{uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8)}
}

// tpRuns collapses a scan into its flat runs, which is the shape the whole
// construction is described in: ground, rule, fill.
type tpSeg struct {
	px  tpPx
	n   int
	pos int
}

func tpScanY(t *testing.T, img image.Image, x, y0, y1 int) []tpSeg {
	t.Helper()
	var segs []tpSeg
	for y := y0; y < y1; y++ {
		p := tpAt(t, img, x, y)
		if n := len(segs); n > 0 && segs[n-1].px.near(p, 1) {
			segs[n-1].n++
			continue
		}
		segs = append(segs, tpSeg{px: p, n: 1, pos: y})
	}
	return segs
}

func tpScanX(t *testing.T, img image.Image, y, x0, x1 int) []tpSeg {
	t.Helper()
	var segs []tpSeg
	for x := x0; x < x1; x++ {
		p := tpAt(t, img, x, y)
		if n := len(segs); n > 0 && segs[n-1].px.near(p, 1) {
			segs[n-1].n++
			continue
		}
		segs = append(segs, tpSeg{px: p, n: 1, pos: x})
	}
	return segs
}

func tpFmt(segs []tpSeg) string {
	s := ""
	for i, g := range segs {
		if i > 0 {
			s += " | "
		}
		s += fmt.Sprintf("%s x%d", g.px, g.n)
	}
	return s
}

// TestTilePaintProbe_TheTileIsOnTheScreen is the painted-pixel arm.
//
// THE THREE CLAIMS, and each is a thing that has actually gone wrong here:
//
//  1. THE TILE PAINTS. A vertical strip through a tile's top edge shows the
//     ground, then the rule, then the fill — three distinct flat steps. The
//     stray-comment regression made this one flat run of ground, and every
//     computed-style probe stayed green through it.
//  2. THE GUTTER EXISTS. A horizontal strip between two adjacent tiles shows
//     the grid's ground. This is the operator's "microseparation" and it is the
//     half of the construction a correctly-drawn tile can still be missing.
//  3. THE STEPS ARE SOFT. The measured fill→rule step is banded, in PAINT, so
//     the loudness law is enforced by something the cascade cannot fake.
func TestTilePaintProbe_TheTileIsOnTheScreen(t *testing.T) {
	if testing.Short() {
		t.Skip("browser probe: skipped under -short (CI's mode) — a skipped run is NOT a pass")
	}
	chrome := findProbeChromium()
	if chrome == "" {
		t.Skip("browser probe: no Chromium binary found")
	}

	// The operator's own case first: a 366px host is what a 390px phone gives
	// the Block, and the calendar they run is a TEN-day week. The wide case is
	// there because the named density (84px column) is a different code path
	// and the same regression killed both.
	runs := []tpRun{
		{"phone 366 · light", 366, false},
		{"phone 366 · dark", 366, true},
		{"desk 1040 · light", 1040, false},
		{"desk 1040 · dark", 1040, true},
	}

	// THE PAINTED BANDS. Both are stated against the FILL because a painted
	// strip gives fill and rule adjacently and needs no third reading to
	// interpret; the cascade-side law in cell_probe_test.go is stated against
	// the ground for a reason given there. Floor: an edge nobody can see.
	// Ceiling: the reference's +14 with room for a hued edge and for light's
	// clamped-at-white arm, which sits about a pop step further out.
	const (
		popMin      = 4  // fill must be this far from the ground, or there is no tile
		ruleStepMin = 6  // fill→rule, worst channel
		ruleStepMax = 34 // fill→rule, worst channel
		gutterMin   = 2  // px of ground between two tiles
		gutterMax   = 6  // px — a gutter this wide is a gap, not a hairline
	)

	for _, r := range runs {
		t.Run(r.label, func(t *testing.T) {
			img, g := tpShoot(t, chrome, r)

			// ── 1. THE VERTICAL STRIP: ground → rule → fill ───────────────
			//
			// Taken through the cell's horizontal CENTRE so the tile's rounded
			// corners are nowhere near it, and started 5px above the cell box
			// so the run of ground before the tile is unambiguous.
			cx := int(g.CellA.X + g.CellA.W/2)
			y0 := int(g.CellA.Y) - 5
			y1 := int(g.CellA.Y) + 12
			segs := tpScanY(t, img, cx, y0, y1)
			t.Logf("vertical strip at x=%d, y=%d..%d: %s", cx, y0, y1, tpFmt(segs))

			if len(segs) < 3 {
				t.Fatalf("the strip through the tile's top edge has %d flat run(s), not the "+
					"three the construction is made of (ground, rule, fill): %s\n"+
					"ONE RUN MEANS THE TILE IS NOT PAINTED AT ALL. That has happened: a "+
					"stray `*/` orphaned a comment, the orphan was parsed as a selector "+
					"prelude and swallowed `.cal-block-host .cell` whole, so `isolation: "+
					"isolate` went with it and the z-index:-1 pseudo-element fell behind "+
					"the grid's ground. Every computed-style probe stayed green",
					len(segs), tpFmt(segs))
			}

			ground, rule, fill := segs[0].px, segs[1].px, segs[2].px
			if ground.near(tpPx{255, 0, 255}, 8) {
				t.Fatalf("the run above the tile is the harness's magenta page, not the "+
					"Block's own ground: %s. The grid is painting nothing at all",
					tpFmt(segs))
			}

			pop := fill.worst(ground)
			step := fill.worst(rule)
			t.Logf("ground %s → rule %s → fill %s · pop %d · fill→rule %d",
				ground, rule, fill, pop, step)

			if pop < popMin {
				t.Errorf("the tile's fill is %d/255 from the grid ground, below the %d floor. "+
					"The whole point of the tile is that it is LIGHTER THAN THE GROUND IT "+
					"SITS ON — that is the first of the reference's three flat steps and "+
					"without it there is no 'pop' to speak of", pop, popMin)
			}
			if step < ruleStepMin {
				t.Errorf("the fill→rule step is %d/255, below the %d floor — the edge has been "+
					"softened until it is gone. A tile is a fill, a rule and a gutter; an "+
					"invisible rule leaves two of three", step, ruleStepMin)
			}
			if step > ruleStepMax {
				t.Errorf("the fill→rule step is %d/255, above the %d ceiling. The reference's "+
					"step is 14. A tile edge is not a structural rule and must not borrow "+
					"the structural token — that shipped once, at 50/255 on light, inside a "+
					"change whose stated purpose was softness", step, ruleStepMax)
			}

			// ── 2. THE GUTTER: ground BETWEEN two tiles ───────────────────
			//
			// Scanned across the boundary at a height well inside both cells,
			// so what is crossed is two tile edges with ground between them.
			my := int(g.CellA.Y + g.CellA.H/2)
			x0 := int(g.CellA.X+g.CellA.W) - 6
			x1 := int(g.CellB.X) + 6
			cross := tpScanX(t, img, my, x0, x1)
			t.Logf("gutter strip at y=%d, x=%d..%d: %s", my, x0, x1, tpFmt(cross))

			gutter := 0
			for _, s := range cross {
				if s.px.near(ground, 2) && s.n > gutter {
					gutter = s.n
				}
			}
			if gutter < gutterMin {
				t.Errorf("only %dpx of grid ground shows between two adjacent tiles (floor %d). "+
					"THIS IS THE OPERATOR'S 'MICROSEPARATION' AND IT IS THE HALF OF THE "+
					"CONSTRUCTION A CORRECTLY-DRAWN TILE CAN STILL BE MISSING: the ruled "+
					"build had a fill and a rule and no space, and read as a table. Strip: %s",
					gutter, gutterMin, tpFmt(cross))
			}
			if gutter > gutterMax {
				t.Errorf("%dpx of ground shows between two adjacent tiles (ceiling %d). The tile "+
					"is inset 1.5px on each side, so two neighbours give up 3px between "+
					"them; more than that means the inset grew — and every pixel of inset "+
					"is a pixel the date and the moon silhouette no longer have. Strip: %s",
					gutter, gutterMax, tpFmt(cross))
			}
		})
	}
}
