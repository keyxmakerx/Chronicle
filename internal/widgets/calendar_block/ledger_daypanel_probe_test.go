// ledger_daypanel_probe_test.go — the day panel MEASURED, in a real engine.
//
// The markup tests next door can see that the panel exists and that its door is
// gated. They cannot see any of the four things that actually decide whether it
// works, because every one is a question about flexbox, sticky positioning, a
// container query and a min-height interacting:
//
//  1. is the panel INVISIBLE at rest and VISIBLE when the day is chosen — on a
//     DOM whose only difference is a checked radio, which is exactly what a tap
//     on the stretched `.dsel` label produces and the whole reason there is no
//     JS in this package;
//  2. at 390px — THE OPERATOR'S PHONE — does the Ledger still sit BELOW the
//     month, or has the panel pushed the column somewhere else;
//  3. is the create door a real hit target: non-zero box, inside the Ledger,
//     and the top element at its own centre point (a door under an overlay is
//     a door nobody can press, and it looks perfect in a screenshot);
//  4. does the sticky panel OVERLAP the first row it is supposed to head.
//
// It reuses the Ledger probe's harness (runLedgerProbe, ledgerProbeBoxes'
// isolate-and-check idiom) rather than starting a second one, so both probes
// measure the same page shape and cannot drift apart about what "the Block at
// 390px" means.

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

// dayPanelReading is one host box's measurement.
type dayPanelReading struct {
	Host int `json:"host"`
	// PanelH / PanelW are the CHOSEN day's panel. Zero on a box with no day
	// chosen, which is the at-rest half of assertion 1.
	PanelH float64 `json:"panelH"`
	PanelW float64 `json:"panelW"`
	// PanelsVisible counts every visible `.ldp` — it must be 0 or 1, never
	// more. Forty panels revealed at once is what a mis-scoped ladder rule
	// would produce, and it would still measure "a panel is visible".
	PanelsVisible int    `json:"panelsVisible"`
	DateText      string `json:"dateText"`
	MoonText      string `json:"moonText"`
	Sticky        string `json:"sticky"`

	// GridBot / LedgerTop answer the stacking question directly.
	GridBot   float64 `json:"gridBot"`
	LedgerTop float64 `json:"ledgerTop"`
	BlockH    float64 `json:"blockH"`
	RowsH     float64 `json:"rowsH"`

	// The door, and whether it can actually be pressed.
	DoorW      float64 `json:"doorW"`
	DoorH      float64 `json:"doorH"`
	DoorInside bool    `json:"doorInside"`
	DoorOnTop  bool    `json:"doorOnTop"`
	// DoorHit NAMES whatever the engine would actually hand the tap. A boolean
	// alone sends the next reader hunting; the culprit's own tag and class list
	// is the difference between a red test and a red test that says what to fix.
	DoorHit string `json:"doorHit"`

	// PanelOverRow is true when the panel's box overlaps a visible row's box.
	// Sticky headers are ALLOWED to overlay while scrolling; at scrollTop 0 the
	// first row must begin under the panel, not behind it.
	PanelOverRow bool `json:"panelOverRow"`
}

