package calendar

// daycard_morph_interpolation_test.go — C-CALV4-EDITOR-R2b stage 18, and it is
// the assertion whose absence let a dead morph ship green through three rounds.
//
// ── WHY A FOURTH MORPH GUARD, WHEN THERE ARE ALREADY THREE ────────────────
//
// Every morph guard this arc had asserts the STATE MACHINE and none of them
// asserts the RENDERED RESULT:
//
//   · test/js/daycard_open_close.test.mjs runs in a layout-less DOM stub and
//     checks classes and END-STATE inline styles. Its own comment concedes
//     that by the time it reads, the module has already written the end state.
//   · daycard_morph_trace_test.go is a MutationObserver over the style
//     attribute and says in its header, in these words, that it "DOES NOT
//     PROVE that the compositor interpolated".
//   · TestBenchCSS_NoMotionAtAll and the `.edmorph` monopoly guard read the
//     SHEET. A sheet declaring four perfect transitions is still a sheet.
//
// All three stay green with the morph COMPLETELY INERT, and that is exactly
// what shipped: the editor popped in at full size and animated only on the way
// out, because the seeded start geometry and the class that declares the
// transition arrived in the SAME style recalc. Per CSS Transitions it is the
// AFTER-change style that starts a transition, so that one recalc started
// transitions running AWAY from the resting box toward the seed, the seed never
// became the settled before-change style, and the final write that followed saw
// no change and started nothing. See edOpen's header for the fix.
//
// ── WHAT THIS PROBE DOES, AND WHY IT NEEDS A REAL TIMELINE ────────────────
//
// It drives the SHIPPED module over the REAL Bench markup with the REAL sheets
// in REAL Chromium, opens the editor the way a reader does (day cell, then
// `+ New event`), and samples `getBoundingClientRect` + `getComputedStyle` on
// the editor's root EVERY ANIMATION FRAME. Then it asserts the three things a
// morph means and a snap does not:
//
//   1. THE FIRST FRAME IS NOT THE LAST ONE. The box starts near the CARD's
//      geometry, not the editor's resting geometry. (Pre-fix this is the
//      assertion that goes red: every sample read the RESTING box at opacity
//      1, from the first frame onward — 760×589 in this fixture.)
//   2. IT PASSES THROUGH THE MIDDLE. At least one frame lands STRICTLY between
//      the two rects on both axes, and the run holds three or more distinct
//      widths — a two-step snap has exactly two.
//   3. IT LANDS ON THE PLACEMENT LAW'S ANSWER, at translate 0 and opacity 1.
//
// …and then it does the same for the CLOSE, which already worked and must not
// regress in the course of fixing the open.
//
// NO `--virtual-time-budget`, AND THAT IS THE WHOLE REASON THIS FILE EXISTS
// SEPARATELY FROM THE OTHER PROBES. Under virtual time the document's rendering
// lifecycle is not run: transitions are never created, `getComputedStyle`
// answers stale values after a forced reflow, and the rig honestly reports that
// it cannot photograph a morph. That limit was read as the morph's limit; it is
// the FLAG's. With a real clock the same page interpolates visibly, so the
// results come back over HTTP from the page itself (a `fetch` POST to the test
// server that served it) rather than out of `--dump-dom`, which would have to
// fire before there is anything to see.
//
// NO NEW DEPENDENCY. `httptest` + the page's own `fetch` replaces CDP entirely;
// there is no WebSocket client, no driver library and no browser download here.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"sort"
	"strings"
	"testing"
	"time"
)

// daycardMorphSample is ONE ANIMATION FRAME of the editor's root, as the
// compositor actually had it — the rect it painted and the opacity it painted
// at, not the inline style that asked for them.
type daycardMorphSample struct {
	T  float64 `json:"t"`  // ms since the door was clicked
	X  float64 `json:"x"`  // getBoundingClientRect().left
	Y  float64 `json:"y"`  // getBoundingClientRect().top
	W  float64 `json:"w"`  // …width
	H  float64 `json:"h"`  // …height
	Op float64 `json:"op"` // getComputedStyle().opacity
	Tr string  `json:"tr"` // …translate
}

// daycardMorphFilm is the whole run: the card's rect (the morph's start), the
// editor's resting rect (its end), and the two directions sampled frame by
// frame.
type daycardMorphFilm struct {
	Card  daycardMorphSample   `json:"card"`
	Rest  daycardMorphSample   `json:"rest"`
	Open  []daycardMorphSample `json:"open"`
	Close []daycardMorphSample `json:"close"`
	Err   string               `json:"err"`
}

