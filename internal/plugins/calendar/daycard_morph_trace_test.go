package calendar

// daycard_morph_trace_test.go — C-CALV4-EDITOR-R2b stage 9, and it exists
// because §10 item 5 could not be closed the way the dispatch asks.
//
// ── WHAT THE CAPTURE RIG CANNOT DO, MEASURED RATHER THAN ASSERTED ─────────
//
// §10 item 5 asks for the morph mid-flight: "pause the animation and shoot at
// least three frames — start, ~50%, end — with getAnimations()-driven pausing".
// The rig does exactly that and the frames come back showing a FINISHED EDITOR
// at every fraction. The reason is measurable and it is the rig's, not the
// module's:
//
//   · The park hook (`transitionrun`, idempotent, on every event) reports
//     PARKED 0 transitions at 0% and, non-deterministically, 1 (`height`) at
//     other fractions. The editor's box measures its FINAL geometry in every
//     frame.
//   · Under `--dump-dom --virtual-time-budget`, `getComputedStyle` returns
//     STALE values after a forced `offsetHeight`: writing `inline-size: 340px`
//     to the root and reading it back yields `760px`, across a rAF as well.
//     The document's rendering lifecycle is not being run, so transitions are
//     never created to be parked.
//
// A capture that cannot create the transition cannot photograph it, and five
// identical images captioned "25%" would be worse than no images. So the claim
// is made where it CAN be made: at the GEOMETRY, in a real browser, through the
// module's own writes.
//
// ── WHAT THIS PROBE PROVES, AND WHAT IT DOES NOT ──────────────────────────
//
// PROVES, in real Chromium, on the real Bench markup with the real sheets and
// the shipped module: the editor's box is SEEDED at the card's measured rect
// (offset by `translate`, sized to the card, `opacity: 0`), the carve-out class
// is on the box while that is true, and the box then lands at the placement
// law's own answer (`translate: 0px`, the sheet's width, `opacity: 1`) — ONE
// BOX, growing from the card's geometry, by the four named properties and no
// fifth. It reads the style attribute through a MutationObserver, so it is
// observing what the module actually wrote rather than what a stub recorded.
//
// DOES NOT PROVE that the compositor interpolated between them. That is the
// part this environment cannot answer, and the report says so in those words
// rather than implying it with a picture.

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

type daycardMorphStep struct {
	Class string `json:"class"`
	Style string `json:"style"`
	W     int    `json:"w"`
	H     int    `json:"h"`
}

type daycardMorphTrace struct {
	CardRect struct {
		L int `json:"l"`
		T int `json:"t"`
		W int `json:"w"`
		H int `json:"h"`
	} `json:"cardRect"`
	Steps []daycardMorphStep `json:"steps"`
}

