/**
 * chip_ink.js - readable words for a tag chip.
 *
 * A chip shows the owner's colour as words on a faint tint of that colour.
 * paint() keeps the tint and shades the words, once for each theme, until they
 * reach 4.5:1 on it, keeping the hue. The two word colours go in --chip-l and
 * --chip-d and the .chip-ink class picks one by theme. Mirrors
 * internal/colour/chip.go: same tint, surfaces, target and step.
 */
(function (root) {
  'use strict';

  var TINT = 34 / 255;
  var LIGHT = ['#ffffff', '#f3f4f6'];
  var DARK = ['#1f2937', '#111827'];

  function clamp(v, a, b) { return Math.max(a, Math.min(b, v)); }
  function valid(h) { return /^#[0-9a-fA-F]{6}$/.test(String(h)); }
  function hexToRgb(h) { return [1, 3, 5].map(function (i) { return parseInt(h.slice(i, i + 2), 16) / 255; }); }
  function rgbToHex(c) {
    return '#' + c.map(function (v) {
      var n = Math.floor(clamp(v, 0, 1) * 255 + 0.5);
      return (n < 16 ? '0' : '') + n.toString(16);
    }).join('');
  }
  function lin(c) { return c <= 0.04045 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4); }
  function delin(c) { return c <= 0.0031308 ? 12.92 * c : 1.055 * Math.pow(c, 1 / 2.4) - 0.055; }
  function luminance(h) { var c = hexToRgb(h).map(lin); return 0.2126 * c[0] + 0.7152 * c[1] + 0.0722 * c[2]; }
  function contrast(a, b) {
    var A = luminance(a), B = luminance(b);
    return (Math.max(A, B) + 0.05) / (Math.min(A, B) + 0.05);
  }
  function toLab(rgb) {
    var r = lin(rgb[0]), g = lin(rgb[1]), b = lin(rgb[2]);
    var l = Math.cbrt(0.4122214708 * r + 0.5363325363 * g + 0.0514459929 * b);
    var m = Math.cbrt(0.2119034982 * r + 0.6806995451 * g + 0.1073969566 * b);
    var s = Math.cbrt(0.0883024619 * r + 0.2817188376 * g + 0.6299787005 * b);
    return [0.2104542553 * l + 0.7936177850 * m - 0.0040720468 * s,
      1.9779984951 * l - 2.4285922050 * m + 0.4505937099 * s,
      0.0259040371 * l + 0.7827717662 * m - 0.8086757660 * s];
  }
  function fromLab(lab) {
    var l = Math.pow(lab[0] + 0.3963377774 * lab[1] + 0.2158037573 * lab[2], 3);
    var m = Math.pow(lab[0] - 0.1055613458 * lab[1] - 0.0638541728 * lab[2], 3);
    var s = Math.pow(lab[0] - 0.0894841775 * lab[1] - 1.2914855480 * lab[2], 3);
    return [delin(4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s),
      delin(-1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s),
      delin(-0.0041960863 * l - 0.7034186147 * m + 1.7076147010 * s)];
  }
  function oklch(h) {
    var L = toLab(hexToRgb(h));
    return [L[0], Math.hypot(L[1], L[2]), (Math.atan2(L[2], L[1]) * 180 / Math.PI + 360) % 360];
  }
  function lchRgb(L, C, h) { var r = h * Math.PI / 180; return fromLab([L, C * Math.cos(r), C * Math.sin(r)]); }
  function inGamut(c) { return c.every(function (v) { return v >= -0.0005 && v <= 1.0005; }); }
  // Keeps lightness and hue, gives up chroma until the colour exists in sRGB.
  function lchHex(L, C, h) {
    var c = lchRgb(L, C, h);
    if (inGamut(c)) return rgbToHex(c);
    var lo = 0, hi = C;
    for (var i = 0; i < 24; i++) {
      var mid = (lo + hi) / 2;
      if (inGamut(lchRgb(L, mid, h))) lo = mid; else hi = mid;
    }
    return rgbToHex(lchRgb(L, lo, h));
  }
  function over(fg, bg, a) {
    var F = hexToRgb(fg), B = hexToRgb(bg);
    return rgbToHex(F.map(function (v, i) { return v * a + B[i] * (1 - a); }));
  }

  // Moves lightness (never hue) until the words reach the target on every
  // background given.
  function readable(hex, bgs, target, darker) {
    function worst(h) { return Math.min.apply(null, bgs.map(function (b) { return contrast(h, b); })); }
    if (worst(hex) >= target) return hex;
    var c = oklch(hex), L = c[0], step = darker ? -0.004 : 0.004, out = hex;
    for (var i = 0; i < 240; i++) {
      L += step;
      if (L <= 0.02 || L >= 0.995) break;
      out = lchHex(L, c[1], c[2]);
      if (worst(out) >= target) break;
    }
    return out;
  }

  // ink returns {light, dark}: the word colours for a chip tinted from hex.
  // Anything that is not #rrggbb comes back unchanged.
  function ink(hex) {
    if (!valid(hex)) return { light: hex, dark: hex };
    hex = hex.toLowerCase();
    function tints(surfaces) { return surfaces.map(function (s) { return over(hex, s, TINT); }); }
    return { light: readable(hex, tints(LIGHT), 4.5, true), dark: readable(hex, tints(DARK), 4.5, false) };
  }

  // paint styles a chip element: the tint, the two word colours, and the
  // class that picks between them. A colour that is not #rrggbb leaves the
  // chip plain.
  function paint(el, hex) {
    if (!el || !valid(hex)) return;
    var c = ink(hex);
    el.classList.add('chip-ink');
    el.style.backgroundColor = hex.toLowerCase() + '22';
    el.style.setProperty('--chip-l', c.light);
    el.style.setProperty('--chip-d', c.dark);
  }

  var api = { ink: ink, paint: paint, contrast: contrast, over: over, TINT: TINT, LIGHT: LIGHT, DARK: DARK };
  root.ChronicleChip = api;
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
})(typeof window !== 'undefined' ? window : globalThis);
