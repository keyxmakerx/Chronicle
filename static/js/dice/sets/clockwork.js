// ==================== Clockwork ====================
// A die built by a watchmaker. Each face is a brushed metal plate with windows cut clean through it;
// the number sits on a small dial held in the middle by bridges, with a screw at every bridge's foot.
// Behind the plates a train of gears turns: it whirs while the die tumbles and, once it rests, ticks
// on a tooth at a time like a watch.
import {
  canvas, hexA, mix, mulberry, paintNumeral, registerSet, strokeW
} from '../engine.js';

// ---------- Colours ----------
// plate: [bright, body, shade]; num: the enamel in the number; gears: the three metals of the train.
var CLOCKWORK = [
  { key: 'brass', name: 'Brass and blued steel', note: 'Brass plates, blue numbers, steel and copper gears', burn: '#ffd97a', ink: '#1c2c7a',
    plate: ['#fff0b4', '#d6a742', '#6e4a10'], num: '#16246e', gears: ['#c9ced8', '#d98a52', '#e6bd58'], metal: { color: '#f2c860', hi: '#fff3c4', lo: '#a07418', rough: 0.3 }, base: { grad: ['#e2b852', '#7a5414'], rough: 0.36 } },
  { key: 'steel', name: 'Steel and brass', note: 'Steel plates, black numbers, brass gears', burn: '#cfe0ff', ink: '#101218',
    plate: ['#ffffff', '#b9c0cc', '#4a505c'], num: '#0e1016', gears: ['#e6bd58', '#d6a742', '#c9ced8'], metal: { color: '#e6eaf0', hi: '#ffffff', lo: '#7b8494', rough: 0.28 }, base: { grad: ['#c8ced8', '#525864'], rough: 0.34 } },
  { key: 'copper', name: 'Copper and verdigris', note: 'Copper plates, green numbers, brass gears', burn: '#ffb48a', ink: '#0f5a4c',
    plate: ['#ffd0b0', '#c9703f', '#5a2410'], num: '#0c4c40', gears: ['#e6bd58', '#c9ced8', '#5fb8a0'], metal: { color: '#f0a070', hi: '#ffd8c0', lo: '#8a401c', rough: 0.3 }, base: { grad: ['#d98456', '#5e2812'], rough: 0.36 } },
  { key: 'gunmetal', name: 'Gunmetal and gold', note: 'Dark plates, gold numbers, gold gears', burn: '#ffd97a', ink: '#f0c558',
    plate: ['#8a909c', '#3a3e48', '#0e0f13'], num: '#f0c558', gears: ['#f0c558', '#e6bd58', '#c9ced8'], metal: { color: '#f0c558', hi: '#fff0b0', lo: '#946a12', rough: 0.3 }, base: { grad: ['#4c515c', '#111216'], rough: 0.4 } },
  { key: 'rose', name: 'Rose gold and ivory', note: 'Rose-gold plates, ivory numbers, silver gears', burn: '#ffd0c0', ink: '#fff6e8',
    plate: ['#fff0e8', '#e6a892', '#7a4038'], num: '#5a1c24', gears: ['#e9edf4', '#e6bd58', '#e6a892'], metal: { color: '#f4c6b4', hi: '#fff4f0', lo: '#8e5048', rough: 0.3 }, base: { grad: ['#eab4a0', '#7a4038'], rough: 0.34 } },
  { key: 'bronze', name: 'Dark bronze and jade', note: 'Aged bronze plates, jade numbers, brass gears', burn: '#9affd0', ink: '#5fe0b0',
    plate: ['#c9a06a', '#6e4c24', '#241408'], num: '#5fe0b0', gears: ['#e6bd58', '#c9703f', '#c9ced8'], metal: { color: '#5fe0b0', hi: '#d0fff0', lo: '#1c7a5c', rough: 0.3 }, base: { grad: ['#84602c', '#28180a'], rough: 0.42 } }
];

