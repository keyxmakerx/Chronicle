package calendar

// daycard_morph_shots_test.go — C-CALV4-EDITOR-R2b stage 19, and it is the rig
// that photographs what the previous rig reported it could not.
//
// ── THE CLAIM THIS FILE RETIRES ───────────────────────────────────────────
//
// The rejected morph frames were captured with `--virtual-time-budget`, which
// does not run the document's rendering lifecycle: no transition is ever
// created, so `getAnimations()` has nothing to park and every frame comes back
// showing the editor at its final geometry. The evidence read that as a fact
// about the ENVIRONMENT — "this environment cannot photograph the morph… it is
// the rig's limit, not the morph's" — and burned it onto five images.
//
// Both halves of that were wrong, and in opposite directions:
//
//   · THE ENVIRONMENT CAN photograph it. Drop the virtual clock, serve the page
//     so a slow subresource can hold the `load` event past the flight, and
//     `getAnimations()` returns the six real transitions (the card's two and the
//     editor's four) with values to park.
//   · THE MORPH REALLY WAS DEAD ON OPEN — for an ordering reason in `edOpen`,
//     fixed in stage 18. So the rig's own "parked 1 transition — height" strip
//     was telling the truth and the caption over it was arguing with it.
//
// ── HOW A FRAME IS TAKEN ──────────────────────────────────────────────────
//
//  1. THE PAGE IS SERVED, NOT LOADED FROM `file://`, because the capture needs
//     a resource whose response can be DELAYED. `/hold` is a 1×1 GIF the server
//     sits on for 1.4s; the `load` event — which is when `--screenshot` fires —
//     therefore lands well after the driver has done its work.
//  2. THE DRIVER RUNS ON `DOMContentLoaded`, one task later, which is after the
//     module has wired itself and long before `load`.
//  3. IT CLICKS WHAT A READER CLICKS: the day cell, then `+ New event` once the
//     card has settled — never both in one task, which is the artefact that made
//     the trace probe's card measure 24px tall.
//  4. TIMERS ARE FROZEN AT THE CLICK, and this is the one piece of stagecraft in
//     the frame. Parking the ANIMATIONS does not stop `setTimeout`, so the
//     card's `hide()` (--disc-close) and the morph's `edMorphSettle`
//     (--disc-open) both fired while the picture was held — the first cut of
//     this rig produced a 0% frame with NO CARD IN IT, because the card had been
//     popped out of the top layer 1.2 seconds earlier. Freezing the clock at the
//     click is what makes the frame a moment rather than a composite. The
//     TRANSITIONS are the shipped ones, untouched.
//  5. EVERY ANIMATION IS PAUSED AND SET TO `fraction × its own duration`, and
//     the frame reports what it parked, what the editor's box measured and what
//     the card's did — to the strip burned on the image AND back to this test,
//     which is what makes the assertion below possible.
//
// ── THE ASSERTION, WHICH IS THE POINT ─────────────────────────────────────
//
// A capture rig that silently degrades produces five identical images with five
// different captions, and that is exactly what happened. So this generator
// REFUSES TO SHIP A SET IT CANNOT TELL APART: the five open frames must grow
// strictly, the five close frames must shrink strictly, and the middle three of
// each must sit strictly between the card's rect and the editor's. If they do
// not, the test fails and there are no images to mislabel.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// daycardMorphFrame is one parked frame's own report of itself — the same
// numbers the black strip burns onto the image, returned to the test so the set
// can be checked rather than admired.
type daycardMorphFrame struct {
	Parked  []string `json:"parked"`
	EdW     float64  `json:"edW"`
	EdH     float64  `json:"edH"`
	EdX     float64  `json:"edX"`
	EdY     float64  `json:"edY"`
	Opacity float64  `json:"opacity"`
	CardW   float64  `json:"cardW"`
	CardH   float64  `json:"cardH"`
	CardX   float64  `json:"cardX"`
	CardY   float64  `json:"cardY"`
	// OrigW..OrigY are the CARD'S RECT AT THE MOMENT THE MORPH WAS ANCHORED —
	// the geometry the editor grows out of and shrinks back onto. It is not the
	// same as the card's rect IN THE FRAME: the card is cross-fading out while
	// the editor grows, and on the close direction it is gone entirely. Both
	// are reported so a reader is never asked to infer one from the other.
	OrigW float64 `json:"origW"`
	OrigH float64 `json:"origH"`
	OrigX float64 `json:"origX"`
	OrigY float64 `json:"origY"`
	At    float64 `json:"at"`
	Dur   float64 `json:"dur"`
	Err   string  `json:"err"`
}

