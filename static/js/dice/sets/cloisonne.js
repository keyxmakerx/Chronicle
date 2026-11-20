// ==================== Cloisonné ====================
// Enamel fired into cells of gold wire. Every face is a small plaque: a ground of one enamel scattered
// with gold curls, a lotus opening round the middle in two rings of petals, and the number in gold on
// a dark roundel at its heart. The wires stand a little proud of the glassy enamel between them, so
// they catch the light as the die turns.
import {
  hexA, mix, mulberry, paintNumeral, registerSet, strokeW
} from '../engine.js';

// ---------- Colours ----------
// ground: the enamel behind everything; petals: [inner ring, its tip, outer ring, its tip]; plate: the
// roundel under the number; wire: the metal of the cells and the number.
var CLOISONNE = [
  { key: 'imperial', name: 'Imperial blue', note: 'Deep blue ground, red and gold lotus, gold wire', burn: '#ffd36b', pop: '#ffe49a', ink: '#f0c860',
    ground: '#1c3fae', petals: ['#d93a2b', '#ff9a6a', '#1fa79c', '#9fe8d8'], plate: '#0a1747', band: '#18a092', wire: ['#f6dc8a', '#e3b548', '#8e6212'], base: { grad: ['#2a52c8', '#0d1f66'], rough: 0.14 } },
  { key: 'jade', name: 'Jade garden', note: 'Green ground, ivory and pink lotus, gold wire', burn: '#b8ffd0', pop: '#fff0c0', ink: '#f0c860',
    ground: '#0e7a58', petals: ['#f4e6c2', '#ffffff', '#e8668f', '#ffc0d2'], plate: '#053325', band: '#1c8fd0', wire: ['#f6dc8a', '#e3b548', '#8e6212'], base: { grad: ['#16986e', '#064631'], rough: 0.14 } },
  { key: 'vermilion', name: 'Vermilion', note: 'Red ground, gold and ivory lotus, gold wire', burn: '#ffb46b', pop: '#fff0c0', ink: '#f6d470',
    ground: '#bd2418', petals: ['#f3bd36', '#fff0a8', '#1b62b4', '#8fc4f2'], plate: '#4a0a06', band: '#f0e2c0', wire: ['#f8e094', '#e6ba4c', '#8e6212'], base: { grad: ['#d93a2a', '#6e120b'], rough: 0.14 } },
  { key: 'plum', name: 'Plum blossom', note: 'Violet ground, pink and cream blossom, gold wire', burn: '#f0b4ff', pop: '#ffe0f0', ink: '#f0c860',
    ground: '#4b1b6a', petals: ['#ef8fba', '#ffe1ec', '#2fb39d', '#b8f0e0'], plate: '#1c0830', band: '#e8a23a', wire: ['#f6dc8a', '#e3b548', '#8e6212'], base: { grad: ['#66289a', '#280c40'], rough: 0.14 } },
  { key: 'snow', name: 'Snow and sapphire', note: 'Ivory ground, blue lotus, gold wire', burn: '#ff9a4a', pop: '#ffe49a', ink: '#f0c860',
    ground: '#efe7d2', petals: ['#2767bd', '#8ec6f0', '#e39a32', '#ffe0a0'], plate: '#142a58', band: '#2767bd', wire: ['#f2d680', '#d9a93e', '#86590c'], base: { grad: ['#f6f0e0', '#cfc4a8'], rough: 0.14 } },
  { key: 'onyx', name: 'Onyx and silver', note: 'Black ground, pearl and garnet lotus, silver wire', burn: '#c9dcff', pop: '#ffffff', ink: '#e6eaf0',
    ground: '#131418', petals: ['#c9ced8', '#ffffff', '#a8324e', '#f08aa0'], plate: '#000000', band: '#5b6fc0', wire: ['#ffffff', '#d6dbe4', '#7b8494'], base: { grad: ['#2a2c34', '#08090b'], rough: 0.14 } }
];

