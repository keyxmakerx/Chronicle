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
	// scroll is a selector whose element is scrolled to its END before the
	// shot. It exists because of the fix round's finding 1 — see
	// daycardFoldDiag's header. Empty means "photograph the resting scroll
	// position", which is what every shot in the rejected set did.
	scroll string
}

// ── THE SCROLL FOLD, AND WHY EVERY EDITOR SHOT NOW REPORTS ITS OWN ────────
//
// `.cal-dayeditor .ed-body` is `max-block-size: min(70vh, 620px); overflow-y:
// auto` (calendar-daycard.css). At 1440×900 in the TWO-COLUMN layout the left
// column is ~437px wide and `.vis` is `repeat(auto-fit, minmax(150px, 1fr))`,
// so the ◈ Restricted card wraps to a second row that falls INSIDE the
// overflow — and with it the allow/deny roster and the whole `Tied to` field.
//
// THE REJECTED SET PHOTOGRAPHED THAT FOLD AND ITS CAPTIONS DID NOT KNOW.
// Shots 01, 04, 11 and 12 showed exactly two visibility cards — `Public` and
// `◥ DM only` — and nothing else before the footer, while their captions and
// the index read "all THREE visibility cards, the allow/deny roster". The
// BUILD was right the whole time: the Go render carries `data-de-restricted`,
// `data-de-aud` and `data-de-tie`, the payload carries `"members"`, and the
// 820px one-column shot photographs all three cards plus `TIED TO`. The
// EVIDENCE was wrong. A caption is not evidence, and a caption that names a
// mark outside its own frame is worse than no caption.
//
// Three things close it, and all three are mechanical rather than editorial:
//
//  1. EVERY editor shot burns its OWN fold report onto the image — how many
//     pixels of `.ed-body` are visible, how many exist, how many are below the
//     fold, and whether this capture scrolled. A reader can no longer be
//     wrong about what is in the frame, because the frame says.
//  2. Every desktop shot that needs the lower band has a LOWER-BAND COMPANION
//     (`…-lowerband.png`) captured with `.ed-body` scrolled to its end, and
//     the pair is what the index cites.
//  3. TestDayCardShotCaptionsStayInsideTheirOwnFrame is always on and refuses
//     a two-column, un-scrolled shot whose caption names a below-fold mark.
//     Finding 1 cannot recur silently.
//
// The fold is NOT "fixed" by the rig. `max-block-size` is the shipped
// geometry — a capture that overrode it would be photographing a box that no
// user has. The rig scrolls, exactly as a user would.
const daycardFoldDiag = `
  var fold = document.getElementById('shot-fold');
  var edb = document.querySelector('.cal-dayeditor .ed-body');
  if (fold) {
    if (!edb) { fold.textContent = 'RIG: no .ed-body on this page (card-only shot)'; }
    else {
      var vis = Math.round(edb.clientHeight), all = Math.round(edb.scrollHeight);
      var below = Math.max(0, all - vis - Math.round(edb.scrollTop));
      fold.textContent = 'RIG: .ed-body shows ' + vis + 'px of ' + all + 'px' +
        ' · scrollTop ' + Math.round(edb.scrollTop) + 'px' +
        ' · ' + below + 'px BELOW THE FOLD in this frame' +
        ' · marks present in the DOM: ' +
        (document.querySelector('[data-de-restricted]') ? '◈ Restricted ' : '') +
        (document.querySelector('[data-de-audrows]') ? '· allow/deny roster ' : '') +
        (document.querySelector('[data-de-tie]') ? '· Tied to' : '');
    }
  }
`

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
//
//  3. PARKING ONLY ONCE, ON THE FIRST transitionrun — fixed at stage 9. The
//     handler used to set a `parked` latch and return on every later event, so
//     any transition Chromium created in a LATER style update ran free to
//     completion while its siblings sat paused. That is exactly what the
//     morph frames showed: the box grew vertically at a fixed left edge and a
//     fixed width, because `height` was parked and `width` / `translate` /
//     `opacity` had already arrived. The frames were honest about being
//     frames; they were not honest about being the morph. Parking is now
//     IDEMPOTENT and runs on EVERY transitionrun — pausing an already-paused
//     animation and re-writing the same currentTime is a no-op, and the
//     union of parked property names is printed on the caption so the scope
//     of the claim is on the image itself.
func daycardPauseAt(fraction float64) string {
	return fmt.Sprintf(`
  var edBox = document.querySelector('[data-cal-dayeditor]');
  var parkedProps = {};
  if (edBox) edBox.addEventListener('transitionrun', function () {
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
      parkedProps[a.transitionProperty || '?'] = true;
    });
    if (window.__report) window.__report('parked');
    // THE CAPTION IS REWRITTEN, NOT APPENDED TO, because this handler runs on
    // EVERY transitionrun rather than only the first — see the header. It
    // prints the UNION of everything parked so far, so the last write is the
    // complete list and the reader is not looking at a partial one.
    var names = Object.keys(parkedProps).sort();
    // THE DIAGNOSTIC GOES IN THE FIXED BOTTOM STRIP, NOT THE FLOWED CAPTION.
    // The editor is a top-layer popover placed at the top of the viewport and
    // it covers the caption — which is how the previous cut's park report ended
    // up written somewhere no reader could see it.
    var tag = document.getElementById('shot-diag');
    if (tag) {
      var r = edBox.getBoundingClientRect();
      tag.textContent = 'RIG: parked ' + names.length + ' transition(s)' +
        (names.length ? ' — ' + names.join(' · ') : '') +
        ' · currentTime ' + Math.round(at) + 'ms of --disc-open ' + Math.round(open) + 'ms' +
        ' · EDITOR BOX ' + Math.round(r.width) + '×' + Math.round(r.height) +
        ' at (' + Math.round(r.left) + ',' + Math.round(r.top) + ')';
    }
    });
  }, true);
`, fraction)
}

