// ---------- Crit success: Dragon's hoard ----------
// The die glows gold from inside, shivers, and bursts: minted gold coins and cut gems (ruby, emerald,
// sapphire, diamond) fountain up out of it toward the viewer, tumbling and flashing as they turn, then
// rain down round it. They bounce, clatter and spin down like real coins and settle into a small
// glittering pile. The pile glitters, a ripple of light runs out from the die and the hoard winks out
// piece by piece in sparks, leaving the 20 burning gold.
//
// Coins are one instanced mesh (a lathe-free disc with a raised rim and a stamped face, real polished
// gold lit by the room), gems are one instanced mesh per cut. Every piece is a small rigid body falling
// on a height map of what has already settled, so later coins land on earlier ones and the pile heaps.
// Glints on turning metal are chosen on the CPU from each piece's orientation against the key light,
// and drawn as one set of points.
import {
  GEMKEY, GEM_FS, GEM_VS, TAN, canvas, clamp01, dimmer, drop, env4, gemMat, glow, glowLayer,
  glowNumeral, heightToNormal, isLight, mulberry, out3, registerCrit, shadeDie, smooth, spriteTex,
  starTex, tintDie, share
} from '../engine.js';

var HOARD = { gold: '#ffc23a', amber: '#ff9a1f', pale: '#fff0b0', num: '#ffd25a', numDeep: '#d9780a', body: '#e8b33c',
  ruby: '#d0102e', emerald: '#00b25a', sapphire: '#0c4cf0', diamond: '#f2f6ff' };

// ---------- The stamped face ----------
// One height image holds both faces side by side (obverse left, reverse right): a beaded border, a
// ring, a crown on one and a dragon's initial on the other. It becomes a normal map and a roughness
// map: the field is mirror-polished and the stamped relief frosted, as on a proof coin.
var HOARD_MAPS = null;
function hoardStamp(g, ox, S, crown) {
  var cx = ox + S / 2, cy = S / 2, k = S / 256, i;
  g.save(); g.shadowColor = 'rgb(255,255,255)'; g.shadowBlur = 3 * k;
  g.fillStyle = g.strokeStyle = 'rgb(214,214,214)';
  for (i = 0; i < 44; i++) { var a = i / 44 * Math.PI * 2; g.beginPath(); g.arc(cx + Math.cos(a) * 103 * k, cy + Math.sin(a) * 103 * k, 3.4 * k, 0, 7); g.fill(); }
  g.lineWidth = 3 * k; g.beginPath(); g.arc(cx, cy, 88 * k, 0, 7); g.stroke();
  g.lineWidth = 1.6 * k; g.beginPath(); g.arc(cx, cy, 80 * k, 0, 7); g.stroke();
  if (crown) {
    // A crown of five points on a jewelled band, between two sprays of leaves.
    g.lineJoin = 'round'; g.lineWidth = 2 * k;
    g.beginPath(); g.moveTo(cx - 44 * k, cy + 18 * k); g.lineTo(cx - 48 * k, cy - 24 * k); g.lineTo(cx - 24 * k, cy - 4 * k); g.lineTo(cx - 12 * k, cy - 34 * k);
    g.lineTo(cx, cy - 6 * k); g.lineTo(cx + 12 * k, cy - 34 * k); g.lineTo(cx + 24 * k, cy - 4 * k); g.lineTo(cx + 48 * k, cy - 24 * k); g.lineTo(cx + 44 * k, cy + 18 * k); g.closePath();
    g.fill(); g.stroke();
    [[-48, -26], [-12, -37], [12, -37], [48, -26], [0, -9]].forEach(function (p) { g.beginPath(); g.arc(cx + p[0] * k, cy + p[1] * k, 5 * k, 0, 7); g.fill(); });
    g.fillRect(cx - 47 * k, cy + 20 * k, 94 * k, 15 * k);
    g.shadowBlur = 0; g.fillStyle = 'rgb(40,40,40)';
    [-28, 0, 28].forEach(function (x) { g.beginPath(); g.arc(cx + x * k, cy + 27.5 * k, 4.2 * k, 0, 7); g.fill(); });
    g.fillStyle = g.strokeStyle = 'rgb(214,214,214)'; g.shadowBlur = 3 * k; g.lineWidth = 3 * k;
    for (var sd = -1; sd <= 1; sd += 2) {
      g.beginPath(); g.arc(cx, cy + 6 * k, 66 * k, Math.PI * (sd < 0 ? 0.62 : 0.12), Math.PI * (sd < 0 ? 0.88 : 0.38)); g.stroke();
      for (i = 0; i < 6; i++) {
        var an = Math.PI * (sd < 0 ? 0.64 + i * 0.044 : 0.14 + i * 0.044), px = cx + Math.cos(an) * 66 * k, py = cy + 6 * k + Math.sin(an) * 66 * k;
        g.beginPath(); g.ellipse(px, py, 7 * k, 3.2 * k, an + 0.9 * sd, 0, 7); g.fill();
      }
    }
  } else {
    // A sunburst behind a heavy serif D.
    g.lineWidth = 2.4 * k;
    for (i = 0; i < 16; i++) { var r = i / 16 * Math.PI * 2; g.beginPath(); g.moveTo(cx + Math.cos(r) * 52 * k, cy + Math.sin(r) * 52 * k); g.lineTo(cx + Math.cos(r) * 76 * k, cy + Math.sin(r) * 76 * k); g.stroke(); }
    g.font = '700 ' + Math.round(104 * k) + 'px Cinzel, Georgia, "Times New Roman", serif'; g.textAlign = 'center'; g.textBaseline = 'middle';
    g.fillStyle = 'rgb(30,30,30)'; g.shadowBlur = 0; g.fillText('D', cx + 2 * k, cy + 5 * k);
    g.fillStyle = 'rgb(236,236,236)'; g.shadowBlur = 3 * k; g.fillText('D', cx, cy + 3 * k);
  }
  g.restore();
}
function hoardCoinMaps(T) {
  if (HOARD_MAPS) return HOARD_MAPS;
  var S = 256, c = canvas(S * 2, S), g = c.getContext('2d', { willReadFrequently: true });
  g.fillStyle = 'rgb(96,96,96)'; g.fillRect(0, 0, S * 2, S);
  hoardStamp(g, 0, S, true); hoardStamp(g, S, S, false);
  var nrm = heightToNormal(c, 3.4), d = g.getImageData(0, 0, S * 2, S), rc = canvas(S * 2, S), rg = rc.getContext('2d'), rd = rg.createImageData(S * 2, S);
  for (var i = 0; i < d.data.length; i += 4) {
    var relief = clamp01((d.data[i] - 96) / 100), v = Math.round(255 * (0.2 + 0.3 * relief));
    rd.data[i] = rd.data[i + 1] = rd.data[i + 2] = v; rd.data[i + 3] = 255;
  }
  rg.putImageData(rd, 0, 0);
  var nt = new T.CanvasTexture(nrm), rt = new T.CanvasTexture(rc);
  nt.anisotropy = rt.anisotropy = 4; share(nt); share(rt);
  HOARD_MAPS = { normal: nt, rough: rt };
  return HOARD_MAPS;
}

// ---------- The coin ----------
// Radius 1, a raised rim and a recessed field on both faces, a plain edge. The faces are mapped flat
// from above onto the two halves of the stamp image; the edge sits on a bare strip of it.
var HOARD_HH = 0.17;
function hoardCoinGeo(T, hd) {
  var N = hd ? 40 : 28, HH = HOARD_HH, pos = [], nor = [], uv = [], idx = [];
  // The profile from the middle out: [radius, height]; the last point drops to the edge.
  var prof = [[0, 0.78 * HH], [0.82, 0.78 * HH], [0.885, HH], [0.965, HH], [0.995, 0.8 * HH]];
  function vert(x, y, z, nx, ny, nz, u, v) { pos.push(x, y, z); nor.push(nx, ny, nz); uv.push(u, v); return pos.length / 3 - 1; }
  function tri(a, b, c, nx, ny, nz) {
    var ax = pos[a * 3], ay = pos[a * 3 + 1], az = pos[a * 3 + 2], ux = pos[b * 3] - ax, uy = pos[b * 3 + 1] - ay, uz = pos[b * 3 + 2] - az, vx = pos[c * 3] - ax, vy = pos[c * 3 + 1] - ay, vz = pos[c * 3 + 2] - az;
    var cx = uy * vz - uz * vy, cy = uz * vx - ux * vz, cz = ux * vy - uy * vx;
    if (cx * nx + cy * ny + cz * nz < 0) idx.push(a, c, b); else idx.push(a, b, c);
  }
  [1, -1].forEach(function (f) {
    function uvOf(r, an) { var x = r * Math.cos(an), z = r * Math.sin(an); return f > 0 ? [0.25 + 0.25 * x, 0.5 + 0.5 * z] : [0.75 - 0.25 * x, 0.5 + 0.5 * z]; }
    var mid = prof[0], c0 = vert(0, f * mid[1], 0, 0, f, 0, uvOf(0, 0)[0], uvOf(0, 0)[1]);
    for (var b = 0; b < prof.length - 1; b++) {
      var A = prof[b], B = prof[b + 1], dr = B[0] - A[0], dh = B[1] - A[1], nl = Math.hypot(dr, dh) || 1;
      // The band's outward normal in the (radius, height) plane, up on the top face and mirrored down on the bottom.
      var bnr = -dh / nl, bny = dr / nl * f, rowA = [], rowB = [];
      for (var i = 0; i < N; i++) {
        var an = i / N * Math.PI * 2, ca = Math.cos(an), sa = Math.sin(an), uA = uvOf(A[0], an), uB = uvOf(B[0], an);
        rowA.push(b === 0 ? c0 : vert(A[0] * ca, f * A[1], A[0] * sa, bnr * ca, bny, bnr * sa, uA[0], uA[1]));
        rowB.push(vert(B[0] * ca, f * B[1], B[0] * sa, bnr * ca, bny, bnr * sa, uB[0], uB[1]));
      }
      for (i = 0; i < N; i++) {
        var j = (i + 1) % N;
        if (b === 0) tri(c0, rowB[i], rowB[j], 0, f, 0); else { tri(rowA[i], rowB[i], rowB[j], 0, f, 0); tri(rowA[i], rowB[j], rowA[j], 0, f, 0); }
      }
    }
  });
  // The edge: a smooth band from the top bevel to the bottom one.
  var E = prof[prof.length - 1], ra = [], rb = [];
  for (var i = 0; i < N; i++) {
    var an = i / N * Math.PI * 2, ca = Math.cos(an), sa = Math.sin(an), eu = 0.02 + 0.015 * (i / N);
    ra.push(vert(E[0] * ca, E[1], E[0] * sa, ca, 0, sa, eu, 0.03)); rb.push(vert(E[0] * ca, -E[1], E[0] * sa, ca, 0, sa, eu, 0.05));
  }
  for (i = 0; i < N; i++) { var j2 = (i + 1) % N; tri(ra[i], rb[i], rb[j2], Math.cos((i + 0.5) / N * 6.2832), 0, Math.sin((i + 0.5) / N * 6.2832)); tri(ra[i], rb[j2], ra[j2], Math.cos((i + 0.5) / N * 6.2832), 0, Math.sin((i + 0.5) / N * 6.2832)); }
  var geo = new T.BufferGeometry();
  geo.setAttribute('position', new T.Float32BufferAttribute(pos, 3)); geo.setAttribute('normal', new T.Float32BufferAttribute(nor, 3)); geo.setAttribute('uv', new T.Float32BufferAttribute(uv, 2));
  geo.setIndex(idx); geo.computeBoundingSphere();
  return geo;
}
// Where a coin touches the ground: its face rims and middles, in a unit-radius coin's own space.
function hoardCoinPts() {
  var out = [], HH = HOARD_HH;
  [1, -1].forEach(function (f) {
    out.push(0, f * 0.78 * HH, 0);
    for (var i = 0; i < 6; i++) { var a = i / 6 * Math.PI * 2 + (f > 0 ? 0 : 0.5); out.push(Math.cos(a) * 0.94, f * HH, Math.sin(a) * 0.94); }
  });
  return new Float32Array(out);
}

