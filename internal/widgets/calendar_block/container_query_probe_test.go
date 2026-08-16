// container_query_probe_test.go — the real-browser proof.
//
// WHY A BROWSER AND NOT ANOTHER DOM ASSERTION. Every other test in this package
// asserts on templ output or on the stylesheet's source text. Neither can see
// the thing that actually matters here: whether a real layout engine, given a
// host of a stated width, measures the column the arithmetic predicts and picks
// the density the contract signs off. Container-query density that is only
// proven by a Go unit test is not proven.
//
// It also measures the ~1.2px-per-column gap between the contract's MODEL of a
// column (sizing.go, calendar-v4.html:1429) and the column a browser actually
// lays out, so the divergence is a printed number in the PR rather than a claim.
//
// SKIPS, HONESTLY, when it cannot do that: under -short (CI's mode — ci.yml runs
// `go test ./... -short`), or when no Chromium binary is present. A skipped run
// is NOT a pass; the skip message names exactly what was missing.
//
//	go test ./internal/widgets/calendar_block/ -run Probe -v
//
// No iframe technique is needed here, unlike the calendar_v2 mobile probe:
// container queries measure the ELEMENT, not the viewport, so every host width
// under test can be laid out side by side in one wide window.
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

// probeHost is one host width under test.
type probeHost struct {
	name  string
	width int
	data  BlockData
	// what the contract says must happen at this width
	wantTier  string
	wantNamed bool
}

// probeReading is what the in-page script measures for one host.
type probeReading struct {
	Host        int     `json:"host"`
	BlockWidth  float64 `json:"blockWidth"`
	CellWidth   float64 `json:"cellWidth"`   // border box of one in-range cell
	ColumnWidth float64 `json:"columnWidth"` // the density container's own measurement
	NamedShown  bool    `json:"namedShown"`
	UnderShown  bool    `json:"underShown"`
	NpHeight    float64 `json:"npHeight"`
	LedgerBes   bool    `json:"ledgerBeside"` // docked to the right (full) vs below (std)
	LedgerWidth float64 `json:"ledgerWidth"`
	IntercShown bool    `json:"intercShown"`
	GridShown   bool    `json:"gridShown"`
	TickShown   bool    `json:"tickShown"`
	HeaderShown bool    `json:"headerShown"`
	SyncFull    bool    `json:"syncFull"`
	SyncCompact bool    `json:"syncCompact"`
	SyncShown   bool    `json:"syncShown"`
	DataDays    int     `json:"dataDays"`
	BlockHeight float64 `json:"blockHeight"`

	// ── THE TILE (C-CALV4-TILES §1) ────────────────────────────────────────
	//
	// These four replace `bandsShown` and `halfRules`, which measured the era
	// caption row and the ruler that lived inside it. Both are deleted; the
	// separation they used to sit above is now a rounded fill drawn INSIDE each
	// cell, and what has to be measured is that the fill is inset — because an
	// inset of zero is a build where every tile touches its neighbour and the
	// micro-separation the operator asked for is simply absent, with nothing in
	// the markup or the source text different.
	TileInsetT  float64 `json:"tileInsetT"`
	TileRadius  string  `json:"tileRadius"`
	TileZ       string  `json:"tileZ"`
	TileGapX    float64 `json:"tileGapX"`   // measured ground between two adjacent tiles
	CellPaints  bool    `json:"cellPaints"` // the cell's OWN background — must be none
	HalfCells   int     `json:"halfCells"`  // the five-column rule's cell-side owner
	HalfHeaders int     `json:"halfHeaders"`
}

func TestProbe_ContainerQuerySizingInRealBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("browser probe: skipped under -short")
	}
	chrome := findProbeChromium()
	if chrome == "" {
		t.Skip("browser probe: no Chromium binary found (set CHROMIUM_BIN)")
	}

	hosts := []probeHost{
		// THE DIVERGENCE PAIR, at the host width the signed sheet names.
		{"harptos-1040", 1040, fxHarptos(true), TierFull, false},
		{"gregorian-1040", 1040, fxGregorian(), TierFull, true},
		// The Bench's own host width, where a ten-day week finally earns names.
		{"harptos-1232", 1232, fxHarptos(true), TierFull, true},
		// The signed std/mobile still: a 390px device less its 32px of gutters.
		{"harptos-358", 358, fxHarptos(true), TierStd, false},
		{"harptos-280", 280, fxHarptos(true), TierMini, false},
		{"harptos-220", 220, fxHarptos(true), TierSubmini, false},
	}

	readings := runSizingProbe(t, chrome, hosts)

	for i, h := range hosts {
		r := readings[i]
		t.Run(h.name, func(t *testing.T) {
			model := ColWidth(SizeClass(h.width), h.width, h.data.Month.WeekLen)
			t.Logf("host %dpx · week %d → size class %s · MEASURED column %.1fpx "+
				"(contract model %.1fpx, delta %+.1fpx) · density %s",
				h.width, h.data.Month.WeekLen, h.wantTier, r.ColumnWidth, model,
				r.ColumnWidth-model, densityWord(r))

			if got := SizeClass(h.width); got != h.wantTier {
				t.Fatalf("test setup: SizeClass(%d) = %s, want %s", h.width, got, h.wantTier)
			}

			// ── density, measured ────────────────────────────────────────────
			if h.wantTier == TierSubmini {
				// The grid is gone at this width, so there is no cell to measure.
				// Density is asserted by its ABSENCE: see the sub-mini block below.
				if r.NamedShown || r.UnderShown {
					t.Error("sub-mini drops the grid entirely; no cell subtree may be laid out")
				}
			} else if r.NamedShown == r.UnderShown {
				t.Errorf("exactly ONE cell subtree must be visible; named=%v under=%v",
					r.NamedShown, r.UnderShown)
			}
			if h.wantTier != TierSubmini && r.NamedShown != h.wantNamed {
				t.Errorf("density: named=%v at a measured %.1fpx column; the contract says %v "+
					"(threshold %.0fpx)", r.NamedShown, r.ColumnWidth, h.wantNamed, NamedColWidthMin)
			}
			// The browser and the model must agree about WHICH SIDE of the
			// threshold they are on, even though they disagree by ~1px about
			// where exactly the column edge is.
			if h.wantTier == TierFull || h.wantTier == TierStd {
				if (r.ColumnWidth >= NamedColWidthMin) != IsNamedCSS(h.wantTier, h.width, h.data.Month.WeekLen) {
					t.Errorf("the measured column (%.1fpx) and the contract model (%.1fpx) fall on "+
						"OPPOSITE sides of the %.0fpx threshold — the flip point has drifted far "+
						"enough to matter", r.ColumnWidth, model, NamedColWidthMin)
				}
			}

			// ── size class, measured ─────────────────────────────────────────
			switch h.wantTier {
			case TierFull:
				if !r.LedgerBes {
					t.Error("at full tier the Ledger docks BESIDE the month, not below it")
				}
				if math.Abs(r.LedgerWidth-float64(ledgerDock)) > 1 {
					t.Errorf("the docked Ledger measures %.1fpx; the full-tier arithmetic "+
						"subtracts %dpx and the two must be the same number", r.LedgerWidth, ledgerDock)
				}
				if got, want := r.IntercShown, len(h.data.Month.Intercalary) > 0; got != want {
					t.Errorf("intercalary row visible=%v at full tier; this calendar declares %d "+
						"intercalary days", got, len(h.data.Month.Intercalary))
				}
				if !r.SyncFull || r.SyncCompact {
					t.Errorf("full tier shows the WIDE sync string only; full=%v compact=%v",
						r.SyncFull, r.SyncCompact)
				}
				if math.Abs(r.NpHeight-60) > 1 {
					t.Errorf("the full-tier Nameplate measures %.1fpx; declared 60px", r.NpHeight)
				}
			case TierStd:
				if r.LedgerBes {
					t.Error("at std the Ledger docks BELOW the month — there is no room beside it")
				}
				if r.IntercShown {
					t.Error("the intercalary row is full tier only")
				}
				if r.SyncFull || !r.SyncCompact {
					t.Errorf("std shows the COMPACT sync string only; full=%v compact=%v",
						r.SyncFull, r.SyncCompact)
				}
			case TierMini:
				if r.SyncShown {
					t.Error("mini shows neither sync string")
				}
				if !r.GridShown || r.TickShown {
					t.Error("mini keeps the grid and does not fall back to the tick rule")
				}
				if r.HeaderShown {
					t.Error("mini drops the weekday header: it does not fit")
				}
				if math.Abs(r.BlockHeight-180) > 1 {
					t.Errorf("the mini Block measures %.1fpx tall; declared 180px", r.BlockHeight)
				}
			case TierSubmini:
				if r.GridShown {
					t.Error("below 240px the grid is DROPPED honestly, not squeezed")
				}
				if !r.TickShown {
					t.Error("sub-mini must render the thirty-tick month rule instead")
				}
			}

			// ── THE TILE, AT EVERY SIZE THAT DRAWS A GRID ────────────────────
			//
			// REPLACES the `bandsShown` / `halfRules` arms (C-CALV4-TILES §1/§3).
			// Those measured the era caption row and the ruler inside it; both
			// are deleted. What has to be measured now is that the cell's
			// separation is a fill drawn INSIDE its box with real ground around
			// it — because the failure mode is silent in every other test in
			// this package: a zero inset, or a background left on `.cell`,
			// renders a grid that looks entirely reasonable and has none of the
			// micro-separation the whole slice exists to add.
			if h.wantTier != TierSubmini {
				t.Logf("tile: inset %.2fpx · radius %s · z-index %s · ground between two "+
					"tiles %.2fpx · cell paints its own box: %v · five-column rule on %d "+
					"cells / %d header cells",
					r.TileInsetT, r.TileRadius, r.TileZ, r.TileGapX, r.CellPaints,
					r.HalfCells, r.HalfHeaders)

				if r.TileInsetT != 1.5 {
					t.Errorf("the tile insets %.2fpx from the cell's padding box; the recipe "+
						"is 1.5px on each of two neighbours, which is the reference's measured "+
						"3px of ground between two cells", r.TileInsetT)
				}
				if r.TileRadius != "6px" {
					t.Errorf("the tile's corner radius is %q; --r-ctl is 6px and the reference "+
						"measures ~6px. A radius that resolved to 0 would make the tile a "+
						"rectangle nobody could tell from the ruled grid it replaced",
						r.TileRadius)
				}
				if r.TileZ != "-1" {
					t.Errorf("the tile's z-index is %q, not -1 — it must paint behind the "+
						"cell's own content and in front of the grid's ground", r.TileZ)
				}
				if r.CellPaints {
					t.Error("`.cell` paints its own box. Its box IS the grid track, so any fill " +
						"there covers the 1.5px the tile insets and closes the gutter — the " +
						"ground stops showing and the tiles read as one continuous surface")
				}
				if r.TileGapX < 2.9 || r.TileGapX > 3.1 {
					t.Errorf("%.2fpx of ground between two adjacent tiles; the reference "+
						"measures 3px and that gap is the micro-separation itself. Measured "+
						"between two cells that are actually side by side, so a wrong inset, "+
						"a stray padding and a re-introduced margin all land here",
						r.TileGapX)
				}
			}

			// ── invariants that hold at every size ───────────────────────────
			//
			// THE FIVE-COLUMN RULE'S OWNERS, re-pointed off `.halfrule` (which
			// died with the band row) onto the two elements that draw it now.
			// The cells carry it at every tier that draws a grid; the header
			// carries it only where the header itself is drawn.
			if h.wantTier != TierSubmini && halfColumn(h.data.Month) > 0 {
				if r.HalfCells != len(h.data.Month.Rows) {
					t.Errorf("%d cells carry the five-column rule; want one per week row (%d)",
						r.HalfCells, len(h.data.Month.Rows))
				}
				wantHeaders := 0
				if r.HeaderShown {
					wantHeaders = 1
				}
				if r.HalfHeaders != wantHeaders {
					t.Errorf("%d weekday-header cells carry the five-column rule; want %d "+
						"(the header is %v at this tier). A ramp step that reaches the cells "+
						"and stops at the header is a visible seam",
						r.HalfHeaders, wantHeaders, r.HeaderShown)
				}
			}
			wantDays := h.data.Month.Days + len(h.data.Month.Intercalary)
			if h.wantTier == TierSubmini {
				wantDays = h.data.Month.Days + len(h.data.Month.Intercalary) + h.data.Month.Days // grid + ticks
			}
			if r.DataDays == 0 {
				t.Error("no element carries data-day — guard B4")
			}
			_ = wantDays
		})
	}
}

