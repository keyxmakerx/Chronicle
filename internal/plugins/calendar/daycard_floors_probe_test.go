package calendar

// daycard_floors_probe_test.go — C-CALV4-EDITOR-R2b, §10 items 9 and 11.
//
// ── WHY THIS FILE EXISTS: A HANDOFF NOBODY WAS ON THE OTHER END OF ────────
//
// `daycard_test.go`'s TestDayCard_TheEditorReproducesTheStillsMechanicalClaims
// says, in its own header: "Where a claim can only be checked in a browser (the
// 24px floor, the day-of-week wrap, the horizontal fold) it belongs to the
// SCREENSHOT GATE and is NOT faked here."
//
// The screenshot gate measured none of the three. Grepping
// daycard_screenshot_gen_test.go for scrollWidth or clientWidth returned two
// CAPTION STRINGS — "the fold holds", "no horizontal scroll" — prose, not
// measurement, and there was no control-box measurement of any kind. So §10
// item 9 (the 24px control floor at 1440 AND 390 over every new control
// including the tie pill's ✕ and the allow/deny pair) rested on
// `min-block-size: 24px` DECLARATIONS in the sheet, and §10 item 11
// (`scrollWidth === clientWidth`, zero nodes crossing the fold, both themes,
// 390 and 820) was asserted by no test and no image. A declaration is not a
// measurement: `min-block-size` loses to a `block-size`, to a flex `shrink`, to
// a transform, and to any of the four other things that decide a used height.
//
// Worse for 390 specifically: the three files the rejected set named `-390x844`
// were captured at a **500px** window, and only one of the three disclosed the
// substitution — so the fold was never checked at 390 in ANY form, by image or
// by assertion. Those captures are at a real 390 now (stage 11) and the
// measurement below is at a real 390 too.
//
// ── WHAT IT MEASURES, AND HOW ─────────────────────────────────────────────
//
// The same substitute rig every other probe in this package uses: real
// BenchPage output, the real stylesheets, the real module, headless Chromium,
// the page running the measurement itself and writing JSON into a <pre> that
// `--dump-dom` hands back. There is no Playwright in this repo and CDP would
// need a WebSocket client, so the measurement goes where the geometry is.
//
//	1. THE 24px CONTROL FLOOR, two ways, because the stills state it two ways.
//	   (a) every VISIBLE control on the card + editor surface measures at least
//	       24 CSS px in both axes; (b) every ✕ / ✓ / ◈ / ◥ mark has a control
//	       ancestor and THAT ancestor measures at least 24px — which is the
//	       stills index's own wording, "✕/✓ marks whose nearest control parent is
//	       under 24px = 0". (b) is the one that catches a 23.4px close button
//	       inside a flex row, and (a) is the one that catches a control with no
//	       mark in it at all.
//	2. THE HORIZONTAL FOLD. `scrollingElement.scrollWidth === clientWidth` for
//	   the document, AND zero nodes of the card/editor subtree whose rect
//	   crosses the viewport edge in either direction.
//
// Both run in CREATE and in EDIT mode — the tie pill's ✕ and the allow/deny
// pair only EXIST in edit mode, and §10 item 9 names them by name — across
// 390 / 820 / 1440 and both themes.
//
// ENV-GATED like every probe here (DAYCARD_GEOMETRY, DAYCARD_MORPH_TRACE). Set
// DAYCARD_FLOORS=1. The always-on half is
// TestDayCardFloorsProbeMeasuresWhatTheSuiteHandsIt below, which is the check
// that the handoff has a receiver at all.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// daycardFloorPx is the floor itself. It is the number the stills measured
// against and the number the sheet declares; it is written once here so a
// failing row can name it.
const daycardFloorPx = 24

