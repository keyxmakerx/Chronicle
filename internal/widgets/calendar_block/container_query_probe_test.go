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
	BandsShown  bool    `json:"bandsShown"`
	SyncFull    bool    `json:"syncFull"`
	SyncCompact bool    `json:"syncCompact"`
	SyncShown   bool    `json:"syncShown"`
	HalfRules   int     `json:"halfRules"`
	DataDays    int     `json:"dataDays"`
	BlockHeight float64 `json:"blockHeight"`
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
				if r.HeaderShown || r.BandsShown {
					t.Error("mini drops the weekday header and the era bands: neither fits")
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

			// ── invariants that hold at every size ───────────────────────────
			if h.wantTier != TierSubmini && r.HalfRules != len(h.data.Month.Rows) && halfColumn(h.data.Month) > 0 {
				// (the ruler lives in the band row, which mini drops)
				if h.wantTier != TierMini {
					t.Errorf("%d five-column rulers rendered; want one per week row (%d)",
						r.HalfRules, len(h.data.Month.Rows))
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
    bandsShown: vis(root.querySelector('.bands')),
    syncFull: vis(root.querySelector('.sync-full')),
    syncCompact: vis(root.querySelector('.sync-compact')),
    syncShown: vis(root.querySelector('.sync')),
    halfRules: [].slice.call(root.querySelectorAll('.halfrule')).filter(vis).length,
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
