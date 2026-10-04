/**
 * shop_room_paper.js — the shop room's paper look: the same room the shop
 * room engine lays out (static/js/widgets/shop_room.js), drawn as a pop-up
 * book. Every furniture piece, good and decoration is a cut-card piece that
 * folds up off an open book, with paper grain, a card edge, soft shadows on
 * the page that fall away from the room's warm lights, and small living
 * details (flames, swaying lanterns, a breathing shopkeeper).
 *
 * The shop room widget calls ShopRoomPaper.render(host, model, opts) after
 * each draw when the shop's look is "paper". Positions are the engine's floor
 * units, so arranging in either look moves the same saved room. The widget
 * keeps all behaviour (buying, arranging, saving); this file only draws and
 * answers where a pointer is on the floor (floorAt) or which spot it is
 * nearest (spotAt).
 *
 * Coordinates: floor x runs left to right along the book, floor y from the
 * back wall toward the viewer. The engine's Y wall (along x at y=0) is the
 * book's back wall; its X wall (along y at x=0) is a side wall on the left.
 */
(function () {
  'use strict';

  var INK = '#2a1f17';
  // The book's floor is T px per tile; heights are Z px per engine height unit.
  var T = 60, Z = 52;

  function esc(s) { return String(s).replace(/[&<>"']/g, function (c) { return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]; }); }
  function hex(c) { c = String(c || '#888888').replace('#', ''); if (c.length === 3) c = c.replace(/./g, '$&$&'); return [parseInt(c.slice(0, 2), 16), parseInt(c.slice(2, 4), 16), parseInt(c.slice(4, 6), 16)]; }
  function rgb(a) { return '#' + a.map(function (v) { v = Math.max(0, Math.min(255, Math.round(v))); return (v < 16 ? '0' : '') + v.toString(16); }).join(''); }
  function dk(c, f) { return rgb(hex(c).map(function (v) { return v * (1 - f); })); }
  function lt(c, f) { return rgb(hex(c).map(function (v) { return v + (255 - v) * f; })); }
  function n1(v) { return Math.round(v * 10) / 10; }

  // One cut-card shape: a darker copy behind for the card's edge, the face,
  // then paper grain and top-lit shading over it.
  function card(d, fill, sw) {
    return '<path d="' + d + '" transform="translate(2 2)" fill="' + dk(fill, .42) + '"/>' +
      '<path d="' + d + '" fill="' + fill + '" stroke="' + INK + '" stroke-width="' + (sw || 1.8) + '" stroke-linejoin="round"/>' +
      '<path d="' + d + '" fill="url(#srp-fib)" opacity=".45"/><path d="' + d + '" fill="url(#srp-sh)"/>';
  }
  function R(x, y, w, h) { return 'M' + n1(x) + ' ' + n1(y) + 'h' + n1(w) + 'v' + n1(h) + 'h' + n1(-w) + 'z'; }
  function line(x1, y1, x2, y2, c, w) { return '<path d="M' + n1(x1) + ' ' + n1(y1) + 'L' + n1(x2) + ' ' + n1(y2) + '" stroke="' + c + '" stroke-width="' + (w || 1.2) + '" fill="none"/>'; }
  function svg(w, h, body, cls) { return '<svg class="' + (cls || '') + '" viewBox="-3 -3 ' + n1(w + 6) + ' ' + n1(h + 6) + '" aria-hidden="true">' + body + '</svg>'; }

  // ---- Paper drawings, one per furniture kind; w and h are the card's px size ----
  var KIND = {};
  function frameShelves(w, h, C, rows) {
    var s = card(R(0, 0, w, h), C.wood) + '<path d="' + R(6, 6, w - 12, h - 12) + '" fill="' + dk(C.wood, .45) + '" stroke="' + INK + '" stroke-width="1"/>';
    for (var i = 1; i <= rows; i++) s += card(R(3, i * (h - 8) / rows, w - 6, 5), lt(C.wood, .1), 1.2);
    return s;
  }
  KIND.shelf = function (w, h, C) { return frameShelves(w, h, C, 3); };
  KIND.bookcase = function (w, h, C) {
    var s = frameShelves(w, h, C, 4), cols = ['#8f3b3b', '#2f4f3d', '#3b4f7a', '#7a5a2a', '#5b2c5c', '#a37a4e'];
    for (var r = 0; r < 4; r++) {
      var y = (r + 1) * (h - 8) / 4, x = 9, k = r * 3;
      while (x < w - 14) { var bw = 5 + (k * 7 % 4), bh = (h - 8) / 4 - 6 - (k * 5 % 5); s += card(R(x, y - bh, bw, bh), cols[k % cols.length], 1) + line(x + 1, y - bh + 4, x + bw - 1, y - bh + 4, '#e8c47c', .8); x += bw + 1.5; k++; }
    }
    return s;
  };
  KIND.cabinet = function (w, h, C) {
    var s = card(R(0, 0, w, h), C.wood), dw = (w - 18) / 2;
    [6, 12 + dw].forEach(function (x) { s += card(R(x, 8, dw, h * .62), '#cfe3ec', 1.4) + line(x + 4, 12, x + dw * .5, h * .4, '#ffffff', 1.4) + '<circle cx="' + n1(x + (x === 6 ? dw - 5 : 5)) + '" cy="' + n1(h * .4) + '" r="2" fill="#c9a24e" stroke="' + INK + '" stroke-width=".8"/>'; });
    s += card(R(6, h * .72, w - 12, h * .2), dk(C.wood, .15), 1.2);
    return s;
  };
  KIND.rack = function (w, h, C) {
    var s = card(R(2, 0, 7, h), C.wood) + card(R(w - 9, 0, 7, h), C.wood);
    [h * .22, h * .55, h * .86].forEach(function (y) { s += card(R(0, y, w, 6), lt(C.wood, .08), 1.2); });
    return s;
  };
  KIND.forge = function (w, h, C) {
    var st = '#8d8780', s = card(R(w * .2, 0, w * .6, h * .55), dk(st, .15));
    s += card('M0 ' + n1(h) + 'V' + n1(h * .5) + 'H' + n1(w) + 'V' + n1(h) + 'Z', st);
    for (var y = h * .58; y < h; y += 9) s += line(2, y, w - 2, y, dk(st, .3), .8);
    var mx = w / 2, my = h * .9;
    s += card('M' + n1(mx - 22) + ' ' + n1(my) + 'v-18a22 18 0 0 1 44 0v18z', '#3a1607', 1.4);
    s += '<g class="srp-fire"><path class="srp-flame" d="M' + n1(mx - 10) + ' ' + n1(my) + 'c-6-8-2-16 3-22 2 6 6 8 6 13 2-4 1-8 1-10 6 6 7 15-10 19z" fill="#ff8a2a"/>' +
      '<path class="srp-flame b" d="M' + n1(mx + 2) + ' ' + n1(my) + 'c-5-6-3-13 2-18 4 6 7 12-2 18z" fill="#ffd36a"/>' +
      '<path class="srp-flame" style="animation-delay:-.2s" d="M' + n1(mx + 10) + ' ' + n1(my) + 'c-4-5-2-10 1-13 3 4 5 8-1 13z" fill="#ff7a1a"/></g>';
    for (var i = 0; i < 6; i++) s += '<circle class="srp-ember" style="--dx:' + ((i * 7) % 13 - 6) + 'px;animation-delay:-' + (i * .43).toFixed(2) + 's" cx="' + n1(mx - 8 + i * 3) + '" cy="' + n1(my - 14) + '" r="1.1" fill="#ffcf6a"/>';
    return s;
  };
  KIND.window = function (w, h, C) {
    var y0 = h * .2, wh = h * .5, s = card(R(4, y0, w - 8, wh), '#9ccbe3', 1.6);
    s += '<path d="M8 ' + n1(y0 + wh * .72) + 'c' + n1(w * .2) + '-8 ' + n1(w * .4) + '-4 ' + n1(w * .5) + ' 0s' + n1(w * .25) + ' 4 ' + n1(w * .32) + '-3v' + n1(wh * .28 - 2) + 'h' + n1(-(w - 16)) + 'z" fill="#7fb07a"/>';
    s += line(w / 2, y0, w / 2, y0 + wh, INK, 2.4) + line(4, y0 + wh / 2, w - 4, y0 + wh / 2, INK, 2.4);
    s += card('M0 ' + n1(y0 - 4) + 'h' + n1(w * .22) + 'c-2 ' + n1(wh * .4) + ' 4 ' + n1(wh * .7) + '-3 ' + n1(wh + 6) + 'h' + n1(-w * .19) + 'z', '#b8494a', 1.3);
    s += card('M' + n1(w) + ' ' + n1(y0 - 4) + 'h' + n1(-w * .22) + 'c2 ' + n1(wh * .4) + '-4 ' + n1(wh * .7) + ' 3 ' + n1(wh + 6) + 'h' + n1(w * .19) + 'z', '#b8494a', 1.3);
    s += card(R(-2, y0 + wh, w + 4, 5), dk('#a37a4e', .1), 1.2);
    return s;
  };
  KIND.herbs = function (w, h) {
    var s = '<path d="M2 4c' + n1(w * .3) + ' 10 ' + n1(w * .7) + ' 10 ' + n1(w - 4) + ' 0" fill="none" stroke="' + INK + '" stroke-width="1.2"/>';
    [.12, .3, .5, .7, .88].forEach(function (f, i) { var x = w * f, y = 6 + Math.sin(f * Math.PI) * 6; s += '<g class="srp-sway" style="animation-delay:-' + (i * .4) + 's">' + line(x, y, x, y + 8, INK, 1) + card('M' + n1(x) + ' ' + n1(y + 8) + 'c-6 5-6 14 0 19 6-5 6-14 0-19z', i % 2 ? '#6e8f4e' : '#8aa35a', 1) + '</g>'; });
    return s;
  };
  KIND.counter = function (w, h, C) {
    var s = card(R(4, 12, w - 8, h - 12), C.wood);
    for (var x = 12; x < w - 30; x += 46) s += card('M' + n1(x) + ' ' + n1(h - 4) + 'v' + n1(-(h - 30)) + 'a18 12 0 0 1 36 0v' + n1(h - 30) + 'z', dk(C.wood, .12), 1.2) + '<circle cx="' + n1(x + 18) + '" cy="' + n1(h * .6) + '" r="2.4" fill="#e8c47c" stroke="' + INK + '" stroke-width=".8"/>';
    s += card(R(0, 0, w, 13), dk(C.wood, .08));
    return s;
  };
  KIND.table = function (w, h, C) {
    return card(R(6, 10, 7, h - 10), dk(C.wood, .1), 1.3) + card(R(w - 13, 10, 7, h - 10), dk(C.wood, .1), 1.3) + card(R(0, 0, w, 11), lt(C.wood, .06));
  };
  KIND.barrel = function (w, h) {
    var s = card('M' + n1(w * .1) + ' 0c' + n1(-w * .12) + ' ' + n1(h * .35) + ' ' + n1(-w * .12) + ' ' + n1(h * .65) + ' 0 ' + n1(h) + 'h' + n1(w * .8) + 'c' + n1(w * .12) + ' ' + n1(-h * .35) + ' ' + n1(w * .12) + ' ' + n1(-h * .65) + ' 0 ' + n1(-h) + 'z', '#9c6a3a');
    [.15, .5, .85].forEach(function (f) { s += line(w * .04, h * f, w * .96, h * f, '#4a4a4a', 3); });
    s += line(w * .35, 2, w * .33, h - 2, '#6b4e2e', .9) + line(w * .65, 2, w * .67, h - 2, '#6b4e2e', .9);
    return s;
  };
  KIND.crate = function (w, h) {
    return card(R(0, 0, w, h), '#b8915e') + line(2, 2, w - 2, h - 2, '#8a6a44', 2.4) + line(w - 2, 2, 2, h - 2, '#8a6a44', 2.4) + line(0, h * .5, w, h * .5, '#8a6a44', 1.2);
  };
  KIND.sack = function (w, h) {
    return card('M' + n1(w * .15) + ' ' + n1(h) + 'C' + n1(-w * .05) + ' ' + n1(h * .4) + ' ' + n1(w * .2) + ' ' + n1(h * .1) + ' ' + n1(w * .4) + ' ' + n1(h * .12) + 'l' + n1(w * .1) + '-8 ' + n1(w * .1) + ' 8C' + n1(w * .8) + ' ' + n1(h * .1) + ' ' + n1(w * 1.05) + ' ' + n1(h * .4) + ' ' + n1(w * .85) + ' ' + n1(h) + 'z', '#d8c39a') + line(w * .38, h * .18, w * .62, h * .18, '#5b4630', 2);
  };
  KIND.anvil = function (w, h) {
    var s = card(R(w * .3, h * .45, w * .4, h * .55), '#8a6a45');
    s += card('M' + n1(w * .05) + ' ' + n1(h * .12) + 'h' + n1(w * .7) + 'l' + n1(w * .25) + ' ' + n1(h * .08) + 'l' + n1(-w * .25) + ' ' + n1(h * .1) + 'h' + n1(-w * .1) + 'l-6 ' + n1(h * .18) + 'h' + n1(-w * .3) + 'l-6 ' + n1(-h * .18) + 'h' + n1(-w * .1) + 'z', '#5a6068');
    for (var i = 0; i < 6; i++) { var a = -Math.PI * (.15 + i * .13), r = 12 + (i * 7 % 10); s += '<circle class="srp-spark" style="--dx:' + Math.round(Math.cos(a) * r) + 'px;--dy:' + Math.round(Math.sin(a) * r) + 'px;animation-delay:-' + (i * .05) + 's" cx="' + n1(w * .45) + '" cy="' + n1(h * .12) + '" r="1.1" fill="#ffe08a"/>'; }
    return s;
  };
  KIND.glass = function (w, h, C) {
    return card(R(0, h * .45, w, h * .55), C.wood) + card(R(3, 0, w - 6, h * .45), '#d8ecf2', 1.3) + line(8, 5, w * .4, h * .38, '#ffffff', 1.4) + card(R(-2, h * .42, w + 4, 5), '#c9a24e', 1);
  };
  KIND.stall = function (w, h, C) {
    var s = card(R(4, h * .2, 6, h * .8), C.wood, 1.3) + card(R(w - 10, h * .2, 6, h * .8), C.wood, 1.3) + card(R(0, h * .62, w, h * .38), dk(C.wood, .05));
    var n = Math.max(3, Math.round(w / 18)), sw = w / n, aw = '';
    for (var i = 0; i < n; i++) aw += card('M' + n1(i * sw) + ' 0h' + n1(sw) + 'v' + n1(h * .2) + 'q' + n1(-sw / 2) + ' 8 ' + n1(-sw) + ' 0z', i % 2 ? '#f3ead2' : C.acc, 1.2);
    return s + aw;
  };
  KIND.rug = function (w, h, C) {
    var s = '<path d="' + R(0, 0, w, h) + '" fill="' + dk(C.acc, .3) + '" stroke="' + INK + '" stroke-width="1.6"/>';
    s += '<path d="' + R(7, 7, w - 14, h - 14) + '" fill="none" stroke="#e8c47c" stroke-width="2"/><path d="' + R(13, 13, w - 26, h - 26) + '" fill="' + dk(C.acc, .05) + '" stroke="#e8c47c" stroke-width="1.1"/>';
    s += '<path d="M' + n1(w / 2) + ' 18L' + n1(w - 22) + ' ' + n1(h / 2) + 'L' + n1(w / 2) + ' ' + n1(h - 18) + 'L22 ' + n1(h / 2) + 'Z" fill="none" stroke="#e8c47c" stroke-width="1.4"/>';
    for (var x = 6; x < w - 4; x += 7) s += line(x, h, x, h + 5, '#e8c47c', 1.4);
    return s + '<path d="' + R(0, 0, w, h) + '" fill="url(#srp-fib)" opacity=".5"/>';
  };
  KIND.pedestal = function (w, h) {
    return card(R(0, h - 8, w, 8), '#8b8796', 1.3) + card(R(w * .25, 10, w * .5, h - 18), '#a9a5b4') + card(R(-2, 2, w + 4, 9), '#b9b5c4', 1.3);
  };
  KIND.lamp = function (w, h) {
    var cx = w / 2;
    return '<g class="srp-sway">' + line(cx, 0, cx, h * .45, INK, 1.2) + card(R(cx - 7, h * .45, 14, 4), '#3b3b3b', 1.2) + card(R(cx - 8, h * .45 + 4, 16, 18), '#f3d27a', 1.3) +
      '<path class="srp-flame" d="M' + n1(cx) + ' ' + n1(h * .45 + 18) + 'c-4-3-2-8 0-11 2 3 4 8 0 11z" fill="#f08a2a"/>' + card(R(cx - 9, h * .45 + 22, 18, 4), '#3b3b3b', 1.2) + '</g>';
  };
  // Anything the drawings do not know yet still shows, as a plain card.
  function fallback(w, h, C) { return card(R(0, 0, w, h), lt(C.wood, .1)); }

  // The shopkeeper as a cut-card figure; with the shop's portrait, it is set in
  // a frame on the figure's head instead of the drawn face.
  function keeperArt(acc, portrait) {
    var s = '<g class="srp-bob">' + card('M14 120c0-38 10-60 36-60s36 22 36 60z', dk(acc, .25)) + card('M34 72h32l-2 48h-28z', '#e2d3b2', 1.6);
    s += line(40, 86, 60, 86, '#b39d74') + line(41, 98, 59, 98, '#b39d74');
    if (portrait) return s + card('M50 8a24 26 0 1 1 0 52a24 26 0 1 1 0-52z', '#c9a24e', 2) + '</g>';
    s += card('M30 38c0-20 40-20 40 0v8c0 18-40 18-40 0z', '#e0b48c') + card('M30 46c2 26 38 26 40 0-4 10-12 14-20 14s-16-4-20-14z', '#d9d2c4', 1.4);
    s += '<path d="M42 56c4 2 12 2 16 0" stroke="' + INK + '" stroke-width="1.4" fill="none"/>' + card('M28 36c-2-8 2-12 6-12-2 6 0 10-2 14zM72 36c2-8-2-12-6-12 2 6 0 10 2 14z', '#c9c2b4', 1.2);
    s += '<g class="srp-blink"><circle cx="43" cy="40" r="2.2" fill="' + INK + '"/><circle cx="57" cy="40" r="2.2" fill="' + INK + '"/></g><path d="M48 44l2 4h-3" stroke="#9c6e4a" stroke-width="1.4" fill="none"/>';
    s += '<g class="srp-arm">' + card('M78 78l14-22c3-4 8-1 6 3l-12 24z', dk(acc, .25), 1.6) + card('M90 52c2-6 10-6 10 0s-6 8-10 4z', '#e0b48c', 1.4) + '</g>';
    return s + card('M14 84l-6 22c-1 4 4 6 6 2l8-20z', dk(acc, .25), 1.6) + '</g>';
  }
  // The shop's name on a hanging cloth sign.
  function signArt(name, acc) {
    var s = '<g class="srp-sway"><path d="M30 0v14M130 0v14" stroke="' + INK + '" stroke-width="1.6"/>';
    s += card('M4 14h152v34l-8 8-8-8h-120l-8 8-8-8z', dk(acc, .35));
    s += '<path d="M10 20h140v24h-140z" fill="none" stroke="#e8c47c" stroke-width="1.2" stroke-dasharray="3 2"/>';
    return s + '<text x="80" y="37" text-anchor="middle" font-family="Georgia,serif" font-weight="700" font-size="13"' + (String(name).length > 16 ? ' textLength="132" lengthAdjust="spacingAndGlyphs"' : '') + ' fill="#e8c47c">' + esc(name) + '</text></g>';
  }
  // A wall stretch: the room's wall finish drawn in ink on paper, with a
  // skirting board and a cap along the top.
  function wallArt(w, h, C, kind) {
    var b = C.wall, j = dk(b, .38), s = card(R(0, 0, w, h), b, 2), y, x, r;
    if (kind === 'stone' || kind === 'brick' || kind === 'tower') {
      var bh = kind === 'brick' ? 11 : 17, bw = kind === 'brick' ? 26 : 38;
      for (y = 8, r = 0; y < h - 12; y += bh, r++) { s += line(0, y, w, y, j, .8); for (x = (r % 2) * bw / 2; x < w; x += bw) s += line(x, y, x, Math.min(y + bh, h - 12), j, .8); }
    } else if (kind === 'panel') {
      for (x = 14; x < w; x += 22) s += line(x, 8, x, h - 12, j, .9);
      s += card(R(0, h * .55, w, 5), C.trim, 1.1);
    } else if (kind === 'plaster' || kind === 'burrow') {
      for (x = 0; x < w; x += 120) s += card(R(x, 8, 9, h - 20), C.wood, 1.2);
      s += card(R(0, h * .35, w, 7), C.wood, 1.2);
    } else if (kind === 'tree') {
      for (x = 10; x < w; x += 23) s += '<path d="M' + n1(x) + ' 8c4 ' + n1(h * .3) + '-4 ' + n1(h * .6) + ' 1 ' + n1(h - 20) + '" fill="none" stroke="' + j + '" stroke-width="1.3"/>';
    } else if (kind === 'cave') {
      for (x = 8; x < w; x += 30) s += '<path d="M' + n1(x) + ' ' + n1(16 + (x * 7) % 30) + 'l12 9-5 12" fill="none" stroke="' + j + '" stroke-width="1.1"/>';
    } else {
      for (y = 18; y < h - 12; y += 15) s += line(0, y, w, y, j, .8);
    }
    return s + card(R(0, h - 12, w, 12), C.trim, 1.4) + card(R(-2, 0, w + 4, 8), dk(C.wood, .2), 1.4);
  }
  // One good or decoration as a little cut-card icon.
  function iconArt(g, fill) {
    var d = g[1];
    return '<path d="' + d + '" transform="translate(26 26)" fill="' + dk(fill, .5) + '"/><path d="' + d + '" fill="' + fill + '" stroke="' + INK + '" stroke-width="30" stroke-linejoin="round" paint-order="stroke"/>' +
      '<path d="' + d + '" fill="url(#srp-sh)"/>';
  }

  // The floor's finish as CSS backgrounds, in paper tones of the room's floor colour.
  function floorCss(kind, f) {
    var a = lt(f, .3), b = lt(f, .12), j = dk(f, .35), t = T + 'px';
    if (kind === 'flags' || kind === 'tower') return 'background:linear-gradient(' + j + ' 0 2px,transparent 2px) 0 0/' + (T * 1.2) + 'px ' + (T * 1.2) + 'px,linear-gradient(90deg,' + j + ' 0 2px,transparent 2px) 0 0/' + (T * 1.2) + 'px ' + (T * 1.2) + 'px,' + a;
    if (kind === 'checker') return 'background:repeating-conic-gradient(' + lt(f, .55) + ' 0 25%,' + b + ' 0 50%) 0 0/' + t + ' ' + t + ',' + a;
    if (kind === 'herring') return 'background:repeating-linear-gradient(45deg,' + a + ' 0 14px,' + b + ' 14px 15px,' + a + ' 15px 28px,' + j + ' 28px 29px),' + a;
    if (kind === 'dirt') return 'background:radial-gradient(circle at 30% 40%,' + b + ' 0 12%,transparent 13%) 0 0/' + (T * 1.6) + 'px ' + (T * 1.6) + 'px,radial-gradient(circle at 70% 80%,' + dk(f, .1) + ' 0 9%,transparent 10%) 0 0/' + (T * 1.3) + 'px ' + (T * 1.3) + 'px,' + a;
    if (kind === 'rings') return 'background:repeating-radial-gradient(circle at 50% 50%,' + a + ' 0 14px,' + j + ' 14px 15px,' + b + ' 15px 26px,' + j + ' 26px 27px),' + a;
    return 'background:repeating-linear-gradient(0deg,' + a + ' 0 ' + (T / 4 - 1) + 'px,' + j + ' ' + (T / 4 - 1) + 'px ' + (T / 4) + 'px),' + a;
  }

  // ---- Layout. The floor is turned a quarter round so both back walls face
  // the viewer, as in the lit room; every card stands on a base point. ----
  var DIAG = Math.SQRT1_2;
  // Where a furniture card stands: base point in floor units, its turn on the
  // floor, its size in px, how high it hangs, and how far along its base the
  // point is (0 = its left end, .5 = its middle).
  function stand(p, WH) {
    var h = p.h * Z, k = p.kind;
    if (p.wall) {
      var o = p.off + (p.depth ? p.depth * .5 : .04), z = 0;
      if (k === 'herbs') { z = (WH - 1.6) * Z; h = 1.5 * Z; }
      if (k === 'window') h = 2.6 * Z;
      return p.wall === 'Y' ? { x: p.x, y: o, rot: 0, w: p.len * T, h: h, z: z, ax: 0 } : { x: o, y: p.y + p.len, rot: -90, w: p.len * T, h: h, z: z, ax: 0 };
    }
    if (k === 'lamp') return { x: p.x + .2, y: p.y + .2, rot: -45, w: 36, h: 1.3 * Z, z: (WH - 1.3) * Z, ax: .5 };
    var r = p.r;
    return { x: r[0] + r[2] / 2, y: r[1] + r[3] / 2, rot: -45, w: Math.max(30, (r[2] + r[3]) * DIAG * T), h: h, z: 0, ax: .5 };
  }
  // Where a good or decoration stands for its spot: in front of a wall
  // piece's card, just behind a floor piece's card so it sits on top, and on
  // a glass case's shelf rather than inside its base.
  function spotPos(a, p) {
    var x = a[0], y = a[1], z = a[2];
    if (p.wall) { var f = p.off + (p.depth ? p.depth * .5 : .04) + .22; if (p.wall === 'Y') y = f; else x = f; }
    else if (p.kind !== 'rug') {
      var s0 = p.r[0] + p.r[2] / 2 + p.r[1] + p.r[3] / 2 - .16, sd = x + y;
      if (sd > s0) { x -= (sd - s0) / 2; y -= (sd - s0) / 2; }
      if (p.kind === 'glass') z = Math.max(z, .66);
    }
    return [x, y, z];
  }
  // How lit a spot is: the room's own dimness, brightened and warmed near the
  // forge and lamps, brightened a little near windows.
  function lightAt(x, y, M) {
    var w = 0, c = 0;
    M.warm.forEach(function (q) { w = Math.max(w, 1 - Math.hypot(x - q[0], y - q[1]) / 4.5); });
    M.cool.forEach(function (q) { c = Math.max(c, 1 - Math.hypot(x - q[0], y - q[1]) / 4); });
    return { b: Math.min(1.12, M.dim + .42 * w + .2 * Math.max(0, c)), s: .45 * w };
  }
  function lf(x, y, M) { if (!M.full) return ''; var L = lightAt(x, y, M); return ';--lf:brightness(' + L.b.toFixed(2) + ') sepia(' + L.s.toFixed(2) + ')'; }
  function at(x, y, z, rot, cls, inner, extra) {
    return '<div class="srp-at' + (cls ? ' ' + cls : '') + '" style="transform:translate3d(' + n1(x * T) + 'px,' + n1(y * T) + 'px,' + n1(z || 0) + 'px) rotateZ(' + rot + 'deg)"' + (extra || '') + '>' + inner + '</div>';
  }
  function up(w, h, ax, i, body, style) {
    return '<div class="srp-up" style="left:' + n1(-w * ax) + 'px;width:' + n1(w) + 'px;height:' + n1(h) + 'px;--i:' + i + (style || '') + '"><div class="srp-cd">' + svg(w, h, body) + '</div></div>';
  }

  // The whole paper room as HTML: o carries the engine's drawing (view), the
  // goods (its), the shop's name and portrait, the icons and the effects level.
  function html(o) {
    var V = o.view, Rm = V.room, C = Rm.colours, N = Rm.N, WH = Rm.wh, ICONS = o.icons || {}, s = '', i = 0;
    var byId = {}; Rm.pieces.forEach(function (p) { byId[p.id] = p; });
    var M = { full: o.fx === 'full', dim: Rm.dim, warm: [], cool: [] };
    Rm.pieces.forEach(function (p) {
      if (p.warm) M.warm.push(p.warm);
      if (p.kind === 'window') { var c = stand(p, WH), f = p.wall === 'Y' ? [p.x + p.len / 2, c.y + 1.1] : [c.x + 1.1, p.y + p.len / 2]; M.cool.push(f); }
    });
    // Page, floor, and the light and shadow lying on it.
    s += '<div class="srp-page" style="inset:' + (-.35 * T) + 'px"></div>';
    s += '<div class="srp-ground" style="' + floorCss(Rm.floor, C.floor) + ';clip-path:polygon(' + Rm.pts.map(function (q) { return n1(q[0] * T) + 'px ' + n1(q[1] * T) + 'px'; }).join(',') + ')"></div>';
    M.cool.forEach(function (q) { s += '<div class="srp-glow cool" style="left:' + n1((q[0] - 1.6) * T) + 'px;top:' + n1((q[1] - 1.6) * T) + 'px;width:' + (3.2 * T) + 'px;height:' + (3.2 * T) + 'px"></div>'; });
    M.warm.forEach(function (q, k) { s += '<div class="srp-glow warm" style="left:' + n1((q[0] - 2.6) * T) + 'px;top:' + n1((q[1] - 2.6) * T) + 'px;width:' + (5.2 * T) + 'px;height:' + (5.2 * T) + 'px;animation-delay:-' + (k * .7) + 's"></div>'; });
    var cast = window.ShopRoomLight ? window.ShopRoomLight.castOffset : function (w, x, y, h) { return [h * .5, h * .16]; };
    Rm.pieces.forEach(function (p) {
      if (p.kind === 'rug' || p.kind === 'lamp' || p.kind === 'window' || p.kind === 'herbs') return;
      var st = stand(p, WH);
      if (p.wall) { s += at(st.x, st.y, .2, st.rot, '', '<div class="srp-foot wall" style="width:' + n1(st.w) + 'px;height:' + (T * .5) + 'px"></div>'); return; }
      var r = p.r, cx = r[0] + r[2] / 2, cy = r[1] + r[3] / 2, off = cast(M.warm, cx, cy, p.h), L = Math.hypot(off[0], off[1]), ang = Math.atan2(off[1], off[0]) * 180 / Math.PI, sz = Math.max(r[2], r[3]);
      s += at(cx + off[0] / 2, cy + off[1] / 2, .25, n1(ang), '', '<div class="srp-cast" style="width:' + n1((L + sz) * T) + 'px;height:' + n1(Math.min(r[2], r[3]) * T * .9) + 'px"></div>');
      s += at(cx, cy, .3, -45, '', '<div class="srp-foot" style="width:' + n1(st.w * 1.05) + 'px"></div>');
    });
    // Rugs lie flat on the floor.
    Rm.pieces.forEach(function (p) {
      if (p.kind !== 'rug') return;
      s += '<div class="srp-rug srp-pc" data-p="' + p.id + '" style="left:' + n1(p.x * T) + 'px;top:' + n1(p.y * T) + 'px;width:' + n1(p.w * T) + 'px;height:' + n1(p.d * T) + 'px' + lf(p.x + p.w / 2, p.y + p.d / 2, M) + '"><div class="srp-cd">' + svg(p.w * T, p.d * T, KIND.rug(p.w * T, p.d * T, C)) + '</div></div>';
    });
    // Back walls, then the sign on the one facing most to the right.
    var signE = null, sb = -9;
    Rm.back.forEach(function (e) {
      var m = [-e.d[1], e.d[0]], mid = [(e.a[0] + e.b[0]) / 2, (e.a[1] + e.b[1]) / 2];
      if (m[0] - Math.abs(m[1]) > sb && e.len > 1.2) { sb = m[0] - Math.abs(m[1]); signE = e; }
      s += at(e.a[0], e.a[1], 0, n1(Math.atan2(e.d[1], e.d[0]) * 180 / Math.PI), 'srp-wall', up(e.len * T + .5, e.h * Z, 0, 0, wallArt(e.len * T + .5, e.h * Z, C, Rm.wall), lf(mid[0] + m[0], mid[1] + m[1], M)));
    });
    if (signE) {
      var sm = [-signE.d[1], signE.d[0]], sx = (signE.a[0] + signE.b[0]) / 2 + sm[0] * .08, sy = (signE.a[1] + signE.b[1]) / 2 + sm[1] * .08;
      s += at(sx, sy, (WH - .15) * Z - 60, n1(Math.atan2(signE.d[1], signE.d[0]) * 180 / Math.PI), 'srp-sign', up(150, 56, .5, 1, signArt(o.name || 'Shop', C.acc), lf(sx, sy, M)));
    }
    // Furniture, goods and decorations, back to front so they pop up in that order.
    var bases = [];
    Rm.pieces.forEach(function (p) {
      if (p.kind === 'rug') return;
      var st = stand(p, WH), draw = KIND[p.kind] || fallback;
      bases.push({ d: st.x + st.y, h: at(st.x, st.y, st.z, st.rot, 'srp-pc' + (p.pinned ? ' pinned' : ''), up(st.w, st.h, st.ax, 0, draw(st.w, st.h, C), lf(st.x, st.y, M)), ' data-p="' + p.id + '"') });
    });
    (V.placed || []).forEach(function (q) {
      var p = byId[q.piece]; if (!p) return;
      var pos = spotPos(q.at, p), body, w, h, g;
      if (q.good !== undefined) {
        var it = (o.its || [])[q.good]; if (!it) return;
        g = ICONS[it.ic] || ICONS.box; if (!g) return;
        h = 34; w = Math.max(16, h * g[0] / 512);
        body = '<svg class="srp-ic" viewBox="-40 -40 ' + (g[0] + 80) + ' 592" aria-hidden="true">' + iconArt(g, it.c) + '</svg>';
        var tag = '<div class="srp-tag" style="transform:translateZ(' + (h + 8) + 'px) translateX(-50%) rotateX(-56deg)"><b>' + esc(it.n) + '</b> · ' + esc(it.price) + (it.out ? ' · sold out' : '') + '</div>';
        bases.push({ d: pos[0] + pos[1] + .01, h: at(pos[0], pos[1], pos[2] * Z, -45, 'srp-good it' + (it.out ? ' out' : ''), '<div class="srp-up" style="left:' + n1(-w / 2) + 'px;width:' + n1(w) + 'px;height:' + h + 'px;--i:0' + lf(pos[0], pos[1], M) + '"><div class="srp-cd">' + body + '</div></div>' + tag,
          ' data-i="' + q.good + '" tabindex="0" role="button" aria-label="' + esc(it.n + ', ' + it.price + (it.out ? ', sold out' : '')) + '"') });
      } else {
        g = ICONS[q.icon]; if (!g) return;
        h = 26; w = Math.max(13, h * g[0] / 512);
        body = (q.hang ? '<div class="srp-str"></div>' : '') + '<svg class="srp-ic" viewBox="-40 -40 ' + (g[0] + 80) + ' 592" aria-hidden="true">' + iconArt(g, Rm.decoCol[q.icon] || '#a3a3a3') + '</svg>';
        bases.push({ d: pos[0] + pos[1] + .01, h: at(pos[0], pos[1], pos[2] * Z - (q.hang ? h : 0), -45, 'srp-deco' + (q.hang ? ' hang' : ''), '<div class="srp-up" style="left:' + n1(-w / 2) + 'px;width:' + n1(w) + 'px;height:' + h + 'px;--i:0' + lf(pos[0], pos[1], M) + '"><div class="srp-cd">' + body + '</div></div>',
          ' data-up="' + q.piece + '" data-ua="' + q.spot + '"') });
      }
    });
    // The keeper stands behind the counter, or toward the back with no counter.
    var ctr = Rm.pieces.filter(function (p) { return p.kind === 'counter'; })[0], kx = ctr ? ctr.r[0] + ctr.r[2] / 2 - .42 : N * .3, ky = ctr ? ctr.r[1] + ctr.r[3] / 2 - .42 : N * .3;
    var kp = o.image ? '<img src="' + esc(o.image) + '" alt="">' : '';
    bases.push({ d: kx + ky, h: at(kx, ky, 0, -45, 'srp-keeper', '<div class="srp-up" style="left:-38px;width:76px;height:90px;--i:0' + lf(kx, ky, M) + '"><div class="srp-cd">' + '<svg viewBox="0 0 104 122" aria-hidden="true">' + keeperArt(C.acc, !!o.image) + '</svg>' + kp + '</div></div>', ' tabindex="0" role="button" aria-label="Talk to the shopkeeper"') });
    bases.sort(function (a, b) { return a.d - b.d; }).forEach(function (b) { s += b.h.replace('--i:0', '--i:' + (i++)); });
    // Markers for turning a pointer into a floor point or a spot.
    [[0, 0], [N, 0], [N, N], [0, N]].forEach(function (q, k) { s += '<i class="srp-mk" data-c="' + k + '" style="transform:translate3d(' + (q[0] * T) + 'px,' + (q[1] * T) + 'px,0)"></i>'; });
    // The tops of the walls' far corners, so fitting knows how tall the room stands.
    [[0, 0], [N, 0], [0, N]].forEach(function (q) { s += '<i class="srp-mk srp-top" style="transform:translate3d(' + (q[0] * T) + 'px,' + (q[1] * T) + 'px,' + (WH * Z) + 'px)"></i>'; });
    (V.anchors || []).forEach(function (a) { var p = byId[a.piece]; if (!p) return; var q = spotPos(a.at, p); s += '<i class="srp-mk" data-s="' + a.piece + ':' + a.spot + '" style="transform:translate3d(' + n1(q[0] * T) + 'px,' + n1(q[1] * T) + 'px,' + n1(q[2] * Z) + 'px)"></i>'; });
    var W0 = N * T * Math.SQRT2;
    return { html: '<div class="srp-floor" style="width:' + (N * T) + 'px;height:' + (N * T) + 'px;left:' + n1(-N * T / 2) + 'px;top:' + n1(-N * T / 2) + 'px">' + s + '</div>', w: W0 + 120, h: W0 * .56 + WH * Z * .83 + 130, fy: W0 * .28 + 60, N: N };
  }

  // ---- Pointer to floor. A flat floor seen in perspective maps to the screen
  // by a homography, so four corner markers are enough to invert it exactly. ----
  function solve(src, dst) {
    var A = [], i, j, k;
    for (i = 0; i < 4; i++) {
      var x = src[i][0], y = src[i][1], u = dst[i][0], v = dst[i][1];
      A.push([x, y, 1, 0, 0, 0, -u * x, -u * y, u]); A.push([0, 0, 0, x, y, 1, -v * x, -v * y, v]);
    }
    for (i = 0; i < 8; i++) {
      var mx = i; for (j = i + 1; j < 8; j++) if (Math.abs(A[j][i]) > Math.abs(A[mx][i])) mx = j;
      var t = A[i]; A[i] = A[mx]; A[mx] = t;
      if (Math.abs(A[i][i]) < 1e-12) return null;
      for (j = 0; j < 8; j++) { if (j === i) continue; var f = A[j][i] / A[i][i]; for (k = i; k < 9; k++) A[j][k] -= f * A[i][k]; }
    }
    return A.map(function (r, n) { return r[8] / r[n]; });
  }
  function apply(h, x, y) { var w = h[6] * x + h[7] * y + 1; return [(h[0] * x + h[1] * y + h[2]) / w, (h[3] * x + h[4] * y + h[5]) / w]; }
  function mid(el) { var r = el.getBoundingClientRect(); return [r.left + r.width / 2, r.top + r.height / 2]; }
  function floorAt(host, cx, cy) {
    var mk = host.querySelectorAll('.srp-mk[data-c]'), N = +(host.firstChild && host.firstChild.getAttribute('data-n')) || 8;
    if (mk.length !== 4) return null;
    var src = [], dst = [[0, 0], [N, 0], [N, N], [0, N]];
    for (var k = 0; k < 4; k++) src.push(mid(mk[k]));
    var h = solve(src, dst); return h ? apply(h, cx, cy) : null;
  }
  // The decoration spot nearest the pointer, within reach.
  function spotAt(host, cx, cy) {
    var best = null, bd = 46 * 46;
    host.querySelectorAll('.srp-mk[data-s]').forEach(function (m) { var q = mid(m), d = (q[0] - cx) * (q[0] - cx) + (q[1] - cy) * (q[1] - cy); if (d < bd) { bd = d; best = m.getAttribute('data-s'); } });
    if (!best) return null;
    var ps = best.split(':'); return { piece: +ps[0], spot: +ps[1] };
  }

  // The good, decoration or keeper under the pointer. Browsers hit-test cards
  // in a 3D scene unreliably when a small card stands just in front of a big
  // one, so this goes by each small card's box, frontmost (last drawn) first.
  function hit(host, cx, cy) {
    var els = host.querySelectorAll('.srp-good,.srp-deco,.srp-keeper');
    for (var i = els.length - 1; i >= 0; i--) {
      var r = els[i].querySelector('.srp-cd').getBoundingClientRect();
      if (cx >= r.left && cx <= r.right && cy >= r.top && cy <= r.bottom) return els[i];
    }
    return null;
  }

  // Scales and centres the book in the space it has, from where the page and
  // the wall tops actually land on screen (perspective makes that hard to guess).
  function fit(host) {
    var root = host.firstChild, page = root && root.querySelector('.srp-page'); if (!page) return;
    root.style.setProperty('--s', 1); root.style.setProperty('--dx', '0px'); root.style.setProperty('--dy', '0px');
    var H = host.getBoundingClientRect(), u = page.getBoundingClientRect(), l = u.left, t = u.top, r = u.right, b = u.bottom;
    root.querySelectorAll('.srp-top').forEach(function (m) { var q = mid(m); l = Math.min(l, q[0]); r = Math.max(r, q[0]); t = Math.min(t, q[1]); b = Math.max(b, q[1]); });
    if (!H.width || r <= l) return;
    var s = Math.min(H.width / (r - l), H.height / (b - t)) * .94;
    root.style.setProperty('--s', s.toFixed(3));
    root.style.setProperty('--dx', n1(H.left + H.width / 2 - (l + r) / 2) + 'px'); root.style.setProperty('--dy', n1(H.top + H.height / 2 - (t + b) / 2) + 'px');
  }
  // Draws the paper room into host. o: { view, its, name, image, icons, fx, arr, shut }.
  function render(host, o) {
    injectStyle();
    var r = html(o), cls = 'srp' + (o.fx === 'full' ? ' full' : ' fxl') + (o.arr ? ' arr' : '') + (o.shut ? ' shut' : '');
    host.innerHTML = DEFS + '<div class="' + cls + '" data-n="' + r.N + '" data-w="' + n1(r.w) + '" data-h="' + n1(r.h) + '" style="--srp-acc:' + o.view.room.colours.acc + '">' +
      '<div class="srp-fit" style="width:' + n1(r.w) + 'px;height:' + n1(r.h) + 'px"><div class="srp-cam"><div class="srp-tilt" style="top:' + n1(r.h - r.fy) + 'px">' + r.html + '</div></div></div><div class="srp-amb"></div></div>';
    // The defs svg comes first; the room itself is the element fit() scales.
    host.insertBefore(host.lastChild, host.firstChild);
    fit(host);
  }

  var DEFS = '<svg width="0" height="0" style="position:absolute" aria-hidden="true"><defs>' +
    '<filter id="srp-fibF" x="0" y="0" width="100%" height="100%"><feTurbulence type="fractalNoise" baseFrequency=".9 .25" numOctaves="2" seed="3"/><feColorMatrix values="0 0 0 0 .35  0 0 0 0 .25  0 0 0 0 .12  0 0 0 .55 -.18"/></filter>' +
    '<pattern id="srp-fib" width="120" height="120" patternUnits="userSpaceOnUse"><rect width="120" height="120" filter="url(#srp-fibF)"/></pattern>' +
    '<linearGradient id="srp-sh" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#fff" stop-opacity=".22"/><stop offset=".55" stop-color="#fff" stop-opacity="0"/><stop offset="1" stop-color="#2a1608" stop-opacity=".22"/></linearGradient></defs></svg>';

  // No mix-blend-mode anywhere inside the book: blending flattens a 3D scene.
  var CSS = [
    '.srp{position:absolute;inset:0;overflow:hidden}',
    '.srp-fit{position:absolute;left:50%;top:50%;transform:translate(-50%,-50%) scale(var(--s,1)) translate(var(--dx,0px),var(--dy,0px))}',
    '.srp-cam{position:absolute;inset:0;perspective:1500px;perspective-origin:50% 20%}',
    '.srp-tilt{position:absolute;left:50%;width:0;height:0;transform-style:preserve-3d;transition:transform .5s cubic-bezier(.2,.7,.2,1)}',
    '.srp-floor{position:absolute;transform-style:preserve-3d;transform:rotateX(56deg) rotateZ(45deg)}',
    '.srp-page{position:absolute;background:#efe4cc;background-image:radial-gradient(circle at 30% 20%,rgb(255 255 255/.35),transparent 60%),repeating-linear-gradient(0deg,rgb(80 60 30/.04) 0 2px,transparent 2px 5px);border-radius:6px;box-shadow:0 0 0 2px #2a1f17,0 30px 60px -20px rgb(0 0 0/.7)}',
    '.srp-ground{position:absolute;inset:0;transform:translateZ(.1px)}',
    '.srp-glow{position:absolute;border-radius:50%;pointer-events:none;transform:translateZ(.5px);background:radial-gradient(closest-side,rgb(255 176 92/.34),rgb(255 150 60/.12) 55%,transparent)}',
    '.srp-glow.cool{background:radial-gradient(closest-side,rgb(214 232 255/.28),transparent)}',
    '.srp.full .srp-glow.warm{animation:srp-fl 2.2s ease-in-out infinite alternate}@keyframes srp-fl{0%{opacity:.82}35%{opacity:1}60%{opacity:.76}100%{opacity:.92}}',
    '.srp-at{position:absolute;left:0;top:0;width:0;height:0;transform-style:preserve-3d}',
    '.srp-cast{position:absolute;left:0;top:0;transform:translate(-50%,-50%);border-radius:50%;background:radial-gradient(closest-side,rgb(30 16 4/.42),rgb(30 16 4/.16) 60%,transparent)}',
    '.srp-foot{position:absolute;left:0;top:0;height:16px;transform:translate(-50%,-50%);border-radius:50%;background:radial-gradient(closest-side,rgb(30 16 4/.5),transparent)}',
    '.srp-foot.wall{transform:none;border-radius:0;background:linear-gradient(rgb(30 16 4/.42),transparent)}',
    '.srp-up{position:absolute;bottom:0;transform-origin:50% 100%;transform-style:preserve-3d;transform:rotateX(-90deg);transition:transform .7s cubic-bezier(.3,1.5,.5,1);transition-delay:calc(min(var(--i,0),40) * 28ms)}',
    '.srp.shut .srp-up{transform:rotateX(0deg);transition:none}',
    '.srp-cd{position:absolute;inset:0;transition:transform .25s cubic-bezier(.3,1.6,.5,1)}',
    '.srp-cd>svg{display:block;width:100%;height:100%;overflow:visible;filter:var(--lf,brightness(1)) drop-shadow(0 1px 0 rgb(40 25 10/.4))}',
    '.srp-rug{position:absolute;transform:translateZ(.6px)}.srp-rug .srp-cd>svg{filter:var(--lf,brightness(1))}',
    '.srp-good{cursor:pointer;outline:none}.srp-good:hover .srp-cd,.srp-good:focus-visible .srp-cd{transform:translateY(-7px)}',
    '.srp-good:focus-visible .srp-cd>svg{filter:var(--lf,brightness(1)) drop-shadow(0 0 2px var(--color-accent,#6366f1)) drop-shadow(0 0 2px var(--color-accent,#6366f1))}',
    '.srp-good.out .srp-cd{opacity:.4}',
    // Price tags lean back toward the viewer's eye so they read straight on.
    '.srp-tag{position:absolute;left:0;bottom:0;transform-origin:50% 100%;white-space:nowrap;padding:3px 9px;background:#fbf5e6;border:2px solid #2a1f17;border-radius:4px;font:600 12px Georgia,serif;color:#7a2a1f;pointer-events:none;opacity:0;transition:opacity .18s}',
    '.srp-tag b{color:#2a1f17}',
    '.srp-good:hover .srp-tag,.srp-good:focus-visible .srp-tag{opacity:1}',
    '.srp.arr .srp-tag{display:none}',
    '.srp-str{position:absolute;left:50%;bottom:100%;width:1px;height:14px;background:#2a1f17}',
    '.srp-keeper{cursor:pointer;outline:none}.srp-keeper img{position:absolute;left:27%;top:7.5%;width:46%;height:41%;border-radius:50%;object-fit:cover}',
    '.srp-keeper:focus-visible .srp-cd>svg{filter:drop-shadow(0 0 2px var(--color-accent,#6366f1))}',
    '.srp-keeper.talk .srp-arm{animation:srp-wave .45s ease-in-out 4 alternate}@keyframes srp-wave{to{transform:rotate(-28deg)}}',
    '.srp-arm{transform-box:fill-box;transform-origin:0 100%}',
    '.srp.arr .srp-pc,.srp.arr .srp-deco{cursor:grab}.srp:not(.arr) .srp-pc,.srp:not(.arr) .srp-wall,.srp:not(.arr) .srp-sign{pointer-events:none}',
    '.srp.arr .srp-pc .srp-cd>svg,.srp.arr .srp-deco .srp-cd>svg{filter:var(--lf,brightness(1)) drop-shadow(0 0 1.5px var(--srp-acc)) drop-shadow(0 0 1.5px var(--srp-acc))}',
    '.srp-mk{position:absolute;left:0;top:0;width:1px;height:1px;pointer-events:none}',
    '.srp-amb{position:absolute;inset:0;pointer-events:none;background:radial-gradient(ellipse at 50% 42%,transparent 40%,rgb(12 9 20/.35) 100%)}',
    '.srp.full .srp-amb{background:radial-gradient(ellipse at 50% 42%,transparent 34%,rgb(12 9 20/.55) 100%)}',
    // Living details: flames, embers, sparks, swaying lanterns, a breathing keeper.
    '.srp-flame{transform-box:fill-box;transform-origin:50% 100%;animation:srp-flame .7s ease-in-out infinite alternate}.srp-flame.b{animation-duration:.9s;animation-delay:-.3s}',
    '@keyframes srp-flame{0%{transform:none}50%{transform:scale(.9,1.12) skewX(-5deg)}100%{transform:scale(1.06,.92) skewX(4deg)}}',
    '.srp-ember{opacity:0;animation:srp-ember 2.6s ease-out infinite}@keyframes srp-ember{0%{transform:none;opacity:0}10%{opacity:1}100%{transform:translate(var(--dx,0),-46px);opacity:0}}',
    '.srp-spark{opacity:0;animation:srp-spark 4.2s ease-out infinite}@keyframes srp-spark{0%,86%{transform:none;opacity:0}88%{opacity:1}100%{transform:translate(var(--dx),var(--dy));opacity:0}}',
    '.srp-sway{transform-box:fill-box;transform-origin:50% 0;animation:srp-sway 3.2s ease-in-out infinite alternate}@keyframes srp-sway{0%{transform:rotate(-3deg)}100%{transform:rotate(3deg)}}',
    '.srp-bob{animation:srp-bob 3s ease-in-out infinite alternate}@keyframes srp-bob{to{transform:translateY(-1.6px)}}',
    '.srp-blink{transform-box:fill-box;transform-origin:50% 50%;animation:srp-blink 4.6s infinite}@keyframes srp-blink{0%,94%,100%{transform:none}96%{transform:scaleY(.1)}}',
    '.srp.fxl .srp-ember,.srp.fxl .srp-spark{display:none}.srp.fxl *{animation:none!important}',
    '@media (prefers-reduced-motion:reduce){.srp *{animation:none!important;transition:none!important}.srp-ember,.srp-spark{display:none}}'
  ].join('\n');
  function injectStyle() {
    if (!window.document || document.getElementById('srp-style')) return;
    var st = document.createElement('style'); st.id = 'srp-style'; st.textContent = CSS; document.head.appendChild(st);
  }

  window.ShopRoomPaper = { KIND: KIND, html: html, render: render, fit: fit, floorAt: floorAt, spotAt: spotAt, hit: hit, solve: solve, apply: apply, spotPos: spotPos };
})();
