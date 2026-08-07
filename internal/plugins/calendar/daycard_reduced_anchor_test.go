package calendar

// daycard_reduced_anchor_test.go — C-CALV4-CARD-REDUCED-ANCHOR, the pin.
//
// ── THE DEFECT THIS FILE EXISTS TO HOLD DOWN ──────────────────────────────
//
// Under `prefers-reduced-motion: reduce` the event editor opened at the
// VIEWPORT'S TOP-LEFT instead of beside the day it belongs to — half a screen
// from the cell the reader clicked. daycard_morph_shots_test.go's shot 13
// caption has said so in terms since R2b, where it was booked rather than fixed
// because [ER-5]/[ER-6] bound that slice to the morph's ordering.
//
// THE MECHANISM IS A SINGLE ORDERING, AND IT IS ALL IN edOpen. The opener
// measured the card, called `closeCard()`, and only THEN handed `edPosition`
// the LIVE card element to measure again. With motion allowed that second read
// is harmless: `closeDelayMS` returns --disc-close and the card is still on
// screen at its own rect when the placement law reads it. Under reduced motion
// `closeDelayMS` returns 0 BY DESIGN — the sheet declares no transition, so
// waiting would leave a fully-styled card sitting there after it was logically
// closed — and `hide()` runs SYNCHRONOUSLY inside `closeCard()`. It clears the
// inline geometry and calls `hidePopover()`, so the very next
// `getBoundingClientRect()` on that element answers 0×0 at 0,0. `placeCard` is
// then doing its job perfectly over an anchor that no longer exists: `below =
// 0 + 8`, `left = max(0, pad) = 8`, and the box lands at (8,8).
//
// ── WHY THE PIN HAS TO BE A BROWSER ───────────────────────────────────────
//
// test/js/daycard_dom.mjs's `El` carries a STATIC `rect`, and neither
// `removeAttribute('style')` nor `hidePopover()` changes it. The Node harness
// cannot express a hidden node's collapsed rect at all, which is precisely why
// the whole suite stayed green over this for three rounds. The measurement has
// to happen where the layout does.
//
// ── WHAT IT MEASURES ──────────────────────────────────────────────────────
//
// The same page, the same clicks, the same mid-grid day cell, driven twice
// through real Chromium: once bare and once with
// `--force-prefers-reduced-motion`. Both arms report where the editor came to
// rest, and the assertion is that they agree — the reduced-motion reader gets
// the box in the same place, just without the flight. The control's own
// placement is checked for being off the viewport edge FIRST, because "both
// arms landed at 8,8" would satisfy an equality test while being the exact
// defect.
//
// IT IS NOT ENV-GATED, for TestDayCardMorphInterpolates' reason: it opens the
// editor twice per arm and finishes in a couple of seconds, and a guard that
// runs only when someone remembers a variable is the guard that was missing.
// It skips — loudly — when there is no Chromium, or when the binary ignores
// the flag and the branch under test therefore cannot be reached.

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// daycardAnchorRect is a placed box, rounded to whole pixels the way every
// other rig in this family reports geometry.
type daycardAnchorRect struct {
	L float64 `json:"l"`
	T float64 `json:"t"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

func (r daycardAnchorRect) String() string {
	return fmt.Sprintf("{l:%g t:%g w:%g h:%g}", r.L, r.T, r.W, r.H)
}

// daycardAnchorShot is one arm of the counterfactual.
type daycardAnchorShot struct {
	// Reduced is what the PAGE saw, not what the flag asked for — a binary that
	// ignores --force-prefers-reduced-motion must not be read as a passing run.
	Reduced bool              `json:"reduced"`
	Day     string            `json:"day"`
	Cell    daycardAnchorRect `json:"cell"`
	Card    daycardAnchorRect `json:"card"`
	Editor  daycardAnchorRect `json:"editor"`
	Sheet   bool              `json:"sheet"`
	Err     string            `json:"err"`
}

// daycardAnchorTolerance is the slack on the comparison. The two arms are
// separate browser processes over the same page, so sub-pixel layout noise is
// possible and 512px of drift — the defect's own figure — is not within 1px of
// anything.
const daycardAnchorTolerance = 1.0

// TestDayCardReducedMotionAnchorsToItsDay drives the editor open in both motion
// modes and asserts the box rests in the same place.
func TestDayCardReducedMotionAnchorsToItsDay(t *testing.T) {
	chrome := benchFindChromium()
	if chrome == "" {
		t.Skip("reduced-motion anchor probe: no Chromium binary found (set CHROMIUM_BIN)")
	}

	data := benchFxData(true, true)
	data.DayCard = DayCardMount{CanCreate: true, CanAuthorDmOnly: true, CanDelete: true,
		CanRestrict: true, CampaignID: "camp-1"}
	cal := benchFxTypedCalendar()
	data.DayCardJSON = dayCardPayloadJSON(
		dayCardSeed{CanAuthor: true, CanRestrict: true, Roster: benchFxRoster()},
		dayCardSource{Block: data.Primary, Calendar: &cal})

	dir := t.TempDir()
	src := filepath.Join(dir, "reduced-anchor.html")
	if err := os.WriteFile(src, []byte(daycardAnchorPage(t, data)), 0o644); err != nil {
		t.Fatalf("write probe page: %v", err)
	}

	ctrl := daycardAnchorRun(t, chrome, src, false)
	red := daycardAnchorRun(t, chrome, src, true)

	for name, shot := range map[string]daycardAnchorShot{"no-preference": ctrl, "reduced": red} {
		if shot.Err != "" {
			t.Fatalf("the %s arm could not drive the editor: %s", name, shot.Err)
		}
	}
	if ctrl.Reduced {
		t.Skip("reduced-motion anchor probe: the bare browser already reports " +
			"prefers-reduced-motion, so there is no control arm to compare against")
	}
	if !red.Reduced {
		t.Skip("reduced-motion anchor probe: --force-prefers-reduced-motion did not flip " +
			"matchMedia on this binary, so closeDelayMS's reduced branch — the whole " +
			"subject of this file — is never reached")
	}
	// A SHEET COVERS THE VIEWPORT BY CONSTRUCTION, so if the fixture ever drifts
	// into [DC-3] bullet 4's fallback this file is measuring nothing at all.
	// That is a loud failure rather than a quiet pass: the guard would still be
	// green while guarding nothing.
	if ctrl.Sheet || red.Sheet {
		t.Fatalf("the editor took the desktop sheet fallback (no-preference sheet=%v, "+
			"reduced sheet=%v) — the sheet covers the viewport by construction, so this "+
			"probe can no longer see where a POPOVER anchors; fix the fixture, do not "+
			"relax the assertion", ctrl.Sheet, red.Sheet)
	}
	// THE CONTROL IS CHECKED FIRST, AND THIS IS THE LINE THAT KEEPS THE
	// ASSERTION FROM BEING VACUOUS. `placeCard` pads the viewport edge by 8px,
	// so a box at left 8 is a box that was handed a collapsed anchor. If BOTH
	// arms landed there the equality below would pass while reporting the exact
	// defect.
	if ctrl.Editor.L <= daycardAnchorEdgePad+daycardAnchorTolerance {
		t.Fatalf("the no-preference control landed at the viewport's left edge (%s) for "+
			"day %q at %s — the control is supposed to be the CORRECT placement, so this "+
			"probe has nothing to compare the reduced arm against",
			ctrl.Editor, ctrl.Day, ctrl.Cell)
	}

	if math.Abs(red.Editor.L-ctrl.Editor.L) > daycardAnchorTolerance ||
		math.Abs(red.Editor.T-ctrl.Editor.T) > daycardAnchorTolerance {
		t.Errorf("C-CALV4-CARD-REDUCED-ANCHOR: under prefers-reduced-motion the editor "+
			"rests at %s but every other reader gets %s, for the same day %q whose cell is "+
			"at %s (the card opened at %s). Reduced motion makes closeDelayMS return 0, so "+
			"closeCard hides the card SYNCHRONOUSLY and the placement law is handed a 0×0 "+
			"anchor; the anchor rect must be captured BEFORE closeCard, beside the morph's "+
			"own fromRect", red.Editor, ctrl.Editor, red.Day, red.Cell, red.Card)
	}
	// SIZE TOO, because an anchor fix that also shrank the box would be a
	// different regression wearing this one's pass.
	if math.Abs(red.Editor.W-ctrl.Editor.W) > daycardAnchorTolerance ||
		math.Abs(red.Editor.H-ctrl.Editor.H) > daycardAnchorTolerance {
		t.Errorf("the reduced-motion editor measures %s where the no-preference one "+
			"measures %s — reduced motion removes the flight, not the box", red.Editor, ctrl.Editor)
	}
}

// daycardAnchorEdgePad is placeCard's viewport pad. A box resting here is a box
// whose anchor measured 0×0 at 0,0, which is the defect's signature.
const daycardAnchorEdgePad = 8

var daycardAnchorResultRe = regexp.MustCompile(`(?s)<pre id="anchor">(.*?)</pre>`)

// daycardAnchorRun drives one arm and returns what the page measured.
func daycardAnchorRun(t *testing.T, chrome, src string, reduced bool) daycardAnchorShot {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	args := []string{
		"--headless", "--no-sandbox", "--disable-gpu", "--hide-scrollbars",
		"--window-size=1440,900", "--virtual-time-budget=8000",
	}
	if reduced {
		args = append(args, "--force-prefers-reduced-motion")
	}
	args = append(args, "--dump-dom", "file://"+src)
	out, err := exec.CommandContext(ctx, chrome, args...).Output()
	if err != nil {
		t.Fatalf("chromium dump-dom (reduced=%v): %v", reduced, err)
	}
	m := daycardAnchorResultRe.FindSubmatch(out)
	if m == nil {
		t.Fatalf("no <pre id=\"anchor\"> in the dump (reduced=%v)", reduced)
	}
	raw := strings.ReplaceAll(string(m[1]), "&quot;", `"`)
	raw = strings.ReplaceAll(raw, "&amp;", "&")
	var shot daycardAnchorShot
	if err := json.Unmarshal([]byte(raw), &shot); err != nil {
		t.Fatalf("probe payload (reduced=%v) is not JSON: %v\n%s", reduced, err, raw)
	}
	t.Logf("%s arm: day %s cell %s card %s editor %s sheet=%v (page saw reduced=%v)",
		map[bool]string{true: "reduced", false: "no-preference"}[reduced],
		shot.Day, shot.Cell, shot.Card, shot.Editor, shot.Sheet, shot.Reduced)
	return shot
}

