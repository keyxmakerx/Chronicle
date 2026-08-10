// daycard_ledger_door_probe_test.go — "Open in the Ledger" HAS TO EARN ITS ROW.
//
// THE MEASUREMENT THAT STARTED THIS. A day cell's real hit target is the
// STRETCHED LABEL `.dsel` (instrument.templ's dayPick), and that label is `for`
// the day's own `input.daypick` radio. So a click on a day ALREADY selects that
// day in the Ledger, natively, before any JavaScript runs — the same radio
// `openInLedger()` goes on to click a second time.
//
// Which leaves the card's `Open in the Ledger` door, on a DOCKED Ledger, doing
// exactly three things: clicking a radio that is already checked; scrolling a
// column into view that is already fully in view; and closing the card. Only the
// third is an effect, and "close this card" is not what the button says. It is a
// row in a two-row foot, on a surface [DC-3] signed as small, spending itself on
// a no-op.
//
// IT IS NOT REDUNDANT EVERYWHERE, and that is why the fix is a CONDITION rather
// than a deletion. `.cal-block-host .body` is a flex COLUMN by default and only
// becomes a two-column grid at `@container cal-block (min-width: 900px)`
// (calendar-block.css:2168). Below that the Ledger is a full-width band STACKED
// BELOW the month — off the bottom of the card's own viewport for most days —
// and jumping to it is a real service.
//
// THE CHOICE, STATED: CONDITIONAL ON THE STACKED LAYOUT, NOT RENAMED.
// A rename cannot fix a docked-Ledger door, because on a docked Ledger the
// control's entire net effect is closing a card that already closes on
// outside-click, on Escape and on the next day click. There is no honest name
// for that button. And the stacked/docked distinction is not a guess this
// module has to invent: it is a rect this module ALREADY measures for the
// occlusion dodge, so the condition is a measurement.
//
// WHAT THIS PROBE ASSERTS, at two container widths, in a real engine:
//
//  1. AT ≥900px THE LEDGER IS DOCKED BESIDE THE MONTH, a day click leaves that
//     day's radio CHECKED, and the Ledger is fully within the viewport. Those
//     three together are the redundancy, measured rather than argued.
//  2. …and therefore the card's foot carries NO `[data-dc-ledger]` door.
//  3. BELOW 900px THE LEDGER IS STACKED BELOW THE MONTH — and the door IS
//     there. Without this arm the whole fix could be "delete the button", which
//     is not the fix.
//
// IT SKIPS HONESTLY under -short or with no Chromium, and a skipped run is NOT a
// pass.
//
//	go test ./internal/plugins/calendar/ -run LedgerDoor -v
package calendar

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"
)

// ledgerDoorReading is one width's answer.
type ledgerDoorReading struct {
	Viewport int `json:"viewport"`
	// The Block's own container width, which is what the 900px boundary is
	// stated against — the viewport is not the subject.
	BlockW float64 `json:"blockW"`
	// Geometry: is the Ledger BESIDE the month or BELOW it?
	MonthBottom float64 `json:"monthBottom"`
	LedgerTop   float64 `json:"ledgerTop"`
	LedgerW     float64 `json:"ledgerW"`
	Stacked     bool    `json:"stacked"`
	// Is the whole Ledger already on screen at the moment the card is open?
	LedgerInView bool `json:"ledgerInView"`
	// After clicking the day: did the native label already select it?
	RadioChecked bool `json:"radioChecked"`
	// What a pointer at the cell's centre actually lands on.
	Hit string `json:"hit"`
	// Did the card open, and does its foot carry the door?
	CardOpen bool `json:"cardOpen"`
	Door     bool `json:"door"`
	// The foot's other doors, so "no Ledger row" can be told apart from "no
	// foot rendered at all".
	FootDoors int `json:"footDoors"`
}

