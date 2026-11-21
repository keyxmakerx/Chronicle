/**
 * hex_map_preview.js -- the picture of a hex map on the AI import's review
 * (ai_workspace/import_review_records.templ, recordHexMap).
 *
 * Mount: <details data-widget="hex-map-preview" data-map-json data-name> with a
 * <summary> holding "See the map" and "Hide the map" and an empty
 * [data-map-body]. data-map-json is the hex-map row's picture data
 * (records/hex_map_preview.go). It is parsed and drawn only when the picture is
 * first asked for, never at page load.
 *
 * - Hover or keyboard focus on the word for 250 ms Peeks the picture as a
 *   floating card (fade and a 4px rise; Calm fades only, Off appears). It sits
 *   inside the side panel, flips above when there is no room below, and stays
 *   clear of the save bar. It leaves with the pointer (from the word and the
 *   card) or the focus. Touch has no hover, so it never peeks.
 * - Click, Enter or tap opens the picture in the row (the details fold). On an
 *   update a Before / After switch flips between the stored map and the result;
 *   the rings round hexes that would change stay in both.
 *
 * The picture is one canvas drawn with the map page's own parts: the terrain
 * art (map_hex_art.js) and the lines, flat colours and Simple icons
 * (map_hex_look.js). No Leaflet, no map page. A map whose hexes would be
 * under 8 pixels is drawn in flat colours, with no art and no names, so a
 * 100 by 100 map still draws at once.
 */