// ---------- The plate ----------
function clkInset(lay, k) { var cx = lay.cv / 2, cy = lay.cv / 2; return lay.inner.map(function (p) { return [cx + (p[0] - cx) * k, cy + (p[1] - cy) * k]; }); }
function clkTrace(g, pts) { g.beginPath(); pts.forEach(function (p, i) { if (i) g.lineTo(p[0], p[1]); else g.moveTo(p[0], p[1]); }); g.closePath(); }
// What one face is made of: the dial(s), the bridges that hold them, the screws, and the image of
// the windows (white where the plate is cut away). Worked out once for each face.
var clkCache = {};
function clkGeom(lay) {
  var key = [lay.cv, lay.label, lay.cx.toFixed(1), lay.cy.toFixed(1), lay.inner.length, lay.cell ? lay.cell.x + ':' + lay.cell.y : ''].join('|');
  if (clkCache[key]) return clkCache[key];
  var cv = lay.cv, k = cv / 256, cx = cv / 2, cy = cv / 2, win = canvas(cv), g = win.getContext('2d'), frame = clkInset(lay, 0.8), n = frame.length;
  var dials = lay.sub ? lay.sub.map(function (s) { return { x: s.cx, y: s.cy, r: s.h * 0.86 }; }) : [{ x: lay.cx, y: lay.cy, r: Math.min(lay.r * 0.56, lay.h * (lay.label.length > 1 ? 0.98 : 0.84)) }];
  var bridges = [], screws = [];
  // A round porthole about the dial, kept inside the face.
  g.save(); clkTrace(g, clkInset(lay, 0.8)); g.clip();
  g.fillStyle = '#fff'; g.beginPath(); g.arc(dials[0].x, dials[0].y, Math.max(lay.r * 0.94, dials[0].r + 13 * k), 0, 7); g.fill(); g.restore();
  g.globalCompositeOperation = 'destination-out'; g.fillStyle = g.strokeStyle = '#000'; g.lineCap = 'butt';
  if (lay.sub) {
    // A d4 keeps its three numbers at its corners, so only its middle is opened.
    g.globalCompositeOperation = 'source-over'; g.clearRect(0, 0, cv, cv); g.fillStyle = '#fff'; g.beginPath(); g.arc(cx, cy, lay.r * 0.3, 0, 7); g.fill();
    for (var q = 0; q < 3; q++) { var qa = q / 3 * 6.2832 + 0.5; screws.push([cx + Math.cos(qa) * lay.r * 0.42, cy + Math.sin(qa) * lay.r * 0.42]); }
  } else {
    var d = dials[0];
    g.beginPath(); g.arc(d.x, d.y, d.r + 4.5 * k, 0, 7); g.fill();
    // A bridge runs from the dial to the middle of each edge, so the windows fall in the corners.
    for (var i = 0; i < n; i++) {
      var a = frame[i], b = frame[(i + 1) % n], mx = (a[0] + b[0]) / 2, my = (a[1] + b[1]) / 2;
      g.lineWidth = 8 * k; g.beginPath(); g.moveTo(d.x, d.y); g.lineTo(mx + (mx - d.x) * 0.3, my + (my - d.y) * 0.3); g.stroke();
      bridges.push([d.x, d.y, mx, my]);
      var L = Math.hypot(mx - d.x, my - d.y), ux = (mx - d.x) / L, uy = (my - d.y) / L;
      screws.push([mx + ux * 2.5 * k, my + uy * 2.5 * k]);
    }
  }
  clkCache[key] = { win: win, dials: dials, bridges: bridges, screws: screws, k: k };
  return clkCache[key];
}
function clkTint(img, col) {
  var c = canvas(img.width), g = c.getContext('2d');
  g.drawImage(img, 0, 0); g.globalCompositeOperation = 'source-in'; g.fillStyle = col; g.fillRect(0, 0, c.width, c.width); return c;
}
function clkScrews(g, geo, r) { geo.screws.forEach(function (s) { g.beginPath(); g.arc(s[0], s[1], r, 0, 7); g.fill(); }); }

