// ==================== Arcana ====================
// Dice made like a tarot deck: matte card stock stamped with metal foil. Every face has a double foil
// border and a figure from the sky behind its number: a sun in splendour, a field of stars joined
// into a constellation, or a compass star. Only the foil shines, so it flashes as the die turns while
// the card stays dark.
import {
  hexA, mix, mulberry, paintNumeral, registerSet, strokeW
} from '../engine.js';

// ---------- Colours ----------
// card: [the stock, its shaded edge]; foil: the stops of the foil's sheen, light to dark (a holographic
// foil has more of them).
var GOLD = ['#fff3c0', '#edc358', '#a8771c'], SILVER = ['#ffffff', '#dfe5ee', '#8791a3'], ROSE = ['#ffe6d6', '#eda886', '#a4583c'];
var ARCANA = [
  { key: 'midnight', name: 'Midnight and gold', note: 'Deep blue card, gold foil', burn: '#ffd97a', ink: '#edc358',
    card: ['#101a44', '#070c24'], foil: GOLD, base: { grad: ['#1a2a66', '#070c24'], rough: 0.9 } },
  { key: 'ivory', name: 'Ivory and gold', note: 'Cream card, gold foil, black numbers', burn: '#ff9a2a', ink: '#1c1a22',
    card: ['#f1e9d6', '#cdbf9f'], foil: ['#f0cf70', '#cf9f34', '#8a5f10'], num: '#1c1a22', base: { grad: ['#f6f0e0', '#cfc2a2'], rough: 0.9 } },
  { key: 'crimson', name: 'Crimson and gold', note: 'Dark red card, gold foil', burn: '#ffd97a', ink: '#edc358',
    card: ['#5e0f18', '#2c0509'], foil: GOLD, base: { grad: ['#7a1722', '#2c0509'], rough: 0.9 } },
  { key: 'void', name: 'Black and silver', note: 'Black card, silver foil', burn: '#cfe0ff', ink: '#dfe5ee',
    card: ['#131318', '#050507'], foil: SILVER, base: { grad: ['#24242c', '#050507'], rough: 0.9 } },
  { key: 'emerald', name: 'Emerald and rose gold', note: 'Green card, rose-gold foil', burn: '#ffc8a8', ink: '#eda886',
    card: ['#0c3b2c', '#041a13'], foil: ROSE, base: { grad: ['#12523d', '#041a13'], rough: 0.9 } },
  { key: 'prism', name: 'Black and rainbow foil', note: 'Black card, foil that turns through every colour', burn: '#d8c0ff', ink: '#c9b8ff',
    card: ['#0e0c14', '#040306'], foil: ['#ff9ad0', '#ffd98a', '#a8f0b0', '#8ad4ff', '#c4a0ff', '#ff9ad0'], base: { grad: ['#1c1826', '#040306'], rough: 0.9 } }
];

