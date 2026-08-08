// sky_measure_probe_test.go — [SKY-15] SIGNED: the three failure counts as
// NUMBERS, measured against the SHIPPED element in a real layout engine.
//
// WHY A BROWSER. The operator's verdict on the old sky was three counts —
// "badly implemented mostly with the animations and colors", "really big and
// bulky, took a lot of room, and was weirdly laid out on mobile", "just god
// awful" — and [SKY-15] turns each into a number because "a count that is
// argued rather than measured is the count failing". None of the three can be
// read off source text:
//
//   - COUNT 2 is a rendered height, and a CSS grep would only prove what the
//     token SAYS. index.md measured the stills' bands twice, independently —
//     once from the custom property and once from getBoundingClientRect — and
//     this probe reproduces both readings against this build.
//   - COUNT 1's hardest half is LAYOUT-INDUCED TRAVEL, which is invisible to a
//     walk of `transition-property`. The drawing pass caught the clock sliding
//     65.0px sideways on every open and close with every property correctly
//     declared and no transform anywhere. So each fact's ANCHORED EDGE is
//     sampled closed and open, and the displacement is printed.
//   - the disc CROSSING is the other half: the drawing pass's round 2 claimed
//     "half in the sky and half on the page" and measured 17.2%. The claim is
//     a third of each disc below the horizon; the probe measures it.
//
// THE WIDTHS ARE THE BLOCK'S COLUMN, NOT THE VIEWPORT, AND THAT IS [SKY-8]'s
// ANSWER RATHER THAN A CONVENIENCE. The density switch is `@container cal-block`
// at the Block's existing `min-width: 481px` boundary, so the seal answers to
// the box the band occupies and never to the window it sits in. 358px is the
// column a Bench Block measures at a 390px viewport.
//
// IT SKIPS HONESTLY when it cannot do that — under -short, or with no Chromium
// present — and a skipped run is NOT a pass.
//
//	go test ./internal/widgets/calendar_block/ -run SkyMeasure -v
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

// skyReading is one host's measurements, closed and open.
type skyReading struct {
	Host int `json:"host"`
	// COUNT 2 — the closed band, read TWICE INDEPENDENTLY: once from the
	// custom property that sizes it, once from the laid-out box.
	BandTokenPx float64 `json:"bandTokenPx"`
	BandRectPx  float64 `json:"bandRectPx"`
	// The density this host actually resolved to.
	Density string `json:"density"`
	// The OPEN total, which count 2 is also about: the pane is the thing being
	// beaten, not only the strip.
	SkyClosedTotalPx float64 `json:"skyClosedTotalPx"`
	SkyOpenTotalPx   float64 `json:"skyOpenTotalPx"`
	// COUNT 1 — each fact's ANCHORED EDGE, closed and open, and the disc's
	// geometry at both ends.
	TimeAnchorClosed   float64 `json:"timeAnchorClosed"`
	TimeAnchorOpen     float64 `json:"timeAnchorOpen"`
	SeasonAnchorClosed float64 `json:"seasonAnchorClosed"`
	SeasonAnchorOpen   float64 `json:"seasonAnchorOpen"`
	DiscAnchorClosed   float64 `json:"discAnchorClosed"`
	DiscAnchorOpen     float64 `json:"discAnchorOpen"`
	DiscSizeClosed     float64 `json:"discSizeClosed"`
	DiscSizeOpen       float64 `json:"discSizeOpen"`
	// The share of the open disc that lands BELOW the band's own bottom edge.
	DiscBelowHorizonPct float64 `json:"discBelowHorizonPct"`
	// COUNT 3 — interactive controls inside the sky, from the live DOM.
	Controls int `json:"controls"`
}

