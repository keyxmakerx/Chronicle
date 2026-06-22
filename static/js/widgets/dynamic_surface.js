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
      '.cs-overlay__panel:focus{outline:none;}',
      '.cs-overlay__panel--full{max-width:min(96vw,1180px);width:100%;max-height:92vh;}',
      // box primitive
      '.cs-box{border:1px solid var(--surface-border,#e5e7eb);border-radius:12px;' +
        'background:var(--surface-bg,#fff);overflow:hidden;}',
      '.cs-box__head{display:flex;align-items:center;gap:8px;padding:0 14px;}',
      '.cs-box__toggle{display:flex;align-items:center;gap:8px;flex:1;padding:10px 0;' +
        'background:none;border:0;cursor:pointer;color:var(--surface-text,#111827);font:inherit;text-align:left;}',
      '.cs-box__toggle:hover .cs-box__title{color:var(--surface-accent,#6366f1);}',
      '.cs-box__caret{flex:none;width:0;height:0;border-left:5px solid currentColor;' +
        'border-top:4px solid transparent;border-bottom:4px solid transparent;opacity:.6;' +
        'transition:transform var(--surface-dur,200ms) var(--surface-ease,ease);}',
      '.cs-box[data-box-state="expanded"] .cs-box__caret{transform:rotate(90deg);}',
      '.cs-box__title{font-weight:600;letter-spacing:.02em;}',
      '.cs-box__body{padding:0 14px 12px;color:var(--surface-text-body,#374151);}',
      '.cs-box__actions{display:inline-flex;gap:6px;align-items:center;}',
      // surface grid (rows -> columns -> boxes) + actions
      '.cs-surface{display:flex;flex-direction:column;gap:16px;}',
      '.cs-row{display:flex;gap:16px;flex-wrap:wrap;}',
      '.cs-col{display:flex;flex-direction:column;gap:16px;min-width:240px;}',
      '.cs-action{padding:4px 10px;border-radius:8px;border:1px solid var(--surface-border,#e5e7eb);' +
        'background:var(--surface-alt,#f3f4f6);color:var(--surface-text,#111827);font:inherit;font-size:12px;cursor:pointer;}',
      '.cs-action:hover{background:var(--surface-accent,#6366f1);color:#fff;}'
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
    if (opts.panelClass) panel.className += ' ' + opts.panelClass;
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
    play(opts.transition || 'scale-fade', panel, { origin: opts.origin, fromRect: opts.fromRect });

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

  // launch(fromEl, content, opts) — the mini→full move: capture fromEl's screen
  // rect and open `content` as an overlay that GROWS from it (container-transform).
  // The mini card and the full surface should share one provider (same key) so
  // the launch is instant + live. opts override transition/panelClass/etc.
  function launch(fromEl, content, opts) {
    opts = opts || {};
    var rect = null;
    if (fromEl && fromEl.getBoundingClientRect) {
      var r = fromEl.getBoundingClientRect();
      rect = { left: r.left, top: r.top, width: r.width, height: r.height };
    }
    return pushOverlay(content, {
      transition: opts.transition || 'container-transform',
      fromRect: rect,
      panelClass: opts.panelClass || 'cs-overlay__panel--full',
      label: opts.label,
      dismissable: opts.dismissable,
      origin: opts.origin
    });
  }

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

  // ── box primitive (expand / collapse / pin / lazy-load) ────────────────────
  //
  // A `data-widget="surface-box"` element with a `[data-box-toggle]` head and a
  // `[data-box-body]` body. boot.js auto-mounts it. Optional attributes:
  //   data-box-state="collapsed|expanded"  initial state (default expanded)
  //   data-box-transition="expand"          which preset animates the body
  //   data-box-key="vitals"                 persist expanded/collapsed in localStorage
  //   data-box-pinned                       stays as-is (toggle disabled)
  //   data-box-lazy + data-box-endpoint     fetch the body's HTML on first expand

  function lsGet(k) { try { return window.localStorage.getItem(k); } catch (e) { return null; } }
  function lsSet(k, v) { try { window.localStorage.setItem(k, v); } catch (e) { /* private mode */ } }

  function setupBox(el) {
    if (el._csBox) return;
    var head = el.querySelector('[data-box-toggle]');
    var body = el.querySelector('[data-box-body]');
    if (!head || !body) return;
    el._csBox = true;

    var key = el.getAttribute('data-box-key');
    var lsKey = key ? ('cs-box:' + key) : null;
    var stored = lsKey ? lsGet(lsKey) : null;
    var expanded = stored != null ? (stored === '1')
      : (el.getAttribute('data-box-state') !== 'collapsed');
    var transition = el.getAttribute('data-box-transition') || 'expand';
    var loaded = false;

    function maybeLoad() {
      if (loaded) return;
      loaded = true;
      var ep = el.getAttribute('data-box-endpoint');
      if (!el.hasAttribute('data-box-lazy') || !ep || !Chronicle.apiFetch) return;
      Chronicle.apiFetch(ep)
        .then(function (r) { return r && r.text ? r.text() : ''; })
        .then(function (html) {
          // Server-rendered same-origin fragment (same trust model as HTMX swaps).
          body.innerHTML = typeof html === 'string' ? html : '';
          if (Chronicle.mountWidgets) Chronicle.mountWidgets(body);
        })
        .catch(function () { /* surfaced by the host's own error handling */ });
    }

    function render(animate) {
      head.setAttribute('aria-expanded', expanded ? 'true' : 'false');
      el.setAttribute('data-box-state', expanded ? 'expanded' : 'collapsed');
      body.setAttribute('aria-hidden', expanded ? 'false' : 'true');
      // Cancel any in-flight animation so its fill state can't compose with the
      // new one (rapid toggles) and so the height isn't clamped by a stale fill.
      if (body._csAnim) { try { body._csAnim.cancel(); } catch (e) {} body._csAnim = null; }
      if (expanded) {
        body.style.display = '';
        if (animate) {
          var a = play(transition, body, {});
          body._csAnim = a;
          // After expanding, RELEASE the held px height/overflow so the body
          // returns to auto and reflows when its content changes (e.g. lazy load).
          var settle = function () { if (expanded) { body.style.height = ''; body.style.overflow = ''; } };
          if (a && a.finished && a.finished.then) a.finished.then(settle, settle);
          else if (a) a.onfinish = settle; else settle();
        } else {
          body.style.height = ''; body.style.overflow = '';
        }
        maybeLoad();
      } else if (animate) {
        var anim = play(transition, body, { reverse: true });
        body._csAnim = anim;
        var hide = function () { if (!expanded) body.style.display = 'none'; };
        if (anim && anim.finished && anim.finished.then) anim.finished.then(hide, hide);
        else if (anim) anim.onfinish = hide; else hide();
      } else {
        body.style.display = 'none';
      }
    }

    function toggle() {
      if (el.hasAttribute('data-box-pinned')) return;
      expanded = !expanded;
      if (lsKey) lsSet(lsKey, expanded ? '1' : '0');
      render(true);
    }

    head.addEventListener('click', toggle);
    render(false);
  }

  Chronicle.register('surface-box', {
    init: function (el) { setupBox(el); },
    destroy: function (el) { el._csBox = false; }
  });

  // ── data provider (memoized fetch + subscribe) ─────────────────────────────
  //
  // Chronicle.surface.provider(key, fetcher, opts) returns a SHARED provider for
  // `key`: it runs `fetcher()` (a Promise of data) at most once and fans the
  // result to every subscriber, so multiple boxes / a mini + its full surface on
  // one key share ONE fetch and live-update together (push). opts.seed adopts a
  // server-embedded payload (zero network). Self-destroys when the last
  // subscriber leaves. Generalizes the worldstate provider pattern.

  var providers = {};

  function Provider(key, fetcher) {
    this.key = key; this._fetcher = fetcher;
    this.data = null; this.loaded = false; this.error = null;
    this._promise = null; this._subs = []; this._errSubs = [];
  }
  Provider.prototype.load = function () {
    if (this._promise) return this._promise;
    var self = this, out;
    try { out = this._fetcher ? this._fetcher() : Promise.resolve(null); }
    catch (e) { out = Promise.reject(e); }
    this._promise = Promise.resolve(out)
      .then(function (d) { self.data = d; self.loaded = true; self._emit(); return d; })
      .catch(function (e) { self.error = e; self._emitError(e); throw e; });
    return this._promise;
  };
  Provider.prototype.get = function () { return this.load(); };
  Provider.prototype.current = function () { return this.data; };
  // push adopts data (a seed or a live update) and marks the provider RESOLVED
  // (_promise set) so a later load() — which mount() always calls — is a no-op
  // and can't clobber the data with a fetcher-less null. refresh() re-arms it.
  Provider.prototype.push = function (d) { this.data = d; this.loaded = true; this._promise = Promise.resolve(d); this._emit(); };
  Provider.prototype.refresh = function () { this._promise = null; this.error = null; return this.load(); };
  Provider.prototype.subscribe = function (fn) {
    if (typeof fn !== 'function') return function () {};
    this._subs.push(fn);
    if (this.loaded) { try { fn(this.data); } catch (e) {} } else { this.load(); }
    var self = this;
    return function () {
      var i = self._subs.indexOf(fn); if (i >= 0) self._subs.splice(i, 1);
      if (!self._subs.length) self.destroy();
    };
  };
  Provider.prototype.onError = function (fn) {
    if (typeof fn !== 'function') return function () {};
    this._errSubs.push(fn);
    if (this.error) { try { fn(this.error); } catch (e) {} }
    var self = this;
    return function () { var i = self._errSubs.indexOf(fn); if (i >= 0) self._errSubs.splice(i, 1); };
  };
  Provider.prototype._emit = function () {
    var d = this.data;
    this._subs.slice().forEach(function (fn) { try { fn(d); } catch (e) {} });
  };
  Provider.prototype._emitError = function (e) {
    this._errSubs.slice().forEach(function (fn) { try { fn(e); } catch (ee) {} });
  };
  Provider.prototype.destroy = function () {
    this._subs = []; this._errSubs = [];
    if (providers[this.key] === this) delete providers[this.key];
  };

  function provider(key, fetcher, opts) {
    key = key || '';
    var p = providers[key];
    if (!p) {
      p = providers[key] = new Provider(key, fetcher);
      if (opts && opts.seed !== undefined) p.push(opts.seed);
    } else if (fetcher && !p._fetcher) {
      p._fetcher = fetcher;
    }
    return p;
  }

  // ── schema-driven mount (rows → columns → boxes) ───────────────────────────
  //
  // Chronicle.surface.mount(container, schema) assembles a sheet from a
  // declarative schema. The FRAME builds the boxes + wires the provider/actions;
  // a SYSTEM supplies box BODIES via Chronicle.surface.registerBox(name, fn) —
  // frame mounts, system renders.
  //
  //   schema = { provider:{key,endpoint}|{key,seed},
  //              rows:[ { columns:[ { width, boxes:[ boxDef ] } ] } ] }
  //   boxDef = { id, title, block, expand?, pinned?, transition?, lazy?,
  //              endpoint?, actions?:[ {label,on:'overlay'|'api',endpoint?,target?,html?} ] }

  var boxRenderers = {};
  function registerBox(name, fn) { if (name && typeof fn === 'function') boxRenderers[name] = fn; }

  function buildAction(a, prov) {
    var btn = document.createElement('button');
    btn.type = 'button';
    btn.className = 'cs-action';
    btn.textContent = a.label || '';
    btn.addEventListener('click', function (e) {
      e.stopPropagation();   // an action must not toggle its box
      if (a.on === 'overlay') {
        if (a.html) { pushOverlay(a.html, { transition: a.transition || 'scale-fade' }); return; }
        if (a.endpoint && Chronicle.apiFetch) {
          Chronicle.apiFetch(a.endpoint)
            .then(function (r) { return r && r.text ? r.text() : ''; })
            .then(function (html) { pushOverlay(html, { transition: a.transition || 'scale-fade' }); })
            .catch(function () {});
        }
      } else if (a.on === 'api' && a.target && Chronicle.apiFetch) {
        var parts = String(a.target).split(' ');
        var method = parts.length > 1 ? parts[0] : 'POST';
        var url = parts.length > 1 ? parts[1] : a.target;
        Chronicle.apiFetch(url, { method: method })
          .then(function () { if (prov) prov.refresh(); })
          .catch(function () {});
      }
    });
    return btn;
  }

  function buildBox(def, prov, cleanups) {
    var box = document.createElement('section');
    box.className = 'cs-box';
    box.setAttribute('data-box-state', def.expand === 'collapsed' ? 'collapsed' : 'expanded');
    if (def.transition) box.setAttribute('data-box-transition', def.transition);
    if (def.pinned) box.setAttribute('data-box-pinned', '');
    if (def.id) box.setAttribute('data-box-key', def.id);

    var head = document.createElement('div');
    head.className = 'cs-box__head';
    var toggle = document.createElement('button');
    toggle.type = 'button';
    toggle.className = 'cs-box__toggle';
    toggle.setAttribute('data-box-toggle', '');
    toggle.innerHTML = '<span class="cs-box__caret"></span><span class="cs-box__title">' +
      Chronicle.escapeHtml(def.title || '') + '</span>';
    head.appendChild(toggle);
    if (def.actions && def.actions.length) {
      var actWrap = document.createElement('span');
      actWrap.className = 'cs-box__actions';
      def.actions.forEach(function (a) { actWrap.appendChild(buildAction(a, prov)); });
      head.appendChild(actWrap);
    }

    var body = document.createElement('div');
    body.className = 'cs-box__body';
    body.setAttribute('data-box-body', '');
    box.appendChild(head);
    box.appendChild(body);

    if (def.block && boxRenderers[def.block]) {
      var renderFn = boxRenderers[def.block];
      var paint = function (data) {
        var out;
        try { out = renderFn(def, data); } catch (e) { return; }
        if (out == null) return;
        if (typeof out === 'string') body.innerHTML = out;
        else { body.innerHTML = ''; body.appendChild(out); }
        if (Chronicle.mountWidgets) Chronicle.mountWidgets(body);
      };
      if (prov) { var unsub = prov.subscribe(paint); if (cleanups) cleanups.push(unsub); }
      else paint(null);
    } else if (def.lazy && def.endpoint) {
      box.setAttribute('data-box-lazy', '');
      box.setAttribute('data-box-endpoint', def.endpoint);
    }

    setupBox(box);
    return box;
  }

  function mount(container, schema) {
    if (!container || !schema) return null;
    if (typeof schema === 'string') { try { schema = JSON.parse(schema); } catch (e) { return null; } }
    injectStyles();

    var cleanups = [];
    var prov = null;
    if (schema.provider) {
      var pk = schema.provider.key || ('surface:' + (schema.provider.endpoint || ''));
      var ep = schema.provider.endpoint;
      prov = provider(pk, ep ? function () {
        return Chronicle.apiFetch(ep).then(function (r) { return r && r.json ? r.json() : null; });
      } : null, { seed: schema.provider.seed });
    }

    container.classList.add('cs-surface');
    (schema.rows || []).forEach(function (row) {
      var rowEl = document.createElement('div');
      rowEl.className = 'cs-row';
      (row.columns || []).forEach(function (col) {
        var colEl = document.createElement('div');
        colEl.className = 'cs-col';
        colEl.style.flex = (col.width || 12) + ' 1 0';
        (col.boxes || []).forEach(function (boxDef) { colEl.appendChild(buildBox(boxDef, prov, cleanups)); });
        rowEl.appendChild(colEl);
      });
      container.appendChild(rowEl);
    });

    // Teardown: free the provider subscriptions on unmount (the provider then
    // self-destroys when its last subscriber leaves).
    container._csSurfaceCleanup = function () {
      cleanups.forEach(function (u) { try { u(); } catch (e) {} });
      cleanups = [];
    };

    if (prov) prov.load();
    return prov;
  }

  // data-widget="dynamic-surface": read the schema from a data attribute or an
  // inline <script type="application/json" data-surface-schema> child, then mount.
  Chronicle.register('dynamic-surface', {
    init: function (el) {
      if (el._csMounted) return;
      el._csMounted = true;
      var schema = el.getAttribute('data-surface-schema');
      if (!schema) {
        var node = el.querySelector('script[type="application/json"][data-surface-schema]');
        if (node) schema = node.textContent;
      }
      if (schema) mount(el, schema);
    },
    destroy: function (el) {
      el._csMounted = false;
      if (el._csSurfaceCleanup) { el._csSurfaceCleanup(); el._csSurfaceCleanup = null; }
    }
  });

  // ── expose ────────────────────────────────────────────────────────────────

  Chronicle.surface = Chronicle.surface || {};
  Chronicle.surface.transitions = TRANSITIONS;   // the menu (Systems read/extend)
  Chronicle.surface.play = play;
  Chronicle.surface.overlay = { push: pushOverlay, pop: popOverlay, popAll: popAllOverlays };
  Chronicle.surface.launch = launch;             // mini -> full container-transform
  Chronicle.surface.box = setupBox;              // enhance a box element programmatically
  Chronicle.surface.provider = provider;         // shared memoized fetch + subscribe
  Chronicle.surface.mount = mount;               // build a sheet from a schema
  Chronicle.surface.registerBox = registerBox;   // a System supplies a box body renderer
  Chronicle.surface.reducedMotion = reducedMotion;
  Chronicle.surface.cssVar = cssVar;

  injectStyles();
})();
