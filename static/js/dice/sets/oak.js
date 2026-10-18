// ==================== Druid's oak ====================
// Dice turned from one block of real wood, the numbers inlaid in brass (or silver, gold, copper). Each
// die is cut from a single imaginary log: its rings are concentric about a pith that lies outside the
// die, so a face cut across the log shows arcs of end grain, a face cut along it shows long stripes
// and cathedral arches, and the grain runs on unbroken from face to face over every edge.
import {
  add, canvas, cross, dot, isLight, mix, mul, mulberry, norm, paintNumeral, polyPath, registerSet,
  strokeW, sub
} from '../engine.js';

// ---------- Noise ----------
var oakPerm = (function () {
  var r = mulberry(90210), p = [], i, o = new Uint8Array(512);
  for (i = 0; i < 256; i++) p.push(i);
  for (i = 255; i > 0; i--) { var j = Math.floor(r() * (i + 1)), t = p[i]; p[i] = p[j]; p[j] = t; }
  for (i = 0; i < 512; i++) o[i] = p[i & 255];
  return o;
})();
// Smooth value noise in 0..1; the hot loop below calls it a few times per pixel, so it allocates nothing.
function oakNoise(x, y, z) {
  var xi = Math.floor(x), yi = Math.floor(y), zi = Math.floor(z), xf = x - xi, yf = y - yi, zf = z - zi, P = oakPerm;
  xi &= 255; yi &= 255; zi &= 255;
  var u = xf * xf * (3 - 2 * xf), v = yf * yf * (3 - 2 * yf), w = zf * zf * (3 - 2 * zf);
  var a = P[xi] + yi, b = P[xi + 1] + yi, aa = P[a] + zi, ab = P[a + 1] + zi, ba = P[b] + zi, bb = P[b + 1] + zi;
  var x0 = P[aa] + (P[ba] - P[aa]) * u, x1 = P[ab] + (P[bb] - P[ab]) * u, x2 = P[aa + 1] + (P[ba + 1] - P[aa + 1]) * u, x3 = P[ab + 1] + (P[bb + 1] - P[ab + 1]) * u;
  var y0 = x0 + (x1 - x0) * v, y1 = x2 + (x3 - x2) * v;
  return (y0 + (y1 - y0) * w) / 255;
}
function oakHashI(i, salt) { return oakPerm[(oakPerm[(i + salt) & 255] + salt * 7) & 255] / 255; }

// ---------- The block each die is cut from ----------
// One block per (die, colour), so every face of a die reads the same wood. The grain axis is tilted a
// little off the die's y so no face is exactly end grain, and the pith sits well outside the die so
// the rings curve gently across it.
var OAKBLOCKS = {};
function oakBlock(sides, c) {
  var key = sides + '|' + c.key;
  if (OAKBLOCKS[key]) return OAKBLOCKS[key];
  var w = c.wood, r = mulberry(sides * 977 + c.key.length * 131 + c.key.charCodeAt(0) * 17);
  var tilt = 0.12 + r() * 0.3, az = r() * 6.28;
  var A = norm({ x: Math.sin(tilt) * Math.cos(az), y: Math.cos(tilt), z: Math.sin(tilt) * Math.sin(az) });
  var E1 = norm(cross(A, Math.abs(A.y) < 0.9 ? { x: 0, y: 1, z: 0 } : { x: 1, y: 0, z: 0 })), E2 = cross(A, E1);
  var ang = r() * 6.28, d0 = w.pith * (0.8 + r() * 0.5);
  var pith = add(mul(E1, Math.cos(ang) * d0), mul(E2, Math.sin(ang) * d0));
  OAKBLOCKS[key] = { A: A, E1: E1, E2: E2, pith: pith, W: w.ring * (0.9 + r() * 0.2), oval: 0.62 + r() * 0.2, taper: (r() - 0.5) * 2 * w.taper,
    warp: w.warp, salt: Math.floor(r() * 200) + 5, off: r() * 40 };
  return OAKBLOCKS[key];
}
// Ring phase at a point in the block's frame: the whole part counts rings, the fraction runs from
// early wood (0) to late wood (1).
function oakPhase(b, ax, ay, az) {
  var rad = Math.sqrt(ax * ax + ay * ay * b.oval) + b.taper * az + b.warp * (oakNoise(ax * 1.7 + b.off, ay * 1.7, az * 0.8 + 3.1) - 0.5), u = rad / b.W;
  // Ring widths wander: some years grew fast, some slow.
  return u + (oakNoise(u * 0.37 + b.off, 3.7, 0.5) - 0.5) * 0.9 + (oakNoise(ax * 6.5 + b.off, ay * 6.5, az * 1.4) - 0.5) * 0.32;
}
// The face a paint call is for: atlas seeds each face with sides * 53 + index * 11 + 5.
function oakFaceIndex(sd, lay, seed) {
  var fi = Math.round((seed - 5 - sd.sides * 53) / 11);
  if (fi >= 0 && fi < sd.faces.length) return fi;
  for (var i = 0; i < sd.faces.length; i++) if ((sd.faces[i].label || String(sd.faces[i].value)) === lay.label) return i;
  return 0;
}

