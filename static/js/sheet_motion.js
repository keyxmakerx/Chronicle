/**
 * sheet_motion.js -- the motion engine for a game system's character sheet.
 *
 * A sheet is any element with data-sheet. This script gives it a style
 * (data-sheet-style, from html[data-cz-sheet], else "modern"), and moves its
 * parts: a part that holds a data-sheet-open button pulls a panel out from
 * behind the sheet (beside it on a wide desk, as a full page on a narrow one),
 * <details data-sheet-fold> folds open to their real height, and land() shows
 * a changed value the way the style does. The looks are sheet_styles.css and
 * sheet_motion.css; the shared pieces are paper.css. A system's template
 * supplies markup and never writes timing or effects.
 *
 * The contract, for a system's template:
 *   [data-sheet]                        the root; a block element that is not transformed
 *   [data-sheet-folio]                  optional wrapper that shifts left when a panel is beside the sheet
 *   .paper-stack > .paper               the sheet itself
 *   [data-sheet-section="id"]           a part, with .paper-pull; clicking it presses its button
 *   [data-sheet-open="panel-id"]        the button inside a part that opens a panel
 *   <template data-sheet-panel="panel-id" data-title data-kind>  the panel's body
 *   <details data-sheet-fold><summary>..</summary><div class="fold-body"><div class="fold-in" data-move="fold">..
 *   [data-v] / [data-pv]                values snap() and changed() watch
 * Events on the root: sheet:panel-ready (detail.panel, detail.body), sheet:open,
 * sheet:close, sheet:close-blocked (a dirty panel was asked to close).
 *
 * Calm and Off are the site's html[data-motion] (and a person's
 * data-view-motion="calm"): Calm fades in place and opens folds at once, Off
 * jumps to the end state. Nothing runs on a page with no [data-sheet].
 * The pure helpers are exported for test/js/sheet_motion.test.mjs.
 */