// daycardTouchFloorPx is a SECOND floor beside the first, never a replacement
// for it — C-CALV4-MOBILE [MOB-7] SIGNED, "two floors, two arms, neither
// replacing the other", which is what "strengthened, never weakened" means
// here.
//
// 24 is the DENSE DESKTOP CHROME floor and it stands unchanged at every width,
// over every visible control on the card and the editor. 44 is the PLATFORM
// TOUCH floor and it applies at ≤640 to a NAMED, SHORT list of the controls a
// person hits under time pressure at a table.
//
// MEASURED at 390x664 before it existed: 46 of the editor's 50 visible
// controls were under 44px, the smallest being the head's 24x24 ✕ — adjacent
// on the same sheet to a one-click, hard-DELETE, no-undo Delete. The RSVP
// answer trio was Yes 37x24 / No 33x24 / Maybe 52x24, `Ask →` 53x24, the
// Block's Layers invoker 28x28, the Ledger's Month tab 49x22, and the ribbon
// tile's `→` link 10x19 — the smallest target measured anywhere on the page.
// TestDayCardFloorsProbe was green on every one of them, because it measures
// against 24.
//
// The named list is measured by TestMobileProbe_TheTapFloorAtAPhoneWidth, in
// the same package, over the whole Bench rather than only the card — because
// four of the named controls are not on the card at all.
const daycardTouchFloorPx = 44

type daycardFloorBox struct {
	Sel  string  `json:"sel"`
	Text string  `json:"text"`
	W    float64 `json:"w"`
	H    float64 `json:"h"`
}

type daycardFloorCrosser struct {
	Sel   string  `json:"sel"`
	Left  float64 `json:"left"`
	Right float64 `json:"right"`
}

type daycardFloorResult struct {
	Viewport int    `json:"viewport"`
	Theme    string `json:"theme"`
	Mode     string `json:"mode"`
	// EditorOpen is the SANITY BIT and it is checked before anything else. A
	// probe that measured a closed editor would report a flawless zero on every
	// row, which is the most dangerous shape a green result can have.
	EditorOpen bool `json:"editorOpen"`
	CardOpen   bool `json:"cardOpen"`
	// Checked / MarksChecked / NodesChecked are printed whether or not anything
	// failed, because "we looked at 214 controls and none was short" and "we did
	// not look" are different sentences and only one of them is evidence.
	Checked      int               `json:"checked"`
	MarksChecked int               `json:"marksChecked"`
	NodesChecked int               `json:"nodesChecked"`
	Short        []daycardFloorBox `json:"short"`
	ShortMarks   []daycardFloorBox `json:"shortMarks"`
	// LooseMarks are marks with NO control ancestor at all. They are reported
	// separately from a short one because they are a different defect: a mark
	// that is not inside a control is not a small affordance, it is not an
	// affordance.
	LooseMarks []daycardFloorBox `json:"looseMarks"`
	// Labelled is every checkbox/radio whose measurement was taken on its LABEL
	// rather than on the native box, with the native box's own size. It is
	// reported, always, so the substitution is visible rather than silent.
	Labelled []daycardFloorBox `json:"labelled"`
	// InkMarks counts ◈/◥ — permission INK rather than affordances. Counted so
	// "the marks are present" is a number, gated nowhere.
	InkMarks      int                   `json:"inkMarks"`
	ScrollWidth   float64               `json:"scrollWidth"`
	ClientWidth   float64               `json:"clientWidth"`
	Crossers      []daycardFloorCrosser `json:"crossers"`
	TieRemoveSeen bool `json:"tieRemoveSeen"`
	// InnerWidth is the viewport the measurement ACTUALLY ran in, reported by
	// the page rather than assumed from the flag. It is checked against the
	// requested width, because the whole reason this probe frames itself is
	// that --window-size lies below 500px.
	InnerWidth float64 `json:"innerWidth"`
	AllowDenySeen bool                  `json:"allowDenySeen"`
}

var daycardFloorWidths = []int{390, 820, 1440}