// ---------- The plaque ----------
// Where each number sits: a d4 has three small roundels, every other die one.
function cloSpots(lay) {
  if (lay.sub) return lay.sub.map(function (s) { return { x: s.cx, y: s.cy, r: s.h * 1.02 }; });
  return [{ x: lay.cx, y: lay.cy, r: Math.min(lay.r * 0.74, lay.h * (lay.label.length > 1 ? 1.04 : 0.9)) }];
}
function cloSeed(lay) { return lay.cell ? lay.cell.x * 13 + lay.cell.y * 7 + lay.cv : lay.cv; }
// One petal: a pointed leaf from radius r0 to r1, half-width hw, pointing along angle a from (x, y).
function cloPetal(g, x, y, a, r0, r1, hw) {
  var c = Math.cos(a), s = Math.sin(a), nx = -s, ny = c, m = r0 + (r1 - r0) * 0.45;
  g.beginPath(); g.moveTo(x + c * r0, y + s * r0);
  g.bezierCurveTo(x + c * (r0 + (m - r0) * 0.3) + nx * hw, y + s * (r0 + (m - r0) * 0.3) + ny * hw, x + c * m + nx * hw, y + s * m + ny * hw, x + c * r1, y + s * r1);
  g.bezierCurveTo(x + c * m - nx * hw, y + s * m - ny * hw, x + c * (r0 + (m - r0) * 0.3) - nx * hw, y + s * (r0 + (m - r0) * 0.3) - ny * hw, x + c * r0, y + s * r0);
  g.closePath();
}
function cloCurl(g, x, y, r, a, dir) {
  g.beginPath();
  for (var i = 0; i <= 14; i++) { var u = i / 14, rr = r * (1 - u * 0.82), aa = a + dir * u * 7.2; if (i) g.lineTo(x + Math.cos(aa) * rr, y + Math.sin(aa) * rr); else g.moveTo(x + Math.cos(aa) * rr, y + Math.sin(aa) * rr); }
}
function cloInset(g, lay, k, add) {
  var cx = lay.cv / 2, cy = lay.cv / 2;
  if (!add) g.beginPath();
  lay.inner.forEach(function (p, i) { var x = cx + (p[0] - cx) * k, y = cy + (p[1] - cy) * k; if (i) g.lineTo(x, y); else g.moveTo(x, y); }); g.closePath();
}
// Everything made of wire, drawn with whatever stroke is set: the same paths serve the colour, the
// height and the surface images, so the three always agree.
function cloWires(g, lay, k, fn) {
  var rnd = mulberry(cloSeed(lay)), spots = cloSpots(lay), one = spots.length === 1, sp = spots[0], cv = lay.cv;
  var N = lay.inner.length === 3 ? 6 : lay.inner.length === 4 ? 8 : 10;
  // Curls on the ground, kept clear of the lotus.
  var curls = [];
  for (var i = 0; i < 60; i++) {
    var x = rnd() * cv, y = rnd() * cv, r = (4.5 + rnd() * 3.5) * k, a = rnd() * 6.28, dir = rnd() > 0.5 ? 1 : -1;
    var clear = spots.every(function (s) { return Math.hypot(x - s.x, y - s.y) > s.r * (one ? 2.05 : 1.25) + r; });
    if (clear && curls.every(function (c) { return Math.hypot(x - c[0], y - c[1]) > (c[2] + r) * 1.05; })) curls.push([x, y, r, a, dir]);
  }
  fn('curls', function () { curls.forEach(function (c) { cloCurl(g, c[0], c[1], c[2], c[3], c[4]); g.stroke(); }); });
  if (one) {
    var rot = -Math.PI / 2;
    fn('outer', function (each) { for (var j = 0; j < N; j++) each(function () { cloPetal(g, sp.x, sp.y, rot + (j + 0.5) / N * 6.2832, sp.r * 1.0, sp.r * 2.0, sp.r * 0.44); }, j); });
    fn('inner', function (each) { for (var j = 0; j < N; j++) each(function () { cloPetal(g, sp.x, sp.y, rot + j / N * 6.2832, sp.r * 0.96, sp.r * 1.62, sp.r * 0.4); }, j); });
  }
  fn('plates', function (each) { spots.forEach(function (s, j) { each(function () { g.beginPath(); g.arc(s.x, s.y, s.r, 0, 7); }, j); }); });
  fn('band', function (add) { cloInset(g, lay, 0.9, add); });
}
function cloGold(g, c, cv) {
  var gr = g.createLinearGradient(0, 0, cv, cv);
  gr.addColorStop(0, c.wire[0]); gr.addColorStop(0.5, c.wire[1]); gr.addColorStop(1, c.wire[2]); return gr;
}

