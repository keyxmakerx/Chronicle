/**
 * desk_carry.js -- the line carries you.
 *
 * Going from a desk line into its full Manage page, the sentence you pressed
 * lifts out of its line and becomes the first line of the page it opens: the
 * other lines fall away and the page settles in under it. Pressing the
 * "Manage" word on that page does it backwards: the sentence returns to its
 * line on the desk, the desk settles in, and the line glows for a moment.
 *
 *   Full   the sentence travels (a clone moved with transform; only transform
 *          and opacity ever animate), --dur-slide in and --dur-large back.
 *   Calm   a plain fade: the old page fades out, the new one fades in.
 *   Off    no motion: the link is an ordinary link (and so is a device that
 *          reduces motion, which reads as Off).
 *
 * It navigates the way the sidebar does, with an htmx boosted swap of
 * #main-content (history, title and widgets come from the usual path), and
 * builds on how annals_pull.js drives that swap. A link carrying a modifier
 * key, a page with unsaved changes and a browser without htmx keep the
 * ordinary page load.
 *
 * It must never leave a page half-moved. Every end, whether the move finished,
 * the request failed, the answer was slow, or the person pressed Back, goes
 * through one cleanup that cancels what is animating, removes the travelling
 * clone, shows the real sentence and drops the html[data-desk-trip] flag that
 * hides the page while it is arriving. A press while a move is playing is
 * ignored: the move in progress finishes where it was going.
 *
 * The press arrives from an inline handler built Go-side (campaigns/
 * desk_view.go and layouts/manage_frame.go), because the desk and its frame
 * arrive by swaps where a script tag or a delegated listener may never run.
 * That handler holds the first press while this file loads, and passes
 * late = 1 so a bail-out still goes to the link.
 */