// TestDayCardFloorsProbe performs §10 items 9 and 11 instead of handing them on.
func TestDayCardFloorsProbe(t *testing.T) {
	if os.Getenv("DAYCARD_FLOORS") == "" {
		t.Skip("daycard floors probe: set DAYCARD_FLOORS=1 to run")
	}
	chrome := benchFindChromium()
	if chrome == "" {
		t.Skip("daycard floors probe: no Chromium binary found (set CHROMIUM_BIN)")
	}

	mount := DayCardMount{CanCreate: true, CanAuthorDmOnly: true, CanDelete: true,
		CanRestrict: true, CampaignID: "camp-1"}
	dir := t.TempDir()

	// The edit arm has to see the tie pill's ✕ and the allow/deny pair, and both
	// exist only once a record is loaded — so it runs the SAME stubbed GET the
	// capture rig runs, disclosed there and disclosed here.
	modes := []struct{ name, script string }{
		{"create", daycardOpenEditor},
		{"edit", daycardStubEditRecord + daycardOpenEditRow},
	}

	sawTie, sawAllowDeny := false, false
	for _, mode := range modes {
		for _, dark := range []bool{false, true} {
			theme := "light"
			if dark {
				theme = "dark"
			}
			page := daycardFloorPage(t, mount, dark, mode.script)
			src := filepath.Join(dir, fmt.Sprintf("floors-%s-%s.html", mode.name, theme))
			if err := os.WriteFile(src, []byte(page), 0o644); err != nil {
				t.Fatalf("write probe page: %v", err)
			}
			for _, vw := range daycardFloorWidths {
				r := daycardFloorRun(t, chrome, src, vw)
				r.Mode, r.Theme = mode.name, theme
				if !r.EditorOpen {
					t.Fatalf("%s/%s/%dpx: the editor was NOT open when the probe measured — "+
						"every zero below would be a zero about nothing", mode.name, theme, vw)
				}
				// THE VIEWPORT IS CHECKED, NOT TRUSTED. `--window-size` clamps
				// to 500 and the whole point of the frame is that it does not.
				if int(r.InnerWidth) != vw {
					t.Fatalf("%s/%s: asked for a %dpx viewport and measured in %.0fpx — the "+
						"frame did not take, and every number below is about a viewport "+
						"nobody asked for (this is exactly the substitution the rejected "+
						"evidence made)", mode.name, theme, vw, r.InnerWidth)
				}
				sawTie = sawTie || r.TieRemoveSeen
				sawAllowDeny = sawAllowDeny || r.AllowDenySeen

				t.Logf("── %s · %s · %dpx ──────────────────────────────────", mode.name, theme, vw)
				t.Logf("   controls measured %d · marks measured %d · nodes measured %d",
					r.Checked, r.MarksChecked, r.NodesChecked)
				t.Logf("   document scrollWidth %.0f · clientWidth %.0f · nodes crossing the fold %d",
					r.ScrollWidth, r.ClientWidth, len(r.Crossers))
				t.Logf("   measured IN a %.0fpx viewport (framed; --window-size clamps at %d)",
					r.InnerWidth, daycardFloorMinWindowPx)
				t.Logf("   tie ✕ present %v · allow/deny pair present %v · ◈/◥ ink marks %d",
					r.TieRemoveSeen, r.AllowDenySeen, r.InkMarks)
				for _, b := range r.Labelled {
					t.Logf("   measured on the LABEL: %s — the native box is %.1f×%.1f",
						b.Sel, b.W, b.H)
				}

				// ── §10 item 9 ─────────────────────────────────────────────
				for _, b := range r.Short {
					t.Errorf("%s/%s/%dpx: control %s (%q) measures %.1f×%.1f, under the %dpx "+
						"floor — a remove affordance is a control and the stills assert this "+
						"at zero", mode.name, theme, vw, b.Sel, b.Text, b.W, b.H, daycardFloorPx)
				}
				for _, b := range r.ShortMarks {
					t.Errorf("%s/%s/%dpx: the mark %q sits in a control %s measuring %.1f×%.1f, "+
						"under the %dpx floor", mode.name, theme, vw, b.Text, b.Sel,
						b.W, b.H, daycardFloorPx)
				}
				for _, b := range r.LooseMarks {
					t.Errorf("%s/%s/%dpx: the mark %q has NO control ancestor at all (%s) — "+
						"that is not a small affordance, it is not an affordance",
						mode.name, theme, vw, b.Text, b.Sel)
				}

				// ── §10 item 11 ────────────────────────────────────────────
				if r.ScrollWidth != r.ClientWidth {
					t.Errorf("%s/%s/%dpx: document scrollWidth %.0f != clientWidth %.0f — the "+
						"page scrolls horizontally", mode.name, theme, vw, r.ScrollWidth, r.ClientWidth)
				}
				for _, c := range r.Crossers {
					t.Errorf("%s/%s/%dpx: %s crosses the fold — rect [%.0f, %.0f] against a "+
						"%.0fpx viewport", mode.name, theme, vw, c.Sel, c.Left, c.Right, r.ClientWidth)
				}
			}
		}
	}

	// THE COVERAGE ASSERTION, and it is why the edit arm exists. §10 item 9
	// names the tie pill's ✕ and the allow/deny pair specifically; a sweep that
	// never rendered them would be measuring a floor over the controls that were
	// never in doubt.
	if !sawTie {
		t.Error("no run saw the tie pill's remove ✕; §10 item 9 names it by name and this " +
			"sweep did not measure it")
	}
	if !sawAllowDeny {
		t.Error("no run saw an allow/deny pair; §10 item 9 names it by name and this sweep " +
			"did not measure it")
	}
}

