// ---------- Ember obsidian ----------
// Black volcanic glass with faint conchoidal ripples and a network of hairline cracks that glow like
// magma. The glow is painted into the face colour in hot colours and the material's shader turns
// anything hot into light (the shared emissive map is wired to the rim only), so it needs no extra
// texture and follows the face wherever the die goes. The number is a polished black inlay sitting in a
// plate of that same light: dark ink on a glow reads on a white page and a dark one alike, and the cracks
// run up to the plate as though they lit it.
import {
  hexA, mix, mulberry, paintNumeral, polyPath, registerSet, strokePaths, strokeW
} from '../engine.js';

// Body colours stay dark in every channel: the shader reads a bright pixel as heat, a dark one as glass.
var OBS_HOT = { lo: 0.09, hi: 0.34 };

// ---------- Geometry helpers (canvas px) ----------
function obsInside(poly, x, y) {
  var ins = false;
  for (var i = 0, j = poly.length - 1; i < poly.length; j = i++) {
    var xi = poly[i][0], yi = poly[i][1], xj = poly[j][0], yj = poly[j][1];
    if ((yi > y) !== (yj > y) && x < (xj - xi) * (y - yi) / (yj - yi) + xi) ins = !ins;
  }
  return ins;
}
function obsSegDist(x, y, a, b) {
  var dx = b[0] - a[0], dy = b[1] - a[1], l2 = dx * dx + dy * dy, t = l2 ? Math.max(0, Math.min(1, ((x - a[0]) * dx + (y - a[1]) * dy) / l2)) : 0;
  return Math.hypot(x - a[0] - dx * t, y - a[1] - dy * t);
}
// Distance from a point to the nearest stroke of the number (the 6 and 9 dot included).
function obsDistToNumber(lay, x, y) {
  var d = 1e9;
  lay.strokes.forEach(function (pl) { for (var i = 1; i < pl.length; i++) d = Math.min(d, obsSegDist(x, y, pl[i - 1], pl[i])); });
  if (lay.label === '6' || lay.label === '9') d = Math.min(d, Math.hypot(x - lay.cx, y - (lay.cy + lay.h * 0.68)) - strokeW(lay) * 0.4);
  return d;
}

