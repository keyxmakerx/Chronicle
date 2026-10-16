/**
 * player_view.js — the owner's "View as player" preview, folding in place.
 *
 * Mount: the header's eye button, <button data-widget="player-view"
 * data-pv-toggle>, drawn for campaign owners only, so only they load this
 * (ADR-063).
 *
 * The header's eye button (owners only) flips a cookie and the page reloads,
 * so the frame (menu, header, banner) is what the server renders for a
 * player. While it is on, <html data-view-as-player> hides every element
 * marked data-dm-only (input.css, "Player view"). That hiding is the owner's
 * preview only: a real player never receives that content from the server.
 *
 * In the Annals page look the switch shows what changes. Going to player
 * view, each Director-only row and block folds away before the reload, and
 * the "Viewing as player" banner folds in after it. Coming back, the banner
 * folds away and the Director-only rows fold back in and glow once. The
 * marks come from the shared block parts, so any block that marks its rows
 * takes part. Calm fades and closes the space at once; Off and reduced
 * motion just swap. The Standard look keeps the plain reload.
 *
 * The page after the reload learns what to play from sessionStorage. A mark
 * that is old, for another page, or that doesn't match the view the server
 * rendered is stale and ignored. This script loads after the first paint,
 * so base.templ's first script sets html[data-pv-reveal] while a fresh mark
 * exists; it holds back what is about to fold in until this script decides,
 * and lifts by itself after a moment.
 */
