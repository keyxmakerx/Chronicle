/**
 * shop_room_parts_trades.js — rooms for more kinds of shop: a taproom,
 * a tailor's cutting room, a chart room, a menagerie, a reliquary and a
 * trading post.
 *
 * A parts pack for the counter room (shop_room_counter.js). It loads after
 * that file and shop_room_parts_strange.js and before shop_room.js, and adds its
 * parts to the room's own tables through ShopRoomCounter.kit, so a room built
 * from them is drawn, saved and shared like any other. Each part is one
 * function keyed by its option value. Walls, display units and counters are
 * baked into images, so whatever moves lives in an opening, a light or a
 * prop. The armory's lists (shop_room_validate.go and
 * shop_room_counter_starters.go) name the same parts and starter rooms.
 */
(function () {
  'use strict';
  var K = window.ShopRoomCounter && window.ShopRoomCounter.kit;
  if (!K || !K.FULL) return;
  var CM_COLS = K.CM_COLS, CM_CSS = K.CM_CSS, CM_LAYER = K.CM_LAYER, CM_LIGHT = K.CM_LIGHT, CM_LPOS = K.CM_LPOS, CM_OPEN_INFO = K.CM_OPEN_INFO, CM_OPT = K.CM_OPT, CM_PROPS = K.CM_PROPS, CM_ROWS = K.CM_ROWS, CM_STARTERS = K.CM_STARTERS, CM_TALL = K.CM_TALL, CM_WALLFQ = K.CM_WALLFQ, CM_WHERE = K.CM_WHERE, COUNTERS = K.COUNTERS, DISPLAYS = K.DISPLAYS, LIGHTS = K.LIGHTS, OPENINGS = K.OPENINGS, PROPS = K.PROPS, PROP_AT = K.PROP_AT, SHAPES = K.SHAPES, WALLS = K.WALLS, cmCandle = K.cmCandle, cmFrame = K.cmFrame, cmGrid = K.cmGrid, cmPost = K.cmPost, cmRainIn = K.cmRainIn, cmShelfBoard = K.cmShelfBoard, cmSky = K.cmSky, cmSkyline = K.cmSkyline, cmTop = K.cmTop, cmUnitFrame = K.cmUnitFrame, f1 = K.f1, lin = K.lin, mix = K.mix, rad = K.rad, rng = K.rng, FULL = K.FULL, RING = K.RING, mp3Defs = K.mp3Defs, mpLump = K.mpLump, mpOpen = K.mpOpen, mpStars = K.mpStars;

  /* ---- Small drawing helpers, shared with the places pack. Every part is
     one function in the same tables as the room's first parts, so an owner
     can mix them with everything already there. ---- */
  function n5p(d, a) { return '<path d="' + d + '" ' + (a || '') + '/>'; }
  function n5r(x, y, w, h, a) { return '<rect x="' + f1(x) + '" y="' + f1(y) + '" width="' + f1(w) + '" height="' + f1(h) + '" ' + (a || '') + '/>'; }
  function n5c(x, y, r, a) { return '<circle cx="' + f1(x) + '" cy="' + f1(y) + '" r="' + f1(r) + '" ' + (a || '') + '/>'; }
  function n5e(x, y, rx, ry, a) { return '<ellipse cx="' + f1(x) + '" cy="' + f1(y) + '" rx="' + f1(rx) + '" ry="' + f1(ry) + '" ' + (a || '') + '/>'; }
  function n5g(a, s) { return '<g ' + a + '>' + s + '</g>'; }
  // A prop says which slot it wants and which layer it is drawn in.
  function n5prop(name, at, layer, w, tall) {
    CM_PROPS.push(name); PROP_AT[name] = w ? { at: at, w: w } : { at: at };
    CM_LAYER[layer].push(name);
    if (tall) CM_TALL.push(name);
  }
  function n5light(name, spec, pos) { CM_OPT.light.push(name); CM_LIGHT[name] = spec; if (pos) CM_LPOS[name] = pos; }
  // A flame that leans and stretches, lit from its own base.
  function n5flame(x, y, h, j, fill, op) {
    var w = h * .34;
    return '<path class="mp-flame src-flame" style="animation-delay:-' + f1((j || 0) * .37 % 1.1) + 's" d="M' + f1(x) + ',' + f1(y - h) + ' c' + f1(-w) + ',' + f1(h * .45) + ' ' + f1(-w * 1.2) + ',' + f1(h * .8) + ' 0,' + f1(h) + ' c' + f1(w * 1.2) + ',' + f1(-h * .2) + ' ' + f1(w) + ',' + f1(-h * .55) + ' 0,' + f1(-h) + ' Z" fill="' + (fill || 'url(#src-flame)') + '"' + (op ? ' opacity="' + op + '"' : '') + '/>';
  }
  // Metals that are the same in every room: pewter, copper, black iron.
  function n5Metals() {
    return lin('n5-pewter', 0, [[0, '#2a2e34'], [.18, '#8a929c'], [.34, '#dfe6ee'], [.5, '#8a929c'], [.8, '#3a4048'], [1, '#1a1e22']]) +
      lin('n5-copper', 0, [[0, '#3a1608'], [.2, '#b0562a'], [.36, '#ffc89a'], [.52, '#b0562a'], [.82, '#5a2410'], [1, '#2a1006']]) +
      lin('n5-black', 0, [[0, '#08090a'], [.3, '#4a4e54'], [.42, '#8a9098'], [.56, '#3a3e44'], [1, '#08090a']]) +
      lin('n5-gold', 0, [[0, '#4a3008'], [.2, '#c89a30'], [.36, '#fff0b0'], [.52, '#d8a838'], [.82, '#7a5210'], [1, '#3a2606']]);
  }
  // Blocks of stone or brick in courses, each its own tone; wrap the result in a stone filter.
  function n5stones(r, x0, y0, x1, y1, h, w0, wv, base, dark, lite, k) {
    var s = '', x, y, w, j = 0;
    for (y = y0; y < y1; y += h, j++) for (x = x0 - (j % 2) * (w0 * .5); x < x1; x += w) { w = w0 + r() * wv; s += n5r(x + 1.4, y + 1.4, w - 2.8, h - 2.8, 'rx="3" fill="' + mix(base, r() < .5 ? dark : lite, r() * (k || .32)) + '"'); }
    return s;
  }
  // A ragged skyline from x0 to x1 around height y, closed down to yb.
  function n5ridge(r, x0, x1, y, amp, step, yb) {
    var d = 'M' + x0 + ',' + yb + ' L' + x0 + ',' + f1(y), x;
    for (x = x0 + step; x < x1; x += step * (.6 + r() * .8)) d += ' L' + f1(x) + ',' + f1(y - r() * amp);
    return d + ' L' + x1 + ',' + f1(y) + ' L' + x1 + ',' + yb + ' Z';
  }
  // A fir in tiers, each a little wider than the one above.
  function n5pine(x, y, h, col, snow) {
    var s = n5r(x - h * .03, y - h * .16, h * .06, h * .16, 'fill="' + mix(col, '#000', .5) + '"'), k, yy, w;
    for (k = 0; k < 5; k++) { yy = y - h * .12 - k * h * .17; w = h * (.3 - k * .048); s += n5p('M' + f1(x - w) + ',' + f1(yy) + ' L' + f1(x) + ',' + f1(yy - h * .3) + ' L' + f1(x + w) + ',' + f1(yy) + ' Z', 'fill="' + col + '"'); if (snow) s += n5p('M' + f1(x - w * .7) + ',' + f1(yy - h * .08) + ' L' + f1(x) + ',' + f1(yy - h * .3) + ' L' + f1(x + w * .5) + ',' + f1(yy - h * .1) + ' Q' + f1(x) + ',' + f1(yy - h * .13) + ' ' + f1(x - w * .7) + ',' + f1(yy - h * .08) + ' Z', 'fill="' + snow + '"'); }
    return s;
  }
  // Cloth hanging in folds from a straight top edge.
  function n5drape(x, y, w, h, col, n, hem) {
    var s = n5r(x, y, w, h, 'fill="' + col + '"'), i, fw = w / n;
    for (i = 0; i < n; i++) s += n5r(x + i * fw, y, fw * .5, h, 'fill="#000" opacity=".22"') + n5r(x + i * fw + fw * .5, y, fw * .16, h, 'fill="#fff" opacity=".1"');
    if (hem) s += n5r(x, y + h - 6, w, 6, 'fill="' + hem + '"');
    return s;
  }
  // A rope or chain that sags between two points.
  function n5sag(x0, y0, x1, y1, sag, col, sw, a) { return n5p('M' + f1(x0) + ',' + f1(y0) + ' Q' + f1((x0 + x1) / 2) + ',' + f1((y0 + y1) / 2 + sag * 2) + ' ' + f1(x1) + ',' + f1(y1), 'stroke="' + col + '" stroke-width="' + (sw || 2) + '" fill="none" ' + (a || '')); }
  // Board seams and uneven tone to lay over a filtered wood rect.
  function n5boards(r, x, y, w, h, bw, vert) {
    var s = '', k;
    if (vert) for (k = x; k < x + w; k += bw) s += n5r(k, y, bw, h, 'fill="' + (r() < .5 ? '#000' : '#fff') + '" opacity="' + f1(r() * .1) + '"') + n5r(k + bw - 1.4, y, 1.6, h, 'fill="#070402" opacity=".75"');
    else for (k = y; k < y + h; k += bw) s += n5r(x, k, w, bw, 'fill="' + (r() < .5 ? '#000' : '#fff') + '" opacity="' + f1(r() * .1) + '"') + n5r(x, k + bw - 1.4, w, 1.6, 'fill="#070402" opacity=".75"');
    return s;
  }
  // The soft shadow a shape throws on what is behind it.
  function n5sh(d, dx, dy, op, bl) { return n5p(d, 'fill="#000" opacity="' + (op || .5) + '" filter="url(#src-blur' + (bl || 5) + ')" transform="translate(' + dx + ',' + dy + ')"'); }
  // Each small movement: its class, its timing and its keyframes.
  [
    ['n5-sway', 'n5-sway 6s ease-in-out infinite', '0%,100%{transform:rotate(-3deg)}50%{transform:rotate(3deg)}'],
    ['n5-swing', 'n5-swing 4.4s ease-in-out infinite', '0%,100%{transform:rotate(-13deg)}50%{transform:rotate(13deg)}'],
    ['n5-nod', 'n5-nod 7s ease-in-out infinite', '0%,62%,100%{transform:rotate(0)}70%{transform:rotate(-5deg)}78%{transform:rotate(3deg)}86%{transform:rotate(-2deg)}'],
    ['n5-ear', 'n5-ear 5s ease-in-out infinite', '0%,80%,100%{transform:rotate(0)}85%{transform:rotate(-22deg)}90%{transform:rotate(6deg)}'],
    ['n5-turn', 'n5-turn 30s linear infinite', 'to{transform:rotate(360deg)}'],
    ['n5-slide', 'n5-slide 24s linear infinite', 'to{transform:translateX(var(--sx,-200px))}'],
    ['n5-snow', 'n5-snow 9s linear infinite', '0%{transform:translate(0,-30px)}100%{transform:translate(var(--fx,-30px),var(--fy,260px))}'],
    ['n5-tick', 'n5-tick 2s steps(1) infinite', '0%{transform:rotate(-7deg)}50%{transform:rotate(7deg)}'],
    ['n5-pend', 'n5-pend 2s ease-in-out infinite', '0%,100%{transform:rotate(-9deg)}50%{transform:rotate(9deg)}'],
    ['n5-puff', 'n5-puff 6s ease-out infinite', '0%{transform:translate(0,0) scale(.5);opacity:0}15%{opacity:.55}100%{transform:translate(var(--fx,14px),var(--fy,-120px)) scale(1.9);opacity:0}'],
    ['n5-glint', 'n5-glint 5s ease-in-out infinite', '0%,78%,100%{opacity:0;transform:scale(.3) rotate(0)}86%{opacity:1;transform:scale(1) rotate(40deg)}'],
    ['n5-swim', 'n5-swim 26s linear infinite', '0%{transform:translateX(0)}100%{transform:translateX(var(--sx,-1700px))}'],
    ['n5-wave', 'n5-wave 5s ease-in-out infinite', '0%,100%{transform:skewX(-5deg)}50%{transform:skewX(5deg)}'],
    ['n5-caust', 'n5-caust 9s ease-in-out infinite', '0%,100%{transform:translate(0,0);opacity:.5}50%{transform:translate(26px,8px);opacity:.9}'],
    ['n5-lid', 'n5-lid 11s ease-in-out infinite', '0%,58%,100%{transform:scaleY(1)}64%,92%{transform:scaleY(.04)}'],
    ['n5-beat', 'n5-beat 4.6s ease-in-out infinite', '0%,100%{opacity:.5}50%{opacity:1}'],
    ['n5-hover', 'n5-hover 7s ease-in-out infinite', '0%,100%{transform:translateY(0)}50%{transform:translateY(-10px)}'],
    ['n5-flit', 'n5-flit 13s ease-in-out infinite', '0%,100%{transform:translate(0,0)}25%{transform:translate(var(--fx,40px),-26px)}50%{transform:translate(calc(var(--fx,40px)*1.6),8px)}75%{transform:translate(calc(var(--fx,40px)*.5),-14px)}'],
    ['n5-rain', 'n5-rain 1.1s linear infinite', 'to{transform:translate(-10px,60px)}'],
    ['n5-streak', 'n5-streak 7s ease-in infinite', '0%{transform:translateY(0);opacity:0}10%{opacity:.8}100%{transform:translateY(var(--fy,160px));opacity:0}'],
    ['n5-shimmer', 'n5-shimmer 3.4s ease-in-out infinite', '0%,100%{opacity:.25}50%{opacity:.85}'],
    ['n5-bob', 'n5-bob 5s ease-in-out infinite', '0%,100%{transform:translateY(0) rotate(-2deg)}50%{transform:translateY(-6px) rotate(2deg)}'],
    ['n5-cloud', 'n5-cloud 60s linear infinite', 'from{transform:translateX(0)}to{transform:translateX(var(--sx,-700px))}'],
    ['n5-flap', 'n5-flap .7s ease-in-out infinite', '0%,100%{transform:scaleY(1)}50%{transform:scaleY(-.5)}'],
    ['n5-blink', 'n5-blink 5s ease-in-out infinite', '0%,90%,100%{transform:scaleY(1)}94%{transform:scaleY(.08)}'],
    ['n5-breath', 'n5-breath 4s ease-in-out infinite', '0%,100%{transform:scale(1,1)}50%{transform:scale(1.02,1.06)}'],
    ['n5-slit', 'n5-slit 6s ease-in-out infinite', '0%,100%{transform:scaleX(1)}50%{transform:scaleX(.42)}'],
    ['n5-haunt', 'n5-haunt 18s ease-in-out infinite', '0%{opacity:0;transform:translateX(var(--hx0,-60px))}25%,70%{opacity:var(--ho,.7)}100%{opacity:0;transform:translateX(var(--hx1,60px))}'],
    ['n5-tilt', 'n5-tilt 9s ease-in-out infinite', '0%,40%,100%{transform:rotate(0)}48%,70%{transform:rotate(-14deg)}78%,90%{transform:rotate(10deg)}'],
    ['n5-peer', 'n5-peer 11s ease-in-out infinite', '0%,40%,100%{transform:translateX(0)}48%,68%{transform:translateX(-2.6px)}76%,92%{transform:translateX(2.6px)}']
  ].forEach(function (q) { CM_CSS.push('@keyframes ' + q[0] + '{' + q[2] + '}', '.' + q[0] + '{animation:' + q[1] + '}'); });
  CM_CSS.push('.n5-vb{transform-box:view-box}.n5-fb{transform-box:fill-box;transform-origin:50% 50%}.n5-fbt{transform-box:fill-box;transform-origin:50% 0}.n5-fbb{transform-box:fill-box;transform-origin:50% 100%}',
    '.n5-puff,.n5-glint,.n5-streak,.n5-haunt{opacity:0}',
    // With motion stopped a glint would stay hidden, so it rests lit.
    '@media (prefers-reduced-motion:reduce){.n5-glint{opacity:.7}}', 'html[data-motion="calm"] .n5-glint,html[data-motion="off"] .n5-glint{opacity:.7}');

  /* ================= Kinds of shop ================= */

  /* ---- The tavern: a half-timbered taproom, a hearth for a window, casks
     for shelves and a bar for a counter. ---- */
  CM_OPT.shape.push('taproom'); CM_OPT.wall.push('daub'); CM_OPT.opening.push('hearth'); CM_OPT.display.push('casks'); CM_OPT.counter.push('bar');
  CM_WALLFQ.daub = [.03, 3.2, 4]; CM_OPEN_INFO.hearth = 'The hearth';
  n5light('firelight', { base: '#ffb070', glow: '#ff9240', amb: '#3a2c3c', reach: 1500, k: 1 }, function (sk) { return sk.opening === 'hearth' ? [912, 384] : null; });
  n5prop('tankards', 'ceil', 'back'); n5prop('keg', 'cL', 'front', 150); n5prop('antlers', 'wallR', 'back');

  WALLS.daub = function (t) {
    var r = rng(19), s, x, i, y, xs = [], tim = '', sh = '';
    s = '<rect ' + FULL + ' fill="#cbb690" filter="url(#cm-wall)"/><rect ' + FULL + ' fill="#3a2616" filter="url(#cm-stain)" opacity=".62"/>';
    // old plaster: hairline cracks, and patches where it has been made good
    for (i = 0; i < 14; i++) { x = -280 + r() * 1760; y = 40 + r() * 380; var d = 'M' + f1(x) + ',' + f1(y), k; for (k = 0; k < 4; k++) { x += (r() - .5) * 26; y += 8 + r() * 18; d += ' L' + f1(x) + ',' + f1(y); } s += n5p(d, 'stroke="#2a1a10" stroke-opacity=".34" stroke-width=".9" fill="none"'); }
    for (i = 0; i < 6; i++) s += n5e(-260 + r() * 1700, 80 + r() * 330, 40 + r() * 60, 24 + r() * 30, 'fill="#e8dcc0" opacity=".07" filter="url(#src-blur8)"');
    // the frame: posts, two rails, and a brace across some of the bays
    for (x = -290; x < 1500; x += 150 + r() * 34) xs.push(Math.round(x));
    xs.forEach(function (px, j) {
      sh += n5r(px + 22, -150, 9, 620, 'fill="#000" opacity=".34" filter="url(#src-blur3)"');
      tim += n5r(px, -150, 22, 620, 'fill="#000" filter="url(#cm-post)"');
      var nx = xs[j + 1]; if (!nx) return;
      if (j % 3 === 1) tim += n5p('M' + (px + 22) + ',300 L' + (nx - 2) + ',128 L' + nx + ',150 L' + (px + 22) + ',322 Z', 'fill="#000" filter="url(#cm-post)"');
      if (j % 3 === 2) tim += n5p('M' + (px + 22) + ',128 L' + nx + ',300 L' + nx + ',322 L' + (px + 22) + ',150 Z', 'fill="#000" filter="url(#cm-post)"');
    });
    [106, 300].forEach(function (ry) { sh += n5r(-300, ry + 22, 1800, 9, 'fill="#000" opacity=".34" filter="url(#src-blur3)"'); tim += n5r(-300, ry, 1800, 22, 'fill="#000" filter="url(#cm-beam)"') + n5r(-300, ry, 1800, 1.6, 'fill="' + t.lc + '" opacity=".22"'); });
    xs.forEach(function (px) { [117, 311].forEach(function (ry) { tim += n5c(px + 7, ry, 2.2, 'fill="#0a0604" opacity=".8"') + n5c(px + 15, ry, 2.2, 'fill="#0a0604" opacity=".8"'); }); });
    return s + sh + tim;
  };
  SHAPES.taproom = function (t) {
    var s = WALLS[t.th.wall](t), i, x, r = rng(7);
    // the floor above: joists with their ends toward you, black with years of smoke
    s += '<defs>' + n5Metals() + '</defs>' + n5r(-300, -150, 1800, 176, 'fill="#000" filter="url(#cm-beam)"') + n5r(-300, 26, 1800, 12, 'fill="#000" opacity=".5" filter="url(#src-blur3)"');
    for (i = 0; i < 11; i++) { x = -260 + i * 170; s += n5r(x, 16, 58, 44, 'rx="3" fill="#000" filter="url(#cm-post)"') + n5r(x, 16, 58, 44, 'rx="3" fill="#0a0604" opacity=".35"') + n5r(x, 16, 58, 2.6, 'fill="' + t.lc + '" opacity=".28"') + n5r(x, 60, 58, 11, 'fill="#000" opacity=".45" filter="url(#src-blur3)"'); }
    return s + cmPost(-6, 34, 26, 474) + cmPost(1172, 34, 26, 474);
  };
  OPENINGS.hearth = function (t) {
    var box = 'M818,452 V306 Q818,238 912,238 Q1006,238 1006,306 V452 Z', out = 'M776,452 V236 Q776,212 800,208 L842,126 V-150 H982 V126 L1024,208 Q1048,212 1048,236 V452 Z', r = rng(29), s, i, x, y, w, fr, st = '';
    s = '<defs>' + rad('n5-firebox', [[0, '#5a1c08'], [.6, '#220a04'], [1, '#0a0302']], ' cx=".5" cy=".95" r=".9"') + rad('n5-coal', [[0, '#fff0b0'], [.4, '#ff9a30'], [1, '#c8300a', 0]]) + n5Metals() + mp3Defs() + '<clipPath id="n5-hood"><path clip-rule="evenodd" d="' + out + ' ' + box + '"/></clipPath></defs>';
    s += n5r(800, 220, 230, 240, 'fill="url(#n5-firebox)"');
    // the fire-back: sooty brick, lit from below
    for (y = 250; y < 440; y += 22) for (x = 812 + (Math.round(y / 22) % 2) * 22; x < 1010; x += 44) s += n5r(x, y, 42, 20, 'fill="#3a1408" opacity="' + f1(.25 + (y - 250) / 500 + r() * .2) + '" stroke="#0a0302" stroke-width="1.4"');
    s += n5g('class="cm-pulse"', n5e(912, 444, 78, 16, 'fill="url(#n5-coal)"'));
    // the breast and hood: rough stone, each block its own tone
    for (y = -150; y < 452; y += 34) for (x = 760 - (Math.round((y + 150) / 34) % 2) * 34; x < 1060; x += w) { w = 46 + r() * 34; st += n5r(x + 1.5, y + 1.5, w - 3, 31, 'rx="4" fill="' + mix('#6e665e', r() < .5 ? '#2a2420' : '#b0a698', r() * .3) + '"'); }
    fr = n5p(out, 'fill="#000" opacity=".5" filter="url(#src-blur8)" transform="translate(-8,4)"') + '<g clip-path="url(#n5-hood)">' + n5r(760, -150, 300, 610, 'fill="#1a1512"') + n5g('filter="url(#mp-stone)"', st) + '</g>';
    // soot above the arch, the arch stones, and the lip the fire lights
    fr += '<g clip-path="url(#n5-hood)">' + n5e(912, 214, 96, 60, 'fill="#000" opacity=".5" filter="url(#src-blur8)"') + '</g>';
    for (i = 0; i < 9; i++) { var a = Math.PI + i * Math.PI / 8, a2 = a + Math.PI / 8; fr += n5p('M' + f1(912 + 96 * Math.cos(a)) + ',' + f1(306 + 70 * Math.sin(a)) + ' L' + f1(912 + 124 * Math.cos(a)) + ',' + f1(306 + 96 * Math.sin(a)) + ' L' + f1(912 + 124 * Math.cos(a2)) + ',' + f1(306 + 96 * Math.sin(a2)) + ' L' + f1(912 + 96 * Math.cos(a2)) + ',' + f1(306 + 70 * Math.sin(a2)) + ' Z', i < 8 ? 'fill="' + mix('#7a7068', '#3a322c', r() * .5) + '" stroke="#14100e" stroke-width="2" filter="url(#mp-stone)"' : 'fill="none"'); }
    fr += n5p(box, 'fill="none" stroke="#ff9a40" stroke-opacity=".55" stroke-width="3" filter="url(#src-blur3)" class="n5-shimmer"') + n5p(box, 'fill="none" stroke="#000" stroke-opacity=".7" stroke-width="2"');
    // the mantel: one oak beam, with the house pewter on it
    fr += n5r(752, 196, 320, 14, 'fill="#000" opacity=".45" filter="url(#src-blur5)" transform="translate(0,12)"') + n5r(752, 182, 320, 24, 'rx="3" fill="#000" filter="url(#cm-beam)"') + n5r(752, 182, 320, 2.4, 'fill="' + t.lc + '" opacity=".4"');
    [[790, 30, 1], [832, 24, 0], [1004, 34, 1], [1040, 22, 0]].forEach(function (q) { var x0 = q[0], h = q[1]; fr += n5e(x0, 183, 13, 3, 'fill="#000" opacity=".5"') + n5p('M' + (x0 - 10) + ',182 L' + (x0 - 8) + ',' + (182 - h) + ' H' + (x0 + 8) + ' L' + (x0 + 10) + ',182 Z', 'fill="url(#' + (q[2] ? 'n5-pewter' : 'n5-copper') + ')"') + n5e(x0, 182 - h, 8, 2, 'fill="#14161a"') + n5p('M' + (x0 + 9) + ',' + (182 - h * .8) + ' q9,2 8,' + f1(h * .3) + ' q-1,6 -8,6', 'stroke="url(#' + (q[2] ? 'n5-pewter' : 'n5-copper') + ')" stroke-width="2.6" fill="none"'); });
    fr += n5r(776, 440, 272, 14, 'fill="#3e3832" filter="url(#mp-stone)"') + n5r(776, 440, 272, 2, 'fill="#ffb060" opacity=".5"');
    return mpOpen(box, s, fr, n5e(912, 420, 300, 200, 'fill="url(#cm-glow)" opacity=".24" pointer-events="none" class="mp-breathe"'));
  };
  // The fire itself: two logs on a bed of coals, flame in three depths, sparks going up.
  function n5fire(r) {
    var s = '<defs>' + rad('n5-coal', [[0, '#fff0b0'], [.4, '#ff9a30'], [1, '#c8300a', 0]]) + '</defs>', i;
    s += n5g('class="cm-pulse"', n5e(0, -8, 78, 16, 'fill="url(#n5-coal)"'));
    for (i = 0; i < 16; i++) s += n5e(-66 + r() * 132, -12 + r() * 8, 5 + r() * 7, 2.6 + r() * 2.6, 'fill="' + (r() < .5 ? '#ffb040' : '#ff6a20') + '" opacity="' + f1(.6 + r() * .4) + '"');
    s += n5r(-66, -32, 132, 17, 'rx="8" fill="#1c0e06" transform="rotate(-4 0 -24)"') + n5r(-54, -45, 112, 15, 'rx="7" fill="#24120a" transform="rotate(5 0 -38)"') + n5p('M-60,-22 q30,-6 60,2 t60,-4', 'stroke="#ff8a30" stroke-width="2" stroke-opacity=".8" fill="none" class="n5-shimmer"') + n5p('M-46,-38 q26,6 50,0 t48,4', 'stroke="#ffb050" stroke-width="1.6" stroke-opacity=".7" fill="none" class="n5-shimmer" style="animation-delay:-1.4s"');
    [[-44, 70, 0], [-20, 104, 1], [6, 124, 2], [30, 96, 3], [50, 64, 4]].forEach(function (q) { s += n5flame(q[0], -38, q[1], q[2] + 2, '#e0520c', .7).replace('/>', ' filter="url(#src-blur3)"/>'); });
    [[-32, 46, 0], [-12, 70, 1], [8, 84, 2], [26, 62, 3], [42, 40, 4]].forEach(function (q) { s += n5flame(q[0], -38, q[1], q[2]); });
    [[-6, 34, 5], [14, 40, 6]].forEach(function (q) { s += n5flame(q[0], -38, q[1], q[2], '#fff6d0', .9); });
    for (i = 0; i < 12; i++) s += n5c(-50 + r() * 100, -52, 1.2 + r() * 1.5, 'class="cm-spark" fill="#ffd27a" style="--dx:' + f1((r() - .5) * 70) + 'px;animation-delay:-' + f1(r() * 3.4) + 's"');
    return s;
  }
  LIGHTS.firelight = function (t) {
    var r = rng(31), x = t.lp.x, s = '<g data-k="lamp" class="src-nohit">';
    if (t.sk.opening === 'hearth') {
      // in the hearth: the crane, a pot on its chain, the dogs the logs lie on
      s += n5p('M828,286 H930 M828,252 V440', 'stroke="#0a0a0c" stroke-width="6" fill="none" stroke-linecap="round"') + n5p('M828,330 L872,286', 'stroke="#0a0a0c" stroke-width="4" fill="none"') + n5p('M916,288 V336', 'stroke="#1a1a1e" stroke-width="2.6" stroke-dasharray="5 3" fill="none"');
      s += n5g('transform="translate(912,452)"', n5fire(r));
      s += n5g('class="n5-vb n5-sway" style="transform-origin:916px 288px;animation-duration:8s"', n5p('M886,350 Q916,326 946,350', 'stroke="#101012" stroke-width="3" fill="none"') + n5p('M882,350 H950 Q956,398 916,402 Q876,398 882,350 Z', 'fill="#0c0c0e"') + n5e(916, 350, 34, 6, 'fill="#1e1e22"') + n5e(916, 351, 28, 4, 'fill="#5a3a1a"') + n5p('M888,366 Q894,394 914,398', 'stroke="#ff9a50" stroke-opacity=".7" stroke-width="2.4" fill="none"') + n5p('M944,366 Q938,394 918,398', 'stroke="#ff9a50" stroke-opacity=".4" stroke-width="2" fill="none"'));
      [0, 1, 2].forEach(function (j) { s += n5e(904 + j * 12, 340, 9, 6, 'class="cm-steam" fill="#e8e0d8" style="animation-delay:-' + (j * 1.7) + 's"'); });
      s += n5p('M852,452 V420 M972,452 V420', 'stroke="#0a0a0c" stroke-width="7" stroke-linecap="round" fill="none"') + n5c(852, 416, 6, 'fill="#1a1a1e"') + n5c(972, 416, 6, 'fill="#1a1a1e"');
    } else {
      // anywhere else: an iron basket of fire stood on the counter
      s += n5e(x, 482, 46, 8, 'fill="url(#src-ao)"') + n5g('transform="translate(' + x + ',462) scale(.5)"', n5fire(r)) + n5p('M' + (x - 40) + ',440 q4,26 40,28 q36,-2 40,-28', 'stroke="#141416" stroke-width="5" fill="none"') + n5p('M' + (x - 24) + ',446 v20 M' + (x - 8) + ',448 v20 M' + (x + 8) + ',448 v20 M' + (x + 24) + ',446 v20 M' + (x - 22) + ',466 l-8,16 M' + (x + 22) + ',466 l8,16', 'stroke="#141416" stroke-width="3.4" fill="none" stroke-linecap="round"');
    }
    return { back: '', front: s + '</g>' };
  };

  DISPLAYS.casks = function (t) {
    var r = rng(43), s, i, x, y, k, st = '';
    s = '<defs>' + mp3Defs() + n5Metals() + rad('n5-caskh', [[0, '#9a7040'], [.62, '#74502c'], [1, '#2e1c0e']]) + '</defs>' + n5r(30, 74, 462, 384, 'fill="#090605"');
    CM_ROWS.forEach(function (ry, j) {
      for (i = 0; i < 4; i++) {
        x = CM_COLS[i]; y = ry - 54; var id = 'n5-ck' + j + i;
        s += '<clipPath id="' + id + '"><circle cx="' + x + '" cy="' + y + '" r="46"/></clipPath>' + n5c(x + 5, y + 6, 53, 'fill="#000" opacity=".6" filter="url(#src-blur5)"');
        // the head: boards across, their seams, the chime round the edge
        st = n5c(x, y, 46, 'fill="url(#n5-caskh)"');
        for (k = -2; k <= 2; k++) st += n5r(x - 48, y + k * 19 - 9.5, 96, 19, 'fill="' + (r() < .5 ? '#000' : '#ffffff') + '" opacity="' + f1(r() * .14) + '"') + n5p('M' + (x - 48) + ',' + f1(y + k * 19 + 9.5) + ' h96', 'stroke="#1a0e06" stroke-width="1.4" stroke-opacity=".8"');
        s += n5g('clip-path="url(#' + id + ')" filter="url(#mp-barkh)"', st);
        s += n5c(x, y, 49, 'fill="none" stroke="url(#n5-black)" stroke-width="7"') + n5c(x, y, 44.5, 'fill="none" stroke="#000" stroke-opacity=".55" stroke-width="2"') + n5p('M' + (x - 34) + ',' + (y - 36) + ' A49,49 0 0 1 ' + (x + 10) + ',' + (y - 48), 'stroke="#fff" stroke-opacity=".3" stroke-width="2" fill="none"');
        // a brand burnt into the head, the bung, and a tap on the lowest row
        s += n5p('M' + (x - 13) + ',' + (y - 26) + ' l8,10 m0,-10 l-8,10 M' + (x - 2) + ',' + (y - 26) + ' l8,10 m0,-10 l-8,10 M' + (x + 9) + ',' + (y - 26) + ' l8,10 m0,-10 l-8,10', 'stroke="#1a0c04" stroke-opacity=".7" stroke-width="2.2" fill="none"') + n5c(x + 26, y + 6, 4, 'fill="#3a2414" stroke="#120904" stroke-width="1.4"');
      }
      // the stillage each row lies on
      s += n5r(36, ry, 450, 12, 'fill="#000" filter="url(#cm-beam)"') + n5r(36, ry, 450, 2, 'fill="' + t.lc + '" opacity=".4"') + n5r(36, ry + 12, 450, 14, 'fill="#000" opacity=".5" filter="url(#src-blur3)"');
      for (i = 0; i < 5; i++) s += n5p('M' + (40 + i * 110) + ',' + (ry + 12) + ' l12,0 l-3,10 h-6 Z', 'fill="#000" filter="url(#cm-post)"');
    });
    s += n5r(24, 68, 16, 390, 'fill="#000" filter="url(#cm-post)"') + n5r(482, 68, 16, 390, 'fill="#000" filter="url(#cm-post)"') + n5r(20, 64, 482, 14, 'fill="#000" filter="url(#cm-beam)"');
    return { art: '<g filter="url(#cm-unitlit)">' + s + '</g>', slots: cmGrid(), zone: [34, 96, 454, 320], wallsh: true, decor: true, label: 'The cask rack' };
  };
  COUNTERS.bar = function (t) {
    var s = cmTop(t, 'M-300,452 H1500 V512 H-300 Z'), r = rng(51), x, g;
    s += '<clipPath id="cm-topclip"><rect x="-300" y="452" width="1800" height="60"/></clipPath>' + n5p('M-300,506 H1500 V518 H-300 Z', 'fill="#000" filter="url(#cm-beam)"') + n5r(-300, 506, 1800, 2.4, 'fill="' + t.lc + '" opacity=".45"') + n5r(-300, 518, 1800, 382, 'fill="#000" filter="url(#cm-front)"');
    // staves like the side of a great tun, with two iron hoops
    for (x = -300; x < 1500; x += 52) s += n5r(x, 518, 52, 382, 'fill="' + (r() < .5 ? '#000' : '#ffffff') + '" opacity="' + f1(r() * .1) + '"') + n5r(x + 51, 518, 1.6, 382, 'fill="#070402" opacity=".8"');
    g = '<defs>' + n5Metals() + '</defs>';
    [552, 644].forEach(function (y) { g += n5r(-300, y + 14, 1800, 10, 'fill="#000" opacity=".5" filter="url(#src-blur3)"') + n5r(-300, y, 1800, 16, 'fill="#17181b"') + n5r(-300, y, 1800, 2, 'fill="#9aa0a8" opacity=".5"') + n5r(-300, y + 13, 1800, 3, 'fill="#000" opacity=".6"'); for (x = -280; x < 1500; x += 104) g += n5c(x, y + 8, 3.2, 'fill="#5a5e66"') + n5c(x - 1, y + 7, 1.2, 'fill="#d0d4da"'); });
    // the foot rail, and a cloth over the edge
    for (x = -220; x < 1500; x += 240) g += n5p('M' + x + ',700 V682 Q' + x + ',672 ' + (x + 12) + ',672', 'stroke="' + mix(t.th.trim, '#000', .5) + '" stroke-width="7" fill="none"');
    g += n5r(-300, 666, 1800, 11, 'rx="5" fill="url(#cm-trimv)"') + n5r(-300, 667.5, 1800, 2, 'fill="#fff" opacity=".5"') + n5r(-300, 680, 1800, 12, 'fill="#000" opacity=".5" filter="url(#src-blur5)"');
    g += n5p('M214,500 H288 V560 Q270,570 251,562 Q232,554 214,566 Z', 'fill="' + mix(t.th.cloth, '#e8dcc0', .7) + '"') + n5p('M214,500 H288 V512 H214 Z', 'fill="#000" opacity=".18"') + n5p('M228,512 V558 M246,512 V560 M266,512 V562', 'stroke="' + t.th.cloth + '" stroke-opacity=".7" stroke-width="3" fill="none"');
    return '<g filter="url(#cm-woodlit)">' + s + g + n5r(-300, 518, 1800, 30, 'fill="#000" opacity=".5" filter="url(#src-blur8)"') + '</g>';
  };
  PROPS.tankards = function (t, p) {
    var s = '<defs>' + n5Metals() + '</defs>', xs = [520, 690, 742, 794, 1070, 1118, 1166], r = rng(63);
    s += n5r(500, 60, 690, 6, 'fill="#17110c"') + n5r(500, 60, 690, 1.4, 'fill="' + t.lc + '" opacity=".3"');
    xs.forEach(function (x, j) {
      var h = 30 + r() * 8, w = 13 + r() * 3, y = 78, cu = j % 3 === 1;
      s += n5g('class="n5-vb n5-sway" style="transform-origin:' + x + 'px 66px;animation-delay:-' + f1(r() * 6) + 's;animation-duration:' + f1(5 + r() * 3) + 's"',
        n5p('M' + x + ',64 v8 q0,6 -6,6', 'stroke="#2a2a2e" stroke-width="2.4" fill="none"') +
        n5p('M' + (x - 6) + ',' + y + ' q-14,2 -13,' + f1(h * .5) + ' q1,' + f1(h * .4) + ' 13,' + f1(h * .4), 'stroke="url(#' + (cu ? 'n5-copper' : 'n5-pewter') + ')" stroke-width="3.4" fill="none"') +
        n5p('M' + (x - 6) + ',' + y + ' H' + f1(x - 6 + w * 2) + ' L' + f1(x - 4 + w * 2) + ',' + f1(y + h) + ' H' + (x - 8) + ' Z', 'fill="url(#' + (cu ? 'n5-copper' : 'n5-pewter') + ')"') +
        n5r(x - 8, y + h - 5, w * 2 + 4, 5, 'rx="1.5" fill="#000" opacity=".3"') + n5r(x - 7, y + 3, w * 2 + 2, 2, 'fill="#000" opacity=".25"') + n5e(x - 6 + w, y, w, 2.6, 'fill="#101216"'));
    });
    return s;
  };
  PROPS.keg = function (t, p) {
    var x = p.x, y = 492, s = '<defs>' + n5Metals() + lin('n5-kegb', 1, [[0, '#a87c48'], [.5, '#7a5430'], [1, '#2a180a']]) + '</defs>', i;
    s += n5e(x + 4, y + 3, 70, 9, 'fill="url(#src-ao)"');
    // the cradle, then the barrel lying on it, head toward you
    s += n5p('M' + (x - 50) + ',' + y + ' l10,-26 h80 l10,26 Z', 'fill="' + mix(t.th.wood, '#000', .45) + '"') + n5p('M' + (x - 40) + ',' + (y - 26) + ' h80', 'stroke="' + t.lc + '" stroke-opacity=".3" stroke-width="1.4"');
    s += n5e(x + 12, y - 58, 52, 40, 'fill="url(#n5-kegb)"') + n5e(x - 4, y - 58, 44, 40, 'fill="' + mix(t.th.wood, '#c89a60', .5) + '"') + n5e(x - 4, y - 58, 44, 40, 'fill="url(#n5-kegb)" opacity=".55"');
    for (i = -2; i <= 2; i++) s += n5p('M' + (x - 46) + ',' + (y - 58 + i * 15) + ' h84', 'stroke="#1a0e06" stroke-opacity=".55" stroke-width="1.2"');
    s += n5e(x - 4, y - 58, 44, 40, 'fill="none" stroke="url(#n5-black)" stroke-width="5"') + n5e(x - 4, y - 58, 36, 32.6, 'fill="none" stroke="#000" stroke-opacity=".4" stroke-width="1.6"') + n5p('M' + (x + 30) + ',' + (y - 92) + ' q26,6 30,34 M' + (x + 46) + ',' + (y - 84) + ' q14,10 14,26', 'stroke="#101114" stroke-width="5" fill="none"');
    // the tap, a slow drip, and a tankard set under it
    s += n5r(x - 10, y - 46, 12, 9, 'rx="2" fill="url(#cm-trim)"') + n5r(x - 6, y - 37, 5, 9, 'fill="url(#cm-trim)"') + n5p('M' + (x - 14) + ',' + (y - 50) + ' h18', 'stroke="url(#cm-trim)" stroke-width="3.4" stroke-linecap="round"');
    s += n5c(x - 3.5, y - 26, 1.8, 'class="cm-drip" fill="#e8b860" style="animation-duration:2.4s"');
    s += n5p('M' + (x - 15) + ',' + (y - 20) + ' H' + (x + 8) + ' L' + (x + 6) + ',' + (y + 1) + ' H' + (x - 13) + ' Z', 'fill="url(#n5-pewter)"') + n5p('M' + (x + 7) + ',' + (y - 16) + ' q10,1 9,8 q-1,7 -9,6', 'stroke="url(#n5-pewter)" stroke-width="3" fill="none"') + n5e(x - 3.5, y - 20, 11.5, 2.6, 'fill="#f4ecd8"') + n5c(x - 8, y - 22, 3, 'fill="#fff8e8"') + n5c(x - 1, y - 23, 3.6, 'fill="#fff8e8"');
    return n5g('class="src-nohit"', s);
  };
  PROPS.antlers = function (t, p) {
    var x = 1118, y = 150, s = '', k;
    function tine(sx) {
      var b = 'M' + x + ',' + (y - 6) + ' q' + (sx * 14) + ',-8 ' + (sx * 22) + ',-34 q' + (sx * 6) + ',-22 ' + (sx * 2) + ',-46';
      return n5p(b, 'stroke="#d8c8a8" stroke-width="5.4" fill="none" stroke-linecap="round"') + n5p('M' + (x + sx * 17) + ',' + (y - 24) + ' q' + (sx * 14) + ',-4 ' + (sx * 20) + ',-22 M' + (x + sx * 23) + ',' + (y - 46) + ' q' + (sx * 12) + ',-6 ' + (sx * 14) + ',-22 M' + (x + sx * 9) + ',' + (y - 12) + ' q' + (sx * 2) + ',-12 ' + (-sx * 3) + ',-20', 'stroke="#d8c8a8" stroke-width="3.8" fill="none" stroke-linecap="round"') +
        n5p(b, 'stroke="#7a6a50" stroke-width="1.4" fill="none" stroke-opacity=".6" transform="translate(' + (sx * 1.6) + ',1.6)"');
    }
    s += n5p('M' + (x - 22) + ',' + (y - 12) + ' h44 v20 q0,22 -22,34 q-22,-12 -22,-34 Z', 'fill="#000" opacity=".5" filter="url(#src-blur5)" transform="translate(-6,5)"') + n5p('M' + (x - 22) + ',' + (y - 12) + ' h44 v20 q0,22 -22,34 q-22,-12 -22,-34 Z', 'fill="' + mix(t.th.wood, '#000', .3) + '" stroke="' + mix(t.th.wood, '#ffffff', .25) + '" stroke-opacity=".5" stroke-width="1.4"');
    s += tine(-1) + tine(1) + n5e(x, y - 4, 9, 7, 'fill="#bfae8c"');
    return n5g('class="src-nohit"', s);
  };


  /* ---- The tailor: papered walls above a dark dado, a tall window with
     heavy drapes, bolts of cloth for shelves and a cutting table for a
     counter. ---- */
  CM_OPT.shape.push('parlour'); CM_OPT.wall.push('damask'); CM_OPT.opening.push('bay-window'); CM_OPT.display.push('bolts'); CM_OPT.counter.push('cutting');
  CM_WALLFQ.damask = [.05, 2.2, 6]; CM_OPEN_INFO['bay-window'] = 'The tall window';
  n5prop('mannequin', 'tallR', 'front', 0, true); n5prop('ribbons', 'ceil', 'back'); n5prop('shears', 'cL', 'front', 130);

  WALLS.damask = function (t) {
    var base = mix(t.th.cloth, '#2c4640', .55), fig = mix(base, '#e8e0c0', .2), r = rng(71), s, x,
      m = 'M0,-48 C14,-34 30,-22 30,-4 C30,12 14,18 0,40 C-14,18 -30,12 -30,-4 C-30,-22 -14,-34 0,-48 Z M0,-30 C-8,-20 -16,-12 -16,-2 C-16,8 -8,12 0,24 C8,12 16,8 16,-2 C16,-12 8,-20 0,-30 Z M0,46 l7,10 l-7,10 l-7,-10 Z M-40,26 a5,5 0 1 0 .1,0 Z M40,26 a5,5 0 1 0 .1,0 Z';
    function mot(x, y) { return '<path transform="translate(' + x + ',' + y + ')" fill-rule="evenodd" d="' + m + '" fill="' + fig + '"/>'; }
    s = '<defs><pattern id="n5-dmk" width="96" height="140" patternUnits="userSpaceOnUse">' + mot(48, 70) + mot(0, 0) + mot(96, 0) + mot(0, 140) + mot(96, 140) + '</pattern></defs>';
    s += '<g filter="url(#cm-wall)"><rect ' + FULL + ' fill="' + base + '"/><rect ' + FULL + ' fill="url(#n5-dmk)"/></g><rect ' + FULL + ' fill="#1a120c" filter="url(#cm-stain)" opacity=".4"/>';
    // the dado: dark panelling to hip height under a moulded rail
    s += n5r(-300, 338, 1800, 132, 'fill="#000" filter="url(#cm-board)"') + n5r(-300, 338, 1800, 132, 'fill="#0a0604" opacity=".3"');
    for (x = -290; x < 1500; x += 118) s += n5r(x + 12, 362, 94, 96, 'fill="none" stroke="#050302" stroke-opacity=".8" stroke-width="3"') + n5r(x + 15, 365, 94, 96, 'fill="none" stroke="' + t.lc + '" stroke-opacity=".14" stroke-width="1.4"') + n5r(x + 22, 372, 74, 76, 'fill="#000" opacity=".16"');
    s += n5r(-300, 326, 1800, 14, 'fill="#000" filter="url(#cm-beam)"') + n5r(-300, 326, 1800, 2, 'fill="' + t.lc + '" opacity=".4"') + n5r(-300, 340, 1800, 10, 'fill="#000" opacity=".45" filter="url(#src-blur3)"');
    return s;
  };
  SHAPES.parlour = function (t) {
    var s = WALLS[t.th.wall](t), x, i, c = mix(t.th.cloth, '#000', .15), cl = mix(t.th.cloth, '#ffffff', .22), d, g = '', tr = t.th.trim;
    // a plaster ceiling, its cornice and a row of dentils
    s += n5r(-300, -150, 1800, 168, 'fill="#d8ccb4" filter="url(#cm-wall)"') + n5r(-300, 18, 1800, 12, 'fill="#efe6d2"') + n5r(-300, 30, 1800, 5, 'fill="#8a7e68"') + n5r(-300, 35, 1800, 12, 'fill="#000" opacity=".4" filter="url(#src-blur3)"');
    for (x = -296; x < 1500; x += 24) s += n5r(x, 2, 14, 14, 'fill="#f4ecda"') + n5r(x + 14, 2, 3, 14, 'fill="#000" opacity=".28"');
    // a pelmet of velvet swags, fringed, with a tail where two meet
    for (i = 0; i < 9; i++) {
      x = -200 + i * 178; d = 'M' + x + ',34 Q' + (x + 89) + ',128 ' + (x + 178) + ',34 Z';
      g += n5sh(d, 0, 8, .5, 5) + n5p(d, 'fill="' + c + '"');
      [.3, .55, .8].forEach(function (k) { g += n5p('M' + (x + 8) + ',' + (36 + k * 4) + ' Q' + (x + 89) + ',' + f1(34 + k * 94) + ' ' + (x + 170) + ',' + (36 + k * 4), 'stroke="' + cl + '" stroke-opacity=".4" stroke-width="2" fill="none"') + n5p('M' + (x + 8) + ',' + (40 + k * 4) + ' Q' + (x + 89) + ',' + f1(40 + k * 94) + ' ' + (x + 170) + ',' + (40 + k * 4), 'stroke="#000" stroke-opacity=".35" stroke-width="3" fill="none"'); });
      g += n5p('M' + x + ',34 Q' + (x + 89) + ',128 ' + (x + 178) + ',34', 'stroke="' + tr + '" stroke-width="6" stroke-dasharray="1.6 2.6" fill="none"') + n5p('M' + x + ',34 Q' + (x + 89) + ',128 ' + (x + 178) + ',34', 'stroke="' + mix(tr, '#fff', .4) + '" stroke-width="1.4" fill="none"');
    }
    for (i = 0; i < 10; i++) { x = -200 + i * 178; g += n5p('M' + (x - 16) + ',34 H' + (x + 16) + ' L' + (x + 12) + ',84 L' + (x + 4) + ',70 L' + x + ',104 L' + (x - 4) + ',70 L' + (x - 12) + ',84 Z', 'fill="' + mix(c, '#000', .25) + '"') + n5p('M' + x + ',36 V100', 'stroke="' + cl + '" stroke-opacity=".3" stroke-width="2"') + n5c(x, 36, 7, 'fill="' + tr + '"') + n5c(x - 2, 34, 2.4, 'fill="#fff" opacity=".6"'); }
    return s + g + cmPost(-6, 34, 34, 474) + cmPost(1172, 34, 34, 474);
  };
  OPENINGS['bay-window'] = function (t) {
    var d = 'M808,306 V132 Q808,72 912,72 Q1016,72 1016,132 V306 Z', r = rng(77), c = mix(t.th.cloth, '#000', .1), cl = mix(t.th.cloth, '#fff', .25), s, fr, i, k, cloud = '', dark = t.time === 'night';
    s = cmSky(t, 'n5-bsky', 800, 64, 224, 250);
    // slow cloud, two sets a window apart so the loop has no seam
    [0, 260].forEach(function (o) { [[820, 110, 40], [900, 96, 54], [990, 128, 36]].forEach(function (q) { cloud += n5e(q[0] + o, q[1], q[2], q[2] * .3, 'fill="#fff" opacity="' + (dark ? .08 : .5) + '" filter="url(#src-blur5)"'); }); });
    s += n5g('class="n5-cloud" style="--sx:-260px;animation-duration:70s"', cloud);
    // the houses across the street, roofs and chimneys, a lit window or two
    s += cmSkyline(800, 150, 224, 110, dark ? '#0c1020' : '#5a5868', 9) + cmSkyline(800, 190, 224, 80, dark ? '#070a14' : '#3e3c4a', 4) + n5r(800, 268, 224, 40, 'fill="' + (dark ? '#05060c' : '#2c2a34') + '"');
    s += cmRainIn(t, 800, 64, 224, 250, 5);
    // lace across the lower lights
    for (i = 0; i < 15; i++) for (k = 0; k < 5; k++) s += n5c(815 + i * 14 + (k % 2) * 7, 240 + k * 13, 4.4, 'fill="none" stroke="#fff" stroke-opacity=".5" stroke-width="1.2"');
    s += n5r(808, 228, 208, 80, 'fill="#fff" opacity=".2"') + n5p('M808,230 q7,9 14,0 t14,0 t14,0 t14,0 t14,0 t14,0 t14,0 t14,0 t14,0 t14,0 t14,0 t14,0 t14,0 t14,0 t14,0', 'stroke="#fff" stroke-opacity=".7" stroke-width="2" fill="none"');
    s += n5p('M820,290 L900,80 h40 L860,290 Z', 'fill="#fff" opacity=".07"');
    fr = n5p('M912,72 V306 M808,150 H1016 M808,228 H1016', 'stroke="' + mix(t.th.wood, '#000', .45) + '" stroke-width="6" fill="none"') + n5p('M912,72 V306 M808,150 H1016 M808,228 H1016', 'stroke="' + mix(t.th.wood, '#fff', .2) + '" stroke-width="1.2" stroke-opacity=".5" fill="none" transform="translate(-2,-2)"') + cmFrame(t, d, 14);
    // the sill, with the spools left on it
    fr += n5r(794, 304, 236, 14, 'rx="2" fill="#000" filter="url(#cm-beam)"') + n5r(794, 304, 236, 2, 'fill="' + t.lc + '" opacity=".5"') + n5r(794, 318, 236, 12, 'fill="#000" opacity=".5" filter="url(#src-blur3)"');
    ['#b83a3a', '#3a6ab8', '#d8b040', '#4a8a5a', '#8a4a9a'].forEach(function (col, j) { var x = 852 + j * 30, h = 15 + (j % 2) * 4; fr += n5r(x - 8, 302 - h - 3, 16, 3, 'fill="#c8b088"') + n5r(x - 6, 302 - h, 12, h, 'fill="' + col + '"') + n5r(x - 6, 302 - h, 4, h, 'fill="#fff" opacity=".25"') + n5r(x - 8, 301, 16, 3, 'fill="#a89068"'); });
    // the drapes, gathered back to a cord at each side, and the pole they hang from
    function drape(k) {
      var q = function (x) { return 912 + k * x; }, o = 'M' + q(138) + ',56 H' + q(66) + ' C' + q(62) + ',150 ' + q(70) + ',204 ' + q(104) + ',244 C' + q(72) + ',270 ' + q(68) + ',300 ' + q(70) + ',338 H' + q(136) + ' Z', g = n5sh(o, -6, 6, .5, 8) + n5p(o, 'fill="' + c + '"'), j;
      for (j = 0; j < 5; j++) g += n5p('M' + q(130 - j * 13) + ',58 C' + q(128 - j * 12) + ',150 ' + q(124 - j * 6) + ',210 ' + q(112) + ',244 C' + q(118 - j * 4) + ',280 ' + q(128 - j * 11) + ',300 ' + q(130 - j * 12) + ',336', 'stroke="' + (j % 2 ? '#000' : cl) + '" stroke-opacity="' + (j % 2 ? .4 : .32) + '" stroke-width="' + (j % 2 ? 5 : 2.4) + '" fill="none"');
      return g + n5p('M' + q(136) + ',240 Q' + q(112) + ',256 ' + q(98) + ',240', 'stroke="' + t.th.trim + '" stroke-width="5" fill="none" stroke-linecap="round"') + n5p('M' + q(100) + ',244 q' + (k * -2) + ',16 ' + (k * 2) + ',30', 'stroke="' + t.th.trim + '" stroke-width="3" fill="none"') + n5e(q(98), 280, 4.4, 9, 'fill="' + t.th.trim + '"');
    }
    fr += drape(-1) + drape(1) + n5r(760, 50, 304, 7, 'rx="3.5" fill="url(#cm-trimv)"') + n5c(758, 53.5, 8, 'fill="url(#cm-trim)"') + n5c(1066, 53.5, 8, 'fill="url(#cm-trim)"');
    return mpOpen(d, s, fr, dark ? '' : n5e(912, 200, 240, 200, 'fill="#fff8e0" opacity=".05" filter="url(#src-blur14)" pointer-events="none"'));
  };
  DISPLAYS.bolts = function (t) {
    var r = rng(81), s = cmUnitFrame(t, .55), cols = ['#8a2e2e', '#2e4e7a', '#3e6a44', '#a07a2e', '#5e2e62', '#9a9488', '#2e6a6a', '#7a4426', '#b8a078', '#44305a'], x, w, h, c, a;
    // each shelf is stood full of bolts, their rolled ends up
    CM_ROWS.forEach(function (ry) {
      for (x = 46; x < 470; x += w + 3) {
        w = 26 + r() * 16; h = 70 + r() * 26; c = mix(cols[Math.floor(r() * cols.length)], '#0a0806', .42); a = f1(r() * 6 - 3);
        s += '<g transform="rotate(' + a + ' ' + f1(x + w / 2) + ' ' + ry + ')">' + n5r(x, ry - h, w, h, 'rx="3" fill="' + c + '"') + n5r(x, ry - h, w * .3, h, 'fill="#000" opacity=".3"') + n5r(x + w * .56, ry - h, w * .14, h, 'fill="#fff" opacity=".12"') +
          n5e(x + w / 2, ry - h, w / 2, 5, 'fill="' + mix(c, '#e8dcc0', .35) + '"') + n5e(x + w / 2, ry - h, w / 4, 2.4, 'fill="none" stroke="#000" stroke-opacity=".5" stroke-width="1"') + n5c(x + w / 2, ry - h, 1.6, 'fill="#c8b088"') +
          (r() < .5 ? n5r(x + 3, ry - h * .5, w - 6, 10, 'fill="#e0d4b4" opacity=".7"') + n5r(x + 6, ry - h * .5 + 4, w - 12, 1.4, 'fill="#3a2a1a" opacity=".6"') : '') + '</g>';
      }
      s += n5r(40, ry - 100, 442, 100, 'fill="#000" opacity=".2"') + cmShelfBoard(t, ry, 40, 482);
    });
    return { art: '<g filter="url(#cm-unitlit)">' + s + '</g>', slots: cmGrid(), zone: [34, 96, 454, 320], wallsh: true, decor: true, label: 'The cloth shelves' };
  };
  COUNTERS.cutting = function (t) {
    var s = cmTop(t, 'M-300,452 H1500 V512 H-300 Z'), r = rng(85), x, y, g = '', i, tr = t.th.trim, hi = mix(t.th.wood, '#ffffff', .22);
    s += '<clipPath id="cm-topclip"><rect x="-300" y="452" width="1800" height="60"/></clipPath>';
    // a brass yard let into the front edge of the top
    s += n5r(300, 496, 660, 9, 'fill="url(#cm-trimv)"') + n5r(300, 496, 660, 1.4, 'fill="#fff" opacity=".5"');
    for (i = 0; i <= 108; i++) s += n5r(302 + i * 6.08, 496, 1, i % 12 === 0 ? 9 : i % 6 === 0 ? 6 : 3.4, 'fill="#2a1c0a"');
    s += n5p('M-300,506 H1500 V520 H-300 Z', 'fill="#000" filter="url(#cm-beam)"') + n5r(-300, 506, 1800, 2.4, 'fill="' + t.lc + '" opacity=".45"') + n5r(-300, 520, 1800, 380, 'fill="#000" filter="url(#cm-front)"');
    // a row of drawers under the top, then cupboards
    for (x = -292; x < 1500; x += 186) {
      g += n5r(x + 6, 534, 174, 62, 'fill="none" stroke="#040201" stroke-opacity=".85" stroke-width="3"') + n5r(x + 9, 537, 174, 62, 'fill="none" stroke="' + hi + '" stroke-opacity=".3" stroke-width="1.4"') + n5r(x + 14, 542, 158, 46, 'fill="#000" opacity=".14"');
      g += n5p('M' + (x + 74) + ',560 q19,16 38,0', 'stroke="' + tr + '" stroke-width="5" fill="none" stroke-linecap="round"') + n5p('M' + (x + 74) + ',560 q19,16 38,0', 'stroke="#fff" stroke-opacity=".4" stroke-width="1.2" fill="none"') + n5e(x + 93, 574, 22, 5, 'fill="#000" opacity=".35" filter="url(#src-blur3)"');
      g += n5r(x + 6, 610, 84, 300, 'fill="none" stroke="#040201" stroke-opacity=".85" stroke-width="3"') + n5r(x + 96, 610, 84, 300, 'fill="none" stroke="#040201" stroke-opacity=".85" stroke-width="3"') + n5r(x + 18, 624, 60, 280, 'fill="#000" opacity=".16"') + n5r(x + 108, 624, 60, 280, 'fill="#000" opacity=".16"') + n5r(x + 9, 613, 84, 300, 'fill="none" stroke="' + hi + '" stroke-opacity=".22" stroke-width="1.2"') + n5r(x + 99, 613, 84, 300, 'fill="none" stroke="' + hi + '" stroke-opacity=".22" stroke-width="1.2"') + n5c(x + 84, 664, 4, 'fill="' + tr + '"') + n5c(x + 102, 664, 4, 'fill="' + tr + '"');
    }
    // a tape left hanging over the edge
    g += n5p('M742,486 C744,500 738,512 742,540 C748,580 730,600 738,640', 'stroke="#000" stroke-opacity=".4" stroke-width="9" fill="none" filter="url(#src-blur3)" transform="translate(4,3)"') + n5p('M742,486 C744,500 738,512 742,540 C748,580 730,600 738,640', 'stroke="#e8cc6a" stroke-width="8" fill="none"') + n5p('M742,486 C744,500 738,512 742,540 C748,580 730,600 738,640', 'stroke="#3a2a10" stroke-width="5" stroke-dasharray="1 5" fill="none" stroke-opacity=".7"') + n5r(733, 638, 11, 8, 'fill="url(#cm-trim)"');
    return '<g filter="url(#cm-woodlit)">' + s + g + n5r(-300, 520, 1800, 30, 'fill="#000" opacity=".45" filter="url(#src-blur8)"') + '</g>';
  };
  PROPS.mannequin = function (t, p) {
    var x = p.x, c = mix(t.th.cloth, '#0a0808', .1), cl = mix(t.th.cloth, '#ffffff', .3), s, i,
      body = 'M' + (x - 34) + ',262 Q' + (x - 41) + ',262 ' + (x - 43) + ',272 L' + (x - 37) + ',300 Q' + (x - 41) + ',322 ' + (x - 30) + ',346 Q' + (x - 24) + ',364 ' + (x - 36) + ',404 L' + (x + 36) + ',404 Q' + (x + 24) + ',364 ' + (x + 30) + ',346 Q' + (x + 41) + ',322 ' + (x + 37) + ',300 L' + (x + 43) + ',272 Q' + (x + 41) + ',262 ' + (x + 34) + ',262 Q' + (x + 12) + ',270 ' + (x + 10) + ',248 L' + (x - 10) + ',248 Q' + (x - 12) + ',270 ' + (x - 34) + ',262 Z',
      coat = 'M' + x + ',270 L' + (x + 34) + ',262 Q' + (x + 41) + ',262 ' + (x + 43) + ',272 L' + (x + 37) + ',300 Q' + (x + 41) + ',322 ' + (x + 30) + ',346 Q' + (x + 24) + ',364 ' + (x + 36) + ',404 L' + (x + 40) + ',420 L' + x + ',424 Z';
    s = '<defs>' + lin('n5-form', 0, [[0, '#fff', .22], [.4, '#fff', 0], [.7, '#000', .1], [1, '#000', .5]]) + '</defs>' + n5e(x + 4, 480, 46, 8, 'fill="url(#src-ao)"');
    // the stand: a turned post on a round foot
    s += n5e(x, 476, 30, 7, 'fill="' + mix(t.th.wood, '#000', .35) + '"') + n5e(x, 474, 30, 7, 'fill="' + mix(t.th.wood, '#fff', .08) + '"') + n5r(x - 4, 404, 8, 70, 'fill="' + mix(t.th.wood, '#000', .2) + '"') + n5r(x - 4, 404, 2.6, 70, 'fill="#fff" opacity=".2"') + n5e(x, 440, 8, 5, 'fill="' + mix(t.th.wood, '#fff', .1) + '"') + n5e(x, 462, 10, 5, 'fill="' + mix(t.th.wood, '#000', .1) + '"');
    // the form in linen, and half a coat pinned to it
    s += n5sh(body, -10, 4, .45, 8) + n5p(body, 'fill="#cdbb9c"') + n5p('M' + x + ',250 V404', 'stroke="#8a7a5c" stroke-width="1.2" stroke-dasharray="3 3"') + n5p(coat, 'fill="' + c + '"') + n5p('M' + x + ',270 L' + (x + 20) + ',266 L' + (x + 5) + ',326 Z', 'fill="' + cl + '" opacity=".55"') + n5p('M' + (x + 17) + ',322 V416', 'stroke="#fff" stroke-opacity=".6" stroke-width="1.4" stroke-dasharray="5 4"') + n5p('M' + (x + 4) + ',424 L' + (x + 40) + ',420', 'stroke="#fff" stroke-opacity=".5" stroke-width="1.2" stroke-dasharray="2 4"');
    s += n5p(body, 'fill="url(#n5-form)"') + n5p(coat, 'fill="url(#n5-form)" opacity=".6"');
    for (i = 0; i < 4; i++) s += n5p('M' + (x - 5) + ',' + (296 + i * 30) + ' l10,-3', 'stroke="#d8dce0" stroke-width="1.2"') + n5c(x - 5.5, 296 + i * 30, 2.2, 'fill="' + (i % 2 ? '#f4f0e8' : t.th.accent) + '"');
    s += n5r(x - 10, 240, 20, 10, 'fill="#b8a684"') + n5e(x, 240, 11, 4, 'fill="url(#cm-trim)"') + n5c(x, 236, 4, 'fill="url(#cm-trim)"');
    // a tape round its neck that stirs when the door opens
    s += n5g('class="n5-vb n5-sway" style="transform-origin:' + x + 'px 256px;animation-duration:7s"', n5p('M' + (x - 12) + ',254 Q' + x + ',268 ' + (x + 12) + ',254', 'stroke="#e8cc6a" stroke-width="5" fill="none"') + n5p('M' + (x - 11) + ',256 C' + (x - 16) + ',300 ' + (x - 10) + ',330 ' + (x - 15) + ',372', 'stroke="#e8cc6a" stroke-width="5" fill="none"') + n5p('M' + (x - 11) + ',256 C' + (x - 16) + ',300 ' + (x - 10) + ',330 ' + (x - 15) + ',372', 'stroke="#3a2a10" stroke-width="3" stroke-dasharray="1 5" fill="none" stroke-opacity=".7"') + n5p('M' + (x + 11) + ',256 C' + (x + 15) + ',290 ' + (x + 9) + ',310 ' + (x + 13) + ',340', 'stroke="#e8cc6a" stroke-width="5" fill="none"') + n5p('M' + (x + 11) + ',256 C' + (x + 15) + ',290 ' + (x + 9) + ',310 ' + (x + 13) + ',340', 'stroke="#3a2a10" stroke-width="3" stroke-dasharray="1 5" fill="none" stroke-opacity=".7"'));
    return n5g('class="src-nohit"', s);
  };
  PROPS.ribbons = function (t, p) {
    var r = rng(87), s = n5r(500, 58, 290, 5, 'rx="2.5" fill="url(#cm-trimv)"') + n5r(1052, 58, 140, 5, 'rx="2.5" fill="url(#cm-trimv)"'), cols = ['#c83a4a', '#e8c860', '#4a8ac8', '#e8e0d0', '#7a4a9a', '#4aa878', '#e88a4a'];
    [[516, 96], [704, 104], [742, 150], [1070, 132], [1106, 92], [1142, 168], [1178, 112]].forEach(function (q, j) {
      var x = q[0], L = q[1], c = cols[j % cols.length], d = 'M' + (x - 4) + ',70 q' + f1(6 + r() * 6) + ',' + f1(L * .3) + ' 0,' + f1(L * .55) + ' t-2,' + f1(L * .45);
      s += n5r(x - 9, 50, 18, 20, 'rx="3" fill="' + mix(c, '#000', .15) + '"') + n5r(x - 9, 50, 5, 20, 'fill="#fff" opacity=".25"') + n5r(x - 11, 48, 22, 3, 'fill="#c8b088"') + n5r(x - 11, 69, 22, 3, 'fill="#a89068"');
      s += n5g('class="n5-vb n5-sway" style="transform-origin:' + x + 'px 70px;animation-duration:' + f1(5 + r() * 4) + 's;animation-delay:-' + f1(r() * 6) + 's"', n5p(d, 'stroke="#000" stroke-opacity=".35" stroke-width="9" fill="none" filter="url(#src-blur3)" transform="translate(-5,3)"') + n5p(d, 'stroke="' + c + '" stroke-width="8" fill="none"') + n5p(d, 'stroke="#fff" stroke-opacity=".3" stroke-width="1.6" fill="none" transform="translate(-2,0)"'));
    });
    return n5g('class="src-nohit"', s);
  };
  PROPS.shears = function (t, p) {
    var x = p.x, y = 492, s = '<defs>' + n5Metals() + '</defs>' + n5e(x, y + 4, 62, 9, 'fill="url(#src-ao)"');
    // shears lying open, a pin cushion and a wedge of chalk
    s += n5p('M' + (x - 54) + ',' + (y - 2) + ' L' + (x + 10) + ',' + (y - 14) + ' L' + (x + 14) + ',' + (y - 8) + ' Z', 'fill="url(#n5-pewter)"') + n5p('M' + (x - 50) + ',' + (y - 16) + ' L' + (x + 12) + ',' + (y - 4) + ' L' + (x + 8) + ',' + (y + 2) + ' Z', 'fill="url(#n5-pewter)"') + n5c(x + 2, y - 8, 2.6, 'fill="#2a2a2e"');
    s += n5e(x + 28, y - 18, 13, 7, 'fill="none" stroke="#141416" stroke-width="5" transform="rotate(-18 ' + (x + 28) + ' ' + (y - 18) + ')"') + n5e(x + 28, y + 2, 13, 7, 'fill="none" stroke="#141416" stroke-width="5" transform="rotate(16 ' + (x + 28) + ' ' + (y + 2) + ')"');
    s += n5c(x - 36, y - 18, 3, 'class="n5-fb n5-glint" fill="#fff"');
    s += n5e(x + 58, y - 2, 16, 4, 'fill="#000" opacity=".4"') + n5p('M' + (x + 42) + ',' + (y - 4) + ' q0,-20 16,-20 q16,0 16,20 q-16,8 -32,0 Z', 'fill="#b8322e"') + n5p('M' + (x + 58) + ',' + (y - 24) + ' v20 M' + (x + 50) + ',' + (y - 22) + ' q-3,10 -2,19 M' + (x + 66) + ',' + (y - 22) + ' q3,10 2,19', 'stroke="#5a1410" stroke-width="1.2" fill="none"');
    [[50, -34, '#f4f0e8'], [58, -38, '#e8c860'], [66, -33, '#6ab0e8']].forEach(function (q) { s += n5p('M' + (x + q[0]) + ',' + (y + q[1]) + ' v14', 'stroke="#d8dce0" stroke-width="1.1"') + n5c(x + q[0], y + q[1], 2.4, 'fill="' + q[2] + '"'); });
    s += n5p('M' + (x - 30) + ',' + (y + 8) + ' l16,-7 l8,9 Z', 'fill="#e8e4dc"') + n5p('M' + (x - 8) + ',' + (y + 10) + ' l14,-5 l6,8 Z', 'fill="#8ab0d8"');
    return n5g('class="src-nohit"', s);
  };

  /* ---- The mapmaker: the wall is one great painted chart, the window looks
     over a harbour, scrolls fill a lattice rack and the counter is a plan
     chest. ---- */
  CM_OPT.shape.push('study'); CM_OPT.wall.push('chart'); CM_OPT.opening.push('harbour-window'); CM_OPT.display.push('scroll-rack'); CM_OPT.counter.push('plan-chest');
  CM_WALLFQ.chart = [.04, 2, 12]; CM_OPEN_INFO['harbour-window'] = 'The harbour window';
  n5prop('globe', 'tallR', 'front', 0, true); n5prop('charts', 'ceil', 'back'); n5prop('inkwell', 'cL', 'front', 140);

  WALLS.chart = function (t) {
    var r = rng(91), ink = '#4a2e14', s, i, x, y, k, a, g = '';
    s = '<rect ' + FULL + ' fill="#cdb98c" filter="url(#cm-wall)"/><rect ' + FULL + ' fill="#3a2410" filter="url(#cm-stain)" opacity=".5"/>';
    // rhumb lines from two wind roses, and the sea's little waves
    [[1118, 190], [560, 420], [200, 120]].forEach(function (c) { for (i = 0; i < 16; i++) { a = i * Math.PI / 8; g += n5p('M' + c[0] + ',' + c[1] + ' l' + f1(Math.cos(a) * 1700) + ',' + f1(Math.sin(a) * 1700), 'stroke="' + ink + '" stroke-opacity="' + (i % 4 ? .13 : .26) + '" stroke-width="1"'); } });
    for (i = 0; i < 150; i++) { x = -280 + r() * 1760; y = 50 + r() * 410; g += n5p('M' + f1(x) + ',' + f1(y) + ' q4,-4 8,0 t8,0', 'stroke="' + ink + '" stroke-opacity=".3" stroke-width="1" fill="none"'); }
    // coasts: land washed in ochre, an inked shore with a hatched edge, hills and towns
    [[520, 300, 86, 14], [1150, 400, 96, 13], [760, 110, 60, 11], [1090, 60, 50, 10], [300, 380, 130, 15], [-60, 200, 150, 14], [1380, 200, 120, 13], [690, 440, 40, 9]].forEach(function (q) {
      var d = mpLump(r, q[0], q[1], q[2], q[3]);
      g += n5p(d, 'fill="#b08a4e" opacity=".5" stroke="none"') + n5p(d, 'fill="none" stroke="' + ink + '" stroke-width="2.2" stroke-linejoin="round"') + n5p(d, 'fill="none" stroke="' + ink + '" stroke-opacity=".5" stroke-width="7" stroke-dasharray="1 4" transform="translate(' + q[0] + ',' + q[1] + ') scale(1.06) translate(' + (-q[0]) + ',' + (-q[1]) + ')"');
      for (k = 0; k < 5; k++) { x = q[0] + (r() - .5) * q[2] * .9; y = q[1] + (r() - .5) * q[2] * .6; g += r() < .6 ? n5p('M' + f1(x - 7) + ',' + f1(y) + ' l7,-10 l7,10 M' + f1(x + 2) + ',' + f1(y) + ' l6,-7 l5,7', 'stroke="' + ink + '" stroke-width="1.4" fill="none"') : n5c(x, y, 2.6, 'fill="' + t.th.accent + '"') + n5p('M' + f1(x + 6) + ',' + f1(y) + ' h' + f1(14 + r() * 16), 'stroke="' + ink + '" stroke-width="1.2" stroke-dasharray="3 2"'); }
    });
    // the great rose, a serpent in the open sea, and the ruled border with its scale
    x = 1118; y = 190;
    g += n5c(x, y, 48, 'fill="#cdb98c" opacity=".6"') + n5c(x, y, 48, 'fill="none" stroke="' + ink + '" stroke-width="1.6"') + n5c(x, y, 40, 'fill="none" stroke="' + ink + '" stroke-width="1" stroke-dasharray="2 3"');
    for (i = 0; i < 8; i++) g += n5p('M' + x + ',' + (y - (i % 2 ? 30 : 58)) + ' l7,' + (i % 2 ? 30 : 58) + ' h-14 Z', 'fill="' + (i % 2 ? ink : i % 4 ? '#1e3a4a' : t.th.accent) + '" transform="rotate(' + (i * 45) + ' ' + x + ' ' + y + ')"') + n5p('M' + x + ',' + (y - (i % 2 ? 30 : 58)) + ' l-7,' + (i % 2 ? 30 : 58) + ' h7 Z', 'fill="#e8dcb8" stroke="' + ink + '" stroke-width=".8" transform="rotate(' + (i * 45) + ' ' + x + ' ' + y + ')"');
    g += n5c(x, y, 5, 'fill="' + ink + '"') + n5p('M' + (x - 5) + ',' + (y - 76) + ' l5,-12 l5,12 M' + (x - 3) + ',' + (y - 80) + ' h6', 'stroke="' + ink + '" stroke-width="2" fill="none"');
    g += n5p('M560,406 q14,-34 30,0 M606,410 q16,-40 34,0 M654,410 q12,-26 26,0', 'stroke="' + ink + '" stroke-width="5" fill="none" stroke-linecap="round"') + n5p('M560,406 q-18,-10 -12,-30 q6,-12 18,-6 l-6,6 l10,2', 'stroke="' + ink + '" stroke-width="4" fill="none" stroke-linecap="round"') + n5p('M680,410 q12,-4 16,-18 l4,10', 'stroke="' + ink + '" stroke-width="3" fill="none"') + n5c(554, 376, 1.6, 'fill="#e8dcb8"');
    g += '<text x="1056" y="330" font-family="Georgia,serif" font-style="italic" font-size="15" fill="' + ink + '" opacity=".8" letter-spacing="2">Mare Incognitum</text><text x="520" y="442" font-family="Georgia,serif" font-style="italic" font-size="13" fill="' + ink + '" opacity=".8" letter-spacing="1.5">here be serpents</text>';
    g += n5r(-300, 52, 1800, 9, 'fill="#e8dcb8" stroke="' + ink + '" stroke-width="1.4"');
    for (x = -300; x < 1500; x += 60) g += n5r(x, 52, 30, 9, 'fill="' + ink + '"');
    return s + n5g('opacity=".85"', g);
  };
  SHAPES.study = function (t) {
    var s = WALLS[t.th.wall](t), x;
    // a dark coffered ceiling on carved brackets, and a brass rail under it
    s += n5r(-300, -150, 1800, 176, 'fill="#000" filter="url(#cm-beam)"') + n5r(-300, -150, 1800, 176, 'fill="#0a0604" opacity=".3"') + n5r(-300, 26, 1800, 12, 'fill="#000" opacity=".5" filter="url(#src-blur3)"');
    for (x = -280; x < 1500; x += 120) s += n5p('M' + x + ',26 h40 v8 q-2,22 -20,26 q-18,-4 -20,-26 Z', 'fill="#000" filter="url(#cm-post)"') + n5p('M' + (x + 6) + ',34 q14,10 28,0', 'stroke="' + t.lc + '" stroke-opacity=".25" stroke-width="1.6" fill="none"') + n5r(x + 44, 30, 8, 30, 'fill="#000" opacity=".35" filter="url(#src-blur3)"');
    s += n5r(-300, 40, 1800, 4, 'fill="url(#cm-trimv)"');
    return s + cmPost(-6, 34, 26, 474) + cmPost(1172, 34, 26, 474);
  };
  OPENINGS['harbour-window'] = function (t) {
    var d = 'M812,306 V164 A100,98 0 0 1 1012,164 V306 Z', r = rng(93), dark = t.time !== '', sea = dark ? '#0c1c2c' : '#2a5a72', land = dark ? '#070b14' : '#27323a', s, fr, i, a;
    s = cmSky(t, 'n5-hsky', 800, 56, 224, 180) + '<defs>' + lin('n5-sea', 1, [[0, mix(sea, '#c8d8e0', .3)], [1, mix(sea, '#000', .4)]]) + '</defs>' + n5r(800, 226, 224, 90, 'fill="url(#n5-sea)"');
    // the headland and its light, turning
    s += n5p(n5ridge(r, 800, 900, 222, 22, 12, 232), 'fill="' + land + '"') + n5p('M846,204 l2,-34 h8 l2,34 Z', 'fill="' + (dark ? '#8a8a94' : '#e8e4dc') + '"') + n5r(846, 164, 12, 7, 'fill="#1a1a20"') + n5p('M844,164 l8,-8 l8,8 Z', 'fill="#5a1e16"');
    s += n5g('class="n5-vb n5-turn" style="transform-origin:852px 168px;animation-duration:11s"', n5p('M852,168 L1100,120 L1100,216 Z', 'fill="#fff6d0" opacity="' + (dark ? .34 : .12) + '"') + n5p('M852,168 L640,140 L640,196 Z', 'fill="#fff6d0" opacity="' + (dark ? .2 : .07) + '"')) + n5c(852, 168, 4, 'fill="#fff6d0"') + n5c(852, 168, 12, 'fill="#ffe8a0" opacity=".5" filter="url(#src-blur3)"');
    // light on the water, and two ships at anchor
    for (i = 0; i < 16; i++) s += n5r(806 + r() * 200, 232 + r() * 70, 10 + r() * 26, 1.4, 'class="n5-shimmer" fill="#fff" opacity=".4" style="animation-delay:-' + f1(r() * 3.4) + 's"');
    function ship(x, y, k, dl) { return n5g('class="n5-fbb n5-bob" style="animation-delay:-' + dl + 's;animation-duration:6s"', n5p('M' + (x - 26 * k) + ',' + y + ' q' + (26 * k) + ',' + (12 * k) + ' ' + (56 * k) + ',0 l' + (-6 * k) + ',' + (9 * k) + ' h' + (-42 * k) + ' Z', 'fill="' + land + '"') + n5p('M' + (x - 6 * k) + ',' + y + ' v' + (-52 * k) + ' M' + (x + 14 * k) + ',' + y + ' v' + (-42 * k) + ' M' + (x - 18 * k) + ',' + (y - 36 * k) + ' h' + (24 * k) + ' M' + (x - 14 * k) + ',' + (y - 22 * k) + ' h' + (18 * k) + ' M' + (x + 4 * k) + ',' + (y - 30 * k) + ' h' + (20 * k) + ' M' + (x - 6 * k) + ',' + (y - 52 * k) + ' L' + (x - 28 * k) + ',' + (y - 2) + ' M' + (x + 14 * k) + ',' + (y - 42 * k) + ' L' + (x + 32 * k) + ',' + (y - 2), 'stroke="' + land + '" stroke-width="' + f1(1.6 * k) + '" fill="none"') + n5c(x + 20 * k, y - 4 * k, 1.6, 'fill="#ffd27a"')); }
    s += ship(952, 250, 1, 0) + ship(900, 236, .5, 2.4);
    [[880, 110, 0], [960, 92, 3], [990, 140, 6]].forEach(function (q) { s += n5g('class="n5-flit" style="--fx:' + (q[2] % 2 ? -30 : 34) + 'px;animation-delay:-' + q[2] + 's"', n5p('M' + (q[0] - 8) + ',' + q[1] + ' q4,-6 8,0 q4,-6 8,0', 'stroke="' + (dark ? '#c8d0e0' : '#1a1e24') + '" stroke-width="1.6" fill="none" stroke-linecap="round"')); });
    s += cmRainIn(t, 800, 56, 224, 260, 7) + n5p('M824,300 L900,70 h34 L858,300 Z', 'fill="#fff" opacity=".06"');
    // glazing bars fanned in the arch, the sill, and a spyglass kept on it
    a = mix(t.th.wood, '#000', .45);
    fr = n5p('M912,66 V306 M812,164 H1012 M812,236 H1012 M912,164 L842,96 M912,164 L982,96', 'stroke="' + a + '" stroke-width="5" fill="none"') + cmFrame(t, d, 14);
    fr += n5r(796, 304, 232, 14, 'rx="2" fill="#000" filter="url(#cm-beam)"') + n5r(796, 304, 232, 2, 'fill="' + t.lc + '" opacity=".5"') + n5r(796, 318, 232, 12, 'fill="#000" opacity=".5" filter="url(#src-blur3)"');
    fr += n5p('M972,304 L986,268 L1002,304 M986,268 V304', 'stroke="#2a1e12" stroke-width="3" fill="none"') + n5g('transform="rotate(-24 986 266)"', n5r(940, 259, 60, 13, 'rx="2" fill="url(#cm-trimv)"') + n5r(996, 256, 30, 19, 'rx="2" fill="url(#cm-trimv)"') + n5r(936, 257, 7, 17, 'rx="1.6" fill="' + mix(t.th.trim, '#000', .4) + '"') + n5r(993, 256, 4, 19, 'fill="#000" opacity=".4"') + n5r(1022, 256, 4, 19, 'fill="#000" opacity=".4"') + n5r(940, 260, 86, 2, 'fill="#fff" opacity=".5"'));
    return mpOpen(d, s, fr);
  };
  DISPLAYS['scroll-rack'] = function (t) {
    var r = rng(95), s = cmUnitFrame(t, .6), a = mix(t.th.wood, '#000', .35), hi = mix(t.th.wood, '#fff', .2), x, y, i, j, k, L = '', H = '', S = '';
    // a lattice of diamonds, a scroll or three in each
    s += '<clipPath id="n5-rack"><rect x="40" y="86" width="442" height="362"/></clipPath>';
    for (i = -8; i < 10; i++) { L += 'M' + (40 + i * 62) + ',86 l362,362 M' + (40 + i * 62 + 362) + ',86 l-362,362 '; }
    for (j = 0; j < 13; j++) for (i = 0; i < 9; i++) {
      x = 40 + i * 62 + (j % 2 ? 0 : 31); y = 86 + j * 31; if (x < 44 || x > 478 || y < 96 || y > 440) continue;
      var n = 1 + Math.floor(r() * 3);
      for (k = 0; k < n; k++) { var sx = x + (k - (n - 1) / 2) * 13 + (r() - .5) * 3, sy = y + 8 - (k % 2) * 9 + (r() - .5) * 3, rr = 7 + r() * 2.6, c = mix('#e4d4a8', '#a8905c', r() * .6);
        S += n5c(sx + 1.5, sy + 2, rr, 'fill="#000" opacity=".5"') + n5c(sx, sy, rr, 'fill="' + c + '" stroke="#5a4424" stroke-width="1"') + n5p('M' + f1(sx - rr * .5) + ',' + f1(sy) + ' a' + f1(rr * .5) + ',' + f1(rr * .5) + ' 0 1 1 ' + f1(rr * .6) + ',' + f1(rr * .4) + ' a' + f1(rr * .25) + ',' + f1(rr * .25) + ' 0 1 1 ' + f1(-rr * .3) + ',' + f1(-rr * .3), 'stroke="#6a5230" stroke-width=".9" fill="none"') + (r() < .22 ? n5c(sx + rr * .5, sy + rr * .5, 2.6, 'fill="' + t.th.accent + '"') : ''); }
    }
    s += '<g clip-path="url(#n5-rack)">' + n5r(40, 86, 442, 362, 'fill="#050302" opacity=".5"') + S + n5p(L, 'stroke="#000" stroke-opacity=".5" stroke-width="9" fill="none" transform="translate(3,4)" filter="url(#src-blur3)"') + n5p(L, 'stroke="' + a + '" stroke-width="7" fill="none"') + n5p(L, 'stroke="' + hi + '" stroke-opacity=".45" stroke-width="1.2" fill="none" transform="translate(-2,-2)"') + n5r(40, 86, 442, 362, 'fill="#000" opacity=".22"') + '</g>';
    CM_ROWS.forEach(function (ry) { s += cmShelfBoard(t, ry, 40, 482); });
    return { art: '<g filter="url(#cm-unitlit)">' + s + '</g>', slots: cmGrid(), zone: [34, 96, 454, 320], wallsh: true, decor: true, label: 'The scroll rack' };
  };
  COUNTERS['plan-chest'] = function (t) {
    var s, r = rng(97), x, y, g = '', lea = mix(t.th.cloth, '#060806', .25), tr = t.th.trim, hi = mix(t.th.wood, '#ffffff', .22);
    // a top of tooled leather, edged in wood
    s = '<defs>' + mp3Defs() + '</defs>' + cmTop(t, 'M-300,452 H1500 V512 H-300 Z') + '<clipPath id="cm-topclip"><rect x="-300" y="452" width="1800" height="60"/></clipPath>';
    s += n5r(-300, 456, 1800, 40, 'fill="' + lea + '" filter="url(#mp-fine)"') + n5r(-300, 456, 1800, 40, 'fill="none" stroke="#000" stroke-opacity=".5" stroke-width="2"') + n5p('M-300,490 H1500', 'stroke="' + tr + '" stroke-width="1.6"') + n5p('M-300,485 H1500', 'stroke="' + tr + '" stroke-width="3" stroke-dasharray="1 7" stroke-linecap="round"') + n5p('M-300,461 H1500', 'stroke="' + tr + '" stroke-opacity=".6" stroke-width="1"');
    s += n5p('M-300,506 H1500 V520 H-300 Z', 'fill="#000" filter="url(#cm-beam)"') + n5r(-300, 506, 1800, 2.4, 'fill="' + t.lc + '" opacity=".45"') + n5r(-300, 520, 1800, 380, 'fill="#000" filter="url(#cm-front)"');
    // wide shallow drawers, each with a label and two brass cups
    for (y = 530; y < 900; y += 48) for (x = -300 + 14; x < 1500; x += 438) {
      g += n5r(x, y, 424, 40, 'fill="none" stroke="#040201" stroke-opacity=".85" stroke-width="2.6"') + n5r(x + 2.6, y + 2.6, 424, 40, 'fill="none" stroke="' + hi + '" stroke-opacity=".26" stroke-width="1.2"') + n5r(x + 6, y + 6, 412, 28, 'fill="' + (r() < .5 ? '#000' : '#fff') + '" opacity="' + f1(r() * .08) + '"');
      g += n5r(x + 186, y + 10, 52, 20, 'rx="2" fill="url(#cm-trim)"') + n5r(x + 190, y + 13, 44, 14, 'fill="#e4d8b4"') + n5p('M' + (x + 194) + ',' + (y + 18) + ' h' + f1(16 + r() * 20) + ' M' + (x + 194) + ',' + (y + 23) + ' h' + f1(10 + r() * 20), 'stroke="#3a2a14" stroke-width="1.2"');
      [70, 354].forEach(function (o) { g += n5p('M' + (x + o - 16) + ',' + (y + 14) + ' q16,16 32,0', 'stroke="' + tr + '" stroke-width="5" fill="none" stroke-linecap="round"') + n5p('M' + (x + o - 16) + ',' + (y + 14) + ' q16,16 32,0', 'stroke="#fff" stroke-opacity=".4" stroke-width="1.2" fill="none"') + n5e(x + o, y + 30, 18, 4, 'fill="#000" opacity=".35" filter="url(#src-blur3)"'); });
    }
    return '<g filter="url(#cm-woodlit)">' + s + g + n5r(-300, 520, 1800, 30, 'fill="#000" opacity=".45" filter="url(#src-blur8)"') + '</g>';
  };
  PROPS.globe = function (t, p) {
    var x = p.x, y = 338, R = 56, r = rng(99), s, land = '', i, lc = '#b89a5a', w = mix(t.th.wood, '#000', .25);
    s = '<defs>' + rad('n5-gsea', [[0, '#5a94a4'], [.6, '#285a6a'], [1, '#0c2430']], ' cx=".36" cy=".3" r=".8"') + rad('n5-gsh', [[0, '#fff', .34], [.4, '#fff', 0], [.75, '#000', .12], [1, '#000', .62]], ' cx=".34" cy=".28" r=".8"') + '<clipPath id="n5-gclip"><circle cx="' + x + '" cy="' + y + '" r="' + R + '"/></clipPath></defs>';
    s += n5e(x + 4, 482, 52, 8, 'fill="url(#src-ao)"');
    // the stand: three turned legs to a ring, the globe hung in a brass meridian
    s += n5p('M' + (x - 44) + ',480 L' + (x - 16) + ',412 M' + (x + 44) + ',480 L' + (x + 16) + ',412 M' + x + ',484 V412', 'stroke="' + w + '" stroke-width="8" stroke-linecap="round" fill="none"') + n5p('M' + (x - 44) + ',480 L' + (x - 16) + ',412 M' + x + ',484 V412', 'stroke="' + t.lc + '" stroke-opacity=".25" stroke-width="2" fill="none" transform="translate(-2,0)"');
    s += n5p('M' + (x - 72) + ',' + (y + 4) + ' A72,13 0 0 1 ' + (x + 72) + ',' + (y + 4), 'stroke="' + mix(w, '#000', .3) + '" stroke-width="9" fill="none"');
    s += n5c(x, y, R, 'fill="url(#n5-gsea)"');
    for (i = 0; i < 9; i++) land += n5p(mpLump(r, x - R + r() * R * 4, y + (r() - .5) * R * 1.5, 12 + r() * 22, 9), 'fill="' + mix(lc, '#5a7a3a', r() * .5) + '" stroke="#4a3a1a" stroke-width=".8"');
    land += land.replace(/translate\([^)]*\)/g, '');
    s += '<g clip-path="url(#n5-gclip)">' + n5g('class="n5-slide" style="--sx:-' + (R * 4) + 'px;animation-duration:36s"', land + n5g('transform="translate(' + (R * 4) + ',0)"', land)) +
      [-.6, -.3, 0, .3, .6].map(function (k) { return n5e(x, y + k * R, R * Math.sqrt(1 - k * k), 5 * (1 - Math.abs(k)) + 1.4, 'fill="none" stroke="#e8dcb8" stroke-opacity=".3" stroke-width=".8"'); }).join('') + [.3, .62, .9].map(function (k) { return n5e(x, y, R * k, R, 'fill="none" stroke="#e8dcb8" stroke-opacity=".3" stroke-width=".8"'); }).join('') + n5c(x, y, R, 'fill="url(#n5-gsh)"') + '</g>';
    s += n5g('transform="rotate(20 ' + x + ' ' + y + ')"', n5c(x, y, R + 6, 'fill="none" stroke="url(#cm-trim)" stroke-width="5"') + n5c(x, y, R + 8, 'fill="none" stroke="#fff" stroke-opacity=".35" stroke-width="1"') + n5c(x, y - R - 6, 4.4, 'fill="url(#cm-trim)"') + n5c(x, y + R + 6, 4.4, 'fill="url(#cm-trim)"'));
    s += n5p('M' + (x - 72) + ',' + (y + 4) + ' A72,13 0 0 0 ' + (x + 72) + ',' + (y + 4), 'stroke="' + w + '" stroke-width="9" fill="none"') + n5p('M' + (x - 70) + ',' + (y + 1) + ' A72,13 0 0 0 ' + (x + 70) + ',' + (y + 1), 'stroke="' + t.lc + '" stroke-opacity=".35" stroke-width="1.6" fill="none"');
    return n5g('class="src-nohit"', s);
  };
  PROPS.charts = function (t, p) {
    var r = rng(101), s = n5sag(500, 50, 790, 52, 7, '#8a7250', 2) + n5sag(1040, 52, 1200, 50, 6, '#8a7250', 2), ink = '#4a2e14';
    [[516, 34, 60, 0], [706, 50, 78, 1], [760, 44, 104, 2], [1068, 56, 96, 3], [1122, 46, 70, 4], [1168, 54, 118, 5]].forEach(function (q) {
      var x = q[0], w = q[1], h = q[2], y = 58, k = q[3], g;
      g = n5r(x - w / 2 + 3, y + 3, w, h, 'fill="#000" opacity=".4" filter="url(#src-blur3)"') + n5r(x - w / 2, y, w, h, 'fill="' + mix('#e4d6ae', '#c0a674', r() * .6) + '"') + n5r(x - w / 2, y, w, h, 'fill="none" stroke="#7a6238" stroke-width="1"');
      g += n5p(mpLump(r, x - 3, y + h * .42, Math.min(w, h) * .26, 9), 'fill="#b08a4e" opacity=".55" stroke="' + ink + '" stroke-width="1"') + n5p('M' + f1(x - w * .3) + ',' + f1(y + h * .8) + ' q' + f1(w * .2) + ',-10 ' + f1(w * .34) + ',-2 t' + f1(w * .22) + ',-8', 'stroke="' + t.th.accent + '" stroke-width="1.4" stroke-dasharray="3 3" fill="none"') + n5p('M' + f1(x + w * .24) + ',' + f1(y + h * .7) + ' l6,6 m0,-6 l-6,6', 'stroke="' + t.th.accent + '" stroke-width="1.6"');
      if (k % 2) g += n5p('M' + f1(x + w * .26) + ',' + (y + 8) + ' l3,9 l-3,-3 l-3,3 Z', 'fill="' + ink + '"');
      g += n5r(x - 3, y - 8, 6, 14, 'rx="1.4" fill="#a8885a"') + n5r(x - 3, y - 2, 6, 1.4, 'fill="#5a4424"');
      s += n5g('class="n5-vb n5-sway" style="transform-origin:' + x + 'px 52px;animation-duration:' + f1(6 + r() * 4) + 's;animation-delay:-' + f1(r() * 7) + 's"', g);
    });
    return n5g('class="src-nohit"', s);
  };
  PROPS.inkwell = function (t, p) {
    var x = p.x, y = 492, s = n5e(x, y + 4, 66, 9, 'fill="url(#src-ao)"');
    // dividers standing open, an inkwell and its quill, a glass, and a chart rolled and tied
    s += n5p('M' + (x - 50) + ',' + (y + 2) + ' L' + (x - 36) + ',' + (y - 62) + ' L' + (x - 20) + ',' + (y + 2), 'stroke="url(#cm-trim)" stroke-width="3.4" fill="none" stroke-linejoin="round"') + n5c(x - 36, y - 62, 5, 'fill="url(#cm-trim)"') + n5p('M' + (x - 44) + ',' + (y - 24) + ' q8,-6 17,0', 'stroke="url(#cm-trim)" stroke-width="2" fill="none"');
    s += n5p('M' + (x - 8) + ',' + y + ' v-16 q0,-6 6,-6 h12 q6,0 6,6 v16 Z', 'fill="#1c2a34"') + n5r(x - 4, y - 28, 20, 7, 'rx="2" fill="url(#cm-trim)"') + n5r(x - 6, y - 14, 4, 12, 'fill="#fff" opacity=".25"') + n5e(x + 6, y - 28, 8, 2, 'fill="#05080c"');
    s += n5p('M' + (x + 6) + ',' + (y - 28) + ' L' + (x + 36) + ',' + (y - 96), 'stroke="#e8e0cc" stroke-width="1.6"') + n5p('M' + (x + 16) + ',' + (y - 50) + ' C' + (x + 6) + ',' + (y - 70) + ' ' + (x + 20) + ',' + (y - 92) + ' ' + (x + 38) + ',' + (y - 100) + ' C' + (x + 44) + ',' + (y - 80) + ' ' + (x + 34) + ',' + (y - 62) + ' ' + (x + 16) + ',' + (y - 50) + ' Z', 'fill="#efe8d6"') + n5p('M' + (x + 18) + ',' + (y - 62) + ' l10,-4 M' + (x + 22) + ',' + (y - 74) + ' l10,-4 M' + (x + 27) + ',' + (y - 86) + ' l8,-3', 'stroke="#a89c80" stroke-width="1"');
    s += n5c(x + 50, y - 8, 15, 'fill="#bfe0ea" opacity=".3"') + n5c(x + 50, y - 8, 15, 'fill="none" stroke="url(#cm-trim)" stroke-width="3.4"') + n5p('M' + (x + 60) + ',' + (y + 3) + ' l12,10', 'stroke="#2a1c10" stroke-width="5" stroke-linecap="round"') + n5p('M' + (x + 40) + ',' + (y - 12) + ' q4,-8 12,-9', 'stroke="#fff" stroke-opacity=".7" stroke-width="1.6" fill="none"') + n5c(x + 44, y - 14, 3, 'class="n5-fb n5-glint" fill="#fff"');
    return n5g('class="src-nohit"', s);
  };


  /* ---- The stable and beast seller: barn boards with daylight in the
     gaps, a hay loft overhead, a half door with a horse looking in, hutches
     for shelves and a feed stall for a counter. ---- */
  CM_OPT.shape.push('barn'); CM_OPT.wall.push('barnboard'); CM_OPT.opening.push('stable-door'); CM_OPT.display.push('hutches'); CM_OPT.counter.push('feed-stall');
  CM_WALLFQ.barnboard = [.03, 2.6, 15]; CM_OPEN_INFO['stable-door'] = 'The stable door';
  n5prop('birdcages', 'ceil', 'back'); n5prop('perch', 'tallR', 'front', 0, true); n5prop('feathers', 'over', 'over'); n5prop('haybale', 'cL', 'front', 170); n5prop('critters', 'air', 'back');

  function n5straw(r, x0, x1, y, n, len, up) {
    var s = '', i, x, a;
    for (i = 0; i < n; i++) { x = x0 + r() * (x1 - x0); a = (r() - .5) * 1.1; s += n5p('M' + f1(x) + ',' + f1(y + (r() - .5) * 4) + ' l' + f1(Math.sin(a) * len * (.5 + r())) + ',' + f1((up ? -1 : 1) * Math.cos(a) * len * (.4 + r())), 'stroke="' + mix('#e8c860', r() < .5 ? '#8a6a24' : '#fff0b0', r() * .6) + '" stroke-width="' + f1(1 + r() * 1.2) + '" stroke-linecap="round" fill="none"'); }
    return s;
  }
  WALLS.barnboard = function (t) {
    var r = rng(111), s = '<rect ' + FULL + ' fill="#000" filter="url(#cm-board)"/>' + n5boards(r, -300, -150, 1800, 620, 62, true), x, i, y;
    // day through the gaps between the boards
    for (x = -300 + 61; x < 1500; x += 62) if (r() < .5) { y = -100 + r() * 400; s += n5r(x - 1, y, 2.4, 60 + r() * 160, 'fill="#fff0c0" opacity=".85"') + n5r(x - 5, y, 10, 60 + r() * 160, 'fill="#ffe8a0" opacity=".3" filter="url(#src-blur3)"'); }
    for (i = 0; i < 22; i++) { x = -280 + r() * 1760; y = -120 + r() * 560; s += n5e(x, y, 5 + r() * 5, 8 + r() * 8, 'fill="#0a0604" opacity=".7"') + n5e(x, y, 9 + r() * 5, 14 + r() * 8, 'fill="none" stroke="#0a0604" stroke-opacity=".4" stroke-width="1.6"'); }
    // rails nailed across, a long brace, and old shoes hung for luck
    [118, 330].forEach(function (ry) { s += n5r(-300, ry + 20, 1800, 9, 'fill="#000" opacity=".4" filter="url(#src-blur3)"') + n5r(-300, ry, 1800, 20, 'fill="#000" filter="url(#cm-beam)"') + n5r(-300, ry, 1800, 1.6, 'fill="' + t.lc + '" opacity=".25"'); });
    s += n5p('M500,330 L790,138 L790,160 L524,338 Z', 'fill="#000" filter="url(#cm-post)"') + n5p('M1030,138 L1320,330 L1296,338 L1030,160 Z', 'fill="#000" filter="url(#cm-post)"');
    [[524, 190], [524, 246], [760, 402]].forEach(function (q) { s += n5p('M' + (q[0] - 11) + ',' + (q[1] - 12) + ' a12,13 0 1 0 22,0', 'stroke="#000" stroke-opacity=".5" stroke-width="6" fill="none" transform="translate(-3,3)"') + n5p('M' + (q[0] - 11) + ',' + (q[1] - 12) + ' a12,13 0 1 0 22,0', 'stroke="#4a4c52" stroke-width="5.4" fill="none" stroke-linecap="round"') + n5p('M' + (q[0] - 11) + ',' + (q[1] - 12) + ' a12,13 0 1 0 22,0', 'stroke="#a8acb4" stroke-opacity=".5" stroke-width="1.2" fill="none" transform="translate(-1,-1)"'); });
    return s;
  };
  SHAPES.barn = function (t) {
    var s = WALLS[t.th.wall](t), r = rng(113), x;
    // the loft floor, hay hanging over its edge
    s += n5r(-300, -150, 1800, 160, 'fill="#0c0806"') + n5straw(r, -300, 1500, 2, 260, 34, true) + n5r(-300, 8, 1800, 30, 'fill="#000" filter="url(#cm-beam)"') + n5r(-300, 8, 1800, 2, 'fill="' + t.lc + '" opacity=".3"') + n5r(-300, 38, 1800, 14, 'fill="#000" opacity=".5" filter="url(#src-blur3)"');
    for (x = -280; x < 1500; x += 170) s += n5r(x, 2, 26, 40, 'rx="2" fill="#000" filter="url(#cm-post)"') + n5r(x + 26, 6, 8, 40, 'fill="#000" opacity=".4" filter="url(#src-blur3)"');
    s += n5straw(r, -300, 1500, 14, 420, 42) + n5straw(r, -300, 1500, 8, 160, 70);
    // a hoist rope and its hook
    s += n5p('M772,8 V96', 'stroke="#a88a5a" stroke-width="3.4"') + n5p('M772,8 V96', 'stroke="#5a4424" stroke-width="3.4" stroke-dasharray="2 4"') + n5p('M772,96 v10 a9,9 0 1 1 -9,9', 'stroke="#3a3c42" stroke-width="4" fill="none" stroke-linecap="round"');
    return s + cmPost(-6, 34, 30, 474) + cmPost(1172, 34, 30, 474) + n5p('M28,150 L110,38 H134 L28,184 Z', 'fill="#000" filter="url(#cm-post)"') + n5p('M1172,150 L1090,38 H1066 L1172,184 Z', 'fill="#000" filter="url(#cm-post)"');
  };
  OPENINGS['stable-door'] = function (t) {
    var d = 'M810,272 V80 H1014 V272 Z', r = rng(115), dark = t.time === 'night', s, fr, w = t.th.wood, hc = '#7a4a2a', hd = '#3e2412', i, x;
    s = cmSky(t, 'n5-ssky', 806, 76, 212, 150) + n5p(n5ridge(r, 806, 1018, 196, 14, 30, 280), 'fill="' + (dark ? '#0a1410' : '#5a7a44') + '"') + n5r(806, 216, 212, 60, 'fill="' + (dark ? '#060c08' : '#44642e') + '"');
    // a rail fence in the paddock
    s += n5p('M806,214 H1018 M806,226 H1018', 'stroke="' + (dark ? '#1a1814' : '#d8ccb0') + '" stroke-width="3"');
    for (x = 820; x < 1018; x += 44) s += n5r(x, 206, 4, 30, 'fill="' + (dark ? '#1a1814' : '#c8bc9c') + '"');
    s += cmRainIn(t, 806, 76, 212, 200, 3);
    // the horse: neck, head, mane, an ear that flicks
    var head = n5p('M962,274 C952,232 946,198 930,170 L968,132 C1000,160 1018,212 1020,274 Z', 'fill="' + hc + '"') + n5p('M970,274 C962,236 958,204 944,178 L966,156 C990,184 1002,226 1004,274 Z', 'fill="#000" opacity=".16"') +
      n5p('M970,130 C952,122 938,128 926,142 L884,196 C875,207 878,222 891,224 C905,226 913,218 919,209 L946,180 C954,172 960,160 970,150 Z', 'fill="' + hc + '"') + n5p('M884,196 C875,207 878,222 891,224 C905,226 913,218 919,209 L908,196 Z', 'fill="' + hd + '" opacity=".6"') +
      n5p('M944,138 L898,198 L906,202 L950,146 Z', 'fill="#f0e8d8" opacity=".9"') + n5e(887, 212, 3.4, 2.2, 'fill="#1a0c06"') + n5p('M888,220 q8,2 14,-3', 'stroke="#1a0c06" stroke-width="1.4" fill="none"') +
      n5c(936, 156, 5, 'fill="#120a06"') + n5c(934.5, 154.5, 1.6, 'fill="#fff" opacity=".9"') + n5p('M929,150 q7,-5 13,1', 'stroke="' + hd + '" stroke-width="2" fill="none"') +
      n5g('class="n5-fbb n5-ear"', n5p('M952,132 L955,102 L969,128 Z', 'fill="' + hc + '"') + n5p('M955,128 L957,110 L964,126 Z', 'fill="' + hd + '"')) +
      n5p('M962,124 C990,144 1010,190 1020,250 L1030,274 H1010 C1004,220 990,170 956,136 Z', 'fill="' + hd + '"') + n5p('M966,128 q-18,10 -26,30 M972,134 q-10,8 -16,22', 'stroke="' + hd + '" stroke-width="5" fill="none" stroke-linecap="round"') +
      n5p('M904,186 L926,204 M926,204 L962,140 M910,180 q10,2 16,10', 'stroke="#2a1a10" stroke-width="3" fill="none"') + n5c(926, 204, 3.4, 'fill="url(#cm-trim)"');
    s += n5g('class="n5-vb n5-nod" style="transform-origin:1000px 274px"', head);
    // the lower leaf, shut: boards, a brace, strap hinges
    fr = n5r(810, 272, 204, 180, 'fill="#000" filter="url(#cm-board)"');
    for (i = 0; i < 6; i++) fr += n5r(810 + i * 34 + 32.6, 272, 1.6, 180, 'fill="#070402" opacity=".8"');
    fr += n5p('M810,286 H1014 V304 H810 Z M810,420 H1014 V438 H810 Z M810,304 L1014,404 V420 L810,320 Z', 'fill="#000" filter="url(#cm-beam)"') + n5p('M810,286 H1014 M810,420 H1014', 'stroke="' + t.lc + '" stroke-opacity=".3" stroke-width="1.4"');
    [295, 429].forEach(function (y) { fr += n5p('M806,' + (y - 5) + ' h70 l8,5 l-8,5 h-70 Z', 'fill="#1c1d20"') + n5c(820, y, 2, 'fill="#8a8e96"') + n5c(850, y, 2, 'fill="#8a8e96"'); });
    fr += n5r(804, 264, 216, 12, 'rx="2" fill="#000" filter="url(#cm-beam)"') + n5r(804, 264, 216, 2, 'fill="' + t.lc + '" opacity=".5"') + n5straw(r, 820, 1000, 264, 16, 10);
    // the upper leaf swung back against the wall, the frame and the lintel
    fr += n5p('M1014,80 L1052,70 V280 L1014,272 Z', 'fill="' + mix(w, '#000', .45) + '"') + n5p('M1026,78 V274 M1040,74 V277', 'stroke="#070402" stroke-opacity=".8" stroke-width="1.4"') + n5p('M1014,80 L1052,70 V280 L1014,272 Z', 'fill="none" stroke="' + mix(w, '#fff', .2) + '" stroke-opacity=".4" stroke-width="1.2"');
    fr += n5r(796, 66, 16, 386, 'fill="#000" filter="url(#cm-post)"') + n5r(1012, 66, 16, 386, 'fill="#000" filter="url(#cm-post)"') + n5r(788, 60, 248, 20, 'fill="#000" filter="url(#cm-beam)"') + n5r(788, 60, 248, 2, 'fill="' + t.lc + '" opacity=".4"') + n5r(788, 80, 248, 10, 'fill="#000" opacity=".4" filter="url(#src-blur3)"');
    return mpOpen(d, s, fr, dark ? '' : n5e(912, 200, 240, 200, 'fill="#fff8e0" opacity=".05" filter="url(#src-blur14)" pointer-events="none"'));
  };
  DISPLAYS.hutches = function (t) {
    var r = rng(117), s = '<defs><pattern id="n5-mesh" width="18" height="16" patternUnits="userSpaceOnUse"><path d="M4.5,0 L0,4 V12 L4.5,16 M4.5,0 H13.5 L18,4 V12 L13.5,16 H4.5" stroke="#b0b0a8" stroke-opacity=".5" stroke-width=".9" fill="none"/></pattern></defs>' + cmUnitFrame(t, .7), i;
    // three long hutches: wire at the back, straw on the floor, a post between each pen
    CM_ROWS.forEach(function (ry) {
      s += n5r(40, ry - 104, 442, 104, 'fill="#060403" opacity=".7"') + n5r(40, ry - 104, 442, 104, 'fill="url(#n5-mesh)"') + n5r(40, ry - 104, 442, 22, 'fill="#000" opacity=".5" filter="url(#src-blur5)"');
      s += n5straw(r, 44, 478, ry - 2, 150, 14, true) + cmShelfBoard(t, ry, 40, 482);
      for (i = 1; i < 4; i++) s += n5r(CM_COLS[i] - 59, ry - 104, 8, 104, 'fill="#000" filter="url(#cm-post)"') + n5r(CM_COLS[i] - 51, ry - 104, 5, 104, 'fill="#000" opacity=".4" filter="url(#src-blur3)"');
      s += n5straw(r, 44, 478, ry + 2, 40, 10);
    });
    return { art: '<g filter="url(#cm-unitlit)">' + s + '</g>', slots: cmGrid(), zone: [34, 96, 454, 320], wallsh: false, decor: true, label: 'The hutches' };
  };
  COUNTERS['feed-stall'] = function (t) {
    var s = cmTop(t, 'M-300,452 H1500 V512 H-300 Z'), r = rng(119), g = '', y, x;
    s += '<clipPath id="cm-topclip"><rect x="-300" y="452" width="1800" height="60"/></clipPath>' + n5p('M-300,506 H1500 V518 H-300 Z', 'fill="#000" filter="url(#cm-beam)"') + n5r(-300, 506, 1800, 2.4, 'fill="' + t.lc + '" opacity=".45"');
    // rough boards with a dark gap between each, and stout posts
    s += n5r(-300, 518, 1800, 382, 'fill="#070504"');
    for (y = 524; y < 900; y += 62) s += n5r(-300, y, 1800, 54, 'fill="#000" filter="url(#cm-front)"') + n5r(-300, y, 1800, 54, 'fill="' + (r() < .5 ? '#000' : '#fff') + '" opacity="' + f1(r() * .09) + '"') + n5r(-300, y, 1800, 1.6, 'fill="' + t.lc + '" opacity=".2"');
    for (x = -220; x < 1500; x += 330) { g += n5r(x, 518, 30, 382, 'fill="#000" filter="url(#cm-post)"') + n5r(x + 30, 518, 10, 382, 'fill="#000" opacity=".4" filter="url(#src-blur3)"'); for (y = 550; y < 900; y += 62) g += n5c(x + 15, y, 3, 'fill="#1a1a1e"') + n5c(x + 14, y - 1, 1, 'fill="#9a9ea6"'); }
    // feed sacks leant on the front, and straw trodden along the foot
    function sack(x, y, w, h, word, a) {
      var c = mix('#b89a68', '#7a6238', r() * .4), d = 'M' + (x - w / 2) + ',' + (y + h) + ' Q' + (x - w / 2 - 8) + ',' + (y + h * .4) + ' ' + (x - w / 2 + 6) + ',' + (y + 10) + ' L' + (x - w / 2 - 6) + ',' + (y - 6) + ' L' + (x - w / 2 + 16) + ',' + (y + 4) + ' Q' + x + ',' + (y - 6) + ' ' + (x + w / 2 - 16) + ',' + (y + 4) + ' L' + (x + w / 2 + 6) + ',' + (y - 6) + ' L' + (x + w / 2 - 6) + ',' + (y + 10) + ' Q' + (x + w / 2 + 8) + ',' + (y + h * .4) + ' ' + (x + w / 2) + ',' + (y + h) + ' Z', o, k;
      o = n5sh(d, 8, 4, .5, 8) + n5p(d, 'fill="' + c + '"');
      for (k = 1; k < 9; k++) o += n5p('M' + (x - w / 2 + 2) + ',' + f1(y + k * h / 9) + ' q' + (w / 2) + ',5 ' + (w - 4) + ',0', 'stroke="#3a2a14" stroke-opacity=".22" stroke-width="1" fill="none"');
      for (k = 1; k < 7; k++) o += n5p('M' + f1(x - w / 2 + k * w / 7) + ',' + (y + 6) + ' V' + (y + h), 'stroke="#3a2a14" stroke-opacity=".14" stroke-width="1"');
      o += n5p('M' + (x - w / 2 + 4) + ',' + (y + 12) + ' Q' + (x - w / 2 - 2) + ',' + (y + h * .5) + ' ' + (x - w / 2 + 4) + ',' + (y + h), 'stroke="#fff" stroke-opacity=".2" stroke-width="5" fill="none"') + '<text x="' + x + '" y="' + f1(y + h * .5) + '" text-anchor="middle" font-family="Georgia,serif" font-weight="700" font-size="' + f1(w * .2) + '" fill="#3a2414" opacity=".75" letter-spacing="2">' + word + '</text>' + n5p('M' + (x - w * .22) + ',' + f1(y + h * .58) + ' h' + f1(w * .44), 'stroke="#3a2414" stroke-opacity=".6" stroke-width="2"');
      return n5g('transform="rotate(' + a + ' ' + x + ' ' + (y + h) + ')"', o);
    }
    g += sack(150, 560, 130, 180, 'OATS', -5) + sack(262, 590, 110, 150, 'BRAN', 4) + sack(1010, 572, 130, 170, 'FEED', 3);
    g += n5straw(r, -300, 1500, 696, 380, 30, true) + n5straw(r, -300, 1500, 704, 200, 16);
    return '<g filter="url(#cm-woodlit)">' + s + g + n5r(-300, 518, 1800, 30, 'fill="#000" opacity=".5" filter="url(#src-blur8)"') + '</g>';
  };
  PROPS.birdcages = function (t, p) {
    var s = '', r = rng(121), tr = t.th.trim;
    [[708, 50, '#e8c040'], [1072, 70, '#d8503a'], [1150, 40, '#58a8d8']].forEach(function (q, j) {
      var x = q[0], y = 44 + q[1], g, k;
      g = n5p('M' + x + ',40 V' + (y - 26), 'stroke="' + tr + '" stroke-width="1.6" stroke-dasharray="4 2"') + n5c(x, y - 24, 3.4, 'fill="none" stroke="' + tr + '" stroke-width="1.6"');
      g += n5p('M' + (x - 22) + ',' + (y + 44) + ' V' + y + ' Q' + (x - 22) + ',' + (y - 22) + ' ' + x + ',' + (y - 22) + ' Q' + (x + 22) + ',' + (y - 22) + ' ' + (x + 22) + ',' + y + ' V' + (y + 44) + ' Z', 'fill="#000" opacity=".25"');
      // the bird hops on its perch
      g += n5p('M' + (x - 20) + ',' + (y + 24) + ' H' + (x + 20), 'stroke="#6a4a2a" stroke-width="2.4"') + n5g('class="n5-fbb n5-bob" style="animation-duration:' + f1(1.6 + r()) + 's"', n5e(x - 2, y + 15, 9, 7.4, 'fill="' + q[2] + '"') + n5c(x + 5, y + 8, 5, 'fill="' + q[2] + '"') + n5p('M' + (x + 9) + ',' + (y + 7) + ' l6,2 l-6,2 Z', 'fill="#e8a030"') + n5c(x + 6, y + 7, 1.1, 'fill="#120a06"') + n5p('M' + (x - 10) + ',' + (y + 15) + ' l-9,6 l3,-8 Z', 'fill="' + mix(q[2], '#000', .3) + '"') + n5p('M' + (x - 6) + ',' + (y + 13) + ' q5,6 10,2', 'stroke="' + mix(q[2], '#000', .35) + '" stroke-width="1.6" fill="none"'));
      for (k = -3; k <= 3; k++) g += n5p('M' + f1(x + k * 7.3) + ',' + (y + 44) + ' V' + (y + 2) + ' Q' + f1(x + k * 7.3) + ',' + f1(y - 22 + Math.abs(k) * 2.4) + ' ' + x + ',' + (y - 22), 'stroke="' + tr + '" stroke-width="1.4" fill="none"');
      g += n5r(x - 24, y + 42, 48, 6, 'rx="2" fill="' + tr + '"') + n5r(x - 24, y + 42, 48, 1.4, 'fill="#fff" opacity=".5"') + n5p('M' + (x - 22) + ',' + y + ' H' + (x + 22), 'stroke="' + tr + '" stroke-width="1.6"');
      s += n5g('class="n5-vb n5-sway" style="transform-origin:' + x + 'px 40px;animation-duration:' + f1(6 + j * 1.3) + 's;animation-delay:-' + f1(j * 2.1) + 's"', g);
    });
    return n5g('class="src-nohit"', s);
  };
  PROPS.perch = function (t, p) {
    var x = p.x, s, w = mix(t.th.wood, '#000', .3), b = '#7e5e3a', bl = '#dccaa0', i;
    s = n5e(x + 4, 482, 40, 7, 'fill="url(#src-ao)"') + n5e(x, 478, 26, 6, 'fill="' + w + '"') + n5r(x - 4, 334, 8, 144, 'fill="' + w + '"') + n5r(x - 4, 334, 2.6, 144, 'fill="#fff" opacity=".18"') + n5r(x - 40, 328, 80, 8, 'rx="4" fill="' + mix(w, '#fff', .1) + '"') + n5r(x - 40, 328, 80, 2, 'fill="#fff" opacity=".25"');
    // an owl: barred wings, a pale breast, a head that turns and eyes that close
    s += n5p('M' + (x - 8) + ',330 l-4,26 l8,-6 l4,10 l4,-10 l8,6 l-4,-26 Z', 'fill="' + mix(b, '#000', .35) + '"');
    s += n5p('M' + (x - 30) + ',292 Q' + (x - 34) + ',250 ' + x + ',246 Q' + (x + 34) + ',250 ' + (x + 30) + ',292 Q' + (x + 28) + ',330 ' + x + ',334 Q' + (x - 28) + ',330 ' + (x - 30) + ',292 Z', 'fill="' + b + '"') + n5p('M' + (x - 16) + ',276 Q' + x + ',262 ' + (x + 16) + ',276 Q' + (x + 18) + ',322 ' + x + ',330 Q' + (x - 18) + ',322 ' + (x - 16) + ',276 Z', 'fill="' + bl + '"');
    for (i = 0; i < 4; i++) s += n5p('M' + (x - 11) + ',' + (286 + i * 11) + ' l4,5 l4,-5 l3,5 l4,-5 l3,5 l4,-5', 'stroke="' + mix(b, '#000', .2) + '" stroke-width="1.4" fill="none"');
    s += n5p('M' + (x - 30) + ',290 q4,24 12,36 M' + (x + 30) + ',290 q-4,24 -12,36', 'stroke="' + mix(b, '#000', .4) + '" stroke-width="3" fill="none"') + n5p('M' + (x - 8) + ',332 v6 m4,-6 v6 M' + (x + 4) + ',332 v6 m4,-6 v6', 'stroke="#d8a840" stroke-width="2" stroke-linecap="round"');
    var hd = n5p('M' + (x - 28) + ',252 Q' + (x - 30) + ',222 ' + (x - 24) + ',206 L' + (x - 14) + ',220 Q' + x + ',212 ' + (x + 14) + ',220 L' + (x + 24) + ',206 Q' + (x + 30) + ',222 ' + (x + 28) + ',252 Q' + x + ',276 ' + (x - 28) + ',252 Z', 'fill="' + b + '"') + n5c(x - 12, 240, 12, 'fill="' + bl + '"') + n5c(x + 12, 240, 12, 'fill="' + bl + '"') +
      n5g('class="n5-fb n5-blink"', n5c(x - 12, 240, 7.4, 'fill="#e8a820"') + n5c(x + 12, 240, 7.4, 'fill="#e8a820"') + n5c(x - 12, 240, 4, 'fill="#0a0604"') + n5c(x + 12, 240, 4, 'fill="#0a0604"') + n5c(x - 13.5, 238.5, 1.4, 'fill="#fff"') + n5c(x + 10.5, 238.5, 1.4, 'fill="#fff"')) + n5p('M' + x + ',244 l-4,8 l4,5 l4,-5 Z', 'fill="#3a2a1a"') + n5p('M' + (x - 22) + ',226 q10,-6 20,2 M' + (x + 22) + ',226 q-10,-6 -20,2', 'stroke="' + mix(b, '#000', .4) + '" stroke-width="2.4" fill="none"');
    s += n5g('class="n5-vb n5-tilt" style="transform-origin:' + x + 'px 262px"', hd);
    return n5g('class="src-nohit"', s);
  };
  PROPS.feathers = function (t, p) {
    var s = '', r = rng(123), i, x, y;
    for (i = 0; i < 9; i++) { x = 80 + r() * 1040; y = 20 + r() * 80; s += n5g('class="n5-snow" style="--fx:' + f1((r() - .5) * 200) + 'px;--fy:' + f1(380 + r() * 160) + 'px;animation-duration:' + f1(13 + r() * 9) + 's;animation-delay:-' + f1(r() * 22) + 's"', n5g('class="n5-fb n5-sway" style="animation-duration:' + f1(2.4 + r() * 2) + 's"', n5p('M' + f1(x) + ',' + f1(y) + ' q10,-10 22,-8 q-4,12 -18,12 Z', 'fill="' + (r() < .5 ? '#f4efe4' : '#c8b08a') + '" opacity=".9"') + n5p('M' + f1(x - 3) + ',' + f1(y + 3) + ' l22,-10', 'stroke="#8a7a60" stroke-width=".8"'))); }
    return n5g('class="src-nohit"', s);
  };
  PROPS.haybale = function (t, p) {
    var x = p.x, y = 494, r = rng(125), s, c = '#2c2a2a';
    s = n5e(x, y + 3, 84, 10, 'fill="url(#src-ao)"') + n5r(x - 74, y - 66, 148, 68, 'rx="6" fill="#b8963a"') + n5r(x - 74, y - 66, 148, 12, 'rx="6" fill="#e0c060" opacity=".7"') + n5r(x - 74, y - 20, 148, 22, 'rx="6" fill="#000" opacity=".25"');
    s += n5straw(r, x - 72, x + 72, y - 34, 200, 26) + n5straw(r, x - 72, x + 72, y - 62, 60, 12, true) + n5straw(r, x - 78, x + 78, y, 30, 10, true);
    [-34, 34].forEach(function (o) { s += n5r(x + o - 2, y - 66, 4, 68, 'fill="#5a4424"') + n5r(x + o - 2, y - 66, 1.4, 68, 'fill="#e8d8a8" opacity=".5"'); });
    // the yard cat, asleep, with a tail that will not keep still
    s += n5g('class="n5-vb n5-sway" style="transform-origin:' + (x + 34) + 'px ' + (y - 70) + 'px;animation-duration:3.6s"', n5p('M' + (x + 34) + ',' + (y - 72) + ' q22,2 24,24 q0,14 -8,18', 'stroke="' + c + '" stroke-width="7" fill="none" stroke-linecap="round"'));
    s += n5g('class="n5-fbb n5-breath"', n5p('M' + (x - 34) + ',' + (y - 66) + ' q-2,-28 30,-30 q36,-2 42,30 Z', 'fill="' + c + '"') + n5p('M' + (x - 26) + ',' + (y - 74) + ' q6,-16 28,-18', 'stroke="#fff" stroke-opacity=".14" stroke-width="3" fill="none"'));
    s += n5c(x - 28, y - 76, 13, 'fill="' + c + '"') + n5p('M' + (x - 40) + ',' + (y - 82) + ' l1,-13 l10,7 Z M' + (x - 26) + ',' + (y - 88) + ' l8,-9 l4,12 Z', 'fill="' + c + '"') + n5p('M' + (x - 36) + ',' + (y - 76) + ' q3,3 6,0 M' + (x - 27) + ',' + (y - 76) + ' q3,3 6,0', 'stroke="#8a8a84" stroke-width="1.2" fill="none"') + n5p('M' + (x - 31) + ',' + (y - 71) + ' l-1.6,2 h3.2 Z', 'fill="#c88a8a"');
    return n5g('class="src-nohit"', s);
  };
  PROPS.critters = function (t, p) {
    var s = '';
    [[151, 132, 0], [371, 250, 2.2], [261, 366, 4.1], [40 + 400, 140, 5.6]].forEach(function (q) { s += n5g('class="n5-fb n5-blink" style="animation-delay:-' + q[2] + 's;animation-duration:' + (4 + q[2] * .6) + 's"', n5e(t.mx(q[0]) - 6, q[1], 3, 3.6, 'fill="#ffe890"') + n5e(t.mx(q[0]) + 6, q[1], 3, 3.6, 'fill="#ffe890"') + n5e(t.mx(q[0]) - 6, q[1], 1, 3, 'fill="#1a1006"') + n5e(t.mx(q[0]) + 6, q[1], 1, 3, 'fill="#1a1006"')); });
    return n5g('class="src-nohit" opacity=".85"', s);
  };

  /* ---- The shrine: dressed stone, a rose window that throws its colours
     across the room, niches for shelves, an altar for a counter and a bank
     of votive lights. ---- */
  CM_OPT.shape.push('chapel'); CM_OPT.wall.push('ashlar'); CM_OPT.opening.push('rose-window'); CM_OPT.display.push('niches'); CM_OPT.counter.push('altar');
  CM_WALLFQ.ashlar = [.035, 2.4, 21]; CM_OPEN_INFO['rose-window'] = 'The rose window';
  n5light('votives', { base: '#ffc880', glow: '#ffb060', amb: '#3a3450', reach: 900, k: .95 });
  n5prop('censer', 'ceil', 'back'); n5prop('casket', 'cL', 'front', 130);

  WALLS.ashlar = function (t) {
    var r = rng(131), s = '<g filter="url(#cm-wall)"><rect ' + FULL + ' fill="#4a463e"/>' + n5stones(r, -300, -150, 1500, 470, 48, 92, 56, '#8e887c', '#5a554c', '#c4bcac', .34) + '</g><rect ' + FULL + ' fill="#1a1612" filter="url(#cm-stain)" opacity=".4"/>', x;
    // a string course with a row of little quatrefoils cut into it
    s += n5r(-300, 330, 1800, 22, 'fill="#9a9488" filter="url(#cm-wall)"') + n5r(-300, 330, 1800, 2, 'fill="#fff" opacity=".25"') + n5r(-300, 352, 1800, 9, 'fill="#000" opacity=".4" filter="url(#src-blur3)"');
    for (x = -280; x < 1500; x += 40) s += n5p('M' + x + ',336 a3.4,3.4 0 1 1 5,0 a3.4,3.4 0 1 1 0,5 a3.4,3.4 0 1 1 -5,0 a3.4,3.4 0 1 1 0,-5 Z', 'fill="#1a1612" opacity=".7"');
    return s;
  };
  function n5column(x, w, y0, y1) {
    var s = n5r(x + w, y0, 12, y1 - y0, 'fill="#000" opacity=".35" filter="url(#src-blur5)"') + n5r(x, y0, w, y1 - y0, 'fill="url(#n5-shaft)"'), k;
    for (k = 1; k < 5; k++) s += n5r(x + k * w / 5, y0 + 40, 1.4, y1 - y0 - 60, 'fill="#000" opacity=".22"');
    s += n5p('M' + (x - 10) + ',' + y0 + ' h' + (w + 20) + ' v12 q-4,22 -10,30 h' + (-w) + ' q-6,-8 -10,-30 Z', 'fill="url(#n5-shaft)"') + n5p('M' + (x - 4) + ',' + (y0 + 14) + ' q8,16 0,22 M' + (x + w + 4) + ',' + (y0 + 14) + ' q-8,16 0,22 M' + (x + w / 2) + ',' + (y0 + 12) + ' v26', 'stroke="#000" stroke-opacity=".35" stroke-width="2.4" fill="none"') + n5r(x - 10, y0, w + 20, 3, 'fill="#fff" opacity=".3"');
    return s + n5r(x - 6, y1 - 20, w + 12, 20, 'fill="url(#n5-shaft)"') + n5r(x - 6, y1 - 20, w + 12, 2, 'fill="#fff" opacity=".3"');
  }
  SHAPES.chapel = function (t) {
    var s = WALLS[t.th.wall](t), x, g = '';
    s += '<defs>' + lin('n5-shaft', 0, [[0, '#57524a'], [.25, '#c9c2b2'], [.45, '#a8a194'], [.8, '#5a554c'], [1, '#2e2b26']]) + '</defs>';
    // a deep cornice cut with a row of little pointed arches
    s += n5r(-300, -150, 1800, 196, 'fill="#8a8478" filter="url(#cm-wall)"') + n5r(-300, 46, 1800, 14, 'fill="#000" opacity=".5" filter="url(#src-blur5)"');
    for (x = -290; x < 1500; x += 58) g += n5p('M' + (x + 6) + ',40 V16 Q' + (x + 6) + ',-6 ' + (x + 29) + ',-12 Q' + (x + 52) + ',-6 ' + (x + 52) + ',16 V40 Z', 'fill="#221e1a"') + n5p('M' + (x + 6) + ',40 V16 Q' + (x + 6) + ',-6 ' + (x + 29) + ',-12', 'stroke="#000" stroke-opacity=".6" stroke-width="3" fill="none"') + n5p('M' + (x + 52) + ',40 V16 Q' + (x + 52) + ',-6 ' + (x + 29) + ',-12', 'stroke="#e8e0d0" stroke-opacity=".4" stroke-width="1.6" fill="none"') + n5c(x + 58, -2, 4, 'fill="#221e1a"');
    s += g + n5r(-300, 40, 1800, 6, 'fill="#c9c2b2"') + n5r(-300, 40, 1800, 1.4, 'fill="#fff" opacity=".4"');
    return s + n5column(-8, 36, 46, 474) + n5column(1172, 36, 46, 474);
  };
  OPENINGS['rose-window'] = function (t) {
    var cx = 912, cy = 190, cols = ['#c8283a', '#2a5ac8', '#e0a828', '#2a9a5a', '#7a3aa8', '#d85a28'], dark = t.time === 'night', s, fr, i, a0, a1, lead = '', sh = '';
    function pt(R, a) { return f1(cx + R * Math.cos(a)) + ',' + f1(cy + R * Math.sin(a)); }
    s = n5c(cx, cy, 112, 'fill="#0a0806"');
    for (i = 0; i < 12; i++) {
      a0 = i * Math.PI / 6 - Math.PI / 2 + .035; a1 = a0 + Math.PI / 6 - .07;
      s += n5p('M' + pt(30, a0) + ' L' + pt(64, a0) + ' Q' + pt(84, (a0 + a1) / 2) + ' ' + pt(64, a1) + ' L' + pt(30, a1) + ' Z', 'fill="' + cols[i % 6] + '"') + n5c(cx + 90 * Math.cos(a0 - .035), cy + 90 * Math.sin(a0 - .035), 11, 'fill="' + cols[(i + 3) % 6] + '"') + n5c(cx + 90 * Math.cos((a0 + a1) / 2), cy + 90 * Math.sin((a0 + a1) / 2), 6, 'fill="' + cols[(i + 1) % 6] + '"');
      lead += 'M' + pt(26, a0 - .035) + ' L' + pt(76, a0 - .035) + ' ';
    }
    s += n5c(cx, cy, 24, 'fill="#f0c440"');
    for (i = 0; i < 8; i++) s += n5p('M' + cx + ',' + (cy - 22) + ' l4,22 h-8 Z', 'fill="#fff4c0" transform="rotate(' + (i * 45) + ' ' + cx + ' ' + cy + ')"');
    s += n5c(cx, cy, 8, 'fill="#fffbe6"');
    // the day behind it moves across the glass
    s += '<defs>' + rad('n5-rglow', [[0, '#fff', .5], [.6, '#fff', .12], [1, '#fff', 0]]) + '</defs>' + n5c(cx, cy, 108, 'fill="url(#n5-rglow)"' + (dark ? ' opacity=".2"' : '')) + n5g('class="n5-vb n5-turn" style="transform-origin:' + cx + 'px ' + cy + 'px;animation-duration:44s"', n5p('M' + cx + ',' + cy + ' L' + (cx + 130) + ',' + (cy - 40) + ' L' + (cx + 130) + ',' + (cy + 30) + ' Z M' + cx + ',' + cy + ' L' + (cx - 130) + ',' + (cy + 50) + ' L' + (cx - 120) + ',' + (cy - 20) + ' Z', 'fill="#fff" opacity="' + (dark ? .05 : .16) + '"'));
    if (dark) s += n5c(cx, cy, 112, 'fill="#05060c" opacity=".55"');
    // the leads, then the stone tracery round them
    s += n5p(lead, 'stroke="#0c0a08" stroke-width="3.4" fill="none"') + n5c(cx, cy, 26, 'fill="none" stroke="#0c0a08" stroke-width="3.4"') + n5c(cx, cy, 76, 'fill="none" stroke="#0c0a08" stroke-width="3"');
    fr = n5p(RING, 'fill="none" stroke="#5e584e" stroke-width="20"') + n5p(RING, 'fill="none" stroke="#9a9488" stroke-width="12"') + n5p(RING, 'fill="none" stroke="#000" stroke-opacity=".5" stroke-width="2"') + n5c(cx, cy, 118, 'fill="none" stroke="#000" stroke-opacity=".4" stroke-width="2"') + n5p('M826,128 A108,108 0 0 1 900,84', 'stroke="#fff" stroke-opacity=".3" stroke-width="4" fill="none"');
    // its colours fall across the room
    if (!dark) cols.forEach(function (c, j) { var x0 = 850 + j * 22; sh += n5p('M' + x0 + ',250 l22,-6 L' + (x0 - 330 + j * 12) + ',520 l-70,10 Z', 'class="n5-shimmer" style="animation-duration:' + (5 + j) + 's;animation-delay:-' + (j * 1.3) + 's" fill="' + c + '" filter="url(#src-blur8)"'); });
    return mpOpen(RING, s, fr, n5g('opacity=".3" pointer-events="none"', sh));
  };
  DISPLAYS.niches = function (t) {
    var r = rng(133), a = t.th.accent, s, i, x, y, st = '';
    s = '<defs>' + mp3Defs() + lin('n5-nich', 1, [[0, '#050403'], [.7, '#1a1410'], [1, mix('#2a2018', a, .3)]]) + '<clipPath id="n5-nwall"><rect x="28" y="64" width="466" height="394"/></clipPath></defs>';
    s += n5r(34, 70, 470, 396, 'fill="#000" opacity=".5" filter="url(#src-blur8)"') + '<g clip-path="url(#n5-nwall)" filter="url(#mp-stone)">' + n5r(28, 64, 466, 394, 'fill="#3e3a34"') + n5stones(r, 28, 64, 494, 458, 38, 70, 40, '#948e82', '#5e594f', '#c8c0b0', .3) + '</g>';
    // twelve pointed niches, each with its own ledge
    CM_ROWS.forEach(function (ry) { for (i = 0; i < 4; i++) {
      x = CM_COLS[i]; var d = 'M' + (x - 42) + ',' + ry + ' V' + (ry - 64) + ' Q' + (x - 42) + ',' + (ry - 98) + ' ' + x + ',' + (ry - 108) + ' Q' + (x + 42) + ',' + (ry - 98) + ' ' + (x + 42) + ',' + (ry - 64) + ' V' + ry + ' Z';
      st += n5p(d, 'fill="url(#n5-nich)"') + n5p(d, 'fill="none" stroke="#000" stroke-opacity=".7" stroke-width="5" transform="translate(2,3)"') + n5p(d, 'fill="none" stroke="#d8d0c0" stroke-opacity=".55" stroke-width="2"') + n5e(x, ry - 4, 34, 12, 'fill="' + a + '" opacity=".16" filter="url(#src-blur5)"');
      st += n5r(x - 50, ry, 100, 9, 'rx="2" fill="#b0a898"') + n5r(x - 50, ry, 100, 2, 'fill="#fff" opacity=".4"') + n5r(x - 48, ry + 9, 96, 8, 'fill="#000" opacity=".5" filter="url(#src-blur3)"');
      if (i < 3) st += n5p('M' + (x + 55) + ',' + (ry - 60) + ' v24 m-8,-16 h16', 'stroke="#2a2620" stroke-width="3.4" stroke-linecap="round"') + n5p('M' + (x + 56) + ',' + (ry - 59) + ' v24 m-8,-16 h16', 'stroke="#e8e0d0" stroke-opacity=".3" stroke-width="1"');
    } });
    s += st + n5r(28, 64, 466, 10, 'fill="#c0b8a8"') + n5r(28, 64, 466, 2, 'fill="#fff" opacity=".4"');
    return { art: '<g filter="url(#cm-unitlit)">' + s + '</g>', slots: cmGrid(), zone: [34, 96, 454, 320], wallsh: false, decor: false, outline: false, label: 'The niches',
      // a votive burns at the foot of every niche that holds something
      front: function (it, p) { var x = p[0] + 33, y = p[1]; return n5c(x, y - 12, 16, 'fill="url(#cm-glow)" opacity=".5" class="cm-pulse"') + n5r(x - 5, y - 10, 10, 10, 'rx="2" fill="' + mix(a, '#a01c1c', .7) + '"') + n5r(x - 5, y - 10, 3, 10, 'fill="#fff" opacity=".3"') + '<path class="src-flame" d="M' + x + ',' + (y - 22) + ' c-3.4,5 -3.4,8 0,11 c3.4,-3 3.4,-6 0,-11 Z" fill="url(#src-flame)"/>'; } };
  };
  COUNTERS.altar = function (t) {
    var r = rng(135), s, g = '', x, c = mix(t.th.cloth, '#0a0606', .08), cl = mix(t.th.cloth, '#ffffff', .25), tr = t.th.trim;
    s = '<defs>' + mp3Defs() + lin('n5-altt', 1, [[0, '#b8b0a0'], [1, '#6e685c']]) + lin('n5-altf', 1, [[0, '#7e786c'], [1, '#1c1a16']]) + '</defs><clipPath id="cm-topclip"><rect x="-300" y="452" width="1800" height="58"/></clipPath>';
    s += '<g filter="url(#mp-stone)">' + n5r(-300, 452, 1800, 58, 'fill="url(#n5-altt)"') + n5r(-300, 510, 1800, 390, 'fill="url(#n5-altf)"') + '</g>' + n5r(-300, 506, 1800, 3, 'fill="' + t.lc + '" opacity=".45"') + n5r(-300, 510, 1800, 14, 'fill="#3a362e"') + n5r(-300, 524, 1800, 22, 'fill="#000" opacity=".5" filter="url(#src-blur5)"');
    // a blind arcade cut along the front
    for (x = -290; x < 1500; x += 124) g += n5p('M' + (x + 14) + ',900 V600 Q' + (x + 14) + ',556 ' + (x + 62) + ',540 Q' + (x + 110) + ',556 ' + (x + 110) + ',600 V900 Z', 'fill="#000" opacity=".4"') + n5p('M' + (x + 14) + ',900 V600 Q' + (x + 14) + ',556 ' + (x + 62) + ',540', 'stroke="#000" stroke-opacity=".7" stroke-width="4" fill="none"') + n5p('M' + (x + 110) + ',900 V600 Q' + (x + 110) + ',556 ' + (x + 62) + ',540', 'stroke="#e8e0d0" stroke-opacity=".35" stroke-width="2" fill="none"') + n5r(x - 4, 596, 22, 8, 'fill="#9a9488"') + n5r(x + 2, 604, 10, 300, 'fill="#000" opacity=".2"');
    // the altar cloth: a long fall with a gold sun worked on it, and a fringe
    g += n5sh('M500,506 H780 V690 H500 Z', 8, 4, .5, 8) + n5drape(500, 506, 280, 180, c, 7) + n5r(500, 506, 280, 16, 'fill="#000" opacity=".3"') + n5r(500, 660, 280, 6, 'fill="' + tr + '"') + n5p('M500,690 H780', 'stroke="' + tr + '" stroke-width="12" stroke-dasharray="2 3"') + n5r(500, 520, 280, 3, 'fill="' + tr + '" opacity=".8"');
    g += n5c(640, 590, 26, 'fill="none" stroke="' + tr + '" stroke-width="4"') + n5c(640, 590, 12, 'fill="' + tr + '"');
    for (x = 0; x < 12; x++) g += n5p('M640,552 l4,-18 l-4,-8 l-4,8 Z', 'fill="' + tr + '" transform="rotate(' + (x * 30) + ' 640 590)"');
    return s + g;
  };
  LIGHTS.votives = function (t) {
    var x = t.lp.x, s = '<g data-k="lamp">' + n5e(x, 482, 60, 8, 'fill="url(#src-ao)"'), i, j, cx, cy, cols = ['#b02a2a', '#c8502a', '#2a4a9a'];
    // three steps of iron, a row of glass cups on each
    for (j = 0; j < 3; j++) {
      cy = 480 - j * 22; s += n5r(x - 52 + j * 12, cy - 6, 104 - j * 24, 6, 'rx="1.6" fill="#1a1a1e"') + n5r(x - 52 + j * 12, cy - 6, 104 - j * 24, 1.2, 'fill="#9aa0a8" opacity=".5"') + n5r(x - 48 + j * 12, cy, 4, 22, 'fill="#101012"') + n5r(x + 44 - j * 12, cy, 4, 22, 'fill="#101012"');
      for (i = 0; i < 5 - j; i++) { cx = x - 40 + j * 12 + i * 20 + (j ? 0 : 0); s += n5r(cx - 6, cy - 19, 12, 13, 'rx="2.6" fill="' + cols[(i + j) % 3] + '"') + n5r(cx - 6, cy - 19, 12, 13, 'rx="2.6" fill="#ffd27a" opacity=".45" class="n5-beat" style="animation-delay:-' + f1((i * 7 + j * 3) % 5 * .9) + 's"') + n5r(cx - 5, cy - 18, 3, 11, 'fill="#fff" opacity=".35"') + '<path class="src-flame" style="animation-delay:-' + f1((i * 3 + j) * .31) + 's" d="M' + cx + ',' + (cy - 31) + ' c-3.4,5 -3.4,8 0,11 c3.4,-3 3.4,-6 0,-11 Z" fill="url(#src-flame)"/>'; }
    }
    return { back: '', front: s + '<circle cx="' + x + '" cy="440" r="120" fill="url(#cm-bloom)" class="cm-pulse" opacity=".8"/></g>' };
  };
  PROPS.censer = function (t, p) {
    var x = 744, y = 128, tr = t.th.trim, s, i, g;
    g = n5p('M' + x + ',40 L' + (x - 12) + ',' + (y - 14) + ' M' + x + ',40 L' + (x + 12) + ',' + (y - 14) + ' M' + x + ',40 V' + (y - 20), 'stroke="' + tr + '" stroke-width="1.4" stroke-dasharray="3 2" fill="none"');
    g += n5c(x, y + 4, 22, 'fill="url(#cm-glow)" opacity=".5" class="cm-pulse"') + n5p('M' + (x - 14) + ',' + y + ' q0,-16 14,-22 q14,6 14,22 Z', 'fill="url(#cm-trim)"') + n5p('M' + (x - 15) + ',' + y + ' h30 q0,16 -15,18 q-15,-2 -15,-18 Z', 'fill="url(#cm-trim)"') + n5r(x - 16, y - 2, 32, 4, 'rx="2" fill="' + mix(tr, '#000', .35) + '"') + n5r(x - 6, y + 18, 12, 4, 'fill="' + mix(tr, '#000', .3) + '"') + n5c(x, y - 24, 3, 'fill="url(#cm-trim)"');
    for (i = -1; i <= 1; i++) g += n5c(x + i * 7, y - 9, 2, 'fill="#ff9a40"') + n5c(x + i * 7, y + 8, 1.8, 'fill="#ff8a30"');
    s = n5g('class="n5-vb n5-swing" style="transform-origin:' + x + 'px 40px"', g);
    for (i = 0; i < 4; i++) s += n5e(x + (i - 1.5) * 10, y - 6, 12, 8, 'class="n5-fb n5-puff" fill="#d8d4e0" filter="url(#src-blur3)" style="--fx:' + ((i % 2 ? 1 : -1) * (10 + i * 6)) + 'px;--fy:-' + (70 + i * 14) + 'px;animation-delay:-' + f1(i * 1.5) + 's"');
    return n5g('class="src-nohit"', s);
  };
  PROPS.casket = function (t, p) {
    var x = p.x, y = 492, a = t.th.accent, s = '<defs>' + n5Metals() + '</defs>' + n5e(x, y + 4, 62, 9, 'fill="url(#src-ao)"'), i;
    // a cushion, and on it a little house of gold with a crystal in its side
    s += n5p('M' + (x - 54) + ',' + y + ' q-6,-14 8,-16 h92 q14,2 8,16 Z', 'fill="' + mix(t.th.cloth, '#000', .1) + '"') + n5p('M' + (x - 46) + ',' + (y - 14) + ' h92', 'stroke="#fff" stroke-opacity=".2" stroke-width="2"') + n5c(x - 52, y - 2, 4, 'fill="' + t.th.trim + '"') + n5c(x + 52, y - 2, 4, 'fill="' + t.th.trim + '"');
    s += n5r(x - 36, y - 52, 72, 38, 'fill="url(#n5-gold)"') + n5p('M' + (x - 40) + ',' + (y - 52) + ' L' + (x - 28) + ',' + (y - 78) + ' H' + (x + 28) + ' L' + (x + 40) + ',' + (y - 52) + ' Z', 'fill="url(#n5-gold)"') + n5p('M' + (x - 40) + ',' + (y - 52) + ' H' + (x + 40), 'stroke="#3a2606" stroke-width="2"') + n5r(x - 36, y - 52, 72, 38, 'fill="#000" opacity=".12"');
    for (i = -2; i <= 2; i++) s += n5c(x + i * 13, y - 65, 3, 'fill="' + ['#c8283a', '#2a5ac8', '#2a9a5a'][(i + 2) % 3] + '"') + n5c(x + i * 13 - 1, y - 66, 1, 'fill="#fff" opacity=".8"');
    s += n5p('M' + (x - 9) + ',' + (y - 18) + ' V' + (y - 38) + ' Q' + x + ',' + (y - 50) + ' ' + (x + 9) + ',' + (y - 38) + ' V' + (y - 18) + ' Z', 'fill="#1a2028"') + n5p('M' + (x - 9) + ',' + (y - 18) + ' V' + (y - 38) + ' Q' + x + ',' + (y - 50) + ' ' + (x + 9) + ',' + (y - 38) + ' V' + (y - 18) + ' Z', 'class="n5-beat" fill="' + mix(a, '#ffffff', .5) + '" opacity=".8"') + n5c(x, y - 30, 16, 'fill="url(#cm-glow)" opacity=".5" class="cm-pulse"');
    s += n5p('M' + x + ',' + (y - 78) + ' v-14 m-5,5 h10', 'stroke="url(#n5-gold)" stroke-width="3"') + n5c(x - 24, y - 70, 3, 'class="n5-fb n5-glint" fill="#fff"') + n5c(x + 30, y - 44, 2.4, 'class="n5-fb n5-glint" fill="#fff" style="animation-delay:-2.3s"');
    return n5g('class="src-nohit"', s);
  };

  /* ---- The trading post: a log cabin under snow, a frosted window, plank
     shelves hung with gear, a counter stacked from firewood and a stove that
     never goes out. ---- */
  CM_OPT.shape.push('cabin'); CM_OPT.wall.push('logs'); CM_OPT.opening.push('frost-window'); CM_OPT.display.push('gear-wall'); CM_OPT.counter.push('woodpile');
  CM_WALLFQ.logs = [.03, 2.6, 25]; CM_OPEN_INFO['frost-window'] = 'The frosted window';
  n5prop('stove', 'tallR', 'back'); n5prop('pelts', 'ceil', 'back'); n5prop('sacks', 'cL', 'front', 150);

  WALLS.logs = function (t) {
    var r = rng(141), base = mix(t.th.wood, '#8a6a48', .45), s = '<defs>' + mp3Defs() + lin('n5-logv', 1, [[0, mix(base, '#fff', .28)], [.3, base], [.8, mix(base, '#000', .5)], [1, mix(base, '#000', .78)]]) + '</defs>', y, i, x, g = '';
    for (y = -150; y < 470; y += 54) { g += n5r(-300, y, 1800, 52, 'fill="url(#n5-logv)"'); for (i = 0; i < 9; i++) { x = -300 + r() * 1800; g += n5p('M' + f1(x) + ',' + f1(y + 8 + r() * 36) + ' q' + f1(30 + r() * 60) + ',' + f1((r() - .5) * 6) + ' ' + f1(70 + r() * 120) + ',0', 'stroke="#0a0604" stroke-opacity="' + f1(.3 + r() * .4) + '" stroke-width="' + f1(1 + r() * 2) + '" fill="none" stroke-linecap="round"'); } if (r() < .7) { x = -260 + r() * 1700; g += n5e(x, y + 26, 12 + r() * 8, 8 + r() * 5, 'fill="#1a0e06" opacity=".7"') + n5e(x, y + 26, 20 + r() * 8, 14, 'fill="none" stroke="#1a0e06" stroke-opacity=".4" stroke-width="2"'); } }
    s += '<g filter="url(#mp-barkh)">' + g + '</g>';
    // moss and clay packed between them
    for (y = -150 + 52; y < 470; y += 54) s += n5r(-300, y - 1, 1800, 4, 'fill="#b8ac90" opacity=".5"') + n5r(-300, y + 3, 1800, 6, 'fill="#000" opacity=".45" filter="url(#src-blur3)"');
    s += '<rect ' + FULL + ' fill="#140c06" filter="url(#cm-stain)" opacity=".45"/>';
    // a pair of snowshoes hung low by the window
    [-24, 24].forEach(function (a) { s += n5g('transform="rotate(' + a + ' 748 398)"', n5e(751, 392, 17, 44, 'fill="none" stroke="#000" stroke-opacity=".5" stroke-width="5"') + n5e(748, 388, 17, 44, 'fill="#1a1008" opacity=".35"') + n5p('M734,366 L762,410 M762,366 L734,410 M733,388 H763 M736,372 H760 M736,404 H760', 'stroke="#c8b488" stroke-opacity=".7" stroke-width="1.2"') + n5e(748, 388, 17, 44, 'fill="none" stroke="#b08a52" stroke-width="4"') + n5p('M748,432 v22', 'stroke="#b08a52" stroke-width="5" stroke-linecap="round"')); });
    return s;
  };
  SHAPES.cabin = function (t) {
    var s = WALLS[t.th.wall](t), x, k, c = mix(t.th.wood, '#b8905c', .55);
    // the roof poles, their cut ends toward you
    s += n5r(-300, -150, 1800, 170, 'fill="#0a0705"') + n5r(-300, 0, 1800, 34, 'fill="#000" filter="url(#cm-beam)"') + n5r(-300, 0, 1800, 2, 'fill="' + t.lc + '" opacity=".3"') + n5r(-300, 34, 1800, 14, 'fill="#000" opacity=".5" filter="url(#src-blur3)"');
    for (x = -270; x < 1500; x += 150) { s += n5c(x + 5, 26, 27, 'fill="#000" opacity=".5" filter="url(#src-blur3)"') + n5c(x, 20, 25, 'fill="' + mix(t.th.wood, '#000', .5) + '"') + n5c(x, 20, 21, 'fill="' + c + '"'); for (k = 1; k < 4; k++) s += n5c(x, 20, k * 5.6, 'fill="none" stroke="#5a3e20" stroke-opacity=".6" stroke-width="1"'); s += n5p('M' + x + ',20 l14,-10 M' + x + ',20 l-6,16', 'stroke="#3a2610" stroke-opacity=".7" stroke-width="1.2"') + n5c(x, 20, 21, 'fill="#000" opacity=".3"'); }
    return s + cmPost(-6, 34, 34, 474) + cmPost(1172, 34, 34, 474);
  };
  OPENINGS['frost-window'] = function (t) {
    var d = 'M812,298 V88 H1012 V298 Z', r = rng(143), s, fr, i, x, y, snow = '', fro = '', pane;
    s = '<defs>' + lin('n5-fsky', 1, [[0, '#141c3c'], [.55, '#3a4a7a'], [1, '#c88a8a']]) + rad('n5-frost', [[0, '#fff', 0], [.55, '#fff', 0], [.85, '#e8f4ff', .5], [1, '#fff', .9]]) + '</defs>' + n5r(806, 82, 212, 222, 'fill="url(#n5-fsky)"') + mpStars(r, 26, 808, 84, 208, 90, true);
    // far peaks under snow, a dark wood below them, drifts to the sill
    s += n5p(n5ridge(r, 806, 1018, 196, 80, 34, 300), 'fill="#7a8ab4"') + n5p(n5ridge(r, 806, 1018, 232, 30, 26, 300), 'fill="#3a4870"');
    for (i = 0; i < 12; i++) s += n5pine(812 + i * 18 + r() * 8, 268 + r() * 8, 46 + r() * 34, mix('#101c28', '#1e3040', r()), '#dce8f8');
    s += n5p('M806,270 q40,-12 80,-2 t70,-4 t62,2 V304 H806 Z', 'fill="#dfe8f6"') + n5p('M806,284 q60,-10 110,0 t102,-4 V304 H806 Z', 'fill="#b8c8e4"');
    for (i = 0; i < 46; i++) snow += n5c(800 + r() * 240, 60 + r() * 60, 1 + r() * 1.8, 'class="n5-snow" fill="#fff" opacity="' + f1(.5 + r() * .5) + '" style="--fx:' + f1(-20 - r() * 40) + 'px;--fy:' + f1(230 + r() * 40) + 'px;animation-duration:' + f1(5 + r() * 7) + 's;animation-delay:-' + f1(r() * 12) + 's"');
    s += snow;
    // frost creeping in from the corners of each pane, snow on the bars
    [[812, 88], [912, 88], [812, 193], [912, 193]].forEach(function (q) {
      fro += n5r(q[0], q[1], 100, 105, 'fill="url(#n5-frost)" opacity=".8"');
      [[0, 0, 1, 1], [100, 0, -1, 1], [0, 105, 1, -1], [100, 105, -1, -1]].forEach(function (c) { for (i = 0; i < 4; i++) { x = q[0] + c[0]; y = q[1] + c[1]; var L = 16 + r() * 22, a = r() * 1.4 + .1; fro += n5p('M' + x + ',' + y + ' l' + f1(c[2] * Math.cos(a) * L) + ',' + f1(c[3] * Math.sin(a) * L) + ' m' + f1(-c[2] * Math.cos(a) * L * .5) + ',' + f1(-c[3] * Math.sin(a) * L * .5) + ' l' + f1(c[2] * Math.cos(a + .7) * L * .4) + ',' + f1(c[3] * Math.sin(a + .7) * L * .4) + ' m' + f1(-c[2] * Math.cos(a + .7) * L * .4) + ',' + f1(-c[3] * Math.sin(a + .7) * L * .4) + ' l' + f1(c[2] * Math.cos(a - .7) * L * .4) + ',' + f1(c[3] * Math.sin(a - .7) * L * .4), 'stroke="#fff" stroke-opacity=".7" stroke-width="1" fill="none"'); } });
      fro += n5p('M' + q[0] + ',' + (q[1] + 105) + ' v-8 q16,-10 34,-4 t36,-2 t30,2 v12 Z', 'fill="#f4f8ff"');
    });
    s += fro + n5p('M822,290 L890,96 h30 L852,290 Z', 'fill="#fff" opacity=".05"');
    pane = mix(t.th.wood, '#000', .4);
    fr = n5p('M912,88 V298 M812,193 H1012', 'stroke="' + pane + '" stroke-width="8" fill="none"') + n5p('M912,88 V298 M812,193 H1012', 'stroke="' + mix(t.th.wood, '#fff', .2) + '" stroke-opacity=".5" stroke-width="1.2" fill="none" transform="translate(-3,-3)"') + cmFrame(t, d, 16);
    fr += n5r(794, 298, 236, 16, 'rx="2" fill="#000" filter="url(#cm-beam)"') + n5r(794, 298, 236, 2, 'fill="' + t.lc + '" opacity=".5"') + n5r(794, 314, 236, 12, 'fill="#000" opacity=".5" filter="url(#src-blur3)"');
    // a candle end and a tin mug on the sill
    fr += n5c(986, 276, 26, 'fill="url(#cm-glow)" opacity=".6" class="cm-pulse"') + cmCandle(986, 298, 16, true) + n5e(986, 298, 11, 2.6, 'fill="#3a3c42"') + n5r(838, 280, 20, 18, 'rx="2" fill="#8a929c"') + n5r(838, 280, 6, 18, 'fill="#fff" opacity=".3"') + n5p('M858,284 q9,1 8,7 q-1,6 -8,5', 'stroke="#8a929c" stroke-width="2.6" fill="none"') + n5e(848, 280, 10, 2.4, 'fill="#2a2e34"');
    return mpOpen(d, s, fr);
  };
  DISPLAYS['gear-wall'] = function (t) {
    var r = rng(145), s = n5r(34, 84, 454, 370, 'fill="#000" opacity=".28" filter="url(#src-blur8)"'), i, x;
    function fur(x, y, h, c) { var d = 'M' + (x - 6) + ',' + y + ' q-14,' + (h * .3) + ' -16,' + (h * .7) + ' q4,' + (h * .34) + ' 22,' + (h * .3) + ' q18,4 22,-' + (h * .3) + ' q-2,-' + (h * .4) + ' -16,-' + (h * .7) + ' Z'; return n5sh(d, 4, 4, .5, 3) + n5p(d, 'fill="' + c + '"') + n5p(d, 'fill="none" stroke="' + mix(c, '#e8dcc8', .3) + '" stroke-width="2.4" stroke-dasharray="1 2.4"') + n5p('M' + x + ',' + (y + 6) + ' q-4,' + (h * .5) + ' 0,' + (h * .9), 'stroke="' + mix(c, '#000', .35) + '" stroke-opacity=".7" stroke-width="5" fill="none"') + n5c(x, y, 3, 'fill="#2a2a2e"'); }
    function coil(x, y) { var o = n5c(x, y - 2, 3, 'fill="#2a2a2e"') + n5e(x + 3, y + 26, 18, 24, 'fill="none" stroke="#000" stroke-opacity=".5" stroke-width="7"'), k; for (k = 0; k < 4; k++) o += n5e(x + (k - 1.5) * 1.6, y + 22 + k, 16 - k * .6, 22 - k, 'fill="none" stroke="' + mix('#c8a868', '#8a6a38', k / 4) + '" stroke-width="4"'); return o + n5p('M' + (x - 12) + ',' + (y + 14) + ' l22,6 M' + (x - 14) + ',' + (y + 24) + ' l26,6', 'stroke="#6a4a24" stroke-opacity=".5" stroke-width="1"'); }
    function trap(x, y) { return n5c(x, y - 2, 3, 'fill="#2a2a2e"') + n5p('M' + x + ',' + y + ' v20', 'stroke="#4a4c52" stroke-width="2" stroke-dasharray="3 2"') + n5p('M' + (x - 16) + ',' + (y + 40) + ' a16,18 0 0 1 32,0 M' + (x - 16) + ',' + (y + 40) + ' h32 M' + (x - 12) + ',' + (y + 40) + ' l3,-7 l3,7 l3,-7 l3,7 l3,-7 l3,7 l3,-7 l3,7', 'stroke="#5a5e66" stroke-width="2.6" fill="none"') + n5r(x - 20, y + 40, 40, 5, 'rx="2" fill="#3a3c42"'); }
    // plank shelves on iron brackets, with the heavy goods hung from pegs beneath
    CM_ROWS.forEach(function (ry, j) {
      for (i = 0; i < 4; i++) { x = 60 + i * 134; s += n5p('M' + x + ',' + (ry + 12) + ' v30 l30,-30 Z', 'fill="none" stroke="#1c1d20" stroke-width="4"') + n5p('M' + x + ',' + (ry + 12) + ' v30', 'stroke="#5a5e66" stroke-width="1" transform="translate(-1,0)"'); }
      s += cmShelfBoard(t, ry, 36, 486);
      if (j < 2) s += fur(151, ry + 18, 60, j ? '#5a3a22' : '#8a8478') + coil(261, ry + 18) + (j ? fur(371, ry + 18, 54, '#9a8a70') : trap(371, ry + 16));
    });
    return { art: '<g filter="url(#cm-unitlit)">' + s + '</g>', slots: cmGrid(), zone: [34, 96, 454, 320], wallsh: true, decor: true, label: 'The gear shelves' };
  };
  COUNTERS.woodpile = function (t) {
    var s = cmTop(t, 'M-300,452 H1500 V512 H-300 Z'), r = rng(147), g = '', x, y, k, rr, c, d, row = 0;
    s += '<clipPath id="cm-topclip"><rect x="-300" y="452" width="1800" height="60"/></clipPath>';
    // the top keeps its bark along the front edge
    d = 'M-300,504'; for (x = -300; x < 1500; x += 30) d += ' q15,' + f1(4 + r() * 8) + ' 30,0'; d += ' V522 H-300 Z';
    s += n5p(d, 'fill="#1c120a"') + n5p(d, 'fill="none" stroke="' + t.lc + '" stroke-opacity=".3" stroke-width="1.4"') + n5r(-300, 518, 1800, 382, 'fill="#060403"');
    // split logs stacked end-on, every one with its rings and its check
    for (y = 548; y < 900; y += 44, row++) for (x = -300 + (row % 2) * 26; x < 1500; x += 52) {
      rr = 21 + r() * 5; c = mix('#c09a62', r() < .5 ? '#8a6234' : '#e0c088', r() * .5); var cx = x + (r() - .5) * 6, cy = y + (r() - .5) * 6;
      g += n5c(cx + 2, cy + 3, rr + 2, 'fill="#000" opacity=".6"') + n5c(cx, cy, rr + 1.6, 'fill="#24160c"') + n5c(cx, cy, rr - 1.6, 'fill="' + c + '"');
      for (k = 1; k < 4; k++) g += n5e(cx + (r() - .5) * 2, cy, (rr - 2) * k / 4, (rr - 2) * k / 4 * (.9 + r() * .1), 'fill="none" stroke="#6a4420" stroke-opacity=".5" stroke-width=".9"');
      k = r() * 6.28; g += n5p('M' + f1(cx) + ',' + f1(cy) + ' l' + f1(Math.cos(k) * rr * .9) + ',' + f1(Math.sin(k) * rr * .9), 'stroke="#2a1608" stroke-width="1.4"') + n5p('M' + f1(cx - rr * .7) + ',' + f1(cy - rr * .5) + ' a' + f1(rr) + ',' + f1(rr) + ' 0 0 1 ' + f1(rr * .9) + ',' + f1(-rr * .36), 'stroke="#fff" stroke-opacity=".2" stroke-width="1.6" fill="none"');
    }
    // a striped trade blanket thrown over one end
    g += n5sh('M860,500 H1060 V624 H860 Z', 8, 5, .55, 8) + n5drape(860, 500, 200, 120, mix(t.th.cloth, '#0a0606', .05), 5);
    [[512, '#1a1a1e', 10], [530, '#e8c860', 6], [544, '#2a5a3a', 6], [592, '#1a1a1e', 10]].forEach(function (q) { g += n5r(860, q[0], 200, q[2], 'fill="' + q[1] + '" opacity=".85"'); });
    g += n5p('M860,624 H1060', 'stroke="' + mix(t.th.cloth, '#fff', .3) + '" stroke-width="10" stroke-dasharray="2 3"') + n5r(860, 500, 200, 14, 'fill="#000" opacity=".3"');
    return '<g filter="url(#cm-woodlit)">' + s + g + n5r(-300, 518, 1800, 34, 'fill="#000" opacity=".55" filter="url(#src-blur8)"') + '</g>';
  };
  PROPS.stove = function (t, p) {
    var x = p.x + 14, s = '<defs>' + n5Metals() + rad('n5-mica', [[0, '#fff0b0'], [.5, '#ff9a30'], [1, '#c8300a']]) + '</defs>', i, y;
    // the pipe to the roof, jointed, then the belly on its collar
    s += n5r(x - 2, -150, 44, 480, 'fill="#000" opacity=".4" filter="url(#src-blur8)"') + n5r(x - 15, -150, 30, 474, 'fill="url(#n5-black)"');
    for (y = 60; y < 320; y += 96) s += n5r(x - 17, y, 34, 7, 'rx="2" fill="url(#n5-black)"') + n5r(x - 17, y + 7, 34, 3, 'fill="#000" opacity=".5"');
    s += n5e(x + 8, 420, 78, 90, 'fill="#000" opacity=".45" filter="url(#src-blur8)"') + n5p('M' + (x - 40) + ',340 Q' + (x - 78) + ',400 ' + (x - 62) + ',500 H' + (x + 62) + ' Q' + (x + 78) + ',400 ' + (x + 40) + ',340 Z', 'fill="url(#n5-black)"') + n5r(x - 44, 322, 88, 20, 'rx="3" fill="url(#n5-black)"') + n5e(x, 322, 48, 7, 'fill="#2e3036"') + n5e(x, 321, 40, 5, 'fill="#17181b"');
    [356, 462].forEach(function (yy) { s += n5p('M' + (x - 54) + ',' + yy + ' q54,10 108,0', 'stroke="#000" stroke-opacity=".6" stroke-width="4" fill="none"') + n5p('M' + (x - 54) + ',' + (yy + 3) + ' q54,10 108,0', 'stroke="#9aa0a8" stroke-opacity=".3" stroke-width="1.2" fill="none"'); });
    // the door, its mica lit from inside
    s += n5c(x, 410, 70, 'fill="url(#cm-glow)" opacity=".5" class="cm-pulse"') + n5r(x - 32, 376, 64, 66, 'rx="8" fill="#0e0f11" stroke="#3e4046" stroke-width="2.4"');
    for (i = 0; i < 3; i++) s += n5r(x - 24 + i * 17, 386, 14, 34, 'rx="2" fill="url(#n5-mica)" class="n5-beat" style="animation-delay:-' + (i * 1.4) + 's;animation-duration:' + (3 + i * .7) + 's"');
    s += n5r(x - 24, 428, 48, 6, 'rx="3" fill="#1a1b1e"') + n5c(x - 14, 431, 1.6, 'fill="#ff9a30"') + n5c(x, 431, 1.6, 'fill="#ff9a30"') + n5c(x + 14, 431, 1.6, 'fill="#ff9a30"') + n5p('M' + (x + 32) + ',400 h8 v16', 'stroke="#6a6e76" stroke-width="3" fill="none" stroke-linecap="round"');
    // a kettle kept on the top, always near the boil
    s += n5p('M' + (x - 30) + ',318 q-2,-24 14,-26 h20 q16,2 14,26 Z', 'fill="url(#n5-copper)"') + n5p('M' + (x + 16) + ',306 l18,-10 l2,5 l-18,12', 'fill="url(#n5-copper)"') + n5p('M' + (x - 18) + ',292 q14,-22 28,0', 'stroke="#1a1a1e" stroke-width="3" fill="none"') + n5e(x - 4, 292, 12, 3, 'fill="#5a2410"') + n5c(x - 4, 288, 3, 'fill="#1a1a1e"');
    [0, 1, 2].forEach(function (j) { s += n5e(x + 38 + j * 3, 290, 8, 6, 'class="cm-steam" fill="#e8e4f0" style="animation-delay:-' + (j * 1.6) + 's"'); });
    return n5g('class="src-nohit"', s);
  };
  PROPS.pelts = function (t, p) {
    var r = rng(149), s = n5r(498, 52, 296, 7, 'rx="3.5" fill="' + mix(t.th.wood, '#000', .3) + '"') + n5r(498, 52, 296, 1.6, 'fill="' + t.lc + '" opacity=".3"');
    function fur(x, y, h, c) { var d = 'M' + (x - 8) + ',' + y + ' q-16,' + f1(h * .3) + ' -18,' + f1(h * .7) + ' q4,' + f1(h * .34) + ' 26,' + f1(h * .3) + ' q22,4 26,-' + f1(h * .3) + ' q-2,-' + f1(h * .4) + ' -18,-' + f1(h * .7) + ' Z'; return n5sh(d, -5, 4, .4, 3) + n5p(d, 'fill="' + c + '"') + n5p(d, 'fill="none" stroke="' + mix(c, '#e8dcc8', .3) + '" stroke-width="2.4" stroke-dasharray="1 2.4"') + n5p('M' + x + ',' + (y + 6) + ' q-5,' + f1(h * .5) + ' 0,' + f1(h * .9), 'stroke="' + mix(c, '#000', .35) + '" stroke-opacity=".7" stroke-width="6" fill="none"'); }
    [[516, 70, '#6a4a2c'], [706, 96, '#8a8680'], [752, 120, '#4a3020']].forEach(function (q, j) { s += n5g('class="n5-vb n5-sway" style="transform-origin:' + q[0] + 'px 56px;animation-duration:' + f1(7 + r() * 3) + 's;animation-delay:-' + f1(r() * 6) + 's"', n5p('M' + q[0] + ',56 v8', 'stroke="#a88a5a" stroke-width="2"') + fur(q[0], 62, q[1], q[2])); });
    return n5g('class="src-nohit"', s);
  };
  PROPS.sacks = function (t, p) {
    var x = p.x, y = 494, r = rng(151), s = n5e(x, y + 3, 80, 10, 'fill="url(#src-ao)"'), k;
    // a crate of tins, a sack of flour and a mug left by them
    s += n5r(x + 2, y - 54, 70, 54, 'fill="' + mix(t.th.wood, '#b8905c', .35) + '"') + n5p('M' + (x + 2) + ',' + (y - 36) + ' h70 M' + (x + 2) + ',' + (y - 18) + ' h70', 'stroke="#2a1a0c" stroke-width="1.6"') + n5r(x + 2, y - 54, 70, 54, 'fill="none" stroke="#2a1a0c" stroke-width="2"') + n5r(x + 2, y - 54, 8, 54, 'fill="#000" opacity=".25"');
    [0, 1, 2].forEach(function (j) { s += n5r(x + 10 + j * 20, y - 72, 16, 20, 'rx="2" fill="' + ['#b83a2a', '#d8b040', '#3a6a8a'][j] + '"') + n5r(x + 10 + j * 20, y - 66, 16, 7, 'fill="#efe6d0"') + n5e(x + 18 + j * 20, y - 72, 8, 2, 'fill="#9aa0a8"'); });
    var d = 'M' + (x - 70) + ',' + y + ' Q' + (x - 78) + ',' + (y - 40) + ' ' + (x - 64) + ',' + (y - 66) + ' L' + (x - 72) + ',' + (y - 82) + ' L' + (x - 50) + ',' + (y - 72) + ' Q' + (x - 36) + ',' + (y - 80) + ' ' + (x - 22) + ',' + (y - 72) + ' L' + (x - 2) + ',' + (y - 82) + ' L' + (x - 8) + ',' + (y - 66) + ' Q' + (x + 6) + ',' + (y - 40) + ' ' + (x - 2) + ',' + y + ' Z';
    s += n5p(d, 'fill="#e4dcc8"') + n5p('M' + (x - 66) + ',' + (y - 60) + ' Q' + (x - 72) + ',' + (y - 30) + ' ' + (x - 66) + ',' + (y - 2), 'stroke="#fff" stroke-opacity=".5" stroke-width="5" fill="none"') + n5p('M' + (x - 14) + ',' + (y - 60) + ' Q' + (x - 4) + ',' + (y - 30) + ' ' + (x - 8) + ',' + (y - 2), 'stroke="#000" stroke-opacity=".18" stroke-width="8" fill="none"');
    for (k = 1; k < 6; k++) s += n5p('M' + (x - 70) + ',' + (y - k * 11) + ' q34,4 68,0', 'stroke="#8a7e64" stroke-opacity=".3" stroke-width=".9" fill="none"');
    s += '<text x="' + (x - 36) + '" y="' + (y - 30) + '" text-anchor="middle" font-family="Georgia,serif" font-weight="700" font-size="14" fill="#3a4a7a" opacity=".8" letter-spacing="1.5">FLOUR</text>' + n5p('M' + (x - 56) + ',' + (y - 22) + ' h40', 'stroke="#b83a2a" stroke-opacity=".7" stroke-width="2.4"');
    return n5g('class="src-nohit"', s);
  };

  CM_WHERE.casks = 'among the casks'; CM_WHERE.bolts = 'among the bolts of cloth'; CM_WHERE['scroll-rack'] = 'in the scroll rack'; CM_WHERE.hutches = 'in the hutches'; CM_WHERE.niches = 'in its niche'; CM_WHERE['gear-wall'] = 'on the gear wall';
  CM_STARTERS.push(
    { name: 'The Taproom', version: 1, skeleton: { shape: 'taproom', opening: 'hearth', display: 'casks', counter: 'bar' },
      theme: { wall: 'daub', wood: '#5a3a22', cloth: '#7a2a22', accent: '#e8a040', trim: '#b89a58', light: 'firelight', warmth: .95, outside: 'night' }, props: ['tankards', 'keg', 'antlers', 'smoke'] },
    { name: 'The Cutting Room', version: 1, skeleton: { shape: 'parlour', opening: 'bay-window', display: 'bolts', counter: 'cutting' },
      theme: { wall: 'damask', wood: '#4a2c1c', cloth: '#6a2438', accent: '#e8c860', trim: '#c8a860', light: 'lamp', warmth: .75, outside: 'day' }, props: ['mannequin', 'ribbons', 'shears', 'motes'] },
    { name: 'The Chart Room', version: 1, skeleton: { shape: 'study', opening: 'harbour-window', display: 'scroll-rack', counter: 'plan-chest' },
      theme: { wall: 'chart', wood: '#3e2a1a', cloth: '#1f4a36', accent: '#c8442a', trim: '#c8a04a', light: 'lamp', warmth: .7, outside: 'dusk' }, props: ['globe', 'charts', 'inkwell'] },
    { name: 'The Menagerie', version: 1, skeleton: { shape: 'barn', opening: 'stable-door', display: 'hutches', counter: 'feed-stall' },
      theme: { wall: 'barnboard', wood: '#6a4a2a', cloth: '#3a5a2a', accent: '#e0b040', trim: '#9a9a94', light: 'lamp', warmth: .85, outside: 'day' }, props: ['birdcages', 'perch', 'haybale', 'critters', 'feathers'] },
    { name: 'The Reliquary', version: 1, skeleton: { shape: 'chapel', opening: 'rose-window', display: 'niches', counter: 'altar' },
      theme: { wall: 'ashlar', wood: '#4a3a2a', cloth: '#7a1a2a', accent: '#e8c050', trim: '#d8b860', light: 'votives', warmth: .8, outside: 'day' }, props: ['censer', 'casket', 'motes'] },
    { name: 'The Trading Post', version: 1, skeleton: { shape: 'cabin', opening: 'frost-window', display: 'gear-wall', counter: 'woodpile' },
      theme: { wall: 'logs', wood: '#6a4a30', cloth: '#8a2a20', accent: '#e8a040', trim: '#8a8a8a', light: 'lamp', warmth: .9, outside: 'night' }, props: ['stove', 'pelts', 'sacks'] }
  );

  // Helpers the later packs draw with too.
  K.n5c = n5c; K.n5column = n5column; K.n5e = n5e; K.n5flame = n5flame; K.n5g = n5g; K.n5light = n5light; K.n5Metals = n5Metals; K.n5p = n5p; K.n5prop = n5prop; K.n5r = n5r; K.n5ridge = n5ridge; K.n5sag = n5sag; K.n5sh = n5sh; K.n5stones = n5stones;
})();
