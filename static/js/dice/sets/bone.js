// ==================== Dragonbone ====================
// Dice carved from bone: a warm, fine-pored surface with faint streaks along the length of the bone and a
// yellowing where hands and years have settled into the recesses. The numbers are cut in and filled with ink,
// and every corner wears a small polished gold cap.
import {
  add, canvas, cross, dot, hexA, isLight, kindOf, len, mix, mul, mulberry, norm, paintNumeral,
  polyPath, registerSet, strokeW, sub, share
} from '../engine.js';

// ---------- Noise ----------
var bonePerm = (function () {
  var r = mulberry(31337), p = [], i, o = new Uint8Array(512);
  for (i = 0; i < 256; i++) p.push(i);
  for (i = 255; i > 0; i--) { var j = Math.floor(r() * (i + 1)), t = p[i]; p[i] = p[j]; p[j] = t; }
  for (i = 0; i < 512; i++) o[i] = p[i & 255];
  return o;
})();
// Smooth value noise in 0..1; allocation-free, called a few times per pixel.
function boneNoise(x, y, z) {
  var xi = Math.floor(x), yi = Math.floor(y), zi = Math.floor(z), xf = x - xi, yf = y - yi, zf = z - zi, P = bonePerm;
  xi &= 255; yi &= 255; zi &= 255;
  var u = xf * xf * (3 - 2 * xf), v = yf * yf * (3 - 2 * yf), w = zf * zf * (3 - 2 * zf);
  var a = P[xi] + yi, b = P[xi + 1] + yi, aa = P[a] + zi, ab = P[a + 1] + zi, ba = P[b] + zi, bb = P[b + 1] + zi;
  var x0 = P[aa] + (P[ba] - P[aa]) * u, x1 = P[ab] + (P[bb] - P[ab]) * u, x2 = P[aa + 1] + (P[ba + 1] - P[aa + 1]) * u, x3 = P[ab + 1] + (P[bb + 1] - P[ab + 1]) * u;
  var y0 = x0 + (x1 - x0) * v, y1 = x2 + (x3 - x2) * v;
  return (y0 + (y1 - y0) * w) / 255;
}

// ---------- The piece of bone each die is carved from ----------
// One per (die, colour): the bone's long axis, so the streaks and pores run the same way on every face
// and carry on over the edges.
var BONEBLOCKS = {};
function boneBlock(sides, c) {
  var key = sides + '|' + c.key;
  if (BONEBLOCKS[key]) return BONEBLOCKS[key];
  var r = mulberry(sides * 613 + c.key.length * 71 + c.key.charCodeAt(0) * 29);
  var tilt = 0.25 + r() * 0.5, az = r() * 6.28;
  var A = norm({ x: Math.sin(tilt) * Math.cos(az), y: Math.cos(tilt), z: Math.sin(tilt) * Math.sin(az) });
  var E1 = norm(cross(A, Math.abs(A.y) < 0.9 ? { x: 0, y: 1, z: 0 } : { x: 1, y: 0, z: 0 })), E2 = cross(A, E1);
  BONEBLOCKS[key] = { A: A, E1: E1, E2: E2, off: r() * 50, pith: add(mul(E1, 0.9 + r() * 0.6), mul(E2, (r() - 0.5) * 1.6)) };
  return BONEBLOCKS[key];
}
// The face a paint call is for: atlas seeds each face with sides * 53 + index * 11 + 5.
function boneFaceIndex(sd, lay, seed) {
  var fi = Math.round((seed - 5 - sd.sides * 53) / 11);
  if (fi >= 0 && fi < sd.faces.length) return fi;
  for (var i = 0; i < sd.faces.length; i++) if ((sd.faces[i].label || String(sd.faces[i].value)) === lay.label) return i;
  return 0;
}
var boneSpeckleTile = null;
function boneSpeckle() {
  if (boneSpeckleTile) return boneSpeckleTile;
  var t = canvas(96), g = t.getContext('2d'), r = mulberry(777);
  for (var i = 0; i < 700; i++) {
    g.fillStyle = r() > 0.55 ? 'rgba(255,250,235,' + (0.04 + r() * 0.08) + ')' : 'rgba(70,45,15,' + (0.03 + r() * 0.07) + ')';
    g.fillRect(Math.floor(r() * 96), Math.floor(r() * 96), 1 + (r() > 0.85 ? 1 : 0), 1);
  }
  boneSpeckleTile = t;
  return t;
}

