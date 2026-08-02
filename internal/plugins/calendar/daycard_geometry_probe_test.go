package calendar

// daycard_geometry_probe_test.go — C-CALV4-EDITOR-R2b, the [ER-5] MEASUREMENT.
//
// WHY THIS FILE EXISTS AT ALL. The 22 sealed stills draw a two-column editor
// ~1008px wide. DAYCARD's placement law treats the docked Ledger's rect as a
// HARD EXCLUSION ZONE and NEVER RESIZES THE BOX TO FIT, so a wider box has
// FEWER CLEAR POSITIONS — and both of that slice's verify-round blockers were
// exactly this: a popover over a stacked full-width Ledger at ~884-944px, then
// a desktop sheet covering 100.0% of it at 625-884px.
//
// [ER-5] SIGNED therefore rules: "the editor's desktop width is bounded by the
// clear band placeCard can actually find, and it is MEASURED before it is
// chosen… if that width is below the drawing's, the build takes the measured
// width and the report names the divergence." This is where it is measured.
//
// IT DRIVES THE REAL THING. Real BenchPage output, the real stylesheets, the
// real module, headless Chromium — the same substitute rig that caught both of
// DAYCARD's blockers. The page runs the sweep itself and writes the result into
// a <pre>, which `--dump-dom` hands back: there is no Playwright in this repo
// and CDP would need a WebSocket client, so the measurement goes where the
// geometry already is rather than reaching in from outside.
//
// ENV-GATED, LIKE EVERY OTHER PROBE HERE (calendar_v2_mobile_breakpoint_probe,
// container_query_probe, shelf_probe). It costs CI nothing and it is
// reproducible by anyone who sets DAYCARD_GEOMETRY=1.
//
// `placeCard` IS NOT RE-OPENED BY ANY OF THIS. The probe reads what the shipped
// law does; it does not propose a fourth geometry. Round 4's lesson was that
// the third was already one too many, and a probe that ended in a placement
// change would be that lesson unlearned.

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

// daycardProbeWidths is the viewport sweep. It spans the two bands DAYCARD's
// blockers lived in (625-884 and 884-944) and reaches past the 1232px the
// screenshot gate shoots at, because the editor got WIDER and that is exactly
// the input those blockers were sensitive to.
var daycardProbeWidths = []int{640, 720, 820, 900, 1000, 1080, 1100, 1232, 1280, 1440, 1600}

// daycardProbeCandidates are the editor widths under test, from the shipped
// stage-2 box up past the drawing's 1008px.
var daycardProbeCandidates = []int{420, 640, 760, 860, 940, 1008}

// daycardProbeRow is one placement. It carries MORE than the gate needs on
// purpose: the first cut reported only {clear, sheet, overlap}, and when the
// sweep went red the row said "clear=true, sheet=false, 70906 px² of overlap",
// which is a contradiction with no next question in it. MeasuredW / Rect are
// what turn that into a diagnosis — a box reporting 420px against a 760px
// --de-w is a stale inline geometry, and a box whose rect starts left of the
// Ledger and ends right of it is a placement that had nowhere to go.
type daycardProbeRect struct {
	L int `json:"l"`
	T int `json:"t"`
	R int `json:"r"`
	B int `json:"b"`
}

type daycardProbeRow struct {
	Viewport int    `json:"viewport"`
	EditorW  int    `json:"editorW"`
	Day      string `json:"day"`
	Clear    bool   `json:"clear"`
	Sheet    bool   `json:"sheet"`
	// Surface is "card" or "editor". The CARD arm is the CONTROL: it is
	// stage-2's box, byte-unchanged by this slice, driven through the same
	// listeners at the same viewports — so a finding that shows up on both is a
	// property of the shipped law and a finding that shows up only on the editor
	// is this slice's to own.
	Surface string `json:"surface"`
	// Host is the Block the clicked cell belongs to, because the Bench renders
	// one Block per calendar and each carries its OWN Ledger.
	Host string `json:"host"`
	// Own is the intersection with the Ledger of the box's OWN host — the rect
	// placeCard was actually handed (`ledgerRect(state.host)`). THIS is the gate.
	Own int `json:"own"`
	// Cross is the largest intersection with any OTHER Block's Ledger. The
	// shipped law excludes ONE host's rect by construction, so this number is a
	// property of that law, not of a width; it is REPORTED with the card's own
	// control figure beside it and never silently folded into the gate.
	Cross int `json:"cross"`
	// CrossHost names the Block whose Ledger the box landed on, when Cross > 0.
	CrossHost string `json:"crossHost"`
	// MeasuredW is the box's own offsetWidth at the moment of the shot — the
	// number edPosition fed to placeCard. For the editor it must equal EditorW,
	// and when it does not, the box was measured through a stale style attribute
	// (the stage-6 fix-forward: edClose's reverse geometry outliving its timer).
	MeasuredW int `json:"measuredW"`
	// Rect is the placed box, so a failing row names its own geometry.
	Rect daycardProbeRect `json:"rect"`
}

