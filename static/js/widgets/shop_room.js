/**
 * shop_room.js -- Chronicle Shop Room Widget
 *
 * Draws a shop as an isometric room: furniture, the shop's goods on shelves
 * and tables, the keeper's portrait, light and shadow. The wares list sits
 * under the room. Owners get Arrange: drag furniture and the portrait, change
 * an item's icon and colour, and run the room generator. The layout is saved
 * per shop; the goods themselves are the shop's "sells" relations, so stock and
 * prices always come from the inventory, never from the room.
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
  function esc(s) {
    return String(s == null ? '' : s).replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/'/g, '&#39;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }

  // createRoom builds the drawing engine for one room. S is the room state the
  // widget owns; the engine reads it and draws, and generate() rewrites
  // S.pieces. Nothing here touches the page.
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
      var p = PAL[S.pal] || PAL.oak, f = S.dark ? .32 : 0;
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
      counter: [2, 1], table: [1.5, 1], barrel: [.8, .8], crate: [.8, .8], anvil: [.8, .6], glass: [1.4, .8], stall: [2.2, 1], rug: [3, 2], pedestal: [.6, .6], lamp: [.4, .4], sack: [.6, .6] };
    var WALLK = { shelf: 1, rack: 1, cabinet: 1, bookcase: 1, forge: 1, window: 1, herbs: 1 };
    // off is how far the piece stands out from y=0 (or x=0): zero on a straight wall, more where a curved wall bends away.
    function wb(p, u0, v0, z0, lu, lv, lz, c, o) { var f = p.off || 0; return p.wall === 'Y' ? box(p.x + u0, f + v0, z0, lu, lv, lz, c, o) : box(f + v0, p.y + u0, z0, lv, lu, lz, c, o); }
    function wpt(p, u, v, z) { var f = p.off || 0; return p.wall === 'Y' ? [p.x + u, f + v, z] : [f + v, p.y + u, z]; }
    function wf(p, u0, dv, z0) { var f = p.off || 0; return function (u, v) { return p.wall === 'Y' ? P(p.x + u0 + u, f + dv, z0 + v) : P(f + dv, p.y + u0 + u, z0 + v); }; }
    // Shadows are drawn together in one blurred layer under the furniture (see castShadows), which is far cheaper than blurring each one.
    function shadow() { return ''; }
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
        p.light = f(1.2, .5);
        a.push(A.apply(null, wpt(p, .25, .7, 1.5).concat([{}]))); a.push(A.apply(null, wpt(p, L - .25, .7, 1.5).concat([{}])));
        return { pre: s, post: '', a: a };
      },
      window: function (p) {
        var L = p.len, f = wf(p, 0, .01, 1.1), s = '', fr = dk(C.wood, .2);
        s += poly([f(-.08, -.08), f(L + .08, -.08), f(L + .08, 1.3), f(-.08, 1.3)], fr);
        s += poly([f(0, 0), f(L, 0), f(L, 1.22), f(0, 1.22)], 'url(#gSky)');
        s += line(f(L / 2, 0), f(L / 2, 1.22), fr, 3) + line(f(0, .61), f(L, .61), fr, 3);
        s += wb(p, -.1, 0, 1.0, L + .2, .22, .1, lt(C.wood, .1));
        // A pool of daylight on the floor in front of the window.
        var o = p.off || 0, g = p.wall === 'Y' ? [P(p.x, o + .2, 0), P(p.x + L, o + .2, 0), P(p.x + L + .8, o + 2.4, 0), P(p.x + .8, o + 2.4, 0)] : [P(o + .2, p.y, 0), P(o + .2, p.y + L, 0), P(o + 2.4, p.y + L + .8, 0), P(o + 2.4, p.y + .8, 0)];
        p.light = f(L / 2, .6); p.lightR = 90; p.cool = 1;
        p.floorLight = poly(g, 'rgba(255,240,200,.18)');
        // A faint shaft of daylight from the window down to that pool.
        p.shaft = poly(hull([f(0, 0), f(L, 0), f(L, 1.22), f(0, 1.22)].concat(g)), 'url(#gShaft)');
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
        p.light = [b[0], b[1] + 8];
        return { pre: s, post: '', a: [] };
      },
      sack: function (p) {
        var c = P(p.x + .3, p.y + .3, 0), s = shadow(p.x, p.y, .6, .6);
        s += '<path d="M' + (c[0] - 15) + ',' + c[1] + ' C' + (c[0] - 19) + ',' + (c[1] - 22) + ' ' + (c[0] - 8) + ',' + (c[1] - 30) + ' ' + c[0] + ',' + (c[1] - 32) + ' C' + (c[0] + 8) + ',' + (c[1] - 30) + ' ' + (c[0] + 19) + ',' + (c[1] - 22) + ' ' + (c[0] + 15) + ',' + c[1] + ' Q' + c[0] + ',' + (c[1] + 7) + ' ' + (c[0] - 15) + ',' + c[1] + 'Z" fill="url(#gSack)"/>';
        s += line([c[0] - 6, c[1] - 30], [c[0] + 6, c[1] - 30], '#5b4630', 2);
        return { pre: s, post: '', a: [] };
      }
    };

    function rng(seed) { return function () { seed |= 0; seed = seed + 0x6D2B79F5 | 0; var t = Math.imul(seed ^ seed >>> 15, 1 | seed); t = t + Math.imul(t ^ t >>> 7, 61 | t) ^ t; return ((t ^ t >>> 14) >>> 0) / 4294967296; }; }
    function shuffle(a, r) { a = a.slice(); for (var i = a.length - 1; i > 0; i--) { var j = Math.floor(r() * (i + 1)), t = a[i]; a[i] = a[j]; a[j] = t; } return a; }
    var SZ = { s: 7, m: 8, l: 10 };
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
      hourglass: '#d4a72c', moon: '#d6dde6', broom: '#a16207', 'bread-slice': '#f59e0b', 'apple-whole': '#dc2626', bell: '#d4a72c' };
    // Only these go on one wall: the forge's hood and the hanging herbs would clash with the shop sign on the other.
    var ONLY_Y = { forge: 1, herbs: 1 };
    var DEPTH = { shelf: .55, rack: .3, cabinet: .66, bookcase: .5, forge: 1.1, window: 0, herbs: 0 };
    var HT = { shelf: 2.6, rack: 2.1, cabinet: 1.85, bookcase: 2.7, forge: 2.9, window: 2.5, herbs: 3.1, counter: 1.15, table: 1, barrel: 1, crate: .8, anvil: .7, glass: 1.15, stall: 2.4, rug: .2, pedestal: 1.1, lamp: 2.3, sack: .7 };

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
        if (G.door && p.kind !== 'rug' && p.kind !== 'lamp' && over(thick(p), G.door.r, 0)) return true;
      }
      return others.some(function (o) {
        if (o === p || o.id === p.id) return false;
        var wa = WALLK[p.kind], wo = WALLK[o.kind];
        if (wa && wo) return sq ? p.wall === o.wall && over(p.wall === 'Y' ? [p.x, 0, p.len, 1] : [0, p.y, 1, p.len], p.wall === 'Y' ? [o.x, 0, o.len, 1] : [0, o.y, 1, o.len], .1) : over(thick(p), thick(o), .1);
        if (p.kind === 'rug' || o.kind === 'rug') return p.kind === o.kind && over(r, rect(o), .2);
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
    // Builds the whole room. Pieces the GM moved by hand are pinned and kept, and count toward the type's must-haves.
    function generate() {
      geometry();
      var N = SZ[S.size], rec = RECIPES[S.roomType], r = rng(S.seeds.room * 7919 + S.key * 131 + N);
      var placed = S.keep ? S.pieces.filter(function (p) { return p.pinned && (!WALLK[p.kind] || hug(p)) && !clashes(p, [], N); }) : [];
      nextId = placed.reduce(function (m, p) { return Math.max(m, p.id + 1); }, 1);
      var have = {}; placed.forEach(function (p) { have[p.kind] = (have[p.kind] || 0) + 1; });
      rec.need.forEach(function (k) { if (have[k]) have[k]--; else tryPlace(k, placed, N, r); });
      var n = { sparse: 2, normal: 5, packed: 9 }[S.full] + (N - 8) - (S.setting === 'room' ? 0 : 1);
      var pool = shuffle(rec.opt.concat(rec.opt), r);
      for (var i = 0; i < pool.length && n > 0; i++) if (tryPlace(pool[i], placed, N, r)) n--;
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
    function defs() {
      var rec = RECIPES[S.roomType];
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
        '<linearGradient id="gSky" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#cfe8ff"/><stop offset="1" stop-color="#f6efd8"/></linearGradient>' +
        '<radialGradient id="gLamp"><stop offset="0" stop-color="#fff7d1"/><stop offset="1" stop-color="#f0a83a"/></radialGradient>' +
        '<linearGradient id="gBarrel" x1="0" x2="1"><stop offset="0" stop-color="#5a3c22"/><stop offset=".35" stop-color="#9a6b3d"/><stop offset=".55" stop-color="#8a5d33"/><stop offset="1" stop-color="#4a3019"/></linearGradient>' +
        '<radialGradient id="gSack" cx="35%" cy="35%" r="75%"><stop offset="0" stop-color="#e3cfa4"/><stop offset="1" stop-color="#9c7d4f"/></radialGradient>' +
        '<radialGradient id="gGlow"><stop offset="0" stop-color="#ffb35c" stop-opacity=".62"/><stop offset=".5" stop-color="#ff9a3c" stop-opacity=".2"/><stop offset="1" stop-color="#ff9a3c" stop-opacity="0"/></radialGradient>' +
        '<radialGradient id="gMagic"><stop offset="0" stop-color="#c4b5fd" stop-opacity=".75"/><stop offset="1" stop-color="#8b5cf6" stop-opacity="0"/></radialGradient>' +
        '<radialGradient id="gVig" cx="50%" cy="45%" r="70%"><stop offset=".6" stop-color="#000" stop-opacity="0"/><stop offset="1" stop-color="#000" stop-opacity=".28"/></radialGradient></defs>';
    }

    // ---- The room ----
    function shell(N) {
      var s = '', rec = RECIPES[S.roomType];
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
              var nm = rec.deco[Math.floor(r() * rec.deco.length)]; if (!ICONS[nm]) continue; var g = ICONS[nm], h = 17, w = h * g[0] / 512, sl = sd[1] / sd[0];
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
      var ao = '', cast = '';
      G.edges.filter(function (e) { return e.back; }).forEach(function (e) { ao += line(P(e.a[0], e.a[1], 0), P(e.b[0], e.b[1], 0), 'rgba(0,0,0,.5)', 14); });
      if (S.setting === 'room') ao += line(P(0, 0, 0), P(0, 0, WH), 'rgba(0,0,0,.4)', 12);
      S.pieces.forEach(function (p) {
        if (p.kind === 'rug' || p.kind === 'lamp' || p.kind === 'window' || p.kind === 'herbs') return;
        var r = rect(p), h = HT[p.kind], m = .06;
        ao += poly([P(r[0] - m, r[1] - m), P(r[0] + r[2] + m, r[1] - m), P(r[0] + r[2] + m, r[1] + r[3] + m), P(r[0] - m, r[1] + r[3] + m)], 'rgba(0,0,0,.42)');
        if (WALLK[p.kind]) return;
        var dx = h * .5, dy = h * .16, cs = [[r[0], r[1]], [r[0] + r[2], r[1]], [r[0] + r[2], r[1] + r[3]], [r[0], r[1] + r[3]]];
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
        '<rect x="' + VB[0] + '" y="' + VB[1] + '" width="' + VB[2] + '" height="' + VB[3] + '" fill="' + (C.dark ? '#04040c' : '#121027') + '" opacity="' + (S.setting === 'cave' ? .64 : .52) + '" mask="url(#lm)" clip-path="url(#room)" style="pointer-events:none"/>';
    }
    var VB, keeperPt, GRAD = {};
    function gradDefs() { return '<defs>' + Object.keys(GRAD).map(function (id) { var c = GRAD[id]; return '<linearGradient id="' + id + '" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="' + lt(c, .55) + '"/><stop offset=".4" stop-color="' + lt(c, .1) + '"/><stop offset=".6" stop-color="' + c + '"/><stop offset="1" stop-color="' + dk(c, .42) + '"/></linearGradient>'; }).join('') + '</defs>'; }
    function draw() {
      colours();
      var N = SZ[S.size], rec = RECIPES[S.roomType], full = S.fx === 'full', sq = S.setting === 'room';
      VB = [-N * 32 - 30, -WH * 32 - 34, N * 64 + 60, N * 32 + WH * 32 + 34 + 26];
      GRAD = {};
      var level = { none: 0, some: .35, lots: .8 }[S.deco];
      // Draw each piece once to learn its anchor spots, then hand goods and decorations out to them.
      var out = S.pieces.slice().sort(function (a, b) { return a.id - b.id; }).map(function (p) { delete p.light; delete p.floorLight; delete p.keeperAt; delete p.cool; delete p.shaft; return { p: p, d: DRAW[p.kind](p), goods: [], decos: [] }; });
      var spots = [], hangs = [];
      out.forEach(function (o, oi) { o.d.a.forEach(function (a, ai) { (a.deco ? hangs : o.d.decoOnly ? hangs : spots).push([oi, ai]); }); });
      var rg = rng(S.seeds.goods * 4049 + S.key * 17), rd = rng(S.seeds.deco * 6151 + S.key * 29), order = shuffle(spots, rg);
      order.forEach(function (sp, k) { if (k < S.its.length) out[sp[0]].goods.push([sp[1], k]); });
      order.slice(S.its.length).concat(hangs).forEach(function (sp) { var a = out[sp[0]].d.a[sp[1]]; if (rd() < (a.hang ? level * 1.4 : level)) out[sp[0]].decos.push([sp[1], rec.deco[Math.floor(rd() * rec.deco.length)]]); });
      // Back-to-front by footprint centre; rugs lie under everything and lamps hang over it.
      function key(o) { var p = o.p, r = rect(p); return p.kind === 'rug' ? -99 : p.kind === 'lamp' ? 99 + r[0] + r[1] : r[0] + r[2] / 2 + r[1] + r[3] / 2; }
      out.sort(function (a, b) { return key(a) - key(b); });
      var lights = [], s = defs() + (sq ? shell(N) : shellShape(N)) + sign(N) + wallDecor(N, rec, rng(S.seeds.deco * 3301 + S.key * 7), level, function (l) { lights.push(l); });
      function piece(o) {
        var p = o.p, g = '<g class="piece' + (p.pinned ? ' pinned' : '') + '" data-p="' + p.id + '"><g class="body">' + o.d.pre + '</g>';
        o.decos.forEach(function (d) { g += decoSvg(o.d.a[d[0]], d[1]); });
        o.goods.forEach(function (gd) { g += itemSvg(o.d.a[gd[0]], gd[1]); });
        g += '<g class="body">' + o.d.post + '</g>';
        if (p.pinned && S.mode === 'arr') { var r = rect(p), q = P(r[0] + r[2] / 2, r[1] + r[3] / 2, HT[p.kind] + .25); g += '<g class="pinbtn" data-unpin="' + p.id + '" style="cursor:pointer"><title>Moved by hand. Click to unpin.</title><line x1="' + q[0] + '" y1="' + q[1] + '" x2="' + q[0] + '" y2="' + (q[1] + 10) + '" stroke="#333" stroke-width="2"/><circle cx="' + q[0] + '" cy="' + q[1] + '" r="6" fill="' + C.acc + '" stroke="#fff" stroke-width="1.5"/></g>'; }
        if (p.light) lights.push({ x: p.light[0], y: p.light[1], r: p.lightR || (p.kind === 'forge' ? 150 : 110), cool: p.cool });
        if (p.keeperAt && !keeperPt) keeperPt = p.keeperAt;
        return g + '</g>';
      }
      keeperPt = null;
      // Rugs and daylight on the floor go under the shadow layer; everything else stands on top of it.
      out.filter(function (o) { return o.p.kind === 'rug'; }).forEach(function (o) { s += piece(o); });
      out.forEach(function (o) { if (o.p.floorLight) s += o.p.floorLight; });
      s += shading(full);
      out.filter(function (o) { return o.p.kind !== 'rug'; }).forEach(function (o) { s += piece(o); });
      s += gradDefs() + '<g style="pointer-events:none">' + out.map(function (o) { return o.p.shaft && full ? o.p.shaft : ''; }).join('') + lights.map(function (l) { return '<circle cx="' + l.x.toFixed(1) + '" cy="' + l.y.toFixed(1) + '" r="' + l.r + '" fill="url(#' + (l.cool ? 'gCool' : 'gGlow') + ')" style="mix-blend-mode:screen"/>'; }).join('') + '</g>';
      if (!sq) s += '<g style="pointer-events:none">' + lip() + '</g>';
      if (full) s += lighting(lights);
      return { svg: s, viewBox: VB, keeperAt: keeperPt };
    }

    return {
      draw: draw, generate: generate, geometry: geometry, rect: rect, clashes: clashes, hug: hug, snap: snap, inside: inside,
      size: function () { return SZ[S.size] || 8; },
      accent: function () { return (PAL[S.pal] || PAL.oak).acc; },
      isWall: function (kind) { return !!WALLK[kind]; },
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
      pieces: S.pieces.map(function (p) {
        var o = { id: p.id, kind: p.kind, wall: p.wall || '', x: p.x || 0, y: p.y || 0, off: p.off || 0, len: p.len || 0, w: p.w || 0, d: p.d || 0, pinned: !!p.pinned };
        ['x', 'y', 'off', 'len', 'w', 'd'].forEach(function (k) { o[k] = Math.round(o[k] * 1000) / 1000; });
        return o;
      }),
      items: S.ov, portrait: S.portrait ? { left: S.portrait[0], top: S.portrait[1] } : null, lines: S.lines.slice()
    };
  }
  function fromLayout(S, L) {
    if (!L) return false;
    S.roomType = L.roomType || S.roomType; S.setting = L.setting || 'room'; S.size = L.size || 'm'; S.full = L.furniture || 'normal';
    S.deco = L.decorations || 'some'; S.pal = L.palette || S.pal;
    if (L.seeds) S.seeds = { room: L.seeds.room || 1, goods: L.seeds.goods || 1, deco: L.seeds.deco || 1 };
    S.pieces = (L.pieces || []).map(function (p) { var o = { id: p.id, kind: p.kind, x: p.x, y: p.y, off: p.off || 0, pinned: !!p.pinned }; if (p.wall) { o.wall = p.wall; o.len = p.len; } else { o.w = p.w; o.d = p.d; } return o; });
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

  window.ShopRoom = { createRoom: createRoom, toLayout: toLayout, fromLayout: fromLayout, shopItems: shopItems, keyOf: keyOf, ROOM_TYPES: ROOM_TYPES };
  if (!window.Chronicle || !window.document) return;

  var FX_KEY = 'chronicle.shopRoom.fx';
  var CSS = [
    '.shr{display:grid;gap:12px;margin-bottom:16px}',
    '.shr.arr{grid-template-columns:minmax(0,1fr) 280px;align-items:start}',
    '@media (max-width:900px){.shr.arr{grid-template-columns:1fr}}',
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
    '.shr-keeper.talk{animation:shr-bob .28s ease-in-out 6 alternate}',
    '@keyframes shr-bob{to{transform:translateY(-4px)}}',
    '.shr-plate{position:absolute;transform:translate(-50%,0);white-space:nowrap;font:600 .78rem Georgia,serif;color:#2a2115;background:linear-gradient(#e9cf7c,#b8913a);border-radius:3px;padding:2px 10px;box-shadow:0 1px 2px rgb(0 0 0/.5);z-index:3;pointer-events:none}',
    '@media (max-width:640px){.shr-keeper{width:52px;height:52px;margin:-26px 0 0 -26px}}',
    '.shr-bar{display:flex;align-items:center;gap:10px;flex-wrap:wrap;padding:8px 12px;border-top:3px solid var(--shr-acc)}',
    '.shr-wbtn{display:flex;align-items:center;gap:8px;border:0;background:none;color:var(--color-text-primary,#111827);cursor:pointer;padding:4px 2px;font-weight:600}',
    '.shr-wbtn span{color:var(--color-text-secondary,#6b7280);font-weight:400}',
    '.shr-chev{display:inline-block;transition:transform .25s}',
    '.shr.open .shr-chev{transform:rotate(180deg)}',
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
    '.shr-row{display:grid;grid-template-columns:34px minmax(0,1fr) auto auto;gap:10px;align-items:center;padding:6px 8px;border-radius:6px}',
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
    '.shr-panel{border:1px solid var(--color-border,#e5e7eb);border-radius:10px;background:var(--color-card-bg,#fff);padding:12px;display:grid;gap:10px}',
    '.shr-panel h3{margin:0;font-size:.95rem;color:var(--color-text-primary,#111827)}',
    '.shr-panel p{margin:0;font-size:.8rem;color:var(--color-text-secondary,#6b7280);line-height:1.45}',
    '.shr-field{display:grid;gap:4px}',
    '.shr-field>span{font-size:.72rem;text-transform:uppercase;letter-spacing:.06em;color:var(--color-text-muted,#9ca3af)}',
    '.shr-field select,.shr-field textarea{width:100%;min-width:0;border:1px solid var(--color-input-border,#d1d5db);border-radius:6px;padding:6px 8px;background:var(--color-input-bg,#fff);color:var(--color-text-primary,#111827);font:inherit}',
    '.shr-field .shr-seg{display:grid}',
    '.shr-check{display:flex;gap:8px;align-items:flex-start;font-size:.82rem}',
    '.shr-gbtn{border:0;border-radius:6px;background:var(--shr-acc);color:#fff;padding:8px 10px;font-weight:600;cursor:pointer;text-align:left}',
    '.shr-gbtn small{display:block;font-weight:400;opacity:.85;font-size:.74rem}',
    '.shr-gbtn.alt{background:var(--color-bg-tertiary,#f3f4f6);color:var(--color-text-primary,#111827);border:1px solid var(--color-border,#e5e7eb)}',
    '.shr-chips{display:flex;flex-wrap:wrap;gap:6px}',
    '.shr-chip{border:1px solid var(--color-border,#e5e7eb);background:var(--color-card-bg,#fff);color:var(--color-text-body,#374151);border-radius:999px;padding:3px 10px;cursor:pointer;display:inline-flex;align-items:center;gap:6px;font-size:.8rem}',
    '.shr-chip[aria-pressed="true"]{border-color:var(--color-text-primary,#111827);box-shadow:inset 0 0 0 1px var(--color-text-primary,#111827)}',
    '.shr-sw{width:12px;height:12px;border-radius:50%}',
    '.shr-status{font-size:.75rem;color:var(--color-text-muted,#9ca3af)}',
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
    '@media (prefers-reduced-motion:reduce){.shr *,.shr-say,.shr-tip{animation:none!important;transition:none!important}}'
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
      var S = { roomType: 'general', pal: 'oak', setting: 'room', fx: readFx(), size: 'm', full: 'normal', deco: 'some', keep: true,
        seeds: { room: 1, goods: 1, deco: 1 }, pieces: [], ov: {}, portrait: null, lines: [], mode: 'shop', its: [],
        key: keyOf(eid), name: ds.shopName || 'Shop', dark: document.documentElement.classList.contains('dark') };
      var room = createRoom(S), ICONS = room.ICONS, rels = [], relsOk = false, tab = 'All', open = false, line = 0, saveT = 0, dirty = false;
      el.hidden = true;

      // ---- Skeleton ----
      el.innerHTML = '<div class="shr' + (S.fx === 'light' ? ' fxlight' : '') + '"><div class="shr-card">' +
        '<div class="shr-top"><b></b><span class="shr-sub">Shop</span>' + (canArrange ? '<span class="shr-mode" role="group" aria-label="Mode"><button type="button" data-mode="shop" aria-pressed="true">Shop</button><button type="button" data-mode="arr" aria-pressed="false">Arrange</button></span>' : '') + '</div>' +
        '<div class="shr-scene"><svg class="shr-iso" role="img"></svg><div class="shr-grain"><svg width="100%" height="100%" aria-hidden="true"><filter id="shr-grn"><feTurbulence type="fractalNoise" baseFrequency=".85" numOctaves="2" stitchTiles="stitch"/><feColorMatrix type="saturate" values="0"/></filter><rect width="100%" height="100%" filter="url(#shr-grn)"/></svg></div>' +
        '<button type="button" class="shr-keeper"></button><div class="shr-plate"></div></div>' +
        '<div class="shr-bar"><button type="button" class="shr-wbtn" aria-expanded="false">Wares <span></span><i class="shr-chev" aria-hidden="true">▾</i></button>' +
        '<span class="shr-fx">Shadows <span class="shr-seg" role="group" aria-label="Shadows and light"><button type="button" data-fx="full">Full</button><button type="button" data-fx="light">Light</button></span></span></div>' +
        '<div class="shr-wares"><div><div class="shr-wp"><div class="shr-tabs"></div><div class="shr-list"></div></div></div></div></div></div>';
      var root = el.firstChild, svg = root.querySelector('.shr-iso'), scene = root.querySelector('.shr-scene'), keeper = root.querySelector('.shr-keeper'), plate = root.querySelector('.shr-plate');
      var tip = document.createElement('div'), say = document.createElement('div'), pick = document.createElement('div'), panel = null;
      tip.className = 'shr-tip'; tip.setAttribute('role', 'tooltip'); say.className = 'shr-say'; say.setAttribute('aria-live', 'polite');
      pick.className = 'shr-pick'; pick.hidden = true; pick.setAttribute('role', 'dialog'); pick.setAttribute('aria-label', 'Change item look');
      document.body.appendChild(tip); document.body.appendChild(say); document.body.appendChild(pick);
      root.querySelector('.shr-top b').textContent = S.name;
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
        var acc = room.accent();
        root.style.setProperty('--shr-acc', acc);
        if (!ds.shopImage) keeper.innerHTML = silhouette(acc);
        placeKeeper();
      }
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
          return '<div class="shr-row' + (it.out ? ' out' : '') + '" data-row="' + it.id + '"><span class="shr-ic" style="--c:' + it.c + '">' + iconSvg(it.ic, ICONS) + '</span><span>' + name + '<br><span class="shr-m">' + esc(it.k) + '</span></span><span class="shr-price">' + esc(it.price) + '</span><span class="shr-stock">' + stock + '</span></div>';
        });
        root.querySelector('.shr-list').innerHTML = rows.join('') || '<p class="shr-empty">' + (canArrange ? 'Nothing for sale yet. Add goods in the shop inventory below.' : 'Nothing for sale right now.') + '</p>';
        root.querySelectorAll('.shr-fx button').forEach(function (b) { b.setAttribute('aria-pressed', b.dataset.fx === S.fx); });
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
        var text = S.lines[line++ % S.lines.length], n = 0, r = keeper.getBoundingClientRect();
        keeper.classList.remove('talk'); void keeper.offsetWidth; keeper.classList.add('talk');
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

      // ---- Saving (owners only). Every change in Arrange saves after a short pause. ----
      function status(t) { var s = panel && panel.querySelector('.shr-status'); if (s) s.textContent = t; }
      function save() {
        if (!canArrange) return;
        dirty = true; clearTimeout(saveT); status('Saving…');
        saveT = setTimeout(function () {
          // Drop looks for goods no longer sold, so the saved list can't creep up to
          // the server's cap; only when the goods list really loaded.
          if (relsOk) { var live = {}; rels.forEach(function (r) { live[String(r.id)] = 1; }); Object.keys(S.ov).forEach(function (k) { if (!live[k]) delete S.ov[k]; }); }
          Chronicle.apiFetch(ds.roomEndpoint, { method: 'PUT', body: toLayout(S), csrfToken: ds.csrfToken })
            .then(function (res) { if (!res.ok) throw new Error('save ' + res.status); dirty = false; status('Saved'); })
            .catch(function () { status('Couldn’t save. Your changes are still here; try again.'); });
        }, 700);
      }

      // ---- Arrange: drag furniture, drag the portrait, unpin, change an item's look ----
      var drag = null, kd = null, picking = null;
      function svgPt(e) { var m = svg.getScreenCTM().inverse(), q = svg.createSVGPoint(); q.x = e.clientX; q.y = e.clientY; q = q.matrixTransform(m); return [q.x, q.y]; }
      function toFloor(q) { var a = q[0] / 32, b = q[1] / 16; return [(a + b) / 2, (b - a) / 2]; }
      function byId(id) { for (var i = 0; i < S.pieces.length; i++) if (S.pieces[i].id === id) return S.pieces[i]; return null; }
      function onSvgDown(e) {
        if (S.mode !== 'arr') return;
        var pin = e.target.closest('.pinbtn');
        if (pin) { var pp = byId(+pin.getAttribute('data-unpin')); if (pp) { pp.pinned = false; draw(); save(); } return; }
        if (e.target.closest('.it')) return;
        var g = e.target.closest('.piece'); if (!g) return;
        var p = byId(+g.getAttribute('data-p')); if (!p) return;
        var q = svgPt(e);
        drag = { p: p, q: q, f: toFloor(q), x: p.x, y: p.y, moved: false };
        svg.setPointerCapture(e.pointerId); e.preventDefault();
      }
      function onSvgMove(e) {
        if (!drag) return;
        var p = drag.p, q = svgPt(e), nx = drag.x, ny = drag.y;
        // Wall pieces slide along their wall; floor pieces move on the floor grid.
        if (room.isWall(p.kind)) { if (p.wall === 'Y') nx = room.snap(drag.x + (q[0] - drag.q[0]) / 32); else ny = room.snap(drag.y - (q[0] - drag.q[0]) / 32); }
        else { var f = toFloor(q); nx = room.snap(drag.x + f[0] - drag.f[0]); ny = room.snap(drag.y + f[1] - drag.f[1]); }
        if (nx === p.x && ny === p.y) return;
        var ox = p.x, oy = p.y, oo = p.off; p.x = nx; p.y = ny;
        if ((room.isWall(p.kind) && !room.hug(p)) || room.clashes(p, S.pieces, room.size())) { p.x = ox; p.y = oy; p.off = oo; return; }
        drag.moved = true; later();
      }
      function onSvgUp() { if (!drag) return; var moved = drag.moved; if (moved) drag.p.pinned = true; drag = null; draw(); if (moved) save(); }
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
        var it = S.its[i], r = g.getBoundingClientRect();
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
        var g = svg.querySelector('.it[data-i="' + picking + '"]'); if (g) openPicker(picking, g);
      }
      function onDocDown(e) { if (picking !== null && !pick.contains(e.target) && !e.target.closest('.it')) closePicker(); }
      function onDocKey(e) { if (e.key === 'Escape') { closePicker(); say.classList.remove('on'); } }
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
      function renderPanel() {
        if (!panel) return;
        var pins = S.pieces.filter(function (p) { return p.pinned; }).length;
        panel.innerHTML = '<h3>Arrange this room</h3><p>Drag furniture and the portrait. Click an item to change its icon. The generator builds the whole room; anything you moved stays put while the box is ticked.</p>' +
          '<label class="shr-field"><span>Shop type</span><select data-sel="roomType">' + ROOM_TYPES.map(function (t) { return '<option value="' + t[0] + '"' + (t[0] === S.roomType ? ' selected' : '') + '>' + t[1] + '</option>'; }).join('') + '</select></label>' +
          '<label class="shr-field"><span>Setting</span><select data-sel="setting">' + room.SETTINGS.map(function (t) { return '<option value="' + t[0] + '"' + (t[0] === S.setting ? ' selected' : '') + '>' + t[1] + '</option>'; }).join('') + '</select></label>' +
          '<div class="shr-field"><span>Room size</span>' + seg('size', [['s', 'Small'], ['m', 'Medium'], ['l', 'Large']]) + '</div>' +
          '<div class="shr-field"><span>Furniture</span>' + seg('full', [['sparse', 'Sparse'], ['normal', 'Normal'], ['packed', 'Packed']]) + '</div>' +
          '<div class="shr-field"><span>Decorations</span>' + seg('deco', [['none', 'None'], ['some', 'Some'], ['lots', 'Lots']]) + '</div>' +
          '<div class="shr-field"><span>Colours</span><span class="shr-chips">' + room.PALS.map(function (p) { return '<button type="button" class="shr-chip" data-k="pal" data-v="' + p[0] + '" aria-pressed="' + (S.pal === p[0]) + '"><span class="shr-sw" style="background:' + p[2] + '"></span>' + p[1] + '</button>'; }).join('') + '</span></div>' +
          '<label class="shr-check"><input type="checkbox" data-keep="1"' + (S.keep ? ' checked' : '') + '> Keep what I’ve moved by hand</label>' +
          '<button type="button" class="shr-gbtn" data-gen="all">Generate whole room<small>New furniture, new spots for goods, new decorations</small></button>' +
          '<button type="button" class="shr-gbtn alt" data-gen="goods">Shuffle goods only<small>Same furniture, goods move to new spots</small></button>' +
          '<button type="button" class="shr-gbtn alt" data-gen="deco">New decorations only</button>' +
          '<label class="shr-field"><span>What the keeper says (one line each)</span><textarea rows="3" data-lines="1" maxlength="2200"></textarea></label>' +
          '<span class="shr-status">' + (pins ? pins + ' moved by hand. ' : '') + (S.portrait ? 'Portrait placed by hand. ' : '') + (dirty ? 'Saving…' : '') + '</span>';
        panel.querySelector('[data-lines]').value = S.lines.join('\n');
      }
      function onRootClick(e) {
        var b = e.target.closest('button'); if (!b) return;
        if (b.dataset.mode) { S.mode = b.dataset.mode; setMode(); return; }
        if (b.dataset.fx) { S.fx = b.dataset.fx; saveFx(S.fx); root.classList.toggle('fxlight', S.fx !== 'full'); draw(); renderList(); return; }
        if (b.dataset.tab) { tab = b.dataset.tab; renderList(); return; }
        if (b.classList.contains('shr-wbtn')) { setOpen(!open); return; }
        if (b === keeper) { onKeeperClick(); return; }
        if (!canArrange) return;
        var k = b.dataset.k;
        if (k === 'pal' || k === 'deco') { S[k] = b.dataset.v; draw(); }
        else if (k === 'size' || k === 'full') { S[k] = b.dataset.v; S.portrait = S.keep ? S.portrait : null; room.generate(); draw(); }
        else if (b.dataset.gen === 'all') { S.seeds.room++; S.seeds.goods++; S.seeds.deco++; if (!S.keep) S.portrait = null; room.generate(); draw(); }
        else if (b.dataset.gen === 'goods') { S.seeds.goods++; draw(); }
        else if (b.dataset.gen === 'deco') { S.seeds.deco++; draw(); }
        else return;
        renderPanel(); save();
      }
      function onRootChange(e) {
        var t = e.target;
        if (t.dataset.sel) { S[t.dataset.sel] = t.value; if (t.dataset.sel === 'setting') S.portrait = null; room.generate(); draw(); renderPanel(); save(); }
        else if (t.dataset.keep) S.keep = t.checked;
        else if (t.dataset.lines) { S.lines = t.value.split('\n').map(function (s) { return s.trim().slice(0, 200); }).filter(Boolean).slice(0, 10); save(); }
      }
      function setMode() {
        root.classList.toggle('arr', S.mode === 'arr');
        root.querySelectorAll('[data-mode]').forEach(function (b) { b.setAttribute('aria-pressed', b.dataset.mode === S.mode); });
        if (S.mode === 'arr' && !panel) { panel = document.createElement('aside'); panel.className = 'shr-panel'; panel.setAttribute('aria-label', 'Arrange this room'); root.appendChild(panel); }
        if (S.mode !== 'arr' && panel) { panel.remove(); panel = null; }
        renderPanel(); closePicker(); draw();
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
      root.addEventListener('click', onRootClick); root.addEventListener('change', onRootChange);
      pick.addEventListener('click', onPickClick);
      document.addEventListener('pointerdown', onDocDown);
      document.addEventListener('keydown', onDocKey);
      window.addEventListener('scroll', onScroll, { passive: true });

      el._shopRoom = {
        onDocDown: onDocDown, onDocKey: onDocKey,
        cleanup: function () {
          clearTimeout(saveT); clearInterval(typeT); clearTimeout(sayT); if (raf) cancelAnimationFrame(raf);
          themeObs.disconnect();
          window.removeEventListener('scroll', onScroll);
          document.removeEventListener('pointerdown', onDocDown); document.removeEventListener('keydown', onDocKey);
          tip.remove(); say.remove(); pick.remove();
        }
      };

      // ---- Load the saved room and the shop's goods ----
      // The room only shows when the armory add-on answers; with the add-on off
      // the endpoint is a 404 and the shop page keeps just its inventory list.
      Promise.all([
        Chronicle.apiFetch(ds.roomEndpoint).then(function (r) { if (!r.ok) throw new Error('room ' + r.status); return r.json(); }),
        Chronicle.apiFetch(ds.relationsEndpoint).then(function (r) { if (r.ok) { relsOk = true; return r.json(); } return []; }).catch(function () { return []; })
      ]).then(function (res) {
        rels = (Array.isArray(res[1]) ? res[1] : (res[1] && res[1].data) || []).filter(function (r) { return r.relationType === 'sells'; }).map(function (r) {
          if (typeof r.metadata === 'string') { try { r.metadata = JSON.parse(r.metadata); } catch (e) { r.metadata = {}; } }
          r.metadata = r.metadata || {}; return r;
        });
        if (fromLayout(S, res[0] && res[0].layout)) room.geometry(); else room.generate();
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
