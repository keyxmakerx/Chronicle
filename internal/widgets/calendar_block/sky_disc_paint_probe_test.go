// sky_disc_paint_probe_test.go — THE DISCS THEMSELVES, not their wrappers.
//
// WHY A SECOND SKY PROBE EXISTS. sky_measure_probe_test.go has been green since
// [SKY-15] landed and it measures `.skb` — the WRAPPER — which is correct at
// every density (13.0px closed → 40.0px open at 1440, 11.0 → 32.0 at the seal)
// and which is exactly why nobody caught this: the operator's first look
// reported the band as a coloured strip with no moons in it, and every shipped
// guard agreed the moons were 40px tall.
//
// THE DEFECT. The disc is the Block's own `.ph` idiom — an `<i>` — and `<i>` is
// `display: inline`. `.cal-block-host .ph` sets `position: relative` and a
// width/height, and `.skygrow .skb .ph` sets `inline-size/block-size: 100%`;
// none of that blockifies anything. In `.phrow` the `.ph` is a FLEX ITEM and is
// blockified by the flex container, which is why the almanac's discs paint. In
// `.skb` it is not: `.skdiscs` is the flex container and `.skb` is its item, so
// `.ph` one level further down stays inline, lays out at 0×0, and its `::before`
// (the half-fill) and `::after` (the terminator) are absolutely positioned into
// a zero-size containing block. The signature move of the whole slice — greyscale
// phase discs growing through the horizon onto the page — has never rendered a
// pixel.
//
// WHAT THIS PROBE ASSERTS, and why each claim is a rect and not a string:
//
//  1. the `.ph` inside `.skb` generates a BLOCK-LEVEL box. A `display` string is
//     the cheapest half of the claim and it is the half a CSS grep could do, so
//     it is claim 1 and not the whole test.
//  2. the `.ph` FILLS its wrapper, closed and open. `inline-size: 100%` is a
//     declaration; a rect that matches `.skb` is the fact. This is the claim
//     that stays true if someone later re-expresses the fix differently.
//  3. the `::before` half-fill and the `::after` terminator have NON-ZERO used
//     sizes. They are the disc's actual ink — the phase — and they are the
//     things that were absolutely positioned into nothing. Read from
//     getComputedStyle on the pseudo-element, which reports used values.
//  4. the PAINTED disc crosses the horizon. sky_measure_probe already measures
//     the wrapper crossing at ~33%; the operator sees ink, not wrappers, so the
//     crossing is re-measured on the thing that carries the ink.
//
// IT SKIPS HONESTLY under -short or with no Chromium, on this package's own
// precedent, and a skipped run is NOT a pass.
//
//	go test ./internal/widgets/calendar_block/ -run SkyDiscPaint -v
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

// skyDiscPaintReading is one host's first disc, measured as ink.
type skyDiscPaintReading struct {
	Host    int    `json:"host"`
	Density string `json:"density"`
	// claim 1 — the used `display` of the `.ph` seated inside `.skb`.
	Display string `json:"display"`
	// claim 2 — wrapper vs disc, closed and open.
	WrapClosedW float64 `json:"wrapClosedW"`
	WrapClosedH float64 `json:"wrapClosedH"`
	DiscClosedW float64 `json:"discClosedW"`
	DiscClosedH float64 `json:"discClosedH"`
	WrapOpenW   float64 `json:"wrapOpenW"`
	WrapOpenH   float64 `json:"wrapOpenH"`
	DiscOpenW   float64 `json:"discOpenW"`
	DiscOpenH   float64 `json:"discOpenH"`
	// claim 3 — the ink, from the pseudo-elements' used values, open.
	BeforeW float64 `json:"beforeW"`
	BeforeH float64 `json:"beforeH"`
	AfterH  float64 `json:"afterH"`
	// claim 4 — the share of the PAINTED disc below the band's bottom edge.
	DiscBelowHorizonPct float64 `json:"discBelowHorizonPct"`
	// How many discs the cluster holds, so a fixture that lost its almanac
	// cannot make every claim above pass vacuously.
	Discs int `json:"discs"`
}

