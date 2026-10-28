/**
 * craft_pictures.js -- the crafting material pictures (window.CraftPictures)
 *
 * One library of small drawings, each named for what it shows (a leaf, a
 * flask, an ingot), never for a material. A material kind starts on a default
 * picture and tint, a campaign can swap those, and one item can pick its own;
 * whatever is chosen, the drawing comes from here, so the Materials tab, the
 * recipe editor, the Crafting block and the bench all draw the same thing.
 *
 * Every drawing sits in a 40×40 box, takes one tint and is lit from the top
 * left. Shading comes from gradients laid over the tint, so one drawing works
 * in any colour. Each copy of a drawing gets its own id prefix, because the
 * same picture often appears many times on one page.
 *
 * The parts that move are <g class="cpx-m"> carrying their own animation in
 * --an, and the picture's motion names how it moves. A picture moves only
 * while the thing holding it (marked cpx-touch) is pointed at or focused, and
 * never under Calm, Off or a device asking for less motion; the CSS for that
 * is injected once here. The keys must match Pictures in
 * internal/plugins/crafting/library.go.
 */
window.CraftPictures = (function () {
  'use strict';
  var INK = 'rgb(0 0 0 / .45)';
  var K = 'stroke="' + INK + '" stroke-width="1.1" stroke-linejoin="round" stroke-linecap="round"';
  var DK = 'rgb(0 0 0 / .32)', LT = 'rgb(255 255 255 / .6)';
  var CORK = '#9a7650', GOLD = '#d1ad52', BONE = '#efe6d2', LEAF = '#62903a', STEM = '#56683a', PAPER = '#efe3c4', IRON = '#8a857c';
  var C = '#8a8a8a', U = 'cpx', seq = 0;

  /* The shared gradients; a drawing names them with url('v') and only the
     ones it uses are written into its copy. */
  var GRADS = {
    v: '<linearGradient id="@v" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#fff" stop-opacity=".5"/><stop offset=".42" stop-color="#fff" stop-opacity="0"/><stop offset=".58" stop-color="#000" stop-opacity="0"/><stop offset="1" stop-color="#000" stop-opacity=".4"/></linearGradient>',
    t: '<linearGradient id="@t" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#fff" stop-opacity=".45"/><stop offset=".45" stop-color="#fff" stop-opacity="0"/><stop offset=".6" stop-color="#000" stop-opacity="0"/><stop offset="1" stop-color="#000" stop-opacity=".35"/></linearGradient>',
    h: '<linearGradient id="@h" x1="0" y1="0" x2="1" y2="0"><stop offset="0" stop-color="#000" stop-opacity=".2"/><stop offset=".28" stop-color="#fff" stop-opacity=".4"/><stop offset=".5" stop-color="#fff" stop-opacity="0"/><stop offset="1" stop-color="#000" stop-opacity=".42"/></linearGradient>',
    r: '<radialGradient id="@r" cx=".36" cy=".3" r=".78"><stop offset="0" stop-color="#fff" stop-opacity=".75"/><stop offset=".3" stop-color="#fff" stop-opacity="0"/><stop offset=".6" stop-color="#000" stop-opacity="0"/><stop offset="1" stop-color="#000" stop-opacity=".45"/></radialGradient>',
    g: '<linearGradient id="@g" x1="0" y1="0" x2="1" y2="0"><stop offset="0" stop-color="#fff" stop-opacity=".55"/><stop offset=".2" stop-color="#fff" stop-opacity=".12"/><stop offset=".62" stop-color="#fff" stop-opacity=".04"/><stop offset=".84" stop-color="#fff" stop-opacity=".38"/><stop offset="1" stop-color="#fff" stop-opacity=".1"/></linearGradient>',
    b: '<filter id="@b" x="-50%" y="-50%" width="200%" height="200%"><feGaussianBlur stdDeviation="1.3"/></filter>'
  };
  function url(g) { return 'url(#' + U + g + ')'; }

  /* A shape three times over: the tint, the light on it, the ink line. */
  function volE(tag, f, g) {
    return '<' + tag + ' fill="' + (f || C) + '"/><' + tag + ' fill="' + url(g || 'v') + '"/><' + tag + ' fill="none" ' + K + '/>';
  }
  function vol(d, f, g) { return volE('path d="' + d + '"', f, g); }
  function circ(x, y, r, f, g) { return volE('circle cx="' + x + '" cy="' + y + '" r="' + r + '"', f, g || 'r'); }
  function sh(d, o) { return '<path d="' + d + '" fill="rgb(0 0 0 / ' + (o || 0.2) + ')"/>'; }
  function hl(d, o) { return '<path d="' + d + '" fill="rgb(255 255 255 / ' + (o || 0.5) + ')"/>'; }
  function ln(d, col, w, o) {
    return '<path d="' + d + '" fill="none" stroke="' + col + '" stroke-width="' + (w || 1) + '" stroke-linecap="round" stroke-linejoin="round"' + (o != null ? ' opacity="' + o + '"' : '') + '/>';
  }
  /* A thick line with an ink edge, for roots, stems and vessels. */
  function rope(d, w, col) { return ln(d, INK, w + 1.2) + ln(d, col || C, w); }
  function shadow(rx, cx) { return '<ellipse cx="' + (cx || 20) + '" cy="36.6" rx="' + (rx || 11) + '" ry="1.9" fill="rgb(0 0 0 / .18)"/>'; }
  /* A moving part. cls cpx-hide keeps it out of sight until it moves. */
  function M(an, o, s, cls) { return '<g class="cpx-m' + (cls ? ' ' + cls : '') + '" style="--an:' + an + ';transform-origin:' + o + '">' + s + '</g>'; }
  function spark(x, y, s, delay) {
    var a = +(s * 0.28).toFixed(2), b = +(s * 0.72).toFixed(2);
    var d = 'M' + x + ' ' + (y - s) + 'l' + a + ' ' + b + ' ' + b + ' ' + a + '-' + b + ' ' + a + '-' + a + ' ' + b + '-' + a + '-' + b + '-' + b + '-' + a + ' ' + b + '-' + a + 'z';
    return M('cpx-twinkle 1.8s ease-in-out ' + (delay || 0) + 's infinite', x + 'px ' + y + 'px', '<path d="' + d + '" fill="#fff"/>', 'cpx-hide');
  }
  /* A bright band that crosses a shape once a beat. */
  function sweep(d) {
    return '<clipPath id="' + U + 's"><path d="' + d + '"/></clipPath><g clip-path="' + url('s') + '">' +
      M('cpx-sweep 2s ease-in-out infinite', '0 0', '<path d="M-10 40L2 0H7L-5 40Z" fill="rgb(255 255 255 / .6)"/>', 'cpx-hide') + '</g>';
  }
  function rnd(seed) {
    var s = seed;
    return function () { s = (s * 16807) % 2147483647; return (s - 1) / 2147483646; };
  }
  /* Small marks scattered in an ellipse, for grain, fur and speckle. */
  function speckle(seed, n, cx, cy, rx, ry, col, r) {
    var g = rnd(seed), out = '';
    for (var i = 0; i < n; i++) {
      var t = g() * 6.283, d = Math.sqrt(g());
      out += '<circle cx="' + (cx + Math.cos(t) * rx * d).toFixed(1) + '" cy="' + (cy + Math.sin(t) * ry * d).toFixed(1) + '" r="' + (r * (0.6 + g() * 0.6)).toFixed(2) + '"/>';
    }
    return '<g fill="' + col + '">' + out + '</g>';
  }
  /* A small leaf lying along +x from (x, y). */
  function leaflet(x, y, ang, s, f) {
    return '<g transform="translate(' + x + ' ' + y + ') rotate(' + ang + ') scale(' + s + ')">' +
      vol('M0 0C3-3.4 8-3.4 10 0C8 3.4 3 3.4 0 0Z', f) + ln('M.8 0H8.6', DK, 0.6) + '</g>';
  }
  /* Liquid behind glass: the surface rolls and tips, bubbles rise. inside is
     the vessel's inner outline, y the surface height. */
  function liquid(inside, y, xs, bottom) {
    var wave = 'M-8 ' + y, top = wave;
    for (var x = -8; x < 56; x += 8) { wave += 'q2-1.2 4 0t4 0'; }
    top = wave; wave += 'V46H-8Z';
    var bub = '';
    (xs || []).forEach(function (bx, i) {
      bub += M('cpx-rise ' + (1.4 + i * 0.3) + 's ease-in ' + (i * 0.45) + 's infinite', bx + 'px ' + bottom + 'px',
        '<circle cx="' + bx + '" cy="' + bottom + '" r="' + (0.9 + (i % 2) * 0.5) + '" fill="none" stroke="rgb(255 255 255 / .75)" stroke-width=".6"/>', 'cpx-hide');
    });
    return '<clipPath id="' + U + 'c"><path d="' + inside + '"/></clipPath><g clip-path="' + url('c') + '">' +
      M('cpx-slosh 1.7s ease-in-out infinite', '20px ' + y + 'px',
        M('cpx-wave 1.1s linear infinite', '0 0', '<path d="' + wave + '" fill="' + C + '"/>' + ln(top, 'rgb(255 255 255 / .5)', 0.8))) +
      '<rect width="40" height="40" fill="' + url('h') + '"/>' + bub + '</g>';
  }
  /* Glass over the liquid: its sheen, a hard highlight and the outline. */
  function glass(d, spec) {
    return '<path d="' + d + '" fill="' + url('g') + '"/>' + ln(spec, 'rgb(255 255 255 / .8)', 1.3) + '<path d="' + d + '" fill="none" ' + K + '/>';
  }

  /* key, label, motion, draw() in the order the picker shows them. A draw
     reads the tint from C. */
  var P = [
    ['leaf', 'Leaf', 'sway', function () {
      var d = 'M20 3C31 9 34.5 23 21 36C7 25 8 10 20 3Z';
      return shadow(8) + M('cpx-sway 1.9s ease-in-out infinite', '21px 37px',
        vol(d) + hl('M19 7.5C13 13 11 20 13.5 27.5C13 19.5 15 13 19 7.5Z', 0.3) +
        ln('M19.6 12l-4.8-3.6M19.7 17l-6.4-3.4M20 22l-6.6-2.4M20.4 27l-5.2-1.2M20 13l5.4-4M20.2 18l7-3.4M20.5 23l7-2.4M20.8 28l5.4-1.2', DK, 0.6) +
        ln('M20 6Q19 20 21 35', DK, 1.1) + ln('M21 35.5l1.6 2.6', STEM, 1.6) +
        '<circle cx="26.5" cy="15" r="1.2" fill="rgb(255 255 255 / .75)" stroke="rgb(0 0 0 / .2)" stroke-width=".4"/>');
    }],
    ['sprig', 'Sprig', 'sway', function () {
      return shadow(7) + M('cpx-sway 2.1s ease-in-out infinite', '20px 37px',
        rope('M20 37C20 28 19 18 21 6', 1.2, STEM) +
        leaflet(20, 27, -175, 1.2) + leaflet(20, 27, -5, 1.2) + leaflet(20, 19, -165, 1.1) + leaflet(20.2, 19, -15, 1.1) +
        leaflet(20.4, 12, -155, 0.95) + leaflet(20.6, 12, -25, 0.95) + leaflet(21, 6.5, -82, 0.8) +
        '<g fill="#fff" opacity=".8"><circle cx="11" cy="24.5" r=".8"/><circle cx="29" cy="16" r=".7"/></g>');
    }],
    ['flower', 'Flower', 'sway', function () {
      var petals = '';
      for (var a = 0; a < 360; a += 60) {
        petals += volE('ellipse cx="20" cy="7.2" rx="3.7" ry="5.6" transform="rotate(' + a + ' 20 13)"') +
          '<path d="M20 11.6V4" transform="rotate(' + a + ' 20 13)" stroke="' + DK + '" stroke-width=".55"/>';
      }
      return shadow(7) + M('cpx-sway 2s ease-in-out infinite', '20px 37px',
        rope('M20 37C19 31 21 26 20 19', 1.4, STEM) + leaflet(20, 31, -150, 1.1, LEAF) + leaflet(20.2, 27, -35, 0.9, LEAF) +
        petals + circ(20, 13, 3.8, '#f2c94c') + speckle(7, 9, 20, 13, 2.4, 2.4, '#a8741a', 0.35));
    }],
    ['root', 'Root', 'none', function () {
      return shadow(12) +
        rope('M17 15C13 17 9 18 6 22', 2) + rope('M23 17C27 18 31 21 33 26', 2) + rope('M19 26C16 29 12 30 9.5 33.5', 1.6) + rope('M22 27C25 29 27 32 28 35.5', 1.6) +
        ln('M6 22l-2 3M33 26l1 3.5M9.5 33.5l-2.5 1M28 35.5l2 .6M11 19.5l-1-2.5M29 21.5l1.6-2', DK, 0.6) +
        vol('M17 4C21 3 25 6 24 11C23 16 22 19 23 24C24 28 22 32 20 35.5C20 31 19 28 18 24C17 19 15 15 16 10C16 7 16 5 17 4Z', null, 'h') +
        ln('M16.4 9q3.6 1.2 7.2 0M16.4 14q3.4 1.3 6.6 0M17.6 20q3 1 5.4 0M18.6 26q2.4.8 4 0', DK, 0.7) +
        ln('M17 4C15 1.4 13 1 11.5 2M19 4c0-2 1-3 3-4M18 4c-.6-2-.4-3 .6-4', LEAF, 1.3);
    }],
    ['moss', 'Moss clump', 'none', function () {
      var s = shadow(15) + '<path d="M5 33c1-3 6-4 15-4s14 1 15 4c-2 2-8 3-15 3S7 35 5 33Z" fill="#7b7468"/>';
      [[11, 27, 7], [29, 27, 7], [20, 22, 9], [14, 30.5, 6], [26, 31, 6]].forEach(function (b) { s += circ(b[0], b[1], b[2]); });
      return s + speckle(11, 26, 20, 26, 13, 7, 'rgb(0 0 0 / .22)', 0.6) + speckle(12, 22, 19, 23, 12, 6, 'rgb(255 255 255 / .38)', 0.5) +
        ln('M18 14q-1-5-3-8M22 13.5q1-4 4-6.5M20 13q0-4 .4-7', '#8a6a3a', 0.7) +
        '<g fill="#b8752e" ' + K + ' stroke-width=".5"><ellipse cx="15" cy="6" rx=".9" ry="1.5" transform="rotate(-30 15 6)"/><ellipse cx="26" cy="7" rx=".9" ry="1.5" transform="rotate(35 26 7)"/><ellipse cx="20.4" cy="5.6" rx=".9" ry="1.5"/></g>';
    }],
    ['mushroom', 'Mushroom', 'none', function () {
      return shadow(13, 21) +
        vol('M16 23C16 28 15 32 14 35H26C25 32 24 28 24 23Z', BONE, 'h') + ln('M15.6 27.4q4.4 2.2 8.8 0', DK, 0.7) +
        vol('M7 21C12 24.5 28 24.5 33 21C28 26 12 26 7 21Z', '#d9c7a2') + ln('M11 22.6l1.4 1.4M15 23.4l.8 1.2M19 23.7v1.2M23 23.6l-.6 1.2M27 23.2l-1 1.2M31 22.2l-1.2 1.2', DK, 0.5) +
        vol('M5 21C5 12 12 6 20 6S35 12 35 21C30 23 10 23 5 21Z', null, 'r') +
        '<g fill="#fff8ec" ' + K + ' stroke-width=".5"><ellipse cx="13" cy="14" rx="2.4" ry="1.9"/><ellipse cx="21" cy="10" rx="1.9" ry="1.4"/><ellipse cx="27.5" cy="15" rx="2.2" ry="1.7"/><circle cx="17.5" cy="18" r="1.2"/><circle cx="30.5" cy="19" r="1"/><circle cx="9" cy="18.6" r=".9"/></g>' +
        vol('M30 30.5h3l.6 5h-4.2z', BONE, 'h') + vol('M27 31c0-3.2 2-5.2 4.5-5.2S36 27.8 36 31c-2 1-7 1-9 0z', null, 'r') +
        ln('M6 36.4q2-2 3-4M8 36.4q.6-1.8 2-3', LEAF, 0.9);
    }],
    ['seeds', 'Seed pouch', 'none', function () {
      var body = 'M12 13C10 17 7 21 7 26C7 32 12 35 20 35S33 32 33 26C33 21 30 17 28 13Z';
      return shadow(13) + vol(body) +
        '<clipPath id="' + U + 'k"><path d="' + body + '"/></clipPath><g clip-path="' + url('k') + '">' +
        ln('M6 16l28 0M6 19.5l28 0M6 23l28 0M6 26.5l28 0M6 30l28 0M6 33.5l28 0', 'rgb(0 0 0 / .12)', 0.5) +
        ln('M10 12v24M13.5 12v24M17 12v24M20.5 12v24M24 12v24M27.5 12v24M31 12v24', 'rgb(255 255 255 / .14)', 0.5) + '</g>' +
        vol('M14 10C16 8.8 24 8.8 26 10L27.5 13.2C24 14.6 16 14.6 12.5 13.2Z') + sh('M15 10.4c2-1.4 8-1.4 10 0-2 1-8 1-10 0z', 0.45) +
        ln('M12.5 13.4C16 15.2 24 15.2 27.5 13.4', '#5a4630', 1.8) + ln('M24.5 14.6c1 2 0 4 1 5.4M25.6 14.4c2 1 2 3 3.2 4.4', '#5a4630', 0.9) +
        '<g fill="#d9c58f" ' + K + ' stroke-width=".5"><ellipse cx="17" cy="8.6" rx="1.5" ry="2.2" transform="rotate(-20 17 8.6)"/><ellipse cx="20.4" cy="7.4" rx="1.5" ry="2.2"/><ellipse cx="23.6" cy="8.6" rx="1.5" ry="2.2" transform="rotate(25 23.6 8.6)"/>' +
        '<ellipse cx="33.8" cy="35" rx="1.3" ry="1.9" transform="rotate(70 33.8 35)"/><ellipse cx="36.6" cy="33.6" rx="1.2" ry="1.7" transform="rotate(20 36.6 33.6)"/></g>' +
        '<path d="M17 7.8l.4 1.4M20.2 6.4l.3 1.4M23.4 7.8l.2 1.3" stroke="rgb(255 255 255 / .6)" stroke-width=".5"/>';
    }],
    ['flask', 'Flask', 'slosh', function () {
      var d = 'M16.5 6H23.5V13.5L31.5 27A5.5 5.5 0 0 1 26.8 35H13.2A5.5 5.5 0 0 1 8.5 27L16.5 13.5Z';
      return shadow(12) + '<path d="' + d + '" fill="rgb(255 255 255 / .3)"/>' + liquid(d, 22, [15.5, 21, 25], 32) +
        glass(d, 'M17.6 14.6L11.2 25.6') + ln('M12.4 29.6l-.7 1.4', 'rgb(255 255 255 / .8)', 1.1) + ln('M27.5 24.5h-2.4M29.4 28h-2.6M25.6 21h-1.8', DK, 0.55) +
        volE('rect x="15.4" y="5" width="9.2" height="2.4" rx="1.1"', 'rgb(255 255 255 / .5)') + vol('M16.4 1.2H23.6L23.2 5.2H16.8Z', CORK, 'h') +
        speckle(3, 5, 20, 3.2, 2.6, 1.4, 'rgb(0 0 0 / .3)', 0.3);
    }],
    ['vial', 'Corked vial', 'slosh', function () {
      var d = 'M14 10.5H26V29.5A6 6 0 0 1 14 29.5Z';
      return shadow(8) + '<path d="' + d + '" fill="rgb(255 255 255 / .3)"/>' + liquid(d, 18, [17, 22], 32) +
        glass(d, 'M16.2 13.6V28.8') + volE('rect x="13" y="9.4" width="14" height="2.6" rx="1.2"', 'rgb(255 255 255 / .5)') +
        vol('M15 3.6H25L24.4 10H15.6Z', CORK, 'h') + speckle(4, 6, 20, 6.8, 3.6, 2.4, 'rgb(0 0 0 / .3)', 0.32) +
        ln('M26 11q4.4.6 5.6 5', '#6b5a44', 0.7) + '<g transform="rotate(14 31.6 18.6)">' + vol('M29 15.6h5.2v6h-5.2z', PAPER, 't') + ln('M30 17.6h3.2M30 19.4h2.2', 'rgb(90 60 30 / .55)', 0.5) + '</g>';
    }],
    ['potion', 'Round bottle', 'slosh', function () {
      var d = 'M16.5 7H23.5V13.6A11 11 0 1 1 16.5 13.6Z';
      return shadow(11) + '<path d="' + d + '" fill="rgb(255 255 255 / .3)"/>' + liquid(d, 21, [15, 20, 25], 32) +
        glass(d, 'M11.6 21.6A9 9 0 0 1 15.6 15.8') + '<circle cx="12.6" cy="25.6" r=".9" fill="rgb(255 255 255 / .8)"/>' + ln('M16.5 10.6h7', GOLD, 1.4) +
        vol('M16 2.4H24V7.4H16Z', CORK, 'h') + vol('M15.4 5.4h9.2v1.8c-1 0-1.2 2.2-2.2 2.2s-1-1.2-2-1.2-1.2 2.4-2.2 2.4-1-2.4-2.8-1.4z', '#9c2f2f', 't');
    }],
    ['jar', 'Jar', 'slosh', function () {
      var d = 'M11 12H29V14C31.5 15 32 17 32 20V32A3 3 0 0 1 29 35H11A3 3 0 0 1 8 32V20C8 17 8.5 15 11 14Z';
      return shadow(13) + '<path d="' + d + '" fill="rgb(255 255 255 / .3)"/>' + liquid(d, 19.5, [12, 26, 19], 33) +
        glass(d, 'M10.6 18.6V31') + '<g transform="rotate(-3 20 26.5)">' + vol('M13.4 23h13.2v7.4H13.4Z', PAPER, 't') + ln('M15.4 25.4h9.2M15.4 27.8h6.4', 'rgb(90 60 30 / .55)', 0.6) + '</g>' +
        vol('M10 6.4H30V12.2H10Z', IRON, 'h') + ln('M12.5 6.8v5M15 6.8v5M17.5 6.8v5M20 6.8v5M22.5 6.8v5M25 6.8v5M27.5 6.8v5', DK, 0.5) + ln('M10.4 7.2H29.6', LT, 0.6);
    }],
    ['drop', 'Drop', 'pulse', function () {
      var d = 'M20 5C26 13 29.5 18 29.5 23.5A9.5 9.5 0 0 1 10.5 23.5C10.5 18 14 13 20 5Z';
      return shadow(9) + M('cpx-glow 1.8s ease-in-out infinite', '20px 23px', '<circle cx="20" cy="22" r="14" fill="' + C + '" opacity=".22"/>') +
        M('cpx-pulse 1.8s ease-in-out infinite', '20px 33px', vol(d, null, 'r') + ln('M14.4 22.6a5.6 5.6 0 0 0 3.2 6.4', 'rgb(255 255 255 / .7)', 1.4) +
        '<circle cx="16.4" cy="18.6" r="1.6" fill="rgb(255 255 255 / .85)"/>' + ln('M15 29.4q5 3.4 10 0', 'rgb(255 255 255 / .3)', 0.9));
    }],
    ['fang', 'Fang and drop', 'drip', function () {
      return shadow(8, 22) +
        vol('M26 10.5C26.5 14 27.5 16 29 17.5C29 15 29.5 13 30.5 10.5Z', BONE, 'h') +
        vol('M12.5 10.5C13 19 16 26 23 32.5C21 26 21.5 18 25 10.5Z', BONE, 'h') +
        ln('M16 12C17 20 19 26 22.6 31.6', DK, 0.6) + ln('M14.4 11.6C15 18 17 23 20 27.6', 'rgb(255 255 255 / .65)', 0.9) +
        '<path d="M20.6 28.4C21.4 30 22.2 31.4 23 32.5C22.5 30.8 22.2 29.4 22.1 27.9Z" fill="' + C + '" opacity=".8"/>' +
        vol('M7 7.6C13 3.6 27 3.6 33 7.6L31.6 12.6C25 10 15 10 8.6 12.6Z', '#a85454', 't') + ln('M9.6 9.6q10.4-3 20.8 0', 'rgb(255 255 255 / .3)', 0.6) +
        M('cpx-drip 1.6s ease-in infinite', '23px 32.5px', '<path d="M23 32.4c1.5 2.1 2.3 3.2 2.3 4.1a2.3 2.3 0 0 1-4.6 0c0-.9.8-2 2.3-4.1z" fill="' + C + '" ' + K + ' stroke-width=".7"/><circle cx="22.3" cy="35.9" r=".55" fill="rgb(255 255 255 / .8)"/>');
    }],
    ['amber', 'Amber lump', 'glint', function () {
      var d = 'M9 23C8 15 14 8 22 8.5C29 9 32 15 31 22C30 30 24 34 18 34C12 34 9.5 29 9 23Z';
      return shadow(12) + '<path d="' + d + '" fill="' + C + '" opacity=".95"/>' +
        '<g opacity=".6" transform="rotate(-20 20 22)"><g stroke="#3a1e08" stroke-width=".5" fill="none"><path d="M18 22l-2.4-2M18 22l-2.6 1.4M18.8 22.6l-1 2.6M20.4 22.4l1.4 2.4M21 21.6l2.4-1.4M20.6 21.4l1.6-2.6"/></g>' +
        '<ellipse cx="17.5" cy="20" rx="2.6" ry="1.1" fill="rgb(255 255 255 / .5)" transform="rotate(-30 17.5 20)"/><ellipse cx="21.5" cy="19.8" rx="2.6" ry="1.1" fill="rgb(255 255 255 / .5)" transform="rotate(30 21.5 19.8)"/>' +
        '<ellipse cx="19.6" cy="22" rx="2.8" ry="1.3" fill="#3a1e08"/><circle cx="22.6" cy="21.8" r="1" fill="#3a1e08"/></g>' +
        '<g fill="none" stroke="rgb(255 255 255 / .45)" stroke-width=".4"><circle cx="14" cy="26" r=".8"/><circle cx="25" cy="27" r=".6"/><circle cx="26" cy="17" r=".5"/></g>' +
        '<path d="' + d + '" fill="' + url('r') + '"/><path d="' + d + '" fill="none" ' + K + '/>' +
        hl('M12.6 17.4C13.8 13 17 10.8 21.4 10.8C17.4 12.8 15 14.8 12.6 17.4Z', 0.6) + spark(26, 13.5, 3.4, 0) + spark(14, 28, 2.2, 0.7);
    }],
    ['candle', 'Wax candle', 'flicker', function () {
      return shadow(13) + vol('M7.5 33h25c0 2.2-4.6 3.6-12.5 3.6S7.5 35.2 7.5 33z', IRON, 't') + volE('ellipse cx="20" cy="33" rx="12.5" ry="2.6"', IRON, 't') +
        vol('M13 16H27V33A2 2 0 0 1 25 35H15A2 2 0 0 1 13 33Z', null, 'h') +
        vol('M13 16C13 14.8 16 14 20 14S27 14.8 27 16C27 17.2 24 18 20 18S13 17.2 13 16Z', null, 't') + sh('M15.5 16c0-.8 2-1.3 4.5-1.3s4.5.5 4.5 1.3-2 1.2-4.5 1.2-4.5-.4-4.5-1.2z', 0.15) +
        vol('M13.4 16.6V22.4A1.1 1.1 0 0 0 15.6 22.4V17.6Z', null, 'h') + vol('M23.6 17.6V25.6A1.1 1.1 0 0 0 25.8 25.6V17Z', null, 'h') +
        ln('M20 15.6v-3.2', '#2a1a10', 1.1) +
        M('cpx-glow 1.2s ease-in-out infinite', '20px 9px', '<circle cx="20" cy="9" r="7.5" fill="#ffb347" opacity=".22"/>') +
        M('cpx-flicker 1.1s ease-in-out infinite', '20px 13px',
          '<path d="M20 2.4C23.6 6 24.6 8.5 24.6 10.6A4.6 4.6 0 0 1 15.4 10.6C15.4 8.5 16.4 6 20 2.4Z" fill="#ff9f38"/>' +
          '<path d="M20 6.4C21.7 8.3 22.3 9.6 22.3 10.8A2.3 2.3 0 0 1 17.7 10.8C17.7 9.6 18.3 8.3 20 6.4Z" fill="#fff1b8"/>') +
        ln('M18.2 13.2q1.8 1.2 3.6 0', '#6aa0ff', 0.8, 0.6);
    }],
    ['ingot', 'Ingot', 'glint', function () {
      var front = 'M6 30L10 21H30L34 30Z';
      return shadow(15) + vol(front, null, 't') + sh('M30 21L34 30H30.6L28 21Z', 0.15) +
        vol('M10 21L13 16H27L30 21Z') + hl('M10 21L13 16H27L30 21Z', 0.3) + ln('M10.4 21H29.6', LT, 0.7) +
        '<path d="M16.6 17.6h6.8l1.2 2H15.4z" fill="none" stroke="' + DK + '" stroke-width=".6"/>' + ln('M18.6 18.6h2.8', DK, 0.6) +
        sweep(front) + spark(28.5, 22.5, 2.8, 0.9);
    }],
    ['ore', 'Ore', 'glint', function () {
      return shadow(14) + vol('M6 29L9 18L18 10L29 13L34 24L28 33H12Z', '#6f6a62') +
        hl('M9 18L16 22L18 10Z', 0.15) + sh('M16 22L12 33H28L28 25Z', 0.18) + ln('M9 18l7 4 2-12M16 22l-4 11M16 22l12 3 6-1M28 25l-6-12', DK, 0.6) +
        rope('M11 21l4 2.5-1.5 5M20 13.5l2 6 6 2M19 28.4l5-1.2M30 19l2 4', 1.8) + ln('M11.4 20.6l3.2 2M20.4 13.8l1.6 4.6', LT, 0.5) +
        circ(23.6, 27.6, 1.6) + circ(13, 16.6, 1.1) + spark(22.4, 19.6, 2.6, 0) + spark(13.4, 27, 2, 0.8);
    }],
    ['coin', 'Coins', 'glint', function () {
      var s = shadow(14);
      [31, 28, 25].forEach(function (y) {
        s += vol('M5 ' + y + 'V' + (y + 2.6) + 'a9 3.4 0 0 0 18 0V' + y, null, 'h') + ln('M7 ' + (y + 2.6) + 'v1.4M10 ' + (y + 3.4) + 'v1.4M14 ' + (y + 3.8) + 'v1.4M18 ' + (y + 3.4) + 'v1.4M21 ' + (y + 2.6) + 'v1.4', DK, 0.4) +
          volE('ellipse cx="14" cy="' + y + '" rx="9" ry="3.4"', null, 't') + '<ellipse cx="14" cy="' + y + '" rx="6.6" ry="2.3" fill="none" stroke="' + DK + '" stroke-width=".5"/>';
      });
      return s + M('cpx-flip 1.6s ease-in-out infinite', '27px 17px',
        circ(27, 17, 9) + '<circle cx="27" cy="17" r="6.8" fill="none" stroke="' + DK + '" stroke-width=".7"/>' +
        '<path d="M27 12.4l1.4 2.9 3.1.4-2.3 2.2.6 3.1-2.8-1.5-2.8 1.5.6-3.1-2.3-2.2 3.1-.4z" fill="rgb(0 0 0 / .22)" transform="translate(.5 .5)"/>' +
        '<path d="M27 12.4l1.4 2.9 3.1.4-2.3 2.2.6 3.1-2.8-1.5-2.8 1.5.6-3.1-2.3-2.2 3.1-.4z" fill="rgb(255 255 255 / .35)"/>') + spark(11, 23.6, 2.4, 0.5);
    }],
    ['gem', 'Cut gem', 'glint', function () {
      return shadow(10) + M('cpx-glow 1.8s ease-in-out infinite', '20px 20px', '<circle cx="20" cy="18" r="15" fill="' + C + '" opacity=".16"/>') +
        '<path d="M8 14L14 7H26L32 14L20 35Z" fill="' + C + '"/>' + hl('M14 7H26L24 14H16Z', 0.45) + hl('M8 14L14 7L16 14Z', 0.25) + sh('M26 7L32 14H24Z', 0.12) +
        hl('M8 14H16L20 35Z', 0.18) + sh('M24 14H32L20 35Z', 0.32) + sh('M16 14H24L20 35Z', 0.06) +
        ln('M8 14H32M14 7L16 14L20 35M26 7L24 14L20 35', 'rgb(255 255 255 / .45)', 0.6) + ln('M11.4 15.6l5 9.4', 'rgb(255 255 255 / .4)', 0.8) +
        '<path d="M8 14L14 7H26L32 14L20 35Z" fill="none" ' + K + '/>' + spark(24, 9.6, 3, 0) + spark(13, 18, 2, 0.8);
    }],
    ['crystals', 'Crystal cluster', 'glint', function () {
      function prism(d, lit, dark) { return '<path d="' + d + '" fill="' + C + '"/>' + hl(lit, 0.32) + sh(dark, 0.22) + '<path d="' + d + '" fill="none" ' + K + '/>'; }
      return shadow(15) + vol('M5 34.5C6 30.5 10 29.5 14 30.5L20 29.5L27 30.5C31 29.5 35 31.5 35 34.5Z', '#6f6a62', 't') +
        prism('M8 32L6 20L9 15L13 19L14 31Z', 'M8 32L6 20L9 15L10.5 20.5L11 31.6Z', 'M11 31.6L10.5 20.5L13 19L14 31Z') +
        prism('M25 32L27 19L31 15L34 20L32 32Z', 'M25 32L27 19L31 15L29.6 20.4L28.6 32Z', 'M28.6 32L29.6 20.4L34 20L32 32Z') +
        prism('M15 32L16 12L21 4L25 12L25 32Z', 'M15 32L16 12L21 4L20 12.6V32Z', 'M20 32V12.6L21 4L25 12V32Z') +
        ln('M20 12.6L16 12M20 12.6L25 12', 'rgb(255 255 255 / .5)', 0.5) + ln('M17.6 14v14', 'rgb(255 255 255 / .45)', 0.8) +
        prism('M17 35l-1-6 2-2.6 2 2.6-.5 6z', 'M17 35l-1-6 2-2.6.2 8.6z', 'M18.2 35l-.2-8.6 2 2.6-.5 6z') +
        spark(19.4, 9, 2.6, 0) + spark(30.6, 18, 2, 0.6) + spark(8.6, 20, 1.8, 1.1);
    }],
    ['stone', 'Stone', 'none', function () {
      var d = 'M6 27C5 21 9 15 16 13C22 11 29 12 33 17C36 21 35 27 31 30C26 33 18 34 12 32C9 31 7 29 6 27Z';
      return shadow(15) + '<path d="' + d + '" fill="' + C + '"/>' + speckle(21, 30, 20, 23, 11, 7, 'rgb(0 0 0 / .22)', 0.45) + speckle(22, 18, 19, 21, 10, 6, 'rgb(255 255 255 / .3)', 0.4) +
        '<path d="' + d + '" fill="' + url('r') + '"/><path d="' + d + '" fill="none" ' + K + '/>' + ln('M18 13.4l1 4-2 3 2 3.4', DK, 0.7) + ln('M19 17.4l2.4 1.4', DK, 0.5) +
        '<path d="M26 14.6c2-1 4 0 4.4 1.6-1.6.6-3.6.2-4.4-1.6z" fill="#9ab04a" opacity=".8"/>' +
        vol('M28 33.4c0-2 2-3 4-3s3.5 1 3.5 2.5-1.5 2.5-3.5 2.5-4-.4-4-2z', null, 'r');
    }],
    ['rune', 'Rune stone', 'pulse', function () {
      var r = 'M20 11V29M20 14l5 4M20 20l-5 4M15 11l2 3';
      return shadow(12) + vol('M10 35L8 14L14 5H26L32 13L30 35Z', '#7b7d84') + sh('M26 5L32 13L30 35H26.4L27.6 13.4Z', 0.12) +
        speckle(31, 14, 20, 22, 9, 11, 'rgb(0 0 0 / .18)', 0.45) + ln('M26.6 8l-2 3 1 3M11 22l2.4 1.6', DK, 0.55) +
        ln(r, 'rgb(0 0 0 / .55)', 2.8) +
        M('cpx-glow 1.6s ease-in-out infinite', '20px 20px', '<path d="' + r + '" fill="none" stroke="' + C + '" stroke-width="4" stroke-linecap="round" filter="' + url('b') + '" opacity=".7"/>') +
        ln(r, C, 1.5) + ln(r, 'rgb(255 255 255 / .45)', 0.5) +
        '<path d="M10 35c2-3 5-3 7-1 2-2 5-1.4 6 .6 2-1.4 5-1 7 .4z" fill="#6a8a3a"/>';
    }],
    ['powder', 'Powder in a dish', 'none', function () {
      return shadow(15) + vol('M4 24H36C35 31 28 35 20 35S5 31 4 24Z', '#b9ad96', 't') +
        vol('M4 24C4 22 11 21 20 21S36 22 36 24C36 26 29 27 20 27S4 26 4 24Z', '#d6cab2', 't') + sh('M6 24c0-1.4 6-2.2 14-2.2s14 .8 14 2.2-6 2-14 2-14-.6-14-2z', 0.25) +
        vol('M10 24.2C12 17 16 12 20 12S28 17 30 24.2C26 26 14 26 10 24.2Z', null, 'r') +
        speckle(41, 20, 20, 19, 7, 5, 'rgb(255 255 255 / .55)', 0.35) + speckle(42, 14, 20, 21, 8, 3.6, 'rgb(0 0 0 / .2)', 0.35) +
        '<g fill="' + C + '"><circle cx="8" cy="36" r=".7"/><circle cx="33.6" cy="36.4" r=".8"/><circle cx="36" cy="35.6" r=".5"/></g>' +
        rope('M27 10.6l8.4-6.8', 1.4, '#9a7048') + '<g transform="rotate(-38 26 11.6)">' + volE('ellipse cx="26" cy="11.6" rx="3" ry="1.8"', '#9a7048', 't') + '<ellipse cx="26" cy="11.3" rx="2" ry=".9" fill="' + C + '"/></g>';
    }],
    ['salt', 'Salt crystals', 'glint', function () {
      function cube(x, y, s) {
        var a = +(s * 0.87).toFixed(2), b = +(s * 0.5).toFixed(2);
        var top = 'M' + x + ' ' + (y - s) + 'l' + a + ' ' + b + '-' + a + ' ' + b + '-' + a + '-' + b + 'z';
        var left = 'M' + (x - a) + ' ' + (y - b) + 'l' + a + ' ' + b + 'v' + s + 'l-' + a + '-' + b + 'z';
        var right = 'M' + x + ' ' + y + 'l' + a + '-' + b + 'v' + s + 'l-' + a + ' ' + b + 'z';
        return '<path d="' + top + left + right + '" fill="' + C + '"/>' + hl(top, 0.5) + sh(right, 0.26) + '<path d="' + top + left + right + '" fill="none" ' + K + ' stroke-width=".8"/>' +
          ln('M' + (x - a + 0.6) + ' ' + (y - b + 0.3) + 'l' + (a - 0.6) + ' ' + (b - 0.3), LT, 0.5);
      }
      return shadow(15) + cube(19, 18, 6) + cube(13, 27, 6) + cube(25.5, 26, 7) + cube(31.5, 33, 2.6) + cube(7.6, 33.6, 2.2) +
        '<g fill="' + C + '"><circle cx="35.5" cy="36" r=".5"/><circle cx="4.6" cy="36.2" r=".45"/><circle cx="20" cy="36.4" r=".5"/></g>' +
        spark(19, 12.6, 2.6, 0) + spark(28.6, 23.4, 2, 0.7);
    }],
    ['log', 'Log', 'none', function () {
      return shadow(16) + rope('M15 14.4l-2.4-5.4', 2.2) + leaflet(12.8, 9.4, -125, 0.75, LEAF) +
        vol('M8 14H30A5 8 0 0 1 30 30H8A5 8 0 0 1 8 14Z', null, 't') +
        ln('M10 18c4-1 8 1 12 0s5-1 6 0M8.6 22c5 1 9-1 14 0s4 .6 5.4 0M10 26c4-1 9 1 13 0M13 15.6l2 2M19 27.6l3 1.4', DK, 0.7) +
        ln('M9.6 16c3-.6 6 0 9-.4', LT, 0.5) + '<path d="M11 14.4c2-2 5-2 7 0z" fill="#6a8a3a"/>' +
        volE('ellipse cx="30" cy="22" rx="5" ry="8"', '#dcbb8a', 't') +
        '<g fill="none" stroke="#a67c4a" stroke-width=".6"><ellipse cx="30" cy="22" rx="3.6" ry="5.8"/><ellipse cx="30" cy="22" rx="2.3" ry="3.7"/><ellipse cx="30" cy="22" rx="1" ry="1.6"/></g>' +
        ln('M30 22l3.4-5', '#7a5530', 0.6) + '<ellipse cx="30" cy="22" rx="5" ry="8" fill="none" stroke="' + C + '" stroke-width="1.2"/><ellipse cx="30" cy="22" rx="5.6" ry="8.6" fill="none" ' + K + '/>';
    }],
    ['plank', 'Planks', 'none', function () {
      function board(dy) {
        var top = 'M4 ' + (27 + dy) + 'L27 ' + (15 + dy) + 'L35 ' + (19 + dy) + 'L12 ' + (31 + dy) + 'Z';
        return '<path d="' + top + '" fill="' + C + '"/>' + hl(top, 0.22) +
          vol('M12 ' + (31 + dy) + 'L35 ' + (19 + dy) + 'V' + (22 + dy) + 'L12 ' + (34 + dy) + 'Z', null, 't') + sh('M12 ' + (31 + dy) + 'L35 ' + (19 + dy) + 'V' + (22 + dy) + 'L12 ' + (34 + dy) + 'Z', 0.2) +
          vol('M4 ' + (27 + dy) + 'L12 ' + (31 + dy) + 'V' + (34 + dy) + 'L4 ' + (30 + dy) + 'Z', null, 't') +
          ln('M8 ' + (27.4 + dy) + 'c5-2 9-5 14-7s4-1.4 6-2.6M11 ' + (28.4 + dy) + 'c4-2 8-4 12-6.6M15 ' + (29 + dy) + 'c4-1.6 9-4.6 14-7.4', DK, 0.45) +
          '<ellipse cx="19" cy="' + (24.6 + dy) + '" rx="1.6" ry=".8" transform="rotate(-27 19 ' + (24.6 + dy) + ')" fill="none" stroke="' + DK + '" stroke-width=".6"/>' +
          '<path d="' + top + '" fill="none" ' + K + '/>' + '<g fill="#5f5a52"><circle cx="8.6" cy="' + (28.4 + dy) + '" r=".6"/><circle cx="30.4" cy="' + (17.6 + dy) + '" r=".6"/></g>';
      }
      return shadow(16) + board(0) + board(-7);
    }],
    ['cloth', 'Folded cloth', 'sway', function () {
      function fold(y, stripe) {
        var top = 'M6 ' + y + 'L20 ' + (y - 6) + 'L34 ' + y + 'L20 ' + (y + 6) + 'Z';
        return vol('M6 ' + y + 'L20 ' + (y + 6) + 'V' + (y + 10) + 'L6 ' + (y + 4) + 'Q4.4 ' + (y + 2) + ' 6 ' + y + 'Z', null, 't') +
          vol('M20 ' + (y + 6) + 'L34 ' + y + 'V' + (y + 4) + 'L20 ' + (y + 10) + 'Z', null, 't') + sh('M20 ' + (y + 6) + 'L34 ' + y + 'V' + (y + 4) + 'L20 ' + (y + 10) + 'Z', 0.22) +
          '<path d="' + top + '" fill="' + C + '"/>' + hl(top, 0.18) + '<path d="' + top + '" fill="none" ' + K + '/>' +
          (stripe ? ln('M10 ' + (y - 0.4) + 'L23.6 ' + (y - 6.6 + 0.4) + 'M12.6 ' + (y + 0.8) + 'L26.2 ' + (y - 5.4 + 0.4), 'rgb(255 255 255 / .55)', 0.8) +
            '<path d="M8 ' + y + 'L20 ' + (y + 5.2) + 'L32 ' + y + '" fill="none" stroke="rgb(255 255 255 / .5)" stroke-width=".5" stroke-dasharray="1 1"/>' : '');
      }
      return shadow(16) + fold(26, false) + fold(21, false) +
        M('cpx-sway 2s ease-in-out infinite', '34px 16px', vol('M33.6 16v4.6c0 4-1.2 7 .8 10.4l-3-1c-1.2-3-.2-6-.2-9.4V17.6Z', null, 'h')) + fold(16, true);
    }],
    ['spool', 'Spool of thread', 'none', function () {
      var wind = '';
      for (var y = 11; y < 31; y += 1.6) wind += 'M12.4 ' + y + 'L27.6 ' + (y + 1.2);
      return shadow(12) +
        vol('M9 31a11 3.5 0 0 0 22 0v2.4a11 3.5 0 0 1-22 0z', '#c79a62', 'h') + volE('ellipse cx="20" cy="31" rx="11" ry="3.5"', '#d8b07a', 't') +
        vol('M12.4 10H27.6V31H12.4Z', null, 'h') + ln(wind, 'rgb(0 0 0 / .18)', 0.5) + ln(wind.replace(/L27.6/g, 'L20'), 'rgb(255 255 255 / .2)', 0.4) +
        vol('M9 9v2.4a11 3.5 0 0 0 22 0V9', '#c79a62', 'h') + volE('ellipse cx="20" cy="9" rx="11" ry="3.5"', '#d8b07a', 't') +
        '<ellipse cx="20" cy="9" rx="2.6" ry="1" fill="#5a4630"/>' + ln('M27.6 24q5 1 6 5t-2 6.4', C, 1.1) +
        ln('M30 5.4L36.6 24.6', '#b8bcc4', 1.2) + ln('M30 5.4L36.6 24.6', 'rgb(0 0 0 / .4)', 0.3) + '<ellipse cx="30.8" cy="7.6" rx=".5" ry="1.1" transform="rotate(-19 30.8 7.6)" fill="#fff"/>' +
        ln('M31 7.4q-3 2-3.4 3', C, 0.8);
    }],
    ['hide', 'Hide', 'none', function () {
      var d = 'M8 8C12 10 16 9 20 6C24 9 28 10 32 8C30 13 32 18 34 21C31 25 31 30 33 33C28 32 24 33 20 35C16 33 12 32 7 33C9 30 9 25 6 21C8 18 10 13 8 8Z';
      var g = rnd(51), fur = '';
      for (var i = 0; i < 34; i++) {
        var x = 10 + g() * 20, y = 10 + g() * 21;
        fur += 'M' + x.toFixed(1) + ' ' + y.toFixed(1) + 'q.8 1.2 .4 2.4';
      }
      return shadow(15) + vol(d) + sh('M20 6.4c-2.2 9-2.2 19.6 0 28.4 2.2-9 2.2-19.6 0-28.4z', 0.18) +
        ln(fur, 'rgb(0 0 0 / .22)', 0.5) + '<g transform="translate(1 -.8)">' + ln(fur, 'rgb(255 255 255 / .22)', 0.4) + '</g>' +
        '<path d="' + d + '" transform="translate(20 20.6) scale(.84) translate(-20 -20.6)" fill="none" stroke="#5a4630" stroke-width="1" stroke-dasharray="1.4 1.8"/>';
    }],
    ['scale', 'Scales', 'glint', function () {
      function sc(x, y) {
        var d = 'M' + (x - 5.6) + ' ' + y + 'C' + (x - 5.6) + ' ' + (y + 5) + ' ' + (x - 2) + ' ' + (y + 8) + ' ' + x + ' ' + (y + 9.6) + 'C' + (x + 2) + ' ' + (y + 8) + ' ' + (x + 5.6) + ' ' + (y + 5) + ' ' + (x + 5.6) + ' ' + y + 'C' + (x + 3) + ' ' + (y - 1.6) + ' ' + (x - 3) + ' ' + (y - 1.6) + ' ' + (x - 5.6) + ' ' + y + 'Z';
        return vol(d) + ln('M' + x + ' ' + (y + 0.4) + 'V' + (y + 8), DK, 0.5) + ln('M' + (x - 4.4) + ' ' + (y + 1) + 'C' + (x - 4) + ' ' + (y + 4) + ' ' + (x - 2.4) + ' ' + (y + 6) + ' ' + (x - 1) + ' ' + (y + 7.4), LT, 0.6);
      }
      return shadow(13) + sc(14, 7) + sc(26, 7) + sc(8, 13) + sc(20, 13) + sc(32, 13) + sc(14, 19) + sc(26, 19) + sc(20, 25) +
        spark(24, 15.6, 2.6, 0) + spark(12, 22, 2, 0.8);
    }],
    ['shell', 'Shell', 'glint', function () {
      var ribs = '';
      [[9, 10.6], [12.6, 7.4], [16.4, 5.6], [20, 5], [23.6, 5.6], [27.4, 7.4], [31, 10.6]].forEach(function (p) { ribs += 'M20 33L' + p[0] + ' ' + p[1]; });
      return shadow(13) + vol('M20 34L6 19C5 11 11 5 20 5S35 11 34 19Z') + ln(ribs, DK, 0.7) +
        '<g transform="translate(.9 0)">' + ln(ribs, 'rgb(255 255 255 / .4)', 0.6) + '</g>' +
        ln('M8.6 17.4Q20 9 31.4 17.4M11.6 23Q20 16 28.4 23M15 28.2Q20 23.4 25 28.2', 'rgb(0 0 0 / .18)', 0.6) +
        vol('M14.6 31.4L20 34.4L25.4 31.4L26.6 35.4H13.4Z', null, 't') +
        circ(29.6, 33.6, 2.6, '#f4efe4') + spark(28.6, 32.4, 1.6, 0.2);
    }],
    ['feather', 'Feather', 'sway', function () {
      var barbs = '', g = rnd(61);
      for (var t = 0.14; t < 0.94; t += 0.06) {
        var x = 10 + 20 * t, y = 31 - 27 * t;
        barbs += 'M' + x.toFixed(1) + ' ' + y.toFixed(1) + 'q' + (-3 - g()).toFixed(1) + ' -1 ' + (-4.6 + t * 2).toFixed(1) + ' -' + (3.6 - t * 2).toFixed(1);
        barbs += 'M' + x.toFixed(1) + ' ' + y.toFixed(1) + 'q' + (2 + g()).toFixed(1) + ' 0 ' + (3.6 - t).toFixed(1) + ' -' + (3 - t).toFixed(1);
      }
      return shadow(9, 12) + M('cpx-sway 1.6s ease-in-out infinite', '9px 36px',
        vol('M31 3C19 5 10 15 10 29L13 26.5C21 25.5 28 17 31 3Z', null, 'h') + ln(barbs, 'rgb(0 0 0 / .22)', 0.45) +
        '<path d="M15.6 19.6l-4.4 1.6" stroke="' + C + '" stroke-width="1.6"/>' +
        rope('M8 37C14 25 21 14 30 4', 0.8, '#f2ead8') + ln('M11 29c-2 0-3-1-4-2M12 28c-1 2-3 3-5 3M10.6 30.6c-1 1-3 1.2-4 .2', 'rgb(255 255 255 / .8)', 0.7));
    }],
    ['bone', 'Bone', 'none', function () {
      var ends = [[10, 9.6], [13.6, 7], [27, 30.6], [30.4, 27.4]], o = '', f = '';
      ends.forEach(function (e) { o += '<circle cx="' + e[0] + '" cy="' + e[1] + '" r="4.6" fill="' + INK + '"/>'; f += '<circle cx="' + e[0] + '" cy="' + e[1] + '" r="4" fill="' + C + '"/>'; });
      return shadow(13) + o + ln('M12 11.4L28.4 28', INK, 8.4) + f + ln('M12 11.4L28.4 28', C, 7.2) +
        ends.map(function (e) { return '<circle cx="' + e[0] + '" cy="' + e[1] + '" r="4" fill="' + url('r') + '"/>'; }).join('') +
        ln('M10.9 14.2L26.9 28.6', 'rgb(0 0 0 / .2)', 2) + ln('M13.4 11.6L29.4 25.8', 'rgb(255 255 255 / .45)', 1.2) +
        ln('M19 19.6l1.6-.4.6 1.6', DK, 0.6) + speckle(71, 6, 28.6, 29, 3, 3, 'rgb(0 0 0 / .2)', 0.3);
    }],
    ['skull', 'Skull', 'none', function () {
      return shadow(11) + vol('M12.5 27.4H27.5V31C27.5 33 25 34.6 20 34.6S12.5 33 12.5 31Z', null, 't') +
        vol('M8 19C8 11 13.5 5 20 5S32 11 32 19C32 22.5 30.5 24.5 28.5 25.5V28H11.5V25.5C9.5 24.5 8 22.5 8 19Z', null, 'r') +
        sh('M9.4 22.6c1 1.6 2.4 2.4 4 2.6l1-2.4c-2 0-3.8-.6-5-.2zM30.6 22.6c-1 1.6-2.4 2.4-4 2.6l-1-2.4c2 0 3.8-.6 5-.2z', 0.2) +
        '<g fill="#2e2620"><path d="M11 18.6c0-2.6 2-3.8 4.2-3.8s4.2 1.6 3.6 4.2c-.5 2.2-2.2 3.4-4.2 3.4S11 21 11 18.6Z"/><path d="M29 18.6c0-2.6-2-3.8-4.2-3.8s-4.2 1.6-3.6 4.2c.5 2.2 2.2 3.4 4.2 3.4S29 21 29 18.6Z"/><path d="M20 22.4l-2 3.4h4z"/></g>' +
        ln('M12.6 17.2q2-1.6 4.6-.8M23 16.4q2.6-.8 4.6.8', 'rgb(255 255 255 / .25)', 0.6) +
        ln('M15 26.6v3.2M17 26.8v3.2M19 27v3.2M21 27v3.2M23 26.8v3.2M25 26.6v3.2M13.4 29.8h13.2', DK, 0.6) +
        ln('M20 5.6c-1 3 1 5 0 8M27 9.6l-2 2.4.6 2', DK, 0.55);
    }],
    ['claw', 'Claw', 'none', function () {
      return shadow(13, 22) + vol('M24 35C24 29 27 23 33 18C32 23 32 28 35 32C31 32 27 33 24 35Z', null, 'h') + sh('M24 35C27 33 31 32 35 32L35.4 34C31 34 27 35 25 36.4Z', 0.35) +
        vol('M9 34C9 24 13 13 25 5C23 13 23 21 27 28C21 27 13 29 9 34Z', null, 'h') +
        ln('M12 30C13 22 17 15 23 8.6M15.4 28.4C16.4 22 19 16.6 23.6 11.4', DK, 0.55) + ln('M10.8 29C11.8 21 16 14 22 8', 'rgb(255 255 255 / .55)', 0.9) +
        vol('M9 34C13 29 21 27 27 28L28 32C22 31 15 33 11 36.4Z', '#5e4a3c', 't') + speckle(81, 8, 18, 31.6, 7, 1.6, 'rgb(255 255 255 / .25)', 0.35);
    }],
    ['horn', 'Horn', 'none', function () {
      var rings = 'M7.4 30.6q5-1 10.4.6M9 25.6q4.6-1.2 9.6 1.4M11.4 20.6q4-1.4 8.6 2M14.4 16q3.4-1.6 7.6 1.8M18 12q3-1.6 6.4 1.2M22.4 8.6q2.4-1.2 5 .6';
      return shadow(13) + vol('M6 35C10 34 15 34 18 35C19 24 24 14 34 5C22 7 9 17 6 35Z', null, 'h') + hl('M34 5C30 6.6 27 9 25 11.4C27.6 10.4 30.6 8 34 5Z', 0.45) +
        ln(rings, DK, 0.7) + '<g transform="translate(.4 -.7)">' + ln(rings, 'rgb(255 255 255 / .35)', 0.5) + '</g>' + ln('M6.4 35c3-1.6 9-1.6 11.6 0', 'rgb(0 0 0 / .35)', 0.9);
    }],
    ['heart', 'Heart', 'beat', function () {
      var body = 'M12 15C9 19 9 26 14 31C17 34 20 35.6 22 35C27 33 31 27 31 21C31 17 29 14 26 13L18 12Z';
      return shadow(10, 21) + M('cpx-beat 1.2s ease-in-out infinite', '20px 24px',
        rope('M13 12V4.6', 2.4) + ln('M13 12V4.6', 'rgb(0 0 0 / .22)', 2.4) +
        rope('M17 13C16 8 17 4 21 3.6C25 3.2 27 5.2 27.6 8.4', 3.6) + rope('M20 4V1.4M23.2 3.6l.6-2.4', 1.6) + ln('M17.8 9.6C17.6 6.6 18.6 4.8 21 4.6', 'rgb(255 255 255 / .45)', 0.8) +
        vol('M10 16C8 12 11 9 14 10C16 11 16 14 15 16Z', null, 'r') + vol('M26 13.4c3-1.2 6 .8 5 4-1 2-3 2.2-5 1z', null, 'r') +
        vol(body, null, 'r') + rope('M21.6 13C21.6 10 23.8 8.4 26 8.6', 2.6) + ln('M21.6 13C21.6 10 23.8 8.4 26 8.6', 'rgb(0 0 0 / .2)', 2.6) +
        ln('M18 13c0 5 2 10 5 15M23 17c2 2 3 5 3 8M18 20c-2 1-4 3-4 6M21 23.4l2.4-.4', 'rgb(0 0 0 / .3)', 0.8) +
        ln('M18.6 13c0 5 2 10 5 15', 'rgb(255 255 255 / .25)', 0.4) + hl('M13 18c1-2 3-3 5-2.6-2 1.6-3.4 4-4 7-1.2-1.2-1.4-2.8-1-4.4z', 0.35));
    }],
    ['eye', 'Eye', 'look', function () {
      var rays = '';
      for (var a = 0; a < 360; a += 30) rays += '<path d="M20 17.2V14.4" transform="rotate(' + a + ' 20 20)"/>';
      return shadow(11) + vol('M33 20l4.4-2.4v4.8z', '#c46a6a', 't') + circ(20, 20, 13, '#f4efe4') +
        ln('M7.6 16c2.4.4 3.6 2 4.6 2.2M8.4 25c2-.6 3.4.4 4.6 0M31.8 14.6c-2 .6-3 2-4.2 2.4M32 25.4c-1.6-.8-3-.4-4.2-1M20 33c0-1.6.8-2.4.6-3.6', '#c0392b', 0.5, 0.75) +
        M('cpx-look 3s ease-in-out infinite', '20px 20px',
          circ(20, 20, 6.4) + '<g stroke="rgb(0 0 0 / .3)" stroke-width=".6">' + rays + '</g><g stroke="rgb(255 255 255 / .3)" stroke-width=".4" transform="rotate(15 20 20)">' + rays + '</g>' +
          '<circle cx="20" cy="20" r="6.4" fill="none" stroke="rgb(0 0 0 / .45)" stroke-width="1"/>' + M('cpx-pulse 1.5s ease-in-out infinite', '20px 20px', '<circle cx="20" cy="20" r="2.7" fill="#120e0c"/>')) +
        '<circle cx="17.2" cy="16.6" r="1.5" fill="#fff"/><circle cx="23" cy="23" r=".6" fill="rgb(255 255 255 / .7)"/>' +
        M('cpx-blink 3s ease-in-out infinite', '20px 7px', '<path d="M7 20A13 13 0 0 1 33 20C28 22 12 22 7 20Z" fill="#c9a98a" ' + K + '/>', 'cpx-hide');
    }],
    ['tentacle', 'Tentacles', 'writhe', function () {
      function arm(x, y, Ls, Ws, As, side, delay) {
        function chain(pass) {
          var open = '', close = '';
          for (var i = 0; i < Ls.length; i++) {
            var L = Ls[i], a = Ws[i] / 2, b = Ws[i + 1] / 2;
            var seg = 'M' + (-a) + ' 0L' + (-b) + ' ' + (-L) + 'A' + b + ' ' + b + ' 0 0 1 ' + b + ' ' + (-L) + 'L' + a + ' 0A' + a + ' ' + a + ' 0 0 1 ' + (-a) + ' 0Z';
            var body;
            if (pass === 0) body = '<path d="' + seg + '" fill="' + INK + '" stroke="' + INK + '" stroke-width="1.3"/>';
            else if (pass === 1) body = '<path d="' + seg + '" fill="' + C + '"/>';
            else {
              body = '<path d="M' + (-side * a * 0.1) + ' 0L' + (-side * b * 0.1) + ' ' + (-L) + 'H' + (-side * b) + 'L' + (-side * a) + ' 0Z" fill="rgb(0 0 0 / .2)"/>' +
                '<path d="M' + (side * a * 0.55) + ' 0L' + (side * b * 0.55) + ' ' + (-L) + 'H' + (side * b * 0.3) + 'L' + (side * a * 0.3) + ' 0Z" fill="rgb(255 255 255 / .28)"/>';
              if (a > 0.9) {
                var r = (a * 0.36).toFixed(2), sx = (side * a * 0.62).toFixed(2), sy = (-L * 0.5).toFixed(2);
                body += '<circle cx="' + sx + '" cy="' + sy + '" r="' + r + '" fill="rgb(255 255 255 / .55)" stroke="' + DK + '" stroke-width=".4"/><circle cx="' + sx + '" cy="' + sy + '" r="' + (r * 0.4).toFixed(2) + '" fill="' + DK + '"/>';
              }
            }
            open += '<g transform="translate(0 ' + (i ? -Ls[i - 1] : 0) + ') rotate(' + As[i] + ')"><g class="cpx-m" style="--an:cpx-writhe 1.7s ease-in-out ' + (delay - i * 0.2).toFixed(2) + 's infinite alternate;transform-origin:0 0">' + body;
            close += '</g></g>';
          }
          return open + close;
        }
        return '<g transform="translate(' + x + ' ' + y + ')">' + chain(0) + chain(1) + chain(2) + '</g>';
      }
      return shadow(15) + '<ellipse cx="20" cy="36.4" rx="13" ry="2.4" fill="rgb(40 70 90 / .22)"/>' +
        arm(25, 37, [6.6, 6, 5.2, 4.4, 3.4, 2.6], [6.4, 5.6, 4.6, 3.6, 2.6, 1.7, 0.8], [4, 6, 10, 16, 28, 44], 1, -0.5) +
        arm(16, 37, [8, 7, 6, 5, 4, 3.2], [8.4, 7.2, 5.9, 4.6, 3.4, 2.2, 1], [-2, -4, -6, -12, -24, -40], -1, 0) +
        ln('M7.6 36.4q5.4-1.4 10.6 0M24.6 36.6q4.6-1 8.6 0', 'rgb(255 255 255 / .45)', 0.6);
    }],
    ['shard', 'Shard', 'pulse', function () {
      var d = 'M18 3L27 14L23 36L13 22Z';
      return M('cpx-glow 1.8s ease-in-out infinite', '20px 20px', '<ellipse cx="20" cy="20" rx="13" ry="16" fill="' + C + '" opacity=".22"/>') + shadow(7) +
        M('cpx-float 2s ease-in-out infinite', '20px 20px',
          '<path d="' + d + '" fill="' + C + '"/>' + hl('M18 3L13 22L20 20Z', 0.38) + hl('M18 3L27 14L20 20Z', 0.12) + sh('M13 22L20 20L23 36Z', 0.1) + sh('M20 20L27 14L23 36Z', 0.32) +
          ln('M18 3L20 20L23 36M13 22L20 20L27 14', 'rgb(255 255 255 / .45)', 0.5) + '<path d="' + d + '" fill="none" ' + K + '/>' +
          vol('M29 8l2.4 1-1 2.6-2-1.2z') + vol('M8.6 27l2-1 .8 2.2-2 .6z')) + spark(16.6, 11, 2.6, 0) + spark(25.6, 26, 2, 0.8);
    }],
    ['orb', 'Orb', 'pulse', function () {
      return shadow(10) + M('cpx-glow 2s ease-in-out infinite', '20px 17px', '<circle cx="20" cy="17" r="14.6" fill="' + C + '" opacity=".22"/>') +
        '<circle cx="20" cy="17" r="11" fill="' + C + '"/><clipPath id="' + U + 'o"><circle cx="20" cy="17" r="10.4"/></clipPath><g clip-path="' + url('o') + '">' +
        M('cpx-spin 5s linear infinite', '20px 17px', ln('M12 19c2-6 10-7 13-2.4S24 25 19 24M15 12c3-2 8-1.6 10 1M14 23.6c3 2.4 9 2.4 12-1', 'rgb(255 255 255 / .4)', 1.1)) + '</g>' +
        '<circle cx="20" cy="17" r="11" fill="' + url('r') + '"/><circle cx="20" cy="17" r="11" fill="none" ' + K + '/>' +
        ln('M13.4 12.6A8 8 0 0 1 19 9', 'rgb(255 255 255 / .85)', 1.7) + '<circle cx="13" cy="15.6" r=".8" fill="#fff"/>' +
        vol('M11 36.4h18l-2-4.2H13z', CORK, 't') + rope('M13.4 32.4c-1.4-2.6-.6-5.4 1.6-6.6M26.6 32.4c1.4-2.6.6-5.4-1.6-6.6M20 32.4v-4', 1.5, CORK);
    }],
    ['ring', 'Ring', 'glint', function () {
      return shadow(11) + '<ellipse cx="20" cy="25" rx="11" ry="8.4" fill="none" stroke="' + INK + '" stroke-width="5.4"/><ellipse cx="20" cy="25" rx="11" ry="8.4" fill="none" stroke="' + GOLD + '" stroke-width="4.2"/>' +
        ln('M9.4 23.6A11 8.4 0 0 1 30.6 23.6', 'rgb(0 0 0 / .3)', 1.6) + ln('M10.4 27.6A11 8.4 0 0 0 29.6 27.6', 'rgb(255 255 255 / .55)', 1) + ln('M11.8 30.6A11 8.4 0 0 0 16 32.6', '#fff', 0.8) +
        vol('M16 18.2h8l-1.4 2.8h-5.2z', GOLD, 't') +
        '<path d="M15 13L17.6 10H22.4L25 13L20 19.4Z" fill="' + C + '"/>' + hl('M17.6 10H22.4L21.6 13H18.4Z', 0.5) + sh('M22.4 10L25 13H21.6Z', 0.15) + sh('M21.6 13H25L20 19.4Z', 0.32) + hl('M15 13H18.4L20 19.4Z', 0.2) +
        '<path d="M15 13L17.6 10H22.4L25 13L20 19.4Z" fill="none" ' + K + ' stroke-width=".8"/>' + spark(23, 11, 2.6, 0) + spark(28.6, 30, 2, 0.8);
    }],
    ['ember', 'Ember', 'flicker', function () {
      function coal(d, crack) { return vol(d, '#3a2a22', 't') + ln(crack, C, 1) + ln(crack, '#fff3c4', 0.35); }
      return M('cpx-glow 1.1s ease-in-out infinite', '20px 26px', '<ellipse cx="20" cy="26" rx="15" ry="11" fill="' + C + '" opacity=".25"/>') + shadow(13) +
        coal('M5 33c0-3.6 3-5.6 6.6-5.6s5.8 2 5.8 4.8-3 4.6-6.6 4.6S5 35.4 5 33z', 'M8 31.6l2.4 1 1.6-1.8') +
        coal('M22.6 32.6c0-3.4 3-5.4 6.4-5.4S35 29 35 32s-3 4.4-6.4 4.4-6-.8-6-3.8z', 'M26 31l2.4 1.4 2.4-1.6M29 34.2l1.6-1') +
        M('cpx-flicker 1s ease-in-out infinite', '20px 30px',
          '<path d="M20 7C24.6 12.4 27 16.4 26 21.4C25 25.6 22.6 28 20 28S15 25.6 14 21.4C13 16.4 15.4 12.4 20 7Z" fill="' + C + '" ' + K + '/>' +
          '<path d="M20 13C22.8 16.4 24.2 19 23.6 22.2A3.6 3.6 0 0 1 16.4 22.2C15.8 19 17.2 16.4 20 13Z" fill="#ffd27a"/>' +
          '<path d="M20 18.4C21.4 20.2 22 21.4 21.8 23A1.8 1.8 0 0 1 18.2 23C18 21.4 18.6 20.2 20 18.4Z" fill="#fff3c4"/>') +
        coal('M12.6 34c0-3.6 3.4-5.8 7.4-5.8s7.4 2.2 7.4 5.4-3.4 3.2-7.4 3.2-7.4.4-7.4-2.8z', 'M15.4 32.6l2.6 1.2 2.6-2 2.4 1.6M17 35l2-.8') +
        '<g fill="' + C + '">' + M('cpx-rise 1.3s ease-out infinite', '14px 12px', '<circle cx="14" cy="12" r=".8"/>', 'cpx-hide') +
        M('cpx-rise 1.7s ease-out .5s infinite', '26px 10px', '<circle cx="26" cy="10" r=".7"/>', 'cpx-hide') + '</g>' +
        '<circle cx="11" cy="9" r=".6" fill="' + C + '"/>';
    }],
    ['scroll', 'Scroll', 'none', function () {
      return shadow(15) + vol('M10 9H30V31H10Z', PAPER, 'v') + sh('M10 9H30V11.6H10Z', 0.15) +
        '<rect x="12.4" y="12.6" width="3.4" height="3.4" fill="' + C + '" opacity=".85"/>' + ln('M17 13.6h10.4M17 15.6h8M12.6 18.6h14.6M12.6 21h11.4M12.6 23.4h13M12.6 25.8h7', 'rgb(90 60 30 / .6)', 0.6) +
        vol('M7 5.4H33V10.4H7Z', null, 't') + vol('M7 29.6H33V34.6H7Z', null, 't') +
        circ(6, 7.9, 1.8, GOLD) + circ(34, 7.9, 1.8, GOLD) + circ(6, 32.1, 1.8, GOLD) + circ(34, 32.1, 1.8, GOLD) +
        rope('M25 27.6l-1.4 5M27 27.6l1.6 4.8', 1, '#a32a2a') + circ(26, 26.4, 2.6, '#a32a2a') + '<circle cx="26" cy="26.4" r="1.4" fill="none" stroke="rgb(0 0 0 / .3)" stroke-width=".5"/>';
    }],
    ['ink', 'Ink and quill', 'none', function () {
      var vane = 'M35 2C28 4 22 10 20 18L22.6 17C28.6 14 33 9 35 2Z';
      return shadow(12) + vol('M10 23C10 20 13 19 20 19S30 20 30 23V31C30 33.6 26 35 20 35S10 33.6 10 31Z', null, 'h') +
        vol('M15.6 15H24.4V19.6H15.6Z', null, 'h') + volE('ellipse cx="20" cy="15" rx="4.4" ry="1.2"', '#1a1a22', 't') +
        ln('M12.4 23.6V30.6', 'rgb(255 255 255 / .55)', 1.1) + ln('M16 15.6v3.2', 'rgb(255 255 255 / .45)', 0.6) +
        vol('M17 19.4v2.6a.9.9 0 0 0 1.8 0V19.4', null, 't') +
        vol(vane, '#f4efe4', 'h') + ln('M23.6 15.6l1.4-4.4M26 13.6l1.4-5M28.6 11.4l1.2-5.4M31 9l1.2-4.6M24.6 16l2.6-.6M27.2 14.4l2.8-.8M29.8 12l2.6-1', 'rgb(0 0 0 / .18)', 0.45) +
        rope('M18.4 21.4L33.4 4', 0.6, '#d9cfba');
    }],
    ['bundle', 'Wrapped bundle', 'none', function () {
      var tw = '#c9a54a';
      return shadow(14) + vol('M8 14L20 20V34L8 28Z', null, 't') + vol('M20 20L32 14V28L20 34Z', null, 't') + sh('M20 20L32 14V28L20 34Z', 0.24) +
        vol('M8 14L20 8L32 14L20 20Z') + hl('M8 14L20 8L32 14L20 20Z', 0.2) +
        ln('M11 18.6l4 4M24 24l4-3.4M13 12.4l3 .6M25 11.4l2.4 1.6', DK, 0.5) +
        rope('M14 11L26 17M26 11L14 17M14 17V31M26 17V31', 1.2, tw) +
        '<g transform="rotate(-20 20 14)">' + volE('ellipse cx="17.4" cy="13" rx="2.6" ry="1.6"', tw, 't') + volE('ellipse cx="22.6" cy="13" rx="2.6" ry="1.6"', tw, 't') + '</g>' + circ(20, 14, 1.2, tw) +
        ln('M26 25q3 1 4 3', '#6b5a44', 0.6) + '<g transform="rotate(16 31 30)">' + vol('M28.6 28h5v4.4h-5z', PAPER, 't') + '</g>';
    }]
  ];

  var CSS = '.cpx-m{transform-box:view-box}.cpx-hide{opacity:0}' +
    '@keyframes cpx-sway{0%,100%{transform:rotate(-5deg)}50%{transform:rotate(5deg)}}' +
    '@keyframes cpx-slosh{0%,100%{transform:rotate(0)}25%{transform:rotate(-7deg)}75%{transform:rotate(7deg)}}' +
    '@keyframes cpx-wave{to{transform:translateX(-8px)}}' +
    '@keyframes cpx-rise{0%{transform:translateY(0);opacity:0}15%{opacity:1}80%{opacity:.8}100%{transform:translateY(-13px);opacity:0}}' +
    '@keyframes cpx-twinkle{0%,45%,100%{opacity:0;transform:scale(.2) rotate(0)}70%{opacity:1;transform:scale(1) rotate(45deg)}}' +
    '@keyframes cpx-sweep{0%{transform:translateX(0);opacity:0}8%{opacity:1}55%{transform:translateX(46px);opacity:1}56%,100%{transform:translateX(46px);opacity:0}}' +
    '@keyframes cpx-glow{0%,100%{opacity:.45;transform:scale(.94)}50%{opacity:1;transform:scale(1.06)}}' +
    '@keyframes cpx-pulse{0%,100%{transform:scale(.95)}50%{transform:scale(1.05)}}' +
    '@keyframes cpx-beat{0%,45%,100%{transform:scale(1)}10%{transform:scale(1.12)}20%{transform:scale(.97)}30%{transform:scale(1.08)}}' +
    '@keyframes cpx-flicker{0%,100%{transform:scaleY(1) skewX(0)}20%{transform:scaleY(1.1) skewX(-5deg)}45%{transform:scaleY(.92) skewX(4deg)}70%{transform:scaleY(1.06) skewX(-2deg)}}' +
    '@keyframes cpx-drip{0%{transform:scale(.3);opacity:1}45%{transform:scale(1) translateY(0);opacity:1}80%{transform:translateY(3px);opacity:1}100%{transform:translateY(7px);opacity:0}}' +
    '@keyframes cpx-flip{0%,20%{transform:scaleX(1)}50%{transform:scaleX(-1)}80%,100%{transform:scaleX(1)}}' +
    '@keyframes cpx-writhe{from{transform:rotate(-7deg)}to{transform:rotate(7deg)}}' +
    '@keyframes cpx-look{0%,100%{transform:translate(0,0)}20%{transform:translate(-2.6px,-.6px)}45%{transform:translate(2.4px,-1px)}70%{transform:translate(1px,1.6px)}}' +
    '@keyframes cpx-blink{0%,86%,100%{opacity:0;transform:scaleY(.1)}93%{opacity:1;transform:scaleY(1)}}' +
    '@keyframes cpx-float{0%,100%{transform:translateY(0)}50%{transform:translateY(-1.8px)}}' +
    '@keyframes cpx-spin{to{transform:rotate(360deg)}}' +
    '.cpx-touch:hover .cpx-m,.cpx-touch:focus-visible .cpx-m,.cpx-touch.cpx-held .cpx-m{animation:var(--an)}' +
    'html[data-motion] .cpx-m{animation:none!important}@media (prefers-reduced-motion:reduce){.cpx-m{animation:none!important}}';
  if (typeof document !== 'undefined' && !document.getElementById('cpx-css')) {
    var st = document.createElement('style');
    st.id = 'cpx-css'; st.textContent = CSS;
    (document.head || document.documentElement).appendChild(st);
  }

  var BY = {}, LIST = [];
  P.forEach(function (p) { BY[p[0]] = p; LIST.push({key: p[0], label: p[1], motion: p[2]}); });

  /* The drawing's insides, for a skin that places it in its own SVG. */
  function inner(key, color) {
    var p = BY[key];
    if (!p) return '<circle cx="20" cy="20" r="14" fill="none" stroke="currentColor" stroke-width="1.6" stroke-dasharray="4 3"/><text x="20" y="25" text-anchor="middle" font-size="14" font-weight="700" fill="currentColor">?</text>';
    C = color || '#8a8a8a'; U = 'cpx' + (++seq) + '-';
    var art = p[3](), defs = '';
    Object.keys(GRADS).forEach(function (g) { if (art.indexOf('url(#' + U + g + ')') >= 0) defs += GRADS[g].replace(/@/g, U); });
    return (defs ? '<defs>' + defs + '</defs>' : '') + art;
  }
  /* A standalone picture. opts.label names it for screen readers; without
     one it is decoration beside the material's name. */
  function cls(key) { var p = BY[key]; return 'cpx' + (p && p[2] !== 'none' ? ' cpx-' + p[2] : ''); }
  function svg(key, color, size, opts) {
    var s = size || 32, o = opts || {};
    return '<svg class="cra-ico ' + cls(key) + (o.cls ? ' ' + o.cls : '') + '" viewBox="0 0 40 40" width="' + s + '" height="' + s + '"' +
      (o.label ? ' role="img" aria-label="' + String(o.label).replace(/[&<>"]/g, '') + '"' : ' aria-hidden="true"') + '>' + inner(key, color) + '</svg>';
  }
  function has(key) { return !!BY[key]; }
  function motion(key) { return BY[key] ? BY[key][2] : 'none'; }

  return {LIST: LIST, svg: svg, inner: inner, cls: cls, has: has, motion: motion};
})();
