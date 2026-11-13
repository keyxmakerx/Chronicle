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
 * The counter view is composed from a theme (skeleton, theme, props) saved in
 * the room layout's `counter` field; with none, the starter for the shop's
 * type. shop_room_builder.js is where the owner edits it.
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
  function esc(t) { return String(t == null ? '' : t).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;'); }
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
     an image and swapped in. If a browser refuses, the live drawing stays,
     only slower to repaint. The wait for a room is the cost of that drawing,
     and it grows with the number of pixels, so a layer is first drawn at the
     scene's own size and shown, then drawn again at twice that out of sight
     and swapped in. The sharp images are kept in memory by their source, so
     turning back to a view already seen is instant, and in the browser's
     cache storage, so the next visit to the same room does not draw at all. */
  var BAKED = {}, SHARP = {}, LIVE = {}, sharpQ = [], sharpBusy = false;
  var BAKE_STORE = 'chronicle-shop-room-v1', BAKE_KEEP = 24;
  // Names a layer by what it is drawn from, so changed art never finds an old picture.
  function bakeKey(s) {
    var a = 0x811c9dc5, b = 0x1b873593, i, c;
    for (i = 0; i < s.length; i++) { c = s.charCodeAt(i); a = Math.imul(a ^ c, 0x01000193); b = Math.imul(b + c, 0x5bd1e995) ^ (b >>> 13); }
    return '/shop-room-bake/' + (a >>> 0).toString(16) + (b >>> 0).toString(16) + s.length.toString(16);
  }
  /* Cache storage exists only on a secure origin, and a browser may refuse it
     in a private window; a room then draws on every visit, as it always did. */
  function bakeStore() { try { return window.caches ? caches.open(BAKE_STORE) : Promise.reject(); } catch (e) { return Promise.reject(e); } }
  function kept(key) { return bakeStore().then(function (c) { return c.match(key); }).then(function (r) { return r ? r.blob() : Promise.reject(); }); }
  // Keeps the newest BAKE_KEEP layers: a few rooms' worth, so the store cannot grow without end.
  function keep(key, blob) {
    bakeStore().then(function (c) {
      return c.put(key, new Response(blob, { headers: { 'Content-Type': blob.type || 'image/png' } })).then(function () { return c.keys(); }).then(function (ks) {
        return Promise.all(ks.slice(0, Math.max(0, ks.length - BAKE_KEEP)).map(function (k) { return c.delete(k); }));
      });
    }).catch(function () { /* a full or refused store only means the next visit draws again */ });
  }
  /* Lights and encodes one baked image off the page's thread: a canvas's
     toBlob holds the page up to two seconds for the largest layers, and
     drawing the SVG straight into a canvas paints its filters on the page's
     thread. Rejects where a browser lacks a step, and the canvas way is used. */
  function offBake(img) {
    if (!window.createImageBitmap || !window.OffscreenCanvas || !OffscreenCanvas.prototype.convertToBlob) return Promise.reject();
    return createImageBitmap(img).then(function (bmp) {
      var oc = new OffscreenCanvas(bmp.width, bmp.height);
      oc.getContext('2d').drawImage(bmp, 0, 0); bmp.close();
      // WebP where the browser can write it: a layer is a fifth the size or less, which matters once it is kept. Others answer with PNG.
      return oc.convertToBlob({ type: 'image/webp', quality: 0.92 });
    });
  }
  // Draws one layer at k times the scene's size. Gives { blob }, or { url } of the undrawn SVG where no canvas will take it.
  function bakeAt(L, k) {
    return new Promise(function (ok, no) {
      var img = new Image();
      img.onload = function () {
        offBake(img).then(function (b) { ok({ blob: b }); }, function () {
          try { var cv = document.createElement('canvas'); cv.width = L.w * k; cv.height = L.h * k; cv.getContext('2d').drawImage(img, 0, 0); cv.toBlob(function (b) { ok(b ? { blob: b } : { url: img.src }); }); }
          catch (e) { ok({ url: img.src }); }
        });
      };
      img.onerror = no;
      img.src = 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent('<svg xmlns="' + NS + '" viewBox="' + L.x + ' ' + L.y + ' ' + L.w + ' ' + L.h + '" width="' + L.w * k + '" height="' + L.h * k + '">' + L.body + '</svg>');
    });
  }
  /* The sharp drawings, one at a time and only when the page is idle, so a
     visitor already looking round the room is not held up by them. */
  function sharpen() {
    if (sharpBusy) return;
    var L = sharpQ.shift(); if (!L) return;
    sharpBusy = true;
    function next() { sharpBusy = false; sharpen(); }
    function go() {
      bakeAt(L, 2).then(function (r) {
        if (!r.blob) return;
        var u = URL.createObjectURL(r.blob), old = BAKED[L.id], ims = LIVE[L.id] || [], pre = new Image();
        BAKED[L.id] = u; SHARP[L.id] = true; delete LIVE[L.id]; keep(L.key, r.blob);
        function put() {
          ims.forEach(function (im) { if (im.isConnected) im.setAttribute('href', u); });
          if (old && old.indexOf('blob:') === 0) setTimeout(function () { URL.revokeObjectURL(old); }, 5000);
        }
        // Decoded first, so the swap never shows a blank layer.
        pre.src = u; (pre.decode ? pre.decode() : Promise.reject()).then(put, put);
      }).then(next, next);
    }
    if (window.requestIdleCallback) requestIdleCallback(go, { timeout: 1500 }); else setTimeout(go, 200);
  }
  function bake(svg, defs, done) {
    var groups = svg.querySelectorAll('[data-bake]'), left = groups.length;
    function one() { if (!--left) { if (done) done(); sharpen(); } }
    if (!left && done) done();
    Array.prototype.forEach.call(groups, function (g) {
      var bb; try { bb = g.getBBox(); } catch (e) { bb = null; }
      if (!bb || !bb.width) { one(); return; }
      var p = 24, L = { x: Math.floor(bb.x - p), y: Math.floor(bb.y - p), w: Math.ceil(bb.width + 2 * p), h: Math.ceil(bb.height + 2 * p), body: '<defs>' + defs + '</defs>' + g.innerHTML };
      L.id = L.x + ' ' + L.y + ' ' + L.w + ' ' + L.h + L.body; L.key = bakeKey(L.id);
      function swap(u) {
        BAKED[L.id] = u; if (!g.isConnected) { one(); return; }
        var im = document.createElementNS(NS, 'image');
        [['href', u], ['x', L.x], ['y', L.y], ['width', L.w], ['height', L.h], ['preserveAspectRatio', 'none']].forEach(function (a) { im.setAttribute(a[0], a[1]); });
        im.addEventListener('load', function () { Array.prototype.slice.call(g.childNodes).forEach(function (n) { if (n !== im) g.removeChild(n); }); one(); });
        im.addEventListener('error', one);
        if (!SHARP[L.id]) (LIVE[L.id] = LIVE[L.id] || []).push(im);
        g.appendChild(im);
      }
      if (BAKED[L.id]) { swap(BAKED[L.id]); return; }
      kept(L.key).then(function (b) { SHARP[L.id] = true; swap(URL.createObjectURL(b)); }, function () {
        bakeAt(L, 1).then(function (r) { swap(r.blob ? URL.createObjectURL(r.blob) : r.url); if (r.blob) sharpQ.push(L); }, one);
      });
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
    // Today's shelf: chalk and paper tags in the Customize page's Kalam file.
    '@font-face{font-family:"Shop Kalam";src:url(/static/fonts/customize/kalam-latin-400.woff2) format("woff2");font-weight:400;font-style:normal;font-display:swap}',
    '.src-todayt{font-family:"Shop Kalam",cursive;font-size:11px;fill:#7a2a1a}',
    '.src-chalk{font-family:"Shop Kalam",cursive;font-size:16px;fill:#f1ede2}.src-chalk2{font-size:12px;fill:#f2c27a}',
    '.src-slate{cursor:pointer}.src-today{animation:src-sway 4.5s ease-in-out infinite}',
    '@keyframes src-sway{0%,100%{transform:rotate(-3deg)}50%{transform:rotate(3deg)}}',
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
    // The composed room's props, and the dashed slots shown while placing them.
    '@keyframes cm-spark{0%{opacity:0;transform:translate(0,0)}10%{opacity:1}70%{opacity:.8}100%{opacity:0;transform:translate(var(--dx,10px),-190px)}}',
    '.cm-spark{animation:cm-spark 3.4s ease-out infinite;opacity:0}',
    '@keyframes cm-pulse{0%,100%{opacity:.72;transform:scale(.98)}50%{opacity:1;transform:scale(1.03)}}',
    '.cm-pulse{transform-box:fill-box;transform-origin:50% 70%;animation:cm-pulse 3.8s ease-in-out infinite}',
    '@keyframes cm-rain{from{transform:translate(0,-80px)}to{transform:translate(-14px,100px)}}',
    '.cm-rain{animation:cm-rain .7s linear infinite}.cm-rain.slow{animation-duration:1.1s}',
    '@keyframes cm-drip{0%{transform:translateY(0);opacity:0}20%{opacity:.9}100%{transform:translateY(46px);opacity:0}}',
    '.cm-drip{animation:cm-drip 1.5s ease-in infinite}',
    '@keyframes cm-bob{0%,100%{transform:translateY(0)}50%{transform:translateY(-9px)}}',
    '.cm-bob{animation:cm-bob 6s ease-in-out infinite}',
    '@keyframes cm-spin{to{transform:rotate(360deg)}}',
    '.cm-orb{transform-box:view-box;transform-origin:0 0;animation:cm-spin 40s linear infinite}',
    '@keyframes cm-mote{0%,100%{opacity:.2;transform:translate(0,0)}50%{opacity:1;transform:translate(6px,-12px)}}',
    '.cm-mote{animation:cm-mote 8s ease-in-out infinite}',
    '@keyframes cm-sway{0%,100%{transform:rotate(-2.5deg)}50%{transform:rotate(2.5deg)}}',
    '.cm-sway{animation:cm-sway 5s ease-in-out infinite}',
    '@keyframes cm-steam{0%{opacity:0;transform:translate(0,0) scale(.7)}25%{opacity:.4}100%{opacity:0;transform:translate(8px,-46px) scale(1.5)}}',
    '.cm-steam{transform-box:fill-box;transform-origin:50% 100%;animation:cm-steam 5s ease-out infinite;opacity:0}',
    '@keyframes cm-bub{0%{transform:translateY(0);opacity:0}30%{opacity:.9}100%{transform:translateY(-26px);opacity:0}}',
    '.cm-bub{animation:cm-bub 2.6s ease-in infinite}',
    '.cm-slot{fill:rgb(255 255 255/.06);stroke:#ffd77a;stroke-width:2;stroke-dasharray:7 5;cursor:copy}',
    '.cm-slot.cm-on{fill:rgb(255 215 122/.28);stroke-width:3}',
    '.cm-slotl{font:600 11px Inter,system-ui,sans-serif;fill:#ffd77a;pointer-events:none}',
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

  /* ---- The composed counter room. One counter view whose room is built
     from three layers read from a plain theme: the skeleton (back wall,
     opening, how goods are displayed, the counter), the look (wall material,
     colours, light, what is outside) and props that each know their own slot.
     Every part is a small function keyed by an option value, so a new option
     is one new function. A bad or old theme falls back to defaults. ---- */
  var CM_OPT = {
    shape: ['square', 'round', 'open-air', 'alcove'],
    opening: ['none', 'window-left', 'window-right', 'window-centre', 'round-window', 'doorway'],
    display: ['shelves', 'pegboard', 'drawers', 'pigeonholes', 'domes'],
    counter: ['straight', 'curved', 'stall-table'],
    wall: ['plaster', 'stone', 'timber', 'brick', 'canvas', 'night-sky'],
    light: ['lamp', 'forge', 'candles', 'lanterns', 'moonlight'],
    outside: ['day', 'dusk', 'night', 'rain']
  };
  var CM_PROPS = ['anvil', 'alembic', 'jars', 'crystal', 'herbs', 'floating-books', 'orrery', 'ladder', 'banner', 'sconce', 'sparks', 'lanterns', 'rain', 'steam-cart', 'motes'];
  var CM_DEFAULT = { name: 'Plain counter', version: 1,
    skeleton: { shape: 'square', opening: 'window-right', display: 'shelves', counter: 'straight' },
    theme: { wall: 'plaster', wood: '#6b4527', cloth: '#5a2024', accent: '#e0a83a', trim: '#c89a4a', light: 'lamp', warmth: .7, outside: 'night' }, props: [] };
  // The starters: the plain counter is the look from before themes, and a shop with no saved theme begins from the one that suits its type.
  var CM_STARTERS = [CM_DEFAULT,
    { name: 'The Smithy', version: 1, skeleton: { shape: 'square', opening: 'window-right', display: 'pegboard', counter: 'straight' },
      theme: { wall: 'stone', wood: '#4a3220', cloth: '#5a3a22', accent: '#ff9a50', trim: '#9aa0a8', light: 'forge', warmth: .9, outside: 'dusk' }, props: ['anvil', 'sparks'] },
    { name: 'The Apothecary', version: 1, skeleton: { shape: 'square', opening: 'window-right', display: 'drawers', counter: 'straight' },
      theme: { wall: 'plaster', wood: '#7a5230', cloth: '#3a5a3a', accent: '#3fae6a', trim: '#c89a4a', light: 'lamp', warmth: .75, outside: 'dusk' }, props: ['herbs', 'alembic', 'jars'] },
    { name: 'The Arcane Shop', version: 1, skeleton: { shape: 'round', opening: 'round-window', display: 'pigeonholes', counter: 'curved' },
      theme: { wall: 'night-sky', wood: '#4a3a5a', cloth: '#4a2a6a', accent: '#8ab4ff', trim: '#d9b25a', light: 'candles', warmth: .45, outside: 'night' }, props: ['orrery', 'floating-books', 'crystal', 'ladder'] },
    { name: 'The Night Market', version: 1, skeleton: { shape: 'open-air', opening: 'none', display: 'domes', counter: 'stall-table' },
      theme: { wall: 'brick', wood: '#4a2a20', cloth: '#8a2a2e', accent: '#d9a441', trim: '#d9a441', light: 'lanterns', warmth: .8, outside: 'rain' }, props: ['rain', 'steam-cart', 'motes'] }];
  var CM_START_FOR = { forge: 1, apothecary: 2, library: 3, market: 4 };
  function starterFor(type) { return CM_STARTERS[Object.prototype.hasOwnProperty.call(CM_START_FOR, type) ? CM_START_FOR[type] : 0]; }
  function cmPick(v, list, d) { return list.indexOf(v) >= 0 ? v : d; }
  function cmHex(v, d) { return /^#[0-9a-f]{6}$/i.test(String(v || '')) ? String(v).toLowerCase() : d; }
  // Every field has a default and unknown values fall back, so a bad theme never breaks the room.
  function cmNorm(raw) {
    raw = raw && typeof raw === 'object' ? raw : {};
    var sk = raw.skeleton && typeof raw.skeleton === 'object' ? raw.skeleton : {}, th = raw.theme && typeof raw.theme === 'object' ? raw.theme : {}, D = CM_DEFAULT, w = +th.warmth;
    var props = [];
    (Array.isArray(raw.props) ? raw.props : []).forEach(function (p) { if (CM_PROPS.indexOf(p) >= 0 && props.indexOf(p) < 0) props.push(p); });
    return { name: String(raw.name || D.name).slice(0, 60), version: 1,
      skeleton: { shape: cmPick(sk.shape, CM_OPT.shape, D.skeleton.shape), opening: cmPick(sk.opening, CM_OPT.opening, D.skeleton.opening), display: cmPick(sk.display, CM_OPT.display, D.skeleton.display), counter: cmPick(sk.counter, CM_OPT.counter, D.skeleton.counter) },
      theme: { wall: cmPick(th.wall, CM_OPT.wall, D.theme.wall), wood: cmHex(th.wood, D.theme.wood), cloth: cmHex(th.cloth, D.theme.cloth), accent: cmHex(th.accent, D.theme.accent), trim: cmHex(th.trim, D.theme.trim),
        light: cmPick(th.light, CM_OPT.light, D.theme.light), warmth: w >= 0 && w <= 1 ? w : D.theme.warmth, outside: cmPick(th.outside, CM_OPT.outside, D.theme.outside) }, props: props };
  }
  function cmRgb(c, k) { var a = hex(c); return a.map(function (v) { return Math.min(1, v / 255 * k); }); }
  // The room's one real light: base colour, glow, ambient and how far it reaches.
  var CM_LIGHT = {
    lamp: { base: '#ffd2a0', glow: '#ffc478', amb: '#3e4a74', reach: 940, k: 1 },
    forge: { base: '#ff9a50', glow: '#ff8a30', amb: '#352c3a', reach: 1020, k: 1 },
    candles: { base: '#ffc880', glow: '#ffbe6a', amb: '#343a62', reach: 820, k: .9 },
    lanterns: { base: '#ff9468', glow: '#ff7a4a', amb: '#2c3254', reach: 1120, k: .95 },
    moonlight: { base: '#a8bcff', glow: '#9ab4ff', amb: '#303c68', reach: 1320, k: .85 }
  };
  function fm(s, o) { return s.replace(/\{(\w+)\}/g, function (m, k) { var v = o[k]; return typeof v === 'number' ? f1(v) : v; }); }

  /* Everything a part needs, worked out once from the theme: mirrored
     layout (a left window puts the goods on the right), the light's place and
     colour, the wood and metal tones, and where each prop landed. */
  function cmCtx(raw) {
    var n = cmNorm(raw), sk = n.skeleton, th = n.theme, t = { n: n, sk: sk, th: th, props: n.props };
    t.mir = sk.opening === 'window-left' && sk.shape !== 'open-air';
    t.mx = function (x) { return t.mir ? 1200 - x : x; };
    t.air = sk.shape === 'open-air'; t.domes = sk.display === 'domes';
    var L = CM_LIGHT[th.light], w = th.warmth;
    t.lc = th.light === 'moonlight' ? mix(L.base, '#ffd8b0', w * .4) : mix(mix(L.base, '#c0d0ff', .55), L.base, w);
    t.amb = mix(L.amb, '#4a3a4c', w * .4); t.L = L;
    t.time = { day: '', dusk: 'dusk', night: 'night', rain: 'rain' }[th.outside];
    // Where the light sits: on the counter, hung from the ceiling over a stall, in the forge's mouth, or from above through the opening.
    var lx = t.mx(404), ly = 372;
    if (th.light === 'forge') { lx = 641; ly = 400; }
    else if (th.light === 'lanterns') { lx = 641; ly = 140; }
    else if (th.light === 'moonlight') { lx = t.mx(sk.opening === 'none' || t.air ? 900 : sk.opening === 'window-centre' ? 641 : 912); ly = 150; }
    else if (t.domes) { lx = t.mx(800); ly = 220; }
    t.lp = { x: lx, y: ly, z: th.light === 'lanterns' ? 240 : 300 };
    t.pend = t.domes && (th.light === 'lamp' || th.light === 'candles');
    t.ink = mix(t.amb, '#000', .62);
    var w0 = mix(t.lc, '#ffffff', .8);
    t.shadeStops = [[0, w0], [.16, w0], [.38, mix(w0, t.ink, .18)], [.58, mix(w0, t.ink, .5)], [.8, mix(w0, t.ink, .78)], [1, t.ink]];
    t.w0 = cmRgb(th.wood, .28); t.w1 = cmRgb(th.wood, 1);
    t.side = t.mir ? 'L' : 'R';
    return t;
  }

  /* Filters and gradients every part of the composed room shares. Literal
     colours only: baked layers are drawn as standalone images. */
  function cmDefs(t) {
    var th = t.th, L = t.lp, lights = [{ x: L.x, y: L.y, z: L.z, col: t.lc }], tr = th.trim, acc = th.accent, sc = th.cloth, wallFq = { plaster: [.03, 3.2, 4], stone: [.05, 4.5, 8], timber: ['.07 .004', 2.2, 6], brick: [.08, 3, 12], canvas: ['.35 .05', 1.1, 5], 'night-sky': [.02, 1.6, 3] }[th.wall];
    if (t.props.indexOf('sconce') >= 0) lights.push({ x: t.mx(1130), y: 340, z: 120, col: '#c0905a', k: .55 });
    var cx = L.x + (600 - L.x) * .35, cy = L.y + 20;
    return litF('cm-wall', [-300, -150, 1800, 620], wallFq[0], wallFq[1], wallFq[2], lights, t.amb, th.wall === 'canvas' ? .62 : .5) +
      litF('cm-woodlit', [-300, 430, 1800, 470], '.003 .07', 2, 31, [{ x: L.x, y: 430, z: 260, col: t.lc }], t.amb, .42, 1.1) +
      litF('cm-unitlit', [-40, 60, 560, 400], .02, 1.4, 9, lights, t.amb, .55) +
      woodF('cm-beam', 3, .004, .16, t.w0, t.w1) + woodF('cm-post', 9, .16, .004, t.w0, t.w1) + woodF('cm-board', 13, .11, .0035, t.w0, cmRgb(th.wood, 1.18)) +
      woodF('cm-top', 21, .0024, .052, cmRgb(th.wood, .3), cmRgb(th.wood, 1.4)) + woodF('cm-front', 33, .0022, .06, cmRgb(th.wood, .24), cmRgb(th.wood, .95)) +
      '<filter id="cm-stain" x="0" y="0" width="1" height="1"><feTurbulence type="fractalNoise" baseFrequency=".006 .014" numOctaves="3" seed="15"/><feColorMatrix type="matrix" values="0 0 0 0 0  0 0 0 0 0  0 0 0 0 0  0 0 0 -2.6 1.45"/><feComposite in="SourceGraphic" operator="in"/></filter>' +
      lin('cm-trim', 0, [[0, mix(tr, '#000', .82)], [.1, mix(tr, '#8a8a8a', .45)], [.22, mix(tr, '#000', .55)], [.48, mix(tr, '#000', .12)], [.7, mix(tr, '#ffffff', .5)], [.78, mix(tr, '#ffffff', .85)], [.86, mix(tr, '#000', .1)], [1, mix(tr, '#000', .78)]]) +
      lin('cm-trimv', 1, [[0, mix(tr, '#ffffff', .55)], [.3, tr], [.75, mix(tr, '#000', .55)], [1, mix(tr, '#000', .8)]]) +
      lin('cm-clothmat', 1, [[0, mix(sc, '#000', .36)], [1, sc]]) + lin('cm-accv', 1, [[0, mix(acc, '#ffffff', .3)], [1, mix(acc, '#000', .45)]]) +
      lin('cm-sil', 1, [[0, '#3a2c22'], [1, '#120c08']]) +
      rad('cm-glow', [[0, t.L.glow, .9], [.4, t.L.glow, .25], [1, t.L.glow, 0]]) + rad('cm-bloom', [[0, '#fffbea', .95], [.14, t.L.glow, .55], [.45, t.L.glow, .1], [1, t.L.glow, 0]]) +
      rad('cm-acc', [[0, acc, .85], [.5, acc, .22], [1, acc, 0]]) +
      rad('cm-shade', shade(t.shadeStops), ' gradientUnits="userSpaceOnUse" cx="' + f1(cx) + '" cy="' + f1(cy) + '" r="' + t.L.reach + '" gradientTransform="translate(' + f1(cx) + ' ' + f1(cy) + ') scale(1 .74) translate(' + f1(-cx) + ' ' + f1(-cy) + ')"') +
      rad('cm-vig', [[.55, '#000', 0], [1, '#000', .55]], ' cx=".5" cy=".46" r=".8"') + lin('cm-shaft', 1, [[0, th.outside === 'day' ? '#fff0c0' : '#a9bcff', th.outside === 'day' ? .22 : .2], [1, '#a9bcff', 0]]) +
      lin('cm-glass', 1, [[0, '#cfe8ff', .22], [.5, '#ffffff', .05], [1, '#cfe8ff', .18]]) +
      '<clipPath id="cm-tclip"><circle cx="641" cy="250" r="92"/></clipPath>';
  }

  /* ---- Outside: the sky and what stands against it, for the opening to
     show or, in an open-air stall, to fill the whole back. ---- */
  var CM_SKY = { rain: ['#46536a', '#8a98ab'] };
  function cmSky(t, id, x, y, w, h) {
    if (t.time !== 'rain') return skyArt(id, x, y, w, h, t.time);
    var c = CM_SKY.rain, r = rng(13), s = '<defs>' + lin(id, 1, [[0, c[0]], [1, c[1]]]) + '</defs><rect x="' + x + '" y="' + y + '" width="' + w + '" height="' + h + '" fill="url(#' + id + ')"/>', i;
    for (i = 0; i < 6; i++) s += '<ellipse cx="' + f1(x + r() * w) + '" cy="' + f1(y + h * (.08 + r() * .4)) + '" rx="' + f1(w * (.14 + r() * .12)) + '" ry="' + f1(h * (.05 + r() * .04)) + '" fill="#2c3648" opacity="' + f1(.28 + r() * .22) + '" filter="url(#src-blur14)"/>';
    return s;
  }
  function cmSkyline(x, y, w, h, col, seed) {
    var r = rng(seed || 5), s = '', cx = x, bw, bh;
    while (cx < x + w) {
      bw = 26 + r() * 36; bh = 20 + r() * h * .5;
      s += '<path d="M' + f1(cx) + ',' + f1(y + h) + ' V' + f1(y + h - bh) + (r() < .5 ? ' L' + f1(cx + bw / 2) + ',' + f1(y + h - bh - 12) + ' L' + f1(cx + bw) + ',' + f1(y + h - bh) : ' H' + f1(cx + bw)) + ' V' + f1(y + h) + ' Z" fill="' + col + '"/>';
      if (r() < .7) s += '<rect x="' + f1(cx + 6 + r() * (bw - 14)) + '" y="' + f1(y + h - bh + 8 + r() * 14) + '" width="3" height="4" fill="#e8b860" opacity="' + f1(.4 + r() * .3) + '"/>';
      cx += bw + 2 + r() * 6;
    }
    return s;
  }
  // The open-air stall's lane: sky, rooftops, houses leaning in on both sides and wet cobbles running away.
  function cmLane(t) {
    var s = cmSky(t, 'cm-lane-sky', -300, -150, 1800, 500), night = t.time === 'night' || t.time === 'rain', dark = night ? '#0b1022' : t.time === 'dusk' ? '#2a1e30' : '#6a7684';
    var wc = { plaster: '#6a5a4a', stone: '#4a4a4e', timber: '#4a3424', brick: '#6a3a2c', canvas: '#7a6a50', 'night-sky': '#1a1e3c' }[t.th.wall], r = rng(21), i, j;
    s += cmSkyline(-300, 200, 1800, 140, dark, 5) + '<rect x="-300" y="338" width="1800" height="130" fill="' + (night ? '#1a2038' : t.time === 'dusk' ? '#4a3a44' : '#8a8a8a') + '"/>';
    // Cobbles, narrowing to the far end of the lane.
    s += '<path d="M-300,452 L-300,470 H1500 V452 L840,338 H360 Z" fill="' + (night ? '#2a3150' : t.time === 'dusk' ? '#5a4650' : '#9a9486') + '"/>';
    for (i = 0; i < 9; i++) s += '<path d="M' + f1(360 + i * 60) + ',338 L' + f1(-300 + i * 225) + ',470" stroke="#000" stroke-opacity=".28" stroke-width="1.2"/>';
    for (i = 1; i < 7; i++) s += '<path d="M' + f1(360 - i * i * 4) + ',' + f1(338 + i * i * 3.2) + ' H' + f1(840 + i * i * 4) + '" stroke="#000" stroke-opacity=".25" stroke-width="1.2"/>';
    // Houses on both sides: a wall in the theme's material leaning in, with lit windows.
    [[-1, 0], [1, 0]].forEach(function (sd) {
      var x0 = sd[0] < 0 ? -300 : 1500, xe = sd[0] < 0 ? 330 : 870, d = 'M' + x0 + ',-110 L' + xe + ',20 L' + xe + ',338 L' + x0 + ',470 Z';
      s += '<path d="' + d + '" fill="' + wc + '"/><path d="' + d + '" fill="#000" opacity="' + (night ? .38 : .22) + '"/>';
      for (j = 0; j < 4; j++) for (i = 0; i < 3; i++) {
        var u = (i + .5) / 3, vx = x0 + (xe - x0) * u, top = -110 + (20 + 110) * u, bot = 470 + (338 - 470) * u, hh = (bot - top), wy = top + hh * (.2 + j * .18), ww = 34 * (1 - u * .6), wh = 44 * (1 - u * .6);
        if (r() < .75) s += '<rect x="' + f1(vx - ww / 2 - 6) + '" y="' + f1(wy - 6) + '" width="' + f1(ww + 12) + '" height="' + f1(wh + 12) + '" fill="#f0b860" opacity="' + (night ? .18 : 0) + '" filter="url(#src-blur5)"/><rect x="' + f1(vx - ww / 2) + '" y="' + f1(wy) + '" width="' + f1(ww) + '" height="' + f1(wh) + '" fill="' + (night ? '#f6c070' : '#2a3a50') + '" opacity="' + (night ? f1(.6 + r() * .4) : .6) + '"/>';
      }
    });
    if (t.time === 'rain' || t.props.indexOf('rain') >= 0) for (i = 0; i < 6; i++) s += '<ellipse cx="' + f1(300 + r() * 600) + '" cy="' + f1(360 + r() * 80) + '" rx="' + f1(40 + r() * 50) + '" ry="3" fill="#cfe2ff" opacity=".13"/>';
    return s;
  }

  /* ---- Back wall materials: each paints the whole wall plane, to be lit by
     the room's light through the 'cm-wall' filter. ---- */
  var WALLS = {
    plaster: function (t) {
      var s = '<rect x="-300" y="-150" width="1800" height="620" fill="#b8a48a" filter="url(#cm-wall)"/><rect x="-300" y="-150" width="1800" height="620" fill="#4a3424" filter="url(#cm-stain)" opacity=".55"/>', r = rng(41), i, x, y;
      for (i = 0; i < 7; i++) { x = r() * 1200; y = 60 + r() * 360; s += '<path d="M' + x.toFixed(0) + ',' + y.toFixed(0) + ' l' + (r() * 30 - 15).toFixed(0) + ',' + (10 + r() * 20).toFixed(0) + ' l' + (r() * 20 - 10).toFixed(0) + ',' + (8 + r() * 16).toFixed(0) + '" stroke="#2a1a10" stroke-opacity=".3" stroke-width=".8" fill="none"/>'; }
      return s;
    },
    stone: function (t) {
      var r = rng(8), s = '', y, x, w, row = 0;
      s += '<rect x="-300" y="-150" width="1800" height="620" fill="#807c76"/>';
      for (y = -150; y < 470; y += 46, row++) for (x = -300 - (row % 2) * 60; x < 1500; x += w) { w = 90 + r() * 70; s += '<rect x="' + f1(x + 2) + '" y="' + f1(y + 2) + '" width="' + f1(w - 4) + '" height="42" rx="3" fill="' + mix('#807c76', r() < .5 ? '#2a2826' : '#cfc8bc', r() * .3) + '"/>'; }
      return '<g filter="url(#cm-wall)">' + s + '</g><rect x="-300" y="-150" width="1800" height="620" fill="#2a2018" filter="url(#cm-stain)" opacity=".4"/>';
    },
    timber: function (t) {
      var r = rng(6), s = '', x, i = 0, base = mix(t.th.wood, '#000', .2);
      for (x = -300; x < 1500; x += 40, i++) s += '<rect x="' + x + '" y="-150" width="39" height="620" fill="' + mix(base, r() < .5 ? '#000' : '#ffffff', r() * .22) + '"/>';
      for (x = -300; x < 1500; x += 40) s += '<rect x="' + (x + 39) + '" y="-150" width="1.6" height="620" fill="#0a0604" opacity=".7"/>';
      for (x = -280; x < 1500; x += 40) { s += '<circle cx="' + x + '" cy="60" r="1.5" fill="#0a0604" opacity=".6"/><circle cx="' + x + '" cy="300" r="1.5" fill="#0a0604" opacity=".6"/>'; }
      return '<g filter="url(#cm-wall)">' + s + '</g>';
    },
    brick: function (t) {
      var r = rng(12), s = '<rect x="-300" y="-150" width="1800" height="620" fill="#2a1612"/>', y, x, row = 0, base = '#9a5a40';
      for (y = -150; y < 470; y += 26, row++) for (x = -300 - (row % 2) * 36; x < 1500; x += 72) s += '<rect x="' + (x + 1.5) + '" y="' + (y + 1.5) + '" width="69" height="23" rx="1.5" fill="' + mix(base, r() < .5 ? '#2a1008' : '#d89a78', r() * .32) + '"/>';
      return '<g filter="url(#cm-wall)">' + s + '</g>';
    },
    canvas: function (t) {
      var s = '<rect x="-300" y="-150" width="1800" height="620" fill="' + mix('#cdbb92', t.th.cloth, .22) + '" filter="url(#cm-wall)"/>', i;
      for (i = 0; i < 17; i++) s += '<rect x="' + (-300 + i * 110) + '" y="-150" width="110" height="620" fill="' + (i % 2 ? '#ffffff' : '#000') + '" opacity="' + (i % 2 ? .07 : .09) + '"/><path d="M' + (-300 + i * 110) + ',-150 V470" stroke="#3a2a1a" stroke-opacity=".28" stroke-width="1.4"/>';
      for (i = 0; i < 40; i++) s += '<path d="M' + (-300 + i * 46) + ',98 h22" stroke="#4a3822" stroke-opacity=".5" stroke-width="2"/>';
      return s + '<rect x="-300" y="90" width="1800" height="16" fill="#000" opacity=".12" filter="url(#src-blur3)"/><rect x="-300" y="94" width="1800" height="4" fill="' + t.th.trim + '" opacity=".6"/>';
    },
    'night-sky': function (t) {
      var r = rng(3), s = '<rect x="-300" y="-150" width="1800" height="620" fill="#222850" filter="url(#cm-wall)"/><ellipse cx="300" cy="120" rx="520" ry="260" fill="#3a2a78" opacity=".35" filter="url(#src-blur14)"/><ellipse cx="900" cy="260" rx="460" ry="200" fill="#1a4a78" opacity=".3" filter="url(#src-blur14)"/>', i, pts = [];
      for (i = 0; i < 150; i++) s += '<circle cx="' + f1(-280 + r() * 1760) + '" cy="' + f1(-140 + r() * 600) + '" r="' + f1(.6 + r() * 1.3) + '" fill="#fff" opacity="' + f1(.3 + r() * .6) + '"/>';
      for (i = 0; i < 6; i++) pts.push([60 + i * 40 + r() * 20, 160 + Math.sin(i * 1.3) * 40 + r() * 20]);
      s += '<path d="M' + pts.map(function (p) { return f1(p[0] + 720) + ',' + f1(p[1] - 40); }).join(' L') + '" fill="none" stroke="' + t.th.accent + '" stroke-opacity=".5" stroke-width="1.2"/>';
      pts.forEach(function (p) { s += '<circle cx="' + f1(p[0] + 720) + '" cy="' + f1(p[1] - 40) + '" r="3.2" fill="' + mix(t.th.accent, '#ffffff', .5) + '"/>'; });
      return s + '<circle cx="190" cy="150" r="30" fill="#e8e2c8" opacity=".9"/><circle cx="204" cy="142" r="27" fill="#222850"/>';
    }
  };

  /* ---- Skeleton: the back wall's shape. Each returns the baked art of the
     back of the room and nothing else; the opening, display and props go on
     top of it. ---- */
  function cmPost(x, w, y0, y1) {
    return '<rect x="' + x + '" y="' + y0 + '" width="' + w + '" height="' + (y1 - y0) + '" fill="#000" filter="url(#cm-post)"/><rect x="' + (x + w) + '" y="' + y0 + '" width="10" height="' + (y1 - y0) + '" fill="#000" opacity=".3" filter="url(#src-blur3)"/>';
  }
  var SHAPES = {
    square: function (t) {
      return WALLS[t.th.wall](t) + '<rect x="-300" y="-150" width="1800" height="184" fill="#000" filter="url(#cm-beam)"/><rect x="-300" y="34" width="1800" height="10" fill="#000" opacity=".4" filter="url(#src-blur3)"/>' +
        cmPost(-6, 34, 34, 474) + cmPost(1172, 34, 34, 474);
    },
    round: function (t) {
      var s = WALLS[t.th.wall](t), i, x;
      // A curved wall: the staves bunch toward the edges, the edges fall into shadow, and the vault's ribs fan out above.
      s += '<defs>' + lin('cm-cyl', 0, [[0, '#000', .72], [.1, '#000', .4], [.3, '#000', .08], [.5, '#000', 0], [.7, '#000', .08], [.9, '#000', .4], [1, '#000', .72]]) + lin('cm-cove', 1, [[0, '#000', .78], [1, '#000', 0]]) + '</defs>';
      for (i = -6; i <= 6; i++) { x = 600 + 720 * Math.sin(i * 12.5 * Math.PI / 180); s += '<path d="M' + f1(x) + ',30 V470" stroke="#000" stroke-opacity=".3" stroke-width="2"/><path d="M' + f1(x + 3) + ',30 V470" stroke="#fff" stroke-opacity=".06" stroke-width="1.4"/>'; }
      s += '<rect x="-300" y="-150" width="1800" height="620" fill="#000" opacity=".5" clip-path="url(#cm-outer)"/><rect x="0" y="-150" width="1200" height="620" fill="url(#cm-cyl)"/><clipPath id="cm-outer"><path d="M-300,-150 H1500 V470 H1200 V-150 H0 V470 H-300 Z"/></clipPath>';
      s += '<path d="M-300,-150 H1500 V20 Q1500,50 1200,52 Q600,150 0,52 Q-300,50 -300,20 Z" fill="#000" filter="url(#cm-beam)"/><path d="M-300,-150 H1500 V20 Q1500,50 1200,52 Q600,150 0,52 Q-300,50 -300,20 Z" fill="url(#cm-cove)" opacity=".5"/>';
      for (i = -5; i <= 5; i++) s += '<path d="M' + f1(600 + i * 130) + ',-150 Q' + f1(600 + i * 118) + ',0 ' + f1(600 + i * 150) + ',' + f1(70 + Math.abs(i) * 8) + '" stroke="#000" stroke-opacity=".5" stroke-width="5" fill="none"/>';
      s += '<path d="M0,52 Q600,150 1200,52" stroke="' + t.th.trim + '" stroke-opacity=".55" stroke-width="3" fill="none"/>';
      [-4, 1158].forEach(function (px) { s += cmPost(px, 46, 20, 474) + '<rect x="' + (px - 6) + '" y="20" width="58" height="12" fill="url(#cm-trim)"/><rect x="' + (px - 6) + '" y="436" width="58" height="14" fill="url(#cm-trim)"/><rect x="' + (px - 6) + '" y="450" width="58" height="4" fill="#000" opacity=".4"/>'; });
      return s;
    },
    alcove: function (t) {
      var N = 'M497,470 V190 A144,124 0 0 1 785,190 V470 Z', s = WALLS[t.th.wall](t);
      s += '<rect x="-300" y="-150" width="1800" height="150" fill="#000" filter="url(#cm-beam)"/><rect x="-300" y="0" width="1800" height="10" fill="#000" opacity=".4" filter="url(#src-blur3)"/>';
      s += '<defs>' + lin('cm-nichel', 0, [[0, '#000', .7], [.18, '#000', .22], [.5, '#000', .05], [.82, '#000', .22], [1, '#000', .7]]) + '</defs><path d="' + N + '" fill="#000" opacity=".5"/><path d="' + N + '" fill="url(#cm-nichel)"/>';
      s += '<path d="M497,452 V190 A144,124 0 0 1 785,190 V452" fill="none" stroke="#000" stroke-opacity=".5" stroke-width="22" transform="translate(-5,3)" filter="url(#src-blur5)"/>';
      s += '<path d="M489,470 V190 A152,132 0 0 1 793,190 V470" fill="none" stroke="url(#cm-trim)" stroke-width="12"/><path d="M481,470 V190 A160,140 0 0 1 801,190 V470" fill="none" stroke="#000" stroke-opacity=".4" stroke-width="3"/>';
      s += '<path d="M626,52 h30 l-4,26 h-22 Z" fill="url(#cm-trimv)"/><path d="M626,52 h30" stroke="#fff" stroke-opacity=".5"/>';
      s += cmPost(-6, 34, 10, 474) + cmPost(1172, 34, 10, 474);
      return s;
    },
    'open-air': function (t) {
      var s = cmLane(t), n = 18, fw = 1088 / n, tw = 800 / n, i, y0 = -110, y1 = 46;
      for (i = 0; i < n; i++) { var t0 = 200 + i * tw, f0 = 56 + i * fw; s += '<path d="M' + f1(t0) + ',' + y0 + ' L' + f1(t0 + tw) + ',' + y0 + ' L' + f1(f0 + fw) + ',' + y1 + ' L' + f1(f0) + ',' + y1 + ' Z" fill="' + (i % 2 ? mix(t.th.accent, '#000', .2) : t.th.cloth) + '"/>'; }
      s += '<defs>' + lin('cm-awshade', 1, [[0, '#000', .5], [.6, '#000', .1], [1, '#000', .3]]) + '</defs><path d="M200,' + y0 + ' H1000 L1144,' + y1 + ' H56 Z" fill="url(#cm-awshade)"/>';
      for (i = 0; i <= n; i++) s += '<path d="M' + f1(200 + i * tw) + ',' + y0 + ' L' + f1(56 + i * fw) + ',' + y1 + '" stroke="#120808" stroke-opacity=".45" stroke-width="1"/>';
      s += '<rect x="52" y="' + (y1 - 2) + '" width="1096" height="8" fill="#000" filter="url(#cm-beam)"/><rect x="52" y="' + (y1 - 2) + '" width="1096" height="1.6" fill="' + t.th.trim + '" opacity=".8"/>';
      for (i = 0; i < n; i++) { var sx = 56 + i * fw; s += '<path d="M' + f1(sx) + ',' + (y1 + 6) + ' H' + f1(sx + fw) + ' V' + (y1 + 14) + ' A' + f1(fw / 2) + ',14 0 0 1 ' + f1(sx) + ',' + (y1 + 14) + ' Z" fill="' + (i % 2 ? mix(t.th.accent, '#000', .35) : mix(t.th.cloth, '#000', .25)) + '"/>'; }
      [38, 1140].forEach(function (px) { s += cmPost(px, 22, y1, 474) + '<rect x="' + (px - 3) + '" y="' + (y1 + 4) + '" width="28" height="9" fill="url(#cm-trim)"/>'; });
      return s;
    }
  };

  /* ---- Skeleton: the opening. What is outside shows through it; rain,
     if there is any, falls only where the opening lets you see. ---- */
  function cmRainPath(x0, y0, w, h, seed, n, len) {
    var r = rng(seed), d = '', i, x, y, l;
    for (i = 0; i < n; i++) { x = x0 + r() * w; y = y0 - 100 + r() * (h + 200); l = len * (.7 + r() * .6); d += 'M' + f1(x) + ',' + f1(y) + ' l' + f1(-l * .14) + ',' + f1(l) + ' '; }
    return d;
  }
  function cmWet(t) { return t.time === 'rain' || t.props.indexOf('rain') >= 0; }
  function cmRainIn(t, x, y, w, h, seed) {
    if (!cmWet(t)) return '';
    return '<g class="src-nohit"><g class="cm-rain"><path d="' + cmRainPath(x, y, w, h, seed, 46, 16) + '" stroke="#cfe2ff" stroke-opacity=".55" stroke-width="1.2" fill="none"/></g><g class="cm-rain slow"><path d="' + cmRainPath(x, y, w, h, seed + 4, 24, 11) + '" stroke="#cfe2ff" stroke-opacity=".3" stroke-width="1" fill="none"/></g></g>';
  }
  function cmFrame(t, d, sw) { return '<path d="' + d + '" fill="none" stroke="' + mix(t.th.wood, '#000', .45) + '" stroke-width="' + sw + '"/><path d="' + d + '" fill="none" stroke="' + mix(t.th.wood, '#ffffff', .12) + '" stroke-width="1.4" stroke-opacity=".5"/>'; }
  var OPENINGS = {
    none: function (t) { return ''; },
    'window-right': function (t) {
      var s = '<g data-k="window"><rect x="782" y="64" width="260" height="246" rx="6" fill="#000" opacity=".45" filter="url(#src-blur8)"/><rect x="792" y="72" width="240" height="224" rx="3" fill="' + mix(t.th.wood, '#000', .5) + '"/>';
      s += '<g clip-path="url(#cm-oclip)">' + cmSky(t, 'cm-osky', 806, 86, 212, 196) + cmSkyline(806, 190, 212, 92, t.time === 'night' || t.time === 'rain' ? '#090d1a' : t.time === 'dusk' ? '#2a1e30' : '#6a7684', 9) + '<rect x="806" y="210" width="212" height="72" fill="url(#src-fog)"/>' + cmRainIn(t, 806, 86, 212, 196, 3) + '<path d="M806,86 L890,86 L806,200 Z" fill="#fff" opacity=".05"/></g>';
      s += '<rect x="909" y="86" width="7" height="196" fill="' + mix(t.th.wood, '#000', .6) + '"/><rect x="806" y="176" width="212" height="6" fill="' + mix(t.th.wood, '#000', .6) + '"/><rect x="914" y="86" width="2" height="196" fill="' + mix(t.th.wood, '#ffffff', .3) + '" opacity=".3"/>';
      return s + '<rect x="786" y="296" width="252" height="12" rx="2" fill="' + mix(t.th.wood, '#000', .2) + '"/><rect x="786" y="296" width="252" height="3.5" fill="' + mix(t.th.wood, '#ffffff', .3) + '"/><rect x="786" y="308" width="252" height="10" fill="#000" opacity=".4" filter="url(#src-blur3)"/></g>' +
        '<path d="M806,282 L760,452 L900,452 L1018,282 Z" fill="url(#cm-shaft)" opacity=".6" pointer-events="none"/><clipPath id="cm-oclip"><rect x="806" y="86" width="212" height="196"/></clipPath>';
    },
    'window-centre': function (t) {
      var d = 'M522,330 V176 A119,106 0 0 1 760,176 V330 Z', s = '<g data-k="window"><path d="' + d + '" fill="#000" opacity=".4" filter="url(#src-blur8)" transform="translate(0,6)"/><path d="' + d + '" fill="' + mix(t.th.wood, '#000', .5) + '"/>';
      s += '<g clip-path="url(#cm-oclip)">' + cmSky(t, 'cm-osky', 510, 60, 270, 280) + cmSkyline(510, 230, 270, 100, t.time === 'night' || t.time === 'rain' ? '#090d1a' : t.time === 'dusk' ? '#2a1e30' : '#6a7684', 14) + '<rect x="510" y="270" width="270" height="60" fill="url(#src-fog)"/>' + cmRainIn(t, 510, 60, 270, 280, 6) + '</g>';
      s += '<path d="M641,70 V330 M522,200 H760" stroke="' + mix(t.th.wood, '#000', .6) + '" stroke-width="7"/>' + cmFrame(t, d, 11);
      return s + '<rect x="506" y="328" width="270" height="12" rx="2" fill="' + mix(t.th.wood, '#000', .2) + '"/><rect x="506" y="328" width="270" height="3.5" fill="' + mix(t.th.wood, '#ffffff', .3) + '"/></g><clipPath id="cm-oclip"><path d="' + d + '"/></clipPath>';
    },
    'round-window': function (t) {
      var c = { x: 912, y: 190, r: 108 }, s = '<g data-k="window"><circle cx="912" cy="196" r="128" fill="#000" opacity=".4" filter="url(#src-blur8)"/><circle cx="912" cy="190" r="122" fill="url(#cm-trim)"/><circle cx="912" cy="190" r="112" fill="#000" opacity=".5"/>';
      s += '<g clip-path="url(#cm-oclip)">' + cmSky(t, 'cm-osky', 800, 78, 224, 224) + cmSkyline(800, 210, 224, 92, t.time === 'night' || t.time === 'rain' ? '#090d1a' : t.time === 'dusk' ? '#2a1e30' : '#6a7684', 17) + cmRainIn(t, 800, 78, 224, 224, 8) + '<path d="M820,120 A108,108 0 0 1 900,86 L860,190 Z" fill="#fff" opacity=".06"/></g>';
      s += '<path d="M912,82 V298 M804,190 H1020" stroke="url(#cm-trim)" stroke-width="5"/><circle cx="912" cy="190" r="108" fill="none" stroke="url(#cm-trim)" stroke-width="7"/><circle cx="912" cy="190" r="14" fill="url(#cm-trimv)"/>';
      return s + '</g><path d="M806,282 L760,452 L900,452 L1018,282 Z" fill="url(#cm-shaft)" opacity=".5" pointer-events="none"/><clipPath id="cm-oclip"><circle cx="912" cy="190" r="108"/></clipPath>';
    },
    doorway: function (t) {
      var d = 'M806,452 V196 A104,104 0 0 1 1014,196 V452 Z', s = '<g data-k="window"><path d="' + d + '" fill="#000" opacity=".45" filter="url(#src-blur8)" transform="translate(4,0)"/>';
      s += '<g clip-path="url(#cm-oclip)">' + cmSky(t, 'cm-osky', 790, 80, 240, 380) + cmSkyline(790, 260, 240, 110, t.time === 'night' || t.time === 'rain' ? '#090d1a' : t.time === 'dusk' ? '#2a1e30' : '#6a7684', 19) +
        '<rect x="790" y="360" width="240" height="100" fill="' + (t.time === 'day' || t.time === '' ? '#9a9486' : '#2a3150') + '"/><path d="M830,460 L880,360 M910,460 L910,360 M990,460 L940,360" stroke="#000" stroke-opacity=".25" stroke-width="1.4"/><rect x="790" y="320" width="240" height="60" fill="url(#src-fog)"/>' + cmRainIn(t, 790, 80, 240, 380, 11) + '</g>';
      s += cmFrame(t, d, 16) + '<path d="M798,452 V196 A112,112 0 0 1 1022,196 V452" fill="none" stroke="#000" stroke-opacity=".4" stroke-width="3"/><rect x="796" y="446" width="226" height="10" fill="' + mix(t.th.wood, '#ffffff', .15) + '"/>';
      return s + '</g><path d="M830,360 L760,470 H1000 L1010,360 Z" fill="url(#cm-shaft)" opacity=".5" pointer-events="none"/><clipPath id="cm-oclip"><path d="' + d + '"/></clipPath>';
    }
  };
  OPENINGS['window-left'] = OPENINGS['window-right'];
  // Where each opening sits and what you see when you point at it, for the slot editor and the card.
  var CM_OPEN_INFO = { 'window-right': 'The window', 'window-left': 'The window', 'window-centre': 'The tall window', 'round-window': 'The round window', doorway: 'The doorway' };

  /* ---- Skeleton: how goods are displayed. Each display draws its own unit
     (baked) and says where the twelve goods go, so goods always land on it.
     base/front draw per good under and over it (a dome's glass goes over). ---- */
  var CM_COLS = [96, 206, 316, 426], CM_ROWS = [176, 290, 404];
  function cmGrid() { var a = [], i; for (i = 0; i < 12; i++) a.push([CM_COLS[i % 4], CM_ROWS[Math.floor(i / 4)], 1.2, 0]); return a; }
  // The frame every wall unit shares: back board, top beam, two uprights and a drop shadow under each shelf.
  function cmUnitFrame(t, board) {
    return '<rect x="34" y="80" width="454" height="370" fill="#000" filter="url(#cm-board)"/><rect x="34" y="80" width="454" height="370" fill="#0a0604" opacity="' + (board || .45) + '"/>' +
      '<rect x="28" y="70" width="466" height="16" fill="#000" filter="url(#cm-beam)"/><rect x="24" y="70" width="16" height="386" fill="#000" filter="url(#cm-post)"/><rect x="482" y="70" width="16" height="386" fill="#000" filter="url(#cm-post)"/>';
  }
  function cmShelfBoard(t, y, x0, x1) {
    return '<rect x="' + x0 + '" y="' + y + '" width="' + (x1 - x0) + '" height="12" fill="#000" filter="url(#cm-beam)"/><rect x="' + x0 + '" y="' + y + '" width="' + (x1 - x0) + '" height="2" fill="' + t.lc + '" opacity=".35"/><rect x="' + x0 + '" y="' + (y + 12) + '" width="' + (x1 - x0) + '" height="14" fill="#000" opacity=".45" filter="url(#src-blur3)"/>';
  }
  var DISPLAYS = {
    shelves: function (t) {
      var s = cmUnitFrame(t, .45), i;
      for (i = 34; i < 488; i += 56) s += '<rect x="' + i + '" y="80" width="1.6" height="370" fill="#050302" opacity=".7"/>';
      CM_ROWS.forEach(function (y) { s += cmShelfBoard(t, y, 40, 482); });
      return { art: '<g filter="url(#cm-unitlit)">' + s + '</g>', slots: cmGrid(), zone: [34, 96, 454, 320], wallsh: true, decor: true, label: 'The shelves' };
    },
    pegboard: function (t) {
      var s = '', i, x, y, bd = mix(t.th.wood, '#ffffff', .3);
      s += '<rect x="34" y="80" width="454" height="372" rx="4" fill="' + bd + '"/><rect x="34" y="80" width="454" height="372" rx="4" fill="#000" filter="url(#cm-board)" opacity=".7"/>';
      for (y = 96; y < 450; y += 20) for (x = 48; x < 484; x += 20) s += '<circle cx="' + x + '" cy="' + y + '" r="2.2" fill="#0a0604" opacity=".75"/>';
      s += '<rect x="34" y="80" width="454" height="372" rx="4" fill="none" stroke="url(#cm-trim)" stroke-width="6"/>';
      CM_ROWS.forEach(function (ry, j) {
        s += '<rect x="42" y="' + ry + '" width="438" height="7" fill="url(#cm-trimv)"/><rect x="42" y="' + (ry + 7) + '" width="438" height="12" fill="#000" opacity=".4" filter="url(#src-blur3)"/>';
        for (i = 0; i < 5; i++) s += '<path d="M' + (42 + i * 110 + 8) + ',' + (ry + 7) + ' v12 h10" stroke="url(#cm-trim)" stroke-width="4" fill="none"/>';
        for (i = 0; i < 4; i++) { x = CM_COLS[i]; s += '<path d="M' + (x - 14) + ',' + (ry - 96) + ' h28 M' + x + ',' + (ry - 96) + ' v-8" stroke="#1a1612" stroke-width="3" fill="none" opacity=".8"/><circle cx="' + x + '" cy="' + (ry - 104) + '" r="3" fill="url(#cm-trim)"/>'; }
      });
      return { art: '<g filter="url(#cm-unitlit)">' + s + '</g>', slots: cmGrid(), zone: [34, 96, 454, 320], wallsh: true, outline: true, label: 'The pegboard' };
    },
    drawers: function (t) {
      var s = cmUnitFrame(t, .55), i, r = rng(17);
      CM_ROWS.forEach(function (y, j) {
        s += cmShelfBoard(t, y, 40, 482);
        for (i = 0; i < 8; i++) {
          var x = 44 + i * 55;
          s += '<rect x="' + x + '" y="' + (y + 15) + '" width="51" height="20" rx="2" fill="' + mix(t.th.wood, '#000', .1 + r() * .14) + '" stroke="#0a0604" stroke-opacity=".7"/><rect x="' + (x + 1) + '" y="' + (y + 16) + '" width="49" height="2" fill="' + t.lc + '" opacity=".22"/>' +
            '<rect x="' + (x + 14) + '" y="' + (y + 20) + '" width="23" height="9" rx="1.5" fill="#e8dcbc" opacity=".85"/><circle cx="' + (x + 25.5) + '" cy="' + (y + 24.5) + '" r="2.2" fill="url(#cm-trim)"/>';
        }
      });
      return { art: '<g filter="url(#cm-unitlit)">' + s + '</g>', slots: cmGrid(), zone: [34, 96, 454, 320], wallsh: true, decor: true, label: 'The drawer wall' };
    },
    pigeonholes: function (t) {
      var s = cmUnitFrame(t, .3), i, r = rng(23);
      CM_ROWS.forEach(function (y, j) {
        for (i = 0; i < 4; i++) {
          var x = CM_COLS[i];
          s += '<rect x="' + (x - 50) + '" y="' + (y - 98) + '" width="100" height="98" rx="9" fill="#05030a" opacity=".72"/><rect x="' + (x - 50) + '" y="' + (y - 98) + '" width="100" height="98" rx="9" fill="none" stroke="#000" stroke-opacity=".6" stroke-width="3"/>' +
            '<path d="M' + (x - 48) + ',' + (y - 4) + ' V' + (y - 80) + ' Q' + (x - 48) + ',' + (y - 96) + ' ' + (x - 32) + ',' + (y - 96) + '" fill="none" stroke="' + t.lc + '" stroke-opacity=".12" stroke-width="2"/>';
        }
        s += '<rect x="36" y="' + y + '" width="450" height="12" fill="#000" filter="url(#cm-beam)"/><rect x="36" y="' + y + '" width="450" height="2" fill="' + t.lc + '" opacity=".35"/><rect x="36" y="' + (y + 12) + '" width="450" height="12" fill="#000" opacity=".5" filter="url(#src-blur3)"/>';
        for (i = 0; i < 4; i++) s += '<circle cx="' + CM_COLS[i] + '" cy="' + (y + 6) + '" r="3.2" fill="url(#cm-trim)"/>';
      });
      for (i = 0; i < 5; i++) s += '<rect x="' + (36 + i * 110 - 5) + '" y="82" width="10" height="368" fill="#000" filter="url(#cm-post)"/>';
      return { art: '<g filter="url(#cm-unitlit)">' + s + '</g>', slots: cmGrid(), zone: [34, 96, 454, 320], wallsh: false, decor: true, label: 'The pigeonholes' };
    },
    /* Glass domes standing on the counter: no wall unit, the goods are on
       the counter itself, eight to the left of the cloth and four to its right. */
    domes: function (t) {
      var sl = [], i;
      for (i = 0; i < 8; i++) sl.push([52 + i * 75, 456, .95, 0]);
      for (i = 0; i < 4; i++) sl.push([940 + i * 68, 456, .9, 1]);
      return { art: '', slots: sl, zone: null, wallsh: false, counterTop: true, label: 'The domes',
        base: function (it, s) {
          var x = s[0], y = s[1], sc = s[2];
          return '<ellipse cx="' + x + '" cy="' + (y + 2) + '" rx="' + f1(34 * sc) + '" ry="' + f1(6 * sc) + '" fill="#000" opacity=".5" filter="url(#src-blur3)"/><ellipse cx="' + x + '" cy="' + y + '" rx="' + f1(32 * sc) + '" ry="' + f1(5.4 * sc) + '" fill="url(#cm-trim)"/><ellipse cx="' + x + '" cy="' + (y - 1.5) + '" rx="' + f1(28 * sc) + '" ry="' + f1(4 * sc) + '" fill="' + mix(t.th.cloth, '#000', .25) + '"/>';
        },
        front: function (it, s) {
          var x = s[0], y = s[1], sc = s[2], rx = 30 * sc, h = 92 * sc, c = safeHex(it.c);
          return '<ellipse cx="' + x + '" cy="' + f1(y - 30 * sc) + '" rx="' + f1(rx * 1.4) + '" ry="' + f1(40 * sc) + '" fill="' + c + '" opacity=".14" filter="url(#src-blur8)" class="src-nohit"/>' +
            '<path d="M' + f1(x - rx) + ',' + y + ' V' + f1(y - h * .52) + ' A' + f1(rx) + ',' + f1(h * .5) + ' 0 0 1 ' + f1(x + rx) + ',' + f1(y - h * .52) + ' V' + y + ' Z" fill="url(#cm-glass)" stroke="#dff0ff" stroke-opacity=".5" stroke-width="1.4" class="src-nohit"/>' +
            '<path d="M' + f1(x - rx * .66) + ',' + f1(y - h * .5) + ' q2,-' + f1(h * .3) + ' ' + f1(rx * .5) + ',-' + f1(h * .38) + '" stroke="#fff" stroke-opacity=".6" stroke-width="2.4" fill="none" stroke-linecap="round" class="src-nohit"/>' +
            '<circle cx="' + x + '" cy="' + f1(y - h * 1.03) + '" r="' + f1(3.4 * sc) + '" fill="url(#cm-trimv)" class="src-nohit"/>';
        } };
    }
  };

  /* ---- Skeleton: the counter. All three share one working surface (top at
     y 452, the cloth and pile on it) so goods always land the same way;
     they differ in the front and the edge. ---- */
  function cmTop(t, d, extra) {
    var r = rng(11), s = '<path d="' + d + '" fill="#000" filter="url(#cm-top)"/>', i, x;
    for (i = 0; i < 5; i++) s += '<path d="M-300,' + (458 + i * 11) + ' H1500" stroke="#1a0e06" stroke-opacity=".5" stroke-width="1.2" clip-path="url(#cm-topclip)"/>';
    for (i = 0; i < 40; i++) { x = r() * 1200; s += '<path d="M' + x.toFixed(0) + ',' + (460 + r() * 40).toFixed(0) + ' l' + (10 + r() * 40).toFixed(0) + ',' + (r() * 2 - 1).toFixed(1) + '" stroke="' + mix(t.th.wood, '#ffffff', .5) + '" stroke-opacity="' + (.06 + r() * .1).toFixed(2) + '" stroke-width=".8"/>'; }
    return s;
  }
  var COUNTERS = {
    straight: function (t) {
      var s = cmTop(t, 'M-300,452 H1500 V512 H-300 Z'), x;
      s += '<clipPath id="cm-topclip"><rect x="-300" y="452" width="1800" height="60"/></clipPath><path d="M-300,506 H1500 V516 H-300 Z" fill="#000" filter="url(#cm-beam)"/><rect x="-300" y="506" width="1800" height="2.4" fill="' + t.lc + '" opacity=".45"/><rect x="-300" y="516" width="1800" height="384" fill="#000" filter="url(#cm-front)"/>';
      for (x = -368; x < 1500; x += 196) s += '<rect x="' + x + '" y="540" width="168" height="140" rx="3" fill="#000" opacity=".28"/><rect x="' + (x + 10) + '" y="550" width="148" height="120" rx="2" fill="#000" filter="url(#cm-front)"/>' +
        '<path d="M' + (x + 10) + ',670 V550 H' + (x + 158) + '" stroke="' + mix(t.th.wood, '#ffffff', .55) + '" stroke-opacity=".2" stroke-width="2" fill="none"/><path d="M' + (x + 158) + ',550 V670 H' + (x + 10) + '" stroke="#000" stroke-opacity=".5" stroke-width="2" fill="none"/>';
      return '<g filter="url(#cm-woodlit)">' + s + '<rect x="-300" y="516" width="1800" height="30" fill="#000" opacity=".5" filter="url(#src-blur8)"/></g>';
    },
    /* A bow-fronted counter: the front edge swells toward you and the apron
       is a row of arched panels that follow it. */
    curved: function (t) {
      function yc(x) { var u = (x + 300) / 1800; return 500 + 80 * u * (1 - u); }
      var top = 'M-300,452 H1500 V500 Q600,540 -300,500 Z', s = cmTop(t, top), x, y;
      s += '<clipPath id="cm-topclip"><path d="' + top + '"/></clipPath><path d="M-300,497 Q600,537 1500,497" fill="none" stroke="' + t.lc + '" stroke-opacity=".5" stroke-width="2.4"/>';
      s += '<path d="M-300,500 Q600,540 1500,500 V700 H-300 Z" fill="#000" filter="url(#cm-front)"/><path d="M-300,500 Q600,540 1500,500 V512 Q600,552 -300,512 Z" fill="url(#cm-trim)" opacity=".85"/>';
      for (x = -250; x < 1500; x += 150) { y = yc(x + 60) + 28; s += '<path d="M' + x + ',700 V' + (y + 70) + ' Q' + x + ',' + y + ' ' + (x + 60) + ',' + y + ' Q' + (x + 120) + ',' + y + ' ' + (x + 120) + ',' + (y + 70) + ' V700 Z" fill="#000" opacity=".3"/><path d="M' + (x + 8) + ',700 V' + (y + 74) + ' Q' + (x + 8) + ',' + (y + 8) + ' ' + (x + 60) + ',' + (y + 8) + ' Q' + (x + 112) + ',' + (y + 8) + ' ' + (x + 112) + ',' + (y + 74) + ' V700' + '" fill="none" stroke="' + mix(t.th.trim, '#000', .35) + '" stroke-width="3"/>'; }
      return '<g filter="url(#cm-woodlit)">' + s + '<path d="M-300,512 Q600,552 1500,512 V560 Q600,600 -300,560 Z" fill="#000" opacity=".4" filter="url(#src-blur8)"/></g>';
    },
    /* A market stall's table: a lacquered top with an inlaid line and brass
       studs, and a hanging skirt with a scalloped valence over a lattice. */
    'stall-table': function (t) {
      var s = cmTop(t, 'M-300,452 H1500 V508 H-300 Z'), x, y, i;
      s += '<clipPath id="cm-topclip"><rect x="-300" y="452" width="1800" height="56"/></clipPath><rect x="-300" y="460" width="1800" height="1" fill="#fff" opacity=".12"/>';
      for (x = -280; x < 1500; x += 120) s += '<circle cx="' + x + '" cy="500" r="3" fill="url(#cm-trimv)"/>';
      s += '<rect x="-300" y="496" width="1800" height="3" fill="' + t.th.accent + '" opacity=".7"/><rect x="-300" y="508" width="1800" height="6" fill="#000" filter="url(#cm-beam)"/><rect x="-300" y="506" width="1800" height="2.4" fill="' + t.lc + '" opacity=".5"/>';
      var sk = '<rect x="-300" y="512" width="1800" height="200" fill="url(#cm-clothmat)"/><rect x="-300" y="512" width="1800" height="200" fill="#000" opacity=".2" filter="url(#src-cloth)"/>';
      for (x = -300; x < 1500; x += 150) for (y = 560; y < 700; y += 38) { sk += '<circle cx="' + x + '" cy="' + y + '" r="38" fill="none" stroke="' + t.th.accent + '" stroke-opacity=".5" stroke-width="1.4"/><circle cx="' + (x + 75) + '" cy="' + (y + 19) + '" r="38" fill="none" stroke="' + t.th.accent + '" stroke-opacity=".5" stroke-width="1.4"/>'; }
      sk += '<rect x="-300" y="512" width="1800" height="40" fill="url(#cm-clothmat)"/>';
      for (i = 0; i < 31; i++) sk += '<path d="M' + (-300 + i * 60) + ',552 a30,26 0 0 0 60,0 Z" fill="' + (i % 2 ? mix(t.th.accent, '#000', .25) : mix(t.th.cloth, '#000', .1)) + '"/><path d="M' + (-298 + i * 60) + ',552 a28,24 0 0 0 56,0" fill="none" stroke="' + t.th.trim + '" stroke-opacity=".7" stroke-width="1.2"/>';
      sk += '<rect x="-300" y="552" width="1800" height="22" fill="#000" opacity=".3" filter="url(#src-blur8)"/>';
      return '<g filter="url(#cm-woodlit)">' + s + '</g>' + sk;
    }
  };

  /* ---- On the counter: the cloth where the keeper sets a good down, the bell,
     the scales, the case and the 'more' card, placed by where the room's light
     and goods leave space. ---- */
  function cmSpots(t, hasCase) {
    var m = t.mx, orr = t.props.indexOf('orrery') >= 0;
    return { bell: [m(t.domes ? 1040 : 950), t.domes ? 508 : 496], scales: t.domes || orr ? null : [m(1046), 490], cs: [m(330), t.domes ? 506 : 486], more: [m(t.domes ? 100 : 1110), t.domes ? 526 : 520] };
  }

  /* ---- Look: the light source. It is the room's one real light: art for it
     (a lamp, a candelabra, a forge, paper lanterns, or just the moon through
     the opening), where it sits and the glow it gives off. ---- */
  function cmCandle(x, y, h, lit) {
    return '<rect x="' + (x - 4) + '" y="' + (y - h) + '" width="8" height="' + h + '" rx="1.6" fill="#e8dcc0"/><rect x="' + (x - 4) + '" y="' + (y - h) + '" width="3" height="' + h + '" fill="#fff" opacity=".2"/>' +
      '<path d="M' + (x + 4) + ',' + (y - h + 2) + ' q2,6 0,10" stroke="#cfc4a8" stroke-width="2" fill="none"/>' +
      (lit ? '<path class="src-flame" d="M' + x + ',' + (y - h - 15) + ' c-4.5,7 -4.5,11 0,14 c4.5,-3 4.5,-7 0,-14 Z" fill="url(#src-flame)"/>' : '');
  }
  function cmLantern(t, x, y, r, j) {
    var c = mix(t.th.accent, '#ff5a2a', .5), h = r * 1.3;
    return '<g class="cm-sway" style="transform-origin:' + x + 'px ' + y + 'px;animation-delay:' + (j * -.9).toFixed(1) + 's"><path d="M' + x + ',' + (y - 16) + ' V' + y + '" stroke="#1a100a" stroke-width="1.2"/>' +
      '<rect x="' + f1(x - r * .45) + '" y="' + y + '" width="' + f1(r * .9) + '" height="4" rx="1.5" fill="#1a100a"/>' +
      '<ellipse cx="' + x + '" cy="' + f1(y + 4 + h) + '" rx="' + r + '" ry="' + f1(h) + '" fill="' + mix(c, '#000', .35) + '"/><ellipse cx="' + x + '" cy="' + f1(y + 4 + h) + '" rx="' + f1(r * .82) + '" ry="' + f1(h * .88) + '" fill="' + mix(c, '#ffe0a0', .25) + '"/>' +
      '<path d="M' + x + ',' + f1(y + 4) + ' V' + f1(y + 4 + 2 * h) + ' M' + f1(x - r * .5) + ',' + f1(y + 8) + ' Q' + f1(x - r * .6) + ',' + f1(y + 4 + h) + ' ' + f1(x - r * .5) + ',' + f1(y + 2 * h) + ' M' + f1(x + r * .5) + ',' + f1(y + 8) + ' Q' + f1(x + r * .6) + ',' + f1(y + 4 + h) + ' ' + f1(x + r * .5) + ',' + f1(y + 2 * h) + '" stroke="' + mix(c, '#000', .55) + '" stroke-width="1" fill="none" opacity=".7"/>' +
      '<rect x="' + f1(x - r * .45) + '" y="' + f1(y + 2 * h + 2) + '" width="' + f1(r * .9) + '" height="4" rx="1.5" fill="#1a100a"/><path d="M' + x + ',' + f1(y + 2 * h + 6) + ' v10" stroke="' + t.th.trim + '" stroke-width="1.6"/></g>';
  }
  // Positions of the lantern string: low over the stall's left, hung from the beam; lower and nearer over the keeper.
  var CM_LANT = [[110, 40, 11], [250, 38, 11], [390, 40, 11], [730, 88, 13], [830, 110, 14], [940, 98, 13], [1050, 112, 14], [1150, 96, 13]];
  function cmLanterns(t, glow) {
    var s = '', g = '';
    s += '<path d="M60,38 H470 M700,50 Q900,130 1180,64" stroke="#1a100a" stroke-width="1.4" fill="none"/>';
    CM_LANT.forEach(function (q, j) { s += cmLantern(t, q[0], q[1], q[2], j); if (glow) g += '<circle cx="' + q[0] + '" cy="' + (q[1] + 38) + '" r="' + (q[2] * 7) + '" fill="url(#cm-glow)" opacity=".6" class="src-nohit"/>'; });
    return s + g;
  }
  var LIGHTS = {
    lamp: function (t) {
      var x = t.lp.x;
      if (t.pend) return { back: '<g data-k="lamp"><path d="M' + x + ',34 V150" stroke="#2a1a10" stroke-width="2"/><path d="M' + (x - 34) + ',196 Q' + x + ',130 ' + (x + 34) + ',196 Z" fill="url(#cm-trim)"/><rect x="' + (x - 36) + '" y="194" width="72" height="6" rx="2" fill="url(#cm-trimv)"/><path class="src-flame" d="M' + x + ',198 c-7,9 -8,17 0,23 c8,-6 7,-14 0,-23 Z" fill="url(#src-flame)"/><circle cx="' + x + '" cy="214" r="86" fill="url(#cm-bloom)" data-flick=".8"/></g>', front: '' };
      return { back: '', front: lampArt(x, 478).replace(/url\(#src-brass\)/g, 'url(#cm-trim)').replace(/url\(#src-brassv\)/g, 'url(#cm-trimv)').replace(/url\(#src-bloom\)/g, 'url(#cm-bloom)') };
    },
    candles: function (t) {
      var x = t.lp.x, s = '';
      if (t.pend) {
        s = '<g data-k="lamp"><path d="M' + (x - 40) + ',34 L' + x + ',170 L' + (x + 40) + ',34" stroke="#2a1a10" stroke-width="1.6" fill="none"/><ellipse cx="' + x + '" cy="182" rx="64" ry="9" fill="url(#cm-trim)"/>';
        [-50, -26, 0, 26, 50].forEach(function (d, j) { s += cmCandle(x + d, 180 - (Math.abs(d) < 30 ? 0 : 0), 24, true); });
        return { back: s + '<circle cx="' + x + '" cy="160" r="100" fill="url(#cm-bloom)" data-flick=".8"/></g>', front: '' };
      }
      s = '<g data-k="lamp"><ellipse cx="' + x + '" cy="480" rx="52" ry="8" fill="url(#src-ao)"/><path d="M' + (x - 22) + ',478 h44 l-6,-8 h-32 Z" fill="url(#cm-trim)"/><rect x="' + (x - 3) + '" y="430" width="6" height="40" fill="url(#cm-trim)"/>' +
        '<path d="M' + (x - 44) + ',428 Q' + (x - 44) + ',444 ' + x + ',444 Q' + (x + 44) + ',444 ' + (x + 44) + ',428" stroke="url(#cm-trim)" stroke-width="4" fill="none"/>' +
        cmCandle(x - 44, 428, 40, true) + cmCandle(x, 440, 46, true) + cmCandle(x + 44, 428, 34, true) + '<circle cx="' + x + '" cy="402" r="84" fill="url(#cm-bloom)" data-flick=".8"/></g>';
      return { back: '', front: s };
    },
    forge: function (t) {
      var hood = 'M500,452 V330 L548,220 L592,34 H690 L734,220 L782,330 V452 Z', mouth = 'M530,440 V300 Q530,214 641,214 Q752,214 752,300 V440 Z', s = '<path d="' + hood + '" fill="#5e5650" filter="url(#cm-wall)"/><clipPath id="cm-hoodclip"><path d="' + hood + '"/></clipPath><g clip-path="url(#cm-hoodclip)">', r = rng(77), x, y, row;
      for (row = 0; row < 14; row++) { y = 40 + row * 30; s += '<path d="M440,' + y + ' H850" stroke="#0a0806" stroke-opacity=".5" stroke-width="1.6"/>'; for (x = 440 + (row % 2) * 34; x < 850; x += 68) s += '<path d="M' + (x + r() * 10).toFixed(0) + ',' + y + ' v30" stroke="#0a0806" stroke-opacity=".5" stroke-width="1.6"/>'; }
      s += '</g><path d="M500,330 H782 V344 H500 Z" fill="url(#src-iron)"/><path d="M548,220 H734 L738,232 H544 Z" fill="url(#src-iron)"/><path d="' + mouth + '" fill="#1a0804"/><path d="M530,300 Q530,214 641,214 Q752,214 752,300" fill="none" stroke="#0c0907" stroke-width="12"/>' +
        '<rect x="512" y="440" width="258" height="16" rx="2" fill="#5c554e"/><rect x="512" y="440" width="258" height="3" fill="' + t.lc + '" opacity=".55"/>';
      var f = '<defs><radialGradient id="cm-fcore" cx=".5" cy=".85" r=".8"><stop offset="0" stop-color="#fff3c0"/><stop offset=".25" stop-color="#ffc060"/><stop offset=".6" stop-color="#e0501a"/><stop offset="1" stop-color="#5a1408" stop-opacity="0"/></radialGradient><clipPath id="cm-mouth"><path d="' + mouth + '"/></clipPath></defs>' +
        '<g clip-path="url(#cm-mouth)"><rect x="520" y="200" width="240" height="250" fill="#240a04"/><g class="cm-pulse"><ellipse cx="641" cy="436" rx="130" ry="170" fill="url(#cm-fcore)"/></g><ellipse cx="641" cy="438" rx="112" ry="18" fill="#ffb050" opacity=".9"/>', r2 = rng(5), i;
      for (i = 0; i < 14; i++) f += '<ellipse cx="' + (550 + r2() * 182).toFixed(0) + '" cy="' + (430 + r2() * 12).toFixed(0) + '" rx="' + (6 + r2() * 8).toFixed(0) + '" ry="' + (3 + r2() * 3).toFixed(0) + '" fill="#ff9a30" opacity="' + (.7 + r2() * .3).toFixed(2) + '"/>';
      f += '</g><g class="cm-pulse src-nohit" style="animation-delay:-1.1s"><ellipse cx="641" cy="400" rx="220" ry="180" fill="url(#cm-glow)"/></g><ellipse class="src-nohit" cx="641" cy="300" rx="200" ry="260" fill="url(#cm-glow)" opacity=".45"/>';
      return { bake: s, back: f, front: '' };
    },
    lanterns: function (t) { return { back: '', front: '', hung: true }; },
    moonlight: function (t) { return { back: '', front: '', moon: true }; }
  };

  /* ---- Props: each knows its own slot (at) and how wide it is (w), and
     draws itself where the placer puts it. A prop with no free slot in this
     skeleton is simply not drawn, and the builder says so. 'cL' is the
     counter's left end, 'tallR' its right end, 'wallR' the wall beside the
     opening, 'ceil' the beam, 'air' open space, 'lane' the street outside,
     'over' a layer over the whole room. ---- */
  var PROP_AT = { anvil: { at: 'cL', w: 170 }, alembic: { at: 'cL', w: 130 }, jars: { at: 'cL', w: 100 }, crystal: { at: 'cL', w: 60 }, herbs: { at: 'ceil' }, 'floating-books': { at: 'air' }, orrery: { at: 'tallR' },
    ladder: { at: 'ladder' }, banner: { at: 'wallR' }, sconce: { at: 'wallR' }, sparks: { at: 'over' }, lanterns: { at: 'ceil' }, rain: { at: 'over' }, 'steam-cart': { at: 'lane' }, motes: { at: 'over' } };
  function cmGlassJar(x, y, kind, col) {
    var w = [22, 14, 28][kind], h = [34, 44, 24][kind], s = '<ellipse cx="' + f1(x) + '" cy="' + f1(y - h / 2) + '" rx="' + f1(w * 1.3) + '" ry="' + f1(h * .8) + '" fill="' + col + '" opacity=".34" filter="url(#src-blur8)"/><ellipse cx="' + f1(x + 3) + '" cy="' + f1(y) + '" rx="' + f1(w / 2 + 2) + '" ry="2" fill="#000" opacity=".4"/>';
    s += '<rect x="' + f1(x - w / 2) + '" y="' + f1(y - h) + '" width="' + w + '" height="' + h + '" rx="6" fill="' + mix(col, '#000', .5) + '"/><rect x="' + f1(x - w / 2 + 1.5) + '" y="' + f1(y - h * .7) + '" width="' + f1(w - 3) + '" height="' + f1(h * .7 - 1.5) + '" rx="5" fill="' + col + '" opacity=".85"/>' +
      '<rect x="' + f1(x - w / 2 - 1) + '" y="' + f1(y - h - 5) + '" width="' + (w + 2) + '" height="6" rx="2" fill="url(#src-cork)"/><rect x="' + f1(x - w / 2) + '" y="' + f1(y - h) + '" width="' + w + '" height="' + h + '" rx="5" fill="url(#src-glassR)"/><rect x="' + f1(x - 5) + '" y="' + f1(y - h * .45) + '" width="10" height="7" fill="#e8dcbc" opacity=".8"/>';
    return s + '<path d="M' + f1(x - w / 2 + 3) + ',' + f1(y - h * .6) + ' v-' + f1(h * .22) + '" stroke="#fff" stroke-opacity=".6" stroke-width="2" stroke-linecap="round"/>';
  }
  var PROPS = {
    anvil: function (t, p) {
      var x = p.x + 16, y = 492, A = 'url(#cm-anvil)';
      return '<defs>' + lin('cm-anvil', 1, [[0, '#6a7078'], [.15, '#2a2e34'], [1, '#0e1012']]) + '</defs><g class="src-nohit"><ellipse cx="' + x + '" cy="' + (y + 4) + '" rx="86" ry="11" fill="#000" opacity=".7" filter="url(#src-blur5)"/>' +
        '<path d="M' + (x - 36) + ',' + y + ' L' + (x + 36) + ',' + y + ' L' + (x + 30) + ',' + (y - 14) + ' L' + (x - 30) + ',' + (y - 14) + ' Z" fill="' + A + '"/>' +
        '<path d="M' + (x - 92) + ',' + (y - 50) + ' Q' + (x - 50) + ',' + (y - 48) + ' ' + (x - 30) + ',' + (y - 56) + ' L' + (x + 60) + ',' + (y - 56) + ' L' + (x + 60) + ',' + (y - 42) + ' L' + (x + 28) + ',' + (y - 40) + ' Q' + (x + 18) + ',' + (y - 36) + ' ' + (x + 18) + ',' + (y - 28) + ' L' + (x + 24) + ',' + (y - 14) + ' L' + (x - 24) + ',' + (y - 14) + ' L' + (x - 18) + ',' + (y - 28) + ' Q' + (x - 18) + ',' + (y - 38) + ' ' + (x - 30) + ',' + (y - 40) + ' Q' + (x - 56) + ',' + (y - 40) + ' ' + (x - 92) + ',' + (y - 50) + ' Z" fill="' + A + '"/>' +
        '<path d="M' + (x - 90) + ',' + (y - 50) + ' Q' + (x - 50) + ',' + (y - 48) + ' ' + (x - 30) + ',' + (y - 56) + ' L' + (x + 60) + ',' + (y - 56) + '" stroke="#d0d4da" stroke-opacity=".7" stroke-width="2" fill="none"/>' +
        '<path d="M' + (x - 90) + ',' + (y - 50) + ' Q' + (x - 50) + ',' + (y - 48) + ' ' + (x - 30) + ',' + (y - 56) + ' L' + (x + 60) + ',' + (y - 56) + '" stroke="' + t.lc + '" stroke-opacity=".5" stroke-width="1" fill="none" transform="translate(0,-1.5)"/><circle cx="' + (x + 6) + '" cy="' + (y - 57) + '" r="1.6" fill="#050506"/></g>';
    },
    alembic: function (t, p) {
      var x = p.x - 20, y = 490, g = 'url(#src-glassR)', liq = mix(t.th.accent, '#3fae6a', .45);
      var s = '<defs>' + lin('cm-liq', 1, [[0, mix(liq, '#ffffff', .35)], [1, mix(liq, '#000', .35)]]) + '</defs><g transform="translate(' + x + ',' + y + ') scale(.58) translate(' + (-x) + ',' + (-y) + ')" class="src-nohit"><ellipse cx="' + x + '" cy="' + (y + 2) + '" rx="46" ry="8" fill="url(#src-ao)"/><ellipse cx="' + (x + 64) + '" cy="' + (y + 2) + '" rx="22" ry="4" fill="url(#src-ao)"/>';
      s += '<path d="M' + (x - 26) + ',' + y + ' L' + (x - 22) + ',' + (y - 20) + ' H' + (x + 22) + ' L' + (x + 26) + ',' + y + ' Z" fill="url(#src-iron)"/><rect x="' + (x - 25) + '" y="' + (y - 24) + '" width="50" height="5" rx="2" fill="url(#cm-trim)"/>' +
        '<path class="src-flame" d="M' + x + ',' + (y - 52) + ' c-9,12 -10,22 0,30 c10,-8 9,-18 0,-30 Z" fill="url(#src-flame)"/>';
      s += '<path d="M' + (x + 16) + ',' + (y - 138) + ' C' + (x + 50) + ',' + (y - 146) + ' ' + (x + 62) + ',' + (y - 136) + ' ' + (x + 64) + ',' + (y - 112) + ' V' + (y - 52) + '" stroke="#5e4318" stroke-width="8" fill="none"/><path d="M' + (x + 16) + ',' + (y - 138) + ' C' + (x + 50) + ',' + (y - 146) + ' ' + (x + 62) + ',' + (y - 136) + ' ' + (x + 64) + ',' + (y - 112) + ' V' + (y - 52) + '" stroke="#d9a85a" stroke-width="5" fill="none"/>';
      s += '<circle cx="' + (x + 64) + '" cy="' + (y - 20) + '" r="18" fill="' + liq + '" fill-opacity=".2"/><path d="M' + (x + 47) + ',' + (y - 14) + ' A18,18 0 1 0 ' + (x + 81) + ',' + (y - 14) + ' Z" fill="url(#cm-liq)"/><circle cx="' + (x + 64) + '" cy="' + (y - 20) + '" r="18" fill="' + g + '"/><circle cx="' + (x + 64) + '" cy="' + (y - 20) + '" r="18" fill="url(#src-ballR)"/><circle class="cm-drip" cx="' + (x + 64) + '" cy="' + (y - 56) + '" r="2" fill="' + mix(liq, '#ffffff', .5) + '"/>';
      s += '<circle cx="' + x + '" cy="' + (y - 82) + '" r="32" fill="' + liq + '" fill-opacity=".16"/><path d="M' + (x - 30.4) + ',' + (y - 92) + ' A32,32 0 1 0 ' + (x + 30.4) + ',' + (y - 92) + ' Z" fill="url(#cm-liq)"/><ellipse cx="' + x + '" cy="' + (y - 92) + '" rx="30.4" ry="3" fill="' + mix(liq, '#ffffff', .55) + '" opacity=".75"/>';
      [[-14, 0, 0], [8, 6, .5], [-2, -4, 1.1], [16, 2, 1.6], [-20, 8, .8]].forEach(function (b, i) { s += '<circle class="cm-bub" cx="' + (x + b[0]) + '" cy="' + (y - 62 + b[1]) + '" r="' + (2 + i % 3 * .7) + '" fill="#f4ffd0" fill-opacity=".8" style="animation-delay:' + b[2] + 's"/>'; });
      return s + '<rect x="' + (x - 8) + '" y="' + (y - 134) + '" width="16" height="28" fill="' + g + '"/><circle cx="' + x + '" cy="' + (y - 82) + '" r="32" fill="' + g + '"/><circle cx="' + x + '" cy="' + (y - 82) + '" r="32" fill="url(#src-ballR)"/>' +
        '<path d="M' + (x - 20) + ',' + (y - 134) + ' q0,-22 20,-24 q20,2 20,24 Z" fill="url(#cm-trim)"/><path d="M' + (x - 24) + ',' + (y - 96) + ' a28,28 0 0 1 8,-24" stroke="#fff" stroke-opacity=".6" stroke-width="3" stroke-linecap="round" fill="none"/></g>';
    },
    jars: function (t, p) {
      var cols = [t.th.accent, '#3fae6a', '#4a82c8', '#d24a3a'], s = '<g class="src-nohit">', x = p.x - 38, i, k = [0, 1, 2, 1];
      s += '<ellipse cx="' + p.x + '" cy="494" rx="60" ry="6" fill="#000" opacity=".45" filter="url(#src-blur3)"/>';
      for (i = 0; i < 4; i++) { s += cmGlassJar(x, 490, k[i], cols[i]); x += [22, 18, 28, 18][i] + 8; }
      return s + '</g>';
    },
    crystal: function (t, p) {
      var x = p.x, y = 490, c = t.th.accent;
      return '<g class="src-nohit"><ellipse cx="' + x + '" cy="' + (y + 2) + '" rx="34" ry="6" fill="url(#src-ao)"/><circle cx="' + x + '" cy="' + (y - 50) + '" r="56" fill="url(#cm-acc)" class="cm-pulse"/>' +
        '<path d="M' + (x - 20) + ',' + y + ' Q' + (x - 12) + ',' + (y - 16) + ' ' + x + ',' + (y - 18) + ' Q' + (x + 12) + ',' + (y - 16) + ' ' + (x + 20) + ',' + y + ' Z" fill="url(#cm-trim)"/>' +
        '<path d="M' + x + ',' + (y - 96) + ' L' + (x + 20) + ',' + (y - 62) + ' L' + (x + 14) + ',' + (y - 22) + ' L' + x + ',' + (y - 14) + ' L' + (x - 14) + ',' + (y - 22) + ' L' + (x - 20) + ',' + (y - 62) + ' Z" fill="' + mix(c, '#ffffff', .35) + '" opacity=".92"/>' +
        '<path d="M' + x + ',' + (y - 96) + ' L' + (x + 20) + ',' + (y - 62) + ' L' + x + ',' + (y - 56) + ' Z" fill="#fff" opacity=".45"/><path d="M' + x + ',' + (y - 56) + ' L' + (x + 20) + ',' + (y - 62) + ' L' + (x + 14) + ',' + (y - 22) + ' L' + x + ',' + (y - 14) + ' Z" fill="' + mix(c, '#000', .35) + '" opacity=".55"/>' +
        '<path d="M' + x + ',' + (y - 96) + ' L' + (x - 20) + ',' + (y - 62) + ' L' + x + ',' + (y - 56) + ' Z" fill="' + mix(c, '#ffffff', .7) + '" opacity=".6"/></g>';
    },
    herbs: function (t, p) {
      var hue = [['#5f8a3c', '#789a46', '#4a7a3a'], ['#8a8a46', '#a89a58', '#6e7a3a'], ['#7a6aa8', '#8a7ab8', '#5f6a8a'], ['#a8884a', '#8a7038', '#b89a58']], s = '', xs = [706, 750, 1104, 1146, 1182], lens = [50, 62, 66, 52, 60];
      xs.forEach(function (x, j) {
        var r = rng(3 + j * 5), H = hue[j % 4], len = lens[j], i, a, L, ex, ey;
        s += '<path d="M' + x + ',34 V46" stroke="#5a4020" stroke-width="1.6"/><ellipse cx="' + (x + 5) + '" cy="' + f1(50 + len * .5) + '" rx="12" ry="' + f1(len * .5) + '" fill="#000" opacity=".3" filter="url(#src-blur5)"/>';
        for (i = 0; i < 11; i++) {
          a = (i - 5) * .1 + (r() - .5) * .06; L = len * (.7 + r() * .3); ex = x + Math.sin(a) * L; ey = 48 + Math.cos(a) * L;
          s += '<path d="M' + x + ',48 Q' + f1(x + Math.sin(a) * L * .4) + ',' + f1(48 + L * .5) + ' ' + f1(ex) + ',' + f1(ey) + '" stroke="' + mix(H[i % 3], '#000', .1 + r() * .3) + '" stroke-width="' + f1(1.6 + r() * 1.2) + '" fill="none"/>';
          if (i % 2) s += '<ellipse cx="' + f1(ex) + '" cy="' + f1(ey - 4) + '" rx="3" ry="' + f1(5 + r() * 4) + '" fill="' + mix(H[(i + 1) % 3], '#ffffff', r() * .15) + '" transform="rotate(' + f1(a * 57) + ' ' + f1(ex) + ' ' + f1(ey) + ')"/>';
        }
        s += '<rect x="' + (x - 4) + '" y="45" width="8" height="6" rx="2" fill="#8a6a3a"/>';
      });
      return '<g class="src-nohit">' + s + '</g>';
    },
    'floating-books': function (t, p) {
      var s = '', pts = [[514, 376, -12, 2, .8], [1108, 70, 8, 4, 1], [790, 112, -6, 3, .72]];
      pts.forEach(function (q) {
        s += '<g class="src-nohit" transform="translate(' + q[0] + ',' + q[1] + ') rotate(' + q[2] + ') scale(' + q[4] + ')"><g class="cm-bob" style="animation-duration:' + (6 + q[3] % 3) + 's;animation-delay:-' + q[3] + 's"><circle cx="0" cy="-8" r="52" fill="url(#cm-acc)" opacity=".4"/>' +
          '<path d="M-42,4 Q0,20 42,4 L42,10 Q0,26 -42,10 Z" fill="' + mix(t.th.cloth, '#000', .3) + '"/><path d="M-40,2 Q-20,-8 0,2 L0,-26 Q-20,-34 -40,-24 Z" fill="#e8dcbc"/><path d="M40,2 Q20,-8 0,2 L0,-26 Q20,-34 40,-24 Z" fill="#d8ccac"/>' +
          '<path d="M-34,-16 q14,-4 28,2 M-34,-9 q14,-4 28,2 M6,-14 q14,-6 28,-2 M6,-7 q14,-6 28,-2" stroke="#6a5a48" stroke-width="1" fill="none"/><path d="M-40,2 Q0,18 40,2" stroke="' + t.th.accent + '" stroke-width="1.4" fill="none"/>' +
          '<path d="M0,-40 l1.4,5.6 l5.6,1.4 l-5.6,1.4 l-1.4,5.6 l-1.4,-5.6 l-5.6,-1.4 l5.6,-1.4 Z" fill="' + mix(t.th.accent, '#ffffff', .6) + '"/></g></g>';
      });
      return s;
    },
    orrery: function (t, p) {
      var x = p.x, y = 246, s = '<g data-k="orrery"><rect x="' + (x - 96) + '" y="' + (y - 40) + '" width="192" height="80" fill="transparent"/>';
      s += '<ellipse cx="' + x + '" cy="470" rx="44" ry="7" fill="url(#src-ao)"/><rect x="' + (x - 4) + '" y="' + (y + 10) + '" width="8" height="220" fill="url(#cm-trim)"/><path d="M' + (x - 40) + ',470 Q' + (x - 30) + ',432 ' + (x - 7) + ',420 L' + (x + 7) + ',420 Q' + (x + 30) + ',432 ' + (x + 40) + ',470 Z" fill="url(#cm-trim)"/><ellipse cx="' + x + '" cy="' + (y + 120) + '" rx="12" ry="4" fill="url(#cm-trimv)"/>';
      [[40, 12, -18, '#7fb6ff', 3.6, 34, 0], [66, 20, 12, mix(t.th.accent, '#ffffff', .2), 4.4, 52, 6], [90, 27, -6, '#c4b0ff', 3.4, 78, 2]].forEach(function (q) {
        var k = q[1] / q[0];
        s += '<g transform="translate(' + x + ',' + y + ') rotate(' + q[2] + ')"><ellipse rx="' + q[0] + '" ry="' + q[1] + '" fill="none" stroke="#000" stroke-opacity=".35" stroke-width="3.6" transform="translate(0,1.5)"/><ellipse rx="' + q[0] + '" ry="' + q[1] + '" fill="none" stroke="url(#cm-trim)" stroke-width="2.4"/>' +
          '<g transform="scale(1,' + k.toFixed(3) + ')"><g class="cm-orb" style="animation-duration:' + q[5] + 's;animation-delay:-' + q[6] + 's"><g transform="translate(' + q[0] + ',0) scale(1,' + (1 / k).toFixed(3) + ')"><circle r="' + q[4] + '" fill="' + q[3] + '"/><circle r="' + q[4] + '" fill="url(#src-ballR)"/></g></g></g></g>';
      });
      return s + '<circle cx="' + x + '" cy="' + y + '" r="46" fill="url(#cm-acc)" opacity=".5" class="src-nohit"/><circle cx="' + x + '" cy="' + y + '" r="11" fill="' + mix(t.th.accent, '#ffffff', .35) + '"/><circle cx="' + x + '" cy="' + y + '" r="11" fill="none" stroke="#6a4a14" stroke-width="1"/></g>';
    },
    ladder: function (t, p) {
      var w = mix(t.th.wood, '#ffffff', .05), s = '<g class="src-nohit"><rect x="30" y="80" width="520" height="6" fill="url(#cm-trim)"/><circle cx="520" cy="83" r="8" fill="url(#cm-trimv)"/>';
      s += '<path d="M512,92 L492,452 M546,92 L526,452" stroke="#000" stroke-opacity=".4" stroke-width="9" transform="translate(5,2)" filter="url(#src-blur3)"/><path d="M512,92 L492,452 M546,92 L526,452" stroke="' + w + '" stroke-width="7"/><path d="M510,92 L490,452 M544,92 L524,452" stroke="#fff" stroke-opacity=".12" stroke-width="1.6"/>';
      for (var i = 1; i < 11; i++) { var y = 92 + i * 33, x0 = 512 - (y - 92) * .0556, x1 = 546 - (y - 92) * .0556; s += '<path d="M' + f1(x0) + ',' + y + ' H' + f1(x1) + '" stroke="' + mix(w, '#000', .15) + '" stroke-width="5"/>'; }
      return s + '</g>';
    },
    banner: function (t, p) {
      var x = p.x, y = 62, a = t.th.accent, c = t.th.cloth;
      return '<g class="src-nohit"><rect x="' + (x - 46) + '" y="' + (y - 6) + '" width="92" height="6" rx="3" fill="url(#cm-trim)"/><circle cx="' + (x - 46) + '" cy="' + (y - 3) + '" r="5" fill="url(#cm-trimv)"/><circle cx="' + (x + 46) + '" cy="' + (y - 3) + '" r="5" fill="url(#cm-trimv)"/>' +
        '<g class="cm-sway" style="transform-origin:' + x + 'px ' + y + 'px"><path d="M' + (x - 38) + ',' + y + ' H' + (x + 38) + ' V' + (y + 200) + ' L' + x + ',' + (y + 170) + ' L' + (x - 38) + ',' + (y + 200) + ' Z" fill="' + mix(c, '#000', .15) + '"/><path d="M' + (x - 38) + ',' + y + ' H' + (x + 38) + ' V' + (y + 200) + ' L' + x + ',' + (y + 170) + ' L' + (x - 38) + ',' + (y + 200) + ' Z" fill="#fff" opacity=".07" filter="url(#src-cloth)"/>' +
        '<path d="M' + (x - 32) + ',' + (y + 6) + ' H' + (x + 32) + ' V' + (y + 186) + ' L' + x + ',' + (y + 160) + ' L' + (x - 32) + ',' + (y + 186) + ' Z" fill="none" stroke="' + a + '" stroke-width="2.4"/>' +
        '<path d="M' + x + ',' + (y + 40) + ' l22,32 l-22,32 l-22,-32 Z" fill="' + a + '"/><path d="M' + x + ',' + (y + 52) + ' l11,20 l-11,20 l-11,-20 Z" fill="' + c + '"/><circle cx="' + x + '" cy="' + (y + 72) + '" r="5" fill="' + mix(a, '#ffffff', .5) + '"/>' +
        '<path d="M' + (x - 24) + ',' + (y + 124) + ' H' + (x + 24) + '" stroke="' + a + '" stroke-width="2"/></g></g>';
    },
    sconce: function (t, p) {
      var x = p.x, y = 340;
      return '<g class="src-nohit"><rect x="' + (x - 4) + '" y="' + (y + 24) + '" width="8" height="30" fill="url(#src-iron)"/><path d="M' + (x - 14) + ',' + (y + 24) + ' h28 l-4,6 h-20 Z" fill="url(#cm-trim)"/>' + cmCandle(x, y + 24, 24, true) + '<circle cx="' + x + '" cy="' + (y - 6) + '" r="46" fill="url(#cm-glow)" data-flick=".5"/></g>';
    },
    lanterns: function (t, p) { return t.th.light === 'lanterns' ? '' : cmLanterns(t, false); },
    'steam-cart': function (t, p) {
      var x = p.x, y = p.y, s = '<g class="src-nohit"><ellipse cx="' + x + '" cy="' + (y + 4) + '" rx="42" ry="6" fill="#000" opacity=".4"/>';
      s += '<rect x="' + (x - 34) + '" y="' + (y - 34) + '" width="68" height="28" rx="3" fill="' + mix(t.th.cloth, '#000', .25) + '"/><path d="M' + (x - 40) + ',' + (y - 34) + ' Q' + x + ',' + (y - 62) + ' ' + (x + 40) + ',' + (y - 34) + ' Z" fill="' + t.th.accent + '"/><path d="M' + (x - 40) + ',' + (y - 34) + ' H' + (x + 40)+ '" stroke="#000" stroke-opacity=".4" stroke-width="2"/>';
      s += '<circle cx="' + (x - 22) + '" cy="' + (y - 4) + '" r="9" fill="#14110e"/><circle cx="' + (x + 22) + '" cy="' + (y - 4) + '" r="9" fill="#14110e"/><circle cx="' + (x - 22) + '" cy="' + (y - 4) + '" r="3" fill="' + t.th.trim + '"/><circle cx="' + (x + 22) + '" cy="' + (y - 4) + '" r="3" fill="' + t.th.trim + '"/>';
      s += '<rect x="' + (x - 28) + '" y="' + (y - 30) + '" width="24" height="16" fill="' + t.lc + '" opacity=".6"/><rect x="' + (x + 24) + '" y="' + (y - 52) + '" width="6" height="22" fill="#1a1612"/>';
      for (var i = 0; i < 4; i++) s += '<ellipse class="cm-steam" cx="' + (x + 27 + (i % 2) * 6) + '" cy="' + (y - 54) + '" rx="' + (7 + i * 2) + '" ry="' + (6 + i * 2) + '" fill="#e8f0ff" opacity=".4" style="animation-delay:' + (-i) + 's"/>';
      return s + '</g>';
    },
    sparks: function (t, p) {
      var s = '', r = rng(19), i, ox = p.x, oy = p.y, dx;
      for (i = 0; i < 12; i++) { dx = ((r() - .5) * 70).toFixed(0); s += '<circle class="cm-spark src-nohit" cx="' + (ox + (r() - .5) * 90).toFixed(0) + '" cy="' + (oy - r() * 20).toFixed(0) + '" r="' + (1.3 + r() * 1.5).toFixed(1) + '" fill="' + mix('#ffd27a', t.th.accent, .25) + '" style="--dx:' + dx + 'px;animation-delay:' + (-r() * 3.4).toFixed(2) + 's;animation-duration:' + (2.6 + r() * 2).toFixed(1) + 's"/>'; }
      return '<g class="src-nohit">' + s + '</g>';
    },
    rain: function (t, p) {
      var s = '', i, n = 18;
      if (!t.air) return '';
      for (i = 0; i < n; i++) if (i % 3 !== 2) s += '<ellipse class="cm-drip" cx="' + f1(56 + i * 60 + 30) + '" cy="64" rx="1.3" ry="2.6" fill="#cfe2ff" opacity=".9" style="animation-delay:' + (-(i * 0.37 % 1.5)).toFixed(2) + 's"/>';
      return '<g class="src-nohit">' + s + '</g>';
    },
    motes: function (t, p) {
      var s = '', r = rng(61), i, pts = [[470, 330], [520, 360], [760, 300], [860, 390], [990, 330], [430, 140], [1030, 200], [700, 420], [610, 120], [1000, 420]];
      for (i = 0; i < pts.length; i++) s += '<circle class="cm-mote" cx="' + pts[i][0] + '" cy="' + pts[i][1] + '" r="' + (1.3 + r() * 1.2).toFixed(1) + '" fill="' + mix(t.th.accent, '#ffffff', .6) + '" style="animation-duration:' + (7 + r() * 5).toFixed(1) + 's;animation-delay:-' + (r() * 9).toFixed(1) + 's"/>';
      return '<g class="src-nohit">' + s + '</g>';
    }
  };

  /* Where each prop lands, from the skeleton alone. Counter props fill the
     counter's left end from the wall inward; whatever does not fit is left
     out, never squeezed onto the goods. */
  function cmPlace(t) {
    var out = {}, cur = 48, end = t.domes ? 0 : 290, orr = t.props.indexOf('orrery') >= 0;
    t.props.forEach(function (p) {
      var m = PROP_AT[p], a = m.at;
      if (a === 'cL') { if (cur + m.w <= end) { out[p] = { x: cur + m.w / 2, y: 492 }; cur += m.w + 6; } else out[p] = null; }
      else if (a === 'tallR') out[p] = t.domes ? null : { x: 1090, y: 470 };
      else if (a === 'wallR') out[p] = p === 'banner' && orr ? null : { x: 1130, y: 0 };
      else if (a === 'ladder') out[p] = t.domes ? null : { x: 0, y: 0 };
      else if (a === 'lane') out[p] = t.air ? { x: 860, y: 372 } : t.sk.opening === 'doorway' ? { x: 910, y: 438 } : null;
      else if (a === 'over') out[p] = { x: 0, y: 0 };
      else out[p] = { x: 0, y: 0 };
    });
    if (out.sparks) {
      var An = out.anvil;
      out.sparks = An ? { x: t.mx(An.x + 16), y: 430 } : t.th.light === 'forge' ? { x: 641, y: 430 } : { x: t.mx(404), y: 440 };
    }
    t.ledger = !t.domes && cur === 48;
    return out;
  }
  function cmM(t, s) { return t.mir && s ? '<g transform="translate(1200,0) scale(-1,1)">' + s + '</g>' : s; }
  var CM_SLOTS = { cL: [44, 400, 242, 100, 'Counter, left end'], tallR: [1040, 150, 150, 330, 'Counter, right end'], wallR: [1076, 56, 110, 330, 'Wall by the opening'], ceil: [500, 30, 680, 110, 'Beam'], air: [440, 300, 140, 90, 'In the air'], ladder: [476, 90, 84, 360, 'Against the shelves'], lane: [806, 340, 200, 100, 'Out in the lane'], over: [760, 392, 210, 46, 'Whole room'] };

  /* ---- The view itself: layers from the back forward, then the same play
     as the plain counter. ---- */
  function composedView(R) {
    var t = cmCtx(R.model.counter || starterFor(R.model.roomType)), D = DISPLAYS[t.sk.display](t), P = cmPlace(t), LG = LIGHTS[t.th.light](t), SP = cmSpots(t), mx = t.mx;
    var CLOTH = { x: 600, y: 476 }, KEEP = { x: 641, y: 250, r: 92 }, LP = t.lp, edit = !!R.model.edit;
    var PILE0 = [[742, 488, 1.12], [778, 486, 1.12], [814, 489, 1.12], [850, 487, 1.12], [886, 488, 1.12], [728, 506, 1.24], [766, 508, 1.24], [804, 505, 1.24], [842, 507, 1.24], [880, 506, 1.24]];
    var PILE = PILE0.map(function (p) { return [mx(p[0]), p[1], p[2]]; }), PX0 = Math.min(mx(700), mx(920)), PX1 = Math.max(mx(700), mx(920));
    var held = null, parcel = 0, sold = 0, lastHeld = '', parcelT = 0;
    var SL = D.slots.map(function (s) { return [mx(s[0]), s[1], s[2], D.counterTop ? (mx(s[0]) < LP.x ? 'R' : 'L') : t.side]; });
    var placed = {}; t.props.forEach(function (p) { placed[p] = !!P[p]; }); R.placed(placed);
    function spot(i) { return SL[Math.min(i, SL.length - 1)]; }
    function shelfIt() { return R.place.shelf; }
    function defs() { return cmDefs(t); }
    // A soft shadow on the counter from x,y, running away from the light and foreshortened like the top.
    function cast(x, y, w, len, op) {
      var a = Math.atan2((y - LP.y - 70) * 2, x - LP.x) * 180 / Math.PI;
      return '<g class="src-nohit" transform="translate(' + f1(x) + ',' + f1(y) + ') scale(1,.42) rotate(' + a.toFixed(1) + ')" opacity="' + (op || 1) + '"><path d="M0,' + (-w / 2) + ' L' + len + ',' + (-w * .9) + ' L' + len + ',' + (w * .9) + ' L0,' + (w / 2) + ' Z" fill="url(#src-cast)" filter="url(#src-blur5)"/></g>';
    }
    // On a wall unit a good's shadow falls on the board behind, away from the light.
    function wallShadow(x, y, w, h) { return '<rect x="' + f1(x - w / 2 + (x - LP.x) * .06) + '" y="' + f1(y - h + (y - LP.y) * .05) + '" width="' + w + '" height="' + h + '" rx="' + f1(w / 2.4) + '" fill="#000" opacity=".36" filter="url(#src-blur5)"/>'; }

    // Spots with nothing for sale hold the keeper's own stock, so a small shop still looks like a working one.
    function decor() {
      var s = '', r = rng(R.seed + 3), n = shelfIt().length;
      if (!D.decor && !D.outline) return '';
      for (var i = n; i < 12; i++) {
        var p = spot(i), x = p[0] + (r() - .5) * 20, y = p[1], k = r();
        if (D.outline) {
          s += '<g stroke="#e8dcc0" stroke-opacity=".38" stroke-width="2.2" fill="none" stroke-dasharray="5 4"><path d="M' + f1(x - 22) + ',' + (y - 70) + ' h30 v14 h-12 v50 h-8 v-50 h-10 Z"/></g>';
          continue;
        }
        if (k < .55) { var w = 22 + r() * 14, h = 30 + r() * 18, col = [mix(t.th.cloth, '#000', .1), '#2a4a5a', '#7a6a2a', '#3a5a7a', '#5a3a6a', '#2a5a3a'][Math.floor(r() * 6)];
          s += (D.wallsh ? wallShadow(x, y, w, h) : '') + '<rect x="' + f1(x - w / 2) + '" y="' + f1(y - h) + '" width="' + f1(w) + '" height="' + f1(h) + '" rx="5" fill="' + col + '"/><rect x="' + f1(x - w / 2) + '" y="' + f1(y - h) + '" width="' + f1(w) + '" height="' + f1(h) + '" rx="5" fill="url(#src-glass' + t.side + ')"/>' +
            '<rect x="' + f1(x - w / 2 - 2) + '" y="' + f1(y - h - 6) + '" width="' + f1(w + 4) + '" height="8" rx="2" fill="url(#src-cork)"/><rect x="' + f1(x - w / 3) + '" y="' + f1(y - h * .6) + '" width="' + f1(w / 1.5) + '" height="' + f1(h * .3) + '" fill="#e8dcbc" opacity=".75"/>'; }
        else if (k < .85) [[mix(t.th.cloth, '#000', .2), 12, 58], ['#1d3a24', 10, 52], ['#1a2644', 14, 62], [mix(t.th.accent, '#000', .55), 9, 48]].forEach(function (b, j) { var bx = x - 20 + j * 12; s += '<rect x="' + f1(bx) + '" y="' + (y - b[2]) + '" width="' + b[1] + '" height="' + b[2] + '" fill="' + b[0] + '"/><rect x="' + f1(bx) + '" y="' + (y - b[2] + 8) + '" width="' + b[1] + '" height="2" fill="' + t.th.trim + '" opacity=".6"/>'; });
        else s += (D.wallsh ? wallShadow(x, y, 46, 26) : '') + '<path d="M' + f1(x - 24) + ',' + (y - 26) + ' h48 q-2,22 -16,26 h-16 q-14,-4 -16,-26 Z" fill="#6a6e76"/><ellipse cx="' + f1(x) + '" cy="' + (y - 26) + '" rx="24" ry="5" fill="#2a2c30"/>';
      }
      return '<g class="src-dof src-nohit">' + s + '</g>';
    }
    function wareOnShelf(it, i) {
      var p = spot(i), x = p[0], y = p[1], sc = p[2], side = p[3], k = 'ware:' + it.id, left = R.stock(it), here = held && String(held.id) === String(it.id), hw = 28 * sc, hh = 64 * sc;
      var hit = '<rect x="' + f1(x - hw) + '" y="' + f1(y - hh) + '" width="' + f1(hw * 2) + '" height="' + f1(hh + 2) + '" fill="transparent"/>', base = D.base ? D.base(it, p) : '', front = D.front ? D.front(it, p) : '';
      if (it.out || left === 0 && !here) return '<g data-k="' + k + '" class="src-out">' + hit + base + '<ellipse cx="' + x + '" cy="' + (y - 2) + '" rx="14" ry="2.6" fill="none" stroke="#c8b898" stroke-opacity=".35" stroke-width="1.2"/>' +
        '<g transform="translate(' + x + ',' + (y - 16) + ') rotate(-6)"><rect x="-22" y="-8" width="44" height="14" fill="#e8dcbc"/><text x="0" y="2" text-anchor="middle" class="src-tag">' + (it.out ? 'sold out' : 'all chosen') + '</text></g></g>';
      if (here && left !== null && left < 2) return '<g data-k="' + k + '" class="src-ware">' + hit + base + '<ellipse cx="' + x + '" cy="' + (y - 1) + '" rx="20" ry="3" fill="#000" opacity=".25"/>' + front + '</g>';
      var s = '<g data-k="' + k + '" class="src-ware">' + hit + base;
      if (left === null || left > 1) s += '<g transform="translate(' + f1(x + 13 * sc) + ',' + (y - 2) + ') scale(' + f1(sc * .87) + ')" opacity=".55" filter="url(#src-dof)">' + ware(it, side, R.icons) + '</g>';
      if (!here) s += (D.wallsh ? wallShadow(x, y, 46, 70) : D.counterTop ? cast(x, y, 40, 90, .7) : '') + '<g transform="translate(' + x + ',' + y + ') scale(' + sc + ')">' + ware(it, side, R.icons) + '</g>';
      s += front;
      if (it.today) s += todayTag(x, y + 2, i);
      return s + '</g>';
    }
    /* Today's goods: a paper tag hangs under each and sways, and a chalk slate says when they go. */
    function todayTag(x, y, i) {
      return '<g class="src-today src-nohit" style="transform-origin:' + x + 'px ' + y + 'px;animation-delay:' + (i * -1.3).toFixed(1) + 's">' +
        '<path d="M' + x + ',' + y + ' L' + (x - 2) + ',' + (y + 14) + '" stroke="#c9b48a" stroke-width="1"/>' +
        '<g transform="translate(' + (x - 2) + ',' + (y + 14) + ') rotate(-6)"><path d="M-22,0 L18,0 L24,8 L18,16 L-22,16 Z" fill="#f1e6cc"/><circle cx="17" cy="8" r="1.8" fill="#8a6a40"/>' +
        '<text x="-2" y="12" text-anchor="middle" class="src-todayt">Today</text></g></g>';
    }
    function slate() {
      var td = R.model.today;
      if (!td || !shelfIt().some(function (it) { return it.today; })) return '';
      var sx = t.mir ? 1200 - 538 - 140 : 538;
      return '<g data-k="slate" class="src-slate" transform="translate(' + sx + ',94)">' +
        '<path d="M66,-14 L66,-2" stroke="#5a3e26" stroke-width="1.5"/><circle cx="66" cy="-16" r="3" fill="#8a6a40"/>' +
        '<rect x="-4" y="-3" width="140" height="56" rx="5" fill="' + mix(t.th.wood, '#000', .1) + '"/><rect x="1" y="2" width="130" height="46" rx="3" fill="#24282a"/>' +
        '<text x="66" y="21" text-anchor="middle" class="src-chalk">Only today</text>' +
        '<text x="66" y="40" text-anchor="middle" class="src-chalk src-chalk2">' + esc(td.line) + '</text></g>';
    }
    function wares() { return shelfIt().map(wareOnShelf).join('') + slate(); }
    /* The keeper as a token: the shop page's picture in a ring on a stand,
       or a plain silhouette when the page has none. The ring takes the
       theme's trim colour. */
    function keeper() {
      var c = KEEP, url = safeUrl(R.model.image), face;
      face = url ? '<rect x="' + (c.x - c.r) + '" y="' + (c.y - c.r) + '" width="' + 2 * c.r + '" height="' + 2 * c.r + '" fill="#2a2018"/><image href="' + esc(url) + '" x="' + (c.x - c.r) + '" y="' + (c.y - c.r) + '" width="' + 2 * c.r + '" height="' + 2 * c.r + '" preserveAspectRatio="xMidYMid slice"/>'
        : '<rect x="' + (c.x - c.r) + '" y="' + (c.y - c.r) + '" width="' + 2 * c.r + '" height="' + 2 * c.r + '" fill="#4a3a2c"/><circle cx="' + c.x + '" cy="' + (c.y - 18) + '" r="34" fill="url(#cm-sil)"/><path d="M' + (c.x - 70) + ',' + (c.y + c.r) + ' C' + (c.x - 66) + ',' + (c.y + 30) + ' ' + (c.x - 30) + ',' + (c.y + 16) + ' ' + c.x + ',' + (c.y + 16) + ' C' + (c.x + 30) + ',' + (c.y + 16) + ' ' + (c.x + 66) + ',' + (c.y + 30) + ' ' + (c.x + 70) + ',' + (c.y + c.r) + ' Z" fill="url(#cm-sil)"/>';
      return '<g data-k="keeper" class="srcb-keeper"><circle cx="' + (c.x + 56) + '" cy="' + (c.y - 20) + '" r="' + c.r + '" fill="#000" opacity=".3" filter="url(#src-blur14)"/>' +
        '<rect x="' + (c.x - 3) + '" y="' + (c.y + c.r) + '" width="6" height="' + (452 - c.y - c.r) + '" fill="url(#cm-trim)"/>' +
        '<g class="srcb-body"><g class="srcb-head"><g clip-path="url(#cm-tclip)">' + face + '</g>' +
        '<circle cx="' + c.x + '" cy="' + c.y + '" r="' + c.r + '" fill="none" stroke="url(#cm-trim)" stroke-width="9"/><circle cx="' + c.x + '" cy="' + c.y + '" r="' + (c.r - 5) + '" fill="none" stroke="#000" stroke-opacity=".5" stroke-width="2"/>' +
        '<path d="M' + f1(c.x - c.r * .8) + ',' + f1(c.y - c.r * .5) + ' A' + c.r + ',' + c.r + ' 0 0 1 ' + f1(c.x + c.r * .2) + ',' + f1(c.y - c.r * .98) + '" stroke="#fff6dc" stroke-opacity=".45" stroke-width="2" fill="none"/></g></g></g>';
    }
    function cloth() {
      var c = CLOTH, d = 'M' + (c.x - 96) + ',' + (c.y - 16) + ' L' + (c.x + 96) + ',' + (c.y - 16) + ' L' + (c.x + 112) + ',' + (c.y + 24) + ' L' + (c.x - 112) + ',' + (c.y + 24) + ' Z';
      return '<g data-k="cloth"><path d="' + d + '" fill="url(#cm-clothmat)"/><path d="' + d + '" fill="#fff" opacity=".06" filter="url(#src-velvet)"/><path d="M' + (c.x - 110) + ',' + (c.y + 22) + ' L' + (c.x + 110) + ',' + (c.y + 22) + '" stroke="' + t.th.trim + '" stroke-opacity=".55" stroke-width="1.4"/>' +
        '<path d="M' + (c.x - 92) + ',' + (c.y - 10) + ' L' + (c.x + 92) + ',' + (c.y - 10) + ' L' + (c.x + 104) + ',' + (c.y + 18) + ' L' + (c.x - 104) + ',' + (c.y + 18) + ' Z" fill="none" stroke="' + t.th.accent + '" stroke-opacity=".5" stroke-width="1.2" stroke-dasharray="5 4"/></g>';
    }
    function bell() {
      var x = SP.bell[0], y = SP.bell[1];
      return '<g data-k="bell"><ellipse cx="' + x + '" cy="' + (y + 1) + '" rx="26" ry="5" fill="url(#src-ao)"/><g class="srcb-bell"><path d="M' + (x - 22) + ',' + y + ' C' + (x - 22) + ',' + (y - 16) + ' ' + (x - 14) + ',' + (y - 30) + ' ' + x + ',' + (y - 30) + ' C' + (x + 14) + ',' + (y - 30) + ' ' + (x + 22) + ',' + (y - 16) + ' ' + (x + 22) + ',' + y + ' Z" fill="url(#cm-trim)"/>' +
        '<rect x="' + (x - 24) + '" y="' + (y - 3) + '" width="48" height="5" rx="2" fill="url(#cm-trimv)"/><circle cx="' + x + '" cy="' + (y - 33) + '" r="4" fill="url(#cm-trim)"/></g></g>';
    }
    function scales() {
      if (!SP.scales) return '';
      var x = SP.scales[0], y = SP.scales[1], k = t.mir ? -1 : 1;
      return '<g data-k="scales"><ellipse cx="' + x + '" cy="' + (y + 2) + '" rx="46" ry="7" fill="url(#src-ao)"/><rect x="' + (x - 30) + '" y="' + (y - 8) + '" width="60" height="9" rx="2" fill="' + mix(t.th.wood, '#000', .35) + '"/>' +
        '<rect x="' + (x - 3) + '" y="' + (y - 96) + '" width="6" height="90" fill="url(#cm-trim)"/><g class="srcb-beam' + (sold ? ' tip' : '') + '" style="transform-origin:' + x + 'px ' + (y - 98) + 'px"><rect x="' + (x - 62) + '" y="' + (y - 98) + '" width="124" height="4" rx="2" fill="url(#cm-trimv)"/>' +
        '<g class="srcb-panL"><path d="M' + (x - 58) + ',' + (y - 94) + ' L' + (x - 74) + ',' + (y - 46) + ' M' + (x - 58) + ',' + (y - 94) + ' L' + (x - 42) + ',' + (y - 46) + '" stroke="#a88a4a" stroke-width="1"/><path d="M' + (x - 80) + ',' + (y - 46) + ' q22,12 44,0 Z" fill="url(#cm-trim)"/></g>' +
        '<g class="srcb-panR"><path d="M' + (x + 58) + ',' + (y - 94) + ' L' + (x + 42) + ',' + (y - 46) + ' M' + (x + 58) + ',' + (y - 94) + ' L' + (x + 74) + ',' + (y - 46) + '" stroke="#a88a4a" stroke-width="1"/><path d="M' + (x + 36) + ',' + (y - 46) + ' q22,12 44,0 Z" fill="url(#cm-trim)"/><g class="srcb-coins">' + coins() + '</g></g></g>' +
        '<circle cx="' + x + '" cy="' + (y - 98) + '" r="5" fill="url(#cm-trim)"/></g>';
    }
    function coins() { if (!SP.scales) return ''; var s = '', x = SP.scales[0] + (t.mir ? -58 : 58); for (var j = 0; j < Math.min(10, sold); j++) s += '<ellipse cx="' + (x + (j % 3 - 1) * 7) + '" cy="' + (SP.scales[1] - 49 - Math.floor(j / 3) * 3) + '" rx="7" ry="2.6" fill="#d6aa4a" stroke="#7a5a1a" stroke-width=".6"/>'; return s; }
    function ledger() {
      if (!t.ledger) return '';
      var x = mx(170), y = 486, pg = 'M' + (x - 100) + ',' + (y - 20) + ' L' + x + ',' + (y - 24) + ' L' + x + ',' + (y + 6) + ' L' + (x - 110) + ',' + (y + 2) + ' Z M' + x + ',' + (y - 24) + ' L' + (x + 100) + ',' + (y - 20) + ' L' + (x + 110) + ',' + (y + 2) + ' L' + x + ',' + (y + 6) + ' Z';
      return '<g class="src-nohit"><ellipse cx="' + x + '" cy="' + (y + 6) + '" rx="110" ry="10" fill="url(#src-ao)" opacity=".8"/><path d="' + pg + '" fill="#ebdfc2"/><path d="' + pg + '" fill="#000" opacity=".08" filter="url(#src-paper)"/>' +
        [0, 1, 2, 3].map(function (i) { return '<path d="M' + (x - 92) + ',' + (y - 14 + i * 5) + ' l80,-2 M' + (x + 10) + ',' + (y - 16 + i * 5) + ' l84,2" stroke="#4a3826" stroke-opacity=".5" stroke-width=".8"/>'; }).join('') +
        '<path d="M' + (x + 60) + ',' + (y - 30) + ' L' + (x + 150) + ',' + (y - 80) + '" stroke="#e8e0d0" stroke-width="2.4"/><path d="M' + (x + 100) + ',' + (y - 58) + ' q30,-30 52,-26 q-14,20 -50,30" fill="#e8e0d0" opacity=".85"/></g>';
    }
    // The keeper's case of finer goods, shut on the counter. Press it and the keeper opens it for you.
    function caseShut() {
      if (!R.place.case.length) return '';
      var x = SP.cs[0], y = SP.cs[1], cb = mix(t.th.wood, '#000', .2);
      return '<g data-k="case"><ellipse cx="' + x + '" cy="' + (y + 3) + '" rx="50" ry="7" fill="url(#src-ao)"/>' +
        '<path d="M' + (x - 44) + ',' + y + ' V' + (y - 30) + ' H' + (x + 44) + ' V' + y + ' Z" fill="' + cb + '"/><path d="M' + (x - 46) + ',' + (y - 30) + ' Q' + x + ',' + (y - 46) + ' ' + (x + 46) + ',' + (y - 30) + ' Z" fill="' + mix(t.th.wood, '#ffffff', .08) + '"/>' +
        '<path d="M' + (x - 46) + ',' + (y - 30) + ' H' + (x + 46) + '" stroke="#2a140a" stroke-width="2"/><path d="M' + (x - 44) + ',' + (y - 29) + ' Q' + x + ',' + (y - 44) + ' ' + (x + 44) + ',' + (y - 29) + '" stroke="' + t.lc + '" stroke-opacity=".35" stroke-width="1.4" fill="none"/>' +
        [-44, 38].map(function (d) { return '<rect x="' + (x + d) + '" y="' + (y - 31) + '" width="6" height="31" fill="url(#cm-trimv)"/>'; }).join('') +
        '<rect x="' + (x - 7) + '" y="' + (y - 26) + '" width="14" height="12" rx="2" fill="url(#cm-trim)"/><circle cx="' + x + '" cy="' + (y - 20) + '" r="2" fill="#3a2a10"/></g>';
    }
    // More goods than the room has places for: a card on the counter says so and opens the list.
    function moreCard() {
      var n = R.place.more.length; if (!n) return '';
      return '<g data-k="more"><g transform="translate(' + SP.more[0] + ',' + SP.more[1] + ') rotate(-4)"><rect x="-58" y="-26" width="116" height="40" rx="3" fill="url(#src-tag)"/><rect x="-58" y="-26" width="116" height="40" rx="3" fill="#000" opacity=".06" filter="url(#src-paper)"/>' +
        '<text x="0" y="-8" text-anchor="middle" class="src-hand">+' + n + ' more</text><text x="0" y="7" text-anchor="middle" class="src-hand small">see the wares list</text></g></g>';
    }
    function pileSvg() {
      if (parcel) {
        var x = PILE[2][0] + (t.mir ? 0 : -2), y = 506, d = 'M' + (x - 62) + ',' + y + ' L' + (x - 56) + ',' + (y - 46) + ' Q' + x + ',' + (y - 58) + ' ' + (x + 56) + ',' + (y - 46) + ' L' + (x + 62) + ',' + y + ' Z';
        return '<g class="srcb-parcel src-nohit"><ellipse cx="' + x + '" cy="' + (y + 2) + '" rx="70" ry="9" fill="#000" opacity=".45" filter="url(#src-blur3)"/><path d="' + d + '" fill="#c8b088"/><path d="' + d + '" fill="#000" opacity=".08" filter="url(#src-paper)"/>' +
          '<path d="M' + (x - 4) + ',' + (y - 54) + ' V' + y + ' M' + (x - 59) + ',' + (y - 22) + ' Q' + x + ',' + (y - 30) + ' ' + (x + 59) + ',' + (y - 22) + '" stroke="#e8dcbc" stroke-width="2.2" fill="none"/></g>';
      }
      var u = [], s = '';
      R.model.its.forEach(function (it) { for (var i = 0, n = R.inBasket(it.id); i < n; i++) u.push(it); });
      var shown = u.slice(0, PILE.length), more = u.length - shown.length, order = shown.map(function (x, i) { return i; });
      order.sort(function (a, b) { return PILE[a][1] - PILE[b][1]; }).forEach(function (i) {
        var p = PILE[i], it = shown[i], tilt = ((i * 37) % 9 - 4) * .8;
        s += '<g data-k="pile:' + it.id + '" class="srcb-pileware"><ellipse class="src-nohit" cx="' + p[0] + '" cy="' + (p[1] + 1) + '" rx="' + f1(20 * p[2]) + '" ry="' + f1(4 * p[2]) + '" fill="#000" opacity=".45" filter="url(#src-blur3)"/>' +
          '<g transform="translate(' + p[0] + ',' + p[1] + ') rotate(' + tilt.toFixed(1) + ') scale(' + p[2] + ')">' + ware(it, t.side, R.icons) + '</g></g>';
      });
      if (more > 0) { var q = PILE[PILE.length - 1]; s += '<g class="src-nohit"><circle cx="' + (q[0] + 18) + '" cy="' + (q[1] - 62) + '" r="13" fill="#2a1a0e" stroke="' + t.th.trim + '" stroke-width="1.4"/><text x="' + (q[0] + 18) + '" y="' + (q[1] - 57) + '" text-anchor="middle" class="src-more">+' + more + '</text></g>'; }
      return s;
    }
    var BACKP = ['ladder', 'banner', 'sconce', 'herbs', 'lanterns', 'floating-books'], FRONTP = ['anvil', 'alembic', 'jars', 'crystal', 'orrery'], OVERP = ['sparks', 'rain', 'motes'];
    function propsOf(list, mirror) {
      var s = '';
      t.props.forEach(function (p) { if (list.indexOf(p) >= 0 && P[p]) s += PROPS[p](t, P[p]); });
      return mirror ? cmM(t, s) : s;
    }
    // Dashed slots while editing: where each kind of prop may be dropped, and the opening when there is none to press.
    function editSlots() {
      if (!edit) return '';
      var s = '<g id="cm-edit">';
      Object.keys(CM_SLOTS).forEach(function (k) {
        var q = CM_SLOTS[k], x = q[0];
        if (k === 'lane' && !(t.air || t.sk.opening === 'doorway')) return;
        if (k === 'ladder' && t.domes) return;
        if (k === 'cL' && t.domes) return;
        if (k === 'tallR' && t.domes) return;
        if (k !== 'over' && k !== 'air' && k !== 'lane' && t.mir) x = 1200 - q[0] - q[2];
        s += '<g><rect class="cm-slot" data-slot="' + k + '" tabindex="0" role="button" aria-label="' + q[4] + ': place the chosen prop here" x="' + x + '" y="' + q[1] + '" width="' + q[2] + '" height="' + q[3] + '" rx="9"/><text class="cm-slotl" x="' + (x + 8) + '" y="' + (q[1] + 16) + '">' + q[4] + '</text></g>';
      });
      if (t.sk.opening === 'none' && !t.air) { var wx = t.mir ? 1200 - 1018 : 806; s += '<rect class="cm-slot" data-k="window" x="' + wx + '" y="86" width="212" height="196" rx="9"/><text class="cm-slotl" x="' + (wx + 8) + '" y="104">Press to add a window</text>'; }
      return s + '</g>';
    }
    function svg() {
      var wall = SHAPES[t.sk.shape](t), open = t.air ? '' : cmM(t, OPENINGS[t.sk.opening](t)), lane = P['steam-cart'] ? PROPS['steam-cart'](t, P['steam-cart']) : '';
      return '<rect width="1200" height="700" fill="#0a0705"/><g data-depth=".55"><rect x="-300" y="440" width="1800" height="460" fill="url(#src-wallfoot)"/><g data-bake="wall">' + wall + '</g>' +
        (t.air && cmWet(t) ? cmRainIn(t, -300, -100, 1800, 560, 7) : '') + open + lane +
        (LG.bake ? '<g data-bake="forge">' + LG.bake + '</g>' : '') + (LG.back || '') + (LG.hung ? cmLanterns(t, true) : '') +
        '<circle cx="' + LP.x + '" cy="' + LP.y + '" r="' + Math.round(t.L.reach * .55) + '" fill="url(#cm-glow)" data-flick=".32"/>' + (LG.moon ? '<circle cx="' + LP.x + '" cy="' + (LP.y + 40) + '" r="320" fill="url(#cm-glow)" opacity=".5" data-flick=".3"/>' : '') +
        (D.art ? '<g data-bake="unit">' + cmM(t, D.art) + '</g>' : '') + (D.zone ? '<rect data-k="shelves" x="' + (t.mir ? 1200 - D.zone[0] - D.zone[2] : D.zone[0]) + '" y="' + D.zone[1] + '" width="' + D.zone[2] + '" height="' + D.zone[3] + '" fill="transparent"/>' : '') +
        (D.counterTop ? '' : decor() + '<g id="srcb-wares">' + wares() + '</g>') + propsOf(BACKP, true) + (t.pend ? '' : '') + '</g>' +
        '<g data-depth=".75">' + keeper() + '</g>' +
        '<g data-depth="1"><g data-bake="counter">' + COUNTERS[t.sk.counter](t) + '</g>' + ledger() + propsOf(FRONTP, true) + (LG.front || '') + cast(LP.x, 478, 40, 120, 0) + caseShut() +
          (D.counterTop ? decor() + '<g id="srcb-wares">' + wares() + '</g>' : '') + cloth() + '<g id="srcb-pile">' + pileSvg() + '</g>' + bell() + scales() + moreCard() + '<g id="srcb-held"></g></g>' +
        propsOf(OVERP.filter(function (p) { return p !== 'sparks'; }), false) + (P.sparks ? PROPS.sparks(t, P.sparks) : '') +
        '<rect data-depth=".8" x="-300" y="-200" width="1800" height="1100" fill="url(#cm-shade)" data-flick=".98" pointer-events="none"/>' +
        '<rect width="1200" height="700" fill="url(#cm-vig)" pointer-events="none"/>' + editSlots();
    }

    var WHERE = { shelves: 'on the shelves', pegboard: 'on the pegboard', drawers: 'on the drawer wall', pigeonholes: 'in its pigeonhole', domes: 'under its dome' };
    function info(k) {
      var p = k.split(':'), it = p[1] ? R.item(p[1]) : null;
      if ((p[0] === 'ware' || p[0] === 'held') && it) return R.info(it, p[0] === 'held' ? 'Drag it onto your pile · Esc puts it back' : it.out ? '' : 'Press and the keeper brings it to the counter');
      if (p[0] === 'pile' && it) return { title: it.n, meta: it.price + ' each', now: R.inBasket(it.id) + ' on your pile', foot: 'Right-click to put one back' };
      if (k === 'case') return { title: 'The keeper’s case', meta: 'Finer goods', body: R.place.case.length + (R.place.case.length === 1 ? ' thing' : ' things') + ' kept shut and out of the dust.', foot: 'Press and the keeper opens it for you' };
      if (k === 'shelves') return { title: D.label, body: 'Everyday goods, a few of each.', foot: 'Press to walk closer' };
      if (k === 'slate') return { title: 'Only today', body: 'The tagged goods are out today only. The shelf changes at dawn.', foot: 'Press to ask the keeper' };
      if (k === 'window') return edit ? { title: 'The opening', foot: 'Press to move it: left, centre, right or none' } : { title: CM_OPEN_INFO[t.sk.opening] || 'The window', body: 'The ' + (t.air ? 'lane' : 'view') + ' outside.' };
      if (k === 'keeper') return { title: R.model.name, meta: 'The keeper', foot: R.model.lines ? 'Press to talk' : '' };
      if (k === 'bell') return { title: 'The counter bell', foot: 'Press to ring it' };
      if (k === 'lamp') return { title: { lamp: 'Oil lamp', candles: 'Candles' }[t.th.light] || 'The light', body: 'The room’s one real light.', foot: 'Press to turn up the light' };
      if (k === 'orrery') return { title: 'The orrery', body: 'Little worlds, turning.' };
      if (k === 'scales') return { title: 'The scales', body: sold ? 'Your coins sit in the pan.' : 'Coins go in the pan when you pay.' };
      if (k === 'cloth') return { title: 'The counter cloth', body: 'The keeper lays a good here for you to look at.' };
      if (k === 'more') return { title: R.place.more.length + ' more for sale', body: 'More goods than the shop has places for. Every one is in the wares list.', foot: 'Press to open the list' };
      return null;
    }
    function hint() {
      if (held && held.phase === 'rest') {
        var it = R.item(held.id), can = R.canAdd(held.id);
        R.say('<span>The keeper sets the ' + esc(it ? it.n : 'good') + ' on the cloth.' + (can ? ' Drag it onto your pile, or:' : ' ' + esc(R.why(held.id))) + '</span><span class="src-acts">' +
          (can ? '<button type="button" class="src-btn primary" data-act="take">Add to my pile</button>' : '') + R.rollActs(held.id) + '<button type="button" class="src-btn" data-act="back">Put it back</button></span>');
      } else R.say(R.model.its.length ? '<span>Press a good ' + (WHERE[t.sk.display] || 'on the shelves') + ' and the keeper brings it to the counter.' + (R.place.case.length ? ' Finer goods are in the case.' : '') + '</span>' : '<span>Nothing for sale right now.</span>');
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
    function indexOfWare(id) { var i = -1; shelfIt().forEach(function (it, j) { if (String(it.id) === String(id)) i = j; }); return i; }
    /* The keeper fetches: the token turns to the display and the good travels in an arc to the cloth, growing as it comes nearer. */
    function fetch(id) {
      var i = indexOfWare(id); if (i < 0) return;
      var p = spot(i);
      held = { id: id, phase: 'fly', t: 0, dur: R.calm ? .01 : .85, from: [p[0], p[1], p[2]], to: [CLOTH.x, CLOTH.y + 8, 2.3], x: p[0], y: p[1], s: p[2] };
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
      var i = Math.max(0, indexOfWare(held.id)), p = spot(i); fly([p[0], p[1], p[2]], 'return', R.calm ? .01 : .6); R.hideCard();
    }
    function heldSvg() {
      if (!held) return '';
      var h = held, it = R.item(h.id), rest = h.phase === 'rest', sh = '';
      if (!it) return '';
      if (rest || h.phase === 'drag') sh = cast(h.x, h.y, 46, 130, .8) + '<ellipse class="src-nohit" cx="' + f1(h.x) + '" cy="' + f1(h.y + 1) + '" rx="' + f1(h.s * 20) + '" ry="' + f1(h.s * 3.4) + '" fill="url(#src-ao)"/>';
      return sh + '<g data-k="held:' + h.id + '" class="src-heldware' + (h.phase === 'drag' ? ' drag' : '') + '" transform="translate(' + f1(h.x) + ',' + f1(h.y) + ') scale(' + h.s.toFixed(3) + ')"><rect x="-30" y="-70" width="60" height="72" fill="transparent"/>' + ware(it, t.side === 'R' ? 'L' : 'R', R.icons) + '</g>' +
        (rest ? '<circle class="src-nohit" cx="' + f1(h.x) + '" cy="' + f1(h.y - 60) + '" r="70" fill="url(#cm-glow)" opacity=".18"/>' : '');
    }
    function frame(tm, dt) {
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
    function overPile(x, y) { return x > PX0 && x < PX1 && y > 420 && y < 540; }
    /* Dragging the good off the cloth: over the basket or the pile it lights up, and letting go there puts it on the pile; anywhere else it settles back on the cloth. */
    function afterDraw() {
      var sv = R.svg;
      sv.addEventListener('pointerdown', function (e) {
        var tg = e.target.closest && e.target.closest('[data-k^="held:"]'); if (!tg || !held || held.phase !== 'rest') return;
        e.preventDefault(); R.dragging = true; R.hideCard(); held.phase = 'drag';
        var q = R.toScene(e); held.dx = held.x - q[0]; held.dy = held.y - q[1];
        try { sv.setPointerCapture(e.pointerId); } catch (x) { /* a synthetic pointer has nothing to capture */ }
      });
      sv.addEventListener('pointermove', function (e) {
        if (!held || held.phase !== 'drag') return;
        var q = R.toScene(e); held.x = q[0] + held.dx; held.y = q[1] + held.dy; held.s = 2.3 - Math.max(0, (CLOTH.y - held.y) / 600);
        R.basketHot(R.overBasket(e.clientX, e.clientY));
        var pg = sv.querySelector('#srcb-pile'); if (pg) pg.classList.toggle('hot', overPile(q[0], q[1]));
      });
      sv.addEventListener('pointerup', function (e) {
        if (!held || held.phase !== 'drag') return;
        setTimeout(function () { R.dragging = false; }, 0); R.basketHot(false);
        var pg = sv.querySelector('#srcb-pile'); if (pg) pg.classList.remove('hot');
        var q = R.toScene(e);
        if (R.overBasket(e.clientX, e.clientY) || overPile(q[0], q[1])) { held.phase = 'rest'; toBasket(); }
        else fly([CLOTH.x, CLOTH.y + 8, 2.3], 'fly', .35);
      });
    }
    function act(a) { if (a === 'take') toBasket(); else if (a === 'back') putBack(); }
    function click(k) {
      var p = k.split(':'), it = p[1] ? R.item(p[1]) : null;
      if (k === 'window' && edit && R.opening()) return true;
      // Walking up to the display; pressing a good from there steps back so the keeper can bring it.
      if (k === 'shelves') { if (R.cam().z < 1.01 && !held) R.closer(mx(260), 250, 1.65); return true; }
      if (p[0] === 'ware' && it) { if (it.out || R.stock(it) === 0) return false; if (R.cam().z > 1.01) R.stepBack(function () { fetch(it.id); }); else fetch(it.id); return true; }
      if (k === 'case' && R.place.case.length) { R.go('case', 'in'); return true; }
      if (p[0] === 'held') return true;
      if (k === 'bell') { ring(); return true; }
      if (k === 'keeper') { nod(); R.talk(R.svg.querySelector('[data-k="keeper"]')); return true; }
      if (k === 'slate' && R.model.today) { nod(); R.talk(R.svg.querySelector('[data-k="keeper"]'), R.model.today.said); return true; }
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
    return { id: 'counter', label: R.model.name + ', seen from the counter, ' + t.n.name, defs: defs, svg: svg, info: info, menu: menu, click: click, frame: frame, afterDraw: afterDraw, act: act, escape: escape, sync: sync, bought: bought, hint: hint,
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
          (can ? '<button type="button" class="src-btn primary" data-act="take">Add to my pile</button>' : '') + R.rollActs(held.id) + '<button type="button" class="src-btn" data-act="back">Put it back</button></span>');
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
          R.rollActs(up.id) + '<button type="button" class="src-btn" data-act="back">Back in the case</button></span>');
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

    function themeKey() { return JSON.stringify([R.model.counter || R.model.roomType, !!R.model.edit]); }
    function setModel(m) {
      R.model = { its: m.its || [], basket: m.basket || {}, icons: m.icons || window.ShopRoomIcons || {}, image: m.image || '', name: m.name || 'Shop', roomType: m.roomType || 'general', time: m.time || '', lines: !!m.lines,
        today: m.today && m.today.line ? { line: String(m.today.line), said: String(m.today.said || '') } : null,
        counter: m.counter && typeof m.counter === 'object' ? m.counter : null, edit: !!m.edit };
      R.place = layout(R.model.its); R.icons = R.model.icons; R.seed = seedOf(R.model.name);
    }
    setModel(model);
    R.stage = stage; R.calm = still();
    // The builder hears which props found no free spot, and what a press on the opening in place mode should do.
    R.placed = function (m) { if (hooks.placed) hooks.placed(m); };
    R.opening = function () { if (!hooks.opening) return false; hooks.opening(); return true; };
    R.item = function (id) { var f = null; R.model.its.forEach(function (it) { if (String(it.id) === String(id)) f = it; }); return f; };
    R.inBasket = function (id) { return +R.model.basket[String(id)] || 0; };
    R.stock = function (it) { return it.qty === null || it.qty === undefined ? null : Math.max(0, it.qty - R.inBasket(it.id)); };
    R.canAdd = function (id) { return !!hooks.canAdd(id); };
    // Haggle and Look closer for the good in hand, as the shop room offers them (escaped there).
    R.rollActs = function (id) { return (hooks.rollActs && hooks.rollActs(id)) || ''; };
    R.why = function (id) { return (hooks.why && hooks.why(id)) || 'That can’t go on your pile.'; };
    R.add = function (id) { if (!R.canAdd(id)) { R.pop(R.why(id), 600, 420); return false; } return hooks.add(id) !== false; };
    R.remove = function (id) { hooks.remove(id); };
    R.talk = function (el, said) { if (hooks.talk) hooks.talk(el, said); };
    R.openList = function () { if (hooks.openList) hooks.openList(); };
    R.info = function (it, foot) {
      var left = R.stock(it), inb = R.inBasket(it.id);
      return { title: it.n, meta: (it.k ? it.k + ' · ' : '') + it.price + (it.today ? ' · Only today' : ''), now: it.out ? 'Sold out' : (left === null ? 'Plenty in stock' : left + ' left') + (inb ? ', ' + inb + ' on your pile' : ''), foot: foot };
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
      if (b && stage.contains(b)) {
        var act = b.getAttribute('data-act'), rm = /^roll:(haggle|look):(.+)$/.exec(act);
        if (rm) { if (hooks.roll) hooks.roll(rm[1], rm[2], b); return; }
        if (view && view.act) view.act(act); return;
      }
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

    var VIEWS = { counter: composedView, floor: floorView, 'case': caseView };
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
      update: function (m) {
        var was = themeKey(); setModel(m);
        // A changed theme (or place mode) redraws the counter room in place; the other views never read it.
        if (view && where === 'counter' && !busy && was !== themeKey()) mountView('counter'); else if (view) view.sync();
      },
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
    render: render, layout: layout, starters: CM_STARTERS, starterFor: starterFor, normalise: cmNorm,
    options: { skeleton: { shape: CM_OPT.shape, opening: CM_OPT.opening, display: CM_OPT.display, counter: CM_OPT.counter }, theme: { wall: CM_OPT.wall, light: CM_OPT.light, outside: CM_OPT.outside }, props: CM_PROPS, propAt: PROP_AT, slots: CM_SLOTS }, family: family, floorPiece: floorPiece, shade: shade, initials: initials, safeUrl: safeUrl, ware: ware, CAP: CAP,
    sync: function (host, model) { if (host && host._counter) host._counter.update(model); },
    escape: function (host) { return !!call(host, 'escape'); },
    pause: function (host, on) { call(host, 'pause', on); },
    bought: function (host, n) { call(host, 'bought', n); },
    destroy: function (host) { if (host && host._counter) { host._counter.destroy(); delete host._counter; } }
  };
})();
