/**
 * craft_paint_forge.js -- the forge, a painted bench (CraftPaint scene "forge")
 *
 * A brick hearth breathing coals, an anvil on a stump, a hammer working a hot
 * bar, sparks that bounce and cool, a quench trough, and a window holding the
 * campaign's sky, all lit by the fire. This is the forge the operator signed;
 * keep its look unless a new one is signed. CraftStage runs it and adds the
 * effects of each level; look tells the stage where those effects go.
 */
(function () {
  'use strict';
  var CP = window.CraftPaint;
  if (!CP) return;

  CP.define('forge', function (g) {
    var W = CP.W, H = CP.H, N = CP.N, P = new CP.Particles(), v = { hits: 0 };
    var wall = CP.stone(W, 270, [96, 82, 70]), floor = CP.flags(W, 90, 270, [92, 80, 70]);
    var brick = CP.tex('brick', 260, 160, function (x, y) {
      var row = Math.floor(y / 14), bx = (x + (row % 2) * 13) % 26, by = y % 14;
      if (bx < 2 || by < 2) return [48, 40, 36];
      var n = N.fbm(x * .2, y * .2, 3), cell = N.v(Math.floor((x + (row % 2) * 13) / 26) * 2.1, row * 3.7), k = .55 + n * .45 + (cell - .5) * .4;
      var e = Math.min(bx - 2, by - 2, 26 - bx, 14 - by); k *= .75 + .25 * Math.min(1, e / 3);
      return [130 * k, 62 * k, 44 * k];
    });
    var hoodTex = CP.tex('hood', 280, 60, function (x, y) { var k = .5 + N.fbm(x * .05, y * .1, 4) * .5; return [70 * k, 58 * k, 50 * k]; });
    var bark = CP.tex('bark', 120, 70, function (x, y) { var k = .5 + N.fbm(x * .5, y * .04, 3) * .6; return [74 * k, 52 * k, 34 * k]; });
    var motes = [], m;
    for (m = 0; m < 40; m++) motes.push({ x: Math.random(), y: Math.random(), s: Math.random() * 9, z: .4 + Math.random() * .8 });
    // Coal lumps, placed once from a seed, each with its own breathing phase.
    var R = CP.rng(11), coals = [];
    for (var i = 0; i < 46; i++) coals.push({ x: 52 + R() * 160, y: 214 + R() * 16 - (i % 3) * 3, r: 5 + R() * 7, ph: R() * 50, s: .6 + R() * .6 });
    coals.sort(function (a, b) { return a.y - b.y; });
    // The baked flipbook's flames: x, phase, flip, width, height, alpha. The back two are smaller and dimmer.
    var FLAMES = [[64, 0, 1, 74, 104, .9], [92, 9, -1, 74, 104, .9], [120, 4, 1, 74, 104, .9], [150, 15, -1, 74, 104, .9], [178, 20, 1, 74, 104, .9], [104, 12, 1, 56, 78, .7], [160, 6, -1, 56, 78, .7]];
    var strike = { t: 0, n: 0, hitDone: false }, shake = 0, flash = 0, barHeat = 1, smoke = new CP.Particles();

    function brickArch() {
      // Soot climbs the wall above the hood.
      var so = g.createRadialGradient(140, 40, 10, 140, 60, 220); so.addColorStop(0, 'rgba(0,0,0,.65)'); so.addColorStop(1, 'rgba(0,0,0,0)');
      g.fillStyle = so; g.fillRect(0, 0, 340, 200);
      // The hood: plastered stone with an iron band, blackened underneath.
      g.save(); g.beginPath(); g.moveTo(10, 40); g.lineTo(270, 40); g.lineTo(250, 95); g.lineTo(30, 95); g.closePath(); g.clip();
      g.drawImage(hoodTex, 0, 38);
      var hg = g.createLinearGradient(0, 40, 0, 95); hg.addColorStop(0, 'rgba(0,0,0,.2)'); hg.addColorStop(1, 'rgba(0,0,0,.55)');
      g.fillStyle = hg; g.fillRect(0, 38, 290, 60); g.restore();
      g.fillStyle = '#26272b'; g.fillRect(26, 86, 228, 7); g.fillStyle = 'rgba(255,170,100,.25)'; g.fillRect(26, 92, 228, 1);
      for (var rv = 0; rv < 9; rv++) { g.fillStyle = '#4a4b50'; g.beginPath(); g.arc(36 + rv * 26, 89.5, 1.6, 0, 7); g.fill(); }
      // Brick surround, with the arch cut out of it.
      g.save(); g.beginPath(); g.rect(24, 95, 216, 140); g.moveTo(40, 235); g.lineTo(40, 140); g.quadraticCurveTo(130, 92, 220, 140); g.lineTo(220, 235); g.closePath();
      g.clip('evenodd'); g.drawImage(brick, 0, 0, 260, 160, 22, 95, 260, 160);
      var bs = g.createLinearGradient(24, 0, 240, 0); bs.addColorStop(0, 'rgba(0,0,0,.35)'); bs.addColorStop(.5, 'rgba(0,0,0,0)'); bs.addColorStop(1, 'rgba(0,0,0,.35)');
      g.fillStyle = bs; g.fillRect(20, 95, 230, 140); g.restore();
      // The arch's rim of darker bricks, scorched near the fire.
      g.strokeStyle = 'rgba(20,10,8,.7)'; g.lineWidth = 5; g.beginPath(); g.moveTo(40, 235); g.lineTo(40, 140); g.quadraticCurveTo(130, 92, 220, 140); g.lineTo(220, 235); g.stroke();
      // The firebox: deep black falling into red at the back.
      var fb = g.createRadialGradient(130, 225, 10, 130, 200, 120); fb.addColorStop(0, '#5a1c08'); fb.addColorStop(.5, '#1d0b06'); fb.addColorStop(1, '#090404');
      g.fillStyle = fb; g.beginPath(); g.moveTo(40, 235); g.lineTo(40, 140); g.quadraticCurveTo(130, 92, 220, 140); g.lineTo(220, 235); g.closePath(); g.fill();
      var lg = g.createLinearGradient(0, 232, 0, 246); lg.addColorStop(0, '#6a5a50'); lg.addColorStop(1, '#3a302a');
      g.fillStyle = lg; g.fillRect(20, 232, 220, 14);
    }
    /* The window: the campaign's sky when the stage hands one over (v.sky, a
       canvas painted by the calendar's sky code), a drawn night otherwise.
       v.skyLight is that sky's average colour, so the shaft is moonlight at
       night, daylight by day and grey in rain. */
    var WX0 = 474, WY0 = 26, WW = 96, WH = 94;
    function windowPane() {
      g.fillStyle = '#1a1512'; g.fillRect(WX0 - 9, WY0 - 9, WW + 18, WH + 18);
      if (v.sky) g.drawImage(v.sky, WX0, WY0, WW, WH);
      else {
        var sky = g.createLinearGradient(0, WY0, 0, WY0 + WH); sky.addColorStop(0, '#1b2846'); sky.addColorStop(1, '#3d5276');
        g.fillStyle = sky; g.fillRect(WX0, WY0, WW, WH);
        g.fillStyle = 'rgba(220,230,255,.85)'; g.beginPath(); g.arc(538, 50, 7, 0, 7); g.fill(); CP.glow(g, 538, 50, 24, [180, 200, 255], .35);
      }
      // Old glass: a faint sheen and the leading between the panes.
      var gl = g.createLinearGradient(WX0, WY0, WX0 + WW, WY0 + WH); gl.addColorStop(0, 'rgba(255,255,255,.06)'); gl.addColorStop(.5, 'rgba(255,255,255,0)'); gl.addColorStop(1, 'rgba(255,255,255,.04)');
      g.fillStyle = gl; g.fillRect(WX0, WY0, WW, WH);
      g.fillStyle = '#121010'; for (var b = 1; b < 4; b++) g.fillRect(WX0 + b * WW / 4 - 2, WY0, 4, WH);
      g.fillRect(WX0, WY0 + WH / 2 - 1.5, WW, 3);
      g.fillStyle = '#3a2f28'; g.fillRect(WX0 - 12, WY0 + WH + 6, WW + 24, 6);
    }
    function shaftLight(t) {
      var L = v.skyLight || { c: [150, 175, 230], k: 1 }, c = L.c.map(Math.round).join(',');
      g.save(); g.globalCompositeOperation = 'lighter';
      var top = WY0 + WH, sh = g.createLinearGradient(WX0 + 40, top, WX0 - 10, 340);
      sh.addColorStop(0, 'rgba(' + c + ',' + (.17 * L.k) + ')'); sh.addColorStop(1, 'rgba(' + c + ',0)');
      g.fillStyle = sh; g.beginPath(); g.moveTo(WX0, top); g.lineTo(WX0 + WW, top); g.lineTo(640, 340); g.lineTo(WX0 - 14, 340); g.closePath(); g.fill();
      motes.forEach(function (o) {
        var span = 340 - top, y = top + ((o.y * span + t * 6 * o.z) % span), q = (y - top) / span;
        var x = WX0 - 14 * q + o.x * (WW + (640 - WX0 - WW + 14) * q) + Math.sin(t * .5 + o.s) * 4;
        var a = (.25 + .25 * Math.sin(t * 1.3 + o.s)) * Math.min(1, L.k);
        g.fillStyle = 'rgba(' + c + ',' + a * o.z + ')'; g.beginPath(); g.arc(x, y, .9 * o.z, 0, 7); g.fill();
      });
      g.restore();
    }
    function wallTools(t) {
      // Tongs and spare hammers on pegs: dark iron catching the firelight on their left edges.
      g.save(); g.lineCap = 'round';
      [[300, 60, .05], [330, 58, -.04], [362, 62, .02]].forEach(function (p, i) {
        g.save(); g.translate(p[0], p[1]); g.rotate(Math.sin(t * .7 + i) * .01 + p[2]);
        g.fillStyle = '#3b2a1c'; g.beginPath(); g.arc(0, 0, 3, 0, 7); g.fill();
        if (i === 1) {
          g.strokeStyle = '#2a2b2e'; g.lineWidth = 3; g.beginPath(); g.moveTo(-2, 2); g.lineTo(-6, 80); g.moveTo(2, 2); g.lineTo(6, 80); g.stroke();
          g.strokeStyle = 'rgba(255,160,90,.35)'; g.lineWidth = 1; g.beginPath(); g.moveTo(-3, 4); g.lineTo(-7, 78); g.stroke();
        } else {
          var hl = g.createLinearGradient(-3, 0, 3, 0); hl.addColorStop(0, '#a8784a'); hl.addColorStop(1, '#5a3a22');
          g.fillStyle = hl; g.fillRect(-2.5, 2, 5, 64);
          var hd = g.createLinearGradient(-12, 0, 12, 0); hd.addColorStop(0, '#8a8f96'); hd.addColorStop(1, '#2f3237');
          g.fillStyle = hd; g.fillRect(-12, 62, 24, 12); g.fillStyle = 'rgba(255,170,100,.35)'; g.fillRect(-12, 62, 3, 12);
        }
        g.restore();
      });
      g.restore();
    }
    function stumpAnvil() {
      CP.shadow(g, 372, 330, 92, 16, 130, .75);
      // The stump, seen from standing height: bark sides and a ringed top.
      g.save(); g.beginPath(); g.moveTo(310, 250); g.bezierCurveTo(310, 290, 306, 304, 302, 312); g.ellipse(372, 312, 70, 17, 0, Math.PI, 0, true);
      g.bezierCurveTo(438, 304, 434, 290, 434, 250); g.ellipse(372, 250, 62, 11, 0, 0, Math.PI, false); g.closePath();
      g.clip(); g.drawImage(bark, 296, 238, 160, 94);
      var sg = g.createLinearGradient(310, 0, 434, 0); sg.addColorStop(0, 'rgba(255,150,80,.25)'); sg.addColorStop(.5, 'rgba(0,0,0,0)'); sg.addColorStop(1, 'rgba(0,0,0,.55)');
      g.fillStyle = sg; g.fillRect(296, 236, 160, 100);
      var rim = g.createLinearGradient(0, 300, 0, 330); rim.addColorStop(0, 'rgba(0,0,0,0)'); rim.addColorStop(1, 'rgba(0,0,0,.45)'); g.fillStyle = rim; g.fillRect(296, 296, 160, 40);
      g.restore();
      g.fillStyle = '#8a6a48'; g.beginPath(); g.ellipse(372, 250, 62, 11, 0, 0, 7); g.fill(); g.strokeStyle = 'rgba(40,26,16,.8)'; g.lineWidth = 1.5; g.stroke(); g.lineWidth = 1;
      g.strokeStyle = 'rgba(60,40,24,.6)'; for (var r = 8; r < 60; r += 7) { g.beginPath(); g.ellipse(372, 250, r, r * .18, 0, 0, 7); g.stroke(); }
      // The anvil: base, waist, face and horn, each face shaded for the fire on the left.
      function poly(pts, fill) { g.beginPath(); g.moveTo(pts[0], pts[1]); for (var k = 2; k < pts.length; k += 2) g.lineTo(pts[k], pts[k + 1]); g.closePath(); g.fillStyle = fill; g.fill(); }
      function steel(x0, x1, a, b) { var q = g.createLinearGradient(x0, 0, x1, 0); q.addColorStop(0, a); q.addColorStop(1, b); return q; }
      poly([330, 250, 414, 250, 404, 238, 340, 238], steel(330, 414, '#5b544f', '#1e1d1f'));
      poly([350, 238, 394, 238, 388, 214, 356, 214], steel(350, 394, '#615954', '#1b1a1c'));
      poly([318, 214, 430, 214, 446, 206, 470, 196, 430, 194, 318, 194], steel(318, 470, '#6e6660', '#26252a'));
      g.fillStyle = steel(318, 470, '#b9b2aa', '#4a4a50'); g.fillRect(318, 192, 112, 4);
      g.beginPath(); g.moveTo(430, 192); g.quadraticCurveTo(468, 192, 478, 198); g.lineTo(446, 200); g.closePath(); g.fill();
      g.fillStyle = 'rgba(255,180,110,.4)'; g.fillRect(318, 194, 3, 20);
    }
    function trough(t) {
      CP.shadow(g, 560, 330, 70, 10, 130, .6);
      g.fillStyle = '#4a3322'; g.fillRect(500, 270, 120, 58);
      for (var k = 0; k < 4; k++) { g.fillStyle = 'rgba(0,0,0,.25)'; g.fillRect(500, 276 + k * 13, 120, 1.5); }
      g.fillStyle = '#2e2f33'; g.fillRect(498, 282, 124, 4); g.fillRect(498, 314, 124, 4);
      var wg = g.createLinearGradient(0, 266, 0, 276); wg.addColorStop(0, '#24384a'); wg.addColorStop(1, '#0f1a24');
      g.fillStyle = wg; g.fillRect(503, 266, 114, 9);
      g.strokeStyle = 'rgba(255,170,100,.35)'; g.lineWidth = 1; g.beginPath();
      for (var x = 506; x < 614; x += 3) g.lineTo(x, 269 + Math.sin(x * .12 + t * 2) * .8);
      g.stroke();
      if (Math.random() < .04) P.add({ x: 520 + Math.random() * 80, y: 264, vx: 0, vy: -10, max: 3, kind: 'steam', s: 6, t0: Math.random() * 9, wob: 1.5 });
    }
    function coalsAndFire(t) {
      g.save(); g.globalCompositeOperation = 'lighter';
      coals.forEach(function (c) {
        var heat = .45 + .55 * N.v(c.ph + t * .6 * c.s, c.ph * .3);
        var core = 'rgba(255,' + Math.round(90 + 90 * heat) + ',' + Math.round(30 * heat) + ',' + (heat * .6) + ')';
        var gr = g.createRadialGradient(c.x, c.y, 0, c.x, c.y, c.r * 1.6); gr.addColorStop(0, core); gr.addColorStop(.6, 'rgba(200,50,10,' + heat * .5 + ')'); gr.addColorStop(1, 'rgba(120,20,0,0)');
        g.fillStyle = gr; g.beginPath(); g.arc(c.x, c.y, c.r * 1.6, 0, 7); g.fill();
      });
      g.restore();
      coals.forEach(function (c) { g.fillStyle = 'rgba(30,12,8,.28)'; g.beginPath(); g.ellipse(c.x - c.r * .2, c.y - c.r * .3, c.r * .55, c.r * .25, -.3, 0, 7); g.fill(); });
      CP.drawFire(g, t, 232, FLAMES);
      if (Math.random() < .5) P.add({ x: 70 + Math.random() * 130, y: 210, vx: (Math.random() - .5) * 10, vy: -30 - Math.random() * 40, max: 2 + Math.random() * 1.5, kind: 'ember', t0: Math.random() * 9, wob: 3, drag: .995 });
    }
    // Three strikes, then a breath. Each strike: lift, a held beat, a fast fall, a small recoil.
    function hammerAngle(dt) {
      strike.t += dt;
      var cyc = strike.n < 3 ? .95 : 2.2;
      if (strike.t > cyc) { strike.t = 0; strike.n = (strike.n + 1) % 4; strike.hitDone = false; }
      if (strike.n === 3) return 50 - Math.min(1, strike.t / .6) * 8;
      var t = strike.t;
      if (t < .34) return 50 * (1 - Math.pow(1 - t / .34, 3));
      if (t < .44) return 50 + Math.sin((t - .34) / .1 * Math.PI) * 2;
      if (t < .51) { var u = (t - .44) / .07; return 50 * (1 - u * u); }
      if (!strike.hitDone) { strike.hitDone = true; hit(); }
      if (t < .7) return Math.sin((t - .51) / .19 * Math.PI) * 8;
      return 0;
    }
    // Where the hammer's face meets the bar: every part of a blow starts here.
    var IX = 389, IY = 186, PIV = [690, 204], lastA = 0;
    function hit() {
      shake = 1; flash = 1; v.hits++; barHeat = Math.max(.55, barHeat - .06);
      // Sparks are flakes of burning steel: white-hot, fast, thrown mostly sideways along the face,
      // bouncing off the floor, some bursting into smaller sparks as they burn.
      for (var k = 0; k < 60; k++) {
        var side = Math.random() < .5 ? -1 : 1, a = side < 0 ? Math.PI + .05 + Math.random() * .7 : -.05 - Math.random() * .7, s = 110 + Math.random() * 230;
        P.add({ x: IX + side * 6, y: IY - 1, vx: Math.cos(a) * s, vy: Math.sin(a) * s - Math.random() * 40, g: 620, max: .3 + Math.random() * .9, kind: 'spark', bounce: .4, drag: .985, w: .6 + Math.random() * .9, hx: [], burst: Math.random() < .16 });
      }
      for (var j = 0; j < 7; j++) P.add({ x: IX + (Math.random() - .5) * 24, y: IY, vx: (Math.random() - .5) * 110, vy: -50 - Math.random() * 80, g: 500, max: 1.6, kind: 'scale', bounce: .2, rot: Math.random() * 6, hot: Math.random() < .5 });
      P.add({ x: IX, y: IY, vx: 0, vy: 0, max: .12, kind: 'star' });
    }
    function burst(p) {
      for (var j = 0; j < 4; j++) {
        var a = Math.random() * 6.28, s = 60 + Math.random() * 120;
        P.add({ x: p.x, y: p.y, vx: p.vx * .3 + Math.cos(a) * s, vy: p.vy * .3 + Math.sin(a) * s, g: 400, max: .12 + Math.random() * .2, kind: 'spark', drag: .97, w: .6, hx: [] });
      }
    }
    // The hammer is held from off the right edge, so it swings on a long arc like a smith's arm.
    // In its own frame the handle runs along x to the head, and the striking face points along -y.
    function hammerShape(a, alpha) {
      var L = Math.hypot(PIV[0] - IX, PIV[1] - IY) - 13, ang = Math.atan2(IY - 13 - PIV[1], IX - 1 - PIV[0]);
      g.save(); g.globalAlpha = alpha; g.translate(PIV[0], PIV[1]); g.rotate(a * .42 * Math.PI / 180 + ang);
      // The handle: ash, tapering toward the head, with its grain and the firelight down one edge.
      var hg = g.createLinearGradient(0, -5, 0, 5); hg.addColorStop(0, '#b98a58'); hg.addColorStop(.45, '#8a5e33'); hg.addColorStop(1, '#3e2716');
      g.fillStyle = hg; g.beginPath(); g.moveTo(0, -6); g.lineTo(L - 40, -4.2); g.quadraticCurveTo(L - 14, -3.2, L - 9, -5); g.lineTo(L + 11, -4.6); g.lineTo(L + 11, 4.6);
      g.lineTo(L - 9, 5); g.quadraticCurveTo(L - 14, 3.2, L - 40, 4.2); g.lineTo(0, 6); g.closePath(); g.fill();
      g.strokeStyle = 'rgba(70,40,18,.35)'; g.lineWidth = .7;
      for (var k = -2; k <= 2; k++) { g.beginPath(); g.moveTo(10, k * 1.6); g.bezierCurveTo(L * .4, k * 1.4 + 1, L * .7, k * 1.1 - 1, L - 12, k * .9); g.stroke(); }
      g.fillStyle = 'rgba(255,190,130,.35)'; g.fillRect(0, -5.6, L - 12, 1.2);
      // The head: forged steel, a crowned face below and a cross-peen above, drawn a size up about
      // the face so the face still lands exactly on the bar.
      g.save(); g.translate(L, -13); g.scale(1.3, 1.3); g.translate(-L, 13);
      var sg = g.createLinearGradient(L - 10, 0, L + 10, 0); sg.addColorStop(0, '#1f2126'); sg.addColorStop(.4, '#4a4d53'); sg.addColorStop(.75, '#6e7076'); sg.addColorStop(1, '#d9a77a');
      g.fillStyle = sg; g.beginPath();
      g.moveTo(L - 10.5, -13); g.quadraticCurveTo(L, -14.2, L + 10.5, -13); g.lineTo(L + 10.5, -10); g.lineTo(L + 9, -8); g.lineTo(L + 9, 7);
      g.lineTo(L + 9, 10); g.lineTo(L + 4, 20); g.quadraticCurveTo(L, 22, L - 4, 20); g.lineTo(L - 9, 10); g.lineTo(L - 9, 7); g.lineTo(L - 9, -8); g.lineTo(L - 10.5, -10); g.closePath(); g.fill();
      g.strokeStyle = 'rgba(10,10,12,.7)'; g.lineWidth = .8; g.stroke();
      // Forge scale, a bright worn face, and the eye with the handle's end and its wedge showing.
      g.fillStyle = 'rgba(20,20,24,.35)'; g.fillRect(L - 9, -2, 18, 9);
      g.fillStyle = 'rgba(255,214,170,.6)'; g.fillRect(L + 7, -9, 1.6, 15);
      g.fillStyle = '#b8a99a'; g.beginPath(); g.moveTo(L - 10.5, -13); g.quadraticCurveTo(L, -14.2, L + 10.5, -13); g.lineTo(L + 10.5, -11.4); g.quadraticCurveTo(L, -12.6, L - 10.5, -11.4); g.fill();
      g.fillStyle = '#8b6440'; g.fillRect(L + 9, -4.6, 2, 9.2); g.fillStyle = '#3a3c40'; g.fillRect(L + 9.4, -.6, 1.4, 1.2);
      g.fillStyle = 'rgba(255,150,80,' + (.25 + .5 * flash) + ')'; g.fillRect(L - 10, -13.6, 20, 1.6);
      g.restore(); g.restore();
    }
    // A fast fall smears: a few faint copies between the last angle and this one.
    function hammer(a) {
      var d = a - lastA;
      if (Math.abs(d) > 2.5) for (var k = 1; k <= 3; k++) hammerShape(lastA + d * k / 4, .12);
      hammerShape(a, 1); lastA = a;
    }
    function hotBar() {
      var hb = barHeat;
      g.save(); g.globalCompositeOperation = 'lighter';
      CP.glow(g, 390, 190, 60, [255, 120, 40], .35 * hb);
      var bg = g.createLinearGradient(330, 0, 440, 0); bg.addColorStop(0, 'rgba(120,40,10,.9)'); bg.addColorStop(.35, 'rgba(255,' + Math.round(150 * hb) + ',60,1)');
      bg.addColorStop(.6, 'rgba(255,' + Math.round(220 * hb) + ',140,1)'); bg.addColorStop(1, 'rgba(255,120,40,.9)');
      g.fillStyle = bg; g.fillRect(332, 186, 104, 7); g.restore();
      // Tongs from the left, holding the cool end.
      g.strokeStyle = '#1d1e21'; g.lineWidth = 3; g.lineCap = 'round';
      g.beginPath(); g.moveTo(240, 196); g.lineTo(334, 187); g.moveTo(240, 202); g.lineTo(334, 192); g.stroke();
      g.strokeStyle = 'rgba(255,170,100,.45)'; g.lineWidth = 1; g.beginPath(); g.moveTo(240, 195); g.lineTo(334, 186); g.stroke();
    }
    function drawParticles() {
      g.save(); g.globalCompositeOperation = 'lighter';
      P.list.forEach(function (p) {
        var k = p.life / p.max;
        if (p.kind === 'spark') {
          // A short trail of where it has been: thick and white at the head, red and thin behind.
          p.hx.push(p.x, p.y); if (p.hx.length > 10) p.hx.splice(0, 2);
          var heat = Math.min(1, k * 1.4), hc = heat > .7 ? '255,226,150' : heat > .4 ? '255,160,60' : '230,80,25';
          for (var j = 2; j < p.hx.length; j += 2) {
            var f = j / p.hx.length;
            g.strokeStyle = 'rgba(' + hc + ',' + (f * (.35 + .65 * heat)) + ')'; g.lineWidth = p.w * (.4 + f);
            g.beginPath(); g.moveTo(p.hx[j - 2], p.hx[j - 1]); g.lineTo(p.hx[j], p.hx[j + 1]); g.stroke();
          }
          if (heat > .7) { g.fillStyle = 'rgba(255,245,215,' + (heat - .7) * 2.5 + ')'; g.fillRect(p.x - p.w * .6, p.y - p.w * .6, p.w * 1.2, p.w * 1.2); }
          if (p.burst && k < .55) { p.burst = false; burst(p); }
        } else if (p.kind === 'star') {
          CP.glow(g, p.x, p.y, 34, [255, 235, 190], .9 * k);
          g.fillStyle = 'rgba(255,250,235,' + k + ')';
          g.beginPath(); g.moveTo(p.x - 46 * k, p.y); g.lineTo(p.x, p.y - 2); g.lineTo(p.x + 46 * k, p.y); g.lineTo(p.x, p.y + 2); g.closePath(); g.fill();
          g.beginPath(); g.moveTo(p.x, p.y - 18 * k); g.lineTo(p.x + 1.5, p.y); g.lineTo(p.x, p.y + 10 * k); g.lineTo(p.x - 1.5, p.y); g.closePath(); g.fill();
        } else if (p.kind === 'ember') {
          g.fillStyle = 'rgba(255,' + Math.round(120 + 100 * k) + ',50,' + Math.sin(k * Math.PI) + ')'; g.beginPath(); g.arc(p.x, p.y, 1.2, 0, 7); g.fill();
        }
      });
      g.restore();
      P.list.forEach(function (p) {
        var k = p.life / p.max;
        if (p.kind === 'scale') {
          g.save(); g.translate(p.x, p.y); g.rotate(p.rot + p.life * 8);
          g.fillStyle = p.hot && k > .6 ? 'rgba(255,' + Math.round(90 + 120 * (k - .6) / .4) + ',40,1)' : 'rgba(30,26,24,' + Math.min(1, k * 2) + ')';
          g.fillRect(-2, -1, 4, 2); g.restore();
        } else if (p.kind === 'steam') {
          var a = Math.sin(k * Math.PI) * .09, r = p.s + (1 - k) * 22, sg = g.createRadialGradient(p.x, p.y, 0, p.x, p.y, r);
          sg.addColorStop(0, 'rgba(220,220,225,' + a + ')'); sg.addColorStop(1, 'rgba(220,220,225,0)');
          g.fillStyle = sg; g.beginPath(); g.arc(p.x, p.y, r, 0, 7); g.fill();
        }
      });
    }
    // Smoke rolling up into the hood and out of sight; only at the richer levels (v.rich), where it reads.
    function smokeRise(dt) {
      if (Math.random() < .35) smoke.add({ x: 70 + Math.random() * 120, y: 150, vx: (Math.random() - .5) * 6, vy: -14 - Math.random() * 8, max: 4, s: 10, t0: Math.random() * 9, wob: 1.2 });
      smoke.step(dt);
      smoke.list.forEach(function (p) {
        var q = p.life / p.max, a = Math.sin(q * Math.PI) * .12, r = p.s + (1 - q) * 30;
        if (p.y < 92) p.vx += (p.x < 140 ? 1 : -1) * 12 * dt;
        var gr = g.createRadialGradient(p.x, p.y, 0, p.x, p.y, r); gr.addColorStop(0, 'rgba(40,34,32,' + a + ')'); gr.addColorStop(1, 'rgba(40,34,32,0)');
        g.fillStyle = gr; g.beginPath(); g.arc(p.x, p.y, r, 0, 7); g.fill();
      });
    }
    // Heat haze on every level: the band above the fire redrawn in thin strips, each nudged sideways.
    // The band is copied out once a frame and the strips drawn from the copy:
    // drawing a canvas onto itself copies the whole canvas on every call.
    var band = null;
    function shimmer(t) {
      var cv = g.canvas, s = cv.width / W, bw = Math.ceil(170 * s), bh = Math.ceil(100 * s);
      if (!band) band = document.createElement('canvas');
      if (band.width !== bw || band.height !== bh) { band.width = bw; band.height = bh; }
      var bg = band.getContext('2d');
      bg.clearRect(0, 0, bw, bh);
      bg.drawImage(cv, 45 * s, 100 * s, bw, bh, 0, 0, bw, bh);
      for (var y = 100; y < 200; y += 3) {
        var off = Math.sin(y * .18 + t * 7) * 1.4 * (1 - (200 - y) / 140);
        g.drawImage(band, 0, (y - 100) * s, 170 * s, 3 * s, 45 + off, y, 170, 3);
      }
    }
    v.frame = function (t, dt) {
      var flick = .85 + .15 * N.v(t * 6, 1) + .05 * Math.sin(t * 23), a = hammerAngle(dt);
      g.save();
      if (shake > 0) { g.translate((Math.random() - .5) * shake * 2, (Math.random() - .5) * shake * 2); shake -= dt * 8; }
      g.drawImage(wall, 0, 0); brickArch(); wallTools(t);
      g.drawImage(floor, 0, 270);
      var fg = g.createLinearGradient(0, 262, 0, 290); fg.addColorStop(0, 'rgba(0,0,0,.6)'); fg.addColorStop(1, 'rgba(0,0,0,0)'); g.fillStyle = fg; g.fillRect(0, 262, W, 30);
      trough(t); stumpAnvil();
      // One light, the flickering fire, over the room's cool ambient; daylight from the window lifts the shadows.
      var SL = v.skyLight, d = SL ? SL.day * .5 : 0;
      CP.lightPool(g, 140, 210, 470 * flick, [255, 200, 150], SL ? [56 + SL.c[0] * d, 48 + SL.c[1] * d, 62 + SL.c[2] * d] : [56, 48, 62]);
      windowPane(); shaftLight(t); coalsAndFire(t); shimmer(t); hotBar(); hammer(a);
      if (flash > 0) { CP.glow(g, IX, IY, 70, [255, 220, 150], flash * .6); flash -= dt * 5; }
      P.step(dt, 316); drawParticles();
      if (v.rich) smokeRise(dt);
      g.restore();
      CP.grain(g, .07); CP.vignette(g, .6);
    };
    // A click starts a fresh run of strikes.
    v.poke = function () { strike.n = 0; strike.t = .3; };
    // Where the stage's effects go, in the scene's 640x360: the window (for the sky), the blow (a
    // shockwave and a streak), the moon's light (rays), and the heat haze over the fire and the bar.
    v.look = { window: [WX0, WY0, WW, WH], hit: [IX, IY], light: [522, 73], haze: [[32, 79, 237, 238], [320, 151, 442, 194]] };
    v.label = 'The forge: a hammer working a hot bar at the anvil, the fire behind it';
    return v;
  });
})();
