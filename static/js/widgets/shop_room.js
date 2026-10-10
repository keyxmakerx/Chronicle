/**
 * shop_room.js -- Chronicle Shop Room Widget
 *
 * Draws a shop as an isometric room: furniture, the shop's goods on shelves
 * and tables, the keeper's portrait, light and shadow. The wares list sits
 * under the room. Owners get Arrange: drag furniture and the portrait, change
 * an item's icon and colour, and run the room generator from a panel that
 * slides out inside the room (keep pin and reroll dice per part, like the
 * other generators). Arrange changes wait for "Save room"; leaving with
 * unsaved changes shakes the panel and says so instead of losing them. The
 * layout is saved per shop; the goods themselves are the shop's "sells"
 * relations, so stock and prices always come from the inventory, never from
 * the room.
 *
 * The scene is one SVG string built by createRoom() below, a pure function of
 * the room state with no DOM access, so it is unit-tested in Node
 * (test/js/shop_room.test.mjs). Shadows and light are drawn once per change and
 * never animated.
 *
 * Mount via:
 *   <div data-widget="shop_room"
 *        data-room-endpoint="/campaigns/:id/armory/shops/:eid/room"
 *        data-relations-endpoint="/campaigns/:id/entities/:eid/relations"
 *        data-shop-name="..." data-shop-image="..."
 *        data-campaign-url="/campaigns/:id"
 *        data-can-arrange="true"
 *        data-csrf-token="..."></div>
 *
 * Needs shop_room_icons.js loaded first (window.ShopRoomIcons).
 */