// TestDayCardMorphInterpolates is the missing assertion: the signed morph must
// actually MOVE, in a browser, in both directions.
//
// IT IS NOT ENV-GATED. The heavy probes in this family sweep 61 cells × 11
// viewports and are gated for their runtime; this one opens the editor twice
// and finishes in about two seconds, and a guard that only runs when someone
// remembers to set a variable is the guard that was missing in the first place.
// It skips — loudly — only when the machine has no Chromium to drive.
func TestDayCardMorphInterpolates(t *testing.T) {
	chrome := benchFindChromium()
	if chrome == "" {
		t.Skip("morph interpolation probe: no Chromium binary found (set CHROMIUM_BIN)")
	}

	data := benchFxData(true, true)
	data.DayCard = DayCardMount{CanCreate: true, CanAuthorDmOnly: true, CanDelete: true,
		CanRestrict: true, CampaignID: "camp-1"}
	cal := benchFxTypedCalendar()
	data.DayCardJSON = dayCardPayloadJSON(
		dayCardSeed{CanAuthor: true, CanRestrict: true, Roster: benchFxRoster()},
		dayCardSource{Block: data.Primary, Calendar: &cal})
	page := daycardMorphFilmPage(t, data)

	film, err := daycardMorphRun(t, chrome, page)
	if err != nil {
		t.Fatalf("morph interpolation probe: %v", err)
	}
	if film.Err != "" {
		t.Fatalf("the page could not drive the morph: %s", film.Err)
	}

	t.Logf("── the morph, sampled every animation frame in real Chromium ────")
	t.Logf("  the card rested at %.0f×%.0f (%.0f,%.0f)",
		film.Card.W, film.Card.H, film.Card.X, film.Card.Y)
	t.Logf("  the editor rests at %.0f×%.0f (%.0f,%.0f)",
		film.Rest.W, film.Rest.H, film.Rest.X, film.Rest.Y)
	daycardMorphLog(t, "open", film.Open)
	daycardMorphLog(t, "close", film.Close)

	daycardMorphAssert(t, "OPEN", film.Open, film.Card, film.Rest, false)
	daycardMorphAssert(t, "CLOSE", film.Close, film.Card, film.Rest, true)
}