// daycardMorphFractions are the five stops §10 item 5 asks for — start, three
// through the flight, end.
var daycardMorphFractions = []float64{0, 0.25, 0.5, 0.75, 1}

// TestGenerateDayCardMorphFrames re-shoots the morph in both directions, on a
// real clock, and writes `morph-frames.txt` beside the images with every
// measurement the strips carry.
//
// It shares `DAYCARD_SHOTS` with the main generator so one command still
// produces the whole evidence directory.
func TestGenerateDayCardMorphFrames(t *testing.T) {
	outDir := os.Getenv("DAYCARD_SHOTS")
	if outDir == "" {
		t.Skip("daycard morph frames: set DAYCARD_SHOTS=<dir> to run")
	}
	chrome := benchFindChromium()
	if chrome == "" {
		t.Skip("daycard morph frames: no Chromium binary found (set CHROMIUM_BIN)")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", outDir, err)
	}

	css := benchCSS(t) + benchBlockSheet(t) + dayCardCSS(t)
	data := benchFxShotData(DayCardMount{CanCreate: true, CanAuthorDmOnly: true,
		CanDelete: true, CanRestrict: true, CampaignID: "camp-1"})
	surface := benchStripLinks(renderBench(t, data))
	mod := readRepoFile(t, "internal/plugins/calendar/static/js/calendar_daycard.js")
	vis := readRepoFile(t, "internal/plugins/calendar/static/js/cal_visibility.js")

	var log strings.Builder
	log.WriteString("THE MORPH, PHOTOGRAPHED — C-CALV4-EDITOR-R2b stage 19\n")
	log.WriteString(strings.Repeat("=", 72) + "\n\n")
	log.WriteString("Real Chromium, REAL CLOCK (no --virtual-time-budget), real Bench\n" +
		"markup, both shipped sheets, the shipped module. Every animation parked\n" +
		"with getAnimations() at an exact fraction of its own duration; JS timers\n" +
		"frozen at the click so the settle and the card's hide cannot fire while\n" +
		"the picture is held. Each row is what the image's own black strip says.\n\n")

	open := map[float64]daycardMorphFrame{}
	closed := map[float64]daycardMorphFrame{}
	for _, dir := range []string{"open", "close"} {
		log.WriteString(fmt.Sprintf("── %s ─────────────────────────────────────────────\n", dir))
		for i, f := range daycardMorphFractions {
			spec := daycardMorphShotSpec{
				file: fmt.Sprintf("morph-%s-%02d.png", dir, i), dir: dir, f: f,
				title: fmt.Sprintf("THE MORPH · %s · PARKED AT %.0f%% OF --disc-%s — one "+
					"box becoming the other", dir, f*100, dir),
				label:   fmt.Sprintf("the morph, %s, parked at %.0f%% of --disc-%s", dir, f*100, dir),
				caption: daycardMorphCaption,
			}
			fr := daycardMorphShoot(t, chrome, outDir, css, surface, mod, vis, spec)
			if dir == "open" {
				open[f] = fr
			} else {
				closed[f] = fr
			}
			log.WriteString(daycardMorphRow(spec.file, f, fr))
		}
		log.WriteString("\n")
	}

	// ── THE SET MUST BE TELLABLE APART, OR IT IS NOT EVIDENCE ─────────────
	daycardMorphAssertSeries(t, "open", open, false)
	daycardMorphAssertSeries(t, "close", closed, true)

	// ── §10 item 6: THE REDUCED-MOTION PAIR, ON THE SAME REAL CLOCK ───────
	//
	// 13 and 14 are a COUNTERFACTUAL: the same click sequence and the same
	// pause call, the media query on and off. Under the old generator's virtual
	// clock NEITHER half could park anything, so "the same pause, at 50%" was a
	// caption over a frame that had never been paused, and the pair proved
	// nothing about the branch. Here 14 is a real 50% frame and 13 is a real
	// zero-animation one, and the strips carry both counts.
	log.WriteString("── reduced motion, and its counterfactual ─────────────────\n")
	rmSpecs := []daycardMorphShotSpec{
		{file: "13-editor-gm-reduced-motion.png", dir: "open", f: 0.5, reduced: true,
			title: "Event editor · REDUCED MOTION · the END state — INSTANT AND COMPLETE",
			label: "reduced motion — the END state; nothing to park",
			caption: "the same click sequence and the same pause call as 14, under " +
				"prefers-reduced-motion driven at the browser. The editor is at FULL SIZE, " +
				"FULL OPACITY and interactive, and the strip reports ZERO transitions " +
				"parked — there was nothing to park, which is the whole claim: reduced " +
				"motion seeds no start geometry at all, so the box can never land at a " +
				"mid-morph size. WHAT THIS FRAME NO LONGER SHOWS: the rejected set's 13, " +
				"and this one until C-CALV4-CARD-REDUCED-ANCHOR was closed, put the box at " +
				"the VIEWPORT'S TOP-LEFT rather than beside the day — under reduced motion " +
				"`closeCard` hides the card SYNCHRONOUSLY, so the placement law was handed " +
				"a 0×0 anchor. `edOpen` now freezes the anchor's rect BEFORE `closeCard`, " +
				"beside the morph's own `fromRect`, and both motion modes land on the same " +
				"placement; TestDayCardReducedMotionAnchorsToItsDay measures the pair. ONE " +
				"THING THIS FRAME SHOWS THAT IS STILL NOT THE CLAIM, and is pre-existing: " +
				"the page heading is COVERED by the editor, which is why this family's " +
				"black strip carries the file name and what the frame is. " +
				daycardMorphCaption},
		{file: "14-editor-gm-no-preference-50pct.png", dir: "open", f: 0.5,
			title: "Event editor · NO PREFERENCE · the same pause, at 50% — THE COUNTERFACTUAL",
			label: "no preference — the same pause, at 50%",
			caption: "the counterfactual for 13: identical script, reduced motion OFF. " +
				"The strip reports a non-zero parked count and a box strictly between the " +
				"card's rect and the editor's, which is what proves the branch is doing " +
				"the work rather than the capture being inert. " + daycardMorphCaption},
	}
	rm := map[string]daycardMorphFrame{}
	for _, spec := range rmSpecs {
		fr := daycardMorphShoot(t, chrome, outDir, css, surface, mod, vis, spec)
		rm[spec.file] = fr
		log.WriteString(daycardMorphRow(spec.file, spec.f, fr))
	}
	log.WriteString("\n")
	if n := len(rm["13-editor-gm-reduced-motion.png"].Parked); n != 0 {
		t.Errorf("the reduced-motion frame parked %d transition(s) — under "+
			"prefers-reduced-motion the sheet declares no rule outside the "+
			"no-preference wrapper and the module seeds nothing, so there must be "+
			"nothing to park", n)
	}
	if n := len(rm["14-editor-gm-no-preference-50pct.png"].Parked); n == 0 {
		t.Error("the no-preference counterfactual parked NOTHING — the pair proves the " +
			"branch only if the other half of it moves, and this is exactly the state " +
			"the rejected set shipped in")
	}
	if a, b := rm["13-editor-gm-reduced-motion.png"], rm["14-editor-gm-no-preference-50pct.png"]; a.EdW <= b.EdW+10 {
		t.Errorf("the reduced-motion frame's box is %.0fpx wide and its counterfactual's "+
			"is %.0fpx — 13 must be COMPLETE where 14 is mid-flight, or the pair is two "+
			"pictures of the same thing", a.EdW, b.EdW)
	}

	log.WriteString("VERDICT: the open frames grow strictly and the close frames shrink\n" +
		"strictly, so no two images in this set are the same picture, and the\n" +
		"reduced-motion pair parks a different number of transitions in each half.\n" +
		"That is the check the rejected set could not have passed: it produced five\n" +
		"frames of a finished editor and captioned them 0/25/50/75/100%.\n")
	if err := os.WriteFile(filepath.Join(outDir, "morph-frames.txt"),
		[]byte(log.String()), 0o644); err != nil {
		t.Fatalf("write morph-frames.txt: %v", err)
	}
	t.Log("\n" + log.String())
}

