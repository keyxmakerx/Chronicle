// ---------- Crit fail: Frostbite ----------
// The 1 turns a sickly cold blue and freezing fog spills out from under the die. Frost ferns grow out
// across the page and rime creeps over the die; ice crystals erupt round it and a block of ice closes
// over it. Hairline cracks race through the ice, a beat, and it bursts into shards that skitter across
// the page and melt, leaving the die frosted with the 1 burning cold blue-white while the frost thaws.
import {
  canvas, clamp01, dimmer, drop, env4, fxLight, glow, glowFace, in3, inout, mix, mulberry,
  numberOnto, out3, registerCrit, shadeDie, smooth, spriteTex, starTex, strokeW, tintDie, worldOf, share
} from '../engine.js';

var FROST_ICE = '#f2fbff', FROST_GLACIER = '#9fdcff', FROST_DEEP = '#2c6fd1', FROST_SICK = '#62c7d9';

// Noise for the ice and frost shaders. Prefixed so it never collides with another effect's copy.
var FROST_GLSL = [
  'float frH(vec2 p){ return fract(sin(dot(p, vec2(127.1, 311.7))) * 43758.5453); }',
  'float frH3(vec3 p){ return fract(sin(dot(p, vec3(127.1, 311.7, 74.7))) * 43758.5453); }',
  'float frN(vec2 p){ vec2 i = floor(p), f = fract(p); vec2 u = f * f * (3.0 - 2.0 * f); return mix(mix(frH(i), frH(i + vec2(1.0, 0.0)), u.x), mix(frH(i + vec2(0.0, 1.0)), frH(i + vec2(1.0, 1.0)), u.x), u.y); }',
  'float frF(vec2 p){ float s = 0.0, a = 0.5; for (int i = 0; i < 4; i++) { s += a * frN(p); p = p * 2.03 + 1.7; a *= 0.5; } return s; }',
  'float frN3(vec3 p){ vec3 i = floor(p), f = fract(p); vec3 u = f * f * (3.0 - 2.0 * f);',
  ' float a = frH3(i), b = frH3(i + vec3(1.0, 0.0, 0.0)), c = frH3(i + vec3(0.0, 1.0, 0.0)), d = frH3(i + vec3(1.0, 1.0, 0.0));',
  ' float e = frH3(i + vec3(0.0, 0.0, 1.0)), f1 = frH3(i + vec3(1.0, 0.0, 1.0)), g = frH3(i + vec3(0.0, 1.0, 1.0)), h = frH3(i + vec3(1.0, 1.0, 1.0));',
  ' return mix(mix(mix(a, b, u.x), mix(c, d, u.x), u.y), mix(mix(e, f1, u.x), mix(g, h, u.x), u.y), u.z); }',
  'float frF3(vec3 p){ float s = 0.0, a = 0.5; for (int i = 0; i < 3; i++) { s += a * frN3(p); p = p * 2.07 + 1.3; a *= 0.5; } return s; }',
  // Distance to the nearest cell wall: straight-sided cells, which is how ice fractures.
  'float frCellEdge(vec2 p){ vec2 i = floor(p), f = fract(p); float d1 = 8.0, d2 = 8.0;',
  ' for (int y = -1; y <= 1; y++) for (int x = -1; x <= 1; x++) { vec2 g = vec2(float(x), float(y)); vec2 o = vec2(frH(i + g), frH(i + g + 17.3));',
  '  float d = length(g + o - f); if (d < d1) { d2 = d1; d1 = d; } else if (d < d2) { d2 = d; } }',
  ' return d2 - d1; }'].join('\n');

// Dice to keep in view (o.clear): up to eight places, each (x, z, radius) on the page. frClear is 0 inside
// the radius k and rises to 1 beyond about twice it, so a layer that reads it leaves a soft hole.
var FROST_CLEAR = [
  'uniform vec3 uClr[8];',
  'float frClear(vec2 w, float k){ float m = 1.0;',
  ' for (int i = 0; i < 8; i++) { float r = uClr[i].z * k; if (r > 0.0) m *= smoothstep(r * 0.8, r * 1.6, length(w - uClr[i].xy)); }',
  ' return m; }'].join('\n');
// The eight nearest to `at`, as the vec3s the shaders read (radius 0 is an unused slot).
function frostSpots(st, clear, at) {
  var T = st.T, out = [], k, near = (clear || []).slice().sort(function (a, b) { return Math.hypot(a.x - at.x, a.z - at.z) - Math.hypot(b.x - at.x, b.z - at.z); });
  for (k = 0; k < 8; k++) out.push(near[k] ? new T.Vector3(near[k].x, near[k].z, near[k].r) : new T.Vector3());
  return out;
}
// The page's darkening patch with a hole round each die to keep in view; with none it is the engine's own.
function frostDimmer(st, at, radius, clr) {
  var m = dimmer(st, at, radius);
  if (!clr) return m;
  m.material.onBeforeCompile = function (sh) {
    sh.uniforms.uClr = { value: clr };
    sh.vertexShader = sh.vertexShader.replace('#include <common>', '#include <common>\nvarying vec2 vHw;')
      .replace('#include <begin_vertex>', '#include <begin_vertex>\nvHw = (modelMatrix * vec4(transformed, 1.0)).xz;');
    sh.fragmentShader = sh.fragmentShader.replace('#include <common>', '#include <common>\nuniform vec3 uClr[8]; varying vec2 vHw;')
      .replace('#include <color_fragment>', '#include <color_fragment>\nfor (int i = 0; i < 8; i++) { float cr = uClr[i].z; if (cr > 0.0) diffuseColor.a *= smoothstep(cr * 0.75, cr * 1.6, length(vHw - uClr[i].xy)); }');
  };
  return m;
}

// A pale stone needs the number in a deeper blue: pale blue light on ivory has nothing to stand on.
function frostLightStone(th) {
  var h = th && th.base && th.base.grad && th.base.grad[0];
  if (!h || h.charAt(0) !== '#') return false;
  var n = parseInt(h.slice(1), 16);
  return (0.3 * (n >> 16 & 255) + 0.59 * (n >> 8 & 255) + 0.11 * (n & 255)) / 255 > 0.62;
}
// The 1 drawn as light into a face's glow canvas. On a pale stone it is solid colour without the white-hot
// core, since light can only add and pale light on ivory has nothing to stand on.
function frostNumeral(gf, pale, color) {
  gf.clear();
  if (pale && gf.lay.style !== 'classic') {
    var g2 = gf.g, w = strokeW(gf.lay);
    g2.lineJoin = 'round'; g2.lineCap = 'round'; g2.shadowColor = color; g2.shadowBlur = w; g2.strokeStyle = color; g2.lineWidth = w * 1.25;
    gf.lay.strokes.forEach(function (pl) { g2.beginPath(); g2.moveTo(pl[0][0], pl[0][1]); for (var q = 1; q < pl.length; q++) g2.lineTo(pl[q][0], pl[q][1]); g2.stroke(); });
    g2.shadowBlur = 0;
  } else numberOnto(gf.g, gf.lay, color);
}
function frostBackOut(x) { x = clamp01(x); var c1 = 1.9, c3 = c1 + 1; return 1 + c3 * Math.pow(x - 1, 3) + c1 * Math.pow(x - 1, 2); }

// A soft cloudy puff for the freezing fog, built once from value noise: the engine's smoke texture is
// a few round blobs, which read as bubbles when they lie flat and spread.
var FROSTFOG = null;
function frostFogTex(T) {
  if (FROSTFOG) return FROSTFOG;
  var N = 128, c = canvas(N), g = c.getContext('2d'), im = g.createImageData(N, N), rnd = mulberry(41), L = 32, lat = new Float32Array(L * L);
  for (var i = 0; i < L * L; i++) lat[i] = rnd();
  function vn(x, y) {
    var xi = Math.floor(x), yi = Math.floor(y), xf = x - xi, yf = y - yi; xf = xf * xf * (3 - 2 * xf); yf = yf * yf * (3 - 2 * yf);
    var a = lat[(yi & 31) * L + (xi & 31)], b = lat[(yi & 31) * L + (xi + 1 & 31)], c2 = lat[(yi + 1 & 31) * L + (xi & 31)], d = lat[(yi + 1 & 31) * L + (xi + 1 & 31)];
    return a + (b - a) * xf + (c2 - a) * yf + (a - b - c2 + d) * xf * yf;
  }
  for (var y = 0; y < N; y++) for (var x = 0; x < N; x++) {
    var u = x / N, v = y / N, r = Math.hypot(u - 0.5, v - 0.5) * 2, f = 0, amp = 0.5, fr = 3;
    for (var o = 0; o < 4; o++) { f += amp * vn(u * fr + o * 7.1, v * fr + o * 3.3); fr *= 2; amp *= 0.5; }
    var dn = clamp01((1 - smooth(0.05, 1, r)) * (f * 1.35 - 0.1)), k = (y * N + x) * 4;
    im.data[k] = im.data[k + 1] = im.data[k + 2] = Math.round(dn * 255); im.data[k + 3] = 255;
  }
  g.putImageData(im, 0, 0);
  FROSTFOG = new T.CanvasTexture(c); share(FROSTFOG);
  return FROSTFOG;
}

// The die's own outline as a closed shell, rebuilt from its face normals and centres alone (each face
// is a plane; corners are where three planes meet), so it works for any die the engine hands over.
// aEdge is 1 at a face's centre and 0 on its rim, which is where frost starts.
function frostHullGeo(T, die, grow) {
  var F = die.faces, P = F.map(function (fc) { return { n: fc.n, d: fc.n.dot(fc.c) }; }), V = [], eps = die.size * 1e-3;
  var a = new T.Vector3(), b = new T.Vector3(), c = new T.Vector3(), x = new T.Vector3();
  for (var i = 0; i < P.length; i++) for (var j = i + 1; j < P.length; j++) for (var k = j + 1; k < P.length; k++) {
    a.crossVectors(P[j].n, P[k].n); var det = P[i].n.dot(a);
    if (Math.abs(det) < 1e-6) continue;
    b.crossVectors(P[k].n, P[i].n); c.crossVectors(P[i].n, P[j].n);
    x.copy(a).multiplyScalar(P[i].d).addScaledVector(b, P[j].d).addScaledVector(c, P[k].d).divideScalar(det);
    var ok = true;
    for (var m = 0; m < P.length && ok; m++) if (P[m].n.dot(x) > P[m].d + eps) ok = false;
    if (ok && !V.some(function (v) { return v.distanceTo(x) < eps * 4; })) V.push(x.clone());
  }
  var pos = [], nrm = [], ed = [];
  F.forEach(function (fc, fi) {
    var on = V.filter(function (v) { return Math.abs(P[fi].n.dot(v) - P[fi].d) < eps * 4; });
    if (on.length < 3) return;
    var u = on[0].clone().sub(fc.c).normalize(), w = new T.Vector3().crossVectors(fc.n, u);
    on.sort(function (p, q) { var pp = p.clone().sub(fc.c), qq = q.clone().sub(fc.c); return Math.atan2(pp.dot(w), pp.dot(u)) - Math.atan2(qq.dot(w), qq.dot(u)); });
    for (var e = 0; e < on.length; e++) {
      var p1 = on[e], p2 = on[(e + 1) % on.length];
      pos.push(fc.c.x * grow, fc.c.y * grow, fc.c.z * grow, p1.x * grow, p1.y * grow, p1.z * grow, p2.x * grow, p2.y * grow, p2.z * grow);
      for (var q = 0; q < 3; q++) nrm.push(fc.n.x, fc.n.y, fc.n.z);
      ed.push(1, 0, 0);
    }
  });
  var geo = new T.BufferGeometry();
  geo.setAttribute('position', new T.Float32BufferAttribute(pos, 3)); geo.setAttribute('normal', new T.Float32BufferAttribute(nrm, 3));
  geo.setAttribute('aEdge', new T.Float32BufferAttribute(ed, 1));
  return geo;
}

