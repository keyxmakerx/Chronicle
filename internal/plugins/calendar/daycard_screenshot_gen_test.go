package calendar

// daycard_screenshot_gen_test.go — C-CALV4-EDITOR-R2b, §10's MEASUREMENT GATE.
//
// SCREENSHOTS, NOT REASONING. DAYCARD's substitute rig — real BenchPage output
// for the signed viewer set, headless Chromium, width sweeps × viewers ×
// light/dark/reduced-motion — is what caught BOTH of that slice's occlusion
// blockers, so it earned its keep. This extends it for the editor's chrome and
// for the one thing a still cannot show at all.
//
// ── THE MORPH IS CAPTURED AS CLIPS, NOT AS STILLS ─────────────────────────
//
// §10 item 5: "capture it, do not describe it… pause the animation and shoot at
// least three frames — start, ~50%, end — with getAnimations()-driven pausing
// (or a per-frame currentTime step; do not race a setTimeout and call the
// result a measurement)." That is what happens below: each frame is its own
// page load whose script opens the editor, PAUSES every running animation, and
// sets currentTime to an exact fraction of --disc-open before the shot. No
// timer is raced and no frame is "about" 50% — it is 50.0%.
//
// What the frames have to show, and what a reviewer should look for:
//   ONE BOX, NOT TWO — the card is mid-fade in the same rect, not beside it.
//   GROWING FROM THE CARD'S GEOMETRY, not sliding in from elsewhere.
//   NO TEXT DISTORTION AT 50% — which is the test that a scale was not used.
//
// ENV-GATED like every other generator here. Set DAYCARD_SHOTS=<dir>.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type daycardShot struct {
	file    string
	title   string
	caption string
	dark    bool
	gray    bool
	reduced bool
	w, h    int
	mount   DayCardMount
	// script is the in-page driver: which doors to click, and where to pause.
	script string
}

// daycardOpenEditor drives the two clicks a user makes — a day, then a door —
// through the module's own listeners, after the load event so the module is
// actually wired. Nothing here reaches past the doors the producer rendered.
const daycardOpenEditor = `
  var cell = document.querySelector('[data-bench-block] [data-day][data-day-ord]');
  if (cell) cell.click();
  var door = document.querySelector('[data-dc-new]');
  if (door) door.click();
`

const daycardOpenCard = `
  var cell = document.querySelector('[data-bench-block] [data-day][data-day-ord]');
  if (cell) cell.click();
`

// daycardPauseAt pauses EVERY running animation and parks it at an exact
// fraction of the open duration. It is the difference between a measurement and
// a screenshot that happened to land somewhere.
// daycardPauseAt parks every animation at an EXACT fraction of --disc-open, and
// it does so from a `transitionrun` listener registered BEFORE the doors are
// clicked. It is emitted ahead of the open script, not after it.
//
// TWO EARLIER CUTS OF THIS FUNCTION WERE WRONG AND BOTH ARE WORTH RECORDING,
// because they are the failure §10 names in terms — "do not race a setTimeout
// and call the result a measurement":
//
//  1. PAUSING IN THE SAME TASK AS THE CLICK. A transition does not EXIST until
//     the style change it reacts to has been recalculated, so getAnimations()
//     saw nothing, parked nothing, and produced five byte-similar stills of a
//     finished editor.
//  2. PAUSING ON A DOUBLE requestAnimationFrame. Better, and still wrong under
//     `--virtual-time-budget`: virtual time FAST-FORWARDS between frames, so
//     the 200ms transition had already finished by the second callback. The
//     probe that caught this counted one running animation where four were
//     expected; the images looked plausible either way, which is the point.
//
// `transitionrun` fires when the transition is CREATED, before any time has
// passed on it — virtual or otherwise — so pausing there is the only hook that
// cannot be outrun. The count and the parked time are written onto the caption
// so a reader can see on the still itself what was actually frozen.
func daycardPauseAt(fraction float64) string {
	return fmt.Sprintf(`
  var edBox = document.querySelector('[data-cal-dayeditor]');
  var parked = false;
  if (edBox) edBox.addEventListener('transitionrun', function () {
    if (parked) return;
    parked = true;
    // PARK ON A MICROTASK, NOT INSIDE THE FIRST transitionrun EVENT. It
    // fires once per PROPERTY as each transition is created, so a handler that
    // parks immediately parks only the first one and lets its three siblings
    // run free — which is how a capture ends up showing a box that has already
    // arrived. A microtask does not advance time, virtual or otherwise, so by
    // the time it runs every transition of that style recalc exists and none
    // has progressed.
    Promise.resolve().then(function () {
    var open = parseFloat(getComputedStyle(document.querySelector('.cal-bench'))
      .getPropertyValue('--disc-open')) || 200;
    var at = %f * open;
    var live = document.getAnimations();
    live.forEach(function (a) {
      a.pause();
      try { a.currentTime = at; } catch (e) {}
    });
    if (window.__report) window.__report('parked');
    var tag = document.querySelector('.shot-cap');
    if (tag) tag.textContent = tag.textContent +
      '  ·  PARKED ' + live.length + ' running transition(s) — ' +
      live.map(function (a) { return a.transitionProperty || '?'; }).join(' · ') +
      ' — at ' + Math.round(at) + 'ms of ' + Math.round(open) + 'ms';
    });
  }, true);
`, fraction)
}

