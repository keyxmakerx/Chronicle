/**
 * craft_results.js -- pictures of made things (window.CraftResults)
 *
 * A crafted item's picture is put together from what went into it: a result
 * shape (a round bottle, a blade, a buckler, a ring) wears its materials'
 * tints, and one material's own picture sits on its label, pommel or boss.
 * Two potions brewed from different herbs look different in the pack.
 *
 * The drawings are lit like the bench they come off (craft_desk.js): one
 * warm light from the top left, glass that shows the wall behind it, metal
 * with a hard highlight and a soft falloff. They are drawn far larger than
 * the material pictures (a 120 box) so they still read at the size of a
 * card or the pack's row.
 *
 * Only tints and picture keys go in, never item names or ids: what a project
 * took is the Director's, and a picture must not tell a player what that was.
 */
window.CraftResults = (function () {
  'use strict';
  var U = 'crx', seq = 0;
  var HEX = /^#[0-9a-f]{6}$/i;

  /* Shading one tint: amt above 0 lightens toward white, below 0 darkens
     toward a warm black, so shadows keep the bench's lamplight. */
  function mix(hex, amt) {
    if (!HEX.test(hex || '')) hex = '#8a8a8a';
    var n = parseInt(hex.slice(1), 16), r = n >> 16, g = (n >> 8) & 255, b = n & 255;
    var t = amt > 0 ? [255, 250, 240] : [22, 14, 10], a = Math.min(1, Math.abs(amt));
    function c(v, w) { return Math.round(v + (w - v) * a); }
    return '#' + ((1 << 24) + (c(r, t[0]) << 16) + (c(g, t[1]) << 8) + c(b, t[2])).toString(16).slice(1);
  }
  function id(n) { return U + n; }
  function url(n) { return 'url(#' + U + n + ')'; }
  function grad(n, x1, y1, x2, y2, stops) {
    return '<linearGradient id="' + id(n) + '" x1="' + x1 + '" y1="' + y1 + '" x2="' + x2 + '" y2="' + y2 + '">' + stops.map(stop).join('') + '</linearGradient>';
  }
  function rgrad(n, cx, cy, r, stops, fx, fy) {
    return '<radialGradient id="' + id(n) + '" cx="' + cx + '" cy="' + cy + '" r="' + r + '"' + (fx != null ? ' fx="' + fx + '" fy="' + fy + '"' : '') + '>' + stops.map(stop).join('') + '</radialGradient>';
  }
  function stop(s) { return '<stop offset="' + s[0] + '" stop-color="' + s[1] + '"' + (s[2] != null ? ' stop-opacity="' + s[2] + '"' : '') + '/>'; }
  function blur(n, d) { return '<filter id="' + id(n) + '" x="-50%" y="-50%" width="200%" height="200%"><feGaussianBlur stdDeviation="' + d + '"/></filter>'; }
  /* The soft contact shadow every made thing sits on. */
  function floor(cx, rx) {
    return '<ellipse cx="' + cx + '" cy="112" rx="' + rx + '" ry="4.5" fill="rgb(30 18 10 / .32)" filter="' + url('soft') + '"/>';
  }
  /* A material's own picture, placed in the result's box. */
  function mark(m, x, y, s) {
    if (!m || !m.picture || !window.CraftPictures || !window.CraftPictures.has(m.picture)) return '';
    return '<svg x="' + x + '" y="' + y + '" width="' + s + '" height="' + s + '" viewBox="0 0 40 40">' + window.CraftPictures.inner(m.picture, m.tint) + '</svg>';
  }

  var SHAPES = {}, DEFAULTS = {};
  var CORK = '#9a7650', GLASS = '#dfeef2';

  /* A round-bottomed bottle: the brew is the body tint, the cork and string
     the trim, the paper tag carries the mark, and the detail tint floats in
     the brew as flecks. Glass shows the dark wall through its back. */
  SHAPES.potion = function (s) {
    var b = s.body, h = window.CraftResults._h;
    var defs = '<defs>' + h.blur('soft', 2.4) + h.blur('glow', 6) +
      h.rgrad('brew', 0.5, 0.78, 0.62, [[0, mix(b, 0.45)], [0.45, b], [1, mix(b, -0.6)]], 0.46, 0.92) +
      h.rgrad('back', 0.5, 0.5, 0.5, [[0.55, '#000', 0], [0.92, '#1a1410', 0.28], [1, '#1a1410', 0.5]]) +
      h.grad('cork', 0, 0, 1, 0, [[0, mix(s.trim, -0.4)], [0.35, mix(s.trim, 0.25)], [1, mix(s.trim, -0.5)]]) +
      h.grad('neck', 0, 0, 1, 0, [[0, '#fff', 0.5], [0.25, '#fff', 0.08], [0.7, '#fff', 0.02], [0.9, '#fff', 0.45], [1, '#fff', 0.1]]) +
      h.grad('tag', 0, 0, 0, 1, [[0, '#f8f2e4'], [1, '#e2d5b8']]) +
      '<clipPath id="' + h.id('in') + '"><circle cx="60" cy="72" r="34"/></clipPath></defs>';
    var flecks = '';
    [[50, 86, 1.6], [66, 92, 1.2], [58, 98, 1.4], [72, 82, 1], [46, 95, 1]].forEach(function (p) {
      flecks += '<circle cx="' + p[0] + '" cy="' + p[1] + '" r="' + p[2] + '" fill="' + mix(s.detail, 0.35) + '" opacity=".9"/>';
    });
    return defs + h.floor(60, 30) +
      '<circle cx="60" cy="86" r="26" fill="' + b + '" opacity=".35" filter="' + h.url('glow') + '"/>' +
      /* the back of the glass, then the brew with its meniscus */
      '<circle cx="60" cy="72" r="34" fill="' + GLASS + '" opacity=".16"/>' +
      '<g clip-path="' + h.url('in') + '"><path d="M20 70 Q60 64 100 70 V110 H20Z" fill="' + h.url('brew') + '"/>' +
      '<ellipse cx="60" cy="68" rx="33" ry="4.2" fill="' + mix(b, 0.3) + '" opacity=".85"/>' +
      '<ellipse cx="60" cy="68" rx="33" ry="4.2" fill="none" stroke="#fff" stroke-opacity=".35" stroke-width=".8"/>' + flecks +
      '<ellipse cx="66" cy="100" rx="14" ry="3" fill="#fff" opacity=".28"/></g>' +
      '<circle cx="60" cy="72" r="34" fill="' + h.url('back') + '"/>' +
      /* neck and cork */
      '<path d="M52 20 V40 Q52 42 50 43 L52 44 H68 L70 43 Q68 42 68 40 V20Z" fill="' + GLASS + '" opacity=".2"/>' +
      '<path d="M52 20 V40 Q52 42 50 43 L52 44 H68 L70 43 Q68 42 68 40 V20Z" fill="' + h.url('neck') + '"/>' +
      '<ellipse cx="60" cy="20" rx="9" ry="2.6" fill="none" stroke="#fff" stroke-opacity=".7" stroke-width="1.4"/>' +
      '<path d="M53 8 h14 l-1 13 h-12z" fill="' + h.url('cork') + '"/>' +
      '<ellipse cx="60" cy="8" rx="7" ry="2" fill="' + mix(s.trim, 0.2) + '"/>' +
      '<path d="M56 11 v8 M60 10 v9 M64 11 v8" stroke="' + mix(s.trim, -0.5) + '" stroke-width=".6" opacity=".5"/>' +
      /* the glass edge and its highlights */
      '<circle cx="60" cy="72" r="34" fill="none" stroke="#fff" stroke-opacity=".55" stroke-width="1.6"/>' +
      '<circle cx="60" cy="72" r="31.6" fill="none" stroke="#fff" stroke-opacity=".14" stroke-width="1"/>' +
      '<path d="M34 58 A28 28 0 0 1 52 42" fill="none" stroke="#fff" stroke-opacity=".85" stroke-width="3" stroke-linecap="round"/>' +
      '<g transform="rotate(-18 44 58)" fill="#fff" opacity=".42"><rect x="38" y="50" width="5" height="7" rx="1"/><rect x="44" y="50" width="5" height="7" rx="1"/><rect x="38" y="58" width="5" height="7" rx="1"/><rect x="44" y="58" width="5" height="7" rx="1"/></g>' +
      '<circle cx="80" cy="56" r="1.8" fill="#fff" opacity=".9"/>' +
      '<path d="M88 84 A30 30 0 0 1 72 102" fill="none" stroke="#fff" stroke-opacity=".3" stroke-width="2" stroke-linecap="round"/>' +
      /* string and tag */
      '<path d="M52 30 Q60 34 68 30" fill="none" stroke="' + mix(s.trim, -0.3) + '" stroke-width="1.4"/>' +
      '<path d="M68 30 Q76 34 80 40" fill="none" stroke="' + mix(s.trim, -0.3) + '" stroke-width="1"/>' +
      '<g transform="rotate(12 90 48)"><path d="M78 38 h22 v20 h-22 l-4 -10z" fill="' + h.url('tag') + '" stroke="#8a7a5a" stroke-width=".8"/>' +
      '<circle cx="79.5" cy="48" r="1.4" fill="#8a7a5a"/>' + h.mark(s.mark, 82, 39, 18) + '</g>';
  };
  DEFAULTS.potion = ['#c3223a', CORK, '#f4ecd6'];

  /* A tall corked vial, for tinctures and oils: the same glass, narrower,
     standing a little turned so the light runs down one side. */
  SHAPES.vial = function (s) {
    var b = s.body, h = window.CraftResults._h;
    var defs = '<defs>' + h.blur('soft', 2.4) + h.blur('glow', 5) +
      h.grad('brew', 0, 0, 1, 0, [[0, mix(b, -0.45)], [0.3, mix(b, 0.3)], [0.55, b], [1, mix(b, -0.6)]]) +
      h.grad('gl', 0, 0, 1, 0, [[0, '#fff', 0.55], [0.18, '#fff', 0.1], [0.75, '#fff', 0.03], [0.9, '#fff', 0.4], [1, '#fff', 0.08]]) +
      h.grad('cork', 0, 0, 1, 0, [[0, mix(s.trim, -0.4)], [0.35, mix(s.trim, 0.25)], [1, mix(s.trim, -0.5)]]) +
      h.grad('tag', 0, 0, 0, 1, [[0, '#f8f2e4'], [1, '#e2d5b8']]) + '</defs>';
    return defs + h.floor(60, 18) +
      '<g transform="rotate(-6 60 70)">' +
      '<rect x="46" y="58" width="28" height="44" rx="6" fill="' + b + '" opacity=".35" filter="' + h.url('glow') + '"/>' +
      '<path d="M46 30 h28 v66 a8 8 0 0 1 -8 8 h-12 a8 8 0 0 1 -8 -8z" fill="' + GLASS + '" opacity=".18"/>' +
      '<path d="M47 56 h26 v40 a7 7 0 0 1 -7 7 h-12 a7 7 0 0 1 -7 -7z" fill="' + h.url('brew') + '"/>' +
      '<ellipse cx="60" cy="56" rx="13" ry="2.4" fill="' + mix(b, 0.35) + '"/>' +
      '<circle cx="55" cy="80" r="1.4" fill="' + mix(s.detail, 0.3) + '"/><circle cx="64" cy="90" r="1.1" fill="' + mix(s.detail, 0.3) + '"/><circle cx="58" cy="96" r="1.2" fill="' + mix(s.detail, 0.3) + '"/>' +
      '<path d="M46 30 h28 v66 a8 8 0 0 1 -8 8 h-12 a8 8 0 0 1 -8 -8z" fill="' + h.url('gl') + '"/>' +
      '<path d="M46 30 h28 v66 a8 8 0 0 1 -8 8 h-12 a8 8 0 0 1 -8 -8z" fill="none" stroke="#fff" stroke-opacity=".55" stroke-width="1.4"/>' +
      '<path d="M50 36 V94" stroke="#fff" stroke-opacity=".75" stroke-width="2.6" stroke-linecap="round"/>' +
      '<ellipse cx="60" cy="30" rx="15" ry="3" fill="none" stroke="#fff" stroke-opacity=".7" stroke-width="1.6"/>' +
      '<path d="M50 14 h20 l-1.5 17 h-17z" fill="' + h.url('cork') + '"/><ellipse cx="60" cy="14" rx="10" ry="2.4" fill="' + mix(s.trim, 0.2) + '"/>' +
      '<path d="M48 36 Q60 41 72 36" fill="none" stroke="' + mix(s.trim, -0.3) + '" stroke-width="1.3"/>' +
      '<path d="M72 36 Q80 40 82 46" fill="none" stroke="' + mix(s.trim, -0.3) + '" stroke-width="1"/>' +
      '<g transform="rotate(14 92 54)"><path d="M80 44 h22 v20 h-22 l-4 -10z" fill="' + h.url('tag') + '" stroke="#8a7a5a" stroke-width=".8"/><circle cx="81.5" cy="54" r="1.4" fill="#8a7a5a"/>' + h.mark(s.mark, 84, 45, 18) + '</g>' +
      '</g>';
  };
  DEFAULTS.vial = ['#4a9a6a', CORK, '#f4ecd6'];

  /* A short sword lying across the box: the blade is the body tint as steel,
     the guard and pommel the trim, the grip wrapped in the detail. The mark
     is set in the pommel. */
  SHAPES.blade = function (s) {
    var b = s.body, t = s.trim, h = window.CraftResults._h;
    var steel = mix(b, 0.15);
    var defs = '<defs>' + h.blur('soft', 2.4) +
      h.grad('bl', 0, 0, 0, 1, [[0, mix(steel, 0.75)], [0.48, mix(steel, 0.35)], [0.52, mix(steel, -0.25)], [1, mix(steel, -0.5)]]) +
      h.grad('gd', 0, 0, 0, 1, [[0, mix(t, 0.55)], [0.4, t], [1, mix(t, -0.55)]]) +
      h.grad('gp', 0, 0, 1, 0, [[0, mix(s.detail, -0.5)], [0.4, mix(s.detail, 0.15)], [1, mix(s.detail, -0.55)]]) +
      h.rgrad('pm', 0.36, 0.3, 0.75, [[0, mix(t, 0.7)], [0.4, t], [1, mix(t, -0.6)]]) + '</defs>';
    var wraps = '';
    for (var i = 0; i < 5; i++) wraps += '<path d="M' + (26 + i * 3.8) + ' 65.5 l3 -7" stroke="' + mix(s.detail, -0.55) + '" stroke-width="1.2" opacity=".7"/>';
    return defs + h.floor(62, 44) +
      '<g transform="rotate(-38 60 66) translate(60 62) scale(1.12 1.25) translate(-60 -62)">' +
      /* blade: two bevels meeting at the ridge, a fuller, and the edge's hard glint */
      '<path d="M50 58 H100 L112 62 L100 66 H50Z" fill="' + h.url('bl') + '"/>' +
      '<path d="M50 62 H106" stroke="' + mix(steel, -0.45) + '" stroke-width=".6" opacity=".7"/>' +
      '<path d="M54 61 H90" stroke="' + mix(steel, -0.3) + '" stroke-width="1.6" stroke-linecap="round" opacity=".55"/>' +
      '<path d="M50 58.4 H100 L111 61.6" fill="none" stroke="#fff" stroke-opacity=".85" stroke-width=".9"/>' +
      '<path d="M70 59 H84" stroke="#fff" stroke-width="1.6" stroke-linecap="round" opacity=".7"/>' +
      '<path d="M50 58 H100 L112 62 L100 66 H50Z" fill="none" stroke="rgb(20 14 10 / .45)" stroke-width=".8"/>' +
      /* guard */
      '<rect x="44" y="49" width="7" height="26" rx="3" fill="' + h.url('gd') + '" stroke="rgb(20 14 10 / .45)" stroke-width=".8"/>' +
      '<circle cx="47.5" cy="49.5" r="3" fill="' + h.url('pm') + '"/><circle cx="47.5" cy="74.5" r="3" fill="' + h.url('pm') + '"/>' +
      /* grip, wrapped */
      '<rect x="24" y="58.5" width="20" height="7" rx="2" fill="' + h.url('gp') + '" stroke="rgb(20 14 10 / .4)" stroke-width=".7"/>' +
      wraps +
      /* pommel with the mark set in it */
      '<circle cx="18" cy="62" r="8" fill="' + h.url('pm') + '" stroke="rgb(20 14 10 / .45)" stroke-width=".8"/>' +
      '<circle cx="18" cy="62" r="5.2" fill="rgb(20 14 10 / .35)"/>' + h.mark(s.mark, 12.2, 56.2, 11.6) +
      '<path d="M13 58 a7 7 0 0 1 6 -3.4" fill="none" stroke="#fff" stroke-opacity=".7" stroke-width="1.2" stroke-linecap="round"/>' +
      '</g>';
  };
  DEFAULTS.blade = ['#8c96a3', '#c9a24a', '#7a4a2a'];

  /* A round buckler: the trim is the iron rim and boss, the body the face
     (scales, hide or wood grain all read as overlapping plates), the detail
     the crossed straps. The mark sits on the boss. */
  SHAPES.shield = function (s) {
    var b = s.body, t = s.trim, h = window.CraftResults._h;
    var defs = '<defs>' + h.blur('soft', 2.4) +
      h.rgrad('rim', 0.38, 0.32, 0.8, [[0, mix(t, 0.7)], [0.45, t], [0.85, mix(t, -0.5)], [1, mix(t, -0.65)]]) +
      h.rgrad('sc', 0.5, 0.15, 0.9, [[0, mix(b, 0.45)], [0.55, b], [1, mix(b, -0.55)]]) +
      h.rgrad('lit', 0.34, 0.28, 0.8, [[0, '#fff', 0.35], [0.4, '#fff', 0], [0.75, '#000', 0], [1, '#000', 0.35]]) +
      h.grad('st', 0, 0, 0, 1, [[0, mix(s.detail, 0.35)], [0.5, s.detail], [1, mix(s.detail, -0.5)]]) +
      '<pattern id="' + h.id('pt') + '" width="12" height="9" patternUnits="userSpaceOnUse">' +
      '<rect width="12" height="9" fill="' + mix(b, -0.35) + '"/>' +
      '<path d="M-6 9 Q0 -3 6 9Z M6 9 Q12 -3 18 9Z" fill="' + h.url('sc') + '" stroke="' + mix(b, -0.6) + '" stroke-width=".6"/>' +
      '<path d="M0 4.5 Q6 -7.5 12 4.5Z" fill="' + h.url('sc') + '" stroke="' + mix(b, -0.6) + '" stroke-width=".6"/></pattern>' +
      '<clipPath id="' + h.id('face') + '"><circle cx="60" cy="60" r="38"/></clipPath></defs>';
    var rivets = '';
    for (var i = 0; i < 12; i++) {
      var a = i / 12 * Math.PI * 2, x = (60 + Math.cos(a) * 44).toFixed(1), y = (60 + Math.sin(a) * 44).toFixed(1);
      rivets += '<circle cx="' + x + '" cy="' + y + '" r="1.9" fill="' + mix(t, 0.5) + '" stroke="' + mix(t, -0.6) + '" stroke-width=".6"/>';
    }
    function strap(d) {
      return '<path d="' + d + '" stroke="' + mix(s.detail, -0.55) + '" stroke-width="11" stroke-linecap="round"/>' +
        '<path d="' + d + '" stroke="' + h.url('st') + '" stroke-width="9.4" stroke-linecap="round"/>' +
        '<path d="' + d + '" stroke="' + mix(s.detail, 0.55) + '" stroke-width="1" stroke-dasharray="2.4 2.2" opacity=".75"/>';
    }
    return defs + h.floor(60, 40) +
      '<circle cx="60" cy="60" r="49" fill="' + h.url('rim') + '"/>' +
      '<circle cx="60" cy="60" r="49" fill="none" stroke="rgb(20 14 10 / .5)" stroke-width="1"/>' +
      '<path d="M22 44 A42 42 0 0 1 56 15" fill="none" stroke="#fff" stroke-opacity=".75" stroke-width="2.2" stroke-linecap="round"/>' + rivets +
      '<circle cx="60" cy="60" r="38.6" fill="rgb(20 14 10 / .5)"/>' +
      '<g clip-path="' + h.url('face') + '"><rect x="20" y="20" width="80" height="80" fill="' + h.url('pt') + '"/>' +
      strap('M30 36 L90 84') + strap('M30 84 L90 36') +
      '<circle cx="60" cy="60" r="38" fill="' + h.url('lit') + '"/></g>' +
      '<circle cx="60" cy="60" r="14" fill="rgb(20 14 10 / .35)" transform="translate(1.4 2)"/>' +
      '<circle cx="60" cy="60" r="13" fill="' + h.url('rim') + '" stroke="rgb(20 14 10 / .55)" stroke-width=".9"/>' +
      h.mark(s.mark, 52, 52, 16) +
      '<path d="M50 54 a12 12 0 0 1 8 -6" fill="none" stroke="#fff" stroke-opacity=".8" stroke-width="1.6" stroke-linecap="round"/>';
  };
  DEFAULTS.shield = ['#3f9a84', '#8c96a3', '#a8743e'];

  /* A ring: the band is the trim as metal, the stone the body, cut with
     facets that catch the light. The detail tints the claws. */
  SHAPES.ring = function (s) {
    var b = s.body, t = s.trim, h = window.CraftResults._h;
    var defs = '<defs>' + h.blur('soft', 2.4) + h.blur('glow', 5) +
      h.grad('bd', 0, 0, 1, 1, [[0, mix(t, 0.75)], [0.35, t], [0.65, mix(t, -0.45)], [1, mix(t, 0.2)]]) +
      h.grad('bk', 0, 0, 1, 0, [[0, mix(t, -0.6)], [0.5, mix(t, -0.2)], [1, mix(t, -0.65)]]) +
      h.rgrad('gm', 0.4, 0.35, 0.7, [[0, mix(b, 0.6)], [0.5, b], [1, mix(b, -0.6)]]) + '</defs>';
    return defs + h.floor(60, 34) +
      '<ellipse cx="60" cy="40" rx="18" ry="12" fill="' + b + '" opacity=".35" filter="' + h.url('glow') + '"/>' +
      /* the band's far side, then the near side over it */
      '<ellipse cx="60" cy="76" rx="34" ry="24" fill="none" stroke="' + h.url('bk') + '" stroke-width="9"/>' +
      '<path d="M26 76 A34 24 0 0 0 94 76" fill="none" stroke="' + h.url('bd') + '" stroke-width="10"/>' +
      '<path d="M30 82 A31 20 0 0 0 66 99" fill="none" stroke="#fff" stroke-opacity=".7" stroke-width="2" stroke-linecap="round"/>' +
      '<path d="M26 76 A34 24 0 0 0 94 76" fill="none" stroke="rgb(20 14 10 / .35)" stroke-width=".8" transform="translate(0 5)"/>' +
      /* setting and claws */
      '<path d="M46 52 L50 40 H70 L74 52Z" fill="' + h.url('bd') + '" stroke="rgb(20 14 10 / .45)" stroke-width=".8"/>' +
      '<path d="M48 42 L44 30 M72 42 L76 30 M54 40 L52 26 M66 40 L68 26" stroke="' + mix(s.detail, 0.1) + '" stroke-width="2.6" stroke-linecap="round"/>' +
      /* the cut stone */
      '<path d="M44 30 L52 20 H68 L76 30 L60 46Z" fill="' + h.url('gm') + '"/>' +
      '<path d="M44 30 H76 M52 20 L56 30 L60 46 L64 30 L68 20 M56 30 L60 20 L64 30" fill="none" stroke="' + mix(b, 0.55) + '" stroke-width=".7" opacity=".8"/>' +
      '<path d="M52 20 H68 L64 30 H56Z" fill="#fff" opacity=".28"/>' +
      '<path d="M44 30 L52 20 H68 L76 30 L60 46Z" fill="none" stroke="rgb(20 14 10 / .5)" stroke-width=".8"/>' +
      '<path d="M53 23 L57 23" stroke="#fff" stroke-width="1.6" stroke-linecap="round"/>' +
      '<path d="M80 22 l1.2 3.4 3.4 1.2 -3.4 1.2 -1.2 3.4 -1.2 -3.4 -3.4 -1.2 3.4 -1.2z" fill="#fff" opacity=".9"/>';
  };
  DEFAULTS.ring = ['#4a7ad8', '#d1ad52', '#d1ad52'];

  /* Anything else: a cloth bundle tied with cord, its tag carrying the
     mark, so an unknown shape still shows what went in. */
  SHAPES.bundle = function (s) {
    var b = s.body, h = window.CraftResults._h;
    var defs = '<defs>' + h.blur('soft', 2.4) +
      h.rgrad('cl', 0.4, 0.3, 0.85, [[0, mix(b, 0.45)], [0.5, b], [1, mix(b, -0.55)]]) +
      h.grad('tag', 0, 0, 0, 1, [[0, '#f8f2e4'], [1, '#e2d5b8']]) + '</defs>';
    return defs + h.floor(60, 36) +
      '<path d="M24 98 Q20 70 34 56 Q46 46 60 46 Q74 46 86 56 Q100 70 96 98 Q60 108 24 98Z" fill="' + h.url('cl') + '" stroke="rgb(20 14 10 / .45)" stroke-width=".9"/>' +
      '<path d="M40 60 Q46 80 42 98 M60 50 Q62 78 60 104 M80 60 Q74 80 78 98" fill="none" stroke="' + mix(b, -0.5) + '" stroke-width="1" opacity=".55"/>' +
      '<path d="M50 46 Q44 30 52 22 Q60 30 60 44 Q60 30 68 22 Q76 30 70 46" fill="' + h.url('cl') + '" stroke="rgb(20 14 10 / .45)" stroke-width=".9"/>' +
      '<path d="M46 48 Q60 54 74 48" fill="none" stroke="' + mix(s.trim, -0.35) + '" stroke-width="4" stroke-linecap="round"/>' +
      '<path d="M46 48 Q60 54 74 48" fill="none" stroke="' + mix(s.trim, 0.3) + '" stroke-width="1.4" stroke-linecap="round"/>' +
      '<path d="M34 64 Q42 58 50 60" fill="none" stroke="#fff" stroke-opacity=".5" stroke-width="2" stroke-linecap="round"/>' +
      '<path d="M74 50 Q84 56 86 64" fill="none" stroke="' + mix(s.trim, -0.35) + '" stroke-width="1.2"/>' +
      '<g transform="rotate(10 92 72)"><path d="M80 62 h22 v20 h-22 l-4 -10z" fill="' + h.url('tag') + '" stroke="#8a7a5a" stroke-width=".8"/><circle cx="81.5" cy="72" r="1.4" fill="#8a7a5a"/>' + h.mark(s.mark, 84, 63, 18) + '</g>';
  };
  DEFAULTS.bundle = ['#7a6a9a', '#c9a54a', '#efe6d2'];

  /* The ways of drawing the same shapes: "drawn" is the illustrated set
     above, and craft_results_real.js adds the lifelike "real" set, which is
     the default once it is loaded. A style must draw every shape. */
  var STYLES = {drawn: SHAPES};
  function addStyle(name, shapes) {
    for (var k in SHAPES) if (typeof shapes[k] !== 'function') return;
    STYLES[name] = shapes;
  }
  /* The campaign's Customize choice rides on <html data-craft-art>, in
     the owner's words; the styles here keep their own names. */
  var ART_ATTR = {illustrated: 'drawn', lifelike: 'real'};
  function styleOf(o) {
    var want = o && o.style;
    if (!want && typeof document !== 'undefined') want = ART_ATTR[document.documentElement.getAttribute('data-craft-art')];
    if (want && Object.prototype.hasOwnProperty.call(STYLES, want)) return STYLES[want];
    return STYLES.real || SHAPES;
  }

  /* spec: {shape, body, trim, detail, mark: {picture, tint}}. Tints are hex. */
  function draw(spec, opts) {
    var s = clean(spec);
    U = 'crx' + (++seq) + '-';
    return styleOf(opts)[s.shape](s);
  }
  /* Every look is someone's data (a stored copy, an AI's recipe), and the
     markup is inserted as HTML: only a known shape, #rrggbb colours and a
     picture key from CraftPictures' own list get through. */
  function clean(spec) {
    var s = spec && typeof spec === 'object' ? spec : {};
    var shape = Object.prototype.hasOwnProperty.call(SHAPES, s.shape) ? s.shape : 'bundle', d = DEFAULTS[shape];
    function col(v, dflt) { return typeof v === 'string' && HEX.test(v) ? v : dflt; }
    var m = s.mark && typeof s.mark === 'object' ? s.mark : null, CP = window.CraftPictures;
    return {
      shape: shape, body: col(s.body, d[0]), trim: col(s.trim, d[1]), detail: col(s.detail, d[2]),
      mark: m && typeof m.picture === 'string' && CP && CP.has(m.picture) ? {picture: m.picture, tint: col(m.tint, '#8a8a8a')} : null
    };
  }
  function svg(spec, size, opts) {
    var z = size || 64, o = opts || {};
    return '<svg class="crx' + (o.cls ? ' ' + o.cls : '') + '" viewBox="0 0 120 120" width="' + z + '" height="' + z + '"' +
      (o.label ? ' role="img" aria-label="' + String(o.label).replace(/[&<>"]/g, '') + '"' : ' aria-hidden="true"') + '>' + draw(spec, o) + '</svg>';
  }

  /* From a result shape and the materials in recipe order to a spec: the
     first material colours the body, the second the trim, the third the
     detail, and the first one with a picture of its own is the mark. A
     shape's own defaults fill what no material gave. */
  function fromMaterials(shape, mats) {
    var m = (mats || []).filter(function (x) { return x && HEX.test(x.tint || ''); });
    var d = DEFAULTS[shape] || DEFAULTS.bundle;
    return {
      shape: SHAPES[shape] ? shape : 'bundle',
      body: m[0] ? m[0].tint : d[0], trim: m[1] ? m[1].tint : d[1], detail: m[2] ? m[2].tint : d[2],
      mark: m.length > 1 ? {picture: m[1].picture, tint: m[1].tint} : null
    };
  }

  return {svg: svg, draw: draw, clean: clean, fromMaterials: fromMaterials, addStyle: addStyle,
    shapes: function () { return Object.keys(SHAPES); }, styles: function () { return Object.keys(STYLES); }, _shapes: SHAPES, _defaults: DEFAULTS, _mix: mix, _h: {id: id, url: url, grad: grad, rgrad: rgrad, blur: blur, floor: floor, mark: mark}};
})();
