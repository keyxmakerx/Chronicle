/**
 * shop_room_parts_places.js — more places to keep a shop: a frost
 * hall, a drowned arcade, a caravan tent, a sky barge, a bog hut, a gearworks,
 * a dragon's hoard, a sun court, a haunted parlour and a glasshouse.
 *
 * A parts pack for the counter room (shop_room_counter.js). It loads after
 * that file and the other two packs and before shop_room.js, and adds its
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
  var CM_COLS = K.CM_COLS, CM_OPEN_INFO = K.CM_OPEN_INFO, CM_OPT = K.CM_OPT, CM_ROWS = K.CM_ROWS, CM_STARTERS = K.CM_STARTERS, CM_WALLFQ = K.CM_WALLFQ, CM_WHERE = K.CM_WHERE, COUNTERS = K.COUNTERS, DISPLAYS = K.DISPLAYS, LIGHTS = K.LIGHTS, OPENINGS = K.OPENINGS, PROPS = K.PROPS, SHAPES = K.SHAPES, WALLS = K.WALLS, cmCandle = K.cmCandle, cmGrid = K.cmGrid, cmPost = K.cmPost, cmRainIn = K.cmRainIn, cmShelfBoard = K.cmShelfBoard, cmSky = K.cmSky, cmTop = K.cmTop, cmUnitFrame = K.cmUnitFrame, f1 = K.f1, lin = K.lin, mix = K.mix, rad = K.rad, rng = K.rng, FULL = K.FULL, RING = K.RING, mp3Defs = K.mp3Defs, mpFern = K.mpFern, mpFernClump = K.mpFernClump, mpFlower = K.mpFlower, mpLeaf = K.mpLeaf, mpLump = K.mpLump, mpMossPad = K.mpMossPad, mpOpen = K.mpOpen, mpSlots = K.mpSlots, mpSmooth = K.mpSmooth, mpStars = K.mpStars, mpTaper = K.mpTaper, mpVine = K.mpVine, n5Metals = K.n5Metals, n5c = K.n5c, n5column = K.n5column, n5e = K.n5e, n5flame = K.n5flame, n5g = K.n5g, n5light = K.n5light, n5p = K.n5p, n5prop = K.n5prop, n5r = K.n5r, n5ridge = K.n5ridge, n5sag = K.n5sag, n5sh = K.n5sh, n5stones = K.n5stones;


  /* ================= Places ================= */

  // A wavy line sampled every few units: withies, water, cloth edges.
  function n5wav(x0, x1, y, amp, per, ph, st) {
    var d = '', x;
    for (x = x0; x <= x1; x += (st || 10)) d += (d ? ' L' : 'M') + f1(x) + ',' + f1(y + Math.sin(x / per * 6.283 + (ph || 0)) * amp);
    return d;
  }

  /* ---- The frost hall: a cave in blue ice, the northern lights through a
     break in the wall, ledges of packed snow and a counter cut from one
     block. ---- */
  CM_OPT.shape.push('ice-cave'); CM_OPT.wall.push('glacier'); CM_OPT.opening.push('aurora'); CM_OPT.display.push('ice-ledges'); CM_OPT.counter.push('ice-block');
  CM_WALLFQ.glacier = [.02, 3, 31]; CM_OPEN_INFO.aurora = 'The break in the ice';
  n5light('frostfire', { base: '#cfeaff', glow: '#7fd0ff', amb: '#1c3658', reach: 1050, k: .9 });
  n5prop('snowfall', 'over', 'over'); n5prop('ice-swan', 'tallR', 'front', 0, true);

  function n5fishbone(x, y, k, a) { var s = 'M' + x + ',' + y + ' h' + (60 * k) + ' M' + (x + 60 * k) + ',' + y + ' l' + (14 * k) + ',' + (-10 * k) + ' v' + (20 * k) + ' Z', i; for (i = 1; i < 6; i++) s += ' M' + (x + i * 9 * k) + ',' + (y - (12 - i) * k) + ' V' + (y + (12 - i) * k); return n5p(s, a) + n5e(x - 6 * k, y, 9 * k, 7 * k, a); }
  WALLS.glacier = function (t) {
    var r = rng(161), s, i, x, y, g = '', d, k;
    s = '<defs>' + lin('n5-ice', 1, [[0, '#0e2c4e'], [.45, '#2a7096'], [.8, '#6fbcd6'], [1, '#bfe6f2']]) + '</defs><rect ' + FULL + ' fill="url(#n5-ice)"/>';
    // the ice is in great plates, some catching the light and some not
    for (i = 0; i < 70; i++) { x = -300 + r() * 1800; y = -150 + r() * 620; g += n5p(mpLump(r, x, y, 60 + r() * 150, 6), 'fill="' + (r() < .5 ? '#ffffff' : '#062038') + '" opacity="' + f1(.03 + r() * .09) + '"'); }
    // things the ice closed over long ago
    g += n5g('opacity=".3" filter="url(#src-blur3)"', n5fishbone(1070, 250, 1.4, 'stroke="#04121e" stroke-width="3" fill="none"') + n5fishbone(560, 400, 1, 'stroke="#04121e" stroke-width="3" fill="none"') + n5fishbone(1110, 400, .8, 'stroke="#04121e" stroke-width="2.4" fill="none"'));
    s += '<g filter="url(#cm-wall)">' + s + g + '</g>';
    // cracks that hold the light, and strings of old air
    for (i = 0; i < 22; i++) { x = -260 + r() * 1720; y = -100 + r() * 440; d = 'M' + f1(x) + ',' + f1(y); for (k = 0; k < 5; k++) { x += (r() - .5) * 60; y += 20 + r() * 40; d += ' L' + f1(x) + ',' + f1(y); } s += n5p(d, 'stroke="#dff6ff" stroke-opacity=".5" stroke-width="5" fill="none" filter="url(#src-blur3)"') + n5p(d, 'stroke="#fff" stroke-opacity=".7" stroke-width="1" fill="none"'); }
    for (i = 0; i < 90; i++) { x = -280 + r() * 1760; y = -120 + r() * 580; s += n5c(x, y, 1 + r() * 2.6, 'fill="none" stroke="#fff" stroke-opacity="' + f1(.2 + r() * .4) + '" stroke-width=".8"'); }
    return s;
  };
  function n5icicles(r, x0, x1, y, lo, hi, gap) {
    var s = '', x, L, w;
    for (x = x0; x < x1; x += w * (.7 + r() * gap)) { w = 8 + r() * 16; L = lo + Math.pow(r(), 2) * (hi - lo); if (x > 500 && x < 720) L = Math.min(L, lo + 26); s += n5p('M' + f1(x - w / 2) + ',' + y + ' Q' + f1(x - w * .2) + ',' + f1(y + L * .5) + ' ' + f1(x) + ',' + f1(y + L) + ' Q' + f1(x + w * .2) + ',' + f1(y + L * .5) + ' ' + f1(x + w / 2) + ',' + y + ' Z', 'fill="url(#n5-icic)"') + n5p('M' + f1(x - w * .2) + ',' + (y + 2) + ' Q' + f1(x - w * .1) + ',' + f1(y + L * .5) + ' ' + f1(x) + ',' + f1(y + L - 3), 'stroke="#fff" stroke-opacity=".7" stroke-width="1" fill="none"'); }
    return s;
  }
  SHAPES['ice-cave'] = function (t) {
    var s = WALLS[t.th.wall](t), r = rng(163), d, x;
    s += '<defs>' + lin('n5-icic', 1, [[0, '#f4fcff', .95], [.5, '#a8dcf0', .8], [1, '#6ab8dc', .35]]) + lin('n5-pill', 0, [[0, '#0c2a48', .9], [.3, '#bfe8f8', .7], [.5, '#5aa6c8', .6], [1, '#06182c', .95]]) + '</defs>';
    // the roof: packed snow with a ragged under-edge, and the icicles under it
    d = 'M-300,-150 H1500 V20'; for (x = 1500; x > -300; x -= 40 + r() * 50) d += ' Q' + f1(x - 20) + ',' + f1(34 + r() * 22) + ' ' + f1(x - 44) + ',' + f1(16 + r() * 14); d += ' L-300,20 Z';
    s += n5sh(d, 0, 12, .45, 8) + n5p(d, 'fill="#e6f2fa"') + n5p(d, 'fill="none" stroke="#8ab8d4" stroke-opacity=".6" stroke-width="4" transform="translate(0,-3)"') + n5icicles(r, -290, 1500, 30, 30, 150, 1.6);
    // a pillar of clear ice at each side
    [[-30, 70], [1150, 80]].forEach(function (q) { var p = 'M' + q[0] + ',-150 q' + (q[1] * .2) + ',300 0,640 h' + q[1] + ' q' + (-q[1] * .2) + ',-340 0,-640 Z'; s += n5sh(p, 10, 0, .4, 8) + n5p(p, 'fill="url(#n5-pill)"') + n5p('M' + (q[0] + q[1] * .3) + ',-100 q8,280 -4,560', 'stroke="#fff" stroke-opacity=".5" stroke-width="3" fill="none"'); });
    return s;
  };
  OPENINGS.aurora = function (t) {
    var d = 'M812,300 L798,224 L826,152 L810,98 L868,70 L930,86 L992,64 L1026,132 L1008,204 L1030,268 L988,312 L900,298 Z', r = rng(165), s, fr, i, x;
    s = '<defs>' + lin('n5-nsky', 1, [[0, '#040a1e'], [.7, '#0e2244'], [1, '#274a70']]) + lin('n5-aur1', 1, [[0, '#6affc0', 0], [.55, '#6affc0', .35], [.86, '#9affd0', .85], [1, '#9affd0', 0]]) + lin('n5-aur2', 1, [[0, '#b08aff', 0], [.5, '#b08aff', .4], [.85, '#e0a0ff', .8], [1, '#5ae0b0', 0]]) + '</defs>';
    s += n5r(790, 56, 250, 270, 'fill="url(#n5-nsky)"') + mpStars(r, 60, 794, 60, 240, 180, true);
    // the lights: three curtains that lean and brighten in their own time
    [[92, 'n5-aur1', 0, 9, 16, 150], [128, 'n5-aur2', 3, 13, 12, 110], [70, 'n5-aur1', 5, 11, 10, 190]].forEach(function (q, j) {
      var top = [], x, p;
      for (x = 780; x <= 1050; x += 10) top.push([x, q[0] + Math.sin(x / q[5] * 6.283 + j * 2) * q[4] + (x - 780) * (j === 1 ? -.12 : .1)]);
      p = 'M' + top.map(function (v) { return v[0] + ',' + f1(v[1]); }).join(' L') + ' L' + top.slice().reverse().map(function (v) { return v[0] + ',' + f1(v[1] + 84); }).join(' L') + ' Z';
      s += n5g('class="n5-fbb n5-wave" style="animation-duration:' + q[3] + 's;animation-delay:-' + q[2] + 's"', n5g('class="n5-shimmer" style="animation-duration:' + f1(q[3] * .7) + 's"', n5p(p, 'fill="url(#' + q[1] + ')" filter="url(#src-blur3)"')));
    });
    // far peaks, and snow to the edge of the break
    s += n5p(n5ridge(r, 790, 1040, 262, 44, 28, 330), 'fill="#1c3a5c"') + n5p(n5ridge(r, 790, 1040, 284, 20, 22, 330), 'fill="#9cc4dc"') + n5p('M790,300 q60,-14 120,-4 t130,-6 V330 H790 Z', 'fill="#e4f2fa"');
    fr = n5p(d, 'fill="none" stroke="#0a2440" stroke-opacity=".7" stroke-width="16" stroke-linejoin="round"') + n5p(d, 'fill="none" stroke="#cfeeff" stroke-opacity=".9" stroke-width="7" stroke-linejoin="round"') + n5p(d, 'fill="none" stroke="#fff" stroke-width="1.6" stroke-linejoin="round" transform="translate(-2,-2)"');
    fr += '<defs>' + lin('n5-icic2', 1, [[0, '#f4fcff', .95], [1, '#6ab8dc', .3]]) + '</defs>';
    [[830, 96, 30], [852, 82, 44], [886, 76, 26], [912, 84, 50], [950, 80, 34], [976, 72, 22], [1004, 92, 40]].forEach(function (q) { fr += n5p('M' + (q[0] - 6) + ',' + q[1] + ' L' + q[0] + ',' + (q[1] + q[2]) + ' L' + (q[0] + 6) + ',' + q[1] + ' Z', 'fill="url(#n5-icic2)"'); });
    return mpOpen(d, s, fr);
  };
  DISPLAYS['ice-ledges'] = function (t) {
    var r = rng(167), s, d, x, i, st = '';
    s = '<defs>' + mp3Defs() + lin('n5-icic', 1, [[0, '#f4fcff', .95], [.5, '#a8dcf0', .8], [1, '#6ab8dc', .35]]) + '<clipPath id="n5-icew"><path d="M30,458 V96 Q30,70 60,68 H464 Q494,70 494,96 V458 Z"/></clipPath></defs>';
    // a wall of cut blocks, clear at the heart and white at the joints
    s += n5p('M30,458 V96 Q30,70 60,68 H464 Q494,70 494,96 V458 Z', 'fill="#000" opacity=".5" filter="url(#src-blur8)" transform="translate(8,6)"') + '<g clip-path="url(#n5-icew)"><g filter="url(#mp-stone)">' + n5r(28, 60, 470, 400, 'fill="#dff2fa"') + n5stones(r, 28, 62, 496, 460, 57, 96, 50, '#6fb4d2', '#2c6c94', '#c8ecf8', .5) + '</g>';
    for (i = 0; i < 26; i++) s += n5p('M' + f1(40 + r() * 440) + ',' + f1(80 + r() * 360) + ' l' + f1((r() - .5) * 50) + ',' + f1(20 + r() * 40), 'stroke="#fff" stroke-opacity=".5" stroke-width="1"');
    s += n5r(28, 60, 470, 400, 'fill="#06203a" opacity=".3"') + '</g>';
    // a drift of snow along each course, little icicles under its lip
    CM_ROWS.forEach(function (ry) {
      d = 'M32,' + (ry + 12); for (x = 32; x < 492; x += 30 + r() * 30) d += ' L' + f1(x) + ',' + (ry + 4) + ' Q' + f1(x + 14) + ',' + f1(ry - 6 - r() * 6) + ' ' + f1(Math.min(492, x + 30)) + ',' + (ry + 2); d += ' L492,' + (ry + 12) + ' Z';
      st += n5r(32, ry + 10, 460, 16, 'fill="#04182c" opacity=".55" filter="url(#src-blur5)"') + n5icicles(r, 38, 488, ry + 10, 6, 30, 3) + n5p(d, 'fill="#f2f9fd"') + n5p('M32,' + (ry + 11) + ' H492', 'stroke="#8ab8d4" stroke-width="3" stroke-opacity=".7"');
    });
    return { art: '<g filter="url(#cm-unitlit)">' + s + st + '</g>', slots: cmGrid(), zone: [34, 96, 454, 320], wallsh: false, decor: true, label: 'The ice ledges' };
  };
  COUNTERS['ice-block'] = function (t) {
    var r = rng(169), s, g = '', i, x, y, d, k;
    s = '<defs>' + mp3Defs() + lin('n5-ibt', 1, [[0, '#f0fafe'], [1, '#a4d4e8']]) + lin('n5-ibf', 1, [[0, '#7cc0dc'], [.3, '#2f7ca4'], [1, '#061c30']]) + lin('n5-icic', 1, [[0, '#f4fcff', .95], [.5, '#a8dcf0', .8], [1, '#6ab8dc', .35]]) + '</defs><clipPath id="cm-topclip"><rect x="-300" y="452" width="1800" height="58"/></clipPath>';
    s += '<g filter="url(#mp-stone)">' + n5r(-300, 452, 1800, 58, 'fill="url(#n5-ibt)"') + n5r(-300, 510, 1800, 390, 'fill="url(#n5-ibf)"') + '</g>';
    // light going down through it, old air, and what it froze round
    for (i = 0; i < 40; i++) { x = -300 + r() * 1800; g += n5r(x, 512, 3 + r() * 16, 380, 'fill="#fff" opacity="' + f1(.02 + r() * .07) + '"'); }
    g += n5g('opacity=".4" filter="url(#src-blur3)"', n5fishbone(180, 610, 1.6, 'stroke="#031220" stroke-width="3.4" fill="none"') + n5fishbone(840, 640, 1.2, 'stroke="#031220" stroke-width="3" fill="none"') + n5fishbone(1060, 590, .9, 'stroke="#031220" stroke-width="2.6" fill="none"'));
    for (i = 0; i < 9; i++) { x = -200 + r() * 1600; y = 520; d = 'M' + f1(x) + ',' + y; for (k = 0; k < 5; k++) { x += (r() - .5) * 70; y += 20 + r() * 34; d += ' L' + f1(x) + ',' + f1(y); } g += n5p(d, 'stroke="#dff6ff" stroke-opacity=".5" stroke-width="5" fill="none" filter="url(#src-blur3)"') + n5p(d, 'stroke="#fff" stroke-opacity=".8" stroke-width="1" fill="none"'); }
    for (i = 0; i < 60; i++) g += n5c(-300 + r() * 1800, 520 + r() * 180, 1 + r() * 3, 'fill="none" stroke="#fff" stroke-opacity="' + f1(.25 + r() * .4) + '" stroke-width=".9"');
    // snow lying over the front edge and drifted along the foot
    d = 'M-300,500'; for (x = -300; x < 1500; x += 40 + r() * 50) d += ' Q' + f1(x + 20) + ',' + f1(516 + r() * 14) + ' ' + f1(x + 46) + ',' + f1(504 + r() * 4); d += ' L1500,498 L-300,498 Z';
    g += n5p(d, 'fill="#04182c" opacity=".5" filter="url(#src-blur5)" transform="translate(0,8)"') + n5icicles(r, -290, 1500, 508, 6, 34, 4) + n5p(d, 'fill="#f6fbfe"') + n5p('M-300,700 q150,-34 300,-12 t320,-10 t300,8 t300,-14 t280,10 V900 H-300 Z', 'fill="#eaf4fa"') + n5p('M-300,700 q150,-34 300,-12 t320,-10 t300,8 t300,-14 t280,10', 'stroke="#8ab8d4" stroke-opacity=".5" stroke-width="3" fill="none" transform="translate(0,6)"');
    return s + g + n5r(-300, 506, 1800, 3, 'fill="' + t.lc + '" opacity=".4"');
  };
  LIGHTS.frostfire = function (t) {
    var x = t.lp.x, s = '<g data-k="lamp">' + n5e(x, 482, 40, 7, 'fill="url(#src-ao)"'), i;
    // a cold flame held in a cage of black iron
    s += n5p('M' + (x - 24) + ',480 h48 l-6,-8 h-36 Z', 'fill="#17181b"') + n5r(x - 20, 392, 40, 5, 'rx="2" fill="#17181b"') + n5p('M' + (x - 16) + ',392 q16,-26 32,0', 'stroke="#17181b" stroke-width="3.4" fill="none"') + n5c(x, 374, 5, 'fill="none" stroke="#17181b" stroke-width="3"');
    s += n5c(x, 440, 46, 'fill="url(#cm-glow)" opacity=".8" class="cm-pulse"') + n5flame(x - 8, 472, 44, 1, '#3aa8e8', .8) + n5flame(x + 8, 472, 52, 2, '#3aa8e8', .8) + n5flame(x, 472, 62, 0, '#9fe0ff', .9) + n5flame(x, 472, 38, 3, '#ffffff', .95);
    for (i = -1; i <= 1; i++) s += n5r(x + i * 17 - 1.4, 396, 2.8, 76, 'fill="#17181b"') + n5r(x + i * 17 - 1.4, 396, 1, 76, 'fill="#9ab0c0" opacity=".5"');
    for (i = 0; i < 5; i++) s += n5c(x - 12 + i * 6, 430, 1.2, 'class="cm-spark" fill="#dff6ff" style="--dx:' + ((i - 2) * 12) + 'px;animation-delay:-' + (i * .7) + 's"');
    return { back: '', front: s + '<circle cx="' + x + '" cy="430" r="120" fill="url(#cm-bloom)" class="cm-pulse" opacity=".8"/></g>' };
  };
  PROPS.snowfall = function (t, p) {
    var s = '', r = rng(171), i;
    for (i = 0; i < 60; i++) s += n5c(40 + r() * 1300, -20 + r() * 60, 1 + r() * 2.2, 'class="n5-snow" fill="#fff" opacity="' + f1(.4 + r() * .5) + '" style="--fx:' + f1(-40 - r() * 120) + 'px;--fy:' + f1(520 + r() * 200) + 'px;animation-duration:' + f1(9 + r() * 10) + 's;animation-delay:-' + f1(r() * 19) + 's"');
    return n5g('class="src-nohit"', s);
  };
  PROPS['ice-swan'] = function (t, p) {
    var x = p.x, s, d, i;
    s = '<defs>' + lin('n5-swan', 0, [[0, '#2c6c94', .9], [.3, '#e8f8ff', .95], [.55, '#8accec', .85], [1, '#123a5c', .95]]) + '</defs>' + n5e(x + 4, 484, 60, 8, 'fill="url(#src-ao)"');
    // carved from one clear block: the body, the folded wings, the long neck
    s += n5p('M' + (x - 50) + ',480 l8,-22 h84 l8,22 Z', 'fill="url(#n5-swan)"') + n5p('M' + (x - 42) + ',458 h84', 'stroke="#fff" stroke-opacity=".8" stroke-width="1.6"');
    d = 'M' + (x - 44) + ',458 Q' + (x - 56) + ',410 ' + (x - 10) + ',396 Q' + (x + 30) + ',388 ' + (x + 52) + ',356 Q' + (x + 50) + ',420 ' + (x + 36) + ',458 Z';
    s += n5p(d, 'fill="url(#n5-swan)"') + n5p('M' + (x - 30) + ',440 Q' + (x + 4) + ',400 ' + (x + 46) + ',372 M' + (x - 22) + ',452 Q' + (x + 10) + ',420 ' + (x + 44) + ',396 M' + (x - 6) + ',456 Q' + (x + 18) + ',436 ' + (x + 40) + ',420', 'stroke="#fff" stroke-opacity=".6" stroke-width="1.6" fill="none"');
    d = 'M' + (x - 36) + ',420 Q' + (x - 62) + ',380 ' + (x - 52) + ',330 Q' + (x - 44) + ',290 ' + (x - 20) + ',290 Q' + (x - 4) + ',292 ' + (x - 2) + ',306 L' + (x + 14) + ',314 L' + (x - 6) + ',318 Q' + (x - 20) + ',308 ' + (x - 32) + ',322 Q' + (x - 44) + ',350 ' + (x - 18) + ',400 Z';
    s += n5p(d, 'fill="url(#n5-swan)"') + n5p('M' + (x - 48) + ',336 Q' + (x - 42) + ',300 ' + (x - 22) + ',296', 'stroke="#fff" stroke-opacity=".9" stroke-width="2" fill="none"') + n5c(x - 14, 302, 1.8, 'fill="#0a2440"');
    [[-40, 310, 0], [30, 380, 1.8], [-2, 440, 3.1], [44, 362, 4.2]].forEach(function (q) { s += n5p('M' + (x + q[0]) + ',' + (q[1] - 7) + ' l2,5 l5,2 l-5,2 l-2,5 l-2,-5 l-5,-2 l5,-2 Z', 'class="n5-fb n5-glint" fill="#fff" style="animation-delay:-' + q[2] + 's"'); });
    return n5g('class="src-nohit"', s);
  };

  /* ---- The drowned arcade: a ruin on the sea bed. Light comes down in
     shafts, a broken arch opens on the deep, shells hold the goods and the
     counter is the rail of a wreck. ---- */
  CM_OPT.shape.push('reef'); CM_OPT.wall.push('deepwater'); CM_OPT.opening.push('sea-arch'); CM_OPT.display.push('shells'); CM_OPT.counter.push('wreck');
  CM_OPEN_INFO['sea-arch'] = 'The broken arch';
  n5light('jellyfish', { base: '#b8f4e8', glow: '#5fe8d0', amb: '#0c2c40', reach: 1050, k: .9 });
  n5prop('fish', 'air', 'back'); n5prop('kelp', 'air', 'back'); n5prop('caustics', 'over', 'over'); n5prop('crab', 'cL', 'front', 110);

  function n5coral(r, x, y, len, a, dep, col, w) {
    var x1 = x + Math.cos(a) * len, y1 = y + Math.sin(a) * len, s = n5p('M' + f1(x) + ',' + f1(y) + ' L' + f1(x1) + ',' + f1(y1), 'stroke="' + col + '" stroke-width="' + f1(w) + '" stroke-linecap="round"');
    if (dep > 0) { s += n5coral(r, x1, y1, len * (.62 + r() * .2), a - .35 - r() * .4, dep - 1, col, w * .72) + n5coral(r, x1, y1, len * (.62 + r() * .2), a + .35 + r() * .4, dep - 1, col, w * .72); if (r() < .4) s += n5coral(r, x1, y1, len * .5, a + (r() - .5) * .3, dep - 1, col, w * .7); }
    else s += n5c(x1, y1, w * .9, 'fill="' + mix(col, '#ffffff', .4) + '"');
    return s;
  }
  function n5fan(r, x, y, R, col) {
    var s = '', i, a;
    for (i = 0; i <= 12; i++) { a = -Math.PI * (.12 + i * .063); s += n5p('M' + x + ',' + y + ' Q' + f1(x + Math.cos(a) * R * .5 + (r() - .5) * 8) + ',' + f1(y + Math.sin(a) * R * .5) + ' ' + f1(x + Math.cos(a) * R) + ',' + f1(y + Math.sin(a) * R), 'stroke="' + col + '" stroke-width="2" fill="none"'); }
    [.4, .6, .8, 1].forEach(function (k) { s += n5p('M' + f1(x + Math.cos(-Math.PI * .12) * R * k) + ',' + f1(y + Math.sin(-Math.PI * .12) * R * k) + ' A' + f1(R * k) + ',' + f1(R * k) + ' 0 0 0 ' + f1(x + Math.cos(-Math.PI * .876) * R * k) + ',' + f1(y + Math.sin(-Math.PI * .876) * R * k), 'stroke="' + col + '" stroke-opacity=".7" stroke-width="1.2" fill="none"'); });
    return s;
  }
  WALLS.deepwater = function (t) {
    var r = rng(181), s, i, x, w, h, o, c;
    s = '<defs>' + lin('n5-deep', 1, [[0, '#3a9ab0'], [.3, '#176a86'], [.7, '#0a3a54'], [1, '#04161f']]) + '</defs><rect ' + FULL + ' fill="url(#n5-deep)"/>';
    // what is left of the arcade, each row of pillars fainter than the last
    [[.22, 30, 300, '#2e8298'], [.4, 44, 360, '#1c6078'], [.7, 62, 440, '#0e3e54']].forEach(function (q, j) {
      for (x = -280 + j * 60; x < 1500; x += 170 + j * 40 + r() * 60) {
        w = q[1] * (.8 + r() * .4); h = q[2] * (.5 + r() * .5); c = q[3];
        s += n5g('opacity="' + q[0] + '"' + (j < 2 ? ' filter="url(#src-blur3)"' : ''), n5p('M' + f1(x) + ',470 V' + f1(470 - h) + ' l' + f1(w * .3) + ',-12 l' + f1(w * .2) + ',8 l' + f1(w * .5) + ',-14 V470 Z', 'fill="' + c + '"') + n5r(x - w * .15, 470 - h * .08, w * 1.3, h * .08, 'fill="' + c + '"') + [.25, .5, .75].map(function (k) { return n5r(x + w * k, 470 - h + 10, 2, h - 20, 'fill="#031018" opacity=".5"'); }).join('') + (r() < .5 ? n5p('M' + f1(x) + ',' + f1(470 - h) + ' q' + f1(-70) + ',-60 -150,-20', 'stroke="' + c + '" stroke-width="' + f1(w * .5) + '" fill="none"') : ''));
      }
    });
    // the day, a long way up, comes down in shafts
    for (i = 0; i < 9; i++) { x = -200 + i * 190 + r() * 80; w = 30 + r() * 70; s += n5p('M' + f1(x) + ',-150 h' + f1(w) + ' L' + f1(x + w - 220) + ',470 h' + f1(-w * 2.4) + ' Z', 'fill="#d8fff4" opacity="' + f1(.05 + r() * .07) + '" filter="url(#src-blur8)"'); }
    // sand, rippled, and what drifts in the water
    s += n5p('M-300,470 V436 q150,-26 300,-8 t300,-10 t300,12 t300,-16 t300,6 V470 Z', 'fill="#1e5a66"') + n5p('M-300,470 V448 q150,-16 300,-4 t300,-8 t300,8 t300,-10 t300,4 V470 Z', 'fill="#2a7078"');
    for (i = 0; i < 140; i++) s += n5c(-300 + r() * 1800, -150 + r() * 620, .6 + r() * 1.4, 'fill="#e8fff8" opacity="' + f1(.1 + r() * .3) + '"');
    return s;
  };
  SHAPES.reef = function (t) {
    var s = WALLS[t.th.wall](t), r = rng(183), i, x, d;
    s += '<defs>' + mp3Defs() + '</defs>';
    // the surface far overhead, and a fallen lintel across the top
    for (i = 0; i < 26; i++) s += n5p(n5wav(-300 + r() * 1500, -300 + r() * 1500 + 160 + r() * 200, -20 + r() * 44, 4, 60 + r() * 60, r() * 6), 'stroke="#e8fff8" stroke-opacity="' + f1(.12 + r() * .2) + '" stroke-width="' + f1(1 + r() * 2) + '" fill="none"');
    d = 'M-300,-150 H1500 V18 L1380,30 L1250,14 L1100,34 L1010,22 L860,40 L700,20 L560,36 L400,16 L240,34 L60,20 L-120,36 L-300,22 Z';
    s += n5sh(d, 0, 12, .5, 8) + n5g('filter="url(#mp-stone)"', n5p(d, 'fill="#27474e"')) + n5p(d, 'fill="none" stroke="#7fd0c8" stroke-opacity=".3" stroke-width="2"');
    for (i = 0; i < 30; i++) { x = -280 + r() * 1760; s += n5p('M' + f1(x) + ',' + f1(20 + r() * 14) + ' q' + f1((r() - .5) * 14) + ',' + f1(10 + r() * 30) + ' ' + f1((r() - .5) * 10) + ',' + f1(24 + r() * 50), 'stroke="' + mix('#2a7a4a', '#7ab84a', r()) + '" stroke-width="' + f1(1.6 + r() * 2.4) + '" fill="none" stroke-linecap="round"'); }
    // rock at each side with coral on it: branching, fanned, brain
    [[-40, 1], [1240, -1]].forEach(function (q) {
      var bx = q[0], k = q[1], rock = 'M' + bx + ',-150 q' + (k * 60) + ',160 ' + (k * 30) + ',300 q' + (k * 70) + ',60 ' + (k * 50) + ',170 q' + (k * 60) + ',70 ' + (k * 130) + ',160 H' + (bx - k * 300) + ' V-150 Z';
      s += n5sh(rock, k * 12, 0, .5, 8) + n5g('filter="url(#mp-stone)"', n5p(rock, 'fill="#1c3a42"'));
      s += n5coral(r, bx + k * 44, 320, 34, -1.57 - k * .2, 4, '#e8686a', 7) + n5coral(r, bx + k * 76, 470, 30, -1.57 + k * .1, 4, '#f0a04a', 6) + n5fan(r, bx + k * 30, 200, 60, '#c86ad0') + n5fan(r, bx + k * 96, 474, 44, '#f0d060');
      s += n5e(bx + k * 20, 398, 30, 20, 'fill="#d87a9a"') + [0, 1, 2, 3].map(function (j) { return n5p(n5wav(bx + k * 20 - 26, bx + k * 20 + 26, 386 + j * 8, 3, 14, j), 'stroke="#8a3a5a" stroke-width="1.6" fill="none"'); }).join('');
    });
    return s;
  };
  OPENINGS['sea-arch'] = function (t) {
    var d = 'M818,452 V214 Q818,84 912,70 Q1006,84 1006,214 V452 Z', r = rng(185), s, fr, i, a, a2, fish = '', st = '';
    s = '<defs>' + lin('n5-far', 1, [[0, '#7fe0e0'], [.5, '#2a8aa8'], [1, '#0a3450']]) + mp3Defs() + '<clipPath id="n5-arch"><path clip-rule="evenodd" d="M776,452 V60 H1048 V452 Z ' + d + '"/></clipPath></defs>' + n5r(800, 60, 230, 400, 'fill="url(#n5-far)"');
    for (i = 0; i < 4; i++) s += n5p('M' + (840 + i * 44) + ',60 h26 l-70,400 h-50 Z', 'fill="#e8fff8" opacity=".1" filter="url(#src-blur5)"');
    // something very large goes by, far out
    s += n5g('class="n5-swim" style="--sx:-520px;animation-duration:64s"', n5g('opacity=".4" filter="url(#src-blur3)"', n5p('M1100,250 q60,-34 150,-20 q50,8 90,34 l40,-22 l-8,34 l12,26 l-44,-14 q-60,22 -130,16 q-70,-6 -110,-54 Z', 'fill="#0a3450"') + n5p('M1190,270 q10,30 -14,44', 'stroke="#0a3450" stroke-width="12" fill="none" stroke-linecap="round"')));
    // a shoal turns through the arch
    for (i = 0; i < 16; i++) { var fx = 1060 + r() * 150, fy = 150 + r() * 120; fish += n5p('M' + f1(fx) + ',' + f1(fy) + ' q8,-6 18,0 l7,-5 v10 l-7,-5 q-10,6 -18,0 Z', 'fill="' + (r() < .5 ? '#d8f4f0' : '#f0c060') + '" opacity=".85"'); }
    s += n5g('class="n5-swim" style="--sx:-460px;animation-duration:22s"', n5g('class="n5-fb n5-bob"', fish));
    s += n5p('M800,452 V420 q60,-20 120,-6 t110,-8 V452 Z', 'fill="#12404e"') + n5coral(r, 846, 440, 22, -1.5, 3, '#0e3440', 5) + n5coral(r, 984, 446, 26, -1.6, 3, '#0e3440', 5);
    for (i = 0; i < 7; i++) s += n5c(830 + r() * 160, 420, 1.6 + r() * 2.4, 'class="n5-streak" fill="none" stroke="#e8fff8" stroke-width="1" style="--fy:-' + f1(200 + r() * 140) + 'px;animation-duration:' + f1(5 + r() * 5) + 's;animation-delay:-' + f1(r() * 8) + 's"');
    // the arch: worn blocks, weed, a crust of small shells
    for (i = 0; i < 36; i++) st += n5r(776 + (i % 2) * 230, 60 + Math.floor(i / 2) * 22 - (i % 3) * 3, 42, 21, 'rx="3" fill="' + mix('#4a7078', r() < .5 ? '#1e3a42' : '#8ab0b0', r() * .4) + '"');
    for (i = 0; i < 9; i++) { a = Math.PI + i * Math.PI / 9; a2 = a + Math.PI / 9; st += n5p('M' + f1(912 + 94 * Math.cos(a)) + ',' + f1(214 + 140 * Math.sin(a)) + ' L' + f1(912 + 136 * Math.cos(a)) + ',' + f1(214 + 168 * Math.sin(a)) + ' L' + f1(912 + 136 * Math.cos(a2)) + ',' + f1(214 + 168 * Math.sin(a2)) + ' L' + f1(912 + 94 * Math.cos(a2)) + ',' + f1(214 + 140 * Math.sin(a2)) + ' Z', 'fill="' + mix('#4a7078', r() < .5 ? '#1e3a42' : '#8ab0b0', r() * .4) + '" stroke="#0a2028" stroke-width="2"'); }
    fr = n5sh('M776,452 V60 H1048 V452 Z', -8, 4, .4, 8) + '<g clip-path="url(#n5-arch)">' + n5r(776, 40, 272, 420, 'fill="#12303a"') + n5g('filter="url(#mp-stone)"', st) + '</g>' + n5p(d, 'fill="none" stroke="#000" stroke-opacity=".6" stroke-width="3"') + n5p(d, 'fill="none" stroke="#9fe8e0" stroke-opacity=".35" stroke-width="1.4" transform="translate(-2,0)"');
    for (i = 0; i < 12; i++) fr += n5p('M' + f1(780 + r() * 262) + ',' + f1(60 + r() * 60) + ' q' + f1((r() - .5) * 12) + ',' + f1(14 + r() * 30) + ' ' + f1((r() - .5) * 8) + ',' + f1(30 + r() * 50), 'stroke="' + mix('#2a7a4a', '#7ab84a', r()) + '" stroke-width="' + f1(1.6 + r() * 2) + '" fill="none" stroke-linecap="round"');
    for (i = 0; i < 40; i++) { var bx = r() < .5 ? 780 + r() * 36 : 1008 + r() * 36; fr += n5c(bx, 80 + r() * 366, 1.6 + r() * 2.6, 'fill="#c8d8d0" opacity=".7"'); }
    return mpOpen(d, s, fr);
  };
  DISPLAYS.shells = function (t) {
    var r = rng(187), P = [[88, 176], [198, 154], [318, 182], [430, 160], [72, 294], [184, 306], [302, 282], [420, 300], [106, 412], [216, 398], [330, 414], [440, 400]], s, i, k;
    s = '<defs>' + mp3Defs() + rad('n5-nacre', [[0, '#fff8f4'], [.5, '#f0c4d0'], [1, '#8e74a8']], ' cx=".5" cy="1" r="1"') + lin('n5-lip', 1, [[0, '#e8c4b4'], [1, '#7a5a5a']]) + '</defs>';
    // a spur of reef the shells have grown on
    s += n5g('filter="url(#mp-stone)"', n5p('M10,470 Q0,300 50,210 Q40,120 120,100 Q200,60 300,110 Q420,80 470,170 Q520,260 480,360 Q510,430 500,470 Z', 'fill="#173840"')) + n5p('M10,470 Q0,300 50,210 Q40,120 120,100 Q200,60 300,110 Q420,80 470,170', 'stroke="#7fd0c8" stroke-opacity=".3" stroke-width="2.4" fill="none"');
    s += n5coral(r, 30, 250, 26, -1.7, 3, '#e8686a', 5) + n5coral(r, 486, 300, 24, -1.4, 3, '#f0a04a', 5) + n5fan(r, 258, 120, 40, '#c86ad0');
    P.forEach(function (p, j) {
      var x = p[0], y = p[1] + 2, R = 50, d = 'M' + (x - 40) + ',' + (y - 8), a;
      // the upper valve stands open behind, ribbed and scalloped; the lower one is the dish
      for (k = 0; k <= 8; k++) { a = Math.PI + k * Math.PI / 8; d += ' Q' + f1(x + Math.cos(a + Math.PI / 16) * (R + 9)) + ',' + f1(y - 8 + Math.sin(a + Math.PI / 16) * (R + 9) * 1.1) + ' ' + f1(x + Math.cos(a + Math.PI / 8) * R * (k === 8 ? .8 : 1)) + ',' + f1(y - 8 + Math.sin(a + Math.PI / 8) * R * 1.1); }
      d = 'M' + (x - R) + ',' + (y - 8); for (k = 0; k < 8; k++) { a = Math.PI + (k + .5) * Math.PI / 8; var a1 = Math.PI + (k + 1) * Math.PI / 8; d += ' Q' + f1(x + Math.cos(a) * (R + 10)) + ',' + f1(y - 8 + Math.sin(a) * (R + 10) * 1.12) + ' ' + f1(x + Math.cos(a1) * R) + ',' + f1(y - 8 + Math.sin(a1) * R * 1.12); } d += ' Z';
      s += n5sh(d, 6, 6, .5, 5) + n5p(d, 'fill="url(#n5-nacre)"');
      for (k = 1; k < 8; k++) { a = Math.PI + k * Math.PI / 8; s += n5p('M' + x + ',' + (y - 4) + ' L' + f1(x + Math.cos(a) * R) + ',' + f1(y - 8 + Math.sin(a) * R * 1.12), 'stroke="#a8809c" stroke-opacity=".5" stroke-width="1.4"'); }
      s += n5p(d, 'fill="none" stroke="#fff" stroke-opacity=".6" stroke-width="1.4"') + n5e(x, y + 10, 46, 9, 'fill="#000" opacity=".45" filter="url(#src-blur3)"') + n5p('M' + (x - 52) + ',' + (y - 4) + ' Q' + x + ',' + (y + 24) + ' ' + (x + 52) + ',' + (y - 4) + ' Q' + x + ',' + (y + 6) + ' ' + (x - 52) + ',' + (y - 4) + ' Z', 'fill="url(#n5-lip)"') + n5p('M' + (x - 52) + ',' + (y - 4) + ' Q' + x + ',' + (y + 6) + ' ' + (x + 52) + ',' + (y - 4), 'stroke="#fff4ec" stroke-opacity=".8" stroke-width="1.6" fill="none"');
      for (k = -3; k <= 3; k++) s += n5p('M' + f1(x + k * 13) + ',' + f1(y + 2 - Math.abs(k) * .6) + ' q' + f1(-k * 2) + ',7 ' + f1(-k * 4) + ',' + f1(12 - Math.abs(k) * 2), 'stroke="#5a3a3e" stroke-opacity=".5" stroke-width="1"');
      if (j % 4 === 1) s += n5c(x + 34, y - 6, 4, 'fill="#fff8f4"') + n5c(x + 33, y - 7, 1.4, 'fill="#fff"');
    });
    return { art: s, slots: mpSlots(P, 1.12), zone: [34, 96, 454, 320], wallsh: false, decor: true, label: 'The shell beds' };
  };
  COUNTERS.wreck = function (t) {
    var s = cmTop(t, 'M-300,452 H1500 V512 H-300 Z'), r = rng(189), g = '', y, x, i;
    s += '<clipPath id="cm-topclip"><rect x="-300" y="452" width="1800" height="60"/></clipPath>' + n5r(-300, 452, 1800, 60, 'fill="#0c3a3a" opacity=".45"') + n5p('M-300,506 H1500 V520 H-300 Z', 'fill="#000" filter="url(#cm-beam)"') + n5r(-300, 506, 1800, 2.4, 'fill="' + t.lc + '" opacity=".45"');
    // the hull's strakes, each lapped over the one below, gone green
    for (y = 520; y < 900; y += 46) s += n5r(-300, y, 1800, 46, 'fill="#000" filter="url(#cm-front)"') + n5r(-300, y, 1800, 46, 'fill="' + (r() < .5 ? '#000' : '#7fd0c8') + '" opacity="' + f1(r() * .1) + '"') + n5r(-300, y + 40, 1800, 6, 'fill="#000" opacity=".6"') + n5r(-300, y, 1800, 1.6, 'fill="' + t.lc + '" opacity=".25"');
    s += n5r(-300, 520, 1800, 380, 'fill="#0c4a44" opacity=".4"');
    for (i = 0; i < 26; i++) { x = -280 + r() * 1760; g += n5p('M' + f1(x) + ',520 q' + f1((r() - .5) * 20) + ',' + f1(40 + r() * 80) + ' ' + f1((r() - .5) * 16) + ',' + f1(70 + r() * 110), 'stroke="' + mix('#1e6a40', '#6aa838', r()) + '" stroke-opacity=".7" stroke-width="' + f1(2 + r() * 5) + '" fill="none" stroke-linecap="round"'); }
    // a port with its glass still in, barnacles, and sand drifted to the keel with a star on it
    [170, 1030].forEach(function (px) { g += n5c(px + 4, 606, 36, 'fill="#000" opacity=".5" filter="url(#src-blur5)"') + n5c(px, 600, 34, 'fill="url(#cm-trim)"') + n5c(px, 600, 25, 'fill="#06181e"') + n5c(px, 600, 25, 'fill="#5fe8d0" opacity=".12"') + n5p('M' + (px - 16) + ',' + 588 + ' q10,-10 22,-8', 'stroke="#fff" stroke-opacity=".4" stroke-width="2.4" fill="none"'); for (i = 0; i < 8; i++) g += n5c(px + 30 * Math.cos(i * .785), 600 + 30 * Math.sin(i * .785), 2.4, 'fill="#3a2c10"'); });
    for (i = 0; i < 120; i++) { x = -300 + r() * 1800; y = 520 + Math.pow(r(), 2) * 170; g += n5c(x, y, 2 + r() * 3.4, 'fill="#cfd8cc" opacity=".75"') + n5c(x, y, .9, 'fill="#1e3a3a"'); }
    g += n5p('M-300,700 q160,-40 320,-14 t300,-12 t320,10 t300,-16 t260,12 V900 H-300 Z', 'fill="#c8b88a"') + n5p('M-300,700 q160,-40 320,-14 t300,-12 t320,10 t300,-16 t260,12', 'stroke="#fff4d0" stroke-opacity=".5" stroke-width="2" fill="none"');
    for (i = 0; i < 5; i++) g += n5p('M560,672 l6,16 l17,2 l-13,11 l4,17 l-14,-9 l-14,9 l4,-17 l-13,-11 l17,-2 Z', i ? 'fill="none"' : 'fill="#e8703a" stroke="#a8401a" stroke-width="1.4"');
    return '<g filter="url(#cm-woodlit)">' + s + g + n5r(-300, 520, 1800, 30, 'fill="#000" opacity=".5" filter="url(#src-blur8)"') + '</g>';
  };
  LIGHTS.jellyfish = function (t) {
    var x = t.lp.x, y = 380, a = t.th.accent, s, i, tn = '';
    s = '<defs>' + rad('n5-jelly', [[0, '#ffffff', .95], [.5, mix(a, '#ffffff', .4), .7], [1, a, .25]], ' cx=".5" cy=".7" r=".8"') + '</defs>';
    for (i = -3; i <= 3; i++) tn += n5g('class="n5-fbt n5-wave" style="animation-duration:' + f1(4 + Math.abs(i) * .6) + 's;animation-delay:-' + f1((i + 3) * .7) + 's"', n5p(n5wav(0, 70 + (3 - Math.abs(i)) * 12, 0, 5, 36, i).replace(/([\d.-]+),([\d.-]+)/g, function (m, px, py) { return f1(x + i * 9 + parseFloat(py)) + ',' + f1(y + 6 + parseFloat(px)); }), 'stroke="' + mix(a, '#ffffff', .5) + '" stroke-opacity=".7" stroke-width="' + (Math.abs(i) % 2 ? 1.4 : 2.6) + '" fill="none" stroke-linecap="round"'));
    s = '<g data-k="lamp"><g class="n5-hover">' + s + tn + n5c(x, y - 14, 60, 'fill="url(#cm-glow)" opacity=".8" class="cm-pulse"') + n5p('M' + (x - 40) + ',' + (y + 4) + ' Q' + (x - 44) + ',' + (y - 50) + ' ' + x + ',' + (y - 54) + ' Q' + (x + 44) + ',' + (y - 50) + ' ' + (x + 40) + ',' + (y + 4) + ' q-7,8 -13,0 q-7,8 -14,0 q-6,8 -13,0 q-7,8 -13,0 q-7,8 -14,0 q-6,8 -13,0 Z', 'fill="url(#n5-jelly)"') + n5p('M' + (x - 26) + ',' + (y - 20) + ' Q' + (x - 24) + ',' + (y - 42) + ' ' + (x - 4) + ',' + (y - 46), 'stroke="#fff" stroke-opacity=".8" stroke-width="2.4" fill="none" stroke-linecap="round"') + [-14, 0, 14].map(function (o) { return n5e(x + o, y - 20, 6, 9, 'fill="#fff" opacity=".35"'); }).join('') + '</g>';
    return { back: '', front: s + '<circle cx="' + x + '" cy="' + (y - 10) + '" r="130" fill="url(#cm-bloom)" class="cm-pulse" opacity=".8"/></g>' };
  };
  PROPS.fish = function (t, p) {
    var r = rng(191), s = '', i, g, x, y, c;
    [[90, 26, '#f0a040'], [200, 38, '#bfe8f0'], [330, 50, '#f06a6a']].forEach(function (q, j) {
      g = ''; for (i = 0; i < 7 + j * 2; i++) { x = 1260 + r() * 260; y = q[0] + (r() - .5) * 90; var k = .8 + r() * .7; c = mix(q[2], '#ffffff', r() * .3);
        g += n5g('class="n5-fb n5-bob" style="animation-duration:' + f1(1.6 + r() * 1.6) + 's;animation-delay:-' + f1(r() * 3) + 's"', n5p('M' + f1(x) + ',' + f1(y) + ' q' + f1(10 * k) + ',' + f1(-9 * k) + ' ' + f1(24 * k) + ',0 l' + f1(10 * k) + ',' + f1(-7 * k) + ' v' + f1(14 * k) + ' l' + f1(-10 * k) + ',' + f1(-7 * k) + ' q' + f1(-14 * k) + ',' + f1(9 * k) + ' ' + f1(-24 * k) + ',0 Z', 'fill="' + c + '"') + n5c(x + 5 * k, y - 1.4 * k, 1.2 * k, 'fill="#06181e"') + n5p('M' + f1(x + 9 * k) + ',' + f1(y - 4 * k) + ' v' + f1(8 * k), 'stroke="#000" stroke-opacity=".25" stroke-width="1"')); }
      s += n5g('class="n5-swim" style="--sx:-1860px;animation-duration:' + q[1] + 's;animation-delay:-' + (j * 9) + 's"', g);
    });
    return '<g class="src-nohit" clip-path="url(#n5-fishclip)"><clipPath id="n5-fishclip"><rect x="500" y="-150" width="1000" height="620"/></clipPath>' + s + '</g>';
  };
  PROPS.kelp = function (t, p) {
    var r = rng(193), s = '';
    [[528, 250, 0], [770, 190, 2], [1052, 300, 4], [1160, 380, 1], [1196, 240, 3]].forEach(function (q) {
      var x = q[0], h = q[1], g = n5p(n5wav(0, h, 0, 9, 90, q[2], 8).replace(/([\d.-]+),([\d.-]+)/g, function (m, px, py) { return f1(x + parseFloat(py)) + ',' + f1(520 - parseFloat(px)); }), 'stroke="#1e5a34" stroke-width="5" fill="none" stroke-linecap="round"'), y, k = 0, c;
      for (y = 20; y < h; y += 22, k++) { c = mix('#2a7a3e', '#7aa838', r() * .7); var ox = Math.sin(y / 90 * 6.283 + q[2]) * 9; g += mpLeaf(x + ox, 520 - y, 30 + r() * 16, (k % 2 ? -150 : -30) + (r() - .5) * 20, c, '#d8f0a0'); }
      s += n5g('class="n5-fbb n5-wave" style="animation-duration:' + f1(6 + r() * 3) + 's;animation-delay:-' + f1(r() * 6) + 's"', g);
    });
    return n5g('class="src-nohit"', s);
  };
  PROPS.caustics = function (t, p) {
    var r = rng(195), s = '', i;
    for (i = 0; i < 14; i++) s += n5p(n5wav(-100 + r() * 1000, 200 + r() * 1100, 40 + i * 46 + r() * 20, 10 + r() * 10, 120 + r() * 90, r() * 6), 'class="n5-caust" stroke="#e8fff8" stroke-opacity=".5" stroke-width="' + f1(2 + r() * 4) + '" fill="none" filter="url(#src-blur5)" style="animation-duration:' + f1(7 + r() * 6) + 's;animation-delay:-' + f1(r() * 9) + 's"');
    for (i = 0; i < 16; i++) s += n5c(60 + r() * 1100, 500 + r() * 60, 1.6 + r() * 3, 'class="n5-streak" fill="none" stroke="#e8fff8" stroke-width="1.2" style="--fy:-' + f1(360 + r() * 240) + 'px;animation-duration:' + f1(7 + r() * 7) + 's;animation-delay:-' + f1(r() * 14) + 's"');
    return n5g('class="src-nohit" opacity=".5"', s);
  };
  PROPS.crab = function (t, p) {
    var x = p.x, y = 494, c = '#d8502a', cd = '#8a2a14', s = n5e(x, y + 2, 40, 6, 'fill="url(#src-ao)"'), g, i;
    g = ''; for (i = 0; i < 3; i++) g += n5p('M' + (x - 20) + ',' + (y - 12 + i * 3) + ' l' + (-16 - i * 2) + ',' + (-4 + i * 5) + ' l-6,' + (10 - i) + ' M' + (x + 20) + ',' + (y - 12 + i * 3) + ' l' + (16 + i * 2) + ',' + (-4 + i * 5) + ' l6,' + (10 - i), 'stroke="' + cd + '" stroke-width="3.4" fill="none" stroke-linecap="round" stroke-linejoin="round"');
    g += n5e(x, y - 14, 26, 15, 'fill="' + c + '"') + n5e(x, y - 18, 20, 8, 'fill="#fff" opacity=".2"') + n5p('M' + (x - 12) + ',' + (y - 8) + ' q12,6 24,0', 'stroke="' + cd + '" stroke-width="1.6" fill="none"');
    g += n5p('M' + (x - 8) + ',' + (y - 26) + ' v-9 M' + (x + 8) + ',' + (y - 26) + ' v-9', 'stroke="' + cd + '" stroke-width="2.4"') + n5g('class="n5-fb n5-blink"', n5c(x - 8, y - 37, 3.4, 'fill="#0a0604"') + n5c(x + 8, y - 37, 3.4, 'fill="#0a0604"') + n5c(x - 9, y - 38, 1, 'fill="#fff"') + n5c(x + 7, y - 38, 1, 'fill="#fff"'));
    // the big claw comes up and goes down again
    g += n5p('M' + (x - 24) + ',' + (y - 18) + ' q-14,-4 -18,-16', 'stroke="' + cd + '" stroke-width="5" fill="none" stroke-linecap="round"') + n5p('M' + (x - 50) + ',' + (y - 34) + ' q-2,-16 12,-18 q-4,8 2,10 q8,-4 8,6 q-8,10 -22,2 Z', 'fill="' + c + '"');
    g += n5g('class="n5-vb n5-nod" style="transform-origin:' + (x + 24) + 'px ' + (y - 18) + 'px;animation-duration:5s"', n5p('M' + (x + 24) + ',' + (y - 18) + ' q16,-6 20,-22', 'stroke="' + cd + '" stroke-width="5" fill="none" stroke-linecap="round"') + n5p('M' + (x + 52) + ',' + (y - 40) + ' q4,-20 -12,-24 q4,10 -4,12 q-8,-6 -10,6 q8,14 26,6 Z', 'fill="' + c + '"'));
    return n5g('class="src-nohit"', s + n5g('class="n5-flit" style="--fx:16px;animation-duration:9s"', g));
  };


  /* ---- The caravan tent: striped cloth walls, the flap tied back on the
     dunes at sundown, carpets behind the goods, a carpet over the counter
     and a pierced lamp that throws its pattern round the room. ---- */
  CM_OPT.shape.push('tent'); CM_OPT.wall.push('tent-stripes'); CM_OPT.opening.push('tent-flap'); CM_OPT.display.push('carpet-wall'); CM_OPT.counter.push('carpet');
  CM_WALLFQ['tent-stripes'] = [.06, 1.8, 41]; CM_OPEN_INFO['tent-flap'] = 'The tent flap';
  n5light('pierced-lamp', { base: '#ffcf90', glow: '#ffb060', amb: '#3a2c40', reach: 980, k: 1 });
  n5prop('glass-lamps', 'ceil', 'back'); n5prop('hookah', 'cL', 'front', 130); n5prop('cobra', 'tallR', 'front', 0, true);

  function n5zig(x0, x1, y, h, st) { var d = 'M' + x0 + ',' + y, x, k = 0; for (x = x0 + st; x <= x1; x += st, k++) d += ' L' + x + ',' + (y + (k % 2 ? 0 : -h)); return d; }
  WALLS['tent-stripes'] = function (t) {
    var c1 = mix(t.th.cloth, '#000', .05), c2 = '#d6c296', r = rng(201), s, x, i, g = '', a = t.th.accent;
    for (x = -300, i = 0; x < 1500; x += 92, i++) g += n5r(x, -150, 92, 620, 'fill="' + (i % 2 ? c2 : c1) + '"') + n5r(x + 90, -150, 2, 620, 'fill="#000" opacity=".25"');
    s = '<defs><pattern id="n5-kil" width="40" height="40" patternUnits="userSpaceOnUse"><path d="M20,2 L38,20 L20,38 L2,20 Z" fill="' + a + '"/><path d="M20,10 L30,20 L20,30 L10,20 Z" fill="' + c1 + '"/><circle cx="20" cy="20" r="3" fill="#f4e8c8"/><path d="M0,0 l6,0 l-6,6 Z M40,0 l-6,0 l6,6 Z M0,40 l6,0 l-6,-6 Z M40,40 l-6,0 l6,-6 Z" fill="#f4e8c8"/></pattern></defs>';
    // a woven band round the tent at shoulder height
    g += n5r(-300, 292, 1800, 48, 'fill="#1c2848"') + n5r(-300, 296, 1800, 40, 'fill="url(#n5-kil)"') + n5r(-300, 292, 1800, 3, 'fill="' + a + '"') + n5r(-300, 337, 1800, 3, 'fill="' + a + '"');
    s += '<g filter="url(#cm-wall)">' + g + '</g>';
    // the cloth hangs in long soft folds
    for (x = -300; x < 1500; x += 184) s += n5r(x + 30 + r() * 30, -150, 60, 620, 'fill="#000" opacity=".26" filter="url(#src-blur14)"') + n5r(x + 120 + r() * 20, -150, 26, 620, 'fill="#fff" opacity=".07" filter="url(#src-blur8)"');
    return s + '<rect ' + FULL + ' fill="#1a0e08" filter="url(#cm-stain)" opacity=".3"/>';
  };
  SHAPES.tent = function (t) {
    var s = WALLS[t.th.wall](t), c1 = mix(t.th.cloth, '#000', .2), c2 = '#bfa878', a = t.th.accent, x, i, g = '';
    // the roof cloth runs up to the pole; its seams fan out from there
    s += n5r(-300, -150, 1800, 190, 'fill="' + mix(c2, '#3a2a1a', .45) + '" filter="url(#cm-wall)"');
    for (i = -9; i <= 9; i++) s += n5p('M600,-420 L' + (600 + i * 150) + ',40', 'stroke="#000" stroke-opacity=".3" stroke-width="2.4"') + n5p('M600,-420 L' + (603 + i * 150) + ',40', 'stroke="#fff" stroke-opacity=".08" stroke-width="2"');
    s += n5r(-300, 34, 1800, 40, 'fill="#000" opacity=".45" filter="url(#src-blur8)"');
    // a scalloped valance with a tassel at each point
    for (x = -280, i = 0; x < 1500; x += 86, i++) g += n5p('M' + x + ',26 h86 v20 q-43,42 -86,0 Z', 'fill="' + (i % 2 ? c1 : c2) + '"') + n5p('M' + x + ',46 q43,42 86,0', 'stroke="' + a + '" stroke-width="3" fill="none"') + n5p('M' + (x + 43) + ',67 v10', 'stroke="' + a + '" stroke-width="1.6"') + n5p('M' + (x + 39) + ',77 h8 l2,16 h-12 Z', 'fill="' + a + '"') + n5c(x + 43, 76, 3.4, 'fill="' + mix(a, '#fff', .3) + '"');
    s += g + n5r(-300, 22, 1800, 8, 'fill="' + a + '"') + n5r(-300, 22, 1800, 1.6, 'fill="#fff" opacity=".4"');
    return s + cmPost(-6, 30, 30, 474) + cmPost(1176, 30, 30, 474);
  };
  OPENINGS['tent-flap'] = function (t) {
    var d = 'M806,452 L912,98 L1018,452 Z', r = rng(203), night = t.time === 'night', c1 = mix(t.th.cloth, '#000', .1), lin1 = '#d6c296', a = t.th.accent, s, fr, cam, i, D = night ? ['#22284a', '#161a34', '#0a0c1c'] : ['#e0a060', '#b8703a', '#6e4024'];
    s = cmSky(t, 'n5-tsky', 800, 90, 224, 280);
    if (!night) s += n5c(962, 300, 60, 'fill="#ffd890" opacity=".5" filter="url(#src-blur8)"') + n5c(962, 304, 30, 'fill="#fff0c0"');
    else s += mpStars(r, 40, 804, 96, 216, 200, true);
    s += n5p('M800,330 q50,-34 110,-10 t120,-16 V460 H800 Z', 'fill="' + D[0] + '"') + n5p('M800,330 q50,-34 110,-10', 'stroke="#fff0c0" stroke-opacity=".4" stroke-width="1.6" fill="none"');
    // a string of camels along the next dune, never arriving
    cam = function (x, y, k) { return n5p('M0,0 l1,-14 q-4,-2 -5,-8 l-7,-3 l1,-3 l8,1 q3,-1 5,3 q3,6 8,6 q3,-8 9,-8 q6,0 8,7 q6,1 8,7 l2,15 h-3 l-2,-12 h-4 l-1,12 h-3 l-1,-12 h-9 l-1,12 h-3 v-12 l-3,-1 l-1,13 Z', 'fill="' + D[2] + '" transform="translate(' + x + ',' + y + ') scale(' + k + ')"'); };
    var line = ''; [0, 280].forEach(function (o) { [0, 56, 110, 170].forEach(function (cx, j) { line += j === 3 ? n5p('M0,0 l2,-10 l-2,-9 l3,-4 l3,4 l-2,9 l2,10 h-2.4 l-1,-8 l-1,8 Z', 'fill="' + D[2] + '" transform="translate(' + (806 + o + cx) + ',372)"') : cam(826 + o + cx, 372 + (j % 2), 1) + n5p('M' + (860 + o + cx) + ',356 q12,6 22,0', 'stroke="' + D[2] + '" stroke-width="1" fill="none"'); }); });
    s += n5g('class="n5-slide" style="--sx:-280px;animation-duration:60s"', line) + n5p('M800,372 q70,-16 130,0 t100,-8 V460 H800 Z', 'fill="' + D[1] + '"') + n5p('M800,410 q60,-20 110,-4 t120,-14 V460 H800 Z', 'fill="' + D[2] + '"');
    s += cmRainIn(t, 800, 90, 224, 370, 5);
    // each side of the door cloth is pulled back and tied
    function flap(k) {
      var q = function (x) { return 912 + k * x; }, o = 'M' + q(0) + ',98 L' + q(-34) + ',98 Q' + q(-70) + ',230 ' + q(-150) + ',306 Q' + q(-122) + ',380 ' + q(-134) + ',452 H' + q(-106) + ' Z', g = n5sh(o, k * 6, 6, .5, 8) + n5p(o, 'fill="' + c1 + '"') + n5p('M' + q(0) + ',98 Q' + q(-52) + ',250 ' + q(-126) + ',310 Q' + q(-104) + ',380 ' + q(-106) + ',452 Z', 'fill="' + lin1 + '"'), j;
      for (j = 0; j < 4; j++) g += n5p('M' + q(-8 - j * 7) + ',104 Q' + q(-60 - j * 5) + ',240 ' + q(-140) + ',306', 'stroke="#000" stroke-opacity=".3" stroke-width="3" fill="none"') + n5p('M' + q(-138) + ',310 Q' + q(-118 + j * 4) + ',380 ' + q(-128 + j * 6) + ',450', 'stroke="#000" stroke-opacity=".25" stroke-width="3" fill="none"');
      return g + n5p('M' + q(-154) + ',300 q' + (k * 14) + ',12 ' + (k * 26) + ',2', 'stroke="' + a + '" stroke-width="5" fill="none" stroke-linecap="round"') + n5p('M' + q(-150) + ',306 v26', 'stroke="' + a + '" stroke-width="2.4"') + n5e(q(-150), 338, 4, 9, 'fill="' + a + '"');
    }
    fr = flap(1) + flap(-1) + n5p(d, 'fill="none" stroke="' + a + '" stroke-width="4" stroke-linejoin="round"') + n5p(d, 'fill="none" stroke="#000" stroke-opacity=".4" stroke-width="1.4"') + n5c(912, 96, 7, 'fill="' + a + '"');
    return mpOpen(d, s, fr, night ? '' : n5e(912, 330, 240, 160, 'fill="#ffb060" opacity=".08" filter="url(#src-blur14)" pointer-events="none"'));
  };
  DISPLAYS['carpet-wall'] = function (t) {
    var r = rng(205), a = t.th.accent, s = n5r(30, 80, 466, 372, 'fill="#000" opacity=".45" filter="url(#src-blur8)"'), F = [[mix(t.th.cloth, '#000', .15), '#1c2848'], ['#1c3a54', mix(t.th.cloth, '#000', .1)], ['#6a4a16', '#243c2e']], k, x0, j, y;
    // three carpets hung side by side behind the shelves
    for (k = 0; k < 3; k++) {
      x0 = 36 + k * 152;
      s += n5r(x0, 80, 146, 364, 'fill="' + F[k][0] + '"') + n5r(x0 + 6, 86, 134, 352, 'fill="none" stroke="' + a + '" stroke-width="2.4"') + n5r(x0 + 14, 94, 118, 336, 'fill="none" stroke="' + F[k][1] + '" stroke-width="8"') + n5r(x0 + 14, 94, 118, 336, 'fill="none" stroke="#f4e8c8" stroke-width="5" stroke-dasharray="3 7"') + n5r(x0 + 22, 102, 102, 320, 'fill="none" stroke="' + a + '" stroke-width="1.4"');
      for (j = 0; j < 3; j++) { y = 152 + j * 110; s += n5p('M' + (x0 + 73) + ',' + (y - 44) + ' l34,44 l-34,44 l-34,-44 Z', 'fill="' + F[k][1] + '" stroke="' + a + '" stroke-width="2"') + n5p('M' + (x0 + 73) + ',' + (y - 22) + ' l17,22 l-17,22 l-17,-22 Z', 'fill="' + a + '"') + n5c(x0 + 73, y, 5, 'fill="#f4e8c8"') + n5p('M' + (x0 + 28) + ',' + (y + 54) + ' l8,-8 l8,8 l-8,8 Z M' + (x0 + 102) + ',' + (y + 54) + ' l8,-8 l8,8 l-8,8 Z', 'fill="#f4e8c8" opacity=".7"'); }
      s += n5p('M' + x0 + ',444 h146', 'stroke="#f4e8c8" stroke-width="10" stroke-dasharray="1.6 3"') + n5r(x0 + 144, 80, 4, 364, 'fill="#000" opacity=".4"');
    }
    s += n5r(34, 78, 458, 372, 'fill="#000" opacity=".3"') + n5r(30, 72, 466, 10, 'rx="5" fill="#000" filter="url(#cm-beam)"') + n5c(30, 77, 8, 'fill="url(#cm-trim)"') + n5c(496, 77, 8, 'fill="url(#cm-trim)"');
    // carved shelves, each with a little arcade cut under its edge
    CM_ROWS.forEach(function (ry) { var x, ap = 'M36,' + (ry + 12); for (x = 36; x < 486; x += 30) ap += ' v10 q15,-18 30,0 v-10'; s += cmShelfBoard(t, ry, 36, 486) + n5p(ap + ' Z', 'fill="' + mix(t.th.wood, '#000', .45) + '"') + n5p(ap, 'stroke="' + t.lc + '" stroke-opacity=".2" stroke-width="1" fill="none"'); });
    return { art: '<g filter="url(#cm-unitlit)">' + s + '</g>', slots: cmGrid(), zone: [34, 96, 454, 320], wallsh: true, decor: true, label: 'The carpet wall',
      // every good is set out on its own brass tray
      base: function (it, p) { return n5e(p[0], p[1] + 2, 42, 7, 'fill="#000" opacity=".4" filter="url(#src-blur3)"') + n5e(p[0], p[1], 40, 6.4, 'fill="url(#cm-trim)"') + n5e(p[0], p[1] - 1, 33, 4.4, 'fill="#000" opacity=".22"') + n5p('M' + (p[0] - 36) + ',' + (p[1] - 2) + ' q36,-7 72,0', 'stroke="#fff" stroke-opacity=".6" stroke-width="1.2" fill="none"'); } };
  };
  COUNTERS.carpet = function (t) {
    var s = cmTop(t, 'M-300,452 H1500 V512 H-300 Z'), r = rng(207), a = t.th.accent, c = mix(t.th.cloth, '#0a0606', .06), nv = '#1c2848', cr = '#f0e2bc', g, x;
    s += '<clipPath id="cm-topclip"><rect x="-300" y="452" width="1800" height="60"/></clipPath>';
    g = '<defs><pattern id="n5-rug" width="132" height="110" patternUnits="userSpaceOnUse"><path d="M66,8 L112,55 L66,102 L20,55 Z" fill="' + nv + '"/><path d="M66,24 L96,55 L66,86 L36,55 Z" fill="' + a + '"/><path d="M66,38 l5,12 l12,5 l-12,5 l-5,12 l-5,-12 l-12,-5 l12,-5 Z" fill="' + cr + '"/><path d="M0,55 l12,-12 v24 Z M132,55 l-12,-12 v24 Z M66,0 l-10,8 h20 Z M66,110 l-10,-8 h20 Z" fill="' + cr + '"/><circle cx="0" cy="0" r="6" fill="' + a + '"/><circle cx="132" cy="0" r="6" fill="' + a + '"/><circle cx="0" cy="110" r="6" fill="' + a + '"/><circle cx="132" cy="110" r="6" fill="' + a + '"/></pattern></defs>';
    // the carpet falls from the top edge to the ground: borders, a field of medallions, a fringe
    g += n5r(-300, 500, 1800, 400, 'fill="' + c + '"') + n5r(-300, 552, 1800, 110, 'fill="url(#n5-rug)"');
    [[512, 34], [664, 30]].forEach(function (q) { g += n5r(-300, q[0], 1800, q[1], 'fill="' + nv + '"') + n5p(n5zig(-300, 1500, q[0] + q[1] - 8, q[1] - 16, 16), 'stroke="' + a + '" stroke-width="3.4" fill="none" stroke-linejoin="round"') + n5p(n5zig(-292, 1500, q[0] + q[1] - 4, q[1] - 16, 16), 'stroke="' + cr + '" stroke-width="1.4" fill="none" stroke-linejoin="round"') + n5r(-300, q[0], 1800, 3, 'fill="' + cr + '"') + n5r(-300, q[0] + q[1] - 3, 1800, 3, 'fill="' + cr + '"'); });
    g += n5p('M-300,700 H1500', 'stroke="' + cr + '" stroke-width="14" stroke-dasharray="2 3.4"') + n5r(-300, 707, 1800, 200, 'fill="#0a0604"');
    for (x = -300; x < 1500; x += 210) g += n5r(x + 40 + r() * 40, 500, 70, 400, 'fill="#000" opacity=".28" filter="url(#src-blur14)"') + n5r(x + 150 + r() * 20, 500, 24, 400, 'fill="#fff" opacity=".07" filter="url(#src-blur8)"');
    return '<g filter="url(#cm-woodlit)">' + s + g + n5r(-300, 500, 1800, 14, 'fill="#000" opacity=".4"') + n5r(-300, 512, 1800, 30, 'fill="#000" opacity=".4" filter="url(#src-blur8)"') + '</g>';
  };
  LIGHTS['pierced-lamp'] = function (t) {
    var x = t.lp.x, r = rng(209), s, i, k, spots = '', body = 'M' + (x - 12) + ',476 Q' + (x - 38) + ',450 ' + (x - 24) + ',420 Q' + (x - 16) + ',402 ' + (x - 9) + ',396 L' + (x - 9) + ',388 Q' + x + ',372 ' + (x + 9) + ',388 L' + (x + 9) + ',396 Q' + (x + 16) + ',402 ' + (x + 24) + ',420 Q' + (x + 38) + ',450 ' + (x + 12) + ',476 Z';
    // its pattern, thrown on everything near and turning very slowly
    for (k = 0; k < 3; k++) for (i = 0; i < 12 + k * 6; i++) { var a = i / (12 + k * 6) * 6.283 + k * .3, R = 110 + k * 90, px = x + Math.cos(a) * R, py = 430 + Math.sin(a) * R * .8, z = 7 + k * 5; spots += n5p('M' + f1(px) + ',' + f1(py - z) + ' l' + f1(z * .6) + ',' + z + ' l' + f1(-z * .6) + ',' + z + ' l' + f1(-z * .6) + ',' + (-z) + ' Z', 'fill="#ffdc98"'); }
    s = '<g data-k="lamp">' + n5g('class="n5-vb n5-turn src-nohit" style="transform-origin:' + x + 'px 430px;animation-duration:140s" opacity=".15" filter="url(#src-blur3)"', spots);
    s += n5e(x, 482, 34, 6, 'fill="url(#src-ao)"') + n5c(x, 436, 50, 'fill="url(#cm-glow)" opacity=".8" class="cm-pulse"') + n5p(body, 'fill="url(#cm-trim)"') + n5p(body, 'fill="#000" opacity=".2"') + n5r(x - 16, 474, 32, 6, 'rx="2" fill="url(#cm-trim)"') + n5c(x, 368, 6, 'fill="none" stroke="url(#cm-trim)" stroke-width="2.4"');
    // the piercings: stars and drops with the flame behind them
    [[0, 410, 5], [-12, 428, 4], [12, 428, 4], [0, 446, 6], [-18, 448, 3.4], [18, 448, 3.4], [-9, 464, 3], [9, 464, 3]].forEach(function (q, j) { s += n5p('M' + (x + q[0]) + ',' + (q[1] - q[2]) + ' l' + f1(q[2] * .4) + ',' + f1(q[2] * .6) + ' l' + f1(q[2] * .6) + ',' + f1(q[2] * .4) + ' l' + f1(-q[2] * .6) + ',' + f1(q[2] * .4) + ' l' + f1(-q[2] * .4) + ',' + f1(q[2] * .6) + ' l' + f1(-q[2] * .4) + ',' + f1(-q[2] * .6) + ' l' + f1(-q[2] * .6) + ',' + f1(-q[2] * .4) + ' l' + f1(q[2] * .6) + ',' + f1(-q[2] * .4) + ' Z', 'class="n5-beat" fill="#fff0b8" style="animation-delay:-' + f1(j * .6) + 's;animation-duration:' + f1(3 + (j % 3)) + 's"'); });
    s += n5p('M' + (x - 22) + ',424 Q' + (x - 30) + ',450 ' + (x - 12) + ',470', 'stroke="#fff" stroke-opacity=".5" stroke-width="1.6" fill="none"');
    return { back: '', front: s + '<circle cx="' + x + '" cy="436" r="120" fill="url(#cm-bloom)" class="cm-pulse" opacity=".8"/></g>' };
  };
  PROPS['glass-lamps'] = function (t, p) {
    var s = '', r = rng(211);
    [[516, 56, '#e8a030'], [704, 86, '#3ac0b0'], [752, 48, '#d8404a'], [1066, 72, '#8a5ad8'], [1120, 106, '#e8a030'], [1170, 54, '#3ac0b0']].forEach(function (q, j) {
      var x = q[0], y = 40 + q[1], c = q[2], g;
      g = n5p('M' + x + ',40 V' + (y - 4), 'stroke="' + t.th.trim + '" stroke-width="1.4" stroke-dasharray="4 2"') + n5c(x, y + 18, 26, 'class="n5-beat" fill="' + c + '" opacity=".4" filter="url(#src-blur5)" style="animation-delay:-' + j + 's"');
      g += n5p('M' + (x - 7) + ',' + y + ' h14 l2,6 h-18 Z', 'fill="url(#cm-trim)"') + n5p('M' + (x - 9) + ',' + (y + 6) + ' Q' + (x - 16) + ',' + (y + 22) + ' ' + x + ',' + (y + 38) + ' Q' + (x + 16) + ',' + (y + 22) + ' ' + (x + 9) + ',' + (y + 6) + ' Z', 'fill="' + c + '"') + n5p('M' + (x - 9) + ',' + (y + 6) + ' Q' + (x - 16) + ',' + (y + 22) + ' ' + x + ',' + (y + 38) + ' Q' + (x + 16) + ',' + (y + 22) + ' ' + (x + 9) + ',' + (y + 6) + ' Z', 'fill="#fff8d0" opacity=".45" class="n5-beat" style="animation-delay:-' + j + 's"');
      g += n5p('M' + x + ',' + (y + 6) + ' V' + (y + 38) + ' M' + (x - 5) + ',' + (y + 6) + ' Q' + (x - 9) + ',' + (y + 22) + ' ' + x + ',' + (y + 38) + ' M' + (x + 5) + ',' + (y + 6) + ' Q' + (x + 9) + ',' + (y + 22) + ' ' + x + ',' + (y + 38), 'stroke="' + mix(t.th.trim, '#000', .3) + '" stroke-width="1.2" fill="none"') + n5c(x, y + 41, 3, 'fill="url(#cm-trim)"') + n5p('M' + (x - 6) + ',' + (y + 12) + ' q-3,8 1,14', 'stroke="#fff" stroke-opacity=".7" stroke-width="1.4" fill="none"');
      s += n5g('class="n5-vb n5-sway" style="transform-origin:' + x + 'px 40px;animation-duration:' + f1(6 + r() * 4) + 's;animation-delay:-' + f1(r() * 6) + 's"', g);
    });
    return n5g('class="src-nohit"', s);
  };
  PROPS.hookah = function (t, p) {
    var x = p.x - 22, y = 492, s = n5e(p.x, y + 4, 64, 9, 'fill="url(#src-ao)"'), i;
    // the pipe: a glass belly, a brass stem, a coal in the bowl and the hose looped beside it
    s += n5p('M' + (x + 8) + ',' + (y - 40) + ' C' + (x + 60) + ',' + (y - 60) + ' ' + (x + 30) + ',' + (y - 6) + ' ' + (x + 70) + ',' + (y - 4), 'stroke="' + mix(t.th.cloth, '#000', .1) + '" stroke-width="5" fill="none" stroke-linecap="round"') + n5p('M' + (x + 8) + ',' + (y - 40) + ' C' + (x + 60) + ',' + (y - 60) + ' ' + (x + 30) + ',' + (y - 6) + ' ' + (x + 70) + ',' + (y - 4), 'stroke="' + t.th.trim + '" stroke-width="5" fill="none" stroke-dasharray="2 9"') + n5r(x + 68, y - 8, 14, 6, 'rx="3" fill="url(#cm-trim)"');
    s += n5e(x, y - 18, 20, 18, 'fill="#1e6a7a"') + n5e(x, y - 18, 20, 18, 'fill="url(#src-glassR)"') + n5e(x - 6, y - 24, 5, 8, 'fill="#fff" opacity=".3"') + n5r(x - 22, y - 2, 44, 5, 'rx="2" fill="url(#cm-trim)"') + n5r(x - 4, y - 96, 8, 64, 'fill="url(#cm-trim)"') + n5e(x, y - 60, 8, 5, 'fill="url(#cm-trim)"') + n5e(x, y - 78, 6, 4, 'fill="url(#cm-trim)"') + n5p('M' + (x - 12) + ',' + (y - 96) + ' h24 l-4,-12 h-16 Z', 'fill="url(#cm-trim)"') + n5e(x, y - 108, 8, 3, 'class="n5-beat" fill="#ff7a30"');
    for (i = 0; i < 4; i++) s += n5e(x + (i - 1.5) * 4, y - 112, 9, 7, 'class="n5-fb n5-puff" fill="#e8e0f0" filter="url(#src-blur3)" style="--fx:' + ((i % 2 ? 1 : -1) * (8 + i * 5)) + 'px;--fy:-' + (80 + i * 16) + 'px;animation-delay:-' + f1(i * 1.5) + 's"');
    // tea, set out on a tray
    s += n5e(x + 62, y - 8, 30, 5, 'fill="url(#cm-trim)"') + n5e(x + 62, y - 9, 24, 3.4, 'fill="#000" opacity=".2"');
    [48, 76].forEach(function (o) { s += n5p('M' + (x + o - 5) + ',' + (y - 10) + ' l-1,-14 h12 l-1,14 Z', 'fill="#c8702a" opacity=".85"') + n5r(x + o - 6, y - 24, 12, 3, 'fill="#fff" opacity=".4"') + n5r(x + o - 6, y - 24, 3, 14, 'fill="#fff" opacity=".3"'); });
    return n5g('class="src-nohit"', s);
  };
  PROPS.cobra = function (t, p) {
    var x = p.x, s, g, i, c = '#9a7e34', b = '#e6d69c';
    s = n5e(x + 4, 484, 54, 8, 'fill="url(#src-ao)"');
    // it rises from its basket and keeps time with something only it can hear
    g = n5p('M' + (x - 4) + ',444 C' + (x - 30) + ',400 ' + (x + 26) + ',380 ' + (x + 4) + ',340 C' + (x - 8) + ',320 ' + (x - 6) + ',306 ' + x + ',296', 'stroke="' + c + '" stroke-width="15" fill="none" stroke-linecap="round"') + n5p('M' + (x - 4) + ',444 C' + (x - 30) + ',400 ' + (x + 26) + ',380 ' + (x + 4) + ',340 C' + (x - 8) + ',320 ' + (x - 6) + ',306 ' + x + ',296', 'stroke="' + b + '" stroke-width="6" fill="none" stroke-linecap="round" stroke-dasharray="3 5"');
    g += n5p('M' + x + ',340 C' + (x - 34) + ',322 ' + (x - 30) + ',274 ' + x + ',262 C' + (x + 30) + ',274 ' + (x + 34) + ',322 ' + x + ',340 Z', 'fill="' + c + '"') + n5p('M' + x + ',334 C' + (x - 20) + ',320 ' + (x - 18) + ',288 ' + x + ',278 C' + (x + 18) + ',288 ' + (x + 20) + ',320 ' + x + ',334 Z', 'fill="' + b + '"');
    for (i = 0; i < 5; i++) g += n5p('M' + (x - 14 + i) + ',' + (290 + i * 9) + ' h' + (28 - i * 2), 'stroke="' + c + '" stroke-opacity=".6" stroke-width="1.4"');
    g += n5p('M' + (x - 11) + ',270 q-3,-18 11,-20 q14,2 11,20 q-11,8 -22,0 Z', 'fill="' + mix(c, '#000', .1) + '"') + n5e(x - 6, 258, 2.4, 3, 'fill="#ffd040"') + n5e(x + 6, 258, 2.4, 3, 'fill="#ffd040"') + n5e(x - 6, 258, .8, 2.6, 'fill="#0a0604"') + n5e(x + 6, 258, .8, 2.6, 'fill="#0a0604"');
    g += n5p('M' + x + ',274 v10 l-4,6 M' + x + ',284 l4,6', 'class="n5-glint" stroke="#c82a3a" stroke-width="1.6" fill="none" style="animation-duration:3.2s"');
    s += n5g('class="n5-vb n5-sway" style="transform-origin:' + x + 'px 444px;animation-duration:4.4s"', g);
    // the basket: coiled rush, a lid leant against it
    s += n5e(x + 44, 474, 26, 8, 'fill="#8a6a34" transform="rotate(-60 ' + (x + 44) + ' 474)"') + n5p('M' + (x - 42) + ',440 q-10,26 6,42 h72 q16,-16 6,-42 Z', 'fill="#a8843e"');
    for (i = 0; i < 5; i++) s += n5p(n5wav(x - 44 + i * 2, x + 44 - i * 2, 446 + i * 8, 2, 12, i * 3.1416, 4), 'stroke="#5a4218" stroke-width="1.6" fill="none"');
    s += n5e(x, 440, 42, 9, 'fill="#c8a458"') + n5e(x, 441, 34, 6, 'fill="#1a1006"') + n5p('M' + (x - 38) + ',452 q-6,14 4,26', 'stroke="#fff" stroke-opacity=".25" stroke-width="4" fill="none"');
    return n5g('class="src-nohit"', s);
  };

  /* ---- The sky barge: no walls at all, only weather. Cloud goes by under
     the keel, the goods ride in slings and the counter is the ship's rail. ---- */
  CM_OPT.shape.push('gondola'); CM_OPT.wall.push('open-sky'); CM_OPT.opening.push('cloud-sea'); CM_OPT.display.push('cargo-nets'); CM_OPT.counter.push('gunwale');
  CM_OPEN_INFO['cloud-sea'] = 'The weather';
  n5prop('pennants', 'ceil', 'back'); n5prop('propeller', 'wallR', 'back'); n5prop('grapnel', 'cL', 'front', 140);

  function n5puff(r, x, y, w, col, op) {
    var s = '', i, n = 5 + Math.floor(r() * 3);
    for (i = 0; i < n; i++) s += n5e(x + (i / (n - 1) - .5) * w, y - Math.sin(i / (n - 1) * 3.1416) * w * .16 - r() * w * .05, w * (.2 + r() * .1), w * (.11 + r() * .07), 'fill="' + col + '"');
    return n5g('opacity="' + op + '" filter="url(#src-blur3)"', s + n5e(x, y + w * .04, w * .56, w * .09, 'fill="' + col + '"'));
  }
  WALLS['open-sky'] = function (t) {
    var tm = t.time, r = rng(231), C = tm === 'night' ? ['#060a22', '#16224c', '#33426e'] : tm === 'dusk' ? ['#2c2a6c', '#c4607c', '#ffc890'] : tm === 'rain' ? ['#3a4658', '#6a7888', '#9aa4b0'] : ['#2c6cc2', '#84bcf0', '#f6e8d0'], cl = tm === 'night' ? '#5a6890' : tm === 'dusk' ? '#ffd0b0' : tm === 'rain' ? '#aab4c0' : '#ffffff', s, i;
    s = '<defs>' + lin('n5-osky', 1, [[0, C[0]], [.62, C[1]], [1, C[2]]]) + '</defs><rect ' + FULL + ' fill="url(#n5-osky)"/>';
    if (tm === 'night') s += mpStars(r, 180, -300, -150, 1800, 440, false) + n5c(1090, 120, 30, 'fill="#f0f0e0"') + n5c(1100, 112, 26, 'fill="' + C[0] + '" opacity=".9"');
    else if (tm !== 'rain') s += n5c(1090, 130, 90, 'fill="#fff6d8" opacity=".5" filter="url(#src-blur14)"') + n5c(1090, 130, 34, 'fill="#fffbe8"');
    // far peaks stand up through the cloud, then the cloud sea itself, bank over bank
    s += n5p(n5ridge(r, -300, 1500, 400, 90, 60, 480), 'fill="' + mix(C[1], '#3a4a6a', .5) + '" opacity=".6"') + n5p(n5ridge(rng(231), -300, 1500, 400, 90, 60, 480), 'fill="none" stroke="#fff" stroke-opacity=".5" stroke-width="2"');
    for (i = 0; i < 46; i++) s += n5puff(r, -300 + r() * 1800, 400 + Math.pow(r(), .7) * 70, 180 + r() * 260, mix(cl, C[1], r() * .3), f1(.55 + r() * .4));
    s += n5r(-300, 440, 1800, 40, 'fill="' + cl + '" opacity=".7" filter="url(#src-blur8)"');
    return s;
  };
  SHAPES.gondola = function (t) {
    var s = WALLS[t.th.wall](t), c1 = mix(t.th.cloth, '#000', .1), c2 = '#e6d8b4', env = 'M-300,-150 H1500 V-60 Q600,130 -300,-60 Z', x, i;
    // the underside of the gas bag: gores in two colours under a net of cord
    s += '<defs><pattern id="n5-net" width="64" height="44" patternUnits="userSpaceOnUse"><path d="M0,22 L32,0 L64,22 L32,44 Z" fill="none" stroke="#5a4424" stroke-opacity=".7" stroke-width="1.6"/></pattern><clipPath id="n5-env"><path d="' + env + '"/></clipPath>' + lin('n5-envs', 1, [[0, '#000', .1], [.6, '#000', .25], [1, '#000', .6]], ' gradientUnits="userSpaceOnUse" x1="0" y1="-150" x2="0" y2="50"') + '</defs>';
    s += n5sh(env, 0, 14, .45, 14) + '<g clip-path="url(#n5-env)">';
    for (x = -300, i = 0; x < 1500; x += 150, i++) s += n5p('M' + x + ',-150 h150 L' + f1(600 + (x + 150 - 600) * .86) + ',60 H' + f1(600 + (x - 600) * .86) + ' Z', 'fill="' + (i % 2 ? c1 : c2) + '"');
    s += n5r(-300, -150, 1800, 220, 'fill="url(#n5-net)"') + n5r(-300, -150, 1800, 220, 'fill="url(#n5-envs)"') + '</g>' + n5p('M-300,-60 Q600,130 1500,-60', 'stroke="#8a6a34" stroke-width="6" fill="none"') + n5p('M-300,-60 Q600,130 1500,-60', 'stroke="#fff" stroke-opacity=".3" stroke-width="1.4" fill="none" transform="translate(0,-2)"');
    return s + cmPost(-6, 30, 0, 474) + cmPost(1176, 30, 0, 474);
  };
  OPENINGS['cloud-sea'] = function (t) {
    var r = rng(233), tm = t.time, cl = tm === 'night' ? '#6a78a0' : tm === 'dusk' ? '#ffd8c0' : tm === 'rain' ? '#b0bac6' : '#ffffff', ink = tm === 'night' ? '#c8d0e8' : '#1c2434', far = '', near = '', s, fr = '', i;
    [0, 1500].forEach(function (o) { var q = rng(235), j; for (j = 0; j < 7; j++) far += n5puff(q, o + 60 + j * 210 + q() * 60, 110 + q() * 200, 110 + q() * 90, cl, .7); });
    [0, 1800].forEach(function (o) { var q = rng(237), j; for (j = 0; j < 5; j++) near += n5puff(q, o + 100 + j * 360 + q() * 100, 250 + q() * 170, 240 + q() * 160, cl, .85); });
    s = n5g('class="n5-cloud" style="--sx:-1500px;animation-duration:150s"', far);
    // another ship, a long way off, making the same passage
    s += n5g('class="n5-swim" style="--sx:-1900px;animation-duration:110s;animation-delay:-30s"', n5g('opacity=".55"', n5e(1420, 200, 34, 15, 'fill="' + ink + '"') + n5r(1406, 220, 28, 7, 'rx="2" fill="' + ink + '"') + n5p('M1396,208 L1408,220 M1444,208 L1432,220 M1420,214 V220', 'stroke="' + ink + '" stroke-width="1"')));
    [[640, 150, 0], [760, 120, 2], [1000, 200, 5], [1080, 110, 7], [900, 90, 9]].forEach(function (q) { s += n5g('class="n5-flit" style="--fx:' + (q[2] % 2 ? -50 : 60) + 'px;animation-delay:-' + q[2] + 's;animation-duration:' + (11 + q[2]) + 's"', n5p('M' + (q[0] - 10) + ',' + q[1] + ' q5,-8 10,0 q5,-8 10,0', 'stroke="' + ink + '" stroke-width="1.8" fill="none" stroke-linecap="round"')); });
    s += n5g('class="n5-cloud" style="--sx:-1800px;animation-duration:90s"', near) + cmRainIn(t, -300, 60, 1800, 420, 9);
    // the shrouds that hold the car to the bag, in front of the weather
    [[10, -4], [506, -20], [786, 0], [1034, 8], [1192, 4]].forEach(function (q) { var u = (q[0] + 300) / 1800, y0 = -60 * (1 - u) * (1 - u) + 260 * u * (1 - u) - 60 * u * u; fr += n5p('M' + q[0] + ',' + f1(y0) + ' L' + (q[0] + q[1]) + ',470', 'stroke="#b89a68" stroke-width="3.4"') + n5p('M' + q[0] + ',' + f1(y0) + ' L' + (q[0] + q[1]) + ',470', 'stroke="#5a4424" stroke-width="3.4" stroke-dasharray="2 5"'); });
    return '<g data-k="window" class="src-nohit"><g clip-path="url(#cm-oclip)">' + s + '</g>' + fr + '</g><clipPath id="cm-oclip"><path d="M-300,40 H1500 V470 H-300 Z"/></clipPath>';
  };
  DISPLAYS['cargo-nets'] = function (t) {
    var s = '', rope = '#b89a68', dk = '#5a4424', i, j, x, y;
    // a spar slung under the bag, and from it three slings to a line
    s += n5p('M60,84 L40,-150 M462,84 L482,-150', 'stroke="' + rope + '" stroke-width="3.4"') + n5r(20, 76, 484, 16, 'rx="8" fill="#000" filter="url(#cm-beam)"') + n5r(20, 76, 484, 2.4, 'fill="' + t.lc + '" opacity=".4"');
    [56, 466].forEach(function (lx) { s += n5r(lx - 5, 72, 10, 24, 'rx="3" fill="' + rope + '"') + n5p('M' + (lx - 5) + ',78 h10 M' + (lx - 5) + ',84 h10 M' + (lx - 5) + ',90 h10', 'stroke="' + dk + '" stroke-width="1"'); });
    for (i = 0; i < 4; i++) for (j = 0; j < 3; j++) {
      x = CM_COLS[i]; y = CM_ROWS[j]; var y0 = j ? CM_ROWS[j - 1] + 16 : 92, k;
      s += n5p('M' + x + ',' + y0 + ' L' + (x - 38) + ',' + (y - 28) + ' M' + x + ',' + y0 + ' L' + (x + 38) + ',' + (y - 28), 'stroke="' + rope + '" stroke-width="2"');
      // the sling: a shallow net with a wooden spreader
      s += n5p('M' + (x - 38) + ',' + (y - 28) + ' Q' + (x - 38) + ',' + (y + 14) + ' ' + x + ',' + (y + 16) + ' Q' + (x + 38) + ',' + (y + 14) + ' ' + (x + 38) + ',' + (y - 28), 'stroke="' + rope + '" stroke-width="2.6" fill="none"');
      for (k = 1; k < 6; k++) s += n5p('M' + f1(x - 38 + k * 12.7) + ',' + (y + 2) + ' Q' + f1(x - 38 + k * 12.7) + ',' + (y + 12) + ' ' + f1(x + (k - 3) * 6) + ',' + (y + 16), 'stroke="' + rope + '" stroke-width="1.2" fill="none"');
      s += n5p('M' + (x - 37) + ',' + (y + 2) + ' Q' + x + ',' + (y + 12) + ' ' + (x + 37) + ',' + (y + 2) + ' M' + (x - 34) + ',' + (y + 9) + ' Q' + x + ',' + (y + 17) + ' ' + (x + 34) + ',' + (y + 9), 'stroke="' + rope + '" stroke-width="1.2" fill="none"') + n5p('M' + (x - 38) + ',' + (y + 2) + ' Q' + x + ',' + (y - 5) + ' ' + (x + 38) + ',' + (y + 2), 'stroke="' + dk + '" stroke-opacity=".6" stroke-width="1" fill="none"');
      s += n5c(x - 38, y - 28, 3, 'fill="url(#cm-trim)"') + n5c(x + 38, y - 28, 3, 'fill="url(#cm-trim)"') + n5c(x, y + 16, 3.4, 'fill="' + dk + '"');
    }
    return { art: s, slots: cmGrid(), zone: [34, 96, 454, 320], wallsh: false, decor: true, label: 'The cargo slings' };
  };
  COUNTERS.gunwale = function (t) {
    var s = cmTop(t, 'M-300,452 H1500 V512 H-300 Z'), r = rng(239), g = '', y, x, row = 0, tr = t.th.trim;
    s += '<clipPath id="cm-topclip"><rect x="-300" y="452" width="1800" height="60"/></clipPath>' + n5p('M-300,504 H1500 V522 H-300 Z', 'fill="#000" filter="url(#cm-beam)"') + n5r(-300, 504, 1800, 2.4, 'fill="' + t.lc + '" opacity=".5"');
    // clinker planking, each strake lapped and riveted
    for (y = 522; y < 900; y += 48, row++) { s += n5r(-300, y, 1800, 48, 'fill="#000" filter="url(#cm-front)"') + n5r(-300, y, 1800, 48, 'fill="' + (r() < .5 ? '#000' : '#fff') + '" opacity="' + f1(r() * .09) + '"') + n5r(-300, y + 41, 1800, 7, 'fill="#000" opacity=".55"') + n5r(-300, y, 1800, 1.6, 'fill="' + t.lc + '" opacity=".28"'); for (x = -290 + (row % 2) * 40; x < 1500; x += 80) g += n5c(x, y + 34, 3, 'fill="' + tr + '"') + n5c(x - .8, y + 33.2, 1, 'fill="#fff" opacity=".7"'); }
    // a cable looped along under the rail, and ballast hung over the side
    for (x = -300; x < 1500; x += 200) g += n5sag(x, 530, x + 200, 530, 22, '#000', 9, 'opacity=".4" filter="url(#src-blur3)" transform="translate(3,5)"') + n5sag(x, 530, x + 200, 530, 22, '#b89a68', 8) + n5sag(x, 530, x + 200, 530, 22, '#5a4424', 8, 'stroke-dasharray="2 7"') + n5c(x, 530, 6, 'fill="' + tr + '"');
    [140, 470, 830, 1090].forEach(function (bx, j) {
      var by = 600 + (j % 2) * 16, d = 'M' + (bx - 8) + ',' + by + ' Q' + (bx - 40) + ',' + (by + 30) + ' ' + (bx - 34) + ',' + (by + 70) + ' Q' + bx + ',' + (by + 92) + ' ' + (bx + 34) + ',' + (by + 70) + ' Q' + (bx + 40) + ',' + (by + 30) + ' ' + (bx + 8) + ',' + by + ' Z';
      g += n5p('M' + bx + ',524 V' + by, 'stroke="#b89a68" stroke-width="3"') + n5sh(d, 8, 6, .5, 5) + n5p(d, 'fill="#b49868"') + n5p('M' + (bx - 30) + ',' + (by + 36) + ' Q' + (bx - 34) + ',' + (by + 60) + ' ' + (bx - 20) + ',' + (by + 76), 'stroke="#fff" stroke-opacity=".3" stroke-width="5" fill="none"') + n5p('M' + (bx + 22) + ',' + (by + 30) + ' Q' + (bx + 30) + ',' + (by + 60) + ' ' + (bx + 12) + ',' + (by + 80), 'stroke="#000" stroke-opacity=".25" stroke-width="9" fill="none"') + n5p('M' + (bx - 10) + ',' + (by + 6) + ' h20', 'stroke="#5a4424" stroke-width="4" stroke-linecap="round"') + n5p('M' + bx + ',' + (by + 12) + ' V' + (by + 84), 'stroke="#6a5430" stroke-width="1.2" stroke-dasharray="4 3"');
    });
    return '<g filter="url(#cm-woodlit)">' + s + g + n5r(-300, 522, 1800, 26, 'fill="#000" opacity=".45" filter="url(#src-blur8)"') + '</g>';
  };
  PROPS.pennants = function (t, p) {
    var s = n5sag(500, 44, 850, 44, 16, '#8a7250', 2) + n5sag(850, 44, 1200, 44, 14, '#8a7250', 2), cols = [mix(t.th.cloth, '#fff', .1), t.th.accent, '#f0e6cc', '#c8404a'], i, x, y, u;
    for (i = 0; i < 14; i++) { x = 520 + i * 49; u = (x < 850 ? (x - 500) / 350 : (x - 850) / 350); y = 44 + 4 * u * (1 - u) * (x < 850 ? 32 : 28); s += n5g('class="n5-fbt n5-wave" style="animation-duration:' + f1(1.6 + (i % 4) * .4) + 's;animation-delay:-' + f1(i * .37) + 's"', n5p('M' + (x - 13) + ',' + f1(y) + ' h26 l-13,34 Z', 'fill="' + cols[i % 4] + '"') + n5p('M' + (x - 13) + ',' + f1(y) + ' h9 l-4,22 Z', 'fill="#000" opacity=".18"')); }
    return n5g('class="src-nohit"', s);
  };
  PROPS.propeller = function (t, p) {
    var x = 1128, y = 226, s, i, bl = '';
    s = n5p('M1210,' + (y - 30) + ' L' + x + ',' + y + ' L1210,' + (y + 40), 'stroke="' + mix(t.th.wood, '#000', .3) + '" stroke-width="9" fill="none" stroke-linejoin="round"') + n5p('M1210,' + (y - 30) + ' L' + x + ',' + y, 'stroke="' + t.lc + '" stroke-opacity=".3" stroke-width="2" fill="none"');
    for (i = 0; i < 3; i++) bl += n5p('M' + x + ',' + (y - 7) + ' Q' + (x + 34) + ',' + (y - 24) + ' ' + (x + 70) + ',' + (y - 5) + ' Q' + (x + 36) + ',' + (y + 8) + ' ' + x + ',' + (y + 7) + ' Z', 'fill="' + mix(t.th.wood, '#b8905c', .4) + '" stroke="#2a1a0c" stroke-width="1" transform="rotate(' + (i * 120) + ' ' + x + ' ' + y + ')"');
    s += n5c(x, y, 72, 'fill="#fff" opacity=".07"') + n5g('class="n5-vb n5-turn" style="transform-origin:' + x + 'px ' + y + 'px;animation-duration:2.2s"', bl) + n5c(x, y, 11, 'fill="url(#cm-trim)"') + n5c(x - 3, y - 3, 3, 'fill="#fff" opacity=".6"');
    return n5g('class="src-nohit"', s);
  };
  PROPS.grapnel = function (t, p) {
    var x = p.x, y = 492, s = n5e(x, y + 4, 70, 9, 'fill="url(#src-ao)"'), i, rope = '#b89a68';
    // a coil of line, the grapnel made fast to it, and the ship's compass
    for (i = 0; i < 5; i++) s += n5e(x - 26, y - 4 - i * 4, 34 - i, 9 - i * .4, 'fill="none" stroke="' + mix(rope, '#5a4424', i % 2 ? .3 : 0) + '" stroke-width="5"');
    s += n5p('M' + (x - 4) + ',' + (y - 22) + ' Q' + (x + 10) + ',' + (y - 50) + ' ' + (x + 20) + ',' + (y - 70), 'stroke="' + rope + '" stroke-width="4" fill="none"') + n5p('M' + (x + 20) + ',' + (y - 76) + ' V' + (y - 10), 'stroke="#2a2c30" stroke-width="5" stroke-linecap="round"') + n5c(x + 20, y - 80, 5, 'fill="none" stroke="#2a2c30" stroke-width="3"') + n5p('M' + (x + 20) + ',' + (y - 10) + ' q-22,6 -24,-14 M' + (x + 20) + ',' + (y - 10) + ' q22,6 24,-14 M' + (x + 20) + ',' + (y - 10) + ' q-4,10 -14,6', 'stroke="#2a2c30" stroke-width="4.4" fill="none" stroke-linecap="round"') + n5p('M' + (x + 18) + ',' + (y - 70) + ' V' + (y - 14), 'stroke="#9aa0a8" stroke-opacity=".5" stroke-width="1"');
    s += n5e(x + 58, y - 2, 20, 5, 'fill="#000" opacity=".4"') + n5c(x + 58, y - 16, 17, 'fill="url(#cm-trim)"') + n5c(x + 58, y - 16, 13, 'fill="#f0e6cc"') + n5g('class="n5-vb n5-tick" style="transform-origin:' + (x + 58) + 'px ' + (y - 16) + 'px;animation-duration:3s"', n5p('M' + (x + 58) + ',' + (y - 27) + ' l3,11 h-6 Z', 'fill="#c8283a"') + n5p('M' + (x + 58) + ',' + (y - 5) + ' l3,-11 h-6 Z', 'fill="#2a2c30"')) + n5c(x + 58, y - 16, 2, 'fill="#2a2c30"') + n5p('M' + (x + 48) + ',' + (y - 24) + ' q6,-6 14,-5', 'stroke="#fff" stroke-opacity=".6" stroke-width="1.6" fill="none"');
    return n5g('class="src-nohit"', s);
  };

  /* ---- The bog witch's hut: wattle walls, a crooked window on the marsh
     by moonlight, shelves that were never straight and a cauldron for a
     lamp. ---- */
  CM_OPT.shape.push('hut'); CM_OPT.wall.push('wattle'); CM_OPT.opening.push('bog-window'); CM_OPT.display.push('crooked-shelves'); CM_OPT.counter.push('driftwood');
  CM_WALLFQ.wattle = [.05, 2.4, 51]; CM_OPEN_INFO['bog-window'] = 'The crooked window';
  n5light('cauldron', { base: '#c8f490', glow: '#8ae04a', amb: '#1c2832', reach: 950, k: .9 });
  n5prop('toad', 'cL', 'front', 130); n5prop('bottle-tree', 'tallR', 'front', 0, true);

  function n5toadstool(x, y, k, col) { return n5p('M' + (x - 3 * k) + ',' + y + ' q-1,' + (-10 * k) + ' 1,' + (-16 * k) + ' h' + (4 * k) + ' q2,' + (6 * k) + ' 1,' + (16 * k) + ' Z', 'fill="#e8e0cc"') + n5p('M' + (x - 13 * k) + ',' + (y - 15 * k) + ' q' + (13 * k) + ',' + (-20 * k) + ' ' + (26 * k) + ',0 q' + (-13 * k) + ',' + (5 * k) + ' ' + (-26 * k) + ',0 Z', 'fill="' + col + '"') + n5c(x - 5 * k, y - 20 * k, 1.8 * k, 'fill="#fff" opacity=".9"') + n5c(x + 4 * k, y - 22 * k, 1.4 * k, 'fill="#fff" opacity=".9"') + n5c(x + 7 * k, y - 17 * k, 1.2 * k, 'fill="#fff" opacity=".9"'); }
  WALLS.wattle = function (t) {
    var r = rng(221), s, x, y, row = 0, g = '', i;
    for (x = -300; x < 1500; x += 70) g += n5r(x - 6, -150, 12, 620, 'fill="#120c06"');
    // withies woven over and under the stakes, course on course
    for (y = -150; y < 470; y += 15, row++) g += n5p(n5wav(-300, 1500, y, 5, 140, row % 2 ? 0 : 3.1416, 14), 'stroke="' + mix('#7a5a34', r() < .5 ? '#4a3420' : '#a88458', r() * .6) + '" stroke-width="' + f1(10 + r() * 4) + '" fill="none" stroke-linecap="round"') + n5p(n5wav(-300, 1500, y - 4, 5, 140, row % 2 ? 0 : 3.1416, 14), 'stroke="#fff" stroke-opacity=".1" stroke-width="2" fill="none"') + n5p(n5wav(-300, 1500, y + 6, 5, 140, row % 2 ? 0 : 3.1416, 14), 'stroke="#000" stroke-opacity=".4" stroke-width="2" fill="none"');
    // mud where it has been patched, and what has grown through since
    for (i = 0; i < 5; i++) g += n5p(mpLump(r, -260 + r() * 1700, -60 + r() * 480, 30 + r() * 50, 11), 'fill="' + mix('#6a5a44', '#3a3020', r()) + '" opacity=".7"');
    s = '<rect ' + FULL + ' fill="#0a0704"/><g filter="url(#cm-wall)">' + g + '</g><rect ' + FULL + ' fill="#0a1208" filter="url(#cm-stain)" opacity=".5"/>';
    for (i = 0; i < 9; i++) s += mpVine(r, -260 + r() * 1720, -150 + r() * 120, 120 + r() * 200, '#2e5a2a', '#5a8a3a');
    return s;
  };
  SHAPES.hut = function (t) {
    var s = WALLS[t.th.wall](t), r = rng(223), i, x, d;
    // thatch from underneath, and two rafters nobody trued up
    s += n5r(-300, -150, 1800, 180, 'fill="#0e0a06"');
    for (i = 0; i < 520; i++) { x = -300 + r() * 1800; s += n5p('M' + f1(x) + ',' + f1(-20 + r() * 44) + ' l' + f1((r() - .5) * 16) + ',' + f1(16 + r() * 34), 'stroke="' + mix('#6a5a2e', r() < .5 ? '#2a2210' : '#a89858', r() * .6) + '" stroke-width="' + f1(1 + r() * 1.4) + '" stroke-linecap="round"'); }
    s += n5r(-300, 30, 1800, 30, 'fill="#000" opacity=".5" filter="url(#src-blur8)"') + n5p('M-300,58 L640,-6 L640,22 L-300,90 Z', 'fill="#000" filter="url(#cm-beam)"') + n5p('M1500,40 L600,4 L600,30 L1500,72 Z', 'fill="#000" filter="url(#cm-beam)"') + n5p('M-300,58 L640,-6', 'stroke="' + t.lc + '" stroke-opacity=".25" stroke-width="1.6"');
    [[-10, 1], [1210, -1]].forEach(function (q) { d = 'M' + q[0] + ',20 q' + (q[1] * 20) + ',220 ' + (q[1] * -6) + ',460 h' + (q[1] * 40) + ' q' + (q[1] * 16) + ',-250 ' + (q[1] * 2) + ',-460 Z'; s += n5sh(d, q[1] * 8, 0, .4, 5) + n5p(d, 'fill="#000" filter="url(#cm-post)"'); });
    return s;
  };
  OPENINGS['bog-window'] = function (t) {
    var d = 'M812,200 C806,130 850,84 918,82 C986,80 1022,132 1018,196 C1016,262 976,302 908,300 C846,300 816,262 812,200 Z', r = rng(225), a = t.th.accent, s, fr, i;
    s = '<defs>' + lin('n5-bsk', 1, [[0, '#081418'], [.6, '#1c3a34'], [1, '#4a6a48']]) + '</defs>' + n5r(800, 76, 226, 232, 'fill="url(#n5-bsk)"') + mpStars(r, 26, 804, 80, 216, 90, true);
    // a low moon, the dead trees across it, the water under it
    s += n5c(946, 160, 70, 'fill="#e8f0c8" opacity=".25" filter="url(#src-blur8)"') + n5c(946, 160, 42, 'fill="#eef2d4"') + n5c(934, 150, 8, 'fill="#c8d0b0" opacity=".6"') + n5c(960, 172, 5, 'fill="#c8d0b0" opacity=".6"') + n5c(952, 142, 3.4, 'fill="#c8d0b0" opacity=".6"');
    s += n5coral(r, 852, 262, 44, -1.5, 4, '#060c0a', 8) + n5coral(r, 988, 262, 52, -1.75, 4, '#060c0a', 9) + n5coral(r, 920, 262, 26, -1.4, 3, '#0c1612', 4);
    s += n5r(800, 256, 226, 52, 'fill="#0a1a18"') + n5p(n5ridge(r, 800, 1026, 258, 6, 14, 262), 'fill="#060c0a"');
    for (i = 0; i < 10; i++) s += n5r(916 + (r() - .5) * 60, 262 + i * 4, 14 + r() * 30, 1.6, 'class="n5-shimmer" fill="#eef2d4" opacity=".5" style="animation-delay:-' + f1(r() * 3) + 's"');
    for (i = 0; i < 6; i++) s += n5p('M' + (812 + i * 9) + ',300 q' + ((i % 2) * 4 - 2) + ',-20 ' + ((i % 3) - 1) * 4 + ',-' + (34 + i * 3), 'stroke="#060c0a" stroke-width="1.6" fill="none"') + n5e(812 + i * 9 + ((i % 3) - 1) * 4, 300 - (38 + i * 3), 2.2, 7, 'fill="#1a120a"');
    // mist that moves, lights that should not be followed, and something watching from the water
    [0, 1, 2].forEach(function (j) { s += n5e(860 + j * 50, 244 + j * 8, 90, 10, 'class="n5-caust" fill="#c8e0d0" opacity=".3" filter="url(#src-blur5)" style="animation-duration:' + (11 + j * 3) + 's;animation-delay:-' + (j * 4) + 's"'); });
    [[850, 214, 0, 44], [984, 230, 4, -50], [904, 240, 8, 36]].forEach(function (q) { s += n5g('class="n5-flit" style="--fx:' + q[3] + 'px;animation-delay:-' + q[2] + 's;animation-duration:' + (12 + q[2]) + 's"', n5c(q[0], q[1], 12, 'class="n5-beat" fill="' + a + '" opacity=".5" filter="url(#src-blur3)"') + n5c(q[0], q[1], 3, 'fill="#f4ffe0"')); });
    s += n5e(880, 266, 12, 5, 'fill="#060c0a"') + n5g('class="n5-fb n5-blink" style="animation-duration:6s"', n5c(875, 262, 3, 'fill="#e8d040"') + n5c(885, 262, 3, 'fill="#e8d040"') + n5r(874, 260.6, 2, 2.8, 'fill="#060c0a"') + n5r(884, 260.6, 2, 2.8, 'fill="#060c0a"'));
    // the frame is bent green wood, with a web in one corner and its maker on a thread
    fr = n5p(d, 'fill="none" stroke="#0c0804" stroke-width="20" stroke-linejoin="round"') + n5p(d, 'fill="none" stroke="' + mix(t.th.wood, '#6a5030', .5) + '" stroke-width="13"') + n5p(d, 'fill="none" stroke="#fff" stroke-opacity=".16" stroke-width="2.4" transform="translate(-3,-3)"') + n5p(d, 'fill="none" stroke="#000" stroke-opacity=".5" stroke-width="13" stroke-dasharray="1 30"');
    [[826, 120, -2.4], [1004, 124, -.8], [1010, 262, .6], [822, 268, 2.6]].forEach(function (q) { fr += n5coral(r, q[0], q[1], 18, q[2], 2, mix(t.th.wood, '#6a5030', .5), 5); });
    fr += mpMossPad(r, 880, 84, 26, 7, '#3a6a2a') + mpMossPad(r, 960, 296, 30, 7, '#2e5a24');
    for (i = 0; i < 6; i++) fr += n5p('M1002,116 L' + f1(1002 - Math.cos(i * .3) * 70) + ',' + f1(116 + Math.sin(i * .3) * 70), 'stroke="#e8f0e8" stroke-opacity=".4" stroke-width=".8"');
    [20, 38, 56].forEach(function (R) { fr += n5p('M' + (1002 - R) + ',116 A' + R + ',' + R + ' 0 0 0 ' + f1(1002 - Math.cos(1.5) * R) + ',' + f1(116 + Math.sin(1.5) * R), 'stroke="#e8f0e8" stroke-opacity=".35" stroke-width=".8" fill="none"'); });
    fr += n5g('class="n5-hover" style="animation-duration:5s"', n5p('M966,138 V184', 'stroke="#e8f0e8" stroke-opacity=".5" stroke-width=".8"') + n5e(966, 190, 5, 6, 'fill="#0a0806"') + n5c(966, 183, 3, 'fill="#0a0806"') + n5p('M961,188 l-7,-4 M961,191 l-8,2 M971,188 l7,-4 M971,191 l8,2 M962,194 l-5,6 M970,194 l5,6', 'stroke="#0a0806" stroke-width="1.1"'));
    return mpOpen(d, s, fr);
  };
  DISPLAYS['crooked-shelves'] = function (t) {
    var r = rng(227), A = [-2.4, 1.8, -1.2], P = [], s = n5r(30, 90, 470, 370, 'fill="#000" opacity=".3" filter="url(#src-blur14)"'), a = t.th.accent;
    CM_ROWS.forEach(function (ry, j) {
      var tn = Math.tan(A[j] * Math.PI / 180), g, yl = ry + (40 - 261) * tn, yr = ry + (482 - 261) * tn, i;
      for (i = 0; i < 4; i++) P.push([CM_COLS[i], Math.round(ry + (CM_COLS[i] - 261) * tn)]);
      // each board hangs from two cords and rests on a forked stick
      s += n5p('M44,' + (j ? CM_ROWS[j - 1] + 14 : 40) + ' L44,' + f1(yl) + ' M478,' + (j ? CM_ROWS[j - 1] + 14 : 40) + ' L478,' + f1(yr), 'stroke="#8a7250" stroke-width="2.4"') + n5p('M44,' + (j ? CM_ROWS[j - 1] + 14 : 40) + ' L44,' + f1(yl) + ' M478,' + (j ? CM_ROWS[j - 1] + 14 : 40) + ' L478,' + f1(yr), 'stroke="#3a2c18" stroke-width="2.4" stroke-dasharray="2 4"');
      g = cmShelfBoard(t, ry, 26, 496) + n5p('M26,' + ry + ' l-8,5 l6,3 l-5,5 h7 Z M496,' + ry + ' l9,4 l-6,4 l5,4 h-8 Z', 'fill="#000" filter="url(#cm-beam)"');
      s += n5g('transform="rotate(' + A[j] + ' 261 ' + ry + ')"', g);
      // and from its underside hang things best not asked about
      if (j < 2) [151, 261, 371].forEach(function (hx, k) {
        var hy = ry + (hx - 261) * tn + 14, L = 22 + r() * 22, kind = (j * 3 + k) % 4;
        s += n5p('M' + hx + ',' + f1(hy) + ' v' + f1(L), 'stroke="#8a7250" stroke-width="1"');
        if (kind === 0) s += n5p('M' + (hx - 3) + ',' + f1(hy + L) + ' q-7,12 3,22 q10,-10 3,-22 Z', 'fill="#3a3a44"') + n5p('M' + hx + ',' + f1(hy + L) + ' v20', 'stroke="#8a8a94" stroke-width=".8"');
        else if (kind === 1) s += n5p('M' + (hx - 2) + ',' + f1(hy + L) + ' v16', 'stroke="#e4dcc4" stroke-width="4" stroke-linecap="round"') + n5c(hx - 4, hy + L, 2.4, 'fill="#e4dcc4"') + n5c(hx + 1, hy + L, 2.4, 'fill="#e4dcc4"') + n5c(hx - 4, hy + L + 16, 2.4, 'fill="#e4dcc4"') + n5c(hx + 1, hy + L + 16, 2.4, 'fill="#e4dcc4"');
        else if (kind === 2) s += n5c(hx, hy + L + 5, 9, 'fill="' + a + '" opacity=".35" filter="url(#src-blur3)"') + n5c(hx, hy + L + 5, 5, 'fill="' + mix(a, '#ffffff', .4) + '"') + n5c(hx - 1.6, hy + L + 3.4, 1.4, 'fill="#fff"');
        else s += n5p('M' + (hx - 6) + ',' + f1(hy + L + 14) + ' l2,-12 l4,-3 l4,3 l2,12 M' + hx + ',' + f1(hy + L) + ' v14 M' + (hx - 3) + ',' + f1(hy + L + 16) + ' v-9 M' + (hx + 3) + ',' + f1(hy + L + 16) + ' v-9', 'stroke="#d8c8a0" stroke-width="1.8" fill="none" stroke-linecap="round"');
      });
      s += n5p('M70,' + f1(ry + (70 - 261) * tn + 12) + ' l-10,40 M66,' + f1(ry + (70 - 261) * tn + 30) + ' l14,-16 M452,' + f1(ry + (452 - 261) * tn + 12) + ' l10,40 M456,' + f1(ry + (452 - 261) * tn + 30) + ' l-14,-16', 'stroke="#1a120a" stroke-width="5" fill="none" stroke-linecap="round"');
    });
    return { art: '<g filter="url(#cm-unitlit)">' + s + '</g>', slots: mpSlots(P, 1.2), zone: [34, 96, 454, 320], wallsh: true, decor: true, label: 'The crooked shelves' };
  };
  COUNTERS.driftwood = function (t) {
    var s = cmTop(t, 'M-300,452 H1500 V512 H-300 Z'), r = rng(229), g = '', x, w, d, i, a = t.th.accent;
    s += '<clipPath id="cm-topclip"><rect x="-300" y="452" width="1800" height="60"/></clipPath>';
    d = 'M-300,502'; for (x = -300; x < 1500; x += 50) d += ' q25,' + f1(4 + r() * 10) + ' 50,' + f1((r() - .5) * 4); d += ' V522 H-300 Z';
    s += n5r(-300, 512, 1800, 390, 'fill="#070503"');
    // whatever the bog gave up, stood on end and lashed together
    for (x = -300; x < 1500; x += w + 4) { w = 34 + r() * 60; var top = 514 + r() * 8; s += n5p('M' + f1(x) + ',' + f1(top) + ' l' + f1(w * .3) + ',' + f1(-3 + r() * 6) + ' l' + f1(w * .7) + ',' + f1(r() * 6) + ' V900 H' + f1(x) + ' Z', 'fill="#000" filter="url(#cm-front)"') + n5r(x, 514, w, 386, 'fill="' + (r() < .5 ? '#000' : '#c8d8b0') + '" opacity="' + f1(r() * .14) + '"') + (r() < .4 ? n5e(x + w * .5, 560 + r() * 120, w * .16, w * .26, 'fill="#070503" opacity=".8"') : ''); }
    s += n5p(d, 'fill="#000" filter="url(#cm-beam)"') + n5p(d.replace(' V522 H-300 Z', ''), 'stroke="' + t.lc + '" stroke-opacity=".35" stroke-width="1.6" fill="none"');
    [566, 664].forEach(function (y) { g += n5p(n5wav(-300, 1500, y + 4, 3, 220, y, 20), 'stroke="#000" stroke-opacity=".5" stroke-width="10" fill="none" filter="url(#src-blur3)"') + n5p(n5wav(-300, 1500, y, 3, 220, y, 20), 'stroke="#9a8256" stroke-width="8" fill="none"') + n5p(n5wav(-300, 1500, y, 3, 220, y, 20), 'stroke="#4a3a1e" stroke-width="8" fill="none" stroke-dasharray="2 7"'); });
    // moss over the edge, toadstools in the cracks, and a very small door
    for (i = 0; i < 16; i++) g += mpMossPad(r, -260 + r() * 1700, 512 + r() * 6, 30 + r() * 40, 8 + r() * 6, mix('#2e5a24', '#6a8a34', r()));
    g += n5toadstool(236, 660, 1.6, '#c8322a') + n5toadstool(262, 664, 1.1, '#c8322a') + n5toadstool(210, 664, .9, '#d8502a') + n5toadstool(860, 562, 1.2, '#8a5ac8') + n5toadstool(880, 564, .8, '#8a5ac8') + n5toadstool(1150, 662, 1.4, '#c8322a');
    g += n5p('M1010,700 V636 Q1010,604 1036,604 Q1062,604 1062,636 V700 Z', 'fill="#1c120a" stroke="#070503" stroke-width="3"') + n5p('M1036,604 V700 M1010,640 h52', 'stroke="#070503" stroke-width="1.6"') + n5c(1036, 630, 6, 'fill="' + a + '" opacity=".9"') + n5c(1036, 630, 14, 'fill="' + a + '" opacity=".3" filter="url(#src-blur3)"') + n5p('M1030,630 h12 M1036,624 v12', 'stroke="#070503" stroke-width="1.4"') + n5c(1054, 664, 2.6, 'fill="url(#cm-trim)"');
    return '<g filter="url(#cm-woodlit)">' + s + g + n5r(-300, 518, 1800, 30, 'fill="#000" opacity=".5" filter="url(#src-blur8)"') + '</g>';
  };
  LIGHTS.cauldron = function (t) {
    var x = t.lp.x, a = t.th.accent, al = mix(a, '#ffffff', .5), s, i;
    s = '<g data-k="lamp"><defs>' + n5Metals() + '</defs>' + n5e(x, 488, 56, 8, 'fill="url(#src-ao)"');
    for (i = 0; i < 5; i++) s += n5e(x + (i - 2) * 9, 426, 14, 10, 'class="n5-fb n5-puff" fill="' + mix(a, '#e8f0e0', .5) + '" filter="url(#src-blur3)" style="--fx:' + ((i % 2 ? 1 : -1) * (10 + i * 7)) + 'px;--fy:-' + (110 + i * 18) + 'px;animation-delay:-' + f1(i * 1.2) + 's"');
    // black iron on three feet, and whatever is in it never quite still
    s += n5p('M' + (x - 30) + ',478 l-8,12 M' + (x + 30) + ',478 l8,12 M' + x + ',484 v8', 'stroke="#0c0c0e" stroke-width="7" stroke-linecap="round"') + n5c(x, 440, 60, 'fill="url(#cm-glow)" opacity=".7" class="cm-pulse"') + n5p('M' + (x - 44) + ',436 Q' + (x - 58) + ',486 ' + x + ',488 Q' + (x + 58) + ',486 ' + (x + 44) + ',436 Z', 'fill="url(#n5-black)"') + n5e(x, 436, 46, 10, 'fill="#17181b"') + n5e(x, 436, 46, 10, 'fill="none" stroke="#5a5e66" stroke-width="1.4"') + n5e(x, 437, 39, 7.4, 'fill="' + a + '"') + n5e(x, 437, 39, 7.4, 'class="n5-beat" fill="' + al + '"');
    [[-22, 0], [-6, 1.1], [10, 2.3], [24, .6], [2, 3.2]].forEach(function (q) { s += n5c(x + q[0], 436, 3.4, 'class="n5-streak" fill="' + al + '" stroke="#fff" stroke-width=".8" style="--fy:-26px;animation-duration:2.6s;animation-delay:-' + q[1] + 's"'); });
    s += n5p('M' + (x + 16) + ',438 L' + (x + 50) + ',388', 'stroke="#5a4424" stroke-width="5" stroke-linecap="round"') + n5c(x - 46, 446, 6, 'fill="none" stroke="#2e3036" stroke-width="3"') + n5c(x + 46, 446, 6, 'fill="none" stroke="#2e3036" stroke-width="3"') + n5p('M' + (x - 34) + ',452 Q' + (x - 40) + ',474 ' + (x - 16) + ',482', 'stroke="' + a + '" stroke-opacity=".5" stroke-width="2.4" fill="none"');
    return { back: '', front: s + '<circle cx="' + x + '" cy="430" r="130" fill="url(#cm-bloom)" class="cm-pulse" opacity=".8"/></g>' };
  };
  PROPS.toad = function (t, p) {
    var x = p.x - 26, y = 494, c = '#5c7a34', cd = '#34481c', s = n5e(p.x, y + 3, 66, 9, 'fill="url(#src-ao)"'), i, r = rng(241);
    // a toad that has seen everything, its throat going in and out
    s += n5g('class="n5-fbb n5-breath" style="animation-duration:2.6s"', n5e(x + 12, y - 12, 18, 12, 'fill="#c8c88a"'));
    s += n5p('M' + (x - 32) + ',' + y + ' Q' + (x - 40) + ',' + (y - 30) + ' ' + (x - 10) + ',' + (y - 40) + ' Q' + (x + 18) + ',' + (y - 46) + ' ' + (x + 30) + ',' + (y - 28) + ' Q' + (x + 36) + ',' + (y - 14) + ' ' + (x + 22) + ',' + (y - 10) + ' Q' + (x + 4) + ',' + (y - 6) + ' ' + (x + 2) + ',' + y + ' Z', 'fill="' + c + '"') + n5p('M' + (x - 30) + ',' + y + ' q-4,-18 14,-22 q10,6 6,22 Z', 'fill="' + cd + '"') + n5p('M' + (x - 2) + ',' + y + ' l14,0 l5,3 M' + (x - 2) + ',' + y + ' q0,-12 8,-14', 'stroke="' + cd + '" stroke-width="4.4" fill="none" stroke-linecap="round"');
    for (i = 0; i < 12; i++) s += n5c(x - 26 + r() * 44, y - 34 + r() * 22, 1.4 + r() * 1.6, 'fill="' + cd + '" opacity=".8"');
    s += n5c(x + 14, y - 40, 7, 'fill="' + c + '"') + n5g('class="n5-fb n5-blink" style="animation-duration:7s"', n5c(x + 15, y - 41, 4.6, 'fill="#e8b830"') + n5e(x + 15, y - 41, 3.6, 1.2, 'fill="#0a0604"')) + n5p('M' + (x + 14) + ',' + (y - 22) + ' q12,2 18,-6', 'stroke="' + cd + '" stroke-width="1.6" fill="none"');
    // a skull kept for a candlestick
    x = p.x + 34;
    s += n5p('M' + (x - 18) + ',' + (y - 6) + ' Q' + (x - 24) + ',' + (y - 40) + ' ' + x + ',' + (y - 42) + ' Q' + (x + 24) + ',' + (y - 40) + ' ' + (x + 18) + ',' + (y - 6) + ' l-4,6 h-28 Z', 'fill="#d8d0b8"') + n5e(x - 8, y - 20, 5, 6, 'fill="#1a140c"') + n5e(x + 8, y - 20, 5, 6, 'fill="#1a140c"') + n5p('M' + x + ',' + (y - 13) + ' l-2.6,5 h5.2 Z', 'fill="#1a140c"') + n5p('M' + (x - 9) + ',' + (y - 4) + ' v5 M' + (x - 3) + ',' + (y - 4) + ' v5 M' + (x + 3) + ',' + (y - 4) + ' v5 M' + (x + 9) + ',' + (y - 4) + ' v5', 'stroke="#1a140c" stroke-width="1.2"') + n5p('M' + (x - 16) + ',' + (y - 30) + ' q4,-8 12,-9', 'stroke="#fff" stroke-opacity=".5" stroke-width="2" fill="none"');
    s += n5c(x, y - 62, 18, 'fill="url(#cm-glow)" opacity=".5" class="cm-pulse"') + n5p('M' + (x - 9) + ',' + (y - 40) + ' q9,-6 18,0 q-3,8 2,12 q-10,-4 -9,-6 q-4,6 -12,4 q4,-4 1,-10 Z', 'fill="#e8dcc0"') + cmCandle(x, y - 40, 14, true);
    return n5g('class="src-nohit"', s);
  };
  PROPS['bottle-tree'] = function (t, p) {
    var x = p.x, s, w = '#1a120a', cols = ['#4aa85a', '#d8a030', '#4a8ad8', '#a85ad8', '#d85a4a'], T = [[-48, 330], [-28, 284], [8, 262], [38, 296], [52, 344]];
    s = n5e(x + 4, 484, 44, 7, 'fill="url(#src-ao)"') + n5p('M' + (x - 22) + ',480 l4,-34 h36 l4,34 Z', 'fill="#7a4a2c"') + n5r(x - 22, 442, 44, 8, 'rx="3" fill="#8a5a36"') + n5p('M' + (x - 16) + ',478 l3,-28', 'stroke="#fff" stroke-opacity=".2" stroke-width="3"');
    // a dead branch stood in a pot, a bottle on every twig and something alight in each
    s += n5p('M' + x + ',446 C' + (x - 8) + ',400 ' + (x + 6) + ',360 ' + (x - 2) + ',320', 'stroke="' + w + '" stroke-width="8" fill="none" stroke-linecap="round"');
    T.forEach(function (q, j) { s += n5p('M' + (x - 2 + (j - 2)) + ',' + (360 - j * 8) + ' Q' + (x + q[0] * .5) + ',' + (q[1] + 10) + ' ' + (x + q[0]) + ',' + q[1], 'stroke="' + w + '" stroke-width="' + (4.4 - (j % 2)) + '" fill="none" stroke-linecap="round"'); });
    T.forEach(function (q, j) {
      var bx = x + q[0], by = q[1], L = 12 + (j % 3) * 8, c = cols[j];
      s += n5g('class="n5-vb n5-sway" style="transform-origin:' + bx + 'px ' + by + 'px;animation-duration:' + (4 + j * .7) + 's;animation-delay:-' + (j * 1.3) + 's"', n5p('M' + bx + ',' + by + ' v' + L, 'stroke="#8a7250" stroke-width="1"') + n5c(bx, by + L + 16, 14, 'class="n5-beat" fill="' + c + '" opacity=".45" filter="url(#src-blur3)" style="animation-delay:-' + j + 's"') + n5r(bx - 3, by + L, 6, 7, 'fill="' + mix(c, '#000', .4) + '"') + n5r(bx - 4, by + L - 2, 8, 3, 'fill="url(#src-cork)"') + n5r(bx - 7, by + L + 6, 14, 20, 'rx="5" fill="' + c + '" opacity=".75"') + n5r(bx - 7, by + L + 6, 14, 20, 'rx="5" fill="url(#src-glassR)"') + n5c(bx, by + L + 17, 2.6, 'class="n5-beat" fill="#fffbe0" style="animation-delay:-' + j + 's;animation-duration:' + (2.4 + j * .5) + 's"'));
    });
    return n5g('class="src-nohit"', s);
  };


  /* ---- The gearworks: riveted plate, a line shaft overhead, a great clock
     seen from behind, goods carried on turning cogs and a counter that is
     half an engine. ---- */
  CM_OPT.shape.push('works'); CM_OPT.wall.push('brass-plate'); CM_OPT.opening.push('clock-face'); CM_OPT.display.push('cogs'); CM_OPT.counter.push('engine');
  CM_WALLFQ['brass-plate'] = [.05, 1.6, 61]; CM_OPEN_INFO['clock-face'] = 'The great clock';
  n5light('filament', { base: '#ffd9a0', glow: '#ffb050', amb: '#2c2a36', reach: 1000, k: 1 });
  n5prop('gears', 'air', 'back'); n5prop('steam-puffs', 'over', 'over'); n5prop('automaton', 'tallR', 'front', 0, true); n5prop('gauge', 'cL', 'front', 120);

  // A toothed wheel about 0,0: a rim, spokes and a hub.
  function n5gear(R, n, col, dark, sp) {
    var d = '', i, p = 6.2832 / n, th = Math.max(5, R * .15), a, q = function (rad, ang) { return f1(Math.cos(ang) * rad) + ',' + f1(Math.sin(ang) * rad); }, s;
    for (i = 0; i < n; i++) { a = i * p; d += (i ? ' L' : 'M') + q(R - th, a) + ' L' + q(R, a + p * .14) + ' L' + q(R, a + p * .4) + ' L' + q(R - th, a + p * .54); }
    s = n5p(d + ' Z', 'fill="' + col + '"') + n5c(0, 0, R - th - R * .12, 'fill="' + dark + '"');
    for (i = 0; i < (sp || 4); i++) s += n5r(-R + th, -R * .08, (R - th) * 2, R * .16, 'fill="' + col + '" transform="rotate(' + (i * 180 / (sp || 4)) + ')"');
    return s + n5c(0, 0, R * .26, 'fill="' + col + '"') + n5c(0, 0, R * .1, 'fill="' + dark + '"') + n5p('M' + q(R - th - 2, -2.4) + ' A' + f1(R - th - 2) + ',' + f1(R - th - 2) + ' 0 0 1 ' + q(R - th - 2, -1.2), 'stroke="#fff" stroke-opacity=".35" stroke-width="1.6" fill="none"');
  }
  function n5rivets(x0, x1, y, st, c) { var s = '', x; for (x = x0; x <= x1; x += st) s += n5c(x, y, 3, 'fill="' + (c || '#2a2218') + '"') + n5c(x - .8, y - .8, 1.1, 'fill="#fff" opacity=".45"'); return s; }
  WALLS['brass-plate'] = function (t) {
    var r = rng(251), base = mix(t.th.trim, '#3a2c18', .55), s, x, y, i, g = '', row = 0;
    for (y = -150; y < 470; y += 124, row++) for (x = -300 - (row % 2) * 90; x < 1500; x += 180) { g += n5r(x + 1, y + 1, 178, 122, 'fill="' + mix(base, r() < .5 ? '#1a140c' : '#c8a868', r() * .22) + '"') + n5r(x + 1, y + 1, 178, 2, 'fill="#fff" opacity=".12"') + n5r(x + 1, y + 121, 178, 2, 'fill="#000" opacity=".4"') + n5r(x + 177, y + 1, 2, 122, 'fill="#000" opacity=".4"'); for (i = 0; i < 6; i++) g += n5c(x + 14 + i * 30, y + 10, 3, 'fill="#2a2014"') + n5c(x + 13, y + 9, 1, 'fill="#fff" opacity=".0"') + n5c(x + 14 + i * 30, y + 114, 3, 'fill="#2a2014"'); }
    s = '<g filter="url(#cm-wall)"><rect ' + FULL + ' fill="#1a140c"/>' + g + '</g>';
    // where the wet has got at it, and the soot above the pipes
    for (i = 0; i < 16; i++) { x = -260 + r() * 1720; y = -100 + r() * 300; s += n5r(x, y, 6 + r() * 14, 80 + r() * 220, 'fill="#3a9a8a" opacity="' + f1(.08 + r() * .1) + '" filter="url(#src-blur5)"'); }
    s += '<rect ' + FULL + ' fill="#0a0806" filter="url(#cm-stain)" opacity=".5"/><defs>' + lin('n5-pipe', 1, [[0, '#3a1608'], [.2, '#c06a34'], [.36, '#ffcfa0'], [.55, '#a8522a'], [1, '#2a1006']]) + lin('n5-pipev', 0, [[0, '#3a1608'], [.2, '#c06a34'], [.36, '#ffcfa0'], [.55, '#a8522a'], [1, '#2a1006']]) + '</defs>';
    // copper pipes with their flanges, and a wheel valve
    s += n5r(-300, 118, 1800, 9, 'fill="#000" opacity=".45" filter="url(#src-blur3)" transform="translate(0,12)"') + n5r(-300, 112, 1800, 16, 'fill="url(#n5-pipe)"') + n5r(-300, 380, 1800, 12, 'fill="url(#n5-pipe)"') + n5r(752, 112, 16, 280, 'fill="url(#n5-pipev)"') + n5r(1136, 120, 14, 266, 'fill="url(#n5-pipev)"');
    for (x = -260; x < 1500; x += 196) s += n5r(x, 108, 10, 24, 'rx="2" fill="url(#n5-pipe)"') + n5r(x + 40, 377, 8, 18, 'rx="2" fill="url(#n5-pipe)"');
    s += n5g('transform="translate(760,250)"', n5c(3, 4, 22, 'fill="#000" opacity=".4" filter="url(#src-blur3)"') + n5c(0, 0, 20, 'fill="none" stroke="#a8322a" stroke-width="5"') + n5p('M-20,0 H20 M0,-20 V20 M-14,-14 L14,14 M-14,14 L14,-14', 'stroke="#a8322a" stroke-width="3"') + n5c(0, 0, 5, 'fill="#3a2c18"'));
    return s;
  };
  SHAPES.works = function (t) {
    var s = WALLS[t.th.wall](t), x, i, tr = t.th.trim, dk = mix(tr, '#000', .55);
    s += '<defs>' + n5Metals() + '</defs>';
    // a riveted girder, and under it the shaft that drives everything, with its drums and belts
    s += n5r(-300, -150, 1800, 172, 'fill="#17181b"') + n5r(-300, 0, 1800, 22, 'fill="url(#n5-black)"') + n5rivets(-290, 1500, 11, 34, '#0a0a0c') + n5r(-300, 22, 1800, 14, 'fill="#000" opacity=".55" filter="url(#src-blur5)"');
    s += n5r(-300, 44, 1800, 9, 'rx="4" fill="url(#n5-pewter)"');
    [100, 380, 720, 1060, 1180].forEach(function (px, j) { var w = 26 + (j % 2) * 12, R = 20 + (j % 3) * 5; s += n5r(px - w / 2, 48 - R, w, R * 2, 'rx="4" fill="' + dk + '"') + n5r(px - w / 2, 48 - R, w, 4, 'fill="#fff" opacity=".2"') + n5r(px - w / 2 + 4, 48 - R, w - 8, R * 2, 'fill="#000" opacity=".25"') + n5p('M' + (px - w / 2 + 5) + ',' + (48 - R) + ' V-150 M' + (px + w / 2 - 5) + ',' + (48 - R) + ' V-150', 'stroke="#3a2a1a" stroke-width="5"'); });
    for (x = -280; x < 1500; x += 220) s += n5p('M' + x + ',22 v14 h-9 v20 h34 v-20 h-9 v-14 Z', 'fill="url(#n5-black)"');
    // iron stanchions at the sides
    [[-8, 34], [1174, 34]].forEach(function (q) { s += n5r(q[0] + q[1], 22, 12, 452, 'fill="#000" opacity=".4" filter="url(#src-blur5)"') + n5r(q[0], 22, q[1], 452, 'fill="url(#n5-black)"'); for (i = 40; i < 470; i += 46) s += n5c(q[0] + 8, i, 3, 'fill="#0a0a0c"') + n5c(q[0] + q[1] - 8, i, 3, 'fill="#0a0a0c"'); });
    return s;
  };
  OPENINGS['clock-face'] = function (t) {
    var cx = 912, cy = 190, a = t.th.accent, tr = t.th.trim, ink = '#1a120a', s, fr, i, ang, big;
    s = '<defs>' + rad('n5-dial', [[0, '#fff6dc'], [.6, '#f0d8a0'], [1, mix(a, '#8a5a1a', .5)]]) + '</defs>' + n5c(cx, cy, 112, 'fill="url(#n5-dial)"');
    // the wheels of the movement, seen through the glass as shadows
    s += n5g('opacity=".2"', n5g('transform="translate(' + (cx - 34) + ',' + (cy + 26) + ')"', n5g('class="n5-turn" style="animation-duration:40s"', n5gear(58, 22, ink, '#f0d8a0', 5))) + n5g('transform="translate(' + (cx + 50) + ',' + (cy - 30) + ')"', n5g('class="n5-turn" style="animation-duration:22s;animation-direction:reverse"', n5gear(36, 14, ink, '#f0d8a0', 4))) + n5g('transform="translate(' + (cx + 44) + ',' + (cy + 54) + ')"', n5g('class="n5-turn" style="animation-duration:14s"', n5gear(24, 10, ink, '#f0d8a0', 3))));
    for (i = 0; i < 60; i++) s += n5r(cx - (i % 5 ? .8 : 2.4), cy - 104, i % 5 ? 1.6 : 4.8, i % 5 ? 7 : 16, 'fill="' + ink + '" transform="rotate(' + (i * 6) + ' ' + cx + ' ' + cy + ')"');
    ['XII', 'III', 'VI', 'IX'].forEach(function (n, j) { ang = j * Math.PI / 2 - Math.PI / 2; s += '<text x="' + f1(cx + Math.cos(ang) * 74) + '" y="' + f1(cy + Math.sin(ang) * 74 + 8) + '" text-anchor="middle" font-family="Georgia,serif" font-weight="700" font-size="22" fill="' + ink + '">' + n + '</text>'; });
    s += n5c(cx, cy, 86, 'fill="none" stroke="' + ink + '" stroke-width="1.4"') + n5c(cx, cy, 107, 'fill="none" stroke="' + ink + '" stroke-width="2"');
    // the hands: the hour hand crawls, the minute hand goes round, the third one ticks
    s += n5g('class="n5-vb n5-turn" style="transform-origin:' + cx + 'px ' + cy + 'px;animation-duration:1440s;animation-delay:-420s"', n5p('M' + (cx - 5) + ',' + (cy + 12) + ' L' + (cx - 3) + ',' + (cy - 34) + ' Q' + (cx - 10) + ',' + (cy - 44) + ' ' + cx + ',' + (cy - 58) + ' Q' + (cx + 10) + ',' + (cy - 44) + ' ' + (cx + 3) + ',' + (cy - 34) + ' L' + (cx + 5) + ',' + (cy + 12) + ' Z', 'fill="' + ink + '"'));
    s += n5g('class="n5-vb n5-turn" style="transform-origin:' + cx + 'px ' + cy + 'px;animation-duration:120s;animation-delay:-20s"', n5p('M' + (cx - 3.4) + ',' + (cy + 16) + ' L' + (cx - 2) + ',' + (cy - 60) + ' Q' + (cx - 6) + ',' + (cy - 70) + ' ' + cx + ',' + (cy - 94) + ' Q' + (cx + 6) + ',' + (cy - 70) + ' ' + (cx + 2) + ',' + (cy - 60) + ' L' + (cx + 3.4) + ',' + (cy + 16) + ' Z', 'fill="' + ink + '"'));
    s += n5g('class="n5-vb n5-turn" style="transform-origin:' + cx + 'px ' + cy + 'px;animation-duration:60s;animation-timing-function:steps(60)"', n5p('M' + cx + ',' + (cy + 22) + ' V' + (cy - 98), 'stroke="#a8322a" stroke-width="1.6"') + n5c(cx, cy + 22, 4, 'fill="#a8322a"')) + n5c(cx, cy, 7, 'fill="' + tr + '"') + n5c(cx, cy, 3, 'fill="' + ink + '"');
    s += n5p('M830,170 A90,90 0 0 1 900,104', 'stroke="#fff" stroke-opacity=".5" stroke-width="10" fill="none" stroke-linecap="round"');
    fr = n5p(RING, 'fill="none" stroke="#17181b" stroke-width="24"') + n5p(RING, 'fill="none" stroke="url(#cm-trim)" stroke-width="14"') + n5p(RING, 'fill="none" stroke="#000" stroke-opacity=".5" stroke-width="2"') + n5c(cx, cy, 120, 'fill="none" stroke="#000" stroke-opacity=".5" stroke-width="2"');
    for (i = 0; i < 16; i++) fr += n5c(cx + Math.cos(i * .3927) * 108, cy + Math.sin(i * .3927) * 108, 3.4, 'fill="#3a2c18"') + n5c(cx + Math.cos(i * .3927) * 108 - 1, cy + Math.sin(i * .3927) * 108 - 1, 1.1, 'fill="#fff" opacity=".6"');
    return mpOpen(RING, s, fr, n5g('opacity=".12" pointer-events="none"', n5c(cx, cy, 190, 'fill="' + a + '" filter="url(#src-blur14)" class="mp-breathe"')));
  };
  DISPLAYS.cogs = function (t) {
    var r = rng(253), tr = t.th.trim, c1 = mix(tr, '#1a140c', .6), c2 = mix(tr, '#1a140c', .78), s, i;
    s = '<defs>' + n5Metals() + '<clipPath id="n5-cogc"><rect x="30" y="68" width="466" height="390"/></clipPath></defs>' + n5r(36, 74, 470, 392, 'fill="#000" opacity=".5" filter="url(#src-blur8)"') + n5r(30, 68, 466, 390, 'fill="#0c0a08"');
    // the train behind the shelves: big wheels, meshed, stopped where they stood
    s += '<g clip-path="url(#n5-cogc)">';
    [[120, 170, 96, 30, 6], [300, 140, 70, 22, 5], [430, 250, 84, 26, 5], [220, 330, 78, 24, 4], [70, 400, 60, 18, 4], [380, 420, 66, 20, 5], [470, 100, 44, 14, 3]].forEach(function (q, j) { s += n5g('transform="translate(' + q[0] + ',' + q[1] + ') rotate(' + f1(r() * 30) + ')"', n5g('transform="translate(4,5)" opacity=".6"', n5gear(q[2], q[3], '#000', '#000', q[4])) + n5gear(q[2], q[3], j % 2 ? c1 : mix(c1, tr, .2), c2, q[4])); });
    s += n5r(30, 68, 466, 390, 'fill="#000" opacity=".3"') + '</g>';
    // iron ledges, flanged and riveted, and the frame they are bolted to
    CM_ROWS.forEach(function (ry) { s += n5r(34, ry, 458, 5, 'fill="url(#n5-pewter)"') + n5r(34, ry + 5, 458, 9, 'fill="#17181b"') + n5rivets(46, 486, ry + 9.5, 40, '#0a0a0c') + n5r(34, ry + 14, 458, 12, 'fill="#000" opacity=".5" filter="url(#src-blur3)"'); });
    s += n5r(24, 62, 16, 396, 'fill="url(#n5-black)"') + n5r(486, 62, 16, 396, 'fill="url(#n5-black)"') + n5r(20, 58, 486, 16, 'fill="#17181b"') + n5r(20, 58, 486, 2, 'fill="#9aa0a8" opacity=".5"') + n5rivets(34, 496, 66, 42, '#0a0a0c');
    return { art: '<g filter="url(#cm-unitlit)">' + s + '</g>', slots: cmGrid(), zone: [34, 96, 454, 320], wallsh: false, decor: true, label: 'The cog shelves',
      // behind each good a small bright wheel goes round, one way then the other along the row
      base: function (it, p) { var k = Math.round(p[0] / 110) % 2; return n5g('transform="translate(' + p[0] + ',' + (p[1] - 40) + ')" opacity=".9"', n5g('class="n5-turn" style="animation-duration:' + (k ? 16 : 22) + 's;animation-direction:' + (k ? 'reverse' : 'normal') + '"', n5gear(38, 14, mix(tr, '#000', .25), '#14100a', 4))); } };
  };
  COUNTERS.engine = function (t) {
    var r = rng(255), tr = t.th.trim, s, g = '', x, y, a = t.th.accent;
    s = '<defs>' + n5Metals() + mp3Defs() + lin('n5-pipe', 1, [[0, '#3a1608'], [.2, '#c06a34'], [.36, '#ffcfa0'], [.55, '#a8522a'], [1, '#2a1006']]) + lin('n5-stl', 1, [[0, '#9aa2ac'], [1, '#4a5058']]) + '</defs><clipPath id="cm-topclip"><rect x="-300" y="452" width="1800" height="58"/></clipPath>';
    // a top of rolled plate with a brass edge
    s += n5r(-300, 452, 1800, 54, 'fill="url(#n5-stl)" filter="url(#mp-fine)"') + n5r(-300, 500, 1800, 12, 'fill="url(#cm-trimv)"') + n5r(-300, 500, 1800, 2, 'fill="#fff" opacity=".5"') + n5rivets(-290, 1500, 506, 30, '#5a4218');
    // the boiler front: plates, a row of fire doors with the fire showing through the bars
    s += n5r(-300, 512, 1800, 390, 'fill="#1c1d21" filter="url(#mp-fine)"');
    for (x = -300; x < 1500; x += 200) g += n5r(x, 512, 2, 390, 'fill="#000" opacity=".7"') + n5r(x + 2, 512, 1.4, 390, 'fill="#fff" opacity=".08"') + [20, 180].map(function (o) { var q = ''; for (y = 528; y < 900; y += 30) q += n5c(x + o, y, 3, 'fill="#0a0a0c"') + n5c(x + o - .8, y - .8, 1, 'fill="#fff" opacity=".3"'); return q; }).join('');
    for (x = -200; x < 1500; x += 400) { g += n5r(x - 6, 596, 132, 92, 'rx="8" fill="#000" opacity=".5" filter="url(#src-blur5)"') + n5r(x, 590, 120, 90, 'rx="8" fill="#0c0c0e" stroke="#3a3c42" stroke-width="3"') + n5r(x + 12, 602, 96, 66, 'rx="4" fill="#ff8a30"') + n5r(x + 12, 636, 96, 32, 'rx="4" fill="#ffd27a" opacity=".7"'); for (y = 0; y < 8; y++) g += n5r(x + 12 + y * 12.6, 600, 7, 70, 'fill="#17181b"'); g += n5r(x + 54, 580, 12, 14, 'rx="2" fill="url(#cm-trim)"') + n5e(x + 60, 640, 90, 60, 'fill="' + a + '" opacity=".12" filter="url(#src-blur8)"'); }
    // a main along the top with three dials on it
    g += n5r(-300, 534, 1800, 12, 'fill="#000" opacity=".5" filter="url(#src-blur3)" transform="translate(0,10)"') + n5r(-300, 530, 1800, 18, 'fill="url(#n5-pipe)"');
    for (x = -240; x < 1500; x += 160) g += n5r(x, 526, 10, 26, 'rx="2" fill="url(#n5-pipe)"');
    [300, 640, 980].forEach(function (gx, j) { g += n5r(gx - 4, 548, 8, 14, 'fill="url(#cm-trim)"') + n5c(gx + 3, 580, 24, 'fill="#000" opacity=".5" filter="url(#src-blur3)"') + n5c(gx, 576, 22, 'fill="url(#cm-trim)"') + n5c(gx, 576, 17, 'fill="#f4ecd8"'); for (y = 0; y < 9; y++) g += n5r(gx - .6, 561, 1.2, 4, 'fill="#1a120a" transform="rotate(' + (-120 + y * 30) + ' ' + gx + ' 576)"'); g += n5p('M' + gx + ',576 l' + [-9, 2, 10][j] + ',' + [-9, -13, -7][j], 'stroke="#a8322a" stroke-width="1.8" stroke-linecap="round"') + n5c(gx, 576, 2.4, 'fill="#1a120a"') + n5p('M' + (gx - 12) + ',568 q6,-8 14,-8', 'stroke="#fff" stroke-opacity=".6" stroke-width="1.6" fill="none"'); });
    return s + g + n5r(-300, 512, 1800, 26, 'fill="#000" opacity=".5" filter="url(#src-blur8)"');
  };
  LIGHTS.filament = function (t) {
    var x = t.lp.x, s = '<g data-k="lamp">' + n5e(x, 482, 36, 6, 'fill="url(#src-ao)"'), i;
    // a glass bulb on a brass column, the wire in it white hot
    s += n5p('M' + (x - 24) + ',480 h48 l-8,-10 h-32 Z', 'fill="url(#cm-trim)"') + n5r(x - 5, 420, 10, 52, 'fill="url(#cm-trim)"') + n5e(x, 446, 9, 4, 'fill="url(#cm-trim)"') + n5r(x - 10, 410, 20, 12, 'rx="2" fill="url(#cm-trim)"') + n5p('M' + (x - 10) + ',414 h20 M' + (x - 10) + ',418 h20', 'stroke="#000" stroke-opacity=".4" stroke-width="1"');
    s += n5c(x, 376, 54, 'fill="url(#cm-glow)" opacity=".9" class="cm-pulse"') + n5p('M' + (x - 9) + ',410 Q' + (x - 30) + ',386 ' + (x - 24) + ',364 A24,24 0 0 1 ' + (x + 24) + ',364 Q' + (x + 30) + ',386 ' + (x + 9) + ',410 Z', 'fill="#fff0c8" opacity=".3"') + n5p('M' + (x - 9) + ',410 Q' + (x - 30) + ',386 ' + (x - 24) + ',364 A24,24 0 0 1 ' + (x + 24) + ',364 Q' + (x + 30) + ',386 ' + (x + 9) + ',410 Z', 'fill="none" stroke="#fff" stroke-opacity=".5" stroke-width="1.2"');
    s += n5p('M' + (x - 5) + ',408 V384 l3,-12 l3,12 l3,-12 l3,12 l3,-12 V408', 'class="n5-beat" stroke="#fff6d0" stroke-width="2" fill="none" stroke-linejoin="round" style="animation-duration:1.8s"') + n5p('M' + (x - 5) + ',408 V384 l3,-12 l3,12 l3,-12 l3,12 l3,-12 V408', 'stroke="#ffb040" stroke-width="5" stroke-opacity=".5" fill="none" filter="url(#src-blur3)"') + n5p('M' + (x - 16) + ',366 q2,-14 14,-18', 'stroke="#fff" stroke-opacity=".7" stroke-width="2" fill="none" stroke-linecap="round"');
    return { back: '', front: s + '<circle cx="' + x + '" cy="380" r="130" fill="url(#cm-bloom)" class="cm-pulse" opacity=".8"/></g>' };
  };
  PROPS.gears = function (t, p) {
    var tr = t.th.trim, s = '', c = mix(tr, '#000', .2), d = mix(tr, '#000', .7);
    // two trains on the wall, each wheel driving the next the other way
    [[1108, 366, 50, 20, 30, 0, 5], [1172, 300, 30, 12, 18, 1, 4], [1160, 414, 22, 9, 13.5, 1, 3], [536, 392, 40, 16, 26, 0, 4], [520, 338, 20, 8, 13, 1, 3], [590, 432, 24, 10, 16.25, 1, 3]].forEach(function (q) {
      s += n5g('transform="translate(' + q[0] + ',' + q[1] + ')"', n5g('transform="translate(4,5)" opacity=".5" filter="url(#src-blur3)"', n5c(0, 0, q[2], 'fill="#000"')) + n5g('class="n5-turn" style="animation-duration:' + q[4] + 's;animation-direction:' + (q[5] ? 'reverse' : 'normal') + '"', n5gear(q[2], q[3], c, d, q[6])));
    });
    return n5g('class="src-nohit"', s);
  };
  PROPS['steam-puffs'] = function (t, p) {
    var s = '', i;
    [[760, 112, -30], [1140, 124, 40], [300, 112, 20], [600, 520, -20]].forEach(function (q, j) { for (i = 0; i < 3; i++) s += n5e(q[0], q[1], 16, 12, 'class="n5-fb n5-puff" fill="#f0f0f4" filter="url(#src-blur5)" style="--fx:' + (q[2] + i * 10) + 'px;--fy:-' + (90 + i * 30) + 'px;animation-duration:' + (7 + j) + 's;animation-delay:-' + f1(j * 1.9 + i * .5) + 's"'); });
    return n5g('class="src-nohit" opacity=".8"', s);
  };
  PROPS.automaton = function (t, p) {
    var x = p.x, s, a = t.th.accent, hd;
    s = '<defs>' + n5Metals() + '</defs>' + n5e(x + 4, 484, 48, 8, 'fill="url(#src-ao)"');
    // a clockwork clerk: a drum of a body with a wheel showing in its chest, and a key in its back that never runs down
    s += n5g('transform="translate(' + (x + 46) + ',400)"', n5g('class="n5-fb n5-flap" style="animation-duration:1.8s"', n5p('M0,-3 h14 M14,0 a9,11 0 1 0 .1,0 Z', 'stroke="url(#cm-trim)" stroke-width="3.4" fill="none"')));
    s += n5r(x - 38, 470, 76, 10, 'rx="3" fill="url(#n5-black)"') + n5p('M' + (x - 34) + ',470 V370 Q' + (x - 34) + ',350 ' + (x - 14) + ',350 H' + (x + 14) + ' Q' + (x + 34) + ',350 ' + (x + 34) + ',370 V470 Z', 'fill="url(#cm-trim)"') + n5p('M' + (x + 12) + ',350 Q' + (x + 34) + ',350 ' + (x + 34) + ',370 V470 H' + (x + 14) + ' Z', 'fill="#000" opacity=".22"') + n5rivets(x - 26, x + 26, 362, 13, '#5a4218') + n5rivets(x - 26, x + 26, 460, 13, '#5a4218');
    s += n5c(x, 412, 24, 'fill="#14100a"') + n5g('transform="translate(' + x + ',412)"', n5g('class="n5-turn" style="animation-duration:9s"', n5gear(20, 10, mix(t.th.trim, '#fff', .15), '#14100a', 3))) + n5c(x, 412, 24, 'fill="none" stroke="#5a4218" stroke-width="3"') + n5p('M' + (x - 16) + ',400 q6,-10 18,-11', 'stroke="#fff" stroke-opacity=".5" stroke-width="2" fill="none"');
    s += n5p('M' + (x - 34) + ',386 h-12 v44 M' + (x + 34) + ',386 h8 v30', 'stroke="url(#n5-pewter)" stroke-width="6" fill="none" stroke-linecap="round" stroke-linejoin="round"') + n5c(x - 46, 432, 6, 'fill="url(#cm-trim)"');
    hd = n5r(x - 6, 336, 12, 16, 'fill="url(#n5-pewter)"') + n5r(x - 28, 284, 56, 54, 'rx="16" fill="url(#cm-trim)"') + n5r(x + 8, 284, 20, 54, 'rx="10" fill="#000" opacity=".2"') + n5c(x - 6, 308, 14, 'fill="#14100a"') + n5c(x - 6, 308, 10, 'class="n5-beat" fill="' + a + '"') + n5c(x - 6, 308, 4, 'fill="#fffbe6"') + n5c(x - 6, 308, 14, 'fill="none" stroke="url(#n5-pewter)" stroke-width="3"') + n5c(x + 15, 304, 5, 'fill="#14100a"') + n5c(x + 15, 304, 2, 'fill="' + a + '"') + n5p('M' + (x - 16) + ',328 h30', 'stroke="#14100a" stroke-width="3" stroke-dasharray="3 2"') + n5p('M' + x + ',284 v-18', 'stroke="url(#n5-pewter)" stroke-width="2.4"') + n5c(x, 264, 4, 'class="n5-beat" fill="' + a + '" style="animation-duration:1.4s"') + n5rivets(x - 20, x + 20, 290, 10, '#5a4218');
    s += n5g('class="n5-vb n5-tilt" style="transform-origin:' + x + 'px 340px"', hd);
    return n5g('class="src-nohit"', s);
  };
  PROPS.gauge = function (t, p) {
    var x = p.x, y = 492, s = n5e(x, y + 4, 56, 8, 'fill="url(#src-ao)"'), i;
    // a dial on a stand whose needle will not settle, and an oil can
    s += n5p('M' + (x - 40) + ',' + y + ' h44 l-6,-8 h-32 Z', 'fill="url(#cm-trim)"') + n5r(x - 22, y - 24, 8, 18, 'fill="url(#cm-trim)"') + n5c(x - 18, y - 48, 28, 'fill="url(#cm-trim)"') + n5c(x - 18, y - 48, 22, 'fill="#f4ecd8"');
    for (i = 0; i < 9; i++) s += n5r(x - 18.6, y - 68, 1.2, 5, 'fill="#1a120a" transform="rotate(' + (-120 + i * 30) + ' ' + (x - 18) + ' ' + (y - 48) + ')"');
    s += n5p('M' + (x - 2) + ',' + (y - 62) + ' A21,21 0 0 1 ' + (x + 1) + ',' + (y - 40), 'stroke="#c8283a" stroke-width="3" fill="none"') + n5g('class="n5-vb n5-tick" style="transform-origin:' + (x - 18) + 'px ' + (y - 48) + 'px;animation-duration:1.2s"', n5p('M' + (x - 18) + ',' + (y - 48) + ' l10,-14', 'stroke="#1a120a" stroke-width="2" stroke-linecap="round"')) + n5c(x - 18, y - 48, 3, 'fill="#1a120a"') + n5p('M' + (x - 32) + ',' + (y - 58) + ' q6,-9 16,-10', 'stroke="#fff" stroke-opacity=".6" stroke-width="2" fill="none"');
    s += n5p('M' + (x + 22) + ',' + y + ' v-22 q0,-8 10,-8 h8 q10,0 10,8 v22 Z', 'fill="url(#cm-trim)"') + n5p('M' + (x + 36) + ',' + (y - 30) + ' v-10 l22,-16', 'stroke="url(#cm-trim)" stroke-width="3" fill="none" stroke-linejoin="round"') + n5c(x + 59, y - 54, 1.6, 'class="cm-drip" fill="#3a2c10" style="animation-duration:3s"') + n5p('M' + (x + 26) + ',' + (y - 20) + ' v16', 'stroke="#fff" stroke-opacity=".4" stroke-width="2"');
    return n5g('class="src-nohit"', s);
  };

  /* ---- The hoard: a dragon's cave. The goods lie on its gold, the counter
     is a drift of coin, and through a gap in the rock the owner has one eye
     on you. ---- */
  CM_OPT.wall.push('lair'); CM_OPT.opening.push('dragon-eye'); CM_OPT.display.push('hoard'); CM_OPT.counter.push('coin-drift');
  CM_WALLFQ.lair = [.022, 4, 71]; CM_OPEN_INFO['dragon-eye'] = 'The gap in the rock';
  n5prop('glints', 'over', 'over'); n5prop('dragon-tail', 'cL', 'front', 230);

  // Coin lying on a slope: thin ovals, each with a lit edge, a gem here and there.
  function n5coins(r, pts, n, sp) {
    var s = '', i, q, x, y, c, G = ['#ffe488', '#f0c040', '#d89a20', '#b87a14'];
    for (i = 0; i < n; i++) { q = pts[Math.floor(r() * pts.length)]; x = q[0] + (r() - .5) * sp; y = q[1] + r() * (q[2] || 10); c = G[Math.floor(r() * 4)];
      if (r() < .04) s += n5p('M' + f1(x) + ',' + f1(y - 5) + ' l5,5 l-5,6 l-5,-6 Z', 'fill="' + ['#d8283a', '#2a6ad8', '#2aa85a', '#a83ad8'][Math.floor(r() * 4)] + '"') + n5p('M' + f1(x) + ',' + f1(y - 5) + ' l-5,5 h10 Z', 'fill="#fff" opacity=".4"');
      else s += n5e(x, y + 1.2, 6, 2.6, 'fill="#5a3a08"') + n5e(x, y, 6, 2.6, 'fill="' + c + '"') + n5p('M' + f1(x - 5) + ',' + f1(y - .6) + ' q5,-3 10,0', 'stroke="#fff8c8" stroke-opacity=".7" stroke-width=".8" fill="none"'); }
    return s;
  }
  function n5mound(r, x0, x1, y, h, seed) {
    var pts = [[x0, y]], x, u; for (x = x0; x <= x1; x += 14) { u = (x - x0) / (x1 - x0); pts.push([x, y - Math.sin(u * 3.1416) * h * (.8 + r() * .3)]); } pts.push([x1, y]);
    return pts;
  }
  WALLS.lair = function (t) {
    var r = rng(271), s, i, x, y, col, g = '', m, k;
    g = '<rect ' + FULL + ' fill="#3a2c22"/>';
    for (i = 0; i < 170; i++) { x = -300 + r() * 1800; y = -150 + r() * 620; col = mix('#3a2c22', r() < .55 ? '#120c08' : '#7a6248', r() * .3); g += n5p(mpLump(r, x, y, 26 + r() * 70, 9), 'fill="' + col + '"'); }
    s = '<defs>' + lin('n5-gold', 1, [[0, '#f0c040'], [.5, '#a87414'], [1, '#3a2406']]) + '</defs><g filter="url(#cm-wall)">' + g + '</g>';
    // scorch, and where claws have been dragged down the stone
    for (i = 0; i < 6; i++) s += n5e(-200 + r() * 1600, 40 + r() * 300, 90 + r() * 120, 60 + r() * 60, 'fill="#000" opacity=".35" filter="url(#src-blur14)"');
    [[560, 90], [1090, 110], [200, 60]].forEach(function (q) { for (k = 0; k < 3; k++) s += n5p('M' + (q[0] + k * 22) + ',' + q[1] + ' q-14,70 -40,150', 'stroke="#0a0604" stroke-width="5" fill="none" stroke-linecap="round"') + n5p('M' + (q[0] + k * 22 + 3) + ',' + (q[1] + 4) + ' q-14,70 -40,146', 'stroke="#c8a880" stroke-opacity=".2" stroke-width="1.4" fill="none"'); });
    // gold banked against the back of the cave
    [[-200, 260, 140], [240, 620, 110], [700, 1060, 150], [980, 1500, 190]].forEach(function (q) {
      m = n5mound(r, q[0], q[1], 472, q[2]);
      s += n5p('M' + m.map(function (v) { return f1(v[0]) + ',' + f1(v[1]); }).join(' L') + ' Z', 'fill="url(#n5-gold)"') + n5coins(r, m.slice(1, -1).map(function (v) { return [v[0], v[1], 472 - v[1]]; }), 420, 20);
    });
    s += n5r(-300, 300, 1800, 180, 'fill="#ffb030" opacity=".1" filter="url(#src-blur14)"');
    return s;
  };
  OPENINGS['dragon-eye'] = function (t) {
    var d = 'M802,196 Q858,98 932,102 Q1004,106 1030,196 Q996,284 920,288 Q846,288 802,196 Z', r = rng(273), cx = 916, cy = 194, s, fr, i, a, sc = '', eye;
    s = '<defs>' + rad('n5-iris', [[0, '#fff4a0'], [.3, '#ffb020'], [.7, '#d8500a'], [1, '#5a1404']]) + '<clipPath id="n5-eyec"><ellipse cx="' + cx + '" cy="' + cy + '" rx="84" ry="62"/></clipPath></defs>' + n5r(790, 90, 250, 210, 'fill="#1a0a06"');
    // hide the colour of old embers, plated
    for (i = 0; i < 90; i++) { var sx = 790 + (i % 15) * 18 + (Math.floor(i / 15) % 2) * 9, sy = 92 + Math.floor(i / 15) * 34; sc += n5p('M' + sx + ',' + sy + ' q9,-12 18,0 q-2,20 -9,28 q-7,-8 -9,-28 Z', 'fill="' + mix('#5a1a10', '#2a0a06', r()) + '" stroke="#0a0302" stroke-width="1.2"') + n5p('M' + (sx + 3) + ',' + (sy + 2) + ' q5,-8 11,-2', 'stroke="#ff9a50" stroke-opacity=".25" stroke-width="1" fill="none"'); }
    s += sc + n5e(cx, cy, 104, 80, 'fill="#000" opacity=".55" filter="url(#src-blur8)"');
    // the eye: it looks about, the pupil breathes, and now and then it shuts
    eye = n5e(cx, cy, 84, 62, 'fill="url(#n5-iris)"');
    for (i = 0; i < 40; i++) { a = i / 40 * 6.283; eye += n5p('M' + f1(cx + Math.cos(a) * 14) + ',' + f1(cy + Math.sin(a) * 22) + ' L' + f1(cx + Math.cos(a) * (60 + r() * 22)) + ',' + f1(cy + Math.sin(a) * (44 + r() * 16)), 'stroke="' + (i % 2 ? '#7a1c04' : '#fff0a0') + '" stroke-opacity=".5" stroke-width="' + f1(.8 + r() * 1.6) + '"'); }
    eye += n5g('class="n5-fb n5-slit"', n5e(cx, cy, 13, 56, 'fill="#050202"')) + n5e(cx - 30, cy - 26, 16, 9, 'fill="#fff" opacity=".7" transform="rotate(-20 ' + (cx - 30) + ' ' + (cy - 26) + ')"') + n5c(cx + 26, cy + 24, 4, 'fill="#fff" opacity=".4"') + n5e(cx, cy, 84, 62, 'fill="none" stroke="#1a0604" stroke-width="5"');
    s += n5g('class="n5-fb n5-blink" style="animation-duration:9s"', '<g clip-path="url(#n5-eyec)">' + n5g('class="n5-flit" style="--fx:12px;animation-duration:17s"', eye) + '</g>' + n5e(cx, cy, 84, 62, 'fill="none" stroke="#0a0302" stroke-width="6"'));
    s += n5p('M' + (cx - 96) + ',' + (cy - 30) + ' Q' + cx + ',' + (cy - 112) + ' ' + (cx + 96) + ',' + (cy - 30), 'stroke="#2a0a06" stroke-width="16" fill="none" stroke-linecap="round"') + n5p('M' + (cx - 90) + ',' + (cy + 40) + ' Q' + cx + ',' + (cy + 100) + ' ' + (cx + 90) + ',' + (cy + 40), 'stroke="#2a0a06" stroke-width="12" fill="none" stroke-linecap="round"');
    fr = '<defs>' + mp3Defs() + '</defs>' + n5p(d, 'fill="none" stroke="#0a0604" stroke-width="22" stroke-linejoin="round"') + n5g('filter="url(#mp-stone)"', n5p(d, 'fill="none" stroke="#4a3a2c" stroke-width="12" stroke-linejoin="round"')) + n5p(d, 'fill="none" stroke="#ffb040" stroke-opacity=".5" stroke-width="2"') + n5p(d, 'fill="none" stroke="#000" stroke-opacity=".6" stroke-width="2" transform="translate(2,3)"');
    return mpOpen(d, s, fr, n5g('opacity=".2" pointer-events="none"', n5e(cx, cy, 220, 170, 'fill="#ff8a20" filter="url(#src-blur14)" class="mp-breathe"')));
  };
  DISPLAYS.hoard = function (t) {
    var r = rng(275), P = [[88, 176], [198, 156], [318, 182], [430, 160], [72, 294], [184, 306], [302, 284], [420, 300], [106, 412], [216, 398], [330, 414], [440, 400]], s, row, m, d, i;
    s = '<defs>' + mp3Defs() + lin('n5-gold', 1, [[0, '#f0c040'], [.5, '#a87414'], [1, '#3a2406']]) + n5Metals() + '</defs>';
    // a shoulder of rock, and three terraces of treasure heaped against it
    s += n5g('filter="url(#mp-stone)"', n5p('M14,470 Q0,300 40,200 Q30,110 110,96 Q220,56 320,104 Q430,76 474,170 Q524,270 486,370 Q512,430 502,470 Z', 'fill="#241a14"'));
    for (row = 0; row < 3; row++) {
      var Q = P.slice(row * 4, row * 4 + 4), pts = [[row ? 10 : 30, Q[0][1] + 34]], bot = Q[0][1] + (row === 2 ? 60 : 70);
      Q.forEach(function (q) { pts.push([q[0] - 44, q[1] + 20], [q[0] - 20, q[1] + 4], [q[0], q[1] + 1], [q[0] + 20, q[1] + 4], [q[0] + 44, q[1] + 20]); });
      pts.push([row ? 506 : 480, Q[3][1] + 36]);
      d = 'M' + pts.map(function (v) { return f1(v[0]) + ',' + f1(v[1]); }).join(' L') + ' L' + (row ? 506 : 480) + ',' + bot + ' L' + (row ? 10 : 30) + ',' + bot + ' Z';
      s += n5sh(d, 4, 8, .6, 8) + n5p(d, 'fill="url(#n5-gold)"') + n5coins(r, pts.map(function (v) { return [v[0], v[1] + 3, bot - v[1] - 12]; }), 520, 26);
      // what else it has taken: a crown, a cup, a blade, a strongbox
      if (row === 0) s += n5p('M252,206 l4,-18 l8,10 l8,-14 l8,14 l8,-10 l4,18 Z', 'fill="url(#n5-gold)" stroke="#5a3a08" stroke-width="1"') + n5c(268, 190, 2.4, 'fill="#d8283a"');
      if (row === 1) s += n5p('M124,330 h22 l-4,16 q-2,6 -4,8 v8 h8 v3 h-22 v-3 h8 v-8 q-2,-2 -4,-8 Z', 'fill="url(#n5-gold)" stroke="#5a3a08" stroke-width="1"') + n5p('M356,300 l34,36', 'stroke="url(#n5-pewter)" stroke-width="5" stroke-linecap="round"') + n5p('M348,312 l16,-16', 'stroke="url(#n5-gold)" stroke-width="4" stroke-linecap="round"');
      if (row === 2) s += n5r(254, 420, 44, 26, 'rx="2" fill="#4a2c14"') + n5p('M254,420 q22,-22 44,0 Z', 'fill="#5a3618"') + n5p('M254,420 h44 M266,412 v34 M286,412 v34', 'stroke="url(#n5-gold)" stroke-width="3"') + n5coins(r, [[276, 416, 4]], 14, 30);
    }
    return { art: s, slots: mpSlots(P, 1.12), zone: [34, 96, 454, 320], wallsh: false, decor: true, label: 'The hoard' };
  };
  COUNTERS['coin-drift'] = function (t) {
    var s = cmTop(t, 'M-300,452 H1500 V512 H-300 Z'), r = rng(277), g, pts = [], x, y, i;
    s += '<clipPath id="cm-topclip"><rect x="-300" y="452" width="1800" height="60"/></clipPath>';
    g = '<defs>' + lin('n5-gold', 1, [[0, '#f0c040'], [.4, '#a87414'], [1, '#2a1a04']]) + n5Metals() + '</defs>' + n5r(-300, 500, 1800, 400, 'fill="url(#n5-gold)"');
    // coin from the edge of the top to the floor, with its own slow avalanches
    for (y = 506; y < 900; y += 9) for (x = -300; x < 1500; x += 60) pts.push([x + r() * 60, y, 9]);
    g += n5coins(r, pts, 3400, 60);
    for (i = 0; i < 9; i++) { x = -260 + r() * 1700; g += n5p('M' + f1(x) + ',512 q' + f1((r() - .5) * 60) + ',80 ' + f1((r() - .5) * 90) + ',200', 'stroke="#2a1a04" stroke-opacity=".35" stroke-width="' + f1(16 + r() * 24) + '" fill="none" filter="url(#src-blur8)"'); }
    // half buried in it: a strongbox with its lid thrown back, a crown, a sword to the hilt
    g += n5sh('M150,560 h130 v80 h-130 Z', 8, 6, .5, 8) + n5r(150, 574, 130, 70, 'rx="4" fill="#4a2c14"') + n5p('M144,574 q0,-40 71,-44 q71,4 71,44 Z', 'fill="#3a2210"') + n5p('M150,574 h130 M176,534 V644 M254,534 V644', 'stroke="url(#n5-gold)" stroke-width="5"') + n5r(206, 578, 18, 20, 'rx="3" fill="url(#n5-gold)"') + n5coins(r, [[215, 566, 6]], 60, 110) + n5p('M190,560 q26,-26 50,0', 'stroke="#f4f0e8" stroke-width="3" stroke-dasharray="1 5" stroke-linecap="round" fill="none"');
    g += n5p('M890,640 l8,-40 l16,22 l16,-30 l16,30 l16,-22 l8,40 Z', 'fill="url(#n5-gold)" stroke="#5a3a08" stroke-width="1.4"') + n5c(930, 606, 5, 'fill="#d8283a"') + n5c(906, 620, 3.4, 'fill="#2a6ad8"') + n5c(954, 620, 3.4, 'fill="#2aa85a"') + n5p('M1080,520 l30,120', 'stroke="url(#n5-pewter)" stroke-width="9" stroke-linecap="round"') + n5p('M1058,536 l44,-12', 'stroke="url(#n5-gold)" stroke-width="7" stroke-linecap="round"') + n5c(1076, 508, 7, 'fill="url(#n5-gold)"');
    return '<g filter="url(#cm-woodlit)">' + s + g + n5r(-300, 500, 1800, 14, 'fill="#000" opacity=".3"') + '</g>';
  };
  PROPS.glints = function (t, p) {
    var s = '', r = rng(279), i, x, y, k;
    for (i = 0; i < 34; i++) { x = 20 + r() * 1160; y = i < 12 ? 380 + r() * 80 : i < 22 ? 150 + r() * 280 : 520 + r() * 170; if (i >= 12 && i < 22) x = 40 + r() * 450; k = 4 + r() * 6; s += n5p('M' + f1(x) + ',' + f1(y - k) + ' l' + f1(k * .22) + ',' + f1(k * .78) + ' l' + f1(k * .78) + ',' + f1(k * .22) + ' l' + f1(-k * .78) + ',' + f1(k * .22) + ' l' + f1(-k * .22) + ',' + f1(k * .78) + ' l' + f1(-k * .22) + ',' + f1(-k * .78) + ' l' + f1(-k * .78) + ',' + f1(-k * .22) + ' l' + f1(k * .78) + ',' + f1(-k * .22) + ' Z', 'class="n5-fb n5-glint" fill="#fffbe0" style="animation-delay:-' + f1(r() * 6) + 's;animation-duration:' + f1(3.4 + r() * 4) + 's"'); }
    return n5g('class="src-nohit"', s);
  };
  PROPS['dragon-tail'] = function (t, p) {
    var r = rng(281), c = '#6a1c10', cd = '#2e0a06', pts = [[30, 474], [80, 462], [130, 470], [176, 488], [214, 482], [246, 468], [272, 474]], d = mpTaper(mpSmooth(pts, 6), function (u) { return 46 - u * 40; }), sm = mpSmooth(pts, 6), s, i, q, n;
    s = n5sh(d, 6, 10, .5, 5) + n5p(d, 'fill="' + c + '"') + n5p('M' + sm.map(function (v) { return f1(v[0]) + ',' + f1(v[1] + 6); }).join(' L'), 'stroke="#e8b060" stroke-opacity=".45" stroke-width="7" fill="none" stroke-linecap="round" stroke-dasharray="3 6"') + n5p('M' + sm.map(function (v) { return f1(v[0]) + ',' + f1(v[1] - 8); }).join(' L'), 'stroke="#ff9a50" stroke-opacity=".25" stroke-width="3" fill="none"');
    // a ridge of spines down it, and a barb at the tip
    for (i = 2; i < sm.length - 3; i += 3) { q = sm[i]; n = 22 * (1 - i / sm.length); s += n5p('M' + f1(q[0] - 6) + ',' + f1(q[1] - n * .6) + ' l8,' + f1(-n * .7) + ' l6,' + f1(n * .6) + ' Z', 'fill="' + cd + '"'); }
    s += n5p('M268,474 l26,-18 l-4,18 l8,16 Z', 'fill="' + cd + '"');
    return n5g('class="src-nohit"', n5g('class="n5-vb n5-sway" style="transform-origin:' + t.mx(30) + 'px 474px;animation-duration:7s"', n5g(t.mir ? 'transform="translate(1200,0) scale(-1,1)"' : '', s)));
  };


  /* ---- The sun court: white marble veined with gold, a colonnade, the sun
     itself in the oculus, goods resting on small clouds in a painted heaven
     and a marble counter. ---- */
  CM_OPT.shape.push('colonnade'); CM_OPT.wall.push('marble'); CM_OPT.opening.push('sun-disc'); CM_OPT.display.push('clouds'); CM_OPT.counter.push('marble-slab');
  CM_WALLFQ.marble = [.014, 1.6, 301]; CM_OPEN_INFO['sun-disc'] = 'The sun';
  n5light('sunlight', { base: '#fff0c8', glow: '#ffe090', amb: '#5a5676', reach: 1300, k: 1 }, function (sk) { return sk.opening === 'sun-disc' ? [912, 190] : null; });
  n5prop('doves', 'air', 'back'); n5prop('laurel', 'ceil', 'back'); n5prop('lyre', 'cL', 'front', 120);

  // A marble vein: a line that wanders downward, soft at the edges.
  function n5vein(r, x, y, n, dx, dy) {
    var d = 'M' + f1(x) + ',' + f1(y), k;
    for (k = 0; k < n; k++) { x += (r() - .35) * dx; y += dy * (.4 + r()); d += ' L' + f1(x) + ',' + f1(y); }
    return d;
  }
  // The sun in splendour: straight rays and wavy ones going round one way, a ring of short ones the other.
  function n5sun(cx, cy, tr, k) {
    var a = '', b = '', i;
    for (i = 0; i < 16; i++) a += n5p(i % 2 ? 'M-7,-44 Q-15,-62 -4,-74 Q7,-86 0,-102 Q15,-84 6,-70 Q-3,-58 7,-44 Z' : 'M-8,-44 L0,-106 L8,-44 Z', 'fill="url(#n5-ray)" transform="rotate(' + (i * 22.5) + ')"');
    for (i = 0; i < 24; i++) b += n5p('M-4,-40 L0,-62 L4,-40 Z', 'fill="' + tr + '" opacity=".8" transform="rotate(' + (i * 15 + 7) + ')"');
    return '<defs>' + lin('n5-ray', 1, [[0, '#fffbe0'], [1, tr]]) + rad('n5-sund', [[0, '#fff8d0'], [.4, '#ffdc6a'], [.8, mix(tr, '#d88a10', .4)], [1, mix(tr, '#7a4a08', .55)]]) + '</defs>' +
      n5g('transform="translate(' + cx + ',' + cy + ') scale(' + (k || 1) + ')"',
        n5g('class="n5-turn" style="animation-duration:120s"', a) + n5g('class="n5-turn" style="animation-duration:80s;animation-direction:reverse"', b) +
        n5c(1, 2, 44, 'fill="#7a4a08" opacity=".4" filter="url(#src-blur3)"') + n5c(0, 0, 42, 'fill="url(#n5-sund)"') + n5c(0, 0, 42, 'fill="none" stroke="' + mix(tr, '#5a3a08', .5) + '" stroke-width="1.4"') +
        n5c(0, 0, 33, 'fill="none" stroke="#fff" stroke-opacity=".6" stroke-width="1.2"') + n5c(0, 0, 35.4, 'fill="none" stroke="#7a4a08" stroke-opacity=".3" stroke-width="1"') +
        n5p('M-26,-14 Q-18,-30 0,-32', 'stroke="#fff" stroke-opacity=".8" stroke-width="3" fill="none" stroke-linecap="round"') + n5g('opacity=".3"', n5c(0, 0, 60, 'fill="#fff6c0" filter="url(#src-blur8)" class="n5-beat"')));
  }

  WALLS.marble = function (t) {
    var r = rng(301), tr = t.th.trim, s = '<rect ' + FULL + ' fill="#e9e2d2" filter="url(#cm-wall)"/>', i, x, d;
    // big slabs with a hairline joint between them, no two quite the same white
    for (x = -300; x < 1500; x += 300) s += n5r(x, -150, 300, 620, 'fill="' + (r() < .5 ? '#ffffff' : '#b9ad96') + '" opacity="' + f1(.05 + r() * .12) + '"') + n5r(x - .8, -150, 1.6, 620, 'fill="#6a5e48" opacity=".45"') + n5r(x + 1, -150, 1, 620, 'fill="#fff" opacity=".5"');
    // veins: long grey ones that wander, a thread of gold beside a few
    for (i = 0; i < 30; i++) {
      d = n5vein(r, -300 + r() * 1800, -150 + r() * 240, 7, 90, 70);
      s += n5p(d, 'stroke="#74727a" stroke-opacity="' + f1(.14 + r() * .24) + '" stroke-width="' + f1(1 + r() * 3.4) + '" fill="none" stroke-linejoin="round" filter="url(#src-blur3)"') + n5p(d, 'stroke="#56525c" stroke-opacity=".3" stroke-width=".7" fill="none" stroke-linejoin="round"');
      if (i % 4 === 0) s += n5p(d, 'stroke="' + tr + '" stroke-opacity=".7" stroke-width="1.1" fill="none" stroke-linejoin="round" transform="translate(3,1)"');
    }
    // a dado of warmer stone under a gold line
    s += n5r(-300, 376, 1800, 100, 'fill="#a8946c" opacity=".42"') + n5r(-300, 370, 1800, 6, 'fill="url(#cm-trimv)"') + n5r(-300, 370, 1800, 1.4, 'fill="#fff" opacity=".6"') + n5r(-300, 376, 1800, 8, 'fill="#000" opacity=".28" filter="url(#src-blur3)"');
    for (x = -280; x < 1500; x += 150) s += n5r(x + 12, 392, 126, 66, 'fill="none" stroke="#5a4a2c" stroke-opacity=".35" stroke-width="1.4"') + n5r(x + 13.4, 393.4, 126, 66, 'fill="none" stroke="#fff" stroke-opacity=".35" stroke-width="1"');
    return s;
  };
  SHAPES.colonnade = function (t) {
    var s = WALLS[t.th.wall](t), tr = t.th.trim, dk = mix(tr, '#3a2406', .6), x, g = '', m = '';
    s += '<defs>' + lin('n5-shaft', 0, [[0, '#a69a84'], [.22, '#fffaf0'], [.45, '#e6dcc8'], [.8, '#a89c86'], [1, '#6e6452']]) + '</defs>';
    // the entablature: a frieze with a gold meander, then a cornice on dentils
    s += n5r(-300, -150, 1800, 196, 'fill="#efe8d8" filter="url(#cm-wall)"') + n5r(-300, 46, 1800, 14, 'fill="#000" opacity=".38" filter="url(#src-blur5)"');
    for (x = -296; x < 1500; x += 34) m += 'M' + x + ',26 v-20 h26 v14 h-14 v-7 h7 ';
    s += n5p(m, 'stroke="' + dk + '" stroke-width="3.4" fill="none" transform="translate(1,1.6)" opacity=".6"') + n5p(m, 'stroke="' + tr + '" stroke-width="3" fill="none"') + n5r(-300, -2, 1800, 2.4, 'fill="' + tr + '"') + n5r(-300, 30, 1800, 2.4, 'fill="' + tr + '"');
    for (x = -296; x < 1500; x += 20) g += n5r(x, 36, 11, 9, 'fill="#fbf6ea"') + n5r(x + 11, 36, 3, 9, 'fill="#000" opacity=".25"');
    s += g + n5r(-300, 34, 1800, 3, 'fill="#fff"') + n5r(-300, 45, 1800, 2, 'fill="#8a7c62" opacity=".6"');
    [[-8, 36], [1172, 36]].forEach(function (q) { s += n5column(q[0], q[1], 46, 474) + n5r(q[0] - 2, 92, q[1] + 4, 6, 'fill="url(#cm-trimv)"') + n5r(q[0] - 2, 446, q[1] + 4, 6, 'fill="url(#cm-trimv)"'); });
    return s;
  };
  OPENINGS['sun-disc'] = function (t) {
    var cx = 912, cy = 190, r = rng(303), tr = t.th.trim, s, fr, i, a, lv = '', gr = '';
    s = '<defs>' + rad('n5-sunsky', [[0, '#fff6d0'], [.42, '#ffdc8a'], [1, '#5a9ce6']]) + '</defs>' + n5c(cx, cy, 114, 'fill="url(#n5-sunsky)"');
    // high cloud going by behind the sun
    for (i = 0; i < 4; i++) s += n5g('class="n5-cloud" style="--sx:-460px;animation-duration:' + (52 + i * 15) + 's;animation-delay:-' + (i * 19) + 's"', n5puff(r, 1060 + i * 30, 116 + i * 46, 92 + r() * 40, '#ffffff', .8));
    s += n5sun(cx, cy, tr, 1) + n5p('M828,170 A90,90 0 0 1 896,104', 'stroke="#fff" stroke-opacity=".35" stroke-width="8" fill="none" stroke-linecap="round"');
    // a marble ring with a gold bead inside it and a wreath of gilded laurel round it
    fr = n5p(RING, 'fill="none" stroke="#7c705a" stroke-width="30"') + n5p(RING, 'fill="none" stroke="#f6f0e2" stroke-width="25"') + n5c(cx, cy, 119, 'fill="none" stroke="#fff" stroke-opacity=".8" stroke-width="1.4"') + n5c(cx, cy, 98, 'fill="none" stroke="url(#cm-trim)" stroke-width="5"') + n5c(cx, cy, 95, 'fill="none" stroke="#000" stroke-opacity=".35" stroke-width="1.4"');
    for (i = 0; i < 44; i++) {
      a = i / 44 * 6.2832 + .07; var lx = cx + Math.cos(a) * 109, ly = cy + Math.sin(a) * 109, dg = a * 180 / Math.PI + (a > Math.PI / 2 && a < Math.PI * 1.5 ? -90 : 90);
      lv += mpLeaf(lx, ly, 15, dg + 28, mix(tr, '#5a3a08', .35), '#fff6c0') + mpLeaf(lx, ly, 15, dg - 28, mix(tr, '#ffffff', .2), '#7a5210');
    }
    fr += n5g('transform="translate(1.4,2)" opacity=".4"', lv.replace(/fill="#[0-9a-f]+"/g, 'fill="#3a2406"')) + lv + n5c(cx, cy + 109, 6, 'fill="url(#cm-trim)" stroke="#5a3a08" stroke-width="1"');
    // the light it throws down the wall
    [[850, 270, 380, 700, 540, 700], [884, 286, 640, 700, 760, 700], [936, 288, 880, 700, 990, 700], [976, 268, 1120, 700, 1280, 700]].forEach(function (q, j) { gr += n5p('M' + q[0] + ',' + q[1] + ' L' + q[2] + ',' + q[3] + ' L' + q[4] + ',' + q[5] + ' Z', 'fill="#fff0b0" class="n5-shimmer" style="animation-duration:' + (6 + j * 1.7) + 's;animation-delay:-' + (j * 2.3) + 's"'); });
    return mpOpen(RING, s, fr, n5g('opacity=".16" filter="url(#src-blur8)" pointer-events="none"', gr));
  };
  DISPLAYS.clouds = function (t) {
    var r = rng(305), tr = t.th.trim, P = [[92, 186], [200, 164], [318, 184], [430, 172], [76, 296], [186, 308], [304, 286], [422, 300], [108, 412], [218, 400], [332, 414], [440, 402]],
      d = 'M40,462 V160 Q40,74 265,66 Q490,74 490,160 V462 Z', s, i, x, y;
    s = '<defs>' + lin('n5-heaven', 1, [[0, '#2a56b0'], [.5, '#6aa4e4'], [.86, '#ffe6b8'], [1, '#ffd08a']]) + '<clipPath id="n5-hvc"><path d="' + d + '"/></clipPath></defs>';
    // a great niche painted as the sky: gold stars high up, banks of cloud lower down
    s += n5sh(d, 6, 8, .4, 8) + n5p(d, 'fill="url(#n5-heaven)"') + '<g clip-path="url(#n5-hvc)">';
    for (i = 0; i < 46; i++) { x = 50 + r() * 430; y = 76 + r() * 150; s += n5p('M' + f1(x) + ',' + f1(y - 4) + ' l1.2,2.8 l2.8,1.2 l-2.8,1.2 l-1.2,2.8 l-1.2,-2.8 l-2.8,-1.2 l2.8,-1.2 Z', 'fill="' + tr + '" opacity="' + f1(.4 + r() * .6) + '"'); }
    for (i = 0; i < 9; i++) s += n5puff(r, 40 + r() * 450, 250 + i * 26 + r() * 20, 150 + r() * 130, i % 2 ? '#ffffff' : '#ffe8c8', .34);
    s += n5e(265, 470, 300, 60, 'fill="#fff6dc" opacity=".6" filter="url(#src-blur14)"') + n5p(d, 'fill="none" stroke="#000" stroke-opacity=".4" stroke-width="16" filter="url(#src-blur5)"') + '</g>';
    // its frame: marble, a gold fillet and a keystone
    s += n5p(d, 'fill="none" stroke="#7c705a" stroke-width="20"') + n5p(d, 'fill="none" stroke="#f6f0e2" stroke-width="16"') + n5p(d, 'fill="none" stroke="url(#cm-trim)" stroke-width="3.4" transform="translate(0,0)"') +
      n5p('M246,48 h38 l-6,34 h-26 Z', 'fill="#f6f0e2" stroke="#7c705a" stroke-width="1.4"') + n5p('M256,56 h18 l-3,18 h-12 Z', 'fill="url(#cm-trim)"') + n5r(22, 456, 486, 16, 'fill="#f6f0e2" stroke="#7c705a" stroke-width="1.2"') + n5r(22, 456, 486, 2, 'fill="#fff"');
    return { art: s, slots: mpSlots(P, 1.12), zone: [34, 96, 454, 320], wallsh: false, decor: false, floaty: true, label: 'The painted heaven',
      // each good rides its own small cloud, and the cloud bobs with it
      base: function (it, p) { var q = rng(Math.round(p[0] * 7 + p[1])); return n5e(p[0], p[1] + 15, 38, 6, 'fill="#5a78b8" opacity=".35" filter="url(#src-blur3)"') + n5puff(q, p[0], p[1] + 7, 88, '#ffffff', .96) + n5e(p[0] - 8, p[1] + 1, 30, 5, 'fill="#fff" opacity=".9" filter="url(#src-blur3)"'); } };
  };
  COUNTERS['marble-slab'] = function (t) {
    var r = rng(307), tr = t.th.trim, dk = mix(tr, '#3a2406', .6), s, x, i, m = '', lv = '', a;
    s = '<defs>' + mp3Defs() + lin('n5-mtop', 1, [[0, '#fffdf6'], [1, '#d6ccb8']]) + lin('n5-mfr', 1, [[0, '#efe8d8'], [.4, '#d8cebb'], [1, '#93876f']]) + '</defs><clipPath id="cm-topclip"><rect x="-300" y="452" width="1800" height="58"/></clipPath>';
    s += n5r(-300, 452, 1800, 54, 'fill="url(#n5-mtop)"');
    for (i = 0; i < 18; i++) { x = -300 + r() * 1800; s += n5p('M' + f1(x) + ',452 q' + f1(20 + r() * 40) + ',' + f1(18 + r() * 12) + ' ' + f1(60 + r() * 90) + ',50', 'stroke="#74727a" stroke-opacity="' + f1(.18 + r() * .25) + '" stroke-width="' + f1(.8 + r() * 1.8) + '" fill="none"'); }
    s += n5r(-300, 500, 1800, 12, 'fill="url(#cm-trimv)"') + n5r(-300, 500, 1800, 2, 'fill="#fff" opacity=".6"');
    // the front: a band of gold key pattern, then panels of veined stone between pilasters
    s += n5r(-300, 512, 1800, 390, 'fill="url(#n5-mfr)"') + [0, 1, 2, 3, 4, 5, 6, 7].map(function (j) { return n5e(-300 + r() * 1800, 580 + r() * 200, 120 + r() * 160, 40 + r() * 50, 'fill="' + (j % 2 ? '#ffffff' : '#8a7e68') + '" opacity=".16" filter="url(#src-blur14)"'); }).join('');
    for (i = 0; i < 22; i++) s += n5p(n5vein(r, -300 + r() * 1800, 560, 6, 70, 60), 'stroke="#74727a" stroke-opacity="' + f1(.12 + r() * .2) + '" stroke-width="' + f1(.8 + r() * 2.6) + '" fill="none" stroke-linejoin="round"');
    for (x = -296; x < 1500; x += 34) m += 'M' + x + ',548 v-20 h26 v14 h-14 v-7 h7 ';
    s += n5p(m, 'stroke="' + dk + '" stroke-width="3.4" fill="none" transform="translate(1,1.6)" opacity=".6"') + n5p(m, 'stroke="' + tr + '" stroke-width="3" fill="none"') + n5r(-300, 520, 1800, 2.4, 'fill="' + tr + '"') + n5r(-300, 553, 1800, 2.4, 'fill="' + tr + '"');
    for (x = -300; x < 1500; x += 300) {
      s += n5r(x - 20, 566, 40, 340, 'fill="#f4eee0"') + n5r(x + 20, 566, 8, 340, 'fill="#000" opacity=".25" filter="url(#src-blur3)"');
      for (i = -2; i < 3; i++) s += n5r(x + i * 7 - 1, 580, 2, 320, 'fill="#000" opacity=".18"');
      s += n5r(x - 24, 562, 48, 10, 'fill="url(#cm-trimv)"') + n5r(x + 42, 584, 216, 320, 'fill="none" stroke="#5a4a2c" stroke-opacity=".4" stroke-width="1.6"') + n5r(x + 43.6, 585.6, 216, 320, 'fill="none" stroke="#fff" stroke-opacity=".5" stroke-width="1"');
    }
    // a gilded wreath on the panel the customer stands at
    for (i = 0; i < 26; i++) { a = i / 26 * 6.2832; if (Math.abs(a - 4.712) < .32) continue; var lx = 600 + Math.cos(a) * 38, ly = 642 + Math.sin(a) * 38, dg = a * 180 / Math.PI + (Math.cos(a) > 0 ? -90 : 90); lv += mpLeaf(lx, ly, 13, dg + 26, mix(tr, '#5a3a08', .3), '#fff6c0') + mpLeaf(lx, ly, 13, dg - 26, mix(tr, '#ffffff', .2), ''); }
    s += n5g('transform="translate(1.4,2.4)" opacity=".4"', lv.replace(/fill="#[0-9a-f]+"/g, 'fill="#3a2406"')) + lv;
    return s + n5r(-300, 512, 1800, 22, 'fill="#000" opacity=".3" filter="url(#src-blur8)"');
  };
  LIGHTS.sunlight = function (t) {
    var x = t.lp.x, tr = t.th.trim, s;
    // the sun is drawn again over the room's glow so its rays stay sharp
    if (t.sk.opening === 'sun-disc') return { back: '', front: '<g class="src-nohit"><clipPath id="n5-sunc"><circle cx="912" cy="190" r="96"/></clipPath><g clip-path="url(#n5-sunc)">' + n5sun(912, 190, tr, 1) + '</g></g>' };
    // with no sun in the wall, a gilded one on a stand
    s = '<g data-k="lamp">' + n5e(x, 482, 34, 6, 'fill="url(#src-ao)"') + n5p('M' + (x - 22) + ',480 h44 l-8,-9 h-28 Z', 'fill="url(#cm-trim)"') + n5r(x - 4, 414, 8, 58, 'fill="url(#cm-trim)"') + n5c(x, 372, 60, 'fill="url(#cm-glow)" opacity=".9" class="cm-pulse"') + n5sun(x, 372, tr, .4);
    return { back: '', front: s + '<circle cx="' + x + '" cy="372" r="130" fill="url(#cm-bloom)" class="cm-pulse" opacity=".8"/></g>' };
  };
  PROPS.doves = function (t, p) {
    var s = '';
    // white doves crossing the court, each at its own height and pace
    [[150, 24, 0, 1], [250, 33, -9, .8], [96, 29, -17, 1.15], [318, 38, -25, .7]].forEach(function (q) {
      var b = n5p('M11,-2 l15,-6 l-2,6 l5,5 l-17,0 Z', 'fill="#e4e6ee"') + n5e(0, 0, 13, 6, 'fill="#fbfbff"') + n5e(1, 2.4, 11, 3.4, 'fill="#b8bccc" opacity=".6"') + n5c(-12, -3.4, 4.8, 'fill="#fbfbff"') + n5p('M-16.4,-3.6 l-5,1.4 l5,1.2 Z', 'fill="#e8b040"') + n5c(-13.4, -4.4, .9, 'fill="#1a1a22"') +
        n5g('class="n5-fbb n5-flap" style="animation-duration:.5s"', n5p('M-5,-2 Q0,-28 22,-32 Q13,-16 9,-2 Z', 'fill="#ffffff" stroke="#b8bccc" stroke-width=".8"') + n5p('M2,-6 Q6,-20 17,-27', 'stroke="#b8bccc" stroke-width=".8" fill="none"'));
      s += n5g('class="n5-swim" style="--sx:-1500px;animation-duration:' + q[1] + 's;animation-delay:' + q[2] + 's"', n5g('transform="translate(1340,' + q[0] + ') scale(' + q[3] + ')"', n5g('class="n5-hover" style="animation-duration:' + f1(3 + q[3] * 2) + 's"', b)));
    });
    return n5g('class="src-nohit"', s);
  };
  PROPS.laurel = function (t, p) {
    var r = rng(311), s = '', tr = t.th.trim;
    // garlands of bay hung along the cornice, tied up with gold ribbon
    [[516, 690, 30], [690, 792, 22], [1032, 1176, 30]].forEach(function (q, j) {
      var g = '', n = Math.round((q[1] - q[0]) / 7), i, u, x, y, dg;
      for (i = 0; i <= n; i++) { u = i / n; x = q[0] + (q[1] - q[0]) * u; y = 58 + 4 * q[2] * u * (1 - u); dg = Math.atan2(4 * q[2] * (1 - 2 * u), q[1] - q[0]) * 180 / Math.PI; g += mpLeaf(x, y, 15 + r() * 4, dg + 150 + r() * 20, mix('#4a6a30', '#1c3014', r() * .6), '#c8e0a0') + mpLeaf(x, y, 15 + r() * 4, dg + 30 - r() * 20, mix('#6a8a40', '#a8c070', r() * .5), '#2a4018') + (r() < .2 ? n5c(x, y + 3, 2.2, 'fill="' + tr + '"') : ''); }
      s += n5g('transform="translate(3,6)" opacity=".35" filter="url(#src-blur3)"', n5sag(q[0], 58, q[1], 58, q[2] / 2, '#000', 12)) + n5g('class="n5-fbt n5-sway" style="animation-duration:' + (8 + j * 1.6) + 's;animation-delay:-' + (j * 2.1) + 's"', g);
    });
    [516, 690, 792, 1032, 1176].forEach(function (x, j) { s += n5g('class="n5-fbt n5-sway" style="animation-duration:' + f1(5 + j * .7) + 's"', n5p('M' + x + ',58 q-10,22 -5,46 l5,-6 l5,6 q5,-24 -5,-46 Z', 'fill="' + tr + '"') + n5p('M' + x + ',60 q-3,22 0,38', 'stroke="#000" stroke-opacity=".3" stroke-width="1" fill="none"')) + n5p('M' + x + ',58 q-16,-12 -14,4 q2,8 14,-4 q16,-12 14,4 q-2,8 -14,-4 Z', 'fill="' + tr + '" stroke="' + mix(tr, '#3a2406', .5) + '" stroke-width="1"') + n5c(x, 58, 3.4, 'fill="' + mix(tr, '#ffffff', .4) + '"'); });
    return n5g('class="src-nohit"', s);
  };
  PROPS.lyre = function (t, p) {
    var x = p.x - 14, y = 492, tr = t.th.trim, dk = mix(tr, '#3a2406', .6), hi = mix(tr, '#ffffff', .55), s = n5e(p.x, y + 3, 56, 8, 'fill="url(#src-ao)"'), i, g = '', arms = '';
    // a gilt lyre, its strings still humming, and a wreath left beside it
    for (i = 0; i < 7; i++) g += n5p('M' + (x - 15 + i * 5) + ',' + (y - 88) + ' V' + (y - 22), 'class="n5-shimmer" style="animation-duration:' + f1(1.6 + i * .23) + 's;animation-delay:-' + f1(i * .4) + 's" stroke="#fffbe8" stroke-width="1.3"');
    [-1, 1].forEach(function (k) { arms += 'M' + (x + k * 12) + ',' + (y - 18) + ' C' + (x + k * 48) + ',' + (y - 34) + ' ' + (x + k * 44) + ',' + (y - 72) + ' ' + (x + k * 26) + ',' + (y - 94) + ' q' + (-k * 3) + ',-9 ' + (k * 10) + ',-10 '; });
    s += n5p(arms, 'stroke="' + dk + '" stroke-width="9" fill="none" stroke-linecap="round"') + n5p(arms, 'stroke="' + tr + '" stroke-width="6" fill="none" stroke-linecap="round"') + n5p(arms, 'stroke="' + hi + '" stroke-width="1.6" fill="none" stroke-linecap="round" transform="translate(-1.4,-1)"');
    s += n5r(x - 34, y - 91, 68, 7, 'rx="3.5" fill="' + tr + '" stroke="' + dk + '" stroke-width="1.2"') + n5r(x - 31, y - 90, 62, 1.6, 'fill="' + hi + '"') + g + n5p('M' + (x - 24) + ',' + y + ' q-4,-24 24,-28 q28,4 24,28 Z', 'fill="' + tr + '" stroke="' + dk + '" stroke-width="1.4"') + n5p('M' + (x - 15) + ',' + (y - 9) + ' q15,-14 30,0', 'stroke="' + hi + '" stroke-width="1.8" fill="none"') + n5r(x - 19, y - 25, 38, 3.4, 'fill="' + dk + '"');
    for (i = 0; i < 16; i++) { var a = i / 16 * 6.2832, lx = x + 62 + Math.cos(a) * 17, ly = y - 3 + Math.sin(a) * 5; s += mpLeaf(lx, ly, 10, a * 180 / Math.PI + 70, i % 2 ? '#5a8a38' : '#8ab050', ''); }
    return s;
  };


  /* ---- The haunted parlour: paper that has faded and let go of the wall, a
     pier glass that shows someone who is not in the room, a cabinet of
     curios under glass, a counter under a dust sheet and cold candles. ---- */
  CM_OPT.wall.push('faded-paper'); CM_OPT.opening.push('tall-mirror'); CM_OPT.display.push('curio'); CM_OPT.counter.push('sheeted');
  CM_WALLFQ['faded-paper'] = [.04, 1.4, 321]; CM_OPEN_INFO['tall-mirror'] = 'The pier glass';
  n5light('candelabra', { base: '#cfe0ff', glow: '#9ab8e8', amb: '#22263c', reach: 900, k: .85 });
  n5prop('ghost', 'air', 'back'); n5prop('portrait', 'wallR', 'back'); n5prop('phonograph', 'cL', 'front', 130);

  // Someone in a winding sheet, drawn about 0,0 at the waist.
  function n5ghost(col) {
    return n5g('filter="url(#src-blur3)"', n5p('M-18,-40 Q-24,-72 0,-76 Q24,-72 18,-40 Q32,0 27,62 l-8,-11 l-6,13 l-8,-13 l-8,13 l-6,-13 l-9,11 Q-32,0 -18,-40 Z', 'fill="' + col + '"')) +
      n5e(-7, -56, 3.4, 5, 'fill="#0a1018" opacity=".75"') + n5e(7, -56, 3.4, 5, 'fill="#0a1018" opacity=".75"') + n5e(0, -44, 3, 4.4, 'fill="#0a1018" opacity=".5"') + n5p('M-14,-20 Q-4,10 -10,50 M10,-22 Q16,10 12,52', 'stroke="#fff" stroke-opacity=".35" stroke-width="1.6" fill="none"');
  }

  WALLS['faded-paper'] = function (t) {
    var r = rng(321), s = '<rect ' + FULL + ' fill="#7f8778" filter="url(#cm-wall)"/>', x, y, i, g = '', j = 0;
    // stripes that were two greens once and are nearly one now, a small sprig between
    for (x = -300; x < 1500; x += 44, j++) {
      s += n5r(x, -150, 22, 524, 'fill="#34443c" opacity=".24"') + n5r(x + 22, -150, 1, 524, 'fill="#d8d0b8" opacity=".14"');
      for (y = -130 + (j % 2) * 28; y < 370; y += 56) g += 'M' + (x + 33) + ',' + (y - 7) + ' q5,4 0,14 q-5,-10 0,-14 M' + (x + 27) + ',' + (y + 2) + ' q6,-2 6,5 M' + (x + 39) + ',' + (y + 2) + ' q-6,-2 -6,5 ';
    }
    s += n5p(g, 'stroke="#d8cca0" stroke-opacity=".3" stroke-width="1.2" fill="none"');
    // where the rain has come through: tide marks down from the cornice
    for (i = 0; i < 8; i++) { x = -260 + r() * 1700; y = 30 + r() * 150; s += n5p(mpLump(r, x, y, 50 + r() * 70, 9), 'fill="#3a2c16" opacity=".3" filter="url(#src-blur8)"') + n5p(mpLump(r, x, y + 10, 40 + r() * 50, 9), 'fill="none" stroke="#2a1e0c" stroke-opacity=".3" stroke-width="1.2"') + n5p('M' + f1(x) + ',' + f1(y + 40) + ' q' + f1((r() - .5) * 14) + ',' + f1(60 + r() * 80) + ' ' + f1((r() - .5) * 10) + ',' + f1(120 + r() * 100), 'stroke="#2a1e0c" stroke-opacity=".28" stroke-width="' + f1(1.5 + r() * 3) + '" fill="none" stroke-linecap="round"'); }
    // where it has let go: laths and old plaster, and the paper curling off them
    [[1112, 118, 46], [770, 262, 30], [724, 96, 26], [-90, 210, 50], [1350, 290, 44]].forEach(function (q, k) {
      var d = mpLump(r, q[0], q[1], q[2], 8), R = q[2], c = 'M' + f1(q[0] - R * .6) + ',' + f1(q[1] + R * .7) + ' q' + f1(R * .6) + ',' + f1(R * .9) + ' ' + f1(R * 1.1) + ',' + f1(R * .05) + ' q' + f1(-R * .5) + ',' + f1(R * .3) + ' ' + f1(-R * 1.1) + ',' + f1(-R * .05) + ' Z';
      s += '<clipPath id="n5-peel' + k + '"><path d="' + d + '"/></clipPath><g clip-path="url(#n5-peel' + k + ')">' + n5r(q[0] - R - 20, q[1] - R - 20, R * 2 + 40, R * 2 + 40, 'fill="#1a1612"');
      for (y = q[1] - R - 20; y < q[1] + R + 20; y += 11) s += n5r(q[0] - R - 20, y, R * 2 + 40, 7.4, 'fill="' + mix('#6a563a', '#3a2e1e', r()) + '"') + n5r(q[0] - R - 20, y, R * 2 + 40, 1.2, 'fill="#fff" opacity=".12"');
      s += n5p(d, 'fill="none" stroke="#000" stroke-opacity=".7" stroke-width="10" filter="url(#src-blur3)"') + '</g>' + n5p(d, 'fill="none" stroke="#b8b49c" stroke-opacity=".5" stroke-width="1.2"');
      s += n5p(c, 'fill="#000" opacity=".4" filter="url(#src-blur3)" transform="translate(3,5)"') + n5p(c, 'fill="#b4b29a" stroke="#6a6858" stroke-width=".8"') + n5p('M' + f1(q[0] - R * .4) + ',' + f1(q[1] + R * .9) + ' q' + f1(R * .5) + ',' + f1(R * .4) + ' ' + f1(R * .8) + ',0', 'stroke="#fff" stroke-opacity=".3" stroke-width="1.2" fill="none"');
    });
    // a dado of dark panelling
    s += n5r(-300, 372, 1800, 100, 'fill="#000" filter="url(#cm-board)"') + n5r(-300, 372, 1800, 100, 'fill="#0a0808" opacity=".5"') + n5r(-300, 364, 1800, 10, 'fill="#000" filter="url(#cm-beam)"') + n5r(-300, 364, 1800, 1.4, 'fill="#fff" opacity=".2"') + n5r(-300, 374, 1800, 8, 'fill="#000" opacity=".5" filter="url(#src-blur3)"');
    for (x = -290; x < 1500; x += 150) s += n5r(x + 12, 390, 126, 70, 'fill="none" stroke="#000" stroke-opacity=".6" stroke-width="2"') + n5r(x + 13.6, 391.6, 126, 70, 'fill="none" stroke="#fff" stroke-opacity=".1" stroke-width="1"');
    return s;
  };
  OPENINGS['tall-mirror'] = function (t) {
    var d = 'M836,330 V140 Q836,76 912,72 Q988,76 988,140 V330 Z', r = rng(323), a = t.th.accent, s, fr, i, x, y, an,
      cr = 'M958,120 L930,150 L936,176 L910,214 M958,120 L972,160 L962,196 M958,120 L920,112 L896,124 M958,120 L980,104 M936,176 L952,200 M930,150 L900,160';
    s = '<defs>' + lin('n5-mirr', 1, [[0, '#18202c'], [.5, '#3c4c5a'], [1, '#121820']]) + '</defs>' + n5r(820, 60, 190, 290, 'fill="url(#n5-mirr)"');
    // the room it shows is not quite this one: a far door standing open, a light beyond it
    s += n5p('M878,330 V168 Q878,150 906,150 Q934,150 934,168 V330 Z', 'fill="#0a0e14"') + n5p('M884,330 V170 Q884,156 906,156 Q928,156 928,170 V330 Z', 'fill="#8aa0b0" opacity=".3" class="n5-beat"') + n5p('M836,304 L878,290 M988,304 L934,290', 'stroke="#000" stroke-opacity=".5" stroke-width="1.4"') + n5r(820, 300, 190, 40, 'fill="#000" opacity=".3"');
    // and someone crosses it, now and then
    s += n5g('class="n5-haunt" style="--hx0:-64px;--hx1:64px;--ho:.85;animation-duration:19s"', n5g('transform="translate(912,246) scale(1.1)"', n5g('class="n5-hover"', n5ghost(mix(a, '#ffffff', .6)))));
    // the silvering going at the edges, and a crack from one corner
    for (i = 0; i < 46; i++) { an = r() * 6.2832; x = 912 + Math.cos(an) * (62 + r() * 16); y = 204 + Math.sin(an) * (116 + r() * 16); s += n5c(x, y, 2 + r() * 7, 'fill="#0a0c10" opacity="' + f1(.2 + r() * .4) + '" filter="url(#src-blur3)"'); }
    s += n5p(cr, 'stroke="#000" stroke-opacity=".5" stroke-width="1.6" fill="none" transform="translate(1,1)"') + n5p(cr, 'stroke="#e8f4ff" stroke-opacity=".7" stroke-width=".9" fill="none"') + n5p('M836,250 L930,72 L960,72 L836,310 Z', 'fill="#fff" opacity=".07"') + n5p('M880,330 L988,130 V170 L900,330 Z', 'fill="#fff" opacity=".04"');
    // a carved frame gone dull, a crest with a cold stone in it, and a web nobody has disturbed
    fr = n5p(d, 'fill="none" stroke="#0c0a08" stroke-width="26" stroke-linejoin="round"') + n5p(d, 'fill="none" stroke="url(#cm-trim)" stroke-width="18" stroke-linejoin="round"') + n5p(d, 'fill="none" stroke="#000" stroke-opacity=".5" stroke-width="2"') + n5p(d, 'fill="none" stroke="#fff" stroke-opacity=".2" stroke-width="1" transform="translate(-4,-4)"');
    for (y = 150; y < 330; y += 22) fr += n5c(836, y, 3, 'fill="#000" opacity=".35"') + n5c(988, y, 3, 'fill="#000" opacity=".35"');
    fr += n5p('M868,72 Q882,32 912,24 Q942,32 956,72 Q912,54 868,72 Z', 'fill="url(#cm-trim)" stroke="#0c0a08" stroke-width="1.6"') + n5p('M892,58 Q912,28 932,58', 'stroke="#000" stroke-opacity=".4" stroke-width="1.4" fill="none"') + n5c(912, 44, 7, 'fill="#0c0a08"') + n5c(912, 44, 4, 'fill="' + a + '" class="n5-beat"') +
      n5r(818, 330, 188, 12, 'fill="url(#cm-trimv)"') + n5r(818, 330, 188, 1.4, 'fill="#fff" opacity=".3"') + n5r(826, 342, 172, 9, 'fill="#000" opacity=".5" filter="url(#src-blur3)"') + n5p('M830,342 q4,16 16,18 M994,342 q-4,16 -16,18', 'stroke="url(#cm-trim)" stroke-width="4" fill="none"') +
      n5p('M820,62 L872,98 M820,62 L850,124 M820,62 L826,136 M838,74 Q844,88 832,96 M852,86 Q860,108 840,118 M828,100 Q836,112 824,120', 'stroke="#fff" stroke-opacity=".3" stroke-width=".7" fill="none"');
    return mpOpen(d, s, fr, n5g('opacity=".14" pointer-events="none"', n5e(912, 210, 150, 190, 'fill="' + a + '" filter="url(#src-blur14)" class="n5-beat"')));
  };
  DISPLAYS.curio = function (t) {
    var r = rng(325), s = cmUnitFrame(t, .78), i, web = 'M40,86 L98,110 M40,86 L80,142 M40,86 L50,152 M62,96 Q68,108 54,116 M80,104 Q86,126 62,138 M50,118 Q58,128 46,136 ';
    // a faded lining, and what the moths left of it
    s += n5r(40, 86, 442, 364, 'fill="' + mix(t.th.cloth, '#000', .5) + '" opacity=".6"');
    for (i = 0; i < 14; i++) s += n5e(50 + r() * 420, 96 + r() * 340, 6 + r() * 16, 4 + r() * 10, 'fill="#000" opacity=".4"');
    // shelves of glass on small pegs
    CM_ROWS.forEach(function (ry) {
      s += n5r(40, ry, 442, 5, 'fill="#bfe0e0" opacity=".5"') + n5r(40, ry, 442, 1.2, 'fill="#fff" opacity=".8"') + n5r(40, ry + 5, 442, 1, 'fill="#1a3038" opacity=".6"') + n5r(40, ry + 6, 442, 10, 'fill="#000" opacity=".3" filter="url(#src-blur3)"');
      [44, 262, 478].forEach(function (px) { s += n5r(px - 3, ry + 5, 6, 4, 'fill="url(#cm-trim)"'); });
    });
    // a broken pediment over it with an urn in the gap, and webs in the corners
    s += n5p('M20,72 V56 L212,28 L228,50 L198,56 V72 Z', 'fill="#000" filter="url(#cm-beam)"') + n5p('M502,72 V56 L310,28 L294,50 L324,56 V72 Z', 'fill="#000" filter="url(#cm-beam)"') + n5p('M20,56 L212,28 M502,56 L310,28', 'stroke="#fff" stroke-opacity=".2" stroke-width="1.4"') +
      n5p('M248,70 h26 l-4,-7 q11,-14 0,-27 h-18 q-11,13 0,27 Z', 'fill="url(#cm-trim)" stroke="#0c0a08" stroke-width="1"') + n5r(249, 31, 24, 4, 'rx="2" fill="url(#cm-trim)"') + n5c(261, 27, 4, 'fill="url(#cm-trim)"') +
      n5p(web, 'stroke="#fff" stroke-opacity=".28" stroke-width=".7" fill="none"') + n5p(web, 'stroke="#fff" stroke-opacity=".28" stroke-width=".7" fill="none" transform="translate(522,0) scale(-1,1)"');
    return { art: '<g filter="url(#cm-unitlit)">' + s + '</g>', slots: cmGrid(), zone: [34, 96, 454, 320], wallsh: true, decor: true, label: 'The curio cabinet',
      // every curio stands on an ebony foot under its own glass dome
      base: function (it, p) { return n5e(p[0], p[1] - .6, 42, 5, 'fill="#0e0a0a"') + n5e(p[0], p[1] - 2.4, 40, 4, 'fill="#3a2c28"') + n5e(p[0] - 8, p[1] - 3, 22, 1.6, 'fill="#fff" opacity=".18"'); },
      front: function (it, p) {
        var x = p[0], y = p[1] - 3, d = 'M' + (x - 37) + ',' + y + ' V' + (y - 52) + ' Q' + (x - 37) + ',' + (y - 87) + ' ' + x + ',' + (y - 87) + ' Q' + (x + 37) + ',' + (y - 87) + ' ' + (x + 37) + ',' + (y - 52) + ' V' + y + ' Z';
        return n5g('class="src-nohit" pointer-events="none"', n5p(d, 'fill="#cfe8f0" fill-opacity=".07" stroke="#dff4fa" stroke-opacity=".5" stroke-width="1.3"') + n5p('M' + (x - 29) + ',' + (y - 12) + ' V' + (y - 52) + ' Q' + (x - 29) + ',' + (y - 76) + ' ' + (x - 12) + ',' + (y - 81), 'stroke="#fff" stroke-opacity=".5" stroke-width="2.4" fill="none" stroke-linecap="round"') + n5p('M' + (x + 30) + ',' + (y - 20) + ' V' + (y - 46), 'stroke="#fff" stroke-opacity=".25" stroke-width="1.4" fill="none" stroke-linecap="round"') + n5c(x, y - 90, 4, 'fill="#dff4fa" opacity=".6"'));
      } };
  };
  COUNTERS.sheeted = function (t) {
    var r = rng(327), s, x, w, f = '', d, hem = '', i;
    function hy(x) { return x < 760 ? 900 : Math.max(548, 900 - (x - 760) * .9); }
    s = '<defs>' + lin('n5-sheet', 1, [[0, '#aeb0b2'], [.2, '#989a9e'], [1, '#4c4d54']]) + '</defs><clipPath id="cm-topclip"><rect x="-300" y="452" width="1800" height="60"/></clipPath>';
    // what is under it, where it shows: dark panelled wood
    s += n5r(-300, 512, 1800, 390, 'fill="#000" filter="url(#cm-front)"');
    for (x = -292; x < 1500; x += 186) s += n5r(x + 14, 566, 158, 300, 'fill="none" stroke="#000" stroke-opacity=".6" stroke-width="3"') + n5r(x + 16, 568, 158, 300, 'fill="none" stroke="' + t.lc + '" stroke-opacity=".18" stroke-width="1.2"') + n5p('M' + (x + 93) + ',600 l24,34 l-24,34 l-24,-34 Z', 'fill="none" stroke="#000" stroke-opacity=".5" stroke-width="2"') + n5p('M' + (x + 93) + ',604 l21,30', 'stroke="' + t.lc + '" stroke-opacity=".2" stroke-width="1" fill="none"');
    // the sheet lies over the top
    s += n5r(-300, 452, 1800, 56, 'fill="#a9abae"');
    for (i = 0; i < 22; i++) { x = -300 + r() * 1800; s += n5p('M' + f1(x) + ',' + f1(454 + r() * 10) + ' q' + f1(20 + r() * 40) + ',' + f1(10 + r() * 20) + ' ' + f1(70 + r() * 80) + ',' + f1(30 + r() * 20), 'stroke="' + (i % 2 ? '#000' : '#fff') + '" stroke-opacity="' + (i % 2 ? .14 : .4) + '" stroke-width="3" fill="none" filter="url(#src-blur3)"'); }
    // and falls down the front in folds; at one end it has slipped
    for (x = 1500; x >= -300; x -= 36) hem += ' L' + x + ',' + f1(hy(x) + r() * 10);
    d = 'M-300,504 H1500' + hem + ' Z';
    s += n5sh(d, 5, 8, .55, 5) + '<clipPath id="n5-shc"><path d="' + d + '"/></clipPath>' + n5p(d, 'fill="url(#n5-sheet)"') + '<g clip-path="url(#n5-shc)">';
    for (x = -300; x < 1500; x += 22 + r() * 48) { w = 14 + r() * 26; f += n5p('M' + f1(x) + ',512 L' + f1(x + (r() - .5) * 30) + ',900 h' + f1(w) + ' L' + f1(x + w * .5) + ',512 Z', 'fill="' + (r() < .6 ? '#000' : '#fff') + '" opacity="' + f1(.08 + r() * .16) + '" filter="url(#src-blur3)"'); }
    s += f + n5r(-300, 508, 1800, 18, 'fill="#000" opacity=".28" filter="url(#src-blur5)"') + '</g>' + n5p('M' + hem.slice(2), 'stroke="#55524c" stroke-width="1.4" fill="none"') + n5r(-300, 503, 1800, 4, 'fill="#c8cacc"');
    return s;
  };
  LIGHTS.candelabra = function (t) {
    var x = t.lp.x, tr = 'url(#cm-trim)', s = '<g data-k="lamp">' + n5e(x, 482, 38, 6, 'fill="url(#src-ao)"');
    // three arms, the candles burnt to different lengths, the flames cold
    s += n5p('M' + (x - 24) + ',480 h48 l-9,-10 h-30 Z', 'fill="' + tr + '"') + n5r(x - 4, 396, 8, 76, 'fill="' + tr + '"') + n5e(x, 440, 9, 5, 'fill="' + tr + '"') + n5e(x, 418, 7, 4, 'fill="' + tr + '"');
    s += n5p('M' + x + ',430 C' + (x - 20) + ',440 ' + (x - 36) + ',424 ' + (x - 36) + ',402 M' + x + ',430 C' + (x + 20) + ',440 ' + (x + 36) + ',424 ' + (x + 36) + ',402', 'stroke="' + t.th.trim + '" stroke-width="5" fill="none" stroke-linecap="round"');
    [[-36, 402, 30], [0, 392, 44], [36, 402, 22]].forEach(function (q, j) {
      var cx = x + q[0], cy = q[1];
      s += n5p('M' + (cx - 9) + ',' + (cy - 4) + ' h18 l-3,8 h-12 Z', 'fill="' + tr + '"') + n5r(cx - 4.4, cy - 4 - q[2], 8.8, q[2], 'fill="#e6e2d6"') + n5r(cx + 1, cy - 4 - q[2], 3.4, q[2], 'fill="#000" opacity=".18"') + n5p('M' + (cx - 4.4) + ',' + (cy - 4 - q[2]) + ' q-2.4,8 0,' + (9 + j * 4) + ' q2.4,3 2,-3 Z', 'fill="#f4f0e4"') + n5p('M' + cx + ',' + (cy - 4 - q[2]) + ' v-4', 'stroke="#1a1a1a" stroke-width="1"');
      s += n5c(cx, cy - 16 - q[2], 22, 'fill="url(#cm-glow)" opacity=".7" class="cm-pulse"') + n5flame(cx, cy - 7 - q[2], 16, j, '#cfe6ff', .95) + n5flame(cx, cy - 7 - q[2], 9, j + 1, '#ffffff', .9);
    });
    return { back: '', front: s + '<circle cx="' + x + '" cy="360" r="130" fill="url(#cm-bloom)" class="cm-pulse" opacity=".7"/></g>' };
  };
  PROPS.ghost = function (t, p) {
    var a = mix(t.th.accent, '#ffffff', .55);
    // someone who walks the room and is gone before you are sure
    return n5g('class="src-nohit"', n5g('class="n5-haunt" style="--hx0:-250px;--hx1:250px;--ho:.5;animation-duration:27s;animation-delay:-6s"', n5g('transform="translate(800,300) scale(1.9)"', n5g('class="n5-hover" style="animation-duration:5s"', n5ghost(a)))) +
      n5g('class="n5-haunt" style="--hx0:30px;--hx1:-30px;--ho:.4;animation-duration:15s;animation-delay:-9s"', n5g('transform="translate(1150,330) scale(1.1)"', n5ghost(a))));
  };
  PROPS.portrait = function (t, p) {
    var cx = 1104, cy = 212, s;
    // a sitter nobody remembers, hung a little off true; the eyes do not stay where they were painted
    s = n5e(cx + 5, cy + 8, 56, 72, 'fill="#000" opacity=".5" filter="url(#src-blur5)"') + n5p('M' + cx + ',' + (cy - 100) + ' L' + (cx - 24) + ',' + (cy - 62) + ' M' + cx + ',' + (cy - 100) + ' L' + (cx + 24) + ',' + (cy - 62), 'stroke="#1a1612" stroke-width="1.2"') + n5c(cx, cy - 100, 2.4, 'fill="#1a1612"') +
      n5e(cx, cy, 54, 70, 'fill="url(#cm-trim)" stroke="#0c0a08" stroke-width="2"') + n5e(cx, cy, 47, 63, 'fill="none" stroke="#000" stroke-opacity=".45" stroke-width="2"') + n5e(cx, cy, 41, 57, 'fill="#14100e"') +
      '<clipPath id="n5-ptc"><ellipse cx="' + cx + '" cy="' + cy + '" rx="41" ry="57"/></clipPath><g clip-path="url(#n5-ptc)">' + n5e(cx, cy - 10, 60, 50, 'fill="#3a3024" opacity=".6" filter="url(#src-blur8)"') +
      n5p('M' + (cx - 40) + ',' + (cy + 60) + ' Q' + (cx - 36) + ',' + (cy + 18) + ' ' + (cx - 10) + ',' + (cy + 12) + ' h20 Q' + (cx + 36) + ',' + (cy + 18) + ' ' + (cx + 40) + ',' + (cy + 60) + ' Z', 'fill="#0e0c10"') + n5p('M' + (cx - 10) + ',' + (cy + 12) + ' l10,14 l10,-14 l-3,-8 h-14 Z', 'fill="#d8d0c0"') +
      n5e(cx, cy - 12, 16, 21, 'fill="#c8bca8"') + n5e(cx + 6, cy - 10, 10, 19, 'fill="#000" opacity=".18"') +
      n5p('M' + (cx - 17) + ',' + (cy - 14) + ' Q' + (cx - 20) + ',' + (cy - 40) + ' ' + cx + ',' + (cy - 38) + ' Q' + (cx + 20) + ',' + (cy - 40) + ' ' + (cx + 17) + ',' + (cy - 12) + ' Q' + (cx + 12) + ',' + (cy - 28) + ' ' + cx + ',' + (cy - 29) + ' Q' + (cx - 12) + ',' + (cy - 28) + ' ' + (cx - 17) + ',' + (cy - 14) + ' Z', 'fill="#1a1412"') +
      n5e(cx - 6.4, cy - 14, 4.4, 2.2, 'fill="#e8e4d8"') + n5e(cx + 6.4, cy - 14, 4.4, 2.2, 'fill="#e8e4d8"') + n5g('class="n5-peer"', n5c(cx - 6.4, cy - 14, 1.6, 'fill="#0a0808"') + n5c(cx + 6.4, cy - 14, 1.6, 'fill="#0a0808"')) +
      n5p('M' + (cx - 11) + ',' + (cy - 19) + ' h9 M' + (cx + 2) + ',' + (cy - 19) + ' h9', 'stroke="#1a1412" stroke-width="1.3"') + n5p('M' + cx + ',' + (cy - 12) + ' v8 h3', 'stroke="#000" stroke-opacity=".3" stroke-width="1" fill="none"') + n5p('M' + (cx - 4) + ',' + (cy - 1) + ' h8', 'stroke="#5a3a34" stroke-width="1.3"') +
      n5e(cx - 14, cy - 28, 20, 30, 'fill="#fff" opacity=".05"') + '</g>';
    return n5g('class="src-nohit" transform="rotate(-3 ' + cx + ' ' + cy + ')"', s);
  };
  PROPS.phonograph = function (t, p) {
    var x = p.x + 6, y = 492, tr = t.th.trim, wd = mix(t.th.wood, '#1a0c08', .2), dk = mix(tr, '#000', .6), s = n5e(x, y + 3, 62, 8, 'fill="url(#src-ao)"'), i;
    // a box of dark wood, the record still going round, the horn turned to the room
    s += n5r(x - 44, y - 28, 88, 28, 'rx="2" fill="' + wd + '"') + n5r(x - 44, y - 28, 88, 2, 'fill="#fff" opacity=".14"') + n5r(x - 36, y - 20, 72, 14, 'fill="none" stroke="#000" stroke-opacity=".5" stroke-width="1.4"') + n5p('M' + (x - 44) + ',' + (y - 28) + ' l8,-10 h72 l8,10 Z', 'fill="' + mix(wd, '#ffffff', .12) + '"');
    s += n5p('M' + (x + 44) + ',' + (y - 14) + ' h10 v-9', 'stroke="' + tr + '" stroke-width="2.4" fill="none"') + n5c(x + 54, y - 25, 3, 'fill="' + tr + '"');
    s += n5g('transform="translate(' + x + ',' + (y - 35) + ') scale(1,.28)"', n5g('class="n5-turn" style="animation-duration:2.8s"', n5c(0, 0, 32, 'fill="#0a0a0c"') + n5c(0, 0, 26, 'fill="none" stroke="#3a3a40" stroke-width="1"') + n5c(0, 0, 19, 'fill="none" stroke="#3a3a40" stroke-width="1"') + n5c(0, 0, 10, 'fill="#8a2a2a"') + n5p('M0,0 L30,-9 A32,32 0 0 1 30,9 Z', 'fill="#fff" opacity=".2"') + n5r(3, -2, 5, 4, 'fill="#e8d8a0"')));
    s += n5p('M' + (x + 12) + ',' + (y - 38) + ' C' + (x + 14) + ',' + (y - 58) + ' ' + (x - 6) + ',' + (y - 64) + ' ' + (x - 22) + ',' + (y - 68) + ' L' + (x - 46) + ',' + (y - 92) + ' Q' + (x - 64) + ',' + (y - 72) + ' ' + (x - 46) + ',' + (y - 48) + ' L' + (x - 22) + ',' + (y - 60) + ' C' + (x - 4) + ',' + (y - 56) + ' ' + (x + 18) + ',' + (y - 54) + ' ' + (x + 18) + ',' + (y - 38) + ' Z', 'fill="url(#cm-trim)" stroke="' + dk + '" stroke-width="1.2"');
    s += n5e(x - 49, y - 70, 9, 23, 'fill="#14100c" stroke="' + tr + '" stroke-width="2" transform="rotate(-6 ' + (x - 49) + ' ' + (y - 70) + ')"') + n5p('M' + (x - 40) + ',' + (y - 84) + ' Q' + (x - 24) + ',' + (y - 74) + ' ' + (x - 8) + ',' + (y - 66), 'stroke="#fff" stroke-opacity=".4" stroke-width="1.4" fill="none"');
    for (i = 0; i < 3; i++) s += '<text class="n5-fb n5-puff" x="' + (x - 62) + '" y="' + (y - 70) + '" font-size="17" font-family="Georgia,serif" fill="' + t.th.accent + '" style="--fx:' + (-10 + i * 22) + 'px;--fy:-' + (70 + i * 16) + 'px;animation-duration:' + (6 + i) + 's;animation-delay:-' + (i * 2.2) + 's">' + (i % 2 ? '♫' : '♪') + '</text>';
    return s;
  };


  /* ---- The glasshouse: iron and glass with the garden beyond every pane,
     doors standing open onto a walk and a fountain, goods on a wrought-iron
     plant stand and a potting bench for a counter. ---- */
  CM_OPT.shape.push('conservatory'); CM_OPT.wall.push('glazing'); CM_OPT.opening.push('garden-door'); CM_OPT.display.push('plant-stand'); CM_OPT.counter.push('potting-bench');
  CM_WALLFQ.glazing = [.03, 1.2, 341]; CM_OPEN_INFO['garden-door'] = 'The garden door';
  n5prop('hanging-baskets', 'ceil', 'back'); n5prop('palm', 'tallR', 'front', 0, true); n5prop('watering-can', 'cL', 'front', 140);

  // The garden's colours at this hour: full by day, sunk toward dusk or night.
  function n5gcol(t, c) { var k = t.time === 'night' ? .72 : t.time === 'dusk' ? .34 : t.time === 'rain' ? .3 : t.time === 'dawn' ? .2 : 0; return k ? mix(c, t.time === 'dusk' ? '#3a2444' : '#0a1626', k) : c; }
  // A flowerpot: a tapered body under a rolled rim, standing on y.
  function n5pot(x, y, w, h, c) {
    c = c || '#b8623a';
    return n5p('M' + f1(x - w * .42) + ',' + f1(y - h * .78) + ' L' + f1(x - w * .3) + ',' + f1(y) + ' H' + f1(x + w * .3) + ' L' + f1(x + w * .42) + ',' + f1(y - h * .78) + ' Z', 'fill="' + c + '"') + n5p('M' + f1(x + w * .08) + ',' + f1(y - h * .78) + ' L' + f1(x + w * .06) + ',' + f1(y) + ' H' + f1(x + w * .3) + ' L' + f1(x + w * .42) + ',' + f1(y - h * .78) + ' Z', 'fill="#000" opacity=".25"') +
      n5r(x - w * .5, y - h, w, h * .24, 'rx="' + f1(h * .08) + '" fill="' + mix(c, '#ffffff', .12) + '"') + n5r(x - w * .5, y - h, w, h * .06, 'fill="#fff" opacity=".25"') + n5r(x - w * .46, y - h * .76, w * .92, h * .07, 'fill="#000" opacity=".3"');
  }
  // A small butterfly that wanders about where it is put.
  function n5fly(x, y, c, fx, dur) {
    return n5g('class="n5-flit" style="--fx:' + fx + 'px;animation-duration:' + dur + 's"', n5g('transform="translate(' + x + ',' + y + ')"', n5g('class="n5-fb n5-flap" style="animation-duration:.45s"', n5p('M0,0 q-7,-8 -9,0 q2,6 9,0 q7,-8 9,0 q-2,6 -9,0 Z', 'fill="' + c + '"'))));
  }

  WALLS.glazing = function (t) {
    var r = rng(341), s = cmSky(t, 'n5-gsky', -300, -150, 1800, 620), i, x, y, g = '', far = n5gcol(t, '#7aa888'), mid = n5gcol(t, '#3c7a48'), near = n5gcol(t, '#1e5230'), bar = '#e9efe4', fl = ['#ff8ab0', '#ffe070', '#ffffff', '#c890ff', '#ff6a5a'];
    // the garden, soft through old glass: far trees, cypresses, a hedge with the beds in flower before it
    for (i = 0; i < 16; i++) g += n5e(-300 + i * 120 + r() * 60, 250 - r() * 70, 90 + r() * 70, 80 + r() * 70, 'fill="' + mix(far, mid, r() * .5) + '"');
    for (i = 0; i < 9; i++) { x = -260 + i * 200 + r() * 80; g += n5p('M' + f1(x - 18) + ',380 Q' + f1(x - 24) + ',240 ' + f1(x) + ',' + f1(140 + r() * 50) + ' Q' + f1(x + 24) + ',240 ' + f1(x + 18) + ',380 Z', 'fill="' + near + '"'); }
    g += n5r(-300, 300, 1800, 170, 'fill="' + mid + '"') + n5r(-300, 300, 1800, 12, 'fill="' + mix(mid, '#ffffff', .2) + '"');
    for (i = 0; i < 150; i++) g += n5c(-300 + r() * 1800, 322 + r() * 56, 3 + r() * 5, 'fill="' + n5gcol(t, fl[i % 5]) + '" opacity=".85"');
    s += n5g('filter="url(#src-blur5)"', g);
    // the glass: a bloom of damp low down, runs where it has gathered, the sky caught in a pane or two
    s += n5r(-300, 210, 1800, 170, 'fill="#fff" opacity=".14" filter="url(#src-blur14)"');
    for (i = 0; i < 40; i++) { x = -300 + r() * 1800; y = 200 + r() * 110; s += n5p('M' + f1(x) + ',' + f1(y) + ' q' + f1((r() - .5) * 4) + ',' + f1(20 + r() * 30) + ' 0,' + f1(40 + r() * 50), 'stroke="#fff" stroke-opacity="' + f1(.14 + r() * .2) + '" stroke-width="' + f1(1 + r()) + '" fill="none" stroke-linecap="round"'); }
    for (x = -300; x < 1500; x += 110) { if (r() < .5) s += n5p('M' + (x + 14) + ',' + 236 + ' L' + (x + 70) + ',114 h22 L' + (x + 36) + ',236 Z', 'fill="#fff" opacity=".07"'); }
    // glazing bars, painted and beginning to need it again
    for (x = -300; x < 1500; x += 110) s += n5r(x + 3, -150, 3, 524, 'fill="#000" opacity=".22"') + n5r(x - 3, -150, 6, 524, 'fill="' + bar + '"') + n5r(x - 3, -150, 1.4, 524, 'fill="#fff"');
    [110, 240].forEach(function (yy) { s += n5r(-300, yy + 2, 1800, 3, 'fill="#000" opacity=".2"') + n5r(-300, yy - 2, 1800, 4, 'fill="' + bar + '"'); });
    // a dwarf wall of brick under a stone sill
    s += n5g('filter="url(#cm-wall)"', n5r(-300, 372, 1800, 100, 'fill="#5a3a2c"') + n5stones(r, -300, 376, 1500, 470, 16, 44, 10, '#a8603e', '#6a3a28', '#c8805a', .4)) + n5r(-300, 362, 1800, 13, 'fill="#d8d4c4"') + n5r(-300, 362, 1800, 2, 'fill="#fff" opacity=".7"') + n5r(-300, 375, 1800, 9, 'fill="#000" opacity=".4" filter="url(#src-blur3)"');
    for (i = 0; i < 10; i++) s += mpMossPad(r, -260 + r() * 1700, 376 + r() * 3, 14 + r() * 20, 5, '#4a7a3a');
    return '<defs>' + mp3Defs() + '</defs>' + s;
  };
  SHAPES.conservatory = function (t) {
    var s = WALLS[t.th.wall](t), x, g = '', bar = '#e9efe4', lace = '';
    s += '<defs>' + lin('n5-shaft', 0, [[0, '#7a8a7e'], [.25, '#f6faf2'], [.5, '#d4dcd0'], [.85, '#8a988c'], [1, '#56625a']]) + '</defs>';
    // the roof plate, and a valance of iron lace under it
    s += n5r(-300, -150, 1800, 180, 'fill="#dfe6da"') + n5r(-300, 22, 1800, 8, 'fill="#aab4a8"') + n5r(-300, 30, 1800, 3, 'fill="#fff"');
    for (x = -296; x < 1500; x += 40) { lace += 'M' + x + ',33 q20,34 40,0 M' + (x + 8) + ',33 q12,20 24,0 M' + (x + 20) + ',50 v8 '; g += n5c(x + 20, 60, 2.6, 'fill="' + bar + '"'); }
    s += n5p(lace, 'stroke="#000" stroke-opacity=".3" stroke-width="3" fill="none" transform="translate(2,4)" filter="url(#src-blur3)"') + n5p(lace, 'stroke="' + bar + '" stroke-width="3" fill="none"') + g;
    // slim iron columns with a scrolled bracket at the head of each
    [[4, 1], [1178, -1]].forEach(function (q) {
      var bx = q[0] + (q[1] > 0 ? 18 : 0), k = q[1];
      s += n5column(q[0], 18, 30, 474) + n5p('M' + bx + ',122 q' + (k * 62) + ',-10 ' + (k * 74) + ',-86 M' + bx + ',88 q' + (k * 26) + ',2 ' + (k * 30) + ',-24 a10,10 0 1 ' + (k > 0 ? 0 : 1) + ' ' + (k * -13) + ',5', 'stroke="' + bar + '" stroke-width="4" fill="none" stroke-linecap="round"');
    });
    return s;
  };
  OPENINGS['garden-door'] = function (t) {
    var d = 'M826,452 V110 Q826,70 912,70 Q998,70 998,110 V452 Z', r = rng(343), s, fr, i, a, x, y, day = !t.time || t.time === 'day',
      far = n5gcol(t, '#7aa888'), mid = n5gcol(t, '#3c7a48'), near = n5gcol(t, '#1e5230'), lite = n5gcol(t, '#5a9a50'), gravel = n5gcol(t, '#e0d0a8'), bar = '#e9efe4', rose = '', ac = t.th.accent;
    s = cmSky(t, 'n5-dsky', 810, 60, 204, 400);
    if (day) s += n5e(940, 150, 70, 50, 'fill="#fff8d0" opacity=".5" filter="url(#src-blur8)"');
    // far trees, then the walk: gravel between clipped hedges, running out to a fountain
    for (i = 0; i < 8; i++) s += n5e(814 + i * 28, 248 - r() * 20, 24 + r() * 14, 34 + r() * 20, 'fill="' + mix(far, mid, r() * .4) + '"');
    s += n5r(810, 262, 204, 200, 'fill="' + mid + '"') + n5p('M899,270 H925 L992,460 H832 Z', 'fill="' + gravel + '"') + n5p('M899,270 H910 L900,460 H832 Z', 'fill="#000" opacity=".08"');
    s += n5p('M810,280 L897,266 V280 L826,460 H810 Z', 'fill="' + near + '"') + n5p('M810,280 L897,266 V270 L810,288 Z', 'fill="' + lite + '"') + n5p('M1014,280 L927,266 V280 L998,460 H1014 Z', 'fill="' + near + '"') + n5p('M1014,280 L927,266 V270 L1014,288 Z', 'fill="' + lite + '"');
    [[886, 302, 7], [868, 350, 11], [846, 412, 16], [938, 302, 7], [956, 350, 11], [978, 412, 16]].forEach(function (q) { s += n5r(q[0] - q[2] * .12, q[1], q[2] * .24, q[2] * 1.5, 'fill="#2a1c10"') + n5pot(q[0], q[1] + q[2] * 2.2, q[2] * 1.3, q[2] * .9, n5gcol(t, '#b8623a')) + n5c(q[0], q[1] - q[2] * .2, q[2], 'fill="' + near + '"') + n5c(q[0] - q[2] * .3, q[1] - q[2] * .5, q[2] * .5, 'fill="' + lite + '" opacity=".7"'); });
    // the fountain at the end of it, playing
    s += n5e(912, 276, 20, 5, 'fill="' + n5gcol(t, '#d8dcd8') + '"') + n5e(912, 275, 16, 3, 'fill="' + n5gcol(t, '#7ab0d0') + '"') + n5r(909.5, 256, 5, 20, 'fill="' + n5gcol(t, '#c8ccc8') + '"') + n5e(912, 256, 11, 3, 'fill="' + n5gcol(t, '#d8dcd8') + '"') + n5p('M912,256 V236', 'stroke="#fff" stroke-opacity=".9" stroke-width="1.6"') + n5p('M912,237 q-11,1 -12,19 M912,237 q11,1 12,19 M912,237 q-5,2 -6,19 M912,237 q5,2 6,19', 'stroke="#fff" stroke-opacity=".7" stroke-width="1.1" fill="none" class="n5-shimmer" style="animation-duration:1.3s"');
    for (i = 0; i < 5; i++) s += n5c(902 + i * 5, 256, 1.1, 'class="n5-streak" fill="#fff" style="--fy:18px;animation-duration:' + f1(1.2 + i * .17) + 's;animation-delay:-' + f1(i * .31) + 's"');
    s += n5fly(866, 330, n5gcol(t, '#ffe070'), 26, 11) + n5fly(950, 300, n5gcol(t, '#ffffff'), -22, 14) + cmRainIn(t, 810, 60, 204, 400, 345);
    // the leaves of the door folded back against the reveals
    s += n5p('M826,104 L850,124 V452 H826 Z', 'fill="#dff2f4" fill-opacity=".2" stroke="' + bar + '" stroke-width="3"') + n5p('M826,200 L850,208 M826,290 L850,294 M826,380 L850,381', 'stroke="' + bar + '" stroke-width="2"') + n5p('M998,104 L974,124 V452 H998 Z', 'fill="#dff2f4" fill-opacity=".2" stroke="' + bar + '" stroke-width="3"') + n5p('M998,200 L974,208 M998,290 L974,294 M998,380 L974,381', 'stroke="' + bar + '" stroke-width="2"');
    // an iron frame, and a rose trained up one side and over
    fr = n5p(d, 'fill="none" stroke="#2a3a30" stroke-width="18" stroke-linejoin="round"') + n5p(d, 'fill="none" stroke="' + bar + '" stroke-width="11" stroke-linejoin="round"') + n5p(d, 'fill="none" stroke="#fff" stroke-opacity=".6" stroke-width="1.4" transform="translate(-2,-2)"');
    for (i = 0; i < 46; i++) {
      if (i < 20) { x = 820 + (r() - .5) * 8; y = 400 - i * 14.5; } else { a = Math.PI + (i - 20) / 26 * 2.5; x = 912 + Math.cos(a) * 94 + (r() - .5) * 6; y = 112 + Math.sin(a) * 50 + (r() - .5) * 6; }
      rose += mpLeaf(x, y, 10 + r() * 5, r() * 360, r() < .5 ? '#2e6a3a' : '#5a9a48', r() < .4 ? '#d8f0b0' : '') + mpLeaf(x, y, 9 + r() * 4, r() * 360, '#1e5230', '') + (i % 3 === 0 ? mpFlower(x + (r() - .5) * 10, y + (r() - .5) * 8, 5 + r() * 2.4, r() < .7 ? ac : '#ffffff') : '');
    }
    fr += n5p('M820,452 V112', 'stroke="#3a2c1c" stroke-width="2.4" fill="none"') + rose;
    return mpOpen(d, s, fr, day ? n5g('opacity=".12" pointer-events="none"', n5e(912, 330, 170, 200, 'fill="#fff8d0" filter="url(#src-blur14)"')) : '');
  };
  DISPLAYS['plant-stand'] = function (t) {
    var r = rng(345), ir = '#1c241e', hi = '#6a7a6e', s = '<defs>' + mp3Defs() + '</defs>', g1 = '#2e6a3a', g2 = '#6aa04a', a = t.th.accent, x, i, sc = '', cr;
    // uprights of twisted iron with a scrolled crest between them
    cr = 'M40,74 Q150,22 265,60 Q380,22 490,74 M265,60 q-30,-36 -60,-10 a12,12 0 1 0 15,10 M265,60 q30,-36 60,-10 a12,12 0 1 1 -15,10 M265,60 V26';
    s += n5p(cr, 'stroke="#000" stroke-opacity=".3" stroke-width="5" fill="none" transform="translate(4,6)" filter="url(#src-blur3)"') + n5p(cr, 'stroke="' + ir + '" stroke-width="5" fill="none" stroke-linecap="round"') + n5p(cr, 'stroke="' + hi + '" stroke-width="1.2" fill="none" transform="translate(-1.2,-1.4)"') + n5p('M265,26 l-6,-12 l6,-14 l6,14 Z', 'fill="' + ir + '"');
    [40, 490].forEach(function (ux) {
      s += n5r(ux + 3, 70, 5, 392, 'fill="#000" opacity=".25" filter="url(#src-blur3)"') + n5r(ux - 3, 70, 6, 392, 'fill="' + ir + '"') + n5r(ux - 3, 70, 1.6, 392, 'fill="' + hi + '"');
      for (i = 80; i < 456; i += 12) s += n5p('M' + (ux - 3) + ',' + i + ' l6,6', 'stroke="#000" stroke-opacity=".6" stroke-width="1.2"');
      s += n5p('M' + ux + ',70 l-6,-14 l6,-16 l6,16 Z', 'fill="' + ir + '"');
    });
    // three shelves of iron on scrolled brackets, a scalloped edge under each
    CM_ROWS.forEach(function (ry) {
      s += n5r(34, ry + 8, 462, 12, 'fill="#000" opacity=".3" filter="url(#src-blur5)"') + n5r(34, ry, 462, 5, 'fill="' + ir + '"') + n5r(34, ry, 462, 1.2, 'fill="' + hi + '"');
      for (x = 46; x < 480; x += 22) sc += 'M' + x + ',' + (ry + 5) + ' q11,12 22,0 ';
      s += n5p('M43,' + (ry + 5) + ' q4,30 30,34 a8,8 0 1 0 -8,-10 M487,' + (ry + 5) + ' q-4,30 -30,34 a8,8 0 1 1 8,-10', 'stroke="' + ir + '" stroke-width="3" fill="none" stroke-linecap="round"');
    });
    s += n5p(sc, 'stroke="' + ir + '" stroke-width="1.6" fill="none"');
    // what grows on it: ivy down the uprights, ferns in pots on the crest
    s += mpVine(r, 42, 74, 150, g1, g2, a) + mpVine(r, 488, 74, 190, g1, g2, a) + mpVine(r, 44, 300, 90, g1, g2, '') + mpVine(r, 486, 200, 110, g1, g2, a) + mpVine(r, 36, 180, 80, g1, g2, a);
    [[150, 44], [380, 44]].forEach(function (q) { s += mpFernClump(r, q[0], q[1] - 14, 36, g1, g2) + n5pot(q[0], q[1], 24, 17) + mpVine(r, q[0] + 9, q[1] - 12, 44, g1, g2, a); });
    return { art: s, slots: cmGrid(), zone: [34, 96, 454, 320], wallsh: false, decor: true, label: 'The plant stand' };
  };
  COUNTERS['potting-bench'] = function (t) {
    var r = rng(347), s = cmTop(t, 'M-300,452 H1500 V512 H-300 Z'), x, i, j, k, a = t.th.accent, g1 = '#2e6a3a', g2 = '#6aa04a', n = 0;
    s += '<defs>' + mp3Defs() + '</defs><clipPath id="cm-topclip"><rect x="-300" y="452" width="1800" height="60"/></clipPath>';
    // crumbs of soil where the last pot was filled
    for (i = 0; i < 70; i++) s += n5c(-300 + r() * 1800, 456 + r() * 50, .8 + r() * 1.6, 'fill="#1a0e06" opacity="' + f1(.3 + r() * .4) + '"');
    // an apron, and the dark under the bench
    s += n5p('M-300,506 H1500 V548 H-300 Z', 'fill="#000" filter="url(#cm-beam)"') + n5r(-300, 506, 1800, 2.4, 'fill="' + t.lc + '" opacity=".4"') + n5r(-300, 548, 1800, 360, 'fill="#0d130e"') + n5r(-300, 548, 1800, 34, 'fill="#000" opacity=".6" filter="url(#src-blur8)"');
    // the lower shelf and what is kept on it: pots in stacks, a fern, a sack of leaf mould
    s += n5r(-300, 664, 1800, 14, 'fill="#000" filter="url(#cm-beam)"') + n5r(-300, 664, 1800, 2, 'fill="' + t.lc + '" opacity=".3"') + n5r(-300, 678, 1800, 240, 'fill="#070a07"');
    for (x = -250; x < 1500; x += 96 + r() * 30, n++) {
      if (n % 4 === 0) { k = 2 + Math.floor(r() * 4); for (j = 0; j < k; j++) s += n5pot(x, 664 - j * 9, 46, 40); }
      else if (n % 4 === 1) s += mpFernClump(r, x, 622, 52, g1, g2) + n5pot(x, 664, 54, 46, '#9a5230');
      else if (n % 4 === 2) s += n5p('M' + (x - 32) + ',664 q-8,-40 6,-66 q26,-12 52,0 q14,26 6,66 Z', 'fill="#b09a70"') + n5p('M' + (x + 4) + ',664 q4,-40 -2,-70 q16,2 22,4 q14,26 6,66 Z', 'fill="#000" opacity=".2"') + n5p('M' + (x - 18) + ',600 q18,8 36,0', 'stroke="#5a4a2c" stroke-width="2.4" fill="none"') + n5p('M' + (x - 16) + ',634 h30 M' + (x - 12) + ',644 h22', 'stroke="#5a4a2c" stroke-opacity=".6" stroke-width="2"');
      else { for (j = 0; j < 5; j++) s += n5p('M' + (x - 14 + j * 7) + ',628 q' + f1((r() - .5) * 8) + ',-16 ' + f1((r() - .5) * 10) + ',-' + f1(22 + r() * 16), 'stroke="' + g1 + '" stroke-width="1.4" fill="none"') + mpFlower(x - 14 + j * 7 + (r() - .5) * 8, 604 - r() * 16, 5 + r() * 2, j % 2 ? a : '#fff0a0'); s += n5pot(x, 664, 44, 38); }
    }
    for (x = -280; x < 1500; x += 300) s += cmPost(x, 26, 548, 910);
    // a rose has got up one leg
    s += mpVine(r, 326, 548, 110, g1, g2, a) + mpVine(r, 340, 556, 70, g1, g2, a) + mpFlower(330, 590, 7, a) + mpFlower(342, 570, 6, a);
    return s + n5r(-300, 512, 1800, 20, 'fill="#000" opacity=".4" filter="url(#src-blur5)"');
  };
  PROPS['hanging-baskets'] = function (t, p) {
    var r = rng(349), s = '', g1 = '#2e6a3a', g2 = '#6aa04a', a = t.th.accent;
    // baskets of moss on chains, ferns standing out of them and everything else trailing
    [[742, 122, 1], [1062, 108, .85], [1146, 152, 1]].forEach(function (q, j) {
      var x = q[0], y = q[1], g;
      g = n5p('M' + x + ',36 L' + (x - 24) + ',' + y + ' M' + x + ',36 L' + (x + 24) + ',' + y + ' M' + x + ',36 V' + y, 'stroke="#2a3a30" stroke-width="1.3" stroke-dasharray="3 2" fill="none"') + mpFernClump(r, x, y + 2, 46 * q[2], g1, g2);
      g += n5p('M' + (x - 26) + ',' + y + ' h52 q-2,22 -26,24 q-24,-2 -26,-24 Z', 'fill="#5a4630"') + n5p('M' + (x + 4) + ',' + y + ' h22 q-2,22 -26,24 Z', 'fill="#000" opacity=".25"') + n5p('M' + (x - 22) + ',' + (y + 7) + ' q22,10 44,0 M' + (x - 16) + ',' + (y + 15) + ' q16,8 32,0', 'stroke="#2a1e10" stroke-width="1.2" fill="none"') + n5r(x - 27, y - 2, 54, 4, 'rx="2" fill="#1c241e"');
      g += mpVine(r, x - 20, y + 4, 60, g1, g2, a) + mpVine(r, x + 16, y + 4, 78, g1, g2, a) + mpVine(r, x - 2, y + 20, 44, g1, g2, '') + mpFlower(x - 10, y - 6, 6, a) + mpFlower(x + 12, y - 10, 5, '#ffffff') + mpFlower(x + 2, y - 2, 5, a);
      s += n5g('class="n5-fbt n5-sway" style="animation-duration:' + f1(7 + j * 1.3) + 's;animation-delay:-' + (j * 2) + 's"', g);
    });
    return n5g('class="src-nohit"', s);
  };
  PROPS.palm = function (t, p) {
    var x = p.x, y = 488, cx = x + 2, cy = y - 138, s, i, c1 = '#2a6a38', c2 = '#5a9a48';
    // a palm in a glazed tub, every frond moving a little on its own
    s = n5e(x, y + 2, 50, 8, 'fill="url(#src-ao)"') + n5p('M' + (x - 5) + ',' + (y - 50) + ' q-5,-44 2,-90 h9 q-5,46 2,90 Z', 'fill="#6a5238"');
    for (i = 0; i < 8; i++) s += n5p('M' + (x - 6) + ',' + (y - 58 - i * 10) + ' q6,4 12,0', 'stroke="#3a2c1c" stroke-width="1.4" fill="none"');
    [[-168, 96], [-140, 112], [-112, 104], [-86, 118], [-60, 106], [-34, 112], [-8, 94]].forEach(function (q, j) {
      s += n5g('class="n5-vb n5-sway" style="transform-origin:' + cx + 'px ' + cy + 'px;animation-duration:' + f1(6 + j * .9) + 's;animation-delay:-' + f1(j * 1.3) + 's"', mpFern(cx, cy, q[1], q[0], j % 2 ? c1 : mix(c1, '#000', .2), c2));
    });
    s += n5c(cx, cy, 7, 'fill="#4a3824"') + n5pot(x, y, 66, 58, '#3a6a8a') + n5r(x - 26, y - 30, 52, 4, 'fill="#e8e0c8" opacity=".5"') + n5r(x - 24, y - 20, 48, 2, 'fill="#e8e0c8" opacity=".35"');
    return s;
  };
  PROPS['watering-can'] = function (t, p) {
    var x = p.x - 14, y = 492, s, i, g1 = '#2e6a3a', g2 = '#8ac060';
    // a copper can with a long spout, still dripping, and a tray of seedlings beside it
    s = '<defs>' + lin('n5-can', 0, [[0, '#3a1608'], [.2, '#b0562a'], [.36, '#ffc89a'], [.55, '#b0562a'], [1, '#3a1608']]) + '</defs>' + n5e(p.x, y + 3, 70, 8, 'fill="url(#src-ao)"');
    s += n5p('M' + (x + 20) + ',' + (y - 40) + ' q30,2 24,30 q-4,12 -22,6', 'stroke="#8a4220" stroke-width="5" fill="none" stroke-linecap="round"') + n5p('M' + (x - 23) + ',' + (y - 14) + ' L' + (x - 62) + ',' + (y - 50) + ' l5,-5 L' + (x - 22) + ',' + (y - 28) + ' Z', 'fill="#a8522a" stroke="#3a1608" stroke-width="1"');
    s += n5p('M' + (x - 22) + ',' + y + ' l-3,-46 h50 l-3,46 Z', 'fill="url(#n5-can)"') + n5e(x, y - 46, 25, 5, 'fill="#3a1608"') + n5e(x, y - 46, 25, 5, 'fill="none" stroke="#ffc89a" stroke-opacity=".6" stroke-width="1.2"') + n5p('M' + (x - 16) + ',' + (y - 47) + ' q16,-28 32,0', 'stroke="#8a4220" stroke-width="4" fill="none" stroke-linecap="round"') + n5r(x - 24, y - 16, 48, 2.4, 'fill="#3a1608" opacity=".5"');
    s += n5e(x - 62, y - 54, 9, 5, 'fill="#6a3018" stroke="#3a1608" stroke-width="1" transform="rotate(-42 ' + (x - 62) + ' ' + (y - 54) + ')"');
    for (i = 0; i < 3; i++) s += n5e(x - 67 + i * 2, y - 52, 1.3, 2.2, 'class="n5-streak" fill="#bfe4ff" style="--fy:48px;animation-duration:' + f1(2.2 + i * .5) + 's;animation-delay:-' + f1(i * .8) + 's"');
    s += n5e(x - 66, y - 1, 9, 2, 'fill="#bfe4ff" opacity=".5" class="n5-shimmer"');
    // the tray
    s += n5p('M' + (x + 38) + ',' + (y - 13) + ' h62 l-4,13 h-54 Z', 'fill="#4a3a28"') + n5r(x + 38, y - 14, 62, 3, 'fill="#6a563c"') + n5r(x + 42, y - 13, 54, 3, 'fill="#1a0e06"');
    for (i = 0; i < 6; i++) { var sx = x + 46 + i * 9.4, h = 8 + (i * 7 % 5) * 2.4; s += n5p('M' + sx + ',' + (y - 12) + ' v-' + h, 'stroke="' + g1 + '" stroke-width="1.2"') + mpLeaf(sx, y - 12 - h, 6, -30, g2, '') + mpLeaf(sx, y - 12 - h, 6, -150, g1, ''); }
    return s;
  };

  CM_WHERE['ice-ledges'] = 'on the ice ledges'; CM_WHERE.shells = 'in the shells'; CM_WHERE['carpet-wall'] = 'on the carpet wall'; CM_WHERE['cargo-nets'] = 'in the cargo nets'; CM_WHERE['crooked-shelves'] = 'on the crooked shelves'; CM_WHERE.cogs = 'on the cogs'; CM_WHERE.hoard = 'on the hoard'; CM_WHERE.clouds = 'on the clouds'; CM_WHERE.curio = 'under its glass'; CM_WHERE['plant-stand'] = 'on the plant stand';
  CM_STARTERS.push(
    { name: 'The Frost Hall', version: 1, skeleton: { shape: 'ice-cave', opening: 'aurora', display: 'ice-ledges', counter: 'ice-block' },
      theme: { wall: 'glacier', wood: '#5a6a7a', cloth: '#2a4a7a', accent: '#8ae0ff', trim: '#c8d8e8', light: 'frostfire', warmth: .2, outside: 'night' }, props: ['snowfall', 'ice-swan', 'crystal', 'drips'] },
    { name: 'The Drowned Arcade', version: 1, skeleton: { shape: 'reef', opening: 'sea-arch', display: 'shells', counter: 'wreck' },
      theme: { wall: 'deepwater', wood: '#3a5a5a', cloth: '#1a4a5a', accent: '#5fe8d0', trim: '#c8b070', light: 'jellyfish', warmth: .3, outside: 'day' }, props: ['fish', 'kelp', 'crab', 'caustics'] },
    { name: 'The Caravan Tent', version: 1, skeleton: { shape: 'tent', opening: 'tent-flap', display: 'carpet-wall', counter: 'carpet' },
      theme: { wall: 'tent-stripes', wood: '#6a4a2a', cloth: '#8a2a3a', accent: '#e8a030', trim: '#d8a840', light: 'pierced-lamp', warmth: .95, outside: 'dusk' }, props: ['glass-lamps', 'hookah', 'cobra', 'motes'] },
    { name: 'The Sky Barge', version: 1, skeleton: { shape: 'gondola', opening: 'cloud-sea', display: 'cargo-nets', counter: 'gunwale' },
      theme: { wall: 'open-sky', wood: '#6a4a2c', cloth: '#a83a2a', accent: '#ffd070', trim: '#c8a04a', light: 'lamp', warmth: .6, outside: 'day' }, props: ['pennants', 'propeller', 'grapnel'] },
    { name: "The Bog Witch's Hut", version: 1, skeleton: { shape: 'hut', opening: 'bog-window', display: 'crooked-shelves', counter: 'driftwood' },
      theme: { wall: 'wattle', wood: '#4a3a26', cloth: '#3a2a4a', accent: '#9ae04a', trim: '#8a7a4a', light: 'cauldron', warmth: .5, outside: 'night' }, props: ['herbs', 'toad', 'bottle-tree', 'cobwebs', 'fireflies'] },
    { name: 'The Gearworks', version: 1, skeleton: { shape: 'works', opening: 'clock-face', display: 'cogs', counter: 'engine' },
      theme: { wall: 'brass-plate', wood: '#4a3a2a', cloth: '#2a3a3a', accent: '#ffb040', trim: '#c89a4a', light: 'filament', warmth: .8, outside: 'night' }, props: ['gears', 'automaton', 'gauge', 'steam-puffs'] },
    { name: "The Dragon's Hoard", version: 1, skeleton: { shape: 'cavern', opening: 'dragon-eye', display: 'hoard', counter: 'coin-drift' },
      theme: { wall: 'lair', wood: '#3a2a1a', cloth: '#6a1a1a', accent: '#ffc840', trim: '#e8c050', light: 'brazier', warmth: .9, outside: 'night' }, props: ['dragon-tail', 'glints', 'smoke', 'embers'] },
    { name: 'The Sun Court', version: 1, skeleton: { shape: 'colonnade', opening: 'sun-disc', display: 'clouds', counter: 'marble-slab' },
      theme: { wall: 'marble', wood: '#b8a888', cloth: '#2a4a8a', accent: '#ffd860', trim: '#e8c860', light: 'sunlight', warmth: .8, outside: 'day' }, props: ['doves', 'laurel', 'lyre', 'orrery', 'motes'] },
    { name: 'The Haunted Parlour', version: 1, skeleton: { shape: 'parlour', opening: 'tall-mirror', display: 'curio', counter: 'sheeted' },
      theme: { wall: 'faded-paper', wood: '#3a2c28', cloth: '#3a2a4a', accent: '#9ad8e8', trim: '#8a8478', light: 'candelabra', warmth: .35, outside: 'night' }, props: ['ghost', 'portrait', 'phonograph', 'cobwebs', 'ghostlights', 'dust'] },
    { name: 'The Glasshouse', version: 1, skeleton: { shape: 'conservatory', opening: 'garden-door', display: 'plant-stand', counter: 'potting-bench' },
      theme: { wall: 'glazing', wood: '#6a5a44', cloth: '#2a5a3a', accent: '#ff8ab0', trim: '#c8b070', light: 'lamp', warmth: .7, outside: 'day' }, props: ['hanging-baskets', 'palm', 'watering-can', 'butterflies', 'motes'] }
  );
})();
