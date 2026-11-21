// ==================== Tooled leather ====================
// Dice cut from thick saddle leather. Each face has the hide's own pebbled grain, edges burnished
// darker by handling, a creased border line, and a row of saddle stitches inside it. The number is
// pressed into the hide with a hot stamp and filled with foil, so it sits below the surface and shines.
import {
  hexA, mix, mulberry, paintNumeral, registerSet, strokeW
} from '../engine.js';

// ---------- Colours ----------
// hide: [the leather, its burnished edge, where it is worn pale]; thread: the stitching;
// metal: the foil in the number.
var GOLD = { color: '#eec35a', hi: '#fff2bd', lo: '#94680f', rough: 0.3 }, SILVER = { color: '#e3e8f0', hi: '#ffffff', lo: '#858f9f', rough: 0.28 },
  COPPER = { color: '#e28a55', hi: '#ffd3b2', lo: '#85401b', rough: 0.3 };
var LEATHER = [
  { key: 'saddle', name: 'Saddle tan and gold', note: 'Tan hide, cream stitching, gold numbers', burn: '#ffb45c', flame: '#ff8a2a', ink: '#eec35a',
    hide: ['#a8672f', '#5c3110', '#c98d52'], thread: '#f1e3c4', metal: GOLD, base: { grad: ['#b9783c', '#6a3a14'], rough: 0.62 } },
  { key: 'oxblood', name: 'Oxblood and gold', note: 'Deep red hide, gold stitching, gold numbers', burn: '#ff9a5c', flame: '#ff6a2a', ink: '#eec35a',
    hide: ['#6e1a1c', '#330709', '#96383a'], thread: '#e2b65a', metal: GOLD, base: { grad: ['#84262a', '#3a0a0c'], rough: 0.6 } },
  { key: 'black', name: 'Black and silver', note: 'Black hide, grey stitching, silver numbers', burn: '#bcd4ff', flame: '#8fb4ff', ink: '#e3e8f0',
    hide: ['#1d1c1f', '#070708', '#3a393f'], thread: '#9aa0ac', metal: SILVER, base: { grad: ['#2e2d32', '#0a0a0c'], rough: 0.58 } },
  { key: 'forest', name: 'Forest green and brass', note: 'Green hide, tan stitching, brass numbers', burn: '#e6ff9a', flame: '#c8f06a', ink: '#e0bd5c',
    hide: ['#234a2e', '#0d2415', '#3f6f4c'], thread: '#d9c08e', metal: { color: '#e0bd5c', hi: '#fff0b8', lo: '#86661a', rough: 0.32 }, base: { grad: ['#2f5e3c', '#102a19'], rough: 0.62 } },
  { key: 'navy', name: 'Navy and copper', note: 'Blue hide, copper stitching, copper numbers', burn: '#ffb07a', flame: '#ff8a4a', ink: '#e28a55',
    hide: ['#1c2c56', '#0a122b', '#34497d'], thread: '#d98a58', metal: COPPER, base: { grad: ['#263a70', '#0c1530'], rough: 0.6 } },
  { key: 'natural', name: 'Natural hide and silver', note: 'Pale undyed hide, dark stitching, silver numbers', burn: '#ff8a2a', flame: '#ff6a1a', ink: '#5a3a1c',
    hide: ['#d9b68a', '#9a7040', '#edd6b4'], thread: '#4a2f18', metal: { color: '#5d3b1c', hi: '#8a6238', lo: '#2c1808', rough: 0.5 }, base: { grad: ['#e2c39c', '#a67c4c'], rough: 0.66 } }
];

