// legend_disclosure_probe_test.go — DOES THE LEGEND ACTUALLY OPEN, ON THE THREE
// DEVICES IT CLAIMS TO? (C-CALV4-TILES §9.4.)
//
// WHY A BROWSER. legend_disclosure_test.go can prove that a checkbox exists,
// that a label addresses it and that four rules read it. It cannot prove that a
// thumb opens the zone, that the keyboard can reach the control, or that the tab
// is big enough to hit — those are claims about a rendered layout under a
// particular pointer, and every one of them has a plausible-looking failure that
// leaves the stylesheet correct:
//
//   - :focus-within instead of :focus-visible reads identically in review and
//     leaves the tab unable to CLOSE on a touch device, because clicking a label
//     focuses its input;
//   - an ungated hover branch reads identically and is the whole affordance on a
//     desktop and none of it on a phone;
//   - a 22px tab reads identically and is half the 44px floor.
//
// So this drives a real Chromium, twice: once with a fine pointer and once with
// a genuinely coarse one (`--blink-settings=primaryPointerType=2,…`, the switch
// moon_reach_probe_test.go verified against chromium-1194 after this package had
// spent a wave believing no such switch existed).
//
// THE PASS ORDER IS LOAD-BEARING and it is the same finding moonReachOpenScript
// records: Chromium's :focus-visible heuristic is document-wide, so the moment
// anything is click()ed, every later programmatic .focus() stops matching it.
// Rest is measured first, then keyboard, then the tap.
//
// IT SKIPS HONESTLY under -short or with no Chromium, and a skipped run is NOT a
// pass. Registered in tools/check-browser-probes.sh.
//
//	go test ./internal/widgets/calendar_block/ -run LegendProbe -v
package calendar_block

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ldReading is one host's three states plus the tab's own geometry.
type ldReading struct {
	Found  bool   `json:"found"`
	Why    string `json:"why"`
	Coarse bool   `json:"coarse"`

	// the three states, each as the disclosed body's own rendered box
	RestOpen bool `json:"restOpen"`
	KeyOpen  bool `json:"keyOpen"`
	TapOpen  bool `json:"tapOpen"`
	// a SECOND tap must close it again. This is the :focus-within defect's own
	// arm: under :focus-within the box un-checks and the zone stays open.
	TapTwiceOpen bool `json:"tapTwiceOpen"`

	// FocusLanded says the keyboard reached the control at all, so a false
	// KeyOpen cannot be read as "the rule is missing" when the truth is "the
	// probe never focused anything".
	FocusLanded  bool `json:"focusLanded"`
	FocusVisible bool `json:"focusVisible"`

	// the tab's rendered target and whether the centre of it actually hits the
	// label — a control the day cells above it overlap is not reachable however
	// big its box is.
	TabW      float64 `json:"tabW"`
	TabH      float64 `json:"tabH"`
	TabHit    bool    `json:"tabHit"`
	TabHitWho string  `json:"tabHitWho"`

	// how many entries the opened body carries, so an "open" that disclosed an
	// empty box cannot pass.
	Entries int `json:"entries"`
}

// ldParts is the shared preamble: it finds one host's three elements and
// answers "is the body open" the way a reader would — a rendered box with area,
// not a computed property alone.
const ldParts = `
  var zone = host.querySelector('[data-layer="legend"]');
  if (!zone) { out.why = 'no legend zone rendered'; return out }
  var pin  = zone.querySelector('.legpin');
  var tab  = zone.querySelector('.legtab');
  var body = zone.querySelector('.legbody');
  if (!pin || !tab || !body) { out.why = 'the disclosure is not assembled'; return out }
  function open(){
    var r = body.getBoundingClientRect();
    return getComputedStyle(body).display !== 'none' && r.width > 0 && r.height > 0;
  }
`

// ldGeomScript is PHASE ONE over every host: rest, the keyboard, and the tab's
// own target. It must complete for ALL hosts before ANYTHING is clicked.
//
// CHROMIUM'S :focus-visible IS DOCUMENT-WIDE STATE — the heuristic asks whether
// the most recent interaction was a pointer one — so the first `click()`
// anywhere on the page makes every LATER programmatic `.focus()` stop matching
// it. Run as one pass per host this probe reported a working keyboard for host 0
// and a broken one for host 1, which is indistinguishable from the rule being
// missing. moonReachOpenScript records the identical finding; this is the second
// time the package has paid for it.
const ldGeomScript = `function(host){
  var out = {found:false, why:'', restOpen:false, keyOpen:false,
             focusLanded:false, focusVisible:false,
             tabW:0, tabH:0, tabHit:false, tabHitWho:'', entries:0};
` + ldParts + `
  out.found = true;
  out.entries = body.querySelectorAll('i').length;

  // ── REST. Nothing pointed at, focused or pressed. Headless Chromium parks
  //    no pointer on the page, so :hover matches nothing and this is the true
  //    resting state.
  out.restOpen = open();

  // ── THE KEYBOARD.
  pin.focus();
  out.focusLanded  = (document.activeElement === pin);
  out.focusVisible = pin.matches(':focus-visible');
  out.keyOpen = open();
  pin.blur();

  // ── THE TAB'S TARGET, measured before it is pressed. elementFromPoint at
  //    the centre answers whether anything is sitting on top of it — a box
  //    with a correct size that another element covers is not a control,
  //    which is exactly how the moon cluster's 20x20 dogear swallowed taps
  //    and did nothing with them.
  var tb = tab.getBoundingClientRect();
  out.tabW = Math.round(tb.width * 100) / 100;
  out.tabH = Math.round(tb.height * 100) / 100;
  var hit = document.elementFromPoint(tb.left + tb.width / 2, tb.top + tb.height / 2);
  out.tabHit = !!(hit && (hit === tab || tab.contains(hit)));
  out.tabHitWho = hit ? (hit.tagName.toLowerCase() + '.' + (hit.className || '')) : 'nothing';
  return out;
}`