const ledgerDoorProbeScript = `
window.addEventListener('load', function () {
  function r(el){ return el ? el.getBoundingClientRect() : null; }
  var out = null;
  var hit = '';
  try {
    var host  = document.querySelector('[data-bench-block="primary"]');
    var month = host && host.querySelector('.inst');
    var zone  = host && host.querySelector('[data-zone="ledger"]');
    var cell  = host && host.querySelector('[data-day][data-day-ord]');
    var card  = document.querySelector('[data-cal-daycard]');
    if (host && month && zone && cell && card) {
      // HIT-TEST, DO NOT cell.click(). The day's real hit target is the
      // absolutely-positioned .dsel label stretched under the cell's
      // interactive children (instrument.templ's dayPick), and a synthetic
      // click dispatched ON THE CELL never activates it — so a probe that
      // clicked the cell would report "the day click does not select the day"
      // and be measuring its own synthesis. elementFromPoint at the cell's
      // centre is what a pointer actually lands on.
      var cr0 = r(cell);
      var target = document.elementFromPoint(
        cr0.left + cr0.width / 2, cr0.top + cr0.height / 2) || cell;
      hit = target.className || target.tagName;
      target.click();
      var hr = r(host), mr = r(month), zr = r(zone);
      var ord = cell.getAttribute('data-day-ord');
      var radio = host.querySelector('input.daypick[data-day-pick="' + ord + '"]');
      var open = card.hasAttribute('data-dc-shown');
      var foot = card.querySelector('[data-dc-foot]');
      out = {
        viewport: window.innerWidth,
        blockW: Math.round(hr.width * 10) / 10,
        monthBottom: Math.round(mr.bottom * 10) / 10,
        ledgerTop: Math.round(zr.top * 10) / 10,
        ledgerW: Math.round(zr.width * 10) / 10,
        // BELOW rather than BESIDE: the band starts at or after the month ends.
        stacked: zr.top >= mr.bottom - 1,
        ledgerInView: zr.top >= 0 && zr.bottom <= window.innerHeight,
        radioChecked: !!(radio && radio.checked),
        hit: hit,
        cardOpen: open,
        door: !!(foot && foot.querySelector('[data-dc-ledger]')),
        footDoors: foot ? foot.children.length : -1
      };
    }
  } catch (e) { out = { err: String(e) }; }
  document.body.setAttribute('data-probe', JSON.stringify(out));
});`

var ledgerDoorPayloadRe = regexp.MustCompile(`data-probe="([^"]*)"`)

