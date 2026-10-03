/**
 * Map focus view
 *
 * A framed map preview on a page (entity page, later a character sheet) opens
 * the full live map "like a paper map": the panel grows out of the preview's
 * own rectangle, opening sideways first and then downward, with faint fold
 * creases that fade as it flattens. Under prefers-reduced-motion it is a plain
 * 150ms fade. One-shot animations only, nothing loops.
 *
 * The preview element is the trigger and the contract. It carries:
 *   data-viewer-url  the server fragment holding the framed viewer
 *   data-page-url    the map's own page ("Open map page")
 *   data-map-name    the title for the bar
 * The fragment is fetched when the panel opens and handed to
 * ChronicleMapViewer (widgets/map_viewer.js), the same viewer the map page
 * runs, so role handling (players never receive dm_only content; Scribe+ get
 * the tools) is whatever the server rendered into the fragment. This file
 * makes no permission decisions.
 *
 * Esc, the backdrop and the Close button close it; an Esc the viewer used to
 * close one of its own panels (flagged on the event as mpConsumed) does not.
 *
 * Mounted via an inline onclick on the preview (an HTMX-swapped block), so it
 * exposes window.ChronicleMapFocus.open and relies on no delegated listener.
 */
(function () {
  var DURATION = 620;
  var FADE_MS = 150;
  var CLOSE_MS = 180;

  // ---- Pure helpers (unit-tested in test/js/map_focus.test.mjs) ----

  /**
   * flipKeyframes: the panel's transform keyframes for growing out of `from`
   * (the preview's rect) into `to` (the panel's final rect). Two folds: open
   * sideways first (x scale reaches 1 at 45%), then downward, with a slight
   * overshoot before settling. A degenerate target rect yields no animation.
   */
  function flipKeyframes(from, to) {
    if (!from || !to || !to.width || !to.height) return null;
    var sx = from.width / to.width;
    var sy = from.height / to.height;
    var dx = from.left - to.left;
    var dy = from.top - to.top;
    return [
      { transform: 'translate(' + dx + 'px,' + dy + 'px) scale(' + sx + ',' + sy + ')', borderRadius: '8px' },
      { transform: 'translate(' + (dx * 0.5) + 'px,' + dy + 'px) scale(1,' + sy + ')', offset: 0.45 },
      { transform: 'translate(0px,' + (dy * 0.15) + 'px) scale(1,1.02)', offset: 0.85 },
      { transform: 'none' },
    ];
  }

  /**
   * escapeShouldClose: whether an Escape keydown should close the focus view.
   * Not when the viewer already used it to close something inside the map, not
   * when a modifier is held, and not while the marker editor is open (it owns
   * Esc until it is dismissed).
   */
  function escapeShouldClose(e, markerModalOpen) {
    if (!e || e.key !== 'Escape') return false;
    if (e.mpConsumed || e.defaultPrevented) return false;
    if (e.ctrlKey || e.metaKey || e.altKey) return false;
    return !markerModalOpen;
  }

  /** nextFocusIndex: Tab wrap-around inside the dialog. */
  function nextFocusIndex(count, current, backwards) {
    if (count <= 0) return -1;
    if (current < 0) return backwards ? count - 1 : 0;
    return backwards ? (current - 1 + count) % count : (current + 1) % count;
  }

  var api = {
    flipKeyframes: flipKeyframes,
    escapeShouldClose: escapeShouldClose,
    nextFocusIndex: nextFocusIndex,
    DURATION: DURATION,
  };

  if (typeof window === 'undefined') {
    if (typeof module !== 'undefined' && module.exports) module.exports = api;
    return;
  }

  // ---- The view ----

  var current = null; // { root, panel, host, restore, onKey, ... }

  function reduced() {
    return !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);
  }

  function el(tag, cls, attrs) {
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (attrs) for (var k in attrs) e.setAttribute(k, attrs[k]);
    return e;
  }

  function markerModalOpen() {
    var m = document.getElementById('marker-modal');
    return !!(m && !m.classList.contains('hidden'));
  }

  function focusables(root) {
    return Array.prototype.slice.call(root.querySelectorAll(
      'a[href], button:not([disabled]), input:not([disabled]):not([type="hidden"]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'
    )).filter(function (n) { return n.offsetParent !== null || n === document.activeElement; });
  }

  function setNote(host, text) {
    host.innerHTML = '';
    var p = el('p', 'mf-note');
    p.textContent = text;
    host.appendChild(p);
  }

  // load fetches the framed viewer fragment into the panel and starts the
  // viewer on it. A failure leaves a message; the bar's controls still work.
  function load(state) {
    var host = state.host;
    if (state.cfgEl) { window.ChronicleMapViewer.destroy(state.cfgEl); state.cfgEl = null; }
    setNote(host, 'Loading map…');
    var token = ++state.loadToken;
    fetch(state.viewerUrl, { credentials: 'same-origin', headers: { Accept: 'text/html' } })
      .then(function (resp) {
        if (!resp.ok) throw new Error('HTTP ' + resp.status);
        return resp.text();
      })
      .then(function (html) {
        if (current !== state || token !== state.loadToken) return;
        host.innerHTML = html;
        var cfgEl = host.querySelector('#map-config');
        if (!cfgEl || !window.ChronicleMapViewer) throw new Error('viewer markup missing');
        state.cfgEl = cfgEl;
        return window.ChronicleMapViewer.open(cfgEl, {
          // A save that would reload the map page refreshes the unfolded map
          // instead, so the person keeps their place on the page beneath.
          onReload: function () { if (current === state) load(state); },
        });
      })
      .then(function (handle) {
        // The panel may still be mid-animation; re-measure once it can settle.
        if (handle && current === state) {
          state.handle = handle;
          setTimeout(function () { if (current === state && handle.map) handle.map.invalidateSize(); }, DURATION + 40);
        }
      })
      .catch(function (err) {
        if (current !== state) return;
        console.error('[map-focus] could not open map:', err);
        setNote(host, 'The map could not be loaded.');
      });
  }

  function open(src) {
    if (!src || !src.dataset || !src.dataset.viewerUrl) return;
    if (current) close(true);
    var name = src.dataset.mapName || 'Map';

    var root = el('div', 'mf-focus', { role: 'dialog', 'aria-modal': 'true', 'aria-label': 'Map: ' + name });
    var scrim = el('div', 'mf-scrim');
    var panel = el('div', 'mf-panel');
    var bar = el('div', 'mf-bar');
    var icon = el('i', 'fa-solid fa-map-location-dot mf-ico', { 'aria-hidden': 'true' });
    var title = el('b', 'mf-title');
    title.textContent = name;
    var sp = el('span', 'mf-sp');
    var page = el('a', 'mf-btn', { href: src.dataset.pageUrl || '#', 'aria-label': 'Open map page' });
    page.innerHTML = '<i class="fa-solid fa-arrow-up-right-from-square" aria-hidden="true"></i><span>Open map page</span>';
    var x = el('button', 'mf-btn mf-x', { type: 'button', 'aria-label': 'Close map' });
    x.innerHTML = '<i class="fa-solid fa-xmark" aria-hidden="true"></i><span>Close</span>';
    var host = el('div', 'mf-host');
    bar.appendChild(icon); bar.appendChild(title); bar.appendChild(sp); bar.appendChild(page); bar.appendChild(x);
    panel.appendChild(bar); panel.appendChild(host);
    root.appendChild(scrim); root.appendChild(panel);

    var state = {
      root: root, panel: panel, host: host, src: src, viewerUrl: src.dataset.viewerUrl,
      restore: document.activeElement, loadToken: 0, cfgEl: null, handle: null,
      prevOverflow: document.documentElement.style.overflow,
    };
    current = state;
    document.body.appendChild(root);
    document.documentElement.style.overflow = 'hidden';

    scrim.addEventListener('click', function () { close(); });
    x.addEventListener('click', function () { close(); });

    // A window listener runs after the viewer's own document-level Esc handler
    // for the same keystroke, so mpConsumed is already set when this reads it.
    state.onKey = function (e) {
      if (current !== state) return;
      if (e.key === 'Escape') {
        if (escapeShouldClose(e, markerModalOpen())) { e.preventDefault(); close(); }
        return;
      }
      if (e.key === 'Tab' && !markerModalOpen()) {
        var items = focusables(root);
        if (!items.length) return;
        var i = items.indexOf(document.activeElement);
        var inside = root.contains(document.activeElement);
        var edge = e.shiftKey ? i <= 0 : i === items.length - 1;
        if (!inside || edge) {
          e.preventDefault();
          items[nextFocusIndex(items.length, inside ? i : -1, e.shiftKey)].focus();
        }
      }
    };
    window.addEventListener('keydown', state.onKey);

    x.focus({ preventScroll: true });
    animateOpen(state);
    load(state);
  }

  function animateOpen(state) {
    var root = state.root;
    var panel = state.panel;
    if (!root.animate) return;
    if (reduced()) { root.animate([{ opacity: 0 }, { opacity: 1 }], { duration: FADE_MS }); return; }
    var kf = flipKeyframes(state.src.getBoundingClientRect(), panel.getBoundingClientRect());
    if (!kf) return;
    var creases = el('div', 'mf-creases', { 'aria-hidden': 'true' });
    panel.appendChild(creases);
    panel.animate(kf, { duration: DURATION, easing: 'cubic-bezier(.3,.8,.3,1)' });
    var fade = creases.animate([{ opacity: 1 }, { opacity: 0.7, offset: 0.6 }, { opacity: 0 }], { duration: 700, fill: 'forwards' });
    var drop = function () { if (creases.parentNode) creases.parentNode.removeChild(creases); };
    fade.onfinish = drop;
    fade.oncancel = drop;
    state.host.animate([{ opacity: 0.2 }, { opacity: 0.2, offset: 0.3 }, { opacity: 1 }], { duration: 520 });
    state.root.firstChild.animate([{ opacity: 0 }, { opacity: 1 }], { duration: 300 });
  }

  function close(immediate) {
    var state = current;
    if (!state) return;
    current = null;
    window.removeEventListener('keydown', state.onKey);
    if (state.cfgEl && window.ChronicleMapViewer) window.ChronicleMapViewer.destroy(state.cfgEl);
    var done = function () {
      if (state.root.parentNode) state.root.parentNode.removeChild(state.root);
      document.documentElement.style.overflow = state.prevOverflow;
      var back = state.src && state.src.isConnected ? state.src : state.restore;
      if (back && back.focus) { try { back.focus({ preventScroll: true }); } catch (e) { /* element gone */ } }
    };
    if (immediate === true || reduced() || !state.root.animate) { done(); return; }
    var a = state.root.animate([{ opacity: 1 }, { opacity: 0 }], { duration: CLOSE_MS });
    a.onfinish = done;
    a.oncancel = done;
  }

  window.ChronicleMapFocus = { open: open, close: close, flipKeyframes: flipKeyframes, escapeShouldClose: escapeShouldClose };
})();