// daycardMorphAssert is the one set of claims, run in both directions. `reverse`
// swaps which rect the run must START at; everything else about a morph is
// symmetric, and writing the assertions twice is how the close half of a pair
// quietly stops being checked.
func daycardMorphAssert(t *testing.T, dir string, run []daycardMorphSample,
	card, rest daycardMorphSample, reverse bool) {
	t.Helper()

	// THE FRAMES AFTER THE BOX IS GONE ARE NOT FRAMES OF THE BOX. `edHide` pops
	// the editor out of the top layer when --disc-close elapses, and every
	// sample after that measures a 0×0 rect at the origin. Scoring those as
	// "where the close landed" would make the reverse half assert that the
	// morph ends nowhere, so the tail is trimmed and the last SAMPLED FLIGHT
	// frame is what the landing claim reads.
	for len(run) > 0 && run[len(run)-1].W <= 0 && run[len(run)-1].H <= 0 {
		run = run[:len(run)-1]
	}

	// A run with a handful of frames is a run that did not sample the flight;
	// 200ms at 60Hz is a dozen. Under 5 the probe itself is the thing at fault
	// and it must say so rather than pass on three identical frames.
	if len(run) < 5 {
		t.Fatalf("%s: only %d frames were sampled — the probe did not observe the "+
			"flight, so nothing below is a measurement", dir, len(run))
	}
	first, last := run[0], run[len(run)-1]
	from, to := card, rest
	if reverse {
		from, to = rest, card
	}

	// ── 1. THE FIRST FRAME IS NOT THE LAST ONE ────────────────────────────
	//
	// THIS IS THE ASSERTION THE DEAD OPEN MORPH FAILS. A box that pops in sits
	// at its resting geometry from the very first frame; a box that morphs
	// starts at the OTHER rect. The margin is half the distance between the two
	// rects, so it cannot be satisfied by a rounding wobble and does not
	// hard-code either geometry.
	spanW, spanH := math.Abs(rest.W-card.W), math.Abs(rest.H-card.H)
	if spanW < 40 || spanH < 40 {
		t.Fatalf("%s: the card (%.0f×%.0f) and the editor (%.0f×%.0f) are nearly the "+
			"same size, so this fixture cannot tell a morph from a snap — the "+
			"fixture, not the module", dir, card.W, card.H, rest.W, rest.H)
	}
	if math.Abs(first.W-from.W) > spanW/2 {
		t.Errorf("%s: the first sampled frame (t=%.1fms) was already %.0fpx wide, where "+
			"the morph must START at %.0fpx — the box did not travel, it appeared. "+
			"That is a reveal wearing a morph's name.", dir, first.T, first.W, from.W)
	}
	if math.Abs(first.H-from.H) > spanH/2 {
		t.Errorf("%s: the first sampled frame (t=%.1fms) was already %.0fpx tall, where "+
			"the morph must START at %.0fpx", dir, first.T, first.H, from.H)
	}
	// IT TRAVELS, TOO. The card and the placed editor are hundreds of pixels
	// apart vertically in this fixture; a morph that resized in place would
	// satisfy the size claims and still be the wrong animation.
	if math.Abs(rest.Y-card.Y) > 40 && math.Abs(first.Y-from.Y) > math.Abs(rest.Y-card.Y)/2 {
		t.Errorf("%s: the first sampled frame sat at y=%.0f where the morph must start "+
			"at y=%.0f — the box resized without moving", dir, first.Y, from.Y)
	}

	// ── 2. IT PASSES THROUGH THE MIDDLE ───────────────────────────────────
	//
	// Start and end alone are also what a TWO-STEP SNAP looks like: seed, then
	// jump. Interpolation means frames that are neither. Both axes, because a
	// height-only transition (which is exactly what the dead morph had — one
	// no-op `height` CSSTransition) would otherwise read as a pass.
	lo, hi := math.Min(card.W, rest.W), math.Max(card.W, rest.W)
	loH, hiH := math.Min(card.H, rest.H), math.Max(card.H, rest.H)
	midW, midH, midOp := 0, 0, 0
	widths := map[int]bool{}
	for _, s := range run {
		widths[int(math.Round(s.W))] = true
		if s.W > lo+20 && s.W < hi-20 {
			midW++
		}
		if s.H > loH+20 && s.H < hiH-20 {
			midH++
		}
		if s.Op > 0.05 && s.Op < 0.95 {
			midOp++
		}
	}
	if midW == 0 {
		t.Errorf("%s: not one of %d frames had a width strictly between the card's "+
			"%.0fpx and the editor's %.0fpx — the box changed size in a single step, "+
			"which is a snap", dir, len(run), card.W, rest.W)
	}
	if midH == 0 {
		t.Errorf("%s: not one of %d frames had a height strictly between %.0fpx and "+
			"%.0fpx", dir, len(run), card.H, rest.H)
	}
	if midOp == 0 {
		t.Errorf("%s: opacity never held an intermediate value across %d frames — the "+
			"box did not cross-fade, it was switched on", dir, len(run))
	}
	if len(widths) < 3 {
		t.Errorf("%s: the whole run holds only %d distinct widths (%v) — two is a "+
			"seed and a jump, not an interpolation", dir, len(widths), daycardMorphKeys(widths))
	}

	// ── 3. IT LANDS WHERE THE PLACEMENT LAW PUT IT ────────────────────────
	//
	// A morph that interpolates to the wrong rect is worse than none: the
	// stale-geometry reopen this arc already paid for was exactly a box
	// rendering somewhere its own occlusion report did not know about.
	//
	// THE TWO DIRECTIONS ARE NOT HELD TO THE SAME TOLERANCE, AND THE REASON IS
	// THE MECHANISM, NOT A LOOSER STANDARD FOR THE CLOSE. The open SETTLES: the
	// box is still there 200ms later and its rect is the placement law's answer
	// to the pixel. The close is TORN DOWN at --disc-close, so the last frame
	// this probe can sample is whatever frame the compositor produced just
	// before `edHide` fired — one frame short of the card's rect is a scheduling
	// fact, not a defect. So the close is asked to ARRIVE (much nearer the card
	// than the editor) rather than to land exactly.
	tolW, tolH := 2.0, 2.0
	if reverse {
		tolW, tolH = spanW/4, spanH/4
	}
	if math.Abs(last.W-to.W) > tolW || math.Abs(last.H-to.H) > tolH {
		t.Errorf("%s: the run ended at %.0f×%.0f where it must land at %.0f×%.0f "+
			"(tolerance %.0f×%.0f)", dir, last.W, last.H, to.W, to.H, tolW, tolH)
	}
	if !reverse {
		if last.Op < 0.99 {
			t.Errorf("OPEN: the box settled at opacity %.3f — it never finished arriving", last.Op)
		}
		if tr := strings.TrimSpace(last.Tr); tr != "" && tr != "none" && !strings.HasPrefix(tr, "0px") {
			t.Errorf("OPEN: the box settled at translate %q rather than on the placement "+
				"law's own answer", tr)
		}
	}
}