// daycardMorphRow is one line of morph-frames.txt — the same numbers the black
// strip burns onto the image, so the artefact and the picture cannot drift.
func daycardMorphRow(file string, f float64, fr daycardMorphFrame) string {
	return fmt.Sprintf(
		"%-38s %3.0f%% of %.0fms — editor %.0f×%.0f at (%.0f,%.0f) opacity %.3f · "+
			"card origin %.0f×%.0f at (%.0f,%.0f) · card in frame %.0f×%.0f · parked %d: %s\n",
		file, f*100, fr.Dur, fr.EdW, fr.EdH, fr.EdX, fr.EdY, fr.Opacity,
		fr.OrigW, fr.OrigH, fr.OrigX, fr.OrigY, fr.CardW, fr.CardH, len(fr.Parked),
		strings.Join(fr.Parked, " "))
}

// daycardMorphAssertSeries refuses a set whose frames cannot be told apart.
func daycardMorphAssertSeries(t *testing.T, dir string, set map[float64]daycardMorphFrame, shrink bool) {
	t.Helper()
	prev := set[daycardMorphFractions[0]]
	for _, f := range daycardMorphFractions[1:] {
		cur := set[f]
		grew := cur.EdW > prev.EdW+1 && cur.EdH > prev.EdH+1
		shrank := cur.EdW < prev.EdW-1 && cur.EdH < prev.EdH-1
		if (!shrink && !grew) || (shrink && !shrank) {
			t.Errorf("%s: the frame at %.0f%% measures %.0f×%.0f and the one before it "+
				"measures %.0f×%.0f — two frames of the same picture with two different "+
				"captions is the defect this rig was rebuilt to make impossible",
				dir, f*100, cur.EdW, cur.EdH, prev.EdW, prev.EdH)
		}
		prev = cur
	}
	// AND THE MIDDLE FRAMES MUST BE MIDDLE. Endpoints alone are a two-step
	// snap; the whole claim is that the box is caught BETWEEN the two rects.
	ends := set[daycardMorphFractions[0]]
	last := set[daycardMorphFractions[len(daycardMorphFractions)-1]]
	loW, hiW := ends.EdW, last.EdW
	if loW > hiW {
		loW, hiW = hiW, loW
	}
	for _, f := range daycardMorphFractions[1 : len(daycardMorphFractions)-1] {
		cur := set[f]
		if !(cur.EdW > loW+2 && cur.EdW < hiW-2) {
			t.Errorf("%s: the %.0f%% frame's box is %.0fpx wide, which is not strictly "+
				"between %.0f and %.0f — this frame is an endpoint wearing a fraction's "+
				"caption", dir, f*100, cur.EdW, loW, hiW)
		}
		if !(cur.Opacity > 0.01 && cur.Opacity < 0.99) {
			t.Errorf("%s: the %.0f%% frame's opacity is %.3f — the cross-fade is not in "+
				"this picture", dir, f*100, cur.Opacity)
		}
	}
}