// ---------- The gems ----------
// Four cuts, each a lathe of a few rings so it has facets in two bands: a round brilliant (diamond),
// an oval (ruby), an octagon step cut stretched to a rectangle (emerald) and a four-sided princess
// cut (sapphire). The gem shader of the jewelled die lights them; here only the shapes are made.
var HOARD_CUTS = [
  { name: 'diamond', col: HOARD.diamond, seg: 16, z: 1, prof: [[0, -0.86], [0.5, -0.45], [1, -0.03], [1, 0.03], [0.78, 0.2], [0.56, 0.34], [0, 0.34]], w: 3 },
  { name: 'ruby', col: HOARD.ruby, seg: 14, z: 0.74, prof: [[0, -0.8], [0.55, -0.4], [1, -0.04], [1, 0.04], [0.72, 0.26], [0.5, 0.36], [0, 0.36]], w: 3 },
  { name: 'emerald', col: HOARD.emerald, seg: 8, z: 0.7, prof: [[0, -0.7], [0.45, -0.52], [0.78, -0.3], [1, -0.04], [1, 0.04], [0.88, 0.12], [0.7, 0.22], [0.52, 0.3], [0, 0.3]], w: 2 },
  { name: 'sapphire', col: HOARD.sapphire, seg: 4, z: 1, prof: [[0, -0.85], [0.7, -0.3], [1, -0.04], [1, 0.04], [0.62, 0.34], [0, 0.34]], w: 2 }
];
// The jewelled die's gem shader, opened up a little: these stones are big on screen and lie face-up to
// the camera, so a facet that sends back the dark under the stone must still show the stone's colour,
// and the colour must not drain away in the thick of a big stone. Own material (disposed with the hoard);
// with no environment map the die's plain gem material stands in.
function hoardGemMat(T, cut, env, fire) {
  if (!env || typeof GEM_FS !== 'string' || typeof GEM_VS !== 'string' || typeof GEMKEY === 'undefined') return gemMat(T, cut.col, env, fire);
  if (!GEMKEY.value) GEMKEY.value = new T.Vector3(-0.55, 0.62, -0.55).normalize();
  // A stone lying pavilion-up shows its pavilion to the camera, which the stock shader lights as a dull
  // grey cone: there the axis is turned over so the same facet mosaic and colour show as on the crown.
  var fs = GEM_FS.replace('vec3 ax = normalize( vAxis ), b1', 'vec3 ax = normalize( vAxis ) * ( dot( N, normalize( vAxis ) ) < -0.05 ? -1.0 : 1.0 ), b1').replace('mix( 0.12, 3.2, lit )', 'mix( 0.5, 3.0, lit )').replace('  gl_FragColor = vec4( col, 1.0 );', '  col += ( 1.0 - F ) * tint * 0.22;\n  gl_FragColor = vec4( col, 1.0 );');
  var m = new T.ShaderMaterial({ vertexShader: GEM_VS, fragmentShader: fs, defines: fire ? { GEM_FIRE: '' } : {}, extensions: { derivatives: true },
    uniforms: { uEnv: { value: env }, uColor: { value: new T.Color(cut.col) }, uKey: GEMKEY, uDepth: { value: 0.8 } } });
  return m;
}
function hoardCutGeo(T, cut) {
  var pts = cut.prof.map(function (p) { return new T.Vector2(p[0], p[1]); });
  var geo = new T.LatheGeometry(pts, cut.seg).toNonIndexed(); geo.scale(1, 1, cut.z); geo.computeVertexNormals(); geo.computeBoundingSphere();
  // Facet normals for glints: the distinct directions the faces of the cut look in.
  var nm = geo.attributes.normal, seen = {}, fac = [], pav = [];
  for (var i = 0; i < nm.count; i += 3) {
    var x = nm.getX(i), y = nm.getY(i), z = nm.getZ(i), key = Math.round(y * 6) + ':' + Math.round(Math.atan2(z, x) * 6);
    if (seen[key]) continue;
    seen[key] = 1;
    if (y > -0.3) { if (fac.length < 24) fac.push([x, y, z]); } else if (pav.length < 24) pav.push([x, y, z]);
  }
  function unit(f) { var l = Math.hypot(f[0], f[1], f[2]) || 1; return [f[0] / l, f[1] / l, f[2] / l]; }
  cut.facets = fac.map(unit); cut.pavFacets = pav.length ? pav.map(unit) : cut.facets;
  // Contact points: the girdle and table rings and the culet, with the stretch applied.
  var cp = [];
  cut.prof.forEach(function (p) {
    if (p[0] < 1e-3) { cp.push(0, p[1], 0); return; }
    var n = Math.min(cut.seg, 8);
    for (var k = 0; k < n; k++) { var a = k / n * Math.PI * 2; cp.push(p[0] * Math.cos(a), p[1], p[0] * Math.sin(a) * cut.z); }
  });
  cut.pts = new Float32Array(cp);
  return geo;
}

// ---------- Falling pieces ----------
// A small rigid-body world over a height map. Each piece is a convex body described by contact points
// in its own space; the ground is the height map (the page, plus every piece that has already come
// to rest), so a later coin lands on the heap and tilts with it. Contacts use impulses with friction,
// so coins bounce, clatter, wobble on their rims and flatten like real ones. Sleeping pieces cost
// nothing. Everything is plain numbers: nothing is allocated while it runs.
var HOARD_R = [0, 0, 0], HOARD_I = [0, 0, 0];
// Rotate a vector by the body's orientation (into HOARD_R), or by its inverse.
function hoardRot(b, px, py, pz) {
  var qx = b.qx, qy = b.qy, qz = b.qz, qw = b.qw;
  var tx = 2 * (qy * pz - qz * py), ty = 2 * (qz * px - qx * pz), tz = 2 * (qx * py - qy * px);
  HOARD_R[0] = px + qw * tx + (qy * tz - qz * ty); HOARD_R[1] = py + qw * ty + (qz * tx - qx * tz); HOARD_R[2] = pz + qw * tz + (qx * ty - qy * tx);
}
function hoardRotInv(b, px, py, pz) {
  var qx = -b.qx, qy = -b.qy, qz = -b.qz, qw = b.qw;
  var tx = 2 * (qy * pz - qz * py), ty = 2 * (qz * px - qx * pz), tz = 2 * (qx * py - qy * px);
  HOARD_R[0] = px + qw * tx + (qy * tz - qz * ty); HOARD_R[1] = py + qw * ty + (qz * tx - qx * tz); HOARD_R[2] = pz + qw * tz + (qx * ty - qy * tx);
}
// The inverse inertia times a world vector, into HOARD_I.
function hoardInvI(b, ux, uy, uz) {
  hoardRotInv(b, ux, uy, uz);
  var bx = HOARD_R[0] * b.iP, by = HOARD_R[1] * b.iA, bz = HOARD_R[2] * b.iP;
  hoardRot(b, bx, by, bz);
  HOARD_I[0] = HOARD_R[0]; HOARD_I[1] = HOARD_R[1]; HOARD_I[2] = HOARD_R[2];
}

