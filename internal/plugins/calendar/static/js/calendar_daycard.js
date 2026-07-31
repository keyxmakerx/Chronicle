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
        // Scribe+ only, and absent entirely below that floor — the producer
        // simply does not emit it, so there is nothing here to gate.
        categories: Array.isArray(cal.categories) ? cal.categories : [],
        days: days,
      };
    });
    return out;
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
  // itself. When the docked Ledger's rect is known the card is kept entirely to
  // its left.
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
  // the caller is the only place that can tell them apart. At desktop widths an
  // occluded Ledger is [DC-3]'s STOP-AND-FLAG. Below the breakpoint the
  // full-width bottom sheet is [DC-3]'s OWN next bullet ("Mobile = bottom
  // sheet, full-width") and §12 scopes the STOP-AND-FLAG row to 1232px, so the
  // overlap there is the signed treatment rather than a defect. Both are
  // recorded on the DOM; only the unsigned one speaks.
  //
  // MOBILE IS A BOTTOM SHEET, still [popover], still the register's motion.
  function placeCard(anchor, size, view, ledger, opts) {
    opts = opts || {};
    var pad = opts.pad === undefined ? 8 : opts.pad;
    var breakpoint = opts.breakpoint === undefined ? 640 : opts.breakpoint;

    if (view.w <= breakpoint) {
      var sTop = Math.max(0, view.h - size.h);
      return {
        left: 0, top: sTop, width: view.w, sheet: true,
        clear: !hitsLedger({ left: 0, top: sTop, width: view.w, height: size.h }, ledger),
      };
    }

    var top = anchor.bottom + pad;
    if (top + size.h > view.h - pad) {
      var above = anchor.top - pad - size.h;
      top = above >= pad ? above : Math.max(pad, view.h - pad - size.h);
    }

    var left = anchor.left;
    if (left + size.w > view.w - pad) left = view.w - pad - size.w;

    // The dodge: keep the box entirely left of the column it links to, when
    // there is room. When there is not, the box stays where the viewport put it
    // and the measurement below tells the truth about it.
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
  //   ONE console warning per session, desktop only — the DEVELOPER-facing half.
  //   Scoped to `!sheet` because below the breakpoint the full-width bottom
  //   sheet covering a stacked Ledger is [DC-3]'s own next bullet and §12 scopes
  //   the STOP-AND-FLAG row to 1232px (DC-MOBILE-4). Warning there would train
  //   the next hand to ignore the warning that matters. One per session, not one
  //   per placement: the card repositions on every open, and a warning that
  //   fires sixty times is a warning nobody reads.
  function occlusionReporter(sink) {
    var fired = false;
    return function (at) {
      if (!at || at.clear || at.sheet || fired) return false;
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
  function applyPlacement(el, at, report) {
    if (!el || !at) return at;
    el.style.left = at.left + 'px';
    el.style.top = at.top + 'px';
    el.style.width = at.sheet ? at.width + 'px' : '';
    el.classList.toggle('dcsheet', !!at.sheet);
    el.setAttribute('data-dc-clear', at.clear ? '1' : '0');
    if (report) report(at);
    return at;
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

    var V = (typeof window !== 'undefined' && window.ChronicleCalVisibility) || null;
    var storedMode = prev && V ? V.modeFor(prev.visibility, prev.visibility_rules) : 'public';
    if (opts.canOfferGMOnly && storedMode !== 'specific' && V) {
      var mapped = V.buildVisibilityPayload(form.gmOnly ? 'gmonly' : 'public', []);
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

    // The editor does not author recurrence in this stage, so it preserves it
    // rather than clearing it: is_recurring defaults false in the request
    // struct, and sending false over a recurring event would un-repeat it.
    if (prev && prev.entity_id) body.entity_id = prev.entity_id;
    if (prev && prev.description_html !== undefined) body.description_html = prev.description_html;
    return body;
  }

  if (typeof window !== 'undefined') {
    window.__calDayCard = {
      indexPayload: indexPayload,
      headText: headText,
      durationMS: durationMS,
      closeDelayMS: closeDelayMS,
      placeCard: placeCard,
      occlusionReporter: occlusionReporter,
      applyPlacement: applyPlacement,
      ordIsSafe: ordIsSafe,
      numOrNull: numOrNull,
      buildEventBody: buildEventBody,
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
    } : null;
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
    var edState = { open: false, timer: 0, mode: '', calId: '', eventID: '', day: null, prev: null, busy: false };

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
    // IT NEVER TOUCHES THE `+ New event` BUTTON, WHICH IS THE PRODUCER'S. Only
    // the Ledger door is managed here, and it is inserted BEFORE whatever the
    // server rendered rather than replacing the foot — a module that cleared
    // this container would be deciding a role gate by omission.
    function renderFoot(cal) {
      var existing = foot.querySelector('[data-dc-ledger]');
      if (existing) foot.removeChild(existing);
      if (!cal || !cal.ledgerDocked) return;
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
      if (typeof card.showPopover === 'function') {
        try { card.showPopover(); } catch (e) { /* already open */ }
      }
    }

    function hide() {
      state.timer = 0;
      card.removeAttribute('data-dc-shown');
      card.removeAttribute('style');
      if (typeof card.hidePopover === 'function') {
        try { card.hidePopover(); } catch (e) { /* already closed */ }
      }
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
      renderFoot(cal);

      state.open = true;
      state.key = key;
      state.calId = calId;
      state.host = host;
      state.ord = day.ord;

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

    function edShow() {
      if (edState.timer) { clearTimeout(edState.timer); edState.timer = 0; }
      ed.root.setAttribute('data-dc-shown', '');
      if (typeof ed.root.showPopover === 'function') {
        try { ed.root.showPopover(); } catch (e) { /* already open */ }
      }
    }

    function edHide() {
      edState.timer = 0;
      ed.root.removeAttribute('data-dc-shown');
      ed.root.removeAttribute('style');
      if (typeof ed.root.hidePopover === 'function') {
        try { ed.root.hidePopover(); } catch (e) { /* already closed */ }
      }
    }

    function edClose() {
      if (!edState.open) return;
      edState.open = false;
      ed.root.classList.remove('dcopen');
      var wait = closeDelayMS(cssVar('--disc-close'), reduced());
      if (wait <= 0) { edHide(); return; }
      if (edState.timer) clearTimeout(edState.timer);
      edState.timer = setTimeout(edHide, wait);
    }

    function edError(msg) {
      if (!ed.err) return;
      ed.err.textContent = msg || '';
      ed.err.hidden = !msg;
    }

    // seedCategories fills the type picker from THE PAGE PAYLOAD, per calendar.
    // The categories route is Owner-only, so a Scribe could not fetch this —
    // [DC-8](c) resolved to answering it from the producer rather than widening
    // that GET's floor.
    function seedCategories(cal) {
      if (!ed.category) return;
      while (ed.category.children.length > 1) {
        ed.category.removeChild(ed.category.children[ed.category.children.length - 1]);
      }
      (cal && cal.categories ? cal.categories : []).forEach(function (cat) {
        var opt = document.createElement('option');
        opt.setAttribute('value', cat.slug);
        opt.textContent = cat.glyph ? cat.glyph + ' ' + cat.name : cat.name;
        ed.category.appendChild(opt);
      });
    }

    function setValue(node, v) {
      if (!node) return;
      node.value = (v === null || v === undefined) ? '' : String(v);
    }

    function edFill(rec) {
      setValue(ed.name, rec.name || '');
      setValue(ed.desc, rec.description || '');
      setValue(ed.category, rec.category || '');
      setValue(ed.year, rec.year);
      setValue(ed.month, rec.month);
      setValue(ed.day, rec.day);
      setValue(ed.startH, rec.start_hour);
      setValue(ed.startM, rec.start_minute);
      setValue(ed.endH, rec.end_hour);
      setValue(ed.endM, rec.end_minute);
      setValue(ed.endYear, rec.end_year);
      setValue(ed.endMonth, rec.end_month);
      setValue(ed.endDay, rec.end_day);
      var allDay = rec.all_day || rec.start_hour === null || rec.start_hour === undefined;
      if (ed.allDay) ed.allDay.checked = !!allDay;
      if (ed.timeRow) ed.timeRow.hidden = !!allDay;
      if (ed.gmOnly) ed.gmOnly.checked = rec.visibility === 'dm_only';
      if (ed.del) ed.del.hidden = !rec.id;
      edError('');
    }

    function edOpen(mode, cal, day, rec, anchor) {
      // A fresh editor session is a fresh write. The flag survives a failed
      // save so the failing one cannot be double-submitted; it must not survive
      // the box being closed and opened again, or a network blip would leave the
      // editor permanently unable to save.
      edState.busy = false;
      edState.mode = mode;
      edState.calId = cal.id;
      edState.day = day;
      edState.eventID = rec && rec.id ? rec.id : '';
      edState.prev = mode === 'edit' ? rec : null;
      seedCategories(cal);
      edFill(rec);
      if (ed.head) {
        ed.head.textContent = (mode === 'edit' ? 'Edit event · ' : 'New event · ') + (day.label || '');
        ed.head.setAttribute('data-day', day.key || '');
      }
      // The card leaves first, so the two boxes are never on screen together.
      closeCard();
      edState.open = true;
      edShow();
      if (anchor) edPosition(anchor);
      void ed.root.offsetHeight;
      ed.root.classList.add('dcopen');
    }

    function edPosition(anchor) {
      if (!anchor.getBoundingClientRect) return;
      var view = { w: window.innerWidth || 0, h: window.innerHeight || 0 };
      var chrome = (ed.root.offsetHeight || 0) - (ed.box ? ed.box.offsetHeight || 0 : 0);
      var size = {
        w: ed.root.offsetWidth || 0,
        h: (ed.box ? ed.box.scrollHeight || 0 : 0) + (chrome > 0 ? chrome : 0),
      };
      applyPlacement(ed.root,
        placeCard(anchor.getBoundingClientRect(), size, view, ledgerRect(state.host), {}),
        reportOcclusion);
    }

    function api() {
      return (window.Chronicle && window.Chronicle.apiFetch) ? window.Chronicle.apiFetch : null;
    }

    function eventsBase() {
      return '/campaigns/' + campaignID + '/calendars/' + edState.calId + '/events';
    }

    function edFormValues() {
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
        gmOnly: ed.gmOnly ? !!ed.gmOnly.checked : false,
      };
    }

    // edFailed re-opens the editor for another attempt. The busy flag is
    // cleared HERE and only here: every success path ends in a reload, and
    // clearing on success would leave a live Save button on a page that is
    // already navigating.
    function edFailed(msg) {
      edState.busy = false;
      edError(msg);
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
      var body = buildEventBody(form, edState.prev, { canOfferGMOnly: !!ed.gmOnly });
      var edit = edState.mode === 'edit' && edState.eventID;
      // Set AFTER validation, so a rejected title does not lock the editor.
      edState.busy = true;
      fetcher(edit ? eventsBase() + '/' + edState.eventID : eventsBase(), {
        method: edit ? 'PUT' : 'POST', body: body,
      }).then(function (resp) {
        if (resp && resp.ok) { window.location.reload(); return; }
        edFailed('That did not save. Check the date and try again.');
      }).catch(function () {
        edFailed('That did not save. Check your connection and try again.');
      });
    }

    function edDelete() {
      if (edState.busy) return;
      var fetcher = api();
      if (!fetcher || !edState.eventID) return;
      edState.busy = true;
      fetcher(eventsBase() + '/' + edState.eventID, { method: 'DELETE' })
        .then(function (resp) {
          if (resp && resp.ok) { window.location.reload(); return; }
          edFailed('That event could not be deleted.');
        })
        .catch(function () { edFailed('That event could not be deleted.'); });
    }

    // edLoad fetches the full record for EDIT mode. It is the one read this
    // slice added, and it is the only thing the editor genuinely cannot get
    // from the page: the card's payload deliberately carries no description, no
    // visibility_rules and no recurrence.
    function edLoad(cal, day, eventID, anchor) {
      var fetcher = api();
      if (!fetcher) return;
      fetcher('/campaigns/' + campaignID + '/calendars/' + cal.id + '/events/' + eventID, { method: 'GET' })
        .then(function (resp) {
          if (!resp || !resp.ok) return;
          return resp.json();
        })
        .then(function (rec) {
          if (!rec) return;
          edOpen('edit', cal, day, rec, anchor);
        })
        .catch(function () { /* a refused read is a closed card, not a broken page */ });
    }

    // --- wiring ------------------------------------------------------------
    //
    // DELEGATED, so an HTMX re-settle of the row grid cannot orphan a listener,
    // and guarded once on the card itself (dataset.dcWired) so a re-init cannot
    // double-bind — the QA2 class of bug event_grid.js carries per-node guards
    // for at :250, :403 and :520.

    function cellFrom(target) {
      if (!target || !target.closest) return null;
      if (target.closest('[data-cal-daycard]')) return null;
      var cell = target.closest('[data-day][data-day-ord]');
      if (!cell) return null;
      var host = cell.closest('[data-bench-block]');
      return host ? { cell: cell, host: host } : null;
    }

    document.addEventListener('click', function (e) {
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
        if (e.target.closest('[data-de-cancel]')) { e.preventDefault(); edClose(); return; }
        if (e.target.closest('[data-de-delete]')) { e.preventDefault(); edDelete(); return; }
        if (e.target.closest('[data-de-save]')) { e.preventDefault(); edSave(); return; }
        if (e.target.closest('[data-cal-dayeditor]')) return;
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

    // The all-day toggle reveals the in-world time row. It is a display switch
    // on a field the API has ALWAYS had — the mockup's `needs backend` chip
    // over start_hour/start_minute was simply wrong and it comes off rather
    // than being ported.
    if (ed && ed.allDay) {
      ed.allDay.addEventListener('change', function () {
        if (ed.timeRow) ed.timeRow.hidden = !!ed.allDay.checked;
      });
    }
    if (ed && ed.form) {
      ed.form.addEventListener('submit', function (e) { e.preventDefault(); edSave(); });
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

    window.addEventListener('resize', function () {
      if (!state.open || !state.host) return;
      var cell = state.host.querySelector('[data-day="' + state.key.replace(/"/g, '') + '"][data-day-ord]');
      if (cell) position(cell);
    });
  }

  if (typeof document !== 'undefined') {
    if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', init);
    else init();
    // Re-init after boosted navigation (the QA2 convention) — guarded per card.
    document.addEventListener('htmx:afterSettle', init);
    document.addEventListener('htmx:load', init);
  }
})();