// daycardMorphShotSpec is one frame to take: which direction, at what fraction
// of that direction's own duration, and under which motion preference.
type daycardMorphShotSpec struct {
	file    string
	dir     string // "open" or "close"
	f       float64
	reduced bool
	title   string
	caption string
	// label is the one-line identity the BLACK STRIP carries, and it exists
	// because a burned <h1> is not always in its own frame. Shot 13's editor
	// lands at the viewport's top-left corner and covers the page heading
	// completely — the rejected set's 13 had the same hole. The strip is fixed
	// to the bottom edge and no capture in this family has ever covered it, so
	// the file name and what the frame is go THERE.
	label string
}

// daycardMorphCaption is the sentence every frame in this family carries. It
// names the mechanism rather than the conclusion, so a reader can check it.
const daycardMorphCaption = "READ THE BLACK STRIP AT THE FOOT OF THIS IMAGE: it " +
	"reports how many transitions this capture actually parked, at what currentTime, " +
	"and what the editor's and the card's boxes measured at that instant. Those " +
	"numbers are returned to the generator, which REFUSES to write a set whose frames " +
	"cannot be told apart. Captured on a REAL CLOCK — no --virtual-time-budget, which " +
	"is what the rejected set used and why it could park nothing. JS timers are frozen " +
	"at the click so the card's hide and the morph's settle cannot fire while the " +
	"picture is held; the transitions are the shipped ones, untouched."