const skyDiscPaintScript = `function(root){
  var sky = root.querySelector('details.skygrow');
  var band = sky && sky.querySelector('summary.skyhdr');
  var wrap = sky && sky.querySelector('.skdiscs .skb');
  var disc = wrap && wrap.querySelector('.ph');
  var rect = function(el){ return el ? el.getBoundingClientRect() : null; };
  var r1 = function(v){ return Math.round(v * 10) / 10; };
  var px = function(s){ var n = parseFloat(s); return isNaN(n) ? -1 : Math.round(n * 10) / 10; };
  var sample = function(){
    var w = rect(wrap), d = rect(disc);
    return {
      wrapW: w ? r1(w.width) : -1, wrapH: w ? r1(w.height) : -1,
      discW: d ? r1(d.width) : -1, discH: d ? r1(d.height) : -1,
      discBottom: d ? d.bottom : 0,
      bandBottom: rect(band) ? rect(band).bottom : 0
    };
  };
  if (!root.__dpClosed) { root.__dpSample = sample; return null; }
  var closed = root.__dpClosed, open = root.__dpOpen;
  // The pseudo-elements are read OPEN, where the disc is at its full diameter
  // and a zero reading cannot be blamed on the closed size being small.
  var before = disc ? getComputedStyle(disc, '::before') : null;
  var after  = disc ? getComputedStyle(disc, '::after')  : null;
  var crossing = open.discH > 0
    ? Math.round((open.discBottom - open.bandBottom) / open.discH * 1000) / 10
    : -1;
  return {
    host: Math.round(root.getBoundingClientRect().width),
    density: getComputedStyle(band).justifyContent === 'flex-end' ? 'C3 seal' : 'C1 horizon',
    display: disc ? getComputedStyle(disc).display : 'MISSING',
    wrapClosedW: closed.wrapW, wrapClosedH: closed.wrapH,
    discClosedW: closed.discW, discClosedH: closed.discH,
    wrapOpenW: open.wrapW, wrapOpenH: open.wrapH,
    discOpenW: open.discW, discOpenH: open.discH,
    beforeW: before ? px(before.width) : -1,
    beforeH: before ? px(before.height) : -1,
    afterH:  after  ? px(after.height)  : -1,
    discBelowHorizonPct: crossing,
    discs: sky.querySelectorAll('.skdiscs .skb .ph').length
  };
}`

