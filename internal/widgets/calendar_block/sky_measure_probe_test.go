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
	//
	// `BandRectPx` IS SAMPLED CLOSED, WHICH IT WAS NOT BEFORE. Until
	// C-CALV4-SKY-CHEST stage 2 this field was filled by the final `read(root)`
	// call — which runs with the band OPEN — and compared against `--bandh`,
	// the CLOSED token. It agreed for the whole life of the probe only because
	// the band had one height in both states. The moment the lid started
	// collapsing on open (40→28 / 32→22, measured) the two readings disagreed
	// by exactly the collapse, and the failure was the probe's, not the
	// sheet's. It now reads the state its own header names, and the OPEN
	// height is a second reading with its own token.
	BandTokenPx float64 `json:"bandTokenPx"`
	BandRectPx  float64 `json:"bandRectPx"`
	// THE LID, OPEN — the chest's own movement, in pixels. --lidh sizes it and
	// the laid-out box is read separately, on the same discipline as the pair
	// above.
	LidTokenPx  float64 `json:"lidTokenPx"`
	LidOpenPx   float64 `json:"lidOpenPx"`
	// THE SCENERY'S SEAT, closed and open. `SceneToFacts` is the inline gap
	// between the moons and the nearest of the lid's own facts — the moons are
	// painted BEHIND the text (z-index: -1), so a negative number is a real
	// overlap and not a paint-order accident. `SceneReach` is how far the
	// scenery's leading edge sits from the band's TRAILING edge, which is the
	// axis --seal-solid is measured on: at the seal the sky stops at 210px and
	// a moon beyond that is painted on masked-out nothing.
	SceneToFactsClosed float64 `json:"sceneToFactsClosed"`
	SceneToFactsOpen   float64 `json:"sceneToFactsOpen"`
	SceneReachClosed   float64 `json:"sceneReachClosed"`
	SealSolidPx        float64 `json:"sealSolidPx"`
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
  var r1 = function(v){ return Math.round(v * 10) / 10; };
  var sample = function(){
    var d = read(sky && sky.querySelector('.skb'));
    var b = read(band);
    var scene = read(sky && sky.querySelector('.skscene'));
    // The first of the lid's own facts on the inline axis. The scenery LEADS
    // and the facts FOLLOW at BOTH densities — that is the reference bar's own
    // arrangement and the seat is authored to keep it — so the gap is read
    // from the same pair of edges either way, and a density that inverted the
    // order would show up here as a large negative rather than being papered
    // over by a conditional.
    var fact = read(sky && sky.querySelector('.sktime'));
    return {
      time:   anchor('.sktime'),
      season: anchor('.skseason'),
      disc:   anchor('.skscene'),
      discSize: d ? r1(d.height) : -1,
      discBottom: d ? d.bottom : 0,
      bandBottom: b ? b.bottom : 0,
      bandH: b ? r1(b.height) : -1,
      sceneToFacts: (scene && fact) ? r1(fact.left - scene.right) : -1,
      sceneReach: (scene && b) ? r1(b.right - scene.left) : -1,
      total: read(sky) ? r1(read(sky).height) : -1
    };
  };
  if (!root.__skyClosed) { root.__skySample = sample; return null; }
  var closed = root.__skyClosed;
  var open = root.__skyOpen;
  var crossing = open.discSize > 0
    ? Math.round((open.discBottom - open.bandBottom) / open.discSize * 1000) / 10
    : -1;
  var bandToken = parseFloat(getComputedStyle(sky).getPropertyValue('--bandh'));
  var lidToken = parseFloat(getComputedStyle(sky).getPropertyValue('--lidh'));
  return {
    host: Math.round(root.getBoundingClientRect().width),
    bandTokenPx: bandToken,
    bandRectPx: closed.bandH,
    lidTokenPx: lidToken,
    lidOpenPx: open.bandH,
    sceneToFactsClosed: closed.sceneToFacts,
    sceneToFactsOpen: open.sceneToFacts,
    sceneReachClosed: closed.sceneReach,
    sealSolidPx: root.__skySealSolid,
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
		wantLid float64 // --lidh: what the lid collapses TO on open
	}{
		{"1440 viewport / wide column", 1440, "C1 horizon", 44, 40, 28},
		{"768 viewport / wide column", 768, "C1 horizon", 44, 40, 28},
		{"390 viewport / the Bench's 358px column", 358, "C3 seal", 36, 32, 22},
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
		// THE SEAL'S REACH IS READ CLOSED, which is the only state it means
		// anything in: --seal-solid sweeps to 100% on open, so a reading taken
		// afterwards would say the sky covers everything and the scenery could
		// never be found off it.
		`root.__skySealSolid=parseFloat(getComputedStyle(` +
		`root.querySelector('details.skygrow')).getPropertyValue('--seal-solid'));` +
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
				"LID open %.1fpx (token %.0fpx) · sky closed %.1fpx → open %.1fpx · "+
				"discs %.1f→%.1fpx, %.1f%% below the horizon · scenery-to-facts "+
				"%.1f→%.1fpx, reach %.1fpx of a %.0f seal · controls %d",
				r.Host, r.Density, r.BandRectPx, r.BandTokenPx,
				r.LidOpenPx, r.LidTokenPx,
				r.SkyClosedTotalPx, r.SkyOpenTotalPx,
				r.DiscSizeClosed, r.DiscSizeOpen, r.DiscBelowHorizonPct,
				r.SceneToFactsClosed, r.SceneToFactsOpen,
				r.SceneReachClosed, r.SealSolidPx, r.Controls)

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

			// ── THE LID (C-CALV4-SKY-CHEST stage 2) ────────────────────────
			// The chest opens by the lid giving up its own height, and that is
			// the whole reason this surface needs no `transform`: the pane and
			// everything in it rises by exactly the pixels the lid loses. The
			// same two-readings discipline applies — the token that sizes it
			// and the box that results are read separately, because "the lid
			// collapses" is a claim about a laid-out box and a declaration is
			// not a box.
			if r.LidTokenPx != h.wantLid {
				t.Errorf("--lidh = %.0fpx, want %.0fpx", r.LidTokenPx, h.wantLid)
			}
			if diff := r.LidOpenPx - r.LidTokenPx; diff > 0.6 || diff < -0.6 {
				t.Errorf("the lid's two readings disagree: token %.1fpx vs laid-out %.1fpx",
					r.LidTokenPx, r.LidOpenPx)
			}
			if rise := r.BandRectPx - r.LidOpenPx; rise < 6 {
				t.Errorf("the lid gives up %.1fpx on open (%.1f → %.1f) — under about six "+
					"the chest does not read as opening at all, and this surface has no "+
					"`transform` to fall back on ([SKY-3] refuses it by name)",
					rise, r.BandRectPx, r.LidOpenPx)
			}
			// AND THE RISE IS THE WHOLE OF THE DIFFERENCE, not a coincidence:
			// the sky's open total must have grown by the pane's height MINUS
			// the lid's collapse. Stated as an inequality rather than an exact
			// figure, because the pane's own height is content-dependent and is
			// not this probe's subject.
			if r.SkyOpenTotalPx-r.SkyClosedTotalPx <= r.BandRectPx-r.LidOpenPx {
				t.Errorf("the sky grew %.1fpx while the lid gave up %.1fpx — the lid is "+
					"collapsing faster than the pane is revealing, which is a header "+
					"eating itself rather than a chest opening",
					r.SkyOpenTotalPx-r.SkyClosedTotalPx, r.BandRectPx-r.LidOpenPx)
			}

			// ── THE SCENERY IS ON THE SKY, AND BEHIND THE FACTS ────────────
			// Two failures the move into the sky could produce, neither of
			// which any string in the sheet would show:
			//
			//  (a) the moons painted OVER the clock. They sit at z-index -1 so
			//      the text wins the paint, but a moon behind the clock's
			//      glyphs is still a moon nobody can read, and at the seal the
			//      band is only ~336px wide with the facts taking ~130 of it.
			//  (b) the moons painted OFF the sky. At C3 the field stops at
			//      --seal-solid (210px measured from the trailing edge) and
			//      everything past it is masked to nothing — a moon out there
			//      is drawn on page white. This is the one number the seat's
			//      inset was chosen against, so it is measured, not trusted.
			for _, s := range []struct {
				state string
				gap   float64
			}{{"closed", r.SceneToFactsClosed}, {"open", r.SceneToFactsOpen}} {
				if s.gap < 4 {
					t.Errorf("%s: the scenery's inline gap to the lid's clock is %.1fpx — "+
						"the moons are painted behind the lid's own facts (z-index: -1), so "+
						"this is a moon under the text rather than a moon in the sky",
						s.state, s.gap)
				}
			}
			if r.Density == "C3 seal" {
				if r.SceneReachClosed > r.SealSolidPx {
					t.Errorf("the scenery's leading edge sits %.1fpx in from the band's "+
						"trailing edge while the sky's solid reach is only %.0fpx — at the "+
						"seal the field is masked out past that line, so the moons are "+
						"painted on page white rather than on sky",
						r.SceneReachClosed, r.SealSolidPx)
				}
				if r.SceneReachClosed <= 0 {
					t.Errorf("the scenery's reach reads %.1fpx — the cluster was not found "+
						"or lies outside the band entirely", r.SceneReachClosed)
				}
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
			// THE WINDOW MOVED, AND IT MOVED BY ARITHMETIC RATHER THAN BY
			// TASTE — C-CALV4-SKY-CHEST stage 2. It was 25–45%, and 33.8%/32.8%
			// was measured against a lid that had ONE height in both states.
			// The moon's own geometry is byte-unchanged: same top anchor
			// ((--bandh − --dsz0) / 2), same 13→40 / 11→32 diameters. What
			// moved is the line it is measured against — the horizon is the
			// lid's bottom edge, and the lid now collapses 12px (C1) / 10px
			// (C3) on open, so the same moon necessarily hangs further below
			// it. The new figures are forced, not chosen:
			//
			//	C1  (13.5 + 40 − 28) / 40 = 63.8%     measured 63.8%
			//	C3  (10.5 + 32 − 22) / 32 = 64.1%     measured 64.1%
			//
			// THE CLAIM THAT SURVIVES is the one the still was making: the moon
			// is not penned inside the strip — it crosses, it grows downward
			// onto the pane's own paper, and it stays a moon in a sky rather
			// than dropping out the bottom of the field. So the window is a
			// window and not a point, and it is bounded at BOTH ends: under a
			// third and the moon is an ornament sitting on a bar again; past
			// four fifths and it has fallen through the sky's lower dissolve.
			//
			// A LATER HAND CHANGING --lidh MUST COME BACK HERE. That is the
			// intended coupling: the two numbers are one geometry, and a
			// silently re-timed lid that left this window alone would be
			// claiming a crossing the build no longer has.
			if r.DiscBelowHorizonPct < 33 || r.DiscBelowHorizonPct > 80 {
				t.Errorf("%.1f%% of each open disc lands below the horizon line, want "+
					"33–80%% — the moons grow DOWNWARD past the lid's bottom edge onto the "+
					"pane's own paper, and with the lid collapsing %0.1fpx on open the "+
					"figure is arithmetic: (top anchor + open diameter − open lid) / "+
					"open diameter", r.DiscBelowHorizonPct, r.BandRectPx-r.LidOpenPx)
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