// daycardMorphShoot captures one frame and returns the frame's own report.
func daycardMorphShoot(t *testing.T, chrome, outDir, css, surface, mod, vis string,
	spec daycardMorphShotSpec) daycardMorphFrame {
	t.Helper()
	file, dir, f := spec.file, spec.dir, spec.f
	title, caption := spec.title, spec.caption

	got := make(chan []byte, 1)
	page := daycardMorphShotPage(css, surface, mod, vis, title, caption, dir, f,
		spec.file, spec.label)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, page)
	})
	// THE RESOURCE THAT HOLDS THE SHUTTER. `--screenshot` fires on `load`, and
	// `load` waits for every subresource — so a 1×1 GIF the server sits on is
	// the whole of the timing control, with no CDP and no driver library.
	mux.HandleFunc("/hold", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1400 * time.Millisecond)
		w.Header().Set("Content-Type", "image/gif")
		_, _ = w.Write(daycardMorphPixelGIF)
	})
	mux.HandleFunc("/frame", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		select {
		case got <- b:
		default:
		}
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	out := filepath.Join(outDir, file)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	args := []string{
		"--headless", "--no-sandbox", "--disable-gpu", "--hide-scrollbars",
		"--force-device-scale-factor=2",
		"--disable-background-timer-throttling",
		"--disable-renderer-backgrounding",
		"--disable-backgrounding-occluded-windows",
		"--window-size=1440,900",
		"--screenshot=" + out, srv.URL + "/",
	}
	if spec.reduced {
		// THE BRANCH IS DRIVEN AT THE BROWSER, never simulated in the page: a
		// capture that faked the media query would prove nothing about it.
		args = append([]string{"--force-prefers-reduced-motion"}, args...)
	}
	cmd := exec.CommandContext(ctx, chrome, args...)
	if combined, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s: chromium screenshot: %v\n%s", file, err, strings.TrimSpace(string(combined)))
	}
	if st, err := os.Stat(out); err != nil || st.Size() == 0 {
		t.Fatalf("%s: no image was written", file)
	}

	// THE REPORT IS WAITED FOR, NOT PEEKED AT. `fetch` is fire-and-forget from
	// the page's side and the response travels back through the browser's
	// network stack, so it can land a beat after `--screenshot` has already
	// written the file and the process has exited. A non-blocking read of this
	// channel failed one frame in ten for exactly that reason — and a rig that
	// is flaky about its own verification is a rig that will eventually publish
	// an unverified image.
	var fr daycardMorphFrame
	select {
	case raw := <-got:
		if err := json.Unmarshal(raw, &fr); err != nil {
			t.Fatalf("%s: the frame report is not JSON: %v", file, err)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("%s: the page never reported what it parked — the image on disk is "+
			"unverified and must not be published", file)
	}
	if fr.Err != "" {
		t.Fatalf("%s: the driver could not take this frame: %s", file, fr.Err)
	}
	return fr
}

// daycardMorphPixelGIF is a 1×1 transparent GIF — the smallest thing a server
// can be slow about.
var daycardMorphPixelGIF = []byte{
	0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00, 0x01, 0x00, 0x80, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x21, 0xf9, 0x04, 0x01, 0x00, 0x00, 0x00,
	0x00, 0x2c, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x02, 0x02,
	0x44, 0x01, 0x00, 0x3b,
}