func daycardMorphKeys(m map[int]bool) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}

// daycardMorphLog prints a readable strip of the flight so a failing run can be
// read without re-running the probe. It thins to at most a dozen rows: the
// point of the log is the SHAPE of the curve.
func daycardMorphLog(t *testing.T, dir string, run []daycardMorphSample) {
	t.Helper()
	t.Logf("  %s — %d frames", dir, len(run))
	step := 1
	if len(run) > 12 {
		step = len(run) / 12
	}
	for i := 0; i < len(run); i += step {
		s := run[i]
		t.Logf("    t=%6.1fms  %6.1f×%-6.1f at (%6.1f,%6.1f)  opacity %.3f  translate %s",
			s.T, s.W, s.H, s.X, s.Y, s.Op, s.Tr)
	}
	if last := run[len(run)-1]; step > 1 {
		t.Logf("    t=%6.1fms  %6.1f×%-6.1f at (%6.1f,%6.1f)  opacity %.3f  translate %s",
			last.T, last.W, last.H, last.X, last.Y, last.Op, last.Tr)
	}
}

// daycardMorphRun serves the page, drives Chromium at it with a REAL clock, and
// waits for the page to post its own film back.
//
// THE PAGE IS SERVED, NOT WRITTEN TO A FILE, for one reason: `file://` pages
// cannot POST anywhere, and without `--virtual-time-budget` there is no
// `--dump-dom` moment to read a `<pre>` out of. One `httptest` server both
// serves the page and collects the result, which also means the browser is
// killed the moment the answer arrives rather than on a timeout.
func daycardMorphRun(t *testing.T, chrome, page string) (*daycardMorphFilm, error) {
	t.Helper()
	got := make(chan []byte, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, page)
	})
	mux.HandleFunc("/film", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(io.LimitReader(r.Body, 4<<20))
		select {
		case got <- b:
		default:
		}
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	profile := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, chrome,
		"--headless", "--no-sandbox", "--disable-gpu", "--hide-scrollbars",
		"--no-first-run", "--no-default-browser-check", "--disable-extensions",
		// THE THROTTLES COME OFF DELIBERATELY. A headless window is
		// "occluded" and "backgrounded" by every heuristic Chromium has, and a
		// throttled rAF loop would sample four frames of a 200ms flight and
		// report a snap that is really a sampling artefact.
		"--disable-background-timer-throttling",
		"--disable-renderer-backgrounding",
		"--disable-backgrounding-occluded-windows",
		"--window-size=1440,900",
		"--user-data-dir="+profile,
		srv.URL+"/",
	)
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start chromium: %w", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	select {
	case raw := <-got:
		var film daycardMorphFilm
		if err := json.Unmarshal(raw, &film); err != nil {
			return nil, fmt.Errorf("film payload is not JSON: %w\n%s", err, truncateFilm(raw))
		}
		return &film, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("the page never posted a film — chromium ran for 90s " +
			"without driving the morph")
	}
}

func truncateFilm(b []byte) string {
	if len(b) > 2000 {
		return string(b[:2000]) + "…"
	}
	return string(b)
}

// daycardMorphFilmPage builds the surface the flight is measured on: the REAL
// Bench markup, BOTH shipped sheets, the shipped module, and a driver that
// clicks what a reader clicks.
func daycardMorphFilmPage(t *testing.T, data BenchData) string {
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
		`<script>` + vis + `</script><script>` + mod + `</script>` +
		`<script>` + daycardMorphFilmScript + `</script>` +
		`</body></html>`
}