// TestDaycardLedgerDoor_ItOnlyRendersWhereItDoesSomething is the whole of it.
func TestDaycardLedgerDoor_ItOnlyRendersWhereItDoesSomething(t *testing.T) {
	if testing.Short() {
		t.Skip("the Ledger-door probe needs a browser; skipped under -short (CI's mode)")
	}
	chrome := findChromium()
	if chrome == "" {
		t.Skip("no Chromium binary found (set CHROMIUM_BIN) — a skipped probe is NOT a pass")
	}

	data := benchFxData(true, true)
	page := daycardProbePage(t, data)
	// The sweep script the geometry probe injects is not wanted here; this file
	// asks one question per width.
	page = strings.Replace(page, `<pre id="probe">[]</pre>`, `<pre id="probe">[]</pre>`, 1)
	page = daycardProbeStripSweep(page) + `<script>` + ledgerDoorProbeScript + `</script></body></html>`

	for _, tc := range []struct {
		name string
		// The VIEWPORT, chosen so the Block's own container lands either side of
		// the 900px boundary the sheet declares.
		viewport int
		stacked  bool
		wantDoor bool
	}{
		{"docked — the Ledger sits beside the month", 1600, false, false},
		{"stacked — the Ledger sits below the month", 820, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := ledgerDoorRun(t, chrome, page, tc.viewport)

			t.Logf("viewport %dpx · Block %.1fpx · month bottom %.1f / Ledger top %.1f "+
				"(w %.1f) · stacked=%v · Ledger fully in view=%v · day radio checked "+
				"after the click=%v · pointer hit %q · card open=%v · foot doors=%d · "+
				"Ledger door=%v",
				r.Viewport, r.BlockW, r.MonthBottom, r.LedgerTop, r.LedgerW, r.Stacked,
				r.LedgerInView, r.RadioChecked, r.Hit, r.CardOpen, r.FootDoors, r.Door)

			if !r.CardOpen {
				t.Fatal("the day card did not open — every claim below would pass vacuously")
			}
			if r.Stacked != tc.stacked {
				t.Fatalf("the Ledger is stacked=%v at %dpx, want %v — this row is about the "+
					"other layout and its verdict would be the wrong one",
					r.Stacked, tc.viewport, tc.stacked)
			}

			// (1) THE REDUNDANCY, MEASURED — only where the door is claimed to be
			// redundant. A day click must ALREADY have selected the day.
			if !tc.stacked {
				if !r.RadioChecked {
					t.Errorf("clicking the day did not check its `daypick` radio — the " +
						"whole premise of this fix is that the stretched .dsel label " +
						"already selects the day, and if that stopped being true the " +
						"Ledger door would be doing real work again and should come back")
				}
				if !r.LedgerInView {
					t.Errorf("the docked Ledger is not fully within the viewport, so " +
						"scrolling to it is not a no-op after all — the door's second " +
						"effect is real here and the condition below is wrong")
				}
			}

			// (2) / (3) THE DOOR IS WHERE IT DOES SOMETHING AND NOWHERE ELSE.
			if r.Door != tc.wantDoor {
				if tc.wantDoor {
					t.Errorf("the card's foot carries NO `Open in the Ledger` door with the " +
						"Ledger stacked BELOW the month — this is the layout where the door " +
						"earns its keep, and dropping it here would be deleting the control " +
						"rather than conditioning it")
				} else {
					t.Errorf("the card's foot carries an `Open in the Ledger` door with the "+
						"Ledger docked BESIDE the month, already showing the day the click "+
						"selected (radio checked=%v) and already fully in view (=%v). The "+
						"door clicks a checked radio, scrolls to something already on "+
						"screen, and closes the card — and \"close this card\" is not what "+
						"it says", r.RadioChecked, r.LedgerInView)
				}
			}
			if r.FootDoors <= 0 && tc.wantDoor {
				t.Errorf("the foot rendered %d children — the door's absence here is the "+
					"foot not rendering at all, which is a different defect", r.FootDoors)
			}
		})
	}
}

// daycardProbeStripSweep removes the geometry probe's own in-page sweep so this
// file's single question runs against a clean page. It cuts at the last
// <script> block, which is where daycardProbePage appends it.
func daycardProbeStripSweep(page string) string {
	i := strings.LastIndex(page, "<script>")
	if i < 0 {
		return page
	}
	return page[:i]
}

func ledgerDoorRun(t *testing.T, chrome, page string, viewport int) ledgerDoorReading {
	t.Helper()
	dir := t.TempDir()
	src := dir + "/ledger-door.html"
	writeProbeFile(t, src, page)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, chrome,
		"--headless", "--no-sandbox", "--disable-gpu", "--hide-scrollbars",
		fmt.Sprintf("--window-size=%d,1200", viewport),
		"--virtual-time-budget=8000",
		"--dump-dom", "file://"+src,
	).Output()
	if err != nil {
		t.Fatalf("chromium at %d: %v", viewport, err)
	}
	m := ledgerDoorPayloadRe.FindStringSubmatch(string(out))
	if m == nil {
		t.Fatalf("no probe payload at %d — the page script did not run", viewport)
	}
	var r ledgerDoorReading
	if err := json.Unmarshal([]byte(html.UnescapeString(m[1])), &r); err != nil {
		t.Fatalf("probe payload at %d: %v\n%s", viewport, err, html.UnescapeString(m[1]))
	}
	return r
}
