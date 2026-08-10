// calendar_daycard.js — C-CALV4-DAYCARD (calendar-v4 round 2, slice R2-2a).
//
// THE POINTER-FIRST ANSWER. Clicking a day in a calendar-v4 Block already does
// one thing: it checks a visually-hidden radio and the generated ANSWER ladder
// filters the docked Ledger to that day. That answer is CSS-only, it is signed,
// and this module does not touch it. What it adds is the answer the operator
// went looking for and did not find — a card, at the day, listing the day.
//
// ── THE BOUNDARY: READ AND LISTEN, NEVER MUTATE ────────────────────────────
//
// This module may QUERY the Block's DOM ([data-day], [data-day-ord],
// [data-event-id], .lrow, [data-zone]) and may ATTACH LISTENERS to it. It may
// NOT insert a node inside .cal-block-host, NOT add or remove a class on
// anything inside it, and NOT animate anything inside it. The Block's interior
// no-JS / no-motion law is a widget-package law and this slice opens no file in
// that package; this is the mechanical form of the same rule one layer out, and
// test/js/daycard_block_immutability.test.mjs asserts a Block-shaped fixture's
// innerHTML is byte-identical before and after open + close.
//
// The ONE thing that looks like an exception and is not: the `Open in the
// Ledger` door calls .click() on the day's own radio. That ACTIVATES a shipped
// control exactly as a pointer would — it changes checkedness, which is IDL
// state and not a content attribute, so the serialised DOM is unchanged. The
// alternative (writing `checked` ourselves and dispatching a synthetic change)
// would be the module simulating the browser instead of using it.
//
// ── PERMISSION IS ABSENCE, AND `needs backend` IS SOMETHING ELSE ───────────
//
// Two rules that look identical in a screenshot and are not:
//
//   PERMISSION IS ABSENCE — data that EXISTS but this viewer is not entitled
//   to is not in their payload at all. Not greyed, not counted, not hinted.
//   The card cannot leak it because the producer never sent it.
//
//   `needs backend` — data that does not exist FOR ANYBODY yet. It is a
//   visible chip, it is never shown to a player, and this module renders none.
//
// ── THE EDITOR IS A CLIENT OF THE SHIPPED EVENT API ────────────────────────
//
// ZERO NEW WRITE ROUTES ([DC-8] SIGNED). Create is POST
// /campaigns/:id/calendars/:calId/events (Scribe), update is PUT
// .../events/:eid (Scribe), delete is DELETE .../events/:eid (Owner). All three
// shipped before this slice, all three are IDOR-closed at
// requireEventInCampaign / requireCalendarInCampaign, and all three ride
// Chronicle.apiFetch, which attaches X-CSRF-Token on every mutating method and
// sets credentials: same-origin. This module widens no request body: a route
// that grew a field it was not named for is a lie in a route name.
//
// THE ONE READ IT NEEDS is GET .../events/:eid, which this slice added and
// which is the whole route budget spent. Edit mode needs the event's full
// record — description, times, end date, category, visibility — and the card's
// page payload deliberately carries none of it.
//
// LOSSLESS BY CONSTRUCTION. The update path re-sends what it was given for
// every field it does not offer a control for. That is not tidiness: PUT
// re-writes the whole record, so an editor that dropped `visibility_rules`
// because it has no chip row yet would DESTROY an event's audience on the first
// save, silently, and the only visible symptom would be players seeing
// something they should not.
//
// ── MOTION IS THE REGISTER'S, AND ONLY THE REGISTER'S ──────────────────────
//
// decisions/2026-07-29-motion-disclosure-register.md, consumed from the
// register section BENCH-R2 landed in static/css/calendar-bench.css: one
// clip-reveal + opacity ramp, --disc-open (200ms) / --disc-close (160ms), one
// easing, close always faster than open. Under prefers-reduced-motion the state
// change is INSTANT AND COMPLETE — the sheet declares no rule at all outside
// the no-preference wrapper, and this module must not wait for an animation
// that will never fire, which is why closeDelayMS() returns 0 there.
(function () {
  'use strict';

  // ── Pure mappers (exposed for the node tests) ────────────────────────────

  // indexPayload turns the page attribute into { calendarId: { …, days: {} } }.
  //
  // DEFENSIVE BY CONSTRUCTION: a malformed or absent attribute yields an empty
  // index and every lookup misses, so the card simply never opens. It never
  // throws on a page it was mounted onto by mistake.
  function indexPayload(raw) {
    var out = {};
    if (!raw) return out;
    var parsed;
    try { parsed = JSON.parse(raw); } catch (e) { return out; }
    var cals = (parsed && parsed.calendars) || [];
    if (!Array.isArray(cals)) return out;
    cals.forEach(function (cal) {
      if (!cal || !cal.id) return;
      var days = {};
      (Array.isArray(cal.days) ? cal.days : []).forEach(function (d) {
        if (d && d.key) days[d.key] = d;
      });
      out[cal.id] = {
        id: cal.id,
        slug: cal.slug || '',
        ledgerDocked: !!cal.ledgerDocked,
        // THE ORDERED LIST, BESIDE THE MAP. The map is the card's hot path (one
        // lookup per click); the ORDER is the editor's — its date grid, its end
        // cycler and its week derivation all need the month's dates in the
        // Ledger's own ordinal order, which a map does not carry. Two views of
        // one array, never a second parse.
        list: (Array.isArray(cal.days) ? cal.days : []).slice(),
        // Scribe+ only, and absent entirely below that floor — the producer
        // simply does not emit it, so there is nothing here to gate.
        categories: Array.isArray(cal.categories) ? cal.categories : [],
        days: days,
      };
    });
    return out;
  }

  // indexMembers reads the Owner-only audience roster off the SAME page
  // attribute ([ER-3] SIGNED).
  //
  // IT IS A SECOND READ OF ONE ATTRIBUTE RATHER THAN A SECOND SHAPE FOR
  // indexPayload'S RETURN. Five test files drive indexPayload's `{calendarId:
  // {...}}` map and the day card's own hot path is that map; widening it to a
  // wrapper object to carry a key only the editor reads would make every one of
  // those call sites carry the editor's concern. The parse happens once per
  // page mount, not once per open.
  //
  // DEFENSIVE BY CONSTRUCTION, exactly as indexPayload is: an absent or
  // malformed attribute, or a payload with no `members` key at all (which is
  // what every viewer below the Owner floor receives), yields an empty roster
  // and the audience list simply has no rows to draw.
  function indexMembers(raw) {
    if (!raw) return [];
    var parsed;
    try { parsed = JSON.parse(raw); } catch (e) { return []; }
    var list = parsed && parsed.members;
    if (!Array.isArray(list)) return [];
    return list.filter(function (m) { return m && m.id; });
  }

  // headText is the card's dated head: "12 Deepwinter 1523 · Sixthday".
  // The weekday segment DROPS rather than printing a dangling separator — the
  // same rule the Ledger row follows for its meta line and its time.
  function headText(day) {
    if (!day) return '';
    var label = day.label || '';
    return day.weekday ? label + ' · ' + day.weekday : label;
  }

  // durationMS parses a CSS time token. The card reads its own close duration
  // off --disc-close rather than carrying a copy of 160, so there is exactly
  // one place the register's two durations live.
  function durationMS(value, fallback) {
    if (value === null || value === undefined) return fallback;
    var m = /^\s*([0-9]*\.?[0-9]+)(ms|s)?\s*$/.exec(String(value));
    if (!m) return fallback;
    var n = parseFloat(m[1]);
    if (!isFinite(n)) return fallback;
    return m[2] === 's' ? n * 1000 : n;
  }

  // closeDelayMS — REDUCED MOTION IS A BRANCH, NOT A SCALE. Under
  // prefers-reduced-motion the sheet declares no transition at all, so there is
  // nothing to wait for and waiting would leave a fully-styled card sitting on
  // screen for 160ms after it was logically closed.
  function closeDelayMS(cssClose, reduced) {
    if (reduced) return 0;
    return durationMS(cssClose, 160);
  }

  // placeCard positions the card from the clicked cell's rect.
  //
  // THE LEDGER IS NEVER OCCLUDED. The card's own `Open in the Ledger` door
  // points AT that column, so covering it would be the card contradicting
  // itself — and since calv4 fix R1 item 3 the exclusion is the WIDER of the two
  // rules rather than the narrower: the door now renders only on a STACKED
  // Ledger (see ledgerIsStacked), but a DOCKED Ledger is repainted by the same
  // click that opened the card, so covering it would hide the answer the reader
  // just asked for. Both layouts keep the hard exclusion, for the two reasons. The LEDGER'S SETTLED RECT IS A HARD EXCLUSION ZONE for the desktop
  // popover, whether that rect is a docked right-hand column or a full-width
  // band the narrow layout has STACKED BELOW THE GRID.
  //
  // THE DODGE IS BOTH AXES (fix-forward, DC3-STACKED-LEDGER-OCCLUSION-1). The
  // first two cuts dodged HORIZONTALLY ONLY — `limit = ledger.left - pad -
  // size.w`, taken when the box shared a Y band with the column — and that
  // dodge is structurally impossible against a STACKED Ledger, because a band
  // that starts at x≈9 and spans the whole Bench leaves no left of it to move
  // to (`limit` came out at ≈-339 and the clamp no-opped). The `top` branch
  // above it only ever flipped for VIEWPORT room, never for the Ledger, so the
  // 227px card — which always fits below its day — landed squarely on the band
  // for EVERY day, EVERY viewer, at every .cal-bench content width from ~625px
  // to ~884px. Measured: 23,131 px² over the GM's Ledger at 884px, sheet=false.
  // The editor escaped only by accident (it is 400px tall, does not fit below,
  // and its viewport flip happened to clear the band) — which is the proof that
  // a vertical dodge was available at those widths and simply not attempted.
  //
  // So the order is: BELOW the day (preferred), then ABOVE it when below cannot
  // clear, then — if neither position clears at popover width — the SHEET, which
  // is [DC-3] bullet 4's own signed answer. There are TWO ANCHORED CANDIDATES
  // AND ONE SHEET, and the card is never resized to fit: a card that shrank to
  // dodge would be a different card, and [DC-3] signed one card and one sheet.
  //
  // THE ABOVE CANDIDATE IS CLAMPED INTO THE VIEWPORT, NEVER DROPPED
  // (fix-forward, DC3-DESKTOP-SHEET-OCCLUSION-R4). The cut that added the
  // vertical dodge admitted `above` only `if (above >= pad)` and pushed
  // everything else onto a `view.h - pad - size.h` clamp that pins the box to
  // the BOTTOM of the viewport — which is precisely where a STACKED Ledger band
  // lives. So a TALL box never flipped at all: the 400px editor under a mid-grid
  // day computes `above` at ≈-64, was dropped, was bottom-pinned, failed the
  // clear test in both candidates and fell to the desktop SHEET — 107,604 px²,
  // 100% of the Ledger, at 884px, on `+ New event` with no unusual data at all.
  // A card with 12+ events on one day (≈379px, ordinary festival data) reached
  // the identical fallback. In every one of those cases A CLEAR POPOVER POSITION
  // EXISTED: a 481px box at top=8 ends at 489, entirely above a band whose top
  // is 595. The module's warning therefore said the placement was impossible
  // when it had merely not been ATTEMPTED, which is the one thing [DC-3]'s
  // STOP-AND-FLAG must never say falsely.
  //
  // The fix is a CLAMP ON THE EXISTING FLIP, not a third geometry: `above` still
  // means "start the box above the day", and where the box is taller than the
  // room above its day it starts at the viewport's top edge instead of
  // off-screen. That is the same clamping the below-candidate has always had,
  // applied in the direction the above-candidate actually prefers — UPWARD. It
  // is also, exactly, the accidental placement the pre-dodge code produced for
  // the editor at y=28 and which measured clear at every width. The sheet
  // fallback stays, unweakened, for the geometry that genuinely runs out.
  //
  // `clear` IS MEASURED, NEVER ASSERTED, AND IT IS MEASURED THE SAME WAY IN
  // BOTH BRANCHES (fix-forward, DC-CLEAR-1 / DC-MOBILE-4). The first cut
  // computed it only on the desktop path, from a Y-overlap test taken BEFORE
  // the left clamp, and hard-coded `clear: true` on the sheet path without ever
  // consulting the Ledger's rect — which was wrong by 54,260 px² at 390px,
  // where the sheet sits over a Ledger the narrow layout has stacked below the
  // grid. Both branches now finish by intersecting the PLACED box against the
  // Ledger's actual rect, so the flag reports where the box landed rather than
  // what the placement intended.
  //
  // `sheet` travels with it because the two facts get different treatments and
  // the caller is the only place that can tell them apart. A POPOVER over the
  // Ledger is [DC-3]'s STOP-AND-FLAG and it speaks. A SHEET over the Ledger is
  // [DC-3] bullet 4's own signed treatment — below the breakpoint because that
  // is the layout, and above it because the sheet is what the dodge falls back
  // to — and §12 scopes the STOP-AND-FLAG row to 1232px, so it is recorded and
  // stays quiet. Both are on the DOM; only the unsigned one warns.
  //
  // MOBILE IS A BOTTOM SHEET, still [popover], still the register's motion.
  function placeCard(anchor, size, view, ledger, opts) {
    opts = opts || {};
    var pad = opts.pad === undefined ? 8 : opts.pad;
    var breakpoint = opts.breakpoint === undefined ? 640 : opts.breakpoint;

    if (view.w <= breakpoint) return sheetPlacement(size, view, ledger, false);

    // The two vertical candidates, in preference order.
    //
    // BELOW keeps its viewport FILTER: a card that would hang off the bottom of
    // the window is not "below its day" in any useful sense, and dropping it
    // here is what lets the flip take over at the foot of the grid.
    //
    // ABOVE takes a viewport CLAMP instead of a filter, for the reason in the
    // header comment: dropping it is what sent every tall box to the sheet while
    // a clear position existed. `lowClamp` is the lowest a box of this height may
    // sit and still fit; `pad` is the highest. A box taller than the whole
    // viewport collapses both to `pad`, which is top-pinned — the right end to
    // pin against a Ledger the narrow layout stacks BELOW the grid.
    var tops = [];
    var below = anchor.bottom + pad;
    var lowClamp = Math.max(pad, view.h - pad - size.h);
    var above = Math.min(Math.max(anchor.top - pad - size.h, pad), lowClamp);
    if (below + size.h <= view.h - pad) tops.push(below);
    if (!tops.length || above !== tops[0]) tops.push(above);

    for (var i = 0; i < tops.length; i++) {
      var at = desktopPlacement(tops[i], anchor, size, view, ledger, pad);
      if (at.clear) return at;
    }
    return sheetPlacement(size, view, ledger, true);
  }

  // desktopPlacement resolves the horizontal axis for one vertical candidate and
  // measures the result. The left dodge stays exactly as it was — it is the
  // right answer for a DOCKED column and it is what keeps the card beside the
  // Ledger rather than above the day at ordinary widths.
  function desktopPlacement(top, anchor, size, view, ledger, pad) {
    var left = anchor.left;
    if (left + size.w > view.w - pad) left = view.w - pad - size.w;
    if (ledger && ledger.width > 0 && overlapsY(top, size.h, ledger)) {
      var limit = ledger.left - pad - size.w;
      if (limit >= pad) left = Math.min(left, limit);
    }
    if (left < pad) left = pad;
    return {
      left: left, top: top, width: 0, sheet: false,
      clear: !hitsLedger({ left: left, top: top, width: size.w, height: size.h }, ledger),
    };
  }

  // sheetPlacement is the full-width bottom sheet — [DC-3] bullet 4's signed
  // treatment. It is reached from TWO conditions and they are different facts,
  // which is what `fallback` carries:
  //
  //   below the mobile breakpoint the sheet is THE LAYOUT. §12 scopes the
  //   STOP-AND-FLAG row to 1232px and DC2-MOBILE-6 accepted the overlap there as
  //   signed, so it is recorded and stays quiet.
  //
  //   at desktop width the sheet is the LAST RESORT, taken because neither
  //   vertical candidate could clear the Ledger. The card no longer covers the
  //   column as a popover — but the geometry still RAN OUT, and that is exactly
  //   the condition [DC-3] signed as a STOP-AND-FLAG. Retiring the warning along
  //   with the harm would quietly un-sign it, so the fallback speaks.
  //
  // `clear` is measured the same way in both, so the report never flatters the
  // placement.
  // ── THE SHEET'S GEOMETRY LEFT JAVASCRIPT (C-CALV4-MOBILE [MOB-2] SIGNED) ──
  //
  // This function used to return an APPLIED top and an APPLIED width, and
  // applyPlacement wrote both onto the element as inline style. They were
  // computed ONCE, at open time, and never again — so the moment a software
  // keyboard shrank the layout viewport (390x664 -> 390x380, measured) the box
  // stayed at `top: 106px` and 84px of it, including the entire footer with
  // Save in it, sat below the fold of a `position: fixed` box that does not
  // scroll with the page. A rotation was the same bug wearing a second hat:
  // `width: 390px` survived onto an 844px viewport.
  //
  // The sheet is now `inset-block-end: 0; inset-inline: 0; inline-size: 100%;
  // max-block-size: 100dvh` in calendar-daycard.css, which the browser
  // re-resolves on every viewport change without being asked. So this function
  // no longer describes where the box WILL BE PUT; it describes where the box
  // WILL LAND, for the sake of the one consumer that still needs to know.
  //
  // AND THAT CONSUMER IS [DC-3]'s HONESTY CHANNEL, WHICH IS WHY THE RECT IS
  // RE-DERIVED RATHER THAN DELETED. `clear` is the STOP-AND-FLAG [DC-3]
  // signed. Retiring the warning along with the pixel is how a signature gets
  // un-signed quietly, so the rect it intersects against the Ledger is the box
  // CSS now renders — bottom-anchored, full-width, clamped to the viewport —
  // rather than the box JS used to write. The two agree at every size the old
  // code could reach and disagree only where the old code was wrong (a sheet
  // taller than the viewport used to be reported at top 0 with its full
  // height; it is now reported clamped, which is what the user sees).
  //
  // `applied: false` is the contract with applyPlacement: this placement has
  // no inline geometry to write.
  function sheetPlacement(size, view, ledger, fallback) {
    var h = Math.min(size.h, view.h);
    var top = Math.max(0, view.h - h);
    return {
      left: 0, top: top, width: view.w, height: h, sheet: true, applied: false,
      fallback: !!fallback,
      clear: !hitsLedger({ left: 0, top: top, width: view.w, height: h }, ledger),
    };
  }

  function overlapsY(top, height, rect) {
    return top < rect.top + rect.height && top + height > rect.top;
  }

  // hitsLedger is the one intersection test, shared by both branches so the two
  // placements cannot drift into two different definitions of "clear". No
  // Ledger rect, or a zero-width one, is not an occlusion — it is the case
  // where the card is the ONLY answer, which is the half of the operator's
  // complaint the CSS-only ladder cannot serve at all.
  function hitsLedger(box, ledger) {
    if (!ledger || !(ledger.width > 0) || !(ledger.height > 0)) return false;
    return box.left < ledger.left + ledger.width &&
      box.left + box.width > ledger.left &&
      box.top < ledger.top + ledger.height &&
      box.top + box.height > ledger.top;
  }

  // ── the occlusion report, and its consumer ─────────────────────────────
  //
  // placeCard's `clear` had no reader in the first cut (DC-CLEAR-1). The
  // comment above it and the stage-1 commit body both said an unclearable
  // geometry would be "visible rather than silent", and neither position() nor
  // edPosition() so much as looked at the flag — so the one condition [DC-3]
  // signed as a STOP-AND-FLAG shipped as the quietest thing on the page. These
  // two functions are the reader. A sentence promising a guard that never ran
  // is the exact defect the previous slice's stage 9 existed to kill, and it
  // does not get to reappear one slice later.
  //
  // TWO MECHANISMS, DELIBERATELY, because the condition has two audiences:
  //
  //   data-dc-clear="0|1" on the card's own root — the MEASURABLE fact, always
  //   written, on every placement, at every width. §12's screenshot gate reads
  //   a rendered attribute rather than re-deriving geometry from two rects, and
  //   a regression that starts covering the Ledger flips a value a test can see.
  //   It is a report, not a state: no stylesheet may style it, because [DC-3]
  //   makes occlusion a build-time flag and not a UI mode somebody designed.
  //
  //   ONE console warning per session — the DEVELOPER-facing half. It fires for
  //   a POPOVER that could not clear (which, after
  //   DC3-STACKED-LEDGER-OCCLUSION-1's two-axis dodge, placeCard no longer
  //   produces — it is kept as the alarm if a future placement path
  //   reintroduces one) and for the DESKTOP SHEET FALLBACK, which is the
  //   geometry-ran-out condition [DC-3] actually signed. It stays SILENT for the
  //   mobile sheet, where the full-width treatment is the layout rather than a
  //   fallback and §12 scopes the STOP-AND-FLAG row to 1232px (DC-MOBILE-4);
  //   warning there would train the next hand to ignore the one that matters.
  //   One per session, not one per placement: the card repositions on every
  //   open, and a warning that fires sixty times is a warning nobody reads.
  function occlusionReporter(sink) {
    var fired = false;
    return function (at) {
      if (!at || at.clear || fired) return false;
      if (at.sheet && !at.fallback) return false;
      fired = true;
      if (sink) {
        sink('calendar day card [C-CALV4-DAYCARD/DC-3]: this geometry cannot ' +
          'place the card clear of the docked Ledger, so the card is covering ' +
          'the column its own "Open in the Ledger" door points at.');
      }
      return true;
    };
  }

  // applyPlacement writes a placement onto a box. Both the card and the editor
  // go through it so the two cannot acquire two different ideas of what a
  // placement means — the editor's own occlusion was invisible for exactly the
  // reason the card's was, and one writer fixes both.
  //
  // The width is CLEARED when the box is not a sheet. A viewport that crosses
  // the breakpoint downward and back left the sheet's inline `width` behind,
  // pinning a desktop card to the phone's width until reload.
  // THE SHEET ARM WRITES NO GEOMETRY AT ALL (C-CALV4-MOBILE [MOB-2] SIGNED).
  // `.dcsheet` owns the box — bottom-anchored, full-width, clamped to 100dvh —
  // so the three inline properties are CLEARED rather than set. Clearing is
  // not optional housekeeping: an inline `top` left behind by a previous
  // popover placement outranks the class's `inset-block-start: auto` and the
  // sheet would hang in mid-air at the old number, which is the same stale-
  // pixel defect this ruling exists to close.
  function applyPlacement(el, at, report) {
    if (!el || !at) return at;
    if (at.applied === false) {
      el.style.left = '';
      el.style.top = '';
      el.style.width = '';
    } else {
      el.style.left = at.left + 'px';
      el.style.top = at.top + 'px';
      el.style.width = '';
    }
    el.classList.toggle('dcsheet', !!at.sheet);
    el.setAttribute('data-dc-clear', at.clear ? '1' : '0');
    if (report) report(at);
    return at;
  }

  // ── THE PAGE LOCK, AND THE RELEASE RULED HARDER THAN THE LOCK ────────────
  //    C-CALV4-MOBILE [MOB-3] SIGNED.
  //
  // MEASURED, with the day card open AND with the editor open, at 390x664,
  // 360x640 and 390x844 — six arms, six identical results:
  //
  //     window.scrollBy(0, 400)  ->  document.scrollingElement.scrollTop 0 -> 400
  //     computed overflow on <html> and <body>: "visible"
  //
  // Because the sheet is `position: fixed` it stays pinned while the calendar
  // behind it scrolls away — so the card can end up describing a day that is
  // no longer on screen. No lock code existed; `document.body.style` was
  // written in exactly two places, both `user-select` for the drag, which is
  // why touching body.style here is not a new boundary crossing.
  //
  // THE `position: fixed` FORM, NOT `overflow: hidden` ON <html>. The audit
  // offered both and noted the second is only safe "if iOS 16 support is not
  // needed". The standing ruling picks the one that works on every phone that
  // could be at the table; the extra code is a stored integer.
  //
  // ONE DERIVED BOOLEAN, NOT TWO LOCKS AND NOT A COUNTER. The card closes AS
  // the editor opens ([DC-7]), so two independent locks race and one of them
  // wins, and a reference count that goes negative leaves a phone on a page
  // that will not scroll with no visible cause and no way out but a reload. A
  // SET is idempotent and cannot go negative: each sheet records whether IT is
  // open and the lock is `card || editor`, recomputed on every transition.
  //
  // THE STATE LIVES HERE, NOT ON THE SHEET. `edHide` does
  // `ed.root.removeAttribute('style')` — a full style wipe — so anything
  // parked on the element is gone before the release could read it.
  //
  // A LOCK NEVER RELEASED IS THE WORSE BUG. The condition it fixes (a page
  // that scrolls when it should not) is survivable; the failure mode of a bad
  // fix is not. Every exit path is proven by
  // TestMobileProbe_ThePageIsLockedBehindASheetAndReleasedOnEveryExit.
  var pageLock = { on: false, y: 0, prev: null, open: { card: false, editor: false } };

  // sheetOpenChanged is the ONE entry point. `which` is 'card' or 'editor'.
  function sheetOpenChanged(which, open) {
    pageLock.open[which] = !!open;
    setPageLock(pageLock.open.card || pageLock.open.editor);
  }

  function setPageLock(want) {
    if (typeof document === 'undefined' || !document.body || !document.body.style) return;
    if (!!want === pageLock.on) return;
    var b = document.body.style;
    if (want) {
      pageLock.y = (typeof window !== 'undefined' && window.pageYOffset) ||
        (document.scrollingElement ? document.scrollingElement.scrollTop : 0) || 0;
      // EVERY property this lock writes is remembered, so the release restores
      // what was there rather than what this module assumes was there.
      pageLock.prev = {
        position: b.position, top: b.top, left: b.left,
        right: b.right, width: b.width,
      };
      b.position = 'fixed';
      b.top = (-pageLock.y) + 'px';
      b.left = '0px';
      b.right = '0px';
      b.width = '100%';
      pageLock.on = true;
      return;
    }
    pageLock.on = false;
    var prev = pageLock.prev || {};
    b.position = prev.position || '';
    b.top = prev.top || '';
    b.left = prev.left || '';
    b.right = prev.right || '';
    b.width = prev.width || '';
    pageLock.prev = null;
    // THE OFFSET COMES BACK. Without this the page snaps to the top on every
    // close, which is a second defect wearing the first one's clothes.
    if (typeof window !== 'undefined' && window.scrollTo) window.scrollTo(0, pageLock.y);
  }

  // ordIsSafe gates the one attribute-selector interpolation in this module.
  // The ordinal comes from our own payload and is "12" or "i1", never anything
  // else — but a selector built from data is exactly where that stops being
  // true one slice later.
  function ordIsSafe(ord) {
    return typeof ord === 'string' && /^i?[0-9]+$/.test(ord);
  }

  // numOrNull turns a form field into an API value. EMPTY IS NULL, NOT ZERO:
  // hour 0 is midnight and "no time" is the absence of a value, so a blank
  // field that resolved to 0 would silently schedule every untimed event at
  // midnight.
  function numOrNull(raw) {
    if (raw === null || raw === undefined) return null;
    var s = String(raw).trim();
    if (s === '') return null;
    var n = Number(s);
    return isFinite(n) ? n : null;
  }

  // buildEventBody assembles the create/update body from the form's own values
  // plus the record the editor opened on.
  //
  // `prev` is the stored record in EDIT mode and null in CREATE mode, and it is
  // what makes the write lossless: every field the editor has no control for is
  // sent back exactly as it arrived. In create mode there is nothing to
  // preserve, so those fields are simply absent.
  //
  // VISIBILITY IS THE SHARED MAPPER'S ([DC-10] SIGNED), never a local copy.
  // When the GM-only control is not rendered — the viewer lacks
  // CanAuthorDmOnly, or the event carries an audience restriction this stage
  // has no chip row for — the stored pair round-trips untouched.
  function buildEventBody(form, prev, opts) {
    opts = opts || {};
    var body = {
      name: form.name || '',
      description: form.description ? form.description : null,
      year: numOrNull(form.year),
      month: numOrNull(form.month),
      day: numOrNull(form.day),
      all_day: !!form.allDay,
      category: form.category ? form.category : null,
      start_hour: form.allDay ? null : numOrNull(form.startHour),
      start_minute: form.allDay ? null : numOrNull(form.startMinute),
      end_hour: form.allDay ? null : numOrNull(form.endHour),
      end_minute: form.allDay ? null : numOrNull(form.endMinute),
      end_year: numOrNull(form.endYear),
      end_month: numOrNull(form.endMonth),
      end_day: numOrNull(form.endDay),
    };

    // ── VISIBILITY ─────────────────────────────────────────────────────────
    //
    // EXTENDED, NOT REPLACED, by C-CALV4-EDITOR-R2b stage 2. The stage-2 rule
    // was: author public-vs-dm_only when the viewer has the capability and the
    // stored mode is not `specific`, else round-trip. Every one of those cases
    // still resolves identically — `form.vis` is ABSENT on the pure-function
    // call sites the existing suite drives, so `mode` falls back to
    // `form.gmOnly` exactly as before ([ER-10] condition 2: the request body is
    // byte-equivalent for equivalent input).
    //
    // What is NEW is the third mode. `restricted` is Owner-only, so it is
    // authored only under `opts.canRestrict`, and the ROUND-TRIP GUARD IS
    // UNCHANGED IN THE DIRECTION THAT MATTERS: a stored `specific` audience is
    // re-authored only by someone who can author restricted. A co-DM saving a
    // restricted event still round-trips the audience they were never shown —
    // dropping it would DESTROY the audience on the first save, silently, and
    // the only visible symptom would be players seeing something they should
    // not.
    //
    // THE MAPPING ITSELF IS THE SHARED MAPPER'S ([DC-10] SIGNED), never a local
    // copy: a fourth copy of those fifteen lines is the one nobody notices
    // going stale.
    var V = (typeof window !== 'undefined' && window.ChronicleCalVisibility) || null;
    var storedMode = prev && V ? V.modeFor(prev.visibility, prev.visibility_rules) : 'public';
    var mode = form.vis || (form.gmOnly ? 'gmonly' : 'public');
    var mayAuthor =
      (mode === 'gmonly' && !!opts.canOfferGMOnly) ||
      (mode === 'restricted' && !!opts.canRestrict) ||
      (mode === 'public' && (!!opts.canOfferGMOnly || !!opts.canRestrict));
    if (storedMode === 'specific' && !opts.canRestrict) mayAuthor = false;
    if (mayAuthor && V) {
      var mapped = V.buildVisibilityPayload(
        mode === 'restricted' ? 'specific' : mode,
        mode === 'restricted' ? (form.audience || []) : []);
      body.visibility = mapped.visibility;
      body.visibility_rules = mapped.visibility_rules;
    } else if (prev) {
      // ROUND-TRIP, NOT DEFAULT. Anything else rewrites an audience the editor
      // never showed anybody.
      body.visibility = prev.visibility || 'everyone';
      body.visibility_rules = prev.visibility_rules === undefined ? null : prev.visibility_rules;
    } else {
      body.visibility = 'everyone';
      body.visibility_rules = null;
    }

    // RECURRENCE IS SENT BACK, NOT LEFT OUT. The editor does not author it in
    // this stage, and OMISSION IS NOT PRESERVATION here: `is_recurring` is a
    // value-typed bool on the shipped PUT and service.UpdateEvent writes it
    // unguarded on purpose ("false IS the value, not 'absent'"), so a body
    // without the key un-repeats the event — and the nil-guarded
    // recurrence_type/interval/end_* survive, leaving exactly the half-state
    // C-CAL-RECURRING-PARTIAL-STATE-CLEANUP already had to clean up once.
    // A title-only save is the likeliest way to hit it.
    //
    // This is the same round-trip discipline the visibility pair above rides,
    // applied to the one field where an ABSENT key is a WRITE. Create mode
    // sends nothing: a new event has no stored recurrence and false is then
    // the true value rather than a silent clear.
    //
    // R2b AUTHORS IT, so the round trip becomes an AUTHOR-vs-OMIT distinction
    // rather than a pure round trip, and the three cases are pinned by name in
    // test/js/daycard_editor_requests.test.mjs:
    //
    //   AUTHORED   → recurrenceBody's mapping is sent (every 1 → weekly,
    //                every 2 → biweekly, every N → custom + interval N;
    //                month → monthly with no interval, because this editor
    //                does not offer the field for the month unit — see
    //                recurrenceInterval; OccursOn honours one since stage 22)
    //   UNTOUCHED  → the stored triple is round-tripped exactly as stage 2 did.
    //                `form.recurrence` is ABSENT on every existing pure-function
    //                case, so those bodies are byte-identical
    //   ONCE       → is_recurring:false AND the type and interval cleared
    //                TOGETHER, never the half-state
    //
    // A CHIPPED UNIT AUTHORS NOTHING. recurrenceBody returns null for `day` and
    // the moon unit, so the branch below falls through to the round trip rather
    // than writing a type the server would silently degrade to one occurrence.
    var authored = form.recurrence ? recurrenceBody(form.recurrence) : null;
    if (authored) {
      body.is_recurring = authored.is_recurring;
      body.recurrence_type = authored.recurrence_type;
      body.recurrence_interval = authored.recurrence_interval;
    } else if (prev) {
      body.is_recurring = !!prev.is_recurring;
      if (prev.recurrence_type !== undefined && prev.recurrence_type !== null) {
        body.recurrence_type = prev.recurrence_type;
      }
      if (prev.recurrence_interval !== undefined && prev.recurrence_interval !== null) {
        body.recurrence_interval = prev.recurrence_interval;
      }
    }
    // THE PRIMARY TIE IS AUTHORED WHEN THE FIELD EXISTS AND ROUND-TRIPPED WHEN
    // IT DOES NOT. `form.entityID` is emitted by edFormValues on every DOM
    // path, so a cleared pill really clears `entity_id`; the pure-function
    // cases that pass no `entityID` at all keep stage 2's round trip exactly
    // ([ER-10] condition 2). `+ tie another` and the multi-tie routes are
    // BOOKED, not shipped — [ER-4] SIGNED makes the single tie with remove the
    // sanctioned shape when the wider list is not proven.
    if (form.entityID !== undefined) body.entity_id = form.entityID || null;
    else if (prev && prev.entity_id) body.entity_id = prev.entity_id;
    if (prev && prev.description_html !== undefined) body.description_html = prev.description_html;
    return body;
  }

  // writeTarget is the ONE place that decides PUT-vs-POST
  // (EDIT-MODE-ID-FALLBACK-3). It is pure and exported so the decision can be
  // pinned directly rather than inferred from whichever request happened to
  // reach a stub: an edit session that has lost its id must REFUSE, because the
  // alternative — falling through to the create branch — silently duplicates
  // the event and shows nothing but an extra row.
  function writeTarget(mode, eventID) {
    if (mode === 'edit') {
      if (!eventID) return null;
      return { method: 'PUT', eventID: String(eventID) };
    }
    return { method: 'POST', eventID: '' };
  }

  // dayRange is STOLEN BY COPY from calendar_v2's event_grid.js:27, with
  // attribution, exactly as [DC-10] SIGNED sanctions ([DC-11] term 4's
  // sibling). It is re-expressed over the ordered DATE LIST rather than over
  // raw day numbers, because this surface has an intercalary day and that day
  // is not "day 31": on the list it is simply the last entry, so a run that
  // ends on it is a run like any other.
  //
  // COPYING FROM A FROZEN FILE IS NOT OPENING IT. event_grid.js is not edited,
  // not imported and not depended on in any way — R2-4 sunsets it and a
  // dependency would die with it.
  function dayRange(dates, aKey, bKey) {
    var a = -1, b = -1;
    for (var i = 0; i < dates.length; i++) {
      if (dates[i].key === aKey) a = i;
      if (dates[i].key === bKey) b = i;
    }
    if (a < 0 || b < 0) return [];
    if (a > b) { var t = a; a = b; b = t; }
    return dates.slice(a, b + 1);
  }

  // ── THE CHROME'S PURE MAPPERS — C-CALV4-EDITOR-R2b stage 2 ──────────────

  // isIntercalary reads the ANSWER key's own namespace rather than a flag.
  //
  // The payload mints `slug-N` / `N` for an ordinary day and `slug-iN` / `iN`
  // for an intercalary one (daycard.go's two mirrored helpers), so the
  // distinction is already on the wire and a `intercalary: true` field would be
  // a NINTH payload key restating it. [DC-1]'s law is not widened to say a
  // thing the key already says.
  function isIntercalary(day) {
    return !!day && typeof day.ord === 'string' && day.ord.charAt(0) === 'i';
  }

  // weekShape DERIVES the calendar's week from the payload's own weekday names.
  //
  // THERE IS NO LITERAL WEEK LENGTH ANYWHERE IN THIS MODULE, THE TEMPLATE OR
  // THE SHEET, and this function is why. Ten-day weeks are native to this
  // product; a `% 7` here is the exact defect css_contract_test.go forbids one
  // layer up. The producer already places every day in its own column and names
  // that column's weekday, so the cycle length is the number of names before
  // the first one comes round again — read off shipped data rather than added
  // to the payload as a tenth field.
  //
  // A calendar that declares NO weekdays returns a length of 0, and every
  // consumer treats that as "there is no week here": the grid flows instead of
  // gridding, and the week-based recurrence unit does not ship at all — which
  // is correct, because WeekLength() 0 makes the server's own stride 0 and
  // OccursOn falls back to a single occurrence.
  function weekShape(list) {
    var ordinary = (list || []).filter(function (d) {
      return d && !isIntercalary(d) && d.weekday;
    }).slice().sort(function (a, b) { return (a.day || 0) - (b.day || 0); });
    var names = [];
    for (var i = 0; i < ordinary.length; i++) {
      var n = ordinary[i].weekday;
      if (names.indexOf(n) >= 0) break;
      names.push(n);
    }
    return { len: names.length, names: names };
  }

  // orderedDates is the month's dates in the LEDGER'S OWN ORDINAL ORDER:
  // ordinary days ascending, then the intercalary days.
  //
  // THE INTERCALARY DAY SORTS LAST, and that single fact is what makes the end
  // date unable to precede its start. The producer already emits the list in
  // this order (buildDayCardCalendar walks the grid then the intercalary
  // months, which is newLedgerView's own walk), so this re-derives nothing — it
  // defends against an index that lost the order on the way through a map.
  function orderedDates(list) {
    var ord = [], ic = [];
    (list || []).forEach(function (d) {
      if (!d || !d.key) return;
      (isIntercalary(d) ? ic : ord).push(d);
    });
    ord.sort(function (a, b) { return (a.day || 0) - (b.day || 0); });
    return ord.concat(ic);
  }

  // nextDate is the `Ends` cycler's whole ordering law: the next date AFTER the
  // given one, on the ordered list, or null when there is no next one.
  //
  // IT COMPARES ORDINALS OFF ONE ORDERED LIST rather than arithmetic on `day`.
  // The drawing lane learned this the hard way: taking `st.day === 'ic' ? days
  // : st.day` as the base made an intercalary START behave like day 30, so the
  // end clamped BACKWARDS to 30 while the readout said Midwinter. On this list
  // the intercalary day is simply the last entry, so a start on it has no next
  // date and the field says "ends the same day" instead of clamping.
  function nextDate(dates, key) {
    if (!key) return (dates && dates.length) ? dates[0] : null;
    for (var i = 0; i < dates.length; i++) {
      if (dates[i].key === key) return dates[i + 1] || null;
    }
    return null;
  }

  // recurrenceUnits is THE CORRECTED UNIT LIST, and it is corrected in three
  // directions the drawing got wrong.
  //
  // `recurrence_type` accepts exactly weekly · biweekly · monthly · custom ·
  // yearly (model.go's Recurrence* constant block) and OccursOn sends anything
  // else to `default: return onBase`. A WRONG UNIT IS NOT AN ERROR, IT IS A
  // SILENT SINGLE OCCURRENCE — which is the failure a chip exists to prevent,
  // so the chip has to land on exactly the right units. Since
  // C-CALV4-GAMEREADY §6 [GR-12] the SERVER also refuses an unsupported type
  // with a 400 rather than a 201, but that is a backstop for integrations; this
  // list is still the only thing standing between the GM and a wrong unit,
  // because a rejected save is a worse table experience than an absent option.
  //
  //  1. THE WEEK UNIT IS NOT INVENTION AND ITS CHIP COMES OFF. Week-based
  //     recurrence strides `WeekLength() × recurrenceWeeks(...)` (model.go:336,
  //     :351-361), so on a ten-day calendar `weekly` MEANS every tenday. §5 of
  //     DAYCARD and the mockup both chip this unit and both are wrong.
  //  2. `year` WAS INVENTION AND IS NOW BACKED — CORRECTED HERE, in the same
  //     commit that built it (C-CALV4-GAMEREADY §6, [GR-11]). This note used to
  //     read "There is no yearly type; it degrades silently. It does not ship
  //     at all", and it was RIGHT when it was written: OccursOn had four types
  //     and dropped everything else, so the drawing's unchipped `year` would
  //     have been the exact trap this list exists to prevent. `RecurrenceYearly`
  //     now expands, so the unit ships `backed: true` — and the stale sentence
  //     goes with it, because a comment asserting an absence the product no
  //     longer has is how the next hand re-derives the gap. A festival, a holy
  //     day, a birthday and a coronation anniversary are the most common
  //     recurring things in a fantasy calendar, and until §6 the editor was
  //     honestly refusing to offer a unit the engine could not keep.
  //  3. THE LABEL IS DERIVED, NEVER A LITERAL. Chronicle's Calendar carries
  //     `Weekdays` and a `WeekLength()` and NO WEEK NOUN AT ALL, so there is no
  //     "the calendar's own week noun" to read: the honest derived label names
  //     the cycle's length, which cannot lie about the stride the way a bare
  //     "week" does on a ten-day calendar and cannot hardcode "tenday" either.
  //
  // `day` and the moon unit stay chipped, exactly as drawn — neither is an
  // accepted type. The moon unit is labelled generically because the payload
  // carries no moon to name; the drawing's "Umber full moon" reads a moon out
  // of its own demo data and adding one here would be a payload field for a
  // control that is chipped precisely because it does not work.
  function recurrenceUnits(weekLen) {
    var out = [];
    if (weekLen > 0) out.push({ id: 'week', label: weekLen + '-day week', backed: true });
    out.push({ id: 'month', label: 'month', backed: true });
    out.push({ id: 'year', label: 'year', backed: true });
    out.push({ id: 'day', label: 'day', backed: false });
    out.push({ id: 'moon', label: 'moon phase', backed: false });
    return out;
  }

  // recurrenceInterval says whether the `every [N]` field applies at all.
  //
  // ONLY THE WEEK UNIT HAS AN INTERVAL — IN THIS FILE, AND NO LONGER IN THE
  // BACKEND. R2-2b withheld the field from the month unit because OccursOn's
  // monthly branch ignored RecurrenceInterval entirely: it checked the
  // day-of-month and the occurrence cap and returned, so `every 2 months` would
  // be stored, accepted and then silently expanded EVERY month. Offering a
  // control the server discards is the same trap as a wrong unit, one level
  // down, so the control was made ABSENT rather than chipped.
  //
  // C-SWEEP-R4 STAGE 22 FIXED THE SERVER. `model.go`'s monthly branch now
  // applies the interval and reads the occurrence cap as occurrences rather
  // than months, so the reason this control is withheld is GONE and restoring
  // it is a UI change with a working backend under it. It is deliberately NOT
  // done in the same stage as the predicate fix — it is booked by name as
  // C-CALV4-MONTHLY-INTERVAL-CONTROL (.ai/todo.md), because it also needs the
  // `every N months` readout wording, the inverse in recurrenceFromRecord, and
  // the daycard request pins updated together.
  function recurrenceInterval(unit) { return unit === 'week'; }

  // recurrenceBody maps the editor's recurrence state onto the request body,
  // and it is the mapping [ER-10] condition 2 makes the real contract.
  //
  //   every 1 → weekly · every 2 → biweekly · every N → custom + interval N
  //   month   → monthly, no interval (see recurrenceInterval — the server
  //             honours one since stage 22; this file has not been taught to
  //             AUTHOR one yet, which is C-CALV4-MONTHLY-INTERVAL-CONTROL)
  //   an UNBACKED unit → null, meaning DO NOT AUTHOR
  //
  // NULL IS THE HONEST ANSWER FOR A CHIPPED UNIT. The caller round-trips the
  // stored recurrence when it gets one, so picking `day` writes nothing at all
  // rather than writing a type the server will silently degrade. That is
  // exactly what the `needs backend` chip beside it promises.
  //
  // `once` CLEARS THE TYPE AND THE INTERVAL TOGETHER, and the empty string is
  // not a flourish: service.UpdateEvent guards the pointer siblings
  // (`if input.RecurrenceType != nil`), so a JSON `null` CANNOT clear the
  // column — it preserves it, which is precisely the half-state
  // C-CAL-RECURRING-PARTIAL-STATE-CLEANUP already had to clean up once
  // (is_recurring=false with a live recurrence_type beside it). "" is a
  // non-nil pointer the switch sends to `default`, so the pair lands
  // consistent. THE END/MAX FIELDS CANNOT BE REACHED FROM HERE AT ALL — the
  // shipped PUT binds none of them — and that is carried, not papered over.
  function recurrenceBody(rec) {
    if (!rec || rec.mode !== 'repeats') {
      return { is_recurring: false, recurrence_type: '', recurrence_interval: 0 };
    }
    if (rec.unit === 'month') {
      return { is_recurring: true, recurrence_type: 'monthly', recurrence_interval: 0 };
    }
    // `year` sends interval 0 for the same reason `month` does: the editor
    // offers no `every [N]` field for it (recurrenceInterval is week-only), so
    // writing a number the author never typed would author a rule they did not
    // choose. The engine reads a missing/0/1 interval as "every year".
    if (rec.unit === 'year') {
      return { is_recurring: true, recurrence_type: 'yearly', recurrence_interval: 0 };
    }
    if (rec.unit !== 'week') return null;
    var every = Math.floor(Number(rec.every));
    if (!isFinite(every) || every < 1) every = 1;
    if (every === 1) return { is_recurring: true, recurrence_type: 'weekly', recurrence_interval: 0 };
    if (every === 2) return { is_recurring: true, recurrence_type: 'biweekly', recurrence_interval: 0 };
    return { is_recurring: true, recurrence_type: 'custom', recurrence_interval: every };
  }

  // recurrenceFromRecord is the INVERSE, so the editor opens on the state the
  // record is actually in and a title-only save round-trips it byte for byte.
  function recurrenceFromRecord(rec, weekLen) {
    var out = { mode: 'once', every: 1, unit: weekLen > 0 ? 'week' : 'month', wd: [] };
    if (!rec || !rec.is_recurring) return out;
    var t = rec.recurrence_type;
    if (t === 'monthly') { out.mode = 'repeats'; out.unit = 'month'; return out; }
    if (t === 'yearly') { out.mode = 'repeats'; out.unit = 'year'; return out; }
    if (t === 'weekly' || t === 'biweekly' || t === 'custom') {
      out.mode = 'repeats';
      out.unit = 'week';
      out.every = t === 'biweekly' ? 2
        : (t === 'custom' && rec.recurrence_interval > 0 ? rec.recurrence_interval : 1);
      return out;
    }
    // A legacy or unknown type expands to a single occurrence server-side, so
    // the editor shows it as one rather than inventing a rule for it.
    return out;
  }

  // audienceFromRules turns the stored allow/deny pair into the roster's own
  // per-member state. IT IS ALLOW-BY-DEFAULT ONLY WHEN THERE IS NO RULE AT ALL:
  // an event with an allowed_users list admits exactly that list, so a member
  // absent from it is denied, and reading them as allowed would silently widen
  // an audience on the first save.
  function audienceFromRules(chips, memberIDs) {
    var allowed = {}, denied = {}, sawAllow = false;
    (chips || []).forEach(function (c) {
      if (!c || c.kind !== 'user' || !c.target) return;
      if (c.mode === 'allow') { allowed[c.target] = true; sawAllow = true; }
      else if (c.mode === 'deny') denied[c.target] = true;
    });
    var out = {};
    (memberIDs || []).forEach(function (id) {
      out[id] = sawAllow ? !!allowed[id] : !denied[id];
    });
    return out;
  }

  // audienceToChips is the return leg, in the shape the SHARED mapper takes
  // ([DC-10] SIGNED — this module never writes visibility_rules itself).
  //
  // It emits an ALLOW list, which is the shape `visibility_rules` carries for a
  // restricted event and the shape the stills draw. A member switched off is
  // absent from the list rather than present on a deny list, because the two
  // are not the same statement: allowed_users is a closed door with a guest
  // list, and denied_users is an open door with a bouncer.
  function audienceToChips(state, memberIDs) {
    var chips = [];
    (memberIDs || []).forEach(function (id) {
      if (state && state[id]) chips.push({ mode: 'allow', kind: 'user', target: id, label: id });
    });
    return chips;
  }

  // ledgerIsStacked answers the ONE question the `Open in the Ledger` door
  // depends on: is the Ledger BELOW the month, or BESIDE it?
  //
  // WHY THE DOOR NEEDS AN ANSWER AT ALL — calv4 fix R1, item 3. A day cell's
  // real hit target is the stretched `.dsel` label (instrument.templ's dayPick),
  // which is `for` that day's own `input.daypick`. So a click on a day ALREADY
  // selects it in the Ledger, natively, before this module runs — measured:
  // the pointer lands on `.dsel` and the radio comes back checked. With the
  // Ledger DOCKED beside the month and fully on screen, the door then clicks a
  // checked radio, scrolls to something already in view, and closes the card.
  // Only the last is an effect, and "close this card" is not what the button
  // says — while the card closes on outside-click, on Escape and on the next
  // day click anyway.
  //
  // It is NOT redundant stacked. `.cal-block-host .body` is a flex COLUMN by
  // default and only becomes a two-column grid at
  // `@container cal-block (min-width: 900px)`; below that the Ledger is a
  // full-width band under the month, and jumping to it is a real service. So
  // the door is CONDITIONED rather than deleted, and rather than renamed:
  // there is no honest name for a docked-Ledger button whose whole net effect
  // is closing the card.
  //
  // IT IS A MEASUREMENT, NOT A BREAKPOINT. The 900px lives in a container
  // query on the BLOCK's own width, which is not the viewport and is not
  // readable from a media query here — and this module already measures the
  // Ledger's rect for the occlusion dodge, so the fact is one it holds anyway.
  // Restating the number in JavaScript is how the two drift.
  //
  // Pure, and exported, so daycard_ledger_door_probe_test.go can drive it
  // without a browser and the browser probe can check the DOM agrees.
  function ledgerIsStacked(month, ledger) {
    if (!month || !ledger) return false;
    // The 1px slack is a sub-pixel allowance, not a tolerance for "nearly
    // below": docked, the two boxes SHARE a top edge and this is false by
    // hundreds of pixels.
    return ledger.top >= month.bottom - 1;
  }

  // ledgerStackedIn is the DOM half. It returns false when either box is
  // missing, which is the safe answer for a door: absent, not asserted.
  function ledgerStackedIn(host) {
    if (!host || !host.querySelector) return false;
    var month = host.querySelector('.inst');
    var zone = host.querySelector('[data-zone="ledger"]');
    if (!month || !zone) return false;
    return ledgerIsStacked(month.getBoundingClientRect(), zone.getBoundingClientRect());
  }

  if (typeof window !== 'undefined') {
    window.__calDayCard = {
      ledgerIsStacked: ledgerIsStacked,
      writeTarget: writeTarget,
      indexPayload: indexPayload,
      indexMembers: indexMembers,
      headText: headText,
      durationMS: durationMS,
      closeDelayMS: closeDelayMS,
      placeCard: placeCard,
      // C-CALV4-MOBILE [MOB-2]: sheetPlacement is exported so the clamped rect
      // it reports — [DC-3]'s honesty channel after the pixel left JavaScript —
      // is asserted directly rather than inferred through placeCard.
      sheetPlacement: sheetPlacement,
      occlusionReporter: occlusionReporter,
      applyPlacement: applyPlacement,
      ordIsSafe: ordIsSafe,
      numOrNull: numOrNull,
      buildEventBody: buildEventBody,
      isIntercalary: isIntercalary,
      weekShape: weekShape,
      orderedDates: orderedDates,
      dayRange: dayRange,
      nextDate: nextDate,
      recurrenceUnits: recurrenceUnits,
      recurrenceInterval: recurrenceInterval,
      recurrenceBody: recurrenceBody,
      recurrenceFromRecord: recurrenceFromRecord,
      audienceFromRules: audienceFromRules,
      audienceToChips: audienceToChips,
    };
  }

  // ── DOM driver ───────────────────────────────────────────────────────────

  function init() {
    if (typeof document === 'undefined') return;
    var card = document.querySelector('[data-cal-daycard]');
    if (!card || card.dataset.dcWired === '1') return;
    var surface = document.querySelector('[data-cal-daycard-payload]');
    if (!surface) return;
    card.dataset.dcWired = '1';

    var index = indexPayload(surface.getAttribute('data-cal-daycard-payload'));
    var head = card.querySelector('[data-dc-head]');
    var rows = card.querySelector('[data-dc-rows]');
    var empty = card.querySelector('[data-dc-empty]');
    var foot = card.querySelector('[data-dc-foot]');
    var box = card.querySelector('[data-dc-box]');
    var state = { open: false, key: '', calId: '', host: null, timer: 0, suppress: false };

    // ONE reporter for the page, shared by the card and the editor: the
    // condition is a property of the layout, not of which box happened to meet
    // it first, so a second warning from the editor about the same geometry
    // would be noise. Built here rather than at module scope so a re-init on a
    // fresh page gets a fresh one-shot.
    var reportOcclusion = occlusionReporter(function (msg) {
      if (typeof console !== 'undefined' && console && console.warn) console.warn(msg);
    });

    // THE AUTHORING GATES ARE READ, NEVER DECIDED. Both facts below were
    // decided by the PRODUCER and rendered into the markup ([DC-9] SIGNED):
    // `data-dc-can-edit` is the Scribe floor, and the editor scaffold's very
    // EXISTENCE is the same gate expressed as absence. This module executes
    // them; it does not compute them, and there is no branch here that could
    // turn a control on for a viewer the server did not render it for.
    var editor = document.querySelector('[data-cal-dayeditor]');
    var canEdit = card.hasAttribute('data-dc-can-edit') && !!editor;
    var campaignID = card.getAttribute('data-campaign-id') || '';
    var ed = editor ? {
      root: editor,
      box: editor.querySelector('[data-dc-box]'),
      head: editor.querySelector('[data-de-head]'),
      form: editor.querySelector('[data-de-form]'),
      name: editor.querySelector('[data-de-name]'),
      desc: editor.querySelector('[data-de-desc]'),
      category: editor.querySelector('[data-de-category]'),
      year: editor.querySelector('[data-de-year]'),
      month: editor.querySelector('[data-de-month]'),
      day: editor.querySelector('[data-de-day]'),
      allDay: editor.querySelector('[data-de-allday]'),
      timeRow: editor.querySelector('[data-de-timerow]'),
      startH: editor.querySelector('[data-de-starth]'),
      startM: editor.querySelector('[data-de-startm]'),
      endH: editor.querySelector('[data-de-endh]'),
      endM: editor.querySelector('[data-de-endm]'),
      endYear: editor.querySelector('[data-de-endyear]'),
      endMonth: editor.querySelector('[data-de-endmonth]'),
      endDay: editor.querySelector('[data-de-endday]'),
      gmOnly: editor.querySelector('[data-de-gmonly]'),
      err: editor.querySelector('[data-de-err]'),
      del: editor.querySelector('[data-de-delete]'),
      // ── THE CHROME'S HANDLES (C-CALV4-EDITOR-R2b stage 2) ─────────────
      //
      // EVERY ONE OF THEM MAY BE null AND THAT IS THE GATE. `restricted`,
      // `vis`, `aud` and `audRows` are absent for a viewer the producer did not
      // render them for, and `gmOnly` was already absent below the capability.
      // The builders below check before they write, so a Scribe-floor render
      // that expects a control and finds none DEGRADES rather than throwing —
      // which is also what the handler's own body gating requires
      // (a player receives neither audience key at all).
      tyRail: editor.querySelector('[data-de-tyrail]'),
      tyGlyph: editor.querySelector('[data-de-tyglyph]'),
      idOut: editor.querySelector('[data-de-id]'),
      typeRail: editor.querySelector('[data-de-typerail]'),
      dateLab: editor.querySelector('[data-de-datelab]'),
      datePicker: editor.querySelector('[data-de-datepicker]'),
      dateRead: editor.querySelector('[data-de-dateread]'),
      endRead: editor.querySelector('[data-de-endread]'),
      recSeg: editor.querySelector('[data-de-recurrence]'),
      recOn: editor.querySelector('[data-de-recon]'),
      recEvery: editor.querySelector('[data-de-recevery]'),
      recUnits: editor.querySelector('[data-de-recunits]'),
      recRead: editor.querySelector('[data-de-recread]'),
      recBox: editor.querySelector('[data-de-recbox]'),
      wdPick: editor.querySelector('[data-de-wdpick]'),
      vis: editor.querySelector('[data-de-vis]'),
      restricted: editor.querySelector('[data-de-restricted]'),
      aud: editor.querySelector('[data-de-aud]'),
      audRows: editor.querySelector('[data-de-audrows]'),
      tieRow: editor.querySelector('[data-de-tierow]'),
      tieSearch: editor.querySelector('[data-de-tiesearch]'),
      tieRes: editor.querySelector('[data-de-tieres]'),
      entity: editor.querySelector('[data-de-entity]'),
      preview: editor.querySelector('[data-de-preview]'),
      // C-CALV4-GAMEREADY §4 [GR-6]. BOTH MAY BE null, and that IS the
      // audience gate: below RoleScribe the producer renders no markup at all,
      // so this module finds nothing and writes nothing. It executes the gate;
      // it never computes one.
      rsvp: editor.querySelector('[data-de-rsvp-toggle]'),
      rsvpHint: editor.querySelector('[data-de-rsvp-hint]'),
      save: editor.querySelector('[data-de-save]'),
    } : null;
    // THE AUDIENCE ROSTER IS OWNER-ONLY AND IT ARRIVES ALREADY GATED ([ER-3]
    // SIGNED). The producer omits the `members` key entirely below the Owner
    // floor, so there is nothing here to gate: a Scribe's page simply has no
    // names in it. This module executes the gate; it does not compute it.
    var members = indexMembers(surface.getAttribute('data-cal-daycard-payload'));
    // `busy` is the WRITE IN FLIGHT flag (DC-SAVE-6). Two independent
    // listeners reach edSave — the delegated document click on [data-de-save]
    // and the form's own submit — and a real user click only fires one of them
    // because the click branch's preventDefault cancels the submit button's
    // activation behaviour. That is a true fact about the browser and a terrible
    // thing to rest a write on: an early return, a move to the capture phase, or
    // a reorder of the branch chain above it would silently turn every Save into
    // two events, and the symptom would read as a server bug. A double-click
    // before the reload lands does it today with nothing refactored at all.
    //
    // So the guard is at the WRITE, not at the listener. It covers both entry
    // points, both verbs, and the delete path, and it is cleared on failure so a
    // refused save can be retried — but never on success, because success ends
    // in a reload and clearing it would open a window for a second POST.
    var edState = {
      open: false, timer: 0, mode: '', calId: '', eventID: '', day: null,
      prev: null, busy: false,
      // DELETE'S TWO-STEP (C-CALV4-GAMEREADY §7 [GR-13]). `delArmed` is the
      // "one click has landed" latch and `delTimer` is its ~4s expiry. Both
      // live on edState rather than in a closure beside edDelete so that
      // edClose and edOpen can clear them — an editor that reopened still
      // armed would delete on the FIRST click of the next session.
      delArmed: false, delTimer: 0,
      // The save's in-flight ceiling (§7's same-handler freebie). A fetch that
      // never resolves must become a stated failure, not a dead button.
      saveTimer: 0,
      // THE MORPH'S MEASURED GEOMETRY, taken from the CARD at open and replayed
      // on close. Null under reduced motion and whenever a rect could not be
      // measured, which is what makes the stage-2 open path the fallback.
      morph: null, morphTimer: 0,
    };

    // DRAG-CREATE'S STATE, declared HERE rather than beside its listeners at
    // the foot of this function. `var` is function-scoped and hoisted, so the
    // click handler above would have worked either way — but a reader meeting
    // `drag.eatClick` two hundred lines before the declaration has to take the
    // hoisting on trust, and stage 4 is supposed to be readable as a severable
    // block, not as a puzzle.
    var drag = {
      on: false, host: null, cal: null, startKey: '', lastKey: '',
      moved: false, layer: null, eatClick: false,
    };

    function reduced() {
      return !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);
    }

    function cssVar(name) {
      if (!window.getComputedStyle) return '';
      return window.getComputedStyle(card).getPropertyValue(name);
    }

    // --- rendering the day -------------------------------------------------

    // renderRows builds the card's list. EVERY FIELD HERE IS ONE THE LEDGER ROW
    // ALREADY PRINTS to this viewer (ledger.templ:155-190) — rail + pattern,
    // the gold rail and `GM` badge for dm_only, the glyph, the title, the
    // audience chip, the time. There is no field the payload could supply that
    // is not on that list, because the payload has none.
    function renderRows(day, cal) {
      while (rows.firstChild) rows.removeChild(rows.firstChild);
      var list = (day && day.events) || [];
      list.forEach(function (ev) { rows.appendChild(buildRow(ev, day, cal)); });
      if (empty) empty.hidden = list.length > 0;
    }

    function buildRow(ev, day, cal) {
      var row = document.createElement('div');
      row.className = 'dc-row';
      // GUARD B4: every dated node carries the ANSWER key, in the dayKey
      // namespace, so a partner surface can match it.
      row.setAttribute('data-day', day.key);
      if (ev.id) row.setAttribute('data-event-id', ev.id);
      if (cal && cal.id) row.setAttribute('data-calendar-id', cal.id);
      if (ev.axis) row.style.setProperty('--axis', ev.axis);

      var rail = document.createElement('i');
      rail.className = 'rail ' + (ev.pattern || 'p1');
      rail.setAttribute('aria-hidden', 'true');
      row.appendChild(rail);

      if (ev.gold) {
        var gr = document.createElement('i');
        gr.className = 'gr';
        gr.setAttribute('title', 'hidden from players');
        gr.setAttribute('aria-hidden', 'true');
        row.appendChild(gr);
      }
      if (ev.glyph) {
        var tok = document.createElement('span');
        tok.className = 'tok';
        tok.setAttribute('aria-hidden', 'true');
        tok.textContent = ev.glyph;
        row.appendChild(tok);
      }

      var mid = document.createElement('span');
      mid.className = 'mid';
      var nm = document.createElement('span');
      nm.className = 'nm';
      nm.textContent = ev.title || '';
      mid.appendChild(nm);
      if (ev.gold) {
        var badge = document.createElement('span');
        badge.className = 'badge gm';
        badge.textContent = 'GM';
        mid.appendChild(badge);
      }
      if (ev.audience) {
        var aud = document.createElement('span');
        aud.className = 'audchip';
        aud.textContent = ev.audience;
        mid.appendChild(aud);
      }
      row.appendChild(mid);

      if (ev.time) {
        var tm = document.createElement('span');
        tm.className = 'tm';
        tm.textContent = ev.time;
        row.appendChild(tm);
      }

      // A ROW IS A DOOR — FOR SCRIBE+ ONLY. A player's rows are read-only text,
      // which is the V2 quick-edit's own server-gated shape
      // (calendar_v2_quickedit.templ:9-14). There is no disabled twin and no
      // title explaining the absence: the button is either here or it is not.
      if (canEdit && ev.id) {
        var edit = document.createElement('button');
        edit.type = 'button';
        edit.className = 'dc-edit';
        edit.setAttribute('data-dc-edit', ev.id);
        edit.setAttribute('aria-label', 'Edit ' + (ev.title || 'event'));
        edit.textContent = 'Edit';
        row.appendChild(edit);
      }
      return row;
    }

    // renderFoot emits the doors. `Open in the Ledger` EXISTS ONLY WHEN THE
    // LEDGER IS ACTUALLY DOCKED for this viewer, and that fact is carried on
    // the payload rather than inferred from the DOM's absence — absence has two
    // causes (a host that never docked the zone, and a viewer who switched the
    // layer off) and a link to a column that is not on the page is a lie.
    // THE PAYLOAD FACT IS NECESSARY AND NO LONGER SUFFICIENT: a docked Ledger
    // that sits BESIDE the month is already showing the day this card is about,
    // so the door is additionally conditioned on the stacked layout below.
    // IT NEVER TOUCHES THE `+ New event` BUTTON, WHICH IS THE PRODUCER'S. Only
    // the Ledger door is managed here, and it is inserted BEFORE whatever the
    // server rendered rather than replacing the foot — a module that cleared
    // this container would be deciding a role gate by omission.
    // AND IT IS CONDITIONED ON THE STACKED LAYOUT — calv4 fix R1, item 3. With
    // the Ledger docked BESIDE the month the door does nothing the day click
    // did not already do except close the card: the stretched `.dsel` label has
    // already checked the day's radio, and the column is already on screen. See
    // ledgerIsStacked above for the whole argument and the measurement. `host`
    // is passed in rather than read from `state`, because openCard sets
    // `state.host` AFTER this runs.
    function renderFoot(cal, host) {
      var existing = foot.querySelector('[data-dc-ledger]');
      if (existing) foot.removeChild(existing);
      if (!cal || !cal.ledgerDocked) return;
      if (!ledgerStackedIn(host)) return;
      var btn = document.createElement('button');
      btn.type = 'button';
      btn.className = 'dc-door';
      btn.setAttribute('data-dc-ledger', '');
      btn.textContent = 'Open in the Ledger';
      if (foot.firstChild) foot.insertBefore(btn, foot.firstChild);
      else foot.appendChild(btn);
    }

    // --- opening and closing ----------------------------------------------

    function show() {
      if (state.timer) { clearTimeout(state.timer); state.timer = 0; }
      card.setAttribute('data-dc-shown', '');
      // [MOB-3]: the lock follows the SHOW, not the intent, so a card that was
      // opened by any path locks the page behind it.
      sheetOpenChanged('card', true);
      if (typeof card.showPopover === 'function') {
        try { card.showPopover(); } catch (e) { /* already open */ }
      }
    }

    function hide() {
      state.timer = 0;
      card.removeAttribute('data-dc-shown');
      card.removeAttribute('style');
      sheetOpenChanged('card', false);
      if (typeof card.hidePopover === 'function') {
        try { card.hidePopover(); } catch (e) { /* already closed */ }
      }
    }

    // ── THE `toggle` EVENT IS THE TRUTH ([MOB-3] SIGNED) ──────────────────
    //
    // `closeCard`/`edClose` are the animated INTENT and `hide`/`edHide` are the
    // teardown, but neither sees a UA-initiated close — Escape in the engines
    // that honour it on a manual popover, or a `hidePopover()` from anywhere
    // else on the page. Only the element's own event sees all of them, and a
    // lock released only on the module's happy paths is exactly the bug this
    // clause exists to prevent. Wiring both is safe because sheetOpenChanged is
    // a SET rather than a counter.
    if (card && card.addEventListener) {
      card.addEventListener('toggle', function (e) {
        // `newState` IS TRUSTED WHEN THE ENGINE SUPPLIES IT, and the attribute
        // is the fallback for one that does not. Reading them as an OR would
        // make a UA-initiated close invisible: `hidePopover()` from anywhere
        // leaves `data-dc-shown` behind, so the OR would answer "still open"
        // for a box that is not on the screen — and the lock would never lift.
        sheetOpenChanged('card', (e && typeof e.newState === 'string')
          ? e.newState === 'open'
          : card.hasAttribute('data-dc-shown'));
      });
    }

    function openCard(host, cell) {
      var calId = host.getAttribute('data-calendar-id') || '';
      var cal = index[calId];
      if (!cal) return;
      var key = cell.getAttribute('data-day') || '';
      var day = cal.days[key];
      if (!day) return;
      // IDEMPOTENT. Both openers are wired ([DC-4]) and a pointer click on the
      // stretched .dsel label fires BOTH — the click and the radio's change.
      // Re-opening the same day must therefore be a no-op rather than a second
      // animation.
      if (state.open && state.key === key && state.calId === calId) return;

      head.textContent = headText(day);
      head.setAttribute('data-day', day.key);
      renderRows(day, cal);
      renderFoot(cal, host);

      state.open = true;
      state.key = key;
      state.calId = calId;
      state.host = host;
      state.ord = day.ord;

      // ONE PANEL AT A TIME, THE OTHER DIRECTION (MN-G13). Declining to open
      // over the moon panel is only half the interlock: a panel opened on row 1
      // and a card opened on row 3 would still be two panels. The card is the
      // one arriving, so the card closes the other.
      //
      // IT ACTIVATES A SHIPPED CONTROL EXACTLY AS A POINTER WOULD — the same
      // move, and the same argument, as the `Open in the Ledger` door two
      // hundred lines up: `.click()` on the moon group's own explicit `none`
      // radio changes checkedness, which is IDL state and not a content
      // attribute, so the Block's serialised DOM is unchanged and
      // daycard_block_immutability.test.mjs stays green. Writing `checked`
      // ourselves and dispatching a synthetic change would be this module
      // simulating the browser instead of using it.
      var moonOff = host.querySelector('.moonpick[data-moon-pick="none"]');
      if (moonOff && !moonOff.checked) moonOff.click();

      show();
      position(cell);
      // FORCED REFLOW, DELIBERATELY. The card was display:none a moment ago, so
      // it has no rendered before-change style and a transition started in the
      // same frame would not run at all. Reading a layout property flushes one,
      // which is what makes the register's reveal fire on a popover without
      // @starting-style — a standing refusal.
      void card.offsetHeight;
      card.classList.add('dcopen');
    }

    function closeCard() {
      if (!state.open) return;
      state.open = false;
      card.classList.remove('dcopen');
      var wait = closeDelayMS(cssVar('--disc-close'), reduced());
      if (wait <= 0) { hide(); return; }
      if (state.timer) clearTimeout(state.timer);
      state.timer = setTimeout(hide, wait);
    }

    // position measures the FINAL height while the box is still collapsed:
    // scrollHeight is the content's height under `overflow:hidden`, and the
    // difference between the card and the box is the card's own chrome.
    function position(cell) {
      if (!cell.getBoundingClientRect || !card.getBoundingClientRect) return;
      var anchor = cell.getBoundingClientRect();
      var view = {
        w: window.innerWidth || 0,
        h: window.innerHeight || 0,
      };
      var chrome = (card.offsetHeight || 0) - (box ? box.offsetHeight || 0 : 0);
      var size = {
        w: card.offsetWidth || 0,
        h: (box ? box.scrollHeight || 0 : 0) + (chrome > 0 ? chrome : 0),
      };
      var ledger = ledgerRect(state.host);
      applyPlacement(card, placeCard(anchor, size, view, ledger, {}), reportOcclusion);
    }

    function ledgerRect(host) {
      if (!host || !host.querySelector) return null;
      var zone = host.querySelector('[data-zone="ledger"]');
      if (!zone || !zone.getBoundingClientRect) return null;
      var r = zone.getBoundingClientRect();
      return { left: r.left, top: r.top, width: r.width, height: r.height };
    }

    // --- the doors ---------------------------------------------------------

    // openInLedger activates the day's OWN radio so the shipped ladder fires,
    // then brings the column into view and leaves. It selects the control the
    // server rendered; it does not synthesise one.
    function openInLedger() {
      var host = state.host;
      if (!host || !ordIsSafe(state.ord)) { closeCard(); return; }
      var radio = host.querySelector('input.daypick[data-day-pick="' + state.ord + '"]');
      if (radio && typeof radio.click === 'function') {
        state.suppress = true;
        try { radio.click(); } finally { state.suppress = false; }
      }
      var zone = host.querySelector('[data-zone="ledger"]');
      if (zone && typeof zone.scrollIntoView === 'function') {
        zone.scrollIntoView({ block: 'nearest' });
      }
      closeCard();
    }

    // --- the editor --------------------------------------------------------
    //
    // THE CARD CLOSES AS THE EDITOR OPENS ([DC-7] SIGNED). The two share an
    // origin, so the pair reads as ONE motion under the register's grammar —
    // no new keyframe, no view-transition, no third signature. The richer
    // shared-element MORPH the operator asked for is ESCALATED as a named
    // carve-out for their signature, not invented here: per-surface motion
    // invention is what produced the skypane verdict.

    // ── THE EDITOR MORPH — the operator's signed carve-out ───────────────
    //
    // decisions/2026-08-01-operator-signatures-wz1-sky-editor.md §3, recorded
    // as: "the day card may visually morph into the editor as its own named
    // motion signature — resolving [DC-7]'s escalation; the DAYCARD-era
    // register-only constraint is lifted FOR R2b ONLY, and the morph must still
    // be instant/complete under reduced motion and never touch the Block's
    // interior."
    //
    // GROW/MORPH CONTINUITY, WHICH IS THE WHOLE POINT. The card does not vanish
    // and the editor does not appear: ONE BOX BECOMES THE OTHER. The editor is
    // placed at its FINAL geometry by the shipped placement law, then seeded
    // back onto the card's measured rect and handed to the register's carve-out
    // rule. The card cross-fades out over the same interval on the mechanism it
    // already had, so at no frame are two full boxes visible — they occupy the
    // same rect while one ramps up and the other ramps down.
    //
    // IT MOVES BY `translate`, NOT BY `left`/`top`, and it does NOT SCALE. A
    // FLIP scale is the cheap way and it visibly squashes the text inside a
    // growing box; this surface is a FORM. `translate` is also nameable in the
    // guard's allowlist on its own, so the allowlist can admit the movement
    // without admitting a scale.
    //
    // `.edmorph` IS PRESENT ONLY IN FLIGHT. It is added to seed, removed when
    // the geometry has landed, and re-added to reverse — so a resting editor
    // carries no transition and opening the audience list later resizes
    // instantly rather than animating something nobody signed.
    //
    // ZERO BLOCK LEAKAGE. The only rect this reads is the CARD's, which is a
    // page-level sibling in the top layer. It inserts no node inside
    // .cal-block-host, adds and removes no class there, and animates nothing
    // there — and test/js/daycard_block_immutability.test.mjs drives this path
    // and re-compares the fixture's innerHTML byte for byte.

    function edMorphRect(el) {
      if (!el || !el.getBoundingClientRect) return null;
      var r = el.getBoundingClientRect();
      if (!(r.width > 0) || !(r.height > 0)) return null;
      return r;
    }

    // edMorphSeed places the editor at the card's geometry once the placement
    // law has decided where it really goes. It returns false when it cannot —
    // no card rect, no resolved target, or reduced motion — and the caller then
    // opens the editor exactly as stage 2 did.
    //
    // THE TARGET SIZE IS THE ONE edPosition ALREADY COMPUTED, NEVER
    // getBoundingClientRect(). This is the bug the parked capture caught, and
    // it is worth the paragraph because it looked right in every still: at the
    // moment the editor is shown, `.cal-dayeditor:not(.edmorph) .dcbox` still
    // collapses the inner box to block-size 0, so the ROOT measures about two
    // pixels tall. A morph seeded from that rect animates a 2px target — the
    // box appears to snap and the frames are stills of nothing. `scrollHeight +
    // chrome` is the content's real height under `overflow:hidden`, it is what
    // the placement law is already measuring, and using the same number means
    // the box lands exactly where placeCard put it.
    //
    // REDUCED MOTION SEEDS NOTHING AT ALL, and that is the second half of
    // "instant AND complete". The sheet declares no rule outside the
    // no-preference wrapper, so a seeded start geometry with no transition to
    // leave it would land the editor INSTANTLY AT A MID-MORPH SIZE — the exact
    // failure mode the signature's two words are guarding against.
    function edMorphSeed(fromEl, target, at) {
      if (reduced()) return false;
      var from = edMorphRect(fromEl);
      if (!from || !target || !(target.w > 0) || !(target.h > 0)) return false;
      edState.morph = {
        dx: Math.round(from.left - (at ? at.left : 0)),
        dy: Math.round(from.top - (at ? at.top : 0)),
        w: Math.round(from.width),
        h: Math.round(from.height),
        toW: Math.round(target.w),
        toH: Math.round(target.h),
      };
      edMorphWrite(edState.morph, true);
      return true;
    }

    // edMorphWrite is the ONE writer of the four transitioned properties, so
    // the outbound and inbound halves cannot drift into two ideas of what the
    // card's geometry was.
    // IT WRITES THE LOGICAL PROPERTIES, AND THAT IS NOT A STYLE PREFERENCE.
    // The carve-out's rule names `inline-size` and `block-size`; writing
    // `style.width` / `style.height` instead leaves the declared
    // transition-property matching nothing, so the box SNAPS to its end
    // geometry while `opacity` alone animates — which looks close enough to a
    // morph in a still and is not one. It was caught by parking the transitions
    // at 0% and finding the editor already at full size.
    function edMorphWrite(m, atCard) {
      if (!m || !ed.root || !ed.root.style) return;
      ed.root.style.setProperty('translate',
        atCard ? m.dx + 'px ' + m.dy + 'px' : '0px 0px');
      ed.root.style.setProperty('inline-size', (atCard ? m.w : m.toW) + 'px');
      ed.root.style.setProperty('block-size', (atCard ? m.h : m.toH) + 'px');
      ed.root.style.setProperty('opacity', atCard ? '0' : '1');
    }

    // edMorphSettle hands the box back to its own natural sizing once the
    // geometry has landed. Leaving the measured height pinned would make
    // `overflow:hidden` clip anything the form grew afterwards — the audience
    // roster opening under Restricted is exactly that case.
    function edMorphSettle() {
      if (edState.morphTimer) { clearTimeout(edState.morphTimer); edState.morphTimer = 0; }
      if (!ed.root || !ed.root.style) return;
      ed.root.classList.remove('edmorph');
      ed.root.style.removeProperty('translate');
      ed.root.style.removeProperty('inline-size');
      ed.root.style.removeProperty('block-size');
      ed.root.style.removeProperty('opacity');
    }

    // edShow OPENS THE BOX, AND ITS FIRST JOB IS TO MAKE THE BOX MEASURABLE.
    //
    // THE STALE-GEOMETRY REOPEN, WHICH WAS THE ROUND-3 BLOCKER ARRIVING THROUGH
    // A STYLE ATTRIBUTE. edClose writes the REVERSE morph geometry as inline
    // `inline-size` / `block-size` / `translate` / `opacity` — the card's
    // measured rect, 420px wide — and edHide is the only thing that clears it,
    // on the --disc-close timer the line above has just CANCELLED. So a reopen
    // inside that 160ms window used to walk into edPosition with the card's
    // width still pinned inline, and `ed.root.offsetWidth` answered 420 for a
    // box the sheet sizes at `--de-w` (760). placeCard then reasoned about a
    // box that is not the one that renders: it found a "clear" position for a
    // 420px box, the 760px box drew there, and the docked Ledger was occluded —
    // with the module's own occlusion report cheerfully saying clear=true.
    //
    // MEASURED, NOT REASONED: DAYCARD_GEOMETRY=1 caught it on 23 of the real
    // world calendar's day cells at viewport 900, up to 70,906 px² of overlap,
    // at EVERY candidate width including the shipped one — because the stale
    // rect is the card's rect and does not vary with --de-w. Closing and
    // immediately opening another day is not an exotic path; it is what reading
    // two days in a row looks like.
    //
    // THE FIX IS TO CLEAR, NOT TO RE-MEASURE. edMorphSettle is already the one
    // place that hands the box back to its own natural sizing, so the reopen
    // borrows it rather than growing a second idea of what "no morph geometry"
    // means. `placeCard` IS NOT TOUCHED BY ANY OF THIS ([ER-5]: a fourth
    // geometry would be the round-4 lesson unlearned) — the law was always
    // right, it was being handed a lie about the box.
    // AND THE SAME LIE ARRIVES A SECOND WAY, THROUGH THE PREVIOUS PLACEMENT'S
    // SHEET — stage 19, found by the [ER-5] probe's own MeasuredW guard.
    //
    // `applyPlacement` is the only writer of `.dcsheet` and of the `style.width`
    // that goes with it, and it writes them AFTER `edPosition` has measured the
    // box. Nothing clears them in between: `edHide` drops the whole style
    // attribute but not the class, and it runs on a --disc-close timer this path
    // has already cancelled. So opening a day whose editor SHEETS and then
    // opening one whose editor does not hands `edPosition` a box still wearing
    // the sheet — `ed.root.offsetWidth` answers the full viewport width for a
    // box that is about to render at `--de-w`, and placeCard reasons about a
    // rectangle that does not exist. That is the round-3 blocker's exact shape
    // arriving through a class instead of an inline style.
    //
    // IT IS PRE-EXISTING AND IT WAS INVISIBLE UNTIL THE MORPH RAN. With the open
    // morph inert the box was never actually sized from that measurement, so
    // every rendered frame looked right and only the PLACEMENT was computed from
    // the wrong number. The stage-18 reorder made the morph animate to that
    // number, the probe read it, and its stale-geometry assertion went red
    // naming the full viewport width. The guard did its job; this is the answer,
    // not an excuse for it.
    //
    // CLEARED, NOT RE-MEASURED, for the reason above — and `placeCard` is still
    // not touched.
    function edShow() {
      if (edState.timer) { clearTimeout(edState.timer); edState.timer = 0; }
      edMorphSettle();
      // BOTH HALVES, AND THROUGH THE SAME WRITER `applyPlacement` USES.
      // `el.style.width = ''` is how the placement law itself clears the sheet's
      // width for a popover placement; using `removeProperty` here instead would
      // be a second idea of what "no sheet" means.
      ed.root.classList.remove('dcsheet');
      if (ed.root.style) ed.root.style.width = '';
      edState.morph = null;
      ed.root.setAttribute('data-dc-shown', '');
      sheetOpenChanged('editor', true);
      if (typeof ed.root.showPopover === 'function') {
        try { ed.root.showPopover(); } catch (e) { /* already open */ }
      }
    }

    function edHide() {
      edState.timer = 0;
      if (edState.morphTimer) { clearTimeout(edState.morphTimer); edState.morphTimer = 0; }
      edState.morph = null;
      ed.root.classList.remove('edmorph');
      ed.root.removeAttribute('data-dc-shown');
      // THE STYLE WIPE IS WHY THE LOCK'S STATE IS NOT PARKED ON THIS ELEMENT
      // ([MOB-3]): anything written here is gone one line later.
      ed.root.removeAttribute('style');
      sheetOpenChanged('editor', false);
      if (typeof ed.root.hidePopover === 'function') {
        try { ed.root.hidePopover(); } catch (e) { /* already closed */ }
      }
    }

    function edClose() {
      if (!edState.open) return;
      edState.open = false;
      // THE ARMED DELETE DIES WITH THE EDITOR (§7 [GR-13]). A sheet that
      // closed while armed and reopened later would delete on the FIRST click
      // of a session the user believes is fresh.
      edDeleteDisarm();
      // `.dcopen` COMES OFF FIRST, AND THE ORDER IS THE WHOLE OF CLAUSE 2.
      // The carve-out's open-state rule is the only thing declaring
      // --disc-open, so removing the class before writing the reverse geometry
      // makes leaving run at --disc-close without anyone having to remember it.
      ed.root.classList.remove('dcopen');
      var wait = closeDelayMS(cssVar('--disc-close'), reduced());
      if (wait <= 0) { edHide(); return; }
      if (edState.morphTimer) { clearTimeout(edState.morphTimer); edState.morphTimer = 0; }
      if (edState.morph) {
        // REVERSE, not a second signature: the same four properties, the same
        // easing, back onto the same measured rect the card occupied.
        ed.root.classList.add('edmorph');
        void ed.root.offsetHeight;
        edMorphWrite(edState.morph, true);
      }
      if (edState.timer) clearTimeout(edState.timer);
      edState.timer = setTimeout(edHide, wait);
    }

    function edError(msg) {
      if (!ed.err) return;
      ed.err.textContent = msg || '';
      ed.err.hidden = !msg;
    }

    // ── THE CHROME'S BUILDERS — C-CALV4-EDITOR-R2b stage 2 ────────────────
    //
    // EVERY ONE OF THESE BUILDS DOM INSIDE THE EDITOR'S OWN BOX AND NOWHERE
    // ELSE. The module's boundary is unchanged: it may query and listen to the
    // Block's DOM and may not insert a node inside .cal-block-host, add or
    // remove a class there, or animate anything there.
    //
    // `edUI` is the chrome's state, and it is deliberately small. Where a value
    // has a hidden input the input IS the state (the write path reads it), and
    // this object carries only what has no field of its own: the calendar under
    // edit, its derived week, the ordered date list, the recurrence triple, the
    // per-member audience and the tie.
    var edUI = {
      cal: null,
      dates: [],
      week: { len: 0, names: [] },
      dayKey: '',
      endKey: '',
      // `touched` IS THE AUTHOR-vs-OMIT DISTINCTION, and it is the whole of
      // the dispatch's three losslessness cases. The chrome AUTHORS recurrence
      // now, so a save has to be able to say "I did not touch this" — otherwise
      // opening an event whose stored type Chronicle does not accept (a legacy
      // `yearly`, which expands to one occurrence) and renaming it would
      // REWRITE the stored rule as a side effect of a title change. Untouched
      // round-trips; touched authors; explicitly Once clears the pair.
      rec: { mode: 'once', every: 1, unit: 'week', wd: {}, touched: false },
      vis: 'public',
      aud: {},
      tie: null,
      tieTimer: 0,
      tieSeq: 0,
    };

    function clear(node) {
      if (!node) return;
      while (node.firstChild) node.removeChild(node.firstChild);
    }

    function mk(tag, cls, text) {
      var n = document.createElement(tag);
      if (cls) n.className = cls;
      if (text !== undefined && text !== null) n.textContent = String(text);
      return n;
    }

    function pressed(node, on) {
      if (node) node.setAttribute('aria-pressed', on ? 'true' : 'false');
    }

    // memberIDs is the audience's iteration order, and it is the ROSTER'S own
    // order — the same order the RSVP panel prints, because the identity pair
    // is keyed to the roster index and two surfaces that reorder the same
    // people would hand the same member two different hues.
    function memberIDs() {
      return members.map(function (m) { return m.id; });
    }

    // ── the type rail: the locked (hue · pattern · glyph) triple ──────────
    //
    // Seeded from THE PAGE PAYLOAD, per calendar, because
    // GET /calendars/:calId/event-categories sits behind an OWNER floor and a
    // Scribe cannot reach it ([DC-8](c) resolved to option ii).
    //
    // `data-de-category` DID NOT MOVE OFF THE NODE THE MODULE READS. It is a
    // hidden input now instead of a <select>, and `ed.category.value` is the
    // same read it always was — which is [ER-10] condition 1, met literally.
    function edTypeRail(cal) {
      if (!ed.typeRail) return;
      clear(ed.typeRail);
      var cats = [{ slug: '', name: 'No type' }].concat(
        (cal && cal.categories) ? cal.categories : []);
      cats.forEach(function (cat) {
        var b = mk('button', 'topt');
        b.type = 'button';
        b.setAttribute('data-type-pick', cat.slug || '');
        b.setAttribute('role', 'radio');
        if (cat.axis) b.style.setProperty('--axis', cat.axis);
        var rail = mk('i', 'rail ' + (cat.pattern || 'p1'));
        rail.setAttribute('aria-hidden', 'true');
        b.appendChild(rail);
        if (cat.glyph) {
          // THE GLYPH IS INKED --text-body BY THE SHEET, NEVER --axis. The
          // stills index measured axis-inked type glyphs at 2.27:1 and 3.06:1
          // in light and retired both sites by name; the rail beside it is
          // where the hue lives.
          var g = mk('span', 'g', cat.glyph);
          g.setAttribute('aria-hidden', 'true');
          b.appendChild(g);
        }
        b.appendChild(mk('span', 'nm', cat.name || cat.slug));
        ed.typeRail.appendChild(b);
      });
      edTypeSet(ed.category ? ed.category.value : '');
    }

    function edTypeSet(slug) {
      if (ed.category) ed.category.value = slug || '';
      if (ed.typeRail) {
        ed.typeRail.querySelectorAll('[data-type-pick]').forEach(function (b) {
          var on = b.getAttribute('data-type-pick') === (slug || '');
          pressed(b, on);
          b.setAttribute('aria-checked', on ? 'true' : 'false');
        });
      }
      edHeadMark();
      edPreview();
    }

    function edCategory(slug) {
      var cats = (edUI.cal && edUI.cal.categories) || [];
      for (var i = 0; i < cats.length; i++) if (cats[i].slug === slug) return cats[i];
      return null;
    }

    // THE HEAD RESTATES THE TYPE IN THREE CHANNELS. The 3px bar carries the
    // event's own hue; `.tymark` directly under it carries the SAME type's
    // stroke pattern and glyph. The stills index measured the old proximity
    // claim at 99px with three controls interposed and replaced the argument
    // with structure — 13.3px worst case, and structural here for the same
    // reason. `--accent` never inks this bar.
    function edHeadMark() {
      var cat = edCategory(ed.category ? ed.category.value : '');
      var axis = (cat && cat.axis) || '';
      if (ed.root) {
        if (axis) ed.root.style.setProperty('--axis', axis);
        else ed.root.style.setProperty('--axis', 'var(--own-none)');
      }
      if (ed.tyRail) ed.tyRail.className = 'rail ' + ((cat && cat.pattern) || 'p1');
      if (ed.tyGlyph) ed.tyGlyph.textContent = (cat && cat.glyph) || '';
    }

    // ── the date picker: a real month grid, week length DERIVED ───────────
    //
    // There is no literal week length here, in the template or in the sheet:
    // weekShape reads the cycle off the payload's own weekday names. The
    // INTERCALARY DAY IS A FULL-WIDTH ROW AND A REAL DATE — it carries its own
    // (year, month) from the producer, because Midwinter 1 is not Deepwinter 1
    // and an editor that pre-filled the rendered month for it would create the
    // event on the wrong date, silently, on every calendar with festival days.
    function edDatePicker(cal) {
      if (!ed.datePicker) return;
      clear(ed.datePicker);
      var week = edUI.week;
      if (week.len > 0) ed.datePicker.style.setProperty('--week-len', String(week.len));
      if (week.len > 0) {
        var head = mk('div', 'dp-head');
        week.names.forEach(function (n) {
          var h = mk('span', 'hd', n);
          h.setAttribute('title', n);
          head.appendChild(h);
        });
        ed.datePicker.appendChild(head);
      }
      var grid = mk('div', 'dp-grid');
      edUI.dates.forEach(function (d) {
        var ic = isIntercalary(d);
        var b = mk('button', ic ? 'dp-ic' : 'dp-c');
        b.type = 'button';
        // GUARD B4: every dated node carries the ANSWER key, in the dayKey
        // namespace the Block mints, so a partner surface can match it.
        b.setAttribute('data-day', d.key);
        b.setAttribute('data-day-pick', d.ord);
        b.setAttribute('aria-label', d.label || String(d.day));
        if (ic) {
          b.appendChild(mk('b', '', String(d.day)));
          b.appendChild(mk('span', 'cap', d.label || ''));
        } else {
          b.appendChild(mk('span', 'dn', String(d.day)));
          var first = (d.events && d.events[0]) || null;
          if (first) {
            // A day that already carries events says so in the event's own two
            // channels — hue and pattern — never in colour alone.
            var mkr = mk('span', 'mk ' + (first.pattern || 'p1'));
            if (first.axis) mkr.style.setProperty('--axis', first.axis);
            mkr.setAttribute('aria-hidden', 'true');
            b.appendChild(mkr);
          }
        }
        grid.appendChild(b);
      });
      ed.datePicker.appendChild(grid);
      edDateSet(edUI.dayKey, true);
    }

    function edDateFor(key) {
      for (var i = 0; i < edUI.dates.length; i++) {
        if (edUI.dates[i].key === key) return edUI.dates[i];
      }
      return null;
    }

    // edDateSet writes the three hidden coordinate fields AND re-resolves the
    // end date. `keepEnd` is false on a user pick, which is what makes an end
    // date unable to survive a start that moved past it.
    function edDateSet(key, keepEnd) {
      var d = edDateFor(key);
      if (!d) return;
      edUI.dayKey = key;
      setValue(ed.year, d.year);
      setValue(ed.month, d.month);
      setValue(ed.day, d.day);
      if (ed.datePicker) {
        ed.datePicker.querySelectorAll('[data-day-pick]').forEach(function (b) {
          pressed(b, b.getAttribute('data-day') === key);
        });
      }
      if (ed.dateRead) ed.dateRead.textContent = edDateLabel(d);
      // AN END DATE MAY NEVER PRECEDE ITS START. Moving the start past the end
      // clears the end rather than clamping it backwards, and a start ON the
      // intercalary day has no next date at all, so the field says "ends the
      // same day" instead of jumping to a numbered day earlier in the month.
      if (!keepEnd && edUI.endKey) {
        var order = edUI.dates.map(function (x) { return x.key; });
        if (order.indexOf(edUI.endKey) <= order.indexOf(key)) edUI.endKey = '';
      }
      edEndSet(edUI.endKey);
      edPreview();
    }

    function edDateLabel(d) {
      if (!d) return '';
      return d.weekday ? (d.label || '') + ' · ' + d.weekday : (d.label || '');
    }

    // ── the `Ends` cycler ─────────────────────────────────────────────────
    //
    // It advances over ONE ORDERED LIST of the month's dates, on which the
    // intercalary day is simply the last entry, and wraps to "ends the same
    // day" after it. That single construction is why `end < start` cannot
    // happen and why an intercalary START offers no end options.
    function edEndAdvance() {
      var from = edUI.endKey || edUI.dayKey;
      var next = nextDate(edUI.dates, from);
      edUI.endKey = next ? next.key : '';
      edEndSet(edUI.endKey);
      edPreview();
    }

    function edEndSet(key) {
      var d = key ? edDateFor(key) : null;
      edUI.endKey = d ? key : '';
      setValue(ed.endYear, d ? d.year : '');
      setValue(ed.endMonth, d ? d.month : '');
      setValue(ed.endDay, d ? d.day : '');
      if (ed.endRead) {
        // A CLOSED DISCLOSURE RENDERS A REAL SUMMARY LABEL, NEVER A BARE
        // CHEVRON — the register's clause 5, which this cycler is a consumer
        // of. "ends the same day" is a state, not a placeholder.
        clear(ed.endRead);
        ed.endRead.appendChild(mk('span', 'lb', d ? (d.label || '') : 'ends the same day'));
        var ar = mk('span', 'ar', '⌄');
        ar.setAttribute('aria-hidden', 'true');
        ed.endRead.appendChild(ar);
      }
    }

    // ── recurrence ────────────────────────────────────────────────────────

    function edRecUnits() {
      if (!ed.recUnits) return;
      clear(ed.recUnits);
      recurrenceUnits(edUI.week.len).forEach(function (u) {
        var b = mk('button', '');
        b.type = 'button';
        b.setAttribute('data-unit-pick', u.id);
        b.setAttribute('role', 'radio');
        b.appendChild(mk('span', '', u.label));
        if (!u.backed) {
          // `.badge.need` MEANS LITERALLY "needs backend" AND NOTHING ELSE. It
          // is here because `recurrence_type` does not accept this unit and
          // OccursOn would expand it to a SINGLE OCCURRENCE without saying so.
          b.appendChild(mk('span', 'badge need', 'needs backend'));
        }
        ed.recUnits.appendChild(b);
      });
      edRecPaint();
    }

    function edWdPick() {
      if (!ed.wdPick) return;
      clear(ed.wdPick);
      edUI.week.names.forEach(function (n, i) {
        var b = mk('button', '', n.slice(0, 2));
        b.type = 'button';
        b.setAttribute('data-wd-pick', String(i));
        b.setAttribute('aria-label', n);
        pressed(b, !!edUI.rec.wd[i]);
        ed.wdPick.appendChild(b);
      });
    }

    // edRecPaint is the ONE place recurrence state reaches the DOM, so the
    // segment, the interval, the unit list, the day-of-week box and the readout
    // can never disagree about what the rule currently says.
    function edRecPaint() {
      var r = edUI.rec;
      if (ed.recSeg) {
        ed.recSeg.querySelectorAll('[data-rec-pick]').forEach(function (b) {
          pressed(b, b.getAttribute('data-rec-pick') === r.mode);
        });
      }
      if (ed.recOn) ed.recOn.hidden = r.mode !== 'repeats';
      if (ed.recUnits) {
        ed.recUnits.querySelectorAll('[data-unit-pick]').forEach(function (b) {
          var on = b.getAttribute('data-unit-pick') === r.unit;
          pressed(b, on);
          b.setAttribute('aria-checked', on ? 'true' : 'false');
        });
      }
      // THE INTERVAL FIELD IS ABSENT FOR month AND year, NOT CHIPPED — and the
      // reason has CHANGED, so the note is rewritten rather than extended.
      // It used to read "OccursOn's monthly branch ignores RecurrenceInterval
      // entirely… there is nothing here for a backend to add"; C-SWEEP-R4 stage
      // 22 made that false for month, and C-CALV4-GAMEREADY §6 built year with
      // the interval applied from the first line. BOTH SERVERS NOW HONOUR IT.
      // The field stays hidden because this file has not been taught to AUTHOR
      // one — the readout wording and the inverse in recurrenceFromRecord come
      // with it — which is C-CALV4-MONTHLY-INTERVAL-CONTROL, booked by name.
      // Hidden-because-unbuilt is not the same as hidden-because-broken, and
      // the difference is the whole reason this comment exists.
      if (ed.recEvery) ed.recEvery.hidden = !recurrenceInterval(r.unit);
      if (ed.recBox) ed.recBox.hidden = !(r.mode === 'repeats' && r.unit === 'week');
      if (ed.recRead) ed.recRead.textContent = edRecReadout();
      edPreview();
    }

    // The readout is the AUTHOR'S OWN RULE, stated plainly. It says what the
    // server will actually do, which for an unbacked unit is "once" — the same
    // thing the chip beside the unit is promising to fix.
    function edRecReadout() {
      var body = recurrenceBody(edUI.rec);
      if (!body || !body.is_recurring) return 'once, on this date';
      if (body.recurrence_type === 'monthly') return 'every month, on this day of the month';
      // The yearly readout names the SKIP rule, because it is the one thing a
      // GM cannot see from the grid: a base day that does not exist in a later
      // year is passed over, never moved to a neighbouring day.
      if (body.recurrence_type === 'yearly') return 'every year, on this month and day (skipped in years without that day)';
      var n = body.recurrence_interval > 0 ? body.recurrence_interval
        : (body.recurrence_type === 'biweekly' ? 2 : 1);
      return 'every ' + (n === 1 ? '' : n + ' × ') + edUI.week.len + ' days, from this date';
    }

    // ── the restricted audience ───────────────────────────────────────────
    //
    // One row per member: hue swatch + LOCKED PATTERN + the RINGED two-letter
    // mark + the name + the role + a real allow/deny pair at the 24px floor.
    // The list that decides who may see a hidden event never speaks in colour
    // alone, and the mark is a ring rather than a filled disc because near-white
    // on a raw owner hue measured 1.72:1 in dark and 2.86:1 in light.
    function edAudience() {
      if (!ed.audRows) return;
      clear(ed.audRows);
      members.forEach(function (m) {
        var row = mk('div', 'mrow');
        if (m.axis) row.style.setProperty('--axis', m.axis);
        var sw = mk('span', 'swatch ' + (m.pattern || 'p1'));
        sw.setAttribute('aria-hidden', 'true');
        row.appendChild(sw);
        var ini = mk('span', 'inimark', m.initials || '');
        ini.setAttribute('aria-hidden', 'true');
        row.appendChild(ini);
        row.appendChild(mk('span', 'nm', m.name || m.id));
        row.appendChild(mk('span', 'mt', '· ' + (m.role || '')));
        row.appendChild(mk('span', 'sp'));
        var ad = mk('span', 'ad');
        var allow = mk('button', '', '✓ allow');
        allow.type = 'button';
        allow.setAttribute('data-aud-pick', 'allow:' + m.id);
        allow.setAttribute('aria-label', 'Allow ' + (m.name || m.id));
        var deny = mk('button', '', '✕ deny');
        deny.type = 'button';
        deny.setAttribute('data-aud-pick', 'deny:' + m.id);
        deny.setAttribute('aria-label', 'Deny ' + (m.name || m.id));
        ad.appendChild(allow);
        ad.appendChild(deny);
        row.appendChild(ad);
        ed.audRows.appendChild(row);
      });
      edAudPaint();
    }

    function edAudPaint() {
      if (!ed.audRows) return;
      ed.audRows.querySelectorAll('[data-aud-pick]').forEach(function (b) {
        var parts = String(b.getAttribute('data-aud-pick')).split(':');
        var on = !!edUI.aud[parts[1]];
        pressed(b, parts[0] === 'allow' ? on : !on);
      });
    }

    // ── visibility ────────────────────────────────────────────────────────
    //
    // The cards are REAL RADIOS and the module only reads them. The gate that
    // decides which cards exist is the PRODUCER'S, in markup, and there is no
    // branch here that could turn one on for a viewer the server did not render
    // it for.
    function edVisSet(mode) {
      edUI.vis = mode || 'public';
      if (ed.vis) {
        ed.vis.querySelectorAll('[data-vis-pick]').forEach(function (r) {
          r.checked = r.getAttribute('data-vis-pick') === edUI.vis;
        });
      }
      if (ed.aud) ed.aud.hidden = edUI.vis !== 'restricted';
      edPreview();
    }

    function edVisRead() {
      if (!ed.vis) return 'public';
      var found = 'public';
      ed.vis.querySelectorAll('[data-vis-pick]').forEach(function (r) {
        if (r.checked) found = r.getAttribute('data-vis-pick');
      });
      return found;
    }

    // ── the tie field ─────────────────────────────────────────────────────
    //
    // `entity_id` on create and update — both shipped writes bind it and
    // eventEditorRecord round-trips it. The picker READS
    // GET /campaigns/:id/entities/search, which is a cross-plugin FETCH and not
    // a cross-plugin IMPORT: check-plugin-isolation.sh polices Go imports, and a
    // client-side call to another plugin's public API breaks nothing.
    //
    // THE ✕ IS A REAL 24px BUTTON. A remove affordance is a control, not a
    // decoration, and the drawing's bare <span> measured 8.4 × 10.0px.
    function edTiePaint() {
      if (!ed.tieRow) return;
      clear(ed.tieRow);
      if (edUI.tie) {
        var pill = mk('span', 'pill tie');
        pill.appendChild(mk('span', '', '◎ ' + (edUI.tie.name || edUI.tie.id)));
        var x = mk('button', 'rmx', '✕');
        x.type = 'button';
        x.setAttribute('data-tie-pick', '');
        x.setAttribute('aria-label', 'Remove the tie to ' + (edUI.tie.name || edUI.tie.id));
        pill.appendChild(x);
        ed.tieRow.appendChild(pill);
      }
      if (ed.entity) ed.entity.value = edUI.tie ? edUI.tie.id : '';
      if (ed.tieSearch) ed.tieSearch.hidden = !!edUI.tie;
      if (ed.tieRes && edUI.tie) { ed.tieRes.hidden = true; clear(ed.tieRes); }
      edPreview();
    }

    function edTieSearch(q) {
      var fetcher = api();
      if (!fetcher || !ed.tieRes) return;
      if (!q || q.trim().length < 2) { ed.tieRes.hidden = true; clear(ed.tieRes); return; }
      var seq = ++edUI.tieSeq;
      fetcher('/campaigns/' + campaignID + '/entities/search?q=' + encodeURIComponent(q.trim()),
        { method: 'GET' })
        .then(function (resp) { return (resp && resp.ok) ? resp.json() : null; })
        .then(function (data) {
          // A STALE RESPONSE NEVER WINS. Typing produces overlapping reads and
          // the last one typed must be the one shown, not the last one to land.
          if (seq !== edUI.tieSeq || !ed.tieRes) return;
          clear(ed.tieRes);
          var list = (data && data.results) || [];
          if (!list.length) {
            ed.tieRes.appendChild(mk('span', 'none', 'No entity matches that.'));
          } else {
            list.slice(0, 8).forEach(function (item) {
              if (!item || !item.id) return;
              var b = mk('button', '', item.type_name
                ? item.name + ' · ' + item.type_name : item.name);
              b.type = 'button';
              b.setAttribute('data-tie-pick', item.id);
              b.setAttribute('data-tie-name', item.name || item.id);
              ed.tieRes.appendChild(b);
            });
          }
          ed.tieRes.hidden = false;
        })
        .catch(function () { /* a refused read is an empty picker, not a broken editor */ });
    }

    // ── the live preview ──────────────────────────────────────────────────
    //
    // A client-side render of marks the payload ALREADY CARRIES — the grid chip
    // and the Ledger row, both of which ship. It resolves no audience and names
    // no viewer: the "who sees it" roster depends on the composed audience,
    // which does not exist on main and is W-G's.
    function edPreview() {
      if (!ed.preview) return;
      clear(ed.preview);
      var cat = edCategory(ed.category ? ed.category.value : '');
      var day = edDateFor(edUI.dayKey);
      var vis = edUI.vis;
      var title = ed.name ? ed.name.value : '';
      var axis = (cat && cat.axis) || '';
      var pat = (cat && cat.pattern) || 'p1';
      var dayLb = day ? (isIntercalary(day) ? (day.label || '').slice(0, 1) : String(day.day)) : '';

      ed.preview.appendChild(mk('span', 'cap', 'Live preview — on the grid'));

      var cell = mk('div', 'pv-cell');
      if (day) cell.setAttribute('data-day', day.key);
      if (axis) cell.style.setProperty('--axis', axis);
      cell.appendChild(mk('span', 'dn', dayLb));
      var chip = mk('div', 'chip');
      var crail = mk('i', 'rail ' + pat);
      crail.setAttribute('aria-hidden', 'true');
      chip.appendChild(crail);
      if (cat && cat.glyph) {
        var ctok = mk('span', 'tok', cat.glyph);
        ctok.setAttribute('aria-hidden', 'true');
        chip.appendChild(ctok);
      }
      chip.appendChild(mk('span', 'lb', title));
      cell.appendChild(chip);
      // THE GOLD DOGEAR IS dm_only AND THE DIAMOND IS RESTRICTED. They are two
      // different marks for two different facts, and drawing one on both would
      // delete the distinction from the product one layer down.
      if (vis === 'gmonly') {
        var dg = mk('span', 'dogear');
        dg.setAttribute('aria-hidden', 'true');
        cell.appendChild(dg);
      }
      if (vis === 'restricted') {
        var am = mk('span', 'audmark');
        am.setAttribute('aria-hidden', 'true');
        cell.appendChild(am);
      }
      ed.preview.appendChild(cell);

      ed.preview.appendChild(mk('span', 'cap', 'In the Ledger'));
      var row = mk('div', 'lrow');
      if (day) row.setAttribute('data-day', day.key);
      if (axis) row.style.setProperty('--axis', axis);
      row.appendChild(mk('span', 'dg', dayLb));
      var rail = mk('i', 'rail ' + pat);
      rail.setAttribute('aria-hidden', 'true');
      row.appendChild(rail);
      if (vis === 'gmonly') {
        var gr = mk('i', 'gr');
        gr.setAttribute('title', 'hidden from players');
        gr.setAttribute('aria-hidden', 'true');
        row.appendChild(gr);
      }
      if (cat && cat.glyph) {
        var tok = mk('span', 'tok', cat.glyph);
        tok.setAttribute('aria-hidden', 'true');
        row.appendChild(tok);
      }
      var mid = mk('span', 'mid');
      mid.appendChild(mk('span', 'nm', title));
      if (vis === 'gmonly') mid.appendChild(mk('span', 'badge gm', 'GM'));
      if (vis === 'restricted') mid.appendChild(mk('span', 'audchip', '◈ Restricted'));
      row.appendChild(mid);
      ed.preview.appendChild(row);
    }

    function setValue(node, v) {
      if (!node) return;
      node.value = (v === null || v === undefined) ? '' : String(v);
    }

    // edFill loads a record into the whole chrome. The order matters: the
    // per-calendar structure (type rail, date grid, unit list, roster) is built
    // by edOpen BEFORE this runs, so every setter here has a control to write.
    function edFill(rec) {
      setValue(ed.name, rec.name || '');
      setValue(ed.desc, rec.description || '');
      edTypeSet(rec.category || '');
      setValue(ed.startH, rec.start_hour);
      setValue(ed.startM, rec.start_minute);
      setValue(ed.endH, rec.end_hour);
      setValue(ed.endM, rec.end_minute);
      var allDay = rec.all_day || rec.start_hour === null || rec.start_hour === undefined;
      if (ed.allDay) ed.allDay.checked = !!allDay;
      if (ed.timeRow) ed.timeRow.hidden = !!allDay;

      // THE DATE COMES FROM THE RECORD'S OWN COORDINATES, matched back onto the
      // ordered list. An intercalary day resolves to its OWN month, which is
      // why the payload carries (year, month) per day rather than deriving them.
      var startKey = edKeyFor(rec.year, rec.month, rec.day);
      edUI.dayKey = startKey || edUI.dayKey;
      edUI.endKey = edKeyFor(rec.end_year, rec.end_month, rec.end_day) || '';
      edDateSet(edUI.dayKey, true);

      edUI.rec = recurrenceFromRecord(rec, edUI.week.len);
      edUI.rec.wd = {};
      edUI.rec.touched = false;
      if (ed.recEvery) ed.recEvery.value = String(edUI.rec.every || 1);
      edWdPick();
      edRecPaint();

      var V = window.ChronicleCalVisibility || null;
      var mode = V ? V.modeFor(rec.visibility, rec.visibility_rules) : 'public';
      // A viewer without the Restricted card must not be shown a mode they
      // cannot author; the write path round-trips the stored pair for them, so
      // the editor opens on Public and never touches the audience.
      var canRestrict = !!ed.restricted;
      edUI.aud = audienceFromRules(
        V ? V.rulesToChips(rec.visibility_rules) : [], memberIDs());
      edVisSet(mode === 'specific' ? (canRestrict ? 'restricted' : 'public')
        : (mode === 'gmonly' && ed.gmOnly ? 'gmonly' : 'public'));
      edAudPaint();

      edUI.tie = rec.entity_id ? { id: rec.entity_id, name: rec.entity_name || rec.entity_id } : null;
      if (ed.tieSearch) ed.tieSearch.value = '';
      edTiePaint();

      if (ed.del) ed.del.hidden = !rec.id;
      edError('');
    }

    // edKeyFor matches a record's (year, month, day) back onto the month's own
    // ordered list. It is a MATCH, not a computation: an intercalary day's
    // month is not the rendered one, and re-deriving it here would be the
    // second copy of an adjacency rule block_geometry.go names as single.
    function edKeyFor(year, month, day) {
      if (year === null || year === undefined || month === null || month === undefined) return '';
      if (day === null || day === undefined) return '';
      for (var i = 0; i < edUI.dates.length; i++) {
        var d = edUI.dates[i];
        if (d.year === Number(year) && d.month === Number(month) && d.day === Number(day)) return d.key;
      }
      return '';
    }

    // edOpen's `doorID` is THE ID OF THE ROW THAT WAS CLICKED, and in edit mode
    // it is the only id the editor will act on (EDIT-MODE-ID-FALLBACK-3).
    //
    // It used to take the id from the SERVER's record — `rec && rec.id ?
    // rec.id : ''` — and edSave then computed `edit = mode === 'edit' &&
    // eventID`. A GET that returned a record without an `id` would therefore
    // make an EDIT-mode Save fall through to the CREATE branch and POST a
    // DUPLICATE event instead of PUTting the original, whose only symptom is a
    // second row appearing. Unreachable today (newEventEditorRecord always sets
    // ID on a non-omitempty json:"id"), and a probe confirmed the server's id
    // also silently WINS over the door's — correct by luck on both counts, on
    // the single line that decides PUT-vs-POST.
    function edOpen(mode, cal, day, rec, anchor, doorID) {
      // A fresh editor session is a fresh write. The flag survives a failed
      // save so the failing one cannot be double-submitted; it must not survive
      // the box being closed and opened again, or a network blip would leave the
      // editor permanently unable to save.
      edState.busy = false;
      // A fresh session also repaints Save live: a previous session's hung
      // write must not hand this one a disabled button (§7's freebie).
      edBusyPaint(false);
      // …and neither may a half-pressed Delete (§7 [GR-13]). edClose already
      // disarms; this covers the path that swaps one event's editor straight
      // into another's without a close in between.
      edDeleteDisarm();
      edState.mode = mode;
      edState.calId = cal.id;
      edState.day = day;
      edState.eventID = mode === 'edit' ? String(doorID || '') : '';
      edState.prev = mode === 'edit' ? rec : null;

      // THE PER-CALENDAR STRUCTURE IS BUILT BEFORE THE RECORD IS FILLED. The
      // week is DERIVED from this calendar's own weekday names, the date list
      // is its own ordered dates, and the unit list drops the week unit
      // entirely when there is no week — so every setter edFill runs has a
      // control to write and none of them has to guess a shape.
      edUI.cal = cal;
      edUI.dates = orderedDates(cal.list);
      edUI.week = weekShape(cal.list);
      edUI.dayKey = day.key || '';
      edUI.endKey = '';
      edTypeRail(cal);
      edDatePicker(cal);
      edRecUnits();
      edAudience();
      if (ed.dateLab) {
        ed.dateLab.textContent = cal.slug ? 'Date · ' + cal.slug : 'Date';
      }
      edFill(rec);
      // Delete targets the same id the save does, so the two can never disagree
      // about which event this editor session is holding.
      if (ed.del) ed.del.hidden = !edState.eventID;
      // …and so does the RSVP opt-in, which is why it reads edState.eventID
      // rather than `mode`: one id, one event, every control in this session.
      edRsvpPaint(rec);
      if (ed.head) {
        ed.head.textContent = (mode === 'edit' ? 'Edit event · ' : 'New event · ') + (day.label || '');
        ed.head.setAttribute('data-day', day.key || '');
      }
      // `draft` BEFORE THE POST HAS RETURNED AN ID. It is a readout of what the
      // record is, not a placeholder standing in for one — the same honesty the
      // absent Delete button on a draft carries.
      if (ed.idOut) ed.idOut.textContent = edState.eventID || 'draft';
      if (ed.save) ed.save.textContent = mode === 'edit' ? 'Save changes' : 'Create event';
      // THE CARD'S RECT IS MEASURED WHILE IT IS STILL THE OPEN BOX, before
      // anything closes it — that rect is the morph's start geometry and the
      // one thing this path reads from outside the editor.
      var fromRect = edMorphRect(anchor);
      // …AND SO IS THE PLACEMENT LAW'S ANCHOR, FOR THE SAME REASON AND ONE LINE
      // LATER THAN IT USED TO BE (C-CALV4-CARD-REDUCED-ANCHOR).
      //
      // edPosition used to be handed the LIVE element and measure it AFTER
      // `closeCard()`. With motion allowed that is harmless — `closeDelayMS`
      // returns --disc-close and the card is still on screen at its own rect
      // when the read happens. Under prefers-reduced-motion it is the whole
      // defect: `closeDelayMS` returns 0 BY DESIGN (the sheet declares no
      // transition, so waiting would leave a fully-styled card sitting there
      // after it was logically closed), `hide()` therefore runs SYNCHRONOUSLY
      // inside `closeCard()`, and it both strips the inline geometry and calls
      // `hidePopover()`. The next getBoundingClientRect on that element answers
      // 0×0 at 0,0 — so `placeCard` did its job perfectly over an anchor that no
      // longer existed and put the editor at (8,8), the viewport's top-left,
      // half a screen from the day it belongs to.
      //
      // FROZEN, NOT RE-READ. The rect is captured while the card is still the
      // open box and handed to edPosition through the same one-shot shim the
      // drag path and the morph seed already use, so there is exactly one
      // measurement of the anchor per open and no ordering can invalidate it.
      // `placeCard` is not touched — [ER-5]'s STOP-AND-FLAG carve-out stays
      // closed; this is a reordering inside edOpen.
      var anchorRect = (anchor && anchor.getBoundingClientRect)
        ? anchor.getBoundingClientRect() : null;
      // The card leaves first: it cross-fades out on the mechanism it already
      // had, over --disc-close, while the editor grows over --disc-open.
      closeCard();
      edState.open = true;
      edShow();
      var placed = anchorRect ? edPosition(
        { getBoundingClientRect: function () { return anchorRect; } }) : null;
      // FORCED REFLOW, DELIBERATELY, and now it carries two jobs. The editor
      // was display:none a moment ago, so it has no rendered before-change
      // style and a transition started in the same frame would not run at all
      // — which is what makes the register's reveal fire on a popover without
      // @starting-style, a standing refusal. The morph's seeded start geometry
      // needs the same flush for the same reason.
      void ed.root.offsetHeight;
      var morphing = (fromRect && placed) ? edMorphSeed(
        { getBoundingClientRect: function () { return fromRect; } },
        placed.size, placed.at) : false;
      if (morphing) {
        // ── THE ORDER IS THE MORPH, AND GETTING IT WRONG COSTS THE WHOLE
        //    OPEN DIRECTION ─────────────────────────────────────────────────
        //
        // SEED → FLUSH → CLASS → FLUSH → FINAL WRITE. Every arrow is load
        // bearing and the FIRST one is the one this shipped without.
        //
        // WHAT WENT WRONG WITHOUT IT, MEASURED IN REAL CHROMIUM RATHER THAN
        // REASONED. The seeded geometry and `.edmorph` used to arrive in the
        // SAME style recalc. CSS Transitions start a transition from the
        // AFTER-change style, so that single recalc read `transition-property:
        // inline-size, block-size, translate, opacity` for the first time
        // WHILE the values were changing — and started 160ms transitions
        // running AWAY from the resting box and TOWARD the seed. The seed
        // therefore never became the settled before-change style:
        // getComputedStyle answered the RESTING box at opacity 1, the
        // final write that followed in the same task saw nothing to change,
        // and no transition to the placed geometry was ever created. The
        // editor POPPED IN at full size and animated only on the way out. The
        // one animation the page really had was a no-op `height` CSSTransition
        // from `calc-size(fit-content, 0px + size)` to itself, which is
        // exactly what the capture rig kept reporting and nobody believed.
        //
        // WITH THE FLUSH the seed lands as a settled style with NO transition
        // declared on it (there is no `.edmorph` yet, so nothing can start),
        // the class then declares the four properties over values that are not
        // changing, and only the final write — the third recalc — has both a
        // declared transition and a changed value. That is the one that runs.
        //
        // THIS IS BROWSER-GENERAL. It follows from the after-change-style rule,
        // not from anything Chromium does on its own, and
        // TestDayCardMorphInterpolates samples the flight frame by frame in a
        // real browser so the ordering cannot silently come apart again.
        void ed.root.offsetHeight;
        // THE CLASS GOES ON AFTER THE START GEOMETRY, so the browser records
        // the card's rect as the transition's from-value rather than animating
        // the box INTO the card on the way in. The flush after it is what makes
        // that recording happen at all — a transition started in the same
        // frame as the style that declared it does not run.
        ed.root.classList.add('edmorph');
        void ed.root.offsetHeight;
      }
      ed.root.classList.add('dcopen');
      if (morphing) {
        edMorphWrite(edState.morph, false);
        // THE SETTLE IS SCHEDULED OFF --disc-open, READ FROM THE SHEET rather
        // than carried as a copy of 200 — the same discipline closeDelayMS
        // already applies to the close.
        edState.morphTimer = setTimeout(edMorphSettle,
          durationMS(cssVar('--disc-open'), 200));
      } else {
        edState.morph = null;
      }
    }

    // edPosition RETURNS what it measured and where it put the box. The morph
    // needs both, and re-measuring afterwards would read a collapsed inner box
    // — see edMorphSeed's header for the frames that proved it.
    function edPosition(anchor) {
      if (!anchor.getBoundingClientRect) return null;
      var view = { w: window.innerWidth || 0, h: window.innerHeight || 0 };
      var chrome = (ed.root.offsetHeight || 0) - (ed.box ? ed.box.offsetHeight || 0 : 0);
      var size = {
        w: ed.root.offsetWidth || 0,
        h: (ed.box ? ed.box.scrollHeight || 0 : 0) + (chrome > 0 ? chrome : 0),
      };
      var at = applyPlacement(ed.root,
        placeCard(anchor.getBoundingClientRect(), size, view, ledgerRect(state.host), {}),
        reportOcclusion);
      return { size: size, at: at };
    }

    function api() {
      return (window.Chronicle && window.Chronicle.apiFetch) ? window.Chronicle.apiFetch : null;
    }

    function eventsBase() {
      return '/campaigns/' + campaignID + '/calendars/' + edState.calId + '/events';
    }

    function edFormValues() {
      var vis = edVisRead();
      return {
        name: ed.name ? ed.name.value : '',
        description: ed.desc ? ed.desc.value : '',
        category: ed.category ? ed.category.value : '',
        year: ed.year ? ed.year.value : '',
        month: ed.month ? ed.month.value : '',
        day: ed.day ? ed.day.value : '',
        allDay: ed.allDay ? !!ed.allDay.checked : true,
        startHour: ed.startH ? ed.startH.value : '',
        startMinute: ed.startM ? ed.startM.value : '',
        endHour: ed.endH ? ed.endH.value : '',
        endMinute: ed.endM ? ed.endM.value : '',
        endYear: ed.endYear ? ed.endYear.value : '',
        endMonth: ed.endMonth ? ed.endMonth.value : '',
        endDay: ed.endDay ? ed.endDay.value : '',
        // BOTH ARE EMITTED, AND `gmOnly` IS NOT VESTIGIAL. It keeps the pure
        // mapper's stage-2 contract exactly as it was for every existing case
        // ([ER-10] condition 2), and `vis` is what the third mode rides on.
        gmOnly: vis === 'gmonly',
        vis: vis,
        audience: audienceToChips(edUI.aud, memberIDs()),
        // NULL WHEN UNTOUCHED. buildEventBody then falls to the stage-2 round
        // trip, so a title-only save on an event whose stored type Chronicle
        // does not accept leaves that rule exactly as it found it.
        recurrence: edUI.rec.touched
          ? { mode: edUI.rec.mode, every: edUI.rec.every, unit: edUI.rec.unit } : null,
        entityID: ed.entity ? ed.entity.value : '',
      };
    }

    // edFailed re-opens the editor for another attempt. The busy flag is
    // cleared HERE and only here: every success path ends in a reload, and
    // clearing on success would leave a live Save button on a page that is
    // already navigating.
    function edFailed(msg) {
      edState.busy = false;
      edBusyPaint(false);
      edError(msg);
    }

    // ── SAVE HAS A BUSY STATE (C-CALV4-GAMEREADY §7 [GR-13], the same-handler
    //    freebie) ─────────────────────────────────────────────────────────────
    //
    // `edState.busy` was invisible: it is cleared only in edFailed, because
    // every success path ends in window.location.reload(). So a save whose
    // fetch NEVER RESOLVES left a button that still looked live, still had
    // `disabled === false`, carried no `aria-busy`, and sat under the PREVIOUS
    // failure's error text. Two more taps fired zero requests and said nothing.
    // A GM who taps Save and gets a dead button under an old error will assume
    // the event saved.
    //
    // Three parts, all inside the editor's own box: paint the button disabled
    // + `aria-busy` while the write is in flight, clear the stale error at the
    // START of a save rather than only on the next failure, and give the fetch
    // a ~15s ceiling so a hung request becomes a stated failure instead of a
    // permanent one.
    var edSaveTimeoutMs = 15000;

    function edBusyPaint(on) {
      if (!ed) return;
      if (ed.save) {
        ed.save.disabled = !!on;
        if (on) ed.save.setAttribute('aria-busy', 'true');
        else ed.save.removeAttribute('aria-busy');
      }
      if (!on && edState.saveTimer) { clearTimeout(edState.saveTimer); edState.saveTimer = 0; }
    }

    function edSave() {
      if (edState.busy) return;
      var fetcher = api();
      if (!fetcher) { edError('Cannot reach Chronicle right now.'); return; }
      var form = edFormValues();
      if (!String(form.name).trim()) { edError('An event needs a title.'); return; }
      if (form.month === '' || form.day === '' || form.year === '') {
        edError('This day has no resolvable date; pick one before saving.');
        return;
      }
      // EDIT MODE REFUSES RATHER THAN FALLING THROUGH TO CREATE
      // (EDIT-MODE-ID-FALLBACK-3). An edit session that has lost its id is a
      // bug, and the one thing it must never do is quietly become a POST — a
      // duplicated event is silent data corruption whose only symptom is a
      // second row, days later.
      var target = writeTarget(edState.mode, edState.eventID);
      if (!target) {
        edError('This editor lost the event it was holding. Close it and click the row again.');
        return;
      }
      var body = buildEventBody(form, edState.prev, { canOfferGMOnly: !!ed.gmOnly });
      // Set AFTER validation, so a rejected title does not lock the editor.
      edState.busy = true;
      // THE PREVIOUS FAILURE'S SENTENCE IS NOT THIS SAVE'S STATE. Clearing it
      // here means a retry never runs under stale text that reads as if the
      // new attempt had already failed.
      edError('');
      edBusyPaint(true);
      edState.saveTimer = setTimeout(function () {
        if (!edState.busy) return;
        edFailed('That save is taking too long. Nothing has been confirmed — try again.');
      }, edSaveTimeoutMs);
      fetcher(target.eventID ? eventsBase() + '/' + target.eventID : eventsBase(), {
        method: target.method, body: body,
      }).then(function (resp) {
        if (resp && resp.ok) { window.location.reload(); return; }
        edFailed('That did not save. Check the date and try again.');
      }).catch(function () {
        edFailed('That did not save. Check your connection and try again.');
      });
    }

    // ── DELETE ARMS ITSELF IN PLACE (C-CALV4-GAMEREADY §7 [GR-13]) ────────
    //
    // The repository does `DELETE FROM calendar_events WHERE id = ?` — no soft
    // delete, no restore path — and this button sits at a 24px tap floor on a
    // phone, on the line ABOVE Save in the same delegated handler
    // (`[data-de-delete]` then `[data-de-save]`). The danger is not "the user
    // did not mean to delete"; it is "the user meant to hit Save".
    //
    // So the first click ARMS: the label becomes `Confirm delete` and a ~4s
    // timer starts. A second click inside the window sends the DELETE. The
    // timer expiring, the editor closing, or ANY other editor interaction
    // disarms it. That turns a mis-tap into a visible label change one pixel
    // from where the thumb was aiming, which is a better signal than a modal
    // the user dismisses reflexively.
    //
    // NO DIALOG, NO `window.confirm`, NO DOM OUTSIDE THE EDITOR'S OWN BOX.
    // This module's own boundary rule (see the delegated handler below: "EVERY
    // CHROME CONTROL IS HANDLED INSIDE THE EDITOR'S OWN SUBTREE AND NOWHERE
    // ELSE") forbids the first; the editor sheet is `position: fixed` and
    // already puts Save under a software keyboard, so a second fixed layer on
    // top of it would be a second trap rather than a confirmation.
    //
    // IT IS A TEXT SWAP AND NOT A TRANSITION. Nothing here animates: the ONE
    // motion register in calendar-bench.css is untouched by design.
    var edDeleteArmMs = 4000;
    var edDeleteRestLabel = '';

    // edDeleteDisarm returns the button to its resting label. Safe to call at
    // any time, including when nothing is armed — every disarm path funnels
    // here so there is exactly one place that can restore the label.
    function edDeleteDisarm() {
      if (edState.delTimer) { clearTimeout(edState.delTimer); edState.delTimer = 0; }
      if (!edState.delArmed) return;
      edState.delArmed = false;
      if (ed && ed.del && edDeleteRestLabel) ed.del.textContent = edDeleteRestLabel;
    }

    function edDeleteArm() {
      if (!ed || !ed.del) return;
      if (!edDeleteRestLabel) edDeleteRestLabel = ed.del.textContent || 'Delete';
      edState.delArmed = true;
      // `aria-live` sits on the button itself so the swap is ANNOUNCED rather
      // than only seen — the two-step is a state change, and a screen-reader
      // user who cannot see the label change would otherwise meet a button
      // that silently did nothing on the first press.
      ed.del.setAttribute('aria-live', 'polite');
      ed.del.textContent = 'Confirm delete';
      if (edState.delTimer) clearTimeout(edState.delTimer);
      edState.delTimer = setTimeout(edDeleteDisarm, edDeleteArmMs);
    }

    function edDelete() {
      if (edState.busy) return;
      var fetcher = api();
      if (!fetcher || !edState.eventID) return;
      // FIRST CLICK SENDS NOTHING. The arm happens after the fetcher and id
      // checks so a button that could never delete anything does not pretend
      // to arm.
      if (!edState.delArmed) { edDeleteArm(); return; }
      edDeleteDisarm();
      edState.busy = true;
      fetcher(eventsBase() + '/' + edState.eventID, { method: 'DELETE' })
        .then(function (resp) {
          if (resp && resp.ok) { window.location.reload(); return; }
          edFailed('That event could not be deleted.');
        })
        .catch(function () { edFailed('That event could not be deleted.'); });
    }

    // ── COLLECT RSVPs (C-CALV4-GAMEREADY §4 [GR-6]) ──────────────────────
    //
    // THE OPERATOR'S OWN GATE FOR STARTING A GAME, which until this slice lived
    // ONLY in the legacy V2 event drawer — a committed deletion. It writes the
    // already-shipped PUT .../events/:eid/rsvp-collection and nothing else.

    // rsvpCreateHint is the V2 drawer's own wording, VERBATIM
    // (calendar_v2.templ:1384). Reused rather than rewritten because it is
    // already the right sentence and the operator has already read it once.
    var rsvpCreateHint = 'Save the event first, then invite the party';

    // rsvpRecurringCaution is the one-line caution [GR-6] requires on a
    // repeating event. The RSVP table is UNIQUE (event_id, user_id) with NO
    // occurrence column (migration 013), so every occurrence of a recurring
    // session shares ONE set of answers: after week one the tally shows last
    // week's replies and nobody can reset them. The real fix is a schema change
    // and is BOOKED (C-CALV4-RSVP-OCCURRENCE); until then the operator is told
    // rather than left to find out at the table.
    var rsvpRecurringCaution = 'This event repeats, and every occurrence shares one set of answers.';

    // edRsvpPaint sets the control from the record. It is called by edOpen for
    // every mode, and the FIRST LINE is the audience gate: below RoleScribe the
    // producer renders no markup, so there is nothing here to paint.
    //
    // DISABLED IN CREATE MODE IS A SEQUENCE FACT, NOT A PERMISSION ONE.
    // collect_rsvps is deliberately off the shared update path, so it cannot
    // ride the create payload — there is no event id to collect against yet.
    function edRsvpPaint(rec) {
      if (!ed.rsvp) return;
      var editing = !!edState.eventID;
      ed.rsvp.disabled = !editing;
      ed.rsvp.checked = editing && !!(rec && rec.collect_rsvps);
      if (!ed.rsvpHint) return;
      if (!editing) { ed.rsvpHint.textContent = rsvpCreateHint; return; }
      ed.rsvpHint.textContent = ed.rsvp.checked
        ? 'The party can answer in-app and by email.'
        : 'Emails the party once, and opens answers in-app.';
      if (rec && rec.is_recurring) {
        ed.rsvpHint.textContent += ' ' + rsvpRecurringCaution;
      }
    }

    // edRsvpWrite flips the opt-in through the SHIPPED endpoint.
    //
    // IT DOES NOT RELOAD THE PAGE, unlike save and delete: arming RSVPs changes
    // no date, no title and no mark, so throwing the GM's open editor away to
    // re-render an identical grid would cost them their place for nothing.
    //
    // THE RESPONSE IS READ, NOT ASSUMED ([GR-10]). The V2 client printed "RSVPs
    // are open — the party has been invited." unconditionally, including when
    // the server sent ZERO email because no SMTP is configured — so the operator
    // armed their gate, believed the sentence, stopped checking, and found out
    // on the day of the session. The endpoint now reports its mail state and
    // this prints what it says, never a sentence of its own invention.
    function edRsvpWrite() {
      if (!ed.rsvp || !edState.eventID) return;
      var fetcher = api();
      var want = !!ed.rsvp.checked;
      if (!fetcher) { ed.rsvp.checked = !want; edError('Cannot reach Chronicle right now.'); return; }
      ed.rsvp.disabled = true;
      fetcher(eventsBase() + '/' + edState.eventID + '/rsvp-collection', {
        method: 'PUT', body: { enabled: want },
      }).then(function (resp) {
        ed.rsvp.disabled = false;
        if (!resp || !resp.ok) {
          // THE CONTROL GOES BACK TO WHAT THE SERVER STILL HOLDS. A checkbox
          // left showing a state the server refused is the same lie in a
          // smaller font.
          ed.rsvp.checked = !want;
          edError('That could not be changed. Try again.');
          return null;
        }
        return resp.json ? resp.json() : null;
      }).then(function (body) {
        if (!ed.rsvpHint || !ed.rsvp.checked) { return; }
        if (body && body.notice) { ed.rsvpHint.textContent = body.notice; return; }
        ed.rsvpHint.textContent = 'The party can answer in-app and by email.';
      }).catch(function () {
        ed.rsvp.disabled = false;
        ed.rsvp.checked = !want;
        edError('That could not be changed. Check your connection and try again.');
      });
    }

    // edLoad fetches the full record for EDIT mode. It is the one read this
    // slice added, and it is the only thing the editor genuinely cannot get
    // from the page: the card's payload deliberately carries no description, no
    // visibility_rules and no recurrence.
    function edLoad(cal, day, eventID, anchor) {
      var fetcher = api();
      // A door with no id is not an edit; it is a malformed row, and reading
      // `/events/` would be a request for the collection rather than a record.
      if (!fetcher || !eventID) return;
      fetcher('/campaigns/' + campaignID + '/calendars/' + cal.id + '/events/' + eventID, { method: 'GET' })
        .then(function (resp) {
          if (!resp || !resp.ok) return;
          return resp.json();
        })
        .then(function (rec) {
          if (!rec) return;
          edOpen('edit', cal, day, rec, anchor, eventID);
        })
        .catch(function () { /* a refused read is a closed card, not a broken page */ });
    }

    // --- wiring ------------------------------------------------------------
    //
    // DELEGATED, so an HTMX re-settle of the row grid cannot orphan a listener,
    // and guarded once on the card itself (dataset.dcWired) so a re-init cannot
    // double-bind — the QA2 class of bug event_grid.js carries per-node guards
    // for at :250, :403 and :520.

    // ── ONE PANEL AT A TIME (C-CALV4-MOONS MN-G13, coordinated with
    //    C-CALV4-EDITOR-MODALITY-SPAN) ────────────────────────────────────
    //
    // The day cell's moon disc cluster became a control in C-CALV4-MOONS: it is
    // a <label> for a hidden radio, and pressing it folds the two-tab moon
    // panel open at the row. That label is INSIDE the cell, so without this
    // clause the same click would also open the day card, and the two would sit
    // over each other disagreeing — the exact failure
    // C-CALV4-EDITOR-MODALITY-SPAN exists because of, when the editor and the
    // card were open together.
    //
    // IT IS A REFUSAL TO OPEN AND NOTHING ELSE. This module still inserts no
    // node inside `.cal-block-host`, adds and removes no class there, and
    // animates nothing there — daycard_block_immutability.test.mjs is untouched
    // by it. `[data-cal-moons]` is the cluster's own hook and
    // `[data-cal-moonpanel]` is the panel's; both are the Block's namespace and
    // both are read, never written.
    // TWO closest() CALLS RATHER THAN ONE COMMA-SEPARATED SELECTOR: the module
    // is exercised against test/js/daycard_dom.mjs, a hand-written DOM whose
    // selector parser handles compound selectors and not selector LISTS. A
    // list here parses to nothing and the interlock silently stops holding —
    // which is the failure mode this whole file is written to avoid.
    var MOON_CLUSTER = '[data-cal-moons]';
    var MOON_PANEL = '[data-cal-moonpanel]';

    function cellFrom(target) {
      if (!target || !target.closest) return null;
      if (target.closest('[data-cal-daycard]')) return null;
      if (target.closest(MOON_CLUSTER)) return null;
      if (target.closest(MOON_PANEL)) return null;
      var cell = target.closest('[data-day][data-day-ord]');
      if (!cell) return null;
      var host = cell.closest('[data-bench-block]');
      return host ? { cell: cell, host: host } : null;
    }

    document.addEventListener('click', function (e) {
      // THE CLICK A REAL DRAG LEAVES BEHIND, consumed exactly once (stage 4,
      // [DC-11] term 5). A drag of zero cells never sets this, so the
      // single-day click is untouched — which is the term, stated as code.
      if (drag.eatClick) { drag.eatClick = false; return; }
      if (e.target && e.target.closest && e.target.closest('[data-dc-ledger]')) {
        e.preventDefault();
        openInLedger();
        return;
      }
      if (canEdit && e.target && e.target.closest) {
        var newBtn = e.target.closest('[data-dc-new]');
        if (newBtn) {
          e.preventDefault();
          var calNew = index[state.calId];
          var dayNew = calNew && calNew.days[state.key];
          if (calNew && dayNew) {
            edOpen('create', calNew, dayNew, {
              year: dayNew.year, month: dayNew.month, day: dayNew.day, all_day: true,
            }, card);
          }
          return;
        }
        var editBtn = e.target.closest('[data-dc-edit]');
        if (editBtn) {
          e.preventDefault();
          var calEd = index[state.calId];
          var dayEd = calEd && calEd.days[state.key];
          if (calEd && dayEd) edLoad(calEd, dayEd, editBtn.getAttribute('data-dc-edit'), card);
          return;
        }
        // ANY CLICK THAT IS NOT ON DELETE DISARMS IT (§7 [GR-13]). This sits
        // ABOVE the branch chain so it covers Cancel, Save, every chrome
        // control in the editor's subtree, and every click elsewhere on the
        // page — the arming window is for a second press on the SAME button
        // and nothing else, so any other intent the user expresses cancels it.
        if (edState.delArmed && !e.target.closest('[data-de-delete]')) edDeleteDisarm();
          if (e.target.closest('[data-de-cancel]')) { e.preventDefault(); edClose(); return; }
        if (e.target.closest('[data-de-delete]')) { e.preventDefault(); edDelete(); return; }
        if (e.target.closest('[data-de-save]')) { e.preventDefault(); edSave(); return; }
        // EVERY CHROME CONTROL IS HANDLED INSIDE THE EDITOR'S OWN SUBTREE AND
        // NOWHERE ELSE. The scoping is not cosmetic: the date grid's buttons
        // carry `data-day` and `data-day-pick`, which are the BLOCK's own key
        // namespaces, and a handler that matched them page-wide would make an
        // editor cell open the card behind it. `cellFrom` cannot reach them
        // either — it requires `[data-day][data-day-ord]` and these carry no
        // ordinal — but relying on that would be relying on an absence.
        if (e.target.closest('[data-cal-dayeditor]')) {
          if (edControl(e.target)) e.preventDefault();
          return;
        }
      }
      var hit = cellFrom(e.target);
      if (hit) { openCard(hit.host, hit.cell); return; }
      // Outside the card and outside a day: dismiss. The module owns dismissal
      // because popover=manual, and it owns it so the register's close runs on
      // every path rather than only on the button.
      if (state.open && e.target && e.target.closest && !e.target.closest('[data-cal-daycard]')) {
        closeCard();
      }
    });

    // The editor's half of [MOB-3]'s `toggle` truth. It is wired HERE rather
    // than beside the card's because `ed` is built later in this mount and a
    // listener attached against a hoisted `undefined` is a listener on nothing.
    if (ed && ed.root && ed.root.addEventListener) {
      ed.root.addEventListener('toggle', function (e) {
        sheetOpenChanged('editor', (e && typeof e.newState === 'string')
          ? e.newState === 'open'
          : ed.root.hasAttribute('data-dc-shown'));
      });
    }

    // The all-day toggle reveals the in-world time row. It is a display switch
    // on a field the API has ALWAYS had — the mockup's `needs backend` chip
    // over start_hour/start_minute was simply wrong and it comes off rather
    // than being ported.
    if (ed && ed.allDay) {
      ed.allDay.addEventListener('change', function () {
        if (ed.timeRow) ed.timeRow.hidden = !!ed.allDay.checked;
      });
    }
    // The RSVP opt-in writes on CHANGE, not on click: `change` is the event a
    // real checkbox emits for a mouse click, a spacebar press and a click on the
    // label alike, so the keyboard path is the same path and there is nothing to
    // route through edControl.
    if (ed && ed.rsvp) {
      ed.rsvp.addEventListener('change', function () { edRsvpWrite(); });
    }
    if (ed && ed.form) {
      ed.form.addEventListener('submit', function (e) { e.preventDefault(); edSave(); });
    }

    // edControl is the chrome's one click router. It returns true when it
    // consumed the event, so the caller preventDefaults exactly the clicks that
    // did something — a blanket preventDefault inside the editor would break
    // text selection in the description and the native label-to-radio path the
    // visibility cards depend on.
    function edControl(target) {
      if (!target || !target.closest) return false;
      var t = target.closest('[data-type-pick]');
      if (t) { edTypeSet(t.getAttribute('data-type-pick')); return true; }
      var d = target.closest('[data-day-pick]');
      if (d) { edDateSet(d.getAttribute('data-day'), false); return true; }
      if (target.closest('[data-end-pick]')) { edEndAdvance(); return true; }
      var r = target.closest('[data-rec-pick]');
      if (r) {
        edUI.rec.mode = r.getAttribute('data-rec-pick');
        edUI.rec.touched = true;
        edRecPaint();
        return true;
      }
      var u = target.closest('[data-unit-pick]');
      if (u) {
        edUI.rec.unit = u.getAttribute('data-unit-pick');
        edUI.rec.touched = true;
        edRecPaint();
        return true;
      }
      var w = target.closest('[data-wd-pick]');
      if (w) {
        var i = w.getAttribute('data-wd-pick');
        edUI.rec.wd[i] = !edUI.rec.wd[i];
        pressed(w, edUI.rec.wd[i]);
        return true;
      }
      var a = target.closest('[data-aud-pick]');
      if (a) {
        var parts = String(a.getAttribute('data-aud-pick')).split(':');
        edUI.aud[parts[1]] = parts[0] === 'allow';
        edAudPaint();
        return true;
      }
      var tie = target.closest('[data-tie-pick]');
      if (tie) {
        var id = tie.getAttribute('data-tie-pick');
        edUI.tie = id ? { id: id, name: tie.getAttribute('data-tie-name') || id } : null;
        if (ed.tieSearch) ed.tieSearch.value = '';
        edTiePaint();
        return true;
      }
      return false;
    }

    // THE VISIBILITY CARDS ARE REAL RADIOS, so their state change is the
    // browser's and this listener only READS it. Nothing here decides a gate:
    // a card the producer did not render has no radio to fire.
    if (ed && ed.vis) {
      ed.vis.addEventListener('change', function () { edVisSet(edVisRead()); });
    }
    if (ed && ed.name) {
      ed.name.addEventListener('input', function () { edPreview(); });
    }
    if (ed && ed.recEvery) {
      ed.recEvery.addEventListener('input', function () {
        var n = Math.floor(Number(ed.recEvery.value));
        edUI.rec.every = (isFinite(n) && n > 0) ? n : 1;
        edUI.rec.touched = true;
        edRecPaint();
      });
    }
    if (ed && ed.tieSearch) {
      // DEBOUNCED, and the debounce is the reason the picker is affordable at
      // all: a read per keystroke against a search endpoint is a cost the
      // editor has no business imposing on every campaign.
      ed.tieSearch.addEventListener('input', function () {
        if (edUI.tieTimer) clearTimeout(edUI.tieTimer);
        var q = ed.tieSearch.value;
        edUI.tieTimer = setTimeout(function () {
          edUI.tieTimer = 0;
          edTieSearch(q);
        }, 200);
      });
    }

    // THE SECOND OPENER ([DC-4] SIGNED): the day radio's `change`. Where the
    // Ledger is docked the day HAS a real focusable control, so keyboard
    // selection opens the card for free. Where it is NOT docked, dayPick emits
    // no radio and no label at all (instrument.templ:213) and the card is
    // POINTER-ONLY for that viewer. That gap is real, it is stated in the
    // slice's report, and it is booked as C-CALV4-DAYPICK-A11Y — it is a widget
    // change (a day cell needs a focusable control that is not gated on the
    // Ledger) and injecting tabindex from here would be both the mutation this
    // module refuses and a control the server never rendered.
    document.addEventListener('change', function (e) {
      // TYPING OR TOGGLING ANYTHING DISARMS DELETE (§7 [GR-13]). The click
      // path is covered in the delegated handler above; this is the keyboard
      // and form-control half of "any other editor interaction".
      if (edState.delArmed) edDeleteDisarm();
      if (state.suppress) return;
      var t = e.target;
      if (!t || !t.matches || !t.matches('input.daypick[data-day-pick]')) return;
      var host = t.closest && t.closest('[data-bench-block]');
      if (!host) return;
      var ord = t.getAttribute('data-day-pick');
      var cell = host.querySelector('[data-day][data-day-ord="' + (ordIsSafe(ord) ? ord : '') + '"]');
      if (cell) openCard(host, cell);
    });

    document.addEventListener('keydown', function (e) {
      if (e.key !== 'Escape') return;
      if (edState.open) { edClose(); return; }
      if (state.open) closeCard();
    });

    // ── DRAG-CREATE — C-CALV4-EDITOR-R2b stage 4, [DC-11] SIGNED ─────────
    //
    // SEVERABLE BY CONSTRUCTION, on the ruling's seven terms, and every one of
    // them is a property of this block rather than a promise about it:
    //
    //  1. IT IS THE LAST STAGE. The chrome and the morph are complete and
    //     shippable without a line of it.
    //  2. IT ADDS CODE TO ONE FILE AND ITS OWN TEST. It changes nothing in
    //     bench.templ, bench.go, routes.go or the handler, and nothing in any
    //     stylesheet beyond APPENDING its own drag rules.
    //  3. IT IS REVERTIBLE BY DELETING ONE COMMIT, verified by doing it.
    //  4. IT CREATES THROUGH THE EXISTING POST. end_year / end_month / end_day
    //     already ship and the chrome already writes them; this fills the same
    //     three hidden fields a pointer would. ZERO NEW API.
    //  5. IT MAY NOT REGRESS THE DAY CLICK, and A DRAG OF ZERO CELLS IS A
    //     CLICK — not "a drag that opens the editor with one day selected".
    //     The single-day case falls through untouched to the shipped opener.
    //  6. IT IS SCRIBE+ AND POINTER-ONLY. `canEdit` is the producer's gate, so
    //     a player never receives the listener at all. THERE IS NO KEYBOARD
    //     EQUIVALENT AND THIS SLICE DOES NOT INVENT ONE — it is booked with
    //     [DC-4]'s a11y follow-on, C-CALV4-DAYPICK-A11Y, because a focusable
    //     day cell that does not depend on a docked Ledger is a WIDGET change.
    //
    //     AND "POINTER-ONLY" IS DESKTOP-ONLY IN PRACTICE, WHICH IS DECLARED
    //     HERE RATHER THAN LEFT TO BE DISCOVERED AT A TABLE
    //     (C-CALV4-MOBILE [MOB-9b] SIGNED).
    //
    //     TOUCH IS A POINTER AND THIS PATH NAMED IT NEITHER WAY, which is the
    //     defect. THE MULTI-DAY PATH ON A PHONE IS THE EDITOR'S END-DATE
    //     FIELD, which already ships and is reachable by every input method.
    //
    //     ** THIS IS [READ] FROM CODE, NOT MEASURED, AND MUST STAY LABELLED
    //     THAT WAY WHEREVER IT IS RESTATED.** Headless Chromium fires no touch
    //     gestures, so nothing in this repository has observed it. The
    //     reasoning: the registration below takes `pointerdown` /
    //     `pointermove` / `pointerup` on `document` with NO `pointerType`
    //     filter; no `touch-action` is declared in any of the four calendar
    //     stylesheets; under the default `touch-action: auto` a browser that
    //     decides a touch is a pan fires `pointercancel` and stops sending
    //     `pointermove`; and this module's own `pointercancel` handler calls
    //     `dragEnd(true)`. So on a phone the drag is usually swallowed as a
    //     page scroll and creates nothing, while on a desktop it works.
    //     Sound, and unverified — ten seconds on a real phone settles it.
    //
    //     `touch-action: none` ON THE DAY CELLS IS REFUSED BY NAME. It would
    //     claim the gesture and KILL PAGE SCROLLING OVER THE MONTH GRID, which
    //     on a phone is a worse trade than losing drag-create — and it would
    //     put a `touch-action` declaration into the four sheets [MOB-4]
    //     deliberately keeps free of one. The touch path is booked as
    //     C-CALV4-DRAGCREATE-TOUCH; this ruling declares the scope, it does
    //     not build the path.
    //  7. TEXT-SELECTION SUPPRESSION IS SCOPED TO THE ACTIVE DRAG and restored
    //     on pointerup — the V2 implementation learned that one the hard way.
    //
    // ── THE PREVIEW IS DRAWN OUTSIDE THE BLOCK OR NOT AT ALL ([ER-8]) ────
    //
    // The obvious implementation adds a class to the cells under the pointer.
    // Those cells are inside `.cal-block-host`, and §1 rule 1 forbids adding or
    // removing a class on ANY element inside it — a rule
    // daycard_block_immutability.test.mjs enforces byte for byte. Term 2's "its
    // own drag-highlight rules" reads like a licence to mark cells and is not
    // one, and "just a class, just during a drag" is exactly how this bound
    // would be lost.
    //
    // So the preview is a PAGE-LEVEL OVERLAY: absolutely-positioned boxes, a
    // sibling of the card, created by this module, positioned from the run's
    // own getBoundingClientRect(). ONE BOX PER CONTIGUOUS ROW, not one union
    // box over the whole span — a union across two weeks would paint days that
    // are not in the run, which is a preview lying about what it is about to
    // create.
    //
    // IT DECLARES NO MOTION AND IT IS NOT MOTION. A highlight that appears and
    // follows the pointer is a POSITION UPDATE, not a transition: it touches
    // neither the register nor the carve-out, and
    // TestDayCardCSS_CarriesNoMotionOfItsOwn stays green with the rules in the
    // card's own sheet where they belong.


    function dragLayer() {
      if (drag.layer && drag.layer.parentNode) return drag.layer;
      var root = card.parentNode;
      if (!root) return null;
      var el = document.createElement('div');
      el.className = 'cal-daycard-drag';
      el.setAttribute('data-dc-drag', '');
      el.setAttribute('aria-hidden', 'true');
      root.appendChild(el);
      drag.layer = el;
      return el;
    }

    function dragClear() {
      if (!drag.layer) return;
      while (drag.layer.firstChild) drag.layer.removeChild(drag.layer.firstChild);
      drag.layer.hidden = true;
    }

    // dragPaint draws the run as one box per contiguous ROW, read off the
    // cells' own rects. It never touches a cell: it reads geometry and draws
    // beside it.
    function dragPaint(run) {
      var layer = dragLayer();
      if (!layer) return;
      dragClear();
      if (run.length < 2) return;
      var rows = [];
      run.forEach(function (d) {
        var cell = drag.host.querySelector('[data-day="' + d.key.replace(/"/g, '') + '"][data-day-ord]');
        if (!cell || !cell.getBoundingClientRect) return;
        var r = cell.getBoundingClientRect();
        if (!(r.width > 0)) return;
        var row = rows.length ? rows[rows.length - 1] : null;
        if (row && Math.abs(row.top - r.top) < 1) {
          row.left = Math.min(row.left, r.left);
          row.right = Math.max(row.right, r.right);
          row.bottom = Math.max(row.bottom, r.bottom);
          return;
        }
        rows.push({ left: r.left, top: r.top, right: r.right, bottom: r.bottom });
      });
      rows.forEach(function (b) {
        var box = document.createElement('i');
        box.className = 'dragbox';
        box.style.left = b.left + 'px';
        box.style.top = b.top + 'px';
        box.style.width = (b.right - b.left) + 'px';
        box.style.height = (b.bottom - b.top) + 'px';
        layer.appendChild(box);
      });
      layer.hidden = rows.length === 0;
    }

    function dragEnd(restore) {
      drag.on = false;
      dragClear();
      // TERM 7: the suppression is scoped to the ACTIVE drag and restored here,
      // on every exit path — including the one where the run turned out to be a
      // single cell and nothing was created.
      if (restore && document.body && document.body.style) {
        document.body.style.removeProperty('user-select');
      }
    }

    if (canEdit) {
      document.addEventListener('pointerdown', function (e) {
        // POINTER ONLY, and PRIMARY pointer only: a right-click or a secondary
        // button is not a drag, and treating it as one would swallow the
        // context menu.
        if (e.button !== undefined && e.button !== 0) return;
        var hit = cellFrom(e.target);
        if (!hit) return;
        var cal = index[hit.host.getAttribute('data-calendar-id') || ''];
        var key = hit.cell.getAttribute('data-day') || '';
        if (!cal || !cal.days[key]) return;
        drag.on = true;
        drag.moved = false;
        drag.host = hit.host;
        drag.cal = cal;
        drag.startKey = key;
        drag.lastKey = key;
      });

      document.addEventListener('pointermove', function (e) {
        if (!drag.on) return;
        var hit = cellFrom(e.target);
        if (!hit || hit.host !== drag.host) return;
        var key = hit.cell.getAttribute('data-day') || '';
        if (!key || key === drag.lastKey) return;
        drag.lastKey = key;
        // A DRAG OF ZERO CELLS IS A CLICK (term 5). `moved` only becomes true
        // when the pointer reaches a DIFFERENT day, so a jittery press on one
        // cell is still a click and still opens the card.
        if (key !== drag.startKey) {
          drag.moved = true;
          if (document.body && document.body.style) {
            document.body.style.setProperty('user-select', 'none');
          }
        }
        dragPaint(dayRange(orderedDates(drag.cal.list), drag.startKey, drag.lastKey));
      });

      document.addEventListener('pointerup', function () {
        if (!drag.on) return;
        var moved = drag.moved;
        var run = moved
          ? dayRange(orderedDates(drag.cal.list), drag.startKey, drag.lastKey) : [];
        var host = drag.host, cal = drag.cal;
        var boxes = [];
        if (drag.layer) {
          for (var i = 0; i < drag.layer.children.length; i++) {
            boxes.push(drag.layer.children[i].getBoundingClientRect());
          }
        }
        dragEnd(true);
        if (!moved || run.length < 2) return;
        // THE CLICK THAT FOLLOWS A REAL DRAG IS SUPPRESSED, so a span-create
        // does not also open the card on the last cell.
        //
        // IT IS ITS OWN FLAG AND NOT `state.suppress`. That one guards the
        // RADIO's change while the Ledger door activates it, inside a
        // try/finally around one synchronous .click(); borrowing it here would
        // have the door's own finally clear a flag this path had just set, and
        // the two would clobber each other in a way no test would name.
        drag.eatClick = true;
        state.host = host;
        var start = run[0], end = run[run.length - 1];
        // TERM 4: the span rides end_year / end_month / end_day, which the
        // shipped POST already binds and the chrome already writes. ZERO NEW API.
        var rec = {
          year: start.year, month: start.month, day: start.day,
          end_year: end.year, end_month: end.month, end_day: end.day,
          all_day: true,
        };
        // THE EDITOR MORPHS OUT OF THE SPAN THE USER JUST DREW. The card is not
        // open on this path, so the drawn overlay's own rect is the honest
        // origin — one box becomes the other, and the box it becomes is the one
        // the pointer described.
        var from = boxes.length ? {
          left: Math.min.apply(null, boxes.map(function (b) { return b.left; })),
          top: Math.min.apply(null, boxes.map(function (b) { return b.top; })),
          right: Math.max.apply(null, boxes.map(function (b) { return b.right; })),
          bottom: Math.max.apply(null, boxes.map(function (b) { return b.bottom; })),
        } : null;
        if (from) { from.width = from.right - from.left; from.height = from.bottom - from.top; }
        edOpen('create', cal, start, rec,
          from ? { getBoundingClientRect: function () { return from; } } : card);
      });

      // A CANCELLED POINTER (the OS took it, the window blurred) must not leave
      // the page unselectable. Same restoration, same exit path.
      document.addEventListener('pointercancel', function () {
        if (drag.on) dragEnd(true);
      });

    }

    window.addEventListener('resize', function () {
      if (!state.open || !state.host) return;
      var cell = state.host.querySelector('[data-day="' + state.key.replace(/"/g, '') + '"][data-day-ord]');
      if (cell) position(cell);
    });
  }

  // ── §2: the Bench's date-verb row ──────────────────────────────────────────
  //
  // C-CALV4-GAMEREADY [GR-SIGN-A] SIGNED / [GR-4]. Two verbs — `+1 day` and
  // `−1 day` — plus an Owner-only Set date, all three writing through the
  // EXISTING `PUT /campaigns/:id/calendar/world-state` with the payload
  // gm_panel.js already sends. No new route, no new capability, no floor moved.
  //
  // WHY IT LIVES IN THIS FILE, WHICH IS NOT ABOUT DAY CARDS. The Bounds of that
  // slice forbid a new page script (tools/check-page-scripts.sh is a shrink-only
  // ratchet) and forbid opening internal/app/routes.go, where the plugin's
  // body-script REGISTRY is built — and a <script> inside an HTMX-swapped
  // fragment is deleted outright by boot.js:163. This module is the plugin's
  // only registry-mounted driver that is already on the Bench, so it is the only
  // legal seat for the one thing the row cannot do declaratively: send JSON.
  // Echo's form binder skips `putWorldStateBody`'s tagless pointer-to-struct
  // members, so a form-encoded PUT binds nothing and silently changes no date
  // (TestWorldStatePut_FormEncodedBindsNothing pins that measurement).
  //
  // IT IS COMPLETELY INDEPENDENT OF THE CARD. Its own mount, its own wired
  // flag, its own early return. A page with a verb row and no day card wires it;
  // a page with a card and no verb row does not.
  //
  // THE ROLLOVER IS THE SERVER'S. This sends `{advance:{days:±1}}` and does no
  // date arithmetic of its own — month ends, year ends and leap geometry are
  // already decided once, server-side, for V2's console. A second definition
  // here is how two surfaces start disagreeing about when a year turns over.
  function initDateVerbs() {
    if (typeof document === 'undefined') return;
    var row = document.querySelector('[data-bench-date-verbs]');
    if (!row || row.dataset.dvWired === '1') return;
    row.dataset.dvWired = '1';

    var url = row.getAttribute('data-verb-url') || '';
    var csrf = row.getAttribute('data-verb-csrf') || '';
    var say = row.querySelector('[data-bench-date-say]');

    function report(msg) { if (say) say.textContent = msg; }

    // AN EMPTIED BOX IS NOT A ZERO. This row shipped reading the Year field
    // through a fieldInt helper whose fallback was zero, so a GM who cleared
    // the field and pressed Set date moved the world to year 0 — 200, stored,
    // no error. The V2 console had the identical fallback; a parity sweep found
    // both. The fallback cannot be made "smarter" by picking a better number,
    // because year 0 and negative years are LEGITIMATE on a fantasy calendar:
    // the only correct behaviour is to refuse a coordinate that is not there
    // and say which one. A typed "0" still parses and is still sent.
    function coordOrNull(sel) {
      var el = row.querySelector(sel);
      if (!el) return null;
      var raw = String(el.value == null ? '' : el.value).trim();
      if (raw === '' || !/^-?\d+$/.test(raw)) return null;
      return parseInt(raw, 10);
    }

    function commit(body, btn) {
      var fetcher = (window.Chronicle && window.Chronicle.apiFetch) ? window.Chronicle.apiFetch : null;
      if (!fetcher || !url) { report('Cannot reach the server.'); return; }
      if (btn) btn.disabled = true;
      report('Working…');
      fetcher(url, { method: 'PUT', body: body, headers: { 'X-CSRF-Token': csrf } })
        .then(function (resp) {
          if (!resp.ok) {
            return resp.json().catch(function () { return {}; }).then(function (b) {
              throw new Error((b && b.message) || 'The date was not changed.');
            });
          }
          // THE PAGE RELOADS RATHER THAN PATCHING THE NAMEPLATE. The date the
          // verbs move is printed by the Block's own projection, and this
          // module may READ the Block's DOM but never MUTATE it (see this
          // file's boundary header). A reload is the honest re-read; a
          // hand-patched label would be a second copy of the date that can
          // disagree with the grid beneath it.
          window.location.reload();
        })
        .catch(function (e) {
          report((e && e.message) || 'The date was not changed.');
          if (btn) btn.disabled = false;
        });
    }

    // DELEGATED AT THE DOCUMENT, exactly as this module's other click handler
    // is: the row's own controls are re-rendered by a boosted swap, and a
    // listener bound to the row instance would go dead the first time the page
    // came back through the sidebar — which is the class of bug the body-script
    // registry exists to have already fixed once.
    document.addEventListener('click', function (ev) {
      if (!ev.target || !ev.target.closest) return;
      if (!ev.target.closest('[data-bench-date-verbs]')) return;
      var step = ev.target.closest('[data-bench-date-step]');
      if (step) {
        var days = parseInt(step.getAttribute('data-bench-date-step'), 10);
        if (!days) return;
        commit({ advance: { days: days, hours: 0, minutes: 0 } }, step);
        return;
      }
      var set = ev.target.closest('[data-bench-date-set]');
      if (set) {
        var want = [
          { key: 'year', sel: '[data-bench-date-year]', label: 'Year' },
          { key: 'month', sel: '[data-bench-date-month]', label: 'Month' },
          { key: 'day', sel: '[data-bench-date-day]', label: 'Day' },
        ];
        var time = {};
        for (var i = 0; i < want.length; i++) {
          var v = coordOrNull(want[i].sel);
          if (v === null) {
            // The row's own aria-live line is the honest place for this: the
            // GM sees which box is empty, and NOTHING is sent.
            report(want[i].label + ' must be a whole number — the date was not changed.');
            return;
          }
          time[want[i].key] = v;
        }
        commit({ time: time }, set);
      }
    });
  }

  function initAll() { init(); initDateVerbs(); }

  if (typeof document !== 'undefined') {
    if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', initAll);
    else initAll();
    // Re-init after boosted navigation (the QA2 convention) — guarded per card.
    document.addEventListener('htmx:afterSettle', initAll);
    document.addEventListener('htmx:load', initAll);
  }
})();
