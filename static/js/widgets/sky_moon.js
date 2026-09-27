/*
 * sky_moon.js — the realistic moon renderer (MOONR) and Chronicle's small
 * library of moon-library body specs (SkyMoon).
 *
 * MOONR is ported close to verbatim from the signed sky-pane design contract
 * (issue #763's artifact): a seeded procedural body (seas, craters, ray
 * craters) is described once on the unit sphere, "prepared" once per pixel
 * size (albedo, relief normals, east/west horizons), and a given night's
 * phase is then a cheap per-pixel shade. This is the SAME renderer the
 * moon-graph widget (#762, not yet built) will use, so a moon drawn in the
 * sky and a moon drawn in its own graph are always the same body.
 *
 * SkyMoon maps a real calendar_moons row (Moon.BaseDesign / Color) onto one
 * of these procedural specs: a handful are hand-authored (mirroring the
 * mockup's SURF.vantre/ashka/cinder/hollow), and any other BaseDesign value
 * (a system's own moon key we don't have art direction for) falls back to a
 * body generated deterministically from a hash of that string, so every
 * moon renders a plausible, STABLE body rather than a blank disc.
 *
 * Registered on window as MOONR (the renderer, unchanged surface from the
 * contract) and SkyMoon (Chronicle's spec library on top of it).
 */