(function () {
  'use strict';

  var NS = 'http://www.w3.org/2000/svg';
  var SQRT3 = Math.sqrt(3);
  // Master canvas pixels per CSS pixel, so a one-pixel line stays crisp on a
  // dense screen and the picture is the same whatever the screen.
  var SCALE = 2;
  var MAX_W = 1800;
  // The hex radius, in master pixels, below which art is not worth drawing, and
  // above which place names are.
  var FLAT_R = 8;
  var NAME_R = 12;
  var HOVER_MS = 250;
  var LEAVE_MS = 150;
  var PEEK_GAP = 6;
  var PEEK_PAD = 8;
  var PEEK_MARGIN = 12;
  var PEEK_MAX_W = 520;
  var PEEK_MIN_W = 180;
  // The radius Detailed and Simple tiles are drawn at before they are scaled.
  var TILE_R = 26;

  // ---- Pure ----

  // parse reads the row's picture data: cells as { col, row, terrain, road,
  // river, trail, name } by "col,row", for Before (what is stored) and After
  // (stored with the block's hexes laid over it). A new map has only After.
  function parse(text) {
    var d;
    try { d = JSON.parse(text); } catch (e) { return null; }
    if (!d || !(d.cols > 0) || !(d.rows > 0) || !Array.isArray(d.cells)) return null;
    var lands = d.lands || [];
    function cell(a) {
      return { col: a[0], row: a[1], terrain: a[2] >= 0 ? lands[a[2]] || '' : '', road: a[3], river: a[4], trail: a[5], name: a[6] || '' };
    }
    var before = {}, after = before;
    d.cells.forEach(function (a) { before[a[0] + ',' + a[1]] = cell(a); });
    if (d.update) {
      after = {};
      Object.keys(before).forEach(function (k) { after[k] = before[k]; });
      (d.after || []).forEach(function (a) { after[a[0] + ',' + a[1]] = cell(a); });
    }
    return { cols: d.cols, rows: d.rows, art: d.art || 'detailed', update: !!d.update, before: before, after: after, ring: d.ring || [] };
  }

  // layout is the field in master pixels for a picture targetW wide: hex (0,0)
  // sits at the top-left corner and the field is as wide as its widest row.
  function layout(cols, rows, targetW) {
    var r = targetW / ((cols + 0.5) * SQRT3), w = SQRT3 * r;
    return {
      r: r, w: w, ox: w / 2, oy: r, cols: cols, rows: rows,
      width: Math.ceil(cols * w + w / 2), height: Math.ceil((rows - 1) * 1.5 * r + 2 * r)
    };
  }

  // lookFor is how the land is drawn: the map's own style, Realistic stepping
  // down to Detailed because its tiles are costly to paint, and flat colours
  // when hexes are too small for art.
  function lookFor(art, r) {
    if (r < FLAT_R) return 'flat';
    return art === 'simple' ? 'simple' : 'detailed';
  }

  // inField keeps the cells that lie inside the field.
  function inField(cells, cols, rows) {
    var out = {};
    Object.keys(cells).forEach(function (k) {
      var c = cells[k];
      if (c.col >= 0 && c.row >= 0 && c.col < cols && c.row < rows) out[k] = c;
    });
    return out;
  }

  // peekPlace puts the Peek card: below the word, or above when below has less
  // room, inside bounds and clear of the save bar (barTop, or null). The card is
  // want.w wide at most; when neither side has room for its height it narrows,
  // the picture keeping its proportions (aspect is height over width, pad the
  // card's padding).
  function peekPlace(anchor, bounds, barTop, want, gap, margin) {
    var bottom = Math.min(bounds.bottom, barTop == null ? Infinity : barTop) - margin;
    var below = bottom - (anchor.bottom + gap);
    var above = (anchor.top - gap) - (bounds.top + margin);
    var w = Math.min(want.w, bounds.right - bounds.left - 2 * margin);
    var h = (w - 2 * want.pad) * want.aspect + 2 * want.pad;
    var up = h > below && above > below;
    var room = up ? above : below;
    if (h > room) {
      w = Math.max(want.min, Math.min(w, (room - 2 * want.pad) / want.aspect + 2 * want.pad));
      h = (w - 2 * want.pad) * want.aspect + 2 * want.pad;
    }
    var left = Math.max(bounds.left + margin, Math.min(anchor.left, bounds.right - margin - w));
    return { left: left, top: up ? anchor.top - gap - h : anchor.bottom + gap, w: w, h: h, above: up };
  }

  var api = { parse: parse, layout: layout, lookFor: lookFor, inField: inField, peekPlace: peekPlace, FLAT_R: FLAT_R, NAME_R: NAME_R, SCALE: SCALE };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  if (typeof window === 'undefined') return;
  window.ChronicleHexPreview = api;

  // ---- Drawing ----

  var L = window.ChronicleHexLook;
  var HA = window.ChronicleHexArt;

  function tokenRGB(name, fallback) {
    var v = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
    return v || fallback;
  }

  function svgImage(svg, w, h) {
    return new Promise(function (resolve) {
      var img = new Image();
      img.onload = function () { resolve(img); };
      img.onerror = function () { resolve(null); };
      img.src = 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(
        '<svg xmlns="' + NS + '" width="' + w + '" height="' + h + '" viewBox="0 0 ' + w + ' ' + h + '">' + svg + '</svg>');
    });
  }

  // Picture draws one map, once per view (Before or After), onto a master
  // canvas that the row and the Peek card both copy from. Tiles are painted
  // once per kind of land and variant and stamped on every hex that shares them.
  function Picture(data, cssWidth) {
    this.data = data;
    this.geo = layout(data.cols, data.rows, Math.min(MAX_W, Math.round(cssWidth * SCALE)));
    this.look = L ? lookFor(data.art, this.geo.r) : 'none';
    if (this.look === 'detailed' && !HA) this.look = 'simple';
    this.tiles = {};
    this.masters = {};
    var bg = tokenRGB('--color-bg-secondary', '#f3f4f6');
    this.colors = {
      bg: bg, grid: tokenRGB('--color-border', '#d1d5db'),
      bad: 'rgb(' + tokenRGB('--color-bad-rgb', '220 38 38') + ')'
    };
  }

  Picture.prototype.tile = function (t, v) {
    var key = this.look + '|' + t + '|' + v;
    if (this.tiles[key] !== undefined) return this.tiles[key];
    var P = this.geo.r, w = Math.ceil(2 * P), h = Math.ceil(2.35 * P);
    var art = this.look === 'simple'
      ? L.simpleArt(t, 0, 0, TILE_R)
      : '<defs>' + HA.detailedDefs('t') + '</defs>' + HA.detailedArt(t, 0, 0, TILE_R, HA.detailedSeed(t, v), 't');
    var svg = '<svg xmlns="' + NS + '" width="' + w + '" height="' + h + '" viewBox="' + (-TILE_R) + ' ' + (-1.25 * TILE_R) + ' ' + (2 * TILE_R) + ' ' + (2.35 * TILE_R) + '">' + art + '</svg>';
    var img = new Image();
    this.tiles[key] = new Promise(function (resolve) {
      img.onload = function () { resolve(img); };
      img.onerror = function () { resolve(null); };
    });
    img.src = 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(svg);
    return this.tiles[key];
  };

  // render is the master canvas of one view; Promise<canvas>.
  Picture.prototype.render = function (which) {
    if (this.masters[which]) return this.masters[which];
    var self = this, g = this.geo, d = this.data, look = this.look;
    var cells = inField(which === 'before' ? d.before : d.after, g.cols, g.rows);
    var cv = document.createElement('canvas');
    cv.width = g.width; cv.height = g.height;
    var cx = cv.getContext('2d');
    cx.fillStyle = this.colors.bg;
    cx.fillRect(0, 0, g.width, g.height);

    // The hex outlines of the whole field, one path.
    var grid = '', row, col, c;
    for (row = 0; row < g.rows; row++) {
      for (col = 0; col < g.cols; col++) { c = L.center(g, col, row); grid += L.hexD(c[0], c[1], g.r); }
    }
    cx.strokeStyle = this.colors.grid;
    cx.lineWidth = Math.max(1, g.r * 0.05);
    cx.stroke(new Path2D(grid));

    var list = Object.keys(cells).map(function (k) { return cells[k]; }).filter(function (x) { return x.terrain && x.terrain !== 'road'; });
    list.sort(function (a, b) { return a.row - b.row || a.col - b.col; });

    var land;
    if (look === 'flat' || look === 'none') {
      var byLand = {};
      list.forEach(function (x) { var p = L.center(g, x.col, x.row); byLand[x.terrain] = (byLand[x.terrain] || '') + L.hexD(p[0], p[1], g.r); });
      Object.keys(byLand).forEach(function (t) {
        cx.fillStyle = (L && L.COLOR[t]) || '#999';
        cx.globalAlpha = 0.85;
        cx.fill(new Path2D(byLand[t]));
      });
      cx.globalAlpha = 1;
      // The fills hide the outlines, so the grid goes back over them, faintly.
      cx.strokeStyle = 'rgba(0,0,0,0.12)';
      cx.stroke(new Path2D(grid));
      land = Promise.resolve();
    } else {
      var pending = list.map(function (x) {
        var v = look === 'simple' ? 0 : HA.detailedVariant(x.col + ',' + x.row);
        return self.tile(x.terrain, v).then(function (img) { return { x: x, img: img }; });
      });
      land = Promise.all(pending).then(function (done) {
        done.forEach(function (it) {
          var p = L.center(g, it.x.col, it.x.row);
          if (it.img) cx.drawImage(it.img, p[0] - g.r, p[1] - 1.25 * g.r, 2 * g.r, 2.35 * g.r);
          else { cx.fillStyle = L.COLOR[it.x.terrain] || '#999'; cx.globalAlpha = 0.6; cx.fill(new Path2D(L.hexD(p[0], p[1], g.r))); cx.globalAlpha = 1; }
        });
      });
    }

    // Lines, names and rings go on top as one picture of their own, drawn by the
    // map page's own line code.
    var kk = g.r / TILE_R, over = '';
    if (L) over += L.linesMarkup(g, cells, look === 'detailed' ? 'detailed' : 'simple', kk);
    if (g.r >= NAME_R) {
      var fs = Math.max(10, g.r * 0.75);
      Object.keys(cells).forEach(function (k) {
        var n = cells[k];
        if (!n.name) return;
        var p = L.center(g, n.col, n.row);
        over += '<text x="' + p[0].toFixed(1) + '" y="' + (p[1] + g.r * 0.8).toFixed(1) + '" text-anchor="middle" font-family="Georgia,serif" font-style="italic" font-weight="600" font-size="' +
          fs.toFixed(1) + '" fill="#2b2112" stroke="#f4ead0" stroke-width="' + (fs * 0.28).toFixed(1) + '" stroke-linejoin="round" paint-order="stroke">' + Chronicle.escapeHtml(n.name) + '</text>';
      });
    }
    if (d.ring.length && L) {
      var ring = '';
      d.ring.forEach(function (h) { var p = L.center(g, h[0], h[1]); ring += L.hexD(p[0], p[1], g.r * 0.9); });
      var sw = Math.max(2.4, g.r * 0.17);
      over += '<path d="' + ring + '" fill="none" stroke="#ffffff" stroke-opacity=".85" stroke-width="' + (sw * 2).toFixed(1) + '" stroke-linejoin="round"/>' +
        '<path d="' + ring + '" fill="none" stroke="' + this.colors.bad + '" stroke-width="' + sw.toFixed(1) + '" stroke-linejoin="round" stroke-dasharray="' +
        (sw * 2.6).toFixed(1) + ' ' + (sw * 1.8).toFixed(1) + '"/>';
    }
    this.masters[which] = land.then(function () { return over ? svgImage(over, g.width, g.height) : null; }).then(function (img) {
      if (img) cx.drawImage(img, 0, 0);
      return cv;
    });
    return this.masters[which];
  };

  // copyTo shows a master on a visible canvas.
  function copyTo(canvas, master) {
    canvas.width = master.width;
    canvas.height = master.height;
    canvas.getContext('2d').drawImage(master, 0, 0);
  }

  // ---- The row ----

  function motionOff() { return document.documentElement.getAttribute('data-motion') === 'off'; }

  function make(tag, cls, text) {
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text) e.textContent = text;
    return e;
  }

  function mount(el) {
    var summary = el.querySelector('summary'), body = el.querySelector('[data-map-body]');
    if (!summary || !body) return null;
    var name = el.getAttribute('data-name') || 'the map';
    var data = null, pic = null, drawing = null, failed = false;
    var view = 'after', card = null, cardCanvas = null;
    var hoverT = 0, leaveT = 0, hideT = 0;
    var openCanvas = null, status = null, seg = null;

    // ready draws the After picture the first time it is wanted; Promise<canvas>.
    function ready() {
      if (drawing) return drawing;
      data = parse(el.getAttribute('data-map-json') || '');
      if (!data || !L) { failed = true; return (drawing = Promise.resolve(null)); }
      var width = (summary.closest('.air-main') || el).clientWidth || 440;
      pic = new Picture(data, width);
      drawing = pic.render('after').catch(function () { failed = true; return null; });
      return drawing;
    }

    function label() {
      return view === 'before' ? 'The map as it is now, ' + name : 'The map as it will look, ' + name;
    }

    // ---- Open in the row ----

    function build() {
      if (status) return;
      status = make('p', 'air-line', 'Drawing the map…');
      status.setAttribute('role', 'status');
      body.appendChild(status);
      ready().then(function (master) {
        if (!master) { status.textContent = 'The picture of this map could not be drawn.'; return; }
        status.remove();
        if (data.update) body.appendChild(buildSwitch());
        openCanvas = make('canvas', 'air-map-cv');
        openCanvas.setAttribute('role', 'img');
        openCanvas.setAttribute('aria-label', label());
        copyTo(openCanvas, master);
        body.appendChild(openCanvas);
        if (data.update) body.appendChild(make('p', 'air-small air-map-note', 'Before is the map now; After is the map with these changes. Hexes that would change are ringed in both.'));
        requestAnimationFrame(function () { openCanvas.classList.add('is-ready'); });
      });
    }

    function buildSwitch() {
      seg = make('div', 'air-seg');
      seg.setAttribute('role', 'radiogroup');
      seg.setAttribute('aria-label', 'Show the map');
      [['before', 'Before'], ['after', 'After']].forEach(function (o) {
        var b = make('button', '', o[1]);
        b.type = 'button';
        b.setAttribute('role', 'radio');
        b.setAttribute('data-view', o[0]);
        b.addEventListener('click', function () { choose(o[0], false); });
        b.addEventListener('keydown', function (ev) {
          if (ev.key !== 'ArrowLeft' && ev.key !== 'ArrowRight') return;
          ev.preventDefault();
          choose(ev.key === 'ArrowLeft' ? 'before' : 'after', true);
        });
        seg.appendChild(b);
      });
      sync();
      return seg;
    }

    function sync() {
      if (!seg) return;
      Array.prototype.forEach.call(seg.children, function (b) {
        var on = b.getAttribute('data-view') === view;
        b.setAttribute('aria-checked', on ? 'true' : 'false');
        b.tabIndex = on ? 0 : -1;
      });
    }

    function choose(which, focus) {
      if (which === view || !pic) return;
      view = which;
      sync();
      if (focus) { var b = seg.querySelector('[data-view="' + which + '"]'); if (b) b.focus(); }
      pic.render(which).then(function (master) {
        if (view !== which || !openCanvas) return;
        copyTo(openCanvas, master);
        openCanvas.setAttribute('aria-label', label());
      });
    }

    // ---- The Peek ----

    function cancelTimers() { clearTimeout(hoverT); clearTimeout(leaveT); }

    function closePeek(now) {
      cancelTimers();
      if (!card || !card.classList.contains('is-in')) return;
      document.removeEventListener('keydown', onEsc, true);
      card.classList.remove('is-in');
      clearTimeout(hideT);
      var c = card;
      hideT = setTimeout(function () { c.hidden = true; }, now || motionOff() ? 0 : 140);
      if (now || motionOff()) c.hidden = true;
    }

    function onEsc(ev) {
      if (ev.key !== 'Escape') return;
      ev.stopPropagation();
      closePeek(true);
    }

    function boundsOf() {
      var panel = el.closest('.ai-panel');
      if (panel) { var r = panel.getBoundingClientRect(); return { left: r.left, right: Math.min(r.right, window.innerWidth), top: Math.max(0, r.top), bottom: Math.min(r.bottom, window.innerHeight) }; }
      return { left: 0, right: window.innerWidth, top: 0, bottom: window.innerHeight };
    }

    function barTopOf() {
      var form = el.closest('form') || el.closest('.air');
      var bar = form && form.querySelector('.air-savebar');
      if (!bar) return null;
      var r = bar.getBoundingClientRect();
      return r.top < window.innerHeight && r.bottom > 0 ? r.top : null;
    }

    function openPeek() {
      if (el.open) return;
      ready().then(function (master) {
        if (!master || el.open) return;
        var wanted = summary.matches(':hover, :focus-visible') || (card && card.matches(':hover'));
        if (!wanted) return;
        if (!card) {
          card = make('div', 'air-mapcard');
          card.setAttribute('aria-hidden', 'true');
          cardCanvas = make('canvas', 'air-mapcard-cv');
          card.appendChild(cardCanvas);
          card.hidden = true;
          card.addEventListener('pointerenter', function () { clearTimeout(leaveT); });
          card.addEventListener('pointerleave', function () { leaveSoon(); });
          (summary.closest('.air-main') || el.parentNode).appendChild(card);
        }
        // The Peek is always the After picture.
        pic.render('after').then(function (m) {
          copyTo(cardCanvas, m);
          clearTimeout(hideT);
          card.hidden = false;
          card.classList.remove('is-in');
          var want = { w: Math.min(PEEK_MAX_W, (summary.closest('.air-main') || el).clientWidth || PEEK_MAX_W), aspect: m.height / m.width, pad: PEEK_PAD, min: PEEK_MIN_W };
          var place = peekPlace(summary.getBoundingClientRect(), boundsOf(), barTopOf(), want, PEEK_GAP, PEEK_MARGIN);
          card.style.width = place.w.toFixed(0) + 'px';
          card.style.left = place.left.toFixed(0) + 'px';
          card.style.top = place.top.toFixed(0) + 'px';
          // A parent that moves fixed children (a transform) would put the card
          // off its spot; measure and take the difference back. The Peek's own
          // 4px rise is smaller than the tolerance.
          var got = card.getBoundingClientRect();
          if (Math.abs(got.left - place.left) > 8 || Math.abs(got.top - place.top) > 8) {
            card.style.left = (place.left - (got.left - place.left)).toFixed(0) + 'px';
            card.style.top = (place.top - (got.top - place.top)).toFixed(0) + 'px';
          }
          card.classList.toggle('is-above', place.above);
          document.addEventListener('keydown', onEsc, true);
          if (motionOff()) card.classList.add('is-in');
          else requestAnimationFrame(function () { requestAnimationFrame(function () { card.classList.add('is-in'); }); });
        });
      });
    }

    function leaveSoon() {
      clearTimeout(leaveT);
      leaveT = setTimeout(function () { closePeek(false); }, LEAVE_MS);
    }

    function soon() {
      if (el.open) return;
      clearTimeout(leaveT);
      clearTimeout(hoverT);
      hoverT = setTimeout(openPeek, HOVER_MS);
    }

    var onEnter = function (ev) { if (ev.pointerType !== 'touch') soon(); };
    var onLeave = function (ev) { if (ev.pointerType !== 'touch') { clearTimeout(hoverT); leaveSoon(); } };
    var onFocus = function () { if (summary.matches(':focus-visible')) soon(); };
    var onBlur = function () { clearTimeout(hoverT); leaveSoon(); };
    var onClick = function () { closePeek(true); };
    var onToggle = function () {
      closePeek(true);
      if (el.open) build();
    };
    var onScroll = function () { closePeek(true); };

    summary.addEventListener('pointerenter', onEnter);
    summary.addEventListener('pointerleave', onLeave);
    summary.addEventListener('focus', onFocus);
    summary.addEventListener('blur', onBlur);
    summary.addEventListener('click', onClick);
    el.addEventListener('toggle', onToggle);
    var scroller = el.closest('.ai-panel');
    if (scroller) scroller.addEventListener('scroll', onScroll, { passive: true });
    window.addEventListener('resize', onScroll);
    if (el.open) build();

    return function destroy() {
      cancelTimers();
      clearTimeout(hideT);
      document.removeEventListener('keydown', onEsc, true);
      window.removeEventListener('resize', onScroll);
      if (scroller) scroller.removeEventListener('scroll', onScroll);
      if (card) card.remove();
    };
  }

  var mounted = new WeakMap();
  Chronicle.register('hex-map-preview', {
    init: function (el) { mounted.set(el, mount(el)); },
    destroy: function (el) { var d = mounted.get(el); if (d) d(); mounted.delete(el); }
  });
})();