const skyProbeScript = `function(root){
  var sky = root.querySelector('details.skygrow');
  var band = sky && sky.querySelector('summary.skyhdr');
  var read = function(el){ return el ? el.getBoundingClientRect() : null; };
  // The ANCHORED EDGE of each fact. At the horizon-line density the band is
  // left-packed, so the anchored edge is the LEFT one; at the seal it packs
  // right and the anchored edge is the RIGHT one. Reading the wrong edge would
  // report a growing element as travel, which is the opposite of the claim.
  var wide = sky ? getComputedStyle(band).justifyContent !== 'flex-end' : true;
  var anchor = function(sel){
    var r = read(sky && sky.querySelector(sel));
    if (!r) return -1;
    return Math.round((wide ? r.left : r.right) * 10) / 10;
  };
  var sample = function(){
    var d = read(sky && sky.querySelector('.skb'));
    return {
      time:   anchor('.sktime'),
      season: anchor('.skseason'),
      disc:   anchor('.skdiscs'),
      discSize: d ? Math.round(d.height * 10) / 10 : -1,
      discBottom: d ? d.bottom : 0,
      bandBottom: read(band) ? read(band).bottom : 0,
      total: read(sky) ? Math.round(read(sky).height * 10) / 10 : -1
    };
  };
  if (!root.__skyClosed) { root.__skySample = sample; return null; }
  var closed = root.__skyClosed;
  var open = root.__skyOpen;
  var crossing = open.discSize > 0
    ? Math.round((open.discBottom - open.bandBottom) / open.discSize * 1000) / 10
    : -1;
  var bandToken = parseFloat(getComputedStyle(sky).getPropertyValue('--bandh'));
  return {
    host: Math.round(root.getBoundingClientRect().width),
    bandTokenPx: bandToken,
    bandRectPx: Math.round(read(band).height * 10) / 10,
    density: getComputedStyle(band).justifyContent === 'flex-end' ? 'C3 seal' : 'C1 horizon',
    skyClosedTotalPx: closed.total,
    skyOpenTotalPx: open.total,
    timeAnchorClosed: closed.time, timeAnchorOpen: open.time,
    seasonAnchorClosed: closed.season, seasonAnchorOpen: open.season,
    discAnchorClosed: closed.disc, discAnchorOpen: open.disc,
    discSizeClosed: closed.discSize, discSizeOpen: open.discSize,
    discBelowHorizonPct: crossing,
    controls: sky.querySelectorAll('input,button,a,select,textarea').length
  };
}`