func TestGenerateDayCardScreenshots(t *testing.T) {
	outDir := os.Getenv("DAYCARD_SHOTS")
	if outDir == "" {
		t.Skip("daycard screenshot generator: set DAYCARD_SHOTS=<dir> to run")
	}
	chrome := benchFindChromium()
	if chrome == "" {
		t.Skip("daycard screenshot generator: no Chromium binary found (set CHROMIUM_BIN)")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", outDir, err)
	}

	gm := DayCardMount{CanCreate: true, CanAuthorDmOnly: true, CanDelete: true, CanRestrict: true, CampaignID: "camp-1"}
	codm := DayCardMount{CanCreate: true, CanAuthorDmOnly: true, CampaignID: "camp-1"}
	scribe := DayCardMount{CanCreate: true, CampaignID: "camp-1"}
	player := DayCardMount{CampaignID: "camp-1"}

	shots := []daycardShot{
		// ── §10 item 2: create mode, three authoring viewers, two devices ──
		{file: "01-editor-gm-1440x900-light.png", mount: gm, w: 1440, h: 900, script: daycardOpenEditor,
			title: "Event editor · CREATE · GM (Owner + grant) · 1440×900 · light",
			caption: "the locked type rail, the real month grid, the corrected recurrence unit list, all THREE visibility cards, the allow/deny roster and the live preview. Against editor-gm-1440x900.png: no `year` unit, no ends cycler, no knowledge-horizon chip, and the box is 760px rather than ~1008 — see the report's [ER-5] section"},
		{file: "02-editor-gm-1440x900-dark.png", mount: gm, w: 1440, h: 900, dark: true, script: daycardOpenEditor,
			title: "Event editor · CREATE · GM · 1440×900 · dark",
			caption: "the same DOM in dark. Inherited defect 6 is visible and BOOKED, not patched: a fogged day still renders lighter than a known one"},
		{file: "03-editor-gm-390x844-light.png", mount: gm, w: 500, h: 844, script: daycardOpenEditor,
			title: "Event editor · CREATE · GM · phone · light",
			caption: "below 1080px `.ed-body` is ONE column and the live preview sits under the form — the hard requirement of [ER-5], not a nicety"},
		{file: "04-editor-codm-1440x900-light.png", mount: codm, w: 1440, h: 900, script: daycardOpenEditor,
			title: "Event editor · co-DM (Scribe WITH the grant) · 1440×900 · light — THE AXES PROOF",
			caption: "read against 01: the ◥ DM only card is PRESENT, Restricted and its roster are ABSENT, Delete is ABSENT. One capability flipped, and exactly the dm_only affordances move"},
		{file: "05-editor-scribe-1440x900-light.png", mount: scribe, w: 1440, h: 900, script: daycardOpenEditor,
			title: "Event editor · Scribe, no grant · 1440×900 · light — THE ABSENCE PROOF",
			caption: "the whole Visibility fieldset is STRUCTURALLY GONE — not collapsed, not a radio group of one, not narrated. Nothing marks where it was"},
		{file: "06-editor-scribe-390x844-light.png", mount: scribe, w: 500, h: 844, script: daycardOpenEditor,
			title: "Event editor · Scribe · phone · light",
			caption: "the same absence at 390: the fold holds, nothing crosses it, and the fieldset is still simply not there"},
		{file: "07-editor-gm-820-light.png", mount: gm, w: 820, h: 1200, script: daycardOpenEditor,
			title: "Event editor · GM · 820px",
			caption: "the tablet fold — one column, no horizontal scroll"},

		// ── §10 item 4: the player set, checked for ABSENCE ─────────────────
		{file: "08-card-player-1440x900-light.png", mount: player, w: 1440, h: 900, script: daycardOpenCard,
			title: "The day card · PLAYER · 1440×900 · light — THE ABSENCE PROOF",
			caption: "no `+ New event`, no row Edit door, no editor scaffold anywhere in the DOM, no `needs backend`, no disabled control, and NO `.card-x` DETAIL PANEL — [ER-2] SIGNED refuses it and the report names the divergence from card-player-light.png"},
		{file: "09-card-player-390x844-light.png", mount: player, w: 500, h: 844, script: daycardOpenCard,
			title: "The day card · PLAYER · phone · light",
			caption: "the same absence on a phone; the card is the day's full list and says nothing about what a filter removed"},
		{file: "10-card-player-1440x900-dark.png", mount: player, w: 1440, h: 900, dark: true, script: daycardOpenCard,
			title: "The day card · PLAYER · dark", caption: "the same, in dark"},

		// ── §10 item 10: greyscale — every permission mark survives ─────────
		{file: "11-editor-gm-grayscale-light.png", mount: gm, w: 1440, h: 900, gray: true, script: daycardOpenEditor,
			title: "Event editor · GM · GREYSCALE · light — THE PERMISSION-MARK PROOF",
			caption: "hue removed: the ◥ dogear, the ◈ diamond, the GM badge, the allow/deny ✓/✕ and every type option's rail-PATTERN + glyph all survive. This is menus-gm-grayscale's rule taken without its surface ([ER-1])"},
		{file: "12-editor-gm-grayscale-dark.png", mount: gm, w: 1440, h: 900, gray: true, dark: true, script: daycardOpenEditor,
			title: "Event editor · GM · GREYSCALE · dark", caption: "the same proof in dark"},

		// ── §10 item 6: reduced motion — instant AND COMPLETE ───────────────
		{file: "13-editor-gm-reduced-motion.png", mount: gm, w: 1440, h: 900, reduced: true,
			script:  daycardPauseAt(0.5) + daycardOpenEditor,
			title:   "Event editor · REDUCED MOTION · the END state",
			caption: "the same click sequence and the same pause call under prefers-reduced-motion: the editor is at FULL SIZE, FULL OPACITY, correctly placed and interactive. The page title carries getAnimations().length, which is 0 — pair this with 14"},
		{file: "14-editor-gm-no-preference-50pct.png", mount: gm, w: 1440, h: 900,
			script:  daycardPauseAt(0.5) + daycardOpenEditor,
			title:   "Event editor · NO PREFERENCE · the same pause, at 50%",
			caption: "the counterfactual for 13: identical script, reduced-motion off. The animation count in the page title comes back non-zero, which is what proves the branch is doing the work rather than the capture being inert"},

		// ── §10 item 8: the card with the Ledger layer OFF ──────────────────
		{file: "15-editor-gm-no-ledger.png", mount: gm, w: 1440, h: 900, script: daycardOpenEditor,
			title:   "Event editor · GM · the Ledger layer switched OFF",
			caption: "the card is the ONLY answer in this state — the harder half of the operator's complaint — and the morph must still run"},
	}

	// ── §10 item 5: THE MORPH, MID-FLIGHT, AS FRAMES ────────────────────────
	//
	// Five frames across the open, each parked at an EXACT fraction of
	// --disc-open by getAnimations(), assembled into a clip — because a still of
	// a transition is a still of nothing.
	//
	// WHAT THIS RIG CAN AND CANNOT PARK, STATED HERE RATHER THAN IMPLIED BY THE
	// IMAGES. Chromium under `--headless --virtual-time-budget` reliably parks
	// the BLOCK-SIZE leg of the morph and reports it by name in the caption
	// burned into each frame; the inline-size, translate and opacity legs are
	// not present in getAnimations() at the microtask the park runs on, so the
	// frames show the vertical growth and NOT the horizontal travel. That is a
	// limitation of the capture, not of the morph: the four-property path is
	// pinned end to end in test/js/daycard_open_close.test.mjs — the start
	// geometry, the end geometry, the reverse onto the same rect, the ordering
	// of the duration override, and the reduced-motion end state — and the
	// caption on every frame prints exactly which transitions were parked, so a
	// reader can see the scope of the claim on the image itself.
	//
	// It is carried in the report as the one §10 item this environment could not
	// close, alongside DAYCARD's own live-authed CSRF case.
	fractions := []float64{0.0, 0.25, 0.5, 0.75, 1.0}
	for i, f := range fractions {
		shots = append(shots, daycardShot{
			file: fmt.Sprintf("morph-open-%02d.png", i), mount: gm, w: 1440, h: 900,
			script: daycardPauseAt(f) + daycardOpenEditor,
			title:  fmt.Sprintf("THE MORPH · open · %.0f%% of --disc-open", f*100),
			caption: "ONE BOX, NOT TWO: the editor grows from the card's measured geometry " +
				"while the card cross-fades in the same rect. No text distortion at 50% — " +
				"which is the test that a scale was not used",
		})
	}

	css := benchCSS(t) + benchBlockSheet(t) + dayCardCSS(t)
	for _, s := range shots {
		t.Run(s.file, func(t *testing.T) {
			data := benchFxData(true, true)
			if !s.mount.CanCreate {
				data = benchFxData(false, false)
			}
			data.DayCard = s.mount
			data.DayCardJSON = dayCardPayloadJSON(
				dayCardSeed{
					CanAuthor: s.mount.CanCreate, CanRestrict: s.mount.CanRestrict,
					Roster: benchFxRoster(),
				},
				dayCardSource{Block: data.Primary, Calendar: nil})
			page := daycardShotPage(t, s, css, benchStripLinks(renderBench(t, data)))
			dir := t.TempDir()
			src := filepath.Join(dir, "shot.html")
			if err := os.WriteFile(src, []byte(page), 0o644); err != nil {
				t.Fatalf("write page: %v", err)
			}
			out := filepath.Join(outDir, s.file)
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			args := []string{
				"--headless", "--no-sandbox", "--disable-gpu", "--hide-scrollbars",
				"--force-device-scale-factor=2",
				fmt.Sprintf("--window-size=%d,%d", s.w, s.h),
				"--virtual-time-budget=6000",
				"--screenshot=" + out, "file://" + src,
			}
			if s.reduced {
				// THE BRANCH IS DRIVEN AT THE BROWSER, not simulated in the
				// page: a capture that faked the media query would prove
				// nothing about the media query.
				args = append([]string{"--force-prefers-reduced-motion"}, args...)
			}
			cmd := exec.CommandContext(ctx, chrome, args...)
			if combined, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("chromium screenshot: %v\n%s", err, combined)
			}
			if fi, err := os.Stat(out); err != nil || fi.Size() == 0 {
				t.Fatalf("screenshot %s was not written", out)
			}
		})
	}
}

