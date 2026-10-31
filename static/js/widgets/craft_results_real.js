/**
 * craft_results_real.js -- the lifelike made-item pictures (CraftResults "real")
 *
 * The same six shapes as craft_results.js, with the same tints in the same
 * places, drawn the way the bench draws its glassware and brass: glass that
 * is clear in the middle and dark at the edges, with the room reflected in
 * it and coloured light thrown through it onto the floor; metal that mirrors
 * a bright sky above a dark horizon; wood, cork, leather and cloth with a
 * grain that the light catches. The illustrated drawings stay as the
 * "drawn" style, which an owner can pick instead.
 *
 * One warm light from the top left, as on the bench. Every surface texture
 * is an SVG filter over the tint, so one drawing works in any colour, and
 * nothing here reads anything but the cleaned spec craft_results.js hands it.
 */
(function () {
  'use strict';
  var R = window.CraftResults;
  if (!R || !R.addStyle) return;
  var h = R._h, mix = R._mix;
  var SH = {};

  /* Blending two tints, for metals: a coloured material gives its colour to
     the steel without turning it into paint. */
  function blend(a, b, t) {
    var x = parseInt(a.slice(1), 16), y = parseInt(b.slice(1), 16);
    function c(s) { var p = (x >> s) & 255, q = (y >> s) & 255; return Math.round(p + (q - p) * t); }
    return '#' + ((1 << 24) + (c(16) << 16) + (c(8) << 8) + c(0)).toString(16).slice(1);
  }
  function blurF(n, d) { return h.blur(n, d); }
  /* A surface's grain: noise as a height map, lit by the key light, laid
     over the tint. fq is the grain (two numbers stretch it along one way),
     depth how far the light picks it out. */
  function grainF(n, fq, seed, depth, oct) {
    return '<filter id="' + h.id(n) + '" x="0" y="0" width="1" height="1" color-interpolation-filters="sRGB">' +
      '<feTurbulence type="fractalNoise" baseFrequency="' + fq + '" numOctaves="' + (oct || 3) + '" seed="' + seed + '" result="t"/>' +
      '<feDiffuseLighting in="t" surfaceScale="' + depth + '" diffuseConstant="1" lighting-color="#fff" result="d"><feDistantLight azimuth="225" elevation="50"/></feDiffuseLighting>' +
      '<feComposite in="SourceGraphic" in2="d" operator="arithmetic" k1=".75" k2=".45" result="l"/>' +
      '<feComposite in="l" in2="SourceAlpha" operator="in"/></filter>';
  }
  /* Rounded form from the outline alone: the shape's blurred edge is its
     height, so a flat silhouette swells like cloth or a cushion under the
     light. lump adds wrinkles to that height. */
  function puffF(n, soft, depth, lump, seed) {
    return '<filter id="' + h.id(n) + '" x="0" y="0" width="1" height="1" color-interpolation-filters="sRGB">' +
      '<feGaussianBlur in="SourceAlpha" stdDeviation="' + soft + '" result="a"/>' +
      '<feTurbulence type="fractalNoise" baseFrequency="' + lump + '" numOctaves="3" seed="' + seed + '" result="t"/>' +
      '<feComposite in="a" in2="t" operator="arithmetic" k2="1" k3=".22" result="ht"/>' +
      '<feDiffuseLighting in="ht" surfaceScale="' + depth + '" diffuseConstant="1.15" lighting-color="#fff" result="d"><feDistantLight azimuth="225" elevation="42"/></feDiffuseLighting>' +
      '<feSpecularLighting in="ht" surfaceScale="' + depth + '" specularConstant=".35" specularExponent="14" lighting-color="#fff4e0" result="s"><feDistantLight azimuth="225" elevation="42"/></feSpecularLighting>' +
      '<feComposite in="SourceGraphic" in2="d" operator="arithmetic" k1="1" k2=".18" result="l"/>' +
      '<feComposite in="s" in2="l" operator="arithmetic" k1="0" k2=".5" k3="1" result="ls"/>' +
      '<feComposite in="ls" in2="SourceAlpha" operator="in"/></filter>';
  }
  /* Hammered or brushed metal: a fine bump that throws small highlights. */
  function metalF(n, fq, seed, k) {
    return '<filter id="' + h.id(n) + '" x="0" y="0" width="1" height="1" color-interpolation-filters="sRGB">' +
      '<feTurbulence type="fractalNoise" baseFrequency="' + fq + '" numOctaves="2" seed="' + seed + '" result="t"/>' +
      '<feSpecularLighting in="t" surfaceScale="1.2" specularConstant="' + (k || .9) + '" specularExponent="34" lighting-color="#fff6e6" result="s"><feDistantLight azimuth="225" elevation="55"/></feSpecularLighting>' +
      '<feComposite in="s" in2="SourceAlpha" operator="in" result="si"/>' +
      '<feComposite in="SourceGraphic" in2="si" operator="arithmetic" k2="1" k3=".22"/></filter>';
  }
  /* Paper fibre for the tags. */
  function paperF(n) {
    return '<filter id="' + h.id(n) + '" x="0" y="0" width="1" height="1"><feTurbulence type="fractalNoise" baseFrequency=".9 .5" numOctaves="3" seed="5"/>' +
      '<feColorMatrix type="matrix" values="0 0 0 0 .42  0 0 0 0 .32  0 0 0 0 .18  0 0 0 -3.2 1.62"/><feComposite in2="SourceGraphic" operator="in"/></filter>';
  }
  /* Light thrown through coloured glass or a stone lands as a bright, tinted
     spot inside the shadow, on the side away from the light. */
  function shadow(cx, rx, tint) {
    return '<ellipse cx="' + (cx + 8) + '" cy="110.5" rx="' + (rx + 6) + '" ry="6.5" fill="rgb(26 16 10 / .3)" filter="' + h.url('sh') + '"/>' +
      '<ellipse cx="' + (cx + 1) + '" cy="108.6" rx="' + (rx * .55) + '" ry="2.2" fill="rgb(34 28 26 / .5)" filter="' + h.url('ao') + '"/>' +
      (tint ? '<ellipse cx="' + (cx + 13) + '" cy="110.4" rx="' + (rx * .45) + '" ry="3.2" fill="' + mix(tint, .55) + '" opacity=".75" filter="' + h.url('sh') + '"/>' : '');
  }
  function shadowDefs() { return blurF('sh', 2.6) + blurF('ao', 1.1) + blurF('s1', .5) + blurF('s2', 1.2) + blurF('s4', 3); }
  /* The material's own picture printed on a paper tag, as ink would sit. */
  function tag(s, x, y, rot, tx) {
    return '<path d="' + tx + '" fill="none" stroke="' + mix(s.trim, -.35) + '" stroke-width="1.1"/>' +
      '<g transform="rotate(' + rot + ' ' + (x + 12) + ' ' + (y + 11) + ')">' +
      '<path d="M' + (x + 2) + ' ' + (y + 3) + ' h24 v20 h-24 l-4 -10z" fill="rgb(20 12 8 / .35)" filter="' + h.url('s2') + '"/>' +
      '<path d="M' + x + ' ' + y + ' h24 v20 h-24 l-4 -10z" fill="' + h.url('tg') + '"/>' +
      '<path d="M' + x + ' ' + y + ' h24 v20 h-24 l-4 -10z" fill="#e9dcc0" filter="' + h.url('pf') + '"/>' +
      '<circle cx="' + (x + 1.6) + '" cy="' + (y + 10) + '" r="2.2" fill="none" stroke="#b49a6a" stroke-width="1.1"/><circle cx="' + (x + 1.6) + '" cy="' + (y + 10) + '" r="1.1" fill="rgb(30 20 10 / .55)"/>' +
      '<g style="mix-blend-mode:multiply" opacity=".92">' + h.mark(s.mark, x + 4, y + 1, 18) + '</g></g>';
  }
  function tagDefs() {
    return h.grad('tg', 0, 0, 1, 1, [[0, '#f7efdc'], [.6, '#ead9b6'], [1, '#d4bf94']]) + paperF('pf');
  }
  /* Cork: porous, a little crumbly at the top edge. */
  function corkDefs(t) {
    return h.grad('ck', 0, 0, 1, 0, [[0, mix(t, -.55)], [.25, mix(t, .12)], [.42, mix(t, .3)], [.75, mix(t, -.15)], [1, mix(t, -.6)]]) +
      h.rgrad('ckt', .4, .4, .7, [[0, mix(t, .35)], [1, mix(t, -.15)]]) + grainF('cg', '.42', 11, 2.4, 3);
  }

  /* --- the shapes ------------------------------------------------------ */

  /* A round-bottomed flask. The brew gathers the light that comes through
     the glass into a bright patch low on the far side and goes dark where
     it is deepest at the rim; flecks of the detail tint hang in it, some in
     focus and some not. The glass is nearly invisible except at its edges,
     where it turns dark and then bright, and where it mirrors the window. */
  SH.potion = function (s) {
    var b = s.body, X = 60, Y = 74, Rr = 33;
    var defs = '<defs>' + shadowDefs() + tagDefs() + corkDefs(s.trim) +
      h.rgrad('lq', .64, .8, .72, [[0, mix(b, .62)], [.18, mix(b, .3)], [.45, b], [.78, mix(b, -.45)], [1, mix(b, -.72)]]) +
      h.grad('lt', 0, 0, 1, 0, [[0, mix(b, -.35)], [.35, mix(b, .45)], [.6, mix(b, .2)], [1, mix(b, -.3)]]) +
      h.rgrad('fr', .5, .5, .5, [[0, '#fff', 0], [.7, '#fff', .02], [.86, '#0b0806', .2], [.945, '#fff', .38], [.975, '#fff', .55], [1, '#fff', .12]]) +
      h.grad('nk', 0, 0, 1, 0, [[0, '#0b0806', .4], [.1, '#fff', .55], [.2, '#fff', .12], [.55, '#fff', .03], [.78, '#ffe4bc', .32], [.88, '#fff', .05], [1, '#0b0806', .45]]) +
      h.grad('lp', 0, 0, 0, 1, [[0, '#fff', .75], [.5, '#e6f0f2', .25], [1, '#0b0806', .3]]) +
      h.grad('tw', 0, 0, 0, 1, [[0, mix(s.trim, .25)], [.5, mix(s.trim, -.2)], [1, mix(s.trim, -.6)]]) +
      '<clipPath id="' + h.id('in') + '"><circle cx="' + X + '" cy="' + Y + '" r="' + (Rr - 1.5) + '"/></clipPath></defs>';
    var fl = '';
    [[48, 90, 1.3, 0], [70, 96, .9, 0], [57, 101, 1.1, 1], [74, 84, 1.6, 1], [44, 98, 1.8, 1], [63, 88, .7, 0], [52, 82, .8, 0]].forEach(function (p) {
      fl += '<circle cx="' + p[0] + '" cy="' + p[1] + '" r="' + p[2] + '" fill="' + mix(s.detail, .2) + '" opacity="' + (p[3] ? .55 : .95) + '"' + (p[3] ? ' filter="' + h.url('s1') + '"' : '') + '/>';
    });
    var bub = '';
    [[66, 98, 1.6], [69, 91, 1.1], [71, 85, .8], [55, 94, .9]].forEach(function (p) {
      bub += '<circle cx="' + p[0] + '" cy="' + p[1] + '" r="' + p[2] + '" fill="' + mix(b, .3) + '" fill-opacity=".35" stroke="#fff" stroke-opacity=".55" stroke-width=".45"/><circle cx="' + (p[0] - p[2] * .35) + '" cy="' + (p[1] - p[2] * .35) + '" r="' + (p[2] * .3) + '" fill="#fff" opacity=".9"/>';
    });
    var twine = '';
    for (var i = 0; i < 3; i++) {
      var y = 25 + i * 2.2;
      twine += '<path d="M51.6 ' + y + ' Q60 ' + (y + 2.6) + ' 68.4 ' + y + '" fill="none" stroke="' + mix(s.trim, -.55) + '" stroke-width="2"/>' +
        '<path d="M51.6 ' + (y - .3) + ' Q60 ' + (y + 2.3) + ' 68.4 ' + (y - .3) + '" fill="none" stroke="' + mix(s.trim, .1) + '" stroke-width="1.1" stroke-dasharray="1.1 .7"/>';
    }
    return defs + shadow(X, 28, b) +
      /* the room seen through the back of the glass, then the brew */
      '<circle cx="' + X + '" cy="' + Y + '" r="' + Rr + '" fill="#e8f2f4" opacity=".07"/>' +
      '<g clip-path="' + h.url('in') + '">' +
      '<rect x="20" y="73" width="80" height="40" fill="' + h.url('lq') + '"/>' +
      '<circle cx="' + X + '" cy="' + Y + '" r="' + (Rr - 1.5) + '" fill="none" stroke="' + mix(b, -.78) + '" stroke-width="7" opacity=".5" filter="' + h.url('s2') + '"/>' +
      '<ellipse cx="69" cy="95" rx="13" ry="8" fill="' + mix(b, .7) + '" opacity=".45" filter="' + h.url('s4') + '"/>' + fl + bub +
      '<ellipse cx="' + X + '" cy="73" rx="31.4" ry="3.6" fill="' + h.url('lt') + '" opacity=".92"/>' +
      '<path d="M28.6 73 A31.4 3.6 0 0 1 91.4 73" fill="none" stroke="#fff" stroke-opacity=".45" stroke-width=".6"/>' +
      '<path d="M28.6 73 A31.4 3.6 0 0 0 91.4 73" fill="none" stroke="' + mix(b, .5) + '" stroke-opacity=".8" stroke-width=".7"/>' +
      '<path d="M28.8 73.4 Q30 70.8 32.6 70.2 M91.2 73.4 Q90 70.8 87.4 70.2" fill="none" stroke="#fff" stroke-opacity=".4" stroke-width=".7"/>' +
      '</g>' +
      /* neck, with the cork seen through it, the rolled lip and the twine */
      '<path d="M53 6.5 H67 L66.2 21 H53.8Z" fill="' + h.url('ck') + '"/>' +
      '<path d="M53 6.5 H67 L66.2 21 H53.8Z" fill="' + s.trim + '" opacity=".55" filter="' + h.url('cg') + '"/>' +
      '<ellipse cx="60" cy="6.5" rx="7" ry="1.9" fill="' + h.url('ckt') + '"/>' +
      '<ellipse cx="60" cy="6.5" rx="7" ry="1.9" fill="' + s.trim + '" opacity=".5" filter="' + h.url('cg') + '"/>' +
      '<rect x="52" y="17" width="16" height="27" fill="#e8f2f4" opacity=".08"/>' +
      '<rect x="52" y="17" width="16" height="27" fill="' + h.url('nk') + '"/>' +
      '<rect x="50.6" y="15" width="18.8" height="4.4" rx="2.2" fill="' + h.url('lp') + '"/>' +
      '<rect x="50.6" y="15" width="18.8" height="4.4" rx="2.2" fill="none" stroke="#0b0806" stroke-opacity=".25" stroke-width=".5"/>' +
      twine +
      /* the glass: dark then bright at the rim, the window, the flame's glint */
      '<circle cx="' + X + '" cy="' + Y + '" r="' + Rr + '" fill="' + h.url('fr') + '"/>' +
      '<circle cx="' + X + '" cy="' + Y + '" r="' + (Rr - 1.5) + '" fill="none" stroke="#0b0806" stroke-opacity=".14" stroke-width=".6"/>' +
      '<path d="M53 21 V41.5 M67 21 V41.5" stroke="#fff" stroke-opacity=".3" stroke-width=".6"/>' +
      '<g filter="' + h.url('s1') + '" opacity=".6" transform="rotate(-24 44 56)"><path d="M36 51 q4 -3 8 -3.4 v6.6 h-8z M45.4 47.6 q4 0 7.6 1.6 v6 h-7.6z M36 56.4 h8 v6.8 h-8.6 q0 -3.6 .6 -6.8z M45.4 56.4 h7.6 v6.8 h-7.6z" fill="#fff"/></g>' +
      '<path d="M31.5 87 A31.5 31.5 0 0 1 36 52" fill="none" stroke="#fff" stroke-opacity=".5" stroke-width="1.4" stroke-linecap="round" filter="' + h.url('s1') + '"/>' +
      '<path d="M90 86 A31 31 0 0 1 74 103" fill="none" stroke="#ffd7a0" stroke-opacity=".55" stroke-width="2.2" stroke-linecap="round" filter="' + h.url('s1') + '"/>' +
      '<circle cx="78" cy="55" r="2.4" fill="#fff" opacity=".55" filter="' + h.url('s1') + '"/><circle cx="78" cy="55" r="1" fill="#fff"/>' +
      '<path d="M54.6 21 V40" stroke="#fff" stroke-opacity=".75" stroke-width="1.2" stroke-linecap="round"/>' +
      tag(s, 78, 40, 12, 'M68.4 27 Q76 30 79 48');
  };
  /* A tall test-tube vial. A cylinder of glass shows the room as upright
     bands: dark at both edges, a sharp window strip on the lit side and the
     flame's warm reflection on the other. The brew is brightest just right
     of centre, where the tube focuses the light, and pools a glow in the
     rounded foot. */
  SH.vial = function (s) {
    var b = s.body;
    var body = 'M47 32 V92 a13 13 0 0 0 26 0 V32z', brew = 'M48.6 54 V92 a11.4 11.4 0 0 0 22.8 0 V54z';
    var defs = '<defs>' + shadowDefs() + tagDefs() + corkDefs(s.trim) +
      h.grad('lq', 0, 0, 1, 0, [[0, mix(b, -.78)], [.12, mix(b, -.45)], [.38, mix(b, .05)], [.62, mix(b, .42)], [.8, b], [.93, mix(b, -.4)], [1, mix(b, -.75)]]) +
      h.grad('lv', 0, 0, 0, 1, [[0, '#fff', .18], [.12, '#000', 0], [.75, '#000', 0], [1, '#000', .3]]) +
      h.grad('gl', 0, 0, 1, 0, [[0, '#0b0806', .5], [.06, '#0b0806', .15], [.11, '#fff', .6], [.17, '#fff', .55], [.21, '#fff', .08], [.6, '#fff', .02], [.8, '#ffe0b4', .3], [.86, '#fff', .04], [.94, '#0b0806', .22], [1, '#0b0806', .55]]) +
      h.grad('lp', 0, 0, 0, 1, [[0, '#fff', .75], [.5, '#e6f0f2', .25], [1, '#0b0806', .3]]) +
      h.grad('wx', 0, 0, 1, 0, [[0, mix(s.detail, -.5)], [.3, mix(s.detail, .25)], [.45, mix(s.detail, .5)], [.8, mix(s.detail, -.2)], [1, mix(s.detail, -.55)]]) +
      '<clipPath id="' + h.id('in') + '"><path d="' + brew + '"/></clipPath></defs>';
    var fl = '';
    [[54, 72, 1, 0], [63, 84, .8, 0], [58, 95, 1.1, 1], [66, 66, 1.2, 1], [52, 88, .7, 0]].forEach(function (p) {
      fl += '<circle cx="' + p[0] + '" cy="' + p[1] + '" r="' + p[2] + '" fill="' + mix(s.detail, .2) + '" opacity="' + (p[3] ? .5 : .95) + '"' + (p[3] ? ' filter="' + h.url('s1') + '"' : '') + '/>';
    });
    return defs + shadow(60, 14, b) +
      '<g transform="rotate(-5 60 104)">' +
      '<path d="' + body + '" fill="#e8f2f4" opacity=".08"/>' +
      '<g clip-path="' + h.url('in') + '">' +
      '<rect x="46" y="52" width="28" height="56" fill="' + h.url('lq') + '"/>' +
      '<rect x="46" y="52" width="28" height="56" fill="' + h.url('lv') + '"/>' +
      '<ellipse cx="63" cy="98" rx="8" ry="6" fill="' + mix(b, .7) + '" opacity=".5" filter="' + h.url('s2') + '"/>' + fl +
      '<circle cx="64" cy="90" r="1.1" fill="none" stroke="#fff" stroke-opacity=".6" stroke-width=".45"/><circle cx="66" cy="80" r=".8" fill="none" stroke="#fff" stroke-opacity=".6" stroke-width=".4"/>' +
      '<ellipse cx="60" cy="54" rx="11.4" ry="2.1" fill="' + mix(b, .4) + '" opacity=".9"/>' +
      '<path d="M48.6 54 A11.4 2.1 0 0 0 71.4 54" fill="none" stroke="' + mix(b, .6) + '" stroke-width=".6"/>' +
      '<path d="M48.6 54.2 Q49 51.6 51 51.2 M71.4 54.2 Q71 51.6 69 51.2" fill="none" stroke="#fff" stroke-opacity=".45" stroke-width=".6"/>' +
      '</g>' +
      /* cork, dipped in wax of the detail tint, under the glass rim */
      '<path d="M50 12 H70 L69 34 H51Z" fill="' + h.url('ck') + '"/>' +
      '<path d="M50 12 H70 L69 34 H51Z" fill="' + s.trim + '" opacity=".55" filter="' + h.url('cg') + '"/>' +
      '<path d="M49.4 11 H70.6 V17 q-1.4 3.4 -2.6 .4 q-1.6 4.8 -3.2 .6 q-2 2.8 -3.6 -.2 q-1.8 5 -3.8 .4 q-1.6 2.6 -3 .2 q-1.4 3.6 -2.8 -.2 q-.8 1.6 -1.6 0z" fill="' + h.url('wx') + '"/>' +
      '<ellipse cx="60" cy="11" rx="10.6" ry="2.4" fill="' + mix(s.detail, .35) + '"/>' +
      '<path d="M52 12.6 Q55 10.6 60 10.4" fill="none" stroke="#fff" stroke-opacity=".7" stroke-width="1" stroke-linecap="round"/>' +
      '<path d="M47 32 H73 V40 H47z" fill="#e8f2f4" opacity=".05"/>' +
      '<path d="' + body + '" fill="' + h.url('gl') + '"/>' +
      '<path d="' + body + '" fill="none" stroke="#0b0806" stroke-opacity=".22" stroke-width=".6"/>' +
      '<rect x="45.2" y="29.4" width="29.6" height="4.4" rx="2.2" fill="' + h.url('lp') + '"/>' +
      '<rect x="45.2" y="29.4" width="29.6" height="4.4" rx="2.2" fill="none" stroke="#0b0806" stroke-opacity=".25" stroke-width=".5"/>' +
      '<path d="M50.4 37 V93" stroke="#fff" stroke-opacity=".8" stroke-width="1.2" stroke-linecap="round"/>' +
      '<path d="M69.4 60 V92 Q69 98 64 101" fill="none" stroke="#ffd7a0" stroke-opacity=".5" stroke-width="1.6" stroke-linecap="round" filter="' + h.url('s1') + '"/>' +
      '<ellipse cx="60" cy="103" rx="6" ry="1.6" fill="#fff" opacity=".35" filter="' + h.url('s1') + '"/>' +
      tag(s, 80, 46, 14, 'M73 38 Q80 40 81 56') +
      '</g>';
  };
  /* A short sword. Polished steel mirrors what is around it, so each bevel
     is a strip of the room: the upper bevel catches the bright sky and the
     lower one the dark horizon and the warm floor, with a long brushed grain
     along the blade. The guard and pommel are the trim as polished metal,
     the grip the detail as leather wound on the bias, and the mark is set
     under a glass cabochon in the pommel. */
  SH.blade = function (s) {
    var t = s.trim, d = s.detail, st = blend(s.body, '#a9b0b8', .45);
    var T = 'rotate(-38 60 66) translate(60 62) scale(1.12 1.25) translate(-60 -62)';
    var bladeP = 'M50 58 H100 L112 62 L100 66 H50Z';
    var defs = '<defs>' + shadowDefs() +
      h.grad('ub', 0, 0, 0, 1, [[0, mix(st, .88)], [.3, mix(st, .55)], [.7, mix(st, .05)], [1, mix(st, -.2)]]) +
      h.grad('lb', 0, 0, 0, 1, [[0, mix(st, -.62)], [.3, mix(st, -.38)], [.62, mix(st, .1)], [.86, blend(mix(st, .3), '#ffd9a8', .35)], [1, mix(st, -.3)]]) +
      h.grad('gd', 0, 0, 1, 0, [[0, mix(t, -.72)], [.2, mix(t, -.15)], [.38, mix(t, .6)], [.5, mix(t, .9)], [.62, t], [.85, mix(t, -.5)], [1, mix(t, -.75)]]) +
      h.grad('fe', 0, 0, 0, 1, [[0, mix(t, .7)], [.35, t], [.7, mix(t, -.55)], [1, mix(t, -.3)]]) +
      h.grad('gp', 0, 0, 0, 1, [[0, mix(d, -.65)], [.28, mix(d, .2)], [.42, mix(d, .38)], [.72, mix(d, -.25)], [1, mix(d, -.7)]]) +
      h.rgrad('pm', .36, .3, .8, [[0, mix(t, .95)], [.12, mix(t, .6)], [.4, t], [.68, mix(t, -.55)], [.82, mix(t, -.2)], [.92, mix(t, .15)], [1, mix(t, -.7)]]) +
      h.rgrad('cb', .35, .3, .75, [[0, '#fff', .7], [.3, '#fff', .12], [.7, '#fff', 0], [1, '#000', .25]]) +
      '<filter id="' + h.id('br') + '" x="0" y="0" width="1" height="1"><feTurbulence type="fractalNoise" baseFrequency=".012 .9" numOctaves="2" seed="4"/>' +
      '<feColorMatrix type="matrix" values="0 0 0 0 1  0 0 0 0 1  0 0 0 0 1  0 0 0 1.6 -.75"/><feComposite in2="SourceGraphic" operator="in"/></filter>' +
      metalF('mt', '.35', 9, .8) + grainF('lg', '.9 .5', 17, 1.6, 2) + '</defs>';
    var wraps = '';
    for (var i = 0; i < 6; i++) {
      var x = 26 + i * 3.1;
      wraps += '<path d="M' + x + ' 58.6 h3.1 l-1.5 6.8 h-3.1z" fill="' + h.url('gp') + '"/>' +
        '<path d="M' + (x + 3.1) + ' 58.6 l-1.5 6.8" stroke="' + mix(d, -.75) + '" stroke-width=".55" opacity=".85"/>' +
        '<path d="M' + (x + .6) + ' 59.6 h1.6" stroke="#fff" stroke-opacity=".35" stroke-width=".5" stroke-linecap="round"/>';
    }
    var sil = '<g transform="' + T + '" fill="#1a1008"><path d="' + bladeP + '"/><rect x="44" y="48" width="7" height="28" rx="3"/><rect x="25" y="58.6" width="20" height="6.8"/><circle cx="17.5" cy="62" r="8.2"/></g>';
    return defs +
      '<g transform="translate(4 7)" opacity=".38" filter="' + h.url('sh') + '">' + sil + '</g>' +
      '<g transform="' + T + '">' +
      /* the blade: two bevels, its grain, the fuller and the honed edges */
      '<path d="M50 58 H100 L112 62 H50Z" fill="' + h.url('ub') + '"/>' +
      '<path d="M50 62 H112 L100 66 H50Z" fill="' + h.url('lb') + '"/>' +
      '<path d="' + bladeP + '" fill="#fff" opacity=".28" filter="' + h.url('br') + '"/>' +
      '<path d="M55 60.7 H93" stroke="' + mix(st, -.6) + '" stroke-width="1.2" stroke-linecap="round" opacity=".75"/>' +
      '<path d="M55 61.7 H93" stroke="#fff" stroke-opacity=".5" stroke-width=".45" stroke-linecap="round"/>' +
      '<path d="M50 58.25 H100 L111.4 61.8" fill="none" stroke="#fff" stroke-opacity=".9" stroke-width=".55"/>' +
      '<path d="M50 65.75 H100 L111.4 62.2" fill="none" stroke="' + mix(st, .55) + '" stroke-opacity=".7" stroke-width=".45"/>' +
      '<ellipse cx="80" cy="59.3" rx="13" ry=".9" fill="#fff" opacity=".85" filter="' + h.url('s1') + '"/>' +
      '<ellipse cx="74" cy="64.4" rx="9" ry=".7" fill="#ffd9a8" opacity=".55" filter="' + h.url('s1') + '"/>' +
      '<path d="' + bladeP + '" fill="none" stroke="rgb(20 14 10 / .5)" stroke-width=".45"/>' +
      /* the langet and crossguard */
      '<path d="M50.6 59 L56 62 L50.6 65Z" fill="' + h.url('fe') + '" stroke="rgb(20 14 10 / .4)" stroke-width=".4"/>' +
      '<rect x="44" y="48" width="7" height="28" rx="3" fill="' + h.url('gd') + '"/>' +
      '<rect x="44" y="48" width="7" height="28" rx="3" fill="' + t + '" opacity=".25" filter="' + h.url('mt') + '"/>' +
      '<rect x="44" y="48" width="7" height="28" rx="3" fill="none" stroke="rgb(20 14 10 / .45)" stroke-width=".5"/>' +
      '<circle cx="47.5" cy="47.6" r="3.4" fill="' + h.url('pm') + '"/><circle cx="47.5" cy="76.4" r="3.4" fill="' + h.url('pm') + '"/>' +
      /* the grip and its two ferrules */
      '<rect x="25" y="58.6" width="19" height="6.8" fill="' + h.url('gp') + '"/>' + wraps +
      '<rect x="25" y="58.6" width="19" height="6.8" fill="' + d + '" opacity=".3" filter="' + h.url('lg') + '"/>' +
      '<rect x="42.2" y="58" width="2.4" height="8" rx=".6" fill="' + h.url('fe') + '"/><rect x="24.2" y="58" width="2.4" height="8" rx=".6" fill="' + h.url('fe') + '"/>' +
      /* the pommel, and the mark under its glass */
      '<circle cx="17.5" cy="62" r="8.2" fill="' + h.url('pm') + '"/>' +
      '<circle cx="17.5" cy="62" r="8.2" fill="' + t + '" opacity=".2" filter="' + h.url('mt') + '"/>' +
      '<circle cx="17.5" cy="62" r="5.4" fill="rgb(16 10 6 / .75)" stroke="' + mix(t, .45) + '" stroke-width=".7"/>' +
      '<g transform="rotate(38 17.5 62)">' + h.mark(s.mark, 12.4, 56.9, 10.2) + '</g>' +
      '<circle cx="17.5" cy="62" r="4.9" fill="' + h.url('cb') + '"/>' +
      '<path d="M14.6 59.4 a4 4 0 0 1 3.4 -1.7" fill="none" stroke="#fff" stroke-opacity=".9" stroke-width=".7" stroke-linecap="round"/>' +
      '<circle cx="17.5" cy="62" r="8.2" fill="none" stroke="rgb(20 14 10 / .5)" stroke-width=".5"/>' +
      '</g>' +
      '<path d="M97 37 l.9 2.7 2.7 .9 -2.7 .9 -.9 2.7 -.9 -2.7 -2.7 -.9 2.7 -.9z" fill="#fff" opacity=".85" filter="' + h.url('s1') + '"/>';
  };
  /* A round buckler, hung face on. The face is painted planks in the body
     tint, the grain showing through and the paint rubbed back to wood at the
     seams; the rim and rivets are hammered trim metal; two bands of the
     detail tint cross it; the boss is a polished dome with the mark enamelled
     on its crown. Its shadow falls on the wall behind it. */
  SH.shield = function (s) {
    var b = s.body, t = s.trim, d = s.detail;
    var c0 = mix(b, -.45), c1 = mix(b, .12);
    function rgb(hx) { var n = parseInt(hx.slice(1), 16); return [(n >> 16 & 255) / 255, (n >> 8 & 255) / 255, (n & 255) / 255]; }
    var A = rgb(c0), B = rgb(c1);
    var defs = '<defs>' + shadowDefs() +
      '<filter id="' + h.id('wd') + '" x="0" y="0" width="1" height="1" color-interpolation-filters="sRGB">' +
      '<feTurbulence type="fractalNoise" baseFrequency=".3 .014" numOctaves="3" seed="21" result="t"/>' +
      '<feComponentTransfer in="t" result="r"><feFuncR type="table" tableValues="0 .55 .3 .75 .45 .9 .5 .8 .35 .65 1"/></feComponentTransfer>' +
      '<feColorMatrix in="r" type="matrix" values="' + (B[0] - A[0]).toFixed(3) + ' 0 0 0 ' + A[0].toFixed(3) + '  ' + (B[1] - A[1]).toFixed(3) + ' 0 0 0 ' + A[1].toFixed(3) + '  ' + (B[2] - A[2]).toFixed(3) + ' 0 0 0 ' + A[2].toFixed(3) + '  0 0 0 0 1" result="c"/>' +
      '<feDiffuseLighting in="t" surfaceScale="1.4" diffuseConstant="1" lighting-color="#fff" result="dl"><feDistantLight azimuth="225" elevation="55"/></feDiffuseLighting>' +
      '<feComposite in="c" in2="dl" operator="arithmetic" k1=".8" k2=".35" result="l"/>' +
      '<feComposite in="l" in2="SourceAlpha" operator="in"/></filter>' +
      '<filter id="' + h.id('wr') + '" x="0" y="0" width="1" height="1"><feTurbulence type="fractalNoise" baseFrequency=".16" numOctaves="4" seed="33"/>' +
      '<feColorMatrix type="matrix" values="0 0 0 0 .62  0 0 0 0 .46  0 0 0 0 .3  0 0 0 -14 6.6"/><feComposite in2="SourceGraphic" operator="in"/></filter>' +
      metalF('mt', '.75', 7, 1) +
      h.grad('rm', 0, 0, 1, 1, [[0, mix(t, .7)], [.3, mix(t, .2)], [.55, mix(t, -.3)], [.8, mix(t, -.55)], [1, mix(t, -.2)]]) +
      h.grad('cr', 0, 0, 1, 1, [[0, '#fff', .85], [.35, '#fff', .25], [.6, '#fff', 0], [.9, '#ffd9a8', .25], [1, '#ffd9a8', .05]]) +
      h.rgrad('dm', .4, .35, .8, [[0, '#fff', .3], [.45, '#fff', 0], [.75, '#000', .05], [1, '#000', .45]]) +
      h.rgrad('bs', .38, .32, .78, [[0, mix(t, .95)], [.1, mix(t, .65)], [.35, t], [.58, mix(t, -.55)], [.7, mix(t, -.3)], [.84, mix(t, .25)], [1, mix(t, -.7)]]) +
      h.rgrad('rv', .35, .3, .7, [[0, mix(t, .9)], [.4, t], [1, mix(t, -.7)]]) +
      h.rgrad('cb', .35, .3, .75, [[0, '#fff', .7], [.3, '#fff', .12], [.7, '#fff', 0], [1, '#000', .25]]) +
      '<clipPath id="' + h.id('fc') + '"><circle cx="60" cy="60" r="39"/></clipPath></defs>';
    function band(x1, y1, x2, y2) {
      var dx = x2 - x1, dy = y2 - y1, L = Math.sqrt(dx * dx + dy * dy), nx = -dy / L * 4.6, ny = dx / L * 4.6;
      var gid = 'bd' + Math.round(x1) + Math.round(y1);
      var g = '<linearGradient id="' + h.id(gid) + '" gradientUnits="userSpaceOnUse" x1="' + (60 - nx) + '" y1="' + (60 - ny) + '" x2="' + (60 + nx) + '" y2="' + (60 + ny) + '">' +
        [[0, mix(d, -.6)], [.15, mix(d, .55)], [.35, mix(d, .2)], [.7, mix(d, -.25)], [.9, mix(d, -.55)], [1, mix(d, -.75)]].map(function (q) { return '<stop offset="' + q[0] + '" stop-color="' + q[1] + '"/>'; }).join('') + '</linearGradient>';
      var p = 'M' + x1 + ' ' + y1 + ' L' + x2 + ' ' + y2, r = '';
      for (var k = 1; k < 6; k++) { if (k === 3) continue; var rx = x1 + dx * k / 6, ry = y1 + dy * k / 6; r += '<circle cx="' + rx.toFixed(1) + '" cy="' + ry.toFixed(1) + '" r="1.5" fill="' + h.url('rv') + '"/>'; }
      return '<defs>' + g + '</defs><path d="' + p + '" stroke="rgb(16 10 6 / .45)" stroke-width="10.6" transform="translate(.8 1.2)" filter="' + h.url('s1') + '"/>' +
        '<path d="' + p + '" stroke="' + h.url(gid) + '" stroke-width="9.2"/><path d="' + p + '" stroke="' + d + '" stroke-width="9.2" opacity=".3" filter="' + h.url('mt') + '"/>' + r;
    }
    var seams = '';
    [34, 47, 60, 73, 86].forEach(function (x) {
      seams += '<path d="M' + x + ' 18 V102" stroke="rgb(16 10 6 / .55)" stroke-width=".9"/><path d="M' + (x + .9) + ' 18 V102" stroke="#fff" stroke-opacity=".12" stroke-width=".5"/>';
    });
    var rivets = '';
    for (var i = 0; i < 12; i++) {
      var a = (i + .5) / 12 * Math.PI * 2, x = 60 + Math.cos(a) * 44, y = 60 + Math.sin(a) * 44;
      rivets += '<circle cx="' + (x + .5).toFixed(1) + '" cy="' + (y + .8).toFixed(1) + '" r="2.2" fill="rgb(16 10 6 / .45)" filter="' + h.url('s1') + '"/><circle cx="' + x.toFixed(1) + '" cy="' + y.toFixed(1) + '" r="2" fill="' + h.url('rv') + '"/>';
    }
    return defs +
      '<circle cx="64" cy="66" r="49" fill="rgb(26 16 10 / .38)" filter="' + h.url('s4') + '"/>' +
      /* the face */
      '<g clip-path="' + h.url('fc') + '">' +
      '<rect x="20" y="20" width="80" height="80" fill="#000" filter="' + h.url('wd') + '"/>' + seams +
      '<rect x="20" y="20" width="80" height="80" fill="#a8845a" filter="' + h.url('wr') + '" opacity=".55"/>' +
      band(30, 34, 90, 86) + band(30, 86, 90, 34) +
      '<circle cx="60" cy="60" r="39" fill="' + h.url('dm') + '"/>' +
      '<circle cx="60" cy="60" r="39" fill="none" stroke="rgb(16 10 6 / .6)" stroke-width="5" filter="' + h.url('s2') + '"/></g>' +
      /* the rim, its crest catching the light, and the rivets */
      '<path d="M60 11 a49 49 0 1 0 .01 0z M60 21 a39 39 0 1 1 -.01 0z" fill-rule="evenodd" fill="' + h.url('rm') + '"/>' +
      '<path d="M60 11 a49 49 0 1 0 .01 0z M60 21 a39 39 0 1 1 -.01 0z" fill-rule="evenodd" fill="' + t + '" opacity=".3" filter="' + h.url('mt') + '"/>' +
      '<circle cx="60" cy="60" r="44.6" fill="none" stroke="' + h.url('cr') + '" stroke-width="2.6" filter="' + h.url('s1') + '"/>' +
      '<circle cx="60" cy="60" r="49" fill="none" stroke="rgb(16 10 6 / .55)" stroke-width=".7"/>' +
      '<circle cx="60" cy="60" r="39" fill="none" stroke="rgb(16 10 6 / .5)" stroke-width=".7"/>' + rivets +
      /* the boss and its enamelled mark */
      '<circle cx="62.6" cy="63.6" r="14.6" fill="rgb(16 10 6 / .5)" filter="' + h.url('s2') + '"/>' +
      '<circle cx="60" cy="60" r="14" fill="' + h.url('bs') + '"/>' +
      '<circle cx="60" cy="60" r="14" fill="' + t + '" opacity=".22" filter="' + h.url('mt') + '"/>' +
      '<circle cx="60" cy="60" r="8.6" fill="rgb(16 10 6 / .7)" stroke="' + mix(t, .5) + '" stroke-width=".8"/>' +
      h.mark(s.mark, 52.6, 52.6, 14.8) +
      '<circle cx="60" cy="60" r="8" fill="' + h.url('cb') + '"/>' +
      '<path d="M50.4 53.6 a11 11 0 0 1 6.6 -5" fill="none" stroke="#fff" stroke-opacity=".85" stroke-width="1.3" stroke-linecap="round" filter="' + h.url('s1') + '"/>' +
      '<circle cx="60" cy="60" r="14" fill="none" stroke="rgb(16 10 6 / .5)" stroke-width=".6"/>';
  };
  /* A ring lying tilted, a cut stone in its setting. The band is the trim
     as polished metal, its inside catching the light on the far side; the
     stone is the body tint cut into facets, each lit by which way it faces,
     with flashes of fire and a pool of its colour thrown on the floor. The
     detail tint is the claws that hold it. */
  SH.ring = function (s) {
    var b = s.body, t = s.trim, d = s.detail, G = 40;
    var defs = '<defs>' + shadowDefs() +
      h.grad('bk', 0, 0, 1, 0, [[0, mix(t, -.3), 0], [.12, mix(t, -.3), .9], [.5, mix(t, .7), .9], [.88, mix(t, -.2), .9], [1, mix(t, -.2), 0]]) +
      h.grad('fr', 0, 0, 1, 0, [[0, mix(t, -.7)], [.12, mix(t, .1)], [.28, mix(t, .85)], [.4, t], [.62, mix(t, -.45)], [.82, mix(t, .4)], [.92, mix(t, .1)], [1, mix(t, -.7)]]) +
      h.grad('cl', 0, 0, 1, 0, [[0, mix(t, -.65)], [.3, mix(t, .5)], [.55, t], [1, mix(t, -.6)]]) +
      h.grad('pr', 0, 0, 1, 0, [[0, mix(d, -.6)], [.35, mix(d, .6)], [1, mix(d, -.5)]]) +
      h.grad('tb', 0, 0, 1, 1, [[0, mix(b, .55)], [.35, mix(b, .15)], [.5, mix(b, .7)], [.62, b], [1, mix(b, -.45)]]) +
      metalF('mt', '.8', 3, .8) + '</defs>';
    /* crown: eight facets between the table and the girdle, each shaded by
       how squarely it faces the light up and to the left */
    var crown = '', pav = '', n = 8, P = [], Q = [], ofs = [.08, -.1, .14, -.04, .1, -.12, .04, -.06];
    for (var i = 0; i < n; i++) {
      var a = (i + .5) / n * Math.PI * 2;
      P.push([60 + Math.cos(a) * 18, G + Math.sin(a) * 5.4]);
      Q.push([60 + Math.cos(a) * 9.6, G - 9 + Math.sin(a) * 2.9]);
    }
    for (i = 0; i < n; i++) {
      var j = (i + 1) % n, am = (i + 1) / n * Math.PI * 2;
      var lit = Math.cos(am - Math.PI * 1.25) * .5 + ofs[i];
      crown += '<path d="M' + Q[i] + ' L' + Q[j] + ' L' + P[j] + ' L' + P[i] + 'Z" fill="' + mix(b, lit) + '" stroke="' + mix(b, .5) + '" stroke-opacity=".55" stroke-width=".35"/>';
      if (P[i][1] >= G - .5 || P[j][1] >= G - .5) {
        pav += '<path d="M' + P[i] + ' L' + P[j] + ' L60 58Z" fill="' + mix(b, i % 2 ? -.35 : .35) + '" stroke="' + mix(b, .4) + '" stroke-opacity=".4" stroke-width=".35"/>';
      }
    }
    var table = 'M' + Q.join(' L') + 'Z';
    function claw(x0, y0, x1, y1) {
      return '<path d="M' + x0 + ' ' + y0 + ' Q' + (x0 + x1) / 2 + ' ' + (y1 + 4) + ' ' + x1 + ' ' + y1 + '" fill="none" stroke="' + mix(d, -.6) + '" stroke-width="3.4" stroke-linecap="round"/>' +
        '<path d="M' + x0 + ' ' + y0 + ' Q' + (x0 + x1) / 2 + ' ' + (y1 + 4) + ' ' + x1 + ' ' + y1 + '" fill="none" stroke="' + mix(d, .2) + '" stroke-width="2.4" stroke-linecap="round"/>' +
        '<circle cx="' + (x1 - .4) + '" cy="' + (y1 - .4) + '" r=".7" fill="#fff" opacity=".85"/>';
    }
    return defs + shadow(60, 30, b) +
      '<ellipse cx="60" cy="98" rx="18" ry="5" fill="' + mix(b, .5) + '" opacity=".35" filter="' + h.url('s4') + '"/>' +
      /* the band's far side, seen from inside */
      '<ellipse cx="60" cy="82" rx="32" ry="18" fill="none" stroke="' + h.url('fr') + '" stroke-width="8.4"/>' +
      '<path d="M30.6 83 A29.4 15 0 0 1 89.4 83" fill="none" stroke="' + h.url('bk') + '" stroke-width="3.4"/>' +
      '<path d="M33 79.5 A27.4 13.6 0 0 1 87 79.5" fill="none" stroke="#fff" stroke-opacity=".35" stroke-width="1" filter="' + h.url('s1') + '"/>' +
      /* the collet and the stone's pavilion inside it */
      '<path d="M46 ' + (G + 2) + ' L74 ' + (G + 2) + ' Q66 54 66 66 L54 66 Q54 54 46 ' + (G + 2) + 'Z" fill="' + h.url('cl') + '"/>' +
      '<path d="M46 ' + (G + 2) + ' L74 ' + (G + 2) + ' Q66 54 66 66 L54 66 Q54 54 46 ' + (G + 2) + 'Z" fill="' + t + '" opacity=".25" filter="' + h.url('mt') + '"/>' +
      '<path d="M48.4 ' + (G + 5) + ' L71.6 ' + (G + 5) + '" stroke="rgb(16 10 6 / .35)" stroke-width=".8"/><path d="M48.8 ' + (G + 6) + ' L71.2 ' + (G + 6) + '" stroke="#fff" stroke-opacity=".4" stroke-width=".5"/>' +
      '<g opacity=".95">' + pav + '</g>' +
      /* the band's near side over the collet's foot */
      '<path d="M33.5 92 A32 18 0 0 0 86.5 92" fill="none" stroke="rgb(16 10 6 / .35)" stroke-width="6" transform="translate(0 2)" filter="' + h.url('s1') + '"/>' +
      '<path d="M28 82 A32 18 0 0 0 92 82" fill="none" stroke="' + h.url('fr') + '" stroke-width="8.4"/>' +
      '<path d="M28 82 A32 18 0 0 0 92 82" fill="none" stroke="' + t + '" stroke-width="8.4" opacity=".25" filter="' + h.url('mt') + '"/>' +
      '<path d="M30 80 A30.6 16.2 0 0 0 72 96.6" fill="none" stroke="#fff" stroke-opacity=".85" stroke-width="1.3" stroke-linecap="round" filter="' + h.url('s1') + '"/>' +
      '<path d="M26.2 83 A33.8 19.6 0 0 0 93.8 83" fill="none" stroke="rgb(16 10 6 / .45)" stroke-width=".7"/>' +
      '<path d="M66 96 A30 15 0 0 0 86 88" fill="none" stroke="#ffd9a8" stroke-opacity=".55" stroke-width="1.2" stroke-linecap="round" filter="' + h.url('s1') + '"/>' +
      /* the stone's crown, its table and claws */
      '<ellipse cx="60" cy="' + G + '" rx="18.4" ry="5.8" fill="' + mix(b, -.5) + '"/>' + crown +
      '<path d="' + table + '" fill="' + h.url('tb') + '" stroke="' + mix(b, .6) + '" stroke-opacity=".7" stroke-width=".4"/>' +
      '<path d="M52 30 L58 29 L56 33Z" fill="#fff" opacity=".55"/>' +
      claw(45, 48, 43.4, 39) + claw(75, 48, 76.6, 39) + claw(53, 50, 51.4, 44.6) + claw(67, 50, 68.6, 44.6) +
      /* fire: the light split into colours, and the brightest glints */
      '<circle cx="66" cy="36" r=".9" fill="#ff7a6a" opacity=".85"/><circle cx="55" cy="42" r=".8" fill="#7affd8" opacity=".75"/><circle cx="70" cy="41" r=".7" fill="#ffe27a" opacity=".85"/>' +
      '<path d="M50 26 l1.1 3.3 3.3 1.1 -3.3 1.1 -1.1 3.3 -1.1 -3.3 -3.3 -1.1 3.3 -1.1z" fill="#fff" filter="' + h.url('s1') + '"/><path d="M50 27.6 l.6 1.8 1.8 .6 -1.8 .6 -.6 1.8 -.6 -1.8 -1.8 -.6 1.8 -.6z" fill="#fff"/>' +
      '<path d="M74 34 l.7 2 2 .7 -2 .7 -.7 2 -.7 -2 -2 -.7 2 -.7z" fill="#fff" opacity=".9"/>';
  };

  /* Anything else: a cloth bundle tied with cord. The cloth swells from
     its outline under the light, shows its weave up close and creases
     where the cord gathers it; the tag carries the mark. */
  SH.bundle = function (s) {
    var b = s.body, t = s.trim;
    var sack = 'M24 100 Q18 74 32 58 Q44 48 51 47 L69 47 Q76 48 88 58 Q102 74 96 100 Q60 109 24 100Z';
    var ears = 'M50 49 Q44 42 44.6 34 Q48 36 50 30 Q53.6 35 57 28.6 Q60.4 34 63.4 28.6 Q66.4 35 70 30 Q72 36 75.4 34 Q76 42 70 49Z';
    var defs = '<defs>' + shadowDefs() + tagDefs() +
      puffF('pf1', 12, 2.6, '.05', 41) + puffF('pf2', 4, 5, '.06', 43) +
      '<filter id="' + h.id('wv') + '" x="0" y="0" width="1" height="1"><feTurbulence type="fractalNoise" baseFrequency="1.6 .03" numOctaves="1" seed="2" result="a"/>' +
      '<feTurbulence type="fractalNoise" baseFrequency=".03 1.6" numOctaves="1" seed="3" result="b"/><feComposite in="a" in2="b" operator="arithmetic" k2=".5" k3=".5"/>' +
      '<feColorMatrix type="matrix" values="0 0 0 0 0  0 0 0 0 0  0 0 0 0 0  0 0 0 -5 2.6"/><feComposite in2="SourceGraphic" operator="in"/></filter>' +
      h.rgrad('cl', .36, .3, .9, [[0, mix(b, .38)], [.35, mix(b, .1)], [.7, mix(b, -.3)], [1, mix(b, -.62)]]) +
      h.rgrad('ec', .38, .4, .8, [[0, mix(b, .3)], [.6, mix(b, -.1)], [1, mix(b, -.55)]]) +
      '<clipPath id="' + h.id('sk') + '"><path d="' + sack + '"/></clipPath></defs>';
    function crease(p, w) {
      return '<path d="' + p + '" fill="none" stroke="rgb(16 10 6 / .45)" stroke-width="' + w + '" filter="' + h.url('s1') + '"/>' +
        '<path d="' + p + '" fill="none" stroke="#fff" stroke-opacity=".22" stroke-width="' + (w * .7) + '" transform="translate(-1.2 -.6)" filter="' + h.url('s1') + '"/>';
    }
    return defs + shadow(60, 36, null) +
      '<path d="' + sack + '" fill="' + h.url('cl') + '" filter="' + h.url('pf1') + '"/>' +
      '<g clip-path="' + h.url('sk') + '"><rect x="16" y="44" width="88" height="66" fill="#000" opacity=".13" filter="' + h.url('wv') + '"/>' +
      crease('M53 50 Q48 66 40 80', 1.4) + crease('M60 51 Q61 70 58 92', 1.2) + crease('M67 50 Q74 64 82 74', 1.4) + crease('M46 52 Q36 60 30 70', 1) +
      '<path d="' + sack + '" fill="none" stroke="rgb(16 10 6 / .35)" stroke-width="6" filter="' + h.url('s2') + '"/></g>' +
      '<path d="' + ears + '" fill="' + h.url('ec') + '" filter="' + h.url('pf2') + '"/>' +
      crease('M53 46 Q52 38 50 32', .8) + crease('M60 46 V32', .7) + crease('M67 46 Q68 38 70 32', .8) +
      /* the cord, wound twice and knotted, trailing to the tag */
      '<path d="M48 48.5 Q60 54 72 48.5" fill="none" stroke="rgb(16 10 6 / .4)" stroke-width="5.4" transform="translate(.4 1.4)" filter="' + h.url('s1') + '"/>' +
      '<path d="M48 48.5 Q60 54 72 48.5" fill="none" stroke="' + mix(t, -.6) + '" stroke-width="3.8" stroke-linecap="round"/>' +
      '<path d="M48 48.5 Q60 54 72 48.5" fill="none" stroke="' + mix(t, .3) + '" stroke-width="2.6" stroke-dasharray="1.5 .9"/>' +
      '<path d="M48.6 46.6 Q60 51.6 71.4 46.6" fill="none" stroke="#fff" stroke-opacity=".3" stroke-width=".7"/>' +
      '<circle cx="71" cy="49" r="3" fill="' + mix(t, -.2) + '"/><circle cx="70.2" cy="48.2" r="1.1" fill="' + mix(t, .45) + '"/>' +
      tag(s, 82, 60, 10, 'M72 50 Q80 53 83 70');
  };

  R.addStyle('real', SH);
  R._real = {blend: blend, grainF: grainF, puffF: puffF, metalF: metalF, shadow: shadow, shadowDefs: shadowDefs, tag: tag, tagDefs: tagDefs, corkDefs: corkDefs};
})();