func TestDayCardMorphGeometryTrace(t *testing.T) {
	if os.Getenv("DAYCARD_MORPH_TRACE") == "" {
		t.Skip("morph geometry trace: set DAYCARD_MORPH_TRACE=1 to run")
	}
	chrome := benchFindChromium()
	if chrome == "" {
		t.Skip("morph geometry trace: no Chromium binary found (set CHROMIUM_BIN)")
	}

	data := benchFxData(true, true)
	data.DayCard = DayCardMount{CanCreate: true, CanAuthorDmOnly: true, CanDelete: true,
		CanRestrict: true, CampaignID: "camp-1"}
	cal := benchFxTypedCalendar()
	data.DayCardJSON = dayCardPayloadJSON(
		dayCardSeed{CanAuthor: true, CanRestrict: true, Roster: benchFxRoster()},
		dayCardSource{Block: data.Primary, Calendar: &cal})

	page := daycardShotPage(t,
		daycardShot{w: 1440, h: 900, script: daycardMorphTraceScript},
		benchCSS(t)+benchBlockSheet(t)+dayCardCSS(t),
		benchStripLinks(renderBench(t, data)))
	dir := t.TempDir()
	src := filepath.Join(dir, "trace.html")
	if err := os.WriteFile(src, []byte(page), 0o644); err != nil {
		t.Fatalf("write trace page: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, chrome,
		"--headless", "--no-sandbox", "--disable-gpu", "--hide-scrollbars",
		fmt.Sprintf("--window-size=%d,%d", 1440, 900),
		"--virtual-time-budget=8000", "--dump-dom", "file://"+src).Output()
	if err != nil {
		t.Fatalf("chromium dump-dom: %v", err)
	}
	m := regexp.MustCompile(`(?s)<pre id="morphtrace">(.*?)</pre>`).FindSubmatch(out)
	if m == nil {
		t.Fatal("no <pre id=\"morphtrace\"> in the dump; the trace did not run")
	}
	raw := strings.ReplaceAll(string(m[1]), "&quot;", `"`)
	raw = strings.ReplaceAll(raw, "&amp;", "&")
	var tr daycardMorphTrace
	if err := json.Unmarshal([]byte(raw), &tr); err != nil {
		t.Fatalf("trace payload is not JSON: %v\n%s", err, raw)
	}

	if tr.CardRect.W <= 0 || tr.CardRect.H <= 0 {
		t.Fatal("the card had no measurable rect, so the morph declined and there is " +
			"nothing to trace — the fixture, not the module")
	}
	t.Logf("── the morph's geometry, traced in real Chromium ──────────────")
	t.Logf("  the card's measured rect: %d×%d at (%d,%d)",
		tr.CardRect.W, tr.CardRect.H, tr.CardRect.L, tr.CardRect.T)
	for i, s := range tr.Steps {
		t.Logf("  %d. class=%q  box=%d×%d", i, s.Class, s.W, s.H)
		t.Logf("     style=%s", s.Style)
	}

	// ── THE FOUR CLAIMS, EACH READ OFF THE TRACE ───────────────────────────
	var seeded, landed *daycardMorphStep
	for i := range tr.Steps {
		s := &tr.Steps[i]
		if !strings.Contains(s.Class, "edmorph") {
			continue
		}
		// THE FIRST opacity:0 STEP IS THE SEED and the LAST opacity:1 step is
		// the landing. Taking the last of each would compare the end state
		// against itself, which is how a morph assertion passes vacuously.
		if seeded == nil && daycardStyleHas(s.Style, "opacity", "0") {
			seeded = s
		}
		if daycardStyleHas(s.Style, "opacity", "1") {
			landed = s
		}
	}
	if seeded == nil {
		t.Fatal("no step carries the carve-out class AND opacity:0 — the box was never " +
			"seeded at the card's geometry, so nothing morphed and the editor merely " +
			"appeared")
	}
	if landed == nil {
		t.Fatal("no step carries the carve-out class AND opacity:1 — the box never " +
			"landed, which under reduced motion would be the mid-morph geometry the " +
			"signature's second word forbids")
	}

	// 1. IT GROWS FROM THE CARD'S GEOMETRY. The seeded width is the CARD's, not
	//    the editor's — that is the whole of "one box becomes the other".
	if got := daycardStyleValue(seeded.Style, "inline-size"); got != fmt.Sprintf("%dpx", tr.CardRect.W) {
		t.Errorf("the seed sized the box at %s where the card measured %dpx — the morph "+
			"starts from a geometry that is not the card's, which is a reveal wearing "+
			"a morph's name", got, tr.CardRect.W)
	}
	// 2. IT TRAVELS BY `translate`, AND LANDS ON THE PLACEMENT LAW'S ANSWER.
	if got := daycardStyleValue(seeded.Style, "translate"); got == "" || got == "0px" {
		t.Errorf("the seed carries translate %q — the box starts where it ends, so it "+
			"does not travel and the two rects are the same one", got)
	}
	if got := daycardStyleValue(landed.Style, "translate"); got != "0px" && got != "0px 0px" {
		t.Errorf("the box landed at translate %q rather than at the placement law's own "+
			"answer — the morph re-decided where the editor goes", got)
	}
	// 3. IT DOES NOT SCALE. A form whose labels stretch during the open is the
	//    verdict this arc has already earned once.
	for _, s := range tr.Steps {
		for _, banned := range []string{"transform", "scale", "zoom"} {
			if daycardStyleValue(s.Style, banned) != "" {
				t.Errorf("the morph wrote %s — [ER-6] SIGNED refuses a scale, and this "+
					"surface is a FORM", banned)
			}
		}
	}
	// 4. IT ENDS AT THE SHEET'S OWN WIDTH, not at a number the module invented.
	want := daycardProbeShippedWidth(t)
	if got := daycardStyleValue(landed.Style, "inline-size"); got != fmt.Sprintf("%dpx", want) {
		t.Errorf("the box landed at %s where the sheet declares --de-w: %dpx — the morph "+
			"and the placement law disagree about how wide the editor is", got, want)
	}
}

// daycardStyleValue reads one declaration out of an inline style attribute.
func daycardStyleValue(style, prop string) string {
	for _, decl := range strings.Split(style, ";") {
		parts := strings.SplitN(decl, ":", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) != prop {
			continue
		}
		return strings.TrimSpace(parts[1])
	}
	return ""
}

func daycardStyleHas(style, prop, val string) bool {
	return daycardStyleValue(style, prop) == val
}

// daycardMorphTraceScript records every style-attribute write the module makes
// to the editor's root, through a MutationObserver — so the trace is what the
// SHIPPED module actually wrote in a real browser, not what a stub recorded.
const daycardMorphTraceScript = `
  var edBox = document.querySelector('[data-cal-dayeditor]');
  var cardBox = document.querySelector('[data-cal-daycard]');
  var steps = [];
  // THE OBSERVER IS ASYNC AND THE MODULE WRITES IN ONE TASK, so reading the
  // attribute inside the callback reads the FINAL value for every record — the
  // first cut of this file did exactly that and produced fourteen identical
  // rows. attributeOldValue is the fix: each record carries the value the
  // attribute held BEFORE that write, so replaying the oldValues in order
  // reconstructs the sequence the module actually wrote, and the current value
  // appended at the end closes it.
  var lastClass = edBox.getAttribute('class') || '';
  var lastStyle = edBox.getAttribute('style') || '';
  var obs = new MutationObserver(function (recs) {
    recs.forEach(function (r) {
      if (r.attributeName === 'class') lastClass = r.oldValue || '';
      if (r.attributeName === 'style') lastStyle = r.oldValue || '';
      steps.push({ class: lastClass, style: lastStyle, w: 0, h: 0 });
    });
  });
  obs.observe(edBox, {
    attributes: true, attributeOldValue: true, attributeFilter: ['style', 'class'],
  });

  var cell = document.querySelector('[data-bench-block] [data-day][data-day-ord]');
  if (cell) cell.click();
  // THE CARD'S RECT IS TAKEN WHILE IT IS STILL THE OPEN BOX — the same
  // measurement the module takes, read independently here so the assertion is a
  // check rather than an echo.
  var cr = cardBox.getBoundingClientRect();
  var door = document.querySelector('[data-dc-new]');
  if (door) door.click();

  setTimeout(function () {
    var pre = document.createElement('pre');
    pre.id = 'morphtrace';
    steps.push({
      class: edBox.getAttribute('class') || '', style: edBox.getAttribute('style') || '',
      w: Math.round(edBox.offsetWidth), h: Math.round(edBox.offsetHeight)
    });
    pre.textContent = JSON.stringify({
      cardRect: { l: Math.round(cr.left), t: Math.round(cr.top),
                  w: Math.round(cr.width), h: Math.round(cr.height) },
      steps: steps
    });
    document.body.appendChild(pre);
  }, 50);
`
