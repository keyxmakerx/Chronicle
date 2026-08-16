// sky_scenery_probe_test.go — THE BAND'S MOONS ARE WEATHER, NOT A CONTROL.
//
// C-CALV4-MOONS [MN-4] / [MN-G1] / [MN-G2]. The operator has said it three
// times: "The moons should be in the skypane but it's like the sun, and not
// interactable." The shipped band disagreed with them in HTML — block.templ
// puts the moon cluster inside `summary.skyhdr`, and a <summary> is
// a click target over its entire box, so every band moon was a hit target that
// toggled the pane. The scenery was a control.
//
// THE CLUSTER IS `.skscene` SINCE C-CALV4-SKY-CHEST STAGE 2, and the rename is
// the point rather than a tidy-up: `.skdiscs` was a FLEX ITEM of the lid — a
// permanent slot beside the clock — and `.skscene` is out-of-flow scenery
// painted onto the sky. THIS FILE'S TWO CLAIMS ARE UNCHANGED IN SUBSTANCE and
// they are the reason the spacer stopped being a hit island in that stage: at
// C1 the scenery sits directly over `.sksp`'s box, `pointer-events: none` on
// the moons only makes the hit fall THROUGH to whatever is beneath, and a live
// spacer beneath a moon is a clickable moon. MN-G1 is asserted here exactly as
// the operator wrote it; the sheet moved to keep it true.
//
// TWO CLAIMS, AND THE SECOND IS WHY THIS FILE IS NOT JUST A GREP.
//
//	MN-G1  no element in `.skscene` or its subtree is a click target: the
//	       subtree carries no <a>/<button>/<label>/<input>/[role=button]/
//	       [tabindex]/onclick/[data-cal-*], AND a real hit test at each disc's
//	       painted centre resolves to a node whose nearest interactive
//	       ancestor is not `summary.skyhdr`. A synthesized click there leaves
//	       the disclosure exactly as it found it.
//	MN-G2  the band STILL OPENS by a genuine hit-tested click at its caret, at
//	       all three densities, on the Harptos preset (intercalary, full tier).
//
// MN-G2 IS NOT OPTIONAL PADDING. C-CALV4-SKY-REPAIR defect (d) established
// that on the default preset the band could not be clicked open AT ALL,
// because a page-sized LABEL.dsel sat on top of it. Narrowing a too-wide hit
// target into a dead one is not progress, so the narrowing and the liveness
// are asserted in the same run, on the same DOM, at the same three widths.
//
// MN-G8: EVERY DIMENSION ASSERTION ASSERTS NON-ZERO FIRST. sky_measure_probe
// passed on a 0×0 disc for weeks. A hit test at the centre of a zero-size box
// is a hit test at an arbitrary page point and would "pass" for the wrong
// reason, so the disc's painted rect is checked before its centre is used.
//
// IT SKIPS HONESTLY under -short or with no Chromium, on this package's own
// precedent, and a skipped run is NOT a pass.
//
//	go test ./internal/widgets/calendar_block/ -run SkyScenery -v
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

// skySceneryDisc is one band disc, measured and hit-tested.
type skySceneryDisc struct {
	W float64 `json:"w"`
	H float64 `json:"h"`
	// Hit is the element the browser actually resolves at the disc's centre,
	// described as tag.class — the fact, not a boolean about the fact.
	Hit string `json:"hit"`
	// Interactive is the nearest interactive ancestor of Hit (self included),
	// described the same way, or "" when there is none.
	Interactive string `json:"interactive"`
	// OpenAfterClick is `details.open` after a click is dispatched on the
	// hit-tested node. Scenery leaves it false.
	OpenAfterClick bool `json:"openAfterClick"`
}

// skySceneryReading is one host width's whole verdict.
type skySceneryReading struct {
	Host    int    `json:"host"`
	Density string `json:"density"`
	// MN-G1, the cheap half: interactive descendants declared in the subtree.
	Forbidden []string `json:"forbidden"`
	// MN-G1, the expensive half: one entry per painted disc.
	Discs []skySceneryDisc `json:"discs"`
	// MN-G2 — the caret's own hit test and what a click there does.
	CaretW        float64 `json:"caretW"`
	CaretH        float64 `json:"caretH"`
	CaretHit      string  `json:"caretHit"`
	CaretOpened   bool    `json:"caretOpened"`
	CaretReclosed bool    `json:"caretReclosed"`
}