// ldTapScript is PHASE TWO: the tap, and the tap back. A real click on the
// LABEL, which is what a thumb lands on.
const ldTapScript = `function(host, out){
` + ldParts + `
  tab.click();
  out.tapOpen = open();
  tab.click();
  out.tapTwiceOpen = open();
  if (pin.checked) { tab.click() }   // leave it closed
  return out;
}`

// ldRun lays one host per case on a page and reads them all back in one run.
// extraFlags carries the coarse-pointer switch for the touch arm.
func ldRun(t *testing.T, chrome string, hosts int, width int, extraFlags ...string) []ldReading {
	t.Helper()
	css := blockCSS(t)

	var boxes strings.Builder
	for i := 0; i < hosts; i++ {
		d := fxAlmanac(t, true)
		// One slug per host. Every id in the Block is derived from it and
		// `label[for]` resolves to the FIRST match in the DOCUMENT, so shared
		// slugs would make every tab open host zero's legend — the same defect
		// moon_reach_probe_test.go reported as "the panel never opens".
		d.CalendarSlug = fmt.Sprintf("%s-l%d", d.CalendarSlug, i)
		d.Layers = LayerState{Enabled: []string{"legend"}}
		markup := cpLinkRe.ReplaceAllString(render(t, d), "")
		fmt.Fprintf(&boxes, `<div class="probe-host" style="width:%dpx">%s</div>`, width, markup)
	}

	page := `<!doctype html><html><head><meta charset="utf-8"><style>` +
		`html,body{margin:0;background:#fff}` +
		`.probe-host{display:block;margin:24px}` +
		css + `</style></head><body>` + boxes.String() +
		// TWO PASSES OVER THE WHOLE PAGE, AND THE ORDER IS LOAD-BEARING — see
		// ldGeomScript. Every host's rest, keyboard and geometry first; only
		// then is anything clicked.
		`<script>document.addEventListener('DOMContentLoaded',function(){` +
		`var geom=` + ldGeomScript + `;` +
		`var tap=` + ldTapScript + `;` +
		`var hosts=[].slice.call(document.querySelectorAll('.probe-host'));` +
		`var out=hosts.map(geom);` +
		`hosts.forEach(function(h,i){if(out[i].found){tap(h,out[i])}});` +
		`var c=matchMedia('(pointer: coarse)').matches;` +
		`out.forEach(function(o){o.coarse=c});` +
		`document.body.setAttribute('data-probe', JSON.stringify(out));});</script>` +
		`</body></html>`

	path := filepath.Join(t.TempDir(), "legendprobe.html")
	if err := os.WriteFile(path, []byte(page), 0o644); err != nil { //nolint:gosec // test artefact
		t.Fatalf("write probe page: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	args := []string{
		"--headless", "--no-sandbox", "--disable-gpu", "--hide-scrollbars",
		"--window-size=1400,1400", "--virtual-time-budget=6000",
	}
	args = append(args, extraFlags...)
	out, err := exec.CommandContext(ctx, chrome, append(args, "--dump-dom", "file://"+path)...).Output()
	if err != nil {
		t.Fatalf("chromium: %v", err)
	}
	m := probePayloadRe.FindStringSubmatch(string(out))
	if m == nil {
		t.Fatal("no probe payload in the rendered DOM — the page script did not run")
	}
	var readings []ldReading
	if err := json.Unmarshal([]byte(html.UnescapeString(m[1])), &readings); err != nil {
		t.Fatalf("probe payload: %v", err)
	}
	if len(readings) != hosts {
		t.Fatalf("probe returned %d readings for %d hosts", len(readings), hosts)
	}
	return readings
}

// TestLegendProbe_TheTabOpensByKeyboardAndByTouch.
//
// PROVEN RED, both arms, before it was registered:
//
//   - deleting the `:has(> .legpin:checked)` opener fails TapOpen at both
//     pointer types — the tab becomes a control that visibly does nothing, which
//     is the state the whole disclosure would have shipped in if hover had been
//     the only opener;
//   - swapping `:focus-visible` for `:focus-within` fails TapTwiceOpen, because
//     the second tap un-checks the box while it still holds focus and the zone
//     stays open. That is the reading the CSS-source test cannot take: both
//     spellings are present, plausible and one line apart.
func TestLegendProbe_TheTabOpensByKeyboardAndByTouch(t *testing.T) {
	// THE GATE IS INLINE, NOT IN A HELPER: tools/check-browser-probes.sh takes
	// its census by looking for a Chromium finder INSIDE each top-level Test
	// function's body, so a probe that reached the browser through a private
	// helper would be neither found nor demanded by the guard.
	if testing.Short() {
		t.Skip("browser probe: skipped under -short (CI's mode) — a skipped run is NOT a pass")
	}
	chrome := findProbeChromium()
	if chrome == "" {
		t.Skip("browser probe: no Chromium binary found")
	}

	for _, arm := range []struct {
		name  string
		flags []string
		want  bool // the pointer type the run must actually have got
	}{
		{"fine pointer", nil, false},
		{"coarse pointer", mrCoarsePointerFlags, true},
	} {
		t.Run(arm.name, func(t *testing.T) {
			readings := ldRun(t, chrome, 2, 420, arm.flags...)
			for i, r := range readings {
				if !r.Found {
					t.Fatalf("host %d: %s — every assertion below would be vacuous", i, r.Why)
				}
				// THE ARM MUST REALLY BE THE ARM IT SAYS. A coarse run that
				// silently stayed fine would measure a mouse's geometry and
				// report it as a thumb's.
				if r.Coarse != arm.want {
					t.Fatalf("host %d: matchMedia('(pointer: coarse)') is %v and this arm "+
						"needs %v — the flag did not take effect", i, r.Coarse, arm.want)
				}
				if r.Entries == 0 {
					t.Fatalf("host %d: the disclosed body carries no entries, so 'it opens' "+
						"would be a claim about an empty box", i)
				}

				t.Logf("host %d · coarse=%v · tab %.2f×%.2fpx, centre hits %s · "+
					"rest=%v keyboard=%v (focus landed=%v, :focus-visible=%v) tap=%v "+
					"tap×2=%v · %d entries",
					i, r.Coarse, r.TabW, r.TabH, r.TabHitWho, r.RestOpen, r.KeyOpen,
					r.FocusLanded, r.FocusVisible, r.TapOpen, r.TapTwiceOpen, r.Entries)

				// ── CLOSED AT REST ──────────────────────────────────────
				if r.RestOpen {
					t.Errorf("host %d: the legend is open with nothing pointed at, focused "+
						"or pressed. The whole change is that it OPENS; a body that never "+
						"closes is the surface that shipped before §9.4", i)
				}

				// ── THE KEYBOARD REACHES IT ─────────────────────────────
				if !r.FocusLanded {
					t.Errorf("host %d: focus() did not land on the disclosure control. It is "+
						"CLIPPED rather than display:none precisely so it stays focusable — "+
						"a display-none input is not", i)
				}
				if !r.KeyOpen {
					t.Errorf("host %d: the keyboard reached the control and the zone stayed "+
						"shut (:focus-visible matched = %v). A control the keyboard can land "+
						"on and cannot use is worse than one it cannot reach",
						i, r.FocusVisible)
				}

				// ── THE THUMB REACHES IT, AND CAN PUT IT BACK ───────────
				if !r.TabHit {
					t.Errorf("host %d: the centre of the tab hits %q, not the tab. A target "+
						"another element covers is not a control however big its box is",
						i, r.TabHitWho)
				}
				if !r.TapOpen {
					t.Errorf("host %d: a tap on the tab did not open the legend. Hover alone "+
						"strands a phone, which is why the checked state exists at all", i)
				}
				if r.TapTwiceOpen {
					t.Errorf("host %d: a SECOND tap did not close it. This is the "+
						":focus-within trap: a tap focuses the label's input, so the zone "+
						"stays open after the box un-checks and the tab visibly does nothing "+
						"on every press after the first", i)
				}

				// ── THE TARGET, AND IT IS REPORTED RATHER THAN ASSUMED ──
				//
				// The 44px floor is a REQUIREMENT here and not a report,
				// unlike the moon opener's: that one cannot meet it (an
				// underline day cell is 52px tall and 43.3px wide, so 44px
				// would take 85% of its block axis), and this one sits under
				// the month in a zone with room to spare. There is no excuse
				// available, so none is taken.
				if arm.want {
					const floor = 44.0
					if r.TabH < floor {
						t.Errorf("host %d: the tab is %.2fpx tall under a coarse pointer, "+
							"%.2fpx under the %.0fpx floor", i, r.TabH, floor-r.TabH, floor)
					}
					if r.TabW < floor {
						t.Errorf("host %d: the tab is %.2fpx wide under a coarse pointer, "+
							"%.2fpx under the %.0fpx floor", i, r.TabW, floor-r.TabW, floor)
					}
				}
			}
		})
	}
}