// TestDayCardGeometryProbe sweeps the editor's width against the clear band the
// shipped placement law can actually find, with a real docked Ledger, and
// reports the largest width that holds 0 px² of overlap everywhere.
func TestDayCardGeometryProbe(t *testing.T) {
	if os.Getenv("DAYCARD_GEOMETRY") == "" {
		t.Skip("daycard geometry probe: set DAYCARD_GEOMETRY=1 to run")
	}
	chrome := benchFindChromium()
	if chrome == "" {
		t.Skip("daycard geometry probe: no Chromium binary found (set CHROMIUM_BIN)")
	}

	data := benchFxData(true, true)
	data.DayCard = DayCardMount{
		CanCreate: true, CanAuthorDmOnly: true, CanDelete: true, CanRestrict: true,
		CampaignID: "camp-1",
	}
	page := daycardProbePage(t, data)
	dir := t.TempDir()
	src := filepath.Join(dir, "probe.html")
	if err := os.WriteFile(src, []byte(page), 0o644); err != nil {
		t.Fatalf("write probe page: %v", err)
	}

	// ── THREE MEASURES, AND THEY ARE THREE DIFFERENT QUESTIONS ─────────────
	//
	// The first cut of this file collapsed them into one number and the number
	// was uninterpretable: it reported "clear=true, sheet=false, 70,906 px² of
	// overlap" at EVERY candidate width including the shipped one, which is a
	// contradiction rather than a finding. Separated, the same measurements say
	// three plain things. The separation is the shipped law's OWN semantics,
	// written down in placeCard's header in these words: "A POPOVER over the
	// Ledger is [DC-3]'s STOP-AND-FLAG and it speaks. A SHEET over the Ledger is
	// [DC-3] bullet 4's own signed treatment… it is recorded and stays quiet."
	//
	//  1. OWN-HOST POPOVER OVERLAP — THE GATE. §10 item 7: "0 px² overlap with
	//     the docked Ledger across the width sweep". `placeCard` is handed
	//     `ledgerRect(state.host)` — the Ledger of the Block the day belongs to —
	//     and treats exactly that rect as a hard exclusion zone. A popover that
	//     lands on it is the law failing at the thing it promises, which is the
	//     round-3/round-4 regression check. **Any px² here is a STOP-AND-FLAG.**
	//
	//  2. THE DESKTOP SHEET FALLBACK — RECORDED, NOT GATED. It is [DC-3] bullet
	//     4's signed geometry-ran-out treatment and it covers the viewport BY
	//     CONSTRUCTION, so scoring its overlap as a failure would make the gate
	//     fire on the very answer the ruling signed for the case where no clear
	//     position exists. It is counted, its viewports are named, and the
	//     secondary criterion below is what actually separates the candidates.
	//
	//  3. CROSS-BLOCK OVERLAP — REPORTED WITH ITS CONTROL, NOT GATED, AND
	//     BOOKED. The Bench renders one Block per calendar and each carries its
	//     own Ledger; `ledgerRect(state.host)` excludes ONE of them, so a box
	//     opened from the real-world Block's grid can land on the PRIMARY
	//     Block's Ledger. That is a property of the shipped single-host
	//     exclusion, not of a width — and the CARD arm proves it: the same
	//     measurement runs on stage-2's card, which this slice does not touch.
	//     Widening the exclusion means re-opening `placeCard`, which [ER-5]
	//     SIGNED makes a STOP-AND-FLAG rather than an edit. So it is measured,
	//     named in the report, and booked. It is NOT quietly folded into the
	//     gate, and it is NOT quietly dropped.
	own := map[int]daycardProbeRow{}
	cross := map[string]daycardProbeRow{}
	sheets := map[int]int{}
	byVP := map[int][]int{}
	for _, vw := range daycardProbeWidths {
		rows := daycardProbeRun(t, chrome, src, vw)
		if len(rows) == 0 {
			t.Fatalf("viewport %d produced no measurements; the sweep did not run", vw)
		}
		for _, r := range rows {
			// THE MOBILE BRANCH IS NOT UNDER TEST AND SAYING SO IS THE POINT.
			// At or below placeCard's 640px breakpoint the FULL-WIDTH SHEET IS
			// THE LAYOUT — [DC-3] bullet 4 signed it, DC2-MOBILE-6 accepted the
			// overlap there in terms, and §12 scopes the STOP-AND-FLAG row to
			// 1232px.
			if r.Viewport <= 640 {
				continue
			}
			// THE STALE-GEOMETRY ASSERTION, AND IT IS THE ONE THIS ROUND ADDED.
			// The editor's box must measure the width the SHEET gives it, which
			// is `min(var(--de-w), calc(100vw - 16px))` — the viewport clamp is
			// part of the declared width, not a discrepancy, so it is computed
			// here rather than excused case by case. What this catches is the
			// other thing: a box carrying a width from somewhere else. That was
			// real — edClose wrote the reverse morph geometry as an INLINE
			// `inline-size` and edHide, the only thing that clears it, runs on a
			// --disc-close timer that edShow cancels. Reopening inside that
			// window handed edPosition the CARD's 420px for a box the sheet
			// sizes at 760px, so placeCard placed a rectangle that does not
			// exist and reported clear=true about it. Fixed in the module at
			// stage 6; pinned here so it cannot come back quietly.
			if want := daycardProbeExpectedW(r.EditorW, r.Viewport); r.Surface == "editor" &&
				!r.Sheet && r.MeasuredW != want {
				t.Errorf("viewport %d, day %s: the editor measured %dpx where the sheet declares "+
					"%dpx (--de-w %d clamped to the viewport) — placeCard was handed a box that "+
					"is not the one that renders, which is how an occlusion hides behind a "+
					"clear=true report", r.Viewport, r.Day, r.MeasuredW, want, r.EditorW)
			}
			if r.Surface == "editor" && !r.Sheet && r.Own > 0 {
				if cur, seen := own[r.EditorW]; !seen || r.Own > cur.Own {
					own[r.EditorW] = r
				}
			}
			if r.Cross > 0 {
				if cur, seen := cross[r.Surface]; !seen || r.Cross > cur.Cross {
					cross[r.Surface] = r
				}
			}
			if r.Surface == "editor" && r.Sheet {
				sheets[r.EditorW]++
				byVP[r.EditorW] = append(byVP[r.EditorW], r.Viewport)
			}
		}
	}

	baseline := sheets[daycardProbeCandidates[0]]
	best := 0
	t.Logf("── [ER-5] measured clear band ─────────────────────────────────")
	t.Logf("  baseline: the shipped stage-2 %dpx box takes the desktop sheet on "+
		"%d placements of the sweep", daycardProbeCandidates[0], baseline)
	for _, w := range daycardProbeCandidates {
		r, bad := own[w]
		switch {
		case bad:
			t.Logf("  %4dpx  OVERLAPS its own host's Ledger — viewport %d, day %s, %d px² · "+
				"box measured %dpx at [%d,%d %d,%d] · clear=%v sheet=%v",
				w, r.Viewport, r.Day, r.Own, r.MeasuredW,
				r.Rect.L, r.Rect.T, r.Rect.R, r.Rect.B, r.Clear, r.Sheet)
		default:
			t.Logf("  %4dpx  0 px² overlap · %d sheet placements (baseline %d) · sheeting "+
				"viewports %v", w, sheets[w], baseline, daycardProbeUniq(byVP[w]))
			// THE SECONDARY CRITERION, AND IT IS THIS SLICE'S OWN. Every width
			// holds the ruling's stated gate (0 px² overlap), so the gate alone
			// would admit the drawing's 1008px — the placement law is doing its
			// job and the round-3 harm does not recur at ANY width. What
			// separates the candidates is where the POPOVER survives, and the
			// honest line is the editor's OWN two-column breakpoint: below
			// 1080px `.ed-body` is one column anyway and the narrow sheet is a
			// legitimate treatment the shipped 420px box already takes; at and
			// above it the two-column chrome is supposed to live BESIDE the
			// docked Ledger, and a width that turns those viewports into sheets
			// has bought its width out of the layout it was drawn for.
			//
			// The desktop sheet fallback also WARNS once per session, and a
			// warning that fires on most opens trains the next hand to ignore
			// the one that matters — the module's own comment says exactly that
			// about the mobile case.
			//
			// REQUIRING "NEVER SHEETS AT ANY DOCKED VIEWPORT" WAS TRIED AND IS
			// REFUSED AS ARITHMETIC, NOT TASTE: at a 900px viewport a 300px
			// docked Ledger leaves 583px of clear band, so that rule admits only
			// the stage-2 420px box and the chrome would never get a second
			// column at all.
			if !daycardProbeSheetsAtOrAbove(byVP[w], daycardEditorTwoColumnPx) && w > best {
				best = w
			}
		}
	}
	t.Logf("  → the largest width holding 0 px² AND costing no clear position is %dpx", best)

	// ── THE CROSS-BLOCK FINDING, WITH ITS CONTROL ──────────────────────────
	//
	// Printed whether or not it fired, because "we looked and it was zero" and
	// "we did not look" are different sentences and only one of them is
	// evidence.
	t.Logf("── cross-Block Ledger occlusion (REPORTED, not gated — see the header) ──")
	for _, surface := range []string{"card", "editor"} {
		if r, hit := cross[surface]; hit {
			t.Logf("  %-6s worst %d px² over the %q Block's Ledger — viewport %d, day %s "+
				"(host %q), box at [%d,%d %d,%d]", surface, r.Cross, r.CrossHost,
				r.Viewport, r.Day, r.Host, r.Rect.L, r.Rect.T, r.Rect.R, r.Rect.B)
			continue
		}
		t.Logf("  %-6s 0 px² over any other Block's Ledger across the whole sweep", surface)
	}
	if _, cardHit := cross["card"]; cardHit {
		t.Logf("  → the CARD hits it too, and this slice does not touch the card's box or " +
			"placeCard: the single-host exclusion is the shipped law's, not the chrome's. " +
			"Widening it means re-opening placeCard, which [ER-5] SIGNED makes a " +
			"STOP-AND-FLAG rather than an edit. Booked.")
	}

	for w, r := range own {
		t.Errorf("editor width %dpx OVERLAPS ITS OWN HOST'S docked Ledger by %d px² at "+
			"viewport %d on day %s — an occluded Ledger is a STOP-AND-FLAG, and this is the "+
			"round-3/round-4 regression check", w, r.Own, r.Viewport, r.Day)
	}
	if best == 0 {
		t.Fatal("no candidate width cleared the sweep; that is a STOP-AND-FLAG, not a tuning")
	}

	// THE SHIPPED VALUE MUST BE THE MEASURED ONE. A sheet whose --de-w drifted
	// past the measured band is the round-3 blocker returning with a nicer font.
	shipped := daycardProbeShippedWidth(t)
	if shipped > best {
		t.Errorf("calendar-daycard.css ships --de-w: %dpx but the measured band tops out "+
			"at %dpx — a wider editor buys its width out of the clear positions the "+
			"placement law can find, which is the exact condition [DC-3] signed as a "+
			"STOP-AND-FLAG", shipped, best)
	}
}

