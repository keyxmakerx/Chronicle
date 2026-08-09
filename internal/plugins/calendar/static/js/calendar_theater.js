// calendar_theater.js — C-CALV4-THEATER (slice R2-3). The theater's door.
//
// ── WHERE THIS MOUNTS FROM, AND WHY IT IS NOT A <script src> ([TH-12]) ──────
//
// This module rides the plugin BODY-SCRIPT REGISTRY in internal/app/routes.go,
// not a `<script src>` in entity_calendar_block.templ. That is a ruling, and it
// is a build-stopper if it is missed: tools/page-script-allowlist.txt pins that
// template at EXACTLY ONE page-side script — already spent on cal-almanac.js —
// and tools/check-page-scripts.sh is a shrink-only ratchet that fails on a
// count ABOVE its allowlisted number as well as below it. Adding a script tag
// reds CI, and bumping the entry to 2 defeats the ratchet's stated purpose.
//
// The registry is also the only home that WORKS. It is injected via
// LayoutInjector on every render, OUTSIDE the swapped region, so a boosted
// navigation and a full load deliver the same scripts. A page-side tag inside
// the swapped region wires on a direct load and silently does nothing when the
// page is reached through the sidebar — the exact defect that shipped the Bench
// with a dead day card.
//
// SO THIS FILE OBEYS THE REGISTRY'S CONVENTION, WHICH THE COMMENT ABOVE THAT
// LITERAL STATES IN THE CODE'S OWN WORDS: it is a DRIVER, not data; it re-inits
// on htmx:afterSettle / htmx:load; it RETURNS IMMEDIATELY when its mount is
// absent (which on this registry is every page but one); its wiring guard is
// idempotent; and IT CARRIES NO PERMISSION DECISION. The gate is, as it always
// was, the producer not rendering the DOM.
//
// ── THE MODULE READS AND LISTENS. IT DOES NOT MUTATE BLOCK DOM ([DC-3]) ─────
//
// The theater contains a second calendar-v4 Block. This module may query it and
// listen to it; it may NOT insert a node inside `.cal-block-host`, add or
// remove a class inside it, set an attribute on it, or reparent it. A Block
// that changes parents mid-session is a container query resolving against a box
// that is being animated. test/js/theater_block_immutability.test.mjs boots this
// module against a Block-shaped fixture and asserts the fixture's innerHTML is
// byte-identical before and after open + close.
//
// It also does not INTERCEPT the Block's own controls. The switchboard inside a
// theater still opens where one is rendered, the day radios still select, the
// Shelf tabs still switch. If a Block control misbehaves inside the theater
// that is a FINDING to report, not a line to patch — it would be a Block bug
// the embed simply never exposed.
//
// ── MOTION IS THE REGISTER'S, ON EVERY PATH ([TH-1], [TH-3], [TH-4]) ────────
//
// The reveal's subject is `.tbox`, not the dialog. An element that was
// `display: none` has no before-change style to transition FROM — which is what
// @starting-style exists for, and @starting-style is a standing refusal — so
// the reveal runs on a child that is rendered the whole time, and this module
// forces ONE LAYOUT FLUSH between showing the dialog and adding the open class.
// THAT FLUSH IS MECHANISM, NOT DECORATION: delete it as a stray `void` and the
// reveal silently stops running, with every guard green.
//
// `<dialog>`'s `cancel` IS CANCELABLE, which is the whole reason [TH-1] diverges
// from the day card's `popover="manual"`. Escape is preventDefault()ed, the
// register's close runs, and `close()` lands on transitionend — with a timeout
// fallback so a browser that never fires it cannot strand the dialog open.
//
// THE REDUCED-MOTION SHORT-CIRCUIT IS REQUIRED, NOT POLITE. Under
// prefers-reduced-motion the [TH-3] rules DO NOT EXIST — they live inside
// calendar-bench.css's single `no-preference` wrapper — so NO transitionend will
// ever fire and a close path that waits on one hangs the theater open. The
// timeout is the belt; the short-circuit is the braces.
(function () {
  'use strict';

  var OPEN_CLASS = 'tbopen';
  var LOCK_CLASS = 'cal-theater-lock';
  // The register's close is 160ms. The fallback has to outlive it without
  // becoming a second duration anybody could mistake for a tuning knob: it is a
  // WATCHDOG, and it fires only when transitionend does not.
  var CLOSE_FALLBACK_MS = 400;

  function reducedMotion() {
    if (typeof window === 'undefined' || !window.matchMedia) return false;
    try {
      return !!window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    } catch (e) {
      return false;
    }
  }

  function box(dialog) { return dialog.querySelector('[data-theater-box]'); }

  // openerFor finds the button that points at this scaffold. It is resolved by
  // ID rather than by proximity, which is [TH-15]'s other half: on a page with
  // two (calendar, entity) pairs, proximity would hand the first opener the
  // wrong theater and the bug would be invisible on a one-embed page.
  function openerFor(doc, dialog) {
    var id = dialog.getAttribute('id');
    if (!id) return null;
    return doc.querySelector('[data-theater-pick="' + id + '"]');
  }

  function lock(doc, on) {
    var root = doc.documentElement;
    if (!root || !root.classList) return;
    if (on) root.classList.add(LOCK_CLASS);
    else root.classList.remove(LOCK_CLASS);
  }

  // ── open ─────────────────────────────────────────────────────────────────

  function open(doc, dialog) {
    if (dialog.open) return;
    var opener = openerFor(doc, dialog);
    // The opener is remembered on the dialog rather than in a module variable,
    // because two theaters on one page would share a variable and the second
    // close would return focus to the first opener.
    dialog._theaterOpener = opener || null;

    if (typeof dialog.showModal === 'function') dialog.showModal();
    else dialog.setAttribute('open', '');

    lock(doc, true);
    if (opener) opener.setAttribute('aria-expanded', 'true');

    var b = box(dialog);
    if (b) {
      // THE ONE LAYOUT FLUSH. See the header: without it the class lands in the
      // same style change as the dialog becoming visible, there is no
      // before-change style to transition from, and the reveal does not run.
      void b.offsetHeight;
      b.classList.add(OPEN_CLASS);
    }

    // FOCUS ENTERS THE THEATER, and it enters at the CLOSE CONTROL rather than
    // at the first focusable descendant of the Block — that would be a day cell
    // deep inside a grid, which is disorienting with a screen reader and gives
    // a keyboard user no obvious way back out.
    var close = dialog.querySelector('[data-theater-close]');
    var target = close || dialog;
    if (target && typeof target.focus === 'function') target.focus();
  }

  // ── close ────────────────────────────────────────────────────────────────
  //
  // ONE PATH FOR EVERY WAY OUT — the close button, Escape, a backdrop click and
  // the HTMX swap all land here, so the register cannot survive on some paths
  // and be skipped on others, and focus returns to the opener on ALL of them. A
  // modal that returns focus to <body> drops a keyboard user at the top of the
  // page they were reading.
  //
  // `immediate` is the reduced-motion / mid-swap short circuit.
  function close(doc, dialog, immediate) {
    if (!dialog.open && !dialog.hasAttribute('open')) return;
    if (dialog._theaterClosing) return;
    dialog._theaterClosing = true;

    var b = box(dialog);
    var finish = function () {
      if (!dialog._theaterClosing) return;
      dialog._theaterClosing = false;
      if (dialog._theaterTimer) {
        clearTimeout(dialog._theaterTimer);
        dialog._theaterTimer = null;
      }
      if (typeof dialog.close === 'function') dialog.close();
      else dialog.removeAttribute('open');
      lock(doc, false);

      var opener = dialog._theaterOpener;
      dialog._theaterOpener = null;
      if (opener) {
        opener.setAttribute('aria-expanded', 'false');
        if (typeof opener.focus === 'function') opener.focus();
      }
    };

    if (b) b.classList.remove(OPEN_CLASS);

    if (immediate || reducedMotion() || !b) {
      finish();
      return;
    }
    dialog._theaterFinish = finish;
    dialog._theaterTimer = setTimeout(finish, CLOSE_FALLBACK_MS);
  }

  // ── wiring ───────────────────────────────────────────────────────────────

  function wire(doc, dialog) {
    // IDEMPOTENT. [TH-12]'s re-init contract and [TH-14]'s: re-binding an
    // already-wired scaffold is a no-op, never a second listener. The scaffold
    // sits inside the region picker.templ swaps with hx-swap="outerHTML", so it
    // is destroyed and re-created with its opener on every binding mutation and
    // this function runs again on the new node.
    if (dialog.dataset && dialog.dataset.theaterWired) return;
    if (dialog.dataset) dialog.dataset.theaterWired = '1';

    var opener = openerFor(doc, dialog);
    if (opener) {
      opener.addEventListener('click', function (ev) {
        if (ev && ev.preventDefault) ev.preventDefault();
        open(doc, dialog);
      });
    }

    var closeBtn = dialog.querySelector('[data-theater-close]');
    if (closeBtn) {
      closeBtn.addEventListener('click', function (ev) {
        if (ev && ev.preventDefault) ev.preventDefault();
        close(doc, dialog, false);
      });
    }

    // ESCAPE, THROUGH THE REGISTER. `cancel` is cancelable — that is the
    // property `popover="auto"` could not offer and the reason [TH-1] chose
    // <dialog>. preventDefault() keeps the UA from closing synchronously, the
    // register's close runs, and close() lands on transitionend.
    dialog.addEventListener('cancel', function (ev) {
      if (ev && ev.preventDefault) ev.preventDefault();
      close(doc, dialog, false);
    });

    // A BACKDROP CLICK CLOSES, same path, same register. The backdrop is not an
    // element that can be listened to, so the signal is a click whose target IS
    // the dialog: anything inside the theater is a descendant, so this cannot
    // swallow a click on the Block.
    dialog.addEventListener('click', function (ev) {
      if (ev && ev.target === dialog) close(doc, dialog, false);
    });

    // The register's close completing. Scoped to the animated box so a
    // transition anywhere inside the Block cannot end the close early.
    dialog.addEventListener('transitionend', function (ev) {
      var b = box(dialog);
      if (!b || !ev || ev.target !== b) return;
      if (dialog._theaterFinish) dialog._theaterFinish();
    });
  }

  // initAll binds EVERY scaffold it finds, never `querySelector` on the first
  // ([TH-15]). A querySelector mount is exactly the defect that block exists to
  // prevent, and it is invisible on a one-embed page.
  function initAll() {
    if (typeof document === 'undefined') return;
    var doc = document;
    var all = doc.querySelectorAll('[data-cal-theater]');
    // The registry mounts this on EVERY page; on all but one there is nothing
    // here and the module returns immediately.
    if (!all || !all.length) return;
    for (var i = 0; i < all.length; i++) wire(doc, all[i]);
  }

  // ── the swap guard ([TH-14], RE-SIGNED 2026-08-08) ────────────────────────
  //
  // The scaffold sits INSIDE an HTMX-swappable region, deliberately: the whole
  // entity calendar block is wrapped in widgetbindings.BlockHost and picker.templ
  // targets THAT wrapper with hx-swap="outerHTML" on three paths, so no position
  // this slice owns is outside one. The failure the original constraint feared —
  // an opener that outlives its scaffold — cannot occur, because opener and
  // scaffold share the swapped subtree and die and revive together, and initAll
  // re-runs on htmx:afterSettle.
  //
  // THE ONE REAL RISK IS A DIALOG THAT IS OPEN WHEN ITS SUBTREE IS SWAPPED: it
  // is removed from the DOM mid-modal, which strands the top layer, leaves the
  // document scroll-locked and drops focus on a detached node. Narrow — while
  // the theater is open the picker behind it is inert, so the swap has to be
  // driven from elsewhere — but real, and this listener is its REQUIRED
  // counterpart. It closes IMMEDIATELY rather than through the register: there
  // is no time for a 160ms reveal before the node is replaced, and a close that
  // waited would run its finish() against a detached element.
  function onBeforeSwap(ev) {
    if (typeof document === 'undefined') return;
    var target = ev && (ev.target || (ev.detail && ev.detail.target));
    if (!target || typeof target.querySelectorAll !== 'function') return;
    var all = target.querySelectorAll('[data-cal-theater]');
    var here = [];
    for (var i = 0; i < all.length; i++) here.push(all[i]);
    if (target.matches && target.matches('[data-cal-theater]')) here.push(target);
    for (var j = 0; j < here.length; j++) close(document, here[j], true);
  }

  if (typeof document !== 'undefined') {
    if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', initAll);
    else initAll();
    document.addEventListener('htmx:afterSettle', initAll);
    document.addEventListener('htmx:load', initAll);
    document.addEventListener('htmx:beforeSwap', onBeforeSwap);
  }

  // The test alias, matching the house convention (__calPerm, __calVis,
  // __calDayCard). Nothing in the product reads it.
  if (typeof window !== 'undefined') {
    window.__calTheater = {
      initAll: initAll,
      open: open,
      close: close,
      onBeforeSwap: onBeforeSwap,
      OPEN_CLASS: OPEN_CLASS,
      LOCK_CLASS: LOCK_CLASS,
      CLOSE_FALLBACK_MS: CLOSE_FALLBACK_MS,
    };
  }
})();
