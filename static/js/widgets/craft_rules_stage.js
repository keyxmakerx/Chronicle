/**
 * craft_rules_stage.js -- the bench a crafting moment plays on, for the
 * rules editor's How it looks (craft_rules_looks.js)
 *
 * A moment plays in two beats, brewing then the outcome, and each of the
 * moment spots (ItemLookFX.MOMENT_SPOTS: window, heat, work, bench, result)
 * plays the look placed on it. It is the shared effects engine's Stage with
 * the alchemy bench drawn in place of the item stage, so the looks, the
 * particles, the sounds, the light that tints the room, the shadows and the
 * one clock (which sleeps when the stage is off screen and honours Calm and
 * Off) are all ItemLookFX's. Exposes window.CraftRulesStage.
 */
(function () {
  'use strict';
  var FX = window.ItemLookFX;
  if (!FX) return;
  var TAU = Math.PI * 2, mix = FX.mix, rgba = FX.rgba, sprite = FX.sprite, MOTIFS = FX.MOTIFS;
  function rand(a, b) { return a + Math.random() * (b - a); }
  function clamp(v, a, b) { return v < a ? a : v > b ? b : v; }
  function ease(t) { t = clamp(t, 0, 1); return 1 - Math.pow(1 - t, 3); }
  function calm() { var d = document.documentElement; return !!d && (d.getAttribute('data-motion') || 'full') !== 'full'; }
  function rr(g, x, y, w, h, r) { g.beginPath(); g.moveTo(x + r, y); g.arcTo(x + w, y, x + w, y + h, r); g.arcTo(x + w, y + h, x, y + h, r); g.arcTo(x, y + h, x, y, r); g.arcTo(x, y, x + w, y, r); g.closePath(); }

  /* What each moment spot is on this bench, where it sits on the 960 by 540
     stage, and when its look plays. The keys are the engine's. */
  var LABELS = {
    window: {name: 'The window', when: 'the whole time', x: 790, y: 142},
    heat: {name: 'The flame', when: 'while it heats', x: 480, y: 342},
    work: {name: 'In the flask', when: 'while it brews', x: 480, y: 258},
    bench: {name: 'The bench', when: 'at the moment', x: 290, y: 430},
    result: {name: 'What comes out', when: 'when it’s done', x: 736, y: 340}
  };
  var SPOTS = FX.MOMENT_SPOTS.map(function (k) { var l = LABELS[k] || {name: k, when: '', x: 480, y: 270}; return {k: k, name: l.name, when: l.when, x: l.x, y: l.y}; });
  var BREW = 0.9, WORDS = 1.05, END = 2.9;

  /* A moon disc at phase p (0 new, .5 full) for the window. */
  function moonDisc(g, x, y, r, col, p) {
    g.fillStyle = 'rgba(0,0,0,.35)'; g.beginPath(); g.arc(x, y, r, 0, TAU); g.fill();
    if (p < 0.02 || p > 0.98) return;
    var k = Math.cos(TAU * p), wax = p < 0.5;
    g.fillStyle = col; g.beginPath();
    if (Math.abs(p - 0.5) < 0.02) g.arc(x, y, r, 0, TAU);
    else { g.arc(x, y, r, -Math.PI / 2, Math.PI / 2, !wax); g.ellipse(x, y, Math.abs(k) * r, r, 0, Math.PI / 2, -Math.PI / 2, k > 0 ? wax : !wax); }
    g.fill();
    g.save(); g.globalCompositeOperation = 'lighter'; g.globalAlpha = 0.25; g.drawImage(sprite(col), x - r * 2.4, y - r * 2.4, r * 4.8, r * 4.8); g.restore();
  }

  function Stage(canvas) {
    this.out = null; this.moons = []; this.liquid = '#9b1c2c'; this.made = false; this.acc = {};
    FX.Stage.call(this, canvas);
  }
  Stage.prototype = Object.create(FX.Stage.prototype);
  Stage.prototype.constructor = Stage;
  /* out is a moment: {spots: {spot: {look, move}}, words, makes}. */
  Stage.prototype.set = function (out) { this.out = out; this.still = false; };
  /* moons are the window's, as on the preview day: [{color, p, gone}]. */
  Stage.prototype.setMoons = function (list) { this.moons = list || []; this.still = false; };
  Stage.prototype.play = function () {
    this.mode = 'play'; this.mt = 0; this.fired = {}; this.words = []; this.rings = []; this.made = false; this.still = false;
    FX.Sound.play('hiss', 0, 0);
  };
  Stage.prototype.slot = function (k) { return (this.out && this.out.spots && this.out.spots[k]) || {}; };
  Stage.prototype.layer = function (k) {
    var s = this.slot(k), L = s.look && Object.prototype.hasOwnProperty.call(FX.LOOKS, s.look) ? FX.LOOKS[s.look] : null;
    if (!L) return null;
    return {motif: L.motif, c: L.color, c2: L.color2, sound: L.sound, pitch: L.pitch, light: L.light == null ? 1 : L.light, shimmer: L.shimmer};
  };
  /* A look with no particles shows as its glints, as the item stage does. */
  Stage.prototype.burst = function (ly, x, y, n, burst, sx, sy) {
    sx = sx || 12; sy = sy || 12;
    if (!ly || !ly.motif || ly.motif === 'none' || !MOTIFS[ly.motif]) {
      if (ly && ly.shimmer) for (var j = 0; j < Math.min(n, 6); j++) this.parts.push({motif: 'glints', c: ly.shimmer, x: x + rand(-sx, sx), y: y + rand(-sy, sy), age: 0, life: rand(0.5, 0.9), size: rand(5, 10), vx: 0, vy: 0, g: 0, drag: 1, seed: 0});
      return;
    }
    for (var i = 0; i < n; i++) {
      var p = {motif: ly.motif, c: ly.c, c2: ly.c2, x: x + rand(-sx, sx), y: y + rand(-sy, sy), age: 0, seed: rand(0, 100), drag: 1, g: 0, rot: 0, vr: 0};
      MOTIFS[ly.motif].make(p);
      if (burst) { var a = rand(0, TAU), s = rand(40, 170) * (burst === true ? 1 : burst); p.vx += Math.cos(a) * s; p.vy += Math.sin(a) * s * 0.8; p.drag = 0.93; p.life *= 0.9; }
      this.parts.push(p);
    }
    if (this.parts.length > 900) this.parts.splice(0, this.parts.length - 900);
  };

  /* The room: wall, a shelf of jars and books, the bench top. Cached. */
  Stage.prototype.backdrop = function () {
    if (this.bg) return this.bg;
    var c = document.createElement('canvas'); c.width = this.cv.width; c.height = this.cv.height;
    var g = c.getContext('2d'); g.scale(this.scale, this.scale);
    var wall = g.createLinearGradient(0, 0, 0, 400); wall.addColorStop(0, '#15110e'); wall.addColorStop(1, '#241b14'); g.fillStyle = wall; g.fillRect(0, 0, 960, 540);
    g.globalAlpha = 0.05; g.strokeStyle = '#000';
    for (var x = 0; x < 960; x += 46) { g.lineWidth = 2; g.beginPath(); g.moveTo(x + Math.sin(x) * 3, 0); g.lineTo(x, 380); g.stroke(); }
    g.globalAlpha = 1;
    var sy = 170; g.fillStyle = '#2c2117'; g.fillRect(40, sy, 580, 9); g.fillStyle = 'rgba(0,0,0,.35)'; g.fillRect(40, sy + 9, 580, 6);
    [[64, 34, 18, '#5a3d2a'], [96, 46, 14, '#2f4a3a'], [124, 30, 20, '#4a3346'], [170, 22, 26, '#3b3424'], [210, 40, 12, '#2b3b52'], [420, 38, 18, '#3d2a2a'], [452, 26, 22, '#2d3f33'], [488, 48, 12, '#4a3b22'], [520, 30, 18, '#33304a'], [566, 36, 16, '#3b2b20']].forEach(function (j) {
      g.fillStyle = j[3]; rr(g, j[0], sy - j[1], j[2], j[1], 4); g.fill();
      g.fillStyle = '#5a4430'; g.fillRect(j[0] + j[2] * 0.2, sy - j[1] - 5, j[2] * 0.6, 5);
      g.fillStyle = 'rgba(255,230,190,.07)'; g.fillRect(j[0] + 3, sy - j[1] + 4, 2, j[1] - 8);
    });
    for (var b = 0; b < 7; b++) { g.fillStyle = ['#4a2a22', '#2a3a4a', '#3a2f22', '#4a3a2a', '#2a2a3a', '#3d2626', '#2c3a2c'][b]; g.fillRect(270 + b * 13, sy - 40 + (b % 3) * 4, 11, 40 - (b % 3) * 4); }
    var led = g.createLinearGradient(0, 392, 0, 540); led.addColorStop(0, '#4a3524'); led.addColorStop(0.08, '#3a2919'); led.addColorStop(1, '#1a120b'); g.fillStyle = led; g.fillRect(0, 392, 960, 148);
    g.fillStyle = 'rgba(255,220,170,.10)'; g.fillRect(0, 392, 960, 2);
    g.globalAlpha = 0.07; g.strokeStyle = '#000';
    for (var i = 0; i < 26; i++) { var y = 400 + i * 5.5 + Math.sin(i) * 3; g.beginPath(); g.moveTo(0, y); g.bezierCurveTo(300, y + 4, 640, y - 4, 960, y + 2); g.stroke(); }
    g.globalAlpha = 1;
    var vg = g.createRadialGradient(480, 290, 160, 480, 290, 640); vg.addColorStop(0, 'rgba(0,0,0,0)'); vg.addColorStop(1, 'rgba(0,0,0,.6)'); g.fillStyle = vg; g.fillRect(0, 0, 960, 540);
    return (this.bg = c);
  };

  /* The bench's things, drawn unlit on their own layer so light can land on
     them: the window with the campaign's moons, a candle, a mortar, the
     burner and flask, and the cloth the finished thing lands on. */
  Stage.prototype.props = function (o) {
    var sc = this.scale, W = this.cv.width, H = this.cv.height;
    if (!this.pc || this.pc.width !== W || this.pc.height !== H) { this.pc = document.createElement('canvas'); this.pc.width = W; this.pc.height = H; }
    var g = this.pc.getContext('2d'); g.setTransform(1, 0, 0, 1, 0, 0); g.globalCompositeOperation = 'source-over'; g.clearRect(0, 0, W, H); g.setTransform(sc, 0, 0, sc, 0, 0);
    var wx = 700 + o.win, wy = 48; g.fillStyle = '#3a2a1c'; rr(g, wx - 8, wy - 8, 196, 206, 6); g.fill();
    var sky = g.createLinearGradient(0, wy, 0, wy + 190); sky.addColorStop(0, o.winDim ? '#05070d' : '#0d1630'); sky.addColorStop(1, o.winDim ? '#0b0d14' : '#26304d'); g.fillStyle = sky; g.fillRect(wx, wy, 180, 190);
    g.save(); g.beginPath(); g.rect(wx, wy, 180, 190); g.clip();
    g.fillStyle = 'rgba(255,255,255,.6)';
    for (var s = 0; s < 22; s++) { g.globalAlpha = 0.25 + 0.35 * Math.abs(Math.sin(this.t * 1.3 + s)); g.fillRect(wx + ((s * 73) % 180), wy + ((s * 41) % 150), 1.4, 1.4); }
    g.globalAlpha = 1;
    var shown = this.moons.filter(function (m) { return !m.gone; }).slice(0, 3);
    shown.forEach(function (m, i) { moonDisc(g, wx + 52 + i * 52, wy + 58 + i * 36, 15 - i * 3, m.color, m.p); });
    g.restore(); g.fillStyle = '#3a2a1c'; g.fillRect(wx + 88, wy, 5, 190); g.fillRect(wx, wy + 92, 180, 5);
    g.fillStyle = '#2a1e14'; g.fillRect(wx - 14, wy + 196, 208, 10);
    var cx = 95 + o.all; g.fillStyle = '#e9dcc0'; rr(g, cx - 11, 330, 22, 62, 3); g.fill(); g.fillStyle = 'rgba(0,0,0,.18)'; g.fillRect(cx + 4, 330, 7, 62); g.fillStyle = '#2a2018'; g.fillRect(cx - 1, 322, 2, 9);
    var mx = 220 + o.all; g.fillStyle = '#6b6258'; g.beginPath(); g.moveTo(mx - 48, 352); g.quadraticCurveTo(mx - 44, 392, mx, 392); g.quadraticCurveTo(mx + 44, 392, mx + 48, 352); g.closePath(); g.fill();
    g.fillStyle = '#544c43'; g.beginPath(); g.ellipse(mx, 352, 48, 10, 0, 0, TAU); g.fill(); g.fillStyle = '#2e2924'; g.beginPath(); g.ellipse(mx, 352, 40, 7, 0, 0, TAU); g.fill();
    g.strokeStyle = '#8a7f72'; g.lineWidth = 10; g.lineCap = 'round'; g.beginPath(); g.moveTo(mx + 8, 350); g.lineTo(mx + 46, 300); g.stroke();
    var fx = 480 + o.all; g.strokeStyle = '#4a3a2c'; g.lineWidth = 5; g.beginPath(); g.moveTo(fx - 42, 392); g.lineTo(fx - 30, 300); g.moveTo(fx + 42, 392); g.lineTo(fx + 30, 300); g.stroke();
    g.strokeStyle = '#5b4836'; g.lineWidth = 4; g.beginPath(); g.ellipse(fx, 300, 32, 6, 0, 0, TAU); g.stroke();
    g.fillStyle = '#5a4a3a'; rr(g, fx - 22, 368, 44, 24, 5); g.fill(); g.fillStyle = '#3a2f25'; g.fillRect(fx - 4, 360, 8, 9);
    var kx = fx + o.flask, ky = 264; g.save(); g.beginPath(); g.arc(kx, ky, 34, 0, TAU); g.clip(); g.fillStyle = 'rgba(255,255,255,.06)'; g.fillRect(kx - 36, ky - 36, 72, 72);
    var wob = Math.sin(this.t * 2.2 + o.flask * 0.3) * 2.5 * (1 + o.boil * 2), lq = g.createLinearGradient(0, ky - 4, 0, ky + 36);
    lq.addColorStop(0, mix(this.liquid, '#ffffff', 0.25)); lq.addColorStop(1, mix(this.liquid, '#000000', 0.45));
    g.fillStyle = lq; g.beginPath(); g.moveTo(kx - 36, ky - 2 + wob); g.quadraticCurveTo(kx, ky - 6 - wob, kx + 36, ky - 2 - wob); g.lineTo(kx + 36, ky + 36); g.lineTo(kx - 36, ky + 36); g.closePath(); g.fill();
    if (o.boil > 0) { g.fillStyle = 'rgba(255,255,255,.35)'; for (var b = 0; b < 6; b++) { var by = ky + 30 - ((this.t * 60 + b * 13) % 34), bx = kx - 18 + ((b * 17) % 36); g.beginPath(); g.arc(bx, by, 1.5 + b % 2, 0, TAU); g.fill(); } }
    g.restore(); g.fillStyle = 'rgba(210,225,235,.10)'; g.fillRect(kx - 8, ky - 62, 16, 30); g.strokeStyle = 'rgba(225,235,245,.6)'; g.lineWidth = 1.6;
    g.beginPath(); g.moveTo(kx - 8, ky - 60); g.lineTo(kx - 8, ky - 33); g.arc(kx, ky, 34, -Math.PI / 2 - 0.24, -Math.PI / 2 + 0.24 + TAU, false); g.stroke(); g.beginPath(); g.moveTo(kx + 8, ky - 60); g.lineTo(kx + 8, ky - 33); g.stroke();
    g.fillStyle = '#9b7550'; rr(g, kx - 9, ky - 70, 18, 11, 3); g.fill(); g.strokeStyle = 'rgba(255,255,255,.5)'; g.lineWidth = 2.5; g.beginPath(); g.arc(kx, ky, 26, Math.PI * 1.1, Math.PI * 1.35); g.stroke();
    var tx = 736 + o.all; g.fillStyle = '#5b3a2e'; g.beginPath(); g.ellipse(tx, 388, 54, 8, 0, 0, TAU); g.fill(); g.fillStyle = 'rgba(255,220,180,.08)'; g.beginPath(); g.ellipse(tx - 10, 386, 30, 3, 0, 0, TAU); g.fill();
    if (o.pop > 0) {
      var vx = tx + o.result; g.save(); g.translate(vx, 386); g.scale(o.pop, o.pop);
      g.fillStyle = mix(this.liquid, '#000000', 0.2); rr(g, -11, -44, 22, 40, 8); g.fill(); g.fillStyle = mix(this.liquid, '#ffffff', 0.25); rr(g, -11, -30, 22, 6, 3); g.fill();
      g.strokeStyle = 'rgba(235,240,250,.65)'; g.lineWidth = 1.4; rr(g, -12, -50, 24, 48, 8); g.stroke(); g.fillStyle = '#9b7550'; rr(g, -6, -58, 12, 9, 2); g.fill();
      g.fillStyle = 'rgba(255,255,255,.45)'; g.fillRect(-7, -44, 2.5, 26); g.restore();
    }
    return this.pc;
  };
  /* Lights land on the things: lit on the side facing each light, shaded on
     the far side from the brightest. */
  Stage.prototype.propLight = function (lights) {
    var g = this.pc.getContext('2d'); g.globalCompositeOperation = 'source-atop';
    lights.forEach(function (li) {
      var k = clamp(li.k, 0, 1.2); if (k < 0.02) return;
      var gr = g.createRadialGradient(li.x, li.y, 0, li.x, li.y, 220 + 140 * k);
      gr.addColorStop(0, rgba(mix(li.c, '#ffffff', 0.3), 0.7 * k)); gr.addColorStop(0.45, rgba(li.c, 0.3 * k)); gr.addColorStop(1, rgba(li.c, 0)); g.fillStyle = gr; g.fillRect(0, 0, 960, 540);
    });
    var main = lights.reduce(function (a, b) { return b.k > a.k ? b : a; });
    var sh = g.createRadialGradient(main.x, main.y, 120, main.x, main.y, 900); sh.addColorStop(0, 'rgba(0,0,0,0)'); sh.addColorStop(1, 'rgba(0,0,0,' + (0.2 + 0.25 * clamp(main.k, 0, 1)).toFixed(3) + ')');
    g.fillStyle = sh; g.fillRect(0, 0, 960, 540); g.globalCompositeOperation = 'source-over';
  };

  /* One frame: lights, shadows, the lit things, the flames, then each spot's
     look: a trickle while idle or brewing, a burst at the moment, and the
     moves (a shake rattles what sits at the spot, dim drops the light). */
  Stage.prototype.frame = function (dt) {
    var g = this.g; if (!this.scale || !this.out) return;
    this.t += dt; if (this.mode === 'play') this.mt += dt;
    var self = this, play = this.mode === 'play', u = this.mt, M = BREW, after = play && u >= M, since = u - M, quiet = calm(), L = {};
    SPOTS.forEach(function (s) { L[s.k] = self.layer(s.k); });
    function mv(k) { return self.slot(k).move; }
    function shk(k) { return mv(k) === 'shake' && after && !quiet ? Math.sin(since * 60) * 6 * Math.exp(-since * 5) : 0; }
    var dimK = 0; SPOTS.forEach(function (s) { if (mv(s.k) === 'dim' && after) dimK = Math.max(dimK, 0.55 * Math.exp(-since * 0.6)); });
    var heat = play ? (u < M ? 0.4 + 0.6 * ease(u / M) : 0.4 + 0.6 * Math.exp(-since * 1.5)) : 0.4, gutter = mv('heat') === 'dim' && after ? Math.exp(-since * 0.8) : 0;
    if (play && after && this.out.makes && u >= M + 0.15) this.made = true;
    var pop = this.made ? (play ? ease((u - M - 0.15) / 0.35) : 1) : 0;
    var o = {all: shk('bench'), win: shk('window'), flask: shk('work'), result: shk('result'), boil: play && u < M + 0.5 ? 1 : 0, pop: pop, winDim: mv('window') === 'dim' && after};
    var flick = quiet ? 0.93 : 0.9 + 0.06 * Math.sin(this.t * 13) + 0.04 * Math.sin(this.t * 29);
    var lights = [{x: 95, y: 318, c: '#ffb36b', k: 0.55 * flick * (1 - dimK)}, {x: 480, y: 348, c: '#ff9a3c', k: 0.38 * heat * (1 - gutter * 0.85) * flick}];
    var at = {window: [790, 142], work: [480, 262], heat: [480, 346], bench: [290, 396], result: [736, 350]};
    SPOTS.forEach(function (s) {
      var ly = L[s.k]; if (!ly) return; var k = 0;
      if (s.k === 'window' || s.k === 'work') k = 0.18 * ly.light;
      if (s.k === 'result' && self.made) k = 0.2 * ly.light;
      if (s.k === 'heat' && play && u < M) k = 0.3 * ly.light * ease(u / M);
      if (after) { var dl = since - (s.k === 'result' ? 0.15 : 0); if (dl >= 0 && (s.k !== 'result' || self.out.makes)) k += ly.light * Math.exp(-dl * 3.5); }
      if (k > 0.02) lights.push({x: at[s.k][0], y: at[s.k][1], c: ly.c2 && after ? mix(ly.c, ly.c2, 0.3) : ly.c, k: k});
    });
    lights.sort(function (a, b) { return b.k - a.k; });
    var top = lights.slice(0, 4), sl = top.slice(0, 3);
    g.setTransform(1, 0, 0, 1, 0, 0); g.drawImage(this.backdrop(), 0, 0); g.setTransform(this.scale, 0, 0, this.scale, 0, 0);
    this.lightRoom(top);
    g.setTransform(this.scale, 0, 0, this.scale, 0, 0);
    FX.shadowUnder(g, 95 + o.all, 392, 22, 5, sl); FX.shadowUnder(g, 220 + o.all, 392, 56, 8, sl); FX.shadowUnder(g, 480 + o.all, 392, 58, 8, sl); FX.shadowUnder(g, 736 + o.all, 392, 52, 7, sl);
    this.props(o); this.propLight(top);
    g.setTransform(1, 0, 0, 1, 0, 0); g.drawImage(this.pc, 0, 0); g.setTransform(this.scale, 0, 0, this.scale, 0, 0);
    g.globalCompositeOperation = 'lighter';
    var cf = 10 * flick * (1 - dimK * 0.6); g.globalAlpha = 0.9; g.drawImage(sprite('#ffb36b'), 95 + o.all - cf * 2.2, 316 - cf * 2.6, cf * 4.4, cf * 4.4); g.globalAlpha = 1;
    g.fillStyle = '#ffe2a8'; g.beginPath(); g.ellipse(95 + o.all, 316, 3.2, cf * 0.75, 0, 0, TAU); g.fill();
    var fh = 26 * heat * (1 - gutter * 0.7) * flick; g.globalAlpha = 0.85; g.drawImage(sprite('#ff8a2a'), 480 + o.all - fh * 1.4, 358 - fh * 1.9, fh * 2.8, fh * 2.8);
    g.globalAlpha = 1; g.fillStyle = '#ffd27a'; g.beginPath(); g.moveTo(480 + o.all, 358 - fh); g.quadraticCurveTo(480 + o.all + fh * 0.35, 354 - fh * 0.3, 480 + o.all, 362); g.quadraticCurveTo(480 + o.all - fh * 0.35, 354 - fh * 0.3, 480 + o.all, 358 - fh); g.fill();
    g.globalCompositeOperation = 'source-over';
    function trickle(key, ly, x, y, rate, sx, sy) {
      if (!ly) return;
      self.acc[key] = (self.acc[key] || 0) + rate * dt * (quiet ? 0.35 : 1);
      while (self.acc[key] >= 1) { self.acc[key] -= 1; self.burst(ly, x, y, 1, false, sx, sy); }
    }
    trickle('w', L.window, 790 + o.win, 142, 5, 80, 85); trickle('k', L.work, 480 + o.flask, 258, play && u < M ? 12 : 5, 14, 10);
    trickle('f', L.heat, 480, 346, play && u < M ? 14 : 2.5, 6, 6); if (this.made) trickle('r', L.result, 736 + o.result, 350, 5, 10, 16);
    var fire = function (n, t, fn) { if (self.fired[n] || !play || u < t) return; self.fired[n] = true; fn(); };
    fire('moment', M, function () {
      var dly = 0;
      if (L.work) { self.burst(L.work, 480, 258, 34, true); self.rings.push({x: 480, y: 258, age: 0, c: L.work.c}); }
      if (L.heat) self.burst(L.heat, 480, 346, 22, 0.8);
      if (L.bench) { self.burst(L.bench, 290, 398, 32, 0.5, 170, 4); self.rings.push({x: 290, y: 400, age: 0.04, c: L.bench.c, flat: true}); }
      if (L.window) self.burst(L.window, 790, 142, 26, 0.6, 60, 60);
      ['work', 'heat', 'bench', 'window'].forEach(function (k) { if (L[k] && L[k].sound && L[k].sound !== 'none') { FX.Sound.play(L[k].sound, L[k].pitch, dly); dly += 0.07; } });
      if (mv('bench') === 'shake' || mv('work') === 'shake') FX.Sound.play('hiss', 0, 0.02);
    });
    fire('result', M + 0.15, function () {
      if (!self.out.makes) return;
      if (L.result) { self.burst(L.result, 736, 350, 40, 1.2); self.rings.push({x: 736, y: 356, age: 0, c: L.result.c}); if (L.result.sound) FX.Sound.play(L.result.sound, L.result.pitch, 0); }
      FX.Sound.play('bell', 7, 0.12);
    });
    fire('words', WORDS, function () { if (self.out.words) self.words.push({text: self.out.words, x: 430, y: 104, age: 0, c: (L.work || L.result || L.window || {c: '#f6ead2'}).c}); });
    if (play && u > END) this.mode = 'idle';
    this.rings = this.rings.filter(function (r) {
      r.age += dt; if (r.age < 0) return true; var k = r.age / 0.7; if (k > 1) return false;
      var rad = 20 + (r.flat ? 190 : 150) * ease(k), band = g.createRadialGradient(0, 0, rad * 0.3, 0, 0, rad);
      band.addColorStop(0, rgba(r.c, 0)); band.addColorStop(0.6, rgba(r.c, (1 - k) * 0.06)); band.addColorStop(0.82, rgba(r.c, (1 - k) * 0.13)); band.addColorStop(1, rgba(r.c, 0));
      g.globalCompositeOperation = 'lighter'; g.save(); g.translate(r.x, r.y); g.scale(1, r.flat ? 0.12 : 0.55); g.fillStyle = band; g.beginPath(); g.arc(0, 0, rad, 0, TAU); g.fill(); g.restore(); g.globalCompositeOperation = 'source-over';
      return true;
    });
    var keep = [];
    for (var i = 0; i < this.parts.length; i++) {
      var p = this.parts[i]; p.age += dt; if (p.age >= p.life) continue;
      p.vy += (p.g || 0) * dt; p.vx *= Math.pow(p.drag || 1, dt * 60); p.vy *= Math.pow(p.drag || 1, dt * 60);
      if (p.wob != null) p.vx += Math.sin(p.age * 4 + p.wob) * 8 * dt * 6;
      p.x += p.vx * dt; p.y += p.vy * dt; p.rot += (p.vr || 0) * dt;
      g.globalCompositeOperation = (MOTIFS[p.motif] || {}).blend || 'lighter'; FX.drawParticle(g, p); keep.push(p);
    }
    this.parts = keep; g.globalCompositeOperation = 'source-over';
    if (dimK > 0.01) { g.fillStyle = 'rgba(0,0,0,' + (dimK * 0.6).toFixed(3) + ')'; g.fillRect(0, 0, 960, 540); }
    this.words = this.words.filter(function (w) {
      w.age += dt; var k = w.age / 1.8; if (k > 1) return false;
      var a = k < 0.15 ? k / 0.15 : 1 - Math.max(0, (k - 0.6) / 0.4), y = w.y - (quiet ? 0 : 22 * ease(k));
      g.globalAlpha = a; g.font = '30px "Crafting Fell", Georgia, serif'; g.textAlign = 'center';
      g.shadowColor = 'rgba(0,0,0,.9)'; g.shadowBlur = 10; g.fillStyle = 'rgba(0,0,0,.001)'; g.fillText(w.text, w.x, y);
      g.shadowColor = rgba(w.c, 0.95); g.shadowBlur = 18; g.fillStyle = mix(w.c, '#ffffff', 0.45); g.fillText(w.text, w.x, y);
      g.shadowBlur = 0; g.globalAlpha = 1; return true;
    });
    if (this.onTick) this.onTick(this);
  };

  window.CraftRulesStage = {Stage: Stage, SPOTS: SPOTS, BREW: BREW, WORDS: WORDS, END: END};
})();