// ---------- The bone on one face ----------
// Colour, height and roughness at half size (the size of the height and roughness maps; the colour is
// stretched, bone being soft-edged), plus which way the grain runs across the face.
function boneField(sd, fi, lay, c) {
  var b = c.bone, blk = boneBlock(sd.sides, c), f = sd.faces[fi], R = f.right, U = f.up, cv = lay.cv, hr = cv >> 1, s = lay.s;
  var da = 2 / s, a0 = (1 - cv / 2) / s, b0 = (cv / 2 - 1) / s;
  var P00 = add(f.c, add(mul(R, a0), mul(U, b0))), Q = sub(P00, blk.pith);
  var qx0 = dot(Q, blk.E1), qy0 = dot(Q, blk.E2), qz0 = dot(Q, blk.A);
  var dxx = dot(R, blk.E1) * da, dxy = dot(R, blk.E2) * da, dxz = dot(R, blk.A) * da;
  var dyx = -dot(U, blk.E1) * da, dyy = -dot(U, blk.E2) * da, dyz = -dot(U, blk.A) * da;
  var mk = function () { var cn = canvas(hr), cg = cn.getContext('2d'); return { cn: cn, cg: cg, im: cg.createImageData(hr, hr) }; };
  var C = mk(), H = mk(), Rg = mk(), HI = b.hi, LO = b.lo, ST = b.streak, YL = b.stain, rough0 = c.base.rough * 255;
  var cd = C.im.data, hd = H.im.data, rd = Rg.im.data;
  for (var y = 0; y < hr; y++) for (var x = 0; x < hr; x++) {
    var ax = qx0 + dxx * x + dyx * y, ay = qy0 + dxy * x + dyy * y, az = qz0 + dxz * x + dyz * y;
    // Broad tonal drift (age and handling), faint lamellae along the bone, hair-fine streaks and a soft patchy yellowing.
    var drift = boneNoise(ax * 1.3 + blk.off, ay * 1.3, az * 0.7), rad = Math.sqrt(ax * ax + ay * ay);
    var lam = 0.5 + 0.5 * Math.sin(rad * b.lamella + boneNoise(ax * 2.2, ay * 2.2, az * 0.9) * 3.2);
    var st = boneNoise(ax * 90 + blk.off, ay * 90, az * 1.4), stain = boneNoise(ax * 2.4 + 11, ay * 2.4, az * 1.1 + blk.off);
    var sq = (st - 0.5) / 0.3; sq = sq < 0 ? 0 : sq > 1 ? 1 : sq;
    var t = Math.min(1, Math.max(0, (drift - 0.3) * 1.6 * b.drift + (1 - lam) * 0.1 * b.lamellaK));
    var rr = HI[0] + (LO[0] - HI[0]) * t, gg = HI[1] + (LO[1] - HI[1]) * t, bb = HI[2] + (LO[2] - HI[2]) * t;
    var sk = sq * b.streakK * 0.65; rr += (ST[0] - rr) * sk; gg += (ST[1] - gg) * sk; bb += (ST[2] - bb) * sk;
    var yq = (stain - 0.38) / 0.45; yq = yq < 0 ? 0 : yq > 1 ? 1 : yq; var yl = yq * yq * (3 - 2 * yq) * 0.75 * b.stainK;
    rr += (YL[0] - rr) * yl; gg += (YL[1] - gg) * yl; bb += (YL[2] - bb) * yl;
    var p = (y * hr + x) * 4;
    cd[p] = rr; cd[p + 1] = gg; cd[p + 2] = bb; cd[p + 3] = 255;
    hd[p] = hd[p + 1] = hd[p + 2] = 246 - sk * 26 - t * 6 + (st - 0.5) * 8; hd[p + 3] = 255;
    var rv = rough0 + sk * 22 + yl * 14; rd[p + 1] = rv > 255 ? 255 : rv; rd[p + 3] = 255;
  }
  [C, H, Rg].forEach(function (m) { m.cg.putImageData(m.im, 0, 0); });
  var gx = dot(blk.A, R), gy = -dot(blk.A, U);
  return { col: C.cn, hgt: H.cn, rgh: Rg.cn, gx: gx, gy: gy, along: Math.min(1, Math.hypot(gx, gy)) };
}