// P: { g, s, cx, cz, half, cs (cell), die: { x, y, z, r } | null, zones: [{ x, z, r }], box: { x0, x1, z0, z1 }, hit(b, speed) }
function hoardWorld(P) {
  var cs = P.cs, n = Math.ceil(P.half * 2 / cs) + 1, ox = P.cx - P.half, oz = P.cz - P.half, H = new Float32Array(n * n), hmax = 0, g = P.g, s = P.s;
  var skin = s * 0.002, sub = 1 / 240;
  function hf(x, z) {
    var u = (x - ox) / cs, v = (z - oz) / cs;
    if (u < 0 || v < 0 || u >= n - 1 || v >= n - 1) return 0;
    var i = u | 0, j = v | 0, fu = u - i, fv = v - j, a = H[j * n + i], b = H[j * n + i + 1], c = H[(j + 1) * n + i], d = H[(j + 1) * n + i + 1];
    return a + (b - a) * fu + (c - a) * fv + (a - b - c + d) * fu * fv;
  }
  // A piece at rest becomes ground: a coin as its tilted slab, a gem as a dome.
  function deposit(b) {
    hoardRot(b, 0, 1, 0); var ax = HOARD_R[0], ay = HOARD_R[1], az = HOARD_R[2], r = b.rad, i, j;
    var i0 = Math.max(0, Math.floor((b.x - r - ox) / cs)), i1 = Math.min(n - 1, Math.ceil((b.x + r - ox) / cs)), j0 = Math.max(0, Math.floor((b.z - r - oz) / cs)), j1 = Math.min(n - 1, Math.ceil((b.z + r - oz) / cs));
    for (j = j0; j <= j1; j++) for (i = i0; i <= i1; i++) {
      var dx = ox + i * cs - b.x, dz = oz + j * cs - b.z, d2 = dx * dx + dz * dz, y;
      if (d2 > r * r) continue;
      if (b.kind === 0) {
        var sy = ay < 0 ? -1 : 1, ayy = Math.max(Math.abs(ay), 0.35);
        y = b.y + (HOARD_HH * r - sy * (ax * dx + az * dz)) / ayy;
      } else y = b.y + r * 0.85 * Math.sqrt(Math.max(0, 1 - d2 / (r * r)));
      if (y > H[j * n + i]) { H[j * n + i] = y; if (y > hmax) hmax = y; }
    }
  }
  function sleepBody(b) { b.vx = b.vy = b.vz = b.wx = b.wy = b.wz = 0; b.mode = 2; deposit(b); }
  // A piece that has stopped moving: the effect may take it over (a stone lying pavilion-up turns over)
  // before it is left to rest.
  function rest(b) { if (P.rest && P.rest(b)) return; sleepBody(b); }
  // The height at which a body's lowest contact point just touches the ground.
  function groundY(b) {
    var y = -1e9, k;
    for (k = 0; k < b.np; k++) { hoardRot(b, b.pts[k * 3], b.pts[k * 3 + 1], b.pts[k * 3 + 2]); var gy = hf(b.x + HOARD_R[0], b.z + HOARD_R[2]) - HOARD_R[1]; if (gy > y) y = gy; }
    return y;
  }
  // One body, one small step.
  function step(b, h) {
    var rad = b.rad, i, k;
    b.vy -= g * h;
    if (b.lam) {
      // Torque-free wobble of a disc: the spin components across its axis turn at a rate set by the spin along it.
      hoardRotInv(b, b.wx, b.wy, b.wz); var wbx = HOARD_R[0], wby = HOARD_R[1], wbz = HOARD_R[2], ph = b.lam * wby * h, cp = Math.cos(ph), sp = Math.sin(ph);
      hoardRot(b, wbx * cp + wbz * sp, wby, -wbx * sp + wbz * cp); b.wx = HOARD_R[0]; b.wy = HOARD_R[1]; b.wz = HOARD_R[2];
    }
    if (b.right && b.vy < 0 && b.touch < 0.02) {
      // Coming down: a stone swings its table up and spins about its own axis, which keeps it there, so it
      // lands crown-up and settles leaning on its pavilion rather than lying with the pavilion to the sky.
      hoardRot(b, 0, 1, 0); var gx = HOARD_R[0], gy = HOARD_R[1], gz = HOARD_R[2], gs = b.wx * gx + b.wy * gy + b.wz * gz, gk = 6 * h;
      b.wx += (gx * gs - b.wx) * gk - gz * 45 * h; b.wy += (gy * gs - b.wy) * gk; b.wz += (gz * gs - b.wz) * gk + gx * 45 * h;
    }
    b.x += b.vx * h; b.y += b.vy * h; b.z += b.vz * h;
    var qx = b.qx, qy = b.qy, qz = b.qz, qw = b.qw, hh = 0.5 * h;
    b.qx = qx + hh * (qw * b.wx + b.wy * qz - b.wz * qy); b.qy = qy + hh * (qw * b.wy + b.wz * qx - b.wx * qz);
    b.qz = qz + hh * (qw * b.wz + b.wx * qy - b.wy * qx); b.qw = qw + hh * (-b.wx * qx - b.wy * qy - b.wz * qz);
    var ql = 1 / Math.sqrt(b.qx * b.qx + b.qy * b.qy + b.qz * b.qz + b.qw * b.qw); b.qx *= ql; b.qy *= ql; b.qz *= ql; b.qw *= ql;
    b.age += h;
    // Things it can hit that are not the ground: the die, the dice to keep clear, the edge of the page.
    var d = P.die;
    if (d && b.age > b.grace) {
      var dx = b.x - d.x, dy = b.y - d.y, dz = b.z - d.z, dl = Math.sqrt(dx * dx + dy * dy + dz * dz) || 1e-4, lim = d.r + rad * 0.7;
      if (dl < lim) {
        var nx = dx / dl, ny = dy / dl, nz = dz / dl, vn = b.vx * nx + b.vy * ny + b.vz * nz;
        b.x = d.x + nx * lim; b.y = d.y + ny * lim; b.z = d.z + nz * lim;
        if (vn < 0) { var jn = -(1 + 0.35) * vn; b.vx += nx * jn; b.vy += ny * jn; b.vz += nz * jn; b.vx *= 0.92; b.vz *= 0.92; if (P.hit && -vn > s * 2) P.hit(b, -vn); }
        // Nothing balances on the crown: a piece that lands on top is nudged off it.
        if (ny > 0.7) { var hl = Math.sqrt(dx * dx + dz * dz) || 1; b.vx += dx / hl * s * 30 * h; b.vz += dz / hl * s * 30 * h; }
      }
    }
    for (i = 0; i < P.zones.length && !b.noZone; i++) {
      var z = P.zones[i], zx = b.x - z.x, zz = b.z - z.z, zl = Math.sqrt(zx * zx + zz * zz) || 1e-4, zlim = z.r + rad + 0.8 * s;
      if (zl < zlim) {
        var znx = zx / zl, znz = zz / zl, zvn = b.vx * znx + b.vz * znz;
        b.x = z.x + znx * zlim; b.z = z.z + znz * zlim;
        if (zvn < 0) { b.vx -= 1.5 * zvn * znx; b.vz -= 1.5 * zvn * znz; }
      }
    }
    var bx = P.box;
    if (b.x < bx.x0) { b.x = bx.x0; if (b.vx < 0) b.vx *= -0.5; } else if (b.x > bx.x1) { b.x = bx.x1; if (b.vx > 0) b.vx *= -0.5; }
    if (b.z < bx.z0) { b.z = bx.z0; if (b.vz < 0) b.vz *= -0.5; } else if (b.z > bx.z1) { b.z = bx.z1; if (b.vz > 0) b.vz *= -0.5; }

    // The ground.
    var contact = false;
    if (b.y - rad < hmax + skin * 4) {
      var pts = b.pts, maxPen = -1, hit = 0;
      for (k = 0; k < b.np; k++) {
        hoardRot(b, pts[k * 3], pts[k * 3 + 1], pts[k * 3 + 2]);
        var rx = HOARD_R[0], ry = HOARD_R[1], rz = HOARD_R[2], pen = hf(b.x + rx, b.z + rz) - (b.y + ry);
        if (pen < -skin) continue;
        contact = true; if (pen > maxPen) maxPen = pen;
        var vpy = b.vy + b.wz * rx - b.wx * rz;
        if (vpy >= 0) continue;
        hoardInvI(b, -rz, 0, rx); var iwx = HOARD_I[0], iwy = HOARD_I[1], iwz = HOARD_I[2];
        var kn = 1 + (iwz * rx - iwx * rz), jn = -(1 + (-vpy > b.vthr ? b.e : 0)) * vpy / kn;
        if (-vpy > hit) hit = -vpy;
        b.vy += jn; b.wx += jn * iwx; b.wy += jn * iwy; b.wz += jn * iwz;
        var vpx = b.vx + b.wy * rz - b.wz * ry, vpz = b.vz + b.wx * ry - b.wy * rx, vt = Math.sqrt(vpx * vpx + vpz * vpz);
        if (vt > 1e-5) {
          var tx = vpx / vt, tz = vpz / vt;
          hoardInvI(b, ry * tz, rz * tx - rx * tz, -ry * tx);
          var jx = HOARD_I[0], jy = HOARD_I[1], jz = HOARD_I[2], kt = 1 + (jy * rz - jz * ry) * tx + (jx * ry - jy * rx) * tz, jt = Math.min(vt / kt, b.mu * jn);
          b.vx -= jt * tx; b.vz -= jt * tz; b.wx -= jt * jx; b.wy -= jt * jy; b.wz -= jt * jz;
        }
      }
      if (maxPen > 0) b.y += Math.min(maxPen, rad * 0.5) * 0.85;
      if (hit > 0 && P.hit) P.hit(b, hit);
    }
    // A little air, and a lot of hand on the table once it has been down a while: nothing rattles forever.
    var dr = 1 - 0.12 * h;
    if (contact) {
      b.touch += h;
      var hold = b.touch > b.settle ? 1 - 10 * h : 1 - 0.9 * h;
      dr *= hold; b.wx *= hold; b.wy *= hold; b.wz *= hold;
      var sp2 = b.vx * b.vx + b.vy * b.vy + b.vz * b.vz, w2 = b.wx * b.wx + b.wy * b.wy + b.wz * b.wz, late = b.touch > b.settle + 0.4;
      if (sp2 < s * s * (late ? 0.64 : 0.16) && w2 < (late ? 9 : 2.6)) { b.still += h; if (b.still > (late ? 0.08 : 0.14)) rest(b); } else b.still = 0;
    } else b.still = 0;
    b.vx *= dr; b.vy *= dr; b.vz *= dr;
  }
  return {
    hf: hf, deposit: deposit, step: step, sleep: sleepBody, groundY: groundY, top: function () { return hmax; },
    // Advance every awake body by dt in small fixed steps (never more than a handful per frame).
    run: function (bodies, dt) {
      var cnt = Math.max(1, Math.min(8, Math.ceil(dt / sub))), h = dt / cnt, i, c;
      for (c = 0; c < cnt; c++) for (i = 0; i < bodies.length; i++) if (bodies[i].mode === 1) step(bodies[i], h);
    }
  };
}

