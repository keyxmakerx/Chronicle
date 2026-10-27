/**
 * item_look_fx.js -- the drawing, particle and sound engine behind the
 * item_look widget (item_look.js).
 *
 * A look is light, particles, motion, sound and words, each from a fixed
 * list, so a saved look can never run code. LOOKS, TINTS and SOUNDS mirror the
 * sets in internal/plugins/armory/item_look_model.go;
 * test/js/item_look.test.mjs fails when the two drift. Sounds are synthesised
 * with Web Audio, so there are no sound files to ship, and play only after the
 * viewer turns them on.
 *
 * Light behaves like light: each glow is a light that tints the room in its
 * colour, lights the side of the target that faces it, and gives every
 * object a contact shadow and a cast shadow that falls away from it. Effects
 * can also sit on spots (the item, its trail, the point of impact, the
 * target's body, the ground), the way games attach effects to sockets; the
 * crafting bench uses the same engine with its own spots.
 *
 * Everything above the Stage is pure (no DOM at load), so the library,
 * resolve(), the spot rules and the shadow maths are unit-tested in Node. html[data-motion] "calm" quietens the
 * idle trickle and drops travel; "off" draws a still picture until someone
 * presses a moment.
 */
(function () {
'use strict';
var TAU = Math.PI * 2;
function rand(a, b) { return a + Math.random() * (b - a); }
function clamp(v, a, b) { return v < a ? a : v > b ? b : v; }
function ease(t) { t = clamp(t, 0, 1); return 1 - Math.pow(1 - t, 3); }
function easeIn(t) { t = clamp(t, 0, 1); return t * t * t; }
function hex2rgb(h) { h = h.replace('#', ''); if (h.length === 3) h = h.replace(/./g, '$&$&'); var n = parseInt(h, 16); return [n >> 16 & 255, n >> 8 & 255, n & 255]; }
function rgba(h, a) { var c = hex2rgb(h); return 'rgba(' + c[0] + ',' + c[1] + ',' + c[2] + ',' + clamp(a, 0, 1).toFixed(3) + ')'; }
function mix(h1, h2, t) { var a = hex2rgb(h1), b = hex2rgb(h2); return '#' + [0, 1, 2].map(function (i) { return ('0' + Math.round(a[i] + (b[i] - a[i]) * t).toString(16)).slice(-2); }).join(''); }
function motionLevel() {
  var d = typeof document !== 'undefined' && document.documentElement;
  return (d && d.getAttribute('data-motion')) || 'full';
}
function calm() { return motionLevel() !== 'full'; }

/* Soft round sprites, one per colour, so glows cost a drawImage. The falloff
   is long and gentle, so a glow reads as light, never as an outline. */
var spriteCache = {};
function sprite(hex) {
  if (spriteCache[hex]) return spriteCache[hex];
  var c = document.createElement('canvas'); c.width = c.height = 64;
  var g = c.getContext('2d'), gr = g.createRadialGradient(32, 32, 0, 32, 32, 32);
  [[0, 1], [0.12, 0.78], [0.28, 0.42], [0.45, 0.19], [0.62, 0.07], [0.8, 0.02], [1, 0]].forEach(function (s) { gr.addColorStop(s[0], rgba(hex, s[1])); });
  g.fillStyle = gr; g.fillRect(0, 0, 64, 64);
  return (spriteCache[hex] = c);
}

/* ---------- Particles: the fixed motif list ---------- */
var MOTIFS = {
  embers:  { blend: 'lighter', make: function (p, o) { p.vx = rand(-14, 14); p.vy = rand(-60, -22); p.life = rand(0.7, 1.5); p.size = rand(1.4, 3); p.g = -10; } },
  frost:   { blend: 'lighter', make: function (p) { p.vx = rand(-10, 10); p.vy = rand(-14, 10); p.life = rand(1, 2); p.size = rand(3, 6); p.rot = rand(0, TAU); p.vr = rand(-1, 1); p.g = 6; } },
  sparks:  { blend: 'lighter', make: function (p) { var a = rand(0, TAU), s = rand(80, 220); p.vx = Math.cos(a) * s; p.vy = Math.sin(a) * s; p.life = rand(0.15, 0.4); p.size = rand(1, 1.8); p.drag = 0.9; p.g = 0; } },
  motes:   { blend: 'lighter', make: function (p) { p.vx = rand(-8, 8); p.vy = rand(-26, -8); p.life = rand(1.2, 2.4); p.size = rand(4, 9); p.g = -2; p.wob = rand(0, TAU); } },
  smoke:   { blend: 'source-over', make: function (p) { p.vx = rand(-10, 10); p.vy = rand(-26, -10); p.life = rand(1.4, 2.6); p.size = rand(10, 18); p.grow = rand(10, 22); p.g = -4; } },
  petals:  { blend: 'source-over', make: function (p) { p.vx = rand(-18, 18); p.vy = rand(-30, -6); p.life = rand(1.4, 2.4); p.size = rand(3, 5.5); p.rot = rand(0, TAU); p.vr = rand(-3, 3); p.g = 22; p.wob = rand(0, TAU); } },
  bubbles: { blend: 'lighter', make: function (p) { p.vx = rand(-6, 6); p.vy = rand(-40, -16); p.life = rand(0.8, 1.6); p.size = rand(2, 5); p.g = -6; p.wob = rand(0, TAU); } },
  shards:  { blend: 'lighter', make: function (p) { p.vx = rand(-24, 24); p.vy = rand(-30, 6); p.life = rand(0.9, 1.7); p.size = rand(3, 6); p.rot = rand(0, TAU); p.vr = rand(-4, 4); p.g = 30; } },
  glints:  { blend: 'lighter', make: function (p) { p.vx = 0; p.vy = 0; p.life = rand(0.5, 0.9); p.size = rand(5, 9); p.g = 0; } }
};
var MOTIF_NAMES = { embers: 'Embers', frost: 'Frost', sparks: 'Sparks', motes: 'Motes of light', smoke: 'Smoke', petals: 'Petals', bubbles: 'Bubbles', shards: 'Glass shards', glints: 'Glints', none: 'None' };

function drawParticle(g, p) {
  var a = 1 - p.age / p.life; if (a <= 0) return;
  var k = p.motif;
  if (k === 'embers') {
    var col = mix(p.c, p.c2 || '#ffffff', p.age / p.life * 0.6), f = 0.7 + 0.3 * Math.sin(p.age * 40 + p.seed);
    g.globalAlpha = a * 0.5 * f; g.drawImage(sprite(col), p.x - p.size * 4, p.y - p.size * 4, p.size * 8, p.size * 8);
    g.globalAlpha = a * f; g.fillStyle = mix(col, '#ffffff', 0.5); g.beginPath(); g.arc(p.x, p.y, p.size * 0.6, 0, TAU); g.fill();
  } else if (k === 'frost') {
    var tw = 0.55 + 0.45 * Math.sin(p.age * 9 + p.seed);
    g.save(); g.translate(p.x, p.y); g.rotate(p.rot); g.globalAlpha = a * tw; g.strokeStyle = mix(p.c, '#ffffff', 0.4); g.lineWidth = 1;
    g.beginPath(); for (var i = 0; i < 6; i++) { var an = i * TAU / 6; g.moveTo(0, 0); g.lineTo(Math.cos(an) * p.size, Math.sin(an) * p.size); var bx = Math.cos(an) * p.size * 0.55, by = Math.sin(an) * p.size * 0.55; g.moveTo(bx, by); g.lineTo(bx + Math.cos(an + 0.7) * p.size * 0.3, by + Math.sin(an + 0.7) * p.size * 0.3); } g.stroke();
    g.globalAlpha = a * tw * 0.5; g.drawImage(sprite(p.c), -p.size * 1.6, -p.size * 1.6, p.size * 3.2, p.size * 3.2); g.restore();
  } else if (k === 'sparks') {
    g.globalAlpha = a; g.strokeStyle = mix(p.c, '#ffffff', 0.55); g.lineWidth = p.size; g.lineCap = 'round';
    g.beginPath(); g.moveTo(p.x, p.y); g.lineTo(p.x - p.vx * 0.04, p.y - p.vy * 0.04); g.stroke();
    g.globalAlpha = a * 0.4; g.drawImage(sprite(p.c), p.x - 8, p.y - 8, 16, 16);
  } else if (k === 'motes') {
    var s = p.size * (0.8 + 0.2 * Math.sin(p.age * 3 + p.seed)), aa = Math.min(a, p.age * 3);
    g.globalAlpha = aa * 0.7; g.drawImage(sprite(p.c), p.x - s * 2, p.y - s * 2, s * 4, s * 4);
    g.globalAlpha = aa; g.fillStyle = mix(p.c, '#ffffff', 0.6); g.beginPath(); g.arc(p.x, p.y, s * 0.18, 0, TAU); g.fill();
  } else if (k === 'smoke') {
    var r = p.size + p.grow * (p.age / p.life);
    g.globalAlpha = a * Math.min(1, p.age * 2) * 0.5; g.drawImage(sprite(p.c), p.x - r * 2, p.y - r * 2, r * 4, r * 4);
    if (p.c2) { g.globalCompositeOperation = 'lighter'; g.globalAlpha = a * 0.25 * (0.5 + 0.5 * Math.sin(p.age * 6 + p.seed)); g.drawImage(sprite(p.c2), p.x - r * 0.5, p.y - r * 0.5, r, r); g.globalCompositeOperation = 'source-over'; }
  } else if (k === 'petals') {
    g.save(); g.translate(p.x, p.y); g.rotate(p.rot); g.globalAlpha = a * 0.9; g.scale(1, 0.45 + 0.4 * Math.abs(Math.sin(p.age * 4 + p.seed)));
    g.fillStyle = p.c; g.beginPath(); g.ellipse(0, 0, p.size, p.size * 0.55, 0, 0, TAU); g.fill();
    g.fillStyle = rgba('#ffffff', 0.35); g.beginPath(); g.ellipse(-p.size * 0.3, -p.size * 0.1, p.size * 0.4, p.size * 0.18, 0, 0, TAU); g.fill(); g.restore();
  } else if (k === 'bubbles') {
    g.globalAlpha = a * 0.85; g.strokeStyle = mix(p.c, '#ffffff', 0.3); g.lineWidth = 1; g.beginPath(); g.arc(p.x, p.y, p.size, 0, TAU); g.stroke();
    g.globalAlpha = a * 0.6; g.fillStyle = '#ffffff'; g.beginPath(); g.arc(p.x - p.size * 0.35, p.y - p.size * 0.35, p.size * 0.22, 0, TAU); g.fill();
  } else if (k === 'shards') {
    g.save(); g.translate(p.x, p.y); g.rotate(p.rot); g.globalAlpha = a * (0.5 + 0.5 * Math.abs(Math.sin(p.age * 7 + p.seed)));
    g.fillStyle = rgba(mix(p.c, '#ffffff', 0.5), 0.8); g.beginPath(); g.moveTo(0, -p.size); g.lineTo(p.size * 0.5, p.size * 0.6); g.lineTo(-p.size * 0.45, p.size * 0.4); g.closePath(); g.fill(); g.restore();
  } else if (k === 'glints') {
    var gl = Math.sin(Math.PI * p.age / p.life), s2 = p.size * gl;
    g.globalAlpha = gl; g.strokeStyle = mix(p.c, '#ffffff', 0.6); g.lineWidth = 1.2;
    g.beginPath(); g.moveTo(p.x - s2, p.y); g.lineTo(p.x + s2, p.y); g.moveTo(p.x, p.y - s2); g.lineTo(p.x, p.y + s2); g.stroke();
    g.globalAlpha = gl * 0.6; g.drawImage(sprite(p.c), p.x - s2, p.y - s2, s2 * 2, s2 * 2);
  }
  g.globalAlpha = 1;
}

/* ---------- Sound: synthesised, so there are no files to ship ---------- */
var Sound = { on: false, ctx: null, out: null, noise: null };
Sound.ensure = function () {
  if (this.ctx) return this.ctx;
  var AC = window.AudioContext || window.webkitAudioContext; if (!AC) return null;
  var c = this.ctx = new AC();
  var comp = c.createDynamicsCompressor(); comp.connect(c.destination);
  this.out = c.createGain(); this.out.gain.value = 0.55; this.out.connect(comp);
  var b = c.createBuffer(1, c.sampleRate, c.sampleRate), d = b.getChannelData(0);
  for (var i = 0; i < d.length; i++) d[i] = Math.random() * 2 - 1;
  this.noise = b; return c;
};
function env(c, node, t, a, peak, d) { var gn = c.createGain(); gn.gain.setValueAtTime(0.0001, t); gn.gain.exponentialRampToValueAtTime(peak, t + a); gn.gain.exponentialRampToValueAtTime(0.0001, t + a + d); node.connect(gn); gn.connect(Sound.out); return gn; }
function tone(c, type, f, t, a, peak, d, f2) { var o = c.createOscillator(); o.type = type; o.frequency.setValueAtTime(f, t); if (f2) o.frequency.exponentialRampToValueAtTime(f2, t + a + d); env(c, o, t, a, peak, d); o.start(t); o.stop(t + a + d + 0.05); }
function noise(c, t, a, peak, d, ftype, f, q, f2) { var s = c.createBufferSource(); s.buffer = Sound.noise; s.loop = true; var fl = c.createBiquadFilter(); fl.type = ftype; fl.frequency.setValueAtTime(f, t); if (f2) fl.frequency.exponentialRampToValueAtTime(f2, t + a + d); fl.Q.value = q || 1; s.connect(fl); env(c, fl, t, a, peak, d); s.start(t, Math.random() * 0.5); s.stop(t + a + d + 0.05); }
var SOUNDS = {
  crackle: function (c, t) { noise(c, t, 0.08, 0.25, 0.5, 'bandpass', 380, 0.7, 1400); for (var i = 0; i < 16; i++) noise(c, t + rand(0, 0.55), 0.002, rand(0.15, 0.5), rand(0.01, 0.03), 'bandpass', rand(1800, 5200), 3); },
  ping:    function (c, t, p) { var m = Math.pow(2, p / 12); [1760, 2349, 2637].forEach(function (f, i) { tone(c, 'sine', f * m, t + i * 0.055, 0.003, 0.18, 0.45); tone(c, 'sine', f * m * 2.01, t + i * 0.055, 0.003, 0.04, 0.2); }); noise(c, t, 0.002, 0.12, 0.05, 'highpass', 6000); },
  zap:     function (c, t) { tone(c, 'sawtooth', 1400, t, 0.005, 0.14, 0.28, 70); noise(c, t, 0.003, 0.35, 0.12, 'highpass', 3000); noise(c, t + 0.08, 0.003, 0.2, 0.1, 'bandpass', 2500, 2); },
  chime:   function (c, t, p) { var f = 784 * Math.pow(2, p / 12); [1, 2.76, 5.4].forEach(function (r, i) { tone(c, 'sine', f * r, t, 0.004, [0.2, 0.06, 0.03][i], 1.4 - i * 0.4); }); tone(c, 'sine', f * 1.5, t + 0.12, 0.004, 0.1, 1.1); },
  bell:    function (c, t, p) { var f = 523 * Math.pow(2, p / 12); [1, 2, 2.4, 3, 4.2].forEach(function (r, i) { tone(c, 'sine', f * r, t, 0.003, [0.22, 0.1, 0.07, 0.05, 0.03][i], 2.2 - i * 0.3); }); },
  thrum:   function (c, t) { tone(c, 'sine', 55, t, 0.15, 0.4, 1.3); tone(c, 'sine', 58.3, t, 0.15, 0.3, 1.3); tone(c, 'triangle', 110, t, 0.1, 0.08, 1); noise(c, t, 0.3, 0.06, 0.9, 'lowpass', 300); },
  bubble:  function (c, t) { for (var i = 0; i < 7; i++) { var s = rand(260, 520); tone(c, 'sine', s, t + rand(0, 0.5), 0.004, 0.12, 0.07, s * 2.4); } },
  hiss:    function (c, t) { noise(c, t, 0.05, 0.16, 0.55, 'highpass', 5200); },
  swish:   function (c, t) { noise(c, t, 0.06, 0.18, 0.18, 'bandpass', 500, 0.9, 2400); },
  note:    function (c, t, p) { var f = 440 * Math.pow(2, p / 12); tone(c, 'triangle', f, t, 0.01, 0.16, 1.2); tone(c, 'sine', f * 2, t, 0.01, 0.05, 0.8); }
};
var SOUND_NAMES = { crackle: 'Crackle', ping: 'Ice ping', zap: 'Zap', chime: 'Chime', bell: 'Bell', thrum: 'Low thrum', bubble: 'Bubbling', hiss: 'Hiss', none: 'Silent' };
Sound.play = function (kind, pitch, delay) {
  if (!this.on || !kind || kind === 'none' || !SOUNDS[kind]) return;
  var c = this.ensure(); if (!c) return;
  if (c.state === 'suspended') c.resume();
  SOUNDS[kind](c, c.currentTime + 0.02 + (delay || 0), pitch || 0);
};

/* ---------- Drawing the things ---------- */
function drawThing(g, thing, look, t) {
  var col = (look && look.color) || '#cfd6e4';
  if (thing.kind === 'blade') {
    var len = thing.long ? 1.35 : 1;
    g.save(); g.scale(1, len);
    var bl = g.createLinearGradient(-9, 0, 9, 0); bl.addColorStop(0, '#59616e'); bl.addColorStop(0.45, '#e8edf3'); bl.addColorStop(0.55, '#aeb7c4'); bl.addColorStop(1, '#4b525d');
    g.fillStyle = bl; g.beginPath(); g.moveTo(0, -72); g.lineTo(8.5, -14); g.lineTo(-8.5, -14); g.closePath(); g.fill();
    g.globalAlpha = 0.35; g.fillStyle = col; g.fill(); g.globalAlpha = 1;
    g.strokeStyle = 'rgba(255,255,255,.55)'; g.lineWidth = 0.8; g.beginPath(); g.moveTo(0, -66); g.lineTo(0, -18); g.stroke();
    g.restore();
    var gd = g.createLinearGradient(0, -16, 0, -8); gd.addColorStop(0, '#e8c27a'); gd.addColorStop(1, '#8a6326');
    g.fillStyle = gd; roundRect(g, -24, -16, 48, 7, 3); g.fill();
    g.fillStyle = '#3b2a20'; roundRect(g, -4.5, -9, 9, 28, 2); g.fill();
    g.strokeStyle = 'rgba(0,0,0,.45)'; g.lineWidth = 1; for (var y = -6; y < 18; y += 4) { g.beginPath(); g.moveTo(-4.5, y); g.lineTo(4.5, y + 2); g.stroke(); }
    var pm = g.createRadialGradient(-2, 21, 1, 0, 23, 7); pm.addColorStop(0, '#ffe7ad'); pm.addColorStop(1, '#8a6326');
    g.fillStyle = pm; g.beginPath(); g.arc(0, 23, 6.5, 0, TAU); g.fill();
  } else if (thing.kind === 'flask') {
    g.save(); g.beginPath(); g.arc(0, 12, 27, 0, TAU); g.clip();
    g.fillStyle = 'rgba(255,255,255,.06)'; g.fillRect(-30, -20, 60, 60);
    var lq = g.createLinearGradient(0, 4, 0, 40); lq.addColorStop(0, mix(col, '#ffffff', 0.25)); lq.addColorStop(1, mix(col, '#000000', 0.45));
    var wob = Math.sin(t * 2.2) * 2.5;
    g.fillStyle = lq; g.beginPath(); g.moveTo(-30, 6 + wob); g.quadraticCurveTo(0, 2 - wob, 30, 6 - wob); g.lineTo(30, 42); g.lineTo(-30, 42); g.closePath(); g.fill();
    g.restore();
    g.fillStyle = 'rgba(210,225,235,.10)'; g.fillRect(-7, -32, 14, 22);
    g.strokeStyle = 'rgba(225,235,245,.6)'; g.lineWidth = 1.6;
    g.beginPath(); g.moveTo(-7, -30); g.lineTo(-7, -14); g.arc(0, 12, 27, -Math.PI / 2 - 0.27, -Math.PI / 2 + 0.27 + TAU, false); g.stroke();
    g.beginPath(); g.moveTo(7, -30); g.lineTo(7, -14); g.stroke();
    g.fillStyle = '#9b7550'; roundRect(g, -8, -40, 16, 11, 3); g.fill();
    g.strokeStyle = 'rgba(255,255,255,.5)'; g.lineWidth = 2.5; g.beginPath(); g.arc(0, 12, 20, Math.PI * 1.1, Math.PI * 1.35); g.stroke();
  } else if (thing.kind === 'ring') {
    var rg = g.createLinearGradient(-26, 0, 26, 0); rg.addColorStop(0, '#7a5a22'); rg.addColorStop(0.35, '#f3d48a'); rg.addColorStop(0.6, '#b9913f'); rg.addColorStop(1, '#5d4317');
    g.strokeStyle = rg; g.lineWidth = 8; g.beginPath(); g.ellipse(0, 8, 24, 24, 0, 0, TAU); g.stroke();
    g.strokeStyle = 'rgba(255,240,200,.5)'; g.lineWidth = 1.5; g.beginPath(); g.ellipse(0, 8, 27, 27, 0, Math.PI * 1.15, Math.PI * 1.55); g.stroke();
    g.fillStyle = '#c9a24c'; g.beginPath(); g.moveTo(-10, -14); g.lineTo(10, -14); g.lineTo(6, -20); g.lineTo(-6, -20); g.closePath(); g.fill();
    var gm = g.createLinearGradient(0, -36, 0, -14); gm.addColorStop(0, mix(col, '#ffffff', 0.6)); gm.addColorStop(1, mix(col, '#000000', 0.35));
    g.fillStyle = gm; g.beginPath(); g.moveTo(0, -38); g.lineTo(11, -26); g.lineTo(0, -16); g.lineTo(-11, -26); g.closePath(); g.fill();
    g.strokeStyle = 'rgba(255,255,255,.7)'; g.lineWidth = 1; g.beginPath(); g.moveTo(-11, -26); g.lineTo(11, -26); g.moveTo(0, -38); g.lineTo(-4, -26); g.lineTo(0, -16); g.stroke();
  } else if (thing.kind === 'picture' && thing.img && thing.img.complete && thing.img.naturalWidth) {
    /* The item's own picture, framed as a round token. */
    g.save(); g.beginPath(); g.arc(0, 0, 34, 0, TAU); g.clip();
    var im = thing.img, sz = Math.max(68 / im.naturalWidth, 68 / im.naturalHeight);
    g.drawImage(im, -im.naturalWidth * sz / 2, -im.naturalHeight * sz / 2, im.naturalWidth * sz, im.naturalHeight * sz); g.restore();
    g.strokeStyle = mix(col, '#ffffff', 0.4); g.lineWidth = 2.5; g.beginPath(); g.arc(0, 0, 35, 0, TAU); g.stroke();
  } else {
    var sc = col;
    g.save(); g.rotate(t * 0.6);
    g.globalAlpha = 0.9; g.strokeStyle = mix(sc, '#ffffff', 0.35); g.lineWidth = 1.6; g.beginPath(); g.arc(0, 0, 32, 0, TAU); g.stroke();
    g.lineWidth = 0.8; g.beginPath(); g.arc(0, 0, 25, 0, TAU); g.stroke();
    g.strokeStyle = mix(sc, '#ffffff', 0.5); g.lineWidth = 1.2; g.beginPath();
    for (var i = 0; i < 24; i++) { var ta = i * TAU / 24, r0 = i % 2 ? 26.5 : 25.5; g.moveTo(Math.cos(ta) * r0, Math.sin(ta) * r0); g.lineTo(Math.cos(ta) * 30.5, Math.sin(ta) * 30.5); }
    g.stroke();
    g.restore();
    g.save(); g.rotate(-t * 0.9); g.strokeStyle = mix(sc, '#ffffff', 0.2); g.lineWidth = 1.3; g.beginPath();
    for (var k = 0; k < 3; k++) { var a0 = k * TAU / 3 - Math.PI / 2; g.moveTo(Math.cos(a0) * 22, Math.sin(a0) * 22); g.lineTo(Math.cos(a0 + TAU / 3) * 22, Math.sin(a0 + TAU / 3) * 22); }
    g.stroke(); g.restore();
    g.globalAlpha = 0.9; g.drawImage(sprite(sc), -12, -12, 24, 24); g.globalAlpha = 1;
  }
}
function roundRect(g, x, y, w, h, r) { g.beginPath(); g.moveTo(x + r, y); g.arcTo(x + w, y, x + w, y + h, r); g.arcTo(x + w, y + h, x, y + h, r); g.arcTo(x, y + h, x, y, r); g.arcTo(x, y, x + w, y, r); g.closePath(); }

/* The figure on the right: a training dummy for attacks, a friend for healing.
   Drawn unlit; litTarget adds the light and shade. */
function drawTarget(g, kind) {
  g.save();
  if (kind === 'ally') {
    g.fillStyle = '#17110d';
    g.beginPath(); g.arc(0, -96, 15, 0, TAU); g.fill();
    g.beginPath(); g.moveTo(-30, -70); g.quadraticCurveTo(0, -84, 30, -70); g.lineTo(36, -18); g.quadraticCurveTo(30, -10, 22, -14); g.lineTo(18, 30); g.lineTo(-18, 30); g.lineTo(-22, -14); g.quadraticCurveTo(-30, -10, -36, -18); g.closePath(); g.fill();
    g.fillRect(-16, 26, 13, 40); g.fillRect(3, 26, 13, 40);
  } else {
    g.fillStyle = '#4a3423'; g.fillRect(-4, -40, 8, 106);
    g.fillStyle = '#5a3f28'; g.fillRect(-40, -62, 80, 8);
    var sk = g.createLinearGradient(-26, 0, 26, 0); sk.addColorStop(0, '#4d3d29'); sk.addColorStop(0.5, '#7a6544'); sk.addColorStop(1, '#3d3020');
    g.fillStyle = sk; roundRect(g, -25, -74, 50, 72, 16); g.fill();
    g.beginPath(); g.arc(0, -92, 15, 0, TAU); g.fill();
    g.strokeStyle = 'rgba(0,0,0,.35)'; g.lineWidth = 1.2; g.beginPath(); g.moveTo(-24, -40); g.lineTo(24, -40); g.moveTo(-23, -20); g.lineTo(23, -20); g.stroke();
    g.strokeStyle = 'rgba(160,40,30,.55)'; g.lineWidth = 2; g.beginPath(); g.arc(0, -42, 8, 0, TAU); g.stroke();
  }
  g.restore();
}

/* ---------- Shadows ---------- */
/* castShadow says where an object at ground x ox, w wide, throws its shadow
   from one light: away from the light, longer for a brighter light, fainter
   for a farther one. Null when the light is too weak to cast one. Pure. */
function castShadow(ox, w, light) {
  var dx = ox - light.x, dist = Math.max(80, Math.abs(dx)), dir = dx === 0 ? 1 : (dx > 0 ? 1 : -1);
  var str = clamp(light.k * 160 / dist, 0, 0.6); if (str < 0.02) return null;
  var len = w * (0.6 + 1.6 * clamp(light.k, 0, 1));
  return { cx: ox + dir * len * 0.55, len: len, alpha: 0.38 * str };
}
/* A dark contact patch where the object meets the ground (ambient
   occlusion), a soft falloff around it, and one cast shadow per light. */
function shadowUnder(g, ox, oy, w, h, lights) {
  g.fillStyle = 'rgba(0,0,0,.55)'; g.beginPath(); g.ellipse(ox, oy, w * 0.55, h * 0.7, 0, 0, TAU); g.fill();
  var ao = g.createRadialGradient(ox, oy, 0, ox, oy, w * 1.25); ao.addColorStop(0, 'rgba(0,0,0,.42)'); ao.addColorStop(1, 'rgba(0,0,0,0)');
  g.save(); g.translate(ox, oy); g.scale(1, h / w * 1.4); g.translate(-ox, -oy); g.fillStyle = ao; g.beginPath(); g.arc(ox, oy, w * 1.25, 0, TAU); g.fill(); g.restore();
  (lights || []).forEach(function (li) {
    var s = castShadow(ox, w, li); if (!s) return;
    var cs = g.createRadialGradient(s.cx, oy, 0, s.cx, oy, s.len); cs.addColorStop(0, 'rgba(0,0,0,' + s.alpha.toFixed(3) + ')'); cs.addColorStop(1, 'rgba(0,0,0,0)');
    g.save(); g.translate(s.cx, oy); g.scale(1, h / s.len * 1.1); g.translate(-s.cx, -oy); g.fillStyle = cs; g.beginPath(); g.arc(s.cx, oy, s.len, 0, TAU); g.fill(); g.restore();
  });
}

/* ---------- Stage ---------- */
var stages = [];
function Stage(canvas, opts) {
  this.cv = canvas; this.g = canvas.getContext('2d'); this.opts = opts || {};
  this.W = 960; this.H = 540; this.parts = []; this.words = []; this.flashes = []; this.rings = [];
  this.mode = 'idle'; this.mt = 0; this.t = 0; this.emitAcc = {}; this.fired = {};
  this.thing = null; this.look = null; this.onTick = null;
  this.resize(); stages.push(this); start();
  var self = this; if (window.ResizeObserver) { this.ro = new ResizeObserver(function () { self.resize(); }); this.ro.observe(canvas); }
}
Stage.prototype.destroy = function () { if (this.ro) this.ro.disconnect(); var i = stages.indexOf(this); if (i >= 0) stages.splice(i, 1); };
Stage.prototype.resize = function () {
  var r = this.cv.getBoundingClientRect(), d = Math.min(window.devicePixelRatio || 1, 2);
  if (!r.width) return;
  this.cv.width = Math.round(r.width * d); this.cv.height = Math.round(r.width * d * this.H / this.W);
  this.scale = this.cv.width / this.W; this.bg = null; this.still = false;
};
Stage.prototype.set = function (thing, look) { this.thing = thing; this.look = look; this.still = false; };
Stage.prototype.anchor = function () { return { x: 270, y: 290 }; };
Stage.prototype.targetPos = function () { return { x: 720, y: 300 }; };
Stage.prototype.play = function (mode) {
  this.mode = mode; this.mt = 0; this.fired = {}; this.words = []; this.rings = []; this.still = false;
  if (mode === 'use') Sound.play('swish', 0, 0);
};
Stage.prototype.layers = function () {
  var L = this.look || {}, out = [];
  if (L.motif && L.motif !== 'none') out.push({ motif: L.motif, c: L.color, c2: L.color2, amount: L.amount == null ? 1 : L.amount, sound: L.sound, pitch: L.pitch });
  else out.push({ motif: 'none', c: L.color, amount: 0, sound: L.sound, pitch: L.pitch });
  (L.extra || []).forEach(function (e) { out.push({ motif: e.motif, c: e.color, c2: e.color2, amount: e.amount == null ? 0.6 : e.amount, sound: e.sound, pitch: e.pitch }); });
  return out;
};
Stage.prototype.spawn = function (layer, x, y, n, burst) {
  if (!layer.motif || layer.motif === 'none' || !MOTIFS[layer.motif]) return;
  var M = MOTIFS[layer.motif], tempo = (this.look && this.look.tempo) || 1;
  for (var i = 0; i < n; i++) {
    var p = { motif: layer.motif, c: layer.c || '#ffffff', c2: layer.c2, x: x + rand(-14, 14), y: y + rand(-14, 14), age: 0, seed: rand(0, 100), drag: 1, g: 0, rot: 0, vr: 0 };
    M.make(p);
    if (burst) { var a = rand(0, TAU), s = rand(40, 170) * (burst === true ? 1 : burst); p.vx += Math.cos(a) * s; p.vy += Math.sin(a) * s * 0.8; p.drag = 0.93; p.life *= 0.9; }
    p.vx *= tempo; p.vy *= tempo; p.life /= Math.sqrt(tempo);
    if (this.look && this.look.jitter) p.jit = this.look.jitter;
    this.parts.push(p);
  }
  if (this.parts.length > 900) this.parts.splice(0, this.parts.length - 900);
};
/* Where the thing is and how it is posed at a moment of the use or made animation. */
Stage.prototype.timing = function () {
  var k = this.thing ? this.thing.kind : 'blade', tempo = (this.look && this.look.tempo) || 1;
  var impact = ({ blade: 0.42, flask: 0.62, ring: 0.5, sigil: 0.52 }[k] || 0.52) / tempo;
  var T = (this.look && this.look.timing) || {};
  var m = T.motion || 0;
  return { motion: m, impact: m + impact, light: T.light != null ? T.light : m + impact, particles: T.particles != null ? T.particles : m + impact, sound: T.sound != null ? T.sound : m + impact - 0.04, words: T.words != null ? T.words : m + impact + 0.08, end: m + impact + 1.4 };
};
Stage.prototype.pose = function () {
  var A = this.anchor(), Tg = this.targetPos(), k = this.thing ? this.thing.kind : 'blade';
  var bob = calm() ? 0 : Math.sin(this.t * 1.6) * 4;
  var base = { x: A.x, y: A.y + bob, rot: k === 'blade' ? -0.55 : 0, s: 1, proj: null };
  if (this.mode === 'made') { var e = ease(this.mt / 0.55); base.s = 0.2 + 0.8 * e; base.alpha = Math.min(1, this.mt * 3); base.y += (1 - e) * 14; return base; }
  if (this.mode !== 'use') return base;
  var tm = this.timing(), tempo = (this.look && this.look.tempo) || 1, u = (this.mt - tm.motion) * tempo;
  if (u < 0) return base;
  if (k === 'blade') {
    if (u < 0.18) { var w = ease(u / 0.18); base.x -= 30 * w; base.rot -= 0.5 * w; }
    else if (u < 0.42) { var l = easeIn((u - 0.18) / 0.24); base.x = A.x - 30 + (Tg.x - 70 - A.x + 30) * l; base.y = A.y + (Tg.y - 50 - A.y) * l; base.rot = -1.05 + (Math.PI / 2 + 0.05) * l; }
    else if (u < 0.62) { base.x = Tg.x - 70; base.y = Tg.y - 50; base.rot = Math.PI / 2 - 1; }
    else { var r = ease((u - 0.62) / 0.5); base.x = Tg.x - 70 + (A.x - Tg.x + 70) * r; base.y = Tg.y - 50 + (A.y - Tg.y + 50) * r; base.rot = (Math.PI / 2 - 1) * (1 - r) - 0.55 * r; }
  } else if (k === 'flask') {
    var up = ease(u / 0.3), back = u > 1.05 ? ease((u - 1.05) / 0.45) : 0;
    base.x += 120 * up * (1 - back); base.y -= 40 * up * (1 - back); base.rot = (u > 0.3 ? 1.6 * ease((u - 0.3) / 0.25) : 0) * (1 - back);
    base.pour = u > 0.42 && u < 1.0;
  } else {
    var lift = ease(u / 0.3) * (u > 1 ? 1 - ease((u - 1) / 0.4) : 1);
    base.y -= 26 * lift; base.s = 1 + 0.12 * lift; base.spin = lift;
    var tp = (u - 0.3) / 0.2;
    if (tp >= 0 && tp <= 1) { var q = easeIn(tp); base.proj = { x: A.x + (Tg.x - A.x) * q, y: A.y - 26 + (Tg.y - 60 - A.y + 26) * q - Math.sin(q * Math.PI) * 40 }; }
  }
  return base;
};
Stage.prototype.backdrop = function () {
  if (this.bg) return this.bg;
  var c = document.createElement('canvas'); c.width = this.cv.width; c.height = this.cv.height;
  var g = c.getContext('2d'); g.scale(this.scale, this.scale);
  var wall = g.createLinearGradient(0, 0, 0, 400); wall.addColorStop(0, '#15110e'); wall.addColorStop(1, '#241b14');
  g.fillStyle = wall; g.fillRect(0, 0, 960, 540);
  g.globalAlpha = 0.05; g.strokeStyle = '#000'; for (var x = 0; x < 960; x += 46) { g.lineWidth = 2; g.beginPath(); g.moveTo(x + Math.sin(x) * 3, 0); g.lineTo(x, 380); g.stroke(); }
  g.globalAlpha = 1;
  /* A dim back shelf, so the room reads as Hessa's workshop and not a void. */
  var sy = 168; g.fillStyle = '#2c2117'; g.fillRect(60, sy, 840, 9); g.fillStyle = 'rgba(0,0,0,.35)'; g.fillRect(60, sy + 9, 840, 6);
  var jars = [[90,34,18,'#5a3d2a'],[122,46,14,'#2f4a3a'],[150,30,20,'#4a3346'],[196,22,26,'#3b3424'],[236,40,12,'#2b3b52'],[600,38,18,'#3d2a2a'],[632,26,22,'#2d3f33'],[668,48,12,'#4a3b22'],[700,30,18,'#33304a'],[820,36,16,'#3b2b20'],[852,24,22,'#2b3a3f']];
  jars.forEach(function (j) { var x = j[0], h = j[1], w = j[2]; g.fillStyle = j[3]; roundRect(g, x, sy - h, w, h, 4); g.fill(); g.fillStyle = '#5a4430'; g.fillRect(x + w * 0.2, sy - h - 5, w * 0.6, 5); g.fillStyle = 'rgba(255,230,190,.07)'; g.fillRect(x + 3, sy - h + 4, 2, h - 8); });
  for (var b2 = 0; b2 < 7; b2++) { g.fillStyle = ['#4a2a22','#2a3a4a','#3a2f22','#4a3a2a','#2a2a3a','#3d2626','#2c3a2c'][b2]; g.fillRect(380 + b2 * 13, sy - 40 + (b2 % 3) * 4, 11, 40 - (b2 % 3) * 4); }
  var warm = g.createRadialGradient(480, 120, 20, 480, 160, 520); warm.addColorStop(0, 'rgba(255,170,90,.10)'); warm.addColorStop(1, 'rgba(255,170,90,0)'); g.fillStyle = warm; g.fillRect(0, 0, 960, 400);
  var ledge = g.createLinearGradient(0, 392, 0, 540); ledge.addColorStop(0, '#4a3524'); ledge.addColorStop(0.08, '#3a2919'); ledge.addColorStop(1, '#1a120b');
  g.fillStyle = ledge; g.fillRect(0, 392, 960, 148);
  g.fillStyle = 'rgba(255,220,170,.10)'; g.fillRect(0, 392, 960, 2);
  g.globalAlpha = 0.07; g.strokeStyle = '#000'; for (var i = 0; i < 26; i++) { var y = 400 + i * 5.5 + Math.sin(i) * 3; g.beginPath(); g.moveTo(0, y); g.bezierCurveTo(300, y + 4, 640, y - 4, 960, y + 2); g.stroke(); }
  g.globalAlpha = 1;
  var vg = g.createRadialGradient(480, 290, 160, 480, 290, 640); vg.addColorStop(0, 'rgba(0,0,0,0)'); vg.addColorStop(1, 'rgba(0,0,0,.6)');
  g.fillStyle = vg; g.fillRect(0, 0, 960, 540);
  return (this.bg = c);
};
Stage.prototype.frame = function (dt) {
  var g = this.g; if (!this.thing || !this.scale) return;
  this.t += dt; this.mt += dt;
  var L = this.look || {}, layers = this.layers(), A = this.anchor(), Tg = this.targetPos();
  var pose = this.pose(), tm = this.timing(), col = L.color || '#cfd6e4', lc = L.lightColor || col;
  g.setTransform(1, 0, 0, 1, 0, 0); g.drawImage(this.backdrop(), 0, 0);
  g.setTransform(this.scale, 0, 0, this.scale, 0, 0);

  /* Light: a steady glow that flickers, and a flash at the moment the timeline says. */
  var light = (L.light == null ? 1 : L.light), flick = L.flicker || 0;
  var base = 0.32 * light * (1 - flick * 0.35 + flick * 0.35 * (Math.sin(this.t * 11) * 0.5 + Math.sin(this.t * 17.3) * 0.5));
  var flash = 0, tflash = 0;
  if (this.mode === 'use') { var dl = this.mt - tm.light; if (dl >= 0) { flash = Math.exp(-dl * 4.5) * light; } }
  if (this.mode === 'made') { flash = Math.exp(-this.mt * 3) * 1.2 * Math.max(light, 0.6); }
  var at = (this.mode === 'use' && this.mt >= tm.impact) ? Tg : pose;
  g.globalCompositeOperation = 'lighter';
  var rl = g.createRadialGradient(pose.x, pose.y, 10, pose.x, pose.y, 340); rl.addColorStop(0, rgba(lc, base * 0.4)); rl.addColorStop(1, rgba(lc, 0));
  g.fillStyle = rl; g.fillRect(0, 0, 960, 540);
  if (flash > 0.01) { var fl = g.createRadialGradient(at.x, at.y - 30, 10, at.x, at.y - 30, 520); fl.addColorStop(0, rgba(L.color2 || lc, flash * 0.42)); fl.addColorStop(0.4, rgba(lc, flash * 0.25)); fl.addColorStop(1, rgba(lc, 0)); g.fillStyle = fl; g.fillRect(0, 0, 960, 540); }
  g.globalCompositeOperation = 'source-over';

  /* Two lights, the item's own glow and the flash where the effect lands.
     They reach the room's objects in their own colours, and every object
     gets a contact shadow plus a cast shadow away from each light. */
  var lights = [{ x: pose.x, y: pose.y, c: lc, k: base * 1.6 }];
  if (flash > 0.01) lights.push({ x: at.x, y: at.y - 30, c: L.color2 || lc, k: flash });
  this.lightRoom(lights);
  shadowUnder(g, A.x, 392, 58, 8, lights);
  shadowUnder(g, Tg.x, Tg.y + 88, 66, 10, lights);

  var shake = (!calm() && this.mode === 'use' && this.mt >= tm.impact && this.thing.target !== 'ally') ? Math.sin((this.mt - tm.impact) * 60) * 5 * Math.exp(-(this.mt - tm.impact) * 7) : 0;
  this.litTarget(g, Tg.x + shake, Tg.y, lights);
  g.fillStyle = 'rgba(255,240,220,.38)'; g.font = '500 12px Inter, system-ui, sans-serif'; g.textAlign = 'center';
  g.fillText(this.thing.target === 'ally' ? 'An ally' : 'Training dummy', Tg.x, Tg.y + 112);

  /* The thing, with its glow behind it. */
  g.save(); g.translate(pose.x, pose.y); g.rotate(pose.rot + (pose.spin ? pose.spin * Math.sin(this.t * 6) * 0.15 : 0)); g.scale(pose.s * 1.45, pose.s * 1.45);
  if (pose.alpha != null) g.globalAlpha = pose.alpha;
  /* A wide faint glow and a small warm core, rather than one bright disc. */
  var pa = pose.alpha == null ? 1 : pose.alpha;
  g.globalCompositeOperation = 'lighter';
  g.globalAlpha = pa * clamp(base * 0.9 + flash * 0.25, 0, 0.55); g.drawImage(sprite(lc), -120, -130, 240, 240);
  g.globalAlpha = pa * clamp(base * 0.7 + flash * 0.2, 0, 0.4); g.drawImage(sprite(mix(lc, '#ffffff', 0.25)), -48, -58, 96, 96);
  g.globalCompositeOperation = 'source-over';
  g.globalAlpha = pa;
  drawThing(g, this.thing, L, this.t); g.restore(); g.globalAlpha = 1;

  /* Emission: idle trickle, trails while moving, bursts on cue. */
  var self = this, still = calm() ? 0.35 : 1;
  if (L.spots) this.spotFrame(dt, pose, Tg, tm, A, still);
  else {
  layers.forEach(function (ly, i) {
    var rate = 9 * ly.amount * still * (self.mode === 'use' ? 0.6 : 1), key = 'e' + i;
    self.emitAcc[key] = (self.emitAcc[key] || 0) + rate * dt;
    while (self.emitAcc[key] >= 1) { self.emitAcc[key] -= 1; self.spawn(ly, pose.x, pose.y - 10, 1); }
    if (pose.proj) self.spawn(ly, pose.proj.x, pose.proj.y, Math.ceil(3 * ly.amount));
    if (pose.pour) { var ang = pose.rot - Math.PI / 2, mx = pose.x + Math.cos(ang) * 40, my = pose.y + Math.sin(ang) * 40; self.spawn(ly, mx + (Tg.x - mx) * Math.random(), my + (Tg.y - 60 - my) * Math.random(), Math.ceil(1.5 * ly.amount)); }
  });
  if (pose.proj) { g.globalCompositeOperation = 'lighter'; g.drawImage(sprite(L.color2 || col), pose.proj.x - 22, pose.proj.y - 22, 44, 44); g.drawImage(sprite(col), pose.proj.x - 40, pose.proj.y - 40, 80, 80); g.globalCompositeOperation = 'source-over'; }
  if (L.shimmer && Math.random() < dt * 5 * still) this.parts.push({ motif: 'glints', c: L.shimmer, x: pose.x + rand(-26, 26), y: pose.y + rand(-60, 20), age: 0, life: rand(0.5, 0.9), size: rand(5, 10), vx: 0, vy: 0, g: 0, drag: 1, seed: 0 });

  var fire = function (name, at, fn) { if (self.fired[name] || self.mt < at) return; self.fired[name] = true; fn(); };
  if (this.mode === 'use') {
    var burstAt = function () { return self.mt >= tm.impact ? { x: Tg.x, y: Tg.y - 62 } : (pose.proj || { x: pose.x, y: pose.y - 10 }); };
    fire('particles', tm.particles, function () { var b = burstAt(); layers.forEach(function (ly) { self.spawn(ly, b.x, b.y, Math.round(34 * ly.amount), true); }); self.rings.push({ x: b.x, y: b.y, age: 0, c: col }); });
    fire('sound', tm.sound, function () { layers.forEach(function (ly, i) { Sound.play(ly.sound, ly.pitch, i * 0.04); }); });
    fire('words', tm.words, function () { if (L.words) self.words.push({ text: L.words, x: Tg.x + 110, y: Tg.y - 118, age: 0, c: L.color2 || col }); });
    if (this.mt > tm.end) this.mode = 'idle';
  } else if (this.mode === 'made') {
    fire('burst', 0, function () { layers.forEach(function (ly) { self.spawn(ly, A.x, A.y - 10, Math.round(46 * Math.max(ly.amount, 0.5)), 1.3); }); self.rings.push({ x: A.x, y: A.y - 10, age: 0, c: col }); self.rings.push({ x: A.x, y: A.y - 10, age: -0.12, c: L.color2 || col });
      layers.forEach(function (ly, i) { Sound.play(ly.sound, ly.pitch, i * 0.05); }); Sound.play('bell', 7, 0.18); });
    fire('name', 0.35, function () { self.words.push({ text: self.thing.name, x: A.x, y: A.y - 140, age: 0, c: '#f6ead2', big: true }); });
    if (this.mt > 2.2) this.mode = 'idle';
  }
  }

  /* Rings (a soft band of light spreading over the ground), particles, words. */
  this.rings = this.rings.filter(function (r) {
    r.age += dt; if (r.age < 0) return true; var k = r.age / 0.7; if (k > 1) return false;
    var rr = 20 + 150 * ease(k), band = g.createRadialGradient(0, 0, rr * 0.3, 0, 0, rr);
    band.addColorStop(0, rgba(r.c, 0)); band.addColorStop(0.6, rgba(r.c, (1 - k) * 0.06)); band.addColorStop(0.82, rgba(r.c, (1 - k) * 0.13)); band.addColorStop(1, rgba(r.c, 0));
    g.globalCompositeOperation = 'lighter'; g.save(); g.translate(r.x, r.y); g.scale(1, 0.55); g.fillStyle = band; g.beginPath(); g.arc(0, 0, rr, 0, TAU); g.fill(); g.restore();
    g.globalCompositeOperation = 'source-over'; return true;
  });
  var ps = this.parts, keep = [];
  for (var i = 0; i < ps.length; i++) {
    var p = ps[i]; p.age += dt; if (p.age >= p.life) continue;
    p.vy += (p.g || 0) * dt; p.vx *= Math.pow(p.drag || 1, dt * 60); p.vy *= Math.pow(p.drag || 1, dt * 60);
    if (p.wob != null) p.vx += Math.sin(p.age * 4 + p.wob) * 8 * dt * 6;
    if (p.jit) { p.vx += rand(-1, 1) * 160 * p.jit * dt; p.vy += rand(-1, 1) * 160 * p.jit * dt; }
    p.x += p.vx * dt; p.y += p.vy * dt; p.rot += (p.vr || 0) * dt;
    g.globalCompositeOperation = (MOTIFS[p.motif] || {}).blend || 'lighter'; drawParticle(g, p); keep.push(p);
  }
  this.parts = keep; g.globalCompositeOperation = 'source-over';
  this.words = this.words.filter(function (w) { w.age += dt; var k = w.age / 1.6; if (k > 1) return false; var a = k < 0.15 ? k / 0.15 : 1 - Math.max(0, (k - 0.6) / 0.4);
    g.globalAlpha = a; g.font = (w.big ? '28px' : '30px') + ' Georgia, "Times New Roman", serif'; g.textAlign = 'center'; g.shadowColor = 'rgba(0,0,0,.9)'; g.shadowBlur = 10; g.fillStyle = 'rgba(0,0,0,.001)'; g.fillText(w.text, w.x, w.y - 26 * ease(k)); g.shadowColor = rgba(w.c, 0.95); g.shadowBlur = 18; g.fillStyle = mix(w.c, '#ffffff', 0.45);
    g.fillText(w.text, w.x, w.y - 26 * ease(k)); g.shadowBlur = 0; g.globalAlpha = 1; return true; });
  if (this.onTick) this.onTick(this);
};

/* Light landing on the room: the backdrop, tinted by the light's colour,
   added back inside the light's reach, so the shelf and ledge pick up the
   colour of whatever is glowing near them. */
Stage.prototype.lightRoom = function (lights) {
  var bg = this.backdrop(), W = bg.width, H = bg.height, sc = this.scale, g = this.g;
  if (!this.litC || this.litC.width !== W || this.litC.height !== H) { this.litC = document.createElement('canvas'); this.litC.width = W; this.litC.height = H; }
  var lg = this.litC.getContext('2d'), litC = this.litC;
  g.save(); g.setTransform(1, 0, 0, 1, 0, 0);
  lights.forEach(function (li) {
    var k = clamp(li.k, 0, 1.2); if (k < 0.03) return;
    lg.globalCompositeOperation = 'copy'; lg.drawImage(bg, 0, 0);
    lg.globalCompositeOperation = 'multiply'; lg.fillStyle = mix(li.c, '#ffffff', 0.35); lg.fillRect(0, 0, W, H);
    lg.globalCompositeOperation = 'destination-in';
    var m = lg.createRadialGradient(li.x * sc, li.y * sc, 0, li.x * sc, li.y * sc, 380 * sc); m.addColorStop(0, 'rgba(0,0,0,1)'); m.addColorStop(0.35, 'rgba(0,0,0,.55)'); m.addColorStop(1, 'rgba(0,0,0,0)');
    lg.fillStyle = m; lg.fillRect(0, 0, W, H);
    g.globalCompositeOperation = 'lighter'; g.globalAlpha = clamp(k * 0.85, 0, 1); g.drawImage(litC, 0, 0);
  });
  g.restore(); g.globalAlpha = 1; g.globalCompositeOperation = 'source-over';
};
/* The target on its own layer, so light and shade follow its silhouette:
   lit on the side facing each light, shaded on the far side. */
Stage.prototype.litTarget = function (g, x, y, lights) {
  var S = 300, sc = this.scale;
  if (!this.tgtC || this.tgtC.width !== Math.round(S * sc)) { this.tgtC = document.createElement('canvas'); this.tgtC.width = this.tgtC.height = Math.round(S * sc); }
  var c = this.tgtC, t = c.getContext('2d');
  t.setTransform(1, 0, 0, 1, 0, 0); t.globalCompositeOperation = 'source-over'; t.clearRect(0, 0, c.width, c.height);
  t.setTransform(sc, 0, 0, sc, 0, 0); t.save(); t.translate(S / 2, S * 0.62); t.scale(1.3, 1.3); drawTarget(t, this.thing.target); t.restore();
  t.globalCompositeOperation = 'source-atop';
  var ox = x - S / 2, oy = y - S * 0.62;
  lights.forEach(function (li) {
    var k = clamp(li.k, 0, 1.2); if (k < 0.02) return;
    var lx = li.x - ox, ly = li.y - oy, gr = t.createRadialGradient(lx, ly, 0, lx, ly, 260 + 120 * k);
    gr.addColorStop(0, rgba(mix(li.c, '#ffffff', 0.3), 0.75 * k)); gr.addColorStop(0.45, rgba(li.c, 0.35 * k)); gr.addColorStop(1, rgba(li.c, 0));
    t.fillStyle = gr; t.fillRect(0, 0, S, S);
  });
  var main = lights.reduce(function (a, b) { return b.k > a.k ? b : a; }), dir = main.x < x ? 1 : -1;
  var sh = t.createLinearGradient(S / 2 - dir * 40, 0, S / 2 + dir * 60, 0); sh.addColorStop(0, 'rgba(0,0,0,0)'); sh.addColorStop(1, 'rgba(0,0,0,' + (0.18 + 0.25 * clamp(main.k, 0, 1)).toFixed(3) + ')');
  t.fillStyle = sh; t.fillRect(0, 0, S, S);
  g.drawImage(c, ox, oy, S, S);
};
/* Effects placed on spots (look.spots, from spotLayers): the item itself,
   the trail on the way, the point of impact, the target's body afterwards
   and the ground under it. Each spot plays at its own moment. */
Stage.prototype.spotFrame = function (dt, pose, Tg, tm, A, still) {
  var S = this.look.spots, L = this.look, self = this, use = this.mode === 'use', u = this.mt, g = this.g;
  function emit(key, ly, x, y, rate, sx, sy) { self.emitAcc[key] = (self.emitAcc[key] || 0) + rate * dt; while (self.emitAcc[key] >= 1) { self.emitAcc[key] -= 1; self.spawn(ly, x + rand(-sx, sx), y + rand(-sy, sy), 1); } }
  if (S.item) emit('item', S.item, pose.x, pose.y - 10, 9 * S.item.amount * still * (use ? 0.6 : 1), 2, 2);
  var moving = use && u >= tm.motion && u < tm.impact + 0.04, tp = pose.proj || { x: pose.x, y: pose.y - 20 };
  if (moving && S.trail) {
    if (pose.pour) { var ang = pose.rot - Math.PI / 2, mx = pose.x + Math.cos(ang) * 40, my = pose.y + Math.sin(ang) * 40; this.spawn(S.trail, mx + (Tg.x - mx) * Math.random(), my + (Tg.y - 60 - my) * Math.random(), Math.ceil(2 * S.trail.amount)); }
    else this.spawn(S.trail, tp.x, tp.y, Math.ceil(3 * S.trail.amount));
  }
  if (pose.proj) {
    var pc = S.trail || S.impact || S.item || { c: '#e8dcc0' };
    g.globalCompositeOperation = 'lighter'; g.drawImage(sprite(pc.c2 || pc.c), pose.proj.x - 22, pose.proj.y - 22, 44, 44); g.drawImage(sprite(pc.c), pose.proj.x - 40, pose.proj.y - 40, 80, 80); g.globalCompositeOperation = 'source-over';
  }
  var fire = function (name, at, fn) { if (self.fired[name] || self.mt < at) return; self.fired[name] = true; fn(); };
  var gy = Tg.y + 86;
  if (use) {
    fire('impact', tm.impact, function () {
      if (S.impact) { self.spawn(S.impact, Tg.x, Tg.y - 62, Math.round(34 * S.impact.amount), true); self.rings.push({ x: Tg.x, y: Tg.y - 62, age: 0, c: S.impact.c }); }
      if (S.ground) { self.spawn(S.ground, Tg.x, gy, Math.round(20 * S.ground.amount), 0.7); self.rings.push({ x: Tg.x, y: gy, age: 0.05, c: S.ground.c }); }
      var d = 0; SPOTS.forEach(function (k) { if (S[k] && S[k].sound && S[k].sound !== 'none') { Sound.play(S[k].sound, S[k].pitch, d); d += 0.07; } });
    });
    if (S.body && u >= tm.impact && u < tm.impact + 1.5) emit('body', S.body, Tg.x, Tg.y - 40, 28 * S.body.amount * still, 26, 52);
    if (S.ground && u >= tm.impact && u < tm.impact + 1.3) emit('ground', S.ground, Tg.x, gy - 4, 22 * S.ground.amount * still, 70, 5);
    fire('words', tm.words, function () { if (L.words) self.words.push({ text: L.words, x: Tg.x + 110, y: Tg.y - 118, age: 0, c: (S.impact || S.item || { c: '#f6ead2' }).c }); });
    if (u > tm.end + (S.body ? 0.4 : 0)) this.mode = 'idle';
  } else if (this.mode === 'made') {
    var m = S.item || S.impact;
    fire('burst', 0, function () { if (m) { self.spawn(m, A.x, A.y - 10, Math.round(46 * Math.max(m.amount, 0.5)), 1.3); self.rings.push({ x: A.x, y: A.y - 10, age: 0, c: m.c }); Sound.play(m.sound, m.pitch, 0); } Sound.play('bell', 7, 0.18); });
    fire('name', 0.35, function () { self.words.push({ text: self.thing.name, x: A.x, y: A.y - 140, age: 0, c: '#f6ead2', big: true }); });
    if (u > 2.2) this.mode = 'idle';
  }
};

/* Small live tiles for galleries: the motif alone, idling in its own colour. */
function Mini(canvas, look) { this.cv = canvas; this.g = canvas.getContext('2d'); this.look = look; this.parts = []; this.acc = 0; this.t = 0; minis.push(this); start(); }
Mini.prototype.destroy = function () { var i = minis.indexOf(this); if (i >= 0) minis.splice(i, 1); };
var minis = [];
Mini.prototype.frame = function (dt) {
  var cv = this.cv, d = Math.min(window.devicePixelRatio || 1, 2), w = cv.clientWidth, h = cv.clientHeight; if (!w) return;
  if (cv.width !== Math.round(w * d)) { cv.width = Math.round(w * d); cv.height = Math.round(h * d); }
  var g = this.g, L = this.look; g.setTransform(d, 0, 0, d, 0, 0); this.t += dt;
  var bgr = g.createLinearGradient(0, 0, 0, h); bgr.addColorStop(0, '#16110d'); bgr.addColorStop(1, '#2a1f16'); g.fillStyle = bgr; g.fillRect(0, 0, w, h);
  g.globalCompositeOperation = 'lighter'; g.globalAlpha = 0.35 + 0.1 * Math.sin(this.t * 3); g.drawImage(sprite(L.color), w / 2 - 50, h / 2 - 45, 100, 100); g.globalAlpha = 1; g.globalCompositeOperation = 'source-over';
  var self = this; this.acc += dt * (calm() ? 3 : 10) * (L.motif === 'none' ? 0 : (L.amount || 1));
  while (this.acc >= 1) { this.acc--; var M = MOTIFS[L.motif]; if (!M) break; var p = { motif: L.motif, c: L.color, c2: L.color2, x: w / 2 + rand(-16, 16), y: h * 0.62 + rand(-6, 6), age: 0, seed: rand(0, 100), drag: 1, g: 0, rot: 0, vr: 0 }; M.make(p); if (L.motif === 'sparks') { p.vx *= 0.4; p.vy *= 0.4; } self.parts.push(p); }
  (L.extra || []).forEach(function (e) { if (Math.random() < dt * 5) { var M = MOTIFS[e.motif]; if (!M) return; var p = { motif: e.motif, c: e.color, c2: e.color2, x: w / 2 + rand(-16, 16), y: h * 0.62, age: 0, seed: rand(0, 100), drag: 1, g: 0, rot: 0, vr: 0 }; M.make(p); self.parts.push(p); } });
  if (L.shimmer && Math.random() < dt * 4) this.parts.push({ motif: 'glints', c: L.shimmer, x: w / 2 + rand(-24, 24), y: h / 2 + rand(-20, 14), age: 0, life: 0.7, size: 6, vx: 0, vy: 0, g: 0, drag: 1, seed: 0 });
  var keep = [];
  this.parts.forEach(function (p) { p.age += dt; if (p.age >= p.life) return; p.vy += (p.g || 0) * dt; p.vx *= Math.pow(p.drag || 1, dt * 60); p.x += p.vx * dt; p.y += p.vy * dt; p.rot += (p.vr || 0) * dt; g.globalCompositeOperation = (MOTIFS[p.motif] || {}).blend || 'lighter'; drawParticle(g, p); keep.push(p); });
  this.parts = keep.slice(-160); g.globalCompositeOperation = 'source-over';
};

/* One clock for every visible stage and tile; hidden ones sleep, and the
   clock stops when nothing is left. With motion off an idle stage is drawn
   once and then left still. */
var last = 0, running = false;
function start() { if (running || typeof requestAnimationFrame === 'undefined') return; running = true; last = 0; requestAnimationFrame(tick); }
function tick(ts) {
  if (!stages.length && !minis.length) { running = false; return; }
  var dt = last ? Math.min((ts - last) / 1000, 0.05) : 0.016; last = ts;
  var off = motionLevel() === 'off';
  stages.forEach(function (s) {
    if (s.cv.offsetParent === null) return;
    if (off && s.mode === 'idle') { if (s.still) return; s.parts = []; s.words = []; s.rings = []; s.frame(0); s.still = true; return; }
    s.frame(dt);
  });
  minis.forEach(function (m) { if (m.cv.offsetParent !== null && !(off && m.t > 0)) m.frame(off ? 0.5 : dt); });
  requestAnimationFrame(tick);
}

/* ---------- The gallery: the only looks a saved item can name ---------- */
var LOOKS = {
  ember: { name: 'Ember strike', color: '#ff7a2f', color2: '#ffd27a', motif: 'embers', sound: 'crackle', light: 1, flicker: 0.6 },
  frost: { name: 'Frost shard', color: '#8fd8ff', color2: '#effaff', motif: 'frost', sound: 'ping', light: 0.85, flicker: 0.1 },
  storm: { name: 'Storm arc', color: '#a99bff', color2: '#ffffff', motif: 'sparks', sound: 'zap', light: 1.1, flicker: 0.9 },
  bloom: { name: 'Healing bloom', color: '#8fe07a', color2: '#fff0b0', motif: 'motes', sound: 'chime', light: 0.85, flicker: 0.05, heals: true },
  dawn:  { name: 'Dawn light', color: '#ffd66b', color2: '#fff6d8', motif: 'motes', sound: 'bell', light: 1, flicker: 0.1, heals: true },
  rot:   { name: 'Remnant rot', color: '#6d45c4', color2: '#7cf5c8', lightColor: '#8a63e0', motif: 'smoke', sound: 'thrum', light: 0.8, flicker: 0.4 },
  venom: { name: 'Venom', color: '#a6e34a', color2: '#e9ffb0', motif: 'bubbles', sound: 'bubble', light: 0.7, flicker: 0.2 },
  glass: { name: 'Glass and starlight', color: '#cfe6ff', color2: '#ffffff', motif: 'shards', sound: 'ping', pitch: 5, light: 0.7, flicker: 0.15 },
  quiet: { name: 'Quiet glint', color: '#e8dcc0', color2: '#ffffff', motif: 'none', shimmer: '#fff2cc', sound: 'none', light: 0.45, flicker: 0 }
};
var TINTS = ['#ff7a2f', '#ffd66b', '#8fe07a', '#7cf5c8', '#8fd8ff', '#a99bff', '#f59ac0', '#e8dcc0'];
var SOUNDS = ['crackle', 'ping', 'zap', 'chime', 'bell', 'thrum', 'bubble', 'hiss', 'none'];

/* resolve turns a saved look ({look, tint, sound, words}) into what the
   Stage draws, or null when it names no gallery look. Pure. */
function resolve(saved) {
  if (!saved || !Object.prototype.hasOwnProperty.call(LOOKS, saved.look)) return null;
  var base = LOOKS[saved.look], L = {};
  for (var k in base) L[k] = base[k];
  if (saved.tint && TINTS.indexOf(saved.tint) >= 0) { L.color = saved.tint; L.color2 = mix(saved.tint, '#ffffff', 0.6); L.lightColor = saved.tint; if (L.shimmer) L.shimmer = saved.tint; }
  if (saved.sound && SOUNDS.indexOf(saved.sound) >= 0) L.sound = saved.sound;
  L.words = typeof saved.words === 'string' ? saved.words : '';
  return L;
}

/* Spots an effect can be placed on. The item stage uses SPOTS. The crafting
   bench uses BENCH_SPOTS for its moments (the window, the flask, the flame,
   the bench top, what comes out), and a crafting motion adds MOTION_SPOTS,
   which follow the moving tool: the tool, its trail, and where it touches. */
var SPOTS = ['item', 'trail', 'impact', 'body', 'ground'];
var BENCH_SPOTS = ['window', 'flask', 'flame', 'bench', 'result'];
var MOTION_SPOTS = ['tool', 'trail', 'contact'];

/* spotLayers turns saved placements ({spot: {look, tint}}) into the layers
   the stage draws on each spot. Only spots in the given list and looks in
   the gallery survive, so saved data can never name anything else. A look
   with no particles shows as its glints; the first spot (the item itself)
   plays a little softer than the moments. Pure. */
function spotLayers(saved, spots) {
  var out = {}, n = 0, list = spots || SPOTS;
  if (!saved || typeof saved !== 'object') return null;
  list.forEach(function (k) {
    if (!Object.prototype.hasOwnProperty.call(saved, k)) return;
    var L = resolve(saved[k]); if (!L) return;
    var amount = (L.amount == null ? 1 : L.amount) * (k === list[0] ? 0.8 : 1);
    out[k] = L.motif === 'none'
      ? { motif: 'glints', c: L.shimmer || L.color, c2: '#ffffff', amount: 0.5, sound: L.sound, pitch: L.pitch }
      : { motif: L.motif, c: L.color, c2: L.color2, amount: amount, sound: L.sound, pitch: L.pitch };
    n++;
  });
  return n ? out : null;
}

/* kindFor picks how the stage draws and moves an item from its type text:
   blades lunge, flasks pour onto an ally, rings and charms throw, anything
   else shows the item's own picture or a sigil. Pure. */
function kindFor(typeText, hasImage) {
  var t = String(typeText || '').toLowerCase();
  if (/sword|dagger|blade|knife|axe|spear|javelin|mace|hammer|weapon|arrow|bow/.test(t)) return 'blade';
  if (/potion|draught|elixir|flask|vial|tonic|philter|brew/.test(t)) return 'flask';
  if (/ring|amulet|necklace|jewel|gem|charm|talisman|trinket/.test(t)) return 'ring';
  return hasImage ? 'picture' : 'sigil';
}

window.ItemLookFX = { Stage: Stage, Mini: Mini, Sound: Sound, LOOKS: LOOKS, TINTS: TINTS, SOUNDS: SOUNDS, SOUND_NAMES: SOUND_NAMES, MOTIF_NAMES: MOTIF_NAMES, resolve: resolve, kindFor: kindFor, mix: mix, rgba: rgba,
  SPOTS: SPOTS, BENCH_SPOTS: BENCH_SPOTS, MOTION_SPOTS: MOTION_SPOTS, spotLayers: spotLayers, castShadow: castShadow, shadowUnder: shadowUnder,
  MOTIFS: MOTIFS, sprite: sprite, drawParticle: drawParticle };
})();