(function () {
  'use strict';
  window.Chronicle = window.Chronicle || {};
  if (window.Chronicle._playerView) return;

  var KEY = 'chronicle-player-view';
  var MAX_AGE = 15000;
  // How long the page after the reload keeps catching Director-only rows
  // that widgets draw late, before it simply shows any still to come.
  var REVEAL_WINDOW = 2500;

  // arrival decides what this page plays: "" (nothing), "player" (the
  // banner folds in) or "director" (Director-only rows fold in).
  function arrival(raw, now, where, viewing) {
    var m;
    try { m = JSON.parse(raw || ''); } catch (e) { return ''; }
    if (!m || (m.to !== 'player' && m.to !== 'director')) return '';
    if (typeof m.t !== 'number' || now - m.t < 0 || now - m.t > MAX_AGE) return '';
    if (m.path !== where) return '';
    if ((m.to === 'player') !== viewing) return '';
    return m.to;
  }

  // motionOf reads how much motion the page allows: html[data-motion] is
  // calm or off, absent means full; a device asking for less is off.
  function motionOf(h, reduced) {
    if (reduced) return 'off';
    var m = h.getAttribute('data-motion');
    return m === 'calm' || m === 'off' ? m : 'full';
  }

  window.Chronicle._playerView = { arrival: arrival, motionOf: motionOf };

  var h = document.documentElement;
  if (window.Chronicle.register) window.Chronicle.register('player-view', { init: function () {} });
  if (h.getAttribute('data-cz-page') !== 'annals') { h.removeAttribute('data-pv-reveal'); return; }

  function store() { try { return window.sessionStorage; } catch (e) { return null; } }
  function here() { return location.pathname + location.search; }
  function motion() {
    var reduced = !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);
    return motionOf(h, reduced);
  }
  function tok(name, fallback) {
    var v = parseFloat(getComputedStyle(h).getPropertyValue(name));
    return isNaN(v) ? fallback : v;
  }
  function inline(el) { return getComputedStyle(el).display.indexOf('inline') === 0; }

  // A fold animates the element's real height (Fold move); inline marks such
  // as tag chips can't fold, so they fade. Calm fades with the space opening
  // or closing at once; Off swaps.
  function foldOut(el, mo) {
    return new Promise(function (done) {
      function end() { el.style.display = 'none'; done(); }
      if (mo === 'off' || !el.animate) { end(); return; }
      var dur = tok('--dur-micro', 120) + 40;
      var frames = [{ opacity: 1 }, { opacity: 0 }];
      if (mo === 'full' && !inline(el)) {
        var hgt = el.getBoundingClientRect().height;
        el.style.overflow = 'hidden';
        frames = [{ height: hgt + 'px', opacity: 1 }, { height: '0px', opacity: 0, paddingTop: '0px', paddingBottom: '0px', marginTop: '0px', marginBottom: '0px' }];
      }
      var a = el.animate(frames, { duration: dur, easing: 'cubic-bezier(.4,0,1,1)' });
      a.onfinish = a.oncancel = function () { el.style.overflow = ''; end(); };
    });
  }

  function foldIn(el, mo, glow) {
    el.setAttribute('data-pv-shown', '');
    if (mo === 'off' || !el.animate) return;
    var dur = tok('--dur-standard', 200);
    var easing = 'cubic-bezier(.2,.8,.2,1)';
    if (mo === 'full' && !inline(el)) {
      var cs = getComputedStyle(el);
      var to = { height: el.getBoundingClientRect().height + 'px', opacity: 1, paddingTop: cs.paddingTop, paddingBottom: cs.paddingBottom, marginTop: cs.marginTop, marginBottom: cs.marginBottom };
      el.style.overflow = 'hidden';
      var a = el.animate([{ height: '0px', opacity: 0, paddingTop: '0px', paddingBottom: '0px', marginTop: '0px', marginBottom: '0px' }, to], { duration: dur, easing: easing });
      a.onfinish = a.oncancel = function () { el.style.overflow = ''; };
    } else {
      el.animate([{ opacity: 0 }, { opacity: 1 }], { duration: dur, easing: easing });
    }
    if (glow) {
      // The list-add glow: a ring in the campaign accent fading over 1.2s.
      var ring = '0 0 0 2px color-mix(in srgb, var(--color-accent) 45%, transparent), 0 0 18px color-mix(in srgb, var(--color-accent) 28%, transparent)';
      el.animate([{ boxShadow: ring }, { boxShadow: getComputedStyle(el).boxShadow }], { duration: 1200, delay: mo === 'full' ? dur : 0, easing: 'ease-out' });
    }
  }

  function shownMarks() {
    var main = document.getElementById('main-content');
    if (!main) return [];
    return Array.prototype.filter.call(main.querySelectorAll('[data-dm-only]'), function (el) {
      // A mark inside another mark folds with it.
      return el.getClientRects().length && !el.parentElement.closest('[data-dm-only]');
    });
  }

  function banner() { return document.querySelector('[data-pv-banner]'); }

  function restore(els) {
    els.forEach(function (el) { el.style.display = ''; });
  }

  // The switch: play the way out on this page, then let htmx send the toggle;
  // its answer reloads the page. If the toggle fails, put everything back.
  var pending = null;
  document.addEventListener('htmx:confirm', function (e) {
    var elt = e.detail && e.detail.elt;
    if (!elt || !elt.hasAttribute || !elt.hasAttribute('data-pv-toggle') || pending) return;
    e.preventDefault();
    var toPlayer = !h.hasAttribute('data-view-as-player');
    var mo = motion();
    var els = toPlayer ? shownMarks() : [banner()].filter(Boolean);
    pending = els;
    var s = store();
    Promise.all(els.map(function (el) { return foldOut(el, mo); })).then(function () {
      if (s) {
        try { s.setItem(KEY, JSON.stringify({ to: toPlayer ? 'player' : 'director', path: here(), t: Date.now() })); } catch (err) { /* plays nothing next */ }
      }
      e.detail.issueRequest(true);
    });
  });
  function failed(e) {
    var elt = e.detail && e.detail.elt;
    if (!pending || !elt || !elt.hasAttribute || !elt.hasAttribute('data-pv-toggle')) return;
    restore(pending);
    pending = null;
    var s = store();
    if (s) { try { s.removeItem(KEY); } catch (err) { /* nothing to clear */ } }
  }
  document.addEventListener('htmx:responseError', failed);
  document.addEventListener('htmx:sendError', failed);

  // The page after the reload.
  var s = store();
  var raw = null;
  if (s) { try { raw = s.getItem(KEY); s.removeItem(KEY); } catch (err) { raw = null; } }
  var plan = arrival(raw, Date.now(), here(), h.hasAttribute('data-view-as-player'));
  if (!plan) { h.removeAttribute('data-pv-reveal'); return; }
  h.setAttribute('data-pv-reveal', '');

  function play() {
    var mo = motion();
    if (plan === 'player') {
      var b = banner();
      if (b) foldIn(b, mo, false);
      return;
    }
    var main = document.getElementById('main-content');
    function reveal(root) {
      var list = root.matches && root.matches('[data-dm-only]') ? [root] : [];
      if (root.querySelectorAll) list = list.concat(Array.prototype.slice.call(root.querySelectorAll('[data-dm-only]')));
      list = list.filter(function (el) { return !el.hasAttribute('data-pv-shown'); });
      // A mark inside another shows with it, before the outer one is
      // measured, so the fold reaches the outer one's whole height.
      var outer = list.filter(function (el) {
        if (el.parentElement && el.parentElement.closest('[data-dm-only]')) { el.setAttribute('data-pv-shown', ''); return false; }
        return true;
      });
      outer.forEach(function (el) { foldIn(el, mo, true); });
    }
    if (main) reveal(main);
    if (main && window.MutationObserver) {
      var obs = new MutationObserver(function (recs) {
        recs.forEach(function (r) {
          Array.prototype.forEach.call(r.addedNodes, function (n) { if (n.nodeType === 1) reveal(n); });
        });
      });
      obs.observe(main, { childList: true, subtree: true });
      setTimeout(function () { obs.disconnect(); }, REVEAL_WINDOW);
    }
  }
  // After the window, anything else shows as it is drawn.
  setTimeout(function () { h.removeAttribute('data-pv-reveal'); }, REVEAL_WINDOW);

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', function () { requestAnimationFrame(play); });
  else requestAnimationFrame(play);
})();