// daycardMorphFilmScript is the driver, and every line of it is chosen so that
// what it samples is what a reader would have seen.
//
// IT WAITS FOR `load`, like every other probe in this family: the module wires
// itself while the parser is still running, so a driver that clicked at parse
// time would click into a page with no listeners.
//
// IT SAMPLES SYNCHRONOUSLY FIRST, THEN EVERY FRAME. The synchronous read is
// taken in the same task as the click, which is where a dead morph is caught
// red-handed — with the transitions started correctly, `getComputedStyle` there
// returns the animation's t=0 value (the seed); with them started against the
// resting box it returns the resting box, and no later frame ever leaves it.
const daycardMorphFilmScript = `
window.addEventListener('load', function () {
  var film = { card: null, rest: null, open: [], close: [], err: '' };
  function post() {
    try {
      fetch('/film', { method: 'POST', body: JSON.stringify(film) });
    } catch (e) { /* the test's own timeout is the backstop */ }
  }
  function fail(msg) { film.err = msg; post(); }

  var ed = document.querySelector('[data-cal-dayeditor]');
  var card = document.querySelector('[data-cal-daycard]');
  var cell = document.querySelector('[data-bench-block] [data-day][data-day-ord]');
  if (!ed || !card || !cell) { fail('the fixture is missing the card, the editor or a day cell'); return; }

  function snap(list, t0) {
    var r = ed.getBoundingClientRect();
    var cs = getComputedStyle(ed);
    list.push({
      t: Math.round((performance.now() - t0) * 10) / 10,
      x: Math.round(r.left * 10) / 10, y: Math.round(r.top * 10) / 10,
      w: Math.round(r.width * 10) / 10, h: Math.round(r.height * 10) / 10,
      op: parseFloat(cs.opacity), tr: cs.translate || ''
    });
  }
  // roll samples every animation frame for ms. THE FIRST SAMPLE IS TAKEN
  // SYNCHRONOUSLY, in the caller's own task, before the browser has had a
  // chance to paint anything — that is where a dead morph is caught red-handed.
  function roll(list, ms, done) {
    var t0 = performance.now();
    (function tick() {
      snap(list, t0);
      if (performance.now() - t0 < ms) { requestAnimationFrame(tick); } else { done(); }
    })();
  }
  function box(el) {
    var r = el.getBoundingClientRect();
    return { t: 0, x: Math.round(r.left * 10) / 10, y: Math.round(r.top * 10) / 10,
             w: Math.round(r.width * 10) / 10, h: Math.round(r.height * 10) / 10,
             op: 1, tr: '' };
  }

  // 1. OPEN THE CARD and let its own disclosure settle, so the rect the morph
  //    starts from is the rect a reader is looking at when they click.
  cell.click();
  setTimeout(function () {
    film.card = box(card);
    if (!(film.card.w > 0) || !(film.card.h > 0)) {
      fail('the card never opened, so there is no start geometry'); return;
    }
    var door = document.querySelector('[data-dc-new]');
    if (!door) { fail('the card carries no [data-dc-new] door'); return; }

    // 2. THE OPEN FLIGHT. --disc-open is 200ms; 420ms of frames covers the
    //    flight and the settle that follows it, so the last frame is the
    //    resting box rather than a guess about one.
    door.click();
    roll(film.open, 420, function () {
      film.rest = box(ed);
      // 3. THE CLOSE FLIGHT, which already worked and is sampled so that
      //    fixing the open cannot quietly break it. --disc-close is 160ms.
      var cancel = document.querySelector('[data-de-cancel]');
      if (!cancel) { fail('the editor carries no [data-de-cancel] control'); return; }
      cancel.click();
      roll(film.close, 300, post);
    });
  }, 320);
});
`

// TestDayCardMorphFilmPageMountsWhatItDrives is the always-on companion to the
// probe above: it needs no browser and it fails the moment the driver is
// pointed at a control the fixture does not carry. A probe that clicks nothing
// skips its way to green.
func TestDayCardMorphFilmPageMountsWhatItDrives(t *testing.T) {
	data := benchFxData(true, true)
	data.DayCard = DayCardMount{CanCreate: true, CanAuthorDmOnly: true, CanDelete: true,
		CanRestrict: true, CampaignID: "camp-1"}
	page := daycardMorphFilmPage(t, data)
	for _, want := range []string{
		"data-cal-daycard", "data-cal-dayeditor", "data-dc-new", "data-de-cancel",
		"__calDayCard", "requestAnimationFrame", "'/film'",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the morph film page is missing %q — the probe would drive nothing "+
				"and report a green flight it never sampled", want)
		}
	}
	if strings.Contains(page, "virtual-time-budget") {
		t.Error("the morph film page mentions virtual time — the whole reason this probe " +
			"exists is that virtual time never runs the rendering lifecycle")
	}
}