const dayPanelProbeScript = `
function(root){
  var r = function(el){ return el ? el.getBoundingClientRect() : null };
  var vis = function(el){ var b = r(el); return !!b && b.width > 0 && b.height > 0 };
  var block  = root.querySelector('.block');
  var grid   = root.querySelector('.grid');
  var ledger = root.querySelector('[data-zone="ledger"]');
  var rows   = root.querySelector('.lrows');
  var panels = [].slice.call(root.querySelectorAll('.ldp')).filter(vis);
  var panel  = panels[0] || null;
  var pb = panel ? r(panel) : null;
  var door = panel ? panel.querySelector('.ldnew') : null;
  var db = door ? r(door) : null;
  var lb = ledger ? r(ledger) : null;

  // THE HIT TEST IS THE POINT. A control with a box is not a control anyone
  // can press: elementFromPoint answers what the ENGINE would hand the tap,
  // which is the only reading that can see a stretched .dsel label, a sticky
  // sibling or a z-index accident sitting over the door.
  //
  // IT SCROLLS THE DOOR INTO VIEW FIRST, AND THAT IS NOT A DETAIL. This page
  // stacks every host box vertically, so at std tier the later boxes sit
  // thousands of pixels down; elementFromPoint is VIEWPORT-relative and answers
  // null for a point below the fold. The first version of this probe read that
  // null as "something is over the door" and reported a defect at three of four
  // widths that did not exist. A rect is re-read AFTER the scroll, because the
  // scroll moves it.
  var onTop = false, hitName = '';
  if (db && db.width > 0 && db.height > 0) {
    door.scrollIntoView({ block: 'center', inline: 'center' });
    var dv = r(door);
    var hit = document.elementFromPoint(dv.left + dv.width / 2, dv.top + dv.height / 2);
    onTop = !!hit && (hit === door || door.contains(hit));
    hitName = hit
      ? (hit.tagName.toLowerCase() + '.' + (hit.className || '(none)') +
         ' <- ' + (hit.parentElement
           ? hit.parentElement.tagName.toLowerCase() + '.' + (hit.parentElement.className || '(none)')
           : '(root)'))
      : '(nothing — the point is outside the viewport, which is a PROBE fault ' +
        'and not a product one: scroll it in before reading)';
  }

  var over = false;
  if (pb) {
    [].slice.call(rows ? rows.querySelectorAll('.lrow') : []).forEach(function(el){
      if (!vis(el)) return;
      var b = r(el);
      if (pb.bottom > b.top + 0.5 && b.bottom > pb.top + 0.5) over = true;
    });
  }

  return {
    host: Math.round(r(root).width),
    panelH: pb ? pb.height : 0,
    panelW: pb ? pb.width : 0,
    panelsVisible: panels.length,
    dateText: panel ? ((panel.querySelector('.ldn') || {}).textContent || '').trim() : '',
    moonText: panel ? ((panel.querySelector('.ldm') || {}).textContent || '').trim() : '',
    sticky: panel ? getComputedStyle(panel).position : '',
    gridBot: grid ? r(grid).bottom : 0,
    ledgerTop: lb ? lb.top : 0,
    blockH: r(block).height,
    rowsH: rows ? r(rows).height : 0,
    doorW: db ? db.width : 0,
    doorH: db ? db.height : 0,
    doorInside: !!(db && lb && db.top >= lb.top - 1 && db.bottom <= lb.bottom + 1),
    doorOnTop: onTop,
    doorHit: hitName,
    panelOverRow: over
  };
}`

// runDayPanelProbe lays every box out in one window and reads them back in one
// Chromium run — the Ledger probe's harness shape, verbatim, so the two cannot
// disagree about what a Block at a given width is.
func runDayPanelProbe(t *testing.T, chrome string, boxes, styles []string) []dayPanelReading {
	t.Helper()
	css := blockCSS(t)
	var body strings.Builder
	for i, markup := range boxes {
		fmt.Fprintf(&body, `<div class="probe-host" id="h%d" style="%s">%s</div>`,
			i, styles[i], markup)
	}
	page := `<!doctype html><html><head><meta charset="utf-8"><style>` +
		`html,body{margin:0;background:#fff}.probe-host{display:block;margin:24px}` +
		css + `</style></head><body>` + body.String() +
		`<script>document.addEventListener('DOMContentLoaded',function(){` +
		`var read=` + dayPanelProbeScript + `;` +
		`var out=[].slice.call(document.querySelectorAll('.probe-host')).map(read);` +
		`document.body.setAttribute('data-probe', JSON.stringify(out));});</script>` +
		`</body></html>`

	dir := t.TempDir()
	path := filepath.Join(dir, "daypanel-probe.html")
	if err := os.WriteFile(path, []byte(page), 0o644); err != nil {
		t.Fatalf("write probe page: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, chrome, "--headless", "--no-sandbox", "--disable-gpu",
		"--hide-scrollbars", "--window-size=1600,2400", "--virtual-time-budget=5000",
		"--dump-dom", "file://"+path)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("chromium: %v", err)
	}
	m := probePayloadRe.FindStringSubmatch(string(out))
	if m == nil {
		t.Fatal("no probe payload in the rendered DOM — the page script did not run")
	}
	var readings []dayPanelReading
	if err := json.Unmarshal([]byte(html.UnescapeString(m[1])), &readings); err != nil {
		t.Fatalf("probe payload: %v", err)
	}
	return readings
}