// ---------- Glints ----------
// One set of points for every glint: one per piece, flaring when a face of it mirrors the key light
// toward the camera, plus a pool of short-lived sparks. Each is a four-pointed star with a hot white
// core and an amber halo, blended premultiplied: the core adds light, the halo also tints the page,
// so a glint reads on a white page as well as a dark one.
var HOARD_GLINT_VS = [
  'uniform float uPx; attribute float aS; attribute vec2 aI; varying vec2 vI;',
  'void main(){ vec4 mv = modelViewMatrix * vec4(position, 1.0); gl_Position = projectionMatrix * mv; gl_PointSize = clamp(aS * uPx / max(-mv.z, 0.1), 0.0, 120.0); vI = aI; }'
].join('\n');
var HOARD_GLINT_FS = [
  'uniform vec3 uHot, uAmber; varying vec2 vI;',
  'void main(){',
  '  if (vI.x < 0.004) discard;',
  '  vec2 p = gl_PointCoord * 2.0 - 1.0; float c = cos(vI.y), sn = sin(vI.y); p = vec2(c * p.x - sn * p.y, sn * p.x + c * p.y);',
  '  float r2 = dot(p, p), core = exp(-r2 * 30.0), halo = exp(-r2 * 6.0);',
  '  float ray = exp(-abs(p.x) * 30.0) * exp(-abs(p.y) * 3.0) + exp(-abs(p.y) * 30.0) * exp(-abs(p.x) * 3.0);',
  '  vec2 d = vec2(p.x + p.y, p.x - p.y) * 0.7071; float ray2 = 0.45 * (exp(-abs(d.x) * 36.0) * exp(-abs(d.y) * 4.5) + exp(-abs(d.y) * 36.0) * exp(-abs(d.x) * 4.5));',
  '  float edge = 1.0 - smoothstep(0.62, 1.0, max(abs(p.x), abs(p.y)));',
  '  float f = (core * 1.5 + ray + ray2 + halo * 0.4) * vI.x * edge;',
  '  float a = clamp(f * 0.62, 0.0, 0.9);',
  '  vec3 col = mix(uAmber, uHot, clamp(core * 1.4 + (ray + ray2) * 0.5, 0.0, 1.0)) * a * 1.2;',
  '  gl_FragColor = vec4(col, a);',
  '  #include <colorspace_fragment>',
  '}'
].join('\n');
function hoardGlints(st, n, front) {
  var T = st.T, pos = new Float32Array(n * 3), sz = new Float32Array(n), it = new Float32Array(n * 2), geo = new T.BufferGeometry();
  var pa = new T.BufferAttribute(pos, 3), sa = new T.BufferAttribute(sz, 1), ia = new T.BufferAttribute(it, 2);
  pa.setUsage(T.DynamicDrawUsage); sa.setUsage(T.DynamicDrawUsage); ia.setUsage(T.DynamicDrawUsage);
  geo.setAttribute('position', pa); geo.setAttribute('aS', sa); geo.setAttribute('aI', ia);
  var mat = new T.ShaderMaterial({ vertexShader: HOARD_GLINT_VS, fragmentShader: HOARD_GLINT_FS, transparent: true, depthWrite: false, depthTest: !front, toneMapped: false,
    blending: T.CustomBlending, blendSrc: T.OneFactor, blendDst: T.OneMinusSrcAlphaFactor, blendSrcAlpha: T.OneFactor, blendDstAlpha: T.OneMinusSrcAlphaFactor,
    uniforms: { uPx: { value: st.H * st.renderer.getPixelRatio() / (2 * TAN) }, uHot: { value: new T.Color('#fff6d6') }, uAmber: { value: new T.Color('#ffa21f') } } });
  var pts = new T.Points(geo, mat); pts.frustumCulled = false; pts.renderOrder = front ? 66 : 12;
  return { mesh: pts, set: function (i, x, y, z, size, inten, rot) { pos[i * 3] = x; pos[i * 3 + 1] = y; pos[i * 3 + 2] = z; sz[i] = size; it[i * 2] = inten; it[i * 2 + 1] = rot; },
    commit: function () { pa.needsUpdate = sa.needsUpdate = ia.needsUpdate = true; } };
}

// The warm light thrown on the page by the hoard: a soft pool, premultiplied so it tints a white page
// amber as well as lighting a dark one.
var HOARD_POOL_FS = [
  'uniform float uR, uGain, uCover, uTime; uniform vec3 uCol; uniform vec3 uZ[4]; varying vec2 vP; varying vec2 vW;',
  'void main(){',
  '  float r = length(vP) / uR, an = atan(vP.y, vP.x);',
  '  float f = exp(-r * r * 2.2) * (0.86 + 0.14 * sin(uTime * 7.0 + an * 3.0) * sin(uTime * 4.3 - an * 2.0));',
  // No light over the dice to keep in view: the pool fades out before it reaches each one's disc.
  '  for (int i = 0; i < 4; i++) f *= uZ[i].z > 0.0 ? smoothstep(uZ[i].z * 0.85, uZ[i].z * 1.6, length(vW - uZ[i].xy)) : 1.0;',
  '  gl_FragColor = vec4(uCol * f * uGain, f * uCover);',
  '  #include <colorspace_fragment>',
  '}'
].join('\n');
function hoardPool(st, at, R, zones) {
  var T = st.T, zv = [0, 1, 2, 3].map(function (i) { var z = zones && zones[i]; return new T.Vector3(z ? z.x : 0, z ? z.z : 0, z ? z.r : 0); });
  var mat = new T.ShaderMaterial({ vertexShader: 'varying vec2 vP; varying vec2 vW; void main(){ vP = position.xy; vec4 w = modelMatrix * vec4(position, 1.0); vW = w.xz; gl_Position = projectionMatrix * viewMatrix * w; }',
    fragmentShader: HOARD_POOL_FS, transparent: true, depthWrite: false, toneMapped: false,
    blending: T.CustomBlending, blendSrc: T.OneFactor, blendDst: T.OneMinusSrcAlphaFactor, blendSrcAlpha: T.OneFactor, blendDstAlpha: T.OneMinusSrcAlphaFactor,
    uniforms: { uR: { value: R }, uGain: { value: 0 }, uCover: { value: 0 }, uTime: { value: 0 }, uCol: { value: new T.Color(HOARD.amber) }, uZ: { value: zv } } });
  var m = new T.Mesh(new T.PlaneGeometry(R * 4, R * 4), mat);
  m.rotation.x = -Math.PI / 2; m.position.set(at.x, 0.012, at.z); m.renderOrder = 1; m.frustumCulled = false;
  return m;
}

// A thin ring of light running out over the page: a bright crest with a soft fall-off behind it and
// nothing inside, so it never fills the page or lights the dice it has not reached. Premultiplied, so
// it shows gold on a white page as well as a dark one.
var HOARD_RING_FS = [
  'uniform float uA; uniform vec3 uCol; varying vec2 vP;',
  'void main(){',
  '  float r = length(vP), f = smoothstep(0.8, 0.975, r); f *= f * (1.0 - smoothstep(0.976, 1.0, r));',
  '  vec3 c = mix(uCol, vec3(1.0, 0.74, 0.26), smoothstep(0.9, 0.97, r));',
  '  gl_FragColor = vec4(c * f * uA * 0.8, f * uA * 0.95);',
  '  #include <colorspace_fragment>',
  '}'
].join('\n');
function hoardRing(st, at, r0, r1, dur, col, amp, linear) {
  var T = st.T, mat = new T.ShaderMaterial({ vertexShader: 'varying vec2 vP; void main(){ vP = position.xy; gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0); }',
    fragmentShader: HOARD_RING_FS, transparent: true, depthWrite: false, toneMapped: false,
    blending: T.CustomBlending, blendSrc: T.OneFactor, blendDst: T.OneMinusSrcAlphaFactor, blendSrcAlpha: T.OneFactor, blendDstAlpha: T.OneMinusSrcAlphaFactor,
    uniforms: { uA: { value: 0 }, uCol: { value: new T.Color(col) } } });
  var m = new T.Mesh(new T.PlaneGeometry(2, 2), mat), t = 0;
  m.rotation.x = -Math.PI / 2; m.position.set(at.x, 0.03, at.z); m.frustumCulled = false; m.renderOrder = 2; st.scene.add(m);
  return { update: function (dt) {
    t += dt; var k = Math.min(1, t / dur), R = r0 + (r1 - r0) * (linear ? k : out3(k));
    m.scale.set(R, R, 1); mat.uniforms.uA.value = amp * smooth(0, 0.1, k) * Math.pow(1 - k, 1.4);
    if (k >= 1) { st.scene.remove(m); m.geometry.dispose(); mat.dispose(); return false; }
    return true;
  } };
}

