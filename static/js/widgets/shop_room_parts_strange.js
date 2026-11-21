/**
 * shop_room_parts_strange.js — places that are not a timber shop: a
 * ship's hold, a hollow tree, caves and tunnels, a fey glade, a volcano's
 * throat, open space, a white nowhere, something eldritch and a sanctum.
 *
 * A parts pack for the counter room (shop_room_counter.js). It loads after
 * that file and before shop_room.js, and adds its
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
  if (!K) return;
  var CM_CSS = K.CM_CSS, CM_LAYER = K.CM_LAYER, CM_LIGHT = K.CM_LIGHT, CM_OPEN_INFO = K.CM_OPEN_INFO, CM_OPT = K.CM_OPT, CM_PROPS = K.CM_PROPS, CM_STARTERS = K.CM_STARTERS, CM_WALLFQ = K.CM_WALLFQ, CM_WHERE = K.CM_WHERE, COUNTERS = K.COUNTERS, DISPLAYS = K.DISPLAYS, LIGHTS = K.LIGHTS, OPENINGS = K.OPENINGS, PROPS = K.PROPS, PROP_AT = K.PROP_AT, SHAPES = K.SHAPES, WALLS = K.WALLS, cmFrame = K.cmFrame, cmM = K.cmM, cmRainIn = K.cmRainIn, cmSky = K.cmSky, cmTop = K.cmTop, f1 = K.f1, hex = K.hex, lin = K.lin, mix = K.mix, rad = K.rad, rng = K.rng;

  /* ---- More parts for the composed counter room: places that are not a
     timber shop. Four back-wall shapes (a cavern, a ship's hold, the inside
     of a great tree, a brick tunnel), three wall materials (rock, hull
     planks, bark), three openings (a porthole on a rolling sea, a drain
     grate, a knothole), two lights (a crystal cluster, a brazier) and seven
     props. Each is one function in the same tables as the first parts. ---- */
  CM_OPT.shape.push('cavern', 'hold', 'hollow', 'tunnel');
  CM_OPT.wall.push('rock', 'hull', 'bark');
  CM_OPT.opening.push('porthole', 'grate', 'knothole');
  CM_OPT.light.push('crystals', 'brazier');
  CM_PROPS.push('fireflies', 'drips', 'cobwebs', 'nets', 'chains', 'mushrooms', 'tentacle');
  CM_WALLFQ.rock = [.035, 5, 8]; CM_WALLFQ.hull = ['.004 .06', 2.4, 6]; CM_WALLFQ.bark = ['.09 .006', 3.4, 6];
  CM_LIGHT.crystals = { base: '#9fe8ff', glow: '#7fd8ff', amb: '#2a3458', reach: 1000, k: .9 };
  CM_LIGHT.brazier = { base: '#ffa860', glow: '#ff8a3a', amb: '#33283a', reach: 980, k: 1 };
  PROP_AT.fireflies = { at: 'over' }; PROP_AT.drips = { at: 'over' }; PROP_AT.cobwebs = { at: 'ceil' }; PROP_AT.nets = { at: 'wallR' };
  PROP_AT.chains = { at: 'ceil' }; PROP_AT.mushrooms = { at: 'cL', w: 120 }; PROP_AT.tentacle = { at: 'tallR' };
  CM_OPEN_INFO.porthole = 'The porthole'; CM_OPEN_INFO.grate = 'The drain'; CM_OPEN_INFO.knothole = 'The knothole';

  CM_CSS.push(
    '@keyframes mp-rock{0%,100%{transform:rotate(-3deg) translateY(3px)}50%{transform:rotate(3deg) translateY(-3px)}}',
    '.mp-rock{transform-box:view-box;animation:mp-rock 7s ease-in-out infinite}',
    '@keyframes mp-wave{from{transform:translateX(0)}to{transform:translateX(-80px)}}',
    '.mp-wave{animation:mp-wave 5s linear infinite}.mp-wave.far{animation-duration:9s}',
    '@keyframes mp-swing{0%,100%{transform:rotate(-4deg)}50%{transform:rotate(4deg)}}',
    '.mp-swing{transform-box:view-box;animation:mp-swing 7s ease-in-out infinite}',
    '@keyframes mp-fly{0%{transform:translate(0,0);opacity:.15}20%{opacity:1}50%{transform:translate(var(--fx,30px),var(--fy,-24px));opacity:.5}70%{opacity:1}100%{transform:translate(0,0);opacity:.15}}',
    '.mp-fly{animation:mp-fly 9s ease-in-out infinite}',
    '@keyframes mp-drip{0%{transform:translateY(0);opacity:0}6%{opacity:.9}70%{opacity:.9}78%,100%{transform:translateY(var(--fall,380px));opacity:0}}',
    '.mp-drip{animation:mp-drip 3.6s cubic-bezier(.55,0,1,.7) infinite;opacity:0}',
    '@keyframes mp-ring{0%,72%{transform:scale(.2);opacity:0}78%{opacity:.7}100%{transform:scale(1.4);opacity:0}}',
    '.mp-ring{transform-box:fill-box;transform-origin:50% 50%;animation:mp-ring 3.6s linear infinite;opacity:0}',
    '@keyframes mp-trickle{to{stroke-dashoffset:-36}}',
    '.mp-trickle{animation:mp-trickle 1.2s linear infinite}',
    '@keyframes mp-curl{0%,100%{transform:rotate(-5deg)}50%{transform:rotate(6deg)}}',
    '.mp-curl{transform-box:view-box;animation:mp-curl 6s ease-in-out infinite}',
    '.mp-curl.tip{animation-duration:4.3s}',
    '@keyframes mp-flame{0%,100%{transform:scale(1,1)}50%{transform:scale(.92,1.12)}}',
    '.mp-flame{transform-box:fill-box;transform-origin:50% 100%;animation:mp-flame 1.1s ease-in-out infinite}'
  );

  // A rough lump of rock: a polygon whose corners wander round a centre.
  function mpLump(r, x, y, rad, n) {
    var d = '', i, a, k;
    for (i = 0; i < n; i++) { a = i / n * Math.PI * 2 + r() * .5; k = rad * (.65 + r() * .5); d += (i ? ' L' : 'M') + f1(x + Math.cos(a) * k) + ',' + f1(y + Math.sin(a) * k * .8); }
    return d + ' Z';
  }

  WALLS.rock = function (t) {
    var r = rng(23), s = '<rect x="-300" y="-150" width="1800" height="620" fill="#5c5650"/>', i, x, y, col;
    for (i = 0; i < 170; i++) { x = -300 + r() * 1800; y = -150 + r() * 620; col = mix('#5c5650', r() < .55 ? '#1c1a18' : '#b0a698', r() * .26); s += '<path d="' + mpLump(r, x, y, 26 + r() * 70, 9) + '" fill="' + col + '"/>'; }
    // cracks: short broken seams that wander down the face
    for (i = 0; i < 22; i++) { x = -260 + r() * 1700; y = -120 + r() * 520; var d = 'M' + f1(x) + ',' + f1(y), j; for (j = 0; j < 4; j++) d += ' l' + f1(r() * 44 - 14) + ',' + f1(14 + r() * 34); s += '<path d="' + d + '" stroke="#100d0b" stroke-opacity=".55" stroke-width="' + f1(1 + r() * 1.8) + '" fill="none" stroke-linejoin="round"/>'; }
    s = '<g filter="url(#cm-wall)">' + s + '</g><rect x="-300" y="-150" width="1800" height="620" fill="#1a1512" filter="url(#cm-stain)" opacity=".5"/>';
    // a vein of ore that catches the light in the theme's accent
    for (i = 0; i < 26; i++) { x = 770 + i * 15 + r() * 10; y = 150 + Math.sin(i * .42) * 60 + i * 5 + r() * 16; s += '<path d="M' + f1(x) + ',' + f1(y) + ' l' + f1(2 + r() * 3) + ',' + f1(-3 - r() * 4) + ' l' + f1(2 + r() * 3) + ',' + f1(3 + r() * 4) + ' Z" fill="' + mix(t.th.accent, '#ffffff', .35) + '" opacity="' + f1(.35 + r() * .5) + '"/>'; }
    return s;
  };
  WALLS.hull = function (t) {
    var r = rng(31), s = '', y, x, row = 0, base = mix(t.th.wood, '#1a120c', .25), w;
    for (y = -150; y < 470; y += 44, row++) {
      s += '<rect x="-300" y="' + y + '" width="1800" height="43" fill="' + mix(base, r() < .5 ? '#000' : '#ffffff', r() * .2) + '"/>';
      for (x = -300 + (row % 3) * 170 + r() * 60; x < 1500; x += w) { w = 300 + r() * 220; s += '<rect x="' + f1(x) + '" y="' + y + '" width="2" height="43" fill="#080504" opacity=".8"/><circle cx="' + f1(x - 9) + '" cy="' + (y + 10) + '" r="2.4" fill="#0c0806"/><circle cx="' + f1(x - 9) + '" cy="' + (y + 32) + '" r="2.4" fill="#0c0806"/><circle cx="' + f1(x + 11) + '" cy="' + (y + 10) + '" r="2.4" fill="#0c0806"/><circle cx="' + f1(x + 11) + '" cy="' + (y + 32) + '" r="2.4" fill="#0c0806"/>'; }
      s += '<rect x="-300" y="' + (y + 43) + '" width="1800" height="2.2" fill="#060403"/><rect x="-300" y="' + y + '" width="1800" height="1.4" fill="#fff" opacity=".07"/>';
    }
    s = '<g filter="url(#cm-wall)">' + s + '</g>';
    // tar runs down from the seams, and the planks are damp near the waterline
    for (x = 0; x < 16; x++) { var tx = -260 + r() * 1700, ty = -150 + Math.floor(r() * 12) * 44 + 44; s += '<path d="M' + f1(tx) + ',' + ty + ' q' + f1(r() * 6 - 3) + ',' + f1(20 + r() * 50) + ' 0,' + f1(40 + r() * 80) + '" stroke="#060403" stroke-opacity=".55" stroke-width="' + f1(2 + r() * 3) + '" stroke-linecap="round" fill="none"/>'; }
    return s + '<defs>' + lin('mp-damp', 1, [[0, '#0a1418', 0], [1, '#0a1418', .55]]) + '</defs><rect x="-300" y="300" width="1800" height="170" fill="url(#mp-damp)"/>';
  };
  WALLS.bark = function (t) {
    var r = rng(47), base = mix('#5a4028', t.th.wood, .35), s = '<rect x="-300" y="-150" width="1800" height="620" fill="' + base + '"/>', x, i, y, d;
    for (x = -300; x < 1500; x += 20 + r() * 26) {
      d = 'M' + f1(x) + ',-150'; for (y = -150; y < 470; y += 60) d += ' q' + f1(r() * 26 - 13) + ',30 ' + f1(r() * 10 - 5) + ',60';
      s += '<path d="' + d + '" stroke="' + mix(base, r() < .6 ? '#0c0704' : '#c8a878', .35 + r() * .4) + '" stroke-width="' + f1(4 + r() * 12) + '" fill="none" stroke-linecap="round"/>';
    }
    for (i = 0; i < 7; i++) { x = -200 + r() * 1600; y = -60 + r() * 460; s += '<ellipse cx="' + f1(x) + '" cy="' + f1(y) + '" rx="' + f1(16 + r() * 16) + '" ry="' + f1(26 + r() * 22) + '" fill="#120a06"/><ellipse cx="' + f1(x) + '" cy="' + f1(y) + '" rx="' + f1(8 + r() * 6) + '" ry="' + f1(14 + r() * 8) + '" fill="none" stroke="' + mix(base, '#d8b888', .4) + '" stroke-width="2.4"/>'; }
    s = '<g filter="url(#cm-wall)">' + s + '</g>';
    for (i = 0; i < 12; i++) s += '<ellipse cx="' + f1(-260 + r() * 1700) + '" cy="' + f1(-100 + r() * 540) + '" rx="' + f1(40 + r() * 80) + '" ry="' + f1(16 + r() * 28) + '" fill="#4a7a2a" opacity="' + f1(.14 + r() * .16) + '" filter="url(#src-blur8)"/>';
    return s;
  };

  // The rim of rock, timber or root that frames a shaped room, lit along its inner edge.
  function mpEdge(d, w, col) { return '<path d="' + d + '" fill="none" stroke="' + col + '" stroke-opacity=".14" stroke-width="' + w + '" stroke-linejoin="round"/>'; }

  SHAPES.cavern = function (t) {
    var s = WALLS[t.th.wall](t), r = rng(5), d = 'M-300,-150 H1500 V40', x, y, rock = '#191614', d2;
    // the roof: a ragged line with teeth of stone hanging from it
    for (x = 1500; x >= -300; x -= 46 + r() * 50) { y = 26 + r() * 46; if (r() < .42 && (x < 470 || x > 560) ) d += ' L' + f1(x + 16) + ',' + f1(y) + ' L' + f1(x + 4 + r() * 6) + ',' + f1(y + 50 + r() * 110) + ' L' + f1(x - 12) + ',' + f1(y); else d += ' L' + f1(x) + ',' + f1(y); }
    d += ' L-300,40 Z';
    s += '<path d="' + d + '" fill="#000" opacity=".45" filter="url(#src-blur8)" transform="translate(0,10)"/><path d="' + d + '" fill="' + rock + '"/>' + mpEdge(d, 2.4, t.lc);
    // pillars of rock at both sides, fatter at the foot
    [[-1, -60], [1, 1260]].forEach(function (sd) {
      var k = sd[0], x0 = sd[1]; d2 = 'M' + x0 + ',-150';
      for (y = -150; y <= 480; y += 52) d2 += ' L' + f1(x0 - k * (70 + r() * 34 + (y > 300 ? (y - 300) * .22 : 0) + (y < 60 ? (60 - y) * .3 : 0))) + ',' + y;
      d2 += ' L' + x0 + ',480 Z';
      s += '<path d="' + d2 + '" fill="#000" opacity=".4" filter="url(#src-blur8)" transform="translate(' + (-k * 10) + ',0)"/><path d="' + d2 + '" fill="' + rock + '"/>' + mpEdge(d2, 2, t.lc);
    });
    // stalagmites standing behind the counter
    [[536, 78], [556, 40], [752, 54], [1128, 96], [1090, 50]].forEach(function (q) { s += '<path d="M' + (q[0] - 16) + ',470 L' + (q[0] - 3) + ',' + (470 - q[1]) + ' L' + (q[0] + 4) + ',' + (476 - q[1]) + ' L' + (q[0] + 18) + ',470 Z" fill="#2a2622"/><path d="M' + (q[0] - 16) + ',470 L' + (q[0] - 3) + ',' + (470 - q[1]) + '" stroke="' + t.lc + '" stroke-opacity=".4" stroke-width="1.6"/>'; });
    return s;
  };
  SHAPES.hold = function (t) {
    var s = WALLS[t.th.wall](t), i, x;
    // the deck above: heavy beams running away overhead, their ends toward you
    s += '<rect x="-300" y="-150" width="1800" height="178" fill="#000" filter="url(#cm-beam)"/><rect x="-300" y="28" width="1800" height="12" fill="#000" opacity=".5" filter="url(#src-blur3)"/>';
    for (i = 0; i < 9; i++) { x = -240 + i * 210; s += '<rect x="' + x + '" y="20" width="64" height="40" rx="3" fill="#000" filter="url(#cm-post)"/><rect x="' + x + '" y="20" width="64" height="3" fill="' + t.lc + '" opacity=".3"/><rect x="' + x + '" y="60" width="64" height="10" fill="#000" opacity=".45" filter="url(#src-blur3)"/>'; }
    // ribs: the hull's frames curve in from the sides and one stands between the goods and the keeper
    function rib(x0, lean, w) {
      var d = 'M' + (x0 + lean) + ',30 Q' + (x0 + lean * .2) + ',260 ' + x0 + ',480 H' + (x0 + w) + ' Q' + (x0 + w + lean * .2) + ',260 ' + (x0 + w + lean) + ',30 Z';
      return '<path d="' + d + '" fill="#000" opacity=".4" filter="url(#src-blur5)" transform="translate(10,0)"/><path d="' + d + '" fill="#000" filter="url(#cm-post)"/><path d="M' + (x0 + lean) + ',30 Q' + (x0 + lean * .2) + ',260 ' + x0 + ',480" stroke="' + t.lc + '" stroke-opacity=".28" stroke-width="2" fill="none"/>';
    }
    s += rib(-34, 60, 44) + rib(504, 14, 30) + rib(1190, -60, 44);
    // iron knees under the beams, and a ring bolt
    [[540, 60], [-6, 60], [1166, 60]].forEach(function (q) { s += '<path d="M' + q[0] + ',' + q[1] + ' h46 v10 q-34,4 -36,40 h-10 Z" fill="url(#src-iron)"/>'; });
    return s + '<circle cx="1120" cy="88" r="13" fill="none" stroke="url(#src-iron)" stroke-width="5"/><rect x="1114" y="60" width="12" height="18" fill="url(#src-iron)"/>';
  };
  SHAPES.hollow = function (t) {
    var s = WALLS[t.th.wall](t), r = rng(9), bark = mix('#2a1c10', t.th.wood, .2), i;
    s += '<defs>' + lin('mp-cyl', 0, [[0, '#000', .78], [.12, '#000', .4], [.32, '#000', .06], [.5, '#000', 0], [.68, '#000', .06], [.88, '#000', .4], [1, '#000', .78]]) + '</defs><rect x="-300" y="-150" width="1800" height="620" fill="url(#mp-cyl)"/>';
    // the trunk closes overhead in a dark arch
    var top = 'M-300,-150 H1500 V90 Q1200,26 900,36 Q600,-10 300,40 Q60,60 -300,120 Z';
    s += '<path d="' + top + '" fill="#000" opacity=".45" filter="url(#src-blur8)" transform="translate(0,12)"/><path d="' + top + '" fill="' + bark + '"/>' + mpEdge(top, 2.4, t.lc);
    // roots and limbs: thick, twisting, lighter along the side the light reaches
    function root(d, w) { return '<path d="' + d + '" stroke="#000" stroke-opacity=".4" stroke-width="' + (w + 10) + '" fill="none" stroke-linecap="round" filter="url(#src-blur5)" transform="translate(8,4)"/><path d="' + d + '" stroke="' + bark + '" stroke-width="' + w + '" fill="none" stroke-linecap="round"/><path d="' + d + '" stroke="' + mix(bark, '#d8b888', .5) + '" stroke-opacity=".5" stroke-width="' + f1(w * .16) + '" fill="none" stroke-linecap="round" transform="translate(' + f1(w * .28) + ',0)"/>'; }
    s += root('M-40,-40 C10,120 -30,300 20,480', 74) + root('M1240,-40 C1180,140 1230,300 1176,480', 74) + root('M470,20 C520,160 490,330 524,480', 30) + root('M-60,60 C140,40 300,56 470,24', 34) + root('M1260,70 C1100,36 900,60 700,30', 30);
    // moss along the roots, and a few leaves that found their way in
    for (i = 0; i < 40; i++) { var mx = r() < .5 ? 10 + r() * 30 : 1160 + r() * 40, my = 40 + r() * 420; s += '<circle cx="' + f1(mx) + '" cy="' + f1(my) + '" r="' + f1(3 + r() * 6) + '" fill="' + mix('#3f6a22', '#9ac850', r()) + '" opacity="' + f1(.4 + r() * .4) + '"/>'; }
    for (i = 0; i < 18; i++) { var lx = 60 + r() * 1080, ly = 30 + r() * 40, a = r() * 180; s += '<path d="M0,0 q7,-9 16,0 q-9,9 -16,0 Z" transform="translate(' + f1(lx) + ',' + f1(ly) + ') rotate(' + f1(a) + ')" fill="' + mix('#3f6a22', '#b8d860', r()) + '" opacity=".8"/>'; }
    return s;
  };
  SHAPES.tunnel = function (t) {
    var s = WALLS[t.th.wall](t), arch = 'M-70,480 V250 A670,350 0 0 1 1270,250 V480', i, a, x, y;
    s += '<path d="M-300,-150 H1500 V480 H-300 Z M-70,480 V250 A670,350 0 0 1 1270,250 V480 Z" fill-rule="evenodd" fill="#0b0908"/>';
    s += '<path d="' + arch + '" fill="none" stroke="#000" stroke-opacity=".55" stroke-width="60" filter="url(#src-blur8)" transform="translate(0,14)"/>';
    // the arch ring: one course of bricks set on edge
    s += '<path d="' + arch + '" fill="none" stroke="#3a2018" stroke-width="40"/><path d="' + arch + '" fill="none" stroke="#8a5038" stroke-width="34" stroke-dasharray="26 5"/><path d="' + arch + '" fill="none" stroke="' + t.lc + '" stroke-opacity=".22" stroke-width="3" transform="translate(0,19)"/>';
    // a stone kerb at the foot and an iron pipe along the wall
    s += '<rect x="-300" y="420" width="1800" height="50" fill="#000" opacity=".35"/><path d="M520,96 H1150" stroke="#1c1a18" stroke-width="16"/><path d="M520,91 H1150" stroke="' + t.lc + '" stroke-opacity=".25" stroke-width="2.4"/>';
    for (i = 0; i < 5; i++) { x = 560 + i * 140; s += '<rect x="' + (x - 6) + '" y="84" width="12" height="24" rx="2" fill="#2a2622"/>'; }
    return s;
  };

  OPENINGS.porthole = function (t) {
    var cx = 912, cy = 196, R0 = 92, night = t.time === 'night' || t.time === 'rain', sea = night ? '#0c2238' : t.time === 'dusk' ? '#2a3a5a' : '#2a6a8a', foam = night ? '#9ab8d8' : '#e8f4ff', i;
    var wave = function (y, amp, cls, op) { var d = 'M' + (cx - 260) + ',' + y, x; for (x = cx - 260; x < cx + 260; x += 40) d += ' q10,' + (-amp) + ' 20,0 t20,0'; return '<g class="mp-wave ' + cls + '"><path d="' + d + ' V' + (cy + 140) + ' H' + (cx - 260) + ' Z" fill="' + sea + '" opacity="' + op + '"/><path d="' + d + '" stroke="' + foam + '" stroke-opacity=".5" stroke-width="1.6" fill="none"/></g>'; };
    var s = '<g data-k="window"><circle cx="' + cx + '" cy="' + (cy + 6) + '" r="' + (R0 + 30) + '" fill="#000" opacity=".45" filter="url(#src-blur8)"/><circle cx="' + cx + '" cy="' + cy + '" r="' + (R0 + 22) + '" fill="url(#cm-trim)"/><circle cx="' + cx + '" cy="' + cy + '" r="' + (R0 + 8) + '" fill="#0a0806"/>';
    // the sea tips one way and the other: it is the ship that is rolling
    s += '<g clip-path="url(#cm-oclip)">' + cmSky(t, 'cm-osky', cx - R0, cy - R0, 2 * R0, 2 * R0) + '<g class="mp-rock" style="transform-origin:' + cx + 'px ' + cy + 'px">' + wave(cy + 6, 7, 'far', .75) + wave(cy + 24, 10, '', 1) + '</g>' + cmRainIn(t, cx - R0, cy - R0, 2 * R0, 2 * R0, 5) +
      '<path d="M' + (cx - R0) + ',' + (cy - 30) + ' A' + R0 + ',' + R0 + ' 0 0 1 ' + (cx - 10) + ',' + (cy - R0) + ' L' + (cx - 44) + ',' + (cy + 10) + ' Z" fill="#fff" opacity=".07"/></g>';
    s += '<circle cx="' + cx + '" cy="' + cy + '" r="' + R0 + '" fill="none" stroke="url(#cm-trim)" stroke-width="7"/>';
    for (i = 0; i < 8; i++) { var a = i * Math.PI / 4 + .39; s += '<circle cx="' + f1(cx + Math.cos(a) * (R0 + 15)) + '" cy="' + f1(cy + Math.sin(a) * (R0 + 15)) + '" r="4.6" fill="url(#cm-trimv)"/><circle cx="' + f1(cx + Math.cos(a) * (R0 + 15) - 1.2) + '" cy="' + f1(cy + Math.sin(a) * (R0 + 15) - 1.2) + '" r="1.4" fill="#fff" opacity=".6"/>'; }
    // the hinge and the dog that screws it shut
    s += '<rect x="' + (cx - R0 - 34) + '" y="' + (cy - 12) + '" width="20" height="24" rx="3" fill="url(#cm-trimv)"/><rect x="' + (cx + R0 + 14) + '" y="' + (cy - 7) + '" width="26" height="14" rx="4" fill="url(#cm-trimv)"/>';
    return s + '</g><path d="M' + (cx - R0) + ',' + (cy + 70) + ' L760,452 L900,452 L' + (cx + R0) + ',' + (cy + 70) + ' Z" fill="url(#cm-shaft)" opacity=".5" pointer-events="none"/><clipPath id="cm-oclip"><circle cx="' + cx + '" cy="' + cy + '" r="' + R0 + '"/></clipPath>';
  };
  OPENINGS.grate = function (t) {
    var d = 'M812,452 V292 A100,100 0 0 1 1012,292 V452 Z', a = t.th.accent, s = '<g data-k="window"><path d="' + d + '" fill="#000" opacity=".5" filter="url(#src-blur8)" transform="translate(4,4)"/><path d="' + d + '" fill="#050606"/>', i, x;
    s += '<defs><radialGradient id="mp-ooze" gradientUnits="userSpaceOnUse" cx="912" cy="440" r="190"><stop offset="0" stop-color="' + mix(a, '#ffffff', .25) + '" stop-opacity=".85"/><stop offset=".5" stop-color="' + a + '" stop-opacity=".3"/><stop offset="1" stop-color="' + a + '" stop-opacity="0"/></radialGradient></defs>';
    // something glows a long way down the drain, and water runs out over the sill
    s += '<g clip-path="url(#cm-oclip)"><rect x="800" y="180" width="224" height="280" fill="url(#mp-ooze)" class="cm-pulse"/><path d="M830,452 Q870,400 860,330 M990,452 Q960,410 968,350" stroke="' + a + '" stroke-opacity=".25" stroke-width="10" fill="none" filter="url(#src-blur5)"/>';
    s += '<path d="M880,300 V452 M930,330 V452 M956,310 V452" stroke="#cfe8e0" stroke-opacity=".55" stroke-width="2.2" stroke-dasharray="14 22" class="mp-trickle" fill="none"/></g>';
    for (i = 0; i < 5; i++) { x = 838 + i * 37; s += '<rect x="' + (x - 5) + '" y="196" width="10" height="258" fill="url(#src-iron)"/><rect x="' + (x - 5) + '" y="196" width="2.4" height="258" fill="' + t.lc + '" opacity=".3"/>'; }
    s += '<rect x="812" y="292" width="200" height="10" fill="url(#src-iron)"/><rect x="812" y="380" width="200" height="10" fill="url(#src-iron)"/>';
    s += '<path d="M800,456 V292 A112,112 0 0 1 1024,292 V456" fill="none" stroke="#3a2018" stroke-width="22"/><path d="M800,456 V292 A112,112 0 0 1 1024,292 V456" fill="none" stroke="#7a4632" stroke-width="17" stroke-dasharray="22 4"/>';
    // slime where the water has run for years
    s += '<path d="M826,452 q-6,-40 4,-70 M998,452 q8,-30 0,-56" stroke="#2f5a2a" stroke-opacity=".7" stroke-width="7" stroke-linecap="round" fill="none"/>';
    return s + '</g><ellipse cx="912" cy="452" rx="150" ry="40" fill="url(#cm-acc)" opacity=".35" pointer-events="none"/><clipPath id="cm-oclip"><path d="' + d + '"/></clipPath>';
  };
  OPENINGS.knothole = function (t) {
    var d = 'M836,206 C820,140 870,84 930,92 C990,96 1022,150 1004,214 C992,266 952,300 902,292 C864,288 844,254 836,206 Z', bark = mix('#2a1c10', t.th.wood, .2), r = rng(15), i;
    var s = '<g data-k="window"><path d="' + d + '" fill="#000" opacity=".5" filter="url(#src-blur8)" transform="translate(3,7)"/><path d="' + d + '" stroke="' + bark + '" stroke-width="44" fill="' + bark + '" stroke-linejoin="round"/>';
    s += '<g clip-path="url(#cm-oclip)">' + cmSky(t, 'cm-osky', 810, 76, 220, 230) + '<rect x="810" y="230" width="220" height="80" fill="url(#src-fog)"/>';
    // boughs across the opening, their leaves stirring
    s += '<path d="M800,250 C860,230 900,180 1040,150" stroke="#1a120a" stroke-width="9" fill="none"/><path d="M880,226 C900,200 890,160 910,120" stroke="#1a120a" stroke-width="5" fill="none"/>';
    for (i = 0; i < 26; i++) { var lx = 820 + r() * 200, ly = 100 + r() * 150; s += '<g class="cm-sway" style="transform-origin:' + f1(lx) + 'px ' + f1(ly) + 'px;animation-delay:' + f1(-r() * 5) + 's"><path d="M0,0 q9,-11 20,0 q-11,11 -20,0 Z" transform="translate(' + f1(lx) + ',' + f1(ly) + ') rotate(' + f1(r() * 360) + ')" fill="' + mix('#24481a', '#8ac048', r()) + '"/></g>'; }
    s += cmRainIn(t, 810, 76, 220, 230, 4) + '</g>';
    // rings of grown-over bark round the hole
    s += '<path d="' + d + '" fill="none" stroke="' + mix(bark, '#d8b888', .4) + '" stroke-opacity=".6" stroke-width="3"/><path d="' + d + '" fill="none" stroke="#0a0604" stroke-opacity=".6" stroke-width="5" transform="translate(912,192) scale(1.16) translate(-912,-192)"/>';
    return s + '</g><path d="M836,270 L760,452 L900,452 L1004,270 Z" fill="url(#cm-shaft)" opacity=".55" pointer-events="none"/><clipPath id="cm-oclip"><path d="' + d + '"/></clipPath>';
  };

  LIGHTS.crystals = function (t) {
    var x = t.lp.x, y = 480, a = mix(t.th.accent, '#9fe8ff', .5), s = '<defs>' + lin('mp-xl', 0, [[0, '#ffffff'], [.4, mix(a, '#ffffff', .5)], [1, mix(a, '#102040', .55)]]) + '</defs><g data-k="lamp"><ellipse cx="' + x + '" cy="' + (y + 2) + '" rx="58" ry="9" fill="url(#src-ao)"/>';
    // shards of different heights leaning out from one root
    [[-34, 44, -24, 13], [-14, 86, -9, 17], [8, 112, 4, 19], [30, 70, 17, 15], [46, 40, 28, 11]].forEach(function (q) {
      s += '<g transform="translate(' + (x + q[0]) + ',' + y + ') rotate(' + q[2] + ')"><path d="M' + (-q[3]) + ',0 L' + (-q[3]) + ',' + (-q[1] + 16) + ' L0,' + (-q[1]) + ' L' + q[3] + ',' + (-q[1] + 16) + ' L' + q[3] + ',0 Z" fill="url(#mp-xl)" opacity=".92"/>' +
        '<path d="M0,' + (-q[1]) + ' V0 M' + (-q[3]) + ',' + (-q[1] + 16) + ' L0,' + (-q[1] + 26) + ' L' + q[3] + ',' + (-q[1] + 16) + '" stroke="#ffffff" stroke-opacity=".6" stroke-width="1.2" fill="none"/></g>';
    });
    s += '<path d="M' + (x - 50) + ',' + y + ' q50,-16 100,0 Z" fill="#2a2622"/><g class="cm-pulse"><circle cx="' + x + '" cy="' + (y - 60) + '" r="120" fill="url(#cm-bloom)"/></g></g>';
    return { back: '', front: s };
  };
  LIGHTS.brazier = function (t) {
    var x = t.lp.x, y = 480, i, r = rng(3), s = '<g data-k="lamp"><ellipse cx="' + x + '" cy="' + (y + 2) + '" rx="56" ry="9" fill="url(#src-ao)"/>';
    s += '<path d="M' + (x - 30) + ',' + y + ' L' + (x - 12) + ',' + (y - 44) + ' M' + (x + 30) + ',' + y + ' L' + (x + 12) + ',' + (y - 44) + ' M' + x + ',' + (y - 2) + ' V' + (y - 44) + '" stroke="url(#src-iron)" stroke-width="6" stroke-linecap="round"/>';
    s += '<path d="M' + (x - 46) + ',' + (y - 62) + ' Q' + x + ',' + (y - 10) + ' ' + (x + 46) + ',' + (y - 62) + ' Z" fill="#1c1a18"/><path d="M' + (x - 46) + ',' + (y - 62) + ' Q' + x + ',' + (y - 10) + ' ' + (x + 46) + ',' + (y - 62) + '" stroke="' + t.lc + '" stroke-opacity=".35" stroke-width="2" fill="none"/>';
    s += '<ellipse cx="' + x + '" cy="' + (y - 62) + '" rx="46" ry="9" fill="#2a1408"/>';
    for (i = 0; i < 9; i++) s += '<ellipse cx="' + f1(x - 34 + r() * 68) + '" cy="' + f1(y - 64 + r() * 5) + '" rx="' + f1(6 + r() * 6) + '" ry="' + f1(3 + r() * 2) + '" fill="' + (r() < .5 ? '#ff9a30' : '#ffd070') + '" opacity="' + f1(.7 + r() * .3) + '"/>';
    [[-18, 34, .85], [2, 52, 1], [20, 30, .8]].forEach(function (q, j) { s += '<path class="mp-flame src-flame" style="animation-delay:' + (-j * .37) + 's" d="M' + (x + q[0]) + ',' + (y - 64 - q[1]) + ' c' + (-10 * q[2]) + ',' + (q[1] * .45) + ' ' + (-12 * q[2]) + ',' + (q[1] * .8) + ' 0,' + q[1] + ' c' + (12 * q[2]) + ',' + (-q[1] * .2) + ' ' + (10 * q[2]) + ',' + (-q[1] * .55) + ' 0,' + (-q[1]) + ' Z" fill="url(#src-flame)"/>'; });
    for (i = 0; i < 6; i++) s += '<circle class="cm-spark src-nohit" cx="' + f1(x - 20 + r() * 40) + '" cy="' + (y - 80) + '" r="' + f1(1.2 + r()) + '" fill="#ffd27a" style="--dx:' + f1((r() - .5) * 50) + 'px;animation-delay:' + f1(-r() * 3.4) + 's"/>';
    s += '<circle cx="' + x + '" cy="' + (y - 90) + '" r="110" fill="url(#cm-bloom)" data-flick=".8"/></g>';
    return { back: '', front: s };
  };

  PROPS.fireflies = function (t, p) {
    var s = '', r = rng(71), i, c = mix(t.th.accent, '#fff6a0', .5);
    for (i = 0; i < 16; i++) { var x = 80 + r() * 1040, y = 70 + r() * 360; s += '<g class="mp-fly" style="--fx:' + f1((r() - .5) * 90) + 'px;--fy:' + f1((r() - .5) * 70) + 'px;animation-duration:' + f1(7 + r() * 7) + 's;animation-delay:-' + f1(r() * 9) + 's"><circle cx="' + f1(x) + '" cy="' + f1(y) + '" r="7" fill="' + c + '" opacity=".25"/><circle cx="' + f1(x) + '" cy="' + f1(y) + '" r="2" fill="#fffbe0"/></g>'; }
    return '<g class="src-nohit">' + s + '</g>';
  };
  PROPS.drips = function (t, p) {
    var s = '';
    [[560, 60, 400], [770, 70, 384], [1066, 54, 440], [1140, 80, 414], [300, 50, 30]].forEach(function (q, j) {
      if (q[2] < 100) return;
      s += '<ellipse class="mp-drip" cx="' + q[0] + '" cy="' + q[1] + '" rx="1.8" ry="4.4" fill="#d8ecff" style="--fall:' + q[2] + 'px;animation-delay:' + (-j * .9) + 's"/>' +
        '<ellipse class="mp-ring" cx="' + q[0] + '" cy="' + (q[1] + q[2]) + '" rx="14" ry="3.4" fill="none" stroke="#d8ecff" stroke-width="1.2" style="animation-delay:' + (-j * .9) + 's"/>';
    });
    return '<g class="src-nohit">' + s + '</g>';
  };
  PROPS.cobwebs = function (t, p) {
    function web(cx, cy, k, n) {
      var s = '', i, j, a, a2, R1;
      for (i = 0; i <= n; i++) { a = i / n * Math.PI / 2; s += '<path d="M' + cx + ',' + cy + ' L' + f1(cx + k * Math.cos(a) * 150) + ',' + f1(cy + Math.sin(a) * 130) + '"/>'; }
      for (j = 1; j <= 5; j++) { R1 = j * 26; var d = ''; for (i = 0; i <= n; i++) { a = i / n * Math.PI / 2; a2 = (i - .5) / n * Math.PI / 2; d += i ? ' Q' + f1(cx + k * Math.cos(a2) * R1 * .86) + ',' + f1(cy + Math.sin(a2) * R1 * .76) + ' ' + f1(cx + k * Math.cos(a) * R1) + ',' + f1(cy + Math.sin(a) * R1 * .88) : 'M' + f1(cx + k * R1) + ',' + cy; } s += '<path d="' + d + '"/>'; }
      return '<g class="cm-sway" style="transform-origin:' + cx + 'px ' + cy + 'px;animation-duration:9s" stroke="#e8e4dc" stroke-opacity=".34" stroke-width="1" fill="none">' + s + '</g>';
    }
    return '<g class="src-nohit">' + web(534, 36, 1, 6) + web(1170, 36, -1, 6) + '</g>';
  };
  PROPS.nets = function (t, p) {
    var x = p.x, s = '<g class="src-nohit"><g class="mp-swing" style="transform-origin:' + x + 'px 60px">', i, j, w = 96, h = 230;
    s += '<g stroke="#c8b48a" stroke-opacity=".7" stroke-width="1.3" fill="none">';
    for (i = 0; i <= 6; i++) s += '<path d="M' + f1(x - w / 2 + i * w / 6) + ',64 Q' + f1(x - w / 2 + i * w / 6 + (i - 3) * 5) + ',' + (64 + h / 2) + ' ' + f1(x + (i - 3) * 6) + ',' + (64 + h) + '"/>';
    for (j = 1; j < 9; j++) { var yy = 64 + j * h / 9, ww = w * (1 - j / 11); s += '<path d="M' + f1(x - ww / 2) + ',' + f1(yy) + ' Q' + x + ',' + f1(yy + 12) + ' ' + f1(x + ww / 2) + ',' + f1(yy) + '"/>'; }
    s += '</g><path d="M' + (x - w / 2 - 6) + ',62 H' + (x + w / 2 + 6) + '" stroke="#4a3622" stroke-width="5" stroke-linecap="round"/>';
    // glass floats caught in the net, each a small lens of the room's light
    [[-16, 150, 17, '#3f9a8a'], [18, 190, 14, '#3a6aa8'], [-4, 230, 19, '#5aa84a']].forEach(function (q) {
      s += '<circle cx="' + (x + q[0]) + '" cy="' + q[1] + '" r="' + q[2] + '" fill="' + q[3] + '" opacity=".75"/><circle cx="' + (x + q[0]) + '" cy="' + q[1] + '" r="' + q[2] + '" fill="none" stroke="#fff" stroke-opacity=".3" stroke-width="1.4"/>' +
        '<path d="M' + (x + q[0] - q[2] * .6) + ',' + (q[1] - q[2] * .3) + ' q' + (q[2] * .2) + ',' + (-q[2] * .5) + ' ' + (q[2] * .7) + ',' + (-q[2] * .5) + '" stroke="#fff" stroke-opacity=".75" stroke-width="2.4" stroke-linecap="round" fill="none"/><circle cx="' + (x + q[0] + q[2] * .3) + '" cy="' + (q[1] + q[2] * .4) + '" r="' + (q[2] * .3) + '" fill="' + t.lc + '" opacity=".35"/>';
    });
    return s + '</g></g>';
  };
  PROPS.chains = function (t, p) {
    var s = '<g class="src-nohit">';
    [[752, 96, 0], [786, 150, 1], [1084, 120, 2]].forEach(function (q) {
      var x = q[0], len = q[1], y;
      s += '<g class="mp-swing" style="transform-origin:' + x + 'px 34px;animation-delay:' + (-q[2] * 1.7) + 's;animation-duration:' + (6 + q[2]) + 's">';
      for (y = 34; y < 34 + len; y += 13) s += '<ellipse cx="' + x + '" cy="' + (y + 7) + '" rx="' + ((y / 13 | 0) % 2 ? 2.4 : 5) + '" ry="7.4" fill="none" stroke="url(#src-iron)" stroke-width="2.6"/>';
      y = 34 + len;
      s += '<path d="M' + x + ',' + y + ' v14 q0,18 -15,18 q-13,0 -13,-13" stroke="url(#src-iron)" stroke-width="4.4" fill="none" stroke-linecap="round"/><path d="M' + (x - 1.5) + ',' + y + ' v14" stroke="' + t.lc + '" stroke-opacity=".4" stroke-width="1.2"/></g>';
    });
    return s + '</g>';
  };
  PROPS.mushrooms = function (t, p) {
    var x = p.x, y = 492, a = t.th.accent, s = '<g class="src-nohit"><ellipse cx="' + x + '" cy="' + (y + 3) + '" rx="60" ry="8" fill="url(#src-ao)"/><circle cx="' + x + '" cy="' + (y - 34) + '" r="76" fill="url(#cm-acc)" class="cm-pulse"/>';
    s += '<path d="M' + (x - 52) + ',' + y + ' q52,-22 104,0 Z" fill="#2a1c12"/>';
    [[-30, 34, 22], [-4, 62, 30], [24, 44, 24], [44, 22, 15], [-48, 18, 13]].forEach(function (q) {
      var cx = x + q[0], top = y - q[1], w = q[2];
      s += '<path d="M' + (cx - 3) + ',' + (y - 4) + ' Q' + (cx - 5) + ',' + (top + 10) + ' ' + (cx - 2) + ',' + top + ' h5 Q' + (cx + 6) + ',' + (top + 10) + ' ' + (cx + 4) + ',' + (y - 4) + ' Z" fill="#e8e0cc"/>' +
        '<path d="M' + (cx - w) + ',' + (top + 4) + ' Q' + (cx - w * .8) + ',' + (top - w * .9) + ' ' + cx + ',' + (top - w * .9) + ' Q' + (cx + w * .8) + ',' + (top - w * .9) + ' ' + (cx + w) + ',' + (top + 4) + ' Q' + cx + ',' + (top + 10) + ' ' + (cx - w) + ',' + (top + 4) + ' Z" fill="' + mix(a, '#102018', .25) + '"/>' +
        '<path d="M' + (cx - w * .7) + ',' + (top - w * .3) + ' Q' + (cx - w * .4) + ',' + (top - w * .75) + ' ' + cx + ',' + (top - w * .78) + '" stroke="' + mix(a, '#ffffff', .65) + '" stroke-width="2.4" stroke-linecap="round" fill="none"/>' +
        '<circle cx="' + (cx + w * .3) + '" cy="' + (top - w * .4) + '" r="' + f1(w * .1) + '" fill="#fff" opacity=".6"/><circle cx="' + (cx - w * .2) + '" cy="' + (top - w * .15) + '" r="' + f1(w * .07) + '" fill="#fff" opacity=".5"/>' +
        '<path d="M' + (cx - w * .9) + ',' + (top + 5) + ' Q' + cx + ',' + (top + 11) + ' ' + (cx + w * .9) + ',' + (top + 5) + '" stroke="#fff" stroke-opacity=".3" stroke-width="1.4" fill="none"/>';
    });
    return s + '</g>';
  };
  // A limb that tapers: points along a spine, half as wide at each as `w` says, closed into one outline.
  function mpTaper(pts, w) {
    var L = [], R = [], i, n = pts.length, dx, dy, k, h;
    for (i = 0; i < n; i++) {
      dx = pts[Math.min(n - 1, i + 1)][0] - pts[Math.max(0, i - 1)][0]; dy = pts[Math.min(n - 1, i + 1)][1] - pts[Math.max(0, i - 1)][1]; k = Math.sqrt(dx * dx + dy * dy) || 1; h = w(i / (n - 1)) / 2;
      L.push(f1(pts[i][0] - dy / k * h) + ',' + f1(pts[i][1] + dx / k * h)); R.unshift(f1(pts[i][0] + dy / k * h) + ',' + f1(pts[i][1] - dx / k * h));
    }
    return 'M' + L.join(' L') + ' L' + R.join(' L') + ' Z';
  }
  PROPS.tentacle = function (t, p) {
    var x = p.x, y = 524, c = mix('#8a3a7a', t.th.accent, .18), cD = mix(c, '#07040a', .62), cL = mix(c, '#ffd8f0', .5), i, u, pts = [], a, rr, sx, sy;
    // the spine rises out from behind the counter, leans, and winds into a curl at the tip
    for (i = 0; i <= 26; i++) { u = i / 26; pts.push([x + Math.sin(u * 2.6) * 34 - u * 20, y - u * 250]); }
    sx = pts[26][0]; sy = pts[26][1];
    for (i = 1; i <= 26; i++) { a = -Math.PI / 2 - .5 + i * .2; rr = 40 * (1 - i / 30); pts.push([sx - 38 + Math.cos(a + 1.2) * rr + 12, sy + 6 + Math.sin(a + 1.2) * rr - 30 * (1 - i / 26)]); }
    function wd(u) { return 62 * Math.pow(1 - u, 1.25) + 5; }
    var d = mpTaper(pts, wd), s = '<g class="src-nohit"><defs>' + lin('mp-tn', 0, [[0, cL], [.35, c], [1, cD]]) + '</defs><g class="mp-curl" style="transform-origin:' + x + 'px ' + y + 'px">';
    s += '<path d="' + d + '" fill="#000" opacity=".4" filter="url(#src-blur5)" transform="translate(8,4)"/><path d="' + d + '" fill="url(#mp-tn)" stroke="' + cD + '" stroke-width="1.4" stroke-linejoin="round"/>';
    // suckers down the pale side, smaller as the limb thins
    for (i = 2; i < 44; i += 3) { u = i / 52; rr = wd(u) * .2; s += '<circle cx="' + f1(pts[i][0] + wd(u) * .2) + '" cy="' + f1(pts[i][1]) + '" r="' + f1(rr) + '" fill="' + cL + '" opacity=".8"/><circle cx="' + f1(pts[i][0] + wd(u) * .2) + '" cy="' + f1(pts[i][1]) + '" r="' + f1(rr * .45) + '" fill="' + cD + '"/>'; }
    // wet light along the lit edge
    s += '<path d="M' + pts.slice(1, 30).map(function (q, j) { return f1(q[0] - wd((j + 1) / 52) * .32) + ',' + f1(q[1]); }).join(' L') + '" stroke="#fff" stroke-opacity=".4" stroke-width="2.2" fill="none" stroke-linecap="round"/>';
    return s + '</g></g>';
  };

  /* ---- Stranger places for the composed counter room: a fey glade, a
     volcano's throat, open space, a white nowhere, something eldritch and a
     sanctum thick with magic. Walls are baked, so everything that moves lives
     in an opening, a light or a prop. ---- */
  CM_OPT.shape.push('glade', 'void', 'ribs');
  CM_OPT.wall.push('glade', 'basalt', 'nebula', 'white', 'obsidian', 'runestone');
  CM_OPT.opening.push('lava-fall', 'planet', 'lone-door', 'rift', 'portal');
  CM_OPT.light.push('wisps', 'star', 'halo', 'witchfire', 'orb');
  CM_PROPS.push('petals', 'butterflies', 'embers', 'smoke', 'comets', 'floating-stones', 'eyes', 'glyphs', 'magic-circle');
  CM_WALLFQ.glade = [.02, 1.2, 5]; CM_WALLFQ.basalt = ['.006 .05', 4, 8]; CM_WALLFQ.nebula = [.02, 1, 3]; CM_WALLFQ.white = [.015, .7, 4];
  CM_WALLFQ.obsidian = [.012, 2.2, 6]; CM_WALLFQ.runestone = [.05, 4.5, 8];
  CM_LIGHT.wisps = { base: '#c8ffe0', glow: '#8affc0', amb: '#2c3a5a', reach: 1080, k: .9 };
  CM_LIGHT.star = { base: '#fff0c8', glow: '#ffe08a', amb: '#2a2c5a', reach: 1100, k: 1 };
  CM_LIGHT.halo = { base: '#ffffff', glow: '#e8f0ff', amb: '#9aa2bc', reach: 1700, k: 1 };
  CM_LIGHT.witchfire = { base: '#b0ffb8', glow: '#5cff8a', amb: '#2a2440', reach: 940, k: .95 };
  CM_LIGHT.orb = { base: '#d0c0ff', glow: '#a88aff', amb: '#2e2a58', reach: 1040, k: .95 };
  ['petals', 'butterflies', 'embers', 'smoke', 'comets', 'glyphs'].forEach(function (p) { PROP_AT[p] = { at: 'over' }; });
  PROP_AT['floating-stones'] = { at: 'ceil' }; PROP_AT.eyes = { at: 'wallR' }; PROP_AT['magic-circle'] = { at: 'ceil' };
  CM_OPEN_INFO['lava-fall'] = 'The lava fall'; CM_OPEN_INFO.planet = 'The viewport'; CM_OPEN_INFO['lone-door'] = 'The door'; CM_OPEN_INFO.rift = 'The rift'; CM_OPEN_INFO.portal = 'The portal';

  CM_CSS.push(
    '@keyframes mp-spin{to{transform:rotate(360deg)}}',
    '.mp-spin{transform-box:view-box;animation:mp-spin 60s linear infinite}.mp-spin.rev{animation-direction:reverse}',
    '@keyframes mp-twinkle{0%,100%{opacity:.25}50%{opacity:1}}',
    '.mp-twinkle{animation:mp-twinkle 3.2s ease-in-out infinite}',
    '@keyframes mp-fall{0%{transform:translate(0,-40px) rotate(0);opacity:0}8%{opacity:.9}90%{opacity:.9}100%{transform:translate(var(--fx,-60px),520px) rotate(var(--fr,300deg));opacity:0}}',
    '.mp-fall{transform-box:fill-box;transform-origin:50% 50%;animation:mp-fall 11s linear infinite;opacity:0}',
    '@keyframes mp-rise{0%{transform:translate(0,0);opacity:0}12%{opacity:1}80%{opacity:.7}100%{transform:translate(var(--fx,20px),var(--up,-380px));opacity:0}}',
    '.mp-rise{animation:mp-rise 7s ease-out infinite;opacity:0}',
    '@keyframes mp-blink{0%,88%,100%{transform:scaleY(1)}92%,96%{transform:scaleY(.06)}}',
    '.mp-blink{transform-box:fill-box;transform-origin:50% 50%;animation:mp-blink 6s ease-in-out infinite}',
    '@keyframes mp-look{0%,40%,100%{transform:translateX(0)}50%,70%{transform:translateX(5px)}80%,90%{transform:translateX(-4px)}}',
    '.mp-look{animation:mp-look 9s ease-in-out infinite}',
    '@keyframes mp-flap{0%,100%{transform:scaleX(1)}50%{transform:scaleX(.25)}}',
    '.mp-flap{transform-box:fill-box;animation:mp-flap .5s ease-in-out infinite}.mp-flap.l{transform-origin:100% 50%}.mp-flap.r{transform-origin:0 50%}',
    '@keyframes mp-smoke{0%{transform:translate(0,0) scale(.8);opacity:0}20%{opacity:.5}100%{transform:translate(var(--fx,120px),-90px) scale(1.7);opacity:0}}',
    '.mp-smoke{transform-box:fill-box;transform-origin:50% 50%;animation:mp-smoke 16s ease-out infinite;opacity:0}',
    '@keyframes mp-comet{0%,82%{transform:translate(0,0);opacity:0}84%{opacity:1}96%{opacity:1}100%{transform:translate(-520px,210px);opacity:0}}',
    '.mp-comet{animation:mp-comet 11s linear infinite;opacity:0}',
    '@keyframes mp-float{0%,100%{transform:translateY(0) rotate(-1.5deg)}50%{transform:translateY(-14px) rotate(1.5deg)}}',
    '.mp-float{transform-box:fill-box;transform-origin:50% 50%;animation:mp-float 8s ease-in-out infinite}',
    '@keyframes mp-flow{to{stroke-dashoffset:-80}}',
    '.mp-flow{animation:mp-flow 1.6s linear infinite}.mp-flow.slow{animation-duration:2.8s}',
    '@keyframes mp-breathe{0%,100%{opacity:.55}50%{opacity:1}}',
    '.mp-breathe{animation:mp-breathe 4.5s ease-in-out infinite}'
  );

  // A made-up letter: a few strokes between the points of a small grid, so no two are alike and none is a real script.
  function mpRune(r, x, y, s) {
    var P = [[0, 0], [1, 0], [0, 1], [1, 1], [0, 2], [1, 2], [.5, .5], [.5, 1.5]], d = '', i, a = Math.floor(r() * 8), b;
    for (i = 0; i < 3 + Math.floor(r() * 2); i++) { b = Math.floor(r() * 8); if (b === a) b = (b + 3) % 8; d += 'M' + f1(x + P[a][0] * s) + ',' + f1(y + P[a][1] * s) + ' L' + f1(x + P[b][0] * s) + ',' + f1(y + P[b][1] * s) + ' '; a = b; }
    return d;
  }
  function mpStars(r, n, x0, y0, w, h, twinkle) {
    var s = '', i;
    for (i = 0; i < n; i++) s += '<circle cx="' + f1(x0 + r() * w) + '" cy="' + f1(y0 + r() * h) + '" r="' + f1(.6 + r() * 1.5) + '" fill="' + (r() < .2 ? '#ffe8c0' : r() < .4 ? '#c0d8ff' : '#fff') + '" opacity="' + f1(.3 + r() * .7) + '"' + (twinkle && r() < .5 ? ' class="mp-twinkle" style="animation-delay:-' + f1(r() * 3) + 's;animation-duration:' + f1(2 + r() * 3) + 's"' : '') + '/>';
    return s;
  }
  var FULL = 'x="-300" y="-150" width="1800" height="620"';

  WALLS.runestone = function (t) {
    var r = rng(53), s = WALLS.stone(t), x, a = t.th.accent, band;
    [58, 338].forEach(function (y) {
      band = '';
      for (x = -280; x < 1500; x += 34) band += mpRune(r, x, y, 15);
      s += '<rect x="-300" y="' + (y - 14) + '" width="1800" height="58" fill="#0a0810" opacity=".55"/><rect x="-300" y="' + (y - 14) + '" width="1800" height="2" fill="' + t.th.trim + '" opacity=".6"/><rect x="-300" y="' + (y + 42) + '" width="1800" height="2" fill="' + t.th.trim + '" opacity=".6"/>' +
        '<path d="' + band + '" stroke="' + a + '" stroke-width="7" fill="none" opacity=".35" filter="url(#src-blur5)"/><path d="' + band + '" stroke="' + mix(a, '#ffffff', .55) + '" stroke-width="1.8" fill="none" stroke-linecap="round"/>';
    });
    return s;
  };


  function mpOpen(d, inner, frame, after) { return '<g data-k="window"><path d="' + d + '" fill="#000" opacity=".5" filter="url(#src-blur8)" transform="translate(4,5)"/><g clip-path="url(#cm-oclip)">' + inner + '</g>' + frame + '</g>' + (after || '') + '<clipPath id="cm-oclip"><path d="' + d + '"/></clipPath>'; }
  var RING = 'M804,190 a108,108 0 1 0 216,0 a108,108 0 1 0 -216,0 Z';

  OPENINGS['lone-door'] = function (t) {
    var d = 'M846,452 V164 H978 V452 Z', r = rng(13), w = t.th.wood, s;
    // a door with no wall round it, standing open on a night that is not this place's
    s = '<defs>' + lin('mp-dn', 1, [[0, '#0c1238'], [.7, '#3a2a6a'], [1, '#c86a5a']]) + '</defs><rect x="840" y="160" width="150" height="300" fill="url(#mp-dn)"/>' + mpStars(r, 40, 848, 166, 128, 200, true) +
      '<circle cx="944" cy="214" r="15" fill="#fff6dc"/><path d="M846,430 q40,-26 70,-6 q30,-18 62,4 V452 H846 Z" fill="#0a0c1c"/>';
    var leaf = '<path d="M978,164 L1040,150 V470 L978,452 Z" fill="' + mix(w, '#000', .25) + '"/><path d="M986,176 L1032,166 V300 L986,304 Z M986,316 L1032,314 V452 L986,440 Z" fill="none" stroke="' + mix(w, '#000', .6) + '" stroke-width="2"/><circle cx="992" cy="310" r="5" fill="url(#cm-trimv)"/>';
    return mpOpen(d, s, cmFrame(t, 'M840,456 V158 H984 V456', 13) + leaf + '<rect x="826" y="452" width="230" height="8" rx="3" fill="' + mix(w, '#000', .4) + '"/>',
      '<path d="M846,452 L760,520 H1010 L978,452 Z" fill="url(#cm-shaft)" opacity=".8" pointer-events="none"/>');
  };
  OPENINGS.portal = function (t) {
    var a = t.th.accent, r = rng(23), i, ang, s, runes = '';
    s = '<rect x="800" y="78" width="224" height="224" fill="#0a0620"/><defs>' + rad('mp-pt', [[0, '#ffffff', .95], [.25, mix(a, '#ffffff', .4), .8], [.7, a, .35], [1, '#0a0620', 0]]) + '</defs>';
    s += '<g class="mp-spin" style="transform-origin:912px 190px;animation-duration:14s">';
    for (i = 0; i < 5; i++) s += '<path d="M912,190 q30,-70 110,-50" stroke="' + mix(a, '#ffffff', .5) + '" stroke-opacity=".7" stroke-width="' + (7 - i) + '" fill="none" stroke-linecap="round" transform="rotate(' + (i * 72) + ' 912 190)"/>';
    s += '</g><circle cx="912" cy="190" r="100" fill="url(#mp-pt)" class="mp-breathe"/>';
    for (i = 0; i < 12; i++) s += '<circle class="mp-twinkle" cx="' + f1(830 + r() * 164) + '" cy="' + f1(108 + r() * 164) + '" r="' + f1(1 + r() * 1.6) + '" fill="#fff" style="animation-delay:-' + f1(r() * 3) + 's"/>';
    // the ring is cut stone, and the letters round it turn the other way
    for (i = 0; i < 16; i++) { ang = i / 16 * 360; runes += '<path d="' + mpRune(r, 906, 52, 11) + '" transform="rotate(' + ang + ' 912 190)"/>'; }
    var frame = '<path d="' + RING + '" fill="none" stroke="#3a3640" stroke-width="40"/><path d="' + RING + '" fill="none" stroke="#6a6474" stroke-width="34" stroke-dasharray="52 4"/>' +
      '<g class="mp-spin rev" style="transform-origin:912px 190px;animation-duration:90s" stroke="' + mix(a, '#ffffff', .6) + '" stroke-width="1.8" fill="none" stroke-linecap="round">' + runes + '</g>' +
      '<circle cx="912" cy="190" r="109" fill="none" stroke="' + a + '" stroke-width="3" opacity=".8" class="mp-breathe"/>';
    return mpOpen(RING, s, frame, '<ellipse cx="912" cy="200" rx="250" ry="230" fill="url(#cm-acc)" opacity=".25" pointer-events="none"/>');
  };

  LIGHTS.wisps = function (t) {
    var x = t.lp.x, s = '<g data-k="lamp">';
    [[-26, 418, 11, 0], [18, 384, 14, 1], [44, 436, 9, 2]].forEach(function (q) {
      s += '<g class="cm-bob" style="animation-delay:-' + (q[3] * 1.9) + 's;animation-duration:' + (5 + q[3]) + 's"><circle cx="' + (x + q[0]) + '" cy="' + q[1] + '" r="' + (q[2] * 4) + '" fill="url(#cm-glow)" opacity=".8"/><circle cx="' + (x + q[0]) + '" cy="' + q[1] + '" r="' + q[2] + '" fill="#f4fff8"/><path d="M' + (x + q[0] - q[2] * .6) + ',' + (q[1] + q[2] * .6) + ' q' + (q[2] * .6) + ',' + (q[2] * 2.4) + ' ' + (q[2] * 1.4) + ',' + (q[2] * 3) + '" stroke="#d8fff0" stroke-opacity=".5" stroke-width="2" fill="none" stroke-linecap="round"/></g>';
    });
    return { back: '', front: s + '<circle cx="' + x + '" cy="410" r="110" fill="url(#cm-bloom)" class="cm-pulse" opacity=".7"/></g>' };
  };
  LIGHTS.star = function (t) {
    var x = t.lp.x, y = 420, i, s = '<g data-k="lamp"><ellipse cx="' + x + '" cy="482" rx="44" ry="7" fill="url(#src-ao)"/><path d="M' + (x - 26) + ',480 h52 l-8,-9 h-36 Z" fill="url(#cm-trim)"/><path d="M' + (x - 20) + ',471 q-16,-50 20,-78 q36,28 20,78" fill="none" stroke="url(#cm-trim)" stroke-width="3"/>';
    s += '<g class="mp-spin" style="transform-origin:' + x + 'px ' + y + 'px;animation-duration:30s">';
    for (i = 0; i < 8; i++) s += '<path d="M' + x + ',' + (y - (i % 2 ? 34 : 52)) + ' L' + (x + 4) + ',' + y + ' L' + (x - 4) + ',' + y + ' Z" fill="#fff6d0" opacity=".85" transform="rotate(' + (i * 45) + ' ' + x + ' ' + y + ')"/>';
    s += '</g><circle cx="' + x + '" cy="' + y + '" r="13" fill="#ffffff"/><circle cx="' + x + '" cy="' + y + '" r="120" fill="url(#cm-bloom)" class="cm-pulse"/></g>';
    return { back: '', front: s };
  };
  LIGHTS.halo = function (t) {
    var x = t.lp.x, s = '<g data-k="lamp"><g class="cm-bob"><ellipse cx="' + x + '" cy="392" rx="46" ry="12" fill="none" stroke="#ffffff" stroke-width="9" opacity=".5" filter="url(#src-blur5)"/><ellipse cx="' + x + '" cy="392" rx="46" ry="12" fill="none" stroke="#ffffff" stroke-width="4"/><ellipse cx="' + x + '" cy="392" rx="46" ry="12" fill="none" stroke="#c8d8ff" stroke-width="1"/></g><ellipse cx="' + x + '" cy="484" rx="40" ry="5" fill="#8a94b0" opacity=".3"/><circle cx="' + x + '" cy="400" r="130" fill="url(#cm-bloom)" opacity=".5"/></g>';
    return { back: '', front: s };
  };
  LIGHTS.witchfire = function (t) { var b = LIGHTS.brazier(t); return { back: '', front: '<g style="filter:hue-rotate(78deg) saturate(1.25)">' + b.front + '</g>' }; };
  LIGHTS.orb = function (t) {
    var x = t.lp.x, y = 410, a = t.th.accent, s = '<defs>' + rad('mp-orb', [[0, '#ffffff'], [.3, mix(a, '#ffffff', .5)], [1, mix(a, '#1a0a3a', .6)]], ' cx=".38" cy=".32" r=".75"') + '</defs><g data-k="lamp"><ellipse cx="' + x + '" cy="482" rx="40" ry="6" fill="url(#src-ao)"/><path d="M' + (x - 22) + ',480 h44 l-6,-8 h-32 Z" fill="url(#cm-trim)"/>';
    s += '<path d="M' + (x - 12) + ',472 q-22,-18 -16,-40 M' + (x + 12) + ',472 q22,-18 16,-40 M' + x + ',472 v-30" stroke="url(#cm-trim)" stroke-width="4" fill="none" stroke-linecap="round"/>';
    s += '<g class="cm-bob" style="animation-duration:7s"><circle cx="' + x + '" cy="' + y + '" r="26" fill="url(#mp-orb)"/><path d="M' + (x - 16) + ',' + (y - 10) + ' q6,-10 16,-12" stroke="#fff" stroke-opacity=".7" stroke-width="3" fill="none" stroke-linecap="round"/>' +
      '<g class="mp-spin" style="transform-origin:' + x + 'px ' + y + 'px;animation-duration:5s"><circle cx="' + (x + 40) + '" cy="' + y + '" r="3" fill="#fff"/></g><g class="mp-spin rev" style="transform-origin:' + x + 'px ' + y + 'px;animation-duration:8s"><circle cx="' + x + '" cy="' + (y - 46) + '" r="2.4" fill="' + mix(a, '#ffffff', .7) + '"/></g></g>';
    return { back: '', front: s + '<circle cx="' + x + '" cy="' + y + '" r="120" fill="url(#cm-bloom)" class="cm-pulse"/></g>' };
  };

  PROPS.petals = function (t, p) {
    var s = '', r = rng(101), i, c;
    for (i = 0; i < 30; i++) { c = mix(t.th.accent, '#ffffff', r() * .6); s += '<path class="mp-fall" d="M0,0 q5,-8 11,0 q-6,7 -11,0 Z" transform="translate(' + f1(40 + r() * 1160) + ',' + f1(-10 + r() * 30) + ')" fill="' + c + '" style="--fx:' + f1(-140 + r() * 180) + 'px;--fr:' + f1(200 + r() * 500) + 'deg;animation-duration:' + f1(9 + r() * 8) + 's;animation-delay:-' + f1(r() * 17) + 's"/>'; }
    return '<g class="src-nohit">' + s + '</g>';
  };
  PROPS.butterflies = function (t, p) {
    var s = '', r = rng(103), c;
    [[600, 110], [1010, 330], [300, 60], [760, 400], [1120, 150]].forEach(function (q, i) {
      c = mix(t.th.accent, ['#ffe070', '#70d0ff', '#ffffff'][i % 3], .5);
      s += '<g class="mp-fly" style="--fx:' + f1((r() - .5) * 160) + 'px;--fy:' + f1((r() - .5) * 90) + 'px;animation-duration:' + f1(9 + r() * 6) + 's;animation-delay:-' + f1(r() * 9) + 's"><g transform="translate(' + q[0] + ',' + q[1] + ') rotate(' + f1(r() * 40 - 20) + ')">' +
        '<path class="mp-flap l" d="M0,0 C-6,-16 -22,-16 -20,-2 C-22,8 -8,10 0,2 Z" fill="' + c + '" style="animation-delay:-' + f1(r()) + 's"/><path class="mp-flap r" d="M0,0 C6,-16 22,-16 20,-2 C22,8 8,10 0,2 Z" fill="' + c + '"/><path d="M0,-5 V6" stroke="#1a1410" stroke-width="2.2" stroke-linecap="round"/></g></g>';
    });
    return '<g class="src-nohit">' + s + '</g>';
  };
  PROPS.embers = function (t, p) {
    var s = '', r = rng(107), i;
    for (i = 0; i < 44; i++) s += '<circle class="mp-rise" cx="' + f1(20 + r() * 1160) + '" cy="' + f1(380 + r() * 130) + '" r="' + f1(1 + r() * 2) + '" fill="' + (r() < .5 ? '#ffd27a' : '#ff8a3a') + '" style="--fx:' + f1((r() - .5) * 120) + 'px;--up:-' + f1(260 + r() * 220) + 'px;animation-duration:' + f1(5 + r() * 6) + 's;animation-delay:-' + f1(r() * 11) + 's"/>';
    return '<g class="src-nohit">' + s + '</g>';
  };
  PROPS.smoke = function (t, p) {
    var s = '', r = rng(109), i;
    for (i = 0; i < 9; i++) s += '<ellipse class="mp-smoke" cx="' + f1(60 + r() * 1080) + '" cy="' + f1(70 + r() * 90) + '" rx="' + f1(90 + r() * 80) + '" ry="' + f1(26 + r() * 20) + '" fill="#1a1614" filter="url(#src-blur14)" style="--fx:' + f1((r() - .3) * 240) + 'px;animation-duration:' + f1(13 + r() * 9) + 's;animation-delay:-' + f1(r() * 22) + 's"/>';
    return '<g class="src-nohit">' + s + '</g>';
  };
  PROPS.comets = function (t, p) {
    var s = '<defs>' + lin('mp-tail', 0, [[0, '#ffffff', .95], [1, '#ffffff', 0]]) + '</defs>', r = rng(113);
    [[1180, 30, 0], [760, 20, 4.2], [1260, 150, 7.5]].forEach(function (q) { s += '<g class="mp-comet" style="animation-delay:-' + q[2] + 's;animation-duration:' + f1(9 + r() * 5) + 's"><g transform="translate(' + q[0] + ',' + q[1] + ') rotate(-22)"><rect x="0" y="-1.5" width="130" height="3" rx="1.5" fill="url(#mp-tail)"/><circle r="3.4" fill="#fff"/></g></g>'; });
    return '<g class="src-nohit">' + s + '</g>';
  };
  PROPS['floating-stones'] = function (t, p) {
    var s = '', r = rng(127), pale = t.th.wall === 'white', rock = pale ? '#7a8196' : '#3a3648';
    [[760, 92, 36], [770, 372, 22], [1090, 130, 44], [1150, 356, 26], [560, 372, 16], [300, 70, 30]].forEach(function (q, i) {
      var x = q[0], y = q[1], w = q[2], d = 'M' + (x - w) + ',' + y + ' h' + (w * 2) + ' l' + f1(-w * .4) + ',' + f1(w * .5) + ' l' + f1(-w * .5) + ',' + f1(w * .35) + ' l' + f1(-w * .6) + ',' + f1(-w * .4) + ' Z';
      s += '<g class="mp-float" style="animation-delay:-' + f1(i * 1.3) + 's;animation-duration:' + f1(6 + r() * 5) + 's"><path d="' + d + '" fill="' + rock + '"/><path d="' + d + '" fill="none" stroke="' + t.lc + '" stroke-opacity=".35" stroke-width="1.4"/><rect x="' + (x - w) + '" y="' + (y - 4) + '" width="' + (w * 2) + '" height="6" rx="3" fill="' + (pale ? '#b8d0a8' : '#5a5670') + '"/>' +
        (i % 2 ? '' : '<path d="M' + (x - w * .3) + ',' + (y - 4) + ' v-' + f1(w * .5) + ' M' + (x - w * .3) + ',' + f1(y - 4 - w * .5) + ' q-8,-6 -2,-14 q9,4 2,14" stroke="#5a8a4a" stroke-width="2" fill="#7ab05a"/>') + '</g>';
    });
    return '<g class="src-nohit">' + s + '</g>';
  };
  PROPS.eyes = function (t, p) {
    var s = '', r = rng(131), a = t.th.accent;
    [[1110, 120, 20, 0], [1150, 230, 13, 1], [1090, 330, 16, 2], [770, 96, 12, 3], [560, 330, 11, 4], [1160, 400, 10, 5], [740, 400, 9, 6]].forEach(function (q) {
      var x = q[0], y = q[1], w = q[2], rot = f1(r() * 50 - 25);
      s += '<g transform="rotate(' + rot + ' ' + x + ' ' + y + ')"><g class="mp-blink" style="animation-delay:-' + f1(r() * 6) + 's;animation-duration:' + f1(4.5 + r() * 5) + 's"><path d="M' + (x - w) + ',' + y + ' Q' + x + ',' + f1(y - w * .9) + ' ' + (x + w) + ',' + y + ' Q' + x + ',' + f1(y + w * .9) + ' ' + (x - w) + ',' + y + ' Z" fill="#0a0610"/>' +
        '<g' + (q[3] % 2 ? '' : ' class="mp-look"') + '><circle cx="' + x + '" cy="' + y + '" r="' + f1(w * .48) + '" fill="' + a + '"/><ellipse cx="' + x + '" cy="' + y + '" rx="' + f1(w * .12) + '" ry="' + f1(w * .4) + '" fill="#05030a"/><circle cx="' + f1(x - w * .16) + '" cy="' + f1(y - w * .18) + '" r="' + f1(w * .09) + '" fill="#fff" opacity=".8"/></g>' +
        '<path d="M' + (x - w) + ',' + y + ' Q' + x + ',' + f1(y - w * .9) + ' ' + (x + w) + ',' + y + '" fill="none" stroke="' + mix(a, '#000', .5) + '" stroke-width="1.6"/></g></g>';
    });
    return '<g class="src-nohit">' + s + '</g>';
  };
  PROPS.glyphs = function (t, p) {
    var s = '', r = rng(137), i, a = mix(t.th.accent, '#ffffff', .45), x, y;
    for (i = 0; i < 20; i++) { x = 60 + r() * 1080; if (x > 530 && x < 750) x += 230; y = 300 + r() * 180; s += '<g class="mp-rise" style="--fx:' + f1((r() - .5) * 60) + 'px;--up:-' + f1(220 + r() * 160) + 'px;animation-duration:' + f1(9 + r() * 8) + 's;animation-delay:-' + f1(r() * 17) + 's"><path d="' + mpRune(r, x, y, 10 + r() * 6) + '" stroke="' + t.th.accent + '" stroke-width="6" fill="none" opacity=".3" filter="url(#src-blur3)"/><path d="' + mpRune(rng(137 + i), x, y, 12) + '" stroke="' + a + '" stroke-width="1.6" fill="none" stroke-linecap="round" opacity=".9"/></g>'; }
    return '<g class="src-nohit">' + s + '</g>';
  };
  PROPS['magic-circle'] = function (t, p) {
    var cx = 641, cy = 250, a = t.th.accent, hi = mix(a, '#ffffff', .6), r = rng(139), i, ang, runes = '', star = '', s;
    for (i = 0; i < 24; i++) { ang = i / 24 * 360; runes += '<path d="' + mpRune(r, cx - 5, cy - 172, 10) + '" transform="rotate(' + ang + ' ' + cx + ' ' + cy + ')"/>'; }
    [0, 180].forEach(function (off) { for (i = 0; i < 3; i++) { ang = (off - 90 + i * 120) * Math.PI / 180; star += (i ? ' L' : 'M') + f1(cx + 148 * Math.cos(ang)) + ',' + f1(cy + 148 * Math.sin(ang)); } star += ' Z '; });
    s = '<g class="src-nohit mp-breathe"><circle cx="' + cx + '" cy="' + cy + '" r="170" fill="none" stroke="' + a + '" stroke-width="14" opacity=".25" filter="url(#src-blur8)"/>' +
      '<g class="mp-spin" style="transform-origin:' + cx + 'px ' + cy + 'px;animation-duration:80s" fill="none" stroke="' + hi + '" stroke-width="1.8"><circle cx="' + cx + '" cy="' + cy + '" r="180"/><circle cx="' + cx + '" cy="' + cy + '" r="150"/><g stroke-linecap="round">' + runes + '</g></g>' +
      '<g class="mp-spin rev" style="transform-origin:' + cx + 'px ' + cy + 'px;animation-duration:50s" fill="none" stroke="' + hi + '" stroke-width="1.6"><path d="' + star + '"/><circle cx="' + cx + '" cy="' + cy + '" r="118"/>';
    for (i = 0; i < 6; i++) s += '<circle cx="' + f1(cx + 148 * Math.cos(i * Math.PI / 3 - Math.PI / 2)) + '" cy="' + f1(cy + 148 * Math.sin(i * Math.PI / 3 - Math.PI / 2)) + '" r="12" fill="#0a0620" stroke="' + hi + '"/>';
    return s + '</g></g>';
  };

  /* ---- Displays and counters that are not carpentry, and a glade drawn
     as a forest. Goods sit on whatever the place offers: bracket fungus
     and roots, basalt columns, stones adrift, orbits, tendrils, or small
     circles of their own. A display still only says where twelve goods go,
     so every room plays the same. ---- */
  CM_OPT.display.push('roots', 'adrift', 'orbits', 'columns', 'tendrils', 'circles');
  CM_OPT.counter.push('log', 'slab', 'adrift');

  // A material: the shape keeps its own colours and gains a lit, uneven surface.
  function mpMat(id, fq, oct, seed, scale, az, k) {
    return '<filter id="' + id + '" x="-4%" y="-4%" width="108%" height="108%" color-interpolation-filters="sRGB"><feTurbulence type="fractalNoise" baseFrequency="' + fq + '" numOctaves="' + oct + '" seed="' + seed + '" result="n"/>' +
      '<feDiffuseLighting in="n" surfaceScale="' + scale + '" diffuseConstant="1" lighting-color="#ffffff" result="l"><feDistantLight azimuth="' + az + '" elevation="54"/></feDiffuseLighting>' +
      '<feComposite in="l" in2="SourceGraphic" operator="arithmetic" k1="' + (k || 1.3) + '" k2="0" k3="0" k4="0" result="m"/><feComposite in="m" in2="SourceGraphic" operator="in"/></filter>';
  }
  function mp3Defs() {
    return mpMat('mp-bark', '.05 .007', 4, 3, 7, 205) + mpMat('mp-barkh', '.007 .055', 4, 5, 7, 250) + mpMat('mp-moss', '.42', 3, 9, 2.6, 225, 1.45) +
      mpMat('mp-stone', '.022 .034', 5, 12, 6, 225) + mpMat('mp-fine', '.9', 2, 4, 1.2, 225, 1.25);
  }
  CM_CSS.push(
    '@keyframes mp-bob{0%,100%{transform:translateY(0)}50%{transform:translateY(-7px)}}',
    '.mp-floaty .src-ware,.mp-floaty .src-out{animation:mp-bob 6s ease-in-out infinite}',
    '@keyframes mp-turn{to{transform:rotate(360deg)}}',
    '.mp-turn{animation:mp-turn 26s linear infinite}.mp-turn.rev{animation-direction:reverse;animation-duration:40s}',
    '@keyframes mp-sway{0%,100%{transform:rotate(-2.2deg)}50%{transform:rotate(2.2deg)}}',
    '.mp-sway{animation:mp-sway 7s ease-in-out infinite}'
  );
  // Floating goods bob out of step with each other.
  [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12].forEach(function (i) {
    CM_CSS.push('.mp-floaty .src-ware:nth-child(' + i + '),.mp-floaty .src-out:nth-child(' + i + '){animation-delay:-' + f1(i * 1.37 % 6) + 's;animation-duration:' + f1(5 + i * .83 % 3) + 's}');
  });
  // A smooth line through the points, so a taper along it bends and never kinks.
  function mpSmooth(p, n) {
    var o = [], i, j, u, a, b, c, d;
    for (i = 0; i < p.length - 1; i++) { a = p[Math.max(0, i - 1)]; b = p[i]; c = p[i + 1]; d = p[Math.min(p.length - 1, i + 2)];
      for (j = 0; j < n; j++) { u = j / n; o.push([.5 * (2 * b[0] + (c[0] - a[0]) * u + (2 * a[0] - 5 * b[0] + 4 * c[0] - d[0]) * u * u + (3 * b[0] - a[0] - 3 * c[0] + d[0]) * u * u * u), .5 * (2 * b[1] + (c[1] - a[1]) * u + (2 * a[1] - 5 * b[1] + 4 * c[1] - d[1]) * u * u + (3 * b[1] - a[1] - 3 * c[1] + d[1]) * u * u * u)]); } }
    o.push(p[p.length - 1]); return o;
  }
  function mpSlots(a, sc) { return a.map(function (q) { return [q[0], q[1], q[2] || sc || 1.15, 0]; }); }

  /* ---- Growing things, drawn leaf by leaf ---- */
  function mpLeaf(x, y, s, a, col, rib) {
    return '<g transform="translate(' + f1(x) + ',' + f1(y) + ') rotate(' + f1(a) + ')"><path d="M0,0 C' + f1(s * .28) + ',' + f1(-s * .4) + ' ' + f1(s * .8) + ',' + f1(-s * .26) + ' ' + f1(s) + ',0 C' + f1(s * .8) + ',' + f1(s * .26) + ' ' + f1(s * .28) + ',' + f1(s * .4) + ' 0,0 Z" fill="' + col + '"/>' +
      (rib ? '<path d="M0,0 H' + f1(s * .92) + '" stroke="' + rib + '" stroke-width=".7" stroke-opacity=".55"/><path d="M' + f1(s * .08) + ',' + f1(-s * .02) + ' C' + f1(s * .3) + ',' + f1(-s * .3) + ' ' + f1(s * .75) + ',' + f1(-s * .2) + ' ' + f1(s * .96) + ',' + f1(-s * .02) + '" stroke="' + rib + '" stroke-width=".8" stroke-opacity=".35" fill="none"/>' : '') + '</g>';
  }
  // A fern frond: a curved rib with paired leaflets that shorten to the tip.
  function mpFern(x, y, len, ang, col, lite) {
    var s = '', n = Math.max(7, Math.round(len / 7)), i, u, px, py, a, l, bend = ang < -90 ? -1 : 1;
    for (i = 1; i <= n; i++) {
      u = i / n; a = (ang + bend * u * u * 55) * Math.PI / 180; px = x + Math.cos((ang + bend * u * 28) * Math.PI / 180) * len * u; py = y + Math.sin((ang + bend * u * 28) * Math.PI / 180) * len * u; l = len * .3 * (1 - u * .82) * (u < .15 ? u / .15 : 1);
      s += mpLeaf(px, py, l, a * 180 / Math.PI + 68, i % 2 ? col : lite) + mpLeaf(px, py, l, a * 180 / Math.PI - 68, i % 2 ? lite : col);
    }
    return '<path d="M' + f1(x) + ',' + f1(y) + ' Q' + f1(x + Math.cos(ang * Math.PI / 180) * len * .5) + ',' + f1(y + Math.sin(ang * Math.PI / 180) * len * .5) + ' ' + f1(px) + ',' + f1(py) + '" stroke="' + col + '" stroke-width="1.6" fill="none"/>' + s;
  }
  function mpFernClump(r, x, y, size, col, lite) {
    var s = '', i, n = 5 + Math.floor(r() * 4);
    for (i = 0; i < n; i++) s += mpFern(x + (r() - .5) * 8, y, size * (.6 + r() * .5), -90 + (i - (n - 1) / 2) * (120 / n) + (r() - .5) * 14, mix(col, '#000', r() * .3), mix(lite, col, r() * .5));
    return s;
  }
  // A small flower: five petals round a pale eye.
  function mpFlower(x, y, s, col) {
    var o = '', i, a;
    for (i = 0; i < 5; i++) { a = i * 72 - 90; o += '<ellipse cx="' + f1(x + Math.cos(a * Math.PI / 180) * s * .55) + '" cy="' + f1(y + Math.sin(a * Math.PI / 180) * s * .55) + '" rx="' + f1(s * .5) + '" ry="' + f1(s * .3) + '" transform="rotate(' + a + ' ' + f1(x + Math.cos(a * Math.PI / 180) * s * .55) + ' ' + f1(y + Math.sin(a * Math.PI / 180) * s * .55) + ')" fill="' + col + '"/>'; }
    return o + '<circle cx="' + f1(x) + '" cy="' + f1(y) + '" r="' + f1(s * .26) + '" fill="#fff4b0"/>';
  }
  // A toadstool with a domed, spotted cap, gills showing under the rim, and its own faint light.
  function mpShroom(x, y, h, col, glow) {
    var w = h * .62, cy = y - h;
    return (glow ? '<circle cx="' + f1(x) + '" cy="' + f1(cy) + '" r="' + f1(h * 1.1) + '" fill="' + col + '" opacity=".16" filter="url(#src-blur8)"/>' : '') +
      '<path d="M' + f1(x - h * .09) + ',' + f1(y) + ' C' + f1(x - h * .05) + ',' + f1(y - h * .5) + ' ' + f1(x - h * .1) + ',' + f1(cy + h * .2) + ' ' + f1(x - h * .07) + ',' + f1(cy) + ' H' + f1(x + h * .07) + ' C' + f1(x + h * .1) + ',' + f1(cy + h * .2) + ' ' + f1(x + h * .12) + ',' + f1(y - h * .5) + ' ' + f1(x + h * .12) + ',' + f1(y) + ' Z" fill="#e8dcc8"/><path d="M' + f1(x + h * .03) + ',' + f1(y) + ' V' + f1(cy) + ' h' + f1(h * .05) + ' C' + f1(x + h * .1) + ',' + f1(cy + h * .2) + ' ' + f1(x + h * .12) + ',' + f1(y - h * .5) + ' ' + f1(x + h * .12) + ',' + f1(y) + ' Z" fill="#000" opacity=".28"/>' +
      '<ellipse cx="' + f1(x) + '" cy="' + f1(cy + h * .03) + '" rx="' + f1(w * .92) + '" ry="' + f1(h * .1) + '" fill="' + mix(col, '#2a1020', .65) + '"/>' +
      '<path d="M' + f1(x - w) + ',' + f1(cy + h * .03) + ' C' + f1(x - w) + ',' + f1(cy - h * .5) + ' ' + f1(x + w) + ',' + f1(cy - h * .5) + ' ' + f1(x + w) + ',' + f1(cy + h * .03) + ' Q' + f1(x) + ',' + f1(cy - h * .06) + ' ' + f1(x - w) + ',' + f1(cy + h * .03) + ' Z" fill="' + col + '"/>' +
      '<path d="M' + f1(x - w * .8) + ',' + f1(cy - h * .08) + ' C' + f1(x - w * .6) + ',' + f1(cy - h * .34) + ' ' + f1(x - w * .1) + ',' + f1(cy - h * .36) + ' ' + f1(x + w * .1) + ',' + f1(cy - h * .34) + '" stroke="#fff" stroke-opacity=".5" stroke-width="' + f1(h * .05) + '" fill="none" stroke-linecap="round"/>' +
      '<path d="M' + f1(x + w * .2) + ',' + f1(cy - h * .33) + ' C' + f1(x + w * .8) + ',' + f1(cy - h * .26) + ' ' + f1(x + w) + ',' + f1(cy - h * .1) + ' ' + f1(x + w) + ',' + f1(cy + h * .03) + ' Q' + f1(x + w * .5) + ',' + f1(cy - h * .04) + ' ' + f1(x + w * .2) + ',' + f1(cy - h * .04) + ' Z" fill="#000" opacity=".22"/>' +
      [[-.5, -.16, .1], [.05, -.26, .08], [.5, -.13, .07], [-.15, -.1, .05]].map(function (q) { return '<ellipse cx="' + f1(x + w * q[0]) + '" cy="' + f1(cy + h * q[1]) + '" rx="' + f1(h * q[2]) + '" ry="' + f1(h * q[2] * .6) + '" fill="#fff8e8" opacity=".85"/>'; }).join('');
  }
  // Moss: a soft pad with a lit top and small tufts along its edge.
  function mpMossPad(r, x, y, w, h, col) {
    var d = 'M' + f1(x - w) + ',' + f1(y + h * .4), i, n = 9, s;
    for (i = 0; i <= n; i++) d += ' Q' + f1(x - w + (i - .5) / n * 2 * w) + ',' + f1(y - h * (1 + r() * .5) * Math.sin(Math.max(.25, (i - .5) / n) * Math.PI)) + ' ' + f1(x - w + i / n * 2 * w) + ',' + f1(y - h * .5 * Math.sin(i / n * Math.PI) + (i === n ? h * .4 : 0));
    d += ' Q' + f1(x) + ',' + f1(y + h * 1.1) + ' ' + f1(x - w) + ',' + f1(y + h * .4) + ' Z';
    s = '<path d="' + d + '" fill="' + col + '" filter="url(#mp-moss)"/>';
    for (i = 0; i < 7; i++) s += '<circle cx="' + f1(x + (r() - .5) * w * 1.6) + '" cy="' + f1(y - h * .5 * r()) + '" r="' + f1(1 + r() * 1.6) + '" fill="' + mix(col, '#e8ffb0', .55) + '" opacity=".7"/>';
    return s;
  }
  // A trailing vine with leaves down its length.
  function mpVine(r, x, y, len, col, lite, flower) {
    var s = '', pts = [], i, n = Math.round(len / 9), sw = (r() - .5) * 30, px, py;
    for (i = 0; i <= n; i++) pts.push([x + Math.sin(i / n * 3 + sw) * 9 + sw * (i / n), y + i / n * len]);
    s += '<path d="M' + pts.map(function (p) { return f1(p[0]) + ',' + f1(p[1]); }).join(' L') + '" stroke="' + mix(col, '#000', .35) + '" stroke-width="1.5" fill="none"/>';
    for (i = 1; i <= n; i++) { px = pts[i][0]; py = pts[i][1]; s += mpLeaf(px, py, 7 + r() * 6, (i % 2 ? 20 : 160) + r() * 30, r() < .5 ? col : lite, r() < .4 ? '#e8ffc0' : ''); }
    if (flower) s += mpFlower(pts[n][0], pts[n][1] + 3, 6, flower);
    return s;
  }
  // Wisteria: a hanging cluster of small blooms that narrows to a point.
  function mpRaceme(r, x, y, len, col) {
    var s = '<path d="M' + f1(x) + ',' + f1(y) + ' v' + f1(len * .2) + '" stroke="#2a3a20" stroke-width="1.4"/>', i, n = Math.round(len * 1.1), u, w;
    for (i = 0; i < n; i++) { u = i / n; w = (1 - u) * len * .22 + 2; s += '<ellipse cx="' + f1(x + (r() - .5) * 2 * w) + '" cy="' + f1(y + len * .12 + u * len * .9) + '" rx="' + f1(2.2 + r() * 2) + '" ry="' + f1(3 + r() * 2.4) + '" fill="' + mix(mix(col, '#3a2a8a', r() * .5), '#ffffff', r() * .45) + '" opacity="' + f1(.75 + r() * .25) + '"/>'; }
    return s;
  }

  /* ---- The glade, again: three depths of forest with mist between them,
     light coming down through the leaves, and undergrowth at the foot. ---- */
  WALLS.glade = function (t) {
    var r = rng(61), a = t.th.accent, s, i, x, w, y, c;
    function trunk(x, w, col, lean, op, extra) {
      var d = 'M' + f1(x - w * .5) + ',-150 C' + f1(x - w * .5 + lean) + ',100 ' + f1(x - w * .7 + lean * 1.4) + ',340 ' + f1(x - w * 1.6 + lean * 1.6) + ',480 H' + f1(x + w * 1.6 + lean * 1.6) + ' C' + f1(x + w * .7 + lean * 1.4) + ',340 ' + f1(x + w * .5 + lean) + ',100 ' + f1(x + w * .5) + ',-150 Z', o = '<path d="' + d + '" fill="' + col + '" opacity="' + op + '"' + (extra || '') + '/>', by, k;
      if (r() < .8) { by = 60 + r() * 220; k = r() < .5 ? -1 : 1; o += '<path d="' + mpTaper([[x + lean * .5, by + 30], [x + k * w * 1.6, by - 10], [x + k * w * 3.4, by - 60], [x + k * w * 5, by - 140]], function (u) { return w * .5 * (1 - u * .85); }) + '" fill="' + col + '" opacity="' + op + '"' + (extra || '') + '/>'; }
      return o;
    }
    s = '<defs>' + mp3Defs() + lin('mp-gl', 1, [[0, '#101a34'], [.28, '#17404e'], [.6, '#2f8472'], [.8, '#1c5444'], [1, '#07140f']]) + rad('mp-glsun', [[0, '#f2ffd8', .62], [.35, '#a8ecc8', .22], [1, '#a8ecc8', 0]]) + lin('mp-ray', 1, [[0, '#f4ffdc', 0], [.12, '#f4ffdc', .3], [.7, '#f4ffdc', .08], [1, '#f4ffdc', 0]]) + '</defs>';
    s += '<rect ' + FULL + ' fill="url(#mp-gl)"/><ellipse cx="860" cy="170" rx="560" ry="330" fill="url(#mp-glsun)"/>';
    // far trees: pale and soft, lost in the haze
    var far = ''; for (i = 0; i < 38; i++) far += trunk(-280 + r() * 1760, 5 + r() * 9, mix('#2f7a72', '#8ed0bc', r()), (r() - .5) * 30, f1(.3 + r() * .35));
    s += '<g filter="url(#src-blur3)">' + far + '</g>';
    for (i = 0; i < 7; i++) s += '<ellipse cx="' + f1(-200 + r() * 1600) + '" cy="' + f1(250 + r() * 150) + '" rx="' + f1(180 + r() * 200) + '" ry="' + f1(26 + r() * 40) + '" fill="#c8f0e0" opacity="' + f1(.06 + r() * .08) + '" filter="url(#src-blur14)"/>';
    // middle trees: darker, with bark you can see
    var mid = ''; for (i = 0; i < 15; i++) { x = -280 + r() * 1760; if (x > 520 && x < 760) x += 260; mid += trunk(x, 12 + r() * 16, mix('#10302c', '#24564c', r()), (r() - .5) * 40, 1); }
    s += '<g filter="url(#mp-bark)">' + mid + '</g>';
    // light falling through
    for (i = 0; i < 7; i++) { x = 420 + i * 130 + r() * 60; w = 30 + r() * 70; s += '<path d="M' + f1(x) + ',-150 h' + f1(w) + ' l' + f1(-300 - w * .6) + ',640 h' + f1(-w * 2.2) + ' Z" fill="url(#mp-ray)" opacity="' + f1(.35 + r() * .5) + '" filter="url(#src-blur8)"/>'; }
    for (i = 0; i < 6; i++) s += '<ellipse cx="' + f1(-200 + r() * 1600) + '" cy="' + f1(360 + r() * 90) + '" rx="' + f1(200 + r() * 200) + '" ry="' + f1(22 + r() * 30) + '" fill="#d8ffe8" opacity="' + f1(.07 + r() * .08) + '" filter="url(#src-blur14)"/>';
    // near trees: almost black against the light, edged where it catches them
    var near = ''; [[-150, 30, 20], [560, 22, -14], [1010, 28, 18], [1330, 34, -20]].forEach(function (q) { near += trunk(q[0], q[1], '#0c1a18', q[2], 1); });
    s += '<g filter="url(#mp-bark)">' + near + '</g>';
    // undergrowth: ferns, foxgloves, grass and small lights low down
    for (i = 0; i < 26; i++) { x = -260 + r() * 1720; y = 452 + r() * 26; s += mpFernClump(r, x, y, 46 + r() * 50, mix('#0a2a1c', '#184a2c', r()), mix('#2a6a3a', '#5aa058', r())); }
    for (i = 0; i < 12; i++) { x = 520 + r() * 680; y = 470; var hgt = 60 + r() * 70, k, bc = mix(a, '#ffffff', r() * .4);
      s += '<path d="M' + f1(x) + ',' + y + ' q' + f1((r() - .5) * 14) + ',' + f1(-hgt * .5) + ' 0,' + f1(-hgt) + '" stroke="#2f6a3a" stroke-width="2" fill="none"/>';
      for (k = 0; k < 9; k++) s += '<path d="M' + f1(x + (k % 2 ? 1 : -1) * 2) + ',' + f1(y - hgt * (.38 + k * .068)) + ' q' + f1((k % 2 ? 1 : -1) * (9 - k * .7)) + ',1 ' + f1((k % 2 ? 1 : -1) * (10 - k * .8)) + ',' + f1(10 - k * .6) + ' q' + f1((k % 2 ? -1 : 1) * 5) + ',1 ' + f1((k % 2 ? -1 : 1) * (10 - k * .8)) + ',' + f1(-6 + k * .3) + ' Z" fill="' + mix(bc, '#5a1a5a', k % 3 * .18) + '"/>'; }
    for (i = 0; i < 120; i++) { x = -280 + r() * 1760; s += '<path d="M' + f1(x) + ',474 q' + f1((r() - .5) * 12) + ',' + f1(-14 - r() * 22) + ' ' + f1((r() - .5) * 22) + ',' + f1(-26 - r() * 34) + '" stroke="' + mix('#1f5a30', '#8ad070', r() * .8) + '" stroke-width="' + f1(.8 + r()) + '" stroke-opacity=".8" fill="none"/>'; }
    for (i = 0; i < 60; i++) { c = mix(a, '#fff8c0', r()); x = -260 + r() * 1720; y = -60 + r() * 500; w = 1.2 + r() * 4; s += '<circle cx="' + f1(x) + '" cy="' + f1(y) + '" r="' + f1(w * 3) + '" fill="' + c + '" opacity="' + f1(.05 + r() * .1) + '" filter="url(#src-blur3)"/><circle cx="' + f1(x) + '" cy="' + f1(y) + '" r="' + f1(w * .5) + '" fill="#fffff0" opacity="' + f1(.3 + r() * .5) + '"/>'; }
    return s;
  };
  SHAPES.glade = function (t) {
    var s = WALLS[t.th.wall](t), r = rng(15), a = t.th.accent, i, x, y, c, lf = '', dk = '', n;
    // two great trees stand at the edges, close enough to see the bark
    function big(x0, k) {
      var d = 'M' + x0 + ',-150 C' + (x0 - k * 10) + ',80 ' + (x0 - k * 26) + ',300 ' + (x0 - k * 78) + ',480 H' + (x0 + k * 400) + ' V-150 Z';
      return '<path d="' + d + '" fill="#000" opacity=".45" filter="url(#src-blur8)" transform="translate(' + (-k * 14) + ',0)"/><path d="' + d + '" fill="#241a12" filter="url(#mp-bark)"/><path d="' + d + '" fill="none" stroke="' + mix(t.lc, '#c8ffe0', .5) + '" stroke-opacity=".22" stroke-width="5" filter="url(#src-blur3)"/>';
    }
    s += big(1142, 1);
    // the canopy: a dark mass first, then leaves laid over it in clumps, lighter where the light comes through
    for (i = 0; i < 40; i++) dk += '<ellipse cx="' + f1(-300 + r() * 1800) + '" cy="' + f1(-150 + r() * 150) + '" rx="' + f1(80 + r() * 120) + '" ry="' + f1(40 + r() * 50) + '" fill="#061410"/>';
    s += '<g filter="url(#src-blur5)">' + dk + '</g>';
    for (i = 0; i < 70; i++) {
      x = -300 + r() * 1800; y = -150 + Math.pow(r(), 1.5) * 235; if (x > 500 && x < 780 && y > 30) y *= .3; n = 7 + Math.floor(r() * 7);
      var lit = Math.max(0, Math.min(1, (x - 300) / 900)) * .5 + r() * .5, base = mix('#0c2a1c', '#2f7a44', Math.min(1, (y + 150) / 240) * .8 + lit * .3), ang0 = r() * 360, k;
      for (k = 0; k < n; k++) lf += mpLeaf(x + (r() - .5) * 30, y + (r() - .5) * 22, 16 + r() * 18, ang0 + k * (360 / n) + r() * 20, mix(base, k % 3 ? '#061410' : '#a8e070', k % 3 ? r() * .35 : r() * .4 * lit), k % 4 === 0 ? '#d8ffb0' : '');
    }
    s += lf;
    // blossom hangs in long clusters, and vines trail down
    [[70, 20, 90], [150, 46, 60], [520, 30, 50], [770, 36, 70], [850, 60, 110], [960, 40, 84], [1050, 70, 120], [1110, 34, 76], [1180, 56, 60]].forEach(function (q) { s += mpRaceme(r, q[0], q[1], q[2], a); });
    [[30, 10, 150, 0], [720, 20, 120, 1], [905, 30, 170, 0], [1010, 20, 100, 1], [1128, 60, 260, 0], [1164, 40, 180, 1]].forEach(function (q) { s += mpVine(r, q[0], q[1], q[2], '#1f5a30', '#4a9a4a', q[3] ? '#fff0f8' : ''); });
    // ivy climbs the near tree
    for (i = 0; i < 46; i++) { y = 120 + r() * 340; x = 1150 - (y > 300 ? (y - 300) * .32 : 0) + r() * 40 - 4; s += mpLeaf(x, y, 9 + r() * 9, r() * 360, mix('#1f5a30', '#6ab050', r()), r() < .5 ? '#d8ffb0' : ''); }
    return s;
  };

  /* ---- Goods on a living tree: bracket fungus for the upper two rows and
     a root that humps along the ground for the lowest. ---- */
  DISPLAYS.roots = function (t) {
    var r = rng(77), a = t.th.accent, P = [[88, 172], [198, 150], [318, 180], [430, 158], [72, 290], [184, 304], [302, 278], [420, 298], [106, 410], [216, 396], [330, 412], [440, 398]], sl = mpSlots(P, 1.15), s, i, x, y, d, q;
    s = '<defs>' + mp3Defs() + lin('mp-bole', 0, [[0, '#120c08'], [.2, '#3a2a1c'], [.5, '#5a4430'], [.78, '#33241a'], [1, '#0e0906']]) + lin('mp-fung', 1, [[0, '#c8b088'], [1, '#4a3420']]) + rad('mp-hollow', [[0, '#000', .95], [.7, '#000', .8], [1, '#000', 0]]) + '</defs>';
    d = 'M74,-150 C70,40 60,230 22,350 C8,400 -30,436 -90,460 H590 C540,440 510,404 498,350 C470,230 456,40 452,-150 Z';
    s += '<path d="' + d + '" fill="#000" opacity=".5" filter="url(#src-blur14)" transform="translate(16,4)"/><path d="' + d + '" fill="url(#mp-bole)" filter="url(#mp-bark)"/>';
    // furrows in the bark, and a few knots
    for (i = 0; i < 46; i++) { x = 40 + r() * 440; y = -100 + r() * 480; q = 40 + r() * 120; s += '<path d="M' + f1(x) + ',' + f1(y) + ' q' + f1((r() - .5) * 14) + ',' + f1(q * .5) + ' ' + f1((r() - .5) * 10) + ',' + f1(q) + '" stroke="#060403" stroke-opacity="' + f1(.35 + r() * .4) + '" stroke-width="' + f1(1 + r() * 2.4) + '" fill="none" stroke-linecap="round"/><path d="M' + f1(x + 3) + ',' + f1(y + 4) + ' q' + f1((r() - .5) * 14) + ',' + f1(q * .5) + ' ' + f1((r() - .5) * 10) + ',' + f1(q - 8) + '" stroke="#c8a880" stroke-opacity=".1" stroke-width="1.2" fill="none"/>'; }
    [[250, 232, 20, 30], [376, 92, 13, 18], [140, 96, 10, 15]].forEach(function (k) { s += '<ellipse cx="' + k[0] + '" cy="' + k[1] + '" rx="' + (k[2] + 8) + '" ry="' + (k[3] + 9) + '" fill="none" stroke="#080504" stroke-opacity=".6" stroke-width="5"/><ellipse cx="' + k[0] + '" cy="' + k[1] + '" rx="' + (k[2] + 3) + '" ry="' + (k[3] + 4) + '" fill="none" stroke="#b89868" stroke-opacity=".16" stroke-width="2"/><ellipse cx="' + k[0] + '" cy="' + k[1] + '" rx="' + k[2] + '" ry="' + k[3] + '" fill="url(#mp-hollow)"/>'; });
    s += '<circle cx="246" cy="236" r="2.6" fill="#ffe890" opacity=".9"/><circle cx="256" cy="236" r="2.6" fill="#ffe890" opacity=".9"/>';
    // the root along the ground: it rises under each of the four low goods
    var pts = [[-30, 452]], humps = [[106, 410], [216, 396], [330, 412], [440, 398]];
    humps.forEach(function (h, j) { pts.push([h[0] - 52, h[1] + 44], [h[0] - 22, h[1] + 17], [h[0], h[1] + 13], [h[0] + 22, h[1] + 17]); });
    pts.push([500, 440], [540, 462]);
    d = mpTaper(pts, function (u) { return 30 - u * 8; });
    s += '<path d="' + d + '" fill="#000" opacity=".55" filter="url(#src-blur5)" transform="translate(4,8)"/><path d="' + d + '" fill="#3a2a1c" filter="url(#mp-barkh)"/><path d="M' + pts.map(function (p) { return f1(p[0]) + ',' + f1(p[1] - 9); }).join(' L') + '" stroke="#d8b888" stroke-opacity=".2" stroke-width="3" fill="none" stroke-linejoin="round" filter="url(#src-blur3)"/>';
    [[60, 452, 30], [262, 452, 26], [386, 454, 22]].forEach(function (k) { var dd = mpTaper([[k[0] - k[2], 462], [k[0], 462 - k[2]], [k[0] + k[2], 462]], function () { return 12; }); s += '<path d="' + dd + '" fill="#2a1c12" filter="url(#mp-barkh)"/>'; });
    // bracket fungus under the upper eight: a banded top, a pale underside with gills, and a shadow on the bark
    P.slice(0, 8).forEach(function (p, j) {
      var x = p[0], y = p[1] + 1, w = 50 + r() * 8, h = 13 + r() * 6, tone = [['#8a5a34', '#c88a48', '#e8c890'], ['#6a4a3a', '#a87858', '#e0c8a8'], ['#7a3a2a', '#c86a3a', '#f0c080']][j % 3], k;
      s += '<ellipse cx="' + (x + 6) + '" cy="' + (y + h + 4) + '" rx="' + w + '" ry="' + (h * .7) + '" fill="#000" opacity=".5" filter="url(#src-blur8)"/>';
      s += '<path d="M' + (x - w) + ',' + y + ' Q' + (x - w * .7) + ',' + (y + h) + ' ' + x + ',' + (y + h + 4) + ' Q' + (x + w * .7) + ',' + (y + h) + ' ' + (x + w) + ',' + y + ' Z" fill="url(#mp-fung)"/>';
      for (k = -7; k <= 7; k++) s += '<path d="M' + f1(x + k * w / 8) + ',' + (y + 3) + ' Q' + f1(x + k * w / 10) + ',' + f1(y + h * .6) + ' ' + f1(x + k * 1.2) + ',' + f1(y + h + 2) + '" stroke="#5a3a24" stroke-opacity=".35" stroke-width=".8" fill="none"/>';
      s += '<ellipse cx="' + x + '" cy="' + y + '" rx="' + w + '" ry="10" fill="' + tone[2] + '"/><ellipse cx="' + x + '" cy="' + (y - 1) + '" rx="' + f1(w * .88) + '" ry="8.4" fill="' + tone[1] + '"/><ellipse cx="' + x + '" cy="' + (y - 2) + '" rx="' + f1(w * .7) + '" ry="6.4" fill="' + tone[0] + '"/><ellipse cx="' + x + '" cy="' + (y - 3) + '" rx="' + f1(w * .5) + '" ry="4.4" fill="' + mix(tone[1], '#000', .2) + '"/><ellipse cx="' + x + '" cy="' + (y - 3.6) + '" rx="' + f1(w * .28) + '" ry="2.6" fill="' + mix(tone[0], '#000', .45) + '"/>';
      s += '<path d="M' + (x - w * .9) + ',' + (y + 2) + ' A' + w + ',10 0 0 0 ' + (x + w * .9) + ',' + (y + 2) + '" stroke="#fff8e0" stroke-opacity=".5" stroke-width="1.4" fill="none"/>';
      if (j % 2) s += mpMossPad(r, x - w * .5, y - 2, 16, 6, mix('#2c5222', '#5c8232', r()));
      for (k = 0; k < 3; k++) s += '<path d="M' + f1(x - w * .7 + r() * w * 1.4) + ',' + (y + h * .6) + ' v' + f1(10 + r() * 22) + '" stroke="#6a9a4a" stroke-opacity=".6" stroke-width="1"/>';
    });
    // moss on the root's humps, with flowers, and small lit toadstools in the gaps
    humps.forEach(function (h, j) { s += mpMossPad(r, h[0], h[1] + 2, 40, 9, mix('#2c5222', '#5c8232', r())); s += mpFlower(h[0] - 34, h[1] - 2, 4.4, mix(a, '#ffffff', .4)) + mpFlower(h[0] + 36, h[1] + 4, 3.6, '#fff8e8'); });
    [[160, 448, 26], [146, 450, 16], [270, 446, 22], [385, 450, 28], [398, 452, 15], [476, 446, 20], [30, 448, 24]].forEach(function (k, j) { s += mpShroom(k[0], k[1], k[2], j % 2 ? mix(a, '#ffffff', .2) : mix(a, '#6a8aff', .5), true); });
    // ivy and ferns grow up the bole
    [[40, 60, 200], [120, 200, 120], [470, 40, 230], [364, 196, 70], [236, 30, 90]].forEach(function (k) { s += mpVine(r, k[0], k[1], k[2], '#1f5a30', '#5aa24a', ''); });
    s += mpFernClump(r, 14, 456, 70, '#0f3a22', '#3a7a3c') + mpFernClump(r, 500, 456, 60, '#0f3a22', '#3a7a3c');
    return { art: s, slots: sl, zone: [34, 96, 454, 320], wallsh: false, decor: false, label: 'The old tree',
      base: function (it, p) { return '<ellipse cx="' + p[0] + '" cy="' + (p[1] + 1) + '" rx="' + f1(24 * p[2]) + '" ry="4" fill="#000" opacity=".45" filter="url(#src-blur3)"/>'; } };
  };

  /* ---- A fallen log for a counter: a split face to work on, bark below,
     moss hanging over the edge and things growing out of it. ---- */
  COUNTERS.log = function (t) {
    var s = cmTop(t, 'M-300,452 H1500 V510 H-300 Z'), r = rng(31), a = t.th.accent, x, i, d, g = '';
    s += '<clipPath id="cm-topclip"><rect x="-300" y="452" width="1800" height="58"/></clipPath><rect x="-300" y="504" width="1800" height="8" fill="#000" filter="url(#cm-beam)"/><rect x="-300" y="504" width="1800" height="2" fill="' + t.lc + '" opacity=".4"/>';
    g += '<defs>' + mp3Defs() + lin('mp-foot', 1, [[0, '#000', 0], [1, '#000', .55]]) + lin('mp-logf', 1, [[0, '#4a3624'], [.2, '#33241a'], [.7, '#1a120c'], [1, '#080504']]) + lin('mp-mossg', 1, [[0, '#6f8f3c'], [.45, '#3a5c28'], [1, '#132a16']]) + '</defs>';
    g += '<rect x="-300" y="510" width="1800" height="390" fill="url(#mp-logf)" filter="url(#mp-barkh)"/>';
    for (i = 0; i < 70; i++) { x = -300 + r() * 1800; var y = 524 + r() * 170, w = 40 + r() * 140; g += '<path d="M' + f1(x) + ',' + f1(y) + ' q' + f1(w * .5) + ',' + f1((r() - .5) * 12) + ' ' + f1(w) + ',' + f1((r() - .5) * 8) + '" stroke="#040302" stroke-opacity="' + f1(.4 + r() * .4) + '" stroke-width="' + f1(1.4 + r() * 3) + '" fill="none" stroke-linecap="round"/><path d="M' + f1(x + 4) + ',' + f1(y + 4) + ' q' + f1(w * .5) + ',' + f1((r() - .5) * 12) + ' ' + f1(w - 10) + ',' + f1((r() - .5) * 8) + '" stroke="#c8a880" stroke-opacity=".1" stroke-width="1.2" fill="none"/>'; }
    // a knot or two, and a broken stub
    [[150, 600, 26], [930, 620, 34]].forEach(function (k) { g += '<ellipse cx="' + k[0] + '" cy="' + k[1] + '" rx="' + (k[2] + 10) + '" ry="' + (k[2] * .8 + 8) + '" fill="none" stroke="#050302" stroke-opacity=".6" stroke-width="6"/><ellipse cx="' + k[0] + '" cy="' + k[1] + '" rx="' + k[2] + '" ry="' + (k[2] * .8) + '" fill="#6a4e34"/><ellipse cx="' + k[0] + '" cy="' + k[1] + '" rx="' + (k[2] * .66) + '" ry="' + (k[2] * .5) + '" fill="none" stroke="#3a2818" stroke-width="2"/><ellipse cx="' + k[0] + '" cy="' + k[1] + '" rx="' + (k[2] * .32) + '" ry="' + (k[2] * .24) + '" fill="none" stroke="#3a2818" stroke-width="1.6"/>'; });
    // moss hangs over the edge in an uneven fringe
    d = 'M-300,500'; for (x = -300; x < 1500;) { var w2 = 26 + r() * 60, dp = r() < .3 ? 26 + r() * 46 : 6 + r() * 16; d += ' q' + f1(w2 * .5) + ',' + f1(dp * 1.6) + ' ' + f1(w2) + ',' + f1((r() - .5) * 4); x += w2; } d += ' V500 Z';
    g += '<path d="' + d + '" fill="#000" opacity=".5" filter="url(#src-blur5)" transform="translate(0,7)"/><path d="' + d + '" fill="url(#mp-mossg)" filter="url(#mp-moss)"/>';
    for (i = 0; i < 90; i++) { x = -300 + r() * 1800; g += '<path d="M' + f1(x) + ',' + f1(506 + r() * 14) + ' v' + f1(8 + r() * 30) + '" stroke="' + mix('#4a8030', '#a8d060', r()) + '" stroke-opacity=".7" stroke-width="' + f1(.8 + r()) + '" stroke-linecap="round"/>'; }
    for (i = 0; i < 14; i++) { x = -280 + r() * 1760; g += mpFlower(x, 508 + r() * 12, 3 + r() * 2.4, r() < .5 ? mix(a, '#ffffff', .3 + r() * .4) : '#fff8e8'); }
    // growth at the foot and on the bark
    for (i = 0; i < 9; i++) g += mpFernClump(r, -200 + i * 190 + r() * 80, 704, 60 + r() * 50, '#0f3a22', '#3a7a3c');
    [[90, 700, 44], [120, 702, 26], [560, 702, 36], [600, 700, 54], [636, 704, 24], [1090, 700, 46], [1130, 704, 28]].forEach(function (k, j) { g += mpShroom(k[0], k[1], k[2], j % 2 ? mix(a, '#ffffff', .2) : mix(a, '#6a8aff', .5), true); });
    [[330, 556], [372, 562], [760, 568], [800, 560], [836, 570]].forEach(function (k, j) { var w = 22 + (j % 3) * 6; g += '<ellipse cx="' + k[0] + '" cy="' + (k[1] + 14) + '" rx="' + w + '" ry="8" fill="#000" opacity=".5" filter="url(#src-blur3)"/><path d="M' + (k[0] - w) + ',' + k[1] + ' Q' + k[0] + ',' + (k[1] + 22) + ' ' + (k[0] + w) + ',' + k[1] + ' Z" fill="#b89868"/><ellipse cx="' + k[0] + '" cy="' + k[1] + '" rx="' + w + '" ry="5" fill="#e8c890"/><ellipse cx="' + k[0] + '" cy="' + (k[1] - 1) + '" rx="' + (w * .7) + '" ry="3.4" fill="#c88a48"/><ellipse cx="' + k[0] + '" cy="' + (k[1] - 1.6) + '" rx="' + (w * .4) + '" ry="2" fill="#8a5a34"/>'; });
    return '<g filter="url(#cm-woodlit)">' + s + g + '</g><rect x="-300" y="560" width="1800" height="160" fill="url(#mp-foot)"/>';
  };

  var MP_P = [[88, 172], [198, 150], [318, 180], [430, 158], [72, 290], [184, 304], [302, 278], [420, 298], [106, 410], [216, 396], [330, 412], [440, 398]];
  function mpHexTop(x, y, w) { return 'M' + (x - w) + ',' + y + ' L' + f1(x - w * .5) + ',' + (y - 9) + ' L' + f1(x + w * .5) + ',' + (y - 9) + ' L' + (x + w) + ',' + y + ' L' + f1(x + w * .5) + ',' + (y + 9) + ' L' + f1(x - w * .5) + ',' + (y + 9) + ' Z'; }

  /* ---- Basalt columns: six-sided pillars of cooled rock at three heights,
     with the fire still showing in the gaps between them. ---- */
  DISPLAYS.columns = function (t) {
    var r = rng(91), a = t.th.accent, s, row, i, base = mix(t.th.wood, '#18161a', .7);
    s = '<defs>' + mp3Defs() + lin('mp-lavaup', 1, [[0, a, 0], [.6, a, .35], [1, '#fff0c0', .8]]) + '</defs>';
    function col(x, y, w, lit) {
      var o = '', k, yy;
      o += '<path d="M' + (x - w) + ',' + y + ' L' + f1(x - w * .5) + ',' + (y + 9) + ' V480 H' + (x - w) + ' Z" fill="' + mix(base, '#000', .35) + '"/><path d="M' + f1(x - w * .5) + ',' + (y + 9) + ' H' + f1(x + w * .5) + ' V480 H' + f1(x - w * .5) + ' Z" fill="' + mix(base, '#6a6058', .25) + '"/><path d="M' + f1(x + w * .5) + ',' + (y + 9) + ' L' + (x + w) + ',' + y + ' V480 H' + f1(x + w * .5) + ' Z" fill="' + mix(base, '#000', .6) + '"/>';
      for (k = 0; k < 3; k++) { yy = y + 30 + r() * 150; o += '<path d="M' + (x - w) + ',' + f1(yy) + ' L' + f1(x - w * .5) + ',' + f1(yy + 7) + ' H' + f1(x + w * .5) + ' L' + (x + w) + ',' + f1(yy - 2) + '" stroke="#000" stroke-opacity=".55" stroke-width="1.6" fill="none"/><path d="M' + f1(x - w * .5) + ',' + f1(yy + 9) + ' H' + f1(x + w * .5) + '" stroke="#b8a898" stroke-opacity=".14" stroke-width="1"/>'; }
      o += '<path d="' + mpHexTop(x, y, w) + '" fill="' + mix(base, '#8a8078', lit ? .5 : .3) + '"/><path d="' + mpHexTop(x, y, w) + '" fill="none" stroke="' + mix(a, '#fff0c0', .3) + '" stroke-opacity=".5" stroke-width="1.3"/>';
      o += '<path d="M' + (x - w + 6) + ',' + y + ' L' + f1(x - w * .2) + ',' + (y + 4) + ' L' + f1(x + w * .3) + ',' + (y - 3) + '" stroke="#000" stroke-opacity=".35" stroke-width="1" fill="none"/>';
      return o;
    }
    for (row = 0; row < 3; row++) {
      var g = '', xs = [], y0 = [165, 292, 404][row];
      // the fire in the gaps behind this row
      s += '<rect x="20" y="' + (y0 - 30) + '" width="480" height="' + (480 - y0 + 30) + '" fill="url(#mp-lavaup)" opacity="' + [.25, .4, .6][row] + '"/>';
      for (i = 0; i < 7; i++) g += col(40 + i * 72 + r() * 20, y0 - 40 + r() * 70 + (i % 2) * 14, 30 + r() * 10, false);
      for (i = 0; i < 4; i++) g += col(MP_P[row * 4 + i][0], MP_P[row * 4 + i][1] + 1, 46, true);
      s += '<g filter="url(#mp-stone)">' + g + '</g>';
      for (i = 0; i < 4; i++) { var x = MP_P[row * 4 + i][0], y = MP_P[row * 4 + i][1]; s += '<path d="M' + (x - 46) + ',' + (y + 2) + ' V' + (y + 90) + '" stroke="' + a + '" stroke-opacity=".5" stroke-width="2.4" filter="url(#src-blur3)"/><path d="M' + (x + 46) + ',' + (y + 2) + ' V' + (y + 70) + '" stroke="' + a + '" stroke-opacity=".35" stroke-width="2" filter="url(#src-blur3)"/>'; }
    }
    s += '<rect x="20" y="430" width="480" height="40" fill="' + a + '" opacity=".35" filter="url(#src-blur8)"/>';
    return { art: s, slots: mpSlots(MP_P, 1.15), zone: [34, 96, 454, 320], wallsh: false, decor: false, label: 'The basalt columns',
      base: function (it, p) { return '<ellipse cx="' + p[0] + '" cy="' + (p[1] + 1) + '" rx="' + f1(24 * p[2]) + '" ry="4.5" fill="#000" opacity=".55" filter="url(#src-blur3)"/>'; } };
  };

  /* ---- Adrift: nothing holds the goods up. Each rides its own piece of
     ground, and they rise and fall a little, out of step. ---- */
  DISPLAYS.adrift = function (t) {
    var pale = t.th.wall === 'white', rock = pale ? '#8a90a4' : mix(t.th.wood, '#2a2840', .5), top = pale ? '#b8d0a8' : mix(t.th.accent, '#3a3860', .6), r = rng(19), s = '', i;
    for (i = 0; i < 30; i++) { var x = 30 + r() * 460, y = 90 + r() * 360, w = 2 + r() * 5; s += '<path d="M' + f1(x - w) + ',' + f1(y) + ' h' + f1(w * 2) + ' l' + f1(-w * .6) + ',' + f1(w * 1.2) + ' Z" fill="' + rock + '" opacity="' + f1(.3 + r() * .5) + '"/>'; }
    return { art: '<defs>' + mp3Defs() + '</defs>' + s, slots: mpSlots(MP_P, 1.15), zone: null, wallsh: false, decor: false, floaty: true, label: 'The drifting stones',
      base: function (it, p) {
        var x = p[0], y = p[1] + 2, w = 34, k = (it.id * 7) % 5, d = 'M' + (x - w) + ',' + y + ' L' + (x - w * .7) + ',' + (y + 10 + k) + ' L' + (x - w * .3) + ',' + (y + 14 + k * 2) + ' L' + (x - 4 + k) + ',' + (y + 34 + k * 2) + ' L' + (x + w * .3) + ',' + (y + 16 + k) + ' L' + (x + w * .75) + ',' + (y + 12) + ' L' + (x + w) + ',' + y + ' Z';
        return '<path d="' + d + '" fill="' + rock + '" filter="url(#mp-stone)"/><path d="M' + (x - 4 + k) + ',' + (y + 34 + k * 2) + ' L' + (x + w * .3) + ',' + (y + 16 + k) + ' L' + (x + w * .75) + ',' + (y + 12) + ' L' + (x + w) + ',' + y + ' H' + (x + 4) + ' Z" fill="#000" opacity=".28"/>' +
          '<ellipse cx="' + x + '" cy="' + y + '" rx="' + w + '" ry="6" fill="' + top + '"/><ellipse cx="' + x + '" cy="' + (y - 1) + '" rx="' + (w - 5) + '" ry="4" fill="#fff" opacity=".18"/>' +
          '<path d="M' + (x - w + 4) + ',' + (y - 1) + ' q-2,-8 -5,-11 M' + (x - w + 9) + ',' + (y - 2) + ' q1,-7 4,-10 M' + (x + w - 6) + ',' + (y - 1) + ' q2,-7 5,-9" stroke="' + mix(top, '#2a5a2a', .5) + '" stroke-width="1.3" fill="none" stroke-linecap="round"/>' +
          '<path d="M' + (x - 8) + ',' + (y + 44 + k * 2) + ' l5,0 l-2,7 Z M' + (x + 12) + ',' + (y + 36 + k) + ' l4,0 l-2,5 Z" fill="' + rock + '" opacity=".8"/>';
      } };
  };

  /* ---- Orbits: three paths drawn across the dark with a small sun at one
     end, and the goods riding them like worlds. ---- */
  function mpThrough(pts) { var d = 'M' + pts[0][0] + ',' + pts[0][1], i; for (i = 1; i < pts.length - 1; i++) d += ' Q' + pts[i][0] + ',' + pts[i][1] + ' ' + f1((pts[i][0] + pts[i + 1][0]) / 2) + ',' + f1((pts[i][1] + pts[i + 1][1]) / 2); return d + ' L' + pts[pts.length - 1][0] + ',' + pts[pts.length - 1][1]; }
  DISPLAYS.orbits = function (t) {
    var a = t.th.accent, r = rng(23), s = '<defs>' + rad('mp-sun', [[0, '#fffbe8', 1], [.25, '#ffe8a0', .8], [.6, a, .25], [1, a, 0]]) + '</defs>', row, i, P = [[80, 196], [192, 160], [312, 172], [432, 204], [66, 300], [184, 286], [306, 292], [428, 314], [98, 408], [214, 420], [334, 414], [446, 392]];
    for (row = 0; row < 3; row++) {
      var q = P.slice(row * 4, row * 4 + 4), pts = [[-20, q[0][1] + 40]].concat(q.map(function (p) { return [p[0], p[1] + 4]; })).concat([[540, q[3][1] + 50]]), d = mpThrough(pts);
      s += '<path d="' + d + '" stroke="' + a + '" stroke-opacity=".35" stroke-width="5" fill="none" filter="url(#src-blur3)"/><path d="' + d + '" stroke="' + mix(a, '#ffffff', .6) + '" stroke-opacity=".8" stroke-width="1.2" fill="none"/><path d="' + d + '" stroke="#fff" stroke-opacity=".7" stroke-width="2.4" fill="none" stroke-dasharray="1 23" stroke-linecap="round"/>';
    }
    s += '<circle cx="258" cy="92" r="70" fill="url(#mp-sun)"/><circle cx="258" cy="92" r="13" fill="#fffbe8"/>' + mpStars(r, 60, 30, 70, 470, 390, false);
    [[150, 236, 5, '#c88a5a'], [372, 246, 7, '#8ab4d8'], [264, 352, 4, '#d8c890'], [472, 356, 5, '#b08ad8']].forEach(function (k) { s += '<circle cx="' + k[0] + '" cy="' + k[1] + '" r="' + k[2] + '" fill="' + k[3] + '"/><path d="M' + k[0] + ',' + (k[1] - k[2]) + ' a' + k[2] + ',' + k[2] + ' 0 0 1 0,' + (k[2] * 2) + ' a' + (k[2] * .5) + ',' + k[2] + ' 0 0 0 0,-' + (k[2] * 2) + ' Z" fill="#000" opacity=".45"/>'; });
    return { art: s, slots: mpSlots(P, 1.12), zone: null, wallsh: false, decor: false, floaty: true, label: 'The orbits',
      base: function (it, p) { return '<ellipse cx="' + p[0] + '" cy="' + (p[1] + 3) + '" rx="34" ry="8" fill="' + a + '" opacity=".35" filter="url(#src-blur3)"/><ellipse cx="' + p[0] + '" cy="' + (p[1] + 2) + '" rx="28" ry="6" fill="#0a0c20" stroke="' + mix(a, '#ffffff', .5) + '" stroke-width="1.4"/><ellipse cx="' + p[0] + '" cy="' + (p[1] + 2) + '" rx="19" ry="3.6" fill="none" stroke="' + a + '" stroke-opacity=".7" stroke-width=".8"/>'; } };
  };

  /* ---- Tendrils: something under the floor holds each thing up for you. ---- */
  DISPLAYS.tendrils = function (t) {
    var a = t.th.accent, r = rng(57), flesh = mix(t.th.cloth, '#1a1024', .45), s = '<defs>' + mp3Defs() + '</defs>', order = [0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11];
    order.forEach(function (i) {
      var x = MP_P[i][0], y = MP_P[i][1], k = i % 2 ? 1 : -1, H = 492 - y, raw = [[x + k * 34, 496], [x + k * 44, y + H * .72], [x - k * 14, y + H * .44], [x + k * 18, y + H * .2], [x - k * 36, y + 20], [x - k * 14, y + 11], [x + k * 18, y + 9], [x + k * 38, y + 3], [x + k * 40, y - 10], [x + k * 28, y - 15], [x + k * 22, y - 7]], pts = mpSmooth(raw, 7), n = pts.length,
        wd = function (u) { return 5 + 24 * Math.pow(1 - u, 1.3); }, d = mpTaper(pts, wd), col = mix(flesh, '#000', i < 4 ? .4 : i < 8 ? .2 : 0), sk = '', j, hi = [], lo = [];
      for (j = 2; j < n - 6; j += 3) { var u = j / (n - 1), dx = pts[j + 1][0] - pts[j - 1][0], dy = pts[j + 1][1] - pts[j - 1][1], L = Math.sqrt(dx * dx + dy * dy) || 1, w = wd(u), sx = pts[j][0] + dy / L * w * .28 * k, sy = pts[j][1] - dx / L * w * .28 * k;
        sk += '<circle cx="' + f1(sx) + '" cy="' + f1(sy) + '" r="' + f1(w * .15) + '" fill="' + mix(col, '#e8c8e0', .5) + '" opacity=".8"/><circle cx="' + f1(sx) + '" cy="' + f1(sy) + '" r="' + f1(w * .07) + '" fill="#120818"/>'; }
      for (j = 0; j < n; j++) { var u2 = j / (n - 1), a1 = pts[Math.min(n - 1, j + 1)], a0 = pts[Math.max(0, j - 1)], ex = a1[0] - a0[0], ey = a1[1] - a0[1], LL = Math.sqrt(ex * ex + ey * ey) || 1; hi.push(f1(pts[j][0] - ey / LL * wd(u2) * .3 * k) + ',' + f1(pts[j][1] + ex / LL * wd(u2) * .3 * k)); }
      s += '<path d="' + d + '" fill="#000" opacity=".5" filter="url(#src-blur5)" transform="translate(8,5)"/><path d="' + d + '" fill="' + col + '" filter="url(#mp-fine)"/><path d="M' + hi.join(' L') + '" stroke="' + mix(a, '#ffffff', .3) + '" stroke-opacity=".3" stroke-width="2.2" fill="none" stroke-linecap="round" filter="url(#src-blur3)"/>' + sk;
    });
    return { art: s, slots: mpSlots(MP_P, 1.12), zone: [34, 96, 454, 320], wallsh: false, decor: false, label: 'The tendrils',
      base: function (it, p) { return '<ellipse cx="' + p[0] + '" cy="' + (p[1] + 3) + '" rx="30" ry="6" fill="' + a + '" opacity=".22" filter="url(#src-blur3)"/>'; } };
  };

  /* ---- Circles: each thing stands over a small working of its own, and
     the workings are not all the same kind. They turn slowly. ---- */
  function mpCircle(kind, a, R) {
    var s = '<circle r="' + R + '" fill="none" stroke="' + a + '" stroke-width="3"/><circle r="' + (R - 8) + '" fill="none" stroke="' + a + '" stroke-width="1.2"/>', i, n, p = [], rr = rng(kind * 13 + 5);
    function poly(n, rad, off) { var q = []; for (var j = 0; j < n; j++) q.push(f1(Math.cos((j / n * 360 + off) * Math.PI / 180) * rad) + ',' + f1(Math.sin((j / n * 360 + off) * Math.PI / 180) * rad)); return '<path d="M' + q.join(' L') + ' Z" fill="none" stroke="' + a + '" stroke-width="2"/>'; }
    if (kind === 0) s += poly(3, R - 8, -90) + poly(3, R - 8, 90) + '<circle r="' + f1(R * .3) + '" fill="none" stroke="' + a + '" stroke-width="1.6"/>';
    else if (kind === 1) { s += poly(4, R - 8, 0) + poly(4, R - 8, 45); for (i = 0; i < 4; i++) s += '<circle cx="' + f1(Math.cos(i * Math.PI / 2 + .78) * (R - 8) * .6) + '" cy="' + f1(Math.sin(i * Math.PI / 2 + .78) * (R - 8) * .6) + '" r="5" fill="none" stroke="' + a + '" stroke-width="1.4"/>'; }
    else if (kind === 2) { s += poly(5, R - 8, -90) + '<path d="M0,' + (-(R - 8)) + ' L' + f1(Math.cos(54 * Math.PI / 180) * (R - 8)) + ',' + f1(Math.sin(54 * Math.PI / 180) * (R - 8)) + ' L' + f1(-Math.cos(18 * Math.PI / 180) * (R - 8)) + ',' + f1(-Math.sin(18 * Math.PI / 180) * (R - 8)) + ' L' + f1(Math.cos(18 * Math.PI / 180) * (R - 8)) + ',' + f1(-Math.sin(18 * Math.PI / 180) * (R - 8)) + ' L' + f1(-Math.cos(54 * Math.PI / 180) * (R - 8)) + ',' + f1(Math.sin(54 * Math.PI / 180) * (R - 8)) + ' Z" fill="none" stroke="' + a + '" stroke-width="1.6"/>'; }
    else { s += '<circle r="' + f1(R * .55) + '" fill="none" stroke="' + a + '" stroke-width="1.6" stroke-dasharray="6 5"/>'; for (i = 0; i < 6; i++) s += '<path d="M' + f1(Math.cos(i * Math.PI / 3) * R * .55) + ',' + f1(Math.sin(i * Math.PI / 3) * R * .55) + ' L' + f1(Math.cos(i * Math.PI / 3) * (R - 8)) + ',' + f1(Math.sin(i * Math.PI / 3) * (R - 8)) + '" stroke="' + a + '" stroke-width="1.6"/>'; }
    n = 14; for (i = 0; i < n; i++) s += '<g transform="rotate(' + (i * 360 / n) + ') translate(' + (-3) + ',' + (-(R - 1)) + ')"><path d="' + mpRune(rr, 0, 0, 3) + '" stroke="' + a + '" stroke-width="1" fill="none"/></g>';
    return s;
  }
  DISPLAYS.circles = function (t) {
    var a = t.th.accent, lite = mix(a, '#ffffff', .45), s = '<defs>' + lin('mp-beam', 1, [[0, a, 0], [1, a, .3]]) + '</defs>', i, row, r = rng(41);
    // lines of power run between the workings, drawn on the wall
    for (row = 0; row < 3; row++) for (i = 0; i < 3; i++) { var p = MP_P[row * 4 + i], q = MP_P[row * 4 + i + 1]; s += '<path d="M' + p[0] + ',' + (p[1] + 4) + ' L' + q[0] + ',' + (q[1] + 4) + '" stroke="' + a + '" stroke-opacity=".3" stroke-width="1.2" stroke-dasharray="3 5"/>'; }
    for (i = 0; i < 8; i++) { var p2 = MP_P[i], q2 = MP_P[i + 4]; s += '<path d="M' + p2[0] + ',' + (p2[1] + 4) + ' L' + q2[0] + ',' + (q2[1] - 60) + '" stroke="' + a + '" stroke-opacity=".16" stroke-width="1" stroke-dasharray="2 6"/>'; }
    MP_P.forEach(function (p) { s += '<rect x="' + (p[0] - 30) + '" y="' + (p[1] - 90) + '" width="60" height="94" fill="url(#mp-beam)" opacity=".5" filter="url(#src-blur5)"/>'; });
    return { art: s, slots: mpSlots(MP_P, 1.12), zone: null, wallsh: false, decor: false, label: 'The workings',
      base: function (it, p) {
        var kind = (it.id || 0) % 4;
        return '<ellipse cx="' + p[0] + '" cy="' + (p[1] + 4) + '" rx="54" ry="14" fill="' + a + '" opacity=".45" filter="url(#src-blur5)"/><g transform="translate(' + p[0] + ',' + (p[1] + 4) + ') scale(1,.3)"><g class="mp-turn' + (kind % 2 ? ' rev' : '') + '">' + mpCircle(kind, lite, 50) + '</g></g>';
      } };
  };

  /* ---- A slab of the local stone for a counter, with fire or worse in
     its cracks and, under a rune wall, a band of writing. ---- */

  /* ---- A counter with nothing under it: a slab that broke off somewhere
     and stayed where it was. ---- */

  /* ---- The void's pieces of ground, drawn as rock: a turfed top, faces
     that catch the light on one side, and a few crumbs falling away. ---- */
  function mpIsle(r, x, y, w, rock, top, lc) {
    var h = w * (.7 + r() * .5), pts = [[x - w, y], [x - w * .78, y + h * .22], [x - w * .5, y + h * .3], [x - w * .34, y + h * .62], [x - w * .08, y + h * .7], [x + w * .04, y + h], [x + w * .22, y + h * .56], [x + w * .5, y + h * .44], [x + w * .7, y + h * .16], [x + w, y]], d = 'M' + pts.map(function (p) { return f1(p[0]) + ',' + f1(p[1]); }).join(' L') + ' Z', i, s;
    s = '<path d="' + d + '" fill="' + rock + '" filter="url(#mp-stone)"/><path d="M' + f1(x + w * .04) + ',' + f1(y + h) + ' L' + f1(x + w * .22) + ',' + f1(y + h * .56) + ' L' + f1(x + w * .5) + ',' + f1(y + h * .44) + ' L' + f1(x + w * .7) + ',' + f1(y + h * .16) + ' L' + (x + w) + ',' + y + ' H' + f1(x + w * .1) + ' Z" fill="#000" opacity=".3"/>' +
      '<path d="M' + f1(x - w * .5) + ',' + f1(y + h * .3) + ' L' + f1(x - w * .3) + ',' + y + ' M' + f1(x - w * .08) + ',' + f1(y + h * .7) + ' L' + f1(x + w * .1) + ',' + f1(y + 4) + '" stroke="#000" stroke-opacity=".3" stroke-width="1.2" fill="none"/>' +
      '<ellipse cx="' + x + '" cy="' + y + '" rx="' + w + '" ry="' + f1(Math.max(4, w * .09)) + '" fill="' + top + '"/><ellipse cx="' + f1(x - w * .1) + '" cy="' + f1(y - 1) + '" rx="' + f1(w * .8) + '" ry="' + f1(Math.max(2.4, w * .05)) + '" fill="#fff" opacity=".2"/>';
    for (i = 0; i < Math.round(w / 5); i++) { var gx = x - w + r() * w * 2; s += '<path d="M' + f1(gx) + ',' + f1(y) + ' q' + f1((r() - .5) * 5) + ',' + f1(-5 - r() * 7) + ' ' + f1((r() - .5) * 9) + ',' + f1(-8 - r() * 9) + '" stroke="' + mix(top, '#2a5a2a', .3 + r() * .4) + '" stroke-width="1.1" fill="none" stroke-linecap="round"/>'; }
    for (i = 0; i < 3; i++) { var cx = x + (r() - .5) * w, cy = y + h * (1 + r() * .5), cw = 2 + r() * w * .08; s += '<path d="M' + f1(cx - cw) + ',' + f1(cy) + ' h' + f1(cw * 2) + ' l' + f1(-cw * .7) + ',' + f1(cw * 1.3) + ' Z" fill="' + rock + '" opacity=".8"/>'; }
    return s;
  }
  SHAPES['void'] = function (t) {
    var s = WALLS[t.th.wall](t) + '<defs>' + mp3Defs() + '</defs>', r = rng(27), pale = t.th.wall === 'white', rock = pale ? '#8a90a4' : mix(t.th.wood, '#7a78a0', .35), top = pale ? '#a8c890' : mix(t.th.accent, '#3a3860', .55), far = '';
    [[760, 300, 22], [580, 370, 14], [1160, 330, 24], [420, 420, 12], [900, 60, 18]].forEach(function (q) { far += mpIsle(r, q[0], q[1], q[2], mix(rock, pale ? '#ffffff' : '#0a0c24', .45), mix(top, pale ? '#ffffff' : '#0a0c24', .4), t.lc); });
    s += '<g opacity=".8" filter="url(#src-blur3)">' + far + '</g>';
    [[-10, 60, 120], [150, 10, 60], [1120, 30, 110], [990, -10, 50], [1210, 250, 60], [-30, 330, 70]].forEach(function (q) { s += mpIsle(r, q[0], q[1], q[2], rock, top, t.lc); });
    return s;
  };

  /* ---- The stranger places up close: rock, bone and cloud are
     made from noise and lit, far things sit in haze, every wall carries a
     little grain, and each place has small effects of its own. The effects
     are small things moving, never the whole room. ---- */
  CM_PROPS.push('dew', 'beads', 'rune-chase', 'arcs', 'bubbles', 'ghostlights', 'crumbs', 'dust');
  ['dew', 'beads', 'rune-chase', 'arcs', 'bubbles', 'ghostlights', 'crumbs', 'dust'].forEach(function (p) { PROP_AT[p] = { at: 'over' }; });

  // Cloud made from noise: one colour, with the noise deciding how much of it is there.
  function mpCloud(id, seed, fq, col, gain, bias, oct) {
    var c = hex(col);
    return '<filter id="' + id + '" x="0" y="0" width="1" height="1" color-interpolation-filters="sRGB"><feTurbulence type="fractalNoise" baseFrequency="' + fq + '" numOctaves="' + (oct || 5) + '" seed="' + seed + '"/><feColorMatrix type="matrix" values="0 0 0 0 ' + f1(c[0] / 255 * 100) / 100 + '  0 0 0 0 ' + f1(c[1] / 255 * 100) / 100 + '  0 0 0 0 ' + f1(c[2] / 255 * 100) / 100 + '  ' + gain + ' 0 0 0 ' + bias + '"/><feComposite in2="SourceGraphic" operator="in"/></filter>';
  }
  function mpGrain(op) { return '<filter id="mp-grain" x="0" y="0" width="1" height="1"><feTurbulence type="fractalNoise" baseFrequency="1.15" numOctaves="2" seed="8"/><feColorMatrix type="matrix" values="0 0 0 0 .5  0 0 0 0 .5  0 0 0 0 .5  1.7 0 0 0 -.6"/></filter><rect ' + FULL + ' filter="url(#mp-grain)" opacity="' + (op || .2) + '"/>'; }
  function mpCrack(r, x, y, n, a, depth) {
    var d = 'M' + f1(x) + ',' + f1(y), k, o = '';
    for (k = 0; k < n; k++) { x += (r() - .5) * 26; y += 9 + r() * 16; d += ' L' + f1(x) + ',' + f1(y); if (depth && r() < .3) o += mpCrack(r, x, y, 2 + Math.floor(r() * 3), a, 0); }
    return '<path d="' + d + '" stroke="' + a + '" stroke-opacity="' + (depth ? .4 : .25) + '" stroke-width="' + (depth ? 5 : 3) + '" fill="none" filter="url(#src-blur3)"/><path d="' + d + '" stroke="#000" stroke-opacity=".7" stroke-width="' + (depth ? 2.6 : 1.6) + '" fill="none" stroke-linejoin="round"/><path d="' + d + '" stroke="' + mix(a, '#fff4d0', .4) + '" stroke-opacity=".9" stroke-width="' + (depth ? 1 : .6) + '" fill="none" stroke-linejoin="round"/>' + o;
  }
  CM_CSS.push(
    '@keyframes mp-arc{0%,100%{opacity:0}2%{opacity:1}5%{opacity:.2}7%{opacity:.9}11%{opacity:0}}',
    '.mp-arc{opacity:0;animation:mp-arc 5s linear infinite}',
    '@keyframes mp-pop{0%{transform:scale(.2);opacity:0}30%{opacity:.95}85%{transform:scale(1);opacity:.9}100%{transform:scale(1.5);opacity:0}}',
    '.mp-pop{transform-box:fill-box;transform-origin:50% 100%;opacity:0;animation:mp-pop 3.4s ease-out infinite}',
    '@keyframes mp-chase{0%,70%,100%{opacity:.12}80%{opacity:1}}',
    '.mp-chase{opacity:.12;animation:mp-chase 6s ease-in-out infinite}',
    '@keyframes mp-drift{0%{transform:translate(0,0);opacity:0}15%{opacity:.8}85%{opacity:.6}100%{transform:translate(var(--fx,30px),var(--fy,-60px));opacity:0}}',
    '.mp-drift{opacity:0;animation:mp-drift 14s linear infinite}'
  );
  // With motion stopped the runes would sit almost dark, so they rest half lit.
  CM_CSS.push('@media (prefers-reduced-motion:reduce){.mp-chase{opacity:.6}}', 'html[data-motion="calm"] .mp-chase,html[data-motion="off"] .mp-chase{opacity:.6}');

  /* ---- Walls ---- */
  WALLS.basalt = function (t) {
    var r = rng(83), a = t.th.accent, s = '', x, w, y, i, g = '';
    // columns of cooled rock, each its own grey, each broken across at its own heights
    for (x = -300; x < 1500; x += w) {
      w = 26 + r() * 34; var c = mix('#26262c', r() < .5 ? '#0e0e12' : '#4a4850', r() * .55);
      g += '<rect x="' + f1(x) + '" y="-150" width="' + f1(w) + '" height="620" fill="' + c + '"/><rect x="' + f1(x) + '" y="-150" width="2.4" height="620" fill="#000" opacity=".7"/><rect x="' + f1(x + 2.4) + '" y="-150" width="1.6" height="620" fill="#9a94a0" opacity=".12"/>';
      for (y = -150 + r() * 140; y < 470; y += 60 + r() * 170) g += '<path d="M' + f1(x) + ',' + f1(y) + ' l' + f1(w * .4) + ',' + f1(r() * 5 - 2) + ' l' + f1(w * .6) + ',' + f1(r() * 5 - 3) + '" stroke="#050404" stroke-width="2.6" fill="none"/><path d="M' + f1(x) + ',' + f1(y + 3) + ' l' + f1(w) + ',1" stroke="#b0a8b8" stroke-opacity=".1" stroke-width="1.2"/>';
    }
    s = '<defs>' + mp3Defs() + mpCloud('mp-soot', 4, '.006 .012', '#000000', 2.2, -.9) + lin('mp-heat', 1, [[0, a, 0], [.6, a, .14], [1, '#ffd8a0', .42]]) + '</defs><g filter="url(#cm-wall)"><g filter="url(#mp-stone)">' + g + '</g></g>';
    s += '<rect ' + FULL + ' fill="#000" filter="url(#mp-soot)" opacity=".7"/>';
    for (i = 0; i < 7; i++) s += mpCrack(r, -200 + r() * 1600, 120 + r() * 220, 5 + Math.floor(r() * 6), a, 1);
    return s + '<rect x="-300" y="230" width="1800" height="240" fill="url(#mp-heat)"/>' + mpGrain(.22);
  };
  WALLS.nebula = function (t) {
    var r = rng(19), a = t.th.accent, s, i, x, y, k;
    s = '<defs>' + lin('mp-deep', 1, [[0, '#05040f'], [.5, '#0b0a24'], [1, '#080616']]) + mpCloud('mp-nb1', 3, '.0028 .004', '#8a2a9a', 2.6, -1.05) + mpCloud('mp-nb2', 11, '.0034 .0026', '#1a6a9a', 2.4, -1) + mpCloud('mp-nb3', 27, '.005', a, 2.2, -1.15) + mpCloud('mp-nb4', 41, '.007 .004', '#d86a5a', 2.6, -1.45) + mpCloud('mp-dust', 8, '.006 .0035', '#020108', 3, -1.3) +
      rad('mp-starg', [[0, '#fff', 1], [.2, '#fff', .5], [1, '#fff', 0]]) + '</defs><rect ' + FULL + ' fill="url(#mp-deep)"/>';
    ['mp-nb1', 'mp-nb2', 'mp-nb3', 'mp-nb4'].forEach(function (id, j) { s += '<rect ' + FULL + ' fill="#000" filter="url(#' + id + ')" opacity="' + [.75, .7, .5, .55][j] + '"/>'; });
    // a band of far stars across it, thick as dust
    for (i = 0; i < 900; i++) { k = r(); x = -300 + k * 1800; y = 330 - k * 380 + (r() + r() + r() - 1.5) * 110; s += '<circle cx="' + f1(x) + '" cy="' + f1(y) + '" r="' + f1(.3 + r() * .7) + '" fill="#fff" opacity="' + f1(.15 + r() * .5) + '"/>'; }
    s += '<rect ' + FULL + ' fill="#000" filter="url(#mp-dust)" opacity=".85"/>' + mpStars(r, 320, -290, -140, 1780, 600, false);
    // a few near stars, bright enough to flare
    for (i = 0; i < 9; i++) { x = -200 + r() * 1600; y = -100 + r() * 480; k = 5 + r() * 9; var c = ['#fff', '#cfe0ff', '#ffe8c8'][i % 3]; s += '<circle cx="' + f1(x) + '" cy="' + f1(y) + '" r="' + f1(k * 2.4) + '" fill="url(#mp-starg)" opacity=".5"/><path d="M' + f1(x - k * 3) + ',' + f1(y) + ' H' + f1(x + k * 3) + ' M' + f1(x) + ',' + f1(y - k * 3) + ' V' + f1(y + k * 3) + '" stroke="' + c + '" stroke-opacity=".55" stroke-width=".8"/><circle cx="' + f1(x) + '" cy="' + f1(y) + '" r="' + f1(k * .22) + '" fill="' + c + '"/>'; }
    return s + mpGrain(.14);
  };
  WALLS.white = function (t) {
    var r = rng(29), s, i;
    s = '<defs>' + lin('mp-wh', 1, [[0, '#dfe6f2'], [.4, '#f4f6fb'], [.63, '#ffffff'], [.68, '#d6dcea'], [1, '#eef1f7']]) + mpCloud('mp-wc1', 5, '.0035 .009', '#ffffff', 2.6, -1) + mpCloud('mp-wc2', 17, '.005 .014', '#b8c2d8', 2.4, -1.15) + '</defs><rect ' + FULL + ' fill="url(#mp-wh)"/>';
    s += '<rect ' + FULL + ' fill="#000" filter="url(#mp-wc2)" opacity=".6"/><rect ' + FULL + ' fill="#000" filter="url(#mp-wc1)" opacity=".9"/>';
    // a pale sun with no edge, somewhere behind all of it
    s += '<defs>' + rad('mp-wsun', [[0, '#fffdf0', .95], [.3, '#fff8e0', .5], [1, '#fff8e0', 0]]) + '</defs><ellipse cx="700" cy="150" rx="420" ry="260" fill="url(#mp-wsun)"/>';
    return s + mpGrain(.12);
  };
  WALLS.obsidian = function (t) {
    var r = rng(37), a = t.th.accent, g = '', s, i, x, y;
    for (i = 0; i < 90; i++) { x = -300 + r() * 1800; y = -150 + r() * 620; g += '<path d="' + mpLump(r, x, y, 50 + r() * 130, 5 + Math.floor(r() * 3)) + '" fill="' + mix('#151020', r() < .62 ? '#040308' : '#4a3c66', r() * .55) + '"/>'; }
    s = '<defs>' + mp3Defs() + mpCloud('mp-ofog', 6, '.004 .008', mix(a, '#102018', .5), 2.2, -1.1) + '</defs><rect ' + FULL + ' fill="#120e1c"/><g filter="url(#cm-wall)"><g filter="url(#mp-stone)">' + g + '</g></g>';
    // glass-sharp edges where a facet turns to the light, long and thin
    for (i = 0; i < 46; i++) { x = -260 + r() * 1700; y = -120 + r() * 560; var L = 20 + r() * 110, an = (r() - .5) * 1.4; s += '<path d="M' + f1(x) + ',' + f1(y) + ' l' + f1(Math.cos(an) * L) + ',' + f1(Math.sin(an) * L) + '" stroke="#efe6ff" stroke-opacity="' + f1(.08 + r() * .22) + '" stroke-width="' + f1(.5 + r() * 1.1) + '" stroke-linecap="round"/>'; }
    for (y = 70; y < 440; y += 124) for (x = 540; x < 1180; x += 30) if (r() < .8) s += '<path d="' + mpRune(r, x, y, 13) + '" stroke="#000" stroke-opacity=".7" stroke-width="3.2" fill="none" stroke-linecap="round"/><path d="' + mpRune(rng(Math.round(x * 7 + y)), x, y, 13) + '" stroke="' + mix(a, '#000', .3) + '" stroke-opacity="0" stroke-width="1.2" fill="none"/>';
    return s + '<rect ' + FULL + ' fill="#000" filter="url(#mp-ofog)" opacity=".55"/>' + mpGrain(.22);
  };
  (function () { var W = WALLS.runestone, G = WALLS.glade; WALLS.runestone = function (t) { return W(t) + mpGrain(.2); }; WALLS.glade = function (t) { return G(t) + mpGrain(.14); }; })();

  /* ---- The roof of ribs, as bone: pitted, yellowed at the joints, lit along the top edge ---- */
  SHAPES.ribs = function (t) {
    var s = WALLS[t.th.wall](t), i, k, pts, u, d, r = rng(9);
    s += '<defs>' + mp3Defs() + mpMat('mp-bonem', '.09 .35', 4, 14, 3.2, 215, 1.25) + lin('mp-bone', 1, [[0, '#8a7c66'], [.25, '#d6c8ac'], [.6, '#a8997c'], [1, '#3a3028']]) + '</defs>';
    s += '<path d="M-300,-150 H1500 V10 Q600,70 -300,10 Z" fill="#08060c"/>';
    for (i = 4; i >= 0; i--) [-1, 1].forEach(function (sd) {
      pts = []; var x0 = sd < 0 ? -40 + i * 28 : 1240 - i * 28, tx = 600 + sd * (60 + i * 104), top = [], wd = function (u) { return (i === 0 ? 46 : 26) * (1 - u * .55); };
      for (k = 0; k <= 22; k++) { u = k / 22; pts.push([x0 + (tx - x0) * Math.pow(u, 2.2), 480 - 470 * Math.sin(u * Math.PI / 2)]); }
      if (i > 1) { pts = pts.slice(12); wd = function (u) { return 22 * (1 - u * .5); }; }
      d = mpTaper(pts, wd);
      for (k = 0; k < pts.length; k++) top.push(f1(pts[k][0] + sd * wd(k / (pts.length - 1)) * .3) + ',' + f1(pts[k][1] - wd(k / (pts.length - 1)) * .12));
      s += '<path d="' + d + '" fill="#000" opacity=".55" filter="url(#src-blur8)" transform="translate(' + (-sd * 10) + ',8)"/><path d="' + d + '" fill="url(#mp-bone)" filter="url(#mp-bonem)"/><path d="M' + top.join(' L') + '" stroke="#fff6e0" stroke-opacity=".28" stroke-width="3" fill="none" filter="url(#src-blur3)"/>';
      for (k = 2; k < pts.length - 2; k += 3) if (r() < .5) s += '<path d="M' + f1(pts[k][0] - 5) + ',' + f1(pts[k][1]) + ' l' + f1(4 + r() * 8) + ',' + f1(r() * 8 - 4) + ' l' + f1(r() * 6) + ',' + f1(r() * 8 - 2) + '" stroke="#1a120c" stroke-opacity=".6" stroke-width="1" fill="none"/>';
    });
    for (i = 0; i < 9; i++) { var vx = 200 + i * 100; s += '<ellipse cx="' + vx + '" cy="20" rx="44" ry="12" fill="#000" opacity=".6" filter="url(#src-blur5)"/><g filter="url(#mp-bonem)"><path d="M' + (vx - 40) + ',6 q4,-22 40,-22 q36,0 40,22 q-8,20 -40,20 q-32,0 -40,-20 Z" fill="url(#mp-bone)"/><path d="M' + (vx - 12) + ',-14 l12,-22 l12,22 Z" fill="#b8a888"/></g><ellipse cx="' + vx + '" cy="22" rx="30" ry="5" fill="#000" opacity=".45"/><ellipse cx="' + (vx + 50) + '" cy="8" rx="9" ry="14" fill="#2a2018" opacity=".8"/>'; }
    return s;
  };

  /* ---- Openings ---- */
  OPENINGS.planet = function (t) {
    var r = rng(11), a = t.th.accent, s;
    s = '<defs>' + lin('mp-sp', 1, [[0, '#04030f'], [1, '#0d0a26']]) + mpCloud('mp-pnb', 31, '.012 .009', '#6a2a9a', 2.6, -1.1) + rad('mp-pl', [[0, mix(a, '#ffffff', .55)], [.55, a], [1, mix(a, '#05040f', .75)]], ' cx=".34" cy=".3" r=".85"') +
      '<filter id="mp-bands" x="0" y="0" width="1" height="1"><feTurbulence type="fractalNoise" baseFrequency=".006 .11" numOctaves="4" seed="6"/><feColorMatrix type="matrix" values="0 0 0 0 .96  0 0 0 0 .9  0 0 0 0 .78  2.4 0 0 0 -1"/><feComposite in2="SourceGraphic" operator="in"/></filter>' +
      '<filter id="mp-bandd" x="0" y="0" width="1" height="1"><feTurbulence type="fractalNoise" baseFrequency=".008 .16" numOctaves="4" seed="19"/><feColorMatrix type="matrix" values="0 0 0 0 .05  0 0 0 0 .04  0 0 0 0 .14  2.6 0 0 0 -1.15"/><feComposite in2="SourceGraphic" operator="in"/></filter>' +
      rad('mp-term', [[0, '#000', 0], [.55, '#000', 0], [.82, '#02010a', .7], [1, '#02010a', .96]], ' cx=".3" cy=".28" r=".95"') + lin('mp-ring', 0, [[0, '#f4e8c8', 0], [.08, '#f4e8c8', .8], [.3, '#c8b890', .5], [.38, '#000', 0], [.44, '#e8d8b0', .85], [.7, '#b8a880', .6], [.92, '#f4e8c8', .8], [1, '#f4e8c8', 0]]) + '<clipPath id="mp-plc"><circle cx="930" cy="210" r="54"/></clipPath></defs>';
    s += '<rect x="800" y="78" width="224" height="224" fill="url(#mp-sp)"/><rect x="800" y="78" width="224" height="224" fill="#000" filter="url(#mp-pnb)" opacity=".8"/>' + mpStars(r, 110, 804, 82, 216, 216, true);
    // a ringed world: cloud bands, a night side, a thin line of air at its edge, and the ring's shadow across it
    s += '<g transform="rotate(-18 930 210)"><ellipse cx="930" cy="210" rx="100" ry="21" fill="none" stroke="url(#mp-ring)" stroke-width="16" opacity=".55"/>' +
      '<circle cx="930" cy="210" r="57" fill="' + a + '" opacity=".25" filter="url(#src-blur5)"/><circle cx="930" cy="210" r="54" fill="url(#mp-pl)"/><g clip-path="url(#mp-plc)"><rect x="870" y="150" width="120" height="120" fill="#000" filter="url(#mp-bands)" opacity=".55"/><rect x="870" y="150" width="120" height="120" fill="#000" filter="url(#mp-bandd)" opacity=".6"/>' +
      '<ellipse cx="946" cy="226" rx="13" ry="6" fill="#fff0d8" opacity=".35"/><path d="M872,214 q58,22 116,0 l0,9 q-58,22 -116,0 Z" fill="#02010a" opacity=".5"/></g>' +
      '<circle cx="930" cy="210" r="54" fill="url(#mp-term)"/><path d="M885,180 A54,54 0 0 1 950,160" stroke="#fff" stroke-opacity=".55" stroke-width="1.6" fill="none"/>' +
      '<path d="M830,210 a100,21 0 0 0 200,0" fill="none" stroke="url(#mp-ring)" stroke-width="16"/><path d="M836,214 a94,17 0 0 0 188,0" fill="none" stroke="#000" stroke-opacity=".25" stroke-width="1"/></g>';
    s += '<g class="mp-spin" style="transform-origin:930px 210px;animation-duration:26s"><circle cx="1040" cy="150" r="9" fill="#cfc6b6"/><circle cx="1043" cy="153" r="9" fill="#02010a" opacity=".6"/><circle cx="1036" cy="146" r="2.2" fill="#8a8272"/></g>';
    return mpOpen(RING, s, '<path d="' + RING + '" fill="none" stroke="#000" stroke-width="22" opacity=".6"/><path d="' + RING + '" fill="none" stroke="url(#cm-trim)" stroke-width="16"/><path d="' + RING + '" fill="none" stroke="#000" stroke-opacity=".5" stroke-width="2"/><path d="M826,128 A108,108 0 0 1 900,84" stroke="#fff" stroke-opacity=".3" stroke-width="4" fill="none" filter="url(#src-blur3)"/>' +
      '<path d="M836,150 Q900,110 1000,150" stroke="#fff" stroke-opacity=".07" stroke-width="40" fill="none" clip-path="url(#cm-oclip)"/>' + [0, 60, 120, 180, 240, 300].map(function (g) { return '<circle cx="912" cy="82" r="3.4" fill="url(#cm-trimv)" transform="rotate(' + g + ' 912 190)"/>'; }).join(''));
  };
  OPENINGS.rift = function (t) {
    var d = 'M912,70 L924,112 L940,150 L926,176 L950,214 L972,250 L948,280 L966,330 L986,380 L958,418 L930,452 L912,424 L900,400 L876,410 L864,356 L852,300 L880,270 L862,228 L846,190 L890,160 L900,112 Z', a = t.th.accent, r = rng(17), i, s;
    s = '<defs>' + rad('mp-rf', [[0, '#ffffff', .95], [.16, a, .8], [.5, '#5a2a8a', .45], [1, '#06020c', 0]]) + mpCloud('mp-rfc', 12, '.012 .02', a, 2.8, -1.2) + mpCloud('mp-rfp', 33, '.016 .01', '#8a3ac8', 2.8, -1.15) + '</defs><rect x="800" y="60" width="230" height="400" fill="#04010a"/>';
    // behind the tear: depth, slow weather in two colours, and far points of light that are not stars
    s += '<g class="mp-spin" style="transform-origin:912px 262px;animation-duration:70s"><rect x="662" y="12" width="500" height="500" fill="#000" filter="url(#mp-rfp)" opacity=".8"/></g><g class="mp-spin rev" style="transform-origin:912px 262px;animation-duration:46s"><rect x="662" y="12" width="500" height="500" fill="#000" filter="url(#mp-rfc)" opacity=".7"/></g>';
    s += mpStars(r, 50, 840, 80, 150, 370, true) + '<circle cx="912" cy="262" r="64" fill="url(#mp-rf)" class="mp-breathe"/><ellipse cx="912" cy="262" rx="9" ry="30" fill="#04010a"/><ellipse cx="912" cy="262" rx="3" ry="26" fill="' + a + '" opacity=".8"/>';
    for (i = 0; i < 3; i++) s += '<path d="M' + (870 + i * 34) + ',470 q' + f1(r() * 30 - 15) + ',-70 ' + f1(r() * 20 - 10) + ',-' + f1(110 + r() * 60) + '" stroke="#06020a" stroke-width="' + (11 - i * 2) + '" fill="none" stroke-linecap="round" class="mp-curl" style="transform-origin:' + (870 + i * 34) + 'px 470px;animation-delay:-' + (i * 1.3) + 's"/>';
    // the wall is thick where it tore: a dark broken edge, lit from behind on its inner lip
    var edge = '<defs>' + mp3Defs() + '</defs><path d="' + d + '" fill="none" stroke="#0a0810" stroke-width="20" stroke-linejoin="bevel" filter="url(#mp-stone)"/><path d="' + d + '" fill="none" stroke="#000" stroke-opacity=".8" stroke-width="5" stroke-linejoin="bevel"/><path d="' + d + '" fill="none" stroke="' + a + '" stroke-width="7" opacity=".35" filter="url(#src-blur5)" class="mp-breathe"/><path d="' + d + '" fill="none" stroke="' + mix(a, '#ffffff', .6) + '" stroke-opacity=".8" stroke-width="1.1" stroke-linejoin="bevel"/>';
    [[940, 150, 1], [972, 250, 1], [846, 190, -1], [852, 300, -1], [986, 380, 1], [900, 112, -1]].forEach(function (q) { var x = q[0], y = q[1], k, dd = 'M' + x + ',' + y; for (k = 0; k < 5; k++) { x += q[2] * (8 + r() * 16); y += (r() - .4) * 22; dd += ' L' + f1(x) + ',' + f1(y); } edge += '<path d="' + dd + '" stroke="#000" stroke-opacity=".8" stroke-width="2.2" fill="none"/><path d="' + dd + '" stroke="' + a + '" stroke-opacity=".5" stroke-width=".7" fill="none"/>'; });
    return mpOpen(d, s, edge, '<ellipse cx="912" cy="262" rx="230" ry="240" fill="url(#cm-acc)" opacity=".22" pointer-events="none"/>');
  };
  OPENINGS['lava-fall'] = function (t) {
    var d = 'M818,452 L836,300 L850,214 Q880,120 912,96 Q946,120 974,214 L990,300 L1006,452 Z', r = rng(7), i, s;
    s = '<defs>' + lin('mp-lava', 0, [[0, '#4a0c02'], [.18, '#c83a08'], [.38, '#ff9a2a'], [.5, '#fff0b0'], [.62, '#ff9a2a'], [.82, '#c83a08'], [1, '#4a0c02']]) + mpCloud('mp-crust', 9, '.05 .006', '#2a0802', 3, -1.35) + mpCloud('mp-hot', 21, '.07 .009', '#fff6c8', 3, -1.7) + mp3Defs() + '</defs><rect x="800" y="60" width="230" height="400" fill="#120402"/>';
    // the fall: bright down the middle, a dark skin of cooling rock riding on it, and both of them moving
    s += '<path d="M872,60 Q862,250 852,452 H972 Q962,250 952,60 Z" fill="url(#mp-lava)"/><g clip-path="none"><rect x="846" y="60" width="132" height="400" fill="#000" filter="url(#mp-crust)" opacity=".85"/><rect x="846" y="60" width="132" height="400" fill="#000" filter="url(#mp-hot)" opacity=".8"/></g>';
    for (i = 0; i < 7; i++) s += '<path d="M' + f1(868 + i * 14 + r() * 6) + ',60 Q' + f1(864 + i * 15) + ',260 ' + f1(858 + i * 17) + ',452" stroke="' + (i % 2 ? '#fff6c8' : '#7a1a04') + '" stroke-opacity="' + (i % 2 ? .55 : .6) + '" stroke-width="' + f1(1.4 + r() * 2.6) + '" stroke-dasharray="' + f1(14 + r() * 40) + ' ' + f1(20 + r() * 44) + '" fill="none" class="mp-flow' + (i % 2 ? '' : ' slow') + '"/>';
    s += '<path d="M800,60 H872 Q862,250 852,452 H800 Z M1030,60 H952 Q962,250 972,452 H1030 Z" fill="#1a1412" filter="url(#mp-stone)"/><path d="M872,60 Q862,250 852,452" stroke="#ff9a3a" stroke-opacity=".7" stroke-width="2" fill="none" filter="url(#src-blur3)"/><path d="M952,60 Q962,250 972,452" stroke="#ff9a3a" stroke-opacity=".7" stroke-width="2" fill="none" filter="url(#src-blur3)"/>';
    s += '<g class="cm-pulse"><ellipse cx="912" cy="456" rx="110" ry="38" fill="#ffd27a" opacity=".85" filter="url(#src-blur8)"/></g>';
    for (i = 0; i < 10; i++) s += '<circle class="cm-spark" cx="' + f1(860 + r() * 104) + '" cy="440" r="' + f1(1.2 + r() * 1.6) + '" fill="#ffd27a" style="--dx:' + f1((r() - .5) * 60) + 'px;animation-delay:-' + f1(r() * 3.4) + 's"/>';
    return mpOpen(d, s, '<path d="' + d + '" fill="none" stroke="#0e0a0a" stroke-width="22" stroke-linejoin="round" filter="url(#mp-stone)"/><path d="' + d + '" fill="none" stroke="#000" stroke-opacity=".7" stroke-width="4" stroke-linejoin="round"/><path d="' + d + '" fill="none" stroke="#ff8a3a" stroke-opacity=".45" stroke-width="1.6" stroke-linejoin="round"/>',
      '<ellipse cx="912" cy="440" rx="220" ry="150" fill="url(#cm-glow)" opacity=".5" pointer-events="none" class="mp-breathe"/>');
  };

  /* ---- Small effects. Each is a handful of small things; none repaints the room. ---- */
  // Dew and wet moss catching the light.
  PROPS.dew = function (t, p) { var s = '', r = rng(211), i; for (i = 0; i < 46; i++) { var x = r() < .5 ? 20 + r() * 480 : r() * 1200, y = r() < .5 ? 500 + r() * 60 : 120 + r() * 340; s += '<g class="mp-twinkle" style="animation-delay:-' + f1(r() * 4) + 's;animation-duration:' + f1(2.4 + r() * 3.4) + 's"><circle cx="' + f1(x) + '" cy="' + f1(y) + '" r="' + f1(.8 + r() * 1.2) + '" fill="#f4fff0"/><path d="M' + f1(x - 4) + ',' + f1(y) + ' h8 M' + f1(x) + ',' + f1(y - 4) + ' v8" stroke="#f4fff0" stroke-width=".5" opacity=".7"/></g>'; } return '<g class="src-nohit">' + s + '</g>'; };
  // Dust turning over in the shafts of light.
  PROPS.dust = function (t, p) { var s = '', r = rng(223), i; for (i = 0; i < 40; i++) s += '<circle class="mp-drift" cx="' + f1(480 + r() * 640) + '" cy="' + f1(60 + r() * 360) + '" r="' + f1(.7 + r() * 1.3) + '" fill="#fffbe0" style="--fx:' + f1(-50 + r() * 30) + 'px;--fy:' + f1(20 + r() * 50) + 'px;animation-duration:' + f1(10 + r() * 12) + 's;animation-delay:-' + f1(r() * 20) + 's"/>'; return '<g class="src-nohit">' + s + '</g>'; };
  // Beads of light running the orbits.
  PROPS.beads = function (t, p) {
    var a = t.th.accent, s = '', P = [[80, 196], [192, 160], [312, 172], [432, 204], [66, 300], [184, 286], [306, 292], [428, 314], [98, 408], [214, 420], [334, 414], [446, 392]], row, j;
    for (row = 0; row < 3; row++) { var q = P.slice(row * 4, row * 4 + 4), d = mpThrough([[-20, q[0][1] + 40]].concat(q.map(function (pp) { return [pp[0], pp[1] + 4]; })).concat([[540, q[3][1] + 50]]));
      for (j = 0; j < 3; j++) s += '<g><circle r="7" fill="' + a + '" opacity=".35"/><circle r="2.4" fill="#fff"/><animateMotion dur="' + (16 + row * 5) + 's" begin="-' + (j * (16 + row * 5) / 3).toFixed(1) + 's" repeatCount="indefinite" path="' + d + '"/></g>'; }
    return '<g class="src-nohit">' + cmM(t, s) + '</g>';
  };
  // The carving lights a letter at a time, along the band and round again.
  PROPS['rune-chase'] = function (t, p) { var s = '', r = rng(53), a = t.th.accent, x, i = 0; [58, 338].forEach(function (y) { for (x = -280; x < 1500; x += 34) { var d = mpRune(r, x, y, 15); if (x > 500 && x < 1210) s += '<path class="mp-chase" d="' + d + '" stroke="#fff" stroke-width="2.2" fill="none" stroke-linecap="round" style="animation-delay:' + f1(i++ * .28 % 6) + 's"/>'; } }); return '<g class="src-nohit" style="filter:drop-shadow(0 0 3px ' + a + ')">' + s + '</g>'; };
  // Light jumps across the tear and out along its cracks.
  PROPS.arcs = function (t, p) { var s = '', r = rng(241), a = t.th.accent, i, k; for (i = 0; i < 5; i++) { var x = 880 + r() * 70, y = 110 + r() * 300, d = 'M' + f1(x) + ',' + f1(y); for (k = 0; k < 6; k++) { x += (r() - .5) * 70; y += (r() - .5) * 60; d += ' L' + f1(x) + ',' + f1(y); } s += '<g class="mp-arc" style="animation-delay:-' + f1(r() * 5) + 's;animation-duration:' + f1(4 + r() * 5) + 's"><path d="' + d + '" stroke="' + a + '" stroke-width="5" stroke-opacity=".4" fill="none"/><path d="' + d + '" stroke="#fff" stroke-width="1.2" fill="none" stroke-linejoin="round"/></g>'; } return '<g class="src-nohit">' + s + '</g>'; };
  // Gas breaks the skin of the pool at the foot of the fall.
  PROPS.bubbles = function (t, p) { var s = '', r = rng(251), i; for (i = 0; i < 9; i++) { var x = 850 + r() * 124, w = 5 + r() * 9; s += '<g class="mp-pop" style="animation-delay:-' + f1(r() * 3.4) + 's;animation-duration:' + f1(2.6 + r() * 2.4) + 's"><path d="M' + f1(x - w) + ',452 a' + f1(w) + ',' + f1(w * .9) + ' 0 0 1 ' + f1(w * 2) + ',0 Z" fill="#ffb040"/><path d="M' + f1(x - w * .6) + ',' + f1(452 - w * .45) + ' a' + f1(w * .6) + ',' + f1(w * .5) + ' 0 0 1 ' + f1(w * .9) + ',-' + f1(w * .2) + '" stroke="#fff6c8" stroke-width="1.2" fill="none"/></g>'; } return '<g class="src-nohit">' + s + '</g>'; };
  // Pale lights rise out of the floor's cracks and go out.
  PROPS.ghostlights = function (t, p) { var s = '', r = rng(261), a = t.th.accent, i; for (i = 0; i < 18; i++) s += '<g class="mp-rise" style="--fx:' + f1((r() - .5) * 50) + 'px;--up:' + f1(-120 - r() * 220) + 'px;animation-duration:' + f1(6 + r() * 7) + 's;animation-delay:-' + f1(r() * 12) + 's"><circle cx="' + f1(20 + r() * 1160) + '" cy="' + f1(450 + r() * 30) + '" r="' + f1(3 + r() * 4) + '" fill="' + a + '" opacity=".3"/><circle cx="0" cy="0" r="0"/></g>'; return '<g class="src-nohit">' + s + '</g>'; };
  // Small pieces let go of the edges and drift upward.
  PROPS.crumbs = function (t, p) { var s = '', r = rng(271), pale = t.th.wall === 'white', c = pale ? '#8a90a4' : '#8a88b0', i; for (i = 0; i < 26; i++) { var w = 1.6 + r() * 3.6; s += '<g transform="translate(' + f1(r() * 1200) + ',' + f1(520 + r() * 170) + ')"><path class="mp-drift" d="M0,0 h' + f1(w * 2) + ' l' + f1(-w * .7) + ',' + f1(w * 1.2) + ' Z" fill="' + c + '" style="--fx:' + f1((r() - .5) * 40) + 'px;--fy:' + f1(-60 - r() * 120) + 'px;animation-duration:' + f1(12 + r() * 14) + 's;animation-delay:-' + f1(r() * 24) + 's"/></g>'; } return '<g class="src-nohit">' + s + '</g>'; };

  /* ---- Cave rock with body: lit from below, dark at the roof, never one flat colour ---- */
  (function () {
    var C = SHAPES.cavern;
    SHAPES.cavern = function (t) {
      var defs = '<defs>' + mp3Defs() + lin('mp-cavr', 1, [[0, '#070606'], [.55, '#1c1816'], [1, mix('#3a322c', t.lc, .25)]]) + '</defs>';
      return defs + C(t).split('fill="#191614"').join('fill="url(#mp-cavr)" filter="url(#mp-stone)"').split('fill="#2a2622"').join('fill="url(#mp-cavr)" filter="url(#mp-stone)"');
    };
  })();

  /* ---- The stone counter: hairline cracks, chipped edge, light only in the deep ones ---- */
  COUNTERS.slab = function (t) {
    var r = rng(47), a = t.th.accent, st = mix(t.th.wood, '#1a181c', .55), s, i, x, g = '', runic = t.th.wall === 'runestone', hot = t.th.wall === 'basalt';
    s = '<defs>' + mp3Defs() + lin('mp-slabt', 1, [[0, mix(st, '#8a8490', .45)], [1, mix(st, '#000', .2)]]) + lin('mp-slabf', 1, [[0, mix(st, '#000', .1)], [1, '#050405']]) + lin('mp-foot', 1, [[0, '#000', 0], [1, '#000', .6]]) + lin('mp-upl', 1, [[0, a, 0], [1, a, .3]]) + '</defs><clipPath id="cm-topclip"><rect x="-300" y="452" width="1800" height="58"/></clipPath>';
    s += '<g filter="url(#mp-stone)"><rect x="-300" y="452" width="1800" height="58" fill="url(#mp-slabt)"/><rect x="-300" y="510" width="1800" height="390" fill="url(#mp-slabf)"/></g><rect x="-300" y="506" width="1800" height="3" fill="' + t.lc + '" opacity=".4"/><rect x="-300" y="510" width="1800" height="22" fill="#000" opacity=".5" filter="url(#src-blur5)"/>';
    // the blocks it is built from: a dark joint with a worn, lighter lip beside it
    for (x = -300; x < 1500; x += 150 + r() * 120) { var lean = (r() - .5) * 14; s += '<path d="M' + f1(x) + ',512 l' + f1(lean) + ',190" stroke="#000" stroke-opacity=".75" stroke-width="3"/><path d="M' + f1(x + 2.4) + ',512 l' + f1(lean) + ',190" stroke="#b8b0c0" stroke-opacity=".12" stroke-width="1.4"/>'; }
    s += '<path d="M-300,596 H1500" stroke="#000" stroke-opacity=".6" stroke-width="2.4"/><path d="M-300,598.4 H1500" stroke="#b8b0c0" stroke-opacity=".1" stroke-width="1.2"/>';
    // chips out of the front edge
    for (i = 0; i < 26; i++) { x = -280 + r() * 1760; var cw = 4 + r() * 12; s += '<path d="M' + f1(x) + ',509 l' + f1(cw * .4) + ',' + f1(3 + r() * 5) + ' l' + f1(cw * .6) + ',' + f1(-3 - r() * 5) + ' Z" fill="#000" opacity=".7"/>'; }
    function hair(x, y, n, deep) { var d = 'M' + f1(x) + ',' + f1(y), k, o = '', dir = (r() - .5) * 8; for (k = 0; k < n; k++) { x += dir + (r() - .5) * 9; y += 5 + r() * 9; d += ' L' + f1(x) + ',' + f1(y); if (deep && r() < .22) o += hair(x, y, 3 + Math.floor(r() * 4), 0); }
      return (deep ? '<path d="' + d + '" stroke="' + a + '" stroke-opacity=".3" stroke-width="5" fill="none" filter="url(#src-blur3)"/>' : '') + '<path d="' + d + '" stroke="#000" stroke-opacity=".8" stroke-width="' + (deep ? 1.8 : 1) + '" fill="none" stroke-linejoin="round"/>' + (deep ? '<path d="' + d + '" stroke="' + mix(a, '#fff4d0', .3) + '" stroke-opacity=".7" stroke-width=".5" fill="none" stroke-linejoin="round"/>' : '<path d="' + d + '" stroke="#c8c0d0" stroke-opacity=".12" stroke-width=".6" fill="none" transform="translate(1,0)"/>') + o; }
    if (!runic) for (i = 0; i < 14; i++) g += hair(-260 + r() * 1720, 512 + r() * 60, 5 + Math.floor(r() * 9), hot ? r() < .45 : r() < .2);
    if (runic || t.th.wall === 'obsidian') { g += '<rect x="-300" y="536" width="1800" height="34" fill="#000" opacity=".4"/>'; for (x = -280; x < 1500; x += 26) g += '<path d="' + mpRune(r, x, 542, 9) + '" stroke="' + a + '" stroke-opacity=".85" stroke-width="1.8" fill="none" stroke-linecap="round"/>'; g += '<rect x="-300" y="536" width="1800" height="34" fill="' + a + '" opacity=".12" filter="url(#src-blur5)"/>'; }
    return s + g + '<rect x="-300" y="560" width="1800" height="160" fill="url(#mp-foot)"/>' + (hot ? '<rect x="-300" y="600" width="1800" height="110" fill="url(#mp-upl)"/>' : '');
  };

  /* ---- The counter with nothing under it: its underside is broken rock in
     two depths, each lump with a lit face and a dark one. ---- */
  COUNTERS.adrift = function (t) {
    var pale = t.th.wall === 'white', r = rng(63), rock = pale ? '#8a90a4' : mix(t.th.wood, '#7a78a0', .45), sky0 = pale ? '#eef1f6' : '#0a0c24', sky1 = pale ? '#c4ccdc' : '#1c1240', s = cmTop(t, 'M-300,452 H1500 V510 H-300 Z'), x, i, g, hz = pale ? '#e4e8f0' : '#0e0c2a';
    s = '<g filter="url(#cm-woodlit)">' + s + '<clipPath id="cm-topclip"><rect x="-300" y="452" width="1800" height="58"/></clipPath><rect x="-300" y="504" width="1800" height="8" fill="#000" filter="url(#cm-beam)"/><rect x="-300" y="504" width="1800" height="2" fill="' + t.lc + '" opacity=".4"/></g>';
    g = '<defs>' + mp3Defs() + lin('mp-under', 1, [[0, sky0], [1, sky1]]) + mpCloud('mp-uc', 23, '.004 .012', pale ? '#ffffff' : t.th.accent, 2.4, pale ? -.9 : -1.3) + '</defs><rect x="-300" y="512" width="1800" height="400" fill="url(#mp-under)"/>';
    if (!pale) g += mpStars(r, 120, -300, 520, 1800, 190, false);
    g += '<rect x="-300" y="512" width="1800" height="200" fill="#000" filter="url(#mp-uc)" opacity="' + (pale ? .9 : .5) + '"/>';
    function lumps(y0, wmin, wmax, dmin, dmax, col, op) {
      var o = '', x, w, dp, ax;
      for (x = -320; x < 1500; x += w * (.55 + r() * .3)) {
        w = wmin + r() * (wmax - wmin); dp = dmin + r() * (dmax - dmin) * (r() < .25 ? 1.5 : 1); ax = x + w * (.35 + r() * .3);
        var L = [[x, y0], [x + w * .1, y0 + dp * (.3 + r() * .2)], [ax - w * .16, y0 + dp * (.6 + r() * .15)], [ax - w * .04, y0 + dp * .92], [ax, y0 + dp]], R = [[ax, y0 + dp], [ax + w * .1, y0 + dp * (.62 + r() * .15)], [ax + w * .26, y0 + dp * (.4 + r() * .15)], [x + w * .9, y0 + dp * (.14 + r() * .12)], [x + w, y0]], P = function (q) { return f1(q[0]) + ',' + f1(q[1]); };
        o += '<path d="M' + L.map(P).join(' L') + ' L' + R.slice(1).map(P).join(' L') + ' Z" fill="' + mix(col, '#ffffff', .08) + '"/><path d="M' + P([ax - w * .1, y0]) + ' L' + R.map(P).join(' L') + ' Z" fill="' + mix(col, '#000', .5) + '"/>' +
          '<path d="M' + P(L[2]) + ' L' + P([ax - w * .22, y0]) + ' M' + P(R[2]) + ' L' + P([ax + w * .3, y0]) + '" stroke="#000" stroke-opacity=".28" stroke-width="1.2" fill="none"/><path d="M' + L.map(P).join(' L') + '" stroke="#fff" stroke-opacity=".14" stroke-width="1.4" fill="none"/>';
      }
      return '<g opacity="' + op + '">' + o + '</g>';
    }
    g += '<g filter="url(#src-blur3)">' + lumps(512, 60, 150, 26, 70, mix(rock, hz, .6), .9) + '</g><g filter="url(#mp-stone)">' + lumps(512, 70, 190, 44, 120, rock, 1) + '</g><rect x="-300" y="512" width="1800" height="30" fill="#000" opacity=".5" filter="url(#src-blur5)"/>';
    // pieces that came away, some near and sharp, some far and soft
    var near = '', farp = '';
    for (i = 0; i < 20; i++) { x = -260 + r() * 1720; var y = 610 + r() * 90, w2 = 5 + r() * 16, pd = mpLump(r, x, y, w2, 5 + Math.floor(r() * 3)), piece = '<path d="' + pd + '" fill="' + mix(rock, '#000', .1 + r() * .3) + '"/><path d="' + mpLump(r, x - w2 * .2, y - w2 * .25, w2 * .5, 4) + '" fill="#fff" opacity=".12"/>'; if (i % 3) near += piece; else farp += piece; }
    return s + g + '<g filter="url(#src-blur3)" opacity=".6">' + farp + '</g><g filter="url(#mp-stone)">' + near + '</g>';
  };

  // The layer each prop is drawn in: behind the keeper, on the counter, or over the whole room.
  CM_LAYER.back.push('cobwebs', 'nets', 'chains', 'magic-circle', 'eyes', 'floating-stones');
  CM_LAYER.front.push('mushrooms', 'tentacle');
  CM_LAYER.over.push('fireflies', 'drips', 'petals', 'butterflies', 'embers', 'smoke', 'comets', 'glyphs', 'dew', 'beads', 'rune-chase', 'arcs', 'bubbles', 'ghostlights', 'crumbs', 'dust');
  CM_WHERE.roots = 'among the roots'; CM_WHERE.adrift = 'floating in the air'; CM_WHERE.orbits = 'in its orbit'; CM_WHERE.columns = 'on the columns'; CM_WHERE.tendrils = 'in the tendrils'; CM_WHERE.circles = 'in its circle';
  CM_STARTERS.push(
    { name: "The Ship's Hold", version: 1, skeleton: { shape: 'hold', opening: 'porthole', display: 'pegboard', counter: 'straight' },
      theme: { wall: 'hull', wood: '#5a3c24', cloth: '#1f4a5a', accent: '#5fc8d8', trim: '#c8a04a', light: 'lamp', warmth: .7, outside: 'dusk' }, props: ['nets', 'chains'] },
    { name: 'The Hollow Tree', version: 1, skeleton: { shape: 'hollow', opening: 'knothole', display: 'shelves', counter: 'curved' },
      theme: { wall: 'bark', wood: '#6a4a2a', cloth: '#3a5a2a', accent: '#b8e060', trim: '#c8a04a', light: 'candles', warmth: .75, outside: 'day' }, props: ['fireflies', 'mushrooms', 'herbs'] },
    { name: 'The Crystal Cavern', version: 1, skeleton: { shape: 'cavern', opening: 'none', display: 'pigeonholes', counter: 'straight' },
      theme: { wall: 'rock', wood: '#4a4038', cloth: '#2a3a6a', accent: '#7fd8ff', trim: '#a8b0c0', light: 'crystals', warmth: .3, outside: 'night' }, props: ['drips', 'motes', 'crystal'] },
    { name: 'The Undercroft', version: 1, skeleton: { shape: 'tunnel', opening: 'grate', display: 'pegboard', counter: 'straight' },
      theme: { wall: 'brick', wood: '#3a2a20', cloth: '#4a2a4a', accent: '#8ae06a', trim: '#8a8a80', light: 'brazier', warmth: .85, outside: 'night' }, props: ['drips', 'cobwebs', 'chains', 'tentacle'] },
    { name: "The Smugglers' Cave", version: 1, skeleton: { shape: 'cavern', opening: 'grate', display: 'drawers', counter: 'straight' },
      theme: { wall: 'rock', wood: '#5a4028', cloth: '#6a2a20', accent: '#ffb050', trim: '#b08a4a', light: 'brazier', warmth: .9, outside: 'night' }, props: ['nets', 'chains', 'jars', 'cobwebs'] },
    { name: 'The Fey Glade', version: 1, skeleton: { shape: 'glade', opening: 'none', display: 'roots', counter: 'log' },
      theme: { wall: 'glade', wood: '#6a4a2c', cloth: '#5a2a6a', accent: '#ff9ad5', trim: '#d8c070', light: 'wisps', warmth: .4, outside: 'night' }, props: ['petals', 'butterflies', 'fireflies', 'dew', 'dust'] },
    { name: 'The Caldera', version: 1, skeleton: { shape: 'cavern', opening: 'lava-fall', display: 'columns', counter: 'slab' },
      theme: { wall: 'basalt', wood: '#3a2a22', cloth: '#6a1a10', accent: '#ff7a30', trim: '#8a7a6a', light: 'brazier', warmth: 1, outside: 'night' }, props: ['embers', 'smoke', 'anvil', 'chains', 'bubbles'] },
    { name: 'The Star Deck', version: 1, skeleton: { shape: 'void', opening: 'planet', display: 'orbits', counter: 'adrift' },
      theme: { wall: 'nebula', wood: '#2a2a4a', cloth: '#1a2a6a', accent: '#8ab4ff', trim: '#c8d0e8', light: 'star', warmth: .5, outside: 'night' }, props: ['comets', 'orrery', 'floating-books', 'motes', 'beads'] },
    { name: 'The Shop at Nowhere', version: 1, skeleton: { shape: 'void', opening: 'lone-door', display: 'adrift', counter: 'adrift' },
      theme: { wall: 'white', wood: '#b0a89c', cloth: '#3a4a7a', accent: '#6a8aff', trim: '#c8b070', light: 'halo', warmth: .5, outside: 'night' }, props: ['floating-stones', 'motes', 'crumbs'] },
    { name: 'The Whispering Vault', version: 1, skeleton: { shape: 'ribs', opening: 'rift', display: 'tendrils', counter: 'slab' },
      theme: { wall: 'obsidian', wood: '#2a2030', cloth: '#3a1a4a', accent: '#9aff6a', trim: '#8a8090', light: 'witchfire', warmth: .6, outside: 'night' }, props: ['eyes', 'tentacle', 'glyphs', 'cobwebs', 'arcs', 'ghostlights'] },
    { name: 'The Sanctum', version: 1, skeleton: { shape: 'round', opening: 'portal', display: 'circles', counter: 'slab' },
      theme: { wall: 'runestone', wood: '#3a2a4a', cloth: '#3a1a6a', accent: '#b08aff', trim: '#d8b860', light: 'orb', warmth: .5, outside: 'night' }, props: ['magic-circle', 'glyphs', 'floating-books', 'crystal', 'rune-chase'] }
  );

  // Helpers the later packs draw with too.
  K.FULL = FULL; K.mp3Defs = mp3Defs; K.mpFern = mpFern; K.mpFernClump = mpFernClump; K.mpFlower = mpFlower; K.mpLeaf = mpLeaf; K.mpLump = mpLump; K.mpMossPad = mpMossPad; K.mpOpen = mpOpen; K.mpSlots = mpSlots; K.mpSmooth = mpSmooth; K.mpStars = mpStars; K.mpTaper = mpTaper; K.mpVine = mpVine; K.RING = RING;
})();