func densityWord(r probeReading) string {
	if r.NamedShown {
		return "named"
	}
	return "underline"
}

// probeScript measures one host box. A zero-area box counts as hidden, which
// catches a `display:contents` wrapper or a collapsed parent that a
// computed-style check alone would miss.
const sizingProbeScript = `
function(root){
  var vis = function(el){
    if (!el) return false;
    var r = el.getBoundingClientRect();
    return r.width > 0 && r.height > 0;
  };
  var w = function(el){ return el ? el.getBoundingClientRect().width : 0 };
  var h = function(el){ return el ? el.getBoundingClientRect().height : 0 };
  var block  = root.querySelector('.block');
  var cell   = root.querySelector('.cell[data-day]');
  var ledger = root.querySelector('[data-zone="ledger"]');
  var inst   = root.querySelector('.inst');
  var colW = 0;
  if (cell) {
    var cs = getComputedStyle(cell);
    // the density container measures its CONTENT box
    colW = cell.getBoundingClientRect().width
         - parseFloat(cs.borderLeftWidth) - parseFloat(cs.borderRightWidth)
         - parseFloat(cs.paddingLeft) - parseFloat(cs.paddingRight);
  }

  // ── THE TILE, READ OFF THE LIVE PSEUDO-ELEMENT ────────────────────────
  // getComputedStyle(el, '::before') resolves an absolutely-positioned
  // pseudo's offsets to USED pixels, so this is the laid-out tile and not
  // the declaration. The gap is then computed between two REAL adjacent
  // cells rather than doubled from one inset, because that is the number a
  // reader actually sees and it is the one a stray padding would change.
  var tileT = -1, tileRad = '', tileZ = '', gapX = -1, cellPaints = false;
  if (cell) {
    var ps = getComputedStyle(cell, '::before');
    tileT = parseFloat(ps.top);
    tileRad = ps.borderTopLeftRadius;
    tileZ = ps.zIndex;
    var ccs = getComputedStyle(cell);
    cellPaints = ccs.backgroundColor !== 'rgba(0, 0, 0, 0)' || ccs.backgroundImage !== 'none';

    var row = cell.closest('.wk');
    var sibs = row ? [].slice.call(row.querySelectorAll('.cell')) : [];
    for (var i = 0; i + 1 < sibs.length && gapX < 0; i++) {
      // skip the five-column boundary: its strong rule sits in the gutter on
      // purpose and widens it, so measuring there would report the exception.
      if (sibs[i].classList.contains('half')) continue;
      var a = sibs[i], b = sibs[i + 1];
      var ar = a.getBoundingClientRect(), br = b.getBoundingClientRect();
      var as = getComputedStyle(a), bs = getComputedStyle(b);
      var ap = getComputedStyle(a, '::before'), bp = getComputedStyle(b, '::before');
      if (br.left < ar.right - 0.5) continue; // not laid out side by side
      var aRight = ar.right - parseFloat(as.borderRightWidth) - parseFloat(ap.right);
      var bLeft  = br.left  + parseFloat(bs.borderLeftWidth)  + parseFloat(bp.left);
      gapX = Math.round((bLeft - aRight) * 100) / 100;
    }
  }

  return {
    host: Math.round(w(root)),
    blockWidth: w(block),
    blockHeight: h(block),
    cellWidth: w(cell),
    columnWidth: Math.round(colW * 10) / 10,
    namedShown: vis(cell && cell.querySelector('.cnamed')),
    underShown: vis(cell && cell.querySelector('.cunder')),
    npHeight: h(root.querySelector('.np')),
    ledgerBeside: !!(ledger && inst &&
        ledger.getBoundingClientRect().left >= inst.getBoundingClientRect().right - 1),
    ledgerWidth: w(ledger),
    intercShown: vis(root.querySelector('.interc-row')),
    gridShown: vis(root.querySelector('.grid')),
    tickShown: vis(root.querySelector('.tickstrip')),
    headerShown: vis(root.querySelector('.hd')),
    syncFull: vis(root.querySelector('.sync-full')),
    syncCompact: vis(root.querySelector('.sync-compact')),
    syncShown: vis(root.querySelector('.sync')),
    tileInsetT: tileT,
    tileRadius: tileRad,
    tileZ: tileZ,
    tileGapX: gapX,
    cellPaints: cellPaints,
    halfCells: [].slice.call(root.querySelectorAll('.cell.half')).filter(vis).length,
    halfHeaders: [].slice.call(root.querySelectorAll('.hd b.half')).filter(vis).length,
    dataDays: root.querySelectorAll('[data-day]').length
  };
}`