(function () {
  'use strict';

  var STYLES = ['modern', 'parchment', 'ledger', 'journal', 'vellum', 'night', 'deck', 'pencil', 'starship', 'neon', 'runes', 'brass'];
  var DEFAULT_STYLE = 'modern';

  // Google Fonts families each style asks for, as css2 family specs. The site's CSP already allows
  // fonts.googleapis.com; the CSS keeps a local fallback stack, so a blocked or slow load degrades to those.
  var FONTS = {
    modern: [],
    parchment: [],
    ledger: ['Source+Serif+4:wght@400;500;600;700', 'Courier+Prime:wght@400;700'],
    journal: ['Alegreya:wght@400;500;700', 'Alegreya+SC:wght@500;700'],
    vellum: ['EB+Garamond:wght@400;500;600;700', 'Cinzel:wght@500;700'],
    night: ['Atkinson+Hyperlegible:wght@400;700', 'Oswald:wght@500;600', 'Share+Tech+Mono'],
    deck: ['Bitter:wght@500;700;800'],
    pencil: ['Source+Serif+4:wght@400;500;600;700', 'Barlow+Condensed:wght@500;600;700', 'Caveat:wght@500;700'],
    starship: ['Exo+2:wght@400;500;600', 'Rajdhani:wght@500;600;700', 'Share+Tech+Mono', 'Orbitron:wght@500;700'],
    neon: ['JetBrains+Mono:wght@400;500;700', 'Big+Shoulders+Display:wght@700;800', 'Barlow+Condensed:wght@500;600;700'],
    runes: ['Source+Serif+4:wght@400;500;600;700', 'Cormorant+SC:wght@600;700', 'Cinzel:wght@500;700', 'Cinzel+Decorative:wght@700', 'Noto+Sans+Runic'],
    brass: ['Lora:wght@400;500;600;700', 'IM+Fell+English+SC', 'Special+Elite']
  };
  // Textures drawn once into a bitmap for the styles that use them (see bake()).
  var BAKED = { runes: '--rn-stone', brass: '--bs-iron' };

  // ---------------------------------------------------------------------
  // Pure helpers
  // ---------------------------------------------------------------------

  function fontHref(style) {
    var f = FONTS[style];
    if (!f || !f.length) return '';
    return 'https://fonts.googleapis.com/css2?family=' + f.join('&family=') + '&display=swap';
  }

  // The campaign's pick wins; an author-set value is kept when there is no pick; anything unknown is Modern.
  function styleFor(cz, existing) {
    if (cz && STYLES.indexOf(cz) >= 0) return cz;
    if (existing && STYLES.indexOf(existing) >= 0) return existing;
    return DEFAULT_STYLE;
  }

  // a: { motion, viewMotion, czReduce } read from <html>. Off wins over Calm.
  function modeOf(a) {
    if (a && a.motion === 'off') return 'off';
    if (a && (a.motion === 'calm' || a.viewMotion === 'calm' || a.czReduce)) return 'calm';
    return 'full';
  }

  // One clock for every slide, slip and pin: --dur-slide in Full, a short fade in Calm, nothing in Off.
  function durFor(mode, fullMs) {
    return { full: fullMs, calm: 120, off: 0 }[mode];
  }

  function msOf(x) {
    x = String(x == null ? '' : x).trim();
    return parseFloat(x) * (/ms$/.test(x) ? 1 : 1000) || 0;
  }

  // How long a move really takes: the longest transition (duration + delay), the lists cycling as CSS does.
  function longestTransition(durations, delays) {
    var d = String(durations || '').split(','), l = String(delays || '0s').split(','), m = 0;
    d.forEach(function (x, i) { m = Math.max(m, msOf(x) + msOf(l[i % l.length])); });
    return m;
  }

  var FX_KINDS = ['rewrite', 'roll', 'glitch', 'carve', 'odometer'];

  // What land() does for a style's --change-move. old is null for something new rather than a changed value.
  function fxPlan(kind, mode, old, now, canAnimate) {
    kind = kind || 'mark';
    if (kind !== 'mark' && mode !== 'off' && canAnimate !== false) {
      if (old == null) return mode === 'calm' ? 'fade-in' : 'arrive:' + kind;
      if (old !== now) return mode === 'calm' ? 'crossfade' : 'fx:' + (FX_KINDS.indexOf(kind) >= 0 ? kind : 'roll');
    }
    return 'mark';
  }

  // Calm and Off open and close a fold at once; only Full measures and animates it.
  function foldPlan(mode) { return mode === 'full' ? 'animate' : 'instant'; }

  // The open/close state machine for the one panel a sheet shows. state is null or { id, trigger, dirty }.
  // Opening the open panel's own button closes it; opening another closes the first unless it holds an
  // unsaved choice, which blocks it ('warn'); closing a dirty panel is blocked unless forced.
  function panelStep(state, ev) {
    if (ev.type === 'open') {
      if (state && state.trigger === ev.trigger) return panelStep(state, { type: 'close', force: false });
      if (state && state.dirty) return { state: state, effects: ['warn'] };
      var next = { id: ev.id, trigger: ev.trigger, dirty: false };
      return { state: next, effects: state ? ['close', 'open'] : ['open'] };
    }
    if (!state) return { state: null, effects: [] };
    if (state.dirty && !ev.force) return { state: state, effects: ['warn'] };
    return { state: null, effects: ['close'] };
  }

  var api = {
    STYLES: STYLES, FONTS: FONTS, BAKED: BAKED, FX_KINDS: FX_KINDS,
    fontHref: fontHref, styleFor: styleFor, modeOf: modeOf, durFor: durFor, msOf: msOf,
    longestTransition: longestTransition, fxPlan: fxPlan, foldPlan: foldPlan, panelStep: panelStep
  };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  if (typeof document === 'undefined') return;

  // ---------------------------------------------------------------------
  // Page helpers
  // ---------------------------------------------------------------------

  var R = document.documentElement;
  function $(s, r) { return (r || document).querySelector(s); }
  function $$(s, r) { return Array.prototype.slice.call((r || document).querySelectorAll(s)); }
  function reflow(el) { void el.offsetWidth; }
  function rootOf(el) { return (el && el.closest && el.closest('[data-sheet]')) || R; }
  function mode() {
    return modeOf({ motion: R.getAttribute('data-motion'), viewMotion: R.getAttribute('data-view-motion'), czReduce: R.hasAttribute('data-cz-reduce') });
  }
  function dur(root) {
    var full = msOf(getComputedStyle(root || R).getPropertyValue('--dur-slide')) || 360;
    return durFor(mode(), full);
  }
  // A style's habits are tokens on the root, never read from the style's name.
  function trait(root, n) { return getComputedStyle(root).getPropertyValue('--' + n).trim(); }
  function role(root, n) { return trait(root, 'sheet-' + n + '-move') || 'slide-out'; }
  function tms(el) {
    var cs = getComputedStyle(el);
    return longestTransition(cs.transitionDuration, cs.transitionDelay);
  }
  function ease(el) {
    var m = /cubic-bezier\([^)]*\)|ease-in-out|ease-in|ease-out|ease|linear/.exec(getComputedStyle(el).transitionTimingFunction);
    return m ? m[0] : 'ease';
  }
  // A layer is promoted only while it moves, so its paper texture is rasterized once instead of on every frame.
  function moving(el, ms) {
    if (!el) return;
    el.classList.add('is-moving');
    clearTimeout(el._mv);
    el._mv = setTimeout(function () { el.classList.remove('is-moving'); }, ms + 80);
  }

  // Fold: the content's real height is measured, animated, then released to auto.
  function fold(d) {
    var body = $('.fold-body', d), inner = $('.fold-in', d), opening = !d.classList.contains('is-in');
    if (d._a) { d._a.cancel(); d._a = null; }
    if (foldPlan(mode()) === 'instant' || !body || !inner || !body.animate) { d.open = opening; reflow(d); d.classList.toggle('is-in', opening); return; }
    var t = tms(inner), e = ease(inner), a;
    if (opening) {
      d.open = true;
      var h = body.scrollHeight;
      reflow(inner);
      d.classList.add('is-in');
      a = body.animate([{ height: '0px' }, { height: h + 'px' }], { duration: t, easing: e });
      a.onfinish = function () { d._a = null; };
    } else {
      var h0 = body.offsetHeight;
      d.classList.remove('is-in');
      a = body.animate([{ height: h0 + 'px' }, { height: '0px' }], { duration: t, easing: e, fill: 'forwards' });
      a.onfinish = function () { d.open = false; d._a = null; a.cancel(); };
    }
    d._a = a;
  }

  // ---------------------------------------------------------------------
  // A changed value
  // ---------------------------------------------------------------------

  function fxKey(e) { return e.hasAttribute('data-v') ? 'v' + e.getAttribute('data-v') : 'pv' + e.getAttribute('data-pv'); }
  // snap/changed: remember the text of every [data-v] / [data-pv] under root before a re-render, then list the ones whose text moved.
  function snap(root) { var m = {}; $$('[data-v],[data-pv]', root).forEach(function (e) { m[fxKey(e)] = e.textContent; }); return m; }
  function changed(root, before) {
    var out = [];
    $$('[data-v],[data-pv]', root).forEach(function (e) { var k = fxKey(e); if (k in before && before[k] !== e.textContent) out.push([e, before[k]]); });
    return out;
  }
  function fxDone(el) { if (el._fxEnd) el._fxEnd(); }
  // An author's panel id goes inside a quoted attribute selector; escape it so a quote or backslash can't break the query.
  function cssStr(v) {
    v = String(v);
    return (typeof CSS !== 'undefined' && CSS.escape) ? CSS.escape(v) : v.replace(/[\\"]/g, '\\$&');
  }

  function node(cls, text) {
    var n = document.createElement('span');
    n.className = cls;
    n.setAttribute('aria-hidden', 'true');
    if (text != null) n.textContent = text;
    return n;
  }
  // The element keeps its new text, wrapped so the effect can clip or hide just that text, with ghost nodes laid over it.
  // Everything is put back when the animations end.
  function fxHost(el, parts) {
    fxDone(el);
    el.classList.add('fx-host');
    var w = document.createElement('span');
    w.className = 'fx-new';
    while (el.firstChild) w.appendChild(el.firstChild);
    el.appendChild(w);
    parts.forEach(function (n) { el.appendChild(n); });
    var h = { el: el, w: w, anims: [] };
    el._fxEnd = function () {
      h.anims.forEach(function (a) { try { a.cancel(); } catch (e) { /* already finished */ } });
      parts.forEach(function (n) { if (n.parentNode) n.parentNode.removeChild(n); });
      while (w.firstChild) el.insertBefore(w.firstChild, w);
      if (w.parentNode) w.parentNode.removeChild(w);
      el.classList.remove('fx-host');
      el._fxEnd = null;
    };
    return h;
  }
  function fxFinish(h) {
    Promise.all(h.anims.map(function (a) { return a.finished; })).then(function () { fxDone(h.el); }, function () { /* cancelled by a newer change */ });
  }
  // Calm, for every style: the old value fades out as the new one fades in.
  function crossfade(el, old) {
    var cs = getComputedStyle(el), g = node('fx-ghost', old), h = fxHost(el, [g]), d = dur(rootOf(el));
    g.style.color = cs.color;
    h.anims.push(g.animate([{ opacity: 1 }, { opacity: 0 }], { duration: d, easing: 'linear', fill: 'both' }), h.w.animate([{ opacity: 0 }, { opacity: 1 }], { duration: d, easing: 'linear' }));
    fxFinish(h);
  }
  // Pencil: the old value is rubbed out (blur, fade, small sideways jitters, a faint smear); the new one is written in left to right.
  function rewrite(el, old) {
    var d = dur(rootOf(el)), E = d * 0.8, W = d * 0.5, g = node('fx-ghost', old), sm = node('fx-smear'), h = fxHost(el, [sm, g]);
    h.anims.push(g.animate([{ transform: 'translateX(0)', filter: 'blur(0px)', opacity: 1, offset: 0 }, { transform: 'translateX(-1.5px)', offset: 0.16 }, { transform: 'translateX(1.5px)', offset: 0.32 },
      { transform: 'translateX(-1px)', offset: 0.5, filter: 'blur(.8px)', opacity: 0.7 }, { transform: 'translateX(1px)', offset: 0.7 }, { transform: 'translateX(0)', filter: 'blur(2px)', opacity: 0, offset: 1 }], { duration: E, easing: 'linear', fill: 'both' }));
    h.anims.push(sm.animate([{ opacity: 0 }, { opacity: 0.9, offset: 0.35 }, { opacity: 0 }], { duration: E + d * 2.4, easing: 'ease-out', fill: 'both' }));
    h.anims.push(h.w.animate([{ clipPath: 'inset(-.3em 100% -.3em -.2em)' }, { clipPath: 'inset(-.3em -.2em -.3em -.2em)' }], { delay: E * 0.9, duration: W, easing: 'cubic-bezier(.4,.1,.5,1)', fill: 'backwards' }));
    fxFinish(h);
  }
  // Starship: the old digits scramble for a few frames and settle on the new value, then it flickers once.
  function roll(el, old) {
    var now = el.textContent, step = Math.max(30, dur(rootOf(el)) * 0.12), color = getComputedStyle(el).color;
    function scr(s, p) { return s.replace(/\d/g, function (c) { return Math.random() < p ? String((+c + 1 + Math.floor(Math.random() * 8)) % 10) : c; }); }
    var frames = /\d/.test(now) && /\d/.test(old) ? [scr(old, 0.7), scr(now, 1), scr(now, 0.5)] : [now, old, now];
    var g = node('fx-ghost');
    g.style.color = color;
    frames.forEach(function (f) { g.appendChild(node('fx-fr', f)); });
    var h = fxHost(el, [g]), total = step * frames.length;
    $$('.fx-fr', g).forEach(function (f, i) { h.anims.push(f.animate([{ visibility: 'visible' }, { visibility: 'visible' }], { delay: i * step, duration: step, fill: 'none' })); });
    h.anims.push(h.w.animate([{ color: 'transparent', textShadow: 'none' }, { color: 'transparent', textShadow: 'none' }], { duration: total, fill: 'none' }));
    h.anims.push(h.w.animate([{ opacity: 0.35 }, { opacity: 1, offset: 0.25 }, { opacity: 0.6, offset: 0.45 }, { opacity: 1 }], { delay: total, duration: step * 2.4, easing: 'steps(4,end)', fill: 'backwards' }));
    fxFinish(h);
  }
  // Neon terminal: the old value splits into a magenta and a cyan copy, offset a few pixels, for three frames (the middle one sliced); then the new value.
  function glitch(el, old) {
    var step = Math.max(34, dur(rootOf(el)) * 0.11), g = node('fx-ghost'), spec = [[-3, 3, 0], [4, -4, 1], [-1.5, 1.5, 0]];
    spec.forEach(function (f) {
      var fr = node('fx-fr'), a = node('gl-a', old), b = node('gl-b', old);
      a.style.transform = 'translateX(' + f[0] + 'px)';
      b.style.transform = 'translateX(' + f[1] + 'px)';
      if (f[2]) { a.style.clipPath = 'inset(0 0 50% 0)'; b.style.clipPath = 'inset(50% 0 0 0)'; }
      fr.appendChild(a); fr.appendChild(b); g.appendChild(fr);
    });
    var h = fxHost(el, [g]), total = step * spec.length;
    $$('.fx-fr', g).forEach(function (f, i) { h.anims.push(f.animate([{ visibility: 'visible' }, { visibility: 'visible' }], { delay: i * step, duration: step, fill: 'none' })); });
    h.anims.push(h.w.animate([{ color: 'transparent', textShadow: 'none' }, { color: 'transparent', textShadow: 'none' }], { duration: total, fill: 'none' }));
    h.anims.push(h.w.animate([{ opacity: 0.5 }, { opacity: 1 }], { delay: total, duration: step, easing: 'steps(2,end)', fill: 'backwards' }));
    fxFinish(h);
  }
  // Rune slate: the old value cracks along a jagged seam, its halves drop away with a few crumbs, and the new value is cut in a stroke at a time with a glow that cools.
  function carve(el, old) {
    var d = dur(rootOf(el)), now = el.textContent, a = node('fx-ghost fx-half', old), b = node('fx-ghost fx-half', old), gl = node('fx-ghost fx-glow', now), cr = [], h, i;
    var y = 44 + Math.round(Math.random() * 10), seam = '-10% ' + (y + 8) + '%,22% ' + (y - 6) + '%,44% ' + (y + 10) + '%,66% ' + (y - 8) + '%,86% ' + (y + 12) + '%,110% ' + y + '%';
    a.style.clipPath = 'polygon(-10% -20%,110% -20%,' + seam.split(',').reverse().join(',') + ')';
    b.style.clipPath = 'polygon(' + seam + ',110% 120%,-10% 120%)';
    for (i = 0; i < 4; i++) { var s = node('fx-sp'); s.style.left = (10 + i * 24 + Math.round(Math.random() * 10)) + '%'; s.style.top = (y - 6 + Math.round(Math.random() * 12)) + '%'; cr.push(s); }
    h = fxHost(el, [a, b, gl].concat(cr));
    var cut = d * 0.5, t0 = d * 0.45, hide = 'inset(-.3em 100% -.3em -.2em)', show = 'inset(-.3em -.2em -.3em -.2em)';
    h.anims.push(a.animate([{ transform: 'translate(0,0) rotate(0deg)', opacity: 1, offset: 0 }, { transform: 'translate(-2px,-1px) rotate(-3deg)', opacity: 1, offset: 0.2 }, { transform: 'translate(-6px,10px) rotate(-14deg)', opacity: 0, offset: 1 }], { duration: d * 0.8, easing: 'cubic-bezier(.5,0,.9,.6)', fill: 'both' }));
    h.anims.push(b.animate([{ transform: 'translate(0,0) rotate(0deg)', opacity: 1, offset: 0 }, { transform: 'translate(2px,1px) rotate(3deg)', opacity: 1, offset: 0.2 }, { transform: 'translate(7px,12px) rotate(16deg)', opacity: 0, offset: 1 }], { duration: d * 0.85, easing: 'cubic-bezier(.5,0,.9,.6)', fill: 'both' }));
    cr.forEach(function (sp, k) { h.anims.push(sp.animate([{ opacity: 0, transform: 'translate(0,0) rotate(0deg)' }, { opacity: 1, offset: 0.15 }, { opacity: 0, transform: 'translate(' + ((k % 2 ? 1 : -1) * (3 + k * 2)) + 'px,' + (16 + k * 3) + 'px) rotate(' + (60 + k * 40) + 'deg)' }], { delay: d * (0.1 + k * 0.04), duration: d * 0.7, easing: 'cubic-bezier(.5,0,.9,.6)', fill: 'both' })); });
    h.anims.push(h.w.animate([{ clipPath: hide }, { clipPath: show }], { delay: t0, duration: cut, easing: 'steps(5,end)', fill: 'backwards' }));
    h.anims.push(gl.animate([{ clipPath: hide, opacity: 1, easing: 'steps(5,end)', offset: 0 }, { clipPath: show, opacity: 1, offset: 0.35 }, { clipPath: show, opacity: 0, offset: 1 }], { delay: t0, duration: cut / 0.35, easing: 'ease-out', fill: 'both' }));
    fxFinish(h);
  }
  // Brass gauges: an odometer. Each changed digit sits in a clipped box and the old and new digits slide past; a value that isn't digits rolls as one box.
  function odometer(el, old) {
    var now = el.textContent, d = dur(rootOf(el)), cs = getComputedStyle(el), lh = parseFloat(cs.lineHeight) || (parseFloat(cs.fontSize) || 14) * 1.2;
    var up = !(parseFloat(now) < parseFloat(old)), g = node('fx-ghost'), cols = [];
    function line(t) { var l = document.createElement('span'); l.textContent = t; l.style.cssText = 'height:' + lh + 'px;line-height:' + lh + 'px'; return l; }
    function col(o, c) {
      var box = node('od-c'), strip = node('od-s');
      box.style.height = lh + 'px';
      if (up) { strip.appendChild(line(o)); strip.appendChild(line(c)); } else { strip.appendChild(line(c)); strip.appendChild(line(o)); }
      box.appendChild(strip); g.appendChild(box); cols.push([strip, g.childNodes.length]);
    }
    if (/\d/.test(old) && /\d/.test(now)) {
      var n = Math.max(old.length, now.length), o = old.padStart(n, ' '), c = now.padStart(n, ' ');
      for (var i = 0; i < n; i++) {
        if (o[i] === c[i] || !(/\d/.test(o[i]) || /\d/.test(c[i]))) { var t = node('od-t', c[i]); t.style.cssText = 'height:' + lh + 'px;line-height:' + lh + 'px'; g.appendChild(t); }
        else col(o[i], c[i]);
      }
    } else col(old, now);
    var h = fxHost(el, [g]), k = cols.length;
    h.anims.push(h.w.animate([{ color: 'transparent', textShadow: 'none' }, { color: 'transparent', textShadow: 'none' }], { duration: d * 0.6 + k * d * 0.05, fill: 'none' }));
    cols.forEach(function (x, j) {
      var from = up ? 0 : -lh, to = up ? -lh : 0;
      h.anims.push(x[0].animate([{ transform: 'translateY(' + from + 'px)' }, { transform: 'translateY(' + to + 'px)' }], { delay: (k - 1 - j) * d * 0.05, duration: d * 0.6, easing: 'cubic-bezier(.3,.7,.2,1)', fill: 'both' }));
    });
    fxFinish(h);
  }
  var FX = { rewrite: rewrite, roll: roll, glitch: glitch, carve: carve, odometer: odometer };

  // Something new rather than a changed value: written in (Pencil), brought up (Starship), flickered on (Neon), cut in (Rune slate) or slid up (Brass).
  function arrive(el, kind) {
    var d = dur(rootOf(el));
    if (kind === 'carve') {
      var cl = 'inset(-.3em 100% -.3em -.2em)', op = 'inset(-.3em -.2em -.3em -.2em)';
      el.animate([{ clipPath: cl, textShadow: '0 0 10px var(--rn-glow)', easing: 'steps(5,end)' }, { clipPath: op, textShadow: '0 0 10px var(--rn-glow)', offset: 0.5 }, { clipPath: op, textShadow: '0 0 0 transparent' }], { duration: d * 1.1 });
      return;
    }
    if (kind === 'odometer') { el.animate([{ clipPath: 'inset(0 0 100% 0)', transform: 'translateY(.4em)' }, { clipPath: 'inset(-.3em -.2em -.3em -.2em)', transform: 'none' }], { duration: d * 0.6, easing: 'cubic-bezier(.3,.7,.2,1)' }); return; }
    if (kind === 'rewrite') el.animate([{ clipPath: 'inset(-.3em 100% -.3em -.2em)' }, { clipPath: 'inset(-.3em -.2em -.3em -.2em)' }], { duration: d * 0.8, easing: 'cubic-bezier(.4,.1,.5,1)' });
    else el.animate([{ opacity: 0 }, { opacity: 1, offset: 0.2 }, { opacity: 0.4, offset: 0.4 }, { opacity: 1, offset: 0.6 }, { opacity: 0.7, offset: 0.8 }, { opacity: 1 }], { duration: d * 0.7, easing: 'steps(6,end)' });
  }

  // What a changed value does is the style's choice (--change-move: mark | rewrite | roll | glitch | carve | odometer),
  // applied to any element: pass it and the text it held before. Without old text the element is new; with unchanged
  // text the style's plain mark plays.
  function land(el, old) {
    if (!el) return;
    fxDone(el);
    var root = rootOf(el), m = mode(), plan = fxPlan(trait(root, 'change-move'), m, old, el.textContent, !!el.animate);
    if (plan === 'fade-in') el.animate([{ opacity: 0 }, { opacity: 1 }], { duration: dur(root) });
    else if (plan === 'crossfade') crossfade(el, old);
    else if (plan.indexOf('arrive:') === 0) arrive(el, plan.slice(7));
    else if (plan.indexOf('fx:') === 0) FX[plan.slice(3)](el, old);
    else { el.classList.remove('paper-landed'); reflow(el); el.classList.add('paper-landed'); }
  }
  // For a button or row whose text lives in a child: the plain mark goes on the host, the effect on the text.
  function landText(host, textEl, old) { if ((trait(rootOf(host), 'change-move') || 'mark') === 'mark') land(host); else land(textEl, old); }

  // ---------------------------------------------------------------------
  // Paper grain, drawn once
  // ---------------------------------------------------------------------

  // The grain and fibre are SVG noise filters. As background images they would be re-run for every tile the
  // browser rasterizes (first paint, scrolling, any repaint of a moving page). They are drawn once into a small
  // bitmap that looks the same, and the token on the sheet root points at that.
  function bake(root, name) {
    try {
      var v = getComputedStyle(root).getPropertyValue(name), m = /url\("(.+)"\)/.exec(v), w = /width='(\d+)'/.exec(v);
      if (!m || !w || !document.createElement('canvas').getContext) return;
      var size = +w[1], sc = (window.devicePixelRatio || 1) >= 1.5 ? 2 : 1, img = new Image();
      img.onload = function () {
        try {
          var c = document.createElement('canvas');
          c.width = c.height = size * sc;
          c.getContext('2d').drawImage(img, 0, 0, size * sc, size * sc);
          // A blob URL, not a data URL: the token is copied into every textured element's style, so it must stay a short string.
          c.toBlob(function (blob) {
            if (!blob) return;
            var u = URL.createObjectURL(blob);
            root.style.setProperty(name, sc > 1 ? 'image-set(url("' + u + '") ' + sc + 'x)' : 'url("' + u + '")');
            root.setAttribute('data-baked', (root.getAttribute('data-baked') || '') + name.slice(2) + ' ');
          }, 'image/png');
        } catch (e) { /* the SVG token stays as it was */ }
      };
      img.src = m[1];
    } catch (e) { /* same */ }
  }
  function bakeOnce(root, name) {
    var done = root._baked || (root._baked = {});
    if (done[name]) return;
    done[name] = 1;
    bake(root, name);
  }

  var fontsAsked = {};
  function ensureFonts(style) {
    var href = fontHref(style);
    if (!href || fontsAsked[href]) return;
    fontsAsked[href] = 1;
    var l = document.createElement('link');
    l.rel = 'stylesheet';
    l.href = href;
    document.head.appendChild(l);
  }

  function applyStyle(root) {
    var style = styleFor(R.getAttribute('data-cz-sheet'), root.getAttribute('data-sheet-style'));
    if (root.getAttribute('data-sheet-style') !== style) root.setAttribute('data-sheet-style', style);
    ensureFonts(style);
    bakeOnce(root, '--paper-grain');
    bakeOnce(root, '--paper-fibre');
    if (BAKED[style]) bakeOnce(root, BAKED[style]);
  }

  // ---------------------------------------------------------------------
  // One sheet
  // ---------------------------------------------------------------------

  var WD = 390, PEEK = 50;

  function mount(root) {
    if (root._sheet) return root._sheet;
    var S = { panel: null }, peekRaf = 0;
    var folio = $('[data-sheet-folio]', root) || root;
    function sheetEl() { return $('.paper-stack > .paper', root) || $('.paper:not(.paper-drawer)', root); }
    function desk() { return root.closest('[data-sheet-desk]') || root.parentElement || root; }
    function narrow() {
      var d = desk(), cs = getComputedStyle(d);
      return d.clientWidth - parseFloat(cs.paddingLeft) - parseFloat(cs.paddingRight) < 820;
    }
    function fire(name, detail) { root.dispatchEvent(new CustomEvent(name, { bubbles: true, detail: detail })); }

    // The trace frame some styles draw around a panel as it opens. It sits beside the card, in its well or layer, so the card's own clip never hides it.
    function addTrace(host, card) {
      if (mode() !== 'full' || !trait(root, 'sheet-trace')) return null;
      var t = document.createElement('i');
      t.className = 'mv-trace';
      t.setAttribute('aria-hidden', 'true');
      t.innerHTML = '<b></b><b></b><b></b><b></b>';
      t.style.cssText = 'left:' + card.offsetLeft + 'px;top:' + card.offsetTop + 'px;width:' + card.offsetWidth + 'px;height:' + card.offsetHeight + 'px';
      host.appendChild(t);
      return t;
    }

    // ----- wells: paper hidden behind an edge, sliding out toward open space
    function makeWell(host, from, child, move) {
      var w = document.createElement('div');
      w.className = 'paper-well';
      w.setAttribute('data-from', from);
      w.appendChild(child);
      if (move) child.setAttribute('data-move', move);
      host.appendChild(w);
      return w;
    }
    // Settle the closed state without a transition (its look can depend on --ox/--oy set a moment ago), so the open transition starts from the right place.
    function rest(el) { el.style.transition = 'none'; reflow(el); el.style.transition = ''; }
    function slideOut(w) {
      rest(w.firstChild);
      moving(w.firstChild, tms(w.firstChild));
      addTrace(w, w.firstChild);
      w.classList.add('is-open');
      if (w.classList.contains('has-over')) {
        var go = function () { w.classList.add('is-over'); };
        if (mode() === 'full') w._t = setTimeout(go, dur(root) * 0.8); else go();
      }
    }
    function slideBack(w, done) {
      clearTimeout(w._t);
      var fin = function () {
        w.classList.remove('is-open');
        moving(w.firstChild, tms(w.firstChild));
        setTimeout(function () { w.remove(); if (done) done(); }, tms(w.firstChild) + 30);
      };
      if (w.classList.contains('is-over')) { w.classList.remove('is-over'); w._t = setTimeout(fin, dur(root) * 0.8); } else fin();
    }

    // ----- peeks: the edge of the paper behind each part
    function layoutPeeks() {
      $$('.paper-peek', folio).forEach(function (p) { p.remove(); });
      var sheet = sheetEl();
      if (!sheet || narrow()) return;
      var fr = folio.getBoundingClientRect(), sr = sheet.getBoundingClientRect(), runs = [];
      $$('[data-sheet-section]', sheet).forEach(function (el) {
        var r = el.getBoundingClientRect(), last = runs[runs.length - 1];
        if (last && r.top - last.bottom < 8) { last.bottom = r.bottom; last.parts.push(el); } else runs.push({ top: r.top, bottom: r.bottom, parts: [el] });
      });
      runs.forEach(function (run) {
        var p = document.createElement('i');
        p.className = 'paper-peek';
        p.setAttribute('aria-hidden', 'true');
        p.style.left = (sr.right - fr.left - 1) + 'px';
        p.style.top = (run.top - fr.top + 5) + 'px';
        p.style.height = Math.max(20, run.bottom - run.top - 10) + 'px';
        folio.appendChild(p);
        run.parts.forEach(function (el) { el._peek = p; });
      });
      if (S.panel && S.panel.part && S.panel.part._peek) S.panel.part._peek.classList.add('is-pulled');
    }
    // Resizes and the sheet's own size changes come in bursts; measure once per frame.
    function schedulePeeks() {
      if (!root.isConnected) { window.removeEventListener('resize', schedulePeeks); return; }
      if (peekRaf) return;
      peekRaf = requestAnimationFrame(function () {
        peekRaf = 0;
        // A panel built for the other layout would be left behind by a resize; put it away.
        if (S.panel && !!S.panel.page !== narrow()) closePanel(false, true);
        layoutPeeks();
      });
    }

    // ----- panels pulled out from behind a part
    function drawer(id, title, kind, body, page) {
      var sec = document.createElement('section'), head = document.createElement('div'), inner = document.createElement('div'), k = document.createElement('div'), h = document.createElement('h2');
      sec.className = 'paper paper-drawer' + (page ? ' pg-card' : '');
      sec.id = 'sheet-panel-' + id;
      sec.setAttribute('data-sheet-panel-open', id);
      sec.setAttribute('role', page ? 'dialog' : 'region');
      if (page) sec.setAttribute('aria-modal', 'true');
      sec.setAttribute('aria-labelledby', 'sheet-panel-h-' + id);
      head.className = 'paper-dhead' + (page ? ' pg-head' : '');
      k.className = 'paper-kind';
      k.textContent = kind;
      h.id = 'sheet-panel-h-' + id;
      h.tabIndex = -1;
      h.textContent = title;
      var close = document.createElement('button');
      close.type = 'button';
      close.setAttribute('data-sheet-close', '');
      if (page) {
        close.className = 'paper-back';
        close.setAttribute('aria-label', 'Back to the sheet');
        close.innerHTML = '<span aria-hidden="true">‹</span> Back';
        var t = document.createElement('div');
        t.className = 'pg-title';
        t.appendChild(k); t.appendChild(h);
        head.appendChild(close); head.appendChild(t);
      } else {
        close.className = 'paper-x';
        close.setAttribute('aria-label', 'Put ' + title + ' back');
        close.textContent = '×';
        inner.appendChild(k); inner.appendChild(h);
        head.appendChild(inner); head.appendChild(close);
      }
      var b = document.createElement('div');
      b.className = 'paper-dbody';
      b.appendChild(body);
      sec.appendChild(head); sec.appendChild(b);
      return sec;
    }

    // Wide desk: the paper comes out beside the part, from behind the sheet's right edge.
    function sidePaper(part, id, title, kind, body) {
      var fr = folio.getBoundingClientRect(), sheet = sheetEl(), sr = sheet.getBoundingClientRect(), pr = part.getBoundingClientRect(), cr = desk().getBoundingClientRect();
      var cur = new DOMMatrixReadOnly(getComputedStyle(folio).transform).m41 || 0;
      // Shift the sheet left as far as the desk allows; whatever still doesn't fit is overlap on the sheet's right column.
      var left0 = sr.left - cur, cap = Math.max(0, left0 - (cr.left + 12)), need = left0 + sr.width + WD - (cr.right - 12);
      var shift = Math.min(cap, Math.max(need, WD / 2)), over = Math.max(0, Math.round(need - shift));
      var move = role(root, 'panel');
      var p = drawer(id, title, kind, body, false), w = makeWell(folio, 'right', p, move);
      p.style.width = WD + 'px';
      p.style.maxHeight = Math.min(sr.height - 28, Math.max(320, innerHeight - 120)) + 'px';
      var ph = p.offsetHeight, rel = sr.right - fr.left, T = pr.top - fr.top - 18;
      T = Math.min(T, sr.bottom - fr.top - 14 - ph);
      T = Math.max(T, sr.top - fr.top - 14);
      w.style.top = T + 'px';
      w.style.height = (ph + 42) + 'px';
      // A Grow starts from the part that was clicked: the paper's left edge, at that part's height.
      p.style.setProperty('--ox', '0px');
      p.style.setProperty('--oy', Math.max(0, Math.min(ph, pr.top + pr.height / 2 - (fr.top + T + 14))) + 'px');
      if (over) {
        w.classList.add(move === 'grow' ? 'pad-over' : 'has-over');
        w.style.setProperty('--clip', (14 + over) + 'px');
        w.style.left = (rel - over - 14) + 'px';
        w.style.width = (WD + 36) + 'px';
      } else {
        w.style.left = rel + 'px';
        w.style.width = (WD + 22) + 'px';
      }
      folio.style.setProperty('--shift', Math.round(shift) + 'px');
      folio.classList.add('is-shifted');
      return { well: w, card: p, start: function () { slideOut(w); } };
    }

    // Narrow desk: the page pulls out of the part by PEEK px, then lifts and moves on top as a full page.
    // Both moves are one geometry (top, left, width and a clip that holds the page behind the part until it lifts).
    function pagePaper(trigger, part, id, title, kind, body) {
      var layer = document.createElement('div'), L = { left: 0, top: 0, width: innerWidth, height: innerHeight };
      layer.className = 'pg-layer';
      root.appendChild(layer);
      var pr = part.getBoundingClientRect(), vt = Math.max(0, pr.top - L.top), vb = Math.min(L.height, pr.bottom - L.top);
      var fw = Math.min(640, L.width - 24);
      var fin = { l: (L.width - fw) / 2, t: 12, w: fw, h: L.height - 24 }, H = fin.h;
      var pl = Math.max(0, pr.left - L.left), pw = Math.max(120, Math.min(pr.width, L.width - pl)), down = (vt + vb) / 2 < L.height / 2;
      var card = drawer(id, title, kind, body, true);
      layer.appendChild(card);
      card.style.height = H + 'px';
      // The default page move pulls the page out of the part, then lifts it (geometry plus a clip). A style that names
      // slide-out or grow hands the page to the engine: it is placed at once and plays that move.
      var mv = role(root, 'page'), eng = mv !== 'pull';
      // The page always has its final box; the pull is a translate and a clip on it, so no frame re-flows the page's text.
      card.style.left = fin.l + 'px'; card.style.top = fin.t + 'px'; card.style.width = fin.w + 'px';
      function put(g) { if (!eng) { card.style.transform = 'translate(' + (g.l - fin.l) + 'px,' + (g.t - fin.t) + 'px)'; card.style.clipPath = g.c; } }
      function ins(top, bottom, w) { return 'inset(' + top + 'px ' + Math.max(0, fin.w - w) + 'px ' + bottom + 'px 0px)'; }
      var full = { l: fin.l, t: fin.t, w: fin.w, c: 'inset(-24px -24px -24px -24px)' };
      var peek = down ? { l: pl, t: vb + PEEK - H, w: pw, c: ins(H - PEEK, 0, pw) } : { l: pl, t: vt - PEEK, w: pw, c: ins(0, H - PEEK, pw) };
      var hid = down ? { l: pl, t: vb - H, w: pw, c: ins(H, 0, pw) } : { l: pl, t: vt, w: pw, c: ins(0, H, pw) };
      function lock(on) { R.style.overflow = on ? 'hidden' : ''; }
      var o = { well: layer, card: card, page: true };
      if (eng) {
        card.setAttribute('data-move', mv);
        card.style.setProperty('--ox', Math.round(pr.left + pr.width / 2 - L.left - fin.l) + 'px');
        card.style.setProperty('--oy', Math.round(pr.top + pr.height / 2 - L.top - fin.t) + 'px');
      }
      o.start = function () {
        lock(true);
        var m = mode();
        if (eng) { moving(card, tms(card)); o.trace = addTrace(layer, card); put(full); rest(card); card.classList.add('is-open'); if (o.trace) o.trace.classList.add('is-open'); layer.classList.add('is-on'); return; }
        moving(card, dur(root) * 2);
        if (m === 'full') { put(hid); reflow(card); put(peek); o._t = setTimeout(function () { put(full); layer.classList.add('is-on'); }, dur(root)); }
        else { put(full); layer.classList.add('is-on'); if (m === 'calm') { card.style.opacity = 0; reflow(card); card.style.opacity = 1; } }
      };
      o.close = function (focus, after) {
        clearTimeout(o._t);
        lock(false);
        if (focus) trigger.focus({ preventScroll: true });
        var done = function () { layer.remove(); if (after) after(); }, m = mode();
        layer.classList.remove('is-on');
        if (eng) { if (o.trace) o.trace.classList.remove('is-open'); card.classList.remove('is-open'); moving(card, tms(card)); o._t = setTimeout(done, tms(card) + 30); return; }
        moving(card, dur(root) * 2);
        if (m === 'full') { put(peek); o._t = setTimeout(function () { put(hid); o._t = setTimeout(done, dur(root) + 20); }, dur(root)); }
        else if (m === 'calm') { card.style.opacity = 0; setTimeout(done, dur(root) + 20); }
        else done();
      };
      return o;
    }

    function panelBody(id) {
      var tpl = $('template[data-sheet-panel="' + cssStr(id) + '"]', root);
      if (!tpl) return null;
      return { tpl: tpl, fragment: tpl.content.cloneNode(true) };
    }

    function openPanel(id, trigger, part) {
      var step = panelStep(S.panel, { type: 'open', id: id, trigger: trigger });
      if (step.effects.indexOf('warn') >= 0) { fire('sheet:close-blocked', { id: S.panel.id }); return false; }
      if (step.effects.indexOf('close') >= 0 && S.panel && S.panel.trigger === trigger) { closePanel(true); return false; }
      var src = panelBody(id);
      if (!src || !sheetEl()) return false;
      if (step.effects.indexOf('close') >= 0) closePanel(false, true);
      part = part || trigger;
      var title = src.tpl.getAttribute('data-title') || trigger.textContent.trim(), kind = src.tpl.getAttribute('data-kind') || '';
      var o = narrow() ? pagePaper(trigger, part, id, title, kind, src.fragment) : sidePaper(part, id, title, kind, src.fragment);
      trigger.setAttribute('aria-expanded', 'true');
      part.classList.add('is-pulled');
      if (part._peek) part._peek.classList.add('is-pulled');
      o.id = id; o.trigger = trigger; o.part = part; o.dirty = false;
      S.panel = o;
      fire('sheet:panel-ready', { id: id, panel: o.card, body: $('.paper-dbody', o.card) });
      o.start();
      fire('sheet:open', { id: id });
      setTimeout(function () { if (S.panel === o) $('h2', o.card).focus({ preventScroll: true }); }, 40);
      return true;
    }

    // Returns false when the panel holds an unsaved choice and force is off: it then reports sheet:close-blocked.
    function closePanel(focus, force, after) {
      var step = panelStep(S.panel, { type: 'close', force: !!force });
      if (step.effects.indexOf('warn') >= 0) { fire('sheet:close-blocked', { id: S.panel.id }); return false; }
      var o = S.panel;
      if (!o) return true;
      S.panel = null;
      o.trigger.setAttribute('aria-expanded', 'false');
      o.part.classList.remove('is-pulled');
      if (o.part._peek) o.part._peek.classList.remove('is-pulled');
      fire('sheet:close', { id: o.id });
      if (o.page) { o.close(focus, after); return true; }
      folio.classList.remove('is-shifted');
      if (focus) o.trigger.focus({ preventScroll: true });
      slideBack(o.well, after);
      return true;
    }

    // ----- events
    root.addEventListener('click', function (e) {
      var sm = e.target.closest && e.target.closest('[data-sheet-fold] > summary');
      if (sm) { e.preventDefault(); fold(sm.parentNode); return; }
      if (e.target.classList && e.target.classList.contains('pg-layer')) { closePanel(true); return; }
      var close = e.target.closest && e.target.closest('[data-sheet-close]');
      if (close) { closePanel(true, !!close.closest('[data-sheet-force-close]')); return; }
      var open = e.target.closest && e.target.closest('[data-sheet-open]');
      if (open) { openPanel(open.getAttribute('data-sheet-open'), open, open.closest('[data-sheet-section]')); return; }
      // Tapping a part anywhere (not only its label) pulls its paper.
      if (e.target.closest && !e.target.closest('a,button,input,select,textarea,summary,label,[contenteditable]')) {
        var part = e.target.closest('[data-sheet-section]');
        var b = part && $('[data-sheet-open]', part);
        if (b && !part.closest('.paper-drawer')) b.click();
      }
    });
    root.addEventListener('keydown', function (e) {
      if (e.key === 'Tab' && S.panel && S.panel.page) {
        var fs = $$('a[href],[contenteditable="true"],button,input,select,textarea,summary,[tabindex]:not([tabindex="-1"])', S.panel.card).filter(function (x) { return !x.disabled && x.offsetParent !== null; });
        if (fs.length) {
          var first = fs[0], last = fs[fs.length - 1];
          if (!S.panel.card.contains(document.activeElement)) { e.preventDefault(); first.focus(); }
          else if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus(); }
          else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus(); }
        }
      }
      if (e.key === 'Escape' && S.panel) { e.preventDefault(); closePanel(true); }
    });
    function hoverPeek(e) {
      var p = e.target.closest && e.target.closest('[data-sheet-section]');
      $$('.paper-peek.is-hover', folio).forEach(function (x) { if (!p || x !== p._peek) x.classList.remove('is-hover'); });
      if (p && p._peek) p._peek.classList.add('is-hover');
    }
    root.addEventListener('mouseover', hoverPeek);
    root.addEventListener('focusin', hoverPeek);
    $$('.paper-settle', root).forEach(function (st) { st.addEventListener('animationend', function (e) { if (e.target === st) st.classList.remove('paper-settle'); }); });

    layoutPeeks();
    var sh = sheetEl();
    if (window.ResizeObserver && sh) new ResizeObserver(schedulePeeks).observe(sh);
    window.addEventListener('resize', schedulePeeks);

    var sheet = {
      root: root,
      open: function (id) { var t = $('[data-sheet-open="' + cssStr(id) + '"]', root); return t ? openPanel(id, t, t.closest('[data-sheet-section]')) : false; },
      close: function (force) { return closePanel(true, force); },
      setDirty: function (on) { if (S.panel) S.panel.dirty = !!on; },
      state: S,
      relayout: schedulePeeks
    };
    root._sheet = sheet;
    return sheet;
  }

  // ---------------------------------------------------------------------
  // Start-up: inert unless the page has a [data-sheet] root.
  // ---------------------------------------------------------------------

  var observer = null;
  function scan() {
    var roots = $$('[data-sheet]');
    if (!roots.length) return;
    roots.forEach(function (root) { applyStyle(root); mount(root); });
    if (observer || !window.MutationObserver) return;
    // The owner's pick can change without a reload (the Customize preview).
    observer = new MutationObserver(function () { $$('[data-sheet]').forEach(applyStyle); });
    observer.observe(R, { attributes: true, attributeFilter: ['data-cz-sheet'] });
  }

  window.Chronicle = window.Chronicle || {};
  window.Chronicle.sheetMotion = {
    mount: mount, land: land, landText: landText, snap: snap, changed: changed, fold: fold, mode: mode, rescan: scan
  };

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', scan);
  else scan();
  // A boosted navigation swaps the page in without a load.
  document.addEventListener('htmx:afterSettle', scan);
})();
