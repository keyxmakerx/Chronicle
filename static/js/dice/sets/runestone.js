// ==================== Runestone ====================
// Dice knapped from rough standing stone. The faces are speckled granite, weathered darker toward the
// edges and chipped where the stone has knocked about. Lichen has taken hold in patches. The number
// is chiselled in as a V-shaped cut and rubbed with pigment, the way a rune stone is painted.
import {
  hexA, mix, mulberry, paintNumeral, registerSet, strokeW
} from '../engine.js';

// ---------- Colours ----------
// stone: [body, its dark grains, its pale grains]; fleck: the bright mineral in it; pig: the pigment
// in the cut; lichen: [body, bright edge].
var RUNESTONE = [
  { key: 'granite', name: 'Grey granite and red ochre', note: 'Grey speckled stone, red numbers, yellow lichen', burn: '#ff8a4a', ink: '#c23a1e',
    stone: ['#8d8f92', '#4c4e52', '#c9cace'], fleck: '#f2ead8', pig: '#b8321a', lichen: ['#c9b23a', '#f0e080'], base: { grad: ['#a6a8ab', '#5a5c60'], rough: 0.88 } },
  { key: 'basalt', name: 'Black basalt and woad', note: 'Dark stone, blue numbers, pale green lichen', burn: '#7ac4ff', ink: '#4a9af0',
    stone: ['#33353a', '#17181b', '#5c6068'], fleck: '#9aa4b8', pig: '#3f8fe8', lichen: ['#9fc4a2', '#dff0d8'], base: { grad: ['#44474d', '#141517'], rough: 0.86 } },
  { key: 'sandstone', name: 'Sandstone and soot', note: 'Warm buff stone, black numbers, grey lichen', burn: '#ff9a3a', ink: '#2a2018',
    stone: ['#c9a878', '#8e6e44', '#ead4ac'], fleck: '#fff2d4', pig: '#2a1e14', lichen: ['#a9b0a2', '#e2e8dc'], base: { grad: ['#d8ba8c', '#94744a'], rough: 0.9 } },
  { key: 'slate', name: 'Blue slate and chalk', note: 'Blue-grey stone, white numbers, orange lichen', burn: '#ffe0b0', ink: '#f2f0e8',
    stone: ['#4f5d70', '#2a3442', '#7f8ea2'], fleck: '#c8d6e8', pig: '#f2f0e6', lichen: ['#d8842a', '#ffc470'], base: { grad: ['#62728a', '#273140'], rough: 0.84 } },
  { key: 'moss', name: 'Mossy greenstone and gold', note: 'Green stone, gold numbers, deep moss', burn: '#ffe07a', ink: '#e8c050',
    stone: ['#5d7a5c', '#2f452f', '#8fae8a'], fleck: '#d8e8c0', pig: '#e8c050', lichen: ['#3f7a2a', '#8fd05a'], base: { grad: ['#6f8f6c', '#2c402c'], rough: 0.88 } },
  { key: 'redstone', name: 'Red sandstone and bone white', note: 'Rust-red stone, white numbers, pale lichen', burn: '#ffd0a0', ink: '#f6efe0',
    stone: ['#a24a34', '#5e2416', '#cf7f66'], fleck: '#ffd8c0', pig: '#f6efe0', lichen: ['#b8c8a0', '#eef4dc'], base: { grad: ['#b85c44', '#5a2014'], rough: 0.9 } }
];