// daycardEdBody is the scroll container the fold lives on. It is named once so
// the rig, the diag and the caption guard cannot drift apart.
const daycardEdBody = ".cal-dayeditor .ed-body"

// daycardScrollTo scrolls a container to its END, exactly as a user's wheel
// would, and does not touch the geometry that put it there.
func daycardScrollTo(sel string) string {
	if sel == "" {
		return ""
	}
	return `
  var sc = document.querySelector('` + sel + `');
  if (sc) sc.scrollTop = sc.scrollHeight;
`
}

// daycardShotList is the whole capture set, extracted from the generator so the
// ALWAYS-ON caption guard can read it. A shot list that only exists inside an
// env-gated test body is a list nothing in CI can check.
func daycardShotList() []daycardShot {
	gm := DayCardMount{CanCreate: true, CanAuthorDmOnly: true, CanDelete: true, CanRestrict: true, CampaignID: "camp-1"}
	codm := DayCardMount{CanCreate: true, CanAuthorDmOnly: true, CampaignID: "camp-1"}
	scribe := DayCardMount{CanCreate: true, CampaignID: "camp-1"}
	player := DayCardMount{CampaignID: "camp-1"}

	shots := []daycardShot{
		// ── §10 item 2: create mode, three authoring viewers, two devices ──
		//
		// EVERY CAPTION HERE NAMES ONLY WHAT IS IN ITS OWN FRAME. At 1440 the
		// two-column body folds — see daycardFoldDiag — so the desktop shots
		// come in PAIRS and the lower band is its own image. The fold report is
		// burned onto both halves.
		{file: "01-editor-gm-1440x900-light.png", mount: gm, w: 1440, h: 900, script: daycardOpenEditor,
			title: "Event editor · CREATE · GM (Owner + grant) · 1440×900 · light — UPPER BAND",
			caption: "the locked type rail, the real month grid, the corrected recurrence unit list, the live preview, and the FIRST ROW of the visibility cards. `.ed-body` scrolls (see the fold strip at the foot): the ◈ Restricted card, the allow/deny roster and `Tied to` are below this frame and are photographed in 01b. Against editor-gm-1440x900.png: no `year` unit, no ends cycler, no knowledge-horizon chip, and the box is 760px rather than ~1008 — see the report's [ER-5] section"},
		{file: "01b-editor-gm-1440x900-light-lowerband.png", mount: gm, w: 1440, h: 900,
			script: daycardOpenEditor, scroll: daycardEdBody,
			title:   "Event editor · CREATE · GM · 1440×900 · light — LOWER BAND (`.ed-body` scrolled to its end)",
			caption: "the SAME capture with `.ed-body` scrolled exactly as a wheel would scroll it — no geometry is overridden. This is where the ◈ Restricted card, the allow/deny roster (5 members, ✓/✕ per row) and the `Tied to` field live at 1440. 01 and 01b together are the desktop GM editor"},
		{file: "02-editor-gm-1440x900-dark.png", mount: gm, w: 1440, h: 900, dark: true, script: daycardOpenEditor,
			title: "Event editor · CREATE · GM · 1440×900 · dark — UPPER BAND",
			caption: "the same DOM in dark, same fold. Inherited defect 6 is visible and BOOKED, not patched: a fogged day still renders lighter than a known one"},
		{file: "02b-editor-gm-1440x900-dark-lowerband.png", mount: gm, w: 1440, h: 900, dark: true,
			script: daycardOpenEditor, scroll: daycardEdBody,
			title:   "Event editor · CREATE · GM · 1440×900 · dark — LOWER BAND",
			caption: "the dark half of the pair: ◈ Restricted, the roster and `Tied to`"},
		{file: "03-editor-gm-390x844-light.png", mount: gm, w: 390, h: 844, script: daycardOpenEditor,
			title: "Event editor · CREATE · GM · 390×844 · light",
			caption: "the REAL 390 viewport, not a 500px stand-in — the rejected set shot three files named `-390x844` at a 500px window and only one of them said so. Below 1080px `.ed-body` is ONE column and the live preview sits under the form ([ER-5]'s hard requirement). The fold strip reports what is below this frame"},
		{file: "03b-editor-gm-390x844-light-lowerband.png", mount: gm, w: 390, h: 844,
			script: daycardOpenEditor, scroll: daycardEdBody,
			title:   "Event editor · CREATE · GM · 390×844 · light — LOWER BAND",
			caption: "the phone's lower band: the one-column body scrolled to its end"},
		{file: "04-editor-codm-1440x900-light.png", mount: codm, w: 1440, h: 900, script: daycardOpenEditor,
			title: "Event editor · co-DM (Scribe WITH the grant) · 1440×900 · light — THE AXES PROOF, UPPER BAND",
			caption: "read against 01 (upper) — the ◥ DM only card is PRESENT. The absences this pair proves are in 04b, which is the frame the ◈ Restricted card and its roster would occupy if the capability were there. Delete is absent from BOTH 01 and 04 because create mode has no id to delete; the Delete axis is proved in the EDIT-mode pair 16/17, not here"},
		{file: "04b-editor-codm-1440x900-light-lowerband.png", mount: codm, w: 1440, h: 900,
			script: daycardOpenEditor, scroll: daycardEdBody,
			title:   "Event editor · co-DM · 1440×900 · light — THE AXES PROOF, LOWER BAND",
			caption: "read against 01b, which is the same scroll position for an Owner. There: ◈ Restricted, the roster, `Tied to`. Here: `Tied to` and NO ◈ Restricted card and NO roster. One capability flipped and exactly the CanRestrict affordances moved"},
		{file: "05-editor-scribe-1440x900-light.png", mount: scribe, w: 1440, h: 900, script: daycardOpenEditor,
			title: "Event editor · Scribe, no grant · 1440×900 · light — THE ABSENCE PROOF",
			caption: "the whole Visibility fieldset is STRUCTURALLY GONE — not collapsed, not a radio group of one, not narrated. Nothing marks where it was. With the fieldset gone the body is short enough that the fold strip reports 0px below the frame"},
		{file: "05b-editor-scribe-1440x900-dark.png", mount: scribe, w: 1440, h: 900, dark: true,
			script: daycardOpenEditor,
			title:  "Event editor · Scribe, no grant · 1440×900 · dark — THE ABSENCE PROOF",
			caption: "the same absence in dark. The rejected set had the Scribe in light only and the co-DM at one viewport in one theme; §10 item 2's matrix is filled here and in 04c/06b"},
		{file: "04c-editor-codm-1440x900-dark.png", mount: codm, w: 1440, h: 900, dark: true,
			script: daycardOpenEditor,
			title:  "Event editor · co-DM · 1440×900 · dark",
			caption: "the co-DM in dark — the theme leg §10 item 2 names and the rejected set did not have"},
		{file: "06-editor-scribe-390x844-light.png", mount: scribe, w: 390, h: 844, script: daycardOpenEditor,
			title: "Event editor · Scribe · 390×844 · light",
			caption: "the same absence at a REAL 390: the fieldset is still simply not there. Whether anything crosses the horizontal fold is NOT read off this image — it is MEASURED, at 390 and 820, in both themes, by TestDayCardFoldProbe (DAYCARD_FLOORS=1); see fold-and-floor-probe.txt"},
		{file: "06b-editor-codm-390x844-light.png", mount: codm, w: 390, h: 844, script: daycardOpenEditor,
			title:   "Event editor · co-DM · 390×844 · light",
			caption: "the co-DM at the phone width — §10 item 2's last matrix cell"},
		{file: "07-editor-gm-820-light.png", mount: gm, w: 820, h: 1200, script: daycardOpenEditor,
			title: "Event editor · GM · 820px",
			caption: "the tablet width: ONE column, and at 1200px tall the body does not fold — so this single frame carries all three visibility cards, the allow/deny roster AND `Tied to`, which is the un-scrolled proof that the build renders what 01b shows. Horizontal scroll is measured, not eyeballed — see fold-and-floor-probe.txt"},

		// ── §10 item 4: the player set, checked for ABSENCE ─────────────────
		{file: "08-card-player-1440x900-light.png", mount: player, w: 1440, h: 900, script: daycardOpenCard,
			title: "The day card · PLAYER · 1440×900 · light — THE ABSENCE PROOF",
			caption: "no `+ New event`, no row Edit door, no editor scaffold anywhere in the DOM, no `needs backend`, no disabled control, and NO `.card-x` DETAIL PANEL — [ER-2] SIGNED refuses it and the report names the divergence from card-player-light.png"},
		{file: "09-card-player-390x844-light.png", mount: player, w: 390, h: 844, script: daycardOpenCard,
			title: "The day card · PLAYER · phone · light",
			caption: "the same absence on a phone; the card is the day's full list and says nothing about what a filter removed"},
		{file: "10-card-player-1440x900-dark.png", mount: player, w: 1440, h: 900, dark: true, script: daycardOpenCard,
			title: "The day card · PLAYER · dark", caption: "the same, in dark"},

		// ── §10 item 10: greyscale — every permission mark survives ─────────
		//
		// THE GREYSCALE PROOF IS A PAIR PER THEME, and that is finding 1's
		// second consequence: §10 item 10 names FIVE marks and two of them —
		// the ◈ diamond and the allow/deny ✓/✕ — live below the fold at 1440.
		// The rejected set's caption listed all five over a frame containing
		// three. The upper frame now claims the three it has; the lower frame
		// claims the two it has.
		{file: "11-editor-gm-grayscale-light.png", mount: gm, w: 1440, h: 900, gray: true, script: daycardOpenEditor,
			title: "Event editor · GM · GREYSCALE · light — PERMISSION MARKS, UPPER BAND",
			caption: "hue removed AT THE TOP LAYER TOO — the card and the editor are [popover] and an ancestor filter on <html> does not reach them, which is why an earlier cut of this pair photographed a grey page with a full-colour editor on it. What must survive IN THIS FRAME: the ◥ dogear on the DM-only card, the GM badge, and every type option's rail-PATTERN + glyph. The other two marks §10 item 10 names — the ◈ diamond and the allow/deny ✓/✕ — are below the fold and are proved in 11b"},
		{file: "11b-editor-gm-grayscale-light-lowerband.png", mount: gm, w: 1440, h: 900, gray: true,
			script: daycardOpenEditor, scroll: daycardEdBody,
			title:   "Event editor · GM · GREYSCALE · light — PERMISSION MARKS, LOWER BAND",
			caption: "the other half of §10 item 10 with hue removed: the ◈ diamond on the Restricted card and the allow/deny ✓/✕ on every roster row. This is menus-gm-grayscale's rule taken without its surface ([ER-1])"},
		{file: "12-editor-gm-grayscale-dark.png", mount: gm, w: 1440, h: 900, gray: true, dark: true, script: daycardOpenEditor,
			title:   "Event editor · GM · GREYSCALE · dark — UPPER BAND",
			caption: "the same three marks in dark: ◥ dogear, GM badge, rail-pattern + glyph"},
		{file: "12b-editor-gm-grayscale-dark-lowerband.png", mount: gm, w: 1440, h: 900, gray: true, dark: true,
			script: daycardOpenEditor, scroll: daycardEdBody,
			title:   "Event editor · GM · GREYSCALE · dark — LOWER BAND",
			caption: "the ◈ diamond and the allow/deny ✓/✕ in dark"},

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

	// ── §10 item 5: THE MORPH, MID-FLIGHT — AND WHAT THIS RIG CANNOT DO ─────
	//
	// STATED FLATLY, BECAUSE THE PREVIOUS CUT OF THIS BLOCK CLAIMED MORE THAN
	// THE IMAGES SHOWED. §10 item 5 asks for start / ~50% / end with
	// getAnimations()-driven pausing. The rig does exactly that. The frames come
	// back showing a FINISHED EDITOR at every fraction, and the reason is
	// MEASURED and is the rig's:
	//
	//   · the park hook (`transitionrun`, idempotent, on every event) reports
	//     PARKED 0 transitions at 0% and, non-deterministically, 1 (`height`) at
	//     other fractions; the editor's box measures its FINAL geometry in every
	//     frame, and the strip burned onto each image says so in those words;
	//   · under `--dump-dom --virtual-time-budget`, getComputedStyle returns
	//     STALE values after a forced `offsetHeight` — writing `inline-size:
	//     340px` to the root and reading it back yields `760px`, across a
	//     requestAnimationFrame as well. The document's rendering lifecycle is
	//     not being run, so there is nothing to park.
	//
	// A rig that cannot create the transition cannot photograph it, and five
	// identical images captioned "25%" would be worse than no images. So THE
	// FRAMES ARE KEPT AND RELABELLED as what they are — the attempt, with the
	// rig's own report on them — and the claim is made where it CAN be made:
	//
	//   · daycard_morph_trace_test.go (DAYCARD_MORPH_TRACE=1) traces the four
	//     properties through a MutationObserver in REAL Chromium and proves the
	//     box is seeded at the card's measured rect, offset by `translate`, at
	//     opacity 0, with the carve-out class on — and lands at translate 0, the
	//     sheet's own --de-w and opacity 1, with no transform / scale / zoom
	//     anywhere in the sequence.
	//   · test/js/daycard_open_close.test.mjs pins the state machine: the start
	//     geometry, the end geometry, the reverse onto the same rect, the
	//     ordering of the duration override, and the reduced-motion end state.
	//
	// WHAT REMAINS UNPROVEN HERE is that the compositor interpolated between the
	// two geometries. That is carried in the report as the one §10 item this
	// environment could not close, alongside DAYCARD's own live-authed CSRF
	// case, and it is a live-client check rather than a claim.
	fractions := []float64{0.0, 0.25, 0.5, 0.75, 1.0}
	for i, f := range fractions {
		shots = append(shots, daycardShot{
			file: fmt.Sprintf("morph-open-%02d.png", i), mount: gm, w: 1440, h: 900,
			script: daycardPauseAt(f) + daycardOpenEditor,
			title: fmt.Sprintf("THE MORPH · open · park attempted at %.0f%% of --disc-open "+
				"— THE RIG COULD NOT CREATE THE TRANSITION", f*100),
			caption: "READ THE BLACK STRIP AT THE FOOT OF THIS IMAGE BEFORE READING THE " +
				"IMAGE. It reports how many transitions this capture actually parked and " +
				"what the editor's box measured at that instant. Headless Chromium under " +
				"--virtual-time-budget does not run the document's rendering lifecycle, so " +
				"the morph's transitions are never created and there is nothing to pause: " +
				"every frame here shows the editor at its FINAL geometry. This is the rig's " +
				"limit, not the morph's — the four-property path is traced in real Chromium " +
				"by daycard_morph_trace_test.go and pinned by daycard_open_close.test.mjs. " +
				"What no artefact in this directory proves is that the compositor " +
				"interpolated between the two geometries; that is a live-client check and " +
				"the report names it as open",
		})
	}
	return shots
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

	css := benchCSS(t) + benchBlockSheet(t) + dayCardCSS(t)
	for _, s := range daycardShotList() {
		t.Run(s.file, func(t *testing.T) {
			data := benchFxData(true, true)
			if !s.mount.CanCreate {
				data = benchFxData(false, false)
			}
			data.DayCard = s.mount
			// THE CALENDAR IS PASSED, AND THAT IS THE WHOLE OF FINDING 4.
			// `Calendar: nil` used to be handed here, and dayCardCategories
			// returns nil for a nil calendar — so the payload carried NO type
			// palette, the module fell to its single `[{slug:'', name:'No
			// type'}]` seed, and all fifteen shots rendered the TYPE row as ONE
			// bare chip with no hue rail, no stroke pattern and no glyph. The
			// captions claimed those shots proved "the locked type rail" and
			// "every type option's rail-pattern plus glyph survive"; they
			// photographed the fallback. A caption is not evidence and a
			// fixture that seeds nothing cannot be evidence of a palette.
			cal := benchFxTypedCalendar()
			data.DayCardJSON = dayCardPayloadJSON(
				dayCardSeed{
					CanAuthor: s.mount.CanCreate, CanRestrict: s.mount.CanRestrict,
					Roster: benchFxRoster(),
				},
				dayCardSource{Block: data.Primary, Calendar: &cal})
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
		//
		// AND AT THE TOP LAYER, WHICH IS THE PART THAT WAS WRONG. The card and
		// the editor are `[popover]`: Chromium promotes them into the TOP LAYER,
		// which is NOT painted as a descendant of <html>, so an ancestor
		// `filter` does not reach them. The two "greyscale" proofs therefore
		// shot a grey PAGE with a FULL-COLOUR EDITOR sitting on it, and their
		// captions claimed the opposite. Measured on the old pair, 20k samples
		// per region: outside the editor max chroma 2 (genuinely grey), inside
		// it 0.39% of pixels above chroma 20 with a maximum of 255 — the "No
		// type" chip's border was still rgb(79,107,239).
		//
		// The filter is therefore declared on the top-layer roots BY NAME as
		// well, and ::backdrop with them. daycardShotChromaSanity is the check
		// that this actually took: it re-reads the page it built and refuses a
		// greyscale shot whose filter does not name every root the module can
		// promote.
		gray = `html{filter:grayscale(1)}` +
			`.cal-daycard,.cal-dayeditor,.cal-daycard-drag{filter:grayscale(1)}` +
			`::backdrop{filter:grayscale(1)}`
	}
	return `<!doctype html><html` + cls + `><head><meta charset="utf-8"><style>` +
		`html,body{margin:0;padding:0}` +
		`body{background:#f9fafb;color:#111827;` +
		`font-family:ui-sans-serif,system-ui,-apple-system,"Segoe UI",sans-serif}` +
		`html.dark body{background:oklch(0.165 0.010 265);color:oklch(0.975 0.002 265)}` +
		gray +
		`.shot-wrap{padding:16px}` +
		// THE RIG'S OWN REPORT, WHERE IT CAN BE READ. Fixed to the bottom edge,
		// which the editor's placed box does not reach at any viewport this
		// generator shoots. It starts with what it starts with — "no transition
		// was created" is a measurement and it is printed as one.
		`.shot-diag{position:fixed;left:0;right:0;bottom:0;z-index:1;` +
		`font:11px/1.5 ui-monospace,SFMono-Regular,Menlo,monospace;padding:6px 10px;` +
		`background:oklch(0.18 0.02 265);color:oklch(0.98 0 0)}` +
		// THE FOLD REPORT SITS ABOVE THE RIG STRIP AND IS ON EVERY SHOT. It is
		// finding 1's mechanical answer: the image says how much of `.ed-body`
		// is inside its own frame, so a reader can never again take a caption's
		// word for a mark that scrolled out of view.
		`.shot-fold{position:fixed;left:0;right:0;bottom:24px;z-index:1;` +
		`font:11px/1.5 ui-monospace,SFMono-Regular,Menlo,monospace;padding:6px 10px;` +
		`background:oklch(0.30 0.09 60);color:oklch(0.99 0 0)}` +
		fmt.Sprintf(`.cal-bench{width:%dpx;max-width:100%%}`, min(s.w-40, 1180)) +
		`h1{font-size:15px;line-height:1.2;margin:0 0 4px;letter-spacing:-.02em}` +
		`.shot-cap{font-size:11px;line-height:1.5;margin:0 0 12px;opacity:.72;max-width:80ch}` +
		css +
		`</style></head><body><div class="shot-wrap">` +
		`<h1>` + s.title + `</h1><p class="shot-cap">` + s.caption + `</p>` +
		body +
		`</div>` +
		`<div class="shot-fold" id="shot-fold">RIG: the fold report did not run</div>` +
		`<div class="shot-diag" id="shot-diag">RIG: no transition was created to park — ` +
		`see the report's morph section</div>` +
		`<script>` + vis + `</script><script>` + mod + `</script>` +
		// THE ORDER IS THE CLAIM'S ORDER: open the doors, scroll to where the
		// caption says it is looking, THEN report the fold. A diag written
		// before the scroll would describe a frame this shot does not contain.
		`<script>window.addEventListener('load', function () {` +
		s.script + daycardScrollTo(s.scroll) + daycardFoldDiag +
		`});</script>` +
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

// benchFxTypedCalendar is the primary fixture calendar WITH an event-type
// palette, and it exists only for the capture rig.
//
// WHY IT IS SEPARATE FROM benchFxHarptos. The Bench fixture is shared by every
// assertion in this package and the Block's own event marks resolve their hue
// and pattern THROUGH the calendar's categories — adding six of them to the
// shared fixture would re-colour marks in tests that are about something else
// entirely. The capture needs the palette in the PAYLOAD, not in the
// projection, and the payload is built from the Calendar passed to
// dayCardSource. So the palette is added here, one call site, and nothing that
// is not a screenshot sees it.
//
// THE SIX ARE THE DRAWING'S OWN. mockups/v4-proposed/event-editor.html's TYPES
// table is session · quest · festival · social · downtime · celestial, and the
// glyphs and hues below are that table's, converted from its --ev-* tokens, so
// a built shot can be laid beside editor-gm-light.png and read.
//
// TWO DIVERGENCES FROM THE DRAWING, BOTH DELIBERATE AND BOTH IN THE REPORT:
//
//   · THE PATTERN IS DERIVED, NOT ASSIGNED. The drawing hand-picks p5/p2/p4/
//     p1/p3/p6. The product derives blockPatternFor(slug) so a type wears the
//     SAME stroke in the editor that its events wear in the grid and the
//     Ledger. A hand-assigned palette would be a second identity for the same
//     category.
//   · THE ICONS ARE OPERATOR DATA. Chronicle's DefaultEventCategories seeds
//     EMOJI (⭐ ⚔ ❗ 🎂 🎉 🚶), which no stylesheet can ink; the geometric marks
//     below are the drawing's and are what a campaign that wants the shape
//     channel would enter. The build law's "glyphs inked --text-body, never
//     --axis" is a claim about the SHEET, and it is asserted directly by
//     TestDayCardCSS_* rather than inferred from a picture.
func benchFxTypedCalendar() Calendar {
	cal := benchFxHarptos()
	cal.EventCategories = []EventCategory{
		{Slug: "session", Name: "Session", Icon: "■", Color: "oklch(0.55 0.04 255)", SortOrder: 0},
		{Slug: "quest", Name: "Quest", Icon: "▲", Color: "oklch(0.60 0.19 25)", SortOrder: 1},
		{Slug: "festival", Name: "Festival", Icon: "✦", Color: "oklch(0.75 0.16 80)", SortOrder: 2},
		{Slug: "social", Name: "Social", Icon: "◆", Color: "oklch(0.55 0.17 258)", SortOrder: 3},
		{Slug: "downtime", Name: "Downtime", Icon: "●", Color: "oklch(0.65 0.13 145)", SortOrder: 4},
		{Slug: "celestial", Name: "Celestial", Icon: "☾", Color: "oklch(0.55 0.19 295)", SortOrder: 5},
	}
	return cal
}

// TestDayCardShotFixtureSeedsThePaletteItPhotographs is ALWAYS ON, and it is the
// cheap check that finding 4 cannot recur silently.
//
// The generator is env-gated, so nothing in CI ever looks at its output. What CI
// CAN do is assert that the page the generator builds actually contains the
// palette its captions claim to prove — a payload with no categories produces a
// type rail with one bare "No type" chip, which looks like a rendered control
// and is a rendered absence.
func TestDayCardShotFixtureSeedsThePaletteItPhotographs(t *testing.T) {
	cal := benchFxTypedCalendar()
	if len(cal.EventCategories) < 2 {
		t.Fatal("the capture fixture seeds fewer than two types; the rail it photographs " +
			"cannot show a palette")
	}
	raw := dayCardPayloadJSON(
		dayCardSeed{CanAuthor: true, CanRestrict: true, Roster: benchFxRoster()},
		dayCardSource{Block: benchFxData(true, true).Primary, Calendar: &cal})
	for _, want := range []string{
		`"categories":`,
		`"slug":"festival"`,
		`"glyph":"✦"`,
		`"pattern":"p`,
	} {
		if !strings.Contains(raw, want) {
			t.Errorf("the capture payload is missing %q — every shot from this generator "+
				"would photograph the module's `No type` fallback while its caption "+
				"claimed a locked triple", want)
		}
	}
}

// daycardBelowFoldMarks are the chrome marks a TWO-COLUMN editor puts below
// `.ed-body`'s scroll fold at the viewport heights this rig shoots. They are
// named as the strings a caption would use, because a caption is the thing
// under test.
//
// Derived, not guessed: at 1440×900 the left column is ~437px and `.vis` is
// `repeat(auto-fit, minmax(150px, 1fr))`, so the third visibility card wraps to
// a second row — and everything after it (`audb`, `Tied to`) wraps with it.
var daycardBelowFoldMarks = []string{
	"Restricted", "roster", "allow/deny", "Tied to", "◈",
}

// TestDayCardShotCaptionsStayInsideTheirOwnFrame is ALWAYS ON, and it is the
// cheap check that the fix round's finding 1 cannot recur silently.
//
// FINDING 1, IN ONE SENTENCE: four desktop captions named the ◈ Restricted
// card, the allow/deny roster and `Tied to` over frames that were cut off at
// `.ed-body`'s 620px scroll fold and contained none of them. The build was
// right; the evidence was not. The failure mode is not a typo — it is a caption
// written from the DOM instead of from the image.
//
// So the rule is mechanical: a shot that is TWO-COLUMN (>= the sheet's own
// 1080px breakpoint) and does NOT scroll `.ed-body` may not name a below-fold
// mark in its title or caption. Scrolled shots may, un-scrolled narrow shots
// may (one column, and 07 at 820×1200 photographs the lot un-scrolled), and a
// caption that merely says where the mark IS NOT — "are below this frame",
// "are photographed in 01b" — is what the pairs are for and is admitted by an
// explicit deferral phrase rather than by accident.
func TestDayCardShotCaptionsStayInsideTheirOwnFrame(t *testing.T) {
	// A caption may name a below-fold mark if it is DEFERRING it to another
	// frame. The phrases are exhaustive and short on purpose: an open-ended
	// escape hatch would make this test decorative.
	defers := []string{
		"below this frame", "are below the fold", "photographed in 01b",
		"are in 04b", "proved in 11b", "would occupy",
	}
	for _, s := range daycardShotList() {
		if s.w < daycardEditorTwoColumnPx || s.scroll != "" {
			continue
		}
		text := s.title + " " + s.caption
		deferred := false
		for _, d := range defers {
			if strings.Contains(text, d) {
				deferred = true
				break
			}
		}
		for _, mark := range daycardBelowFoldMarks {
			if !strings.Contains(text, mark) || deferred {
				continue
			}
			t.Errorf("shot %s is %dpx wide (two-column) and does not scroll %s, but its "+
				"caption names %q — that mark is below `.ed-body`'s scroll fold and is NOT "+
				"in this frame. Either scroll the body, split the shot into an upper/lower "+
				"pair, or defer the mark to the frame that has it. This is the fix round's "+
				"finding 1, and it is the difference between evidence and a sentence",
				s.file, s.w, daycardEdBody, mark)
		}
	}
}

// TestDayCardShotRigReportsTheFoldOnEveryShot is ALWAYS ON. The fold report is
// the part of finding 1's answer a reader who never opens this file relies on:
// it is burned onto the image itself. If the strip or its script went missing,
// every future capture would go back to being silent about what it cropped.
func TestDayCardShotRigReportsTheFoldOnEveryShot(t *testing.T) {
	data := benchFxData(true, true)
	data.DayCard = DayCardMount{CanCreate: true, CanAuthorDmOnly: true, CanDelete: true,
		CanRestrict: true, CampaignID: "camp-1"}
	page := daycardShotPage(t,
		daycardShot{w: 1440, h: 900, script: daycardOpenEditor, scroll: daycardEdBody},
		dayCardCSS(t), benchStripLinks(renderBench(t, data)))
	for _, want := range []string{
		`id="shot-fold"`,       // the strip exists…
		".shot-fold{position:", // …and is pinned where the editor cannot cover it
		"BELOW THE FOLD in this frame",
		"sc.scrollTop = sc.scrollHeight", // the scroll actually happens
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the shot page is missing %q — a capture from this rig would not say "+
				"what it cropped, which is the whole of finding 1", want)
		}
	}
	// …and the fold report must come AFTER the scroll, or it describes a frame
	// the image does not contain.
	if strings.Index(page, "sc.scrollTop") > strings.Index(page, "BELOW THE FOLD in this frame") {
		t.Error("the fold report runs before the scroll; it would describe the resting " +
			"scroll position of an image captured somewhere else")
	}
	// The sheet must still declare the fold this whole apparatus is about. If
	// `.ed-body` ever stops scrolling, this file's pairs become noise and
	// somebody should be told rather than left to notice.
	sheet := dayCardCSS(t)
	if !strings.Contains(sheet, "max-block-size: min(70vh, 620px)") ||
		!strings.Contains(sheet, "overflow-y: auto") {
		t.Error("calendar-daycard.css no longer declares a scrolling `.ed-body`; the " +
			"upper/lower band pairs and this guard are about a fold that has moved")
	}
}

// TestDayCardShotGreyscaleReachesTheTopLayer is ALWAYS ON, and it is the cheap
// check that finding 2 cannot recur silently.
//
// A `[popover]` is promoted to the TOP LAYER and is not painted as a descendant
// of <html>, so `html{filter:grayscale(1)}` does not reach it. The two proofs
// that claimed "hue removed" shot a grey page with a full-colour editor on it.
// This asserts the built page names every root the module can promote — the
// only mechanical statement available without decoding a PNG here.
func TestDayCardShotGreyscaleReachesTheTopLayer(t *testing.T) {
	data := benchFxData(true, true)
	data.DayCard = DayCardMount{CanCreate: true, CanAuthorDmOnly: true, CanDelete: true,
		CanRestrict: true, CampaignID: "camp-1"}
	page := daycardShotPage(t,
		daycardShot{w: 1440, h: 900, gray: true, script: daycardOpenEditor}, dayCardCSS(t),
		benchStripLinks(renderBench(t, data)))
	for _, root := range []string{".cal-daycard", ".cal-dayeditor", ".cal-daycard-drag"} {
		if !strings.Contains(page, root+",") && !strings.Contains(page, root+"{") {
			t.Errorf("a greyscale shot page does not filter %s — that root is a top-layer "+
				"popover and an ancestor filter does not reach it, so the capture would "+
				"show it in full colour while the caption said hue was removed", root)
		}
	}
	// …and the counterfactual: a NON-greyscale page must carry none of it, or
	// the assertion above is true of every page and proves nothing.
	plain := daycardShotPage(t,
		daycardShot{w: 1440, h: 900, script: daycardOpenEditor}, dayCardCSS(t),
		benchStripLinks(renderBench(t, data)))
	if strings.Contains(plain, "grayscale(1)") {
		t.Error("a non-greyscale shot page declares a greyscale filter; the two arms of " +
			"this proof are the same page")
	}
}