// A hexagonal ice crystal, base at the origin and point at y = 1: six sides, a shoulder, and a point a
// little off true, the way real ones grow.
function frostPrismGeo(T) {
  var pos = [], H = 0.66, tip = [0.08, 1, -0.05], r0 = [], r1 = [];
  for (var k = 0; k < 6; k++) {
    var a = k / 6 * Math.PI * 2, r = 1 + (k % 2 ? 0.07 : -0.05);
    r0.push([Math.cos(a) * r, 0, Math.sin(a) * r]); r1.push([Math.cos(a) * r * 0.95, H + (k % 3 ? 0 : 0.06), Math.sin(a) * r * 0.95]);
  }
  function tri(p, q, s) { pos.push(p[0], p[1], p[2], q[0], q[1], q[2], s[0], s[1], s[2]); }
  for (k = 0; k < 6; k++) { var j = (k + 1) % 6; tri(r0[k], r1[k], r1[j]); tri(r0[k], r1[j], r0[j]); tri(r1[k], tip, r1[j]); }
  var geo = new T.BufferGeometry(); geo.setAttribute('position', new T.Float32BufferAttribute(pos, 3)); geo.computeVertexNormals();
  return geo;
}
// A flake of broken ice: a thin, uneven wedge with sharp edges.
function frostShardGeo(T) {
  var pos = [], top = [0.1, 0.3, 0.05], bot = [-0.08, -0.22, -0.04], rim = [[1, 0, 0.15], [0.2, 0.05, 0.85], [-0.7, -0.03, 0.45], [-0.85, 0.02, -0.35], [0.15, 0, -0.9]];
  function tri(p, q, s) { pos.push(p[0], p[1], p[2], q[0], q[1], q[2], s[0], s[1], s[2]); }
  for (var k = 0; k < rim.length; k++) { var j = (k + 1) % rim.length; tri(rim[k], top, rim[j]); tri(rim[k], rim[j], bot); }
  var geo = new T.BufferGeometry(); geo.setAttribute('position', new T.Float32BufferAttribute(pos, 3)); geo.computeVertexNormals();
  return geo;
}
// A rough block of ice round the die: a jittered sphere with a few flat cuts, so it reads as hewn ice
// and not a bubble, flattened where it sits on the page.
function frostBlockGeo(T, rnd, rx, ry, yc) {
  var geo = new T.IcosahedronGeometry(1, 1), P = geo.attributes.position, v = new T.Vector3(), cuts = [];
  for (var c = 0; c < 6; c++) cuts.push({ n: new T.Vector3(rnd() - 0.5, (rnd() - 0.2) * 0.9, rnd() - 0.5).normalize(), d: 0.8 + rnd() * 0.1 });
  for (var i = 0; i < P.count; i++) {
    v.fromBufferAttribute(P, i);
    // Jitter keyed on the rounded position, so the copies of a corner in each triangle move together.
    var h = Math.sin(Math.round(v.x * 1000) * 0.01299 + Math.round(v.y * 1000) * 0.07823 + Math.round(v.z * 1000) * 0.03771) * 43758.5453;
    v.multiplyScalar(0.94 + 0.12 * (h - Math.floor(h)));
    cuts.forEach(function (cu) { var e = v.dot(cu.n) - cu.d; if (e > 0) v.addScaledVector(cu.n, -e); });
    P.setXYZ(i, v.x * rx, Math.max(0.012, yc + v.y * ry), v.z * rx);
  }
  geo.computeVertexNormals();
  return geo;
}

// Frost ferns, in canvas px round the centre: stems that branch at about 60 degrees, the angle ice
// crystals branch at, each branch shorter toward the tip, so each arm grows into a feather. d is the
// distance along the growth path, which becomes the time that bit freezes.
function frostFernSegs(C, rnd, hd, r0) {
  var segs = [], tracks = [], step = C / (hd ? 190 : 125), cx = C / 2, maxR = C * 0.48;
  // lv 0 is a stem, 1 a barb off a stem, 2 a fine barb off a barb.
  function arm(x, y, a, L, w, lv, d, curl, forks) {
    var n = Math.max(2, Math.round(L / step)), tr = lv < 2 ? [[x, y, d]] : null, side = rnd() < 0.5 ? 1 : -1;
    var fork = lv === 0 && forks > 0 && rnd() < 0.55 ? Math.round(n * (0.25 + rnd() * 0.35)) : -1;
    for (var i = 1; i <= n; i++) {
      a += curl + (rnd() - 0.5) * (lv ? 0.1 : 0.06);
      var nx = x + Math.cos(a) * step, ny = y + Math.sin(a) * step;
      if ((nx - cx) * (nx - cx) + (ny - cx) * (ny - cx) > maxR * maxR) break;
      var u = i / n, rem = (n - i) * step; d += step;
      segs.push([x, y, nx, ny, d, Math.max(0.7, w * (1 - 0.55 * u)), lv]);
      x = nx; y = ny; if (tr) tr.push([x, y, d]);
      if (i === fork) arm(x, y, a + side * (0.4 + rnd() * 0.25), rem * (0.6 + rnd() * 0.3), w * 0.85, 0, d, -curl, forks - 1);
      if (lv === 0 && i % 2 === 0 && i > 1 && i < n - 1) {
        // Barbs on both sides at the angle ice branches at, shorter toward the tip, a few missing.
        var bl = Math.min(rem * 0.6, L * 0.2) * (0.55 + rnd() * 0.6), sp = 1.0 + (rnd() - 0.5) * 0.25;
        if (bl > step * 1.5) {
          if (rnd() < 0.85) arm(x, y, a + sp, bl, w * 0.6, 1, d, 0, 0);
          if (rnd() < 0.85) arm(x, y, a - sp, bl * (0.8 + rnd() * 0.3), w * 0.6, 1, d, 0, 0);
        }
      } else if (lv === 1 && i % 2 === 0 && i < n - 1 && rnd() < (hd ? 0.55 : 0.3)) {
        var bl2 = rem * (0.4 + rnd() * 0.4);
        if (bl2 > step) arm(x, y, a + side * (1.0 + (rnd() - 0.5) * 0.3), bl2, w * 0.65, 2, d, 0, 0);
        side = -side;
      }
    }
    if (tr && tr.length > 5) tracks.push(tr);
  }
  var NS = hd ? 11 : 10, k, a, rr;
  for (k = 0; k < NS; k++) {
    a = k / NS * Math.PI * 2 + (rnd() - 0.5) * 0.6; rr = r0 * (0.9 + rnd() * 0.25);
    arm(cx + Math.cos(a) * rr, cx + Math.sin(a) * rr, a + (rnd() - 0.5) * 0.5, (maxR - rr) * (0.35 + rnd() * 0.65), 1.65, 0, 0, (rnd() - 0.5) * 0.03, 1);
  }
  // A crust of short needles round the base, where the cold is deepest.
  var NC = hd ? 26 : 14;
  for (k = 0; k < NC; k++) {
    a = rnd() * Math.PI * 2; rr = r0 * (0.95 + rnd() * 0.4);
    arm(cx + Math.cos(a) * rr, cx + Math.sin(a) * rr, a + (rnd() - 0.5) * 1.2, maxR * (0.05 + rnd() * 0.1), 1.2, 1, rnd() * maxR * 0.1, 0, 0);
  }
  var dMax = 0; segs.forEach(function (sg) { dMax = Math.max(dMax, sg[4]); });
  return { segs: segs, tracks: tracks, dMax: dMax };
}
// The ferns painted as data, not colour: red is coverage (a soft halo, then the crystal itself), and
// blue and green are the time each bit freezes and how fine it is, both multiplied by coverage so the
// mipmaps still average to the right time. Grouped into batches so it paints in a few hundred strokes,
// spread over the first frames.
function frostFernBatches(fern) {
  var LV = 96, by = {}, out = [];
  var passes = [{ R: 26, wk: 5, w0: 2, lvMax: 0 }, { R: 72, wk: 2.6, w0: 1.2, lvMax: 3 }, { R: 255, wk: 1, w0: 0, lvMax: 3 }];
  passes.forEach(function (ps, pi) {
    fern.segs.forEach(function (sg) {
      if (sg[6] > ps.lvMax) return;
      var b = Math.min(LV - 1, Math.floor(sg[4] / fern.dMax * LV)), wq = sg[5] > 1.55 ? 2 : sg[5] > 1.0 ? 1 : 0, key = pi + '|' + sg[6] + '|' + wq + '|' + b;
      var bt = by[key];
      if (!bt) {
        var bn = (b + 0.5) / LV, ln = sg[6] / 3;
        bt = by[key] = { pass: pi, b: b, style: 'rgb(' + ps.R + ',' + Math.round(ps.R * ln) + ',' + Math.round(ps.R * bn) + ')', w: [0.8, 1.3, 1.9][wq] * ps.wk + ps.w0, s: [] };
        out.push(bt);
      }
      bt.s.push(sg);
    });
  });
  out.sort(function (p, q) { return p.pass - q.pass || p.b - q.b; });
  return out;
}
function frostPaintBatch(g, bt, wScale) {
  g.strokeStyle = bt.style; g.lineWidth = bt.w * wScale; g.beginPath();
  for (var i = 0; i < bt.s.length; i++) { var sg = bt.s[i]; g.moveTo(sg[0], sg[1]); g.lineTo(sg[2], sg[3]); }
  g.stroke();
}