function runeSeed(lay) { return lay.cell ? lay.cell.x * 17 + lay.cell.y * 5 + lay.cv : lay.cv; }
function runeSpots(lay) { return lay.sub ? lay.sub.map(function (s) { return { x: s.cx, y: s.cy, r: s.h * 0.8 }; }) : [{ x: lay.cx, y: lay.cy, r: lay.h * 0.85 }]; }
// What the weather has done to one face: chips out of its edges and patches of lichen, the same every
// time so the colour, the height and the surface images agree.
function runeMarks(lay, k) {
  var rnd = mulberry(runeSeed(lay)), spots = runeSpots(lay), chips = [], lichen = [], P = lay.poly, n = P.length, cv = lay.cv;
  var nc = 3 + Math.floor(rnd() * 4);
  for (var i = 0; i < nc; i++) {
    var e = Math.floor(rnd() * n), a = P[e], b = P[(e + 1) % n], t = 0.12 + rnd() * 0.76, x = a[0] + (b[0] - a[0]) * t, y = a[1] + (b[1] - a[1]) * t;
    var r = (8 + rnd() * 11) * k, pts = [], m = 6 + Math.floor(rnd() * 3), a0 = rnd() * 6.28;
    for (var j = 0; j < m; j++) { var aa = a0 + j / m * 6.2832, rr = r * (0.55 + rnd() * 0.6); pts.push([x + Math.cos(aa) * rr, y + Math.sin(aa) * rr]); }
    chips.push(pts);
  }
  var nl = 2 + Math.floor(rnd() * 2), guard = 0;
  while (lichen.length < nl && guard++ < 40) {
    var lx = rnd() * cv, ly = rnd() * cv, lr = (13 + rnd() * 14) * k;
    if (Math.hypot(lx - cv / 2, ly - cv / 2) > lay.r * 1.2) continue;
    if (!spots.every(function (s) { return Math.hypot(lx - s.x, ly - s.y) > s.r + lr * 0.9; })) continue;
    var blobs = [], nb = 18 + Math.floor(rnd() * 20);
    for (var q = 0; q < nb; q++) { var ba = rnd() * 6.28, bd = Math.sqrt(rnd()) * lr; blobs.push([lx + Math.cos(ba) * bd, ly + Math.sin(ba) * bd, (1.8 + rnd() * 4.2) * k * (1.15 - bd / lr * 0.6), rnd()]); }
    lichen.push(blobs);
  }
  return { chips: chips, lichen: lichen };
}
function runePoly(g, pts) { g.beginPath(); pts.forEach(function (p, i) { if (i) g.lineTo(p[0], p[1]); else g.moveTo(p[0], p[1]); }); g.closePath(); }