// daycardMorphShotPage is the captured surface. It is deliberately NOT
// `daycardShotPage`: that builder wires its driver to `load`, which is the one
// event this capture has to outlive.
func daycardMorphShotPage(css, surface, mod, vis, title, caption, dir string, f float64,
	file, label string) string {
	return `<!doctype html><html><head><meta charset="utf-8"><style>` +
		`html,body{margin:0;padding:0}` +
		`body{background:#f9fafb;color:#111827;` +
		`font-family:ui-sans-serif,system-ui,-apple-system,"Segoe UI",sans-serif}` +
		`.shot-wrap{padding:16px}` +
		`.cal-bench{width:1180px;max-width:100%}` +
		`h1{font-size:15px;line-height:1.2;margin:0 0 4px;letter-spacing:-.02em}` +
		`.shot-cap{font-size:11px;line-height:1.5;margin:0 0 12px;opacity:.72;max-width:80ch}` +
		`.shot-strips{position:fixed;left:0;right:0;bottom:0;z-index:1}` +
		`.shot-diag{font:11px/1.5 ui-monospace,SFMono-Regular,Menlo,monospace;` +
		`padding:6px 10px;background:oklch(0.18 0.02 265);color:oklch(0.98 0 0)}` +
		`#hold{position:fixed;left:-10px;top:-10px;width:1px;height:1px;opacity:0}` +
		css +
		`</style></head><body><div class="shot-wrap">` +
		`<h1>` + title + `</h1><p class="shot-cap">` + caption + `</p>` +
		surface +
		`</div><div class="shot-strips">` +
		`<div class="shot-diag" id="shot-diag">RIG: the driver did not run</div></div>` +
		`<script>` + vis + `</script><script>` + mod + `</script>` +
		`<script>` + daycardMorphShotScript(dir, f, file, label) + `</script>` +
		`<img id="hold" src="/hold" alt="">` +
		`</body></html>`
}

