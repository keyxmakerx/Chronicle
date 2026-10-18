// ==================== Dragonscale ====================
// Overlapping metal scales, each a raised, rounded shield whose free edge catches the light and
// shades the row below it. The colour map shifts hue scale by scale and a thin-film (iridescent)
// coat shifts it again with the angle. The number sits on a smooth polished plate in the middle of
// the face (three small plates on a d4), cut in and filled with a lighter metal.
import {
  RIMPATCH, hexA, mix, mulberry, paintNumeral, registerSet, strokeW
} from '../engine.js';

// ---------- Colours ----------
// scales: [colour at one end of the shift, colour at the other, bright edge]; plate: [centre, edge]
// of the polished plate; metal.color is what the Molten show's gleam reads.
var SCALECOL = [
  { key: 'red', irid: 0.3, name: 'Red dragon', note: 'Crimson scales shifting to ember, gold numbers',
    scales: ['#c4161f', '#ff6a2a', '#ffc08f'], plate: ['#9a1019', '#420509'], ink: '#ffe2a6', burn: '#ff6a2a', metal: { color: '#ff8a3a', rough: 0.3 },
    base: { grad: ['#b3141c', '#4a0509'], rough: 0.36 } },
  { key: 'green', irid: 0.3, name: 'Green dragon', note: 'Emerald scales shifting to lime, pale numbers',
    scales: ['#12934f', '#9be640', '#dcffa8'], plate: ['#0d7a42', '#04301a'], ink: '#f2ffd2', burn: '#9dff5a', metal: { color: '#b4f060', rough: 0.3 },
    base: { grad: ['#14864c', '#06301c'], rough: 0.36 } },
  { key: 'blue', irid: 0.3, name: 'Blue dragon', note: 'Sapphire scales shifting to ice, pale numbers',
    scales: ['#2149d0', '#3fd2f5', '#c9efff'], plate: ['#1a3aa8', '#081652'], ink: '#e2f5ff', burn: '#5fd2ff', metal: { color: '#69d4ff', rough: 0.3 },
    base: { grad: ['#2145c0', '#0a1a58'], rough: 0.36 } },
  { key: 'gold', irid: 0.1, name: 'Gold dragon', note: 'Bright gold scales shifting to copper, pale numbers',
    scales: ['#e0a020', '#fff1a6', '#fff6d0'], plate: ['#a87818', '#472c06'], ink: '#fff3c6', burn: '#ffcf5a', metal: { color: '#ffd25a', rough: 0.28 },
    base: { grad: ['#e8b84a', '#8a5a10'], rough: 0.34 } },
  { key: 'black', irid: 0.75, name: 'Black dragon', note: 'Black scales with an oil-slick sheen, silver numbers',
    scales: ['#1c1f2a', '#5559c8', '#aab2ff'], plate: ['#1a1c28', '#07080e'], ink: '#e9edff', burn: '#9fa8ff', metal: { color: '#a8b0ff', rough: 0.3 },
    base: { grad: ['#2a2c38', '#07080c'], rough: 0.36 } }
];

// ---------- The scales ----------
// Rows run across the face, offset by half a scale. Back rows are drawn first so each row's free
// edges land on top of the row below, and the same grid serves the colour, height and surface maps.
function sclSeed(lay) { return lay.cell ? lay.cell.x * 31 + lay.cell.y * 17 + lay.cv : lay.cv; }
function sclEach(cv, fn) {
  var W = cv * 0.165, P = W * 0.55, rx = W * 0.56, ry = W * 0.7, rows = Math.ceil(cv / P) + 3, cols = Math.ceil(cv / W) + 3, i, j;
  for (j = rows - 1; j >= 0; j--) for (i = -1; i < cols; i++) fn(i * W + (j % 2 ? W / 2 : 0), j * P - P, rx, ry, i, j);
}
// A shield: flat top (hidden under the row above), rounded shoulders, a soft point at the free edge.
function sclShape(g, x, y, rx, ry) {
  g.beginPath(); g.moveTo(x - rx, y - ry); g.lineTo(x - rx, y + ry * 0.1);
  g.bezierCurveTo(x - rx, y + ry * 0.62, x - rx * 0.42, y + ry * 0.86, x, y + ry);
  g.bezierCurveTo(x + rx * 0.42, y + ry * 0.86, x + rx, y + ry * 0.62, x + rx, y + ry * 0.1);
  g.lineTo(x + rx, y - ry); g.closePath();
}
// Just the free edge, for the bright line along it.
function sclTip(g, x, y, rx, ry) {
  g.beginPath(); g.moveTo(x - rx, y + ry * 0.1);
  g.bezierCurveTo(x - rx, y + ry * 0.62, x - rx * 0.42, y + ry * 0.86, x, y + ry);
  g.bezierCurveTo(x + rx * 0.42, y + ry * 0.86, x + rx, y + ry * 0.62, x + rx, y + ry * 0.1);
}