var probePayloadRe = regexp.MustCompile(`data-probe="([^"]*)"`)

// runSizingProbe lays every host out side by side in one wide window and reads
// them all back in a single Chromium run.
func runSizingProbe(t *testing.T, chrome string, hosts []probeHost) []probeReading {
	t.Helper()
	css := blockCSS(t)

	var boxes strings.Builder
	for i, h := range hosts {
		markup := render(t, h.data)
		// The <link> cannot resolve over file://; the sheet is inlined instead,
		// which is also what makes this a test of THIS sheet rather than of a
		// stale build artefact.
		markup = regexp.MustCompile(`<link[^>]*>`).ReplaceAllString(markup, "")
		fmt.Fprintf(&boxes, `<div class="probe-host" id="h%d" style="width:%dpx">%s</div>`,
			i, h.width, markup)
	}

	page := `<!doctype html><html><head><meta charset="utf-8"><style>` +
		`html,body{margin:0;background:#fff}` +
		`.probe-host{display:block;margin:24px}` +
		css + `</style></head><body>` + boxes.String() +
		`<script>document.addEventListener('DOMContentLoaded',function(){` +
		`var read=` + sizingProbeScript + `;` +
		`var out=[].slice.call(document.querySelectorAll('.probe-host')).map(read);` +
		`document.body.setAttribute('data-probe', JSON.stringify(out));});</script>` +
		`</body></html>`

	dir := t.TempDir()
	path := filepath.Join(dir, "probe.html")
	if err := os.WriteFile(path, []byte(page), 0o644); err != nil {
		t.Fatalf("write probe page: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, chrome,
		"--headless", "--no-sandbox", "--disable-gpu", "--hide-scrollbars",
		"--window-size=1600,1200", "--virtual-time-budget=5000",
		"--dump-dom", "file://"+path,
	)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("chromium: %v", err)
	}
	m := probePayloadRe.FindStringSubmatch(string(out))
	if m == nil {
		t.Fatalf("no probe payload in the rendered DOM — the page script did not run")
	}
	var readings []probeReading
	if err := json.Unmarshal([]byte(html.UnescapeString(m[1])), &readings); err != nil {
		t.Fatalf("probe payload: %v", err)
	}
	if len(readings) != len(hosts) {
		t.Fatalf("probe returned %d readings for %d hosts", len(readings), len(hosts))
	}
	return readings
}

func findProbeChromium() string {
	if p := os.Getenv("CHROMIUM_BIN"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	for _, name := range []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	for _, pattern := range []string{
		"/opt/pw-browsers/chromium-*/chrome-linux/chrome",
		filepath.Join(os.Getenv("HOME"), ".cache/ms-playwright/chromium-*/chrome-linux/chrome"),
	} {
		if matches, _ := filepath.Glob(pattern); len(matches) > 0 {
			return matches[len(matches)-1]
		}
	}
	return ""
}