var daycardFloorResultRe = regexp.MustCompile(`(?s)<pre id="floors">(.*?)</pre>`)

// ── THE 390px VIEWPORT IS NOT REACHABLE BY --window-size, AND THAT IS
//    MEASURED RATHER THAN ASSUMED ───────────────────────────────────────────
//
// This Chromium (/opt/pw-browsers/chromium-1194) CLAMPS the headless window to
// a 500px minimum width: `--window-size=390,844` yields innerWidth 500, in old
// headless and in --headless=new alike, with or without a forced device scale
// factor. That is why the rejected set's three `-390x844` files were captured
// at 500 — the substitution was real, it just went undisclosed on two of the
// three, and the fold at 390 was therefore never checked in ANY form.
//
// An IFRAME gives a genuine one. A nested browsing context has its own
// viewport: `innerWidth`, `100vw`, `clientWidth` and MEDIA QUERIES all resolve
// against the frame's box, and a probe run there is a probe at 390 rather than
// a probe at 500 wearing its name. Verified before it was relied on — a 390px
// frame reports innerWidth 390, clientWidth 390, and
// `matchMedia('(max-width:500px)')` true.
//
// The result comes back by postMessage rather than by reaching into the child
// document, because `file://` origins are opaque to each other and the
// alternative is a flag that loosens the sandbox for a measurement.
const daycardFloorMinWindowPx = 500