// ---------- The plate ----------
// One rounded plate shaped like the face and sized to hold the number, or on a d4 one small rounded
// plate round each corner number. grow (in px) widens or narrows it for the bevel and the inner line.
function sclPlates(lay) {
  var cv = lay.cv, w = strokeW(lay), out = [];
  if (lay.sub) {
    lay.sub.forEach(function (sl) {
      var c = Math.cos(sl.rot), s = Math.sin(sl.rot), x0 = 1e9, x1 = -1e9, y0 = 1e9, y1 = -1e9;
      sl.strokes.forEach(function (pl) { pl.forEach(function (p) {
        var dx = p[0] - sl.cx, dy = p[1] - sl.cy, u = c * dx + s * dy, v = -s * dx + c * dy;
        if (u < x0) x0 = u; if (u > x1) x1 = u; if (v < y0) y0 = v; if (v > y1) y1 = v;
      }); });
      var pad = w * 1.0 + sl.h * 0.06;
      out.push({ rect: true, nx: sl.cx, ny: sl.cy, rot: sl.rot, ox: (x0 + x1) / 2, oy: (y0 + y1) / 2, hw: (x1 - x0) / 2 + pad, hh: (y1 - y0) / 2 + pad, rho: Math.min(w * 1.6, (x1 - x0) / 2 + pad) });
    });
    return out;
  }
  var C = [cv / 2, cv / 2], poly = lay.inner, n = poly.length, nrm = [], x0 = 1e9, x1 = -1e9, y0 = 1e9, y1 = -1e9, i;
  for (i = 0; i < n; i++) {
    var a = poly[i], b = poly[(i + 1) % n], ex = b[0] - a[0], ey = b[1] - a[1], L = Math.hypot(ex, ey) || 1, nx = ey / L, ny = -ex / L;
    if (nx * ((a[0] + b[0]) / 2 - C[0]) + ny * ((a[1] + b[1]) / 2 - C[1]) < 0) { nx = -nx; ny = -ny; }
    nrm.push([nx, ny, nx * (a[0] - C[0]) + ny * (a[1] - C[1])]);
  }
  lay.strokes.forEach(function (pl) { pl.forEach(function (p) { if (p[0] < x0) x0 = p[0]; if (p[0] > x1) x1 = p[0]; if (p[1] < y0) y0 = p[1]; if (p[1] > y1) y1 = p[1]; }); });
  // The plate is the face's own outline moved in by the same distance on every side, as far as the
  // number (and a margin) allows, so a kite stays a kite and the rim of scales has an even width.
  var pad = w * 0.8 + lay.h * 0.03, inset = 1e9;
  nrm.forEach(function (m) {
    var need = -1e9;
    [[x0 - pad, y0 - pad], [x1 + pad, y0 - pad], [x0 - pad, y1 + pad], [x1 + pad, y1 + pad]].forEach(function (p) { need = Math.max(need, m[0] * (p[0] - C[0]) + m[1] * (p[1] - C[1])); });
    inset = Math.min(inset, m[2] - need);
  });
  out.push({ rect: false, nrm: nrm, poly: poly, inset: Math.max(cv * 0.045, inset), rho: cv * 0.12 });
  return out;
}
// The face outline with every edge moved in by d (negative: out); each corner is where its two moved edges meet.
function sclInset(p, d) {
  var n = p.poly.length, pts = [], i;
  for (i = 0; i < n; i++) {
    var m0 = p.nrm[(i + n - 1) % n], m1 = p.nrm[i], k = 1 + m0[0] * m1[0] + m0[1] * m1[1], v = p.poly[i];
    pts.push([v[0] - d * (m0[0] + m1[0]) / k, v[1] - d * (m0[1] + m1[1]) / k]);
  }
  return pts;
}
function sclPlatePath(g, p, grow) {
  var i, n;
  if (p.rect) {
    var hw = Math.max(1, p.hw + grow), hh = Math.max(1, p.hh + grow), r = Math.min(p.rho + grow * 0.5, hw, hh), x = p.ox, y = p.oy;
    g.save(); g.translate(p.nx, p.ny); g.rotate(p.rot);
    g.beginPath(); g.moveTo(x - hw + r, y - hh); g.arcTo(x + hw, y - hh, x + hw, y + hh, r); g.arcTo(x + hw, y + hh, x - hw, y + hh, r);
    g.arcTo(x - hw, y + hh, x - hw, y - hh, r); g.arcTo(x - hw, y - hh, x + hw, y - hh, r); g.closePath();
    g.restore(); return;
  }
  var pts = sclInset(p, p.inset - grow), rho = Math.max(1, p.rho + grow * 0.5);
  n = pts.length; g.beginPath(); g.moveTo((pts[n - 1][0] + pts[0][0]) / 2, (pts[n - 1][1] + pts[0][1]) / 2);
  for (i = 0; i < n; i++) { var q = pts[(i + 1) % n]; g.arcTo(pts[i][0], pts[i][1], (pts[i][0] + q[0]) / 2, (pts[i][1] + q[1]) / 2, rho); }
  g.closePath();
}