// daycardProbeViewportPad mirrors the sheet's own clamp — `.cal-dayeditor`
// declares `inline-size: min(var(--de-w), calc(100vw - 16px))`, so 8px of pad on
// each side is part of the declared width and not a drift from it.
const daycardProbeViewportPad = 16

// daycardProbeExpectedW is what the box MUST measure at a given viewport. It is
// derived from the sheet's rule rather than restated as a number, so a change to
// the clamp cannot leave this assertion quietly asserting the old one.
func daycardProbeExpectedW(deW, viewport int) int {
	if clamped := viewport - daycardProbeViewportPad; clamped < deW {
		return clamped
	}
	return deW
}

// daycardProbeUniq collapses a placement list to the viewports it names.
func daycardProbeUniq(vps []int) []int {
	seen := map[int]bool{}
	out := []int{}
	for _, v := range vps {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// daycardEditorTwoColumnPx is the editor's own two-column breakpoint, and it is
// read from the sheet rather than restated here so the criterion cannot drift
// away from the layout it is about.
const daycardEditorTwoColumnPx = 1080

func daycardProbeSheetsAtOrAbove(vps []int, floor int) bool {
	for _, v := range vps {
		if v >= floor {
			return true
		}
	}
	return false
}

var daycardProbeWidthRe = regexp.MustCompile(`--de-w:\s*(\d+)px`)

func daycardProbeShippedWidth(t *testing.T) int {
	t.Helper()
	m := daycardProbeWidthRe.FindStringSubmatch(dayCardCSS(t))
	if len(m) != 2 {
		t.Fatal("calendar-daycard.css does not declare --de-w; [ER-5]'s measurement has no subject")
	}
	var n int
	if _, err := fmt.Sscanf(m[1], "%d", &n); err != nil {
		t.Fatalf("parse --de-w: %v", err)
	}
	return n
}

var daycardProbeResultRe = regexp.MustCompile(`(?s)<pre id="probe">(.*?)</pre>`)

func daycardProbeRun(t *testing.T, chrome, src string, viewport int) []daycardProbeRow {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, chrome,
		"--headless", "--no-sandbox", "--disable-gpu", "--hide-scrollbars",
		fmt.Sprintf("--window-size=%d,900", viewport),
		"--virtual-time-budget=8000",
		"--dump-dom", "file://"+src,
	)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("chromium dump-dom at %d: %v", viewport, err)
	}
	if d := regexp.MustCompile(`(?s)<pre id="diag">(.*?)</pre>`).FindSubmatch(out); d != nil {
		t.Logf("viewport %d diag: %s", viewport, strings.ReplaceAll(string(d[1]), "&quot;", `"`))
	}
	m := daycardProbeResultRe.FindSubmatch(out)
	if m == nil {
		t.Fatalf("no <pre id=\"probe\"> in the dump at viewport %d", viewport)
	}
	raw := strings.ReplaceAll(string(m[1]), "&quot;", `"`)
	raw = strings.ReplaceAll(raw, "&amp;", "&")
	var rows []daycardProbeRow
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		t.Fatalf("probe payload at %d is not JSON: %v\n%s", viewport, err, raw)
	}
	return rows
}

