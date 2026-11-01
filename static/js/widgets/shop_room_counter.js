/**
 * shop_room_counter.js — the shop room's counter look: the shop seen from
 * the customer's side of the counter, the way the crafting desk shows a desk.
 * The keeper stands behind the counter as a token, lit from below by the oil
 * lamp, with shelves of goods behind; turn round and the shop floor is behind
 * you (the door, a glass cabinet, barrels, the room's own working piece), and
 * the keeper's finer goods wait in a case on the counter.
 *
 * The shop room widget (static/js/widgets/shop_room.js) calls
 * ShopRoomCounter.render(host, model, hooks) after each draw when the shop's
 * look is "counter". The widget keeps the basket, buying and saving; this file
 * draws, lays the goods out (layout), and asks the widget through hooks to
 * put things in the basket or take them out.
 *
 * One lit SVG scene per view, 1200 by 700. Lighting filters are baked once
 * into images, so per-pixel light costs nothing per frame. No filter uses a
 * point light, and the lamp's falloff is a see-through dark gradient rather
 * than a blend, because some browsers get both wrong (see shade and dist).
 * Layers marked data-depth move by their own share of the camera, which gives
 * stepping back and walking closer their depth.
 */
(function () {
  'use strict';

  var NS = 'http://www.w3.org/2000/svg', W = 1200, H = 700;

  function rng(s) { s = s % 2147483647 || 1; return function () { s = (s * 16807) % 2147483647; return (s - 1) / 2147483646; }; }
  function esc(t) { return String(t == null ? '' : t).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;'); }
  function stops(a) { return a.map(function (s) { return '<stop offset="' + s[0] + '" stop-color="' + s[1] + '"' + (s[2] != null ? ' stop-opacity="' + s[2] + '"' : '') + '/>'; }).join(''); }
  function lin(id, v, a, o) { return '<linearGradient id="' + id + '" x1="0" y1="0" x2="' + (v ? 0 : 1) + '" y2="' + (v ? 1 : 0) + '"' + (o || '') + '>' + stops(a) + '</linearGradient>'; }
  function rad(id, a, o) { return '<radialGradient id="' + id + '"' + (o || '') + '>' + stops(a) + '</radialGradient>'; }
  function hex(c) { c = String(c || '#888888').replace('#', ''); return [parseInt(c.slice(0, 2), 16) || 0, parseInt(c.slice(2, 4), 16) || 0, parseInt(c.slice(4, 6), 16) || 0]; }
  function mix(a, b, t) { var A = hex(a), B = hex(b); return '#' + A.map(function (v, i) { var c = Math.round(v + (B[i] - v) * t); return (c < 16 ? '0' : '') + c.toString(16); }).join(''); }
  function f1(v) { return (Math.round(v * 10) / 10).toString(); }
  function safeHex(c) { return /^#[0-9a-f]{6}$/i.test(c || '') ? c : '#8a8e96'; }

  /* The lamp's falloff as a plain see-through darkening, not a multiply
     blend: some browsers blend only within a layer's own group and wash the
     whole scene out white. Each stop's darkness is how much a multiply by
     that colour would take away. */
  function shade(a) {
    return a.map(function (s) { var c = parseInt(s[1].slice(1), 16), l = ((c >> 16 & 255) + (c >> 8 & 255) + (c & 255)) / 765; return [s[0], '#0b0e20', +(1 - l).toFixed(3)]; });
  }
  /* Wood: grain from stretched noise, coloured between a dark and a light
     tone, with pores. Literal colours only, because a baked layer is drawn
     as a standalone image that cannot see the page's CSS. */
  function woodF(id, seed, fx, fy, c0, c1) {
    var d = [c1[0] - c0[0], c1[1] - c0[1], c1[2] - c0[2]];
    return '<filter id="' + id + '" x="0" y="0" width="1" height="1" color-interpolation-filters="sRGB">' +
      '<feTurbulence type="fractalNoise" baseFrequency="' + fx + ' ' + fy + '" numOctaves="3" seed="' + seed + '" result="t"/>' +
      '<feComponentTransfer in="t" result="r"><feFuncR type="table" tableValues="0 .5 .28 .7 .42 .88 .5 .8 .34 .62 1"/></feComponentTransfer>' +
      '<feColorMatrix in="r" type="matrix" values="' + d[0] + ' 0 0 0 ' + c0[0] + '  ' + d[1] + ' 0 0 0 ' + c0[1] + '  ' + d[2] + ' 0 0 0 ' + c0[2] + '  0 0 0 0 1" result="c"/>' +
      '<feTurbulence type="fractalNoise" baseFrequency="' + (fx * 14).toFixed(3) + ' ' + (fy * 6).toFixed(3) + '" numOctaves="2" seed="' + (seed + 5) + '" result="p"/>' +
      '<feColorMatrix in="p" type="matrix" values="0 0 0 0 .05  0 0 0 0 .025  0 0 0 0 .01  0 0 0 -6 2.25" result="pores"/>' +
      '<feComposite in="pores" in2="c" operator="over" result="w"/><feComposite in="w" in2="SourceAlpha" operator="in"/></filter>';
  }
  /* A surface lit per pixel: a bump from noise, lit by each lamp, plus the
     room's ambient; the light multiplies the flat paint under it. No point
     lights: some phone GPUs work their distances out in half precision and
     the surface comes out black. Each lamp is a distant light at the angle it
     would have at the box's centre, and the scene's shade adds the falloff. */
  function dist(box, x, y, z) {
    var cx = box[0] + box[2] / 2, cy = box[1] + box[3] / 2, d = Math.max(1, Math.hypot(x - cx, y - cy));
    return '<feDistantLight azimuth="' + (Math.atan2(y - cy, x - cx) * 180 / Math.PI).toFixed(1) + '" elevation="' + (Math.atan2(z, d) * 180 / Math.PI).toFixed(1) + '"/>';
  }
  function litF(id, box, fq, sc, seed, lights, amb, k3, spec) {
    var s = '<filter id="' + id + '" filterUnits="userSpaceOnUse" x="' + box[0] + '" y="' + box[1] + '" width="' + box[2] + '" height="' + box[3] + '" color-interpolation-filters="sRGB">' +
      '<feTurbulence type="fractalNoise" baseFrequency="' + fq + '" numOctaves="4" seed="' + seed + '" result="n"/>' +
      '<feTurbulence type="fractalNoise" baseFrequency=".0045" numOctaves="3" seed="' + (seed + 3) + '" result="m"/>' +
      '<feComposite in="n" in2="m" operator="arithmetic" k2=".3" k3=".9" k4="-.1" result="h"/>';
    lights.forEach(function (L, i) {
      s += '<feDiffuseLighting in="h" surfaceScale="' + sc + '" diffuseConstant="' + (L.k || 1) + '" lighting-color="' + L.col + '" result="d' + i + '">' + dist(box, L.x, L.y, L.z) + '</feDiffuseLighting>';
      s += i ? '<feComposite in="d' + i + '" in2="D' + (i - 1) + '" operator="arithmetic" k2="1" k3="1" result="D' + i + '"/>' : '<feComposite in="d0" in2="d0" operator="arithmetic" k2="1" result="D0"/>';
    });
    s += '<feFlood flood-color="' + amb + '" result="amb"/><feComposite in="D' + (lights.length - 1) + '" in2="amb" operator="arithmetic" k2="1" k3="' + k3 + '" result="L"/>' +
      '<feComposite in="SourceGraphic" in2="L" operator="arithmetic" k1="1.2" result="lit"/>';
    if (spec) {
      var L0 = lights[0];
      s += '<feSpecularLighting in="h" surfaceScale="3" specularConstant="' + spec + '" specularExponent="22" lighting-color="' + L0.col + '" result="s">' + dist(box, L0.x, L0.y + 120, 160) + '</feSpecularLighting>' +
        '<feComposite in="s" in2="SourceAlpha" operator="in" result="s2"/><feComposite in="lit" in2="s2" operator="arithmetic" k2="1" k3=".5" result="lit"/>';
    }
    return s + '<feComposite in="lit" in2="SourceAlpha" operator="in"/></filter>';
  }
  /* Filters and gradients every view uses: blurs, soft shadows, glass, brass
     and iron, the lamp's flame and glow. */
  function common() {
    return '<filter id="src-blur14" x="-20%" y="-20%" width="140%" height="140%"><feGaussianBlur stdDeviation="14"/></filter>' +
      '<filter id="src-blur8" x="-30%" y="-30%" width="160%" height="160%"><feGaussianBlur stdDeviation="8"/></filter>' +
      '<filter id="src-blur5" x="-30%" y="-60%" width="160%" height="220%"><feGaussianBlur stdDeviation="5"/></filter>' +
      '<filter id="src-blur3" x="-30%" y="-30%" width="160%" height="160%"><feGaussianBlur stdDeviation="3"/></filter>' +
      '<filter id="src-dof" x="-2%" y="-2%" width="104%" height="104%"><feGaussianBlur stdDeviation=".8"/></filter>' +
      '<filter id="src-paper" x="0" y="0" width="1" height="1"><feTurbulence type="fractalNoise" baseFrequency=".09 .5" numOctaves="3" seed="5"/>' +
        '<feColorMatrix type="matrix" values="0 0 0 0 .35  0 0 0 0 .25  0 0 0 0 .12  0 0 0 -3 1.75"/><feComposite in2="SourceGraphic" operator="in"/></filter>' +
      '<filter id="src-cloth" x="0" y="0" width="1" height="1"><feTurbulence type="turbulence" baseFrequency=".5 .06" numOctaves="2" seed="4"/>' +
        '<feColorMatrix type="matrix" values="0 0 0 0 0  0 0 0 0 0  0 0 0 0 0  0 0 0 -3 1.4"/><feComposite in2="SourceGraphic" operator="in"/></filter>' +
      '<filter id="src-velvet" x="0" y="0" width="1" height="1"><feTurbulence type="fractalNoise" baseFrequency=".8" numOctaves="2" seed="9"/>' +
        '<feColorMatrix type="matrix" values="0 0 0 0 1  0 0 0 0 .8  0 0 0 0 .8  0 0 0 -7 3.2"/><feComposite in2="SourceGraphic" operator="in"/></filter>' +
      lin('src-brass', 0, [[0, '#24180a'], [.1, '#6a5a3a'], [.22, '#3b2a11'], [.48, '#8e6a2b'], [.7, '#e6c178'], [.78, '#fff0c8'], [.86, '#bd8b3a'], [1, '#2e200c']]) +
      lin('src-brassv', 1, [[0, '#f6dc9c'], [.3, '#b88b3e'], [.75, '#5e4318'], [1, '#2e200c']]) +
      lin('src-iron', 0, [[0, '#0e0d0c'], [.4, '#2a2724'], [.7, '#4a4540'], [.85, '#6a6258'], [1, '#1a1816']]) +
      lin('src-steel', 0, [[0, '#3a3e44'], [.35, '#9aa0a8'], [.5, '#e8ecf0'], [.65, '#8a9098'], [1, '#2a2e34']]) +
      lin('src-frame', 0, [[0, '#24170c'], [.5, '#3a2716'], [.85, '#5a3d22'], [1, '#3a2716']]) +
      '<radialGradient id="src-flame" cx=".5" cy=".75" r=".7"><stop offset="0" stop-color="#fffbe6"/><stop offset=".35" stop-color="#ffd27a"/><stop offset=".8" stop-color="#ff8a2a"/><stop offset="1" stop-color="#ff5a1a" stop-opacity="0"/></radialGradient>' +
      rad('src-glow', [[0, '#ffc478', .9], [.4, '#ffc478', .25], [1, '#ffc478', 0]]) +
      rad('src-bloom', [[0, '#fffbea', .95], [.14, '#ffc478', .55], [.45, '#ffc478', .1], [1, '#ffc478', 0]]) +
      rad('src-ao', [[0, '#000', .85], [.55, '#000', .4], [1, '#000', 0]]) +
      lin('src-cast', 0, [[0, '#000', .6], [.45, '#000', .28], [1, '#000', 0]]) +
      lin('src-fog', 1, [[0, '#dfe8ff', 0], [1, '#dfe8ff', .16]]) +
      lin('src-lampglass', 0, [[0, '#ffe8b0', .35], [.3, '#fff6dc', .9], [.7, '#ffd890', .7], [1, '#a86a2a', .4]]) +
      lin('src-tag', 1, [[0, '#f1e6cc'], [1, '#d8c8a2']]) + lin('src-wallfoot', 1, [[0, '#6a5440'], [.3, '#2a1e14'], [1, '#120c08']]) +
      wareDefs();
  }

  /* ---- Laying the goods out ----
     Every good is drawn by its family, chosen from its icon, so a shop's own
     goods look like what they are without anyone drawing them. */
  var FAMILY = {
    flask: 'bottle', 'kit-medical': 'box', vial: 'tall', 'bottle-droplet': 'tall', 'beer-mug': 'jar',
    jar: 'jar', 'jar-wheat': 'jar', 'mortar-pestle': 'jar', candy: 'jar', lollipop: 'jar', cupcake: 'jar', cheese: 'jar', 'apple-whole': 'jar', bucket: 'jar', 'bread-slice': 'jar', 'oil-can': 'jar',
    box: 'box', 'boxes-stacked': 'box', gift: 'box', 'bag-shopping': 'box', tent: 'box',
    book: 'book', 'book-open': 'book', 'eye-slash': 'book', scroll: 'scroll', feather: 'scroll',
    leaf: 'bundle', seedling: 'bundle', 'wheat-awn': 'bundle', spa: 'bundle', fish: 'bundle',
    bandage: 'cloth', shirt: 'cloth', vest: 'cloth', mask: 'cloth',
    sword: 'blade', dagger: 'blade', axe: 'blade', bow: 'blade', hammer: 'blade', 'screwdriver-wrench': 'blade', broom: 'blade', 'wand-sparkles': 'blade',
    'shield-halved': 'shield',
    ring: 'trinket', gem: 'trinket', crown: 'trinket', eye: 'trinket', key: 'trinket', compass: 'trinket', star: 'trinket', moon: 'trinket', sun: 'trinket', shell: 'trinket',
    anchor: 'trinket', butterfly: 'trinket', skull: 'trinket', spider: 'trinket', ghost: 'trinket', web: 'trinket', tentacle: 'trinket', snowflake: 'trinket', hourglass: 'trinket',
    bell: 'trinket', 'scale-balanced': 'trinket', binoculars: 'trinket', handcuffs: 'trinket', link: 'trinket', lightbulb: 'trinket', fire: 'trinket'
  };
  function family(ic) { return FAMILY[ic] || 'crate'; }
  // Families too big for a shelf spot stand out on the shop floor.
  var BIG = { box: 1, blade: 1, shield: 1, cloth: 1, crate: 1 };
  var CAP = { shelf: 12, case: 8, cabinet: 6, barrel: 2 };

  /* Where each good goes, from the goods alone, so the room lays itself out
     and a new good finds a place without anyone arranging it. The case holds
     the dearest goods (twice the median price or more) once a shop has five
     or more; barrels take bulk goods (no stock limit, or ten or more) that are
     bundles or jars; the cabinet takes the bigger things at or above the
     median; the rest go on the shelves behind the keeper. Whatever does not
     fit is counted in more, and the scene says so: nothing is hidden. */
  function layout(its) {
    var out = { shelf: [], case: [], cabinet: [], barrel: [], more: [] };
    var prices = its.map(function (it) { return +it.p || 0; }).sort(function (a, b) { return a - b; });
    var med = prices.length ? prices[Math.floor((prices.length - 1) / 2)] : 0;
    var left = [];
    its.forEach(function (it) {
      var p = +it.p || 0, fam = family(it.ic), bulk = it.qty === null || it.qty >= 10;
      if (its.length >= 5 && med > 0 && p >= med * 2 && out.case.length < CAP.case) out.case.push(it);
      else if (bulk && (fam === 'bundle' || fam === 'jar') && out.barrel.length < CAP.barrel) out.barrel.push(it);
      else if (BIG[fam] && p >= med && out.cabinet.length < CAP.cabinet) out.cabinet.push(it);
      else left.push(it);
    });
    left.forEach(function (it) {
      if (out.shelf.length < CAP.shelf) out.shelf.push(it);
      else if (out.cabinet.length < CAP.cabinet) out.cabinet.push(it);
      else if (out.case.length < CAP.case && its.length >= 5) out.case.push(it);
      else out.more.push(it);
    });
    return out;
  }
  // The room's own working piece on the shop floor, by shop type.
  function floorPiece(roomType) { return { apothecary: 'still', library: 'still', forge: 'anvil' }[roomType] || 'crates'; }

  /* ---- The goods, drawn. Each stands front-on with its bottom centre at
     0,0, about 60 units tall at scale 1, so a view can stand it on a shelf,
     lay it in a case or bring it to the counter. Glass catches the lamp on
     the side it faces: side is 'L' or 'R'. c is the good's own colour. ---- */
  function wareDefs() {
    var s = '';
    ['L', 'R'].forEach(function (sd) {
      var r = sd === 'R';
      s += lin('src-glass' + sd, 0, r ? [[0, '#04070a', .55], [.14, '#04070a', .2], [.36, '#fff', 0], [.66, '#fff', .06], [.8, '#ffe6c4', .34], [.9, '#04070a', .18], [1, '#04070a', .6]] : [[0, '#04070a', .6], [.1, '#04070a', .18], [.2, '#ffe6c4', .34], [.34, '#fff', .06], [.64, '#fff', 0], [.86, '#04070a', .2], [1, '#04070a', .55]]);
      s += rad('src-ball' + sd, [[0, '#fff', .55], [.18, '#fff', .12], [.55, '#000', 0], [1, '#000', .55]], ' cx="' + (r ? .68 : .32) + '" cy=".3" r=".75"');
      s += lin('src-side' + sd, 0, r ? [[0, '#000', .45], [.3, '#000', .1], [.7, '#fff', .08], [.86, '#ffd9a8', .2], [1, '#000', .4]] : [[0, '#000', .4], [.14, '#ffd9a8', .2], [.3, '#fff', .08], [.7, '#000', .1], [1, '#000', .45]]);
    });
    return s + lin('src-cork', 0, [[0, '#3a2412'], [.5, '#9a6a3a'], [.8, '#c89a62'], [1, '#5a3a1a']]) + lin('src-wood', 0, [[0, '#3a2212'], [.4, '#7a4a24'], [.8, '#9a6434'], [1, '#4a2a14']]);
  }
  // A good's icon, its height h, centred on x with its foot at y.
  function iconAt(ic, icons, x, y, h, fill, extra) {
    var g = icons && (icons[ic] || icons.box); if (!g) return '';
    var w = h * g[0] / 512;
    return '<svg x="' + f1(x - w / 2) + '" y="' + f1(y - h) + '" width="' + f1(w) + '" height="' + f1(h) + '" viewBox="0 0 ' + g[0] + ' 512" overflow="visible"' + (extra || '') + '><path d="' + g[1] + '" fill="' + fill + '"/></svg>';
  }
  function label(ic, icons, x, y, w, h) {
    return '<rect x="' + f1(x - w / 2) + '" y="' + f1(y - h) + '" width="' + w + '" height="' + h + '" rx="1.5" fill="#e8dcbc" opacity=".94"/>' + iconAt(ic, icons, x, y - h * .18, h * .64, '#3a2a1a');
  }
  var ART = {
    bottle: function (c, sd, ic, I) {
      return '<ellipse cx="0" cy="-1" rx="22" ry="4" fill="#000" opacity=".4"/><circle cx="0" cy="-22" r="21" fill="' + mix(c, '#000', .55) + '"/>' +
        '<path d="M-20,-24 a20,20 0 0 0 40,0 Z" fill="' + c + '"/><ellipse cx="0" cy="-24" rx="20" ry="3" fill="' + mix(c, '#fff', .35) + '" opacity=".7"/>' +
        '<circle cx="0" cy="-22" r="21" fill="url(#src-ball' + sd + ')"/><rect x="-6" y="-58" width="12" height="20" fill="url(#src-glass' + sd + ')"/><rect x="-6.5" y="-66" width="13" height="10" rx="2" fill="url(#src-cork)"/>' +
        label(ic, I, 0, -14, 16, 12) + '<circle cx="' + (sd === 'R' ? 9 : -9) + '" cy="-32" r="3.2" fill="#fff" opacity=".7"/>';
    },
    tall: function (c, sd, ic, I) {
      return '<ellipse cx="0" cy="-1" rx="12" ry="3" fill="#000" opacity=".4"/><rect x="-9" y="-50" width="18" height="49" rx="5" fill="' + mix(c, '#000', .6) + '"/>' +
        '<rect x="-8" y="-32" width="16" height="30" rx="4" fill="' + c + '" opacity=".85"/><rect x="-9" y="-50" width="18" height="49" rx="5" fill="url(#src-glass' + sd + ')"/>' +
        '<rect x="-5" y="-60" width="10" height="11" fill="url(#src-glass' + sd + ')"/><path d="M-7,-62 h14 v6 c-2,3 -12,3 -14,0 Z" fill="#8a1a14"/>' + label(ic, I, 0, -14, 14, 12);
    },
    jar: function (c, sd, ic, I) {
      return '<ellipse cx="0" cy="-1" rx="17" ry="3.6" fill="#000" opacity=".4"/><path d="M-15,-2 v-26 q0,-4 4,-5 h22 q4,1 4,5 v26 Z" fill="' + mix(c, '#000', .45) + '"/>' +
        '<path d="M-14,-3 v-16 h28 v16 Z" fill="' + c + '" opacity=".6"/><path d="M-15,-2 v-26 q0,-4 4,-5 h22 q4,1 4,5 v26 Z" fill="url(#src-glass' + sd + ')"/>' +
        '<path d="M-16,-35 q16,-12 32,0 l-3,5 h-26 Z" fill="#c8b8a0"/><path d="M-16,-35 q16,-12 32,0 l-3,5 h-26 Z" fill="#000" opacity=".15" filter="url(#src-cloth)"/>' +
        '<path d="M-14,-31 h28" stroke="#6a1a14" stroke-width="2.4"/>' + label(ic, I, 0, -8, 16, 13);
    },
    box: function (c, sd, ic, I) {
      var wd = mix('#6a4020', c, .3);
      return '<ellipse cx="0" cy="-1" rx="28" ry="4.4" fill="#000" opacity=".45"/><rect x="-26" y="-26" width="52" height="25" rx="2" fill="' + wd + '"/><rect x="-26" y="-26" width="52" height="25" rx="2" fill="url(#src-side' + sd + ')"/>' +
        '<path d="M-28,-26 q28,-14 56,0 Z" fill="' + mix(wd, '#fff', .12) + '"/><path d="M-26,-26 h52" stroke="#2a160a" stroke-width="1.6"/>' +
        '<rect x="-27" y="-27" width="5" height="26" fill="url(#src-brassv)"/><rect x="22" y="-27" width="5" height="26" fill="url(#src-brassv)"/>' +
        '<rect x="-9" y="-22" width="18" height="15" rx="2" fill="url(#src-brass)"/>' + iconAt(ic, I, 0, -9, 10, '#3a2a10');
    },
    book: function (c, sd, ic, I) {
      return '<ellipse cx="0" cy="-1" rx="20" ry="3.6" fill="#000" opacity=".4"/><rect x="-14" y="-52" width="30" height="51" rx="2" fill="#e8dcbc"/>' +
        '<rect x="-16" y="-54" width="28" height="53" rx="2" fill="' + mix(c, '#000', .35) + '"/><rect x="-16" y="-54" width="28" height="53" rx="2" fill="url(#src-side' + sd + ')"/>' +
        '<path d="M-16,-46 h28 M-16,-9 h28" stroke="#c8a060" stroke-width="1.6" opacity=".8"/>' + iconAt(ic, I, -2, -18, 18, '#e6c178');
    },
    scroll: function (c, sd, ic, I) {
      return '<ellipse cx="0" cy="-1" rx="24" ry="3.6" fill="#000" opacity=".4"/><rect x="-22" y="-16" width="44" height="14" rx="7" fill="#e8dcbc"/>' +
        '<rect x="-22" y="-16" width="44" height="14" rx="7" fill="#000" opacity=".1" filter="url(#src-paper)"/><ellipse cx="22" cy="-9" rx="4" ry="7" fill="#c8b898"/>' +
        '<path d="M-4,-17 v16" stroke="' + c + '" stroke-width="5"/><circle cx="-4" cy="-9" r="7" fill="#8a1a14"/>' + iconAt(ic, I, -4, -5, 8, '#e8b8a0');
    },
    bundle: function (c, sd, ic, I) {
      var s = '<ellipse cx="0" cy="-1" rx="20" ry="4" fill="#000" opacity=".4"/>';
      for (var i = 0; i < 11; i++) { var a = (i - 5) * .07; s += '<path d="M0,-4 q' + (Math.sin(a) * 30).toFixed(0) + ',-30 ' + (Math.sin(a) * 44).toFixed(0) + ',-56" stroke="' + mix(c, i % 2 ? '#000' : '#fff', .18) + '" stroke-width="2.4" fill="none"/>'; }
      return s + '<rect x="-7" y="-22" width="14" height="9" fill="#c8b898"/>' + iconAt(ic, I, 0, -13.5, 6, '#3a2a1a');
    },
    cloth: function (c, sd, ic, I) {
      var s = '<ellipse cx="0" cy="-1" rx="26" ry="4" fill="#000" opacity=".4"/>';
      [0, 1, 2].forEach(function (i) { var y = -2 - i * 10; s += '<rect x="-24" y="' + (y - 10) + '" width="48" height="10" rx="4" fill="' + mix(c, i % 2 ? '#000' : '#fff', .14) + '"/><rect x="-24" y="' + (y - 10) + '" width="48" height="10" rx="4" fill="#000" opacity=".12" filter="url(#src-cloth)"/>'; });
      return s + '<rect x="-24" y="-32" width="48" height="30" rx="4" fill="url(#src-side' + sd + ')"/><path d="M12,-28 l10,-6 v10 Z" fill="#e8dcbc"/>' + iconAt(ic, I, 17, -26, 6, '#3a2a1a');
    },
    blade: function (c, sd, ic, I) {
      return '<ellipse cx="0" cy="-1" rx="24" ry="4" fill="#000" opacity=".4"/><rect x="-20" y="-8" width="40" height="7" rx="1.5" fill="url(#src-wood)"/>' +
        iconAt(ic, I, 0, -6, 58, 'url(#src-steel)') + iconAt(ic, I, 0, -6, 58, c, ' opacity=".28"');
    },
    shield: function (c, sd, ic, I) {
      var d = 'M-22,-58 h44 v22 c0,18 -10,28 -22,34 c-12,-6 -22,-16 -22,-34 Z';
      return '<ellipse cx="0" cy="-1" rx="20" ry="3.6" fill="#000" opacity=".4"/><path d="' + d + '" fill="' + c + '"/><path d="' + d + '" fill="url(#src-side' + sd + ')"/>' +
        '<path d="' + d + '" fill="none" stroke="url(#src-brass)" stroke-width="3.4"/>' + iconAt(ic, I, 0, -20, 22, mix(c, '#fff', .7));
    },
    trinket: function (c, sd, ic, I) {
      return '<ellipse cx="0" cy="-1" rx="24" ry="4" fill="#000" opacity=".4"/><path d="M-24,-4 q-2,-12 8,-14 h32 q10,2 8,14 Z" fill="#5a1418"/>' +
        '<path d="M-24,-4 q-2,-12 8,-14 h32 q10,2 8,14 Z" fill="#fff" opacity=".08" filter="url(#src-velvet)"/>' +
        iconAt(ic, I, 0, -14, 30, c) + iconAt(ic, I, 0, -14, 30, 'url(#src-ball' + sd + ')') + '<circle cx="' + (sd === 'R' ? 9 : -9) + '" cy="-36" r="1.8" fill="#fff" opacity=".9"/>';
    },
    crate: function (c, sd, ic, I) {
      return '<ellipse cx="0" cy="-1" rx="26" ry="4.4" fill="#000" opacity=".45"/><rect x="-24" y="-40" width="48" height="39" fill="url(#src-wood)"/><rect x="-24" y="-40" width="48" height="39" fill="url(#src-side' + sd + ')"/>' +
        '<path d="M-24,-40 L24,-1 M-24,-1 L24,-40" stroke="#3a2212" stroke-width="3" opacity=".6"/><rect x="-24" y="-40" width="48" height="39" fill="none" stroke="#2a160a" stroke-width="3"/>' +
        label(ic, I, 0, -12, 22, 18) + '<rect x="-24" y="-40" width="48" height="4" fill="' + c + '" opacity=".5"/>';
    }
  };
  function ware(it, sd, icons) { return (ART[family(it.ic)] || ART.crate)(safeHex(it.c), sd === 'L' ? 'L' : 'R', it.ic, icons); }

  /* ---- Baking: each [data-bake] group is drawn once, filters and all, into
     an image at twice the scene's resolution and swapped in. If a browser
     refuses, the live drawing stays, only slower to repaint. Images are kept
     by their source, so turning back to a view already seen is instant. */
  var BAKED = {};
  function bake(svg, defs, done) {
    var groups = svg.querySelectorAll('[data-bake]'), left = groups.length;
    function one() { if (!--left && done) done(); }
    if (!left && done) done();
    Array.prototype.forEach.call(groups, function (g) {
      var bb; try { bb = g.getBBox(); } catch (e) { bb = null; }
      if (!bb || !bb.width) { one(); return; }
      var p = 24, x = Math.floor(bb.x - p), y = Math.floor(bb.y - p), w = Math.ceil(bb.width + 2 * p), h = Math.ceil(bb.height + 2 * p), k = 2;
      var src = '<svg xmlns="' + NS + '" viewBox="' + x + ' ' + y + ' ' + w + ' ' + h + '" width="' + w * k + '" height="' + h * k + '"><defs>' + defs + '</defs>' + g.innerHTML + '</svg>', img = new Image();
      function swap(u) {
        BAKED[src] = u; if (!g.isConnected) { one(); return; }
        var im = document.createElementNS(NS, 'image');
        [['href', u], ['x', x], ['y', y], ['width', w], ['height', h], ['preserveAspectRatio', 'none']].forEach(function (a) { im.setAttribute(a[0], a[1]); });
        im.addEventListener('load', function () { Array.prototype.slice.call(g.childNodes).forEach(function (n) { if (n !== im) g.removeChild(n); }); one(); });
        im.addEventListener('error', one);
        g.appendChild(im);
      }
      if (BAKED[src]) { swap(BAKED[src]); return; }
      img.onload = function () {
        try { var cv = document.createElement('canvas'); cv.width = w * k; cv.height = h * k; cv.getContext('2d').drawImage(img, 0, 0); cv.toBlob(function (b) { swap(b ? URL.createObjectURL(b) : img.src); }); }
        catch (e) { swap(img.src); }
      };
      img.onerror = one;
      img.src = 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(src);
    });
  }
  // Film grain over the frame, so the drawn shapes sit in one photograph.
  var GRAIN = null;
  function grain() {
    if (GRAIN === null) {
      try {
        var c = document.createElement('canvas'), n = 160; c.width = c.height = n;
        var x = c.getContext('2d'), im = x.createImageData(n, n);
        for (var i = 0; i < im.data.length; i += 4) { var v = 128 + (Math.random() - .5) * 120 | 0; im.data[i] = im.data[i + 1] = im.data[i + 2] = v; im.data[i + 3] = 255; }
        x.putImageData(im, 0, 0); GRAIN = c.toDataURL();
      } catch (e) { GRAIN = ''; }
    }
    return GRAIN;
  }

  /* The window's sky, from the room's time of day. TODO(#1839): the
     campaign calendar's own sky, as the header shows it. */
  var SKY = { '': ['#5a86c0', '#a8c4e0'], dawn: ['#3a4a7a', '#e8a888'], dusk: ['#2a2a5a', '#d87a5a'], night: ['#0b1430', '#2a3b66'] };
  function skyArt(id, x, y, w, h, time) {
    var c = SKY[time] || SKY[''], s = lin(id, 1, [[0, c[0]], [1, c[1]]]), R = rng(7), i;
    s = '<defs>' + s + '</defs><rect x="' + x + '" y="' + y + '" width="' + w + '" height="' + h + '" fill="url(#' + id + ')"/>';
    if (time === 'night') {
      for (i = 0; i < 16; i++) s += '<circle cx="' + f1(x + R() * w) + '" cy="' + f1(y + R() * h * .6) + '" r="' + f1(.6 + R() * .8) + '" fill="#fff" opacity="' + f1(.4 + R() * .5) + '"/>';
      s += '<circle cx="' + f1(x + w * .72) + '" cy="' + f1(y + h * .28) + '" r="' + f1(Math.min(w, h) * .12) + '" fill="#f2ecd6"/><circle cx="' + f1(x + w * .72) + '" cy="' + f1(y + h * .28) + '" r="' + f1(Math.min(w, h) * .3) + '" fill="#cfd8ff" opacity=".12"/>';
    } else if (time === '') s += '<circle cx="' + f1(x + w * .3) + '" cy="' + f1(y + h * .25) + '" r="' + f1(Math.min(w, h) * .2) + '" fill="#fff6dc" opacity=".55"/>';
    return s;
  }

  /* The counter's oil lamp, the shop's one real light; the case view draws
     the same lamp nearer. */
  function lampArt(x, y) {
    return '<g data-k="lamp"><ellipse cx="' + x + '" cy="' + (y + 2) + '" rx="40" ry="8" fill="url(#src-ao)"/>' +
      '<path d="M' + (x - 30) + ',' + y + ' q30,-12 60,0 l-6,-8 q-24,-10 -48,0 Z" fill="url(#src-brass)"/><path d="M' + (x - 20) + ',' + (y - 8) + ' q20,-30 40,0 Z" fill="url(#src-brass)"/>' +
      '<rect x="' + (x - 6) + '" y="' + (y - 30) + '" width="12" height="12" fill="url(#src-brass)"/><path d="M' + (x - 22) + ',' + (y - 30) + ' h44 l-4,-6 h-36 Z" fill="url(#src-brassv)"/>' +
      '<path d="M' + (x - 14) + ',' + (y - 36) + ' C' + (x - 26) + ',' + (y - 58) + ' ' + (x - 24) + ',' + (y - 80) + ' ' + (x - 10) + ',' + (y - 92) + ' L' + (x - 8) + ',' + (y - 122) + ' h16 L' + (x + 10) + ',' + (y - 92) + ' C' + (x + 24) + ',' + (y - 80) + ' ' + (x + 26) + ',' + (y - 58) + ' ' + (x + 14) + ',' + (y - 36) + ' Z" fill="url(#src-lampglass)"/>' +
      '<path class="src-flame" d="M' + x + ',' + (y - 86) + ' c-7,10 -8,20 0,26 c8,-6 7,-16 0,-26 Z" fill="url(#src-flame)"/>' +
      '<circle cx="' + x + '" cy="' + (y - 74) + '" r="90" fill="url(#src-bloom)" data-flick=".8"/></g>';
  }
  // A picture a page can point at: same-site or http(s), nothing else.
  function safeUrl(u) { u = String(u || ''); return /^(https?:\/\/|\/(?!\/))/i.test(u) ? u : ''; }
  // Up to two initials from the shop's name, for the case's monogram.
  function initials(name) { return String(name || '').split(/[\s'’-]+/).filter(function (w) { return /^[A-Za-zÀ-ɏ]/.test(w) && !/^(the|of|and|a)$/i.test(w); }).slice(0, 2).map(function (w) { return w.charAt(0).toUpperCase(); }).join('·'); }
  function ease(t) { return t < .5 ? 2 * t * t : 1 - Math.pow(-2 * t + 2, 2) / 2; }
  function short(t, n) { t = String(t || ''); return t.length > n ? t.slice(0, n - 1).trim() + '…' : t; }

  /* The counter look keeps its own dark lamplit palette in both site themes;
     the card, menu and hint over it are dark glass to match. */
  var CSS = [
    '.src-stage{position:absolute;inset:0;overflow:hidden;background:#120c08;user-select:none;-webkit-user-select:none;font-family:Inter,system-ui,-apple-system,"Segoe UI",sans-serif}',
    '.src-svg{display:block;width:100%;height:100%;touch-action:none;transition:transform .45s cubic-bezier(.5,0,.75,0),opacity .45s ease-in}',
    '.src-grain{position:absolute;inset:0;pointer-events:none;mix-blend-mode:overlay;opacity:.14;background-size:160px 160px}',
    '.src-stage::after{content:"Lighting the lamps…";position:absolute;inset:0;display:grid;place-items:center;background:#0a0705;color:rgb(255 226 180/.55);font:500 14px/1 Inter,system-ui,sans-serif;letter-spacing:.04em;opacity:0;pointer-events:none;transition:opacity .45s ease-out;z-index:4}',
    '.src-stage.src-veil::after{opacity:1;transition:none}',
    '.src-svg [data-k]{cursor:pointer;outline:none}',
    '.src-svg [data-k]:focus-visible{filter:drop-shadow(0 0 3px #ffd77a) drop-shadow(0 0 8px rgb(255 215 122/.8))}',
    '.src-svg .src-hov{filter:drop-shadow(0 0 7px rgb(255 214 120/.65)) brightness(1.12)}',
    '.src-svg .src-out{cursor:default}.src-dof{filter:url(#src-dof)}',
    // Light is paint, never a target: glows, blooms and the falloff let the pointer through.
    '.src-svg [data-flick],.src-svg .src-nohit{pointer-events:none}',
    '.src-hint{position:absolute;left:50%;bottom:14px;transform:translateX(-50%);display:flex;align-items:center;gap:10px;max-width:calc(100% - 24px);padding:7px 16px;border-radius:999px;font-size:13.5px;line-height:1.35;background:rgb(20 14 9/.8);color:#f3e6cc;border:1px solid rgb(228 194 122/.35);backdrop-filter:blur(6px);-webkit-backdrop-filter:blur(6px);box-shadow:0 8px 24px -8px rgb(0 0 0/.6);z-index:5}',
    '.src-hint[hidden]{display:none}.src-hint .src-acts{display:flex;gap:6px}',
    '.src-btn{white-space:nowrap;font:inherit;font-size:13px;font-weight:600;border-radius:999px;padding:5px 14px;cursor:pointer;border:1px solid rgb(228 194 122/.4);background:rgb(255 255 255/.08);color:#f3e6cc;height:auto;line-height:1.3}',
    '.src-btn:hover{background:rgb(255 255 255/.16)}.src-btn.primary{background:#e4b04a;color:#1d140c;border-color:#e4b04a}.src-btn.primary:hover{background:#ffd77a}',
    '.src-btn:focus-visible,.src-menu button:focus-visible{outline:2px solid #ffd28a;outline-offset:2px}',
    '.src-card{position:absolute;z-index:6;max-width:250px;padding:10px 12px;border-radius:10px;font-size:12.5px;line-height:1.4;pointer-events:none;opacity:0;transform:translateY(4px);transition:opacity .15s,transform .15s;display:flex;flex-direction:column;gap:3px;background:rgb(24 17 11/.92);color:#efe2c8;border:1px solid rgb(228 194 122/.4);box-shadow:0 14px 30px -10px rgb(0 0 0/.7)}',
    '.src-card.show{opacity:1;transform:none}.src-card.pinned{pointer-events:auto;padding-right:30px}',
    '.src-card b{font-family:"IM Fell English",Georgia,serif;font-size:17px;font-weight:400;color:#ffe2a8}',
    '.src-card .src-meta{font-size:11px;letter-spacing:.04em;color:#b9a27a}.src-card p{margin:0}.src-card .src-now{color:#ffd77a}.src-card .src-foot{font-size:11px;color:#a8977a;margin-top:2px}',
    '.src-x{position:absolute;top:4px;right:6px;background:none;border:0;font-size:18px;line-height:1;cursor:pointer;padding:2px 4px;color:#efe2c8;height:auto}',
    '.src-menu{position:absolute;z-index:7;min-width:190px;padding:6px;border-radius:10px;display:flex;flex-direction:column;background:rgb(24 17 11/.96);border:1px solid rgb(228 194 122/.4);box-shadow:0 14px 30px -10px rgb(0 0 0/.7)}',
    '.src-menu[hidden]{display:none}.src-mhead{font-size:11px;text-transform:uppercase;letter-spacing:.08em;padding:4px 10px 6px;color:#b9a27a}',
    '.src-menu button{font:inherit;font-size:13px;text-align:left;background:none;border:0;border-radius:6px;padding:7px 10px;cursor:pointer;color:#efe2c8;height:auto;line-height:1.3}',
    '.src-menu button:hover{background:rgb(228 194 122/.16)}.src-menu button[disabled]{opacity:.4;cursor:default;background:none}',
    '@keyframes src-pop{0%{opacity:0;transform:translateY(10px) scale(.8)}20%{opacity:1;transform:none}100%{opacity:0;transform:translateY(-26px)}}',
    '.src-pop{font-family:"IM Fell English",Georgia,serif;font-size:24px;fill:#ffd77a;transform-box:fill-box;transform-origin:50% 50%;animation:src-pop 1.2s cubic-bezier(.2,.7,.2,1) forwards;paint-order:stroke;stroke:#1a0f07;stroke-width:4px;pointer-events:none}',
    '@keyframes src-flick{0%{transform:scale(1,1) skewX(0)}35%{transform:scale(.96,1.08) skewX(-3deg)}70%{transform:scale(1.03,.95) skewX(2deg)}100%{transform:scale(.98,1.04) skewX(-1deg)}}',
    '.src-flame{transform-box:fill-box;transform-origin:50% 100%;animation:src-flick 1.3s ease-in-out infinite alternate}',
    '.src-tag{font-family:"IM Fell English",Georgia,serif;font-size:10px;fill:#3a2a1a}',
    '.src-hand{font-family:"IM Fell English",Georgia,serif;font-style:italic;font-size:11px;fill:#3a2a1a}.src-hand.small{font-size:9px}',
    '.src-more{font:600 13px Inter,system-ui,sans-serif;fill:#ffd77a}',
    '.src-heldware{cursor:grab}.src-heldware.drag{cursor:grabbing;filter:drop-shadow(0 14px 10px rgb(0 0 0/.5))}',
    // At the counter: the keeper breathes and nods, the bell rings, the scales tip.
    '@keyframes srcb-breathe{0%,100%{transform:scaleY(1)}50%{transform:scaleY(1.012)}}',
    '.srcb-body{transform-box:fill-box;transform-origin:50% 100%;animation:srcb-breathe 4.6s ease-in-out infinite}',
    '.srcb-head{transform-box:view-box;transform-origin:641px 340px}',
    '@keyframes srcb-look{0%{transform:none}30%,70%{transform:rotate(-6deg) translateX(-6px)}100%{transform:none}}.srcb-head.look{animation:srcb-look 1.1s cubic-bezier(.2,.7,.2,1)}',
    '@keyframes srcb-nod{0%,100%{transform:none}35%{transform:rotate(2deg) translateY(3px)}65%{transform:rotate(-1deg)}}.srcb-head.nod{animation:srcb-nod .9s ease-in-out}',
    '@keyframes src-ring{0%,100%{transform:rotate(0)}15%{transform:rotate(-14deg)}35%{transform:rotate(11deg)}55%{transform:rotate(-7deg)}75%{transform:rotate(4deg)}}',
    '.srcb-bell{transform-box:fill-box;transform-origin:50% 100%}.srcb-bell.ring{animation:src-ring .9s ease-out}',
    '.srcb-beam{transform-box:view-box;transform-origin:1046px 392px;transition:transform 1.2s cubic-bezier(.3,1.6,.4,1)}.srcb-beam.tip{transform:rotate(4deg)}',
    '.srcb-beam .srcb-panL,.srcb-beam .srcb-panR{transition:transform 1.2s cubic-bezier(.3,1.6,.4,1)}.srcb-beam.tip .srcb-panL{transform:translate(2px,-8px)}.srcb-beam.tip .srcb-panR{transform:translate(-2px,8px)}',
    '.src-svg .srcb-pileware.src-hov,.src-svg [data-k="case"].src-hov{filter:brightness(1.18)}',
    '.src-svg #srcb-pile.hot{filter:drop-shadow(0 0 10px rgb(255 210 160/.8))}',
    '.srcb-parcel{animation:srcb-parcel .5s cubic-bezier(.2,.7,.2,1)}@keyframes srcb-parcel{from{opacity:0;transform:translateY(-14px)}to{opacity:1;transform:none}}',
    // The shop floor and the case.
    '.srcf-chalk{font-family:"IM Fell English",Georgia,serif;font-size:14px;fill:#e8e4d8;opacity:.85}',
    '.srcf-bell{transform-box:fill-box;transform-origin:50% 0}.srcf-bell.ring{animation:src-ring .8s ease-in-out}',
    '.srcf-coals{animation:srcf-coal 2.2s ease-in-out infinite}@keyframes srcf-coal{0%,100%{opacity:.75}50%{opacity:1}}',
    '.srcc-lift{transition:transform .25s cubic-bezier(.2,.7,.2,1)}',
    // Moving between views.
    '.src-stage.src-leave .src-svg{transform:scale(1.5);opacity:0}',
    '.src-stage.src-arrive .src-svg{animation:src-arrive .5s cubic-bezier(.2,.7,.2,1)}@keyframes src-arrive{from{transform:scale(.92);opacity:0}to{transform:none;opacity:1}}',
    '.src-stage.src-fadein .src-svg{animation:src-fade .45s ease-out}@keyframes src-fade{from{opacity:0}to{opacity:1}}',
    // Ways out of a view: a tab on the edge, with its words on hover or focus.
    '.src-turn{position:absolute;top:50%;transform:translateY(-50%);z-index:6;display:flex;align-items:center;gap:8px;border:1px solid rgb(255 220 170/.28);background:rgb(20 14 8/.72);color:#f3e6cf;font:500 13px Inter,system-ui,sans-serif;padding:14px 10px;border-radius:10px;cursor:pointer;backdrop-filter:blur(4px);height:auto;line-height:1}',
    '.src-turn span{font-size:28px;line-height:1}.src-turn em{font-style:normal;max-width:0;overflow:hidden;white-space:nowrap;transition:max-width .25s ease}',
    '.src-turn:hover em,.src-turn:focus-visible em{max-width:200px}.src-turn:focus-visible{outline:2px solid #ffd28a;outline-offset:2px}',
    '.src-turn-left{left:max(10px,env(safe-area-inset-left,0px))}.src-turn-right{right:max(10px,env(safe-area-inset-right,0px));flex-direction:row-reverse}',
    '.src-turn-back{left:max(10px,env(safe-area-inset-left,0px));top:14px;transform:none;padding:8px 12px}.src-turn-back span{font-size:20px}.src-turn-back em{max-width:200px}',
    '.src-stepback[hidden]{display:none}.src-stage.src-close .src-turn-left,.src-stage.src-close .src-turn-right{display:none}',
    // The widget's basket lights up while a good is dragged over it.
    '.src-hot{box-shadow:0 0 0 2px #e4b04a,0 0 24px rgb(255 200 110/.5)}',
    '@media (max-width:640px){.src-hint{left:6px;right:6px;bottom:6px;transform:none;border-radius:10px;padding:4px 10px;font-size:11.5px;justify-content:space-between;flex-wrap:wrap}.src-card{max-width:200px}.src-turn{padding:10px 6px}.src-turn span{font-size:22px}.src-turn-back em{max-width:0}}',
    '@media (prefers-reduced-motion:reduce){.src-stage *{animation:none!important;transition:none!important}}',
    'html[data-motion="calm"] .src-stage *,html[data-motion="off"] .src-stage *{animation:none!important;transition:none!important}'
  ].join('\n');
  function injectStyle() {
    if (document.getElementById('src-style')) return;
    var st = document.createElement('style'); st.id = 'src-style'; st.textContent = CSS; document.head.appendChild(st);
  }

  /* ---- At the counter. You stand at the keeper's counter; the keeper's
     token stands behind it, lit from below by the lamp, with the shelves and
     the window behind. Press a good and the keeper brings it to the cloth,
     where you can look at it and say whether it goes on your pile. ---- */
  function counterView(R) {
    var LAMP = { x: 404, y: 372 }, LC = '#ffd2a0', CLOTH = { x: 600, y: 476 }, CASE = { x: 330, y: 486 }, KEEP = { x: 641, y: 250, r: 92 };
    var SHELF = [176, 290, 404], COLS = [92, 192, 292, 392];
    // What you choose piles up on the counter beside the cloth, back row first: x, y, scale.
    var PILE = [[742, 488, 1.12], [778, 486, 1.12], [814, 489, 1.12], [850, 487, 1.12], [886, 488, 1.12], [728, 506, 1.24], [766, 508, 1.24], [804, 505, 1.24], [842, 507, 1.24], [880, 506, 1.24]];
    var held = null, parcel = 0, sold = 0, lastHeld = '', parcelT = 0;
    function spot(i) { return [COLS[i % 4], SHELF[Math.floor(i / 4)]]; }
    function shelfIt() { return R.place.shelf; }

    function defs() {
      var lights = [{ x: LAMP.x, y: LAMP.y, z: 300, col: LC }, { x: 1128, y: 206, z: 120, col: '#c0905a', k: .55 }];
      return litF('srcb-plaster', [-300, -150, 1800, 620], .03, 3.2, 4, lights, '#3e4a74', .5) +
        litF('srcb-woodlit', [-300, 430, 1800, 470], '.003 .07', 2, 31, [{ x: LAMP.x, y: LAMP.y + 60, z: 260, col: LC }], '#3c4560', .42, 1.1) +
        litF('srcb-shelflit', [30, 90, 470, 340], .02, 1.4, 9, lights, '#3a3f5a', .55) +
        woodF('srcb-beam', 3, .004, .16, [.12, .07, .035], [.42, .27, .15]) + woodF('srcb-post', 9, .16, .004, [.12, .07, .035], [.4, .26, .14]) +
        woodF('srcb-board', 13, .11, .0035, [.14, .08, .04], [.5, .33, .18]) + woodF('srcb-top', 21, .0024, .052, [.17, .09, .04], [.6, .38, .2]) + woodF('srcb-front', 33, .0022, .06, [.11, .06, .03], [.42, .26, .14]) +
        '<filter id="srcb-stain" x="0" y="0" width="1" height="1"><feTurbulence type="fractalNoise" baseFrequency=".006 .014" numOctaves="3" seed="15"/>' +
          '<feColorMatrix type="matrix" values="0 0 0 0 0  0 0 0 0 0  0 0 0 0 0  0 0 0 -2.6 1.45"/><feComposite in="SourceGraphic" operator="in"/></filter>' +
        rad('srcb-shade', shade([[0, '#fff6e6'], [.16, '#fff6e6'], [.38, '#e6ded4'], [.58, '#a6a6b6'], [.8, '#525874'], [1, '#20243a']]), ' gradientUnits="userSpaceOnUse" cx="' + (LAMP.x + 150) + '" cy="' + (LAMP.y + 20) + '" r="940" gradientTransform="translate(' + (LAMP.x + 150) + ' ' + (LAMP.y + 20) + ') scale(1 .74) translate(' + (-LAMP.x - 150) + ' ' + (-LAMP.y - 20) + ')"') +
        rad('srcb-vig', [[.55, '#000', 0], [1, '#000', .55]], ' cx=".5" cy=".46" r=".8"') +
        lin('srcb-shaft', 1, [[0, '#a9bcff', .2], [1, '#a9bcff', 0]]) + lin('srcb-clothmat', 1, [[0, '#3a1416'], [1, '#5a2024']]) +
        lin('srcb-sil', 1, [[0, '#3a2c22'], [1, '#120c08']]) +
        '<clipPath id="srcb-winclip"><rect x="806" y="86" width="212" height="196"/></clipPath><clipPath id="srcb-tclip"><circle cx="' + KEEP.x + '" cy="' + KEEP.y + '" r="' + KEEP.r + '"/></clipPath>';
    }
    // A soft shadow on the counter from x,y, running away from the lamp and foreshortened like the top.
    function cast(x, y, w, len, op) {
      var a = Math.atan2((y - LAMP.y - 70) * 2, x - LAMP.x) * 180 / Math.PI;
      return '<g class="src-nohit" transform="translate(' + f1(x) + ',' + f1(y) + ') scale(1,.42) rotate(' + a.toFixed(1) + ')" opacity="' + (op || 1) + '"><path d="M0,' + (-w / 2) + ' L' + len + ',' + (-w * .9) + ' L' + len + ',' + (w * .9) + ' L0,' + (w / 2) + ' Z" fill="url(#src-cast)" filter="url(#src-blur5)"/></g>';
    }
    // On a shelf, a good's shadow falls on the wall behind, away from the lamp.
    function wallShadow(x, y, w, h) { return '<rect x="' + f1(x - w / 2 + (x - LAMP.x) * .06) + '" y="' + f1(y - h + (y - LAMP.y) * .05) + '" width="' + w + '" height="' + h + '" rx="' + f1(w / 2.4) + '" fill="#000" opacity=".36" filter="url(#src-blur5)"/>'; }

    function wall() {
      var s = '', r = rng(41), i, x, y;
      s += '<rect x="-300" y="-150" width="1800" height="620" fill="#b8a48a" filter="url(#srcb-plaster)"/><rect x="-300" y="-150" width="1800" height="620" fill="#4a3424" filter="url(#srcb-stain)" opacity=".55"/>';
      for (i = 0; i < 7; i++) { x = r() * 1200; y = 60 + r() * 360; s += '<path d="M' + x.toFixed(0) + ',' + y.toFixed(0) + ' l' + (r() * 30 - 15).toFixed(0) + ',' + (10 + r() * 20).toFixed(0) + ' l' + (r() * 20 - 10).toFixed(0) + ',' + (8 + r() * 16).toFixed(0) + '" stroke="#2a1a10" stroke-opacity=".3" stroke-width=".8" fill="none"/>'; }
      s += '<rect x="-300" y="-150" width="1800" height="184" fill="#000" filter="url(#srcb-beam)"/><rect x="-300" y="34" width="1800" height="10" fill="#000" opacity=".4" filter="url(#src-blur3)"/>';
      [[502, 26], [1060, 30]].forEach(function (p) { s += '<rect x="' + p[0] + '" y="34" width="' + p[1] + '" height="440" fill="#000" filter="url(#srcb-post)"/><rect x="' + (p[0] + p[1]) + '" y="34" width="12" height="440" fill="#000" opacity=".3" filter="url(#src-blur3)"/>'; });
      return s;
    }
    function shelves() {
      var s = '<rect x="34" y="80" width="452" height="370" fill="#000" filter="url(#srcb-board)"/><rect x="34" y="80" width="452" height="370" fill="#0a0604" opacity=".45"/>';
      for (var x = 34; x < 486; x += 56) s += '<rect x="' + x + '" y="80" width="1.6" height="370" fill="#050302" opacity=".7"/>';
      s += '<rect x="28" y="70" width="464" height="16" fill="#000" filter="url(#srcb-beam)"/><rect x="24" y="70" width="16" height="386" fill="#000" filter="url(#srcb-post)"/><rect x="480" y="70" width="16" height="386" fill="#000" filter="url(#srcb-post)"/>';
      SHELF.forEach(function (y) { s += '<rect x="40" y="' + y + '" width="440" height="12" fill="#000" filter="url(#srcb-beam)"/><rect x="40" y="' + y + '" width="440" height="2" fill="' + LC + '" opacity=".35"/><rect x="40" y="' + (y + 12) + '" width="440" height="14" fill="#000" opacity=".45" filter="url(#src-blur3)"/>'; });
      return '<g filter="url(#srcb-shelflit)">' + s + '</g>';
    }
    /* Spots with nothing for sale hold the keeper's own stock (jars, books, a
       mortar), so a small shop still looks like a working one. */
    function shelfDecor() {
      var s = '', r = rng(R.seed + 3), n = shelfIt().length;
      for (var i = n; i < 12; i++) {
        var p = spot(i), x = p[0] + (r() - .5) * 20, y = p[1], k = r();
        if (k < .55) { var w = 22 + r() * 14, h = 30 + r() * 18, col = ['#5a2a2a', '#2a4a5a', '#7a6a2a', '#3a5a7a', '#5a3a6a', '#2a5a3a'][Math.floor(r() * 6)];
          s += wallShadow(x, y, w, h) + '<rect x="' + f1(x - w / 2) + '" y="' + f1(y - h) + '" width="' + f1(w) + '" height="' + f1(h) + '" rx="5" fill="' + col + '"/><rect x="' + f1(x - w / 2) + '" y="' + f1(y - h) + '" width="' + f1(w) + '" height="' + f1(h) + '" rx="5" fill="url(#src-glassR)"/>' +
            '<rect x="' + f1(x - w / 2 - 2) + '" y="' + f1(y - h - 6) + '" width="' + f1(w + 4) + '" height="8" rx="2" fill="url(#src-cork)"/><rect x="' + f1(x - w / 3) + '" y="' + f1(y - h * .6) + '" width="' + f1(w / 1.5) + '" height="' + f1(h * .3) + '" fill="#e8dcbc" opacity=".75"/>'; }
        else if (k < .85) [['#4c1f1a', 12, 58], ['#1d3a24', 10, 52], ['#1a2644', 14, 62], ['#5a4a1a', 9, 48]].forEach(function (b, j) { var bx = x - 20 + j * 12; s += '<rect x="' + f1(bx) + '" y="' + (y - b[2]) + '" width="' + b[1] + '" height="' + b[2] + '" fill="' + b[0] + '"/><rect x="' + f1(bx) + '" y="' + (y - b[2] + 8) + '" width="' + b[1] + '" height="2" fill="#c8a060" opacity=".6"/>'; });
        else s += wallShadow(x, y, 46, 26) + '<path d="M' + f1(x - 24) + ',' + (y - 26) + ' h48 q-2,22 -16,26 h-16 q-14,-4 -16,-26 Z" fill="#6a6e76"/><ellipse cx="' + f1(x) + '" cy="' + (y - 26) + '" rx="24" ry="5" fill="#2a2c30"/>';
      }
      return '<g class="src-dof src-nohit">' + s + '</g>';
    }
    function wareOnShelf(it, i) {
      var p = spot(i), x = p[0], y = p[1], k = 'ware:' + it.id, left = R.stock(it), here = held && String(held.id) === String(it.id);
      if (it.out || left === 0 && !here) return '<g data-k="' + k + '" class="src-out"><rect x="' + (x - 30) + '" y="' + (y - 70) + '" width="60" height="70" fill="transparent"/><ellipse cx="' + x + '" cy="' + (y - 2) + '" rx="14" ry="2.6" fill="none" stroke="#c8b898" stroke-opacity=".35" stroke-width="1.2"/>' +
        '<g transform="translate(' + x + ',' + (y - 16) + ') rotate(-6)"><rect x="-22" y="-8" width="44" height="14" fill="#e8dcbc"/><text x="0" y="2" text-anchor="middle" class="src-tag">' + (it.out ? 'sold out' : 'all chosen') + '</text></g></g>';
      if (here && left !== null && left < 2) return '<g data-k="' + k + '" class="src-ware"><rect x="' + (x - 30) + '" y="' + (y - 70) + '" width="60" height="70" fill="transparent"/><ellipse cx="' + x + '" cy="' + (y - 1) + '" rx="20" ry="3" fill="#000" opacity=".25"/></g>';
      var s = '<g data-k="' + k + '" class="src-ware"><rect x="' + (x - 34) + '" y="' + (y - 78) + '" width="68" height="78" fill="transparent"/>';
      // More than one left: the rest stand behind, darker.
      if (left === null || left > 1) s += '<g transform="translate(' + (x + 16) + ',' + (y - 2) + ') scale(1.04)" opacity=".55" filter="url(#src-dof)">' + ware(it, 'R', R.icons) + '</g>';
      if (!here) s += wallShadow(x, y, 46, 70) + '<g transform="translate(' + x + ',' + y + ') scale(1.2)">' + ware(it, 'R', R.icons) + '</g>';
      return s + '</g>';
    }
    function wares() { return shelfIt().map(wareOnShelf).join(''); }
    function windowArt() {
      var s = '<g data-k="window"><rect x="782" y="64" width="260" height="246" rx="6" fill="#000" opacity=".45" filter="url(#src-blur8)"/><rect x="792" y="72" width="240" height="224" rx="3" fill="url(#src-frame)"/>';
      s += '<g clip-path="url(#srcb-winclip)">' + skyArt('srcb-sky', 806, 86, 212, 196, R.model.time) +
        '<path d="M806,282 L806,222 L822,222 L822,212 L840,200 L858,212 L858,228 L876,228 L876,216 L892,206 L908,216 L908,236 L930,236 L930,282 Z" fill="#090d1a"/>' +
        '<g fill="#e8b860" opacity=".6"><rect x="828" y="230" width="3" height="4"/><rect x="884" y="222" width="3" height="4"/><rect x="914" y="244" width="3" height="3"/></g>' +
        '<path d="M960,282 L960,244 L976,232 L992,244 L992,282 Z" fill="#0a0f1f"/><rect x="806" y="210" width="212" height="72" fill="url(#src-fog)"/><path d="M806,86 L890,86 L806,200 Z" fill="#fff" opacity=".05"/></g>';
      s += '<rect x="909" y="86" width="7" height="196" fill="#2e1e10"/><rect x="806" y="176" width="212" height="6" fill="#2e1e10"/><rect x="914" y="86" width="2" height="196" fill="#e0a868" opacity=".3"/>';
      return s + '<rect x="786" y="296" width="252" height="12" rx="2" fill="#4a3019"/><rect x="786" y="296" width="252" height="3.5" fill="#8a6038"/><rect x="786" y="308" width="252" height="10" fill="#000" opacity=".4" filter="url(#src-blur3)"/></g>';
    }
    function sconce() {
      var x = 1128, y = 206;
      return '<g class="src-nohit"><rect x="' + (x - 4) + '" y="' + (y + 24) + '" width="8" height="30" fill="url(#src-iron)"/><path d="M' + (x - 14) + ',' + (y + 24) + ' h28 l-4,6 h-20 Z" fill="url(#src-brass)"/>' +
        '<rect x="' + (x - 5) + '" y="' + (y + 2) + '" width="10" height="22" rx="2" fill="#e8dcc0"/><path class="src-flame" d="M' + x + ',' + (y - 14) + ' c-5,8 -5,12 0,16 c5,-4 5,-8 0,-16 Z" fill="url(#src-flame)"/><circle cx="' + x + '" cy="' + (y - 6) + '" r="46" fill="url(#src-glow)" data-flick=".5"/></g>';
    }
    /* The keeper as a token: the shop page's picture in a brass ring on a
       stand, the way it sits on a map, or a plain silhouette when the page
       has no picture. The room's light and falloff lie over it like the
       rest, and it breathes and nods as one piece. */
    function keeper() {
      var c = KEEP, url = safeUrl(R.model.image), face;
      face = url ? '<rect x="' + (c.x - c.r) + '" y="' + (c.y - c.r) + '" width="' + 2 * c.r + '" height="' + 2 * c.r + '" fill="#2a2018"/><image href="' + esc(url) + '" x="' + (c.x - c.r) + '" y="' + (c.y - c.r) + '" width="' + 2 * c.r + '" height="' + 2 * c.r + '" preserveAspectRatio="xMidYMid slice"/>'
        : '<rect x="' + (c.x - c.r) + '" y="' + (c.y - c.r) + '" width="' + 2 * c.r + '" height="' + 2 * c.r + '" fill="#4a3a2c"/><circle cx="' + c.x + '" cy="' + (c.y - 18) + '" r="34" fill="url(#srcb-sil)"/><path d="M' + (c.x - 70) + ',' + (c.y + c.r) + ' C' + (c.x - 66) + ',' + (c.y + 30) + ' ' + (c.x - 30) + ',' + (c.y + 16) + ' ' + c.x + ',' + (c.y + 16) + ' C' + (c.x + 30) + ',' + (c.y + 16) + ' ' + (c.x + 66) + ',' + (c.y + 30) + ' ' + (c.x + 70) + ',' + (c.y + c.r) + ' Z" fill="url(#srcb-sil)"/>';
      return '<g data-k="keeper" class="srcb-keeper"><circle cx="' + (c.x + 56) + '" cy="' + (c.y - 20) + '" r="' + c.r + '" fill="#000" opacity=".3" filter="url(#src-blur14)"/>' +
        '<rect x="' + (c.x - 3) + '" y="' + (c.y + c.r) + '" width="6" height="' + (452 - c.y - c.r) + '" fill="url(#src-brass)"/>' +
        '<g class="srcb-body"><g class="srcb-head"><g clip-path="url(#srcb-tclip)">' + face + '</g>' +
        '<circle cx="' + c.x + '" cy="' + c.y + '" r="' + c.r + '" fill="none" stroke="url(#src-brass)" stroke-width="9"/><circle cx="' + c.x + '" cy="' + c.y + '" r="' + (c.r - 5) + '" fill="none" stroke="#000" stroke-opacity=".5" stroke-width="2"/>' +
        '<path d="M' + f1(c.x - c.r * .8) + ',' + f1(c.y - c.r * .5) + ' A' + c.r + ',' + c.r + ' 0 0 1 ' + f1(c.x + c.r * .2) + ',' + f1(c.y - c.r * .98) + '" stroke="#fff6dc" stroke-opacity=".45" stroke-width="2" fill="none"/></g></g></g>';
    }
    // A dark oak front of raised panels and a worn top the lamp shines across.
    function counter() {
      var s = '<path d="M-300,452 H1500 V512 H-300 Z" fill="#000" filter="url(#srcb-top)"/>', r = rng(11), i, x;
      for (i = 0; i < 5; i++) s += '<path d="M-300,' + (458 + i * 11) + ' H1500" stroke="#1a0e06" stroke-opacity=".5" stroke-width="1.2"/>';
      for (i = 0; i < 40; i++) { x = r() * 1200; s += '<path d="M' + x.toFixed(0) + ',' + (460 + r() * 46).toFixed(0) + ' l' + (10 + r() * 40).toFixed(0) + ',' + (r() * 2 - 1).toFixed(1) + '" stroke="#e8c090" stroke-opacity="' + (.06 + r() * .1).toFixed(2) + '" stroke-width=".8"/>'; }
      s += '<path d="M-300,506 H1500 V516 H-300 Z" fill="#000" filter="url(#srcb-beam)"/><rect x="-300" y="506" width="1800" height="2.4" fill="' + LC + '" opacity=".45"/><rect x="-300" y="516" width="1800" height="384" fill="#000" filter="url(#srcb-front)"/>';
      for (x = -368; x < 1500; x += 196) s += '<rect x="' + x + '" y="540" width="168" height="140" rx="3" fill="#000" opacity=".28"/><rect x="' + (x + 10) + '" y="550" width="148" height="120" rx="2" fill="#000" filter="url(#srcb-front)"/>' +
        '<path d="M' + (x + 10) + ',670 V550 H' + (x + 158) + '" stroke="#e8c090" stroke-opacity=".18" stroke-width="2" fill="none"/><path d="M' + (x + 158) + ',550 V670 H' + (x + 10) + '" stroke="#000" stroke-opacity=".5" stroke-width="2" fill="none"/>';
      return '<g filter="url(#srcb-woodlit)">' + s + '<rect x="-300" y="516" width="1800" height="30" fill="#000" opacity=".5" filter="url(#src-blur8)"/></g>';
    }
    function cloth() {
      var c = CLOTH, d = 'M' + (c.x - 96) + ',' + (c.y - 16) + ' L' + (c.x + 96) + ',' + (c.y - 16) + ' L' + (c.x + 112) + ',' + (c.y + 24) + ' L' + (c.x - 112) + ',' + (c.y + 24) + ' Z';
      return '<g data-k="cloth"><path d="' + d + '" fill="url(#srcb-clothmat)"/><path d="' + d + '" fill="#fff" opacity=".06" filter="url(#src-velvet)"/><path d="M' + (c.x - 110) + ',' + (c.y + 22) + ' L' + (c.x + 110) + ',' + (c.y + 22) + '" stroke="#c8a060" stroke-opacity=".5" stroke-width="1.4"/></g>';
    }
    function bell() {
      var x = 950, y = 496;
      return '<g data-k="bell"><ellipse cx="' + x + '" cy="' + (y + 1) + '" rx="26" ry="5" fill="url(#src-ao)"/><g class="srcb-bell"><path d="M' + (x - 22) + ',' + y + ' C' + (x - 22) + ',' + (y - 16) + ' ' + (x - 14) + ',' + (y - 30) + ' ' + x + ',' + (y - 30) + ' C' + (x + 14) + ',' + (y - 30) + ' ' + (x + 22) + ',' + (y - 16) + ' ' + (x + 22) + ',' + y + ' Z" fill="url(#src-brass)"/>' +
        '<rect x="' + (x - 24) + '" y="' + (y - 3) + '" width="48" height="5" rx="2" fill="url(#src-brassv)"/><circle cx="' + x + '" cy="' + (y - 33) + '" r="4" fill="url(#src-brass)"/></g></g>';
    }
    function scales() {
      var x = 1046, y = 490;
      return '<g data-k="scales"><ellipse cx="' + x + '" cy="' + (y + 2) + '" rx="46" ry="7" fill="url(#src-ao)"/><rect x="' + (x - 30) + '" y="' + (y - 8) + '" width="60" height="9" rx="2" fill="url(#src-frame)"/>' +
        '<rect x="' + (x - 3) + '" y="' + (y - 96) + '" width="6" height="90" fill="url(#src-brass)"/><g class="srcb-beam' + (sold ? ' tip' : '') + '"><rect x="' + (x - 62) + '" y="' + (y - 98) + '" width="124" height="4" rx="2" fill="url(#src-brassv)"/>' +
        '<g class="srcb-panL"><path d="M' + (x - 58) + ',' + (y - 94) + ' L' + (x - 74) + ',' + (y - 46) + ' M' + (x - 58) + ',' + (y - 94) + ' L' + (x - 42) + ',' + (y - 46) + '" stroke="#a88a4a" stroke-width="1"/><path d="M' + (x - 80) + ',' + (y - 46) + ' q22,12 44,0 Z" fill="url(#src-brass)"/></g>' +
        '<g class="srcb-panR"><path d="M' + (x + 58) + ',' + (y - 94) + ' L' + (x + 42) + ',' + (y - 46) + ' M' + (x + 58) + ',' + (y - 94) + ' L' + (x + 74) + ',' + (y - 46) + '" stroke="#a88a4a" stroke-width="1"/><path d="M' + (x + 36) + ',' + (y - 46) + ' q22,12 44,0 Z" fill="url(#src-brass)"/><g class="srcb-coins">' + coins() + '</g></g></g>' +
        '<circle cx="' + x + '" cy="' + (y - 98) + '" r="5" fill="url(#src-brass)"/></g>';
    }
    function coins() { var s = ''; for (var j = 0; j < Math.min(10, sold); j++) s += '<ellipse cx="' + (1104 + (j % 3 - 1) * 7) + '" cy="' + (441 - Math.floor(j / 3) * 3) + '" rx="7" ry="2.6" fill="#d6aa4a" stroke="#7a5a1a" stroke-width=".6"/>'; return s; }
    function ledger() {
      var x = 170, y = 486, pg = 'M' + (x - 100) + ',' + (y - 20) + ' L' + x + ',' + (y - 24) + ' L' + x + ',' + (y + 6) + ' L' + (x - 110) + ',' + (y + 2) + ' Z M' + x + ',' + (y - 24) + ' L' + (x + 100) + ',' + (y - 20) + ' L' + (x + 110) + ',' + (y + 2) + ' L' + x + ',' + (y + 6) + ' Z';
      return '<g class="src-nohit"><ellipse cx="' + x + '" cy="' + (y + 6) + '" rx="110" ry="10" fill="url(#src-ao)" opacity=".8"/><path d="' + pg + '" fill="#ebdfc2"/><path d="' + pg + '" fill="#000" opacity=".08" filter="url(#src-paper)"/>' +
        [0, 1, 2, 3].map(function (i) { return '<path d="M' + (x - 92) + ',' + (y - 14 + i * 5) + ' l80,-2 M' + (x + 10) + ',' + (y - 16 + i * 5) + ' l84,2" stroke="#4a3826" stroke-opacity=".5" stroke-width=".8"/>'; }).join('') +
        '<path d="M' + (x + 60) + ',' + (y - 30) + ' L' + (x + 150) + ',' + (y - 80) + '" stroke="#e8e0d0" stroke-width="2.4"/><path d="M' + (x + 100) + ',' + (y - 58) + ' q30,-30 52,-26 q-14,20 -50,30" fill="#e8e0d0" opacity=".85"/></g>';
    }
    // The keeper's case of finer goods, shut on the counter. Press it and the keeper opens it for you.
    function caseShut() {
      if (!R.place.case.length) return '';
      var x = CASE.x, y = CASE.y;
      return '<g data-k="case"><ellipse cx="' + x + '" cy="' + (y + 3) + '" rx="50" ry="7" fill="url(#src-ao)"/>' +
        '<path d="M' + (x - 44) + ',' + y + ' V' + (y - 30) + ' H' + (x + 44) + ' V' + y + ' Z" fill="#5a2e14"/><path d="M' + (x - 46) + ',' + (y - 30) + ' Q' + x + ',' + (y - 46) + ' ' + (x + 46) + ',' + (y - 30) + ' Z" fill="#6a3818"/>' +
        '<path d="M' + (x - 46) + ',' + (y - 30) + ' H' + (x + 46) + '" stroke="#2a140a" stroke-width="2"/><path d="M' + (x - 44) + ',' + (y - 29) + ' Q' + x + ',' + (y - 44) + ' ' + (x + 44) + ',' + (y - 29) + '" stroke="' + LC + '" stroke-opacity=".35" stroke-width="1.4" fill="none"/>' +
        [-44, 38].map(function (d) { return '<rect x="' + (x + d) + '" y="' + (y - 31) + '" width="6" height="31" fill="url(#src-brassv)"/>'; }).join('') +
        '<rect x="' + (x - 7) + '" y="' + (y - 26) + '" width="14" height="12" rx="2" fill="url(#src-brass)"/><circle cx="' + x + '" cy="' + (y - 20) + '" r="2" fill="#3a2a10"/></g>';
    }
    // More goods than the room has places for: a card on the counter says so and opens the list.
    function moreCard() {
      var n = R.place.more.length; if (!n) return '';
      return '<g data-k="more"><g transform="translate(1110,520) rotate(-4)"><rect x="-58" y="-26" width="116" height="40" rx="3" fill="url(#src-tag)"/><rect x="-58" y="-26" width="116" height="40" rx="3" fill="#000" opacity=".06" filter="url(#src-paper)"/>' +
        '<text x="0" y="-8" text-anchor="middle" class="src-hand">+' + n + ' more</text><text x="0" y="7" text-anchor="middle" class="src-hand small">see the wares list</text></g></g>';
    }
    /* The pile: one of each thing chosen, in the order chosen. Past ten, the
       last one carries a count. After paying, one brown-paper parcel. */
    function pileSvg() {
      if (parcel) {
        var x = 812, y = 506, d = 'M' + (x - 62) + ',' + y + ' L' + (x - 56) + ',' + (y - 46) + ' Q' + x + ',' + (y - 58) + ' ' + (x + 56) + ',' + (y - 46) + ' L' + (x + 62) + ',' + y + ' Z';
        return '<g class="srcb-parcel src-nohit"><ellipse cx="' + x + '" cy="' + (y + 2) + '" rx="70" ry="9" fill="#000" opacity=".45" filter="url(#src-blur3)"/><path d="' + d + '" fill="#c8b088"/><path d="' + d + '" fill="#000" opacity=".08" filter="url(#src-paper)"/>' +
          '<path d="M' + (x - 4) + ',' + (y - 54) + ' V' + y + ' M' + (x - 59) + ',' + (y - 22) + ' Q' + x + ',' + (y - 30) + ' ' + (x + 59) + ',' + (y - 22) + '" stroke="#e8dcbc" stroke-width="2.2" fill="none"/></g>';
      }
      var u = [], s = '';
      R.model.its.forEach(function (it) { for (var i = 0, n = R.inBasket(it.id); i < n; i++) u.push(it); });
      var shown = u.slice(0, PILE.length), more = u.length - shown.length, order = shown.map(function (x, i) { return i; });
      order.sort(function (a, b) { return PILE[a][1] - PILE[b][1]; }).forEach(function (i) {
        var p = PILE[i], it = shown[i], tilt = ((i * 37) % 9 - 4) * .8;
        s += '<g data-k="pile:' + it.id + '" class="srcb-pileware"><ellipse class="src-nohit" cx="' + p[0] + '" cy="' + (p[1] + 1) + '" rx="' + f1(20 * p[2]) + '" ry="' + f1(4 * p[2]) + '" fill="#000" opacity=".45" filter="url(#src-blur3)"/>' +
          '<g transform="translate(' + p[0] + ',' + p[1] + ') rotate(' + tilt.toFixed(1) + ') scale(' + p[2] + ')">' + ware(it, 'R', R.icons) + '</g></g>';
      });
      if (more > 0) { var q = PILE[PILE.length - 1]; s += '<g class="src-nohit"><circle cx="' + (q[0] + 18) + '" cy="' + (q[1] - 62) + '" r="13" fill="#2a1a0e" stroke="#c8a060" stroke-width="1.4"/><text x="' + (q[0] + 18) + '" y="' + (q[1] - 57) + '" text-anchor="middle" class="src-more">+' + more + '</text></g>'; }
      return s;
    }
    function svg() {
      // The wall runs on, in shadow, below the counter's edge, so walking closer never shows a gap under it.
      return '<rect width="1200" height="700" fill="#0a0705"/><g data-depth=".55"><rect x="-300" y="440" width="1800" height="460" fill="url(#src-wallfoot)"/><g data-bake="wall">' + wall() + '</g>' +
        '<circle cx="' + LAMP.x + '" cy="' + LAMP.y + '" r="520" fill="url(#src-glow)" data-flick=".32"/><g data-bake="shelves">' + shelves() + '</g>' +
        '<rect data-k="shelves" x="34" y="96" width="452" height="320" fill="transparent"/>' + shelfDecor() + '<g id="srcb-wares">' + wares() + '</g>' + windowArt() + sconce() +
        '<path d="M806,282 L760,452 L900,452 L1018,282 Z" fill="url(#srcb-shaft)" opacity=".6" pointer-events="none"/></g>' +
        '<g data-depth=".75">' + keeper() + '</g>' +
        '<g data-depth="1"><g data-bake="counter">' + counter() + '</g>' + cast(LAMP.x, 478, 40, 120, 0) + ledger() + caseShut() + lampArt(LAMP.x, 478) + cloth() +
          '<g id="srcb-pile">' + pileSvg() + '</g>' + bell() + scales() + moreCard() + '<g id="srcb-held"></g></g>' +
        '<rect data-depth=".8" x="-300" y="-200" width="1800" height="1100" fill="url(#srcb-shade)" data-flick=".98" pointer-events="none"/>' +
        '<rect width="1200" height="700" fill="url(#srcb-vig)" pointer-events="none"/>';
    }

    function info(k) {
      var p = k.split(':'), it = p[1] ? R.item(p[1]) : null;
      if ((p[0] === 'ware' || p[0] === 'held') && it) return R.info(it, p[0] === 'held' ? 'Drag it onto your pile · Esc puts it back' : it.out ? '' : 'Press and the keeper brings it to the counter');
      if (p[0] === 'pile' && it) return { title: it.n, meta: it.price + ' each', now: R.inBasket(it.id) + ' on your pile', foot: 'Right-click to put one back' };
      if (k === 'case') return { title: 'The keeper’s case', meta: 'Finer goods', body: R.place.case.length + (R.place.case.length === 1 ? ' thing' : ' things') + ' kept shut and out of the dust.', foot: 'Press and the keeper opens it for you' };
      if (k === 'shelves') return { title: 'The shelves', body: 'Everyday goods, a few of each.', foot: 'Press to walk closer' };
      if (k === 'window') return { title: 'The window', body: 'The lane outside.' };
      if (k === 'keeper') return { title: R.model.name, meta: 'The keeper', foot: R.model.lines ? 'Press to talk' : '' };
      if (k === 'bell') return { title: 'The counter bell', foot: 'Press to ring it' };
      if (k === 'lamp') return { title: 'Oil lamp', body: 'The shop’s one real light.', foot: 'Press to turn up the wick' };
      if (k === 'scales') return { title: 'The scales', body: sold ? 'Your coins sit in the pan.' : 'Coins go in the pan when you pay.' };
      if (k === 'cloth') return { title: 'The counter cloth', body: 'The keeper lays a good here for you to look at.' };
      if (k === 'more') return { title: R.place.more.length + ' more for sale', body: 'More goods than the shop has places for. Every one is in the wares list.', foot: 'Press to open the list' };
      return null;
    }
    function hint() {
      if (held && held.phase === 'rest') {
        var it = R.item(held.id), can = R.canAdd(held.id);
        R.say('<span>The keeper sets the ' + esc(it ? it.n : 'good') + ' on the cloth.' + (can ? ' Drag it onto your pile, or:' : ' ' + esc(R.why(held.id))) + '</span><span class="src-acts">' +
          (can ? '<button type="button" class="src-btn primary" data-act="take">Add to my pile</button>' : '') + '<button type="button" class="src-btn" data-act="back">Put it back</button></span>');
      } else R.say(R.model.its.length ? '<span>Press a good on the shelves and the keeper brings it to the counter.' + (R.place.case.length ? ' Finer goods are in the case.' : '') + '</span>' : '<span>Nothing for sale right now.</span>');
    }
    function menu(k) {
      var p = k.split(':'), it = p[1] ? R.item(p[1]) : null;
      if (p[0] === 'ware' && it) return [['Ask the keeper to bring it', function () { fetch(it.id); }, it.out || R.stock(it) === 0], ['Put one on my pile', function () { R.add(it.id); }, !R.canAdd(it.id)]];
      if (p[0] === 'pile' && it) return [['Put one back', function () { R.remove(it.id); R.hideCard(); }], ['Put them all back', function () { while (R.inBasket(it.id)) R.remove(it.id); R.hideCard(); }]];
      if (p[0] === 'held') return [['Add to my pile', toBasket, !R.canAdd(held.id)], ['Put it back', putBack]];
      if (k === 'case') return [['Ask the keeper to open it', function () { click('case'); }]];
      return null;
    }
    function nod() { var h = R.svg.querySelector('.srcb-head'); if (!h) return; h.classList.remove('nod', 'look'); void h.getBBox(); h.classList.add('nod'); }
    function look() { var h = R.svg.querySelector('.srcb-head'); if (!h) return; h.classList.remove('nod', 'look'); void h.getBBox(); h.classList.add('look'); }
    function ring() { var b = R.svg.querySelector('.srcb-bell'); if (b) { b.classList.remove('ring'); void b.getBBox(); b.classList.add('ring'); } nod(); R.talk(R.svg.querySelector('[data-k="keeper"]')); }
    function rewares() { var g = R.svg.querySelector('#srcb-wares'); if (g) { g.innerHTML = wares(); R.tab(g); } }
    function repile() { var g = R.svg.querySelector('#srcb-pile'); if (g) { g.innerHTML = pileSvg(); R.tab(g); } }
    /* The keeper fetches: the token turns to the shelf and the good travels in
       an arc to the cloth, growing as it comes nearer. */
    function fetch(id) {
      var i = -1; shelfIt().forEach(function (it, j) { if (String(it.id) === String(id)) i = j; });
      if (i < 0) return;
      var p = spot(i);
      held = { id: id, phase: 'fly', t: 0, dur: R.calm ? .01 : .85, from: [p[0], p[1], 1.2], to: [CLOTH.x, CLOTH.y + 8, 2.3], x: p[0], y: p[1], s: 1.2 };
      R.hideCard(); rewares(); look();
    }
    function fly(to, phase, dur) { held.from = [held.x, held.y, held.s]; held.to = to; held.phase = phase; held.t = 0; held.dur = dur; }
    function slot() { var n = 0; R.model.its.forEach(function (it) { n += R.inBasket(it.id); }); return PILE[Math.min(PILE.length - 1, n)]; }
    function toBasket() {
      if (!held) return;
      if (!R.canAdd(held.id)) { R.pop(R.why(held.id), held.x, held.y - 80); fly([CLOTH.x, CLOTH.y + 8, 2.3], 'fly', .3); return; }
      var p = slot(); fly([p[0], p[1], p[2]], 'tobasket', R.calm ? .01 : .45);
    }
    function putBack() {
      if (!held) return;
      var i = 0; shelfIt().forEach(function (it, j) { if (String(it.id) === String(held.id)) i = j; });
      var p = spot(i); fly([p[0], p[1], 1.2], 'return', R.calm ? .01 : .6); R.hideCard();
    }
    function heldSvg() {
      if (!held) return '';
      var h = held, it = R.item(h.id), rest = h.phase === 'rest', sh = '';
      if (!it) return '';
      if (rest || h.phase === 'drag') sh = cast(h.x, h.y, 46, 130, .8) + '<ellipse class="src-nohit" cx="' + f1(h.x) + '" cy="' + f1(h.y + 1) + '" rx="' + f1(h.s * 20) + '" ry="' + f1(h.s * 3.4) + '" fill="url(#src-ao)"/>';
      return sh + '<g data-k="held:' + h.id + '" class="src-heldware' + (h.phase === 'drag' ? ' drag' : '') + '" transform="translate(' + f1(h.x) + ',' + f1(h.y) + ') scale(' + h.s.toFixed(3) + ')"><rect x="-30" y="-70" width="60" height="72" fill="transparent"/>' + ware(it, 'L', R.icons) + '</g>' +
        (rest ? '<circle class="src-nohit" cx="' + f1(h.x) + '" cy="' + f1(h.y - 60) + '" r="70" fill="url(#src-glow)" opacity=".18"/>' : '');
    }
    function frame(t, dt) {
      var g = R.svg.querySelector('#srcb-held'); if (!g) return;
      if (held && held.phase !== 'rest' && held.phase !== 'drag') {
        held.t += dt / held.dur;
        var k = Math.min(1, held.t), e = ease(k), f = held.from, to = held.to;
        held.x = f[0] + (to[0] - f[0]) * e; held.y = f[1] + (to[1] - f[1]) * e - Math.sin(Math.PI * k) * (held.phase === 'tobasket' ? 60 : 120); held.s = f[2] + (to[2] - f[2]) * e;
        if (k >= 1) {
          var ph = held.phase;
          if (ph === 'fly') { held.phase = 'rest'; hint(); }
          else if (ph === 'tobasket') { var id = held.id; held = null; R.add(id); rewares(); hint(); }
          else { held = null; rewares(); hint(); }
        }
      }
      var out = heldSvg();
      if (out !== lastHeld) { g.innerHTML = out; lastHeld = out; R.tab(g); }
    }
    function overPile(x, y) { return x > 700 && x < 920 && y > 420 && y < 540; }
    /* Dragging the good off the cloth: over the basket or the pile it lights
       up, and letting go there puts it on the pile; anywhere else it settles
       back on the cloth. */
    function afterDraw() {
      var svg = R.svg;
      svg.addEventListener('pointerdown', function (e) {
        var t = e.target.closest && e.target.closest('[data-k^="held:"]'); if (!t || !held || held.phase !== 'rest') return;
        e.preventDefault(); R.dragging = true; R.hideCard(); held.phase = 'drag';
        var q = R.toScene(e); held.dx = held.x - q[0]; held.dy = held.y - q[1];
        try { svg.setPointerCapture(e.pointerId); } catch (x) { /* a synthetic pointer has nothing to capture */ }
      });
      svg.addEventListener('pointermove', function (e) {
        if (!held || held.phase !== 'drag') return;
        var q = R.toScene(e); held.x = q[0] + held.dx; held.y = q[1] + held.dy; held.s = 2.3 - Math.max(0, (CLOTH.y - held.y) / 600);
        R.basketHot(R.overBasket(e.clientX, e.clientY));
        var pg = svg.querySelector('#srcb-pile'); if (pg) pg.classList.toggle('hot', overPile(q[0], q[1]));
      });
      svg.addEventListener('pointerup', function (e) {
        if (!held || held.phase !== 'drag') return;
        setTimeout(function () { R.dragging = false; }, 0); R.basketHot(false);
        var pg = svg.querySelector('#srcb-pile'); if (pg) pg.classList.remove('hot');
        var q = R.toScene(e);
        if (R.overBasket(e.clientX, e.clientY) || overPile(q[0], q[1])) { held.phase = 'rest'; toBasket(); }
        else fly([CLOTH.x, CLOTH.y + 8, 2.3], 'fly', .35);
      });
    }
    function act(a) { if (a === 'take') toBasket(); else if (a === 'back') putBack(); }
    function click(k) {
      var p = k.split(':'), it = p[1] ? R.item(p[1]) : null;
      // Walking up to the shelves; pressing a good from there steps back so the keeper can bring it.
      if (k === 'shelves') { if (R.cam().z < 1.01 && !held) R.closer(260, 250, 1.65); return true; }
      if (p[0] === 'ware' && it) { if (it.out || R.stock(it) === 0) return false; if (R.cam().z > 1.01) R.stepBack(function () { fetch(it.id); }); else fetch(it.id); return true; }
      if (k === 'case' && R.place.case.length) { R.go('case', 'in'); return true; }
      if (p[0] === 'held') return true;
      if (k === 'bell') { ring(); return true; }
      if (k === 'keeper') { nod(); R.talk(R.svg.querySelector('[data-k="keeper"]')); return true; }
      if (k === 'lamp') { R.flare(1); return true; }
      if (k === 'more') { R.openList(); return true; }
      return false;
    }
    function escape() { if (held && held.phase === 'rest') { putBack(); return true; } return false; }
    function sync() { rewares(); repile(); if (held && !R.item(held.id)) { held = null; } hint(); }
    // Paying: the coins drop into the scales' far pan and it dips, and the pile becomes a parcel.
    function bought(n) {
      parcel = 1; sold += Math.max(1, Math.ceil(n / 2)); repile();
      var cg = R.svg.querySelector('.srcb-coins'), bm = R.svg.querySelector('.srcb-beam');
      if (cg) cg.innerHTML = coins(); if (bm) bm.classList.add('tip');
      clearTimeout(parcelT); parcelT = setTimeout(function () { parcel = 0; if (R.svg.isConnected) repile(); }, 4200);
      nod();
    }
    return { id: 'counter', label: R.model.name + ', seen from the counter by lamplight', defs: defs, svg: svg, info: info, menu: menu, click: click, frame: frame, afterDraw: afterDraw, act: act, escape: escape, sync: sync, bought: bought, hint: hint,
      destroy: function () { clearTimeout(parcelT); } };
  }

  /* ---- The shop floor: the other side of the shop, behind you at the
     counter. The door to the lane with the sky in its panes, a glass cabinet
     of the bigger goods, barrels of goods sold in bulk with a chalk board,
     and the room's own working piece. One hanging lantern lights it; the
     door gets the moonlight so it always reads. Press a good to pick it up;
     it goes to your pile only when you say so. ---- */
  function floorView(R) {
    var LANT = { x: 720, y: 196 }, LC = '#ffcf90', HOLD = [600, 600, 2.3];
    var CAB = [[393, 184], [497, 184], [393, 296], [497, 296], [393, 408], [497, 408]], BARREL = [662, 784], BY = 522;
    var held = null, lastHeld = '', flying = [], lastFly = '', at = null, piece = floorPiece(R.model.roomType);
    var NEAR = { cabinet: [445, 250, 1.7], board: [725, 420, 1.6], piece: [1010, 440, 1.7], door: [160, 280, 1.6] };

    function defs() {
      var L = [{ x: LANT.x, y: LANT.y, z: 300, col: LC }, { x: 160, y: 200, z: 160, col: '#8ea4e0', k: .35 }];
      return litF('srcf-plaster', [-300, -150, 1800, 630], .03, 3.2, 14, L, '#3e4a74', .5) +
        litF('srcf-woodlit', [-300, 440, 1800, 560], '.004 .08', 2, 21, [{ x: LANT.x, y: LANT.y + 200, z: 300, col: LC }], '#3c4560', .42, 1.1) +
        litF('srcf-oaklit', [30, 40, 1170, 600], .02, 1.4, 9, L, '#3a3f5a', .55) +
        woodF('srcf-boards', 61, .0025, .07, [.13, .07, .03], [.46, .28, .14]) + woodF('srcf-oak', 63, .09, .004, [.15, .08, .035], [.5, .3, .15]) +
        woodF('srcf-beam', 65, .003, .09, [.1, .05, .02], [.36, .2, .1]) + woodF('srcf-stave', 67, .12, .006, [.18, .1, .04], [.58, .36, .18]) +
        '<filter id="srcf-stain" x="0" y="0" width="1" height="1"><feTurbulence type="fractalNoise" baseFrequency=".006 .02" numOctaves="3" seed="19"/>' +
          '<feColorMatrix type="matrix" values="0 0 0 0 0  0 0 0 0 0  0 0 0 0 0  0 0 0 -2.6 1.45"/><feComposite in="SourceGraphic" operator="in"/></filter>' +
        // A wide falloff: the door at the far left stays readable.
        rad('srcf-shade', shade([[0, '#fff6e6'], [.16, '#fff6e6'], [.38, '#e6ded4'], [.58, '#a6a6b6'], [.8, '#525874'], [1, '#20243a']]), ' gradientUnits="userSpaceOnUse" cx="' + LANT.x + '" cy="' + (LANT.y + 120) + '" r="680" gradientTransform="translate(' + LANT.x + ' ' + (LANT.y + 120) + ') scale(1.4 .8) translate(' + (-LANT.x) + ' ' + (-LANT.y - 120) + ')"') +
        rad('srcf-vig', [[.55, '#000', 0], [1, '#000', .55]], ' cx=".5" cy=".46" r=".8"') +
        lin('srcf-shaft', 1, [[0, '#a9bcff', .22], [1, '#a9bcff', 0]]) + lin('srcf-copper', 0, [[0, '#3a1a0a'], [.3, '#a8582a'], [.55, '#f0a060'], [.7, '#b8642e'], [1, '#3a1a0a']]) +
        lin('srcf-glass', 0, [[0, '#fff', 0], [.2, '#fff', .1], [.24, '#fff', .02], [.7, '#fff', 0], [.74, '#fff', .08], [1, '#fff', 0]]) + lin('srcf-chalk', 1, [[0, '#232a26'], [1, '#151a17']]) +
        '<clipPath id="srcf-paneclip"><rect x="98" y="148" width="124" height="140"/></clipPath>';
    }
    function wall() {
      var s = '', r = rng(43), i, x, y;
      s += '<rect x="-300" y="-150" width="1800" height="630" fill="#b8a48a" filter="url(#srcf-plaster)"/><rect x="-300" y="-150" width="1800" height="630" fill="#4a3424" filter="url(#srcf-stain)" opacity=".55"/>';
      for (i = 0; i < 7; i++) { x = r() * 1200; y = 60 + r() * 360; s += '<path d="M' + x.toFixed(0) + ',' + y.toFixed(0) + ' l' + (r() * 30 - 15).toFixed(0) + ',' + (10 + r() * 20).toFixed(0) + '" stroke="#2a1a10" stroke-opacity=".3" stroke-width=".8" fill="none"/>'; }
      s += '<rect x="-300" y="-150" width="1800" height="184" fill="#000" filter="url(#srcf-beam)"/><rect x="-300" y="34" width="1800" height="10" fill="#000" opacity=".4" filter="url(#src-blur3)"/>';
      [[284, 26], [860, 28]].forEach(function (p) { s += '<rect x="' + p[0] + '" y="34" width="' + p[1] + '" height="446" fill="#000" filter="url(#srcf-beam)"/><rect x="' + (p[0] + p[1]) + '" y="34" width="12" height="446" fill="#000" opacity=".3" filter="url(#src-blur3)"/>'; });
      return s + '<rect x="-300" y="468" width="1800" height="14" fill="#000" filter="url(#srcf-beam)"/><rect x="-300" y="468" width="1800" height="2" fill="' + LC + '" opacity=".25"/>';
    }
    // Floorboards running away toward the wall; each seam stops at the floor's edge so the baked image stays small.
    function floor() {
      var s = '<rect x="-300" y="480" width="1800" height="420" fill="#000" filter="url(#srcf-boards)"/>', x, i, r = rng(5);
      for (x = -300; x <= 1500; x += 46) { var bx = 600 + (x - 600) * 4.4, ex = Math.max(-300, Math.min(1500, bx)), ey = 480 + 420 * (bx === x ? 1 : (ex - x) / (bx - x)); s += '<path d="M' + x + ',480 L' + ex.toFixed(0) + ',' + ey.toFixed(0) + '" stroke="#140a04" stroke-opacity=".7" stroke-width="1.6"/>'; }
      for (i = 0; i < 16; i++) s += '<path d="M' + (r() * 1200).toFixed(0) + ',' + (486 + Math.pow(r(), 1.4) * 210).toFixed(0) + ' h' + (20 + r() * 40).toFixed(0) + '" stroke="#140a04" stroke-opacity=".6" stroke-width="1.2"/>';
      s += '<path d="M380,700 L520,480 L700,480 L860,700 Z" fill="#e8c090" opacity=".07"/>';
      s += '<path d="M560,560 L880,560 L960,660 L470,660 Z" fill="#6a2a24"/><path d="M560,560 L880,560 L960,660 L470,660 Z" fill="#000" opacity=".18" filter="url(#src-cloth)"/>' +
        '<path d="M580,570 L862,570 L934,650 L500,650 Z" fill="none" stroke="#c8a060" stroke-opacity=".5" stroke-width="2"/>';
      return '<g filter="url(#srcf-woodlit)">' + s + '</g>';
    }
    function door() {
      var s = '<g data-k="door"><rect x="54" y="96" width="212" height="384" fill="#000" opacity=".5" filter="url(#src-blur8)"/><rect x="58" y="98" width="204" height="378" fill="url(#src-frame)"/>';
      s += '<rect x="72" y="112" width="176" height="364" fill="#6a4426"/><rect x="72" y="112" width="176" height="364" fill="#000" opacity=".35" filter="url(#src-cloth)"/>';
      for (var i = 1; i < 5; i++) s += '<path d="M' + (72 + i * 35) + ',300 V476" stroke="#120a04" stroke-opacity=".7" stroke-width="2"/>';
      s += '<g clip-path="url(#srcf-paneclip)">' + skyArt('srcf-sky', 98, 148, 124, 140, R.model.time) +
        '<path d="M98,288 L98,250 L112,250 L112,240 L128,230 L144,240 L144,256 L170,256 L170,246 L188,238 L204,246 L204,262 L222,262 L222,288 Z" fill="#090d1a"/>' +
        '<g fill="#e8b860" opacity=".6"><rect x="120" y="258" width="3" height="4"/><rect x="180" y="252" width="3" height="4"/></g><rect x="98" y="148" width="124" height="140" fill="url(#src-fog)" opacity=".5"/></g>';
      s += '<rect x="157" y="148" width="7" height="140" fill="#2e1e10"/><rect x="98" y="214" width="124" height="7" fill="#2e1e10"/><rect x="92" y="142" width="136" height="152" fill="none" stroke="#2e1e10" stroke-width="8"/>';
      [330, 420].forEach(function (y) { s += '<rect x="72" y="' + y + '" width="176" height="10" fill="url(#src-iron)"/>'; });
      s += '<circle cx="226" cy="372" r="8" fill="url(#src-iron)"/><path d="M262,96 h26 v6 h-20 v8" stroke="#2a2a2a" stroke-width="4" fill="none"/>' +
        '<g class="srcf-bell"><path d="M272,110 c-10,0 -12,18 -14,22 h28 c-2,-4 -4,-22 -14,-22 Z" fill="url(#src-brass)"/><circle cx="272" cy="134" r="3" fill="url(#src-brass)"/></g>';
      return s + '</g>';
    }
    function cabinet() {
      var s = '<rect x="322" y="62" width="246" height="418" fill="#000" opacity=".5" filter="url(#src-blur14)" transform="translate(-26,6)"/>';
      s += '<rect x="330" y="62" width="230" height="412" rx="3" fill="#000" filter="url(#srcf-oak)"/><path d="M322,62 h246 v-14 l-8,-10 h-230 l-8,10 Z" fill="#000" filter="url(#srcf-oak)"/>';
      s += '<rect x="344" y="78" width="202" height="368" fill="#1a120c"/><rect x="344" y="78" width="202" height="368" fill="#3a2a1a" opacity=".5" filter="url(#src-cloth)"/>';
      [188, 300, 410].forEach(function (y) { s += '<rect x="344" y="' + (y - 2) + '" width="202" height="8" fill="#5a3a20"/><rect x="344" y="' + (y - 2) + '" width="202" height="2" fill="' + LC + '" opacity=".35"/><rect x="344" y="' + (y + 6) + '" width="202" height="8" fill="#000" opacity=".4" filter="url(#src-blur3)"/>'; });
      return '<g filter="url(#srcf-oaklit)">' + s + '<rect x="330" y="446" width="230" height="28" fill="#000" filter="url(#srcf-oak)"/></g>';
    }
    function cabGlass() {
      return '<g class="src-nohit"><rect x="344" y="78" width="202" height="368" fill="url(#srcf-glass)"/><rect x="442" y="78" width="6" height="368" fill="#2a1a0c"/>' +
        '<path d="M356,90 L392,90 L362,440 L356,440 Z" fill="#fff" opacity=".06"/><path d="M470,90 L486,90 L456,440 L450,440 Z" fill="#fff" opacity=".05"/>' +
        '<rect x="344" y="78" width="202" height="368" fill="none" stroke="#2a1a0c" stroke-width="5"/><circle cx="436" cy="262" r="3" fill="url(#src-brass)"/><circle cx="454" cy="262" r="3" fill="url(#src-brass)"/></g>';
    }
    // Spots in the cabinet with nothing for sale hold old books or a jar, not for sale.
    function cabDecor() {
      var s = '', n = R.place.cabinet.length;
      for (var i = n; i < CAB.length; i++) {
        var p = CAB[i];
        s += i % 2 ? '<rect x="' + (p[0] - 20) + '" y="' + (p[1] - 50) + '" width="40" height="50" rx="6" fill="#4a5a3a" opacity=".85"/><rect x="' + (p[0] - 20) + '" y="' + (p[1] - 50) + '" width="40" height="50" rx="6" fill="url(#src-glassR)"/><rect x="' + (p[0] - 22) + '" y="' + (p[1] - 56) + '" width="44" height="8" rx="2" fill="url(#src-cork)"/>'
          : [['#4c1f1a', 12, 50], ['#1d3a24', 10, 46], ['#1a2644', 14, 54]].map(function (b, j) { return '<rect x="' + (p[0] - 20 + j * 14) + '" y="' + (p[1] - b[2]) + '" width="' + b[1] + '" height="' + b[2] + '" fill="' + b[0] + '"/><rect x="' + (p[0] - 20 + j * 14) + '" y="' + (p[1] - b[2] + 6) + '" width="' + b[1] + '" height="2" fill="#c8a060" opacity=".6"/>'; }).join('');
      }
      return '<g class="src-nohit" opacity=".9">' + s + '</g>';
    }
    function goodAt(it, x, y, sc, sd) {
      var k = 'ware:' + it.id, left = R.stock(it), here = held && String(held.id) === String(it.id);
      if (it.out || left === 0 && !here) return '<g data-k="' + k + '" class="src-out"><rect x="' + (x - 34) + '" y="' + (y - 80) + '" width="68" height="80" fill="transparent"/><g transform="translate(' + x + ',' + (y - 16) + ') rotate(-6)"><rect x="-22" y="-8" width="44" height="14" fill="#e8dcbc"/><text x="0" y="2" text-anchor="middle" class="src-tag">' + (it.out ? 'sold out' : 'all chosen') + '</text></g></g>';
      var s = '<g data-k="' + k + '" class="src-ware"><rect x="' + (x - 34) + '" y="' + (y - 80) + '" width="68" height="80" fill="transparent"/>';
      if (!(here && left !== null && left < 2)) s += '<rect x="' + (x - 14) + '" y="' + (y - 62) + '" width="44" height="62" rx="16" fill="#000" opacity=".35" filter="url(#src-blur5)"/><g transform="translate(' + x + ',' + y + ') scale(' + sc + ')">' + ware(it, sd, R.icons) + '</g>';
      return s + '</g>';
    }
    function cabWares() { return R.place.cabinet.map(function (it, i) { return goodAt(it, CAB[i][0], CAB[i][1], 1.15, 'L'); }).join(''); }
    // Barrels of goods sold in bulk, with what is in them heaped on top and their price chalked up.
    function barrels() {
      var s = '', b = R.place.barrel;
      if (b.length) s += '<g data-k="board"><rect x="640" y="300" width="170" height="104" rx="4" fill="url(#src-frame)"/><rect x="648" y="308" width="154" height="88" fill="url(#srcf-chalk)"/>' +
        b.map(function (it, i) { return '<text x="725" y="' + (338 + i * 26) + '" text-anchor="middle" class="srcf-chalk">' + esc(short(it.n, 16)) + ' · ' + esc(short(it.price, 10)) + '</text>'; }).join('') +
        '<path d="M690,300 L725,276 L760,300" stroke="#2a2a2a" stroke-width="2" fill="none"/></g>';
      b.forEach(function (it, i) {
        var x = BARREL[i], y = BY, c = safeHex(it.c), left = R.stock(it), full = !it.out && left !== 0;
        s += '<g data-k="ware:' + it.id + '" class="' + (full ? 'src-ware' : 'src-out') + '"><ellipse cx="' + (x + 30) + '" cy="' + (y + 4) + '" rx="70" ry="12" fill="#000" opacity=".5" filter="url(#src-blur8)"/>' +
          '<path d="M' + (x - 50) + ',' + (y - 110) + ' Q' + (x - 60) + ',' + (y - 55) + ' ' + (x - 50) + ',' + y + ' Q' + x + ',' + (y + 12) + ' ' + (x + 50) + ',' + y + ' Q' + (x + 60) + ',' + (y - 55) + ' ' + (x + 50) + ',' + (y - 110) + ' Z" fill="#7a4a24"/>' +
          '<path d="M' + (x - 50) + ',' + (y - 110) + ' Q' + (x - 60) + ',' + (y - 55) + ' ' + (x - 50) + ',' + y + ' Q' + x + ',' + (y + 12) + ' ' + (x + 50) + ',' + y + ' Q' + (x + 60) + ',' + (y - 55) + ' ' + (x + 50) + ',' + (y - 110) + ' Z" fill="url(#src-sideL)"/>' +
          [y - 96, y - 14].map(function (h) { return '<path d="M' + (x - 53) + ',' + h + ' Q' + x + ',' + (h + 10) + ' ' + (x + 53) + ',' + h + '" stroke="#2a2c30" stroke-width="7" fill="none"/>'; }).join('') +
          '<ellipse cx="' + x + '" cy="' + (y - 110) + '" rx="50" ry="11" fill="#2a1608"/>' +
          (full ? '<path d="M' + (x - 46) + ',' + (y - 110) + ' q10,-26 26,-22 q12,-16 26,-4 q18,-10 24,8 q6,10 -8,16 Z" fill="' + c + '"/><path d="M' + (x - 40) + ',' + (y - 112) + ' q20,-12 40,-6 q14,-8 30,0" stroke="' + mix(c, '#000', .4) + '" stroke-width="3" fill="none"/>' +
            '<g transform="translate(' + x + ',' + (y - 40) + ')">' + label(it.ic, R.icons, 0, 0, 34, 26) + '</g>' : '<g transform="translate(' + x + ',' + (y - 50) + ') rotate(-6)"><rect x="-22" y="-8" width="44" height="14" fill="#e8dcbc"/><text x="0" y="2" text-anchor="middle" class="src-tag">' + (it.out ? 'sold out' : 'all chosen') + '</text></g>') + '</g>';
      });
      return s;
    }
    function herbs() {
      if (piece === 'anvil') return '';
      var s = '<g class="src-nohit">', r = rng(17);
      for (var i = 0; i < 7; i++) {
        var x = 600 + i * 40 + r() * 10, len = 60 + r() * 50, col = ['#5a6a3a', '#6a5a2a', '#4a5a42', '#7a3a2a'][i % 4], y0 = 50 + r() * 14;
        s += '<path d="M' + x.toFixed(0) + ',34 v' + (y0 - 34).toFixed(0) + '" stroke="#c8b898" stroke-width="1.4"/><rect x="' + (x - 4).toFixed(0) + '" y="' + y0.toFixed(0) + '" width="8" height="6" fill="#c8b898"/>';
        for (var j = 0; j < 9; j++) { var a = (j - 4) * .12; s += '<path d="M' + x.toFixed(0) + ',' + (y0 + 5).toFixed(0) + ' q' + (Math.sin(a) * len * .6).toFixed(0) + ',' + (len * .5).toFixed(0) + ' ' + (Math.sin(a) * len).toFixed(0) + ',' + len.toFixed(0) + '" stroke="' + col + '" stroke-width="2.2" fill="none" opacity=".9"/>'; }
      }
      return s + '</g>';
    }
    // The room's working piece, not for sale: a still, an anvil or a stack of crates.
    function workPiece() {
      var s = '<g data-k="piece"><ellipse cx="1010" cy="612" rx="170" ry="16" fill="#000" opacity=".5" filter="url(#src-blur8)"/>';
      if (piece === 'still') {
        s += '<rect x="880" y="470" width="270" height="18" fill="#5a3418"/><rect x="880" y="470" width="270" height="3" fill="' + LC + '" opacity=".35"/>';
        [896, 1126].forEach(function (x) { s += '<rect x="' + x + '" y="488" width="14" height="118" fill="#4a2a14"/>'; });
        s += '<path d="M930,470 l8,-30 h52 l8,30 Z" fill="url(#src-iron)"/><ellipse cx="964" cy="440" rx="26" ry="4" fill="#ff7a2a" class="srcf-coals"/>' +
          '<path d="M926,440 C914,404 930,372 964,370 C998,372 1014,404 1002,440 Z" fill="url(#srcf-copper)"/><path d="M944,372 q20,-30 40,0" fill="url(#srcf-copper)"/><path d="M964,350 v-14 q0,-12 14,-12 h60" stroke="url(#srcf-copper)" stroke-width="7" fill="none"/>';
        for (var i = 0; i < 4; i++) s += '<ellipse cx="1046" cy="' + (340 + i * 22) + '" rx="22" ry="7" fill="none" stroke="#b8642e" stroke-width="5"/>';
        s += '<path d="M1074,470 c-14,0 -20,-12 -18,-24 c2,-10 8,-14 8,-26 h12 c0,12 6,16 8,26 c2,12 -4,24 -18,24 Z" fill="#c8d8c0" opacity=".35"/>';
      } else if (piece === 'anvil') {
        s += '<path d="M940,610 l10,-80 h120 l10,80 Z" fill="#4a2a14"/><ellipse cx="1010" cy="530" rx="60" ry="10" fill="#6a4020"/>' +
          '<path d="M948,530 v-14 h-34 q30,-8 40,-30 h120 v20 q-10,10 -20,10 h-30 v14 Z" fill="url(#src-iron)"/><rect x="954" y="482" width="112" height="6" fill="#8a8e96" opacity=".6"/>' +
          '<g transform="translate(1060,470) rotate(-30)"><rect x="-4" y="-50" width="8" height="54" fill="#7a4a24"/><rect x="-16" y="-62" width="32" height="14" rx="2" fill="url(#src-iron)"/></g>';
      } else {
        [[930, 610, 120], [1060, 610, 110], [990, 520, 100]].forEach(function (c) { s += '<g transform="translate(' + c[0] + ',' + c[1] + ') scale(' + (c[2] / 48) + ')"><rect x="-24" y="-40" width="48" height="40" fill="url(#src-wood)"/><path d="M-24,-40 L24,0 M-24,0 L24,-40" stroke="#3a2212" stroke-width="2.4" opacity=".6"/><rect x="-24" y="-40" width="48" height="40" fill="none" stroke="#2a160a" stroke-width="2.4"/></g>'; });
      }
      return s + '</g>';
    }
    function lantern() {
      var x = LANT.x, y = LANT.y;
      return '<g data-k="lantern"><path d="M' + x + ',0 V' + (y - 44) + '" stroke="#2a2a2a" stroke-width="3" stroke-dasharray="6 3"/><path d="M' + (x - 18) + ',' + (y - 30) + ' h36 l-6,-14 h-24 Z" fill="url(#src-iron)"/>' +
        '<rect x="' + (x - 16) + '" y="' + (y - 30) + '" width="32" height="44" fill="#ffd890" opacity=".55"/><rect x="' + (x - 16) + '" y="' + (y - 30) + '" width="32" height="44" fill="none" stroke="#2a2a2a" stroke-width="3"/>' +
        '<path class="src-flame" d="M' + x + ',' + (y - 16) + ' c-6,8 -7,16 0,22 c7,-6 6,-14 0,-22 Z" fill="url(#src-flame)"/><path d="M' + (x - 20) + ',' + (y + 14) + ' h40 l-6,8 h-28 Z" fill="url(#src-iron)"/>' +
        '<circle cx="' + x + '" cy="' + y + '" r="120" fill="url(#src-bloom)" data-flick=".8"/></g>';
    }
    function svg() {
      return '<rect width="1200" height="700" fill="#0a0705"/><g data-depth=".6"><rect x="-300" y="460" width="1800" height="440" fill="url(#src-wallfoot)"/><g data-bake="fwall">' + wall() + '</g>' + herbs() + door() + '<g data-bake="cab">' + cabinet() + '</g>' + cabDecor() +
        '<rect data-k="cabinet" x="330" y="62" width="230" height="412" fill="transparent"/><g id="srcf-wares">' + cabWares() + '</g>' + cabGlass() + '</g>' +
        '<g data-depth=".8"><g data-bake="ffloor">' + floor() + '</g><path d="M98,288 L222,288 L420,700 L120,700 Z" fill="url(#srcf-shaft)" opacity=".5" pointer-events="none"/></g>' +
        '<g data-depth="1"><g id="srcf-barrels">' + barrels() + '</g>' + workPiece() + lantern() + '</g>' +
        '<rect data-depth=".8" x="-300" y="-200" width="1800" height="1100" fill="url(#srcf-shade)" data-flick=".98" pointer-events="none"/>' +
        // What you hold and what flies to the counter stay put when the camera moves: they are in your hands.
        '<g id="srcf-held"></g><g id="srcf-fly"></g><rect width="1200" height="700" fill="url(#srcf-vig)" pointer-events="none"/>';
    }

    var PIECE = { still: ['The still', 'Copper, older than the shop. Something is coming over, a drop at a time.'], anvil: ['The anvil', 'Scarred from years of work.'], crates: ['Crates', 'Stock waiting to be unpacked.'] };
    function info(k) {
      var p = k.split(':'), it = p[1] ? R.item(p[1]) : null;
      if ((p[0] === 'ware' || p[0] === 'held') && it) return R.info(it, p[0] === 'held' ? 'Drag it to your basket · Esc puts it back' : it.out ? '' : 'Press to pick it up');
      if (k === 'door') return { title: 'The door to the lane', body: 'The bell rings when someone comes in.', foot: 'Press to walk up to it' };
      if (k === 'board') return { title: 'The price board', body: 'Goods sold by the bundle, chalked up fresh.', foot: 'Press to walk up to the barrels' };
      if (k === 'cabinet') return { title: 'The display cabinet', body: R.place.cabinet.length ? 'The bigger goods, behind glass.' : 'Nothing for sale in it today.', foot: 'Press to walk closer' };
      if (k === 'piece') return { title: PIECE[piece][0], meta: 'Not for sale', body: PIECE[piece][1], foot: 'Press to walk closer' };
      if (k === 'lantern') return { title: 'Hanging lantern', body: 'The only warm light on this side of the shop.', foot: 'Press to turn up the wick' };
      return null;
    }
    function menu(k) {
      var p = k.split(':'), it = p[1] ? R.item(p[1]) : null;
      if (p[0] === 'held') return [['Add to my pile', addHeld, !R.canAdd(held.id)], ['Put it back', function () { putBack(); }]];
      if (p[0] === 'ware' && it) return [['Pick it up', function () { pickUp(it.id); }, it.out || R.stock(it) === 0], ['Add one to my pile', function () { take(it.id); }, !R.canAdd(it.id)], ['Put one back', function () { R.remove(it.id); }, !R.inBasket(it.id)]];
      return null;
    }
    function spotOf(id) {
      var a = null;
      R.place.cabinet.forEach(function (it, i) { if (String(it.id) === String(id)) a = [CAB[i][0], CAB[i][1], 1.15]; });
      R.place.barrel.forEach(function (it, i) { if (String(it.id) === String(id)) a = [BARREL[i], BY - 60, 1]; });
      return a || [600, 400, 1];
    }
    /* Where a good is on screen now, in scene units, with the camera's move
       counted, so a good picked up from a close-up starts where you see it. */
    function screenAt(id) {
      var e = R.svg.querySelector('[data-k="ware:' + id + '"]'), sp = spotOf(id);
      if (!e) return sp;
      var r = e.getBoundingClientRect(), q = R.toScene({ clientX: r.left + r.width / 2, clientY: r.bottom });
      return [q[0], q[1], sp[2] * R.cam().z];
    }
    // Taking something from this side: it arcs off toward the counter behind you, onto your pile.
    function take(id, x, y, sc) {
      if (!R.add(id)) return;
      var sp = spotOf(id); R.hideCard();
      x = x == null ? sp[0] : x; y = y == null ? sp[1] - 30 : y;
      flying.push({ id: id, t: 0, x0: x, y0: y, s0: sc || sp[2] }); R.pop('on your pile', x, y - 60); rewares();
    }
    /* Picking something up: it comes off the shelf into your hands, and only
       goes to your pile when you say so, by the button or by dragging it to
       the basket. Esc or "Put it back" returns it. */
    function pickUp(id) {
      if (held) putBack(true);
      var a = screenAt(id); R.hideCard();
      held = { id: id, phase: 'lift', t: 0, dur: R.calm ? .01 : .55, from: a, to: HOLD.slice(), x: a[0], y: a[1], s: a[2] };
      rewares(); hint();
    }
    function addHeld() {
      if (!held) return;
      if (!R.canAdd(held.id)) { R.pop(R.why(held.id), held.x, held.y - 120); return; }
      var h = held; held = null; take(h.id, h.x, h.y - 40, h.s); hint();
    }
    function putBack(quick) {
      if (!held) return;
      if (quick) { held = null; rewares(); return; }
      held.from = [held.x, held.y, held.s]; held.to = screenAt(held.id); held.phase = 'return'; held.t = 0; held.dur = R.calm ? .01 : .45; R.hideCard(); hint();
    }
    function hint() {
      if (held && held.phase !== 'return') {
        var it = R.item(held.id), can = R.canAdd(held.id);
        R.say('<span>You pick up the ' + esc(it ? it.n : 'good') + '.' + (can ? ' Drag it to your basket, or:' : ' ' + esc(R.why(held.id))) + '</span><span class="src-acts">' +
          (can ? '<button type="button" class="src-btn primary" data-act="take">Add to my pile</button>' : '') + '<button type="button" class="src-btn" data-act="back">Put it back</button></span>');
      } else R.say(R.place.cabinet.length + R.place.barrel.length ? '<span>Press anything for sale to pick it up. The counter is behind you.</span>' : '<span>Nothing for sale on this side. The counter is behind you.</span>');
    }
    function heldSvg() {
      if (!held) return '';
      var h = held, it = R.item(h.id); if (!it) return '';
      return '<ellipse class="src-nohit" cx="' + f1(h.x) + '" cy="' + f1(h.y + 40) + '" rx="' + f1(h.s * 30) + '" ry="' + f1(h.s * 6) + '" fill="#000" opacity="' + (h.phase === 'return' ? .1 : .28) + '" filter="url(#src-blur8)"/>' +
        '<g data-k="held:' + h.id + '" class="src-heldware' + (h.phase === 'drag' ? ' drag' : '') + '" transform="translate(' + f1(h.x) + ',' + f1(h.y) + ') scale(' + h.s.toFixed(3) + ')"><rect x="-30" y="-70" width="60" height="72" fill="transparent"/>' + ware(it, 'L', R.icons) + '</g>';
    }
    function click(k) {
      var p = k.split(':'), it = p[1] ? R.item(p[1]) : null;
      // From one close-up you can walk straight to the next.
      if (NEAR[k] && (R.cam().z < 1.01 || at !== k)) { var n = NEAR[k]; at = k; R.closer(n[0], n[1], n[2]); if (k === 'door') ringDoor(); return true; }
      if (NEAR[k]) return false;
      if (p[0] === 'held') return true;
      if (p[0] === 'ware' && it) { if (!it.out && R.stock(it) !== 0) pickUp(it.id); return true; }
      if (k === 'lantern') { R.flare(1); return true; }
      return false;
    }
    function ringDoor() { var b = R.svg.querySelector('.srcf-bell'); if (b) { b.classList.remove('ring'); void b.getBBox(); b.classList.add('ring'); } }
    function rewares() {
      var g = R.svg.querySelector('#srcf-wares'); if (g) { g.innerHTML = cabWares(); R.tab(g); }
      var b = R.svg.querySelector('#srcf-barrels'); if (b) { b.innerHTML = barrels(); R.tab(b); }
    }
    function frame(t, dt) {
      var hg = R.svg.querySelector('#srcf-held');
      if (held && (held.phase === 'lift' || held.phase === 'return')) {
        held.t += dt / held.dur;
        var k = Math.min(1, held.t), e = ease(k), f = held.from, to = held.to;
        held.x = f[0] + (to[0] - f[0]) * e; held.y = f[1] + (to[1] - f[1]) * e - Math.sin(Math.PI * k) * 40; held.s = f[2] + (to[2] - f[2]) * e;
        if (k >= 1) { if (held.phase === 'lift') held.phase = 'rest'; else { held = null; rewares(); } }
      }
      if (hg) { var hs = heldSvg(); if (hs !== lastHeld) { hg.innerHTML = hs; lastHeld = hs; R.tab(hg); } }
      var g = R.svg.querySelector('#srcf-fly'); if (!g) return;
      var out = '';
      flying = flying.filter(function (f) { f.t += dt / (R.calm ? .01 : .7); return f.t < 1; });
      flying.forEach(function (f) {
        var k = f.t, x = f.x0 + (1180 - f.x0) * k, y = f.y0 + (760 - f.y0) * k - Math.sin(Math.PI * k) * 160, sc = f.s0 * (1 + k * 1.2), it = R.item(f.id);
        if (it) out += '<g class="src-nohit" transform="translate(' + f1(x) + ',' + f1(y) + ') scale(' + sc.toFixed(2) + ')" opacity="' + (1 - k * k).toFixed(2) + '">' + ware(it, 'R', R.icons) + '</g>';
      });
      if (out !== lastFly) { g.innerHTML = out; lastFly = out; }
    }
    function afterDraw() {
      var svg = R.svg;
      svg.addEventListener('pointerdown', function (e) {
        var t = e.target.closest && e.target.closest('[data-k^="held:"]'); if (!t || !held || held.phase !== 'rest') return;
        e.preventDefault(); R.dragging = true; R.hideCard(); held.phase = 'drag';
        var q = R.toScene(e); held.dx = held.x - q[0]; held.dy = held.y - q[1];
        try { svg.setPointerCapture(e.pointerId); } catch (x) { /* a synthetic pointer has nothing to capture */ }
      });
      svg.addEventListener('pointermove', function (e) {
        if (!held || held.phase !== 'drag') return;
        var q = R.toScene(e); held.x = q[0] + held.dx; held.y = q[1] + held.dy; R.basketHot(R.overBasket(e.clientX, e.clientY));
      });
      svg.addEventListener('pointerup', function (e) {
        if (!held || held.phase !== 'drag') return;
        setTimeout(function () { R.dragging = false; }, 0); R.basketHot(false);
        if (R.overBasket(e.clientX, e.clientY)) { held.phase = 'rest'; addHeld(); }
        else { held.from = [held.x, held.y, held.s]; held.to = HOLD.slice(); held.phase = 'lift'; held.t = 0; held.dur = .3; }
      });
    }
    function act(a) { if (a === 'take') addHeld(); else if (a === 'back') putBack(); }
    function escape() { if (held && held.phase === 'rest') { putBack(); return true; } return false; }
    function sync() { rewares(); if (held && !R.item(held.id)) held = null; hint(); }
    return { id: 'floor', label: R.model.name + ', the shop floor: the door, a display cabinet and barrels', defs: defs, svg: svg, info: info, menu: menu, click: click, frame: frame, afterDraw: afterDraw, act: act, escape: escape, sync: sync, hint: hint,
      bought: function () { rewares(); }, destroy: function () {} };
  }

  /* ---- The keeper's case, opened on the counter: eight velvet hollows of
     finer goods, the shop's name on a brass plate and its monogram worked
     into the lid, lit by the counter's lamp pulled close. Hover a good and it
     lifts; press it and it comes up to you. ---- */
  function caseView(R) {
    var CANDLE = { x: 1076, y: 410 }, KEY = '#ffd2a0', TQ = [[268, 372], [932, 372], [1016, 628], [184, 628]];
    var lifted = null, up = null, last = '';
    function lerp(a, b, t) { return [a[0] + (b[0] - a[0]) * t, a[1] + (b[1] - a[1]) * t]; }
    // A point in the tray: u across (0 left, 1 right), v from back (0) to front (1).
    function at(u, v) { return lerp(lerp(TQ[0], TQ[1], u), lerp(TQ[3], TQ[2], u), v); }
    function poly(q) { return 'M' + q.map(function (p) { return f1(p[0]) + ',' + f1(p[1]); }).join(' L') + ' Z'; }
    function cell(i) {
      var c = i % 4, r = Math.floor(i / 4), u0 = .015 + c * .2475, u1 = u0 + .2325, v0 = .03 + r * .49, v1 = v0 + .45, base = at((u0 + u1) / 2, v0 + .36);
      return { q: [at(u0, v0), at(u1, v0), at(u1, v1), at(u0, v1)], x: base[0], y: base[1], s: r ? 2.05 : 1.62 };
    }
    function goods() { return R.place.case; }
    function indexOf(id) { var n = -1; goods().forEach(function (it, i) { if (String(it.id) === String(id)) n = i; }); return n; }
    function defs() {
      var L = [{ x: CANDLE.x, y: CANDLE.y, z: 260, col: KEY }];
      return litF('srcc-tablelit', [0, 0, 1200, 700], '.003 .06', 2, 31, L, '#3c4560', .42, 1.1) +
        woodF('srcc-top', 21, .0024, .05, [.14, .08, .035], [.5, .31, .16]) + woodF('srcc-case', 33, .11, .004, [.16, .07, .03], [.56, .3, .15]) + woodF('srcc-lidwood', 47, .004, .11, [.14, .06, .03], [.5, .27, .13]) +
        litF('srcc-caselit', [150, 20, 920, 690], .02, 1.6, 9, L, '#4a4f6a', .62, .8) +
        '<filter id="srcc-velv" x="0" y="0" width="1" height="1" color-interpolation-filters="sRGB"><feTurbulence type="fractalNoise" baseFrequency=".9" numOctaves="2" seed="4" result="n"/>' +
          '<feColorMatrix in="n" type="matrix" values="0 0 0 0 .08  0 0 0 0 .2  0 0 0 0 .14  0 0 0 0 1" result="c"/><feTurbulence type="fractalNoise" baseFrequency=".02" numOctaves="2" seed="8" result="m"/>' +
          '<feColorMatrix in="m" type="matrix" values="0 0 0 0 .02  0 0 0 0 .07  0 0 0 0 .05  0 0 0 -1.5 1.1" result="mm"/><feComposite in="mm" in2="c" operator="over" result="v"/><feComposite in="v" in2="SourceAlpha" operator="in"/></filter>' +
        rad('srcc-shade', shade([[0, '#fff6e6'], [.22, '#fff6e6'], [.46, '#ece4da'], [.66, '#b4b4c4'], [.84, '#5e6480'], [1, '#22263a']]), ' gradientUnits="userSpaceOnUse" cx="' + (CANDLE.x - 330) + '" cy="' + (CANDLE.y + 70) + '" r="1050" gradientTransform="translate(' + (CANDLE.x - 330) + ' ' + (CANDLE.y + 70) + ') scale(1 .72) translate(' + (-CANDLE.x + 330) + ' ' + (-CANDLE.y - 70) + ')"') +
        rad('srcc-vig', [[.5, '#000', 0], [1, '#000', .6]], ' cx=".5" cy=".5" r=".78"') + rad('srcc-hollow', [[0, '#000', 0], [.6, '#000', .15], [1, '#000', .75]], ' cx=".5" cy=".42" r=".62"') +
        lin('srcc-gilt', 0, [[0, '#6a4a18'], [.3, '#e6c178'], [.5, '#fff0c8'], [.7, '#c89a4a'], [1, '#6a4a18']]);
    }
    function table() {
      var s = '<rect x="0" y="0" width="1200" height="700" fill="#000" filter="url(#srcc-top)"/>', r = rng(7), i;
      for (i = 0; i < 9; i++) s += '<path d="M0,' + (60 + i * 76) + ' H1200" stroke="#120a04" stroke-opacity=".5" stroke-width="1.4"/>';
      for (i = 0; i < 60; i++) s += '<path d="M' + (r() * 1200).toFixed(0) + ',' + (r() * 700).toFixed(0) + ' l' + (10 + r() * 40).toFixed(0) + ',' + (r() * 2 - 1).toFixed(1) + '" stroke="#e8c090" stroke-opacity="' + (.04 + r() * .08).toFixed(2) + '" stroke-width=".8"/>';
      return '<g filter="url(#srcc-tablelit)">' + s + '</g>';
    }
    function caseArt() {
      var s = '<path d="M170,640 L150,690 L860,700 L1020,640 Z" fill="#000" opacity=".55" filter="url(#src-blur14)"/><path d="M240,372 L60,120 L160,80 L300,372 Z" fill="#000" opacity=".35" filter="url(#src-blur14)"/>';
      s += '<path d="M232,378 L196,46 L1004,46 L968,378 Z" fill="#000" filter="url(#srcc-lidwood)"/><path d="M232,378 L196,46 L1004,46 L968,378 Z" fill="#000" opacity=".25"/>';
      s += '<path d="M254,370 L224,72 L976,72 L946,370 Z" fill="#000" filter="url(#srcc-velv)"/><path d="M224,72 L976,72 L972,96 L227,96 Z" fill="#000" opacity=".35"/>';
      s += '<rect x="390" y="82" width="420" height="30" rx="3" fill="url(#srcc-gilt)"/><rect x="394" y="86" width="412" height="22" rx="2" fill="none" stroke="#6a4a18" stroke-width="1"/>' +
        '<text x="600" y="103" text-anchor="middle" font-family="\'IM Fell English\',Georgia,serif" font-size="12.5" letter-spacing="1.25" fill="#3a2408">' + esc(short(R.model.name, 40).toUpperCase()) + '</text>';
      // Baked text carries its own font: a baked layer cannot see the page's CSS.
      // The shop's initials worked into the velvet in gold thread, in a wreath of leaves.
      s += '<text x="600" y="236" text-anchor="middle" font-family="\'IM Fell English\',Georgia,serif" font-size="64" font-style="italic" fill="#c8a048" opacity=".75" letter-spacing="2.5">' + esc(initials(R.model.name)) + '</text>';
      for (var a = 0; a < 28; a++) { var t = a / 28 * Math.PI * 2, lx = 600 + Math.cos(t) * 104, ly = 214 + Math.sin(t) * 72, rot = t * 180 / Math.PI + 90 + (a % 2 ? 28 : -28); s += '<ellipse cx="' + f1(lx) + '" cy="' + f1(ly) + '" rx="3" ry="9" transform="rotate(' + rot.toFixed(0) + ' ' + f1(lx) + ' ' + f1(ly) + ')" fill="#b8903a" opacity=".55"/>'; }
      [300, 900].forEach(function (x) { s += '<rect x="' + (x - 26) + '" y="366" width="52" height="14" rx="3" fill="url(#src-brassv)"/>'; });
      s += '<path d="' + poly([[252, 360], [948, 360], [1040, 640], [160, 640]]) + '" fill="#000" filter="url(#srcc-case)"/><path d="' + poly(TQ) + '" fill="#000" filter="url(#srcc-velv)"/>';
      s += '<path d="M160,640 L1040,640 L1040,688 L160,688 Z" fill="#000" filter="url(#srcc-case)"/><rect x="160" y="640" width="880" height="3" fill="' + KEY + '" opacity=".45"/>';
      s += '<path d="M160,640 h40 v8 h-32 v40 h-8 Z M1040,640 h-40 v8 h32 v40 h8 Z" fill="url(#src-brass)"/><rect x="574" y="644" width="52" height="34" rx="4" fill="url(#src-brassv)"/><circle cx="600" cy="660" r="6" fill="#3a2a10"/>';
      return '<g filter="url(#srcc-caselit)">' + s + '</g>';
    }
    function hollows() {
      var s = '<defs>', i;
      for (i = 0; i < 8; i++) s += '<clipPath id="srcc-cl' + i + '"><path d="' + poly(cell(i).q) + '"/></clipPath>';
      s += '</defs>';
      for (i = 0; i < 8; i++) { var c = cell(i), q = c.q;
        s += '<g clip-path="url(#srcc-cl' + i + ')"><path d="' + poly(q) + '" fill="url(#srcc-hollow)"/><path d="' + poly(q) + '" fill="none" stroke="#000" stroke-opacity=".75" stroke-width="16" filter="url(#src-blur5)"/>' +
          '<path d="M' + f1(q[0][0]) + ',' + f1(q[0][1]) + ' L' + f1(q[1][0]) + ',' + f1(q[1][1]) + '" stroke="#000" stroke-opacity=".7" stroke-width="22" filter="url(#src-blur8)"/></g>'; }
      var d = '';
      [0, .2475, .495, .7425, .99].forEach(function (u0) { var a = at(u0 + .0075, 0), b = at(u0 + .0075, 1); d += '<path d="M' + f1(a[0]) + ',' + f1(a[1]) + ' L' + f1(b[0]) + ',' + f1(b[1]) + '" stroke="#2a160a" stroke-width="9"/><path d="M' + f1(a[0] + 3) + ',' + f1(a[1]) + ' L' + f1(b[0] + 3) + ',' + f1(b[1]) + '" stroke="' + KEY + '" stroke-opacity=".22" stroke-width="2"/>'; });
      [.015, .505, .99].forEach(function (v0) { var a = at(0, v0), b = at(1, v0); d += '<path d="M' + f1(a[0]) + ',' + f1(a[1]) + ' L' + f1(b[0]) + ',' + f1(b[1]) + '" stroke="#2a160a" stroke-width="' + (8 + v0 * 5).toFixed(1) + '"/><path d="M' + f1(a[0]) + ',' + f1(a[1] + 3) + ' L' + f1(b[0]) + ',' + f1(b[1] + 3) + '" stroke="' + KEY + '" stroke-opacity=".3" stroke-width="1.6"/>'; });
      return s + '<g filter="url(#srcc-caselit)">' + d + '</g>';
    }
    function tag(it, c) {
      var x = c.q[3][0] + 12, y = c.q[3][1] - 4, sc = c.s / 1.45, left = R.stock(it);
      return '<g transform="translate(' + f1(x) + ',' + f1(y) + ') scale(' + sc.toFixed(2) + ') rotate(-4)"><path d="M0,0 L94,0 L100,9 L94,18 L0,18 Z" fill="url(#src-tag)"/><circle cx="92" cy="9" r="2" fill="#3a2a1a"/>' +
        '<text x="6" y="13" class="src-hand">' + esc(it.out ? 'sold out' : short(it.price, 12) + (left === null ? '' : ' · ' + left + ' left')) + '</text></g>';
    }
    function wares() {
      var s = '';
      goods().forEach(function (it, i) {
        var c = cell(i), left = R.stock(it), lift = lifted === String(it.id) ? -10 : 0, gone = up && String(up.id) === String(it.id) && left !== null && left < 2, has = !it.out && left !== 0;
        s += '<g data-k="ware:' + it.id + '" class="srcc-ware' + (has ? '' : ' src-out') + '"><path d="' + poly(c.q) + '" fill="transparent"/>';
        if (has && !gone) s += '<ellipse cx="' + f1(c.x) + '" cy="' + f1(c.y + 2) + '" rx="' + f1(24 * c.s) + '" ry="' + f1(5 * c.s) + '" fill="#000" opacity="' + (lift ? .35 : .55) + '" filter="url(#src-blur3)"/>' +
          '<g class="srcc-lift" transform="translate(' + f1(c.x) + ',' + f1(c.y + lift) + ') scale(' + c.s + ')">' + ware(it, 'R', R.icons) + '</g>';
        else s += '<ellipse cx="' + f1(c.x) + '" cy="' + f1(c.y - 4) + '" rx="' + f1(18 * c.s) + '" ry="' + f1(6 * c.s) + '" fill="#000" opacity=".45" filter="url(#src-blur3)"/>';
        s += tag(it, c) + '</g>';
      });
      return s;
    }
    function svg() {
      return '<g data-bake="table">' + table() + '<rect x="0" y="672" width="1200" height="28" fill="#1a0e06"/><rect x="0" y="670" width="1200" height="3" fill="' + KEY + '" opacity=".4"/></g>' +
        '<g data-bake="case">' + caseArt() + '</g>' + hollows() + '<g id="srcc-wares">' + wares() + '</g>' +
        '<ellipse cx="' + CANDLE.x + '" cy="566" rx="64" ry="12" fill="url(#src-ao)"/><g transform="translate(' + CANDLE.x + ',560) scale(1.5) translate(-404,-478)">' + lampArt(404, 478) + '</g>' +
        '<circle cx="' + CANDLE.x + '" cy="' + CANDLE.y + '" r="560" fill="url(#src-glow)" data-flick=".22"/><rect width="1200" height="700" fill="url(#srcc-shade)" data-flick=".98" pointer-events="none"/>' +
        '<g id="srcc-up"></g><rect width="1200" height="700" fill="url(#srcc-vig)" pointer-events="none"/>';
    }
    function info(k) {
      var p = k.split(':'), it = p[1] ? R.item(p[1]) : null;
      if ((p[0] === 'ware' || p[0] === 'up') && it) return R.info(it, p[0] === 'up' ? 'Drag it to the basket · Esc puts it back' : it.out ? '' : 'Press to take it out');
      if (k === 'lamp') return { title: 'Oil lamp', body: 'The counter’s lamp, pulled close for the case.', foot: 'Press to turn up the wick' };
      return null;
    }
    function menu(k) {
      var p = k.split(':'), it = p[1] ? R.item(p[1]) : null;
      if (p[0] === 'ware' && it) return [['Take it out', function () { bring(it.id); }, it.out || R.stock(it) === 0], ['Put one on my pile', function () { R.add(it.id); }, !R.canAdd(it.id)]];
      if (p[0] === 'up') return [['Add to my pile', toBasket, !R.canAdd(up.id)], ['Put it back in the case', putBack]];
      return null;
    }
    function hint() {
      if (up && up.phase === 'rest') {
        var it = R.item(up.id), can = R.canAdd(up.id);
        R.say('<span>' + esc(it ? it.n + ', ' + it.price : '') + (can ? '' : '. ' + esc(R.why(up.id))) + '</span><span class="src-acts">' + (can ? '<button type="button" class="src-btn primary" data-act="take">Add to my pile</button>' : '') +
          '<button type="button" class="src-btn" data-act="back">Back in the case</button></span>');
      } else R.say('<span>Hover a good to lift it. Press it to take it out of the case.</span><span class="src-acts"><button type="button" class="src-btn" data-act="counter">Back to the counter</button></span>');
    }
    function rewares() { var g = R.svg.querySelector('#srcc-wares'); if (g) { g.innerHTML = wares(); R.tab(g); } }
    // Taking a good out: it rises from its hollow and comes up to the front, large, and turns slowly in the lamplight.
    function bring(id) {
      var c = cell(indexOf(id)); lifted = null;
      up = { id: id, phase: 'fly', t: 0, dur: R.calm ? .01 : .7, from: [c.x, c.y, c.s], to: [600, 560, 3.4], x: c.x, y: c.y, s: c.s };
      R.hideCard(); rewares();
    }
    function fly(to, phase, dur) { up.from = [up.x, up.y, up.s]; up.to = to; up.phase = phase; up.t = 0; up.dur = dur; }
    function toBasket() { if (!up) return; if (!R.canAdd(up.id)) { R.pop(R.why(up.id), 600, 420); fly([600, 560, 3.4], 'fly', .3); return; } fly([600, 780, 1.6], 'tobasket', R.calm ? .01 : .45); }
    function putBack() { if (!up) return; var c = cell(indexOf(up.id)); fly([c.x, c.y, c.s], 'return', R.calm ? .01 : .55); R.hideCard(); }
    function upSvg(t) {
      if (!up) return '';
      var u = up, it = R.item(u.id), rest = u.phase === 'rest', turn = rest ? Math.sin(t * .9) * .2 : 0, sx = Math.cos(turn) * u.s;
      if (!it) return '';
      return (rest ? '<rect class="src-nohit" width="1200" height="700" fill="#05070c" opacity=".45"/><ellipse class="src-nohit" cx="600" cy="' + f1(u.y + 10) + '" rx="' + f1(u.s * 26) + '" ry="' + f1(u.s * 5) + '" fill="#000" opacity=".5" filter="url(#src-blur8)"/><circle class="src-nohit" cx="600" cy="' + f1(u.y - u.s * 30) + '" r="' + f1(u.s * 60) + '" fill="url(#src-glow)" opacity=".28"/>' : '') +
        '<g data-k="up:' + u.id + '" class="src-heldware' + (u.phase === 'drag' ? ' drag' : '') + '" transform="translate(' + f1(u.x) + ',' + f1(u.y) + ') scale(' + sx.toFixed(3) + ',' + u.s.toFixed(3) + ')"><rect x="-30" y="-70" width="60" height="72" fill="transparent"/>' + ware(it, turn > 0 ? 'L' : 'R', R.icons) + '</g>';
    }
    function frame(t, dt) {
      var g = R.svg.querySelector('#srcc-up'); if (!g) return;
      if (up && up.phase !== 'rest' && up.phase !== 'drag') {
        up.t += dt / up.dur;
        var k = Math.min(1, up.t), e = ease(k), f = up.from, to = up.to;
        up.x = f[0] + (to[0] - f[0]) * e; up.y = f[1] + (to[1] - f[1]) * e - Math.sin(Math.PI * k) * 60; up.s = f[2] + (to[2] - f[2]) * e;
        if (k >= 1) {
          var ph = up.phase;
          if (ph === 'fly') { up.phase = 'rest'; hint(); }
          else if (ph === 'tobasket') { var id = up.id; up = null; if (R.add(id)) R.pop('on your pile', 600, 640); rewares(); hint(); }
          else { up = null; rewares(); hint(); }
        }
      }
      var out = upSvg(R.calm ? 0 : t);
      if (out !== last) { g.innerHTML = out; last = out; R.tab(g); }
    }
    function afterDraw() {
      var svg = R.svg;
      svg.addEventListener('pointerover', function (e) {
        if (R.dragging || up) return;
        var t = e.target.closest && e.target.closest('[data-k^="ware:"]'), id = t ? t.getAttribute('data-k').slice(5) : null, it = id ? R.item(id) : null;
        if (it && (it.out || R.stock(it) === 0)) id = null;
        if (id !== lifted) {
          lifted = id;
          goods().forEach(function (w, i) { var el = svg.querySelector('[data-k="ware:' + w.id + '"] .srcc-lift'); if (!el) return; var c = cell(i); el.setAttribute('transform', 'translate(' + f1(c.x) + ',' + f1(c.y + (String(w.id) === lifted ? -12 : 0)) + ') scale(' + c.s + ')'); });
        }
      });
      svg.addEventListener('pointerdown', function (e) {
        var t = e.target.closest && e.target.closest('[data-k^="up:"]'); if (!t || !up || up.phase !== 'rest') return;
        e.preventDefault(); R.dragging = true; R.hideCard(); up.phase = 'drag';
        var q = R.toScene(e); up.dx = up.x - q[0]; up.dy = up.y - q[1];
        try { svg.setPointerCapture(e.pointerId); } catch (x) { /* a synthetic pointer has nothing to capture */ }
      });
      svg.addEventListener('pointermove', function (e) {
        if (!up || up.phase !== 'drag') return;
        var q = R.toScene(e); up.x = q[0] + up.dx; up.y = q[1] + up.dy; up.s = Math.max(1.2, 3.4 - Math.max(0, 560 - up.y) / 170); R.basketHot(R.overBasket(e.clientX, e.clientY));
      });
      svg.addEventListener('pointerup', function (e) {
        if (!up || up.phase !== 'drag') return;
        setTimeout(function () { R.dragging = false; }, 0); R.basketHot(false);
        if (R.overBasket(e.clientX, e.clientY)) { up.phase = 'rest'; toBasket(); } else fly([600, 560, 3.4], 'fly', .35);
      });
    }
    function act(a) { if (a === 'take') toBasket(); else if (a === 'back') putBack(); else if (a === 'counter') R.go('counter', 'out'); }
    function click(k) {
      var p = k.split(':'), it = p[1] ? R.item(p[1]) : null;
      if (p[0] === 'ware' && it) { if (!it.out && R.stock(it) !== 0) bring(it.id); return true; }
      if (p[0] === 'up') return true;
      if (k === 'lamp') { R.flare(1); return true; }
      return false;
    }
    // Esc puts a good back first; with nothing out, it closes the case.
    function escape() { if (up && up.phase === 'rest') { putBack(); return true; } if (!up) { R.go('counter', 'out'); return true; } return false; }
    function sync() { rewares(); if (up && !R.item(up.id)) up = null; hint(); }
    return { id: 'case', label: R.model.name + ', the keeper’s case open on the counter', defs: defs, svg: svg, info: info, menu: menu, click: click, frame: frame, afterDraw: afterDraw, act: act, escape: escape, sync: sync, hint: hint,
      bought: function () { rewares(); }, destroy: function () {} };
  }

  /* ---- One shop's counter look, mounted in a host element. It owns the
     views and moves between them, the camera, the hover card, the
     right-click menu and the hint bar; each view draws its side of the shop
     and says what each piece is and does. ---- */
  function still() {
    var mr = window.MotionRest;
    try { if (matchMedia('(prefers-reduced-motion: reduce)').matches) return true; } catch (e) { /* no matchMedia: assume motion is fine */ }
    return /^(calm|off)$/.test(document.documentElement.getAttribute('data-motion') || '') || !!(mr && mr.still && mr.still());
  }
  function seedOf(t) { var h = 7; t = String(t || ''); for (var i = 0; i < t.length; i++) h = (h * 31 + t.charCodeAt(i)) | 0; return Math.abs(h) % 100000 + 1; }

  function Room(host, model, hooks) {
    var R = { dragging: false }, view = null, where = 'counter', busy = false, paused = false, raf = 0, last = 0, flare = 0, n1 = 0, flick = null;
    var hov = null, pinned = null, navs = [], backBtn = null, veilT = 0, goT = 0, arriveT = 0;
    var cam = { x: 600, y: 350, z: 1 }, camA = null, camKey = '';
    host.innerHTML = '<div class="src-stage"><div class="src-grain" aria-hidden="true"></div><div class="src-card" role="status"></div><div class="src-menu" role="menu" hidden></div><div class="src-hint" aria-live="polite"></div></div>';
    var stage = host.firstChild, card = stage.querySelector('.src-card'), menuEl = stage.querySelector('.src-menu'), hintEl = stage.querySelector('.src-hint');
    var gr = grain(); if (gr) stage.querySelector('.src-grain').style.backgroundImage = 'url(' + gr + ')';

    function setModel(m) {
      R.model = { its: m.its || [], basket: m.basket || {}, icons: m.icons || window.ShopRoomIcons || {}, image: m.image || '', name: m.name || 'Shop', roomType: m.roomType || 'general', time: m.time || '', lines: !!m.lines };
      R.place = layout(R.model.its); R.icons = R.model.icons; R.seed = seedOf(R.model.name);
    }
    setModel(model);
    R.stage = stage; R.calm = still();
    R.item = function (id) { var f = null; R.model.its.forEach(function (it) { if (String(it.id) === String(id)) f = it; }); return f; };
    R.inBasket = function (id) { return +R.model.basket[String(id)] || 0; };
    R.stock = function (it) { return it.qty === null || it.qty === undefined ? null : Math.max(0, it.qty - R.inBasket(it.id)); };
    R.canAdd = function (id) { return !!hooks.canAdd(id); };
    R.why = function (id) { return (hooks.why && hooks.why(id)) || 'That can’t go on your pile.'; };
    R.add = function (id) { if (!R.canAdd(id)) { R.pop(R.why(id), 600, 420); return false; } return hooks.add(id) !== false; };
    R.remove = function (id) { hooks.remove(id); };
    R.talk = function (el) { if (hooks.talk) hooks.talk(el); };
    R.openList = function () { if (hooks.openList) hooks.openList(); };
    R.info = function (it, foot) {
      var left = R.stock(it), inb = R.inBasket(it.id);
      return { title: it.n, meta: (it.k ? it.k + ' · ' : '') + it.price, now: it.out ? 'Sold out' : (left === null ? 'Plenty in stock' : left + ' left') + (inb ? ', ' + inb + ' on your pile' : ''), foot: foot };
    };
    R.say = function (h) { hintEl.innerHTML = h || ''; hintEl.hidden = !h; };
    R.tab = function (root) { Array.prototype.forEach.call(root.querySelectorAll('[data-k]'), function (e) { e.setAttribute('tabindex', '0'); }); };
    R.toScene = function (e) { var r = R.svg.getBoundingClientRect(); return [(e.clientX - r.left) / r.width * W, (e.clientY - r.top) / r.height * H]; };
    R.overBasket = function (cx, cy) { var b = hooks.basketEl && hooks.basketEl(); if (!b) return false; var r = b.getBoundingClientRect(); return r.width > 0 && cx > r.left - 20 && cx < r.right + 20 && cy > r.top - 20 && cy < r.bottom + 20; };
    R.basketHot = function (v) { var b = hooks.basketEl && hooks.basketEl(); if (b) b.classList.toggle('src-hot', !!v); };
    // A word that floats up from a point and fades.
    R.pop = function (text, x, y) {
      if (!R.svg) return;
      var t = document.createElementNS(NS, 'text');
      t.setAttribute('x', x); t.setAttribute('y', y); t.setAttribute('class', 'src-pop'); t.setAttribute('text-anchor', 'middle'); t.textContent = text;
      R.svg.appendChild(t); setTimeout(function () { t.remove(); }, 1300);
    };
    R.flare = function (v) { flare = Math.max(flare, v); };
    R.cam = function () { return cam; };
    R.camTo = camTo; R.closer = closer; R.stepBack = stepBack; R.go = go; R.hideCard = hideCard;

    /* The camera, for the 2.5D feel: each layer marked data-depth scales and
       shifts by its own share of the move, so near things travel further than
       the wall behind them when you step back or walk closer. */
    function camApply() {
      var key = cam.x.toFixed(1) + '|' + cam.y.toFixed(1) + '|' + cam.z.toFixed(4); if (key === camKey || !R.svg) return; camKey = key;
      var m = Math.min(1, Math.abs(cam.z - 1) / .3);
      Array.prototype.forEach.call(R.svg.querySelectorAll('[data-depth]'), function (g) {
        var d = +g.getAttribute('data-depth'), sc = 1 + (cam.z - 1) * d, cx = 600 + (cam.x - 600) * m * d, cy = 350 + (cam.y - 350) * m * d;
        g.setAttribute('transform', 'translate(600,350) scale(' + sc.toFixed(4) + ') translate(' + (-cx).toFixed(1) + ',' + (-cy).toFixed(1) + ')');
      });
    }
    function camTo(x, y, z, dur, cb) { camA = { f: { x: cam.x, y: cam.y, z: cam.z }, to: { x: x, y: y, z: z }, t: 0, dur: R.calm ? .001 : dur, cb: cb }; if (paused) camStep(1); }
    function camSet(x, y, z) { cam.x = x; cam.y = y; cam.z = z; camA = null; camKey = ''; camApply(); }
    function camStep(dt) {
      if (!camA) return;
      camA.t += dt / camA.dur;
      var k = Math.min(1, camA.t), e = ease(k), f = camA.f, to = camA.to;
      cam.x = f.x + (to.x - f.x) * e; cam.y = f.y + (to.y - f.y) * e; cam.z = f.z + (to.z - f.z) * e;
      if (k >= 1) { var cb = camA.cb; camA = null; if (cb) cb(); }
    }
    // Walking up to one part of the room, and stepping back from it.
    function closer(x, y, z) {
      hideCard(); camTo(x, y, z || 1.6, .75);
      if (!backBtn) {
        backBtn = document.createElement('button'); backBtn.type = 'button'; backBtn.className = 'src-turn src-turn-back src-stepback';
        backBtn.innerHTML = '<span aria-hidden="true">‹</span><em>Step back</em>'; backBtn.addEventListener('click', function () { stepBack(); });
        stage.appendChild(backBtn);
      }
      backBtn.hidden = false; stage.classList.add('src-close');
    }
    function stepBack(cb) { if (backBtn) backBtn.hidden = true; stage.classList.remove('src-close'); camTo(600, 350, 1, .6, function () { if (cb && R.svg && R.svg.isConnected) cb(); }); }

    // ---- Hover card and right-click menu ----
    function showCard(el, pin) {
      var inf = view && view.info(el.getAttribute('data-k')); if (!inf) { hideCard(); return; }
      card.innerHTML = '<b>' + esc(inf.title) + '</b>' + (inf.meta ? '<span class="src-meta">' + esc(inf.meta) + '</span>' : '') + (inf.body ? '<p>' + esc(inf.body) + '</p>' : '') +
        (inf.now ? '<span class="src-now">' + esc(inf.now) + '</span>' : '') + (inf.foot ? '<span class="src-foot">' + esc(inf.foot) + '</span>' : '') + (pin ? '<button class="src-x" type="button" aria-label="Close">×</button>' : '');
      var sr = stage.getBoundingClientRect(), rb = el.getBoundingClientRect();
      card.classList.add('show'); card.classList.toggle('pinned', !!pin);
      var cw = card.offsetWidth, ch = card.offsetHeight, x = rb.right - sr.left + 12, y = rb.top - sr.top;
      if (x + cw > sr.width - 8) x = rb.left - sr.left - cw - 12;
      if (x < 8) x = 8; if (y + ch > sr.height - 8) y = sr.height - ch - 8; if (y < 8) y = 8;
      card.style.left = x + 'px'; card.style.top = y + 'px';
      if (pin) { pinned = el; card.querySelector('.src-x').addEventListener('click', hideCard); }
    }
    function hideCard() { card.classList.remove('show', 'pinned'); pinned = null; }
    function closeMenu() { menuEl.hidden = true; }
    function openMenu(el, e) {
      var k = el.getAttribute('data-k'), items = view && view.menu ? view.menu(k) : null, inf = view && view.info(k);
      if (!items || !items.length) return false;
      hideCard();
      menuEl.innerHTML = '<div class="src-mhead">' + esc(inf ? inf.title : '') + '</div>';
      items.forEach(function (it) {
        var b = document.createElement('button'); b.type = 'button'; b.setAttribute('role', 'menuitem'); b.textContent = it[0]; if (it[2]) b.disabled = true;
        b.addEventListener('click', function () { closeMenu(); it[1](); }); menuEl.appendChild(b);
      });
      menuEl.hidden = false;
      var sr = stage.getBoundingClientRect(), x, y;
      if (e && e.clientX) { x = e.clientX - sr.left; y = e.clientY - sr.top; } else { var bb = el.getBoundingClientRect(); x = bb.left + bb.width / 2 - sr.left; y = bb.top + bb.height / 2 - sr.top; }
      x = Math.min(x, sr.width - menuEl.offsetWidth - 8); y = Math.min(y, sr.height - menuEl.offsetHeight - 8);
      menuEl.style.left = Math.max(8, x) + 'px'; menuEl.style.top = Math.max(8, y) + 'px';
      var f = menuEl.querySelector('button:not([disabled])'); if (f) f.focus({ preventScroll: true });
      return true;
    }
    function target(e) { var t = e.target && e.target.closest ? e.target.closest('[data-k]') : null; return t && R.svg && R.svg.contains(t) ? t : null; }
    function press(t, e) { if (!view.click(t.getAttribute('data-k'), t, e)) showCard(t, true); }
    function onMove(e) {
      if (R.dragging || busy) return;
      var t = target(e);
      if (t !== hov) { if (hov) hov.classList.remove('src-hov'); hov = t; if (t) { t.classList.add('src-hov'); if (!pinned) showCard(t); } else if (!pinned) hideCard(); }
    }
    function onLeave() { if (hov) hov.classList.remove('src-hov'); hov = null; if (!pinned) hideCard(); }
    function onClick(e) {
      var b = e.target.closest && e.target.closest('[data-act]');
      if (b && stage.contains(b)) { if (view && view.act) view.act(b.getAttribute('data-act')); return; }
      if (busy || !view || !R.svg || !R.svg.contains(e.target)) return;
      var t = target(e); closeMenu();
      if (!t) { hideCard(); if (cam.z > 1.01 && !R.dragging) stepBack(); return; }
      if (R.dragging) return;
      press(t, e);
    }
    function onContext(e) { var t = target(e); if (!t) return; e.preventDefault(); openMenu(t, e); }
    function onKey(e) {
      var t = target(e); if (!t) return;
      if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); press(t, e); }
      else if (e.key === 'ContextMenu' || (e.shiftKey && e.key === 'F10')) { e.preventDefault(); openMenu(t); }
    }
    function onFocus(e) { var t = target(e); if (t && !pinned) showCard(t); }
    function onDocDown(e) { if (!menuEl.hidden && !menuEl.contains(e.target)) closeMenu(); }
    // Arrow keys turn between the counter and the shop floor while the shop is open.
    function onDocKey(e) {
      if (busy || paused || !host.isConnected || !host.offsetParent) return;
      var t = e.target; if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.tagName === 'SELECT' || t.isContentEditable)) return;
      if (e.key === 'ArrowLeft' && where === 'counter') { e.preventDefault(); go('floor', 'left'); }
      else if (e.key === 'ArrowRight' && where === 'floor') { e.preventDefault(); go('counter', 'right'); }
    }
    stage.addEventListener('pointermove', onMove); stage.addEventListener('pointerleave', onLeave); stage.addEventListener('click', onClick);
    stage.addEventListener('contextmenu', onContext); stage.addEventListener('keydown', onKey); stage.addEventListener('focusin', onFocus);
    document.addEventListener('pointerdown', onDocDown, true); document.addEventListener('keydown', onDocKey);

    var VIEWS = { counter: counterView, floor: floorView, 'case': caseView };
    function mountView(which) {
      if (view && view.destroy) view.destroy();
      where = which; R.dragging = false; hov = null; hideCard(); closeMenu();
      view = VIEWS[which](R);
      var defs = common() + view.defs(), s = document.createElementNS(NS, 'svg');
      s.setAttribute('viewBox', '0 0 ' + W + ' ' + H); s.setAttribute('class', 'src-svg'); s.setAttribute('role', 'img'); s.setAttribute('aria-label', view.label);
      s.innerHTML = '<defs>' + defs + '</defs>' + view.svg();
      if (R.svg) R.svg.remove();
      stage.insertBefore(s, stage.firstChild); R.svg = s; R.tab(s); flick = null; camKey = '';
      /* A first visit takes a moment to light; until it is ready the room
         stays dark rather than showing the slow unlit drawing, then fades up. */
      stage.classList.add('src-baking'); clearTimeout(veilT);
      veilT = setTimeout(function () { if (stage.classList.contains('src-baking')) stage.classList.add('src-veil'); }, 60);
      bake(s, defs, function () { if (R.svg !== s) return; clearTimeout(veilT); stage.classList.remove('src-baking', 'src-veil'); prebake(); });
      view.afterDraw(); view.hint(); addNav(); camApply();
    }
    /* Once the first view is lit, the other views' lighting is baked out of
       sight, one at a time, so turning round or opening the case never waits
       on the lamps. Baked layers hold no goods, so the cache stays good. */
    var pre = [], preT = 0;
    function prebake() {
      if (!pre) return;
      if (!pre.length) pre = ['floor', 'case'].filter(function (w) { return w !== where && (w !== 'case' || R.place.case.length); });
      var w = pre.shift(); if (!w) { pre = null; return; }
      preT = setTimeout(function () {
        if (paused || !host.isConnected) { pre.unshift(w); return; }
        var v = VIEWS[w](R), defs = common() + v.defs(), box = document.createElement('div'), s = document.createElementNS(NS, 'svg');
        box.setAttribute('aria-hidden', 'true'); box.style.cssText = 'position:absolute;left:-20000px;top:0;width:1200px;height:700px;visibility:hidden;pointer-events:none';
        s.setAttribute('viewBox', '0 0 ' + W + ' ' + H); s.setAttribute('width', W); s.setAttribute('height', H);
        s.innerHTML = '<defs>' + defs + '</defs>' + v.svg(); box.appendChild(s); stage.appendChild(box);
        if (v.destroy) v.destroy();
        bake(s, defs, function () { box.remove(); if (pre && pre.length) prebake(); else pre = null; });
      }, 250);
    }
    // Buttons on the edges of the view for the ways out of it.
    function addNav() {
      navs.forEach(function (b) { b.remove(); }); navs = [];
      if (backBtn) backBtn.hidden = true; stage.classList.remove('src-close');
      var b = where === 'counter' ? ['left', 'floor', 'Turn to the shop floor'] : where === 'floor' ? ['right', 'counter', 'Turn to the counter'] : ['back', 'counter', 'Back to the counter'];
      var e = document.createElement('button'); e.type = 'button'; e.className = 'src-turn src-turn-' + b[0]; e.setAttribute('aria-label', b[2]);
      e.innerHTML = '<span aria-hidden="true">' + (b[0] === 'right' ? '›' : '‹') + '</span><em>' + b[2] + '</em>';
      e.addEventListener('click', function () { go(b[1], b[0] === 'back' ? 'out' : b[0]); });
      stage.appendChild(e); navs.push(e);
    }
    /* Moving between views. Turning is a 2.5D move: step back from the wall,
       swing round, walk up to the other one. The case zooms in from where it
       sits on the counter, and out again. */
    function go(which, how) {
      if (busy || which === where) return;
      busy = true; hideCard(); closeMenu();
      var side = how === 'left' || how === 'right', s = R.svg;
      if (side) { s.style.transform = 'none'; camTo(how === 'left' ? 160 : 1040, 350, .86, .45); }
      else { s.style.transformOrigin = how === 'in' ? '27.5% 69%' : '50% 50%'; s.style.transform = how === 'out' ? 'scale(.82)' : 'scale(1.5)'; }
      stage.classList.add('src-leave');
      goT = setTimeout(function () {
        stage.classList.remove('src-leave');
        mountView(which);
        if (side) { camSet(how === 'left' ? 1040 : 160, 350, .86); camTo(600, 350, 1, .6); } else camSet(600, 350, 1);
        stage.classList.add(side ? 'src-fadein' : 'src-arrive');
        arriveT = setTimeout(function () { stage.classList.remove('src-arrive', 'src-fadein'); busy = false; }, R.calm ? 0 : 520);
      }, R.calm ? 0 : 420);
    }
    function loop(now) {
      if (paused) return;
      raf = requestAnimationFrame(loop);
      var t = now / 1000, raw = last ? t - last : 0, dt = Math.min(.05, raw); last = t;
      if (!view || !R.svg) return;
      /* Every lamp's light breathes together with its flame. The flicker is
         tuned per sixtieth of a second and scaled by the frame's length, so a
         120 or 144 Hz screen does not flicker twice as fast. */
      var k = dt * 60;
      flare = Math.max(0, flare - .012 * k); n1 += (Math.random() - .5) * .25 * Math.sqrt(k); n1 *= Math.pow(.92, k);
      var f = R.calm ? 1 : (.9 + n1 * .25 + Math.sin(t * 7.3) * .03 + Math.sin(t * 13.1) * .02) * (1 + flare * .8);
      flick = flick || R.svg.querySelectorAll('[data-flick]');
      for (var i = 0; i < flick.length; i++) { var e = flick[i]; if (!e.isConnected) { flick = null; break; } e.setAttribute('opacity', Math.min(1, +e.getAttribute('data-flick') * f).toFixed(3)); }
      // Uncapped so a slow first frame, while the lamps bake, does not leave the camera crawling.
      camStep(Math.min(.2, raw)); camApply();
      if (view.frame) view.frame(t, dt);
    }
    function pause(on) {
      paused = !!on; cancelAnimationFrame(raf); raf = 0; last = 0;
      if (!paused) { R.calm = still(); raf = requestAnimationFrame(loop); if (pre && pre.length) prebake(); }
    }
    mountView('counter'); pause(false);
    return {
      update: function (m) { setModel(m); if (view) view.sync(); },
      escape: function () {
        if (!menuEl.hidden) { closeMenu(); return true; }
        if (pinned) { hideCard(); return true; }
        if (busy) return true;
        if (view && view.escape()) return true;
        if (cam.z > 1.01) { stepBack(); return true; }
        return false;
      },
      pause: pause,
      bought: function (n) { if (view && view.bought) view.bought(n || 1); },
      where: function () { return where; },
      go: go,
      destroy: function () {
        pause(true); clearTimeout(veilT); clearTimeout(goT); clearTimeout(preT); pre = null; clearTimeout(arriveT); if (view && view.destroy) view.destroy();
        document.removeEventListener('pointerdown', onDocDown, true); document.removeEventListener('keydown', onDocKey);
        host.innerHTML = '';
      }
    };
  }

  function render(host, model, hooks) {
    injectStyle();
    if (host._counter) { host._counter.update(model); return host._counter; }
    host._counter = Room(host, model, hooks || {});
    return host._counter;
  }
  function call(host, fn, a) { var c = host && host._counter; return c ? c[fn](a) : false; }
  window.ShopRoomCounter = {
    render: render, layout: layout, family: family, floorPiece: floorPiece, shade: shade, initials: initials, safeUrl: safeUrl, ware: ware, CAP: CAP,
    sync: function (host, model) { if (host && host._counter) host._counter.update(model); },
    escape: function (host) { return !!call(host, 'escape'); },
    pause: function (host, on) { call(host, 'pause', on); },
    bought: function (host, n) { call(host, 'bought', n); },
    destroy: function (host) { if (host && host._counter) { host._counter.destroy(); delete host._counter; } }
  };
})();
