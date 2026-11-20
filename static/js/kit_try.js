/**
 * kit_try.js — "Try a style kit here", opened from the header brush
 * (widgets/customize_brush.js), for the campaign's owner.
 *
 * The page is redrawn in another style kit in this browser only: the kit
 * reaches the page the way a saved one does, as data-cz-kit on <html> (and
 * Material's card entrance as data-cz-entrance) for style_kits.css, with the
 * kit's tokens and faces from GET .../customize/kit-try, keyed on
 * html[data-kit-try] so they out-rank the saved kit's (layouts.KitTryCSS).
 * Nothing is written to the server until "Keep this kit", which puts the
 * kit into Customize's draft and opens Customize › Look on it.
 *
 * A strip at the foot of the window names the page, the kit and "not
 * saved", offers the six kits as words, and "Keep this kit" and "Put it
 * back". The try-out follows the owner across pages while the strip is open
 * (sessionStorage, so closing the tab ends it); "Put it back" or Escape in
 * the strip ends it too. The strip Settles in (a fade and a short rise),
 * fades only under Calm and simply appears under Off; picking a kit swaps it
 * at once. kit_try_early.js puts an open try-out back on before a full
 * page load's first paint.
 *
 * Customize › Look uses two more ways in (look_kit_frame.js): dress() puts a
 * kit on the page in its example frame, storing nothing, and walk() opens a
 * page with this tab's try-out on in a kit. A framed page never takes the
 * tab's try-out.
 */