(function () {
  'use strict';

  var MOONR = (function () {
    function rng(seed) { var s = seed >>> 0; return function () { s = (s + 0x6D2B79F5) >>> 0; var t = s; t = Math.imul(t ^ (t >>> 15), t | 1); t ^= t + Math.imul(t ^ (t >>> 7), t | 61); return ((t ^ (t >>> 14)) >>> 0) / 4294967296; }; }
    function makeNoise(seed) {
      var r = rng(seed), p = new Uint8Array(512), g = new Float32Array(256), i;
      for (i = 0; i < 256; i++) { p[i] = i; g[i] = r() * 2 - 1; }
      for (i = 255; i > 0; i--) { var j = (r() * (i + 1)) | 0, t = p[i]; p[i] = p[j]; p[j] = t; }
      for (i = 0; i < 256; i++) p[i + 256] = p[i];
      return function (x, y, z) {
        var X = Math.floor(x), Y = Math.floor(y), Z = Math.floor(z), xf = x - X, yf = y - Y, zf = z - Z;
        X &= 255; Y &= 255; Z &= 255;
        var u = xf * xf * (3 - 2 * xf), v = yf * yf * (3 - 2 * yf), w = zf * zf * (3 - 2 * zf);
        var a = p[X] + Y, aa = p[a] + Z, ab = p[a + 1] + Z, b = p[X + 1] + Y, ba = p[b] + Z, bb = p[b + 1] + Z;
        var x1 = g[p[aa]] + (g[p[ba]] - g[p[aa]]) * u, x2 = g[p[ab]] + (g[p[bb]] - g[p[ab]]) * u;
        var x3 = g[p[aa + 1]] + (g[p[ba + 1]] - g[p[aa + 1]]) * u, x4 = g[p[ab + 1]] + (g[p[bb + 1]] - g[p[ab + 1]]) * u;
        var y1 = x1 + (x2 - x1) * v, y2 = x3 + (x4 - x3) * v;
        return y1 + (y2 - y1) * w;
      };
    }
    function fbm(n, x, y, z, oct) { var s = 0, a = .5, f = 1; for (var i = 0; i < oct; i++) { s += a * n(x * f, y * f, z * f); a *= .5; f *= 2.07; } return s; }
    function smooth(a, b, x) { var t = (x - a) / (b - a); t = t < 0 ? 0 : t > 1 ? 1 : t; return t * t * (3 - 2 * t); }
    function norm3(v) { var n = Math.hypot(v[0], v[1], v[2]) || 1; return [v[0] / n, v[1] / n, v[2] / n]; }
    function tangent(c) { var e1 = norm3(Math.abs(c[1]) < .9 ? [c[2], 0, -c[0]] : [0, c[2], -c[1]]); return [e1, norm3([c[1] * e1[2] - c[2] * e1[1], c[2] * e1[0] - c[0] * e1[2], c[0] * e1[1] - c[1] * e1[0]])]; }

    function makeBody(spec) {
      var r = rng(spec.seed), B = { spec: spec, lobes: [], craters: [], rays: [], n1: makeNoise(spec.seed * 7 + 1), n2: makeNoise(spec.seed * 13 + 5), n3: makeNoise(spec.seed * 29 + 11) };
      function dir(zmin) {
        for (;;) {
          var u = r() * 2 - 1, v = r() * 2 - 1, w = r() * 2 - 1, q = u * u + v * v + w * w;
          if (q > 1 || q < 1e-4) continue;
          q = Math.sqrt(q); var d = [u / q, v / q, w / q];
          if (d[2] >= zmin) return d;
        }
      }
      var seas = spec.seas || [];
      if (!spec.seas) for (var i = 0; i < spec.basins; i++) { var bc0 = dir(.25); seas.push([bc0[0], bc0[1], spec.basinR[0] + r() * (spec.basinR[1] - spec.basinR[0]), .85 + r() * .3]); }
      seas.forEach(function (q) { var z0 = Math.sqrt(Math.max(0, 1 - q[0] * q[0] - q[1] * q[1])), an = q[6] || 0; B.lobes.push({ c: [q[0], q[1], z0], r: q[2], w: q[3] || 1, dk: q[4] || (.78 + r() * .36), st: q[5] || 1, ca: Math.cos(an), sa: Math.sin(an), soft: q[7] || 0 }); });
      B.warp = function (x, y, z) { return .3 * fbm(B.n1, x * 2.2 + 3, y * 2.2, z * 2.2, 2) + .2 * fbm(B.n2, x * 7.5 + 1, y * 7.5, z * 7.5 + 2, 3); };
      B.mid = function (x, y, z) { return fbm(B.n2, x * 3.4, y * 3.4 + 3, z * 3.4, 3); };
      B.flow = function (x, y, z) { return fbm(B.n1, x * 7.5 + 9, y * 7.5, z * 7.5, 2); };
      B.ocn = function (x, y, z) { return fbm(B.n2, x * 1.3, y * 1.3 + 7, z * 1.3, 4); };
      B.mareAt = function (x, y, z) { return seaMask(B, x, y, z, B.warp(x, y, z), .5, 0).m; };
      var i, k;
      var nC = spec.craters, rMin = spec.rMin, rMax = spec.rMax, q2 = (rMin / rMax) * (rMin / rMax);
      for (i = 0; i < nC * 3 && B.craters.length < nC; i++) {
        var cr = rMin / Math.sqrt(1 - r() * (1 - q2)), cc = dir(-.1);
        if (B.mareAt(cc[0], cc[1], cc[2]) > .5 && r() > spec.mareKeep) continue;
        var big = cr > .04, u0 = r(), age = u0 < .1 ? u0 * 2 : .35 + .65 * r();
        B.craters.push({ c: cc, r: cr, depth: cr * (big ? .12 : .22), big: big, age: age, dark: big && r() < .3, peak: cr > .055 });
      }
      for (i = 0; i < (spec.rayAt ? spec.rayAt.length : spec.rays); i++) {
        var ra = spec.rayAt && spec.rayAt[i], rc = ra ? [ra[0], ra[1], Math.sqrt(Math.max(0, 1 - ra[0] * ra[0] - ra[1] * ra[1]))] : dir(.1), rr = ra ? ra[2] : .018 + r() * .024, lobes = [], nl = 16 + ((r() * 12) | 0);
        for (k = 0; k < nl; k++) lobes.push({ a: r() * Math.PI * 2, w: .045 + r() * .1, len: 5 + r() * (spec.rayLen || 11), s: .45 + r() * .55 });
        B.craters.push({ c: rc, r: rr, depth: rr * .24, big: false, age: 0, dark: false, peak: false });
        var T2 = tangent(rc), mx = 0; lobes.forEach(function (l) { mx = Math.max(mx, l.len); });
        B.rays.push({ c: rc, r: rr, lobes: lobes, str: .85 + r() * .3, e1: T2[0], e2: T2[1], max: mx });
      }
      B.craters.sort(function (a, b) { return b.r - a.r; });
      return B;
    }
    var SEA = { m: 0, dk: 1, f: 0 };
    function seaMask(B, x, y, z, warp, soft01, wig) {
      var f = 0, dk = 1, sf = 0, dkw = 0, sfw = 0, L = B.lobes;
      for (var k = 0; k < L.length; k++) {
        var o = L[k], dx = x - o.c[0], dy = y - o.c[1], dz = z - o.c[2];
        if (o.st !== 1) { var u = dx * o.ca + dy * o.sa, v = -dx * o.sa + dy * o.ca; dx = u / o.st; dy = v; }
        var q = (dx * dx + dy * dy + dz * dz) / (o.r * o.r);
        if (q > 9) continue;
        var e = o.w * Math.exp(-q); f += e; dkw += e * o.dk; sfw += e * o.soft;
      }
      dk = f > 1e-6 ? dkw / f : 1; sf = f > 1e-6 ? sfw / f : 0;
      var hw = .01 + .09 * Math.max(sf, soft01 * soft01); SEA.f = f; SEA.m = smooth(.48 - hw, .48 + hw, f + warp + wig); SEA.dk = dk; return SEA;
    }

    function craterH(d, C) {
      if (d >= 2.4) return 0;
      var rim = .15 * (1 - .65 * C.age);
      if (d < 1) {
        var h = (-1 + (1 + rim) * Math.pow(d, C.big ? 4.2 - 1.6 * C.age : 2.2 - .7 * C.age)) * (1 - .5 * C.age);
        if (C.peak) h += .4 * (1 - .5 * C.age) * Math.exp(-(d * d) / .018);
        return h;
      }
      return rim * Math.exp(-(d - 1) * (3.4 - C.age)) * (1 - smooth(1.9, 2.4, d));
    }
    function grid(S, G, fn) {
      var GW = Math.floor((S - 1) / G) + 2, v = new Float32Array(GW * GW), c = S / 2, R = S / 2 - 1;
      for (var gj = 0; gj < GW; gj++) for (var gi = 0; gi < GW; gi++) {
        var x = (gi * G + .5 - c) / R, y = (c - (gj * G + .5)) / R, rr = Math.sqrt(x * x + y * y);
        if (rr > 1) { x /= rr; y /= rr; }
        v[gj * GW + gi] = fn(x, y, Math.sqrt(Math.max(0, 1 - x * x - y * y)));
      }
      return { v: v, W: GW, G: G };
    }
    function samp(g, i, j) {
      var fx = i / g.G, fy = j / g.G, x0 = fx | 0, y0 = fy | 0, tx = fx - x0, ty = fy - y0, W = g.W, v = g.v, k = y0 * W + x0;
      return (v[k] * (1 - tx) + v[k + 1] * tx) * (1 - ty) + (v[k + W] * (1 - tx) + v[k + W + 1] * tx) * ty;
    }

    function bodyKey(sp) { return sp._bk || (sp._bk = JSON.stringify([sp.seed, sp.seas, sp.basins, sp.basinR, sp.craters, sp.rMin, sp.rMax, sp.mareKeep, sp.rays, sp.rayAt, sp.rayLen, sp.hi, sp.lo, sp.relief])); }
    function skinKey(sp) { return sp._sk || (sp._sk = bodyKey(sp) + '|' + sp.color + '|' + sp.tint); }
    var PREP = {};
    function prepare(B, S) {
      var key = bodyKey(B.spec) + ':' + S; if (PREP[key]) return PREP[key];
      var N = S * S, c = S / 2, R = S / 2 - 1, sp = B.spec;
      var X = new Float32Array(N), Y = new Float32Array(N), Z = new Float32Array(N), cov = new Float32Array(N);
      var alb = new Float32Array(N), hgt = new Float32Array(N), mare = new Float32Array(N);
      var i, j, k, idx, G = S >= 180 ? 4 : S >= 72 ? 2 : 1, F = S >= 240 ? 2 : 1;
      var gW = grid(S, G, B.warp), gM = grid(S, G, B.mid), gF = grid(S, G, B.flow);
      var gFine = grid(S, F, function (x, y, z) { return fbm(B.n3, x * 10, y * 10, z * 10, S > 160 ? 4 : 3); });
      var gL = grid(S, G, function (x, y, z) { return fbm(B.n3, x * 3.2 - 4, y * 3.2, z * 3.2 + 1, 2); });
      for (j = 0; j < S; j++) for (i = 0; i < S; i++) {
        idx = j * S + i;
        var x = (i + .5 - c) / R, y = (c - (j + .5)) / R, rr = Math.sqrt(x * x + y * y);
        cov[idx] = Math.max(0, Math.min(1, (1 - rr) * R + .5));
        if (cov[idx] <= 0) continue;
        if (rr > 1) { x /= rr; y /= rr; }
        var z = Math.sqrt(Math.max(0, 1 - x * x - y * y));
        X[idx] = x; Y[idx] = y; Z[idx] = z;
        var fine = samp(gFine, i, j), mid = samp(gM, i, j), flow = samp(gF, i, j);
        var lv = samp(gL, i, j), s = seaMask(B, x, y, z, samp(gW, i, j), smooth(-.1, .35, lv), .12 * fine), m = s.m;
        mare[idx] = m;
        var shore = m * (1 - smooth(.55, .95, m)) * 2.2, hiA = sp.hi * (1 + .1 * mid + .08 * fine);
        var loA = sp.lo * s.dk * (1 + .5 * lv + .22 * flow + .06 * fine) * (1 - .12 * smooth(.6, 1.3, s.f)) * (1 - .1 * shore);
        alb[idx] = hiA * (1 - m) + loA * m;
        hgt[idx] = -.0045 * smooth(.2, .9, s.f) + (.0026 * fine + .002 * mid) * (1 - .8 * m);
      }
      B.craters.forEach(function (C) {
        if (C.r * R < .8) return;
        var reach = C.r * 2.4, x0 = Math.max(0, Math.floor(c + (C.c[0] - reach) * R)), x1 = Math.min(S - 1, Math.ceil(c + (C.c[0] + reach) * R));
        var y0 = Math.max(0, Math.floor(c - (C.c[1] + reach) * R)), y1 = Math.min(S - 1, Math.ceil(c - (C.c[1] - reach) * R));
        var fresh = 1 - smooth(.04, .3, C.age);
        for (var jj = y0; jj <= y1; jj++) for (var ii = x0; ii <= x1; ii++) {
          var q = jj * S + ii; if (cov[q] <= 0) continue;
          var dx = X[q] - C.c[0], dy = Y[q] - C.c[1], dz = Z[q] - C.c[2], d = Math.sqrt(dx * dx + dy * dy + dz * dz) / C.r;
          if (d >= 2.4) continue;
          hgt[q] += craterH(d, C) * C.depth;
          var a = alb[q], young = 1 - C.age;
          if (d < 1) {
            if (C.dark && d < .84) a += (sp.lo * .9 - a) * .75 * (1 - smooth(.66, .84, d));
            a += fresh * (.13 * (1 - smooth(.2, 1, d)) + .07) + .05 * young * smooth(.7, .98, d) - .02 * C.age * (1 - smooth(.3, .8, d));
          } else a += fresh * .17 * (1 - smooth(1, 2.2, d)) + .035 * young * (1 - smooth(1, 1.45, d));
          alb[q] = a;
        }
      });
      if (B.rays.length) {
        var gRay = grid(S, S >= 180 ? 2 : 1, function (x, y, z) {
          var s = 0;
          for (var n = 0; n < B.rays.length; n++) {
            var Ry = B.rays[n], cc = Ry.c, dx = x - cc[0], dy = y - cc[1], dz = z - cc[2], dist = Math.sqrt(dx * dx + dy * dy + dz * dz) / Ry.r;
            if (dist < 1 || dist > Ry.max) continue;
            var th = Math.atan2(dx * Ry.e2[0] + dy * Ry.e2[1] + dz * Ry.e2[2], dx * Ry.e1[0] + dy * Ry.e1[1] + dz * Ry.e1[2]), t0 = 0;
            for (var L = 0; L < Ry.lobes.length; L++) {
              var lb = Ry.lobes[L], t = dist / lb.len; if (t >= 1) continue;
              var da = th - lb.a; da -= Math.round(da / (Math.PI * 2)) * Math.PI * 2;
              var w = lb.w * (1 + dist * .05);
              t0 += lb.s * Math.exp(-(da * da) / (w * w)) * Math.pow(1 - t, 1.3);
            }
            s += (t0 * .3 * (.55 + .45 * B.n3(x * 38, y * 38, z * 38)) + Math.exp(-(dist - 1) * .8) * .16) * Ry.str;
          }
          return Math.min(.42, s);
        });
        for (j = 0; j < S; j++) for (i = 0; i < S; i++) { idx = j * S + i; if (cov[idx] > 0) alb[idx] += samp(gRay, i, j) * (1 - .3 * mare[idx]); }
      }
      var E = sp.relief * (S < 90 ? 1.9 : S < 200 ? 1.4 : 1.1), NX = new Float32Array(N), NY = new Float32Array(N), NZ = new Float32Array(N);
      for (j = 0; j < S; j++) for (i = 0; i < S; i++) {
        idx = j * S + i; if (cov[idx] <= 0) continue;
        var il = i > 0 && cov[idx - 1] > 0 ? idx - 1 : idx, ir = i < S - 1 && cov[idx + 1] > 0 ? idx + 1 : idx;
        var ju = j > 0 && cov[idx - S] > 0 ? idx - S : idx, jd = j < S - 1 && cov[idx + S] > 0 ? idx + S : idx;
        var hx = (hgt[ir] - hgt[il]) / Math.max(1, ir - il) * R * E, hy = -(hgt[jd] - hgt[ju]) / Math.max(1, (jd - ju) / S) * R * E;
        var xx = X[idx], yy = Y[idx], zz = Math.max(.12, Z[idx]), h1 = 1 + hgt[idx] * E;
        var ax = h1 + xx * hx, ay = yy * hx, az = -xx / zz * h1 + zz * hx;
        var bx = xx * hy, by = h1 + yy * hy, bz = -yy / zz * h1 + zz * hy;
        var nx = ay * bz - az * by, ny = az * bx - ax * bz, nz = ax * by - ay * bx, nl = Math.sqrt(nx * nx + ny * ny + nz * nz) || 1;
        var fl = smooth(.02, .3, Z[idx]); nx = nx / nl * fl + xx * (1 - fl); ny = ny / nl * fl + yy * (1 - fl); nz = nz / nl * fl + Z[idx] * (1 - fl); nl = Math.sqrt(nx * nx + ny * ny + nz * nz) || 1;
        NX[idx] = nx / nl; NY[idx] = ny / nl; NZ[idx] = nz / nl;
      }
      var HE = null, HW = null;
      if (S >= 64) {
        HE = new Float32Array(N); HW = new Float32Array(N);
        var PX = new Float32Array(N), PY = new Float32Array(N), PZ = new Float32Array(N);
        for (idx = 0; idx < N; idx++) { if (cov[idx] <= 0) continue; var hh = 1 + hgt[idx] * E; PX[idx] = X[idx] * hh; PY[idx] = Y[idx] * hh; PZ[idx] = Z[idx] * hh; }
        var steps = [1, 2, 3, 4, 6, 8, 11, 15, 20, 27, 36, 48, 64, 85].filter(function (s) { return s < S * .45; }), ns = steps.length;
        for (j = 0; j < S; j++) {
          var row = j * S;
          for (i = 0; i < S; i++) {
            idx = row + i; if (cov[idx] <= 0) continue;
            var px = X[idx], py = Y[idx], pz = Z[idx], Px = PX[idx], Py = PY[idx], Pz = PZ[idx], be = -1, bw = -1;
            for (k = 0; k < ns; k++) {
              var ie = i + steps[k]; if (ie >= S) break; var qe = row + ie; if (cov[qe] <= 0) break;
              var Dx = PX[qe] - Px, Dy = PY[qe] - Py, Dz = PZ[qe] - Pz, el = (Dx * px + Dy * py + Dz * pz) / Math.sqrt(Dx * Dx + Dy * Dy + Dz * Dz);
              if (el > be) be = el;
            }
            for (k = 0; k < ns; k++) {
              var iw = i - steps[k]; if (iw < 0) break; var qw = row + iw; if (cov[qw] <= 0) break;
              var Ex = PX[qw] - Px, Ey = PY[qw] - Py, Ez = PZ[qw] - Pz, el2 = (Ex * px + Ey * py + Ez * pz) / Math.sqrt(Ex * Ex + Ey * Ey + Ez * Ez);
              if (el2 > bw) bw = el2;
            }
            HE[idx] = be; HW[idx] = bw;
          }
        }
      }
      return (PREP[key] = { S: S, X: X, Y: Y, Z: Z, cov: cov, alb: alb, NX: NX, NY: NY, NZ: NZ, HE: HE, HW: HW });
    }

    function lin(v) { v /= 255; return v <= .04045 ? v / 12.92 : Math.pow((v + .055) / 1.055, 2.4); }
    var GAM = new Uint8ClampedArray(4096);
    for (var gi = 0; gi < 4096; gi++) { var gv = gi / 4095; GAM[gi] = Math.round(255 * (gv <= .0031308 ? gv * 12.92 : 1.055 * Math.pow(gv, 1 / 2.4) - .055)); }
    function tintOf(hex, amt) {
      var n = parseInt(hex.slice(1), 16), c = [lin((n >> 16) & 255), lin((n >> 8) & 255), lin(n & 255)], Yl = .2126 * c[0] + .7152 * c[1] + .0722 * c[2];
      return c.map(function (v) { return 1 + (v / Yl - 1) * amt; });
    }
    function litFrac(p) { return (1 - Math.cos(2 * Math.PI * p)) / 2; }
    var BL0 = [lin(20), lin(2), lin(5)], BL1 = [lin(104), lin(12), lin(22)], SHEEN = [.92, .74, .78];

    function shade(B, S, p, o) {
      o = o || {};
      var P = prepare(B, S), sp = o.spec || B.spec, N = S * S, img = new ImageData(S, S), D = img.data;
      var th = 2 * Math.PI * p, Lx = Math.sin(th), Lz = -Math.cos(th), H = Lx >= 0 ? P.HE : P.HW, tint = tintOf(sp.color, sp.tint);
      var blood = !!o.blood, soft = .012 + 1.2 / S;
      var lf = litFrac(p), es = .016 * Math.pow(1 - lf, 2), ang = Math.min(1, Math.abs(Lx) / .06);
      var wl = .06 + .3 * Math.sqrt(Math.abs(Lx));
      var nR = .0065, nG = .0075, nB = .011;
      for (var i = 0; i < N; i++) {
        var cv = P.cov[i]; if (cv <= 0) continue;
        var x = P.X[i], z = P.Z[i], a = P.alb[i], nx = P.NX[i], nz = P.NZ[i];
        var mu0s = x * Lx + z * Lz, mu0 = nx * Lx + nz * Lz, mu = nz > .05 ? nz : .05, sh;
        if (H) { var hz = H[i] * ang; sh = smooth(hz - soft, hz + soft, mu0s); }
        else sh = smooth(-soft * 1.5, soft * 1.5, mu0s);
        var I = 0;
        if (mu0 > 0) { var ls = 2 * mu0 / (mu0 + mu); I = a * ((1 - wl) * ls + wl * mu0) * (sh + (1 - sh) * .07) * (.8 + .2 * Math.sqrt(z)) * 1.1; }
        else if (mu0s > 0) I = a * .035 * mu0s;
        var dark = 1 - smooth(-.06, .1, mu0s), Es = es * a * (.4 + .6 * z) * dark, r, g, b;
        if (blood) {
          var yy = P.Y[i], ny = P.NY[i], heavy = smooth(-.15, -.92, yy), clot = smooth(.3, .85, a / sp.hi + .18 * (nx * -.4 + ny * .6));
          var t = clot * (1 - .7 * heavy), v = (.62 + .38 * z) * (1 - .35 * heavy);
          var nd = -.36 * nx + .74 * ny + .57 * nz, gl = Math.max(0, 2 * nd * nz - .57), spec = (Math.pow(gl, 26) * .5 + Math.pow(gl, 7) * .045) * (.35 + .65 * clot);
          var rr = Math.sqrt(x * x + yy * yy), men = smooth(.88, .965, rr) * (1 - smooth(.97, 1, rr)) * smooth(-.05, -.75, yy) * .09;
          r = (BL0[0] + (BL1[0] - BL0[0]) * t) * v + (spec + men) * SHEEN[0]; g = (BL0[1] + (BL1[1] - BL0[1]) * t) * v + (spec + men * .45) * SHEEN[1]; b = (BL0[2] + (BL1[2] - BL0[2]) * t) * v + (spec + men * .5) * SHEEN[2];
        } else {
          var vv = I + Es;
          r = vv * tint[0] + nR * dark; g = vv * tint[1] + nG * dark; b = vv * tint[2] + nB * dark;
        }
        var k4 = i * 4;
        D[k4] = GAM[r >= 1 ? 4095 : (r * 4095) | 0]; D[k4 + 1] = GAM[g >= 1 ? 4095 : (g * 4095) | 0]; D[k4 + 2] = GAM[b >= 1 ? 4095 : (b * 4095) | 0];
        D[k4 + 3] = Math.round(cv * 255);
      }
      return img;
    }
    var CACHE = {}, BODIES = {};
    function body(spec) { var k = bodyKey(spec); return BODIES[k] || (BODIES[k] = makeBody(spec)); }
    function sprite(spec, p, S, o) {
      o = o || {};
      var bucket = Math.round(((p % 1) + 1) % 1 * 120) % 120, key = skinKey(spec) + ':' + S + ':' + bucket + (o.blood ? ':b' : '');
      if (CACHE[key]) return CACHE[key];
      var cv = document.createElement('canvas'); cv.width = S; cv.height = S;
      cv.getContext('2d').putImageData(shade(body(spec), S, bucket / 120, { blood: o.blood, spec: spec }), 0, 0);
      return (CACHE[key] = cv);
    }
    return { sprite: sprite, shade: shade, body: body, prepare: prepare, rng: rng, makeNoise: makeNoise, fbm: fbm, key: skinKey };
  })();

  // ── Chronicle's moon-library: maps a real calendar_moons row onto a MOONR
  // spec. A handful of named bodies are hand-authored; anything else gets a
  // deterministic generated body so every BaseDesign value renders something
  // plausible and stable (same key -> same body, forever). ──
  var LIB = {
    'moon-realistic-selene': { seed: 11, seas: [[-.4, .38, .28, 1.05, .86], [.1, .52, .15, 1, .95], [.38, .24, .16, 1, 1.05, 1.5, .6], [.02, -.3, .2, .9, .8, 1.6, -.3, .35], [.7, -.1, .1, 1.05, .9], [-.66, -.2, .26, .85, 1.02, 1.7, 1.3, .5], [-.3, 0, .14, .8, .92, 1, 0, .3]], craters: 3200, rMin: .0045, rMax: .15, mareKeep: .35, rayAt: [[.08, -.7, .024], [-.18, .14, .03], [.55, .55, .02]], hi: .58, lo: .2, relief: 1 },
    'moon-realistic-umbra': { seed: 23, seas: [[.3, .32, .17, 1, .9, 1.4, .4], [-.42, -.18, .13, 1, 1.05], [-.05, .55, .1, .9, .95, 1, 0, .5]], craters: 3000, rMin: .0045, rMax: .13, mareKeep: .45, rays: 5, rayLen: 13, hi: .64, lo: .27, relief: 1 },
    'moon-realistic-ember': { seed: 37, seas: [[-.2, .12, .2, 1, .9, 1.3, -.8], [.45, -.35, .1, .9, 1.05]], craters: 3600, rMin: .0045, rMax: .17, mareKeep: .6, rays: 1, hi: .5, lo: .27, relief: 1.35 },
    'moon-realistic-tide': { seed: 53, seas: [[.08, -.04, .46, 1.25, .78, 1, 0, .25], [-.62, .5, .12, .9], [.65, .55, .09, .9]], craters: 2600, rMin: .005, rMax: .14, mareKeep: .35, rays: 0, hi: .52, lo: .19, relief: 1.1 }
  };

  function hashSeed(s) {
    s = String(s || '');
    var v = 0;
    for (var i = 0; i < s.length; i++) v = (v * 131 + s.charCodeAt(i)) >>> 0;
    return (v % 90000) + 5;
  }
  // A generated body for a BaseDesign we have no hand-authored art for: its
  // shape is a function of the design key alone (stable across reloads), and
  // its colour of the moon's own Color field (set once more below).
  function generatedSpec(designKey) {
    var seed = hashSeed(designKey), r = MOONR.rng(seed * 9301 + 49297);
    var basins = 2 + Math.floor(r() * 3);
    return {
      seed: seed, basins: basins, basinR: [.14, .3],
      craters: 2400 + Math.floor(r() * 1400), rMin: .0045, rMax: .1 + r() * .08,
      mareKeep: .3 + r() * .3, rays: Math.floor(r() * 4), rayLen: 8 + r() * 6,
      hi: .48 + r() * .14, lo: .18 + r() * .12, relief: .9 + r() * .5
    };
  }

  var SPECS = {};
  // specFor(moon): moon is a real calendar_moons row ({BaseDesign, Color,
  // Tint}) or the mockup-shaped {baseDesign, color}. Cached per (design,
  // color, tint) key, since two moons sharing a design still need their own
  // spec object when their colour differs (MOONR bakes colour into the spec).
  function specFor(moon) {
    var design = (moon.base_design || moon.BaseDesign || moon.baseDesign || 'moon-realistic-selene');
    var color = moon.color || moon.Color || '#d9e1ec';
    var tintRaw = moon.tint || moon.Tint;
    var tint = tintRaw != null ? Number(tintRaw) : .4;
    if (!(tint >= 0 && tint <= 1)) tint = .4;
    var key = design + '|' + color + '|' + tint;
    if (SPECS[key]) return SPECS[key];
    var base = LIB[design] || generatedSpec(design);
    var spec = {};
    for (var k in base) if (Object.prototype.hasOwnProperty.call(base, k)) spec[k] = base[k];
    spec.color = /^#[0-9a-fA-F]{3,6}$/.test(color) ? color : '#d9e1ec';
    spec.tint = tint;
    return (SPECS[key] = spec);
  }

  window.MOONR = MOONR;
  window.SkyMoon = { specFor: specFor, library: LIB };
})();