// ---------- The tooling ----------
function leaSeed(lay) { return lay.cell ? lay.cell.x * 13 + lay.cell.y * 7 + lay.cv : lay.cv; }
function leaInset(lay, k) { var cx = lay.cv / 2, cy = lay.cv / 2; return lay.inner.map(function (p) { return [cx + (p[0] - cx) * k, cy + (p[1] - cy) * k]; }); }
function leaTrace(g, pts) { g.beginPath(); pts.forEach(function (p, i) { if (i) g.lineTo(p[0], p[1]); else g.moveTo(p[0], p[1]); }); g.closePath(); }
// Evenly spaced places along an outline, each with the direction the outline runs there.
function leaWalk(pts, step, fn) {
  pts.forEach(function (a, i) {
    var b = pts[(i + 1) % pts.length], dx = b[0] - a[0], dy = b[1] - a[1], L = Math.hypot(dx, dy), n = Math.max(2, Math.round(L / step)), ang = Math.atan2(dy, dx);
    for (var j = 0; j < n; j++) { var t = (j + 0.5) / n; fn(a[0] + dx * t, a[1] + dy * t, ang, L / n); }
  });
}
// Where numbers sit, so the stitching can leave them room on a d4, whose numbers are at its corners.
function leaClear(lay, x, y) {
  if (!lay.sub) return true;
  return lay.sub.every(function (s) { return Math.hypot(x - s.cx, y - s.cy) > s.h * 0.82; });
}
// The grain: short creases and pores, the same every time for one face so colour and height agree.
function leaGrain(g, lay, k, each) {
  var rnd = mulberry(leaSeed(lay)), cv = lay.cv;
  for (var i = 0; i < 620; i++) {
    var x = rnd() * cv, y = rnd() * cv, r = (1.6 + rnd() * 3.4) * k, a = rnd() * 6.28, sw = 0.7 + rnd() * 1.6;
    each(i, rnd()); g.beginPath(); g.arc(x, y, r, a, a + sw); g.stroke();
  }
}
function leaStitches(g, lay, k, draw) {
  leaWalk(leaInset(lay, 0.79), 8.6 * k, function (x, y, ang, len) { if (leaClear(lay, x, y)) draw(x, y, ang, Math.min(len * 0.62, 5.6 * k)); });
}