function arcSeed(lay) { return lay.cell ? lay.cell.x * 11 + lay.cell.y * 19 + lay.cv : lay.cv; }
function arcInset(lay, k) { var cx = lay.cv / 2, cy = lay.cv / 2; return lay.inner.map(function (p) { return [cx + (p[0] - cx) * k, cy + (p[1] - cy) * k]; }); }
function arcTrace(g, pts) { g.beginPath(); pts.forEach(function (p, i) { if (i) g.lineTo(p[0], p[1]); else g.moveTo(p[0], p[1]); }); g.closePath(); }
function arcStar(g, x, y, r, rot) {
  g.beginPath();
  for (var i = 0; i < 8; i++) { var a = rot + i / 8 * 6.2832, rr = i % 2 ? r * 0.28 : r; if (i) g.lineTo(x + Math.cos(a) * rr, y + Math.sin(a) * rr); else g.moveTo(x + Math.cos(a) * rr, y + Math.sin(a) * rr); }
  g.closePath(); g.fill();
}
// Everything stamped in foil but the number, drawn with whatever fill and stroke are set, so the
// colour, the height and the surface images agree.
function arcFoil(g, lay, k) {
  var rnd = mulberry(arcSeed(lay)), cv = lay.cv, i, a;
  g.lineJoin = 'miter'; g.lineCap = 'round';
  g.lineWidth = 2.6 * k; arcTrace(g, arcInset(lay, 0.93)); g.stroke();
  var inner = arcInset(lay, 0.83);
  g.lineWidth = 0.9 * k; arcTrace(g, inner); g.stroke();
  inner.forEach(function (p) { g.beginPath(); g.arc(p[0], p[1], 2 * k, 0, 7); g.fill(); });
  g.save(); arcTrace(g, arcInset(lay, 0.8)); g.clip();
  if (lay.sub) {
    // A d4 has three numbers, so it gets a ring round each and stars between them.
    lay.sub.forEach(function (s) { g.lineWidth = 0.9 * k; g.beginPath(); g.arc(s.cx, s.cy, s.h * 0.86, 0, 7); g.stroke(); });
    for (i = 0; i < 9; i++) { a = rnd() * 6.28; var d = rnd() * lay.r * 0.42; arcStar(g, cv / 2 + Math.cos(a) * d, cv / 2 + Math.sin(a) * d, (2 + rnd() * 3.2) * k, rnd()); }
    g.restore(); return;
  }
  var x = lay.cx, y = lay.cy, R = Math.min(lay.r * 0.7, lay.h * (lay.label.length > 1 ? 1.0 : 0.86)), kind = Math.floor(rnd() * 3);
  g.lineWidth = 1.1 * k; g.beginPath(); g.arc(x, y, R, 0, 7); g.stroke();
  g.lineWidth = 0.6 * k; g.beginPath(); g.arc(x, y, R + 2.4 * k, 0, 7); g.stroke();
  if (kind === 0) {
    // The sun in splendour: straight rays, long and short by turns.
    for (i = 0; i < 24; i++) {
      a = i / 24 * 6.2832; var r1 = R + (i % 2 ? 13 : 24) * k * (lay.r / (86 * k)), hw = (i % 2 ? 0.045 : 0.06);
      g.beginPath(); g.moveTo(x + Math.cos(a - hw) * (R + 4 * k), y + Math.sin(a - hw) * (R + 4 * k)); g.lineTo(x + Math.cos(a) * r1, y + Math.sin(a) * r1); g.lineTo(x + Math.cos(a + hw) * (R + 4 * k), y + Math.sin(a + hw) * (R + 4 * k)); g.closePath(); g.fill();
    }
  } else if (kind === 1) {
    // A constellation: stars scattered round the ring, a few joined by a fine line.
    var st = [];
    for (i = 0; i < 40 && st.length < 9; i++) {
      a = rnd() * 6.28; var dd = R + (7 + rnd() * 26) * k * (lay.r / (86 * k)), sx = x + Math.cos(a) * dd, sy = y + Math.sin(a) * dd;
      if (st.every(function (q) { return Math.hypot(q[0] - sx, q[1] - sy) > 11 * k; })) st.push([sx, sy, (2.2 + rnd() * 3.6) * k, a]);
    }
    st.sort(function (p, q) { return p[3] - q[3]; });
    g.lineWidth = 0.55 * k; g.beginPath(); st.slice(0, 5).forEach(function (p, j) { if (j) g.lineTo(p[0], p[1]); else g.moveTo(p[0], p[1]); }); g.stroke();
    st.forEach(function (p) { arcStar(g, p[0], p[1], p[2], 0); });
  } else {
    // A compass star: four long points and four short, with a dot between each.
    for (i = 0; i < 8; i++) {
      a = i / 8 * 6.2832 - Math.PI / 2; var r2 = R + (i % 2 ? 14 : 30) * k * (lay.r / (86 * k)), h2 = i % 2 ? 0.1 : 0.14;
      g.beginPath(); g.moveTo(x + Math.cos(a - h2) * (R + 3.4 * k), y + Math.sin(a - h2) * (R + 3.4 * k)); g.lineTo(x + Math.cos(a) * r2, y + Math.sin(a) * r2); g.lineTo(x + Math.cos(a + h2) * (R + 3.4 * k), y + Math.sin(a + h2) * (R + 3.4 * k)); g.closePath(); g.fill();
      var am = a + 6.2832 / 16; g.beginPath(); g.arc(x + Math.cos(am) * (R + 9 * k), y + Math.sin(am) * (R + 9 * k), 1.3 * k, 0, 7); g.fill();
    }
  }
  g.restore();
}
function arcSheen(g, c, cv) {
  var gr = g.createLinearGradient(0, 0, cv, cv), n = c.foil.length;
  c.foil.forEach(function (h, i) { gr.addColorStop(i / (n - 1), h); }); return gr;
}