// ---------- The hoard: pieces, their meshes and their glints ----------
// o: { s, hd, die (position of the die), top (y of its top face), zones, front (draw above the
// rest, for an echo), seed, nCoin, nGem, half, onHit }. Bodies are made with addCoin/addGem and
// launched at their own times; update(t, dt) runs everything.
function hoardSystem(st, o) {
  var T = st.T, s = o.s, hd = o.hd, rnd = mulberry(o.seed), sys = { bodies: [], objects: [] };
  var cm = o.nCoin ? hoardCoinMaps(T) : null, i;
  var key = st.sun.position.clone().normalize(), cam = st.cam.position;
  // ----- the coin mesh
  var coinGeo = o.nCoin ? hoardCoinGeo(T, hd) : null, coinMat = null, coinMesh = null;
  if (o.nCoin) {
    coinMat = new T.MeshStandardMaterial({ color: HOARD.body, metalness: 1, roughness: 1, roughnessMap: cm.rough, normalMap: cm.normal, normalScale: new T.Vector2(1.2, 1.2), envMapIntensity: 1.7 });
    coinMesh = new T.InstancedMesh(coinGeo, coinMat, o.nCoin);
    coinMesh.castShadow = coinMesh.receiveShadow = true; coinMesh.layers.enable(1); coinMesh.frustumCulled = false; coinMesh.renderOrder = o.front ? 60 : 4;
    coinMesh.instanceMatrix.setUsage(T.DynamicDrawUsage);
    var tones = ['#ffc23a', '#f2b02e', '#ffd25c', '#e9a42a', '#ffcc48'], col = new T.Color();
    for (i = 0; i < o.nCoin; i++) coinMesh.setColorAt(i, col.set(tones[Math.floor(rnd() * tones.length)]));
    sys.objects.push(coinMesh);
  }
  // ----- the gem meshes, one per cut
  var gemMeshes = [], cuts = HOARD_CUTS, fire = hd && st.q.gems !== false, gemSets = cuts.map(function () { return []; });
  for (i = 0; i < o.nGem; i++) { var wsum = 0; cuts.forEach(function (c) { wsum += c.w; }); var pick = rnd() * wsum, ci = 0; while (pick > cuts[ci].w && ci < cuts.length - 1) { pick -= cuts[ci].w; ci++; } gemSets[ci].push(i); }
  cuts.forEach(function (cut, k) {
    var geo = hoardCutGeo(T, cut), count = gemSets[k].length;
    if (!count) { geo.dispose(); gemMeshes.push(null); return; }
    var mat = hoardGemMat(T, cut, st.env3 && st.env3.cube, fire), mesh = new T.InstancedMesh(geo, mat, count);
    mesh.castShadow = true; mesh.layers.enable(1); mesh.frustumCulled = false; mesh.renderOrder = o.front ? 60 : 4; mesh.instanceMatrix.setUsage(T.DynamicDrawUsage);
    gemMeshes.push(mesh); sys.objects.push(mesh);
  });
  var NE = hd ? 90 : 50, nP = o.nCoin + o.nGem, glints = hoardGlints(st, nP + NE, o.front), ev = [], evNext = 0;
  for (i = 0; i < NE; i++) ev.push({ x: 0, y: 0, z: 0, vx: 0, vy: 0, vz: 0, age: 9, life: 1, size: 0, inten: 0, rot: 0 });
  sys.objects.push(glints.mesh);

  var world = hoardWorld({ g: 32 * s, s: s, cx: o.die.x, cz: o.die.z, half: o.half, cs: 0.3 * s, die: o.dieBall, zones: o.zones,
    box: { x0: -st.hw + 0.7 * s, x1: st.hw - 0.7 * s, z0: -st.hh + 0.7 * s, z1: st.hh - 0.7 * s },
    rest: function (b) { return b.right && flipStart(b); },
    hit: function (b, v) { if (o.onHit) o.onHit(b, v); } });
  sys.world = world;

  // A short-lived spark: a glint that drifts and dies.
  sys.spark = function (x, y, z, vx, vy, vz, size, inten, life) {
    var e = ev[evNext]; evNext = (evNext + 1) % NE;
    e.x = x; e.y = y; e.z = z; e.vx = vx; e.vy = vy; e.vz = vz; e.size = size; e.inten = inten; e.life = life; e.age = 0; e.rot = rnd() * 6.28;
  };

  // ----- bodies
  var coinPts = hoardCoinPts(), nCoin = 0, gemCount = cuts.map(function () { return 0; }), gemOf = [];
  gemSets.forEach(function (l, k) { l.forEach(function (idx) { gemOf[idx] = k; }); });
  function common(b, spec) {
    b.x = spec.x; b.y = spec.y; b.z = spec.z; b.vx = spec.vx || 0; b.vy = spec.vy || 0; b.vz = spec.vz || 0;
    b.wx = spec.wx || 0; b.wy = spec.wy || 0; b.wz = spec.wz || 0;
    var q = spec.q || [rnd() - 0.5, rnd() - 0.5, rnd() - 0.5, rnd() - 0.5], ql = Math.sqrt(q[0] * q[0] + q[1] * q[1] + q[2] * q[2] + q[3] * q[3]) || 1;
    b.qx = q[0] / ql; b.qy = q[1] / ql; b.qz = q[2] / ql; b.qw = q[3] / ql;
    b.tL = spec.tL; b.tV = spec.tV == null ? 99 : spec.tV; b.grace = spec.grace == null ? 0.3 : spec.grace; b.age = 0; b.touch = 0; b.still = 0; b.mode = 0; b.settle = spec.settle || 0.3;
    b.gl = sys.bodies.length; b.seed = rnd(); b.shrink = 1; b.spin = spec.spin || 0;
    sys.bodies.push(b); return b;
  }
  sys.addCoin = function (spec) {
    var r = spec.r, pts = new Float32Array(coinPts.length), b = { kind: 0, mesh: 0, idx: nCoin++, rad: r, np: coinPts.length / 3, pts: pts, sx: r, sy: r, sz: r };
    for (var k = 0; k < pts.length; k++) pts[k] = coinPts[k] * r;
    b.iP = 1 / (r * r * 0.25 + Math.pow(2 * HOARD_HH * r, 2) / 12); b.iA = 2 / (r * r); b.lam = (b.iP / b.iA) - 1;
    b.e = 0.27; b.mu = 0.6; b.vthr = 2.2 * s;
    return common(b, spec);
  };
  sys.addGem = function (spec, gi) {
    var k = gemOf[gi], cut = cuts[k], r = spec.r, pts = new Float32Array(cut.pts.length), b = { kind: 1, mesh: k, idx: gemCount[k]++, rad: r, np: cut.pts.length / 3, pts: pts, sx: r, sy: r, sz: r, cut: cut };
    for (var j = 0; j < pts.length; j += 3) { pts[j] = cut.pts[j] * r; pts[j + 1] = cut.pts[j + 1] * r; pts[j + 2] = cut.pts[j + 2] * r; }
    b.iP = b.iA = 1 / (0.4 * r * r); b.lam = 0; b.e = 0.25; b.mu = 0.55; b.vthr = 2.4 * s;
    b.fac = [cut.facets[Math.floor(rnd() * cut.facets.length)], cut.facets[Math.floor(rnd() * cut.facets.length)], cut.pavFacets[Math.floor(rnd() * cut.pavFacets.length)], cut.pavFacets[Math.floor(rnd() * cut.pavFacets.length)]];
    b.right = true;
    return common(b, spec);
  };
  sys.gemKind = function (gi) { return gemOf[gi]; };

  // ----- a spinning coin settling like Euler's disk -----
  // It stands on one point of its rim while its tilt falls away and the wobble round the vertical
  // speeds up until the coin clatters flat: the tilt goes as the cube root of the time left, the
  // wobble rate as its inverse square root. A real contact would take far too long to show this.
  function spinStart(b) {
    hoardRot(b, 0, 1, 0); var ax = HOARD_R[0], ay = HOARD_R[1], az = HOARD_R[2];
    if (ay < 0.25) { b.spin = 0; return; }
    var al = Math.max(Math.acos(Math.min(1, ay)), 0.5), ph = Math.atan2(az, ax), r = b.rad * 0.97, hh = HOARD_HH * b.rad;
    var sa = Math.sin(al), ca = Math.cos(al), ux = -ca * Math.cos(ph), uy = sa, uz = -ca * Math.sin(ph);
    b.spA = al; b.spPh = ph; b.spT = 0; b.spTf = b.spin; b.spW = (rnd() < 0.5 ? -1 : 1) * (6.5 + 3 * rnd()); b.spPsi = rnd() * 6.2832;
    b.spVx = b.vx; b.spVz = b.vz;
    b.spPx = b.x - r * ux - hh * Math.sin(al) * Math.cos(ph); b.spPz = b.z - r * uz - hh * Math.sin(al) * Math.sin(ph);
    b.spG = world.hf(b.spPx, b.spPz); b.spY = b.y;
    b.mode = 4; b.vx = b.vy = b.vz = b.wx = b.wy = b.wz = 0;
    if (o.onHit) o.onHit(b, s * 9);
  }
  function spinStep(b, dt) {
    b.spT += dt; b.age += dt;
    var Tf = b.spTf, sl = Math.max(0, 1 - b.spT / Tf), r = b.rad * 0.97, hh = HOARD_HH * b.rad, al, ph;
    var done = sl < 0.03;
    al = done ? 0.0 : b.spA * Math.pow(sl, 0.4);
    ph = b.spPh + b.spW * 2 * Tf * (1 - Math.sqrt(Math.max(sl, 0.03)));
    var sa = Math.sin(al), ca = Math.cos(al), ux = -ca * Math.cos(ph), uy = sa, uz = -ca * Math.sin(ph);
    var ex = Math.exp(-4 * b.spT);
    b.spPx += b.spVx * dt * ex * 0.5; b.spPz += b.spVz * dt * ex * 0.5;
    b.x = b.spPx + r * ux + hh * sa * Math.cos(ph); b.y = b.spY + (b.spG + r * uy + hh * ca - b.spY) * smooth(0, 0.07, b.spT); b.z = b.spPz + r * uz + hh * sa * Math.sin(ph);
    var psi = b.spPsi - ph * ca, hs = Math.sin(al / 2), hc = Math.cos(al / 2), Ax = Math.sin(ph) * hs, Az = -Math.cos(ph) * hs, sp = Math.sin(psi / 2), cp = Math.cos(psi / 2);
    b.qx = Ax * cp - Az * sp; b.qy = hc * sp; b.qz = Ax * sp + Az * cp; b.qw = hc * cp;
    if (done) { b.mode = 2; b.vx = b.vy = b.vz = 0; world.deposit(b); if (o.onHit) o.onHit(b, s * 7); }
  }

  // ----- a stone turning over -----
  // One that comes to rest pavilion-up rolls over onto the cone of its pavilion, leaning with its table
  // to the sky, so every stone on the heap shows its crown. It is a short scripted roll with a small
  // hop; the stone is put down on whatever is under it when it ends.
  function flipStart(b) {
    if (b.flipped) return false;
    b.flipped = true;
    hoardRot(b, 0, 1, 0); var ax = HOARD_R[0], ay = HOARD_R[1], az = HOARD_R[2];
    if (ay > 0.2) return false;
    var hl = Math.sqrt(ax * ax + az * az), ph = hl > 1e-3 ? Math.atan2(az, ax) : rnd() * 6.2832, al = 0.85;
    var dx = Math.sin(al) * Math.cos(ph), dy = Math.cos(al), dz = Math.sin(al) * Math.sin(ph);
    // The turn that takes the stone's axis to the leaning one, laid over its present orientation.
    var rx = ay * dz - az * dy, ry = az * dx - ax * dz, rz = ax * dy - ay * dx, rw = 1 + ax * dx + ay * dy + az * dz, rl = Math.sqrt(rx * rx + ry * ry + rz * rz + rw * rw) || 1;
    rx /= rl; ry /= rl; rz /= rl; rw /= rl;
    b.fq = [b.qx, b.qy, b.qz, b.qw];
    b.ft = [rw * b.qx + rx * b.qw + ry * b.qz - rz * b.qy, rw * b.qy - rx * b.qz + ry * b.qw + rz * b.qx, rw * b.qz + rx * b.qy - ry * b.qx + rz * b.qw, rw * b.qw - rx * b.qx - ry * b.qy - rz * b.qz];
    if (b.fq[0] * b.ft[0] + b.fq[1] * b.ft[1] + b.fq[2] * b.ft[2] + b.fq[3] * b.ft[3] < 0) b.ft = [-b.ft[0], -b.ft[1], -b.ft[2], -b.ft[3]];
    b.fT = 0; b.mode = 5; b.vx = b.vy = b.vz = b.wx = b.wy = b.wz = 0;
    return true;
  }
  function flipStep(b, dt) {
    b.fT += dt; b.age += dt;
    var k = clamp01(b.fT / 0.32), e = k * k * (3 - 2 * k), q = b.fq, t2 = b.ft, ql;
    b.qx = q[0] + (t2[0] - q[0]) * e; b.qy = q[1] + (t2[1] - q[1]) * e; b.qz = q[2] + (t2[2] - q[2]) * e; b.qw = q[3] + (t2[3] - q[3]) * e;
    ql = 1 / Math.sqrt(b.qx * b.qx + b.qy * b.qy + b.qz * b.qz + b.qw * b.qw); b.qx *= ql; b.qy *= ql; b.qz *= ql; b.qw *= ql;
    b.y = world.groundY(b) + b.rad * 0.9 * Math.sin(Math.PI * k);
    if (k >= 1) { b.mode = 2; b.y = world.groundY(b); world.deposit(b); if (o.onHit) o.onHit(b, s * 4); }
  }

  var CM = coinMesh ? coinMesh.instanceMatrix.array : null;
  function put(arr, i, b, f) {
    var qx = b.qx, qy = b.qy, qz = b.qz, qw = b.qw, xx = qx * qx, yy = qy * qy, zz = qz * qz, xy = qx * qy, xz = qx * qz, yz = qy * qz, wx = qw * qx, wy = qw * qy, wz = qw * qz;
    var sx = b.sx * f, sy = b.sy * f, sz = b.sz * f, m = i * 16;
    arr[m] = (1 - 2 * (yy + zz)) * sx; arr[m + 1] = 2 * (xy + wz) * sx; arr[m + 2] = 2 * (xz - wy) * sx; arr[m + 3] = 0;
    arr[m + 4] = 2 * (xy - wz) * sy; arr[m + 5] = (1 - 2 * (xx + zz)) * sy; arr[m + 6] = 2 * (yz + wx) * sy; arr[m + 7] = 0;
    arr[m + 8] = 2 * (xz + wy) * sz; arr[m + 9] = 2 * (yz - wx) * sz; arr[m + 10] = (1 - 2 * (xx + yy)) * sz; arr[m + 11] = 0;
    arr[m + 12] = b.x; arr[m + 13] = b.y; arr[m + 14] = b.z; arr[m + 15] = 1;
  }
  // Strength of the glint a piece throws right now: a face of it mirrors the key light toward the camera.
  function mirror(b) {
    var vx = cam.x - b.x, vy = cam.y - b.y, vz = cam.z - b.z, vl = Math.sqrt(vx * vx + vy * vy + vz * vz) || 1;
    var hx = key.x + vx / vl, hy = key.y + vy / vl, hz = key.z + vz / vl, hl = Math.sqrt(hx * hx + hy * hy + hz * hz) || 1, best = 0, c, k;
    hx /= hl; hy /= hl; hz /= hl;
    if (b.kind === 0) {
      hoardRot(b, 0, 1, 0); c = Math.abs(HOARD_R[0] * hx + HOARD_R[1] * hy + HOARD_R[2] * hz);
      return clamp01((c - 0.962) / 0.034);
    }
    for (k = 0; k < b.fac.length; k++) { var f = b.fac[k]; hoardRot(b, f[0], f[1], f[2]); c = HOARD_R[0] * hx + HOARD_R[1] * hy + HOARD_R[2] * hz; best = Math.max(best, clamp01((c - 0.95) / 0.045)); }
    return best;
  }
  var SIZE = s * 1.0;
  // t: time since the effect began; ex(b, t): extra glint strength from the effect (twinkle, sweep); dt as given.
  sys.update = function (t, dt, ex) {
    var bs = sys.bodies, b, k;
    for (k = 0; k < bs.length; k++) {
      b = bs[k];
      if (b.mode === 0 && t >= b.tL) { b.mode = 1; b.age = 0; if (o.onLaunch) o.onLaunch(b); }
    }
    world.run(bs, Math.min(dt, 0.05));
    for (k = 0; k < bs.length; k++) {
      b = bs[k];
      if (b.spin && b.mode === 1 && b.vy < 0 && b.age > 0.3 && b.y - world.hf(b.x, b.z) < b.rad * 0.95) spinStart(b);
      if (b.mode === 4) spinStep(b, Math.min(dt, 0.05));
      if (b.mode === 5) flipStep(b, Math.min(dt, 0.05));
    }
    for (k = 0; k < bs.length; k++) {
      b = bs[k];
      var arr = b.kind === 0 ? CM : gemMeshes[b.mesh].instanceMatrix.array, f = 0;
      if (b.mode === 1 || b.mode === 2 || b.mode === 4 || b.mode === 5) {
        f = clamp01(b.age / 0.14 + 0.12);
        if (t > b.tV) { var vk = (t - b.tV) / 0.2; if (vk >= 1) { b.mode = 3; f = 0; } else f *= 1 - vk * vk * vk; }
      }
      put(arr, b.idx, b, f);
      var gi = 0, size = 0;
      if (f > 0) {
        gi = mirror(b) * (b.kind === 1 ? 1.15 : 0.9);
        if (ex) gi = Math.max(gi, ex(b, t));
        gi *= f;
        size = SIZE * (b.kind === 1 ? 0.8 : 0.62) * (0.5 + 0.9 * gi) * (1 + 0.15 * Math.sin(t * 40 + b.seed * 30));
      }
      glints.set(b.gl, b.x, b.y + b.rad * 0.6 + 0.04, b.z, size, gi, b.seed * 6.28);
    }
    for (k = 0; k < NE; k++) {
      var e = ev[k], n = nP + k;
      if (e.age >= e.life) { glints.set(n, 0, -5, 0, 0, 0, 0); continue; }
      e.age += dt; e.x += e.vx * dt; e.y += e.vy * dt; e.z += e.vz * dt; e.vx *= 1 - 1.6 * dt; e.vz *= 1 - 1.6 * dt; e.vy -= s * 4 * dt;
      var q = e.age / e.life, inten = e.inten * (q < 0.12 ? q / 0.12 : Math.pow(1 - (q - 0.12) / 0.88, 1.6));
      glints.set(n, e.x, Math.max(e.y, 0.05), e.z, e.size * (0.7 + 0.5 * (1 - q)), inten, e.rot + q * 1.5);
    }
    glints.commit();
    if (CM) coinMesh.instanceMatrix.needsUpdate = true;
    gemMeshes.forEach(function (m) { if (m) m.instanceMatrix.needsUpdate = true; });
  };
  sys.awake = function () { for (var k = 0; k < sys.bodies.length; k++) if (sys.bodies[k].mode < 2 || sys.bodies[k].mode >= 4 && sys.bodies[k].mode < 6) return true; return false; };
  sys.addTo = function (scene) { sys.objects.forEach(function (m) { scene.add(m); }); };
  sys.dispose = function () {
    sys.objects.forEach(function (m) { if (m.parent) m.parent.remove(m); if (m.isInstancedMesh) { m.dispose(); if (m.geometry && !m.geometry.userData.shared) m.geometry.dispose(); if (m.material && !m.material.userData.shared && m.material !== coinMat) m.material.dispose(); } else { m.geometry.dispose(); m.material.dispose(); } });
    if (coinMat) coinMat.dispose();
  };
  return sys;
}