// daycardProbePage builds the page the sweep runs in: the REAL Bench surface,
// the REAL card and editor scaffolds, the REAL payload attribute, both
// stylesheets inlined and the shipped module running over all of it.
func daycardProbePage(t *testing.T, data BenchData) string {
	t.Helper()
	surface := renderBench(t, data)
	css := benchCSS(t) + benchBlockSheet(t) + dayCardCSS(t)
	mod := readRepoFile(t, "internal/plugins/calendar/static/js/calendar_daycard.js")
	vis := readRepoFile(t, "internal/plugins/calendar/static/js/cal_visibility.js")

	return `<!doctype html><html><head><meta charset="utf-8"><style>` +
		`html,body{margin:0;padding:0;background:#f9fafb;` +
		`font-family:ui-sans-serif,system-ui,-apple-system,sans-serif}` +
		css +
		`</style></head><body>` +
		benchStripLinks(surface) +
		`<pre id="probe">[]</pre><pre id="diag">{}</pre>` +
		`<script>` + vis + `</script><script>` + mod + `</script>` +
		`<script>` + daycardProbeScript(daycardProbeCandidates) + `</script>` +
		`</body></html>`
}

// daycardProbeScript is the in-page sweep. It opens the editor on EVERY day
// cell at EVERY candidate width and records what the shipped placement law
// actually produced — `data-dc-clear` (the module's own occlusion report), the
// sheet flag, and the measured intersection with the Ledger's real rect.
func daycardProbeScript(candidates []int) string {
	list, _ := json.Marshal(candidates)
	return `
// THE SWEEP RUNS AFTER THE LOAD EVENT, NOT AT PARSE TIME. The module wires itself on
// DOMContentLoaded when the parser is still running, so a probe that ran inline
// would click into a page with no listeners on it and measure a box that never
// opened — which is exactly what the first cut of this file did, and the diag
// line below is why it was caught rather than believed.
window.addEventListener('load', function () {
  function rect(el) { var r = el.getBoundingClientRect();
    return { l: r.left, t: r.top, r: r.right, b: r.bottom }; }
  function overlap(a, b) {
    if (!a || !b) return 0;
    var w = Math.min(a.r, b.r) - Math.max(a.l, b.l);
    var h = Math.min(a.b, b.b) - Math.max(a.t, b.t);
    return (w > 0 && h > 0) ? Math.round(w * h) : 0;
  }
  var out = [];
  var editor = document.querySelector('[data-cal-dayeditor]');
  // EVERY DOCKED LEDGER ON THE PAGE, NOT THE FIRST ONE. The Bench renders a
  // Block per calendar and each carries its own Ledger zone; the placement law
  // excludes the rect of the host the card opened FROM (ledgerRect(state.host)),
  // so a probe that measured against document.querySelector's first match would
  // be scoring the editor against a Ledger it was never asked to avoid — and
  // scoring the RIGHT one not at all. Both are collected, and every row is
  // measured against the union of them.
  var ledgerZones = Array.prototype.slice.call(
    document.querySelectorAll('[data-zone="ledger"]'));
  var ledgerZone = ledgerZones[0] || null;
  var diag = {
    cells: 0, opened: 0,
    ledger: ledgerZone ? rect(ledgerZone) : null,
    ledgers: ledgerZones.map(function (z) {
      var host = z.closest('[data-bench-block]');
      return { host: host ? (host.getAttribute('data-bench-block') || '?') : '?',
               rect: rect(z), w: Math.round(z.getBoundingClientRect().width),
               h: Math.round(z.getBoundingClientRect().height) };
    })
  };
  var card = document.querySelector('[data-cal-daycard]');
  var cells = Array.prototype.slice.call(
    document.querySelectorAll('[data-bench-block] [data-day][data-day-ord]'));
  // hostOf names the Block a cell belongs to, and ledgersFor splits the page's
  // Ledgers into the one placeCard was handed and the ones it was not.
  function hostOf(el) {
    var h = el.closest('[data-bench-block]');
    return h ? (h.getAttribute('data-bench-block') || '?') : '?';
  }
  function measure(box, host) {
    var ownPx = 0, crossPx = 0, crossHost = '';
    var r = rect(box);
    ledgerZones.forEach(function (z) {
      var zh = hostOf(z);
      var px = overlap(r, rect(z));
      if (zh === host) { ownPx = Math.max(ownPx, px); return; }
      if (px > crossPx) { crossPx = px; crossHost = zh; }
    });
    return { own: ownPx, cross: crossPx, crossHost: crossHost, r: r };
  }
  var widths = ` + string(list) + `;
  widths.forEach(function (w) {
    editor.style.setProperty('--de-w', w + 'px');
    cells.forEach(function (cell) {
      var host = hostOf(cell);
      // The card first, then its ` + "`+ New event`" + ` door — the same two
      // clicks a user makes, through the same listeners.
      cell.click();
      // THE CARD ARM IS THE CONTROL AND IT IS MEASURED BEFORE THE DOOR IS
      // CLICKED. It is stage-2's box, which this slice does not touch; a
      // finding that appears on both arms belongs to the shipped law.
      if (card && card.hasAttribute('data-dc-shown')) {
        var cm = measure(card, host);
        out.push({
          viewport: window.innerWidth, editorW: w, surface: 'card',
          day: cell.getAttribute('data-day'), host: host,
          clear: card.getAttribute('data-dc-clear') !== '0',
          sheet: card.classList.contains('dcsheet'),
          own: cm.own, cross: cm.cross, crossHost: cm.crossHost,
          measuredW: Math.round(card.offsetWidth),
          rect: { l: Math.round(cm.r.l), t: Math.round(cm.r.t),
                  r: Math.round(cm.r.r), b: Math.round(cm.r.b) }
        });
      }
      var doorNew = document.querySelector('[data-dc-new]');
      if (doorNew) doorNew.click();
      if (editor.hasAttribute('data-dc-shown')) diag.opened++;
      var em = measure(editor, host);
      out.push({
        viewport: window.innerWidth, editorW: w, surface: 'editor',
        day: cell.getAttribute('data-day'), host: host,
        clear: editor.getAttribute('data-dc-clear') !== '0',
        sheet: editor.classList.contains('dcsheet'),
        own: em.own, cross: em.cross, crossHost: em.crossHost,
        measuredW: Math.round(editor.offsetWidth),
        rect: { l: Math.round(em.r.l), t: Math.round(em.r.t),
                r: Math.round(em.r.r), b: Math.round(em.r.b) }
      });
      var cancel = editor.querySelector('[data-de-cancel]');
      if (cancel) cancel.click();
    });
  });
  diag.cells = cells.length;
  document.getElementById('probe').textContent = JSON.stringify(out);
  document.getElementById('diag').textContent = JSON.stringify(diag);
});
`
}