// Real-looking ice: physically lit and clear-coated, slightly blue, brighter at grazing angles, milky
// in places, with flat fracture sheets and trapped bubbles inside and frost where it meets the page.
// shared holds the crack uniforms that the ring and the block use together, so one crack runs
// through both. Built the same way every time so all of them share one compiled shader.
function frostIceMat(st, hd, shared, o) {
  var T = st.T, uni = { uRise: { value: 100 }, uFront: { value: 0 }, uFade: { value: 1 }, uGlowK: { value: 1 }, uBaseK: { value: o.baseK }, uTopY: { value: o.topY || 100 },
    uRim: { value: new T.Color(FROST_GLACIER) }, uDeep: { value: new T.Color(FROST_DEEP) }, uIce: { value: new T.Color(FROST_ICE) } };
  Object.keys(shared).forEach(function (k) { uni[k] = shared[k]; });
  var mat = new T.MeshPhysicalMaterial({ color: o.color, metalness: 0, roughness: 0.05, clearcoat: 1, clearcoatRoughness: 0.03, ior: 1.31, specularIntensity: 1,
    envMapIntensity: 1.6, transparent: true, opacity: o.opacity, side: T.DoubleSide, flatShading: true });
  mat.defines = { FROST_HD: hd ? 1 : 0 };
  mat.onBeforeCompile = function (sh) {
    Object.assign(sh.uniforms, uni);
    sh.vertexShader = sh.vertexShader.replace('#include <common>', '#include <common>\nvarying vec3 vFrL; varying vec3 vFrW; varying float vFrS;')
      .replace('#include <begin_vertex>', ['#include <begin_vertex>',
        'vFrL = position; vec4 frw = vec4(transformed, 1.0); vFrS = 0.37;',
        '#ifdef USE_INSTANCING', 'frw = instanceMatrix * frw; vFrS = fract(sin(dot(instanceMatrix[3].xyz, vec3(12.99, 78.23, 37.71))) * 43758.55);', '#endif',
        'vFrW = (modelMatrix * frw).xyz;'].join('\n'));
    sh.fragmentShader = sh.fragmentShader.replace('#include <common>', ['#include <common>',
        'uniform float uTime, uCrack, uCrackK, uRise, uFront, uFade, uGlowK, uBaseK, uTopY; uniform vec3 uImp, uRim, uDeep, uIce;',
        'varying vec3 vFrL; varying vec3 vFrW; varying float vFrS;', FROST_GLSL].join('\n'))
      // The block forms from the page up: anything above the freezing line is not ice yet.
      .replace('#include <clipping_planes_fragment>', '#include <clipping_planes_fragment>\nif (vFrW.y > uRise) discard;')
      .replace('#include <color_fragment>', ['#include <color_fragment>',
        'float frCloud = frF3(vFrL * 2.6 + vFrS * 11.0);',
        'float frBase = smoothstep(0.32, 0.0, vFrW.y) * uBaseK;',
        'float frSheet = 0.0;',
        '#if FROST_HD',
        'for (int k = 0; k < 3; k++) { float fk = float(k) * 1.7 + vFrS * 7.0;',
        ' vec3 nk = normalize(vec3(frH(vec2(fk, 1.3)) - 0.5, frH(vec2(fk, 2.7)) - 0.5, frH(vec2(fk, 4.1)) - 0.5));',
        ' float dk = dot(vFrL, nk) - (frH(vec2(fk, 5.9)) - 0.5) * 0.6;',
        ' frSheet = max(frSheet, smoothstep(0.03, 0.0, abs(dk)) * smoothstep(0.38, 0.62, frN3(vFrL * 3.0 + fk))); }',
        '#endif',
        // Bubbles: some cells hold one, a dark disc with a bright rim.
        'vec3 frBc = vFrL * 7.0 + vFrS * 3.0; vec3 frBi = floor(frBc); vec3 frBf = fract(frBc); float frBh = frH3(frBi);',
        'vec3 frBo = vec3(frH3(frBi + 3.1), frH3(frBi + 7.7), frH3(frBi + 11.3)) * 0.5 + 0.25;',
        'float frBd = length(frBf - frBo), frBr = 0.07 + 0.1 * fract(frBh * 13.0);',
        'float frBub = step(0.68, frBh) * smoothstep(frBr, frBr * 0.7, frBd);',
        'float frBubRim = frBub * smoothstep(frBr * 0.4, frBr * 0.9, frBd);',
        'diffuseColor.rgb = mix(diffuseColor.rgb, uDeep, 0.32 * (1.0 - frCloud));',
        'diffuseColor.rgb = mix(diffuseColor.rgb, uIce, clamp(frCloud * 0.3 + frBase * 0.7 + frSheet * 0.6, 0.0, 1.0));',
        'diffuseColor.rgb = mix(diffuseColor.rgb, uDeep * 0.6, frBub * (1.0 - frBubRim) * 0.6);',
        'diffuseColor.a = clamp(diffuseColor.a * (0.55 + frCloud * 0.9) + frBase * 0.45 + frSheet * 0.35 + frBub * 0.3, 0.0, 1.0);',
        // The cracks race out from the impact: cell walls plus a few rays, dark on the outside (so they
        // show on a white page) with a hot core added below as light.
        'float frRd = length(vFrW - uImp);',
        'float frClear = smoothstep(uTopY - 1.0, uTopY, vFrW.y);',
        'float frReach = smoothstep(uCrack, uCrack - 0.3, frRd) * step(0.001, uCrack) * (1.0 - 0.7 * frClear);',
        'float frCell = frCellEdge(vFrW.xz * 1.4 + vFrW.y * 0.7);',
        'float frAng = atan(vFrW.z - uImp.z, vFrW.x - uImp.x);',
        'float frRad = abs(fract(frAng * 1.7507 + frN(vec2(frRd * 1.7, vFrS * 9.0)) * 0.5) - 0.5);',
        'float frThin = max(smoothstep(0.03, 0.0, frCell), smoothstep(0.016, 0.0, frRad)) * frReach;',
        'float frWide = max(smoothstep(0.065, 0.0, frCell), smoothstep(0.04, 0.0, frRad)) * frReach;',
        // Clearer toward the top of the block, so the die shows through from above.
        'diffuseColor.a *= 1.0 - 0.6 * frClear;',
        'diffuseColor.rgb = mix(diffuseColor.rgb, uDeep * 0.2, frWide * 0.9);',
        'diffuseColor.a = max(diffuseColor.a, frWide * 0.85);',
        // The freezing line as the block forms: a band of fresh white rime just below it.
        'float frFront = smoothstep(0.22, 0.0, uRise - vFrW.y) * min(1.0, uFront);',
        'diffuseColor.rgb = mix(diffuseColor.rgb, uIce, frFront * 0.8);',
        'diffuseColor.a = max(diffuseColor.a, frFront * 0.8) * uFade;'].join('\n'))
      .replace('#include <roughnessmap_fragment>', '#include <roughnessmap_fragment>\nroughnessFactor = clamp(roughnessFactor + frBase * 0.55 + frWide * 0.25 + frCloud * 0.08, 0.0, 1.0);')
      .replace('#include <emissivemap_fragment>', ['#include <emissivemap_fragment>',
        'float frFr = pow(1.0 - abs(dot(normal, normalize(vViewPosition))), 3.0);',
        'totalEmissiveRadiance += uRim * (frFr * 0.55 + frSheet * 0.3 + frBubRim * 0.45) * uGlowK * uFade;',
        'totalEmissiveRadiance += uIce * frThin * uCrackK * uFade;',
        'totalEmissiveRadiance += uIce * frFront * uFront;'].join('\n'));
  };
  mat.userData.uni = uni;
  return mat;
}

// Rime on the die: a shell just outside its faces that frosts from the rims inward, thickest on the
// edges, kept thin over the middle of the face with the number so the 1 stays readable, and thawing
// back toward the rims at the end.
function frostShellMat(st) {
  var T = st.T, uni = { uCover: { value: 0 }, uThaw: { value: 0 }, uTime: { value: 0 }, uAK: { value: st.cine ? 0.6 : 1 }, uIce: { value: new T.Color(FROST_ICE) }, uRim: { value: new T.Color(FROST_GLACIER) } };
  var mat = new T.MeshPhysicalMaterial({ color: '#e4f4ff', metalness: 0, roughness: 0.62, transparent: true, depthWrite: false, clearcoat: 0.25, clearcoatRoughness: 0.4,
    sheen: 1, sheenColor: new T.Color(FROST_GLACIER), sheenRoughness: 0.4, envMapIntensity: 1.2 });
  mat.onBeforeCompile = function (sh) {
    Object.assign(sh.uniforms, uni);
    sh.vertexShader = sh.vertexShader.replace('#include <common>', '#include <common>\nattribute float aEdge; varying float vFsE; varying vec3 vFsL; varying float vFsUp;')
      .replace('#include <begin_vertex>', '#include <begin_vertex>\nvFsE = aEdge; vFsL = position; vFsUp = normalize(mat3(modelMatrix) * normal).y;');
    sh.fragmentShader = sh.fragmentShader.replace('#include <common>', ['#include <common>',
        'uniform float uCover, uThaw, uTime, uAK; uniform vec3 uIce, uRim; varying float vFsE; varying vec3 vFsL; varying float vFsUp;', FROST_GLSL].join('\n'))
      .replace('#include <color_fragment>', ['#include <color_fragment>',
        'float fsG = frF(vFsL.xy * 6.0 + vFsL.z * 4.0);',
        'float fsFine = frN(vec2(vFsL.x * 37.0 + vFsL.y * 23.0, vFsL.z * 37.0 - vFsL.y * 11.0));',
        'float fsEdge = 1.0 - vFsE;',
        'float fsRime = smoothstep(0.0, 0.16, uCover * 1.35 - vFsE - (fsG - 0.5) * 0.7);',
        'float fsTop = smoothstep(0.93, 0.99, vFsUp);',
        'float fsThick = mix(0.1, 0.85, pow(fsEdge, 1.8)) * (0.7 + 0.6 * fsFine);',
        'fsThick *= 1.0 - fsTop * smoothstep(0.2, 0.6, vFsE) * 0.72;',
        'float fsKeep = smoothstep(uThaw * 1.2 - 0.2, uThaw * 1.2, fsEdge * 0.75 + fsG * 0.25);',
        'float fsA = clamp(fsRime * fsThick * fsKeep * uAK, 0.0, 1.0);',
        'diffuseColor.rgb = mix(uRim, uIce, 0.55 + 0.45 * fsFine);',
        'diffuseColor.a = fsA;'].join('\n'))
      .replace('#include <emissivemap_fragment>', ['#include <emissivemap_fragment>',
        // Glints: a few facets of the rime catch the light and twinkle.
        'float fsH = fract(sin(dot(floor(vFsL * 26.0), vec3(12.99, 78.23, 37.71))) * 43758.55);',
        'float fsGl = step(0.965, fsH) * pow(0.5 + 0.5 * sin(uTime * 7.0 + fsH * 80.0), 6.0);',
        'totalEmissiveRadiance += (uRim * 0.18 + uIce * fsGl * 2.2) * fsA;'].join('\n'));
  };
  mat.userData.uni = uni;
  return mat;
}