(function () {
  'use strict';
  window.Chronicle = window.Chronicle || {};
  var C = window.Chronicle;
  if (C.kitTry) return;

  var KEY = 'chronicle-kit-try';
  var root = document.documentElement;
  var orig = null;     // the page's own kit and entrance, as the server drew them
  var state = null;    // { c, url, kit, kits, css, entrance } while trying
  var strip = null, styleEl = null, ro = null, onEnd = null, busy = false;

  function save(v) {
    try {
      if (v) sessionStorage.setItem(KEY, JSON.stringify(v));
      else sessionStorage.removeItem(KEY);
    } catch (e) { /* private window: the try-out ends with the page */ }
  }
  function saved() {
    try { return JSON.parse(sessionStorage.getItem(KEY) || 'null'); } catch (e) { return null; }
  }

  // remember keeps what the server drew; kit_try_early.js, having already
  // put a kit on before the first paint, left it on data-kit-saved.
  function remember() {
    if (orig) return;
    if (root.hasAttribute('data-kit-saved')) {
      orig = { kit: root.getAttribute('data-kit-saved'), entrance: root.getAttribute('data-kit-saved-entrance') };
    } else {
      orig = { kit: root.getAttribute('data-cz-kit') || 'material', entrance: root.getAttribute('data-cz-entrance') };
    }
  }
  function setAttr(n, v) {
    if (v == null || v === '') root.removeAttribute(n);
    else root.setAttribute(n, v);
  }

  // dressed is the test style_kits.css makes: a page with a title, and not a
  // workshop, which keeps Material.
  function dressed() {
    var m = document.getElementById('main-content');
    return !!(m && m.querySelector('.cb-title') && !m.querySelector('[data-workshop]'));
  }

  function apply(kit) {
    if (!styleEl) styleEl = document.querySelector('style[data-kit-try-css]');
    if (!styleEl) {
      styleEl = document.createElement('style');
      styleEl.setAttribute('data-kit-try-css', '');
      document.head.appendChild(styleEl);
    }
    if (styleEl.textContent !== state.css) styleEl.textContent = state.css;
    root.setAttribute('data-kit-try', kit);
    setAttr('data-cz-kit', kit === 'material' ? null : kit);
    setAttr('data-cz-entrance', kit === 'material' ? state.entrance : null);
  }
  function unapply() {
    root.removeAttribute('data-kit-try');
    root.removeAttribute('data-kit-saved');
    root.removeAttribute('data-kit-saved-entrance');
    root.style.removeProperty('--kit-try-h');
    if (orig) {
      setAttr('data-cz-kit', orig.kit === 'material' ? null : orig.kit);
      setAttr('data-cz-entrance', orig.entrance);
    }
    if (!styleEl) styleEl = document.querySelector('style[data-kit-try-css]');
    if (styleEl) { styleEl.remove(); styleEl = null; }
  }

  function labelOf(id) {
    for (var i = 0; i < state.kits.length; i++) if (state.kits[i].id === id) return state.kits[i].label;
    return id;
  }
  // The place's name: the page's title where a kit dresses it, else the
  // campaign's.
  function placeName() {
    var t = dressed() ? document.querySelector('#main-content .cb-title') : null;
    var name = t ? (t.querySelector('h1, h2') || t).textContent : '';
    if (!name.trim()) {
      var n = document.querySelector('[data-topbar-name]');
      name = n ? n.textContent : '';
    }
    return name.replace(/\s+/g, ' ').trim();
  }

  function el(tag, cls, text) {
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text) e.textContent = text;
    return e;
  }

  function build() {
    var s = el('div', 'kit-try');
    s.setAttribute('role', 'region');
    s.setAttribute('aria-label', 'Try a style kit');
    s.setAttribute('data-kit-try-strip', '');
    var top = el('div', 'kit-try-top');
    var label = el('p', 'kit-try-l');
    label.setAttribute('aria-live', 'polite');
    label.setAttribute('data-kit-try-label', '');
    var acts = el('div', 'kit-try-acts');
    var keep = el('button', 'kit-try-q', 'Keep this kit');
    keep.type = 'button';
    keep.setAttribute('data-kit-keep', '');
    var back = el('button', 'kit-try-q', 'Put it back');
    back.type = 'button';
    back.setAttribute('data-kit-back', '');
    acts.appendChild(keep);
    acts.appendChild(back);
    top.appendChild(label);
    top.appendChild(acts);
    var kits = el('div', 'kit-try-kits');
    kits.setAttribute('role', 'group');
    kits.setAttribute('aria-label', 'Style kits');
    state.kits.forEach(function (k) {
      var b = el('button', 'kit-try-k');
      b.type = 'button';
      b.setAttribute('data-kit', k.id);
      b.appendChild(el('span', '', k.label));
      if (k.id === orig.kit) b.appendChild(el('span', 'sr-only', ', saved'));
      kits.appendChild(b);
    });
    s.appendChild(top);
    s.appendChild(kits);

    kits.addEventListener('click', function (e) {
      var b = e.target.closest('[data-kit]');
      if (b) pick(b.getAttribute('data-kit'));
    });
    kits.addEventListener('keydown', function (e) {
      var list = Array.prototype.slice.call(kits.querySelectorAll('[data-kit]'));
      var at = list.indexOf(document.activeElement);
      var next = -1;
      if (e.key === 'ArrowRight' || e.key === 'ArrowDown') next = (at + 1) % list.length;
      else if (e.key === 'ArrowLeft' || e.key === 'ArrowUp') next = (at - 1 + list.length) % list.length;
      else if (e.key === 'Home') next = 0;
      else if (e.key === 'End') next = list.length - 1;
      if (next < 0) return;
      e.preventDefault();
      list.forEach(function (b, i) { b.tabIndex = i === next ? 0 : -1; });
      list[next].focus();
      list[next].scrollIntoView({ block: 'nearest', inline: 'nearest' });
    });
    s.addEventListener('keydown', function (e) {
      if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); end(true); }
    });
    keep.addEventListener('click', keepKit);
    back.addEventListener('click', function () { end(true); });
    return s;
  }

  function paint() {
    if (!strip || !state) return;
    var name = placeName();
    strip.querySelector('[data-kit-try-label]').textContent =
      (name ? name + ' in ' : '') + labelOf(state.kit) + (state.kit === orig.kit ? ' · as saved' : ' · not saved');
    Array.prototype.forEach.call(strip.querySelectorAll('[data-kit]'), function (b) {
      var on = b.getAttribute('data-kit') === state.kit;
      b.setAttribute('aria-pressed', on ? 'true' : 'false');
      b.tabIndex = on ? 0 : -1;
    });
    var keep = strip.querySelector('[data-kit-keep]');
    keep.disabled = state.kit === orig.kit || busy;
  }

  // The page keeps room below its last line for the strip. On a phone the
  // strip stands above the table's tab at the window's foot, never on it.
  var phone = window.matchMedia ? window.matchMedia('(max-width: 767.98px)') : null;
  function makeRoom() {
    if (!strip) return;
    var lift = 0, tt = document.getElementById('tt-drawer');
    if (tt && phone && phone.matches) {
      var top = tt.getBoundingClientRect().top;
      if (top > window.innerHeight / 2) lift = Math.max(0, Math.round(window.innerHeight - top));
    }
    strip.style.setProperty('--kit-try-lift', lift + 'px');
    root.style.setProperty('--kit-try-h', (strip.offsetHeight + lift) + 'px');
  }

  function show(animate) {
    if (!strip) {
      strip = build();
      if (animate) strip.classList.add('kt-from');
      document.body.appendChild(strip);
      if (animate) {
        void strip.offsetWidth;
        strip.classList.add('kt-in');
        strip.classList.remove('kt-from');
      }
      if (window.ResizeObserver) { ro = new ResizeObserver(makeRoom); ro.observe(strip); }
    }
    paint();
    makeRoom();
  }

  function hide() {
    if (ro) { ro.disconnect(); ro = null; }
    if (strip) { strip.remove(); strip = null; }
  }

  function pick(kit) {
    if (!state || busy) return;
    state.kit = kit;
    apply(kit);
    save(state);
    paint();
  }

  // end puts the page back as saved; back hands focus to the brush.
  function end(back) {
    var f = onEnd;
    unapply();
    hide();
    save(null);
    state = null;
    onEnd = null;
    if (back && f) f();
  }

  function keepKit() {
    if (!state || busy || state.kit === orig.kit) return;
    busy = true;
    paint();
    var kit = state.kit;
    C.apiFetch(state.url + '/keep', { method: 'POST', body: { kit: kit } }).then(function (r) {
      if (!r.ok) throw new Error('status ' + r.status);
      return r.json();
    }).then(function (j) {
      save(null);
      window.location.assign(j.href);
    }).catch(function () {
      busy = false;
      paint();
      if (C.notify) C.notify('The kit could not be kept. Nothing changed.', 'error');
    });
  }

  function load(url) {
    return C.apiFetch(url).then(function (r) {
      if (!r.ok) throw new Error('status ' + r.status);
      return r.json();
    });
  }

  // open starts a try-out from the brush: the strip appears with the kit the
  // page wears now marked, and focus moves to it.
  function open(opts) {
    if (state) {
      onEnd = opts.onEnd || onEnd;
      var b = strip && strip.querySelector('[aria-pressed="true"]');
      if (b) b.focus();
      return Promise.resolve(true);
    }
    remember();
    return load(opts.url).then(function (j) {
      state = { c: opts.campaign, url: opts.url, kit: orig.kit, kits: j.kits, css: j.css, entrance: j.entrance || '' };
      onEnd = opts.onEnd || null;
      save(state);
      apply(state.kit);
      show(true);
      var b = strip.querySelector('[aria-pressed="true"]');
      if (b) b.focus();
      return true;
    }).catch(function () {
      if (C.notify) C.notify('The style kits could not be loaded. Nothing changed.', 'error');
      return false;
    });
  }

  // resume carries an open try-out onto a page loaded in full: the kit from
  // this tab's try-out is put on again, and its tokens fetched afresh in
  // case the campaign's look changed.
  function resume(opts) {
    var s = saved();
    // A framed page (Look's example) is dressed by its parent, never from
    // the tab's try-out.
    if (window.top !== window) return;
    if (state || !s || s.c !== opts.campaign || s.url !== opts.url || !s.css || !s.kits) return;
    remember();
    state = s;
    onEnd = opts.onEnd || null;
    apply(state.kit);
    show(false);
    load(opts.url).then(function (j) {
      if (!state) return;
      state.css = j.css;
      state.kits = j.kits;
      state.entrance = j.entrance || '';
      apply(state.kit);
      save(state);
    }, function () { /* the stored tokens stand */ });
  }

  // dress puts kit on another document, the page in Customize › Look's
  // example frame, the way apply puts it on this one, from the same tokens
  // (o: { kit, css, entrance }). It stores nothing and shows no strip; the
  // frame's brush and strip are hidden, and the page is inert, a picture.
  // Called again it swaps the kit in place, without reloading the frame.
  var FRAME_CSS = '[data-kit-frame] .tb-brush,[data-kit-frame] .kit-try{display:none!important}';
  function dress(doc, o) {
    var h = doc.documentElement;
    if (!h.hasAttribute('data-kit-saved')) {
      h.setAttribute('data-kit-saved', h.getAttribute('data-cz-kit') || 'material');
      var e = h.getAttribute('data-cz-entrance');
      if (e) h.setAttribute('data-kit-saved-entrance', e);
    }
    var st = doc.querySelector('style[data-kit-try-css]');
    if (!st) {
      st = doc.createElement('style');
      st.setAttribute('data-kit-try-css', '');
      doc.head.appendChild(st);
    }
    var css = FRAME_CSS + o.css;
    if (st.textContent !== css) st.textContent = css;
    h.setAttribute('data-kit-frame', '');
    h.setAttribute('data-kit-try', o.kit);
    if (o.kit === 'material') {
      h.removeAttribute('data-cz-kit');
      if (o.entrance) h.setAttribute('data-cz-entrance', o.entrance);
      else h.removeAttribute('data-cz-entrance');
    } else {
      h.setAttribute('data-cz-kit', o.kit);
      h.removeAttribute('data-cz-entrance');
    }
    if (doc.body) doc.body.inert = true;
  }

  // walk opens href with this tab's try-out on in kit, so the strip is
  // there to walk the owner's pages in it (o: { url, campaign, kit, href,
  // kits, css, entrance }).
  function walk(o) {
    save({ c: o.campaign, url: o.url, kit: o.kit, kits: o.kits, css: o.css, entrance: o.entrance || '' });
    window.location.assign(o.href);
  }

  // A boosted page change keeps the header and the strip; the label follows.
  document.addEventListener('htmx:afterSettle', function () { paint(); makeRoom(); });
  window.addEventListener('resize', makeRoom);

  C.kitTry = { open: open, resume: resume, end: end, dress: dress, walk: walk, dressed: dressed, active: function () { return !!state; } };
})();
