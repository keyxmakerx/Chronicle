/**
 * map_hex_look.js -- how the hex layer's flat parts look, shared by the map
 * page (map_hexes.js) and the AI review's picture of a hex map
 * (static/js/widgets/hex_map_preview.js): the terrain kinds and their flat
 * colours, the Simple icons, the hex outline, hex centres, and the road, trail
 * and river lines with the stroke each art style gives them. Everything here
 * is pure (strings and numbers in, strings out) and needs no Leaflet, so a page
 * that only wants to show a hex map loads this and map_hex_art.js and nothing
 * else. The Detailed and Realistic tiles are map_hex_art.js.
 */
(function () {
  'use strict';

  var SQRT3 = Math.sqrt(3);

  function key(col, row) { return col + ',' + row; }

  // The terrain kinds the server allows, with the label and flat colour of the
  // palette. Town is shown to people as "Places".
  var TERRAINS = [
    ['plains', 'Plains', '#c9d58a'], ['forest', 'Forest', '#5f8f4a'], ['hills', 'Hills', '#b8a46a'],
    ['mountain', 'Mountains', '#8d8478'], ['water', 'Water', '#6f9fc0'], ['swamp', 'Swamp', '#6f7d55'],
    ['desert', 'Desert', '#e3cc8c'], ['snow', 'Snow', '#e8eef2'],
    ['jungle', 'Jungle', '#3f8f6a'], ['farmland', 'Farmland', '#d6c463'], ['scrub', 'Scrubland', '#b3a771'],
    ['badlands', 'Badlands', '#cf8f63'], ['waste', 'Wasteland', '#7a6f6a'], ['volcanic', 'Volcanic', '#8c4638'],
    ['coast', 'Coast', '#bfe0cf'], ['deep', 'Deep sea', '#3d5f92'], ['ice', 'Ice', '#bfe1ee'],
    ['cavern', 'Caverns', '#5a5470'], ['town', 'Places', '#c4a27a'],
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
    plains: '<path d="M-6 6L-8-1M-4 6V-3M-2 6L0 0M3 6L2-2M5 6V-4M7 6L9 0" stroke="#5a7530" stroke-width="1.4" stroke-linecap="round" fill="none"/>',
    jungle: '<path d="M0 7Q1.6 1-.6-4" fill="none" stroke="#5b3f24" stroke-width="1.8" stroke-linecap="round"/><path d="M-.6-4q-5-4.4-9.4.4M-.6-4q5-4.4 9.4.4M-.6-4q-3.2-5.6-7.4-5M-.6-4q3.2-5.6 7.4-5" fill="none" stroke="#1f6b3a" stroke-width="2.2" stroke-linecap="round"/>',
    farmland: '<path d="M-11 5L-5-5H11L5 5z" fill="#d2b84f" stroke="#7a6a2a" stroke-width="1" stroke-linejoin="round"/><path d="M-6.4 5L-.4-5M-1.6 5L4.4-5" fill="none" stroke="#7a6a2a" stroke-width="1"/>',
    scrub: '<circle cx="-4.5" cy="2.4" r="4" fill="#77803f"/><circle cx="1.4" cy="1" r="5.2" fill="#8d9550" stroke="#4f5628" stroke-width="1"/><path d="M7.4 6.4L9.2 1M10 6.4V2.6" fill="none" stroke="#5a6230" stroke-width="1.4" stroke-linecap="round"/>',
    badlands: '<path d="M-11 7L-7.6-6H7.6L11 7z" fill="#c07a52" stroke="#6e3f27" stroke-width="1" stroke-linejoin="round"/><path d="M-9-.6H9M-10 3.2H10" fill="none" stroke="#6e3f27" stroke-width=".9"/>',
    waste: '<path d="M0 7V-8M0-1L-5.4-6.4M0-3.6L5-8.6M0 2.2L4.2-.8" fill="none" stroke="#3f3632" stroke-width="1.8" stroke-linecap="round"/><path d="M-8 7h16" stroke="#3f3632" stroke-width="1.5" stroke-linecap="round"/>',
    volcanic: '<path d="M-11 7L-3.4-6h6.8L11 7z" fill="#4f3d3d" stroke="#241a1a" stroke-width="1" stroke-linejoin="round"/><path d="M-3.4-6h6.8L2-2.2.4-4.4-1.2-1z" fill="#ff7a1a"/><path d="M0-7.4q-2.2-1.8 0-4" fill="none" stroke="#6b5a5a" stroke-width="1.4" stroke-linecap="round"/>',
    coast: '<path d="M-11 7Q-4-7 11-3V7z" fill="#eedfa9" stroke="#a5874a" stroke-width="1" stroke-linejoin="round"/><path d="M-10-5.4q2-3 4 0t4 0" fill="none" stroke="#2f6488" stroke-width="1.6" stroke-linecap="round"/>',
    deep: '<path d="M0 3Q0-2.4-6-7Q-1.4-6 0-2.6Q1.4-6 6-7Q0-2.4 0 3z" fill="#1d3558"/><path d="M-9 6.4q2.25-3 4.5 0t4.5 0t4.5 0t4.5 0" fill="none" stroke="#1d3558" stroke-width="1.7" stroke-linecap="round"/>',
    ice: '<path d="M-7 7L-2.6-8L2 7z" fill="#a4d3e8" stroke="#4d7f99" stroke-width="1" stroke-linejoin="round"/><path d="M1 7L5-2.6L9 7z" fill="#d2ecf6" stroke="#4d7f99" stroke-width="1" stroke-linejoin="round"/>',
    cavern: '<path d="M-10 7Q-10-8 0-8Q10-8 10 7z" fill="#6d6680" stroke="#2e2a3a" stroke-width="1" stroke-linejoin="round"/><path d="M-4.4 7Q-4.4-2.4 0-2.4Q4.4-2.4 4.4 7z" fill="#231f2e"/>'
  };

  // Icons are drawn a little larger than a hex's radius divided by 26, as the
  // approved design does, so they fill the hex without touching its edge.
  var ICON_SCALE = 1.55 / 26;

  // hexD is the outline of a hex with corner radius r centred on (cx, cy).
  function hexD(cx, cy, r) {
    var d = '';
    for (var i = 0; i < 6; i++) {
      var a = Math.PI / 3 * i - Math.PI / 2;
      d += (i ? 'L' : 'M') + (cx + r * Math.cos(a)).toFixed(1) + ' ' + (cy + r * Math.sin(a)).toFixed(1);
    }
    return d + 'Z';
  }

  // center is the centre of hex (col,row) in map pixels.
  function center(g, col, row) {
    return [g.ox + col * g.w + ((row & 1) ? g.w / 2 : 0), g.oy + row * 1.5 * g.r];
  }

  // noise01 is a steady number from 0 to 1 for a hex, so a rough edge or a
  // river's bends come out the same on every draw.
  function noise01(col, row, salt) {
    var n = Math.sin(col * 127.1 + row * 311.7 + (salt || 0) * 74.7) * 43758.5453;
    return n - Math.floor(n);
  }

  // ---- Lines (pure) ----
  //
  // A hex carries a line as a number 0..63, or null for none: bit i says the
  // line leaves the hex through side i. Sides run east first and clockwise,
  // the order of Neighbors in hex.go. 0 is a line on the hex with no exit,
  // drawn as a dot.

  var SIDE_DCOL = [[1, 0, -1, -1, -1, 0], [1, 1, 0, -1, 0, 1]];  // [even row, odd row]
  var SIDE_DROW = [0, 1, 1, 0, -1, -1];

  // sideNeighbor is the hex across side i of (col,row).
  function sideNeighbor(col, row, i) {
    return { col: col + SIDE_DCOL[row & 1][i], row: row + SIDE_DROW[i] };
  }

  // sideToward is which side of a faces the hex b, or -1 if b is not next to a.
  function sideToward(a, b) {
    for (var i = 0; i < 6; i++) {
      var n = sideNeighbor(a.col, a.row, i);
      if (n.col === b.col && n.row === b.row) return i;
    }
    return -1;
  }

  // oppositeSide is the side of the neighbour that faces back.
  function oppositeSide(i) { return (i + 3) % 6; }

  // linePath draws every hex of cells (an object keyed "col,row") that carries
  // the line of kind, as one path. A hex's point is its middle, nudged by bend
  // (a fraction of a radius) so rivers and trails wander. Each exit ends at
  // halfway to the neighbour's point where the neighbour carries the line too,
  // else at the middle of the shared edge. One exit is a straight run, two a
  // curve through the point, more a curve from the first exit to each other.
  function linePath(g, cells, kind, bend) {
    var d = '';
    function pt(col, row) {
      var p = center(g, col, row);
      if (!bend) return p;
      return [p[0] + (noise01(col, row, 1) - 0.5) * bend * g.r, p[1] + (noise01(col, row, 2) - 0.5) * bend * g.r];
    }
    function f(n) { return n.toFixed(1); }
    for (var k in cells) {
      var c = cells[k], v = c[kind];
      if (v == null) continue;
      var p = pt(c.col, c.row), here = center(g, c.col, c.row), joins = [];
      for (var i = 0; i < 6; i++) {
        if (!(v & (1 << i))) continue;
        var n = sideNeighbor(c.col, c.row, i), nc = cells[key(n.col, n.row)];
        var a = p, b = pt(n.col, n.row);
        if (!(nc && nc[kind] != null)) { a = here; b = center(g, n.col, n.row); }
        joins.push([(a[0] + b[0]) / 2, (a[1] + b[1]) / 2]);
      }
      if (!joins.length) d += 'M' + f(p[0]) + ' ' + f(p[1]) + 'h0.01';
      else if (joins.length === 1) d += 'M' + f(p[0]) + ' ' + f(p[1]) + 'L' + f(joins[0][0]) + ' ' + f(joins[0][1]);
      else for (var j = 1; j < joins.length; j++) d += 'M' + f(joins[0][0]) + ' ' + f(joins[0][1]) + 'Q' + f(p[0]) + ' ' + f(p[1]) + ' ' + f(joins[j][0]) + ' ' + f(joins[j][1]);
    }
    return d;
  }

  // ROAD_CORNER is how far from the hex's middle, as a fraction of the way to
  // the exit, a road's corner starts.
  var ROAD_CORNER = 0.45;

  // roadLine draws every hex of cells that carries a road as one path. A built
  // road runs straight and then turns, which sets it apart from the wandering
  // river and trail: one exit is a straight run from the middle to the shared
  // edge, two are straight runs joined by a short corner at the middle, more
  // are straight spokes meeting there.
  function roadLine(g, cells) {
    var d = '';
    function f(n) { return n.toFixed(1); }
    function xy(p) { return f(p[0]) + ' ' + f(p[1]); }
    for (var k in cells) {
      var c = cells[k], v = c.road;
      if (v == null) continue;
      var p = center(g, c.col, c.row), joins = [];
      for (var i = 0; i < 6; i++) {
        if (!(v & (1 << i))) continue;
        var n = sideNeighbor(c.col, c.row, i), q = center(g, n.col, n.row);
        joins.push([(p[0] + q[0]) / 2, (p[1] + q[1]) / 2]);
      }
      if (!joins.length) d += 'M' + xy(p) + 'h0.01';
      else if (joins.length === 1) d += 'M' + xy(p) + 'L' + xy(joins[0]);
      else if (joins.length === 2) {
        var j0 = joins[0], j1 = joins[1];
        var a = [p[0] + (j0[0] - p[0]) * ROAD_CORNER, p[1] + (j0[1] - p[1]) * ROAD_CORNER];
        var b = [p[0] + (j1[0] - p[0]) * ROAD_CORNER, p[1] + (j1[1] - p[1]) * ROAD_CORNER];
        d += 'M' + xy(j0) + 'L' + xy(a) + 'Q' + xy(p) + ' ' + xy(b) + 'L' + xy(j1);
      } else for (var j = 0; j < joins.length; j++) d += 'M' + xy(joins[j]) + 'L' + xy(p);
    }
    return d;
  }

  // linesMarkup draws the three lines over the land: river under trail
  // under road, in the stroke of the map's art.
  function linesMarkup(geo, cells, amode, kk0) {
    var f = '', rv = linePath(geo, cells, 'river', 0.55), tr = linePath(geo, cells, 'trail', 0.5), rd = roadLine(geo, cells);
    var kw = amode === 'real' ? 1.45 : 1;
    function w(n) { return (n * kk0 * kw).toFixed(2); }
    if (rv) {
      if (amode === 'simple') f += '<path d="' + rv + '" fill="none" stroke="#3f7fb5" stroke-width="' + w(3) + '" stroke-linecap="round" stroke-linejoin="round"/>';
      else {
        f += '<path d="' + rv + '" fill="none" stroke="' + (amode === 'real' ? '#1f4468' : '#2f5d86') + '" stroke-width="' + w(6.4) + '" stroke-linecap="round" stroke-linejoin="round" opacity=".9"/>' +
          '<path d="' + rv + '" fill="none" stroke="#5fa3d4" stroke-width="' + w(4.4) + '" stroke-linecap="round" stroke-linejoin="round"/>' +
          '<path d="' + rv + '" fill="none" stroke="#d6ebf8" stroke-width="' + w(0.9) + '" stroke-linecap="round" stroke-dasharray="' + w(4) + ' ' + w(6) + '" opacity=".8"/>';
      }
    }
    if (tr) {
      f += '<path d="' + tr + '" fill="none" stroke="#f4e8c9" stroke-width="' + w(3.4) + '" stroke-linecap="round" opacity="' + (amode === 'real' ? '.85' : '.5') + '"/>' +
        '<path d="' + tr + '" fill="none" stroke="' + (amode === 'simple' ? '#7a5a35' : '#5a3f22') + '" stroke-width="' + w(1.7) + '" stroke-linecap="round" stroke-dasharray="' + w(0.1) + ' ' + w(3.6) + '"/>';
    }
    if (rd) f += roadMarkup(rd, amode, kk0);
    return f;
  }

  // roadMarkup strokes a road path as a band of packed earth with a soft
  // edge; it deliberately has no dashes and no dark outline.
  function roadMarkup(rd, amode, kk0) {
    var k = amode === 'real' ? 1.3 : 1;
    function w(n) { return (n * kk0 * k).toFixed(2); }
    function stroke(col, width, extra) {
      return '<path d="' + rd + '" fill="none" stroke="' + col + '" stroke-width="' + w(width) + '" stroke-linecap="round" stroke-linejoin="round"' + (extra || '') + '/>';
    }
    if (amode === 'simple') return stroke('#b08a58', 3.2);
    if (amode === 'real') return stroke('#5f4729', 5.6, ' opacity=".5"') + stroke('#c4a676', 4.5);
    return stroke('#9a7848', 5.3) + stroke('#d3b98a', 4.4);
  }

  // simpleArt is the Simple look of one hex of a terrain: its flat colour and
  // icon. Road is a line, so it has no art.
  function simpleArt(t, cx, cy, r) {
    if (!t || t === 'road') return '';
    var s = r * ICON_SCALE;
    return '<path d="' + hexD(cx, cy, r) + '" fill="' + COLOR[t] + '" fill-opacity=".4"/>' +
      (SIMPLE[t] ? '<g transform="translate(' + cx.toFixed(1) + ' ' + cy.toFixed(1) + ') scale(' + s.toFixed(4) + ')">' + SIMPLE[t] + '</g>' : '');
  }

  var api = {
    TERRAINS: TERRAINS, COLOR: COLOR, SIMPLE: SIMPLE, ICON_SCALE: ICON_SCALE, hexD: hexD, center: center, noise01: noise01,
    sideNeighbor: sideNeighbor, sideToward: sideToward, oppositeSide: oppositeSide,
    linePath: linePath, roadLine: roadLine, linesMarkup: linesMarkup, roadMarkup: roadMarkup, simpleArt: simpleArt
  };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  var root = typeof window !== 'undefined' ? window : globalThis;
  root.ChronicleHexLook = api;
})();