// TestSkyDiscPaintProbe_TheDiscsAreInkAndNotOnlyWrappers is the whole of it.
func TestSkyDiscPaintProbe_TheDiscsAreInkAndNotOnlyWrappers(t *testing.T) {
	if testing.Short() {
		t.Skip("the disc paint probe needs a browser; skipped under -short (CI's mode)")
	}
	chrome := findProbeChromium()
	if chrome == "" {
		t.Skip("no Chromium binary found (set CHROMIUM_BIN) — a skipped probe is NOT a pass")
	}

	hosts := []struct {
		name    string
		width   int
		density string
		// The wrapper diameters sky_measure_probe already pins. The disc must
		// MATCH them, so they are restated here as the target rather than
		// re-derived: two probes disagreeing about the same number is a thing
		// this package would rather find than ship.
		closedPx float64
		openPx   float64
	}{
		{"1440 viewport / wide column", 1440, "C1 horizon", 13.0, 40.0},
		{"768 viewport / wide column", 768, "C1 horizon", 13.0, 40.0},
		{"390 viewport / the Bench's 358px column", 358, "C3 seal", 11.0, 32.0},
	}

	d := fxSky(t, true)
	var boxes strings.Builder
	for i, h := range hosts {
		markup := regexp.MustCompile(`<link[^>]*>`).ReplaceAllString(render(t, d), "")
		fmt.Fprintf(&boxes, `<div class="probe-host cal-bench" id="h%d" style="width:%dpx">%s</div>`,
			i, h.width, markup)
	}

	// BOTH sheets, for the reason sky_measure_probe states: the markup is the
	// Block's and the rules are the Bench's, and the `.ph` idiom itself is
	// declared in calendar-block.css while its sizing is declared in
	// calendar-bench.css — this defect lives precisely in the seam between them.
	page := `<!doctype html><html><head><meta charset="utf-8"><style>` +
		`html,body{margin:0;background:#fff}` +
		blockCSS(t) + skyProbeBenchCSS(t) +
		`.probe-host{display:block;margin:24px;max-width:none}` +
		`</style></head><body>` + boxes.String() +
		`<script>document.addEventListener('DOMContentLoaded',function(){` +
		`var read=` + skyDiscPaintScript + `;` +
		`var hosts=[].slice.call(document.querySelectorAll('.probe-host'));` +
		`hosts.forEach(function(root){read(root);` +
		`root.__dpClosed=root.__dpSample();` +
		`root.querySelector('details.skygrow').setAttribute('open','');});` +
		`setTimeout(function(){` +
		`hosts.forEach(function(root){root.__dpOpen=root.__dpSample();});` +
		`var out=hosts.map(read);` +
		`document.body.setAttribute('data-probe', JSON.stringify(out));},600);});</script>` +
		`</body></html>`

	path := filepath.Join(t.TempDir(), "sky-disc-paint.html")
	if err := os.WriteFile(path, []byte(page), 0o644); err != nil { //nolint:gosec // test artefact
		t.Fatalf("write probe page: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, chrome,
		"--headless", "--no-sandbox", "--disable-gpu", "--hide-scrollbars",
		"--window-size=1600,1400", "--virtual-time-budget=5000",
		"--dump-dom", "file://"+path).Output()
	if err != nil {
		t.Fatalf("chromium: %v", err)
	}
	m := probePayloadRe.FindStringSubmatch(string(out))
	if m == nil {
		t.Fatal("no probe payload — the page script did not run")
	}
	var readings []skyDiscPaintReading
	if err := json.Unmarshal([]byte(html.UnescapeString(m[1])), &readings); err != nil {
		t.Fatalf("probe payload: %v", err)
	}
	if len(readings) != len(hosts) {
		t.Fatalf("probe returned %d readings for %d hosts", len(readings), len(hosts))
	}

	for i, h := range hosts {
		r := readings[i]
		t.Run(h.name, func(t *testing.T) {
			t.Logf("host %dpx · %s · %d discs · display %q · wrapper %.1f→%.1fpx · "+
				"DISC %.1f→%.1fpx · ::before %.1f×%.1fpx · ::after h%.1fpx · "+
				"%.1f%% of the painted disc below the horizon",
				r.Host, r.Density, r.Discs, r.Display,
				r.WrapClosedH, r.WrapOpenH, r.DiscClosedH, r.DiscOpenH,
				r.BeforeW, r.BeforeH, r.AfterH, r.DiscBelowHorizonPct)

			if r.Density != h.density {
				t.Fatalf("density = %q, want %q — the host is not the column this "+
					"row is about and every number below would be the wrong one",
					r.Density, h.density)
			}
			if r.Discs == 0 {
				t.Fatal("the cluster holds no `.ph` at all — the fixture lost its " +
					"almanac and every claim below would pass vacuously")
			}

			// (1) THE BOX GENERATES. `<i>` is inline by default and inline
			// non-replaced boxes ignore inline-size/block-size entirely, which
			// is the whole defect in one word.
			if r.Display == "inline" {
				t.Errorf("the disc's `.ph` computes `display: inline` — an inline "+
					"non-replaced box ignores inline-size/block-size, so the disc lays "+
					"out at 0×0 and its ::before/::after are positioned into a zero-size "+
					"containing block. In `.phrow` the same idiom is a FLEX ITEM and is "+
					"blockified for free; inside `.skb` it is not, and nothing here "+
					"blockifies it (display = %q)", r.Display)
			}

			// (2) THE DISC FILLS ITS WRAPPER, at both ends of the envelope. The
			// wrapper numbers are sky_measure_probe's and are restated as the
			// target: if the two probes ever disagree, one of them is lying.
			for _, c := range []struct {
				state              string
				wrapW, wrapH       float64
				discW, discH, want float64
			}{
				{"closed", r.WrapClosedW, r.WrapClosedH, r.DiscClosedW, r.DiscClosedH, h.closedPx},
				{"open", r.WrapOpenW, r.WrapOpenH, r.DiscOpenW, r.DiscOpenH, h.openPx},
			} {
				if diff := c.wrapH - c.want; diff > 0.6 || diff < -0.6 {
					t.Errorf("%s: the WRAPPER measures %.1fpx against sky_measure_probe's "+
						"%.1fpx — the two probes disagree about the same box", c.state, c.wrapH, c.want)
				}
				if c.discH <= 0 || c.discW <= 0 {
					t.Errorf("%s: the disc's own box is %.1f×%.1fpx inside a %.1f×%.1fpx "+
						"wrapper — the wrapper is correct and PAINTS NOTHING, which is "+
						"exactly what the operator saw: a coloured strip with no moons in it",
						c.state, c.discW, c.discH, c.wrapW, c.wrapH)
					continue
				}
				if diff := c.discH - c.wrapH; diff > 0.6 || diff < -0.6 {
					t.Errorf("%s: the disc is %.1fpx tall inside a %.1fpx wrapper — "+
						"`block-size: 100%%` is declared and is not landing", c.state, c.discH, c.wrapH)
				}
				if diff := c.discW - c.wrapW; diff > 0.6 || diff < -0.6 {
					t.Errorf("%s: the disc is %.1fpx wide inside a %.1fpx wrapper — "+
						"`inline-size: 100%%` is declared and is not landing", c.state, c.discW, c.wrapW)
				}
			}

			// (3) THE INK. `::before` is the half-fill (inset: 0, clipped to a
			// half) and `::after` is the terminator (top/bottom: -1px). Both are
			// absolutely positioned against the `.ph` box, so both are zero when
			// that box is zero — and both are what a reader actually calls a
			// moon phase.
			if r.BeforeW <= 0 || r.BeforeH <= 0 {
				t.Errorf("the half-fill `::before` used size is %.1f×%.1fpx — the phase "+
					"itself is not painted; `inset: 0` against a zero-size containing "+
					"block is zero", r.BeforeW, r.BeforeH)
			} else if diff := r.BeforeH - r.DiscOpenH; diff > 0.6 || diff < -0.6 {
				t.Errorf("the half-fill is %.1fpx tall on a %.1fpx disc — `inset: 0` "+
					"should make it the disc exactly", r.BeforeH, r.DiscOpenH)
			}
			// top:-1px and bottom:-1px, so the terminator overshoots by 2px. It
			// is asserted as "taller than the disc" rather than as an exact
			// number so a later change to that overshoot is a design decision
			// and not a red test.
			if r.AfterH <= 0 {
				t.Errorf("the terminator `::after` used height is %.1fpx — the "+
					"light/dark boundary is not painted", r.AfterH)
			} else if r.AfterH < r.DiscOpenH {
				t.Errorf("the terminator is %.1fpx tall on a %.1fpx disc — it is "+
					"declared with top:-1px/bottom:-1px and must overshoot, not fall short",
					r.AfterH, r.DiscOpenH)
			}

			// (4) THE INK CROSSES THE HORIZON. The claim the stills make is
			// about what a reader sees, and a reader sees the disc, not `.skb`.
			if r.DiscBelowHorizonPct < 20 || r.DiscBelowHorizonPct > 45 {
				t.Errorf("%.1f%% of the PAINTED disc lands below the band's bottom edge, "+
					"want roughly a third (20–45%%) — \"a third of each disc ends below "+
					"the horizon line at BOTH densities\" is a claim about ink",
					r.DiscBelowHorizonPct)
			}
		})
	}
}