// ---------- Colours ----------
// bone: hi / lo the two body tones, streak the colour of the faint lines, stain the yellowing, pore the
// colour of the pits; drift, streakK, stainK, lamellaK how strong each is. ink is the filling of the
// numbers (inkMetal how metallic it is); pop is the enamel the show raises.
var BONECOLOURS = [
  { key: 'bleached', name: 'Bleached and black ink', note: 'Sun-bleached white bone, black ink numbers', burn: '#ff7a52', pop: '#d4202f', ink: '#17151b', inkMetal: 0,
    base: { grad: ['#f2ecde', '#d8ceb8'], rough: 0.44 },
    bone: { hi: [238, 231, 214], lo: [212, 200, 176], streak: [198, 184, 156], stain: [214, 188, 138], pore: [128, 108, 78], drift: 1, streakK: 0.7, stainK: 0.55, lamellaK: 1, lamella: 38 } },
  { key: 'ivory', name: 'Ivory and red ink', note: 'Cream ivory with fine grain, red ink numbers', burn: '#ff5a48', pop: '#ff3d33', ink: '#a3151f', inkMetal: 0,
    base: { grad: ['#eddcb6', '#cbb283'], rough: 0.42 },
    bone: { hi: [236, 218, 180], lo: [204, 176, 126], streak: [176, 144, 94], stain: [196, 152, 86], pore: [112, 84, 48], drift: 1.2, streakK: 1.1, stainK: 0.85, lamellaK: 1.3, lamella: 46 } },
  { key: 'ochre', name: 'Ochre and umber ink', note: 'Ochre-stained bone, umber ink numbers', burn: '#ffbe4a', pop: '#b8400e', ink: '#3a2310', inkMetal: 0,
    base: { grad: ['#d3a560', '#a87638'], rough: 0.46 },
    bone: { hi: [216, 170, 100], lo: [166, 114, 52], streak: [140, 90, 38], stain: [128, 78, 30], pore: [90, 54, 22], drift: 1.3, streakK: 1.1, stainK: 0.85, lamellaK: 1, lamella: 40 } },
  { key: 'charred', name: 'Charred and gold ink', note: 'Charred black bone, gold ink numbers', burn: '#ffd36b', pop: '#ffd24a', ink: '#e8bb48', inkMetal: 0.65,
    base: { grad: ['#34302c', '#100f0e'], rough: 0.4 },
    bone: { hi: [58, 53, 48], lo: [22, 20, 18], streak: [96, 88, 78], stain: [70, 56, 40], pore: [8, 7, 6], drift: 1.2, streakK: 0.8, stainK: 0.45, lamellaK: 1, lamella: 36 } },
  { key: 'jade', name: 'Jade and green ink', note: 'Jade-stained bone, dark green ink numbers', burn: '#8dffc4', pop: '#0e6a43', ink: '#0d3324', inkMetal: 0,
    base: { grad: ['#a5c9a8', '#6f9f82'], rough: 0.42 },
    bone: { hi: [170, 205, 172], lo: [96, 148, 116], streak: [70, 118, 88], stain: [132, 156, 92], pore: [36, 72, 52], drift: 1.3, streakK: 1.1, stainK: 0.8, lamellaK: 1.1, lamella: 42 } }
];

// The number: a cut with a stained edge, filled with ink.
var boneCutW = 1.3, boneInkW = 1.02;
function boneCarve(g, lay, c) {
  var w = strokeW(lay), b = c.bone, st = b.stain, k = lay.cv / 256;
  g.save();
  // Yellowing settles into the recess and spreads a little over the face round it.
  g.fillStyle = g.strokeStyle = 'rgba(' + st.join(',') + ',.4)'; try { g.filter = 'blur(' + (w * 0.9) + 'px)'; } catch (e) {}
  paintNumeral(g, lay, lay.style, w * 2.5);
  try { g.filter = 'none'; } catch (e) {}
  // The wall of the cut, darker than the face, then the ink.
  var dk = mix('#' + ((1 << 24) + (st[0] << 16) + (st[1] << 8) + st[2]).toString(16).slice(1), '#000000', c.key === 'charred' ? 0.2 : 0.55);
  g.fillStyle = g.strokeStyle = hexA(dk, 0.85); paintNumeral(g, lay, lay.style, w * boneCutW);
  g.fillStyle = g.strokeStyle = c.ink; paintNumeral(g, lay, lay.style, w * boneInkW);
  g.restore();
}