// daycardAnchorPage builds the page both arms run: the REAL Bench surface, the
// REAL sheets, the REAL module, and nothing stubbed.
func daycardAnchorPage(t *testing.T, data BenchData) string {
	t.Helper()
	surface := renderBench(t, data)
	css := benchCSS(t) + benchBlockSheet(t) + dayCardCSS(t)
	mod := readRepoFile(t, "internal/plugins/calendar/static/js/calendar_daycard.js")
	vis := readRepoFile(t, "internal/plugins/calendar/static/js/cal_visibility.js")

	return `<!doctype html><html><head><meta charset="utf-8"><style>` +
		`html,body{margin:0;padding:0;background:#f9fafb;` +
		`font-family:ui-sans-serif,system-ui,-apple-system,sans-serif}` +
		`.cal-bench{width:1180px;max-width:100%}` +
		css +
		`</style></head><body>` +
		benchStripLinks(surface) +
		`<pre id="anchor">{"err":"the driver never ran"}</pre>` +
		`<script>` + vis + `</script><script>` + mod + `</script>` +
		`<script>` + daycardAnchorScript + `</script>` +
		`</body></html>`
}

// daycardAnchorScript is the driver. It makes the two clicks a reader makes,
// through the module's own listeners, and reports where the box came to rest.
//
// IT PICKS A MID-GRID CELL, NOT THE FIRST ONE. The first day of the fixture's
// grid sits near the left edge, where a correctly placed box and a box dumped
// at the viewport's corner are only a few pixels apart — the middle of the row
// is where the defect's 512px is visible and where the control is honestly off
// the edge.
const daycardAnchorScript = `
window.addEventListener('load', function () {
  var shot = { reduced: false, day: '', cell: null, card: null, editor: null,
               sheet: false, err: '' };
  function rect(el) {
    var r = el.getBoundingClientRect();
    return { l: Math.round(r.left), t: Math.round(r.top),
             w: Math.round(r.width), h: Math.round(r.height) };
  }
  function done() {
    document.getElementById('anchor').textContent = JSON.stringify(shot);
  }
  try {
    shot.reduced = !!(window.matchMedia &&
      window.matchMedia('(prefers-reduced-motion: reduce)').matches);
    var ed = document.querySelector('[data-cal-dayeditor]');
    var card = document.querySelector('[data-cal-daycard]');
    // ONE BLOCK, AND IT IS THE ONE THE MOUNT'S PAYLOAD DESCRIBES. The Bench
    // renders a Block per calendar and the day-card payload carries exactly
    // one of them, so a cell taken from the page-wide list can belong to a
    // calendar the module has no record of — openCard returns early, nothing
    // opens, and the probe measures two collapsed boxes instead of a placement.
    var block = document.querySelector('[data-bench-block]');
    var cells = block ? Array.prototype.slice.call(
      block.querySelectorAll('[data-day][data-day-ord]')) : [];
    if (!ed || !card || !cells.length) {
      shot.err = 'the fixture is missing the card, the editor or a day cell';
      done();
      return;
    }
    var cell = cells[Math.floor(cells.length / 2)];
    shot.day = cell.getAttribute('data-day') || '';
    shot.cell = rect(cell);
    cell.click();
    if (!card.hasAttribute('data-dc-shown')) {
      shot.err = 'clicking day ' + shot.day + ' did not open the card';
      done();
      return;
    }
    shot.card = rect(card);
    var door = document.querySelector('[data-dc-new]');
    if (!door) {
      shot.err = 'the card opened without a "+ New event" door';
      done();
      return;
    }
    door.click();
    // The morph is a transient and this driver is synchronous, so without the
    // settle the no-preference arm would report the transition's t=0 value —
    // the CARD's rect — and the two arms would be compared at different moments
    // of the same open. See daycardSettleMorph's header.
` + daycardSettleMorph + `
    shot.editor = rect(ed);
    shot.sheet = ed.classList.contains('dcsheet');
  } catch (e) {
    shot.err = String((e && e.message) || e);
  }
  done();
});
`