// TestSkyMeasureProbe_TheThreeCounts is the whole of [SKY-15].
func TestSkyMeasureProbe_TheThreeCounts(t *testing.T) {
	if testing.Short() {
		t.Skip("the sky measurement probe needs a browser; skipped under -short (CI's mode)")
	}
	chrome := findProbeChromium()
	if chrome == "" {
		t.Skip("no Chromium binary found (set CHROMIUM_BIN) — a skipped probe is NOT a pass")
	}

	hosts := []struct {
		name    string
		width   int
		density string
		budget  float64 // the closed-band ceiling this width answers to
		wantPx  float64 // the signed still's measured height
	}{
		{"1440 viewport / wide column", 1440, "C1 horizon", 44, 40},
		{"768 viewport / wide column", 768, "C1 horizon", 44, 40},
		{"390 viewport / the Bench's 358px column", 358, "C3 seal", 36, 32},
	}

	d := fxSky(t, true)
	var boxes strings.Builder
	for i, h := range hosts {
		markup := regexp.MustCompile(`<link[^>]*>`).ReplaceAllString(render(t, d), "")
		fmt.Fprintf(&boxes, `<div class="probe-host cal-bench" id="h%d" style="width:%dpx">%s</div>`,
			i, h.width, markup)
	}

	// BOTH sheets are inlined, and that pairing is the point: the band's markup
	// is the Block's and its rules are the Bench's ([SKY-2]), so a probe with
	// only one of them would measure an unstyled strip and call it 0px.
	page := `<!doctype html><html><head><meta charset="utf-8"><style>` +
		`html,body{margin:0;background:#fff}` +
		blockCSS(t) + skyProbeBenchCSS(t) +
		// AFTER the sheets, so it wins on order: .cal-bench carries the page's
		// own 1180px measure, which is a real product constraint and not this
		// probe's subject. The subject is the width of the COLUMN the band sits
		// in, so the host states one and the measure is lifted off it.
		`.probe-host{display:block;margin:24px;max-width:none}` +
		`</style></head><body>` + boxes.String() +
		`<script>document.addEventListener('DOMContentLoaded',function(){` +
		`var read=` + skyProbeScript + `;` +
		`var hosts=[].slice.call(document.querySelectorAll('.probe-host'));` +
		// THE OPEN STATE IS SAMPLED AFTER THE ENVELOPE HAS RUN, NOT ON THE NEXT
		// LINE. The carve-out's open total is 200ms; a synchronous read after
		// setAttribute('open') samples the FIRST frame and reports the discs at
		// their closed diameter, which is a probe measuring its own impatience.
		// 600ms of virtual time is three envelopes.
		`hosts.forEach(function(root){read(root);` +
		`root.__skyClosed=root.__skySample();` +
		`root.querySelector('details.skygrow').setAttribute('open','');});` +
		`setTimeout(function(){` +
		`hosts.forEach(function(root){root.__skyOpen=root.__skySample();});` +
		`var out=hosts.map(read);` +
		`document.body.setAttribute('data-probe', JSON.stringify(out));},600);});</script>` +
		`</body></html>`

	path := filepath.Join(t.TempDir(), "sky.html")
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
	var readings []skyReading
	if err := json.Unmarshal([]byte(html.UnescapeString(m[1])), &readings); err != nil {
		t.Fatalf("probe payload: %v", err)
	}
	if len(readings) != len(hosts) {
		t.Fatalf("probe returned %d readings for %d hosts", len(readings), len(hosts))
	}

	for i, h := range hosts {
		r := readings[i]
		t.Run(h.name, func(t *testing.T) {
			t.Logf("host %dpx · %s · closed band %.1fpx (token %.0fpx) · "+
				"sky closed %.1fpx → open %.1fpx · discs %.1f→%.1fpx, %.1f%% below the "+
				"horizon · controls %d",
				r.Host, r.Density, r.BandRectPx, r.BandTokenPx,
				r.SkyClosedTotalPx, r.SkyOpenTotalPx,
				r.DiscSizeClosed, r.DiscSizeOpen, r.DiscBelowHorizonPct, r.Controls)

			// COUNT 2 — big and bulky. TWO INDEPENDENT READINGS, and they must
			// AGREE: a token that says 40 while the box lays out at 52 is the
			// defect a CSS grep cannot see.
			if r.Density != h.density {
				t.Errorf("density = %q, want %q — the C1/C3 crossover is the Block's "+
					"existing min-width:481px boundary and there is no fifth number",
					r.Density, h.density)
			}
			if r.BandTokenPx != h.wantPx {
				t.Errorf("--bandh = %.0fpx, want %.0fpx (the signed still's measurement)",
					r.BandTokenPx, h.wantPx)
			}
			if diff := r.BandRectPx - r.BandTokenPx; diff > 0.6 || diff < -0.6 {
				t.Errorf("the two readings disagree: token %.1fpx vs laid-out %.1fpx — "+
					"index.md's whole discipline is that the property that SIZES the band "+
					"and the box that RESULTS are measured separately", r.BandTokenPx, r.BandRectPx)
			}
			if r.BandRectPx > h.budget {
				t.Errorf("the closed band measures %.1fpx against a %.0fpx budget — count 2 "+
					"is \"really big and bulky\" and this is the number it becomes",
					r.BandRectPx, h.budget)
			}
			if r.SkyOpenTotalPx <= r.SkyClosedTotalPx {
				t.Errorf("the sky opens to %.1fpx from a closed %.1fpx — the expansion has "+
					"no height and the pane is not revealing", r.SkyOpenTotalPx, r.SkyClosedTotalPx)
			}

			// COUNT 1 — the anchored-edge travel. Every fact has ONE anchored
			// edge and its displacement across a full open is a NUMBER, because
			// the drawing pass caught a 65px slide that a walk of
			// `transition-property` could not see.
			for _, fact := range []struct {
				name         string
				closed, open float64
			}{
				{"the clock", r.TimeAnchorClosed, r.TimeAnchorOpen},
				{"the season word", r.SeasonAnchorClosed, r.SeasonAnchorOpen},
				{"the disc cluster", r.DiscAnchorClosed, r.DiscAnchorOpen},
			} {
				travel := fact.open - fact.closed
				if travel < 0 {
					travel = -travel
				}
				t.Logf("  anchored-edge travel · %s: %.1fpx", fact.name, travel)
				if travel > 2 {
					t.Errorf("%s travels %.1fpx across a full open — layout-induced travel "+
						"is invisible to a walk of transition-property and is exactly the "+
						"defect the drawing pass caught twice", fact.name, travel)
				}
			}

			// The discs GROW IN PLACE and cross the horizon. "A third of each
			// disc below the horizon line" is the claim; round 2's prose said
			// half and the pixels said 17.2%, which is why this is measured.
			if r.DiscSizeOpen <= r.DiscSizeClosed {
				t.Errorf("the discs do not grow: %.1f → %.1fpx", r.DiscSizeClosed, r.DiscSizeOpen)
			}
			if r.DiscBelowHorizonPct < 25 || r.DiscBelowHorizonPct > 45 {
				t.Errorf("%.1f%% of each open disc lands below the horizon line, want about a "+
					"third — the discs grow DOWNWARD onto the pane's own paper, and the "+
					"arrival is a change of scale and of ground rather than a caption",
					r.DiscBelowHorizonPct)
			}

			// COUNT 3 — from the LIVE DOM this time, not from the markup string.
			if r.Controls != 3 {
				t.Errorf("the open sky carries %d interactive controls, want exactly 3 — "+
					"the Tonight/Month/Moons trio, and the band itself is the disclosure "+
					"and does not count against them", r.Controls)
			}
		})
	}
}

// skyProbeBenchCSS reads calendar-bench.css. The sky's rules are authored there
// even though its markup is the Block's ([SKY-2]), which is the one thing about
// this surface a reader has to hold in mind, so the probe states it by needing
// both files.
func skyProbeBenchCSS(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "..", "static", "css", "calendar-bench.css"))
	if err != nil {
		t.Fatalf("read calendar-bench.css: %v", err)
	}
	return string(body)
}