// Fine speckle laid over every face: tiny lighter and darker flecks, drawn from one small tile.
var oakSpeckleTile = null;
function oakSpeckle() {
  if (oakSpeckleTile) return oakSpeckleTile;
  var t = canvas(96), g = t.getContext('2d'), r = mulberry(4242);
  for (var i = 0; i < 900; i++) {
    g.fillStyle = r() > 0.5 ? 'rgba(255,240,210,' + (0.03 + r() * 0.07) + ')' : 'rgba(20,8,0,' + (0.04 + r() * 0.09) + ')';
    g.fillRect(Math.floor(r() * 96), Math.floor(r() * 96), 1 + (r() > 0.8 ? 1 : 0), 1);
  }
  oakSpeckleTile = t;
  return t;
}

// ---------- The wood on one face ----------
// Every pixel of the face is looked up in the block, at full size for the colour and at half size
// (the size of the height and roughness maps) for the rest. Returns the images plus which way the grain
// runs across the face.
function oakField(sd, fi, lay, c) {
  var w = c.wood, blk = oakBlock(sd.sides, c), f = sd.faces[fi], R = f.right, U = f.up, cv = lay.cv, hr = cv >> 1, s = lay.s;
  var da = 1 / s, a0 = (0.5 - cv / 2) / s, b0 = (cv / 2 - 0.5) / s;
  var P00 = add(f.c, add(mul(R, a0), mul(U, b0))), Q = sub(P00, blk.pith);
  var qx0 = dot(Q, blk.E1), qy0 = dot(Q, blk.E2), qz0 = dot(Q, blk.A);
  var dxx = dot(R, blk.E1) * da, dxy = dot(R, blk.E2) * da, dxz = dot(R, blk.A) * da;
  var dyx = -dot(U, blk.E1) * da, dyy = -dot(U, blk.E2) * da, dyz = -dot(U, blk.A) * da;
  var xmin = cv, xmax = 0, ymin = cv, ymax = 0;
  lay.inner.forEach(function (p) { xmin = Math.min(xmin, p[0]); xmax = Math.max(xmax, p[0]); ymin = Math.min(ymin, p[1]); ymax = Math.max(ymax, p[1]); });
  xmin = Math.max(0, Math.floor(xmin) - 6); xmax = Math.min(cv - 1, Math.ceil(xmax) + 6); ymin = Math.max(0, Math.floor(ymin) - 6); ymax = Math.min(cv - 1, Math.ceil(ymax) + 6);
  var mk = function (n) { var cn = canvas(n), cg = cn.getContext('2d'); return { cn: cn, cg: cg, im: cg.createImageData(n, n) }; };
  var C = mk(cv), H = mk(hr), Rg = mk(hr), E = w.ew, L = w.lw, PC = w.pore, rough0 = c.base.rough * 255;
  var mid = [(E[0] + L[0]) / 2, (E[1] + L[1]) / 2, (E[2] + L[2]) / 2], cd = C.im.data, hd = H.im.data, rd = Rg.im.data, i;
  for (i = 0; i < cv * cv; i++) { cd[i * 4] = mid[0]; cd[i * 4 + 1] = mid[1]; cd[i * 4 + 2] = mid[2]; cd[i * 4 + 3] = 255; }
  for (i = 0; i < hr * hr; i++) { hd[i * 4] = hd[i * 4 + 1] = hd[i * 4 + 2] = 240; hd[i * 4 + 3] = 255; rd[i * 4 + 1] = rough0; rd[i * 4 + 3] = 255; }
  // The smooth parts (ring phase, mottling) are looked up on a coarse grid and blended between; only the fine
  // grain is looked up at every pixel.
  var G = 4, gw = ((xmax - xmin) >> 2) + 2, gh = ((ymax - ymin) >> 2) + 2, gu = new Float32Array(gw * gh), gm = new Float32Array(gw * gh), gi, gj;
  for (gj = 0; gj < gh; gj++) for (gi = 0; gi < gw; gi++) {
    var X = xmin + gi * G, Y = ymin + gj * G, gx0 = qx0 + dxx * X + dyx * Y, gy0 = qy0 + dxy * X + dyy * Y, gz0 = qz0 + dxz * X + dyz * Y;
    gu[gj * gw + gi] = oakPhase(blk, gx0, gy0, gz0); gm[gj * gw + gi] = oakNoise(gx0 * 2.1, gy0 * 2.1 + 5, gz0 * 0.6) - 0.5;
  }
  for (var y = ymin; y <= ymax; y++) for (var x = xmin; x <= xmax; x++) {
    var ax = qx0 + dxx * x + dyx * y, ay = qy0 + dxy * x + dyy * y, az = qz0 + dxz * x + dyz * y;
    var bi = (x - xmin) >> 2, bj = (y - ymin) >> 2, fx = ((x - xmin) & 3) / G, fy = ((y - ymin) & 3) / G, o0 = bj * gw + bi, o1 = o0 + gw;
    var u = (gu[o0] * (1 - fx) + gu[o0 + 1] * fx) * (1 - fy) + (gu[o1] * (1 - fx) + gu[o1 + 1] * fx) * fy;
    var mot = (gm[o0] * (1 - fx) + gm[o0 + 1] * fx) * (1 - fy) + (gm[o1] * (1 - fx) + gm[o1 + 1] * fx) * fy;
    var iu = Math.floor(u), fr = u - iu, ring = oakHashI(iu, blk.salt);
    // Late wood: a quick ramp to a dark band that ends at a soft-edged step (a hard one aliases).
    var gq = (fr - 0.4) / 0.34; gq = gq < 0 ? 0 : gq > 1 ? 1 : gq; var tl = gq * gq * (3 - 2 * gq) * (0.82 + 0.3 * fr);
    var dr = (fr - 0.92) / 0.08; dr = dr < 0 ? 0 : dr > 1 ? 1 : dr; tl *= 1 - dr * dr * (3 - 2 * dr);
    // A hair-thin darker line where a ring ends.
    var gl = (fr - 0.9) / 0.08; gl = gl < 0 || gl > 1 ? 0 : 1 - Math.abs(gl * 2 - 1);
    var fib = oakNoise(ax * 70, ay * 70, az * 2.2 + 9) - 0.5, fib2 = oakNoise(ax * 160 + 5, ay * 160, az * 3.4) - 0.5;
    // A hair-thin darker line where a ring ends.
    var gl = (fr - 0.9) / 0.08; gl = gl < 0 ? 0 : gl > 1 ? 0 : 1 - Math.abs(gl * 2 - 1);
    var tone = (tl * (0.5 + 0.7 * ring) + gl * 0.18) * w.contrast + (ring - 0.5) * 0.14 + fib * w.fibre + fib2 * w.fibre * 0.7 + mot * 0.2;
    tone = tone < 0 ? 0 : tone > 1 ? 1 : tone;
    var rr = E[0] + (L[0] - E[0]) * tone, gg = E[1] + (L[1] - E[1]) * tone, bb = E[2] + (L[2] - E[2]) * tone;
    // Pores: stretched along the grain, so dashes on a side face and dots on an end face; thickest in the early wood.
    var zone = fr < 0.4 ? 1 : fr < 0.58 ? (0.58 - fr) / 0.18 : 0;
    var pore = (oakNoise(ax * 120 + 31, ay * 120, az * 9) - (0.88 - 0.16 * zone)) / 0.07; pore = pore < 0 ? 0 : pore > 1 ? 1 : pore; pore *= w.pores;
    rr += (PC[0] - rr) * pore * 0.5; gg += (PC[1] - gg) * pore * 0.5; bb += (PC[2] - bb) * pore * 0.5;
    // Medullary rays: thin sheets of pale tissue standing out from the pith, seen as bright flecks where a face cuts across them.
    if (w.rays) {
      var th = Math.atan2(ay, ax) * 1.8, ti = Math.floor(th / 0.075), tf = th / 0.075 - ti, zq = az / 0.5, zi = Math.floor(zq), zf = zq - zi;
      if (oakHashI(ti + zi * 31, 91) < 0.3) {
        var ry = (1 - Math.abs(tf - 0.5) / 0.2), rz = Math.min(1, Math.min(zf, 1 - zf) * 7);
        ry = ry < 0 ? 0 : ry > 1 ? 1 : ry; var rays = ry * rz * w.rays;
        rr += (246 - rr) * rays * 0.22; gg += (224 - gg) * rays * 0.22; bb += (184 - bb) * rays * 0.2;
      }
    }
    var p = (y * cv + x) * 4;
    cd[p] = rr; cd[p + 1] = gg; cd[p + 2] = bb;
    if (!((x | y) & 1)) {
      // Early wood is softer and sits a touch lower; pores are tiny pits.
      var q = ((y >> 1) * hr + (x >> 1)) * 4, hv = 248 - 18 * (1 - tl) * 0.5 + fib * 22 - pore * 80, rv = rough0 + (0.5 - tl) * 18 + pore * 60;
      hd[q] = hd[q + 1] = hd[q + 2] = hv; rd[q + 1] = rv < 0 ? 0 : rv > 255 ? 255 : rv;
    }
  }
  [C, H, Rg].forEach(function (m) { m.cg.putImageData(m.im, 0, 0); });
  // The grain's direction across the face, in canvas px (the block's axis seen from above the face), and how end-on it is.
  var gx = dot(blk.A, R), gy = -dot(blk.A, U);
  return { col: C.cn, hgt: H.cn, rgh: Rg.cn, blk: blk, gx: gx, gy: gy, along: Math.min(1, Math.hypot(gx, gy)),
    at: function (X, Y) { var a = (X - cv / 2) / s, b = (cv / 2 - Y) / s; var Qp = add(sub(f.c, blk.pith), add(mul(R, a), mul(U, b))); return oakPhase(blk, dot(Qp, blk.E1), dot(Qp, blk.E2), dot(Qp, blk.A)); } };
}

