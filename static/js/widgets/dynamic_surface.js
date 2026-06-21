/**
 * Chronicle Dynamic Surface — motion engine (Wave 1, part of the dynamic-surface frame).
 *
 * The frame OWNS a library of named transition PRESETS; a System (via the surface
 * schema) NAMES which one fits a given box / card / action / overlay — it never
 * writes animation code. This realizes the operator's "a few options to choose
 * from, fitting the type of card" (see Cordinator design §12.7).
 *
 * Presets are small Web-Animations-API routines built over the existing motion
 * tokens (`--ease-*`, `--dur-*`, `--elev-*`) and the dynamic-surface token
 * contract (`--surface-*`), so they stay theme-aware. Every preset collapses to a
 * quick fade / instant under the global `prefers-reduced-motion` guard.
 *
 * Public API (mirrors the `Chronicle.tooltip` helper pattern):
 *   Chronicle.surface.play(name, el, opts) -> Animation | null
 *   Chronicle.surface.transitions          -> the preset map (Systems may read/extend)
 *   Chronicle.surface.reducedMotion()      -> boolean
 *
 * Vanilla browser JS (no build/runtime node dependency), loaded after boot.js.
 */
(function () {
  'use strict';

  if (typeof window === 'undefined' || !window.Chronicle) return;
  var Chronicle = window.Chronicle;

  // ── environment + token helpers ──────────────────────────────────────────

  // cssVar reads a CSS custom property off :root, trimmed, with a fallback.
  function cssVar(name, fallback) {
    try {
      var v = getComputedStyle(document.documentElement).getPropertyValue(name);
      return (v && v.trim()) || fallback;
    } catch (e) {
      return fallback;
    }
  }

  // durMs resolves a duration token ("200ms" / "0.2s") to a number of ms.
  function durMs(name, fallbackMs) {
    var raw = cssVar(name, '');
    if (!raw) return fallbackMs;
    if (raw.indexOf('ms') !== -1) return parseFloat(raw) || fallbackMs;
    if (raw.indexOf('s') !== -1) return (parseFloat(raw) || 0) * 1000 || fallbackMs;
    return parseFloat(raw) || fallbackMs;
  }

  // easeFn resolves an easing token to a CSS timing-function string.
  function easeFn(name, fallback) {
    return cssVar(name, fallback || 'ease');
  }

  // reducedMotion reflects the user's OS "reduce motion" preference.
  function reducedMotion() {
    return !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);
  }

  // applyFinal jumps an element to a keyframe's end state (WAAPI-less fallback).
  function applyFinal(el, frames) {
    var last = frames[frames.length - 1] || {};
    for (var k in last) {
      if (Object.prototype.hasOwnProperty.call(last, k) && k !== 'offset') {
        el.style[k] = last[k];
      }
    }
  }

  // animate is the shared WAAPI runner. opts: { duration, easing, reverse, fill }.
  function animate(el, frames, durationMs, easing, opts) {
    opts = opts || {};
    if (opts.reverse) frames = frames.slice().reverse();
    if (!el || !el.animate) {
      if (el) applyFinal(el, frames);
      return null;
    }
    return el.animate(frames, {
      duration: opts.duration != null ? opts.duration : durationMs,
      easing: opts.easing || easing,
      fill: opts.fill || 'both'
    });
  }

  // ── the preset library (the MENU a System picks from) ─────────────────────

  var TRANSITIONS = {
    // Quiet cross-fade — low-emphasis swaps (tab content, replacements).
    'fade': function (el, opts) {
      return animate(el, [{ opacity: 0 }, { opacity: 1 }],
        durMs('--dur-standard', 200), easeFn('--ease-standard', 'ease'), opts);
    },

    // Scale up + fade from the trigger — DEFAULT for dialogs / overlays.
    'scale-fade': function (el, opts) {
      opts = opts || {};
      if (opts.origin) el.style.transformOrigin = opts.origin;
      return animate(el, [
        { opacity: 0, transform: 'scale(0.92)' },
        { opacity: 1, transform: 'scale(1)' }
      ], durMs('--dur-standard', 200), easeFn('--ease-out', 'ease-out'), opts);
    },

    // Rise toward the viewer with a growing shadow — emphasis / "bring forward".
    'lift': function (el, opts) {
      var rest = cssVar('--elev-resting', '0 1px 2px 0 rgba(0,0,0,0.05)');
      var peak = cssVar('--surface-elev-dragged', cssVar('--elev-dragged', '0 16px 36px -8px rgba(0,0,0,0.22)'));
      return animate(el, [
        { opacity: 0, transform: 'translateY(10px) scale(0.98)', boxShadow: rest },
        { opacity: 1, transform: 'translateY(0) scale(1)', boxShadow: peak }
      ], durMs('--dur-large', 280), easeFn('--ease-out', 'ease-out'), opts);
    },

    // Slide in from an edge — drawers / side panels / mobile sheets.
    'slide': function (el, opts) {
      opts = opts || {};
      var map = { right: 'translateX(24px)', left: 'translateX(-24px)', top: 'translateY(-24px)', bottom: 'translateY(24px)' };
      var off = map[opts.from || 'right'] || map.right;
      return animate(el, [
        { opacity: 0, transform: off }, { opacity: 1, transform: 'none' }
      ], durMs('--dur-standard', 200), easeFn('--ease-out', 'ease-out'), opts);
    },

    // Slide + rotate in from an origin/stack — a cross-ref "dealt" onto the stack.
    'deal': function (el, opts) {
      opts = opts || {};
      var from = opts.originTransform || 'translate(-40px, 30px) rotate(-8deg) scale(0.9)';
      return animate(el, [
        { opacity: 0, transform: from }, { opacity: 1, transform: 'none' }
      ], durMs('--dur-large', 280), easeFn('--ease-out', 'ease-out'), opts);
    },

    // 3D card flip — reveal-style cards (a stat card, a secret).
    'flip': function (el, opts) {
      el.style.backfaceVisibility = 'hidden';
      return animate(el, [
        { opacity: 0.4, transform: 'perspective(900px) rotateY(90deg)' },
        { opacity: 1, transform: 'perspective(900px) rotateY(0deg)' }
      ], durMs('--dur-large', 280), easeFn('--ease-in-out', 'ease-in-out'), opts);
    },

    // Accordion height open — in-list expansion where the element stays put.
    'expand': function (el, opts) {
      el.style.overflow = 'hidden';
      var h = el.scrollHeight;
      return animate(el, [
        { height: '0px', opacity: 0 }, { height: h + 'px', opacity: 1 }
      ], durMs('--dur-standard', 200), easeFn('--ease-in-out', 'ease-in-out'), opts);
    },

    // The existing element slides + grows in place into its detail (Material
    // container transform, via FLIP) — DEFAULT for opening a card into its full
    // view. opts.fromRect = the source element's bounds (getBoundingClientRect).
    'container-transform': function (el, opts) {
      opts = opts || {};
      var fromRect = opts.fromRect;
      if (!fromRect || !el.getBoundingClientRect) {
        return TRANSITIONS['scale-fade'](el, opts); // graceful fallback
      }
      var to = el.getBoundingClientRect();
      var dx = fromRect.left - to.left;
      var dy = fromRect.top - to.top;
      var sx = to.width ? (fromRect.width / to.width) : 1;
      var sy = to.height ? (fromRect.height / to.height) : 1;
      el.style.transformOrigin = 'top left';
      return animate(el, [
        { opacity: opts.fade === false ? 1 : 0.65,
          transform: 'translate(' + dx + 'px,' + dy + 'px) scale(' + sx + ',' + sy + ')' },
        { opacity: 1, transform: 'none' }
      ], durMs('--dur-large', 280), easeFn('--ease-in-out', 'ease-in-out'), opts);
    },

    // Instant — accessibility / "no motion".
    'none': function (el, opts) {
      return animate(el, [{ opacity: 1 }, { opacity: 1 }], 0, easeFn('--ease-standard', 'ease'), opts);
    }
  };

  // ── public selector ───────────────────────────────────────────────────────

  // play applies a named preset to `el`. Unknown names fall back to 'fade'. Under
  // reduce-motion every preset collapses to a quick fade (or instant for 'none').
  function play(name, el, opts) {
    if (!el) return null;
    opts = opts || {};
    if (reducedMotion()) {
      if (name === 'none') return TRANSITIONS['none'](el, { duration: 0 });
      return TRANSITIONS['fade'](el, { duration: 120, reverse: opts.reverse });
    }
    return (TRANSITIONS[name] || TRANSITIONS['fade'])(el, opts);
  }

  // ── injected component styles (widget convention: JS-injected <style>, like
  //    entity_tooltip; component styling lives with the widget, not input.css) ──

  function injectStyles() {
    if (document.getElementById('cs-surface-styles')) return;
    var css = [
      '.cs-overlay-root{position:relative;z-index:100;}',
      '.cs-overlay{position:fixed;inset:0;display:flex;align-items:center;justify-content:center;padding:24px;}',
      '.cs-overlay__backdrop{position:absolute;inset:0;background:var(--surface-overlay,rgba(6,7,11,.55));}',
      '.cs-overlay__panel{position:relative;max-width:min(92vw,560px);max-height:88vh;overflow:auto;' +
        'background:var(--surface-bg,#fff);color:var(--surface-text,#111827);' +
        'border:1px solid var(--surface-border,#e5e7eb);border-radius:16px;' +
        'box-shadow:var(--surface-elev,0 16px 36px -8px rgba(0,0,0,.22));}',
      '.cs-overlay__panel:focus{outline:none;}'
    ].join('');
    var style = document.createElement('style');
    style.id = 'cs-surface-styles';
    style.textContent = css;
    (document.head || document.documentElement).appendChild(style);
  }

  // ── overlay stack (actions push overlays; Escape / backdrop / Back pop one) ──
  //
  // push(content, opts) layers a dialog over the page and animates it in with the
  // chosen preset (default 'scale-fade'); overlays stack (z-index by depth) so a
  // drill-down can deal another card on top. opts: { transition, origin, label,
  // dismissable (default true) }. Focus moves into the panel and is trapped within
  // the TOP layer; pop() restores the prior focus.

  var overlayRoot = null;
  var stack = [];

  function ensureRoot() {
    if (!overlayRoot) {
      overlayRoot = document.createElement('div');
      overlayRoot.className = 'cs-overlay-root';
      document.body.appendChild(overlayRoot);
    }
    return overlayRoot;
  }

  function topEntry() { return stack.length ? stack[stack.length - 1] : null; }

  function focusables(root) {
    var sel = 'a[href],button:not([disabled]),input:not([disabled]),select:not([disabled]),' +
      'textarea:not([disabled]),[tabindex]:not([tabindex="-1"])';
    return Array.prototype.slice.call(root.querySelectorAll(sel)).filter(function (n) {
      return n.offsetParent !== null;
    });
  }

  function pushOverlay(content, opts) {
    opts = opts || {};
    injectStyles();
    ensureRoot();
    var entry = { prevFocus: document.activeElement, opts: opts };

    var layer = document.createElement('div');
    layer.className = 'cs-overlay';
    layer.style.zIndex = String(100 + stack.length);
    var backdrop = document.createElement('div');
    backdrop.className = 'cs-overlay__backdrop';
    var panel = document.createElement('div');
    panel.className = 'cs-overlay__panel';
    panel.setAttribute('role', 'dialog');
    panel.setAttribute('aria-modal', 'true');
    if (opts.label) panel.setAttribute('aria-label', opts.label);
    panel.tabIndex = -1;
    if (typeof content === 'string') panel.innerHTML = content;
    else if (content && content.nodeType) panel.appendChild(content);

    layer.appendChild(backdrop);
    layer.appendChild(panel);
    overlayRoot.appendChild(layer);
    entry.layer = layer; entry.panel = panel; entry.backdrop = backdrop;
    stack.push(entry);

    play('fade', backdrop, {});
    play(opts.transition || 'scale-fade', panel, { origin: opts.origin });

    var first = panel.querySelector('[autofocus]');
    try { (first || panel).focus(); } catch (e) { /* focus is best-effort */ }

    if (opts.dismissable !== false) {
      backdrop.addEventListener('click', function () { popOverlay(); });
    }
    return entry;
  }

  function popOverlay() {
    var entry = stack.pop();
    if (!entry) return null;
    var out = { reverse: true };
    play('fade', entry.backdrop, out);
    var anim = play(entry.opts.transition || 'scale-fade', entry.panel, out);
    function done() {
      if (entry.layer && entry.layer.parentNode) entry.layer.parentNode.removeChild(entry.layer);
      if (entry.prevFocus && entry.prevFocus.focus) {
        try { entry.prevFocus.focus(); } catch (e) { /* element may be gone */ }
      }
    }
    if (anim && anim.finished && anim.finished.then) anim.finished.then(done, done);
    else if (anim) anim.onfinish = done;
    else done();
    return entry;
  }

  function popAllOverlays() { while (stack.length) popOverlay(); }

  // Global key handling for the top layer: Escape pops; Tab is trapped within it.
  document.addEventListener('keydown', function (e) {
    if (!stack.length) return;
    var top = topEntry();
    if (e.key === 'Escape') {
      if (!top || top.opts.dismissable !== false) { e.preventDefault(); e.stopPropagation(); popOverlay(); }
      return;
    }
    if (e.key === 'Tab' && top) {
      var f = focusables(top.panel);
      if (!f.length) { e.preventDefault(); try { top.panel.focus(); } catch (ignore) {} return; }
      var firstEl = f[0], lastEl = f[f.length - 1];
      if (e.shiftKey && document.activeElement === firstEl) { e.preventDefault(); lastEl.focus(); }
      else if (!e.shiftKey && document.activeElement === lastEl) { e.preventDefault(); firstEl.focus(); }
    }
  }, true);

  // ── expose ────────────────────────────────────────────────────────────────

  Chronicle.surface = Chronicle.surface || {};
  Chronicle.surface.transitions = TRANSITIONS;   // the menu (Systems read/extend)
  Chronicle.surface.play = play;
  Chronicle.surface.overlay = { push: pushOverlay, pop: popOverlay, popAll: popAllOverlays };
  Chronicle.surface.reducedMotion = reducedMotion;
  Chronicle.surface.cssVar = cssVar;

  injectStyles();
})();