// ---------- Gold caps ----------
// One cap for every corner, as a single instanced mesh. A cap is a short collar and a low dome, set so the
// dome stands just clear of the rounded corner and the collar sinks into the faces round it.
var boneCapGeo = {}, boneCapMat = null, boneTips = {};
function boneCapGeometry(T, hd) {
  var k = hd ? 'hd' : 'basic';
  if (boneCapGeo[k]) return boneCapGeo[k];
  var pts = [[0, -1.3], [1.06, -1.3], [1.06, -0.16], [1.0, -0.06], [1.04, 0.02], [0.98, 0.1], [0.9, 0.18], [0.78, 0.34], [0.6, 0.5], [0.4, 0.62], [0.2, 0.69], [0, 0.71]]
    .map(function (p) { return new T.Vector2(p[0], p[1]); });
  var g = new T.LatheGeometry(pts, hd ? 24 : 14); share(g);
  boneCapGeo[k] = g;
  return g;
}
function boneCapMaterial(T) {
  if (boneCapMat) return boneCapMat;
  // Polished gold: a mirror, tinted, kept a little soft so the studio's boxes read as gleams, not a white hole.
  boneCapMat = new T.MeshPhysicalMaterial({ color: 0xf3c64f, metalness: 1, roughness: 0.2, envMapIntensity: 1.5 });
  share(boneCapMat);
  return boneCapMat;
}
// How far out along each corner's direction the rounded die reaches (unit size).
function boneTipsOf(die) {
  var key = die.sides + '|' + die.atl.bv.rim.length + '|' + kindOf(die.kind).bev;
  if (boneTips[key]) return boneTips[key];
  var out = die.sd.v.map(function (v) {
    var n = norm(v), best = 0;
    die.atl.bv.rim.forEach(function (R) { R.p.forEach(function (p) { if (len(sub(p, v)) < 0.4) best = Math.max(best, dot(p, n)); }); });
    return best || len(v) * 0.95;
  });
  boneTips[key] = out;
  return out;
}
var BONECAPR = { 4: 0.15, 6: 0.14, 8: 0.14, 10: 0.115, 100: 0.115, 12: 0.12, 20: 0.1 };