// ---------- The effect ----------
// Dice to keep clear: the ones the throw lists, and any other die on the table.
function hoardZones(st, die, clear) {
  var zs = (clear || []).map(function (c) { return { x: c.x, z: c.z, r: c.r }; });
  st.dice.forEach(function (d) {
    if (d === die) return;
    var p = d.group.position, dup = false;
    zs.forEach(function (c) { if (Math.hypot(c.x - p.x, c.z - p.z) < 0.5) dup = true; });
    if (!dup) zs.push({ x: p.x, z: p.z, r: d.size * 1.9 });
  });
  return zs;
}
// A spot on the page for a piece to land: not near a die to keep clear, nor on a path that crosses one
// (with room for it to roll on), and inside the page.
function hoardTarget(st, s, zones, rnd, cx, cz, sx, sz, rMin, rMax, bias) {
  function ok(tx, tz) {
    if (Math.abs(tx) > st.hw - 1.1 * s || Math.abs(tz) > st.hh - 1.1 * s) return false;
    for (var i = 0; i < zones.length; i++) {
      var z = zones[i];
      if (Math.hypot(tx - z.x, tz - z.z) < z.r + 1.4 * s) return false;
      for (var f = 0.25; f < 1.3; f += 0.25) if (Math.hypot(sx + (tx - sx) * f - z.x, sz + (tz - sz) * f - z.z) < z.r + 1.25 * s) return false;
    }
    return true;
  }
  var tr, a, R, x, z;
  for (tr = 0; tr < 28; tr++) {
    a = rnd() * 6.2832; R = s * (rMin + (rMax - rMin) * Math.pow(rnd(), bias)); x = cx + Math.cos(a) * R; z = cz + Math.sin(a) * R;
    if (ok(x, z)) return [x, z];
  }
  // Crowded: land close in, on the side away from the nearest die.
  var near = null, nd = 1e9;
  zones.forEach(function (q) { var d = Math.hypot(q.x - cx, q.z - cz); if (d < nd) { nd = d; near = q; } });
  a = near ? Math.atan2(cz - near.z, cx - near.x) + (rnd() - 0.5) * 1.6 : rnd() * 6.2832; R = s * (rMin + 0.4 * rnd());
  return [cx + Math.cos(a) * R, cz + Math.sin(a) * R];
}
// A random orientation of a coin leaning tilt radians from flat, as a quaternion [x, y, z, w].
function hoardTilted(rnd, tilt) {
  var yaw = rnd() * 6.2832, az = rnd() * 6.2832, sa = Math.sin(tilt / 2), ca = Math.cos(tilt / 2), sy = Math.sin(yaw / 2), cy = Math.cos(yaw / 2);
  var tx = Math.cos(az) * sa, tz = Math.sin(az) * sa;
  // tilt quaternion (tx, 0, tz, ca) times yaw quaternion (0, sy, 0, cy)
  return [tx * cy - tz * sy, ca * sy, tx * sy + tz * cy, ca * cy];
}