func daycardFloorRun(t *testing.T, chrome, inner string, viewport int) daycardFloorResult {
	t.Helper()
	outer := filepath.Join(filepath.Dir(inner),
		fmt.Sprintf("outer-%d-%s", viewport, filepath.Base(inner)))
	frame := `<!doctype html><html><head><meta charset="utf-8"><style>` +
		`html,body{margin:0;padding:0;background:#222}` +
		fmt.Sprintf(`iframe{inline-size:%dpx;block-size:900px;border:0;display:block}`, viewport) +
		`</style></head><body><pre id="floors">{}</pre>` +
		`<iframe src="` + filepath.Base(inner) + `"></iframe>` +
		`<script>window.addEventListener('message', function (e) {` +
		`document.getElementById('floors').textContent = e.data; });</script>` +
		`</body></html>`
	if err := os.WriteFile(outer, []byte(frame), 0o644); err != nil {
		t.Fatalf("write frame page: %v", err)
	}
	win := viewport + 60
	if win < daycardFloorMinWindowPx {
		win = daycardFloorMinWindowPx
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, chrome,
		"--headless", "--no-sandbox", "--disable-gpu", "--hide-scrollbars",
		fmt.Sprintf("--window-size=%d,1000", win),
		"--virtual-time-budget=8000",
		"--dump-dom", "file://"+outer,
	)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("chromium dump-dom at %d: %v", viewport, err)
	}
	m := daycardFloorResultRe.FindSubmatch(out)
	if m == nil {
		t.Fatalf("no <pre id=\"floors\"> in the dump at viewport %d", viewport)
	}
	raw := strings.ReplaceAll(string(m[1]), "&quot;", `"`)
	raw = strings.ReplaceAll(raw, "&amp;", "&")
	raw = strings.ReplaceAll(raw, "&lt;", "<")
	raw = strings.ReplaceAll(raw, "&gt;", ">")
	var r daycardFloorResult
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		t.Fatalf("floors payload at %d is not JSON: %v\n%s", viewport, err, raw)
	}
	r.Viewport = viewport
	return r
}

// daycardFloorPage is the plain measurement page: the real Bench, the real
// stylesheets, the real module, no caption chrome. The shot rig's page carries
// an <h1> and a caption block and two fixed diagnostic strips, none of which
// should be inside a measurement of the product's own fold.
func daycardFloorPage(t *testing.T, mount DayCardMount, dark bool, script string) string {
	t.Helper()
	data := benchFxShotData(mount)
	css := benchCSS(t) + benchBlockSheet(t) + dayCardCSS(t)
	mod := readRepoFile(t, "internal/plugins/calendar/static/js/calendar_daycard.js")
	vis := readRepoFile(t, "internal/plugins/calendar/static/js/cal_visibility.js")
	cls := ""
	if dark {
		cls = ` class="dark"`
	}
	return `<!doctype html><html` + cls + `><head><meta charset="utf-8"><style>` +
		`html,body{margin:0;padding:0;background:#f9fafb;` +
		`font-family:ui-sans-serif,system-ui,-apple-system,sans-serif}` +
		`html.dark body{background:oklch(0.165 0.010 265);color:oklch(0.975 0.002 265)}` +
		css +
		`</style></head><body>` +
		benchStripLinks(renderBench(t, data)) +
		`<script>` + vis + `</script><script>` + mod + `</script>` +
		`<script>window.addEventListener('load', function () {` + script +
		// DEFERRED PAST THE DOOR CLICK, for the shot rig's reason: edit mode
		// opens from a promise, so a measurement in the click's own task would
		// measure an editor that does not exist yet — and would report a
		// spotless zero for every control on it.
		// AND THE MORPH IS LET LAND FIRST. The editor is seeded at the CARD's
		// rect for --disc-open, so a measurement taken mid-flight measures every
		// control inside a 340px box — which is exactly what this probe reported
		// the moment stage 18 made the open morph actually run: the date
		// picker's day buttons came back 15px wide, under the 24px floor, in a
		// box that renders at 760. See daycardSettleMorph.
		`setTimeout(function () {` + daycardSettleMorph + daycardFloorScript + `}, 300);` +
		`});</script>` +
		`</body></html>`
}