// ---------- The cracks ----------
// Cracks run in from the edge of the face, wander, fork, and stop where they meet the glowing plate round
// the number or leave the face. Generated once per face; paint and height both read the same set.
function obsCrackSet(lay, seed) {
  if (lay._obs) return lay._obs;
  var rnd = mulberry(seed * 13 + 5), k = lay.cv / 256, poly = lay.inner, cx = lay.cv / 2, cy = lay.cv / 2, w = strokeW(lay);
  var pad = w * 1.0, reach = w * 0.5 + pad * 0.9, out = [];
  function walk(x, y, ang, wd, life, heat, depth, inside) {
    var pts = [[x, y]], wds = [wd];
    for (var i = 0; i < life; i++) {
      ang += (rnd() - 0.5) * 0.7 + (rnd() < 0.09 ? (rnd() - 0.5) * 1.7 : 0);
      var step = (6 + rnd() * 8) * k, nx = x + Math.cos(ang) * step, ny = y + Math.sin(ang) * step;
      var dn = obsDistToNumber(lay, (x + nx) / 2, (y + ny) / 2), dm = obsDistToNumber(lay, nx, ny);
      if (dn < reach || dm < reach) { pts.push([nx, ny]); wds.push(wd * 0.7); break; }
      var now = obsInside(poly, nx, ny);
      if (!now && inside) { pts.push([nx, ny]); wds.push(wd * 0.6); break; }
      if (now) inside = true;
      x = nx; y = ny; wd *= 0.972; pts.push([x, y]); wds.push(wd);
      if (depth < 2 && out.length < 46 && rnd() < 0.08 + (depth ? 0 : 0.03))
        walk(x, y, ang + (rnd() < 0.5 ? -1 : 1) * (0.4 + rnd() * 0.75), wd * 0.72, Math.floor(life * (0.22 + rnd() * 0.3)), heat * (0.78 + rnd() * 0.2), depth + 1, true);
    }
    if (pts.length > 2) out.push({ pts: pts, w: wds, heat: heat });
  }
  var n = poly.length, per = 0, edges = [];
  poly.forEach(function (p, i) { var q = poly[(i + 1) % n], l = Math.hypot(q[0] - p[0], q[1] - p[1]); edges.push([p, q, l]); per += l; });
  var starts = 3 + Math.floor(rnd() * 2);
  for (var s = 0; s < starts; s++) {
    var u = (s + 0.2 + rnd() * 0.6) / starts * per, e = 0;
    while (e < n - 1 && u > edges[e][2]) { u -= edges[e][2]; e++; }
    var a = edges[e][0], b = edges[e][1], f = u / (edges[e][2] || 1), x0 = a[0] + (b[0] - a[0]) * f, y0 = a[1] + (b[1] - a[1]) * f;
    var ang0 = Math.atan2(cy - y0, cx - x0) + (rnd() - 0.5) * 1.4;
    walk(x0, y0, ang0, (1.5 + rnd() * 0.8) * k, 14 + Math.floor(rnd() * 16), 0.62 + rnd() * 0.38, 0, false);
  }
  // A couple of loose hairlines that start inside the face and run both ways.
  for (var h = 0; h < 2; h++) {
    var hx = cx + (rnd() - 0.5) * lay.cv * 0.6, hy = cy + (rnd() - 0.5) * lay.cv * 0.6, ha = rnd() * 6.28;
    if (!obsInside(poly, hx, hy) || obsDistToNumber(lay, hx, hy) < reach * 1.2) continue;
    walk(hx, hy, ha, (1.1 + rnd() * 0.6) * k, 7 + Math.floor(rnd() * 8), 0.45 + rnd() * 0.3, 1, true);
    walk(hx, hy, ha + Math.PI, (1.1 + rnd() * 0.6) * k, 7 + Math.floor(rnd() * 8), 0.45 + rnd() * 0.3, 1, true);
  }
  // Conchoidal shells: a few sets of curved ripples fanning out from a point near the edge.
  var shells = [];
  for (var q = 0; q < 4; q++) {
    var sx = cx + (rnd() - 0.5) * lay.cv * 1.1, sy = cy + (rnd() - 0.5) * lay.cv * 1.1, a0 = rnd() * 6.28;
    shells.push({ x: sx, y: sy, a0: a0, a1: a0 + 0.9 + rnd() * 1.3, r0: (14 + rnd() * 30) * k, d: (6 + rnd() * 6) * k, n: 5 + Math.floor(rnd() * 5) });
  }
  lay._obs = { cracks: out, shells: shells, pad: pad };
  return lay._obs;
}
// One crack in heat colours: a soft halo, a dark red edge, an orange body, a pale hot core.
function obsDrawCrack(g, cr, heat, k) {
  var n = cr.pts.length - 1, h = cr.heat;
  function pass(scale, col, a) {
    g.strokeStyle = hexA(col, a);
    for (var i = 0; i < n; i++) { g.lineWidth = Math.max(0.7, cr.w[i] * scale); g.beginPath(); g.moveTo(cr.pts[i][0], cr.pts[i][1]); g.lineTo(cr.pts[i + 1][0], cr.pts[i + 1][1]); g.stroke(); }
  }
  g.lineCap = 'round'; g.lineJoin = 'round';
  g.save(); g.shadowColor = hexA(heat.edge, 0.9); g.shadowBlur = 7 * k; pass(3.0, heat.edge, 0.2 * h); g.restore();
  pass(2.0, mix(heat.edge, '#000000', 0.15), 0.85 * Math.min(1, h + 0.2));
  pass(1.35, mix(heat.edge, heat.mid, h), 1);
  pass(0.6, mix(heat.mid, heat.core, h * h), 1);
}
// The number as one round-capped stroke of a given width (halo layers), dot of a 6 or 9 included.
function obsStrokeAll(g, lay, w) {
  g.lineCap = 'round'; g.lineJoin = 'round'; g.lineWidth = w;
  strokePaths(g, lay.strokes);
  if (lay.label === '6' || lay.label === '9') { g.beginPath(); g.arc(lay.cx, lay.cy + lay.h * 0.68, w * 0.62, 0, 7); g.fill(); }
}
// The plate of light the number sits in: hot beside the stroke, cooling outward, plus a soft bleed into the
// glass round it.
function obsPlate(g, lay, heat) {
  var w = strokeW(lay), pad = w * 1.0;
  g.save();
  try { g.filter = 'blur(' + (pad * 0.55) + 'px)'; } catch (e) {}
  g.fillStyle = g.strokeStyle = hexA(heat.edge, 0.55); obsStrokeAll(g, lay, w + pad * 3.0);
  g.restore();
  var bands = [[2.0, mix(heat.edge, '#000000', 0.2)], [1.55, heat.edge], [1.2, mix(heat.edge, heat.mid, 0.55)], [0.85, heat.mid], [0.5, mix(heat.mid, heat.core, 0.55)], [0.22, heat.core]];
  bands.forEach(function (b) { g.fillStyle = g.strokeStyle = b[1]; obsStrokeAll(g, lay, w + pad * b[0]); });
}