// ---------- The train ----------
var clkGears = {};
function clkGearGeo(T, teeth) {
  if (clkGears[teeth]) return clkGears[teeth];
  var sh = new T.Shape(), r1 = 1, r0 = 1 - 2.4 / teeth, i, a;
  for (i = 0; i < teeth; i++) {
    a = i / teeth * 6.2832; var st = 6.2832 / teeth;
    var pts = [[r0, a], [r0, a + st * 0.22], [r1, a + st * 0.36], [r1, a + st * 0.64], [r0, a + st * 0.78]];
    pts.forEach(function (p, j) { var x = Math.cos(p[1]) * p[0], y = Math.sin(p[1]) * p[0]; if (!i && !j) sh.moveTo(x, y); else sh.lineTo(x, y); });
  }
  sh.closePath();
  // Crossed out like a watch wheel: five openings leave a rim, a hub and five spokes.
  for (i = 0; i < 5; i++) {
    var h = new T.Path(), a0 = i / 5 * 6.2832 + 0.2, a1 = (i + 1) / 5 * 6.2832 - 0.2, ri = 0.3, ro = r0 - 0.16, j;
    for (j = 0; j <= 6; j++) { a = a0 + (a1 - a0) * j / 6; if (j) h.lineTo(Math.cos(a) * ro, Math.sin(a) * ro); else h.moveTo(Math.cos(a) * ro, Math.sin(a) * ro); }
    for (j = 6; j >= 0; j--) { a = a0 + 0.12 + (a1 - a0 - 0.24) * j / 6; h.lineTo(Math.cos(a) * ri, Math.sin(a) * ri); }
    h.closePath(); sh.holes.push(h);
  }
  var geo = new T.ExtrudeGeometry(sh, { depth: 0.16, bevelEnabled: false, curveSegments: 4 });
  geo.translate(0, 0, -0.08);
  clkGears[teeth] = geo; return geo;
}
var clkCount = 0;
function clkBuild(T, die, q) {
  var c = die.th, hd = q.detail !== 'basic', rnd = mulberry(++clkCount * 331 + die.sides * 7), R = die.rest, root = new T.Group(), wheels = [], mats = [];
  c.gears.forEach(function (h, i) { mats.push(new T.MeshStandardMaterial({ color: new T.Color(h), metalness: 0.9, roughness: 0.32 + i * 0.05, side: T.DoubleSide })); });
  // [teeth, size, where along the arbor, which way it turns and how fast]
  var plan = [[24, 0.74, 0, 1], [12, 0.4, 0.34, -2], [16, 0.52, -0.3, -1.5], [10, 0.3, -0.56, 2.4], [18, 0.56, 0.58, 1.3]].slice(0, hd ? 5 : 3);
  // Three arbors, each tilted its own way, so some wheel faces every window.
  var arbors = [0, 1, 2].map(function (i) { var gp = new T.Group(); gp.rotation.set(rnd() * 6.28, rnd() * 6.28, rnd() * 6.28); if (i === 1) gp.rotateX(Math.PI / 2); if (i === 2) gp.rotateY(Math.PI / 2); root.add(gp); return gp; });
  plan.forEach(function (p, i) {
    var m = new T.Mesh(clkGearGeo(T, p[0]), mats[i % mats.length]); m.scale.set(R * p[1], R * p[1], R * 0.5);
    m.position.z = R * p[2] * 0.62; arbors[i % 3].add(m); wheels.push({ m: m, speed: p[3], at: rnd() * 6.28 });
  });
  var t = rnd() * 9, s = 0, turn = 0;
  return { parts: [root], tick: function (dt, spin) {
    dt = Math.min(dt, 0.05); t += dt;
    var want = Math.min(1, Math.max(0, spin / 10)); s += (want - s) * (1 - Math.exp(-dt * (want > s ? 9 : 2)));
    // At rest the train moves a tooth at a time, four beats a second, each beat a quick step and a stop.
    var beat = t * 4, step = Math.floor(beat) + Math.min(1, (beat - Math.floor(beat)) * 5);
    turn += dt * 5 * s;
    wheels.forEach(function (w) { w.m.rotation.z = w.at + (turn + step * 0.045) * w.speed; });
  }, dispose: function () { mats.forEach(function (m) { m.dispose(); }); } };
}