// ---------- Colours ----------
// wood: ew / lw early and late wood, pore, ring (spacing in die radii), contrast, fibre, pores, rays,
// warp, taper, pith (how far outside the die the log's centre lies). metal is the inlay; fey is the colour of the
// plant the Overgrow show grows, which glows in burn.
var OAKCOLOURS = [
  { key: 'oak', name: 'Oak and brass', note: 'Pale English oak with medullary flecks, brass numbers', burn: '#ffcf6b', grow: '#e6b650', ink: '#dcae4c', metal: '#e6b84e', fey: '#4f8a34',
    wood: { ew: [196, 156, 98], lw: [126, 86, 44], pore: [92, 60, 28], ring: 0.12, contrast: 1, fibre: 0.2, pores: 1, rays: 1, warp: 0.08, taper: 0.1, pith: 1.5 },
    base: { grad: ['#c9a468', '#8c6234'], rough: 0.5 } },
  { key: 'walnut', name: 'Walnut and brass', note: 'Dark chocolate walnut, bright brass numbers', burn: '#ffc766', grow: '#eec45e', ink: '#e8bf5a', metal: '#e2b64f', fey: '#3f7a2e',
    wood: { ew: [122, 86, 62], lw: [58, 38, 26], pore: [26, 15, 9], ring: 0.15, contrast: 1.1, fibre: 0.26, pores: 0.7, rays: 0.15, warp: 0.1, taper: 0.12, pith: 1.7 },
    base: { grad: ['#6e4c37', '#3a2418'], rough: 0.46 } },
  { key: 'ebony', name: 'Ebony and silver', note: 'Black ebony, silver numbers', burn: '#c4e4ff', grow: '#e8edf5', ink: '#dde2ea', metal: '#d8dde6', fey: '#2f6f5a',
    wood: { ew: [58, 50, 44], lw: [16, 14, 13], pore: [6, 5, 5], ring: 0.1, contrast: 0.85, fibre: 0.22, pores: 0.3, rays: 0, warp: 0.06, taper: 0.07, pith: 1.4 },
    base: { grad: ['#2c2723', '#0f0d0c'], rough: 0.4 } },
  { key: 'bloodwood', name: 'Bloodwood and gold', note: 'Deep red bloodwood, gold numbers', burn: '#ffd36b', grow: '#ffd56c', ink: '#f2c24e', metal: '#f0c04c', fey: '#4a7a2a',
    wood: { ew: [172, 54, 32], lw: [92, 20, 14], pore: [50, 10, 6], ring: 0.11, contrast: 0.95, fibre: 0.24, pores: 0.45, rays: 0.3, warp: 0.08, taper: 0.1, pith: 1.5 },
    base: { grad: ['#a8382a', '#5e170f'], rough: 0.46 } },
  { key: 'ash', name: 'Ash and copper', note: 'Pale ash with bold grain, copper numbers', burn: '#ffa468', grow: '#eb9462', ink: '#d98650', metal: '#e88c52', fey: '#5a8a30',
    wood: { ew: [238, 226, 196], lw: [190, 160, 116], pore: [138, 104, 66], ring: 0.14, contrast: 1, fibre: 0.2, pores: 0.8, rays: 0.1, warp: 0.09, taper: 0.1, pith: 1.6 },
    base: { grad: ['#e6d6b0', '#b89c6c'], rough: 0.5 } }
];