function fxHoard(st, die, fi, th, o) {
  var T = st.T, g = die.group, s = die.size, hd = !(st.q && st.q.detail === 'basic'), t = 0, rnd = mulberry(8101 + Math.floor(Math.random() * 1e6));
  var E0 = 0.5, END = 4.6, G = 32 * s;
  g.updateMatrixWorld(true);
  var p0 = g.position.clone(), q0 = g.quaternion.clone(), at = new T.Vector3(p0.x, 0, p0.z), top = p0.y + die.rest;
  // Light adds, so on a pale stone a pale gold number washes out: there it burns a deeper amber.
  var NUM = isLight(th) ? HOARD.numDeep : HOARD.num;
  var zones = hoardZones(st, die, o && o.clear), flare = 0;

  // ----- the hoard, planned up front: where every piece starts, flies and lands -----
  var sys = hoardSystem(st, { s: s, hd: hd, seed: 5003 + Math.floor(Math.random() * 1e5), die: p0, dieBall: { x: p0.x, y: p0.y, z: p0.z, r: s * 0.93 }, zones: zones,
    nCoin: hd ? 112 : 56, nGem: hd ? 32 : 16, half: 7.5 * s,
    onHit: function (b, v) {
      if (b.kind === 1 ? v < s * 4 : v < s * 6 || rnd() < 0.4) return;
      sys.spark(b.x, b.y + b.rad * 0.4, b.z, (rnd() - 0.5) * s * 2, s * (0.8 + rnd() * 1.6), (rnd() - 0.5) * s * 2, s * (0.4 + 0.4 * Math.min(1, v / (s * 14))), 0.9, 0.2 + 0.15 * rnd());
    },
    onLaunch: function (b) { sys.spark(b.x, b.y, b.z, 0, s * 1.5, 0, s * (b.kind ? 0.8 : 0.55), 0.95, 0.22 + 0.1 * rnd()); } });
  function aimed(spec, r, tL, Tf, rMin, rMax, bias) {
    var a = rnd() * 6.2832, rr = s * 0.42 * Math.sqrt(rnd()), sx = p0.x + Math.cos(a) * rr, sz = p0.z + Math.sin(a) * rr, sy = top + s * 0.28;
    var tg = hoardTarget(st, s, zones, rnd, p0.x, p0.z, sx, sz, rMin, rMax, bias);
    spec.x = sx; spec.y = sy; spec.z = sz; spec.r = r; spec.tL = tL;
    spec.vx = (tg[0] - sx) / Tf; spec.vz = (tg[1] - sz) / Tf; spec.vy = (0.5 * G * Tf * Tf - (sy - r * 0.2)) / Tf;
  }
  var i, nC = hd ? 112 : 56, nG = hd ? 32 : 16;
  var nSp = hd ? 9 : 5;
  for (i = 0; i < nC; i++) {
    var burst = i < nC * 0.45, spec = {}, r = s * (0.38 + rnd() * 0.06), spinner = i < nSp;
    // A spinner flies with its face turned up, leaning a little and spinning fast about its own axis; it
    // lands in the open ring and spins down like Euler's disk. The rest tumble end over end.
    aimed(spec, r, burst ? E0 + rnd() * 0.12 : E0 + 0.06 + 0.85 * Math.pow(rnd(), 0.85), burst ? 0.95 + rnd() * 0.4 : 0.8 + rnd() * 0.4, spinner ? 2.5 : 1.55, spinner ? 4.2 : 4.6, spinner ? 1 : 1.5);
    if (spinner) {
      spec.q = hoardTilted(rnd, 0.3 + rnd() * 0.5); spec.spin = 1.0 + 0.3 * rnd(); spec.tL = E0 + 0.02 + 0.1 * rnd();
      var ax = 2 * (spec.q[0] * spec.q[1] - spec.q[3] * spec.q[2]), ay = 1 - 2 * (spec.q[0] * spec.q[0] + spec.q[2] * spec.q[2]), az = 2 * (spec.q[1] * spec.q[2] + spec.q[3] * spec.q[0]), sw = (rnd() < 0.5 ? -1 : 1) * (26 + 16 * rnd());
      spec.wx = ax * sw; spec.wy = ay * sw; spec.wz = az * sw;
    } else {
      var wa = rnd() * 6.2832, wb = rnd() * 2 - 1, wm = 8 + rnd() * 17, wr = Math.sqrt(1 - wb * wb);
      spec.wx = Math.cos(wa) * wr * wm; spec.wy = wb * wm; spec.wz = Math.sin(wa) * wr * wm;
    }
    sys.addCoin(spec);
  }
  for (i = 0; i < nG; i++) {
    var gs = {}, gr = s * (0.25 + rnd() * 0.09), ga = rnd() * 6.2832, gb = rnd() * 2 - 1, gm = 6 + rnd() * 10, gq = Math.sqrt(1 - gb * gb);
    aimed(gs, gr, E0 + 0.03 + 0.9 * rnd(), 0.9 + rnd() * 0.45, 1.55, 4.2, 1.3);
    if (i % 4 !== 3) {
      // Table-up and spinning about its own axis, which holds it that way through the flight.
      gs.q = hoardTilted(rnd, 0.15 + rnd() * 0.45);
      var gax = 2 * (gs.q[0] * gs.q[1] - gs.q[3] * gs.q[2]), gay = 1 - 2 * (gs.q[0] * gs.q[0] + gs.q[2] * gs.q[2]), gaz = 2 * (gs.q[1] * gs.q[2] + gs.q[3] * gs.q[0]), gsw = (rnd() < 0.5 ? -1 : 1) * (9 + 11 * rnd());
      gs.wx = gax * gsw; gs.wy = gay * gsw; gs.wz = gaz * gsw;
    } else { gs.wx = Math.cos(ga) * gq * gm; gs.wy = gb * gm; gs.wz = Math.sin(ga) * gq * gm; }
    sys.addGem(gs, i);
  }

  // ----- the light: a warm pool on the page, a halo and a star over the die, one light in HD -----
  var dim = dimmer(st, at, s * 6.6), pool = hoardPool(st, at, s * 2.5, zones.slice().sort(function (a, b) { return Math.hypot(a.x - at.x, a.z - at.z) - Math.hypot(b.x - at.x, b.z - at.z); }));
  dim.material.color.set('#241505');
  var halo = glow(st, spriteTex(T), HOARD.gold, s * 4.2), star = glow(st, starTex(T), HOARD.pale, s * 8), star2 = glow(st, starTex(T), HOARD.gold, s * 6);
  halo.position.set(p0.x, p0.y + s * 0.4, p0.z); star.position.set(p0.x, top + s * 0.6, p0.z); star2.position.copy(star.position);
  halo.renderOrder = 6; star.renderOrder = 9; star2.renderOrder = 9;
  var lamp = hd ? new T.PointLight(0xffb040, 0, s * 8, 1.6) : null;
  if (lamp) { lamp.position.set(p0.x, top + s * 2.6, p0.z); st.scene.add(lamp); }
  st.scene.add(dim, pool, halo, star, star2); sys.addTo(st.scene);
  var fc = glowLayer(die, fi); glowNumeral(die, fi, NUM); fc.mat.emissiveIntensity = 0;
  var done = {}, acc = 0, vset = false;

  // Extra glint on a piece: a settled piece twinkling, and the ripple of light that runs out from the die.
  function extra(b, tt) {
    var gl = 0, d = Math.hypot(b.x - p0.x, b.z - p0.z);
    if (b.mode === 2 && tt > 2.2) {
      var tw = Math.pow(0.5 + 0.5 * Math.sin(tt * (3.5 + 5 * b.seed) + b.seed * 61), b.kind ? 10 : 20);
      gl = tw * (b.kind ? 1.1 : 0.85) * smooth(2.2, 3.1, tt) * (1 - smooth(3.4, 3.6, tt));
    }
    if (tt > 3.2 && tt < 4.5) {
      var fr = s * (1.3 + 7.64 * (tt - 3.3)), q = (d - fr) / (0.75 * s);
      gl = Math.max(gl, 1.25 * Math.exp(-q * q));
    }
    return gl;
  }

  return { update: function (dt) {
    t += dt;
    if (t >= END) {
      sys.dispose();
      drop(st, [dim, pool, halo, star, star2, lamp]);
      if (lamp) lamp.dispose();
      g.position.copy(p0); g.quaternion.copy(q0); g.updateMatrixWorld(true);
      shadeDie(die, 1); tintDie(die, th.burn, 0); glowNumeral(die, fi, NUM);
      return false;
    }
    var charge = smooth(0, E0, t), after = t < E0 ? 0 : 1 - smooth(3.5, 4.5, t), erupt = env4(t, E0 - 0.03, E0 + 0.02, E0 + 0.08, E0 + 0.6);

    // Sparks off the rim while it charges, and a stream of them from the crown while the fountain runs.
    var rate = (t < E0 ? 30 * charge : 0) + (t > E0 && t < 1.45 ? (hd ? 70 : 36) : 0);
    acc += rate * dt;
    while (acc >= 1) {
      acc -= 1;
      var a = rnd() * 6.2832;
      if (t < E0) { var rr = s * (0.5 + 0.45 * rnd()); sys.spark(p0.x + Math.cos(a) * rr, p0.y + s * (0.1 + 0.8 * rnd()), p0.z + Math.sin(a) * rr, Math.cos(a) * s * 0.4, s * 1.8, Math.sin(a) * s * 0.4, s * 0.3, 0.9, 0.3 + 0.2 * rnd()); }
      else { var sp = s * (2 + 5 * rnd()); sys.spark(p0.x + Math.cos(a) * s * 0.3, top + s * 0.2, p0.z + Math.sin(a) * s * 0.3, Math.cos(a) * sp, s * (2 + 5 * rnd()), Math.sin(a) * sp, s * (0.28 + 0.3 * rnd()), 1, 0.35 + 0.3 * rnd()); }
    }
    if (!done.erupt && t >= E0) {
      done.erupt = true;
      st.fx.push(hoardRing(st, at, s * 0.8, s * 3.1, 0.55, '#f0a01a', 0.95, false));
      for (var k = Math.round((hd ? 44 : 22)); k > 0; k--) { var ea = rnd() * 6.2832, es = s * (3 + 7 * rnd()); sys.spark(p0.x, top + s * 0.2, p0.z, Math.cos(ea) * es, s * (1 + 6 * rnd()), Math.sin(ea) * es, s * (0.3 + 0.4 * rnd()), 1, 0.4 + 0.4 * rnd()); }
    }
    if (!vset && t >= 3.3) {
      vset = true;
      sys.bodies.forEach(function (b) { b.tV = 3.6 + 0.55 * clamp01((Math.hypot(b.x - p0.x, b.z - p0.z) - 1.3 * s) / (4.2 * s)) + 0.1 * rnd(); });
      st.fx.push(hoardRing(st, at, s * 1.3, s * 4.2, 0.38, '#f0a01a', 0.9, true));
    }
    // Pieces that wink out leave a spark where they were.
    for (var j = 0; j < sys.bodies.length; j++) {
      var b = sys.bodies[j];
      if (vset && !b.popped && t >= b.tV && b.mode < 3) {
        b.popped = true;
        for (var m = 0; m < (hd ? 2 : 1); m++) sys.spark(b.x, b.y + b.rad * 0.5, b.z, (rnd() - 0.5) * s * 1.6, s * (1 + 2 * rnd()), (rnd() - 0.5) * s * 1.6, s * (b.kind ? 0.85 : 0.6), 1, 0.35 + 0.25 * rnd());
      }
    }
    sys.update(t, dt, extra);

    // The die: a shiver as it charges, a hop as it bursts.
    var sh = s * 0.02 * smooth(0.15, E0, t) * (t < E0 + 0.05 ? 1 : 0), hop = s * 0.2 * Math.sin(Math.PI * clamp01((t - E0) / 0.3));
    g.position.set(p0.x + (rnd() - 0.5) * sh * 2, p0.y + hop + rnd() * sh, p0.z + (rnd() - 0.5) * sh * 2);
    g.quaternion.copy(q0);
    flare *= Math.max(0, 1 - 6 * dt);

    // Light: it builds in the die, flashes at the burst and holds while the hoard lies glittering.
    var hold = t < E0 ? charge : after;
    tintDie(die, HOARD.amber, 1.05 * hold + 2.2 * erupt + flare);
    shadeDie(die, 1 - 0.2 * smooth(0.1, E0, t) * (1 - smooth(3.6, 4.4, t)));
    fc.mat.emissiveIntensity = 1.05 * hold + 1.2 * erupt + 0.4 * flare;
    pool.material.uniforms.uTime.value = t;
    pool.material.uniforms.uGain.value = 0.26 * hold + 0.12 * erupt; pool.material.uniforms.uCover.value = 0.16 * hold + 0.04 * erupt;
    pool.visible = hold + erupt > 0.003;
    dim.material.opacity = 0.5 * smooth(0.1, E0, t) * (1 - smooth(3.9, 4.55, t));
    halo.material.opacity = 0.28 * hold + 0.2 * erupt; halo.visible = halo.material.opacity > 0.003;
    var sk = clamp01((t - E0) / 0.4);
    star.material.opacity = t >= E0 ? 0.65 * (1 - sk) * (1 - sk) : 0.25 * charge * charge; star.scale.setScalar(s * (t >= E0 ? 3.6 + 3.4 * out3(sk) : 3.6 * charge)); star.material.rotation = sk * 0.5;
    star2.material.opacity = star.material.opacity * 0.8; star2.scale.setScalar(star.scale.x * 0.62); star2.material.rotation = 0.785 - sk * 0.4;
    star.visible = star.material.opacity > 0.003; star2.visible = hd && star.visible;
    if (lamp) lamp.intensity = (24 * hold + 38 * erupt + 8 * flare) * (0.92 + 0.08 * Math.sin(t * 31));
    return true;
  } };
}

