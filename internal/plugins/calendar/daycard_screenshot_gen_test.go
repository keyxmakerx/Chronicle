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

	calblock "github.com/keyxmakerx/chronicle/internal/widgets/calendar_block"
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
  var edroot = document.querySelector('[data-cal-dayeditor]');
  if (fold) {
    if (!edroot || !edroot.hasAttribute('data-dc-shown')) {
      fold.textContent = 'RIG: the editor is not open in this shot (card or drag capture) — ' +
        'there is no fold to report';
    } else if (!edb) { fold.textContent = 'RIG: no .ed-body on this page'; }
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
    // RELAYED OUT OF THE FRAME. A sub-500 shot renders inside a nested
    // browsing context (see daycardShotFrame) and the editor is a TOP-LAYER
    // popover, so these strips sit underneath it and out of the picture. The
    // parent prints them below the frame, where they are readable.
    if (window.parent && window.parent !== window) {
      window.parent.postMessage('FOLD:' + fold.textContent, '*');
      var dg = document.getElementById('shot-diag');
      if (dg) window.parent.postMessage('DIAG:' + dg.textContent, '*');
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

// ── EDIT MODE, AND WHY THE REJECTED SET HAD NONE OF IT ────────────────────
//
// FINDING 2, IN ONE SENTENCE: every shot in the rejected set ran
// daycardOpenEditor, which is `cell.click()` then `[data-dc-new]`.click() —
// CREATE mode — and of the 22 gating stills TWENTY-ONE are edit-mode editors
// or player cards. So the chrome states the stills actually gate were
// photographed nowhere: the tie pill and its 24px ✕, `Delete event`, the
// `Save changes` footer, the populated `every [N] [unit]` recurrence row and
// the Restricted-SELECTED roster. Four of the report's own named divergences
// (the unit list, the `10-day week` label, `moon phase`, no `+ tie another`)
// are claims about controls that render only in those states.
//
// THE DOOR IS REAL AND SO IS THE PATH. `[data-dc-edit]` is the row door the
// producer renders for a Scribe, `edLoad` fetches the record through
// `Chronicle.apiFetch`, and `edOpen('edit', …)` fills the chrome from it.
// Nothing below reaches past that door.
//
// WHAT IS SUBSTITUTED, AND IT IS DISCLOSED ON THE IMAGE. A `file://` capture
// has no server, so `Chronicle.apiFetch` is stubbed to answer exactly one
// request — `GET …/events/:eid` — with a canned record. That is the ONLY
// fabrication in this directory and it is the record, never the render: the
// chrome that draws it is the shipped chrome, the mapping from record to
// controls is the shipped `edFill`, and the stub writes what it answered onto
// the shot's own diagnostic strip. Every other verb is REFUSED by the stub, so
// a capture cannot accidentally photograph a save it did not make.
//
// THE RECORD IS CHOSEN TO LIGHT THE CONTROLS THE STILLS GATE, not to look
// pretty: a timed event (the time row un-hidden), a `custom` recurrence with
// interval 3 (the `every 3 <week-noun>` row populated, which is where the
// corrected unit list is legible), an allow+deny `visibility_rules` pair (the
// ◈ Restricted card SELECTED and the roster showing both marks), and an
// `entity_id` (the tie pill with its real 24px ✕ button).
const daycardStubEditRecord = `
  window.Chronicle = window.Chronicle || {};
  window.__shotStub = 'not called';
  window.Chronicle.apiFetch = function (url, opts) {
    var method = (opts && opts.method) || 'GET';
    // THE STUB ANSWERS ONE READ AND REFUSES EVERYTHING ELSE. A capture that
    // could reach a write path would be a capture that might photograph a
    // state no user produced.
    if (method !== 'GET' || !/\/events\/[^/]+$/.test(url)) {
      window.__shotStub = 'REFUSED ' + method + ' ' + url;
      return Promise.resolve({ ok: false, status: 405,
        json: function () { return Promise.resolve({}); } });
    }
    // The date comes from the day the card is actually open on, read off the
    // page's own payload — a record dated somewhere else would make edFill's
    // edKeyFor miss and the date grid would disagree with the head.
    var ymd = { year: null, month: null, day: null };
    var head = document.querySelector('[data-cal-daycard] [data-dc-head]');
    var key = head ? head.getAttribute('data-day') : '';
    var pel = document.querySelector('[data-cal-daycard-payload]');
    if (pel && key) {
      try {
        var pay = JSON.parse(pel.getAttribute('data-cal-daycard-payload'));
        (pay.calendars || []).forEach(function (c) {
          (c.days || []).forEach(function (d) {
            if (d.key === key) { ymd = { year: d.year, month: d.month, day: d.day }; }
          });
        });
      } catch (e) { /* an unreadable payload leaves the record undated */ }
    }
    // THE ID IS THE ONE THAT WAS ASKED FOR, echoed back. A stub that answered
    // a fixed id for every row would make the editor's evt- readout a
    // decoration rather than a readout, and the caption below would be naming
    // a request that was not made.
    var asked = String(url).split('/').pop();
    var rec = {
      id: asked, name: 'Feast of the Moonmaiden',
      description: 'The tenday-long feast the Vayle house keeps for its founder. ' +
        'Doors open at dusk; the Umber moon is full on the third night.',
      category: 'festival',
      year: ymd.year, month: ymd.month, day: ymd.day,
      all_day: false, start_hour: 19, start_minute: 30, end_hour: 23, end_minute: 0,
      is_recurring: true, recurrence_type: 'custom', recurrence_interval: 3,
      visibility: 'everyone',
      visibility_rules: '{"allowed_users":["u-kael","u-nissa"],"denied_users":["u-tam"]}',
      entity_id: 'ent-nissa-vayle', entity_name: 'Nissa Vayle'
    };
    window.__shotStub = 'ANSWERED GET ' + url;
    var tag = document.getElementById('shot-diag');
    if (tag) {
      tag.textContent = 'RIG: EDIT MODE. Chronicle.apiFetch is STUBBED for this capture — ' +
        'a file:// page has no server. It answered exactly one request, ' +
        'GET …/events/' + asked + ', with a canned record (festival · timed 19:30–23:00 · ' +
        'custom recurrence interval 3 · allowed u-kael,u-nissa · denied u-tam · ' +
        'tied to ent-nissa-vayle) and REFUSES every other verb with a 405. ' +
        'The record is substituted; the chrome that draws it is the shipped chrome.';
    }
    return Promise.resolve({ ok: true, status: 200,
      json: function () { return Promise.resolve(rec); } });
  };
`

// daycardOpenEditRow walks the day cells until one of them opens a card with a
// row `Edit` door on it, then clicks that door. It walks rather than assuming,
// because which fixture day carries an event is not this file's business and a
// driver that clicked cell zero and found nothing would silently photograph a
// create-mode editor with an edit-mode caption on it.
// It picks the day with the MOST rows rather than the first day with any,
// because a one-row card photographs a row list that cannot be read as one and
// the day the fixture loads up is the day worth photographing.
const daycardOpenEditRow = `
  var cells = Array.prototype.slice.call(
    document.querySelectorAll('[data-bench-block] [data-day][data-day-ord]'));
  var bestCell = null, bestN = 0;
  cells.forEach(function (c) {
    c.click();
    var n = document.querySelectorAll('[data-cal-daycard] [data-dc-edit]').length;
    if (n > bestN) { bestN = n; bestCell = c; }
  });
  if (bestCell) {
    bestCell.click();
    var door = document.querySelector('[data-cal-daycard] [data-dc-edit]');
    if (door) door.click();
  }
`

// ── §10 ITEM 12: THE DRAG PREVIEW, PHOTOGRAPHED ───────────────────────────
//
// The row asks for "the drag preview at a 1-cell, 3-cell and month-spanning
// drag; the single-day click unregressed; and the Block's DOM byte-identical
// before and after." The behaviour is pinned by
// test/js/daycard_drag_create.test.mjs (11 tests, green) — but the row is a
// CAPTURE row and the rejected set neither performed it nor disclosed it as
// skipped, which is the same silence finding 3 is about.
//
// daycardDragScript drives the shipped pointer path — pointerdown on a cell,
// pointermove across the run — and STOPS THERE. There is no pointerup, so the
// overlay stays painted and the frame is the preview mid-drag rather than the
// editor the drag would have opened.
//
// IT CARRIES ITS OWN BLOCK-IMMUTABILITY VERDICT ONTO THE IMAGE. The Block's
// innerHTML is captured before the first pointer event and re-compared after
// the last one, and the strip prints IDENTICAL or DIFFERS with the byte
// lengths. [ER-8] and bound 4 both turn on the preview being a page-level
// overlay that never touches a cell; a photograph of a highlight cannot show
// you which mechanism drew it, and this can.
func daycardDragScript(fromOrd, toOrd int, why string) string {
	return fmt.Sprintf(`
  var host = document.querySelector('[data-bench-block]');
  var cells = Array.prototype.slice.call(
    host.querySelectorAll('[data-day][data-day-ord]'));
  var before = host.innerHTML;
  function pev(el, type) {
    el.dispatchEvent(new PointerEvent(type,
      { bubbles: true, cancelable: true, button: 0, pointerId: 1, isPrimary: true }));
  }
  var from = %d, to = %d;
  if (cells[from]) {
    pev(cells[from], 'pointerdown');
    for (var i = from + 1; i <= to && cells[i]; i++) { pev(cells[i], 'pointermove'); }
  }
  var layer = document.querySelector('[data-dc-drag]');
  var boxes = layer && !layer.hidden ? layer.children.length : 0;
  var same = host.innerHTML === before;
  var tag = document.getElementById('shot-diag');
  if (tag) {
    tag.textContent = 'RIG: DRAG PREVIEW, MID-DRAG (pointerdown + pointermove, NO pointerup). ' +
      %q + ' · cells %d→%d · preview boxes drawn: ' + boxes +
      ' · the Block DOM is ' + (same ? 'BYTE-IDENTICAL' : 'DIFFERENT') +
      ' before and after (' + before.length + ' vs ' + host.innerHTML.length + ' chars) — ' +
      'the preview is a page-level overlay and never touches a cell ([ER-8], bound 4).';
  }
`, fromOrd, toOrd, why, fromOrd, toOrd)
}

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
			caption: "the same absence at a REAL 390 — a nested browsing context, because `--window-size` clamps this Chromium to 500px and the rejected set's three `-390x844` files were 500px wide. Whether anything crosses the horizontal fold is NOT read off this image: it is MEASURED, at 390 / 820 / 1440 in both themes and in both modes, by TestDayCardFloorsProbe (DAYCARD_FLOORS=1); see fold-and-floor-probe.txt"},
		{file: "06b-editor-codm-390x844-light.png", mount: codm, w: 390, h: 844, script: daycardOpenEditor,
			title:   "Event editor · co-DM · 390×844 · light",
			caption: "the co-DM at the phone width — §10 item 2's last matrix cell"},
		{file: "07-editor-gm-820-light.png", mount: gm, w: 820, h: 1200, script: daycardOpenEditor,
			title: "Event editor · GM · 820×1200 · light",
			caption: "the tablet width: ONE column, and the widest un-scrolled frame the fold allows — `.ed-body` caps at min(70vh, 620px), so the 620px cap binds at every viewport this rig shoots and 122px of the live preview sit below this frame (the strip says so). What IS in this one un-scrolled frame: all three visibility cards and `Tied to`, which is the direct proof that the build renders what 01b has to scroll to reach. Horizontal scroll is measured, not eyeballed — TestDayCardFloorsProbe, see fold-and-floor-probe.txt"},
		{file: "07b-editor-gm-820-light-lowerband.png", mount: gm, w: 820, h: 1200,
			script: daycardOpenEditor, scroll: daycardEdBody,
			title:   "Event editor · GM · 820×1200 · light — LOWER BAND",
			caption: "the 122px 07 leaves below its frame: the live-preview column's foot"},

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

		// ── §10 items 1 and 2: EDIT MODE — the state 21 of the 22 stills gate ──
		//
		// The rejected set had none of it. See daycardStubEditRecord's header
		// for what is substituted (one GET, one record) and what is not (the
		// door, the fetch path, edFill, and every pixel of the chrome).
		{file: "16-editor-gm-edit-820-light.png", mount: gm, w: 820, h: 1400,
			script: daycardStubEditRecord + daycardOpenEditRow,
			title:  "Event editor · EDIT · GM · 820×1400 · light — UPPER BAND",
			caption: "edit mode, one column. In THIS frame: the head reads `Edit event · 14 Deepwinter 1523`, the id readout carries the id the stub was actually asked for rather than `draft`, `Festival` is the selected type, the time row is un-hidden at 19:30–23:00, and the recurrence row is populated — `Repeats · every 3 · 10-day week`, which is where the corrected unit list is legible: no `year` at all, `day` and `moon phase` chipped, and the week unit wearing the CALENDAR's own noun rather than the literal string `week`. Pinned to the box and therefore also in frame: `Delete event` and a footer reading `Save changes`. The rest is in 16b"},
		{file: "16b-editor-gm-edit-820-light-lowerband.png", mount: gm, w: 820, h: 1400,
			script: daycardStubEditRecord + daycardOpenEditRow, scroll: daycardEdBody,
			title:  "Event editor · EDIT · GM · 820×1400 · light — LOWER BAND",
			caption: "what 16 leaves below its frame, and it is the state no image in the rejected set contained: the ◈ Restricted card SELECTED, the allow/deny roster with all five members and both marks (Kael and Nissa allowed, Bryn, Rell and Tam denied), and the `Tied to` pill for Nissa Vayle with its real 24px ✕ button. This is the frame to lay beside editor-gm-1440x900.png"},
		{file: "17-editor-gm-edit-1440x900-light.png", mount: gm, w: 1440, h: 900,
			script: daycardStubEditRecord + daycardOpenEditRow,
			title:  "Event editor · EDIT · GM · 1440×900 · light — UPPER BAND",
			caption: "edit mode at the desktop width: the head reads `Edit event`, the id readout carries `evt-7f3a` rather than `draft`, the type rail has `Festival` selected, the time row is un-hidden at 19:30–23:00, and the recurrence row is populated. The footer's `Delete event` and `Save changes` are pinned to the box and are in this frame. What is below it is in 17b"},
		{file: "17b-editor-gm-edit-1440x900-light-lowerband.png", mount: gm, w: 1440, h: 900,
			script: daycardStubEditRecord + daycardOpenEditRow, scroll: daycardEdBody,
			title:  "Event editor · EDIT · GM · 1440×900 · light — LOWER BAND",
			caption: "the ◈ Restricted card SELECTED (not merely present — this is the state no shot in the rejected set contained), the allow/deny roster with Kael and Nissa allowed and Tam denied, and the `Tied to` pill with its 24px ✕"},
		{file: "18-editor-gm-edit-1440x900-dark.png", mount: gm, w: 1440, h: 900, dark: true,
			script: daycardStubEditRecord + daycardOpenEditRow,
			title:  "Event editor · EDIT · GM · 1440×900 · dark — UPPER BAND",
			caption: "the same record in dark"},
		{file: "18b-editor-gm-edit-1440x900-dark-lowerband.png", mount: gm, w: 1440, h: 900, dark: true,
			script: daycardStubEditRecord + daycardOpenEditRow, scroll: daycardEdBody,
			title:   "Event editor · EDIT · GM · 1440×900 · dark — LOWER BAND",
			caption: "the selected ◈ Restricted card, its roster and the tie pill, in dark"},
		{file: "19-editor-codm-edit-1440x900-light.png", mount: codm, w: 1440, h: 900,
			script: daycardStubEditRecord + daycardOpenEditRow,
			title:  "Event editor · EDIT · co-DM · 1440×900 · light — THE DELETE AXIS",
			caption: "read against 17. The record is byte-identical and the viewer is not: `Delete event` is ABSENT here and present there, because Delete is `CanDelete` (Owner) and this viewer is a Scribe with the dm_only grant. The ◥ DM only card is still present. This is the axis proof create mode cannot make — a draft has no id to delete, so 01 and 04 are both Delete-less for a reason that has nothing to do with permission"},
		{file: "19b-editor-codm-edit-1440x900-light-lowerband.png", mount: codm, w: 1440, h: 900,
			script: daycardStubEditRecord + daycardOpenEditRow, scroll: daycardEdBody,
			title:  "Event editor · EDIT · co-DM · 1440×900 · light — LOWER BAND",
			caption: "read against 17b. The stored record's audience pair is unchanged and the co-DM is shown NEITHER the ◈ card NOR the names — the editor opens on Public for a viewer who cannot author the mode, and the write path round-trips the stored pair untouched. The `Tied to` pill is here, because ties are Scribe-floor"},
		{file: "20-editor-scribe-edit-1440x900-light.png", mount: scribe, w: 1440, h: 900,
			script: daycardStubEditRecord + daycardOpenEditRow,
			title:  "Event editor · EDIT · Scribe, no grant · 1440×900 · light",
			caption: "the edit-mode absence proof: no Visibility fieldset at all, no Delete, and `Save changes` in the footer. A Scribe editing a restricted event neither sees nor disturbs its audience"},
		{file: "21-editor-gm-edit-390x844-light.png", mount: gm, w: 390, h: 844,
			script: daycardStubEditRecord + daycardOpenEditRow,
			title:  "Event editor · EDIT · GM · 390×844 · light",
			caption: "edit mode on a real phone viewport, one column. The 390px wrap of the trailing `:`+minute in the time row is PRE-EXISTING IN THE DRAWING and out of scope — it is not silently fixed and it is not copied as a target"},
		{file: "21b-editor-gm-edit-390x844-light-lowerband.png", mount: gm, w: 390, h: 844,
			script: daycardStubEditRecord + daycardOpenEditRow, scroll: daycardEdBody,
			title:   "Event editor · EDIT · GM · 390×844 · light — LOWER BAND",
			caption: "the phone's lower band in edit mode: the selected ◈ card, the roster, the tie pill and the footer"},

		// ── §10 item 12: drag-create, the three spans it names ──────────────
		{file: "22-drag-1cell-1440x900-light.png", mount: gm, w: 1440, h: 900,
			script: daycardDragScript(3, 3, "a ONE-CELL press: a drag of zero cells is a click"),
			title:  "Drag-create · ONE CELL · 1440×900 · light — THE UNREGRESSED CLICK",
			caption: "pointerdown on a day and no move. `drag.moved` never becomes true, `dragPaint` returns before drawing anything, and NO preview box exists — which is [DC-11] term 5 as a picture: a drag of zero cells is a click and still opens the card. The strip counts the boxes (0) and re-compares the Block's innerHTML"},
		{file: "23-drag-3cell-1440x900-light.png", mount: gm, w: 1440, h: 900,
			script: daycardDragScript(3, 5, "a THREE-CELL run inside one week row"),
			title:  "Drag-create · THREE CELLS · 1440×900 · light",
			caption: "three contiguous days in one row: ONE preview box, drawn by the page-level overlay from the cells' own rects. The Block is not marked, not classed and not entered — the strip says so with the byte lengths"},
		{file: "24-drag-monthspan-1440x900-light.png", mount: gm, w: 1440, h: 900,
			script: daycardDragScript(0, 29, "a MONTH-SPANNING run, day 1 to day 30"),
			title:  "Drag-create · MONTH-SPANNING · 1440×900 · light",
			caption: "day 1 to day 30 across every row of a ten-day-week month: ONE BOX PER CONTIGUOUS ROW, never a union box over the whole span. A union would paint days that ARE in the run and days that are not with the same ink, which is a preview lying about what it is about to create"},
		{file: "25-drag-monthspan-1440x900-dark.png", mount: gm, w: 1440, h: 900, dark: true,
			script: daycardDragScript(0, 29, "a MONTH-SPANNING run, day 1 to day 30"),
			title:   "Drag-create · MONTH-SPANNING · 1440×900 · dark",
			caption: "the same run in dark; the overlay's ink is its own and reads on both grounds"},
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
			// THE FIXTURE CARRIES A PALETTE **AND** EVENTS, and each half was
			// its own finding.
			//
			// The palette was finding 4 of the previous round: `Calendar: nil`
			// used to be handed to the payload builder, dayCardCategories
			// returns nil for a nil calendar, so the module fell to its single
			// `[{slug:'', name:'No type'}]` seed and every shot rendered TYPE as
			// ONE bare chip while its caption claimed a locked triple.
			//
			// The EVENTS are finding 2 of this one: benchFxData projects with
			// none, so every card said "No events on this day", no row carried
			// an Edit door, and create mode was the only editor the rig could
			// reach — for a set whose fidelity gate is 21 edit-mode stills.
			//
			// A caption is not evidence, and a fixture that seeds nothing cannot
			// be evidence of the thing it does not seed.
			data := benchFxShotData(s.mount)
			page := daycardShotPage(t, s, css, benchStripLinks(renderBench(t, data)))
			dir := t.TempDir()
			src := filepath.Join(dir, "shot.html")
			if err := os.WriteFile(src, []byte(page), 0o644); err != nil {
				t.Fatalf("write page: %v", err)
			}
			// ── A PHONE SHOT IS TAKEN IN A PHONE VIEWPORT, OR IT IS NOT A
			//    PHONE SHOT ──────────────────────────────────────────────
			//
			// This Chromium CLAMPS the headless window to a 500px minimum
			// width — `--window-size=390,844` yields innerWidth 500, in old
			// headless and in --headless=new alike. That is why the rejected
			// set's three `-390x844` files were captured at 500, and only one
			// of the three said so.
			//
			// A nested browsing context has its own viewport: innerWidth,
			// 100vw, clientWidth and MEDIA QUERIES all resolve against the
			// frame's box. So a sub-500 shot is framed at its exact size, the
			// frame's own edge is visible in the image, and a note beside it
			// says what was done and why. Verified before it was relied on: a
			// 390px frame reports innerWidth 390 and matchMedia('(max-width:
			// 500px)') true.
			target, winW, winH := src, s.w, s.h
			if s.w < daycardFloorMinWindowPx {
				outer := filepath.Join(dir, "outer.html")
				if err := os.WriteFile(outer,
					[]byte(daycardShotFrame(s.w, s.h)), 0o644); err != nil {
					t.Fatalf("write frame page: %v", err)
				}
				target, winW, winH = outer, daycardFloorMinWindowPx, s.h+420
			}
			out := filepath.Join(outDir, s.file)
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			args := []string{
				"--headless", "--no-sandbox", "--disable-gpu", "--hide-scrollbars",
				"--force-device-scale-factor=2",
				fmt.Sprintf("--window-size=%d,%d", winW, winH),
				"--virtual-time-budget=6000",
				"--screenshot=" + out, "file://" + target,
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

// daycardShotFrame wraps a shot page in a viewport of an EXACT size, for the
// widths --window-size will not give. See the runner for the measurement that
// makes it necessary; the note is burned into the image so the substitution
// travels with the artefact rather than only with this file.
func daycardShotFrame(w, h int) string {
	return `<!doctype html><html><head><meta charset="utf-8"><style>` +
		`html,body{margin:0;padding:0;background:oklch(0.22 0.02 265)}` +
		fmt.Sprintf(`iframe{inline-size:%dpx;block-size:%dpx;border:0;display:block}`, w, h) +
		`.vpnote{font:11px/1.5 ui-monospace,SFMono-Regular,Menlo,monospace;` +
		`padding:8px 10px;background:oklch(0.30 0.09 60);color:oklch(0.99 0 0)}` +
		`.vpnote.diag{background:oklch(0.18 0.02 265)}` +
		`.vpnote:empty{display:none}` +
		`</style></head><body>` +
		fmt.Sprintf(`<div class="vpnote">RIG: this frame is a REAL %d×%dpx viewport — `+
			`innerWidth %d, 100vw %dpx, media queries resolved against it. `+
			`--window-size cannot deliver it: this Chromium clamps the headless window `+
			`to a %dpx minimum width, which is why three files in the rejected set were `+
			`named -390x844 and were 500px wide. The dark border is the frame's, not the `+
			`product's.</div>`, w, h, w, w, daycardFloorMinWindowPx) +
		`<iframe src="shot.html"></iframe>` +
		`<div class="vpnote" id="frame-fold">RIG: the framed shot did not report its fold</div>` +
		`<div class="vpnote diag" id="frame-diag"></div>` +
		`<script>window.addEventListener('message', function (e) {` +
		`var d = String(e.data || '');` +
		`if (d.indexOf('FOLD:') === 0) document.getElementById('frame-fold').textContent = d.slice(5);` +
		`if (d.indexOf('DIAG:') === 0) document.getElementById('frame-diag').textContent = d.slice(5);` +
		`});</script>` +
		`</body></html>`
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
		// THE TWO STRIPS ARE STACKED IN ONE FIXED FOOTER, not pinned
		// independently. The first cut gave each its own `bottom:` and the
		// diag — which grows to three lines in edit mode — painted straight
		// over the fold report. A diagnostic a reader cannot see is the same
		// defect as a caption a reader cannot check.
		`.shot-strips{position:fixed;left:0;right:0;bottom:0;z-index:1}` +
		`.shot-diag{` +
		`font:11px/1.5 ui-monospace,SFMono-Regular,Menlo,monospace;padding:6px 10px;` +
		`background:oklch(0.18 0.02 265);color:oklch(0.98 0 0)}` +
		// THE FOLD REPORT SITS ABOVE THE RIG STRIP AND IS ON EVERY SHOT. It is
		// finding 1's mechanical answer: the image says how much of `.ed-body`
		// is inside its own frame, so a reader can never again take a caption's
		// word for a mark that scrolled out of view.
		`.shot-fold{` +
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
		`<div class="shot-strips">` +
		`<div class="shot-fold" id="shot-fold">RIG: the fold report did not run</div>` +
		`<div class="shot-diag" id="shot-diag">RIG: no transition was created to park — ` +
		`see the report's morph section</div></div>` +
		`<script>` + vis + `</script><script>` + mod + `</script>` +
		// THE ORDER IS THE CLAIM'S ORDER: open the doors, scroll to where the
		// caption says it is looking, THEN report the fold. A diag written
		// before the scroll would describe a frame this shot does not contain.
		// THE ORDER IS THE CLAIM'S ORDER AND THE DELAY IS NOT COSMETIC. Edit
		// mode opens from a PROMISE — `edLoad` fetches, then `edOpen` runs in a
		// `.then` — so a scroll and a fold report emitted in the same task as
		// the door click describe an editor that does not exist yet. The first
		// cut did exactly that and burned "shows 0px of 0px" onto an image of a
		// populated editor, which is the same class of defect as the captions
		// this round is fixing. The timer also lets the morph's settle land, so
		// the box being measured is the placed one.
		`<script>window.addEventListener('load', function () {` +
		s.script +
		`setTimeout(function () {` + daycardScrollTo(s.scroll) + daycardFoldDiag + `}, 250);` +
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
// benchFxShotEvents is the capture rig's event set, and it exists because the
// shared Bench fixture has NONE.
//
// THAT ABSENCE IS THE ROOT OF FINDING 2. `benchFxData` projects its Blocks with
// no Events at all, so every card the rejected set opened printed "No events on
// this day", no row carried a `[data-dc-edit]` door, and the ONLY editor a
// driver could reach was the create-mode one behind `+ New event`. Twenty-one
// of the 22 gating stills are edit-mode editors or player cards with a row
// list; the rig could not have photographed either.
//
// SEPARATE FROM THE SHARED FIXTURE, for benchFxTypedCalendar's reason exactly:
// the Block's marks, the Ledger's rows, the count oracle and the attention
// tallies are all projected from the event list, so adding events to the shared
// fixture would move numbers in dozens of assertions that are about something
// else. One call site, capture only.
//
// THE SET IS CHOSEN TO LIGHT THE CHROME THE STILLS GATE: three events on one
// day so the card has a row list and a row to Edit, one of each visibility so
// the gold rail and the Restricted chip are both drawn, a timed event and an
// all-day one, a recurring event, and a tie.
func benchFxShotEvents() []Event {
	s := func(v string) *string { return &v }
	i := func(v int) *int { return &v }
	return []Event{
		{
			ID: "evt-7f3a", CalendarID: "cal-harptos", Name: "Feast of the Moonmaiden",
			Description: s("The tenday-long feast the Vayle house keeps for its founder."),
			Year:        1523, Month: 1, Day: 14, AllDay: false,
			StartHour: i(19), StartMinute: i(30), EndHour: i(23), EndMinute: i(0),
			IsRecurring: true, RecurrenceType: s("custom"), RecurrenceInterval: i(3),
			Visibility:      "everyone",
			VisibilityRules: s(`{"allowed_users":["u-kael","u-nissa"],"denied_users":["u-tam"]}`),
			Category:        s("festival"), EntityID: s("ent-nissa-vayle"),
		},
		{
			ID: "evt-2b17", CalendarID: "cal-harptos", Name: "Session 12 — The Sunken Vault",
			Year: 1523, Month: 1, Day: 14, AllDay: true,
			Visibility: "everyone", Category: s("session"),
		},
		{
			ID: "evt-9c40", CalendarID: "cal-harptos", Name: "What the Umber council decided",
			Year: 1523, Month: 1, Day: 14, AllDay: true,
			Visibility: "dm_only", Category: s("quest"),
		},
		{
			ID: "evt-3d51", CalendarID: "cal-harptos", Name: "Caravan leaves for Tam's Ford",
			Year: 1523, Month: 1, Day: 6, AllDay: true,
			Visibility: "everyone", Category: s("downtime"),
		},
		{
			ID: "evt-5e62", CalendarID: "cal-harptos", Name: "Umber moon full",
			Year: 1523, Month: 1, Day: 21, AllDay: true,
			Visibility: "everyone", Category: s("celestial"),
		},
	}
}

// benchFxShotData is the capture rig's BenchData: the shared fixture with its
// PRIMARY Block re-projected through the REAL projection over benchFxShotEvents
// and benchFxTypedCalendar, and the day card's payload rebuilt from it.
//
// It re-projects rather than reaching into the projected Block, so what a shot
// photographs is what production renders for those events — the marks, their
// hue/pattern/glyph, the viewer filter that removes a dm_only row for a player,
// and the Ledger's own rows all come from the shipped projection.
func benchFxShotData(mount DayCardMount) BenchData {
	data := benchFxData(true, true)
	role := 3
	if !mount.CanCreate {
		data = benchFxData(false, false)
		// A PLAYER IS PROJECTED AS A PLAYER, so the dm_only row is removed by
		// the shipped viewer filter rather than by the fixture omitting it.
		role = 1
	}
	cal := benchFxTypedCalendar()
	d := projectBlock(BlockProjectionInput{
		Calendar: &cal, Events: benchFxShotEvents(),
		Viewer:     BlockViewer{UserID: "u-1", Role: role},
		MonthIndex: cal.CurrentMonth - 1, Year: cal.CurrentYear,
		Sync: calblock.SyncPill{State: blockSyncStateOK, Linked: 1, Total: 4,
			Full: "In sync · 1 of 4 linked", Compact: "In sync · 1 of 4"},
		MoonCap: benchMoonCap,
	})
	d.Layers = benchBlockLayers(blockLayerPrefs{})
	data.Primary = &BenchBlock{Data: d, Manage: benchManage(&cal, "cal-harptos", "camp-1")}
	data.DayCard = mount
	data.DayCardJSON = dayCardPayloadJSON(
		dayCardSeed{
			CanAuthor: mount.CanCreate, CanRestrict: mount.CanRestrict,
			Roster: benchFxRoster(),
		},
		dayCardSource{Block: data.Primary, Calendar: &cal})
	return data
}

// TestDayCardShotFixtureCarriesEventsToEdit is ALWAYS ON, and it is the second
// half of finding 2's mechanical answer: an edit-mode DRIVER with no event to
// drive is a driver that silently photographs a create-mode editor.
func TestDayCardShotFixtureCarriesEventsToEdit(t *testing.T) {
	data := benchFxShotData(DayCardMount{CanCreate: true, CanAuthorDmOnly: true,
		CanDelete: true, CanRestrict: true, CampaignID: "camp-1"})
	raw := data.DayCardJSON
	for _, want := range []string{
		`"id":"evt-7f3a"`, // the row the edit driver clicks
		`"events":[`,      // …reached through a day that HAS events
		`"gold":true`,     // the dm_only rail, so the card's GM marks are drawn
	} {
		if !strings.Contains(raw, want) {
			t.Errorf("the capture payload is missing %q — the card would print \"No events on "+
				"this day\", no row would carry a `data-dc-edit` door, and every shot in the "+
				"set would be create mode with an edit-mode caption on it", want)
		}
	}
	// …and the player arm must lose the dm_only row through the SHIPPED filter.
	player := benchFxShotData(DayCardMount{CampaignID: "camp-1"})
	if strings.Contains(player.DayCardJSON, "evt-9c40") {
		t.Error("a player's capture payload still carries the dm_only event; the shot would " +
			"photograph a leak rather than the viewer filter")
	}
}

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

// TestDayCardShotSetCoversEditMode is ALWAYS ON, and it is the cheap check that
// the fix round's finding 2 cannot recur silently.
//
// FINDING 2, IN ONE SENTENCE: every shot in the rejected set ran the CREATE
// driver, and of the 22 gating stills 21 are edit-mode editors or player cards
// — so the by-eye comparison the dispatch's §10 item 1 asks for could not have
// been made for the state 21 of them are in. The chrome states that render ONLY
// in edit mode (the tie pill, `Delete event`, the `Save changes` footer, the
// populated recurrence row, the SELECTED Restricted card) were in no image.
//
// The rule is mechanical: the set must drive the row `Edit` door, at more than
// one viewer, and at more than one width — and the edit-mode driver must be the
// producer's own door rather than a state written into the page.
func TestDayCardShotSetCoversEditMode(t *testing.T) {
	viewers := map[string]bool{}
	widths := map[int]bool{}
	edit := 0
	for _, s := range daycardShotList() {
		if !strings.Contains(s.script, "data-dc-edit") {
			continue
		}
		edit++
		widths[s.w] = true
		viewers[fmt.Sprintf("%v/%v/%v", s.mount.CanAuthorDmOnly, s.mount.CanDelete, s.mount.CanRestrict)] = true
	}
	if edit == 0 {
		t.Fatal("no shot in the set drives `[data-dc-edit]` — the whole set is CREATE mode, " +
			"which is the state exactly ONE of the 22 gating stills is in. This is the fix " +
			"round's finding 2")
	}
	if len(viewers) < 3 {
		t.Errorf("edit mode is captured for %d viewer(s); §10 item 2 names GM / co-DM / Scribe, "+
			"and the Delete axis is only visible in edit mode because a draft has no id to "+
			"delete", len(viewers))
	}
	if len(widths) < 2 {
		t.Errorf("edit mode is captured at %d width(s); §10 item 2 names 1440 AND 390", len(widths))
	}
	// …and the stub must still be the ONE substitution it claims to be: a
	// single GET, every other verb refused. A stub that answered writes could
	// photograph a state no user produced.
	if !strings.Contains(daycardStubEditRecord, "REFUSED") ||
		!strings.Contains(daycardStubEditRecord, "status: 405") {
		t.Error("the edit-mode stub no longer refuses non-GET verbs; a capture could " +
			"photograph a save it did not make")
	}
	if !strings.Contains(daycardStubEditRecord, "STUBBED for this capture") {
		t.Error("the edit-mode stub no longer discloses itself on the shot's diagnostic " +
			"strip — the one fabrication in this directory must say so on the image")
	}
}

// TestDayCardShotSetPerformsTheDragRow is ALWAYS ON. §10 item 12 is a CAPTURE
// row and the rejected set neither performed it nor disclosed it as skipped —
// the behaviour was pinned in test/js/daycard_drag_create.test.mjs and the row
// was simply not mentioned again. A row that is neither done nor named is the
// same silence finding 3 is about.
func TestDayCardShotSetPerformsTheDragRow(t *testing.T) {
	spans := map[string]bool{}
	for _, s := range daycardShotList() {
		if !strings.Contains(s.script, "'pointerdown'") {
			continue
		}
		switch {
		case strings.Contains(s.script, "ONE-CELL"):
			spans["1"] = true
		case strings.Contains(s.script, "THREE-CELL"):
			spans["3"] = true
		case strings.Contains(s.script, "MONTH-SPANNING"):
			spans["month"] = true
		}
		// Every drag shot must carry its own Block-immutability verdict, or it
		// is a photograph of a highlight with no way to tell which mechanism
		// drew it — which is the whole of [ER-8] and of bound 4.
		if !strings.Contains(s.script, "BYTE-IDENTICAL") {
			t.Errorf("drag shot %s does not re-compare the Block's innerHTML; a picture of a "+
				"highlight cannot show whether a cell was marked", s.file)
		}
		if strings.Contains(s.script, "'pointerup'") {
			t.Errorf("drag shot %s dispatches pointerup; the run would be committed and the "+
				"frame would be the editor rather than the PREVIEW", s.file)
		}
	}
	for _, want := range []string{"1", "3", "month"} {
		if !spans[want] {
			t.Errorf("§10 item 12 names a 1-cell, a 3-cell and a month-spanning drag; the "+
				"%q span is captured nowhere and disclosed nowhere", want)
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
		`id="shot-fold"`,         // the strip exists…
		`.shot-strips{position:`, // …inside the one fixed footer…
		`class="shot-strips"`,    // …which is actually on the page
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
	// …and BOTH must be deferred past the task the doors were clicked in. Edit
	// mode opens from a promise; a report emitted in the click's own task
	// measured an editor that had not been created and burned "0px of 0px" onto
	// an image of a populated one.
	if strings.Index(page, "setTimeout(function () {") > strings.Index(page, "sc.scrollTop") {
		t.Error("the scroll and the fold report are not deferred past the door click; in " +
			"edit mode they would measure an editor that does not exist yet")
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
