/**
 * sidebar_peek.js -- the hidden sidebar's hover peek (app.templ, input.css)
 *
 * When the sidebar is hidden, resting the pointer against the window's left
 * edge grows a soft accent light outward from the pointer like a wave. Once
 * it has spread (WAIT_MS), the page slides aside by PEEK_PX and the sidebar
 * shows underneath, so its icons can be clicked. Clicking empty space in the
 * peek opens the sidebar fully; moving away tucks it back.
 *
 * The edge is watched from document mousemove rather than an overlay, so
 * nothing under the edge is ever blocked. Pointer devices without hover and
 * narrow screens keep the restore arrow only. Under reduced motion the light
 * appears at full size and the page moves without sliding.
 */
(function () {
  'use strict';

  var EDGE_PX = 6;      // how close to the edge counts as resting on it
  var STILL_PX = 4;     // pointer drift allowed before the light restarts
  var WAIT_MS = 1000;   // rest time before the peek opens
  var PEEK_PX = 70;     // matches .sidebar-peek-open #app-main in input.css
  var TUCK_MS = 300;    // grace before tucking once the pointer leaves
  var SLIDE_MS = 380;   // matches #app-main's peek transition

  var canHover = window.matchMedia('(hover: hover) and (pointer: fine)');
  var wide = window.matchMedia('(min-width: 768px)');
  var reduce = window.matchMedia('(prefers-reduced-motion: reduce)');
  var root = document.documentElement;

  // idle -> arming -> open -> tucking -> idle
  var state = 'idle';
  var glow = null, armTimer = 0, tuckTimer = 0, settleTimer = 0, raf = 0;
  var armY = 0, armStart = 0, reach = 0;

  function sidebar() { return document.getElementById('sidebar'); }
  function isHidden() {
    var sb = sidebar();
    return !!sb && sb.classList.contains('sidebar-fully-hidden');
  }

  function ensureGlow() {
    if (!glow) {
      glow = document.createElement('div');
      glow.className = 'sidebar-peek-glow';
      glow.setAttribute('aria-hidden', 'true');
      document.body.appendChild(glow);
    }
    return glow;
  }

  // A soft body of light plus a brighter crest at its rim; as the radius
  // grows the crest travels up and down the edge like a wave. The colour is
  // the campaign's own glow colour when it set one (--peek-glow-rgb from
  // Customize), otherwise the site's customizable accent.
  function paint(r) {
    var a = 'var(--peek-glow-rgb, var(--color-accent-rgb, 99 102 241))';
    var at = 'at 0 ' + armY + 'px';
    glow.style.background =
      'radial-gradient(ellipse 24px ' + r + 'px ' + at + ', transparent 64%, rgb(' + a + ' / 0.30) 84%, transparent 97%),' +
      'radial-gradient(ellipse 36px ' + r + 'px ' + at + ', rgb(' + a + ' / 0.22), rgb(' + a + ' / 0.07) 60%, transparent 78%)';
  }

  function frame(now) {
    var t = Math.min(1, (now - armStart) / WAIT_MS);
    var eased = 1 - Math.pow(1 - t, 3);
    paint(8 + eased * (reach - 8));
    if (t < 1 && state === 'arming') raf = window.requestAnimationFrame(frame);
  }

  function arm(y) {
    clearArm();
    state = 'arming';
    armY = y;
    reach = Math.max(y, window.innerHeight - y) * 1.15;
    ensureGlow().classList.add('is-on');
    root.classList.add('sidebar-peek-arming');
    if (reduce.matches) {
      paint(reach);
    } else {
      armStart = performance.now();
      paint(8);
      raf = window.requestAnimationFrame(frame);
    }
    armTimer = window.setTimeout(open, WAIT_MS);
  }

  function clearArm() {
    window.clearTimeout(armTimer);
    window.cancelAnimationFrame(raf);
    root.classList.remove('sidebar-peek-arming');
    if (glow) glow.classList.remove('is-on');
  }

  function disarm() {
    clearArm();
    if (state === 'arming') state = 'idle';
  }

  function open() {
    clearArm();
    var sb = sidebar();
    if (!sb || !isHidden()) { state = 'idle'; return; }
    window.clearTimeout(settleTimer);
    state = 'open';
    sb.classList.add('sidebar-peeking');
    root.classList.add('sidebar-peek-active');
    // Two frames so the page's starting position is painted and its slide
    // actually runs.
    window.requestAnimationFrame(function () {
      window.requestAnimationFrame(function () {
        if (state === 'open') root.classList.add('sidebar-peek-open');
      });
    });
  }

  function tuck() {
    if (state !== 'open') return;
    state = 'tucking';
    root.classList.remove('sidebar-peek-open');
    settleTimer = window.setTimeout(settle, reduce.matches ? 0 : SLIDE_MS);
  }

  // Puts the sidebar back to hidden without letting its own width or slide
  // transitions play: it is still under the page and must not be seen moving.
  function settle() {
    window.clearTimeout(tuckTimer);
    window.clearTimeout(settleTimer);
    tuckTimer = 0;
    var sb = sidebar();
    root.classList.remove('sidebar-peek-open');
    if (sb) {
      sb.classList.add('sidebar-peek-settling');
      sb.classList.remove('sidebar-peeking');
      void sb.offsetWidth;
      sb.classList.remove('sidebar-peek-settling');
    }
    root.classList.remove('sidebar-peek-active');
    state = 'idle';
  }

  function fullOpen() {
    settle();
    window.dispatchEvent(new CustomEvent('chronicle:sidebar-show'));
  }

  function onMove(e) {
    if (!canHover.matches || !wide.matches) return;

    if (state === 'open') {
      if (!isHidden()) { settle(); return; }
      if (e.clientX > PEEK_PX + 4) {
        if (!tuckTimer) tuckTimer = window.setTimeout(function () { tuckTimer = 0; tuck(); }, TUCK_MS);
      } else if (tuckTimer) {
        window.clearTimeout(tuckTimer);
        tuckTimer = 0;
      }
      return;
    }
    if (state === 'tucking') return;

    if (!isHidden()) { if (state === 'arming') disarm(); return; }
    if (e.clientX <= EDGE_PX) {
      if (state !== 'arming' || Math.abs(e.clientY - armY) > STILL_PX) arm(e.clientY);
    } else if (state === 'arming') {
      disarm();
    }
  }

  // Leaving the window anywhere but the left edge drops the light; leaving
  // past the left edge keeps it, since that is where the pointer was pushed.
  function onOut(e) {
    if (e.relatedTarget) return;
    if (state === 'arming' && e.clientX > EDGE_PX) disarm();
    if (state === 'open' && !tuckTimer) tuckTimer = window.setTimeout(function () { tuckTimer = 0; tuck(); }, TUCK_MS);
  }

  // In the peek, links and buttons work as usual; a click on empty space
  // opens the sidebar fully.
  function onClick(e) {
    if (state !== 'open') return;
    var sb = sidebar();
    if (!sb || !sb.contains(e.target)) return;
    if (e.target.closest('a, button, input, select, textarea, summary, label, [role="button"], [tabindex]')) return;
    e.preventDefault();
    fullOpen();
  }

  document.addEventListener('mousemove', onMove, { passive: true });
  document.addEventListener('mouseout', onOut);
  document.addEventListener('click', onClick, true);
  window.addEventListener('chronicle:navigated', function () { if (state === 'open') tuck(); });
  window.addEventListener('blur', function () { if (state === 'arming') disarm(); });
})();