// skySceneryScript is the whole measurement, run once per page.
//
// The click is dispatched on the element the browser ITSELF resolved at the
// point, never on a node this script chose — that is the difference between a
// hit test and a wish.
const skySceneryScript = `function(){
  var INTERACTIVE = 'a[href],button,label,input,select,textarea,summary,[role=button],[tabindex],[onclick]';
  var root = document.querySelector('.probe-host');
  var sky  = root.querySelector('details.skygrow');
  var band = sky.querySelector('summary.skyhdr');
  var cluster = sky.querySelector('.skscene');
  var caret = sky.querySelector('.skcaret');
  var r1 = function(v){ return Math.round(v * 10) / 10; };
  var desc = function(el){
    if (!el) return '';
    var c = (el.getAttribute && el.getAttribute('class')) || '';
    return el.tagName.toLowerCase() + (c ? '.' + c.trim().split(/\s+/).join('.') : '');
  };
  var forbidden = [];
  if (cluster) {
    ['a','button','label','input','select','textarea','[role=button]','[tabindex]','[onclick]']
      .forEach(function(sel){
        if (cluster.querySelector(sel) || (cluster.matches && cluster.matches(sel))) forbidden.push(sel);
      });
    // [data-cal-*] action hooks: the Block's own namespace for "this does
    // something". Scenery declares none.
    var all = [cluster].concat([].slice.call(cluster.querySelectorAll('*')));
    all.forEach(function(el){
      [].slice.call(el.attributes || []).forEach(function(a){
        if (a.name.indexOf('data-cal-') === 0) forbidden.push(a.name);
      });
    });
  }
  var discs = [];
  [].slice.call(sky.querySelectorAll('.skscene .ph')).forEach(function(d){
    var r = d.getBoundingClientRect();
    var entry = { w: r1(r.width), h: r1(r.height), hit: '', interactive: '', openAfterClick: false };
    if (r.width > 0 && r.height > 0) {
      var hit = document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2);
      entry.hit = desc(hit);
      var near = hit && hit.closest ? hit.closest(INTERACTIVE) : null;
      entry.interactive = desc(near);
      var was = sky.open;
      sky.open = false;
      if (hit) hit.click();
      entry.openAfterClick = sky.open;
      sky.open = was;
    }
    discs.push(entry);
  });
  var cr = caret ? caret.getBoundingClientRect() : null;
  var caretHit = null, opened = false, reclosed = false;
  if (cr && cr.width > 0 && cr.height > 0) {
    caretHit = document.elementFromPoint(cr.left + cr.width / 2, cr.top + cr.height / 2);
    sky.open = false;
    if (caretHit) caretHit.click();
    opened = sky.open;
    if (caretHit) caretHit.click();
    reclosed = !sky.open;
    sky.open = false;
  }
  return {
    host: Math.round(root.getBoundingClientRect().width),
    density: getComputedStyle(band).justifyContent === 'flex-end' ? 'C3 seal' : 'C1 horizon',
    forbidden: forbidden,
    discs: discs,
    caretW: cr ? r1(cr.width) : -1,
    caretH: cr ? r1(cr.height) : -1,
    caretHit: desc(caretHit),
    caretOpened: opened,
    caretReclosed: reclosed
  };
}`