// The frost ferns on the page. The texture holds when each bit freezes, so growing, sparkling at the
// growing tips and thawing are all uniforms: nothing is redrawn while it plays. White crystal with a
// deep blue halo, so it reads on a white page as well as a dark one.
function frostFernMat(st, tex, at, FW, clr) {
  var T = st.T, uni = { uMap: { value: tex }, uGrow: { value: 0 }, uThaw: { value: 0 }, uFlash: { value: 0 }, uTime: { value: 0 }, uOn: { value: 1 }, uBoost: { value: st.cine ? 1 : 1.7 },
    uBounds: { value: new T.Vector4(at.x, at.z, st.hw, st.hh) }, uFW: { value: FW }, uClr: { value: clr || frostSpots(st, null, at) },
    uIce: { value: new T.Color(FROST_ICE) }, uGlacier: { value: new T.Color(FROST_GLACIER) }, uDeep: { value: new T.Color(FROST_DEEP) } };
  return new T.ShaderMaterial({ uniforms: uni, transparent: true, depthWrite: false,
    vertexShader: 'varying vec2 vUv; void main(){ vUv = uv; gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0); }',
    fragmentShader: [
      'uniform sampler2D uMap; uniform float uGrow, uThaw, uFlash, uTime, uOn, uFW, uBoost; uniform vec4 uBounds; uniform vec3 uIce, uGlacier, uDeep; varying vec2 vUv;', FROST_GLSL, FROST_CLEAR,
      'void main(){',
      ' vec4 tx = texture2D(uMap, vUv); float cov = tx.r; if (cov < 0.02) discard;',
      ' float b = tx.b / cov, lv = tx.g / cov, ahead = uGrow - b;',
      ' float on = smoothstep(0.0, 0.01, ahead); if (on <= 0.0) discard;',
      ' float core = smoothstep(0.3, 0.85, cov);',
      // The growing front burns white for a moment.
      ' float tip = exp(-ahead * 70.0) * (1.0 - step(0.999, uGrow));',
      ' float n = frN(vUv * 140.0);',
      // Thaw from the tips back toward the die, the finest needles first.
      ' float keep = smoothstep(uThaw * 1.3 - 0.3, uThaw * 1.3, (1.0 - b) * 0.55 + (1.0 - lv) * 0.25 + n * 0.2);',
      ' float glint = step(0.995, frH(floor(vUv * 300.0) + floor(uTime * 9.0) * 0.37)) * core;',
      ' vec3 col = mix(uDeep, uGlacier, smoothstep(0.1, 0.3, cov));',
      ' col = mix(col, mix(uGlacier, uIce, 0.6), core);',
      ' col = mix(col, uGlacier * 0.75, uThaw * 0.5);',
      ' col += uIce * (tip * 2.6 + glint * 1.6 + uFlash * core * 1.4);',
      ' float a = mix(min(1.0, cov / 0.28) * 0.34, 0.86, core) * on * keep * uOn;',
      // Fade out before the edge of the canvas, so frost never stops at a hard line.
      ' vec2 w = uBounds.xy + (vUv - 0.5) * vec2(uFW, -uFW);',
      ' a *= smoothstep(0.0, 0.9, uBounds.z - abs(w.x)) * smoothstep(0.0, 0.9, uBounds.w - abs(w.y));',
      // Frost lies on the page under the dice, so it only gives way round a die's own footprint.
      ' a *= frClear(w, 0.6);',
      ' gl_FragColor = vec4(col, 1.0 - pow(1.0 - clamp(a, 0.0, 1.0), uBoost));',
      ' #include <tonemapping_fragment>',
      ' #include <colorspace_fragment>',
      '}'].join('\n') });
}

// Freezing fog: flat puffs lying on the page (the camera looks straight down, so a flat quad faces it).
// Two tones together, pale blue-white and blue-grey, so the mist shows on a dark page and a light one.
function frostFogMat(st, clr) {
  var T = st.T;
  return new T.ShaderMaterial({ transparent: true, depthWrite: false,
    uniforms: { uMap: { value: frostFogTex(T) }, uPale: { value: new T.Color('#e4f2fb') }, uGrey: { value: new T.Color('#7890a6') }, uBoost: { value: st.cine ? 1 : 1.8 }, uClr: { value: clr || frostSpots(st, null, new T.Vector3()) } },
    vertexShader: 'attribute float aAlpha; attribute float aTone; varying vec2 vUv; varying float vA; varying float vT; varying vec3 vW; void main(){ vUv = uv; vA = aAlpha; vT = aTone; vW = (instanceMatrix * vec4(position, 1.0)).xyz; gl_Position = projectionMatrix * modelViewMatrix * instanceMatrix * vec4(position, 1.0); }',
    fragmentShader: ['uniform sampler2D uMap; uniform vec3 uPale; uniform vec3 uGrey; uniform float uBoost; varying vec2 vUv; varying float vA; varying float vT; varying vec3 vW;', FROST_CLEAR,
      'void main(){ float d = texture2D(uMap, vUv).r * vA * frClear(vW.xz, 1.0); if (d < 0.004) discard; d = 1.0 - pow(1.0 - d, mix(uBoost, 1.0, vT)); gl_FragColor = vec4(mix(uPale, uGrey, vT), d);',
      ' #include <tonemapping_fragment>',
      ' #include <colorspace_fragment>',
      '}'].join('\n') });
}

// A pool of points that bursts are fired into, so many small bursts cost one draw call.
function frostPool(st, n, o) {
  var T = st.T, pos = new Float32Array(n * 3), col = new Float32Array(n * 4), vel = new Float32Array(n * 3), age = new Float32Array(n), life = new Float32Array(n), ph = new Float32Array(n), cur = 0, rnd = Math.random;
  var cs = o.colors.map(function (h) { return new T.Color(h).multiplyScalar(o.boost || 1); });
  var geo = new T.BufferGeometry(); geo.setAttribute('position', new T.BufferAttribute(pos, 3)); geo.setAttribute('color', new T.BufferAttribute(col, 4));
  var pts = new T.Points(geo, new T.PointsMaterial({ size: st.ptSize(o.size), map: o.tex, transparent: true, depthWrite: false, vertexColors: true, sizeAttenuation: true,
    blending: o.add ? T.AdditiveBlending : T.NormalBlending, toneMapped: !o.add }));
  pts.frustumCulled = false; pts.renderOrder = o.order; st.scene.add(pts);
  return { obj: pts,
    emit: function (k, fn) {
      for (var j = 0; j < k; j++) {
        var i = cur, sp = fn(j, rnd), c = cs[j % cs.length]; cur = (cur + 1) % n;
        pos[i * 3] = sp.p[0]; pos[i * 3 + 1] = sp.p[1]; pos[i * 3 + 2] = sp.p[2]; vel[i * 3] = sp.v[0]; vel[i * 3 + 1] = sp.v[1]; vel[i * 3 + 2] = sp.v[2];
        age[i] = -(sp.delay || 0); life[i] = sp.life; ph[i] = rnd() * 6.283;
        col[i * 4] = c.r; col[i * 4 + 1] = c.g; col[i * 4 + 2] = c.b; col[i * 4 + 3] = 0;
      }
    },
    update: function (dt, fade) {
      var g = o.g || 0, dd = Math.max(0, 1 - (o.drag || 0) * dt);
      for (var i = 0; i < n; i++) {
        if (life[i] <= 0) continue;
        age[i] += dt;
        if (age[i] < 0) { col[i * 4 + 3] = 0; continue; }
        var k = age[i] / life[i], j = i * 3;
        if (k >= 1) { col[i * 4 + 3] = 0; life[i] = 0; continue; }
        vel[j + 1] -= g * dt; vel[j] *= dd; vel[j + 1] *= dd; vel[j + 2] *= dd;
        pos[j] += vel[j] * dt; pos[j + 1] += vel[j + 1] * dt; pos[j + 2] += vel[j + 2] * dt;
        if (pos[j + 1] < 0.03) { pos[j + 1] = 0.03; vel[j + 1] *= -0.3; vel[j] *= 0.7; vel[j + 2] *= 0.7; }
        var a = o.alpha * fade * smooth(0, 0.12, k) * (1 - smooth(0.55, 1, k));
        // Glitter twinkles: each grain turns to catch the light now and then.
        if (o.twinkle) a *= 0.15 + 0.85 * Math.pow(0.5 + 0.5 * Math.sin(age[i] * o.twinkle + ph[i]), 3);
        col[i * 4 + 3] = a;
      }
      geo.attributes.position.needsUpdate = true; geo.attributes.color.needsUpdate = true;
    } };
}