// TestProbe_DayPanelAppearsOnTapAndCostsTheLedgerNothing.
//
// 390px is the operator's phone and it is in this list by name. 358px and 420px
// are the two production std host widths CTS-8 measures at; 1232px is full tier.
func TestProbe_DayPanelAppearsOnTapAndCostsTheLedgerNothing(t *testing.T) {
	if testing.Short() {
		t.Skip("browser probe: skipped under -short")
	}
	chrome := findProbeChromium()
	if chrome == "" {
		t.Skip("browser probe: no Chromium binary found (set CHROMIUM_BIN)")
	}

	widths := []int{1232, 420, 390, 358}
	d := fxDayPanel(t, true)
	base := stripLink(render(t, d))

	var boxes, styles []string
	for i, w := range widths {
		// Radios sharing a name are ONE GROUP document-wide, so each box gets
		// its own suffix — without it only the last `checked` in the page
		// survives and every earlier box measures unselected, which would make
		// the whole probe pass vacuously.
		unsel := isolateBlockControls(base, fmt.Sprintf("-q%da", i))
		sel := isolateBlockControls(base, fmt.Sprintf("-q%db", i))
		picked := strings.Replace(sel,
			`data-day-pick="4" name=`, `data-day-pick="4" checked name=`, 1)
		if picked == sel {
			t.Fatal("could not check day 4 in the rendered markup — the probe would measure " +
				"two identical DOMs and pass vacuously")
		}
		boxes = append(boxes, unsel, picked)
		styles = append(styles,
			fmt.Sprintf("width:%dpx", w), fmt.Sprintf("width:%dpx", w))
	}

	readings := runDayPanelProbe(t, chrome, boxes, styles)
	if len(readings) != 2*len(widths) {
		t.Fatalf("probe returned %d readings for %d boxes", len(readings), 2*len(widths))
	}

	for i, w := range widths {
		rest, sel := readings[2*i], readings[2*i+1]
		t.Logf("host %dpx — at rest: %d panels visible, block %.1fpx, rows %.1fpx · "+
			"chosen: %d panel(s) %.1f×%.1fpx (%s), block %.1fpx, rows %.1fpx, "+
			"door %.1f×%.1fpx inside=%v onTop=%v, date %q",
			w, rest.PanelsVisible, rest.BlockH, rest.RowsH,
			sel.PanelsVisible, sel.PanelW, sel.PanelH, sel.Sticky,
			sel.BlockH, sel.RowsH, sel.DoorW, sel.DoorH, sel.DoorInside, sel.DoorOnTop,
			sel.DateText)

		// 1. AT REST THERE IS NO PANEL, AND WHEN A DAY IS CHOSEN THERE IS
		//    EXACTLY ONE. "Exactly one" is the half that would fail silently: a
		//    mis-scoped reveal would show forty and still satisfy "a panel is
		//    visible".
		if rest.PanelsVisible != 0 {
			t.Errorf("host %dpx: %d day panels visible with NO day chosen — every panel is "+
				"emitted on every render and the ladder reveals one", w, rest.PanelsVisible)
		}
		if sel.PanelsVisible != 1 {
			t.Errorf("host %dpx: %d day panels visible with day 4 chosen; the ladder reveals "+
				"exactly one", w, sel.PanelsVisible)
		}
		if sel.PanelH <= 0 || sel.PanelW <= 0 {
			t.Errorf("host %dpx: the chosen day's panel measures %.1f×%.1fpx",
				w, sel.PanelW, sel.PanelH)
		}
		if sel.Sticky != "sticky" {
			t.Errorf("host %dpx: the panel computes position=%q, not \"sticky\" — it is a "+
				"header for the rows under it and they scroll (.lband's idiom)", w, sel.Sticky)
		}
		if !strings.Contains(sel.DateText, "4 ") {
			t.Errorf("host %dpx: the panel's date header reads %q; it must name the chosen day",
				w, sel.DateText)
		}
		if sel.MoonText == "" {
			t.Errorf("host %dpx: the panel drew no moon line on a four-moon calendar", w)
		}

		// 2. THE PANEL COSTS THE BLOCK NOTHING. Same promise the docked Ledger
		//    was built on, now with a header inside the scroller: choosing a day
		//    repaints a column that was already there.
		if diff := sel.BlockH - rest.BlockH; diff > 1 || diff < -1 {
			t.Errorf("host %dpx: the Block is %.1fpx at rest and %.1fpx with a day chosen — "+
				"the panel reflowed the component, which is the one thing the docked Ledger "+
				"exists to prevent", w, rest.BlockH, sel.BlockH)
		}
		if diff := sel.RowsH - rest.RowsH; diff > 1 || diff < -1 {
			t.Errorf("host %dpx: `.lrows` is %.1fpx at rest and %.1fpx with a day chosen — "+
				"the panel is INSIDE the scroller precisely so the box does not move",
				w, rest.RowsH, sel.RowsH)
		}

		// 3. THE DOOR IS A REAL HIT TARGET. Tap is primary; a door that
		//    measures a box but hands its centre point to something else is a
		//    door nobody can press and a screenshot cannot tell you so.
		if sel.DoorW <= 0 || sel.DoorH <= 0 {
			t.Errorf("host %dpx: the create door measures %.1f×%.1fpx", w, sel.DoorW, sel.DoorH)
		}
		if !sel.DoorInside {
			t.Errorf("host %dpx: the create door is drawn outside the Ledger's own box", w)
		}
		if !sel.DoorOnTop {
			t.Errorf("host %dpx: elementFromPoint at the create door's centre is NOT the door "+
				"— the tap would go to %s instead", w, sel.DoorHit)
		}

		// 4. THE STICKY HEADER DOES NOT SIT ON THE FIRST ROW at scrollTop 0.
		if sel.PanelOverRow {
			t.Errorf("host %dpx: the day panel overlaps a listed row at rest — a sticky "+
				"header may overlay while SCROLLING, never before anything has scrolled", w)
		}
	}
}