registerSet({
  key: 'scale', name: 'Dragonscale', show: 'molten', bev: 0.075, see: null, relief: 2.2,
  note: 'Overlapping metal scales with a colour-shifting sheen. The number on a smooth polished plate in the middle of each face.',
  highlightWhat: 'The metal in the number runs molten, white-hot, then cools to a bright gleam.',
  colours: SCALECOL,

  paint: function (g, lay, c) {
    var cv = lay.cv, k = cv / 256, w = strokeW(lay), seed = sclSeed(lay), a = c.scales[0], b = c.scales[1], edge = c.scales[2], plates = sclPlates(lay);
    g.fillStyle = mix(a, '#000000', 0.72); g.fillRect(0, 0, cv, cv);
    g.lineJoin = 'round';
    sclEach(cv, function (x, y, rx, ry, i, j) {
      var r = mulberry(seed + i * 91 + j * 17 + 5), u = Math.max(0, Math.min(1, (x + y * 0.7) / (cv * 1.5) + (r() - 0.5) * 0.55 + 0.1)), lum = 0.82 + r() * 0.36;
      var col = mix(a, b, u); col = lum < 1 ? mix(col, '#000000', 1 - lum) : mix(col, '#ffffff', (lum - 1) * 0.4);
      var gr = g.createLinearGradient(x, y - ry, x, y + ry);
      gr.addColorStop(0, mix(col, '#000000', 0.55)); gr.addColorStop(0.45, mix(col, '#000000', 0.1)); gr.addColorStop(1, mix(col, edge, 0.3));
      g.save(); g.shadowColor = 'rgba(0,0,0,.6)'; g.shadowBlur = 4 * k; g.shadowOffsetY = 2.5 * k;
      sclShape(g, x, y, rx, ry); g.fillStyle = gr; g.fill(); g.restore();
      g.strokeStyle = 'rgba(0,0,0,.5)'; g.lineWidth = 1.3 * k; sclShape(g, x, y, rx, ry); g.stroke();
      g.save(); g.translate(0, -1.3 * k); g.strokeStyle = hexA(edge, 0.7); g.lineWidth = 1.2 * k; sclTip(g, x, y, rx * 0.97, ry * 0.97); g.stroke(); g.restore();
      g.strokeStyle = hexA(edge, 0.16); g.lineWidth = 1.1 * k; g.beginPath(); g.moveTo(x, y - ry * 0.1); g.lineTo(x, y + ry * 0.78); g.stroke();
    });
    plates.forEach(function (p) {
      g.save(); g.shadowColor = 'rgba(0,0,0,.75)'; g.shadowBlur = 9 * k; g.shadowOffsetY = 2 * k; sclPlatePath(g, p, 3.4 * k); g.fillStyle = '#000'; g.fill(); g.restore();
      sclPlatePath(g, p, 3.4 * k); g.fillStyle = mix(b, '#ffffff', 0.3); g.fill();
      sclPlatePath(g, p, 1.6 * k); g.fillStyle = '#000'; g.fill();
      sclPlatePath(g, p, 0); var pg = g.createRadialGradient(lay.cx, lay.cy, 0, lay.cx, lay.cy, cv * 0.45); pg.addColorStop(0, c.plate[0]); pg.addColorStop(1, c.plate[1]); g.fillStyle = pg; g.fill();
      sclPlatePath(g, p, -3 * k); g.strokeStyle = hexA(edge, 0.3); g.lineWidth = 1 * k; g.stroke();
    });
    g.fillStyle = g.strokeStyle = 'rgba(0,0,0,.8)'; paintNumeral(g, lay, lay.style, w * 1.45);
    g.fillStyle = g.strokeStyle = c.ink; paintNumeral(g, lay, lay.style, w);
  },

  // Metal everywhere; the plate is polished, the scale edges a little brighter than their faces, and
  // the number's fill is half enamel so the room's reflections cannot empty it.
  surface: function (g, lay, c) {
    var cv = lay.cv, k = cv / 256, rough = c.base.rough, seed = sclSeed(lay), plates = sclPlates(lay), w = strokeW(lay), MET = 222;
    g.fillStyle = 'rgb(0,' + Math.round(rough * 255) + ',' + MET + ')'; g.fillRect(0, 0, cv, cv);
    sclEach(cv, function (x, y, rx, ry, i, j) {
      var r = mulberry(seed + i * 91 + j * 17 + 9), rr = rough * (0.85 + r() * 0.3);
      sclShape(g, x, y, rx, ry); g.fillStyle = 'rgb(0,' + Math.round(rr * 255) + ',' + MET + ')'; g.fill();
      sclTip(g, x, y, rx * 0.97, ry * 0.97); g.strokeStyle = 'rgb(0,' + Math.round(rr * 0.65 * 255) + ',' + MET + ')'; g.lineWidth = 2 * k; g.stroke();
    });
    plates.forEach(function (p) { sclPlatePath(g, p, 3.4 * k); g.fillStyle = 'rgb(0,' + Math.round(rough * 0.6 * 255) + ',' + MET + ')'; g.fill(); sclPlatePath(g, p, 0); g.fillStyle = 'rgb(0,50,' + (MET + 10) + ')'; g.fill(); });
    g.fillStyle = g.strokeStyle = 'rgb(0,100,90)'; paintNumeral(g, lay, lay.style, w * 1.05);
  },

  // Scales are domes with a raised free edge, dimmed where the row above covers them; the plate is
  // flat and stands proud with a small bevel; the number is cut into it.
  height: function (g, lay, c) {
    var cv = lay.cv, k = cv / 256, plates = sclPlates(lay), w = strokeW(lay);
    g.fillStyle = '#222'; g.fillRect(0, 0, cv, cv);
    sclEach(cv, function (x, y, rx, ry) {
      var gr = g.createLinearGradient(x, y - ry, x, y + ry);
      gr.addColorStop(0, '#303030'); gr.addColorStop(0.5, '#9a9a9a'); gr.addColorStop(1, '#d8d8d8');
      g.save(); g.shadowColor = 'rgba(0,0,0,.9)'; g.shadowBlur = 4 * k; g.shadowOffsetY = 2.5 * k;
      sclShape(g, x, y, rx, ry); g.fillStyle = gr; g.fill(); g.restore();
      g.save(); try { g.filter = 'blur(' + (0.7 * k) + 'px)'; } catch (e) {} sclTip(g, x, y, rx * 0.96, ry * 0.96); g.strokeStyle = '#fff'; g.lineWidth = 2.2 * k; g.stroke(); g.restore();
    });
    g.save(); try { g.filter = 'blur(' + (1.3 * k) + 'px)'; } catch (e) {}
    plates.forEach(function (p) { sclPlatePath(g, p, 3.4 * k); g.fillStyle = '#707070'; g.fill(); sclPlatePath(g, p, 0); g.fillStyle = '#fff'; g.fill(); });
    g.restore();
    g.save(); try { g.filter = 'blur(' + (0.6 * k) + 'px)'; } catch (e) {} g.fillStyle = g.strokeStyle = '#c8c8c8'; paintNumeral(g, lay, lay.style, w * 1.1); g.restore();
    // The rim samples one point of the corner patch, where the surface must be flat: a slope there has no texture direction to bend along.
    g.fillStyle = '#808080'; g.fillRect(0, 0, Math.ceil(cv * RIMPATCH) + 10 * k, Math.ceil(cv * RIMPATCH) + 10 * k);
  },

  rim: function (c) { return { col: mix(mix(c.scales[0], c.scales[1], 0.35), '#000000', 0.4), rough: 0.42, metal: 1 }; },
  // Real metal under a thin film: the film's colour shifts with the angle, and with each scale's own
  // tint, so a die flashes through its two colours as it turns.
  physical: function (c, q) { var hd = q.detail !== 'basic'; return { iridescence: hd ? c.irid : 0, iridescenceIOR: 1.7, iridescenceThicknessRange: [180, 520], envMapIntensity: 1.6 }; },
  shader: function (s) {
    s.fragmentShader = s.fragmentShader.replace('material.iridescenceThickness = iridescenceThicknessMaximum;',
      'material.iridescenceThickness = mix( iridescenceThicknessMinimum, iridescenceThicknessMaximum, clamp( dot( diffuseColor.rgb, vec3( 0.9, 0.7, 0.5 ) ) * 0.8, 0.0, 1.0 ) );');
  },
  cover: function () { return 0.65; }
});
