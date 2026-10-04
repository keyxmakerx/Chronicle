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
 * By default the field covers the whole map. When the layer is pinned to a
 * picture (layer.anchor_drawing_id) the field is laid inside that picture's box
 * and follows it live as it is moved or resized: the geometry is built once in
 * a box 1000 wide, and the picture's position and size only change one
 * transform, so cell keys never change with the picture's size.
 *
 * Data comes from, and is saved to, the REST API:
 *   GET   /campaigns/:id/maps/:mid/hexes          layer + the cells this viewer may see
 *   PATCH /campaigns/:id/maps/:mid/hexes/cells    { cells: [{col,row,terrain?,name?,notes?}] }
 *   PUT   /campaigns/:id/maps/:mid/hexes/layer    { anchor_drawing_id?, fog_enabled? } (owner or DM; the viewer sends it)
 *   POST  /campaigns/:id/maps/:mid/hexes/fog      { cells: [{col,row}], explored } or { reset: true } (DM)
 *   PUT   /campaigns/:id/maps/:mid/hexes/party    { col, row } -> { version, path }
 * PATCH is partial: a stroke sends only terrain, an edit of a name sends only
 * the name. The server decides who may write; ctx.canPaintHexes (the same rule
 * as HexService.requireWriter) only decides what is offered.
 *
 * Fog of war. A viewer who cannot see DM-only content is sent explored cells
 * only, so "unexplored" on their side is simply every field hex without a
 * cell; the DM is sent every cell with its explored flag. Unexplored hexes are
 * drawn as part of the shared smoke (map_shadow.js, one union mask, so overlaps
 * never darken) for players and faintly hatched for the DM. The server tells
 * every client "hex.changed" with a version (and, when safe, the party's path);
 * the client refetches the filtered read, never trusting the event for data.
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

  // The width a pinned field is built at. grid.size is measured as if the
  // picture were this wide, so a picture's real size only scales the result.
  var REF_W = 1000;

  function clampAxis(n) { return Math.max(1, Math.min(MAX_HEX_AXIS, n)); }

  // anchoredGeometry lays the field inside a box (boxW by boxH, origin 0,0): hex
  // (0,0) is the first hex that fits whole at the top-left, and the field stops
  // where the next hex would cross the right or bottom edge, so no hex spills
  // off the picture. Mirrors NewAnchoredHexGeometry in hex.go.
  function anchoredGeometry(gridSize, boxW, boxH) {
    var r = gridSize * boxW / 1000 / 2;
    var w = SQRT3 * r;
    if (!(r > 0)) return { r: r, w: w, ox: 0, oy: 0, cols: 0, rows: 0 };
    return {
      r: r, w: w, ox: w / 2, oy: r,
      cols: clampAxis(Math.floor((boxW - 1.5 * w) / w) + 1),
      rows: clampAxis(Math.floor((boxH - 2 * r) / (1.5 * r)) + 1)
    };
  }

  // boxFromPoints turns a picture's two corners (map percentages) into a box in
  // map pixels, or null when they do not make a usable box.
  function boxFromPoints(points, mapW, mapH) {
    if (!Array.isArray(points) || points.length !== 2) return null;
    var a = points[0], b = points[1];
    if (!a || !b || !isFinite(a.x) || !isFinite(a.y) || !isFinite(b.x) || !isFinite(b.y)) return null;
    var box = {
      x: Math.min(a.x, b.x) / 100 * mapW, y: Math.min(a.y, b.y) / 100 * mapH,
      w: Math.abs(b.x - a.x) / 100 * mapW, h: Math.abs(b.y - a.y) / 100 * mapH
    };
    return box.w > 0 && box.h > 0 ? box : null;
  }

  // layoutFor is where the field is and how it sits on the map: the geometry in
  // its own units, and the transform {x, y, s} that carries those units to map
  // pixels. With no box the field covers the whole map and the transform does
  // nothing; with a box the geometry is always built REF_W wide, so moving or
  // resizing the picture changes only the transform.
  function layoutFor(gridSize, box, mapW, mapH) {
    if (!box) return { geo: geometry(gridSize, mapW, mapH), xf: { x: 0, y: 0, s: 1 } };
    var s = box.w / REF_W;
    return { geo: anchoredGeometry(gridSize, REF_W, box.h / s), xf: { x: box.x, y: box.y, s: s } };
  }

  // hexAtMap is the hex under a map point (pixels), or null outside the field.
  function hexAtMap(lay, x, y) {
    return hexAt(lay.geo, (x - lay.xf.x) / lay.xf.s, (y - lay.xf.y) / lay.xf.s);
  }

  function xfAttr(xf) { return 'translate(' + xf.x + ' ' + xf.y + ') scale(' + xf.s + ')'; }

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


  // ---- Fog and the party (pure) ----

  var STEP_MS = 170;   // time the party token takes to cross one hex
  var LIFT = 0.12;     // the token's little hop, as a fraction of a hex radius

  // neighbors lists the six hexes around (col,row), odd rows shifted right
  // (the same order and rule as Neighbors in hex.go).
  function neighbors(col, row) {
    var odd = row & 1;
    return [
      { col: col + 1, row: row }, { col: col - 1, row: row },
      { col: col + (odd ? 1 : 0), row: row - 1 }, { col: col + (odd ? 0 : -1), row: row - 1 },
      { col: col + (odd ? 1 : 0), row: row + 1 }, { col: col + (odd ? 0 : -1), row: row + 1 }
    ];
  }

  // unexploredKeys lists every hex of the field that is not in the explored
  // set (an object keyed "col,row"). One definition of "unexplored" for the
  // smoke, the DM's hatch and the click card.
  function unexploredKeys(g, explored) {
    var out = [];
    for (var row = 0; row < g.rows; row++) {
      for (var col = 0; col < g.cols; col++) {
        if (!explored[key(col, row)]) out.push({ col: col, row: row });
      }
    }
    return out;
  }

  // fogPolygons turns hexes into flat corner lists [x0,y0,x1,y1,...] in a
  // canvas whose pixels are sx and sy per map pixel, through the field's
  // transform xf. The shadow module fills them into its mask.
  function fogPolygons(g, xf, list, sx, sy) {
    var out = [];
    for (var i = 0; i < list.length; i++) {
      var c = center(g, list[i].col, list[i].row), poly = [];
      for (var k = 0; k < 6; k++) {
        var a = Math.PI / 3 * k - Math.PI / 2;
        poly.push((xf.x + (c[0] + g.r * Math.cos(a)) * xf.s) * sx, (xf.y + (c[1] + g.r * Math.sin(a)) * xf.s) * sy);
      }
      out.push(poly);
    }
    return out;
  }

  // walkPoint is where the party token is, elapsed ms after it set off along
  // points ([[x,y],...] hex centres). Each hex takes stepMs, eased so the token
  // pauses a beat at every hex, with a small hop of lift pixels between them.
  // done is true once it has arrived.
  function walkPoint(points, elapsed, stepMs, lift) {
    var n = points.length - 1;
    if (n < 1) return { x: points[0][0], y: points[0][1], done: true };
    var e = Math.max(0, elapsed) / stepMs;
    if (e >= n) return { x: points[n][0], y: points[n][1], done: true };
    var i = Math.floor(e), f = e - i;
    f = f * f * (3 - 2 * f);
    var a = points[i], b = points[i + 1];
    return {
      x: a[0] + (b[0] - a[0]) * f,
      y: a[1] + (b[1] - a[1]) * f - Math.sin(f * Math.PI) * lift,
      done: false
    };
  }

  // revealSchedule says when each hex a move reveals should come out of the
  // fog: the hexes around path[i] when the token is half a step into hex i.
  // A hex near several path hexes appears with the first. The field's edge
  // clips it, as the server does. Returns [{col,row,at}] with at in ms.
  function revealSchedule(path, stepMs, g) {
    var seen = {}, out = [];
    function add(c, at) {
      if (c.col < 0 || c.row < 0 || c.col >= g.cols || c.row >= g.rows) return;
      var k = key(c.col, c.row);
      if (seen[k]) return;
      seen[k] = true;
      out.push({ col: c.col, row: c.row, at: at });
    }
    for (var i = 0; i < path.length; i++) {
      var at = i * stepMs + stepMs / 2;
      add(path[i], at);
      neighbors(path[i].col, path[i].row).forEach(function (n) { add(n, at); });
    }
    return out;
  }

  // fogBatches splits a reveal or hide into requests of at most BATCH_MAX
  // hexes, each saying which way it goes (the server refuses a body that does
  // not say). cells is [{col,row}].
  function fogBatches(cells, explored) {
    var out = [];
    for (var i = 0; i < cells.length; i += BATCH_MAX) {
      out.push({ cells: cells.slice(i, i + BATCH_MAX).map(function (c) { return { col: c.col, row: c.row }; }), explored: !!explored });
    }
    return out;
  }

  // partyMoverAllowed mirrors HexService.requirePartyMover, which decides what
  // is offered: a DM always, a scribe unless the map keeps the party to
  // owners, a player never. The server enforces it regardless.
  function partyMoverAllowed(o) {
    if (o.isDM) return true;
    return !!o.isScribe && o.partyWho !== 'owners';
  }

  // isStale is true when an event tells nothing new: its version is not past
  // the state this client last read, or is the version of a write this client
  // made itself (own is an object keyed by version), which echoes back.
  function isStale(known, incoming, own) {
    if (typeof incoming !== 'number' || incoming <= 0) return false;
    return incoming <= known || !!(own && own[incoming]);
  }

  // The mapped picture a viewer below DM level is shown while fog hides land
  // from them: the server's smudged copy. The version only busts the browser's
  // cache; the server always renders the current state.
  function playerImageURL(campaignID, mapID, version) {
    return '/campaigns/' + campaignID + '/maps/' + mapID + '/player-image?v=' + version;
  }

  // wantsPlayerCopy says whether the map picture must be the server's copy.
  // A layer pinned to a picture leaves the map's own picture alone. A page that
  // was served the copy keeps asking for it, because the original address is
  // not known to the page and the copy is always current.
  function wantsPlayerCopy(o) {
    if (o.dmViewer) return false;
    return (!!o.fogOn && !o.anchored) || !!o.startedWithCopy;
  }

  function esc(s) {
    return String(s).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }

  if (typeof module !== 'undefined' && module.exports) {
    module.exports = {
      TERRAINS: TERRAINS, SIMPLE: SIMPLE, BATCH_MAX: BATCH_MAX, MAX_HEX_AXIS: MAX_HEX_AXIS, geometry: geometry, center: center,
      anchoredGeometry: anchoredGeometry, boxFromPoints: boxFromPoints, layoutFor: layoutFor, hexAtMap: hexAtMap, xfAttr: xfAttr,
      hexAt: hexAt, cubeOf: cubeOf, distance: distance, hexLine: hexLine, hexD: hexD,
      cellsPath: cellsPath, linesPath: linesPath, roadPath: roadPath, batches: batches,
      mergeEntry: mergeEntry,
      STEP_MS: STEP_MS, neighbors: neighbors, unexploredKeys: unexploredKeys, fogPolygons: fogPolygons,
      walkPoint: walkPoint, revealSchedule: revealSchedule, fogBatches: fogBatches,
      partyMoverAllowed: partyMoverAllowed, isStale: isStale, playerImageURL: playerImageURL,
      wantsPlayerCopy: wantsPlayerCopy
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
    var layout = null;        // { geo, xf }: where the field sits on the map
    var anchorId = null;      // the picture the server says the layer covers, or null for the whole map
    var hidden = false;       // the server withholds this layer from the viewer
    var loading = false;
    var loadTried = false;
    var placedSig = '';
    var placePending = false;
    var pendingOpen = null;   // 'look' | 'paint': open the tool once the field is up
    var reloadedFor = null;
    var picUnsub = null;
    var root = null;
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
    var canFog = !!ctx.canFog;                 // DM: reveal and hide hexes (HexService.RevealFog)
    var canParty = !!ctx.canMoveParty;         // offered Party mode; the server decides
    var seesAll = !!ctx.canDmOnly;             // the server sends this viewer every hex
    var fogLayerOn = false;   // the layer's saved fog_enabled
    var lastFog = false;
    var party = null;         // { col, row } or null
    var partyVer = 0;         // version of the last move animated, so one move plays once
    var partyAnim = null;     // { pts, t0 } while the token walks
    var partyRaf = 0;
    var partyBusy = false;
    var revealAt = {};        // "col,row" -> performance.now() time a hex leaves the fog
    var revealTimers = [];
    var fogQ = null;          // { value, cells } reveals or hides waiting to be saved
    var loadedVersion = 0;    // version of the state last read
    var own = {};             // versions of this client's own writes
    var pendingSaves = 0;
    var remoteWaiting = false;
    var hatchDirty = true;
    var gHatch, gParty, partyTok, fieldD = '';
    var shadow = null, smokeSet = false;
    var startedWithCopy = null, initialImage = null, copyVer = null;

    var pane = map.getPane('mpHexes') || map.createPane('mpHexes');
    pane.style.zIndex = 390;
    pane.style.pointerEvents = 'none';

    var wrap = document.getElementById('map-wrap');
    var panel = document.getElementById('mp-hexpanel');
    var container = map.getContainer();

    function display() { return ctx.getDisplay ? ctx.getDisplay() : {}; }

    // fogOn is whether fog hides land right now: the owner's unsaved choice in
    // the settings sheet while it is open (a live preview), else the layer's.
    function fogOn() {
      var D = display();
      return Object.prototype.hasOwnProperty.call(D, 'hex_fog') ? !!D.hex_fog : fogLayerOn;
    }

    // ---- Overlay ----

    function removeOverlay() {
      if (overlay) { map.removeLayer(overlay); overlay = null; }
      root = null;
      placedSig = '';
    }

    function applyTransform() {
      if (root && layout) root.setAttribute('transform', xfAttr(layout.xf));
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
      gHatch = svgEl('g');
      gParty = svgEl('g');
      gParty.style.pointerEvents = 'none';
      gHatch.style.pointerEvents = 'none';
      hoverPath = svgEl('path', {
        fill: '#ffffff', 'fill-opacity': '.14', stroke: '#ffffff', 'stroke-width': '2',
        'stroke-linejoin': 'round', 'vector-effect': 'non-scaling-stroke'
      });
      hoverPath.style.display = 'none';
      gHover.appendChild(hoverPath);
      // Everything is drawn in the field's own units inside one group; its
      // transform is all that moves when a pinned picture does.
      root = svgEl('g');
      root.appendChild(gFills);
      root.appendChild(gHatch);
      root.appendChild(gLines);
      root.appendChild(gTop);
      root.appendChild(gParty);
      root.appendChild(gHover);
      svg.appendChild(defs);
      svg.appendChild(root);
      applyTransform();
      // The hex lines: one path for the whole field, a constant 1px wide at
      // every zoom, like the plain grid it replaces.
      fieldD = linesPath(geo);
      hatchDirty = true;
      partyTok = null;
      gLines.appendChild(svgEl('path', {
        d: fieldD, fill: 'none', stroke: '#241c10',
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
      if (hatchDirty) renderHatch();
      renderParty();
    }

    // ---- Fog: what is unexplored, and how each kind of viewer sees it ----

    // heldBack is true while a hex a move just revealed has not been reached by
    // the walking party yet, so land comes out of the fog as the token arrives.
    function heldBack(k) { return revealAt[k] !== undefined && performance.now() < revealAt[k]; }

    function exploredSet() {
      var set = {};
      for (var k in cells) if (cells[k].explored && !heldBack(k)) set[k] = true;
      return set;
    }

    // The DM's view of the fog: unexplored hexes faintly hatched. The hatch is
    // one rectangle masked by the field minus the explored hexes, so a drag
    // costs the explored count, not the field.
    function renderHatch() {
      hatchDirty = false;
      if (!gHatch) return;
      while (gHatch.firstChild) gHatch.removeChild(gHatch.firstChild);
      if (!layerOn || !geo || !fogOn() || !seesAll) return;
      var k = geo.r / 26, exp = [];
      var set = exploredSet();
      for (var c in set) exp.push(cells[c]);
      var bb = { x: geo.ox - geo.w / 2, y: geo.oy - geo.r, w: geo.cols * geo.w + geo.w, h: (geo.rows - 1) * 1.5 * geo.r + 2 * geo.r };
      var defsEl = svgEl('defs');
      var pat = svgEl('pattern', { id: uid + 'hatch', patternUnits: 'userSpaceOnUse', width: (7 * k).toFixed(2), height: (7 * k).toFixed(2), patternTransform: 'rotate(45)' });
      pat.appendChild(svgEl('line', { x1: 0, y1: 0, x2: 0, y2: (7 * k).toFixed(2), stroke: '#1d262d', 'stroke-width': (2.2 * k).toFixed(2) }));
      var mask = svgEl('mask', { id: uid + 'hm', maskUnits: 'userSpaceOnUse', x: bb.x.toFixed(1), y: bb.y.toFixed(1), width: bb.w.toFixed(1), height: bb.h.toFixed(1) });
      mask.appendChild(svgEl('path', { d: fieldD, fill: '#fff' }));
      mask.appendChild(svgEl('path', { d: cellsPath(geo, exp), fill: '#000' }));
      defsEl.appendChild(pat);
      defsEl.appendChild(mask);
      gHatch.appendChild(defsEl);
      gHatch.appendChild(svgEl('rect', {
        x: bb.x.toFixed(1), y: bb.y.toFixed(1), width: bb.w.toFixed(1), height: bb.h.toFixed(1),
        fill: 'url(#' + uid + 'hatch)', mask: 'url(#' + uid + 'hm)', opacity: '.42'
      }));
    }

    // paintSmoke fills the unexplored hexes into the shadow module's mask, so
    // they share one smoke with every shadowed area and overlaps never darken.
    function paintSmoke(g2, cw, ch) {
      if (!layout || !geo) return;
      var polys = fogPolygons(geo, layout.xf, unexploredKeys(geo, exploredSet()), cw / mapW, ch / mapH);
      g2.beginPath();
      polys.forEach(function (p) {
        g2.moveTo(p[0], p[1]);
        for (var i = 2; i < p.length; i += 2) g2.lineTo(p[i], p[i + 1]);
        g2.closePath();
      });
      g2.fill();
      // A hairline stroke closes the seams between neighbouring hexes.
      g2.lineWidth = Math.max(1, cw / mapW * 1.5);
      g2.strokeStyle = '#000';
      g2.stroke();
    }

    function smoke() {
      if (!shadow && window.ChronicleMapShadow && window.L) {
        shadow = window.ChronicleMapShadow.attach(map, {
          toLatLng: function (pt) { return L.latLng(mapH - (pt.y / 100) * mapH, (pt.x / 100) * mapW); }
        });
      }
      return shadow;
    }

    // fogView brings everything fog draws in line with the current state: the
    // players' smoke, the DM's hatch, and the picture players are shown.
    function fogView() {
      var on = layerOn && loaded && !hidden && fogOn();
      // The smoke is only needed by viewers the fog hides land from.
      var sh = (on && !seesAll) || smokeSet ? smoke() : null;
      if (sh) {
        if (on && !seesAll) {
          if (!smokeSet) { sh.setHexFog(paintSmoke); smokeSet = true; } else sh.refreshHexFog();
        } else if (smokeSet) { sh.setHexFog(null); smokeSet = false; }
      }
      hatchDirty = true;
      renderSoon();
      syncImage();
    }

    // The picture a viewer below DM level may see while fog hides land is the
    // server's copy with those hexes smudged. It is asked for by version, so a
    // reveal fetches a fresh one.
    function syncImage() {
      if (!ctx.setMapImage || !ctx.getMapImage || !loaded) return;
      if (startedWithCopy === null) {
        initialImage = ctx.getMapImage() || '';
        startedWithCopy = /\/player-image\?/.test(initialImage);
        if (startedWithCopy) copyVer = version;
      }
      var want = wantsPlayerCopy({ dmViewer: seesAll, fogOn: fogLayerOn && !hidden, anchored: !!anchorId, startedWithCopy: startedWithCopy });
      if (want) {
        if (copyVer !== version) { copyVer = version; ctx.setMapImage(playerImageURL(ctx.campaignID, ctx.mapID, version)); }
      } else if (copyVer !== null && !startedWithCopy) {
        copyVer = null;
        ctx.setMapImage(initialImage);
      }
    }

    // ---- The party ----

    function tokenNode(k) {
      var g2 = svgEl('g');
      g2.appendChild(svgEl('circle', { r: (10 * k + 2).toFixed(2), fill: '#b91c1c', stroke: '#fff', 'stroke-width': (2.5 * k).toFixed(2) }));
      g2.appendChild(svgEl('circle', { r: (10 * k - 1.5).toFixed(2), fill: 'none', stroke: '#fca5a5', 'stroke-width': k.toFixed(2), opacity: '.7' }));
      g2.appendChild(svgEl('path', {
        transform: 'scale(' + k.toFixed(3) + ')', d: 'M-3.5 6V-6M-3.5-6h8l-2.2 3 2.2 3h-8',
        fill: '#fff', stroke: '#fff', 'stroke-width': '1.3', 'stroke-linejoin': 'round'
      }));
      return g2;
    }

    function partyShown() {
      if (!party || !layerOn || hidden) return false;
      // Never draw the party on land the viewer has not been shown, whatever
      // the server sent.
      if (fogOn() && !seesAll && !partyAnim) {
        var c = cells[key(party.col, party.row)];
        return !!(c && c.explored);
      }
      return true;
    }

    function placeToken(x, y) {
      if (partyTok) partyTok.setAttribute('transform', 'translate(' + x.toFixed(1) + ' ' + y.toFixed(1) + ')');
    }

    function renderParty() {
      if (!gParty || !geo) return;
      if (!partyShown()) { while (gParty.firstChild) gParty.removeChild(gParty.firstChild); partyTok = null; return; }
      if (!partyTok) {
        partyTok = svgEl('g');
        partyTok.appendChild(tokenNode(geo.r / 26));
        gParty.appendChild(partyTok);
      }
      if (!partyAnim) { var c = center(geo, party.col, party.row); placeToken(c[0], c[1]); }
    }

    function stopWalk() {
      partyAnim = null;
      if (partyRaf) { cancelAnimationFrame(partyRaf); partyRaf = 0; }
    }

    // walk moves the token along the path hex by hex, easing at each. With
    // reduced motion, or no path to walk, it simply appears at the end.
    function walk(path) {
      stopWalk();
      if (!geo || reduced() || path.length < 2) { renderParty(); return; }
      var pts = path.map(function (p) { return center(geo, p.col, p.row); });
      partyAnim = { pts: pts, t0: performance.now() };
      renderParty();
      (function frame() {
        if (!partyAnim) return;
        var w = walkPoint(partyAnim.pts, performance.now() - partyAnim.t0, STEP_MS, geo.r * LIFT);
        placeToken(w.x, w.y);
        if (w.done) { partyAnim = null; partyRaf = 0; renderParty(); return; }
        partyRaf = requestAnimationFrame(frame);
      })();
    }

    // applyParty plays a move once: the land around the path stays in the fog
    // until the token reaches it, then the token walks. The same move can
    // arrive twice (the write's answer and the live event); the version makes
    // the second a no-op.
    function applyParty(path, ver) {
      if (!geo || !path || !path.length) return;
      if (ver) { if (ver <= partyVer) return; partyVer = ver; }
      if (!reduced() && path.length > 1) {
        var t = performance.now();
        revealSchedule(path, STEP_MS, geo).forEach(function (r) {
          var k = key(r.col, r.row);
          if (cells[k] && cells[k].explored) return;
          revealAt[k] = t + r.at;
          revealTimers.push(setTimeout(function () { hatchDirty = true; if (smokeSet && shadow) shadow.refreshHexFog(); renderSoon(); }, r.at + 20));
        });
      }
      var last = path[path.length - 1];
      party = { col: last.col, row: last.row };
      walk(path);
    }

    function moveParty(h) {
      if (partyBusy || !canParty) return;
      if (party && party.col === h.col && party.row === h.row) return;
      partyBusy = true;
      Chronicle.apiFetch(base + '/party', { method: 'PUT', body: { col: h.col, row: h.row } }).then(function (res) {
        if (res.ok) {
          return res.json().then(function (r) {
            if (r && r.version) { own[r.version] = true; version = Math.max(version, r.version); }
            applyParty((r && r.path) || [], r && r.version);
            return load();
          });
        }
        return res.json().catch(function () { return {}; }).then(function (err) {
          Chronicle.notify(err.message || 'Could not move the party', 'error');
          return load();
        });
      }).catch(function () {
        Chronicle.notify('Could not move the party', 'error');
        return load();
      }).then(function () { partyBusy = false; });
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
      return cells[k] || (cells[k] = { col: col, row: row, terrain: null, name: '', notes: null, explored: false });
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
      batches(entries).forEach(function (body) {
        pendingSaves++;
        saving = saving.then(function () { return send(body); }).then(settled);
      });
      // Reveals and hides go the same way, one request per 500 hexes.
      if (fogQ) {
        var q = fogQ;
        fogQ = null;
        fogBatches(Object.keys(q.cells).map(function (k) { return q.cells[k]; }), q.value).forEach(function (body) {
          pendingSaves++;
          saving = saving.then(function () { return sendFog(body); }).then(settled);
        });
      }
      return saving;
    }

    // settled runs after each save. A remote change that arrived while edits
    // were unsaved is read now, so it cannot overwrite what is on screen.
    function settled() {
      pendingSaves = Math.max(0, pendingSaves - 1);
      if (pendingSaves === 0 && remoteWaiting && !stroke && !flushTimer && !fogQ && !Object.keys(queue).length) {
        remoteWaiting = false;
        load();
      }
    }

    function sendFog(body) {
      var keep = new TextEncoder().encode(JSON.stringify(body)).length < KEEPALIVE_MAX_BYTES;
      return Chronicle.apiFetch(base + '/fog', { method: 'POST', body: body, keepalive: keep }).then(function (res) {
        if (res.ok) {
          return res.json().then(function (r) { if (r && r.version) { own[r.version] = true; version = Math.max(version, r.version); } });
        }
        return res.json().catch(function () { return {}; }).then(function (err) {
          Chronicle.notify(err.message || 'Could not save the fog', 'error');
          return load();
        });
      }).catch(function () {
        Chronicle.notify('Could not save the fog', 'error');
        return load();
      });
    }

    // queueFog notes one hex to reveal (value true) or hide (false). A change of
    // direction sends what is waiting first, so order is kept.
    function queueFog(col, row, value) {
      if (fogQ && fogQ.value !== value) flush();
      if (!fogQ) fogQ = { value: value, cells: {} };
      fogQ.cells[key(col, row)] = { col: col, row: row };
    }

    // setExplored flips one hex's fog locally and queues the save.
    function setExplored(col, row, value) {
      var c = cellAt(col, row);
      if (c.explored === value) return false;
      c.explored = value;
      queueFog(col, row, value);
      hatchDirty = true;
      if (smokeSet && shadow) shadow.refreshHexFog();
      return true;
    }

    function send(body) {
      // keepalive lets a save that is in flight when the page closes still
      // finish; browsers cap such a body, so a larger one goes the plain way.
      var keep = new TextEncoder().encode(JSON.stringify(body)).length < KEEPALIVE_MAX_BYTES;
      return Chronicle.apiFetch(base + '/cells', { method: 'PATCH', body: body, keepalive: keep }).then(function (res) {
        if (res.ok) {
          return res.json().then(function (r) { if (r && r.version) { own[r.version] = true; version = Math.max(version, r.version); } });
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
            col: c.col, row: c.row, terrain: c.terrain || null, name: c.name || '', notes: c.notes || null,
            explored: !!c.explored
          };
        });
        version = data.version || 0;
        loadedVersion = version;
        var L2 = data.layer || {};
        fogLayerOn = !!L2.fog_enabled;
        party = (L2.party_col != null && L2.party_row != null) ? { col: L2.party_col, row: L2.party_row } : null;
        if (ctx.onHexFog) ctx.onHexFog(fogLayerOn);
        hatchDirty = true;
        anchorId = (data.layer && data.layer.anchor_drawing_id) || null;
        hidden = !!data.hidden;
        if (ctx.onHexAnchor) ctx.onHexAnchor(anchorId);
        loaded = true;
        fx = {};
        if (layerOn) fogView(); else syncImage();
        renderSoon();
        renderPanel();
      }).catch(function () { /* the lines still show; painting is just empty */ });
    }

    // ---- Pointer ----

    function pointToHex(e) {
      var ll = map.mouseEventToLatLng(e);
      return hexAtMap(layout, ll.lng, mapH - ll.lat);
    }

    function moveHover(e) {
      if (!active || !geo || !layout) { hoverPath.style.display = 'none'; return; }
      var h = pointToHex(e);
      if (!h) { hoverPath.style.display = 'none'; return; }
      var c = center(geo, h.col, h.row);
      hoverPath.setAttribute('d', hexD(c[0], c[1], geo.r));
      hoverPath.style.display = '';
    }

    // strokeKind is what a drag does in the current mode: paint terrain, reveal
    // or hide hexes, or nothing (Look, Party and Move pan the map).
    function strokeKind() {
      if (mode === 'paint' && canWrite) return 'paint';
      if (mode === 'fog' && canFog) return 'fog';
      return null;
    }

    // fogStroke applies the stroke's direction to one hex: the first hex
    // decides whether the drag reveals or hides, the rest follow it.
    function fogStroke(h) {
      if (stroke.value === undefined) stroke.value = !cellAt(h.col, h.row).explored;
      setExplored(h.col, h.row, stroke.value);
    }

    function onPointerDown(e) {
      var kind = strokeKind();
      if (!active || !layerOn || !kind || e.button !== 0) return;
      if (e.target.closest && e.target.closest('.leaflet-marker-icon, .leaflet-popup, .leaflet-control')) return;
      var h = pointToHex(e);
      if (!h) return;
      e.preventDefault();
      try { container.setPointerCapture(e.pointerId); } catch (err) { /* capture is a nicety */ }
      stroke = { last: h, kind: kind };
      if (kind === 'fog') fogStroke(h); else paintHex(h.col, h.row);
      renderSoon();
    }

    function onPointerMove(e) {
      moveHover(e);
      if (!stroke) return;
      var h = pointToHex(e);
      if (!h || (h.col === stroke.last.col && h.row === stroke.last.row)) return;
      hexLine(stroke.last, h).forEach(function (p) { if (stroke.kind === 'fog') fogStroke(p); else paintHex(p.col, p.row); });
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

    // Look mode: a click selects a hex and opens its card. Party mode: a click
    // moves the party there.
    function onMapClick(e) {
      if (!active || !layerOn) return;
      if (mode === 'party') {
        var ph = hexAtMap(layout, e.latlng.lng, mapH - e.latlng.lat);
        if (ph) moveParty(ph);
        return;
      }
      if (mode !== 'look') return;
      var h = hexAtMap(layout, e.latlng.lng, mapH - e.latlng.lat);
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

    function modes() {
      var m = [['look', 'Look']];
      if (canWrite) m.push(['paint', 'Paint']);
      if (canFog) m.push(['fog', 'Fog']);
      if (canParty) m.push(['party', 'Party']);
      return m;
    }

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
      if (mode === 'fog') {
        h += '<p>Click or drag to reveal hexes; drag over revealed ones to cover them again. Players see unexplored hexes under a moving shadow.</p>';
        if (!fogOn()) h += '<p>Fog of war is off, so players see every hex. Turn it on in Map settings.</p>';
        h += '<button type="button" class="mp-chip" id="mp-hx-reset">Cover everything again</button>';
      }
      if (mode === 'party') h += '<p>Click a hex to move the party. The hexes around them are revealed for everyone.</p>';
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
      var reset = document.getElementById('mp-hx-reset');
      if (reset) reset.onclick = resetFog;
      if (mode === 'look' && selected) fillCard();
    }

    // resetFog covers every hex again, after asking: it undoes all exploring.
    function resetFog() {
      if (!window.confirm('Cover every hex again? Players will see them all as unexplored.')) return;
      flush();
      saving = saving.then(function () {
        return Chronicle.apiFetch(base + '/fog', { method: 'POST', body: { reset: true } }).then(function (res) {
          if (res.ok) return res.json().then(function (r) { if (r && r.version) { own[r.version] = true; } });
          return res.json().catch(function () { return {}; }).then(function (err) { Chronicle.notify(err.message || 'Could not cover the hexes', 'error'); });
        }).catch(function () { Chronicle.notify('Could not cover the hexes', 'error'); });
      }).then(function () { return load(); });
    }

    // fillCard builds the open hex's card. Names and notes go in as values and
    // text, never as markup, because people type them.
    function fillCard() {
      var card = document.getElementById('mp-hx-card');
      if (!card) return;
      var p = selected.split(',');
      var c = cells[selected] || { col: +p[0], row: +p[1], name: '', notes: null };
      // Unexplored land has no card for people who cannot see it; the server
      // sent them nothing about it either.
      if (fogOn() && !seesAll && !(cells[selected] && cells[selected].explored && !heldBack(selected))) {
        var ut = document.createElement('b');
        ut.textContent = 'Unexplored';
        var ub = document.createElement('span');
        ub.textContent = 'The party hasn’t been here yet.';
        card.appendChild(ut);
        card.appendChild(ub);
        return;
      }
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
        if (canFog) {
          var lab = document.createElement('label');
          lab.className = 'mp-check';
          var lt = document.createElement('span');
          lt.textContent = 'Explored';
          var cb = document.createElement('input');
          cb.type = 'checkbox'; cb.id = 'mp-hx-ex'; cb.checked = !!c.explored;
          cb.onchange = function () {
            if (setExplored(+p[0], +p[1], cb.checked)) { scheduleFlush(FLUSH_MS); renderSoon(); }
          };
          lab.appendChild(lt);
          lab.appendChild(cb);
          card.appendChild(lab);
        }
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
      var paintOn = active && layerOn && !!strokeKind();
      // While painting or fogging, a drag paints rather than pans; Look, Party and Move pan.
      if (paintOn && !dragDisabled) { map.dragging.disable(); dragDisabled = true; }
      if (!paintOn && dragDisabled) { map.dragging.enable(); dragDisabled = false; }
      if (active && layerOn) container.style.cursor = paintOn ? 'crosshair' : '';
      if (ctx.setHint && active) {
        ctx.setHint({
          paint: 'Click or drag to paint terrain', fog: 'Click or drag to reveal or cover hexes',
          party: 'Click a hex to move the party'
        }[mode] || 'Click a hex to see it');
      }
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

    // wantedAnchor is the picture the field should cover right now: the owner's
    // unsaved choice in the settings sheet while it is open (it previews live),
    // otherwise what the server holds.
    function wantedAnchor() {
      var D = display();
      return Object.prototype.hasOwnProperty.call(D, 'hex_anchor') ? D.hex_anchor : anchorId;
    }

    // off takes the field down. keepLines keeps the viewer's plain grid away as
    // well, for a layer that exists but may not be shown to this viewer: the
    // plain hex grid would give away the very thing being withheld.
    function off(keepLines) {
      var wasActive = active;
      layerOn = false;
      stopWalk();
      setActive(false);
      // The viewer's own tool state must not keep pointing at a tool that is gone.
      if (wasActive && ctx.setTool) ctx.setTool('move');
      removeOverlay();
      showTool(false);
      fogView();
      if (ctx.onHexLines) ctx.onHexLines(!!keepLines);
      renderPanel();
    }

    // placeSoon coalesces picture and settings changes to one placement a frame,
    // so dragging a picture does not rebuild the field on every pointer event.
    function placeSoon() {
      if (placePending) return;
      placePending = true;
      requestAnimationFrame(function () { placePending = false; if (loaded && display().grid_type === 'hex' && !hidden) place(); });
    }

    // place puts the field where it belongs. If only the picture's position or
    // size changed it moves the group; anything else rebuilds the overlay.
    function place() {
      var D = display();
      var aid = wantedAnchor();
      var box = null;
      if (aid) {
        var pic = ctx.pictures ? ctx.pictures.get(aid) : null;
        box = pic ? boxFromPoints(pic.points, mapW, mapH) : null;
        if (!box) {
          // The picture is not known yet (the drawing module is still loading)
          // or it is gone. Once drawings are in, a missing server anchor is
          // re-read once: the server then answers with the whole map.
          off(true);
          if (ctx.pictures && ctx.pictures.ready() && aid === anchorId && reloadedFor !== aid) {
            reloadedFor = aid;
            load().then(function () { if (loaded) refresh(); });
          }
          return;
        }
      }
      var next = layoutFor(D.grid_size || 50, box, mapW, mapH);
      var g = next.geo;
      if (!box && !(g.r >= 1)) { off(); return; }
      if (g.cols * g.rows > MAX_POSITIONS || !(g.cols > 0)) { off(); return; }
      var sig = [D.grid_size, D.grid_strength, g.r, g.cols, g.rows].join('|');
      layout = next;
      geo = g;
      if (overlay && sig === placedSig) { applyTransform(); return; }
      placedSig = sig;
      layerOn = true;
      buildOverlay();
      showTool(true);
      fogView();
      if (ctx.onHexLines) ctx.onHexLines(true);
      renderSoon();
      renderPanel();
      if (pendingOpen) {
        mode = pendingOpen === 'paint' && canWrite ? 'paint' : 'look';
        pendingOpen = null;
        if (ctx.setTool) ctx.setTool('hex');
      }
    }

    // refresh follows the map's display settings: it turns the layer on or off,
    // and re-lays the hexes when the grid size or strength changes (the owner's
    // settings sheet previews them live). The layer is read first, because
    // where it sits depends on which picture it covers.
    function refresh() {
      if (display().grid_type !== 'hex') { off(); return; }
      if (!loaded) {
        // One attempt: if the read fails the viewer's plain grid stays, and
        // the slider does not hammer a server that is not answering.
        if (loading || loadTried) return;
        loading = loadTried = true;
        load().then(function () { loading = false; if (loaded) refresh(); });
        return;
      }
      if (hidden) { off(true); return; }
      bindPictures();
      place();
      // The settings sheet previews the fog switch live.
      var f = fogOn();
      if (f !== lastFog) { lastFog = f; if (layerOn) fogView(); }
    }

    // onChanged follows a "hex.changed" live event: a version and maybe the
    // party's path, never hex contents. The path (only sent when it is safe for
    // this viewer) is played; the filtered read is fetched afresh. An event
    // that tells nothing new, or the echo of this client's own write, is
    // ignored; one that lands while edits are unsaved waits for them, so it
    // cannot wipe the screen.
    function onChanged(msg) {
      if (!msg || String(msg.map_id) !== String(ctx.mapID) || !loaded) return;
      if (isStale(loadedVersion, msg.version, own)) return;
      if (msg.party_path && msg.party_path.length) applyParty(msg.party_path, msg.version);
      if (stroke || flushTimer || fogQ || pendingSaves || Object.keys(queue).length) { remoteWaiting = true; return; }
      load();
    }

    function bindPictures() {
      if (picUnsub || !ctx.pictures) return;
      picUnsub = ctx.pictures.subscribe(placeSoon);
    }

    var handle = {
      refresh: refresh,
      // bindPictures is called by the drawing module once it has the pictures,
      // which may be after this module started.
      bindPictures: function () { bindPictures(); if (loaded) placeSoon(); },
      anchorId: function () { return anchorId; },
      hasCells: function () { return Object.keys(cells).length > 0; },
      // reloadLayer re-reads the layer (after the cover was changed) and lays
      // the field out again.
      reloadLayer: function () { reloadedFor = null; loadTried = true; return load().then(function () { if (loaded) refresh(); }); },
      // open shows the Hexes tool in the given mode as soon as the field is up.
      open: function (m) {
        pendingOpen = m;
        if (!layerOn) return;
        pendingOpen = null;
        mode = m === 'paint' && canWrite ? 'paint' : 'look';
        if (ctx.setTool) ctx.setTool('hex');
      },
      setActive: setActive,
      isOn: function () { return layerOn; },
      onChanged: onChanged,
      // resync reads the layer again, after the live connection was lost and
      // events may have been missed.
      resync: function () { if (loaded && !stroke && !flushTimer && !fogQ && !pendingSaves) load(); },
      // shadowReady is called once the shadow module is loaded, which may be
      // after this module started: the smoke can then be set up.
      shadowReady: function () { if (layerOn) fogView(); },
      // escape steps back from an open card; false means nothing was open.
      escape: function () {
        if (active && selected) { selected = null; renderSoon(); renderPanel(); return true; }
        return false;
      },
      destroy: function () {
        window.removeEventListener('pagehide', onPageHide);
        if (picUnsub) { picUnsub(); picUnsub = null; }
        flush();
        clearTimeout(fxTimer);
        revealTimers.forEach(clearTimeout);
        stopWalk();
        if (shadow) { if (smokeSet) shadow.setHexFog(null); smokeSet = false; shadow.destroy(); shadow = null; }
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