// ---------- Colours ----------
var OBS_COLOURS = [
  { key: 'magma', name: 'Magma', note: 'Black glass, cracks of orange fire', burn: '#ff8a2a', ink: '#ffb04a', flame: '#ff8a1c',
    base: { grad: ['#202026', '#08080b'], rough: 0.16 }, heat: { core: '#fff1b8', mid: '#ff8a1c', edge: '#b0270a' } },
  { key: 'ember', name: 'Ember', note: 'Dark red glass, deep red cracks', burn: '#ff4a36', ink: '#ff7a52', flame: '#e8301c',
    base: { grad: ['#2a1618', '#0a0405'], rough: 0.16 }, heat: { core: '#ffb090', mid: '#e8301c', edge: '#620609' } },
  { key: 'witchfire', name: 'Witchfire', note: 'Black glass, green fire in the cracks', burn: '#6dff7a', ink: '#b6ff9a', flame: '#3fe05a',
    base: { grad: ['#16231c', '#050907'], rough: 0.16 }, heat: { core: '#efffc8', mid: '#3fe05a', edge: '#0a642c' } },
  { key: 'soulfire', name: 'Soulfire', note: 'Blue-black glass, cold blue fire', burn: '#5ab8ff', ink: '#a8dcff', flame: '#3a9cff',
    base: { grad: ['#161c30', '#05060d'], rough: 0.16 }, heat: { core: '#e8f8ff', mid: '#3a9cff', edge: '#0c3a9c' } },
  { key: 'mahogany', name: 'Mahogany obsidian', note: 'Brown and red glass, amber cracks', burn: '#ff9a4a', ink: '#ffc27a', flame: '#f07a22',
    base: { grad: ['#472a1d', '#150907'], rough: 0.17 }, heat: { core: '#ffe4ac', mid: '#f07a22', edge: '#86220e' }, mottle: '#5a2a1a' }
];

// ---------- Materials ----------
// Emission from the face colour: a pixel brighter than glass turns into light of its own colour, and the
// diffuse light it would catch is taken away so it reads as fire rather than paint. A thin warm glow
// at the die's grazing edges is the glass thinning out. Dimmed with the body colour, so a crit that
// darkens the die darkens the fire with it.
function obsShader(s, c) {
  s.uniforms.uObsT = { value: 0 }; s.uniforms.uObsEdge = { value: this.emissive.clone().set(c.heat.edge) };
  this.userData.obsT = s.uniforms.uObsT;
  s.fragmentShader = 'uniform float uObsT; uniform vec3 uObsEdge;\n' + s.fragmentShader.replace('#include <lights_physical_fragment>',
    'vec3 obsC = sampledDiffuseColor.rgb;\n' +
    'float obsM = max( obsC.r, max( obsC.g, obsC.b ) ), obsHot = smoothstep( ' + OBS_HOT.lo.toFixed(3) + ', ' + OBS_HOT.hi.toFixed(3) + ', obsM );\n' +
    'float obsPulse = 0.93 + 0.07 * sin( uObsT * 1.4 + vMapUv.x * 31.0 + vMapUv.y * 23.0 );\n' +
    'diffuseColor.rgb *= 1.0 - 0.9 * obsHot;\n' +
    'totalEmissiveRadiance += obsC * obsHot * obsPulse * 2.1 * diffuse.g;\n' +
    'totalEmissiveRadiance += uObsEdge * pow( 1.0 - clamp( dot( normal, normalize( vViewPosition ) ), 0.0, 1.0 ), 3.0 ) * 0.22 * diffuse.g;\n' +
    '#include <lights_physical_fragment>');
}