(function () {
  'use strict';

  // What a press plays: 'carry' (full), 'fade' (Calm) or '' (an ordinary
  // link). Anything that cannot run it plays nothing.
  function modeOf(o) {
    if (!o.hasHtmx || o.motion === 'off' || o.reducedMotion) return '';
    if (o.motion === 'calm' || o.czReduce || o.viewMotion === 'calm') return 'fade';
    return 'carry';
  }

  function plainPress(e) {
    return !!e && (e.button === undefined || e.button === 0) && !e.metaKey && !e.ctrlKey && !e.shiftKey && !e.altKey;
  }

  // What can be animated of a page: its content, not the style and script
  // tags the frame carries inside it.
  function movable(main) {
    return Array.prototype.filter.call(main.children, function (el) { return !/^(LINK|SCRIPT|STYLE)$/.test(el.tagName); });
  }

  var api = { modeOf: modeOf, plainPress: plainPress };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  if (typeof document === 'undefined') return;

  var C = (window.Chronicle = window.Chronicle || {});
  var H = document.documentElement;
  var busy = null;
  // A move that gets no answer in this long ends where it is and lets the
  // page arrive the ordinary way.
  var GIVE_UP = 6000;

  function level() {
    var m = H.getAttribute('data-motion');
    return m === 'off' ? 'off' : m === 'calm' ? 'calm' : 'full';
  }
  function token(name, fallback) {
    try {
      var v = getComputedStyle(H).getPropertyValue(name).trim();
      return v || fallback;
    } catch (e) { return fallback; }
  }
  function ms(name, fallback) {
    var v = parseFloat(token(name, ''));
    return isFinite(v) && v >= 0 ? v : fallback;
  }

  function currentMode() {
    var reduce = false;
    try { reduce = window.matchMedia('(prefers-reduced-motion: reduce)').matches; } catch (e) { /* treated as no */ }
    return modeOf({
      hasHtmx: !!(window.htmx && typeof window.htmx.ajax === 'function'),
      motion: H.getAttribute('data-motion'),
      reducedMotion: reduce,
      czReduce: H.hasAttribute('data-cz-reduce'),
      viewMotion: H.getAttribute('data-view-motion')
    });
  }

  function animate(el, frames, opts, t) {
    if (!el || !el.animate) return Promise.resolve();
    var a;
    try { a = el.animate(frames, opts); } catch (e) { return Promise.resolve(); }
    t.anims.push(a);
    return a.finished.then(function () {}, function () {});
  }

  function makeFly(src) {
    var r = src.getBoundingClientRect();
    var f = src.cloneNode(true);
    f.removeAttribute('id');
    Array.prototype.forEach.call(f.querySelectorAll('[id]'), function (n) { n.removeAttribute('id'); });
    f.className = 'desk-s desk-fly';
    f.setAttribute('aria-hidden', 'true');
    f.style.left = r.left + 'px';
    f.style.top = r.top + 'px';
    f.style.width = r.width + 'px';
    document.body.appendChild(f);
    return { el: f, rect: r };
  }

  function lineIdOf(dir, a, main) {
    if (dir === 'in') {
      var ln = a.closest('[data-line]');
      return ln ? ln.getAttribute('data-line') : '';
    }
    var s = main.querySelector('[data-manage-sentence]');
    return s ? s.getAttribute('data-line') : '';
  }

  function sentenceOn(dir, a, main) {
    if (dir === 'in') {
      var ln = a.closest('[data-line]');
      return ln ? ln.querySelector(':scope > .desk-row > .desk-s') : null;
    }
    return main.querySelector('[data-manage-sentence] > .desk-s');
  }

  function destOn(dir, main, lineId) {
    if (dir === 'in') return main.querySelector('[data-manage-sentence] > .desk-s');
    if (!lineId) return null;
    var ln = main.querySelector('[data-line="' + lineId.replace(/[^A-Za-z0-9_-]/g, '') + '"]');
    return ln ? ln.querySelector(':scope > .desk-row > .desk-s') : null;
  }

  // The one way a move ends. It is safe to call more than once and from any
  // state: it leaves the real page showing and nothing animating or hidden.
  function cleanup(t) {
    if (t.over) return;
    t.over = true;
    clearTimeout(t.timer);
    t.anims.forEach(function (a) { try { a.cancel(); } catch (e) { /* gone */ } });
    if (t.fly) t.fly.el.remove();
    [t.src, t.dst].forEach(function (el) {
      if (el && el.style) { el.style.visibility = ''; el.style.opacity = ''; }
    });
    H.removeAttribute('data-desk-trip');
    if (busy === t) busy = null;
    window.removeEventListener('popstate', t.onPop);
    if (t.main && t.onSettle) t.main.removeEventListener('htmx:afterSettle', t.onSettle);
  }

  function land(t, dir, lineId, main) {
    var lv = level();
    var fresh = movable(main);
    var dst = destOn(dir, main, lineId);
    if (dir === 'in') main.scrollTop = 0;
    if (dir === 'back' && dst) {
      var ln = dst.closest('[data-line]');
      if (ln && ln.scrollIntoView) ln.scrollIntoView({ block: 'nearest' });
    }
    t.dst = dst;
    var rise = t.mode === 'carry' && lv === 'full';
    // The place the sentence lands is measured before anything on the new
    // page starts to move, so a rising page never throws the landing off.
    var dRect = dst ? dst.getBoundingClientRect() : null;
    if (dst && t.fly) dst.style.visibility = 'hidden';

    var settle = fresh.map(function (el) {
      var from = rise ? { opacity: 0, transform: 'translateY(6px)' } : { opacity: 0 };
      var to = rise ? { opacity: 1, transform: 'none' } : { opacity: 1 };
      return animate(el, [from, to], {
        duration: t.mode === 'fade' ? ms('--dur-page', 90) : (dir === 'in' ? ms('--dur-large', 280) : ms('--dur-large', 280)),
        delay: t.mode === 'carry' ? (dir === 'in' ? ms('--dur-micro', 120) : 0) : 0,
        easing: t.mode === 'fade' ? 'linear' : token('--ease-out', 'ease-out'),
        fill: 'both'
      }, t);
    });

    var flying = Promise.resolve();
    if (t.fly && dst && dRect) {
      var dx = dRect.left - t.fly.rect.left, dy = dRect.top - t.fly.rect.top;
      var dur = dir === 'in' ? ms('--dur-slide', 360) : ms('--dur-large', 280);
      var ease = token('--ease-slide', 'ease-out');
      // The clone and the real sentence swap over the last quarter of the
      // way, so a line that wraps differently on the page never jumps.
      var tail = Math.round(dur * 0.75);
      flying = Promise.all([
        animate(t.fly.el, [{ transform: 'translate(0,0)' }, { transform: 'translate(' + dx + 'px,' + dy + 'px)' }], { duration: dur, easing: ease, fill: 'forwards' }, t),
        animate(t.fly.el, [{ opacity: 1 }, { opacity: 1, offset: 0.75 }, { opacity: 0 }], { duration: dur, easing: 'linear', fill: 'forwards' }, t),
        animate(dst, [{ opacity: 0 }, { opacity: 1 }], { duration: dur - tail, delay: tail, easing: 'linear', fill: 'backwards' }, t)
      ]).then(function () {});
      dst.style.visibility = '';
    } else if (t.fly) {
      // Nothing to land on: the clone just fades, the page is already right.
      flying = animate(t.fly.el, [{ opacity: 1 }, { opacity: 0 }], { duration: ms('--dur-micro', 120), fill: 'forwards' }, t);
    }

    Promise.all(settle.concat([flying])).then(function () {
      if (t.over) return;
      cleanup(t);
      afterLand(dir, lineId, main, lv);
    });
  }

  function afterLand(dir, lineId, main, lv) {
    if (dir === 'in') {
      var e = main.querySelector('.manage-eyebrow');
      if (e) e.focus({ preventScroll: true });
      return;
    }
    var ln = lineId ? main.querySelector('[data-line="' + lineId.replace(/[^A-Za-z0-9_-]/g, '') + '"]') : null;
    if (!ln) return;
    if (lv !== 'off') {
      ln.classList.remove('is-glow');
      void ln.offsetWidth;
      ln.classList.add('is-glow');
      setTimeout(function () { ln.classList.remove('is-glow'); }, 1300);
    }
    var f = ln.querySelector('.desk-chev') || ln.querySelector('.desk-q');
    if (f) f.focus({ preventScroll: true });
  }

  function trip(dir, a, url, main, mode) {
    var t = { anims: [], fly: null, src: null, dst: null, over: false, mode: mode, timer: null, main: main };
    busy = t;
    var lineId = lineIdOf(dir, a, main);
    var lv = level();
    var fadeMs = mode === 'fade' ? ms('--dur-page', 90) : ms('--dur-micro', 120);

    t.src = mode === 'carry' ? sentenceOn(dir, a, main) : null;
    if (t.src) {
      t.fly = makeFly(t.src);
      t.src.style.visibility = 'hidden';
    }
    H.setAttribute('data-desk-trip', dir);
    // The page you are on falls away.
    movable(main).forEach(function (el) {
      animate(el, [{ opacity: 1 }, { opacity: 0 }], { duration: fadeMs, easing: mode === 'fade' ? 'linear' : token('--ease-in', 'ease-in'), fill: 'forwards' }, t);
    });

    // Back and forward are ordinary history restores: cancel the trip so the
    // restored page shows as it is, and stop the answer from swapping over it.
    t.onPop = function () {
      if (!t.swapped && window.htmx && a.isConnected) { try { window.htmx.trigger(a, 'htmx:abort'); } catch (e) { /* none */ } }
      cleanup(t);
    };
    window.addEventListener('popstate', t.onPop);
    t.timer = setTimeout(function () { cleanup(t); }, GIVE_UP);

    // The swap is delayed so the old page can finish falling away, so the
    // request's own promise (which settles when the answer arrives) says
    // nothing about the page. The swap itself is what is waited for.
    var failed = function (err) {
      if (window.console && console.warn) console.warn('desk carry:', err && err.message);
      if (t.over) return;
      cleanup(t);
      // The person still arrives: the ordinary way.
      location.assign(url.href);
    };
    t.onSettle = function (ev) {
      if (ev.target !== main || t.over) return;
      main.removeEventListener('htmx:afterSettle', t.onSettle);
      a.removeAttribute('hx-push-url');
      if (location.pathname !== url.pathname) { failed(new Error('page not swapped')); return; }
      t.swapped = true;
      land(t, dir, lineId, main);
    };
    main.addEventListener('htmx:afterSettle', t.onSettle);
    a.addEventListener('htmx:responseError', failed);
    a.addEventListener('htmx:sendError', failed);
    a.setAttribute('hx-push-url', 'true');
    window.htmx.ajax('GET', url.pathname + url.search, {
      source: a, target: main, select: '#main-content > *',
      swap: 'innerHTML swap:' + Math.round(fadeMs) + 'ms',
      headers: { 'HX-Boosted': 'true' }
    }).catch(failed);
  }

  function start(dir, a, e, late) {
    if (!a || !a.href) return;
    var url;
    try { url = new URL(a.href, location.href); } catch (err) { return; }
    var main = document.getElementById('main-content');
    var go = function () { if (late) location.assign(a.href); };
    if (!plainPress(e)) return;
    var mode = currentMode();
    if (!mode || !main || a.target || url.origin !== location.origin || (C.isDirty && C.isDirty())) { go(); return; }
    if (e && e.preventDefault) e.preventDefault();
    if (busy) return;
    trip(dir, a, url, main, mode);
  }

  C.deskCarry = {
    go: function (a, e, late) { start('in', a, e, late); },
    back: function (a, e, late) { start('back', a, e, late); },
    // for tests
    busy: function () { return !!busy; },
    abort: function () { if (busy) cleanup(busy); }
  };
})();