(function () {
  // What a press outside an open shop does: it closes, unless the basket
  // still holds something, when the shop stays open and says so instead.
  function outsideClose(basket) {
    for (var k in basket) { if (Object.prototype.hasOwnProperty.call(basket, k) && basket[k] > 0) return 'warn'; }
    return 'close';
  }

  window.ShopRoomDoor = { outsideClose: outsideClose };

  // Where a piece's floor shadow points, in floor units: away from the nearest
  // warm light (forge, lamp) and longer the taller the piece, or back and to
  // the right as before when the room has no warm light.
  function castOffset(warm, cx, cy, h) {
    var best = null, bd = Infinity;
    (warm || []).forEach(function (w) { var d = (cx - w[0]) * (cx - w[0]) + (cy - w[1]) * (cy - w[1]); if (d < bd) { bd = d; best = w; } });
    if (!best || bd < 1e-4) return [h * .5, h * .16];
    var len = Math.sqrt(bd), k = Math.min(h * .55, .4 + h * .35);
    return [(cx - best[0]) / len * k, (cy - best[1]) / len * k];
  }
  window.ShopRoomLight = { castOffset: castOffset };

  // Moving one of the room's own decorations keeps all of them: each becomes
  // a hand-placed one where it stands and the generator adds no more, so
  // nothing else in the room shifts when one is picked up.
  function freezeDecor(S, placed) {
    (placed || []).forEach(function (d) { if (d.icon && !d.mine) S.decor.push({ piece: d.piece, spot: d.spot, icon: d.icon }); });
    S.deco = 'none';
  }
  window.ShopRoomDecor = { freeze: freezeDecor };

  function esc(s) {
    return String(s == null ? '' : s).replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/'/g, '&#39;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }

  // createRoom builds the drawing engine for one room. S is the room state the
  // widget owns; the engine reads it and draws, and generate() rewrites
  // S.pieces. Nothing here touches the page.
  // Each mood is one named recipe: colours, wall and floor finishes, how dark
  // the room sits, the colour of its lamp glow, haze and drifting motes, the
  // decorations it scatters, the furniture it brings and its starting effects.
  // Cozy is the room as it always was, so the Colours choice still applies.
  var MOODS = {
    cozy: { name: 'Cozy', sw: ['#e6dfd3', '#a07a55', '#f0a12e'], need: [], efx: {} },
    eldritch: { name: 'Eldritch horror', short: 'Eldritch', sw: ['#3a3646', '#5c4a7e', '#3fbf7f'], pal: { wall: '#4a4558', wood: '#3a2f44', trim: '#5a4f6c', floor: '#4a4452', acc: '#3fbf7f' },
      wall: 'stone', floor: 'flags', dim: .5, dark: 1.3, glow: ['#5affa0', '#2fbf73'], haze: '#50dc8c', mote: '#8dffb8',
      need: ['circle', 'monolith', 'tentacle', 'tentacle'], deco: ['eye', 'tentacle', 'skull', 'book-open', 'spider', 'moon'], efx: { shadows: 130, warmth: 110, haze: 200, vignette: 140, time: 'night' } },
    candy: { name: 'Candy land', sw: ['#ffd6e8', '#f39bc0', '#ff5fa2'], pal: { wall: '#ffe0ee', wood: '#f39bc0', trim: '#ffffff', floor: '#fff0bf', acc: '#ff5fa2' },
      wall: 'panel', floor: 'checker', dim: .95, dark: .55, glow: ['#ffb3dc', '#ff7fbf'], haze: '#ffbee6', mote: '#ffffff',
      need: ['lolly', 'cane', 'cane', 'gumdrop', 'gumdrop'], deco: ['candy', 'lollipop', 'cupcake', 'star', 'apple-whole', 'gem'], efx: { shadows: 70, warmth: 80, haze: 120, vignette: 50 } },
    haunted: { name: 'Haunted', sw: ['#5a5f6b', '#3d3530', '#9db4ff'], pal: { wall: '#6a6f7b', wood: '#4a403a', trim: '#7a7f8a', floor: '#5a5652', acc: '#9db4ff' },
      wall: 'plank', floor: 'plank', dim: .55, dark: 1.25, glow: ['#b4c8ff', '#7896ff'], haze: '#c8d7f0', mote: '#dfe7ff',
      need: ['coffin', 'ghost', 'ghost'], deco: ['skull', 'web', 'ghost', 'hourglass', 'spider', 'moon'], efx: { shadows: 120, warmth: 90, haze: 250, vignette: 150, time: 'night', weather: 'rain' } },
    festive: { name: 'Festive', sw: ['#efe1c6', '#3f6b4a', '#c8102e'], pal: { wall: '#efe1c6', wood: '#7a3b2e', trim: '#3f6b4a', floor: '#8a5a3c', acc: '#c8102e' },
      wall: 'panel', floor: 'herring', dim: .8, dark: .95, glow: ['#ffc070', '#ff9a3c'], haze: '#ffd6a0', mote: '#fff4d6',
      need: ['pine', 'gift', 'gift'], deco: ['gift', 'snowflake', 'bell', 'star', 'apple-whole'], efx: { warmth: 140, haze: 110, time: 'dusk', weather: 'snow' } },
    underwater: { name: 'Underwater', sw: ['#2f6f80', '#cdb98a', '#5ff0e0'], pal: { wall: '#2f6f80', wood: '#3d5f63', trim: '#6fb3b0', floor: '#cdb98a', acc: '#5ff0e0' },
      wall: 'stone', floor: 'dirt', dim: .62, dark: 1.05, glow: ['#8ff7ff', '#3cc8dc'], haze: '#5ac8e6', mote: '#e6fbff',
      need: ['coral', 'coral', 'kelp', 'kelp'], deco: ['fish', 'shell', 'anchor', 'star', 'gem'], efx: { shadows: 90, warmth: 80, haze: 220, vignette: 130 } },
    fey: { name: 'Fey forest', sw: ['#dce6c8', '#5f7a45', '#d36fd0'], pal: { wall: '#dce6c8', wood: '#6b5a3a', trim: '#a9c48a', floor: '#5f7a45', acc: '#d36fd0' },
      wall: 'plaster', floor: 'dirt', dim: .72, dark: 1, glow: ['#f4ffa8', '#b8f07a'], haze: '#d8f0b0', mote: '#f0ff8a',
      need: ['mushroom', 'mushroom', 'bloom', 'bloom'], deco: ['leaf', 'seedling', 'butterfly', 'feather', 'moon', 'spa'], efx: { haze: 150, time: 'dusk' } },
    desert: { name: 'Desert bazaar', sw: ['#ecd3a6', '#9a6436', '#e0662a'], pal: { wall: '#ecd3a6', wood: '#9a6436', trim: '#d29c55', floor: '#c99b62', acc: '#e0662a' },
      wall: 'plaster', floor: 'flags', dim: .9, dark: .8, glow: ['#ffd27a', '#ffa640'], haze: '#ffd9a0', mote: '#fff1cf',
      need: ['rug', 'urn', 'urn', 'palm'], deco: ['jar', 'sun', 'scroll', 'gem', 'key', 'hourglass'], efx: { window: 150, warmth: 120, shadows: 120, haze: 80 } },
    dwarven: { name: 'Dwarven hall', sw: ['#7f7b76', '#5a3d28', '#e09b2d'], pal: { wall: '#7f7b76', wood: '#5a3d28', trim: '#b08a3e', floor: '#5c5853', acc: '#e09b2d' },
      wall: 'stone', floor: 'flags', dim: .55, dark: 1.2, glow: ['#ffb35c', '#ff8a2a'], haze: '#ffb070', mote: '#ffd49a',
      need: ['pillar', 'pillar', 'brazier'], deco: ['hammer', 'axe', 'shield-halved', 'gem', 'crown', 'beer-mug'], efx: { warmth: 130, shadows: 125, vignette: 130, smoke: true } }
  };
  var MOOD_ORDER = ['cozy', 'eldritch', 'candy', 'haunted', 'festive', 'underwater', 'fey', 'desert', 'dwarven'];
  // Which mood each piece of mood furniture belongs to, for the Add list.
  var MOOD_KIND = { tentacle: 'eldritch', monolith: 'eldritch', circle: 'eldritch', cane: 'candy', gumdrop: 'candy', lolly: 'candy', ghost: 'haunted', coffin: 'haunted',
    gift: 'festive', pine: 'festive', coral: 'underwater', kelp: 'underwater', mushroom: 'fey', bloom: 'fey', urn: 'desert', palm: 'desert', pillar: 'dwarven', brazier: 'dwarven' };
  // Effect levels are percentages of the plain look; a saved layout carries all of them.
  var EFX0 = { shadows: 100, warmth: 100, window: 100, haze: 100, vignette: 100, dust: true, flicker: true, embers: true, smoke: false, weather: '', time: '' };
  function moodEfx(m) { var o = {}, k, e = (MOODS[m] || MOODS.cozy).efx; for (k in EFX0) o[k] = EFX0[k]; for (k in e) o[k] = e[k]; return o; }

  function createRoom(S) {
    var ICONS = window.ShopRoomIcons || {};
    var MAT = { steel: '#a8b0ba', iron: '#6b7280', gold: '#d4a72c', silver: '#d6dde6', red: '#dc2626', green: '#65a30d', teal: '#14b8a6', blue: '#3b82f6', violet: '#a78bfa', leather: '#a16207', paper: '#e8dcbc', cloth: '#c2410c', wood: '#9a6b3d', dark: '#4b5563', bone: '#e7e2d4', honey: '#f59e0b' };
    var PALS = [['oak', 'Oak', '#a07a55'], ['ember', 'Ember', '#c2410c'], ['moss', 'Moss', '#3f7d3a'], ['gilt', 'Gilt', '#a88324'], ['slate', 'Slate', '#0d9488'], ['dusk', 'Dusk', '#8b5cf6']];
    // Palettes as real colours, since the room is drawn in SVG. Any palette goes with any shop type.
    var PAL = {
      oak:   { wall: '#e6dfd3', wood: '#a07a55', trim: '#c9bfb0', floor: '#a7835f', acc: '#6366f1' },
      ember: { wall: '#e8d5c0', wood: '#7e4f33', trim: '#b0896a', floor: '#76503a', acc: '#c2410c' },
      moss:  { wall: '#dbe2cf', wood: '#786848', trim: '#9cb08c', floor: '#6f6447', acc: '#3f7d3a' },
      gilt:  { wall: '#f0e7d2', wood: '#bfa070', trim: '#d6c08a', floor: '#b39a70', acc: '#a88324' },
      slate: { wall: '#c3cad1', wood: '#5a5048', trim: '#6d7a83', floor: '#535a60', acc: '#0d9488' },
      dusk:  { wall: '#d6cde3', wood: '#4d3d56', trim: '#9d86c9', floor: '#463b50', acc: '#8b5cf6' }
    };
    function hex(h) { h = h.replace('#', ''); return [parseInt(h.slice(0, 2), 16), parseInt(h.slice(2, 4), 16), parseInt(h.slice(4, 6), 16)]; }
    function mix(a, b, t) { var A = hex(a), B = hex(b); return '#' + A.map(function (v, i) { var c = Math.round(v + (B[i] - v) * t); return (c < 16 ? '0' : '') + c.toString(16); }).join(''); }
    function dk(c, t) { return mix(c, '#000000', t); }
    function lt(c, t) { return mix(c, '#ffffff', t); }
    var C = {};
    function colours() {
      var M = MOODS[S.mood], p = M && M.pal ? M.pal : PAL[S.pal] || PAL.oak, f = S.dark ? .32 : 0;
      C = { wall: dk(p.wall, f), wood: dk(p.wood, f * .6), trim: dk(p.trim, f * .5), floor: dk(p.floor, f * .6), acc: p.acc, dark: !!S.dark };
    }

    // Isometric projection: tile is 64x32 on screen, one height unit is 32px.
    var TW = 64, TH = 32, ZH = 32, WH = 3.3;
    function P(x, y, z) { return [(x - y) * TW / 2, (x + y) * TH / 2 - (z || 0) * ZH]; }
    function pts(a) { return a.map(function (p) { return p[0].toFixed(1) + ',' + p[1].toFixed(1); }).join(' '); }
    function poly(a, fill, extra) { return '<polygon points="' + pts(a) + '" fill="' + fill + '"' + (extra || '') + '/>'; }
    function line(a, b, stroke, w) { return '<line x1="' + a[0].toFixed(1) + '" y1="' + a[1].toFixed(1) + '" x2="' + b[0].toFixed(1) + '" y2="' + b[1].toFixed(1) + '" stroke="' + stroke + '" stroke-width="' + (w || 1) + '" stroke-linecap="round"/>'; }
    // A box with its three visible faces: top lightest, the +y face mid, the +x face darkest (light comes from the left).
    function box(x, y, z, w, d, h, c, o) {
      o = o || {};
      // With full shading each face also gets a soft light-to-dark gradient, which is what makes flat colour read as a lit surface.
      var s = '', sh = S.fx === 'full' && !(o.l && String(o.l).indexOf('rgba') === 0), fl, fr, ft;
      fl = [P(x, y + d, z), P(x + w, y + d, z), P(x + w, y + d, z + h), P(x, y + d, z + h)];
      fr = [P(x + w, y, z), P(x + w, y + d, z), P(x + w, y + d, z + h), P(x + w, y, z + h)];
      ft = [P(x, y, z + h), P(x + w, y, z + h), P(x + w, y + d, z + h), P(x, y + d, z + h)];
      if (!o.noL) s += poly(fl, o.l || dk(c, .18), o.lx) + (sh && h > .04 ? poly(fl, 'url(#fL)') + poly(fl, 'url(#tL)') : '');
      if (!o.noR) s += poly(fr, o.r || dk(c, .36), o.rx) + (sh && h > .04 ? poly(fr, 'url(#fR)') + poly(fr, 'url(#tR)') : '');
      if (!o.noT) s += poly(ft, o.t || lt(c, .08), o.tx) + (sh && w * d > .1 ? poly(ft, 'url(#fT)') : '');
      if (o.edge !== false) s += '<polyline points="' + pts([P(x, y + d, z + h), P(x + w, y + d, z + h), P(x + w, y, z + h)]) + '" fill="none" stroke="' + lt(c, .3) + '" stroke-width=".8" opacity=".6"/>';
      return s;
    }
    // Points on a face, in that face's own units, for drawing details like planks and drawers.
    function onL(x, y, z, d) { return function (u, v) { return P(x + u, y + d, z + v); }; }
    function onR(x, y, z, w) { return function (u, v) { return P(x + w, y + u, z + v); }; }
    function grain(f, len, h, n, c) { var s = ''; for (var i = 1; i < n; i++) s += line(f(0, h * i / n), f(len, h * i / n), c, .6); return s; }

    // ---- Furniture. Wall pieces run along a wall ('Y' is the back-right wall, 'X' the back-left); floor pieces stand anywhere. ----
    var SIZE = { shelf: 2, rack: 2, cabinet: 2.2, bookcase: 1.6, forge: 2.4, window: 1.2, herbs: 2,
      counter: [2, 1], table: [1.5, 1], barrel: [.8, .8], crate: [.8, .8], anvil: [.8, .6], glass: [1.4, .8], stall: [2.2, 1], rug: [3, 2], pedestal: [.6, .6], lamp: [.4, .4], sack: [.6, .6],
      tentacle: [.7, .5], monolith: [.9, .6], circle: [2.4, 2.4], cane: [.6, .3], gumdrop: [.7, .6], lolly: [.9, .3], ghost: [.8, .5], coffin: [.8, .4],
      gift: [.7, .7], pine: [1.2, 1.2], coral: [.9, .6], kelp: [.6, .4], mushroom: [.8, .8], bloom: [.7, .5], urn: [.6, .6], palm: [.9, .9], pillar: [.8, .8], brazier: [.8, .8] };
    var FLAT = { rug: 1, circle: 1 };
    var WALLK = { shelf: 1, rack: 1, cabinet: 1, bookcase: 1, forge: 1, window: 1, herbs: 1 };
    // off is how far the piece stands out from y=0 (or x=0): zero on a straight wall, more where a curved wall bends away.
    function wb(p, u0, v0, z0, lu, lv, lz, c, o) { var f = p.off || 0; return p.wall === 'Y' ? box(p.x + u0, f + v0, z0, lu, lv, lz, c, o) : box(f + v0, p.y + u0, z0, lv, lu, lz, c, o); }
    function wpt(p, u, v, z) { var f = p.off || 0; return p.wall === 'Y' ? [p.x + u, f + v, z] : [f + v, p.y + u, z]; }
    function wf(p, u0, dv, z0) { var f = p.off || 0; return function (u, v) { return p.wall === 'Y' ? P(p.x + u0 + u, f + dv, z0 + v) : P(f + dv, p.y + u0 + u, z0 + v); }; }
    // Shadows are drawn together in one blurred layer under the furniture (see castShadows), which is far cheaper than blurring each one.
    function shadow() { return ''; }
    // Living details are plain shapes with classes; the widget's CSS moves them.
    // Offsets come from the piece id so a room looks the same every time it is drawn.
    function jit(id, i, n) { var v = Math.sin(id * 12.9898 + i * 78.233) * 43758.5453; return (v - Math.floor(v)) * n; }
    function flames(x, y, k) {
      var f = function (dx, h, w, c, d) { return '<path class="shr-flame" style="animation-delay:-' + d + 's" d="M' + (x + dx).toFixed(1) + ' ' + y.toFixed(1) + 'c' + (-w) + ' ' + (-h * .4) + ' ' + (-w * .3) + ' ' + (-h * .8) + ' 0 ' + (-h) + 'c' + (w * .3) + ' ' + (h * .2) + ' ' + w + ' ' + (h * .6) + ' 0 ' + h + 'z" fill="' + c + '"/>'; };
      return f(-5 * k, 9 * k, 4 * k, '#ff8a2a', .1) + f(0, 12 * k, 4.5 * k, '#ffb347', .35) + f(5 * k, 8 * k, 3.5 * k, '#ff7a1a', .2) + f(0, 6 * k, 2.5 * k, '#fff0b0', .5);
    }
    function embers(x, y, n, id) {
      var s = '';
      for (var i = 0; i < n; i++) s += '<circle class="shr-ember" style="--dx:' + (jit(id, i, 24) - 12).toFixed(0) + 'px;animation-delay:-' + jit(id, i + 9, 2.6).toFixed(2) + 's;animation-duration:' + (2 + jit(id, i + 3, 1.4)).toFixed(2) + 's" cx="' + (x + jit(id, i + 5, 16) - 8).toFixed(1) + '" cy="' + y.toFixed(1) + '" r="' + (.8 + jit(id, i + 7, .7)).toFixed(1) + '" fill="#ffcf6a"/>';
      return s;
    }
    function motes(a, b, n, id) {
      var s = '';
      for (var i = 0; i < n; i++) { var t = jit(id, i, 1), x = a[0] + (b[0] - a[0]) * t + jit(id, i + 4, 30) - 15, y = a[1] + (b[1] - a[1]) * t;
        s += '<circle class="shr-mote" style="animation-delay:-' + jit(id, i + 2, 10).toFixed(1) + 's;animation-duration:' + (7 + jit(id, i + 6, 6)).toFixed(1) + 's" cx="' + x.toFixed(1) + '" cy="' + y.toFixed(1) + '" r="' + (.5 + jit(id, i + 8, .6)).toFixed(1) + '" fill="' + ((MOODS[S.mood] || {}).mote || '#fff') + '"/>'; }
      return s;
    }
    function sparks(x, y, n, id) {
      var s = '';
      for (var i = 0; i < n; i++) { var a = -Math.PI * (.1 + jit(id, i, .8)), r = 12 + jit(id, i + 3, 14);
        s += '<circle class="shr-spark" style="--dx:' + (Math.cos(a) * r).toFixed(0) + 'px;--dy:' + (Math.sin(a) * r).toFixed(0) + 'px;animation-delay:-' + (i * .04).toFixed(2) + 's" cx="' + x.toFixed(1) + '" cy="' + y.toFixed(1) + '" r="1" fill="#ffe08a"/>'; }
      return s;
    }
    function A(x, y, z, o) { o = o || {}; return { x: x, y: y, z: z, rot: o.rot || 0, s: o.s || 1 }; }
    function prng(seed) { return rng(seed); }

    var DRAW = {
      shelf: function (p) {
        var L = p.len, w = C.wood, s = wb(p, 0, 0, 0, L, .08, 2.5, dk(w, .12)), a = [];
        s += grain(wf(p, 0, .08, 0), L, 2.5, 9, dk(w, .2));
        s += wb(p, 0, 0, 0, .08, .55, 2.5, w);
        [.05, .85, 1.65, 2.42].forEach(function (z, i) {
          s += wb(p, .08, 0, z, L - .16, .55, .07, lt(w, .06));
          if (i < 3) [.25, .5, .75].forEach(function (f) { a.push(A.apply(null, wpt(p, L * f, .32, z + .07).concat([{}]))); });
        });
        return { pre: s, post: wb(p, L - .08, 0, 0, .08, .55, 2.5, w), a: a };
      },
      rack: function (p) {
        var L = p.len, w = C.wood, s = wb(p, 0, 0, .95, L, .1, 1.05, dk(w, .05)), a = [];
        s += grain(wf(p, 0, .1, .95), L, 1.05, 4, dk(w, .25));
        [.2, .5, .8].forEach(function (f) { s += wb(p, L * f - .04, 0, 1.25, .08, .3, .06, dk(w, .3)); s += wb(p, L * f - .04, 0, 1.7, .08, .3, .06, dk(w, .3)); a.push(A.apply(null, wpt(p, L * f, .22, 1.18).concat([{ rot: p.wall === 'Y' ? -38 : 38, s: 1.35 }]))); });
        return { pre: s, post: '', a: a };
      },
      cabinet: function (p) {
        var L = p.len, w = C.wood, s = wb(p, 0, 0, 0, L, .6, 1.7, w), f = wf(p, 0, .6, 0), a = [];
        for (var r = 0; r < 4; r++) for (var c = 0; c < 5; c++) {
          var u0 = .08 + c * (L - .16) / 5, v0 = .1 + r * .38, du = (L - .16) / 5 - .05;
          s += poly([f(u0, v0), f(u0 + du, v0), f(u0 + du, v0 + .32), f(u0, v0 + .32)], lt(w, .1), ' stroke="' + dk(w, .35) + '" stroke-width=".7"');
          s += poly([f(u0 + du * .25, v0 + .2), f(u0 + du * .75, v0 + .2), f(u0 + du * .75, v0 + .28), f(u0 + du * .25, v0 + .28)], '#efe6cf');
          var k = f(u0 + du / 2, v0 + .12); s += '<circle cx="' + k[0].toFixed(1) + '" cy="' + k[1].toFixed(1) + '" r="1.6" fill="#c9a54b"/>';
        }
        s += wb(p, -.04, 0, 1.7, L + .08, .66, .08, lt(w, .1));
        [.2, .5, .8].forEach(function (fr) { a.push(A.apply(null, wpt(p, L * fr, .32, 1.78).concat([{}]))); });
        return { pre: s, post: '', a: a };
      },
      bookcase: function (p) {
        var L = p.len, w = dk(C.wood, .15), s = wb(p, 0, 0, 0, L, .5, 2.6, w, { l: dk(w, .45), r: dk(w, .5) }), f = wf(p, 0, .5, 0), a = [], r = prng(p.id * 31 + 7);
        var cols = ['#7f1d1d', '#1e3a5f', '#14532d', '#4a1d5e', '#713f12', '#334155', '#9a3412', '#3f6212'];
        [0.12, .74, 1.36, 1.98].forEach(function (z, row) {
          var u = .1, gapAt = row === 1 || row === 3 ? .4 + r() * (L - 1) : -1;
          s += wb(p, .05, 0, z - .05, L - .1, .5, .05, lt(C.wood, .05));
          while (u < L - .14) {
            if (gapAt > 0 && u > gapAt && u < gapAt + .45) { if (u < gapAt + .05) a.push(A.apply(null, wpt(p, gapAt + .22, .3, z).concat([{ s: .9 }]))); u += .45; continue; }
            var bw = .05 + r() * .06, bh = .34 + r() * .18, c = cols[Math.floor(r() * cols.length)];
            if (u + bw > L - .1) break;
            s += poly([f(u, z), f(u + bw, z), f(u + bw, z + bh), f(u, z + bh)], c, ' stroke="rgba(0,0,0,.35)" stroke-width=".4"');
            s += line(f(u + bw * .2, z + bh * .8), f(u + bw * .8, z + bh * .8), 'rgba(255,215,120,.6)', .6);
            u += bw + .008;
          }
        });
        return { pre: s, post: '', a: a };
      },
      forge: function (p) {
        var L = p.len, st = mix(C.wall, '#6b6560', .6), s = wb(p, .5, 0, 1.5, 1.4, .8, WH - 1.5, dk(st, .1)), f, a = [];
        s += wb(p, 0, 0, 0, L, 1.1, 1.5, st);
        f = wf(p, 0, 1.1, 0);
        for (var v = .25; v < 1.5; v += .25) s += line(f(0, v), f(L, v), dk(st, .35), .7);
        for (var i = 0; i < 6; i++) for (var j = 0; j < 9; j++) { var uu = j * .3 + (i % 2) * .15; if (uu < L) s += line(f(uu, i * .25), f(uu, i * .25 + .25), dk(st, .35), .7); }
        var m = [f(.7, .15), f(1.7, .15), f(1.7, .8), f(1.45, 1.05), f(.95, 1.05), f(.7, .8)];
        s += poly(m, 'url(#gFire)', ' stroke="' + dk(st, .5) + '" stroke-width="2"');
        s += poly([f(.8, .15), f(1.6, .15), f(1.6, .3), f(.8, .3)], '#3a1607');
        [[.95, .22], [1.2, .26], [1.4, .2]].forEach(function (q) { var c = f(q[0], q[1]); s += '<circle cx="' + c[0].toFixed(1) + '" cy="' + c[1].toFixed(1) + '" r="2.4" fill="#ffcf6b"/>'; });
        p.light = f(1.2, .5); p.warm = wpt(p, 1.2, 1.6, 0);
        var mo = f(1.2, .3);
        p.life = flames(mo[0], mo[1] - 2, 1) + embers(mo[0], mo[1] - 6, 7, p.id) + (efx().smoke ? smokeAt(mo[0], mo[1] - 24, p.id) : '');
        a.push(A.apply(null, wpt(p, .25, .7, 1.5).concat([{}]))); a.push(A.apply(null, wpt(p, L - .25, .7, 1.5).concat([{}])));
        return { pre: s, post: '', a: a };
      },
      window: function (p) {
        var L = p.len, f = wf(p, 0, .01, 1.1), s = '', fr = dk(C.wood, .2);
        s += poly([f(-.08, -.08), f(L + .08, -.08), f(L + .08, 1.3), f(-.08, 1.3)], fr);
        s += poly([f(0, 0), f(L, 0), f(L, 1.22), f(0, 1.22)], 'url(#gSky)') + weather(p.id, [f(0, 0), f(L, 0), f(L, 1.22), f(0, 1.22)], 14);
        s += line(f(L / 2, 0), f(L / 2, 1.22), fr, 3) + line(f(0, .61), f(L, .61), fr, 3);
        s += wb(p, -.1, 0, 1.0, L + .2, .22, .1, lt(C.wood, .1));
        // A pool of daylight on the floor in front of the window.
        var o = p.off || 0, g = p.wall === 'Y' ? [P(p.x, o + .2, 0), P(p.x + L, o + .2, 0), P(p.x + L + .8, o + 2.4, 0), P(p.x + .8, o + 2.4, 0)] : [P(o + .2, p.y, 0), P(o + .2, p.y + L, 0), P(o + 2.4, p.y + L + .8, 0), P(o + 2.4, p.y + .8, 0)];
        p.light = f(L / 2, .6); p.lightR = 90; p.cool = 1;
        p.floorLight = poly(g, 'rgba(255,240,200,.18)');
        // A faint shaft of daylight from the window down to that pool.
        p.shaft = poly(hull([f(0, 0), f(L, 0), f(L, 1.22), f(0, 1.22)].concat(g)), 'url(#gShaft)');
        var pool = [(g[0][0] + g[1][0] + g[2][0] + g[3][0]) / 4, (g[0][1] + g[1][1] + g[2][1] + g[3][1]) / 4];
        p.life = motes(f(L / 2, .6), pool, 9, p.id);
        return { pre: s, post: '', a: [] };
      },
      herbs: function (p) {
        var L = p.len, s = '', a = [], f = wf(p, 0, .5, 0);
        [.15, .35, .55, .75, .9].forEach(function (fr, i) { var top = f(L * fr, WH), bot = f(L * fr, WH - .6 - (i % 3) * .2); s += line(top, bot, 'rgba(40,30,20,.6)', .8); a.push(A.apply(null, wpt(p, L * fr, .5, WH - 1.25 - (i % 3) * .2).concat([{ deco: 1, hang: 1 }]))); });
        return { pre: s, post: '', a: a, decoOnly: 1 };
      },
      counter: function (p) {
        var w = C.wood, s = shadow(p.x, p.y, p.w, p.d) + box(p.x, p.y, 0, p.w, p.d, .95, w), fl = onL(p.x, p.y, 0, p.d), fr = onR(p.x, p.y, 0, p.w), a = [];
        for (var u = .1; u < p.w - .1; u += .5) s += poly([fl(u + .06, .12), fl(u + .44, .12), fl(u + .44, .8), fl(u + .06, .8)], 'none', ' stroke="' + dk(w, .35) + '" stroke-width=".8"');
        s += poly([fr(.1, .12), fr(p.d - .1, .12), fr(p.d - .1, .8), fr(.1, .8)], 'none', ' stroke="' + dk(w, .5) + '" stroke-width=".8"');
        s += box(p.x - .06, p.y - .06, .95, p.w + .12, p.d + .12, .1, lt(w, .14));
        a.push(A(p.x + p.w * .7, p.y + p.d * .55, 1.05)); a.push(A(p.x + p.w * .35, p.y + p.d * .55, 1.05));
        p.keeperAt = P(p.x + p.w * .45, p.y + .15, 2.1);
        return { pre: s, post: '', a: a };
      },
      table: function (p) {
        var w = C.wood, s = shadow(p.x, p.y, p.w, p.d), a = [];
        [[.05, .05], [p.w - .15, .05], [.05, p.d - .15], [p.w - .15, p.d - .15]].forEach(function (q) { s += box(p.x + q[0], p.y + q[1], 0, .1, .1, .8, dk(w, .1)); });
        s += box(p.x, p.y, .8, p.w, p.d, .09, lt(w, .06));
        s += grain(onL(p.x, p.y, .8, p.d), p.w, .09, 2, dk(w, .3));
        [.25, .55, .8].forEach(function (f, i) { a.push(A(p.x + p.w * f, p.y + p.d * (i % 2 ? .35 : .65), .89)); });
        return { pre: s, post: '', a: a };
      },
      barrel: function (p) {
        var c = P(p.x + .4, p.y + .4, 0), t = P(p.x + .4, p.y + .4, .9), rx = 21, ry = 10.5, s = shadow(p.x, p.y, .8, .8);
        s += '<path d="M' + (c[0] - rx) + ',' + t[1] + ' L' + (c[0] - rx) + ',' + c[1] + ' A' + rx + ',' + ry + ' 0 0 0 ' + (c[0] + rx) + ',' + c[1] + ' L' + (c[0] + rx) + ',' + t[1] + ' Z" fill="url(#gBarrel)"/>';
        [.18, .72].forEach(function (f) { var y = c[1] + (t[1] - c[1]) * f; s += '<path d="M' + (c[0] - rx) + ',' + y + ' A' + rx + ',' + ry + ' 0 0 0 ' + (c[0] + rx) + ',' + y + '" fill="none" stroke="#3f3a34" stroke-width="2.4"/>'; });
        s += '<ellipse cx="' + t[0] + '" cy="' + t[1] + '" rx="' + rx + '" ry="' + ry + '" fill="' + dk('#7a5634', .25) + '" stroke="#3f3a34" stroke-width="2"/>';
        s += line([t[0] - rx * .6, t[1] - 3], [t[0] + rx * .6, t[1] - 3], 'rgba(0,0,0,.3)', .7) + line([t[0] - rx * .7, t[1] + 3], [t[0] + rx * .7, t[1] + 3], 'rgba(0,0,0,.3)', .7);
        return { pre: s, post: '', a: [A(p.x + .4, p.y + .4, .9)] };
      },
      crate: function (p) {
        var cw = '#8a6a45', s = shadow(p.x, p.y, .8, .8) + box(p.x, p.y, 0, .8, .8, .7, cw), fl = onL(p.x, p.y, 0, .8), fr = onR(p.x, p.y, 0, .8);
        [fl, fr].forEach(function (f) { s += line(f(0, .06), f(.8, .06), dk(cw, .45), 2) + line(f(0, .64), f(.8, .64), dk(cw, .45), 2) + line(f(.06, .06), f(.74, .64), dk(cw, .4), 2) + line(f(0, .35), f(.8, .35), dk(cw, .3), .6); });
        return { pre: s, post: '', a: [A(p.x + .4, p.y + .4, .7)] };
      },
      anvil: function (p) {
        var ir = '#4b5058', s = shadow(p.x, p.y, .8, .6);
        s += box(p.x + .15, p.y + .1, 0, .5, .4, .14, '#3a2d22') + box(p.x + .28, p.y + .2, .14, .26, .2, .28, ir) + box(p.x + .05, p.y + .12, .42, .7, .36, .16, ir);
        s += poly([P(p.x + .75, p.y + .2, .58), P(p.x + 1.0, p.y + .3, .52), P(p.x + .75, p.y + .4, .5)], lt(ir, .1));
        var top = P(p.x + .45, p.y + .3, .6);
        p.life = sparks(top[0], top[1], 7, p.id);
        return { pre: s, post: '', a: [A(p.x + .4, p.y + .3, .58)] };
      },
      glass: function (p) {
        var base = dk(C.acc, .45), s = shadow(p.x, p.y, p.w, p.d) + box(p.x, p.y, 0, p.w, p.d, .45, base) + box(p.x + .04, p.y + .04, .45, p.w - .08, p.d - .08, .01, dk(C.acc, .2));
        var a = [A(p.x + p.w * .3, p.y + p.d * .5, .46), A(p.x + p.w * .7, p.y + p.d * .5, .46)];
        var g = box(p.x, p.y, .45, p.w, p.d, .6, '#ffffff', { l: 'rgba(255,255,255,.14)', r: 'rgba(255,255,255,.1)', t: 'rgba(255,255,255,.12)', lx: ' stroke="rgba(255,255,255,.7)" stroke-width=".8"', rx: ' stroke="rgba(255,255,255,.6)" stroke-width=".8"', tx: ' stroke="rgba(255,255,255,.7)" stroke-width=".8"' });
        g += line(P(p.x + .2, p.y + p.d, .55), P(p.x + .5, p.y + p.d, .95), 'rgba(255,255,255,.55)', 2);
        return { pre: s, post: g, a: a };
      },
      stall: function (p) {
        var s = DRAW.table({ x: p.x, y: p.y, w: p.w, d: p.d }).pre, a = DRAW.table({ x: p.x, y: p.y, w: p.w, d: p.d }).a, post = '', n = 8;
        [[0, p.d + .2], [p.w - .08, p.d + .2]].forEach(function (q) { post += box(p.x + q[0], p.y + q[1], 0, .08, .08, 1.8, dk(C.wood, .2)); });
        for (var i = 0; i < n; i++) { var u0 = p.w * i / n, u1 = p.w * (i + 1) / n; post += poly([P(p.x + u0, p.y - .1, 2.3), P(p.x + u1, p.y - .1, 2.3), P(p.x + u1, p.y + p.d + .3, 1.8), P(p.x + u0, p.y + p.d + .3, 1.8)], i % 2 ? dk(C.acc, .25) : '#efe4cf', ' stroke="rgba(0,0,0,.2)" stroke-width=".5"'); }
        post += poly([P(p.x, p.y + p.d + .3, 1.8), P(p.x + p.w, p.y + p.d + .3, 1.8), P(p.x + p.w, p.y + p.d + .3, 1.66), P(p.x, p.y + p.d + .3, 1.66)], dk(C.acc, .4));
        return { pre: s, post: post, a: a };
      },
      rug: function (p) {
        var r = C.acc, s = poly([P(p.x, p.y, .01), P(p.x + p.w, p.y, .01), P(p.x + p.w, p.y + p.d, .01), P(p.x, p.y + p.d, .01)], dk(r, .35));
        s += poly([P(p.x + .15, p.y + .15, .012), P(p.x + p.w - .15, p.y + .15, .012), P(p.x + p.w - .15, p.y + p.d - .15, .012), P(p.x + .15, p.y + p.d - .15, .012)], dk(r, .1), ' stroke="#e9cf7c" stroke-width="1.2"');
        s += poly([P(p.x + p.w / 2, p.y + .35, .013), P(p.x + p.w - .45, p.y + p.d / 2, .013), P(p.x + p.w / 2, p.y + p.d - .35, .013), P(p.x + .45, p.y + p.d / 2, .013)], 'none', ' stroke="#e9cf7c" stroke-width="1"');
        return { pre: s, post: '', a: [] };
      },
      pedestal: function (p) {
        var st = '#9b97a6', s = shadow(p.x, p.y, .6, .6) + box(p.x, p.y, 0, .6, .6, .12, dk(st, .1)) + box(p.x + .14, p.y + .14, .12, .32, .32, .78, st) + box(p.x + .04, p.y + .04, .9, .52, .52, .1, lt(st, .1));
        return { pre: s, post: '', a: [A(p.x + .3, p.y + .3, 1.0, { s: 1.15 })] };
      },
      lamp: function (p) {
        var top = P(p.x + .2, p.y + .2, WH), b = P(p.x + .2, p.y + .2, 2.3), s = line(top, b, '#222', 1);
        s += '<rect x="' + (b[0] - 6) + '" y="' + b[1] + '" width="12" height="16" rx="2" fill="url(#gLamp)" stroke="#1f1f1f" stroke-width="1.5"/>';
        p.light = [b[0], b[1] + 8]; p.warm = [p.x + .2, p.y + .2];
        return { pre: s, post: '', a: [] };
      },
      sack: function (p) {
        var c = P(p.x + .3, p.y + .3, 0), s = shadow(p.x, p.y, .6, .6);
        s += '<path d="M' + (c[0] - 15) + ',' + c[1] + ' C' + (c[0] - 19) + ',' + (c[1] - 22) + ' ' + (c[0] - 8) + ',' + (c[1] - 30) + ' ' + c[0] + ',' + (c[1] - 32) + ' C' + (c[0] + 8) + ',' + (c[1] - 30) + ' ' + (c[0] + 19) + ',' + (c[1] - 22) + ' ' + (c[0] + 15) + ',' + c[1] + ' Q' + c[0] + ',' + (c[1] + 7) + ' ' + (c[0] - 15) + ',' + c[1] + 'Z" fill="url(#gSack)"/>';
        s += line([c[0] - 6, c[1] - 30], [c[0] + 6, c[1] - 30], '#5b4630', 2);
        return { pre: s, post: '', a: [] };
      }
    };


    // ---- Mood furniture. Each stands on the centre of its footprint and is
    // drawn in screen space with soft gradients, so it sits in the lit room
    // like the rest rather than as a flat cut-out. ----
    function foot(p) { return P(p.x + p.w / 2, p.y + p.d / 2, 0); }
    function gl(c) { if (S.fx !== 'full') return c; var id = 'gm' + c.slice(1); GRAD[id] = c; return 'url(#' + id + ')'; }
    function pa(d, fill, ex) { return '<path d="' + d + '" fill="' + fill + '"' + (ex || '') + '/>'; }
    function n1(v) { return (Math.round(v * 10) / 10).toString(); }
    function bez(a, b, c, d, n) { var o = []; for (var i = 0; i <= n; i++) { var t = i / n, u = 1 - t; o.push([u * u * u * a[0] + 3 * u * u * t * b[0] + 3 * u * t * t * c[0] + t * t * t * d[0], u * u * u * a[1] + 3 * u * u * t * b[1] + 3 * u * t * t * c[1] + t * t * t * d[1]]); } return o; }
    // A tapered tube along a centre line: tentacles, kelp, coral, trunks.
    function tube(c, w0, w1) {
      var L = [], R = [];
      c.forEach(function (q, i) {
        var a = c[Math.max(0, i - 1)], b = c[Math.min(c.length - 1, i + 1)], dx = b[0] - a[0], dy = b[1] - a[1], m = Math.sqrt(dx * dx + dy * dy) || 1, w = (w0 + (w1 - w0) * i / (c.length - 1)) / 2;
        L.push(n1(q[0] - dy / m * w) + ' ' + n1(q[1] + dx / m * w)); R.unshift(n1(q[0] + dy / m * w) + ' ' + n1(q[1] - dx / m * w));
      });
      return 'M' + L.join('L') + 'L' + R.join('L') + 'Z';
    }
    function dot(x, y, r, f, ex) { return '<circle cx="' + n1(x) + '" cy="' + n1(y) + '" r="' + r + '" fill="' + f + '"' + (ex || '') + '/>'; }
    function oval(x, y, rx, ry, f, ex) { return '<ellipse cx="' + n1(x) + '" cy="' + n1(y) + '" rx="' + n1(rx) + '" ry="' + n1(ry) + '" fill="' + f + '"' + (ex || '') + '/>'; }
    function smokeAt(x, y, id) { var s = ''; for (var j = 0; j < 4; j++) s += '<circle class="shr-smoke" cx="' + n1(x) + '" cy="' + n1(y) + '" r="6" fill="#6b6460" style="animation-delay:-' + (j * .9 + jit(id, j, .4)).toFixed(2) + 's"/>'; return s; }
    var MOODDRAW = {
      tentacle: function (p) {
        var c = foot(p), h = HT.tentacle * ZH, x = c[0], y = c[1], s = oval(x, y, 20, 7, 'rgba(20,10,30,.55)');
        [[-7, 1, 13], [9, .7, 9]].forEach(function (t, k) {
          var cl = bez([x + t[0], y], [x + t[0] - 18 * t[1], y - h * .4 * t[1]], [x + t[0] + 24 * t[1], y - h * .72 * t[1]], [x + t[0] + 6, y - h * t[1]], 14);
          s += pa(tube(cl, t[2], 2), gl(k ? '#57447e' : '#6b4f96'));
          for (var i = 2; i < 12; i += 2) { var q = cl[i], r = 2.4 - i * .15; s += oval(q[0] + (k ? -1 : 1) * (t[2] / 2 - i * .4), q[1], r, r * .75, '#d9b8f0', ' opacity=".9"'); }
        });
        return { pre: s, post: '', a: [] };
      },
      monolith: function (p) {
        var x = p.x + .15, y = p.y + .1, w = p.w - .3, d = p.d - .2, h = HT.monolith, s = box(x, y, 0, w, d, h, '#3b3548'), f = onL(x, y, 0, d);
        for (var v = .3; v < h * .55; v += .3) s += '<g class="shr-pulse">' + line(f(w * .2, v), f(w * .8, v + .08), '#6dffa8', 1.2) + '</g>';
        var e = f(w / 2, h * .74); s += oval(e[0], e[1], 7, 4, '#d9ffe6') + dot(e[0], e[1], 2.6, '#0f3a24');
        p.light = e; p.lightR = 80;
        return { pre: s, post: '', a: [] };
      },
      circle: function (p) {
        var cx = p.x + p.w / 2, cy = p.y + p.d / 2, r = Math.min(p.w, p.d) / 2 - .1, ring = function (rr) { var o = []; for (var a = 0; a < 32; a++) o.push(P(cx + Math.cos(a / 32 * Math.PI * 2) * rr, cy + Math.sin(a / 32 * Math.PI * 2) * rr, .01)); return o; };
        var s = '<g class="shr-pulse">' + poly(ring(r), 'rgba(20,60,40,.35)', ' stroke="#7cf0a0" stroke-width="2.4"') + poly(ring(r * .72), 'none', ' stroke="#7cf0a0" stroke-width="1.3"');
        var st = []; for (var k = 0; k < 5; k++) { var a = -Math.PI / 2 + k * 4 * Math.PI / 5; st.push(P(cx + Math.cos(a) * r * .72, cy + Math.sin(a) * r * .72, .01)); }
        s += poly(st, 'none', ' stroke="#7cf0a0" stroke-width="1.3"') + '</g>';
        p.light = P(cx, cy, .3); p.lightR = 120;
        return { pre: s, post: '', a: [] };
      },
      cane: function (p) {
        var c = foot(p), h = HT.cane * ZH, x = c[0] - 4, y = c[1], d = 'M' + n1(x) + ' ' + n1(y) + 'V' + n1(y - h * .78) + 'a9 9 0 0 1 18 0v6';
        return { pre: oval(c[0], y, 9, 3, 'rgba(0,0,0,.25)') + '<path d="' + d + '" fill="none" stroke="rgba(60,10,20,.45)" stroke-width="9" stroke-linecap="round"/><path d="' + d + '" fill="none" stroke="#fff6f0" stroke-width="7" stroke-linecap="round"/><path d="' + d + '" fill="none" stroke="#e8364f" stroke-width="7" stroke-dasharray="5 6"/><path d="' + d + '" fill="none" stroke="#fff" stroke-opacity=".5" stroke-width="1.5" transform="translate(-1.5 0)"/>', post: '', a: [] };
      },
      gumdrop: function (p) {
        var c = foot(p), h = HT.gumdrop * ZH, x = c[0], y = c[1], r = p.w * 20, col = ['#ff6fae', '#7fd6ff', '#9be37a'][p.id % 3], s = oval(x, y, r, r * .35, 'rgba(0,0,0,.22)');
        s += pa('M' + n1(x - r) + ' ' + n1(y) + 'C' + n1(x - r) + ' ' + n1(y - h * 1.4) + ' ' + n1(x + r) + ' ' + n1(y - h * 1.4) + ' ' + n1(x + r) + ' ' + n1(y) + 'Q' + n1(x) + ' ' + n1(y + 6) + ' ' + n1(x - r) + ' ' + n1(y) + 'Z', gl(col));
        for (var k = 0; k < 9; k++) s += dot(x - r * .7 + jit(p.id, k, r * 1.4), y - 3 - jit(p.id, k + 4, h * .8), .9, '#fff', ' opacity=".85"');
        return { pre: s, post: '', a: [] };
      },
      lolly: function (p) {
        var c = foot(p), h = HT.lolly * ZH, x = c[0], y = c[1], cy = y - h * .74, r = 15, sp = 'M' + n1(x) + ' ' + n1(cy);
        for (var t = 0; t < 14; t += .5) sp += 'L' + n1(x + Math.cos(t) * t * r / 14) + ' ' + n1(cy + Math.sin(t) * t * r / 14);
        return { pre: oval(x, y, 10, 3.5, 'rgba(0,0,0,.25)') + line([x, y], [x, cy], '#fff4e6', 4) + dot(x, cy, r, gl('#ffb0d6')) + '<path d="' + sp + '" fill="none" stroke="#ff3d8f" stroke-width="3" stroke-linecap="round"/>', post: '', a: [] };
      },
      ghost: function (p) {
        var c = foot(p), h = HT.ghost * ZH, x = c[0], y = c[1] - 10, w = 22;
        var d = 'M' + n1(x - w) + ' ' + n1(y) + 'C' + n1(x - w) + ' ' + n1(y - h * 1.2) + ' ' + n1(x + w) + ' ' + n1(y - h * 1.2) + ' ' + n1(x + w) + ' ' + n1(y) + 'l-7-6-7 6-8-6-8 6-7-6z';
        p.light = [x, y - h * .5]; p.lightR = 60; p.cool = 1;
        return { pre: oval(x, c[1], 16, 5, 'rgba(0,0,0,.25)') + '<g class="shr-haunt">' + pa(d, '#eef3ff', ' opacity=".82"') + oval(x - 7, y - h * .55, 3, 5, '#1d2233') + oval(x + 7, y - h * .55, 3, 5, '#1d2233') + oval(x, y - h * .32, 4, 5.5, '#1d2233') + '</g>', post: '', a: [] };
      },
      coffin: function (p) {
        var x = p.x + .1, y = p.y + .05, w = p.w - .2, d = p.d - .1, s = box(x, y, 0, w, d, HT.coffin, '#4a3528'), f = onL(x, y, 0, d);
        s += poly([f(w * .44, .55), f(w * .56, .55), f(w * .56, 1.9), f(w * .44, 1.9)], '#c9b48a') + poly([f(w * .28, 1.45), f(w * .72, 1.45), f(w * .72, 1.6), f(w * .28, 1.6)], '#c9b48a');
        return { pre: s, post: '', a: [] };
      },
      gift: function (p) {
        var x = p.x + .05, y = p.y + .05, w = p.w - .1, d = p.d - .1, s = box(x, y, 0, w, d, .45, '#c8102e');
        s += box(x + w / 2 - .05, y - .01, 0, .1, d + .02, .46, '#e9c46a', { edge: false }) + box(x - .01, y + d / 2 - .05, 0, w + .02, .1, .46, '#e9c46a', { edge: false });
        s += box(x + .15, y + .12, .46, w - .3, d - .3, .3, '#2f7a4a') + box(x + w / 2 - .04, y + .12, .46, .08, d - .3, .31, '#e9c46a', { edge: false });
        var t = P(x + w / 2, y + d / 2 - .03, .78); s += oval(t[0] - 6, t[1] - 3, 6, 3.5, '#f2d27a') + oval(t[0] + 6, t[1] - 3, 6, 3.5, '#f2d27a') + dot(t[0], t[1] - 2, 2.4, '#d4a72c');
        return { pre: s, post: '', a: [] };
      },
      pine: function (p) {
        var c = foot(p), h = HT.pine * ZH, x = c[0], y = c[1], s = box(p.x + p.w / 2 - .1, p.y + p.d / 2 - .1, 0, .2, .2, .35, '#6b4a2e'), hw = p.w * 30;
        for (var i = 0; i < 3; i++) {
          var b = y - h * (.12 + .26 * i), t = b - h * .42, w = hw * (1 - .24 * i);
          s += pa('M' + n1(x - w) + ' ' + n1(b) + 'L' + n1(x) + ' ' + n1(t) + 'L' + n1(x) + ' ' + n1(b + 5) + 'Z', '#3f8a50') + pa('M' + n1(x) + ' ' + n1(t) + 'L' + n1(x + w) + ' ' + n1(b) + 'L' + n1(x) + ' ' + n1(b + 5) + 'Z', '#2a6339');
          for (var k = 0; k < 4; k++) { var u = jit(p.id, i * 7 + k, 1.6) - .8, v = jit(p.id, i * 7 + k + 3, .7); s += dot(x + u * w * (1 - v) * .9, b - v * h * .36, 2.4, ['#e8364f', '#e9c46a', '#7fd6ff'][(i + k) % 3]); }
          for (k = 0; k < 5; k++) s += dot(x - w * .8 + k * w * .4, b - 2 - Math.abs(k - 2) * 1.5, 1.3, '#fff3b0', ' class="shr-twinkle" style="animation-delay:-' + (k * .7 + i * .3).toFixed(1) + 's"');
        }
        var top = y - h * .94; s += pa('M' + n1(x) + ' ' + n1(top - 8) + 'l2.4 5.6 6 .4-4.6 3.8 1.5 5.8-5.3-3.2-5.3 3.2 1.5-5.8-4.6-3.8 6-.4z', '#ffd84a');
        p.light = [x, y - h * .5]; p.lightR = 80; p.warm = [p.x + p.w / 2, p.y + p.d / 2];
        return { pre: s, post: '', a: [] };
      },
      coral: function (p) {
        var c = foot(p), h = HT.coral * ZH, x = c[0], y = c[1], s = oval(x, y, 18, 6, '#6b6f73') + oval(x - 3, y - 2, 13, 4, '#8a8f94');
        [[-14, -.9, '#ff7f6e'], [-5, -.3, '#ff9a7a'], [4, .2, '#ff7f6e'], [13, .8, '#ffb36e'], [0, 0, '#ff6f61']].forEach(function (b, k) {
          var hh = h * (.65 + jit(p.id, k, .35)), cl = bez([x + b[0] * .4, y - 3], [x + b[0] * .6, y - hh * .4], [x + b[0] + b[1] * 6, y - hh * .7], [x + b[0] * 1.2 + b[1] * 8, y - hh], 8);
          s += pa(tube(cl, 6, 3.5), gl(b[2])) + dot(cl[8][0], cl[8][1], 2.6, b[2]);
          var m = cl[4]; s += pa(tube(bez(m, [m[0] + b[1] * 8 + 4, m[1] - 4], [m[0] + b[1] * 10 + 6, m[1] - 10], [m[0] + b[1] * 12 + 7, m[1] - 15], 5), 4, 2.5), b[2]);
        });
        return { pre: s, post: '', a: [] };
      },
      kelp: function (p) {
        var c = foot(p), h = HT.kelp * ZH, x = c[0], y = c[1], s = oval(x, y, 12, 4, 'rgba(0,0,0,.25)');
        [[-6, '#3f8a4e', 1], [3, '#5aa85e', .86], [9, '#4a7a3a', .7]].forEach(function (k, i) {
          var hh = h * k[2], cl = bez([x + k[0], y], [x + k[0] + 12, y - hh * .35], [x + k[0] - 12, y - hh * .7], [x + k[0] + 6, y - hh], 16);
          s += pa(tube(cl, 7, 3), gl(k[1]));
          [5, 10, 14].forEach(function (j) { s += oval(cl[j][0] + 3, cl[j][1], 2.2, 1.6, '#c9b86a'); });
        });
        return { pre: s, post: '', a: [] };
      },
      mushroom: function (p) {
        var c = foot(p), h = HT.mushroom * ZH, x = c[0], y = c[1], glow = p.id % 2, cap = glow ? '#b06ad8' : '#d8434b', s = oval(x, y, 16, 5, 'rgba(0,0,0,.25)');
        s += pa(tube(bez([x, y], [x - 3, y - h * .3], [x + 3, y - h * .6], [x, y - h * .78], 6), 11, 8), gl('#f2e6cf'));
        var cy = y - h * .74; s += pa('M' + n1(x - 24) + ' ' + n1(cy) + 'C' + n1(x - 24) + ' ' + n1(cy - 26) + ' ' + n1(x + 24) + ' ' + n1(cy - 26) + ' ' + n1(x + 24) + ' ' + n1(cy) + 'Q' + n1(x) + ' ' + n1(cy + 6) + ' ' + n1(x - 24) + ' ' + n1(cy) + 'Z', gl(cap));
        [[-12, -8, 3], [2, -14, 3.6], [13, -7, 2.6], [-3, -4, 2]].forEach(function (q) { s += dot(x + q[0], cy + q[1], q[2], '#fff6ea', glow ? ' class="shr-pulse"' : ''); });
        if (glow) { p.light = [x, cy - 8]; p.lightR = 70; }
        return { pre: s, post: '', a: [] };
      },
      bloom: function (p) {
        var c = foot(p), h = HT.bloom * ZH, x = c[0], y = c[1], s = oval(x, y, 12, 4, 'rgba(0,0,0,.25)'), cl = bez([x, y], [x - 6, y - h * .3], [x + 6, y - h * .6], [x, y - h * .8], 8), t = cl[8];
        s += pa(tube(cl, 4, 2.5), '#4f8a3a') + pa('M' + n1(cl[3][0]) + ' ' + n1(cl[3][1]) + 'q-16-2-20-12q14-2 20 12z', '#5aa85e') + pa('M' + n1(cl[5][0]) + ' ' + n1(cl[5][1]) + 'q16-4 18-14q-14 0-18 14z', '#4f8a3a');
        for (var k = 0; k < 6; k++) { var a = k * Math.PI / 3; s += oval(t[0] + Math.cos(a) * 8, t[1] + Math.sin(a) * 6, 7, 5, gl(p.id % 2 ? '#f6a6d8' : '#c9a6ff'), ' transform="rotate(' + (k * 60) + ' ' + n1(t[0] + Math.cos(a) * 8) + ' ' + n1(t[1] + Math.sin(a) * 6) + ')"'); }
        s += dot(t[0], t[1], 4, '#ffd84a');
        p.light = t; p.lightR = 50;
        return { pre: s, post: '', a: [] };
      },
      urn: function (p) {
        var c = foot(p), h = HT.urn * ZH, x = c[0], y = c[1], s = oval(x, y, 13, 4.5, 'rgba(0,0,0,.28)');
        var d = 'M' + n1(x - 6) + ' ' + n1(y) + 'C' + n1(x - 18) + ' ' + n1(y - h * .3) + ' ' + n1(x - 20) + ' ' + n1(y - h * .62) + ' ' + n1(x - 7) + ' ' + n1(y - h * .82) + 'L' + n1(x - 7) + ' ' + n1(y - h * .95) + 'L' + n1(x - 10) + ' ' + n1(y - h) + 'H' + n1(x + 10) + 'L' + n1(x + 7) + ' ' + n1(y - h * .95) + 'L' + n1(x + 7) + ' ' + n1(y - h * .82) + 'C' + n1(x + 20) + ' ' + n1(y - h * .62) + ' ' + n1(x + 18) + ' ' + n1(y - h * .3) + ' ' + n1(x + 6) + ' ' + n1(y) + 'Z';
        s += pa(d, gl(p.id % 2 ? '#c46a3a' : '#2f7d8a'));
        var by = y - h * .55; s += '<path d="M' + n1(x - 16) + ' ' + n1(by) + 'l4 -4 4 4 4 -4 4 4 4 -4 4 4 4 -4 4 4" fill="none" stroke="#f3e2c0" stroke-width="1.4"/>';
        s += '<path d="M' + n1(x - 7) + ' ' + n1(y - h * .86) + 'q-10 2 -9 12M' + n1(x + 7) + ' ' + n1(y - h * .86) + 'q10 2 9 12" fill="none" stroke="#3a2416" stroke-width="2"/>';
        return { pre: s, post: '', a: [] };
      },
      palm: function (p) {
        var c = foot(p), h = HT.palm * ZH, x = c[0], y = c[1], s = box(p.x + p.w / 2 - .22, p.y + p.d / 2 - .22, 0, .44, .44, .42, '#b8643a');
        var cl = bez([x, y - 12], [x - 4, y - h * .4], [x + 8, y - h * .7], [x + 4, y - h * .86], 10), t = cl[10];
        s += pa(tube(cl, 7, 4.5), gl('#8a6a44'));
        for (var i = 2; i < 10; i += 2) s += line([cl[i][0] - 3, cl[i][1]], [cl[i][0] + 3, cl[i][1] - 1.5], '#5c4426', 1);
        [[-1, -.2], [-.8, .4], [-.3, .9], [.3, .9], [.8, .4], [1, -.2]].forEach(function (f, k) {
          var e = [t[0] + f[0] * 34, t[1] + 10 - f[1] * 16];
          s += pa(tube(bez(t, [t[0] + f[0] * 12, t[1] - 12], [t[0] + f[0] * 26, t[1] - 8], e, 8), 7, 1.5), k % 2 ? '#3f8a50' : '#4f9f5a');
        });
        return { pre: s, post: '', a: [] };
      },
      pillar: function (p) {
        var x = p.x + .05, y = p.y + .05, w = p.w - .1, d = p.d - .1, st = mix(C.wall, '#8a857e', .4), s = box(x, y, 0, w, d, .3, dk(st, .1));
        s += box(x + .1, y + .1, .3, w - .2, d - .2, WH - .7, st) + box(x + .08, y + .08, 1.4, w - .16, d - .16, .18, '#b08a3e') + box(x, y, WH - .4, w, d, .3, dk(st, .1));
        var f = onL(x + .1, y + .1, 0, d - .2); for (var v = .5; v < WH - .5; v += .45) if (Math.abs(v - 1.5) > .25) s += line(f(0, v), f(w - .2, v), dk(st, .25), .7);
        return { pre: s, post: '', a: [] };
      },
      brazier: function (p) {
        var c = foot(p), x = c[0], y = c[1], top = y - HT.brazier * ZH * .78, s = oval(x, y, 18, 6, 'rgba(0,0,0,.3)');
        [[-14, 0], [14, 0], [0, 7]].forEach(function (l) { s += line([x + l[0], y + l[1] - 2], [x + l[0] * .4, top + 6], '#2f2a26', 2.4); });
        s += pa('M' + n1(x - 20) + ' ' + n1(top) + 'Q' + n1(x) + ' ' + n1(top + 22) + ' ' + n1(x + 20) + ' ' + n1(top) + 'Z', gl('#4a4440')) + oval(x, top, 20, 5, '#2a1208') + oval(x, top - .5, 15, 3.4, 'url(#gFire)');
        p.light = [x, top - 8]; p.lightR = 130; p.warm = [p.x + p.w / 2, p.y + p.d / 2];
        p.life = flames(x, top, 1.15) + embers(x, top - 6, 7, p.id) + (efx().smoke ? smokeAt(x, top - 22, p.id) : '');
        return { pre: s, post: '', a: [] };
      }
    };
    for (var mk in MOODDRAW) DRAW[mk] = MOODDRAW[mk];

    function rng(seed) { return function () { seed |= 0; seed = seed + 0x6D2B79F5 | 0; var t = Math.imul(seed ^ seed >>> 15, 1 | seed); t = t + Math.imul(t ^ t >>> 7, 61 | t) ^ t; return ((t ^ t >>> 14) >>> 0) / 4294967296; }; }
    function shuffle(a, r) { a = a.slice(); for (var i = a.length - 1; i > 0; i--) { var j = Math.floor(r() * (i + 1)), t = a[i]; a[i] = a[j]; a[j] = t; } return a; }
    var SZ = { s: 7, m: 8, l: 10 };
    function recOf() {
      var r = RECIPES[S.roomType], M = MOODS[S.mood];
      if (!M || !M.pal) return r;
      return { wall: M.wall, floor: M.floor, need: r.need, opt: r.opt, deco: M.deco };
    }
    // Each type has its own wall and floor finish, the furniture it always gets, a pool to draw extras from, and its decorations.
    var RECIPES = {
      forge: { wall: 'stone', floor: 'flags', need: ['counter', 'forge', 'anvil', 'rack'], opt: ['rack', 'shelf', 'barrel', 'crate', 'crate', 'table', 'window', 'barrel', 'sack'],
        deco: ['fire', 'oil-can', 'hammer', 'link', 'boxes-stacked', 'bucket'] },
      apothecary: { wall: 'plaster', floor: 'plank', need: ['counter', 'cabinet', 'herbs', 'window'], opt: ['shelf', 'shelf', 'table', 'sack', 'barrel', 'glass', 'rug', 'lamp', 'sack'],
        deco: ['leaf', 'seedling', 'jar-wheat', 'wheat-awn', 'spa', 'mortar-pestle', 'feather'] },
      jeweler: { wall: 'panel', floor: 'checker', need: ['counter', 'glass', 'glass', 'pedestal', 'rug'], opt: ['pedestal', 'pedestal', 'shelf', 'window', 'lamp', 'cabinet', 'glass'],
        deco: ['gem', 'star', 'crown', 'scale-balanced', 'feather'] },
      market: { wall: 'brick', floor: 'flags', need: ['counter', 'stall', 'lamp', 'crate'], opt: ['crate', 'crate', 'barrel', 'sack', 'rack', 'shelf', 'lamp', 'table', 'barrel'],
        deco: ['skull', 'spider', 'boxes-stacked', 'box', 'mask', 'key'] },
      library: { wall: 'panel', floor: 'herring', need: ['counter', 'bookcase', 'bookcase', 'table'], opt: ['bookcase', 'pedestal', 'rug', 'lamp', 'window', 'shelf', 'table'],
        deco: ['book', 'book-open', 'scroll', 'hourglass', 'feather', 'moon', 'star'] },
      general: { wall: 'plank', floor: 'plank', need: ['counter', 'shelf', 'shelf', 'barrel'], opt: ['barrel', 'crate', 'sack', 'table', 'rack', 'window', 'rug', 'herbs', 'lamp', 'sack'],
        deco: ['broom', 'bucket', 'bread-slice', 'wheat-awn', 'apple-whole', 'box', 'bell'] }
    };
    var DECOCOL = { fire: '#f97316', 'oil-can': '#6b7280', hammer: '#8b929c', link: '#8b929c', 'boxes-stacked': '#a16207', bucket: '#9a6b3d', leaf: '#65a30d', seedling: '#84cc16',
      'jar-wheat': '#f59e0b', 'wheat-awn': '#d4a72c', spa: '#a78bfa', 'mortar-pestle': '#e7e2d4', feather: '#ece6d6', gem: '#60a5fa', star: '#e9cf7c', crown: '#d4a72c',
      'scale-balanced': '#d4a72c', skull: '#e7e2d4', spider: '#1f2937', box: '#a16207', mask: '#c2410c', key: '#d4a72c', book: '#7f1d1d', 'book-open': '#e8dcbc', scroll: '#e8dcbc',
      hourglass: '#d4a72c', moon: '#d6dde6', broom: '#a16207', 'bread-slice': '#f59e0b', 'apple-whole': '#dc2626', bell: '#d4a72c',
      eye: '#d9ffe6', tentacle: '#8e6fc0', web: '#e5e9f2', ghost: '#eef3ff', candy: '#ff6fae', lollipop: '#ffb3d9', cupcake: '#ffcf8a', gift: '#e8364f', snowflake: '#e6f4ff',
      fish: '#5fb8d6', shell: '#ffd2c2', anchor: '#9aa4ae', butterfly: '#e08ae0', sun: '#ffc14a', jar: '#c46a3a', 'beer-mug': '#e0a63a', axe: '#9aa4ae', 'shield-halved': '#b08a3e' };
    // Only these go on one wall: the forge's hood and the hanging herbs would clash with the shop sign on the other.
    var ONLY_Y = { forge: 1, herbs: 1 };
    var DEPTH = { shelf: .55, rack: .3, cabinet: .66, bookcase: .5, forge: 1.1, window: 0, herbs: 0 };
    var HT = { shelf: 2.6, rack: 2.1, cabinet: 1.85, bookcase: 2.7, forge: 2.9, window: 2.5, herbs: 3.1, counter: 1.15, table: 1, barrel: 1, crate: .8, anvil: .7, glass: 1.15, stall: 2.4, rug: .2, pedestal: 1.1, lamp: 2.3, sack: .7,
      tentacle: 2.2, monolith: 2.6, circle: .02, cane: 1.8, gumdrop: .8, lolly: 2.4, ghost: 1.6, coffin: 2.4, gift: .75, pine: 2.8, coral: 1.4, kelp: 2.6, mushroom: 1.3, bloom: 1.6, urn: 1.2, palm: 2.6, pillar: 3.3, brazier: 1.2 };

    // Footprint on the floor, as [x, y, w, d].
    // ---- Room shape. A room is an outline on the floor; the walls along its back half are drawn and the front is cut away so you can see in. ----
    var SETTINGS = [['room', 'Square room'], ['tower', 'Round tower'], ['tree', 'Inside a tree'], ['burrow', 'Burrow'], ['cave', 'Cave']];
    var G = null;
    function geometry() {
      var N = SZ[S.size], st = S.setting, pts = [], hs = [], c = N / 2, R = N / 2 - .15;
      if (st === 'room') { pts = [[0, 0], [N, 0], [N, N], [0, N]]; hs = [WH, WH, WH, WH]; }
      else {
        var k = st === 'cave' ? 30 : 28, r = rng(S.seeds.room * 13 + N), ph = [r() * 6.3, r() * 6.3, r() * 6.3];
        for (var i = 0; i < k; i++) {
          var t = i / k * Math.PI * 2, rr = st === 'cave' ? R * (.86 + .07 * Math.sin(3 * t + ph[0]) + .04 * Math.sin(5 * t + ph[1]) + .025 * Math.sin(9 * t + ph[2])) : R;
          pts.push([c + rr * Math.cos(t), c + rr * Math.sin(t)]);
          hs.push(st === 'cave' ? WH - .05 + r() * .45 : st === 'burrow' ? WH - .35 : WH);
        }
      }
      var edges = pts.map(function (a, i) {
        var j = (i + 1) % pts.length, b = pts[j], dx = b[0] - a[0], dy = b[1] - a[1], len = Math.hypot(dx, dy), n = [dy / len, -dx / len];
        return { a: a, b: b, d: [dx / len, dy / len], len: len, n: n, m: [-n[0], -n[1]], back: n[0] + n[1] < -.05, h0: hs[i], h1: hs[j] };
      });
      G = { N: N, pts: pts, edges: edges, c: c, R: R, door: null };
      // A burrow gets a round door at the very back, kept clear of furniture.
      if (st === 'burrow') { var e = edges.filter(function (e) { return e.back; }).sort(function (x, y) { return (x.a[0] + x.b[0] + x.a[1] + x.b[1]) - (y.a[0] + y.b[0] + y.a[1] + y.b[1]); })[0]; var mx = (e.a[0] + e.b[0]) / 2, my = (e.a[1] + e.b[1]) / 2; G.door = { e: e, x: mx, y: my, r: [mx - 1.1, my - 1.1, 2.2, 2.4] }; }
    }
    function inside(x, y) {
      var p = G.pts, ins = false;
      for (var i = 0, j = p.length - 1; i < p.length; j = i++) if ((p[i][1] > y) !== (p[j][1] > y) && x < (p[j][0] - p[i][0]) * (y - p[i][1]) / (p[j][1] - p[i][1]) + p[i][0]) ins = !ins;
      return ins;
    }
    function rectIn(r, m) {
      var x0 = r[0] - m, y0 = r[1] - m, x1 = r[0] + r[2] + m, y1 = r[1] + r[3] + m, xm = (x0 + x1) / 2, ym = (y0 + y1) / 2;
      return [[x0, y0], [x1, y0], [x1, y1], [x0, y1], [xm, y0], [x1, ym], [xm, y1], [x0, ym]].every(function (q) { return inside(q[0], q[1]); });
    }
    function rect(p) {
      var o = p.off || 0;
      if (WALLK[p.kind]) return p.wall === 'Y' ? [p.x, o, p.len, DEPTH[p.kind]] : [o, p.y, DEPTH[p.kind], p.len];
      return [p.x, p.y, p.w, p.d];
    }
    // On a curved wall, slide the piece out from the back until it fits inside, so it stands against the wall.
    function hug(p) {
      if (S.setting === 'room') { p.off = 0; return true; }
      for (var o = 0; o < G.N; o += .05) { p.off = o; if (rectIn(rect(p), .03)) return backAt(p); }
      return false;
    }
    // A wall piece must stand against wall that is drawn (the back half), never against the cut-away front.
    function backAt(p) {
      var x = p.wall === 'Y' ? p.x + p.len / 2 : p.off, y = p.wall === 'Y' ? p.off : p.y + p.len / 2, best = null, bd = 1e9;
      G.edges.forEach(function (e) { var t = Math.max(0, Math.min(e.len, (x - e.a[0]) * e.d[0] + (y - e.a[1]) * e.d[1])), dx = e.a[0] + e.d[0] * t - x, dy = e.a[1] + e.d[1] * t - y, d = dx * dx + dy * dy; if (d < bd) { bd = d; best = e; } });
      return best && best.n[0] + best.n[1] < -.35;
    }
    function over(a, b, m) { return a[0] < b[0] + b[2] + m && b[0] < a[0] + a[2] + m && a[1] < b[1] + b[3] + m && b[1] < a[1] + a[3] + m; }
    function thick(p) { var r = rect(p); if (WALLK[p.kind]) { if (p.wall === 'Y') r[3] = Math.max(r[3], .3); else r[2] = Math.max(r[2], .3); } return r; }
    // Whether piece p can stand where it is, given the others. Rugs lie under things and lamps hang over them.
    function clashes(p, others, N) {
      var r = rect(p), sq = S.setting === 'room';
      if (sq) {
        if (r[0] < 0 || r[1] < 0 || r[0] + r[2] > N - .3 || r[1] + r[3] > N - .3) return true;
        if (WALLK[p.kind] && (p.wall === 'Y' ? p.x : p.y) < .9) return true;
      } else {
        if (!rectIn(r, WALLK[p.kind] ? .03 : .25)) return true;
        if (G.door && !FLAT[p.kind] && p.kind !== 'lamp' && over(thick(p), G.door.r, 0)) return true;
      }
      return others.some(function (o) {
        if (o === p || o.id === p.id) return false;
        var wa = WALLK[p.kind], wo = WALLK[o.kind];
        if (wa && wo) return sq ? p.wall === o.wall && over(p.wall === 'Y' ? [p.x, 0, p.len, 1] : [0, p.y, 1, p.len], p.wall === 'Y' ? [o.x, 0, o.len, 1] : [0, o.y, 1, o.len], .1) : over(thick(p), thick(o), .1);
        if (FLAT[p.kind] || FLAT[o.kind]) return !!FLAT[p.kind] === !!FLAT[o.kind] && over(r, rect(o), .2);
        if (p.kind === 'lamp' || o.kind === 'lamp') return p.kind === o.kind && over(r, rect(o), 1.5);
        if (wa && !DEPTH[p.kind]) return false;
        if (wo && !DEPTH[o.kind]) return false;
        return over(r, rect(o), wa || wo ? .3 : .4);
      });
    }
    var nextId = 1;
    function snap(v) { return Math.round(v * 4) / 4; }
    function tryPlace(kind, placed, N, r) {
      if (kind === 'window' && S.setting === 'cave') return null;
      var sq = S.setting === 'room', back = G.edges.filter(function (e) { return e.back; }), total = back.reduce(function (s, e) { return s + e.len; }, 0);
      for (var t = 0; t < 120; t++) {
        var p = { id: nextId, kind: kind };
        if (WALLK[kind]) {
          p.len = SIZE[kind];
          if (sq) {
            p.wall = ONLY_Y[kind] || r() < .5 ? 'Y' : 'X';
            var pos = snap(1 + r() * (N - 1.4 - p.len));
            if (p.wall === 'Y') { p.x = pos; p.y = 0; } else { p.x = 0; p.y = pos; }
            p.off = 0;
          } else {
            // Pick a spot along the back wall, face the piece the way the wall faces there, then push it back against the wall.
            var w = r() * total, e = back[0];
            for (var i = 0; i < back.length; i++) { if (w < back[i].len) { e = back[i]; break; } w -= back[i].len; }
            var px = e.a[0] + e.d[0] * w, py = e.a[1] + e.d[1] * w;
            p.wall = Math.abs(e.m[1]) >= Math.abs(e.m[0]) ? 'Y' : 'X';
            if (ONLY_Y[kind] && p.wall !== 'Y') continue;
            if (p.wall === 'Y') { p.x = snap(px - p.len / 2); p.y = 0; } else { p.y = snap(py - p.len / 2); p.x = 0; }
            if (!hug(p)) continue;
          }
        } else {
          var sz = SIZE[kind], turn = (kind === 'table' || kind === 'glass') && r() < .4;
          p.w = turn ? sz[1] : sz[0]; p.d = turn ? sz[0] : sz[1];
          // The keeper's counter goes off to one side, nearer the front, so the room stays open.
          if (sq) {
            if (kind === 'counter' && t < 60) { p.x = snap(1.1 + r() * 1.2); p.y = snap(N * .45 + r() * (N * .55 - p.d - .7)); }
            else { p.x = snap(1.1 + r() * (N - 1.6 - p.w)); p.y = snap(1.1 + r() * (N - 1.6 - p.d)); }
          } else {
            if (kind === 'counter' && t < 60) { p.x = snap(G.c - G.R * .75 + r() * G.R * .5 - p.w / 2); p.y = snap(G.c - G.R * .1 + r() * G.R * .5 - p.d / 2); }
            else { p.x = snap(.4 + r() * (N - .8 - p.w)); p.y = snap(.4 + r() * (N - .8 - p.d)); }
          }
        }
        if (!clashes(p, placed, N)) { nextId++; placed.push(p); return p; }
      }
      return null;
    }
    // Puts a piece the GM dragged in from the list as near the drop point (in
    // floor tiles) as it fits, searching outward; null when nothing near fits.
    // Wall pieces go on the nearest drawn wall. The piece counts as moved by hand.
    function place(kind, fx, fy) {
      if (!SIZE[kind] || (kind === 'window' && S.setting === 'cave')) return null;
      var N = SZ[S.size], sq = S.setting === 'room', back = G.edges.filter(function (e) { return e.back; });
      var id = S.pieces.reduce(function (m, q) { return Math.max(m, q.id + 1); }, 1);
      for (var d = 0; d <= 3; d += .5) {
        for (var k = 0; k < (d ? 8 : 1); k++) {
          var cx = fx + d * Math.cos(k * Math.PI / 4), cy = fy + d * Math.sin(k * Math.PI / 4), p = { id: id, kind: kind, pinned: true };
          if (WALLK[kind]) {
            p.len = SIZE[kind]; p.off = 0;
            if (sq) {
              p.wall = ONLY_Y[kind] || cy <= cx ? 'Y' : 'X';
              if (p.wall === 'Y') { p.x = snap(cx - p.len / 2); p.y = 0; } else { p.x = 0; p.y = snap(cy - p.len / 2); }
            } else {
              var e = null, bd = 1e9, px = 0, py = 0;
              back.forEach(function (b) { var t = Math.max(0, Math.min(b.len, (cx - b.a[0]) * b.d[0] + (cy - b.a[1]) * b.d[1])), qx = b.a[0] + b.d[0] * t, qy = b.a[1] + b.d[1] * t, dd = (qx - cx) * (qx - cx) + (qy - cy) * (qy - cy); if (dd < bd) { bd = dd; e = b; px = qx; py = qy; } });
              if (!e) return null;
              p.wall = Math.abs(e.m[1]) >= Math.abs(e.m[0]) ? 'Y' : 'X';
              if (ONLY_Y[kind] && p.wall !== 'Y') continue;
              if (p.wall === 'Y') { p.x = snap(px - p.len / 2); p.y = 0; } else { p.y = snap(py - p.len / 2); p.x = 0; }
              if (!hug(p)) continue;
            }
          } else {
            p.w = SIZE[kind][0]; p.d = SIZE[kind][1];
            p.x = snap(cx - p.w / 2); p.y = snap(cy - p.d / 2);
          }
          if (!clashes(p, S.pieces, N)) { S.pieces.push(p); return p; }
        }
      }
      return null;
    }

    // Builds the whole room. Pieces the GM moved by hand are pinned and kept, and count toward the type's must-haves.
    function generate() {
      geometry();
      var N = SZ[S.size], rec = recOf(), r = rng(S.seeds.room * 7919 + S.key * 131 + N);
      var placed = S.keep ? S.pieces.filter(function (p) { return p.pinned && (!WALLK[p.kind] || hug(p)) && !clashes(p, [], N); }) : [];
      nextId = placed.reduce(function (m, p) { return Math.max(m, p.id + 1); }, 1);
      var have = {}; placed.forEach(function (p) { have[p.kind] = (have[p.kind] || 0) + 1; });
      rec.need.concat(MOODS[S.mood] ? MOODS[S.mood].need : []).forEach(function (k) { if (have[k]) have[k]--; else tryPlace(k, placed, N, r); });
      var n = { sparse: 2, normal: 5, packed: 9 }[S.full] + (N - 8) - (S.setting === 'room' ? 0 : 1);
      var pool = shuffle(rec.opt.concat(rec.opt), r);
      for (var i = 0; i < pool.length && n > 0; i++) if (tryPlace(pool[i], placed, N, r)) n--;
      S.pieces = placed;
    }

    // Changing mood swaps the old mood's furniture for the new one's, leaving
    // everything else (and anything moved by hand) where it stands.
    function applyMood() {
      geometry();
      var N = SZ[S.size], r = rng(S.seeds.room * 5381 + S.key * 61 + MOOD_ORDER.indexOf(S.mood));
      var placed = S.pieces.filter(function (p) { return p.pinned || !MOOD_KIND[p.kind]; });
      nextId = placed.reduce(function (m, p) { return Math.max(m, p.id + 1); }, 1);
      var have = {}; placed.forEach(function (p) { have[p.kind] = (have[p.kind] || 0) + 1; });
      (MOODS[S.mood] ? MOODS[S.mood].need : []).forEach(function (k) { if (have[k]) have[k]--; else tryPlace(k, placed, N, r); });
      S.pieces = placed;
    }

    // ---- Textures. Patterns are drawn in tile units and mapped onto each plane with one matrix. ----
    var MF = 'matrix(32,16,-32,16,0,0)', MY = 'matrix(32,16,0,-32,0,0)', MX = 'matrix(-32,16,0,-32,0,0)';
    function pat(id, w, h, tf, body) { return '<pattern id="' + id + '" patternUnits="userSpaceOnUse" width="' + w + '" height="' + h + '" patternTransform="' + tf + '">' + body + '</pattern>'; }
    function R(x, y, w, h, f, ex) { return '<rect x="' + x + '" y="' + y + '" width="' + w + '" height="' + h + '" fill="' + f + '"' + (ex || '') + '/>'; }
    function L(x1, y1, x2, y2, c, w) { return '<line x1="' + x1 + '" y1="' + y1 + '" x2="' + x2 + '" y2="' + y2 + '" stroke="' + c + '" stroke-width="' + (w || .025) + '"/>'; }
    function wallPat(id, b, kind, tf) {
      var j = dk(b, .32);
      if (kind === 'stone' || kind === 'brick') {
        var bw = kind === 'brick' ? .5 : .75, bh = kind === 'brick' ? .2 : .32;
        return pat(id, bw * 2, bh * 2, tf, R(0, 0, bw * 2, bh * 2, b) + R(0, 0, bw, bh, lt(b, .05)) + R(bw, 0, bw, bh, dk(b, .05)) + R(bw / 2, bh, bw, bh, dk(b, .09)) + R(0, bh, bw / 2, bh, lt(b, .02)) + R(bw * 1.5, bh, bw / 2, bh, lt(b, .02)) +
          L(0, 0, bw * 2, 0, j, .035) + L(0, bh, bw * 2, bh, j, .035) + L(0, 0, 0, bh, j, .035) + L(bw, 0, bw, bh, j, .035) + L(bw / 2, bh, bw / 2, bh * 2, j, .035) + L(bw * 1.5, bh, bw * 1.5, bh * 2, j, .035));
      }
      if (kind === 'plaster') return pat(id, 1.3, 1.1, tf, R(0, 0, 1.3, 1.1, b) + [[.2, .3, .05], [.8, .7, .03], [1.1, .2, .04], [.5, .95, .03], [.95, .45, .02]].map(function (c) { return '<circle cx="' + c[0] + '" cy="' + c[1] + '" r="' + c[2] + '" fill="' + dk(b, .07) + '"/>'; }).join(''));
      if (kind === 'panel') return pat(id, .45, 4, tf, R(0, 0, .45, 4, b) + L(0, 0, 0, 4, dk(b, .28), .03) + L(.03, 0, .03, 4, lt(b, .12), .02) + R(.12, 0, .2, 4, dk(b, .03)));
      return pat(id, 3, .3, tf, R(0, 0, 3, .3, b) + R(1.1, .15, 1.9, .15, dk(b, .04)) + L(0, 0, 3, 0, j, .03) + L(1.1, 0, 1.1, .3, j, .03) + L(2.4, 0, 2.4, .15, j, .03));
    }
    function floorPat(id, b, kind) {
      var j = dk(b, .38);
      if (kind === 'dirt') return pat(id, 1.6, 1.6, MF, R(0, 0, 1.6, 1.6, b) + [[.3, .4, .22, .06], [1.1, .3, .3, -.05], [.8, 1.1, .26, .07], [1.4, 1.3, .14, -.06], [.2, 1.3, .12, .05]].map(function (q) { return '<ellipse cx="' + q[0] + '" cy="' + q[1] + '" rx="' + q[2] + '" ry="' + (q[2] * .7) + '" fill="' + (q[3] > 0 ? dk(b, q[3]) : lt(b, -q[3])) + '"/>'; }).join('') + '<circle cx=".6" cy=".8" r=".05" fill="' + lt(b, .2) + '"/><circle cx="1.3" cy=".7" r=".04" fill="' + dk(b, .3) + '"/>');
      if (kind === 'rings') return pat(id, 1, 1, MF, R(0, 0, 1, 1, b));
      if (kind === 'flags') return pat(id, 2, 2, MF, R(0, 0, 2, 2, j) + R(.03, .03, .94, .94, lt(b, .04)) + R(1.03, .03, .94, .44, dk(b, .05)) + R(1.03, .53, .94, .44, b) + R(.03, 1.03, .44, .94, dk(b, .08)) + R(.53, 1.03, .44, .94, lt(b, .02)) + R(1.03, 1.03, .94, .94, dk(b, .02)));
      if (kind === 'checker') return pat(id, 1, 1, MF, R(0, 0, 1, 1, lt(b, .5)) + R(0, 0, .5, .5, dk(b, .15)) + R(.5, .5, .5, .5, dk(b, .15)) + L(0, 0, 1, 0, 'rgba(0,0,0,.18)', .015) + L(0, .5, 1, .5, 'rgba(0,0,0,.18)', .015) + L(0, 0, 0, 1, 'rgba(0,0,0,.18)', .015) + L(.5, 0, .5, 1, 'rgba(0,0,0,.18)', .015));
      if (kind === 'herring') return pat(id, 1, 1, MF, R(0, 0, 1, 1, b) + R(0, 0, .25, .5, lt(b, .05)) + R(.25, .5, .25, .5, dk(b, .05)) + R(.5, 0, .5, .25, dk(b, .03)) + R(.5, .75, .5, .25, lt(b, .03)) +
        L(0, 0, 1, 0, j) + L(0, 0, 0, 1, j) + L(.25, 0, .25, 1, j) + L(.5, 0, .5, 1, j) + L(.5, .25, 1, .25, j) + L(.5, .5, 1, .5, j) + L(.5, .75, 1, .75, j) + L(0, .5, .5, .5, j));
      return pat(id, 2, .5, MF, R(0, 0, 2, .5, b) + R(0, .25, 2, .25, dk(b, .06)) + R(.7, 0, .9, .25, lt(b, .04)) + L(0, 0, 2, 0, j) + L(0, .25, 2, .25, j) + L(.7, 0, .7, .25, j) + L(1.6, .25, 1.6, .5, j));
    }
    function efx() { return S.efx || moodEfx(S.mood); }
    function glowCol() { var M = MOODS[S.mood]; return M && M.glow ? M.glow : ['#ffb35c', '#ff9a3c']; }
    function hazeCol() { var M = MOODS[S.mood]; return M && M.haze ? M.haze : '#ffd6a0'; }
    function skyCol() {
      var e = efx(), t = e.time, wet = e.weather;
      var c = t === 'night' ? ['#0f1a3a', '#28325c'] : t === 'dusk' ? ['#6a3c5c', '#f5a37a'] : t === 'dawn' ? ['#c9d8ff', '#ffd1c1'] : ['#cfe8ff', '#f6efd8'];
      return wet ? [mix(c[0], '#7a8594', .55), mix(c[1], '#a3acb8', .55)] : c;
    }
    // Rain or snow beyond a window's glass, clipped to it; the glass is a flat face, so screen-down is falling.
    function weather(id, q, n) {
      var e = efx(); if (!e.weather || S.fx !== 'full') return '';
      var xs = q.map(function (v) { return v[0]; }), ys = q.map(function (v) { return v[1]; }), x0 = Math.min.apply(0, xs), x1 = Math.max.apply(0, xs), y0 = Math.min.apply(0, ys), y1 = Math.max.apply(0, ys), s = '';
      for (var i = 0; i < n; i++) {
        var x = x0 + jit(id, i, 1) * (x1 - x0), y = y0 - 8 + jit(id, i + 11, 1) * (y1 - y0) * .3, d = '-' + jit(id, i + 5, 2).toFixed(2) + 's';
        s += e.weather === 'rain' ? '<line class="shr-rain" x1="' + x.toFixed(1) + '" y1="' + y.toFixed(1) + '" x2="' + (x - 1.5).toFixed(1) + '" y2="' + (y + 7).toFixed(1) + '" stroke="#eef5ff" stroke-width="1" stroke-linecap="round" style="animation-delay:' + d + ';--fall:' + (y1 - y0 + 10).toFixed(0) + 'px"/>'
          : '<circle class="shr-snow" cx="' + x.toFixed(1) + '" cy="' + y.toFixed(1) + '" r="1.2" fill="#fff" style="animation-delay:-' + (jit(id, i + 5, 4)).toFixed(2) + 's;--fall:' + (y1 - y0 + 10).toFixed(0) + 'px"/>';
      }
      return '<clipPath id="wc' + id + '"><polygon points="' + pts(q) + '"/></clipPath><g clip-path="url(#wc' + id + ')" style="pointer-events:none">' + s + '</g>';
    }
    function defs() {
      var rec = recOf();
      var fk = S.setting === 'room' ? rec.floor : { tower: 'flags', tree: 'rings', burrow: 'plank', cave: 'dirt' }[S.setting], fb = S.setting === 'tree' ? mix(C.floor, '#d2b07a', .55) : S.setting === 'cave' ? mix(C.floor, '#6e5a44', .5) : C.floor;
      return '<defs>' + floorPat('pFloor', fb, fk) + wallPat('pWallY', C.wall, rec.wall, MY) + wallPat('pWallX', dk(C.wall, .16), rec.wall, MX) +
        '<filter id="soft" x="-20%" y="-50%" width="140%" height="200%"><feGaussianBlur stdDeviation="3"/></filter>' +
        // Fine streaks along each face direction: wood grain on wood, tool marks on stone. Same texture everywhere keeps it cheap.
        '<pattern id="tL" patternUnits="userSpaceOnUse" width="60" height="24" patternTransform="skewY(26.565)"><line x1="0.6" y1="7.8" x2="22.900000000000002" y2="7.8" stroke="#000" stroke-opacity="0.09" stroke-width="0.7"/><line x1="25.5" y1="1.4" x2="34.3" y2="1.4" stroke="#000" stroke-opacity="0.06" stroke-width="0.6"/><line x1="47.9" y1="10.2" x2="58.599999999999994" y2="10.2" stroke="#000" stroke-opacity="0.1" stroke-width="1.1"/><line x1="17.8" y1="13.9" x2="47.3" y2="13.9" stroke="#000" stroke-opacity="0.12" stroke-width="0.7"/><line x1="-1.8" y1="3.5" x2="13.0" y2="3.5" stroke="#fff" stroke-opacity="0.06" stroke-width="0.8"/><line x1="16.1" y1="15.3" x2="36.2" y2="15.3" stroke="#000" stroke-opacity="0.05" stroke-width="0.6"/><line x1="19.9" y1="16.3" x2="34.8" y2="16.3" stroke="#000" stroke-opacity="0.09" stroke-width="0.7"/><line x1="38.9" y1="19.1" x2="52.3" y2="19.1" stroke="#000" stroke-opacity="0.09" stroke-width="1.0"/><line x1="10.2" y1="17.5" x2="39.8" y2="17.5" stroke="#000" stroke-opacity="0.08" stroke-width="1.0"/><line x1="24.2" y1="3.6" x2="33.1" y2="3.6" stroke="#fff" stroke-opacity="0.11" stroke-width="0.8"/><line x1="12.0" y1="21.0" x2="35.3" y2="21.0" stroke="#000" stroke-opacity="0.1" stroke-width="0.8"/><line x1="56.1" y1="20.2" x2="74.5" y2="20.2" stroke="#fff" stroke-opacity="0.05" stroke-width="0.9"/><line x1="59.5" y1="15.5" x2="85.6" y2="15.5" stroke="#000" stroke-opacity="0.08" stroke-width="0.9"/><line x1="22.3" y1="0.5" x2="34.0" y2="0.5" stroke="#000" stroke-opacity="0.05" stroke-width="1.0"/></pattern>' +
        '<pattern id="tR" patternUnits="userSpaceOnUse" width="60" height="24" patternTransform="skewY(-26.565)"><line x1="7.3" y1="3.1" x2="23.900000000000002" y2="3.1" stroke="#fff" stroke-opacity="0.06" stroke-width="0.8"/><line x1="51.8" y1="13.2" x2="77.8" y2="13.2" stroke="#fff" stroke-opacity="0.07" stroke-width="0.7"/><line x1="51.9" y1="8.6" x2="81.0" y2="8.6" stroke="#000" stroke-opacity="0.06" stroke-width="0.6"/><line x1="23.9" y1="5.6" x2="44.9" y2="5.6" stroke="#000" stroke-opacity="0.05" stroke-width="0.8"/><line x1="29.6" y1="8.9" x2="58.6" y2="8.9" stroke="#fff" stroke-opacity="0.09" stroke-width="0.9"/><line x1="-6.2" y1="16.2" x2="21.6" y2="16.2" stroke="#fff" stroke-opacity="0.12" stroke-width="1.0"/><line x1="17.9" y1="9.4" x2="28.2" y2="9.4" stroke="#fff" stroke-opacity="0.05" stroke-width="0.5"/><line x1="1.4" y1="5.0" x2="16.9" y2="5.0" stroke="#000" stroke-opacity="0.05" stroke-width="0.6"/><line x1="15.5" y1="2.4" x2="24.1" y2="2.4" stroke="#fff" stroke-opacity="0.1" stroke-width="0.6"/><line x1="14.3" y1="6.1" x2="30.3" y2="6.1" stroke="#000" stroke-opacity="0.12" stroke-width="1.1"/><line x1="23.9" y1="11.2" x2="33.8" y2="11.2" stroke="#000" stroke-opacity="0.08" stroke-width="0.7"/><line x1="1.3" y1="19.9" x2="9.8" y2="19.9" stroke="#fff" stroke-opacity="0.09" stroke-width="0.6"/><line x1="-8.1" y1="13.0" x2="11.500000000000002" y2="13.0" stroke="#fff" stroke-opacity="0.12" stroke-width="0.9"/><line x1="15.7" y1="6.3" x2="27.4" y2="6.3" stroke="#fff" stroke-opacity="0.09" stroke-width="1.0"/></pattern>' +
        '<linearGradient id="fL" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#fff" stop-opacity=".12"/><stop offset=".5" stop-color="#fff" stop-opacity="0"/><stop offset="1" stop-color="#000" stop-opacity=".16"/></linearGradient>' +
        '<linearGradient id="fR" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#fff" stop-opacity=".05"/><stop offset="1" stop-color="#000" stop-opacity=".22"/></linearGradient>' +
        '<linearGradient id="fT" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#fff" stop-opacity=".16"/><stop offset=".6" stop-color="#fff" stop-opacity="0"/><stop offset="1" stop-color="#000" stop-opacity=".08"/></linearGradient>' +
        '<linearGradient id="gSheen" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#fff" stop-opacity=".1"/><stop offset=".45" stop-color="#fff" stop-opacity="0"/><stop offset="1" stop-color="#000" stop-opacity=".12"/></linearGradient>' +
        '<linearGradient id="gShaft" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#fff6dc" stop-opacity=".2"/><stop offset="1" stop-color="#fff6dc" stop-opacity=".02"/></linearGradient>' +
        '<filter id="ao" x="-10%" y="-10%" width="120%" height="120%"><feGaussianBlur stdDeviation="6"/></filter>' +
        '<radialGradient id="gHole"><stop offset="0" stop-color="#000" stop-opacity="1"/><stop offset=".55" stop-color="#000" stop-opacity=".55"/><stop offset="1" stop-color="#000" stop-opacity="0"/></radialGradient>' +
        '<radialGradient id="gCool"><stop offset="0" stop-color="#eaf4ff" stop-opacity=".28"/><stop offset="1" stop-color="#eaf4ff" stop-opacity="0"/></radialGradient>' +
        '<radialGradient id="gFire" cx="50%" cy="80%" r="70%"><stop offset="0" stop-color="#fff3b0"/><stop offset=".35" stop-color="#ffb347"/><stop offset=".75" stop-color="#e2541b"/><stop offset="1" stop-color="#5c1a08"/></radialGradient>' +
        '<linearGradient id="gSky" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="' + skyCol()[0] + '"/><stop offset="1" stop-color="' + skyCol()[1] + '"/></linearGradient>' +
        '<radialGradient id="gLamp"><stop offset="0" stop-color="#fff7d1"/><stop offset="1" stop-color="#f0a83a"/></radialGradient>' +
        '<linearGradient id="gBarrel" x1="0" x2="1"><stop offset="0" stop-color="#5a3c22"/><stop offset=".35" stop-color="#9a6b3d"/><stop offset=".55" stop-color="#8a5d33"/><stop offset="1" stop-color="#4a3019"/></linearGradient>' +
        '<radialGradient id="gSack" cx="35%" cy="35%" r="75%"><stop offset="0" stop-color="#e3cfa4"/><stop offset="1" stop-color="#9c7d4f"/></radialGradient>' +
        '<radialGradient id="gGlow"><stop offset="0" stop-color="' + glowCol()[0] + '" stop-opacity=".5"/><stop offset=".5" stop-color="' + glowCol()[1] + '" stop-opacity=".2"/><stop offset="1" stop-color="' + glowCol()[1] + '" stop-opacity="0"/></radialGradient>' +
        '<linearGradient id="gHaze" x1="0" y1="1" x2="0" y2="0"><stop offset="0" stop-color="' + hazeCol() + '" stop-opacity=".55"/><stop offset=".7" stop-color="' + hazeCol() + '" stop-opacity="0"/></linearGradient>' +
        '<radialGradient id="gVig2" cx="50%" cy="50%" r="72%"><stop offset=".55" stop-color="#000" stop-opacity="0"/><stop offset="1" stop-color="#000" stop-opacity=".5"/></radialGradient>' +
        '<radialGradient id="gMagic"><stop offset="0" stop-color="#c4b5fd" stop-opacity=".75"/><stop offset="1" stop-color="#8b5cf6" stop-opacity="0"/></radialGradient>' +
        '<radialGradient id="gVig" cx="50%" cy="45%" r="70%"><stop offset=".6" stop-color="#000" stop-opacity="0"/><stop offset="1" stop-color="#000" stop-opacity=".28"/></radialGradient></defs>';
    }

    // ---- The room ----
    function shell(N) {
      var s = '', rec = recOf();
      s += box(0, 0, -.35, N, N, .35, dk(C.floor, .25), { noT: true });
      s += poly([P(0, 0), P(N, 0), P(N, N), P(0, N)], 'url(#pFloor)');
      // Floor darkens toward the back corner where little light reaches.
      s += poly([P(0, 0), P(N, 0), P(N, N), P(0, N)], 'url(#gVig)', ' opacity=".6"');
      s += poly([P(0, 0), P(N, 0), P(N, N), P(0, N)], 'url(#gSheen)');
      s += box(-.3, 0, 0, .3, N, WH, C.wall, { noT: true, r: 'url(#pWallX)', l: dk(C.wall, .3) });
      s += box(0, -.3, 0, N, .3, WH, C.wall, { noT: true, l: 'url(#pWallY)', r: dk(C.wall, .38) });
      // Plaster gets timber framing, panelling a dado rail; the trim colour comes from the palette.
      if (rec.wall === 'plaster') {
        for (var u = 0; u <= N; u += 2) { s += poly([P(u, .01, 0), P(u + .14, .01, 0), P(u + .14, .01, WH), P(u, .01, WH)], C.wood); s += poly([P(.01, u, 0), P(.01, u + .14, 0), P(.01, u + .14, WH), P(.01, u, WH)], dk(C.wood, .2)); }
        s += poly([P(0, .01, 2.2), P(N, .01, 2.2), P(N, .01, 2.34), P(0, .01, 2.34)], C.wood) + poly([P(.01, 0, 2.2), P(.01, N, 2.2), P(.01, N, 2.34), P(.01, 0, 2.34)], dk(C.wood, .2));
      }
      if (rec.wall === 'panel') s += poly([P(0, .01, .95), P(N, .01, .95), P(N, .01, 1.05), P(0, .01, 1.05)], C.trim) + poly([P(.01, 0, .95), P(.01, N, .95), P(.01, N, 1.05), P(.01, 0, 1.05)], dk(C.trim, .2));
      s += box(0, 0, 0, N, .06, .2, C.trim, { edge: false }) + box(0, .06, 0, .06, N - .06, .2, C.trim, { edge: false });
      s += box(-.3, -.3, WH, N + .3, .3, .12, C.trim) + box(-.3, 0, WH, .3, N, .12, C.trim);
      return s;
    }
    // The shop's sign hangs on the left-hand wall: flat on it in a square room, on chains just in front of a curved one.
    function sign(N) {
      var x0 = 0, ym = N / 2;
      if (S.setting !== 'room') { var e = G.edges.filter(function (e) { return e.back; }).sort(function (a, b) { return (b.m[0] - Math.abs(b.m[1])) - (a.m[0] - Math.abs(a.m[1])); })[0]; x0 = (e.a[0] + e.b[0]) / 2 + .45; ym = (e.a[1] + e.b[1]) / 2; }
      var y0 = ym - 1.5, s = box(x0, y0, 2.66, .07, 3, .5, dk(C.wood, .35), { t: dk(C.wood, .2) });
      s += line(P(x0 + .04, y0 + .3, 3.16), P(x0 + .04, y0 + .3, WH), '#2a2a2a', 1) + line(P(x0 + .04, y0 + 2.7, 3.16), P(x0 + .04, y0 + 2.7, WH), '#2a2a2a', 1);
      var c = P(x0 + .07, ym, 2.91);
      return s + '<text transform="translate(' + c[0].toFixed(1) + ',' + c[1].toFixed(1) + ') skewY(-26.565)" text-anchor="middle" dominant-baseline="middle" font-family="Georgia,serif" font-weight="700" font-size="14" fill="#e9cf7c" style="pointer-events:none">' + esc(S.name) + '</text>';
    }
    // Shaped rooms: floor, curved back walls with their own surface, and the burrow's round door.
    function wallBase() { return { tower: mix(C.wall, '#8f8a80', .35), tree: mix(C.wood, '#3b2a1c', .45), burrow: C.wall, cave: mix(C.wall, '#5f5a55', .62) }[S.setting]; }
    function capCol() { return { tower: C.trim, tree: dk(mix(C.wood, '#3b2a1c', .45), .3), burrow: '#5f8a3a', cave: dk(mix(C.wall, '#5f5a55', .62), .25) }[S.setting]; }
    function at3(e, u, z, o) { return P(e.a[0] + e.d[0] * u + e.m[0] * (o || 0), e.a[1] + e.d[1] * u + e.m[1] * (o || 0), z); }
    function byDepth(list) { return list.slice().sort(function (x, y) { return (x.a[0] + x.b[0] + x.a[1] + x.b[1]) - (y.a[0] + y.b[0] + y.a[1] + y.b[1]); }); }
    function shellShape(N) {
      var s = '', st = S.setting, base = wallBase(), cap = capCol(), r = rng(S.seeds.room * 31 + 5), fp = G.pts.map(function (q) { return P(q[0], q[1], 0); });
      s += poly(fp, 'url(#pFloor)');
      if (st === 'tree') {
        var cc = P(G.c, G.c, 0), rc = mix(C.floor, '#d2b07a', .55);
        s += '<clipPath id="fc"><polygon points="' + pts(fp) + '"/></clipPath><g clip-path="url(#fc)">';
        for (var k = Math.ceil(G.R / .32); k > 0; k--) s += '<ellipse cx="' + cc[0] + '" cy="' + cc[1] + '" rx="' + (k * .32 * 45.25).toFixed(1) + '" ry="' + (k * .32 * 22.63).toFixed(1) + '" fill="' + (k % 2 ? lt(rc, .04) : dk(rc, .05)) + '" stroke="' + dk(rc, .28) + '" stroke-width="' + (k % 3 ? .6 : 1.2) + '"/>';
        s += '<ellipse cx="' + cc[0] + '" cy="' + cc[1] + '" rx="5" ry="2.5" fill="' + dk(rc, .4) + '"/>';
        for (var q = 0; q < 6; q++) { var t = q * 1.05 + .3; s += line(P(G.c + .3 * Math.cos(t), G.c + .3 * Math.sin(t), 0), P(G.c + G.R * .8 * Math.cos(t), G.c + G.R * .8 * Math.sin(t), 0), dk(rc, .3), .7); }
        s += '</g>';
      }
      s += poly(fp, 'url(#gSheen)');
      byDepth(G.edges.filter(function (e) { return e.back; })).forEach(function (e, i) {
        var t = .08 * (1 + e.m[0] - e.m[1]) + (st === 'cave' ? (r() - .5) * .1 : 0), col = dk(base, t), q = [P(e.a[0], e.a[1], 0), P(e.b[0], e.b[1], 0), P(e.b[0], e.b[1], e.h1), P(e.a[0], e.a[1], e.h0)];
        s += poly(q, col, ' stroke="' + col + '" stroke-width="1.3"');
        if (st === 'tower') for (var row = 0, z = .33; z < WH; z += .33, row++) { s += line(at3(e, 0, z), at3(e, e.len, z), dk(col, .3), .7); var f = row % 2 ? .3 : .78; s += line(at3(e, e.len * f, z - .33), at3(e, e.len * f, z), dk(col, .3), .7); }
        if (st === 'tree') [.25 + r() * .15, .65 + r() * .15].forEach(function (f) { s += line(at3(e, e.len * f, 0), at3(e, e.len * (f + (r() - .5) * .1), e.h0), dk(col, .4), 1.6) + line(at3(e, e.len * f + .05, 0), at3(e, e.len * f + .05, e.h0), lt(col, .08), .7); });
        if (st === 'burrow' && i % 4 === 0) s += poly([at3(e, 0, 0, .01), at3(e, .16, 0, .01), at3(e, .16, e.h0, .01), at3(e, 0, e.h0, .01)], dk(C.wood, t));
        if (st === 'cave') for (var k2 = 0; k2 < 2; k2++) { var hm = Math.min(e.h0, e.h1), f1 = r(), z1 = r() * (hm - .7); s += line(at3(e, e.len * f1, z1), at3(e, e.len * Math.min(1, f1 + .25), z1 + .3 + r() * .3), dk(col, .3), .8); }
        s += poly([P(e.a[0], e.a[1], e.h0), P(e.b[0], e.b[1], e.h1), P(e.b[0] + e.n[0] * .3, e.b[1] + e.n[1] * .3, e.h1), P(e.a[0] + e.n[0] * .3, e.a[1] + e.n[1] * .3, e.h0)], cap, ' stroke="' + cap + '" stroke-width="1.3"');
      });
      if (st === 'burrow') s += burrowBand();
      if (G.door) {
        var e = G.door.e, u0 = e.len / 2, ring = [], ring2 = [];
        for (var a = 0; a < 24; a++) { var an = a / 24 * Math.PI * 2; ring.push(at3(e, u0 + Math.cos(an) * 1.0, 1.05 + Math.sin(an) * 1.0, .03)); ring2.push(at3(e, u0 + Math.cos(an) * .82, 1.05 + Math.sin(an) * .82, .05)); }
        s += poly(ring, dk(C.wood, .2)) + poly(ring2, '#2f6b3a', ' stroke="#1f4a27" stroke-width="1"');
        for (var pl = -.6; pl <= .6; pl += .3) s += line(at3(e, u0 + pl, .23 + (1 - Math.sqrt(1 - Math.pow(pl / .82, 2))) * .82, .05), at3(e, u0 + pl, 1.87 - (1 - Math.sqrt(1 - Math.pow(pl / .82, 2))) * .82, .05), '#24532d', .8);
        var kn = at3(e, u0, 1.05, .06); s += '<circle cx="' + kn[0].toFixed(1) + '" cy="' + kn[1].toFixed(1) + '" r="3.2" fill="#e0b443" stroke="#7a5a12" stroke-width=".8"/>';
      }
      return s;
    }
    // The burrow's walls curve into a ceiling, suggested by a band of roof timbers along the top.
    function burrowBand() {
      var s = '';
      byDepth(G.edges.filter(function (e) { return e.back; })).forEach(function (e) { s += poly([at3(e, 0, e.h0 - .28, .01), at3(e, e.len, e.h1 - .28, .01), at3(e, e.len, e.h1, .01), at3(e, 0, e.h0, .01)], dk(C.wood, .1 + .08 * (1 + e.m[0] - e.m[1]))); });
      return s;
    }
    // A low lip along the cut-away front, so a round room reads as round.
    function lip() {
      var s = '', base = wallBase(), cap = capCol();
      byDepth(G.edges.filter(function (e) { return !e.back; })).forEach(function (e) {
        var a2 = [e.a[0] + e.n[0] * .3, e.a[1] + e.n[1] * .3], b2 = [e.b[0] + e.n[0] * .3, e.b[1] + e.n[1] * .3], t = .27 + .09 * (e.n[0] - e.n[1]);
        s += poly([P(a2[0], a2[1], -.35), P(b2[0], b2[1], -.35), P(b2[0], b2[1], .3), P(a2[0], a2[1], .3)], dk(base, t), ' stroke="' + dk(base, t) + '" stroke-width="1.3"');
        s += poly([P(e.a[0], e.a[1], .3), P(e.b[0], e.b[1], .3), P(b2[0], b2[1], .3), P(a2[0], a2[1], .3)], cap, ' stroke="' + cap + '" stroke-width="1.3"');
      });
      return s;
    }
    function icon(name, x, y, sz, fill, o) {
      o = o || {};
      // Items get a lit, slightly glossy gradient in their own colour instead of a flat fill.
      if (S.fx === 'full' && /^#[0-9a-f]{6}$/i.test(fill)) { var gid = 'gm' + fill.slice(1); GRAD[gid] = fill; fill = 'url(#' + gid + ')'; }
      if (!ICONS[name]) name = 'box';
      USED[name] = 1;
      var g = ICONS[name], w = sz * g[0] / 512, tf = o.rot ? ' transform="rotate(' + o.rot + ' ' + x.toFixed(1) + ' ' + (y - sz / 2).toFixed(1) + ')"' : '';
      return '<use href="#i-' + name + '" x="' + (x - w / 2).toFixed(1) + '" y="' + (y - sz).toFixed(1) + '" width="' + w.toFixed(1) + '" height="' + sz + '" fill="' + fill + '" stroke="rgba(0,0,0,.55)" stroke-width="22" paint-order="stroke"' + tf + (o.op ? ' opacity="' + o.op + '"' : '') + '/>';
    }
    function itemSvg(a, i) {
      var it = S.its[i], pt = P(a.x, a.y, a.z), sz = 21 * a.s, s = '<g class="it item' + (it.out ? ' out' : '') + '" data-i="' + i + '" tabindex="0" role="button" aria-label="' + esc(it.n + ', ' + it.price + (it.out ? ', sold out' : '')) + '">';
      s += '<ellipse cx="' + pt[0].toFixed(1) + '" cy="' + pt[1].toFixed(1) + '" rx="' + (sz * .38).toFixed(1) + '" ry="' + (sz * .14).toFixed(1) + '" fill="rgba(0,0,0,.32)"/>';
      if (it.magic) s += '<circle cx="' + pt[0].toFixed(1) + '" cy="' + (pt[1] - sz / 2).toFixed(1) + '" r="' + (sz * .85).toFixed(1) + '" fill="url(#gMagic)"/>';
      s += icon(it.ic, pt[0], pt[1], sz, it.c, { rot: a.rot });
      return s + '<circle class="ring" cx="' + pt[0].toFixed(1) + '" cy="' + (pt[1] - sz / 2).toFixed(1) + '" r="' + (sz * .62).toFixed(1) + '" fill="transparent" stroke="none"/></g>';
    }
    function decoSvg(a, name) {
      var pt = P(a.x, a.y, a.z), sz = 17 * a.s;
      if (a.hang) return icon(name, pt[0], pt[1] + sz, sz, DECOCOL[name] || '#a3a3a3', { rot: 180, op: .95 });
      return '<ellipse cx="' + pt[0].toFixed(1) + '" cy="' + pt[1].toFixed(1) + '" rx="' + (sz * .35).toFixed(1) + '" ry="' + (sz * .12).toFixed(1) + '" fill="rgba(0,0,0,.25)"/>' + icon(name, pt[0], pt[1], sz, DECOCOL[name] || '#a3a3a3', { op: .92 });
    }
    // Bare stretches of wall get framed pictures and candle sconces, more of them as decorations go up.
    function wallDecor(N, rec, r, level, light) {
      var s = '', sq = S.setting === 'room', walls = S.pieces.filter(function (p) { return WALLK[p.kind]; }).map(thick);
      G.edges.filter(function (e) { return e.back; }).forEach(function (e) {
        if (e.len < .8) return;
        for (var u = sq ? 1.1 : (e.len - .8) / 2; u + .8 <= e.len - (sq ? .4 : 0); u += 1.3) {
          var fx = e.a[0] + e.d[0] * (u + .4) + e.m[0] * .35, fy = e.a[1] + e.d[1] * (u + .4) + e.m[1] * .35;
          if (walls.some(function (w) { return over([fx, fy, 0, 0], w, .55); }) || (G.door && over([fx, fy, 0, 0], G.door.r, .3))) continue;
          if (r() > level * (sq ? 1.1 : .5)) continue;
          var at = function (du, z) { return at3(e, u + du, z, .02); }, sd = [(e.d[0] - e.d[1]) * 32, (e.d[0] + e.d[1]) * 16];
          if (r() < .6) {
            var fr = dk(C.wood, .25), c = at(.4, 1.75);
            s += poly([at(0, 1.3), at(.8, 1.3), at(.8, 2.2), at(0, 2.2)], fr) + poly([at(.07, 1.37), at(.73, 1.37), at(.73, 2.13), at(.07, 2.13)], mix(C.wall, '#2a2320', .55));
            if (Math.abs(sd[0]) > 8) {
              var nm = rec.deco[Math.floor(r() * rec.deco.length)]; if (!ICONS[nm]) continue; USED[nm] = 1; var g = ICONS[nm], h = 17, w = h * g[0] / 512, sl = sd[1] / sd[0];
              s += '<g transform="matrix(1,' + sl.toFixed(3) + ',0,1,' + c[0].toFixed(1) + ',' + c[1].toFixed(1) + ')"><use href="#i-' + nm + '" x="' + (-w / 2).toFixed(1) + '" y="' + (-h / 2) + '" width="' + w.toFixed(1) + '" height="' + h + '" fill="' + (DECOCOL[nm] || '#ccc') + '" opacity=".85"/></g>';
            }
          } else {
            var bx = e.a[0] + e.d[0] * (u + .4) + e.m[0] * .1, by = e.a[1] + e.d[1] * (u + .4) + e.m[1] * .1, b0 = P(bx, by, 1.58), fl = P(bx, by, 2.02);
            s += box(bx - .08, by - .08, 1.5, .16, .16, .08, '#3b3b3b');
            s += '<rect x="' + (b0[0] - 3).toFixed(1) + '" y="' + (b0[1] - 15).toFixed(1) + '" width="6" height="11" rx="1.5" fill="#f3ead2" stroke="rgba(0,0,0,.3)" stroke-width=".6"/>';
            s += '<ellipse cx="' + fl[0].toFixed(1) + '" cy="' + (fl[1] + 1).toFixed(1) + '" rx="2.6" ry="4.5" fill="#ffcf6b"/>';
            light({ x: fl[0], y: fl[1], r: 46 });
          }
        }
      });
      return s;
    }
    // Soft shading that makes the room sit together: darker seams where walls meet the floor, contact shadows under
    // furniture, and short cast shadows. It is one blurred layer, drawn once, so it costs little.
    function hull(ps) {
      ps = ps.slice().sort(function (a, b) { return a[0] - b[0] || a[1] - b[1]; });
      var cr = function (o, a, b) { return (a[0] - o[0]) * (b[1] - o[1]) - (a[1] - o[1]) * (b[0] - o[0]); }, lo = [], up = [];
      ps.forEach(function (p) { while (lo.length > 1 && cr(lo[lo.length - 2], lo[lo.length - 1], p) <= 0) lo.pop(); lo.push(p); });
      ps.slice().reverse().forEach(function (p) { while (up.length > 1 && cr(up[up.length - 2], up[up.length - 1], p) <= 0) up.pop(); up.push(p); });
      return lo.slice(0, -1).concat(up.slice(0, -1));
    }
    function shading(full) {
      var ao = '', cast = '', warm = S.pieces.filter(function (p) { return p.warm; }).map(function (p) { return p.warm; });
      G.edges.filter(function (e) { return e.back; }).forEach(function (e) { ao += line(P(e.a[0], e.a[1], 0), P(e.b[0], e.b[1], 0), 'rgba(0,0,0,.5)', 14); });
      if (S.setting === 'room') ao += line(P(0, 0, 0), P(0, 0, WH), 'rgba(0,0,0,.4)', 12);
      S.pieces.forEach(function (p) {
        if (FLAT[p.kind] || p.kind === 'lamp' || p.kind === 'window' || p.kind === 'herbs') return;
        var r = rect(p), h = HT[p.kind], m = .06;
        ao += poly([P(r[0] - m, r[1] - m), P(r[0] + r[2] + m, r[1] - m), P(r[0] + r[2] + m, r[1] + r[3] + m), P(r[0] - m, r[1] + r[3] + m)], 'rgba(0,0,0,.42)');
        if (WALLK[p.kind]) return;
        var off = castOffset(warm, r[0] + r[2] / 2, r[1] + r[3] / 2, h), dx = off[0], dy = off[1], cs = [[r[0], r[1]], [r[0] + r[2], r[1]], [r[0] + r[2], r[1] + r[3]], [r[0], r[1] + r[3]]];
        cast += poly(hull(cs.concat(cs.map(function (q) { return [q[0] + dx, q[1] + dy]; }))).map(function (q) { return P(q[0], q[1], 0); }), 'rgba(0,0,0,.22)');
      });
      if (!full) return '<g style="pointer-events:none" opacity=".6">' + cast + '</g>';
      return '<g style="pointer-events:none"><g filter="url(#ao)">' + ao + '</g><g filter="url(#soft)">' + cast + '</g></g>';
    }
    // Darken the whole room a little, then let each light source punch a soft hole in that darkness.
    function lighting(lights) {
      var N = G.N, c = P(G.c, G.c, .8), sil = '';
      if (S.setting === 'room') sil = '<polygon points="' + pts([P(-.3, N, WH + .12), P(-.3, -.3, WH + .12), P(N, -.3, WH + .12), P(N, -.3, 0), P(N, 0, -.35), P(N, N, -.35), P(0, N, -.35), P(-.3, N, 0)]) + '"/>';
      else { sil = '<polygon points="' + pts(G.pts.map(function (q) { return P(q[0], q[1], 0); })) + '"/>'; G.edges.forEach(function (e) { var a2 = [e.a[0] + e.n[0] * .3, e.a[1] + e.n[1] * .3], b2 = [e.b[0] + e.n[0] * .3, e.b[1] + e.n[1] * .3]; sil += '<polygon points="' + pts(e.back ? [P(e.a[0], e.a[1], 0), P(e.b[0], e.b[1], 0), P(b2[0], b2[1], e.h1), P(a2[0], a2[1], e.h0), P(e.a[0], e.a[1], e.h0)] : [P(e.a[0], e.a[1], .3), P(e.b[0], e.b[1], .3), P(b2[0], b2[1], -.36), P(a2[0], a2[1], -.36)]) + '"/>'; }); }
      var holes = '<ellipse cx="' + c[0] + '" cy="' + c[1] + '" rx="' + N * 34 + '" ry="' + N * 22 + '" fill="url(#gHole)" opacity=".8"/>' + lights.map(function (l) { return '<circle cx="' + l.x.toFixed(1) + '" cy="' + l.y.toFixed(1) + '" r="' + (l.r * 2.1).toFixed(0) + '" fill="url(#gHole)"/>'; }).join('');
      return '<clipPath id="room">' + sil + '</clipPath><mask id="lm" maskUnits="userSpaceOnUse" x="' + VB[0] + '" y="' + VB[1] + '" width="' + VB[2] + '" height="' + VB[3] + '"><rect x="' + VB[0] + '" y="' + VB[1] + '" width="' + VB[2] + '" height="' + VB[3] + '" fill="#fff"/>' + holes + '</mask>' +
        '<rect x="' + VB[0] + '" y="' + VB[1] + '" width="' + VB[2] + '" height="' + VB[3] + '" fill="' + (C.dark ? '#04040c' : '#121027') + '" opacity="' + darkness().toFixed(3) + '" mask="url(#lm)" clip-path="url(#room)" style="pointer-events:none"/>';
    }
    // How dark the unlit room sits: the setting's own dimness, the mood's, the time of day and the owner's Shadows level.
    function darkness() {
      var e = efx(), M = MOODS[S.mood] || {}, t = { night: 1.4, dusk: 1.12, dawn: 1.05 }[e.time] || 1;
      return Math.min(.9, (S.setting === 'cave' ? .64 : .52) * (M.dark || 1) * t * e.shadows / 100);
    }
    // The owner's Window light, dimmed by the hour: a night window lets in almost nothing.
    function winK() { var e = efx(); return e.window / 100 * ({ night: .2, dusk: .6, dawn: .8 }[e.time] || 1); }
    // Overlays over the finished room: haze rising off the floor, the hour's tint, dark corners.
    function atmosphere(N) {
      var e = efx(), t = e.time, s = '<g style="pointer-events:none" clip-path="url(#room)">';
      s += '<rect x="' + VB[0] + '" y="' + VB[1] + '" width="' + VB[2] + '" height="' + VB[3] + '" fill="url(#gHaze)" opacity="' + Math.min(1, .22 * e.haze / 100).toFixed(3) + '"/>';
      if (t) s += '<rect x="' + VB[0] + '" y="' + VB[1] + '" width="' + VB[2] + '" height="' + VB[3] + '" fill="' + { night: '#0a1030', dusk: '#ff7a3c', dawn: '#ffc6b0' }[t] + '" opacity="' + { night: .32, dusk: .1, dawn: .08 }[t] + '"/>';
      s += '</g><rect x="' + VB[0] + '" y="' + VB[1] + '" width="' + VB[2] + '" height="' + VB[3] + '" fill="url(#gVig2)" opacity="' + Math.min(1, .5 * e.vignette / 100).toFixed(3) + '" style="pointer-events:none"/>';
      return s;
    }
    var VB, keeperPt, GRAD = {}, USED = {};
    // Every icon the room draws is a <use> of a symbol; this defines the ones
    // this drawing used, so goods and decorations have shapes to point at.
    function iconDefs() { return '<defs>' + Object.keys(USED).map(function (n) { var g = ICONS[n]; return '<symbol id="i-' + n + '" viewBox="0 0 ' + g[0] + ' 512"><path d="' + g[1] + '"/></symbol>'; }).join('') + '</defs>'; }
    function gradDefs() { return '<defs>' + Object.keys(GRAD).map(function (id) { var c = GRAD[id]; return '<linearGradient id="' + id + '" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="' + lt(c, .55) + '"/><stop offset=".4" stop-color="' + lt(c, .1) + '"/><stop offset=".6" stop-color="' + c + '"/><stop offset="1" stop-color="' + dk(c, .42) + '"/></linearGradient>'; }).join('') + '</defs>'; }
    function draw() {
      colours();
      var N = SZ[S.size], rec = recOf(), full = S.fx === 'full', sq = S.setting === 'room';
      VB = [-N * 32 - 30, -WH * 32 - 34, N * 64 + 60, N * 32 + WH * 32 + 34 + 26];
      GRAD = {}; USED = {};
      var level = { none: 0, some: .35, lots: .8 }[S.deco];
      // Draw each piece once to learn its anchor spots, then hand goods and decorations out to them.
      var out = S.pieces.slice().sort(function (a, b) { return a.id - b.id; }).map(function (p) { delete p.light; delete p.floorLight; delete p.keeperAt; delete p.cool; delete p.shaft; delete p.warm; delete p.life; return { p: p, d: DRAW[p.kind](p), goods: [], decos: [] }; });
      // Decorations the GM placed take their spots first; goods and the generated ones share the rest.
      var spots = [], hangs = [], mine = {}, anchors = [];
      (S.decor || []).forEach(function (d) { mine[d.piece + ':' + d.spot] = d.icon; });
      out.forEach(function (o, oi) {
        o.d.a.forEach(function (a, ai) {
          anchors.push({ piece: o.p.id, spot: ai, pt: P(a.x, a.y, a.z), at: [a.x, a.y, a.z] });
          var ic = mine[o.p.id + ':' + ai], held = !!(ic && ICONS[ic]);
          if (held) o.decos.push([ai, ic, 1]);
          (a.deco ? hangs : o.d.decoOnly ? hangs : spots).push([oi, ai, held]);
        });
      });
      // Spots are shuffled with the GM's decorations still in them, so placing
      // or moving a decoration never sends the goods to different shelves.
      var rg = rng(S.seeds.goods * 4049 + S.key * 17), rd = rng(S.seeds.deco * 6151 + S.key * 29), order = shuffle(spots, rg).filter(function (sp) { return !sp[2]; });
      order.forEach(function (sp, k) { if (k < S.its.length) out[sp[0]].goods.push([sp[1], k]); });
      order.slice(S.its.length).concat(hangs.filter(function (sp) { return !sp[2]; })).forEach(function (sp) { var a = out[sp[0]].d.a[sp[1]]; if (rd() < (a.hang ? level * 1.4 : level)) out[sp[0]].decos.push([sp[1], rec.deco[Math.floor(rd() * rec.deco.length)]]); });
      // Back-to-front by footprint centre; rugs lie under everything and lamps hang over it.
      function key(o) { var p = o.p, r = rect(p); return FLAT[p.kind] ? -99 : p.kind === 'lamp' ? 99 + r[0] + r[1] : r[0] + r[2] / 2 + r[1] + r[3] / 2; }
      out.sort(function (a, b) { return key(a) - key(b); });
      var lights = [], s = defs() + (sq ? shell(N) : shellShape(N)) + sign(N) + wallDecor(N, rec, rng(S.seeds.deco * 3301 + S.key * 7), level, function (l) { lights.push(l); });
      function piece(o) {
        var p = o.p, g = '<g class="piece' + (p.pinned ? ' pinned' : '') + '" data-p="' + p.id + '"><g class="body">' + o.d.pre + '</g>';
        o.decos.forEach(function (d) { g += '<g class="udeco" data-up="' + p.id + '" data-ua="' + d[0] + '">' + decoSvg(o.d.a[d[0]], d[1]) + '</g>'; });
        o.goods.forEach(function (gd) { g += itemSvg(o.d.a[gd[0]], gd[1]); });
        g += '<g class="body">' + o.d.post + '</g>';
        if (p.pinned && S.mode === 'arr') { var r = rect(p), q = P(r[0] + r[2] / 2, r[1] + r[3] / 2, HT[p.kind] + .25); g += '<g class="pinbtn" data-unpin="' + p.id + '" style="cursor:pointer"><title>Moved by hand. Click to unpin.</title><line x1="' + q[0] + '" y1="' + q[1] + '" x2="' + q[0] + '" y2="' + (q[1] + 10) + '" stroke="#333" stroke-width="2"/><circle cx="' + q[0] + '" cy="' + q[1] + '" r="6" fill="' + C.acc + '" stroke="#fff" stroke-width="1.5"/></g>'; }
        if (p.light) lights.push({ x: p.light[0], y: p.light[1], r: p.lightR || (p.kind === 'forge' ? 150 : 110), cool: p.cool });
        if (p.keeperAt && !keeperPt) keeperPt = p.keeperAt;
        return g + '</g>';
      }
      keeperPt = null;
      // Rugs and daylight on the floor go under the shadow layer; everything else stands on top of it.
      out.filter(function (o) { return FLAT[o.p.kind]; }).forEach(function (o) { s += piece(o); });
      s += '<g opacity="' + Math.min(1, winK()).toFixed(3) + '">'; out.forEach(function (o) { if (o.p.floorLight) s += o.p.floorLight; }); s += '</g>';
      s += shading(full);
      out.filter(function (o) { return !FLAT[o.p.kind]; }).forEach(function (o) { s += piece(o); });
      // Lamp glow and window light each follow the owner's level; past 100% a glow reaches further rather than burning white.
      var wk = efx().warmth / 100, ck = winK();
      s += gradDefs() + iconDefs() + '<g class="shr-glows" style="pointer-events:none"><g opacity="' + Math.min(1, ck).toFixed(3) + '">' + out.map(function (o) { return o.p.shaft && full ? o.p.shaft : ''; }).join('') + '</g>' + lights.map(function (l, i) { var k = l.cool ? ck : wk; return '<circle' + (l.cool ? '' : ' class="shr-fl" style="animation-delay:-' + (i * .37 % 2).toFixed(2) + 's;mix-blend-mode:screen"') + ' cx="' + l.x.toFixed(1) + '" cy="' + l.y.toFixed(1) + '" r="' + (l.r * Math.max(1, Math.sqrt(k))).toFixed(0) + '" opacity="' + Math.min(1, k).toFixed(3) + '" fill="url(#' + (l.cool ? 'gCool' : 'gGlow') + ')"' + (l.cool ? ' style="mix-blend-mode:screen"' : '') + '/>'; }).join('') + '</g>';
      // Small living details: flames, embers, dust in the window light, sparks off the anvil.
      if (full) s += '<g class="shr-life" style="pointer-events:none">' + out.map(function (o) { return o.p.life || ''; }).join('') + '</g>';
      if (!sq) s += '<g style="pointer-events:none">' + lip() + '</g>';
      if (full) s += lighting(lights) + atmosphere(N);
      // A mood with its own drifting motes (fireflies, bubbles, spores) fills the room with them, not just the window light.
      if (full && MOODS[S.mood] && MOODS[S.mood].pal) s += '<g class="shr-life" style="pointer-events:none">' + motes(P(1, 1, 2.6), P(N - 1, N - 1, .3), 16, 4242) + motes(P(N - 1, 1.5, 2.2), P(1.5, N - 1, .4), 12, 4343) + '</g>';
      // What sits where, in floor units, for the paper look to stand on its own pieces.
      var placed = [];
      out.forEach(function (o) {
        o.goods.forEach(function (gd) { var a = o.d.a[gd[0]]; placed.push({ piece: o.p.id, spot: gd[0], at: [a.x, a.y, a.z], good: gd[1] }); });
        o.decos.forEach(function (d) { var a = o.d.a[d[0]]; placed.push({ piece: o.p.id, spot: d[0], at: [a.x, a.y, a.z], icon: d[1], mine: !!d[2], hang: !!a.hang }); });
      });
      var e = efx(), cls = (e.dust ? '' : ' no-dust') + (e.flicker ? '' : ' no-flicker') + (e.embers ? '' : ' no-embers');
      return { svg: s, viewBox: VB, keeperAt: keeperPt, anchors: anchors, placed: placed, room: roomModel(out, rec), cls: cls, mood: S.mood || 'cozy', efx: e };
    }
    // The same room as plain data in floor units, for the paper look to build from.
    function roomModel(out, rec) {
      var sq = S.setting === 'room';
      return {
        N: G.N, setting: S.setting, wall: sq ? rec.wall : S.setting, floor: sq ? rec.floor : { tower: 'flags', tree: 'rings', burrow: 'plank', cave: 'dirt' }[S.setting],
        colours: { wall: sq ? C.wall : wallBase(), wood: C.wood, trim: sq ? C.trim : capCol(), floor: S.setting === 'tree' ? mix(C.floor, '#d2b07a', .55) : S.setting === 'cave' ? mix(C.floor, '#6e5a44', .5) : C.floor, acc: C.acc, dark: C.dark },
        pts: G.pts, back: G.edges.filter(function (e) { return e.back; }).map(function (e) { return { a: e.a, b: e.b, d: e.d, len: e.len, h: Math.max(e.h0, e.h1) }; }),
        pieces: out.map(function (o) { var p = o.p; return { id: p.id, kind: p.kind === 'circle' ? 'rug' : p.kind, art: p.kind === 'circle' ? 'circle' : '', wall: WALLK[p.kind] ? p.wall : '', x: p.x, y: p.y, off: p.off || 0, len: p.len || 0, w: p.w || 0, d: p.d || 0, pinned: !!p.pinned, r: rect(p), h: HT[p.kind], depth: DEPTH[p.kind] || 0, warm: p.warm || null }; }),
        decoCol: DECOCOL, wh: WH, dim: MOODS[S.mood] && MOODS[S.mood].pal ? MOODS[S.mood].dim : S.setting === 'cave' ? .6 : .72
      };
    }

    return {
      draw: draw, generate: generate, applyMood: applyMood, geometry: geometry, place: place, rect: rect, clashes: clashes, hug: hug, snap: snap, inside: inside,
      size: function () { return SZ[S.size] || 8; },
      accent: function () { return (PAL[S.pal] || PAL.oak).acc; },
      isWall: function (kind) { return !!WALLK[kind]; },
      height: function (kind) { return HT[kind] || 1; },
      WH: WH,
      // What the GM can add by hand: every furniture kind, and every decoration any shop type uses.
      KINDS: Object.keys(SIZE), DECOS: Object.keys(DECOCOL).filter(function (n) { return ICONS[n]; }),
      MAT: MAT, PALS: PALS, SETTINGS: SETTINGS, RECIPES: RECIPES, ICONS: ICONS
    };
  }

  // Shop types the generator knows, in the order the GM sees them.
  var ROOM_TYPES = [['general', 'General store'], ['forge', 'Blacksmith'], ['apothecary', 'Apothecary'], ['jeweler', 'Jeweler'], ['library', 'Arcane shop'], ['market', 'Black market']];

  // A stable number from the shop id, so two shops with the same seeds still differ.
  function keyOf(id) { var h = 7; for (var i = 0; i < String(id).length; i++) h = (h * 31 + String(id).charCodeAt(i)) | 0; return Math.abs(h) % 100000; }

  // The layout as saved on the server, from the room state, and back.
  function toLayout(S) {
    return {
      version: 1, roomType: S.roomType, setting: S.setting, size: S.size, furniture: S.full, decorations: S.deco, palette: S.pal,
      seeds: { room: S.seeds.room, goods: S.seeds.goods, deco: S.seeds.deco },
      look: S.look === 'paper' ? 'paper' : 'lit',
      mood: S.mood || '', effects: S.efx ? JSON.parse(JSON.stringify(S.efx)) : null,
      pieces: S.pieces.map(function (p) {
        var o = { id: p.id, kind: p.kind, wall: p.wall || '', x: p.x || 0, y: p.y || 0, off: p.off || 0, len: p.len || 0, w: p.w || 0, d: p.d || 0, pinned: !!p.pinned };
        ['x', 'y', 'off', 'len', 'w', 'd'].forEach(function (k) { o[k] = Math.round(o[k] * 1000) / 1000; });
        return o;
      }),
      decor: (S.decor || []).filter(function (d) { return S.pieces.some(function (p) { return p.id === d.piece; }); }).map(function (d) { return { piece: d.piece, spot: d.spot, icon: d.icon }; }),
      items: S.ov, portrait: S.portrait ? { left: S.portrait[0], top: S.portrait[1] } : null, lines: S.lines.slice()
    };
  }
  function fromLayout(S, L) {
    if (!L) return false;
    S.roomType = L.roomType || S.roomType; S.setting = L.setting || 'room'; S.size = L.size || 'm'; S.full = L.furniture || 'normal';
    S.deco = L.decorations || 'some'; S.pal = L.palette || S.pal;
    S.look = L.look === 'paper' ? 'paper' : 'lit';
    S.mood = MOODS[L.mood] ? L.mood : '';
    S.efx = null;
    if (L.effects) { S.efx = moodEfx(S.mood); for (var k in EFX0) if (typeof L.effects[k] === typeof EFX0[k]) S.efx[k] = L.effects[k]; }
    if (L.seeds) S.seeds = { room: L.seeds.room || 1, goods: L.seeds.goods || 1, deco: L.seeds.deco || 1 };
    S.pieces = (L.pieces || []).map(function (p) { var o = { id: p.id, kind: p.kind, x: p.x, y: p.y, off: p.off || 0, pinned: !!p.pinned }; if (p.wall) { o.wall = p.wall; o.len = p.len; } else { o.w = p.w; o.d = p.d; } return o; });
    S.decor = []; (L.decor || []).forEach(function (d) { S.decor.push({ piece: d.piece, spot: d.spot, icon: d.icon }); });
    S.ov = L.items || {};
    S.portrait = L.portrait ? [L.portrait.left, L.portrait.top] : null;
    S.lines = (L.lines || []).slice();
    return true;
  }

  // Item look: the GM's choice, else the item page's own icon when the room has
  // it, else a box. Colours are only ever a material name or a #rrggbb value.
  function iconFor(name, ICONS) { var n = String(name || '').replace(/^fa[srb]?\s+/, '').replace(/^fa-/, ''); return ICONS[n] ? n : 'box'; }
  function safeHex(c) { return /^#[0-9a-f]{6}$/i.test(c || '') ? c : ''; }
  function shopItems(rels, ov, MAT, ICONS) {
    return rels.map(function (r) {
      var m = r.metadata || {}, o = ov[String(r.id)] || {}, q = m.quantity, price = Number(m.price) || 0, cur = m.currency || 'gp';
      var out = m.in_stock === false || (q !== null && q !== undefined && !m.unlimited && Number(q) <= 0);
      return {
        id: r.id, n: r.targetEntityId ? (r.targetEntityName || 'Item') : (m.custom_name || 'Unnamed item'), k: r.targetEntityType || 'Goods',
        ic: o.icon && ICONS[o.icon] ? o.icon : iconFor(r.targetEntityIcon, ICONS), mat: o.color || '',
        c: (o.color && MAT[o.color]) || safeHex(r.targetEntityColor) || MAT.steel, p: price, cur: cur, price: price + ' ' + cur,
        qty: q === null || q === undefined || m.unlimited ? null : Number(q), out: out, magic: false, entityId: r.targetEntityId || ''
      };
    });
  }

  // 5e coin values in copper. The server rounds a price up to a whole copper
  // the same way, so the short check here agrees with what it will accept.
  var COIN_CP = { cp: 1, sp: 10, ep: 50, gp: 100, pp: 1000 };
  var COIN_ORDER = ['pp', 'gp', 'ep', 'sp', 'cp'];
  function formatPurse(p) {
    var parts = [];
    COIN_ORDER.forEach(function (c) { if (p && p[c] > 0) parts.push(p[c] + ' ' + c); });
    return parts.length ? parts.join(' ') : '0 gp';
  }
  function basketCp(total, cur) {
    var rate = COIN_CP[cur];
    return rate ? Math.ceil(Math.round(total * 100) * rate / 100) : null;
  }

  // What the basket adds up to for one payer. Prices here are for display
  // only; the server prices the sale again from the inventory. A buyer's kind
  // says how they pay: "wealth" needs Wealth at least the dearest unit price,
  // "purse" and a gp "coins" sheet compare in copper when the listing is in a
  // 5e coin, anything else compares the plain numbers.
  function basketSummary(basket, its, buyer) {
    var count = 0, total = 0, curs = [], top = 0;
    its.forEach(function (it) {
      var q = basket[String(it.id)] || 0; if (!q || it.out) return;
      var cur = String(it.cur || 'gp').trim().toLowerCase();
      count += q; total += it.p * q; if (curs.indexOf(cur) < 0) curs.push(cur);
      if (it.p > top) top = it.p;
    });
    var money = buyer && typeof buyer.money === 'number' ? buyer.money : null;
    var cur0 = curs[0] || 'gp', kind = buyer && buyer.kind || 'coins';
    var short = money !== null && total > money, need = 0, moneyText = null, unit = cur0;
    if (kind === 'wealth') {
      need = top; short = money !== null && money < need; unit = '';
      moneyText = money === null ? null : 'Wealth ' + money + (need ? ', needs ' + need : '');
    } else if (money !== null) {
      var have = buyer && typeof buyer.moneyCp === 'number' ? buyer.moneyCp : null, want = basketCp(total, cur0);
      if (have !== null && want !== null) short = want > have;
      if (kind === 'purse') { unit = 'gp'; moneyText = formatPurse(buyer.purse); }
      else { if (buyer.moneyKey === 'gp') unit = 'gp'; moneyText = money + ' ' + unit; }
    }
    return {
      count: count, total: Math.round(total * 100) / 100, currency: cur0, mixed: curs.length > 1,
      noField: !!buyer && money === null, short: short, money: money, kind: kind, need: need, moneyText: moneyText
    };
  }

  window.ShopRoom = { MOODS: MOODS, MOOD_ORDER: MOOD_ORDER, MOOD_KIND: MOOD_KIND, moodEfx: moodEfx, createRoom: createRoom, toLayout: toLayout, fromLayout: fromLayout, shopItems: shopItems, basketSummary: basketSummary, formatPurse: formatPurse, keyOf: keyOf, ROOM_TYPES: ROOM_TYPES };
  if (!window.Chronicle || !window.document) return;

  var FX_KEY = 'chronicle.shopRoom.fx';
  // The shop's door in black-and-white line art: the dark doorway behind, and
  // the leaf that swings open. ZZ becomes a per-widget id prefix.
  var DOOR_WAY = '<svg viewBox="0 0 64 96" aria-hidden="true"><path d="M6 94V34A26 26 0 0 1 58 34V94Z" fill="#000"/><path d="M2 95V34A30 30 0 0 1 62 34V95" fill="none" stroke="currentColor" stroke-width="3"/><path d="M0 95H64" stroke="currentColor" stroke-width="3"/></svg>';
  var DOOR_LEAF = '<svg viewBox="0 0 64 96" aria-hidden="true"><defs><clipPath id="ZZc"><path d="M6 94V34A26 26 0 0 1 58 34V94Z"/></clipPath></defs><path d="M6 94V34A26 26 0 0 1 58 34V94Z" fill="var(--color-card-bg,#fff)" stroke="currentColor" stroke-width="2.5"/><g clip-path="url(#ZZc)" stroke="currentColor" stroke-width="1.4"><path d="M19 4V94M32 4V94M45 4V94"/></g><g fill="currentColor"><rect x="6" y="40" width="34" height="4.5" rx="1"/><rect x="6" y="74" width="34" height="4.5" rx="1"/><circle cx="12" cy="42.2" r="1.3" fill="var(--color-card-bg,#fff)"/><circle cx="12" cy="76.2" r="1.3" fill="var(--color-card-bg,#fff)"/></g><circle cx="49" cy="62" r="4.2" fill="none" stroke="currentColor" stroke-width="2"/><circle cx="49" cy="57" r="1.6" fill="currentColor"/></svg>';
  var doorSeq = 0;
  var KIND_LABEL = { shelf: 'Shelf', rack: 'Rack', cabinet: 'Cabinet', bookcase: 'Bookcase', forge: 'Forge', window: 'Window', herbs: 'Hanging herbs', counter: 'Counter', table: 'Table',
    barrel: 'Barrel', crate: 'Crate', anvil: 'Anvil', glass: 'Glass case', stall: 'Market stall', rug: 'Rug', pedestal: 'Pedestal', lamp: 'Lamp', sack: 'Sack',
    tentacle: 'Tentacle', monolith: 'Watching stone', circle: 'Summoning circle', cane: 'Candy cane', gumdrop: 'Gumdrop', lolly: 'Lollipop tree', ghost: 'Ghost', coffin: 'Coffin',
    gift: 'Gifts', pine: 'Festive tree', coral: 'Coral', kelp: 'Kelp', mushroom: 'Toadstool', bloom: 'Giant bloom', urn: 'Urn', palm: 'Potted palm', pillar: 'Stone pillar', brazier: 'Brazier' };
  var KIND_ICON = { tentacle: 'fa-staff-snake', monolith: 'fa-monument', circle: 'fa-circle-notch', cane: 'fa-candy-cane', gumdrop: 'fa-droplet', lolly: 'fa-lollipop', ghost: 'fa-ghost', coffin: 'fa-cross',
    gift: 'fa-gift', pine: 'fa-tree', coral: 'fa-fish', kelp: 'fa-leaf', mushroom: 'fa-umbrella', bloom: 'fa-seedling', urn: 'fa-wine-bottle', palm: 'fa-tree', pillar: 'fa-landmark', brazier: 'fa-fire-flame-curved', shelf: 'fa-table-list', rack: 'fa-grip-lines', cabinet: 'fa-box-archive', bookcase: 'fa-book', forge: 'fa-fire', window: 'fa-window-maximize', herbs: 'fa-leaf', counter: 'fa-cash-register',
    table: 'fa-table', barrel: 'fa-database', crate: 'fa-box', anvil: 'fa-hammer', glass: 'fa-gem', stall: 'fa-store', rug: 'fa-rug', pedestal: 'fa-monument', lamp: 'fa-lightbulb', sack: 'fa-sack-dollar' };
  var CSS = [
    '.shr{display:grid;gap:12px;margin-bottom:16px}',
    '.shr.arr .shr-scene{min-height:540px}',
    '.shr-card{border:1px solid var(--color-border,#e5e7eb);border-radius:10px;overflow:hidden;background:var(--color-card-bg,#fff)}',
    '.shr-top{display:flex;align-items:center;gap:10px;flex-wrap:wrap;padding:8px 12px;border-bottom:1px solid var(--color-border,#e5e7eb)}',
    '.shr-top b{color:var(--color-text-primary,#111827)}',
    '.shr-sub{color:var(--color-text-secondary,#6b7280);font-size:.8rem}',
    '.shr-mode{margin-left:auto;display:inline-flex;border:1px solid var(--color-border,#e5e7eb);border-radius:8px;overflow:hidden}',
    '.shr-mode button{border:0;background:transparent;color:var(--color-text-secondary,#6b7280);padding:5px 12px;cursor:pointer;font-size:.85rem}',
    '.shr-mode button[aria-pressed="true"]{background:var(--shr-acc);color:#fff;font-weight:600}',
    '.shr-scene{position:relative;touch-action:none;background:radial-gradient(ellipse at 50% 40%,color-mix(in srgb,var(--color-text-primary,#111827) 6%,var(--color-bg-primary,#f9fafb)),color-mix(in srgb,var(--color-text-primary,#111827) 14%,var(--color-bg-primary,#f9fafb)))}',
    '.shr.arr .shr-scene{background-image:radial-gradient(circle,color-mix(in srgb,var(--shr-acc) 35%,transparent) 1px,transparent 1.5px);background-size:16px 16px}',
    '.shr-iso{display:block;width:100%;height:auto;user-select:none}',
    // The paper look sits over the scene on a dark desk; the drawing beneath keeps the size.
    '.shr-paper{position:absolute;inset:0;z-index:2;user-select:none}.shr:not(.paper) .shr-paper{display:none}',
    '.shr.paper .shr-iso{visibility:hidden}.shr.paper .shr-keeper,.shr.paper .shr-plate{display:none}',
    '.shr.paper .shr-scene{background-color:#1d1a15;background-image:radial-gradient(120% 90% at 50% 0%,#3a3328,#17140f 75%)}',
    '.shr-grain{position:absolute;inset:0;pointer-events:none;mix-blend-mode:overlay;opacity:.32;z-index:1}',
    '.shr-grain svg{display:block}',
    '.shr.fxlight .shr-grain{display:none}',
    '.shr.arr .piece{cursor:grab}',
    '.shr.arr .piece:hover>.body{filter:brightness(1.12)}',
    '.shr .it{cursor:pointer}.shr .it:focus{outline:none}',
    '.shr .it:hover use,.shr .it:focus-visible use{filter:brightness(1.25)}',
    '.shr .it:focus-visible circle.ring{stroke:var(--color-accent,#6366f1);stroke-width:2}',
    '.shr .it.out{opacity:.35}',
    '.shr-keeper{position:absolute;width:74px;height:74px;margin:-37px 0 0 -37px;border-radius:50%;padding:0;border:4px solid #c9a54b;overflow:hidden;cursor:pointer;background:#333;z-index:3;box-shadow:0 0 0 2px rgb(0 0 0/.35),0 14px 18px -8px rgb(0 0 0/.7)}',
    '.shr-keeper img,.shr-keeper svg{width:100%;height:100%;display:block;object-fit:cover}',
    '.shr.arr .shr-keeper{cursor:move;outline:2px dashed var(--shr-acc);outline-offset:4px}',
    '.shr-keeper.talk{animation:none}',
    '@keyframes shr-bob{to{transform:translateY(-4px)}}',
    '.shr-plate{position:absolute;transform:translate(-50%,0);white-space:nowrap;font:600 .78rem Georgia,serif;color:#2a2115;background:linear-gradient(#e9cf7c,#b8913a);border-radius:3px;padding:2px 10px;box-shadow:0 1px 2px rgb(0 0 0/.5);z-index:3;pointer-events:none}',
    '@media (max-width:640px){.shr-keeper{width:52px;height:52px;margin:-26px 0 0 -26px}}',
    '.shr-bar{display:flex;align-items:center;gap:10px;flex-wrap:wrap;padding:8px 12px;border-top:3px solid var(--shr-acc)}',
    '.shr-wbtn{display:flex;align-items:center;gap:8px;border:0;background:none;color:var(--color-text-primary,#111827);cursor:pointer;padding:4px 2px;font-weight:600}',
    '.shr-wbtn span{color:var(--color-text-secondary,#6b7280);font-weight:400}',
    '.shr-chev{display:inline-block;transition:transform .25s}',
    '.shr.open .shr-chev{transform:rotate(180deg)}',
    '.shr-bk{margin-left:auto;display:flex;align-items:center;gap:8px;flex-wrap:wrap;font-size:.84rem;color:var(--color-text-secondary,#6b7280)}',
    '.shr-bk:empty{display:none}',
    '.shr-bk select{font:inherit;font-size:.82rem;padding:3px 6px;border:1px solid var(--color-border,#e5e7eb);border-radius:6px;background:var(--color-bg-primary,#fff);color:var(--color-text-primary,#111827)}',
    '.shr-bk .shr-note{flex-basis:100%;text-align:right;font-size:.78rem}',
    '.shr-add,.shr-buy{border:1px solid var(--color-border,#e5e7eb);background:transparent;color:var(--color-text-primary,#111827);border-radius:6px;padding:4px 10px;cursor:pointer;font-size:.8rem;white-space:nowrap}',
    '.shr-buy{background:var(--shr-acc);border-color:var(--shr-acc);color:#fff;font-weight:600}',
    '.shr-add:disabled,.shr-buy:disabled{opacity:.5;cursor:default}',
    '.shr.arr .shr-add,.shr.arr .shr-bk{display:none}',
    '.shr-fx{margin-left:auto;display:inline-flex;align-items:center;gap:6px;font-size:.78rem;color:var(--color-text-secondary,#6b7280)}',
    '.shr-seg{display:inline-grid;grid-auto-flow:column;grid-auto-columns:1fr;border:1px solid var(--color-border,#e5e7eb);border-radius:6px;overflow:hidden}',
    '.shr-seg button{border:0;background:transparent;color:var(--color-text-secondary,#6b7280);padding:4px 8px;cursor:pointer;font-size:.8rem}',
    '.shr-seg button[aria-pressed="true"]{background:var(--color-text-primary,#111827);color:var(--color-bg-primary,#fff);font-weight:600}',
    '.shr-wares{display:grid;grid-template-rows:0fr;transition:grid-template-rows .3s}',
    '.shr.open .shr-wares{grid-template-rows:1fr}',
    '.shr-wares>div{overflow:hidden;min-height:0}',
    '.shr-wp{border-top:1px solid var(--color-border,#e5e7eb);padding:10px 12px 12px;display:grid;gap:8px}',
    '.shr-tabs{display:flex;gap:4px;flex-wrap:wrap}',
    '.shr-tab{border:0;background:transparent;color:var(--color-text-secondary,#6b7280);padding:5px 10px;border-radius:6px;cursor:pointer}',
    '.shr-tab[aria-pressed="true"]{color:var(--shr-acc);background:color-mix(in srgb,var(--shr-acc) 12%,transparent);font-weight:600}',
    '.shr-list{display:grid;gap:2px;max-height:320px;overflow:auto}',
    '.shr-row{display:grid;grid-template-columns:34px minmax(0,1fr) auto auto auto;gap:10px;align-items:center;padding:6px 8px;border-radius:6px}',
    '.shr-row:hover,.shr-row.hl{background:var(--color-bg-tertiary,#f3f4f6)}',
    '.shr-row.out{opacity:.55}',
    '.shr-ic{width:30px;height:30px;border-radius:6px;display:grid;place-items:center;background:color-mix(in srgb,var(--c) 18%,transparent);color:color-mix(in srgb,var(--c),var(--color-text-primary,#111827) 25%)}',
    '.shr-ic svg{width:18px;height:18px;fill:currentColor}',
    '.shr-nm{color:var(--color-text-primary,#111827);font-weight:600;text-decoration:none}',
    'a.shr-nm:hover{text-decoration:underline}',
    '.shr-m{font-size:.76rem;color:var(--color-text-secondary,#6b7280)}',
    '.shr-price{color:var(--shr-acc);font-weight:600;font-variant-numeric:tabular-nums}',
    '.shr-stock{font-size:.76rem;color:var(--color-text-secondary,#6b7280);min-width:4.5em;text-align:right}',
    '.shr-empty{color:var(--color-text-secondary,#6b7280);font-size:.85rem;padding:6px 8px}',
    // The arrange panel slides out inside the room's scene, on its right edge.
    '.shr-panel{position:absolute;top:0;right:0;bottom:0;z-index:20;width:min(340px,100%);display:flex;flex-direction:column;background:var(--color-card-bg,#fff);border-left:1px solid var(--color-border,#e5e7eb);box-shadow:-18px 0 40px -24px rgb(0 0 0/.45);transform:translateX(105%);visibility:hidden;transition:transform .55s cubic-bezier(.32,1.25,.5,1),visibility 0s linear .55s;touch-action:auto}',
    // The panel slides out with no overshoot; leaving with unsaved changes rings the warning instead of shaking.
    '.shr-panel.on{transform:none;visibility:visible;transition:transform var(--dur-slide,360ms) var(--ease-slide,cubic-bezier(.22,.8,.24,1)),visibility 0s}',
    '.shr-panel.nag .shr-warn{box-shadow:inset 0 0 0 2px var(--color-warning,#d97706)}',
    '.shr-ph{display:flex;align-items:center;gap:8px;padding:12px 14px 10px;border-bottom:1px dashed var(--color-border,#e5e7eb)}',
    '.shr-ph h3{flex:1;margin:0;font-size:.66rem;font-weight:700;line-height:1;letter-spacing:.12em;text-transform:uppercase;color:var(--color-text-secondary,#6b7280)}',
    '.shr-x{border:0;background:transparent;color:var(--color-text-secondary,#6b7280);cursor:pointer;width:28px;height:28px;border-radius:6px;font-size:1rem;line-height:1}',
    '.shr-x:hover{background:var(--color-bg-tertiary,#f3f4f6);color:var(--color-text-primary,#111827)}',
    '.shr-pb{flex:1;min-height:0;overflow:auto;padding:12px 14px 6px;display:grid;gap:10px;align-content:start}',
    '.shr-pf{display:flex;justify-content:flex-end;gap:8px;padding:10px 14px;border-top:1px solid var(--color-border,#e5e7eb)}',
    '.shr-pf button{border:0;border-radius:8px;padding:8px 14px;font-size:.82rem;font-weight:600;line-height:1;cursor:pointer;background:transparent;color:var(--color-text-body,#374151)}',
    '.shr-pf .shr-save{background:var(--shr-acc);color:#fff}',
    '.shr-pf .shr-save:disabled{opacity:.5;cursor:default}',
    '.shr-field{display:grid;gap:4px}',
    '.shr-hint{font-size:.72rem;color:var(--color-text-secondary,#6b7280)}',
    '.shr-field>span{font-size:.72rem;text-transform:uppercase;letter-spacing:.06em;color:var(--color-text-muted,#9ca3af)}',
    '.shr-field select,.shr-field textarea{width:100%;min-width:0;border:1px solid var(--color-input-border,#d1d5db);border-radius:6px;padding:6px 8px;background:var(--color-input-bg,#fff);color:var(--color-text-primary,#111827);font:inherit}',
    // A shorthand background here used to wipe the site's dropdown chevron, so
    // the selects read as text boxes; they draw their own, the same as Chronicle's.
    '.shr-field select{-webkit-appearance:none;appearance:none;background-color:var(--color-input-bg,#fff);background-image:url("data:image/svg+xml,%3csvg xmlns=%27http://www.w3.org/2000/svg%27 fill=%27none%27 viewBox=%270 0 20 20%27%3e%3cpath stroke=%27%236b7280%27 stroke-linecap=%27round%27 stroke-linejoin=%27round%27 stroke-width=%271.5%27 d=%27M6 8l4 4 4-4%27/%3e%3c/svg%3e");background-repeat:no-repeat;background-position:right .5rem center;background-size:1.25em 1.25em;padding:8px 2.25rem 8px 10px;border-radius:8px;cursor:pointer}',
    '.shr-field select:hover{border-color:var(--color-text-muted,#9ca3af)}',
    '.shr-field .shr-seg{display:grid}',
    // The generators' shared rows: a part, its keep pin and its reroll dice.
    '.shr-gh{display:flex;align-items:center;gap:8px;margin-top:4px}',
    '.shr-gh>span{flex:1;font-size:.72rem;text-transform:uppercase;letter-spacing:.06em;color:var(--color-text-muted,#9ca3af)}',
    '.shr-rest{display:inline-flex;align-items:center;gap:6px;border:0;background:transparent;color:var(--color-text-body,#374151);font-size:.78rem;font-weight:600;line-height:1;padding:6px 8px;border-radius:8px;cursor:pointer}',
    '.shr-rest:hover{background:var(--color-bg-tertiary,#f3f4f6)}',
    '.shr-gr{display:grid;grid-template-columns:24px minmax(0,1fr) auto;align-items:center;gap:8px;min-height:48px;padding:6px 6px 6px 10px;border-radius:11px;background:var(--color-card-bg,#fff);box-shadow:inset 0 0 0 1px var(--color-border,#e5e7eb);transition:transform .15s,box-shadow .15s}',
    '.shr-gr:hover{transform:translateY(-1px);box-shadow:inset 0 0 0 1px color-mix(in srgb,var(--shr-acc) 55%,transparent),0 4px 12px -6px rgb(0 0 0/.25)}',
    '.shr-gr.kept{background:oklch(.9 .07 85/.3)}',
    '.shr-gr>i{color:var(--color-text-secondary,#6b7280);text-align:center}',
    '.shr-gr b{display:block;font-size:.8rem;color:var(--color-text-primary,#111827)}',
    '.shr-gr small{display:block;font-size:.72rem;color:var(--color-text-secondary,#6b7280);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}',
    '.shr-gr small.roll{animation:shr-roll .3s ease-out}',
    '@keyframes shr-roll{from{opacity:.1}to{opacity:1}}',
    // Adding by hand: a list of furniture and decorations to drag into the room,
    // a ghost that follows the pointer, and a bin that appears while dragging.
    '.shr-addl{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:6px;max-height:212px;overflow:auto;padding:2px}',
    '.shr-addl.dec{grid-template-columns:repeat(6,minmax(0,1fr))}',
    '.shr-addt{display:grid;justify-items:center;gap:3px;border:1px solid var(--color-border,#e5e7eb);background:var(--color-card-bg,#fff);color:var(--color-text-body,#374151);border-radius:8px;padding:7px 4px;font-size:.7rem;line-height:1.1;cursor:grab;touch-action:none;transition:transform .15s,border-color .15s}',
    '.shr-addt:hover{transform:translateY(-1px);border-color:var(--shr-acc)}.shr-addt i{font-size:1rem;color:var(--color-text-secondary,#6b7280)}',
    '.shr-addt svg{width:20px;height:20px}',
    '.shr-ghost{position:fixed;z-index:10000;pointer-events:none;transform:translate(-50%,-50%) rotate(-2deg);animation:shr-lift var(--dur-micro,120ms) var(--ease-out,ease-out);transition:opacity 200ms var(--ease-out,ease-out),transform 200ms var(--ease-out,ease-out);padding:6px 10px;border-radius:999px;background:var(--color-card-bg,#fff);color:var(--color-text-primary,#111827);box-shadow:0 8px 24px -6px rgb(0 0 0/.4);font-size:.78rem;display:flex;align-items:center;gap:6px}',
    '.shr-ghost svg{width:18px;height:18px}',
    '.shr-bin{position:absolute;left:50%;bottom:12px;z-index:15;transform:translate(-50%,140%);display:flex;align-items:center;gap:8px;padding:9px 16px;border-radius:999px;background:rgb(127 29 29/.92);color:#fff;font-size:.8rem;pointer-events:none;opacity:0;transition:transform .3s cubic-bezier(.32,1.25,.5,1),opacity .2s}',
    '.shr.arr .shr-bin{left:calc((100% - min(320px,100%)) / 2)}',
    '.shr.dragging .shr-bin{transform:translate(-50%,0);opacity:1}.shr-bin.hot{background:#b91c1c;transform:translate(-50%,0) scale(1.08)}',
    '.shr.arr .udeco{cursor:grab}',
    '.shr-ib{width:30px;height:30px;border:0;border-radius:8px;display:inline-grid;place-items:center;background:transparent;color:var(--color-text-body,#374151);cursor:pointer;transition:background .15s,color .15s}',
    '.shr-ib:hover:not(:disabled){background:var(--color-bg-tertiary,#f3f4f6);color:var(--color-text-primary,#111827)}',
    '.shr-ib:disabled{opacity:.35;cursor:default}',
    '.shr-ib[aria-pressed="true"]{color:oklch(.62 .12 75);background:oklch(.9 .07 85/.45)}',
    '.shr-warn{display:none;align-items:center;gap:8px;margin:0 14px 10px;padding:6px 6px 6px 12px;border-radius:10px;background:oklch(.95 .06 80);color:oklch(.38 .09 60);box-shadow:inset 0 0 0 1px oklch(.8 .1 75);font-size:.8rem;font-weight:600}',
    '.shr-warn.on{display:flex;animation:shr-warn .28s cubic-bezier(.32,1.25,.5,1) both}',
    '@keyframes shr-warn{from{opacity:0;transform:translateY(8px)}to{opacity:1;transform:none}}',
    '.shr-warn button{margin-left:auto;border:0;background:transparent;color:inherit;font:inherit;text-decoration:underline;cursor:pointer;padding:4px 6px}',
    '.shr-toast{position:absolute;left:50%;bottom:16px;z-index:25;transform:translateX(-50%);background:#111827;color:#fff;font-size:.82rem;font-weight:600;padding:9px 14px;border-radius:9px;box-shadow:0 10px 30px -10px rgb(0 0 0/.5);animation:shr-warn .25s ease-out both;pointer-events:none}',
    '.shr-chips{display:flex;flex-wrap:wrap;gap:6px}',
    '.shr-chip{border:1px solid var(--color-border,#e5e7eb);background:var(--color-card-bg,#fff);color:var(--color-text-body,#374151);border-radius:999px;padding:3px 10px;cursor:pointer;display:inline-flex;align-items:center;gap:6px;font-size:.8rem}',
    '.shr-chip[aria-pressed="true"]{border-color:var(--color-text-primary,#111827);box-shadow:inset 0 0 0 1px var(--color-text-primary,#111827)}',
    '.shr-sw{width:12px;height:12px;border-radius:50%}',
    '.shr-status{font-size:.75rem;color:var(--color-text-muted,#9ca3af)}',
    '.shr-status:empty{display:none}',
    '.shr-tip{position:fixed;z-index:60;max-width:220px;padding:6px 8px;font-size:.75rem;line-height:1.35;background:#111827;color:#f9fafb;border-radius:6px;pointer-events:none;opacity:0;transition:opacity .15s}',
    '.shr-tip.on{opacity:1}.shr-tip b{display:block}',
    '.shr-say{position:fixed;z-index:55;max-width:230px;padding:7px 10px;border-radius:8px;background:var(--color-card-bg,#fff);color:var(--color-text-primary,#111827);border:1px solid var(--color-border,#e5e7eb);font:italic .85rem Georgia,serif;box-shadow:0 8px 20px -6px rgb(0 0 0/.4);opacity:0;transition:opacity .2s;pointer-events:none}',
    '.shr-say.on{opacity:1}',
    '.shr-pick{position:fixed;z-index:70;width:280px;background:var(--color-card-bg,#fff);border:1px solid var(--color-border,#e5e7eb);border-radius:10px;box-shadow:0 16px 36px -10px rgb(0 0 0/.45);padding:10px;display:grid;gap:8px}',
    '.shr-pick[hidden]{display:none}',
    '.shr-pick h4{margin:0;font-size:.85rem;color:var(--color-text-primary,#111827)}',
    '.shr-pick .g{display:grid;grid-template-columns:repeat(7,1fr);gap:4px;max-height:240px;overflow:auto}',
    '.shr-pick .g button{border:1px solid var(--color-border,#e5e7eb);background:var(--color-bg-tertiary,#f3f4f6);border-radius:6px;aspect-ratio:1;display:grid;place-items:center;cursor:pointer;color:var(--color-text-primary,#111827);padding:0}',
    '.shr-pick .g button[aria-pressed="true"]{border-color:var(--shr-acc);box-shadow:inset 0 0 0 1px var(--shr-acc)}',
    '.shr-pick .g svg{width:18px;height:18px;fill:currentColor}',
    '.shr-pick .c{display:flex;gap:4px;flex-wrap:wrap;align-items:center}',
    '.shr-pick .c button.sw{width:20px;height:20px;border-radius:50%;border:2px solid var(--color-card-bg,#fff);box-shadow:0 0 0 1px var(--color-border,#e5e7eb);cursor:pointer;padding:0}',
    '.shr-pick .c button.sw[aria-pressed="true"]{box-shadow:0 0 0 2px var(--color-text-primary,#111827)}',
    '.shr-pick .c .rs{margin-left:auto;border:1px solid var(--color-border,#e5e7eb);background:transparent;color:var(--color-text-body,#374151);border-radius:6px;padding:2px 8px;cursor:pointer;font-size:.78rem}',
    '.shr-pick p{margin:0;font-size:.74rem;color:var(--color-text-secondary,#6b7280)}',
    '.shr button:focus-visible,.shr-pick button:focus-visible{outline:2px solid var(--color-accent,#6366f1);outline-offset:2px}',
    // Closed, the shop is a shopfront with a drawn door. Pressing the door
    // swings it open where it stands, then the room grows out of the
    // doorway as you walk through it.
    '.shr{position:relative}',
    '.shr-front{display:none;align-items:center;gap:14px;width:100%;text-align:left;border:1px solid var(--color-border,#e5e7eb);border-radius:10px;background:var(--color-card-bg,#fff);color:var(--color-text-primary,#111827);padding:14px 16px;cursor:pointer;font:inherit}',
    // Pointing at the door is stepping up to it: the door grows and the wall around it dims.
    '.shr-front:hover{background:radial-gradient(circle at 46px 50%,var(--color-card-bg,#fff) 0,color-mix(in srgb,var(--color-card-bg,#fff) 88%,#78350f) 45%,color-mix(in srgb,var(--color-card-bg,#fff) 80%,#78350f) 100%)}',
    '.shr-front .nm{transition:transform .5s,opacity .5s}.shr-front:hover .nm{transform:translateX(6px);opacity:.8}',
    '.shr-front:hover .shr-dr{transform:scale(1.08)}',
    '.shr-front .nm{flex:1;min-width:0}.shr-front .nm b{display:block}.shr-front .nm span{font-size:.8rem;color:var(--color-text-secondary,#6b7280)}',
    '.shr-dr{position:relative;flex:none;width:64px;height:96px;perspective:420px;transform-origin:50% 55%;transition:transform .5s cubic-bezier(.3,.7,.3,1)}.shr-dr>span{position:absolute;inset:0}.shr-dr svg{display:block;width:100%;height:100%}',
    '.shr-dr .lf{transform-origin:6px 50%;transition:transform .7s cubic-bezier(.45,.05,.3,1)}.shr-front.opening .lf{transform:rotateY(-110deg)}',
    '.shr.closed .shr-front{display:flex}.shr.closed .shr-card{display:none}',
    // Walking in: the dark doorway grows until the widget is black, the room
    // swaps in behind the dark (so its taller height never shows as a jump),
    // then the dark lifts. Only transform and opacity animate, which phones keep smooth.
    '.shr.walking{overflow:hidden;border-radius:10px}',
    '.shr-hole{position:absolute;z-index:8;display:none;background:#000;border-radius:999px 999px 0 0;pointer-events:none;will-change:transform}',
    '.shr-hole.cover{transition:transform .6s cubic-bezier(.55,0,.4,1)}',
    '.shr-dark{position:absolute;inset:0;z-index:8;background:#000;border-radius:10px;opacity:0;visibility:hidden;pointer-events:none}',
    '.shr-dark.on{opacity:1;visibility:visible}.shr-dark.fade{visibility:visible;transition:opacity .8s ease}',
    // The lit room: glows flicker, flames lick, embers rise, dust drifts and the
    // anvil throws sparks. Lights warm up after you walk in (.cold), the room
    // tilts a little toward the mouse, and motion rests with MotionRest.
    '.shr-fl{animation:shr-fl 2.1s ease-in-out infinite alternate}@keyframes shr-fl{0%{opacity:.86}30%{opacity:1}55%{opacity:.8}80%{opacity:.95}100%{opacity:.84}}',
    '.shr-flame{transform-box:fill-box;transform-origin:50% 100%;animation:shr-flame .7s ease-in-out infinite alternate}@keyframes shr-flame{0%{transform:none}50%{transform:scale(.9,1.12) skewX(-5deg)}100%{transform:scale(1.06,.92) skewX(4deg)}}',
    '.shr-ember{opacity:0;animation:shr-ember 2.6s ease-out infinite}@keyframes shr-ember{0%{transform:none;opacity:0}10%{opacity:1}100%{transform:translate(var(--dx,0),-60px);opacity:0}}',
    '.shr-mote{opacity:0;animation:shr-mote 9s linear infinite}@keyframes shr-mote{0%{transform:none;opacity:0}15%{opacity:.8}85%{opacity:.6}100%{transform:translate(-30px,50px);opacity:0}}',
    '.shr-spark{opacity:0;animation:shr-spark 4.2s ease-out infinite}@keyframes shr-spark{0%,86%{transform:none;opacity:0}88%{opacity:1}100%{transform:translate(var(--dx),var(--dy));opacity:0}}',
    '.shr-glows,.shr-life{transition:opacity 1.4s ease}.shr.cold .shr-glows,.shr.cold .shr-life{opacity:0;transition:none}',
    '.shr-iso{transition:transform .5s cubic-bezier(.2,.7,.2,1)}',
    '.shr-stay{animation:shr-nudge .45s ease}@keyframes shr-nudge{0%,100%{transform:none}25%{transform:translateX(-5px)}75%{transform:translateX(5px)}}',
    // The standard drag move: lifts to the dragged elevation with a slight tilt, settles as it drops.
    '@keyframes shr-lift{from{transform:translate(-50%,-50%) scale(.96);box-shadow:none}}.shr-ghost{box-shadow:var(--elev-dragged,0 16px 36px -8px rgb(0 0 0/.22))!important}.shr-ghost.drop{opacity:0;transform:translate(-50%,-50%) scale(.97)}',
    '.shr-ghost .shr-mart{width:26px;height:26px}',
    // Mood and atmosphere in the lit room: fades for living details, falling weather in the windows.
    '.shr-pulse{animation:shr-pulse 3s ease-in-out infinite alternate}@keyframes shr-pulse{from{opacity:.55}to{opacity:1}}',
    '.shr-haunt{animation:shr-haunt 3.4s ease-in-out infinite alternate}@keyframes shr-haunt{from{opacity:.9}to{opacity:.45}}',
    '.shr-twinkle{animation:shr-tw 2.4s ease-in-out infinite alternate}@keyframes shr-tw{from{opacity:.35}to{opacity:1}}',
    '.shr-rain{animation:shr-fall .55s linear infinite}.shr-snow{animation:shr-fall 4s linear infinite}@keyframes shr-fall{0%{transform:none;opacity:0}10%{opacity:.85}100%{transform:translate(-3px,var(--fall));opacity:.5}}',
    '.shr-smoke{opacity:0;transform-box:fill-box;transform-origin:50% 50%;filter:blur(1.2px);animation:shr-smoke 3.6s ease-out infinite}@keyframes shr-smoke{0%{transform:scale(.5);opacity:0}20%{opacity:.7}100%{transform:translate(8px,-36px) scale(2.4);opacity:0}}',
    '.shr-iso.no-dust .shr-mote,.shr-iso.no-embers .shr-ember,.shr-iso.no-embers .shr-spark{display:none}',
    '.shr-iso.no-flicker .shr-fl,.shr-iso.no-flicker .shr-flame,.shr-iso.no-flicker .shr-pulse,.shr-iso.no-flicker .shr-twinkle,.shr-iso.no-flicker .shr-haunt{animation:none}',
    // Calm and Off: every loop stops and the travelling bits go; colour and fades still change. Off also drops transitions.
    'html[data-motion=calm] .shr *,html[data-motion=off] .shr *{animation:none!important}',
    'html[data-motion=calm] .shr :is(.shr-ember,.shr-spark,.shr-mote,.shr-rain,.shr-snow,.shr-smoke),html[data-motion=off] .shr :is(.shr-ember,.shr-spark,.shr-mote,.shr-rain,.shr-snow,.shr-smoke){display:none}',
    'html[data-motion=calm] .shr-panel{transition:opacity var(--dur-standard,.2s),visibility 0s!important;transform:none;opacity:0}html[data-motion=calm] .shr-panel.on{opacity:1}',
    'html[data-motion=off] .shr *,html[data-motion=off] .shr-ghost{transition:none!important}',
    // Arrange panel sections: each folds to its own height.
    '.shr-sec{border:1px solid var(--color-border,#e5e7eb);border-radius:10px;background:var(--color-card-bg,#fff)}',
    '.shr-sh{display:flex;width:100%;align-items:center;gap:8px;border:0;background:transparent;padding:10px 10px;cursor:pointer;color:var(--color-text-primary,#111827);text-align:left;font:inherit;border-radius:10px}',
    '.shr-sh:hover{background:var(--color-bg-secondary,#f9fafb)}.shr-sh>i:first-child{width:16px;text-align:center;color:var(--color-text-secondary,#6b7280)}',
    '.shr-sh b{font-size:.82rem;font-weight:600}.shr-sh small{flex:1;min-width:0;font-size:.72rem;color:var(--color-text-secondary,#6b7280);text-align:right;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}',
    '.shr-sh .chev{font-size:.7rem;color:var(--color-text-muted,#9ca3af);transition:transform var(--dur-standard,.2s) var(--ease-out,ease-out)}.shr-sh[aria-expanded="true"] .chev{transform:rotate(180deg)}',
    '.shr-sb{overflow:hidden}.shr-sb[hidden]{display:none}.shr-sbi{padding:2px 10px 12px;display:grid;gap:10px}',
    '.shr-moods{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:6px}',
    '.shr-mood{border:1px solid var(--color-border,#e5e7eb);background:var(--color-card-bg,#fff);border-radius:8px;padding:0;cursor:pointer;overflow:hidden;color:var(--color-text-body,#374151);font:inherit;font-size:.7rem;text-align:left}',
    '.shr-mood .sw{display:flex;height:20px}.shr-mood .sw i{flex:1}.shr-mood b{display:block;padding:4px 6px 5px;font-weight:600;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}',
    '.shr-mood:hover{border-color:var(--color-text-muted,#9ca3af)}.shr-mood[aria-pressed="true"]{border-color:var(--shr-acc);box-shadow:0 0 0 1px var(--shr-acc);color:var(--color-text-primary,#111827)}',
    '.shr-rng{display:grid;grid-template-columns:84px minmax(0,1fr) 40px;align-items:center;gap:8px;font-size:.78rem;color:var(--color-text-body,#374151)}',
    '.shr-rng input{accent-color:var(--shr-acc);min-width:0;width:100%}.shr-rng output{text-align:right;font-variant-numeric:tabular-nums;color:var(--color-text-secondary,#6b7280);font-size:.72rem}',
    '.shr-tgl{display:flex;flex-wrap:wrap;gap:6px}.shr-tgl button{border:1px solid var(--color-border,#e5e7eb);background:var(--color-card-bg,#fff);color:var(--color-text-body,#374151);border-radius:999px;padding:3px 10px;font:inherit;font-size:.76rem;cursor:pointer}',
    '.shr-tgl button[aria-pressed="true"]{border-color:var(--shr-acc);color:var(--color-text-primary,#111827);background:color-mix(in srgb,var(--shr-acc) 12%,transparent)}',
    '.shr-note{margin:0;font-size:.74rem;color:var(--color-text-secondary,#6b7280)}.shr-link{border:0;background:none;padding:0;color:var(--shr-acc);font:inherit;font-size:.76rem;cursor:pointer;justify-self:start}',
    '.shr-addl .shr-sub2{grid-column:1/-1;font-size:.64rem;letter-spacing:.06em;text-transform:uppercase;color:var(--color-text-muted,#9ca3af);margin:4px 0 -2px}',
    '.shr-addt .shr-mart{width:30px;height:30px}',
    '@media (prefers-reduced-motion:reduce){.shr *,.shr-say,.shr-tip{animation:none!important;transition:none!important}.shr-ember,.shr-mote,.shr-spark,.shr-rain,.shr-snow,.shr-smoke{display:none}}'
  ].join('\n');

  function injectStyle() {
    if (document.getElementById('shr-style')) return;
    var st = document.createElement('style'); st.id = 'shr-style'; st.textContent = CSS; document.head.appendChild(st);
  }
  function iconSvg(name, ICONS) { var g = ICONS[name] || ICONS.box; return g ? '<svg viewBox="0 0 ' + g[0] + ' 512" aria-hidden="true"><path d="' + g[1] + '"/></svg>' : ''; }
  function readFx() { try { return localStorage.getItem(FX_KEY) === 'light' ? 'light' : 'full'; } catch (e) { return 'full'; } }
  function saveFx(v) { try { localStorage.setItem(FX_KEY, v); } catch (e) { /* private window: the choice lasts this page only */ } }
  function silhouette(color) {
    return '<svg viewBox="0 0 100 100" aria-hidden="true"><rect width="100" height="100" fill="' + color + '"/><circle cx="50" cy="42" r="18" fill="rgb(0 0 0 / .45)"/><path d="M12 100c4-26 20-36 38-36s34 10 38 36z" fill="rgb(0 0 0 / .45)"/></svg>';
  }

  Chronicle.register('shop_room', {
    init: function (el) {
      injectStyle();
      var ds = el.dataset, canArrange = ds.canArrange === 'true', campaignUrl = ds.campaignUrl || '';
      var eid = (ds.roomEndpoint || '').split('/shops/')[1] || '';
      var buyersEndpoint = (ds.roomEndpoint || '').replace(/\/room$/, '/buyers'), buyEndpoint = (ds.roomEndpoint || '').replace(/\/room$/, '/buy');
      var S = { roomType: 'general', pal: 'oak', mood: '', efx: null, setting: 'room', look: 'lit', fx: readFx(), size: 'm', full: 'normal', deco: 'some', keep: true,
        seeds: { room: 1, goods: 1, deco: 1 }, pieces: [], decor: [], ov: {}, portrait: null, lines: [], mode: 'shop', its: [],
        key: keyOf(eid), name: ds.shopName || 'Shop', dark: document.documentElement.classList.contains('dark') };
      var room = createRoom(S), ICONS = room.ICONS, rels = [], relsOk = false, buy = null, basket = {}, payer = '', busy = false, note = '', tab = 'All', open = false, line = 0, dirty = false, busySave = false, savedLayout = null, partKeep = { room: false, goods: false, deco: false };
      el.hidden = true;

      // ---- Skeleton ----
      var uid = ++doorSeq;
      el.innerHTML = '<div class="shr closed' + (S.fx === 'light' ? ' fxlight' : '') + '">' +
        '<button type="button" class="shr-front" data-enter="1" aria-expanded="false"><span class="shr-dr"><span class="w">' + DOOR_WAY.replace(/ZZ/g, 'shr-d' + uid) + '</span><span class="lf">' + DOOR_LEAF.replace(/ZZ/g, 'shr-d' + uid) + '</span></span><span class="nm"><b></b><span>Press the door to step inside</span></span></button><div class="shr-hole" aria-hidden="true"></div><div class="shr-dark" aria-hidden="true"></div>' +
        '<div class="shr-card">' +
        '<div class="shr-top"><b></b><span class="shr-sub">Shop</span>' + (canArrange ? '<span class="shr-mode" role="group" aria-label="Mode"><button type="button" data-mode="shop" aria-pressed="true">Shop</button><button type="button" data-mode="arr" aria-pressed="false">Arrange</button></span>' : '') + '</div>' +
        '<div class="shr-scene"><svg class="shr-iso" role="img"></svg><div class="shr-paper"></div><div class="shr-grain"><svg width="100%" height="100%" aria-hidden="true"><filter id="shr-grn"><feTurbulence type="fractalNoise" baseFrequency=".85" numOctaves="2" stitchTiles="stitch"/><feColorMatrix type="saturate" values="0"/></filter><rect width="100%" height="100%" filter="url(#shr-grn)"/></svg></div>' +
        '<button type="button" class="shr-keeper"></button><div class="shr-plate"></div></div>' +
        '<div class="shr-bar"><button type="button" class="shr-wbtn" aria-expanded="false">Wares <span></span><i class="shr-chev" aria-hidden="true">▾</i></button><div class="shr-bk" aria-live="polite"></div>' +
        '<span class="shr-fx">Shadows <span class="shr-seg" role="group" aria-label="Shadows and light"><button type="button" data-fx="full">Full</button><button type="button" data-fx="light">Light</button></span></span></div>' +
        '<div class="shr-wares"><div><div class="shr-wp"><div class="shr-tabs"></div><div class="shr-list"></div></div></div></div></div></div>';
      var root = el.firstChild, svg = root.querySelector('.shr-iso'), pel = root.querySelector('.shr-paper'), scene = root.querySelector('.shr-scene'), keeper = root.querySelector('.shr-keeper'), plate = root.querySelector('.shr-plate');
      var tip = document.createElement('div'), say = document.createElement('div'), pick = document.createElement('div'), panel = null;
      tip.className = 'shr-tip'; tip.setAttribute('role', 'tooltip'); say.className = 'shr-say'; say.setAttribute('aria-live', 'polite');
      pick.className = 'shr-pick'; pick.hidden = true; pick.setAttribute('role', 'dialog'); pick.setAttribute('aria-label', 'Change item look');
      document.body.appendChild(tip); document.body.appendChild(say); document.body.appendChild(pick);
      root.querySelector('.shr-top b').textContent = S.name;
      root.querySelector('.shr-front b').textContent = S.name;
      keeper.setAttribute('aria-label', 'Talk to the shopkeeper');
      if (ds.shopImage) { var im = document.createElement('img'); im.src = ds.shopImage; im.alt = ''; keeper.appendChild(im); }

      // ---- Drawing ----
      var view = null;
      function draw() {
        S.dark = document.documentElement.classList.contains('dark');
        S.its = shopItems(rels, S.ov, room.MAT, ICONS);
        view = room.draw();
        svg.setAttribute('viewBox', view.viewBox.join(' '));
        svg.setAttribute('aria-label', S.name + ', the shop room');
        svg.innerHTML = view.svg;
        svg.setAttribute('class', 'shr-iso' + (view.cls || ''));
        var acc = room.accent();
        root.style.setProperty('--shr-acc', acc);
        if (!ds.shopImage) keeper.innerHTML = silhouette(acc);
        placeKeeper();
        // The paper look stands the same room up as cut card over the drawing,
        // which stays underneath to keep the scene's size.
        var paper = paperOn();
        root.classList.toggle('paper', paper);
        if (paper) window.ShopRoomPaper.render(pel, { view: view, its: S.its, name: S.name, image: ds.shopImage, icons: ICONS, fx: S.fx, mood: view.mood, efx: view.efx, arr: S.mode === 'arr', shut: root.classList.contains('cold') });
        else pel.innerHTML = '';
      }
      function paperOn() { return S.look === 'paper' && !!window.ShopRoomPaper; }
      function placeKeeper() {
        var l = 18, t = 40, VB = view.viewBox;
        if (S.portrait) { l = S.portrait[0]; t = S.portrait[1]; }
        else if (view.keeperAt) { l = (view.keeperAt[0] - VB[0]) / VB[2] * 100; t = (view.keeperAt[1] - VB[1]) / VB[3] * 100; }
        keeper.style.left = l + '%'; keeper.style.top = t + '%';
        plate.style.left = l + '%'; plate.style.top = 'calc(' + t + '% + ' + (keeper.offsetHeight / 2 + 2) + 'px)';
        plate.textContent = S.name;
      }
      var raf = 0;
      function later() { if (!raf) raf = requestAnimationFrame(function () { raf = 0; draw(); }); }

      // ---- Wares list ----
      function renderList() {
        var cats = ['All'];
        S.its.forEach(function (it) { if (cats.indexOf(it.k) < 0) cats.push(it.k); });
        if (cats.indexOf(tab) < 0) tab = 'All';
        root.querySelector('.shr-wbtn span').textContent = S.its.length + (S.its.length === 1 ? ' item' : ' items');
        root.querySelector('.shr-tabs').innerHTML = cats.length > 2 ? cats.map(function (c) { return '<button type="button" class="shr-tab" data-tab="' + esc(c) + '" aria-pressed="' + (c === tab) + '">' + esc(c) + '</button>'; }).join('') : '';
        var rows = S.its.filter(function (it) { return tab === 'All' || it.k === tab; }).map(function (it) {
          var name = it.entityId ? '<a class="shr-nm" href="' + esc(campaignUrl + '/entities/' + it.entityId) + '" data-hx-boost="true">' + esc(it.n) + '</a>' : '<span class="shr-nm">' + esc(it.n) + '</span>';
          var stock = it.out ? 'Sold out' : it.qty === null ? '' : it.qty + ' left';
          return '<div class="shr-row' + (it.out ? ' out' : '') + '" data-row="' + it.id + '"><span class="shr-ic" style="--c:' + it.c + '">' + iconSvg(it.ic, ICONS) + '</span><span>' + name + '<br><span class="shr-m">' + esc(it.k) + '</span></span><span class="shr-price">' + esc(it.price) + '</span><span class="shr-stock">' + stock + '</span>' + addBtn(it) + '</div>';
        });
        root.querySelector('.shr-list').innerHTML = rows.join('') || '<p class="shr-empty">' + (canArrange ? 'Nothing for sale yet. Add goods in the shop inventory below.' : 'Nothing for sale right now.') + '</p>';
        root.querySelectorAll('.shr-fx button').forEach(function (b) { b.setAttribute('aria-pressed', b.dataset.fx === S.fx); });
        renderBasket();
      }
      function addBtn(it) {
        if (!buy || !buy.buyers.length) return '<span></span>';
        var q = basket[String(it.id)] || 0;
        // Only what the server will sell: priced, in stock, and no more than is left.
        var max = Math.min(99, it.qty === null ? 99 : it.qty);
        if (it.out) return '<button type="button" class="shr-add" disabled>Sold out</button>';
        if (!(it.p > 0)) return '<button type="button" class="shr-add" disabled>No price</button>';
        return '<button type="button" class="shr-add" data-add="' + esc(it.id) + '"' + (q >= max ? ' disabled' : '') + '>' + (q ? 'Add another (' + q + ')' : 'Add') + '</button>';
      }
      function buyer() { var b = null; (buy ? buy.buyers : []).forEach(function (x) { if (x.id === payer) b = x; }); return b; }
      // The basket line in the bar: what's in it, who pays, and Buy.
      function renderBasket() {
        var bk = root.querySelector('.shr-bk');
        if (!buy || !buy.buyers.length) { bk.innerHTML = ''; return; }
        var who = buyer(), sm = basketSummary(basket, S.its, who), h = '';
        h += sm.count ? '<span>' + sm.count + ' in basket · <span class="shr-price">' + esc(sm.total + ' ' + sm.currency) + '</span></span><button type="button" class="shr-add" data-empty="1">Empty</button>' : '<span>Basket is empty</span>';
        var money = sm.moneyText === null ? '' : ' · ' + esc(sm.moneyText);
        if (buy.buyers.length > 1) {
          h += '<label>Paying: <select data-payer="1">' + buy.buyers.map(function (b) { return '<option value="' + esc(b.id) + '"' + (b.id === payer ? ' selected' : '') + '>' + esc(b.name) + '</option>'; }).join('') + '</select>' + money + '</label>';
        } else if (who) h += '<span>' + esc(who.name) + (sm.moneyText === null ? '' : (sm.kind === 'wealth' ? ' · ' : ' has ') + esc(sm.moneyText)) + '</span>';
        var why = sm.noField ? 'This sheet has no coin field' : sm.mixed ? 'Mixed currencies' : sm.short ? (sm.kind === 'wealth' ? 'Needs Wealth ' + sm.need : 'Not enough coin') : '';
        // Outside downtime a player asks; the GM approves it on the Stashes page.
        var closed = !buy.canBuyNow, label = why || (busy ? (closed ? 'Asking…' : 'Buying…') : closed ? 'Ask to buy' : 'Buy');
        h += '<button type="button" class="shr-buy" data-buy="1"' + (!sm.count || why || busy ? ' disabled' : '') + '>' + label + '</button>';
        var n = note || (closed ? 'It’s not downtime, so the GM approves this on the Stashes page.' : '');
        if (n) h += '<span class="shr-note">' + esc(n) + '</span>';
        bk.innerHTML = h;
      }
      function loadBuyers() {
        return Chronicle.apiFetch(buyersEndpoint).then(function (r) { return r.ok ? r.json() : null; }).catch(function () { return null; }).then(function (j) {
          buy = j && Array.isArray(j.buyers) ? { canBuyNow: !!j.canBuyNow, buyers: j.buyers } : null;
          if (buy && !buyer()) payer = buy.buyers.length ? buy.buyers[0].id : '';
        });
      }
      function loadGoods() {
        return Chronicle.apiFetch(ds.relationsEndpoint).then(function (r) { if (r.ok) { relsOk = true; return r.json(); } return []; }).catch(function () { return []; }).then(function (j) {
          rels = (Array.isArray(j) ? j : (j && j.data) || []).filter(function (r) { return r.relationType === 'sells'; }).map(function (r) {
            if (typeof r.metadata === 'string') { try { r.metadata = JSON.parse(r.metadata); } catch (e) { r.metadata = {}; } }
            r.metadata = r.metadata || {}; return r;
          });
        });
      }
      function doBuy() {
        var sm = basketSummary(basket, S.its, buyer());
        var items = S.its.filter(function (it) { return basket[String(it.id)] && !it.out; }).map(function (it) { return { relationId: it.id, quantity: basket[String(it.id)] }; });
        if (!items.length || busy) return;
        busy = true; note = ''; renderBasket();
        Chronicle.apiFetch(buyEndpoint, { method: 'POST', body: { buyerEntityId: payer, items: items }, csrfToken: ds.csrfToken }).then(function (r) {
          return r.json().catch(function () { return {}; }).then(function (j) {
            if (!r.ok) throw new Error((j && (j.message || j.error)) || 'That didn’t go through. Try again.');
            var who = buyer(), name = who ? who.name : 'They';
            var what = sm.count + (sm.count === 1 ? ' item' : ' items');
            if (j.status === 'requested') note = 'Asked the GM for ' + what + '.';
            else if (sm.kind === 'wealth') note = 'Bought ' + what + '. Wealth stays at ' + j.moneyLeft + '.';
            else {
              // A gp sheet is charged in gp whatever the shop's coin, so its balance reads in gp.
              var left = j.purseLeft ? j.purseLeft : j.moneyLeft + ' ' + (who && who.moneyKey === 'gp' ? 'gp' : j.currency);
              note = 'Bought ' + what + ' for ' + j.spent + ' ' + j.currency + '. ' + name + ' has ' + left + ' left.' + (j.change ? ' Change: ' + j.change + '.' : '');
            }
            basket = {};
            return Promise.all([loadGoods(), loadBuyers()]);
          });
        }).catch(function (e) { note = e.message; }).then(function () { busy = false; draw(); renderList(); });
      }
      function setOpen(v) { open = v; root.classList.toggle('open', v); root.querySelector('.shr-wbtn').setAttribute('aria-expanded', v); }

      // ---- Tooltip and the keeper's lines ----
      function showTip(g) {
        var it = S.its[+g.getAttribute('data-i')]; if (!it) return;
        var r = g.getBoundingClientRect();
        tip.innerHTML = '<b>' + esc(it.n) + '</b>' + esc(it.price) + (it.out ? ' · sold out' : it.qty !== null ? ' · ' + it.qty + ' left' : '');
        tip.classList.add('on');
        tip.style.left = Math.max(8, Math.min(innerWidth - tip.offsetWidth - 8, r.left + r.width / 2 - tip.offsetWidth / 2)) + 'px';
        tip.style.top = Math.max(8, r.top - tip.offsetHeight - 8) + 'px';
      }
      var typeT = 0, sayT = 0;
      function talk() {
        if (!S.lines.length) return;
        var kEl = (paperOn() && pel.querySelector('.srp-keeper')) || keeper, kc = kEl.querySelector('.srp-cd') || kEl;
        var text = S.lines[line++ % S.lines.length], n = 0, r = kc.getBoundingClientRect();
        kEl.classList.remove('talk'); void kEl.offsetWidth; kEl.classList.add('talk');
        clearInterval(typeT); clearTimeout(sayT);
        say.textContent = '“' + text + '”'; say.classList.add('on');
        var x = r.right + 10; if (x + say.offsetWidth > innerWidth - 8) x = r.left - say.offsetWidth - 10;
        say.style.left = Math.max(8, x) + 'px'; say.style.top = (r.top + 6) + 'px';
        if (!matchMedia('(prefers-reduced-motion: reduce)').matches) {
          say.textContent = '“';
          typeT = setInterval(function () { n++; say.textContent = '“' + text.slice(0, n) + (n < text.length ? '' : '”'); if (n >= text.length) clearInterval(typeT); }, 35);
        }
        sayT = setTimeout(function () { say.classList.remove('on'); }, 4000);
      }

      // ---- Saving (owners only). Changes in Arrange wait for "Save room";
      // Cancel or Discard puts the room back as it was last saved. ----
      function status(t) { var s = panel && panel.querySelector('.shr-status'); if (s) s.textContent = t; }
      function save() {
        if (!canArrange) return;
        dirty = true;
        if (panel) { panel.querySelector('.shr-warn').classList.remove('on'); panel.querySelector('.shr-save').disabled = false; }
        status('');
      }
      function saveRoom() {
        if (!canArrange || busySave) return;
        // Drop looks for goods no longer sold, so the saved list can't creep up to
        // the server's cap; only when the goods list really loaded.
        if (relsOk) { var live = {}; rels.forEach(function (r) { live[String(r.id)] = 1; }); Object.keys(S.ov).forEach(function (k) { if (!live[k]) delete S.ov[k]; }); }
        var layout = toLayout(S);
        busySave = true; status('Saving…');
        Chronicle.apiFetch(ds.roomEndpoint, { method: 'PUT', body: layout, csrfToken: ds.csrfToken })
          .then(function (res) {
            if (!res.ok) throw new Error('save ' + res.status);
            savedLayout = JSON.parse(JSON.stringify(layout)); dirty = false; busySave = false;
            S.mode = 'shop'; setMode(); toast('Room saved');
          })
          .catch(function () { busySave = false; status('Couldn’t save. Your changes are still here; try again.'); });
      }
      // Leaving Arrange: straight away when nothing changed, otherwise the
      // panel shakes and says so, and nothing is lost.
      function tryLeave() {
        if (!panel || S.mode !== 'arr') return;
        if (!dirty) { S.mode = 'shop'; setMode(); return; }
        var w = panel.querySelector('.shr-warn');
        panel.classList.remove('nag'); void panel.offsetWidth; panel.classList.add('nag');
        w.classList.remove('on'); void w.offsetWidth; w.classList.add('on');
      }
      function discard() {
        fromLayout(S, JSON.parse(JSON.stringify(savedLayout))); room.geometry();
        dirty = false; S.mode = 'shop'; setMode(); renderList();
      }
      function toast(t) {
        var d = document.createElement('div'); d.className = 'shr-toast'; d.setAttribute('role', 'status'); d.textContent = t;
        scene.appendChild(d); setTimeout(function () { d.remove(); }, 1800);
      }

      // ---- Arrange: drag furniture, drag the portrait, unpin, change an item's look ----
      var drag = null, kd = null, picking = null, adding = null, addTab = 'm', bin = null, secOpen = { mood: true, add: true };
      function svgPt(e) { var m = svg.getScreenCTM().inverse(), q = svg.createSVGPoint(); q.x = e.clientX; q.y = e.clientY; q = q.matrixTransform(m); return [q.x, q.y]; }
      function toFloor(q) { var a = q[0] / 32, b = q[1] / 16; return [(a + b) / 2, (b - a) / 2]; }
      function byId(id) { for (var i = 0; i < S.pieces.length; i++) if (S.pieces[i].id === id) return S.pieces[i]; return null; }
      function onSvgDown(e) {
        if (S.mode !== 'arr') return;
        var pin = e.target.closest('.pinbtn');
        if (pin) { var pp = byId(+pin.getAttribute('data-unpin')); if (pp) { pp.pinned = false; draw(); save(); } return; }
        if (e.target.closest('.it')) return;
        var ud = e.target.closest('.udeco');
        if (ud) { pickDeco(e, +ud.getAttribute('data-up'), +ud.getAttribute('data-ua')); return; }
        var g = e.target.closest('.piece'); if (!g) return;
        var p = byId(+g.getAttribute('data-p')); if (!p) return;
        var q = svgPt(e);
        drag = { p: p, q: q, f: toFloor(q), x: p.x, y: p.y, moved: false };
        showBin(true);
        svg.setPointerCapture(e.pointerId); e.preventDefault();
      }
      function onSvgMove(e) {
        if (!drag || drag.paper) return;
        var p = drag.p, q = svgPt(e), nx = drag.x, ny = drag.y;
        // Wall pieces slide along their wall; floor pieces move on the floor grid.
        if (room.isWall(p.kind)) { if (p.wall === 'Y') nx = room.snap(drag.x + (q[0] - drag.q[0]) / 32); else ny = room.snap(drag.y - (q[0] - drag.q[0]) / 32); }
        else { var f = toFloor(q); nx = room.snap(drag.x + f[0] - drag.f[0]); ny = room.snap(drag.y + f[1] - drag.f[1]); }
        moveTo(p, nx, ny);
      }
      // Picking up a decoration. The room's own ones are kept where they stand
      // first (see ShopRoomDecor.freeze), so any of them can be moved.
      function pickDeco(e, up, ua) {
        var find = function () { return S.decor.filter(function (d) { return d.piece === up && d.spot === ua; })[0]; }, was = find();
        if (!was && window.ShopRoomDecor) { window.ShopRoomDecor.freeze(S, view.placed); was = find(); save(); renderPanel(); }
        if (was) startAdd(e, { icon: was.icon, from: was });
      }
      // The paper look: the same drags, with the pointer turned into a floor point.
      function floorFrom(e) { return paperOn() ? window.ShopRoomPaper.floorAt(pel, e.clientX, e.clientY) : toFloor(svgPt(e)); }
      function spotFrom(e) { return paperOn() ? window.ShopRoomPaper.spotAt(pel, e.clientX, e.clientY) : nearestSpot(svgPt(e)); }
      function onPaperDown(e) {
        if (S.mode !== 'arr') return;
        var h = window.ShopRoomPaper.hit(pel, e.clientX, e.clientY) || e.target;
        if (h.closest('.it,.srp-keeper')) return;
        var dc = h.closest('.srp-deco');
        if (dc) { pickDeco(e, +dc.getAttribute('data-up'), +dc.getAttribute('data-ua')); return; }
        var g = e.target.closest('.srp-pc'), p = g && byId(+g.getAttribute('data-p')), f = p && floorFrom(e);
        if (!f) return;
        drag = { p: p, f: f, x: p.x, y: p.y, moved: false, paper: true };
        showBin(true);
        pel.setPointerCapture(e.pointerId); e.preventDefault();
      }
      function onPaperMove(e) {
        if (!drag || !drag.paper) return;
        if (bin) bin.classList.toggle('hot', overBin(e));
        var p = drag.p, f = floorFrom(e), nx = drag.x, ny = drag.y;
        if (!f) return;
        if (room.isWall(p.kind)) { if (p.wall === 'Y') nx = room.snap(drag.x + f[0] - drag.f[0]); else ny = room.snap(drag.y + f[1] - drag.f[1]); }
        else { nx = room.snap(drag.x + f[0] - drag.f[0]); ny = room.snap(drag.y + f[1] - drag.f[1]); }
        moveTo(p, nx, ny);
      }
      function onPaperClick(e) {
        var h = window.ShopRoomPaper.hit(pel, e.clientX, e.clientY) || e.target, g = h.closest('.it'); if (g) { useItem(g); return; }
        if (h.closest('.srp-keeper')) onKeeperClick();
      }
      function onPaperKey(e) { if ((e.key === 'Enter' || e.key === ' ') && e.target.closest('.srp-keeper')) { e.preventDefault(); onKeeperClick(); return; } onSvgKey(e); }
      function moveTo(p, nx, ny) {
        if (nx === p.x && ny === p.y) return;
        var ox = p.x, oy = p.y, oo = p.off; p.x = nx; p.y = ny;
        if ((room.isWall(p.kind) && !room.hug(p)) || room.clashes(p, S.pieces, room.size())) { p.x = ox; p.y = oy; p.off = oo; return; }
        drag.moved = true; later();
      }
      function onSvgUp(e) {
        if (!drag) return;
        var p = drag.p, moved = drag.moved; drag = null; showBin(false);
        if (e && overBin(e)) { removePiece(p); return; }
        if (moved) p.pinned = true;
        draw(); if (moved) save();
      }
      function onSvgMoveBin(e) { if (drag) bin.classList.toggle('hot', overBin(e)); }

      // ---- Adding furniture and decorations by hand ----
      function showBin(on) {
        if (!bin) { bin = document.createElement('div'); bin.className = 'shr-bin'; bin.setAttribute('aria-hidden', 'true'); bin.innerHTML = '<i class="fa-solid fa-trash-can"></i><span>Drop here to remove</span>'; scene.appendChild(bin); }
        root.classList.toggle('dragging', on); if (!on) bin.classList.remove('hot');
      }
      function overBin(e) { if (!bin || !root.classList.contains('dragging')) return false; var r = bin.getBoundingClientRect(); return e.clientX >= r.left - 12 && e.clientX <= r.right + 12 && e.clientY >= r.top - 12 && e.clientY <= r.bottom + 12; }
      function removePiece(p) {
        S.pieces = S.pieces.filter(function (q) { return q !== p; });
        S.decor = S.decor.filter(function (d) { return d.piece !== p.id; });
        draw(); save(); status('Removed. Save room to keep it.');
      }
      function startAdd(e, what) {
        var g = document.createElement('div'); g.className = 'shr-ghost';
        g.innerHTML = what.kind ? kindArt(what.kind) + esc(KIND_LABEL[what.kind] || what.kind) : iconSvg(what.icon, ICONS);
        g.style.left = e.clientX + 'px'; g.style.top = e.clientY + 'px'; g.hidden = !what.from;
        document.body.appendChild(g);
        adding = { what: what, ghost: g, x: e.clientX, y: e.clientY, moved: !!what.from };
        if (what.from) showBin(true);
        e.preventDefault();
      }
      function onAddMove(e) {
        if (!adding) return;
        if (!adding.moved && Math.abs(e.clientX - adding.x) + Math.abs(e.clientY - adding.y) > 6) { adding.moved = true; adding.ghost.hidden = false; showBin(true); }
        adding.ghost.style.left = e.clientX + 'px'; adding.ghost.style.top = e.clientY + 'px';
        if (bin) bin.classList.toggle('hot', overBin(e));
      }
      // A drop on the room places the piece or decoration there; a press with no
      // drag places it in the middle (keyboard and touch friendly); the bin removes.
      function onAddUp(e) {
        if (!adding) return;
        var a = adding, w = a.what; adding = null; a.ghost.classList.add('drop'); setTimeout(function () { a.ghost.remove(); }, 200);
        var binned = overBin(e); showBin(false);
        if (w.from) S.decor = S.decor.filter(function (d) { return d !== w.from; });
        if (binned) { if (w.from) { draw(); save(); status('Removed. Save room to keep it.'); } return; }
        var r = svg.getBoundingClientRect(), inRoom = e.clientX >= r.left && e.clientX <= r.right && e.clientY >= r.top && e.clientY <= r.bottom;
        if (a.moved && !inRoom && !w.from) return;
        var q = a.moved && inRoom;
        if (w.kind) {
          var N = room.size(), f = (q && floorFrom(e)) || [N / 2, N / 2];
          if (!room.place(w.kind, f[0], f[1])) { status('No space there for that. Try another spot.'); return; }
        } else {
          var sp = q ? spotFrom(e) : freeSpot();
          if (!sp) { if (w.from) S.decor.push(w.from); status(q ? 'Drop decorations on a shelf, table or wall spot.' : 'Every spot is taken.'); draw(); return; }
          S.decor = S.decor.filter(function (d) { return !(d.piece === sp.piece && d.spot === sp.spot); });
          S.decor.push({ piece: sp.piece, spot: sp.spot, icon: w.icon });
        }
        draw(); save();
      }
      function nearestSpot(q) {
        var best = null, bd = 30 * 30;
        (view.anchors || []).forEach(function (s) { var dx = s.pt[0] - q[0], dy = s.pt[1] - q[1], d = dx * dx + dy * dy; if (d < bd) { bd = d; best = s; } });
        return best;
      }
      function freeSpot() {
        var taken = {}; S.decor.forEach(function (d) { taken[d.piece + ':' + d.spot] = 1; });
        return (view.anchors || []).filter(function (s) { return !taken[s.piece + ':' + s.spot]; })[0] || null;
      }
      function onPanelDown(e) {
        var t = e.target.closest('[data-addk],[data-addd]'); if (!t || S.mode !== 'arr') return;
        startAdd(e, t.dataset.addk ? { kind: t.dataset.addk } : { icon: t.dataset.addd });
      }
      function onSvgClick(e) {
        var g = e.target.closest('.it'); if (!g) return;
        useItem(g);
      }
      function onSvgKey(e) { var g = e.target.closest('.it'); if (g && (e.key === 'Enter' || e.key === ' ')) { e.preventDefault(); useItem(g); } }
      function onSvgOver(e) { var g = e.target.closest('.it'); if (g) showTip(g); }
      function onSvgOut(e) { if (e.target.closest('.it')) tip.classList.remove('on'); }
      function useItem(g) {
        var i = +g.getAttribute('data-i'), it = S.its[i]; if (!it) return;
        if (S.mode === 'arr') { openPicker(i, g); return; }
        // Shop mode: show the item in the wares list.
        setOpen(true); tab = 'All'; renderList();
        var row = root.querySelector('[data-row="' + it.id + '"]');
        if (row) { row.classList.add('hl'); row.scrollIntoView({ block: 'nearest' }); setTimeout(function () { row.classList.remove('hl'); }, 1600); }
      }
      function openPicker(i, g) {
        picking = i;
        var it = S.its[i], r = (g.querySelector('.srp-cd') || g).getBoundingClientRect();
        pick.innerHTML = '<h4></h4><div class="g">' + Object.keys(ICONS).map(function (n) { return '<button type="button" data-pk="' + n + '" aria-label="' + n + '" aria-pressed="' + (n === it.ic) + '">' + iconSvg(n, ICONS) + '</button>'; }).join('') + '</div>' +
          '<div class="c">' + Object.keys(room.MAT).map(function (m) { return '<button type="button" class="sw" data-pc="' + m + '" aria-label="' + m + '" aria-pressed="' + (m === it.mat) + '" style="background:' + room.MAT[m] + '"></button>'; }).join('') + '<button type="button" class="rs" data-reset="1">Reset</button></div>' +
          '<p>Only the look in this room changes. The item’s name, price and stock stay as they are.</p>';
        pick.querySelector('h4').textContent = 'Look of ' + it.n;
        pick.hidden = false; tip.classList.remove('on');
        var x = Math.min(innerWidth - pick.offsetWidth - 8, Math.max(8, r.left + r.width / 2 - pick.offsetWidth / 2)), y = r.bottom + 8;
        if (y + pick.offsetHeight > innerHeight - 8) y = Math.max(8, r.top - pick.offsetHeight - 8);
        pick.style.left = x + 'px'; pick.style.top = y + 'px';
      }
      function closePicker() { picking = null; pick.hidden = true; }
      function onPickClick(e) {
        var b = e.target.closest('button'); if (!b || picking === null) return;
        var it = S.its[picking], k = String(it.id), o = S.ov[k] || {};
        if (b.dataset.pk) o.icon = b.dataset.pk;
        else if (b.dataset.pc) o.color = b.dataset.pc;
        if (b.dataset.reset) delete S.ov[k]; else S.ov[k] = o;
        draw(); renderList(); save();
        var g = (paperOn() ? pel : svg).querySelector('.it[data-i="' + picking + '"]'); if (g) openPicker(picking, g);
      }
      function onDocDown(e) { if (picking !== null && !pick.contains(e.target) && !e.target.closest('.it')) closePicker(); }
      function onDocKey(e) { if (e.key === 'Escape') { if (picking !== null) { closePicker(); return; } say.classList.remove('on'); tryLeave(); } }
      function onScroll() { tip.classList.remove('on'); say.classList.remove('on'); }

      function onKeeperDown(e) {
        if (S.mode !== 'arr') return;
        kd = { r: scene.getBoundingClientRect(), moved: false }; keeper.setPointerCapture(e.pointerId); e.preventDefault();
      }
      function onKeeperMove(e) {
        if (!kd) return;
        kd.moved = true;
        S.portrait = [Math.max(4, Math.min(96, (e.clientX - kd.r.left) / kd.r.width * 100)), Math.max(6, Math.min(90, (e.clientY - kd.r.top) / kd.r.height * 100))];
        S.portrait = [Math.round(S.portrait[0] * 10) / 10, Math.round(S.portrait[1] * 10) / 10];
        placeKeeper();
      }
      function onKeeperUp() { if (!kd) return; var moved = kd.moved; kd = null; if (moved) save(); }
      function onKeeperClick() { if (S.mode === 'arr') return; setOpen(true); talk(); }

      // ---- Owner panel: the generator and the keeper's lines ----
      function seg(key, opts) { return '<span class="shr-seg" role="group">' + opts.map(function (o) { return '<button type="button" data-k="' + key + '" data-v="' + o[0] + '" aria-pressed="' + (S[key] === o[0]) + '">' + o[1] + '</button>'; }).join('') + '</span>'; }
      // One generator row: a part of the room with its keep pin and reroll dice.
      function genRow(part, icon, name, sub) {
        var k = partKeep[part];
        return '<div class="shr-gr' + (k ? ' kept' : '') + '"><i class="fa-solid ' + icon + '" aria-hidden="true"></i><span><b>' + name + '</b><small data-sub="' + part + '">' + esc(sub) + '</small></span><span>' +
          '<button type="button" class="shr-ib" data-pin="' + part + '" aria-pressed="' + k + '" title="' + (k ? 'Kept' : 'Keep') + '" aria-label="Keep ' + name + '"><i class="fa-solid fa-thumbtack" aria-hidden="true"></i></button>' +
          '<button type="button" class="shr-ib" data-roll="' + part + '" title="Reroll" aria-label="Reroll ' + name + '"' + (k ? ' disabled' : '') + '><i class="fa-solid fa-dice" aria-hidden="true"></i></button></span></div>';
      }
      // A tile's picture: mood furniture uses its paper drawing, so the tile looks like what lands in the room.
      function kindArt(k) {
        var P = window.ShopRoomPaper, h = k === 'circle' ? 40 : Math.max(26, room.height(k) * 18);
        if (MOOD_KIND[k] && P && P.KIND && P.KIND[k]) return '<svg class="shr-mart" viewBox="-4 -4 ' + (48) + ' ' + (h + 8) + '" aria-hidden="true">' + P.KIND[k](40, h, { wood: '#a07a55', acc: room.accent(), wall: '#e6dfd3' }) + '</svg>';
        return '<i class="fa-solid ' + (KIND_ICON[k] || 'fa-cube') + '" aria-hidden="true"></i>';
      }
      function efxNow() { return S.efx || moodEfx(S.mood); }
      // One folding section: header button with a one-line summary, body measured when it opens.
      function sec(id, icon, name, sum, body) {
        var o = !!secOpen[id];
        return '<section class="shr-sec"><button type="button" class="shr-sh" data-sec="' + id + '" aria-expanded="' + o + '"><i class="fa-solid ' + icon + '" aria-hidden="true"></i><b>' + name + '</b><small>' + esc(sum) + '</small><i class="fa-solid fa-chevron-down chev" aria-hidden="true"></i></button>' +
          '<div class="shr-sb"' + (o ? '' : ' hidden') + '><div class="shr-sbi">' + body + '</div></div></section>';
      }
      function addTiles() {
        var tile = function (k) { return '<button type="button" class="shr-addt" data-addk="' + k + '" aria-label="Add ' + esc(KIND_LABEL[k] || k) + '">' + kindArt(k) + esc(KIND_LABEL[k] || k) + '</button>'; };
        var dtile = function (n) { return '<button type="button" class="shr-addt" data-addd="' + n + '" title="' + esc(n.replace(/-/g, ' ')) + '" aria-label="Add ' + esc(n.replace(/-/g, ' ')) + '">' + iconSvg(n, ICONS) + '</button>'; };
        var sub = function (t) { return '<span class="shr-sub2">' + t + '</span>'; }, M = MOODS[S.mood];
        if (addTab === 'm') {
          var mine = Object.keys(MOOD_KIND).filter(function (k) { return MOOD_KIND[k] === S.mood; }), rest = Object.keys(MOOD_KIND).filter(function (k) { return MOOD_KIND[k] !== S.mood; });
          return (mine.length ? sub(esc(M.name)) + mine.map(tile).join('') + sub('From other moods') : '') + rest.map(tile).join('');
        }
        if (addTab === 'd') {
          var md = M && M.deco ? M.deco.filter(function (n) { return ICONS[n]; }) : [];
          return (md.length ? sub(esc(M.name)) + md.map(dtile).join('') + sub('Everything else') : '') + room.DECOS.filter(function (n) { return md.indexOf(n) < 0; }).map(dtile).join('');
        }
        return room.KINDS.filter(function (k) { return !MOOD_KIND[k]; }).map(tile).join('');
      }
      function renderPanel() {
        if (!panel) return;
        var pins = S.pieces.filter(function (p) { return p.pinned; }).length, mood = MOODS[S.mood] ? S.mood : 'cozy', M = MOODS[mood], e = efxNow();
        var typeName = (ROOM_TYPES.filter(function (t) { return t[0] === S.roomType; })[0] || [0, ''])[1];
        var room1 = (window.ShopRoomPaper ? '<div class="shr-field"><span>Look</span>' + seg('look', [['lit', 'Lit room'], ['paper', 'Paper']]) + '</div>' : '') +
          '<label class="shr-field"><span>Shop type</span><select data-sel="roomType">' + ROOM_TYPES.map(function (t) { return '<option value="' + t[0] + '"' + (t[0] === S.roomType ? ' selected' : '') + '>' + t[1] + '</option>'; }).join('') + '</select></label>' +
          '<label class="shr-field"><span>Setting</span><select data-sel="setting">' + room.SETTINGS.map(function (t) { return '<option value="' + t[0] + '"' + (t[0] === S.setting ? ' selected' : '') + '>' + t[1] + '</option>'; }).join('') + '</select></label>' +
          '<div class="shr-field"><span>Room size</span>' + seg('size', [['s', 'Small'], ['m', 'Medium'], ['l', 'Large']]) + '</div>' +
          '<div class="shr-field"><span>Furniture</span>' + seg('full', [['sparse', 'Sparse'], ['normal', 'Normal'], ['packed', 'Packed']]) + '</div>' +
          '<div class="shr-field"><span>Decorations</span>' + seg('deco', [['none', 'None'], ['some', 'Some'], ['lots', 'Lots']]) + '</div>';
        var mood1 = '<div class="shr-moods">' + MOOD_ORDER.map(function (m) { var x = MOODS[m]; return '<button type="button" class="shr-mood" data-mood="' + m + '" aria-pressed="' + (m === mood) + '"><span class="sw">' + x.sw.map(function (c) { return '<i style="background:' + c + '"></i>'; }).join('') + '</span><b title="' + esc(x.name) + '">' + esc(x.short || x.name) + '</b></button>'; }).join('') + '</div>' +
          (M.pal ? '<p class="shr-note">The mood sets the colours, walls, floor and light, and brings its own furniture. Anything you moved by hand stays put.</p>'
            : '<div class="shr-field"><span>Colours</span><span class="shr-chips">' + room.PALS.map(function (p) { return '<button type="button" class="shr-chip" data-k="pal" data-v="' + p[0] + '" aria-pressed="' + (S.pal === p[0]) + '"><span class="shr-sw" style="background:' + p[2] + '"></span>' + p[1] + '</button>'; }).join('') + '</span></div>');
        var add1 = '<span class="shr-seg" role="group" style="display:grid"><button type="button" data-addtab="m" aria-pressed="' + (addTab === 'm') + '">Mood pieces</button><button type="button" data-addtab="f" aria-pressed="' + (addTab === 'f') + '">Furniture</button><button type="button" data-addtab="d" aria-pressed="' + (addTab === 'd') + '">Decorations</button></span>' +
          '<div class="shr-addl' + (addTab === 'd' ? ' dec' : '') + '">' + addTiles() + '</div>' +
          '<small class="shr-hint">' + (addTab === 'd' ? 'Drop a decoration on a shelf, table or wall spot.' : 'Drag a piece onto the floor.') + ' Drag anything in the room onto the bin to remove it.</small>';
        var rng = function (k, label) { return '<label class="shr-rng"><span>' + label + '</span><input type="range" min="0" max="250" step="5" data-efx="' + k + '" value="' + e[k] + '"><output>' + e[k] + '%</output></label>'; };
        var tg = function (k, label) { return '<button type="button" data-eft="' + k + '" aria-pressed="' + !!e[k] + '">' + label + '</button>'; };
        var es = function (k, opts) { return '<span class="shr-seg" role="group" style="display:grid">' + opts.map(function (o) { return '<button type="button" data-efs="' + k + '" data-v="' + o[0] + '" aria-pressed="' + (e[k] === o[0]) + '">' + o[1] + '</button>'; }).join('') + '</span>'; };
        var fine1 = rng('shadows', 'Shadows') + rng('warmth', 'Lamp glow') + rng('window', 'Window light') + rng('haze', 'Haze') + rng('vignette', 'Dark corners') +
          '<div class="shr-tgl" role="group" aria-label="Living details">' + tg('dust', 'Dust') + tg('flicker', 'Flicker') + tg('embers', 'Embers') + tg('smoke', 'Smoke') + '</div>' +
          '<div class="shr-field"><span>Weather</span>' + es('weather', [['', 'Clear'], ['rain', 'Rain'], ['snow', 'Snow']]) + '</div>' +
          '<div class="shr-field"><span>Time of day</span>' + es('time', [['', 'Day'], ['dawn', 'Dawn'], ['dusk', 'Dusk'], ['night', 'Night']]) + '</div>' +
          (S.efx ? '<button type="button" class="shr-link" data-efxreset="1">Back to ' + esc(M.name) + '’s settings</button>' : '');
        var gen1 = '<div class="shr-gh"><span>Reroll</span><button type="button" class="shr-rest" data-roll="rest" title="Keeps every part you pinned and everything you moved by hand"><i class="fa-solid fa-dice" aria-hidden="true"></i> Reroll the rest</button></div>' +
          genRow('room', 'fa-chair', 'Furniture', S.pieces.length + (S.pieces.length === 1 ? ' piece' : ' pieces') + (pins ? ' · ' + pins + ' moved by you' : '')) +
          genRow('goods', 'fa-coins', 'Goods', S.its.length + (S.its.length === 1 ? ' item' : ' items') + ' on show') +
          genRow('deco', 'fa-wand-magic-sparkles', 'Decorations', { none: 'None', some: 'Some', lots: 'Lots' }[S.deco] || '');
        var fineSum = S.efx ? 'Changed by you' : 'As the mood sets it';
        panel.innerHTML = '<div class="shr-ph"><h3>Arrange this room</h3><button type="button" class="shr-x" data-leave="1" aria-label="Close">✕</button></div><div class="shr-pb">' +
          sec('room', 'fa-house', 'Room', typeName + ' · ' + (S.look === 'paper' ? 'Paper' : 'Lit room'), room1) +
          sec('mood', 'fa-masks-theater', 'Mood', M.name, mood1) +
          sec('add', 'fa-plus', 'Add pieces', 'Drag into the room', add1) +
          sec('fine', 'fa-sliders', 'Fine-tune', fineSum, fine1) +
          sec('gen', 'fa-dice', 'Generate', 'Reroll parts', gen1) +
          sec('keeper', 'fa-comment', 'Keeper', S.lines.length ? S.lines.length + (S.lines.length === 1 ? ' line' : ' lines') : 'No lines yet', '<label class="shr-field"><span>What the keeper says (one line each)</span><textarea rows="3" data-lines="1" maxlength="2200"></textarea></label>') +
          '<span class="shr-status" aria-live="polite">' + (S.portrait ? 'Portrait placed by hand.' : '') + '</span></div>' +
          '<div class="shr-warn" role="alert"><i class="fa-solid fa-triangle-exclamation" aria-hidden="true"></i><span>Not saved yet</span><button type="button" data-discard="1">Discard</button></div>' +
          '<div class="shr-pf"><button type="button" data-discard="1">Cancel</button><button type="button" class="shr-save" data-saveroom="1"' + (dirty ? '' : ' disabled') + '>Save room</button></div>';
        panel.querySelector('[data-lines]').value = S.lines.join('\n');
      }
      // Fold: animate to the body's real height, then let it size itself; Calm and Off open at once.
      function fold(id) {
        secOpen[id] = !secOpen[id];
        var b = panel.querySelector('[data-sec="' + id + '"]'), body = b.nextElementSibling, open = secOpen[id], mo = document.documentElement.getAttribute('data-motion');
        b.setAttribute('aria-expanded', open);
        if (mo === 'calm' || mo === 'off' || !body.animate) { body.hidden = !open; return; }
        body.hidden = false;
        var h = body.scrollHeight, an = body.animate({ height: open ? ['0px', h + 'px'] : [h + 'px', '0px'] }, { duration: 200, easing: 'cubic-bezier(.16,1,.3,1)' });
        an.onfinish = function () { if (!secOpen[id]) body.hidden = true; };
      }
      // Rolls one part of the room; the furniture rebuilds around pieces moved by hand.
      function roll(part) {
        S.seeds[part]++;
        if (part === 'room') room.generate();
      }
      function onRootClick(e) {
        var b = e.target.closest('button'); if (!b) return;
        if (b.dataset.enter) { enter(); return; }
        if (b.dataset.addtab) { addTab = b.dataset.addtab; renderPanel(); return; }
        if (b.dataset.sec) { fold(b.dataset.sec); return; }
        if (b.dataset.mode) { if (b.dataset.mode === 'shop') tryLeave(); else if (S.mode !== 'arr') { S.mode = 'arr'; setMode(); } return; }
        if (b.dataset.leave) { tryLeave(); return; }
        if (b.dataset.discard) { discard(); return; }
        if (b.dataset.saveroom) { saveRoom(); return; }
        if (b.dataset.fx) { S.fx = b.dataset.fx; saveFx(S.fx); root.classList.toggle('fxlight', S.fx !== 'full'); draw(); renderList(); return; }
        if (b.dataset.tab) { tab = b.dataset.tab; renderList(); return; }
        if (b.dataset.add) { basket[b.dataset.add] = (basket[b.dataset.add] || 0) + 1; note = ''; renderList(); return; }
        if (b.dataset.empty) { basket = {}; note = ''; renderList(); return; }
        if (b.dataset.buy) { doBuy(); return; }
        if (b.classList.contains('shr-wbtn')) { setOpen(!open); return; }
        if (b === keeper) { onKeeperClick(); return; }
        if (!canArrange) return;
        var k = b.dataset.k;
        // A new mood starts from its own effects and swaps in its furniture.
        if (b.dataset.mood) { S.mood = b.dataset.mood; S.efx = null; room.applyMood(); draw(); renderPanel(); save(); return; }
        if (b.dataset.eft) { S.efx = efxNow(); S.efx[b.dataset.eft] = !S.efx[b.dataset.eft]; draw(); renderPanel(); save(); return; }
        if (b.dataset.efs) { S.efx = efxNow(); S.efx[b.dataset.efs] = b.dataset.v; draw(); renderPanel(); save(); return; }
        if (b.dataset.efxreset) { S.efx = null; draw(); renderPanel(); save(); return; }
        if (k === 'pal' || k === 'deco' || k === 'look') { S[k] = b.dataset.v; draw(); }
        else if (k === 'size' || k === 'full') { S[k] = b.dataset.v; room.generate(); draw(); }
        else if (b.dataset.pin) { partKeep[b.dataset.pin] = !partKeep[b.dataset.pin]; renderPanel(); if (dirty) save(); return; }
        else if (b.dataset.roll === 'rest') { ['room', 'goods', 'deco'].forEach(function (p) { if (!partKeep[p]) roll(p); }); draw(); }
        else if (b.dataset.roll) { roll(b.dataset.roll); draw(); }
        else return;
        renderPanel(); save();
        var rolled = b.dataset.roll === 'rest' ? ['room', 'goods', 'deco'].filter(function (p) { return !partKeep[p]; }) : b.dataset.roll ? [b.dataset.roll] : [];
        rolled.forEach(function (p) { var sub = panel && panel.querySelector('[data-sub="' + p + '"]'); if (sub) sub.classList.add('roll'); });
      }
      // Sliders redraw on the next frame while dragging; the summary updates once they let go.
      function onRootInput(e) {
        var t = e.target; if (!t.dataset.efx) return;
        S.efx = efxNow(); S.efx[t.dataset.efx] = +t.value; t.nextElementSibling.textContent = t.value + '%';
        later(); save();
      }
      function onRootChange(e) {
        var t = e.target;
        if (t.dataset.efx) { renderPanel(); return; }
        if (t.dataset.payer) { payer = t.value; note = ''; renderBasket(); return; }
        if (t.dataset.sel) { S[t.dataset.sel] = t.value; if (t.dataset.sel === 'setting') S.portrait = null; room.generate(); draw(); renderPanel(); save(); }
        else if (t.dataset.lines) { S.lines = t.value.split('\n').map(function (s) { return s.trim().slice(0, 200); }).filter(Boolean).slice(0, 10); save(); }
      }
      function setMode() {
        root.classList.toggle('arr', S.mode === 'arr'); if (S.mode === 'arr') tilt(0, 0);
        root.querySelectorAll('[data-mode]').forEach(function (b) { b.setAttribute('aria-pressed', b.dataset.mode === S.mode); });
        if (canArrange && !panel) { panel = document.createElement('aside'); panel.className = 'shr-panel'; panel.setAttribute('aria-label', 'Arrange this room'); scene.appendChild(panel); panel.addEventListener('animationend', function () { panel.classList.remove('nag'); }); panel.addEventListener('pointerdown', onPanelDown); }
        if (panel) {
          renderPanel();
          // Next frame, so the slide plays from its closed position.
          var on = S.mode === 'arr';
          requestAnimationFrame(function () { panel.classList.toggle('on', on); if (on) { var f = panel.querySelector('select'); if (f) f.focus({ preventScroll: true }); } });
        }
        closePicker(); draw();
      }
      // ---- Opening and closing ----
      var front = root.querySelector('.shr-front');
      var hole = root.querySelector('.shr-hole'), dark = root.querySelector('.shr-dark');
      var walkT = 0;
      // Opening: the door swings open in place, then the room is revealed
      // as its dark doorway grows to fill the widget.
      function enter() {
        if (!root.classList.contains('closed') || front.classList.contains('opening')) return;
        front.setAttribute('aria-expanded', 'true');
        if (matchMedia('(prefers-reduced-motion: reduce)').matches) { root.classList.remove('closed'); draw(); alive(true); return; }
        front.classList.add('opening');
        walkT = setTimeout(walkIn, 700);
      }
      function walkIn() {
        var dr = front.querySelector('.shr-dr').getBoundingClientRect(), r0 = root.getBoundingClientRect();
        // The doorway's opening inside the door drawing's 64 by 96 box.
        var w = dr.width * 52 / 64, h = dr.height * 86 / 96;
        hole.style.left = (dr.left - r0.left + dr.width * 6 / 64) + 'px';
        hole.style.top = (dr.top - r0.top + dr.height * 8 / 96) + 'px';
        hole.style.width = w + 'px'; hole.style.height = h + 'px';
        hole.style.transformOrigin = '50% 60%'; hole.style.transform = 'scale(1)'; hole.style.display = 'block';
        root.classList.add('walking');
        var k = 2.2 * Math.hypot(r0.width, r0.height) / Math.min(w, h);
        void hole.offsetWidth;
        hole.classList.add('cover'); hole.style.transform = 'scale(' + k + ')';
        walkT = setTimeout(function () {
          dark.classList.add('on');
          root.classList.add('cold'); root.classList.remove('closed'); draw(); alive(true);
          hole.classList.remove('cover'); hole.style.display = 'none';
          requestAnimationFrame(function () { requestAnimationFrame(function () {
            dark.classList.add('fade'); dark.classList.remove('on'); root.classList.remove('cold');
            var book = pel.querySelector('.srp'); if (book) book.classList.remove('shut');
            walkT = setTimeout(settle, 850);
          }); });
        }, 620);
      }
      function settle() {
        root.classList.remove('walking', 'cold'); front.classList.remove('opening');
        hole.classList.remove('cover'); hole.style.display = 'none'; dark.className = 'shr-dark';
        placeKeeper();
      }
      // Looping details follow MotionRest: they slow with it and pause once it is still.
      var aliveT = 0;
      function alive(on) {
        clearInterval(aliveT); aliveT = 0;
        if (!on || !svg.getAnimations) return;
        aliveT = setInterval(function () {
          var MR = window.MotionRest, sp = MR ? MR.speed() : 1;
          svg.getAnimations({ subtree: true }).concat(pel.getAnimations({ subtree: true })).forEach(function (a) {
            if (sp === 0) { if (a.playState === 'running') a.pause(); return; }
            a.playbackRate = Math.max(.02, sp); if (a.playState === 'paused') a.play();
          });
        }, 250);
      }
      // A few degrees of tilt toward the mouse gives the room depth. Not while
      // arranging, where drags map straight onto the floor, and not on touch.
      function tilt(x, y) {
        var book = pel.querySelector('.srp-tilt');
        if (book) book.style.transform = x || y ? 'rotateX(' + (-y * 5).toFixed(2) + 'deg) rotateY(' + (x * 8).toFixed(2) + 'deg)' : '';
        svg.style.transform = x || y ? 'perspective(1400px) rotateX(' + (-y * 4).toFixed(2) + 'deg) rotateY(' + (x * 6).toFixed(2) + 'deg)' : '';
      }
      function onTilt(e) {
        if (e.pointerType !== 'mouse' || S.mode === 'arr' || S.fx !== 'full' || root.classList.contains('closed') || matchMedia('(prefers-reduced-motion: reduce)').matches) return;
        var r = scene.getBoundingClientRect();
        tilt((e.clientX - r.left) / r.width - .5, (e.clientY - r.top) / r.height - .5);
      }
      scene.addEventListener('pointermove', onTilt);
      scene.addEventListener('pointerleave', function () { tilt(0, 0); });
      function leave() {
        clearTimeout(walkT); settle(); alive(false); tilt(0, 0);
        root.classList.add('closed'); front.setAttribute('aria-expanded', 'false');
        setOpen(false);
      }
      // A press outside the open shop closes it, unless the basket still has
      // something in it; arranging has its own leave rule below.
      function onOutsideShop(e) {
        if (root.classList.contains('closed') || S.mode === 'arr') return;
        if (root.contains(e.target) || pick.contains(e.target) || say.contains(e.target) || tip.contains(e.target)) return;
        if (outsideClose(basket) === 'close') { leave(); return; }
        note = 'Your basket still has items. Buy or empty it before you leave.'; renderBasket();
        var bk = root.querySelector('.shr-bk'); bk.classList.remove('shr-stay'); void bk.offsetWidth; bk.classList.add('shr-stay');
      }

      // A press outside the whole room while arranging counts as leaving it;
      // the room itself (dragging furniture) and the item picker do not.
      function onOutsideDown(e) {
        if (S.mode !== 'arr' || root.contains(e.target) || pick.contains(e.target)) return;
        tryLeave();
      }

      // Redraw when the site switches between light and dark.
      var themeObs = new MutationObserver(function () { var d = document.documentElement.classList.contains('dark'); if (d !== S.dark) draw(); });
      themeObs.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] });

      svg.addEventListener('pointerdown', onSvgDown); svg.addEventListener('pointermove', onSvgMove);
      svg.addEventListener('pointerup', onSvgUp); svg.addEventListener('pointercancel', onSvgUp);
      svg.addEventListener('click', onSvgClick); svg.addEventListener('keydown', onSvgKey);
      svg.addEventListener('pointerover', onSvgOver); svg.addEventListener('pointerout', onSvgOut);
      keeper.addEventListener('pointerdown', onKeeperDown); keeper.addEventListener('pointermove', onKeeperMove);
      keeper.addEventListener('pointerup', onKeeperUp); keeper.addEventListener('pointercancel', onKeeperUp);
      root.addEventListener('click', onRootClick); root.addEventListener('change', onRootChange); root.addEventListener('input', onRootInput);
      pick.addEventListener('click', onPickClick);
      document.addEventListener('pointerdown', onDocDown);
      // The shop's outside check runs before the arranging one, so a single
      // press outside while arranging leaves Arrange without closing the shop.
      document.addEventListener('pointerdown', onOutsideShop);
      document.addEventListener('pointerdown', onOutsideDown);
      document.addEventListener('pointermove', onAddMove); document.addEventListener('pointerup', onAddUp);
      svg.addEventListener('pointermove', onSvgMoveBin);
      pel.addEventListener('pointerdown', onPaperDown); pel.addEventListener('pointermove', onPaperMove);
      pel.addEventListener('pointerup', onSvgUp); pel.addEventListener('pointercancel', onSvgUp);
      pel.addEventListener('click', onPaperClick); pel.addEventListener('keydown', onPaperKey);
      var resizeObs = window.ResizeObserver ? new ResizeObserver(function () { if (paperOn()) window.ShopRoomPaper.fit(pel); }) : null;
      if (resizeObs) resizeObs.observe(scene);
      document.addEventListener('keydown', onDocKey);
      window.addEventListener('scroll', onScroll, { passive: true });

      el._shopRoom = {
        onDocDown: onDocDown, onDocKey: onDocKey,
        cleanup: function () {
          clearInterval(typeT); clearInterval(aliveT); clearTimeout(sayT); clearTimeout(walkT); if (raf) cancelAnimationFrame(raf);
          themeObs.disconnect(); if (resizeObs) resizeObs.disconnect();
          window.removeEventListener('scroll', onScroll);
          document.removeEventListener('pointerdown', onDocDown); document.removeEventListener('pointerdown', onOutsideDown); document.removeEventListener('pointerdown', onOutsideShop); document.removeEventListener('pointermove', onAddMove); document.removeEventListener('pointerup', onAddUp); if (adding) adding.ghost.remove(); document.removeEventListener('keydown', onDocKey);
          tip.remove(); say.remove(); pick.remove();
        }
      };

      // ---- Load the saved room and the shop's goods ----
      // The room only shows when the armory add-on answers; with the add-on off
      // the endpoint is a 404 and the shop page keeps just its inventory list.
      Promise.all([
        Chronicle.apiFetch(ds.roomEndpoint).then(function (r) { if (!r.ok) throw new Error('room ' + r.status); return r.json(); }),
        loadGoods(),
        loadBuyers()
      ]).then(function (res) {
        if (fromLayout(S, res[0] && res[0].layout)) room.geometry(); else room.generate();
        savedLayout = toLayout(S);
        el.hidden = false;
        draw(); renderList();
      }).catch(function () { el.hidden = true; });
    },

    destroy: function (el) {
      var r = el._shopRoom;
      if (r) {
        document.removeEventListener('pointerdown', r.onDocDown);
        document.removeEventListener('keydown', r.onDocKey);
        r.cleanup(); delete el._shopRoom;
      }
      el.innerHTML = '';
    }
  });
})();