function fxFrost(st, die, fi, th, o) {
  var T = st.T, g = die.group, s = die.size, hd = st.q.detail === 'hd', t = 0, rnd = mulberry(4021), DUR = 4.9, i;
  // With the glow pass on, layers blend in linear light, where a faint pale layer looks much stronger
  // than when drawn straight to the page; alphas are set for each.
  var lin = !!st.cine;
  var p0 = g.position.clone(), q0 = g.quaternion.clone(), y0 = p0.y, at = new T.Vector3(p0.x, 0, p0.z), topY = y0 * 2;
  var T_GROW = 0.5, T_GROW1 = 2.05, T_ERUPT = 1.42, T_BLOCK = 1.8, T_BLOCK1 = 2.42, T_CRACK = 2.86, T_SHAT = 3.3;
  var m4 = new T.Matrix4(), v3 = new T.Vector3(), dq = new T.Quaternion(), qy = new T.Quaternion(), Y = new T.Vector3(0, 1, 0), ZERO = new T.Matrix4().makeScale(0, 0, 0);
  // Everything is built and in the scene from the first frame, hidden by scale or alpha, so every
  // shader compiles now and not in the middle of the eruption or the shatter.
  var clear = o && o.clear || [], clr = clear.length ? frostSpots(st, clear, at) : null;
  var dim = frostDimmer(st, at, s * 8.5, clr);

  // The page frost: painted as data over the first frames, then revealed by uniforms.
  var C = hd ? 1024 : 512, FW = s * 12.5, fcv = canvas(C), fg = fcv.getContext('2d');
  fg.fillStyle = '#000'; fg.fillRect(0, 0, C, C); fg.lineCap = 'round';
  var fern = frostFernSegs(C, rnd, hd, s * 0.82 / FW * C), batches = frostFernBatches(fern), painted = 0, perFrame = Math.ceil(batches.length / 20), wScale = hd ? 1 : 0.72;
  var fernTex = new T.CanvasTexture(fcv); fernTex.anisotropy = 4;
  var fernMat = frostFernMat(st, fernTex, at, FW, clr), fu = fernMat.uniforms;
  var fernMesh = new T.Mesh(new T.PlaneGeometry(FW, FW), fernMat);
  fernMesh.rotation.x = -Math.PI / 2; fernMesh.position.set(at.x, 0.008, at.z); fernMesh.renderOrder = 1; st.scene.add(fernMesh);
  // A sparkle riding the tip of each of the longest arms while it grows.
  var tracks = fern.tracks.slice().sort(function (p, q) { return q.length - p.length; }).slice(0, hd ? 44 : 24), NT = tracks.length;
  var tipIdx = new Int32Array(NT), tpPos = new Float32Array(NT * 3), tpCol = new Float32Array(NT * 4), tpGeo = new T.BufferGeometry(), ice = new T.Color(FROST_ICE).multiplyScalar(1.5);
  for (i = 0; i < NT; i++) { tpCol[i * 4] = ice.r; tpCol[i * 4 + 1] = ice.g; tpCol[i * 4 + 2] = ice.b; }
  tpGeo.setAttribute('position', new T.BufferAttribute(tpPos, 3)); tpGeo.setAttribute('color', new T.BufferAttribute(tpCol, 4));
  var tips = new T.Points(tpGeo, new T.PointsMaterial({ size: st.ptSize(s * 0.55), map: starTex(T), transparent: true, depthWrite: false, vertexColors: true, blending: T.AdditiveBlending, toneMapped: false }));
  tips.frustumCulled = false; tips.renderOrder = 6; st.scene.add(tips);

  // Freezing fog: most of it exhaled at once from under the die, the rest seeping out after. Each puff
  // is drawn out along the way it flows, so it streams across the page instead of sitting in balls.
  var NF = hd ? 56 : 28, fogGeo = new T.PlaneGeometry(1, 1), fA = new Float32Array(NF), fT = new Float32Array(NF), fogP = [];
  fogGeo.rotateX(-Math.PI / 2);
  fogGeo.setAttribute('aAlpha', new T.InstancedBufferAttribute(fA, 1)); fogGeo.setAttribute('aTone', new T.InstancedBufferAttribute(fT, 1));
  var fog = new T.InstancedMesh(fogGeo, frostFogMat(st, clr), NF); fog.frustumCulled = false; fog.renderOrder = 4; st.scene.add(fog);
  for (i = 0; i < NF; i++) {
    var a = rnd() * Math.PI * 2, early = i < NF * 0.55, r0 = s * (0.6 + rnd() * 0.5), sp = s * (0.9 + rnd() * 1.3) * (early ? 1.3 : 0.8), tone = rnd() < 0.45 ? 1 : 0;
    fT[i] = tone; fog.setMatrixAt(i, ZERO);
    fogP.push({ x: at.x + Math.cos(a) * r0, y: 0.12 + rnd() * 0.3, z: at.z + Math.sin(a) * r0, vx: Math.cos(a) * sp, vz: Math.sin(a) * sp, rot: -a + (rnd() - 0.5) * 0.5, rv: (rnd() - 0.5) * 0.3, st: 1.5 + rnd() * 0.6,
      age: -(early ? 0.05 + rnd() * 0.5 : 0.6 + rnd() * 1.2), life: 2.0 + rnd() * 1.3, peak: tone ? 0.16 + rnd() * 0.08 : 0.11 + rnd() * 0.06, s0: s * (0.9 + rnd() * 0.5), s1: s * (2.6 + rnd() * 1.6) });
  }

  // Rime on the die itself.
  var shellMat = frostShellMat(st), su = shellMat.userData.uni, shell = new T.Mesh(frostHullGeo(T, die, 1.03), shellMat);
  shell.renderOrder = 2; g.add(shell);

  // The ice: a jagged ring of crystals leaning outward, and the block that closes over the die. One
  // set of crack uniforms runs through both.
  var crackU = { uCrack: { value: 0 }, uCrackK: { value: 0 }, uImp: { value: new T.Vector3(at.x, topY + s * 0.5, at.z) }, uTime: { value: 0 } };
  var ringMat = frostIceMat(st, hd, crackU, { color: '#7fc2ea', opacity: lin ? 0.5 : 0.6, baseK: 1 }), ru = ringMat.userData.uni, crystals = [], NCL = hd ? 8 : 6;
  for (var c = 0; c < NCL; c++) {
    var ca = c / NCL * Math.PI * 2 + (rnd() - 0.5) * 0.5, cr = s * (1.4 + rnd() * 0.25), nk = hd ? 3 + Math.floor(rnd() * 2.2) : 2 + (rnd() < 0.5 ? 1 : 0);
    for (var k = 0; k < nk; k++) {
      var oa = ca + (rnd() - 0.5) * 0.45, orr = cr + (rnd() - 0.5) * s * 0.35, dirA = ca + (rnd() - 0.5) * 1.1, lean = 0.45 + rnd() * 0.5;
      var cq = new T.Quaternion().setFromAxisAngle(new T.Vector3(Math.sin(dirA), 0, -Math.cos(dirA)), lean).multiply(qy.setFromAxisAngle(Y, rnd() * 6.283));
      crystals.push({ p: new T.Vector3(at.x + Math.cos(oa) * orr, 0, at.z + Math.sin(oa) * orr), dirA: dirA, q: cq, fired: false,
        h: s * (k === 0 ? 2.5 + rnd() * 0.8 : 1.0 + rnd() * 1.3), w: s * (k === 0 ? 0.27 + rnd() * 0.07 : 0.14 + rnd() * 0.1), t0: T_ERUPT + c / NCL * 0.34 + k * 0.05 + rnd() * 0.04 });
    }
  }
  var ring = new T.InstancedMesh(frostPrismGeo(T), ringMat, crystals.length);
  ring.castShadow = true; ring.renderOrder = 3; ring.frustumCulled = false; st.scene.add(ring);
  for (i = 0; i < crystals.length; i++) ring.setMatrixAt(i, ZERO);
  var bRx = s * 1.4, bRy = s * 1.25, blockTop = y0 + bRy, blockMat = frostIceMat(st, hd, crackU, { color: '#9ad2ef', opacity: lin ? 0.36 : 0.44, baseK: 0.35, topY: blockTop }), bu = blockMat.userData.uni;
  var block = new T.Mesh(frostBlockGeo(T, rnd, bRx, bRy, y0), blockMat);
  block.position.copy(at); block.renderOrder = 3; bu.uRise.value = -1; st.scene.add(block);

  // The shards it bursts into.
  var NSH = hd ? 56 : 28, shardMat = frostIceMat(st, hd, { uCrack: { value: 0 }, uCrackK: { value: 0 }, uImp: { value: new T.Vector3() }, uTime: crackU.uTime }, { color: '#78bde6', opacity: lin ? 0.42 : 0.55, baseK: 0 });
  var shards = new T.InstancedMesh(frostShardGeo(T), shardMat, NSH), sh = [];
  shards.castShadow = true; shards.renderOrder = 3; shards.frustumCulled = false; st.scene.add(shards);
  for (i = 0; i < NSH; i++) { shards.setMatrixAt(i, ZERO); sh.push({ p: new T.Vector3(), v: new T.Vector3(), q: new T.Quaternion(), ax: new T.Vector3(), w: 0, sc: new T.Vector3(), tm: 0 }); }

  // Glitter of ice dust (light, for dark pages and the glow) and a blue-grey dust (for light pages).
  var glit = frostPool(st, hd ? 220 : 110, { size: s * 0.3, tex: starTex(T), add: true, colors: [FROST_ICE, FROST_GLACIER], boost: 1.3, drag: 1.6, g: s * 2.5, twinkle: 22, alpha: 1, order: 6 });
  var dust = frostPool(st, hd ? 80 : 40, { size: s * 0.2, tex: spriteTex(T), add: false, colors: ['#87a8c2', '#d4e9f6'], drag: 2.2, g: s * 1.5, alpha: 0.75, order: 5 });

  // The 1's cold light: a halo over it, a lens flare for the crack and the burst, and one point light
  // inside that lights the die and the ice from within.
  var halo = glow(st, spriteTex(T), FROST_SICK, s * 3.0), flare = glow(st, starTex(T), FROST_ICE, s * 9);
  halo.position.set(at.x, topY + 0.05, at.z); halo.renderOrder = 5; halo.material.depthTest = false;
  flare.position.set(at.x, blockTop, at.z); flare.renderOrder = 7; flare.material.depthTest = false;
  st.scene.add(halo, flare);
  var lt = fxLight(st, new T.Vector3(at.x, y0 + s * 0.7, at.z), FROST_SICK, DUR, function (tt) {
    var k = 10 * smooth(0, 0.6, tt) * (1 - 0.45 * smooth(T_BLOCK, T_BLOCK1, tt));
    if (tt >= T_CRACK) k += 30 * Math.exp(-(tt - T_CRACK) * 12) + (tt < T_SHAT ? 8 * Math.abs(Math.sin(tt * 37)) : 0);
    if (tt >= T_SHAT) k += 150 * Math.exp(-(tt - T_SHAT) * 5);
    return k * (1 - smooth(4.3, 4.85, tt));
  }, s * 12);
  st.fx.push(lt);

  var pale = frostLightStone(th), sickCol = pale ? mix(FROST_SICK, FROST_DEEP, 0.45) : FROST_SICK;
  var gf = glowFace(die, fi), numKey = '', cSick = new T.Color(FROST_SICK), cGlacier = new T.Color(FROST_GLACIER);
  // On a pale stone the number is drawn in solid colour, without the white-hot core, and the faces are
  // dimmed a little as the engine does under a burning number on a light stone: light can only add,
  // so on ivory the 1 has to read by its colour.
  function numeral(color, k) {
    if (color !== numKey) { numKey = color; frostNumeral(gf, pale, color); }
    gf.commit(k);
  }
  var shattered = false, cracked = false, finalCol = pale ? '#1f63d6' : mix(FROST_GLACIER, FROST_ICE, 0.25), entombCol = mix(FROST_SICK, FROST_DEEP, pale ? 0.7 : 0.35);

  function shatter() {
    shattered = true; block.visible = false; ring.visible = false;
    var nb = Math.round(NSH * 0.55), hx, hz;
    for (var i = 0; i < NSH; i++) {
      var S = sh[i];
      if (i < nb) {
        var a = rnd() * 6.283, el = (rnd() - 0.2) * 1.2;
        S.p.set(at.x + Math.cos(a) * Math.cos(el) * bRx * (0.6 + rnd() * 0.4), Math.max(0.15, y0 + Math.sin(el) * bRy * 0.8), at.z + Math.sin(a) * Math.cos(el) * bRx * (0.6 + rnd() * 0.4));
        hx = Math.cos(a); hz = Math.sin(a);
      } else {
        var cr = crystals[i % crystals.length];
        v3.set(0, cr.h * (0.15 + rnd() * 0.75), 0).applyQuaternion(cr.q);
        S.p.set(cr.p.x + v3.x, Math.max(0.12, v3.y), cr.p.z + v3.z); hx = Math.cos(cr.dirA); hz = Math.sin(cr.dirA);
      }
      // Out low across the page, a little scattered, tumbling.
      var spd = s * (3.2 + rnd() * 5.0) * (i < nb ? 1 : 0.8), ja = (rnd() - 0.5) * 0.5, cj = Math.cos(ja), sj = Math.sin(ja);
      S.v.set((hx * cj - hz * sj) * spd, s * (1.0 + rnd() * 2.8), (hx * sj + hz * cj) * spd);
      S.ax.set(rnd() - 0.5, rnd() - 0.5, rnd() - 0.5).normalize(); S.w = 8 + rnd() * 16; S.q.setFromAxisAngle(S.ax, rnd() * 6.283);
      var sz = s * (i < nb ? 0.16 + rnd() * 0.3 : 0.12 + rnd() * 0.22); S.sc.set(sz, sz * (0.6 + rnd() * 0.8), sz * (0.6 + rnd() * 0.6));
      S.tm = Math.min(DUR - 0.6, 3.85 + rnd() * 0.3 + sz / s * 0.4);
    }
    glit.emit(hd ? 90 : 45, function (j, r) { var a2 = r() * 6.283, sp2 = s * (1.5 + r() * 4.5), rr = bRx * 0.7 * r(); return { p: [at.x + Math.cos(a2) * rr, y0 + (r() - 0.3) * bRy, at.z + Math.sin(a2) * rr], v: [Math.cos(a2) * sp2, s * (1 + r() * 5), Math.sin(a2) * sp2], life: 0.9 + r() * 1.1, delay: r() * 0.06 }; });
    dust.emit(hd ? 56 : 28, function (j, r) { var a2 = r() * 6.283, sp2 = s * (1.5 + r() * 3.5), rr = bRx * 0.8 * r(); return { p: [at.x + Math.cos(a2) * rr, 0.2 + r() * y0, at.z + Math.sin(a2) * rr], v: [Math.cos(a2) * sp2, s * (0.5 + r() * 2), Math.sin(a2) * sp2], life: 0.8 + r() * 0.8 }; });
    // The blast pushes the fog out.
    fogP.forEach(function (f) { var dx = f.x - at.x, dz = f.z - at.z, d = Math.hypot(dx, dz) || 1, k = s * 3.2 / (1 + d / (s * 3)); f.vx += dx / d * k; f.vz += dz / d * k; });
    halo.material.color.set(FROST_GLACIER); lt.light.color.set(FROST_ICE);
  }

  return { update: function (dt) {
    t += dt;
    crackU.uTime.value = t; su.uTime.value = t; fu.uTime.value = t;
    dim.material.opacity = 0.55 * smooth(0, 0.8, t) * (1 - smooth(4.0, 4.85, t));

    // 1. The chill: the 1 turns sickly blue and guttering, then dims inside the ice, then burns cold.
    var nc, nk2;
    if (t < 0.4) { nc = mix(th.burn, sickCol, smooth(0, 0.4, t)); nk2 = 1.6 + 0.6 * smooth(0, 0.4, t); }
    else if (t < T_SHAT) {
      nc = t < T_BLOCK ? sickCol : entombCol; nk2 = (2.2 + 0.4 * Math.sin(t * 9) * Math.sin(t * 3.7 + 1)) * (1 - 0.3 * smooth(T_BLOCK, T_BLOCK1, t));
      if (t > T_CRACK) nk2 += 0.9 * smooth(T_CRACK, T_SHAT, t) * (0.6 + 0.4 * Math.sin(t * 50));
    } else { nc = finalCol; nk2 = 1.75 + 1.8 * Math.exp(-(t - T_SHAT) * 3.2); }
    numeral(nc, nk2);
    if (pale) shadeDie(die, 1 - 0.14 * smooth(0, 0.4, t) - 0.1 * env4(t, T_BLOCK, T_BLOCK1, T_SHAT, T_SHAT + 0.5));
    halo.material.opacity = (t < T_SHAT ? 0.3 * smooth(0, 0.5, t) * (1 - 0.7 * smooth(T_BLOCK, T_BLOCK1, t)) : 0.25 + 0.5 * Math.exp(-(t - T_SHAT) * 3)) * (1 - smooth(4.3, 4.85, t));
    if (!shattered) lt.light.color.copy(t < T_BLOCK ? cSick : cGlacier);
    tintDie(die, FROST_GLACIER, 0.35 * smooth(0.7, 1.8, t) * (1 - smooth(4.0, 4.8, t)));

    // Fog.
    var fogK = (1 - 0.35 * smooth(1.7, 2.3, t)) * (1 - smooth(4.2, 4.85, t));
    for (var i = 0; i < NF; i++) {
      var f = fogP[i]; f.age += dt;
      if (f.age < 0 || f.age >= f.life) { fog.setMatrixAt(i, ZERO); fA[i] = 0; continue; }
      var k = f.age / f.life, dd = Math.max(0, 1 - 1.1 * dt), dx = f.x - at.x, dz = f.z - at.z;
      f.vx = (f.vx - dz * 0.25 * dt) * dd; f.vz = (f.vz + dx * 0.25 * dt) * dd;
      f.x += f.vx * dt; f.z += f.vz * dt; f.rot += f.rv * dt;
      var sz = f.s0 + (f.s1 - f.s0) * out3(Math.min(1, k * 1.3));
      m4.makeRotationY(f.rot); m4.scale(v3.set(sz * f.st, 1, sz)); m4.setPosition(f.x, f.y, f.z); fog.setMatrixAt(i, m4);
      fA[i] = f.peak * smooth(0, 0.2, k) * (1 - smooth(0.5, 1, k)) * fogK;
    }
    fog.instanceMatrix.needsUpdate = true; fogGeo.attributes.aAlpha.needsUpdate = true;

    // 2. Frost spreads: the ferns paint over the first frames, then grow; the die rimes over.
    if (painted < batches.length) {
      for (var b = 0; b < perFrame && painted < batches.length; b++) frostPaintBatch(fg, batches[painted++], wScale);
      if (painted >= batches.length) fernTex.needsUpdate = true;
    }
    var gk = clamp01((t - T_GROW) / (T_GROW1 - T_GROW));
    fu.uGrow.value = painted >= batches.length ? 1 - Math.pow(1 - gk, 1.7) : 0;
    fu.uThaw.value = smooth(3.55, 4.8, t); fu.uOn.value = 1 - smooth(4.72, 4.88, t);
    fu.uFlash.value = t > T_SHAT ? Math.exp(-(t - T_SHAT) * 6) : 0;
    var gd = fu.uGrow.value * fern.dMax, growing = fu.uGrow.value > 0 && fu.uGrow.value < 0.999;
    for (i = 0; i < NT; i++) {
      var tr = tracks[i], ix = tipIdx[i];
      while (ix < tr.length - 1 && tr[ix + 1][2] <= gd) ix++;
      tipIdx[i] = ix;
      var pt = tr[ix], live = growing && gd >= tr[0][2] && ix < tr.length - 1;
      tpPos[i * 3] = at.x + (pt[0] / C - 0.5) * FW; tpPos[i * 3 + 1] = 0.06; tpPos[i * 3 + 2] = at.z + (pt[1] / C - 0.5) * FW;
      tpCol[i * 4 + 3] = live ? 0.55 + 0.45 * Math.sin(t * 31 + i * 2.3) : 0;
    }
    tpGeo.attributes.position.needsUpdate = true; tpGeo.attributes.color.needsUpdate = true;
    su.uCover.value = smooth(0.7, 1.95, t); su.uThaw.value = smooth(3.9, 4.8, t);

    // 3. Entombed: the crystals erupt round the ring, then the block freezes up from the page.
    if (!shattered) {
      var shake = t > T_CRACK ? s * 0.014 * smooth(T_CRACK, T_SHAT, t) : 0;
      for (i = 0; i < crystals.length; i++) {
        var cr = crystals[i], e = (t - cr.t0) / 0.2;
        if (e <= 0) { ring.setMatrixAt(i, ZERO); continue; }
        var wk = out3(e * 1.4);
        v3.set(cr.w * wk, cr.h * frostBackOut(e), cr.w * wk);
        m4.compose(cr.p, cr.q, v3); ring.setMatrixAt(i, m4);
        if (!cr.fired) {
          cr.fired = true;
          var cx = cr.p.x, cz = cr.p.z, ux = Math.cos(cr.dirA), uz = Math.sin(cr.dirA);
          dust.emit(hd ? 4 : 2, function (j, r) { return { p: [cx, 0.1, cz], v: [(ux + (r() - 0.5)) * s * 1.6, s * (0.5 + r()), (uz + (r() - 0.5)) * s * 1.6], life: 0.5 + r() * 0.5 }; });
          glit.emit(hd ? 5 : 3, function (j, r) { return { p: [cx, 0.15, cz], v: [(ux + (r() - 0.5)) * s * 2, s * (1 + r() * 2.5), (uz + (r() - 0.5)) * s * 2], life: 0.5 + r() * 0.6 }; });
        }
      }
      ring.instanceMatrix.needsUpdate = true;
      ring.position.set((rnd() - 0.5) * shake, 0, (rnd() - 0.5) * shake);
      var bk = inout((t - T_BLOCK) / (T_BLOCK1 - T_BLOCK));
      bu.uRise.value = t < T_BLOCK ? -1 : 0.01 + bk * (blockTop + 0.08);
      bu.uFront.value = t > T_BLOCK ? (lin ? 1.6 : 1.2) * (1 - smooth(T_BLOCK1 - 0.12, T_BLOCK1 + 0.05, t)) : 0;
      var bs = 0.9 + 0.1 * bk; block.scale.set(bs, 1, bs);
      block.position.set(at.x + (rnd() - 0.5) * shake, 0, at.z + (rnd() - 0.5) * shake);
      g.position.set(p0.x + (rnd() - 0.5) * shake * 0.4, p0.y, p0.z + (rnd() - 0.5) * shake * 0.4);
      ru.uGlowK.value = bu.uGlowK.value = 1 + 0.25 * Math.sin(t * 5) + (t >= T_CRACK ? 2.5 * Math.exp(-(t - T_CRACK) * 16) : 0);
    }

    // 4. The shatter: cracks race out with a bright crack, a beat, then the ice bursts.
    if (t >= T_CRACK && !shattered) {
      crackU.uCrack.value = 0.05 + out3((t - T_CRACK) / 0.2) * s * 5.5;
      crackU.uCrackK.value = (lin ? 1.4 : 1.6) * (1 + 0.6 * smooth(T_CRACK + 0.2, T_SHAT, t)) * (0.8 + 0.2 * Math.sin((t - T_CRACK) * 40)) + 2.5 * Math.exp(-(t - T_CRACK) * 14);
      if (!cracked) {
        cracked = true;
        glit.emit(hd ? 18 : 10, function (j, r) { var a2 = r() * 6.283, sp2 = s * (0.5 + r() * 1.5); return { p: [at.x, blockTop, at.z], v: [Math.cos(a2) * sp2, s * (1 + r() * 2), Math.sin(a2) * sp2], life: 0.4 + r() * 0.4 }; });
      }
    }
    var fl = 0;
    if (t >= T_CRACK) fl = 0.5 * Math.exp(-(t - T_CRACK) * 11);
    if (t >= T_SHAT) fl += 0.6 * Math.exp(-(t - T_SHAT) * 6);
    flare.material.opacity = Math.min(1, fl); flare.scale.setScalar(s * (t >= T_SHAT ? 9 : 6) * (0.8 + 0.4 * Math.min(1, fl)));
    flare.material.rotation = t * 0.3;
    if (!shattered && t >= T_SHAT) { shatter(); g.position.copy(p0); }

    if (shattered) {
      var G = s * 24;
      for (i = 0; i < NSH; i++) {
        var S = sh[i];
        S.v.y -= G * dt; S.p.addScaledVector(S.v, dt);
        var rest = S.sc.y * 0.3, mk = clamp01((t - S.tm) / 0.5);
        if (S.p.y < rest) {
          S.p.y = rest;
          if (S.v.y < 0) { S.v.y *= -0.3; if (S.v.y < s * 0.5) S.v.y = 0; S.v.x *= 0.72; S.v.z *= 0.72; S.w *= 0.6; }
        }
        // Sliding on the page: friction stops the skitter and the tumbling.
        if (S.p.y <= rest + 1e-4) { var fr = Math.max(0, 1 - 3.5 * dt); S.v.x *= fr; S.v.z *= fr; S.w *= Math.max(0, 1 - 5 * dt); }
        S.q.premultiply(dq.setFromAxisAngle(S.ax, S.w * dt));
        // Melting: the shard slumps and shrinks to nothing.
        if (mk >= 1) { shards.setMatrixAt(i, ZERO); continue; }
        var spread = (1 - 0.6 * mk) * (1 - in3(mk));
        v3.set(S.sc.x * spread, S.sc.y * Math.pow(1 - mk, 2), S.sc.z * spread);
        m4.compose(S.p, S.q, v3); shards.setMatrixAt(i, m4);
      }
      shards.instanceMatrix.needsUpdate = true;
    }
    var pf = 1 - smooth(4.45, 4.85, t); glit.update(dt, pf); dust.update(dt, pf);

    if (t >= DUR) {
      ring.dispose(); shards.dispose(); fog.dispose();
      drop(st, [dim, fernMesh, tips, fog, shell, ring, block, shards, halo, flare, glit.obj, dust.obj]);
      fernTex.dispose();
      g.position.copy(p0); g.quaternion.copy(q0);
      tintDie(die, th.burn, 0);
      numeral(finalCol, pale ? 2 : 1.6);
      if (pale) shadeDie(die, 0.86);
      return false;
    }
    return true;
  } };
}

