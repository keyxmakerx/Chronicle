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
  // its left; `clear` reports whether that succeeded, so a geometry in which it
  // cannot is visible rather than silent.
  //
  // MOBILE IS A BOTTOM SHEET, still [popover], still the register's motion.
  function placeCard(anchor, size, view, ledger, opts) {
    opts = opts || {};
    var pad = opts.pad === undefined ? 8 : opts.pad;
    var breakpoint = opts.breakpoint === undefined ? 640 : opts.breakpoint;

    if (view.w <= breakpoint) {
      return {
        left: 0, top: Math.max(0, view.h - size.h),
        width: view.w, sheet: true, clear: true,
      };
    }

    var top = anchor.bottom + pad;
    if (top + size.h > view.h - pad) {
      var above = anchor.top - pad - size.h;
      top = above >= pad ? above : Math.max(pad, view.h - pad - size.h);
    }

    var left = anchor.left;
    if (left + size.w > view.w - pad) left = view.w - pad - size.w;

    var clear = true;
    if (ledger && ledger.width > 0 && overlapsY(top, size.h, ledger)) {
      var limit = ledger.left - pad - size.w;
      if (limit >= pad) left = Math.min(left, limit);
      else clear = false;
    }
    if (left < pad) left = pad;

    return { left: left, top: top, width: 0, sheet: false, clear: clear };
  }

  function overlapsY(top, height, rect) {
    return top < rect.top + rect.height && top + height > rect.top;
  }

  // ordIsSafe gates the one attribute-selector interpolation in this module.
  // The ordinal comes from our own payload and is "12" or "i1", never anything
  // else — but a selector built from data is exactly where that stops being
  // true one slice later.
  function ordIsSafe(ord) {
    return typeof ord === 'string' && /^i?[0-9]+$/.test(ord);
  }

  if (typeof window !== 'undefined') {
    window.__calDayCard = {
      indexPayload: indexPayload,
      headText: headText,
      durationMS: durationMS,
      closeDelayMS: closeDelayMS,
      placeCard: placeCard,
      ordIsSafe: ordIsSafe,
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
      return row;
    }

    // renderFoot emits the doors. `Open in the Ledger` EXISTS ONLY WHEN THE
    // LEDGER IS ACTUALLY DOCKED for this viewer, and that fact is carried on
    // the payload rather than inferred from the DOM's absence — absence has two
    // causes (a host that never docked the zone, and a viewer who switched the
    // layer off) and a link to a column that is not on the page is a lie.
    function renderFoot(cal) {
      while (foot.firstChild) foot.removeChild(foot.firstChild);
      if (!cal || !cal.ledgerDocked) return;
      var btn = document.createElement('button');
      btn.type = 'button';
      btn.className = 'dc-door';
      btn.setAttribute('data-dc-ledger', '');
      btn.textContent = 'Open in the Ledger';
      foot.appendChild(btn);
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
      var at = placeCard(anchor, size, view, ledger, {});
      card.style.left = at.left + 'px';
      card.style.top = at.top + 'px';
      if (at.sheet) card.style.width = at.width + 'px';
      card.classList.toggle('dcsheet', !!at.sheet);
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
      var hit = cellFrom(e.target);
      if (hit) { openCard(hit.host, hit.cell); return; }
      // Outside the card and outside a day: dismiss. The module owns dismissal
      // because popover=manual, and it owns it so the register's close runs on
      // every path rather than only on the button.
      if (state.open && e.target && e.target.closest && !e.target.closest('[data-cal-daycard]')) {
        closeCard();
      }
    });

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
      if (e.key === 'Escape' && state.open) closeCard();
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