// TestSkySceneryProbe_TheBandsMoonsAreNotAControl is MN-G1 and MN-G2, together.
func TestSkySceneryProbe_TheBandsMoonsAreNotAControl(t *testing.T) {
	if testing.Short() {
		t.Skip("the scenery probe needs a browser; skipped under -short (CI's mode)")
	}
	chrome := findProbeChromium()
	if chrome == "" {
		t.Skip("no Chromium binary found (set CHROMIUM_BIN) — a skipped probe is NOT a pass")
	}

	// The Harptos preset, seated: intercalary month, full tier, three moons.
	d := fxSky(t, true)
	markup := regexp.MustCompile(`<link[^>]*>`).ReplaceAllString(render(t, d), "")

	hosts := []struct {
		name    string
		width   int
		density string
	}{
		{"1440 viewport / wide column", 1440, "C1 horizon"},
		{"768 viewport / wide column", 768, "C1 horizon"},
		{"390 viewport / the Bench's 358px column", 358, "C3 seal"},
	}

	for _, h := range hosts {
		t.Run(h.name, func(t *testing.T) {
			// ONE HOST PER PAGE, deliberately: elementFromPoint is viewport
			// relative and returns null outside it, so a stack of three hosts
			// would silently turn the third row's hit tests into nothing.
			page := `<!doctype html><html><head><meta charset="utf-8"><style>` +
				`html,body{margin:0;background:#fff}` +
				blockCSS(t) + skyProbeBenchCSS(t) +
				`.probe-host{display:block;margin:8px;max-width:none;width:` +
				fmt.Sprint(h.width) + `px}` +
				`</style></head><body>` +
				`<div class="probe-host cal-bench">` + markup + `</div>` +
				`<script>document.addEventListener('DOMContentLoaded',function(){` +
				`setTimeout(function(){var read=` + skySceneryScript + `;` +
				`document.body.setAttribute('data-probe', JSON.stringify(read()));},400);});</script>` +
				`</body></html>`

			path := filepath.Join(t.TempDir(), "sky-scenery.html")
			if err := os.WriteFile(path, []byte(page), 0o644); err != nil { //nolint:gosec // test artefact
				t.Fatalf("write probe page: %v", err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			out, err := exec.CommandContext(ctx, chrome,
				"--headless", "--no-sandbox", "--disable-gpu", "--hide-scrollbars",
				fmt.Sprintf("--window-size=%d,1200", h.width+64),
				"--virtual-time-budget=5000", "--dump-dom", "file://"+path).Output()
			if err != nil {
				t.Fatalf("chromium: %v", err)
			}
			m := probePayloadRe.FindStringSubmatch(string(out))
			if m == nil {
				t.Fatal("no probe payload — the page script did not run")
			}
			var r skySceneryReading
			if err := json.Unmarshal([]byte(html.UnescapeString(m[1])), &r); err != nil {
				t.Fatalf("probe payload: %v", err)
			}

			t.Logf("host %dpx · %s · %d discs · caret %.1f×%.1fpx hit %q",
				r.Host, r.Density, len(r.Discs), r.CaretW, r.CaretH, r.CaretHit)
			for i, dd := range r.Discs {
				t.Logf("  disc %d · %.1f×%.1fpx · elementFromPoint → %s · nearest interactive → %q · opens: %v",
					i, dd.W, dd.H, dd.Hit, dd.Interactive, dd.OpenAfterClick)
			}

			if r.Density != h.density {
				t.Fatalf("density = %q, want %q — the host is not the column this row "+
					"is about and every claim below would be about the wrong band",
					r.Density, h.density)
			}
			if len(r.Discs) == 0 {
				t.Fatal("the band's cluster holds no `.ph` at all — the fixture lost " +
					"its almanac and MN-G1 would pass vacuously")
			}

			// ── MN-G1, the declared half ────────────────────────────────────
			if len(r.Forbidden) > 0 {
				t.Errorf("`.skscene` declares interactive descendants %v — the band's "+
					"moons are scenery, like the sun ([MN-4]); they report the sky by "+
					"being legible, not by being pressable", r.Forbidden)
			}

			// ── MN-G1, the hit-tested half ──────────────────────────────────
			for i, dd := range r.Discs {
				// MN-G8: the painted box first. A centre computed from a 0×0
				// rect is a point somewhere else on the page.
				if dd.W <= 0 || dd.H <= 0 {
					t.Errorf("disc %d paints %.1f×%.1fpx — a hit test at the centre of a "+
						"zero-size box is a hit test at an arbitrary page point, so this "+
						"row's verdict would be about nothing (MN-G8)", i, dd.W, dd.H)
					continue
				}
				if dd.Interactive == "summary.skyhdr" {
					t.Errorf("disc %d: elementFromPoint resolves to %s, whose nearest "+
						"interactive ancestor is `summary.skyhdr` — the band's moon IS a "+
						"click target, which is the conflation the operator has corrected "+
						"three times ([MN-4], block.templ:157)", i, dd.Hit)
				}
				if dd.OpenAfterClick {
					t.Errorf("disc %d: a click on the node the browser resolved there "+
						"TOGGLED the disclosure. The band's opening is the band's "+
						"affordance, not the moons' — the caret is the control and the "+
						"discs are the weather ([MN-4])", i)
				}
			}

			// ── MN-G2 — and the band is still alive ─────────────────────────
			if r.CaretW <= 0 || r.CaretH <= 0 {
				t.Fatalf("the caret paints %.1f×%.1fpx — MN-G2 cannot be asserted against "+
					"a box that is not there (MN-G8)", r.CaretW, r.CaretH)
			}
			if !r.CaretOpened {
				t.Errorf("a genuine hit-tested click at the caret's centre (which resolved "+
					"to %s) did NOT open the band. C-CALV4-SKY-REPAIR defect (d) is exactly "+
					"this failure, and trading a too-wide hit target for a dead one is not "+
					"progress ([MN-G2])", r.CaretHit)
			}
			if !r.CaretReclosed {
				t.Errorf("the band opened at the caret but a second click there did not " +
					"close it — a disclosure that only opens is half a control ([MN-G2])")
			}
		})
	}
}

// TestSkyScenery_TheClusterCarriesNoActionHookInTheMarkup is MN-G1's cheap half
// as a pure render assertion, so a CI run with no browser still fails on the
// grep-able form of the defect. A probe that is skipped is not a pass, and this
// is the part that never needs one.
func TestSkyScenery_TheClusterCarriesNoActionHookInTheMarkup(t *testing.T) {
	body := flatten(render(t, fxSky(t, true)))
	start := strings.Index(body, `<span class="skscene">`)
	if start < 0 {
		t.Fatal("no `.skscene` in the seated Block's markup — the band lost its moons")
	}
	rest := body[start:]
	end := strings.Index(rest, `</span><span class="sksp">`)
	if end < 0 {
		t.Fatal("could not bound the `.skscene` subtree — the band's markup changed shape")
	}
	subtree := rest[:end]
	for _, bad := range []string{"<a ", "<button", "<label", "<input", `role="button"`, "tabindex", "onclick", "data-cal-"} {
		if strings.Contains(subtree, bad) {
			t.Errorf("the band's disc cluster carries %q. It is scenery, like the sun "+
				"([MN-4]) — a `title` is a tooltip and stays; anything that makes it "+
				"pressable does not:\n%s", bad, subtree)
		}
	}
}

// TestCSS_TheIntercalaryDayPositionsItsOwnStretchedLabel is MN-G2's other half,
// browser-free, so a CI run with no Chromium still fails on the defect.
//
// `.cell` is `position: relative` BECAUSE dayPick's `LABEL.dsel` is
// `position: absolute; inset: 0` and needs a containing block. The intercalary
// day emits the same control (instrument.templ, intercalaryRows → dayPick) into
// a box that had none, so the label escaped to the initial containing block and
// painted the whole viewport — 1504×1113px, measured on the Harptos preset at
// 1440, sitting over the sky band's caret. The two facts belong together and
// this test is where they are stated together.
func TestCSS_TheIntercalaryDayPositionsItsOwnStretchedLabel(t *testing.T) {
	sheet := blockCSS(t)
	for _, sel := range []string{".cal-block-host .cell {", ".cal-block-host .interc {"} {
		i := strings.Index(sheet, sel)
		if i < 0 {
			t.Fatalf("no %q rule in calendar-block.css — the guard cannot find its subject", sel)
		}
		end := strings.Index(sheet[i:], "}")
		if end < 0 {
			t.Fatalf("unterminated %q rule", sel)
		}
		if !strings.Contains(sheet[i:i+end], "position: relative") {
			t.Errorf("%q does not declare `position: relative`. It hosts dayPick's "+
				"`LABEL.dsel` (`position: absolute; inset: 0`), and without a containing "+
				"block that label resolves to the INITIAL one and stretches across the "+
				"whole viewport — swallowing the sky band's caret, the nameplate and the "+
				"Ledger alike (C-CALV4-SKY-REPAIR defect (d); C-CALV4-MOONS MN-G2)", sel)
		}
	}
}