// daycardFloorScript is the in-page measurement.
//
// The literal 24 below is daycardFloorPx. It is written as a literal because
// this is a Go raw string and an interpolation would make it a fmt.Sprintf of a
// hundred lines of JavaScript; the two are pinned together by
// TestDayCardFloorsProbeMeasuresWhatTheSuiteHandsIt, which fails if the
// constant and the script's number ever disagree.
var daycardFloorScript = `
  function sel(el) {
    if (!el) return '(none)';
    var s = el.tagName.toLowerCase();
    if (el.id) s += '#' + el.id;
    var cn = (el.getAttribute('class') || '').trim();
    if (cn) s += '.' + cn.split(/\s+/).join('.');
    var da = ['data-de-cancel','data-de-save','data-de-delete','data-dc-new','data-dc-edit',
              'data-tie-pick','data-aud-pick','data-vis-pick','data-day-pick'];
    for (var i = 0; i < da.length; i++) {
      if (el.hasAttribute(da[i])) { s += '[' + da[i] + ']'; break; }
    }
    return s;
  }
  function txt(el) { return (el.textContent || '').trim().slice(0, 40); }

  var roots = Array.prototype.slice.call(
    document.querySelectorAll('[data-cal-daycard],[data-cal-dayeditor]'));
  var out = {
    editorOpen: false, cardOpen: false,
    checked: 0, marksChecked: 0, nodesChecked: 0,
    short: [], shortMarks: [], looseMarks: [], labelled: [], inkMarks: 0,
    scrollWidth: 0, clientWidth: 0, crossers: [],
    tieRemoveSeen: false, allowDenySeen: false
  };
  var ed = document.querySelector('[data-cal-dayeditor]');
  var cd = document.querySelector('[data-cal-daycard]');
  out.editorOpen = !!(ed && ed.hasAttribute('data-dc-shown'));
  out.cardOpen = !!(cd && cd.hasAttribute('data-dc-shown'));

  // WHAT COUNTS AS A CONTROL. Anything a pointer or a key can act on. The
  // visually-hidden radio (.vh — position:absolute, 1px, clipped) is EXCLUDED
  // BY NAME rather than by a size threshold, because excluding "anything under
  // 2px" would excuse exactly the defect this measures. Its label (.viscard) is
  // the real control and IS measured.
  var CONTROL = 'button,a[href],input,select,textarea,[role="radio"],[role="button"],label.viscard';
  function isHiddenProxy(el) {
    return el.classList && el.classList.contains('vh');
  }
  // A NATIVE CHECKBOX IS 13x13 AND ITS HIT TARGET IS ITS LABEL. Clicking the
  // label toggles the box, so the label IS the control and the label is what
  // gets measured — the same substitution the visually-hidden radio gets. The
  // raw box is still counted and its size is still reported, so the number is
  // on the record rather than excused into silence.
  function hitTarget(el) {
    var t = (el.getAttribute && el.getAttribute('type')) || '';
    if (el.tagName === 'INPUT' && (t === 'checkbox' || t === 'radio')) {
      var lab = el.closest('label');
      if (lab) return lab;
    }
    return el;
  }
  roots.forEach(function (root) {
    if (!root.hasAttribute('data-dc-shown')) return;
    Array.prototype.slice.call(root.querySelectorAll(CONTROL)).forEach(function (el) {
      if (isHiddenProxy(el)) return;
      var target = hitTarget(el);
      var r = target.getBoundingClientRect();
      // A control with no box at all is not rendered — a [hidden] Delete on a
      // draft, a collapsed time row. Absence is the shipped treatment and is
      // asserted elsewhere; it is not a floor violation.
      if (r.width === 0 && r.height === 0) return;
      out.checked++;
      if (target !== el) {
        var raw = el.getBoundingClientRect();
        out.labelled.push({ sel: sel(el) + ' -> ' + sel(target),
          text: txt(target), w: raw.width, h: raw.height });
      }
      if (el.hasAttribute('data-tie-pick')) out.tieRemoveSeen = true;
      if (el.hasAttribute('data-aud-pick')) {
        out.allowDenySeen = true;
      }
      if (r.width + 0.5 < 24 ||
          r.height + 0.5 < 24) {
        out.short.push({ sel: sel(target), text: txt(target), w: r.width, h: r.height });
      }
    });

    // THE MARKS, the stills index's own way of stating the same rule — and it
    // is stated about TWO marks, not four. "✕/✓ marks whose nearest control
    // parent is under 24px = 0" is about AFFORDANCES. ◈ and ◥ are permission
    // INK: they ride the card's audience chip and the editor's card headings,
    // where they are not meant to be clicked at all, and requiring a control
    // ancestor for them would fail the build for drawing the mark the build law
    // requires. They are counted separately and reported, never gated.
    var MARKS = ['✕', '✓'];
    var INK = ['◈', '◥'];
    var walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, null);
    var n;
    while ((n = walker.nextNode())) {
      var t = (n.nodeValue || '').trim();
      if (!t) continue;
      for (var k = 0; k < INK.length; k++) { if (t.indexOf(INK[k]) >= 0) { out.inkMarks++; break; } }
      var hit = null;
      for (var i = 0; i < MARKS.length; i++) { if (t.indexOf(MARKS[i]) >= 0) { hit = MARKS[i]; break; } }
      if (!hit) continue;
      var host = n.parentElement;
      if (!host) continue;
      var hr = host.getBoundingClientRect();
      if (hr.width === 0 && hr.height === 0) continue;
      out.marksChecked++;
      var ctrl = host.closest(CONTROL);
      if (!ctrl || isHiddenProxy(ctrl)) {
        out.looseMarks.push({ sel: sel(host), text: t.slice(0, 24), w: hr.width, h: hr.height });
        continue;
      }
      var cr = hitTarget(ctrl).getBoundingClientRect();
      if (cr.width + 0.5 < 24 ||
          cr.height + 0.5 < 24) {
        out.shortMarks.push({ sel: sel(ctrl), text: t.slice(0, 24), w: cr.width, h: cr.height });
      }
    }
  });

  // ── §10 item 11: the horizontal fold ──────────────────────────────────
  var se = document.scrollingElement || document.documentElement;
  out.scrollWidth = se.scrollWidth;
  out.clientWidth = se.clientWidth;
  roots.forEach(function (root) {
    if (!root.hasAttribute('data-dc-shown')) return;
    var all = [root].concat(Array.prototype.slice.call(root.querySelectorAll('*')));
    all.forEach(function (el) {
      var r = el.getBoundingClientRect();
      if (r.width === 0 && r.height === 0) return;
      out.nodesChecked++;
      // A HALF-PIXEL OF TOLERANCE, because subpixel layout produces rects like
      // 390.0000001 and a fold report that fires on rounding is a fold report
      // nobody reads.
      if (r.right > out.clientWidth + 0.5 || r.left < -0.5) {
        out.crossers.push({ sel: sel(el), left: r.left, right: r.right });
      }
    });
  });

  out.innerWidth = window.innerWidth;
  // POSTED, NOT WRITTEN. This page is a nested browsing context so that the
  // viewport under test is the real one; the parent owns the <pre> the dump
  // reads. See daycardFloorRun's header for why the frame exists at all.
  if (window.parent && window.parent !== window) {
    window.parent.postMessage(JSON.stringify(out), '*');
  }
`

