/**
 * map_hexes.js -- hexes as a layer on a Chronicle map.
 *
 * When a map's grid type is "hex" the viewer hands this module its context and
 * the module draws the whole hex layer itself: the hex lines, the painted
 * terrain, the names, and the Hexes tool (Look and Paint) with its panel. It
 * owns the lines because one set is enough: while it is running the viewer
 * does not draw its plain hex grid underneath (ctx.onHexLines).
 *
 * The layer is one SVG laid over the map in a Leaflet pane at z-index 390,
 * above pictures (380) and below the vector drawings (400). Terrain is drawn
 * as one path per terrain kind (a flat colour plus a repeating icon pattern),
 * never one element per hex, so a large painted map stays cheap. Only a hex
 * that is changing right now gets its own short-lived elements, to cross-fade
 * the old art into the new.
 *
 * Hex maths are pointy-top with odd rows shifted right, the same layout as the
 * viewer's plain grid and the server (hex.go). The pure parts are at the top
 * and exported for test/js/map_hexes.test.mjs; the browser part needs Leaflet
 * and window.chronicleMap.
 *
 * Data comes from, and is saved to, the REST API:
 *   GET   /campaigns/:id/maps/:mid/hexes          layer + the cells this viewer may see
 *   PATCH /campaigns/:id/maps/:mid/hexes/cells    { cells: [{col,row,terrain?,name?,notes?}] }
 * PATCH is partial: a stroke sends only terrain, an edit of a name sends only
 * the name. The server decides who may write; ctx.canPaintHexes (the same rule
 * as HexService.requireWriter) only decides what is offered.
 */