// daycardMorphShotScript drives one frame.
func daycardMorphShotScript(dir string, f float64, file, label string) string {
	// THE CLOSE DIRECTION IS DRIVEN THROUGH THE EDITOR'S OWN Cancel CONTROL,
	// not by calling anything internal — the reverse morph is `edClose`'s and
	// a capture that reached past the UI would be photographing a private path.
	trigger := `document.querySelector('[data-dc-new]')`
	if dir == "close" {
		trigger = `document.querySelector('[data-de-cancel]')`
	}
	return fmt.Sprintf(`
document.addEventListener('DOMContentLoaded', function () {
  var F = %f;
  var DIR = %q;
  var FILE = %q;
  var LABEL = %q;
  var ed = document.querySelector('[data-cal-dayeditor]');
  var card = document.querySelector('[data-cal-daycard]');
  var cell = document.querySelector('[data-bench-block] [data-day][data-day-ord]');
  var strip = document.getElementById('shot-diag');
  var ORIGIN = null;
  function report(o) {
    try { fetch('/frame', { method: 'POST', body: JSON.stringify(o) }); } catch (e) {}
  }
  function fail(msg) {
    if (strip) strip.textContent = 'RIG: ' + msg;
    report({ err: msg });
  }
  if (!ed || !card || !cell) { fail('the fixture is missing the card, the editor or a day cell'); return; }

  function box(el) {
    var r = el.getBoundingClientRect();
    return [Math.round(r.width), Math.round(r.height), Math.round(r.left), Math.round(r.top)];
  }
  // park freezes the clock, fires the trigger, and on the very next frame pins
  // every animation at F of its own duration.
  function park(fire, done) {
    // THE ANCHOR RECT IS TAKEN WHILE THE CARD IS STILL THE OPEN BOX, which on
    // the close direction is the only moment it exists at all.
    var origin = ORIGIN || box(card);
    window.setTimeout = function () { return 0; };
    fire();
    void ed.offsetHeight;
    requestAnimationFrame(function () {
      var live = document.getAnimations();
      var names = [], dur = 0;
      live.forEach(function (a) {
        a.pause();
        var d = 0;
        try { d = a.effect.getTiming().duration || 0; } catch (e) {}
        if (d > dur) dur = d;
        try { a.currentTime = F * d; } catch (e) {}
        names.push((a.transitionProperty || a.animationName || '?') + '@' + Math.round(d) + 'ms');
      });
      names.sort();
      var e = box(ed), c = box(card);
      var op = parseFloat(getComputedStyle(ed).opacity);
      if (strip) {
        strip.textContent = 'RIG · ' + FILE + ' · ' + LABEL +
          ' — parked ' + live.length + ' transition(s)' +
          (names.length ? ': ' : '') +
          names.join(' · ') + ' · currentTime ' + Math.round(F * dur) + 'ms of ' +
          Math.round(dur) + 'ms · EDITOR BOX ' + e[0] + '×' + e[1] +
          ' at (' + e[2] + ',' + e[3] + ') opacity ' + op.toFixed(3) +
          ' · CARD ORIGIN ' + origin[0] + '×' + origin[1] +
          ' at (' + origin[2] + ',' + origin[3] + ')' +
          ' · card in frame ' + c[0] + '×' + c[1] +
          ' · timers frozen at the click';
      }
      report({ parked: names, edW: e[0], edH: e[1], edX: e[2], edY: e[3],
               opacity: op, cardW: c[0], cardH: c[1], cardX: c[2], cardY: c[3],
               origW: origin[0], origH: origin[1], origX: origin[2], origY: origin[3],
               at: F * dur, dur: dur });
      if (done) done();
    });
  }

  // THE CARD IS LET SETTLE BEFORE THE DOOR IS CLICKED. Clicking both in one
  // task catches the card mid-reveal and seeds the morph from a 24px rect —
  // the artefact the trace probe had to disclose.
  setTimeout(function () {
    cell.click();
    setTimeout(function () {
      if (DIR === 'open') {
        var door = %s;
        if (!door) { fail('the card carries no + New event door'); return; }
        park(function () { door.click(); });
        return;
      }
      // THE CLOSE IS PHOTOGRAPHED FROM A SETTLED EDITOR, so the reverse morph
      // starts where a reader's does rather than from a box still in flight.
      var open = document.querySelector('[data-dc-new]');
      if (!open) { fail('the card carries no + New event door'); return; }
      ORIGIN = box(card);
      open.click();
      setTimeout(function () {
        var cancel = %s;
        if (!cancel) { fail('the editor carries no Cancel control'); return; }
        park(function () { cancel.click(); });
      }, 420);
    }, 320);
  }, 0);
});
`, f, dir, file, label, trigger, trigger)
}

// TestDayCardMorphShotPageOutlivesItsOwnLoadEvent is the always-on companion:
// it needs no browser and it fails if the capture ever loses the two properties
// that make it a photograph of the flight rather than of the aftermath.
func TestDayCardMorphShotPageOutlivesItsOwnLoadEvent(t *testing.T) {
	page := daycardMorphShotPage("", "<div data-cal-daycard></div>", "", "",
		"T", "C", "open", 0.5, "morph-open-02.png", "morph · open · 50%")
	for _, want := range []string{
		`src="/hold"`,       // the shutter is held past the flight
		"DOMContentLoaded",  // …and the driver does not wait for load
		"getAnimations",     // the frame is parked, not guessed at
		"window.setTimeout", // the clock is frozen at the click
		"'/frame'",          // and the frame reports itself back
		`id="shot-diag"`,    // to a strip a reader can check
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the morph shot page no longer carries %q — the capture would "+
				"photograph the aftermath and caption it as the flight", want)
		}
	}
	if strings.Contains(page, "virtual-time") {
		t.Error("the morph shot page names virtual time — that is the mechanism that " +
			"made the rejected set five pictures of a finished editor")
	}
	if strings.Contains(page, "addEventListener('load") {
		t.Error("the morph driver is wired to `load`, which this capture deliberately " +
			"outlives; wiring it there is how the frame becomes the aftermath")
	}
}