registerSet({
  key: 'leather', name: 'Tooled leather', note: 'Thick saddle leather with a pebbled grain, burnished edges and a row of stitches. The number is hot-stamped in foil.',
  show: 'kindle', bev: 0.085, see: null, relief: 1.7,
  colours: LEATHER,
  paint: function (g, lay, c, seed) {
    var cv = lay.cv, k = cv / 256, w = strokeW(lay), rnd = mulberry(seed), m = c.metal;
    g.fillStyle = c.hide[0]; g.fillRect(0, 0, cv, cv);
    // Wear: soft patches paler where the hide has rubbed, darker where oil has soaked in.
    g.save(); try { g.filter = 'blur(' + (10 * k) + 'px)'; } catch (e) {}
    for (var i = 0; i < 9; i++) { g.fillStyle = hexA(rnd() > 0.45 ? c.hide[2] : c.hide[1], 0.3); g.beginPath(); g.arc(rnd() * cv, rnd() * cv, (16 + rnd() * 30) * k, 0, 7); g.fill(); }
    g.restore();
    g.lineCap = 'round'; g.lineWidth = 0.8 * k;
    leaGrain(g, lay, k, function (i, u) { g.strokeStyle = u > 0.3 ? hexA(c.hide[1], 0.3) : hexA(c.hide[2], 0.26); });
    // Burnished edges: the colour deepens toward every edge of the face.
    var gr = g.createRadialGradient(cv / 2, cv / 2, lay.r * 0.55, cv / 2, cv / 2, lay.r * 1.45);
    gr.addColorStop(0, hexA(c.hide[1], 0)); gr.addColorStop(1, hexA(c.hide[1], 0.85));
    g.fillStyle = gr; g.fillRect(0, 0, cv, cv);
    // The creased border line.
    g.lineJoin = 'round';
    g.strokeStyle = hexA(mix(c.hide[1], '#000000', 0.5), 0.8); g.lineWidth = 2.1 * k; leaTrace(g, leaInset(lay, 0.9)); g.stroke();
    g.save(); g.translate(0.9 * k, 0.9 * k); g.strokeStyle = hexA(c.hide[2], 0.35); g.lineWidth = 0.8 * k; leaTrace(g, leaInset(lay, 0.9)); g.stroke(); g.restore();
    // Saddle stitches: a hole at each end, the thread lying between them with its shadow under it.
    leaStitches(g, lay, k, function (x, y, ang, len) {
      var dx = Math.cos(ang) * len / 2, dy = Math.sin(ang) * len / 2;
      g.fillStyle = 'rgba(0,0,0,.55)'; g.beginPath(); g.arc(x - dx, y - dy, 1.25 * k, 0, 7); g.arc(x + dx, y + dy, 1.25 * k, 0, 7); g.fill();
      g.strokeStyle = 'rgba(0,0,0,.5)'; g.lineWidth = 2.3 * k; g.beginPath(); g.moveTo(x - dx + 0.7 * k, y - dy + 0.9 * k); g.lineTo(x + dx + 0.7 * k, y + dy + 0.9 * k); g.stroke();
      g.strokeStyle = c.thread; g.lineWidth = 1.7 * k; g.beginPath(); g.moveTo(x - dx, y - dy); g.lineTo(x + dx, y + dy); g.stroke();
      g.strokeStyle = 'rgba(255,255,255,.35)'; g.lineWidth = 0.5 * k; g.beginPath(); g.moveTo(x - dx * 0.6, y - dy * 0.6 - 0.4 * k); g.lineTo(x + dx * 0.6, y + dy * 0.6 - 0.4 * k); g.stroke();
    });
    // The number: the scorched lip of the stamp, the shadow of its upper wall, then the foil.
    g.fillStyle = g.strokeStyle = hexA(mix(c.hide[1], '#000000', 0.6), 0.9); paintNumeral(g, lay, lay.style, w * 1.42);
    var fg = g.createLinearGradient(0, lay.cy - lay.h * 0.6, 0, lay.cy + lay.h * 0.6); fg.addColorStop(0, m.lo); fg.addColorStop(0.5, m.color); fg.addColorStop(1, m.hi);
    g.fillStyle = g.strokeStyle = fg; paintNumeral(g, lay, lay.style, w);
    g.fillStyle = g.strokeStyle = 'rgba(0,0,0,.4)'; g.save(); g.translate(0, -w * 0.3); paintNumeral(g, lay, lay.style, w * 0.3); g.restore();
  },
  // Leather is rough and not metal; the foil is metal, the thread is rougher still.
  surface: function (g, lay, c) {
    var cv = lay.cv, k = cv / 256, w = strokeW(lay);
    g.fillStyle = 'rgb(0,' + Math.round(c.base.rough * 255) + ',0)'; g.fillRect(0, 0, cv, cv);
    // Burnishing polishes the edges and the crease.
    var gr = g.createRadialGradient(cv / 2, cv / 2, lay.r * 0.6, cv / 2, cv / 2, lay.r * 1.4);
    gr.addColorStop(0, 'rgba(0,95,0,0)'); gr.addColorStop(1, 'rgba(0,95,0,.8)'); g.fillStyle = gr; g.fillRect(0, 0, cv, cv);
    g.lineCap = 'round'; g.strokeStyle = 'rgb(0,215,0)'; g.lineWidth = 1.7 * k;
    leaStitches(g, lay, k, function (x, y, ang, len) { var dx = Math.cos(ang) * len / 2, dy = Math.sin(ang) * len / 2; g.beginPath(); g.moveTo(x - dx, y - dy); g.lineTo(x + dx, y + dy); g.stroke(); });
    g.fillStyle = g.strokeStyle = 'rgb(0,' + Math.round(c.metal.rough * 255) + ',' + (c.metal.rough > 0.45 ? 0 : 110) + ')'; paintNumeral(g, lay, lay.style, w);
  },
  height: function (g, lay) {
    var cv = lay.cv, k = cv / 256, w = strokeW(lay);
    g.fillStyle = '#909090'; g.fillRect(0, 0, cv, cv);
    g.lineCap = 'round'; g.lineWidth = 1.0 * k;
    leaGrain(g, lay, k, function (i, u) { g.strokeStyle = u > 0.3 ? 'rgba(40,40,40,.5)' : 'rgba(215,215,215,.4)'; });
    g.save(); try { g.filter = 'blur(' + (0.7 * k) + 'px)'; } catch (e) {}
    g.lineJoin = 'round'; g.strokeStyle = '#3c3c3c'; g.lineWidth = 2.1 * k; leaTrace(g, leaInset(lay, 0.9)); g.stroke();
    leaStitches(g, lay, k, function (x, y, ang, len) {
      var dx = Math.cos(ang) * len / 2, dy = Math.sin(ang) * len / 2;
      g.fillStyle = '#303030'; g.beginPath(); g.arc(x - dx, y - dy, 1.3 * k, 0, 7); g.arc(x + dx, y + dy, 1.3 * k, 0, 7); g.fill();
      g.strokeStyle = '#f0f0f0'; g.lineWidth = 1.7 * k; g.beginPath(); g.moveTo(x - dx, y - dy); g.lineTo(x + dx, y + dy); g.stroke();
    });
    g.fillStyle = g.strokeStyle = '#2a2a2a'; paintNumeral(g, lay, lay.style, w * 1.3);
    g.fillStyle = g.strokeStyle = '#484848'; paintNumeral(g, lay, lay.style, w * 0.85);
    g.restore();
  },
  rim: function (c) { return { col: mix(c.hide[1], c.hide[0], 0.3), rough: 0.42, metal: 0 }; },
  physical: function (c, q) { return { sheen: q.detail === 'basic' ? 0 : 0.7, sheenRoughness: 0.5, sheenColor: mix(c.hide[2], '#ffffff', 0.25), specularIntensity: 0.7 }; }
});