(function () {
  'use strict';

  var NS = 'http://www.w3.org/2000/svg';
  var SQRT3 = Math.sqrt(3);

  // The terrain kinds the server allows, with the label and flat colour of the
  // palette. Town is shown to people as "Places".
  var TERRAINS = [
    ['plains', 'Plains', '#c9d58a'], ['forest', 'Forest', '#5f8f4a'], ['hills', 'Hills', '#b8a46a'],
    ['mountain', 'Mountains', '#8d8478'], ['water', 'Water', '#6f9fc0'], ['swamp', 'Swamp', '#6f7d55'],
    ['desert', 'Desert', '#e3cc8c'], ['snow', 'Snow', '#e8eef2'], ['town', 'Places', '#c4a27a'],
    ['road', 'Road', '#a0805a']
  ];
  var COLOR = {};
  TERRAINS.forEach(function (t) { COLOR[t[0]] = t[2]; });

  // Simple art: one flat icon per terrain, drawn in a box about 24 wide and
  // centred on the origin. Road has none: it is a line through its hexes.
  var SIMPLE = {
    forest: '<path d="M-6 6l5-11 5 11z" fill="#2f5d2a"/><path d="M2 7l5-12 5 12z" fill="#3f7534"/>',
    mountain: '<path d="M-11 7L-1-9L10 7z" fill="#7b7368" stroke="#3f3a33" stroke-width="1" stroke-linejoin="round"/><path d="M-1-9L-4.6-3.2-2.6-4-1-2.4.8-4 2.6-3.2z" fill="#fff"/>',
    hills: '<path d="M-11 6Q-6-4-1 6z" fill="#b39d5f" stroke="#6e5f35" stroke-width="1"/><path d="M-2 6Q4-6 11 6z" fill="#c4ae6c" stroke="#6e5f35" stroke-width="1"/>',
    water: '<path d="M-8-1.5q2-3 4 0t4 0t4 0t4 0M-8 3.5q2-3 4 0t4 0t4 0t4 0" fill="none" stroke="#2f6488" stroke-width="1.6" stroke-linecap="round"/>',
    swamp: '<path d="M-6 6V-4M0 6V-7M6 6V-3" stroke="#44532d" stroke-width="1.5" stroke-linecap="round"/><ellipse cx="-6" cy="-5" rx="1.3" ry="2.6" fill="#6b4a2a"/><ellipse cx="0" cy="-8" rx="1.3" ry="2.6" fill="#6b4a2a"/><path d="M-9 6h18" stroke="#58777e" stroke-width="1.6" stroke-linecap="round"/>',
    desert: '<path d="M-11 5Q-3-5 5-2Q9 0 11 5z" fill="#e3c37c" stroke="#a5874a" stroke-width="1"/><circle cx="6" cy="-6" r="2.4" fill="#e6a23c"/>',
    snow: '<path d="M0-8V8M-7-4L7 4M-7 4L7-4" stroke="#6f8ea4" stroke-width="1.6" stroke-linecap="round"/><circle r="1.6" fill="#6f8ea4"/>',
    town: '<path d="M-9 6V0l4-4 4 4v6z" fill="#d8c7a2" stroke="#6b5233" stroke-width="1"/><path d="M-10 0.5l5-5.5 5 5.5" fill="none" stroke="#9b4a32" stroke-width="2" stroke-linejoin="round"/><path d="M1 6V-2l4-4.5 4 4.5v8z" fill="#e6d8b8" stroke="#6b5233" stroke-width="1"/><path d="M0-1.5l5-5.5 5 5.5" fill="none" stroke="#7a3f2c" stroke-width="2" stroke-linejoin="round"/>',
    plains: '<path d="M-6 6L-8-1M-4 6V-3M-2 6L0 0M3 6L2-2M5 6V-4M7 6L9 0" stroke="#5a7530" stroke-width="1.4" stroke-linecap="round" fill="none"/>'
  };

  // Icons are drawn a little larger than a hex's radius divided by 26, as the
  // approved design does, so they fill the hex without touching its edge.
  var ICON_SCALE = 1.55 / 26;

  // Hexes along one axis, one more than the server's MaxHexCoord (hex.go), so
  // the field never offers a position the server would refuse.
  var MAX_HEX_AXIS = 400;
  // A keepalive request body may not exceed 64 KiB in browsers; stay under it.
  var KEEPALIVE_MAX_BYTES = 60000;
  // The server's limit on entries per request.
  var BATCH_MAX = 500;
  // A layer this dense is skipped, as the plain grid does, so a tiny grid size
  // on a huge picture cannot freeze the page.
  var MAX_POSITIONS = 60000;
  // How long the old art takes to fade into the new, and how long painting
  // waits for more strokes before it saves.
  var FX_MS = 480;
  var FLUSH_MS = 250;
  var TEXT_FLUSH_MS = 700;

  // ---- Pure maths ----

  function key(col, row) { return col + ',' + row; }

  // geometry lays the hex field over the whole map. gridSize is the stored
  // grid.size, measured as if the map were 1000 wide; hex (0,0) is centred on
  // the map's top-left corner, which is where the plain grid starts too.
  function geometry(gridSize, mapW, mapH) {
    var r = gridSize * mapW / 1000 / 2;
    var w = SQRT3 * r;
    return {
      r: r, w: w, ox: 0, oy: 0,
      cols: Math.min(MAX_HEX_AXIS, Math.ceil(mapW / w) + 1),
      rows: Math.min(MAX_HEX_AXIS, Math.ceil((mapH + r) / (1.5 * r)))
    };
  }

  // center is the centre of hex (col,row) in map pixels.
  function center(g, col, row) {
    return [g.ox + col * g.w + ((row & 1) ? g.w / 2 : 0), g.oy + row * 1.5 * g.r];
  }

  // hexAt is the hex under a map point, or null outside the field. It rounds in
  // cube space so a point on a border lands in exactly one hex.
  function hexAt(g, x, y) {
    if (!(g.r > 0)) return null;
    x -= g.ox; y -= g.oy;
    var cx = (SQRT3 / 3 * x - y / 3) / g.r;
    var cz = (2 / 3 * y) / g.r;
    var cy = -cx - cz;
    var rx = Math.round(cx), ry = Math.round(cy), rz = Math.round(cz);
    var dx = Math.abs(rx - cx), dy = Math.abs(ry - cy), dz = Math.abs(rz - cz);
    if (dx > dy && dx > dz) rx = -ry - rz;
    else if (dy <= dz) rz = -rx - ry;
    var row = rz + 0; // + 0 turns a -0 into 0
    var col = rx + (row - (row & 1)) / 2;
    if (col < 0 || row < 0 || col >= g.cols || row >= g.rows) return null;
    return { col: col, row: row };
  }

  function cubeOf(col, row) {
    var x = col - (row - (row & 1)) / 2;
    // 0 - x, not -x: negating zero would give -0, which prints and compares oddly.
    return [x, 0 - x - row, row];
  }

  // distance is the number of steps between two hexes.
  function distance(a, b) {
    var p = cubeOf(a.col, a.row), q = cubeOf(b.col, b.row);
    return Math.max(Math.abs(p[0] - q[0]), Math.abs(p[1] - q[1]), Math.abs(p[2] - q[2]));
  }

  // hexLine lists the hexes from a to b inclusive. A fast drag skips hexes
  // between two pointer samples; painting the line between them leaves no gaps.
  function hexLine(a, b) {
    var n = distance(a, b), p = cubeOf(a.col, a.row), q = cubeOf(b.col, b.row), out = [];
    for (var i = 0; i <= n; i++) {
      var t = n ? i / n : 0;
      var x = p[0] + (q[0] - p[0]) * t, y = p[1] + (q[1] - p[1]) * t, z = p[2] + (q[2] - p[2]) * t;
      var rx = Math.round(x), ry = Math.round(y), rz = Math.round(z);
      var dx = Math.abs(rx - x), dy = Math.abs(ry - y), dz = Math.abs(rz - z);
      if (dx > dy && dx > dz) rx = -ry - rz;
      else if (dy <= dz) rz = -rx - ry;
      out.push({ col: rx + (rz - (rz & 1)) / 2, row: rz + 0 });
    }
    return out;
  }

  // hexD is the outline of a hex with corner radius r centred on (cx, cy).
  function hexD(cx, cy, r) {
    var d = '';
    for (var i = 0; i < 6; i++) {
      var a = Math.PI / 3 * i - Math.PI / 2;
      d += (i ? 'L' : 'M') + (cx + r * Math.cos(a)).toFixed(1) + ' ' + (cy + r * Math.sin(a)).toFixed(1);
    }
    return d + 'Z';
  }

  // cellsPath joins the outlines of many hexes into one path, so a terrain
  // kind is one element however many hexes carry it.
  function cellsPath(g, cells) {
    var d = '';
    for (var i = 0; i < cells.length; i++) {
      var c = center(g, cells[i].col, cells[i].row);
      d += hexD(c[0], c[1], g.r);
    }
    return d;
  }

  // linesPath draws every hex outline of the field as one path.
  function linesPath(g) {
    var d = '';
    for (var row = 0; row < g.rows; row++) {
      for (var col = 0; col < g.cols; col++) {
        var c = center(g, col, row);
        d += hexD(c[0], c[1], g.r);
      }
    }
    return d;
  }

  // roadPath joins each road hex to the road hexes next to it, so a road
  // network of any shape is one path. A road hex with no road neighbour is a
  // dot, so painting one still shows something.
  function roadPath(g, cells) {
    var set = {}, i, d = '';
    for (i = 0; i < cells.length; i++) set[key(cells[i].col, cells[i].row)] = true;
    for (i = 0; i < cells.length; i++) {
      var a = cells[i], ac = center(g, a.col, a.row), linked = false;
      var even = !(a.row & 1);
      // Of each pair of neighbours, only the one to the right or below is
      // drawn from here, so a link is not drawn twice.
      var nb = [
        [a.col + 1, a.row],
        [even ? a.col - 1 : a.col, a.row + 1],
        [even ? a.col : a.col + 1, a.row + 1]
      ];
      var back = [
        [a.col - 1, a.row],
        [even ? a.col - 1 : a.col, a.row - 1],
        [even ? a.col : a.col + 1, a.row - 1]
      ];
      for (var j = 0; j < 3; j++) {
        if (set[key(back[j][0], back[j][1])]) linked = true;
        if (set[key(nb[j][0], nb[j][1])]) {
          var bc = center(g, nb[j][0], nb[j][1]);
          d += 'M' + ac[0].toFixed(1) + ' ' + ac[1].toFixed(1) + 'L' + bc[0].toFixed(1) + ' ' + bc[1].toFixed(1);
          linked = true;
        }
      }
      if (!linked) d += 'M' + ac[0].toFixed(1) + ' ' + ac[1].toFixed(1) + 'h0.01';
    }
    return d;
  }

  // batches splits queued changes into requests the server accepts. Each entry
  // carries only the fields that changed, which is what keeps a paint stroke
  // from touching a hex's name or notes.
  function batches(entries) {
    var out = [];
    for (var i = 0; i < entries.length; i += BATCH_MAX) out.push({ cells: entries.slice(i, i + BATCH_MAX) });
    return out;
  }

  // mergeEntry folds a later change to the same hex into the queued one.
  function mergeEntry(queued, change) {
    var out = { col: queued.col, row: queued.row };
    ['terrain', 'name', 'notes'].forEach(function (f) {
      if (Object.prototype.hasOwnProperty.call(queued, f)) out[f] = queued[f];
      if (Object.prototype.hasOwnProperty.call(change, f)) out[f] = change[f];
    });
    return out;
  }

  function esc(s) {
    return String(s).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }

  if (typeof module !== 'undefined' && module.exports) {
    module.exports = {
      TERRAINS: TERRAINS, SIMPLE: SIMPLE, BATCH_MAX: BATCH_MAX, MAX_HEX_AXIS: MAX_HEX_AXIS, geometry: geometry, center: center,
      hexAt: hexAt, cubeOf: cubeOf, distance: distance, hexLine: hexLine, hexD: hexD,
      cellsPath: cellsPath, linesPath: linesPath, roadPath: roadPath, batches: batches,
      mergeEntry: mergeEntry
    };
  }
  if (typeof window === 'undefined') return;

  // ---- Browser part ----

  var uidCounter = 0;
  var reduceQuery = window.matchMedia ? window.matchMedia('(prefers-reduced-motion: reduce)') : null;
  function reduced() { return !!(reduceQuery && reduceQuery.matches); }

  function svgEl(name, attrs) {
    var el = document.createElementNS(NS, name);
    Object.keys(attrs || {}).forEach(function (k) { el.setAttribute(k, attrs[k]); });
    return el;
  }

  function attach(ctx) {
    if (!ctx || ctx.hexes || !ctx.map) return ctx && ctx.hexes;
    var map = ctx.map;
    var mapW = ctx.imageW, mapH = ctx.imageH;
    var base = '/campaigns/' + ctx.campaignID + '/maps/' + ctx.mapID + '/hexes';
    var uid = 'mph' + (++uidCounter);

    var layerOn = false;      // grid type is hex and the field is small enough to draw
    var active = false;       // the Hexes tool is the current tool
    var mode = 'look';
    var paintTerrain = 'forest';
    var geo = null;
    var cells = {};           // "col,row" -> { col, row, terrain, name, notes }
    var fx = {};              // "col,row" -> { prev, next, t }: hexes cross-fading now
    var version = 0;
    var loaded = false;
    var selected = null;      // "col,row" of the hex whose card is open
    var queue = {};           // changes waiting to be saved, by hex
    var flushTimer = null;
    var fxTimer = null;
    var renderPending = false;
    var saving = Promise.resolve();
    var overlay = null;
    var gFills, gLines, gTop, gHover, hoverPath, defs;
    var dragDisabled = false;
    var stroke = null;        // { last: {col,row} } while the pointer is down in Paint mode
    var canWrite = !!ctx.canPaintHexes;

    var pane = map.getPane('mpHexes') || map.createPane('mpHexes');
    pane.style.zIndex = 390;
    pane.style.pointerEvents = 'none';

    var wrap = document.getElementById('map-wrap');
    var panel = document.getElementById('mp-hexpanel');
    var container = map.getContainer();

    function display() { return ctx.getDisplay ? ctx.getDisplay() : {}; }

    // ---- Overlay ----

    function removeOverlay() {
      if (overlay) { map.removeLayer(overlay); overlay = null; }
    }

    function buildOverlay() {
      removeOverlay();
      var D = display();
      var svg = svgEl('svg', { xmlns: NS, viewBox: '0 0 ' + mapW + ' ' + mapH, preserveAspectRatio: 'none' });
      defs = svgEl('defs');
      gFills = svgEl('g');
      gLines = svgEl('g');
      gTop = svgEl('g');
      gHover = svgEl('g');
      hoverPath = svgEl('path', {
        fill: '#ffffff', 'fill-opacity': '.14', stroke: '#ffffff', 'stroke-width': '2',
        'stroke-linejoin': 'round', 'vector-effect': 'non-scaling-stroke'
      });
      hoverPath.style.display = 'none';
      gHover.appendChild(hoverPath);
      svg.appendChild(defs);
      svg.appendChild(gFills);
      svg.appendChild(gLines);
      svg.appendChild(gTop);
      svg.appendChild(gHover);
      // The hex lines: one path for the whole field, a constant 1px wide at
      // every zoom, like the plain grid it replaces.
      gLines.appendChild(svgEl('path', {
        d: linesPath(geo), fill: 'none', stroke: '#241c10',
        'stroke-opacity': String((D.grid_strength || 30) / 100),
        'stroke-width': '1', 'vector-effect': 'non-scaling-stroke'
      }));
      overlay = L.svgOverlay(svg, [[0, 0], [mapH, mapW]], { pane: 'mpHexes', interactive: false, className: 'mp-hexes' }).addTo(map);
    }

    // iconPattern repeats a terrain's icon at every hex centre. The tile is one
    // hex wide and two rows tall: even-row centres sit mid-tile and odd-row
    // centres on its corners, so the corner icons are drawn at all four.
    function iconPattern(t) {
      var g = geo, w = g.w, h = 3 * g.r, s = g.r * ICON_SCALE;
      function at(x, y) { return '<g transform="translate(' + x.toFixed(1) + ' ' + y.toFixed(1) + ') scale(' + s.toFixed(4) + ')">' + SIMPLE[t] + '</g>'; }
      return '<pattern id="' + uid + t + '" patternUnits="userSpaceOnUse" x="' + (g.ox - w / 2).toFixed(1) + '" y="' + (g.oy - 1.5 * g.r).toFixed(1) +
        '" width="' + w.toFixed(2) + '" height="' + h.toFixed(2) + '">' +
        at(w / 2, 1.5 * g.r) + at(0, 0) + at(w, 0) + at(0, h) + at(w, h) + '</pattern>';
    }

    // artFor is the full art of one hex of a terrain, used only for the hexes
    // that are cross-fading and for the palette swatches.
    function artFor(t, cx, cy, r) {
      if (!t || t === 'road') return '';
      var s = r * ICON_SCALE;
      return '<path d="' + hexD(cx, cy, r) + '" fill="' + COLOR[t] + '" fill-opacity=".4"/>' +
        (SIMPLE[t] ? '<g transform="translate(' + cx.toFixed(1) + ' ' + cy.toFixed(1) + ') scale(' + s.toFixed(4) + ')">' + SIMPLE[t] + '</g>' : '');
    }

    function render() {
      renderPending = false;
      if (!overlay) return;
      var byTerrain = {}, nameCells = [], k;
      for (k in cells) {
        var c = cells[k];
        if (c.terrain && !fx[k]) (byTerrain[c.terrain] = byTerrain[c.terrain] || []).push(c);
        if (c.name) nameCells.push(c);
      }
      var d = '', f = '';
      TERRAINS.forEach(function (t) {
        var list = byTerrain[t[0]];
        if (!list || t[0] === 'road') return;
        d += iconPattern(t[0]);
        var p = cellsPath(geo, list);
        f += '<path d="' + p + '" fill="' + t[2] + '" fill-opacity=".4"/><path d="' + p + '" fill="url(#' + uid + t[0] + ')"/>';
      });
      if (byTerrain.road) {
        f += '<path d="' + roadPath(geo, byTerrain.road) + '" fill="none" stroke="#7a5a35" stroke-width="' + (2.6 * geo.r / 26).toFixed(2) +
          '" stroke-linecap="round" stroke-dasharray="' + (5 * geo.r / 26).toFixed(2) + ' ' + (3 * geo.r / 26).toFixed(2) + '"/>';
      }
      // Hexes in transition: the old art fades out as the new fades in.
      for (k in fx) {
        var e = fx[k], cc = cells[k];
        if (!cc) continue;
        var ct = center(geo, cc.col, cc.row);
        if (e.prev && e.prev !== 'road') f += '<g class="mp-hx-out">' + artFor(e.prev, ct[0], ct[1], geo.r) + '</g>';
        if (e.next && e.next !== 'road') f += '<g class="mp-hx-in">' + artFor(e.next, ct[0], ct[1], geo.r) + '</g>';
      }
      defs.innerHTML = d;
      gFills.innerHTML = f;

      // The top group is rebuilt from DOM nodes, never from strings, because
      // hex names are typed by people.
      while (gTop.firstChild) gTop.removeChild(gTop.firstChild);
      var kk = geo.r / 26;
      nameCells.forEach(function (c2) {
        var ct2 = center(geo, c2.col, c2.row);
        var tx = svgEl('text', {
          x: ct2[0].toFixed(1), y: (ct2[1] + geo.r * 0.8).toFixed(1), 'text-anchor': 'middle',
          'font-family': 'Georgia,serif', 'font-style': 'italic', 'font-weight': '600',
          'font-size': (9 * kk + 2.5).toFixed(1), fill: '#2b2112', stroke: '#f4ead0',
          'stroke-width': '3', 'stroke-linejoin': 'round'
        });
        tx.style.paintOrder = 'stroke';
        tx.textContent = c2.name;
        gTop.appendChild(tx);
      });
      if (selected && active) {
        var sp = selected.split(',');
        var sc = center(geo, +sp[0], +sp[1]);
        gTop.appendChild(svgEl('path', {
          d: hexD(sc[0], sc[1], geo.r), fill: '#2563eb', 'fill-opacity': '.08', stroke: '#2563eb',
          'stroke-width': '3', 'stroke-linejoin': 'round', 'vector-effect': 'non-scaling-stroke'
        }));
      }
    }

    // Redraws coalesce to one per frame, so a drag over many hexes paints once
    // per frame rather than once per pointer event.
    function renderSoon() {
      if (renderPending) return;
      renderPending = true;
      requestAnimationFrame(render);
    }

    function endFx() {
      fxTimer = null;
      var now = Date.now(), pending = false, k;
      for (k in fx) {
        if (now - fx[k].t >= FX_MS) delete fx[k];
        else pending = true;
      }
      renderSoon();
      if (pending) fxTimer = setTimeout(endFx, FX_MS);
    }

    // ---- Cells ----

    function cellAt(col, row) {
      var k = key(col, row);
      return cells[k] || (cells[k] = { col: col, row: row, terrain: null, name: '', notes: null });
    }

    function queueChange(col, row, change) {
      var k = key(col, row);
      var next = { col: col, row: row };
      queue[k] = queue[k] ? mergeEntry(queue[k], change) : mergeEntry(next, change);
    }

    function paintHex(col, row) {
      var t = paintTerrain || null;
      var c = cellAt(col, row);
      if (c.terrain === t) return;
      var prev = c.terrain;
      c.terrain = t;
      // Roads are a line, not a fill, so only a change in the filled art fades.
      var a = prev === 'road' ? null : prev, b = t === 'road' ? null : t;
      if (!reduced() && a !== b) {
        fx[key(col, row)] = { prev: prev, next: t, t: Date.now() };
        if (!fxTimer) fxTimer = setTimeout(endFx, FX_MS + 60);
      }
      queueChange(col, row, { terrain: t });
    }

    function scheduleFlush(ms) {
      clearTimeout(flushTimer);
      flushTimer = setTimeout(flush, ms);
    }

    // flush saves everything queued, one PATCH per 500 hexes. Saves run one
    // after another so a later stroke can never land before an earlier one.
    function flush() {
      clearTimeout(flushTimer);
      flushTimer = null;
      var entries = Object.keys(queue).map(function (k) { return queue[k]; });
      queue = {};
      if (!entries.length) return saving;
      batches(entries).forEach(function (body) {
        saving = saving.then(function () { return send(body); });
      });
      return saving;
    }

    function send(body) {
      // keepalive lets a save that is in flight when the page closes still
      // finish; browsers cap such a body, so a larger one goes the plain way.
      var keep = new TextEncoder().encode(JSON.stringify(body)).length < KEEPALIVE_MAX_BYTES;
      return Chronicle.apiFetch(base + '/cells', { method: 'PATCH', body: body, keepalive: keep }).then(function (res) {
        if (res.ok) {
          return res.json().then(function (r) { if (r && r.version) version = r.version; });
        }
        return res.json().catch(function () { return {}; }).then(function (err) {
          Chronicle.notify(err.message || 'Could not save those hexes', 'error');
          // What is on screen no longer matches the server; show the server's.
          return load();
        });
      }).catch(function () {
        Chronicle.notify('Could not save those hexes', 'error');
        return load();
      });
    }

    function load() {
      return Chronicle.apiFetch(base).then(function (res) { return res.ok ? res.json() : null; }).then(function (data) {
        if (!data) return;
        cells = {};
        (data.cells || []).forEach(function (c) {
          cells[key(c.col, c.row)] = {
            col: c.col, row: c.row, terrain: c.terrain || null, name: c.name || '', notes: c.notes || null
          };
        });
        version = data.version || 0;
        loaded = true;
        fx = {};
        renderSoon();
        renderPanel();
      }).catch(function () { /* the lines still show; painting is just empty */ });
    }

    // ---- Pointer ----

    function pointToHex(e) {
      var ll = map.mouseEventToLatLng(e);
      return hexAt(geo, ll.lng, mapH - ll.lat);
    }

    function moveHover(e) {
      if (!active || !geo) { hoverPath.style.display = 'none'; return; }
      var h = pointToHex(e);
      if (!h) { hoverPath.style.display = 'none'; return; }
      var c = center(geo, h.col, h.row);
      hoverPath.setAttribute('d', hexD(c[0], c[1], geo.r));
      hoverPath.style.display = '';
    }

    function onPointerDown(e) {
      if (!active || !layerOn || mode !== 'paint' || !canWrite || e.button !== 0) return;
      if (e.target.closest && e.target.closest('.leaflet-marker-icon, .leaflet-popup, .leaflet-control')) return;
      var h = pointToHex(e);
      if (!h) return;
      e.preventDefault();
      try { container.setPointerCapture(e.pointerId); } catch (err) { /* capture is a nicety */ }
      stroke = { last: h };
      paintHex(h.col, h.row);
      renderSoon();
    }

    function onPointerMove(e) {
      moveHover(e);
      if (!stroke) return;
      var h = pointToHex(e);
      if (!h || (h.col === stroke.last.col && h.row === stroke.last.row)) return;
      hexLine(stroke.last, h).forEach(function (p) { paintHex(p.col, p.row); });
      stroke.last = h;
      renderSoon();
    }

    function endStroke() {
      if (!stroke) return;
      stroke = null;
      // One request for the whole drag; a quick second stroke joins it.
      scheduleFlush(FLUSH_MS);
    }

    function onPointerLeave() { if (hoverPath) hoverPath.style.display = 'none'; }

    // Look mode: a click selects a hex and opens its card.
    function onMapClick(e) {
      if (!active || !layerOn || mode !== 'look') return;
      var h = hexAt(geo, e.latlng.lng, mapH - e.latlng.lat);
      if (!h) return;
      flush();
      selected = key(h.col, h.row);
      renderSoon();
      renderPanel();
    }

    container.addEventListener('pointerdown', onPointerDown);
    container.addEventListener('pointermove', onPointerMove);
    // Leaving the page (a link, a reload, a closed tab) must not drop edits
    // still waiting on their debounce timer.
    function onPageHide() { flush(); }
    window.addEventListener('pagehide', onPageHide);
    container.addEventListener('pointerup', endStroke);
    container.addEventListener('pointercancel', endStroke);
    container.addEventListener('pointerleave', onPointerLeave);
    map.on('click', onMapClick);

    // ---- Panel ----

    function swatch(t) {
      if (t === 'road') {
        return '<path d="' + hexD(0, 0, 13) + '" fill="#e8dcc0"/><path d="M-11 5Q0-6 11 2" fill="none" stroke="#5a4128" stroke-width="4" stroke-linecap="round"/><path d="M-11 5Q0-6 11 2" fill="none" stroke="#c19a66" stroke-width="2.4" stroke-linecap="round"/>';
      }
      return '<path d="' + hexD(0, 0, 13) + '" fill="#efe6cf"/>' + artFor(t, 0, 0, 13);
    }

    function modes() { return canWrite ? [['look', 'Look'], ['paint', 'Paint']] : [['look', 'Look']]; }

    function renderPanel() {
      if (!panel) return;
      if (!active || !layerOn) { panel.hidden = true; return; }
      var ms = modes();
      if (!ms.some(function (m) { return m[0] === mode; })) mode = 'look';
      var h = '<div class="mp-hx-head">Hexes<button type="button" class="mp-hx-x" aria-label="Close hexes"><svg class="mp-hexico" viewBox="0 0 24 24" aria-hidden="true"><path d="M6 6l12 12M18 6L6 18"/></svg></button></div>';
      h += '<div class="mp-chips">' + ms.map(function (m) {
        return '<button type="button" class="mp-chip" data-hm="' + m[0] + '" aria-pressed="' + (mode === m[0]) + '">' + m[1] + '</button>';
      }).join('') + '</div>';
      if (mode === 'paint') {
        h += '<div class="mp-hx-pal">' + TERRAINS.map(function (t) {
          return '<button type="button" data-ter="' + t[0] + '" aria-pressed="' + (paintTerrain === t[0]) + '"><svg viewBox="-15 -14 30 28" width="36" height="32" aria-hidden="true">' + swatch(t[0]) + '</svg>' + esc(t[1]) + '</button>';
        }).join('') + '<button type="button" data-ter="" aria-pressed="' + (paintTerrain === '') + '"><i></i>Clear</button></div>' +
          '<p>Click or drag across hexes. The picture still shows through.</p>';
      }
      if (mode === 'look') {
        if (selected) h += '<div class="mp-hx-card" id="mp-hx-card"></div>';
        else h += '<p>Click a hex to see ' + (canWrite ? 'or write what’s there.' : 'what the party knows about it.') + '</p>';
      }
      panel.innerHTML = h;
      panel.hidden = false;
      panel.querySelector('.mp-hx-x').onclick = function () { if (ctx.setTool) ctx.setTool('move'); };
      Array.prototype.forEach.call(panel.querySelectorAll('[data-hm]'), function (b) {
        b.onclick = function () {
          flush();
          mode = b.dataset.hm;
          if (mode !== 'look') selected = null;
          applyMode();
          renderSoon();
          renderPanel();
        };
      });
      Array.prototype.forEach.call(panel.querySelectorAll('[data-ter]'), function (b) {
        b.onclick = function () { paintTerrain = b.dataset.ter; renderPanel(); };
      });
      if (mode === 'look' && selected) fillCard();
    }

    // fillCard builds the open hex's card. Names and notes go in as values and
    // text, never as markup, because people type them.
    function fillCard() {
      var card = document.getElementById('mp-hx-card');
      if (!card) return;
      var p = selected.split(',');
      var c = cells[selected] || { col: +p[0], row: +p[1], name: '', notes: null };
      if (canWrite) {
        var name = document.createElement('input');
        name.className = 'mp-input'; name.id = 'mp-hx-name'; name.maxLength = 120;
        name.placeholder = 'Name this hex'; name.setAttribute('aria-label', 'Hex name'); name.value = c.name || '';
        var notes = document.createElement('textarea');
        notes.className = 'mp-input'; notes.id = 'mp-hx-notes'; notes.rows = 3; notes.maxLength = 2000;
        notes.placeholder = 'What’s here'; notes.setAttribute('aria-label', 'Notes'); notes.value = c.notes || '';
        name.oninput = function () {
          cellAt(+p[0], +p[1]).name = name.value;
          queueChange(+p[0], +p[1], { name: name.value });
          scheduleFlush(TEXT_FLUSH_MS);
          renderSoon();
        };
        notes.oninput = function () {
          cellAt(+p[0], +p[1]).notes = notes.value || null;
          queueChange(+p[0], +p[1], { notes: notes.value || null });
          scheduleFlush(TEXT_FLUSH_MS);
        };
        name.onblur = notes.onblur = function () { flush(); };
        card.appendChild(name);
        card.appendChild(notes);
        var note = document.createElement('small');
        note.className = 'mp-hx-note';
        note.style.cssText = 'display:block;margin-top:4px;color:var(--text-muted,#6b7280);font-size:11px';
        note.textContent = 'Everyone who can see this map can read these notes.';
        card.appendChild(note);
      } else {
        var title = document.createElement('b');
        title.textContent = c.name || 'Unnamed hex';
        var body = document.createElement('span');
        body.textContent = c.notes || '';
        card.appendChild(title);
        card.appendChild(body);
      }
    }

    // ---- Tool state ----

    function applyMode() {
      var paintOn = active && layerOn && mode === 'paint' && canWrite;
      // While painting, a drag paints rather than pans; Look and Move pan.
      if (paintOn && !dragDisabled) { map.dragging.disable(); dragDisabled = true; }
      if (!paintOn && dragDisabled) { map.dragging.enable(); dragDisabled = false; }
      if (active && layerOn) container.style.cursor = paintOn ? 'crosshair' : '';
      if (ctx.setHint && active) ctx.setHint(mode === 'paint' ? 'Click or drag to paint terrain' : 'Click a hex to see it');
    }

    function setActive(on) {
      on = !!on && layerOn;
      if (on === active) { if (on) renderPanel(); return; }
      active = on;
      if (!active) {
        flush();
        selected = null;
        stroke = null;
        if (dragDisabled) { map.dragging.enable(); dragDisabled = false; }
        if (hoverPath) hoverPath.style.display = 'none';
      }
      applyMode();
      renderSoon();
      renderPanel();
    }

    function showTool(on) {
      if (!wrap) return;
      Array.prototype.forEach.call(wrap.querySelectorAll('[data-tool="hex"]'), function (b) { b.hidden = !on; });
      // People who cannot use the other tools get a rail only while hexes exist.
      var rail = wrap.querySelector('.mp-rail');
      if (rail && !ctx.isScribe) rail.hidden = !on;
    }

    // refresh follows the map's display settings: it turns the layer on or off,
    // and re-lays the hexes when the grid size or strength changes (the owner's
    // settings sheet previews them live).
    function refresh() {
      var D = display();
      var g = null;
      if (D.grid_type === 'hex') {
        g = geometry(D.grid_size || 50, mapW, mapH);
        if (!(g.r >= 1) || g.cols * g.rows > MAX_POSITIONS) g = null;
      }
      if (!g) {
        layerOn = false;
        setActive(false);
        removeOverlay();
        showTool(false);
        if (ctx.onHexLines) ctx.onHexLines(false);
        return;
      }
      geo = g;
      layerOn = true;
      buildOverlay();
      showTool(true);
      if (ctx.onHexLines) ctx.onHexLines(true);
      if (!loaded) load(); else renderSoon();
      renderPanel();
    }

    var handle = {
      refresh: refresh,
      setActive: setActive,
      isOn: function () { return layerOn; },
      // escape steps back from an open card; false means nothing was open.
      escape: function () {
        if (active && selected) { selected = null; renderSoon(); renderPanel(); return true; }
        return false;
      },
      destroy: function () {
        window.removeEventListener('pagehide', onPageHide);
        flush();
        clearTimeout(fxTimer);
        container.removeEventListener('pointerdown', onPointerDown);
        container.removeEventListener('pointermove', onPointerMove);
        container.removeEventListener('pointerup', endStroke);
        container.removeEventListener('pointercancel', endStroke);
        container.removeEventListener('pointerleave', onPointerLeave);
        map.off('click', onMapClick);
        if (dragDisabled) { try { map.dragging.enable(); } catch (e) { /* map already gone */ } }
        removeOverlay();
        if (panel) panel.hidden = true;
      }
    };
    ctx.hexes = handle;
    refresh();
    return handle;
  }

  window.ChronicleMapHexes = { init: attach };
})();
