/**
 * map_pictures.js -- pictures placed on a Chronicle map.
 *
 * A picture is a drawing of type "image": two opposite corners in map
 * percentages, a media file (image_id), rotation in degrees, opacity in
 * fill_alpha (0.2-1), an edge crop {t,r,b,l} in percent of the picture, a
 * stacking number (sort_order) and visibility ("dm_only" hides it from
 * players). Everyone sees pictures; only people who may draw can select and
 * edit them. The server withholds hidden pictures, so this file is only the
 * picture on screen.
 *
 * Pictures are DOM elements in a Leaflet pane BELOW the vector drawings
 * (z-index 380), so shapes and pins stay clickable over a picture. Each is an
 * outer box that Leaflet moves and zooms, an inner box that carries rotation,
 * opacity and the crop, and a frame (never clipped) that carries the outline
 * and the handles.
 *
 * The pure geometry lives at the top and is exported for tests; the browser
 * part needs Leaflet and a drawing module that supplies the coordinate
 * helpers and saves edits.
 */
(function () {
  'use strict';

  var MIN_OPACITY = 0.2;
  var MAX_CROP = 45;
  var MAX_TURN = 45;
  // A picture never shrinks below this share of the map width, so its
  // handles stay reachable.
  var MIN_WIDTH_PCT = 2;

  // ---- Pure geometry ----

  function clamp(v, lo, hi) { return Math.min(hi, Math.max(lo, v)); }

  // clampOpacity mirrors the server: unset means opaque, never below 0.2.
  function clampOpacity(a) {
    a = Number(a);
    if (!isFinite(a) || a === 0) return 1;
    return clamp(a, MIN_OPACITY, 1);
  }

  function clampTurn(deg) {
    deg = Number(deg);
    return isFinite(deg) ? clamp(deg, -MAX_TURN, MAX_TURN) : 0;
  }

  // normalizeCrop returns all four sides, each 0-45, whatever was stored.
  function normalizeCrop(c) {
    c = c || {};
    function side(v) { v = Number(v); return isFinite(v) ? clamp(v, 0, MAX_CROP) : 0; }
    return { t: side(c.t), r: side(c.r), b: side(c.b), l: side(c.l) };
  }

  function cropIsEmpty(c) { return !c.t && !c.r && !c.b && !c.l; }

  // clipPath is the CSS that trims the picture by its crop.
  function clipPath(c) {
    c = normalizeCrop(c);
    if (cropIsEmpty(c)) return 'none';
    return 'inset(' + c.t + '% ' + c.r + '% ' + c.b + '% ' + c.l + '%)';
  }

  // boxFromPoints orders a picture's two corners, or returns null when the
  // points are not exactly two usable coordinate pairs.
  function boxFromPoints(points) {
    if (!Array.isArray(points) || points.length !== 2) return null;
    var a = points[0], b = points[1];
    if (!a || !b || !isFinite(a.x) || !isFinite(a.y) || !isFinite(b.x) || !isFinite(b.y)) return null;
    return {
      minX: Math.min(a.x, b.x), maxX: Math.max(a.x, b.x),
      minY: Math.min(a.y, b.y), maxY: Math.max(a.y, b.y)
    };
  }

  function round(v, places) { var f = Math.pow(10, places); return Math.round(v * f) / f; }

  // geoFromBox / boxFromGeo convert between percentages and a centre-based box
  // in map units, where rotation and resizing are easiest to reason about.
  function geoFromBox(box, mapW, mapH) {
    var w = (box.maxX - box.minX) / 100 * mapW;
    var h = (box.maxY - box.minY) / 100 * mapH;
    return { cx: (box.minX / 100 * mapW) + w / 2, cy: (box.minY / 100 * mapH) + h / 2, w: w, h: h };
  }

  function pointsFromGeo(g, mapW, mapH) {
    return [
      { x: round((g.cx - g.w / 2) / mapW * 100, 3), y: round((g.cy - g.h / 2) / mapH * 100, 3) },
      { x: round((g.cx + g.w / 2) / mapW * 100, 3), y: round((g.cy + g.h / 2) / mapH * 100, 3) }
    ];
  }

  // rotate turns a vector by deg clockwise on screen (y grows downwards).
  function rotate(x, y, deg) {
    var r = deg * Math.PI / 180, c = Math.cos(r), s = Math.sin(r);
    return { x: x * c - y * s, y: x * s + y * c };
  }

  // scaleGeoAbout scales a box by s while a chosen point stays where it is on
  // screen. (rx, ry) is that point relative to the box centre, in the box's own
  // (unrotated) frame; this is what keeps a turned picture from swinging
  // around its centre while it is resized.
  function scaleGeoAbout(g, rotDeg, rx, ry, s) {
    var d = rotate(rx, ry, rotDeg);
    return { cx: g.cx + (1 - s) * d.x, cy: g.cy + (1 - s) * d.y, w: g.w * s, h: g.h * s };
  }

  // cropDrag moves one edge of the crop by a drag delta given in the picture's
  // own (unrotated) frame and map units, keeping every side within 0-45.
  function cropDrag(side, crop, ldx, ldy, w, h) {
    var c = normalizeCrop(crop);
    if (side === 't') c.t += ldy / h * 100;
    else if (side === 'b') c.b -= ldy / h * 100;
    else if (side === 'l') c.l += ldx / w * 100;
    else if (side === 'r') c.r -= ldx / w * 100;
    return {
      t: round(clamp(c.t, 0, MAX_CROP), 1), r: round(clamp(c.r, 0, MAX_CROP), 1),
      b: round(clamp(c.b, 0, MAX_CROP), 1), l: round(clamp(c.l, 0, MAX_CROP), 1)
    };
  }

  // resizeScale turns a corner drag into a scale factor that keeps the picture's
  // proportions: the average of the horizontal and vertical stretch of the
  // visible part, held between the smallest and largest sensible size.
  function resizeScale(visW, visH, ldx, ldy, g, mapW) {
    var s = 1 + (ldx / visW + ldy / visH) / 2;
    var minS = (mapW * MIN_WIDTH_PCT / 100) / g.w;
    var maxS = (mapW * 4) / g.w;
    return clamp(s, minS, maxS);
  }

  // placeNew sizes a new picture to about a third of the view's width, keeps its
  // proportions, never lets it fill more than 60% of the view's height, and
  // centres it on the view. Everything is in map units; the result is two
  // percentage corners.
  function placeNew(natW, natH, viewW, viewH, centre, mapW, mapH) {
    var ratio = natW > 0 && natH > 0 ? natH / natW : 1;
    var w = Math.min(viewW * 0.33, mapW * 0.9);
    var h = w * ratio;
    var maxH = Math.min(viewH * 0.6, mapH * 0.9);
    if (h > maxH) { h = maxH; w = h / ratio; }
    return pointsFromGeo({ cx: centre.x, cy: centre.y, w: w, h: h }, mapW, mapH);
  }

  // reorder moves one picture a step forward or back among the pictures and
  // returns only the stacking numbers that must change. items are
  // { id, sort_order } in any order; ties keep their given order.
  function reorder(items, id, dir) {
    var list = items.map(function (it, i) { return { id: it.id, order: it.sort_order, seq: i }; });
    list.sort(function (a, b) { return a.order - b.order || a.seq - b.seq; });
    var i = list.findIndex(function (it) { return it.id === id; });
    var j = dir === 'forward' ? i + 1 : i - 1;
    if (i < 0 || j < 0 || j >= list.length) return [];
    var moved = list.splice(i, 1)[0];
    list.splice(j, 0, moved);
    var changes = [];
    list.forEach(function (it, n) {
      if (it.order !== n) changes.push({ id: it.id, sort_order: n });
    });
    return changes;
  }

  // ---- Browser-only part ----

  if (typeof window === 'undefined' || typeof document === 'undefined') {
    if (typeof module !== 'undefined' && module.exports) {
      module.exports = {
        clampOpacity: clampOpacity, clampTurn: clampTurn, normalizeCrop: normalizeCrop,
        clipPath: clipPath, boxFromPoints: boxFromPoints, geoFromBox: geoFromBox,
        pointsFromGeo: pointsFromGeo, rotate: rotate, scaleGeoAbout: scaleGeoAbout,
        cropDrag: cropDrag, resizeScale: resizeScale, placeNew: placeNew, reorder: reorder
      };
    }
    return;
  }

  var styleAdded = false;
  function addStyle() {
    if (styleAdded || document.getElementById('mp-pictures-style')) { styleAdded = true; return; }
    styleAdded = true;
    var s = document.createElement('style');
    s.id = 'mp-pictures-style';
    s.textContent = [
      '.mp-pic { position: absolute; left: 0; top: 0; pointer-events: none; }',
      '.mp-pic-in, .mp-pic-frame { position: absolute; inset: 0; }',
      '.mp-pic-in img { display: block; width: 100%; height: 100%; user-select: none; -webkit-user-drag: none; pointer-events: none; }',
      '.mp-pic-live .mp-pic-in { pointer-events: auto; cursor: pointer; }',
      '.mp-pic-sel .mp-pic-in { cursor: move; }',
      '.mp-pic-frame { pointer-events: none; }',
      '.mp-pic-box { position: absolute; box-sizing: border-box; }',
      '.mp-pic-hidden .mp-pic-box { outline: 2px dashed #f59e0b; outline-offset: -1px; }',
      '.mp-pic-sel .mp-pic-box { outline: 2px solid var(--mp-accent, #6366f1); outline-offset: -1px; }',
      '.mp-pic-missing .mp-pic-in { background: rgb(128 128 128 / 0.25); outline: 1px dashed rgb(128 128 128 / 0.8); }',
      '.mp-pic-h { position: absolute; width: 14px; height: 14px; box-sizing: border-box; border-radius: 3px; background: #fff; border: 2px solid var(--mp-accent, #6366f1); pointer-events: auto; touch-action: none; }',
      '.mp-pic-h-se { right: -7px; bottom: -7px; cursor: nwse-resize; border-radius: 50%; }',
      '.mp-pic-h-t { left: 50%; top: -7px; margin-left: -7px; cursor: ns-resize; }',
      '.mp-pic-h-b { left: 50%; bottom: -7px; margin-left: -7px; cursor: ns-resize; }',
      '.mp-pic-h-l { top: 50%; left: -7px; margin-top: -7px; cursor: ew-resize; }',
      '.mp-pic-h-r { top: 50%; right: -7px; margin-top: -7px; cursor: ew-resize; }',
      '.mp-picbar { left: 50%; bottom: 16px; transform: translateX(-50%); display: flex; flex-wrap: wrap; align-items: center; justify-content: center; gap: 4px 10px; padding: 6px 10px; max-width: calc(100% - 24px); font-size: 12px; }',
      '.mp-picbar[hidden] { display: none; }',
      '.mp-picbar label { display: flex; align-items: center; gap: 6px; color: var(--color-text-secondary); }',
      '.mp-picbar input[type=range] { width: 96px; accent-color: var(--mp-accent, #6366f1); }',
      '.mp-picbar button { border: 1px solid var(--color-border); background: var(--color-card-bg); border-radius: 7px; padding: 4px 9px; font-size: 12px; color: var(--color-text-body); }',
      '.mp-picbar button:hover { background: var(--color-bg-tertiary); }',
      '.mp-picbar button[aria-pressed="true"] { background: var(--mp-accent, #6366f1); border-color: var(--mp-accent, #6366f1); color: #fff; }',
      '.mp-picbar .mp-pic-del { color: #dc2626; }',
      '.mp-picbar .mp-pic-hex { background: var(--mp-accent, #6366f1); border-color: var(--mp-accent, #6366f1); color: #fff; }',
      '.mp-picbar .mp-pic-hex:hover { filter: brightness(1.08); background: var(--mp-accent, #6366f1); }',
      '.mp-picbar .mp-pic-hexline { color: var(--color-text-secondary); }',
      '.mp-picbar [hidden] { display: none !important; }',
      '.mp-picbar .mp-pic-sep { width: 1px; height: 18px; background: var(--color-border); }'
    ].join('\n');
    document.head.appendChild(s);
  }

  function el(tag, cls, attrs) {
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (attrs) Object.keys(attrs).forEach(function (k) { e.setAttribute(k, attrs[k]); });
    return e;
  }

  /**
   * attach prepares a viewer mount.
   *
   * opts: {
   *   mapW, mapH            map size in map units (the image's pixels),
   *   toLatLng({x,y})       percentages to a Leaflet LatLng,
   *   mediaURL(d)           URL of a picture's file: the signed image_url the
   *                         server sent for this viewer,
   *   refreshURL(d)         resolves a fresh signed URL (or '') once an image
   *                         fails to load, since signed URLs expire,
   *   canEdit               whether this viewer may select and edit,
   *   canDelete(d)          whether this viewer may delete picture d (the
   *                         server's rule: owners and DM access any, a
   *                         scribe the ones they added),
   *   onPatch(id, fields)   saves only the changed fields; resolves true/false,
   *   onDelete(id)          asks the host to confirm and delete,
   *   onChange(kind, id, points)  told as a picture changes: 'geo' while it is
   *                         being moved or resized (points are the live corners),
   *                         'patch' once an edit was applied or taken back,
   *   hexCover              the viewer's hex-cover controls (see map_viewer.js);
   *                         without it the picture bar offers no hex buttons
   * }
   * Returns { layer(drawing), selectMode(on), escape(), deleteSelected(),
   * selectedId(), select(id), placement(natW, natH), destroy() }.
   */
  function attach(map, opts) {
    addStyle();
    var mapW = opts.mapW, mapH = opts.mapH;
    var canEdit = !!opts.canEdit;
    var canDelete = typeof opts.canDelete === 'function' ? opts.canDelete : function () { return !!opts.canDelete; };
    var pane = map.getPane('mpPictures') || map.createPane('mpPictures');
    // Below the staff shadows (390) and the vector pane (400), so drawings and
    // shapes over a picture stay clickable and a shadow still covers it.
    pane.style.zIndex = 380;

    var layers = [];
    var seq = 0;
    var selected = null;
    var live = true;       // the move tool is active, so pictures may be picked
    var cropping = false;
    var destroyed = false;
    var bar = null;
    var barRefs = null;

    function emit(kind, id, points) {
      if (typeof opts.onChange === 'function') opts.onChange(kind, id, points);
    }

    // ---- stacking ----

    function restack() {
      layers.slice().sort(function (a, b) {
        return (a._d.sort_order || 0) - (b._d.sort_order || 0) || a._seq - b._seq;
      }).forEach(function (l, i) { if (l._outer) l._outer.style.zIndex = String(i); });
    }

    // ---- the picture bar ----

    function buildBar() {
      if (bar) return;
      var host = map.getContainer().parentNode;
      bar = el('div', 'mp-fl mp-picbar', { role: 'toolbar', 'aria-label': 'Picture' });
      bar.hidden = true;

      function slider(label, min, max, step, cls) {
        var wrap = el('label');
        wrap.appendChild(document.createTextNode(label));
        var input = el('input', cls, { type: 'range', min: min, max: max, step: step, 'aria-label': label });
        wrap.appendChild(input);
        bar.appendChild(wrap);
        return { input: input };
      }
      function button(label, cls) {
        var b = el('button', cls || '', { type: 'button' });
        b.textContent = label;
        bar.appendChild(b);
        return b;
      }
      function sep() { bar.appendChild(el('span', 'mp-pic-sep')); }

      // Hex controls come first, as in the approved design: either the way in
      // ("Turn into a hex map") or, for the picture that already carries the
      // hexes, what it is and the two things to do about it.
      var hexTurn = button('Turn into a hex map', 'mp-pic-hex');
      var hexLine = el('span', 'mp-pic-hexline');
      hexLine.textContent = 'This picture is a hex map. The hexes move and resize with it.';
      bar.appendChild(hexLine);
      var hexOpen = button('Open hexes');
      var hexOff = button('Take the hexes off');
      var see = slider('See-through', 20, 100, 5, '');
      var turn = slider('Turn', -MAX_TURN, MAX_TURN, 1, '');
      sep();
      var crop = button('Crop');
      crop.setAttribute('aria-pressed', 'false');
      var fwd = button('Forward');
      var back = button('Back');
      var hideWrap = el('label');
      var hide = el('input', '', { type: 'checkbox' });
      hideWrap.appendChild(hide);
      hideWrap.appendChild(document.createTextNode('Hide from players'));
      bar.appendChild(hideWrap);
      sep();
      // Shown per picture by syncBar, following the server's delete rule.
      var del = button('Delete', 'mp-pic-del');
      var done = button('', 'mp-pic-done');
      done.setAttribute('aria-label', 'Done');
      done.innerHTML = '<i class="fa-solid fa-xmark" aria-hidden="true"></i>';

      see.input.addEventListener('input', function () {
        if (!selected) return;
        selected._view.alpha = clampOpacity(see.input.value / 100);
        selected._style();
        syncBar();
      });
      see.input.addEventListener('change', function () {
        if (selected) patch(selected, { fill_alpha: clampOpacity(see.input.value / 100) });
      });
      turn.input.addEventListener('input', function () {
        if (!selected) return;
        selected._view.rot = clampTurn(turn.input.value);
        selected._style();
        syncBar();
      });
      turn.input.addEventListener('change', function () {
        if (selected) patch(selected, { rotation: clampTurn(turn.input.value) });
      });
      crop.addEventListener('click', function () {
        cropping = !cropping;
        if (selected) selected._handles();
        syncBar();
      });
      fwd.addEventListener('click', function () { step('forward'); });
      back.addEventListener('click', function () { step('back'); });
      hide.addEventListener('change', function () {
        if (!selected) return;
        patch(selected, { visibility: hide.checked ? 'dm_only' : 'everyone' });
      });
      del.addEventListener('click', function () { if (selected && canDelete(selected._d)) opts.onDelete(selected._d.id); });
      done.addEventListener('click', function () { cropping = false; deselect(); });

      host.appendChild(bar);
      hexTurn.addEventListener('click', function () {
        if (selected && opts.hexCover) opts.hexCover.turnInto(selected._d.id);
      });
      hexOpen.addEventListener('click', function () { if (opts.hexCover) opts.hexCover.open(); });
      hexOff.addEventListener('click', function () { if (opts.hexCover) opts.hexCover.takeOff(); });
      if (opts.hexCover) opts.hexCover.onChange(syncBar);
      barRefs = { see: see, turn: turn, crop: crop, hide: hide, del: del, hexTurn: hexTurn, hexLine: hexLine, hexOpen: hexOpen, hexOff: hexOff };
    }

    function syncBar() {
      if (!bar) return;
      bar.hidden = !selected;
      if (!selected) return;
      var v = selected._view;
      barRefs.see.input.value = String(Math.round(v.alpha * 100));
      barRefs.turn.input.value = String(Math.round(v.rot));
      barRefs.crop.setAttribute('aria-pressed', cropping ? 'true' : 'false');
      barRefs.crop.textContent = cropping ? 'Finish crop' : 'Crop';
      barRefs.hide.checked = selected._d.visibility === 'dm_only';
      barRefs.del.hidden = !canDelete(selected._d);
      // Turning and cropping would move the picture away from the hexes laid
      // on it, so a picture that carries hexes offers neither.
      var hc = opts.hexCover;
      var carries = !!(hc && hc.carries(selected._d.id));
      barRefs.hexTurn.hidden = !(hc && !carries && hc.canTurn());
      barRefs.hexLine.hidden = barRefs.hexOpen.hidden = !carries;
      barRefs.hexOff.hidden = !(carries && hc.canTakeOff());
      barRefs.turn.input.parentNode.hidden = carries;
      barRefs.crop.hidden = carries;
    }

    function step(dir) {
      if (!selected) return;
      var changes = reorder(layers.map(function (l) {
        return { id: l._d.id, sort_order: l._d.sort_order || 0 };
      }), selected._d.id, dir);
      changes.forEach(function (c) {
        var l = byID(c.id);
        if (l) patch(l, { sort_order: c.sort_order });
      });
    }

    function byID(id) {
      for (var i = 0; i < layers.length; i++) if (layers[i]._d.id === id) return layers[i];
      return null;
    }

    // ---- saving ----

    // patch applies the change at once and sends ONLY the changed fields; if
    // the server refuses, the picture goes back to what it was.
    function patch(layer, fields) {
      var before = {};
      Object.keys(fields).forEach(function (k) { before[k] = layer._d[k]; layer._d[k] = fields[k]; });
      layer._read();
      layer._refresh();
      restack();
      syncBar();
      emit('patch', layer._d.id);
      Promise.resolve(opts.onPatch(layer._d.id, fields)).then(function (ok) {
        if (ok) return;
        Object.keys(before).forEach(function (k) { layer._d[k] = before[k]; });
        layer._read();
        layer._refresh();
        restack();
        syncBar();
        emit('patch', layer._d.id);
      });
    }

    // ---- selection ----

    function select(layer) {
      if (selected === layer) return;
      var prev = selected;
      selected = layer;
      cropping = false;
      if (prev) prev._select(false);
      if (layer) { buildBar(); layer._select(true); }
      syncBar();
    }

    function deselect() { var was = !!selected; select(null); return was; }

    // ---- dragging ----

    // drag follows the pointer until it is released. Mouse and touch both
    // route through here; the handlers are removed on release.
    function drag(startEvent, onMove, onEnd) {
      var moved = false;
      var sx = pointOf(startEvent).x, sy = pointOf(startEvent).y;
      function move(e) {
        var p = pointOf(e);
        var dx = p.x - sx, dy = p.y - sy;
        if (!moved && Math.abs(dx) + Math.abs(dy) < 3) return;
        moved = true;
        if (e.cancelable) e.preventDefault();
        onMove(dx, dy);
      }
      function up() {
        window.removeEventListener('mousemove', move);
        window.removeEventListener('mouseup', up);
        window.removeEventListener('touchmove', move);
        window.removeEventListener('touchend', up);
        window.removeEventListener('touchcancel', up);
        onEnd(moved);
      }
      window.addEventListener('mousemove', move);
      window.addEventListener('mouseup', up);
      window.addEventListener('touchmove', move, { passive: false });
      window.addEventListener('touchend', up);
      window.addEventListener('touchcancel', up);
    }

    function pointOf(e) {
      var t = e.touches && e.touches[0] ? e.touches[0] : (e.changedTouches && e.changedTouches[0]) || e;
      return { x: t.clientX, y: t.clientY };
    }

    // ---- the layer ----

    var PictureLayer = L.Layer.extend({
      options: { pane: 'mpPictures' },

      initialize: function (d) {
        this._d = d;
        this._seq = seq++;
        this._sel = false;
        this._read();
      },

      // reload re-reads the stored drawing after something outside this module
      // changed it (the server straightened it when hexes were pinned to it).
      reload: function () {
        this._read();
        this._refresh();
        restack();
        syncBar();
      },

      // _read copies what is stored into the working view that the live
      // sliders and drags change before anything is saved.
      _read: function () {
        var box = boxFromPoints(this._d.points);
        this._geo = box ? geoFromBox(box, mapW, mapH) : null;
        this._view = {
          alpha: clampOpacity(this._d.fill_alpha),
          rot: Number(this._d.rotation) || 0,
          crop: normalizeCrop(this._d.crop)
        };
      },

      onAdd: function () {
        if (!this._geo) return;
        this._build();
        this.getPane().appendChild(this._outer);
        layers.push(this);
        this._refresh();
        restack();
      },

      onRemove: function () {
        var i = layers.indexOf(this);
        if (i !== -1) layers.splice(i, 1);
        if (selected === this) deselect();
        if (this._outer && this._outer.parentNode) this._outer.parentNode.removeChild(this._outer);
        this._outer = null;
      },

      getEvents: function () {
        var ev = { zoom: this._place, viewreset: this._place };
        if (this._zoomAnimated) ev.zoomanim = this._animateZoom;
        return ev;
      },

      _build: function () {
        var self = this;
        var outer = el('div', 'mp-pic' + (this._zoomAnimated ? ' leaflet-zoom-animated' : ''));
        var inner = el('div', 'mp-pic-in');
        var img = el('img', '', { alt: '', draggable: 'false' });
        // A signed URL lasts about an hour. When the file will not load, ask
        // once for a fresh one before showing the picture as missing.
        var retried = false;
        img.addEventListener('error', function () {
          if (retried || !opts.refreshURL) { outer.classList.add('mp-pic-missing'); return; }
          retried = true;
          Promise.resolve(opts.refreshURL(self._d)).then(function (url) {
            if (url && self._outer === outer) { self._d.image_url = url; img.src = url; }
            else outer.classList.add('mp-pic-missing');
          }, function () { outer.classList.add('mp-pic-missing'); });
        });
        img.addEventListener('load', function () { outer.classList.remove('mp-pic-missing'); });
        img.src = opts.mediaURL(this._d);
        inner.appendChild(img);
        var frame = el('div', 'mp-pic-frame');
        var box = el('div', 'mp-pic-box');
        frame.appendChild(box);
        outer.appendChild(inner);
        outer.appendChild(frame);
        this._outer = outer; this._inner = inner; this._frame = frame; this._box = box;

        if (canEdit) {
          // An unselected picture lets the map pan under a drag and only a
          // clean click picks it; a selected one is dragged instead.
          var downAt = null;
          L.DomEvent.on(inner, 'mousedown touchstart', function (e) {
            if (!live) return;
            var p = pointOf(e);
            downAt = { x: p.x, y: p.y };
            if (selected === self) {
              L.DomEvent.stopPropagation(e);
              self._startMove(e);
            }
          });
          L.DomEvent.on(inner, 'click', function (e) {
            if (!live || !downAt) return;
            var p = { x: e.clientX, y: e.clientY };
            var near = Math.abs(p.x - downAt.x) + Math.abs(p.y - downAt.y) < 5;
            downAt = null;
            if (near && !self._justDragged) select(self);
            self._justDragged = false;
          });
        }
      },

      // _px is how many screen pixels one map unit currently takes.
      _px: function () {
        return this._geo.w > 0 ? this._outer.offsetWidth / this._geo.w : 1;
      },

      _place: function () {
        if (!this._outer || !this._geo) return;
        var pts = pointsFromGeo(this._geo, mapW, mapH);
        var tl = this._map.latLngToLayerPoint(opts.toLatLng(pts[0]));
        var br = this._map.latLngToLayerPoint(opts.toLatLng(pts[1]));
        this._tl = opts.toLatLng(pts[0]);
        this._outer.style.width = Math.max(1, br.x - tl.x) + 'px';
        this._outer.style.height = Math.max(1, br.y - tl.y) + 'px';
        L.DomUtil.setPosition(this._outer, tl);
      },

      _animateZoom: function (e) {
        if (!this._outer || !this._tl) return;
        var scale = this._map.getZoomScale(e.zoom);
        var tl = this._map._latLngToNewLayerPoint(this._tl, e.zoom, e.center);
        L.DomUtil.setTransform(this._outer, tl, scale);
      },

      // _style writes opacity, turn and crop from the working view.
      _style: function () {
        if (!this._outer) return;
        var v = this._view;
        var turn = 'rotate(' + v.rot + 'deg)';
        this._inner.style.transform = turn;
        this._frame.style.transform = turn;
        this._inner.style.opacity = String(v.alpha);
        this._inner.style.clipPath = clipPath(v.crop);
        var c = v.crop;
        this._box.style.left = c.l + '%';
        this._box.style.right = c.r + '%';
        this._box.style.top = c.t + '%';
        this._box.style.bottom = c.b + '%';
      },

      _refresh: function () {
        if (!this._outer) return;
        this._place();
        this._style();
        var o = this._outer.classList;
        o.toggle('mp-pic-hidden', canEdit && this._d.visibility === 'dm_only');
        o.toggle('mp-pic-live', canEdit && live);
        this._handles();
      },

      _select: function (on) {
        this._sel = on;
        if (!this._outer) return;
        this._outer.classList.toggle('mp-pic-sel', on);
        this._handles();
      },

      // _handles draws the corner handle (resize) or the four edge handles
      // (crop) on the visible part of a selected picture.
      _handles: function () {
        if (!this._box) return;
        while (this._box.firstChild) this._box.removeChild(this._box.firstChild);
        if (!this._sel || !canEdit) return;
        var self = this;
        if (cropping) {
          ['t', 'r', 'b', 'l'].forEach(function (side) {
            var h = el('div', 'mp-pic-h mp-pic-h-' + side, { 'aria-label': 'Trim edge' });
            L.DomEvent.on(h, 'mousedown touchstart', function (e) {
              L.DomEvent.stopPropagation(e);
              L.DomEvent.preventDefault(e);
              self._startCrop(e, side);
            });
            self._box.appendChild(h);
          });
        } else {
          var h = el('div', 'mp-pic-h mp-pic-h-se', { 'aria-label': 'Resize' });
          L.DomEvent.on(h, 'mousedown touchstart', function (e) {
            L.DomEvent.stopPropagation(e);
            L.DomEvent.preventDefault(e);
            self._startResize(e);
          });
          this._box.appendChild(h);
        }
      },

      _startMove: function (startEvent) {
        var self = this;
        var g0 = { cx: this._geo.cx, cy: this._geo.cy, w: this._geo.w, h: this._geo.h };
        var px = this._px();
        drag(startEvent, function (dx, dy) {
          self._geo = { cx: g0.cx + dx / px, cy: g0.cy + dy / px, w: g0.w, h: g0.h };
          self._place();
          emit('geo', self._d.id, pointsFromGeo(self._geo, mapW, mapH));
        }, function (moved) {
          if (!moved) return;
          self._justDragged = true;
          patch(self, { points: pointsFromGeo(self._geo, mapW, mapH) });
        });
      },

      _startResize: function (startEvent) {
        var self = this;
        var g0 = { cx: this._geo.cx, cy: this._geo.cy, w: this._geo.w, h: this._geo.h };
        var c = this._view.crop, rot = this._view.rot;
        var px = this._px();
        var visW = g0.w * (1 - (c.l + c.r) / 100);
        var visH = g0.h * (1 - (c.t + c.b) / 100);
        // The visible top-left corner stays put; the rest grows from it.
        var rx = g0.w * c.l / 100 - g0.w / 2;
        var ry = g0.h * c.t / 100 - g0.h / 2;
        drag(startEvent, function (dx, dy) {
          var local = rotate(dx / px, dy / px, -rot);
          var s = resizeScale(visW, visH, local.x, local.y, g0, mapW);
          self._geo = scaleGeoAbout(g0, rot, rx, ry, s);
          self._place();
          emit('geo', self._d.id, pointsFromGeo(self._geo, mapW, mapH));
        }, function (moved) {
          if (!moved) return;
          patch(self, { points: pointsFromGeo(self._geo, mapW, mapH) });
        });
      },

      _startCrop: function (startEvent, side) {
        var self = this;
        var crop0 = this._view.crop, rot = this._view.rot;
        var px = this._px();
        var w = this._geo.w, h = this._geo.h;
        drag(startEvent, function (dx, dy) {
          var local = rotate(dx / px, dy / px, -rot);
          self._view.crop = cropDrag(side, crop0, local.x, local.y, w, h);
          self._style();
        }, function (moved) {
          if (!moved) return;
          var c = self._view.crop;
          // An untrimmed picture is stored as no crop at all.
          patch(self, { crop: cropIsEmpty(c) ? null : c });
        });
      }
    });

    // A click on the empty map drops the selection; a click on a picture or
    // the bar does not.
    function onMapClick(e) {
      var t = e.originalEvent && e.originalEvent.target;
      if (t && t.closest && t.closest('.mp-pic, .mp-picbar')) return;
      deselect();
    }
    map.on('click', onMapClick);

    return {
      layer: function (d) { return new PictureLayer(d); },
      // selectMode(false) is called while another tool is active: pictures
      // then let every click through and nothing stays selected.
      selectMode: function (on) {
        live = !!on;
        if (!live) deselect();
        layers.forEach(function (l) {
          if (l._outer) l._outer.classList.toggle('mp-pic-live', canEdit && live);
        });
      },
      // escape backs out of cropping first, then drops the selection.
      escape: function () {
        if (cropping) { cropping = false; if (selected) selected._handles(); syncBar(); return true; }
        return deselect();
      },
      deleteSelected: function () {
        if (!selected || !canDelete(selected._d)) return false;
        opts.onDelete(selected._d.id);
        return true;
      },
      selectedId: function () { return selected ? selected._d.id : null; },
      select: function (id) { var l = byID(id); if (l) select(l); },
      nextSortOrder: function () {
        var max = -1;
        layers.forEach(function (l) { max = Math.max(max, l._d.sort_order || 0); });
        return max + 1;
      },
      // placement returns the two corners for a new picture, centred on the
      // current view.
      placement: function (natW, natH) {
        var b = map.getBounds();
        var c = map.getCenter();
        var viewW = Math.abs(b.getEast() - b.getWest());
        var viewH = Math.abs(b.getNorth() - b.getSouth());
        return placeNew(natW, natH, viewW, viewH, { x: c.lng, y: mapH - c.lat }, mapW, mapH);
      },
      destroy: function () {
        if (destroyed) return;
        destroyed = true;
        map.off('click', onMapClick);
        if (bar && bar.parentNode) bar.parentNode.removeChild(bar);
        bar = null;
      }
    };
  }

  window.ChronicleMapPictures = {
    attach: attach,
    clampOpacity: clampOpacity,
    clipPath: clipPath,
    reorder: reorder
  };
})();