// The number: a quiet, plain patch of wood, a burnt channel cut into it with a soft scorch round the cut, and the
// metal set in the channel. The patch and the char are what make the number readable against busy grain.
var oakChannel = 1.85, oakMetal = 1.1;
function oakInlay(g, lay, c) {
  var w = strokeW(lay), cv = lay.cv, W = c.wood, ext = lay.strokes.reduce(function (m, pl) { pl.forEach(function (p) { m[0] = Math.min(m[0], p[0]); m[1] = Math.min(m[1], p[1]); m[2] = Math.max(m[2], p[0]); m[3] = Math.max(m[3], p[1]); }); return m; }, [cv, cv, 0, 0]);
  var plain = [0, 1, 2].map(function (i) { return Math.round(W.ew[i] * 0.7 + W.lw[i] * 0.3); });
  g.save();
  g.fillStyle = g.strokeStyle = 'rgba(' + plain.join(',') + ',.62)'; try { g.filter = 'blur(' + (w * 1.7) + 'px)'; } catch (e) {}
  paintNumeral(g, lay, lay.style, w * 4.4);
  g.fillStyle = g.strokeStyle = 'rgba(22,9,2,.5)'; try { g.filter = 'blur(' + (w * 0.55) + 'px)'; } catch (e) {}
  paintNumeral(g, lay, lay.style, w * 2.9);
  try { g.filter = 'none'; } catch (e) {}
  g.fillStyle = g.strokeStyle = 'rgba(8,3,1,.97)'; paintNumeral(g, lay, lay.style, w * oakChannel);
  var gr = g.createLinearGradient(ext[0], ext[1], ext[2], ext[3]);
  gr.addColorStop(0, mix(c.metal, '#ffffff', 0.2)); gr.addColorStop(0.55, c.metal); gr.addColorStop(1, mix(c.metal, '#2a1608', 0.12));
  g.fillStyle = g.strokeStyle = gr; paintNumeral(g, lay, lay.style, w * oakMetal);
  g.restore();
}