registerSet({
  key: 'obsidian', name: 'Ember obsidian', note: 'Black volcanic glass with hairline cracks of heat. The number polished smooth, lit from the cracks.', show: 'kindle', bev: 0.04,
  colours: OBS_COLOURS,
  see: null, relief: 0.9,
  paint: function (g, lay, c, seed) {
    var cv = lay.cv, k = cv / 256, rnd = mulberry(seed), S = obsCrackSet(lay, seed), heat = c.heat, w = strokeW(lay);
    var gr = g.createRadialGradient(cv * 0.36, cv * 0.3, cv * 0.04, cv / 2, cv / 2, cv * 0.8);
    gr.addColorStop(0, c.base.grad[0]); gr.addColorStop(1, c.base.grad[1]); g.fillStyle = gr; g.fillRect(0, 0, cv, cv);
    // Glass has depth: faint smoky veils and a few specks, never bright enough to count as heat.
    g.save(); try { g.filter = 'blur(' + (7 * k) + 'px)'; } catch (e) {}
    for (var i = 0; i < (c.mottle ? 12 : 7); i++) {
      var x = rnd() * cv, y = rnd() * cv, r = (16 + rnd() * 44) * k, tone = c.mottle ? (rnd() < 0.6 ? c.mottle : '#0b0504') : '#3a3a46';
      g.fillStyle = hexA(tone, c.mottle ? 0.55 : 0.2 + rnd() * 0.2); g.beginPath(); g.ellipse(x, y, r, r * (0.4 + rnd() * 0.5), rnd() * 3, 0, 7); g.fill();
    }
    g.restore();
    g.save(); polyPath(g, lay.inner); g.clip();
    // The hot work, clipped to the face.
    S.cracks.forEach(function (cr) { obsDrawCrack(g, cr, heat, k); });
    // A little loose heat: a few bright specks beside the cracks.
    for (var p = 0; p < 10; p++) {
      var cr = S.cracks[Math.floor(rnd() * S.cracks.length)]; if (!cr) break;
      var j = Math.floor(rnd() * cr.pts.length), px = cr.pts[j][0] + (rnd() - 0.5) * 6 * k, py = cr.pts[j][1] + (rnd() - 0.5) * 6 * k;
      g.fillStyle = hexA(heat.mid, 0.5 + rnd() * 0.4); g.beginPath(); g.arc(px, py, (0.5 + rnd() * 0.7) * k, 0, 7); g.fill();
    }
    obsPlate(g, lay, heat);
    g.restore();
    // The number itself: polished black, no heat of its own.
    g.fillStyle = g.strokeStyle = '#08070a'; paintNumeral(g, lay, lay.style, w);
  },
  surface: function (g, lay, c) {
    var w = strokeW(lay), S = obsCrackSet(lay, 0), pad = S.pad;
    g.fillStyle = g.strokeStyle = 'rgb(0,34,0)'; obsStrokeAll(g, lay, w + pad * 2.2);
    g.fillStyle = g.strokeStyle = 'rgb(0,13,0)'; paintNumeral(g, lay, lay.style, w * 1.1);
  },
  height: function (g, lay, c) {
    var cv = lay.cv, k = cv / 256, S = obsCrackSet(lay, 0), w = strokeW(lay);
    g.fillStyle = '#c8c8c8'; g.fillRect(0, 0, cv, cv);
    g.save(); polyPath(g, lay.inner); g.clip(); try { g.filter = 'blur(' + (1.1 * k) + 'px)'; } catch (e) {}
    S.shells.forEach(function (sh) {
      for (var j = 0; j < sh.n; j++) {
        var fade = 1 - j / (sh.n + 1);
        g.lineWidth = sh.d * 0.5;
        g.strokeStyle = hexA('#6a6a6a', 0.5 * fade); g.beginPath(); g.arc(sh.x, sh.y, sh.r0 + j * sh.d, sh.a0, sh.a1); g.stroke();
        g.strokeStyle = hexA('#ffffff', 0.55 * fade); g.beginPath(); g.arc(sh.x, sh.y, sh.r0 + (j + 0.5) * sh.d, sh.a0, sh.a1); g.stroke();
      }
    });
    // The cracks are cut a little below the glass.
    g.strokeStyle = '#7c7c7c'; g.lineCap = 'round';
    S.cracks.forEach(function (cr) { for (var i = 0; i < cr.pts.length - 1; i++) { g.lineWidth = Math.max(1, cr.w[i] * 1.3); g.beginPath(); g.moveTo(cr.pts[i][0], cr.pts[i][1]); g.lineTo(cr.pts[i + 1][0], cr.pts[i + 1][1]); g.stroke(); } });
    g.restore();
    // The number and its plate are polished flat.
    g.fillStyle = g.strokeStyle = '#c8c8c8'; obsStrokeAll(g, lay, w + S.pad * 2.4);
  },
  rim: function (c) { return { col: mix(c.base.grad[0], '#6a6a78', 0.3), rough: 0.2, metal: 0.4 }; },
  physical: function (c, q) {
    var hd = q.detail !== 'basic';
    return { clearcoat: hd ? 0.55 : 0, clearcoatRoughness: 0.32, envMapIntensity: 0.85, specularIntensity: 0.9 };
  },
  shader: obsShader,
  build: function (T, die, q) {
    var t = 0;
    return { parts: [], tick: function (dt) { t += dt; var u = die.mat.userData.obsT; if (u) u.value = t; }, dispose: function () {} };
  }
});