registerSet({
  key: 'bone', name: 'Dragonbone', note: 'Carved bone with gold-capped corners. Numbers carved and filled with ink.', show: 'enamel', bev: 0.07,
  colours: BONECOLOURS,
  paint: function (g, lay, c, seed, sd) {
    var cv = lay.cv, k = cv / 256, fi = boneFaceIndex(sd, lay, seed), F = boneField(sd, fi, lay, c), rnd = mulberry(seed * 5 + 1), b = c.bone, i;
    g.imageSmoothingEnabled = true; g.imageSmoothingQuality = 'high';
    g.drawImage(F.col, 0, 0, cv, cv);
    g.save(); polyPath(g, lay.inner); g.clip();
    var hy = Math.hypot(F.gx, F.gy) || 1, nx = F.gx / hy, ny = F.gy / hy, pc = b.pore, n;
    // Pores: tiny pits and short channels running with the bone (specks on a cut end).
    g.lineCap = 'round';
    n = Math.round(cv * cv / 150);
    for (i = 0; i < n; i++) {
      var X = rnd() * cv, Y = rnd() * cv, L = (0.5 + 3.8 * F.along) * k * (0.4 + rnd());
      g.strokeStyle = 'rgba(' + pc[0] + ',' + pc[1] + ',' + pc[2] + ',' + (0.3 + rnd() * 0.4) + ')'; g.lineWidth = (0.6 + rnd() * 0.8) * k;
      g.beginPath(); g.moveTo(X, Y); g.lineTo(X + nx * L, Y + ny * L); g.stroke();
    }
    // A few longer, shallow grooves where small vessels ran, wandering a little.
    if (F.along > 0.1) {
      n = 2 + Math.floor(rnd() * 4);
      for (i = 0; i < n; i++) {
        var gx = rnd() * cv, gy = rnd() * cv, gl = (30 + rnd() * 90) * k * F.along, steps = 7;
        g.strokeStyle = 'rgba(' + pc[0] + ',' + pc[1] + ',' + pc[2] + ',' + (0.1 + rnd() * 0.14) + ')'; g.lineWidth = (0.9 + rnd() * 0.9) * k;
        g.beginPath(); g.moveTo(gx, gy);
        for (var j = 1; j <= steps; j++) g.lineTo(gx + nx * gl * j / steps - ny * Math.sin(j * 0.9 + i) * 2.5 * k, gy + ny * gl * j / steps + nx * Math.sin(j * 0.9 + i) * 2.5 * k);
        g.stroke();
      }
    }
    // Hair-thin streaks along the bone.
    if (F.along > 0.1) {
      n = Math.round(cv * cv / 260 * F.along); g.lineWidth = 0.8 * k; var sc = b.streak;
      for (i = 0; i < n; i++) {
        var sx = rnd() * cv, sy = rnd() * cv, sl = (10 + rnd() * 40) * k * F.along;
        g.strokeStyle = 'rgba(' + sc[0] + ',' + sc[1] + ',' + sc[2] + ',' + (0.07 + rnd() * 0.12) + ')'; g.beginPath(); g.moveTo(sx, sy); g.lineTo(sx + nx * sl, sy + ny * sl); g.stroke();
      }
    }
    g.fillStyle = g.createPattern(boneSpeckle(), 'repeat'); g.fillRect(0, 0, cv, cv);
    g.restore();
    // Yellowing settles at the edges where the die has been held; drawn past the outline so no pale line is left along it.
    g.save(); g.strokeStyle = 'rgba(' + b.stain.join(',') + ',.44)'; g.lineWidth = cv * 0.05; try { g.filter = 'blur(' + (cv * 0.018) + 'px)'; } catch (e) {}
    polyPath(g, lay.inner); g.stroke(); g.restore();
    boneCarve(g, lay, c);
    lay._bone = F;
  },
  surface: function (g, lay, c) {
    var F = lay._bone, w = strokeW(lay), cv = lay.cv;
    if (F) g.drawImage(F.rgh, 0, 0, cv, cv);
    // The cut's wall is rough; the ink in it is smooth and a little glossy (a touch metallic for gold ink).
    g.fillStyle = g.strokeStyle = 'rgb(0,190,0)'; paintNumeral(g, lay, lay.style, w * boneCutW);
    g.fillStyle = g.strokeStyle = 'rgb(0,' + Math.round(c.inkMetal ? 85 : 80) + ',' + Math.round(c.inkMetal * 255) + ')'; paintNumeral(g, lay, lay.style, w * boneInkW);
    if (F) F.rgh = null;
  },
  height: function (g, lay, c) {
    var F = lay._bone, w = strokeW(lay), cv = lay.cv;
    if (F) g.drawImage(F.hgt, 0, 0, cv, cv);
    try { g.filter = 'blur(0.7px)'; } catch (e) {}
    g.fillStyle = g.strokeStyle = '#4a4a4a'; paintNumeral(g, lay, lay.style, w * boneCutW);
    g.fillStyle = g.strokeStyle = '#767676'; paintNumeral(g, lay, lay.style, w * boneInkW);
    lay._bone = null;
  },
  relief: 0.7,
  // The rounded edges: the bone a shade yellower and darker, as handling leaves it.
  rim: function (c) { return { col: mix(mix(c.base.grad[0], c.base.grad[1], 0.5), '#' + ((1 << 24) + (c.bone.stain[0] << 16) + (c.bone.stain[1] << 8) + c.bone.stain[2]).toString(16).slice(1), 0.42), rough: Math.min(0.7, c.base.rough + 0.1), metal: 0 }; },
  // Bone is a little waxy: a soft sheen at grazing angles keeps it from reading as painted plastic.
  physical: function (c, q) { return { specularIntensity: 0.9, sheen: q.detail === 'basic' ? 0 : 0.5, sheenRoughness: 0.55, sheenColor: isLight(c) ? '#fff3dc' : '#6a6258' }; },
  build: function (T, die, q) {
    var hd = q.detail !== 'basic', sd = die.sd, size = die.size, n = sd.v.length, tips = boneTipsOf(die), R = (BONECAPR[die.sides] || 0.12) * size;
    var im = new T.InstancedMesh(boneCapGeometry(T, hd), boneCapMaterial(T), n), Y = new T.Vector3(0, 1, 0), P = new T.Vector3(), N = new T.Vector3(), S = new T.Vector3(R, R, R), Qt = new T.Quaternion(), M = new T.Matrix4();
    for (var i = 0; i < n; i++) {
      var v = sd.v[i]; N.set(v.x, v.y, v.z).normalize();
      // Origin of the cap: the dome's top stands a hair beyond the corner's tip.
      P.copy(N).multiplyScalar(tips[i] * size + 0.03 * R - 0.71 * R);
      Qt.setFromUnitVectors(Y, N); im.setMatrixAt(i, M.compose(P, Qt, S));
    }
    im.castShadow = true; im.layers.enable(1); im.instanceMatrix.needsUpdate = true;
    return { parts: [im], tick: null, dispose: function () { im.dispose(); } };
  },
  cover: function (c) { return isLight(c) ? 0.65 : 0; }
});