// TestProbe_AtThePhoneWidthTheLedgerStillSitsBelowTheMonth is the brief's item
// 4, measured rather than asserted.
//
// The std tier stacks the Ledger UNDER the month — that is what makes the
// component usable in one thumb's width — and a header added inside the Ledger
// is exactly the kind of change that quietly turns a column back into a
// side-by-side. 390px is the operator's own device width; 358px and 420px are
// the two production std hosts; 1232px is full tier, where the Ledger sits
// BESIDE the month and must still do so.
func TestProbe_AtThePhoneWidthTheLedgerStillSitsBelowTheMonth(t *testing.T) {
	if testing.Short() {
		t.Skip("browser probe: skipped under -short")
	}
	chrome := findProbeChromium()
	if chrome == "" {
		t.Skip("browser probe: no Chromium binary found (set CHROMIUM_BIN)")
	}

	widths := []int{1232, 420, 390, 358}
	d := fxDayPanel(t, true)
	base := stripLink(render(t, d))

	var boxes, styles []string
	for i, w := range widths {
		sel := isolateBlockControls(base, fmt.Sprintf("-s%d", i))
		picked := strings.Replace(sel,
			`data-day-pick="4" name=`, `data-day-pick="4" checked name=`, 1)
		if picked == sel {
			t.Fatal("could not check day 4 — the probe would measure an unselected Block")
		}
		boxes = append(boxes, picked)
		styles = append(styles, fmt.Sprintf("width:%dpx", w))
	}

	readings := runDayPanelProbe(t, chrome, boxes, styles)
	for i, w := range widths {
		r := readings[i]
		stacked := r.LedgerTop >= r.GridBot-1
		t.Logf("host %dpx — grid bottom %.1f, ledger top %.1f → %s (panel %.1fpx tall)",
			w, r.GridBot, r.LedgerTop, map[bool]string{true: "STACKED", false: "beside"}[stacked],
			r.PanelH)

		if w >= 1232 {
			if stacked {
				t.Errorf("host %dpx: the Ledger stacked below the month at FULL tier — the "+
					"docked column is the whole point of the tier", w)
			}
			continue
		}
		if !stacked {
			t.Errorf("host %dpx: the Ledger's top is %.1f and the month's bottom is %.1f — at "+
				"std tier the Ledger sits BELOW the month, and a phone that puts them side by "+
				"side has two unusable columns", w, r.LedgerTop, r.GridBot)
		}
	}
}