registerSet({
  key: 'oak', name: "Druid's oak", note: 'Carved wood with real grain. Numbers in inlaid brass.', show: 'overgrow', bev: 0.045,
  colours: OAKCOLOURS,
  paint: function (g, lay, c, seed, sd) {
    var cv = lay.cv, k = cv / 256, fi = oakFaceIndex(sd, lay, seed), F = oakField(sd, fi, lay, c), rnd = mulberry(seed * 7 + 3), w = c.wood;
    g.drawImage(F.col, 0, 0);
    g.save(); polyPath(g, lay.inner); g.clip();
    // Hand-drawn accents over the lookup: fine pores in the early wood and hair-thin grain lines in the late, each along
    // the grain (a dot or a speck on an end face).
    var hy = Math.hypot(F.gx, F.gy) || 1, nx = F.gx / hy, ny = F.gy / hy, pc = w.pore, i, n = Math.round(cv * cv / 90);
    g.lineCap = 'round';
    for (i = 0; i < n; i++) {
      var X = rnd() * cv, Y = rnd() * cv, fr = F.at(X, Y), z = fr - Math.floor(fr), take = z < 0.42 ? 0.5 : z < 0.58 ? 0.18 : 0.03;
      if (rnd() > take * w.pores) continue;
      var L = (0.7 + 3.4 * F.along) * k * (0.5 + rnd());
      g.strokeStyle = 'rgba(' + pc[0] + ',' + pc[1] + ',' + pc[2] + ',' + (0.28 + rnd() * 0.3) + ')'; g.lineWidth = (0.7 + rnd() * 0.6) * k;
      g.beginPath(); g.moveTo(X, Y); g.lineTo(X + nx * L, Y + ny * L); g.stroke();
    }
    if (F.along > 0.12) {
      n = Math.round(cv * cv / 420 * F.along); g.lineWidth = 0.8 * k;
      for (i = 0; i < n; i++) {
        var X2 = rnd() * cv, Y2 = rnd() * cv, f2 = F.at(X2, Y2), z2 = f2 - Math.floor(f2);
        if (rnd() > 0.15 + 0.85 * Math.max(0, (z2 - 0.5) * 2)) continue;
        var L2 = (14 + rnd() * 46) * k * F.along;
        g.strokeStyle = 'rgba(' + pc[0] + ',' + pc[1] + ',' + pc[2] + ',' + (0.05 + rnd() * 0.12) + ')';
        g.beginPath(); g.moveTo(X2, Y2); g.lineTo(X2 + nx * L2, Y2 + ny * L2); g.stroke();
      }
    }
    g.globalAlpha = 1; g.fillStyle = g.createPattern(oakSpeckle(), 'repeat'); g.fillRect(0, 0, cv, cv);
    // Oiled and handled: the middle of a face is polished warm.
    var pol = g.createRadialGradient(lay.cx, lay.cy, 0, lay.cx, lay.cy, lay.r * 1.5); pol.addColorStop(0, 'rgba(255,232,190,.06)'); pol.addColorStop(1, 'rgba(255,232,190,0)');
    g.fillStyle = pol; g.fillRect(0, 0, cv, cv);
    g.restore();
    // The edges darken where fingers have held the die; drawn past the face's outline so no pale line is left along it.
    g.save(); g.strokeStyle = 'rgba(30,13,2,.4)'; g.lineWidth = cv * 0.05; try { g.filter = 'blur(' + (cv * 0.018) + 'px)'; } catch (e) {}
    polyPath(g, lay.inner); g.stroke(); g.restore();
    oakInlay(g, lay, c);
    lay._oak = F;
  },
  surface: function (g, lay, c) {
    var F = lay._oak, w = strokeW(lay), cv = lay.cv;
    if (F) g.drawImage(F.rgh, 0, 0, cv, cv);
    // The gap is dull bare wood; the metal is a polished inlay, satin from handling.
    g.fillStyle = g.strokeStyle = 'rgb(0,235,0)'; paintNumeral(g, lay, lay.style, w * oakChannel);
    g.fillStyle = g.strokeStyle = 'rgb(0,90,255)'; paintNumeral(g, lay, lay.style, w * oakMetal);
    if (F) F.rgh = null;
  },
  height: function (g, lay, c) {
    var F = lay._oak, w = strokeW(lay), cv = lay.cv;
    if (F) g.drawImage(F.hgt, 0, 0, cv, cv);
    try { g.filter = 'blur(0.7px)'; } catch (e) {}
    g.fillStyle = g.strokeStyle = '#2c2c2c'; paintNumeral(g, lay, lay.style, w * oakChannel);
    g.fillStyle = g.strokeStyle = '#bdbdbd'; paintNumeral(g, lay, lay.style, w * oakMetal);
    lay._oak = null;
  },
  relief: 0.5,
  // The rounded edges: the wood a shade darker, as it goes where hands have worn and oiled it.
  rim: function (c) { return { col: mix(mix(c.base.grad[0], c.base.grad[1], 0.5), '#1c0e04', 0.42), rough: Math.min(0.72, c.base.rough + 0.14), metal: 0 }; },
  physical: function (c, q) { return { specularIntensity: 0.8, envMapIntensity: 1.05 }; },
  // The brass mirrors the bright room around the die, not just the dark ceiling over it.
  shader: function (s, c) {
    s.fragmentShader = s.fragmentShader.replace('#include <opaque_fragment>', 'outgoingLight += diffuseColor.rgb * metalnessFactor * 0.16;\n#include <opaque_fragment>');
  },
  cover: function (c) { return isLight(c) ? 0.6 : 0; }
});