registerSet({
  key: 'arcana', name: 'Arcana', note: 'Matte card stamped in metal foil, like a tarot deck: a sun, a constellation or a compass star behind every number.',
  show: 'inscribe', bev: 0.035, see: null, relief: 0.7,
  colours: ARCANA,
  paint: function (g, lay, c, seed) {
    var cv = lay.cv, k = cv / 256, w = strokeW(lay), rnd = mulberry(seed);
    var gr = g.createRadialGradient(cv / 2, cv / 2, lay.r * 0.2, cv / 2, cv / 2, lay.r * 1.4);
    gr.addColorStop(0, c.card[0]); gr.addColorStop(1, c.card[1]); g.fillStyle = gr; g.fillRect(0, 0, cv, cv);
    // The stock's fibres.
    g.lineWidth = 0.5 * k;
    for (var i = 0; i < 160; i++) { var x = rnd() * cv, y = rnd() * cv, a = rnd() * 6.28, l = (3 + rnd() * 7) * k; g.strokeStyle = hexA(rnd() > 0.5 ? '#ffffff' : '#000000', 0.07); g.beginPath(); g.moveTo(x, y); g.lineTo(x + Math.cos(a) * l, y + Math.sin(a) * l); g.stroke(); }
    var foil = arcSheen(g, c, cv);
    g.fillStyle = g.strokeStyle = foil; arcFoil(g, lay, k);
    // The number: on a pale card it is printed in ink inside a foil line, on a dark card it is foil.
    if (c.num) {
      g.fillStyle = g.strokeStyle = foil; paintNumeral(g, lay, lay.style, w * 1.38);
      g.fillStyle = g.strokeStyle = c.num; paintNumeral(g, lay, lay.style, w * 0.98);
    } else {
      g.fillStyle = g.strokeStyle = hexA(c.card[1], 0.9); paintNumeral(g, lay, lay.style, w * 1.6);
      g.fillStyle = g.strokeStyle = foil; paintNumeral(g, lay, lay.style, w * 1.08);
    }
  },
  // Card is as matte as a surface gets; foil is polished metal.
  surface: function (g, lay, c) {
    var cv = lay.cv, k = cv / 256, w = strokeW(lay);
    g.fillStyle = 'rgb(0,' + Math.round(c.base.rough * 255) + ',0)'; g.fillRect(0, 0, cv, cv);
    g.fillStyle = g.strokeStyle = 'rgb(0,66,235)'; arcFoil(g, lay, k);
    paintNumeral(g, lay, lay.style, w * (c.num ? 1.38 : 1.08));
    if (c.num) { g.fillStyle = g.strokeStyle = 'rgb(0,150,0)'; paintNumeral(g, lay, lay.style, w * 0.98); }
  },
  // Foil is pressed in: it sits a hair below the card.
  height: function (g, lay, c) {
    var cv = lay.cv, k = cv / 256, w = strokeW(lay);
    g.fillStyle = '#a0a0a0'; g.fillRect(0, 0, cv, cv);
    g.save(); try { g.filter = 'blur(' + (0.5 * k) + 'px)'; } catch (e) {}
    g.fillStyle = g.strokeStyle = '#6c6c6c'; arcFoil(g, lay, k);
    paintNumeral(g, lay, lay.style, w * (c.num ? 1.38 : 1.08));
    g.restore();
  },
  rim: function (c) { return { col: c.foil[1], rough: 0.32, metal: 1 }; },
  physical: function () { return { envMapIntensity: 1.5, specularIntensity: 0.35 }; },
  cover: function (c) { return c.num ? 0.65 : 0; }
});