registerSet({
  key: 'runestone', name: 'Runestone', note: 'Rough standing stone, chipped and weathered, with lichen in patches. The number is chiselled deep and rubbed with pigment.',
  show: 'inscribe', bev: 0.1, see: null, relief: 2.3,
  colours: RUNESTONE,
  paint: function (g, lay, c, seed) {
    var cv = lay.cv, k = cv / 256, w = strokeW(lay), rnd = mulberry(seed), mk = runeMarks(lay, k);
    g.fillStyle = c.stone[0]; g.fillRect(0, 0, cv, cv);
    g.save(); try { g.filter = 'blur(' + (7 * k) + 'px)'; } catch (e) {}
    for (var i = 0; i < 12; i++) { g.fillStyle = hexA(rnd() > 0.5 ? c.stone[1] : c.stone[2], 0.3); g.beginPath(); g.arc(rnd() * cv, rnd() * cv, (12 + rnd() * 26) * k, 0, 7); g.fill(); }
    g.restore();
    // The grains of the stone: dark, pale and a few bright flecks.
    for (var j = 0; j < 2300; j++) {
      var u = rnd();
      g.fillStyle = u < 0.45 ? hexA(c.stone[1], 0.72) : u < 0.86 ? hexA(c.stone[2], 0.7) : hexA(c.fleck, 0.95);
      g.beginPath(); g.arc(rnd() * cv, rnd() * cv, (0.5 + rnd() * rnd() * 2.6) * k, 0, 7); g.fill();
    }
    // Weather darkens the edges.
    var gr = g.createRadialGradient(cv / 2, cv / 2, lay.r * 0.5, cv / 2, cv / 2, lay.r * 1.4);
    gr.addColorStop(0, hexA(c.stone[1], 0)); gr.addColorStop(1, hexA(c.stone[1], 0.55)); g.fillStyle = gr; g.fillRect(0, 0, cv, cv);
    // Chips: the fresh stone inside is paler, with a shadow under its upper lip.
    mk.chips.forEach(function (p) {
      runePoly(g, p); g.fillStyle = mix(c.stone[0], c.stone[2], 0.55); g.fill();
      g.save(); g.clip(); g.translate(0, 2.2 * k); runePoly(g, p); g.strokeStyle = hexA('#000000', 0.5); g.lineWidth = 3 * k; g.stroke(); g.restore();
    });
    mk.lichen.forEach(function (blobs) {
      blobs.forEach(function (b) { g.fillStyle = hexA('#000000', 0.25); g.beginPath(); g.arc(b[0] + 0.7 * k, b[1] + 0.9 * k, b[2], 0, 7); g.fill(); });
      blobs.forEach(function (b) { g.fillStyle = mix(c.lichen[0], c.lichen[1], b[3] * 0.6); g.beginPath(); g.arc(b[0], b[1], b[2], 0, 7); g.fill(); g.strokeStyle = hexA(c.lichen[1], 0.7); g.lineWidth = 0.6 * k; g.stroke(); });
    });
    // The cut: its shadowed wall, its lit wall, then the pigment lying in the bottom.
    g.fillStyle = g.strokeStyle = hexA(mix(c.stone[1], '#000000', 0.55), 0.95); paintNumeral(g, lay, lay.style, w * 1.36);
    g.save(); g.translate(w * 0.16, w * 0.2); g.fillStyle = g.strokeStyle = hexA(c.stone[2], 0.6); paintNumeral(g, lay, lay.style, w * 0.9); g.restore();
    g.fillStyle = g.strokeStyle = c.pig; paintNumeral(g, lay, lay.style, w * 0.92);
    // Pigment wears thin: a few grains of stone show through it.
    g.save(); g.globalCompositeOperation = 'source-atop';
    for (var q = 0; q < 240; q++) { g.fillStyle = hexA(c.stone[rnd() > 0.5 ? 1 : 0], 0.22); g.beginPath(); g.arc(rnd() * cv, rnd() * cv, (0.5 + rnd() * 1.3) * k, 0, 7); g.fill(); }
    g.restore();
  },
  surface: function (g, lay, c) {
    var cv = lay.cv, k = cv / 256, w = strokeW(lay), mk = runeMarks(lay, k), rnd = mulberry(runeSeed(lay) + 3);
    g.fillStyle = 'rgb(0,' + Math.round(c.base.rough * 255) + ',0)'; g.fillRect(0, 0, cv, cv);
    // A few grains are glassy and catch the light.
    for (var j = 0; j < 160; j++) { g.fillStyle = 'rgb(0,70,0)'; g.beginPath(); g.arc(rnd() * cv, rnd() * cv, (0.5 + rnd() * 1.1) * k, 0, 7); g.fill(); }
    g.fillStyle = 'rgb(0,250,0)'; mk.lichen.forEach(function (blobs) { blobs.forEach(function (b) { g.beginPath(); g.arc(b[0], b[1], b[2], 0, 7); g.fill(); }); });
    g.fillStyle = g.strokeStyle = 'rgb(0,175,0)'; paintNumeral(g, lay, lay.style, w * 0.92);
  },
  height: function (g, lay) {
    var cv = lay.cv, k = cv / 256, w = strokeW(lay), mk = runeMarks(lay, k), rnd = mulberry(runeSeed(lay) + 9);
    g.fillStyle = '#a0a0a0'; g.fillRect(0, 0, cv, cv);
    // The stone's own unevenness: broad swells, then pits and grains.
    g.save(); try { g.filter = 'blur(' + (5 * k) + 'px)'; } catch (e) {}
    for (var i = 0; i < 26; i++) { g.fillStyle = rnd() > 0.5 ? 'rgba(255,255,255,.3)' : 'rgba(0,0,0,.3)'; g.beginPath(); g.arc(rnd() * cv, rnd() * cv, (6 + rnd() * 16) * k, 0, 7); g.fill(); }
    g.restore();
    for (var j = 0; j < 900; j++) { g.fillStyle = rnd() > 0.5 ? 'rgba(255,255,255,.3)' : 'rgba(0,0,0,.4)'; g.beginPath(); g.arc(rnd() * cv, rnd() * cv, (0.5 + rnd() * rnd() * 2.2) * k, 0, 7); g.fill(); }
    g.save(); try { g.filter = 'blur(' + (0.9 * k) + 'px)'; } catch (e) {}
    g.fillStyle = '#585858'; mk.chips.forEach(function (p) { runePoly(g, p); g.fill(); });
    g.fillStyle = '#c4c4c4'; mk.lichen.forEach(function (blobs) { blobs.forEach(function (b) { g.beginPath(); g.arc(b[0], b[1], b[2], 0, 7); g.fill(); }); });
    g.fillStyle = g.strokeStyle = '#505050'; paintNumeral(g, lay, lay.style, w * 1.36);
    g.fillStyle = g.strokeStyle = '#181818'; paintNumeral(g, lay, lay.style, w * 0.6);
    g.restore();
  },
  rim: function (c) { return { col: mix(c.stone[0], c.stone[1], 0.45), rough: 0.92, metal: 0 }; },
  physical: function () { return { specularIntensity: 0.5 }; },
  cover: function () { return 0.3; }
});