registerSet({
  key: 'clockwork', name: 'Clockwork', note: 'Brushed metal plates with windows cut through them and a train of gears turning inside: a whir while it tumbles, then a tick at a time.',
  show: 'molten', bev: 0.06, see: 'cutout', relief: 1.4,
  colours: CLOCKWORK,
  paint: function (g, lay, c, seed) {
    var cv = lay.cv, geo = clkGeom(lay), k = geo.k, w = strokeW(lay), rnd = mulberry(seed), P = c.plate;
    var gr = g.createLinearGradient(0, 0, cv, cv); gr.addColorStop(0, mix(P[0], P[1], 0.55)); gr.addColorStop(0.5, P[1]); gr.addColorStop(1, mix(P[1], P[2], 0.4));
    g.fillStyle = gr; g.fillRect(0, 0, cv, cv);
    // Brushed: fine straight scratches all one way.
    g.lineWidth = 0.5 * k;
    for (var i = 0; i < 260; i++) { var y = rnd() * cv, x = rnd() * cv, l = (20 + rnd() * 70) * k; g.strokeStyle = hexA(rnd() > 0.5 ? P[0] : P[2], 0.16); g.beginPath(); g.moveTo(x, y); g.lineTo(x + l, y + l * 0.06); g.stroke(); }
    // The dial: a ring of minute marks round a turned centre.
    geo.dials.forEach(function (d) {
      g.strokeStyle = hexA(P[2], 0.75); g.lineWidth = 1.1 * k; g.beginPath(); g.arc(d.x, d.y, d.r, 0, 7); g.stroke();
      g.strokeStyle = hexA(P[0], 0.5); g.lineWidth = 0.6 * k; g.beginPath(); g.arc(d.x, d.y, d.r - 1.6 * k, 0, 7); g.stroke();
      for (var j = 0; j < 36; j++) { var a = j / 36 * 6.2832, r0 = d.r + (j % 3 ? 1.2 : 0.6) * k, r1 = d.r + 3.4 * k; g.strokeStyle = hexA(P[2], 0.8); g.lineWidth = (j % 3 ? 0.6 : 1.1) * k; g.beginPath(); g.moveTo(d.x + Math.cos(a) * r0, d.y + Math.sin(a) * r0); g.lineTo(d.x + Math.cos(a) * r1, d.y + Math.sin(a) * r1); g.stroke(); }
    });
    // An engraved line along the face's edge.
    g.strokeStyle = hexA(P[2], 0.7); g.lineWidth = 1 * k; clkTrace(g, clkInset(lay, 0.9)); g.stroke();
    // Screws: a dark seat, a bright head, a slot.
    g.fillStyle = hexA(P[2], 0.9); clkScrews(g, geo, 3.5 * k);
    g.fillStyle = mix(P[0], '#ffffff', 0.3); clkScrews(g, geo, 2.7 * k);
    g.strokeStyle = hexA(P[2], 0.95); g.lineWidth = 0.9 * k; geo.screws.forEach(function (s, j) { var a = j * 1.9; g.beginPath(); g.moveTo(s[0] - Math.cos(a) * 2.4 * k, s[1] - Math.sin(a) * 2.4 * k); g.lineTo(s[0] + Math.cos(a) * 2.4 * k, s[1] + Math.sin(a) * 2.4 * k); g.stroke(); });
    // The number: engraved, and filled with enamel.
    g.fillStyle = g.strokeStyle = hexA(P[2], 0.95); paintNumeral(g, lay, lay.style, w * 1.34);
    g.fillStyle = g.strokeStyle = c.num; paintNumeral(g, lay, lay.style, w);
    g.fillStyle = g.strokeStyle = hexA(mix(c.num, '#ffffff', 0.6), 0.5); g.save(); g.translate(-w * 0.1, -w * 0.16); paintNumeral(g, lay, lay.style, w * 0.2); g.restore();
    // Shade the plate round each window, then cut the windows through.
    g.save(); g.globalCompositeOperation = 'source-atop'; g.globalAlpha = 0.75; try { g.filter = 'blur(' + (3 * k) + 'px)'; } catch (e) {}
    g.drawImage(clkTint(geo.win, 'rgb(10,6,2)'), 0, 0); g.restore();
    g.save(); g.globalCompositeOperation = 'destination-out'; g.drawImage(geo.win, 0, 0); g.restore();
  },
  // Metal plate; the enamel in the number is glassy and not metal.
  surface: function (g, lay, c) {
    var cv = lay.cv, geo = clkGeom(lay), k = geo.k, w = strokeW(lay);
    g.fillStyle = 'rgb(0,' + Math.round(c.base.rough * 255) + ',200)'; g.fillRect(0, 0, cv, cv);
    g.fillStyle = 'rgb(0,50,225)'; clkScrews(g, geo, 2.7 * k);
    g.fillStyle = g.strokeStyle = 'rgb(0,46,0)'; paintNumeral(g, lay, lay.style, w);
  },
  height: function (g, lay) {
    var cv = lay.cv, geo = clkGeom(lay), k = geo.k, w = strokeW(lay);
    g.fillStyle = '#a0a0a0'; g.fillRect(0, 0, cv, cv);
    g.save(); try { g.filter = 'blur(' + (0.6 * k) + 'px)'; } catch (e) {}
    geo.dials.forEach(function (d) { g.strokeStyle = '#505050'; g.lineWidth = 1.2 * k; g.beginPath(); g.arc(d.x, d.y, d.r, 0, 7); g.stroke(); });
    g.strokeStyle = '#585858'; g.lineWidth = 1 * k; clkTrace(g, clkInset(lay, 0.9)); g.stroke();
    g.fillStyle = '#383838'; clkScrews(g, geo, 3.5 * k); g.fillStyle = '#f4f4f4'; clkScrews(g, geo, 2.7 * k);
    g.fillStyle = g.strokeStyle = '#404040'; paintNumeral(g, lay, lay.style, w * 1.34);
    g.fillStyle = g.strokeStyle = '#787878'; paintNumeral(g, lay, lay.style, w * 0.9);
    // The plate rolls off a little into each window.
    g.restore(); g.save(); try { g.filter = 'blur(' + (1.6 * k) + 'px)'; } catch (e) {}
    g.globalAlpha = 0.6; g.drawImage(clkTint(geo.win, '#202020'), 0, 0); g.restore();
  },
  rim: function (c) { return { col: mix(c.plate[1], c.plate[2], 0.35), rough: 0.4, metal: 0.9 }; },
  physical: function () { return { envMapIntensity: 1.4, specularIntensity: 1, alphaToCoverage: true }; },
  build: clkBuild,
  cover: function () { return 0; }
});