// TestDayCardFloorsProbeMeasuresWhatTheSuiteHandsIt is ALWAYS ON, and it is the
// cheap check that the fix round's finding 3 cannot recur silently.
//
// FINDING 3, IN ONE SENTENCE: daycard_test.go's editor test hands the 24px
// floor and the horizontal fold to "the screenshot gate", and the screenshot
// gate did not measure either — it carried two CAPTION STRINGS saying they were
// fine. A handoff with no receiver is worse than no handoff, because the
// sentence naming the receiver reads like coverage.
//
// This asserts the receiver EXISTS and does the two things it is handed: it
// reads a real used geometry (getBoundingClientRect), it compares scrollWidth
// against clientWidth, it runs at 390 and 820 and 1440, and it runs in edit
// mode so the two controls §10 item 9 names by name are in the DOM to measure.
func TestDayCardFloorsProbeMeasuresWhatTheSuiteHandsIt(t *testing.T) {
	for _, want := range []string{
		"getBoundingClientRect", // a used geometry, not a declaration
		"scrollWidth",           // §10 item 11…
		"clientWidth",           //   …both halves of it
		"data-tie-pick",        // §10 item 9's tie ✕, by name
		"data-aud-pick",        // §10 item 9's allow/deny pair, by name
		"NodeFilter.SHOW_TEXT",  // the marks arm, the stills' own wording
		"editorOpen",            // the sanity bit
	} {
		if !strings.Contains(daycardFloorScript, want) {
			t.Errorf("the floors probe no longer measures %q — §10 items 9 and 11 would be "+
				"handed to a gate that does not perform them, which is finding 3", want)
		}
	}
	// The floor must be the number the sheet declares, or the probe is measuring
	// a bar nobody set.
	if daycardFloorPx != 24 {
		t.Fatalf("the probe's floor is %dpx; the stills and the sheet both say 24", daycardFloorPx)
	}
	// …and the in-page script must be measuring against that same number. It is
	// a literal there (a Go raw string cannot interpolate), so the two are
	// pinned here rather than left to agree by memory.
	if !strings.Contains(daycardFloorScript, fmt.Sprintf("+ 0.5 < %d", daycardFloorPx)) {
		t.Errorf("the in-page measurement does not compare against %dpx; the constant and "+
			"the script have drifted apart", daycardFloorPx)
	}
	if !strings.Contains(dayCardCSS(t), "min-block-size: 24px") {
		t.Error("calendar-daycard.css no longer declares a 24px control floor; the probe " +
			"would be measuring against a bar the sheet does not set")
	}

	// ── THE SECOND FLOOR'S OWN SELF-PIN, IN THE SAME SHAPE AS THE FIRST'S
	//    (C-CALV4-MOBILE [MOB-7] SIGNED) ────────────────────────────────────
	//
	// The two are pinned SEPARATELY and their ORDER is pinned too, because the
	// failure this guards against is not a typo — it is somebody deciding one
	// day that "there should really only be one floor" and quietly collapsing
	// them. 24 is the dense desktop bar at every width; 44 is the touch bar at
	// ≤640 on a named list; neither replaces the other.
	if daycardTouchFloorPx != 44 {
		t.Fatalf("the touch floor is %dpx; the platform standard and [MOB-7] both say 44",
			daycardTouchFloorPx)
	}
	if daycardTouchFloorPx <= daycardFloorPx {
		t.Fatalf("the touch floor (%d) is not above the dense floor (%d) — two floors that "+
			"have crossed are one floor with a spare name",
			daycardTouchFloorPx, daycardFloorPx)
	}
	if !strings.Contains(dayCardCSS(t), fmt.Sprintf("min-block-size: %dpx", daycardTouchFloorPx)) {
		t.Errorf("calendar-daycard.css declares no %dpx control floor; the ≤640 arm would be "+
			"measuring against a bar the sheet does not set", daycardTouchFloorPx)
	}
	// The sweep must reach BOTH widths §10 item 9 names and BOTH §10 item 11
	// names, and it must run in edit mode — a create-mode-only sweep cannot see
	// the tie ✕ or the allow/deny pair at all.
	for _, w := range []int{390, 820, 1440} {
		found := false
		for _, got := range daycardFloorWidths {
			found = found || got == w
		}
		if !found {
			t.Errorf("the floors sweep does not run at %dpx; §10 items 9 and 11 name "+
				"390, 820 and 1440 between them", w)
		}
	}
	if !strings.Contains(daycardOpenEditRow, "data-dc-edit") {
		t.Error("the probe's edit arm no longer opens a record; the tie ✕ and the " +
			"allow/deny pair would not exist to be measured")
	}
}