registerSet({
  key: 'cloisonne', name: 'Cloisonné', note: 'Glassy enamel in cells of gold wire: a lotus on every face, the number in gold at its heart.',
  show: 'enamel', bev: 0.06, see: null, relief: 1.5,
  colours: CLOISONNE,
  paint: function (g, lay, c, seed) {
    var cv = lay.cv, k = cv / 256, w = strokeW(lay), rnd = mulberry(seed), gold = cloGold(g, c, cv), ww = 1.9 * k;
    g.fillStyle = c.ground; g.fillRect(0, 0, cv, cv);
    // Enamel is never flat: it pools a shade deeper in places.
    g.save(); try { g.filter = 'blur(' + (9 * k) + 'px)'; } catch (e) {}
    for (var i = 0; i < 9; i++) { g.fillStyle = hexA(rnd() > 0.5 ? '#000000' : '#ffffff', 0.1); g.beginPath(); g.arc(rnd() * cv, rnd() * cv, (18 + rnd() * 30) * k, 0, 7); g.fill(); }
    g.restore();
    g.lineJoin = 'round'; g.lineCap = 'round';
    function wire() { g.strokeStyle = 'rgba(0,0,0,.45)'; g.lineWidth = ww * 1.9; g.stroke(); g.strokeStyle = gold; g.lineWidth = ww; g.stroke(); }
    cloWires(g, lay, k, function (what, draw) {
      if (what === 'curls') { g.strokeStyle = 'rgba(0,0,0,.35)'; g.lineWidth = ww * 1.5; draw(); g.strokeStyle = gold; g.lineWidth = ww * 0.72; draw(); return; }
      if (what === 'outer' || what === 'inner') {
        var a = what === 'inner' ? c.petals[0] : c.petals[2], b = what === 'inner' ? c.petals[1] : c.petals[3], sp = cloSpots(lay)[0];
        draw(function (path) {
          var gr = g.createRadialGradient(sp.x, sp.y, sp.r, sp.x, sp.y, sp.r * 2); gr.addColorStop(0, mix(a, '#000000', 0.18)); gr.addColorStop(0.55, a); gr.addColorStop(1, b);
          path(); g.fillStyle = gr; g.fill(); wire();
        });
        return;
      }
      if (what === 'plates') { draw(function (path) { path(); g.fillStyle = c.plate; g.fill(); g.strokeStyle = 'rgba(0,0,0,.5)'; g.lineWidth = ww * 3.4; g.stroke(); g.strokeStyle = gold; g.lineWidth = ww * 2.1; g.stroke(); }); return; }
      // The band round the edge: a second enamel between the face's edge and a wire.
      g.save(); g.fillStyle = c.band; g.beginPath(); g.rect(0, 0, cv, cv); draw(true); g.fill('evenodd'); g.restore();
      draw(); wire();
    });
    // The number is wire too, heavier than the cells so it reads first.
    g.fillStyle = g.strokeStyle = 'rgba(0,0,0,.7)'; paintNumeral(g, lay, lay.style, w * 1.4);
    g.fillStyle = g.strokeStyle = gold; paintNumeral(g, lay, lay.style, w);
    g.fillStyle = g.strokeStyle = hexA(c.wire[0], 0.8); g.save(); g.translate(-w * 0.12, -w * 0.16); paintNumeral(g, lay, lay.style, w * 0.26); g.restore();
  },
  // Enamel is glass: smooth and not metal. The wires and the number are polished metal.
  surface: function (g, lay) {
    var cv = lay.cv, k = cv / 256, w = strokeW(lay), ww = 1.9 * k, M = 'rgb(0,74,215)';
    g.lineJoin = 'round'; g.lineCap = 'round'; g.strokeStyle = M;
    cloWires(g, lay, k, function (what, draw) {
      g.lineWidth = what === 'curls' ? ww * 0.72 : what === 'plates' ? ww * 2.1 : ww;
      if (what === 'curls') { draw(); return; }
      if (what === 'band') { draw(); g.stroke(); return; }
      draw(function (path) { path(); g.stroke(); });
    });
    g.fillStyle = g.strokeStyle = 'rgb(0,84,190)'; paintNumeral(g, lay, lay.style, w);
  },
  height: function (g, lay) {
    var cv = lay.cv, k = cv / 256, w = strokeW(lay), ww = 1.9 * k;
    g.fillStyle = '#8a8a8a'; g.fillRect(0, 0, cv, cv);
    g.save(); try { g.filter = 'blur(' + (0.7 * k) + 'px)'; } catch (e) {}
    g.lineJoin = 'round'; g.lineCap = 'round'; g.strokeStyle = '#fff';
    cloWires(g, lay, k, function (what, draw) {
      g.lineWidth = what === 'curls' ? ww * 0.9 : what === 'plates' ? ww * 2.3 : ww * 1.2;
      if (what === 'curls') { draw(); return; }
      if (what === 'band') { draw(); g.stroke(); return; }
      draw(function (path) { path(); g.stroke(); });
    });
    g.fillStyle = g.strokeStyle = '#fff'; paintNumeral(g, lay, lay.style, w * 1.05);
    g.restore();
  },
  rim: function (c) { return { col: c.wire[1], rough: 0.34, metal: 1 }; },
  physical: function (c, q) { return { clearcoat: q.detail === 'basic' ? 0 : 1, clearcoatRoughness: 0.08, envMapIntensity: 1.25 }; }
});
