/*
 * checkup.js — the site check-up's shield in the Site admin top bar.
 *
 * The shield, its sheet and the reminder strip are server-rendered into
 * #checkup-host (admin/checkup.templ) and re-fetched after each admin page.
 * This widget only does what the server can't: hover to look, click to keep
 * it open, Escape or a click elsewhere to close, and the few-second peek the
 * server asks for once per sign-in (data-checkup-peek).
 *
 * The sheet grows out of the shield (CSS); on a phone it is a bottom sheet.
 * Calm and Off motion fade it in place, and the peek stays open longer so it
 * can still be read without movement drawing the eye.
 */
(function () {
  'use strict';

  var CLOSE_DELAY = 250;
  var PEEK_FULL = 4000;
  var PEEK_CALM = 6000;

  function lessMotion() {
    var h = document.documentElement;
    return h.hasAttribute('data-motion') ||
      (window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);
  }

  Chronicle.register('checkup', {
    init: function (host) {
      var pinned = false;
      var hovering = false;
      var closeTimer = null;
      var peekTimer = null;

      function sheet() { return host.querySelector('[data-checkup-sheet]'); }
      function chip() { return host.querySelector('[data-checkup-chip]'); }

      function setOpen(on, note) {
        var s = sheet();
        var c = chip();
        if (!s || !c) return;
        s.classList.toggle('is-open', on);
        c.setAttribute('aria-expanded', on ? 'true' : 'false');
        var n = s.querySelector('[data-checkup-peeknote]');
        if (n) n.textContent = on && note ? note : '';
      }

      function close() {
        pinned = false;
        clearTimeout(peekTimer);
        setOpen(false);
      }

      function onEnter() {
        hovering = true;
        clearTimeout(closeTimer);
        setOpen(true);
      }
      function onLeave() {
        hovering = false;
        if (pinned) return;
        closeTimer = setTimeout(function () { setOpen(false); }, CLOSE_DELAY);
      }

      function onHostClick(e) {
        if (e.target.closest('[data-checkup-chip]')) {
          pinned = !pinned;
          clearTimeout(peekTimer);
          setOpen(pinned || hovering);
          return;
        }
        // A fix takes the admin somewhere else; the sheet steps aside.
        if (e.target.closest('[data-checkup-fix]')) close();
      }

      function onDocClick(e) {
        if (e.target.closest('[data-checkup-open]')) {
          pinned = true;
          setOpen(true);
          var c = chip();
          if (c) c.focus();
          return;
        }
        if (pinned && !host.contains(e.target) && !e.target.closest('#checkup-strip')) close();
      }

      function onKey(e) {
        if (e.key === 'Escape' && sheet() && sheet().classList.contains('is-open')) close();
      }

      // A refresh replaces the sheet; keep it open if the admin had it open,
      // and peek when the server says this sign-in hasn't seen it yet. This
      // waits for the settle: htmx gives the new sheet the old one's classes
      // until then, and would undo an earlier change.
      function afterSettle() {
        if (pinned || hovering) setOpen(true);
        var s = sheet();
        if (!s || !s.hasAttribute('data-checkup-peek') || pinned) return;
        setOpen(true, 'Peeking for a few seconds…');
        clearTimeout(peekTimer);
        peekTimer = setTimeout(function () {
          if (!pinned && !hovering) setOpen(false);
        }, lessMotion() ? PEEK_CALM : PEEK_FULL);
      }

      host.addEventListener('mouseenter', onEnter);
      host.addEventListener('mouseleave', onLeave);
      host.addEventListener('click', onHostClick);
      host.addEventListener('htmx:afterSettle', afterSettle);
      // Kept on the host so destroy() can take them off the document again.
      host._ckClick = onDocClick;
      host._ckKey = onKey;
      host._ckTimers = function () {
        clearTimeout(closeTimer);
        clearTimeout(peekTimer);
      };
      document.addEventListener('click', host._ckClick);
      document.addEventListener('keydown', host._ckKey);
      if (sheet()) afterSettle();
    },
    destroy: function (host) {
      if (host._ckTimers) host._ckTimers();
      document.removeEventListener('click', host._ckClick);
      document.removeEventListener('keydown', host._ckKey);
    },
  });
})();
