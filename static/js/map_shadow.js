/**
 * map_shadow.js -- shadowed areas on a Chronicle map.
 *
 * A shadow is a drawing of type "shadow": two opposite corners in map
 * percentages and a strength in fill_alpha (0.5 "A hint", 0.85 "Almost
 * nothing"). Players see slow dark smoke over the box; owners and co-DMs see
 * a faint version with a dashed outline and a label. What lies under a shadow
 * is withheld by the server; this file is only the picture.
 *
 * Players get ONE smoke element per strength, masked by the union of that
 * strength's rectangles, so overlapping shadows never stack into a darker,
 * blockier patch. DMs get one faint element per rectangle (outline, label).
 *
 * Unexplored hexes (fog of war) join the same hint-level smoke: the hex layer
 * registers a painter with setHexFog and its shapes go into the same mask as
 * the shadow rectangles. attach is a per-map singleton, so the drawing tools
 * and the hex layer share one smoke and one motion loop.
 *
 * Motion rule: one requestAnimationFrame loop drives the drift. It eases to a
 * stop (about a second) when the tab is hidden or after IDLE_MS without input,
 * eases back on the next input or when the tab is visible again, and never
 * runs under prefers-reduced-motion. A stopped loop is not scheduled at all.
 */
(function () {
  'use strict';

  var IDLE_MS = 30000;
  var EASE_MS = 300;       // time constant: ~1s to settle
  var TEXTURE = 320;       // smoke tile size in px
  var STRENGTH_ALMOST = 0.7;

  // The smoke drifts diagonally, and its two layers move at different rates,
  // which is what makes it read as moving volume rather than a sliding image.
  var VEL_X = 9;  // px per second of the primary layer
  var VEL_Y = 4;

  // levelFor maps the stored strength onto the two offered looks.
  function levelFor(alpha) {
    return alpha >= STRENGTH_ALMOST ? 'almost' : 'hint';
  }

  // boxFromPoints returns the ordered box of a shadow's two corners, or null
  // when the points are not exactly two usable coordinate pairs.
  function boxFromPoints(points) {
    if (!Array.isArray(points) || points.length !== 2) return null;
    var a = points[0], b = points[1];
    if (!a || !b || !isFinite(a.x) || !isFinite(a.y) || !isFinite(b.x) || !isFinite(b.y)) return null;
    return {
      minX: Math.min(a.x, b.x), maxX: Math.max(a.x, b.x),
      minY: Math.min(a.y, b.y), maxY: Math.max(a.y, b.y)
    };
  }

  /**
   * createMotion drives the drift. Everything environmental is injected so the
   * rules can be tested without a browser.
   *
   * env: { raf, caf, now, setTimer, clearTimer, reduced, onFrame(x, y, factor),
   *        isActive() }
   */
  function createMotion(env) {
    var factor = 0;          // 0 = still, 1 = full drift
    var target = 0;
    var x = 0, y = 0;
    var rafId = null;
    var last = 0;
    var lastInput = 0;
    var timer = null;
    var hidden = false;
    var destroyed = false;

    function schedule() {
      if (rafId === null && !destroyed) rafId = env.raf(tick);
    }

    function tick(t) {
      rafId = null;
      if (destroyed) return;
      var dt = last ? Math.min(t - last, 100) : 16;
      last = t;
      // Nothing to animate (no shadow layers left, or the tab went away):
      // ease out so the loop ends instead of spinning over an empty pane.
      if (!env.isActive()) target = 0;
      factor += (target - factor) * (1 - Math.exp(-dt / EASE_MS));
      if (Math.abs(target - factor) < 0.01) factor = target;
      x += VEL_X * factor * dt / 1000;
      y += VEL_Y * factor * dt / 1000;
      env.onFrame(x, y, factor);
      if (target === 0 && factor === 0) { last = 0; return; } // loop ends here
      schedule();
    }

    function armIdle() {
      if (timer !== null) return;
      var wait = IDLE_MS - (env.now() - lastInput);
      timer = env.setTimer(function () {
        timer = null;
        if (destroyed) return;
        if (env.now() - lastInput >= IDLE_MS) { target = 0; if (factor !== 0) schedule(); }
        else armIdle();
      }, Math.max(wait, 0));
    }

    function wake() {
      if (destroyed || env.reduced || hidden || !env.isActive()) return;
      lastInput = env.now();
      target = 1;
      schedule();
      armIdle();
    }

    return {
      // Any pointer, key or wheel input.
      input: function () {
        lastInput = env.now();
        if (target === 0) wake();
      },
      // The tab's visibility changed.
      setHidden: function (h) {
        hidden = !!h;
        if (hidden) { target = 0; schedule(); } else wake();
      },
      // A shadow was added: start drifting if allowed.
      start: wake,
      running: function () { return rafId !== null; },
      state: function () { return { factor: factor, target: target, x: x, y: y }; },
      destroy: function () {
        destroyed = true;
        if (rafId !== null) env.caf(rafId);
        rafId = null;
        if (timer !== null) env.clearTimer(timer);
        timer = null;
      }
    };
  }

  // ---- Browser-only part ----

  if (typeof window === 'undefined' || typeof document === 'undefined') {
    if (typeof module !== 'undefined' && module.exports) {
      module.exports = { levelFor: levelFor, boxFromPoints: boxFromPoints, createMotion: createMotion, IDLE_MS: IDLE_MS };
    }
    return;
  }

  var textureURL = null;
  // One seamless smoke tile, made once and shared by every shadow on the page.
  // Each dark blob is also drawn at the wrapped offsets so the tile repeats
  // without a visible seam.
  function smokeTexture() {
    if (textureURL) return textureURL;
    var c = document.createElement('canvas');
    c.width = c.height = TEXTURE;
    var g = c.getContext('2d');
    if (!g) return null;
    var seed = 7;
    function rnd() { seed = (seed * 16807) % 2147483647; return seed / 2147483647; }
    for (var i = 0; i < 90; i++) {
      var cx = rnd() * TEXTURE, cy = rnd() * TEXTURE;
      var r = 40 + rnd() * 80;
      var a = 0.05 + rnd() * 0.16;
      for (var ox = -1; ox <= 1; ox++) {
        for (var oy = -1; oy <= 1; oy++) {
          var px = cx + ox * TEXTURE, py = cy + oy * TEXTURE;
          var grad = g.createRadialGradient(px, py, 0, px, py, r);
          grad.addColorStop(0, 'rgba(6,8,14,' + a + ')');
          grad.addColorStop(1, 'rgba(6,8,14,0)');
          g.fillStyle = grad;
          g.fillRect(px - r, py - r, r * 2, r * 2);
        }
      }
    }
    textureURL = c.toDataURL('image/png');
    return textureURL;
  }

  var styleAdded = false;
  function addStyle() {
    if (styleAdded) return;
    styleAdded = true;
    var smoke = '  background-image: var(--mp-smoke, none), var(--mp-smoke, none);',
        pos = '  background-position: calc(var(--sx) * 1px) calc(var(--sy) * 1px), calc(var(--sx) * -.6px + 90px) calc(var(--sy) * .8px + 40px);';
    var s = document.createElement('style');
    s.textContent = [
      '.mp-shadow-pane { --sx: 0; --sy: 0; }',
      // Players: the union layers. The mask (set per layer from a canvas) is
      // what gives the soft edges.
      '.mp-shadow-union { pointer-events: none; background-color: rgb(10 12 18 / .2);',
      smoke,
      '  background-size: 520px 520px, 340px 340px;',
      pos,
      '  -webkit-backdrop-filter: blur(2.5px) brightness(.8) saturate(.6); backdrop-filter: blur(2.5px) brightness(.8) saturate(.6);',
      '  -webkit-mask-size: 100% 100%; mask-size: 100% 100%; -webkit-mask-repeat: no-repeat; mask-repeat: no-repeat; }',
      '.mp-shadow-union[data-level="almost"] { background-color: rgb(8 10 14 / .32);',
      '  -webkit-backdrop-filter: blur(4px) brightness(.78) saturate(.6); backdrop-filter: blur(4px) brightness(.78) saturate(.6); }',
      // Staff: faint smoke, a hard dashed edge and a label, per rectangle.
      '.mp-shadow { box-sizing: border-box; overflow: hidden; background-color: rgb(10 12 18 / .22);',
      smoke,
      '  background-size: 520px 520px, 340px 340px;',
      pos,
      '  outline: 1.5px dashed rgb(255 255 255 / .6); outline-offset: -1.5px; cursor: inherit; }',
      '.mp-shadow-label { position: absolute; left: 8px; top: 6px; max-width: calc(100% - 16px); padding: 2px 7px; border-radius: 999px;',
      '  background: rgb(10 12 18 / .7); color: #fff; font: 600 11px/1.4 system-ui, sans-serif; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }'
    ].join('\n');
    document.head.appendChild(s);
  }

  // Both element kinds are image overlays whose element is a div: Leaflet's
  // overlay class already handles placement and the zoom animation, which is
  // exactly what a box pinned to image percentages needs.
  var AreaLayer = null, UnionLayer = null, EntryLayer = null;
  function defineLayers() {
    if (AreaLayer || !window.L) return;
    AreaLayer = L.ImageOverlay.extend({
      _initImage: function () {
        var el = this._image = L.DomUtil.create('div', 'mp-shadow leaflet-image-layer' + (this._zoomAnimated ? ' leaflet-zoom-animated' : ''));
        var label = L.DomUtil.create('span', 'mp-shadow-label', el);
        label.textContent = 'Players see a moving shadow here';
        el.onselectstart = L.Util.falseFn;
        el.onmousemove = L.Util.falseFn;
      }
    });
    UnionLayer = L.ImageOverlay.extend({
      _initImage: function () {
        var el = this._image = L.DomUtil.create('div', 'mp-shadow-union leaflet-image-layer' + (this._zoomAnimated ? ' leaflet-zoom-animated' : ''));
        el.setAttribute('data-level', this.options.level);
        el.onselectstart = L.Util.falseFn;
        el.onmousemove = L.Util.falseFn;
      }
    });
    // A player-side shadow has no element of its own; it only registers its
    // rectangle with the union while it is on the map, so the Drawings toggle,
    // removal and reloads all work through the ordinary layer group.
    EntryLayer = L.Layer.extend({
      initialize: function (entry, union) { this._entry = entry; this._union = union; },
      onAdd: function () { this._union.add(this._entry); },
      onRemove: function () { this._union.remove(this._entry); }
    });
  }

  // paintMask returns a data URL whose alpha is the union of `include` boxes
  // minus `exclude` boxes, blurred once as a whole (percent boxes, canvas
  // sized to the map's aspect ratio).
  // extra, when given, paints more included shapes (the fogged hexes) in the
  // canvas's own pixels, before the excluded boxes are cut out.
  function paintMask(include, exclude, aspect, extra) {
    var cw = 512, ch = Math.max(8, Math.round(512 / aspect));
    if (aspect < 1) { ch = 512; cw = Math.max(8, Math.round(512 * aspect)); }
    var src = document.createElement('canvas');
    src.width = cw; src.height = ch;
    var g = src.getContext('2d');
    if (!g) return null;
    function rects(list) {
      list.forEach(function (b) {
        g.fillRect(b.minX / 100 * cw, b.minY / 100 * ch, (b.maxX - b.minX) / 100 * cw, (b.maxY - b.minY) / 100 * ch);
      });
    }
    g.fillStyle = '#000';
    rects(include);
    if (extra) extra(g, cw, ch);
    g.globalCompositeOperation = 'destination-out';
    rects(exclude);
    var out = document.createElement('canvas');
    out.width = cw; out.height = ch;
    var o = out.getContext('2d');
    // ctx.filter is missing in older Safari; the mask is then simply unblurred.
    if ('filter' in o) o.filter = 'blur(' + Math.max(2, cw * 0.008).toFixed(1) + 'px)';
    o.drawImage(src, 0, 0);
    return out.toDataURL('image/png');
  }

  /**
   * attach prepares a viewer mount. opts.toLatLng maps {x,y} percentages to a
   * Leaflet point. Returns { add(drawing, staff), setHexFog(fn), refreshHexFog(),
   * motion, destroy() }; add returns a layer for the caller to put in its
   * drawing group. A map has one smoke: later callers share it, and it goes
   * away when the last of them destroys its handle.
   */
  function attach(map, opts) {
    var shared = map.__mpShadow;
    if (shared) { shared.refs++; return handleOf(map, shared); }
    shared = build(map, opts || {});
    shared.refs = 1;
    map.__mpShadow = shared;
    return handleOf(map, shared);
  }

  // Each caller gets its own handle so that one destroy() never takes the smoke
  // from the other; a second destroy() from the same caller does nothing.
  function handleOf(map, shared) {
    var released = false;
    return {
      add: shared.add,
      motion: shared.motion,
      setHexFog: shared.setHexFog,
      refreshHexFog: shared.refreshHexFog,
      destroy: function () {
        if (released) return;
        released = true;
        if (--shared.refs > 0) return;
        if (map.__mpShadow === shared) map.__mpShadow = null;
        shared.teardown();
      }
    };
  }

  function build(map, opts) {
    var toLatLng = opts.toLatLng;
    addStyle();
    defineLayers();
    // Players' smoke sits above the drawing vectors (overlay pane, 400) and
    // below pins (marker pane, 600); it is never interactive. Staff elements
    // sit BELOW the vectors so drawings and shapes over a shadow stay
    // clickable, while the shadow itself is clickable where nothing else is.
    var pane = map.getPane('mpShadow') || map.createPane('mpShadow');
    pane.style.zIndex = 450;
    var staffPane = map.getPane('mpShadowStaff') || map.createPane('mpShadowStaff');
    staffPane.style.zIndex = 390;
    var url = smokeTexture();
    [pane, staffPane].forEach(function (p) {
      p.classList.add('mp-shadow-pane');
      if (url) p.style.setProperty('--mp-smoke', 'url(' + url + ')');
    });

    var count = 0;
    var reduced = !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);
    var motion = createMotion({
      raf: function (fn) { return window.requestAnimationFrame(fn); },
      caf: function (id) { window.cancelAnimationFrame(id); },
      now: function () { return Date.now(); },
      setTimer: function (fn, ms) { return window.setTimeout(fn, ms); },
      clearTimer: function (id) { window.clearTimeout(id); },
      reduced: reduced,
      isActive: function () { return count > 0 && !document.hidden; },
      onFrame: function (x, y) {
        // One write per frame; every shadow reads these through inheritance.
        [pane, staffPane].forEach(function (p) {
          p.style.setProperty('--sx', x.toFixed(1));
          p.style.setProperty('--sy', y.toFixed(1));
        });
      }
    });

    function onInput() { motion.input(); }
    function onVisibility() { motion.setHidden(document.hidden); }
    var container = map.getContainer();
    ['pointermove', 'pointerdown', 'wheel'].forEach(function (n) { container.addEventListener(n, onInput, { passive: true }); });
    document.addEventListener('keydown', onInput);
    document.addEventListener('visibilitychange', onVisibility);

    // ---- Player union layers ----
    var whole = toLatLng ? L.latLngBounds(toLatLng({ x: 0, y: 0 }), toLatLng({ x: 100, y: 100 })) : null;
    var aspect = 1;
    if (whole) {
      var sz = { w: Math.abs(whole.getEast() - whole.getWest()), h: Math.abs(whole.getNorth() - whole.getSouth()) };
      if (sz.w > 0 && sz.h > 0) aspect = sz.w / sz.h;
    }
    var entries = [];
    var layers = {};
    var pending = false;
    // hexFog paints the unexplored hexes into the hint mask: fn(ctx2d, w, h).
    var hexFog = null;
    function repaint() {
      pending = false;
      var hint = entries.filter(function (e) { return e.level === 'hint'; }).map(function (e) { return e.box; });
      var dense = entries.filter(function (e) { return e.level === 'almost'; }).map(function (e) { return e.box; });
      // The hint layer leaves the dense areas out, so where the two overlap the
      // result is the dense look alone, never hint on top of dense.
      var sets = { hint: [hint, dense], almost: [dense, []] };
      Object.keys(sets).forEach(function (lvl) {
        var inc = sets[lvl][0];
        var layer = layers[lvl];
        var extra = lvl === 'hint' ? hexFog : null;
        if (!inc.length && !extra) { if (layer && map.hasLayer(layer)) map.removeLayer(layer); return; }
        if (!layer) layer = layers[lvl] = new UnionLayer('', whole, { pane: 'mpShadow', interactive: false, level: lvl });
        if (!map.hasLayer(layer)) layer.addTo(map);
        var m = paintMask(inc, sets[lvl][1], aspect, extra);
        var el = layer.getElement();
        if (m && el) {
          var v = 'url(' + m + ')';
          el.style.webkitMaskImage = v;
          el.style.maskImage = v;
        }
      });
    }
    function schedule() {
      if (pending) return;
      pending = true;
      Promise.resolve().then(repaint);
    }
    var union = {
      add: function (e) { entries.push(e); schedule(); },
      remove: function (e) { var i = entries.indexOf(e); if (i !== -1) entries.splice(i, 1); schedule(); }
    };

    function track(layer) {
      layer.on('add', function () { count++; motion.start(); });
      layer.on('remove', function () { count = Math.max(0, count - 1); });
      return layer;
    }

    return {
      refs: 0,
      // setHexFog registers (or, with null, removes) the painter of the fogged
      // hexes. While one is set the smoke counts as active, so it drifts.
      setHexFog: function (fn) {
        var had = !!hexFog;
        hexFog = typeof fn === 'function' ? fn : null;
        if (hexFog && !had) { count++; motion.start(); }
        if (!hexFog && had) count = Math.max(0, count - 1);
        schedule();
      },
      // refreshHexFog repaints after the explored set changed.
      refreshHexFog: function () { if (hexFog) schedule(); },
      add: function (d, staff) {
        var box = boxFromPoints(d.points);
        if (!box || !AreaLayer || !toLatLng) return null;
        if (!staff) {
          return track(new EntryLayer({ box: box, level: levelFor(d.fill_alpha) }, union));
        }
        var bounds = L.latLngBounds(
          toLatLng({ x: box.minX, y: box.minY }),
          toLatLng({ x: box.maxX, y: box.maxY })
        );
        return track(new AreaLayer('', bounds, { pane: 'mpShadowStaff', interactive: true }));
      },
      motion: motion,
      teardown: function () {
        motion.destroy();
        ['pointermove', 'pointerdown', 'wheel'].forEach(function (n) { container.removeEventListener(n, onInput); });
        document.removeEventListener('keydown', onInput);
        document.removeEventListener('visibilitychange', onVisibility);
      }
    };
  }

  window.ChronicleMapShadow = {
    attach: attach,
    levelFor: levelFor,
    boxFromPoints: boxFromPoints,
    createMotion: createMotion
  };
})();