// ---------- The echo ----------
// A few coins. When the grand fountain is on the table they arc across from it and land on this die with
// a clink, then hop off and spin to a stop beside it; alone they pop off this die. They wink out in
// sparks, and the die is left with its number burning gold. Drawn above anything the grand put down.
function fxHoardEcho(st, die, fi, th, e) {
  var T = st.T, g = die.group, s = die.size, hd = !(st.q && st.q.detail === 'basic'), t = 0, rnd = mulberry(9157 + Math.floor(Math.random() * 1e6));
  var END = 1.88, G = 32 * s, n = hd ? 6 : 4;
  g.updateMatrixWorld(true);
  var p0 = g.position.clone(), q0 = g.quaternion.clone(), top = p0.y + die.rest, at = new T.Vector3(p0.x, 0, p0.z);
  var NUM = isLight(th) ? HOARD.numDeep : HOARD.num, flare = 0, zones = hoardZones(st, die, null);
  var from = e && e.from && e.from.group ? e.from.group.position.clone() : null, fromTop = from ? from.y + e.from.rest : 0;
  var sys = hoardSystem(st, { s: s, hd: hd, seed: 777 + Math.floor(Math.random() * 1e5), die: p0, dieBall: { x: p0.x, y: p0.y, z: p0.z, r: s * 0.93 }, zones: zones,
    nCoin: n, nGem: 0, half: 4.2 * s, front: true,
    onHit: function (b, v) { if (v > s * 5 && rnd() < 0.5) sys.spark(b.x, b.y + b.rad * 0.4, b.z, (rnd() - 0.5) * s, s * (0.6 + rnd()), (rnd() - 0.5) * s, s * 0.4, 0.9, 0.2); } });
  var i, hf = HOARD_HH;
  for (i = 0; i < n; i++) {
    var r = s * (0.38 + rnd() * 0.05), spec = { r: r, settle: 0.22, grace: 0.3 }, ca = rnd() * 6.2832, cr = s * 0.4 * Math.sqrt(rnd());
    if (from) {
      // Out of the grand's crown, across to a spot on this die's top face.
      var sx = from.x + Math.cos(ca) * cr, sz = from.z + Math.sin(ca) * cr, sy = fromTop + s * 0.3, tx = p0.x + (rnd() - 0.5) * s * 0.8, tz = p0.z + (rnd() - 0.5) * s * 0.8, ty = top + r * hf + s * 0.02;
      var Tf = Math.min(0.66, Math.max(0.46, 0.42 + Math.hypot(tx - sx, tz - sz) / 46));
      spec.x = sx; spec.y = sy; spec.z = sz; spec.vx = (tx - sx) / Tf; spec.vz = (tz - sz) / Tf; spec.vy = (0.5 * G * Tf * Tf + ty - sy) / Tf;
      spec.tL = 0.05 * i; spec.grace = 9; spec.leg = 1; spec.noZone = true;
    } else {
      var px = p0.x + Math.cos(ca) * cr, pz = p0.z + Math.sin(ca) * cr, py = top + s * 0.1, tg = hoardTarget(st, s, zones, rnd, p0.x, p0.z, px, pz, 2.4, 3.3, 1), Tp = 0.55 + rnd() * 0.15;
      spec.x = px; spec.y = py; spec.z = pz; spec.vx = (tg[0] - px) / Tp; spec.vz = (tg[1] - pz) / Tp; spec.vy = (0.5 * G * Tp * Tp + r * hf - py) / Tp; spec.tL = 0.04 + 0.07 * i; spec.leg = 0;
    }
    // The first coin spins on its face and wobbles to a stop; the rest tumble.
    if (i === 0) {
      spec.q = hoardTilted(rnd, 0.3 + rnd() * 0.3);
      var ax = 2 * (spec.q[0] * spec.q[1] - spec.q[3] * spec.q[2]), ay = 1 - 2 * (spec.q[0] * spec.q[0] + spec.q[2] * spec.q[2]), az = 2 * (spec.q[1] * spec.q[2] + spec.q[3] * spec.q[0]);
      spec.wx = ax * 34; spec.wy = ay * 34; spec.wz = az * 34; spec.spin = 0.8;
    } else {
      var wa = rnd() * 6.2832, wb = rnd() * 2 - 1, wm = 9 + rnd() * 14, wr = Math.sqrt(1 - wb * wb);
      spec.wx = Math.cos(wa) * wr * wm; spec.wy = wb * wm; spec.wz = Math.sin(wa) * wr * wm;
    }
    spec.tV = (from ? 1.32 : 1.18) + 0.055 * i;
    var cb = sys.addCoin(spec); cb.leg = spec.leg; cb.noZone = !!spec.noZone;
  }
  var dim = dimmer(st, at, s * 5), halo = glow(st, spriteTex(T), HOARD.gold, s * 4.2), star = glow(st, starTex(T), HOARD.pale, s * 6);
  dim.material.color.set('#241505'); dim.renderOrder = -2; halo.position.set(p0.x, p0.y + s * 0.4, p0.z); star.position.set(p0.x, top + s * 0.4, p0.z); halo.renderOrder = 62; star.renderOrder = 63;
  st.scene.add(dim, halo, star); sys.addTo(st.scene);
  var fc = glowLayer(die, fi); glowNumeral(die, fi, NUM); fc.mat.emissiveIntensity = 0;
  var pops = {}, starK = 0;

  // A coin from the grand reaches the top face: a clink, and it hops off to a landing beside the die.
  function hopOff(b) {
    b.leg = 2; b.noZone = false; b.grace = 0.6; b.y = top + b.rad * hf;
    var Tf = 0.5, tg = hoardTarget(st, s, zones, rnd, p0.x, p0.z, b.x, b.z, 2.4, 3.2, 1);
    b.vx = (tg[0] - b.x) / Tf; b.vz = (tg[1] - b.z) / Tf; b.vy = (0.5 * G * Tf * Tf + b.rad * hf - b.y) / Tf;
    var wa = rnd() * 6.2832; b.wx = Math.cos(wa) * 16; b.wz = Math.sin(wa) * 16; b.wy = (rnd() - 0.5) * 24;
    flare = Math.min(1.2, flare + 0.55); starK = 1;
    sys.spark(b.x, top + s * 0.1, b.z, 0, s * 0.5, 0, s * 1.1, 1, 0.3);
    for (var k = 0; k < 4; k++) { var ka = rnd() * 6.2832; sys.spark(b.x, top + s * 0.1, b.z, Math.cos(ka) * s * 2.5, s * (1 + 2 * rnd()), Math.sin(ka) * s * 2.5, s * 0.3, 0.9, 0.3); }
  }
  return { update: function (dt) {
    t += dt;
    if (t >= END) {
      sys.dispose(); drop(st, [dim, halo, star]);
      g.position.copy(p0); g.quaternion.copy(q0); g.updateMatrixWorld(true);
      shadeDie(die, 1); tintDie(die, th.burn, 0); glowNumeral(die, fi, NUM);
      return false;
    }
    var bs = sys.bodies, j, b;
    for (j = 0; j < bs.length; j++) {
      b = bs[j];
      if (b.mode === 1 && b.leg === 1 && b.vy < 0 && b.y - b.rad * hf <= top + 0.02 && Math.hypot(b.x - p0.x, b.z - p0.z) < 1.1 * s) hopOff(b);
      if (t >= b.tV && !pops[j] && b.mode < 3) { pops[j] = 1; for (var m = 0; m < 2; m++) sys.spark(b.x, b.y + b.rad * 0.5, b.z, (rnd() - 0.5) * s, s * (1 + 2 * rnd()), (rnd() - 0.5) * s, s * 0.55, 1, 0.3 + 0.2 * rnd()); }
    }
    sys.update(t, dt, function (b, tt) { return b.mode === 2 ? Math.pow(0.5 + 0.5 * Math.sin(tt * (5 + 6 * b.seed) + b.seed * 50), 12) * 0.8 : 0; });
    flare *= Math.max(0, 1 - 4.5 * dt); starK *= Math.max(0, 1 - 7 * dt);
    var lit = smooth(0, 0.3, t) * (1 - smooth(END - 0.3, END, t) * 0.0);
    tintDie(die, HOARD.amber, 0.4 * lit + flare);
    fc.mat.emissiveIntensity = 1.3 * lit + 0.9 * flare;
    dim.material.opacity = 0.3 * smooth(0, 0.35, t) * (1 - smooth(1.4, 1.85, t));
    halo.material.opacity = 0.25 * lit + 0.3 * flare; halo.visible = halo.material.opacity > 0.003;
    star.material.opacity = 0.65 * starK; star.visible = starK > 0.01; star.scale.setScalar(s * (3.4 + 2.4 * (1 - starK)));
    return true;
  } };
}
registerCrit('hoard', { name: "Dragon's hoard", kind: 'succ',
  what: 'Gold coins and gems fountain out of the die and rain down around it.', fx: fxHoard, beat: 0.5, peak: 1.6,
  echo: fxHoardEcho, echoWhat: 'A few coins hop off the die and spin to a stop.' });