// The echo: the same cold on another die. A cold shard skips across the page from the grand die (alone,
// it begins at the die), rime creeps over the die and a small ring of frost spreads round it, a skin of
// ice closes over the die and cracks, and it bursts into a few splinters that melt. No lights: the die's
// own tint and a halo carry it, so it costs no recompile when it starts.
function fxFrostEcho(st, die, fi, th, e) {
  var T = st.T, g = die.group, s = die.size, hd = st.q.detail === 'hd', t = 0, rnd = mulberry(7301 + Math.floor(Math.random() * 1e6)), END = 1.95, i;
  var lin = !!st.cine, Q = hd ? 1 : 0.5;
  var p0 = g.position.clone(), q0 = g.quaternion.clone(), y0 = p0.y, at = new T.Vector3(p0.x, 0, p0.z), topY = worldOf(die, die.faces[fi].c).y;
  var FROM = !!(e && e.from), T_HIT = 0.34, T_R0 = FROM ? 0.28 : 0.04, T_R1 = T_R0 + 0.8, T_CK = 1.02, T_BU = 1.2;
  var m4 = new T.Matrix4(), v3 = new T.Vector3(), dq = new T.Quaternion(), ZERO = new T.Matrix4().makeScale(0, 0, 0);

  // Where the cold comes in: on the side facing the grand die, or anywhere when alone.
  var S = new T.Vector3(), ang = rnd() * 6.283;
  if (FROM) { S.copy(worldOf(e.from, e.from.faces[e.fromFi].c)); ang = Math.atan2(S.z - at.z, S.x - at.x); }
  var ux = Math.cos(ang), uz = Math.sin(ang), IMP = new T.Vector3(at.x + ux * s * 0.8, y0 + s * 0.15, at.z + uz * s * 0.8);

  // A small ring of frost on the page round the die.
  var C = hd ? 512 : 384, FW = s * 4.7, fcv = canvas(C), fg = fcv.getContext('2d');
  fg.fillStyle = '#000'; fg.fillRect(0, 0, C, C); fg.lineCap = 'round';
  var fern = frostFernSegs(C, rnd, hd, s * 0.8 / FW * C), batches = frostFernBatches(fern), wScale = hd ? 1 : 0.72, painted = 0, perFrame = Math.ceil(batches.length / 3);
  var fernTex = new T.CanvasTexture(fcv); fernTex.anisotropy = 4;
  var fernMat = frostFernMat(st, fernTex, at, FW, null), fu = fernMat.uniforms;
  fu.uBoost.value = st.cine ? 1 : 1.4;
  var fernMesh = new T.Mesh(new T.PlaneGeometry(FW, FW), fernMat);
  fernMesh.rotation.x = -Math.PI / 2; fernMesh.position.set(at.x, 0.008, at.z); fernMesh.renderOrder = 60; fernMat.depthTest = false;

  // Rime on the die, then a skin of ice over it that cracks.
  var shellMat = frostShellMat(st), su = shellMat.userData.uni, shell = new T.Mesh(frostHullGeo(T, die, 1.03), shellMat);
  shell.renderOrder = 60; g.add(shell);
  var crackU = { uCrack: { value: 0 }, uCrackK: { value: 0 }, uImp: { value: IMP }, uTime: { value: 0 } };
  var skinMat = frostIceMat(st, hd, crackU, { color: '#9ad2ef', opacity: lin ? 0.34 : 0.42, baseK: 0, topY: topY + 0.02 }), ku = skinMat.userData.uni;
  ku.uFade.value = 0;
  var skin = new T.Mesh(frostHullGeo(T, die, 1.08), skinMat); skin.renderOrder = 61; g.add(skin);

  // The splinters: one that crosses the page, the rest from the burst.
  var NSH = (hd ? 14 : 8) + 1, shardMat = frostIceMat(st, hd, { uCrack: { value: 0 }, uCrackK: { value: 0 }, uImp: { value: new T.Vector3() }, uTime: crackU.uTime }, { color: '#78bde6', opacity: lin ? 0.42 : 0.55, baseK: 0 });
  var shards = new T.InstancedMesh(frostShardGeo(T), shardMat, NSH), sh = [];
  shards.renderOrder = 62; shards.frustumCulled = false; st.scene.add(shards);
  for (i = 0; i < NSH; i++) { shards.setMatrixAt(i, ZERO); sh.push({ p: new T.Vector3(), v: new T.Vector3(), q: new T.Quaternion(), ax: new T.Vector3(rnd() - 0.5, rnd() - 0.5, rnd() - 0.5).normalize(), w: 0, sc: new T.Vector3(), tm: 0 }); }

  var glit = frostPool(st, hd ? 110 : 56, { size: s * 0.26, tex: starTex(T), add: true, colors: [FROST_ICE, FROST_GLACIER], boost: 1.3, drag: 1.6, g: s * 2.5, twinkle: 22, alpha: 1, order: 66 });
  var dust = frostPool(st, hd ? 36 : 18, { size: s * 0.18, tex: spriteTex(T), add: false, colors: ['#87a8c2', '#d4e9f6'], drag: 2.2, g: s * 1.5, alpha: 0.7, order: 65 });
  var dim = dimmer(st, at, s * 3.2), halo = glow(st, spriteTex(T), FROST_SICK, s * 2.4), flare = glow(st, starTex(T), FROST_ICE, s * 4);
  halo.position.set(at.x, topY + 0.05, at.z); flare.position.set(IMP.x, IMP.y, IMP.z);
  halo.renderOrder = 64; flare.renderOrder = 67; halo.material.depthTest = false; flare.material.depthTest = false; dim.renderOrder = -2;
  glit.obj.material.depthTest = false; dust.obj.material.depthTest = false;
  st.scene.add(fernMesh, halo, flare);

  // The shard that crosses: it skips from the grand die and lands against this one.
  var trav = sh[0], tsz = s * 0.55, trest = tsz * 0.3, trail = 0;
  trav.sc.set(tsz, tsz * 0.8, tsz * 0.7);
  var pale = frostLightStone(th), sickCol = pale ? mix(FROST_SICK, FROST_DEEP, 0.45) : FROST_SICK, finalCol = pale ? '#1f63d6' : mix(FROST_GLACIER, FROST_ICE, 0.25);
  var entombCol = mix(FROST_SICK, FROST_DEEP, pale ? 0.7 : 0.35);
  var gf = glowFace(die, fi), numKey = '';
  function numeral(color, k) { if (color !== numKey) { numKey = color; frostNumeral(gf, pale, color); } gf.commit(k); }

  function hit() {
    glit.emit(Math.round(14 * Q), function (j, r) { var a2 = (r() - 0.5) * 2.4 + ang + Math.PI, sp2 = s * (1 + r() * 3); return { p: [IMP.x, IMP.y, IMP.z], v: [Math.cos(a2) * sp2, s * (1 + r() * 3), Math.sin(a2) * sp2], life: 0.5 + r() * 0.5 }; });
    dust.emit(Math.round(8 * Q), function (j, r) { var a2 = (r() - 0.5) * 2.4 + ang + Math.PI, sp2 = s * (0.8 + r() * 2); return { p: [IMP.x, 0.2, IMP.z], v: [Math.cos(a2) * sp2, s * (0.5 + r()), Math.sin(a2) * sp2], life: 0.5 + r() * 0.5 }; });
  }
  var hitDone = !FROM, burst = false;
  function shatter() {
    burst = true; skin.visible = false;
    for (var i2 = FROM ? 1 : 0, k = 0; i2 < NSH; i2++, k++) {
      var S2 = sh[i2], a = rnd() * 6.283, el = (rnd() - 0.1) * 1.1, rr = s * (0.9 + rnd() * 0.35);
      S2.p.set(at.x + Math.cos(a) * Math.cos(el) * rr, Math.max(0.15, y0 + Math.sin(el) * s * 0.9), at.z + Math.sin(a) * Math.cos(el) * rr);
      var spd = s * (2.4 + rnd() * 3.4), ja = rnd() * 0.5 - 0.25;
      S2.v.set(Math.cos(a + ja) * spd, s * (0.8 + rnd() * 2.2), Math.sin(a + ja) * spd);
      S2.w = 8 + rnd() * 14; S2.q.setFromAxisAngle(S2.ax, rnd() * 6.283);
      var sz = s * (0.14 + rnd() * 0.22); S2.sc.set(sz, sz * (0.6 + rnd() * 0.8), sz * (0.6 + rnd() * 0.6));
      S2.tm = T_BU + 0.2 + rnd() * 0.2;
    }
    glit.emit(Math.round(46 * Q), function (j, r) { var a2 = r() * 6.283, sp2 = s * (1.2 + r() * 3.6), rr = s * 0.9 * r(); return { p: [at.x + Math.cos(a2) * rr, y0 + (r() - 0.3) * s, at.z + Math.sin(a2) * rr], v: [Math.cos(a2) * sp2, s * (1 + r() * 4), Math.sin(a2) * sp2], life: 0.7 + r() * 0.8, delay: r() * 0.05 }; });
    dust.emit(Math.round(20 * Q), function (j, r) { var a2 = r() * 6.283, sp2 = s * (1.2 + r() * 2.6), rr = s * 0.9 * r(); return { p: [at.x + Math.cos(a2) * rr, 0.2 + r() * y0, at.z + Math.sin(a2) * rr], v: [Math.cos(a2) * sp2, s * (0.5 + r() * 1.6), Math.sin(a2) * sp2], life: 0.6 + r() * 0.6 }; });
    halo.material.color.set(FROST_GLACIER); flare.position.set(at.x, y0 + s * 0.6, at.z);
  }

  return { update: function (dt) {
    t += dt;
    crackU.uTime.value = t; su.uTime.value = t; fu.uTime.value = t;

    // The 1 turns sickly blue while the rime closes in, then burns cold when the ice lets go.
    var nc, nk;
    if (t < T_BU) { nc = mix(th.burn, t < T_CK ? sickCol : entombCol, smooth(T_R0, T_R0 + 0.35, t)); nk = 1.6 + 0.6 * smooth(T_R0, T_R0 + 0.35, t); if (t > T_CK) nk += 0.7 * smooth(T_CK, T_BU, t); }
    else { nc = finalCol; nk = 1.75 + 1.4 * Math.exp(-(t - T_BU) * 3.4); }
    numeral(nc, nk);
    if (pale) shadeDie(die, 1 - 0.12 * smooth(T_R0, T_R0 + 0.35, t));
    tintDie(die, FROST_GLACIER, 0.3 * smooth(T_R0, T_R1, t) * (1 - smooth(1.6, 1.9, t)));
    dim.material.opacity = 0.32 * smooth(0, 0.4, t) * (1 - smooth(1.45, 1.95, t));
    halo.material.opacity = (t < T_BU ? 0.22 * smooth(T_R0, T_R1, t) : 0.2 + 0.4 * Math.exp(-(t - T_BU) * 3.2)) * (1 - smooth(1.6, 1.95, t));

    // The ring of frost on the page grows with the rime and thaws with it.
    if (painted < batches.length) {
      for (var b = 0; b < perFrame && painted < batches.length; b++) frostPaintBatch(fg, batches[painted++], wScale);
      if (painted >= batches.length) fernTex.needsUpdate = true;
    }
    var gk = clamp01((t - (T_R0 + 0.02)) / 0.85);
    fu.uGrow.value = painted >= batches.length ? 1 - Math.pow(1 - gk, 1.7) : 0;
    fu.uThaw.value = smooth(1.3, 1.9, t); fu.uOn.value = 1 - smooth(1.8, 1.95, t);
    fu.uFlash.value = t > T_BU ? 0.7 * Math.exp(-(t - T_BU) * 6) : 0;
    su.uCover.value = smooth(T_R0, T_R1, t); su.uThaw.value = smooth(1.35, 1.9, t);

    // The shard crossing the page.
    if (FROM && t < T_HIT) {
      var u = t / T_HIT, k1 = 1 - (1 - u) * (1 - u);
      trav.p.set(S.x + (IMP.x - S.x) * k1, trest + s * 0.6 * Math.sin(Math.PI * Math.min(1, u * 1.1)), S.z + (IMP.z - S.z) * k1);
      trav.q.premultiply(dq.setFromAxisAngle(trav.ax, 26 * dt));
      m4.compose(trav.p, trav.q, trav.sc); shards.setMatrixAt(0, m4);
      trail += 80 * Q * dt;
      while (trail >= 1) { trail -= 1; glit.emit(1, function (j, r) { return { p: [trav.p.x, trav.p.y, trav.p.z], v: [(r() - 0.5) * s, s * r() * 1.2, (r() - 0.5) * s], life: 0.3 + r() * 0.3 }; }); }
    } else if (!hitDone) { hitDone = true; hit(); shards.setMatrixAt(0, ZERO); }

    // The ice skin closes over the die, cracks from where the cold came in, and lets go.
    if (!burst) {
      var shake = t > T_CK ? s * 0.012 * smooth(T_CK, T_BU, t) : 0;
      ku.uFade.value = smooth(T_R0 + 0.3, T_R1, t);
      ku.uGlowK.value = 1 + 0.25 * Math.sin(t * 5) + (t >= T_CK ? 2.2 * Math.exp(-(t - T_CK) * 16) : 0);
      if (t >= T_CK) {
        crackU.uCrack.value = 0.05 + out3((t - T_CK) / 0.18) * s * 3.6;
        crackU.uCrackK.value = (lin ? 1.4 : 1.6) * (1 + 0.6 * smooth(T_CK + 0.18, T_BU, t)) * (0.8 + 0.2 * Math.sin((t - T_CK) * 40)) + 2.2 * Math.exp(-(t - T_CK) * 14);
      }
      g.position.set(p0.x + (rnd() - 0.5) * shake, p0.y, p0.z + (rnd() - 0.5) * shake);
      if (t >= T_BU) { shatter(); g.position.copy(p0); }
    }
    var fl = t >= T_CK ? 0.4 * Math.exp(-(t - T_CK) * 11) : 0;
    if (t >= T_BU) fl += 0.45 * Math.exp(-(t - T_BU) * 6);
    if (FROM && t >= T_HIT) fl += 0.4 * Math.exp(-(t - T_HIT) * 9);
    flare.material.opacity = Math.min(1, fl); flare.scale.setScalar(s * 3.4 * (0.8 + 0.4 * Math.min(1, fl))); flare.material.rotation = t * 0.3;

    if (burst) {
      var G = s * 24;
      for (i = FROM ? 1 : 0; i < NSH; i++) {
        var S3 = sh[i];
        S3.v.y -= G * dt; S3.p.addScaledVector(S3.v, dt);
        var rest = S3.sc.y * 0.3, mk = clamp01((t - S3.tm) / 0.4);
        if (S3.p.y < rest) { S3.p.y = rest; if (S3.v.y < 0) { S3.v.y *= -0.3; if (S3.v.y < s * 0.5) S3.v.y = 0; S3.v.x *= 0.72; S3.v.z *= 0.72; S3.w *= 0.6; } }
        if (S3.p.y <= rest + 1e-4) { var fr = Math.max(0, 1 - 3.5 * dt); S3.v.x *= fr; S3.v.z *= fr; S3.w *= Math.max(0, 1 - 5 * dt); }
        S3.q.premultiply(dq.setFromAxisAngle(S3.ax, S3.w * dt));
        if (mk >= 1) { shards.setMatrixAt(i, ZERO); continue; }
        var spread = (1 - 0.6 * mk) * (1 - in3(mk));
        v3.set(S3.sc.x * spread, S3.sc.y * Math.pow(1 - mk, 2), S3.sc.z * spread);
        m4.compose(S3.p, S3.q, v3); shards.setMatrixAt(i, m4);
      }
    }
    shards.instanceMatrix.needsUpdate = true;
    var pf = 1 - smooth(1.65, 1.95, t); glit.update(dt, pf); dust.update(dt, pf);

    if (t >= END) {
      shards.dispose();
      drop(st, [dim, fernMesh, shell, skin, shards, halo, flare, glit.obj, dust.obj]);
      fernTex.dispose();
      g.position.copy(p0); g.quaternion.copy(q0);
      tintDie(die, th.burn, 0);
      numeral(finalCol, pale ? 2 : 1.6);
      if (pale) shadeDie(die, 0.88);
      return false;
    }
    return true;
  } };
} registerCrit('frost', { name: 'Frostbite', kind: 'fail', what: 'The 1 turns a sickly cold blue as frost spreads across the page and a block of ice closes over the die, then the ice cracks and shatters into shards that skitter away and melt, leaving the 1 burning cold blue-white.', fx: fxFrost,
  // beat: the ice bursts; peak: the flash has gone and the shards are melting.
  beat: 3.3, peak: 3.9, echo: fxFrostEcho, echoWhat: 'Frost creeps over the die and cracks.' });