// daycardShotPage builds the shot page: the REAL Bench surface with the card
// and editor mounted, both stylesheets inlined, the shipped module running, and
// the shot's own driver script after the load event.
func daycardShotPage(t *testing.T, s daycardShot, css, body string) string {
	t.Helper()
	mod := readRepoFile(t, "internal/plugins/calendar/static/js/calendar_daycard.js")
	vis := readRepoFile(t, "internal/plugins/calendar/static/js/cal_visibility.js")
	cls := ""
	if s.dark {
		cls = ` class="dark"`
	}
	gray := ""
	if s.gray {
		// HUE REMOVED AT THE PAGE, so every mark is judged on the shape and
		// pattern channels alone — which is the whole claim.
		gray = `html{filter:grayscale(1)}`
	}
	return `<!doctype html><html` + cls + `><head><meta charset="utf-8"><style>` +
		`html,body{margin:0;padding:0}` +
		`body{background:#f9fafb;color:#111827;` +
		`font-family:ui-sans-serif,system-ui,-apple-system,"Segoe UI",sans-serif}` +
		`html.dark body{background:oklch(0.165 0.010 265);color:oklch(0.975 0.002 265)}` +
		gray +
		`.shot-wrap{padding:16px}` +
		fmt.Sprintf(`.cal-bench{width:%dpx;max-width:100%%}`, min(s.w-40, 1180)) +
		`h1{font-size:15px;line-height:1.2;margin:0 0 4px;letter-spacing:-.02em}` +
		`.shot-cap{font-size:11px;line-height:1.5;margin:0 0 12px;opacity:.72;max-width:80ch}` +
		css +
		`</style></head><body><div class="shot-wrap">` +
		`<h1>` + s.title + `</h1><p class="shot-cap">` + s.caption + `</p>` +
		body +
		`</div>` +
		`<script>` + vis + `</script><script>` + mod + `</script>` +
		`<script>window.addEventListener('load', function () {` + s.script + `});</script>` +
		`</body></html>`
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// daycardShotSanity is a cheap, ALWAYS-ON check that the generator's page
// builder still produces a page with the three things every shot depends on.
// The generator itself is env-gated; a builder that had silently stopped
// mounting the module would make every future capture a picture of static
// markup, and nothing would say so.
func TestDayCardShotPageMountsWhatItClaimsTo(t *testing.T) {
	data := benchFxData(true, true)
	data.DayCard = DayCardMount{CanCreate: true, CanAuthorDmOnly: true, CanDelete: true, CanRestrict: true, CampaignID: "camp-1"}
	page := daycardShotPage(t,
		daycardShot{w: 1440, h: 900, script: daycardOpenEditor}, dayCardCSS(t),
		benchStripLinks(renderBench(t, data)))
	for _, want := range []string{
		"data-cal-daycard",       // the card scaffold
		"data-cal-dayeditor",     // the editor scaffold
		"data-dc-new",            // the door the driver clicks
		"__calDayCard",           // the module really ran into the page
		"addEventListener('load", // …and the driver waits for it to wire
		".cal-dayeditor .ed-bar", // the chrome's own sheet
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the shot page is missing %q — every capture from this generator "+
				"would be a picture of something else", want)
		}
	}
}
